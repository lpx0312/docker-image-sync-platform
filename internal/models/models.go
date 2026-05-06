// Package models 定义Docker镜像同步平台的数据模型
//
// models.go 文件包含了系统的核心数据结构定义，主要包括：
// - 数据库实体模型（GORM模型）
// - API请求和响应结构体
// - 业务常量和枚举定义
// - 数据传输对象（DTO）
//
// 核心模型：
// - ImageSyncRecord: 镜像同步记录，记录每个镜像的同步状态和详情
// - SyncTask: 同步任务，管理批量镜像同步的整体状态
// - SystemConfig: 系统配置，存储平台的配置参数
//
// 请求响应模型：
// - 支持单个和批量镜像同步请求
// - 提供详细的状态查询响应
// - 包含完整的错误信息和进度跟踪
//
// 业务特性：
// - 支持多架构镜像同步（amd64, arm64等）
// - 提供优先级和重试机制
// - 支持并发控制和进度监控
// - 集成GitHub Actions工作流跟踪
//
// 作者: Docker镜像同步平台开发团队
// 版本: v1.0.0
package models

import (
	"time"

	"gorm.io/gorm"
)

// ImageSyncRecord 镜像同步记录数据模型
//
// 表示单个Docker镜像的同步记录，包含完整的同步生命周期信息。
// 该模型是系统的核心实体，记录了从源镜像到目标ACR的完整同步过程。
//
// 核心字段：
//   - OriginalImage: 源镜像地址，支持各种Docker Registry
//   - ACRImage: 目标ACR镜像地址，自动生成
//   - Tag: 镜像标签，默认为latest
//   - Architecture: 目标架构，支持amd64、arm64等
//   - SyncStatus: 同步状态，包含完整的状态流转
//
// 扩展功能：
//   - 支持原始输入格式保存和顺序记录
//   - 提供优先级和重试机制
//   - 记录详细的性能指标（耗时、大小）
//   - 集成任务管理和错误追踪
//
// 状态流转：
//
//	pending -> syncing -> success/failed
//	failed -> retrying -> success/failed
//	任何状态 -> skipped（手动跳过）
//
// 使用场景：
//   - 镜像同步状态的实时跟踪
//   - 同步历史记录的查询和统计
//   - 失败重试和错误分析
//   - 性能监控和容量规划
type ImageSyncRecord struct {
	ID            uint           `json:"id" gorm:"primaryKey"`                                                                                              // 主键ID
	OriginalImage string         `json:"original_image" gorm:"type:varchar(500);not null;index"`                                                            // 源镜像地址，建立索引以提高查询性能
	ACRImage      string         `json:"acr_image" gorm:"type:varchar(500)"`                                                                                // 目标ACR镜像地址
	Tag           string         `json:"tag" gorm:"type:varchar(100)"`                                                                                      // 镜像标签
	Architecture       string `json:"architecture" gorm:"type:varchar(50);default:''"`                     // 用户侧架构意图；空表示未指定，由 Actions/ACR 多架构决定
	AcrArchitectures   string `json:"acr_architectures" gorm:"column:acr_architectures;type:text"`           // ACR 中实际存在的架构列表 JSON，如 ["amd64","arm64"]
	OriginalInput string         `json:"original_input" gorm:"type:varchar(600)"`                                                                           // 保存用户原始输入格式，用于追溯
	InputOrder    int            `json:"input_order" gorm:"default:0;index"`                                                                                // 原始输入顺序，用于批量处理时保持顺序
	SyncStatus    string         `json:"sync_status" gorm:"type:enum('pending','syncing','success','failed','retrying','skipped');default:'pending';index"` // 同步状态，建立索引以提高状态查询性能
	ErrorMessage  string         `json:"error_message" gorm:"type:text"`                                                                                    // 错误信息，支持长文本存储
	Description   string         `json:"description" gorm:"type:varchar(500)"`                                                                              // 同步说明，描述同步目的和用途
	TaskID        string         `json:"task_id" gorm:"type:varchar(100);index"`                                                                            // 关联的任务ID，建立索引以提高任务查询性能
	Priority      int            `json:"priority" gorm:"default:0"`                                                                                         // 优先级，数字越大优先级越高
	RetryCount    int            `json:"retry_count" gorm:"default:0"`                                                                                      // 当前重试次数
	MaxRetries    int            `json:"max_retries" gorm:"default:3"`                                                                                      // 最大重试次数
	StartedAt     *time.Time     `json:"started_at"`                                                                                                        // 开始同步时间
	CompletedAt   *time.Time     `json:"completed_at"`                                                                                                      // 完成同步时间
	Duration      int64          `json:"duration" gorm:"default:0"`                                                                                         // 同步耗时（秒），用于性能分析
	ImageSize     int64          `json:"image_size" gorm:"default:0"`                                                                                       // 镜像大小（字节），用于容量统计
	CreatedAt     time.Time      `json:"created_at" gorm:"index"`                                                                                           // 创建时间，建立索引以提高时间范围查询性能
	UpdatedAt     time.Time      `json:"updated_at"`                                                                                                        // 更新时间
	DeletedAt     gorm.DeletedAt `json:"-" gorm:"index"`                                                                                                    // 软删除时间，建立索引以提高软删除查询性能
}

