// Package database 提供数据库连接和管理功能
//
// 本包负责：
// 1. 数据库连接的初始化和配置
// 2. 连接池的管理和优化
// 3. 数据库表的自动迁移
// 4. 系统默认配置的初始化
// 5. 数据库连接的优雅关闭
//
// 技术栈：
// - GORM: Go语言ORM库，提供数据库操作抽象
// - MySQL: 关系型数据库，存储镜像同步记录和配置
// - 连接池: 优化数据库连接性能和资源使用
//
// 使用方式：
//
//	// 初始化数据库
//	if err := database.InitDatabase(); err != nil {
//	    log.Fatal(err)
//	}
//	defer database.CloseDatabase()
//
//	// 获取数据库实例
//	db := database.GetDB()
//
// Author: Docker Image Sync Platform Team
// Version: 1.0.0
package database

import (
	"crypto/aes"
	"crypto/cipher"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"fmt"
	"io"
	"log"
	"os"
	"strings"
	"time"

	"docker-image-sync-platform/internal/config"
	"docker-image-sync-platform/internal/models"

	"golang.org/x/crypto/bcrypt"
	"gorm.io/driver/mysql"
	"gorm.io/gorm"
	"gorm.io/gorm/logger"
)

// DB 全局数据库连接实例
// 使用GORM提供的数据库抽象层，支持MySQL数据库
// 注意：这是一个全局变量，在应用启动时初始化，整个应用生命周期内复用
var DB *gorm.DB

// InitDatabase 初始化数据库连接
//
// 功能说明：
// 1. 根据配置文件建立MySQL数据库连接
// 2. 配置连接池参数以优化性能
// 3. 测试数据库连接的可用性
// 4. 设置GORM日志级别为Info，便于调试
//
// 连接池配置：
// - MaxIdleConns: 10 (最大空闲连接数)
// - MaxOpenConns: 100 (最大打开连接数)
// - ConnMaxLifetime: 1小时 (连接最大生存时间)
//
// 返回值：
//   - error: 连接失败时返回错误信息，成功时返回nil
func InitDatabase() error {
	// ====================================================================
	// 第一步：构建数据库连接字符串
	// ====================================================================
	// 从配置文件中获取数据库连接信息（主机、端口、用户名、密码等）
	// DSN格式：username:password@tcp(host:port)/database?charset=utf8mb4&parseTime=True&loc=Local
	dsn := config.AppConfig.Database.GetDSN()

	// ====================================================================
	// 第二步：建立GORM数据库连接
	// ====================================================================
	var err error
	DB, err = gorm.Open(mysql.Open(dsn), &gorm.Config{
		// 设置日志级别为Info，记录SQL执行信息
		// 生产环境可考虑设置为Error级别以减少日志输出
		Logger: logger.Default.LogMode(logger.Info),
	})

	if err != nil {
		return fmt.Errorf("连接数据库失败: %w", err)
	}

	// ====================================================================
	// 第三步：配置数据库连接池
	// ====================================================================
	// 获取底层的sql.DB对象，用于设置连接池参数
	// GORM在底层使用标准库的sql.DB进行连接管理
	sqlDB, err := DB.DB()
	if err != nil {
		return fmt.Errorf("获取数据库连接失败: %w", err)
	}

	// 连接池参数配置（根据应用负载和数据库性能调整）
	sqlDB.SetMaxIdleConns(10)           // 最大空闲连接数：保持10个连接以快速响应请求
	sqlDB.SetMaxOpenConns(100)          // 最大打开连接数：限制并发连接数，避免数据库过载
	sqlDB.SetConnMaxLifetime(time.Hour) // 连接最大生存时间：1小时后重新创建连接，避免长连接问题

	// ====================================================================
	// 第四步：测试数据库连接
	// ====================================================================
	// 发送ping命令验证数据库连接是否正常
	// 这是一个轻量级的连接测试，确保数据库可达且可用
	if err := sqlDB.Ping(); err != nil {
		return fmt.Errorf("数据库连接测试失败: %w", err)
	}

	log.Println("数据库连接成功")

	// 执行数据库迁移
	RunMigrations()

	return nil
}

