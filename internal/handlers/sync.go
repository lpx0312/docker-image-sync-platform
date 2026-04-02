// Package handlers 提供HTTP请求处理器实现
//
// 本包实现了Docker镜像同步平台的核心HTTP处理器，包括：
// - 镜像同步任务的提交和管理
// - 批量同步任务的处理和监控
// - GitHub Actions工作流的集成
// - 同步状态的查询和历史记录管理
//
// 主要功能模块：
// - SyncHandler: 处理镜像同步相关的HTTP请求
// - 支持单个和批量镜像同步
// - 提供实时状态查询和历史记录
// - 集成GitHub Actions自动化工作流
//
// 作者: Docker镜像同步平台开发团队
// 版本: v1.0.0
package handlers

import (
	"context"  // 上下文管理
	"fmt"      // 格式化输出
	"net/http" // HTTP状态码和处理
	"strconv"  // 字符串转换
	"strings"  // 字符串操作
	"sync"     // 并发同步原语
	"time"     // 时间处理

	"docker-image-sync-platform/internal/database" // 数据库操作
	"docker-image-sync-platform/internal/logger"   // 日志记录
	"docker-image-sync-platform/internal/models"   // 数据模型
	"docker-image-sync-platform/internal/services" // 业务服务
	"docker-image-sync-platform/internal/utils"    // 共享工具函数

	"github.com/gin-gonic/gin"       // Web框架
	"github.com/google/uuid"         // UUID生成
	"go.uber.org/zap"                // 结构化日志
)

// SyncHandler 镜像同步处理器
//
// 负责处理所有与Docker镜像同步相关的HTTP请求，包括：
// - 单个镜像同步任务的提交和管理
// - 批量镜像同步任务的处理和监控
// - 同步状态的实时查询和历史记录
// - GitHub Actions工作流的集成和管理
//
// 核心功能：
// - 异步处理同步任务，避免阻塞HTTP请求
// - 支持并发控制，防止系统资源过载
// - 提供详细的错误处理和状态反馈
// - 集成Git服务和GitHub Actions自动化
// - 支持动态Git仓库选择（Gitee/GitHub）
type SyncHandler struct {
	gitServiceFactory *services.GitServiceFactory // Git服务工厂，用于动态选择Git服务和GitHub Actions监控
}

// NewSyncHandler 创建新的同步处理器实例
//
// 参数:
//   - gitServiceFactory: Git服务工厂实例，用于动态选择Git服务和GitHub Actions监控
//
// 返回:
//   - *SyncHandler: 初始化完成的同步处理器实例
//
// 使用示例:
//
//	gitFactory := services.NewGitServiceFactory()
//	syncHandler := NewSyncHandler(gitFactory)
func NewSyncHandler(gitServiceFactory *services.GitServiceFactory) *SyncHandler {
	return &SyncHandler{
		gitServiceFactory: gitServiceFactory,
	}
}

// SubmitBatchSync 提交批量镜像同步任务
//
// 处理批量镜像同步请求，支持多个镜像的并发同步。
// 该方法会创建一个批量同步任务，并为每个镜像创建独立的同步记录，
// 然后异步执行同步操作，避免阻塞HTTP请求。
//
// HTTP方法: POST
// 路径: /api/v1/sync/batch
//
// 请求体: models.BatchSyncRequest
//   - Images: 要同步的镜像列表
//   - MaxConcurrent: 最大并发数 (1-10，默认3)
//   - AutoRetry: 是否自动重试失败的任务
//   - RetryCount: 重试次数
//
// 响应:
//   - 200: 任务提交成功，返回任务ID和预计完成时间
//   - 400: 请求参数错误
//   - 500: 服务器内部错误
//
// 功能特性:
//   - 支持1-10个并发同步任务
//   - 自动生成唯一任务ID
//   - 预估任务完成时间
//   - 异步处理，立即返回响应
//   - 支持自动重试机制
func (h *SyncHandler) SubmitBatchSync(c *gin.Context) {
	// ====================================================================
	// 请求参数解析和验证
	// ====================================================================

	var req models.BatchSyncRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		logger.Logger.Error("解析批量同步请求参数失败", zap.Error(err))
		c.JSON(http.StatusBadRequest, gin.H{"error": "请求参数格式错误"})
		return
	}

	// 验证镜像列表不能为空
	if len(req.Images) == 0 {
		c.JSON(http.StatusBadRequest, gin.H{"error": "镜像列表不能为空"})
		return
	}

	// 验证并发数限制，确保系统资源不被过度占用
	// 允许范围：1-10，超出范围则使用默认值3
	if req.MaxConcurrent < 1 || req.MaxConcurrent > 10 {
		req.MaxConcurrent = 3 // 默认并发数
	}

	// ====================================================================
	// 任务创建和初始化
	// ====================================================================

	// 生成全局唯一的任务ID，用于跟踪和查询任务状态
	taskID := uuid.New().String()

	// 创建批量同步任务主记录
	// 包含任务的基本信息和配置参数
	task := &models.SyncTask{
		TaskID:        taskID,                   // 唯一任务标识
		Status:        models.TaskStatusPending, // 初始状态：等待处理
		MaxConcurrent: req.MaxConcurrent,        // 最大并发数
		TotalImages:   len(req.Images),          // 镜像总数
		AutoRetry:     req.AutoRetry,            // 自动重试开关
		RetryCount:    req.RetryCount,           // 重试次数限制
	}

	// 构建镜像信息的JSON字符串，用于任务记录
	// 格式：每行一个镜像，包含源镜像和目标标签
	var imageStrings []string
	for _, img := range req.Images {
		imageStr := img.SourceImage
		// 如果指定了目标标签，则添加到镜像字符串中
		if img.TargetTag != "" {
			imageStr = imageStr + ":" + img.TargetTag
		}
		imageStrings = append(imageStrings, imageStr)
	}
	task.ImagesJSON = strings.Join(imageStrings, "\n")

	// 保存批量同步任务到数据库
	if err := database.DB.Create(task).Error; err != nil {
		logger.Logger.Error("创建批量同步任务失败", zap.Error(err))
		c.JSON(http.StatusInternalServerError, gin.H{"error": "创建同步任务失败"})
		return
	}

	// ====================================================================
	// 创建镜像同步记录
	// ====================================================================

	// 为批量任务中的每个镜像创建独立的同步记录
	// 每个记录包含镜像的详细信息和同步配置
	for i, img := range req.Images {
		// 解析镜像信息：镜像名、标签、架构
		originalImage, tag, architecture := parseImageInfo(img.SourceImage)

		// 使用请求中指定的标签和架构，覆盖解析的默认值
		if img.TargetTag != "" {
			tag = img.TargetTag // 使用自定义目标标签
		}
		if img.Architecture != "" {
			architecture = img.Architecture // 使用指定架构
		}
		// 如果没有指定架构，默认使用amd64
		if architecture == "" {
			architecture = "amd64"
		}

		// 构建原始输入格式
		var originalInput string
		imageWithTag := originalImage
		if tag != "" {
			imageWithTag = originalImage + ":" + tag
		}

		if architecture == "arm64" {
			originalInput = "--platform=linux/arm64 " + imageWithTag
		} else {
			originalInput = imageWithTag
		}

		// 创建镜像同步记录
		record := &models.ImageSyncRecord{
			TaskID:        taskID,                   // 关联的批量任务ID
			OriginalImage: originalImage,            // 源镜像名称
			Tag:           tag,                      // 镜像标签
			Architecture:  architecture,             // 目标架构
			SyncStatus:    models.SyncStatusPending, // 初始状态：等待同步
			InputOrder:    i + 1,                    // 在批量任务中的顺序
			Priority:      img.Priority,             // 同步优先级
			MaxRetries:    req.RetryCount,           // 最大重试次数
			Description:   img.Description,          // 同步说明描述
			OriginalInput: originalInput,            // 保存用户原始输入（根据架构格式化）
		}

		// 保存镜像同步记录到数据库
		if err := database.DB.Create(record).Error; err != nil {
			logger.Logger.Error("创建镜像同步记录失败", zap.Error(err))
			c.JSON(http.StatusInternalServerError, gin.H{"error": "创建镜像记录失败"})
			return
		}
	}

	// ====================================================================
	// 异步任务处理和响应
	// ====================================================================

	// 启动异步goroutine处理批量同步任务
	// 避免阻塞HTTP请求，提供更好的用户体验
	go h.processSyncTask(taskID)

	// 计算预计完成时间
	// 基于镜像数量、并发数和平均处理时间（3分钟/镜像）
	estimatedMinutes := len(req.Images) * 3 / req.MaxConcurrent
	estimatedCompletion := time.Now().Add(time.Duration(estimatedMinutes) * time.Minute)

	// 返回任务提交成功的响应
	// 包含任务ID、状态信息和预计完成时间
	c.JSON(http.StatusOK, gin.H{
		"task_id":              taskID,                                            // 任务唯一标识
		"status":               models.TaskStatusPending,                          // 当前任务状态
		"total_images":         len(req.Images),                                   // 镜像总数
		"max_concurrent":       req.MaxConcurrent,                                 // 最大并发数
		"estimated_completion": estimatedCompletion.Format("2006-01-02 15:04:05"), // 预计完成时间
		"message":              "批量同步任务已提交，正在处理中",                                 // 提示信息
	})
}

