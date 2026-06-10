package models

import (
	"time"

	"gorm.io/gorm"
)

// SyncBatch 同步批次（极瘦头表）
type SyncBatch struct {
	ID          string         `json:"task_id" gorm:"column:id;type:char(36);primaryKey"`
	Description string         `json:"description" gorm:"type:varchar(500)"`
	IsMock      bool           `json:"is_mock" gorm:"default:false"`
	CreatedAt   time.Time      `json:"created_at"`
	UpdatedAt   time.Time      `json:"updated_at"`
	DeletedAt   gorm.DeletedAt `json:"-" gorm:"index"`
}

func (SyncBatch) TableName() string { return "sync_batches" }

// SyncRecord 镜像同步明细
type SyncRecord struct {
	ID               uint           `json:"id" gorm:"primaryKey"`
	BatchID          string         `json:"task_id" gorm:"column:batch_id;type:char(36);not null;index"`
	AcrRegistryID    uint           `json:"acr_registry_id" gorm:"not null;index"`
	OriginalImage    string         `json:"original_image" gorm:"type:varchar(500);not null;index"`
	ACRImage         string         `json:"acr_image" gorm:"type:varchar(500)"`
	Tag              string         `json:"tag" gorm:"type:varchar(100);default:latest"`
	Architecture     string         `json:"architecture" gorm:"type:varchar(50);default:''"`
	AcrArchitectures string         `json:"acr_architectures" gorm:"column:acr_architectures;type:text"`
	OriginalInput    string         `json:"original_input" gorm:"type:varchar(600)"`
	InputOrder       int            `json:"input_order" gorm:"default:0;index"`
	Status           string         `json:"sync_status" gorm:"column:status;type:enum('pending','syncing','success','failed');default:'pending';index"`
	ErrorMessage     string         `json:"error_message" gorm:"type:text"`
	Description      string         `json:"description" gorm:"type:varchar(500)"`
	RetryCount       int            `json:"retry_count" gorm:"default:0"`
	StartedAt        *time.Time     `json:"started_at"`
	CompletedAt      *time.Time     `json:"completed_at"`
	Duration         int64          `json:"duration" gorm:"default:0"`
	ImageSize        int64          `json:"image_size" gorm:"default:0"`
	GitHubRunID      string         `json:"github_run_id" gorm:"-"`
	GitHubRunURL     string         `json:"github_run_url" gorm:"-"`
	CreatedAt        time.Time      `json:"created_at" gorm:"index"`
	UpdatedAt        time.Time      `json:"updated_at"`
	DeletedAt        gorm.DeletedAt `json:"-" gorm:"index"`
	AcrRegistry      *AcrRegistry   `json:"acr_registry,omitempty" gorm:"foreignKey:AcrRegistryID"`
}

func (SyncRecord) TableName() string { return "sync_records" }

// SyncWorkflowRun 批次 × ACR 的 GitHub Actions 运行记录
type SyncWorkflowRun struct {
	ID              uint      `json:"id" gorm:"primaryKey"`
	BatchID         string    `json:"batch_id" gorm:"column:batch_id;type:char(36);not null;index"`
	AcrRegistryID   uint      `json:"acr_registry_id" gorm:"not null;index"`
	GitHubRunID     string    `json:"github_run_id" gorm:"column:github_run_id;type:varchar(100)"`
	GitHubActionURL string    `json:"github_action_url" gorm:"column:github_action_url;type:varchar(500)"`
	CommitSHA       string    `json:"commit_sha" gorm:"column:commit_sha;type:varchar(100)"`
	CreatedAt       time.Time `json:"created_at"`
}

func (SyncWorkflowRun) TableName() string { return "sync_workflow_runs" }

// WorkflowRunResponse API 返回的 workflow run 摘要
type WorkflowRunResponse struct {
	AcrRegistryID   uint   `json:"acr_registry_id"`
	GitHubRunID     string `json:"github_run_id"`
	GitHubActionURL string `json:"github_action_url"`
	CommitSHA       string `json:"commit_sha"`
}

// BatchStatusAggregate 从 sync_records 聚合的批次状态
type BatchStatusAggregate struct {
	Status          string
	Progress        float64
	TotalImages     int
	CompletedImages int
	FailedImages    int
	PendingCount    int
	SyncingCount    int
	SuccessCount    int
	FailedCount     int
}

// ComputeBatchStatus 根据记录状态聚合批次 status
func ComputeBatchStatus(records []SyncRecord) BatchStatusAggregate {
	agg := BatchStatusAggregate{TotalImages: len(records)}
	if len(records) == 0 {
		agg.Status = TaskStatusPending
		return agg
	}
	for _, r := range records {
		switch r.Status {
		case SyncStatusPending:
			agg.PendingCount++
		case SyncStatusSyncing:
			agg.SyncingCount++
		case SyncStatusSuccess:
			agg.SuccessCount++
			agg.CompletedImages++
		case SyncStatusFailed:
			agg.FailedCount++
			agg.FailedImages++
		}
	}
	terminal := agg.PendingCount + agg.SyncingCount == 0
	switch {
	case agg.SyncingCount > 0:
		agg.Status = TaskStatusRunning
	case !terminal:
		agg.Status = TaskStatusPending
	case agg.FailedCount == 0:
		agg.Status = TaskStatusCompleted
	case agg.SuccessCount == 0:
		agg.Status = TaskStatusFailed
	default:
		agg.Status = TaskStatusPartialSuccess
	}
	if agg.TotalImages > 0 {
		agg.Progress = float64(agg.CompletedImages+agg.FailedCount) / float64(agg.TotalImages) * 100
	}
	return agg
}
