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

// MigrateFromConfig 按照优先级（环境变量 > config.yaml > 默认值）迁移配置到数据库
func (m *ConfigMigrator) MigrateFromConfig(configPath string) error {
	// 设置默认值
	m.setDefaults()

	// 设置环境变量映射
	m.setupEnvKeyMapping()

	// 启用自动环境变量读取
	viper.AutomaticEnv()

	// 读取配置文件（如果文件不存在，仅使用默认值和环境变量）
	viper.SetConfigFile(configPath)
	if err := viper.ReadInConfig(); err != nil {
		log.Printf("警告: 无法读取配置文件 %s: %v，将使用默认值和环境变量", configPath, err)
	} else {
		log.Printf("配置文件加载成功: %s", configPath)
	}

	log.Printf("配置加载完成，优先级: 环境变量 > 配置文件 > 默认值")

	// 迁移Git配置
	if err := m.migrateGitConfig(); err != nil {
		return fmt.Errorf("迁移Git配置失败: %v", err)
	}

	// 迁移阿里云配置
	if err := m.migrateAliyunConfig(); err != nil {
		return fmt.Errorf("迁移阿里云配置失败: %v", err)
	}

	// 迁移系统配置
	if err := m.migrateSystemConfig(); err != nil {
		return fmt.Errorf("迁移系统配置失败: %v", err)
	}

	return nil
}

// setDefaults 设置默认配置值
func (m *ConfigMigrator) setDefaults() {
	viper.SetDefault("server.port", "8080")
	viper.SetDefault("server.mode", "debug")

	viper.SetDefault("database.host", "localhost")
	viper.SetDefault("database.port", 3306)
	viper.SetDefault("database.charset", "utf8mb4")
	viper.SetDefault("database.parse_time", true)
	viper.SetDefault("database.loc", "Local")

	viper.SetDefault("git.local_repo_path", "./temp/docker_image_pusher")

	viper.SetDefault("log.level", "info")
	viper.SetDefault("log.file_path", "./logs/app.log")
	viper.SetDefault("log.max_size", 100)
	viper.SetDefault("log.max_backups", 3)
	viper.SetDefault("log.max_age", 28)

	viper.SetDefault("sync.timeout_minutes", 30)
}

// setupEnvKeyMapping 设置环境变量键名映射
func (m *ConfigMigrator) setupEnvKeyMapping() {
	// 服务器配置
	viper.BindEnv("server.host", "SERVER_HOST")
	viper.BindEnv("server.port", "APP_PORT")
	viper.BindEnv("server.mode", "GIN_MODE")

	// 数据库配置
	viper.BindEnv("database.host", "DB_HOST")
	viper.BindEnv("database.port", "DB_PORT")
	viper.BindEnv("database.username", "DB_USERNAME")
	viper.BindEnv("database.password", "DB_PASSWORD")
	viper.BindEnv("database.database", "DB_DATABASE")
	viper.BindEnv("database.charset", "DB_CHARSET")
	viper.BindEnv("database.parse_time", "DB_PARSE_TIME")
	viper.BindEnv("database.loc", "DB_LOC")

	// Git配置 - Gitee
	viper.BindEnv("git.gitee.repo_url", "GITEE_REPO_URL")
	viper.BindEnv("git.gitee.username", "GITEE_USERNAME")
	viper.BindEnv("git.gitee.password", "GITEE_PASSWORD")
	viper.BindEnv("git.gitee.token", "GITEE_TOKEN")
	viper.BindEnv("git.gitee.email", "GITEE_EMAIL")

	// Git配置 - GitHub
	viper.BindEnv("git.github.repo_url", "GITHUB_REPO_URL")
	viper.BindEnv("git.github.username", "GITHUB_USERNAME")
	viper.BindEnv("git.github.token", "GITHUB_TOKEN")

	// Git本地路径
	viper.BindEnv("git.local_repo_path", "GIT_LOCAL_REPO_PATH")

	// 阿里云配置
	viper.BindEnv("aliyun.registry", "ALIYUN_REGISTRY")
	viper.BindEnv("aliyun.namespace", "ALIYUN_NAMESPACE")
	viper.BindEnv("aliyun.username", "ALIYUN_USERNAME")
	viper.BindEnv("aliyun.password", "ALIYUN_PASSWORD")

	// 日志配置
	viper.BindEnv("log.level", "LOG_LEVEL")
	viper.BindEnv("log.file_path", "LOG_FILE_PATH")
}

