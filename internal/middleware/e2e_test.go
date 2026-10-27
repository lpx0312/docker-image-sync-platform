package middleware

// e2e 风格测试：真实 gin 引擎 + httptest，请求走完整中间件链
// （CORS -> ErrorHandler -> BodySizeLimit -> RateLimitByKey -> AuthRequired -> PermissionRequired -> handler）

import (
	"net/http"
	"strings"
	"testing"
	"time"

	"docker-image-sync-platform/internal/config"
	"docker-image-sync-platform/internal/models"
	"docker-image-sync-platform/internal/services"

	"github.com/gin-gonic/gin"
	"go.uber.org/zap"
	"golang.org/x/time/rate"
)

// fullChainDeps 汇聚完整链路依赖，便于测试复用
type fullChainDeps struct {
	engine    *gin.Engine
	auth      *services.AuthService
	roleSvc   *services.RoleService
	adminRole uint
	userRole  uint
}

// buildFullChain 搭建完整中间件链的引擎
func buildFullChain(t *testing.T) *fullChainDeps {
	t.Helper()
	// JWT / CORS 配置（全局保存/恢复由 withAppConfig 处理）
	withAppConfig(t, func(c *config.Config) {
		c.Auth.JWTSecret = "e2e-secret"
		c.Auth.TokenExpiry = "1h"
		c.Security.CORS.AllowedOrigins = []string{"https://console.example.com"}
	})
	resetTokenBlacklist(t)

	authSvc := services.NewAuthService()
	roleSvc, _, adminRole, userRole := newRoleServiceWithSeeds(t)

	engine := newTestEngine()
	engine.Use(CORS())
	engine.Use(ErrorHandler(zap.NewNop()))
	engine.Use(BodySizeLimit(1024))

	api := engine.Group("/api/v1")
	api.Use(AuthRequired(authSvc))
	api.GET("/me", func(c *gin.Context) {
		username, _ := c.Get("username")
		c.JSON(http.StatusOK, gin.H{"username": username})
	})
	api.POST("/echo", func(c *gin.Context) {
		var payload struct {
			Data string `json:"data"`
		}
		if err := c.ShouldBindJSON(&payload); err != nil {
			c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
			return
		}
		c.JSON(http.StatusOK, gin.H{"len": len(payload.Data)})
	})
	api.GET("/panic", func(c *gin.Context) { panic("e2e-boom") })

	admin := api.Group("/admin")
	admin.Use(PermissionRequired(roleSvc, models.PermConfig))
	admin.GET("/config", func(c *gin.Context) { c.JSON(http.StatusOK, gin.H{"ok": true}) })

	// 独立限流路由（全局固定键，突发容量 1）
	syncGroup := engine.Group("/api/v1/sync")
	syncGroup.Use(RateLimitByKey(rate.Every(time.Hour), 1, func(c *gin.Context) string { return "sync" }))
	syncGroup.POST("/start", func(c *gin.Context) { c.JSON(http.StatusOK, gin.H{"ok": true}) })

	return &fullChainDeps{
		engine: engine, auth: authSvc, roleSvc: roleSvc,
		adminRole: adminRole, userRole: userRole,
	}
}

// token 生成便捷方法
func (d *fullChainDeps) token(t *testing.T, username string, roleID uint, roleCode string) string {
	t.Helper()
	token, _, err := d.auth.GenerateToken(1, username, roleID, roleCode, false)
	ok(t, err, "生成 token 失败")
	return token
}

