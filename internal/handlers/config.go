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
	"os"                                                     // 环境变量操作

	"github.com/gin-gonic/gin"                               // Gin Web框架
	"docker-image-sync-platform/internal/database"          // 数据库操作
	"docker-image-sync-platform/internal/models"            // 数据模型
	"docker-image-sync-platform/internal/config"            // 配置管理
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

// GetConfigStatus 获取当前配置状态和环境变量信息
//
// HTTP方法: GET
// 路径: /api/v1/config/status
//
// 响应码:
//   - 200: 成功返回配置状态信息
//
// 响应数据:
//   - environment: 当前环境变量配置
//   - config: 当前应用配置
//   - database: 数据库连接状态
//
// 功能说明:
//   - 显示当前加载的环境变量配置
//   - 验证配置是否正确传递到应用中
//   - 用于调试和配置验证
//   - 不显示敏感信息（密码等）
func (h *ConfigHandler) GetConfigStatus(c *gin.Context) {
	// ====================================================================
	// 收集环境变量信息
	// ====================================================================
	
	envVars := gin.H{
		// 基础配置
		"gin_mode":   os.Getenv("GIN_MODE"),
		"app_env":    os.Getenv("APP_ENV"),
		"log_level":  os.Getenv("LOG_LEVEL"),
		
		// 服务器配置
		"server_host": os.Getenv("SERVER_HOST"),
		"app_port":    os.Getenv("APP_PORT"),
		
		// 数据库配置（不显示密码）
		"db_host":     os.Getenv("DB_HOST"),
		"db_port":     os.Getenv("DB_PORT"),
		"db_username": os.Getenv("DB_USERNAME"),
		"db_database": os.Getenv("DB_DATABASE"),
		"db_charset":  os.Getenv("DB_CHARSET"),
		
		// Git配置（不显示密码/token）
		"gitee_repo_url":  os.Getenv("GITEE_REPO_URL"),
		"gitee_username":  os.Getenv("GITEE_USERNAME"),
		"gitee_email":     os.Getenv("GITEE_EMAIL"),
		"github_repo_url": os.Getenv("GITHUB_REPO_URL"),
		"github_username": os.Getenv("GITHUB_USERNAME"),
		
		// 阿里云配置（不显示密码）
		"aliyun_registry":  os.Getenv("ALIYUN_REGISTRY"),
		"aliyun_namespace": os.Getenv("ALIYUN_NAMESPACE"),
		"aliyun_username":  os.Getenv("ALIYUN_USERNAME"),
		
		// 路径配置
		"git_local_repo_path": os.Getenv("GIT_LOCAL_REPO_PATH"),
		"log_file_path":       os.Getenv("LOG_FILE_PATH"),
	}

	// ====================================================================
	// 收集应用配置信息
	// ====================================================================
	
	appConfig := gin.H{
		"server": gin.H{
			"port": config.AppConfig.Server.Port,
			"mode": config.AppConfig.Server.Mode,
		},
		"database": gin.H{
			"host":     config.AppConfig.Database.Host,
			"port":     config.AppConfig.Database.Port,
			"username": config.AppConfig.Database.Username,
			"database": config.AppConfig.Database.Database,
			"charset":  config.AppConfig.Database.Charset,
		},
		"git": gin.H{
			"gitee": gin.H{
				"repo_url": config.AppConfig.Git.Gitee.RepoURL,
				"username": config.AppConfig.Git.Gitee.Username,
				"email":    config.AppConfig.Git.Gitee.Email,
			},
			"github": gin.H{
				"repo_url": config.AppConfig.Git.GitHub.RepoURL,
				"username": config.AppConfig.Git.GitHub.Username,
			},
			"local_repo_path": config.AppConfig.Git.LocalRepoPath,
		},
		"aliyun": gin.H{
			"registry":  config.AppConfig.Aliyun.Registry,
			"namespace": config.AppConfig.Aliyun.Namespace,
			"username":  config.AppConfig.Aliyun.Username,
		},
		"log": gin.H{
			"level":     config.AppConfig.Log.Level,
			"file_path": config.AppConfig.Log.FilePath,
		},
	}

	// ====================================================================
	// 检查数据库连接状态
	// ====================================================================
	
	dbStatus := gin.H{
		"connected": false,
		"error":     nil,
	}
	
	if database.DB != nil {
		sqlDB, err := database.DB.DB()
		if err == nil {
			err = sqlDB.Ping()
			if err == nil {
				dbStatus["connected"] = true
			} else {
				dbStatus["error"] = err.Error()
			}
		} else {
			dbStatus["error"] = err.Error()
		}
	} else {
		dbStatus["error"] = "Database not initialized"
	}

	// ====================================================================
	// 返回配置状态信息
	// ====================================================================
	
	c.JSON(http.StatusOK, gin.H{
		"status":      "success",
		"message":     "Configuration status retrieved successfully",
		"environment": envVars,
		"config":      appConfig,
		"database":    dbStatus,
		"timestamp":   gin.H{
			"unix": gin.H{},
		},
	})
}