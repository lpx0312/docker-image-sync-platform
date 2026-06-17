// Package handlers 提供HTTP请求处理器实现
//
// image.go 文件实现了镜像管理相关的HTTP处理器，主要负责：
// - Docker镜像记录的查询、删除和统计
// - 镜像同步状态的管理和监控
// - 镜像存在性验证和批量检查
// - 镜像同步重试机制
//
// 镜像管理功能：
// - 支持分页查询和多条件过滤
// - 提供镜像去重和架构筛选
// - 支持模糊搜索和动态排序
// - 实现镜像状态统计和监控
// - 提供镜像存在性验证服务
//
// 主要业务场景：
// - 镜像列表展示和管理
// - 镜像同步状态监控
// - 失败镜像重试处理
// - 镜像仓库验证服务
//
// 作者: Docker镜像同步平台开发团队
// 版本: v1.0.0
package handlers

import (
	"fmt"
	"net/http"
	"strconv"
	"strings"

	"docker-image-sync-platform/internal/database"
	"docker-image-sync-platform/internal/logger"
	"docker-image-sync-platform/internal/models"
	"docker-image-sync-platform/internal/utils"

	"github.com/gin-gonic/gin"
	"go.uber.org/zap"
	"gorm.io/gorm"
)

// applyArchitectureFilter 按「记录中的 architecture」或「ACR 实测架构 JSON」筛选。
func applyArchitectureFilter(db *gorm.DB, architecture string) *gorm.DB {
	if architecture == "" {
		return db
	}
	like := "%\"" + architecture + "\"%"
	if architecture == "amd64" {
		return db.Where("(architecture = ? OR architecture IS NULL OR architecture = '') OR acr_architectures LIKE ?", architecture, like)
	}
	return db.Where("architecture = ? OR acr_architectures LIKE ?", architecture, like)
}

// ImageHandler 镜像处理器
//
// 负责处理与镜像相关的HTTP请求，包括镜像列表查询、
// 镜像详情获取、镜像状态管理等功能。
//
// 主要功能:
//   - 镜像列表查询（支持分页、过滤、排序、去重）
//   - 镜像详情获取
//   - 镜像状态统计
//   - 镜像仓库验证
type ImageHandler struct{}

// NewImageHandler 创建镜像处理器实例
//
// 返回:
//   - *ImageHandler: 镜像处理器实例
func NewImageHandler() *ImageHandler {
	return &ImageHandler{}
}

