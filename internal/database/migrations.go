package database

import (
	"docker-image-sync-platform/internal/models"
	"docker-image-sync-platform/internal/logger"

	"go.uber.org/zap"
)

// RunMigrations 执行数据库迁移
func RunMigrations() {
	logger.Logger.Info("开始执行数据库迁移...")

	// 自动迁移 ACR 配置表
	if err := DB.AutoMigrate(&models.AcrRegistry{}); err != nil {
		logger.Logger.Error("ACR配置表迁移失败", zap.Error(err))
	} else {
		logger.Logger.Info("ACR配置表迁移成功")
	}

	// 自动迁移 ACR 镜像仓库表
	if err := DB.AutoMigrate(&models.AcrRepository{}); err != nil {
		logger.Logger.Error("ACR镜像仓库表迁移失败", zap.Error(err))
	} else {
		logger.Logger.Info("ACR镜像仓库表迁移成功")
	}

	logger.Logger.Info("数据库迁移完成")
}
