// Package main 是Docker镜像同步平台的主程序入口
//
// 本程序提供了一个完整的Docker镜像同步解决方案，支持：
// - 从Gitee/GitHub等Git仓库同步Docker镜像配置
// - 自动推送镜像到阿里云容器镜像服务
// - 提供Web界面进行镜像管理和状态监控
// - 支持批量同步和单个镜像同步
// - 集成GitHub Actions工作流监控
//
// 架构说明：
// - 使用Gin框架提供RESTful API服务
// - 使用GORM进行数据库操作
// - 使用Zap进行结构化日志记录
// - 支持优雅关闭和信号处理
//
// 启动方式：
//
//	go run main.go
//	或
//	./docker-sync-platform
//
// 环境要求：
// - Go 1.21+
// - MySQL 8.0+
// - Git客户端
// - Docker环境（用于镜像操作）
//
// 配置文件：config.yaml
// 日志输出：logs/app.log
// 数据库：MySQL（自动创建表结构）
//
// Author: Docker Image Sync Platform Team
// Version: 1.0.0
package main

import (
	"context"
	"fmt"
	"log"
	"net/http"
	"os"
	"os/signal"
	"strconv"
	"syscall"
	"time"

	"docker-image-sync-platform/internal/config"
	"docker-image-sync-platform/internal/database"
	"docker-image-sync-platform/internal/handlers"
	"docker-image-sync-platform/internal/logger"
	"docker-image-sync-platform/internal/middleware"
	"docker-image-sync-platform/internal/models"
	"docker-image-sync-platform/internal/services"

	"github.com/gin-gonic/gin"
	"github.com/sirupsen/logrus"
	"go.uber.org/zap"
	"golang.org/x/time/rate"
)

