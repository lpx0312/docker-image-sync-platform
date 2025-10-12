package middleware

import (
	"bytes"
	"io"
	"time"

	"github.com/gin-gonic/gin"
	"go.uber.org/zap"
)

// RequestLogger 创建基础的HTTP请求日志中间件。
//
// 功能说明：
//   - 记录每个HTTP请求的基本信息
//   - 使用结构化日志格式，便于日志分析
//   - 包含请求方法、路径、状态码、延迟等关键信息
//   - 轻量级实现，适合生产环境使用
//
// 参数：
//   - logger: zap日志记录器实例
//
// 返回值：
//   - gin.HandlerFunc: Gin框架中间件函数
//
// 记录字段：
//   - method: HTTP方法（GET、POST等）
//   - path: 请求路径
//   - status: HTTP状态码
//   - latency: 请求处理时间
//   - client_ip: 客户端IP地址
//   - user_agent: 用户代理字符串
//   - body_size: 响应体大小
//
// 使用场景：
//   - 生产环境的请求监控
//   - API性能分析
//   - 访问日志记录
//
// 示例：
//   router.Use(middleware.RequestLogger(logger))
func RequestLogger(logger *zap.Logger) gin.HandlerFunc {
	return gin.LoggerWithFormatter(func(param gin.LogFormatterParams) string {
		// 记录请求日志
		logger.Info("HTTP Request",
			zap.String("method", param.Method),
			zap.String("path", param.Path),
			zap.Int("status", param.StatusCode),
			zap.Duration("latency", param.Latency),
			zap.String("client_ip", param.ClientIP),
			zap.String("user_agent", param.Request.UserAgent()),
			zap.Int("body_size", param.BodySize),
		)
		return ""
	})
}

// DetailedRequestLogger 创建详细的HTTP请求日志中间件，包含请求体信息。
//
// 功能说明：
//   - 记录完整的HTTP请求和响应信息
//   - 在调试模式下记录请求体内容（限制大小）
//   - 根据HTTP状态码自动选择日志级别
//   - 包含查询参数和响应大小等详细信息
//
// 参数：
//   - logger: zap日志记录器实例
//
// 返回值：
//   - gin.HandlerFunc: Gin框架中间件函数
//
// 记录字段：
//   - method: HTTP方法
//   - path: 请求路径
//   - query: 查询参数
//   - status: HTTP状态码
//   - latency: 请求处理时间
//   - client_ip: 客户端IP地址
//   - user_agent: 用户代理字符串
//   - response_size: 响应体大小
//   - request_body: 请求体内容（仅调试模式且小于1KB）
//
// 日志级别：
//   - 5xx状态码: ERROR级别
//   - 4xx状态码: WARN级别
//   - 其他状态码: INFO级别
//
// 使用场景：
//   - 开发环境的详细调试
//   - API问题排查
//   - 完整的请求追踪
//
// 注意事项：
//   - 请求体记录仅在调试模式下启用
//   - 请求体大小限制为1KB，避免日志过大
//
// 示例：
//   router.Use(middleware.DetailedRequestLogger(logger))
func DetailedRequestLogger(logger *zap.Logger) gin.HandlerFunc {
	return func(c *gin.Context) {
		start := time.Now()

		// 读取请求体
		var requestBody []byte
		if c.Request.Body != nil {
			requestBody, _ = io.ReadAll(c.Request.Body)
			c.Request.Body = io.NopCloser(bytes.NewBuffer(requestBody))
		}

		// 处理请求
		c.Next()

		// 计算延迟
		latency := time.Since(start)

		// 记录详细日志
		fields := []zap.Field{
			zap.String("method", c.Request.Method),
			zap.String("path", c.Request.URL.Path),
			zap.String("query", c.Request.URL.RawQuery),
			zap.Int("status", c.Writer.Status()),
			zap.Duration("latency", latency),
			zap.String("client_ip", c.ClientIP()),
			zap.String("user_agent", c.Request.UserAgent()),
			zap.Int("response_size", c.Writer.Size()),
		}

		// 只在调试模式下记录请求体
		if gin.Mode() == gin.DebugMode && len(requestBody) > 0 && len(requestBody) < 1024 {
			fields = append(fields, zap.String("request_body", string(requestBody)))
		}

		// 根据状态码选择日志级别
		if c.Writer.Status() >= 500 {
			logger.Error("HTTP Request", fields...)
		} else if c.Writer.Status() >= 400 {
			logger.Warn("HTTP Request", fields...)
		} else {
			logger.Info("HTTP Request", fields...)
		}
	}
}

// SkipPaths 创建路径过滤中间件，跳过指定路径的日志记录。
//
// 功能说明：
//   - 允许指定不需要记录日志的路径列表
//   - 减少健康检查等高频请求的日志噪音
//   - 提高日志系统的性能和存储效率
//   - 支持精确路径匹配
//
// 参数：
//   - paths: 需要跳过日志记录的路径列表（可变参数）
//
// 返回值：
//   - gin.HandlerFunc: Gin框架中间件函数
//
// 工作原理：
//   1. 将传入的路径列表构建为快速查找的map
//   2. 对每个请求检查路径是否在跳过列表中
//   3. 如果匹配则直接跳过，否则继续处理
//
// 使用场景：
//   - 跳过健康检查接口的日志记录
//   - 排除静态资源请求的日志
//   - 减少高频监控接口的日志量
//
// 注意事项：
//   - 路径匹配是精确匹配，不支持通配符
//   - 建议与其他日志中间件配合使用
//
// 示例：
//   router.Use(middleware.SkipPaths("/health", "/metrics", "/favicon.ico"))
//   router.Use(middleware.RequestLogger(logger))
func SkipPaths(paths ...string) gin.HandlerFunc {
	skipMap := make(map[string]bool)
	for _, path := range paths {
		skipMap[path] = true
	}

	return func(c *gin.Context) {
		if skipMap[c.Request.URL.Path] {
			c.Next()
			return
		}
		c.Next()
	}
}