// main_e2e_test.go 根包（main）的 e2e 全链路测试
//
// 背景：main.go 全部 492 行都是唯一的 main() 函数，其中包含
// http.ListenAndServe 与信号阻塞等待，且禁止修改生产代码抽函数，
// 因此 main() 本体不可直接调用。本文件按 main.go 完全相同的装配顺序
// （配置 -> 日志 -> 数据库 -> 服务 -> 中间件 -> 路由）在测试内重建
// gin 引擎，用 glebarez/sqlite 内存库替换全局 database.DB，用
// zap.NewNop 兜底全局 logger.Logger，然后 httptest 全链路验证：
//
//   - 健康检查 /api/v1/health
//   - 静态资源（/、/favicon.ico、/static、/api/v1/docs*）200 与缺失文件 404
//   - SPA history 路由兜底（非 /api/ 前缀回退 index.html；/api/ 前缀 JSON 404）
//   - CORS 预检与通配回显
//   - 未带 token 访问受保护接口 401（含格式错误 / 非法 token）
//   - 登录接口：参数校验 400、错误密码 401、admin 登录成功拿 token
//   - 携带 token 访问业务接口 200（me / images / sync / config / acr / 用户与角色管理）
//   - RBAC 权限隔离：普通用户访问 config/users 接口 403，sync/images 200
//   - 请求体大小限制（>5MB 400）
//   - 登出后 token 进黑名单立即 401
//
// 不访问真实外网：/api/v1/github/* 四个路由会真实调用 api.github.com
//（GitHubService 未提供 baseURL/客户端注入点），故只注册不请求。
//
// 注：断言使用标准库 + 本文件内的轻量辅助函数，不引入 testify ——
// 直接 import testify 需要 go.mod 增补 go.yaml.in/yaml/v3 require 行，
// 而本任务禁止修改 go.mod / go.sum。
package main

import (
	"bytes"
	"context"
	"database/sql"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
	"testing"

	"docker-image-sync-platform/internal/config"
	"docker-image-sync-platform/internal/database"
	"docker-image-sync-platform/internal/logger"
	"docker-image-sync-platform/internal/models"

	"github.com/gin-gonic/gin"
	"github.com/glebarez/sqlite"
	"go.uber.org/zap"
	"gorm.io/gorm"
)

func TestMain(m *testing.M) {
	gin.SetMode(gin.TestMode)
	// 全局 logger 兜底：泄漏的后台协程（如 ListenAndServe）可能在测试结束后写日志
	if logger.Logger == nil {
		logger.Logger = zap.NewNop()
	}
	os.Exit(m.Run())
}

// ---------------------------------------------------------------------------
// 轻量断言辅助（替代 testify，见文件头说明）
// ---------------------------------------------------------------------------

func mustStatus(t *testing.T, w *httptest.ResponseRecorder, want int, ctx ...string) {
	t.Helper()
	if w.Code != want {
		prefix := ""
		if len(ctx) > 0 {
			prefix = " (" + strings.Join(ctx, " ") + ")"
		}
		t.Fatalf("状态码 = %d, 期望 %d%s; body=%s", w.Code, want, prefix, w.Body.String())
	}
}

func mustContains(t *testing.T, s, sub string, ctx ...string) {
	t.Helper()
	if !strings.Contains(s, sub) {
		t.Fatalf("响应缺少 %q%s; 内容=%s", sub, strings.Join(ctx, " "), s)
	}
}

func mustJSON(t *testing.T, w *httptest.ResponseRecorder) map[string]any {
	t.Helper()
	var m map[string]any
	if err := json.Unmarshal(w.Body.Bytes(), &m); err != nil {
		t.Fatalf("解析响应 JSON 失败: %v; body=%s", err, w.Body.String())
	}
	return m
}

func mustStr(t *testing.T, m map[string]any, key string) string {
	t.Helper()
	v, ok := m[key].(string)
	if !ok {
		t.Fatalf("响应字段 %q 不是字符串: %#v", key, m[key])
	}
	return v
}

func failIf(t *testing.T, err error, format string, args ...any) {
	t.Helper()
	if err != nil {
		t.Fatalf(format+": %v", append(args, err)...)
	}
}

// ---------------------------------------------------------------------------
// fixture 与环境
// ---------------------------------------------------------------------------

