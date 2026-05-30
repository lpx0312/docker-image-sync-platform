package services

import (
	"errors"
	"fmt"
	"sort"
	"strings"

	"docker-image-sync-platform/internal/logger"
	"docker-image-sync-platform/internal/models"

	"go.uber.org/zap"
	"gorm.io/gorm"
)

// SyncFromRecordsResult 从同步记录导入的结果
type SyncFromRecordsResult struct {
	Created           int      `json:"created"`
	Skipped           int      `json:"skipped"`
	AlreadyExist      int      `json:"already_exist"`
	CreatedNames      []string `json:"created_names"`
	MissingInACR      []string `json:"missing_in_acr"`
	CheckFailedNames  []string `json:"check_failed_names"`
	AlreadyExistNames []string `json:"already_exist_names"`
}

// BatchCreateResult 批量添加镜像的结果
type BatchCreateResult struct {
	Created           int      `json:"created"`
	AlreadyExist      int      `json:"already_exist"`
	CreatedNames      []string `json:"created_names"`
	MissingInACR      []string `json:"missing_in_acr"`
	CheckFailedNames  []string `json:"check_failed_names"`
	AlreadyExistNames []string `json:"already_exist_names"`
	DuplicateInInput  []string `json:"duplicate_in_input"`
}

// AcrRepositoryService ACR镜像仓库服务
type AcrRepositoryService struct {
	db            *gorm.DB
	acrAPIService *AcrAPIService
	encryptionSvc *EncryptionService
}

