package handlers

import (
	"net/http"

	"github.com/gin-gonic/gin"
	"docker-image-sync-platform/internal/database"
	"docker-image-sync-platform/internal/models"
)

// ConfigHandler 配置处理器
type ConfigHandler struct{}

// NewConfigHandler 创建配置处理器
func NewConfigHandler() *ConfigHandler {
	return &ConfigHandler{}
}

// GetAliyunConfig 获取阿里云配置信息
func (h *ConfigHandler) GetAliyunConfig(c *gin.Context) {
	var registryConfig models.SystemConfig
	var namespaceConfig models.SystemConfig

	// 获取阿里云仓库前缀
	database.DB.Where("config_key = ?", "aliyun_registry_prefix").First(&registryConfig)
	
	// 获取阿里云命名空间
	database.DB.Where("config_key = ?", "aliyun_namespace").First(&namespaceConfig)

	registry := registryConfig.ConfigValue
	if registry == "" {
		registry = "registry.cn-hangzhou.aliyuncs.com"
	}

	namespace := namespaceConfig.ConfigValue
	if namespace == "" {
		namespace = "docker-sync"
	}

	c.JSON(http.StatusOK, gin.H{
		"registry":  registry,
		"namespace": namespace,
	})
}