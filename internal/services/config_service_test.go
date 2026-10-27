package services

import (
	"testing"

	"docker-image-sync-platform/internal/models"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// newTestConfigService 构造 ConfigService（内存库 + 默认密钥加密服务）
func newTestConfigService(t *testing.T) (*ConfigService, *EncryptionService) {
	t.Helper()
	db := newSvcTestDB(t)
	enc := newSvcEncryptionService(t, "config-test-key")
	cs := NewConfigService(db, enc, newSilentLogger())
	return cs, enc
}

func TestConfigNeedsEncryption(t *testing.T) {
	cs, _ := newTestConfigService(t)
	assert.True(t, cs.needsEncryption("git_password"))
	assert.True(t, cs.needsEncryption("github_token"))
	assert.True(t, cs.needsEncryption("my_secret"))
	assert.True(t, cs.needsEncryption("aliyun_key"))
	assert.False(t, cs.needsEncryption("config_key")) // config_key 特例
	assert.False(t, cs.needsEncryption("git_repo_url"))
	// 大小写不敏感
	assert.True(t, cs.needsEncryption("GitHub_Token"))
}

func TestConfigSetAndGet(t *testing.T) {
	cs, enc := newTestConfigService(t)
	db := cs.db

	t.Run("普通配置明文存储", func(t *testing.T) {
		require.NoError(t, cs.SetConfig("git_repo_url", "https://github.com/u/r", "desc", "git", 1))
		var row models.SystemConfig
		require.NoError(t, db.Where("config_key = ?", "git_repo_url").First(&row).Error)
		assert.Equal(t, "https://github.com/u/r", row.ConfigValue)
		assert.False(t, row.IsEncrypted)
		assert.Equal(t, "git", row.ConfigGroup)
		assert.Equal(t, 1, row.DisplayOrder)

		val, err := cs.GetConfig("git_repo_url")
		require.NoError(t, err)
		assert.Equal(t, "https://github.com/u/r", val)
	})

	t.Run("敏感配置加密存储", func(t *testing.T) {
		require.NoError(t, cs.SetConfig("github_token", "ghp-secret", "desc", "git", 2))
		var row models.SystemConfig
		require.NoError(t, db.Where("config_key = ?", "github_token").First(&row).Error)
		assert.True(t, row.IsEncrypted)
		assert.True(t, enc.IsEncrypted(row.ConfigValue), "密文应带 ENC: 前缀")
		assert.NotEqual(t, "ghp-secret", row.ConfigValue)

		val, err := cs.GetConfig("github_token")
		require.NoError(t, err)
		assert.Equal(t, "ghp-secret", val)
	})

	t.Run("更新已有配置", func(t *testing.T) {
		require.NoError(t, cs.SetConfig("plain_val", "v1", "", "system", 1))
		require.NoError(t, cs.SetConfig("plain_val", "v2", "d2", "system", 5))
		var row models.SystemConfig
		require.NoError(t, cs.db.Where("config_key = ?", "plain_val").First(&row).Error)
		assert.Equal(t, "v2", row.ConfigValue)
		assert.Equal(t, "d2", row.Description)
		assert.Equal(t, 5, row.DisplayOrder)

		val, err := cs.GetConfig("plain_val")
		require.NoError(t, err)
		assert.Equal(t, "v2", val)
	})

	t.Run("缓存命中", func(t *testing.T) {
		cs.cache.Store("cached_key", "cached-value")
		val, err := cs.GetConfig("cached_key")
		require.NoError(t, err)
		assert.Equal(t, "cached-value", val)
	})

	t.Run("不存在", func(t *testing.T) {
		_, err := cs.GetConfig("missing_key")
		require.Error(t, err)
		assert.Contains(t, err.Error(), "config not found")
	})

	t.Run("解密失败", func(t *testing.T) {
		require.NoError(t, cs.db.Create(&models.SystemConfig{
			ConfigKey: "bad_secret", ConfigValue: "ENC:!!!invalid-base64!!!", IsEncrypted: true,
		}).Error)
		_, err := cs.GetConfig("bad_secret")
		require.Error(t, err)
		assert.Contains(t, err.Error(), "failed to decrypt config value")
	})
}

func TestConfigSetErrors(t *testing.T) {
	t.Run("加密失败", func(t *testing.T) {
		cs, _ := newTestConfigService(t)
		// 敏感 key + 空值 → Encrypt("") 失败
		err := cs.SetConfig("github_token", "", "", "git", 1)
		require.Error(t, err)
		assert.Contains(t, err.Error(), "failed to encrypt config value")
	})

	t.Run("创建失败", func(t *testing.T) {
		cs, _ := newTestConfigService(t)
		require.NoError(t, cs.db.Exec(
			`CREATE TRIGGER trg_cfg_no_insert BEFORE INSERT ON system_configs BEGIN SELECT RAISE(ABORT, 'no insert'); END`).Error)
		err := cs.SetConfig("some_key", "v", "", "git", 1)
		require.Error(t, err)
		assert.Contains(t, err.Error(), "failed to create config")
	})

	t.Run("更新失败", func(t *testing.T) {
		cs, _ := newTestConfigService(t)
		require.NoError(t, cs.SetConfig("upd_key", "v1", "", "git", 1))
		require.NoError(t, cs.db.Exec(
			`CREATE TRIGGER trg_cfg_no_update BEFORE UPDATE ON system_configs BEGIN SELECT RAISE(ABORT, 'no update'); END`).Error)
		err := cs.SetConfig("upd_key", "v2", "", "git", 1)
		require.Error(t, err)
		assert.Contains(t, err.Error(), "failed to update config")
	})

	t.Run("查询失败", func(t *testing.T) {
		cs, _ := newTestConfigService(t)
		cs.db = svcClosedDB(t)
		err := cs.SetConfig("k", "v", "", "git", 1)
		require.Error(t, err)
		assert.Contains(t, err.Error(), "failed to query config")
	})
}

func TestConfigGetError(t *testing.T) {
	cs, _ := newTestConfigService(t)
	cs.db = svcClosedDB(t)
	_, err := cs.GetConfig("any")
	require.Error(t, err)
	assert.Contains(t, err.Error(), "failed to query config")
}

func TestConfigGetConfigsByGroup(t *testing.T) {
	cs, _ := newTestConfigService(t)
	require.NoError(t, cs.SetConfig("g_url", "https://x", "", "git", 2))
	require.NoError(t, cs.SetConfig("g_token", "tok", "", "git", 1))
	require.NoError(t, cs.SetConfig("other", "v", "", "system", 1))

	configs, err := cs.GetConfigsByGroup("git")
	require.NoError(t, err)
	require.Len(t, configs, 2)
	// 排序：display_order 升序（g_token=1 在前）
	assert.Equal(t, "g_token", configs[0].ConfigKey)
	assert.Equal(t, "***", configs[0].ConfigValue, "加密配置应脱敏")
	assert.Equal(t, "https://x", configs[1].ConfigValue)

	// 错误分支
	cs2, _ := newTestConfigService(t)
	cs2.db = svcClosedDB(t)
	_, err = cs2.GetConfigsByGroup("git")
	require.Error(t, err)
	assert.Contains(t, err.Error(), "failed to query configs by group")
}

func TestConfigDelete(t *testing.T) {
	t.Run("成功并清缓存", func(t *testing.T) {
		cs, _ := newTestConfigService(t)
		require.NoError(t, cs.SetConfig("del_key", "v", "", "git", 1))
		require.NoError(t, cs.DeleteConfig("del_key"))

		var count int64
		cs.db.Model(&models.SystemConfig{}).Where("config_key = ?", "del_key").Count(&count)
		assert.Zero(t, count)
		// 缓存清理后再读应 not found
		_, err := cs.GetConfig("del_key")
		require.Error(t, err)
	})

	t.Run("不存在", func(t *testing.T) {
		cs, _ := newTestConfigService(t)
		err := cs.DeleteConfig("missing")
		require.Error(t, err)
		assert.Contains(t, err.Error(), "config not found")
	})

	t.Run("查询失败", func(t *testing.T) {
		cs, _ := newTestConfigService(t)
		cs.db = svcClosedDB(t)
		err := cs.DeleteConfig("k")
		require.Error(t, err)
		assert.Contains(t, err.Error(), "failed to query config")
	})

	t.Run("删除失败", func(t *testing.T) {
		cs, _ := newTestConfigService(t)
		require.NoError(t, cs.SetConfig("del_key2", "v", "", "git", 1))
		// SystemConfig 为软删除（GORM 转为 UPDATE deleted_at），拦截 UPDATE
		require.NoError(t, cs.db.Exec(
			`CREATE TRIGGER trg_cfg_no_delete BEFORE UPDATE ON system_configs BEGIN SELECT RAISE(ABORT, 'no soft delete'); END`).Error)
		err := cs.DeleteConfig("del_key2")
		require.Error(t, err)
		assert.Contains(t, err.Error(), "failed to delete config")
	})
}

func TestConfigGitConfig(t *testing.T) {
	t.Run("GitHub配置", func(t *testing.T) {
		cs, _ := newTestConfigService(t)
		err := cs.SetGitConfig("github", GitConfig{
			RepoURL:  "https://github.com/u/r",
			Username: "alice",
			Token:    "ghp-tok",
			Email:    "a@x.com",
			Branch:   "main",
		})
		require.NoError(t, err)

		got, err := cs.GetGitConfig("github")
		require.NoError(t, err)
		assert.Equal(t, "https://github.com/u/r", got.RepoURL)
		assert.Equal(t, "alice", got.Username)
		assert.Equal(t, "ghp-tok", got.Token, "token 应解密回明文")
		assert.Equal(t, "a@x.com", got.Email)
		assert.Equal(t, "main", got.Branch)

		// token 加密落库
		var row models.SystemConfig
		require.NoError(t, cs.db.Where("config_key = ?", "github_token").First(&row).Error)
		assert.True(t, row.IsEncrypted)
	})

	t.Run("Gitee配置含password与token", func(t *testing.T) {
		cs, _ := newTestConfigService(t)
		err := cs.SetGitConfig("gitee", GitConfig{
			RepoURL:  "https://gitee.com/u/r",
			Username: "bob",
			Password: "pw",
			Token:    "gtok",
			Branch:   "dev",
		})
		require.NoError(t, err)

		got, err := cs.GetGitConfig("gitee")
		require.NoError(t, err)
		assert.Equal(t, "pw", got.Password)
		assert.Equal(t, "gtok", got.Token)
	})

	t.Run("空值跳过", func(t *testing.T) {
		cs, _ := newTestConfigService(t)
		require.NoError(t, cs.SetGitConfig("github", GitConfig{RepoURL: "https://x"}))
		got, err := cs.GetGitConfig("github")
		require.NoError(t, err)
		assert.Equal(t, "https://x", got.RepoURL)
		assert.Empty(t, got.Username)
		assert.Empty(t, got.Token)
	})

	t.Run("事务失败回滚", func(t *testing.T) {
		cs, _ := newTestConfigService(t)
		require.NoError(t, cs.db.Exec(
			`CREATE TRIGGER trg_cfg_no_insert2 BEFORE INSERT ON system_configs BEGIN SELECT RAISE(ABORT, 'no insert'); END`).Error)
		err := cs.SetGitConfig("github", GitConfig{RepoURL: "https://x", Token: "t"})
		require.Error(t, err)
		assert.Contains(t, err.Error(), "failed to set")
	})
}

func TestConfigAliyunConfig(t *testing.T) {
	cs, _ := newTestConfigService(t)

	err := cs.SetAliyunConfig(AliyunConfig{
		Registry:  "registry.cn-hangzhou.aliyuncs.com",
		Namespace: "ns",
		Username:  "u",
		Password:  "pw",
		Region:    "cn-hangzhou",
	})
	require.NoError(t, err)

	got, err := cs.GetAliyunConfig()
	require.NoError(t, err)
	assert.Equal(t, "registry.cn-hangzhou.aliyuncs.com", got.Registry)
	assert.Equal(t, "ns", got.Namespace)
	assert.Equal(t, "u", got.Username)
	assert.Equal(t, "pw", got.Password, "密码应解密回明文")
	assert.Equal(t, "cn-hangzhou", got.Region)

	// 全空配置 → 无写入，直接成功
	err = cs.SetAliyunConfig(AliyunConfig{})
	require.NoError(t, err)

	// 失败回滚
	cs2, _ := newTestConfigService(t)
	require.NoError(t, cs2.db.Exec(
		`CREATE TRIGGER trg_cfg_no_insert3 BEFORE INSERT ON system_configs BEGIN SELECT RAISE(ABORT, 'no insert'); END`).Error)
	err = cs2.SetAliyunConfig(AliyunConfig{Registry: "r"})
	require.Error(t, err)
	assert.Contains(t, err.Error(), "failed to set")
}
