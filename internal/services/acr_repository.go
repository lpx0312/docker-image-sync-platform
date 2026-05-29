package services

import (
	"errors"
	"fmt"
	"strings"

	"docker-image-sync-platform/internal/models"

	"gorm.io/gorm"
)

// AcrRepositoryService ACR镜像仓库服务
type AcrRepositoryService struct {
	db *gorm.DB
}

// NewAcrRepositoryService 创建ACR镜像仓库服务实例
func NewAcrRepositoryService(db *gorm.DB) *AcrRepositoryService {
	return &AcrRepositoryService{db: db}
}

// GetAll 获取指定ACR的所有镜像
func (s *AcrRepositoryService) GetAll(acrRegistryID uint) ([]models.AcrRepository, error) {
	var repos []models.AcrRepository
	if err := s.db.Where("acr_registry_id = ?", acrRegistryID).
		Order("repository_name ASC").
		Find(&repos).Error; err != nil {
		return nil, fmt.Errorf("获取镜像列表失败: %w", err)
	}
	return repos, nil
}

// GetByID 根据ID获取镜像
func (s *AcrRepositoryService) GetByID(id uint) (*models.AcrRepository, error) {
	var repo models.AcrRepository
	if err := s.db.First(&repo, id).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, fmt.Errorf("镜像不存在: %d", id)
		}
		return nil, fmt.Errorf("获取镜像失败: %w", err)
	}
	return &repo, nil
}

// Create 创建镜像
func (s *AcrRepositoryService) Create(req *models.AcrRepositoryRequest) (*models.AcrRepository, error) {
	// 检查是否已存在
	var count int64
	s.db.Model(&models.AcrRepository{}).
		Where("acr_registry_id = ? AND repository_name = ?", req.AcrRegistryID, req.RepositoryName).
		Count(&count)
	if count > 0 {
		return nil, fmt.Errorf("镜像已存在: %s", req.RepositoryName)
	}

	repo := &models.AcrRepository{
		AcrRegistryID:  req.AcrRegistryID,
		RepositoryName: req.RepositoryName,
	}

	if err := s.db.Create(repo).Error; err != nil {
		return nil, fmt.Errorf("创建镜像失败: %w", err)
	}

	return repo, nil
}

// BatchCreate 批量创建镜像
func (s *AcrRepositoryService) BatchCreate(req *models.AcrRepositoryBatchRequest) (int, error) {
	created := 0
	for _, name := range req.RepositoryNames {
		name = strings.TrimSpace(name)
		if name == "" {
			continue
		}

		// 检查是否已存在
		var count int64
		s.db.Model(&models.AcrRepository{}).
			Where("acr_registry_id = ? AND repository_name = ?", req.AcrRegistryID, name).
			Count(&count)
		if count > 0 {
			continue
		}

		repo := &models.AcrRepository{
			AcrRegistryID:  req.AcrRegistryID,
			RepositoryName: name,
		}

		if err := s.db.Create(repo).Error; err != nil {
			continue
		}
		created++
	}

	return created, nil
}

// Delete 删除镜像
func (s *AcrRepositoryService) Delete(id uint) error {
	if err := s.db.Delete(&models.AcrRepository{}, id).Error; err != nil {
		return fmt.Errorf("删除镜像失败: %w", err)
	}
	return nil
}

// SyncFromRecords 从同步记录中提取镜像名称
func (s *AcrRepositoryService) SyncFromRecords(acrRegistryID uint) (int, error) {
	// 查询指定 ACR 的成功同步记录
	var records []models.ImageSyncRecord
	if err := s.db.Where("acr_registry_id = ? AND sync_status = ?", acrRegistryID, models.SyncStatusSuccess).
		Find(&records).Error; err != nil {
		return 0, fmt.Errorf("查询同步记录失败: %w", err)
	}

	created := 0
	for _, record := range records {
		// 提取镜像名称（不含 tag）
		repoName := extractRepoName(record.OriginalImage)
		if repoName == "" {
			continue
		}

		// 检查是否已存在
		var count int64
		s.db.Model(&models.AcrRepository{}).
			Where("acr_registry_id = ? AND repository_name = ?", acrRegistryID, repoName).
			Count(&count)
		if count > 0 {
			continue
		}

		repo := &models.AcrRepository{
			AcrRegistryID:  acrRegistryID,
			RepositoryName: repoName,
		}

		if err := s.db.Create(repo).Error; err != nil {
			continue
		}
		created++
	}

	return created, nil
}

// extractRepoName 从镜像地址中提取仓库名称（不含 tag 和 registry/命名空间前缀）
// 与 utils.GenerateACRImage 的命名规则一致：取路径最后一段
func extractRepoName(image string) string {
	image = strings.TrimSpace(image)
	if image == "" {
		return ""
	}

	// 移除 tag（OriginalImage 通常不含 tag，此处作防御性处理）
	if idx := strings.LastIndex(image, ":"); idx != -1 {
		afterColon := image[idx+1:]
		if !strings.Contains(afterColon, "/") {
			image = image[:idx]
		}
	}

	// 如 gitlab/gitlab-ce -> gitlab-ce，gcr.io/google-containers/pause -> pause
	if strings.Contains(image, "/") {
		parts := strings.Split(image, "/")
		return parts[len(parts)-1]
	}

	return image
}
