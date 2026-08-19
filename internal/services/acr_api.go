package services

import (
	"fmt"
	"strings"
	"sync"
	"time"

	"github.com/go-resty/resty/v2"
)

// AcrAPIService 阿里云 ACR 数据面 API 客户端
//
// 认证流程：Basic 凭证换取 dockerauth.{region}.aliyuncs.com 的 Bearer token，
// 之后所有 /v2 请求携带 Authorization: Bearer <token>。
type AcrAPIService struct {
	client     *resty.Client
	tokenCache map[string]*tokenCacheItem
	tokenMu    sync.Mutex
}

var _ RegistryAPIClient = (*AcrAPIService)(nil)

// NewAcrAPIService 创建ACR API服务实例
func NewAcrAPIService() *AcrAPIService {
	return &AcrAPIService{
		client:     resty.New().SetTimeout(30 * time.Second),
		tokenCache: make(map[string]*tokenCacheItem),
	}
}

// getRegionFromRegistry 从 registry 地址推断 region
func getRegionFromRegistry(registry string) string {
	// registry.cn-hangzhou.aliyuncs.com -> cn-hangzhou
	// crpi-xxx.cn-hangzhou.personal.cr.aliyuncs.com -> cn-hangzhou
	parts := strings.Split(registry, ".")

	// 查找包含 "cn-" 或 "us-" 等 region 格式的 part
	for _, part := range parts {
		if strings.HasPrefix(part, "cn-") || strings.HasPrefix(part, "us-") ||
		   strings.HasPrefix(part, "eu-") || strings.HasPrefix(part, "ap-") {
			return part
		}
	}

	// 如果没找到，返回默认值
	return "cn-hangzhou"
}

// getDefaultAuthServer 从 registry 推断默认认证服务器地址
func getDefaultAuthServer(registry string) string {
	region := getRegionFromRegistry(registry)
	return fmt.Sprintf("dockerauth.%s.aliyuncs.com", region)
}

func tokenCacheKey(registry, namespace, repo string) string {
	return fmt.Sprintf("%s:%s/%s", registry, namespace, repo)
}

func (s *AcrAPIService) invalidateTokenCache(registry, namespace, repo string) {
	s.tokenMu.Lock()
	defer s.tokenMu.Unlock()
	delete(s.tokenCache, tokenCacheKey(registry, namespace, repo))
}

// getTokenWithScope 按指定 scope 获取 ACR 认证 Token（repository:{ns}/{repo}:pull 或 registry:catalog:*）
func (s *AcrAPIService) getTokenWithScope(registry, username, password, scope, authServer, dockerService, cacheKey string) (string, error) {
	s.tokenMu.Lock()
	if item, ok := s.tokenCache[cacheKey]; ok && time.Now().Before(item.expiresAt) {
		token := item.token
		s.tokenMu.Unlock()
		return token, nil
	}
	s.tokenMu.Unlock()

	// 使用配置的 auth_server，为空时自动推断
	if authServer == "" {
		authServer = getDefaultAuthServer(registry)
	}
	// 使用配置的 docker_service，为空时使用默认值
	if dockerService == "" {
		dockerService = "registry.aliyuncs.com:cn-hangzhou:26842"
	}

	var result struct {
		Token     string `json:"token"`
		ExpiresIn int    `json:"expires_in"`
	}

	resp, err := s.client.R().
		SetBasicAuth(username, password).
		SetFormData(map[string]string{
			"service": dockerService,
			"scope":   scope,
		}).
		SetResult(&result).
		Post(fmt.Sprintf("https://%s/auth", authServer))

	if err != nil {
		return "", fmt.Errorf("获取Token失败: %w", err)
	}

	if resp.StatusCode() != 200 {
		return "", fmt.Errorf("获取Token失败: HTTP %d", resp.StatusCode())
	}

	if result.Token == "" {
		return "", fmt.Errorf("获取Token失败: 返回空Token")
	}

	expiresIn := result.ExpiresIn
	if expiresIn <= 0 {
		expiresIn = 300
	}
	// 提前 60 秒过期，避免边界时刻 401
	cacheTTL := time.Duration(expiresIn-60) * time.Second
	if cacheTTL < time.Minute {
		cacheTTL = time.Minute
	}

	s.tokenMu.Lock()
	s.tokenCache[cacheKey] = &tokenCacheItem{
		token:     result.Token,
		expiresAt: time.Now().Add(cacheTTL),
	}
	s.tokenMu.Unlock()

	return result.Token, nil
}