// SubmitSync 提交单个镜像同步任务
//
// 处理单个或少量镜像的同步请求，相比批量同步更简单直接。
// 该方法创建一个同步任务，为每个镜像创建同步记录，然后异步执行同步操作。
//
// HTTP方法: POST
// 路径: /api/v1/sync/submit
//
// 请求体: models.SyncRequest
//   - Images: 要同步的镜像列表（字符串数组）
//   - Architecture: 目标架构（可选，默认amd64）
//   - Description: 任务描述（可选）
//
// 响应:
//   - 200: 任务提交成功，返回任务ID和预计完成时间
//   - 400: 请求参数错误
//   - 500: 服务器内部错误
//
// 功能特性:
//   - 顺序处理镜像（MaxConcurrent=1）
//   - 自动解析镜像名称和标签
//   - 异步处理，立即返回响应
//   - 预估任务完成时间
func (h *SyncHandler) SubmitSync(c *gin.Context) {
	// ====================================================================
	// 请求参数解析和验证
	// ====================================================================

	var req models.SyncRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		logger.Logger.Error("解析同步请求参数失败", zap.Error(err))
		c.JSON(http.StatusBadRequest, gin.H{"error": "请求参数格式错误"})
		return
	}

	// 验证镜像列表不能为空
	if len(req.Images) == 0 {
		c.JSON(http.StatusBadRequest, gin.H{"error": "镜像列表不能为空"})
		return
	}

	// ====================================================================
	// 任务创建和初始化
	// ====================================================================

	// 生成全局唯一的任务ID
	taskID := uuid.New().String()

	// 创建单个同步任务记录
	// 与批量同步不同，这里MaxConcurrent固定为1，按顺序处理
	task := &models.SyncTask{
		TaskID:        taskID,                   // 唯一任务标识
		Status:        models.TaskStatusPending, // 初始状态：等待处理
		MaxConcurrent: 1,                        // 单个同步，顺序处理
		TotalImages:   len(req.Images),          // 镜像总数
		Description:   req.Description,          // 任务描述
	}

	// 构建镜像信息的JSON字符串
	// 格式：每行一个镜像名称
	var imageStrings []string
	for _, img := range req.Images {
		imageStrings = append(imageStrings, img)
	}
	task.ImagesJSON = strings.Join(imageStrings, "\n")

	// 保存同步任务到数据库
	if err := database.DB.Create(task).Error; err != nil {
		logger.Logger.Error("创建同步任务失败", zap.Error(err))
		c.JSON(http.StatusInternalServerError, gin.H{"error": "创建同步任务失败"})
		return
	}

	// ====================================================================
	// 创建镜像同步记录
	// ====================================================================

	// 为任务中的每个镜像创建独立的同步记录
	for i, imageStr := range req.Images {
		// 解析镜像名称和标签
		// 支持格式：image:tag 或 image（默认latest）
		originalImage, tag := h.parseImageNameAndTag(imageStr)

		// 构建原始输入格式
		var originalInput string
		if req.Architecture == "arm64" {
			originalInput = "--platform=linux/arm64 " + imageStr
		} else {
			originalInput = imageStr
		}

		// 创建镜像同步记录
		record := &models.ImageSyncRecord{
			TaskID:        taskID,                   // 关联的任务ID
			OriginalImage: originalImage,            // 源镜像名称
			Tag:           tag,                      // 镜像标签
			Architecture:  req.Architecture,         // 目标架构
			SyncStatus:    models.SyncStatusPending, // 初始状态：等待同步
			InputOrder:    i + 1,                    // 在任务中的顺序
			OriginalInput: originalInput,            // 保存用户原始输入（根据架构格式化）
			Description:   req.Description,          // 同步说明描述
		}

		// 保存镜像同步记录到数据库
		if err := database.DB.Create(record).Error; err != nil {
			logger.Logger.Error("创建镜像同步记录失败", zap.Error(err))
			c.JSON(http.StatusInternalServerError, gin.H{"error": "创建镜像记录失败"})
			return
		}
	}

	// ====================================================================
	// 异步任务处理和响应
	// ====================================================================

	// 启动异步goroutine处理同步任务
	// 避免阻塞HTTP请求，提供更好的用户体验
	go h.processSyncTask(taskID)

	// 计算预计完成时间
	// 基于镜像数量和平均处理时间（3分钟/镜像）
	estimatedMinutes := len(req.Images) * 3
	estimatedCompletion := time.Now().Add(time.Duration(estimatedMinutes) * time.Minute)

	c.JSON(http.StatusOK, gin.H{
		"task_id":              taskID,
		"status":               models.TaskStatusPending,
		"total_images":         len(req.Images),
		"estimated_completion": estimatedCompletion.Format("2006-01-02 15:04:05"),
		"message":              "同步任务已提交，正在处理中",
	})
}

// GetSyncStatus 获取同步任务状态详情
//
// 查询指定任务的详细状态信息，包括任务基本信息、进度统计和所有镜像的同步状态。
// 该接口提供实时的任务执行状态，用于前端展示和监控。
//
// HTTP方法: GET
// 路径: /api/v1/sync/status/:taskId
//
// 路径参数:
//   - taskId: 任务ID（必需）
//
// 响应:
//   - 200: 返回任务详细状态信息
//   - 400: 任务ID参数错误
//   - 404: 任务不存在
//   - 500: 服务器内部错误
//
// 响应数据包含:
//   - 任务基本信息（ID、状态、进度等）
//   - GitHub Actions集成信息
//   - 镜像状态统计和详细记录
//   - 时间戳信息（创建、开始、完成时间）
func (h *SyncHandler) GetSyncStatus(c *gin.Context) {
	// ====================================================================
	// 参数验证
	// ====================================================================

	// 从URL路径中获取任务ID
	taskID := c.Param("taskId")
	if taskID == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "任务ID不能为空"})
		return
	}

	// ====================================================================
	// 查询任务基本信息
	// ====================================================================

	// 查询同步任务的基本信息
	// 包含任务状态、进度、GitHub Actions信息等
	var task models.SyncTask
	if err := database.DB.Where("task_id = ?", taskID).First(&task).Error; err != nil {
		logger.Logger.Error("查询同步任务失败", zap.Error(err))
		c.JSON(http.StatusNotFound, gin.H{"error": "任务不存在"})
		return
	}

	// ====================================================================
	// 查询镜像同步记录
	// ====================================================================

	// 查询任务下所有镜像的同步记录
	// 按输入顺序排序，保持与提交时的顺序一致
	var records []models.ImageSyncRecord
	if err := database.DB.Where("task_id = ?", taskID).
		Order("input_order ASC").
		Find(&records).Error; err != nil {
		logger.Logger.Error("查询镜像记录失败", zap.Error(err))
		c.JSON(http.StatusInternalServerError, gin.H{"error": "查询镜像记录失败"})
		return
	}

	// ====================================================================
	// 统计各状态的镜像数量
	// ====================================================================

	// 统计不同状态的镜像数量，用于前端展示进度
	var pendingCount, syncingCount, successCount, failedCount int
	for _, record := range records {
		switch record.SyncStatus {
		case models.SyncStatusPending:
			pendingCount++ // 等待同步
		case models.SyncStatusSyncing:
			syncingCount++ // 正在同步
		case models.SyncStatusSuccess:
			successCount++ // 同步成功
		case models.SyncStatusFailed:
			failedCount++ // 同步失败
		}
	}

	// ====================================================================
	// 构建响应数据
	// ====================================================================

	// 构建完整的任务状态响应
	// 包含任务信息、统计数据和详细记录
	response := gin.H{
		// 任务基本信息
		"task_id":          task.TaskID,          // 任务唯一标识
		"status":           task.Status,          // 任务状态（pending/running/completed/failed）
		"total_images":     task.TotalImages,     // 总镜像数量
		"completed_images": task.CompletedImages, // 已完成镜像数量
		"failed_images":    task.FailedImages,    // 失败镜像数量
		"progress":         task.Progress,        // 进度百分比

		// GitHub Actions集成信息
		"github_action_url": task.GitHubActionURL, // GitHub Actions工作流URL
		"github_run_id":     task.GitHubRunID,     // GitHub Actions运行ID
		"commit_sha":        task.CommitSHA,       // 提交SHA值

		// 时间信息
		"started_at":   task.StartedAt,   // 任务开始时间
		"completed_at": task.CompletedAt, // 任务完成时间
		"created_at":   task.CreatedAt,   // 任务创建时间
		"updated_at":   task.UpdatedAt,   // 任务更新时间

		// 错误和描述信息
		"error_message": task.ErrorMessage, // 错误信息（如果有）
		"description":   task.Description,  // 任务描述

		// 镜像详细信息
		"images": gin.H{
			"pending": pendingCount, // 等待同步的镜像数量
			"syncing": syncingCount, // 正在同步的镜像数量
			"success": successCount, // 同步成功的镜像数量
			"failed":  failedCount,  // 同步失败的镜像数量
			"records": records,      // 所有镜像的详细记录
		},
	}

	// 返回JSON响应
	c.JSON(http.StatusOK, response)
}

