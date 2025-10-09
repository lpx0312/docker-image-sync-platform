package handlers

import (
	"fmt"
	"net/http"
	"strconv"
	"strings"
	"time"

	"docker-image-sync-platform/internal/database"
	"docker-image-sync-platform/internal/logger"
	"docker-image-sync-platform/internal/models"
	"docker-image-sync-platform/internal/services"

	"github.com/gin-gonic/gin"
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
	for _, img := range req.Images {
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
			OriginalImage: originalImage,
			Tag:           tag,
			Architecture:  architecture,
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

	// 异步执行批量同步操作
	go h.processBatchSyncTask(taskID)

	// 计算预估时间
	estimatedTime := h.calculateEstimatedTime(len(req.Images), req.MaxConcurrent)

	logger.Logger.Info("批量同步任务已提交", 
		zap.String("task_id", taskID),
		zap.Int("image_count", len(req.Images)),
		zap.Int("max_concurrent", req.MaxConcurrent))

	c.JSON(http.StatusOK, models.BatchSyncResponse{
		TaskID:        taskID,
		Message:       "批量同步任务已提交，正在处理中...",
		ImageCount:    len(req.Images),
		EstimatedTime: estimatedTime,
	})
}

// SubmitSync 提交同步任务
func (h *SyncHandler) SubmitSync(c *gin.Context) {
	var req models.ImageRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		logger.Logger.Error("解析请求参数失败", zap.Error(err))
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
		TaskID:     taskID,
		ImagesJSON: strings.Join(req.Images, "\n"),
		Status:     models.TaskStatusPending,
	}

	if err := database.DB.Create(task).Error; err != nil {
		logger.Logger.Error("创建同步任务失败", zap.Error(err))
		c.JSON(http.StatusInternalServerError, gin.H{"error": "创建同步任务失败"})
		return
	}

	// 为每个镜像创建同步记录
	for _, image := range req.Images {
		// 解析镜像信息
		originalImage, tag, architecture := parseImageInfo(image)
		
		// 如果请求中指定了架构，使用请求中的架构
		if req.Architecture != "" {
			architecture = req.Architecture
		}
		
		// 如果架构为空，设置默认值
		if architecture == "" {
			architecture = "amd64"
		}
		
		record := &models.ImageSyncRecord{
			OriginalImage: originalImage,
			Tag:           tag,
			Architecture:  architecture,
			SyncStatus:    models.SyncStatusPending,
			TaskID:        taskID,
		}

		if err := database.DB.Create(record).Error; err != nil {
			logger.Logger.Error("创建镜像同步记录失败", 
				zap.Error(err), 
				zap.String("image", originalImage))
		}
	}

	// 异步执行同步操作
	go h.processSyncTask(taskID, req.Images)

	logger.Logger.Info("同步任务已提交", 
		zap.String("task_id", taskID),
		zap.Int("image_count", len(req.Images)))

	c.JSON(http.StatusOK, models.SyncResponse{
		TaskID:  taskID,
		Message: "同步任务已提交，正在处理中...",
	})
}

// GetSyncStatus 获取同步状态
func (h *SyncHandler) GetSyncStatus(c *gin.Context) {
	taskID := c.Param("taskId")
	if taskID == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "任务ID不能为空"})
		return
	}

	var task models.SyncTask
	if err := database.DB.Where("task_id = ?", taskID).First(&task).Error; err != nil {
		logger.Logger.Error("查询同步任务失败", zap.Error(err), zap.String("task_id", taskID))
		c.JSON(http.StatusNotFound, gin.H{"error": "任务不存在"})
		return
	}

	// 如果任务状态为running且有GitHub Run ID，检查实际状态
	if task.Status == models.TaskStatusRunning && task.GitHubRunID != "" {
		h.CheckAndUpdateTaskStatus(&task)
	}

	// 获取镜像列表
	images := strings.Split(task.ImagesJSON, "\n")
	if len(images) == 1 && images[0] == "" {
		images = []string{}
	}

	// 获取第一个镜像记录的详细信息
	var record models.ImageSyncRecord
	var sourceImage, targetImage, architecture string
	if err := database.DB.Where("task_id = ?", taskID).First(&record).Error; err == nil {
		sourceImage = record.OriginalImage
		if record.Tag != "" {
			sourceImage = sourceImage + ":" + record.Tag
		}
		
		// 生成目标镜像地址
		targetImage = record.ACRImage
		if targetImage == "" {
			targetImage = h.generateACRImageWithArchitecture(record.OriginalImage, record.Tag, record.Architecture)
		} else if !strings.Contains(targetImage, ":") {
			tag := record.Tag
			if tag == "" {
				tag = "latest"
			}
			targetImage = fmt.Sprintf("%s:%s", targetImage, tag)
		}
		
		architecture = record.Architecture
		if architecture == "" {
			architecture = "amd64"
		}
	}

	response := models.TaskStatusResponse{
		TaskID:          task.TaskID,
		Status:          task.Status,
		SourceImage:     sourceImage,
		TargetImage:     targetImage,
		Architecture:    architecture,
		GitHubActionURL: task.GitHubActionURL,
		StartedAt:       task.StartedAt,
		CompletedAt:     task.CompletedAt,
		ErrorMessage:    task.ErrorMessage,
		Images:          images,
	}

	c.JSON(http.StatusOK, response)
}

