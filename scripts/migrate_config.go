package main

import (
	"fmt"
	"log"
	"os"
	"path/filepath"

	"github.com/sirupsen/logrus"
	"github.com/spf13/viper"
	"gorm.io/driver/mysql"
	"gorm.io/gorm"

	"docker-image-sync-platform/internal/config"
	"docker-image-sync-platform/internal/models"
	"docker-image-sync-platform/internal/services"
)

// ConfigMigrator 配置迁移器
type ConfigMigrator struct {
	db                *gorm.DB
	encryptionService *services.EncryptionService
}

// NewConfigMigrator 创建配置迁移器
func NewConfigMigrator(db *gorm.DB, encryptionService *services.EncryptionService) *ConfigMigrator {
	return &ConfigMigrator{
		db:                db,
		encryptionService: encryptionService,
	}
}

// MigrateFromYAML 从YAML配置文件迁移到数据库
func (m *ConfigMigrator) MigrateFromYAML(configPath string) error {
	// 加载YAML配置
	viper.SetConfigFile(configPath)
	if err := viper.ReadInConfig(); err != nil {
		return fmt.Errorf("读取配置文件失败: %v", err)
	}

	// 迁移Git配置
	if err := m.migrateGitConfig(); err != nil {
		return fmt.Errorf("迁移Git配置失败: %v", err)
	}

	// 迁移阿里云配置
	if err := m.migrateAliyunConfig(); err != nil {
		return fmt.Errorf("迁移阿里云配置失败: %v", err)
	}

	// 迁移其他系统配置
	if err := m.migrateSystemConfig(); err != nil {
		return fmt.Errorf("迁移系统配置失败: %v", err)
	}

	log.Println("配置迁移完成")
	return nil
}

// migrateGitConfig 迁移Git配置
func (m *ConfigMigrator) migrateGitConfig() error {
	log.Println("开始迁移Git配置...")

	// Git仓库类型配置
	repoType := "gitee" // 默认使用gitee
	if viper.IsSet("git.github.repo_url") && viper.GetString("git.github.repo_url") != "" {
		repoType = "github"
	}
	
	if err := m.upsertConfig("git_repository_type", repoType, "git", false, 1); err != nil {
		return err
	}

	// Gitee配置
	if viper.IsSet("git.gitee") {
		configs := []struct {
			key       string
			value     string
			encrypted bool
			order     int
		}{
			{"gitee_repo_url", viper.GetString("git.gitee.repo_url"), false, 2},
			{"gitee_username", viper.GetString("git.gitee.username"), false, 3},
			{"gitee_password", viper.GetString("git.gitee.password"), true, 4},
			{"gitee_email", viper.GetString("git.gitee.email"), false, 5},
		}

		for _, cfg := range configs {
			if err := m.upsertConfig(cfg.key, cfg.value, "git", cfg.encrypted, cfg.order); err != nil {
				return err
			}
		}
	}

	// GitHub配置
	if viper.IsSet("git.github") {
		configs := []struct {
			key       string
			value     string
			encrypted bool
			order     int
		}{
			{"github_repo_url", viper.GetString("git.github.repo_url"), false, 6},
			{"github_username", viper.GetString("git.github.username"), false, 7},
			{"github_token", viper.GetString("git.github.token"), true, 8},
		}

		for _, cfg := range configs {
			if err := m.upsertConfig(cfg.key, cfg.value, "git", cfg.encrypted, cfg.order); err != nil {
				return err
			}
		}
	}

	// Git本地路径配置
	if err := m.upsertConfig("git_local_repo_path", viper.GetString("git.local_repo_path"), "git", false, 9); err != nil {
		return err
	}

	log.Println("Git配置迁移完成")
	return nil
}

// migrateAliyunConfig 迁移阿里云配置
func (m *ConfigMigrator) migrateAliyunConfig() error {
	log.Println("开始迁移阿里云配置...")

	if !viper.IsSet("aliyun") {
		log.Println("未找到阿里云配置，跳过迁移")
		return nil
	}

	configs := []struct {
		key       string
		value     string
		encrypted bool
		order     int
	}{
		{"aliyun_registry", viper.GetString("aliyun.registry"), false, 1},
		{"aliyun_namespace", viper.GetString("aliyun.namespace"), false, 2},
		{"aliyun_username", viper.GetString("aliyun.username"), false, 3},
		{"aliyun_password", viper.GetString("aliyun.password"), true, 4},
	}

	for _, cfg := range configs {
		if err := m.upsertConfig(cfg.key, cfg.value, "aliyun", cfg.encrypted, cfg.order); err != nil {
			return err
		}
	}

	log.Println("阿里云配置迁移完成")
	return nil
}

