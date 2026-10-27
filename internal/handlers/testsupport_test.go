package handlers

// 共享测试脚手架：本包所有 _test.go 复用，禁止在其他测试文件中重复定义
// TestMain / newTestDB / withGlobalDB / seedRBAC，避免符号冲突。
// 各测试文件如需私有辅助函数，请以自己覆盖的源文件为名做前缀（如 syncXxx、authXxx）。

import (
	"os"
	"path/filepath"
	"testing"

	"docker-image-sync-platform/internal/database"
	"docker-image-sync-platform/internal/logger"
	"docker-image-sync-platform/internal/models"

	"github.com/glebarez/sqlite"
	"github.com/gin-gonic/gin"
	"go.uber.org/zap"
	"gorm.io/gorm"
	gl "gorm.io/gorm/logger"
)

func TestMain(m *testing.M) {
	gin.SetMode(gin.TestMode)
	if logger.Logger == nil {
		logger.Logger = zap.NewNop()
	}
	os.Exit(m.Run())
}

// newTestDB 打开文件型 SQLite（TempDir）并迁移全部模型。
// 不用 :memory:：单连接的内存库会自死锁——buildBatchSyncTask 在
// db.Transaction（占用唯一连接）内再做亲和性查询需要第二个连接；
// 文件库允许 MaxOpenConns>1，与生产 MySQL 连接池语义一致。
// 生产模型的 MySQL enum 列标签在 sqlite 上不可直接建表，先以兼容 DDL 预建
// 两张 enum 表（裸 enum 类型 + 双引号默认值，GORM 比较列类型用 HasPrefix 即视为一致），
// 再 AutoMigrate 全模型。
func newTestDB(t *testing.T) *gorm.DB {
	t.Helper()
	dsn := "file:" + filepath.Join(t.TempDir(), "test.db") + "?_pragma=busy_timeout(5000)"
	db, err := gorm.Open(sqlite.Open(dsn), &gorm.Config{
		Logger:                                   gl.Default.LogMode(gl.Silent),
		DisableForeignKeyConstraintWhenMigrating: true,
	})
	if err != nil {
		t.Fatalf("打开 SQLite 失败: %v", err)
	}
	sqlDB, err := db.DB()
	if err != nil {
		t.Fatalf("获取底层连接失败: %v", err)
	}
	sqlDB.SetMaxOpenConns(4)
	sqlDB.SetMaxIdleConns(4)
	// 先关连接池再让 TempDir 删文件（Cleanup LIFO：本注册晚于 TempDir，先执行），
	// 否则 Windows 下文件句柄未释放导致 RemoveAll 失败
	t.Cleanup(func() { _ = sqlDB.Close() })

	pre := func(ddl string) {
		if err := db.Exec(ddl).Error; err != nil {
			t.Fatalf("预建表失败: %v", err)
		}
	}
	pre(`CREATE TABLE IF NOT EXISTS image_sync_records (
		id integer PRIMARY KEY AUTOINCREMENT,
		original_image varchar(500) NOT NULL,
		acr_image varchar(500),
		tag varchar(100),
		architecture varchar(50) DEFAULT "",
		acr_architectures text,
		acr_registry_id integer,
		original_input varchar(600),
		input_order integer DEFAULT 0,
		sync_status enum DEFAULT "pending",
		error_message text,
		description varchar(500),
		task_id varchar(100),
		priority integer DEFAULT 0,
		started_at datetime,
		completed_at datetime,
		duration integer DEFAULT 0,
		image_size integer DEFAULT 0,
		created_at datetime,
		updated_at datetime,
		deleted_at datetime
	)`)
	pre(`CREATE TABLE IF NOT EXISTS sync_tasks (
		id integer PRIMARY KEY AUTOINCREMENT,
		task_id varchar(100) NOT NULL,
		images_json text,
		status enum DEFAULT "pending",
		github_action_url varchar(500),
		github_run_id varchar(100),
		commit_sha varchar(100),
		started_at datetime,
		completed_at datetime,
		error_message text,
		acr_registry_id integer DEFAULT 0,
		description varchar(500),
		max_concurrent integer DEFAULT 3,
		total_images integer DEFAULT 0,
		completed_images integer DEFAULT 0,
		failed_images integer DEFAULT 0,
		progress decimal(5,2) DEFAULT 0.00,
		created_at datetime,
		updated_at datetime,
		deleted_at datetime
	)`)

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

// seedRBAC 写入三种系统角色并返回 admin 角色 ID
func seedRBAC(t *testing.T, db *gorm.DB) uint {
	t.Helper()
	admin := models.Role{Code: models.RoleAdmin, Name: "管理员", Description: "管理员", IsSystem: true}
	op := models.Role{Code: models.RoleOperator, Name: "运维员", Description: "运维员", IsSystem: true}
	user := models.Role{Code: models.RoleUser, Name: "普通用户", Description: "普通用户", IsSystem: true}
	for _, r := range []*models.Role{&admin, &op, &user} {
		if err := db.Create(r).Error; err != nil {
			t.Fatalf("创建角色 %s 失败: %v", r.Code, err)
		}
	}
	return admin.ID
}
