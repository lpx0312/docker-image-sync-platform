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

// - aliyun_namespace: 阿里云容器注册表命名空间
//
// 作者: Docker镜像同步平台开发团队
// 版本: v1.0.0
package handlers

import (
	"context"  // 上下文控制
	"encoding/base64" // Base64编码
	"encoding/json"   // JSON处理
	"fmt"      // 字符串格式化
	"io"       // IO操作
	"net/http" // HTTP状态码和处理
	"net/url" // URL解析
	"os"       // 环境变量操作
	"strings"  // 字符串操作
	"time"     // 时间操作

	"docker-image-sync-platform/internal/config"   // 配置管理
	"docker-image-sync-platform/internal/database" // 数据库操作
	"docker-image-sync-platform/internal/logger"   // 日志管理
	"docker-image-sync-platform/internal/services" // 业务服务
	"github.com/gin-gonic/gin"                     // Gin Web框架
	"github.com/go-git/go-git/v5"                  // Git操作
	gitconfig "github.com/go-git/go-git/v5/config" // Git配置
	githttp "github.com/go-git/go-git/v5/plumbing/transport/http" // Git HTTP认证
	"github.com/go-git/go-git/v5/storage/memory"   // Git内存存储
	"go.uber.org/zap"                              // Zap日志库
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

// 旧版GetAliyunConfig函数已删除，统一使用GetAliyunConfigNew

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
		"log_file_path":       os.Getenv("LOG_FILE_PATH"),
	}

	// ====================================================================
	// 收集应用配置信息（从数据库获取）
	// ====================================================================

	// 从数据库获取配置
	getConfigValue := func(key, defaultValue string) string {
		if value, err := h.configService.GetConfig(key); err == nil {
			return value
		}
		return defaultValue
	}

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
				"repo_url": getConfigValue("gitee_repo_url", ""),
				"username": getConfigValue("gitee_username", ""),
				"email":    getConfigValue("gitee_email", ""),
			},
			"github": gin.H{
				"repo_url": getConfigValue("github_repo_url", ""),
				"username": getConfigValue("github_username", ""),
			},

		},
		"aliyun": gin.H{
			"registry":  getConfigValue("aliyun_registry", "registry.cn-hangzhou.aliyuncs.com"),
			"namespace": getConfigValue("aliyun_namespace", "lipanxiang"),
			"username":  getConfigValue("aliyun_username", ""),
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
			"unix": time.Now().Unix(),
		},
	})
}