// GetImages 获取镜像同步记录列表
//
// HTTP方法: GET
// 路径: /api/images
//
// 查询参数:
//   - page: 页码，默认为1
//   - page_size: 每页大小，默认为10，最大100
//   - status: 同步状态过滤（pending/syncing/success/failed）
//   - search: 搜索关键词（匹配原始镜像或ACR镜像）
//   - architecture: 架构过滤（amd64/arm64等）
//   - sort_by: 排序字段（original_image/sync_status/architecture/created_at/updated_at）
//   - sort_order: 排序方向（asc/desc），默认desc
//   - deduplicate: 是否去重，默认false
//
// 响应码:
//   - 200: 成功返回镜像列表
//   - 500: 服务器内部错误
//
// 响应数据:
//   - total: 总记录数
//   - page: 当前页码
//   - page_size: 每页大小
//   - data: 镜像记录数组
//
// 特殊功能:
//   - 去重模式: 基于源镜像、标签、目标镜像、架构和状态的组合去重
//   - 架构处理: amd64架构兼容空值和NULL值
//   - 模糊搜索: 支持原始镜像和ACR镜像的模糊匹配
func (h *ImageHandler) GetImages(c *gin.Context) {
	// ====================================================================
	// 解析请求参数
	// ====================================================================

	// 分页参数解析
	page, _ := strconv.Atoi(c.DefaultQuery("page", "1"))
	pageSize, _ := strconv.Atoi(c.DefaultQuery("page_size", "10"))

	// 过滤参数解析
	status := c.Query("status")             // 同步状态过滤
	search := c.Query("search")             // 搜索关键词
	architecture := c.Query("architecture") // 架构过滤

	// 排序参数解析
	sortBy := c.DefaultQuery("sort_by", "created_at") // 排序字段
	sortOrder := c.DefaultQuery("sort_order", "desc") // 排序方向

	// 去重参数解析
	deduplicate := c.DefaultQuery("deduplicate", "false") == "true" // 是否启用去重

	// 记录API调用参数的调试日志
	logger.Logger.Info("GetImages API调用参数",
		zap.Int("page", page),
		zap.Int("pageSize", pageSize),
		zap.String("status", status),
		zap.String("search", search),
		zap.String("architecture", architecture),
		zap.String("sortBy", sortBy),
		zap.String("sortOrder", sortOrder),
		zap.Bool("deduplicate", deduplicate))

	// ====================================================================
	// 参数验证和标准化
	// ====================================================================

	// 页码验证：确保页码不小于1
	if page < 1 {
		page = 1
	}

	// 页面大小验证：限制在1-100之间，默认10
	if pageSize < 1 || pageSize > 100 {
		pageSize = 10
	}

	// 计算数据库查询的偏移量
	offset := (page - 1) * pageSize

	// ====================================================================
	// 构建基础查询
	// ====================================================================

	// 创建基础查询对象
	query := database.DB.Model(&models.ImageSyncRecord{})

	// ====================================================================
	// 应用过滤条件
	// ====================================================================

	// 同步状态过滤
	// 支持的状态: pending, syncing, success, failed
	if status != "" {
		query = query.Where("sync_status = ?", status)
	}

	if architecture != "" {
		query = applyArchitectureFilter(query, architecture)
	}

	// 搜索关键词过滤
	// 支持在原始镜像名、ACR镜像名、标签和描述中进行模糊搜索
	if search != "" {
		query = query.Where("original_image LIKE ? OR acr_image LIKE ? OR tag LIKE ? OR description LIKE ? OR task_id LIKE ?",
			"%"+search+"%", "%"+search+"%", "%"+search+"%", "%"+search+"%", "%"+search+"%")
	}

	// ====================================================================
	// 初始化查询结果变量
	// ====================================================================

	var images []models.ImageSyncRecord // 镜像记录列表
	var total int64                     // 总记录数

	// ====================================================================
	// 根据去重模式选择不同的查询策略
	// ====================================================================

	if deduplicate {
		// ================================================================
		// 去重模式：三步查询策略
		// ================================================================
		//
		// 去重逻辑：基于源镜像、标签、目标镜像、架构和同步状态的组合去重
		// 当存在多条相同组合的记录时，保留ID最大的记录（最新记录）
		//
		// 查询步骤：
		// 1. 先进行去重，获取每个组合的最大ID
		// 2. 基于去重后的ID列表应用过滤条件
		// 3. 进行分页和排序查询

		// 第一步：获取去重后的记录ID
		// 使用GROUP BY和MAX函数获取每个组合的最新记录ID
		subQuery := database.DB.Model(&models.ImageSyncRecord{}).
			Select("MAX(id) as max_id").
			Group("original_image, tag, acr_image, architecture, sync_status")

		// 第二步：执行去重查询，获取所有最大ID
		var maxIds []struct {
			MaxId uint `json:"max_id"`
		}
		if err := subQuery.Find(&maxIds).Error; err != nil {
			logger.Logger.Error("查询去重镜像ID失败", zap.Error(err))
			c.JSON(http.StatusInternalServerError, gin.H{"error": "查询镜像列表失败"})
			return
		}

		// 第三步：提取ID列表用于后续查询
		var deduplicatedIds []uint
		for _, item := range maxIds {
			deduplicatedIds = append(deduplicatedIds, item.MaxId)
		}

		// 处理空结果情况
		if len(deduplicatedIds) == 0 {
			// 没有找到任何记录
			images = []models.ImageSyncRecord{}
			total = 0
		} else {
			// 第四步：在去重后的记录上应用用户的过滤条件
			filteredQuery := database.DB.Model(&models.ImageSyncRecord{}).Where("id IN ?", deduplicatedIds)

			// 重新应用所有过滤条件（在去重结果基础上）
			if status != "" {
				filteredQuery = filteredQuery.Where("sync_status = ?", status)
			}
			if architecture != "" {
				filteredQuery = applyArchitectureFilter(filteredQuery, architecture)
			}
			if search != "" {
				filteredQuery = filteredQuery.Where("original_image LIKE ? OR acr_image LIKE ? OR tag LIKE ? OR description LIKE ? OR task_id LIKE ?",
					"%"+search+"%", "%"+search+"%", "%"+search+"%", "%"+search+"%", "%"+search+"%")
			}

			// 统计过滤后的总记录数
			filteredQuery.Count(&total)

			// ========================================================
			// 构建排序规则
			// ========================================================

			// 设置默认排序：按创建时间降序
			orderStr := "created_at DESC"

			if sortBy != "" {
				// 定义允许的排序字段，防止SQL注入
				validSortFields := map[string]bool{
					"original_image": true, // 原始镜像名
					"sync_status":    true, // 同步状态
					"architecture":   true, // 架构
					"created_at":     true, // 创建时间
					"updated_at":     true, // 更新时间
				}

				// 验证排序字段的合法性
				if validSortFields[sortBy] {
					if sortOrder == "asc" {
						orderStr = fmt.Sprintf("%s ASC", sortBy)
					} else {
						orderStr = fmt.Sprintf("%s DESC", sortBy)
					}
				}
			}

			// ========================================================
			// 执行分页查询
			// ========================================================

			// 执行最终的分页查询，获取镜像记录
			if err := filteredQuery.
				Order(orderStr). // 应用排序
				Offset(offset).  // 设置偏移量
				Limit(pageSize). // 设置页面大小
				Find(&images).Error; err != nil {
				logger.Logger.Error("查询过滤后的镜像数据失败", zap.Error(err))
				c.JSON(http.StatusInternalServerError, gin.H{"error": "查询镜像列表失败"})
				return
			}
		}
	} else {
		// ================================================================
		// 普通模式：直接查询，不进行去重
		// ================================================================
		//
		// 普通模式下直接应用所有过滤条件，不进行去重处理
		// 这种模式下可能会显示重复的镜像记录，但查询性能更好

		// 重新构建查询对象以确保所有筛选条件都被正确应用
		filteredQuery := database.DB.Model(&models.ImageSyncRecord{})

		// ============================================================
		// 应用所有过滤条件
		// ============================================================

		// 同步状态过滤
		if status != "" {
			filteredQuery = filteredQuery.Where("sync_status = ?", status)
		}

		if architecture != "" {
			filteredQuery = applyArchitectureFilter(filteredQuery, architecture)
		}

		// 搜索关键词过滤
		if search != "" {
			filteredQuery = filteredQuery.Where("original_image LIKE ? OR acr_image LIKE ? OR tag LIKE ? OR description LIKE ? OR task_id LIKE ?",
				"%"+search+"%", "%"+search+"%", "%"+search+"%", "%"+search+"%", "%"+search+"%")
		}

		// 统计总记录数
		filteredQuery.Count(&total)

		// ============================================================
		// 构建排序规则
		// ============================================================

		// 设置默认排序：按创建时间降序
		orderStr := "created_at DESC"

		if sortBy != "" {
			// 定义允许的排序字段，防止SQL注入
			validSortFields := map[string]bool{
				"original_image": true, // 原始镜像名
				"sync_status":    true, // 同步状态
				"architecture":   true, // 架构
				"created_at":     true, // 创建时间
				"updated_at":     true, // 更新时间
			}

			// 验证排序字段的合法性
			if validSortFields[sortBy] {
				if sortOrder == "asc" {
					orderStr = fmt.Sprintf("%s ASC", sortBy)
				} else {
					orderStr = fmt.Sprintf("%s DESC", sortBy)
				}
			}
		}

		// ============================================================
		// 执行分页查询
		// ============================================================

		// 执行分页查询获取镜像数据
		if err := filteredQuery.Order(orderStr).
			Offset(offset).  // 设置偏移量
			Limit(pageSize). // 设置页面大小
			Find(&images).Error; err != nil {
			logger.Logger.Error("查询镜像列表失败", zap.Error(err))
			c.JSON(http.StatusInternalServerError, gin.H{"error": "查询镜像列表失败"})
			return
		}
	}

	// ====================================================================
	// 数据后处理：确保ACR镜像地址的完整性
	// ====================================================================

	// 遍历所有查询到的镜像记录，确保ACR地址包含完整的标签信息
	// 这是为了解决历史数据中可能存在的ACR地址不完整的问题
	for i := range images {
		if images[i].SyncStatus == models.SyncStatusSuccess {
			// 只处理同步成功的镜像记录

			if images[i].ACRImage == "" {
				// 情况1：ACR地址为空，需要重新生成完整的ACR地址
				images[i].ACRImage = utils.GenerateACRImage(images[i].OriginalImage, images[i].Tag)
			} else if !strings.Contains(images[i].ACRImage, ":") {
				// 情况2：ACR地址存在但缺少标签，需要补充标签
				tag := images[i].Tag
				if tag == "" {
					tag = "latest" // 默认标签
				}
				images[i].ACRImage = fmt.Sprintf("%s:%s", images[i].ACRImage, tag)
			}
			// 情况3：ACR地址完整，无需处理
		}
	}

	// ====================================================================
	// 构建并返回JSON响应
	// ====================================================================

	// 返回标准的分页响应格式
	c.JSON(http.StatusOK, gin.H{
		"total":     total,    // 总记录数
		"data":      images,   // 镜像记录数组
		"page":      page,     // 当前页码
		"page_size": pageSize, // 每页大小
	})
}