// e2eConfigYAML 与生产 config.yaml 同构的测试配置。
// aliyun.password 留非空值，使 initDefaultConfigs 产生 ENC: 记录，
// 从而让 MigrateEncryptionKeys 真正走到查询与解密校验分支。
const e2eConfigYAML = `server:
  port: "18080"
  mode: debug

database:
  host: 127.0.0.1
  port: 3306
  username: root
  password: e2e-not-used
  database: docker_sync_e2e
  charset: utf8mb4
  parse_time: true
  loc: Local

log:
  level: info
  file_path: ./logs/app.log
  max_size: 10
  max_backups: 1
  max_age: 1

auth:
  jwt_secret: e2e-test-jwt-secret
  token_expiry: 1h
  remember_me_expiry: 2h
  default_admin_username: admin
  default_admin_password: admin123

aliyun:
  registry: registry.cn-hangzhou.aliyuncs.com
  namespace: e2e-namespace
  username: e2e-user
  password: e2e-aliyun-secret
`

// ---------------------------------------------------------------------------
// sqlite DDL 兼容层
//
// 生产模型（ImageSyncRecord.SyncStatus / SyncTask.Status 等）带 MySQL 专用的
// `type:enum(...)` 标签，直接在 glebarez/sqlite 上 AutoMigrate 会因
// `enum('a','b')` 语法报错。生产代码禁止修改，故在测试侧包装连接池，
// 将 DDL 中的 enum(...) 列类型改写为 text、双引号 DEFAULT 改写为单引号。
// 仅影响本测试文件创建的连接。
// ---------------------------------------------------------------------------

var (
	enumTypeRe  = regexp.MustCompile(`(?i)enum\([^)]*\)`)
	doubleQuote = regexp.MustCompile(`(?i)(DEFAULT\s+)"([^"]*)"`)
)

func rewriteSQLiteDDL(query string) string {
	if strings.Contains(strings.ToLower(query), "enum(") {
		query = enumTypeRe.ReplaceAllString(query, "text")
	}
	if doubleQuote.MatchString(query) {
		query = doubleQuote.ReplaceAllString(query, `${1}'$2'`)
	}
	return query
}

type ddlRewriteDB struct{ *sql.DB }

func (d ddlRewriteDB) ExecContext(ctx context.Context, query string, args ...any) (sql.Result, error) {
	if ddlDebugLog != nil && strings.Contains(strings.ToLower(query), "acr") {
		ddlDebugLog("EXEC: %.120s", rewriteSQLiteDDL(query))
	}
	return d.DB.ExecContext(ctx, rewriteSQLiteDDL(query), args...)
}

func (d ddlRewriteDB) QueryContext(ctx context.Context, query string, args ...any) (*sql.Rows, error) {
	if ddlDebugLog != nil && strings.Contains(strings.ToLower(query), "acr") {
		ddlDebugLog("QUERY: %.120s", rewriteSQLiteDDL(query))
	}
	return d.DB.QueryContext(ctx, rewriteSQLiteDDL(query), args...)
}

func (d ddlRewriteDB) QueryRowContext(ctx context.Context, query string, args ...any) *sql.Row {
	return d.DB.QueryRowContext(ctx, rewriteSQLiteDDL(query), args...)
}

func (d ddlRewriteDB) PrepareContext(ctx context.Context, query string) (*sql.Stmt, error) {
	return d.DB.PrepareContext(ctx, rewriteSQLiteDDL(query))
}

// BeginTx 签名必须返回 gorm.ConnPool（gorm v1.25.7 ConnPoolBeginner 的要求，
// 返回 gorm.Tx 会导致接口断言失败、Begin 走 default 分支报 invalid transaction）。
// 事务包装器一律用指针接收者：gorm Commit 里的 reflect IsNil 检查对 struct 值会 panic。
func (d ddlRewriteDB) BeginTx(ctx context.Context, opts *sql.TxOptions) (gorm.ConnPool, error) {
	if ddlDebugLog != nil {
		ddlDebugLog("BeginTx on ddlRewriteDB")
	}
	tx, err := d.DB.BeginTx(ctx, opts)
	if err != nil {
		return nil, err
	}
	return &ddlRewriteTx{tx}, nil
}

