package handlers

import (
	"net/http"
	"strconv"

	"docker-image-sync-platform/internal/database"
	"docker-image-sync-platform/internal/logger"
	"docker-image-sync-platform/internal/models"

	"github.com/gin-gonic/gin"
	"go.uber.org/zap"
)

// ImageHandler 镜像处理器
type ImageHandler struct{}

// NewImageHandler 创建镜像处理器
func NewImageHandler() *ImageHandler {
	return &ImageHandler{}
}

// GetImages 获取镜像列表
func (h *ImageHandler) GetImages(c *gin.Context) {
	page, _ := strconv.Atoi(c.DefaultQuery("page", "1"))
	pageSize, _ := strconv.Atoi(c.DefaultQuery("page_size", "10"))
	status := c.Query("status")
	search := c.Query("search")

	if page < 1 {
		page = 1
	}
	if pageSize < 1 || pageSize > 100 {
		pageSize = 10
	}

	offset := (page - 1) * pageSize

	// 构建查询
	query := database.DB.Model(&models.ImageSyncRecord{})

	// 状态过滤
	if status != "" {
		query = query.Where("sync_status = ?", status)
	}

	// 搜索过滤
	if search != "" {
		query = query.Where("original_image LIKE ? OR acr_image LIKE ?", 
			"%"+search+"%", "%"+search+"%")
	}

	// 查询总数
	var total int64
	query.Count(&total)

	// 查询数据
	var images []models.ImageSyncRecord
	if err := query.Order("created_at DESC").
		Offset(offset).
		Limit(pageSize).
		Find(&images).Error; err != nil {
		logger.Logger.Error("查询镜像列表失败", zap.Error(err))
		c.JSON(http.StatusInternalServerError, gin.H{"error": "查询镜像列表失败"})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"total":     total,
		"data":      images,
		"page":      page,
		"page_size": pageSize,
	})
}

// GetImage 获取镜像详情
func (h *ImageHandler) GetImage(c *gin.Context) {
	idStr := c.Param("id")
	id, err := strconv.ParseUint(idStr, 10, 32)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "无效的镜像ID"})
		return
	}

	var image models.ImageSyncRecord
	if err := database.DB.First(&image, uint(id)).Error; err != nil {
		logger.Logger.Error("查询镜像详情失败", zap.Error(err), zap.Uint64("id", id))
		c.JSON(http.StatusNotFound, gin.H{"error": "镜像不存在"})
		return
	}

	c.JSON(http.StatusOK, image)
}

// DeleteImage 删除镜像记录
func (h *ImageHandler) DeleteImage(c *gin.Context) {
	idStr := c.Param("id")
	id, err := strconv.ParseUint(idStr, 10, 32)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "无效的镜像ID"})
		return
	}

	// 检查镜像是否存在
	var image models.ImageSyncRecord
	if err := database.DB.First(&image, uint(id)).Error; err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "镜像不存在"})
		return
	}

	// 删除镜像记录
	if err := database.DB.Delete(&image).Error; err != nil {
		logger.Logger.Error("删除镜像记录失败", zap.Error(err), zap.Uint64("id", id))
		c.JSON(http.StatusInternalServerError, gin.H{"error": "删除镜像记录失败"})
		return
	}

	logger.Logger.Info("镜像记录已删除", 
		zap.Uint64("id", id),
		zap.String("image", image.OriginalImage))

	c.JSON(http.StatusOK, gin.H{"message": "镜像记录已删除"})
}

// GetImageStats 获取镜像统计信息
func (h *ImageHandler) GetImageStats(c *gin.Context) {
	var stats struct {
		Total     int64 `json:"total"`
		Pending   int64 `json:"pending"`
		Syncing   int64 `json:"syncing"`
		Success   int64 `json:"success"`
		Failed    int64 `json:"failed"`
	}

	// 总数
	database.DB.Model(&models.ImageSyncRecord{}).Count(&stats.Total)

	// 各状态数量
	database.DB.Model(&models.ImageSyncRecord{}).
		Where("sync_status = ?", models.SyncStatusPending).
		Count(&stats.Pending)

	database.DB.Model(&models.ImageSyncRecord{}).
		Where("sync_status = ?", models.SyncStatusSyncing).
		Count(&stats.Syncing)

	database.DB.Model(&models.ImageSyncRecord{}).
		Where("sync_status = ?", models.SyncStatusSuccess).
		Count(&stats.Success)

	database.DB.Model(&models.ImageSyncRecord{}).
		Where("sync_status = ?", models.SyncStatusFailed).
		Count(&stats.Failed)

	c.JSON(http.StatusOK, stats)
}

// RetrySync 重试同步
func (h *ImageHandler) RetrySync(c *gin.Context) {
	idStr := c.Param("id")
	id, err := strconv.ParseUint(idStr, 10, 32)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "无效的镜像ID"})
		return
	}

	// 检查镜像是否存在
	var image models.ImageSyncRecord
	if err := database.DB.First(&image, uint(id)).Error; err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "镜像不存在"})
		return
	}

	// 只有失败的镜像才能重试
	if image.SyncStatus != models.SyncStatusFailed {
		c.JSON(http.StatusBadRequest, gin.H{"error": "只有失败的镜像才能重试"})
		return
	}

	// 重置状态
	if err := database.DB.Model(&image).Updates(map[string]interface{}{
		"sync_status":   models.SyncStatusPending,
		"error_message": "",
		"acr_image":     "",
	}).Error; err != nil {
		logger.Logger.Error("重置镜像状态失败", zap.Error(err))
		c.JSON(http.StatusInternalServerError, gin.H{"error": "重置镜像状态失败"})
		return
	}

	logger.Logger.Info("镜像重试同步", 
		zap.Uint64("id", id),
		zap.String("image", image.OriginalImage))

	c.JSON(http.StatusOK, gin.H{"message": "镜像已重置为待同步状态"})
}