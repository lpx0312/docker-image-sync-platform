package middleware

import (
	"net/http"
	"runtime/debug"

	"github.com/gin-gonic/gin"
	"go.uber.org/zap"
)

// ErrorResponse 错误响应结构
type ErrorResponse struct {
	Code    int    `json:"code"`
	Message string `json:"message"`
	Error   string `json:"error,omitempty"`
}

// ErrorHandler 全局错误处理中间件
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

// APIError 自定义API错误
type APIError struct {
	Code    int    `json:"code"`
	Message string `json:"message"`
	Err     error  `json:"-"`
}

func (e *APIError) Error() string {
	if e.Err != nil {
		return e.Err.Error()
	}
	return e.Message
}

// NewAPIError 创建新的API错误
func NewAPIError(code int, message string, err error) *APIError {
	return &APIError{
		Code:    code,
		Message: message,
		Err:     err,
	}
}

// HandleError 处理错误并返回JSON响应
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

// ValidationError 验证错误
func ValidationError(message string) *APIError {
	return NewAPIError(http.StatusBadRequest, message, nil)
}

// NotFoundError 资源未找到错误
func NotFoundError(message string) *APIError {
	return NewAPIError(http.StatusNotFound, message, nil)
}

// ConflictError 冲突错误
func ConflictError(message string) *APIError {
	return NewAPIError(http.StatusConflict, message, nil)
}

// InternalError 内部错误
func InternalError(message string, err error) *APIError {
	return NewAPIError(http.StatusInternalServerError, message, err)
}