// AutoMigrate 自动迁移数据库表
//
// 功能说明：
// 1. 根据Go模型结构自动创建或更新数据库表
// 2. 确保数据库表结构与代码模型保持同步
// 3. 初始化系统运行所需的默认配置
//
// 迁移的表：
// - image_sync_records: 镜像同步记录表
// - sync_tasks: 同步任务表
// - system_configs: 系统配置表
//
// 注意事项：
// - GORM的AutoMigrate只会添加新字段和索引，不会删除现有字段
// - 生产环境建议使用专门的迁移脚本而非AutoMigrate
// - 此函数应在数据库连接建立后立即调用
//
// 返回值：
//   - error: 迁移失败时返回错误信息，成功时返回nil
func AutoMigrate() error {
	// ====================================================================
	// 第一步：执行数据库表结构迁移
	// ====================================================================
	// 使用GORM的AutoMigrate功能自动创建或更新表结构
	// 这会根据模型定义创建表、字段、索引等
	err := DB.AutoMigrate(
		&models.ImageSyncRecord{},
		&models.SyncTask{},
		&models.SystemConfig{},
		&models.User{},
		&models.LoginLog{},
	)

	if err != nil {
		return fmt.Errorf("数据库表迁移失败: %w", err)
	}

	log.Println("数据库表迁移完成")

	if err := initDefaultConfigs(); err != nil {
		return fmt.Errorf("初始化默认配置失败: %w", err)
	}

	if err := initDefaultAdmin(); err != nil {
		return fmt.Errorf("初始化默认管理员失败: %w", err)
	}

	return nil
}

// initDefaultAdmin 初始化默认管理员账号
func initDefaultAdmin() error {
	var count int64
	DB.Model(&models.User{}).Count(&count)
	if count > 0 {
		return nil
	}

	username := config.AppConfig.Auth.DefaultAdminUsername
	password := config.AppConfig.Auth.DefaultAdminPassword
	if username == "" {
		username = "admin"
	}
	if password == "" {
		password = "admin123"
	}

	hash, err := bcrypt.GenerateFromPassword([]byte(password), bcrypt.DefaultCost)
	if err != nil {
		return fmt.Errorf("加密默认管理员密码失败: %w", err)
	}

	admin := models.User{
		Username:     username,
		PasswordHash: string(hash),
		Email:        "admin@example.com",
		Role:         models.RoleAdmin,
		Status:       models.UserStatusActive,
	}

	if err := DB.Create(&admin).Error; err != nil {
		return fmt.Errorf("创建默认管理员失败: %w", err)
	}

	log.Printf("默认管理员账号已创建: %s (请及时修改默认密码)", username)
	return nil
}

// encryptSensitiveValue 加密敏感配置值
// 使用AES-256-GCM算法加密敏感数据，避免循环依赖
func encryptSensitiveValue(plaintext string) (string, error) {
	if plaintext == "" {
		return "", nil
	}

	// 获取加密密钥
	var key []byte
	if envKey := os.Getenv("ENCRYPTION_KEY"); envKey != "" {
		hash := sha256.Sum256([]byte(envKey))
		key = hash[:]
	} else if ginMode := os.Getenv("GIN_MODE"); ginMode == "release" {
		return "", fmt.Errorf("生产环境必须设置 ENCRYPTION_KEY 环境变量")
	} else {
		defaultKey := "docker-sync-platform-default-key-2024"
		hash := sha256.Sum256([]byte(defaultKey))
		key = hash[:]
	}

	// 创建AES密码器
	block, err := aes.NewCipher(key)
	if err != nil {
		return "", fmt.Errorf("failed to create cipher: %w", err)
	}

	// 创建GCM模式
	gcm, err := cipher.NewGCM(block)
	if err != nil {
		return "", fmt.Errorf("failed to create GCM: %w", err)
	}

	// 生成随机nonce
	nonce := make([]byte, gcm.NonceSize())
	if _, err := io.ReadFull(rand.Reader, nonce); err != nil {
		return "", fmt.Errorf("failed to generate nonce: %w", err)
	}

	// 加密数据
	ciphertext := gcm.Seal(nonce, nonce, []byte(plaintext), nil)

	// Base64编码并添加前缀
	encoded := base64.StdEncoding.EncodeToString(ciphertext)
	return "ENC:" + encoded, nil
}

