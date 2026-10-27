package main

// login / whoami / logout / passwd 的 e2e 测试（cobra 整链路 + httptest 服务端）。

import (
	"encoding/json"
	"errors"
	"net/http"
	"os"
	"os/exec"
	"strings"
	"testing"
)

// withStdin 把 os.Stdin 替换为内容为 data 的管道，测试结束后恢复。
func withStdin(t *testing.T, data string) {
	t.Helper()
	old := os.Stdin
	r, w, err := os.Pipe()
	if err != nil {
		t.Fatal(err)
	}
	if _, err := w.WriteString(data); err != nil {
		t.Fatal(err)
	}
	w.Close()
	os.Stdin = r
	t.Cleanup(func() { os.Stdin = old })
}

// withStdinClosed 把 os.Stdin 替换为已关闭（EOF）的管道。
func withStdinClosed(t *testing.T) {
	t.Helper()
	old := os.Stdin
	r, w, err := os.Pipe()
	if err != nil {
		t.Fatal(err)
	}
	w.Close()
	r.Close()
	os.Stdin = r
	t.Cleanup(func() { os.Stdin = old })
}

// readOnlyFile 把文件置为只读（跨平台：Windows 映射只读属性），
// 用于制造「配置可读不可写」的场景，测试结束后恢复可写以便清理。
func readOnlyFile(t *testing.T, path string) {
	t.Helper()
	if err := os.Chmod(path, 0o444); err != nil {
		t.Fatalf("chmod 只读失败: %v", err)
	}
	t.Cleanup(func() { _ = os.Chmod(path, 0o600) })
}

func TestLoginE2E(t *testing.T) {
	f := newFakePlatform(t)
	dir, cfgFile := isolatedConfigPaths(t)
	t.Setenv("DSYNC_PASSWORD", "mock-pass")

	t.Run("未指定 server 且配置为空时报错", func(t *testing.T) {
		writeConfigFile(t, cfgFile, &Config{})
		_, _, err := execCLI(t, dir, cfgFile, "login", "--username", "admin")
		if err == nil || !strings.Contains(err.Error(), "--server") {
			t.Fatalf("期望提示指定 server, got %v", err)
		}
	})

	t.Run("成功登录并落盘（尾部斜杠裁剪）", func(t *testing.T) {
		writeConfigFile(t, cfgFile, &Config{})
		out, _, err := execCLI(t, dir, cfgFile, "login", "--server", f.Server.URL+"/", "--username", "admin")
		if err != nil {
			t.Fatal(err)
		}
		if !strings.Contains(out, "登录成功") || !strings.Contains(out, "管理员") {
			t.Errorf("输出 = %q", out)
		}
		data, _ := os.ReadFile(cfgFile)
		var c Config
		if err := json.Unmarshal(data, &c); err != nil {
			t.Fatal(err)
		}
		if c.Server != f.Server.URL {
			t.Errorf("server = %q, 期望去掉尾部斜杠的 %q", c.Server, f.Server.URL)
		}
		if c.Token != "fake-token" || c.Username != "admin" || c.Password != "mock-pass" {
			t.Errorf("落盘配置 = %+v", c)
		}
	})

	t.Run("server 缺省时取配置中的值", func(t *testing.T) {
		writeConfigFile(t, cfgFile, &Config{Server: f.Server.URL})
		withStdin(t, "admin\n") // 用户名交互输入
		out, _, err := execCLI(t, dir, cfgFile, "login")
		if err != nil {
			t.Fatal(err)
		}
		if !strings.Contains(out, "登录成功") {
			t.Errorf("输出 = %q", out)
		}
	})

	t.Run("用户名从配置继承", func(t *testing.T) {
		writeConfigFile(t, cfgFile, &Config{Server: f.Server.URL, Username: "saved-user"})
		out, _, err := execCLI(t, dir, cfgFile, "login")
		if err != nil {
			t.Fatal(err)
		}
		if !strings.Contains(out, "saved-user") {
			t.Errorf("输出 = %q", out)
		}
	})

	t.Run("交互输入用户名", func(t *testing.T) {
		writeConfigFile(t, cfgFile, &Config{Server: f.Server.URL})
		withStdin(t, "interactive-user\n")
		out, _, err := execCLI(t, dir, cfgFile, "login")
		if err != nil {
			t.Fatal(err)
		}
		if !strings.Contains(out, "interactive-user") {
			t.Errorf("输出 = %q", out)
		}
	})

	t.Run("交互用户名为空报错", func(t *testing.T) {
		writeConfigFile(t, cfgFile, &Config{Server: f.Server.URL})
		withStdin(t, "\n")
		_, _, err := execCLI(t, dir, cfgFile, "login")
		if err == nil || !strings.Contains(err.Error(), "用户名不能为空") {
			t.Fatalf("期望用户名为空报错, got %v", err)
		}
	})

	t.Run("读取用户名失败", func(t *testing.T) {
		writeConfigFile(t, cfgFile, &Config{Server: f.Server.URL})
		withStdinClosed(t)
		_, _, err := execCLI(t, dir, cfgFile, "login")
		if err == nil || !strings.Contains(err.Error(), "读取用户名失败") {
			t.Fatalf("期望读取用户名失败, got %v", err)
		}
	})

	t.Run("非交互环境读取密码失败", func(t *testing.T) {
		writeConfigFile(t, cfgFile, &Config{Server: f.Server.URL})
		withStdin(t, "admin\n")
		t.Setenv("DSYNC_PASSWORD", "") // 强制走交互输入，管道非终端 → ReadPassword 失败
		_, _, err := execCLI(t, dir, cfgFile, "login")
		if err == nil || !strings.Contains(err.Error(), "读取密码失败") {
			t.Fatalf("期望读取密码失败, got %v", err)
		}
	})

	t.Run("服务端拒绝登录", func(t *testing.T) {
		f2 := newFakePlatform(t)
		dir2, cfg2 := isolatedConfigPaths(t)
		f2.Override("POST /api/v1/auth/login", func(w http.ResponseWriter, r *http.Request) {
			w.WriteHeader(http.StatusUnauthorized)
			respondJSON(w, `{"error":"用户名或密码错误"}`)
		})
		t.Setenv("DSYNC_PASSWORD", "wrong")
		_, _, err := execCLI(t, dir2, cfg2, "login", "--server", f2.Server.URL, "--username", "admin")
		if err == nil || !strings.Contains(err.Error(), "用户名或密码错误") {
			t.Fatalf("期望登录失败, got %v", err)
		}
	})

	t.Run("配置写盘失败", func(t *testing.T) {
		f3 := newFakePlatform(t)
		dir3, cfg3 := isolatedConfigPaths(t)
		writeConfigFile(t, cfg3, &Config{})
		readOnlyFile(t, cfg3)
		_, _, err := execCLI(t, dir3, cfg3, "login", "--server", f3.Server.URL, "--username", "admin")
		if err == nil || !strings.Contains(err.Error(), "写入配置文件失败") {
			t.Fatalf("期望写盘失败, got %v", err)
		}
	})

	t.Run("json 输出", func(t *testing.T) {
		writeConfigFile(t, cfgFile, &Config{})
		out, _, err := execCLI(t, dir, cfgFile, "--json", "login", "--server", f.Server.URL, "--username", "admin")
		if err != nil {
			t.Fatal(err)
		}
		var m map[string]any
		if err := json.Unmarshal([]byte(out), &m); err != nil {
			t.Fatalf("输出应为 JSON: %v\n%s", err, out)
		}
		if m["token"] != "fake-token" {
			t.Errorf("token = %v", m["token"])
		}
	})
}

