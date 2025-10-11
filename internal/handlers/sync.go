package handlers

import (
	"context"
	"fmt"
	"net/http"
	"strconv"
	"strings"
	"sync"
	"time"

	"docker-image-sync-platform/internal/database"
	"docker-image-sync-platform/internal/logger"
	"docker-image-sync-platform/internal/models"
	"docker-image-sync-platform/internal/services"

	"github.com/gin-gonic/gin"
	"github.com/google/go-containerregistry/pkg/name"
	"github.com/google/go-containerregistry/pkg/v1/remote"
	"github.com/google/uuid"
	"go.uber.org/zap"
)

// SyncHandler 同步处理器
type SyncHandler struct {
	gitService    *services.GitService
	githubService *services.GitHubService
}

// NewSyncHandler 创建同步处理器
func NewSyncHandler(gitService *services.GitService, githubService *services.GitHubService) *SyncHandler {
	return &SyncHandler{
		gitService:    gitService,
		githubService: githubService,
	}
}

// SubmitBatchSync 提交批量同步任务
func (h *SyncHandler) SubmitBatchSync(c *gin.Context) {
	var req models.BatchSyncRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		logger.Logger.Error("解析批量同步请求参数失败", zap.Error(err))
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
		logger.Logger.Error("创建批量同步任务失败", zap.Error(err))
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
		
		record := &models.ImageSyncRecord{
			TaskID:        taskID,
			OriginalImage: originalImage,
			Tag:           tag,
			Architecture:  architecture,
			SyncStatus:    models.SyncStatusPending,
			InputOrder:    i + 1,
			Priority:      img.Priority,
			MaxRetries:    req.RetryCount,
		}

		if err := database.DB.Create(record).Error; err != nil {
			logger.Logger.Error("创建镜像同步记录失败", zap.Error(err))
			c.JSON(http.StatusInternalServerError, gin.H{"error": "创建镜像记录失败"})
			return
		}
	}

	// 异步处理批量同步任务
	go h.processSyncTask(taskID)

	// 计算预计完成时间
	estimatedMinutes := len(req.Images) * 3 / req.MaxConcurrent
	estimatedCompletion := time.Now().Add(time.Duration(estimatedMinutes) * time.Minute)

	c.JSON(http.StatusOK, gin.H{
		"task_id":              taskID,
		"status":               models.TaskStatusPending,
		"total_images":         len(req.Images),
		"max_concurrent":       req.MaxConcurrent,
		"estimated_completion": estimatedCompletion.Format("2006-01-02 15:04:05"),
		"message":              "批量同步任务已提交，正在处理中",
	})
}

// SubmitSync 提交同步任务
func (h *SyncHandler) SubmitSync(c *gin.Context) {
	var req models.SyncRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		logger.Logger.Error("解析同步请求参数失败", zap.Error(err))
		c.JSON(http.StatusBadRequest, gin.H{"error": "请求参数格式错误"})
		return
	}

	if len(req.Images) == 0 {
		c.JSON(http.StatusBadRequest, gin.H{"error": "镜像列表不能为空"})
		return
	}

	// 生成任务ID
	taskID := uuid.New().String()

	// 创建同步任务记录
	task := &models.SyncTask{
		TaskID:        taskID,
		Status:        models.TaskStatusPending,
		MaxConcurrent: 1, // 单个同步
		TotalImages:   len(req.Images),
		Description:   req.Description,
	}

	// 构建镜像JSON字符串
	var imageStrings []string
	for _, img := range req.Images {
		imageStrings = append(imageStrings, img)
	}
	task.ImagesJSON = strings.Join(imageStrings, "\n")

	if err := database.DB.Create(task).Error; err != nil {
		logger.Logger.Error("创建同步任务失败", zap.Error(err))
		c.JSON(http.StatusInternalServerError, gin.H{"error": "创建同步任务失败"})
		return
	}

	// 为每个镜像创建同步记录
	for i, imageStr := range req.Images {
		// 解析镜像名称和标签
		originalImage, tag := h.parseImageNameAndTag(imageStr)
		
		record := &models.ImageSyncRecord{
			TaskID:        taskID,
			OriginalImage: originalImage,
			Tag:           tag,
			Architecture:  req.Architecture,
			SyncStatus:    models.SyncStatusPending,
			InputOrder:    i + 1,
		}

		if err := database.DB.Create(record).Error; err != nil {
			logger.Logger.Error("创建镜像同步记录失败", zap.Error(err))
			c.JSON(http.StatusInternalServerError, gin.H{"error": "创建镜像记录失败"})
			return
		}
	}

	// 异步处理同步任务
	go h.processSyncTask(taskID)

	// 计算预计完成时间（每个镜像预计3分钟）
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