// ddlDebugLog 仅供临时调试，正常为 nil。
var ddlDebugLog func(format string, args ...any)

type ddlRewriteTx struct{ *sql.Tx }

func (t *ddlRewriteTx) ExecContext(ctx context.Context, query string, args ...any) (sql.Result, error) {
	return t.Tx.ExecContext(ctx, rewriteSQLiteDDL(query), args...)
}

func (t *ddlRewriteTx) QueryContext(ctx context.Context, query string, args ...any) (*sql.Rows, error) {
	return t.Tx.QueryContext(ctx, rewriteSQLiteDDL(query), args...)
}

func (t *ddlRewriteTx) QueryRowContext(ctx context.Context, query string, args ...any) *sql.Row {
	return t.Tx.QueryRowContext(ctx, rewriteSQLiteDDL(query), args...)
}

func (t *ddlRewriteTx) PrepareContext(ctx context.Context, query string) (*sql.Stmt, error) {
	return t.Tx.PrepareContext(ctx, rewriteSQLiteDDL(query))
}

func (t *ddlRewriteTx) Commit() error {
	if ddlDebugLog != nil {
		ddlDebugLog("Commit on ddlRewriteTx")
	}
	return t.Tx.Commit()
}

func (t *ddlRewriteTx) Rollback() error {
	if ddlDebugLog != nil {
		ddlDebugLog("Rollback on ddlRewriteTx")
	}
	return t.Tx.Rollback()
}

// setupE2E 准备 fixture 文件、全局状态并按 main.go 装配出完整 gin 引擎。
func setupE2E(t *testing.T) *gin.Engine {
	t.Helper()

	// fixture：config.yaml、前端静态资源、docs 目录（SPA 回退需要真实文件）
	tmp := t.TempDir()
	writeFixture := func(rel, content string) {
		t.Helper()
		abs := filepath.Join(tmp, filepath.FromSlash(rel))
		failIf(t, os.MkdirAll(filepath.Dir(abs), 0o755), "创建目录 %s 失败", abs)
		failIf(t, os.WriteFile(abs, []byte(content), 0o644), "写入文件 %s 失败", abs)
	}
	writeFixture("config.yaml", e2eConfigYAML)
	writeFixture("web/dist/index.html", "<!doctype html><html><body>docker-sync-platform e2e</body></html>")
	writeFixture("web/dist/favicon.ico", "fake-icon-bytes")
	writeFixture("web/dist/static/app.css", "body{color:#333}")
	writeFixture("docs/swagger.json", `{"swagger":"2.0","info":{"title":"docker-image-sync-platform"}}`)
	writeFixture("docs/swagger-ui.html", "<html>swagger-ui</html>")
	chdir(t, tmp)

	// 环境变量兜底：固定加密密钥；GIN_MODE=debug 防止外部 release 环境干扰
	t.Setenv("ENCRYPTION_KEY", "e2e-encryption-key")
	t.Setenv("GIN_MODE", "debug")

	// 全局日志兜底：zap.NewNop（参考 internal/logger），测试结束恢复
	savedLogger := logger.Logger
	logger.Logger = zap.NewNop()
	t.Cleanup(func() { logger.Logger = savedLogger })

	// 加载配置（与 main.go 相同的调用方式）
	failIf(t, config.LoadConfig("config.yaml"), "加载配置失败")

	// 用 glebarez/sqlite 内存库替换全局 database.DB（保存 + 恢复）
	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{
		DisableForeignKeyConstraintWhenMigrating: true,
	})
	failIf(t, err, "打开内存 SQLite 失败")
	sqlDB, err := db.DB()
	failIf(t, err, "获取底层连接失败")
	// 包装连接池：生产模型的 MySQL enum 列类型在 sqlite 上不可用，见 ddlRewriteDB 注释
	// 注意 gorm 在 Open 时已把连接池写入 Statement 模板，两处都要替换
	db.ConnPool = ddlRewriteDB{DB: sqlDB}
	db.Statement.ConnPool = ddlRewriteDB{DB: sqlDB}
	savedDB := database.DB
	database.DB = db
	t.Cleanup(func() { database.DB = savedDB })

	// 与 main.go 相同：AutoMigrate 会 seed 角色/权限并创建默认管理员 admin/admin123
	failIf(t, database.AutoMigrate(), "数据库表迁移失败")
	failIf(t, database.MigrateEncryptionKeys(), "加密密钥迁移失败")
	// main.go 中 ACR 相关服务使用的表（database.AutoMigrate 未覆盖）
	failIf(t, db.AutoMigrate(&models.AcrRegistry{}, &models.AcrRepository{}), "ACR 表迁移失败")

	// 用真实的生产装配函数（initBusinessServices + setupRouter）构建引擎，
	// 而非复刻路由表 —— e2e 走的就是线上代码路径
	app, err := initBusinessServices()
	failIf(t, err, "初始化业务服务失败")
	gin.SetMode(gin.TestMode) // setupRouter 只在 release 配置时覆盖模式，这里强制回 TestMode
	router := setupRouter(app)
	return router
}

