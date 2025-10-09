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
	Architecture  string         `json:"architecture" gorm:"type:varchar(50)"`
	SyncStatus    string         `json:"sync_status" gorm:"type:enum('pending','syncing','success','failed');default:'pending';index"`
	ErrorMessage  string         `json:"error_message" gorm:"type:text"`
	TaskID        string         `json:"task_id" gorm:"type:varchar(100);index"`
	CreatedAt     time.Time      `json:"created_at" gorm:"index"`
	UpdatedAt     time.Time      `json:"updated_at"`
	DeletedAt     gorm.DeletedAt `json:"-" gorm:"index"`
}

// SyncTask 同步任务
type SyncTask struct {
	ID              uint           `json:"id" gorm:"primaryKey"`
	TaskID          string         `json:"task_id" gorm:"type:varchar(100);uniqueIndex;not null"`
	ImagesJSON      string         `json:"images_json" gorm:"type:text"`
	Status          string         `json:"status" gorm:"type:enum('pending','running','completed','failed');default:'pending';index"`
	GitHubActionURL string         `json:"github_action_url" gorm:"type:varchar(500)"`
	GitHubRunID     string         `json:"github_run_id" gorm:"type:varchar(100)"`
	CommitSHA       string         `json:"commit_sha" gorm:"type:varchar(100)"`
	StartedAt       *time.Time     `json:"started_at"`
	CompletedAt     *time.Time     `json:"completed_at"`
	ErrorMessage    string         `json:"error_message" gorm:"type:text"`
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
)

// TaskStatus 任务状态常量
const (
	TaskStatusPending   = "pending"
	TaskStatusRunning   = "running"
	TaskStatusCompleted = "completed"
	TaskStatusFailed    = "failed"
)

// ImageRequest 镜像同步请求
type ImageRequest struct {
	Images []string `json:"images" binding:"required"`
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
	GitHubActionURL string     `json:"github_action_url"`
	StartedAt       *time.Time `json:"started_at"`
	CompletedAt     *time.Time `json:"completed_at"`
	ErrorMessage    string     `json:"error_message"`
	Images          []string   `json:"images"`
}