// GetSyncStatus 获取同步状态
func (h *SyncHandler) GetSyncStatus(c *gin.Context) {
	taskID := c.Param("taskId")
	if taskID == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "任务ID不能为空"})
		return
	}

	// 查询任务信息
	var task models.SyncTask
	if err := database.DB.Where("task_id = ?", taskID).First(&task).Error; err != nil {
		logger.Logger.Error("查询同步任务失败", zap.Error(err))
		c.JSON(http.StatusNotFound, gin.H{"error": "任务不存在"})
		return
	}

	// 查询镜像记录
	var records []models.ImageSyncRecord
	if err := database.DB.Where("task_id = ?", taskID).
		Order("input_order ASC").
		Find(&records).Error; err != nil {
		logger.Logger.Error("查询镜像记录失败", zap.Error(err))
		c.JSON(http.StatusInternalServerError, gin.H{"error": "查询镜像记录失败"})
		return
	}

	// 统计状态
	var pendingCount, syncingCount, successCount, failedCount int
	for _, record := range records {
		switch record.SyncStatus {
		case models.SyncStatusPending:
			pendingCount++
		case models.SyncStatusSyncing:
			syncingCount++
		case models.SyncStatusSuccess:
			successCount++
		case models.SyncStatusFailed:
			failedCount++
		}
	}

	response := gin.H{
		"task_id":           task.TaskID,
		"status":            task.Status,
		"total_images":      task.TotalImages,
		"completed_images":  task.CompletedImages,
		"failed_images":     task.FailedImages,
		"progress":          task.Progress,
		"github_action_url": task.GitHubActionURL,
		"github_run_id":     task.GitHubRunID,
		"commit_sha":        task.CommitSHA,
		"started_at":        task.StartedAt,
		"completed_at":      task.CompletedAt,
		"error_message":     task.ErrorMessage,
		"description":       task.Description,
		"created_at":        task.CreatedAt,
		"updated_at":        task.UpdatedAt,
		"images": gin.H{
			"pending": pendingCount,
			"syncing": syncingCount,
			"success": successCount,
			"failed":  failedCount,
			"records": records,
		},
	}

	c.JSON(http.StatusOK, response)
}

// GetBatchSyncStatus 获取批量同步状态 - 已废弃
func (h *SyncHandler) GetBatchSyncStatus(c *gin.Context) {
	logger.Logger.Warn("尝试调用已废弃的批量同步状态查询API")
	c.JSON(http.StatusGone, gin.H{
		"error": "批量同步功能已废弃，请使用模拟同步功能进行测试",
		"code":  "FEATURE_DEPRECATED",
	})
}

// GetSyncHistory 获取同步历史
func (h *SyncHandler) GetSyncHistory(c *gin.Context) {
	// 分页参数
	page := 1
	pageSize := 20
	
	if p := c.Query("page"); p != "" {
		if parsed, err := strconv.Atoi(p); err == nil && parsed > 0 {
			page = parsed
		}
	}
	
	if ps := c.Query("page_size"); ps != "" {
		if parsed, err := strconv.Atoi(ps); err == nil && parsed > 0 && parsed <= 100 {
			pageSize = parsed
		}
	}

	// 状态过滤
	status := c.Query("status")
	
	// 构建查询
	query := database.DB.Model(&models.SyncTask{})
	if status != "" {
		query = query.Where("status = ?", status)
	}

	// 获取总数
	var total int64
	if err := query.Count(&total).Error; err != nil {
		logger.Logger.Error("查询同步历史总数失败", zap.Error(err))
		c.JSON(http.StatusInternalServerError, gin.H{"error": "查询失败"})
		return
	}

	// 获取数据
	var tasks []models.SyncTask
	offset := (page - 1) * pageSize
	if err := query.Order("created_at DESC").
		Limit(pageSize).
		Offset(offset).
		Find(&tasks).Error; err != nil {
		logger.Logger.Error("查询同步历史失败", zap.Error(err))
		c.JSON(http.StatusInternalServerError, gin.H{"error": "查询失败"})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"total":     total,
		"page":      page,
		"page_size": pageSize,
		"data":      tasks,
	})
}