// GetImage 获取单个镜像的详细信息
//
// HTTP方法: GET
// 路径: /api/images/:id
//
// 路径参数:
//   - id: 镜像记录的唯一标识符
//
// 响应码:
//   - 200: 成功返回镜像详情
//   - 400: 无效的镜像ID
//   - 404: 镜像记录不存在
//   - 500: 服务器内部错误
//
// 响应数据:
//   - 完整的镜像同步记录信息
//   - 包含处理后的完整ACR地址
//
// 特殊处理:
//   - 自动补全ACR镜像地址的标签信息
//   - 确保返回的数据格式一致性
func (h *ImageHandler) GetImage(c *gin.Context) {
	// ====================================================================
	// 解析和验证路径参数
	// ====================================================================

	// 获取镜像ID参数
	idStr := c.Param("id")

	// 将字符串ID转换为数字ID
	id, err := strconv.ParseUint(idStr, 10, 32)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "无效的镜像ID"})
		return
	}

	// ====================================================================
	// 查询镜像记录
	// ====================================================================

	var image models.ImageSyncRecord
	if err := database.DB.First(&image, uint(id)).Error; err != nil {
		logger.Logger.Error("查询镜像详情失败", zap.Error(err), zap.Uint64("id", id))
		c.JSON(http.StatusNotFound, gin.H{"error": "镜像不存在"})
		return
	}

	// ====================================================================
	// 数据后处理：确保ACR镜像地址的完整性
	// ====================================================================

	// 对于同步成功的镜像，确保ACR地址包含完整标签
	if image.SyncStatus == models.SyncStatusSuccess {
		if image.ACRImage == "" {
			// 情况1：ACR地址为空，重新生成完整的ACR地址
			image.ACRImage = utils.GenerateACRImage(image.OriginalImage, image.Tag)
		} else if !strings.Contains(image.ACRImage, ":") {
			// 情况2：ACR地址存在但缺少标签，补充标签
			tag := image.Tag
			if tag == "" {
				tag = "latest"
			}
			image.ACRImage = fmt.Sprintf("%s:%s", image.ACRImage, tag)
		}
	}

	// ====================================================================
	// 返回镜像详情
	// ====================================================================

	c.JSON(http.StatusOK, image)
}