// NewAcrRepositoryService 创建ACR镜像仓库服务实例
func NewAcrRepositoryService(db *gorm.DB, acrAPIService *AcrAPIService, encryptionSvc *EncryptionService) *AcrRepositoryService {
	return &AcrRepositoryService{
		db:            db,
		acrAPIService: acrAPIService,
		encryptionSvc: encryptionSvc,
	}
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

// BatchCreate 批量创建镜像，校验本地重复与 ACR 是否存在
func (s *AcrRepositoryService) BatchCreate(req *models.AcrRepositoryBatchRequest) (*BatchCreateResult, error) {
	var acr models.AcrRegistry
	if err := s.db.First(&acr, req.AcrRegistryID).Error; err != nil {
		return nil, fmt.Errorf("ACR配置不存在: %w", err)
	}

	password, err := s.encryptionSvc.Decrypt(acr.Password)
	if err != nil {
		return nil, fmt.Errorf("解密ACR密码失败: %w", err)
	}

	result := &BatchCreateResult{
		CreatedNames:      []string{},
		MissingInACR:      []string{},
		CheckFailedNames:  []string{},
		AlreadyExistNames: []string{},
		DuplicateInInput:  []string{},
	}

	seen := make(map[string]struct{})
	names := make([]string, 0, len(req.RepositoryNames))
	for _, rawName := range req.RepositoryNames {
		name := strings.TrimSpace(rawName)
		if name == "" {
			continue
		}
		if _, ok := seen[name]; ok {
			result.DuplicateInInput = append(result.DuplicateInInput, name)
			continue
		}
		seen[name] = struct{}{}
		names = append(names, name)
	}
	sort.Strings(result.DuplicateInInput)

	for _, repoName := range names {
		var count int64
		s.db.Model(&models.AcrRepository{}).
			Where("acr_registry_id = ? AND repository_name = ?", req.AcrRegistryID, repoName).
			Count(&count)
		if count > 0 {
			result.AlreadyExist++
			result.AlreadyExistNames = append(result.AlreadyExistNames, repoName)
			continue
		}

		exists, err := s.acrAPIService.RepositoryExists(
			acr.RegistryURL, acr.Username, password, acr.Namespace, repoName,
			acr.AuthServer, acr.DockerService,
		)
		if err != nil {
			if s.acrAPIService.IsRepositoryNotFound(err) {
				logger.Logger.Info("ACR中不存在该仓库，跳过添加",
					zap.String("repository", repoName))
				result.MissingInACR = append(result.MissingInACR, repoName)
				continue
			}
			logger.Logger.Warn("检查ACR仓库是否存在失败，跳过添加",
				zap.String("repository", repoName),
				zap.Error(err))
			result.CheckFailedNames = append(result.CheckFailedNames, repoName)
			continue
		}
		if !exists {
			logger.Logger.Info("ACR中不存在该仓库，跳过添加",
				zap.String("repository", repoName))
			result.MissingInACR = append(result.MissingInACR, repoName)
			continue
		}

		repo := &models.AcrRepository{
			AcrRegistryID:  req.AcrRegistryID,
			RepositoryName: repoName,
		}
		if err := s.db.Create(repo).Error; err != nil {
			logger.Logger.Warn("创建镜像记录失败，跳过",
				zap.String("repository", repoName),
				zap.Error(err))
			result.CheckFailedNames = append(result.CheckFailedNames, repoName)
			continue
		}

		result.Created++
		result.CreatedNames = append(result.CreatedNames, repoName)
	}

	return result, nil
}

// Delete 删除镜像
func (s *AcrRepositoryService) Delete(id uint) error {
	if err := s.db.Delete(&models.AcrRepository{}, id).Error; err != nil {
		return fmt.Errorf("删除镜像失败: %w", err)
	}
	return nil
}

// EnsureRepository 确保仓库台账中存在指定仓库（同步成功后自动登记）
func (s *AcrRepositoryService) EnsureRepository(acrRegistryID uint, repositoryName string) error {
	repositoryName = strings.TrimSpace(repositoryName)
	if acrRegistryID == 0 || repositoryName == "" {
		return nil
	}

	var count int64
	if err := s.db.Model(&models.AcrRepository{}).
		Where("acr_registry_id = ? AND repository_name = ?", acrRegistryID, repositoryName).
		Count(&count).Error; err != nil {
		return fmt.Errorf("检查仓库台账失败: %w", err)
	}
	if count > 0 {
		return nil
	}

	repo := &models.AcrRepository{
		AcrRegistryID:  acrRegistryID,
		RepositoryName: repositoryName,
	}
	if err := s.db.Create(repo).Error; err != nil {
		return fmt.Errorf("登记仓库台账失败: %w", err)
	}
	return nil
}

// SyncFromRecords 从同步记录中提取镜像名称，仅导入 ACR 中仍存在的仓库
func (s *AcrRepositoryService) SyncFromRecords(acrRegistryID uint) (*SyncFromRecordsResult, error) {
	var acr models.AcrRegistry
	if err := s.db.First(&acr, acrRegistryID).Error; err != nil {
		return nil, fmt.Errorf("ACR配置不存在: %w", err)
	}

	password, err := s.encryptionSvc.Decrypt(acr.Password)
	if err != nil {
		return nil, fmt.Errorf("解密ACR密码失败: %w", err)
	}

	var records []models.ImageSyncRecord
	if err := s.db.Where("acr_registry_id = ? AND sync_status = ?", acrRegistryID, models.SyncStatusSuccess).
		Find(&records).Error; err != nil {
		return nil, fmt.Errorf("查询同步记录失败: %w", err)
	}

	repoNames := make(map[string]struct{})
	for _, record := range records {
		repoName := ExtractRepoName(record.OriginalImage)
		if repoName != "" {
			repoNames[repoName] = struct{}{}
		}
	}

	result := &SyncFromRecordsResult{
		CreatedNames:      []string{},
		MissingInACR:      []string{},
		CheckFailedNames:  []string{},
		AlreadyExistNames: []string{},
	}

	names := make([]string, 0, len(repoNames))
	for name := range repoNames {
		names = append(names, name)
	}
	sort.Strings(names)

	for _, repoName := range names {
		var count int64
		s.db.Model(&models.AcrRepository{}).
			Where("acr_registry_id = ? AND repository_name = ?", acrRegistryID, repoName).
			Count(&count)
		if count > 0 {
			result.AlreadyExist++
			result.AlreadyExistNames = append(result.AlreadyExistNames, repoName)
			continue
		}

		exists, err := s.acrAPIService.RepositoryExists(
			acr.RegistryURL, acr.Username, password, acr.Namespace, repoName,
			acr.AuthServer, acr.DockerService,
		)
		if err != nil {
			logger.Logger.Warn("检查ACR仓库是否存在失败，跳过导入",
				zap.String("repository", repoName),
				zap.Error(err))
			result.Skipped++
			result.CheckFailedNames = append(result.CheckFailedNames, repoName)
			continue
		}
		if !exists {
			logger.Logger.Info("ACR中不存在该仓库，跳过导入",
				zap.String("repository", repoName))
			result.Skipped++
			result.MissingInACR = append(result.MissingInACR, repoName)
			continue
		}

		repo := &models.AcrRepository{
			AcrRegistryID:  acrRegistryID,
			RepositoryName: repoName,
		}
		if err := s.db.Create(repo).Error; err != nil {
			logger.Logger.Warn("创建镜像记录失败，跳过",
				zap.String("repository", repoName),
				zap.Error(err))
			continue
		}
		result.Created++
		result.CreatedNames = append(result.CreatedNames, repoName)
	}

	return result, nil
}

// ExtractRepoName 从镜像地址中提取仓库名称（不含 tag 和 registry/命名空间前缀）
// 与 utils.GenerateACRImage 的命名规则一致：取路径最后一段
func ExtractRepoName(image string) string {
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