// DebugGetConfig 调试配置获取
func (h *ConfigHandler) DebugGetConfig(c *gin.Context) {
	key := c.Param("key")
	if key == "" {
		c.JSON(http.StatusBadRequest, gin.H{
			"status":  "error",
			"message": "key parameter is required",
		})
		return
	}

	value, err := h.configService.GetConfig(key)
	c.JSON(http.StatusOK, gin.H{
		"status": "success",
		"key":    key,
		"value":  value,
		"error":  func() string {
			if err != nil {
				return err.Error()
			}
			return ""
		}(),
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
		"repo_url":   giteeConfig.RepoURL,
		"username":   giteeConfig.Username,
		"email":      giteeConfig.Email,
		"branch":     giteeConfig.Branch,
		"local_path": giteeConfig.LocalPath,
	}

	githubResponse := gin.H{
		"repo_url":   githubConfig.RepoURL,
		"username":   githubConfig.Username,
		"email":      githubConfig.Email,
		"branch":     githubConfig.Branch,
		"local_path": githubConfig.LocalPath,
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
	var request struct {
		Type     string `json:"type" binding:"required"`
		RepoURL  string `json:"repo_url" binding:"required"`
		Username string `json:"username" binding:"required"`
		Password string `json:"password"`
		Token    string `json:"token"`
		Email    string `json:"email"`
	}
	
	if err := c.ShouldBindJSON(&request); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{
			"status":  "error",
			"message": "Invalid request format",
			"error":   err.Error(),
		})
		return
	}
	
	gitType := request.Type
	if gitType != "gitee" && gitType != "github" {
		c.JSON(http.StatusBadRequest, gin.H{
			"status":  "error",
			"message": "Invalid git type. Must be 'gitee' or 'github'",
		})
		return
	}

	// 验证必填字段
	if request.RepoURL == "" || request.Username == "" {
		c.JSON(http.StatusBadRequest, gin.H{
			"status":  "error",
			"message": "repo_url and username are required",
		})
		return
	}

	// 根据类型验证认证字段
	if gitType == "gitee" && request.Password == "" {
		c.JSON(http.StatusBadRequest, gin.H{
			"status":  "error",
			"message": "password is required for Gitee",
		})
		return
	}
	
	if gitType == "github" && request.Token == "" {
		c.JSON(http.StatusBadRequest, gin.H{
			"status":  "error",
			"message": "token is required for GitHub",
		})
		return
	}

	// 实际的连接测试逻辑
	err := h.testGitRepositoryConnection(request.RepoURL, request.Username, request.Password, request.Token, gitType)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{
			"status":  "error",
			"message": fmt.Sprintf("Git connection test failed: %s", err.Error()),
		})
		return
	}
	
	c.JSON(http.StatusOK, gin.H{
		"status":  "success",
		"message": fmt.Sprintf("%s connection test successful", gitType),
		"data": gin.H{
			"repo_url": request.RepoURL,
			"username": request.Username,
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
func (h *ConfigHandler) GetAliyunConfig(c *gin.Context) {
	aliyunConfig, err := h.configService.GetAliyunConfig()
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{
			"status":  "error",
			"message": "Failed to get Aliyun configuration",
			"error":   err.Error(),
		})
		return
	}

	// 处理敏感信息 - 密码用占位符替代
	password := ""
	if aliyunConfig.Password != "" {
		password = "***" // 如果有密码，显示占位符
	}
	
	response := gin.H{
		"registry_url": aliyunConfig.Registry,
		"namespace":    aliyunConfig.Namespace,
		"username":     aliyunConfig.Username,
		"password":     password,
		"region":       aliyunConfig.Region,
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

// TestAliyunConnection 测试阿里云镜像仓库连接
//
// HTTP方法: POST
// 路径: /api/v1/config/aliyun/test
//
// 请求体:
//   - registry_url: 镜像仓库地址
//   - namespace: 命名空间
//   - username: 用户名
//   - password: 密码
//   - region: 地域
//
// 响应码:
//   - 200: 连接测试成功
//   - 400: 请求参数错误
//   - 500: 连接测试失败
//
// 功能说明:
//   - 验证阿里云镜像仓库的连接配置
//   - 检查认证信息是否正确
//   - 验证仓库访问权限
func (h *ConfigHandler) TestAliyunConnection(c *gin.Context) {
	var request struct {
		RegistryURL string `json:"registry_url" binding:"required"`
		Namespace   string `json:"namespace" binding:"required"`
		Username    string `json:"username" binding:"required"`
		Password    string `json:"password" binding:"required"`
		Region      string `json:"region"`
	}

	if err := c.ShouldBindJSON(&request); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{
			"status":  "error",
			"message": "Invalid request format",
			"error":   err.Error(),
		})
		return
	}

	// 测试阿里云镜像仓库连接
	err := h.testAliyunRegistryConnection(request.RegistryURL, request.Namespace, request.Username, request.Password)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{
			"status":  "error",
			"message": "阿里云镜像仓库连接失败",
			"error":   err.Error(),
		})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"status":  "success",
		"message": "阿里云镜像仓库连接测试成功，配置正确",
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
				"repo_url":   giteeConfig.RepoURL,
				"username":   giteeConfig.Username,
				"email":      giteeConfig.Email,
				"branch":     giteeConfig.Branch,
				"local_path": giteeConfig.LocalPath,
			},
			"github": gin.H{
				"repo_url":   githubConfig.RepoURL,
				"username":   githubConfig.Username,
				"email":      githubConfig.Email,
				"branch":     githubConfig.Branch,
				"local_path": githubConfig.LocalPath,
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

// testGitHubAPIConnection 测试GitHub API连接
//
// 使用GitHub API验证访问令牌和仓库访问权限
//
// 参数：
//   - repoURL: 仓库URL
//   - username: 用户名
//   - token: 访问令牌
//
// 返回：
//   - error: 连接错误，nil表示连接成功
func (h *ConfigHandler) testGitHubAPIConnection(repoURL, username, token string) error {
	// 从仓库URL中提取owner和repo名称
	// 例如：https://github.com/owner/repo.git -> owner/repo
	repoURL = strings.TrimSuffix(repoURL, ".git")
	parts := strings.Split(strings.TrimPrefix(repoURL, "https://github.com/"), "/")
	if len(parts) != 2 {
		return fmt.Errorf("无效的GitHub仓库URL格式")
	}
	owner, repo := parts[0], parts[1]

	// 构建GitHub API URL
	apiURL := fmt.Sprintf("https://api.github.com/repos/%s/%s", owner, repo)

	// 创建HTTP客户端，支持代理设置
	transport := &http.Transport{}
	
	// 检查是否设置了HTTP代理
	if proxyURL := os.Getenv("HTTP_PROXY"); proxyURL != "" {
		if proxy, err := url.Parse(proxyURL); err == nil {
			transport.Proxy = http.ProxyURL(proxy)
		}
	} else if proxyURL := os.Getenv("HTTPS_PROXY"); proxyURL != "" {
		if proxy, err := url.Parse(proxyURL); err == nil {
			transport.Proxy = http.ProxyURL(proxy)
		}
	}
	
	client := &http.Client{
		Timeout:   30 * time.Second,
		Transport: transport,
	}

	// 创建请求
	req, err := http.NewRequest("GET", apiURL, nil)
	if err != nil {
		return fmt.Errorf("创建GitHub API请求失败: %v", err)
	}

	// 添加认证头
	req.Header.Set("Authorization", "token "+token)
	req.Header.Set("User-Agent", "Docker-Image-Sync-Platform/1.0")
	req.Header.Set("Accept", "application/vnd.github.v3+json")

	// 发送请求
	resp, err := client.Do(req)
	if err != nil {
		// 检查是否是网络连接问题
		errStr := err.Error()
		if strings.Contains(errStr, "timeout") || 
		   strings.Contains(errStr, "connection refused") ||
		   strings.Contains(errStr, "no route to host") ||
		   strings.Contains(errStr, "network is unreachable") {
			return fmt.Errorf("GitHub网络连接失败: %v。可能的解决方案：\n1. 检查网络连接是否正常\n2. 确认防火墙是否阻止了GitHub访问\n3. 尝试设置HTTP代理：export HTTPS_PROXY=http://proxy:port\n4. 如果在中国大陆，可能需要使用VPN或代理服务\n5. 稍后重试，可能是临时网络问题", err)
		}
		return fmt.Errorf("GitHub API请求失败: %v", err)
	}
	defer resp.Body.Close()

	// 读取响应体
	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return fmt.Errorf("读取GitHub API响应失败: %v", err)
	}

	// 检查响应状态码
	switch resp.StatusCode {
	case http.StatusOK:
		// API调用成功，仓库存在且可访问
		return nil
	case http.StatusUnauthorized:
		return fmt.Errorf("GitHub访问令牌无效或已过期。请检查：\n1. 令牌是否正确复制\n2. 令牌是否已过期\n3. 令牌是否被撤销")
	case http.StatusForbidden:
		return fmt.Errorf("GitHub访问令牌权限不足。请确保：\n1. 令牌具有repo权限\n2. 对私有仓库有访问权限\n3. 令牌未被限制访问该仓库")
	case http.StatusNotFound:
		return fmt.Errorf("GitHub仓库不存在或无访问权限。请检查：\n1. 仓库URL是否正确\n2. 仓库是否存在\n3. 是否有访问该仓库的权限")
	default:
		// 尝试解析错误信息
		var errorResp map[string]interface{}
		if json.Unmarshal(body, &errorResp) == nil {
			if msg, ok := errorResp["message"].(string); ok {
				return fmt.Errorf("GitHub API错误: %s", msg)
			}
		}
		return fmt.Errorf("GitHub API请求失败，状态码: %d", resp.StatusCode)
	}
}

