package middleware

import (
	"net/http"

	"github.com/gin-gonic/gin"
)

// CORS 跨域中间件
func CORS() gin.HandlerFunc {
	return func(c *gin.Context) {
		method := c.Request.Method
		origin := c.Request.Header.Get("Origin")

		// 设置CORS头
		c.Header("Access-Control-Allow-Origin", "*")
		c.Header("Access-Control-Allow-Methods", "GET, POST, PUT, DELETE, OPTIONS")
		c.Header("Access-Control-Allow-Headers", "Origin, X-Requested-With, Content-Type, Accept, Authorization, Cache-Control, X-File-Name")
		c.Header("Access-Control-Expose-Headers", "Content-Length, Access-Control-Allow-Origin, Access-Control-Allow-Headers, Content-Type")
		c.Header("Access-Control-Allow-Credentials", "true")

		// 处理预检请求
		if method == "OPTIONS" {
			c.AbortWithStatus(http.StatusNoContent)
			return
		}

		// 记录跨域请求
		if origin != "" {
			c.Set("origin", origin)
		}

		c.Next()
	}
}

// CORSWithConfig 带配置的CORS中间件
func CORSWithConfig(allowOrigins []string, allowMethods []string, allowHeaders []string) gin.HandlerFunc {
	return func(c *gin.Context) {
		method := c.Request.Method
		origin := c.Request.Header.Get("Origin")

		// 检查是否允许该来源
		allowOrigin := "*"
		if len(allowOrigins) > 0 {
			allowOrigin = ""
			for _, ao := range allowOrigins {
				if ao == origin || ao == "*" {
					allowOrigin = origin
					break
				}
			}
			if allowOrigin == "" {
				c.AbortWithStatus(http.StatusForbidden)
				return
			}
		}

		// 设置允许的方法
		allowMethodsStr := "GET, POST, PUT, DELETE, OPTIONS"
		if len(allowMethods) > 0 {
			allowMethodsStr = ""
			for i, method := range allowMethods {
				if i > 0 {
					allowMethodsStr += ", "
				}
				allowMethodsStr += method
			}
		}

		// 设置允许的头部
		allowHeadersStr := "Origin, X-Requested-With, Content-Type, Accept, Authorization, Cache-Control, X-File-Name"
		if len(allowHeaders) > 0 {
			allowHeadersStr = ""
			for i, header := range allowHeaders {
				if i > 0 {
					allowHeadersStr += ", "
				}
				allowHeadersStr += header
			}
		}

		// 设置CORS头
		c.Header("Access-Control-Allow-Origin", allowOrigin)
		c.Header("Access-Control-Allow-Methods", allowMethodsStr)
		c.Header("Access-Control-Allow-Headers", allowHeadersStr)
		c.Header("Access-Control-Expose-Headers", "Content-Length, Access-Control-Allow-Origin, Access-Control-Allow-Headers, Content-Type")
		c.Header("Access-Control-Allow-Credentials", "true")

		// 处理预检请求
		if method == "OPTIONS" {
			c.AbortWithStatus(http.StatusNoContent)
			return
		}

		c.Next()
	}
}