package main

// 共享测试基建：
// - execCLI：进程内执行 rootCmd 子命令，捕获 stdout/stderr，隔离全局状态
//   （cfgPath/jsonOut/cfg/configDirOverride），并递归重置所有 flag。
//   注意：cobra 的 FlagSet 值在多次 Execute 之间会残留（pflag 不会把未出现的
//   flag 重置回默认值），因此每次执行前必须显式重置，否则用例间会互相污染。
// - fakePlatform：可按需覆写 handler 的 httptest 平台模拟，带请求录制。

import (
	"bytes"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"path/filepath"
	"sync"
	"testing"
	"time"

	"github.com/spf13/cobra"
	"github.com/spf13/pflag"
)

// resetAllFlags 递归把命令树中所有 flag 的值重置回默认值。
func resetAllFlags(c *cobra.Command) {
	c.Flags().VisitAll(func(f *pflag.Flag) { _ = f.Value.Set(f.DefValue) })
	c.PersistentFlags().VisitAll(func(f *pflag.Flag) { _ = f.Value.Set(f.DefValue) })
	for _, sub := range c.Commands() {
		resetAllFlags(sub)
	}
}

// execCLI 在进程内执行 dsync 子命令并捕获 stdout/stderr。
// dir 用作隔离的配置目录（configDirOverride，tag 缓存也落在这里），
// cfgFile 为配置文件路径。恢复全局状态由 t.Cleanup 保证。
func execCLI(t *testing.T, dir, cfgFile string, args ...string) (stdout, stderr string, err error) {
	t.Helper()

	// 保存全局状态
	oldCfgPath, oldJSONOut, oldCfg, oldDir := cfgPath, jsonOut, cfg, configDirOverride
	t.Cleanup(func() {
		cfgPath, jsonOut, cfg, configDirOverride = oldCfgPath, oldJSONOut, oldCfg, oldDir
	})
	cfgPath = cfgFile
	jsonOut = false
	configDirOverride = dir
	resetAllFlags(rootCmd)

	// 捕获 stdout 与 stderr
	outR, outW, err := os.Pipe()
	if err != nil {
		t.Fatal(err)
	}
	errR, errW, err2 := os.Pipe()
	if err != nil {
		t.Fatal(err)
	}
	if err2 != nil {
		t.Fatal(err2)
	}
	oldStdout, oldStderr := os.Stdout, os.Stderr
	os.Stdout, os.Stderr = outW, errW

	outCh, errCh := make(chan string, 1), make(chan string, 1)
	go func() { data, _ := io.ReadAll(outR); outCh <- string(data) }()
	go func() { data, _ := io.ReadAll(errR); errCh <- string(data) }()

	func() {
		defer func() {
			// 防止被测命令 panic 导致全局 fd 泄漏影响后续用例
			if r := recover(); r != nil {
				t.Errorf("命令 panic: %v", r)
			}
		}()
		rootCmd.SetArgs(append([]string{"--config", cfgFile}, args...))
		err = rootCmd.Execute()
	}()

	outW.Close()
	errW.Close()
	os.Stdout, os.Stderr = oldStdout, oldStderr
	stdout, stderr = <-outCh, <-errCh
	return stdout, stderr, err
}

// captureStdout 捕获直接调用打印函数时的 stdout。
func captureStdout(t *testing.T, fn func()) string {
	t.Helper()
	old := os.Stdout
	r, w, err := os.Pipe()
	if err != nil {
		t.Fatal(err)
	}
	os.Stdout = w
	done := make(chan string, 1)
	go func() { data, _ := io.ReadAll(r); done <- string(data) }()

	fn()

	w.Close()
	os.Stdout = old
	return <-done
}

// writeConfigFile 在指定路径写入配置文件。
func writeConfigFile(t *testing.T, path string, c *Config) {
	t.Helper()
	data, err := json.MarshalIndent(c, "", "  ")
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, data, 0o600); err != nil {
		t.Fatal(err)
	}
}