// testGitRepositoryConnection 测试Git仓库连接
//
// 使用go-git库尝试连接到指定的Git仓库，验证认证信息是否正确
//
// 参数：
//   - repoURL: 仓库URL
//   - username: 用户名
//   - password: 密码（用于Gitee）
//   - token: 访问令牌（用于GitHub）
//   - gitType: Git类型（gitee或github）
//
// 返回：
//   - error: 连接错误，nil表示连接成功
func (h *ConfigHandler) testGitRepositoryConnection(repoURL, username, password, token, gitType string) error {
	// 对于GitHub，先尝试使用API验证token和仓库访问权限
	if gitType == "github" {
		if err := h.testGitHubAPIConnection(repoURL, username, token); err != nil {
			return err
		}
	}
	// 创建内存存储，用于测试连接而不实际克隆仓库
	storage := memory.NewStorage()
	
	// 准备认证信息
	var auth *githttp.BasicAuth
	if gitType == "gitee" {
		auth = &githttp.BasicAuth{
			Username: username,
			Password: password,
		}
	} else if gitType == "github" {
		auth = &githttp.BasicAuth{
			Username: username,
			Password: token,
		}
	}
	
	// 尝试列出远程引用来测试连接
	remote := git.NewRemote(storage, &gitconfig.RemoteConfig{
		Name: "origin",
		URLs: []string{repoURL},
	})
	
	// 创建带超时的上下文 - 增加超时时间以适应网络延迟
	timeout := 120 * time.Second
	if gitType == "github" {
		// GitHub在某些网络环境下可能需要更长时间
		timeout = 180 * time.Second
	}
	ctx, cancel := context.WithTimeout(context.Background(), timeout)
	defer cancel()
	
	// 创建一个通道来接收结果
	resultChan := make(chan error, 1)
	
	// 在goroutine中执行Git操作
	go func() {
		defer func() {
			if r := recover(); r != nil {
				resultChan <- fmt.Errorf("Git操作发生异常: %v", r)
			}
		}()
		
		// 记录开始时间用于调试
		startTime := time.Now()
		
		_, err := remote.List(&git.ListOptions{
			Auth: auth,
		})
		
		// 记录操作耗时
		duration := time.Since(startTime)
		if err != nil {
			// 添加详细的错误信息
			err = fmt.Errorf("Git操作失败 (耗时: %v, 仓库: %s, 用户: %s): %v", 
				duration, repoURL, username, err)
		}
		
		select {
		case resultChan <- err:
		case <-ctx.Done():
			// 如果上下文已取消，不发送结果
		}
	}()
	
	// 等待结果或超时
	select {
	case err := <-resultChan:
		if err != nil {
			// 检查是否是超时相关的错误
			errStr := err.Error()
			if strings.Contains(errStr, "context deadline exceeded") || 
			   strings.Contains(errStr, "timeout") ||
			   strings.Contains(errStr, "connection timeout") ||
			   strings.Contains(errStr, "i/o timeout") {
				if gitType == "github" {
					return fmt.Errorf("GitHub连接超时：网络连接到GitHub可能受限。可能的解决方案：\n1. 检查网络连接是否正常\n2. 确认防火墙是否阻止了GitHub访问\n3. 尝试设置HTTP代理：export HTTPS_PROXY=http://proxy:port\n4. 如果在中国大陆，可能需要使用VPN或代理服务\n5. 稍后重试，可能是临时网络问题")
				} else {
					return fmt.Errorf("Gitee连接超时：无法连接到Gitee仓库。请检查网络连接或稍后重试")
				}
			}
			
			// 检查网络连接错误
			if strings.Contains(errStr, "unexpected EOF") ||
			   strings.Contains(errStr, "connection refused") ||
			   strings.Contains(errStr, "no route to host") ||
			   strings.Contains(errStr, "network is unreachable") {
				if gitType == "github" {
					return fmt.Errorf("GitHub网络连接失败: %s。可能的解决方案：\n1. 检查网络连接是否正常\n2. 确认防火墙是否阻止了GitHub访问\n3. 尝试设置HTTP代理：export HTTPS_PROXY=http://proxy:port\n4. 如果在中国大陆，可能需要使用VPN或代理服务\n5. 稍后重试，可能是临时网络问题", err.Error())
				} else {
					return fmt.Errorf("Gitee网络连接失败: %s。请检查网络连接或稍后重试", err.Error())
				}
			}
			
			// 检查认证相关错误
			if strings.Contains(errStr, "authentication required") ||
			   strings.Contains(errStr, "401") ||
			   strings.Contains(errStr, "403") ||
			   strings.Contains(errStr, "unauthorized") {
				if gitType == "github" {
					return fmt.Errorf("GitHub认证失败：访问令牌无效或权限不足。请检查：\n1. 令牌是否正确复制\n2. 令牌是否有repo权限\n3. 仓库是否存在且可访问\n4. 令牌是否已过期")
				} else {
					return fmt.Errorf("Gitee认证失败：用户名或密码错误。请检查：\n1. 用户名是否正确\n2. 密码是否正确\n3. 账户是否被锁定")
				}
			}
			
			// 检查仓库不存在的错误
			if strings.Contains(errStr, "repository not found") ||
			   strings.Contains(errStr, "404") ||
			   strings.Contains(errStr, "not found") {
				return fmt.Errorf("仓库不存在：请检查：\n1. 仓库URL是否正确\n2. 仓库是否存在\n3. 是否有访问该仓库的权限")
			}
			
			// 提供更详细的错误信息
			if gitType == "github" {
				return fmt.Errorf("GitHub连接失败: %s。请检查：\n1. 仓库URL格式是否正确\n2. 用户名和访问令牌是否有效\n3. 网络连接是否正常\n4. 仓库是否存在且可访问", err.Error())
			} else {
				return fmt.Errorf("Gitee连接失败: %s。请检查：\n1. 仓库URL格式是否正确\n2. 用户名和密码是否正确\n3. 网络连接是否正常\n4. 仓库是否存在且可访问", err.Error())
			}
		}
		return nil
	case <-ctx.Done():
		if gitType == "github" {
			return fmt.Errorf("GitHub连接超时：无法在60秒内连接到GitHub仓库。这可能是由于网络限制导致的，建议：1) 检查网络连接 2) 确认防火墙设置 3) 尝试使用代理或VPN 4) 稍后重试")
		} else {
			return fmt.Errorf("Gitee连接超时：无法在60秒内连接到Gitee仓库。请检查网络连接或稍后重试")
		}
	}
}