// SyncTask 同步任务数据模型
//
// 表示一个完整的镜像同步任务，可以包含单个或多个镜像的同步。
// 该模型管理整个同步流程的生命周期，提供任务级别的状态跟踪和控制。
//
// 核心功能：
//   - 任务状态管理和生命周期控制
//   - 批量镜像同步的协调和监控
//   - GitHub Actions工作流的集成跟踪
//   - 并发控制和进度监控
//
// 批量同步特性：
//   - 支持自定义并发数量控制
//   - 提供自动重试机制
//   - 实时进度计算和状态更新
//   - 详细的统计信息和错误追踪
//
// GitHub集成：
//   - 自动关联GitHub Actions工作流
//   - 记录提交SHA和运行ID
//   - 提供工作流页面的直接链接
//
// 使用场景：
//   - 批量镜像同步任务的管理
//   - CI/CD流水线的状态跟踪
//   - 同步进度的实时监控
//   - 任务历史记录和统计分析
type SyncTask struct {
	ID              uint       `json:"id" gorm:"primaryKey"`                                                                               // 主键ID
	TaskID          string     `json:"task_id" gorm:"type:varchar(100);uniqueIndex;not null"`                                              // 任务唯一标识符，建立唯一索引
	ImagesJSON      string     `json:"images_json" gorm:"type:text"`                                                                       // 镜像列表的JSON存储，支持复杂数据结构
	Status          string     `json:"status" gorm:"type:enum('pending','running','completed','failed','paused','partial_success');default:'pending';index"` // 任务状态，建立索引以提高状态查询性能
	GitHubActionURL string     `json:"github_action_url" gorm:"column:github_action_url;type:varchar(500)"`                                  // GitHub Actions工作流页面链接
	GitHubRunID     string     `json:"github_run_id" gorm:"column:github_run_id;type:varchar(100)"`                                        // GitHub Actions运行ID
	CommitSHA       string     `json:"commit_sha" gorm:"column:commit_sha;type:varchar(100)"`                                              // 关联的Git提交SHA
	StartedAt       *time.Time `json:"started_at"`                                                                                         // 任务开始时间
	CompletedAt     *time.Time `json:"completed_at"`                                                                                       // 任务完成时间
	ErrorMessage    string     `json:"error_message" gorm:"type:text"`                                                                     // 任务级别的错误信息
	// 批量同步相关字段
	Description     string         `json:"description" gorm:"type:varchar(500)"`           // 任务描述，便于用户识别和管理
	MaxConcurrent   int            `json:"max_concurrent" gorm:"default:3"`                // 最大并发数，控制同时进行的镜像同步数量
	TotalImages     int            `json:"total_images" gorm:"default:0"`                  // 总镜像数量
	CompletedImages int            `json:"completed_images" gorm:"default:0"`              // 已完成镜像数量
	FailedImages    int            `json:"failed_images" gorm:"default:0"`                 // 失败镜像数量
	AutoRetry       bool           `json:"auto_retry" gorm:"default:false"`                // 是否启用自动重试
	RetryCount      int            `json:"retry_count" gorm:"default:0"`                   // 允许的重试次数
	CurrentRetry    int            `json:"current_retry" gorm:"default:0"`                 // 当前重试次数
	Progress        float64        `json:"progress" gorm:"type:decimal(5,2);default:0.00"` // 任务进度百分比（0.00-100.00）
	CreatedAt       time.Time      `json:"created_at" gorm:"index"`                        // 创建时间，建立索引以提高时间范围查询性能
	UpdatedAt       time.Time      `json:"updated_at"`                                     // 更新时间
	DeletedAt       gorm.DeletedAt `json:"-" gorm:"index"`                                 // 软删除时间，建立索引以提高软删除查询性能
}

