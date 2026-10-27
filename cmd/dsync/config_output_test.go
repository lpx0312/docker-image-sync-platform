package main

// config.go 与 output.go 的单元/边界测试。

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

// clearUserConfigEnv 让 os.UserConfigDir() 失败（跨平台：
// Windows 清空 AppData，POSIX 清空 XDG_CONFIG_HOME 与 HOME）。
func clearUserConfigEnv(t *testing.T) {
	t.Helper()
	t.Setenv("AppData", "")
	t.Setenv("XDG_CONFIG_HOME", "")
	t.Setenv("HOME", "")
}

// isolateGlobals 保存全局可变状态并在测试结束后恢复。
func isolateGlobals(t *testing.T) {
	t.Helper()
	oldPath, oldOverride := cfgPath, configDirOverride
	t.Cleanup(func() { cfgPath, configDirOverride = oldPath, oldOverride })
}

func TestConfigDirOverride(t *testing.T) {
	isolateGlobals(t)
	dir := t.TempDir()
	configDirOverride = dir
	got, err := configDir()
	if err != nil {
		t.Fatalf("configDir 报错: %v", err)
	}
	if got != dir {
		t.Errorf("configDir = %q, 期望 %q", got, dir)
	}
}

func TestConfigDirUserConfigError(t *testing.T) {
	isolateGlobals(t)
	configDirOverride = ""
	clearUserConfigEnv(t)
	if _, err := configDir(); err == nil {
		t.Error("无法确定配置目录时应报错")
	}
}

func TestConfigDirMkdirFail(t *testing.T) {
	isolateGlobals(t)
	configDirOverride = ""
	// 让 UserConfigDir 指向一个普通文件，其下创建目录必然失败
	file := filepath.Join(t.TempDir(), "plain-file")
	if err := os.WriteFile(file, []byte("x"), 0o600); err != nil {
		t.Fatal(err)
	}
	t.Setenv("AppData", file)
	t.Setenv("XDG_CONFIG_HOME", file)
	if _, err := configDir(); err == nil {
		t.Error("配置目录创建失败时应报错")
	}
}

func TestConfigFilePath(t *testing.T) {
	isolateGlobals(t)

	t.Run("cfgPath 覆盖", func(t *testing.T) {
		cfgPath = "/custom/config.json"
		if got := configFilePath(); got != "/custom/config.json" {
			t.Errorf("configFilePath = %q, 期望 /custom/config.json", got)
		}
	})

	t.Run("configDir 失败回退当前目录", func(t *testing.T) {
		cfgPath = ""
		configDirOverride = ""
		clearUserConfigEnv(t)
		if got := configFilePath(); got != "dsync-config.json" {
			t.Errorf("configFilePath = %q, 期望 dsync-config.json", got)
		}
	})

	t.Run("默认配置目录拼接", func(t *testing.T) {
		cfgPath = ""
		dir := t.TempDir()
		configDirOverride = dir
		want := filepath.Join(dir, "config.json")
		if got := configFilePath(); got != want {
			t.Errorf("configFilePath = %q, 期望 %q", got, want)
		}
	})
}

func TestLoadConfig(t *testing.T) {
	isolateGlobals(t)
	dir := t.TempDir()
	configDirOverride = dir

	t.Run("文件不存在返回空配置", func(t *testing.T) {
		cfgPath = filepath.Join(dir, "missing.json")
		c, err := loadConfig()
		if err != nil {
			t.Fatalf("不应报错: %v", err)
		}
		if c == nil || c.Server != "" || c.Token != "" {
			t.Errorf("期望空配置, 得到 %+v", c)
		}
	})

	t.Run("畸形 JSON 报错", func(t *testing.T) {
		p := filepath.Join(dir, "bad.json")
		os.WriteFile(p, []byte("{not-json"), 0o600)
		cfgPath = p
		if _, err := loadConfig(); err == nil {
			t.Error("畸形 JSON 应报错")
		}
	})

	t.Run("正常读取", func(t *testing.T) {
		p := filepath.Join(dir, "ok.json")
		os.WriteFile(p, []byte(`{"server":"http://x","username":"u","token":"t"}`), 0o600)
		cfgPath = p
		c, err := loadConfig()
		if err != nil {
			t.Fatal(err)
		}
		if c.Server != "http://x" || c.Username != "u" || c.Token != "t" {
			t.Errorf("读取结果 = %+v", c)
		}
	})
}

