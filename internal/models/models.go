package models

import (
	"time"

	"gorm.io/gorm"
)

// ImageSyncRecord 镜像同步记录
type ImageSyncRecord struct {
	ID            uint           `json:"id" gorm:"primaryKey"`
	OriginalImage string         `json:"original_image" gorm:"type:varchar(500);not null;index"`
	ACRImage      string         `json:"acr_image" gorm:"type:varchar(500)"`
	Tag           string         `json:"tag" gorm:"type:varchar(100)"`
	Architecture  string         `json:"architecture" gorm:"type:varchar(50);default:'amd64'"`
	OriginalInput string         `json:"original_input" gorm:"type:varchar(600)"` // 保存原始输入格式
	InputOrder    int            `json:"input_order" gorm:"default:0;index"`      // 保存原始输入顺序
	SyncStatus    string         `json:"sync_status" gorm:"type:enum('pending','syncing','success','failed','retrying','skipped');default:'pending';index"`
	ErrorMessage  string         `json:"error_message" gorm:"type:text"`
	TaskID        string         `json:"task_id" gorm:"type:varchar(100);index"`
	Priority      int            `json:"priority" gorm:"default:0"`
	RetryCount    int            `json:"retry_count" gorm:"default:0"`
	MaxRetries    int            `json:"max_retries" gorm:"default:3"`
	StartedAt     *time.Time     `json:"started_at"`
	CompletedAt   *time.Time     `json:"completed_at"`
	Duration      int64          `json:"duration" gorm:"default:0"` // 同步耗时（秒）
	ImageSize     int64          `json:"image_size" gorm:"default:0"` // 镜像大小（字节）
	CreatedAt     time.Time      `json:"created_at" gorm:"index"`
	UpdatedAt     time.Time      `json:"updated_at"`
	DeletedAt     gorm.DeletedAt `json:"-" gorm:"index"`
}

// SyncTask 同步任务
type SyncTask struct {
	ID              uint           `json:"id" gorm:"primaryKey"`
	TaskID          string         `json:"task_id" gorm:"type:varchar(100);uniqueIndex;not null"`
	ImagesJSON      string         `json:"images_json" gorm:"type:text"`
	Status          string         `json:"status" gorm:"type:enum('pending','running','completed','failed','paused');default:'pending';index"`
	GitHubActionURL string         `json:"github_action_url" gorm:"type:varchar(500)"`
	GitHubRunID     string         `json:"github_run_id" gorm:"type:varchar(100)"`
	CommitSHA       string         `json:"commit_sha" gorm:"type:varchar(100)"`
	StartedAt       *time.Time     `json:"started_at"`
	CompletedAt     *time.Time     `json:"completed_at"`
	ErrorMessage    string         `json:"error_message" gorm:"type:text"`
	// 批量同步相关字段
	Description     string         `json:"description" gorm:"type:varchar(500)"`
	MaxConcurrent   int            `json:"max_concurrent" gorm:"default:3"`
	TotalImages     int            `json:"total_images" gorm:"default:0"`
	CompletedImages int            `json:"completed_images" gorm:"default:0"`
	FailedImages    int            `json:"failed_images" gorm:"default:0"`
	AutoRetry       bool           `json:"auto_retry" gorm:"default:false"`
	RetryCount      int            `json:"retry_count" gorm:"default:0"`
	CurrentRetry    int            `json:"current_retry" gorm:"default:0"`
	Progress        float64        `json:"progress" gorm:"type:decimal(5,2);default:0.00"`
	CreatedAt       time.Time      `json:"created_at" gorm:"index"`
	UpdatedAt       time.Time      `json:"updated_at"`
	DeletedAt       gorm.DeletedAt `json:"-" gorm:"index"`
}

// SystemConfig 系统配置
type SystemConfig struct {
	ID          uint           `json:"id" gorm:"primaryKey"`
	ConfigKey   string         `json:"config_key" gorm:"type:varchar(100);uniqueIndex;not null"`
	ConfigValue string         `json:"config_value" gorm:"type:text"`
	Description string         `json:"description" gorm:"type:varchar(500)"`
	CreatedAt   time.Time      `json:"created_at"`
	UpdatedAt   time.Time      `json:"updated_at"`
	DeletedAt   gorm.DeletedAt `json:"-" gorm:"index"`
}

// TableName 设置表名
func (ImageSyncRecord) TableName() string {
	return "image_sync_records"
}

func (SyncTask) TableName() string {
	return "sync_tasks"
}

