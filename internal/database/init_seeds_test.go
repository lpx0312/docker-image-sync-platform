package database

import (
	"strings"
	"testing"
	"time"

	"docker-image-sync-platform/internal/config"
	"docker-image-sync-platform/internal/models"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"golang.org/x/crypto/bcrypt"
)

// ====================================================================
// initRolesAndPermissions：角色/权限种子初始化
// ====================================================================

func TestInitRolesAndPermissions_CreatesSeeds(t *testing.T) {
	db := setupTestDB(t, nil)
	require.NoError(t, db.AutoMigrate(&models.Permission{}, &models.Role{}))

	require.NoError(t, initRolesAndPermissions())

	assert.EqualValues(t, len(models.DefaultPermissionSeeds), getCount(t, db, &models.Permission{}, ""), "权限应全部创建")
	assert.EqualValues(t, len(models.DefaultRoleSeeds), getCount(t, db, &models.Role{}, ""), "角色应全部创建")

	// 校验每个角色的权限关联数量
	want := map[string]int{
		models.RoleAdmin:    6,
		models.RoleOperator: 4,
		models.RoleUser:     1,
	}
	for code, n := range want {
		var role models.Role
		require.NoError(t, db.Preload("Permissions").Where("code = ?", code).First(&role).Error)
		assert.Len(t, role.Permissions, n, "角色 %s 的权限数不符", code)
		assert.True(t, role.IsSystem, "种子角色应标记为系统角色")
	}
}

func TestInitRolesAndPermissions_IdempotentAndRepairs(t *testing.T) {
	db := setupTestDB(t, nil)
	require.NoError(t, db.AutoMigrate(&models.Permission{}, &models.Role{}))
	require.NoError(t, initRolesAndPermissions())

	// 人为破坏：改权限名/角色名、删除一条角色-权限关联
	require.NoError(t, db.Model(&models.Permission{}).Where("code = ?", models.PermSync).
		Update("name", "被篡改的名字").Error)
	require.NoError(t, db.Model(&models.Role{}).Where("code = ?", models.RoleUser).
		Update("name", "被篡改的角色").Error)
	adminRoleID, err := getRoleIDByCode(models.RoleAdmin)
	require.NoError(t, err)
	require.NoError(t, db.Exec("DELETE FROM role_permissions WHERE role_id = ?", adminRoleID).Error)

	// 重复执行：走 Updates 分支修复元数据，Replace 恢复关联
	require.NoError(t, initRolesAndPermissions())

	var perm models.Permission
	require.NoError(t, db.Where("code = ?", models.PermSync).First(&perm).Error)
	for _, seed := range models.DefaultPermissionSeeds {
		if seed.Code == models.PermSync {
			assert.Equal(t, seed.Name, perm.Name, "权限名称应被种子值修复")
		}
	}
	var role models.Role
	require.NoError(t, db.Preload("Permissions").Where("code = ?", models.RoleAdmin).First(&role).Error)
	assert.Len(t, role.Permissions, 6, "被删除的关联应被 Replace 恢复")
	assert.EqualValues(t, len(models.DefaultPermissionSeeds), getCount(t, db, &models.Permission{}, ""), "重复执行不应产生重复权限")
	assert.EqualValues(t, len(models.DefaultRoleSeeds), getCount(t, db, &models.Role{}, ""), "重复执行不应产生重复角色")
}

func TestInitRolesAndPermissions_MissingTables(t *testing.T) {
	db := setupTestDB(t, nil) // 未做任何迁移
	assert.False(t, db.Migrator().HasTable(&models.Permission{}), "前置：permissions 表不存在")

	err := initRolesAndPermissions()
	require.Error(t, err)
	assert.Contains(t, err.Error(), "创建权限", "permissions 表缺失时应报创建权限失败")
}

func TestInitRolesAndPermissions_MissingRolesTable(t *testing.T) {
	db := setupTestDB(t, nil)
	require.NoError(t, db.AutoMigrate(&models.Permission{})) // 只建权限表

	err := initRolesAndPermissions()
	require.Error(t, err)
	assert.Contains(t, err.Error(), "创建角色", "roles 表缺失时应报创建角色失败")
}

