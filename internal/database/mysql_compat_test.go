package database

// mysql_compat_test.go 用 modernc(go-sqlite) 的自定义函数 + ATTACH 模拟 MySQL 的
// information_schema / DATABASE()，使 ensureUserRoleIDColumn / migrateUserRoleIDs /
// dropLegacyUserRoleColumn 这些 MySQL 专有辅助函数在 sqlite 上也能覆盖主分支。
//
// 原理：
//  1. RegisterScalarFunction("DATABASE") 让 Raw SQL 里的 DATABASE() 可求值；
//  2. ATTACH ':memory:' AS information_schema 提供同名 schema；
//  3. 在其中建 tables/COLUMNS 两张表并注入元数据行，精确控制各分支。

import (
	"database/sql/driver"
	"testing"

	"docker-image-sync-platform/internal/config"
	"docker-image-sync-platform/internal/models"

	gosqlite "github.com/glebarez/go-sqlite"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"
)

const mysqlCompatSchema = "docker_sync_test"

// registerMySQLCompat 注册 DATABASE() 函数（进程内只执行一次）。
var registerMySQLCompat = func() (ok bool) {
	ok = true
	gosqlite.MustRegisterScalarFunction("DATABASE", 0, func(ctx *gosqlite.FunctionContext, args []driver.Value) (driver.Value, error) {
		return mysqlCompatSchema, nil
	})
	return
}()

// attachInformationSchema 在测试库连接上挂出 information_schema 并注入元数据。
// tables/COLUMNS 的行由调用方通过 addISRow 增补。
func attachInformationSchema(t *testing.T, db *gorm.DB) {
	t.Helper()
	require.True(t, registerMySQLCompat)
	require.NoError(t, db.Exec(`ATTACH ':memory:' AS "information_schema"`).Error)
	require.NoError(t, db.Exec(`CREATE TABLE information_schema.tables (
		TABLE_SCHEMA text, TABLE_NAME text, table_type text)`).Error)
	require.NoError(t, db.Exec(`CREATE TABLE information_schema."COLUMNS" (
		TABLE_SCHEMA text, TABLE_NAME text, COLUMN_NAME text)`).Error)
}

// addISTableRow 声明一张表存在（table_type 默认 BASE TABLE）。
func addISTableRow(t *testing.T, db *gorm.DB, table string, tableType string) {
	t.Helper()
	if tableType == "" {
		tableType = "BASE TABLE"
	}
	require.NoError(t, db.Exec(`INSERT INTO information_schema.tables VALUES (?, ?, ?)`,
		mysqlCompatSchema, table, tableType).Error)
}

// addISColumnRow 声明某表的某列存在。
func addISColumnRow(t *testing.T, db *gorm.DB, table, column string) {
	t.Helper()
	require.NoError(t, db.Exec(`INSERT INTO information_schema."COLUMNS" VALUES (?, ?, ?)`,
		mysqlCompatSchema, table, column).Error)
}

func TestEnsureUserRoleIDColumn_ColumnExists(t *testing.T) {
	db := setupTestDB(t, nil)
	require.NoError(t, db.AutoMigrate(&models.User{}))
	attachInformationSchema(t, db)
	addISColumnRow(t, db, "users", "role_id")

	require.NoError(t, ensureUserRoleIDColumn(), "role_id 已存在应直接返回 nil")
	assert.True(t, db.Migrator().HasColumn(&models.User{}, "role_id"))
}

func TestEnsureUserRoleIDColumn_NoUsersTable(t *testing.T) {
	db := setupTestDB(t, nil)
	require.NoError(t, db.AutoMigrate(&models.User{}))
	attachInformationSchema(t, db)
	// 不注入 users 表行 → 视为表不存在 → 直接 nil
	require.NoError(t, ensureUserRoleIDColumn())
}

func TestEnsureUserRoleIDColumn_AddsColumn(t *testing.T) {
	db := setupTestDB(t, nil)
	// 建一张不含 role_id 的旧版 users 表
	require.NoError(t, db.Exec(`CREATE TABLE users (id integer PRIMARY KEY, username text, role text)`).Error)
	attachInformationSchema(t, db)
	addISTableRow(t, db, "users", "")
	addISColumnRow(t, db, "users", "role") // 旧 role 列存在，role_id 不存在

	require.NoError(t, ensureUserRoleIDColumn())
	assert.True(t, db.Migrator().HasColumn(&models.User{}, "role_id"), "应补建 role_id 列")
}

func TestEnsureUserRoleIDColumn_AlterFails(t *testing.T) {
	db := setupTestDB(t, nil)
	// users 建成视图：information_schema 声明它存在，但 ALTER 视图必失败
	require.NoError(t, db.Exec(`CREATE VIEW users AS SELECT 1 AS id`).Error)
	attachInformationSchema(t, db)
	addISTableRow(t, db, "users", "")

	err := ensureUserRoleIDColumn()
	require.Error(t, err)
	assert.Contains(t, err.Error(), "添加 users.role_id 列失败")
}

func TestMigrateUserRoleIDs_NoUsersTable(t *testing.T) {
	db := setupTestDB(t, nil)
	require.NoError(t, db.AutoMigrate(&models.User{}))
	attachInformationSchema(t, db)
	// users 表不存在 → 直接 nil
	require.NoError(t, migrateUserRoleIDs())
}