// initDefaultConfigs 初始化默认配置
//
// 功能说明：
// 1. 创建系统运行所需的默认配置项
// 2. 如果配置已存在则跳过，避免重复创建
// 3. 配置项包括阿里云设置、同步参数等
// 4. 自动加密敏感配置字段
//
// 默认配置项：

// - aliyun_namespace: 阿里云镜像仓库命名空间
// - sync_check_interval: 同步状态检查间隔时间
// - max_concurrent_syncs: 最大并发同步数量
//
// 返回值：
//   - error: 配置初始化失败时返回错误信息，成功时返回nil
func initDefaultConfigs() error {
	// ====================================================================
	// 从配置文件读取系统默认配置项
	// ====================================================================
	// 这些配置项从 config.yaml 文件中读取，避免硬编码
	// 如果配置文件中没有相应配置，则使用合理的默认值

	// 从配置文件中读取阿里云配置
	aliyunRegistry := config.AppConfig.Aliyun.Registry
	if aliyunRegistry == "" {
		aliyunRegistry = "registry.cn-hangzhou.aliyuncs.com"
	}
	
	aliyunNamespace := config.AppConfig.Aliyun.Namespace
	if aliyunNamespace == "" {
		aliyunNamespace = "your-namespace"
	}
	
	aliyunUsername := config.AppConfig.Aliyun.Username
	aliyunPassword := config.AppConfig.Aliyun.Password

	// 从配置文件中读取Git仓库类型
	gitRepositoryType := config.AppConfig.Git.RepositoryType
	if gitRepositoryType == "" {
		gitRepositoryType = "gitee" // 默认使用Gitee
	}

	// 从配置文件中读取Git配置
	giteeRepoURL := config.AppConfig.Git.Gitee.RepoURL
	giteeUsername := config.AppConfig.Git.Gitee.Username
	giteePassword := config.AppConfig.Git.Gitee.Password
	giteeToken := config.AppConfig.Git.Gitee.Token
	giteeEmail := config.AppConfig.Git.Gitee.Email
	giteeBranch := config.AppConfig.Git.Gitee.Branch
	if giteeBranch == "" {
		giteeBranch = "main" // 默认分支
	}
	// API模式下不再需要本地路径
	// giteeLocalPath := config.AppConfig.Git.Gitee.LocalPath
	// if giteeLocalPath == "" {
	//	giteeLocalPath = "./gitee-repo" // 默认本地路径
	// }

	githubRepoURL := config.AppConfig.Git.GitHub.RepoURL
	githubUsername := config.AppConfig.Git.GitHub.Username
	githubToken := config.AppConfig.Git.GitHub.Token
	githubEmail := config.AppConfig.Git.GitHub.Email
	githubBranch := config.AppConfig.Git.GitHub.Branch
	if githubBranch == "" {
		githubBranch = "main" // 默认分支
	}
	// API模式下不再需要本地路径
	// githubLocalPath := config.AppConfig.Git.GitHub.LocalPath
	// if githubLocalPath == "" {
	//	githubLocalPath = "./github-repo" // 默认本地路径
	// }
	


	// 从配置文件中读取同步配置
	syncTimeoutMinutes := fmt.Sprintf("%d", config.AppConfig.Sync.TimeoutMinutes)
	if config.AppConfig.Sync.TimeoutMinutes == 0 {
		syncTimeoutMinutes = "30" // 默认30分钟
	}

	// 从配置文件中读取最大并发数
	maxConcurrentSyncs := fmt.Sprintf("%d", config.AppConfig.Sync.MaxConcurrentJobs)
	if config.AppConfig.Sync.MaxConcurrentJobs == 0 {
		maxConcurrentSyncs = "3" // 默认3个并发任务，与config.yaml中的值一致
	}

	// 从配置文件中读取重试配置
	maxRetryCount := fmt.Sprintf("%d", config.AppConfig.Sync.MaxRetryCount)
	if config.AppConfig.Sync.MaxRetryCount == 0 {
		maxRetryCount = "3" // 默认重试3次
	}

	retryIntervalMinutes := fmt.Sprintf("%d", config.AppConfig.Sync.RetryIntervalMinutes)
	if config.AppConfig.Sync.RetryIntervalMinutes == 0 {
		retryIntervalMinutes = "5" // 默认重试间隔5分钟
	}

	defaultConfigs := []models.SystemConfig{
		// 阿里云配置
		{
			ConfigKey:   "aliyun_namespace",
			ConfigValue: aliyunNamespace,
			Description: "阿里云镜像仓库命名空间",
		},
		{
			ConfigKey:   "aliyun_registry",
			ConfigValue: aliyunRegistry,
			Description: "阿里云镜像仓库地址",
		},
		{
			ConfigKey:   "aliyun_username",
			ConfigValue: aliyunUsername,
			Description: "阿里云镜像仓库用户名",
		},
		{
			ConfigKey:   "aliyun_password",
			ConfigValue: aliyunPassword,
			Description: "阿里云镜像仓库密码",
			IsEncrypted: true,
		},
		
		// Git仓库配置
		{
			ConfigKey:   "git_repository_type",
			ConfigValue: gitRepositoryType,
			Description: "Git仓库类型 (gitee/github)",
		},

		
		// Gitee配置
		{
			ConfigKey:   "gitee_repo_url",
			ConfigValue: giteeRepoURL,
			Description: "Gitee仓库URL",
		},
		{
			ConfigKey:   "gitee_username",
			ConfigValue: giteeUsername,
			Description: "Gitee用户名",
		},
		{
			ConfigKey:   "gitee_password",
			ConfigValue: giteePassword,
			Description: "Gitee密码",
			IsEncrypted: true,
		},
		{
			ConfigKey:   "gitee_token",
			ConfigValue: giteeToken,
			Description: "Gitee访问令牌",
			IsEncrypted: true,
		},
		{
			ConfigKey:   "gitee_email",
			ConfigValue: giteeEmail,
			Description: "Gitee邮箱地址",
		},
		{
			ConfigKey:   "gitee_branch",
			ConfigValue: giteeBranch,
			Description: "Gitee仓库分支",
		},
		// API模式下不再需要本地路径配置
		// {
		//	ConfigKey:   "gitee_local_path",
		//	ConfigValue: giteeLocalPath,
		//	Description: "Gitee本地仓库路径",
		// },
		
		// GitHub配置
		{
			ConfigKey:   "github_repo_url",
			ConfigValue: githubRepoURL,
			Description: "GitHub仓库URL",
		},
		{
			ConfigKey:   "github_username",
			ConfigValue: githubUsername,
			Description: "GitHub用户名",
		},
		{
			ConfigKey:   "github_token",
			ConfigValue: githubToken,
			Description: "GitHub访问令牌",
			IsEncrypted: true,
		},
		{
			ConfigKey:   "github_email",
			ConfigValue: githubEmail,
			Description: "GitHub邮箱地址",
		},
		{
			ConfigKey:   "github_branch",
			ConfigValue: githubBranch,
			Description: "GitHub仓库分支",
		},
		// API模式下不再需要本地路径配置
		// {
		//	ConfigKey:   "github_local_path",
		//	ConfigValue: githubLocalPath,
		//	Description: "GitHub本地仓库路径",
		// },
		
		// 同步配置
		{
			ConfigKey:   "sync_timeout_minutes",
			ConfigValue: syncTimeoutMinutes,
			Description: "同步任务超时时间（分钟）",
		},
		{
			ConfigKey:   "max_concurrent_syncs",
			ConfigValue: maxConcurrentSyncs,
			Description: "最大并发同步数量",
		},
		{
			ConfigKey:   "max_retry_count",
			ConfigValue: maxRetryCount,
			Description: "同步失败重试次数",
		},
		{
			ConfigKey:   "retry_interval_minutes",
			ConfigValue: retryIntervalMinutes,
			Description: "重试间隔时间（分钟）",
		},
	}

	// ====================================================================
	// 批量创建默认配置
	// ====================================================================
	// 遍历所有默认配置，检查是否已存在，不存在则创建
	for _, config := range defaultConfigs {
		var existingConfig models.SystemConfig

		// 查询配置是否已存在
		result := DB.Where("config_key = ?", config.ConfigKey).First(&existingConfig)

		// 如果配置不存在，则创建新配置
		if result.Error == gorm.ErrRecordNotFound {
			// 如果是敏感字段且有值，则先加密
			if config.IsEncrypted && config.ConfigValue != "" {
				encryptedValue, err := encryptSensitiveValue(config.ConfigValue)
				if err != nil {
					return fmt.Errorf("加密配置 %s 失败: %w", config.ConfigKey, err)
				}
				config.ConfigValue = encryptedValue
				log.Printf("创建加密配置: %s = %s", config.ConfigKey, "ENC:***")
			} else {
				log.Printf("创建默认配置: %s = %s", config.ConfigKey, config.ConfigValue)
			}

			if err := DB.Create(&config).Error; err != nil {
				return fmt.Errorf("创建默认配置 %s 失败: %w", config.ConfigKey, err)
			}
		}
		// 如果配置已存在，则跳过创建，保持用户的自定义设置
	}

	return nil
}