// GetToken 获取 ACR 认证 Token
func (s *AcrAPIService) GetToken(registry, username, password, namespace, repo, authServer, dockerService string) (string, error) {
	cacheKey := tokenCacheKey(registry, namespace, repo)
	scope := fmt.Sprintf("repository:%s/%s:pull", namespace, repo)
	return s.getTokenWithScope(registry, username, password, scope, authServer, dockerService, cacheKey)
}

// GetTags 获取镜像的 Tag 列表
func (s *AcrAPIService) GetTags(registry, username, password, namespace, repo, authServer, dockerService string) ([]string, error) {
	for attempt := 0; attempt < 2; attempt++ {
		token, err := s.GetToken(registry, username, password, namespace, repo, authServer, dockerService)
		if err != nil {
			return nil, err
		}

		var result struct {
			Name string   `json:"name"`
			Tags []string `json:"tags"`
		}

		resp, err := s.client.R().
			SetAuthToken(token).
			SetHeader("Accept", "application/vnd.docker.distribution.manifest.v2+json").
			SetResult(&result).
			Get(fmt.Sprintf("https://%s/v2/%s/%s/tags/list", registry, namespace, repo))

		if err != nil {
			return nil, fmt.Errorf("获取Tag列表失败: %w", err)
		}

		if resp.StatusCode() == 401 && attempt == 0 {
			s.invalidateTokenCache(registry, namespace, repo)
			continue
		}

		if resp.StatusCode() == 404 {
			return []string{}, nil
		}

		if resp.StatusCode() != 200 {
			return nil, fmt.Errorf("获取Tag列表失败: HTTP %d", resp.StatusCode())
		}

		return result.Tags, nil
	}

	return nil, fmt.Errorf("获取Tag列表失败: HTTP 401")
}

// RepositoryExists 检查仓库中是否存在指定镜像（有至少一个 Tag）
// 通过 tags/list 判断：404 或空 Tag 列表视为不存在
func (s *AcrAPIService) RepositoryExists(registry, username, password, namespace, repo, authServer, dockerService string) (bool, error) {
	tags, err := s.GetTags(registry, username, password, namespace, repo, authServer, dockerService)
	if err != nil {
		return false, err
	}
	return len(tags) > 0, nil
}

// IsRepositoryNotFound 判断 tags/list 错误是否表示仓库不存在。
// 阿里云 ACR 对不存在的仓库可能返回 404 或 401，而非标准 404。
func (s *AcrAPIService) IsRepositoryNotFound(err error) bool {
	if err == nil {
		return false
	}
	msg := err.Error()
	return strings.Contains(msg, "HTTP 404") || strings.Contains(msg, "HTTP 401")
}

func (s *AcrAPIService) fetchManifest(registry, namespace, repo, tag string, username, password, authServer, dockerService string) ([]byte, string, string, int, error) {
	return fetchManifestWithRetry(s.client, registry, namespace, repo, tag, func(forceRefresh bool) (string, error) {
		if forceRefresh {
			s.invalidateTokenCache(registry, namespace, repo)
		}
		return s.GetToken(registry, username, password, namespace, repo, authServer, dockerService)
	})
}

// GetTagDetail 获取 Tag 详细信息
func (s *AcrAPIService) GetTagDetail(registry, username, password, namespace, repo, tag, authServer, dockerService string) (*TagDetail, error) {
	token, err := s.GetToken(registry, username, password, namespace, repo, authServer, dockerService)
	if err != nil {
		return nil, err
	}

	body, contentDigest, manifestLastModified, _, err := s.fetchManifest(registry, namespace, repo, tag, username, password, authServer, dockerService)
	if err != nil {
		return nil, err
	}

	return parseManifestToTagDetail(s.client, tag, registry, namespace, repo, token, body, contentDigest, manifestLastModified)
}

// GetTagNames 获取镜像 Tag 名称列表
func (s *AcrAPIService) GetTagNames(registry, username, password, namespace, repo, authServer, dockerService string) ([]string, error) {
	return s.GetTags(registry, username, password, namespace, repo, authServer, dockerService)
}

