// Package models 定义Docker镜像同步平台的数据模型
//
// models.go 文件包含了系统的核心数据结构定义，主要包括：
// - 数据库实体模型（GORM模型）
// - API请求和响应结构体
// - 业务常量和枚举定义
// - 数据传输对象（DTO）
//
// 核心模型：
// - SyncRecord: 镜像同步明细
// - SyncBatch: 同步批次头表
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
	RoleID       uint           `json:"role_id" gorm:"not null;index"`
	Role         *Role          `json:"role,omitempty" gorm:"foreignKey:RoleID"`
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
func (SystemConfig) TableName() string { return "system_configs" }
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
	PermImages = "images"
	PermGitHub = "github"
	PermConfig = "config"
	PermUsers  = "users"
	PermRoles  = "roles"
)

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
)

// API请求响应模型定义
//
// 以下结构体定义了系统API的请求和响应格式，提供了完整的数据传输对象（DTO）。
// 这些模型与前端交互，支持数据验证和序列化。

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
	Images        []string `json:"images" binding:"required"` // 镜像列表，必填字段
	Architecture  string   `json:"architecture"`              // 目标架构，可选
	Description   string   `json:"description"`               // 任务描述，可选
	AcrRegistryID uint     `json:"acr_registry_id"`           // ACR配置ID，0表示使用默认配置
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
	AcrRegistryID uint            `json:"acr_registry_id"` // ACR配置ID，0表示亲和性解析
}

// CheckAcrRequest 批量检查镜像与 ACR 归属冲突
type CheckAcrRequest struct {
	Images        []string `json:"images" binding:"required,min=1"`
	AcrRegistryID uint     `json:"acr_registry_id"`
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
	Architecture string `json:"architecture"` // 目标架构，可选
	Description  string `json:"description"`  // 同步说明
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
	Total int           `json:"total"`
	Data  []*SyncRecord `json:"data"`
}