func TestWhoamiE2E(t *testing.T) {
	f, dir, cfgFile := setupLoggedIn(t)

	t.Run("成功", func(t *testing.T) {
		out, _, err := execCLI(t, dir, cfgFile, "whoami")
		if err != nil {
			t.Fatal(err)
		}
		if !strings.Contains(out, "admin") || !strings.Contains(out, "管理员") || !strings.Contains(out, "权限: 1 项") {
			t.Errorf("输出 = %q", out)
		}
	})

	t.Run("json 输出", func(t *testing.T) {
		out, _, err := execCLI(t, dir, cfgFile, "--json", "whoami")
		if err != nil {
			t.Fatal(err)
		}
		var m map[string]any
		if err := json.Unmarshal([]byte(out), &m); err != nil {
			t.Fatalf("输出应为 JSON: %v\n%s", err, out)
		}
	})

	t.Run("接口失败", func(t *testing.T) {
		f.Override("GET /api/v1/auth/me", func(w http.ResponseWriter, r *http.Request) { w.WriteHeader(500) })
		_, _, err := execCLI(t, dir, cfgFile, "whoami")
		if err == nil {
			t.Fatal("期望报错")
		}
	})
}

func TestLogoutE2E(t *testing.T) {
	f, dir, cfgFile := setupLoggedIn(t)

	t.Run("带 token 登出并清除凭据", func(t *testing.T) {
		out, _, err := execCLI(t, dir, cfgFile, "logout")
		if err != nil {
			t.Fatal(err)
		}
		if !strings.Contains(out, "已登出") {
			t.Errorf("输出 = %q", out)
		}
		if !f.HasRequest("POST", "/api/v1/auth/logout") {
			t.Error("应调用服务端登出接口")
		}
		data, _ := os.ReadFile(cfgFile)
		if strings.Contains(string(data), "fake-token") || strings.Contains(string(data), "\"username\"") {
			t.Errorf("凭据未清除: %s", data)
		}
	})

	t.Run("服务端登出失败仍清除本地凭据", func(t *testing.T) {
		f2, dir2, cfg2 := setupLoggedIn(t)
		f2.Override("POST /api/v1/auth/logout", func(w http.ResponseWriter, r *http.Request) { w.WriteHeader(500) })
		_, stderr, err := execCLI(t, dir2, cfg2, "logout")
		if err != nil {
			t.Fatal(err)
		}
		if !strings.Contains(stderr, "服务端登出失败（忽略）") {
			t.Errorf("stderr = %q", stderr)
		}
		data, _ := os.ReadFile(cfg2)
		if strings.Contains(string(data), "fake-token") {
			t.Error("凭据未清除")
		}
	})

	t.Run("无 token 时不调服务端", func(t *testing.T) {
		f3, dir3, cfg3 := setupLoggedIn(t)
		writeConfigFile(t, cfg3, &Config{Server: f3.Server.URL})
		if _, _, err := execCLI(t, dir3, cfg3, "logout"); err != nil {
			t.Fatal(err)
		}
		if f3.HasRequest("POST", "/api/v1/auth/logout") {
			t.Error("无 token 不应调用服务端登出")
		}
	})

	t.Run("写盘失败报错", func(t *testing.T) {
		_, dir4, cfg4 := setupLoggedIn(t)
		readOnlyFile(t, cfg4)
		_, _, err := execCLI(t, dir4, cfg4, "logout")
		if err == nil || !strings.Contains(err.Error(), "写入配置文件失败") {
			t.Fatalf("期望写盘失败, got %v", err)
		}
	})
}

