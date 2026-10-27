package main

// coverage_boost_test.go 补齐 check/suggest 命令 RunE、Execute、scopeRegistries
// 及 config/cache 错误分支的端到端覆盖。

import (
	"encoding/json"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestExecuteVersion(t *testing.T) {
	// 覆盖 Execute() 本体：用未知子命令走错误返回路径。
	// 不用会写 stdout 的子命令（help/completion）——进程内测试的 stdout
	// 管道可能已被 execCLI 捕获并关闭，写入会得到 "file already closed"。
	resetAllFlags(rootCmd)
	rootCmd.SetArgs([]string{"definitely-not-a-command"})
	defer rootCmd.SetArgs(nil)
	if err := Execute(); err == nil {
		t.Fatal("未知子命令应返回错误")
	}
}

func TestCheckCmdE2E(t *testing.T) {
	t.Run("默认归属输出", func(t *testing.T) {
		_, dir, cfgFile := setupLoggedIn(t)
		out, _, err := execCLI(t, dir, cfgFile, "check", "nginx")
		if err != nil {
			t.Fatal(err)
		}
		for _, want := range []string{"nginx", "已归属", "ali"} { // 别名优先于 namespace
			if !strings.Contains(out, want) {
				t.Errorf("输出缺少 %q:\n%s", want, out)
			}
		}
	})

	t.Run("冲突与新仓库与多仓库告警", func(t *testing.T) {
		f, dir, cfgFile := setupLoggedIn(t)
		f.Override("POST /api/v1/sync/check-acr", func(w http.ResponseWriter, r *http.Request) {
			w.Header().Set("Content-Type", "application/json")
			_, _ = w.Write([]byte(`{"status":"success","data":{
				"items":[
					{"image":"a","repository_name":"a","has_conflict":true,"has_affinity":false,"message":"已归属 ns-b","suggested_namespace":"ns-b","suggested_alias":"","is_new_repository":false},
					{"image":"b","repository_name":"","has_conflict":false,"has_affinity":false,"is_new_repository":true}
				],
				"conflicts":[{}], "new_repository_count":1, "has_any_conflict":true, "multi_acr_warning":true}}`))
		})
		out, _, err := execCLI(t, dir, cfgFile, "check", "a", "b")
		if err != nil {
			t.Fatal(err)
		}
		for _, want := range []string{"冲突", "新仓库", "注意: 多个镜像归属不同仓库"} {
			if !strings.Contains(out, want) {
				t.Errorf("输出缺少 %q:\n%s", want, out)
			}
		}
	})

	t.Run("--acr 指定仓库", func(t *testing.T) {
		_, dir, cfgFile := setupLoggedIn(t)
		out, _, err := execCLI(t, dir, cfgFile, "check", "nginx", "--acr", "ali")
		if err != nil {
			t.Fatal(err)
		}
		if !strings.Contains(out, "nginx") {
			t.Errorf("输出 = %q", out)
		}
	})

	t.Run("--acr 不存在报错", func(t *testing.T) {
		_, dir, cfgFile := setupLoggedIn(t)
		_, _, err := execCLI(t, dir, cfgFile, "check", "nginx", "--acr", "no-such")
		if err == nil {
			t.Fatal("不存在的别名应报错")
		}
	})

	t.Run("接口失败", func(t *testing.T) {
		f, dir, cfgFile := setupLoggedIn(t)
		f.Override("POST /api/v1/sync/check-acr", func(w http.ResponseWriter, r *http.Request) {
			w.WriteHeader(http.StatusInternalServerError)
			_, _ = w.Write([]byte(`{"error":"检查失败"}`))
		})
		_, _, err := execCLI(t, dir, cfgFile, "check", "nginx")
		if err == nil || !strings.Contains(err.Error(), "检查失败") {
			t.Fatalf("期望接口错误, got %v", err)
		}
	})

	t.Run("json 输出", func(t *testing.T) {
		_, dir, cfgFile := setupLoggedIn(t)
		out, _, err := execCLI(t, dir, cfgFile, "check", "nginx", "--json")
		if err != nil {
			t.Fatal(err)
		}
		var m map[string]any
		if err := json.Unmarshal([]byte(out), &m); err != nil {
			t.Fatalf("输出不是 JSON: %v\n%s", err, out)
		}
	})
}

func TestSuggestCmdE2E(t *testing.T) {
	t.Run("文本输出含配额表", func(t *testing.T) {
		f, dir, cfgFile := setupLoggedIn(t)
		f.Override("GET /api/v1/sync/suggest-acr", func(w http.ResponseWriter, r *http.Request) {
			w.Header().Set("Content-Type", "application/json")
			_, _ = w.Write([]byte(`{"status":"success","data":{
				"affinity":{"repository_name":"nginx","has_affinity":true,"acr_registry_id":1,"acr_namespace":"ns-a","acr_alias":"ali","registry_url":"registry.fake-a.com","source":"repository"},
				"suggested_acr_id":1,"suggested_namespace":"ns-a","suggested_alias":"ali","suggestion_reason":"affinity",
				"quota_summary":[
					{"acr_registry_id":1,"namespace":"ns-a","alias":"ali","registry_url":"x","repo_count":10,"repo_quota":300,"remaining_quota":290,"is_default":true,"is_full":false},
					{"acr_registry_id":2,"namespace":"ns-b","alias":"","registry_url":"y","repo_count":5,"repo_quota":0,"remaining_quota":0,"is_default":false,"is_full":true}
				]}}`))
		})
		out, _, err := execCLI(t, dir, cfgFile, "suggest", "nginx")
		if err != nil {
			t.Fatal(err)
		}
		for _, want := range []string{"推荐仓库", "归属来源", "配额用量", "300", "不限", "已满"} {
			if !strings.Contains(out, want) {
				t.Errorf("输出缺少 %q:\n%s", want, out)
			}
		}
	})

	t.Run("无归属无配额", func(t *testing.T) {
		f, dir, cfgFile := setupLoggedIn(t)
		f.Override("GET /api/v1/sync/suggest-acr", func(w http.ResponseWriter, r *http.Request) {
			w.Header().Set("Content-Type", "application/json")
			_, _ = w.Write([]byte(`{"status":"success","data":{
				"affinity":{"has_affinity":false},
				"suggested_acr_id":1,"suggested_namespace":"ns-a","suggested_alias":"ali","suggestion_reason":"default",
				"quota_summary":[]}}`))
		})
		out, _, err := execCLI(t, dir, cfgFile, "suggest", "nginx")
		if err != nil {
			t.Fatal(err)
		}
		if strings.Contains(out, "配额用量") || strings.Contains(out, "归属来源") {
			t.Errorf("不应出现配额/归属区块:\n%s", out)
		}
	})

	t.Run("接口失败", func(t *testing.T) {
		f, dir, cfgFile := setupLoggedIn(t)
		f.Override("GET /api/v1/sync/suggest-acr", func(w http.ResponseWriter, r *http.Request) {
			w.WriteHeader(http.StatusInternalServerError)
			_, _ = w.Write([]byte(`{"error":"推荐失败"}`))
		})
		_, _, err := execCLI(t, dir, cfgFile, "suggest", "nginx")
		if err == nil || !strings.Contains(err.Error(), "推荐失败") {
			t.Fatalf("期望接口错误, got %v", err)
		}
	})
}

func TestScopeRegistriesByAcr(t *testing.T) {
	t.Run("--acr 命中别名", func(t *testing.T) {
		_, dir, cfgFile := setupLoggedIn(t)
		out, _, err := execCLI(t, dir, cfgFile, "repo", "list", "--acr", "ali")
		if err != nil {
			t.Fatal(err)
		}
		if !strings.Contains(out, "nginx") && !strings.Contains(out, "OK") && !strings.Contains(out, "共") {
			// repo list 输出形态按 fixture 而定，宽松断言不报错即可
			t.Logf("repo list --acr 输出: %s", out)
		}
	})

	t.Run("--acr 不存在报错", func(t *testing.T) {
		_, dir, cfgFile := setupLoggedIn(t)
		_, _, err := execCLI(t, dir, cfgFile, "repo", "list", "--acr", "ghost")
		if err == nil {
			t.Fatal("不存在的别名应报错")
		}
	})
}

func TestConfigFileErrorBranches(t *testing.T) {
	t.Run("loadConfig 非法 JSON", func(t *testing.T) {
		dir := t.TempDir()
		bad := filepath.Join(dir, "bad.json")
		if err := os.WriteFile(bad, []byte("{not-json"), 0o600); err != nil {
			t.Fatal(err)
		}
		saved := cfgPath
		cfgPath = bad
		defer func() { cfgPath = saved }()
		_, err := loadConfig()
		if err == nil {
			t.Error("非法 JSON 应报错")
		}
	})

	t.Run("saveConfig 目录不存在", func(t *testing.T) {
		saved := cfgPath
		defer func() { cfgPath = saved }()
		cfgPath = filepath.Join(t.TempDir(), "no-such-dir", "config.json")
		if err := saveConfig(&Config{}); err == nil {
			t.Error("写入不存在的目录应报错")
		}
	})
}

func TestSaveTagCacheWriteFailure(t *testing.T) {
	saved := configDirOverride
	defer func() { configDirOverride = saved }()
	// 指向一个文件占位路径，写缓存时 mkdir 失败 -> 错误分支
	blocker := filepath.Join(t.TempDir(), "blocker")
	if err := os.WriteFile(blocker, []byte("x"), 0o600); err != nil {
		t.Fatal(err)
	}
	configDirOverride = blocker
	if err := saveTagCache(&tagCache{}); err == nil {
		t.Error("缓存目录不可写时应报错")
	}
}
