package services

import (
	"encoding/json"
	"fmt"
	"sort"
	"strings"
	"sync"
	"time"

	"github.com/go-resty/resty/v2"
)

// AcrAPIService ACR API调用服务
type AcrAPIService struct {
	client     *resty.Client
	tokenCache map[string]*tokenCacheItem
	tokenMu    sync.Mutex
}

type tokenCacheItem struct {
	token     string
	expiresAt time.Time
}

// ACRManifest ACR Manifest 结构
type ACRManifest struct {
	SchemaVersion int    `json:"schemaVersion"`
	MediaType     string `json:"mediaType"`
	Manifests     []struct {
		MediaType string `json:"mediaType"`
		Size      int64  `json:"size"`
		Digest    string `json:"digest"`
		Platform  struct {
			Architecture string `json:"architecture"`
			OS           string `json:"os"`
		} `json:"platform"`
	} `json:"manifests"`
	Config struct {
		MediaType string `json:"mediaType"`
		Size      int64  `json:"size"`
		Digest    string `json:"digest"`
	} `json:"config"`
	Layers []struct {
		MediaType string `json:"mediaType"`
		Size      int64  `json:"size"`
		Digest    string `json:"digest"`
	} `json:"layers"`
}

// TagDetail Tag详细信息
type TagDetail struct {
	Tag           string            `json:"tag"`
	Architectures []string          `json:"architectures"`
	Digests       map[string]string `json:"digests"`
	Sizes         map[string]int64  `json:"sizes"`
	PushedAt      map[string]string `json:"pushed_at"`
}

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

