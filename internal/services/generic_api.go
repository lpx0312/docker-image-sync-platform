package services

import (
	"encoding/json"
	"fmt"
	"strings"
	"sync"
	"time"

	"github.com/go-resty/resty/v2"
)

// GenericAPIService 通用 OCI/Docker Registry v2 协议客户端，
// 覆盖 Harbor、自建 Registry 等标准实现（腾讯云 CCR 的数据面读取也走此协议）。
//
// 认证策略（与 docker/skopeo 客户端行为一致）：
//  1. 携带凭证的请求优先 Basic 直连（Harbor 等直接接受）
//  2. 收到 401 且响应头为 Bearer challenge 时，自动解析 realm/service，
//     用 Basic 凭证换取 token 后以 Bearer 重试（Docker Hub 风格）
//
// 限制：要求仓库地址具备公网有效的 TLS 证书（自签名证书的 Harbor
// 需自行替换证书后使用）。
type GenericAPIService struct {
	client *resty.Client

	tokenMu    sync.Mutex
	tokenCache map[string]*tokenCacheItem

	authMu    sync.Mutex
	authCache map[string]*genericAuthInfo
}

var _ RegistryAPIClient = (*GenericAPIService)(nil)

type genericAuthInfo struct {
	Realm   string
	Service string
}

// NewGenericAPIService 创建通用 Registry API 客户端实例
func NewGenericAPIService() *GenericAPIService {
	return &GenericAPIService{
		client:     resty.New().SetTimeout(30 * time.Second),
		tokenCache: make(map[string]*tokenCacheItem),
		authCache:  make(map[string]*genericAuthInfo),
	}
}

// discoverAuth 探测 /v2/ 的认证 challenge（Bearer realm），按 registry 缓存
func (s *GenericAPIService) discoverAuth(registry, username, password string) *genericAuthInfo {
	s.authMu.Lock()
	if info, ok := s.authCache[registry]; ok {
		s.authMu.Unlock()
		return info
	}
	s.authMu.Unlock()

	info := &genericAuthInfo{}
	resp, err := s.client.R().Get(fmt.Sprintf("https://%s/v2/", registry))
	if err == nil && resp.StatusCode() == 401 {
		if parsed := parseBearerChallenge(resp.Header().Get("WWW-Authenticate")); parsed != nil {
			info = &genericAuthInfo{Realm: parsed.Realm, Service: parsed.Service}
		}
	}

	s.authMu.Lock()
	s.authCache[registry] = info
	s.authMu.Unlock()
	return info
}

func genericTokenCacheKey(registry, scope string) string {
	return registry + "|" + scope
}

func (s *GenericAPIService) invalidateTokenCache(registry, scope string) {
	s.tokenMu.Lock()
	defer s.tokenMu.Unlock()
	delete(s.tokenCache, genericTokenCacheKey(registry, scope))
}