// SystemConfig 系统配置数据模型
//
// 存储Docker镜像同步平台的各种配置参数，支持动态配置管理。
// 该模型提供了灵活的键值对存储机制，支持系统运行时的配置调整。
//
// 核心特性：
//   - 键值对存储，支持任意配置项
//   - 配置描述，便于管理和维护
//   - 软删除支持，保留配置历史
//   - 唯一键约束，防止配置冲突
//   - 加密存储支持，保护敏感信息安全
//   - 配置分组管理，便于组织和展示
//   - 显示顺序控制，支持自定义排序
//
// 常用配置项：
//   - ACR注册表配置（地址、命名空间、认证）
//   - Git仓库配置（Gitee、GitHub地址和认证）
//   - 同步策略配置（重试次数、超时时间）
//   - 系统限制配置（并发数、队列大小）
//
// 使用场景：
//   - 系统参数的动态配置
//   - 多环境配置管理
//   - 运行时配置热更新
//   - 配置变更历史追踪
type SystemConfig struct {
	ID           uint           `json:"id" gorm:"primaryKey"`                                     // 主键ID
	ConfigKey    string         `json:"config_key" gorm:"type:varchar(100);uniqueIndex;not null"` // 配置键，建立唯一索引确保键的唯一性
	ConfigValue  string         `json:"config_value" gorm:"type:text"`                            // 配置值，支持长文本存储复杂配置
	Description  string         `json:"description" gorm:"type:varchar(500)"`                     // 配置描述，便于理解配置用途
	IsEncrypted  bool           `json:"is_encrypted" gorm:"default:false"`                        // 是否加密存储，用于标识敏感信息
	ConfigGroup  string         `json:"config_group" gorm:"type:varchar(50);default:'default'"`   // 配置分组，用于组织和分类配置项
	DisplayOrder int            `json:"display_order" gorm:"default:0"`                           // 显示顺序，用于前端展示时的排序
	CreatedAt    time.Time      `json:"created_at"`                                               // 创建时间
	UpdatedAt    time.Time      `json:"updated_at"`                                               // 更新时间
	DeletedAt    gorm.DeletedAt `json:"-" gorm:"index"`                                           // 软删除时间，建立索引以提高软删除查询性能
}

// User 用户数据模型
type User struct {
	ID           uint           `json:"id" gorm:"primaryKey"`
	Username     string         `json:"username" gorm:"type:varchar(50);uniqueIndex;not null"`
	PasswordHash string         `json:"-" gorm:"type:varchar(255);not null"`
	Email        string         `json:"email" gorm:"type:varchar(100)"`
	Role         string         `json:"role" gorm:"type:varchar(20);default:'user'"`
	Status       string         `json:"status" gorm:"type:varchar(20);default:'active'"`
	LastLoginAt  *time.Time     `json:"last_login_at"`
	CreatedAt    time.Time      `json:"created_at"`
	UpdatedAt    time.Time      `json:"updated_at"`
	DeletedAt    gorm.DeletedAt `json:"-" gorm:"index"`
}

// LoginLog 登录日志数据模型
type LoginLog struct {
	ID        uint      `json:"id" gorm:"primaryKey"`
	UserID    uint      `json:"user_id" gorm:"index;not null"`
	Username  string    `json:"username" gorm:"type:varchar(50);not null"`
	IP        string    `json:"ip" gorm:"type:varchar(45)"`
	UserAgent string    `json:"user_agent" gorm:"type:varchar(500)"`
	Status    string    `json:"status" gorm:"type:varchar(20);not null"`
	Message   string    `json:"message" gorm:"type:varchar(255)"`
	CreatedAt time.Time `json:"created_at" gorm:"index"`
}

