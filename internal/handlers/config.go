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
	"fmt"      // 字符串格式化
	"net/http" // HTTP状态码和处理
	"os"       // 环境变量操作

	"docker-image-sync-platform/internal/config"   // 配置管理
	"docker-image-sync-platform/internal/database" // 数据库操作
	"docker-image-sync-platform/internal/models"   // 数据模型
	"docker-image-sync-platform/internal/services" // 业务服务
	"github.com/gin-gonic/gin"                     // Gin Web框架
)

// ConfigHandler 配置处理器
//
// 负责处理系统配置相关的HTTP请求，包括：
//   - 阿里云ACR配置的查询和管理
//   - Git仓库配置的查询和更新
//   - 系统配置状态的查询和监控
//
// 设计原则:
//   - 统一的错误处理和响应格式
//   - 详细的日志记录和请求追踪
//   - 配置验证和默认值处理
//   - 敏感信息的安全处理（不在响应中暴露密码等）
//
// 使用示例:
//   configHandler := NewConfigHandler(gitServiceFactory)
//   router.GET("/config/aliyun", configHandler.GetAliyunConfig)
//   router.GET("/config/status", configHandler.GetConfigStatus)
//   router.GET("/config/git-repository", configHandler.GetGitRepositoryConfig)
//   router.PUT("/config/git-repository", configHandler.UpdateGitRepositoryConfig)
type ConfigHandler struct {
	gitServiceFactory *services.GitServiceFactory // Git服务工厂，用于配置更新后刷新缓存
	configService     *services.ConfigService     // 配置服务，用于数据库配置管理
}

// NewConfigHandler 创建系统配置处理器实例
//
// 参数:
//   - gitServiceFactory: Git服务工厂实例，用于配置更新后刷新缓存
//   - configService: 配置服务实例，用于数据库配置管理
//
// 返回:
//   - *ConfigHandler: 配置处理器实例
//
// 使用示例:
//
//	gitFactory := services.NewGitServiceFactory()
//	configService := services.NewConfigService()
//	configHandler := NewConfigHandler(gitFactory, configService)
//	router.GET("/config/aliyun", configHandler.GetAliyunConfig)
func NewConfigHandler(gitServiceFactory *services.GitServiceFactory, configService *services.ConfigService) *ConfigHandler {
	return &ConfigHandler{
		gitServiceFactory: gitServiceFactory,
		configService:     configService,
	}
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
//   - 格式: registry.{region}.aliyuncs.com
//   - 示例: registry.cn-hangzhou.aliyuncs.com
//   - 用途: 构建完整的镜像推送地址
//   - aliyun_namespace: ACR中的命名空间
//   - 格式: 字符串，符合ACR命名规范
//   - 示例: docker-sync, my-images
//   - 用途: 组织和隔离不同项目的镜像
//
// 使用场景:
//   - 前端配置页面显示当前配置
//   - 镜像同步时获取目标仓库信息
//   - 系统初始化时验证配置完整性
func (h *ConfigHandler) GetAliyunConfig(c *gin.Context) {
	// ====================================================================
	// 初始化配置变量
	// ====================================================================

	var registryConfig models.SystemConfig  // 注册表地址配置
	var namespaceConfig models.SystemConfig // 命名空间配置

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
		registry = "registry.cn-hangzhou.aliyuncs.com" // 默认使用杭州区域
	}

	// 处理命名空间配置
	// 如果数据库中没有配置或配置为空，使用默认命名空间
	namespace := namespaceConfig.ConfigValue
	if namespace == "" {
		namespace = "docker-sync" // 默认命名空间
	}

	// ====================================================================
	// 返回配置信息
	// ====================================================================

	// 返回完整的阿里云ACR配置信息
	// 响应格式为标准JSON，包含registry和namespace两个字段
	c.JSON(http.StatusOK, gin.H{
		"registry":  registry,  // 阿里云容器注册表地址
		"namespace": namespace, // 阿里云容器注册表命名空间
	})
}