// MigrateEncryptionKeys 启动时检查并迁移加密密钥
//
// 当用户首次设置 ENCRYPTION_KEY 环境变量时，数据库中可能存在用旧默认密钥加密的数据。
// 此函数用旧密钥解密后，再用新密钥重新加密，确保平滑过渡。
func MigrateEncryptionKeys() error {
	envKey := os.Getenv("ENCRYPTION_KEY")
	if envKey == "" {
		return nil
	}

	newKeyHash := sha256.Sum256([]byte(envKey))
	newKey := newKeyHash[:]

	oldDefaultKey := "docker-sync-platform-default-key-2024"
	oldKeyHash := sha256.Sum256([]byte(oldDefaultKey))
	oldKey := oldKeyHash[:]

	type configRow struct {
		ID          uint
		ConfigKey   string
		ConfigValue string
		IsEncrypted bool
	}

	var records []configRow
	if err := DB.Raw("SELECT id, config_key, config_value, is_encrypted FROM system_configs WHERE config_value LIKE 'ENC:%' AND deleted_at IS NULL").Scan(&records).Error; err != nil {
		return fmt.Errorf("查询加密配置失败: %w", err)
	}

	if len(records) == 0 {
		log.Println("加密密钥迁移：无加密记录，跳过")
		return nil
	}

	log.Printf("加密密钥迁移：发现 %d 条加密记录，开始检查...", len(records))

	migratedCount := 0
	for _, record := range records {
		// 先尝试用新密钥解密 — 成功说明已是新密钥加密的
		if tryDecryptAESGCM(record.ConfigValue, newKey) {
			continue
		}

		// 用旧密钥解密
		plaintext, err := decryptAESGCM(record.ConfigValue, oldKey)
		if err != nil {
			log.Printf("加密密钥迁移：配置 %s 无法用旧密钥解密（%v），跳过", record.ConfigKey, err)
			continue
		}

		// 用新密钥重新加密
		newCiphertext, err := encryptAESGCM(plaintext, newKey)
		if err != nil {
			return fmt.Errorf("加密密钥迁移：重新加密配置 %s 失败: %w", record.ConfigKey, err)
		}

		// 更新数据库
		if err := DB.Exec("UPDATE system_configs SET config_value = ? WHERE id = ?", newCiphertext, record.ID).Error; err != nil {
			return fmt.Errorf("加密密钥迁移：更新配置 %s 失败: %w", record.ConfigKey, err)
		}

		migratedCount++
		log.Printf("加密密钥迁移：成功迁移配置 %s", record.ConfigKey)
	}

	if migratedCount > 0 {
		log.Printf("加密密钥迁移完成：共迁移 %d 条记录", migratedCount)
	} else {
		log.Println("加密密钥迁移：所有记录已使用当前密钥，无需迁移")
	}

	return nil
}

