package services

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"sort"
	"strings"
	"time"

	"github.com/go-resty/resty/v2"
)

// 本文件存放 ACR / SWR 客户端共用的 Docker Registry v2 协议解析逻辑，
// 以 *resty.Client 为首参的包级函数形式提供，两类客户端复用。

const (
	tagDetailMaxRetries    = 4
	tagDetailConcurrency   = 5
	tagDetailRetryBaseGap  = 300 * time.Millisecond
	tagDetailsBatchMaxSize = 50

	manifestAcceptHeader = "application/vnd.oci.image.index.v1+json,application/vnd.oci.image.manifest.v1+json,application/vnd.docker.distribution.manifest.v2+json,application/vnd.docker.distribution.manifest.list.v2+json"
)

// tokenCacheItem token 缓存项（ACR/SWR 通用）
type tokenCacheItem struct {
	token     string
	expiresAt time.Time
}

// registryManifest 通用 manifest 结构（manifest list / OCI index / 单架构）
type registryManifest struct {
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

func firstNonEmpty(values ...string) string {
	for _, value := range values {
		if value != "" {
			return value
		}
	}
	return ""
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

// computeManifestDigest 从 manifest body 计算 sha256 digest（仓库未返回
// Docker-Content-Digest 响应头时的兜底，如部分 SWR 场景）
func computeManifestDigest(body []byte) string {
	sum := sha256.Sum256(body)
	return "sha256:" + hex.EncodeToString(sum[:])
}

func fetchImageArch(client *resty.Client, registry, token, namespace, repo, configDigest string) string {
	if configDigest == "" || token == "" {
		return ""
	}

	var cfg struct {
		Architecture string `json:"architecture"`
		OS           string `json:"os"`
	}

	for attempt := 0; attempt <= tagDetailMaxRetries; attempt++ {
		resp, err := client.R().
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

func fetchResourceLastModified(client *resty.Client, url, token string) string {
	if token == "" {
		// Basic 直连/匿名场景无 Bearer token，跳过（推送时间为空的代价可接受）
		return ""
	}
	resp, err := client.R().
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

func fetchManifestLastModified(client *resty.Client, registry, token, namespace, repo, reference string) string {
	if reference == "" {
		return ""
	}

	url := fmt.Sprintf("https://%s/v2/%s/%s/manifests/%s", registry, namespace, repo, reference)
	return fetchResourceLastModified(client, url, token)
}

func fetchBlobLastModified(client *resty.Client, registry, token, namespace, repo, digest string) string {
	if digest == "" {
		return ""
	}

	url := fmt.Sprintf("https://%s/v2/%s/%s/blobs/%s", registry, namespace, repo, digest)
	return fetchResourceLastModified(client, url, token)
}

func parseManifestToTagDetail(client *resty.Client, tag, registry, namespace, repo, token string, body []byte, contentDigest, manifestLastModified string) (*TagDetail, error) {
	var manifest registryManifest
	if err := json.Unmarshal(body, &manifest); err != nil {
		return nil, fmt.Errorf("解析Manifest失败: %w", err)
	}

	if contentDigest == "" {
		contentDigest = computeManifestDigest(body)
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
					fetchManifestLastModified(client, registry, token, namespace, repo, m.Digest),
					fetchBlobLastModified(client, registry, token, namespace, repo, m.Digest),
				)
			}
		}
		detail.Architectures = sortArchitectures(detail.Architectures)
		return detail, nil
	}

	arch := inferArchFromTag(tag)
	if arch == "" {
		arch = fetchImageArch(client, registry, token, namespace, repo, manifest.Config.Digest)
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
		fetchManifestLastModified(client, registry, token, namespace, repo, tag),
		fetchManifestLastModified(client, registry, token, namespace, repo, contentDigest),
		fetchBlobLastModified(client, registry, token, namespace, repo, digestForPushTime),
	)

	return detail, nil
}

// fetchManifestWithRetry 拉取 manifest 并处理 401 token 刷新与 429 限流重试。
// getToken(forceRefresh) 由各客户端提供 token 获取/刷新逻辑。
func fetchManifestWithRetry(client *resty.Client, registry, namespace, repo, tag string, getToken func(forceRefresh bool) (string, error)) ([]byte, string, string, int, error) {
	var lastStatus int
	tokenRefreshed := false

	for attempt := 0; attempt <= tagDetailMaxRetries; attempt++ {
		token, err := getToken(false)
		if err != nil {
			return nil, "", "", 0, err
		}

		resp, err := client.R().
			SetAuthToken(token).
			SetHeader("Accept", manifestAcceptHeader).
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
			contentDigest := resp.Header().Get("Docker-Content-Digest")
			return resp.Body(), contentDigest, lastModified, lastStatus, nil
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
