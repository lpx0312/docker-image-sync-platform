package database

import (
	"testing"

	"docker-image-sync-platform/internal/config"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// ====================================================================
// InitDatabase / GetDB / CloseDatabase
// 连接生命周期测试。InitDatabase 的成功路径需要真实 MySQL，无法在
// 单测环境覆盖；这里只验证失败路径与关闭行为。
// ====================================================================

func TestGetDB_ReturnsGlobal(t *testing.T) {
	db := setupTestDB(t, nil)
	assert.Same(t, db, GetDB(), "GetDB 应返回全局 DB 实例")
}

func TestInitDatabase_BadDSN(t *testing.T) {
	oldDB, oldCfg := DB, config.AppConfig
	t.Cleanup(func() { DB, config.AppConfig = oldDB, oldCfg })

	// 指向本机必然拒绝连接的端口：gorm.Open 内部会自动 Ping，立即失败
	config.AppConfig = &config.Config{}
	config.AppConfig.Database = config.DatabaseConfig{
		Host:      "127.0.0.1",
		Port:      1, // 保留端口，无监听
		Username:  "u",
		Password:  "p",
		Database:  "d",
		Charset:   "utf8mb4",
		ParseTime: true,
		Loc:       "Local",
	}

	err := InitDatabase()
	require.Error(t, err)
	assert.Contains(t, err.Error(), "连接数据库失败")
	// 当前行为：失败时全局 DB 仍会被赋成半初始化实例（见汇报），仅验证非 nil 假设之外的语义不受影响
}

func TestInitDatabase_InvalidDSNFormat(t *testing.T) {
	oldDB, oldCfg := DB, config.AppConfig
	t.Cleanup(func() { DB, config.AppConfig = oldDB, oldCfg })

	// DSN 里 host 含 ')'：tcp(...) 段提前截断，mysql 驱动解析 DSN 直接失败
	config.AppConfig = &config.Config{}
	config.AppConfig.Database = config.DatabaseConfig{
		Host:      "bad)host",
		Port:      3306,
		Username:  "u",
		Password:  "p",
		Database:  "d",
		Charset:   "utf8mb4",
		ParseTime: true,
		Loc:       "Local",
	}
	dsn := config.AppConfig.Database.GetDSN()
	require.Contains(t, dsn, "bad)host", "前置校验：DSN 应含构造的非法 host")

	err := InitDatabase()
	require.Error(t, err)
	assert.Contains(t, err.Error(), "连接数据库失败")
}

func TestCloseDatabase_NilDB(t *testing.T) {
	oldDB := DB
	t.Cleanup(func() { DB = oldDB })
	DB = nil

	require.NoError(t, CloseDatabase(), "DB 为 nil 时应静默返回 nil")
}

func TestCloseDatabase_NormalAndTwice(t *testing.T) {
	oldDB := DB
	t.Cleanup(func() { DB = oldDB })
	DB = openSQLiteDB(t)

	// 第一次关闭：正常成功
	require.NoError(t, CloseDatabase())

	// 第二次关闭：database/sql 的 Close 幂等，当前实现返回 nil（不报错）
	require.NoError(t, CloseDatabase(), "二次关闭按当前行为应返回 nil（sql.DB.Close 幂等）")
}