// main 是应用程序的主入口点
// 负责初始化所有组件、设置路由、启动HTTP服务器并处理优雅关闭
func main() {
	// ========================================================================
	// 第一阶段：基础组件初始化
	// ========================================================================

	// 加载应用配置文件
	// 从config.yaml文件中读取数据库连接、服务器端口、Git配置等信息
	if err := config.LoadConfig("config.yaml"); err != nil {
		log.Fatalf("加载配置失败: %v", err)
	}

	// 初始化结构化日志系统
	// 使用Zap库提供高性能的结构化日志记录
	// 日志将输出到控制台和文件（logs/app.log）
	if err := logger.InitLogger(); err != nil {
		log.Fatalf("初始化日志失败: %v", err)
	}
	defer logger.Logger.Sync() // 确保程序退出前刷新日志缓冲区

	// 初始化数据库连接
	// 连接到MySQL数据库，使用连接池管理数据库连接
	if err := database.InitDatabase(); err != nil {
		logger.Logger.Fatal("初始化数据库失败", zap.Error(err))
	}
	defer database.CloseDatabase() // 确保程序退出前关闭数据库连接

	// 自动迁移数据库表结构
	// 根据模型定义自动创建或更新数据库表
	// 包括：images表、sync_tasks表等
	if err := database.AutoMigrate(); err != nil {
		logger.Logger.Fatal("数据库表迁移失败", zap.Error(err))
	}

	// 检查并迁移加密密钥（从旧默认密钥迁移到新的 ENCRYPTION_KEY）
	if err := database.MigrateEncryptionKeys(); err != nil {
		logger.Logger.Fatal("加密密钥迁移失败", zap.Error(err))
	}

	// ========================================================================
	// 第二阶段：业务服务初始化
	// ========================================================================

	// 创建logrus logger用于配置服务
	// 配置服务需要logrus.Logger，这里创建一个简单的实例
	logrusLogger := logrus.New()
	logrusLogger.SetLevel(logrus.InfoLevel)

	// 初始化加密服务
	// 负责敏感信息的加密解密
	encryptionService, err := services.NewEncryptionService(logrusLogger)
	if err != nil {
		logger.Logger.Fatal("初始化加密服务失败", zap.Error(err))
	}

	// 初始化Git服务工厂
	// 负责根据配置动态选择Git服务（Gitee/GitHub）
	gitServiceFactory := services.NewGitServiceFactory(encryptionService)

	// 记录Git优化服务状态
	logger.Logger.Info("Git服务工厂初始化完成",
		zap.Bool("use_optimized", gitServiceFactory.IsUsingOptimized()),
		zap.String("default_mode", "sparse_checkout"))

	// 初始化配置服务
	// 负责数据库配置的CRUD操作和加密解密
	configService := services.NewConfigService(database.DB, encryptionService, logrusLogger)

	// 初始化认证服务和用户服务
	authService := services.NewAuthService()
	userService := services.NewUserService(database.DB, authService)

	// 初始化HTTP请求处理器
	syncHandler := handlers.NewSyncHandler(gitServiceFactory)
	imageHandler := handlers.NewImageHandler()
	configHandler := handlers.NewConfigHandler(gitServiceFactory, configService)
	authHandler := handlers.NewAuthHandler(authService, userService)

	// TODO: 定时任务初始化
	// 可以在这里添加定时任务，用于：
	// - 监控长时间运行的同步任务
	// - 定期清理过期的日志和临时文件
	// - 定期检查镜像状态和健康度

	// ========================================================================
	// 第三阶段：HTTP服务器和路由配置
	// ========================================================================

	// 设置Gin框架运行模式
	// release模式：生产环境，禁用调试信息，提高性能
	// debug模式：开发环境，输出详细的调试信息
	if config.AppConfig.Server.Mode == "release" {
		gin.SetMode(gin.ReleaseMode)
	}

	// 创建Gin路由器实例
	// 使用gin.New()而不是gin.Default()以便自定义中间件
	router := gin.New()

	// ========================================================================
	// 中间件配置（按执行顺序）
	// ========================================================================

	// 1. 请求日志中间件
	// 记录每个HTTP请求的详细信息（方法、路径、状态码、响应时间等）
	router.Use(middleware.RequestLogger(logger.Logger))

	// 2. 错误处理中间件
	// 统一处理和记录应用程序中的错误
	router.Use(middleware.ErrorHandler(logger.Logger))

	// 3. CORS跨域中间件
	// 允许前端应用从不同域名访问API
	router.Use(middleware.CORS())

	// 4. 全局限流中间件
	// 防止API被恶意调用，保护服务器资源
	// 限制：每秒100个请求，突发允许200个请求
	router.Use(middleware.RateLimit(rate.Limit(100), 200))

	// ========================================================================
	// 静态文件服务配置
	// ========================================================================

	// 前端静态资源服务
	// 为Vue.js构建的前端应用提供静态文件服务
	router.Static("/static", "./web/dist/static")               // CSS、JS、图片等静态资源
	router.StaticFile("/", "./web/dist/index.html")             // 前端应用入口页面
	router.StaticFile("/favicon.ico", "./web/dist/favicon.ico") // 网站图标

	// ========================================================================
	// API路由配置
	// ========================================================================

	api := router.Group("/api/v1")
	{
		// 公开接口（无需认证）
		api.GET("/health", func(c *gin.Context) {
			c.JSON(http.StatusOK, gin.H{
				"status":    "ok",
				"timestamp": time.Now().Unix(),
				"version":   "1.0.0",
			})
		})

		api.Static("/docs", "./docs")
		api.StaticFile("/docs.html", "./docs/swagger-ui.html")
		api.StaticFile("/swagger.json", "./docs/swagger.json")

		// 认证接口（公开）
		auth := api.Group("/auth")
		{
			auth.POST("/login", authHandler.Login)
		}

		// 需要登录的认证接口
		authProtected := api.Group("/auth")
		authProtected.Use(middleware.AuthRequired(authService))
		{
			authProtected.GET("/me", authHandler.GetCurrentUser)
			authProtected.PUT("/password", authHandler.ChangePassword)
			authProtected.POST("/logout", authHandler.Logout)
		}

		// 管理员接口
		authAdmin := api.Group("/auth")
		authAdmin.Use(middleware.AuthRequired(authService), middleware.AdminRequired())
		{
			authAdmin.GET("/roles", authHandler.GetRoles)
			authAdmin.GET("/login-logs", authHandler.GetLoginLogs)
			authAdmin.GET("/users", authHandler.ListUsers)
			authAdmin.POST("/users", authHandler.CreateUser)
			authAdmin.PUT("/users/:id/status", authHandler.UpdateUserStatus)
			authAdmin.PUT("/users/:id/role", authHandler.UpdateUserRole)
			authAdmin.DELETE("/users/:id", authHandler.DeleteUser)
			authAdmin.PUT("/users/:id/password", authHandler.ResetUserPassword)
		}

		// 以下所有业务接口需要登录
		protected := api.Group("")
		protected.Use(middleware.AuthRequired(authService))
		{
			syncGroup := protected.Group("/sync")
			{
				syncGroup.POST("/submit", middleware.SyncRateLimit(), syncHandler.SubmitSync)
				syncGroup.POST("/batch", middleware.SyncRateLimit(), syncHandler.SubmitBatchSync)
				syncGroup.POST("/batch/mock", middleware.SyncRateLimit(), syncHandler.SubmitMockBatchSync)
				syncGroup.GET("/status/:taskId", syncHandler.GetSyncStatus)
				syncGroup.GET("/batch/status/:taskId", syncHandler.GetBatchSyncStatus)
				syncGroup.GET("/history", syncHandler.GetSyncHistory)
			}

			images := protected.Group("/images")
			{
				images.GET("/list", imageHandler.GetImages)
				images.GET("/stats", imageHandler.GetImageStats)
				images.POST("/batch-check", imageHandler.BatchCheckImages)
				images.GET("/:id", imageHandler.GetImage)
				images.DELETE("/:id", imageHandler.DeleteImage)
				images.POST("/:id/retry", imageHandler.RetrySync)
				images.POST("/:id/check", imageHandler.CheckImageExists)
			}

			github := protected.Group("/github")
			github.Use(middleware.PermissionRequired(models.PermGitHub))
			{
			github.GET("/runs", func(c *gin.Context) {
				page := 1
				perPage := 10
				if p := c.Query("page"); p != "" {
					if parsed, err := strconv.Atoi(p); err == nil && parsed > 0 {
						page = parsed
					}
				}
				if pp := c.Query("per_page"); pp != "" {
					if parsed, err := strconv.Atoi(pp); err == nil && parsed > 0 && parsed <= 100 {
						perPage = parsed
					}
				}
				status := c.Query("status")
				githubAPIService := gitServiceFactory.GetGitHubAPIService()
				runs, err := githubAPIService.ListWorkflowRuns(page, perPage, status)
				if err != nil {
					c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
					return
				}
				c.JSON(http.StatusOK, runs)
			})

				github.GET("/runs/:runId", func(c *gin.Context) {
					runID := c.Param("runId")
					githubAPIService := gitServiceFactory.GetGitHubAPIService()
					run, err := githubAPIService.GetWorkflowRunDetails(runID)
					if err != nil {
						c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
						return
					}
					c.JSON(http.StatusOK, run)
				})

				github.GET("/rate-limit", func(c *gin.Context) {
					githubAPIService := gitServiceFactory.GetGitHubAPIService()
					rateLimit, err := githubAPIService.CheckRateLimit()
					if err != nil {
						c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
						return
					}
					c.JSON(http.StatusOK, rateLimit)
				})
			}

			configGroup := protected.Group("/config")
			configGroup.Use(middleware.PermissionRequired(models.PermConfig))
			{
				configGroup.GET("/status", configHandler.GetConfigStatus)
				configGroup.GET("/git-repository", configHandler.GetGitRepositoryConfig)
				configGroup.PUT("/git-repository", configHandler.UpdateGitRepositoryConfig)
				configGroup.GET("/git", configHandler.GetGitConfig)
				configGroup.PUT("/git/gitee", configHandler.UpdateGiteeConfig)
				configGroup.PUT("/git/github", configHandler.UpdateGitHubConfig)
				configGroup.POST("/git/test", configHandler.TestGitConnection)
				configGroup.GET("/git-optimization", configHandler.GetGitOptimizationConfig)
				configGroup.PUT("/git-optimization", configHandler.UpdateGitOptimizationConfig)
				configGroup.GET("/git-performance", configHandler.GetGitPerformanceMetrics)
				configGroup.GET("/git-network-test", configHandler.TestGitNetworkQuality)
				configGroup.POST("/git-test-operations", configHandler.TestGitOperations)
				configGroup.GET("/aliyun-db", configHandler.GetAliyunConfig)
				configGroup.PUT("/aliyun-db", configHandler.UpdateAliyunConfig)
				configGroup.POST("/aliyun/test", configHandler.TestAliyunConnection)
				configGroup.GET("/all", configHandler.GetAllConfigs)

				// debug 路由仅在开发环境下注册
				if config.AppConfig.Server.Mode != "release" {
					configGroup.GET("/debug/:key", configHandler.DebugGetConfig)
				}
			}

			// ACR配置管理
			acrRegistryService := services.NewAcrRegistryService(database.DB, encryptionService)
			acrRegistryHandler := handlers.NewAcrRegistryHandler(acrRegistryService)

			acrRegistries := protected.Group("/acr-registries")
			acrRegistries.Use(middleware.PermissionRequired(models.PermConfig))
			{
				acrRegistries.GET("", acrRegistryHandler.GetAll)
				acrRegistries.POST("", acrRegistryHandler.Create)
				acrRegistries.GET("/default", acrRegistryHandler.GetDefault)
				acrRegistries.GET("/:id", acrRegistryHandler.GetByID)
				acrRegistries.PUT("/:id", acrRegistryHandler.Update)
				acrRegistries.DELETE("/:id", acrRegistryHandler.Delete)
				acrRegistries.PUT("/:id/default", acrRegistryHandler.SetDefault)
			}

			// ACR镜像管理
			acrRepositoryService := services.NewAcrRepositoryService(database.DB)
			acrRepositoryHandler := handlers.NewAcrRepositoryHandler(acrRepositoryService)

			acrRepositories := protected.Group("/acr-repositories")
			acrRepositories.Use(middleware.PermissionRequired(models.PermConfig))
			{
				acrRepositories.GET("", acrRepositoryHandler.GetAll)
				acrRepositories.POST("", acrRepositoryHandler.Create)
				acrRepositories.POST("/batch", acrRepositoryHandler.BatchCreate)
				acrRepositories.DELETE("/:id", acrRepositoryHandler.Delete)
				acrRepositories.POST("/sync-from-records", acrRepositoryHandler.SyncFromRecords)
			}

			// ACR Tag查询
			acrAPIService := services.NewAcrAPIService()
			acrTagHandler := handlers.NewAcrTagHandler(acrAPIService, encryptionService)

			acrTags := protected.Group("/acr-tags")
			acrTags.Use(middleware.PermissionRequired(models.PermConfig))
			{
				acrTags.GET("", acrTagHandler.GetTags)
				acrTags.GET("/detail", acrTagHandler.GetTagDetail)
			}
		}
	}

	// ========================================================================
	// HTTP服务器启动和优雅关闭
	// ========================================================================

	// 创建HTTP服务器实例
	// 配置监听地址和处理器
	srv := &http.Server{
		Addr:    fmt.Sprintf(":%s", config.AppConfig.Server.Port), // 监听端口
		Handler: router,                                           // 路由处理器
	}

	// 在独立的goroutine中启动HTTP服务器
	// 避免阻塞主线程，以便处理优雅关闭信号
	go func() {
		logger.Logger.Info("正在启动HTTP服务器",
			zap.String("address", srv.Addr),
			zap.String("mode", config.AppConfig.Server.Mode))

		if err := srv.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			logger.Logger.Fatal("启动服务器失败", zap.Error(err))
		}
	}()

	// ========================================================================
	// 优雅关闭处理
	// ========================================================================

	// 创建信号通道，用于接收系统中断信号
	quit := make(chan os.Signal, 1)

	// 注册要监听的系统信号
	// SIGINT: Ctrl+C中断信号
	// SIGTERM: 终止信号（Docker stop等）
	signal.Notify(quit, syscall.SIGINT, syscall.SIGTERM)

	// 阻塞等待中断信号
	<-quit

	logger.Logger.Info("收到关闭信号，正在优雅关闭服务器...")

	// 设置关闭超时时间为30秒
	// 给正在处理的请求充足时间完成，避免强制中断
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	// 执行优雅关闭
	// 停止接受新请求，等待现有请求完成
	if err := srv.Shutdown(ctx); err != nil {
		logger.Logger.Fatal("服务器强制关闭", zap.Error(err))
	}

	// 等待后台同步协程完成
	logger.Logger.Info("等待后台同步协程完成...")
	syncHandler.Shutdown()

	logger.Logger.Info("服务器已安全关闭")
}