// processSyncTask 处理同步任务
func (h *SyncHandler) processSyncTask(taskID string) {
	logger.Logger.Info("开始处理同步任务", zap.String("task_id", taskID))

	// 更新任务状态为运行中
	now := time.Now()
	if err := database.DB.Model(&models.SyncTask{}).
		Where("task_id = ?", taskID).
		Updates(map[string]interface{}{
			"status":     models.TaskStatusRunning,
			"started_at": &now,
		}).Error; err != nil {
		logger.Logger.Error("更新任务状态失败", zap.Error(err))
		return
	}

	// 获取任务信息
	var task models.SyncTask
	if err := database.DB.Where("task_id = ?", taskID).First(&task).Error; err != nil {
		logger.Logger.Error("查询任务失败", zap.Error(err))
		h.handleSyncError(taskID, fmt.Sprintf("查询任务失败: %v", err))
		return
	}

	// 获取所有待同步的镜像记录
	var records []models.ImageSyncRecord
	if err := database.DB.Where("task_id = ? AND sync_status = ?", taskID, models.SyncStatusPending).
		Order("input_order ASC").
		Find(&records).Error; err != nil {
		h.handleSyncError(taskID, fmt.Sprintf("查询镜像记录失败: %v", err))
		return
	}

	// 更新镜像状态为同步中
	for _, record := range records {
		if err := database.DB.Model(&models.ImageSyncRecord{}).
			Where("id = ?", record.ID).
			Updates(map[string]interface{}{
				"sync_status": models.SyncStatusSyncing,
				"started_at":  &now,
			}).Error; err != nil {
			logger.Logger.Error("更新镜像状态失败", zap.Error(err))
		}
	}

	// 更新images.txt文件并推送到Git
	commitSHA, err := h.updateImagesFile(records)
	if err != nil {
		h.handleSyncError(taskID, fmt.Sprintf("更新images.txt失败: %v", err))
		return
	}

	// 更新任务的commit SHA
	if err := database.DB.Model(&models.SyncTask{}).
		Where("task_id = ?", taskID).
		Update("commit_sha", commitSHA).Error; err != nil {
		logger.Logger.Error("更新commit SHA失败", zap.Error(err))
	}

	// 监控GitHub Actions
	go h.monitorGitHubActions(taskID, commitSHA)

	logger.Logger.Info("同步任务已提交到Git", zap.String("task_id", taskID), zap.String("commit_sha", commitSHA))
}

// updateImagesFile 更新images.txt文件
func (h *SyncHandler) updateImagesFile(records []models.ImageSyncRecord) (string, error) {
	var imageLines []string
	
	// 获取任务信息以确定是否为批量同步
	var task models.SyncTask
	if len(records) > 0 {
		if err := database.DB.Where("task_id = ?", records[0].TaskID).First(&task).Error; err != nil {
			logger.Logger.Error("查询任务信息失败", zap.Error(err))
			return "", err
		}
	}
	
	for _, record := range records {
		imageLine := record.OriginalImage
		if record.Tag != "" {
			imageLine = imageLine + ":" + record.Tag
		}
		// 只有非AMD64架构才添加--platform前缀
		if record.Architecture != "" && record.Architecture != "amd64" {
			imageLine = "--platform=linux/" + record.Architecture + " " + imageLine
		}
		imageLines = append(imageLines, imageLine)
	}

	return h.gitService.UpdateImagesFile(imageLines)
}

