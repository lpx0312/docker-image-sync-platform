package database

import (
	"testing"

	"docker-image-sync-platform/internal/models"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// ====================================================================
// AutoMigrate 端到端：内存 sqlite 上完整跑
// RBAC 建表 -> initRolesAndPermissions -> 旧列兼容(no-op) -> 业务表 ->
// initDefaultConfigs -> initDefaultAdmin，并验证幂等
// ====================================================================

func TestAutoMigrate_FullFlowOnSQLite(t *testing.T) {
	db := setupTestDB(t, nil)
	preCreateMySQLCompatTables(t, db)

	require.NoError(t, AutoMigrate())

	// 角色与权限落库
	assert.EqualValues(t, len(models.DefaultRoleSeeds), getCount(t, db, &models.Role{}, ""))
	assert.EqualValues(t, len(models.DefaultPermissionSeeds), getCount(t, db, &models.Permission{}, ""))

	// 权限关联数量正确
	var adminRole models.Role
	require.NoError(t, db.Preload("Permissions").Where("code = ?", models.RoleAdmin).First(&adminRole).Error)
	assert.Len(t, adminRole.Permissions, 6)

	// 默认管理员落库
	var admin models.User
	require.NoError(t, db.Where("username = ?", "admin").First(&admin).Error)
	assert.Equal(t, models.UserStatusActive, admin.Status)
	adminRoleID, err := getRoleIDByCode(models.RoleAdmin)
	require.NoError(t, err)
	assert.Equal(t, adminRoleID, admin.RoleID)

	// 默认配置落库
	assert.EqualValues(t, 18, getCount(t, db, &models.SystemConfig{}, ""), "默认配置项应为 18 条")

	// 业务表可写入且默认值生效
	rec := models.ImageSyncRecord{OriginalImage: "nginx:latest"}
	require.NoError(t, db.Create(&rec).Error)
	assert.Equal(t, models.SyncStatusPending, rec.SyncStatus, "新记录应取 sync_status 默认值")
	task := models.SyncTask{TaskID: "task-1"}
	require.NoError(t, db.Create(&task).Error)
	assert.Equal(t, models.TaskStatusPending, task.Status)

	// 幂等：重复执行不报错、不产生重复数据
	require.NoError(t, AutoMigrate())
	assert.EqualValues(t, len(models.DefaultRoleSeeds), getCount(t, db, &models.Role{}, ""))
	assert.EqualValues(t, len(models.DefaultPermissionSeeds), getCount(t, db, &models.Permission{}, ""))
	assert.EqualValues(t, 1, getCount(t, db, &models.User{}, ""))
	assert.EqualValues(t, 18, getCount(t, db, &models.SystemConfig{}, ""))
}

func TestAutoMigrate_MySQLEnumFailsOnPlainSQLite(t *testing.T) {
	// 记录当前行为：ImageSyncRecord/SyncTask 的 MySQL enum 列类型
	// 在 sqlite 上生成非法 DDL，AutoMigrate 在"数据库表迁移"步骤失败。
	// 真实 MySQL 环境不受影响；此处验证错误被正确包装上报。
	db := setupTestDB(t, nil)
	_ = db // 不预建兼容表

	err := AutoMigrate()
	require.Error(t, err)
	assert.Contains(t, err.Error(), "数据库表迁移失败")
}

func TestAutoMigrate_RBACTableMigrationFailure(t *testing.T) {
	db := setupTestDB(t, nil)
	// 关闭连接使最开始的 RBAC 建表即失败
	sqlDB, err := db.DB()
	require.NoError(t, err)
	require.NoError(t, sqlDB.Close())

	err = AutoMigrate()
	require.Error(t, err)
	assert.Contains(t, err.Error(), "RBAC 表迁移失败")
}

func TestAutoMigrate_RolePermissionInitFailure(t *testing.T) {
	db := setupTestDB(t, nil)
	preCreateMySQLCompatTables(t, db)
	require.NoError(t, db.AutoMigrate(&models.Permission{}, &models.Role{}))

	// 阻断关联表写入：initRolesAndPermissions 在 Replace 处失败
	require.NoError(t, db.Exec(`CREATE TRIGGER block_join_insert BEFORE INSERT ON role_permissions
		BEGIN SELECT RAISE(ABORT, 'insert blocked'); END`).Error)

	err := AutoMigrate()
	require.Error(t, err)
	assert.Contains(t, err.Error(), "初始化角色权限失败")
}

func TestAutoMigrate_DefaultConfigInitFailure(t *testing.T) {
	cfg := fullTestConfig() // aliyun_password 非空
	db := setupTestDB(t, cfg)
	preCreateMySQLCompatTables(t, db)

	// release 模式且无 ENCRYPTION_KEY -> 加密失败 -> initDefaultConfigs 报错
	t.Setenv("GIN_MODE", "release")
	err := AutoMigrate()
	require.Error(t, err)
	assert.Contains(t, err.Error(), "初始化默认配置失败")
}

func TestAutoMigrate_DefaultAdminInitFailure(t *testing.T) {
	db := setupTestDB(t, nil)
	preCreateMySQLCompatTables(t, db)
	require.NoError(t, AutoMigrate())

	// 软删除唯一管理员：Count=0 触发重建，username 唯一索引冲突 -> 创建失败
	require.NoError(t, db.Exec("UPDATE users SET deleted_at = CURRENT_TIMESTAMP WHERE username = 'admin'").Error)

	err := AutoMigrate()
	require.Error(t, err)
	assert.Contains(t, err.Error(), "初始化默认管理员失败")
}
