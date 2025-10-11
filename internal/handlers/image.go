package handlers

import (
	"context"
	"fmt"
	"net/http"
	"strconv"
	"strings"
	"time"

	"docker-image-sync-platform/internal/database"
	"docker-image-sync-platform/internal/logger"
	"docker-image-sync-platform/internal/models"

	"github.com/gin-gonic/gin"
	"go.uber.org/zap"
	"github.com/google/go-containerregistry/pkg/name"
	"github.com/google/go-containerregistry/pkg/v1/remote"
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
	architecture := c.Query("architecture")
	sortBy := c.DefaultQuery("sort_by", "created_at")
	sortOrder := c.DefaultQuery("sort_order", "desc")
	deduplicate := c.DefaultQuery("deduplicate", "false") == "true" // 新增去重参数

	// 添加调试日志
	logger.Logger.Info("GetImages API调用参数", 
		zap.Int("page", page),
		zap.Int("pageSize", pageSize),
		zap.String("status", status),
		zap.String("search", search),
		zap.String("architecture", architecture),
		zap.String("sortBy", sortBy),
		zap.String("sortOrder", sortOrder),
		zap.Bool("deduplicate", deduplicate))

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

	// 架构过滤
	if architecture != "" {
		if architecture == "amd64" {
			// amd64 匹配空值、NULL值和明确的amd64值
			query = query.Where("(architecture = ? OR architecture IS NULL OR architecture = '')", architecture)
		} else {
			// 其他架构需要精确匹配
			query = query.Where("architecture = ?", architecture)
		}
	}

	// 搜索过滤
	if search != "" {
		query = query.Where("original_image LIKE ? OR acr_image LIKE ?", 
			"%"+search+"%", "%"+search+"%")
	}

	var images []models.ImageSyncRecord
	var total int64

	if deduplicate {
		// 去重模式：先去重，再应用过滤条件，确保去重优先级最高
		// 第一步：获取所有去重后的记录ID（基于源镜像(含tag)、目标镜像、架构和同步状态的组合去重）
		subQuery := database.DB.Model(&models.ImageSyncRecord{}).
			Select("MAX(id) as max_id").
			Group("original_image, tag, acr_image, architecture, sync_status")

		// 第二步：获取去重后的所有记录ID
		var maxIds []struct {
			MaxId uint `json:"max_id"`
		}
		if err := subQuery.Find(&maxIds).Error; err != nil {
			logger.Logger.Error("查询去重镜像ID失败", zap.Error(err))
			c.JSON(http.StatusInternalServerError, gin.H{"error": "查询镜像列表失败"})
			return
		}

		// 提取ID列表
		var deduplicatedIds []uint
		for _, item := range maxIds {
			deduplicatedIds = append(deduplicatedIds, item.MaxId)
		}

		if len(deduplicatedIds) == 0 {
			// 没有数据
			images = []models.ImageSyncRecord{}
			total = 0
		} else {
			// 第三步：在去重后的记录上应用过滤条件
			filteredQuery := database.DB.Model(&models.ImageSyncRecord{}).Where("id IN ?", deduplicatedIds)

			// 应用过滤条件
			if status != "" {
				filteredQuery = filteredQuery.Where("sync_status = ?", status)
			}
			if architecture != "" {
				if architecture == "amd64" {
					filteredQuery = filteredQuery.Where("(architecture = ? OR architecture IS NULL OR architecture = '')", architecture)
				} else {
					filteredQuery = filteredQuery.Where("architecture = ?", architecture)
				}
			}
			if search != "" {
				filteredQuery = filteredQuery.Where("original_image LIKE ? OR acr_image LIKE ?", 
					"%"+search+"%", "%"+search+"%")
			}

			// 获取过滤后的总数
			filteredQuery.Count(&total)

			// 构建排序字符串
			orderStr := "created_at DESC" // 默认排序
			if sortBy != "" {
				// 验证排序字段
				validSortFields := map[string]bool{
					"original_image": true,
					"sync_status":    true,
					"architecture":   true,
					"created_at":     true,
					"updated_at":     true,
				}
				
				if validSortFields[sortBy] {
					if sortOrder == "asc" {
						orderStr = fmt.Sprintf("%s ASC", sortBy)
					} else {
						orderStr = fmt.Sprintf("%s DESC", sortBy)
					}
				}
			}

			// 分页查询
			if err := filteredQuery.
				Order(orderStr).
				Offset(offset).
				Limit(pageSize).
				Find(&images).Error; err != nil {
				logger.Logger.Error("查询过滤后的镜像数据失败", zap.Error(err))
				c.JSON(http.StatusInternalServerError, gin.H{"error": "查询镜像列表失败"})
				return
			}
		}
	} else {
		// 普通模式：不去重，直接应用所有筛选条件
		// 重新构建查询以确保所有筛选条件都被应用
		filteredQuery := database.DB.Model(&models.ImageSyncRecord{})

		// 应用所有筛选条件
		if status != "" {
			filteredQuery = filteredQuery.Where("sync_status = ?", status)
		}
		if architecture != "" {
			if architecture == "amd64" {
				filteredQuery = filteredQuery.Where("(architecture = ? OR architecture IS NULL OR architecture = '')", architecture)
			} else {
				filteredQuery = filteredQuery.Where("architecture = ?", architecture)
			}
		}
		if search != "" {
			filteredQuery = filteredQuery.Where("original_image LIKE ? OR acr_image LIKE ?", 
				"%"+search+"%", "%"+search+"%")
		}

		// 查询总数
		filteredQuery.Count(&total)

		// 构建排序字符串
		orderStr := "created_at DESC" // 默认排序
		if sortBy != "" {
			// 验证排序字段
			validSortFields := map[string]bool{
				"original_image": true,
				"sync_status":    true,
				"architecture":   true,
				"created_at":     true,
				"updated_at":     true,
			}
			
			if validSortFields[sortBy] {
				if sortOrder == "asc" {
					orderStr = fmt.Sprintf("%s ASC", sortBy)
				} else {
					orderStr = fmt.Sprintf("%s DESC", sortBy)
				}
			}
		}

		// 查询数据
		if err := filteredQuery.Order(orderStr).
			Offset(offset).
			Limit(pageSize).
			Find(&images).Error; err != nil {
			logger.Logger.Error("查询镜像列表失败", zap.Error(err))
			c.JSON(http.StatusInternalServerError, gin.H{"error": "查询镜像列表失败"})
			return
		}
	}

	// 处理镜像数据，确保目标镜像地址包含完整标签
	for i := range images {
		if images[i].SyncStatus == models.SyncStatusSuccess {
			// 对于成功的镜像，确保ACR地址包含完整标签
			if images[i].ACRImage == "" {
				// 如果没有ACR地址，生成完整的ACR地址
				images[i].ACRImage = h.generateACRImageWithArchitecture(images[i].OriginalImage, images[i].Tag, images[i].Architecture)
			} else if !strings.Contains(images[i].ACRImage, ":") {
				// 如果ACR地址没有标签，添加标签
				tag := images[i].Tag
				if tag == "" {
					tag = "latest"
				}
				images[i].ACRImage = fmt.Sprintf("%s:%s", images[i].ACRImage, tag)
			}
		}
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

    // 确保目标镜像地址包含完整标签（详情页）
    if image.SyncStatus == models.SyncStatusSuccess {
        if image.ACRImage == "" {
            // 如果没有ACR地址，生成完整的ACR地址
            image.ACRImage = h.generateACRImageWithArchitecture(image.OriginalImage, image.Tag, image.Architecture)
        } else if !strings.Contains(image.ACRImage, ":") {
            // 如果ACR地址没有标签，添加标签
            tag := image.Tag
            if tag == "" {
                tag = "latest"
            }
            image.ACRImage = fmt.Sprintf("%s:%s", image.ACRImage, tag)
        }
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

// generateACRImage 生成阿里云ACR镜像地址
func (h *ImageHandler) generateACRImage(originalImage, tag string) string {
	return h.generateACRImageWithArchitecture(originalImage, tag, "")
}

// generateACRImageWithArchitecture 生成带架构信息的阿里云ACR镜像地址
func (h *ImageHandler) generateACRImageWithArchitecture(originalImage, tag, architecture string) string {
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

// CheckImageExists 检测镜像是否存在
func (h *ImageHandler) CheckImageExists(c *gin.Context) {
	idStr := c.Param("id")
	id, err := strconv.ParseUint(idStr, 10, 32)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "无效的镜像ID"})
		return
	}

	var image models.ImageSyncRecord
	if err := database.DB.First(&image, uint(id)).Error; err != nil {
		logger.Logger.Error("查询镜像失败", zap.Error(err), zap.Uint64("id", id))
		c.JSON(http.StatusNotFound, gin.H{"error": "镜像不存在"})
		return
	}

	// 生成目标镜像地址
	targetImage := h.generateACRImageWithArchitecture(image.OriginalImage, image.Tag, image.Architecture)
	
	// 检测镜像是否存在
	exists, err := h.checkImageExistsInRegistry(targetImage)
	if err != nil {
		logger.Logger.Error("检测镜像存在性失败", 
			zap.Error(err), 
			zap.String("target_image", targetImage))
		c.JSON(http.StatusInternalServerError, gin.H{
			"error": "检测镜像存在性失败",
			"details": err.Error(),
		})
		return
	}

	// 根据检测结果和当前状态决定是否更新状态
	if exists {
		// 只有当前状态为失败时，检测成功才更新为成功
		if image.SyncStatus == models.SyncStatusFailed {
			if err := database.DB.Model(&image).Updates(map[string]interface{}{
				"sync_status": models.SyncStatusSuccess,
				"acr_image":   targetImage,
				"error_message": "",
			}).Error; err != nil {
				logger.Logger.Error("更新镜像状态失败", zap.Error(err))
				c.JSON(http.StatusInternalServerError, gin.H{"error": "更新镜像状态失败"})
				return
			}
			
			logger.Logger.Info("镜像检测成功，状态已更新", 
				zap.Uint64("id", id),
				zap.String("target_image", targetImage))
		} else {
			logger.Logger.Info("镜像检测成功，但状态已为成功，无需更新", 
				zap.Uint64("id", id),
				zap.String("target_image", targetImage))
		}
	} else {
		// 镜像不存在时，更新状态为失败
		if err := database.DB.Model(&image).Updates(map[string]interface{}{
			"sync_status": models.SyncStatusFailed,
			"error_message": "镜像不存在",
		}).Error; err != nil {
			logger.Logger.Error("更新镜像状态失败", zap.Error(err))
			c.JSON(http.StatusInternalServerError, gin.H{"error": "更新镜像状态失败"})
			return
		}
		
		logger.Logger.Info("镜像检测失败，状态已更新为失败", 
			zap.Uint64("id", id),
			zap.String("target_image", targetImage))
	}

	c.JSON(http.StatusOK, gin.H{
		"exists": exists,
		"target_image": targetImage,
		"message": func() string {
			if exists {
				return "镜像存在，状态已更新为成功"
			}
			return "镜像不存在，状态已更新为失败"
		}(),
	})
}

// BatchCheckImages 批量检测镜像是否存在
func (h *ImageHandler) BatchCheckImages(c *gin.Context) {
	var request struct {
		IDs []uint `json:"ids" binding:"required"`
	}

	if err := c.ShouldBindJSON(&request); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "请求参数错误"})
		return
	}

	if len(request.IDs) == 0 {
		c.JSON(http.StatusBadRequest, gin.H{"error": "镜像ID列表不能为空"})
		return
	}

	if len(request.IDs) > 50 {
		c.JSON(http.StatusBadRequest, gin.H{"error": "一次最多检测50个镜像"})
		return
	}

	var images []models.ImageSyncRecord
	if err := database.DB.Where("id IN ?", request.IDs).Find(&images).Error; err != nil {
		logger.Logger.Error("查询镜像列表失败", zap.Error(err))
		c.JSON(http.StatusInternalServerError, gin.H{"error": "查询镜像列表失败"})
		return
	}

	results := make([]map[string]interface{}, 0, len(images))
	successCount := 0
	failedCount := 0

	for _, image := range images {
		targetImage := h.generateACRImageWithArchitecture(image.OriginalImage, image.Tag, image.Architecture)
		
		exists, err := h.checkImageExistsInRegistry(targetImage)
		if err != nil {
			logger.Logger.Error("检测镜像存在性失败", 
				zap.Error(err), 
				zap.Uint("id", image.ID),
				zap.String("target_image", targetImage))
			
			// 检测失败时，也要更新镜像状态为失败
			if updateErr := database.DB.Model(&image).Updates(map[string]interface{}{
				"sync_status": models.SyncStatusFailed,
				"error_message": fmt.Sprintf("检测失败: %v", err),
			}).Error; updateErr != nil {
				logger.Logger.Error("更新镜像状态失败", zap.Error(updateErr), zap.Uint("id", image.ID))
			}
			
			results = append(results, map[string]interface{}{
				"id": image.ID,
				"original_image": image.OriginalImage,
				"target_image": targetImage,
				"exists": false,
				"error": err.Error(),
			})
			failedCount++
			continue
		}

		// 根据检测结果和当前状态决定是否更新状态
		if exists {
			// 只有当前状态为失败时，检测成功才更新为成功
			if image.SyncStatus == models.SyncStatusFailed {
				if err := database.DB.Model(&image).Updates(map[string]interface{}{
					"sync_status": models.SyncStatusSuccess,
					"acr_image":   targetImage,
					"error_message": "",
				}).Error; err != nil {
					logger.Logger.Error("更新镜像状态失败", zap.Error(err), zap.Uint("id", image.ID))
				}
			}
			successCount++
		} else {
			// 镜像不存在时，更新状态为失败
			if err := database.DB.Model(&image).Updates(map[string]interface{}{
				"sync_status": models.SyncStatusFailed,
				"error_message": "镜像不存在",
			}).Error; err != nil {
				logger.Logger.Error("更新镜像状态失败", zap.Error(err), zap.Uint("id", image.ID))
			}
			failedCount++
		}

		results = append(results, map[string]interface{}{
			"id": image.ID,
			"original_image": image.OriginalImage,
			"target_image": targetImage,
			"exists": exists,
		})
	}

	logger.Logger.Info("批量检测镜像完成", 
		zap.Int("total", len(images)),
		zap.Int("success", successCount),
		zap.Int("failed", failedCount))

	c.JSON(http.StatusOK, gin.H{
		"total": len(images),
		"success_count": successCount,
		"failed_count": failedCount,
		"results": results,
		"message": fmt.Sprintf("检测完成：%d个镜像存在，%d个镜像不存在", successCount, failedCount),
	})
}

// checkImageExistsInRegistry 检测镜像在注册表中是否存在
func (h *ImageHandler) checkImageExistsInRegistry(imageRef string) (bool, error) {
	// 解析镜像引用
	ref, err := name.ParseReference(imageRef)
	if err != nil {
		return false, fmt.Errorf("解析镜像引用失败: %v", err)
	}

	// 创建带超时的上下文
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	// 尝试获取镜像的manifest
	_, err = remote.Head(ref, remote.WithContext(ctx))
	if err != nil {
		// 如果是404错误，说明镜像不存在
		if strings.Contains(err.Error(), "404") || strings.Contains(err.Error(), "not found") {
			return false, nil
		}
		// 其他错误返回错误信息
		return false, fmt.Errorf("检测镜像存在性失败: %v", err)
	}

	return true, nil
}