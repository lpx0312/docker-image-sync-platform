package middleware

import (
	"net/http"
	"testing"

	"docker-image-sync-platform/internal/config"

	"github.com/gin-gonic/gin"
)

// newCORSEngine 搭建 CORS 中间件 + 标记 handler 的引擎
func newCORSEngine(mw gin.HandlerFunc) *gin.Engine {
	engine := newTestEngine()
	engine.Use(mw)
	engine.GET("/data", func(c *gin.Context) {
		c.JSON(http.StatusOK, gin.H{"ok": true})
	})
	// OPTIONS 由 CORS 直接 abort，无需注册路由（gin 会先跑中间件）
	return engine
}

// ---------- CORS()：config.AppConfig 驱动 ----------

func TestCORS_DefaultWildcardWhenAppConfigNil(t *testing.T) {
	// AppConfig 为 nil 时默认 "*"
	saved := config.AppConfig
	config.AppConfig = nil
	t.Cleanup(func() { config.AppConfig = saved })

	engine := newCORSEngine(CORS())
	rec := performRequest(engine, http.MethodGet, "/data", nil, nil)
	ensureStatus(t, rec, http.StatusOK)
	eq(t, "*", rec.Header().Get("Access-Control-Allow-Origin"))
	eq(t, "GET, POST, PUT, DELETE, OPTIONS", rec.Header().Get("Access-Control-Allow-Methods"))
	eq(t, "Content-Length, Content-Type", rec.Header().Get("Access-Control-Expose-Headers"))
	isTrue(t, rec.Header().Get("Access-Control-Allow-Headers") != "", "Allow-Headers 应有默认值")
}

func TestCORS_DefaultWildcardWhenListEmpty(t *testing.T) {
	// 配置存在但列表为空 -> 通配
	withAppConfig(t, func(c *config.Config) {
		c.Security.CORS.AllowedOrigins = nil
	})
	engine := newCORSEngine(CORS())
	rec := performRequest(engine, http.MethodGet, "/data", nil, nil)
	ensureStatus(t, rec, http.StatusOK)
	eq(t, "*", rec.Header().Get("Access-Control-Allow-Origin"))
}

func TestCORS_ExactWhitelist(t *testing.T) {
	withAppConfig(t, func(c *config.Config) {
		c.Security.CORS.AllowedOrigins = []string{"https://console.example.com", "https://app.example.com"}
	})
	engine := newCORSEngine(CORS())

	// 白名单内 Origin：回显并写上下文
	rec := performRequest(engine, http.MethodGet, "/data", nil,
		map[string]string{"Origin": "https://app.example.com"})
	ensureStatus(t, rec, http.StatusOK)
	eq(t, "https://app.example.com", rec.Header().Get("Access-Control-Allow-Origin"))

	// 白名单外 Origin：不设置 Allow-Origin（浏览器侧拦截），请求本身仍放行
	rec = performRequest(engine, http.MethodGet, "/data", nil,
		map[string]string{"Origin": "https://evil.example.net"})
	ensureStatus(t, rec, http.StatusOK)
	eq(t, "", rec.Header().Get("Access-Control-Allow-Origin"))

	// 无 Origin 的普通请求：放行且不设置 Allow-Origin
	rec = performRequest(engine, http.MethodGet, "/data", nil, nil)
	ensureStatus(t, rec, http.StatusOK)
	eq(t, "", rec.Header().Get("Access-Control-Allow-Origin"))

	// 大小写敏感：origin 值必须完全匹配
	rec = performRequest(engine, http.MethodGet, "/data", nil,
		map[string]string{"Origin": "https://APP.example.com"})
	ensureStatus(t, rec, http.StatusOK)
	eq(t, "", rec.Header().Get("Access-Control-Allow-Origin"))
}

func TestCORS_WildcardInList(t *testing.T) {
	// 列表包含 "*" 时按通配处理（且 break 后不再收集其余条目）
	withAppConfig(t, func(c *config.Config) {
		c.Security.CORS.AllowedOrigins = []string{"https://a.com", "*", "https://b.com"}
	})
	engine := newCORSEngine(CORS())
	rec := performRequest(engine, http.MethodGet, "/data", nil,
		map[string]string{"Origin": "https://anything.com"})
	ensureStatus(t, rec, http.StatusOK)
	eq(t, "*", rec.Header().Get("Access-Control-Allow-Origin"))
}

func TestCORS_OptionsPreflight(t *testing.T) {
	t.Run("通配模式预检", func(t *testing.T) {
		withAppConfig(t, func(c *config.Config) { c.Security.CORS.AllowedOrigins = nil })
		engine := newCORSEngine(CORS())
		rec := performRequest(engine, http.MethodOptions, "/data", nil,
			map[string]string{
				"Origin": "https://app.example.com",
				"Access-Control-Request-Method": "POST",
			})
		ensureStatus(t, rec, http.StatusNoContent)
		eq(t, "*", rec.Header().Get("Access-Control-Allow-Origin"))
	})

	t.Run("白名单模式预检", func(t *testing.T) {
		withAppConfig(t, func(c *config.Config) {
			c.Security.CORS.AllowedOrigins = []string{"https://app.example.com"}
		})
		engine := newCORSEngine(CORS())
		rec := performRequest(engine, http.MethodOptions, "/data", nil,
			map[string]string{
				"Origin": "https://app.example.com",
				"Access-Control-Request-Method": "PUT",
			})
		ensureStatus(t, rec, http.StatusNoContent)
		eq(t, "https://app.example.com", rec.Header().Get("Access-Control-Allow-Origin"))
	})

	t.Run("白名单外 Origin 预检仍返回 204 但无 Allow-Origin", func(t *testing.T) {
		withAppConfig(t, func(c *config.Config) {
			c.Security.CORS.AllowedOrigins = []string{"https://app.example.com"}
		})
		engine := newCORSEngine(CORS())
		rec := performRequest(engine, http.MethodOptions, "/data", nil,
			map[string]string{"Origin": "https://evil.example.net"})
		ensureStatus(t, rec, http.StatusNoContent)
		eq(t, "", rec.Header().Get("Access-Control-Allow-Origin"))
	})
}

