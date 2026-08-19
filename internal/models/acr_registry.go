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
	// Alias 别名：平台内展示与选择的唯一标识（ACR/SWR 的 namespace 可能同名，
	// 需要用别名区分）。留空时默认取 namespace，同名冲突时需手动指定
	Alias         string         `json:"alias" gorm:"type:varchar(100)"`
	Username      string         `json:"username" gorm:"type:varchar(100);not null"`
	Password      string         `json:"-" gorm:"type:varchar(500);not null"`
	AuthServer    string         `json:"auth_server" gorm:"type:varchar(255)"`
	DockerService string         `json:"docker_service" gorm:"type:varchar(255)"`
	RegistryType  string         `json:"registry_type" gorm:"type:varchar(20);default:acr"`
	// SWR 专用：管理面（获取镜像列表等）所需的 IAM AK/SK，与登录凭证（Username/Password）相互独立。
	// AK 如用户名可回显，SK 如密码加密存储不回显
	AccessKey string       `json:"access_key" gorm:"type:varchar(200)"`
	SecretKey string       `json:"-" gorm:"type:varchar(500)"`
	IsDefault bool         `json:"is_default" gorm:"default:false"`
	CreatedAt time.Time    `json:"created_at"`
	UpdatedAt time.Time    `json:"updated_at"`
	DeletedAt gorm.DeletedAt `json:"-" gorm:"index"`
}

// AcrRegistryRequest ACR配置请求
type AcrRegistryRequest struct {
	RegistryURL   string `json:"registry_url" binding:"required"`
	Namespace     string `json:"namespace" binding:"required"`
	Alias         string `json:"alias"`
	Username      string `json:"username" binding:"required"`
	Password      string `json:"password" binding:"required"`
	AuthServer    string `json:"auth_server"`
	DockerService string `json:"docker_service"`
	RegistryType  string `json:"registry_type"`
	AccessKey     string `json:"access_key"`
	SecretKey     string `json:"secret_key"`
}

// AcrRegistryUpdateRequest ACR配置更新请求
type AcrRegistryUpdateRequest struct {
	RegistryURL   string `json:"registry_url"`
	Namespace     string `json:"namespace"`
	Alias         string `json:"alias"`
	Username      string `json:"username"`
	Password      string `json:"password"`
	AuthServer    string `json:"auth_server"`
	DockerService string `json:"docker_service"`
	RegistryType  string `json:"registry_type"`
	AccessKey     string `json:"access_key"`
	SecretKey     string `json:"secret_key"`
}
