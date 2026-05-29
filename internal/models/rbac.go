package models

import "time"

// Permission 权限定义（注册表，启动时 seed）
type Permission struct {
	ID          uint      `json:"id" gorm:"primaryKey"`
	Code        string    `json:"code" gorm:"type:varchar(50);uniqueIndex;not null"`
	Name        string    `json:"name" gorm:"type:varchar(100);not null"`
	Description string    `json:"description" gorm:"type:varchar(255)"`
	SortOrder   int       `json:"sort_order" gorm:"default:0"`
	CreatedAt   time.Time `json:"created_at"`
	UpdatedAt   time.Time `json:"updated_at"`
}

// Role 角色定义
type Role struct {
	ID          uint         `json:"id" gorm:"primaryKey"`
	Code        string       `json:"code" gorm:"type:varchar(50);uniqueIndex;not null"`
	Name        string       `json:"name" gorm:"type:varchar(100);not null"`
	Description string       `json:"description" gorm:"type:varchar(255)"`
	IsSystem    bool         `json:"is_system" gorm:"default:false"`
	Permissions []Permission `json:"permissions,omitempty" gorm:"many2many:role_permissions;"`
	CreatedAt   time.Time    `json:"created_at"`
	UpdatedAt   time.Time    `json:"updated_at"`
}

func (Permission) TableName() string { return "permissions" }
func (Role) TableName() string       { return "roles" }

// DefaultPermissionSeeds 默认权限 seed 数据
var DefaultPermissionSeeds = []Permission{
	{Code: PermSync, Name: "镜像同步", Description: "提交同步任务与查看同步历史", SortOrder: 1},
	{Code: PermImages, Name: "镜像管理", Description: "镜像记录查询、检测与重试", SortOrder: 2},
	{Code: PermGitHub, Name: "GitHub Actions", Description: "GitHub 工作流监控", SortOrder: 3},
	{Code: PermConfig, Name: "系统配置", Description: "系统与 ACR 配置管理", SortOrder: 4},
	{Code: PermUsers, Name: "用户管理", Description: "用户账号与登录日志管理", SortOrder: 5},
	{Code: PermRoles, Name: "角色管理", Description: "角色与权限配置", SortOrder: 6},
}

// DefaultRoleSeeds 默认角色 seed 数据（code -> name, isSystem, permission codes）
var DefaultRoleSeeds = []struct {
	Code        string
	Name        string
	Description string
	IsSystem    bool
	Permissions []string
}{
	{Code: RoleAdmin, Name: "管理员", Description: "拥有全部权限", IsSystem: true, Permissions: []string{PermSync, PermImages, PermGitHub, PermConfig, PermUsers, PermRoles}},
	{Code: RoleOperator, Name: "运维员", Description: "镜像同步与系统运维", IsSystem: true, Permissions: []string{PermSync, PermImages, PermGitHub, PermConfig}},
	{Code: RoleUser, Name: "普通用户", Description: "仅镜像同步", IsSystem: true, Permissions: []string{PermSync}},
}