// testAliyunRegistryConnection 测试阿里云镜像仓库连接
//
// 通过调用阿里云容器镜像服务API来验证认证信息和仓库访问权限
//
// 参数：
//   - registryURL: 镜像仓库地址
//   - namespace: 命名空间
//   - username: 用户名
//   - password: 密码
//
// 返回：
//   - error: 连接错误，nil表示连接成功
func (h *ConfigHandler) testAliyunRegistryConnection(registryURL, namespace, username, password string) error {
	startTime := time.Now()
	
	// 记录开始测试
	fmt.Printf("[阿里云连接测试] 开始测试 - 仓库: %s, 用户: %s, 命名空间: %s\n", registryURL, username, namespace)
	
	// 构建Docker Registry API的认证URL
	// 阿里云容器镜像服务使用标准的Docker Registry v2 API
	authURL := fmt.Sprintf("https://%s/v2/", strings.TrimPrefix(registryURL, "https://"))
	fmt.Printf("[阿里云连接测试] 测试URL: %s\n", authURL)
	
	// 创建HTTP客户端，支持代理设置
	transport := &http.Transport{}
	
	// 检查是否设置了HTTP代理
	if proxyURL := os.Getenv("HTTP_PROXY"); proxyURL != "" {
		if proxy, err := url.Parse(proxyURL); err == nil {
			transport.Proxy = http.ProxyURL(proxy)
			fmt.Printf("[阿里云连接测试] 使用HTTP代理: %s\n", proxyURL)
		}
	} else if proxyURL := os.Getenv("HTTPS_PROXY"); proxyURL != "" {
		if proxy, err := url.Parse(proxyURL); err == nil {
			transport.Proxy = http.ProxyURL(proxy)
			fmt.Printf("[阿里云连接测试] 使用HTTPS代理: %s\n", proxyURL)
		}
	}
	
	client := &http.Client{
		Timeout:   30 * time.Second, // 增加超时时间
		Transport: transport,
	}
	
	// 创建请求
	req, err := http.NewRequest("GET", authURL, nil)
	if err != nil {
		return fmt.Errorf("创建请求失败: %v", err)
	}
	
	// 添加Basic认证头
	auth := base64.StdEncoding.EncodeToString([]byte(username + ":" + password))
	req.Header.Set("Authorization", "Basic "+auth)
	req.Header.Set("User-Agent", "Docker-Image-Sync-Platform/1.0")
	
	// 发送请求
	fmt.Printf("[阿里云连接测试] 发送请求...\n")
	resp, err := client.Do(req)
	if err != nil {
		elapsed := time.Since(startTime)
		fmt.Printf("[阿里云连接测试] 请求失败 - 耗时: %v, 错误: %v\n", elapsed, err)
		return fmt.Errorf("连接阿里云镜像仓库失败: %v", err)
	}
	defer resp.Body.Close()
	
	elapsed := time.Since(startTime)
	fmt.Printf("[阿里云连接测试] 收到响应 - 状态码: %d, 耗时: %v\n", resp.StatusCode, elapsed)
	
	// 读取响应体
	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return fmt.Errorf("读取响应失败: %v", err)
	}
	
	// 记录响应头信息
	fmt.Printf("[阿里云连接测试] 响应头: %v\n", resp.Header)
	if len(body) > 0 && len(body) < 1000 {
		fmt.Printf("[阿里云连接测试] 响应体: %s\n", string(body))
	}
	
	// 检查响应状态码
	switch resp.StatusCode {
	case http.StatusOK:
		// 连接成功
		fmt.Printf("[阿里云连接测试] 连接成功！\n")
		return nil
	case http.StatusUnauthorized:
		// 认证失败 - 这是阿里云的正常行为，需要Bearer token认证
		fmt.Printf("[阿里云连接测试] 收到401状态码，这是正常的，表示需要Bearer token认证\n")
		
		// 尝试获取Bearer token
		return h.testAliyunBearerAuth(registryURL, namespace, username, password)
	case http.StatusForbidden:
		// 权限不足
		return fmt.Errorf("权限不足：用户没有访问该镜像仓库的权限")
	case http.StatusNotFound:
		// 仓库不存在
		return fmt.Errorf("镜像仓库不存在或地址错误")
	default:
		// 其他错误
		var errorMsg string
		if len(body) > 0 {
			// 尝试解析错误响应
			var errorResp map[string]interface{}
			if json.Unmarshal(body, &errorResp) == nil {
				if msg, ok := errorResp["message"].(string); ok {
					errorMsg = msg
				} else if errors, ok := errorResp["errors"].([]interface{}); ok && len(errors) > 0 {
					if errorMap, ok := errors[0].(map[string]interface{}); ok {
						if msg, ok := errorMap["message"].(string); ok {
							errorMsg = msg
						}
					}
				}
			}
		}
		
		if errorMsg == "" {
			errorMsg = fmt.Sprintf("HTTP %d", resp.StatusCode)
		}
		
		return fmt.Errorf("连接失败: %s", errorMsg)
	}
}

