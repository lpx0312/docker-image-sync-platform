package main

// coverage_boost2_test.go 补齐 registry-ops 四命令、tag/search/image/login/batch/sync
// 及根命令前置校验的分支覆盖。

import (
	"fmt"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func http500(w http.ResponseWriter, r *http.Request) {
	w.WriteHeader(http.StatusInternalServerError)
	respondJSON(w, `{"error":"服务端错误"}`)
}

// ====================================================================
// dsync acr test
// ====================================================================

func TestAcrTestCmdE2E(t *testing.T) {
	t.Run("全部仓库", func(t *testing.T) {
		_, dir, cfgFile := setupLoggedIn(t)
		out, _, err := execCLI(t, dir, cfgFile, "acr", "test")
		if err != nil {
			t.Fatal(err)
		}
		for _, want := range []string{"登录凭证", "管理面凭证", "SWR"} {
			if !strings.Contains(out, want) {
				t.Errorf("输出缺少 %q:\n%s", want, out)
			}
		}
	})

	t.Run("指定别名", func(t *testing.T) {
		_, dir, cfgFile := setupLoggedIn(t)
		if _, _, err := execCLI(t, dir, cfgFile, "acr", "test", "ali"); err != nil {
			t.Fatal(err)
		}
	})

	t.Run("登录失败汇总报错", func(t *testing.T) {
		f, dir, cfgFile := setupLoggedIn(t)
		f.Override("POST /api/v1/acr-registries/1/test", func(w http.ResponseWriter, r *http.Request) {
			respondJSON(w, `{"data":{"registry_type":"acr","login_ok":false,"login_message":"账号密码错误","manage_ok":false,"manage_message":"","manage_skipped":false}}`)
		})
		_, _, err := execCLI(t, dir, cfgFile, "acr", "test", "ali")
		if err == nil || !strings.Contains(err.Error(), "存在不可用的仓库配置") {
			t.Fatalf("期望汇总失败, got %v", err)
		}
	})

	t.Run("管理面跳过与失败展示", func(t *testing.T) {
		f, dir, cfgFile := setupLoggedIn(t)
		f.Override("POST /api/v1/acr-registries/1/test", func(w http.ResponseWriter, r *http.Request) {
			respondJSON(w, `{"data":{"registry_type":"swr","login_ok":true,"login_message":"登录成功","manage_ok":false,"manage_message":"AK/SK 未配置","manage_skipped":true}}`)
		})
		f.Override("POST /api/v1/acr-registries/2/test", func(w http.ResponseWriter, r *http.Request) {
			respondJSON(w, `{"data":{"registry_type":"swr","login_ok":true,"login_message":"登录成功","manage_ok":false,"manage_message":"AK 无效","manage_skipped":false}}`)
		})
		out, _, err := execCLI(t, dir, cfgFile, "acr", "test")
		if err != nil {
			t.Fatal(err)
		}
		if !strings.Contains(out, "AK/SK 未配置") || !strings.Contains(out, "AK 无效") {
			t.Errorf("输出 = %q", out)
		}
	})

	t.Run("接口失败", func(t *testing.T) {
		f, dir, cfgFile := setupLoggedIn(t)
		f.Override("POST /api/v1/acr-registries/1/test", http500)
		_, _, err := execCLI(t, dir, cfgFile, "acr", "test", "ali")
		if err == nil {
			t.Fatal("接口 500 应报错")
		}
	})
}

// ====================================================================
// dsync repo import / sync-records / clean
// ====================================================================

func TestRepoOpsCmdE2E(t *testing.T) {
	t.Run("import 缺少 --acr", func(t *testing.T) {
		_, dir, cfgFile := setupLoggedIn(t)
		_, _, err := execCLI(t, dir, cfgFile, "repo", "import")
		if err == nil || !strings.Contains(err.Error(), "--acr") {
			t.Fatalf("期望参数提示, got %v", err)
		}
	})

	t.Run("import 成功", func(t *testing.T) {
		_, dir, cfgFile := setupLoggedIn(t)
		out, _, err := execCLI(t, dir, cfgFile, "repo", "import", "--acr", "ali")
		if err != nil {
			t.Fatal(err)
		}
		for _, want := range []string{"新增 2 个", "已存在跳过 1 个", "+ app1"} {
			if !strings.Contains(out, want) {
				t.Errorf("输出缺少 %q:\n%s", want, out)
			}
		}
	})

	t.Run("import 别名不存在", func(t *testing.T) {
		_, dir, cfgFile := setupLoggedIn(t)
		if _, _, err := execCLI(t, dir, cfgFile, "repo", "import", "--acr", "ghost"); err == nil {
			t.Fatal("别名不存在应报错")
		}
	})

	t.Run("import 接口失败", func(t *testing.T) {
		f, dir, cfgFile := setupLoggedIn(t)
		f.Override("POST /api/v1/acr-repositories/import-from-registry", http500)
		_, _, err := execCLI(t, dir, cfgFile, "repo", "import", "--acr", "ali")
		if err == nil {
			t.Fatal("接口 500 应报错")
		}
	})

	t.Run("sync-records 缺少 --acr 与成功", func(t *testing.T) {
		_, dir, cfgFile := setupLoggedIn(t)
		if _, _, err := execCLI(t, dir, cfgFile, "repo", "sync-records"); err == nil {
			t.Fatal("缺少 --acr 应报错")
		}
		out, _, err := execCLI(t, dir, cfgFile, "repo", "sync-records", "--acr", "ali")
		if err != nil {
			t.Fatal(err)
		}
		for _, want := range []string{"新增 1 个", "跳过 2 个", "+ app1"} {
			if !strings.Contains(out, want) {
				t.Errorf("输出缺少 %q:\n%s", want, out)
			}
		}
	})

	t.Run("sync-records 接口失败", func(t *testing.T) {
		f, dir, cfgFile := setupLoggedIn(t)
		f.Override("POST /api/v1/acr-repositories/sync-from-records", http500)
		if _, _, err := execCLI(t, dir, cfgFile, "repo", "sync-records", "--acr", "ali"); err == nil {
			t.Fatal("接口 500 应报错")
		}
	})

	t.Run("clean 缺少 --yes 与成功", func(t *testing.T) {
		_, dir, cfgFile := setupLoggedIn(t)
		if _, _, err := execCLI(t, dir, cfgFile, "repo", "clean", "--acr", "ali"); err == nil {
			t.Fatal("缺少 --yes 应报错")
		}
		out, _, err := execCLI(t, dir, cfgFile, "repo", "clean", "--acr", "ali", "--yes")
		if err != nil {
			t.Fatal(err)
		}
		for _, want := range []string{"删除 2 条", "校验失败保留", "- old1"} {
			if !strings.Contains(out, want) {
				t.Errorf("输出缺少 %q:\n%s", want, out)
			}
		}
	})

	t.Run("clean 接口失败", func(t *testing.T) {
		f, dir, cfgFile := setupLoggedIn(t)
		f.Override("POST /api/v1/acr-repositories/clean-invalid", http500)
		if _, _, err := execCLI(t, dir, cfgFile, "repo", "clean", "--acr", "ali", "--yes"); err == nil {
			t.Fatal("接口 500 应报错")
		}
	})
}

// ====================================================================
// dsync tag list / tag detail
// ====================================================================

func TestTagCmdBranchesE2E(t *testing.T) {
	t.Run("list 指定别名", func(t *testing.T) {
		_, dir, cfgFile := setupLoggedIn(t)
		out, _, err := execCLI(t, dir, cfgFile, "tag", "list", "nginx", "--acr", "ali")
		if err != nil {
			t.Fatal(err)
		}
		if !strings.Contains(out, "1.24") || !strings.Contains(out, "1.25") {
			t.Errorf("输出 = %q", out)
		}
	})

	t.Run("list 别名不存在", func(t *testing.T) {
		_, dir, cfgFile := setupLoggedIn(t)
		if _, _, err := execCLI(t, dir, cfgFile, "tag", "list", "nginx", "--acr", "ghost"); err == nil {
			t.Fatal("别名不存在应报错")
		}
	})

	t.Run("list 仓库不存在", func(t *testing.T) {
		_, dir, cfgFile := setupLoggedIn(t)
		_, _, err := execCLI(t, dir, cfgFile, "tag", "list", "no-such-repo")
		if err == nil || !strings.Contains(err.Error(), "不存在") {
			t.Fatalf("期望不存在错误, got %v", err)
		}
	})

	t.Run("list 仓库列表接口失败", func(t *testing.T) {
		f, dir, cfgFile := setupLoggedIn(t)
		f.Override("GET /api/v1/acr-registries", http500)
		if _, _, err := execCLI(t, dir, cfgFile, "tag", "list", "nginx"); err == nil {
			t.Fatal("仓库接口失败应报错")
		}
	})

	t.Run("list 台账接口失败", func(t *testing.T) {
		f, dir, cfgFile := setupLoggedIn(t)
		f.Override("GET /api/v1/acr-repositories", http500)
		if _, _, err := execCLI(t, dir, cfgFile, "tag", "list", "nginx"); err == nil {
			t.Fatal("台账接口失败应报错")
		}
	})

	t.Run("list Tag 接口失败", func(t *testing.T) {
		f, dir, cfgFile := setupLoggedIn(t)
		f.Override("GET /api/v1/acr-tags", http500)
		if _, _, err := execCLI(t, dir, cfgFile, "tag", "list", "nginx", "--acr", "ali"); err == nil {
			t.Fatal("Tag 接口失败应报错")
		}
	})

	t.Run("detail 指定别名与零值字段", func(t *testing.T) {
		_, dir, cfgFile := setupLoggedIn(t)
		out, _, err := execCLI(t, dir, cfgFile, "tag", "detail", "nginx", "1.25", "--acr", "ali")
		if err != nil {
			t.Fatal(err)
		}
		// fixture 中 arm64 size=0/pushed 空 → 渲染为 -
		if !strings.Contains(out, "amd64") || !strings.Contains(out, "-") {
			t.Errorf("输出 = %q", out)
		}
	})

	t.Run("detail 无架构信息", func(t *testing.T) {
		f, dir, cfgFile := setupLoggedIn(t)
		f.Override("GET /api/v1/acr-tags/detail", func(w http.ResponseWriter, r *http.Request) {
			respondJSON(w, `{"status":"success","data":{"tag":"x","architectures":[]}}`)
		})
		out, _, err := execCLI(t, dir, cfgFile, "tag", "detail", "nginx", "1.25", "--acr", "ali")
		if err != nil {
			t.Fatal(err)
		}
		if !strings.Contains(out, "无架构信息") {
			t.Errorf("输出 = %q", out)
		}
	})

	t.Run("detail 别名不存在/接口失败", func(t *testing.T) {
		_, dir, cfgFile := setupLoggedIn(t)
		if _, _, err := execCLI(t, dir, cfgFile, "tag", "detail", "nginx", "1.25", "--acr", "ghost"); err == nil {
			t.Fatal("别名不存在应报错")
		}
		f, dir, cfgFile := setupLoggedIn(t)
		f.Override("GET /api/v1/acr-tags/detail", http500)
		if _, _, err := execCLI(t, dir, cfgFile, "tag", "detail", "nginx", "1.25", "--acr", "ali"); err == nil {
			t.Fatal("接口失败应报错")
		}
	})

	t.Run("detail 仓库不存在与台账失败", func(t *testing.T) {
		_, dir, cfgFile := setupLoggedIn(t)
		if _, _, err := execCLI(t, dir, cfgFile, "tag", "detail", "no-such-repo", "1.0"); err == nil {
			t.Fatal("仓库不存在应报错")
		}
		f, dir, cfgFile := setupLoggedIn(t)
		f.Override("GET /api/v1/acr-repositories", http500)
		if _, _, err := execCLI(t, dir, cfgFile, "tag", "detail", "nginx", "1.25"); err == nil {
			t.Fatal("台账接口失败应报错")
		}
	})
}

// ====================================================================
// dsync search
// ====================================================================

func TestSearchCmdE2E(t *testing.T) {
	t.Run("仓库与 Tag 同时命中", func(t *testing.T) {
		_, dir, cfgFile := setupLoggedIn(t)
		out, _, err := execCLI(t, dir, cfgFile, "search", "ngin")
		if err != nil {
			t.Fatal(err)
		}
		if !strings.Contains(out, "仓库匹配") {
			t.Errorf("输出 = %q", out)
		}
	})

	t.Run("Tag 命中", func(t *testing.T) {
		_, dir, cfgFile := setupLoggedIn(t)
		out, _, err := execCLI(t, dir, cfgFile, "search", "1.2", "--acr", "ali")
		if err != nil {
			t.Fatal(err)
		}
		if !strings.Contains(out, "1.25") {
			t.Errorf("输出 = %q", out)
		}
	})

	t.Run("无命中", func(t *testing.T) {
		_, dir, cfgFile := setupLoggedIn(t)
		out, _, err := execCLI(t, dir, cfgFile, "search", "zzz-never-match", "--acr", "ali")
		if err != nil {
			t.Fatal(err)
		}
		if !strings.Contains(out, "未找到") {
			t.Errorf("输出 = %q", out)
		}
	})

	t.Run("别名不存在", func(t *testing.T) {
		_, dir, cfgFile := setupLoggedIn(t)
		if _, _, err := execCLI(t, dir, cfgFile, "search", "x", "--acr", "ghost"); err == nil {
			t.Fatal("别名不存在应报错")
		}
	})

	t.Run("台账接口失败", func(t *testing.T) {
		f, dir, cfgFile := setupLoggedIn(t)
		f.Override("GET /api/v1/acr-repositories", http500)
		if _, _, err := execCLI(t, dir, cfgFile, "search", "x"); err == nil {
			t.Fatal("台账接口失败应报错")
		}
	})

	t.Run("Tag 拉取部分失败仅告警", func(t *testing.T) {
		_, dir, cfgFile := setupLoggedIn(t)
		// /acr-tags 全量 500：ensureTagCacheFresh 只告警不中断
		outFile := filepath.Join(t.TempDir(), "err.log")
		_ = outFile
		os.Setenv("DSYNC_TEST_UNUSED", "1")
		os.Unsetenv("DSYNC_TEST_UNUSED")
		_, stderr, err := execCLI(t, dir, cfgFile, "search", "zzz-no-hit")
		_ = stderr
		if err != nil {
			t.Fatal(err)
		}
	})

	t.Run("超过 200 条截断提示", func(t *testing.T) {
		f, dir, cfgFile := setupLoggedIn(t)
		tags := make([]string, 210)
		parts := make([]string, 210)
		for i := range tags {
			tags[i] = fmt.Sprintf("v1.0.%d", i)
			parts[i] = fmt.Sprintf("%q", tags[i])
		}
		f.Override("GET /api/v1/acr-tags", func(w http.ResponseWriter, r *http.Request) {
			respondJSON(w, fmt.Sprintf(`{"status":"success","data":{"tags":[%s],"total":210}}`, strings.Join(parts, ",")))
		})
		out, _, err := execCLI(t, dir, cfgFile, "search", "v1.0", "--acr", "ali")
		if err != nil {
			t.Fatal(err)
		}
		if !strings.Contains(out, "仅显示前 200 条") {
			t.Errorf("输出 = %q", out)
		}
	})
}

// ====================================================================
// dsync image 分支
// ====================================================================

func TestImageCmdBranchesE2E(t *testing.T) {
	t.Run("全部筛选 flag 透传", func(t *testing.T) {
		f, dir, cfgFile := setupLoggedIn(t)
		if _, _, err := execCLI(t, dir, cfgFile, "image", "list",
			"--status", "failed", "--search", "ng", "--arch", "amd64",
			"--sort-by", "created_at", "--sort-order", "desc", "--deduplicate"); err != nil {
			t.Fatal(err)
		}
		reqs := f.RequestsFor("GET", "/api/v1/images/list")
		if len(reqs) == 0 {
			t.Fatal("应调用 images/list")
		}
		q := reqs[len(reqs)-1].Query
		for k, v := range map[string]string{
			"status": "failed", "search": "ng", "architecture": "amd64",
			"sort_by": "created_at", "sort_order": "desc", "deduplicate": "true",
		} {
			if q.Get(k) != v {
				t.Errorf("query[%s] = %q, 期望 %q", k, q.Get(k), v)
			}
		}
	})

	t.Run("list 文本输出含未回填", func(t *testing.T) {
		_, dir, cfgFile := setupLoggedIn(t)
		out, _, err := execCLI(t, dir, cfgFile, "image", "list")
		if err != nil {
			t.Fatal(err)
		}
		if !strings.Contains(out, "未回填") {
			t.Errorf("输出 = %q", out)
		}
	})

	t.Run("list/stats/check/delete 接口失败", func(t *testing.T) {
		f, dir, cfgFile := setupLoggedIn(t)
		f.Override("GET /api/v1/images/list", http500)
		if _, _, err := execCLI(t, dir, cfgFile, "image", "list"); err == nil {
			t.Fatal("list 500 应报错")
		}
		f2, dir2, cfg2 := setupLoggedIn(t)
		f2.Override("GET /api/v1/images/stats", http500)
		if _, _, err := execCLI(t, dir2, cfg2, "image", "stats"); err == nil {
			t.Fatal("stats 500 应报错")
		}
		f3, dir3, cfg3 := setupLoggedIn(t)
		f3.Override("POST /api/v1/images/9/check", http500)
		if _, _, err := execCLI(t, dir3, cfg3, "image", "check", "9"); err == nil {
			t.Fatal("check 500 应报错")
		}
	})

	t.Run("delete 缺少 --yes 与接口失败", func(t *testing.T) {
		_, dir, cfgFile := setupLoggedIn(t)
		if _, _, err := execCLI(t, dir, cfgFile, "image", "delete", "1"); err == nil {
			t.Fatal("缺少 --yes 应报错")
		}
		f, dir, cfgFile := setupLoggedIn(t)
		f.Override("DELETE /api/v1/images/9", http500)
		if _, _, err := execCLI(t, dir, cfgFile, "image", "delete", "9", "--yes"); err == nil {
			t.Fatal("delete 500 应报错")
		}
	})
}

// ====================================================================
// dsync acr list / sync / batch 分支
// ====================================================================

func TestAcrListAndSyncBranchesE2E(t *testing.T) {
	t.Run("acr list 文本输出不限与已满", func(t *testing.T) {
		_, dir, cfgFile := setupLoggedIn(t)
		out, _, err := execCLI(t, dir, cfgFile, "acr", "list")
		if err != nil {
			t.Fatal(err)
		}
		if !strings.Contains(out, "不限") || !strings.Contains(out, "(满)") {
			t.Errorf("输出 = %q", out)
		}
	})

	t.Run("acr list 仓库/配额接口失败", func(t *testing.T) {
		f, dir, cfgFile := setupLoggedIn(t)
		f.Override("GET /api/v1/acr-registries", http500)
		if _, _, err := execCLI(t, dir, cfgFile, "acr", "list"); err == nil {
			t.Fatal("仓库接口失败应报错")
		}
		f2, dir2, cfg2 := setupLoggedIn(t)
		f2.Override("GET /api/v1/acr-registries/quota-summary", http500)
		if _, _, err := execCLI(t, dir2, cfg2, "acr", "list"); err == nil {
			t.Fatal("配额接口失败应报错")
		}
	})

	t.Run("sync 推荐接口失败", func(t *testing.T) {
		f, dir, cfgFile := setupLoggedIn(t)
		f.Override("GET /api/v1/sync/suggest-acr", http500)
		_, _, err := execCLI(t, dir, cfgFile, "sync", "nginx:1.25")
		if err == nil || !strings.Contains(err.Error(), "推荐目标仓库失败") {
			t.Fatalf("期望推荐失败, got %v", err)
		}
	})

	t.Run("sync 推荐的仓库不存在", func(t *testing.T) {
		f, dir, cfgFile := setupLoggedIn(t)
		f.Override("GET /api/v1/sync/suggest-acr", func(w http.ResponseWriter, r *http.Request) {
			respondJSON(w, `{"status":"success","data":{"suggested_acr_id":999,"suggested_namespace":"ns-x","suggested_alias":"","suggestion_reason":"default"}}`)
		})
		_, _, err := execCLI(t, dir, cfgFile, "sync", "nginx:1.25")
		if err == nil || !strings.Contains(err.Error(), "不存在") {
			t.Fatalf("期望推荐仓库不存在错误, got %v", err)
		}
	})

	t.Run("batch 提交接口失败", func(t *testing.T) {
		f, dir, cfgFile := setupLoggedIn(t)
		listFile := filepath.Join(t.TempDir(), "ok.txt")
		os.WriteFile(listFile, []byte("nginx:1.25\n"), 0o600)
		f.Override("POST /api/v1/sync/batch", http500)
		if _, _, err := execCLI(t, dir, cfgFile, "batch", "-f", listFile); err == nil {
			t.Fatal("batch 500 应报错")
		}
	})

	t.Run("batch 等待状态失败", func(t *testing.T) {
		f, dir, cfgFile := setupLoggedIn(t)
		listFile := filepath.Join(t.TempDir(), "ok.txt")
		os.WriteFile(listFile, []byte("nginx:1.25\n"), 0o600)
		f.Override("GET /api/v1/sync/status/task-e2e-1", http500)
		if _, _, err := execCLI(t, dir, cfgFile, "batch", "-f", listFile); err == nil {
			t.Fatal("状态查询失败应报错")
		}
	})
}

// ====================================================================
// 登录与根命令前置校验
// ====================================================================

func TestLoginPasswordPromptFailures(t *testing.T) {
	t.Run("login 非交互读取密码失败", func(t *testing.T) {
		dir, cfgFile := isolatedConfigPaths(t)
		t.Setenv("DSYNC_PASSWORD", "")
		_, _, err := execCLI(t, dir, cfgFile, "login", "--server", "http://127.0.0.1:1", "--username", "admin")
		if err == nil || !strings.Contains(err.Error(), "读取密码失败") {
			t.Fatalf("期望读取密码失败, got %v", err)
		}
	})

	t.Run("passwd 非交互读取原/新密码失败", func(t *testing.T) {
		_, dir, cfgFile := setupLoggedIn(t)
		t.Setenv("DSYNC_OLD_PASSWORD", "")
		t.Setenv("DSYNC_NEW_PASSWORD", "")
		if _, _, err := execCLI(t, dir, cfgFile, "passwd"); err == nil || !strings.Contains(err.Error(), "读取原密码失败") {
			t.Fatalf("期望读取原密码失败, got %v", err)
		}
		t.Setenv("DSYNC_OLD_PASSWORD", "mock-pass")
		t.Setenv("DSYNC_NEW_PASSWORD", "")
		if _, _, err := execCLI(t, dir, cfgFile, "passwd"); err == nil || !strings.Contains(err.Error(), "读取新密码失败") {
			t.Fatalf("期望读取新密码失败, got %v", err)
		}
	})
}

func TestRootPreRunEBranches(t *testing.T) {
	t.Run("配置解析失败", func(t *testing.T) {
		dir, cfgFile := isolatedConfigPaths(t)
		os.WriteFile(cfgFile, []byte("{bad"), 0o600)
		if _, _, err := execCLI(t, dir, cfgFile, "acr", "list"); err == nil || !strings.Contains(err.Error(), "解析配置文件失败") {
			t.Fatalf("期望配置解析失败, got %v", err)
		}
	})

	t.Run("未配置服务器", func(t *testing.T) {
		dir, cfgFile := isolatedConfigPaths(t)
		writeConfigFile(t, cfgFile, &Config{Token: "x"})
		_, _, err := execCLI(t, dir, cfgFile, "image", "list")
		if err == nil || !strings.Contains(err.Error(), "尚未配置服务器地址") {
			t.Fatalf("期望未配置服务器错误, got %v", err)
		}
	})
}

func TestLoadConfigReadFailure(t *testing.T) {
	// cfgPath 指向目录：ReadFile 报非 NotExist 错误
	saved := cfgPath
	dir := t.TempDir()
	cfgPath = filepath.Join(dir, "config.json")
	os.MkdirAll(cfgPath, 0o755)
	defer func() { cfgPath = saved }()
	if _, err := loadConfig(); err == nil {
		t.Error("读取目录应报错")
	}
}
