package services

import (
	"encoding/json"
	"fmt"
	"strings"
	"time"

	"github.com/go-resty/resty/v2"
)

// AcrAPIService ACR API调用服务
type AcrAPIService struct {
	client     *resty.Client
	tokenCache map[string]*tokenCacheItem
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

// getAuthServer 从 registry 推断认证服务器地址
func getAuthServer(registry string) string {
	// registry.cn-hangzhou.aliyuncs.com -> dockerauth.cn-hangzhou.aliyuncs.com
	region := "cn-hangzhou"
	parts := strings.Split(registry, ".")
	if len(parts) >= 3 {
		region = parts[1]
	}
	return fmt.Sprintf("dockerauth.%s.aliyuncs.com", region)
}

// GetToken 获取 ACR 认证 Token
func (s *AcrAPIService) GetToken(registry, username, password, namespace, repo string) (string, error) {
	cacheKey := fmt.Sprintf("%s:%s/%s", registry, namespace, repo)

	// 检查缓存
	if item, ok := s.tokenCache[cacheKey]; ok && time.Now().Before(item.expiresAt) {
		return item.token, nil
	}

	authServer := getAuthServer(registry)
	scope := fmt.Sprintf("repository:%s/%s:pull", namespace, repo)
	service := fmt.Sprintf("registry.aliyuncs.com:cn-hangzhou:%s", namespace)

	var result struct {
		Token string `json:"token"`
	}

	resp, err := s.client.R().
		SetBasicAuth(username, password).
		SetFormData(map[string]string{
			"service": service,
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

	// 缓存 Token（25分钟）
	s.tokenCache[cacheKey] = &tokenCacheItem{
		token:     result.Token,
		expiresAt: time.Now().Add(25 * time.Minute),
	}

	return result.Token, nil
}

// GetTags 获取镜像的 Tag 列表
func (s *AcrAPIService) GetTags(registry, username, password, namespace, repo string) ([]string, error) {
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

	if resp.StatusCode() != 200 {
		return nil, fmt.Errorf("获取Tag列表失败: HTTP %d", resp.StatusCode())
	}

	return result.Tags, nil
}

// GetTagDetail 获取 Tag 详细信息
func (s *AcrAPIService) GetTagDetail(registry, username, password, namespace, repo, tag string) (*TagDetail, error) {
	token, err := s.GetToken(registry, username, password, namespace, repo)
	if err != nil {
		return nil, err
	}

	acceptHeader := "application/vnd.oci.image.index.v1+json,application/vnd.oci.image.manifest.v1+json,application/vnd.docker.distribution.manifest.v2+json,application/vnd.docker.distribution.manifest.list.v2+json"

	resp, err := s.client.R().
		SetAuthToken(token).
		SetHeader("Accept", acceptHeader).
		Get(fmt.Sprintf("https://%s/v2/%s/%s/manifests/%s", registry, namespace, repo, tag))

	if err != nil {
		return nil, fmt.Errorf("获取Manifest失败: %w", err)
	}

	if resp.StatusCode() != 200 {
		return nil, fmt.Errorf("获取Manifest失败: HTTP %d", resp.StatusCode())
	}

	var manifest ACRManifest
	if err := json.Unmarshal(resp.Body(), &manifest); err != nil {
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
		// 多架构镜像
		for _, m := range manifest.Manifests {
			if m.Platform.OS == "linux" && m.Platform.Architecture != "" {
				arch := m.Platform.Architecture
				detail.Architectures = append(detail.Architectures, arch)
				detail.Digests[arch] = m.Digest
				detail.Sizes[arch] = m.Size
			}
		}
	} else {
		// 单架构镜像
		detail.Architectures = append(detail.Architectures, "unknown")
		detail.Digests["unknown"] = resp.Header().Get("Docker-Content-Digest")
	}

	return detail, nil
}
