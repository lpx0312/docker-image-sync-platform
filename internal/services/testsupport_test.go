package services

// 共享测试脚手架：本包所有 _test.go 复用，禁止在其他测试文件中重复定义
// TestMain / newTestDB / withGlobalDB，避免符号冲突。
// 各测试文件如需私有辅助函数，请以自己覆盖的源文件为名做前缀（如 gitXxx、githubXxx、acrXxx）。

import (
	"os"
	"testing"

	"docker-image-sync-platform/internal/database"
	"docker-image-sync-platform/internal/logger"
	"docker-image-sync-platform/internal/models"

	"github.com/glebarez/sqlite"
	"github.com/gin-gonic/gin"
	"go.uber.org/zap"
	"gorm.io/gorm"
)

func TestMain(m *testing.M) {
	gin.SetMode(gin.TestMode)
	if logger.Logger == nil {
		logger.Logger = zap.NewNop()
	}
	os.Exit(m.Run())
}

// newTestDB 打开内存 SQLite 并迁移全部模型
func newTestDB(t *testing.T) *gorm.DB {
	t.Helper()
	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
	if err != nil {
		t.Fatalf("打开内存 SQLite 失败: %v", err)
	}
	if err := db.AutoMigrate(
		&models.Permission{}, &models.Role{},
		&models.ImageSyncRecord{}, &models.SyncTask{}, &models.SystemConfig{},
		&models.User{}, &models.LoginLog{},
		&models.AcrRegistry{}, &models.AcrRepository{},
	); err != nil {
		t.Fatalf("AutoMigrate 失败: %v", err)
	}
	return db
}

// withGlobalDB 将全局 database.DB 临时替换为测试库，测试结束自动恢复
func withGlobalDB(t *testing.T, db *gorm.DB) {
	t.Helper()
	saved := database.DB
	database.DB = db
	t.Cleanup(func() { database.DB = saved })
}