func TestCORS_SetsOriginInContext(t *testing.T) {
	withAppConfig(t, func(c *config.Config) {
		c.Security.CORS.AllowedOrigins = []string{"https://app.example.com"}
	})
	engine := newTestEngine()
	engine.Use(CORS())
	engine.GET("/origin", func(c *gin.Context) {
		v, exists := c.Get("origin")
		if !exists {
			c.JSON(http.StatusOK, gin.H{"origin": nil})
			return
		}
		c.JSON(http.StatusOK, gin.H{"origin": v})
	})

	// 白名单内：上下文有 origin
	rec := performRequest(engine, http.MethodGet, "/origin", nil,
		map[string]string{"Origin": "https://app.example.com"})
	eq(t, "https://app.example.com", decodeJSON(t, rec)["origin"])

	// 白名单外：不写上下文
	rec = performRequest(engine, http.MethodGet, "/origin", nil,
		map[string]string{"Origin": "https://evil.example.net"})
	eq(t, nil, decodeJSON(t, rec)["origin"])
}

// ---------- CORSWithConfig() ----------

func TestCORSWithConfig_EmptyOriginsMeansWildcard(t *testing.T) {
	engine := newCORSEngine(CORSWithConfig(nil, nil, nil))

	// 无 Origin 请求也放行，Allow-Origin 为 "*"
	rec := performRequest(engine, http.MethodGet, "/data", nil, nil)
	ensureStatus(t, rec, http.StatusOK)
	eq(t, "*", rec.Header().Get("Access-Control-Allow-Origin"))
	eq(t, "GET, POST, PUT, DELETE, OPTIONS", rec.Header().Get("Access-Control-Allow-Methods"))
	eq(t, "Origin, X-Requested-With, Content-Type, Accept, Authorization, Cache-Control, X-File-Name",
		rec.Header().Get("Access-Control-Allow-Headers"))
}

func TestCORSWithConfig_ExactMatch(t *testing.T) {
	engine := newCORSEngine(CORSWithConfig(
		[]string{"https://a.com", "https://b.com"}, nil, nil))

	rec := performRequest(engine, http.MethodGet, "/data", nil,
		map[string]string{"Origin": "https://a.com"})
	ensureStatus(t, rec, http.StatusOK)
	eq(t, "https://a.com", rec.Header().Get("Access-Control-Allow-Origin"))

	// 未授权来源：403
	rec = performRequest(engine, http.MethodGet, "/data", nil,
		map[string]string{"Origin": "https://evil.com"})
	ensureStatus(t, rec, http.StatusForbidden)

	// 无 Origin 头：同样 403（严格模式）
	rec = performRequest(engine, http.MethodGet, "/data", nil, nil)
	ensureStatus(t, rec, http.StatusForbidden)
}

func TestCORSWithConfig_StarInListRequiresOriginHeader(t *testing.T) {
	// 当前列表模式实现："*" 分支回显请求 Origin 而非输出 "*"；
	// 且 Origin 缺失时 allowOrigin 为空串 -> 403（按当前实际行为断言）
	engine := newCORSEngine(CORSWithConfig([]string{"*"}, nil, nil))

	rec := performRequest(engine, http.MethodGet, "/data", nil,
		map[string]string{"Origin": "https://any.com"})
	ensureStatus(t, rec, http.StatusOK)
	eq(t, "https://any.com", rec.Header().Get("Access-Control-Allow-Origin"))

	rec = performRequest(engine, http.MethodGet, "/data", nil, nil)
	ensureStatus(t, rec, http.StatusForbidden)
}

func TestCORSWithConfig_CustomMethodsAndHeaders(t *testing.T) {
	engine := newCORSEngine(CORSWithConfig(
		[]string{"https://a.com"},
		[]string{"GET", "PATCH", "DELETE"},
		[]string{"Content-Type", "X-API-Key"}))

	rec := performRequest(engine, http.MethodGet, "/data", nil,
		map[string]string{"Origin": "https://a.com"})
	ensureStatus(t, rec, http.StatusOK)
	eq(t, "GET, PATCH, DELETE", rec.Header().Get("Access-Control-Allow-Methods"))
	eq(t, "Content-Type, X-API-Key", rec.Header().Get("Access-Control-Allow-Headers"))
	eq(t, "Content-Length, Content-Type", rec.Header().Get("Access-Control-Expose-Headers"))
}

func TestCORSWithConfig_OptionsPreflight(t *testing.T) {
	engine := newCORSEngine(CORSWithConfig(
		[]string{"https://a.com"}, []string{"POST"}, []string{"Authorization"}))

	rec := performRequest(engine, http.MethodOptions, "/data", nil,
		map[string]string{
			"Origin": "https://a.com",
			"Access-Control-Request-Method": "POST",
		})
	ensureStatus(t, rec, http.StatusNoContent)
	eq(t, "https://a.com", rec.Header().Get("Access-Control-Allow-Origin"))
	eq(t, "POST", rec.Header().Get("Access-Control-Allow-Methods"))

	// 未授权来源的预检：403
	rec = performRequest(engine, http.MethodOptions, "/data", nil,
		map[string]string{"Origin": "https://evil.com"})
	ensureStatus(t, rec, http.StatusForbidden)
}