// testAliyunBearerAuth 测试阿里云Bearer认证
//
// 阿里云容器镜像服务使用Bearer token认证机制
// 首先需要通过用户名密码获取Bearer token，然后使用token访问API
//
// 参数：
//   - registryURL: 镜像仓库地址
//   - namespace: 命名空间
//   - username: 用户名
//   - password: 密码
//
// 返回：
//   - error: 认证错误，nil表示认证成功
func (h *ConfigHandler) testAliyunBearerAuth(registryURL, namespace, username, password string) error {
	fmt.Printf("[阿里云Bearer认证] 开始Bearer token认证流程\n")
	
	// 构建认证URL
	// 从WWW-Authenticate头中解析realm和service
	authURL := fmt.Sprintf("https://%s/v2/", strings.TrimPrefix(registryURL, "https://"))
	
	// 创建HTTP客户端
	transport := &http.Transport{}
	
	// 检查是否设置了HTTP代理
	if proxyURL := os.Getenv("HTTP_PROXY"); proxyURL != "" {
		if proxy, err := url.Parse(proxyURL); err == nil {
			transport.Proxy = http.ProxyURL(proxy)
		}
	} else if proxyURL := os.Getenv("HTTPS_PROXY"); proxyURL != "" {
		if proxy, err := url.Parse(proxyURL); err == nil {
			transport.Proxy = http.ProxyURL(proxy)
		}
	}
	
	client := &http.Client{
		Timeout:   30 * time.Second,
		Transport: transport,
	}
	
	// 第一步：获取认证信息
	req, err := http.NewRequest("GET", authURL, nil)
	if err != nil {
		return fmt.Errorf("创建认证请求失败: %v", err)
	}
	
	resp, err := client.Do(req)
	if err != nil {
		return fmt.Errorf("获取认证信息失败: %v", err)
	}
	defer resp.Body.Close()
	
	// 解析WWW-Authenticate头
	authHeader := resp.Header.Get("WWW-Authenticate")
	fmt.Printf("[阿里云Bearer认证] WWW-Authenticate: %s\n", authHeader)
	
	if authHeader == "" {
		return fmt.Errorf("未找到WWW-Authenticate头")
	}
	
	// 解析realm和service
	var realm, service string
	
	// 检查是否是Bearer认证
	if !strings.HasPrefix(authHeader, "Bearer ") {
		return fmt.Errorf("不支持的认证类型: %s", authHeader)
	}
	
	// 去掉"Bearer "前缀
	authParams := strings.TrimPrefix(authHeader, "Bearer ")
	fmt.Printf("[阿里云Bearer认证] 认证参数: %s\n", authParams)
	
	// 解析参数
	parts := strings.Split(authParams, ",")
	for _, part := range parts {
		part = strings.TrimSpace(part)
		if strings.HasPrefix(part, "realm=") {
			realm = strings.Trim(strings.TrimPrefix(part, "realm="), "\"")
		} else if strings.HasPrefix(part, "service=") {
			service = strings.Trim(strings.TrimPrefix(part, "service="), "\"")
		}
	}
	
	if realm == "" {
		return fmt.Errorf("未找到认证realm")
	}
	
	fmt.Printf("[阿里云Bearer认证] Realm: %s, Service: %s\n", realm, service)
	
	// 第二步：使用用户名密码获取Bearer token
	tokenURL := realm
	if service != "" {
		tokenURL += "?service=" + service
	}
	
	tokenReq, err := http.NewRequest("GET", tokenURL, nil)
	if err != nil {
		return fmt.Errorf("创建token请求失败: %v", err)
	}
	
	// 添加Basic认证
	auth := base64.StdEncoding.EncodeToString([]byte(username + ":" + password))
	tokenReq.Header.Set("Authorization", "Basic "+auth)
	
	tokenResp, err := client.Do(tokenReq)
	if err != nil {
		return fmt.Errorf("获取Bearer token失败: %v", err)
	}
	defer tokenResp.Body.Close()
	
	tokenBody, err := io.ReadAll(tokenResp.Body)
	if err != nil {
		return fmt.Errorf("读取token响应失败: %v", err)
	}
	
	fmt.Printf("[阿里云Bearer认证] Token响应状态码: %d\n", tokenResp.StatusCode)
	
	if tokenResp.StatusCode == http.StatusUnauthorized {
		return fmt.Errorf("认证失败：用户名或密码错误")
	}
	
	if tokenResp.StatusCode != http.StatusOK {
		return fmt.Errorf("获取Bearer token失败，状态码: %d, 响应: %s", tokenResp.StatusCode, string(tokenBody))
	}
	
	// 解析token响应
	var tokenData map[string]interface{}
	if err := json.Unmarshal(tokenBody, &tokenData); err != nil {
		return fmt.Errorf("解析token响应失败: %v", err)
	}
	
	token, ok := tokenData["token"].(string)
	if !ok {
		return fmt.Errorf("未找到Bearer token")
	}
	
	fmt.Printf("[阿里云Bearer认证] 成功获取Bearer token\n")
	
	// 第三步：使用Bearer token访问API
	apiReq, err := http.NewRequest("GET", authURL, nil)
	if err != nil {
		return fmt.Errorf("创建API请求失败: %v", err)
	}
	
	apiReq.Header.Set("Authorization", "Bearer "+token)
	
	apiResp, err := client.Do(apiReq)
	if err != nil {
		return fmt.Errorf("使用Bearer token访问API失败: %v", err)
	}
	defer apiResp.Body.Close()
	
	fmt.Printf("[阿里云Bearer认证] API访问状态码: %d\n", apiResp.StatusCode)
	
	if apiResp.StatusCode == http.StatusOK {
		fmt.Printf("[阿里云Bearer认证] 认证成功！\n")
		return nil
	}
	
	apiBody, _ := io.ReadAll(apiResp.Body)
	return fmt.Errorf("Bearer token认证失败，状态码: %d, 响应: %s", apiResp.StatusCode, string(apiBody))
}