// GetToken 获取 ACR 认证 Token
func (s *AcrAPIService) GetToken(registry, username, password, namespace, repo, authServer, dockerService string) (string, error) {
	cacheKey := tokenCacheKey(registry, namespace, repo)

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
	scope := fmt.Sprintf("repository:%s/%s:pull", namespace, repo)
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

// RepositoryExists 检查 ACR 中仓库是否存在（有至少一个 Tag）
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

const (
	tagDetailMaxRetries      = 4
	tagDetailConcurrency     = 5
	tagDetailRetryBaseGap    = 300 * time.Millisecond
	tagDetailsBatchMaxSize   = 50
)

func inferArchFromTag(tag string) string {
	lower := strings.ToLower(tag)
	if strings.Contains(lower, "arm64") {
		return "arm64"
	}
	if strings.Contains(lower, "amd64") {
		return "amd64"
	}
	return ""
}

func sortArchitectures(archs []string) []string {
	sorted := append([]string(nil), archs...)
	sort.Slice(sorted, func(i, j int) bool {
		order := map[string]int{"amd64": 0, "arm64": 1}
		oi, oki := order[sorted[i]]
		oj, okj := order[sorted[j]]
		switch {
		case oki && okj:
			return oi < oj
		case oki:
			return true
		case okj:
			return false
		default:
			return sorted[i] < sorted[j]
		}
	})
	return sorted
}

func (s *AcrAPIService) fetchImageArch(registry, token, namespace, repo, configDigest string) string {
	if configDigest == "" {
		return ""
	}

	var cfg struct {
		Architecture string `json:"architecture"`
		OS           string `json:"os"`
	}

	for attempt := 0; attempt <= tagDetailMaxRetries; attempt++ {
		resp, err := s.client.R().
			SetAuthToken(token).
			Get(fmt.Sprintf("https://%s/v2/%s/%s/blobs/%s", registry, namespace, repo, configDigest))

		if err != nil {
			return ""
		}

		if resp.StatusCode() == 429 && attempt < tagDetailMaxRetries {
			time.Sleep(tagDetailRetryBaseGap * time.Duration(attempt+1))
			continue
		}

		if resp.StatusCode() != 200 {
			return ""
		}

		if err := json.Unmarshal(resp.Body(), &cfg); err != nil {
			return ""
		}

		if cfg.OS == "linux" && cfg.Architecture != "" {
			return cfg.Architecture
		}

		return ""
	}

	return ""
}

func parseHTTPTime(value string) string {
	if value == "" {
		return ""
	}

	layouts := []string{
		time.RFC1123,
		time.RFC1123Z,
		time.RFC3339,
		time.RFC3339Nano,
	}
	for _, layout := range layouts {
		if t, err := time.Parse(layout, value); err == nil {
			return t.Format(time.RFC3339)
		}
	}

	return value
}

func (s *AcrAPIService) fetchResourceLastModified(url, token string) string {
	resp, err := s.client.R().
		SetAuthToken(token).
		Head(url)
	if err != nil || resp.StatusCode() != 200 {
		return ""
	}

	if lastModified := parseHTTPTime(resp.Header().Get("Last-Modified")); lastModified != "" {
		return lastModified
	}

	return parseHTTPTime(resp.Header().Get("Date"))
}

func (s *AcrAPIService) fetchManifestLastModified(registry, token, namespace, repo, reference string) string {
	if reference == "" {
		return ""
	}

	url := fmt.Sprintf("https://%s/v2/%s/%s/manifests/%s", registry, namespace, repo, reference)
	return s.fetchResourceLastModified(url, token)
}

func (s *AcrAPIService) fetchBlobLastModified(registry, token, namespace, repo, digest string) string {
	if digest == "" {
		return ""
	}

	url := fmt.Sprintf("https://%s/v2/%s/%s/blobs/%s", registry, namespace, repo, digest)
	return s.fetchResourceLastModified(url, token)
}

func firstNonEmpty(values ...string) string {
	for _, value := range values {
		if value != "" {
			return value
		}
	}
	return ""
}

func (s *AcrAPIService) parseManifestToTagDetail(tag, registry, namespace, repo, token string, body []byte, contentDigest, manifestLastModified string) (*TagDetail, error) {
	var manifest ACRManifest
	if err := json.Unmarshal(body, &manifest); err != nil {
		return nil, fmt.Errorf("解析Manifest失败: %w", err)
	}

	detail := &TagDetail{
		Tag:           tag,
		Architectures: make([]string, 0),
		Digests:       make(map[string]string),
		Sizes:         make(map[string]int64),
		PushedAt:      make(map[string]string),
	}

	if len(manifest.Manifests) > 0 {
		for _, m := range manifest.Manifests {
			if m.Platform.OS == "linux" && m.Platform.Architecture != "" {
				arch := m.Platform.Architecture
				detail.Architectures = append(detail.Architectures, arch)
				detail.Digests[arch] = m.Digest
				detail.Sizes[arch] = m.Size
				detail.PushedAt[arch] = firstNonEmpty(
					manifestLastModified,
					s.fetchManifestLastModified(registry, token, namespace, repo, m.Digest),
					s.fetchBlobLastModified(registry, token, namespace, repo, m.Digest),
				)
			}
		}
		detail.Architectures = sortArchitectures(detail.Architectures)
		return detail, nil
	}

	arch := inferArchFromTag(tag)
	if arch == "" {
		arch = s.fetchImageArch(registry, token, namespace, repo, manifest.Config.Digest)
	}
	if arch == "" {
		return detail, nil
	}

	detail.Architectures = append(detail.Architectures, arch)
	detail.Digests[arch] = contentDigest

	var totalSize int64
	for _, layer := range manifest.Layers {
		totalSize += layer.Size
	}
	if totalSize > 0 {
		detail.Sizes[arch] = totalSize
	}

	digestForPushTime := contentDigest
	if manifest.Config.Digest != "" {
		digestForPushTime = manifest.Config.Digest
	}
	detail.PushedAt[arch] = firstNonEmpty(
		manifestLastModified,
		s.fetchManifestLastModified(registry, token, namespace, repo, tag),
		s.fetchManifestLastModified(registry, token, namespace, repo, contentDigest),
		s.fetchBlobLastModified(registry, token, namespace, repo, digestForPushTime),
	)

	return detail, nil
}

func (s *AcrAPIService) fetchManifest(registry, token, namespace, repo, tag string) ([]byte, string, string, int, error) {
	return s.fetchManifestWithRetry(registry, namespace, repo, tag, func(forceRefresh bool) (string, error) {
		if forceRefresh {
			return "", fmt.Errorf("token refresh not supported")
		}
		return token, nil
	})
}

func (s *AcrAPIService) fetchManifestWithRetry(registry, namespace, repo, tag string, getToken func(forceRefresh bool) (string, error)) ([]byte, string, string, int, error) {
	acceptHeader := "application/vnd.oci.image.index.v1+json,application/vnd.oci.image.manifest.v1+json,application/vnd.docker.distribution.manifest.v2+json,application/vnd.docker.distribution.manifest.list.v2+json"

	var lastStatus int
	tokenRefreshed := false

	for attempt := 0; attempt <= tagDetailMaxRetries; attempt++ {
		token, err := getToken(false)
		if err != nil {
			return nil, "", "", 0, err
		}

		resp, err := s.client.R().
			SetAuthToken(token).
			SetHeader("Accept", acceptHeader).
			Get(fmt.Sprintf("https://%s/v2/%s/%s/manifests/%s", registry, namespace, repo, tag))

		if err != nil {
			return nil, "", "", 0, fmt.Errorf("获取Manifest失败: %w", err)
		}

		lastStatus = resp.StatusCode()
		if lastStatus == 200 {
			lastModified := firstNonEmpty(
				parseHTTPTime(resp.Header().Get("Last-Modified")),
				parseHTTPTime(resp.Header().Get("Date")),
			)
			return resp.Body(), resp.Header().Get("Docker-Content-Digest"), lastModified, lastStatus, nil
		}

		if lastStatus == 401 && !tokenRefreshed {
			tokenRefreshed = true
			if _, err := getToken(true); err != nil {
				return nil, "", "", lastStatus, fmt.Errorf("获取Manifest失败: HTTP %d", lastStatus)
			}
			continue
		}

		if lastStatus == 429 && attempt < tagDetailMaxRetries {
			time.Sleep(tagDetailRetryBaseGap * time.Duration(attempt+1))
			continue
		}

		return nil, "", "", lastStatus, fmt.Errorf("获取Manifest失败: HTTP %d", lastStatus)
	}

	return nil, "", "", lastStatus, fmt.Errorf("获取Manifest失败: HTTP %d", lastStatus)
}

// GetTagDetail 获取 Tag 详细信息
func (s *AcrAPIService) GetTagDetail(registry, username, password, namespace, repo, tag, authServer, dockerService string) (*TagDetail, error) {
	token, err := s.GetToken(registry, username, password, namespace, repo, authServer, dockerService)
	if err != nil {
		return nil, err
	}

	body, contentDigest, manifestLastModified, _, err := s.fetchManifest(registry, token, namespace, repo, tag)
	if err != nil {
		return nil, err
	}

	return s.parseManifestToTagDetail(tag, registry, namespace, repo, token, body, contentDigest, manifestLastModified)
}

func emptyTagDetail(tag string) *TagDetail {
	return &TagDetail{
		Tag:           tag,
		Architectures: []string{},
		Digests:       map[string]string{},
		Sizes:         map[string]int64{},
		PushedAt:      map[string]string{},
	}
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

			body, contentDigest, manifestLastModified, _, err := s.fetchManifestWithRetry(registry, namespace, repo, tag, getToken)
			if err != nil {
				details[index] = emptyTagDetail(tag)
				return
			}

			token, _ := getToken(false)
			detail, err := s.parseManifestToTagDetail(tag, registry, namespace, repo, token, body, contentDigest, manifestLastModified)
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