func (SystemConfig) TableName() string {
	return "system_configs"
}

// SyncStatus 同步状态常量
const (
	SyncStatusPending  = "pending"
	SyncStatusSyncing  = "syncing"
	SyncStatusSuccess  = "success"
	SyncStatusFailed   = "failed"
	SyncStatusRetrying = "retrying"
	SyncStatusSkipped  = "skipped"
)

// TaskStatus 任务状态常量
const (
	TaskStatusPending   = "pending"
	TaskStatusRunning   = "running"
	TaskStatusCompleted = "completed"
	TaskStatusFailed    = "failed"
	TaskStatusPaused    = "paused"
)

// ImageRequest 镜像同步请求（保持向后兼容）
type ImageRequest struct {
	Images       []string `json:"images" binding:"required"`
	Architecture string   `json:"architecture"`
}

// BatchSyncRequest 批量镜像同步请求
type BatchSyncRequest struct {
	Images        []ImageSyncItem `json:"images" binding:"required"`
	MaxConcurrent int             `json:"max_concurrent" binding:"min=1,max=10"`
	AutoRetry     bool            `json:"auto_retry"`
	RetryCount    int             `json:"retry_count" binding:"min=0,max=3"`
}

// ImageSyncItem 单个镜像同步项
type ImageSyncItem struct {
	SourceImage  string `json:"source_image" binding:"required"`
	TargetTag    string `json:"target_tag"`
	Architecture string `json:"architecture"`
	Priority     int    `json:"priority"` // 优先级，数字越大优先级越高
}

// BatchSyncResponse 批量同步响应
type BatchSyncResponse struct {
	TaskID      string `json:"task_id"`
	Message     string `json:"message"`
	ImageCount  int    `json:"image_count"`
	EstimatedTime string `json:"estimated_time"`
}

// SyncResponse 同步响应
type SyncResponse struct {
	TaskID  string `json:"task_id"`
	Message string `json:"message"`
}

// ImageListResponse 镜像列表响应
type ImageListResponse struct {
	Total int                `json:"total"`
	Data  []*ImageSyncRecord `json:"data"`
}

// TaskStatusResponse 任务状态响应
type TaskStatusResponse struct {
	TaskID          string     `json:"task_id"`
	Status          string     `json:"status"`
	SourceImage     string     `json:"source_image"`
	TargetImage     string     `json:"target_image"`
	Architecture    string     `json:"architecture"`
	GitHubActionURL string     `json:"github_action_url"`
	StartedAt       *time.Time `json:"started_at"`
	CompletedAt     *time.Time `json:"completed_at"`
	ErrorMessage    string     `json:"error_message"`
	Images          []string   `json:"images"`
}

// BatchTaskStatusResponse 批量任务状态响应
type BatchTaskStatusResponse struct {
	TaskID          string                    `json:"task_id"`
	Status          string                    `json:"status"`
	Description     string                    `json:"description"`
	TotalImages     int                       `json:"total_images"`
	CompletedImages int                       `json:"completed_images"`
	FailedImages    int                       `json:"failed_images"`
	Progress        float64                   `json:"progress"`
	MaxConcurrent   int                       `json:"max_concurrent"`
	AutoRetry       bool                      `json:"auto_retry"`
	CurrentRetry    int                       `json:"current_retry"`
	RetryCount      int                       `json:"retry_count"`
	GitHubActionURL string                    `json:"github_action_url"`
	StartedAt       *time.Time                `json:"started_at"`
	CompletedAt     *time.Time                `json:"completed_at"`
	ErrorMessage    string                    `json:"error_message"`
	ImageDetails    []ImageSyncDetailResponse `json:"image_details"`
	EstimatedTime   string                    `json:"estimated_time"`
}

// ImageSyncDetailResponse 镜像同步详情响应
type ImageSyncDetailResponse struct {
	ID            uint       `json:"id"`
	OriginalImage string     `json:"original_image"`
	ACRImage      string     `json:"acr_image"`
	Tag           string     `json:"tag"`
	Architecture  string     `json:"architecture"`
	SyncStatus    string     `json:"sync_status"`
	ErrorMessage  string     `json:"error_message"`
	Priority      int        `json:"priority"`
	RetryCount    int        `json:"retry_count"`
	MaxRetries    int        `json:"max_retries"`
	StartedAt     *time.Time `json:"started_at"`
	CompletedAt   *time.Time `json:"completed_at"`
	Duration      int64      `json:"duration"`
	ImageSize     int64      `json:"image_size"`
}