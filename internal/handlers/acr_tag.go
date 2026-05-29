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

// AcrTagHandler ACR Tag查询处理器
type AcrTagHandler struct {
	acrAPIService *services.AcrAPIService
	encryptionSvc *services.EncryptionService
}

// NewAcrTagHandler 创建ACR Tag查询处理器实例
func NewAcrTagHandler(acrAPIService *services.AcrAPIService, encryptionSvc *services.EncryptionService) *AcrTagHandler {
	return &AcrTagHandler{
		acrAPIService: acrAPIService,
		encryptionSvc: encryptionSvc,
	}
}

// getAcrConfig 获取 ACR 配置并解密密码
func (h *AcrTagHandler) getAcrConfig(acrRegistryID uint) (*models.AcrRegistry, string, error) {
	var acr models.AcrRegistry
	if err := database.DB.First(&acr, acrRegistryID).Error; err != nil {
		return nil, "", err
	}

	password, err := h.encryptionSvc.Decrypt(acr.Password)
	if err != nil {
		return nil, "", err
	}

	return &acr, password, nil
}

// GetTags 获取镜像的Tag列表
func (h *AcrTagHandler) GetTags(c *gin.Context) {
	acrRegistryIDStr := c.Query("acr_registry_id")
	repositoryName := c.Query("repository_name")

	if acrRegistryIDStr == "" || repositoryName == "" {
		c.JSON(http.StatusBadRequest, gin.H{"status": "error", "message": "缺少必要参数"})
		return
	}

	acrRegistryID, err := strconv.ParseUint(acrRegistryIDStr, 10, 32)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"status": "error", "message": "无效的 acr_registry_id"})
		return
	}

	// 获取 ACR 配置
	acr, password, err := h.getAcrConfig(uint(acrRegistryID))
	if err != nil {
		c.JSON(http.StatusNotFound, gin.H{"status": "error", "message": "ACR配置不存在"})
		return
	}

	// 获取 Tag 列表及详细信息（服务端批量拉取，避免前端 N+1 触发 ACR 限流）
	tags, err := h.acrAPIService.GetTagsWithDetails(acr.RegistryURL, acr.Username, password, acr.Namespace, repositoryName, acr.AuthServer, acr.DockerService)
	if err != nil {
		logger.Logger.Error("获取Tag列表失败", zap.Error(err))
		c.JSON(http.StatusInternalServerError, gin.H{"status": "error", "message": err.Error()})
		return
	}

	c.JSON(http.StatusOK, gin.H{"status": "success", "data": tags})
}

// GetTagDetail 获取Tag详细信息
func (h *AcrTagHandler) GetTagDetail(c *gin.Context) {
	acrRegistryIDStr := c.Query("acr_registry_id")
	repositoryName := c.Query("repository_name")
	tag := c.Query("tag")

	if acrRegistryIDStr == "" || repositoryName == "" || tag == "" {
		c.JSON(http.StatusBadRequest, gin.H{"status": "error", "message": "缺少必要参数"})
		return
	}

	acrRegistryID, err := strconv.ParseUint(acrRegistryIDStr, 10, 32)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"status": "error", "message": "无效的 acr_registry_id"})
		return
	}

	// 获取 ACR 配置
	acr, password, err := h.getAcrConfig(uint(acrRegistryID))
	if err != nil {
		c.JSON(http.StatusNotFound, gin.H{"status": "error", "message": "ACR配置不存在"})
		return
	}

	// 获取 Tag 详细信息
	detail, err := h.acrAPIService.GetTagDetail(acr.RegistryURL, acr.Username, password, acr.Namespace, repositoryName, tag, acr.AuthServer, acr.DockerService)
	if err != nil {
		logger.Logger.Error("获取Tag详细信息失败", zap.Error(err))
		c.JSON(http.StatusInternalServerError, gin.H{"status": "error", "message": err.Error()})
		return
	}

	c.JSON(http.StatusOK, gin.H{"status": "success", "data": detail})
}