// GetBatchSyncStatus 获取批量同步状态
func (h *SyncHandler) GetBatchSyncStatus(c *gin.Context) {
	taskID := c.Param("taskId")
	if taskID == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "任务ID不能为空"})
		return
	}

	var task models.SyncTask
	if err := database.DB.Where("task_id = ?", taskID).First(&task).Error; err != nil {
		logger.Logger.Error("查询批量同步任务失败", zap.Error(err), zap.String("task_id", taskID))
		c.JSON(http.StatusNotFound, gin.H{"error": "任务不存在"})
		return
	}

	// 获取所有镜像同步记录
	var records []models.ImageSyncRecord
	if err := database.DB.Where("task_id = ?", taskID).
		Order("priority DESC, created_at ASC").
		Find(&records).Error; err != nil {
		logger.Logger.Error("查询镜像同步记录失败", zap.Error(err))
		c.JSON(http.StatusInternalServerError, gin.H{"error": "查询镜像记录失败"})
		return
	}

	// 构建镜像详情列表
	var imageDetails []models.ImageSyncDetailResponse
	for _, record := range records {
		// 生成目标镜像地址
		targetImage := record.ACRImage
		if targetImage == "" {
			targetImage = h.generateACRImageWithArchitecture(record.OriginalImage, record.Tag, record.Architecture)
		} else if !strings.Contains(targetImage, ":") {
			tag := record.Tag
			if tag == "" {
				tag = "latest"
			}
			targetImage = fmt.Sprintf("%s:%s", targetImage, tag)
		}

		detail := models.ImageSyncDetailResponse{
			ID:            record.ID,
			OriginalImage: record.OriginalImage,
			ACRImage:      targetImage,
			Tag:           record.Tag,
			Architecture:  record.Architecture,
			SyncStatus:    record.SyncStatus,
			ErrorMessage:  record.ErrorMessage,
			Priority:      record.Priority,
			RetryCount:    record.RetryCount,
			MaxRetries:    record.MaxRetries,
			StartedAt:     record.StartedAt,
			CompletedAt:   record.CompletedAt,
			Duration:      record.Duration,
			ImageSize:     record.ImageSize,
		}
		imageDetails = append(imageDetails, detail)
	}

	// 计算预估剩余时间
	estimatedTime := h.calculateRemainingTime(task.TotalImages, task.CompletedImages, task.MaxConcurrent)

	response := models.BatchTaskStatusResponse{
		TaskID:          task.TaskID,
		Status:          task.Status,
		Description:     task.Description,
		TotalImages:     task.TotalImages,
		CompletedImages: task.CompletedImages,
		FailedImages:    task.FailedImages,
		Progress:        task.Progress,
		MaxConcurrent:   task.MaxConcurrent,
		AutoRetry:       task.AutoRetry,
		CurrentRetry:    task.CurrentRetry,
		RetryCount:      task.RetryCount,
		GitHubActionURL: task.GitHubActionURL,
		StartedAt:       task.StartedAt,
		CompletedAt:     task.CompletedAt,
		ErrorMessage:    task.ErrorMessage,
		ImageDetails:    imageDetails,
		EstimatedTime:   estimatedTime,
	}

	c.JSON(http.StatusOK, response)
}

// SyncHistoryItem 同步历史项
type SyncHistoryItem struct {
	TaskID          string     `json:"task_id"`
	SourceImage     string     `json:"source_image"`
	TargetImage     string     `json:"target_image"`
	Architecture    string     `json:"architecture"`
	Status          string     `json:"status"`
	GitHubActionURL string     `json:"github_action_url"`
	StartedAt       *time.Time `json:"started_at"`
	CompletedAt     *time.Time `json:"completed_at"`
	ErrorMessage    string     `json:"error_message"`
	CreatedAt       time.Time  `json:"created_at"`
}