func TestMigrateUserRoleIDs_NoRoleColumn_FillsDefault(t *testing.T) {
	db := setupTestDB(t, nil)
	require.NoError(t, db.AutoMigrate(&models.Role{}, &models.User{}))
	attachInformationSchema(t, db)
	addISTableRow(t, db, "users", "")

	role := models.Role{Code: models.RoleUser, Name: "普通用户"}
	require.NoError(t, db.Create(&role).Error)
	// role_id 为 0 的用户应被填默认角色
	u := models.User{Username: "u1", PasswordHash: "x", RoleID: 0}
	require.NoError(t, db.Create(&u).Error)

	require.NoError(t, migrateUserRoleIDs())

	var got models.User
	require.NoError(t, db.Where("username = ?", "u1").First(&got).Error)
	assert.Equal(t, role.ID, got.RoleID, "role_id=0 的用户应被填为默认角色")
}

func TestMigrateUserRoleIDs_WithLegacyRoleColumn_JoinFailsOnSQLite(t *testing.T) {
	db := setupTestDB(t, nil)
	require.NoError(t, db.AutoMigrate(&models.Role{}, &models.User{}))
	attachInformationSchema(t, db)
	addISTableRow(t, db, "users", "")
	addISColumnRow(t, db, "users", "role") // 旧 role 列存在 → 走 INNER JOIN 迁移

	// sqlite 不支持 UPDATE ... INNER JOIN ... 语法 → 走"从旧 role 字段迁移失败"错误分支
	err := migrateUserRoleIDs()
	require.Error(t, err)
	assert.Contains(t, err.Error(), "从旧 role 字段迁移失败")
}

func TestDropLegacyUserRoleColumn_NoColumn(t *testing.T) {
	db := setupTestDB(t, nil)
	require.NoError(t, db.AutoMigrate(&models.User{}))
	attachInformationSchema(t, db)
	// 不注入 role 列 → 视为不存在 → nil
	require.NoError(t, dropLegacyUserRoleColumn())
}

func TestDropLegacyUserRoleColumn_Drops(t *testing.T) {
	db := setupTestDB(t, nil)
	require.NoError(t, db.Exec(`CREATE TABLE users (id integer PRIMARY KEY, role_id integer, role text)`).Error)
	attachInformationSchema(t, db)
	addISColumnRow(t, db, "users", "role")

	require.NoError(t, dropLegacyUserRoleColumn())
	assert.False(t, db.Migrator().HasColumn(&models.User{}, "Role"), "旧 role 列应被删除")
}

func TestDropLegacyUserRoleColumn_Fails(t *testing.T) {
	db := setupTestDB(t, nil)
	// 声明有 role 列但表实际不存在 → DROP 报错
	attachInformationSchema(t, db)
	addISColumnRow(t, db, "users", "role")

	err := dropLegacyUserRoleColumn()
	require.Error(t, err)
	assert.Contains(t, err.Error(), "删除 users.role 列失败")
}

// 保底：GetDSN 输出格式（数据库包对 config 的唯一消费点）
func TestGetDSNFormat(t *testing.T) {
	c := config.DatabaseConfig{
		Host: "h", Port: 3306, Username: "u", Password: "p",
		Database: "d", Charset: "utf8mb4", ParseTime: true, Loc: "Local",
	}
	assert.Equal(t, "u:p@tcp(h:3306)/d?charset=utf8mb4&parseTime=true&loc=Local", c.GetDSN())
}

// ====================================================================
// AutoMigrate 的 MySQL 专有子步骤错误分支（依赖 information_schema 模拟）
// ====================================================================

func TestAutoMigrate_EnsureUserRoleIDColumnFails(t *testing.T) {
	db := setupTestDB(t, emptyConfig())
	require.NoError(t, db.AutoMigrate(&models.Permission{}, &models.Role{}))
	// users 建成视图 + IS 声明存在：ensureUserRoleIDColumn 的 ALTER 失败
	require.NoError(t, db.Exec(`CREATE VIEW users AS SELECT 1 AS id`).Error)
	attachInformationSchema(t, db)
	addISTableRow(t, db, "users", "")

	err := AutoMigrate()
	require.Error(t, err)
	assert.Contains(t, err.Error(), "准备用户角色字段失败")
}

func TestAutoMigrate_MigrateUserRoleIDsFails(t *testing.T) {
	db := setupTestDB(t, emptyConfig())
	require.NoError(t, db.AutoMigrate(&models.Permission{}, &models.Role{}))
	// 手工建旧版 users 表：没有 role_id（让 ensure 的 ALTER 成功）但有 role 列
	require.NoError(t, db.Exec(`CREATE TABLE users (id integer PRIMARY KEY, username text, role text)`).Error)
	attachInformationSchema(t, db)
	addISTableRow(t, db, "users", "")
	addISColumnRow(t, db, "users", "role") // 触发 UPDATE INNER JOIN → sqlite 语法错误

	err := AutoMigrate()
	require.Error(t, err)
	assert.Contains(t, err.Error(), "迁移用户角色数据失败")
}

func TestAutoMigrate_DropLegacyColumnFails(t *testing.T) {
	db := setupTestDB(t, emptyConfig())
	require.NoError(t, db.AutoMigrate(&models.Permission{}, &models.Role{}))
	require.NoError(t, db.AutoMigrate(&models.User{}))
	// 声明存在旧 role 列，但实际 users 表没有该列 → DROP 失败
	attachInformationSchema(t, db)
	addISColumnRow(t, db, "users", "role")

	err := AutoMigrate()
	require.Error(t, err)
	assert.Contains(t, err.Error(), "清理旧用户角色字段失败")
}