func TestInitRolesAndPermissions_AssociationReplaceFailure(t *testing.T) {
	db := setupTestDB(t, nil)
	require.NoError(t, db.AutoMigrate(&models.Permission{}, &models.Role{}))

	// 用 sqlite 触发器阻止关联表写入，覆盖 Replace 失败分支
	// （无法用"删掉关联表"构造：AutoMigrate 会自动补建）
	require.NoError(t, db.Exec(`CREATE TRIGGER block_join_insert BEFORE INSERT ON role_permissions
		BEGIN SELECT RAISE(ABORT, 'insert blocked'); END`).Error)

	err := initRolesAndPermissions()
	require.Error(t, err)
	assert.Contains(t, err.Error(), "同步角色", "关联写入被阻止时应报同步角色权限失败")
}

func TestGetRoleIDByCode(t *testing.T) {
	db := setupTestDB(t, nil)
	require.NoError(t, db.AutoMigrate(&models.Role{}))
	created := models.Role{Code: models.RoleAdmin, Name: "管理员"}
	require.NoError(t, db.Create(&created).Error)

	id, err := getRoleIDByCode(models.RoleAdmin)
	require.NoError(t, err)
	assert.Equal(t, created.ID, id)

	_, err = getRoleIDByCode("nonexistent")
	require.Error(t, err)
	assert.Contains(t, err.Error(), "不存在")
}

// ====================================================================
// initDefaultConfigs：默认配置初始化
// ====================================================================

func TestInitDefaultConfigs_DefaultValuesOnEmptyConfig(t *testing.T) {
	db := setupTestDB(t, emptyConfig())
	require.NoError(t, db.AutoMigrate(&models.SystemConfig{}))

	require.NoError(t, initDefaultConfigs())

	// 空配置时全部默认值兜底
	want := map[string]string{
		"aliyun_namespace":    "your-namespace",
		"aliyun_registry":     "registry.cn-hangzhou.aliyuncs.com",
		"git_repository_type": "gitee",
		"gitee_branch":        "main",
		"github_branch":       "main",
		"sync_timeout_minutes": "30",
		"max_concurrent_syncs": "3",
	}
	for key, val := range want {
		got, _ := getConfigRow(t, db, key)
		assert.Equal(t, val, got, "配置 %s 未使用默认值兜底", key)
	}

	// 敏感字段值为空时存空串，不加密
	val, isEnc := getConfigRow(t, db, "aliyun_password")
	assert.Empty(t, val)
	assert.True(t, isEnc, "aliyun_password 应保留 IsEncrypted 标记")

	// 非敏感字段 IsEncrypted=false
	_, isEnc = getConfigRow(t, db, "aliyun_namespace")
	assert.False(t, isEnc)
}

func TestInitDefaultConfigs_ValuesFromConfigAndEncrypted(t *testing.T) {
	cfg := fullTestConfig()
	db := setupTestDB(t, cfg)
	t.Setenv("ENCRYPTION_KEY", "cfg-test-key")
	require.NoError(t, db.AutoMigrate(&models.SystemConfig{}))

	require.NoError(t, initDefaultConfigs())

	// 明文字段按配置值落库
	val, _ := getConfigRow(t, db, "aliyun_namespace")
	assert.Equal(t, "myns", val)
	val, _ = getConfigRow(t, db, "sync_timeout_minutes")
	assert.Equal(t, "45", val)
	val, _ = getConfigRow(t, db, "max_concurrent_syncs")
	assert.Equal(t, "7", val)

	// 敏感字段加密落库且可解回
	for key, want := range map[string]string{
		"aliyun_password": "ali-pass",
		"gitee_password":  "gitee-pass",
		"gitee_token":     "gitee-token",
		"github_token":    "github-token",
	} {
		val, isEnc := getConfigRow(t, db, key)
		require.True(t, isEnc, "%s 应标记加密", key)
		require.True(t, strings.HasPrefix(val, "ENC:"), "%s 应以 ENC: 前缀落库", key)
		plaintext, err := decryptAESGCM(val, deriveKey("cfg-test-key"))
		require.NoError(t, err)
		assert.Equal(t, want, plaintext)
	}
}

