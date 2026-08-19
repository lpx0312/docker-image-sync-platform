package handlers

import (
	"net/http"
	"strconv"

	"docker-image-sync-platform/internal/database"
	"docker-image-sync-platform/internal/logger"
	"docker-image-sync-platform/internal/models"
	"docker-image-sync-platform/internal/services"

	"github.com/gin-gonic/gin"
	"go.uber.org/zap"
)

// AcrRegistryHandler ACR配置处理器
type AcrRegistryHandler struct {
	service *services.AcrRegistryService
}

// NewAcrRegistryHandler 创建ACR配置处理器实例
func NewAcrRegistryHandler(service *services.AcrRegistryService) *AcrRegistryHandler {
	return &AcrRegistryHandler{service: service}
}

// GetAll 获取所有ACR配置
func (h *AcrRegistryHandler) GetAll(c *gin.Context) {
	registries, err := h.service.GetAll()
	if err != nil {
		logger.Logger.Error("获取ACR配置列表失败", zap.Error(err))
		c.JSON(http.StatusInternalServerError, gin.H{"status": "error", "message": err.Error()})
		return
	}

	c.JSON(http.StatusOK, gin.H{"status": "success", "data": registries})
}

// GetByID 根据ID获取ACR配置
func (h *AcrRegistryHandler) GetByID(c *gin.Context) {
	id, err := strconv.ParseUint(c.Param("id"), 10, 32)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"status": "error", "message": "无效的ID"})
		return
	}

	registry, err := h.service.GetByID(uint(id))
	if err != nil {
		logger.Logger.Error("获取ACR配置失败", zap.Error(err))
		c.JSON(http.StatusNotFound, gin.H{"status": "error", "message": err.Error()})
		return
	}

	c.JSON(http.StatusOK, gin.H{"status": "success", "data": registry})
}

// GetDefault 获取默认ACR配置
func (h *AcrRegistryHandler) GetDefault(c *gin.Context) {
	registry, err := h.service.GetDefault()
	if err != nil {
		logger.Logger.Error("获取默认ACR失败", zap.Error(err))
		c.JSON(http.StatusNotFound, gin.H{"status": "error", "message": err.Error()})
		return
	}

	c.JSON(http.StatusOK, gin.H{"status": "success", "data": registry})
}

// Create 创建ACR配置
func (h *AcrRegistryHandler) Create(c *gin.Context) {
	var req models.AcrRegistryRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"status": "error", "message": "请求参数错误: " + err.Error()})
		return
	}

	registry, err := h.service.Create(&req)
	if err != nil {
		logger.Logger.Error("创建ACR配置失败", zap.Error(err))
		c.JSON(http.StatusInternalServerError, gin.H{"status": "error", "message": err.Error()})
		return
	}

	c.JSON(http.StatusCreated, gin.H{"status": "success", "data": registry})
}

// Update 更新ACR配置
func (h *AcrRegistryHandler) Update(c *gin.Context) {
	id, err := strconv.ParseUint(c.Param("id"), 10, 32)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"status": "error", "message": "无效的ID"})
		return
	}

	var req models.AcrRegistryUpdateRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"status": "error", "message": "请求参数错误: " + err.Error()})
		return
	}

	registry, err := h.service.Update(uint(id), &req)
	if err != nil {
		logger.Logger.Error("更新ACR配置失败", zap.Error(err))
		c.JSON(http.StatusInternalServerError, gin.H{"status": "error", "message": err.Error()})
		return
	}

	c.JSON(http.StatusOK, gin.H{"status": "success", "data": registry})
}

// Delete 删除ACR配置
func (h *AcrRegistryHandler) Delete(c *gin.Context) {
	id, err := strconv.ParseUint(c.Param("id"), 10, 32)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"status": "error", "message": "无效的ID"})
		return
	}

	if err := h.service.Delete(uint(id)); err != nil {
		logger.Logger.Error("删除ACR配置失败", zap.Error(err))
		c.JSON(http.StatusInternalServerError, gin.H{"status": "error", "message": err.Error()})
		return
	}

	c.JSON(http.StatusOK, gin.H{"status": "success", "message": "删除成功"})
}

// SetDefault 设置默认ACR
func (h *AcrRegistryHandler) SetDefault(c *gin.Context) {
	id, err := strconv.ParseUint(c.Param("id"), 10, 32)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"status": "error", "message": "无效的ID"})
		return
	}

	if err := h.service.SetDefault(uint(id)); err != nil {
		logger.Logger.Error("设置默认ACR失败", zap.Error(err))
		c.JSON(http.StatusInternalServerError, gin.H{"status": "error", "message": err.Error()})
		return
	}

	c.JSON(http.StatusOK, gin.H{"status": "success", "message": "默认ACR已更新"})
}

// TestConnection 测试仓库配置连通性（登录凭证 + SWR 管理面 AK/SK）
func (h *AcrRegistryHandler) TestConnection(c *gin.Context) {
	id, err := strconv.ParseUint(c.Param("id"), 10, 32)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"status": "error", "message": "无效的ID"})
		return
	}

	result, err := h.service.TestConnection(uint(id))
	if err != nil {
		logger.Logger.Error("测试仓库连通性失败", zap.Error(err))
		c.JSON(http.StatusInternalServerError, gin.H{"status": "error", "message": err.Error()})
		return
	}

	c.JSON(http.StatusOK, gin.H{"status": "success", "data": result})
}

// GetQuotaSummary 获取所有 ACR 的仓库配额用量
func (h *AcrRegistryHandler) GetQuotaSummary(c *gin.Context) {
	affinitySvc := services.NewAcrAffinityService(database.DB)
	summary, err := affinitySvc.GetQuotaSummary()
	if err != nil {
		logger.Logger.Error("获取 ACR 配额摘要失败", zap.Error(err))
		c.JSON(http.StatusInternalServerError, gin.H{"status": "error", "message": err.Error()})
		return
	}

	c.JSON(http.StatusOK, gin.H{"status": "success", "data": summary})
}
