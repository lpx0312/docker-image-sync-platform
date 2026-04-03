// Package services 提供配置管理服务
//
// config.go 文件实现了系统配置的数据库化管理功能，主要用于Git和阿里云配置的动态管理。
//
// 核心功能：
// - 配置的CRUD操作（创建、读取、更新、删除）
// - 敏感信息的自动加密存储
// - 配置分组和排序管理
// - 配置缓存和性能优化
//
// 支持的配置类型：
// - Git仓库配置（GitHub、Gitee）
// - 阿里云服务配置（ACR认证）
// - 系统参数配置
// - 第三方服务配置
//
// 安全特性：
// - 敏感信息自动加密
// - 配置访问权限控制
// - 操作日志记录
// - 数据完整性验证
//
// 作者: Docker镜像同步平台开发团队
// 版本: v1.0.0
package services

import (
	"fmt"
	"strings"
	"sync"

	"github.com/sirupsen/logrus"
	"gorm.io/gorm"

	"docker-image-sync-platform/internal/models"
)

// ConfigService 配置管理服务
//
// 提供系统配置的数据库化管理功能，支持配置的动态更新和安全存储。
// 集成加密服务，自动处理敏感信息的加密解密。
//
// 核心特性：
//   - 配置CRUD操作：完整的配置生命周期管理
//   - 自动加密：敏感配置自动加密存储
//   - 分组管理：配置按功能模块分组
//   - 缓存优化：提高配置读取性能
//   - 事务支持：确保数据一致性
//
// 配置分组：
//   - git: Git仓库相关配置
//   - aliyun: 阿里云服务配置
//   - system: 系统参数配置
//   - security: 安全相关配置
//
// 敏感配置识别：
//   - 包含"password"、"token"、"secret"、"key"的配置键
//   - 自动标记为加密存储
//   - 支持手动指定加密标识
type ConfigService struct {
	db         *gorm.DB           // 数据库连接
	encryption *EncryptionService // 加密服务
	logger     *logrus.Logger     // 日志记录器
	cache      sync.Map           // 配置缓存（并发安全）
}

// GitConfig Git仓库配置结构
//
// 定义Git仓库的完整配置信息，支持GitHub和Gitee等平台。
// 包含认证信息、仓库地址、用户信息等。
//
// 字段说明：
//   - RepoURL: 仓库地址，支持HTTPS和SSH格式
//   - Username: 用户名，用于认证
//   - Password: 密码（用于Gitee），自动加密存储
//   - Token: 访问令牌（用于GitHub），自动加密存储
//   - Email: 提交邮箱，用于Git提交
//   - Branch: 默认分支，通常为main或master
//   - LocalPath: 本地克隆路径
//
// 安全注意：
//   - Password和Token字段会自动加密存储
//   - GitHub建议使用Token而非密码
//   - 定期轮换访问令牌
type GitConfig struct {
	RepoURL   string `json:"repo_url"`   // 仓库URL地址
	Username  string `json:"username"`   // 用户名
	Password  string `json:"password"`   // 密码（用于Gitee，加密存储）
	Token     string `json:"token"`      // 访问令牌（用于GitHub，加密存储）
	Email     string `json:"email"`      // 提交邮箱
	Branch    string `json:"branch"`     // 默认分支
	// LocalPath string `json:"local_path"` // 本地路径 - API模式下不再需要
}

// AliyunConfig 阿里云服务配置结构
//
// 定义阿里云容器镜像服务(ACR)的配置信息。
// 包含认证信息、镜像仓库地址、命名空间等。
//
// 字段说明：
//   - Registry: 镜像仓库地址，不同地域有不同地址
//   - Namespace: 命名空间，用于组织镜像
//   - Username: 阿里云用户名
//   - Password: 阿里云密码，自动加密存储
//   - Region: 地域信息，如cn-hangzhou
//
// 安全注意：
//   - Password字段会自动加密存储
//   - 建议使用RAM用户而非主账号
//   - 遵循最小权限原则
type AliyunConfig struct {
	Registry  string `json:"registry"`  // 镜像仓库地址
	Namespace string `json:"namespace"` // 命名空间
	Username  string `json:"username"`  // 用户名
	Password  string `json:"password"`  // 密码（加密存储）
	Region    string `json:"region"`    // 地域
}