// GetTagsDetailsBatch 批量获取指定 Tag 的详细信息，带并发控制和限流重试
func (s *AcrAPIService) GetTagsDetailsBatch(registry, username, password, namespace, repo, authServer, dockerService string, tagNames []string) ([]*TagDetail, error) {
	if len(tagNames) == 0 {
		return []*TagDetail{}, nil
	}
	if len(tagNames) > tagDetailsBatchMaxSize {
		return nil, fmt.Errorf("单次最多查询 %d 个 Tag", tagDetailsBatchMaxSize)
	}

	var tokenMu sync.Mutex
	currentToken, err := s.GetToken(registry, username, password, namespace, repo, authServer, dockerService)
	if err != nil {
		return nil, err
	}

	getToken := func(forceRefresh bool) (string, error) {
		tokenMu.Lock()
		defer tokenMu.Unlock()

		if forceRefresh {
			s.invalidateTokenCache(registry, namespace, repo)
			refreshed, err := s.GetToken(registry, username, password, namespace, repo, authServer, dockerService)
			if err != nil {
				return "", err
			}
			currentToken = refreshed
			return currentToken, nil
		}

		return currentToken, nil
	}

	details := make([]*TagDetail, len(tagNames))
	sem := make(chan struct{}, tagDetailConcurrency)
	var wg sync.WaitGroup

	for i, tagName := range tagNames {
		wg.Add(1)
		go func(index int, tag string) {
			defer wg.Done()
			sem <- struct{}{}
			defer func() { <-sem }()

			body, contentDigest, manifestLastModified, _, err := fetchManifestWithRetry(s.client, registry, namespace, repo, tag, getToken)
			if err != nil {
				details[index] = emptyTagDetail(tag)
				return
			}

			token, _ := getToken(false)
			detail, err := parseManifestToTagDetail(s.client, tag, registry, namespace, repo, token, body, contentDigest, manifestLastModified)
			if err != nil {
				details[index] = emptyTagDetail(tag)
				return
			}

			details[index] = detail
		}(i, tagName)
	}

	wg.Wait()
	return details, nil
}

// GetTagsWithDetails 获取 Tag 列表及其详细信息，带并发控制和限流重试
func (s *AcrAPIService) GetTagsWithDetails(registry, username, password, namespace, repo, authServer, dockerService string) ([]*TagDetail, error) {
	tagNames, err := s.GetTagNames(registry, username, password, namespace, repo, authServer, dockerService)
	if err != nil {
		return nil, err
	}

	return s.GetTagsDetailsBatch(registry, username, password, namespace, repo, authServer, dockerService, tagNames)
}

// ListRepositories 通过 /v2/_catalog 列出远程仓库中的全部镜像仓库名（从仓库导入用）
func (s *AcrAPIService) ListRepositories(registry, username, password, accessKey, secretKey, namespace, authServer, dockerService string) ([]string, error) {
	cacheKey := fmt.Sprintf("%s:_catalog", registry)
	scope := "registry:catalog:*"

	repositories := make([]string, 0, 64)
	last := ""
	for {
		var result struct {
			Repositories []string `json:"repositories"`
		}

		reqURL := fmt.Sprintf("https://%s/v2/_catalog?n=1000", registry)
		if last != "" {
			reqURL += "&last=" + last
		}

		token, err := s.getTokenWithScope(registry, username, password, scope, authServer, dockerService, cacheKey)
		if err != nil {
			return nil, fmt.Errorf("获取Catalog Token失败: %w", err)
		}

		resp, err := s.client.R().
			SetAuthToken(token).
			SetResult(&result).
			Get(reqURL)
		if err != nil {
			return nil, fmt.Errorf("获取仓库列表失败: %w", err)
		}
		if resp.StatusCode() != 200 {
			return nil, fmt.Errorf("获取仓库列表失败: HTTP %d", resp.StatusCode())
		}

		for _, repo := range result.Repositories {
			// catalog 返回形如 namespace/repo 的全路径，过滤出本命名空间
			if namespace != "" && strings.HasPrefix(repo, namespace+"/") {
				repositories = append(repositories, strings.TrimPrefix(repo, namespace+"/"))
			}
		}

		if len(result.Repositories) < 1000 {
			break
		}
		last = result.Repositories[len(result.Repositories)-1]
	}

	return repositories, nil
}

// TestConnection 测试 ACR 配置连通性（地址 + 账号密码）
func (s *AcrAPIService) TestConnection(registry, username, password, accessKey, secretKey, namespace, authServer, dockerService string) *RegistryTestResult {
	result := &RegistryTestResult{RegistryType: "acr"}

	// ACR 对不存在的仓库 tags/list 可能返回 401，不能用它做连通性验证；
	// 只做换 token（dockerauth Basic 认证），成功即代表地址/账号密码正确
	if _, err := s.GetToken(registry, username, password, namespace, "connection-test", authServer, dockerService); err != nil {
		result.LoginMessage = err.Error()
	} else {
		result.LoginOK = true
		result.LoginMessage = "凭证可用（仓库地址与账号密码验证通过）"
	}
	return result
}
