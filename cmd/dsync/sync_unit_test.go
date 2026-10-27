package main

// cmd_sync.go / cmd_task.go / cmd_batch.go 中可直调函数的单元测试。

import (
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"docker-image-sync-platform/internal/models"
)

func TestIsTerminalTaskStatus(t *testing.T) {
	for _, s := range []string{"completed", "failed", "partial_success", "paused"} {
		if !isTerminalTaskStatus(s) {
			t.Errorf("%q 应为终态", s)
		}
	}
	for _, s := range []string{"pending", "running", "syncing", ""} {
		if isTerminalTaskStatus(s) {
			t.Errorf("%q 不应为终态", s)
		}
	}
}

func TestPrintTaskResult(t *testing.T) {
	st := &syncStatusResponse{
		TaskID: "task-123456789", Status: "completed", TotalImages: 4,
		GitHubActionURL: "https://github.com/x/run/1", ErrorMessage: "",
		Images: struct {
			Pending int                      `json:"pending"`
			Syncing int                      `json:"syncing"`
			Success int                      `json:"success"`
			Failed  int                      `json:"failed"`
			Records []models.ImageSyncRecord `json:"records"`
		}{
			Success: 2, Failed: 1,
			Records: []models.ImageSyncRecord{
				{ID: 1, OriginalImage: "nginx", Tag: "1.25", ACRImage: "reg/ns/nginx:1.25", SyncStatus: "success"},
				{ID: 2, OriginalImage: "redis", Tag: "7", ACRImage: "", SyncStatus: "success"},       // 地址未回填
				{ID: 3, OriginalImage: "busybox", Tag: "1", SyncStatus: "failed", ErrorMessage: "拉取超时"},
				{ID: 4, OriginalImage: "old", Tag: "1", SyncStatus: "skipped"},
				{ID: 5, OriginalImage: "new", Tag: "1", SyncStatus: "pending"},
			},
		},
	}
	out := captureStdout(t, func() { printTaskResult(st) })
	for _, want := range []string{
		"task-123", "completed", // shortID（前 8 位）+ 状态
		"reg/ns/nginx:1.25",
		"(目标地址暂未回填)",
		"拉取超时",
		"已跳过",
		"pending",
		"https://github.com/x/run/1",
	} {
		if !strings.Contains(out, want) {
			t.Errorf("输出未包含 %q:\n%s", want, out)
		}
	}

	t.Run("任务级错误", func(t *testing.T) {
		stErr := &syncStatusResponse{TaskID: "t", Status: "failed", ErrorMessage: "Actions 执行失败"}
		out := captureStdout(t, func() { printTaskResult(stErr) })
		if !strings.Contains(out, "任务错误: Actions 执行失败") {
			t.Errorf("输出 = %q", out)
		}
	})
}

func TestRenderProgressLine(t *testing.T) {
	st := &syncStatusResponse{TaskID: "t", Status: "syncing", Progress: 42.5}
	// 渲染到 stderr，只验证不 panic 与基本结构
	renderProgressLine(st, 90*time.Second)
}

func TestGetSyncStatus(t *testing.T) {
	f := newFakePlatform(t)
	st, err := getSyncStatus(newTestClient(f.Server.URL), "task-e2e-1")
	if err != nil {
		t.Fatal(err)
	}
	if st.TaskID != "task-e2e-1" || st.Status != "completed" {
		t.Errorf("st = %+v", st)
	}

	t.Run("接口错误", func(t *testing.T) {
		srv := newErrServer()
		defer srv.Close()
		if _, err := getSyncStatus(newTestClient(srv.URL), "t"); err == nil {
			t.Error("应报错")
		}
	})
}

func TestWaitForTaskSingleQuery(t *testing.T) {
	// poll=false：单次查询即返回，且不渲染进度行
	f := newFakePlatform(t)
	st, err := waitForTask(newTestClient(f.Server.URL), "task-e2e-1", false)
	if err != nil {
		t.Fatal(err)
	}
	if st.Status != "completed" {
		t.Errorf("status = %q", st.Status)
	}
}

func TestWaitForTaskPolling(t *testing.T) {
	if testing.Short() {
		t.Skip("跳过 5s 轮询等待")
	}
	f := newFakePlatform(t)
	f.statusSeq = []string{"running", "completed"} // 第一次 running → 等待 ticker(5s) → completed

	start := time.Now()
	st, err := waitForTask(newTestClient(f.Server.URL), "task-e2e-1", true)
	if err != nil {
		t.Fatal(err)
	}
	if st.Status != "completed" {
		t.Errorf("status = %q", st.Status)
	}
	if elapsed := time.Since(start); elapsed < 5*time.Second {
		t.Errorf("轮询间隔应至少一个 ticker 周期, got %v", elapsed)
	}
}

func TestWaitForTaskQueryError(t *testing.T) {
	srv := newErrServer()
	defer srv.Close()
	if _, err := waitForTask(newTestClient(srv.URL), "t", false); err == nil {
		t.Error("状态查询失败应报错")
	}
}

