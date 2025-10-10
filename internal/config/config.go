package config

import (
	"fmt"
	"log"

	"github.com/spf13/viper"
)

// Config 应用配置结构
type Config struct {
	Server   ServerConfig   `mapstructure:"server"`
	Database DatabaseConfig `mapstructure:"database"`
	Git      GitConfig      `mapstructure:"git"`
	Aliyun   AliyunConfig   `mapstructure:"aliyun"`
	Log      LogConfig      `mapstructure:"log"`
	Sync     SyncConfig     `mapstructure:"sync"`
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
	Gitee         GiteeConfig `mapstructure:"gitee"`
	GitHub        GitHubConfig `mapstructure:"github"`
	LocalRepoPath string      `mapstructure:"local_repo_path"`
}

// GiteeConfig Gitee配置
type GiteeConfig struct {
	RepoURL  string `mapstructure:"repo_url"`
	Username string `mapstructure:"username"`
	Password string `mapstructure:"password"`
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
	TimeoutMinutes int `mapstructure:"timeout_minutes"`
}

var AppConfig *Config

// LoadConfig 加载配置文件
func LoadConfig(configPath string) error {
	viper.SetConfigFile(configPath)
	viper.SetConfigType("yaml")

	// 设置默认值
	setDefaults()

	if err := viper.ReadInConfig(); err != nil {
		return fmt.Errorf("读取配置文件失败: %w", err)
	}

	AppConfig = &Config{}
	if err := viper.Unmarshal(AppConfig); err != nil {
		return fmt.Errorf("解析配置文件失败: %w", err)
	}

	log.Printf("配置文件加载成功: %s", configPath)
	return nil
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