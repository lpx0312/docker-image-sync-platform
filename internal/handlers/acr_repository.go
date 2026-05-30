package handlers

import (
	"fmt"
	"net/http"
	"strconv"
	"strings"

	"docker-image-sync-platform/internal/database"
	"docker-image-sync-platform/internal/logger"
	"docker-image-sync-platform/internal/models"
	"docker-image-sync-platform/internal/services"

	"github.com/gin-gonic/gin"
	"go.uber.org/zap"
)

// AcrRepositoryHandler ACR镜像仓库处理器
type AcrRepositoryHandler struct {
	service *services.AcrRepositoryService
}

// NewAcrRepositoryHandler 创建ACR镜像仓库处理器实例
func NewAcrRepositoryHandler(service *services.AcrRepositoryService) *AcrRepositoryHandler {
	return &AcrRepositoryHandler{service: service}
}

// GetAll 获取所有镜像
func (h *AcrRepositoryHandler) GetAll(c *gin.Context) {
	acrRegistryIDStr := c.Query("acr_registry_id")
	if acrRegistryIDStr == "" {
		c.JSON(http.StatusBadRequest, gin.H{"status": "error", "message": "缺少 acr_registry_id 参数"})
		return
	}

	acrRegistryID, err := strconv.ParseUint(acrRegistryIDStr, 10, 32)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"status": "error", "message": "无效的 acr_registry_id"})
		return
	}

	repos, err := h.service.GetAll(uint(acrRegistryID))
	if err != nil {
		logger.Logger.Error("获取镜像列表失败", zap.Error(err))
		c.JSON(http.StatusInternalServerError, gin.H{"status": "error", "message": err.Error()})
		return
	}

	c.JSON(http.StatusOK, gin.H{"status": "success", "data": repos})
}

// Create 创建镜像
func (h *AcrRepositoryHandler) Create(c *gin.Context) {
	var req models.AcrRepositoryRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"status": "error", "message": "请求参数错误: " + err.Error()})
		return
	}

	repo, err := h.service.Create(&req)
	if err != nil {
		logger.Logger.Error("创建镜像失败", zap.Error(err))
		c.JSON(http.StatusInternalServerError, gin.H{"status": "error", "message": err.Error()})
		return
	}

	c.JSON(http.StatusCreated, gin.H{"status": "success", "data": repo})
}

// BatchCreate 批量创建镜像
func (h *AcrRepositoryHandler) BatchCreate(c *gin.Context) {
	var req models.AcrRepositoryBatchRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"status": "error", "message": "请求参数错误: " + err.Error()})
		return
	}

	created, err := h.service.BatchCreate(&req)
	if err != nil {
		logger.Logger.Error("批量创建镜像失败", zap.Error(err))
		c.JSON(http.StatusInternalServerError, gin.H{"status": "error", "message": err.Error()})
		return
	}

	message := fmt.Sprintf("成功添加 %d 个镜像", created.Created)
	if len(created.DuplicateInInput) > 0 {
		message += fmt.Sprintf("，输入重复 %d 个：%s", len(created.DuplicateInInput), strings.Join(created.DuplicateInInput, "、"))
	}
	if len(created.AlreadyExistNames) > 0 {
		message += fmt.Sprintf("，本地已存在 %d 个：%s", len(created.AlreadyExistNames), strings.Join(created.AlreadyExistNames, "、"))
	}
	if len(created.MissingInACR) > 0 {
		message += fmt.Sprintf("，ACR 中不存在 %d 个：%s", len(created.MissingInACR), strings.Join(created.MissingInACR, "、"))
	}
	if len(created.CheckFailedNames) > 0 {
		message += fmt.Sprintf("，检查失败 %d 个：%s", len(created.CheckFailedNames), strings.Join(created.CheckFailedNames, "、"))
	}

	c.JSON(http.StatusOK, gin.H{
		"status":  "success",
		"message": message,
		"data":    created,
	})
}

