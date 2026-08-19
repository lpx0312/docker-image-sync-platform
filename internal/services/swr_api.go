package services

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"sync"
	"time"

	"github.com/go-resty/resty/v2"
)

// SwrAPIService 华为云 SWR 数据面 API 客户端
//
// 实测认证流程（与 docker login 行为一致）：
//  1. GET https://{registry}/v2/（不带认证）→ 401，响应头
//     WWW-Authenticate: Bearer realm="https://{registry}/swr/auth/v2/registry/auth/",service="dockyard"
//  2. GET {realm}?service=...&scope=repository:{组织}/{仓库}:pull（Basic 认证）→ {"token":"<JWT>"}（约 900s 有效）
//  3. 之后所有 /v2 请求携带 Authorization: Bearer <token>
//
// 注意：
//   - Basic 直连 /v2 会被拒绝（401），必须走 token 流程
//   - scope 必须携带具体 repository:{ns}/{repo}:pull，无 scope 的 token 无仓库访问权限
//   - 组织不存在时 token 端点直接返回 401 DENIED（"Image organization does not exist"）
//   - /v2/_catalog 不被支持（404），列举仓库走管理面 API（见 ListRepositories）
//   - 不存在的仓库 tags/list 返回 200 且 tags 为 null，RepositoryExists 依此判断
type SwrAPIService struct {
	client *resty.Client

	tokenMu    sync.Mutex
	tokenCache map[string]*tokenCacheItem

	authMu    sync.Mutex
	authCache map[string]*swrAuthInfo
}

var _ RegistryAPIClient = (*SwrAPIService)(nil)

type swrAuthInfo struct {
	Realm   string
	Service string
}

// NewSwrAPIService 创建 SWR API 服务实例
func NewSwrAPIService() *SwrAPIService {
	return &SwrAPIService{
		client:     resty.New().SetTimeout(30 * time.Second),
		tokenCache: make(map[string]*tokenCacheItem),
		authCache:  make(map[string]*swrAuthInfo),
	}
}

// discoverAuth 从 /v2/ 的认证 challenge 解析 token 端点（realm/service），按 registry 缓存
func (s *SwrAPIService) discoverAuth(registry string) *swrAuthInfo {
	s.authMu.Lock()
	if info, ok := s.authCache[registry]; ok {
		s.authMu.Unlock()
		return info
	}
	s.authMu.Unlock()

	info := &swrAuthInfo{
		Realm:   fmt.Sprintf("https://%s/swr/auth/v2/registry/auth/", registry),
		Service: "dockyard",
	}

	resp, err := s.client.R().Get(fmt.Sprintf("https://%s/v2/", registry))
	if err == nil && resp.StatusCode() == 401 {
		if parsed := parseBearerChallenge(resp.Header().Get("WWW-Authenticate")); parsed != nil {
			info = parsed
		}
	}

	s.authMu.Lock()
	s.authCache[registry] = info
	s.authMu.Unlock()
	return info
}

// parseBearerChallenge 解析 WWW-Authenticate: Bearer realm="...",service="..."
func parseBearerChallenge(header string) *swrAuthInfo {
	if !strings.HasPrefix(strings.ToLower(strings.TrimSpace(header)), "bearer ") {
		return nil
	}
	rest := strings.TrimSpace(header[len("Bearer "):])

	info := &swrAuthInfo{}
	for _, part := range strings.Split(rest, ",") {
		part = strings.TrimSpace(part)
		eq := strings.IndexByte(part, '=')
		if eq < 0 {
			continue
		}
		k := strings.TrimSpace(part[:eq])
		v := strings.Trim(strings.TrimSpace(part[eq+1:]), "\"")
		switch k {
		case "realm":
			info.Realm = v
		case "service":
			info.Service = v
		}
	}
	if info.Realm == "" {
		return nil
	}
	return info
}

func swrTokenCacheKey(registry, namespace, repo string) string {
	return fmt.Sprintf("%s:%s/%s", registry, namespace, repo)
}

func (s *SwrAPIService) invalidateTokenCache(registry, namespace, repo string) {
	s.tokenMu.Lock()
	defer s.tokenMu.Unlock()
	delete(s.tokenCache, swrTokenCacheKey(registry, namespace, repo))
}