// monitorGitHubActions 监控GitHub Actions执行状态
func (h *SyncHandler) monitorGitHubActions(taskID, commitSHA string) {
	logger.Logger.Info("开始监控GitHub Actions", zap.String("task_id", taskID), zap.String("commit_sha", commitSHA))

	// 等待GitHub Actions开始执行
	time.Sleep(30 * time.Second)

	// 获取GitHub Actions运行信息
	runID, runURL, err := h.githubService.GetWorkflowRun(commitSHA)
	if err != nil {
		logger.Logger.Error("获取GitHub Actions运行信息失败", zap.Error(err))
		h.handleSyncError(taskID, fmt.Sprintf("获取GitHub Actions运行信息失败: %v", err))
		return
	}

	// 更新任务的GitHub信息
	if err := database.DB.Model(&models.SyncTask{}).
		Where("task_id = ?", taskID).
		Updates(map[string]interface{}{
			"github_run_id":     runID,
			"github_action_url": runURL,
		}).Error; err != nil {
		logger.Logger.Error("更新GitHub信息失败", zap.Error(err))
	}

	// 监控GitHub Actions执行状态
	maxWaitTime := 30 * time.Minute
	checkInterval := 30 * time.Second
	startTime := time.Now()

	for time.Since(startTime) < maxWaitTime {
		status, err := h.githubService.GetWorkflowRunStatus(runID)
		if err != nil {
			logger.Logger.Error("获取GitHub Actions状态失败", zap.Error(err))
			time.Sleep(checkInterval)
			continue
		}

		logger.Logger.Info("GitHub Actions状态", 
			zap.String("task_id", taskID),
			zap.String("run_id", runID),
			zap.String("status", status))

		if status == "completed" {
			conclusion, err := h.githubService.GetWorkflowRunConclusion(runID)
			if err != nil {
				logger.Logger.Error("获取GitHub Actions结论失败", zap.Error(err))
				h.handleSyncError(taskID, fmt.Sprintf("获取GitHub Actions结论失败: %v", err))
				return
			}

			if conclusion == "success" {
				h.handleSyncSuccess(taskID)
			} else {
				h.handleSyncError(taskID, fmt.Sprintf("GitHub Actions执行失败，结论: %s", conclusion))
			}
			return
		}

		if status == "cancelled" || status == "failure" {
			h.handleSyncError(taskID, fmt.Sprintf("GitHub Actions执行失败，状态: %s", status))
			return
		}

		time.Sleep(checkInterval)
	}

	// 超时处理
	h.handleSyncError(taskID, "GitHub Actions执行超时")
}

