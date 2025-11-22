package services

import (
	"docker-image-sync-platform/internal/database"
	"docker-image-sync-platform/internal/logger"
	"docker-image-sync-platform/internal/models"
	"fmt"
	"sync"

	"github.com/sirupsen/logrus"
	"go.uber.org/zap"
)

// GitServiceFactory Git服务工厂
// 根据配置动态创建和管理Git服务实例
type GitServiceFactory struct {
	giteeService      *GitService          // Gitee服务实例
	githubService     *GitService          // GitHub服务实例（如果需要的话）
	optimizedService  *GitOptimizedService  // 优化后的Git服务实例
	gitFileService    GitFileService       // Git文件API服务实例
	gitHubAPIService  *GitHubService       // GitHub API服务实例（用于Actions监控）
	encryptionService *EncryptionService   // 加密服务，用于创建GitService时传入
	mutex             sync.RWMutex          // 读写锁，保护服务实例
	gitHubAPIMutex    sync.RWMutex          // GitHub API服务锁
	configCache       string                // 配置缓存，避免频繁查询数据库
	cacheMutex        sync.RWMutex          // 配置缓存锁
	useOptimized      bool                 // 是否使用优化服务
	useAPI            bool                 // 是否使用API模式
}

// NewGitServiceFactory 创建Git服务工厂
func NewGitServiceFactory(encryptionService *EncryptionService) *GitServiceFactory {
	factory := &GitServiceFactory{
		encryptionService: encryptionService,
		useOptimized:      true, // 默认使用优化服务
		useAPI:            true, // 默认使用API模式
	}

	// 创建服务实例
	factory.giteeService = NewGitService(encryptionService)
	factory.optimizedService = NewGitOptimizedService(encryptionService)

	return factory
}

// GetGitService 获取当前配置的Git服务
func (f *GitServiceFactory) GetGitService() (*GitService, error) {
	// 如果启用优化服务，使用优化版本
	if f.useOptimized {
		return f.getOptimizedGitService()
	}

	// 否则使用原有逻辑
	return f.getOriginalGitService()
}

// GetGitServiceInterface 获取统一的Git服务接口
// 返回实现GitServiceInterface接口的服务实例
func (f *GitServiceFactory) GetGitServiceInterface() (GitServiceInterface, error) {
	// 如果启用优化服务，返回优化服务
	if f.useOptimized {
		return f.GetOptimizedGitService()
	}

	// 否则返回原有服务
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
		if f.githubService == nil {
			f.githubService = NewGitService(f.encryptionService)
		}
		return f.githubService, nil
	default:
		return nil, fmt.Errorf("不支持的Git仓库类型: %s", repoType)
	}
}

// getOptimizedGitService 获取优化后的Git服务
func (f *GitServiceFactory) getOptimizedGitService() (*GitService, error) {
	// 获取当前配置的Git仓库类型
	repoType, err := f.getGitRepositoryType()
	if err != nil {
		return nil, fmt.Errorf("获取Git仓库配置失败: %v", err)
	}

	f.mutex.RLock()
	defer f.mutex.RUnlock()

	// 目前优化服务统一处理所有仓库类型
	if f.optimizedService == nil {
		f.optimizedService = NewGitOptimizedService(f.encryptionService)
	}

	// 记录使用优化服务的信息
	logger.Logger.Info("使用Git服务工厂选择合适的实现", zap.String("repo_type", repoType))

	// 根据仓库类型返回相应的Git服务实例
	// GitService和GitOptimizedService都实现了GitServiceInterface接口
	// 工厂模式确保了接口的兼容性和可扩展性

	switch repoType {
	case "gitee":
		if f.giteeService == nil {
			f.giteeService = NewGitService(f.encryptionService)
		}
		// 返回原有服务用于兼容，但实际优化操作将在 GitOptimizedService 中进行
		return f.giteeService, nil
	case "github":
		if f.githubService == nil {
			f.githubService = NewGitService(f.encryptionService)
		}
		// 返回原有服务用于兼容，但实际优化操作将在 GitOptimizedService 中进行
		return f.githubService, nil
	default:
		return nil, fmt.Errorf("不支持的Git仓库类型: %s", repoType)
	}
}

