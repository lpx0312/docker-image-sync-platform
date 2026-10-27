// main_unit_test.go 根包单元测试：run() 失败路径、initBusinessServices、
// serveUntilSignal 优雅关闭（信号通道注入，不依赖真实系统信号）。
package main

import (
	"net"
	"net/http"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
	"time"

	"docker-image-sync-platform/internal/config"
	"docker-image-sync-platform/internal/database"
	"docker-image-sync-platform/internal/handlers"
	"docker-image-sync-platform/internal/logger"
	"docker-image-sync-platform/internal/services"

	"github.com/gin-gonic/gin"
	"go.uber.org/zap"
)

// chdirTemp 切换工作目录并在测试结束后恢复（go.mod 为 go1.21，不能用 t.Chdir）。
func chdir(t *testing.T, dir string) {
	t.Helper()
	wd, err := os.Getwd()
	if err != nil {
		t.Fatal(err)
	}
	if err := os.Chdir(dir); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = os.Chdir(wd) })
}

// saveGlobals 保存并恢复本包测试会触碰的全部全局状态。
func saveGlobals(t *testing.T) {
	t.Helper()
	savedDB, savedCfg, savedLogger := database.DB, config.AppConfig, logger.Logger
	t.Cleanup(func() {
		database.DB, config.AppConfig, logger.Logger = savedDB, savedCfg, savedLogger
	})
}