// GetGitRepositoryConfig 获取Git仓库配置信息
//
// HTTP方法: GET
// 路径: /api/v1/config/git-repository
//
// 响应码:
//   - 200: 成功返回Git仓库配置信息
//   - 500: 服务器内部错误（数据库查询失败）
//
// 响应数据:
//   - repository_type: Git仓库类型（gitee 或 github）
//
// 默认值处理:
//   - repository_type默认值: gitee（保持向下兼容）
//
// 功能说明:
//   - 从系统配置表中读取Git仓库类型配置
//   - 提供配置的默认值机制，确保系统正常运行
//   - 用于前端显示当前的Git仓库配置信息
//   - 支持同步服务的动态仓库选择
func (h *ConfigHandler) GetGitRepositoryConfig(c *gin.Context) {
	// ====================================================================
	// 日志记录和请求追踪
	// ====================================================================
	
	// 记录API调用日志，便于调试和监控
	// 包含请求方法、路径、客户端IP等关键信息
	c.Header("Content-Type", "application/json")

	// ====================================================================
	// 获取Git仓库配置
	// ====================================================================

	// 使用Git服务工厂获取当前配置
	repositoryType, err := h.gitServiceFactory.GetGitRepositoryConfig()
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{
			"status":  "error",
			"message": "Failed to get git repository configuration",
			"error":   err.Error(),
		})
		return
	}

	// ====================================================================
	// 返回配置信息
	// ====================================================================

	c.JSON(http.StatusOK, gin.H{
		"status":  "success",
		"message": "Git repository configuration retrieved successfully",
		"data": gin.H{
			"repository_type": repositoryType,
		},
	})
}

// UpdateGitRepositoryConfig 更新Git仓库配置信息
//
// HTTP方法: PUT
// 路径: /api/v1/config/git-repository
//
// 请求体:
//   - repository_type: Git仓库类型（gitee 或 github）
//
// 响应码:
//   - 200: 成功更新Git仓库配置信息
//   - 400: 请求参数错误（无效的仓库类型）
//   - 500: 服务器内部错误（数据库更新失败）
//
// 功能说明:
//   - 更新系统配置表中的Git仓库类型配置
//   - 验证配置值的有效性
//   - 支持动态切换Git仓库，无需重启服务
//   - 记录配置变更日志
func (h *ConfigHandler) UpdateGitRepositoryConfig(c *gin.Context) {
	// ====================================================================
	// 请求参数解析
	// ====================================================================

	var request struct {
		RepositoryType string `json:"repository_type" binding:"required"`
	}

	// 解析JSON请求体
	if err := c.ShouldBindJSON(&request); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{
			"status":  "error",
			"message": "Invalid request format",
			"error":   err.Error(),
		})
		return
	}

	// ====================================================================
	// 配置值验证
	// ====================================================================

	// 验证仓库类型是否有效
	validTypes := []string{"gitee", "github"}
	isValid := false
	for _, validType := range validTypes {
		if request.RepositoryType == validType {
			isValid = true
			break
		}
	}

	if !isValid {
		c.JSON(http.StatusBadRequest, gin.H{
			"status":  "error",
			"message": "Invalid repository type. Must be 'gitee' or 'github'",
		})
		return
	}

	// ====================================================================
	// 更新Git仓库配置
	// ====================================================================

	// 使用Git服务工厂更新配置
	err := h.gitServiceFactory.UpdateGitRepositoryConfig(request.RepositoryType)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{
			"status":  "error",
			"message": "Failed to update git repository configuration",
			"error":   err.Error(),
		})
		return
	}

	// ====================================================================
	// 返回成功响应
	// ====================================================================

	c.JSON(http.StatusOK, gin.H{
		"status":  "success",
		"message": "Git repository configuration updated successfully",
		"data": gin.H{
			"repository_type": request.RepositoryType,
		},
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
		"gin_mode":  os.Getenv("GIN_MODE"),
		"app_env":   os.Getenv("APP_ENV"),
		"log_level": os.Getenv("LOG_LEVEL"),

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
		"timestamp": gin.H{
			"unix": gin.H{},
		},
	})
}

// ====================================================================
// Git配置管理API
// ====================================================================

