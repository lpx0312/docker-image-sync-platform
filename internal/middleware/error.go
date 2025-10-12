package middleware

import (
	"net/http"
	"runtime/debug"

	"github.com/gin-gonic/gin"
	"go.uber.org/zap"
)

// ErrorResponse 定义统一的错误响应结构体，用于API错误返回。
//
// 字段说明：
//   - Code: HTTP状态码，如400、404、500等
//   - Message: 用户友好的错误消息，用于前端显示
//   - Error: 详细错误信息，可选字段，调试时使用
//
// JSON示例：
//   {
//     "code": 400,
//     "message": "参数验证失败",
//     "error": "name字段不能为空"
//   }
type ErrorResponse struct {
	Code    int    `json:"code"`
	Message string `json:"message"`
	Error   string `json:"error,omitempty"`
}

// ErrorHandler 创建全局错误处理和恢复中间件。
//
// 功能说明：
//   - 捕获panic异常并进行恢复，防止服务崩溃
//   - 记录详细的错误日志，包括堆栈信息
//   - 返回统一格式的错误响应给客户端
//   - 隐藏内部错误细节，提高安全性
//
// 参数：
//   - logger: zap日志记录器，用于记录错误信息
//
// 返回值：
//   - gin.HandlerFunc: Gin框架中间件函数
//
// 日志记录：
//   - 错误消息和堆栈跟踪
//   - 请求路径和HTTP方法
//   - 时间戳和日志级别
//
// 使用场景：
//   - 生产环境的错误恢复和日志记录
//   - 统一错误响应格式
//   - 防止敏感信息泄露
//
// 示例：
//   router.Use(middleware.ErrorHandler(logger))
func ErrorHandler(logger *zap.Logger) gin.HandlerFunc {
	return gin.CustomRecovery(func(c *gin.Context, recovered interface{}) {
		if err, ok := recovered.(string); ok {
			logger.Error("Panic recovered",
				zap.String("error", err),
				zap.String("path", c.Request.URL.Path),
				zap.String("method", c.Request.Method),
				zap.String("stack", string(debug.Stack())),
			)
		}

		c.JSON(http.StatusInternalServerError, ErrorResponse{
			Code:    http.StatusInternalServerError,
			Message: "Internal server error",
		})
	})
}

// APIError 定义自定义的API错误类型，实现error接口。
//
// 字段说明：
//   - Code: HTTP状态码，用于HTTP响应
//   - Message: 用户友好的错误消息
//   - Err: 底层错误对象，不会序列化到JSON响应中
//
// 特性：
//   - 实现error接口，可以作为标准错误使用
//   - 支持错误链，保留原始错误信息
//   - JSON序列化时自动排除内部错误详情
//
// 使用场景：
//   - 业务逻辑错误的统一表示
//   - 错误信息的分层处理
//   - API响应的标准化
type APIError struct {
	Code    int    `json:"code"`
	Message string `json:"message"`
	Err     error  `json:"-"`
}

// Error 实现error接口，返回错误的字符串表示。
//
// 返回值：
//   - string: 如果存在底层错误则返回其消息，否则返回APIError的消息
//
// 行为说明：
//   - 优先返回底层错误的详细信息
//   - 如果没有底层错误，返回用户友好的消息
//   - 用于日志记录和错误调试
func (e *APIError) Error() string {
	if e.Err != nil {
		return e.Err.Error()
	}
	return e.Message
}

// NewAPIError 创建一个新的API错误实例。
//
// 参数：
//   - code: HTTP状态码，如400、404、500等
//   - message: 用户友好的错误消息，用于前端显示
//   - err: 底层错误对象，可以为nil
//
// 返回值：
//   - *APIError: 新创建的API错误实例
//
// 使用场景：
//   - 业务逻辑中创建标准化错误
//   - 包装底层错误并添加上下文信息
//   - 统一错误处理流程
//
// 示例：
//   err := NewAPIError(400, "用户名不能为空", nil)
//   err := NewAPIError(500, "数据库连接失败", dbErr)
func NewAPIError(code int, message string, err error) *APIError {
	return &APIError{
		Code:    code,
		Message: message,
		Err:     err,
	}
}

