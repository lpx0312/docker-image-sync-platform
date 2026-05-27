package handlers

import (
	"fmt"
	"net/http"
	"strconv"

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

	c.JSON(http.StatusOK, gin.H{
		"status":  "success",
		"message": fmt.Sprintf("成功添加 %d 个镜像", created),
		"data":    gin.H{"created": created},
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

	c.JSON(http.StatusOK, gin.H{
		"status":  "success",
		"message": fmt.Sprintf("成功导入 %d 个镜像", created),
		"data":    gin.H{"created": created},
	})
}
