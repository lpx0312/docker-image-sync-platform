package main

// resolver.go 与 cache.go 的单元与边界测试。

import (
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func TestResolveAcrByNamespace(t *testing.T) {
	regs := func(w http.ResponseWriter) {
		respondJSON(w, `{"status":"success","data":[
			{"id":1,"registry_url":"r1","namespace":"ns-a","alias":"ali","username":"u"},
			{"id":2,"registry_url":"r2","namespace":"ns-a","alias":"hw","username":"u"},
			{"id":3,"registry_url":"r3","namespace":"ns-c","alias":"","username":"u"}
		]}`)
	}
	newSrv := func(h http.HandlerFunc) *httptest.Server { return httptest.NewServer(h) }

	t.Run("别名精确匹配优先", func(t *testing.T) {
		srv := newSrv(func(w http.ResponseWriter, r *http.Request) { regs(w) })
		defer srv.Close()
		reg, err := resolveAcrByNamespace(newTestClient(srv.URL), "ali")
		if err != nil {
			t.Fatal(err)
		}
		if reg.Alias != "ali" || reg.ID != 1 {
			t.Errorf("命中的 ACR = %+v, 期望 id=1", reg)
		}
	})
	t.Run("namespace 唯一命中", func(t *testing.T) {
		srv := newSrv(func(w http.ResponseWriter, r *http.Request) { regs(w) })
		defer srv.Close()
		reg, err := resolveAcrByNamespace(newTestClient(srv.URL), "ns-c")
		if err != nil {
			t.Fatal(err)
		}
		if reg.ID != 3 {
			t.Errorf("命中 = %+v, 期望 id=3", reg)
		}
	})
	t.Run("namespace 命中多个要求改用别名", func(t *testing.T) {
		srv := newSrv(func(w http.ResponseWriter, r *http.Request) {
			respondJSON(w, `{"status":"success","data":[
				{"id":1,"namespace":"dup","alias":"a1","username":"u"},
				{"id":2,"namespace":"dup","alias":"a2","username":"u"}
			]}`)
		})
		defer srv.Close()
		_, err := resolveAcrByNamespace(newTestClient(srv.URL), "dup")
		if err == nil || !strings.Contains(err.Error(), "同时是多个仓库的 namespace") {
			t.Fatalf("期望多命中报错, got %v", err)
		}
	})
	t.Run("未命中列出可用别名", func(t *testing.T) {
		srv := newSrv(func(w http.ResponseWriter, r *http.Request) { regs(w) })
		defer srv.Close()
		_, err := resolveAcrByNamespace(newTestClient(srv.URL), "nope")
		if err == nil || !strings.Contains(err.Error(), "未找到别名") {
			t.Fatalf("期望未命中报错, got %v", err)
		}
		if !strings.Contains(err.Error(), "ali") || !strings.Contains(err.Error(), "hw") {
			t.Errorf("报错应列出可用别名: %v", err)
		}
	})
	t.Run("接口错误透传", func(t *testing.T) {
		srv := newSrv(func(w http.ResponseWriter, r *http.Request) {
			w.WriteHeader(http.StatusInternalServerError)
		})
		defer srv.Close()
		if _, err := resolveAcrByNamespace(newTestClient(srv.URL), "x"); err == nil {
			t.Fatal("接口错误应透传")
		}
	})
}

func TestAliasOrNamespace(t *testing.T) {
	if aliasOrNamespace("a", "n") != "a" {
		t.Error("别名优先")
	}
	if aliasOrNamespace("", "n") != "n" {
		t.Error("别名缺失回退 namespace")
	}
}

// fetchersError 各查询函数在网络错误时的表现（覆盖各自错误分支）。
func TestFetchersNetworkError(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {}))
	url := srv.URL
	srv.Close()
	c := newTestClient(url)

	if _, err := listRegistries(c); err == nil {
		t.Error("listRegistries 应报错")
	}
	if _, err := fetchQuotaSummary(c); err == nil {
		t.Error("fetchQuotaSummary 应报错")
	}
	if _, err := fetchRepositories(c, 1); err == nil {
		t.Error("fetchRepositories 应报错")
	}
	if _, err := fetchTagNames(c, 1, "nginx"); err == nil {
		t.Error("fetchTagNames 应报错")
	}
	if _, err := suggestAcr(c, "nginx:1.0"); err == nil {
		t.Error("suggestAcr 应报错")
	}
}

