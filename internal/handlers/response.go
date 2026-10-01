package handlers

import (
	"net/http"

	"docker-image-sync-platform/internal/logger"

	"github.com/gin-gonic/gin"
	"go.uber.org/zap"
)

// respondInternalError 统一内部错误响应。
//
// 完整错误详情（含 GORM SQL 片段、数据库主机、解密失败原因等）只进日志，
// 客户端仅收到通用文案，避免内部信息泄漏。需要向用户传达具体原因的业务
// 错误（如参数校验、连接测试结果）不应使用本函数。
func respondInternalError(c *gin.Context, operation string, err error) {
	logger.Logger.Error(operation, zap.Error(err))
	c.JSON(http.StatusInternalServerError, gin.H{"error": "服务器内部错误，请稍后重试"})
}
