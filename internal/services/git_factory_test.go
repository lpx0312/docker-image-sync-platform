package services

// git_factory_test.go：GitServiceFactory 全方法测试（内存库 + 配置驱动）。

import (
	"testing"

	"docker-image-sync-platform/internal/models"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestGitServiceFactory_All(t *testing.T) {
	db := newSvcTestDB(t)
	withGlobalDB(t, db)
	enc := svcEnc(t)
	f := NewGitServiceFactory(enc)

	t.Run("默认走优化服务", func(t *testing.T) {
		require.True(t, f.IsUsingOptimized())
		svc, err := f.GetGitServiceInterface()
		require.NoError(t, err)
		assert.NotNil(t, svc)
		opt, err := f.GetOptimizedGitService()
		require.NoError(t, err)
		assert.NotNil(t, opt)
	})

	t.Run("关闭优化后按类型创建", func(t *testing.T) {
		f2 := NewGitServiceFactory(enc)
		f2.SetUseOptimized(false)
		assert.False(t, f2.IsUsingOptimized())

		// 无配置 → 默认 gitee
		svc, err := f2.GetGitServiceInterface()
		require.NoError(t, err)
		assert.NotNil(t, svc)

		// 配置 github
		require.NoError(t, db.Create(&models.SystemConfig{ConfigKey: "git_repository_type", ConfigValue: "github"}).Error)
		f2.RefreshConfig()
		svc, err = f2.GetGitServiceInterface()
		require.NoError(t, err)
		assert.NotNil(t, svc)

		// 非法类型
		require.NoError(t, db.Model(&models.SystemConfig{}).Where("config_key = ?", "git_repository_type").
			Update("config_value", "gitlab").Error)
		f2.RefreshConfig()
		_, err = f2.GetGitServiceInterface()
		require.Error(t, err)
		assert.Contains(t, err.Error(), "不支持的Git仓库类型")
	})

	t.Run("配置缓存与刷新", func(t *testing.T) {
		f3 := NewGitServiceFactory(enc)
		got, err := f3.GetGitRepositoryConfig()
		require.NoError(t, err)
		assert.Equal(t, "gitlab", got, "读库值并缓存")

		// 改库不刷新 → 仍读缓存
		require.NoError(t, db.Model(&models.SystemConfig{}).Where("config_key = ?", "git_repository_type").
			Update("config_value", "gitee").Error)
		got, err = f3.GetGitRepositoryConfig()
		require.NoError(t, err)
		assert.Equal(t, "gitlab", got)

		f3.RefreshConfig()
		got, err = f3.GetGitRepositoryConfig()
		require.NoError(t, err)
		assert.Equal(t, "gitee", got)
	})

	t.Run("UpdateGitRepositoryConfig", func(t *testing.T) {
		f4 := NewGitServiceFactory(enc)
		require.NoError(t, f4.UpdateGitRepositoryConfig("github"))
		got, err := f4.GetGitRepositoryConfig()
		require.NoError(t, err)
		assert.Equal(t, "github", got)

		// 无记录时创建
		require.NoError(t, db.Exec("DELETE FROM system_configs WHERE config_key = 'git_repository_type'").Error)
		f4.RefreshConfig()
		require.NoError(t, f4.UpdateGitRepositoryConfig("gitee"))
		var cfg models.SystemConfig
		require.NoError(t, db.Where("config_key = ?", "git_repository_type").First(&cfg).Error)
		assert.Equal(t, "gitee", cfg.ConfigValue)

		require.Error(t, f4.UpdateGitRepositoryConfig("svn"))
	})

	t.Run("GitFileService 创建与缓存", func(t *testing.T) {
		f5 := NewGitServiceFactory(enc)
		require.NoError(t, db.Model(&models.SystemConfig{}).Where("config_key = ?", "git_repository_type").
			Update("config_value", "github").Error)
		// GitHub 文件服务依赖完整 github 配置
		for k, v := range map[string]string{
			"github_repo_url": "https://github.com/own/rep",
			"github_username": "own",
			"github_token":    "tk",
			"github_email":    "own@example.com",
			"github_branch":   "main",
		} {
			require.NoError(t, db.Create(&models.SystemConfig{ConfigKey: k, ConfigValue: v}).Error)
		}
		f5.RefreshConfig()

		fs, err := f5.GetGitFileService()
		require.NoError(t, err)
		assert.NotNil(t, fs)

		// 缓存：二次获取同实例
		fs2, err := f5.GetGitFileService()
		require.NoError(t, err)
		assert.Same(t, fs, fs2) // 接口比较：同指针

		f5.ClearGitFileServiceCache()
		// 清理空缓存不 panic
		f5.ClearGitFileServiceCache()

		// 非法类型
		require.NoError(t, db.Model(&models.SystemConfig{}).Where("config_key = ?", "git_repository_type").
			Update("config_value", "svn").Error)
		f5.RefreshConfig()
		f5.ClearGitFileServiceCache()
		_, err = f5.GetGitFileService()
		require.Error(t, err)
	})

	t.Run("API 模式开关", func(t *testing.T) {
		f6 := NewGitServiceFactory(enc)
		assert.True(t, f6.IsUsingAPI())
		f6.SetUseAPI(false)
		assert.False(t, f6.IsUsingAPI())
	})

	t.Run("GitHubAPIService 单例与缓存清理", func(t *testing.T) {
		f7 := NewGitServiceFactory(enc)
		s1 := f7.GetGitHubAPIService()
		require.NotNil(t, s1)
		s2 := f7.GetGitHubAPIService()
		assert.Same(t, s1, s2)

		f7.ClearGitHubAPIServiceCache()
		f7.ClearGitHubAPIServiceCache() // 空缓存不 panic
		s3 := f7.GetGitHubAPIService()
		assert.NotSame(t, s1, s3)
	})

	t.Run("ConfigService 与 EncryptionService 透出", func(t *testing.T) {
		f8 := NewGitServiceFactory(enc)
		assert.NotNil(t, f8.GetConfigService())
		assert.Same(t, enc, f8.GetEncryptionService())
	})
}