// GetBatchSyncStatus 获取批量同步状态 - 已废弃的API
//
// 该接口已被废弃，不再提供批量同步功能。
// 客户端调用此接口将收到410 Gone状态码，提示使用替代方案。
//
// HTTP方法: GET
// 路径: /api/v1/sync/batch/status/:taskId
//
// 响应:
//   - 410: 功能已废弃，返回错误信息和建议
//
// 废弃原因:
//   - 批量同步功能已整合到单一同步接口中
//   - 简化API设计，减少维护复杂度
//   - 推荐使用模拟同步功能进行测试
func (h *SyncHandler) GetBatchSyncStatus(c *gin.Context) {
	// 记录废弃API的调用日志
	logger.Logger.Warn("尝试调用已废弃的批量同步状态查询API")

	// 返回410 Gone状态码，表示资源已永久移除
	c.JSON(http.StatusGone, gin.H{
		"error": "批量同步功能已废弃，请使用模拟同步功能进行测试",
		"code":  "FEATURE_DEPRECATED",
	})
}

// GetSyncHistory 获取同步任务历史记录
//
// 分页查询所有同步任务的历史记录，支持按状态过滤。
// 用于管理界面展示历史任务列表和统计信息。
//
// HTTP方法: GET
// 路径: /api/v1/sync/history
//
// 查询参数:
//   - page: 页码（可选，默认1）
//   - page_size: 每页大小（可选，默认20，最大100）
//   - status: 状态过滤（可选，如pending/running/completed/failed）
//
// 响应:
//   - 200: 返回分页的任务历史列表
//   - 500: 服务器内部错误
//
// 响应数据包含:
//   - total: 总记录数
//   - page: 当前页码
//   - page_size: 每页大小
//   - data: 任务列表数组
func (h *SyncHandler) GetSyncHistory(c *gin.Context) {
	// ====================================================================
	// 解析分页参数
	// ====================================================================

	// 默认分页参数
	page := 1
	pageSize := 20

	// 解析页码参数
	if p := c.Query("page"); p != "" {
		if parsed, err := strconv.Atoi(p); err == nil && parsed > 0 {
			page = parsed
		}
	}

	// 解析每页大小参数，限制最大值为100
	if ps := c.Query("page_size"); ps != "" {
		if parsed, err := strconv.Atoi(ps); err == nil && parsed > 0 && parsed <= 100 {
			pageSize = parsed
		}
	}

	// ====================================================================
	// 解析过滤参数
	// ====================================================================

	// 状态过滤参数（可选）
	status := c.Query("status")

	// ====================================================================
	// 构建数据库查询
	// ====================================================================

	// 构建基础查询，指定SyncTask模型
	query := database.DB.Model(&models.SyncTask{})

	// 如果指定了状态过滤，添加WHERE条件
	if status != "" {
		query = query.Where("status = ?", status)
	}

	// ====================================================================
	// 查询总记录数
	// ====================================================================

	// 获取符合条件的总记录数，用于分页计算
	var total int64
	if err := query.Count(&total).Error; err != nil {
		logger.Logger.Error("查询同步历史总数失败", zap.Error(err))
		c.JSON(http.StatusInternalServerError, gin.H{"error": "查询失败"})
		return
	}

	// ====================================================================
	// 查询分页数据
	// ====================================================================

	// 查询指定页的任务数据
	var tasks []models.SyncTask
	offset := (page - 1) * pageSize // 计算偏移量

	if err := query.Order("created_at DESC"). // 按创建时间倒序排列
							Limit(pageSize). // 限制返回数量
							Offset(offset).  // 设置偏移量
							Find(&tasks).Error; err != nil {
		logger.Logger.Error("查询同步历史失败", zap.Error(err))
		c.JSON(http.StatusInternalServerError, gin.H{"error": "查询失败"})
		return
	}

	// ====================================================================
	// 构建响应数据
	// ====================================================================

	// 返回分页结果，包含总数、分页信息和数据列表
	c.JSON(http.StatusOK, gin.H{
		"total":     total,    // 总记录数
		"page":      page,     // 当前页码
		"page_size": pageSize, // 每页大小
		"data":      tasks,    // 任务数据列表
	})
}