// 隔离配置目录：返回 (dir, cfgFile)，测试结束后无需手动恢复
// （execCLI 自己会保存/恢复 configDirOverride）。
func isolatedConfigPaths(t *testing.T) (string, string) {
	t.Helper()
	dir := t.TempDir()
	return dir, filepath.Join(dir, "config.json")
}

// respondJSON 以 JSON Content-Type 输出响应体。
// resty 只在 Content-Type 为 JSON 时才把响应体反序列化进 Result，
// 测试服务端必须显式设置该头，否则 CLI 会拿到空结果。
func respondJSON(w http.ResponseWriter, body string) {
	w.Header().Set("Content-Type", "application/json")
	_, _ = io.WriteString(w, body)
}

// newErrServer 所有接口都返回 500 的服务器。
func newErrServer() *httptest.Server {
	return httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
	}))
}

// newJSONServer 对所有请求返回同一 JSON 响应体。
func newJSONServer(body string) *httptest.Server {
	return httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		respondJSON(w, body)
	}))
}

// newJSONServerMux 按路径分发 handler 的简单 JSON 服务器。
func newJSONServerMux(routes map[string]http.HandlerFunc) *httptest.Server {
	mux := http.NewServeMux()
	for p, h := range routes {
		mux.HandleFunc(p, h)
	}
	return httptest.NewServer(mux)
}

// reqRecord 录制到 fakePlatform 的请求
type reqRecord struct {
	Method string
	Path   string
	Query  url.Values
	Body   []byte
	Header http.Header
}

// fakePlatform 可覆写 handler 的平台模拟。
// 默认提供 CLI 涉及的全部接口（与真实平台响应格式一致），
// 用 Override 替换特定接口、Requests 检查 CLI 实际发出的请求。
type fakePlatform struct {
	*httptest.Server

	mu        sync.Mutex
	overrides map[string]http.HandlerFunc // key: "METHOD /api/v1/path"
	reqs      []reqRecord

	// 可变默认状态
	statusSeq []string // /sync/status/ 的状态序列，耗尽后用最后一个
	statusIdx int
	lastBody  map[string]any // 最近一次 /sync/submit 或 /sync/batch 请求体
}

