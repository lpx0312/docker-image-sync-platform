package services

import (
	"errors"
	"fmt"

	"docker-image-sync-platform/internal/models"

	"gorm.io/gorm"
)

const DefaultAcrRepoQuota = 300

// repoQuotaFor 按仓库类型返回仓库配额：0 表示不限。
// 华为云 SWR 单组织内镜像仓库数量无限制（仅限制租户组织总数）；
// 腾讯云 CCR 个人版同样无仓库数量硬限制。
func repoQuotaFor(registryType string) int {
	// 仅阿里云 ACR 个人版有已知的仓库数配额；其余类型（SWR/CCR/Harbor/通用）按不限处理
	if registryType == models.RegistryTypeACR {
		return DefaultAcrRepoQuota
	}
	return 0
}

// AcrAffinityService ACR 仓库归属与配额查询服务
type AcrAffinityService struct {
	db *gorm.DB
}

// NewAcrAffinityService 创建 ACR 归属服务实例
func NewAcrAffinityService(db *gorm.DB) *AcrAffinityService {
	return &AcrAffinityService{db: db}
}

// AcrQuotaSummary 单个仓库的配额摘要（repo_quota=0 表示不限，如 SWR）
type AcrQuotaSummary struct {
	AcrRegistryID  uint   `json:"acr_registry_id"`
	Alias          string `json:"alias"`
	Namespace      string `json:"namespace"`
	RegistryURL    string `json:"registry_url"`
	RegistryType   string `json:"registry_type"`
	RepoCount      int    `json:"repo_count"`
	RepoQuota      int    `json:"repo_quota"`
	RemainingQuota int    `json:"remaining_quota"`
	IsDefault      bool   `json:"is_default"`
	IsFull         bool   `json:"is_full"`
}

// AcrAffinityResult 仓库归属查询结果
type AcrAffinityResult struct {
	RepositoryName string `json:"repository_name"`
	HasAffinity    bool   `json:"has_affinity"`
	AcrRegistryID  uint   `json:"acr_registry_id,omitempty"`
	AcrAlias       string `json:"acr_alias,omitempty"`
	AcrNamespace   string `json:"acr_namespace,omitempty"`
	RegistryURL    string `json:"registry_url,omitempty"`
	Source         string `json:"source"` // repository | sync_record | none
	RepoCount      int    `json:"repo_count,omitempty"`
	RepoQuota      int    `json:"repo_quota"`
	RemainingQuota int    `json:"remaining_quota,omitempty"`
	TagCount       int    `json:"tag_count,omitempty"`
}

// AcrResolveResult 目标 ACR 解析结果
type AcrResolveResult struct {
	Affinity           *AcrAffinityResult `json:"affinity"`
	SuggestedAcrID     uint               `json:"suggested_acr_id"`
	SuggestedAlias     string             `json:"suggested_alias,omitempty"`
	SuggestedNamespace string             `json:"suggested_namespace,omitempty"`
	SuggestionReason   string             `json:"suggestion_reason"` // affinity | default | alternate | default_full | fallback
}

// AcrConflictInfo ACR 选择冲突信息
type AcrConflictInfo struct {
	HasConflict        bool   `json:"has_conflict"`
	RepositoryName     string `json:"repository_name"`
	SelectedAcrID      uint   `json:"selected_acr_id"`
	SuggestedAcrID     uint   `json:"suggested_acr_id,omitempty"`
	SuggestedAlias     string `json:"suggested_alias,omitempty"`
	SuggestedNamespace string `json:"suggested_namespace,omitempty"`
	Message            string `json:"message,omitempty"`
}

// AcrCheckItem 批量检查的单个镜像结果
type AcrCheckItem struct {
	Image              string `json:"image"`
	RepositoryName     string `json:"repository_name"`
	HasAffinity        bool   `json:"has_affinity"`
	HasConflict        bool   `json:"has_conflict"`
	SuggestedAcrID     uint   `json:"suggested_acr_id,omitempty"`
	SuggestedAlias     string `json:"suggested_alias,omitempty"`
	SuggestedNamespace string `json:"suggested_namespace,omitempty"`
	Message            string `json:"message,omitempty"`
	IsNewRepository    bool   `json:"is_new_repository"`
}

