//go:build repro

// 临时复现测试：在"旧版结构"的 MySQL 库上跑 AutoMigrate，复现
// Error 1091: Can't DROP FOREIGN KEY `uni_permissions_code`。
// 用法: REPRO_DSN="root:xx@tcp(127.0.0.1:13306)/docker_sync?..." \
//       go test -tags repro ./internal/database -run TestReproLegacyMySQL -v
// 验证完成后删除本文件。

package database

import (
	"os"
	"testing"

	"docker-image-sync-platform/internal/config"

	"gorm.io/driver/mysql"
	"gorm.io/gorm"
	"gorm.io/gorm/logger"
)

func TestReproLegacyMySQL(t *testing.T) {
	dsn := os.Getenv("REPRO_DSN")
	if dsn == "" {
		t.Skip("REPRO_DSN 未设置")
	}
	if config.AppConfig == nil {
		if err := config.LoadConfig(""); err != nil {
			t.Fatalf("加载配置失败: %v", err)
		}
	}
	db, err := gorm.Open(mysql.Open(dsn), &gorm.Config{
		Logger:                                   logger.Default.LogMode(logger.Info),
		DisableForeignKeyConstraintWhenMigrating: true,
	})
	if err != nil {
		t.Fatalf("连接失败: %v", err)
	}
	DB = db
	if err := AutoMigrate(); err != nil {
		t.Fatalf("AutoMigrate 失败: %v", err)
	}
}
