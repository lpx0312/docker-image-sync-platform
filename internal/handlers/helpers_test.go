package handlers

// helpers_test.go 跨测试文件共享的私有测试工具，全部以 h 前缀命名，
// 避免与其他测试文件的私有 helper 冲突。
//
// 说明：共享脚手架 testsupport_test.go 的 TestMain 未初始化
// logger.Logger / config.AppConfig，而 handler 大量使用这两个全局变量
// （zap nil 指针会 panic），因此在包 init 中兜底初始化。

import (
	"bytes"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"docker-image-sync-platform/internal/config"
	"docker-image-sync-platform/internal/database"
	"docker-image-sync-platform/internal/logger"
	"docker-image-sync-platform/internal/models"
	"docker-image-sync-platform/internal/services"

	"github.com/gin-gonic/gin"
	"github.com/sirupsen/logrus"
	"go.uber.org/zap"
	"gorm.io/gorm"
)

func init() {
	if logger.Logger == nil {
		logger.Logger = zap.NewNop()
	}
	if config.AppConfig == nil {
		config.AppConfig = &config.Config{}
	}
	// AuthService.GenerateToken/ValidateToken 在 secret 为空时使用默认值，这里显式固定
	config.AppConfig.Auth.JWTSecret = "handlers-test-jwt-secret"
	config.AppConfig.Auth.TokenExpiry = "1h"
	config.AppConfig.Auth.RememberMeExpiry = "168h"
	// normalizeBatchSyncRequest 的默认并发来源
	config.AppConfig.Sync.MaxConcurrentJobs = 3
}

// hWithAppConfig 临时替换全局 AppConfig 并自动恢复
func hWithAppConfig(t *testing.T, mutate func(*config.Config)) {
	t.Helper()
	saved := config.AppConfig
	cfg := *saved
	mutate(&cfg)
	config.AppConfig = &cfg
	t.Cleanup(func() { config.AppConfig = saved })
}

// hNewEngine 创建测试 gin 引擎
func hNewEngine() *gin.Engine {
	return gin.New()
}

// hPerform 执行 HTTP 请求并返回响应记录器
func hPerform(t *testing.T, r http.Handler, method, path, body string, headers map[string]string) *httptest.ResponseRecorder {
	t.Helper()
	var req *http.Request
	if body == "" {
		req = httptest.NewRequest(method, path, nil)
	} else {
		req = httptest.NewRequest(method, path, bytes.NewBufferString(body))
	}
	req.Header.Set("Content-Type", "application/json")
	for k, v := range headers {
		req.Header.Set(k, v)
	}
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)
	return w
}

// hPerformJSON 执行请求并把响应解析为 JSON map
func hPerformJSON(t *testing.T, r http.Handler, method, path, body string, headers map[string]string) (*httptest.ResponseRecorder, map[string]interface{}) {
	t.Helper()
	w := hPerform(t, r, method, path, body, headers)
	var m map[string]interface{}
	if w.Body.Len() > 0 {
		if err := json.Unmarshal(w.Body.Bytes(), &m); err != nil {
			t.Fatalf("响应不是合法 JSON: %v\n%s", err, w.Body.String())
		}
	}
	return w, m
}

// hPathOf 从路径参数构造路由（把 :id 换成实际值）
func hPath(t *testing.T, r *gin.Engine, pattern, actual string) string {
	t.Helper()
	return strings.Replace(pattern, ":id", actual, 1)
}

// hSeedFullRBAC 写入权限、三个系统角色及其关联，返回角色 ID。
// seedRBAC 只建角色不建权限/关联，AuthRequired/PermissionRequired 相关
// 的 e2e 场景需要完整的 RBAC 数据。
func hSeedFullRBAC(t *testing.T, db *gorm.DB) (adminID, operatorID, userID uint) {
	t.Helper()
	if err := db.Create(&models.DefaultPermissionSeeds).Error; err != nil {
		t.Fatalf("创建权限失败: %v", err)
	}
	var perms []models.Permission
	if err := db.Find(&perms).Error; err != nil {
		t.Fatalf("查询权限失败: %v", err)
	}
	permByCode := map[string]uint{}
	for _, p := range perms {
		permByCode[p.Code] = p.ID
	}
	for _, seed := range models.DefaultRoleSeeds {
		role := &models.Role{Code: seed.Code, Name: seed.Name, Description: seed.Description, IsSystem: seed.IsSystem}
		var rolePerms []models.Permission
		for _, code := range seed.Permissions {
			rolePerms = append(rolePerms, models.Permission{ID: permByCode[code]})
		}
		role.Permissions = rolePerms
		if err := db.Create(role).Error; err != nil {
			t.Fatalf("创建角色 %s 失败: %v", seed.Code, err)
		}
		switch seed.Code {
		case models.RoleAdmin:
			adminID = role.ID
		case models.RoleOperator:
			operatorID = role.ID
		case models.RoleUser:
			userID = role.ID
		}
	}
	return
}

