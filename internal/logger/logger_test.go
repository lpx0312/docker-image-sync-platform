package logger

import (
	"encoding/json"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
	"time"

	"docker-image-sync-platform/internal/config"

	"go.uber.org/zap"
)

// ---------------------------------------------------------------------------
// 轻量断言助手（不引入新依赖）
// ---------------------------------------------------------------------------

func eq(t *testing.T, want, got interface{}) {
	t.Helper()
	if !reflect.DeepEqual(want, got) {
		t.Fatalf("不相等:\n期望: %#v\n实际: %#v", want, got)
	}
}

func noErr(t *testing.T, err error) {
	t.Helper()
	if err != nil {
		t.Fatalf("期望无错误, 实际: %v", err)
	}
}

func contains(t *testing.T, s, sub string) {
	t.Helper()
	if !strings.Contains(s, sub) {
		t.Fatalf("%q 未包含 %q", s, sub)
	}
}

func isTrue(t *testing.T, v bool) {
	t.Helper()
	if !v {
		t.Fatalf("期望 true, 实际 false")
	}
}

// saveGlobals 保存并最终恢复全局 Logger 与 config.AppConfig，避免测试间污染
func saveGlobals(t *testing.T) {
	t.Helper()
	oldLogger := Logger
	oldAppConfig := config.AppConfig
	t.Cleanup(func() {
		Logger = oldLogger
		config.AppConfig = oldAppConfig
	})
}

// setLogConfig 构造并安装日志相关的最小 AppConfig
func setLogConfig(t *testing.T, level, filePath string, maxSize, maxBackups, maxAge int) {
	t.Helper()
	config.AppConfig = &config.Config{}
	config.AppConfig.Log = config.LogConfig{
		Level:      level,
		FilePath:   filePath,
		MaxSize:    maxSize,
		MaxBackups: maxBackups,
		MaxAge:     maxAge,
	}
}

// readLogFile 读取并返回日志文件全部内容
func readLogFile(t *testing.T, path string) string {
	t.Helper()
	b, err := os.ReadFile(path)
	noErr(t, err)
	return string(b)
}

// eachJSONLine 逐行解析 JSON 日志
func eachJSONLine(t *testing.T, content string, fn func(map[string]interface{})) {
	t.Helper()
	for _, line := range strings.Split(strings.TrimSpace(content), "\n") {
		line = strings.TrimSpace(line)
		if line == "" {
			continue
		}
		var m map[string]interface{}
		if err := json.Unmarshal([]byte(line), &m); err != nil {
			t.Fatalf("日志行不是合法 JSON: %v\n行内容: %s", err, line)
		}
		fn(m)
	}
}

// ---------------------------------------------------------------------------
// 单元 / e2e 测试
// ---------------------------------------------------------------------------

func TestInitLogger_WritesFile(t *testing.T) {
	saveGlobals(t)
	logFile := filepath.Join(tempLogDir(t), "logs", "app.log")
	setLogConfig(t, "info", logFile, 100, 3, 28)

	noErr(t, InitLogger())

	// Logger 已初始化且可获取
	if Logger == nil {
		t.Fatalf("Logger 应已初始化")
	}
	eq(t, Logger, GetLogger())

	Logger.Info("hello-logger", zap.String("key", "value-中文"))
	Sync()

	// e2e: 验证日志真实落盘，JSON 格式且包含关键字段
	content := readLogFile(t, logFile)
	contains(t, content, "hello-logger")
	contains(t, content, "value-中文")
	contains(t, content, "日志系统初始化完成")

	var sawInfo bool
	eachJSONLine(t, content, func(m map[string]interface{}) {
		// 自定义时间字段名 timestamp、大写级别、caller 信息
		if _, ok := m["timestamp"]; !ok {
			t.Fatalf("日志行缺少 timestamp 字段: %v", m)
		}
		if _, ok := m["caller"]; !ok {
			t.Fatalf("日志行缺少 caller 字段: %v", m)
		}
		if lv, _ := m["level"].(string); lv == "INFO" {
			sawInfo = true
		}
	})
	isTrue(t, sawInfo)
}