// DeleteImage 删除指定的镜像同步记录
//
// HTTP方法: DELETE
// 路径: /api/images/:id
//
// 路径参数:
//   - id: 要删除的镜像记录ID
//
// 响应码:
//   - 200: 成功删除镜像记录
//   - 400: 无效的镜像ID
//   - 404: 镜像记录不存在
//   - 500: 服务器内部错误
//
// 响应数据:
//   - message: 删除成功的确认消息
//
// 注意事项:
//   - 删除操作是不可逆的
//   - 删除前会验证记录是否存在
//   - 删除操作会记录详细的审计日志
func (h *ImageHandler) DeleteImage(c *gin.Context) {
	// ====================================================================
	// 解析和验证路径参数
	// ====================================================================

	// 获取镜像ID参数
	idStr := c.Param("id")

	// 将字符串ID转换为数字ID
	id, err := strconv.ParseUint(idStr, 10, 32)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "无效的镜像ID"})
		return
	}

	// ====================================================================
	// 验证镜像记录是否存在
	// ====================================================================

	// 查询镜像记录以确认其存在性
	var image models.ImageSyncRecord
	if err := database.DB.First(&image, uint(id)).Error; err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "镜像不存在"})
		return
	}

	// ====================================================================
	// 执行删除操作
	// ====================================================================

	// 从数据库中删除镜像记录
	if err := database.DB.Delete(&image).Error; err != nil {
		logger.Logger.Error("删除镜像记录失败", zap.Error(err), zap.Uint64("id", id))
		c.JSON(http.StatusInternalServerError, gin.H{"error": "删除镜像记录失败"})
		return
	}

	// ====================================================================
	// 记录审计日志并返回响应
	// ====================================================================

	// 记录删除操作的审计日志
	logger.Logger.Info("镜像记录已删除",
		zap.Uint64("id", id),
		zap.String("image", image.OriginalImage))

	// 返回删除成功的响应
	c.JSON(http.StatusOK, gin.H{"message": "镜像记录已删除"})
}