func TestFullChain_CORSOptionsPreflight(t *testing.T) {
	d := buildFullChain(t)

	// 预检：CORS 最先执行并 abort 204，无需 JWT（OPTIONS 不进入 auth）
	rec := performRequest(d.engine, http.MethodOptions, "/api/v1/me", nil, map[string]string{
		"Origin":                        "https://console.example.com",
		"Access-Control-Request-Method": "GET",
	})
	ensureStatus(t, rec, http.StatusNoContent)
	eq(t, "https://console.example.com", rec.Header().Get("Access-Control-Allow-Origin"))
	eq(t, "GET, POST, PUT, DELETE, OPTIONS", rec.Header().Get("Access-Control-Allow-Methods"))

	// 未授权 Origin 的预检：204 但不回显 Allow-Origin
	rec = performRequest(d.engine, http.MethodOptions, "/api/v1/me", nil,
		map[string]string{"Origin": "https://evil.example.net"})
	ensureStatus(t, rec, http.StatusNoContent)
	eq(t, "", rec.Header().Get("Access-Control-Allow-Origin"))
}

func TestFullChain_AuthScenarios(t *testing.T) {
	d := buildFullChain(t)

	// 1) 缺失 token -> 401
	rec := performRequest(d.engine, http.MethodGet, "/api/v1/me", nil, nil)
	ensureStatus(t, rec, http.StatusUnauthorized)
	eq(t, "未提供认证信息", decodeJSON(t, rec)["error"])

	// 2) 非法 token -> 401
	rec = performRequest(d.engine, http.MethodGet, "/api/v1/me", nil,
		map[string]string{"Authorization": "Bearer garbage"})
	ensureStatus(t, rec, http.StatusUnauthorized)
	eq(t, "认证已过期，请重新登录", decodeJSON(t, rec)["error"])

	// 3) 篡改 token -> 401
	valid := d.token(t, "e2e-user", d.adminRole, "admin")
	rec = performRequest(d.engine, http.MethodGet, "/api/v1/me", nil,
		map[string]string{"Authorization": "Bearer " + valid + "x"})
	ensureStatus(t, rec, http.StatusUnauthorized)

	// 4) 有效 token -> 200 回显用户名
	rec = performRequest(d.engine, http.MethodGet, "/api/v1/me", nil,
		map[string]string{"Authorization": "Bearer " + valid})
	ensureStatus(t, rec, http.StatusOK)
	eq(t, "e2e-user", decodeJSON(t, rec)["username"])

	// 5) 黑名单（登出吊销）token -> 401
	token, expiresAt, err := d.auth.GenerateToken(2, "bob", d.adminRole, "admin", false)
	ok(t, err, "生成 token 失败")
	rec = performRequest(d.engine, http.MethodGet, "/api/v1/me", nil,
		map[string]string{"Authorization": "Bearer " + token})
	ensureStatus(t, rec, http.StatusOK)
	RevokeToken(token, expiresAt)
	rec = performRequest(d.engine, http.MethodGet, "/api/v1/me", nil,
		map[string]string{"Authorization": "Bearer " + token})
	ensureStatus(t, rec, http.StatusUnauthorized)
	eq(t, "认证已失效，请重新登录", decodeJSON(t, rec)["error"])
}

func TestFullChain_BodyLimitScenarios(t *testing.T) {
	d := buildFullChain(t)
	authHeader := map[string]string{
		"Authorization": "Bearer " + d.token(t, "e2e-user", d.adminRole, "admin"),
		"Content-Type":  "application/json",
	}

	// 正常大小 body
	rec := performRequest(d.engine, http.MethodPost, "/api/v1/echo",
		strings.NewReader(`{"data":"ok"}`), authHeader)
	ensureStatus(t, rec, http.StatusOK)
	eq(t, float64(2), decodeJSON(t, rec)["len"])

	// 中文与 emoji 多字节 body
	rec = performRequest(d.engine, http.MethodPost, "/api/v1/echo",
		strings.NewReader(`{"data":"中文🐳"}`), authHeader)
	ensureStatus(t, rec, http.StatusOK)
	eq(t, float64(len("中文🐳")), decodeJSON(t, rec)["len"])

	// 超限 body（> 1024 字节）-> 400
	huge := `{"data":"` + strings.Repeat("z", 4096) + `"}`
	rec = performRequest(d.engine, http.MethodPost, "/api/v1/echo",
		strings.NewReader(huge), authHeader)
	ensureStatus(t, rec, http.StatusBadRequest)
	isTrue(t, strings.Contains(decodeJSON(t, rec)["error"].(string), "body too large"),
		"超限应返回 body too large")
}