// TableName 设置数据库表名
func (ImageSyncRecord) TableName() string { return "image_sync_records" }
func (SyncTask) TableName() string        { return "sync_tasks" }
func (SystemConfig) TableName() string    { return "system_configs" }
func (User) TableName() string            { return "users" }
func (LoginLog) TableName() string        { return "login_logs" }

// 用户角色常量
const (
	RoleAdmin    = "admin"
	RoleOperator = "operator"
	RoleUser     = "user"
)

// 权限标识常量
const (
	PermSync   = "sync"
	PermGitHub = "github"
	PermConfig = "config"
	PermUsers  = "users"
)

// RoleInfo 角色信息（用于 API 返回）
type RoleInfo struct {
	Role        string   `json:"role"`
	Label       string   `json:"label"`
	Permissions []string `json:"permissions"`
}

// rolePermissions 角色→权限映射（包内私有，通过函数暴露）
var rolePermissions = map[string][]string{
	RoleAdmin:    {PermSync, PermGitHub, PermConfig, PermUsers},
	RoleOperator: {PermSync, PermGitHub, PermConfig},
	RoleUser:     {PermSync},
}

// GetRolePermissions 返回指定角色拥有的权限列表
func GetRolePermissions(role string) []string {
	if perms, ok := rolePermissions[role]; ok {
		return perms
	}
	return []string{PermSync}
}

// HasPermission 判断指定角色是否拥有某项权限
func HasPermission(role, permission string) bool {
	for _, p := range GetRolePermissions(role) {
		if p == permission {
			return true
		}
	}
	return false
}

// IsValidRole 检查角色标识是否合法
func IsValidRole(role string) bool {
	_, ok := rolePermissions[role]
	return ok
}

// GetAllRoles 返回所有角色的信息列表
func GetAllRoles() []RoleInfo {
	return []RoleInfo{
		{Role: RoleAdmin, Label: "管理员", Permissions: rolePermissions[RoleAdmin]},
		{Role: RoleOperator, Label: "运维员", Permissions: rolePermissions[RoleOperator]},
		{Role: RoleUser, Label: "普通用户", Permissions: rolePermissions[RoleUser]},
	}
}

// 用户状态常量
const (
	UserStatusActive   = "active"
	UserStatusDisabled = "disabled"
)

// 登录状态常量
const (
	LoginStatusSuccess = "success"
	LoginStatusFailed  = "failed"
)

// 同步状态常量定义
//
// 定义了镜像同步过程中的所有可能状态，确保状态管理的一致性。
// 这些常量与数据库枚举类型保持一致，提供类型安全的状态操作。

// SyncStatus 镜像同步状态常量
//
// 状态流转说明：
//   - pending: 初始状态，等待开始同步
//   - syncing: 正在进行同步操作
//   - success: 同步成功完成
//   - failed: 同步失败
//   - retrying: 正在重试同步
//   - skipped: 手动跳过同步
const (
	SyncStatusPending  = "pending"  // 等待同步
	SyncStatusSyncing  = "syncing"  // 正在同步
	SyncStatusSuccess  = "success"  // 同步成功
	SyncStatusFailed   = "failed"   // 同步失败
	SyncStatusRetrying = "retrying" // 正在重试
	SyncStatusSkipped  = "skipped"  // 跳过同步
)

// TaskStatus 任务状态常量
//
// 状态流转说明：
//   - pending: 任务创建，等待开始执行
//   - running: 任务正在执行中
//   - completed: 任务执行完成（可能包含部分失败）
//   - failed: 任务执行失败
//   - paused: 任务暂停执行
const (
	TaskStatusPending        = "pending"         // 等待执行
	TaskStatusRunning        = "running"         // 正在执行
	TaskStatusCompleted      = "completed"       // 执行完成
	TaskStatusSuccess        = "completed"       // 成功状态的别名，保持向后兼容
	TaskStatusFailed         = "failed"          // 执行失败
	TaskStatusPartialSuccess = "partial_success" // 部分成功（部分镜像同步失败）
	TaskStatusPaused         = "paused"          // 暂停执行
)