func TestFetchRepositoriesAndTags(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/api/v1/acr-repositories":
			respondJSON(w, `{"status":"success","data":[
				{"id":11,"acr_registry_id":1,"repository_name":"nginx","created_at":"2026-10-01T00:00:00Z"}
			]}`)
		case "/api/v1/acr-tags":
			respondJSON(w, `{"status":"success","data":{"tags":["a","b"],"total":2}}`)
		default:
			w.WriteHeader(http.StatusNotFound)
		}
	}))
	defer srv.Close()
	c := newTestClient(srv.URL)

	repos, err := fetchRepositories(c, 1)
	if err != nil {
		t.Fatal(err)
	}
	if len(repos) != 1 || repos[0].RepositoryName != "nginx" || repos[0].AcrRegistryID != 1 {
		t.Errorf("repos = %+v", repos)
	}

	tags, err := fetchTagNames(c, 1, "nginx")
	if err != nil {
		t.Fatal(err)
	}
	if len(tags) != 2 || tags[0] != "a" {
		t.Errorf("tags = %v", tags)
	}
}

// ---- cache.go ----

func TestTagCachePath(t *testing.T) {
	isolateGlobals(t)

	t.Run("正常目录", func(t *testing.T) {
		dir := t.TempDir()
		configDirOverride = dir
		if got := tagCachePath(); got != filepath.Join(dir, "tag-cache.json") {
			t.Errorf("tagCachePath = %q", got)
		}
	})
	t.Run("configDir 失败回退当前目录", func(t *testing.T) {
		configDirOverride = ""
		clearUserConfigEnv(t)
		if got := tagCachePath(); got != "dsync-tag-cache.json" {
			t.Errorf("tagCachePath = %q, 期望 dsync-tag-cache.json", got)
		}
	})
}

func TestLoadTagCache(t *testing.T) {
	isolateGlobals(t)
	dir := t.TempDir()
	configDirOverride = dir
	p := filepath.Join(dir, "tag-cache.json")

	t.Run("文件不存在返回空缓存", func(t *testing.T) {
		c := loadTagCache()
		if c.Entries == nil || len(c.Entries) != 0 {
			t.Errorf("期望空缓存, got %+v", c)
		}
	})
	t.Run("畸形 JSON 返回空缓存", func(t *testing.T) {
		os.WriteFile(p, []byte("{broken"), 0o600)
		c := loadTagCache()
		if len(c.Entries) != 0 {
			t.Errorf("畸形 JSON 应回退空缓存, got %+v", c)
		}
	})
	t.Run("entries 为 null 时初始化", func(t *testing.T) {
		os.WriteFile(p, []byte(`{"entries":null}`), 0o600)
		c := loadTagCache()
		if c.Entries == nil {
			t.Error("entries 不应为 nil")
		}
	})
	t.Run("正常读取", func(t *testing.T) {
		os.WriteFile(p, []byte(`{"entries":{"1/nginx":{"acr_registry_id":1,"namespace":"ns","repository_name":"nginx","tags":["t1"],"fetched_at":"2026-10-01T00:00:00Z"}}}`), 0o600)
		c := loadTagCache()
		e, ok := c.Entries["1/nginx"]
		if !ok || len(e.Tags) != 1 || e.Tags[0] != "t1" {
			t.Errorf("读取结果 = %+v", c.Entries)
		}
	})
}

