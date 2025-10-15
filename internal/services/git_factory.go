package services

import (
	"docker-image-sync-platform/internal/database"
	"docker-image-sync-platform/internal/models"
	"fmt"
	"sync"
)

// GitServiceFactory Git服务工厂
// 根据配置动态创建和管理Git服务实例
type GitServiceFactory struct {
	giteeService      *GitService        // Gitee服务实例
	githubService     *GitService        // GitHub服务实例（如果需要的话）
	encryptionService *EncryptionService // 加密服务，用于创建GitService时传入
	mutex             sync.RWMutex       // 读写锁，保护服务实例
	configCache       string             // 配置缓存，避免频繁查询数据库
	cacheMutex        sync.RWMutex       // 配置缓存锁
}

// NewGitServiceFactory 创建Git服务工厂
func NewGitServiceFactory(encryptionService *EncryptionService) *GitServiceFactory {
	return &GitServiceFactory{
		encryptionService: encryptionService,
		giteeService:      NewGitService(encryptionService), // 默认创建Gitee服务
	}
}

// GetGitService 获取当前配置的Git服务
func (f *GitServiceFactory) GetGitService() (*GitService, error) {
	// 获取当前配置的Git仓库类型
	repoType, err := f.getGitRepositoryType()
	if err != nil {
		return nil, fmt.Errorf("获取Git仓库配置失败: %v", err)
	}

	f.mutex.RLock()
	defer f.mutex.RUnlock()

	switch repoType {
	case "gitee":
		if f.giteeService == nil {
			f.giteeService = NewGitService(f.encryptionService)
		}
		return f.giteeService, nil
	case "github":
		// 注意：这里返回的仍然是GitService，因为当前的GitService
		// 主要用于Git操作，GitHub的API操作由GitHubService处理
		// 如果需要支持GitHub作为Git仓库，需要扩展GitService
		// 或者创建新的GitHubGitService
		if f.githubService == nil {
			// 这里可以创建专门的GitHub Git服务
			// 目前先返回默认的GitService
			f.githubService = NewGitService(f.encryptionService)
		}
		return f.githubService, nil
	default:
		return nil, fmt.Errorf("不支持的Git仓库类型: %s", repoType)
	}
}

// getGitRepositoryType 获取Git仓库类型配置
func (f *GitServiceFactory) getGitRepositoryType() (string, error) {
	// 先检查缓存
	f.cacheMutex.RLock()
	if f.configCache != "" {
		cached := f.configCache
		f.cacheMutex.RUnlock()
		return cached, nil
	}
	f.cacheMutex.RUnlock()

	// 从数据库查询配置
	var systemConfig models.SystemConfig
	err := database.DB.Where("config_key = ?", "git_repository_type").First(&systemConfig).Error
	if err != nil {
		// 如果配置不存在，返回默认值
		return "gitee", nil
	}

	// 更新缓存
	f.cacheMutex.Lock()
	f.configCache = systemConfig.ConfigValue
	f.cacheMutex.Unlock()

	return systemConfig.ConfigValue, nil
}

// RefreshConfig 刷新配置缓存
// 当配置更新时调用此方法
func (f *GitServiceFactory) RefreshConfig() {
	f.cacheMutex.Lock()
	f.configCache = ""
	f.cacheMutex.Unlock()
}

// GetGitRepositoryConfig 获取Git仓库配置（用于API）
func (f *GitServiceFactory) GetGitRepositoryConfig() (string, error) {
	return f.getGitRepositoryType()
}

// UpdateGitRepositoryConfig 更新Git仓库配置（用于API）
func (f *GitServiceFactory) UpdateGitRepositoryConfig(repoType string) error {
	// 验证配置值
	if repoType != "gitee" && repoType != "github" {
		return fmt.Errorf("无效的Git仓库类型: %s，只支持 'gitee' 或 'github'", repoType)
	}

	// 更新数据库配置
	var systemConfig models.SystemConfig
	err := database.DB.Where("config_key = ?", "git_repository_type").First(&systemConfig).Error
	if err != nil {
		// 如果配置不存在，创建新配置
		systemConfig = models.SystemConfig{
			ConfigKey:   "git_repository_type",
			ConfigValue: repoType,
			Description: "Git仓库类型选择（gitee或github）",
		}
		err = database.DB.Create(&systemConfig).Error
	} else {
		// 更新现有配置
		systemConfig.ConfigValue = repoType
		err = database.DB.Save(&systemConfig).Error
	}

	if err != nil {
		return fmt.Errorf("更新Git仓库配置失败: %v", err)
	}

	// 刷新缓存
	f.RefreshConfig()

	return nil
}