// GetGitOptimizationConfig 获取Git优化配置
//
// HTTP方法: GET
// 路径: /api/v1/config/git-optimization
//
// 响应:
//   - 200: 返回Git优化配置信息
//   - 500: 服务器内部错误
//
// 响应数据包含:
//   - use_optimized: 是否使用优化服务
//   - available_modes: 可用的操作模式列表
//   - current_mode: 当前操作模式
//   - performance_metrics: 性能指标统计
func (h *ConfigHandler) GetGitOptimizationConfig(c *gin.Context) {
	// 检查是否使用优化服务
	useOptimized := h.gitServiceFactory.IsUsingOptimized()

	// 获取性能指标
	var metrics map[string]interface{}
	if optimizedService, err := h.gitServiceFactory.GetOptimizedGitService(); err == nil {
		metrics = optimizedService.GetPerformanceMetrics()
	}

	// 构建响应数据
	response := gin.H{
		"use_optimized":    useOptimized,
		"available_modes":  []string{"sparse", "full", "auto"},
		"current_mode":     "auto", // 可以从配置获取
		"performance_metrics": metrics,
		"optimization_enabled": true, // 系统是否支持优化
	}

	c.JSON(http.StatusOK, gin.H{
		"success": true,
		"data":    response,
		"message": "Git优化配置获取成功",
	})
}

