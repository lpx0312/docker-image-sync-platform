// Package handlers 提供HTTP请求处理器实现
//
// config.go 文件实现了系统配置相关的HTTP处理器，主要负责：
// - 阿里云容器镜像服务(ACR)配置的查询和管理
// - 系统配置项的统一访问接口
// - 配置默认值的处理和验证
//
// 配置管理功能：
// - 支持从数据库动态读取配置
// - 提供配置项的默认值机制
// - 确保配置的一致性和可用性
//
// 主要配置项：
// - aliyun_registry_prefix: 阿里云容器注册表地址前缀
// - aliyun_namespace: 阿里云容器注册表命名空间
//
// 作者: Docker镜像同步平台开发团队
// 版本: v1.0.0
package handlers

import (
	"net/http"                                               // HTTP状态码和处理

	"github.com/gin-gonic/gin"                               // Gin Web框架
	"docker-image-sync-platform/internal/database"          // 数据库操作
	"docker-image-sync-platform/internal/models"            // 数据模型
)

// ConfigHandler 系统配置处理器
//
// 负责处理与系统配置相关的HTTP请求，包括阿里云ACR配置的
// 查询和管理功能。
//
// 主要功能:
//   - 阿里云ACR注册表配置查询
//   - 命名空间配置管理
//   - 系统配置的默认值处理
//   - 配置信息的统一返回格式
//
// 配置项说明:
//   - aliyun_registry_prefix: 阿里云容器注册表地址前缀
//   - aliyun_namespace: 阿里云容器注册表命名空间
//
// 设计原则:
//   - 配置项支持动态更新，无需重启服务
//   - 提供合理的默认值，确保系统可用性
//   - 配置访问统一化，便于维护和扩展
type ConfigHandler struct{}

// NewConfigHandler 创建系统配置处理器实例
//
// 返回:
//   - *ConfigHandler: 配置处理器实例
//
// 使用示例:
//   configHandler := NewConfigHandler()
//   router.GET("/config/aliyun", configHandler.GetAliyunConfig)
func NewConfigHandler() *ConfigHandler {
	return &ConfigHandler{}
}

// GetAliyunConfig 获取阿里云ACR配置信息
//
// HTTP方法: GET
// 路径: /api/v1/config/aliyun
//
// 响应码:
//   - 200: 成功返回阿里云配置信息
//   - 500: 服务器内部错误（数据库查询失败）
//
// 响应数据:
//   - registry: 阿里云容器注册表地址（如 registry.cn-hangzhou.aliyuncs.com）
//   - namespace: 阿里云容器注册表命名空间（如 docker-sync）
//
// 默认值处理:
//   - registry默认值: registry.cn-hangzhou.aliyuncs.com（杭州区域）
//   - namespace默认值: docker-sync
//
// 功能说明:
//   - 从系统配置表中读取阿里云ACR相关配置
//   - 提供配置的默认值机制，确保系统正常运行
//   - 用于前端显示当前的阿里云配置信息
//   - 支持镜像同步服务的配置获取
//
// 配置项详解:
//   - aliyun_registry_prefix: 阿里云ACR的完整域名地址
//     * 格式: registry.{region}.aliyuncs.com
//     * 示例: registry.cn-hangzhou.aliyuncs.com
//     * 用途: 构建完整的镜像推送地址
//   - aliyun_namespace: ACR中的命名空间
//     * 格式: 字符串，符合ACR命名规范
//     * 示例: docker-sync, my-images
//     * 用途: 组织和隔离不同项目的镜像
//
// 使用场景:
//   - 前端配置页面显示当前配置
//   - 镜像同步时获取目标仓库信息
//   - 系统初始化时验证配置完整性
func (h *ConfigHandler) GetAliyunConfig(c *gin.Context) {
	// ====================================================================
	// 初始化配置变量
	// ====================================================================
	
	var registryConfig models.SystemConfig   // 注册表地址配置
	var namespaceConfig models.SystemConfig  // 命名空间配置

	// ====================================================================
	// 查询阿里云注册表配置
	// ====================================================================
	
	// 获取阿里云容器注册表地址前缀配置
	// 配置键: aliyun_registry_prefix
	// 用途: 指定阿里云ACR的完整地址，如 registry.cn-hangzhou.aliyuncs.com
	database.DB.Where("config_key = ?", "aliyun_registry_prefix").First(&registryConfig)
	
	// 获取阿里云容器注册表命名空间配置
	// 配置键: aliyun_namespace
	// 用途: 指定在ACR中使用的命名空间，用于组织和隔离镜像
	database.DB.Where("config_key = ?", "aliyun_namespace").First(&namespaceConfig)

	// ====================================================================
	// 配置值处理和默认值设置
	// ====================================================================
	
	// 处理注册表地址配置
	// 如果数据库中没有配置或配置为空，使用默认的杭州区域地址
	registry := registryConfig.ConfigValue
	if registry == "" {
		registry = "registry.cn-hangzhou.aliyuncs.com"  // 默认使用杭州区域
	}

	// 处理命名空间配置
	// 如果数据库中没有配置或配置为空，使用默认命名空间
	namespace := namespaceConfig.ConfigValue
	if namespace == "" {
		namespace = "docker-sync"  // 默认命名空间
	}

	// ====================================================================
	// 返回配置信息
	// ====================================================================
	
	// 返回完整的阿里云ACR配置信息
	// 响应格式为标准JSON，包含registry和namespace两个字段
	c.JSON(http.StatusOK, gin.H{
		"registry":  registry,   // 阿里云容器注册表地址
		"namespace": namespace,  // 阿里云容器注册表命名空间
	})
}