package main

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
)

// Config CLI 本地配置，持久化在 ~/.config/dsync/config.json（权限 0600）。
// 保存登录凭据与 token，token 过期后由 client 自动用存量密码重登。
type Config struct {
	Server         string `json:"server"`                     // 平台地址，如 http://localhost:8080
	Username       string `json:"username,omitempty"`         // 登录用户名
	Password       string `json:"password,omitempty"`         // 登录密码（明文，依赖文件权限保护）
	Token          string `json:"token,omitempty"`            // JWT token
	TokenExpiresAt string `json:"token_expires_at,omitempty"` // token 过期时间（RFC3339）
}

// configDirOverride 测试中覆盖配置目录，避免污染真实 ~/.config/dsync
var configDirOverride string

// configDir 返回 CLI 配置目录（~/.config/dsync），不存在则创建。
func configDir() (string, error) {
	if configDirOverride != "" {
		return configDirOverride, nil
	}
	base, err := os.UserConfigDir()
	if err != nil {
		return "", fmt.Errorf("无法确定配置目录: %w", err)
	}
	dir := filepath.Join(base, "dsync")
	if err := os.MkdirAll(dir, 0o700); err != nil {
		return "", fmt.Errorf("创建配置目录失败: %w", err)
	}
	return dir, nil
}

// configFilePath 返回配置文件路径，可通过 --config 覆盖。
func configFilePath() string {
	if cfgPath != "" {
		return cfgPath
	}
	dir, err := configDir()
	if err != nil {
		return filepath.Join("dsync-config.json")
	}
	return filepath.Join(dir, "config.json")
}

// loadConfig 读取配置文件；文件不存在时返回空配置而非错误。
func loadConfig() (*Config, error) {
	cfg := &Config{}
	data, err := os.ReadFile(configFilePath())
	if err != nil {
		if os.IsNotExist(err) {
			return cfg, nil
		}
		return nil, fmt.Errorf("读取配置文件失败: %w", err)
	}
	if err := json.Unmarshal(data, cfg); err != nil {
		return nil, fmt.Errorf("解析配置文件失败: %w", err)
	}
	return cfg, nil
}

// saveConfig 以 0600 权限写入配置文件，避免凭据泄露。
func saveConfig(cfg *Config) error {
	data, err := json.MarshalIndent(cfg, "", "  ")
	if err != nil {
		return fmt.Errorf("序列化配置失败: %w", err)
	}
	path := configFilePath()
	if err := os.WriteFile(path, data, 0o600); err != nil {
		return fmt.Errorf("写入配置文件失败: %w", err)
	}
	return nil
}

// clearAuth 清除本地凭据（登出用），保留服务器地址。
func clearAuth(cfg *Config) {
	cfg.Username = ""
	cfg.Password = ""
	cfg.Token = ""
	cfg.TokenExpiresAt = ""
}
