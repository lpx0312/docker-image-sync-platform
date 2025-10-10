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
	"go.uber.org/zap"
	"golang.org/x/time/rate"
)

func main() {
	// 加载配置
	if err := config.LoadConfig("config.yaml"); err != nil {
		log.Fatalf("加载配置失败: %v", err)
	}

	// 初始化日志
	if err := logger.InitLogger(); err != nil {
		log.Fatalf("初始化日志失败: %v", err)
	}
	defer logger.Logger.Sync()

	// 初始化数据库
	if err := database.InitDatabase(); err != nil {
		logger.Logger.Fatal("初始化数据库失败", zap.Error(err))
	}
	defer database.CloseDatabase()

	// 自动迁移数据库表
	if err := database.AutoMigrate(); err != nil {
		logger.Logger.Fatal("数据库表迁移失败", zap.Error(err))
	}

	// 初始化服务
	gitService := services.NewGitService()
	githubService := services.NewGitHubService()

	// 初始化处理器
	syncHandler := handlers.NewSyncHandler(gitService, githubService)
	imageHandler := handlers.NewImageHandler()
	configHandler := handlers.NewConfigHandler()

	// 启动时检查并修复卡住的任务状态
	go checkStuckTasks(syncHandler)

	// 设置Gin模式
	if config.AppConfig.Server.Mode == "release" {
		gin.SetMode(gin.ReleaseMode)
	}

	// 创建路由
	router := gin.New()

	// 中间件
	router.Use(middleware.RequestLogger(logger.Logger))
	router.Use(middleware.ErrorHandler(logger.Logger))
	router.Use(middleware.CORS())
	
	// 全局限流：每秒100个请求，突发200个
	router.Use(middleware.RateLimit(rate.Limit(100), 200))

	// 静态文件服务（前端）
	router.Static("/static", "./web/dist/static")
	router.StaticFile("/", "./web/dist/index.html")
	router.StaticFile("/favicon.ico", "./web/dist/favicon.ico")

	// API路由
	api := router.Group("/api/v1")
	{
		// 镜像同步相关
		sync := api.Group("/sync")
		{
			// 同步操作使用更严格的限流
			sync.POST("/submit", middleware.SyncRateLimit(), syncHandler.SubmitSync)
			sync.POST("/batch", middleware.SyncRateLimit(), syncHandler.SubmitBatchSync)
			sync.GET("/status/:taskId", syncHandler.GetSyncStatus)
			sync.GET("/batch/status/:taskId", syncHandler.GetBatchSyncStatus)
			sync.GET("/history", syncHandler.GetSyncHistory)
		}

		// 镜像管理相关
		images := api.Group("/images")
		{
			images.GET("/list", imageHandler.GetImages)
			images.GET("/:id", imageHandler.GetImage)
			images.DELETE("/:id", imageHandler.DeleteImage)
			images.GET("/stats", imageHandler.GetImageStats)
			images.POST("/:id/retry", imageHandler.RetrySync)
		}

		// GitHub Actions相关
		github := api.Group("/github")
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

				runs, err := githubService.ListWorkflowRuns(page, perPage)
				if err != nil {
					c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
					return
				}

				c.JSON(http.StatusOK, runs)
			})
			
			github.GET("/runs/:runId", func(c *gin.Context) {
				runID := c.Param("runId")
				run, err := githubService.GetWorkflowRunDetails(runID)
				if err != nil {
					c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
					return
				}

				c.JSON(http.StatusOK, run)
			})
			
			github.GET("/rate-limit", func(c *gin.Context) {
				rateLimit, err := githubService.CheckRateLimit()
				if err != nil {
					c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
					return
				}

				c.JSON(http.StatusOK, rateLimit)
			})
		}

		// 配置相关
		config := api.Group("/config")
		{
			config.GET("/aliyun", configHandler.GetAliyunConfig)
		}

		// 健康检查
		api.GET("/health", func(c *gin.Context) {
			c.JSON(http.StatusOK, gin.H{
				"status":    "ok",
				"timestamp": time.Now().Unix(),
				"version":   "1.0.0",
			})
		})
	}

	// 创建HTTP服务器
	srv := &http.Server{
		Addr:    fmt.Sprintf(":%s", config.AppConfig.Server.Port),
		Handler: router,
	}

	// 启动服务器
	go func() {
		logger.Logger.Info("启动HTTP服务器", 
			zap.String("address", srv.Addr),
			zap.String("mode", config.AppConfig.Server.Mode))
		
		if err := srv.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			logger.Logger.Fatal("启动服务器失败", zap.Error(err))
		}
	}()

	// 等待中断信号
	quit := make(chan os.Signal, 1)
	signal.Notify(quit, syscall.SIGINT, syscall.SIGTERM)
	<-quit

	logger.Logger.Info("正在关闭服务器...")

	// 优雅关闭
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	if err := srv.Shutdown(ctx); err != nil {
		logger.Logger.Fatal("服务器强制关闭", zap.Error(err))
	}

	logger.Logger.Info("服务器已关闭")
}

// checkStuckTasks 检查并修复卡住的任务状态
func checkStuckTasks(syncHandler *handlers.SyncHandler) {
	logger.Logger.Info("开始检查卡住的任务状态...")
	
	// 查找所有状态为running的任务
	var stuckTasks []struct {
		TaskID      string
		GitHubRunID string
		CreatedAt   time.Time
		StartedAt   *time.Time
	}
	
	if err := database.DB.Table("sync_tasks").
		Select("task_id, github_run_id, created_at, started_at").
		Where("status = ?", "running").
		Find(&stuckTasks).Error; err != nil {
		logger.Logger.Error("查询卡住的任务失败", zap.Error(err))
		return
	}
	
	if len(stuckTasks) == 0 {
		logger.Logger.Info("没有发现卡住的任务")
		return
	}
	
	logger.Logger.Info("发现卡住的任务", zap.Int("count", len(stuckTasks)))
	
	// 检查每个卡住的任务
	now := time.Now()
	timeoutDuration := time.Duration(config.AppConfig.Sync.TimeoutMinutes) * time.Minute
	
	for _, taskInfo := range stuckTasks {
		// 计算任务运行时间
		var taskStartTime time.Time
		if taskInfo.StartedAt != nil {
			taskStartTime = *taskInfo.StartedAt
		} else {
			taskStartTime = taskInfo.CreatedAt
		}
		elapsed := now.Sub(taskStartTime)
		
		logger.Logger.Info("检查任务状态", 
			zap.String("task_id", taskInfo.TaskID),
			zap.String("run_id", taskInfo.GitHubRunID),
			zap.Duration("elapsed", elapsed),
			zap.Bool("is_timeout", elapsed > timeoutDuration))
		
		// 获取完整的任务信息
		var task models.SyncTask
		if err := database.DB.Where("task_id = ?", taskInfo.TaskID).First(&task).Error; err != nil {
			logger.Logger.Error("获取任务详情失败", 
				zap.Error(err), 
				zap.String("task_id", taskInfo.TaskID))
			continue
		}
		
		// 使用syncHandler的方法检查和更新状态
		syncHandler.CheckAndUpdateTaskStatus(&task)
		
		// 添加延迟避免API限制
		time.Sleep(1 * time.Second)
	}
	
	logger.Logger.Info("任务状态检查完成")
}