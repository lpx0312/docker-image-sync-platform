package main

// 平台 API 响应中由 handler 直接拼 gin.H 的部分，
// CLI 在此定义对应的本地结构体（models 包里的结构体直接复用）。

import (
	"time"

	"docker-image-sync-platform/internal/models"
)

// 通用 {"status":"success","data":...} 包装
type envelope struct {
	Status string `json:"status"`
	Data   any    `json:"data"`
}

// AcrRegistryInfo ACR 配置（与 models.AcrRegistry 字段一致，显式声明避免耦合 gorm 模型演进）
type AcrRegistryInfo = models.AcrRegistry

// quotaSummaryItem 单个 ACR 的配额摘要（对应 services.AcrQuotaSummary 的 JSON）
type quotaSummaryItem struct {
	AcrRegistryID  uint   `json:"acr_registry_id"`
	Namespace      string `json:"namespace"`
	RegistryURL    string `json:"registry_url"`
	RepoCount      int    `json:"repo_count"`
	RepoQuota      int    `json:"repo_quota"`
	RemainingQuota int    `json:"remaining_quota"`
	IsDefault      bool   `json:"is_default"`
	IsFull         bool   `json:"is_full"`
}

// acrRegistriesResponse GET /acr-registries
type acrRegistriesResponse struct {
	Status string            `json:"status"`
	Data   []AcrRegistryInfo `json:"data"`
}

// quotaSummaryResponse GET /acr-registries/quota-summary
type quotaSummaryResponse struct {
	Status string             `json:"status"`
	Data   []quotaSummaryItem `json:"data"`
}

// acrRepositoriesResponse GET /acr-repositories
type acrRepositoriesResponse struct {
	Status string                 `json:"status"`
	Data   []models.AcrRepository `json:"data"`
}

// acrTagsResponse GET /acr-tags
type acrTagsResponse struct {
	Status string `json:"status"`
	Data   struct {
		Tags  []string `json:"tags"`
		Total int      `json:"total"`
	} `json:"data"`
}

// submitResponse POST /sync/submit 与 /sync/batch 的成功响应
type submitResponse struct {
	TaskID              string `json:"task_id"`
	Status              string `json:"status"`
	TotalImages         int    `json:"total_images"`
	EstimatedCompletion string `json:"estimated_completion"`
	Message             string `json:"message"`
}

// submitRequest POST /sync/submit 请求体（单镜像提交，不支持 target_tag）
type submitRequest struct {
	Images        []string `json:"images"`
	Architecture  string   `json:"architecture,omitempty"`
	Description   string   `json:"description,omitempty"`
	AcrRegistryID uint     `json:"acr_registry_id,omitempty"`
}

// batchImageItem 批量同步中的单个镜像项
type batchImageItem struct {
	SourceImage  string `json:"source_image"`
	TargetTag    string `json:"target_tag,omitempty"`
	Architecture string `json:"architecture,omitempty"`
}

// batchRequest POST /sync/batch 请求体
type batchRequest struct {
	Images        []batchImageItem `json:"images"`
	MaxConcurrent int              `json:"max_concurrent,omitempty"`
	AutoRetry     bool             `json:"auto_retry"`
	RetryCount    int              `json:"retry_count,omitempty"`
	AcrRegistryID uint             `json:"acr_registry_id,omitempty"`
}

// syncStatusResponse GET /sync/status/:taskId 的响应
type syncStatusResponse struct {
	TaskID          string     `json:"task_id"`
	Status          string     `json:"status"`
	TotalImages     int        `json:"total_images"`
	CompletedImages int        `json:"completed_images"`
	FailedImages    int        `json:"failed_images"`
	Progress        float64    `json:"progress"`
	GitHubActionURL string     `json:"github_action_url"`
	GitHubRunID     string     `json:"github_run_id"`
	CommitSHA       string     `json:"commit_sha"`
	ErrorMessage    string     `json:"error_message"`
	Description     string     `json:"description"`
	StartedAt       *time.Time `json:"started_at"`
	CompletedAt     *time.Time `json:"completed_at"`
	CreatedAt       time.Time  `json:"created_at"`
	Images          struct {
		Pending int                      `json:"pending"`
		Syncing int                      `json:"syncing"`
		Success int                      `json:"success"`
		Failed  int                      `json:"failed"`
		Records []models.ImageSyncRecord `json:"records"`
	} `json:"images"`
}

// historyResponse GET /sync/history 的响应
type historyResponse struct {
	Total    int64             `json:"total"`
	Page     int               `json:"page"`
	PageSize int               `json:"page_size"`
	Data     []models.SyncTask `json:"data"`
}

// checkAcrResponse POST /sync/check-acr 的响应
type checkAcrResponse struct {
	Status string       `json:"status"`
	Data   checkAcrData `json:"data"`
}

type checkAcrData struct {
	Items              []checkAcrItem `json:"items"`
	Conflicts          []checkAcrItem `json:"conflicts"`
	NewRepositoryCount int            `json:"new_repository_count"`
	HasAnyConflict     bool           `json:"has_any_conflict"`
	MultiAcrWarning    bool           `json:"multi_acr_warning"`
}

type checkAcrItem struct {
	Image              string `json:"image"`
	RepositoryName     string `json:"repository_name"`
	HasAffinity        bool   `json:"has_affinity"`
	HasConflict        bool   `json:"has_conflict"`
	SuggestedAcrID     uint   `json:"suggested_acr_id"`
	SuggestedNamespace string `json:"suggested_namespace,omitempty"`
	Message            string `json:"message,omitempty"`
	IsNewRepository    bool   `json:"is_new_repository"`
}

// suggestAcrResponse GET /sync/suggest-acr 的响应
type suggestAcrResponse struct {
	Status string         `json:"status"`
	Data   suggestAcrData `json:"data"`
}

type suggestAcrData struct {
	Affinity struct {
		RepositoryName string `json:"repository_name"`
		HasAffinity    bool   `json:"has_affinity"`
		AcrRegistryID  uint   `json:"acr_registry_id"`
		AcrNamespace   string `json:"acr_namespace"`
		RegistryURL    string `json:"registry_url"`
		Source         string `json:"source"`
	} `json:"affinity"`
	SuggestedAcrID     uint               `json:"suggested_acr_id"`
	SuggestedNamespace string             `json:"suggested_namespace"`
	SuggestionReason   string             `json:"suggestion_reason"`
	QuotaSummary       []quotaSummaryItem `json:"quota_summary"`
}

// suggestReasonText 将推荐原因枚举翻译为中文
func suggestReasonText(reason string) string {
	switch reason {
	case "affinity":
		return "仓库已有归属（亲和性）"
	case "default":
		return "平台默认 ACR"
	case "alternate":
		return "默认 ACR 已满，自动换用"
	case "default_full":
		return "默认 ACR 已满（回退使用）"
	case "fallback":
		return "无可用配额，兜底选择"
	default:
		return reason
	}
}