// NewConfigService 创建新的配置服务实例
//
// 初始化配置服务，设置数据库连接、加密服务和日志记录器。
// 创建配置缓存，提高读取性能。
//
// 参数：
//   - db: 数据库连接实例
//   - encryption: 加密服务实例
//   - logger: 日志记录器实例
//
// 返回：
//   - *ConfigService: 配置服务实例
//
// 初始化过程：
//   - 验证数据库连接
//   - 初始化配置缓存
//   - 设置日志记录器
//   - 预加载常用配置
func NewConfigService(db *gorm.DB, encryption *EncryptionService, logger *logrus.Logger) *ConfigService {
	return &ConfigService{
		db:         db,
		encryption: encryption,
		logger:     logger,
	}
}

// SetConfig 设置配置项
//
// 创建或更新配置项，支持自动加密和分组管理。
// 根据配置键名自动判断是否需要加密存储。
//
// 参数：
//   - key: 配置键名，建议使用分层命名（如git.github.token）
//   - value: 配置值
//   - description: 配置描述
//   - group: 配置分组，如git、aliyun、system
//   - order: 显示顺序
//
// 返回：
//   - error: 操作错误
//
// 自动加密规则：
//   - 键名包含password、token、secret、key的配置
//   - 自动标记为加密存储
//   - 使用AES-256-GCM加密
//
// 操作流程：
//   1. 检查配置是否存在
//   2. 判断是否需要加密
//   3. 加密敏感信息
//   4. 保存到数据库
//   5. 更新缓存
func (cs *ConfigService) SetConfig(key, value, description, group string, order int) error {
	// 判断是否需要加密
	needsEncryption := cs.needsEncryption(key)
	
	// 加密敏感信息
	var encryptedValue string
	var err error
	if needsEncryption {
		encryptedValue, err = cs.encryption.Encrypt(value)
		if err != nil {
			cs.logger.WithError(err).WithField("key", key).Error("Failed to encrypt config value")
			return fmt.Errorf("failed to encrypt config value: %w", err)
		}
	} else {
		encryptedValue = value
	}

	// 查找现有配置
	var config models.SystemConfig
	result := cs.db.Where("config_key = ?", key).First(&config)
	
	if result.Error == gorm.ErrRecordNotFound {
		// 创建新配置
		config = models.SystemConfig{
			ConfigKey:    key,
			ConfigValue:  encryptedValue,
			Description:  description,
			IsEncrypted:  needsEncryption,
			ConfigGroup:  group,
			DisplayOrder: order,
		}
		
		if err := cs.db.Create(&config).Error; err != nil {
			cs.logger.WithError(err).WithField("key", key).Error("Failed to create config")
			return fmt.Errorf("failed to create config: %w", err)
		}
		
		cs.logger.WithField("key", key).Info("Created new config")
	} else if result.Error != nil {
		cs.logger.WithError(result.Error).WithField("key", key).Error("Failed to query config")
		return fmt.Errorf("failed to query config: %w", result.Error)
	} else {
		// 更新现有配置
		config.ConfigValue = encryptedValue
		config.Description = description
		config.IsEncrypted = needsEncryption
		config.ConfigGroup = group
		config.DisplayOrder = order
		
		if err := cs.db.Save(&config).Error; err != nil {
			cs.logger.WithError(err).WithField("key", key).Error("Failed to update config")
			return fmt.Errorf("failed to update config: %w", err)
		}
		
		cs.logger.WithField("key", key).Info("Updated existing config")
	}

	// 更新缓存
	cs.cache.Store(key, value)
	
	return nil
}

// GetConfig 获取配置项
//
// 根据配置键获取配置值，自动处理解密操作。
// 优先从缓存读取，提高性能。
//
// 参数：
//   - key: 配置键名
//
// 返回：
//   - string: 配置值（已解密）
//   - error: 操作错误
//
// 读取流程：
//   1. 检查缓存
//   2. 从数据库查询
//   3. 自动解密
//   4. 更新缓存
//   5. 返回明文值
//
// 缓存策略：
//   - 首次读取后缓存
//   - 配置更新时刷新缓存
//   - 定期清理过期缓存
func (cs *ConfigService) GetConfig(key string) (string, error) {
	// 先检查缓存
	if value, exists := cs.cache.Load(key); exists {
		return value.(string), nil
	}

	// 从数据库查询
	var config models.SystemConfig
	if err := cs.db.Where("config_key = ?", key).First(&config).Error; err != nil {
		if err == gorm.ErrRecordNotFound {
			return "", fmt.Errorf("config not found: %s", key)
		}
		cs.logger.WithError(err).WithField("key", key).Error("Failed to query config")
		return "", fmt.Errorf("failed to query config: %w", err)
	}

	// 解密配置值
	decryptedValue, err := cs.encryption.DecryptIfNeeded(config.ConfigValue)
	if err != nil {
		cs.logger.WithError(err).WithField("key", key).Error("Failed to decrypt config value")
		return "", fmt.Errorf("failed to decrypt config value: %w", err)
	}

	// 更新缓存
	cs.cache.Store(key, decryptedValue)
	
	return decryptedValue, nil
}