func TestInitDefaultConfigs_ExistingNotOverwritten(t *testing.T) {
	db := setupTestDB(t, emptyConfig())
	require.NoError(t, db.AutoMigrate(&models.SystemConfig{}))

	// 预置用户自定义值
	require.NoError(t, db.Create(&models.SystemConfig{
		ConfigKey: "aliyun_namespace", ConfigValue: "custom-ns", Description: "用户自定义",
	}).Error)

	require.NoError(t, initDefaultConfigs())
	val, _ := getConfigRow(t, db, "aliyun_namespace")
	assert.Equal(t, "custom-ns", val, "已存在的配置不应被覆盖")

	// 幂等：重复执行所有既有值不变
	before := map[string]string{}
	var rows []models.SystemConfig
	require.NoError(t, db.Find(&rows).Error)
	for _, r := range rows {
		before[r.ConfigKey] = r.ConfigValue
	}
	require.NoError(t, initDefaultConfigs())
	require.NoError(t, db.Find(&rows).Error)
	assert.Len(t, rows, len(before), "重复执行不应新增或删除配置")
	for _, r := range rows {
		assert.Equal(t, before[r.ConfigKey], r.ConfigValue, "配置 %s 的值发生变化", r.ConfigKey)
	}
}

func TestInitDefaultConfigs_EncryptionFailure(t *testing.T) {
	cfg := fullTestConfig()
	db := setupTestDB(t, cfg)
	require.NoError(t, db.AutoMigrate(&models.SystemConfig{}))

	// release 模式且无 ENCRYPTION_KEY：加密必失败 -> 整个初始化报错
	t.Setenv("GIN_MODE", "release")
	err := initDefaultConfigs()
	require.Error(t, err)
	assert.Contains(t, err.Error(), "加密配置 aliyun_password 失败")
}

func TestInitDefaultConfigs_CreateFailure(t *testing.T) {
	db := setupTestDB(t, emptyConfig())
	require.NoError(t, db.AutoMigrate(&models.SystemConfig{}))

	// 软删除占用 config_key 唯一索引：First 查不到（软删除过滤），
	// Create 却撞唯一约束 -> 覆盖创建失败分支
	sc := models.SystemConfig{ConfigKey: "aliyun_namespace", ConfigValue: "old"}
	require.NoError(t, db.Create(&sc).Error)
	require.NoError(t, db.Model(&sc).Update("deleted_at", time.Now()).Error)

	err := initDefaultConfigs()
	require.Error(t, err)
	assert.Contains(t, err.Error(), "创建默认配置 aliyun_namespace 失败")
}

func TestInitDefaultConfigs_SilentlySkipsWhenTableMissing(t *testing.T) {
	// 当前行为：system_configs 表不存在时 First 返回非 ErrRecordNotFound 错误，
	// 代码只认 ErrRecordNotFound -> 静默跳过、返回 nil（疑似缺陷，按现状断言）
	db := setupTestDB(t, emptyConfig())
	require.NoError(t, initDefaultConfigs())
	assert.False(t, db.Migrator().HasTable(&models.SystemConfig{}), "前置：表确实不存在")
}

// ====================================================================
// initDefaultAdmin：默认管理员初始化
// ====================================================================

func TestInitDefaultAdmin_DefaultCredentials(t *testing.T) {
	db := setupTestDB(t, emptyConfig()) // Auth 零值 -> 默认 admin/admin123
	require.NoError(t, db.AutoMigrate(&models.User{}))
	migrateRBAC(t, db)

	require.NoError(t, initDefaultAdmin())

	var admin models.User
	require.NoError(t, db.Where("username = ?", "admin").First(&admin).Error)
	assert.Equal(t, models.UserStatusActive, admin.Status)
	assert.Equal(t, "admin@example.com", admin.Email)

	// 密码哈希可校验
	require.NoError(t, bcrypt.CompareHashAndPassword([]byte(admin.PasswordHash), []byte("admin123")))

	// 角色为 admin
	roleID, err := getRoleIDByCode(models.RoleAdmin)
	require.NoError(t, err)
	assert.Equal(t, roleID, admin.RoleID)
}

func TestInitDefaultAdmin_CustomCredentials(t *testing.T) {
	cfg := configWithAdmin("rootadmin", "rootpass")
	db := setupTestDB(t, cfg)
	require.NoError(t, db.AutoMigrate(&models.User{}))
	migrateRBAC(t, db)

	require.NoError(t, initDefaultAdmin())

	var admin models.User
	require.NoError(t, db.Where("username = ?", "rootadmin").First(&admin).Error)
	require.NoError(t, bcrypt.CompareHashAndPassword([]byte(admin.PasswordHash), []byte("rootpass")))
	assert.EqualValues(t, 1, getCount(t, db, &models.User{}, ""), "只应创建一个管理员")
}