// getToken 按 scope 获取 Bearer token（Basic 换取；匿名场景请求公共仓库）
func (s *GenericAPIService) getToken(registry, username, password, namespace, repo, scope string) (string, error) {
	cacheKey := genericTokenCacheKey(registry, scope)

	s.tokenMu.Lock()
	if item, ok := s.tokenCache[cacheKey]; ok && time.Now().Before(item.expiresAt) {
		token := item.token
		s.tokenMu.Unlock()
		return token, nil
	}
	s.tokenMu.Unlock()

	auth := s.discoverAuth(registry, username, password)
	if auth.Realm == "" {
		return "", fmt.Errorf("仓库 %s 未返回 Bearer 认证信息（可能不支持 token 认证或地址不可达）", registry)
	}

	var result struct {
		Token     string `json:"token"`
		ExpiresIn int    `json:"expires_in"`
	}
	req := s.client.R().SetQueryParams(map[string]string{
		"service": auth.Service,
		"scope":   scope,
	}).SetResult(&result)
	if username != "" {
		req = req.SetBasicAuth(username, password)
	}

	resp, err := req.Get(auth.Realm)
	if err != nil {
		return "", fmt.Errorf("获取Token失败: %w", err)
	}
	if resp.StatusCode() == 401 {
		return "", fmt.Errorf("获取Token失败: 认证被拒绝（用户名或密码错误）")
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
	cacheTTL := time.Duration(expiresIn-60) * time.Second
	if cacheTTL < time.Minute {
		cacheTTL = time.Minute
	}

	s.tokenMu.Lock()
	s.tokenCache[cacheKey] = &tokenCacheItem{token: result.Token, expiresAt: time.Now().Add(cacheTTL)}
	s.tokenMu.Unlock()
	return result.Token, nil
}

// doV2Request 执行 v2 数据面请求：Basic 直连优先，401 Bearer challenge 时换 token 重试
func (s *GenericAPIService) doV2Request(registry, username, password, namespace, repo, method, path, accept string) (*resty.Response, error) {
	scope := fmt.Sprintf("repository:%s/%s:pull", namespace, repo)

	buildReq := func() *resty.Request {
		req := s.client.R().SetHeader("Accept", accept)
		if username != "" {
			req = req.SetBasicAuth(username, password)
		}
		return req
	}

	resp, err := buildReq().Execute(method, fmt.Sprintf("https://%s%s", registry, path))
	if err != nil {
		return nil, err
	}

	// Basic 直连被拒且返回 Bearer challenge → 走 token 流程
	if resp.StatusCode() == 401 && parseBearerChallenge(resp.Header().Get("WWW-Authenticate")) != nil {
		token, tokenErr := s.getToken(registry, username, password, namespace, repo, scope)
		if tokenErr != nil {
			return resp, nil
		}
		resp, err = buildReq().SetAuthToken(token).Execute(method, fmt.Sprintf("https://%s%s", registry, path))
		if err != nil {
			return nil, err
		}
	}

	return resp, nil
}

// GetTags 获取镜像的所有 Tag 名称
func (s *GenericAPIService) GetTags(registry, username, password, namespace, repo, authServer, dockerService string) ([]string, error) {
	resp, err := s.doV2Request(registry, username, password, namespace, repo, "GET",
		fmt.Sprintf("/v2/%s/%s/tags/list", namespace, repo),
		"application/vnd.docker.distribution.manifest.v2+json")
	if err != nil {
		return nil, fmt.Errorf("获取Tag列表失败: %w", err)
	}
	if resp.StatusCode() == 404 {
		return []string{}, nil
	}
	if resp.StatusCode() == 401 {
		return nil, fmt.Errorf("获取Tag列表失败: 认证被拒绝（HTTP 401）")
	}
	if resp.StatusCode() != 200 {
		return nil, fmt.Errorf("获取Tag列表失败: HTTP %d", resp.StatusCode())
	}

	var result struct {
		Name string   `json:"name"`
		Tags []string `json:"tags"`
	}
	if err := json.Unmarshal(resp.Body(), &result); err != nil {
		return nil, fmt.Errorf("解析Tag列表失败: %w", err)
	}
	if result.Tags == nil {
		return []string{}, nil
	}
	return result.Tags, nil
}

// GetTagNames 获取镜像 Tag 名称列表
func (s *GenericAPIService) GetTagNames(registry, username, password, namespace, repo, authServer, dockerService string) ([]string, error) {
	return s.GetTags(registry, username, password, namespace, repo, authServer, dockerService)
}

// RepositoryExists 检查仓库是否存在（有至少一个 Tag）
func (s *GenericAPIService) RepositoryExists(registry, username, password, namespace, repo, authServer, dockerService string) (bool, error) {
	tags, err := s.GetTags(registry, username, password, namespace, repo, authServer, dockerService)
	if err != nil {
		return false, err
	}
	return len(tags) > 0, nil
}

// IsRepositoryNotFound 判断错误是否表示仓库不存在（404）或无权限（401/403）
func (s *GenericAPIService) IsRepositoryNotFound(err error) bool {
	if err == nil {
		return false
	}
	msg := err.Error()
	return strings.Contains(msg, "HTTP 404") || strings.Contains(msg, "HTTP 401") || strings.Contains(msg, "HTTP 403")
}

// fetchManifestGeneric 拉取 manifest：Basic 直连优先，401 Bearer challenge 时
// 换 token 重试（doV2Request 策略）；带 429 限流重试。
// 返回 body、digest、Last-Modified 与 bearer token（Basic 模式下为空）
func (s *GenericAPIService) fetchManifestGeneric(registry, username, password, namespace, repo, tag string, forceTokenRefresh bool) ([]byte, string, string, string, error) {
	path := fmt.Sprintf("/v2/%s/%s/manifests/%s", namespace, repo, tag)
	scope := fmt.Sprintf("repository:%s/%s:pull", namespace, repo)
	if forceTokenRefresh {
		s.invalidateTokenCache(registry, scope)
	}

	for attempt := 0; attempt <= tagDetailMaxRetries; attempt++ {
		resp, err := s.doV2Request(registry, username, password, namespace, repo, "GET", path, manifestAcceptHeader)
		if err != nil {
			return nil, "", "", "", fmt.Errorf("获取Manifest失败: %w", err)
		}

		switch {
		case resp.StatusCode() == 200:
			lastModified := firstNonEmpty(
				parseHTTPTime(resp.Header().Get("Last-Modified")),
				parseHTTPTime(resp.Header().Get("Date")),
			)
			token := ""
			if info := s.cachedAuth(registry); info != nil && info.Realm != "" {
				token, _ = s.getToken(registry, username, password, namespace, repo, scope)
			}
			return resp.Body(), resp.Header().Get("Docker-Content-Digest"), lastModified, token, nil
		case resp.StatusCode() == 429 && attempt < tagDetailMaxRetries:
			time.Sleep(tagDetailRetryBaseGap * time.Duration(attempt+1))
			continue
		default:
			return nil, "", "", "", fmt.Errorf("获取Manifest失败: HTTP %d", resp.StatusCode())
		}
	}
	return nil, "", "", "", fmt.Errorf("获取Manifest失败: HTTP 429")
}

// cachedAuth 读取已缓存的认证信息（不触发探测）
func (s *GenericAPIService) cachedAuth(registry string) *genericAuthInfo {
	s.authMu.Lock()
	defer s.authMu.Unlock()
	return s.authCache[registry]
}

// GetTagDetail 获取 Tag 详细信息（架构/digest/大小/推送时间）
func (s *GenericAPIService) GetTagDetail(registry, username, password, namespace, repo, tag, authServer, dockerService string) (*TagDetail, error) {
	body, contentDigest, manifestLastModified, token, err := s.fetchManifestGeneric(registry, username, password, namespace, repo, tag, false)
	if err != nil {
		return nil, err
	}
	return parseManifestToTagDetail(s.client, tag, registry, namespace, repo, token, body, contentDigest, manifestLastModified)
}

// GetTagsDetailsBatch 批量获取指定 Tag 的详细信息
func (s *GenericAPIService) GetTagsDetailsBatch(registry, username, password, namespace, repo, authServer, dockerService string, tagNames []string) ([]*TagDetail, error) {
	if len(tagNames) == 0 {
		return []*TagDetail{}, nil
	}
	if len(tagNames) > tagDetailsBatchMaxSize {
		return nil, fmt.Errorf("单次最多查询 %d 个 Tag", tagDetailsBatchMaxSize)
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

			body, contentDigest, manifestLastModified, token, err := s.fetchManifestGeneric(registry, username, password, namespace, repo, tag, false)
			if err != nil {
				details[index] = emptyTagDetail(tag)
				return
			}

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

// GetTagsWithDetails 获取全部 Tag 及详情
func (s *GenericAPIService) GetTagsWithDetails(registry, username, password, namespace, repo, authServer, dockerService string) ([]*TagDetail, error) {
	tagNames, err := s.GetTagNames(registry, username, password, namespace, repo, authServer, dockerService)
	if err != nil {
		return nil, err
	}
	return s.GetTagsDetailsBatch(registry, username, password, namespace, repo, authServer, dockerService, tagNames)
}

// ListRepositories 通过 /v2/_catalog 列出仓库（标准 Registry/Harbor 支持；
// 不支持的仓库会返回 404，此时请使用"批量添加"）
func (s *GenericAPIService) ListRepositories(registry, username, password, accessKey, secretKey, namespace, authServer, dockerService string) ([]string, error) {
	scope := "registry:catalog:*"

	repos := make([]string, 0, 64)
	last := ""
	for {
		path := "/v2/_catalog?n=1000"
		if last != "" {
			path += "&last=" + last
		}

		token, err := s.getToken(registry, username, password, "", "", scope)
		if err != nil {
			return nil, fmt.Errorf("获取Catalog权限失败: %w", err)
		}

		req := s.client.R().SetHeader("Accept", "application/json")
		if username != "" {
			req = req.SetBasicAuth(username, password)
		}
		resp, err := req.SetAuthToken(token).Get(fmt.Sprintf("https://%s%s", registry, path))
		if err != nil {
			return nil, fmt.Errorf("获取仓库列表失败: %w", err)
		}
		if resp.StatusCode() == 404 {
			return nil, fmt.Errorf("该仓库不支持 /v2/_catalog，无法远程列举仓库，请使用批量添加")
		}
		if resp.StatusCode() == 401 {
			return nil, fmt.Errorf("获取仓库列表失败: 认证被拒绝（HTTP 401）")
		}
		if resp.StatusCode() != 200 {
			return nil, fmt.Errorf("获取仓库列表失败: HTTP %d", resp.StatusCode())
		}

		var result struct {
			Repositories []string `json:"repositories"`
		}
		if err := json.Unmarshal(resp.Body(), &result); err != nil {
			return nil, fmt.Errorf("解析仓库列表失败: %w", err)
		}
		for _, repo := range result.Repositories {
			// catalog 返回形如 namespace/repo 的全路径，过滤出本命名空间
			if namespace != "" && strings.HasPrefix(repo, namespace+"/") {
				repos = append(repos, strings.TrimPrefix(repo, namespace+"/"))
			}
		}

		if len(result.Repositories) < 1000 {
			break
		}
		last = result.Repositories[len(result.Repositories)-1]
	}

	return repos, nil
}

// TestConnection 测试配置连通性（地址 + 账号密码：/v2/ ping，Basic 直连或 Bearer challenge）
func (s *GenericAPIService) TestConnection(registry, username, password, accessKey, secretKey, namespace, authServer, dockerService string) *RegistryTestResult {
	result := &RegistryTestResult{RegistryType: "generic"}

	req := s.client.R()
	if username != "" {
		req = req.SetBasicAuth(username, password)
	}
	resp, err := req.Get(fmt.Sprintf("https://%s/v2/", registry))
	if err != nil {
		result.LoginMessage = fmt.Sprintf("连接失败: %v", err)
		return result
	}

	switch {
	case resp.StatusCode() == 200:
		result.LoginOK = true
		result.LoginMessage = "凭证可用（仓库地址与账号密码验证通过）"
	case resp.StatusCode() == 401 && parseBearerChallenge(resp.Header().Get("WWW-Authenticate")) != nil:
		// 需要 token 认证：尝试对探测仓库换取 token
		if _, tokenErr := s.getToken(registry, username, password, namespace, "connection-test",
			fmt.Sprintf("repository:%s/connection-test:pull", namespace)); tokenErr != nil {
			result.LoginMessage = tokenErr.Error()
		} else {
			result.LoginOK = true
			result.LoginMessage = "凭证可用（仓库地址与账号密码验证通过）"
		}
	case resp.StatusCode() == 401:
		result.LoginMessage = "认证失败（HTTP 401）：用户名或密码错误"
	default:
		result.LoginMessage = fmt.Sprintf("仓库返回异常状态码 %d（地址可能不正确）", resp.StatusCode())
	}
	return result
}
