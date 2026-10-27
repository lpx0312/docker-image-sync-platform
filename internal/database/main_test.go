package database

import (
	"crypto/sha256"
	"os"
	"testing"

	"docker-image-sync-platform/internal/config"
	"docker-image-sync-platform/internal/logger"
	"docker-image-sync-platform/internal/models"

	"github.com/glebarez/sqlite"
	"github.com/stretchr/testify/require"
	"go.uber.org/zap"
	"gorm.io/gorm"
	gl "gorm.io/gorm/logger"
)

// TestMain 兜底初始化全局 logger。
// migrations.go 中的 RunMigrations 直接使用全局 logger.Logger，
// 测试进程中不经过 main.go 的初始化流程，必须在此保证其非 nil，否则 nil pointer panic。
func TestMain(m *testing.M) {
	if logger.Logger == nil {
		logger.Logger = zap.NewNop()
	}
	os.Exit(m.Run())
}

// openSQLiteDB 打开内存 sqlite 数据库。
//
// 关键点：
//  1. :memory: 数据库在 database/sql 连接池里每个物理连接各自独立，
//     因此必须限制 MaxOpenConns(1)，否则 GORM 借出第二个连接时会看到"空库"。
//  2. 连接参数与生产 InitDatabase 保持一致（DisableForeignKeyConstraintWhenMigrating），
//     避免为 acr_registry_id=0 等哨兵值创建外键。
func openSQLiteDB(t *testing.T) *gorm.DB {
	t.Helper()
	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{
		Logger:                                   gl.Default.LogMode(gl.Silent),
		DisableForeignKeyConstraintWhenMigrating: true,
	})
	require.NoError(t, err)
	sqlDB, err := db.DB()
	require.NoError(t, err)
	sqlDB.SetMaxOpenConns(1)
	return db
}

// setupTestDB 建立隔离的测试环境：
// 保存并恢复全局 DB / config.AppConfig，清空影响加密行为的两个环境变量。
// 每个测试拿到独立的内存库与零值配置，避免相互污染。
func setupTestDB(t *testing.T, cfg *config.Config) *gorm.DB {
	t.Helper()
	if cfg == nil {
		cfg = &config.Config{}
	}
	oldDB, oldCfg := DB, config.AppConfig
	t.Cleanup(func() {
		DB, config.AppConfig = oldDB, oldCfg
	})
	DB = openSQLiteDB(t)
	config.AppConfig = cfg

	t.Setenv("ENCRYPTION_KEY", "")
	t.Setenv("GIN_MODE", "")
	return DB
}

// preCreateMySQLCompatTables 用 sqlite 兼容 DDL 预建两张带 MySQL enum 标签的表。
//
// 生产模型 ImageSyncRecord.SyncStatus / SyncTask.Status 使用
// `type:enum('pending',...)` MySQL 专有列类型，GORM 在 sqlite 上会原样生成
// `CREATE TABLE ... enum('pending',...)`，sqlite 解析器拒绝带字符串字面量的列类型，
// 因此内存库上直接跑 AutoMigrate() 必然失败。
//
// 预建表时做了两处对齐，让 GORM 的"智能迁移"认为表已符合模型、不触发 ALTER：
//  1. 枚举列声明为裸 `enum` 类型 —— 迁移比较满足 strings.HasPrefix("enum('pending',...)", "enum")；
//  2. 字符串默认值用双引号（DEFAULT "pending"）—— 驱动解析 DDL 时只剥双引号，
//     与 GORM 从 tag 中剥引号后的 DefaultValue 字符串完全相等。
func preCreateMySQLCompatTables(t *testing.T, db *gorm.DB) {
	t.Helper()
	err := db.Exec(`CREATE TABLE IF NOT EXISTS image_sync_records (
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
	)`).Error
	require.NoError(t, err)

	err = db.Exec(`CREATE TABLE IF NOT EXISTS sync_tasks (
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
	)`).Error
	require.NoError(t, err)
}

// fullTestConfig 返回各字段均有值的配置，用于验证配置值真正落库与加密行为。
func fullTestConfig() *config.Config {
	return &config.Config{
		Aliyun: config.AliyunConfig{
			Registry:  "registry.example.com",
			Namespace: "myns",
			Username:  "ali-user",
			Password:  "ali-pass",
		},
		Git: config.GitConfig{
			RepositoryType: "github",
			Gitee: config.GiteeConfig{
				RepoURL:  "https://gitee.com/u/r",
				Username: "gitee-user",
				Password: "gitee-pass",
				Token:    "gitee-token",
				Email:    "gitee@example.com",
				Branch:   "dev",
			},
			GitHub: config.GitHubConfig{
				RepoURL:  "https://github.com/u/r",
				Username: "github-user",
				Token:    "github-token",
				Email:    "github@example.com",
				Branch:   "develop",
			},
		},
		Sync: config.SyncConfig{TimeoutMinutes: 45, MaxConcurrentJobs: 7},
		Auth: config.AuthConfig{DefaultAdminUsername: "rootadmin", DefaultAdminPassword: "rootpass"},
	}
}

// deriveKey 按 encryptSensitiveValue/MigrateEncryptionKeys 相同的方式派生 AES-256 密钥。
func deriveKey(material string) []byte {
	h := sha256.Sum256([]byte(material))
	return h[:]
}

// migrateRBAC 建 RBAC 表并初始化角色权限种子（init 系列函数的前置条件）。
func migrateRBAC(t *testing.T, db *gorm.DB) {
	t.Helper()
	require.NoError(t, db.AutoMigrate(&models.Permission{}, &models.Role{}))
	require.NoError(t, initRolesAndPermissions())
}

// getCount 统计表行数。
func getCount(t *testing.T, db *gorm.DB, model interface{}, query string, args ...interface{}) int64 {
	t.Helper()
	var n int64
	q := db.Model(model)
	if query != "" {
		q = q.Where(query, args...)
	}
	require.NoError(t, q.Count(&n).Error)
	return n
}

// getConfigValue 读取指定 key 的配置值。
func getConfigRow(t *testing.T, db *gorm.DB, key string) (value string, isEncrypted bool) {
	t.Helper()
	row := struct {
		ConfigValue string
		IsEncrypted bool
	}{}
	require.NoError(t, db.Raw("SELECT config_value, is_encrypted FROM system_configs WHERE config_key = ?", key).Scan(&row).Error)
	return row.ConfigValue, row.IsEncrypted
}