func TestSaveConfig(t *testing.T) {
	isolateGlobals(t)
	dir := t.TempDir()
	configDirOverride = dir

	t.Run("写入并回读", func(t *testing.T) {
		p := filepath.Join(dir, "c1.json")
		cfgPath = p
		in := &Config{Server: "http://srv", Username: "u", Password: "p", Token: "tk", TokenExpiresAt: "exp"}
		if err := saveConfig(in); err != nil {
			t.Fatalf("saveConfig: %v", err)
		}
		out, err := loadConfig()
		if err != nil {
			t.Fatal(err)
		}
		if *out != *in {
			t.Errorf("回读 = %+v, 期望 %+v", out, in)
		}
	})

	t.Run("路径是目录时写入失败", func(t *testing.T) {
		cfgPath = dir // 目录本身
		if err := saveConfig(&Config{Server: "x"}); err == nil {
			t.Error("写入目录应报错")
		}
	})
}

func TestClearAuth(t *testing.T) {
	c := &Config{Server: "http://srv", Username: "u", Password: "p", Token: "t", TokenExpiresAt: "e"}
	clearAuth(c)
	if c.Username != "" || c.Password != "" || c.Token != "" || c.TokenExpiresAt != "" {
		t.Errorf("凭据未清除: %+v", c)
	}
	if c.Server != "http://srv" {
		t.Error("clearAuth 不应清除服务器地址")
	}
}

func TestToString(t *testing.T) {
	if got := toString("abc"); got != "abc" {
		t.Errorf("toString(string) = %q", got)
	}
	if got := toString(123); got != "" {
		t.Errorf("toString(int) = %q, 期望空串", got)
	}
	if got := toString(nil); got != "" {
		t.Errorf("toString(nil) = %q, 期望空串", got)
	}
}

// ---- output.go ----

func TestPaint(t *testing.T) {
	old := colorEnabled
	t.Cleanup(func() { colorEnabled = old })

	colorEnabled = false
	if got := paint(colorRed, "x"); got != "x" {
		t.Errorf("关闭颜色时应原样返回, got %q", got)
	}
	colorEnabled = true
	if got := paint(colorRed, "x"); got != colorRed+"x"+colorReset {
		t.Errorf("开启颜色时应包裹 ANSI 码, got %q", got)
	}
}

func TestColorStatus(t *testing.T) {
	old := colorEnabled
	t.Cleanup(func() { colorEnabled = old })
	colorEnabled = true

	cases := map[string]string{
		"success":         colorGreen,
		"completed":       colorGreen,
		"failed":          colorRed,
		"partial_success": colorYellow,
		"syncing":         colorBlue,
		"running":         colorBlue,
		"retrying":        colorBlue,
		"paused":          colorGray,
		"":                colorGray,
	}
	for status, wantColor := range cases {
		got := colorStatus(status)
		if !strings.HasPrefix(got, wantColor) {
			t.Errorf("colorStatus(%q) = %q, 期望前缀 %q", status, got, wantColor)
		}
	}
	// 大小写不敏感
	if got := colorStatus("SUCCESS"); !strings.HasPrefix(got, colorGreen) {
		t.Errorf("colorStatus 应忽略大小写, got %q", got)
	}
}

func TestEmitJSON(t *testing.T) {
	out := captureStdout(t, func() {
		if err := emitJSON(map[string]int{"a": 1}); err != nil {
			t.Errorf("emitJSON: %v", err)
		}
	})
	var m map[string]int
	if err := json.Unmarshal([]byte(out), &m); err != nil {
		t.Fatalf("输出不是合法 JSON: %v\n%s", err, out)
	}
	if m["a"] != 1 {
		t.Errorf("输出 = %v", m)
	}
}