// hHash 用真实 AuthService 生成 bcrypt 哈希
func hHash(t *testing.T, password string) string {
	t.Helper()
	auth := services.NewAuthService()
	hash, err := auth.HashPassword(password)
	if err != nil {
		t.Fatalf("哈希密码失败: %v", err)
	}
	return hash
}

// hNewUser 直插用户
func hNewUser(t *testing.T, db *gorm.DB, username, password string, roleID uint) *models.User {
	t.Helper()
	u := &models.User{
		Username:     username,
		PasswordHash: hHash(t, password),
		Email:        username + "@example.com",
		RoleID:       roleID,
		Status:       models.UserStatusActive,
	}
	if err := db.Create(u).Error; err != nil {
		t.Fatalf("创建用户失败: %v", err)
	}
	return u
}

// hNewAuthHandler 构造真实依赖的 AuthHandler 并挂好全局 DB
func hNewAuthHandler(t *testing.T, db *gorm.DB) *AuthHandler {
	t.Helper()
	withGlobalDB(t, db)
	authService := services.NewAuthService()
	roleService := services.NewRoleService(db)
	userService := services.NewUserService(db, authService, roleService)
	return NewAuthHandler(authService, userService)
}

// hWithIdentity 注入登录身份的测试中间件（等价于通过 AuthRequired 后的上下文）
func hWithIdentity(userID uint, username string) gin.HandlerFunc {
	return func(c *gin.Context) {
		c.Set("userID", userID)
		c.Set("username", username)
		c.Next()
	}
}

// hDropTable 删除表（用于构造数据库错误分支），测试结束自动恢复表结构。
// 恢复时直接回放删除前从 sqlite_master 抓取的建表 SQL——
// 生产模型带 MySQL enum 列标签，用 AutoMigrate 恢复在 sqlite 上必然失败。
func hDropTable(t *testing.T, db *gorm.DB, model interface{}, tableName string) {
	t.Helper()
	var restoreSQL string
	if err := db.Raw("SELECT sql FROM sqlite_master WHERE type='table' AND name = ?", tableName).Scan(&restoreSQL).Error; err != nil || restoreSQL == "" {
		t.Fatalf("抓取表 %s 的建表 SQL 失败: %v", tableName, err)
	}
	if err := db.Migrator().DropTable(tableName); err != nil {
		t.Fatalf("drop 表 %s 失败: %v", tableName, err)
	}
	t.Cleanup(func() {
		if err := db.Exec(restoreSQL).Error; err != nil {
			t.Fatalf("恢复表 %s 失败: %v", tableName, err)
		}
	})
}

// hFailUpdatesOn 创建 BEFORE UPDATE 触发器让指定表的 UPDATE 全部失败，
// 用于覆盖数据库更新失败的分支
func hFailUpdatesOn(t *testing.T, db *gorm.DB, tableName string) {
	t.Helper()
	trig := fmt.Sprintf("trig_fail_upd_%s", tableName)
	if err := db.Exec(fmt.Sprintf(
		"CREATE TRIGGER %s BEFORE UPDATE ON %s BEGIN SELECT RAISE(ABORT, 'forced update failure'); END",
		trig, tableName)).Error; err != nil {
		t.Fatalf("创建触发器失败: %v", err)
	}
	t.Cleanup(func() {
		_ = db.Exec(fmt.Sprintf("DROP TRIGGER IF EXISTS %s", trig)).Error
	})
}

// hFailDeletesOn 创建 BEFORE DELETE 触发器让指定表的 DELETE 全部失败
func hFailDeletesOn(t *testing.T, db *gorm.DB, tableName string) {
	t.Helper()
	trig := fmt.Sprintf("trig_fail_del_%s", tableName)
	if err := db.Exec(fmt.Sprintf(
		"CREATE TRIGGER %s BEFORE DELETE ON %s BEGIN SELECT RAISE(ABORT, 'forced delete failure'); END",
		trig, tableName)).Error; err != nil {
		t.Fatalf("创建触发器失败: %v", err)
	}
	t.Cleanup(func() {
		_ = db.Exec(fmt.Sprintf("DROP TRIGGER IF EXISTS %s", trig)).Error
	})
}