// API请求响应模型定义
//
// 以下结构体定义了系统API的请求和响应格式，提供了完整的数据传输对象（DTO）。
// 这些模型与前端交互，支持数据验证和序列化。

// ImageRequest 镜像同步请求（向后兼容）
//
// 保持与旧版本API的兼容性，支持简单的镜像同步请求。
// 该结构体主要用于单个或少量镜像的快速同步。
//
// 字段说明：
//   - Images: 镜像列表，支持多个镜像地址
//   - Architecture: 目标架构，可选参数
//
// 使用场景：
//   - 简单的镜像同步需求
//   - 与旧版本客户端的兼容
//   - 快速测试和验证
type ImageRequest struct {
	Images       []string `json:"images" binding:"required"` // 镜像列表，必填字段
	Architecture string   `json:"architecture"`              // 目标架构，可选
}

// SyncRequest 单个同步请求
//
// 用于单个同步任务的请求，支持基本的同步参数配置。
// 相比ImageRequest，增加了任务描述功能。
//
// 字段说明：
//   - Images: 镜像列表，支持多个镜像地址
//   - Architecture: 目标架构，可选参数
//   - Description: 任务描述，便于识别和管理
//
// 使用场景：
//   - 需要任务描述的同步请求
//   - 简单的批量同步
//   - 基础的同步功能
type SyncRequest struct {
	Images       []string `json:"images" binding:"required"` // 镜像列表，必填字段
	Architecture string   `json:"architecture"`              // 目标架构，可选
	Description  string   `json:"description"`               // 任务描述，可选
}

// BatchSyncRequest 批量镜像同步请求
//
// 用于高级批量同步功能，支持详细的同步控制和配置。
// 该结构体提供了完整的批量同步参数，包括并发控制和重试策略。
//
// 字段说明：
//   - Images: 镜像同步项列表，每项包含详细配置
//   - MaxConcurrent: 最大并发数，控制同时进行的同步数量
//   - AutoRetry: 是否启用自动重试
//   - RetryCount: 重试次数限制
//
// 验证规则：
//   - Images: 必填，至少包含一个镜像项
//   - MaxConcurrent: 范围1-10，防止过度并发
//   - RetryCount: 范围0-3，合理的重试限制
//
// 使用场景：
//   - 大规模镜像同步任务
//   - 需要精细控制的同步操作
//   - 生产环境的批量迁移
//   - 自动化CI/CD流水线
type BatchSyncRequest struct {
	Images        []ImageSyncItem `json:"images" binding:"required"` // 镜像同步项列表，必填
	MaxConcurrent int             `json:"max_concurrent"`            // 最大并发数；0 或未传由服务端按配置填充
	AutoRetry     bool            `json:"auto_retry"`                // 是否启用自动重试
	RetryCount    int             `json:"retry_count"`               // 重试次数；0 且启用重试时由服务端按配置填充
}

// ImageSyncItem 单个镜像同步项
//
// 表示批量同步请求中的单个镜像配置，提供了详细的同步参数。
// 该结构体支持每个镜像的独立配置，包括优先级和目标标签。
//
// 字段说明：
//   - SourceImage: 源镜像地址，必填
//   - TargetTag: 目标标签，可选，默认使用源镜像标签
//   - Architecture: 目标架构，可选，默认使用全局配置
//   - Priority: 优先级，数字越大优先级越高
//
// 使用场景：
//   - 需要不同优先级的镜像同步
//   - 自定义目标标签的场景
//   - 混合架构的批量同步
//   - 复杂的同步策略配置
type ImageSyncItem struct {
	SourceImage  string `json:"source_image" binding:"required"` // 源镜像地址，必填
	TargetTag    string `json:"target_tag"`                      // 目标标签，可选
	Architecture string `json:"architecture"`                    // 目标架构，可选
	Priority     int    `json:"priority"`                        // 优先级，数字越大优先级越高
	Description  string `json:"description"`                     // 同步说明，描述同步目的和用途
}