// GetSyncHistory 获取同步历史
func (h *SyncHandler) GetSyncHistory(c *gin.Context) {
	page, _ := strconv.Atoi(c.DefaultQuery("page", "1"))
	pageSize, _ := strconv.Atoi(c.DefaultQuery("page_size", "10"))
	
	if page < 1 {
		page = 1
	}
	if pageSize < 1 || pageSize > 100 {
		pageSize = 10
	}

	offset := (page - 1) * pageSize

	var tasks []models.SyncTask
	var total int64

	// 查询总数
	database.DB.Model(&models.SyncTask{}).Count(&total)

	// 查询数据
	if err := database.DB.Order("created_at DESC").
		Offset(offset).
		Limit(pageSize).
		Find(&tasks).Error; err != nil {
		logger.Logger.Error("查询同步历史失败", zap.Error(err))
		c.JSON(http.StatusInternalServerError, gin.H{"error": "查询同步历史失败"})
		return
	}

	// 构建历史项列表
	var historyItems []SyncHistoryItem
	for _, task := range tasks {
		// 获取该任务的第一个镜像记录作为代表
		var record models.ImageSyncRecord
		database.DB.Where("task_id = ?", task.TaskID).First(&record)

		// 构建源镜像地址
		sourceImage := record.OriginalImage
		if record.Tag != "" {
			sourceImage = fmt.Sprintf("%s:%s", record.OriginalImage, record.Tag)
		} else {
			// 如果没有标签，默认添加latest
			sourceImage = fmt.Sprintf("%s:latest", record.OriginalImage)
		}

		// 目标镜像地址
		targetImage := record.ACRImage
		if targetImage == "" {
			// 如果没有ACR地址，使用generateACRImageWithArchitecture生成
			targetImage = h.generateACRImageWithArchitecture(record.OriginalImage, record.Tag, record.Architecture)
		} else if !strings.Contains(targetImage, ":") {
			// 如果ACR地址没有标签，添加标签
			tag := record.Tag
			if tag == "" {
				tag = "latest"
			}
			targetImage = fmt.Sprintf("%s:%s", targetImage, tag)
		}

		// 架构信息，如果为空则默认为amd64
		architecture := record.Architecture
		if architecture == "" {
			architecture = "amd64"
		}

		historyItem := SyncHistoryItem{
			TaskID:          task.TaskID,
			SourceImage:     sourceImage,
			TargetImage:     targetImage,
			Architecture:    architecture,
			Status:          task.Status,
			GitHubActionURL: task.GitHubActionURL,
			StartedAt:       task.StartedAt,
			CompletedAt:     task.CompletedAt,
			ErrorMessage:    task.ErrorMessage,
			CreatedAt:       task.CreatedAt,
		}

		historyItems = append(historyItems, historyItem)
	}

	c.JSON(http.StatusOK, gin.H{
		"total": total,
		"data":  historyItems,
		"page":  page,
		"page_size": pageSize,
	})
}

// processSyncTask 处理同步任务
func (h *SyncHandler) processSyncTask(taskID string, images []string) {
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

	// 更新镜像状态为同步中
	database.DB.Model(&models.ImageSyncRecord{}).
		Where("task_id = ?", taskID).
		Update("sync_status", models.SyncStatusSyncing)

	// 根据架构信息构建正确的镜像字符串
	var processedImages []string
	var records []models.ImageSyncRecord
	if err := database.DB.Where("task_id = ?", taskID).Find(&records).Error; err != nil {
		h.handleSyncError(taskID, fmt.Sprintf("查询镜像记录失败: %v", err))
		return
	}

	for _, record := range records {
		imageStr := record.OriginalImage
		if record.Tag != "" {
			imageStr = imageStr + ":" + record.Tag
		}
		
		// 如果是arm64架构，添加platform参数
		if record.Architecture == "arm64" {
			imageStr = "--platform=linux/arm64 " + imageStr
		}
		
		processedImages = append(processedImages, imageStr)
	}

	// 执行Git操作
	commitSHA, err := h.gitService.UpdateImagesFile(processedImages)
	if err != nil {
		h.handleSyncError(taskID, fmt.Sprintf("Git操作失败: %v", err))
		return
	}

	// 更新提交SHA
	database.DB.Model(&models.SyncTask{}).
		Where("task_id = ?", taskID).
		Update("commit_sha", commitSHA)

	// 等待GitHub同步并监控Actions
	go h.monitorGitHubActions(taskID, commitSHA)

	logger.Logger.Info("同步任务Git操作完成", 
		zap.String("task_id", taskID),
		zap.String("commit_sha", commitSHA))
}