func TestFullChain_PanicRecovery(t *testing.T) {
	d := buildFullChain(t)
	token := d.token(t, "e2e-user", d.adminRole, "admin")

	rec := performRequest(d.engine, http.MethodGet, "/api/v1/panic", nil,
		map[string]string{"Authorization": "Bearer " + token})
	ensureStatus(t, rec, http.StatusInternalServerError)
	body := decodeJSON(t, rec)
	eq(t, float64(500), body["code"])
	eq(t, "Internal server error", body["message"])

	// 引擎在 panic 后仍可服务后续请求
	rec = performRequest(d.engine, http.MethodGet, "/api/v1/me", nil,
		map[string]string{"Authorization": "Bearer " + token})
	ensureStatus(t, rec, http.StatusOK)
}

func TestFullChain_PermissionGate(t *testing.T) {
	d := buildFullChain(t)

	// user 角色（无 config 权限）-> 403
	userToken := d.token(t, "plain-user", d.userRole, "user")
	rec := performRequest(d.engine, http.MethodGet, "/api/v1/admin/config", nil,
		map[string]string{"Authorization": "Bearer " + userToken})
	ensureStatus(t, rec, http.StatusForbidden)
	eq(t, "没有访问该功能的权限", decodeJSON(t, rec)["error"])

	// admin 角色 -> 200
	adminToken := d.token(t, "boss", d.adminRole, "admin")
	rec = performRequest(d.engine, http.MethodGet, "/api/v1/admin/config", nil,
		map[string]string{"Authorization": "Bearer " + adminToken})
	ensureStatus(t, rec, http.StatusOK)
	eq(t, true, decodeJSON(t, rec)["ok"])
}

func TestFullChain_RateLimitTrigger(t *testing.T) {
	d := buildFullChain(t)

	// /api/v1/sync/start 全局固定键限流，突发容量 1：首个通过，后续 429
	rec := performRequest(d.engine, http.MethodPost, "/api/v1/sync/start", nil, nil)
	ensureStatus(t, rec, http.StatusOK)

	rec = performRequest(d.engine, http.MethodPost, "/api/v1/sync/start", nil, nil)
	ensureStatus(t, rec, http.StatusTooManyRequests)
	body := decodeJSON(t, rec)
	eq(t, float64(http.StatusTooManyRequests), body["code"])
	eq(t, "Too many requests", body["message"])

	// 其他路由不受该限流影响
	rec = performRequest(d.engine, http.MethodGet, "/api/v1/me", nil,
		map[string]string{"Authorization": "Bearer " + d.token(t, "x", d.adminRole, "admin")})
	ensureStatus(t, rec, http.StatusOK)
}

func TestFullChain_CORSHeadersOnNormalRequest(t *testing.T) {
	d := buildFullChain(t)
	token := d.token(t, "e2e-user", d.adminRole, "admin")

	// 白名单内 Origin 的正常业务请求：CORS 头 + 200
	rec := performRequest(d.engine, http.MethodGet, "/api/v1/me", nil, map[string]string{
		"Authorization": "Bearer " + token,
		"Origin":        "https://console.example.com",
	})
	ensureStatus(t, rec, http.StatusOK)
	eq(t, "https://console.example.com", rec.Header().Get("Access-Control-Allow-Origin"))
	eq(t, "e2e-user", decodeJSON(t, rec)["username"])

	// 白名单外 Origin：业务仍 200，但无 Allow-Origin 头
	rec = performRequest(d.engine, http.MethodGet, "/api/v1/me", nil, map[string]string{
		"Authorization": "Bearer " + token,
		"Origin":        "https://evil.example.net",
	})
	ensureStatus(t, rec, http.StatusOK)
	eq(t, "", rec.Header().Get("Access-Control-Allow-Origin"))
}
