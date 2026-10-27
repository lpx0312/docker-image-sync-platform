package middleware

import (
	"bytes"
	"io"
	"net/http"
	"strings"
	"testing"

	"github.com/gin-gonic/gin"
)

// newBodyLimitEngine 搭建 body 限制 + 回显 handler 的引擎
func newBodyLimitEngine(maxBytes int64) *gin.Engine {
	engine := newTestEngine()
	engine.Use(BodySizeLimit(maxBytes))
	engine.POST("/echo", func(c *gin.Context) {
		var payload struct {
			Data string `json:"data"`
		}
		if err := c.ShouldBindJSON(&payload); err != nil {
			c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
			return
		}
		c.JSON(http.StatusOK, gin.H{"len": len(payload.Data)})
	})
	engine.GET("/read", func(c *gin.Context) {
		if c.Request.Body == nil {
			c.JSON(http.StatusOK, gin.H{"len": 0})
			return
		}
		body, err := io.ReadAll(c.Request.Body)
		if err != nil {
			c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
			return
		}
		c.JSON(http.StatusOK, gin.H{"len": len(body)})
	})
	return engine
}

func TestBodySizeLimit_WithinLimit(t *testing.T) {
	engine := newBodyLimitEngine(1024)

	// 小 JSON 正常通过
	rec := performRequest(engine, http.MethodPost, "/echo",
		strings.NewReader(`{"data":"hello 中文 🐳"}`), nil)
	ensureStatus(t, rec, http.StatusOK)
	body := decodeJSON(t, rec)
	eq(t, float64(len("hello 中文 🐳")), body["len"])
}

func TestBodySizeLimit_ExactLimit(t *testing.T) {
	engine := newBodyLimitEngine(64)

	// 恰好等于上限字节数的请求应通过
	payload := `{"data":"01234567890123456789012345678901234567890123"}` // 54 字节
	isTrue(t, int64(len(payload)) <= 64, "测试数据应不超过上限")
	rec := performRequest(engine, http.MethodPost, "/echo", strings.NewReader(payload), nil)
	ensureStatus(t, rec, http.StatusOK)
}

func TestBodySizeLimit_RejectsOverLimit(t *testing.T) {
	engine := newBodyLimitEngine(1024)

	// 超过上限：MaxBytesReader 触发 "http: request body too large"
	huge := `{"data":"` + strings.Repeat("a", 2048) + `"}`
	rec := performRequest(engine, http.MethodPost, "/echo", strings.NewReader(huge), nil)
	ensureStatus(t, rec, http.StatusBadRequest)
	body := decodeJSON(t, rec)
	isTrue(t, strings.Contains(body["error"].(string), "body too large"),
		"应返回 body too large 错误, got: %v", body["error"])
}

func TestBodySizeLimit_ZeroLimit(t *testing.T) {
	engine := newBodyLimitEngine(0)

	// 上限为 0：任何非空请求体都会被拒
	rec := performRequest(engine, http.MethodPost, "/echo",
		strings.NewReader(`{"data":"x"}`), nil)
	ensureStatus(t, rec, http.StatusBadRequest)

	// 空请求体（GET /read，http.NoBody）在 0 上限下可读 0 字节
	rec = performRequest(engine, http.MethodGet, "/read", nil, nil)
	ensureStatus(t, rec, http.StatusOK)
	eq(t, float64(0), decodeJSON(t, rec)["len"])
}

func TestBodySizeLimit_NilBody(t *testing.T) {
	engine := newBodyLimitEngine(1024)

	// 手动将 Body 置为 nil：中间件应跳过包装，请求正常
	req := newRequestNilBody(http.MethodGet, "/read")
	rec := httptestRecorder()
	engine.ServeHTTP(rec, req)
	ensureStatus(t, rec, http.StatusOK)
	eq(t, float64(0), decodeJSON(t, rec)["len"])
}

func TestBodySizeLimit_PartialReadThenOver(t *testing.T) {
	engine := newBodyLimitEngine(32)

	// 超限 body 用 io.ReadAll 读取同样失败
	huge := bytes.Repeat([]byte("x"), 512)
	rec := performRequest(engine, http.MethodGet, "/read", bytes.NewReader(huge), nil)
	ensureStatus(t, rec, http.StatusBadRequest)
}