// migrateGitConfig 迁移Git配置
func (m *ConfigMigrator) migrateGitConfig() error {
	log.Println("开始迁移Git配置...")

	// Git仓库类型配置
	repoType := "gitee" // 默认使用gitee
	if viper.IsSet("git.github.repo_url") && viper.GetString("git.github.repo_url") != "" {
		repoType = "github"
	}

	if err := m.upsertConfig("git_repository_type", repoType, "git", "Git仓库类型选择（gitee/github），决定使用哪个Git平台进行镜像列表文件管理", false, 1); err != nil {
		return err
	}

	// Gitee配置
	if viper.IsSet("git.gitee") {
		configs := []struct {
			key         string
			value       string
			description string
			encrypted   bool
			order       int
		}{
			{"gitee_repo_url", viper.GetString("git.gitee.repo_url"), "Gitee仓库地址，用于存储和管理镜像列表文件（images.txt）", false, 2},
			{"gitee_username", viper.GetString("git.gitee.username"), "Gitee用户名，用于Git操作的身份认证", false, 3},
			{"gitee_password", viper.GetString("git.gitee.password"), "Gitee密码或访问令牌，用于Git推送操作的身份验证（加密存储）", true, 4},
			{"gitee_email", viper.GetString("git.gitee.email"), "Gitee邮箱地址，用于Git提交时的作者信息设置", false, 5},
		}

		for _, cfg := range configs {
			if err := m.upsertConfig(cfg.key, cfg.value, "git", cfg.description, cfg.encrypted, cfg.order); err != nil {
				return err
			}
		}
	}

	// GitHub配置
	if viper.IsSet("git.github") {
		configs := []struct {
			key         string
			value       string
			description string
			encrypted   bool
			order       int
		}{
			{"github_repo_url", viper.GetString("git.github.repo_url"), "GitHub仓库地址，用于存储和管理镜像列表文件（images.txt）", false, 6},
			{"github_username", viper.GetString("git.github.username"), "GitHub用户名，用于Git操作的身份认证", false, 7},
			{"github_token", viper.GetString("git.github.token"), "GitHub访问令牌（Personal Access Token），用于API调用和Git操作认证（加密存储）", true, 8},
		}

		for _, cfg := range configs {
			if err := m.upsertConfig(cfg.key, cfg.value, "git", cfg.description, cfg.encrypted, cfg.order); err != nil {
				return err
			}
		}
	}

	// Git本地路径配置
	if err := m.upsertConfig("git_local_repo_path", viper.GetString("git.local_repo_path"), "git", "Git仓库本地克隆路径，用于临时存储和操作Git仓库文件", false, 9); err != nil {
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
		key         string
		value       string
		description string
		encrypted   bool
		order       int
	}{
		{"aliyun_registry", viper.GetString("aliyun.registry"), "阿里云容器镜像服务（ACR）的注册表地址，如 registry.cn-hangzhou.aliyuncs.com", false, 1},
		{"aliyun_namespace", viper.GetString("aliyun.namespace"), "阿里云ACR命名空间，用于组织和管理镜像仓库", false, 2},
		{"aliyun_username", viper.GetString("aliyun.username"), "阿里云ACR访问用户名，用于镜像推送和拉取的身份认证", false, 3},
		{"aliyun_password", viper.GetString("aliyun.password"), "阿里云ACR访问密码，用于镜像推送和拉取的身份验证（加密存储）", true, 4},
	}

	for _, cfg := range configs {
		if err := m.upsertConfig(cfg.key, cfg.value, "aliyun", cfg.description, cfg.encrypted, cfg.order); err != nil {
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
			key         string
			value       string
			description string
			order       int
		}{
			{"server_port", fmt.Sprintf("%d", viper.GetInt("server.port")), "服务器监听端口号，默认为8080", 1},
			{"server_mode", viper.GetString("server.mode"), "服务器运行模式，可选值：debug、release、test", 2},
			{"server_host", viper.GetString("server.host"), "服务器绑定的主机地址，默认为0.0.0.0", 3},
		}

		for _, cfg := range configs {
			if err := m.upsertConfig(cfg.key, cfg.value, "server", cfg.description, false, cfg.order); err != nil {
				return err
			}
		}
	}

	// 同步配置
	if viper.IsSet("sync") {
		configs := []struct {
			key         string
			value       string
			description string
			order       int
		}{
			{"sync_timeout_minutes", fmt.Sprintf("%d", viper.GetInt("sync.timeout_minutes")), "镜像同步任务超时时间（分钟），超过此时间任务将被取消", 1},
			{"sync_max_concurrent_jobs", fmt.Sprintf("%d", viper.GetInt("sync.max_concurrent_jobs")), "最大并发同步任务数量，控制系统资源使用", 2},
			{"sync_max_retry_count", fmt.Sprintf("%d", viper.GetInt("sync.max_retry_count")), "同步失败时的最大重试次数", 3},
			{"sync_retry_interval_minutes", fmt.Sprintf("%d", viper.GetInt("sync.retry_interval_minutes")), "同步失败后重试的间隔时间（分钟）", 4},
		}

		for _, cfg := range configs {
			if err := m.upsertConfig(cfg.key, cfg.value, "sync", cfg.description, false, cfg.order); err != nil {
				return err
			}
		}
	}

	// GitHub Actions配置
	if viper.IsSet("github_actions") {
		configs := []struct {
			key         string
			value       string
			description string
			order       int
		}{
			{"github_actions_workflow_file", viper.GetString("github_actions.workflow_file"), "GitHub Actions工作流文件路径，用于自动化镜像同步", 1},
			{"github_actions_api_timeout_seconds", fmt.Sprintf("%d", viper.GetInt("github_actions.api_timeout_seconds")), "GitHub Actions API调用超时时间（秒）", 2},
			{"github_actions_status_check_interval_seconds", fmt.Sprintf("%d", viper.GetInt("github_actions.status_check_interval_seconds")), "GitHub Actions状态检查间隔时间（秒）", 3},
		}

		for _, cfg := range configs {
			if err := m.upsertConfig(cfg.key, cfg.value, "github_actions", cfg.description, false, cfg.order); err != nil {
				return err
			}
		}
	}

	log.Println("系统配置迁移完成")
	return nil
}