func TestRunDedupCheck(t *testing.T) {
	reg := &AcrRegistryInfo{ID: 1, Namespace: "ns-a", Alias: "ali", RegistryURL: "registry.fake-a.com"}

	t.Run("已有归属且 tag 新", func(t *testing.T) {
		f := newFakePlatform(t)
		outcome, err := runDedupCheck(newTestClient(f.Server.URL), "nginx:1.99", "", reg)
		if err != nil {
			t.Fatal(err)
		}
		if outcome.Action != "info" {
			t.Errorf("action = %q (%s)", outcome.Action, outcome.Message)
		}
	})
	t.Run("target-tag 参与 tag 比对", func(t *testing.T) {
		f := newFakePlatform(t) // nginx 已有 1.24/1.25
		outcome, err := runDedupCheck(newTestClient(f.Server.URL), "nginx:1.25", "1.24", reg)
		if err != nil {
			t.Fatal(err)
		}
		// 源 tag 1.25，但目标 tag 显式指定为已存在的 1.24 → 应拦截
		if outcome.Action != "block" {
			t.Errorf("action = %q (%s)", outcome.Action, outcome.Message)
		}
	})
	t.Run("check-acr 接口失败", func(t *testing.T) {
		srv := newErrServer()
		defer srv.Close()
		_, err := runDedupCheck(newTestClient(srv.URL), "nginx", "", reg)
		if err == nil || !strings.Contains(err.Error(), "查重请求失败") {
			t.Fatalf("期望查重请求失败, got %v", err)
		}
	})
	t.Run("查重响应为空", func(t *testing.T) {
		srv := newJSONServer(`{"status":"success","data":{"items":[],"conflicts":[],"new_repository_count":0,"has_any_conflict":false,"multi_acr_warning":false}}`)
		defer srv.Close()
		_, err := runDedupCheck(newTestClient(srv.URL), "nginx", "", reg)
		if err == nil || !strings.Contains(err.Error(), "查重响应为空") {
			t.Fatalf("期望空响应报错, got %v", err)
		}
	})
	t.Run("归属其他 ACR 时不再拉取 tag", func(t *testing.T) {
		srv := newJSONServer(`{"status":"success","data":{"items":[{` +
			`"image":"nginx","repository_name":"nginx","has_affinity":true,` +
			`"suggested_acr_id":99,"suggested_namespace":"other"}],` +
			`"conflicts":[],"new_repository_count":0,"has_any_conflict":false,"multi_acr_warning":false}}`)
		defer srv.Close()
		outcome, err := runDedupCheck(newTestClient(srv.URL), "nginx:1.25", "", reg)
		if err != nil {
			t.Fatal(err)
		}
		if outcome.Action != "block" || !strings.Contains(outcome.Message, "已归属") {
			t.Errorf("outcome = %+v", outcome)
		}
	})
	t.Run("tag 拉取失败时按未知处理", func(t *testing.T) {
		// 归属相同 ACR，但 /acr-tags 返回 500 → tagsKnown=false → info（Tag 校验跳过）
		srv := newJSONServerMux(map[string]http.HandlerFunc{
			"/api/v1/sync/check-acr": func(w http.ResponseWriter, r *http.Request) {
				respondJSON(w, `{"status":"success","data":{"items":[{"image":"nginx","repository_name":"nginx","has_affinity":true,"suggested_acr_id":1,"suggested_namespace":"ns-a"}],"conflicts":[],"new_repository_count":0,"has_any_conflict":false,"multi_acr_warning":false}}`)
			},
			"/api/v1/acr-tags": func(w http.ResponseWriter, r *http.Request) {
				w.WriteHeader(http.StatusInternalServerError)
			},
		})
		defer srv.Close()
		outcome, err := runDedupCheck(newTestClient(srv.URL), "nginx:1.25", "", reg)
		if err != nil {
			t.Fatal(err)
		}
		if outcome.Action != "info" || !strings.Contains(outcome.Message, "Tag 校验跳过") {
			t.Errorf("outcome = %+v", outcome)
		}
	})
}

func TestParseBatchFileEdges(t *testing.T) {
	t.Run("文件不存在", func(t *testing.T) {
		if _, err := parseBatchFile(filepath.Join(t.TempDir(), "nope.txt")); err == nil {
			t.Error("应报错")
		}
	})
	t.Run("只有注释与空行", func(t *testing.T) {
		p := filepath.Join(t.TempDir(), "e.txt")
		os.WriteFile(p, []byte("\n  \n# c\n"), 0o600)
		if _, err := parseBatchFile(p); err == nil || !strings.Contains(err.Error(), "没有有效镜像") {
			t.Errorf("期望没有有效镜像错误, got %v", err)
		}
	})
	t.Run("超长行触发 scanner 错误", func(t *testing.T) {
		p := filepath.Join(t.TempDir(), "long.txt")
		longLine := strings.Repeat("a", 100*1024) // 超过 bufio.Scanner 默认 64KB 上限
		os.WriteFile(p, []byte(longLine), 0o600)
		if _, err := parseBatchFile(p); err == nil || !strings.Contains(err.Error(), "读取文件失败") {
			t.Errorf("期望 scanner 错误, got %v", err)
		}
	})
}