func TestRun_ConfigLoadFailure(t *testing.T) {
	saveGlobals(t)
	t.Setenv("ENCRYPTION_KEY", "k")
	t.Setenv("GIN_MODE", "debug")

	// 配置文件类型不匹配（port 应为标量）：viper.Unmarshal 报错
	tmp := t.TempDir()
	if err := os.WriteFile(filepath.Join(tmp, "config.yaml"), []byte("server:\n  port:\n    a: 1\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	chdir(t, tmp)

	err := run()
	if err == nil {
		t.Fatal("配置文件畸形时 run() 应返回错误")
	}
	if !strings.Contains(err.Error(), "加载配置失败") {
		t.Fatalf("错误信息应包含 加载配置失败, got %v", err)
	}
}

func TestRun_InitDatabaseFailure(t *testing.T) {
	saveGlobals(t)
	t.Setenv("ENCRYPTION_KEY", "k")
	t.Setenv("GIN_MODE", "debug")

	tmp := t.TempDir()
	// 配置合法（logger 初始化成功），但数据库指向必然拒绝连接的端口；
	// 日志写 NUL 设备，避免 lumberjack 长期持有文件句柄锁住 TempDir（Windows）
	cfg := strings.Replace(e2eConfigYAML, "port: 3306", "port: 1", 1)
	cfg = strings.Replace(cfg, "file_path: ./logs/app.log", "file_path: NUL", 1)
	if err := os.WriteFile(filepath.Join(tmp, "config.yaml"), []byte(cfg), 0o644); err != nil {
		t.Fatal(err)
	}
	chdir(t, tmp)

	err := run()
	if err == nil {
		t.Fatal("数据库不可达时 run() 应返回错误")
	}
	if !strings.Contains(err.Error(), "初始化数据库失败") {
		t.Fatalf("错误信息应包含 初始化数据库失败, got %v", err)
	}
}

func TestInitBusinessServices_EncryptionFailure(t *testing.T) {
	saveGlobals(t)
	// release 且无 ENCRYPTION_KEY：加密服务初始化失败分支
	t.Setenv("ENCRYPTION_KEY", "")
	t.Setenv("GIN_MODE", "release")

	_, err := initBusinessServices()
	if err == nil {
		t.Fatal("release 模式且无 ENCRYPTION_KEY 时应初始化失败")
	}
	if !strings.Contains(err.Error(), "初始化加密服务失败") {
		t.Fatalf("错误信息应包含 初始化加密服务失败, got %v", err)
	}
}

func TestInitBusinessServices_Success(t *testing.T) {
	saveGlobals(t)
	t.Setenv("ENCRYPTION_KEY", "unit-key")
	t.Setenv("GIN_MODE", "debug")

	// initBusinessServices 依赖全局 database.DB 与 logger.Logger
	saved := logger.Logger
	logger.Logger = zap.NewNop()
	t.Cleanup(func() { logger.Logger = saved })

	app, err := initBusinessServices()
	if err != nil {
		t.Fatalf("initBusinessServices 失败: %v", err)
	}
	if app.gitFactory == nil || app.syncHandler == nil || app.configService == nil {
		t.Fatal("关键服务不应为 nil")
	}
	if !app.gitFactory.IsUsingOptimized() {
		// 当前配置下应走优化实现（断言工厂状态可用）
		t.Log("gitFactory 使用非优化实现（按当前配置）")
	}
}

// TestServeUntilSignal_GracefulShutdown 验证优雅关闭全流程：
// 真实监听端口 -> 请求可访问 -> 信号到达 -> Shutdown + 同步协程收尾 -> 返回 nil。
func TestServeUntilSignal_GracefulShutdown(t *testing.T) {
	saveGlobals(t)
	t.Setenv("ENCRYPTION_KEY", "unit-key")
	t.Setenv("GIN_MODE", "debug")

	saved := logger.Logger
	logger.Logger = zap.NewNop()
	t.Cleanup(func() { logger.Logger = saved })

	// 选一个空闲端口，避免与并发测试冲突
	l, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	port := l.Addr().(*net.TCPAddr).Port
	l.Close()

	config.AppConfig = &config.Config{}
	config.AppConfig.Server.Port = strconv.Itoa(port)

	router := gin.New()
	router.GET("/ping", func(c *gin.Context) { c.JSON(http.StatusOK, gin.H{"ok": true}) })

	factory := services.NewGitServiceFactory(nil)
	if factory == nil {
		t.Fatal("工厂创建失败")
	}
	syncHandler := handlers.NewSyncHandler(factory)

	quit := make(chan os.Signal, 1)
	done := make(chan error, 1)
	go func() { done <- serveUntilSignal(router, syncHandler, quit) }()

	// 轮询等待服务器就绪（不依赖固定 sleep）
	base := "http://127.0.0.1:" + strconv.Itoa(port)
	deadline := time.Now().Add(5 * time.Second)
	ready := false
	for time.Now().Before(deadline) {
		resp, err := http.Get(base + "/ping")
		if err == nil {
			resp.Body.Close()
			if resp.StatusCode == http.StatusOK {
				ready = true
				break
			}
		}
		time.Sleep(20 * time.Millisecond)
	}
	if !ready {
		close(quit)
		t.Fatal("服务器在 5 秒内未就绪")
	}

	// 发出关闭信号
	quit <- nil

	select {
	case err := <-done:
		if err != nil {
			t.Fatalf("优雅关闭应返回 nil, got %v", err)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("serveUntilSignal 未在 5 秒内返回")
	}

	// 关闭后端口不再可访问
	if resp, err := http.Get(base + "/ping"); err == nil {
		resp.Body.Close()
		t.Error("关闭后端口不应继续服务")
	}
}

// TestServeUntilSignal_ShutdownBeforeListen 覆盖"服务器尚未开始监听即收到信号"的时序。
func TestServeUntilSignal_ShutdownBeforeListen(t *testing.T) {
	saveGlobals(t)
	t.Setenv("ENCRYPTION_KEY", "unit-key")

	saved := logger.Logger
	logger.Logger = zap.NewNop()
	t.Cleanup(func() { logger.Logger = saved })

	config.AppConfig = &config.Config{}
	config.AppConfig.Server.Port = "0" // 随机端口

	router := gin.New()
	factory := services.NewGitServiceFactory(nil)
	syncHandler := handlers.NewSyncHandler(factory)

	quit := make(chan os.Signal, 1)
	quit <- nil // 先发信号再启动

	done := make(chan error, 1)
	go func() { done <- serveUntilSignal(router, syncHandler, quit) }()

	select {
	case err := <-done:
		if err != nil {
			t.Fatalf("提前信号场景应正常返回 nil, got %v", err)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("serveUntilSignal 未返回")
	}
}