// newFakePlatform 启动默认行为完整的模拟平台。
func newFakePlatform(t *testing.T) *fakePlatform {
	t.Helper()
	f := &fakePlatform{
		overrides: map[string]http.HandlerFunc{},
		statusSeq: []string{"completed"},
	}
	mux := http.NewServeMux()

	// 通用包装：录制请求 + 查找覆写。
	// 录制时会把 body 读空，需回填一个新 reader 供后续 handler 使用。
	wrap := func(method, pattern string, def http.HandlerFunc) {
		mux.HandleFunc(pattern, func(w http.ResponseWriter, r *http.Request) {
			f.mu.Lock()
			body, _ := io.ReadAll(r.Body)
			r.Body = io.NopCloser(bytes.NewReader(body))
			f.reqs = append(f.reqs, reqRecord{
				Method: r.Method, Path: r.URL.Path, Query: r.URL.Query(),
				Body: body, Header: r.Header.Clone(),
			})
			h, ok := f.overrides[method+" "+r.URL.Path]
			f.mu.Unlock()
			if ok {
				h(w, r)
				return
			}
			def(w, r)
		})
	}

	writeJSON := func(w http.ResponseWriter, v any) {
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(v)
	}

	// ---- auth ----
	wrap("POST", "/api/v1/auth/login", func(w http.ResponseWriter, r *http.Request) {
		writeJSON(w, map[string]any{
			"token":      "fake-token",
			"user":       map[string]any{"username": "admin", "role_name": "管理员"},
			"expires_at": time.Now().Add(24 * time.Hour),
		})
	})
	wrap("GET", "/api/v1/auth/me", func(w http.ResponseWriter, r *http.Request) {
		writeJSON(w, map[string]any{"username": "admin", "role_name": "管理员", "permissions": []string{"sync"}})
	})
	wrap("POST", "/api/v1/auth/logout", func(w http.ResponseWriter, r *http.Request) {
		writeJSON(w, map[string]any{"status": "success"})
	})
	wrap("PUT", "/api/v1/auth/password", func(w http.ResponseWriter, r *http.Request) {
		writeJSON(w, map[string]any{"status": "success", "message": "密码修改成功，请重新登录"})
	})

	// ---- acr ----
	wrap("GET", "/api/v1/acr-registries", func(w http.ResponseWriter, r *http.Request) {
		writeJSON(w, map[string]any{"status": "success", "data": []map[string]any{
			{"id": 1, "registry_url": "registry.fake-a.com", "namespace": "ns-a", "alias": "ali", "username": "u", "is_default": true, "registry_type": "acr"},
			{"id": 2, "registry_url": "registry.fake-b.com", "namespace": "ns-b", "alias": "", "username": "u", "is_default": false, "registry_type": ""},
		}})
	})
	wrap("GET", "/api/v1/acr-registries/quota-summary", func(w http.ResponseWriter, r *http.Request) {
		writeJSON(w, map[string]any{"status": "success", "data": []map[string]any{
			{"acr_registry_id": 1, "namespace": "ns-a", "alias": "ali", "registry_url": "registry.fake-a.com",
				"repo_count": 10, "repo_quota": 300, "remaining_quota": 290, "is_default": true, "is_full": false},
			{"acr_registry_id": 2, "namespace": "ns-b", "registry_url": "registry.fake-b.com",
				"repo_count": 5, "repo_quota": 0, "remaining_quota": 0, "is_full": true},
		}})
	})
	wrap("POST", "/api/v1/acr-registries/1/test", func(w http.ResponseWriter, r *http.Request) {
		writeJSON(w, map[string]any{"data": map[string]any{
			"registry_type": "acr", "login_ok": true, "login_message": "登录成功",
			"manage_ok": false, "manage_message": "", "manage_skipped": false,
		}})
	})
	wrap("POST", "/api/v1/acr-registries/2/test", func(w http.ResponseWriter, r *http.Request) {
		writeJSON(w, map[string]any{"data": map[string]any{
			"registry_type": "", "login_ok": true, "login_message": "登录成功",
			"manage_ok": true, "manage_message": "SWR 管理面可用", "manage_skipped": false,
		}})
	})

	// ---- repositories / tags ----
	wrap("GET", "/api/v1/acr-repositories", func(w http.ResponseWriter, r *http.Request) {
		byID := map[string][]map[string]any{
			"1": {
				{"id": 11, "acr_registry_id": 1, "repository_name": "nginx", "created_at": time.Now()},
				{"id": 12, "acr_registry_id": 1, "repository_name": "redis", "created_at": time.Now()},
			},
			"2": {
				{"id": 21, "acr_registry_id": 2, "repository_name": "shared", "created_at": time.Now()},
			},
		}
		data := byID[r.URL.Query().Get("acr_registry_id")]
		if data == nil {
			data = []map[string]any{}
		}
		writeJSON(w, map[string]any{"status": "success", "data": data})
	})
	wrap("GET", "/api/v1/acr-tags", func(w http.ResponseWriter, r *http.Request) {
		tags := map[string][]string{
			"nginx": {"1.24", "1.25"},
		}
		repo := r.URL.Query().Get("repository_name")
		list, ok := tags[repo]
		if !ok {
			list = []string{}
		}
		writeJSON(w, map[string]any{"status": "success", "data": map[string]any{"tags": list, "total": len(list)}})
	})
	wrap("GET", "/api/v1/acr-tags/detail", func(w http.ResponseWriter, r *http.Request) {
		writeJSON(w, map[string]any{"status": "success", "data": map[string]any{
			"tag":           r.URL.Query().Get("tag"),
			"architectures": []string{"amd64", "arm64"},
			"digests":       map[string]string{"amd64": "sha256:aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa", "arm64": "sha256:bbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbb"},
			"sizes":         map[string]int64{"amd64": 12345678, "arm64": 0},
			"pushed_at":     map[string]string{"amd64": "2026-08-20T10:00:00Z", "arm64": ""},
		}})
	})
	wrap("POST", "/api/v1/acr-repositories/import-from-registry", func(w http.ResponseWriter, r *http.Request) {
		writeJSON(w, map[string]any{"data": map[string]any{
			"created": 2, "already_exist": 1,
			"created_names": []string{"app1", "app2"}, "already_exist_names": []string{"nginx"},
		}})
	})
	wrap("POST", "/api/v1/acr-repositories/sync-from-records", func(w http.ResponseWriter, r *http.Request) {
		writeJSON(w, map[string]any{"data": map[string]any{
			"created": 1, "skipped": 2, "already_exist": 0,
			"created_names": []string{"app1"}, "missing_in_acr": []string{"x"}, "already_exist_names": []string{},
		}})
	})
	wrap("POST", "/api/v1/acr-repositories/clean-invalid", func(w http.ResponseWriter, r *http.Request) {
		writeJSON(w, map[string]any{"data": map[string]any{
			"cleaned": 2, "cleaned_names": []string{"old1", "old2"}, "check_failed_names": []string{"bad1"},
		}})
	})

	// ---- sync ----
	wrap("GET", "/api/v1/sync/suggest-acr", func(w http.ResponseWriter, r *http.Request) {
		writeJSON(w, map[string]any{"status": "success", "data": map[string]any{
			"affinity": map[string]any{
				"repository_name": "nginx", "has_affinity": true,
				"acr_registry_id": 1, "acr_namespace": "ns-a", "acr_alias": "ali",
				"registry_url": "registry.fake-a.com", "source": "repository",
			},
			"suggested_acr_id": 1, "suggested_namespace": "ns-a", "suggested_alias": "ali",
			"suggestion_reason": "affinity",
			"quota_summary":     []map[string]any{},
		}})
	})
	wrap("POST", "/api/v1/sync/check-acr", func(w http.ResponseWriter, r *http.Request) {
		writeJSON(w, map[string]any{"status": "success", "data": map[string]any{
			"items": []map[string]any{{
				"image": "nginx", "repository_name": "nginx",
				"has_affinity": true, "has_conflict": false,
				"suggested_acr_id": 1, "suggested_namespace": "ns-a", "suggested_alias": "ali",
				"is_new_repository": false,
			}},
			"conflicts":            []map[string]any{},
			"new_repository_count": 0, "has_any_conflict": false, "multi_acr_warning": false,
		}})
	})
	submitHandler := func(w http.ResponseWriter, r *http.Request) {
		writeJSON(w, map[string]any{
			"task_id": "task-e2e-1", "status": "pending", "total_images": 2,
			"estimated_completion": "2026-10-27 12:00:00", "message": "同步任务已提交，正在处理中",
		})
	}
	wrap("POST", "/api/v1/sync/submit", submitHandler)
	wrap("POST", "/api/v1/sync/batch", submitHandler)
	mux.HandleFunc("/api/v1/sync/status/", func(w http.ResponseWriter, r *http.Request) {
		f.mu.Lock()
		f.reqs = append(f.reqs, reqRecord{Method: r.Method, Path: r.URL.Path, Query: r.URL.Query()})
		st := "completed"
		if f.statusIdx < len(f.statusSeq) {
			st = f.statusSeq[f.statusIdx]
			f.statusIdx++
		} else if len(f.statusSeq) > 0 {
			st = f.statusSeq[len(f.statusSeq)-1]
		}
		h, ok := f.overrides["GET "+r.URL.Path]
		f.mu.Unlock()
		if ok {
			h(w, r)
			return
		}
		writeJSON(w, map[string]any{
			"task_id": "task-e2e-1", "status": st, "total_images": 1,
			"completed_images": 1, "failed_images": 0, "progress": 100,
			"github_action_url": "https://github.com/example/run/1",
			"images": map[string]any{
				"pending": 0, "syncing": 0, "success": 1, "failed": 0,
				"records": []map[string]any{{
					"id": 1, "original_image": "nginx", "tag": "1.26",
					"acr_image":   "registry.fake-a.com/ns-a/nginx:1.26",
					"sync_status": "success",
				}},
			},
		})
	})
	wrap("GET", "/api/v1/sync/history", func(w http.ResponseWriter, r *http.Request) {
		writeJSON(w, map[string]any{
			"total": 1, "page": 1, "page_size": 20,
			"data": []map[string]any{{
				"task_id": "task-e2e-1", "status": "completed",
				"total_images": 3, "completed_images": 2, "failed_images": 1, "progress": 66,
				"github_action_url": "https://github.com/example/run/1",
				"created_at":        time.Now(),
			}},
		})
	})

	// ---- images ----
	wrap("GET", "/api/v1/images/list", func(w http.ResponseWriter, r *http.Request) {
		writeJSON(w, map[string]any{
			"total": 1, "page": 1, "page_size": 20,
			"data": []map[string]any{{
				"id": 7, "original_image": "nginx", "tag": "1.26",
				"acr_image": "", "sync_status": "success", "task_id": "task-e2e-1",
				"created_at": time.Now(),
			}},
		})
	})
	wrap("GET", "/api/v1/images/stats", func(w http.ResponseWriter, r *http.Request) {
		writeJSON(w, map[string]any{"total": 10, "pending": 1, "syncing": 2, "success": 6, "failed": 1})
	})
	wrap("POST", "/api/v1/images/7/check", func(w http.ResponseWriter, r *http.Request) {
		writeJSON(w, map[string]any{
			"exists": false, "target_image": "registry.fake-a.com/ns-a/nginx:1.26",
			"architectures": []string{}, "acr_architectures": "",
			"message": "镜像不存在，状态已标记失败",
		})
	})
	wrap("POST", "/api/v1/images/7/retry", func(w http.ResponseWriter, r *http.Request) {
		writeJSON(w, map[string]any{"status": "success", "message": "重试已提交"})
	})
	// --task 批量重试用到的记录接口（go.mod 为 go1.21，ServeMux 不支持 {id} 通配，只能精确注册）
	wrap("POST", "/api/v1/images/31/retry", func(w http.ResponseWriter, r *http.Request) {
		writeJSON(w, map[string]any{"status": "success", "message": "重试已提交"})
	})
	wrap("POST", "/api/v1/images/32/retry", func(w http.ResponseWriter, r *http.Request) {
		writeJSON(w, map[string]any{"status": "success", "message": "重试已提交"})
	})
	wrap("DELETE", "/api/v1/images/7", func(w http.ResponseWriter, r *http.Request) {
		writeJSON(w, map[string]any{"message": "镜像记录已删除"})
	})

	f.Server = httptest.NewServer(mux)
	t.Cleanup(f.Server.Close)
	return f
}

