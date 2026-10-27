package main

// sync / batch / task / retry 的 e2e 测试（cobra 整链路 + httptest 服务端）。

import (
	"encoding/json"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestSyncE2E(t *testing.T) {
	t.Run("默认链路-查重info后提交并等待完成", func(t *testing.T) {
		f, dir, cfgFile := setupLoggedIn(t)
		// 默认 check-acr：nginx 已归属 ACR1；默认 tags: 1.24/1.25 → 1.26 是新 tag → info 放行
		out, stderr, err := execCLI(t, dir, cfgFile, "sync", "nginx:1.26")
		if err != nil {
			t.Fatal(err)
		}
		if !strings.Contains(stderr, "目标镜像仓库") || !strings.Contains(stderr, "仓库已有归属") {
			t.Errorf("stderr = %q", stderr)
		}
		if !strings.Contains(out, "registry.fake-a.com/ns-a/nginx:1.26") {
			t.Errorf("输出 = %q", out)
		}
		// 无 --target-tag 时走单镜像接口
		if len(f.RequestsFor("POST", "/api/v1/sync/submit")) != 1 {
			t.Errorf("应调用 /sync/submit 一次, got %d", len(f.RequestsFor("POST", "/api/v1/sync/submit")))
		}
		submits := f.RequestsFor("POST", "/api/v1/sync/submit")
		var body submitRequest
		if err := json.Unmarshal(submits[0].Body, &body); err != nil {
			t.Fatal(err)
		}
		if len(body.Images) != 1 || body.Images[0] != "nginx:1.26" {
			t.Errorf("submit body = %+v", body)
		}
	})

	t.Run("同tag已存在被拦截", func(t *testing.T) {
		f, dir, cfgFile := setupLoggedIn(t)
		_, _, err := execCLI(t, dir, cfgFile, "sync", "nginx:1.25")
		if err == nil || !strings.Contains(err.Error(), "已拦截") {
			t.Fatalf("期望查重拦截, got %v", err)
		}
		// 拦截后不应提交
		if len(f.RequestsFor("POST", "/api/v1/sync/submit")) != 0 {
			t.Error("被拦截时不应提交任务")
		}
	})

	t.Run("force 跳过查重", func(t *testing.T) {
		f, dir, cfgFile := setupLoggedIn(t)
		if _, _, err := execCLI(t, dir, cfgFile, "sync", "nginx:1.25", "--force"); err != nil {
			t.Fatal(err)
		}
		if len(f.RequestsFor("POST", "/api/v1/sync/check-acr")) != 0 {
			t.Error("--force 不应触发查重")
		}
		if len(f.RequestsFor("POST", "/api/v1/sync/submit")) != 1 {
			t.Error("应直接提交")
		}
	})

	t.Run("target-tag 走 batch 接口", func(t *testing.T) {
		f, dir, cfgFile := setupLoggedIn(t)
		out, _, err := execCLI(t, dir, cfgFile, "sync", "nginx:1.25", "--target-tag", "1.25-amd64", "--arch", "amd64", "--desc", "测试")
		if err != nil {
			t.Fatal(err)
		}
		if !strings.Contains(out, "registry.fake-a.com/ns-a/nginx:1.26") {
			t.Errorf("输出 = %q", out)
		}
		batches := f.RequestsFor("POST", "/api/v1/sync/batch")
		if len(batches) != 1 {
			t.Fatalf("应调用 /sync/batch, got %d", len(batches))
		}
		var body batchRequest
		if err := json.Unmarshal(batches[0].Body, &body); err != nil {
			t.Fatal(err)
		}
		if len(body.Images) != 1 || body.Images[0].TargetTag != "1.25-amd64" || body.Images[0].Architecture != "amd64" {
			t.Errorf("batch body = %+v", body)
		}
	})

	t.Run("target-tag 与已有 tag 相同被拦截", func(t *testing.T) {
		_, dir, cfgFile := setupLoggedIn(t)
		_, _, err := execCLI(t, dir, cfgFile, "sync", "nginx:9.9", "--target-tag", "1.25")
		if err == nil || !strings.Contains(err.Error(), "已拦截") {
			t.Fatalf("期望按目标 tag 拦截, got %v", err)
		}
	})

	t.Run("--acr 指定别名", func(t *testing.T) {
		f, dir, cfgFile := setupLoggedIn(t)
		if _, _, err := execCLI(t, dir, cfgFile, "sync", "redis:7", "--acr", "ali"); err != nil {
			t.Fatal(err)
		}
		submits := f.RequestsFor("POST", "/api/v1/sync/submit")
		if len(submits) != 1 {
			t.Fatalf("应提交一次, got %d", len(submits))
		}
		var body submitRequest
		_ = json.Unmarshal(submits[0].Body, &body)
		if body.AcrRegistryID != 1 {
			t.Errorf("acr_registry_id = %d, 期望 1", body.AcrRegistryID)
		}
	})

	t.Run("--acr 别名不存在", func(t *testing.T) {
		_, dir, cfgFile := setupLoggedIn(t)
		_, _, err := execCLI(t, dir, cfgFile, "sync", "redis:7", "--acr", "nope")
		if err == nil || !strings.Contains(err.Error(), "未找到别名") {
			t.Fatalf("期望别名错误, got %v", err)
		}
	})

	t.Run("服务端推荐的 ACR 不存在", func(t *testing.T) {
		f, dir, cfgFile := setupLoggedIn(t)
		f.Override("GET /api/v1/sync/suggest-acr", func(w http.ResponseWriter, r *http.Request) {
			respondJSON(w, `{"status":"success","data":{"affinity":{"has_affinity":false},"suggested_acr_id":99,"suggested_namespace":"ns-x","suggestion_reason":"default","quota_summary":[]}}`)
		})
		_, _, err := execCLI(t, dir, cfgFile, "sync", "redis:7")
		if err == nil || !strings.Contains(err.Error(), "不存在") {
			t.Fatalf("期望推荐 ACR 不存在错误, got %v", err)
		}
	})

	t.Run("--no-wait 立即返回", func(t *testing.T) {
		f, dir, cfgFile := setupLoggedIn(t)
		out, _, err := execCLI(t, dir, cfgFile, "sync", "redis:7", "--no-wait", "--force")
		if err != nil {
			t.Fatal(err)
		}
		if !strings.Contains(out, "任务已提交") || !strings.Contains(out, "task-e2e-1") {
			t.Errorf("输出 = %q", out)
		}
		if len(f.RequestsFor("GET", "/api/v1/sync/status/task-e2e-1")) != 0 {
			t.Error("--no-wait 不应轮询状态")
		}
	})

	t.Run("--json 输出终态", func(t *testing.T) {
		_, dir, cfgFile := setupLoggedIn(t)
		out, _, err := execCLI(t, dir, cfgFile, "--json", "sync", "redis:7", "--force")
		if err != nil {
			t.Fatal(err)
		}
		var st syncStatusResponse
		if err := json.Unmarshal([]byte(out), &st); err != nil {
			t.Fatalf("输出应为 JSON: %v\n%s", err, out)
		}
		if st.TaskID != "task-e2e-1" {
			t.Errorf("task_id = %q", st.TaskID)
		}
	})

	t.Run("提交接口失败", func(t *testing.T) {
		f, dir, cfgFile := setupLoggedIn(t)
		f.Override("POST /api/v1/sync/submit", func(w http.ResponseWriter, r *http.Request) {
			w.WriteHeader(http.StatusInternalServerError)
			respondJSON(w, `{"error":"任务创建失败"}`)
		})
		_, _, err := execCLI(t, dir, cfgFile, "sync", "redis:7", "--force", "--no-wait")
		if err == nil || !strings.Contains(err.Error(), "任务创建失败") {
			t.Fatalf("期望提交失败, got %v", err)
		}
	})

	t.Run("参数校验", func(t *testing.T) {
		_, dir, cfgFile := setupLoggedIn(t)
		if _, _, err := execCLI(t, dir, cfgFile, "sync"); err == nil {
			t.Error("无参数应报错")
		}
		if _, _, err := execCLI(t, dir, cfgFile, "sync", "a", "b"); err == nil {
			t.Error("多参数应报错")
		}
	})
}

func TestBatchE2E(t *testing.T) {
	t.Run("批量提交并等待", func(t *testing.T) {
		f, dir, cfgFile := setupLoggedIn(t)
		listFile := filepath.Join(t.TempDir(), "images.txt")
		os.WriteFile(listFile, []byte("nginx:1.25\nredis:7 7-amd64\n"), 0o600)

		out, _, err := execCLI(t, dir, cfgFile, "batch", "-f", listFile, "--arch", "amd64",
			"--concurrency", "3", "--acr", "ali")
		if err != nil {
			t.Fatal(err)
		}
		if !strings.Contains(out, "registry.fake-a.com/ns-a/nginx:1.26") {
			t.Errorf("输出 = %q", out)
		}
		batches := f.RequestsFor("POST", "/api/v1/sync/batch")
		if len(batches) != 1 {
			t.Fatalf("应调用 batch 一次, got %d", len(batches))
		}
		var body batchRequest
		if err := json.Unmarshal(batches[0].Body, &body); err != nil {
			t.Fatal(err)
		}
		if len(body.Images) != 2 || body.MaxConcurrent != 3 || body.AcrRegistryID != 1 {
			t.Errorf("batch body = %+v", body)
		}
		if body.Images[1].TargetTag != "7-amd64" || body.Images[0].Architecture != "amd64" {
			t.Errorf("镜像项 = %+v", body.Images)
		}
	})

	t.Run("未指定文件报错", func(t *testing.T) {
		_, dir, cfgFile := setupLoggedIn(t)
		_, _, err := execCLI(t, dir, cfgFile, "batch")
		if err == nil || !strings.Contains(err.Error(), "-f") {
			t.Fatalf("期望 -f 提示, got %v", err)
		}
	})

	t.Run("文件解析失败", func(t *testing.T) {
		_, dir, cfgFile := setupLoggedIn(t)
		listFile := filepath.Join(t.TempDir(), "bad.txt")
		os.WriteFile(listFile, []byte("# 空\n"), 0o600)
		_, _, err := execCLI(t, dir, cfgFile, "batch", "-f", listFile)
		if err == nil || !strings.Contains(err.Error(), "没有有效镜像") {
			t.Fatalf("期望解析失败, got %v", err)
		}
	})

	t.Run("--acr 别名不存在", func(t *testing.T) {
		_, dir, cfgFile := setupLoggedIn(t)
		listFile := filepath.Join(t.TempDir(), "ok.txt")
		os.WriteFile(listFile, []byte("nginx:1.25\n"), 0o600)
		_, _, err := execCLI(t, dir, cfgFile, "batch", "-f", listFile, "--acr", "nope")
		if err == nil || !strings.Contains(err.Error(), "未找到别名") {
			t.Fatalf("期望别名错误, got %v", err)
		}
	})

	t.Run("--no-wait", func(t *testing.T) {
		f, dir, cfgFile := setupLoggedIn(t)
		listFile := filepath.Join(t.TempDir(), "ok.txt")
		os.WriteFile(listFile, []byte("nginx:1.25\n"), 0o600)
		out, _, err := execCLI(t, dir, cfgFile, "batch", "-f", listFile, "--no-wait")
		if err != nil {
			t.Fatal(err)
		}
		if !strings.Contains(out, "批量任务已提交") {
			t.Errorf("输出 = %q", out)
		}
		if len(f.RequestsFor("GET", "/api/v1/sync/status/task-e2e-1")) != 0 {
			t.Error("--no-wait 不应轮询")
		}
	})

	t.Run("接口失败", func(t *testing.T) {
		f, dir, cfgFile := setupLoggedIn(t)
		listFile := filepath.Join(t.TempDir(), "ok.txt")
		os.WriteFile(listFile, []byte("nginx:1.25\n"), 0o600)
		f.Override("POST /api/v1/sync/batch", func(w http.ResponseWriter, r *http.Request) {
			w.WriteHeader(http.StatusInternalServerError)
			respondJSON(w, `{"error":"批量创建失败"}`)
		})
		_, _, err := execCLI(t, dir, cfgFile, "batch", "-f", listFile, "--no-wait")
		if err == nil || !strings.Contains(err.Error(), "批量创建失败") {
			t.Fatalf("期望接口失败, got %v", err)
		}
	})
}

func TestTaskListE2E(t *testing.T) {
	f, dir, cfgFile := setupLoggedIn(t)

	t.Run("文本输出", func(t *testing.T) {
		out, _, err := execCLI(t, dir, cfgFile, "task", "list")
		if err != nil {
			t.Fatal(err)
		}
		// 默认输出用 shortID 截断任务 ID（前 8 位）；failed_images=1 → 行内含 失败1
		if !strings.Contains(out, "task-e2e") || !strings.Contains(out, "失败1") {
			t.Errorf("输出 = %q", out)
		}
		if !strings.Contains(out, "共 1 个任务") {
			t.Errorf("输出 = %q", out)
		}
		// --long 显示完整 task-id
		outLong, _, err := execCLI(t, dir, cfgFile, "task", "list", "--long")
		if err != nil {
			t.Fatal(err)
		}
		if !strings.Contains(outLong, "task-e2e-1") {
			t.Errorf("--long 输出应含完整 ID: %q", outLong)
		}
	})

	t.Run("status 过滤透传", func(t *testing.T) {
		if _, _, err := execCLI(t, dir, cfgFile, "task", "list", "--status", "failed", "--page", "2", "--page-size", "5"); err != nil {
			t.Fatal(err)
		}
		reqs := f.RequestsFor("GET", "/api/v1/sync/history")
		if len(reqs) == 0 {
			t.Fatal("应调用 history")
		}
		q := reqs[len(reqs)-1].Query
		if q.Get("status") != "failed" || q.Get("page") != "2" || q.Get("page_size") != "5" {
			t.Errorf("query = %v", q)
		}
	})

	t.Run("--long 带 Actions 链接", func(t *testing.T) {
		out, _, err := execCLI(t, dir, cfgFile, "task", "list", "--long")
		if err != nil {
			t.Fatal(err)
		}
		if !strings.Contains(out, "https://github.com/example/run/1") {
			t.Errorf("输出 = %q", out)
		}
	})

	t.Run("--json", func(t *testing.T) {
		out, _, err := execCLI(t, dir, cfgFile, "--json", "task", "list")
		if err != nil {
			t.Fatal(err)
		}
		var resp historyResponse
		if err := json.Unmarshal([]byte(out), &resp); err != nil {
			t.Fatalf("输出应为 JSON: %v\n%s", err, out)
		}
		if resp.Total != 1 {
			t.Errorf("total = %d", resp.Total)
		}
	})

	t.Run("接口失败", func(t *testing.T) {
		f.Override("GET /api/v1/sync/history", func(w http.ResponseWriter, r *http.Request) { w.WriteHeader(500) })
		if _, _, err := execCLI(t, dir, cfgFile, "task", "list"); err == nil {
			t.Error("期望报错")
		}
	})
}

func TestTaskStatusE2E(t *testing.T) {
	t.Run("单次查询", func(t *testing.T) {
		f, dir, cfgFile := setupLoggedIn(t)
		out, _, err := execCLI(t, dir, cfgFile, "task", "status", "task-e2e-1")
		if err != nil {
			t.Fatal(err)
		}
		if !strings.Contains(out, "registry.fake-a.com/ns-a/nginx:1.26") {
			t.Errorf("输出 = %q", out)
		}
		if len(f.RequestsFor("GET", "/api/v1/sync/status/task-e2e-1")) != 1 {
			t.Error("非 watch 模式只应查询一次")
		}
	})

	t.Run("参数校验", func(t *testing.T) {
		_, dir, cfgFile := setupLoggedIn(t)
		if _, _, err := execCLI(t, dir, cfgFile, "task", "status"); err == nil {
			t.Error("缺 task-id 应报错")
		}
		if _, _, err := execCLI(t, dir, cfgFile, "task", "status", "a", "b"); err == nil {
			t.Error("多参数应报错")
		}
	})

	t.Run("查询失败", func(t *testing.T) {
		f, dir, cfgFile := setupLoggedIn(t)
		f.Override("GET /api/v1/sync/status/task-e2e-1", func(w http.ResponseWriter, r *http.Request) { w.WriteHeader(500) })
		if _, _, err := execCLI(t, dir, cfgFile, "task", "status", "task-e2e-1"); err == nil {
			t.Error("期望报错")
		}
	})
}

func TestRetryE2E(t *testing.T) {
	t.Run("按记录 ID 重试", func(t *testing.T) {
		f, dir, cfgFile := setupLoggedIn(t)
		out, _, err := execCLI(t, dir, cfgFile, "retry", "7")
		if err != nil {
			t.Fatal(err)
		}
		if !strings.Contains(out, "记录 7 重试已提交") {
			t.Errorf("输出 = %q", out)
		}
		if !f.HasRequest("POST", "/api/v1/images/7/retry") {
			t.Error("应调用重试接口")
		}
	})

	t.Run("无参数报错", func(t *testing.T) {
		_, dir, cfgFile := setupLoggedIn(t)
		_, _, err := execCLI(t, dir, cfgFile, "retry")
		if err == nil || !strings.Contains(err.Error(), "请指定记录 ID") {
			t.Fatalf("期望参数提示, got %v", err)
		}
	})

	t.Run("多参数报错", func(t *testing.T) {
		_, dir, cfgFile := setupLoggedIn(t)
		if _, _, err := execCLI(t, dir, cfgFile, "retry", "1", "2"); err == nil {
			t.Error("多参数应报错")
		}
	})

	t.Run("重试接口失败", func(t *testing.T) {
		f, dir, cfgFile := setupLoggedIn(t)
		f.Override("POST /api/v1/images/7/retry", func(w http.ResponseWriter, r *http.Request) {
			w.WriteHeader(http.StatusInternalServerError)
			respondJSON(w, `{"error":"重试失败"}`)
		})
		_, _, err := execCLI(t, dir, cfgFile, "retry", "7")
		if err == nil || !strings.Contains(err.Error(), "重试记录 7 失败") {
			t.Fatalf("期望重试失败, got %v", err)
		}
	})

	t.Run("--task 重试全部失败记录", func(t *testing.T) {
		f, dir, cfgFile := setupLoggedIn(t)
		// 任务状态里塞两条失败记录（/images/{id}/retry 有通配默认路由）
		f.Override("GET /api/v1/sync/status/task-e2e-1", func(w http.ResponseWriter, r *http.Request) {
			respondJSON(w, `{
				"task_id":"task-e2e-1","status":"partial_success","total_images":2,
				"completed_images":0,"failed_images":2,"progress":100,
				"images":{"pending":0,"syncing":0,"success":0,"failed":2,"records":[
					{"id":31,"original_image":"a","tag":"1","sync_status":"failed"},
					{"id":32,"original_image":"b","tag":"2","sync_status":"failed"},
					{"id":33,"original_image":"c","tag":"3","sync_status":"success"}
				]}
			}`)
		})
		out, _, err := execCLI(t, dir, cfgFile, "retry", "--task", "task-e2e-1")
		if err != nil {
			t.Fatal(err)
		}
		if !strings.Contains(out, "记录 31 重试已提交") || !strings.Contains(out, "记录 32 重试已提交") {
			t.Errorf("输出 = %q", out)
		}
		if !f.HasRequest("POST", "/api/v1/images/31/retry") || !f.HasRequest("POST", "/api/v1/images/32/retry") {
			t.Error("应重试两条失败记录")
		}
		if f.HasRequest("POST", "/api/v1/images/33/retry") {
			t.Error("成功记录不应重试")
		}
		if !strings.Contains(out, "dsync task status task-e2e-1 --watch") {
			t.Errorf("输出 = %q", out)
		}
	})

	t.Run("--task 无失败记录", func(t *testing.T) {
		f, dir, cfgFile := setupLoggedIn(t)
		f.Override("GET /api/v1/sync/status/task-e2e-1", func(w http.ResponseWriter, r *http.Request) {
			respondJSON(w, `{"task_id":"task-e2e-1","status":"completed","total_images":1,
				"images":{"records":[{"id":1,"sync_status":"success"}]}}`)
		})
		out, _, err := execCLI(t, dir, cfgFile, "retry", "--task", "task-e2e-1")
		if err != nil {
			t.Fatal(err)
		}
		if !strings.Contains(out, "没有失败记录") {
			t.Errorf("输出 = %q", out)
		}
	})

	t.Run("--task 状态查询失败", func(t *testing.T) {
		f, dir, cfgFile := setupLoggedIn(t)
		f.Override("GET /api/v1/sync/status/task-e2e-1", func(w http.ResponseWriter, r *http.Request) { w.WriteHeader(500) })
		if _, _, err := execCLI(t, dir, cfgFile, "retry", "--task", "task-e2e-1"); err == nil {
			t.Error("期望报错")
		}
	})
}