// GetToken 获取 SWR 访问 token（scope 限定到具体仓库）
func (s *SwrAPIService) GetToken(registry, username, password, namespace, repo string) (string, error) {
	cacheKey := swrTokenCacheKey(registry, namespace, repo)

	s.tokenMu.Lock()
	if item, ok := s.tokenCache[cacheKey]; ok && time.Now().Before(item.expiresAt) {
		token := item.token
		s.tokenMu.Unlock()
		return token, nil
	}
	s.tokenMu.Unlock()

	auth := s.discoverAuth(registry)
	scope := fmt.Sprintf("repository:%s/%s:pull", namespace, repo)

	var result struct {
		Token     string `json:"token"`
		ExpiresIn int    `json:"expires_in"`
	}

	resp, err := s.client.R().
		SetBasicAuth(username, password).
		SetQueryParams(map[string]string{
			"service": auth.Service,
			"scope":   scope,
		}).
		SetResult(&result).
		Get(auth.Realm)
	if err != nil {
		return "", fmt.Errorf("获取SWR Token失败: %w", err)
	}
	if resp.StatusCode() == 401 {
		return "", fmt.Errorf("获取SWR Token失败: 认证被拒绝（组织不存在或无权限）")
	}
	if resp.StatusCode() != 200 {
		return "", fmt.Errorf("获取SWR Token失败: HTTP %d", resp.StatusCode())
	}
	if result.Token == "" {
		return "", fmt.Errorf("获取SWR Token失败: 返回空Token")
	}

	// SWR token 为 JWT（实测约 900s），响应通常无 expires_in，按 10 分钟缓存并提前 60s 失效
	expiresIn := result.ExpiresIn
	if expiresIn <= 0 {
		expiresIn = 600
	}
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

// GetTags 获取镜像的所有 Tag 名称
func (s *SwrAPIService) GetTags(registry, username, password, namespace, repo, _, _ string) ([]string, error) {
	for attempt := 0; attempt < 2; attempt++ {
		token, err := s.GetToken(registry, username, password, namespace, repo)
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

		// SWR 对不存在的仓库返回 200 且 tags 为 null；列表中可能混入空串条目
		if result.Tags == nil {
			return []string{}, nil
		}
		tags := make([]string, 0, len(result.Tags))
		for _, tag := range result.Tags {
			if tag = strings.TrimSpace(tag); tag != "" {
				tags = append(tags, tag)
			}
		}
		return tags, nil
	}

	return nil, fmt.Errorf("获取Tag列表失败: HTTP 401")
}

// GetTagNames 获取镜像 Tag 名称列表
func (s *SwrAPIService) GetTagNames(registry, username, password, namespace, repo, _, _ string) ([]string, error) {
	return s.GetTags(registry, username, password, namespace, repo, "", "")
}

// RepositoryExists 检查仓库是否存在（有至少一个 Tag）
// SWR 对已存在组织下不存在的仓库返回 200 + tags:null，依此判断
func (s *SwrAPIService) RepositoryExists(registry, username, password, namespace, repo, authServer, dockerService string) (bool, error) {
	tags, err := s.GetTags(registry, username, password, namespace, repo, authServer, dockerService)
	if err != nil {
		return false, err
	}
	return len(tags) > 0, nil
}

// IsRepositoryNotFound 判断错误是否表示仓库不存在（404）或组织不存在/无权限（401 DENIED）
func (s *SwrAPIService) IsRepositoryNotFound(err error) bool {
	if err == nil {
		return false
	}
	msg := err.Error()
	return strings.Contains(msg, "HTTP 404") || strings.Contains(msg, "HTTP 401") || strings.Contains(msg, "认证被拒绝")
}

// fetchManifest 拉取 manifest，401 时自动刷新 token
func (s *SwrAPIService) fetchManifest(registry, namespace, repo, tag string, getToken func(forceRefresh bool) (string, error)) ([]byte, string, string, int, error) {
	return fetchManifestWithRetry(s.client, registry, namespace, repo, tag, getToken)
}

// GetTagDetail 获取 Tag 详细信息（架构/digest/大小/推送时间）
func (s *SwrAPIService) GetTagDetail(registry, username, password, namespace, repo, tag, _, _ string) (*TagDetail, error) {
	token, err := s.GetToken(registry, username, password, namespace, repo)
	if err != nil {
		return nil, err
	}

	getToken := func(forceRefresh bool) (string, error) {
		if forceRefresh {
			s.invalidateTokenCache(registry, namespace, repo)
			return s.GetToken(registry, username, password, namespace, repo)
		}
		return token, nil
	}

	body, contentDigest, manifestLastModified, _, err := s.fetchManifest(registry, namespace, repo, tag, getToken)
	if err != nil {
		return nil, err
	}

	return parseManifestToTagDetail(s.client, tag, registry, namespace, repo, token, body, contentDigest, manifestLastModified)
}

// GetTagsDetailsBatch 批量获取指定 Tag 的详细信息
func (s *SwrAPIService) GetTagsDetailsBatch(registry, username, password, namespace, repo, _, _ string, tagNames []string) ([]*TagDetail, error) {
	if len(tagNames) == 0 {
		return []*TagDetail{}, nil
	}
	if len(tagNames) > tagDetailsBatchMaxSize {
		return nil, fmt.Errorf("单次最多查询 %d 个 Tag", tagDetailsBatchMaxSize)
	}

	var tokenMu sync.Mutex
	currentToken, err := s.GetToken(registry, username, password, namespace, repo)
	if err != nil {
		return nil, err
	}

	getToken := func(forceRefresh bool) (string, error) {
		tokenMu.Lock()
		defer tokenMu.Unlock()

		if forceRefresh {
			s.invalidateTokenCache(registry, namespace, repo)
			refreshed, err := s.GetToken(registry, username, password, namespace, repo)
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

			body, contentDigest, manifestLastModified, _, err := s.fetchManifest(registry, namespace, repo, tag, getToken)
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

// GetTagsWithDetails 获取全部 Tag 及详情
func (s *SwrAPIService) GetTagsWithDetails(registry, username, password, namespace, repo, authServer, dockerService string) ([]*TagDetail, error) {
	tagNames, err := s.GetTagNames(registry, username, password, namespace, repo, authServer, dockerService)
	if err != nil {
		return nil, err
	}
	return s.GetTagsDetailsBatch(registry, username, password, namespace, repo, authServer, dockerService, tagNames)
}

// ListRepositories 通过 SWR 管理面 API（swr-api.{region}.myhuaweicloud.com）
// 列出组织内全部镜像仓库，用于「从仓库导入」。SWR 数据面不支持 /v2/_catalog。
//
// 管理面凭证为 IAM AK/SK（与数据面登录凭证相互独立）：accessKey/secretKey
// 来自镜像仓库配置中单独填写的 Access Key / Secret Key；accessKey 为空时
// 兜底从登录用户名（区域@AK）中提取。
func (s *SwrAPIService) ListRepositories(registry, username, password, accessKey, secretKey, namespace, authServer, dockerService string) ([]string, error) {
	if secretKey == "" {
		return nil, fmt.Errorf("未配置 SWR 管理面 Secret Key（AK/SK），无法获取镜像列表，请在镜像仓库配置中填写")
	}
	region := swrRegion(registry, username)
	if region == "" {
		return nil, fmt.Errorf("无法从仓库地址 %s 或用户名 %s 推断 SWR 区域", registry, username)
	}
	ak := accessKey
	if ak == "" {
		ak = swrAKFromUsername(username)
	}
	host := fmt.Sprintf("swr-api.%s.myhuaweicloud.com", region)
	signer := swrSigner{AccessKey: ak, SecretKey: secretKey}

	httpClient := &http.Client{Timeout: 30 * time.Second}
	repos := make([]string, 0, 64)
	const limit = 100

	for offset := 0; ; offset += limit {
		u := &url.URL{
			Scheme: "https",
			Host:   host,
			Path:   "/v2/manage/repos",
			RawQuery: url.Values{
				"namespace": []string{namespace},
				"limit":     []string{fmt.Sprintf("%d", limit)},
				"offset":    []string{fmt.Sprintf("%d", offset)},
			}.Encode(),
		}
		req, err := http.NewRequest(http.MethodGet, u.String(), nil)
		if err != nil {
			return nil, err
		}
		signer.sign(req, time.Now())

		resp, err := httpClient.Do(req)
		if err != nil {
			return nil, fmt.Errorf("请求 SWR 管理面失败: %w", err)
		}
		body, _ := io.ReadAll(io.LimitReader(resp.Body, 8<<20))
		resp.Body.Close()

		if resp.StatusCode == http.StatusUnauthorized {
			return nil, fmt.Errorf("SWR 管理面 AK/SK 签名验证失败，请检查镜像仓库配置中的 Access Key / Secret Key（华为云控制台「我的凭证 → 访问密钥」）")
		}
		if resp.StatusCode != http.StatusOK {
			detail := string(body)
			if len(detail) > 200 {
				detail = detail[:200]
			}
			return nil, fmt.Errorf("获取 SWR 仓库列表失败: HTTP %d: %s", resp.StatusCode, detail)
		}

		items, err := parseSwrRepoList(body)
		if err != nil {
			return nil, err
		}
		for _, it := range items {
			name := it
			if idx := strings.LastIndex(name, "/"); idx >= 0 {
				name = name[idx+1:]
			}
			if name = strings.TrimSpace(name); name != "" {
				repos = append(repos, name)
			}
		}
		if len(items) < limit {
			break
		}
	}

	return repos, nil
}

// parseSwrRepoList 兼容数组与 {"body":[...]} 两种响应形态
func parseSwrRepoList(body []byte) ([]string, error) {
	var arr []struct {
		Name string `json:"name"`
		Path string `json:"path"`
	}
	if err := json.Unmarshal(body, &arr); err == nil {
		names := make([]string, 0, len(arr))
		for _, it := range arr {
			if it.Name != "" {
				names = append(names, it.Name)
			} else if it.Path != "" {
				names = append(names, it.Path)
			}
		}
		return names, nil
	}

	var wrapped struct {
		Body []struct {
			Name string `json:"name"`
			Path string `json:"path"`
		} `json:"body"`
	}
	if err := json.Unmarshal(body, &wrapped); err == nil {
		names := make([]string, 0, len(wrapped.Body))
		for _, it := range wrapped.Body {
			if it.Name != "" {
				names = append(names, it.Name)
			} else if it.Path != "" {
				names = append(names, it.Path)
			}
		}
		return names, nil
	}

	return nil, fmt.Errorf("解析 SWR 仓库列表响应失败")
}

// swrRegion 从仓库地址（swr.{region}.myhuaweicloud.com）或用户名（{region}@AK）推断区域
func swrRegion(registry, username string) string {
	if parts := strings.Split(registry, "."); len(parts) >= 2 && parts[0] == "swr" {
		return parts[1]
	}
	if idx := strings.Index(username, "@"); idx > 0 {
		return username[:idx]
	}
	return ""
}

// TestConnection 测试 SWR 配置连通性：数据面登录凭证（推送/拉取）+ 管理面 AK/SK（获取镜像列表）
func (s *SwrAPIService) TestConnection(registry, username, password, accessKey, secretKey, namespace, authServer, dockerService string) *RegistryTestResult {
	result := &RegistryTestResult{RegistryType: "swr"}

	// 数据面：用登录凭证换取 scoped token（与 docker login 等价），组织不存在/无权限会报错
	if _, err := s.GetToken(registry, username, password, namespace, "connection-test"); err != nil {
		result.LoginMessage = err.Error()
	} else {
		result.LoginOK = true
		result.LoginMessage = "登录凭证可用（可推送/拉取镜像）"
	}

	// 管理面：AK/SK 签名查询组织内仓库（limit=1，仅验证连通与权限）
	if secretKey == "" {
		result.ManageSkipped = true
		result.ManageMessage = "未配置 AK/SK（可选；不配置则「从仓库导入」不可用）"
		return result
	}
	if err := s.testManageAccess(registry, username, accessKey, secretKey, namespace); err != nil {
		result.ManageMessage = err.Error()
	} else {
		result.ManageOK = true
		result.ManageMessage = "AK/SK 可用（可获取镜像列表）"
	}
	return result
}

// testManageAccess 用 AK/SK 签名调用管理面验证连通性（GET /v2/manage/repos?namespace=xx&limit=1）
func (s *SwrAPIService) testManageAccess(registry, username, accessKey, secretKey, namespace string) error {
	region := swrRegion(registry, username)
	if region == "" {
		return fmt.Errorf("无法从仓库地址 %s 或用户名 %s 推断 SWR 区域", registry, username)
	}
	host := fmt.Sprintf("swr-api.%s.myhuaweicloud.com", region)
	ak := accessKey
	if ak == "" {
		ak = swrAKFromUsername(username)
	}

	u := &url.URL{
		Scheme:   "https",
		Host:     host,
		Path:     "/v2/manage/repos",
		RawQuery: url.Values{"namespace": []string{namespace}, "limit": []string{"1"}}.Encode(),
	}
	req, err := http.NewRequest(http.MethodGet, u.String(), nil)
	if err != nil {
		return err
	}
	swrSigner{AccessKey: ak, SecretKey: secretKey}.sign(req, time.Now())

	resp, err := (&http.Client{Timeout: 30 * time.Second}).Do(req)
	if err != nil {
		return fmt.Errorf("请求 SWR 管理面失败: %w", err)
	}
	defer resp.Body.Close()
	body, _ := io.ReadAll(io.LimitReader(resp.Body, 4096))

	switch resp.StatusCode {
	case 200:
		return nil
	case 401:
		return fmt.Errorf("AK/SK 签名验证失败（请检查 Access Key / Secret Key）")
	case 404:
		return fmt.Errorf("组织 %s 不存在", namespace)
	default:
		detail := string(body)
		if len(detail) > 200 {
			detail = detail[:200]
		}
		return fmt.Errorf("HTTP %d: %s", resp.StatusCode, detail)
	}
}