// API响应模型定义
//
// 以下结构体定义了系统API的响应格式，提供了统一的数据返回结构。
// 这些模型确保了API响应的一致性和完整性。

// BatchSyncResponse 批量同步响应
//
// 批量同步请求的响应结果，包含任务基本信息和预估时间。
// 该响应为客户端提供了任务跟踪所需的关键信息。
//
// 字段说明：
//   - TaskID: 任务唯一标识符，用于后续状态查询
//   - Message: 响应消息，描述任务创建结果
//   - ImageCount: 镜像数量，确认处理的镜像总数
//   - EstimatedTime: 预估完成时间，帮助用户了解等待时长
//
// 使用场景：
//   - 批量同步任务的创建确认
//   - 任务跟踪信息的提供
//   - 用户界面的状态显示
type BatchSyncResponse struct {
	TaskID        string `json:"task_id"`        // 任务ID，用于状态查询
	Message       string `json:"message"`        // 响应消息
	ImageCount    int    `json:"image_count"`    // 镜像数量
	EstimatedTime string `json:"estimated_time"` // 预估完成时间
}

// SyncResponse 同步响应
//
// 简单同步请求的响应结果，提供基本的任务信息。
// 该响应用于单个或简单批量同步的确认。
//
// 字段说明：
//   - TaskID: 任务唯一标识符
//   - Message: 响应消息
//
// 使用场景：
//   - 简单同步任务的创建确认
//   - 快速同步操作的响应
//   - 基础API的返回格式
type SyncResponse struct {
	TaskID  string `json:"task_id"` // 任务ID
	Message string `json:"message"` // 响应消息
}

// ImageListResponse 镜像列表响应
//
// 镜像查询API的响应格式，支持分页和统计信息。
// 该响应为镜像管理界面提供了完整的数据结构。
//
// 字段说明：
//   - Total: 符合条件的镜像总数
//   - Data: 镜像记录列表，支持分页
//
// 使用场景：
//   - 镜像列表的分页查询
//   - 镜像管理界面的数据展示
//   - 镜像统计信息的提供
type ImageListResponse struct {
	Total int                `json:"total"` // 镜像总数
	Data  []*ImageSyncRecord `json:"data"`  // 镜像记录列表
}

// TaskStatusResponse 任务状态响应
//
// 单个任务状态查询的响应格式，提供任务的详细状态信息。
// 该响应支持简单任务的状态跟踪和监控。
//
// 字段说明：
//   - TaskID: 任务唯一标识符
//   - Status: 当前任务状态
//   - SourceImage: 源镜像地址
//   - TargetImage: 目标镜像地址
//   - Architecture: 目标架构
//   - GitHubActionURL: GitHub Actions链接
//   - StartedAt: 开始时间
//   - CompletedAt: 完成时间
//   - ErrorMessage: 错误信息
//   - Images: 镜像列表
//
// 使用场景：
//   - 单个任务的状态查询
//   - 简单任务的监控界面
//   - 任务完成状态的确认
type TaskStatusResponse struct {
	TaskID          string     `json:"task_id"`           // 任务ID
	Status          string     `json:"status"`            // 任务状态
	SourceImage     string     `json:"source_image"`      // 源镜像地址
	TargetImage     string     `json:"target_image"`      // 目标镜像地址
	Architecture    string     `json:"architecture"`      // 目标架构
	GitHubActionURL string     `json:"github_action_url"` // GitHub Actions链接
	StartedAt       *time.Time `json:"started_at"`        // 开始时间
	CompletedAt     *time.Time `json:"completed_at"`      // 完成时间
	ErrorMessage    string     `json:"error_message"`     // 错误信息
	Images          []string   `json:"images"`            // 镜像列表
}

