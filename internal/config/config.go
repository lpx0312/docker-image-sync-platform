// Package config 提供了应用程序的配置管理功能
//
// 本包负责：
// 1. 配置文件的加载和解析（支持YAML格式）
// 2. 配置结构的定义和验证
// 3. 默认配置值的设置和管理
// 4. 配置参数的类型安全访问
// 5. 数据库连接字符串的生成
//
// 技术特性：
// - Viper: 强大的配置管理库，支持多种格式和来源
// - Mapstructure: 结构化配置映射，支持标签驱动的字段映射
// - 类型安全: 强类型配置结构，编译时检查配置完整性
// - 默认值: 合理的默认配置，降低配置复杂度
//
// 配置文件结构：
// - server: 服务器相关配置（端口、运行模式）
// - database: 数据库连接配置（MySQL连接参数）
// - git: Git仓库配置（Gitee、GitHub集成）
// - aliyun: 阿里云镜像仓库配置
// - log: 日志系统配置（级别、文件、轮转）
// - sync: 同步任务配置（超时、重试等）
//
// 使用方式：
//
//	// 加载配置文件
//	if err := config.LoadConfig("config.yaml"); err != nil {
//	    log.Fatal(err)
//	}
//
//	// 访问配置
//	port := config.AppConfig.Server.Port
//	dsn := config.AppConfig.Database.GetDSN()
//
// Author: Docker Image Sync Platform Team
// Version: 1.0.0
package config

import (
	"fmt"
	"log"

	"github.com/spf13/viper"
)

// Config 应用程序主配置结构
//
// 包含了应用运行所需的所有配置项，通过mapstructure标签
// 与YAML配置文件中的字段进行映射。每个子配置都有独立的
// 结构体定义，便于模块化管理和类型安全访问。
//
// 配置层次：
// - 顶层配置：应用级别的配置组织
// - 模块配置：按功能模块划分的配置组
// - 参数配置：具体的配置参数和值
//
// 字段说明：
// - Server: HTTP服务器配置（端口、模式等）
// - Database: 数据库连接配置（MySQL参数）
// - Git: Git仓库集成配置（Gitee、GitHub）
// - Aliyun: 阿里云镜像仓库配置
// - Log: 日志系统配置（级别、文件管理）
// - Sync: 同步任务配置（超时、策略等）
type Config struct {
	Server   ServerConfig   `mapstructure:"server"`   // HTTP服务器配置
	Database DatabaseConfig `mapstructure:"database"` // 数据库连接配置
	Git      GitConfig      `mapstructure:"git"`      // Git仓库配置
	Aliyun   AliyunConfig   `mapstructure:"aliyun"`   // 阿里云配置
	Log      LogConfig      `mapstructure:"log"`      // 日志配置
	Sync     SyncConfig     `mapstructure:"sync"`     // 同步配置
}

// ServerConfig 服务器配置
type ServerConfig struct {
	Port string `mapstructure:"port"`
	Mode string `mapstructure:"mode"`
}

// DatabaseConfig 数据库配置
type DatabaseConfig struct {
	Host      string `mapstructure:"host"`
	Port      int    `mapstructure:"port"`
	Username  string `mapstructure:"username"`
	Password  string `mapstructure:"password"`
	Database  string `mapstructure:"database"`
	Charset   string `mapstructure:"charset"`
	ParseTime bool   `mapstructure:"parse_time"`
	Loc       string `mapstructure:"loc"`
}

// GitConfig Git配置
type GitConfig struct {
	Gitee         GiteeConfig  `mapstructure:"gitee"`
	GitHub        GitHubConfig `mapstructure:"github"`
	LocalRepoPath string       `mapstructure:"local_repo_path"`
}

// GiteeConfig Gitee配置
type GiteeConfig struct {
	RepoURL  string `mapstructure:"repo_url"`
	Username string `mapstructure:"username"`
	Password string `mapstructure:"password"`
	Token    string `mapstructure:"token"`    // 访问令牌，优先使用Token认证
	Email    string `mapstructure:"email"`
}

// GitHubConfig GitHub配置
type GitHubConfig struct {
	RepoURL  string `mapstructure:"repo_url"`
	Username string `mapstructure:"username"`
	Token    string `mapstructure:"token"`
}

// AliyunConfig 阿里云配置
type AliyunConfig struct {
	Registry  string `mapstructure:"registry"`
	Namespace string `mapstructure:"namespace"`
	Username  string `mapstructure:"username"`
	Password  string `mapstructure:"password"`
}

// LogConfig 日志配置
type LogConfig struct {
	Level      string `mapstructure:"level"`
	FilePath   string `mapstructure:"file_path"`
	MaxSize    int    `mapstructure:"max_size"`
	MaxBackups int    `mapstructure:"max_backups"`
	MaxAge     int    `mapstructure:"max_age"`
}

// SyncConfig 同步配置
type SyncConfig struct {
	TimeoutMinutes       int `mapstructure:"timeout_minutes"`        // 同步任务超时时间（分钟）
	MaxConcurrentJobs    int `mapstructure:"max_concurrent_jobs"`    // 最大并发同步任务数
	MaxRetryCount        int `mapstructure:"max_retry_count"`        // 失败重试次数
	RetryIntervalMinutes int `mapstructure:"retry_interval_minutes"` // 重试间隔时间（分钟）
}

var AppConfig *Config

// LoadConfig 加载配置文件
func LoadConfig(configPath string) error {
	viper.SetConfigFile(configPath)
	viper.SetConfigType("yaml")

	// 设置默认值
	setDefaults()

	// 启用环境变量支持
	viper.AutomaticEnv()
	
	// 设置环境变量前缀（可选）
	// viper.SetEnvPrefix("APP")
	
	// 设置环境变量键名映射
	setupEnvKeyMapping()

	// 读取配置文件（如果文件不存在，仅使用默认值和环境变量）
	if err := viper.ReadInConfig(); err != nil {
		log.Printf("警告: 无法读取配置文件 %s: %v，将使用默认值和环境变量", configPath, err)
	} else {
		log.Printf("配置文件加载成功: %s", configPath)
	}

	AppConfig = &Config{}
	if err := viper.Unmarshal(AppConfig); err != nil {
		return fmt.Errorf("解析配置失败: %w", err)
	}

	log.Printf("配置加载完成，优先级: 环境变量 > 配置文件 > 默认值")
	return nil
}

// setupEnvKeyMapping 设置环境变量键名映射
func setupEnvKeyMapping() {
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

// setDefaults 设置默认配置值
func setDefaults() {
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

// GetDSN 获取数据库连接字符串
func (c *DatabaseConfig) GetDSN() string {
	return fmt.Sprintf("%s:%s@tcp(%s:%d)/%s?charset=%s&parseTime=%t&loc=%s",
		c.Username, c.Password, c.Host, c.Port, c.Database, c.Charset, c.ParseTime, c.Loc)
}
