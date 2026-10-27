package middleware

import (
	"net/http"
	"strings"
	"testing"
	"time"

	"docker-image-sync-platform/internal/config"
	"docker-image-sync-platform/internal/models"
	"docker-image-sync-platform/internal/services"

	"github.com/gin-gonic/gin"
)

// setTestJWTConfig 设置测试 JWT 配置（全局保存/恢复由 withAppConfig 处理）
func setTestJWTConfig(t *testing.T, tokenExpiry string) {
	t.Helper()
	withAppConfig(t, func(c *config.Config) {
		c.Auth.JWTSecret = "unit-test-secret"
		c.Auth.TokenExpiry = tokenExpiry
	})
}

// generateTestToken 生成测试 JWT（先设置配置再签发）
func generateTestToken(t *testing.T, userID uint, username string, roleID uint, roleCode, expiry string) (string, time.Time) {
	t.Helper()
	setTestJWTConfig(t, expiry)
	token, expiresAt, err := services.NewAuthService().GenerateToken(userID, username, roleID, roleCode, false)
	ok(t, err, "生成 token 失败")
	return token, expiresAt
}

// newAuthEngine 搭建带 AuthRequired 的引擎，业务 handler 回显上下文信息
func newAuthEngine(echoContext bool) *gin.Engine {
	engine := newTestEngine()
	engine.GET("/protected", AuthRequired(services.NewAuthService()), func(c *gin.Context) {
		if !echoContext {
			c.JSON(http.StatusOK, gin.H{"ok": true})
			return
		}
		userID, _ := c.Get("userID")
		username, _ := c.Get("username")
		roleID, _ := c.Get("roleID")
		roleCode, _ := c.Get("roleCode")
		c.JSON(http.StatusOK, gin.H{
			"userID":   userID,
			"username": username,
			"roleID":   roleID,
			"roleCode": roleCode,
		})
	})
	return engine
}

func TestAuthRequired_MissingAuthorizationHeader(t *testing.T) {
	setTestJWTConfig(t, "1h")
	engine := newAuthEngine(false)

	rec := performRequest(engine, http.MethodGet, "/protected", nil, nil)
	ensureStatus(t, rec, http.StatusUnauthorized)
	eq(t, "未提供认证信息", decodeJSON(t, rec)["error"])

	// 空字符串的 Authorization 头同样视为未提供
	rec = performRequest(engine, http.MethodGet, "/protected", nil,
		map[string]string{"Authorization": ""})
	ensureStatus(t, rec, http.StatusUnauthorized)
	eq(t, "未提供认证信息", decodeJSON(t, rec)["error"])
}

func TestAuthRequired_MalformedAuthorizationHeader(t *testing.T) {
	setTestJWTConfig(t, "1h")
	engine := newAuthEngine(false)

	// 精确断言三种格式错误场景：无 token 部分 / 非 Bearer 方案（大小写敏感）
	for _, header := range []string{"Bearer", "bearer abc.def.ghi", "Basic dXNlcjpwYXNz"} {
		rec := performRequest(engine, http.MethodGet, "/protected", nil,
			map[string]string{"Authorization": header})
		ensureStatus(t, rec, http.StatusUnauthorized)
		eq(t, "认证格式错误", decodeJSON(t, rec)["error"])
	}

	// 方案正确但 token 为空 -> 走 ValidateToken 失败分支
	rec := performRequest(engine, http.MethodGet, "/protected", nil,
		map[string]string{"Authorization": "Bearer "})
	ensureStatus(t, rec, http.StatusUnauthorized)
	eq(t, "认证已过期，请重新登录", decodeJSON(t, rec)["error"])

	// 只有 token 没有方案前缀
	rec = performRequest(engine, http.MethodGet, "/protected", nil,
		map[string]string{"Authorization": "abc.def.ghi"})
	ensureStatus(t, rec, http.StatusUnauthorized)
	eq(t, "认证格式错误", decodeJSON(t, rec)["error"])
}

func TestAuthRequired_InvalidToken(t *testing.T) {
	setTestJWTConfig(t, "1h")
	engine := newAuthEngine(false)

	rec := performRequest(engine, http.MethodGet, "/protected", nil,
		map[string]string{"Authorization": "Bearer not-a-valid-jwt"})
	ensureStatus(t, rec, http.StatusUnauthorized)
	eq(t, "认证已过期，请重新登录", decodeJSON(t, rec)["error"])
}

func reverseString(s string) string {
	b := []byte(s)
	for i, j := 0, len(b)-1; i < j; i, j = i+1, j-1 {
		b[i], b[j] = b[j], b[i]
	}
	return string(b)
}

