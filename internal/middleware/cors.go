// Package middleware 提供了HTTP中间件功能，包括跨域资源共享(CORS)、错误处理、日志记录和速率限制等。
// 这些中间件用于增强Web服务的安全性、可观测性和性能控制。
//
// 主要功能：
//   - CORS: 处理跨域请求，支持预检请求和自定义配置
//   - Error: 统一错误处理和响应格式化
//   - Logger: 请求日志记录和性能监控
//   - RateLimit: 请求频率限制和防护
//
// 使用示例：
//
//	router := gin.New()
//	router.Use(middleware.CORS())
//	router.Use(middleware.Logger())
//	router.Use(middleware.ErrorHandler())
package middleware

import (
	"net/http"

	"docker-image-sync-platform/internal/config"

	"github.com/gin-gonic/gin"
)

// CORS 创建跨域资源共享中间件，来源白名单由配置 security.cors.allowed_origins 驱动。
//
// 安全说明：
//   - 平台认证使用 Authorization 请求头（token 存 localStorage），不使用 Cookie，
//     因此不需要 Access-Control-Allow-Credentials，也不反射请求 Origin；
//     反射 Origin + credentials 的组合会让任意恶意网页携带凭据跨域调用全部 API
//   - allowed_origins 为空或包含 "*" 时返回通配 "*"；配置了具体域名列表时，
//     仅白名单内的 Origin 会获得 Access-Control-Allow-Origin 回显，其余请求
//     不设置该响应头（浏览器侧拦截跨域读取，非浏览器调用不受影响）
func CORS() gin.HandlerFunc {
	allowedOrigins := []string{"*"}
	if config.AppConfig != nil && len(config.AppConfig.Security.CORS.AllowedOrigins) > 0 {
		allowedOrigins = config.AppConfig.Security.CORS.AllowedOrigins
	}

	wildcard := false
	exact := make(map[string]struct{}, len(allowedOrigins))
	for _, o := range allowedOrigins {
		if o == "*" {
			wildcard = true
			break
		}
		exact[o] = struct{}{}
	}

	return func(c *gin.Context) {
		method := c.Request.Method
		origin := c.Request.Header.Get("Origin")

		if wildcard {
			c.Header("Access-Control-Allow-Origin", "*")
		} else if origin != "" {
			if _, ok := exact[origin]; ok {
				c.Header("Access-Control-Allow-Origin", origin)
				c.Set("origin", origin)
			}
			// 白名单外的 Origin 不回显 Allow-Origin，浏览器将拒绝跨域读取
		}

		c.Header("Access-Control-Allow-Methods", "GET, POST, PUT, DELETE, OPTIONS")
		c.Header("Access-Control-Allow-Headers", "Origin, X-Requested-With, Content-Type, Accept, Authorization, Cache-Control, X-File-Name")
		c.Header("Access-Control-Expose-Headers", "Content-Length, Content-Type")

		if method == "OPTIONS" {
			c.AbortWithStatus(http.StatusNoContent)
			return
		}

		c.Next()
	}
}


// CORSWithConfig 创建一个可配置的跨域资源共享(CORS)中间件。
//
// 功能说明：
//   - 支持自定义允许的来源、方法和请求头
//   - 严格验证请求来源，拒绝未授权的跨域请求
//   - 自动处理OPTIONS预检请求
//   - 灵活配置CORS策略以满足不同安全需求
//
// 参数：
//   - allowOrigins: 允许的来源列表，支持具体域名或"*"通配符
//   - allowMethods: 允许的HTTP方法列表，如["GET", "POST", "PUT"]
//   - allowHeaders: 允许的请求头列表，如["Content-Type", "Authorization"]
//
// 返回值：
//   - gin.HandlerFunc: 配置好的Gin框架中间件函数
//
// 安全特性：
//   - 来源白名单验证，拒绝未授权域名的请求
//   - 方法和头部限制，防止不当的跨域操作
//   - 返回403状态码拒绝非法请求
//
// 使用场景：
//   - 生产环境需要严格控制跨域访问
//   - 多域名或子域名的复杂部署场景
//   - 需要精确控制允许的HTTP方法和头部
//
// 示例：
//
//	origins := []string{"https://example.com", "https://app.example.com"}
//	methods := []string{"GET", "POST", "PUT", "DELETE"}
//	headers := []string{"Content-Type", "Authorization", "X-API-Key"}
//	router.Use(middleware.CORSWithConfig(origins, methods, headers))
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

		c.Header("Access-Control-Allow-Origin", allowOrigin)
		c.Header("Access-Control-Allow-Methods", allowMethodsStr)
		c.Header("Access-Control-Allow-Headers", allowHeadersStr)
		c.Header("Access-Control-Expose-Headers", "Content-Length, Content-Type")
		// 平台认证走 Authorization 头而非 Cookie，无需 Allow-Credentials

		// 处理预检请求
		if method == "OPTIONS" {
			c.AbortWithStatus(http.StatusNoContent)
			return
		}

		c.Next()
	}
}