// Override 覆写指定接口的处理函数。key 形如 "GET /api/v1/acr-registries"。
func (f *fakePlatform) Override(key string, h http.HandlerFunc) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.overrides[key] = h
}

// Requests 返回已录制的请求（全部）。
func (f *fakePlatform) Requests() []reqRecord {
	f.mu.Lock()
	defer f.mu.Unlock()
	out := make([]reqRecord, len(f.reqs))
	copy(out, f.reqs)
	return out
}

// RequestsFor 返回方法+路径匹配的请求。
func (f *fakePlatform) RequestsFor(method, path string) []reqRecord {
	var out []reqRecord
	for _, r := range f.Requests() {
		if r.Method == method && r.Path == path {
			out = append(out, r)
		}
	}
	return out
}

// HasRequest 判断是否发出过指定请求。
func (f *fakePlatform) HasRequest(method, path string) bool {
	return len(f.RequestsFor(method, path)) > 0
}

// loggedInConfig 返回指向 fake 平台、已登录的配置。
func (f *fakePlatform) loggedInConfig() *Config {
	return &Config{
		Server:   f.Server.URL,
		Username: "admin",
		Password: "pass",
		Token:    "fake-token",
	}
}

// setupLoggedIn 组装 fake 平台 + 隔离配置目录 + 已写好的配置文件。
func setupLoggedIn(t *testing.T) (*fakePlatform, string, string) {
	t.Helper()
	f := newFakePlatform(t)
	dir, cfgFile := isolatedConfigPaths(t)
	writeConfigFile(t, cfgFile, f.loggedInConfig())
	return f, dir, cfgFile
}