func TestAuthRequired_TamperedToken(t *testing.T) {
	resetTokenBlacklist(t)
	token, _ := generateTestToken(t, 1, "alice", 2, "admin", "1h")
	setTestJWTConfig(t, "1h")
	engine := newAuthEngine(false)

	// 篡改签名最后一段（翻转字符）
	parts := strings.Split(token, ".")
	tampered := parts[0] + "." + parts[1] + "." + reverseString(parts[2])
	rec := performRequest(engine, http.MethodGet, "/protected", nil,
		map[string]string{"Authorization": "Bearer " + tampered})
	ensureStatus(t, rec, http.StatusUnauthorized)
	eq(t, "认证已过期，请重新登录", decodeJSON(t, rec)["error"])

	// 篡改 payload（改变用户 ID 字符）
	parts = strings.Split(token, ".")
	payload := []byte(parts[1])
	payload[0] ^= 0x01
	tampered2 := parts[0] + "." + string(payload) + "." + parts[2]
	rec = performRequest(engine, http.MethodGet, "/protected", nil,
		map[string]string{"Authorization": "Bearer " + tampered2})
	ensureStatus(t, rec, http.StatusUnauthorized)
}

func TestAuthRequired_ExpiredToken(t *testing.T) {
	resetTokenBlacklist(t)
	// 负数 duration 生成已过期的 token
	token, _ := generateTestToken(t, 1, "alice", 2, "admin", "-1m")
	setTestJWTConfig(t, "1h")
	engine := newAuthEngine(false)

	rec := performRequest(engine, http.MethodGet, "/protected", nil,
		map[string]string{"Authorization": "Bearer " + token})
	ensureStatus(t, rec, http.StatusUnauthorized)
	eq(t, "认证已过期，请重新登录", decodeJSON(t, rec)["error"])
}

func TestAuthRequired_ValidTokenSetsContext(t *testing.T) {
	resetTokenBlacklist(t)
	token, _ := generateTestToken(t, 42, "alice", 7, "admin", "1h")
	setTestJWTConfig(t, "1h")
	engine := newAuthEngine(true)

	rec := performRequest(engine, http.MethodGet, "/protected", nil,
		map[string]string{"Authorization": "Bearer " + token})
	ensureStatus(t, rec, http.StatusOK)
	body := decodeJSON(t, rec)
	eq(t, float64(42), body["userID"])
	eq(t, "alice", body["username"])
	eq(t, float64(7), body["roleID"])
	eq(t, "admin", body["roleCode"])
}

func TestAuthRequired_BlacklistedToken(t *testing.T) {
	resetTokenBlacklist(t)
	token, expiresAt := generateTestToken(t, 1, "alice", 2, "admin", "1h")
	setTestJWTConfig(t, "1h")

	// 登出吊销
	RevokeToken(token, expiresAt)
	isTrue(t, isTokenRevoked(token), "吊销后应被识别")

	engine := newAuthEngine(false)
	rec := performRequest(engine, http.MethodGet, "/protected", nil,
		map[string]string{"Authorization": "Bearer " + token})
	ensureStatus(t, rec, http.StatusUnauthorized)
	eq(t, "认证已失效，请重新登录", decodeJSON(t, rec)["error"])
}

func TestAuthRequired_UnicodeUsername(t *testing.T) {
	resetTokenBlacklist(t)
	// unicode 用户名走完整链路
	token, _ := generateTestToken(t, 3, "用户🐳admin", 1, "user", "1h")
	setTestJWTConfig(t, "1h")
	engine := newAuthEngine(true)

	rec := performRequest(engine, http.MethodGet, "/protected", nil,
		map[string]string{"Authorization": "Bearer " + token})
	ensureStatus(t, rec, http.StatusOK)
	eq(t, "用户🐳admin", decodeJSON(t, rec)["username"])
}

// ---------- PermissionRequired / PermissionRequiredAny ----------

func TestPermissionRequired_NoRoleIDInContext(t *testing.T) {
	roleSvc, _, _, _ := newRoleServiceWithSeeds(t)
	engine := newTestEngine()
	engine.GET("/res", PermissionRequired(roleSvc, models.PermConfig),
		func(c *gin.Context) { c.JSON(http.StatusOK, gin.H{"ok": true}) })

	rec := performRequest(engine, http.MethodGet, "/res", nil, nil)
	ensureStatus(t, rec, http.StatusForbidden)
	eq(t, "无法获取用户角色", decodeJSON(t, rec)["error"])
}

