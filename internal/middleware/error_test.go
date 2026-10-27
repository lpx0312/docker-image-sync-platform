package middleware

import (
	"encoding/json"
	"errors"
	"net/http"
	"testing"

	"github.com/gin-gonic/gin"
	"go.uber.org/zap"
	"go.uber.org/zap/zapcore"
)

// ---------- ErrorHandler ----------

func newErrorEngine(logger *zap.Logger) *gin.Engine {
	engine := newTestEngine()
	engine.Use(ErrorHandler(logger))
	engine.GET("/ok", func(c *gin.Context) { c.JSON(http.StatusOK, gin.H{"ok": true}) })
	engine.GET("/panic-string", func(c *gin.Context) { panic("boom-string") })
	engine.GET("/panic-error", func(c *gin.Context) { panic(errors.New("boom-error")) })
	engine.GET("/panic-int", func(c *gin.Context) { panic(42) })
	return engine
}

func TestErrorHandler_NoPanic(t *testing.T) {
	logger, logs := newObservedLogger(t)
	engine := newErrorEngine(logger)

	rec := performRequest(engine, http.MethodGet, "/ok", nil, nil)
	ensureStatus(t, rec, http.StatusOK)
	eq(t, true, decodeJSON(t, rec)["ok"])
	eq(t, 0, logs.Len())
}

func TestErrorHandler_RecoversStringPanic(t *testing.T) {
	logger, logs := newObservedLogger(t)
	engine := newErrorEngine(logger)

	rec := performRequest(engine, http.MethodGet, "/panic-string", nil, nil)
	ensureStatus(t, rec, http.StatusInternalServerError)
	body := decodeJSON(t, rec)
	eq(t, float64(500), body["code"])
	eq(t, "Internal server error", body["message"])
	// 内部细节不泄漏
	if _, exists := body["error"]; exists {
		t.Error("不应包含 error 明细")
	}

	// 字符串 panic 记录了日志（含堆栈与路径）
	entries := logs.FilterMessage("Panic recovered")
	eq(t, 1, entries.Len())
	ctx := entries.All()[0].ContextMap()
	eq(t, "boom-string", ctx["error"])
	eq(t, "/panic-string", ctx["path"])
	eq(t, "GET", ctx["method"])
	if _, hasStack := ctx["stack"]; !hasStack {
		t.Error("日志应包含 stack 字段")
	}
}

func TestErrorHandler_RecoversNonStringPanic(t *testing.T) {
	// 非 string 的 panic 值（error / int）同样被恢复为 500，
	// 但当前实现不记录日志（类型断言 string 失败直接跳过日志分支）
	for _, path := range []string{"/panic-error", "/panic-int"} {
		t.Run(path, func(t *testing.T) {
			logger, logs := newObservedLogger(t)
			engine := newErrorEngine(logger)

			rec := performRequest(engine, http.MethodGet, path, nil, nil)
			ensureStatus(t, rec, http.StatusInternalServerError)
			body := decodeJSON(t, rec)
			eq(t, float64(500), body["code"])
			eq(t, "Internal server error", body["message"])
			eq(t, 0, logs.Len())
		})
	}
}

func TestErrorHandler_NilLoggerWithNonStringPanic(t *testing.T) {
	// 非 string panic 且未记录日志时不会调用 logger，nil logger 也不会 panic
	engine := newTestEngine()
	engine.Use(ErrorHandler(nil))
	engine.GET("/boom", func(c *gin.Context) { panic(errors.New("x")) })

	rec := performRequest(engine, http.MethodGet, "/boom", nil, nil)
	ensureStatus(t, rec, http.StatusInternalServerError)
}

// ---------- APIError ----------

func TestAPIError_Error(t *testing.T) {
	// 无底层错误：返回 Message
	e := &APIError{Code: 400, Message: "参数错误"}
	eq(t, "参数错误", e.Error())

	// 有底层错误：优先返回底层错误信息
	underlying := errors.New("db connection refused")
	e = &APIError{Code: 500, Message: "内部错误", Err: underlying}
	eq(t, "db connection refused", e.Error())
}

func TestNewAPIError(t *testing.T) {
	underlying := errors.New("boom")
	e := NewAPIError(http.StatusConflict, "冲突", underlying)
	eq(t, http.StatusConflict, e.Code)
	eq(t, "冲突", e.Message)
	eq(t, underlying, e.Err)

	e2 := NewAPIError(400, "msg", nil)
	isTrue(t, e2.Err == nil, "Err 应为 nil")
	eq(t, "msg", e2.Error())
}