// migrateSystemConfig 迁移系统配置
func (m *ConfigMigrator) migrateSystemConfig() error {
	log.Println("开始迁移系统配置...")

	// 服务器配置
	if viper.IsSet("server") {
		configs := []struct {
			key   string
			value string
			order int
		}{
			{"server_port", fmt.Sprintf("%d", viper.GetInt("server.port")), 1},
			{"server_mode", viper.GetString("server.mode"), 2},
			{"server_host", viper.GetString("server.host"), 3},
		}

		for _, cfg := range configs {
			if err := m.upsertConfig(cfg.key, cfg.value, "server", false, cfg.order); err != nil {
				return err
			}
		}
	}

	// 同步配置
	if viper.IsSet("sync") {
		configs := []struct {
			key   string
			value string
			order int
		}{
			{"sync_timeout_minutes", fmt.Sprintf("%d", viper.GetInt("sync.timeout_minutes")), 1},
			{"sync_max_concurrent_jobs", fmt.Sprintf("%d", viper.GetInt("sync.max_concurrent_jobs")), 2},
			{"sync_max_retry_count", fmt.Sprintf("%d", viper.GetInt("sync.max_retry_count")), 3},
			{"sync_retry_interval_minutes", fmt.Sprintf("%d", viper.GetInt("sync.retry_interval_minutes")), 4},
		}

		for _, cfg := range configs {
			if err := m.upsertConfig(cfg.key, cfg.value, "sync", false, cfg.order); err != nil {
				return err
			}
		}
	}

	// GitHub Actions配置
	if viper.IsSet("github_actions") {
		configs := []struct {
			key   string
			value string
			order int
		}{
			{"github_actions_workflow_file", viper.GetString("github_actions.workflow_file"), 1},
			{"github_actions_api_timeout_seconds", fmt.Sprintf("%d", viper.GetInt("github_actions.api_timeout_seconds")), 2},
			{"github_actions_status_check_interval_seconds", fmt.Sprintf("%d", viper.GetInt("github_actions.status_check_interval_seconds")), 3},
		}

		for _, cfg := range configs {
			if err := m.upsertConfig(cfg.key, cfg.value, "github_actions", false, cfg.order); err != nil {
				return err
			}
		}
	}

	log.Println("系统配置迁移完成")
	return nil
}

// upsertConfig 插入或更新配置项
func (m *ConfigMigrator) upsertConfig(key, value, group string, encrypted bool, order int) error {
	if value == "" {
		log.Printf("跳过空值配置: %s", key)
		return nil
	}

	// 如果需要加密，先加密值
	finalValue := value
	if encrypted && m.encryptionService != nil {
		encryptedValue, err := m.encryptionService.Encrypt(value)
		if err != nil {
			return fmt.Errorf("加密配置值失败 %s: %v", key, err)
		}
		finalValue = encryptedValue
	}

	// 检查配置是否已存在
	var existingConfig models.SystemConfig
	result := m.db.Where("config_key = ?", key).First(&existingConfig)

	if result.Error == gorm.ErrRecordNotFound {
		// 创建新配置
		newConfig := models.SystemConfig{
			ConfigKey:    key,
			ConfigValue:  finalValue,
			ConfigGroup:  group,
			IsEncrypted:  encrypted,
			DisplayOrder: order,
		}
		if err := m.db.Create(&newConfig).Error; err != nil {
			return fmt.Errorf("创建配置失败 %s: %v", key, err)
		}
		log.Printf("创建配置: %s = %s", key, value)
	} else if result.Error == nil {
		// 更新现有配置
		existingConfig.ConfigValue = finalValue
		existingConfig.ConfigGroup = group
		existingConfig.IsEncrypted = encrypted
		existingConfig.DisplayOrder = order
		if err := m.db.Save(&existingConfig).Error; err != nil {
			return fmt.Errorf("更新配置失败 %s: %v", key, err)
		}
		log.Printf("更新配置: %s = %s", key, value)
	} else {
		return fmt.Errorf("查询配置失败 %s: %v", key, result.Error)
	}

	return nil
}

func main() {
	// 获取项目根目录
	rootDir, err := os.Getwd()
	if err != nil {
		log.Fatal("获取当前目录失败:", err)
	}

	// 如果当前在scripts目录，则回到上级目录
	if filepath.Base(rootDir) == "scripts" {
		rootDir = filepath.Dir(rootDir)
	}

	// 配置文件路径
	configPath := filepath.Join(rootDir, "config.yaml")
	if _, err := os.Stat(configPath); os.IsNotExist(err) {
		log.Fatal("配置文件不存在:", configPath)
	}

	// 加载应用配置
	config.LoadConfig(configPath)

	// 连接数据库
	dsn := fmt.Sprintf("%s:%s@tcp(%s:%d)/%s?charset=%s&parseTime=%t&loc=%s",
		config.AppConfig.Database.Username,
		config.AppConfig.Database.Password,
		config.AppConfig.Database.Host,
		config.AppConfig.Database.Port,
		config.AppConfig.Database.Database,
		config.AppConfig.Database.Charset,
		config.AppConfig.Database.ParseTime,
		config.AppConfig.Database.Loc,
	)

	db, err := gorm.Open(mysql.Open(dsn), &gorm.Config{})
	if err != nil {
		log.Fatal("连接数据库失败:", err)
	}

	// 确保system_configs表存在
	if err := db.AutoMigrate(&models.SystemConfig{}); err != nil {
		log.Fatal("迁移数据库表失败:", err)
	}

	// 初始化加密服务（创建logger）
	logger := logrus.New()
	logger.SetLevel(logrus.InfoLevel)
	encryptionService, err := services.NewEncryptionService(logger)
	if err != nil {
		log.Printf("警告: 初始化加密服务失败，敏感配置将不会被加密: %v", err)
	}

	// 创建迁移器并执行迁移
	migrator := NewConfigMigrator(db, encryptionService)
	if err := migrator.MigrateFromYAML(configPath); err != nil {
		log.Fatal("配置迁移失败:", err)
	}

	log.Println("配置迁移成功完成！")
}