// processSyncTask 异步处理同步任务
//
// 该方法在后台异步执行同步任务的核心逻辑，包括：
// 1. 更新任务和镜像状态
// 2. 生成images.txt文件内容
// 3. 提交到Git仓库并推送
// 4. 启动GitHub Actions监控
//
// 参数:
//   - taskID: 要处理的任务ID
//
// 处理流程:
//  1. 将任务状态更新为"运行中"
//  2. 查询任务和镜像记录信息
//  3. 更新所有镜像状态为"同步中"
//  4. 生成并提交images.txt文件
//  5. 启动GitHub Actions监控
//
// 注意: 该方法通过goroutine异步调用，不返回错误给调用方
func (h *SyncHandler) processSyncTask(taskID string) {
	// 记录任务开始处理的日志
	logger.Logger.Info("开始处理同步任务", zap.String("task_id", taskID))

	// 记录任务开始时间
	now := time.Now()

	// ====================================================================
	// 查询任务基本信息（在事务外，不涉及状态更新）
	// ====================================================================

	// 获取任务的详细信息，用于后续处理
	var task models.SyncTask
	if err := database.DB.Where("task_id = ?", taskID).First(&task).Error; err != nil {
		logger.Logger.Error("查询任务失败", zap.Error(err))
		h.handleSyncError(taskID, fmt.Sprintf("查询任务失败: %v", err))
		return
	}

	// ====================================================================
	// 查询待同步的镜像记录（在事务外，不涉及状态更新）
	// ====================================================================

	// 获取所有状态为"待同步"的镜像记录
	// 按输入顺序排序，确保处理顺序与提交顺序一致
	var records []models.ImageSyncRecord
	if err := database.DB.Where("task_id = ? AND sync_status = ?", taskID, models.SyncStatusPending).
		Order("input_order ASC").
		Find(&records).Error; err != nil {
		h.handleSyncError(taskID, fmt.Sprintf("查询镜像记录失败: %v", err))
		return
	}

	// 检查是否有镜像需要处理
	if len(records) == 0 {
		logger.Logger.Warn("没有找到待同步的镜像记录", zap.String("task_id", taskID))
		h.handleSyncError(taskID, "没有找到待同步的镜像记录")
		return
	}

	// ====================================================================
	// 使用数据库事务处理关键状态更新
	// ====================================================================

	// 开始数据库事务
	tx := database.DB.Begin()
	defer func() {
		if r := recover(); r != nil {
			tx.Rollback()
			logger.Logger.Error("事务发生panic，已回滚", zap.String("task_id", taskID), zap.Any("panic", r))
			h.handleSyncError(taskID, fmt.Sprintf("事务执行时发生panic: %v", r))
		}
	}()

	// 步骤1: 更新任务状态为运行中
	if err := tx.Model(&models.SyncTask{}).
		Where("task_id = ?", taskID).
		Updates(map[string]interface{}{
			"status":     models.TaskStatusRunning, // 设置为运行中状态
			"started_at": &now,                     // 记录开始时间
		}).Error; err != nil {
		tx.Rollback()
		logger.Logger.Error("更新任务状态失败，事务已回滚", zap.String("task_id", taskID), zap.Error(err))
		h.handleSyncError(taskID, fmt.Sprintf("更新任务状态失败: %v", err))
		return
	}

	// 步骤2: 批量更新所有镜像记录的状态为"同步中"
	// 使用事务确保原子性操作
	var failedRecords []uint
	for _, record := range records {
		if err := tx.Model(&models.ImageSyncRecord{}).
			Where("id = ?", record.ID).
			Updates(map[string]interface{}{
				"sync_status": models.SyncStatusSyncing, // 设置为同步中状态
				"started_at":  &now,                     // 记录开始时间
			}).Error; err != nil {
			logger.Logger.Error("更新镜像状态失败",
				zap.String("task_id", taskID),
				zap.Uint("record_id", record.ID),
				zap.String("image", record.OriginalImage),
				zap.Error(err))
			failedRecords = append(failedRecords, record.ID)
		}
	}

	// 检查是否有镜像状态更新失败
	if len(failedRecords) > 0 {
		tx.Rollback()
		errorMsg := fmt.Sprintf("批量更新镜像状态失败，失败的记录ID: %v", failedRecords)
		logger.Logger.Error("批量更新镜像状态失败，事务已回滚",
			zap.String("task_id", taskID),
			zap.Any("failed_record_ids", failedRecords))
		h.handleSyncError(taskID, errorMsg)
		return
	}

	// 提交事务（此时任务和镜像状态已更新）
	if err := tx.Commit().Error; err != nil {
		logger.Logger.Error("提交事务失败", zap.String("task_id", taskID), zap.Error(err))
		h.handleSyncError(taskID, fmt.Sprintf("提交事务失败: %v", err))
		return
	}

	logger.Logger.Info("数据库事务提交成功",
		zap.String("task_id", taskID),
		zap.Int("image_count", len(records)))

	// ====================================================================
	// 生成并提交images.txt文件（事务提交后执行）
	// ====================================================================

	// 根据镜像记录生成images.txt文件内容并提交到Git仓库
	// 这将触发GitHub Actions工作流开始执行镜像同步
	commitSHA, err := h.updateImagesFile(records)
	if err != nil {
		// Git操作失败，但数据库事务已提交，此时需要回滚数据库状态
		logger.Logger.Error("更新images.txt失败，需要回滚数据库状态", zap.String("task_id", taskID), zap.Error(err))
		h.handleSyncError(taskID, fmt.Sprintf("Git操作失败: %v", err))
		return
	}

	// ====================================================================
	// 更新任务的Git提交信息（单独事务）
	// ====================================================================

	// 将Git提交的SHA值保存到任务记录中
	// 使用单独的事务，因为此时主要事务已经提交
	if err := database.DB.Model(&models.SyncTask{}).
		Where("task_id = ?", taskID).
		Update("commit_sha", commitSHA).Error; err != nil {
		logger.Logger.Error("更新commit SHA失败，但不影响主要流程", zap.String("task_id", taskID), zap.Error(err))
		// 这里不返回错误，因为主要的同步流程已经完成
	}

	// ====================================================================
	// 启动GitHub Actions监控
	// ====================================================================

	// 异步启动GitHub Actions工作流监控
	// 监控同步进度并更新数据库中的状态信息
	go h.monitorGitHubActions(taskID, commitSHA)

	// 记录任务提交成功的日志
	logger.Logger.Info("同步任务已提交到Git，事务已成功提交",
		zap.String("task_id", taskID),
		zap.String("commit_sha", commitSHA),
		zap.Int("image_count", len(records)))
}

// updateImagesFile 生成并提交images.txt文件到Git仓库
//
// 根据镜像同步记录生成符合GitHub Actions工作流要求的images.txt文件内容，
// 并将其提交到Git仓库以触发自动化同步流程。
//
// 参数:
//   - records: 镜像同步记录列表
//
// 返回值:
//   - string: Git提交的SHA值
//   - error: 操作过程中的错误
//
// 镜像格式化规则:
//   - 基础格式: image_name:tag
//   - 多架构支持: --platform=linux/架构 image_name:tag
//   - AMD64架构默认不添加platform前缀
//   - 按输入顺序保持镜像列表顺序
func (h *SyncHandler) updateImagesFile(records []models.ImageSyncRecord) (string, error) {
	// ====================================================================
	// 初始化镜像列表
	// ====================================================================

	var imageLines []string

	// ====================================================================
	// 查询任务信息（用于扩展功能）
	// ====================================================================

	// 获取任务基本信息，为后续功能扩展预留
	var task models.SyncTask
	if len(records) > 0 {
		if err := database.DB.Where("task_id = ?", records[0].TaskID).First(&task).Error; err != nil {
			logger.Logger.Error("查询任务信息失败", zap.Error(err))
			return "", err
		}
	}

	// ====================================================================
	// 格式化镜像信息
	// ====================================================================

	// 遍历所有镜像记录，生成符合GitHub Actions要求的格式
	for _, record := range records {
		// 构建基础镜像名称
		imageLine := record.OriginalImage

		// 添加标签（如果指定）
		if record.Tag != "" {
			imageLine = imageLine + ":" + record.Tag
		}

		// 添加平台架构前缀（非AMD64架构）
		// AMD64是默认架构，不需要显式指定platform参数
		if record.Architecture != "" && record.Architecture != "amd64" {
			imageLine = "--platform=linux/" + record.Architecture + " " + imageLine
		}

		// 添加到镜像列表
		imageLines = append(imageLines, imageLine)
	}

	// ====================================================================
	// 提交到Git仓库（API优化版本）
	// ====================================================================

	var commitSHA string
	var err error

	// 构建文件内容
	content := strings.Join(imageLines, "\n")
	commitMessage := fmt.Sprintf("Add %d new images for sync", len(imageLines))

	logger.Logger.Info("开始更新images.txt文件",
		zap.Int("image_count", len(imageLines)),
		zap.Int("content_length", len(content)))

	// 优先尝试使用API模式
	useAPI := h.gitServiceFactory.IsUsingAPI()
	if useAPI {
		logger.Logger.Info("使用API模式更新文件")
		commitSHA, err = h.updateImagesFileWithAPI(content, commitMessage)
		if err == nil {
			logger.Logger.Info("使用API模式成功更新文件",
				zap.String("commit_sha", commitSHA),
				zap.String("mode", "api"))
			return commitSHA, nil
		}
		logger.Logger.Error("API模式更新失败，回退到传统模式",
			zap.Error(err),
			zap.String("fallback_reason", "api_failed"))
	}

	// 检查是否启用优化服务
	// 使用统一的Git服务接口
	ctx := context.Background()
	gitServiceInterface, err := h.gitServiceFactory.GetGitServiceInterface()
	if err != nil {
		logger.Logger.Error("获取Git服务接口失败", zap.Error(err))
		return "", fmt.Errorf("获取Git服务接口失败: %v", err)
	}

	// 调用统一的Git服务接口更新images.txt文件并推送到远程仓库
	// 这将触发GitHub Actions工作流开始执行
	commitSHA, err = gitServiceInterface.UpdateImagesFile(ctx, imageLines)
	if err != nil {
		return "", fmt.Errorf("原Git服务更新失败: %v", err)
	}

	logger.Logger.Info("使用完整克隆模式成功更新文件",
		zap.String("commit_sha", commitSHA))
	return commitSHA, nil
}