// handleSyncSuccess 处理同步成功
func (h *SyncHandler) handleSyncSuccess(taskID string) {
	logger.Logger.Info("同步任务成功", zap.String("task_id", taskID))

	// 获取任务的镜像记录
	var records []models.ImageSyncRecord
	if err := database.DB.Where("task_id = ?", taskID).Find(&records).Error; err != nil {
		logger.Logger.Error("查询镜像记录失败", zap.Error(err))
		return
	}

	// 验证每个镜像是否成功同步到ACR
	successCount := 0
	for _, record := range records {
		// 生成ACR镜像地址
		acrImage := h.generateACRImageWithArchitecture(record.OriginalImage, record.Tag, record.Architecture)
		
		// 检查镜像是否存在于ACR
		exists := h.checkImageExistsInRegistry(acrImage)
		
		completedTime := time.Now()
		var duration int64
		if record.StartedAt != nil {
			duration = int64(completedTime.Sub(*record.StartedAt).Seconds())
		}

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
func (h *SyncHandler) handleSyncError(taskID, errorMessage string) {
	logger.Logger.Error("同步任务失败", 
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

	// 更新所有相关镜像记录为失败
	if err := database.DB.Model(&models.ImageSyncRecord{}).
		Where("task_id = ? AND sync_status IN (?)", taskID, []string{
			models.SyncStatusPending,
			models.SyncStatusSyncing,
		}).
		Updates(map[string]interface{}{
			"sync_status":   models.SyncStatusFailed,
			"completed_at":  &now,
			"error_message": errorMessage,
		}).Error; err != nil {
		logger.Logger.Error("更新镜像失败状态失败", zap.Error(err))
	}
}

// generateACRImage 生成阿里云ACR镜像地址
func (h *SyncHandler) generateACRImage(originalImage, tag string) string {
	return h.generateACRImageWithArchitecture(originalImage, tag, "")
}

// generateACRImageWithArchitecture 生成带架构信息的阿里云ACR镜像地址
func (h *SyncHandler) generateACRImageWithArchitecture(originalImage, tag, architecture string) string {
	// 从配置中获取阿里云信息
	var registryConfig models.SystemConfig
	database.DB.Where("config_key = ?", "aliyun_registry_prefix").First(&registryConfig)
	
	registry := registryConfig.ConfigValue
	if registry == "" {
		registry = "registry.cn-hangzhou.aliyuncs.com"
	}

	// 从配置中获取阿里云命名空间
	var namespaceConfig models.SystemConfig
	database.DB.Where("config_key = ?", "aliyun_namespace").First(&namespaceConfig)
	
	namespace := namespaceConfig.ConfigValue
	if namespace == "" {
		namespace = "lpx03" // 使用与GitHub Action一致的命名空间
	}

	// 解析镜像名称
	imageName := originalImage
	if strings.Contains(imageName, "/") {
		parts := strings.Split(imageName, "/")
		imageName = parts[len(parts)-1]
	}
	
	// 生成架构后缀，将架构信息添加到tag后面
	architectureSuffix := ""
	if architecture != "" && architecture != "amd64" {
		// 将简化的架构名转换为完整的平台字符串
		var platform string
		switch architecture {
		case "arm64":
			platform = "linux/arm64"
		case "arm":
			platform = "linux/arm"
		case "386":
			platform = "linux/386"
		default:
			// 如果已经是完整格式（如linux/arm64），直接使用
			if strings.Contains(architecture, "/") {
				platform = architecture
			} else {
				platform = "linux/" + architecture
			}
		}
		// 将 linux/arm64 转换为 -linux-arm64
		architectureSuffix = "-" + strings.ReplaceAll(platform, "/", "-")
	}
	
	// 构建最终的镜像名称和标签
	finalTag := tag
	if finalTag == "" {
		finalTag = "latest"
	}
	
	// 构建标签：tag + architectureSuffix
	finalTagWithArch := finalTag + architectureSuffix
	
	return fmt.Sprintf("%s/%s/%s:%s", registry, namespace, imageName, finalTagWithArch)
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

// checkImageExistsInRegistry 检测镜像是否存在于注册表中
func (h *SyncHandler) checkImageExistsInRegistry(imageRef string) bool {
	// 解析镜像引用
	ref, err := name.ParseReference(imageRef)
	if err != nil {
		logger.Logger.Error("解析镜像引用失败", 
			zap.Error(err), 
			zap.String("image_ref", imageRef))
		return false
	}

	// 创建带超时的上下文
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	// 尝试获取镜像的manifest
	_, err = remote.Head(ref, remote.WithContext(ctx))
	if err != nil {
		// 如果是404错误，说明镜像不存在
		if strings.Contains(err.Error(), "404") || strings.Contains(err.Error(), "not found") {
			logger.Logger.Debug("镜像不存在", 
				zap.String("image_ref", imageRef))
			return false
		}
		// 其他错误也认为镜像不存在
		logger.Logger.Warn("检测镜像存在性失败", 
			zap.Error(err), 
			zap.String("image_ref", imageRef))
		return false
	}

	logger.Logger.Debug("镜像存在", 
		zap.String("image_ref", imageRef))
	return true
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
	acrImage := h.generateACRImageWithArchitecture(record.OriginalImage, record.Tag, record.Architecture)

	// 检测目标镜像是否存在
	exists := h.checkImageExistsInRegistry(acrImage)
	
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
		Total     int64
		Success   int64
		Failed    int64
		Pending   int64
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
		"status":          finalStatus,
		"completed_images": stats.Success,
		"failed_images":   stats.Failed,
		"progress":        float64(stats.Success+stats.Failed) / float64(stats.Total) * 100,
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