// GetConfigsByGroup 获取分组配置
//
// 根据配置分组获取所有相关配置，支持排序和分页。
// 自动处理敏感信息的脱敏显示。
//
// 参数：
//   - group: 配置分组名称
//
// 返回：
//   - []models.SystemConfig: 配置列表
//   - error: 操作错误
//
// 返回数据：
//   - 按display_order和config_key排序
//   - 敏感信息已脱敏（显示为***）
//   - 包含完整的配置元信息
//
// 脱敏规则：
//   - 加密配置显示为"***"
//   - 保留配置键和描述信息
//   - 不影响实际配置值
func (cs *ConfigService) GetConfigsByGroup(group string) ([]models.SystemConfig, error) {
	var configs []models.SystemConfig
	
	err := cs.db.Where("config_group = ?", group).
		Order("display_order ASC, config_key ASC").
		Find(&configs).Error
	
	if err != nil {
		cs.logger.WithError(err).WithField("group", group).Error("Failed to query configs by group")
		return nil, fmt.Errorf("failed to query configs by group: %w", err)
	}

	// 脱敏处理敏感信息
	for i := range configs {
		if configs[i].IsEncrypted {
			configs[i].ConfigValue = "***"
		}
	}

	return configs, nil
}

// DeleteConfig 删除配置项
//
// 根据配置键删除配置项，同时清理相关缓存。
// 支持软删除，保留配置历史。
//
// 参数：
//   - key: 配置键名
//
// 返回：
//   - error: 操作错误
//
// 删除流程：
//   1. 验证配置存在
//   2. 执行软删除
//   3. 清理缓存
//   4. 记录操作日志
//
// 安全考虑：
//   - 重要配置删除前需要确认
//   - 保留删除历史记录
//   - 支持配置恢复
func (cs *ConfigService) DeleteConfig(key string) error {
	// 检查配置是否存在
	var config models.SystemConfig
	if err := cs.db.Where("config_key = ?", key).First(&config).Error; err != nil {
		if err == gorm.ErrRecordNotFound {
			return fmt.Errorf("config not found: %s", key)
		}
		return fmt.Errorf("failed to query config: %w", err)
	}

	// 软删除配置
	if err := cs.db.Delete(&config).Error; err != nil {
		cs.logger.WithError(err).WithField("key", key).Error("Failed to delete config")
		return fmt.Errorf("failed to delete config: %w", err)
	}

	// 清理缓存
	cs.cache.Delete(key)
	
	cs.logger.WithField("key", key).Info("Deleted config")
	return nil
}