// updateImagesFileWithAPI 使用API模式更新images.txt文件
//
// 参数:
//   - content: 文件内容
//   - commitMessage: 提交信息
//
// 返回:
//   - string: 提交SHA
//   - error: 操作过程中的错误
func (h *SyncHandler) updateImagesFileWithAPI(content, commitMessage string) (string, error) {
	startTime := time.Now()
	logger.Logger.Info("开始使用API模式更新images.txt文件",
		zap.Int("content_length", len(content)),
		zap.String("commit_message", commitMessage))

	// 获取Git文件API服务
	gitFileService, err := h.gitServiceFactory.GetGitFileService()
	if err != nil {
		return "", fmt.Errorf("获取Git文件API服务失败: %w", err)
	}

	// 执行文件更新
	commitSHA, err := gitFileService.UpdateImagesFile(content, commitMessage)
	if err != nil {
		duration := time.Since(startTime)
		logger.Logger.Error("API模式更新文件失败",
			zap.Error(err),
			zap.Duration("duration", duration))
		return "", err
	}

	duration := time.Since(startTime)
	logger.Logger.Info("API模式成功更新images.txt文件",
		zap.String("commit_sha", commitSHA),
		zap.Duration("duration", duration),
		zap.String("estimated_speed", "fast"))

	return commitSHA, nil
}

// monitorGitHubActions 异步监控GitHub Actions工作流执行状态
//
// 该方法持续监控GitHub Actions工作流的执行状态，并根据执行结果
// 更新数据库中的任务和镜像同步状态。
//
// 参数:
//   - taskID: 同步任务ID
//   - commitSHA: Git提交的SHA值，用于查找对应的工作流运行
//
// 监控流程:
//  1. 等待GitHub Actions工作流启动
//  2. 获取工作流运行ID和URL
//  3. 定期检查工作流执行状态
//  4. 根据执行结果更新任务状态
//  5. 处理超时情况
//
// 监控配置:
//   - 最大等待时间: 30分钟
//   - 检查间隔: 30秒
//   - 启动等待时间: 30秒
func (h *SyncHandler) monitorGitHubActions(taskID, commitSHA string) {
	// 记录监控开始的日志
	logger.Logger.Info("开始监控GitHub Actions", zap.String("task_id", taskID), zap.String("commit_sha", commitSHA))

	// ====================================================================
	// 等待GitHub Actions工作流启动
	// ====================================================================

	// GitHub Actions需要一定时间来检测提交并启动工作流
	// 等待30秒确保工作流已经开始执行
	time.Sleep(30 * time.Second)

	// ====================================================================
	// 获取GitHub Actions运行信息
	// ====================================================================

	// 根据提交SHA查找对应的工作流运行
	githubAPIService := h.gitServiceFactory.GetGitHubAPIService()
	runID, runURL, err := githubAPIService.GetWorkflowRun(commitSHA)
	if err != nil {
		logger.Logger.Error("获取GitHub Actions运行信息失败", zap.Error(err))
		// 对于无法获取工作流信息的情况，仍然尝试验证镜像状态
		errorMessage := fmt.Sprintf("获取GitHub Actions运行信息失败: %v", err)
		h.handlePartialSyncFailure(taskID, errorMessage)
		return
	}

	// ====================================================================
	// 更新任务的GitHub集成信息
	// ====================================================================

	// 将GitHub Actions的运行ID和URL保存到数据库
	// 用于前端展示和后续状态追踪
	if err := database.DB.Model(&models.SyncTask{}).
		Where("task_id = ?", taskID).
		Updates(map[string]interface{}{
			"github_run_id":     runID,  // GitHub Actions运行ID
			"github_action_url": runURL, // GitHub Actions运行URL
		}).Error; err != nil {
		logger.Logger.Error("更新GitHub信息失败", zap.Error(err))
	}

	// ====================================================================
	// 配置监控参数
	// ====================================================================

	// 设置监控的时间限制和检查间隔
	maxWaitTime := 30 * time.Minute   // 最大等待时间：30分钟
	checkInterval := 30 * time.Second // 检查间隔：30秒
	startTime := time.Now()           // 记录开始时间

	// ====================================================================
	// 循环监控工作流执行状态
	// ====================================================================

	// 在最大等待时间内持续检查工作流状态
	for time.Since(startTime) < maxWaitTime {
		// 获取当前工作流运行状态
		githubAPIService := h.gitServiceFactory.GetGitHubAPIService()
		status, err := githubAPIService.GetWorkflowRunStatus(runID)
		if err != nil {
			logger.Logger.Error("获取GitHub Actions状态失败", zap.Error(err))
			time.Sleep(checkInterval)
			continue // 出错时继续下一次检查
		}

		// 记录当前状态到日志
		logger.Logger.Info("GitHub Actions状态",
			zap.String("task_id", taskID),
			zap.String("run_id", runID),
			zap.String("status", status))

		// ================================================================
		// 处理工作流完成状态
		// ================================================================

		if status == "completed" {
			// 获取工作流执行结论（成功/失败）
			githubAPIService := h.gitServiceFactory.GetGitHubAPIService()
			conclusion, err := githubAPIService.GetWorkflowRunConclusion(runID)
			if err != nil {
				logger.Logger.Error("获取GitHub Actions结论失败", zap.Error(err))
				// 对于无法获取结论的情况，仍然尝试验证镜像状态
				errorMessage := fmt.Sprintf("获取GitHub Actions结论失败: %v", err)
				h.handlePartialSyncFailure(taskID, errorMessage)
				return
			}

			// 根据执行结论处理任务结果
			if conclusion == "success" {
				h.handleSyncSuccess(taskID) // 处理同步成功
			} else {
				// 即使工作流结论是失败，也要逐个验证镜像状态
				errorMessage := fmt.Sprintf("GitHub Actions执行失败，结论: %s", conclusion)
				h.handlePartialSyncFailure(taskID, errorMessage)
			}
			return
		}

		// ================================================================
		// 处理工作流失败或取消状态
		// ================================================================

		if status == "cancelled" || status == "failure" {
			// 获取工作流执行结论以提供更详细的错误信息
			githubAPIService := h.gitServiceFactory.GetGitHubAPIService()
			conclusion, err := githubAPIService.GetWorkflowRunConclusion(runID)
			var errorMessage string
			if err != nil {
				errorMessage = fmt.Sprintf("GitHub Actions执行失败，状态: %s，无法获取详细结论", status)
			} else {
				errorMessage = fmt.Sprintf("GitHub Actions执行失败，状态: %s，结论: %s", status, conclusion)
			}

			// 使用新的部分失败处理函数，逐个验证镜像状态而不是全部标记为失败
			h.handlePartialSyncFailure(taskID, errorMessage)
			return
		}

		// 等待下一次检查
		time.Sleep(checkInterval)
	}

	// ====================================================================
	// 处理监控超时
	// ====================================================================

	// 如果在最大等待时间内工作流仍未完成，也要尝试验证镜像状态
	h.handlePartialSyncFailure(taskID, "GitHub Actions执行超时")
}