// UpdateGitOptimizationConfig 更新Git优化配置
//
// HTTP方法: PUT
// 路径: /api/v1/config/git-optimization
//
// 请求体:
//   - use_optimized: 是否使用优化服务
//
// 响应:
//   - 200: 配置更新成功
//   - 400: 请求参数错误
//   - 500: 服务器内部错误
func (h *ConfigHandler) UpdateGitOptimizationConfig(c *gin.Context) {
	var req struct {
		UseOptimized bool `json:"use_optimized" binding:"required"`
	}

	if err := c.ShouldBindJSON(&req); err != nil {
		logger.Logger.Error("解析Git优化配置请求参数失败", zap.Error(err))
		c.JSON(http.StatusBadRequest, gin.H{
			"success": false,
			"error":   "请求参数格式错误",
		})
		return
	}

	// 更新配置
	h.gitServiceFactory.SetUseOptimized(req.UseOptimized)

	logger.Logger.Info("Git优化配置已更新",
		zap.Bool("use_optimized", req.UseOptimized))

	c.JSON(http.StatusOK, gin.H{
		"success": true,
		"data": gin.H{
			"use_optimized": req.UseOptimized,
		},
		"message": "Git优化配置更新成功",
	})
}

// GetGitPerformanceMetrics 获取Git性能指标
//
// HTTP方法: GET
// 路径: /api/v1/config/git-performance
//
// 响应:
//   - 200: 返回性能指标数据
//   - 500: 服务器内部错误
//
// 响应数据包含:
//   - sparse_operations: 稀疏检出操作次数
//   - full_operations: 完整克隆操作次数
//   - fallback_count: 降级次数
//   - cache_hits: 缓存命中次数
//   - cache_misses: 缓存未命中次数
//   - average_sparse_time: 平均稀疏检出时间
//   - average_full_time: 平均完整克隆时间
func (h *ConfigHandler) GetGitPerformanceMetrics(c *gin.Context) {
	// 获取优化服务
	optimizedService, err := h.gitServiceFactory.GetOptimizedGitService()
	if err != nil {
		logger.Logger.Error("获取优化Git服务失败", zap.Error(err))
		c.JSON(http.StatusInternalServerError, gin.H{
			"success": false,
			"error":   "优化服务未可用",
		})
		return
	}

	// 获取性能指标
	metrics := optimizedService.GetPerformanceMetrics()

	c.JSON(http.StatusOK, gin.H{
		"success": true,
		"data":    metrics,
		"message": "性能指标获取成功",
	})
}