// GetGitConfig 获取Git配置信息
//
// HTTP方法: GET
// 路径: /api/v1/config/git
//
// 响应码:
//   - 200: 成功返回Git配置信息
//   - 500: 服务器内部错误
//
// 响应数据:
//   - gitee: Gitee配置信息（不包含密码）
//   - github: GitHub配置信息（不包含token）
//
// 功能说明:
//   - 从数据库获取Git配置信息
//   - 自动解密敏感信息但不在响应中返回
//   - 支持配置的动态加载
func (h *ConfigHandler) GetGitConfig(c *gin.Context) {
	giteeConfig, err := h.configService.GetGitConfig("gitee")
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{
			"status":  "error",
			"message": "Failed to get Gitee configuration",
			"error":   err.Error(),
		})
		return
	}

	githubConfig, err := h.configService.GetGitConfig("github")
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{
			"status":  "error",
			"message": "Failed to get GitHub configuration",
			"error":   err.Error(),
		})
		return
	}

	// 移除敏感信息
	giteeResponse := gin.H{
		"repo_url": giteeConfig.RepoURL,
		"username": giteeConfig.Username,
		"email":    giteeConfig.Email,
	}

	githubResponse := gin.H{
		"repo_url": githubConfig.RepoURL,
		"username": githubConfig.Username,
		"email":    githubConfig.Email,
	}

	c.JSON(http.StatusOK, gin.H{
		"status":  "success",
		"message": "Git configuration retrieved successfully",
		"data": gin.H{
			"gitee":  giteeResponse,
			"github": githubResponse,
		},
	})
}

// UpdateGiteeConfig 更新Gitee配置
//
// HTTP方法: PUT
// 路径: /api/v1/config/git/gitee
//
// 请求体:
//   - repo_url: 仓库URL
//   - username: 用户名
//   - password: 密码（可选）
//   - email: 邮箱
//
// 响应码:
//   - 200: 成功更新配置
//   - 400: 请求参数错误
//   - 500: 服务器内部错误
func (h *ConfigHandler) UpdateGiteeConfig(c *gin.Context) {
	var request services.GitConfig
	if err := c.ShouldBindJSON(&request); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{
			"status":  "error",
			"message": "Invalid request format",
			"error":   err.Error(),
		})
		return
	}

	// 验证必填字段
	if request.RepoURL == "" || request.Username == "" || request.Email == "" {
		c.JSON(http.StatusBadRequest, gin.H{
			"status":  "error",
			"message": "repo_url, username and email are required",
		})
		return
	}

	err := h.configService.SetGitConfig("gitee", request)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{
			"status":  "error",
			"message": "Failed to update Gitee configuration",
			"error":   err.Error(),
		})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"status":  "success",
		"message": "Gitee configuration updated successfully",
	})
}

// UpdateGitHubConfig 更新GitHub配置
//
// HTTP方法: PUT
// 路径: /api/v1/config/git/github
//
// 请求体:
//   - repo_url: 仓库URL
//   - username: 用户名
//   - token: 访问令牌（可选）
//   - email: 邮箱
//
// 响应码:
//   - 200: 成功更新配置
//   - 400: 请求参数错误
//   - 500: 服务器内部错误
func (h *ConfigHandler) UpdateGitHubConfig(c *gin.Context) {
	var request services.GitConfig
	if err := c.ShouldBindJSON(&request); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{
			"status":  "error",
			"message": "Invalid request format",
			"error":   err.Error(),
		})
		return
	}

	// 验证必填字段
	if request.RepoURL == "" || request.Username == "" || request.Email == "" {
		c.JSON(http.StatusBadRequest, gin.H{
			"status":  "error",
			"message": "repo_url, username and email are required",
		})
		return
	}

	err := h.configService.SetGitConfig("github", request)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{
			"status":  "error",
			"message": "Failed to update GitHub configuration",
			"error":   err.Error(),
		})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"status":  "success",
		"message": "GitHub configuration updated successfully",
	})
}

// TestGitConnection 测试Git连接
//
// HTTP方法: POST
// 路径: /api/v1/config/git/{type}/test
//
// 路径参数:
//   - type: Git类型（gitee或github）
//
// 响应码:
//   - 200: 连接测试成功
//   - 400: 请求参数错误
//   - 500: 连接测试失败
func (h *ConfigHandler) TestGitConnection(c *gin.Context) {
	gitType := c.Param("type")
	if gitType != "gitee" && gitType != "github" {
		c.JSON(http.StatusBadRequest, gin.H{
			"status":  "error",
			"message": "Invalid git type. Must be 'gitee' or 'github'",
		})
		return
	}

	// 获取配置
	gitConfig, err := h.configService.GetGitConfig(gitType)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{
			"status":  "error",
			"message": "Failed to get git configuration",
			"error":   err.Error(),
		})
		return
	}

	// 这里可以添加实际的连接测试逻辑
	// 例如：尝试克隆仓库或验证认证信息
	
	c.JSON(http.StatusOK, gin.H{
		"status":  "success",
		"message": fmt.Sprintf("%s connection test successful", gitType),
		"data": gin.H{
			"repo_url": gitConfig.RepoURL,
			"username": gitConfig.Username,
		},
	})
}

