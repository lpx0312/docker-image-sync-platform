package main

import (
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
	"time"
)

func TestSplitImageTag(t *testing.T) {
	tests := []struct {
		name string
		ref  string
		repo string
		tag  string
	}{
		{"无tag默认latest", "nginx", "nginx", "latest"},
		{"dockerhub带tag", "nginx:1.25", "nginx", "1.25"},
		{"带仓库地址", "ghcr.io/prometheus/prometheus:v2.50.0", "ghcr.io/prometheus/prometheus", "v2.50.0"},
		{"带端口不误判", "registry.example.com:5000/app", "registry.example.com:5000/app", "latest"},
		{"带端口且带tag", "registry.example.com:5000/app:v1", "registry.example.com:5000/app", "v1"},
		{"空tag回退latest", "nginx:", "nginx", "latest"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			repo, tag := splitImageTag(tt.ref)
			if repo != tt.repo || tag != tt.tag {
				t.Errorf("splitImageTag(%q) = (%q, %q), 期望 (%q, %q)", tt.ref, repo, tag, tt.repo, tt.tag)
			}
		})
	}
}

func TestParseBatchFile(t *testing.T) {
	content := `# 注释行
nginx:1.25
redis:7-alpine  7-alpine-amd64

  # 缩进注释
k8s.gcr.io/pause:3.9	3.9
`
	path := filepath.Join(t.TempDir(), "images.txt")
	if err := os.WriteFile(path, []byte(content), 0o600); err != nil {
		t.Fatal(err)
	}

	items, err := parseBatchFile(path)
	if err != nil {
		t.Fatalf("parseBatchFile 报错: %v", err)
	}
	want := []batchImageItem{
		{SourceImage: "nginx:1.25"},
		{SourceImage: "redis:7-alpine", TargetTag: "7-alpine-amd64"},
		{SourceImage: "k8s.gcr.io/pause:3.9", TargetTag: "3.9"},
	}
	if !reflect.DeepEqual(items, want) {
		t.Errorf("解析结果 = %+v, 期望 %+v", items, want)
	}
}

func TestParseBatchFileEmpty(t *testing.T) {
	path := filepath.Join(t.TempDir(), "empty.txt")
	os.WriteFile(path, []byte("# 只有注释\n\n"), 0o600)
	if _, err := parseBatchFile(path); err == nil {
		t.Error("空文件应报错")
	}
}

func TestDecideDedup(t *testing.T) {
	item := func(hasAffinity bool, suggestedID uint, suggestedNs string) checkAcrItem {
		return checkAcrItem{
			Image:              "nginx:1.25",
			RepositoryName:     "nginx",
			HasAffinity:        hasAffinity,
			SuggestedAcrID:     suggestedID,
			SuggestedNamespace: suggestedNs,
		}
	}
	tests := []struct {
		name     string
		item     checkAcrItem
		acrID    uint
		acrNs    string
		tag      string
		tags     []string
		known    bool
		action   string
		contains string
	}{
		{
			name:  "新仓库放行",
			item:  item(false, 0, ""),
			acrID: 1, acrNs: "ns-a", tag: "1.25",
			action: "proceed",
		},
		{
			name:  "归属其他ACR拦截",
			item:  item(true, 2, "ns-b"),
			acrID: 1, acrNs: "ns-a", tag: "1.25",
			action:   "block",
			contains: "已归属",
		},
		{
			name:  "同tag已存在拦截且输出完整地址",
			item:  item(true, 1, "ns-a"),
			acrID: 1, acrNs: "ns-a", tag: "1.25",
			tags: []string{"1.24", "1.25"}, known: true,
			action:   "block",
			contains: "registry.example.com/ns-a/nginx:1.25",
		},
		{
			name:  "仓库存在但tag是新的放行",
			item:  item(true, 1, "ns-a"),
			acrID: 1, acrNs: "ns-a", tag: "1.27",
			tags: []string{"1.24", "1.25"}, known: true,
			action:   "info",
			contains: "追加新 Tag",
		},
		{
			name:  "tag校验不可用时提示放行",
			item:  item(true, 1, "ns-a"),
			acrID: 1, acrNs: "ns-a", tag: "1.25",
			known:    false,
			action:   "info",
			contains: "Tag 校验跳过",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := decideDedup(tt.item, tt.acrID, "registry.example.com", "", tt.acrNs, tt.tag, tt.tags, tt.known)
			if got.Action != tt.action {
				t.Errorf("action = %q, 期望 %q（消息: %s）", got.Action, tt.action, got.Message)
			}
			if tt.contains != "" && !strings.Contains(got.Message, tt.contains) {
				t.Errorf("消息 %q 未包含 %q", got.Message, tt.contains)
			}
		})
	}
}

func TestTagCacheExpiry(t *testing.T) {
	now := time.Now()
	fresh := tagCacheEntry{FetchedAt: now.Add(-1 * time.Hour)}
	stale := tagCacheEntry{FetchedAt: now.Add(-25 * time.Hour)}
	if fresh.isExpired(now) {
		t.Error("1 小时前的缓存不应过期")
	}
	if !stale.isExpired(now) {
		t.Error("25 小时前的缓存应已过期")
	}
}