// TestGitNetworkQuality 测试Git网络质量
//
// HTTP方法: GET
// 路径: /api/v1/config/git-network-test
//
// 响应:
//   - 200: 返回网络质量测试结果
//   - 500: 服务器内部错误
//
// 响应数据包含:
//   - quality: 网络质量等级
//   - latency: 连接延迟
//   - recommendation: 推荐的操作策略
func (h *ConfigHandler) TestGitNetworkQuality(c *gin.Context) {
	// 获取优化服务
	optimizedService, err := h.gitServiceFactory.GetOptimizedGitService()
	if err != nil {
		logger.Logger.Error("获取优化Git服务失败", zap.Error(err))
		c.JSON(http.StatusInternalServerError, gin.H{
			"success": false,
			"error":   "优化服务未可用",
		})
		return
	}

	// 检测网络质量
	quality, err := optimizedService.DetectNetworkQuality()
	if err != nil {
		logger.Logger.Error("网络质量检测失败", zap.Error(err))
		c.JSON(http.StatusInternalServerError, gin.H{
			"success": false,
			"error":   "网络质量检测失败",
		})
		return
	}

	// 根据网络质量推荐策略
	var recommendation string
	switch quality {
	case services.NetworkQualityGood:
		recommendation = "网络质量优秀，建议使用稀疏检出模式以获得最佳性能"
	case services.NetworkQualityMedium:
		recommendation = "网络质量一般，建议使用稀疏检出模式，必要时会自动降级"
	case services.NetworkQualityPoor:
		recommendation = "网络质量较差，建议使用完整克隆模式以确保稳定性"
	default:
		recommendation = "网络质量未知，建议使用自动模式让系统智能选择"
	}

	response := gin.H{
		"quality":       h.getQualityString(quality),
		"recommendation": recommendation,
		"test_time":     time.Now().Format("2006-01-02 15:04:05"),
	}

	c.JSON(http.StatusOK, gin.H{
		"success": true,
		"data":    response,
		"message": "网络质量检测完成",
	})
}

// getQualityString 获取网络质量字符串表示
func (h *ConfigHandler) getQualityString(quality services.NetworkQuality) string {
	switch quality {
	case services.NetworkQualityGood:
		return "good"
	case services.NetworkQualityMedium:
		return "medium"
	case services.NetworkQualityPoor:
		return "poor"
	default:
		return "unknown"
	}
}
