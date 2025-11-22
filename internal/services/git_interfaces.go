package services

import "context"

// GitServiceInterface 统一的Git服务接口
// 定义了所有Git服务实现必须遵循的契约
type GitServiceInterface interface {
	// 核心方法
	UpdateImagesFile(ctx context.Context, newImages []string) (string, error)
	PullLatest(ctx context.Context) error
	GetRepoStatus(ctx context.Context) (map[string]interface{}, error)
	CleanRepository(ctx context.Context) error

	// 测试方法（可选实现）
	TestConnection() error
}

// GitOptimizedServiceInterface 优化Git服务接口
// 扩展基础接口，添加优化功能
type GitOptimizedServiceInterface interface {
	GitServiceInterface

	// 优化功能
	UpdateImagesFileOptimized(ctx context.Context, newImages []string) (string, error)
	GetPerformanceMetrics() map[string]interface{}

	// 测试方法
	PullImagesFileForTesting() (string, error)
	UpdateImagesFileForTesting(newImages []string, description string) (string, error)
}

// GitFileServiceInterface Git文件API服务接口
// 用于基于API的Git操作
type GitFileServiceInterface interface {
	ReadImagesFile() (string, error)
	UpdateImagesFile(content, commitMessage string) (string, error)
	TestConnection() error
}