func TestInitDefaultAdmin_SkipsWhenUserExists(t *testing.T) {
	db := setupTestDB(t, emptyConfig())
	require.NoError(t, db.AutoMigrate(&models.User{}))
	migrateRBAC(t, db)

	roleID, err := getRoleIDByCode(models.RoleUser)
	require.NoError(t, err)
	require.NoError(t, db.Create(&models.User{
		Username: "someone", PasswordHash: "x", RoleID: roleID, Status: models.UserStatusActive,
	}).Error)

	require.NoError(t, initDefaultAdmin())
	assert.EqualValues(t, 1, getCount(t, db, &models.User{}, ""), "已有用户时不应再创建默认管理员")
	var cnt int64
	db.Model(&models.User{}).Where("username = ?", "admin").Count(&cnt)
	assert.Zero(t, cnt)
}

func TestInitDefaultAdmin_MissingAdminRole(t *testing.T) {
	db := setupTestDB(t, emptyConfig())
	require.NoError(t, db.AutoMigrate(&models.User{}))
	// 不初始化角色 -> admin 角色不存在

	err := initDefaultAdmin()
	require.Error(t, err)
	assert.Contains(t, err.Error(), "角色 admin 不存在")
}

func TestInitDefaultAdmin_PasswordTooLong(t *testing.T) {
	// bcrypt 对超过 72 字节的密码报错 -> 覆盖加密密码失败分支
	longPwd := strings.Repeat("p", 100)
	cfg := configWithAdmin("admin", longPwd)
	db := setupTestDB(t, cfg)
	require.NoError(t, db.AutoMigrate(&models.User{}))
	migrateRBAC(t, db)

	err := initDefaultAdmin()
	require.Error(t, err)
	assert.Contains(t, err.Error(), "加密默认管理员密码失败")
}

func TestInitDefaultAdmin_DuplicateUsername(t *testing.T) {
	db := setupTestDB(t, emptyConfig())
	require.NoError(t, db.AutoMigrate(&models.User{}))
	migrateRBAC(t, db)

	// 软删除 admin：Count 为 0 走创建流程，但 username 唯一索引仍占用 -> Create 失败
	first := models.User{Username: "admin", PasswordHash: "x", RoleID: 1}
	require.NoError(t, db.Create(&first).Error)
	require.NoError(t, db.Model(&first).Update("deleted_at", time.Now()).Error)
	assert.EqualValues(t, 0, getCount(t, db, &models.User{}, ""), "前置：软删除后 Count 应为 0")

	err := initDefaultAdmin()
	require.Error(t, err)
	assert.Contains(t, err.Error(), "创建默认管理员失败")
}

// ====================================================================
// 旧 role 字段迁移辅助函数：sqlite 下走 information_schema 不可用分支
// ====================================================================

func TestLegacyUserRoleColumnHelpers_OnSQLite(t *testing.T) {
	db := setupTestDB(t, nil)
	require.NoError(t, db.AutoMigrate(&models.User{}, &models.Role{}))

	// sqlite 没有 information_schema：查询失败 -> count 保持 0 -> 全部走"不存在"分支
	assert.False(t, hasUsersTable(), "sqlite 下 information_schema 查询失败，应视为表不存在")
	assert.False(t, hasUsersRoleColumn(), "sqlite 下应视为 role 列不存在")
	require.NoError(t, ensureUserRoleIDColumn(), "role_id 列与 users 表均判定不存在时应直接返回 nil")
	require.NoError(t, migrateUserRoleIDs(), "users 表判定不存在时应直接返回 nil")
	require.NoError(t, dropLegacyUserRoleColumn(), "role 列判定不存在时应直接返回 nil")

	// 确认这些 no-op 没有误建/误删列
	assert.True(t, db.Migrator().HasColumn(&models.User{}, "role_id"))
}

// ====================================================================
// 测试配置构造辅助
// ====================================================================

// emptyConfig 返回全零配置（所有默认值兜底分支的前提）。
func emptyConfig() *config.Config { return &config.Config{} }

// configWithAdmin 返回指定管理员账号密码的配置。
func configWithAdmin(username, password string) *config.Config {
	return &config.Config{
		Auth: config.AuthConfig{DefaultAdminUsername: username, DefaultAdminPassword: password},
	}
}