// handleSyncSuccess 处理GitHub Actions工作流执行成功后的逻辑
//
// 当GitHub Actions工作流成功完成后，该方法会验证每个镜像是否真正
// 同步到了阿里云容器镜像服务(ACR)，并更新数据库中的状态信息。
//
// 参数:
//   - taskID: 同步任务ID
//
// 处理流程:
//  1. 查询任务下的所有镜像记录
//  2. 逐个验证镜像是否存在于ACR
//  3. 更新每个镜像的同步状态和结果
//  4. 更新任务的整体状态和统计信息
//  5. 记录详细的执行日志
//
// 验证机制:
//   - 生成ACR镜像地址
//   - 调用镜像仓库API检查镜像存在性
//   - 计算同步耗时
//   - 更新状态为成功或失败
func (h *SyncHandler) handleSyncSuccess(taskID string) {
	// 记录处理开始的日志
	logger.Logger.Info("同步任务成功", zap.String("task_id", taskID))

	// ====================================================================
	// 查询任务的镜像记录
	// ====================================================================

	// 获取任务下所有的镜像同步记录
	var records []models.ImageSyncRecord
	if err := database.DB.Where("task_id = ?", taskID).Find(&records).Error; err != nil {
		logger.Logger.Error("查询镜像记录失败", zap.Error(err))
		return
	}

	// ====================================================================
	// 验证镜像同步结果
	// ====================================================================

	// 统计成功同步的镜像数量
	successCount := 0

	// 逐个验证每个镜像是否成功同步到ACR
	for _, record := range records {
		// 生成目标ACR镜像地址（包含架构信息）
		acrImage := utils.GenerateACRImageWithArchitecture(record.OriginalImage, record.Tag, record.Architecture)

		// 检查镜像是否真正存在于ACR中
		exists := utils.CheckImageExistsInRegistry(acrImage)

		// 计算同步耗时
		completedTime := time.Now()
		var duration int64
		if record.StartedAt != nil {
			duration = int64(completedTime.Sub(*record.StartedAt).Seconds())
		}

		// ================================================================
		// 处理镜像验证成功的情况
		// ================================================================

		if exists {
			// 镜像存在，标记为成功
			if err := database.DB.Model(&models.ImageSyncRecord{}).
				Where("id = ?", record.ID).
				Updates(map[string]interface{}{
					"sync_status":  models.SyncStatusSuccess,
					"completed_at": &completedTime,
					"duration":     duration,
					"acr_image":    acrImage,
				}).Error; err != nil {
				logger.Logger.Error("更新镜像成功状态失败", zap.Error(err))
			} else {
				successCount++
			}
		} else {
			// 镜像不存在，标记为失败
			errorMessage := "镜像未成功同步到ACR"
			if err := database.DB.Model(&models.ImageSyncRecord{}).
				Where("id = ?", record.ID).
				Updates(map[string]interface{}{
					"sync_status":   models.SyncStatusFailed,
					"completed_at":  &completedTime,
					"duration":      duration,
					"acr_image":     acrImage,
					"error_message": errorMessage,
				}).Error; err != nil {
				logger.Logger.Error("更新镜像失败状态失败", zap.Error(err))
			}
		}
	}

	// 更新任务状态
	completedTime := time.Now()
	taskStatus := models.TaskStatusCompleted
	if successCount == 0 {
		taskStatus = models.TaskStatusFailed
	} else if successCount < len(records) {
		taskStatus = models.TaskStatusPartialSuccess
	}

	if err := database.DB.Model(&models.SyncTask{}).
		Where("task_id = ?", taskID).
		Updates(map[string]interface{}{
			"status":           taskStatus,
			"completed_at":     &completedTime,
			"completed_images": successCount,
			"failed_images":    len(records) - successCount,
			"progress":         100.0,
		}).Error; err != nil {
		logger.Logger.Error("更新任务完成状态失败", zap.Error(err))
	}

	logger.Logger.Info("同步任务处理完成",
		zap.String("task_id", taskID),
		zap.String("status", taskStatus),
		zap.Int("success_count", successCount),
		zap.Int("total_count", len(records)))
}

// handleSyncError 处理同步错误
// 使用数据库事务确保任务状态和镜像状态更新的原子性
func (h *SyncHandler) handleSyncError(taskID, errorMessage string) {
	logger.Logger.Error("同步任务失败",
		zap.String("task_id", taskID),
		zap.String("error", errorMessage))

	// 开始数据库事务
	tx := database.DB.Begin()
	defer func() {
		if r := recover(); r != nil {
			tx.Rollback()
			logger.Logger.Error("错误处理事务发生panic，已回滚",
				zap.String("task_id", taskID),
				zap.Any("panic", r))
		}
	}()

	// 记录错误时间
	now := time.Now()

	// 步骤1: 更新任务状态为失败
	if err := tx.Model(&models.SyncTask{}).
		Where("task_id = ?", taskID).
		Updates(map[string]interface{}{
			"status":        models.TaskStatusFailed,
			"completed_at":  &now,
			"error_message": errorMessage,
		}).Error; err != nil {
		tx.Rollback()
		logger.Logger.Error("更新任务失败状态失败，事务已回滚",
			zap.String("task_id", taskID),
			zap.Error(err))
		return
	}

	// 步骤2: 更新所有相关镜像记录为失败
	if err := tx.Model(&models.ImageSyncRecord{}).
		Where("task_id = ? AND sync_status IN (?)", taskID, []string{
			models.SyncStatusPending,
			models.SyncStatusSyncing,
		}).
		Updates(map[string]interface{}{
			"sync_status":   models.SyncStatusFailed,
			"completed_at":  &now,
			"error_message": errorMessage,
		}).Error; err != nil {
		tx.Rollback()
		logger.Logger.Error("更新镜像失败状态失败，事务已回滚",
			zap.String("task_id", taskID),
			zap.Error(err))
		return
	}

	// 提交事务
	if err := tx.Commit().Error; err != nil {
		logger.Logger.Error("提交错误处理事务失败",
			zap.String("task_id", taskID),
			zap.Error(err))
		return
	}

	logger.Logger.Info("同步任务错误处理完成，事务已提交",
		zap.String("task_id", taskID))
}

// handlePartialSyncFailure 处理GitHub Actions工作流执行失败后的镜像状态验证
//
// 当GitHub Actions工作流失败时，该方法会逐个验证每个镜像是否真正
// 同步到了阿里云容器镜像服务(ACR)，而不是简单地将所有镜像标记为失败。
//
// 参数:
//   - taskID: 同步任务ID
//   - workflowErrorMessage: GitHub Actions工作流的错误信息
//
// 处理流程:
//  1. 查询任务下的所有镜像记录
//  2. 逐个验证镜像是否存在于ACR（即使GitHub Actions失败了，有些镜像可能已经成功）
//  3. 根据实际验证结果更新每个镜像的状态
//  4. 更新任务的整体状态为部分成功或失败
//  5. 记录详细的执行日志
func (h *SyncHandler) handlePartialSyncFailure(taskID, workflowErrorMessage string) {
	logger.Logger.Info("开始处理部分同步失败",
		zap.String("task_id", taskID),
		zap.String("workflow_error", workflowErrorMessage))

	// ====================================================================
	// 查询任务的镜像记录
	// ====================================================================

	// 获取任务下所有的镜像同步记录
	var records []models.ImageSyncRecord
	if err := database.DB.Where("task_id = ?", taskID).Find(&records).Error; err != nil {
		logger.Logger.Error("查询镜像记录失败", zap.Error(err))
		return
	}

	if len(records) == 0 {
		logger.Logger.Warn("任务下没有找到镜像记录", zap.String("task_id", taskID))
		return
	}

	// ====================================================================
	// 验证镜像同步结果
	// ====================================================================

	// 统计成功和失败的镜像数量
	successCount := 0
	failedCount := 0
	now := time.Now()

	// 逐个验证每个镜像是否成功同步到ACR
	for _, record := range records {
		// 生成目标ACR镜像地址（包含架构信息）
		acrImage := utils.GenerateACRImageWithArchitecture(record.OriginalImage, record.Tag, record.Architecture)

		// 检查镜像是否真正存在于ACR中
		exists := utils.CheckImageExistsInRegistry(acrImage)

		// 计算同步耗时
		var duration int64
		if record.StartedAt != nil {
			duration = int64(now.Sub(*record.StartedAt).Seconds())
		}

		// ================================================================
		// 处理镜像验证成功的情况
		// ================================================================

		if exists {
			// 镜像存在，标记为成功
			if err := database.DB.Model(&models.ImageSyncRecord{}).
				Where("id = ?", record.ID).
				Updates(map[string]interface{}{
					"sync_status":  models.SyncStatusSuccess,
					"completed_at": &now,
					"duration":     duration,
					"acr_image":    acrImage,
				}).Error; err != nil {
				logger.Logger.Error("更新镜像成功状态失败", zap.Error(err))
			} else {
				successCount++
				logger.Logger.Info("镜像验证成功",
					zap.String("task_id", taskID),
					zap.String("image", record.OriginalImage),
					zap.String("acr_image", acrImage))
			}
		} else {
			// 镜像不存在，标记为失败
			individualErrorMessage := fmt.Sprintf("GitHub Actions工作流失败: %s; 镜像未成功同步到ACR", workflowErrorMessage)
			if err := database.DB.Model(&models.ImageSyncRecord{}).
				Where("id = ?", record.ID).
				Updates(map[string]interface{}{
					"sync_status":   models.SyncStatusFailed,
					"completed_at":  &now,
					"duration":      duration,
					"acr_image":     acrImage,
					"error_message": individualErrorMessage,
				}).Error; err != nil {
				logger.Logger.Error("更新镜像失败状态失败", zap.Error(err))
			} else {
				failedCount++
				logger.Logger.Info("镜像验证失败",
					zap.String("task_id", taskID),
					zap.String("image", record.OriginalImage),
					zap.String("error", individualErrorMessage))
			}
		}
	}

	// ====================================================================
	// 更新任务状态
	// ====================================================================

	// 根据成功/失败数量确定任务状态
	var taskStatus string
	var finalErrorMessage string

	if successCount == 0 {
		// 全部失败
		taskStatus = models.TaskStatusFailed
		finalErrorMessage = workflowErrorMessage
	} else if failedCount == 0 {
		// 全部成功（GitHub Actions报错但实际都成功了）
		taskStatus = models.TaskStatusCompleted
		finalErrorMessage = ""
	} else {
		// 部分成功部分失败
		taskStatus = models.TaskStatusPartialSuccess
		finalErrorMessage = fmt.Sprintf("GitHub Actions工作流失败，但%d个镜像成功同步，%d个镜像失败。工作流错误: %s",
			successCount, failedCount, workflowErrorMessage)
	}

	// 更新任务状态
	if err := database.DB.Model(&models.SyncTask{}).
		Where("task_id = ?", taskID).
		Updates(map[string]interface{}{
			"status":           taskStatus,
			"completed_at":     &now,
			"completed_images": successCount,
			"failed_images":    failedCount,
			"progress":         100.0,
			"error_message":    finalErrorMessage,
		}).Error; err != nil {
		logger.Logger.Error("更新任务状态失败", zap.Error(err))
	}

	logger.Logger.Info("部分同步失败处理完成",
		zap.String("task_id", taskID),
		zap.String("final_status", taskStatus),
		zap.Int("success_count", successCount),
		zap.Int("failed_count", failedCount),
		zap.Int("total_count", len(records)))
}

