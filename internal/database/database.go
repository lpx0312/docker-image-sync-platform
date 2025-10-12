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
//   // 初始化数据库
//   if err := database.InitDatabase(); err != nil {
//       log.Fatal(err)
//   }
//   defer database.CloseDatabase()
//
//   // 获取数据库实例
//   db := database.GetDB()
//
// Author: Docker Image Sync Platform Team
// Version: 1.0.0
package database

import (
	"fmt"
	"log"
	"time"

	"docker-image-sync-platform/internal/config"
	"docker-image-sync-platform/internal/models"

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
		&models.ImageSyncRecord{}, // 镜像同步记录表：存储每个镜像的同步状态和结果
		&models.SyncTask{},        // 同步任务表：存储批量同步任务的信息
		&models.SystemConfig{},    // 系统配置表：存储应用运行时的配置参数
	)
	
	if err != nil {
		return fmt.Errorf("数据库表迁移失败: %w", err)
	}

	log.Println("数据库表迁移完成")
	
	// ====================================================================
	// 第二步：初始化系统默认配置
	// ====================================================================
	// 在表结构创建完成后，插入系统运行所需的默认配置
	// 这些配置包括阿里云设置、同步参数等
	if err := initDefaultConfigs(); err != nil {
		return fmt.Errorf("初始化默认配置失败: %w", err)
	}
	
	return nil
}

// initDefaultConfigs 初始化默认配置
//
// 功能说明：
// 1. 创建系统运行所需的默认配置项
// 2. 如果配置已存在则跳过，避免重复创建
// 3. 配置项包括阿里云设置、同步参数等
//
// 默认配置项：
// - aliyun_registry_prefix: 阿里云镜像仓库前缀地址
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
		aliyunRegistry = "registry.cn-hangzhou.aliyuncs.com" // 默认值
	}
	
	aliyunNamespace := config.AppConfig.Aliyun.Namespace
	if aliyunNamespace == "" {
		aliyunNamespace = "your-namespace" // 默认值
	}
	
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
		{
			ConfigKey:   "aliyun_registry_prefix",
			ConfigValue: aliyunRegistry,
			Description: "阿里云镜像仓库前缀",
			// 说明：从config.yaml中的aliyun.registry读取
			// 不同地域有不同的前缀，如杭州地域为 registry.cn-hangzhou.aliyuncs.com
		},
		{
			ConfigKey:   "aliyun_namespace",
			ConfigValue: aliyunNamespace,
			Description: "阿里云镜像仓库命名空间",
			// 说明：从config.yaml中的aliyun.namespace读取
			// 用户在阿里云创建的命名空间名称
		},
		{
			ConfigKey:   "sync_timeout_minutes",
			ConfigValue: syncTimeoutMinutes,
			Description: "同步任务超时时间（分钟）",
			// 说明：从config.yaml中的sync.timeout_minutes读取
			// 单个镜像同步任务的最大执行时间
		},
		{
			ConfigKey:   "max_concurrent_syncs",
			ConfigValue: maxConcurrentSyncs,
			Description: "最大并发同步数量",
			// 说明：从config.yaml中的sync.max_concurrent_jobs读取
			// 同时进行的镜像同步任务数量上限
		},
		{
			ConfigKey:   "max_retry_count",
			ConfigValue: maxRetryCount,
			Description: "同步失败重试次数",
			// 说明：从config.yaml中的sync.max_retry_count读取
			// 同步失败时的自动重试次数
		},
		{
			ConfigKey:   "retry_interval_minutes",
			ConfigValue: retryIntervalMinutes,
			Description: "重试间隔时间（分钟）",
			// 说明：从config.yaml中的sync.retry_interval_minutes读取
			// 重试之间的等待时间
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
			if err := DB.Create(&config).Error; err != nil {
				return fmt.Errorf("创建默认配置 %s 失败: %w", config.ConfigKey, err)
			}
			log.Printf("创建默认配置: %s = %s", config.ConfigKey, config.ConfigValue)
		}
		// 如果配置已存在，则跳过创建，保持用户的自定义设置
	}

	return nil
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