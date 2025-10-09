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

var DB *gorm.DB

// InitDatabase 初始化数据库连接
func InitDatabase() error {
	dsn := config.AppConfig.Database.GetDSN()
	
	var err error
	DB, err = gorm.Open(mysql.Open(dsn), &gorm.Config{
		Logger: logger.Default.LogMode(logger.Info),
	})
	
	if err != nil {
		return fmt.Errorf("连接数据库失败: %w", err)
	}

	// 获取底层的sql.DB对象进行连接池配置
	sqlDB, err := DB.DB()
	if err != nil {
		return fmt.Errorf("获取数据库连接失败: %w", err)
	}

	// 设置连接池参数
	sqlDB.SetMaxIdleConns(10)
	sqlDB.SetMaxOpenConns(100)
	sqlDB.SetConnMaxLifetime(time.Hour)

	// 测试连接
	if err := sqlDB.Ping(); err != nil {
		return fmt.Errorf("数据库连接测试失败: %w", err)
	}

	log.Println("数据库连接成功")
	return nil
}

// AutoMigrate 自动迁移数据库表
func AutoMigrate() error {
	err := DB.AutoMigrate(
		&models.ImageSyncRecord{},
		&models.SyncTask{},
		&models.SystemConfig{},
	)
	
	if err != nil {
		return fmt.Errorf("数据库表迁移失败: %w", err)
	}

	log.Println("数据库表迁移完成")
	
	// 初始化默认配置
	if err := initDefaultConfigs(); err != nil {
		return fmt.Errorf("初始化默认配置失败: %w", err)
	}
	
	return nil
}

// initDefaultConfigs 初始化默认配置
func initDefaultConfigs() error {
	defaultConfigs := []models.SystemConfig{
		{
			ConfigKey:   "aliyun_registry_prefix",
			ConfigValue: config.AppConfig.Aliyun.Registry,
			Description: "阿里云镜像仓库前缀",
		},
		{
			ConfigKey:   "aliyun_namespace",
			ConfigValue: config.AppConfig.Aliyun.Namespace,
			Description: "阿里云镜像仓库命名空间",
		},
		{
			ConfigKey:   "sync_check_interval",
			ConfigValue: "30",
			Description: "同步状态检查间隔（秒）",
		},
		{
			ConfigKey:   "max_concurrent_syncs",
			ConfigValue: "5",
			Description: "最大并发同步数量",
		},
	}

	for _, cfg := range defaultConfigs {
		var existingConfig models.SystemConfig
		result := DB.Where("config_key = ?", cfg.ConfigKey).First(&existingConfig)
		
		if result.Error == gorm.ErrRecordNotFound {
			// 配置不存在，创建新配置
			if err := DB.Create(&cfg).Error; err != nil {
				return fmt.Errorf("创建默认配置失败 [%s]: %w", cfg.ConfigKey, err)
			}
			log.Printf("创建默认配置: %s = %s", cfg.ConfigKey, cfg.ConfigValue)
		} else if result.Error != nil {
			return fmt.Errorf("查询配置失败 [%s]: %w", cfg.ConfigKey, result.Error)
		}
		// 如果配置已存在，不做任何操作
	}

	return nil
}

// GetDB 获取数据库连接
func GetDB() *gorm.DB {
	return DB
}

// CloseDatabase 关闭数据库连接
func CloseDatabase() error {
	if DB != nil {
		sqlDB, err := DB.DB()
		if err != nil {
			return err
		}
		return sqlDB.Close()
	}
	return nil
}