package models

import (
	"time"

	"gorm.io/gorm"
)

// 镜像仓库类型
const (
	RegistryTypeACR = "acr" // 阿里云 ACR
	RegistryTypeSWR = "swr" // 华为云 SWR
)

// IsValidRegistryType 校验仓库类型取值
func IsValidRegistryType(t string) bool {
	return t == RegistryTypeACR || t == RegistryTypeSWR
}

// AcrRegistry ACR镜像仓库配置数据模型
type AcrRegistry struct {
	ID            uint           `json:"id" gorm:"primaryKey"`
	RegistryURL   string         `json:"registry_url" gorm:"type:varchar(255);not null"`
	Namespace     string         `json:"namespace" gorm:"type:varchar(100);not null"`
	Username      string         `json:"username" gorm:"type:varchar(100);not null"`
	Password      string         `json:"-" gorm:"type:varchar(500);not null"`
	AuthServer    string         `json:"auth_server" gorm:"type:varchar(255)"`
	DockerService string         `json:"docker_service" gorm:"type:varchar(255)"`
	RegistryType  string         `json:"registry_type" gorm:"type:varchar(20);default:acr"`
	IsDefault     bool           `json:"is_default" gorm:"default:false"`
	CreatedAt     time.Time      `json:"created_at"`
	UpdatedAt     time.Time      `json:"updated_at"`
	DeletedAt     gorm.DeletedAt `json:"-" gorm:"index"`
}

// AcrRegistryRequest ACR配置请求
type AcrRegistryRequest struct {
	RegistryURL   string `json:"registry_url" binding:"required"`
	Namespace     string `json:"namespace" binding:"required"`
	Username      string `json:"username" binding:"required"`
	Password      string `json:"password" binding:"required"`
	AuthServer    string `json:"auth_server"`
	DockerService string `json:"docker_service"`
	RegistryType  string `json:"registry_type"`
}

// AcrRegistryUpdateRequest ACR配置更新请求
type AcrRegistryUpdateRequest struct {
	RegistryURL   string `json:"registry_url"`
	Namespace     string `json:"namespace"`
	Username      string `json:"username"`
	Password      string `json:"password"`
	AuthServer    string `json:"auth_server"`
	DockerService string `json:"docker_service"`
	RegistryType  string `json:"registry_type"`
}
