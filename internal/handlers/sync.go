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

	// 获取镜像列表
	images := strings.Split(task.ImagesJSON, "\n")
	if len(images) == 1 && images[0] == "" {
		images = []string{}
	}

	response := models.TaskStatusResponse{
		TaskID:          task.TaskID,
		Status:          task.Status,
		GitHubActionURL: task.GitHubActionURL,
		StartedAt:       task.StartedAt,
		CompletedAt:     task.CompletedAt,
		ErrorMessage:    task.ErrorMessage,
		Images:          images,
	}

	c.JSON(http.StatusOK, response)
}

// SyncHistoryItem 同步历史项
type SyncHistoryItem struct {
	TaskID          string     `json:"task_id"`
	SourceImage     string     `json:"source_image"`
	TargetImage     string     `json:"target_image"`
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
		if record.Tag != "" && record.Tag != "latest" {
			sourceImage = fmt.Sprintf("%s:%s", record.OriginalImage, record.Tag)
		}

		// 目标镜像地址
		targetImage := record.ACRImage
		if targetImage == "" {
			// 如果没有ACR地址，使用generateACRImage生成
			targetImage = h.generateACRImage(record.OriginalImage, record.Tag)
		}

		historyItem := SyncHistoryItem{
			TaskID:          task.TaskID,
			SourceImage:     sourceImage,
			TargetImage:     targetImage,
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

	// 执行Git操作
	commitSHA, err := h.gitService.UpdateImagesFile(images)
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
		acrImage := h.generateACRImage(record.OriginalImage, record.Tag)
		
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
		namespace = "docker-sync" // 默认命名空间
	}

	// 解析镜像名称
	imageName := originalImage
	if strings.Contains(imageName, "/") {
		parts := strings.Split(imageName, "/")
		imageName = parts[len(parts)-1]
	}
	
	if tag != "" && tag != "latest" {
		return fmt.Sprintf("%s/%s/%s:%s", registry, namespace, imageName, tag)
	}
	
	return fmt.Sprintf("%s/%s/%s", registry, namespace, imageName)
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