func TestPermissionRequired_InvalidRoleIDType(t *testing.T) {
	roleSvc, _, _, _ := newRoleServiceWithSeeds(t)

	cases := []struct {
		name  string
		value any
	}{
		{"字符串类型", "7"},
		{"int 类型", 7},
		{"nil", nil},
		{"零值", uint(0)},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			engine := newTestEngine()
			engine.GET("/res",
				func(c *gin.Context) { c.Set("roleID", tc.value) },
				PermissionRequired(roleSvc, models.PermConfig),
				func(c *gin.Context) { c.JSON(http.StatusOK, gin.H{"ok": true}) })

			rec := performRequest(engine, http.MethodGet, "/res", nil, nil)
			ensureStatus(t, rec, http.StatusForbidden)
			eq(t, "无法获取用户角色", decodeJSON(t, rec)["error"])
		})
	}
}

func TestPermissionRequired_AllowAndDeny(t *testing.T) {
	roleSvc, _, adminID, userID := newRoleServiceWithSeeds(t)

	build := func(roleID uint) *gin.Engine {
		engine := newTestEngine()
		engine.GET("/res",
			func(c *gin.Context) { c.Set("roleID", roleID) },
			PermissionRequired(roleSvc, models.PermConfig),
			func(c *gin.Context) { c.JSON(http.StatusOK, gin.H{"ok": true}) })
		return engine
	}

	// admin 有 config 权限
	rec := performRequest(build(adminID), http.MethodGet, "/res", nil, nil)
	ensureStatus(t, rec, http.StatusOK)
	eq(t, true, decodeJSON(t, rec)["ok"])

	// user 无 config 权限
	rec = performRequest(build(userID), http.MethodGet, "/res", nil, nil)
	ensureStatus(t, rec, http.StatusForbidden)
	eq(t, "没有访问该功能的权限", decodeJSON(t, rec)["error"])

	// 不存在的角色 ID
	rec = performRequest(build(9999), http.MethodGet, "/res", nil, nil)
	ensureStatus(t, rec, http.StatusForbidden)
	eq(t, "没有访问该功能的权限", decodeJSON(t, rec)["error"])
}

func TestPermissionRequiredAny(t *testing.T) {
	roleSvc, _, adminID, userID := newRoleServiceWithSeeds(t)

	build := func(roleID uint, perms ...string) *gin.Engine {
		engine := newTestEngine()
		handlers := []gin.HandlerFunc{
			func(c *gin.Context) { c.Set("roleID", roleID) },
			PermissionRequiredAny(roleSvc, perms...),
			func(c *gin.Context) { c.JSON(http.StatusOK, gin.H{"ok": true}) },
		}
		engine.GET("/res", handlers...)
		return engine
	}

	// admin 满足第一个权限
	rec := performRequest(build(adminID, models.PermConfig, models.PermUsers), http.MethodGet, "/res", nil, nil)
	ensureStatus(t, rec, http.StatusOK)

	// user 满足第二个权限（sync）
	rec = performRequest(build(userID, models.PermUsers, models.PermSync), http.MethodGet, "/res", nil, nil)
	ensureStatus(t, rec, http.StatusOK)

	// user 无 config/users，均不满足 -> 403
	rec = performRequest(build(userID, models.PermConfig, models.PermUsers), http.MethodGet, "/res", nil, nil)
	ensureStatus(t, rec, http.StatusForbidden)
	eq(t, "没有访问该功能的权限", decodeJSON(t, rec)["error"])

	// roleID 缺失
	engine := newTestEngine()
	engine.GET("/res",
		PermissionRequiredAny(roleSvc, models.PermSync),
		func(c *gin.Context) { c.JSON(http.StatusOK, gin.H{"ok": true}) })
	rec = performRequest(engine, http.MethodGet, "/res", nil, nil)
	ensureStatus(t, rec, http.StatusForbidden)
	eq(t, "无法获取用户角色", decodeJSON(t, rec)["error"])

	// roleID 类型非法
	engine = newTestEngine()
	engine.GET("/res",
		func(c *gin.Context) { c.Set("roleID", "bad")},
		PermissionRequiredAny(roleSvc, models.PermSync),
		func(c *gin.Context) { c.JSON(http.StatusOK, gin.H{"ok": true}) })
	rec = performRequest(engine, http.MethodGet, "/res", nil, nil)
	ensureStatus(t, rec, http.StatusForbidden)
	eq(t, "无法获取用户角色", decodeJSON(t, rec)["error"])
}
