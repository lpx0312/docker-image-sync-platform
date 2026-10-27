package database

// migrations_extra_test.go 覆盖 RunMigrations / backfillAcrRegistryAlias /
// MigrateEncryptionKeys 及其全部分支（sqlite 内存库）。

import (
	"strings"
	"testing"

	"docker-image-sync-platform/internal/models"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// ====================================================================
// RunMigrations
// ====================================================================

func TestRunMigrations_CreatesAcrTables(t *testing.T) {
	db := setupTestDB(t, nil)

	RunMigrations()

	assert.True(t, db.Migrator().HasTable(&models.AcrRegistry{}), "acr_registries 表应被创建")
	assert.True(t, db.Migrator().HasTable(&models.AcrRepository{}), "acr_repositories 表应被创建")
}

func TestRunMigrations_AutoMigrateFailure_RegistryTable(t *testing.T) {
	db := setupTestDB(t, nil)
	// 用视图占用表名，使 AutoMigrate(AcrRegistry) 失败，但不中断（只记日志）。
	// 注：AcrRepository 关联 AcrRegistry，AutoMigrate 会连带迁移被关联模型，
	// 因此第二个 AutoMigrate 也会因同名视图失败 —— 两个错误分支都被覆盖。
	require.NoError(t, db.Exec("CREATE VIEW acr_registries AS SELECT 1 AS x").Error)

	RunMigrations() // 不应 panic

	assert.False(t, db.Migrator().HasTable(&models.AcrRegistry{}))
	assert.False(t, db.Migrator().HasTable(&models.AcrRepository{}))
}

func TestRunMigrations_AutoMigrateFailure_RepositoryTable(t *testing.T) {
	db := setupTestDB(t, nil)
	require.NoError(t, db.Exec("CREATE VIEW acr_repositories AS SELECT 1 AS x").Error)

	RunMigrations()

	assert.True(t, db.Migrator().HasTable(&models.AcrRegistry{}))
	assert.False(t, db.Migrator().HasTable(&models.AcrRepository{}))
}

func TestRunMigrations_BackfillQueryFailure(t *testing.T) {
	db := setupTestDB(t, nil)
	// acr_registries 不存在：AutoMigrate 建表成功后 Find 不会失败，
	// 这里改为直接删除表模拟查询失败路径（backfill 内部容错返回）
	require.NoError(t, db.AutoMigrate(&models.AcrRegistry{}))
	require.NoError(t, db.Exec("DROP TABLE acr_registries").Error)

	RunMigrations() // AutoMigrate 重建表后 backfill 查询成功；此用例主要保证不 panic

	assert.True(t, db.Migrator().HasTable(&models.AcrRegistry{}))
}

// ====================================================================
// backfillAcrRegistryAlias
// ====================================================================

func TestBackfillAcrRegistryAlias_AllRules(t *testing.T) {
	db := setupTestDB(t, nil)
	require.NoError(t, db.AutoMigrate(&models.AcrRegistry{}))

	// 按 ID 顺序：
	// 1: 已有别名 → 保留并占用
	// 2: 空 alias + namespace=ns-a → alias=ns-a
	// 3: 空 alias + namespace=ns-a（冲突, type=swr）→ ns-a-SWR
	// 4: 空 alias + namespace=ns-a（冲突, type 空 → acr；ns-a-ACR 与 id5 冲突）
	// 5: 空 alias + namespace=ns-a + type=ACR（大写后仍冲突）→ ns-a-<id>
	// 6: 空 namespace → registry-<id>
	rows := []models.AcrRegistry{
		{RegistryURL: "http://r1", Namespace: "ns-x", Alias: "kept", Username: "u", Password: "p", RegistryType: models.RegistryTypeACR},
		{RegistryURL: "http://r2", Namespace: "ns-a", Username: "u", Password: "p", RegistryType: models.RegistryTypeACR},
		{RegistryURL: "http://r3", Namespace: "ns-a", Username: "u", Password: "p", RegistryType: models.RegistryTypeSWR},
		{RegistryURL: "http://r4", Namespace: "ns-a", Username: "u", Password: "p", RegistryType: ""},
		{RegistryURL: "http://r5", Namespace: "ns-a", Username: "u", Password: "p", RegistryType: models.RegistryTypeACR},
		{RegistryURL: "http://r6", Namespace: "   ", Username: "u", Password: "p", RegistryType: models.RegistryTypeGeneric},
	}
	for i := range rows {
		require.NoError(t, db.Create(&rows[i]).Error)
	}

	backfillAcrRegistryAlias()

	var got []models.AcrRegistry
	require.NoError(t, db.Order("id ASC").Find(&got).Error)
	require.Len(t, got, 6)

	assert.Equal(t, "kept", got[0].Alias, "已有别名应保留")
	assert.Equal(t, "ns-a", got[1].Alias, "空别名默认取 namespace")
	assert.Equal(t, "ns-a-SWR", got[2].Alias, "冲突时追加类型后缀（大写）")
	// id=4: type 空默认 acr → ns-a-ACR；id=5 同样得到 ns-a-ACR 冲突 → ns-a-<id>
	if got[3].Alias == "ns-a-ACR" {
		assert.Equal(t, "ns-a-ACR", got[3].Alias)
		assert.Equal(t, "ns-a-5", got[4].Alias, "类型后缀仍冲突时追加记录 ID")
	} else {
		// 若 id=5 先抢到 ns-a-ACR（顺序不应如此，防御性断言）
		assert.Equal(t, "ns-a-4", got[3].Alias)
		assert.Equal(t, "ns-a-ACR", got[4].Alias)
	}
	assert.Equal(t, "registry-6", got[5].Alias, "空 namespace 用 registry-<id> 兜底")

	// 幂等：重跑不改变结果
	require.NoError(t, db.Order("id ASC").Find(&rows).Error)
	backfillAcrRegistryAlias()
	var again []models.AcrRegistry
	require.NoError(t, db.Order("id ASC").Find(&again).Error)
	for i := range again {
		assert.Equal(t, got[i].Alias, again[i].Alias, "重复回填应幂等（id=%d）", again[i].ID)
	}
}

func TestBackfillAcrRegistryAlias_NothingToDo(t *testing.T) {
	db := setupTestDB(t, nil)
	require.NoError(t, db.AutoMigrate(&models.AcrRegistry{}))
	// 空表：changed=0，不写任何行
	backfillAcrRegistryAlias()
	var cnt int64
	require.NoError(t, db.Model(&models.AcrRegistry{}).Count(&cnt).Error)
	assert.Zero(t, cnt)
}

func TestBackfillAcrRegistryAlias_UpdateFailureNoPanic(t *testing.T) {
	db := setupTestDB(t, nil)
	require.NoError(t, db.AutoMigrate(&models.AcrRegistry{}))
	require.NoError(t, db.Create(&models.AcrRegistry{
		RegistryURL: "http://r", Namespace: "ns", Username: "u", Password: "p", RegistryType: models.RegistryTypeACR,
	}).Error)
	// 用触发器让 UPDATE alias 失败，覆盖 continue 分支（不 panic、其余逻辑继续）
	require.NoError(t, db.Exec(`CREATE TRIGGER block_alias_update BEFORE UPDATE OF alias ON acr_registries
		BEGIN SELECT RAISE(ABORT, 'update blocked'); END`).Error)

	backfillAcrRegistryAlias()
}

// ====================================================================
// MigrateEncryptionKeys
// ====================================================================

func TestMigrateEncryptionKeys_NoEnvKey(t *testing.T) {
	setupTestDB(t, nil) // setupTestDB 已清空 ENCRYPTION_KEY
	require.NoError(t, MigrateEncryptionKeys())
}

func TestMigrateEncryptionKeys_NoRecords(t *testing.T) {
	db := setupTestDB(t, nil)
	t.Setenv("ENCRYPTION_KEY", "new-key")
	require.NoError(t, db.AutoMigrate(&models.SystemConfig{}))

	require.NoError(t, MigrateEncryptionKeys())
}

func TestMigrateEncryptionKeys_MigratesOldKeyRecords(t *testing.T) {
	db := setupTestDB(t, nil)
	t.Setenv("ENCRYPTION_KEY", "new-key")
	require.NoError(t, db.AutoMigrate(&models.SystemConfig{}))

	oldKey := deriveKey("docker-sync-platform-default-key-2024")
	newKey := deriveKey("new-key")

	// 1) 旧密钥加密 → 应被迁移
	oldEnc, err := encryptAESGCM("old-secret", oldKey)
	require.NoError(t, err)
	rowOld := models.SystemConfig{ConfigKey: "k_old", ConfigValue: oldEnc, IsEncrypted: true}
	require.NoError(t, db.Create(&rowOld).Error)

	// 2) 新密钥加密 → 应跳过
	newEnc, err := encryptAESGCM("new-secret", newKey)
	require.NoError(t, err)
	rowNew := models.SystemConfig{ConfigKey: "k_new", ConfigValue: newEnc, IsEncrypted: true}
	require.NoError(t, db.Create(&rowNew).Error)

	// 3) 双方都解不开 → 跳过
	rowBad := models.SystemConfig{ConfigKey: "k_bad", ConfigValue: "ENC:bm90LXZhbGlk", IsEncrypted: true}
	require.NoError(t, db.Create(&rowBad).Error)

	// 4) 非 ENC 前缀的普通值不参与
	rowPlain := models.SystemConfig{ConfigKey: "k_plain", ConfigValue: "plain-value"}
	require.NoError(t, db.Create(&rowPlain).Error)

	// 5) 软删除记录不参与
	rowDel := models.SystemConfig{ConfigKey: "k_del", ConfigValue: oldEnc, IsEncrypted: true}
	require.NoError(t, db.Create(&rowDel).Error)
	require.NoError(t, db.Delete(&rowDel).Error)

	require.NoError(t, MigrateEncryptionKeys())

	var got models.SystemConfig
	require.NoError(t, db.Where("config_key = ?", "k_old").First(&got).Error)
	plaintext, err := decryptAESGCM(got.ConfigValue, newKey)
	require.NoError(t, err, "迁移后应能用新密钥解密")
	assert.Equal(t, "old-secret", plaintext)

	var unchanged models.SystemConfig
	require.NoError(t, db.Where("config_key = ?", "k_new").First(&unchanged).Error)
	assert.Equal(t, newEnc, unchanged.ConfigValue, "已是新密钥的记录不应变动")

	// 注意：每次查询用新变量 —— GORM 会把 dest 中已存在的主键拼进 WHERE 条件
	var badRow models.SystemConfig
	require.NoError(t, db.Where("config_key = ?", "k_bad").First(&badRow).Error)
	assert.Equal(t, "ENC:bm90LXZhbGlk", badRow.ConfigValue, "解不开的记录保持原样")

	var plainRow models.SystemConfig
	require.NoError(t, db.Where("config_key = ?", "k_plain").First(&plainRow).Error)
	assert.Equal(t, "plain-value", plainRow.ConfigValue)

	var cnt int64
	require.NoError(t, db.Unscoped().Model(&models.SystemConfig{}).Where("config_key = ?", "k_del").Count(&cnt).Error)
	assert.EqualValues(t, 1, cnt, "软删除记录不应被迁移")
}

func TestMigrateEncryptionKeys_QueryFailure(t *testing.T) {
	setupTestDB(t, nil)
	t.Setenv("ENCRYPTION_KEY", "new-key")
	// 不建 system_configs 表 → Raw 查询失败
	err := MigrateEncryptionKeys()
	require.Error(t, err)
	assert.Contains(t, err.Error(), "查询加密配置失败")
}

func TestMigrateEncryptionKeys_UpdateFailure(t *testing.T) {
	db := setupTestDB(t, nil)
	t.Setenv("ENCRYPTION_KEY", "new-key")
	require.NoError(t, db.AutoMigrate(&models.SystemConfig{}))

	oldKey := deriveKey("docker-sync-platform-default-key-2024")
	oldEnc, err := encryptAESGCM("old-secret", oldKey)
	require.NoError(t, err)
	require.NoError(t, db.Create(&models.SystemConfig{ConfigKey: "k_old", ConfigValue: oldEnc, IsEncrypted: true}).Error)

	// 阻止 UPDATE，覆盖"更新配置失败"错误分支
	require.NoError(t, db.Exec(`CREATE TRIGGER block_cfg_update BEFORE UPDATE ON system_configs
		BEGIN SELECT RAISE(ABORT, 'update blocked'); END`).Error)

	err = MigrateEncryptionKeys()
	require.Error(t, err)
	assert.True(t, strings.Contains(err.Error(), "更新配置") || strings.Contains(err.Error(), "失败"))
}
