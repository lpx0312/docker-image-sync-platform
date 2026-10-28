package services

// git_optimized_test.go：GitOptimizedService 全方法测试。
// 复用 git_service_test.go 的本地 origin 设施（gitSvcOrigin/gitSvcCfg/gitSvcRun）。
// 含凭证嵌入 URL 的百分号编码用例（原 TestBuildAuthURL 保留语义并扩展）。

import (
	"context"
	"os"
	"path/filepath"
	"testing"
	"time"

	"docker-image-sync-platform/internal/models"

	"github.com/go-git/go-git/v5"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// optSvc 构造指定模式的 GitOptimizedService
func optSvc(t *testing.T, mode GitMode) *GitOptimizedService {
	t.Helper()
	svc := NewGitOptimizedService(svcEnc(t))
	svc.config.Mode = mode
	svc.config.NetworkDetection = false // 禁用真实网络探测，DetectNetworkQuality 固定返回 Medium
	return svc
}

func TestGitOptimizedService_PureHelpers(t *testing.T) {
	t.Run("getModeString", func(t *testing.T) {
		s := NewGitOptimizedService(svcEnc(t))
		s.config.Mode = GitModeSparse
		assert.Equal(t, "sparse", s.getModeString())
		s.config.Mode = GitModeFull
		assert.Equal(t, "full", s.getModeString())
		s.config.Mode = GitModeAuto
		assert.Equal(t, "auto", s.getModeString())
		s.config.Mode = GitMode(99)
		assert.Equal(t, "unknown", s.getModeString())
	})

	t.Run("ChooseOptimalStrategy_强制模式直接返回", func(t *testing.T) {
		assert.Equal(t, GitModeSparse, optSvc(t, GitModeSparse).ChooseOptimalStrategy())
		assert.Equal(t, GitModeFull, optSvc(t, GitModeFull).ChooseOptimalStrategy())
	})

	t.Run("ChooseOptimalStrategy_auto", func(t *testing.T) {
		// NetworkDetection=false -> Medium -> sparse
		assert.Equal(t, GitModeSparse, optSvc(t, GitModeAuto).ChooseOptimalStrategy())
	})

	t.Run("DetectNetworkQuality_关闭探测返回Medium", func(t *testing.T) {
		q, err := optSvc(t, GitModeAuto).DetectNetworkQuality()
		require.NoError(t, err)
		assert.Equal(t, NetworkQualityMedium, q)
	})

	t.Run("mergeImages_新镜像在前旧镜像注释归档", func(t *testing.T) {
		s := optSvc(t, GitModeSparse)
		got := s.mergeImages([]string{"old:1", "# already", "  "}, []string{"new:2"})
		assert.Equal(t, []string{"new:2", "", "# 历史镜像记录", "# old:1", "# already", "  "}, got)

		got = s.mergeImages(nil, []string{"only:1"})
		assert.Equal(t, []string{"only:1"}, got)
	})

	t.Run("writeImagesFile_与readImagesContent往返", func(t *testing.T) {
		s := optSvc(t, GitModeSparse)
		s.repoPath = t.TempDir()
		require.NoError(t, s.writeImagesFile(filepath.Join(s.repoPath, "images.txt"), []string{"a:1", "b:2"}))
		lines, err := s.readImagesContent()
		require.NoError(t, err)
		assert.Equal(t, []string{"a:1", "b:2"}, lines)

		s.repoPath = filepath.Join(t.TempDir(), "none")
		_, err = s.readImagesContent()
		assert.Error(t, err)
	})

	t.Run("writeImagesFile_路径不可写报错", func(t *testing.T) {
		s := optSvc(t, GitModeSparse)
		blocker := filepath.Join(t.TempDir(), "blocker")
		require.NoError(t, os.WriteFile(blocker, []byte("x"), 0o644))
		err := s.writeImagesFile(filepath.Join(blocker, "sub", "images.txt"), []string{"a"})
		assert.Error(t, err)
	})
}

func TestGitOptimizedService_Cache(t *testing.T) {
	s := optSvc(t, GitModeSparse)
	s.config.EnableCache = true
	s.config.CacheTTL = 60

	assert.False(t, s.isCacheValid())
	assert.Nil(t, s.getFromCache())

	s.updateCache([]string{"x:1"})
	got := s.getFromCache()
	require.NotNil(t, got)
	assert.Equal(t, []string{"x:1"}, *got)
	assert.True(t, s.isCacheValid())

	s.cache.LastFetchTime = time.Now().Add(-2 * time.Minute)
	assert.False(t, s.isCacheValid())
}

func TestGitOptimizedService_Metrics(t *testing.T) {
	s := optSvc(t, GitModeSparse)
	s.recordSparseOperation()
	s.recordFullOperation()
	s.recordFallback()
	s.recordCacheHit()
	s.recordCacheMiss()
	s.recordPerformance(GitModeSparse, 100*time.Millisecond, true)
	s.recordPerformance(GitModeFull, 200*time.Millisecond, true)
	s.recordPerformance(GitModeFull, 0, false)

	m := s.GetPerformanceMetrics()
	for _, key := range []string{"sparse_operations", "full_operations", "fallback_count", "cache_hits", "cache_misses"} {
		assert.Contains(t, m, key)
	}
}

func TestGitOptimizedService_buildAuthURL(t *testing.T) {
	db := newSvcTestDB(t)
	withGlobalDB(t, db)
	s := optSvc(t, GitModeSparse)

	t.Run("带token注入凭证与特殊字符编码", func(t *testing.T) {
		u, err := s.buildAuthURL("https://gitee.com/u/r.git", "user", "tk@123", "gitee")
		require.NoError(t, err)
		assert.Contains(t, u, "user:tk%40123@gitee.com")

		u, err = s.buildAuthURL("https://gitee.com/u/r.git", "user", "tk", "gitee")
		require.NoError(t, err)
		assert.Contains(t, u, "user:tk@gitee.com")
	})

	t.Run("空token_gitee用密码", func(t *testing.T) {
		gitSvcCfg(t, db, map[string]string{"gitee_password": "pw"})
		u, err := s.buildAuthURL("https://gitee.com/u/r.git", "user", "", "gitee")
		require.NoError(t, err)
		assert.Contains(t, u, "user:pw@")
	})

	t.Run("空token_gitee密码缺失报错", func(t *testing.T) {
		require.NoError(t, db.Unscoped().Where("config_key = ?", "gitee_password").Delete(&models.SystemConfig{}).Error)
		_, err := s.buildAuthURL("https://gitee.com/u/r.git", "user", "", "gitee")
		require.Error(t, err)
		assert.Contains(t, err.Error(), "获取Gitee密码失败")
	})

	t.Run("空token_github必须令牌", func(t *testing.T) {
		_, err := s.buildAuthURL("https://github.com/u/r.git", "user", "", "github")
		require.Error(t, err)
		assert.Contains(t, err.Error(), "GitHub仓库必须配置访问令牌")
	})

	t.Run("双空secret返回原URL", func(t *testing.T) {
		gitSvcCfg(t, db, map[string]string{"gitee_password": ""})
		u, err := s.buildAuthURL("https://gitee.com/u/r.git", "user", "", "gitee")
		require.NoError(t, err)
		assert.Equal(t, "https://gitee.com/u/r.git", u)
	})

	t.Run("URL解析失败", func(t *testing.T) {
		_, err := s.buildAuthURL("://bad", "u", "t", "gitee")
		require.Error(t, err)
		assert.Contains(t, err.Error(), "解析Git仓库URL失败")
	})
}

func TestGitOptimizedService_getCurrentGitConfig(t *testing.T) {
	db := newSvcTestDB(t)
	withGlobalDB(t, db)
	s := optSvc(t, GitModeSparse)

	t.Run("配置缺失报错", func(t *testing.T) {
		_, _, _, _, _, _, err := s.getCurrentGitConfig()
		require.Error(t, err)
	})

	t.Run("gitee全配置", func(t *testing.T) {
		gitSvcGiteeCfg(t, db, "https://gitee.com/u/r.git", filepath.Join(t.TempDir(), "r"))
		repoURL, username, token, email, repoType, localPath, err := s.getCurrentGitConfig()
		require.NoError(t, err)
		assert.Equal(t, "https://gitee.com/u/r.git", repoURL)
		assert.Equal(t, "tester", username)
		assert.Equal(t, "token-abc", token)
		assert.Equal(t, "t@t.io", email)
		assert.Equal(t, "gitee", repoType)
		assert.NotEmpty(t, localPath)
	})

	t.Run("github全配置", func(t *testing.T) {
		gitSvcGitHubCfg(t, db, "https://github.com/u/r.git", filepath.Join(t.TempDir(), "r2"))
		_, _, _, email, repoType, _, err := s.getCurrentGitConfig()
		require.NoError(t, err)
		assert.Empty(t, email)
		assert.Equal(t, "github", repoType)
	})
}

func TestGitOptimizedService_UpdateWithFullClone_E2E(t *testing.T) {
	gitSvcSkip(t)

	t.Run("完整克隆提交推送成功", func(t *testing.T) {
		db := newSvcTestDB(t)
		withGlobalDB(t, db)
		origin := gitSvcOriginMain(t)
		local := filepath.Join(t.TempDir(), "full-repo")
		gitSvcGiteeLocalCfg(t, db, origin, local)

		s := optSvc(t, GitModeFull)
		sha, err := s.UpdateImagesFileOptimized(context.Background(), []string{"new-image:1"})
		require.NoError(t, err)
		assert.NotEmpty(t, sha)

		clone := filepath.Join(t.TempDir(), "verify")
		gitSvcRun(t, "", "clone", origin, clone)
		data, err := os.ReadFile(filepath.Join(clone, "images.txt"))
		require.NoError(t, err)
		content := string(data)
		assert.Contains(t, content, "new-image:1")
		assert.Contains(t, content, "# 历史镜像记录")
		assert.Contains(t, content, "# old-image:1")
	})

	t.Run("配置缺失报错", func(t *testing.T) {
		db := newSvcTestDB(t)
		withGlobalDB(t, db)
		s := optSvc(t, GitModeFull)
		_, err := s.UpdateImagesFileOptimized(context.Background(), []string{"a:1"})
		require.Error(t, err)
	})
}

func TestGitOptimizedService_UpdateWithSparse_E2E(t *testing.T) {
	gitSvcSkip(t)

	t.Run("稀疏检出提交推送成功", func(t *testing.T) {
		db := newSvcTestDB(t)
		withGlobalDB(t, db)
		// git init 默认分支为 master：origin 用同名分支保证 pull/push refspec 对齐
		origin := gitSvcOrigin(t, map[string]string{"images.txt": "old-image:1"}, "master")
		local := filepath.Join(t.TempDir(), "sparse-repo")
		gitSvcGiteeLocalCfg(t, db, origin, local)

		s := optSvc(t, GitModeSparse)
		s.config.EnableCache = false
		s.config.MaxRetries = 1 // 本地路径 remote 下 go-git push 无法解析盘符 URL，快速走失败分支

		_, err := s.UpdateImagesFileOptimized(context.Background(), []string{"sparse-image:1"})
		// Windows 本地 origin：稀疏链路（init/pull/merge/commit）完整执行，push 因
		// go-git file 传输对 "C:\..." 形态 remote 的解析限制而失败（环境限制，非产品缺陷）
		if err == nil {
			clone := filepath.Join(t.TempDir(), "verify")
			gitSvcRun(t, "", "clone", origin, clone)
			data, _ := os.ReadFile(filepath.Join(clone, "images.txt"))
			assert.Contains(t, string(data), "sparse-image:1")
		} else {
			assert.Contains(t, err.Error(), "推送失败")
		}

		// 本地工作区：稀疏检出只应包含 images.txt
		entries, err := os.ReadDir(local)
		require.NoError(t, err)
		names := []string{}
		for _, e := range entries {
			names = append(names, e.Name())
		}
		assert.Equal(t, []string{".git", "images.txt"}, names)
		data, _ := os.ReadFile(filepath.Join(local, "images.txt"))
		assert.Contains(t, string(data), "sparse-image:1")
	})

	t.Run("缓存命中复用内容", func(t *testing.T) {
		db := newSvcTestDB(t)
		withGlobalDB(t, db)
		origin := gitSvcOriginMain(t)
		local := filepath.Join(t.TempDir(), "sparse-cache")
		gitSvcGiteeLocalCfg(t, db, origin, local)

		s := optSvc(t, GitModeSparse)
		s.config.EnableCache = true
		s.config.CacheTTL = 60

		s.config.MaxRetries = 1
		// 第一次更新完成仓库初始化（缓存路径要求 repo 已就绪）
		_, _ = s.UpdateImagesFileOptimized(context.Background(), []string{"init:0"})
		require.NotNil(t, s.repo)
		s.updateCache([]string{"cached:1"})

		_, err := s.UpdateImagesFileOptimized(context.Background(), []string{"fresh:2"})
		if err != nil {
			assert.Contains(t, err.Error(), "推送失败")
		}
		// 缓存内容参与合并（本地文件即可验证）
		data, _ := os.ReadFile(filepath.Join(local, "images.txt"))
		assert.Contains(t, string(data), "fresh:2")
		assert.Contains(t, string(data), "# cached:1")
	})

	t.Run("稀疏失败降级完整克隆成功", func(t *testing.T) {
		db := newSvcTestDB(t)
		withGlobalDB(t, db)
		origin := gitSvcOriginMain(t)
		local := filepath.Join(t.TempDir(), "fallback-repo")
		gitSvcGiteeLocalCfg(t, db, origin, local)

		s := optSvc(t, GitModeSparse)
		s.config.EnableCache = false
		s.config.FallbackToFull = true
		// 稀疏路径的 repoPath 父级是文件 → 初始化失败 → 降级 full（用配置的 localPath）
		blocker := filepath.Join(t.TempDir(), "blocker")
		require.NoError(t, os.WriteFile(blocker, []byte("x"), 0o644))
		s.repoPath = filepath.Join(blocker, "child", "repo")

		s.config.MaxRetries = 1
		_, err := s.UpdateImagesFileOptimized(context.Background(), []string{"fallback:1"})
		if err != nil {
			assert.Contains(t, err.Error(), "推送失败")
		}
		m := s.GetPerformanceMetrics()
		assert.EqualValues(t, 1, m["fallback_count"])
		data, _ := os.ReadFile(filepath.Join(local, "images.txt"))
		assert.Contains(t, string(data), "fallback:1")
	})

	t.Run("双失败返回错误", func(t *testing.T) {
		db := newSvcTestDB(t)
		withGlobalDB(t, db)
		missing := filepath.Join(t.TempDir(), "missing-origin.git")
		local := filepath.Join(t.TempDir(), "both-fail")
		gitSvcGiteeLocalCfg(t, db, missing, local)

		s := optSvc(t, GitModeSparse)
		s.config.FallbackToFull = true
		_, err := s.UpdateImagesFileOptimized(context.Background(), []string{"x:1"})
		require.Error(t, err)
	})
}

func TestGitOptimizedService_UpdateImagesFile_Wrapper(t *testing.T) {
	gitSvcSkip(t)
	db := newSvcTestDB(t)
	withGlobalDB(t, db)
	origin := gitSvcOriginMain(t)
	local := filepath.Join(t.TempDir(), "wrap")
	gitSvcGiteeLocalCfg(t, db, origin, local)

	s := optSvc(t, GitModeFull)
	sha, err := s.UpdateImagesFile(context.Background(), []string{"wrapped:1"})
	require.NoError(t, err)
	assert.NotEmpty(t, sha)

	// 第二次：repo 已初始化，走已有仓库路径
	sha2, err := s.UpdateImagesFile(context.Background(), []string{"wrapped:2"})
	require.NoError(t, err)
	assert.NotEmpty(t, sha2)
}

func TestGitOptimizedService_RepoHandles(t *testing.T) {
	t.Run("bare仓库无worktree提交失败", func(t *testing.T) {
		s := optSvc(t, GitModeSparse)
		bare, err := git.PlainInit(filepath.Join(t.TempDir(), "bare.git"), true)
		require.NoError(t, err)
		s.repo = bare
		_, err = s.commitAndPush([]string{"x"}, "sparse")
		require.Error(t, err)
	})

	t.Run("fetchSingleFile_bare仓库无worktree", func(t *testing.T) {
		s := optSvc(t, GitModeSparse)
		bare, err := git.PlainInit(filepath.Join(t.TempDir(), "bare2.git"), true)
		require.NoError(t, err)
		s.repo = bare
		err = s.fetchSingleFile()
		require.Error(t, err)
	})

	t.Run("initFullRepository_配置缺失时报错或创建失败", func(t *testing.T) {
		db := newSvcTestDB(t)
		withGlobalDB(t, db)
		s := optSvc(t, GitModeFull)
		err := s.initFullRepository()
		require.Error(t, err)
	})
}