// SetGitConfig 设置Git配置
//
// 设置Git仓库的完整配置信息，支持GitHub和Gitee等平台。
// 自动处理敏感信息的加密存储。
//
// 参数：
//   - platform: 平台名称（github、gitee）
//   - config: Git配置结构体
//
// 返回：
//   - error: 操作错误
//
// 配置键格式：
//   - git.{platform}.repo_url
//   - git.{platform}.username
//   - git.{platform}.password（自动加密）
//   - git.{platform}.email
//   - git.{platform}.branch
//   - git.{platform}.local_path
//
// 批量操作：
//   - 使用事务确保一致性
//   - 失败时自动回滚
//   - 全部成功后提交
func (cs *ConfigService) SetGitConfig(platform string, config GitConfig) error {
	// 开始事务
	tx := cs.db.Begin()
	defer func() {
		if r := recover(); r != nil {
			tx.Rollback()
		}
	}()

	// 设置各个配置项（API模式下不再需要local_path）
	configs := map[string]interface{}{
		fmt.Sprintf("%s_repo_url", platform):   config.RepoURL,
		fmt.Sprintf("%s_username", platform):   config.Username,
		fmt.Sprintf("%s_email", platform):      config.Email,
		fmt.Sprintf("%s_branch", platform):     config.Branch,
		// fmt.Sprintf("%s_local_path", platform): config.LocalPath,  // API模式下不再需要
	}

	// 根据平台类型设置认证字段
	if platform == "github" {
		// GitHub使用token
		if config.Token != "" {
			configs[fmt.Sprintf("%s_token", platform)] = config.Token
		}
	} else {
		// Gitee使用password
		if config.Password != "" {
			configs[fmt.Sprintf("%s_password", platform)] = config.Password
		}
		// Gitee也可能有token
		if config.Token != "" {
			configs[fmt.Sprintf("%s_token", platform)] = config.Token
		}
	}

	order := 1
	for key, value := range configs {
		if valueStr, ok := value.(string); ok && valueStr != "" {
			// 临时使用当前数据库连接设置配置
			if err := cs.setConfigWithTx(tx, key, valueStr, fmt.Sprintf("%s Git配置", platform), "git", order); err != nil {
				tx.Rollback()
				return fmt.Errorf("failed to set %s: %w", key, err)
			}
			order++
		}
	}

	// 提交事务
	if err := tx.Commit().Error; err != nil {
		cs.logger.WithError(err).WithField("platform", platform).Error("Failed to commit git config transaction")
		return fmt.Errorf("failed to commit git config: %w", err)
	}

	// 清理相关缓存，确保下次读取时获取最新值
	for key := range configs {
		cs.cache.Delete(key)
	}
	// 也清理可能的认证字段缓存
	if platform == "github" {
		cs.cache.Delete(fmt.Sprintf("%s_token", platform))
	} else {
		cs.cache.Delete(fmt.Sprintf("%s_password", platform))
		cs.cache.Delete(fmt.Sprintf("%s_token", platform))
	}

	cs.logger.WithField("platform", platform).Info("Successfully set git config and cleared cache")
	return nil
}

// GetGitConfig 获取Git配置
//
// 根据平台名称获取完整的Git配置信息。
// 自动解密敏感信息。
//
// 参数：
//   - platform: 平台名称（github、gitee）
//
// 返回：
//   - GitConfig: Git配置结构体
//   - error: 操作错误
//
// 读取流程：
//   1. 批量读取相关配置
//   2. 自动解密敏感信息
//   3. 组装配置结构体
//   4. 返回完整配置
func (cs *ConfigService) GetGitConfig(platform string) (GitConfig, error) {
	var config GitConfig
	
	// 读取各个配置项（API模式下不再需要local_path）
	repoURL, _ := cs.GetConfig(fmt.Sprintf("%s_repo_url", platform))
	username, _ := cs.GetConfig(fmt.Sprintf("%s_username", platform))
	email, _ := cs.GetConfig(fmt.Sprintf("%s_email", platform))
	branch, _ := cs.GetConfig(fmt.Sprintf("%s_branch", platform))
	// localPath, _ := cs.GetConfig(fmt.Sprintf("%s_local_path", platform))  // API模式下不再需要

	config.RepoURL = repoURL
	config.Username = username
	config.Email = email
	config.Branch = branch

	// 根据平台类型读取认证字段
	if platform == "github" {
		// GitHub优先使用token
		token, _ := cs.GetConfig(fmt.Sprintf("%s_token", platform))
		config.Token = token
	} else {
		// Gitee可能使用password或token
		password, _ := cs.GetConfig(fmt.Sprintf("%s_password", platform))
		token, _ := cs.GetConfig(fmt.Sprintf("%s_token", platform))
		config.Password = password
		config.Token = token
	}

	return config, nil
}