// hLocalRegistry 本地模拟 Docker Registry v2（go-containerregistry 对
// 127.0.0.1 使用明文 http，可用于 remote.Head / remote.Index）。
// mode:
//   - "ok":      /v2/ ping 200，manifest 返回 OCI index（amd64+arm64）
//   - "missing": manifest 返回 404（镜像不存在）
//   - "error":   manifest 返回 500（检测异常）
func hLocalRegistry(t *testing.T, mode string) *httptest.Server {
	t.Helper()
	const indexManifest = `{
  "schemaVersion": 2,
  "mediaType": "application/vnd.oci.image.index.v1+json",
  "manifests": [
    {"mediaType":"application/vnd.oci.image.manifest.v1+json","digest":"sha256:1111111111111111111111111111111111111111111111111111111111111111","size":714,"platform":{"architecture":"amd64","os":"linux"}},
    {"mediaType":"application/vnd.oci.image.manifest.v1+json","digest":"sha256:2222222222222222222222222222222222222222222222222222222222222222","size":714,"platform":{"architecture":"arm64","os":"linux"}}
  ]
}`
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case r.URL.Path == "/v2/" || r.URL.Path == "/v2":
			w.Header().Set("Docker-Distribution-API-Version", "registry/2.0")
			w.WriteHeader(http.StatusOK)
			_, _ = w.Write([]byte("{}"))
		case strings.Contains(r.URL.Path, "/manifests/"):
			switch mode {
			case "ok":
				w.Header().Set("Content-Type", "application/vnd.oci.image.index.v1+json")
				w.Header().Set("Docker-Content-Digest", "sha256:3333333333333333333333333333333333333333333333333333333333333333")
				w.WriteHeader(http.StatusOK)
				_, _ = w.Write([]byte(indexManifest))
			case "missing":
				w.WriteHeader(http.StatusNotFound)
				_, _ = w.Write([]byte(`{"errors":[{"code":"NAME_UNKNOWN","message":"manifest unknown"}]}`))
			default:
				w.WriteHeader(http.StatusInternalServerError)
				_, _ = w.Write([]byte(`{"errors":[{"code":"UNKNOWN","message":"boom"}]}`))
			}
		default:
			w.WriteHeader(http.StatusNotFound)
		}
	}))
	t.Cleanup(ts.Close)
	return ts
}

// hLocalRegistryHost 返回 127.0.0.1:port 形式的地址
func hLocalRegistryHost(ts *httptest.Server) string {
	return strings.TrimPrefix(ts.URL, "http://")
}

// hSeedAcrRegistry 直插一条 ACR 配置（密码默认空，避免 buildRegistryInputs 触发真实 GitHub secret 写入）
func hSeedAcrRegistry(t *testing.T, db *gorm.DB, registryURL, namespace string, isDefault bool) uint {
	t.Helper()
	acr := &models.AcrRegistry{
		RegistryURL:  registryURL,
		Namespace:    namespace,
		Alias:        namespace,
		Username:     "",
		Password:     "",
		RegistryType: models.RegistryTypeACR,
		IsDefault:    isDefault,
	}
	if err := db.Create(acr).Error; err != nil {
		t.Fatalf("创建 ACR 配置失败: %v", err)
	}
	return acr.ID
}

// hSeedSyncRecord 直插镜像同步记录
func hSeedSyncRecord(t *testing.T, db *gorm.DB, rec *models.ImageSyncRecord) uint {
	t.Helper()
	if rec.SyncStatus == "" {
		rec.SyncStatus = models.SyncStatusPending
	}
	if err := db.Create(rec).Error; err != nil {
		t.Fatalf("创建镜像记录失败: %v", err)
	}
	return rec.ID
}

// hNewLogrus 测试用 logrus
func hNewLogrus() *logrus.Logger {
	l := logrus.New()
	l.SetLevel(logrus.PanicLevel)
	return l
}

// hDB 返回当前全局 database.DB
func hDB() *gorm.DB {
	return database.DB
}

// hSetConfig 直插系统配置
func hSetConfig(t *testing.T, db *gorm.DB, key, value string) {
	t.Helper()
	if err := db.Where(models.SystemConfig{ConfigKey: key}).FirstOrCreate(&models.SystemConfig{ConfigKey: key, ConfigValue: value}).Error; err != nil {
		t.Fatalf("写入配置 %s 失败: %v", key, err)
	}
}