// AcrCheckResult 批量 ACR 检查结果
type AcrCheckResult struct {
	Items              []AcrCheckItem    `json:"items"`
	Conflicts          []AcrCheckItem    `json:"conflicts"`
	NewRepositoryCount int               `json:"new_repository_count"`
	QuotaSummary       []AcrQuotaSummary `json:"quota_summary"`
	HasAnyConflict     bool              `json:"has_any_conflict"`
	MultiAcrWarning    bool              `json:"multi_acr_warning"`
	SuggestedAcrIDs    []uint            `json:"suggested_acr_ids,omitempty"`
}

// DuplicateRepositoryReport 跨 ACR 重复仓库报告项
type DuplicateRepositoryReport struct {
	RepositoryName string `json:"repository_name"`
	AcrCount       int    `json:"acr_count"`
	AcrRegistryIDs []uint `json:"acr_registry_ids"`
	Namespaces     []string `json:"namespaces"`
}

// FindAffinity 按仓库名查找跨 ACR 归属
func (s *AcrAffinityService) FindAffinity(repositoryName string) (*AcrAffinityResult, error) {
	repositoryName = ExtractRepoName(repositoryName)
	result := &AcrAffinityResult{
		RepositoryName: repositoryName,
		Source:         "none",
		RepoQuota:      DefaultAcrRepoQuota,
	}

	if repositoryName == "" {
		return result, nil
	}

	var repo models.AcrRepository
	err := s.db.Where("repository_name = ?", repositoryName).Order("id ASC").First(&repo).Error
	if err == nil {
		return s.fillAffinityFromRegistry(result, repo.AcrRegistryID, "repository")
	}
	if !errors.Is(err, gorm.ErrRecordNotFound) {
		return nil, fmt.Errorf("查询仓库归属失败: %w", err)
	}

	var records []models.ImageSyncRecord
	if err := s.db.Where("sync_status = ? AND acr_registry_id > 0", models.SyncStatusSuccess).
		Order("COALESCE(completed_at, updated_at) DESC").
		Find(&records).Error; err != nil {
		return nil, fmt.Errorf("查询同步记录失败: %w", err)
	}

	for _, record := range records {
		if ExtractRepoName(record.OriginalImage) != repositoryName {
			continue
		}
		return s.fillAffinityFromRegistry(result, record.AcrRegistryID, "sync_record")
	}

	return result, nil
}

func (s *AcrAffinityService) fillAffinityFromRegistry(result *AcrAffinityResult, acrRegistryID uint, source string) (*AcrAffinityResult, error) {
	var acr models.AcrRegistry
	if err := s.db.First(&acr, acrRegistryID).Error; err != nil {
		return nil, fmt.Errorf("获取 ACR 配置失败: %w", err)
	}

	repoCount, err := s.countRepositories(acrRegistryID)
	if err != nil {
		return nil, err
	}

	tagCount, err := s.countSuccessTags(acrRegistryID, result.RepositoryName)
	if err != nil {
		return nil, err
	}

	quota := repoQuotaFor(acr.RegistryType)
	result.HasAffinity = true
	result.AcrRegistryID = acrRegistryID
	result.AcrAlias = acr.Alias
	result.AcrNamespace = acr.Namespace
	result.RegistryURL = acr.RegistryURL
	result.Source = source
	result.RepoCount = repoCount
	result.RepoQuota = quota
	if quota > 0 {
		result.RemainingQuota = quota - repoCount
		if result.RemainingQuota < 0 {
			result.RemainingQuota = 0
		}
	}
	result.TagCount = tagCount
	return result, nil
}

func (s *AcrAffinityService) countRepositories(acrRegistryID uint) (int, error) {
	var count int64
	if err := s.db.Model(&models.AcrRepository{}).
		Where("acr_registry_id = ?", acrRegistryID).
		Count(&count).Error; err != nil {
		return 0, fmt.Errorf("统计仓库数量失败: %w", err)
	}
	return int(count), nil
}

func (s *AcrAffinityService) countSuccessTags(acrRegistryID uint, repositoryName string) (int, error) {
	var records []models.ImageSyncRecord
	if err := s.db.Where("acr_registry_id = ? AND sync_status = ?", acrRegistryID, models.SyncStatusSuccess).
		Find(&records).Error; err != nil {
		return 0, fmt.Errorf("统计 Tag 数量失败: %w", err)
	}

	count := 0
	for _, record := range records {
		if ExtractRepoName(record.OriginalImage) == repositoryName {
			count++
		}
	}
	return count, nil
}

