package main

import (
	"fmt"
	"net/url"
	"strings"
	"time"
)

// acrRepoEntry 平台本地库中的仓库记录（展示用精简版）
type acrRepoEntry struct {
	AcrRegistryID  uint      `json:"acr_registry_id"`
	RepositoryName string    `json:"repository_name"`
	CreatedAt      time.Time `json:"created_at"`
}

// listRegistries 获取所有 ACR 配置。
func listRegistries(c *Client) ([]AcrRegistryInfo, error) {
	var resp acrRegistriesResponse
	if err := c.do("GET", "/acr-registries", nil, &resp); err != nil {
		return nil, err
	}
	return resp.Data, nil
}

// fetchQuotaSummary 获取所有 ACR 的配额摘要。
func fetchQuotaSummary(c *Client) ([]quotaSummaryItem, error) {
	var resp quotaSummaryResponse
	if err := c.do("GET", "/acr-registries/quota-summary", nil, &resp); err != nil {
		return nil, err
	}
	return resp.Data, nil
}

// resolveAcrByNamespace 按 namespace 精确匹配 ACR。
// 未命中时列出所有可用 namespace，方便用户纠正。
func resolveAcrByNamespace(c *Client, ns string) (*AcrRegistryInfo, error) {
	regs, err := listRegistries(c)
	if err != nil {
		return nil, err
	}
	for i := range regs {
		if regs[i].Namespace == ns {
			return &regs[i], nil
		}
	}
	names := make([]string, 0, len(regs))
	for _, r := range regs {
		names = append(names, r.Namespace)
	}
	return nil, fmt.Errorf("未找到 namespace 为 %q 的 ACR，可用：%s", ns, strings.Join(names, ", "))
}

// suggestAcr 调用服务端亲和性逻辑推荐目标 ACR。
func suggestAcr(c *Client, image string) (*suggestAcrData, error) {
	var resp suggestAcrResponse
	if err := c.do("GET", "/sync/suggest-acr?image="+url.QueryEscape(image), nil, &resp); err != nil {
		return nil, err
	}
	return &resp.Data, nil
}

// fetchRepositories 获取指定 ACR 的本地镜像仓库列表。
func fetchRepositories(c *Client, acrRegistryID uint) ([]acrRepoEntry, error) {
	var resp acrRepositoriesResponse
	path := fmt.Sprintf("/acr-repositories?acr_registry_id=%d", acrRegistryID)
	if err := c.do("GET", path, nil, &resp); err != nil {
		return nil, err
	}
	entries := make([]acrRepoEntry, 0, len(resp.Data))
	for _, r := range resp.Data {
		entries = append(entries, acrRepoEntry{
			AcrRegistryID:  r.AcrRegistryID,
			RepositoryName: r.RepositoryName,
			CreatedAt:      r.CreatedAt,
		})
	}
	return entries, nil
}

// fetchTagNames 实时获取指定仓库的 tag 列表。
func fetchTagNames(c *Client, acrRegistryID uint, repo string) ([]string, error) {
	var resp acrTagsResponse
	path := fmt.Sprintf("/acr-tags?acr_registry_id=%d&repository_name=%s",
		acrRegistryID, url.QueryEscape(repo))
	if err := c.do("GET", path, nil, &resp); err != nil {
		return nil, err
	}
	return resp.Data.Tags, nil
}

// splitImageTag 拆分镜像引用为仓库名与 tag，无 tag 时默认 latest。
func splitImageTag(ref string) (repo, tag string) {
	tag = "latest"
	if i := strings.LastIndex(ref, ":"); i > strings.LastIndex(ref, "/") {
		repo = ref[:i]
		if t := ref[i+1:]; t != "" {
			tag = t
		}
	} else {
		repo = ref
	}
	return repo, tag
}
