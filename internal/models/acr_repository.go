package models

import (
	"time"

	"gorm.io/gorm"
)

// AcrRepository ACR镜像仓库数据模型
type AcrRepository struct {
	ID             uint           `json:"id" gorm:"primaryKey"`
	AcrRegistryID  uint           `json:"acr_registry_id" gorm:"not null;index"`
	RepositoryName string         `json:"repository_name" gorm:"type:varchar(255);not null"`
	CreatedAt      time.Time      `json:"created_at"`
	UpdatedAt      time.Time      `json:"updated_at"`
	DeletedAt      gorm.DeletedAt `json:"-" gorm:"index"`
	// 关联
	AcrRegistry    *AcrRegistry   `json:"acr_registry,omitempty" gorm:"foreignKey:AcrRegistryID"`
}

// AcrRepositoryRequest 添加镜像请求
type AcrRepositoryRequest struct {
	AcrRegistryID  uint   `json:"acr_registry_id" binding:"required"`
	RepositoryName string `json:"repository_name" binding:"required"`
}

// AcrRepositoryBatchRequest 批量添加镜像请求
type AcrRepositoryBatchRequest struct {
	AcrRegistryID   uint     `json:"acr_registry_id" binding:"required"`
	RepositoryNames []string `json:"repository_names" binding:"required,min=1"`
}

// AcrRepositorySyncRequest 从同步记录导入请求
type AcrRepositorySyncRequest struct {
	AcrRegistryID uint `json:"acr_registry_id" binding:"required"`
}
