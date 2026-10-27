package middleware

import (
	"net/http"
	"strings"
	"testing"

	"github.com/gin-gonic/gin"
	"go.uber.org/zap"
)

// ---------- RequestLogger ----------

func TestRequestLogger_RecordsRequest(t *testing.T) {
	logger, logs := newObservedLogger(t)
	engine := newTestEngine()
	engine.Use(RequestLogger(logger))
	engine.GET("/hello", func(c *gin.Context) { c.JSON(http.StatusOK, gin.H{"ok": true}) })

	rec := performRequest(engine, http.MethodGet, "/hello?a=1", nil,
		map[string]string{"User-Agent": "unit-agent"})
	ensureStatus(t, rec, http.StatusOK)

	entries := logs.FilterMessage("HTTP Request")
	eq(t, 1, entries.Len())
	ctx := entries.All()[0].ContextMap()
	eq(t, "GET", ctx["method"])
	// gin 的 LogFormatterParams.Path 会带上 RawQuery
	eq(t, "/hello?a=1", ctx["path"])
	eq(t, int64(200), ctx["status"])
	eq(t, "unit-agent", ctx["user_agent"])
	if _, exists := ctx["latency"]; !exists {
		t.Error("日志应包含 latency 字段")
	}
	if _, exists := ctx["client_ip"]; !exists {
		t.Error("日志应包含 client_ip 字段")
	}
	if _, exists := ctx["body_size"]; !exists {
		t.Error("日志应包含 body_size 字段")
	}
}

func TestRequestLogger_MultipleRequests(t *testing.T) {
	logger, logs := newObservedLogger(t)
	engine := newTestEngine()
	engine.Use(RequestLogger(logger))
	engine.GET("/x", func(c *gin.Context) { c.Status(http.StatusTeapot) })

	for i := 0; i < 3; i++ {
		rec := performRequest(engine, http.MethodGet, "/x", nil, nil)
		ensureStatus(t, rec, http.StatusTeapot)
	}
	eq(t, 3, logs.FilterMessage("HTTP Request").Len())
}

// ---------- DetailedRequestLogger ----------

func newDetailedEngine(logger *zap.Logger, status int) *gin.Engine {
	engine := newTestEngine()
	engine.Use(DetailedRequestLogger(logger))
	engine.Any("/any", func(c *gin.Context) {
		// 读取请求体验证中间件已将 body 复位（nil body 返回空）
		body := ""
		if c.Request.Body != nil {
			buf := make([]byte, 4096)
			n, _ := c.Request.Body.Read(buf)
			if n > 0 {
				body = string(buf[:n])
			}
		}
		c.JSON(status, gin.H{"body": body})
	})
	return engine
}

func withGinMode(t *testing.T, mode string) {
	t.Helper()
	saved := gin.Mode()
	gin.SetMode(mode)
	t.Cleanup(func() { gin.SetMode(saved) })
}

func TestDetailedRequestLogger_InfoLevelFor2xx(t *testing.T) {
	withGinMode(t, gin.TestMode)
	logger, logs := newObservedLogger(t)
	engine := newDetailedEngine(logger, http.StatusOK)

	rec := performRequest(engine, http.MethodGet, "/any", nil, nil)
	ensureStatus(t, rec, http.StatusOK)

	entries := logs.FilterMessage("HTTP Request")
	eq(t, 1, entries.Len())
	entry := entries.All()[0]
	isTrue(t, entry.Level.String() == "info", "2xx 应为 Info 级别, got %v", entry.Level)
	ctx := entry.ContextMap()
	eq(t, int64(200), ctx["status"])
	if _, exists := ctx["query"]; !exists {
		t.Error("日志应包含 query 字段")
	}
	if _, exists := ctx["response_size"]; !exists {
		t.Error("日志应包含 response_size 字段")
	}
}

func TestDetailedRequestLogger_WarnLevelFor4xx(t *testing.T) {
	withGinMode(t, gin.TestMode)
	logger, logs := newObservedLogger(t)
	engine := newDetailedEngine(logger, http.StatusNotFound)

	rec := performRequest(engine, http.MethodGet, "/any", nil, nil)
	ensureStatus(t, rec, http.StatusNotFound)

	entry := logs.FilterMessage("HTTP Request").All()[0]
	isTrue(t, entry.Level.String() == "warn", "4xx 应为 Warn 级别, got %v", entry.Level)
}