// parseImageInfo 解析镜像信息
func parseImageInfo(imageStr string) (image, tag, architecture string) {
	// 处理架构信息
	if strings.Contains(imageStr, "--platform=") {
		parts := strings.Fields(imageStr)
		for i, part := range parts {
			if strings.HasPrefix(part, "--platform=") {
				architecture = strings.TrimPrefix(part, "--platform=")
				// 移除架构参数，重新组合镜像字符串
				parts = append(parts[:i], parts[i+1:]...)
				imageStr = strings.Join(parts, " ")
				break
			}
		}
	}

	// 解析镜像和标签
	if strings.Contains(imageStr, ":") {
		parts := strings.Split(imageStr, ":")
		image = parts[0]
		tag = parts[1]
	} else {
		image = imageStr
		tag = "latest"
	}

	return strings.TrimSpace(image), strings.TrimSpace(tag), strings.TrimSpace(architecture)
}

// parseImageNameAndTag 解析镜像名称和标签
func (h *SyncHandler) parseImageNameAndTag(imageStr string) (string, string) {
	// 解析镜像和标签
	if strings.Contains(imageStr, ":") {
		parts := strings.Split(imageStr, ":")
		return strings.TrimSpace(parts[0]), strings.TrimSpace(parts[1])
	}
	return strings.TrimSpace(imageStr), "latest"
}

// SubmitMockBatchSync 提交模拟批量同步任务
func (h *SyncHandler) SubmitMockBatchSync(c *gin.Context) {
	var req models.BatchSyncRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		logger.Logger.Error("解析模拟批量同步请求参数失败", zap.Error(err))
		c.JSON(http.StatusBadRequest, gin.H{"error": "请求参数格式错误"})
		return
	}

	if len(req.Images) == 0 {
		c.JSON(http.StatusBadRequest, gin.H{"error": "镜像列表不能为空"})
		return
	}

	// 验证并发数限制
	if req.MaxConcurrent < 1 || req.MaxConcurrent > 10 {
		req.MaxConcurrent = 3 // 默认值
	}

	// 生成任务ID
	taskID := uuid.New().String()

	// 创建批量同步任务记录
	task := &models.SyncTask{
		TaskID:        taskID,
		Status:        models.TaskStatusPending,
		MaxConcurrent: req.MaxConcurrent,
		TotalImages:   len(req.Images),
		AutoRetry:     req.AutoRetry,
		RetryCount:    req.RetryCount,
	}

	// 构建镜像JSON字符串
	var imageStrings []string
	for _, img := range req.Images {
		imageStr := img.SourceImage
		if img.TargetTag != "" {
			imageStr = imageStr + ":" + img.TargetTag
		}
		imageStrings = append(imageStrings, imageStr)
	}
	task.ImagesJSON = strings.Join(imageStrings, "\n")

	if err := database.DB.Create(task).Error; err != nil {
		logger.Logger.Error("创建模拟批量同步任务失败", zap.Error(err))
		c.JSON(http.StatusInternalServerError, gin.H{"error": "创建同步任务失败"})
		return
	}

	// 为每个镜像创建同步记录
	for i, img := range req.Images {
		originalImage, tag, architecture := parseImageInfo(img.SourceImage)

		// 使用请求中的标签和架构
		if img.TargetTag != "" {
			tag = img.TargetTag
		}
		if img.Architecture != "" {
			architecture = img.Architecture
		}
		if architecture == "" {
			architecture = "amd64"
		}

		// 构建原始输入格式
		var originalInput string
		imageWithTag := originalImage
		if tag != "" {
			imageWithTag = originalImage + ":" + tag
		}

		if architecture == "arm64" {
			originalInput = "--platform=linux/arm64 " + imageWithTag
		} else {
			originalInput = imageWithTag
		}

		record := &models.ImageSyncRecord{
			OriginalImage: originalImage,
			Tag:           tag,
			Architecture:  architecture,
			OriginalInput: originalInput,
			InputOrder:    i, // 保存输入顺序
			SyncStatus:    models.SyncStatusPending,
			TaskID:        taskID,
			Priority:      img.Priority,
			MaxRetries:    req.RetryCount,
			Description:   img.Description, // 添加描述字段
		}

		if err := database.DB.Create(record).Error; err != nil {
			logger.Logger.Error("创建镜像同步记录失败",
				zap.Error(err),
				zap.String("image", originalImage))
		}
	}

	// 异步执行模拟批量同步操作
	go h.processMockBatchSyncTask(taskID)

	// 计算预估时间
	estimatedTime := h.calculateEstimatedTime(len(req.Images), req.MaxConcurrent)

	logger.Logger.Info("模拟批量同步任务已提交",
		zap.String("task_id", taskID),
		zap.Int("image_count", len(req.Images)),
		zap.Int("max_concurrent", req.MaxConcurrent))

	c.JSON(http.StatusOK, models.BatchSyncResponse{
		TaskID:        taskID,
		Message:       "模拟批量同步任务已提交，正在处理中...",
		ImageCount:    len(req.Images),
		EstimatedTime: fmt.Sprintf("%d秒", estimatedTime),
	})
}

// processMockBatchSyncTask 处理模拟批量同步任务
func (h *SyncHandler) processMockBatchSyncTask(taskID string) {
	logger.Logger.Info("开始处理模拟批量同步任务", zap.String("task_id", taskID))

	// 更新任务状态为运行中
	now := time.Now()
	if err := database.DB.Model(&models.SyncTask{}).
		Where("task_id = ?", taskID).
		Updates(map[string]interface{}{
			"status":     models.TaskStatusRunning,
			"started_at": &now,
		}).Error; err != nil {
		logger.Logger.Error("更新模拟批量任务状态失败", zap.Error(err))
		return
	}

	// 获取任务信息
	var task models.SyncTask
	if err := database.DB.Where("task_id = ?", taskID).First(&task).Error; err != nil {
		logger.Logger.Error("查询模拟批量任务失败", zap.Error(err))
		return
	}

	// 获取所有待同步的镜像记录，按输入顺序排序
	var records []models.ImageSyncRecord
	if err := database.DB.Where("task_id = ? AND sync_status = ?", taskID, models.SyncStatusPending).
		Order("input_order ASC").
		Find(&records).Error; err != nil {
		h.handleBatchSyncError(taskID, fmt.Sprintf("查询镜像记录失败: %v", err))
		return
	}

	// 使用信号量控制并发数
	semaphore := make(chan struct{}, task.MaxConcurrent)
	var wg sync.WaitGroup

	// 处理每个镜像
	for _, record := range records {
		wg.Add(1)
		go func(r models.ImageSyncRecord) {
			defer wg.Done()

			// 获取信号量
			semaphore <- struct{}{}
			defer func() { <-semaphore }()

			// 处理单个镜像的模拟同步
			h.processSingleMockImage(taskID, r)
		}(record)
	}

	// 等待所有镜像处理完成
	wg.Wait()

	// 更新任务最终状态
	h.updateBatchTaskFinalStatus(taskID)

	logger.Logger.Info("模拟批量同步任务完成", zap.String("task_id", taskID))
}