// doRequest 发送 JSON 请求的辅助函数；token 非空时附带 Bearer 头。
func doRequest(t *testing.T, router *gin.Engine, method, path, token string, body any) *httptest.ResponseRecorder {
	t.Helper()
	var reader io.Reader
	if body != nil {
		raw, err := json.Marshal(body)
		failIf(t, err, "构造请求体失败")
		reader = bytes.NewReader(raw)
	}
	req := httptest.NewRequest(method, path, reader)
	req.Header.Set("Content-Type", "application/json")
	if token != "" {
		req.Header.Set("Authorization", "Bearer "+token)
	}
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)
	return w
}

// ---------------------------------------------------------------------------
// e2e 主流程（子测试按依赖顺序执行，共享同一引擎与数据库）
// ---------------------------------------------------------------------------

func TestMainE2EFlow(t *testing.T) {
	router := setupE2E(t)

	do := func(method, path, token string, body any) *httptest.ResponseRecorder {
		t.Helper()
		return doRequest(t, router, method, path, token, body)
	}

	// ==================================================================
	// 1. 健康检查（公开接口）
	// ==================================================================
	t.Run("健康检查", func(t *testing.T) {
		w := do(http.MethodGet, "/api/v1/health", "", nil)
		mustStatus(t, w, http.StatusOK)
		body := mustJSON(t, w)
		if got := mustStr(t, body, "status"); got != "ok" {
			t.Errorf("status = %q, 期望 ok", got)
		}
		if got := mustStr(t, body, "version"); got != "1.0.0" {
			t.Errorf("version = %q, 期望 1.0.0", got)
		}
		if ts, ok := body["timestamp"].(float64); !ok || ts <= 0 {
			t.Errorf("timestamp = %#v, 期望正数", body["timestamp"])
		}
	})

	// ==================================================================
	// 2. 静态资源：入口页 / favicon / /static / docs
	// ==================================================================
	t.Run("静态资源", func(t *testing.T) {
		w := do(http.MethodGet, "/", "", nil)
		mustStatus(t, w, http.StatusOK)
		mustContains(t, w.Body.String(), "docker-sync-platform e2e")
		mustContains(t, w.Header().Get("Content-Type"), "text/html")

		w = do(http.MethodGet, "/favicon.ico", "", nil)
		mustStatus(t, w, http.StatusOK)

		w = do(http.MethodGet, "/static/app.css", "", nil)
		mustStatus(t, w, http.StatusOK)
		mustContains(t, w.Header().Get("Content-Type"), "text/css")
		if w.Body.String() != "body{color:#333}" {
			t.Errorf("静态文件内容不符: %s", w.Body.String())
		}

		// 静态目录中不存在的文件 -> 实际走 SPA 回退返回 index.html（NoRoute 兜底）
		w = do(http.MethodGet, "/static/missing.css", "", nil)
		mustStatus(t, w, http.StatusOK, "missing.css")
		mustContains(t, w.Body.String(), "docker-sync-platform e2e", "missing.css 应回退 index.html")

		w = do(http.MethodGet, "/api/v1/docs/swagger.json", "", nil)
		mustStatus(t, w, http.StatusOK)
		mustContains(t, w.Body.String(), "docker-image-sync-platform")

		w = do(http.MethodGet, "/api/v1/docs.html", "", nil)
		mustStatus(t, w, http.StatusOK)
	})

	// ==================================================================
	// 3. SPA history 路由兜底
	// ==================================================================
	t.Run("SPA回退与API未知路径404", func(t *testing.T) {
		for _, path := range []string{"/sync", "/images/2/argocd/tags", "/login"} {
			w := do(http.MethodGet, path, "", nil)
			mustStatus(t, w, http.StatusOK, "path="+path)
			mustContains(t, w.Body.String(), "docker-sync-platform e2e", "path="+path)
		}

		for _, path := range []string{"/api/v1/nonexistent", "/api/unknown"} {
			w := do(http.MethodGet, path, "", nil)
			mustStatus(t, w, http.StatusNotFound, "path="+path)
			body := mustJSON(t, w)
			if got := mustStr(t, body, "error"); got != "接口不存在" {
				t.Errorf("未知 API 错误文案 = %q, 期望 接口不存在", got)
			}
		}
	})

	// ==================================================================
	// 4. CORS：通配来源回显 + OPTIONS 预检 204
	// ==================================================================
	t.Run("CORS", func(t *testing.T) {
		w := do(http.MethodGet, "/api/v1/health", "", nil)
		if got := w.Header().Get("Access-Control-Allow-Origin"); got != "*" {
			t.Errorf("Access-Control-Allow-Origin = %q, 期望 *", got)
		}

		req := httptest.NewRequest(http.MethodOptions, "/api/v1/health", nil)
		req.Header.Set("Origin", "http://example.com")
		req.Header.Set("Access-Control-Request-Method", "POST")
		rec := httptest.NewRecorder()
		router.ServeHTTP(rec, req)
		mustStatus(t, rec, http.StatusNoContent)
		if got := rec.Header().Get("Access-Control-Allow-Origin"); got != "*" {
			t.Errorf("预检 Access-Control-Allow-Origin = %q, 期望 *", got)
		}
	})

	// ==================================================================
	// 5. 未认证访问受保护接口 -> 401
	// ==================================================================
	t.Run("未认证401", func(t *testing.T) {
		for _, tc := range []struct{ method, path string }{
			{http.MethodGet, "/api/v1/auth/me"},
			{http.MethodGet, "/api/v1/images/list"},
			{http.MethodGet, "/api/v1/sync/history"},
			{http.MethodPost, "/api/v1/sync/submit"},
			{http.MethodGet, "/api/v1/config/status"},
			{http.MethodGet, "/api/v1/acr-registries"},
		} {
			w := do(tc.method, tc.path, "", nil)
			mustStatus(t, w, http.StatusUnauthorized, tc.method+" "+tc.path)
			body := mustJSON(t, w)
			if got := mustStr(t, body, "error"); got != "未提供认证信息" {
				t.Errorf("%s %s 错误文案 = %q", tc.method, tc.path, got)
			}
		}

		// 非法 Authorization 头格式
		req := httptest.NewRequest(http.MethodGet, "/api/v1/auth/me", nil)
		req.Header.Set("Authorization", "Token abc.def.ghi")
		rec := httptest.NewRecorder()
		router.ServeHTTP(rec, req)
		mustStatus(t, rec, http.StatusUnauthorized)
		body := mustJSON(t, rec)
		if got := mustStr(t, body, "error"); got != "认证格式错误" {
			t.Errorf("错误文案 = %q, 期望 认证格式错误", got)
		}

		// Bearer 携带伪造 token -> 验签失败
		w := do(http.MethodGet, "/api/v1/auth/me", "fake.token.value", nil)
		mustStatus(t, w, http.StatusUnauthorized)
		body = mustJSON(t, w)
		if got := mustStr(t, body, "error"); got != "认证已过期，请重新登录" {
			t.Errorf("错误文案 = %q, 期望 认证已过期，请重新登录", got)
		}
	})

	// ==================================================================
	// 6. 登录接口：参数校验 / 错误密码 / 成功
	// ==================================================================
	var adminToken string
	t.Run("登录", func(t *testing.T) {
		// 缺少字段 -> 400
		w := do(http.MethodPost, "/api/v1/auth/login", "", map[string]any{})
		mustStatus(t, w, http.StatusBadRequest)

		// 错误密码 -> 401
		w = do(http.MethodPost, "/api/v1/auth/login", "", map[string]any{
			"username": "admin", "password": "wrong-password",
		})
		mustStatus(t, w, http.StatusUnauthorized)

		// database.AutoMigrate seed 的默认管理员（database 包同款方式）
		w = do(http.MethodPost, "/api/v1/auth/login", "", map[string]any{
			"username": "admin", "password": "admin123",
		})
		mustStatus(t, w, http.StatusOK)
		var body struct {
			Token string         `json:"token"`
			User  map[string]any `json:"user"`
		}
		failIf(t, json.Unmarshal(w.Body.Bytes(), &body), "解析登录响应失败")
		if body.Token == "" {
			t.Fatalf("登录未返回 token: %s", w.Body.String())
		}
		if got := mustStr(t, body.User, "username"); got != "admin" {
			t.Errorf("登录用户 = %q, 期望 admin", got)
		}
		adminToken = body.Token
	})

	// ==================================================================
	// 7. 携带 token 访问各业务模块（管理员拥有全部权限）
	// ==================================================================
	t.Run("认证后业务接口", func(t *testing.T) {
		if adminToken == "" {
			t.Fatal("前置条件失败：无 admin token")
		}

		// 当前用户
		w := do(http.MethodGet, "/api/v1/auth/me", adminToken, nil)
		mustStatus(t, w, http.StatusOK)
		me := mustJSON(t, w)
		if got := mustStr(t, me, "username"); got != "admin" {
			t.Errorf("me.username = %q, 期望 admin", got)
		}

		// 角色下拉选项（仅需登录）
		w = do(http.MethodGet, "/api/v1/auth/roles/options", adminToken, nil)
		mustStatus(t, w, http.StatusOK)
		var opts struct {
			Data []map[string]any `json:"data"`
		}
		failIf(t, json.Unmarshal(w.Body.Bytes(), &opts), "解析角色选项失败")
		if len(opts.Data) != 3 {
			t.Errorf("角色数量 = %d, 期望 3（admin/operator/user）, body=%s", len(opts.Data), w.Body.String())
		}

		// 镜像模块（PermissionRequiredAny(sync, images)）
		for _, path := range []string{"/api/v1/images/list", "/api/v1/images/stats"} {
			w = do(http.MethodGet, path, adminToken, nil)
			mustStatus(t, w, http.StatusOK, path)
		}

		// 同步模块（sync 权限）
		w = do(http.MethodGet, "/api/v1/sync/history", adminToken, nil)
		mustStatus(t, w, http.StatusOK)

		// 配置模块（config 权限）
		w = do(http.MethodGet, "/api/v1/config/status", adminToken, nil)
		mustStatus(t, w, http.StatusOK)

		// ACR 配置 / 台账
		for _, path := range []string{
			"/api/v1/acr-registries",
			"/api/v1/acr-registries/quota-summary",
		} {
			w = do(http.MethodGet, path, adminToken, nil)
			mustStatus(t, w, http.StatusOK, path)
		}

		// 创建一条 ACR 配置（管理员有 config 权限），再用其 ID 拉台账
		w = do(http.MethodPost, "/api/v1/acr-registries", adminToken, map[string]any{
			"registry_url":  "registry.cn-hangzhou.aliyuncs.com",
			"namespace":     "e2e-ns",
			"alias":         "e2e-alias",
			"username":      "e2e-user",
			"password":      "e2e-pass-123",
			"registry_type": "acr",
		})
		mustStatus(t, w, http.StatusCreated, "创建 ACR 配置")
		var acrCreated struct {
			Data struct {
				ID uint `json:"id"`
			} `json:"data"`
		}
		failIf(t, json.Unmarshal(w.Body.Bytes(), &acrCreated), "解析 ACR 创建响应失败")
		if acrCreated.Data.ID == 0 {
			t.Fatalf("ACR 配置创建未返回 id: %s", w.Body.String())
		}
		w = do(http.MethodGet, "/api/v1/acr-repositories?acr_registry_id="+strconv.FormatUint(uint64(acrCreated.Data.ID), 10), adminToken, nil)
		mustStatus(t, w, http.StatusOK, "acr-repositories")

		// 用户与角色管理（users / roles 权限）
		for _, path := range []string{
			"/api/v1/auth/users",
			"/api/v1/auth/roles",
			"/api/v1/auth/login-logs",
		} {
			w = do(http.MethodGet, path, adminToken, nil)
			mustStatus(t, w, http.StatusOK, path)
		}
	})

	// ==================================================================
	// 8. RBAC 权限隔离：创建普通用户并验证 403 / 200
	// ==================================================================
	var viewerToken string
	t.Run("权限隔离", func(t *testing.T) {
		// 找到“普通用户”角色 ID
		w := do(http.MethodGet, "/api/v1/auth/roles/options", adminToken, nil)
		mustStatus(t, w, http.StatusOK)
		var opts struct {
			Data []struct {
				ID   uint   `json:"id"`
				Code string `json:"code"`
			} `json:"data"`
		}
		failIf(t, json.Unmarshal(w.Body.Bytes(), &opts), "解析角色选项失败")
		var viewerRoleID uint
		for _, r := range opts.Data {
			if r.Code == models.RoleUser {
				viewerRoleID = r.ID
			}
		}
		if viewerRoleID == 0 {
			t.Fatalf("未找到 user 角色，seed 异常: %s", w.Body.String())
		}

		// 管理员创建普通用户（users 权限 + 强密码校验）
		w = do(http.MethodPost, "/api/v1/auth/users", adminToken, map[string]any{
			"username": "viewer",
			"password": "Viewer#123",
			"email":    "viewer@example.com",
			"role_id":  viewerRoleID,
		})
		mustStatus(t, w, http.StatusOK)

		// 普通用户登录
		w = do(http.MethodPost, "/api/v1/auth/login", "", map[string]any{
			"username": "viewer", "password": "Viewer#123",
		})
		mustStatus(t, w, http.StatusOK)
		var loginBody struct {
			Token string `json:"token"`
		}
		failIf(t, json.Unmarshal(w.Body.Bytes(), &loginBody), "解析 viewer 登录响应失败")
		if loginBody.Token == "" {
			t.Fatalf("viewer 登录未返回 token: %s", w.Body.String())
		}
		viewerToken = loginBody.Token

		// 普通用户仅有 sync 权限：config / users -> 403
		w = do(http.MethodGet, "/api/v1/config/status", viewerToken, nil)
		mustStatus(t, w, http.StatusForbidden)
		body := mustJSON(t, w)
		if got := mustStr(t, body, "error"); got != "没有访问该功能的权限" {
			t.Errorf("403 文案 = %q", got)
		}

		w = do(http.MethodGet, "/api/v1/auth/users", viewerToken, nil)
		mustStatus(t, w, http.StatusForbidden)

		// sync -> 200；images（PermissionRequiredAny 含 sync）-> 200
		w = do(http.MethodGet, "/api/v1/sync/history", viewerToken, nil)
		mustStatus(t, w, http.StatusOK)

		w = do(http.MethodGet, "/api/v1/images/list", viewerToken, nil)
		mustStatus(t, w, http.StatusOK)
	})

	// ==================================================================
	// 9. 请求体大小限制：>5MB 登录请求 -> 400
	// ==================================================================
	t.Run("请求体大小限制", func(t *testing.T) {
		huge := `{"username":"admin","password":"` + strings.Repeat("x", 6<<20) + `"}`
		req := httptest.NewRequest(http.MethodPost, "/api/v1/auth/login", bytes.NewReader([]byte(huge)))
		req.Header.Set("Content-Type", "application/json")
		rec := httptest.NewRecorder()
		router.ServeHTTP(rec, req)
		mustStatus(t, rec, http.StatusBadRequest)
	})

	// ==================================================================
	// 10. 登出吊销 token（放最后：adminToken 之后不可再用）
	// ==================================================================
	t.Run("登出与token吊销", func(t *testing.T) {
		w := do(http.MethodPost, "/api/v1/auth/logout", adminToken, nil)
		mustStatus(t, w, http.StatusOK)

		w = do(http.MethodGet, "/api/v1/auth/me", adminToken, nil)
		mustStatus(t, w, http.StatusUnauthorized)
		body := mustJSON(t, w)
		if got := mustStr(t, body, "error"); got != "认证已失效，请重新登录" {
			t.Errorf("吊销后错误文案 = %q, 期望 认证已失效，请重新登录", got)
		}

		// 其他 token（viewer）不受影响
		if viewerToken != "" {
			w = do(http.MethodGet, "/api/v1/sync/history", viewerToken, nil)
			mustStatus(t, w, http.StatusOK)
		}
	})
}