// SuggestAcrForImage 根据源镜像地址建议 ACR
func (s *AcrAffinityService) SuggestAcrForImage(sourceImage string) (*AcrAffinityResult, error) {
	repoName := ExtractRepoName(sourceImage)
	return s.FindAffinity(repoName)
}

// ResolveTargetAcr 解析镜像应同步到的目标 ACR
// 规则：有归属用归属 ACR；无归属用默认 ACR（未满）；默认已满则用其他未满 ACR
func (s *AcrAffinityService) ResolveTargetAcr(sourceImage string) (*AcrResolveResult, error) {
	affinity, err := s.SuggestAcrForImage(sourceImage)
	if err != nil {
		return nil, err
	}

	result := &AcrResolveResult{Affinity: affinity}
	if affinity.HasAffinity {
		result.SuggestedAcrID = affinity.AcrRegistryID
		result.SuggestedAlias = affinity.AcrAlias
		result.SuggestedNamespace = affinity.AcrNamespace
		result.SuggestionReason = "affinity"
		return result, nil
	}

	quotaSummary, err := s.GetQuotaSummary()
	if err != nil {
		return nil, err
	}

	for _, item := range quotaSummary {
		if item.IsDefault && !item.IsFull {
			result.SuggestedAcrID = item.AcrRegistryID
			result.SuggestedAlias = item.Alias
			result.SuggestedNamespace = item.Namespace
			result.SuggestionReason = "default"
			return result, nil
		}
	}

	for _, item := range quotaSummary {
		if !item.IsFull {
			result.SuggestedAcrID = item.AcrRegistryID
			result.SuggestedAlias = item.Alias
			result.SuggestedNamespace = item.Namespace
			result.SuggestionReason = "alternate"
			return result, nil
		}
	}

	for _, item := range quotaSummary {
		if item.IsDefault {
			result.SuggestedAcrID = item.AcrRegistryID
			result.SuggestedAlias = item.Alias
			result.SuggestedNamespace = item.Namespace
			result.SuggestionReason = "default_full"
			return result, nil
		}
	}

	if len(quotaSummary) > 0 {
		result.SuggestedAcrID = quotaSummary[0].AcrRegistryID
		result.SuggestedAlias = quotaSummary[0].Alias
		result.SuggestedNamespace = quotaSummary[0].Namespace
		result.SuggestionReason = "fallback"
	}

	return result, nil
}

// CheckConflict 检查所选 ACR 是否与仓库历史归属冲突
func (s *AcrAffinityService) CheckConflict(repositoryName string, selectedAcrID uint) (*AcrConflictInfo, error) {
	repositoryName = ExtractRepoName(repositoryName)
	info := &AcrConflictInfo{
		RepositoryName: repositoryName,
		SelectedAcrID:  selectedAcrID,
	}

	affinity, err := s.FindAffinity(repositoryName)
	if err != nil {
		return nil, err
	}

	if !affinity.HasAffinity {
		return info, nil
	}

	if selectedAcrID == 0 || selectedAcrID == affinity.AcrRegistryID {
		return info, nil
	}

	info.HasConflict = true
	info.SuggestedAcrID = affinity.AcrRegistryID
	info.SuggestedAlias = affinity.AcrAlias
	info.SuggestedNamespace = affinity.AcrNamespace
	info.Message = fmt.Sprintf(
		"仓库 %s 已归属镜像仓库「%s」，当前选择了其他仓库，建议同步到原仓库",
		repositoryName,
		affinity.AcrAlias,
	)
	return info, nil
}

