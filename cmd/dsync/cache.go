package main

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sync"
	"time"
)

// tagCacheTTL 缓存有效期，过期后 search 会增量重拉对应仓库的 tag。
const tagCacheTTL = 24 * time.Hour

// tagFetchConcurrency 并发拉取 tag 的上限，避免触发平台全局限流。
const tagFetchConcurrency = 4

// tagCacheEntry 单个仓库的 tag 缓存条目
type tagCacheEntry struct {
	AcrRegistryID  uint      `json:"acr_registry_id"`
	Namespace      string    `json:"namespace"`
	RepositoryName string    `json:"repository_name"`
	Tags           []string  `json:"tags"`
	FetchedAt      time.Time `json:"fetched_at"`
}

// tagCache 全量 tag 缓存，key 为 "<acrID>/<repo>"
type tagCache struct {
	Entries map[string]tagCacheEntry `json:"entries"`
}

func tagCachePath() string {
	dir, err := configDir()
	if err != nil {
		return "dsync-tag-cache.json"
	}
	return filepath.Join(dir, "tag-cache.json")
}

func loadTagCache() *tagCache {
	cache := &tagCache{Entries: map[string]tagCacheEntry{}}
	data, err := os.ReadFile(tagCachePath())
	if err != nil {
		return cache
	}
	_ = json.Unmarshal(data, cache)
	if cache.Entries == nil {
		cache.Entries = map[string]tagCacheEntry{}
	}
	return cache
}

func saveTagCache(cache *tagCache) error {
	data, err := json.MarshalIndent(cache, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(tagCachePath(), data, 0o600)
}

// isExpired 缓存条目是否过期。
func (e tagCacheEntry) isExpired(now time.Time) bool {
	return now.Sub(e.FetchedAt) > tagCacheTTL
}

// ensureTagCacheFresh 确保作用域内所有仓库的 tag 缓存可用：
// 缺失或过期的条目（force 时全部）以并发 4 增量拉取并落盘。
// 返回刷新的条目数与失败数。
func ensureTagCacheFresh(client *Client, scope []AcrRegistryInfo, force bool) (refreshed, failed int, err error) {
	cache := loadTagCache()
	now := time.Now()

	// 收集需要拉取的仓库
	type fetchJob struct {
		acrID uint
		ns    string
		repo  string
	}
	var jobs []fetchJob
	totalRepos := 0
	for _, reg := range scope {
		repos, err := fetchRepositories(client, reg.ID)
		if err != nil {
			return 0, 0, fmt.Errorf("获取 %s 的仓库列表失败: %w", reg.Namespace, err)
		}
		for _, r := range repos {
			totalRepos++
			key := fmt.Sprintf("%d/%s", reg.ID, r.RepositoryName)
			entry, ok := cache.Entries[key]
			if !force && ok && !entry.isExpired(now) {
				// 缓存可用，同步 namespace 以防 ACR 配置变更
				entry.Namespace = reg.Namespace
				cache.Entries[key] = entry
				continue
			}
			jobs = append(jobs, fetchJob{acrID: reg.ID, ns: reg.Namespace, repo: r.RepositoryName})
		}
	}

	if len(jobs) > 0 {
		fmt.Fprintf(os.Stderr, "刷新 tag 缓存: %d 个仓库需要更新（共 %d 个）...\n", len(jobs), totalRepos)
		var mu sync.Mutex
		done := 0
		sem := make(chan struct{}, tagFetchConcurrency)
		var wg sync.WaitGroup
		for _, job := range jobs {
			wg.Add(1)
			go func(job fetchJob) {
				defer wg.Done()
				sem <- struct{}{}
				defer func() { <-sem }()
				tags, err := fetchTagNames(client, job.acrID, job.repo)
				mu.Lock()
				defer mu.Unlock()
				if err != nil {
					failed++
				} else {
					cache.Entries[fmt.Sprintf("%d/%s", job.acrID, job.repo)] = tagCacheEntry{
						AcrRegistryID:  job.acrID,
						Namespace:      job.ns,
						RepositoryName: job.repo,
						Tags:           tags,
						FetchedAt:      time.Now(),
					}
					refreshed++
				}
				done++
				fmt.Fprintf(os.Stderr, "\r  %d/%d", done, len(jobs))
			}(job)
		}
		wg.Wait()
		fmt.Fprintln(os.Stderr)

		if err := saveTagCache(cache); err != nil {
			fmt.Fprintf(os.Stderr, "警告: tag 缓存写入失败: %v\n", err)
		}
	}
	return refreshed, failed, nil
}