// BatchTaskStatusResponse 批量任务状态响应
//
// 批量任务状态查询的响应格式，提供完整的任务监控信息。
// 该响应支持复杂批量任务的详细状态跟踪和进度监控。
//
// 字段说明：
//   - TaskID: 任务唯一标识符
//   - Status: 当前任务状态
//   - Description: 任务描述
//   - TotalImages: 总镜像数量
//   - CompletedImages: 已完成镜像数量
//   - FailedImages: 失败镜像数量
//   - Progress: 任务进度百分比
//   - MaxConcurrent: 最大并发数
//   - AutoRetry: 是否启用自动重试
//   - CurrentRetry: 当前重试次数
//   - RetryCount: 总重试次数
//   - GitHubActionURL: GitHub Actions链接
//   - StartedAt: 开始时间
//   - CompletedAt: 完成时间
//   - ErrorMessage: 错误信息
//   - ImageDetails: 镜像详细状态列表
//   - EstimatedTime: 预估剩余时间
//
// 使用场景：
//   - 批量任务的详细状态查询
//   - 任务进度的实时监控
//   - 复杂任务的管理界面
//   - 任务性能分析和优化
type BatchTaskStatusResponse struct {
	TaskID          string                    `json:"task_id"`           // 任务ID
	Status          string                    `json:"status"`            // 任务状态
	Description     string                    `json:"description"`       // 任务描述
	TotalImages     int                       `json:"total_images"`      // 总镜像数量
	CompletedImages int                       `json:"completed_images"`  // 已完成镜像数量
	FailedImages    int                       `json:"failed_images"`     // 失败镜像数量
	Progress        float64                   `json:"progress"`          // 任务进度百分比
	MaxConcurrent   int                       `json:"max_concurrent"`    // 最大并发数
	AutoRetry       bool                      `json:"auto_retry"`        // 是否启用自动重试
	CurrentRetry    int                       `json:"current_retry"`     // 当前重试次数
	RetryCount      int                       `json:"retry_count"`       // 总重试次数
	GitHubActionURL string                    `json:"github_action_url"` // GitHub Actions链接
	StartedAt       *time.Time                `json:"started_at"`        // 开始时间
	CompletedAt     *time.Time                `json:"completed_at"`      // 完成时间
	ErrorMessage    string                    `json:"error_message"`     // 错误信息
	ImageDetails    []ImageSyncDetailResponse `json:"image_details"`     // 镜像详细状态列表
	EstimatedTime   string                    `json:"estimated_time"`    // 预估剩余时间
}

// ImageSyncDetailResponse 镜像同步详情响应
//
// 单个镜像同步的详细状态信息，用于批量任务的细粒度监控。
// 该响应提供了每个镜像的完整同步状态和性能指标。
//
// 字段说明：
//   - ID: 镜像记录ID
//   - OriginalImage: 源镜像地址
//   - ACRImage: 目标ACR镜像地址
//   - Tag: 镜像标签
//   - Architecture: 目标架构
//   - SyncStatus: 同步状态
//   - ErrorMessage: 错误信息
//   - Priority: 优先级
//   - RetryCount: 当前重试次数
//   - MaxRetries: 最大重试次数
//   - StartedAt: 开始时间
//   - CompletedAt: 完成时间
//   - Duration: 同步耗时
//   - ImageSize: 镜像大小
//
// 使用场景：
//   - 批量任务中单个镜像的状态查询
//   - 镜像同步的详细监控
//   - 性能分析和故障排查
//   - 同步历史记录的详细展示
type ImageSyncDetailResponse struct {
	ID            uint       `json:"id"`             // 镜像记录ID
	OriginalImage string     `json:"original_image"` // 源镜像地址
	ACRImage      string     `json:"acr_image"`      // 目标ACR镜像地址
	Tag           string     `json:"tag"`            // 镜像标签
	Architecture  string     `json:"architecture"`   // 目标架构
	SyncStatus    string     `json:"sync_status"`    // 同步状态
	ErrorMessage  string     `json:"error_message"`  // 错误信息
	Priority      int        `json:"priority"`       // 优先级
	RetryCount    int        `json:"retry_count"`    // 当前重试次数
	MaxRetries    int        `json:"max_retries"`    // 最大重试次数
	StartedAt     *time.Time `json:"started_at"`     // 开始时间
	CompletedAt   *time.Time `json:"completed_at"`   // 完成时间
	Duration      int64      `json:"duration"`       // 同步耗时（秒）
	ImageSize     int64      `json:"image_size"`     // 镜像大小（字节）
}