// CheckImages 批量检查镜像与所选 ACR 的冲突
func (s *AcrAffinityService) CheckImages(images []string, selectedAcrID uint) (*AcrCheckResult, error) {
	quotaSummary, err := s.GetQuotaSummary()
	if err != nil {
		return nil, err
	}

	result := &AcrCheckResult{
		Items:        make([]AcrCheckItem, 0, len(images)),
		Conflicts:    []AcrCheckItem{},
		QuotaSummary: quotaSummary,
	}

	suggestedSet := make(map[uint]struct{})

	for _, image := range images {
		resolved, err := s.ResolveTargetAcr(image)
		if err != nil {
			return nil, err
		}

		affinity := resolved.Affinity
		repoName := affinity.RepositoryName

		item := AcrCheckItem{
			Image:              image,
			RepositoryName:     repoName,
			HasAffinity:        affinity.HasAffinity,
			IsNewRepository:    !affinity.HasAffinity,
			SuggestedAcrID:     resolved.SuggestedAcrID,
			SuggestedAlias:     resolved.SuggestedAlias,
			SuggestedNamespace: resolved.SuggestedNamespace,
		}

		if affinity.HasAffinity {
			suggestedSet[resolved.SuggestedAcrID] = struct{}{}

			if selectedAcrID > 0 && selectedAcrID != resolved.SuggestedAcrID {
				item.HasConflict = true
				item.Message = fmt.Sprintf(
					"仓库「%s」已归属镜像仓库「%s」，但强制选择了其他仓库",
					repoName,
					affinity.AcrAlias,
				)
				result.Conflicts = append(result.Conflicts, item)
				result.HasAnyConflict = true
			}
		} else {
			result.NewRepositoryCount++
		}

		result.Items = append(result.Items, item)
	}

	for id := range suggestedSet {
		result.SuggestedAcrIDs = append(result.SuggestedAcrIDs, id)
	}
	result.MultiAcrWarning = len(suggestedSet) > 1

	return result, nil
}

// GetQuotaSummary 获取所有 ACR 的配额用量
func (s *AcrAffinityService) GetQuotaSummary() ([]AcrQuotaSummary, error) {
	var registries []models.AcrRegistry
	if err := s.db.Order("is_default DESC, id ASC").Find(&registries).Error; err != nil {
		return nil, fmt.Errorf("获取 ACR 列表失败: %w", err)
	}

	summary := make([]AcrQuotaSummary, 0, len(registries))
	for _, acr := range registries {
		repoCount, err := s.countRepositories(acr.ID)
		if err != nil {
			return nil, err
		}
		quota := repoQuotaFor(acr.RegistryType)
		remaining := 0
		if quota > 0 {
			remaining = quota - repoCount
			if remaining < 0 {
				remaining = 0
			}
		}
		summary = append(summary, AcrQuotaSummary{
			AcrRegistryID:  acr.ID,
			Alias:          acr.Alias,
			Namespace:      acr.Namespace,
			RegistryURL:    acr.RegistryURL,
			RegistryType:   acr.RegistryType,
			RepoCount:      repoCount,
			RepoQuota:      quota,
			RemainingQuota: remaining,
			IsDefault:      acr.IsDefault,
			IsFull:         quota > 0 && repoCount >= quota,
		})
	}
	return summary, nil
}

// GetDuplicateRepositories 查找跨 ACR 重复的仓库名
func (s *AcrAffinityService) GetDuplicateRepositories() ([]DuplicateRepositoryReport, error) {
	type row struct {
		RepositoryName string
		AcrRegistryID  uint
		Namespace      string
	}

	var rows []row
	if err := s.db.Table("acr_repositories AS r").
		Select("r.repository_name, r.acr_registry_id, a.namespace").
		Joins("JOIN acr_registries AS a ON a.id = r.acr_registry_id AND a.deleted_at IS NULL").
		Where("r.deleted_at IS NULL").
		Order("r.repository_name ASC, r.acr_registry_id ASC").
		Scan(&rows).Error; err != nil {
		return nil, fmt.Errorf("查询重复仓库失败: %w", err)
	}

	grouped := make(map[string]*DuplicateRepositoryReport)
	for _, item := range rows {
		report, ok := grouped[item.RepositoryName]
		if !ok {
			report = &DuplicateRepositoryReport{
				RepositoryName: item.RepositoryName,
				AcrRegistryIDs: []uint{},
				Namespaces:     []string{},
			}
			grouped[item.RepositoryName] = report
		}
		report.AcrRegistryIDs = append(report.AcrRegistryIDs, item.AcrRegistryID)
		report.Namespaces = append(report.Namespaces, item.Namespace)
	}

	result := make([]DuplicateRepositoryReport, 0)
	for _, report := range grouped {
		report.AcrCount = len(report.AcrRegistryIDs)
		if report.AcrCount > 1 {
			result = append(result, *report)
		}
	}
	return result, nil
}
