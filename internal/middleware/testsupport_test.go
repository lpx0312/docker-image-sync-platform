package middleware

// 共享测试脚手架：本包所有 _test.go 复用，禁止在其他测试文件中重复定义
// TestMain / 断言助手 / 环境工具，避免符号冲突。

import (
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"reflect"
	"testing"
	"time"

	"docker-image-sync-platform/internal/config"
	"docker-image-sync-platform/internal/models"
	"docker-image-sync-platform/internal/services"

	"github.com/gin-gonic/gin"
	"github.com/glebarez/sqlite"
	"go.uber.org/zap"
	"go.uber.org/zap/zapcore"
	"go.uber.org/zap/zaptest/observer"
	"gorm.io/gorm"
	gormLogger "gorm.io/gorm/logger"
)

// 说明：go.mod 中 testify v1.12.1 依赖的 go.yaml.in/yaml/v3 未声明，
// testify 实际不可编译（internal/utils 既有测试同样失败），按约定不改
// go.mod/go.sum，测试统一使用标准库 + 下列轻量断言助手。

func TestMain(m *testing.M) {
	gin.SetMode(gin.TestMode)
	// 屏蔽 gin Recovery 默认输出，保持测试输出干净
	gin.DefaultErrorWriter = io.Discard
	os.Exit(m.Run())
}

// ---------- 轻量断言助手（仅测试使用） ----------

func eq(t *testing.T, want, got any) {
	t.Helper()
	if !reflect.DeepEqual(want, got) {
		t.Errorf("不相等:\nwant: %#v\n got: %#v", want, got)
	}
}

func isTrue(t *testing.T, cond bool, format string, args ...any) {
	t.Helper()
	if !cond {
		t.Errorf("条件为假: "+format, args...)
	}
}

func isFalse(t *testing.T, cond bool, format string, args ...any) {
	t.Helper()
	if cond {
		t.Errorf("条件为真: "+format, args...)
	}
}

func ok(t *testing.T, err error, format string, args ...any) {
	t.Helper()
	if err != nil {
		t.Fatalf("意外错误: %v; "+format, append([]any{err}, args...)...)
	}
}

func bad(t *testing.T, err error, format string, args ...any) {
	t.Helper()
	if err == nil {
		t.Fatalf("期望错误但得到 nil; "+format, args...)
	}
}

// ---------- 全局状态工具 ----------

// withAppConfig 临时替换全局 config.AppConfig（按当前值拷贝后 mutate），测试结束恢复
func withAppConfig(t *testing.T, mutate func(cfg *config.Config)) *config.Config {
	t.Helper()
	saved := config.AppConfig
	cfg := &config.Config{}
	if saved != nil {
		*cfg = *saved
	}
	if mutate != nil {
		mutate(cfg)
	}
	config.AppConfig = cfg
	t.Cleanup(func() { config.AppConfig = saved })
	return cfg
}

// resetTokenBlacklist 重置内存 token 黑名单，测试结束恢复
func resetTokenBlacklist(t *testing.T) {
	t.Helper()
	tokenBlacklist.Lock()
	saved := tokenBlacklist.entries
	tokenBlacklist.entries = make(map[string]time.Time)
	tokenBlacklist.Unlock()
	t.Cleanup(func() {
		tokenBlacklist.Lock()
		tokenBlacklist.entries = saved
		tokenBlacklist.Unlock()
	})
}

// newObservedLogger 返回可捕获日志条目的 zap logger
func newObservedLogger(t *testing.T) (*zap.Logger, *observer.ObservedLogs) {
	t.Helper()
	core, logs := observer.New(zapcore.DebugLevel)
	return zap.New(core), logs
}

// ---------- HTTP 工具 ----------

// newTestEngine 创建不挂任何默认中间件的 gin 引擎
func newTestEngine() *gin.Engine {
	return gin.New()
}

// performRequest 对引擎发起请求并返回响应记录器
func performRequest(engine *gin.Engine, method, target string, body io.Reader, headers map[string]string) *httptest.ResponseRecorder {
	req := httptest.NewRequest(method, target, body)
	req.RemoteAddr = "192.0.2.10:1234" // 固定 ClientIP，保证限流测试稳定
	for k, v := range headers {
		req.Header.Set(k, v)
	}
	rec := httptest.NewRecorder()
	engine.ServeHTTP(rec, req)
	return rec
}

// newRequestNilBody 构造 Body 为 nil 的请求（覆盖 BodySizeLimit 的 nil 分支）
func newRequestNilBody(method, target string) *http.Request {
	req := httptest.NewRequest(method, target, nil)
	req.Body = nil
	req.RemoteAddr = "192.0.2.10:1234"
	return req
}

// httptestRecorder 返回空响应记录器
func httptestRecorder() *httptest.ResponseRecorder {
	return httptest.NewRecorder()
}

// decodeJSON 将响应体解析为 map
func decodeJSON(t *testing.T, rec *httptest.ResponseRecorder) map[string]any {
	t.Helper()
	m := map[string]any{}
	ok(t, json.Unmarshal(rec.Body.Bytes(), &m), "响应体不是合法 JSON: %s", rec.Body.String())
	return m
}

// ---------- RBAC 测试数据 ----------

// newRoleServiceWithSeeds 建立内存 SQLite 并 seed 权限 + 三种系统角色
func newRoleServiceWithSeeds(t *testing.T) (*services.RoleService, *gorm.DB, uint, uint) {
	t.Helper()
	// 静默 gorm 日志，避免 record not found 等测试内噪音
	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{
		Logger: gormLogger.Default.LogMode(gormLogger.Silent),
	})
	ok(t, err, "打开内存 SQLite 失败")
	ok(t, db.AutoMigrate(&models.Permission{}, &models.Role{}, &models.User{}), "AutoMigrate RBAC 模型失败")

	ok(t, db.Create(&models.DefaultPermissionSeeds).Error, "权限 seed 失败")

	var all []models.Permission
	ok(t, db.Find(&all).Error, "查询权限失败")
	var syncOnly, allButUsers []models.Permission
	for _, p := range all {
		switch p.Code {
		case models.PermSync:
			syncOnly = append(syncOnly, p)
			allButUsers = append(allButUsers, p)
		case models.PermUsers:
			// allButUsers 不含 users/roles
		case models.PermRoles:
		default:
			allButUsers = append(allButUsers, p)
		}
	}

	admin := models.Role{Code: models.RoleAdmin, Name: "管理员", IsSystem: true, Permissions: all}
	operator := models.Role{Code: models.RoleOperator, Name: "运维员", IsSystem: true, Permissions: allButUsers}
	user := models.Role{Code: models.RoleUser, Name: "普通用户", IsSystem: true, Permissions: syncOnly}
	ok(t, db.Create(&admin).Error, "创建 admin 失败")
	ok(t, db.Create(&operator).Error, "创建 operator 失败")
	ok(t, db.Create(&user).Error, "创建 user 失败")

	return services.NewRoleService(db), db, admin.ID, user.ID
}

// ensureStatus 校验状态码
func ensureStatus(t *testing.T, rec *httptest.ResponseRecorder, want int) {
	t.Helper()
	if rec.Code != want {
		t.Fatalf("状态码不符: want %d got %d, body=%s", want, rec.Code, rec.Body.String())
	}
}