// GetImageStats 获取镜像同步状态的统计信息
//
// HTTP方法: GET
// 路径: /api/images/stats
//
// 响应码:
//   - 200: 成功返回统计信息
//   - 500: 服务器内部错误
//
// 响应数据:
//   - total: 总镜像数量
//   - pending: 待同步镜像数量
//   - syncing: 同步中镜像数量
//   - success: 同步成功镜像数量
//   - failed: 同步失败镜像数量
//
// 用途:
//   - 仪表板数据展示
//   - 系统状态监控
//   - 统计报表生成
func (h *ImageHandler) GetImageStats(c *gin.Context) {
	// ====================================================================
	// 定义统计数据结构
	// ====================================================================

	// 定义统计信息的数据结构
	var stats struct {
		Total   int64 `json:"total"`   // 总镜像数量
		Pending int64 `json:"pending"` // 待同步数量
		Syncing int64 `json:"syncing"` // 同步中数量
		Success int64 `json:"success"` // 成功数量
		Failed  int64 `json:"failed"`  // 失败数量
	}

	// ====================================================================
	// 查询各种状态的统计数据
	// ====================================================================

	// 查询总镜像数量
	database.DB.Model(&models.ImageSyncRecord{}).Count(&stats.Total)

	// 查询待同步状态的镜像数量
	database.DB.Model(&models.ImageSyncRecord{}).
		Where("sync_status = ?", models.SyncStatusPending).
		Count(&stats.Pending)

	// 查询同步中状态的镜像数量
	database.DB.Model(&models.ImageSyncRecord{}).
		Where("sync_status = ?", models.SyncStatusSyncing).
		Count(&stats.Syncing)

	// 查询同步成功状态的镜像数量
	database.DB.Model(&models.ImageSyncRecord{}).
		Where("sync_status = ?", models.SyncStatusSuccess).
		Count(&stats.Success)

	// 查询同步失败状态的镜像数量
	database.DB.Model(&models.ImageSyncRecord{}).
		Where("sync_status = ?", models.SyncStatusFailed).
		Count(&stats.Failed)

	// ====================================================================
	// 返回统计结果
	// ====================================================================

	// 返回完整的统计信息
	c.JSON(http.StatusOK, stats)
}