func TestPasswdE2E(t *testing.T) {
	t.Setenv("DSYNC_OLD_PASSWORD", "old-pass")
	t.Setenv("DSYNC_NEW_PASSWORD", "NewPass123!")

	t.Run("成功修改并更新本地凭据", func(t *testing.T) {
		_, dir, cfgFile := setupLoggedIn(t)
		out, _, err := execCLI(t, dir, cfgFile, "passwd")
		if err != nil {
			t.Fatal(err)
		}
		if !strings.Contains(out, "密码修改成功") {
			t.Errorf("输出 = %q", out)
		}
		data, _ := os.ReadFile(cfgFile)
		var c Config
		_ = json.Unmarshal(data, &c)
		if c.Password != "NewPass123!" || c.Token != "" {
			t.Errorf("本地凭据 = %+v, 期望更新密码且清空 token", c)
		}
	})

	t.Run("非交互环境读取原密码失败", func(t *testing.T) {
		_, dir, cfgFile := setupLoggedIn(t)
		withStdin(t, "")
		t.Setenv("DSYNC_OLD_PASSWORD", "")
		t.Setenv("DSYNC_NEW_PASSWORD", "x")
		_, _, err := execCLI(t, dir, cfgFile, "passwd")
		if err == nil || !strings.Contains(err.Error(), "读取原密码失败") {
			t.Fatalf("期望读取原密码失败, got %v", err)
		}
	})

	t.Run("非交互环境读取新密码失败", func(t *testing.T) {
		_, dir, cfgFile := setupLoggedIn(t)
		withStdin(t, "")
		t.Setenv("DSYNC_OLD_PASSWORD", "old")
		t.Setenv("DSYNC_NEW_PASSWORD", "")
		_, _, err := execCLI(t, dir, cfgFile, "passwd")
		if err == nil || !strings.Contains(err.Error(), "读取新密码失败") {
			t.Fatalf("期望读取新密码失败, got %v", err)
		}
	})

	t.Run("接口失败", func(t *testing.T) {
		f, dir, cfgFile := setupLoggedIn(t)
		f.Override("PUT /api/v1/auth/password", func(w http.ResponseWriter, r *http.Request) {
			w.WriteHeader(http.StatusBadRequest)
			respondJSON(w, `{"error":"密码强度不足"}`)
		})
		_, _, err := execCLI(t, dir, cfgFile, "passwd")
		if err == nil || !strings.Contains(err.Error(), "密码强度不足") {
			t.Fatalf("期望接口错误, got %v", err)
		}
	})

	t.Run("写盘失败仅告警不报错", func(t *testing.T) {
		_, dir, cfgFile := setupLoggedIn(t)
		readOnlyFile(t, cfgFile)
		_, stderr, err := execCLI(t, dir, cfgFile, "passwd")
		if err != nil {
			t.Fatalf("写盘失败不应导致命令失败: %v", err)
		}
		if !strings.Contains(stderr, "本地凭据更新失败") {
			t.Errorf("stderr = %q", stderr)
		}
	})
}

// TestMainExitCode 在子进程中验证 main() 的错误退出码为 1。
func TestMainExitCode(t *testing.T) {
	if os.Getenv("DSYNC_TEST_MAIN_EXIT") == "1" {
		rootCmd.SetArgs([]string{"definitely-not-a-command"})
		main()
		return // main 正常返回则父进程断言失败（期望非零退出码）
	}
	cmd := exec.Command(os.Args[0], "-test.run=TestMainExitCode", "-test.timeout=60s")
	cmd.Env = append(os.Environ(), "DSYNC_TEST_MAIN_EXIT=1")
	err := cmd.Run()
	var exitErr *exec.ExitError
	if !errors.As(err, &exitErr) {
		t.Fatalf("期望非零退出码, got %v", err)
	}
}