func TestAPIError_JSONHidesErr(t *testing.T) {
	e := NewAPIError(500, "内部错误", errors.New("secret-detail"))
	data, err := json.Marshal(e)
	ok(t, err, "Marshal 失败")
	m := map[string]any{}
	ok(t, json.Unmarshal(data, &m), "Unmarshal 失败")
	eq(t, float64(500), m["code"])
	eq(t, "内部错误", m["message"])
	if _, exists := m["Err"]; exists {
		t.Error("Err 字段不应序列化")
	}
}

func TestErrorResponseJSONOmitEmptyError(t *testing.T) {
	// Error 为空时不输出该字段
	data, err := json.Marshal(ErrorResponse{Code: 400, Message: "bad"})
	ok(t, err, "Marshal 失败")
	eq(t, `{"code":400,"message":"bad"}`, string(data))

	data, err = json.Marshal(ErrorResponse{Code: 400, Message: "bad", Error: "detail"})
	ok(t, err, "Marshal 失败")
	eq(t, `{"code":400,"message":"bad","error":"detail"}`, string(data))
}

// ---------- 错误构造函数 ----------

func TestErrorConstructors(t *testing.T) {
	v := ValidationError("用户名不能为空")
	eq(t, http.StatusBadRequest, v.Code)
	eq(t, "用户名不能为空", v.Message)
	isTrue(t, v.Err == nil, "ValidationError Err 应为 nil")

	n := NotFoundError("镜像不存在")
	eq(t, http.StatusNotFound, n.Code)

	c := ConflictError("镜像同步中")
	eq(t, http.StatusConflict, c.Code)

	underlying := errors.New("io timeout")
	i := InternalError("数据库操作失败", underlying)
	eq(t, http.StatusInternalServerError, i.Code)
	eq(t, underlying, i.Err)
	eq(t, "io timeout", i.Error())
}

// ---------- HandleError ----------

func TestHandleError_APIError(t *testing.T) {
	logger, logs := newObservedLogger(t)
	engine := newTestEngine()
	engine.GET("/boom", func(c *gin.Context) {
		HandleError(c, logger, NewAPIError(http.StatusNotFound, "镜像记录不存在", errors.New("row not found")))
	})

	rec := performRequest(engine, http.MethodGet, "/boom", nil, nil)
	ensureStatus(t, rec, http.StatusNotFound)
	body := decodeJSON(t, rec)
	eq(t, float64(404), body["code"])
	eq(t, "镜像记录不存在", body["message"])

	entries := logs.FilterMessage("API Error")
	eq(t, 1, entries.Len())
	ctx := entries.All()[0].ContextMap()
	// zap.Int 字段在 ContextMap 中为 int64
	eq(t, int64(404), ctx["code"])
	eq(t, "镜像记录不存在", ctx["message"])
	eq(t, "/boom", ctx["path"])
}

func TestHandleError_GenericError(t *testing.T) {
	logger, logs := newObservedLogger(t)
	engine := newTestEngine()
	engine.GET("/boom", func(c *gin.Context) {
		HandleError(c, logger, errors.New("unexpected failure"))
	})

	rec := performRequest(engine, http.MethodGet, "/boom", nil, nil)
	ensureStatus(t, rec, http.StatusInternalServerError)
	body := decodeJSON(t, rec)
	eq(t, float64(500), body["code"])
	eq(t, "Internal server error", body["message"])

	entries := logs.FilterMessage("Unexpected error")
	eq(t, 1, entries.Len())
	ctx := entries.All()[0].ContextMap()
	eq(t, "/boom", ctx["path"])
	eq(t, "GET", ctx["method"])
}

func TestHandleError_WrappedAPIErrorPointer(t *testing.T) {
	// 通过 error 接口传入 *APIError 应被正确识别
	logger, logs := newObservedLogger(t)
	engine := newTestEngine()
	engine.GET("/wrapped", func(c *gin.Context) {
		var err error = ValidationError("参数非法")
		HandleError(c, logger, err)
	})

	rec := performRequest(engine, http.MethodGet, "/wrapped", nil, nil)
	ensureStatus(t, rec, http.StatusBadRequest)
	eq(t, "参数非法", decodeJSON(t, rec)["message"])
	// 命中 API Error 日志分支
	eq(t, 1, logs.FilterMessage("API Error").Len())
}

func TestHandleError_UsesRequestContext(t *testing.T) {
	logger, logs := newObservedLogger(t)
	engine := newTestEngine()
	engine.POST("/ctx", func(c *gin.Context) {
		HandleError(c, logger, errors.New("x"))
	})

	rec := performRequest(engine, http.MethodPost, "/ctx", nil, nil)
	ensureStatus(t, rec, http.StatusInternalServerError)
	// 日志携带请求上下文（POST 方法）
	entry := logs.FilterMessage("Unexpected error").All()[0]
	eq(t, "POST", entry.ContextMap()["method"])
}

// 编译期确认 ContextMap 数值类型的取值方式（避免 magic 断言错误）
var _ = zapcore.InfoLevel
