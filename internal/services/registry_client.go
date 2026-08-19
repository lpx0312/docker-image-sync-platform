package services

import (
	"strings"
	"sync"

	"docker-image-sync-platform/internal/models"
)

// RegistryAPIClient 镜像仓库数据面 API 客户端统一接口
//
// ACR 与 SWR 均实现该接口，镜像管理（Tag 浏览/仓库存在性校验）与
// 同步链路（成功登记/校验）通过 NewRegistryAPIService 按仓库类型分发。
type RegistryAPIClient interface {
	// GetTags 获取镜像的所有 Tag 名称
	GetTags(registry, username, password, namespace, repo, authServer, dockerService string) ([]string, error)
	// GetTagNames GetTags 的别名（保持与 AcrAPIService 现有签名一致）
	GetTagNames(registry, username, password, namespace, repo, authServer, dockerService string) ([]string, error)
	// GetTagDetail 获取单个 Tag 详情（架构/digest/大小/推送时间）
	GetTagDetail(registry, username, password, namespace, repo, tag, authServer, dockerService string) (*TagDetail, error)
	// GetTagsDetailsBatch 批量获取 Tag 详情
	GetTagsDetailsBatch(registry, username, password, namespace, repo, authServer, dockerService string, tagNames []string) ([]*TagDetail, error)
	// GetTagsWithDetails 获取全部 Tag 及详情
	GetTagsWithDetails(registry, username, password, namespace, repo, authServer, dockerService string) ([]*TagDetail, error)
	// RepositoryExists 仓库是否存在（有至少一个 Tag）
	RepositoryExists(registry, username, password, namespace, repo, authServer, dockerService string) (bool, error)
	// IsRepositoryNotFound 错误是否表示仓库不存在
	IsRepositoryNotFound(err error) bool
	// ListRepositories 列出远程仓库中的全部镜像仓库名（"从仓库导入"用）；
	// SWR 走管理面 API（需永久 IAM AK/SK），ACR 走 /v2/_catalog
	ListRepositories(registry, username, password, namespace, authServer, dockerService string) ([]string, error)
}

var (
	registryClientInstancesMu sync.Mutex
	acrClientInstance         RegistryAPIClient
	swrClientInstance         RegistryAPIClient
)

// IsSWRRegistry 通过地址判断是否为华为云 SWR
func IsSWRRegistry(registry string) bool {
	return strings.Contains(strings.ToLower(registry), "myhuaweicloud.com")
}

// NewRegistryAPIService 按仓库类型返回对应的 API 客户端（进程内单例，保留 token 缓存）。
// registryURL 兜底识别：地址含 myhuaweicloud.com 时即使类型缺失也按 SWR 处理。
func NewRegistryAPIService(registryType, registryURL string) RegistryAPIClient {
	if registryType == models.RegistryTypeSWR || IsSWRRegistry(registryURL) {
		registryClientInstancesMu.Lock()
		defer registryClientInstancesMu.Unlock()
		if swrClientInstance == nil {
			swrClientInstance = NewSwrAPIService()
		}
		return swrClientInstance
	}

	registryClientInstancesMu.Lock()
	defer registryClientInstancesMu.Unlock()
	if acrClientInstance == nil {
		acrClientInstance = NewAcrAPIService()
	}
	return acrClientInstance
}