func TestEmitOrPrint(t *testing.T) {
	oldJSON := jsonOut
	t.Cleanup(func() { jsonOut = oldJSON })

	t.Run("文本模式执行渲染函数", func(t *testing.T) {
		jsonOut = false
		called := false
		out := captureStdout(t, func() {
			_ = emitOrPrint(map[string]int{"k": 1}, func() { called = true })
		})
		if !called {
			t.Error("文本模式应调用渲染函数")
		}
		if out != "" {
			t.Errorf("渲染函数无输出时不应有内容: %q", out)
		}
	})

	t.Run("json 模式输出 JSON 且不执行渲染", func(t *testing.T) {
		jsonOut = true
		called := false
		out := captureStdout(t, func() {
			if err := emitOrPrint(map[string]int{"k": 2}, func() { called = true }); err != nil {
				t.Fatal(err)
			}
		})
		if called {
			t.Error("json 模式不应调用渲染函数")
		}
		if !strings.Contains(out, `"k": 2`) {
			t.Errorf("json 输出异常: %q", out)
		}
	})
}

func TestPrintTable(t *testing.T) {
	out := captureStdout(t, func() {
		printTable([]string{"H1", "H2"}, [][]string{{"a", "b"}, {"c", "d"}})
	})
	for _, want := range []string{"H1", "H2", "a", "b", "c", "d"} {
		if !strings.Contains(out, want) {
			t.Errorf("表格输出缺少 %q:\n%s", want, out)
		}
	}
}

func TestShortID(t *testing.T) {
	if got := shortID("abcdefghijklmnop"); got != "abcdefgh" {
		t.Errorf("shortID(长) = %q", got)
	}
	if got := shortID("short"); got != "short" {
		t.Errorf("shortID(短) = %q", got)
	}
	if got := shortID(""); got != "" {
		t.Errorf("shortID(空) = %q", got)
	}
}

func TestHumanDuration(t *testing.T) {
	if got := humanDuration(59 * time.Second); got != "59s" {
		t.Errorf("humanDuration(59s) = %q", got)
	}
	if got := humanDuration(0); got != "0s" {
		t.Errorf("humanDuration(0) = %q", got)
	}
	if got := humanDuration(192 * time.Second); got != "3m12s" {
		t.Errorf("humanDuration(192s) = %q, 期望 3m12s", got)
	}
}

func TestHumanBytes(t *testing.T) {
	cases := []struct {
		n    int64
		want string
	}{
		{0, "0 B"},
		{1023, "1023 B"},
		{1024, "1.0 KiB"},
		{12 * 1024 * 1024, "12.0 MiB"},
		{3 * 1024 * 1024 * 1024, "3.0 GiB"},
		{2 * 1024 * 1024 * 1024 * 1024, "2.0 TiB"},
	}
	for _, c := range cases {
		if got := humanBytes(c.n); got != c.want {
			t.Errorf("humanBytes(%d) = %q, 期望 %q", c.n, got, c.want)
		}
	}
}

func TestShortDigest(t *testing.T) {
	long := "sha256:0123456789abcdef0123456789abcdef"
	if got := shortDigest(long); got != long[:19] {
		t.Errorf("shortDigest(长) = %q", got)
	}
	if got := shortDigest("sha256:abc"); got != "sha256:abc" {
		t.Errorf("shortDigest(短) = %q", got)
	}
}

// ---- apitypes.go ----

func TestSuggestReasonText(t *testing.T) {
	cases := map[string]string{
		"affinity":     "仓库已有归属（亲和性）",
		"default":      "平台默认 ACR",
		"alternate":    "默认 ACR 已满，自动换用",
		"default_full": "默认 ACR 已满（回退使用）",
		"fallback":     "无可用配额，兜底选择",
		"unknown_x":    "unknown_x", // 未知枚举原样返回
	}
	for in, want := range cases {
		if got := suggestReasonText(in); got != want {
			t.Errorf("suggestReasonText(%q) = %q, 期望 %q", in, got, want)
		}
	}
}