// RetrySync 重试指定镜像的同步操作
//
// HTTP方法: POST
// 路径: /api/images/:id/retry
//
// 路径参数:
//   - id: 要重试同步的镜像记录ID
//
// 响应码:
//   - 200: 成功启动重试操作
//   - 400: 无效的镜像ID或镜像状态不允许重试
//   - 404: 镜像记录不存在
//   - 500: 服务器内部错误
//
// 响应数据:
//   - message: 重试操作的确认消息
//
// 重试条件:
//   - 只有失败状态的镜像才能重试
//   - 重试会重置镜像状态为待同步
//   - 重试会清除之前的错误信息
//
// 注意事项:
//   - 重试操作会创建新的同步任务
//   - 重试不会影响其他镜像的同步状态
func (h *ImageHandler) RetrySync(c *gin.Context) {
	// ====================================================================
	// 解析和验证路径参数
	// ====================================================================

	// 获取镜像ID参数
	idStr := c.Param("id")

	// 将字符串ID转换为数字ID
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

	// 业务逻辑验证：只有失败的镜像才能重试
	// 确保重试操作的合理性，避免对正在进行或已成功的任务进行重试
	if image.SyncStatus != models.SyncStatusFailed {
		c.JSON(http.StatusBadRequest, gin.H{"error": "只有失败的镜像才能重试"})
		return
	}

	// 状态重置：将镜像状态重置为待同步状态
	// 清空错误信息和ACR镜像地址，为重新同步做准备
	if err := database.DB.Model(&image).Updates(map[string]interface{}{
		"sync_status":   models.SyncStatusPending, // 重置为待同步状态
		"error_message": "",                       // 清空错误信息
		"acr_image":     "",                       // 清空ACR镜像地址
	}).Error; err != nil {
		logger.Logger.Error("重置镜像状态失败", zap.Error(err))
		c.JSON(http.StatusInternalServerError, gin.H{"error": "重置镜像状态失败"})
		return
	}

	// 操作日志记录
	logger.Logger.Info("镜像重试同步",
		zap.Uint64("id", id),
		zap.String("image", image.OriginalImage))

	c.JSON(http.StatusOK, gin.H{"message": "镜像已重置为待同步状态"})
}