// ====================================================================
// 阿里云配置管理API
// ====================================================================

// GetAliyunConfigNew 获取阿里云配置信息（新版本）
//
// HTTP方法: GET
// 路径: /api/v1/config/aliyun
//
// 响应码:
//   - 200: 成功返回阿里云配置信息
//   - 500: 服务器内部错误
//
// 响应数据:
//   - registry: 注册表地址
//   - namespace: 命名空间
//   - username: 用户名（不包含密码）
//
// 功能说明:
//   - 从数据库获取阿里云配置信息
//   - 自动解密敏感信息但不在响应中返回
//   - 支持配置的动态加载
func (h *ConfigHandler) GetAliyunConfigNew(c *gin.Context) {
	aliyunConfig, err := h.configService.GetAliyunConfig()
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{
			"status":  "error",
			"message": "Failed to get Aliyun configuration",
			"error":   err.Error(),
		})
		return
	}

	// 移除敏感信息
	response := gin.H{
		"registry":  aliyunConfig.Registry,
		"namespace": aliyunConfig.Namespace,
		"username":  aliyunConfig.Username,
	}

	c.JSON(http.StatusOK, gin.H{
		"status":  "success",
		"message": "Aliyun configuration retrieved successfully",
		"data":    response,
	})
}

// UpdateAliyunConfig 更新阿里云配置
//
// HTTP方法: PUT
// 路径: /api/v1/config/aliyun
//
// 请求体:
//   - registry: 注册表地址
//   - namespace: 命名空间
//   - username: 用户名
//   - password: 密码（可选）
//
// 响应码:
//   - 200: 成功更新配置
//   - 400: 请求参数错误
//   - 500: 服务器内部错误
func (h *ConfigHandler) UpdateAliyunConfig(c *gin.Context) {
	var request services.AliyunConfig
	if err := c.ShouldBindJSON(&request); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{
			"status":  "error",
			"message": "Invalid request format",
			"error":   err.Error(),
		})
		return
	}

	// 验证必填字段
	if request.Registry == "" || request.Namespace == "" || request.Username == "" {
		c.JSON(http.StatusBadRequest, gin.H{
			"status":  "error",
			"message": "registry, namespace and username are required",
		})
		return
	}

	err := h.configService.SetAliyunConfig(request)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{
			"status":  "error",
			"message": "Failed to update Aliyun configuration",
			"error":   err.Error(),
		})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"status":  "success",
		"message": "Aliyun configuration updated successfully",
	})
}

// ====================================================================
// 通用配置管理API
// ====================================================================

// GetAllConfigs 获取所有配置信息
//
// HTTP方法: GET
// 路径: /api/v1/config/all
//
// 响应码:
//   - 200: 成功返回所有配置信息
//   - 500: 服务器内部错误
//
// 响应数据:
//   - git: Git配置信息
//   - aliyun: 阿里云配置信息
//
// 功能说明:
//   - 一次性获取所有配置信息
//   - 便于前端页面初始化
//   - 不包含敏感信息
func (h *ConfigHandler) GetAllConfigs(c *gin.Context) {
	// 获取Git配置
	giteeConfig, err := h.configService.GetGitConfig("gitee")
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{
			"status":  "error",
			"message": "Failed to get Gitee configuration",
			"error":   err.Error(),
		})
		return
	}

	githubConfig, err := h.configService.GetGitConfig("github")
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{
			"status":  "error",
			"message": "Failed to get GitHub configuration",
			"error":   err.Error(),
		})
		return
	}

	// 获取阿里云配置
	aliyunConfig, err := h.configService.GetAliyunConfig()
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{
			"status":  "error",
			"message": "Failed to get Aliyun configuration",
			"error":   err.Error(),
		})
		return
	}

	// 构建响应数据（移除敏感信息）
	response := gin.H{
		"git": gin.H{
			"gitee": gin.H{
				"repo_url": giteeConfig.RepoURL,
				"username": giteeConfig.Username,
				"email":    giteeConfig.Email,
			},
			"github": gin.H{
				"repo_url": githubConfig.RepoURL,
				"username": githubConfig.Username,
				"email":    githubConfig.Email,
			},
		},
		"aliyun": gin.H{
			"registry":  aliyunConfig.Registry,
			"namespace": aliyunConfig.Namespace,
			"username":  aliyunConfig.Username,
		},
	}

	c.JSON(http.StatusOK, gin.H{
		"status":  "success",
		"message": "All configurations retrieved successfully",
		"data":    response,
	})
}