// processSingleMockImage 处理单个镜像的模拟同步
func (h *SyncHandler) processSingleMockImage(taskID string, record models.ImageSyncRecord) {
	logger.Logger.Info("开始模拟镜像同步",
		zap.String("task_id", taskID),
		zap.String("image", record.OriginalImage))

	startTime := time.Now()

	// 更新镜像状态为同步中
	if err := database.DB.Model(&models.ImageSyncRecord{}).
		Where("id = ?", record.ID).
		Updates(map[string]interface{}{
			"sync_status": models.SyncStatusSyncing,
			"started_at":  &startTime,
		}).Error; err != nil {
		logger.Logger.Error("更新镜像状态失败", zap.Error(err))
		return
	}

	// 模拟同步过程 - 随机延迟2-5秒
	mockDuration := time.Duration(2+record.ID%4) * time.Second
	time.Sleep(mockDuration)

	// 生成ACR镜像地址
	acrImage := utils.GenerateACRImageWithArchitecture(record.OriginalImage, record.Tag, record.Architecture)

	// 检测目标镜像是否存在
	exists := utils.CheckImageExistsInRegistry(acrImage)

	completedTime := time.Now()
	duration := int64(completedTime.Sub(startTime).Seconds())

	if exists {
		// 目标镜像存在，设置为成功
		if err := database.DB.Model(&models.ImageSyncRecord{}).
			Where("id = ?", record.ID).
			Updates(map[string]interface{}{
				"sync_status":  models.SyncStatusSuccess,
				"completed_at": &completedTime,
				"duration":     duration,
				"acr_image":    acrImage,
			}).Error; err != nil {
			logger.Logger.Error("更新镜像完成状态失败", zap.Error(err))
		}

		logger.Logger.Info("模拟镜像同步完成",
			zap.String("task_id", taskID),
			zap.String("image", record.OriginalImage),
			zap.String("acr_image", acrImage),
			zap.Int64("duration", duration))
	} else {
		// 目标镜像不存在，设置为失败
		errorMessage := "目标镜像不存在于注册表中"
		if err := database.DB.Model(&models.ImageSyncRecord{}).
			Where("id = ?", record.ID).
			Updates(map[string]interface{}{
				"sync_status":   models.SyncStatusFailed,
				"completed_at":  &completedTime,
				"duration":      duration,
				"acr_image":     acrImage,
				"error_message": errorMessage,
			}).Error; err != nil {
			logger.Logger.Error("更新镜像失败状态失败", zap.Error(err))
		}

		logger.Logger.Warn("模拟镜像同步失败",
			zap.String("task_id", taskID),
			zap.String("image", record.OriginalImage),
			zap.String("acr_image", acrImage),
			zap.String("error", errorMessage),
			zap.Int64("duration", duration))
	}

	// 更新批量任务进度
	h.updateBatchTaskProgress(taskID)
}

// calculateEstimatedTime 计算预估时间（秒）
func (h *SyncHandler) calculateEstimatedTime(imageCount, maxConcurrent int) int {
	// 每个镜像平均需要30秒
	avgTimePerImage := 30
	// 考虑并发处理
	totalTime := (imageCount * avgTimePerImage) / maxConcurrent
	if totalTime < avgTimePerImage {
		totalTime = avgTimePerImage
	}
	return totalTime
}

// handleBatchSyncError 处理批量同步错误
func (h *SyncHandler) handleBatchSyncError(taskID, errorMessage string) {
	logger.Logger.Error("批量同步任务失败",
		zap.String("task_id", taskID),
		zap.String("error", errorMessage))

	// 更新任务状态为失败
	now := time.Now()
	if err := database.DB.Model(&models.SyncTask{}).
		Where("task_id = ?", taskID).
		Updates(map[string]interface{}{
			"status":        models.TaskStatusFailed,
			"completed_at":  &now,
			"error_message": errorMessage,
		}).Error; err != nil {
		logger.Logger.Error("更新任务失败状态失败", zap.Error(err))
	}
}

// updateBatchTaskFinalStatus 更新批量任务最终状态
func (h *SyncHandler) updateBatchTaskFinalStatus(taskID string) {
	// 统计各状态的镜像数量
	var stats struct {
		Total   int64
		Success int64
		Failed  int64
		Pending int64
	}

	database.DB.Model(&models.ImageSyncRecord{}).
		Where("task_id = ?", taskID).
		Count(&stats.Total)

	database.DB.Model(&models.ImageSyncRecord{}).
		Where("task_id = ? AND sync_status = ?", taskID, models.SyncStatusSuccess).
		Count(&stats.Success)

	database.DB.Model(&models.ImageSyncRecord{}).
		Where("task_id = ? AND sync_status = ?", taskID, models.SyncStatusFailed).
		Count(&stats.Failed)

	database.DB.Model(&models.ImageSyncRecord{}).
		Where("task_id = ? AND sync_status = ?", taskID, models.SyncStatusPending).
		Count(&stats.Pending)

	// 确定最终状态
	var finalStatus string
	var errorMessage string

	if stats.Pending > 0 {
		finalStatus = models.TaskStatusRunning
	} else if stats.Failed > 0 {
		if stats.Success > 0 {
			finalStatus = models.TaskStatusPartialSuccess
			errorMessage = fmt.Sprintf("部分成功: %d成功, %d失败", stats.Success, stats.Failed)
		} else {
			finalStatus = models.TaskStatusFailed
			errorMessage = fmt.Sprintf("全部失败: %d失败", stats.Failed)
		}
	} else {
		finalStatus = models.TaskStatusSuccess
	}

	// 更新任务状态
	now := time.Now()
	updates := map[string]interface{}{
		"status":           finalStatus,
		"completed_images": stats.Success,
		"failed_images":    stats.Failed,
		"progress":         float64(stats.Success+stats.Failed) / float64(stats.Total) * 100,
	}

	if finalStatus != models.TaskStatusRunning {
		updates["completed_at"] = &now
	}

	if errorMessage != "" {
		updates["error_message"] = errorMessage
	}

	if err := database.DB.Model(&models.SyncTask{}).
		Where("task_id = ?", taskID).
		Updates(updates).Error; err != nil {
		logger.Logger.Error("更新任务最终状态失败", zap.Error(err))
	}

	logger.Logger.Info("批量任务状态已更新",
		zap.String("task_id", taskID),
		zap.String("status", string(finalStatus)),
		zap.Int64("success", stats.Success),
		zap.Int64("failed", stats.Failed))
}

// updateBatchTaskProgress 更新批量任务进度
func (h *SyncHandler) updateBatchTaskProgress(taskID string) {
	// 统计完成的镜像数量
	var completed, total int64

	database.DB.Model(&models.ImageSyncRecord{}).
		Where("task_id = ?", taskID).
		Count(&total)

	database.DB.Model(&models.ImageSyncRecord{}).
		Where("task_id = ? AND sync_status IN (?)", taskID, []string{
			models.SyncStatusSuccess,
			models.SyncStatusFailed,
		}).Count(&completed)

	// 计算进度百分比
	var progress float64
	if total > 0 {
		progress = float64(completed) / float64(total) * 100
	}

	// 更新任务进度
	if err := database.DB.Model(&models.SyncTask{}).
		Where("task_id = ?", taskID).
		Update("progress", progress).Error; err != nil {
		logger.Logger.Error("更新任务进度失败", zap.Error(err))
	}
}

// updateTaskStatus 更新任务状态
func (h *SyncHandler) updateTaskStatus(taskID, status, errorMessage string) {
	updates := map[string]interface{}{
		"status": status,
	}

	if status == "completed" {
		now := time.Now()
		updates["completed_at"] = &now
	} else if status == "failed" {
		now := time.Now()
		updates["completed_at"] = &now
		if errorMessage != "" {
			updates["error_message"] = errorMessage
		}
	}

	if err := database.DB.Model(&models.SyncTask{}).
		Where("task_id = ?", taskID).
		Updates(updates).Error; err != nil {
		logger.Logger.Error("更新任务状态失败", zap.Error(err))
	}
}