// tryDecryptAESGCM 尝试用指定密钥解密，成功返回 true
func tryDecryptAESGCM(ciphertext string, key []byte) bool {
	_, err := decryptAESGCM(ciphertext, key)
	return err == nil
}

// decryptAESGCM 用指定密钥解密 AES-GCM 数据
func decryptAESGCM(ciphertext string, key []byte) (string, error) {
	if !strings.HasPrefix(ciphertext, "ENC:") {
		return ciphertext, nil
	}

	encoded := strings.TrimPrefix(ciphertext, "ENC:")
	data, err := base64.StdEncoding.DecodeString(encoded)
	if err != nil {
		return "", err
	}

	block, err := aes.NewCipher(key)
	if err != nil {
		return "", err
	}

	gcm, err := cipher.NewGCM(block)
	if err != nil {
		return "", err
	}

	nonceSize := gcm.NonceSize()
	if len(data) < nonceSize {
		return "", fmt.Errorf("ciphertext too short")
	}

	nonce, cipherData := data[:nonceSize], data[nonceSize:]
	plaintext, err := gcm.Open(nil, nonce, cipherData, nil)
	if err != nil {
		return "", err
	}

	return string(plaintext), nil
}

// encryptAESGCM 用指定密钥加密数据
func encryptAESGCM(plaintext string, key []byte) (string, error) {
	block, err := aes.NewCipher(key)
	if err != nil {
		return "", err
	}

	gcm, err := cipher.NewGCM(block)
	if err != nil {
		return "", err
	}

	nonce := make([]byte, gcm.NonceSize())
	if _, err := io.ReadFull(rand.Reader, nonce); err != nil {
		return "", err
	}

	sealed := gcm.Seal(nonce, nonce, []byte(plaintext), nil)
	encoded := base64.StdEncoding.EncodeToString(sealed)
	return "ENC:" + encoded, nil
}