// upsertConfig 插入或更新配置项
func (m *ConfigMigrator) upsertConfig(key, value, group, description string, encrypted bool, order int) error {
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
			Description:  description,
			IsEncrypted:  encrypted,
			DisplayOrder: order,
		}
		if err := m.db.Create(&newConfig).Error; err != nil {
			return fmt.Errorf("创建配置失败 %s: %v", key, err)
		}
		log.Printf("创建配置: %s = %s (描述: %s)", key, value, description)
	} else if result.Error == nil {
		// 更新现有配置
		existingConfig.ConfigValue = finalValue
		existingConfig.ConfigGroup = group
		existingConfig.Description = description
		existingConfig.IsEncrypted = encrypted
		existingConfig.DisplayOrder = order
		if err := m.db.Save(&existingConfig).Error; err != nil {
			return fmt.Errorf("更新配置失败 %s: %v", key, err)
		}
		log.Printf("更新配置: %s = %s (描述: %s)", key, value, description)
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

	// 配置文件路径 - 支持命令行参数
	var configPath string
	if len(os.Args) > 1 {
		configPath = os.Args[1]
		// 如果是相对路径，则相对于项目根目录
		if !filepath.IsAbs(configPath) {
			configPath = filepath.Join(rootDir, configPath)
		}
	} else {
		configPath = filepath.Join(rootDir, "config.yaml")
	}

	if _, err := os.Stat(configPath); os.IsNotExist(err) {
		log.Fatal("配置文件不存在:", configPath)
	}

	log.Printf("使用配置文件: %s", configPath)

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
	if err := migrator.MigrateFromConfig(configPath); err != nil {
		log.Fatal("配置迁移失败:", err)
	}

	log.Println("配置迁移成功完成！")
}