func TestInitLogger_LevelFiltering(t *testing.T) {
	t.Run("debug 级别全部落盘", func(t *testing.T) {
		saveGlobals(t)
		logFile := filepath.Join(tempLogDir(t), "app.log")
		setLogConfig(t, "debug", logFile, 100, 3, 28)
		noErr(t, InitLogger())

		Logger.Debug("dbg-msg")
		Logger.Info("info-msg")
		Logger.Warn("warn-msg")
		Logger.Error("err-msg")
		Sync()

		content := readLogFile(t, logFile)
		contains(t, content, "dbg-msg")
		contains(t, content, "info-msg")
		contains(t, content, "warn-msg")
		contains(t, content, "err-msg")
	})

	t.Run("warn 级别抑制 info 与 debug", func(t *testing.T) {
		saveGlobals(t)
		logFile := filepath.Join(tempLogDir(t), "app.log")
		setLogConfig(t, "warn", logFile, 100, 3, 28)
		noErr(t, InitLogger())

		Logger.Debug("dbg-msg")
		Logger.Info("info-msg")
		Logger.Warn("warn-msg")
		Logger.Error("err-msg")
		Sync()

		content := readLogFile(t, logFile)
		if strings.Contains(content, "dbg-msg") || strings.Contains(content, "info-msg") {
			t.Fatalf("warn 级别不应记录 debug/info 日志:\n%s", content)
		}
		contains(t, content, "warn-msg")
		contains(t, content, "err-msg")
	})

	t.Run("error 级别仅记录 error 且带堆栈", func(t *testing.T) {
		saveGlobals(t)
		logFile := filepath.Join(tempLogDir(t), "app.log")
		setLogConfig(t, "error", logFile, 100, 3, 28)
		noErr(t, InitLogger())

		Logger.Info("info-msg")
		Logger.Warn("warn-msg")
		Logger.Error("err-msg")
		Sync()

		content := readLogFile(t, logFile)
		if strings.Contains(content, "info-msg") || strings.Contains(content, "warn-msg") {
			t.Fatalf("error 级别不应记录 info/warn 日志:\n%s", content)
		}
		contains(t, content, "err-msg")
		contains(t, content, "stacktrace")
	})

	t.Run("无效级别回退为 info", func(t *testing.T) {
		saveGlobals(t)
		logFile := filepath.Join(tempLogDir(t), "app.log")
		setLogConfig(t, "verbose-invalid", logFile, 100, 3, 28)
		noErr(t, InitLogger())

		Logger.Debug("dbg-msg")
		Logger.Info("info-msg")
		Sync()

		content := readLogFile(t, logFile)
		if strings.Contains(content, "dbg-msg") {
			t.Fatalf("无效级别默认 info，不应记录 debug 日志:\n%s", content)
		}
		contains(t, content, "info-msg")
	})
}

func TestInitLogger_RelativeFilePath(t *testing.T) {
	saveGlobals(t)
	// 相对路径: 日志目录为当前目录，t.Chdir 到临时目录避免污染仓库
	t.Chdir(tempLogDir(t))
	setLogConfig(t, "info", "app.log", 100, 3, 28)

	noErr(t, InitLogger())
	Logger.Info("relative-path-msg")
	Sync()

	content := readLogFile(t, "app.log")
	contains(t, content, "relative-path-msg")
}

func TestInitLogger_MkdirFailure(t *testing.T) {
	saveGlobals(t)
	// 用一个普通文件占用目录位置，MkdirAll 必然失败
	dir := t.TempDir()
	blocker := filepath.Join(dir, "blocker")
	noErr(t, os.WriteFile(blocker, []byte("i am a file"), 0644))
	setLogConfig(t, "info", filepath.Join(blocker, "app.log"), 100, 3, 28)

	err := InitLogger()
	if err == nil {
		t.Fatalf("期望 MkdirAll 失败返回错误")
	}
	// 失败时不应覆盖已有 Logger
	if Logger != nil {
		t.Fatalf("初始化失败不应设置 Logger")
	}
}