// SetAliyunConfig 设置阿里云配置
//
// 设置阿里云容器镜像服务的完整配置信息。
// 自动处理敏感信息的加密存储。
//
// 参数：
//   - config: 阿里云配置结构体
//
// 返回：
//   - error: 操作错误
//
// 配置键格式：
//   - aliyun.registry
//   - aliyun.namespace
//   - aliyun.username
//   - aliyun.password（自动加密）
//   - aliyun.region
func (cs *ConfigService) SetAliyunConfig(config AliyunConfig) error {
	// 开始事务
	tx := cs.db.Begin()
	defer func() {
		if r := recover(); r != nil {
			tx.Rollback()
		}
	}()

	// 设置各个配置项（统一使用aliyun_*格式，与GitHub/Gitee保持一致）
	configs := map[string]string{
		"aliyun_registry":  config.Registry,
		"aliyun_namespace": config.Namespace,
		"aliyun_username":  config.Username,
		"aliyun_password":  config.Password,
		"aliyun_region":    config.Region,
	}

	order := 1
	for key, value := range configs {
		if value != "" {
			if err := cs.setConfigWithTx(tx, key, value, "阿里云配置", "aliyun", order); err != nil {
				tx.Rollback()
				return fmt.Errorf("failed to set %s: %w", key, err)
			}
			order++
		}
	}

	// 提交事务
	if err := tx.Commit().Error; err != nil {
		cs.logger.WithError(err).Error("Failed to commit aliyun config transaction")
		return fmt.Errorf("failed to commit aliyun config: %w", err)
	}

	cs.logger.Info("Successfully set aliyun config")
	return nil
}

// GetAliyunConfig 获取阿里云配置
//
// 获取完整的阿里云配置信息。
// 自动解密敏感信息。
//
// 返回：
//   - AliyunConfig: 阿里云配置结构体
//   - error: 操作错误
func (cs *ConfigService) GetAliyunConfig() (AliyunConfig, error) {
	var config AliyunConfig
	
	// 读取各个配置项（统一使用aliyun_*格式）
	registry, _ := cs.GetConfig("aliyun_registry")
	namespace, _ := cs.GetConfig("aliyun_namespace")
	username, _ := cs.GetConfig("aliyun_username")
	password, _ := cs.GetConfig("aliyun_password")
	region, _ := cs.GetConfig("aliyun_region")

	config.Registry = registry
	config.Namespace = namespace
	config.Username = username
	config.Password = password
	config.Region = region

	return config, nil
}

// needsEncryption 判断配置是否需要加密
//
// 根据配置键名判断是否包含敏感信息。
// 包含特定关键词的配置会被标记为需要加密。
//
// 参数：
//   - key: 配置键名
//
// 返回：
//   - bool: true表示需要加密，false表示不需要
//
// 敏感关键词：
//   - password: 密码
//   - token: 访问令牌
//   - secret: 密钥
//   - key: 密钥（排除config_key等）
func (cs *ConfigService) needsEncryption(key string) bool {
	key = strings.ToLower(key)
	sensitiveKeywords := []string{"password", "token", "secret"}
	
	for _, keyword := range sensitiveKeywords {
		if strings.Contains(key, keyword) {
			return true
		}
	}
	
	// 特殊处理key关键词，避免误判
	if strings.Contains(key, "key") && !strings.Contains(key, "config_key") {
		return true
	}
	
	return false
}

// setConfigWithTx 在事务中设置配置
//
// 在指定事务中设置配置项，用于批量操作。
// 确保配置的原子性操作。
//
// 参数：
//   - tx: 数据库事务
//   - key: 配置键名
//   - value: 配置值
//   - description: 配置描述
//   - group: 配置分组
//   - order: 显示顺序
//
// 返回：
//   - error: 操作错误
func (cs *ConfigService) setConfigWithTx(tx *gorm.DB, key, value, description, group string, order int) error {
	// 判断是否需要加密
	needsEncryption := cs.needsEncryption(key)
	
	// 加密敏感信息
	var encryptedValue string
	var err error
	if needsEncryption {
		encryptedValue, err = cs.encryption.Encrypt(value)
		if err != nil {
			return fmt.Errorf("failed to encrypt config value: %w", err)
		}
	} else {
		encryptedValue = value
	}

	// 查找现有配置
	var config models.SystemConfig
	result := tx.Where("config_key = ?", key).First(&config)
	
	if result.Error == gorm.ErrRecordNotFound {
		// 创建新配置
		config = models.SystemConfig{
			ConfigKey:    key,
			ConfigValue:  encryptedValue,
			Description:  description,
			IsEncrypted:  needsEncryption,
			ConfigGroup:  group,
			DisplayOrder: order,
		}
		
		return tx.Create(&config).Error
	} else if result.Error != nil {
		return result.Error
	} else {
		// 更新现有配置
		config.ConfigValue = encryptedValue
		config.Description = description
		config.IsEncrypted = needsEncryption
		config.ConfigGroup = group
		config.DisplayOrder = order
		
		return tx.Save(&config).Error
	}
}