// monitorGitHubActions 监控GitHub Actions
func (h *SyncHandler) monitorGitHubActions(taskID, commitSHA string) {
	logger.Logger.Info("开始监控GitHub Actions", 
		zap.String("task_id", taskID),
		zap.String("commit_sha", commitSHA))

	// 等待一段时间让GitHub同步
	time.Sleep(30 * time.Second)

	// 查询GitHub Actions状态
	runID, actionURL, err := h.githubService.GetWorkflowRun(commitSHA)
	if err != nil {
		h.handleSyncError(taskID, fmt.Sprintf("查询GitHub Actions失败: %v", err))
		return
	}

	// 更新GitHub Action信息
	database.DB.Model(&models.SyncTask{}).
		Where("task_id = ?", taskID).
		Updates(map[string]interface{}{
			"github_run_id":     runID,
			"github_action_url": actionURL,
		})

	// 持续监控Actions状态
	for i := 0; i < 60; i++ { // 最多监控30分钟
		time.Sleep(30 * time.Second)

		status, err := h.githubService.GetWorkflowRunStatus(runID)
		if err != nil {
			logger.Logger.Error("查询Actions状态失败", zap.Error(err))
			continue
		}

		if status == "completed" {
			// Actions完成，检查结果
			conclusion, err := h.githubService.GetWorkflowRunConclusion(runID)
			if err != nil {
				h.handleSyncError(taskID, fmt.Sprintf("获取Actions结果失败: %v", err))
				return
			}

			if conclusion == "success" {
				h.handleSyncSuccess(taskID)
			} else {
				h.handleSyncError(taskID, fmt.Sprintf("GitHub Actions执行失败: %s", conclusion))
			}
			return
		}

		if status == "cancelled" || status == "failure" {
			h.handleSyncError(taskID, fmt.Sprintf("GitHub Actions执行失败: %s", status))
			return
		}
	}

	// 超时处理
	h.handleSyncError(taskID, "GitHub Actions监控超时")
}

// handleSyncSuccess 处理同步成功
func (h *SyncHandler) handleSyncSuccess(taskID string) {
	logger.Logger.Info("同步任务成功完成", zap.String("task_id", taskID))

	now := time.Now()
	
	// 更新任务状态
	database.DB.Model(&models.SyncTask{}).
		Where("task_id = ?", taskID).
		Updates(map[string]interface{}{
			"status":       models.TaskStatusCompleted,
			"completed_at": &now,
		})

	// 更新镜像状态并生成ACR地址
	var records []models.ImageSyncRecord
	database.DB.Where("task_id = ?", taskID).Find(&records)

	for _, record := range records {
		acrImage := h.generateACRImageWithArchitecture(record.OriginalImage, record.Tag, record.Architecture)
		
		database.DB.Model(&record).Updates(map[string]interface{}{
			"sync_status": models.SyncStatusSuccess,
			"acr_image":   acrImage,
		})
	}
}