func TestSaveTagCacheRoundtrip(t *testing.T) {
	isolateGlobals(t)
	dir := t.TempDir()
	configDirOverride = dir

	cache := &tagCache{Entries: map[string]tagCacheEntry{
		"1/nginx": {AcrRegistryID: 1, Namespace: "ns", RepositoryName: "nginx",
			Tags: []string{"1.25"}, FetchedAt: time.Now()},
	}}
	if err := saveTagCache(cache); err != nil {
		t.Fatalf("saveTagCache: %v", err)
	}
	loaded := loadTagCache()
	if len(loaded.Entries) != 1 || loaded.Entries["1/nginx"].RepositoryName != "nginx" {
		t.Errorf("回读 = %+v", loaded.Entries)
	}
}

func TestEnsureTagCacheFresh(t *testing.T) {
	isolateGlobals(t)
	dir := t.TempDir()
	configDirOverride = dir

	reg := AcrRegistryInfo{ID: 1, Namespace: "ns", Alias: "ali", RegistryURL: "r"}

	t.Run("全量拉取并落盘", func(t *testing.T) {
		srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			switch r.URL.Path {
			case "/api/v1/acr-repositories":
				respondJSON(w, `{"status":"success","data":[
					{"id":11,"acr_registry_id":1,"repository_name":"nginx","created_at":"2026-10-01T00:00:00Z"},
					{"id":12,"acr_registry_id":1,"repository_name":"redis","created_at":"2026-10-01T00:00:00Z"}
				]}`)
			case "/api/v1/acr-tags":
				respondJSON(w, `{"status":"success","data":{"tags":["1.25"],"total":1}}`)
			}
		}))
		defer srv.Close()

		refreshed, failed, err := ensureTagCacheFresh(newTestClient(srv.URL), []AcrRegistryInfo{reg}, false)
		if err != nil || failed != 0 || refreshed != 2 {
			t.Fatalf("refreshed=%d failed=%d err=%v", refreshed, failed, err)
		}
		loaded := loadTagCache()
		if _, ok := loaded.Entries["1/nginx"]; !ok {
			t.Errorf("缓存未落盘: %+v", loaded.Entries)
		}
	})

	t.Run("缓存新鲜时跳过", func(t *testing.T) {
		cache := &tagCache{Entries: map[string]tagCacheEntry{
			"1/nginx": {AcrRegistryID: 1, Namespace: "ns", RepositoryName: "nginx",
				Tags: []string{"1.25"}, FetchedAt: time.Now()},
		}}
		if err := saveTagCache(cache); err != nil {
			t.Fatal(err)
		}
		// tags 接口 500：若缓存未生效就会重拉并计入 failed，据此证明跳过逻辑
		srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			switch r.URL.Path {
			case "/api/v1/acr-repositories":
				respondJSON(w, `{"status":"success","data":[{"id":11,"acr_registry_id":1,"repository_name":"nginx","created_at":"2026-10-01T00:00:00Z"}]}`)
			default:
				w.WriteHeader(http.StatusInternalServerError)
			}
		}))
		defer srv.Close()

		refreshed, failed, err := ensureTagCacheFresh(newTestClient(srv.URL), []AcrRegistryInfo{reg}, false)
		if err != nil {
			t.Fatal(err)
		}
		if refreshed != 0 || failed != 0 {
			t.Errorf("新鲜缓存不应重拉, refreshed=%d failed=%d", refreshed, failed)
		}
		// namespace 以 ACR 配置为准同步
		loaded := loadTagCache()
		if loaded.Entries["1/nginx"].Namespace != "ns" {
			t.Errorf("namespace 未同步: %+v", loaded.Entries["1/nginx"])
		}
	})

	t.Run("force 强制重拉", func(t *testing.T) {
		cache := &tagCache{Entries: map[string]tagCacheEntry{
			"1/nginx": {AcrRegistryID: 1, Namespace: "old", RepositoryName: "nginx",
				Tags: []string{"1.25"}, FetchedAt: time.Now()},
		}}
		if err := saveTagCache(cache); err != nil {
			t.Fatal(err)
		}
		srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			switch r.URL.Path {
			case "/api/v1/acr-repositories":
				respondJSON(w, `{"status":"success","data":[{"id":11,"acr_registry_id":1,"repository_name":"nginx","created_at":"2026-10-01T00:00:00Z"}]}`)
			case "/api/v1/acr-tags":
				respondJSON(w, `{"status":"success","data":{"tags":["9.9"],"total":1}}`)
			}
		}))
		defer srv.Close()

		refreshed, failed, err := ensureTagCacheFresh(newTestClient(srv.URL), []AcrRegistryInfo{reg}, true)
		if err != nil || failed != 0 || refreshed != 1 {
			t.Fatalf("refreshed=%d failed=%d err=%v", refreshed, failed, err)
		}
		if tags := loadTagCache().Entries["1/nginx"].Tags; len(tags) != 1 || tags[0] != "9.9" {
			t.Errorf("force 后 tags = %v", tags)
		}
	})

	t.Run("部分仓库拉取失败计入 failed", func(t *testing.T) {
		srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			switch r.URL.Path {
			case "/api/v1/acr-repositories":
				respondJSON(w, `{"status":"success","data":[
					{"id":11,"acr_registry_id":1,"repository_name":"nginx","created_at":"2026-10-01T00:00:00Z"},
					{"id":12,"acr_registry_id":1,"repository_name":"redis","created_at":"2026-10-01T00:00:00Z"}
				]}`)
			case "/api/v1/acr-tags":
				if r.URL.Query().Get("repository_name") == "redis" {
					w.WriteHeader(http.StatusInternalServerError)
					return
				}
				respondJSON(w, `{"status":"success","data":{"tags":["1.25"],"total":1}}`)
			}
		}))
		defer srv.Close()

		refreshed, failed, err := ensureTagCacheFresh(newTestClient(srv.URL), []AcrRegistryInfo{reg}, true)
		if err != nil {
			t.Fatal(err)
		}
		if refreshed != 1 || failed != 1 {
			t.Errorf("refreshed=%d failed=%d, 期望 1/1", refreshed, failed)
		}
	})

	t.Run("仓库列表获取失败报错", func(t *testing.T) {
		srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			w.WriteHeader(http.StatusInternalServerError)
		}))
		defer srv.Close()
		_, _, err := ensureTagCacheFresh(newTestClient(srv.URL), []AcrRegistryInfo{reg}, false)
		if err == nil || !strings.Contains(err.Error(), "获取 ns 的仓库列表失败") {
			t.Fatalf("期望仓库列表错误, got %v", err)
		}
	})

	t.Run("缓存写盘失败仅告警不报错", func(t *testing.T) {
		file := filepath.Join(t.TempDir(), "a-file")
		os.WriteFile(file, []byte("x"), 0o600)
		configDirOverride = file // configDir 指向文件 → tag-cache.json 写入失败

		srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			switch r.URL.Path {
			case "/api/v1/acr-repositories":
				respondJSON(w, `{"status":"success","data":[{"id":11,"acr_registry_id":1,"repository_name":"nginx","created_at":"2026-10-01T00:00:00Z"}]}`)
			case "/api/v1/acr-tags":
				respondJSON(w, `{"status":"success","data":{"tags":["1.25"],"total":1}}`)
			}
		}))
		defer srv.Close()

		refreshed, failed, err := ensureTagCacheFresh(newTestClient(srv.URL), []AcrRegistryInfo{reg}, false)
		if err != nil {
			t.Fatalf("写盘失败不应导致错误: %v", err)
		}
		if refreshed != 1 || failed != 0 {
			t.Errorf("refreshed=%d failed=%d", refreshed, failed)
		}
	})
}