// HandleError 统一处理错误并返回标准化的JSON响应。
//
// 功能说明：
//   - 识别APIError类型并使用其状态码和消息
//   - 记录详细的错误日志，包括请求上下文
//   - 对于非APIError类型，返回通用的500错误
//   - 确保错误响应格式的一致性
//
// 参数：
//   - c: Gin上下文对象，用于HTTP响应
//   - logger: zap日志记录器，用于错误日志
//   - err: 要处理的错误对象
//
// 错误处理逻辑：
//   1. 检查是否为APIError类型
//   2. 记录错误日志（包含请求路径、方法等上下文）
//   3. 返回相应的HTTP状态码和错误消息
//   4. 对未知错误返回500状态码
//
// 使用场景：
//   - 控制器中的统一错误处理
//   - 业务逻辑错误的标准化响应
//   - 错误日志的集中记录
//
// 示例：
//   if err != nil {
//       HandleError(c, logger, err)
//       return
//   }
func HandleError(c *gin.Context, logger *zap.Logger, err error) {
	if apiErr, ok := err.(*APIError); ok {
		logger.Error("API Error",
			zap.Int("code", apiErr.Code),
			zap.String("message", apiErr.Message),
			zap.Error(apiErr.Err),
			zap.String("path", c.Request.URL.Path),
			zap.String("method", c.Request.Method),
		)

		c.JSON(apiErr.Code, ErrorResponse{
			Code:    apiErr.Code,
			Message: apiErr.Message,
		})
		return
	}

	// 默认错误处理
	logger.Error("Unexpected error",
		zap.Error(err),
		zap.String("path", c.Request.URL.Path),
		zap.String("method", c.Request.Method),
	)

	c.JSON(http.StatusInternalServerError, ErrorResponse{
		Code:    http.StatusInternalServerError,
		Message: "Internal server error",
	})
}

// ValidationError 创建参数验证错误（400 Bad Request）。
//
// 参数：
//   - message: 验证失败的具体原因
//
// 返回值：
//   - *APIError: 状态码为400的API错误
//
// 使用场景：
//   - 请求参数格式错误
//   - 必填字段缺失
//   - 参数值不符合业务规则
//
// 示例：
//   return ValidationError("用户名长度必须在3-20个字符之间")
func ValidationError(message string) *APIError {
	return NewAPIError(http.StatusBadRequest, message, nil)
}

// NotFoundError 创建资源未找到错误（404 Not Found）。
//
// 参数：
//   - message: 未找到资源的描述信息
//
// 返回值：
//   - *APIError: 状态码为404的API错误
//
// 使用场景：
//   - 请求的资源不存在
//   - 用户无权访问的资源
//   - 已删除的资源
//
// 示例：
//   return NotFoundError("指定的镜像记录不存在")
func NotFoundError(message string) *APIError {
	return NewAPIError(http.StatusNotFound, message, nil)
}

// ConflictError 创建资源冲突错误（409 Conflict）。
//
// 参数：
//   - message: 冲突的具体原因
//
// 返回值：
//   - *APIError: 状态码为409的API错误
//
// 使用场景：
//   - 资源已存在，无法重复创建
//   - 并发操作导致的状态冲突
//   - 业务规则冲突
//
// 示例：
//   return ConflictError("该镜像正在同步中，请勿重复操作")
func ConflictError(message string) *APIError {
	return NewAPIError(http.StatusConflict, message, nil)
}

// InternalError 创建内部服务器错误（500 Internal Server Error）。
//
// 参数：
//   - message: 用户友好的错误消息
//   - err: 底层错误对象，用于日志记录和调试
//
// 返回值：
//   - *APIError: 状态码为500的API错误
//
// 使用场景：
//   - 数据库连接失败
//   - 外部服务调用失败
//   - 系统内部异常
//
// 示例：
//   return InternalError("数据库操作失败", dbErr)
func InternalError(message string, err error) *APIError {
	return NewAPIError(http.StatusInternalServerError, message, err)
}