func TestDetailedRequestLogger_ErrorLevelFor5xx(t *testing.T) {
	withGinMode(t, gin.TestMode)
	logger, logs := newObservedLogger(t)
	engine := newDetailedEngine(logger, http.StatusInternalServerError)

	rec := performRequest(engine, http.MethodGet, "/any", nil, nil)
	ensureStatus(t, rec, http.StatusInternalServerError)

	entry := logs.FilterMessage("HTTP Request").All()[0]
	isTrue(t, entry.Level.String() == "error", "5xx 应为 Error 级别, got %v", entry.Level)
}

func TestDetailedRequestLogger_RequestBodyInDebugMode(t *testing.T) {
	withGinMode(t, gin.DebugMode)
	logger, logs := newObservedLogger(t)
	engine := newDetailedEngine(logger, http.StatusOK)

	rec := performRequest(engine, http.MethodPost, "/any",
		strings.NewReader(`{"data":"中文🐳payload"}`), nil)
	ensureStatus(t, rec, http.StatusOK)

	// 中间件读取 body 后复位，业务 handler 仍可读取
	eq(t, `{"data":"中文🐳payload"}`, decodeJSON(t, rec)["body"])

	entry := logs.FilterMessage("HTTP Request").All()[0]
	ctx := entry.ContextMap()
	eq(t, `{"data":"中文🐳payload"}`, ctx["request_body"])
}

func TestDetailedRequestLogger_LargeBodyNotLogged(t *testing.T) {
	withGinMode(t, gin.DebugMode)
	logger, logs := newObservedLogger(t)
	engine := newDetailedEngine(logger, http.StatusOK)

	big := strings.Repeat("x", 2048)
	rec := performRequest(engine, http.MethodPost, "/any", strings.NewReader(big), nil)
	ensureStatus(t, rec, http.StatusOK)

	entry := logs.FilterMessage("HTTP Request").All()[0]
	ctx := entry.ContextMap()
	if _, exists := ctx["request_body"]; exists {
		t.Error("超过 1KB 的请求体不应记录")
	}
}

func TestDetailedRequestLogger_NonDebugModeSkipsBody(t *testing.T) {
	withGinMode(t, gin.ReleaseMode)
	logger, logs := newObservedLogger(t)
	engine := newDetailedEngine(logger, http.StatusOK)

	rec := performRequest(engine, http.MethodPost, "/any",
		strings.NewReader(`{"small":"body"}`), nil)
	ensureStatus(t, rec, http.StatusOK)

	entry := logs.FilterMessage("HTTP Request").All()[0]
	ctx := entry.ContextMap()
	if _, exists := ctx["request_body"]; exists {
		t.Error("非 Debug 模式不应记录请求体")
	}
}

func TestDetailedRequestLogger_NilBody(t *testing.T) {
	withGinMode(t, gin.TestMode)
	logger, logs := newObservedLogger(t)
	engine := newDetailedEngine(logger, http.StatusOK)

	req := newRequestNilBody(http.MethodGet, "/any")
	rec := httptestRecorder()
	engine.ServeHTTP(rec, req)
	ensureStatus(t, rec, http.StatusOK)
	eq(t, "", decodeJSON(t, rec)["body"])
	eq(t, 1, logs.FilterMessage("HTTP Request").Len())
}

// ---------- SkipPaths ----------

func TestSkipPaths(t *testing.T) {
	engine := newTestEngine()
	engine.Use(SkipPaths("/health", "/metrics"))
	reached := ""
	engine.GET("/health", func(c *gin.Context) { reached = "health"; c.Status(http.StatusOK) })
	engine.GET("/metrics", func(c *gin.Context) { reached = "metrics"; c.Status(http.StatusOK) })
	engine.GET("/api/data", func(c *gin.Context) { reached = "data"; c.Status(http.StatusOK) })

	// 跳过列表内路径：请求继续（当前实现两个分支行为一致，均放行）
	rec := performRequest(engine, http.MethodGet, "/health", nil, nil)
	ensureStatus(t, rec, http.StatusOK)
	eq(t, "health", reached)

	// 跳过列表外路径：请求同样继续
	rec = performRequest(engine, http.MethodGet, "/api/data", nil, nil)
	ensureStatus(t, rec, http.StatusOK)
	eq(t, "data", reached)

	// 空参数
	engine2 := newTestEngine()
	engine2.Use(SkipPaths())
	engine2.GET("/x", func(c *gin.Context) { reached = "x"; c.Status(http.StatusOK) })
	rec = performRequest(engine2, http.MethodGet, "/x", nil, nil)
	ensureStatus(t, rec, http.StatusOK)
	eq(t, "x", reached)
}
