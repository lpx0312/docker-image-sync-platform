package main

import (
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

// mockPlatform 用 httptest 模拟平台后端，覆盖 CLI 用到的全部接口，
// 用于端到端验证 CLI 的登录、查询、查重、提交与结果渲染链路。
func mockPlatform(t *testing.T) *httptest.Server {
	t.Helper()
	mux := http.NewServeMux()

	writeJSON := func(w http.ResponseWriter, v any) {
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(v)
	}

	mux.HandleFunc("/api/v1/auth/login", func(w http.ResponseWriter, r *http.Request) {
		writeJSON(w, map[string]any{
			"token":      "mock-token",
			"user":       map[string]any{"username": "admin", "role_name": "管理员"},
			"expires_at": time.Now().Add(24 * time.Hour),
		})
	})
	mux.HandleFunc("/api/v1/auth/me", func(w http.ResponseWriter, r *http.Request) {
		writeJSON(w, map[string]any{"username": "admin", "role_name": "管理员", "permissions": []string{"sync", "images"}})
	})
	mux.HandleFunc("/api/v1/acr-registries", func(w http.ResponseWriter, r *http.Request) {
		writeJSON(w, map[string]any{"status": "success", "data": []map[string]any{
			{"id": 1, "registry_url": "registry.cn-hangzhou.aliyuncs.com", "namespace": "ns-a", "username": "u", "is_default": true},
			{"id": 2, "registry_url": "registry.cn-beijing.aliyuncs.com", "namespace": "ns-b", "username": "u", "is_default": false},
		}})
	})
	mux.HandleFunc("/api/v1/acr-registries/quota-summary", func(w http.ResponseWriter, r *http.Request) {
		writeJSON(w, map[string]any{"status": "success", "data": []map[string]any{
			{"acr_registry_id": 1, "namespace": "ns-a", "registry_url": "registry.cn-hangzhou.aliyuncs.com",
				"repo_count": 10, "repo_quota": 300, "remaining_quota": 290, "is_default": true, "is_full": false},
		}})
	})
	mux.HandleFunc("/api/v1/acr-repositories", func(w http.ResponseWriter, r *http.Request) {
		writeJSON(w, map[string]any{"status": "success", "data": []map[string]any{
			{"id": 1, "acr_registry_id": 1, "repository_name": "nginx", "created_at": time.Now()},
			{"id": 2, "acr_registry_id": 1, "repository_name": "redis", "created_at": time.Now()},
		}})
	})
	mux.HandleFunc("/api/v1/acr-tags", func(w http.ResponseWriter, r *http.Request) {
		writeJSON(w, map[string]any{"status": "success", "data": map[string]any{
			"tags": []string{"1.24", "1.25"}, "total": 2,
		}})
	})
	mux.HandleFunc("/api/v1/sync/suggest-acr", func(w http.ResponseWriter, r *http.Request) {
		writeJSON(w, map[string]any{"status": "success", "data": map[string]any{
			"affinity": map[string]any{
				"repository_name": "nginx", "has_affinity": true,
				"acr_registry_id": 1, "acr_namespace": "ns-a",
				"registry_url": "registry.cn-hangzhou.aliyuncs.com", "source": "repository",
			},
			"suggested_acr_id": 1, "suggested_namespace": "ns-a", "suggestion_reason": "affinity",
			"quota_summary": []map[string]any{},
		}})
	})
	mux.HandleFunc("/api/v1/sync/check-acr", func(w http.ResponseWriter, r *http.Request) {
		writeJSON(w, map[string]any{"status": "success", "data": map[string]any{
			"items": []map[string]any{{
				"image": "nginx", "repository_name": "nginx",
				"has_affinity": true, "has_conflict": false,
				"suggested_acr_id": 1, "suggested_namespace": "ns-a",
				"is_new_repository": false,
			}},
			"conflicts":            []map[string]any{},
			"new_repository_count": 0, "has_any_conflict": false, "multi_acr_warning": false,
		}})
	})
	mux.HandleFunc("/api/v1/sync/submit", func(w http.ResponseWriter, r *http.Request) {
		writeJSON(w, map[string]any{
			"task_id": "smoke-task-1", "status": "pending", "total_images": 1,
			"estimated_completion": "2026-08-16 12:00:00", "message": "同步任务已提交，正在处理中",
		})
	})
	mux.HandleFunc("/api/v1/sync/status/", func(w http.ResponseWriter, r *http.Request) {
		writeJSON(w, map[string]any{
			"task_id": "smoke-task-1", "status": "completed", "total_images": 1,
			"completed_images": 1, "failed_images": 0, "progress": 100,
			"github_action_url": "https://github.com/example/run/1",
			"images": map[string]any{
				"pending": 0, "syncing": 0, "success": 1, "failed": 0,
				"records": []map[string]any{{
					"id": 1, "original_image": "nginx", "tag": "1.26",
					"acr_image":   "registry.cn-hangzhou.aliyuncs.com/ns-a/nginx:1.26",
					"sync_status": "success",
				}},
			},
		})
	})
	mux.HandleFunc("/api/v1/sync/history", func(w http.ResponseWriter, r *http.Request) {
		writeJSON(w, map[string]any{
			"total": 1, "page": 1, "page_size": 20,
			"data": []map[string]any{{
				"task_id": "smoke-task-1", "status": "completed",
				"total_images": 1, "completed_images": 1, "failed_images": 0, "progress": 100,
				"created_at": time.Now(),
			}},
		})
	})

	mux.HandleFunc("/api/v1/images/list", func(w http.ResponseWriter, r *http.Request) {
		writeJSON(w, map[string]any{
			"total": 1, "page": 1, "page_size": 20,
			"data": []map[string]any{{
				"id": 1, "original_image": "nginx", "tag": "1.26",
				"acr_image": "registry.cn-hangzhou.aliyuncs.com/ns-a/nginx:1.26",
				"sync_status": "success", "task_id": "smoke-task-1",
				"created_at": time.Now(),
			}},
		})
	})
	mux.HandleFunc("/api/v1/images/stats", func(w http.ResponseWriter, r *http.Request) {
		writeJSON(w, map[string]any{"total": 10, "pending": 1, "syncing": 2, "success": 6, "failed": 1})
	})
	mux.HandleFunc("/api/v1/images/", func(w http.ResponseWriter, r *http.Request) {
		switch r.Method {
		case "DELETE":
			writeJSON(w, map[string]any{"message": "镜像记录已删除"})
		case "POST":
			writeJSON(w, map[string]any{
				"exists":            true,
				"target_image":      "registry.cn-hangzhou.aliyuncs.com/ns-a/nginx:1.26",
				"architectures":     []string{"amd64"},
				"acr_architectures": "[\"amd64\"]",
				"message":           "镜像存在，状态已更新为成功",
			})
		}
	})
	mux.HandleFunc("/api/v1/acr-tags/detail", func(w http.ResponseWriter, r *http.Request) {
		writeJSON(w, map[string]any{"status": "success", "data": map[string]any{
			"tag":           r.URL.Query().Get("tag"),
			"architectures": []string{"amd64", "arm64"},
			"digests":       map[string]string{"amd64": "sha256:aaa", "arm64": "sha256:bbb"},
			"sizes":         map[string]int64{"amd64": 12345678, "arm64": 22345678},
			"pushed_at":     map[string]string{"amd64": "2026-08-20T10:00:00Z", "arm64": "2026-08-20T10:00:00Z"},
		}})
	})
	mux.HandleFunc("/api/v1/auth/password", func(w http.ResponseWriter, r *http.Request) {
		writeJSON(w, map[string]any{"message": "密码修改成功，请重新登录"})
	})

	return httptest.NewServer(mux)
}

// runCLI 在进程内执行 dsync 子命令并捕获 stdout。
// 通过 DSYNC_PASSWORD 提供登录密码，避免测试环境无法交互输入。
func runCLI(t *testing.T, cfgFile string, args ...string) (stdout string, err error) {
	t.Helper()
	cfgPath = cfgFile
	jsonOut = false

	old := os.Stdout
	r, wr, err := os.Pipe()
	if err != nil {
		t.Fatal(err)
	}
	os.Stdout = wr
	done := make(chan string, 1)
	go func() {
		data, _ := io.ReadAll(r)
		done <- string(data)
	}()

	rootCmd.SetArgs(append([]string{"--config", cfgFile}, args...))
	err = rootCmd.Execute()

	wr.Close()
	os.Stdout = old
	return <-done, err
}

func TestSmokeEndToEnd(t *testing.T) {
	if testing.Short() {
		t.Skip("跳过端到端冒烟")
	}

	server := mockPlatform(t)
	defer server.Close()

	// 隔离配置与缓存目录，避免污染真实 ~/.config/dsync
	tmpDir := t.TempDir()
	configDirOverride = tmpDir
	cfgFile := filepath.Join(tmpDir, "config.json")
	t.Setenv("DSYNC_PASSWORD", "mock-pass")
	t.Setenv("DSYNC_OLD_PASSWORD", "mock-pass")
	t.Setenv("DSYNC_NEW_PASSWORD", "NewPass123!")

	steps := []struct {
		name    string
		args    []string
		wantErr string // 非空时期望命令报错且错误信息包含该串
		wantOut []string
	}{
		{
			name:    "login",
			args:    []string{"login", "--server", server.URL, "--username", "admin"},
			wantOut: []string{"登录成功", "admin"},
		},
		{
			name:    "acr list",
			args:    []string{"acr", "list"},
			wantOut: []string{"ns-a", "ns-b", "10/300"},
		},
		{
			name:    "repo list",
			args:    []string{"repo", "list"},
			wantOut: []string{"nginx", "redis"},
		},
		{
			name:    "tag list",
			args:    []string{"tag", "list", "nginx"},
			wantOut: []string{"1.24", "1.25"},
		},
		{
			name:    "suggest",
			args:    []string{"suggest", "nginx:1.26"},
			wantOut: []string{"ns-a", "仓库已有归属"},
		},
		{
			name:    "check",
			args:    []string{"check", "nginx:1.26"},
			wantOut: []string{"已归属"},
		},
		{
			// 目标 ACR 已存在 nginx:1.25，应被查重拦截
			name:    "sync 被查重拦截",
			args:    []string{"sync", "nginx:1.25"},
			wantErr: "已拦截",
		},
		{
			// 新 tag 放行，提交并等待完成，输出目标镜像地址
			name:    "sync 新 tag 完整链路",
			args:    []string{"sync", "nginx:1.26"},
			wantOut: []string{"registry.cn-hangzhou.aliyuncs.com/ns-a/nginx:1.26"},
		},
		{
			name:    "task list",
			args:    []string{"task", "list", "--long"},
			wantOut: []string{"smoke-task-1"},
		},
		{
			name:    "task status",
			args:    []string{"task", "status", "smoke-task-1"},
			wantOut: []string{"registry.cn-hangzhou.aliyuncs.com/ns-a/nginx:1.26"},
		},
		{
			name:    "search 命中 tag 缓存",
			args:    []string{"search", "1.25"},
			wantOut: []string{"nginx", "1.25"},
		},
		{
			name:    "image list",
			args:    []string{"image", "list", "--long"},
			wantOut: []string{"nginx", "1.26", "smoke-task-1"},
		},
		{
			name:    "image stats",
			args:    []string{"image", "stats"},
			wantOut: []string{"总计", "成功"},
		},
		{
			name:    "image check",
			args:    []string{"image", "check", "1"},
			wantOut: []string{"nginx:1.26", "存在"},
		},
		{
			name:    "image delete",
			args:    []string{"image", "delete", "1", "--yes"},
			wantOut: []string{"已删除"},
		},
		{
			name:    "tag detail",
			args:    []string{"tag", "detail", "nginx", "1.25"},
			wantOut: []string{"amd64", "arm64"},
		},
		{
			name:    "passwd",
			args:    []string{"passwd"},
			wantOut: []string{"密码修改成功"},
		},
		{
			// passwd 清除了 token，重新登录恢复，验证改密后仍能登录
			name:    "relogin after passwd",
			args:    []string{"login", "--server", server.URL, "--username", "admin"},
			wantOut: []string{"登录成功", "admin"},
		},
	}

	for _, step := range steps {
		t.Run(step.name, func(t *testing.T) {
			out, err := runCLI(t, cfgFile, step.args...)
			if step.wantErr != "" {
				if err == nil {
					t.Fatalf("期望报错但成功了，输出:\n%s", out)
				}
				if !strings.Contains(err.Error(), step.wantErr) {
					t.Fatalf("错误信息 %q 未包含 %q", err.Error(), step.wantErr)
				}
				return
			}
			if err != nil {
				t.Fatalf("报错: %v", err)
			}
			for _, want := range step.wantOut {
				if !strings.Contains(out, want) {
					t.Errorf("输出未包含 %q:\n%s", want, out)
				}
			}
		})
	}

	// 验证凭据落盘且权限为 0600
	info, err := os.Stat(cfgFile)
	if err != nil {
		t.Fatalf("配置文件未生成: %v", err)
	}
	if perm := info.Mode().Perm(); perm != 0o600 {
		t.Errorf("配置文件权限 = %o, 期望 0600", perm)
	}
	data, _ := os.ReadFile(cfgFile)
	if !strings.Contains(string(data), "mock-token") {
		t.Error("配置文件应包含 token")
	}
}
