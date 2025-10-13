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
//   go run main.go
//   或
//   ./docker-sync-platform
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

	// 内部包导入
	"docker-image-sync-platform/internal/config"
	"docker-image-sync-platform/internal/database"
	"docker-image-sync-platform/internal/handlers"
	"docker-image-sync-platform/internal/logger"
	"docker-image-sync-platform/internal/middleware"
	"docker-image-sync-platform/internal/services"

	// 第三方包导入
	"github.com/gin-gonic/gin"
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

	// ========================================================================
	// 第二阶段：业务服务初始化
	// ========================================================================
	
	// 初始化Git服务
	// 负责从Git仓库（Gitee/GitHub）克隆和解析镜像配置文件
	gitService := services.NewGitService()
	
	// 初始化GitHub服务
	// 负责GitHub Actions工作流的监控和管理
	githubService := services.NewGitHubService()

	// 初始化HTTP请求处理器
	// 每个处理器负责特定的业务逻辑
	syncHandler := handlers.NewSyncHandler(gitService, githubService)     // 同步操作处理器
	imageHandler := handlers.NewImageHandler()                           // 镜像管理处理器
	configHandler := handlers.NewConfigHandler()                         // 配置管理处理器

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
	router.Static("/static", "./web/dist/static")           // CSS、JS、图片等静态资源
	router.StaticFile("/", "./web/dist/index.html")         // 前端应用入口页面
	router.StaticFile("/favicon.ico", "./web/dist/favicon.ico") // 网站图标

	// ========================================================================
	// API路由配置
	// ========================================================================
	
	// API版本分组：/api/v1
	// 所有API接口都在此分组下，便于版本管理和升级
	api := router.Group("/api/v1")
	{
		// ====================================================================
		// 镜像同步相关API
		// ====================================================================
		sync := api.Group("/sync")
		{
			// 同步操作使用更严格的限流中间件
			// 防止频繁的同步请求对系统造成压力
			
			// POST /api/v1/sync/submit - 提交单个镜像同步任务
			// 从Git仓库解析单个镜像配置并提交同步任务
			sync.POST("/submit", middleware.SyncRateLimit(), syncHandler.SubmitSync)
			
			// POST /api/v1/sync/batch - 提交批量镜像同步任务
			// 从Git仓库解析多个镜像配置并批量提交同步任务
			sync.POST("/batch", middleware.SyncRateLimit(), syncHandler.SubmitBatchSync)
			
			// POST /api/v1/sync/batch/mock - 提交模拟批量同步任务
			// 用于测试和演示，不执行实际的镜像同步操作
			sync.POST("/batch/mock", middleware.SyncRateLimit(), syncHandler.SubmitMockBatchSync)
			
			// GET /api/v1/sync/status/:taskId - 查询单个同步任务状态
			// 实时查询指定任务的执行状态和进度
			sync.GET("/status/:taskId", syncHandler.GetSyncStatus)
			
			// GET /api/v1/sync/batch/status/:taskId - 查询批量同步任务状态
			// 查询批量任务的整体状态和各子任务的执行情况
			sync.GET("/batch/status/:taskId", syncHandler.GetBatchSyncStatus)
			
			// GET /api/v1/sync/history - 获取同步历史记录
			// 分页查询历史同步任务，支持状态筛选和时间范围查询
			sync.GET("/history", syncHandler.GetSyncHistory)
		}

		// ====================================================================
		// 镜像管理相关API
		// ====================================================================
		images := api.Group("/images")
		{
			// GET /api/v1/images/list - 获取镜像列表
			// 分页查询镜像列表，支持状态筛选、关键词搜索等
			images.GET("/list", imageHandler.GetImages)
			
			// GET /api/v1/images/:id - 获取单个镜像详情
			// 查询指定镜像的详细信息，包括同步历史、状态等
			images.GET("/:id", imageHandler.GetImage)
			
			// DELETE /api/v1/images/:id - 删除镜像记录
			// 软删除指定的镜像记录（设置deleted_at字段）
			images.DELETE("/:id", imageHandler.DeleteImage)
			
			// GET /api/v1/images/stats - 获取镜像统计信息
			// 返回镜像总数、各状态数量、成功率等统计数据
			images.GET("/stats", imageHandler.GetImageStats)
			
			// POST /api/v1/images/:id/retry - 重试镜像同步
			// 重新提交失败的镜像同步任务
			images.POST("/:id/retry", imageHandler.RetrySync)
			
			// POST /api/v1/images/:id/check - 检查单个镜像是否存在
			// 检查指定镜像在目标仓库中是否存在
			images.POST("/:id/check", imageHandler.CheckImageExists)
			
			// POST /api/v1/images/batch-check - 批量检查镜像存在性
			// 批量检查多个镜像在目标仓库中的存在状态
			images.POST("/batch-check", imageHandler.BatchCheckImages)
		}

		// ====================================================================
		// GitHub Actions集成API
		// ====================================================================
		github := api.Group("/github")
		{
			// GET /api/v1/github/runs - 获取工作流运行列表
			// 查询GitHub Actions工作流运行历史，支持分页查询
			github.GET("/runs", func(c *gin.Context) {
				page := 1
				perPage := 10
				
				// 解析分页参数
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

				// 调用GitHub服务获取工作流运行列表
				runs, err := githubService.ListWorkflowRuns(page, perPage)
				if err != nil {
					c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
					return
				}

				c.JSON(http.StatusOK, runs)
			})
			
			// GET /api/v1/github/runs/:runId - 获取工作流运行详情
			// 查询指定工作流运行的详细信息，包括日志、状态、执行时间等
			github.GET("/runs/:runId", func(c *gin.Context) {
				runID := c.Param("runId")
				
				// 调用GitHub服务获取工作流运行详情
				run, err := githubService.GetWorkflowRunDetails(runID)
				if err != nil {
					c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
					return
				}

				c.JSON(http.StatusOK, run)
			})
			
			// GET /api/v1/github/rate-limit - 获取GitHub API速率限制状态
			// 查询当前GitHub API的调用次数限制和剩余次数
			github.GET("/rate-limit", func(c *gin.Context) {
				// 调用GitHub服务检查API速率限制
				rateLimit, err := githubService.CheckRateLimit()
				if err != nil {
					c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
					return
				}

				c.JSON(http.StatusOK, rateLimit)
			})
		}

		// ====================================================================
		// 系统配置相关API
		// ====================================================================
		config := api.Group("/config")
		{
			// GET /api/v1/config/aliyun - 获取阿里云容器镜像服务配置
			// 返回阿里云ACR的基本配置信息（不包含敏感信息）
			config.GET("/aliyun", configHandler.GetAliyunConfig)
			
			// GET /api/v1/config/status - 获取当前配置状态和环境变量信息
			// 用于调试和验证配置是否正确加载（不包含敏感信息）
			config.GET("/status", configHandler.GetConfigStatus)
		}

		// ====================================================================
		// 系统健康检查API
		// ====================================================================
		// GET /api/v1/health - 系统健康检查接口
		// 用于负载均衡器、监控系统等检查服务状态
		api.GET("/health", func(c *gin.Context) {
			c.JSON(http.StatusOK, gin.H{
				"status":    "ok",                    // 服务状态
				"timestamp": time.Now().Unix(),       // 当前时间戳
				"version":   "1.0.0",                // 应用版本号
			})
		})
	}

	// ========================================================================
	// HTTP服务器启动和优雅关闭
	// ========================================================================
	
	// 创建HTTP服务器实例
	// 配置监听地址和处理器
	srv := &http.Server{
		Addr:    fmt.Sprintf(":%s", config.AppConfig.Server.Port),  // 监听端口
		Handler: router,                                            // 路由处理器
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

	logger.Logger.Info("服务器已安全关闭")
}