// getOriginalGitService 获取原有的Git服务
func (f *GitServiceFactory) getOriginalGitService() (*GitService, error) {
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
		if f.githubService == nil {
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

// GetOptimizedGitService 获取优化后的Git服务实例
func (f *GitServiceFactory) GetOptimizedGitService() (*GitOptimizedService, error) {
	f.mutex.RLock()
	defer f.mutex.RUnlock()

	if f.optimizedService == nil {
		f.optimizedService = NewGitOptimizedService(f.encryptionService)
	}

	return f.optimizedService, nil
}

// SetUseOptimized 设置是否使用优化服务
func (f *GitServiceFactory) SetUseOptimized(useOptimized bool) {
	f.mutex.Lock()
	defer f.mutex.Unlock()
	f.useOptimized = useOptimized

	logger.Logger.Info("Git服务模式已更新", zap.Bool("use_optimized", useOptimized))
}

// IsUsingOptimized 检查是否使用优化服务
func (f *GitServiceFactory) IsUsingOptimized() bool {
	f.mutex.RLock()
	defer f.mutex.RUnlock()
	return f.useOptimized
}

// GetGitFileService 获取Git文件API服务实例
func (f *GitServiceFactory) GetGitFileService() (GitFileService, error) {
	f.mutex.Lock()
	defer f.mutex.Unlock()

	// 如果API服务已存在，直接返回
	if f.gitFileService != nil {
		return f.gitFileService, nil
	}

	// 获取当前配置的Git仓库类型
	repoType, err := f.getGitRepositoryType()
	if err != nil {
		return nil, fmt.Errorf("获取Git仓库配置失败: %v", err)
	}

	logger.Logger.Info("创建Git文件API服务", zap.String("repo_type", repoType))

	// 根据仓库类型创建对应的API服务
	gitFileService, err := CreateGitFileService(repoType, f.encryptionService)
	if err != nil {
		return nil, fmt.Errorf("创建Git文件API服务失败: %w", err)
	}

	// 缓存服务实例
	f.gitFileService = gitFileService

	logger.Logger.Info("Git文件API服务创建成功", zap.String("repo_type", repoType))
	return f.gitFileService, nil
}

// SetUseAPI 设置是否使用API模式
func (f *GitServiceFactory) SetUseAPI(useAPI bool) {
	f.mutex.Lock()
	defer f.mutex.Unlock()
	f.useAPI = useAPI

	logger.Logger.Info("Git服务API模式已更新", zap.Bool("use_api", useAPI))
}

// IsUsingAPI 检查是否使用API模式
func (f *GitServiceFactory) IsUsingAPI() bool {
	f.mutex.RLock()
	defer f.mutex.RUnlock()
	return f.useAPI
}

// TestGitFileService 测试Git文件API服务连接
func (f *GitServiceFactory) TestGitFileService() error {
	gitFileService, err := f.GetGitFileService()
	if err != nil {
		return fmt.Errorf("获取Git文件API服务失败: %w", err)
	}

	return gitFileService.TestConnection()
}

// ClearGitFileServiceCache 清理Git文件API服务缓存
// 当配置更新时调用此方法，确保下次调用时重新创建服务实例
func (f *GitServiceFactory) ClearGitFileServiceCache() {
	f.mutex.Lock()
	defer f.mutex.Unlock()

	if f.gitFileService != nil {
		logger.Logger.Info("清理Git文件API服务缓存",
			zap.String("previous_service", fmt.Sprintf("%T", f.gitFileService)))
		f.gitFileService = nil
		logger.Logger.Info("Git文件API服务缓存已清理")
	}
}

// GetGitHubAPIService 获取GitHub API服务实例（用于Actions监控）
// 使用缓存机制，避免重复创建服务实例
func (f *GitServiceFactory) GetGitHubAPIService() *GitHubService {
	f.gitHubAPIMutex.RLock()
	if f.gitHubAPIService != nil {
		defer f.gitHubAPIMutex.RUnlock()
		return f.gitHubAPIService
	}
	f.gitHubAPIMutex.RUnlock()

	// 需要创建新的服务实例
	f.gitHubAPIMutex.Lock()
	defer f.gitHubAPIMutex.Unlock()

	// 双重检查，防止并发创建多个实例
	if f.gitHubAPIService == nil {
		// 创建临时的logrus logger实例
		tempLogger := logrus.New()
		tempLogger.SetLevel(logrus.InfoLevel)

		configService := NewConfigService(database.DB, f.encryptionService, tempLogger)
		f.gitHubAPIService = NewGitHubService(configService)
		logger.Logger.Info("GitHub API服务实例已创建",
			zap.String("service_type", fmt.Sprintf("%T", f.gitHubAPIService)))
	}

	return f.gitHubAPIService
}

// ClearGitHubAPIServiceCache 清理GitHub API服务缓存
// 当GitHub配置更新时调用此方法，确保下次调用时重新创建服务实例
func (f *GitServiceFactory) ClearGitHubAPIServiceCache() {
	f.gitHubAPIMutex.Lock()
	defer f.gitHubAPIMutex.Unlock()

	if f.gitHubAPIService != nil {
		logger.Logger.Info("清理GitHub API服务缓存",
			zap.String("previous_service", fmt.Sprintf("%T", f.gitHubAPIService)))
		f.gitHubAPIService = nil
		logger.Logger.Info("GitHub API服务缓存已清理")
	}
}