package handlers

import (
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

// AcrTagHandler 镜像仓库 Tag 查询处理器（ACR / SWR 通用，按仓库类型分发客户端）
type AcrTagHandler struct {
	encryptionSvc *services.EncryptionService
}

// NewAcrTagHandler 创建Tag查询处理器实例
func NewAcrTagHandler(encryptionSvc *services.EncryptionService) *AcrTagHandler {
	return &AcrTagHandler{
		encryptionSvc: encryptionSvc,
	}
}

// getAcrConfig 获取镜像仓库配置并解密密码，同时返回对应类型的数据面 API 客户端
func (h *AcrTagHandler) getAcrConfig(acrRegistryID uint) (*models.AcrRegistry, string, services.RegistryAPIClient, error) {
	var acr models.AcrRegistry
	if err := database.DB.First(&acr, acrRegistryID).Error; err != nil {
		return nil, "", nil, err
	}

	password, err := h.encryptionSvc.Decrypt(acr.Password)
	if err != nil {
		return nil, "", nil, err
	}

	return &acr, password, services.NewRegistryAPIService(acr.RegistryType, acr.RegistryURL), nil
}

// GetTags 获取镜像的 Tag 名称列表（轻量，不含 manifest 详情）
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

	acr, password, apiClient, err := h.getAcrConfig(uint(acrRegistryID))
	if err != nil {
		c.JSON(http.StatusNotFound, gin.H{"status": "error", "message": "镜像仓库配置不存在"})
		return
	}

	tagNames, err := apiClient.GetTagNames(acr.RegistryURL, acr.Username, password, acr.Namespace, repositoryName, acr.AuthServer, acr.DockerService)
	if err != nil {
		logger.Logger.Error("获取Tag列表失败", zap.Error(err))
		c.JSON(http.StatusInternalServerError, gin.H{"status": "error", "message": err.Error()})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"status": "success",
		"data": gin.H{
			"tags":  tagNames,
			"total": len(tagNames),
		},
	})
}

// GetTagsDetails 批量获取指定 Tag 的详细信息
func (h *AcrTagHandler) GetTagsDetails(c *gin.Context) {
	acrRegistryIDStr := c.Query("acr_registry_id")
	repositoryName := c.Query("repository_name")
	tagsParam := c.Query("tags")

	if acrRegistryIDStr == "" || repositoryName == "" || tagsParam == "" {
		c.JSON(http.StatusBadRequest, gin.H{"status": "error", "message": "缺少必要参数"})
		return
	}

	acrRegistryID, err := strconv.ParseUint(acrRegistryIDStr, 10, 32)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"status": "error", "message": "无效的 acr_registry_id"})
		return
	}

	tagNames := splitTagsParam(tagsParam)
	if len(tagNames) == 0 {
		c.JSON(http.StatusBadRequest, gin.H{"status": "error", "message": "tags 参数无效"})
		return
	}

	acr, password, apiClient, err := h.getAcrConfig(uint(acrRegistryID))
	if err != nil {
		c.JSON(http.StatusNotFound, gin.H{"status": "error", "message": "镜像仓库配置不存在"})
		return
	}

	details, err := apiClient.GetTagsDetailsBatch(acr.RegistryURL, acr.Username, password, acr.Namespace, repositoryName, acr.AuthServer, acr.DockerService, tagNames)
	if err != nil {
		logger.Logger.Error("批量获取Tag详情失败", zap.Error(err))
		c.JSON(http.StatusInternalServerError, gin.H{"status": "error", "message": err.Error()})
		return
	}

	c.JSON(http.StatusOK, gin.H{"status": "success", "data": details})
}

func splitTagsParam(tagsParam string) []string {
	parts := strings.Split(tagsParam, ",")
	tagNames := make([]string, 0, len(parts))
	for _, part := range parts {
		tag := strings.TrimSpace(part)
		if tag != "" {
			tagNames = append(tagNames, tag)
		}
	}
	return tagNames
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

	// 获取镜像仓库配置
	acr, password, apiClient, err := h.getAcrConfig(uint(acrRegistryID))
	if err != nil {
		c.JSON(http.StatusNotFound, gin.H{"status": "error", "message": "镜像仓库配置不存在"})
		return
	}

	// 获取 Tag 详细信息
	detail, err := apiClient.GetTagDetail(acr.RegistryURL, acr.Username, password, acr.Namespace, repositoryName, tag, acr.AuthServer, acr.DockerService)
	if err != nil {
		logger.Logger.Error("获取Tag详细信息失败", zap.Error(err))
		c.JSON(http.StatusInternalServerError, gin.H{"status": "error", "message": err.Error()})
		return
	}

	c.JSON(http.StatusOK, gin.H{"status": "success", "data": detail})
}