// CheckImageExists 检测镜像是否存在
// HTTP方法: GET
// 路径: /api/images/:id/exists
// 参数: id (路径参数) - 镜像记录ID
// 响应码:
//   - 200: 检测成功，返回存在状态
//   - 400: 无效的镜像ID
//   - 404: 镜像记录不存在
//   - 500: 服务器内部错误
//
// 响应数据: {"exists": boolean, "acr_image": string}
// 功能说明:
//   - 检查指定镜像在ACR中的存在状态
//   - 用于验证同步结果的准确性
//   - 支持实时状态查询
func (h *ImageHandler) CheckImageExists(c *gin.Context) {
	// 参数解析：获取并验证镜像ID
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

	targetImage := utils.BuildACRImageRef(image.AcrRegistryID, image.OriginalImage, image.Tag)

	// 检测镜像是否存在
	exists, err := utils.CheckImageExistsInRegistryWithErr(targetImage, image.AcrRegistryID)
	if err != nil {
		logger.Logger.Error("检测镜像存在性失败",
			zap.Error(err),
			zap.String("target_image", targetImage))
		c.JSON(http.StatusInternalServerError, gin.H{
			"error":   "检测镜像存在性失败",
			"details": err.Error(),
		})
		return
	}

	var arches []string
	if exists {
		if detected, derr := utils.DetectImageArchitecturesInRegistry(targetImage, image.AcrRegistryID); derr == nil {
			arches = detected
		}
	}
	archJSON := utils.ArchitecturesToJSON(arches)

	// 根据检测结果和当前状态决定是否更新状态
	if exists {
		// 只有当前状态为失败时，检测成功才更新为成功
		if image.SyncStatus == models.SyncStatusFailed {
			if err := database.DB.Model(&image).Updates(map[string]interface{}{
				"sync_status":         models.SyncStatusSuccess,
				"acr_image":           targetImage,
				"acr_architectures":   archJSON,
				"error_message":       "",
			}).Error; err != nil {
				logger.Logger.Error("更新镜像状态失败", zap.Error(err))
				c.JSON(http.StatusInternalServerError, gin.H{"error": "更新镜像状态失败"})
				return
			}

			logger.Logger.Info("镜像检测成功，状态已更新",
				zap.Uint64("id", id),
				zap.String("target_image", targetImage))
		} else {
			if err := database.DB.Model(&image).Updates(map[string]interface{}{
				"acr_image":         targetImage,
				"acr_architectures": archJSON,
			}).Error; err != nil {
				logger.Logger.Error("更新镜像架构信息失败", zap.Error(err))
			}
			logger.Logger.Info("镜像检测成功，已刷新ACR架构信息",
				zap.Uint64("id", id),
				zap.String("target_image", targetImage))
		}
	} else {
		// 镜像不存在时，更新状态为失败
		if err := database.DB.Model(&image).Updates(map[string]interface{}{
			"sync_status":       models.SyncStatusFailed,
			"error_message":     "镜像不存在",
			"acr_architectures": archJSON,
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
		"exists":          exists,
		"target_image":    targetImage,
		"architectures":   arches,
		"acr_architectures": archJSON,
		"message": func() string {
			if exists {
				return "镜像存在，状态已更新为成功"
			}
			return "镜像不存在，状态已更新为失败"
		}(),
	})
}

// BatchCheckImages 批量检测镜像是否存在
// HTTP方法: POST
// 路径: /api/images/batch-check
// 请求体: {"ids": [1, 2, 3, ...]}
// 参数:
//   - ids: 要检测的镜像ID数组（必需，最多50个）
//
// 响应码:
//   - 200: 批量检测完成，返回检测结果
//   - 400: 请求参数错误或ID列表为空/超限
//   - 500: 服务器内部错误
//
// 响应数据:
//   - total: 检测的镜像总数
//   - success_count: 存在的镜像数量
//   - failed_count: 不存在的镜像数量
//   - results: 详细检测结果数组
//   - message: 检测结果摘要
//
// 功能说明:
//   - 批量检测多个镜像在ACR中的存在状态
//   - 自动更新镜像的同步状态
//   - 提供详细的检测结果和统计信息
//   - 支持错误处理和状态回滚
func (h *ImageHandler) BatchCheckImages(c *gin.Context) {
	// ====================================================================
	// 请求参数解析和验证
	// ====================================================================

	// 定义请求体结构
	var request struct {
		IDs []uint `json:"ids" binding:"required"` // 镜像ID列表
	}

	// 解析JSON请求体
	if err := c.ShouldBindJSON(&request); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "请求参数错误"})
		return
	}

	// 验证ID列表不为空
	if len(request.IDs) == 0 {
		c.JSON(http.StatusBadRequest, gin.H{"error": "镜像ID列表不能为空"})
		return
	}

	// 限制批量检测的数量，防止系统过载
	if len(request.IDs) > 50 {
		c.JSON(http.StatusBadRequest, gin.H{"error": "一次最多检测50个镜像"})
		return
	}

	// ====================================================================
	// 查询镜像记录
	// ====================================================================

	// 根据ID列表查询镜像记录
	var images []models.ImageSyncRecord
	if err := database.DB.Where("id IN ?", request.IDs).Find(&images).Error; err != nil {
		logger.Logger.Error("查询镜像列表失败", zap.Error(err))
		c.JSON(http.StatusInternalServerError, gin.H{"error": "查询镜像列表失败"})
		return
	}

	// ====================================================================
	// 初始化检测结果变量
	// ====================================================================

	// 初始化结果收集器
	results := make([]map[string]interface{}, 0, len(images))
	successCount := 0 // 存在的镜像计数
	failedCount := 0  // 不存在的镜像计数

	// ====================================================================
	// 逐个检测镜像存在性
	// ====================================================================

	for _, image := range images {
		targetImage := utils.BuildACRImageRef(image.AcrRegistryID, image.OriginalImage, image.Tag)

		// 检测镜像在注册表中的存在性
		exists, err := utils.CheckImageExistsInRegistryWithErr(targetImage, image.AcrRegistryID)
		if err != nil {
			// 检测过程中发生错误的处理
			logger.Logger.Error("检测镜像存在性失败",
				zap.Error(err),
				zap.Uint("id", image.ID),
				zap.String("target_image", targetImage))

			// 检测失败时，更新镜像状态为失败
			if updateErr := database.DB.Model(&image).Updates(map[string]interface{}{
				"sync_status":   models.SyncStatusFailed,
				"error_message": fmt.Sprintf("检测失败: %v", err),
			}).Error; updateErr != nil {
				logger.Logger.Error("更新镜像状态失败", zap.Error(updateErr), zap.Uint("id", image.ID))
			}

			// 记录检测失败的结果
			results = append(results, map[string]interface{}{
				"id":             image.ID,
				"original_image": image.OriginalImage,
				"target_image":   targetImage,
				"exists":         false,
				"error":          err.Error(),
			})
			failedCount++
			continue
		}

		// ================================================================
		// 根据检测结果更新镜像状态
		// ================================================================

		var arches []string
		if exists {
			if detected, derr := utils.DetectImageArchitecturesInRegistry(targetImage, image.AcrRegistryID); derr == nil {
				arches = detected
			}
		}
		archJSON := utils.ArchitecturesToJSON(arches)

		if exists {
			if image.SyncStatus == models.SyncStatusFailed {
				if err := database.DB.Model(&image).Updates(map[string]interface{}{
					"sync_status":         models.SyncStatusSuccess,
					"acr_image":           targetImage,
					"acr_architectures":   archJSON,
					"error_message":       "",
				}).Error; err != nil {
					logger.Logger.Error("更新镜像状态失败", zap.Error(err), zap.Uint("id", image.ID))
				}
			} else {
				_ = database.DB.Model(&image).Updates(map[string]interface{}{
					"acr_image":         targetImage,
					"acr_architectures": archJSON,
				})
			}
			successCount++
		} else {
			if err := database.DB.Model(&image).Updates(map[string]interface{}{
				"sync_status":       models.SyncStatusFailed,
				"error_message":     "镜像不存在",
				"acr_architectures": archJSON,
			}).Error; err != nil {
				logger.Logger.Error("更新镜像状态失败", zap.Error(err), zap.Uint("id", image.ID))
			}
			failedCount++
		}

		results = append(results, map[string]interface{}{
			"id":                image.ID,
			"original_image":    image.OriginalImage,
			"target_image":      targetImage,
			"exists":            exists,
			"architectures":     arches,
			"acr_architectures": archJSON,
		})
	}

	// ====================================================================
	// 记录操作日志并返回结果
	// ====================================================================

	// 记录批量检测完成的日志
	logger.Logger.Info("批量检测镜像完成",
		zap.Int("total", len(images)),
		zap.Int("success", successCount),
		zap.Int("failed", failedCount))

	// 返回批量检测的完整结果
	c.JSON(http.StatusOK, gin.H{
		"total":         len(images),                                                     // 检测总数
		"success_count": successCount,                                                    // 成功数量
		"failed_count":  failedCount,                                                     // 失败数量
		"results":       results,                                                         // 详细结果
		"message":       fmt.Sprintf("检测完成：%d个镜像存在，%d个镜像不存在", successCount, failedCount), // 结果摘要
	})
}