// GetDB 获取数据库连接
func GetDB() *gorm.DB {
	return DB
}

// CloseDatabase 关闭数据库连接
//
// 功能说明：
// 1. 优雅地关闭数据库连接
// 2. 释放连接池中的所有连接
// 3. 确保数据完整性和资源清理
//
// 使用场景：
// - 应用程序正常退出时
// - 接收到系统信号时
// - 程序异常终止前的清理工作
//
// 注意事项：
// - 此函数应在程序退出前调用
// - 调用后不应再使用数据库连接
// - 建议配合defer语句或信号处理使用
//
// 返回值：
//   - error: 关闭连接失败时返回错误信息，成功时返回nil
func CloseDatabase() error {
	// ====================================================================
	// 检查数据库连接是否存在
	// ====================================================================
	if DB != nil {
		// ====================================================================
		// 获取底层的 *sql.DB 实例
		// ====================================================================
		// GORM 封装了标准库的 database/sql，需要获取底层连接才能关闭
		sqlDB, err := DB.DB()
		if err != nil {
			return fmt.Errorf("获取底层数据库连接失败: %w", err)
		}

		// ====================================================================
		// 关闭数据库连接池
		// ====================================================================
		// 这会关闭连接池中的所有连接，包括空闲和活跃的连接
		// 确保所有数据库资源得到正确释放
		if err := sqlDB.Close(); err != nil {
			return fmt.Errorf("关闭数据库连接失败: %w", err)
		}

		log.Println("数据库连接已关闭")
	}
	// 如果 DB 为 nil，说明数据库连接未初始化或已关闭，无需操作
	return nil
}