// handleSyncError 处理同步错误
func (h *SyncHandler) handleSyncError(taskID, errorMsg string) {
	logger.Logger.Error("同步任务失败", 
		zap.String("task_id", taskID),
		zap.String("error", errorMsg))

	now := time.Now()

	// 更新任务状态
	database.DB.Model(&models.SyncTask{}).
		Where("task_id = ?", taskID).
		Updates(map[string]interface{}{
			"status":        models.TaskStatusFailed,
			"completed_at":  &now,
			"error_message": errorMsg,
		})

	// 更新镜像状态
	database.DB.Model(&models.ImageSyncRecord{}).
		Where("task_id = ?", taskID).
		Updates(map[string]interface{}{
			"sync_status":   models.SyncStatusFailed,
			"error_message": errorMsg,
		})
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

// CheckAndUpdateTaskStatus 检查并更新任务状态
func (h *SyncHandler) CheckAndUpdateTaskStatus(task *models.SyncTask) {
	if task.GitHubRunID == "" {
		// 如果没有GitHub Run ID但状态是running，说明任务启动失败
		logger.Logger.Warn("发现没有GitHub Run ID的running任务，标记为失败", 
			zap.String("task_id", task.TaskID))
		
		now := time.Now()
		task.Status = models.TaskStatusFailed
		task.CompletedAt = &now
		task.ErrorMessage = "任务启动失败，未获取到GitHub Action Run ID"
		
		// 更新所有相关镜像记录的状态
		database.DB.Model(&models.ImageSyncRecord{}).
			Where("task_id = ?", task.TaskID).
			Updates(map[string]interface{}{
				"sync_status": models.SyncStatusFailed,
				"error_message": "任务启动失败，未获取到GitHub Action Run ID",
			})
		
		// 保存更新到数据库
		if err := database.DB.Save(task).Error; err != nil {
			logger.Logger.Error("更新任务状态失败", 
				zap.Error(err), 
				zap.String("task_id", task.TaskID))
		} else {
			logger.Logger.Info("任务状态已更新为失败", 
				zap.String("task_id", task.TaskID))
		}
		return
	}

	// 检查任务是否超时（30分钟）
	now := time.Now()
	var taskStartTime time.Time
	if task.StartedAt != nil {
		taskStartTime = *task.StartedAt
	} else {
		taskStartTime = task.CreatedAt
	}
	
	timeoutDuration := 30 * time.Minute
	if now.Sub(taskStartTime) > timeoutDuration {
		logger.Logger.Warn("任务执行超时，标记为失败", 
			zap.String("task_id", task.TaskID),
			zap.String("run_id", task.GitHubRunID),
			zap.Duration("elapsed", now.Sub(taskStartTime)),
			zap.Duration("timeout", timeoutDuration))
		
		task.Status = models.TaskStatusFailed
		task.CompletedAt = &now
		task.ErrorMessage = fmt.Sprintf("任务执行超时（超过%v），自动标记为失败", timeoutDuration)
		
		// 更新所有相关镜像记录的状态
		database.DB.Model(&models.ImageSyncRecord{}).
			Where("task_id = ?", task.TaskID).
			Updates(map[string]interface{}{
				"sync_status": models.SyncStatusFailed,
				"error_message": fmt.Sprintf("任务执行超时（超过%v），自动标记为失败", timeoutDuration),
			})
		
		// 保存更新到数据库
		if err := database.DB.Save(task).Error; err != nil {
			logger.Logger.Error("更新超时任务状态失败", 
				zap.Error(err), 
				zap.String("task_id", task.TaskID))
		} else {
			logger.Logger.Info("超时任务状态已更新为失败", 
				zap.String("task_id", task.TaskID))
		}
		return
	}

	// 获取GitHub Action的实际状态
	runDetails, err := h.githubService.GetWorkflowRunDetails(task.GitHubRunID)
	if err != nil {
		logger.Logger.Error("获取GitHub Action状态失败", 
			zap.Error(err), 
			zap.String("task_id", task.TaskID),
			zap.String("run_id", task.GitHubRunID))
		return
	}

	logger.Logger.Info("检查GitHub Action状态", 
		zap.String("task_id", task.TaskID),
		zap.String("run_id", task.GitHubRunID),
		zap.String("status", runDetails.Status),
		zap.String("conclusion", runDetails.Conclusion))

	// 根据GitHub Action状态更新数据库
	var needUpdate bool
	completedAt := time.Now()

	switch runDetails.Status {
	case "completed":
		if runDetails.Conclusion == "success" {
			task.Status = models.TaskStatusCompleted
			task.CompletedAt = &completedAt
			needUpdate = true
			
			// 更新所有相关镜像记录的状态
			database.DB.Model(&models.ImageSyncRecord{}).
				Where("task_id = ?", task.TaskID).
				Update("sync_status", models.SyncStatusSuccess)
				
			logger.Logger.Info("任务同步成功", zap.String("task_id", task.TaskID))
		} else {
			task.Status = models.TaskStatusFailed
			task.CompletedAt = &completedAt
			task.ErrorMessage = fmt.Sprintf("GitHub Action执行失败: %s", runDetails.Conclusion)
			needUpdate = true
			
			// 更新所有相关镜像记录的状态
			database.DB.Model(&models.ImageSyncRecord{}).
				Where("task_id = ?", task.TaskID).
				Updates(map[string]interface{}{
					"sync_status": models.SyncStatusFailed,
					"error_message": fmt.Sprintf("GitHub Action执行失败: %s", runDetails.Conclusion),
				})
				
			logger.Logger.Error("任务同步失败", 
				zap.String("task_id", task.TaskID),
				zap.String("conclusion", runDetails.Conclusion))
		}
	case "cancelled", "failure", "timed_out":
		task.Status = models.TaskStatusFailed
		task.CompletedAt = &completedAt
		task.ErrorMessage = fmt.Sprintf("GitHub Action状态异常: %s", runDetails.Status)
		needUpdate = true
		
		// 更新所有相关镜像记录的状态
		database.DB.Model(&models.ImageSyncRecord{}).
			Where("task_id = ?", task.TaskID).
			Updates(map[string]interface{}{
				"sync_status": models.SyncStatusFailed,
				"error_message": fmt.Sprintf("GitHub Action状态异常: %s", runDetails.Status),
			})
			
		logger.Logger.Error("任务状态异常", 
			zap.String("task_id", task.TaskID),
			zap.String("status", runDetails.Status))
	}

	// 保存更新到数据库
	if needUpdate {
		if err := database.DB.Save(task).Error; err != nil {
			logger.Logger.Error("更新任务状态失败", 
				zap.Error(err), 
				zap.String("task_id", task.TaskID))
		} else {
			logger.Logger.Info("任务状态已更新", 
				zap.String("task_id", task.TaskID),
				zap.String("new_status", task.Status))
		}
	}
}

// processBatchSyncTask 处理批量同步任务
func (h *SyncHandler) processBatchSyncTask(taskID string) {
	logger.Logger.Info("开始处理批量同步任务", zap.String("task_id", taskID))

	// 更新任务状态为运行中
	now := time.Now()
	if err := database.DB.Model(&models.SyncTask{}).
		Where("task_id = ?", taskID).
		Updates(map[string]interface{}{
			"status":     models.TaskStatusRunning,
			"started_at": &now,
		}).Error; err != nil {
		logger.Logger.Error("更新批量任务状态失败", zap.Error(err))
		return
	}

	// 获取任务信息
	var task models.SyncTask
	if err := database.DB.Where("task_id = ?", taskID).First(&task).Error; err != nil {
		logger.Logger.Error("查询批量任务失败", zap.Error(err))
		return
	}

	// 获取所有待同步的镜像记录，按优先级排序
	var records []models.ImageSyncRecord
	if err := database.DB.Where("task_id = ? AND sync_status = ?", taskID, models.SyncStatusPending).
		Order("priority DESC, created_at ASC").
		Find(&records).Error; err != nil {
		h.handleBatchSyncError(taskID, fmt.Sprintf("查询镜像记录失败: %v", err))
		return
	}

	// 构建要添加到images.txt的镜像列表
	var processedImages []string
	for _, record := range records {
		imageStr := record.OriginalImage
		if record.Tag != "" {
			imageStr = imageStr + ":" + record.Tag
		}
		
		// 如果是arm64架构，添加platform参数
		if record.Architecture == "arm64" {
			imageStr = "--platform=linux/arm64 " + imageStr
		}
		
		processedImages = append(processedImages, imageStr)
	}

	// 执行Git操作
	commitSHA, err := h.gitService.UpdateImagesFile(processedImages)
	if err != nil {
		h.handleBatchSyncError(taskID, fmt.Sprintf("Git操作失败: %v", err))
		return
	}

	// 更新提交SHA
	database.DB.Model(&models.SyncTask{}).
		Where("task_id = ?", taskID).
		Update("commit_sha", commitSHA)

	// 更新所有镜像记录状态为同步中，并设置ACR镜像地址
	for _, record := range records {
		acrImage := h.generateACRImageWithArchitecture(record.OriginalImage, record.Tag, record.Architecture)
		database.DB.Model(&models.ImageSyncRecord{}).
			Where("id = ?", record.ID).
			Updates(map[string]interface{}{
				"sync_status": models.SyncStatusSyncing,
				"started_at":  &now,
				"acr_image":   acrImage,
			})
	}

	// 等待GitHub同步并监控Actions
	go h.monitorBatchGitHubActions(taskID, commitSHA)

	logger.Logger.Info("批量同步任务Git操作完成", 
		zap.String("task_id", taskID),
		zap.String("commit_sha", commitSHA))
}

// monitorBatchGitHubActions 监控批量同步的GitHub Actions
func (h *SyncHandler) monitorBatchGitHubActions(taskID, commitSHA string) {
	logger.Logger.Info("开始监控批量同步GitHub Actions", 
		zap.String("task_id", taskID),
		zap.String("commit_sha", commitSHA))

	// 等待一段时间让GitHub同步
	time.Sleep(30 * time.Second)

	// 查询GitHub Actions状态
	runID, actionURL, err := h.githubService.GetWorkflowRun(commitSHA)
	if err != nil {
		h.handleBatchSyncError(taskID, fmt.Sprintf("查询GitHub Actions失败: %v", err))
		return
	}

	// 更新GitHub Action信息
	database.DB.Model(&models.SyncTask{}).
		Where("task_id = ?", taskID).
		Updates(map[string]interface{}{
			"github_run_id":     runID,
			"github_action_url": actionURL,
		})

	// 持续监控Actions状态
	for i := 0; i < 60; i++ { // 最多监控30分钟
		time.Sleep(30 * time.Second)

		status, err := h.githubService.GetWorkflowRunStatus(runID)
		if err != nil {
			logger.Logger.Error("查询Actions状态失败", zap.Error(err))
			continue
		}

		if status == "completed" {
			// Actions完成，检查结果
			conclusion, err := h.githubService.GetWorkflowRunConclusion(runID)
			if err != nil {
				h.handleBatchSyncError(taskID, fmt.Sprintf("获取Actions结果失败: %v", err))
				return
			}

			if conclusion == "success" {
				h.handleBatchSyncSuccess(taskID)
			} else {
				h.handleBatchSyncError(taskID, fmt.Sprintf("GitHub Actions执行失败: %s", conclusion))
			}
			return
		}

		if status == "cancelled" || status == "failure" {
			h.handleBatchSyncError(taskID, fmt.Sprintf("GitHub Actions执行失败: %s", status))
			return
		}
	}

	// 超时处理
	h.handleBatchSyncError(taskID, "GitHub Actions监控超时")
}

// handleBatchSyncSuccess 处理批量同步成功
func (h *SyncHandler) handleBatchSyncSuccess(taskID string) {
	logger.Logger.Info("批量同步任务成功完成", zap.String("task_id", taskID))

	now := time.Now()
	
	// 更新任务状态
	database.DB.Model(&models.SyncTask{}).
		Where("task_id = ?", taskID).
		Updates(map[string]interface{}{
			"status":       models.TaskStatusCompleted,
			"completed_at": &now,
		})

	// 更新所有镜像记录状态为成功
	database.DB.Model(&models.ImageSyncRecord{}).
		Where("task_id = ? AND sync_status = ?", taskID, models.SyncStatusSyncing).
		Updates(map[string]interface{}{
			"sync_status":  models.SyncStatusSuccess,
			"completed_at": &now,
		})
}

// processSingleImage 处理单个镜像同步
func (h *SyncHandler) processSingleImage(taskID string, record models.ImageSyncRecord) {
	logger.Logger.Info("开始处理单个镜像", 
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

	// 构建镜像字符串
	imageStr := record.OriginalImage
	if record.Tag != "" {
		imageStr = imageStr + ":" + record.Tag
	}
	
	// 如果是arm64架构，添加platform参数
	if record.Architecture == "arm64" {
		imageStr = "--platform=linux/arm64 " + imageStr
	}

	// 测试模式：跳过Git操作，直接模拟同步过程
	logger.Logger.Info("测试模式：模拟镜像同步", 
		zap.String("image", imageStr),
		zap.String("architecture", record.Architecture))
	
	// 模拟同步过程
	time.Sleep(time.Duration(2+record.ID%3) * time.Second) // 模拟不同的同步时间

	// 更新镜像状态为成功
	completedTime := time.Now()
	duration := int64(completedTime.Sub(startTime).Seconds())
	
	if err := database.DB.Model(&models.ImageSyncRecord{}).
		Where("id = ?", record.ID).
		Updates(map[string]interface{}{
			"sync_status":  models.SyncStatusSuccess,
			"completed_at": &completedTime,
			"duration":     duration,
			"acr_image":    h.generateACRImageWithArchitecture(record.OriginalImage, record.Tag, record.Architecture),
		}).Error; err != nil {
		logger.Logger.Error("更新镜像完成状态失败", zap.Error(err))
	}

	logger.Logger.Info("镜像同步完成", 
		zap.String("task_id", taskID),
		zap.String("image", record.OriginalImage),
		zap.Int64("duration", duration))

	// 更新批量任务进度
	h.updateBatchTaskProgress(taskID)
}

// handleSingleImageError 处理单个镜像同步错误
func (h *SyncHandler) handleSingleImageError(recordID uint, errorMsg string) {
	completedTime := time.Now()
	
	if err := database.DB.Model(&models.ImageSyncRecord{}).
		Where("id = ?", recordID).
		Updates(map[string]interface{}{
			"sync_status":   models.SyncStatusFailed,
			"error_message": errorMsg,
			"completed_at":  &completedTime,
		}).Error; err != nil {
		logger.Logger.Error("更新镜像错误状态失败", zap.Error(err))
	}
}

// updateBatchTaskProgress 更新批量任务进度
func (h *SyncHandler) updateBatchTaskProgress(taskID string) {
	var completedCount, failedCount int64
	
	// 统计已完成的镜像数
	database.DB.Model(&models.ImageSyncRecord{}).
		Where("task_id = ? AND sync_status = ?", taskID, models.SyncStatusSuccess).
		Count(&completedCount)
	
	// 统计失败的镜像数
	database.DB.Model(&models.ImageSyncRecord{}).
		Where("task_id = ? AND sync_status = ?", taskID, models.SyncStatusFailed).
		Count(&failedCount)

	// 获取总镜像数
	var task models.SyncTask
	if err := database.DB.Where("task_id = ?", taskID).First(&task).Error; err != nil {
		logger.Logger.Error("查询任务失败", zap.Error(err))
		return
	}

	// 计算进度
	progress := float64(0)
	if task.TotalImages > 0 {
		progress = float64(completedCount+failedCount) * 100.0 / float64(task.TotalImages)
	}

	// 更新任务进度
	if err := database.DB.Model(&models.SyncTask{}).
		Where("task_id = ?", taskID).
		Updates(map[string]interface{}{
			"completed_images": completedCount,
			"failed_images":    failedCount,
			"progress":         progress,
		}).Error; err != nil {
		logger.Logger.Error("更新任务进度失败", zap.Error(err))
	}
}

// updateBatchTaskFinalStatus 更新批量任务最终状态
func (h *SyncHandler) updateBatchTaskFinalStatus(taskID string) {
	var task models.SyncTask
	if err := database.DB.Where("task_id = ?", taskID).First(&task).Error; err != nil {
		logger.Logger.Error("查询任务失败", zap.Error(err))
		return
	}

	completedTime := time.Now()
	finalStatus := models.TaskStatusCompleted
	
	// 如果有失败的镜像，根据策略决定最终状态
	if task.FailedImages > 0 {
		if task.FailedImages == task.TotalImages {
			finalStatus = models.TaskStatusFailed
		} else {
			finalStatus = models.TaskStatusCompleted // 部分成功也算完成
		}
	}

	if err := database.DB.Model(&models.SyncTask{}).
		Where("task_id = ?", taskID).
		Updates(map[string]interface{}{
			"status":       finalStatus,
			"completed_at": &completedTime,
			"progress":     100.0,
		}).Error; err != nil {
		logger.Logger.Error("更新任务最终状态失败", zap.Error(err))
	}

	logger.Logger.Info("批量同步任务完成", 
		zap.String("task_id", taskID),
		zap.String("final_status", finalStatus),
		zap.Int("total", task.TotalImages),
		zap.Int("completed", task.CompletedImages),
		zap.Int("failed", task.FailedImages))
}

// handleBatchSyncError 处理批量同步错误
func (h *SyncHandler) handleBatchSyncError(taskID, errorMsg string) {
	completedTime := time.Now()
	
	if err := database.DB.Model(&models.SyncTask{}).
		Where("task_id = ?", taskID).
		Updates(map[string]interface{}{
			"status":        models.TaskStatusFailed,
			"error_message": errorMsg,
			"completed_at":  &completedTime,
		}).Error; err != nil {
		logger.Logger.Error("更新批量任务错误状态失败", zap.Error(err))
	}

	logger.Logger.Error("批量同步任务失败", 
		zap.String("task_id", taskID),
		zap.String("error", errorMsg))
}

// calculateEstimatedTime 计算预估时间
func (h *SyncHandler) calculateEstimatedTime(imageCount, maxConcurrent int) string {
	// 假设每个镜像平均需要2分钟
	avgTimePerImage := 2.0 // 分钟
	
	// 考虑并发处理
	totalTime := (float64(imageCount) / float64(maxConcurrent)) * avgTimePerImage
	
	if totalTime < 1 {
		return "< 1分钟"
	} else if totalTime < 60 {
		return fmt.Sprintf("约 %.0f 分钟", totalTime)
	} else {
		hours := int(totalTime / 60)
		minutes := int(totalTime) % 60
		return fmt.Sprintf("约 %d 小时 %d 分钟", hours, minutes)
	}
}

// calculateRemainingTime 计算剩余时间
func (h *SyncHandler) calculateRemainingTime(total, completed, maxConcurrent int) string {
	remaining := total - completed
	if remaining <= 0 {
		return "已完成"
	}
	
	return h.calculateEstimatedTime(remaining, maxConcurrent)
}