func TestInitLogger_RepeatedInit(t *testing.T) {
	saveGlobals(t)
	dir1 := filepath.Join(tempLogDir(t), "a.log")
	dir2 := filepath.Join(tempLogDir(t), "b.log")

	setLogConfig(t, "info", dir1, 100, 3, 28)
	noErr(t, InitLogger())
	first := Logger
	Logger.Info("to-first-file")

	// 重复初始化： 替换 Logger，写新文件（当前实际行为）
	setLogConfig(t, "info", dir2, 100, 3, 28)
	noErr(t, InitLogger())
	if Logger == first {
		t.Fatalf("重复初始化应替换 Logger 实例")
	}
	Logger.Info("to-second-file")
	Sync()

	contains(t, readLogFile(t, dir1), "to-first-file")
	contains(t, readLogFile(t, dir2), "to-second-file")
}

func TestSync_NilSafe(t *testing.T) {
	saveGlobals(t)
	// Logger 未初始化时 Sync 应安全返回
	Logger = nil
	Sync() // 不应 panic
}

func TestGetLogger_Uninitialized(t *testing.T) {
	saveGlobals(t)
	Logger = nil
	if GetLogger() != nil {
		t.Fatalf("未初始化时 GetLogger 应返回 nil")
	}
}

// tempLogDir 手动创建临时目录并注册“尽力清理”。
// 说明: 生产代码中 lumberjack 会长期持有当前日志文件句柄且无 Close 途径，
// Windows 下 t.TempDir 的强制清理会因文件占用而使测试失败，故此处忽略清理错误。
func tempLogDir(t *testing.T) string {
	t.Helper()
	dir, err := os.MkdirTemp("", "logger-test-*")
	noErr(t, err)
	t.Cleanup(func() { _ = os.RemoveAll(dir) })
	return dir
}

// quietStdout 在测试期间把 os.Stdout 重定向到 DevNull。
// InitLogger 的控制台输出核心在初始化时捕获 os.Stdout，因此须在 InitLogger 之前调用。
func quietStdout(t *testing.T) {
	t.Helper()
	devnull, err := os.OpenFile(os.DevNull, os.O_WRONLY, 0)
	noErr(t, err)
	orig := os.Stdout
	os.Stdout = devnull
	t.Cleanup(func() {
		os.Stdout = orig
		_ = devnull.Close()
	})
}

// e2e: 验证基于大小限制的日志轮转与备份保留
func TestInitLogger_Rotation(t *testing.T) {
	saveGlobals(t)
	// lumberjack 会一直持有当前日志文件句柄（生产代码无 Close 途径），
	// Windows 下 t.TempDir 的强制清理会因文件占用而失败，故用手动目录 + 尽力清理
	dir, err := os.MkdirTemp("", "logger-rotation-*")
	noErr(t, err)
	t.Cleanup(func() { _ = os.RemoveAll(dir) })

	logFile := filepath.Join(dir, "app.log")
	// MaxSize=1MB, MaxBackups=0（不限制备份数量）
	setLogConfig(t, "info", logFile, 1, 0, 0)
	quietStdout(t) // 该测试写入 2000+ 条日志，屏蔽控制台输出避免刷屏
	noErr(t, InitLogger())

	payload := strings.Repeat("x", 900)
	for i := 0; i < 2200; i++ {
		Logger.Info("rotation-test-entry", zap.Int("i", i), zap.String("payload", payload))
	}
	Sync()

	entries, err := os.ReadDir(dir)
	noErr(t, err)

	var backups int
	for _, e := range entries {
		name := e.Name()
		if strings.HasPrefix(name, "app-") &&
			(strings.HasSuffix(name, ".log") || strings.HasSuffix(name, ".log.gz")) {
			backups++
		}
	}
	if backups < 1 {
		t.Fatalf("写入超过 MaxSize 后应产生轮转备份文件, 目录内容: %v", entries)
	}

	// 当前主日志文件仍存在且可读
	content := readLogFile(t, logFile)
	contains(t, content, "rotation-test-entry")

	// 备份文件（或其压缩形式）等待异步压缩完成，最多 3 秒
	deadline := time.Now().Add(3 * time.Second)
	hasGz := false
	for time.Now().Before(deadline) {
		entries, _ := os.ReadDir(dir)
		for _, e := range entries {
			if strings.HasSuffix(e.Name(), ".gz") {
				hasGz = true
			}
		}
		if hasGz {
			break
		}
		time.Sleep(50 * time.Millisecond)
	}
	isTrue(t, hasGz)
}
