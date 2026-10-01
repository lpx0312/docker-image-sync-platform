// Package middleware 提供了HTTP中间件功能，包括跨域资源共享(CORS)、错误处理、日志记录和速率限制等。
package middleware

import (
	"net/http"

	"github.com/gin-gonic/gin"
)

// BodySizeLimit 限制请求体大小，防止超大 JSON/multipart 消耗内存。
//
// 超限时底层读取返回 "http: request body too large"，ShouldBindJSON 会以
// 400 返回给客户端；对已开始写入响应的流式请求，连接会被中断。
//
// 用途：全局兜底（建议 1-5MB），防止恶意/失控客户端把整个请求体读入内存。
func BodySizeLimit(maxBytes int64) gin.HandlerFunc {
	return func(c *gin.Context) {
		if c.Request.Body != nil {
			c.Request.Body = http.MaxBytesReader(c.Writer, c.Request.Body, maxBytes)
		}
		c.Next()
	}
}