// BatchDelete 批量删除镜像
func (h *AcrRepositoryHandler) BatchDelete(c *gin.Context) {
	var req models.AcrRepositoryBatchDeleteRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"status": "error", "message": "请求参数错误: " + err.Error()})
		return
	}

	deleted, err := h.service.BatchDelete(req.IDs)
	if err != nil {
		logger.Logger.Error("批量删除镜像失败", zap.Error(err))
		c.JSON(http.StatusInternalServerError, gin.H{"status": "error", "message": err.Error()})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"status":  "success",
		"message": fmt.Sprintf("成功删除 %d 个镜像", deleted),
		"data": gin.H{
			"deleted": deleted,
		},
	})
}

// CleanInvalid 清理本地存在但 ACR 中不存在的镜像
func (h *AcrRepositoryHandler) CleanInvalid(c *gin.Context) {
	var req models.AcrRepositoryCleanInvalidRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"status": "error", "message": "请求参数错误: " + err.Error()})
		return
	}

	result, err := h.service.CleanInvalid(req.AcrRegistryID)
	if err != nil {
		logger.Logger.Error("清理无效镜像失败", zap.Error(err))
		c.JSON(http.StatusInternalServerError, gin.H{"status": "error", "message": err.Error()})
		return
	}

	message := fmt.Sprintf("清理 %d 个无效镜像", result.Cleaned)
	if len(result.CheckFailedNames) > 0 {
		message += fmt.Sprintf("，%d 个检查失败未清理", len(result.CheckFailedNames))
	}

	c.JSON(http.StatusOK, gin.H{
		"status":  "success",
		"message": message,
		"data":    result,
	})
}

// Delete 删除镜像
func (h *AcrRepositoryHandler) Delete(c *gin.Context) {
	idStr := c.Param("id")
	id, err := strconv.ParseUint(idStr, 10, 32)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"status": "error", "message": "无效的ID"})
		return
	}

	if err := h.service.Delete(uint(id)); err != nil {
		logger.Logger.Error("删除镜像失败", zap.Error(err))
		c.JSON(http.StatusInternalServerError, gin.H{"status": "error", "message": err.Error()})
		return
	}

	c.JSON(http.StatusOK, gin.H{"status": "success", "message": "删除成功"})
}

// SyncFromRecords 从同步记录导入
func (h *AcrRepositoryHandler) SyncFromRecords(c *gin.Context) {
	var req models.AcrRepositorySyncRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"status": "error", "message": "请求参数错误: " + err.Error()})
		return
	}

	created, err := h.service.SyncFromRecords(req.AcrRegistryID)
	if err != nil {
		logger.Logger.Error("从同步记录导入失败", zap.Error(err))
		c.JSON(http.StatusInternalServerError, gin.H{"status": "error", "message": err.Error()})
		return
	}

	message := fmt.Sprintf("成功导入 %d 个镜像", created.Created)
	if len(created.MissingInACR) > 0 {
		message += fmt.Sprintf("，%d 个在 ACR 中不存在：%s", len(created.MissingInACR), strings.Join(created.MissingInACR, "、"))
	}
	if len(created.CheckFailedNames) > 0 {
		message += fmt.Sprintf("，%d 个检查失败：%s", len(created.CheckFailedNames), strings.Join(created.CheckFailedNames, "、"))
	}
	if created.AlreadyExist > 0 {
		message += fmt.Sprintf("，%d 个已存在", created.AlreadyExist)
	}

	c.JSON(http.StatusOK, gin.H{
		"status":  "success",
		"message": message,
		"data":    created,
	})
}

// GetDuplicates 获取跨 ACR 重复的仓库名
func (h *AcrRepositoryHandler) GetDuplicates(c *gin.Context) {
	affinitySvc := services.NewAcrAffinityService(database.DB)
	duplicates, err := affinitySvc.GetDuplicateRepositories()
	if err != nil {
		logger.Logger.Error("查询跨 ACR 重复仓库失败", zap.Error(err))
		c.JSON(http.StatusInternalServerError, gin.H{"status": "error", "message": err.Error()})
		return
	}

	c.JSON(http.StatusOK, gin.H{"status": "success", "data": duplicates})
}
