package handlers

// response_test.go 覆盖 response.go：统一 500 响应（详情只进日志）

import (
	"errors"
	"net/http"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/assert"
)

func TestRespondInternalError(t *testing.T) {
	e := hNewEngine()
	e.GET("/boom", func(c *gin.Context) {
		respondInternalError(c, "op-x", errors.New("inner detail"))
	})
	w, body := hPerformJSON(t, e, "GET", "/boom", "", nil)
	assert.Equal(t, http.StatusInternalServerError, w.Code)
	// 内部错误详情不外泄
	assert.Equal(t, "服务器内部错误，请稍后重试", body["error"])
}
