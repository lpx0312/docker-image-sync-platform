package services

// git_service_test.go：git.go（GitService，go-git 实现）全方法测试。
//
// 本地 origin 策略（Windows 真实行为，见各用例注释）：
//   - 用 go-git PlainInit 建 bare 仓库作为 origin，返回“原生反斜杠路径”。
//   - 该路径经 url.Parse 后是 opaque URL（scheme=c），生产代码嵌入的凭证
//     （url.UserPassword）在 String() 时被静默丢弃，因此系统 `git clone`
//     可直接成功 —— 恰好复刻生产“带凭证 URL 克隆”的真实路径。
//   - 克隆后 origin remote 也是该路径，go-git 将其按 file 传输解析
//     （parseFile，Path=原生路径），git-upload-pack/git-receive-pack 可
//     直接打开，Pull/Fetch/Push 全部可用（file 传输忽略 BasicAuth）。
//   - 注意不能用 file:///D:/... 形式：go-git 的 ep.Path 会带上前导斜杠
//     （/D:/...），git-upload-pack 在 Windows 上打不开，报 repository not found。

import (
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"docker-image-sync-platform/internal/models"

	"github.com/go-git/go-git/v5"
	"github.com/go-git/go-git/v5/config"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"
)

// --------------------------------------------------------------------
// 测试脚手架
// --------------------------------------------------------------------

// gitSvcSkip 系统无 git CLI 时跳过（生产代码依赖系统 git 执行 clone 等操作）
func gitSvcSkip(t *testing.T) {
	t.Helper()
	if _, err := exec.LookPath("git"); err != nil {
		t.Skip("系统缺少 git CLI，跳过依赖本地 git 的用例")
	}
}

// gitSvcRun 执行系统 git 命令，失败即 Fatal
func gitSvcRun(t *testing.T, dir string, args ...string) string {
	t.Helper()
	full := args
	if dir != "" {
		full = append([]string{"-C", dir}, args...)
	}
	out, err := exec.Command("git", full...).CombinedOutput()
	require.NoError(t, err, "git %v 失败: %s", args, string(out))
	return string(out)
}

// gitSvcOrigin 建 bare 仓库并在 branch 分支提交 files（key=相对路径，value=内容），
// 返回 origin 的原生路径
func gitSvcOrigin(t *testing.T, files map[string]string, branch string) string {
	t.Helper()
	base := t.TempDir()
	bare := filepath.Join(base, "origin.git")
	_, err := git.PlainInit(bare, true)
	require.NoError(t, err)

	seed := filepath.Join(base, "seed")
	gitSvcRun(t, "", "clone", bare, seed)
	for name, content := range files {
		dst := filepath.Join(seed, filepath.FromSlash(name))
		require.NoError(t, os.MkdirAll(filepath.Dir(dst), 0o755))
		require.NoError(t, os.WriteFile(dst, []byte(content), 0o644))
	}
	gitSvcRun(t, seed, "add", ".")
	gitSvcRun(t, seed, "-c", "user.name=tester", "-c", "user.email=t@t.io", "commit", "-m", "init")
	gitSvcRun(t, seed, "push", "origin", "HEAD:refs/heads/"+branch)
	gitSvcRun(t, bare, "symbolic-ref", "HEAD", "refs/heads/"+branch)
	return bare
}

// gitSvcOriginMain 常规 origin：main 分支 + images.txt
func gitSvcOriginMain(t *testing.T) string {
	t.Helper()
	return gitSvcOrigin(t, map[string]string{"images.txt": "old-image:1\n"}, "main")
}

// gitSvcCfg 直插一批系统配置（幂等：先硬删同 key —— 软删行也占用唯一索引）
func gitSvcCfg(t *testing.T, db *gorm.DB, kv map[string]string) {
	t.Helper()
	for k, v := range kv {
		require.NoError(t, db.Unscoped().Where("config_key = ?", k).Delete(&models.SystemConfig{}).Error)
		require.NoError(t, db.Create(&models.SystemConfig{ConfigKey: k, ConfigValue: v}).Error)
	}
}

// gitSvcGiteeCfg 种入完整 gitee 配置（带 token）
func gitSvcGiteeCfg(t *testing.T, db *gorm.DB, repoURL, localPath string) {
	t.Helper()
	gitSvcCfg(t, db, map[string]string{
		"git_repository_type": "gitee",
		"gitee_repo_url":      repoURL,
		"gitee_username":      "tester",
		"gitee_token":         "token-abc",
		"gitee_email":         "t@t.io",
		"gitee_local_path":    localPath,
	})
}

// gitSvcGitHubCfg 种入完整 github 配置
func gitSvcGitHubCfg(t *testing.T, db *gorm.DB, repoURL, localPath string) {
	t.Helper()
	gitSvcCfg(t, db, map[string]string{
		"git_repository_type": "github",
		"github_repo_url":     repoURL,
		"github_username":     "tester",
		"github_token":        "gh-token",
		"github_local_path":   localPath,
	})
}

// newGitSvcEnv 建服务实例并替换全局 DB
func newGitSvcEnv(t *testing.T) (*GitService, *gorm.DB, *EncryptionService) {
	t.Helper()
	db := newSvcTestDB(t)
	withGlobalDB(t, db)
	enc := svcEnc(t)
	return NewGitService(enc), db, enc
}

// gitSvcOpen 克隆 origin 到 dst 并用 go-git 打开，返回 (dst, repo)
func gitSvcOpen(t *testing.T, origin, dst string) (*git.Repository, string) {
	t.Helper()
	gitSvcRun(t, "", "clone", origin, dst)
	repo, err := git.PlainOpen(dst)
	require.NoError(t, err)
	return repo, dst
}

// gitSvcOriginFileContent 读取 origin main 分支上某文件内容
func gitSvcOriginFileContent(t *testing.T, origin, branch, name string) string {
	t.Helper()
	out := gitSvcRun(t, origin, "show", branch+":"+name)
	return out
}

// --------------------------------------------------------------------
// getCurrentGitConfig / NewGitService
// --------------------------------------------------------------------

func TestGitService_NewAndGetConfig(t *testing.T) {
	svc, db, _ := newGitSvcEnv(t)

	t.Run("构造函数", func(t *testing.T) {
		assert.NotNil(t, svc)
		assert.NotNil(t, svc.encryptionService)
		assert.Empty(t, svc.repoPath)
		assert.Nil(t, svc.repo)
	})

	t.Run("gitee完整配置", func(t *testing.T) {
		local := filepath.Join(t.TempDir(), "repo")
		gitSvcGiteeCfg(t, db, `https://gitee.com/u/r.git`, local)
		repoURL, username, token, email, repoType, localPath, err := svc.getCurrentGitConfig()
		require.NoError(t, err)
		assert.Equal(t, "https://gitee.com/u/r.git", repoURL)
		assert.Equal(t, "tester", username)
		assert.Equal(t, "token-abc", token)
		assert.Equal(t, "t@t.io", email)
		assert.Equal(t, "gitee", repoType)
		assert.Equal(t, local, localPath)
	})

	t.Run("类型缺失或非法时默认gitee", func(t *testing.T) {
		local := filepath.Join(t.TempDir(), "repo1")
		gitSvcCfg(t, db, map[string]string{
			"gitee_repo_url":   "u1",
			"gitee_username":   "u",
			"gitee_token":      "tk",
			"gitee_email":      "e",
			"gitee_local_path": local,
		})
		_, _, _, _, repoType, _, err := svc.getCurrentGitConfig()
		require.NoError(t, err)
		assert.Equal(t, "gitee", repoType)

		gitSvcCfg(t, db, map[string]string{"git_repository_type": "gitlab"})
		_, _, _, _, repoType, _, err = svc.getCurrentGitConfig()
		require.NoError(t, err)
		assert.Equal(t, "gitee", repoType) // default 分支
	})

	t.Run("github完整配置_email为空", func(t *testing.T) {
		local := filepath.Join(t.TempDir(), "repo2")
		gitSvcGitHubCfg(t, db, `https://github.com/u/r.git`, local)
		_, username, token, email, repoType, localPath, err := svc.getCurrentGitConfig()
		require.NoError(t, err)
		assert.Equal(t, "tester", username)
		assert.Equal(t, "gh-token", token)
		assert.Empty(t, email)
		assert.Equal(t, "github", repoType)
		assert.Equal(t, local, localPath)
	})

	t.Run("加密token自动解密", func(t *testing.T) {
		enc := svcEnc(t)
		encVal, err := enc.Encrypt("secret-token")
		require.NoError(t, err)
		gitSvcCfg(t, db, map[string]string{"git_repository_type": "gitee"})
		gitSvcCfg(t, db, map[string]string{
			"gitee_repo_url":   "u2",
			"gitee_username":   "u",
			"gitee_email":      "e",
			"gitee_local_path": filepath.Join(t.TempDir(), "r"),
		})
		// 解密依赖 IsEncrypted 标记，硬删后带标记插入
		require.NoError(t, db.Unscoped().Where("config_key = ?", "gitee_token").Delete(&models.SystemConfig{}).Error)
		require.NoError(t, db.Create(&models.SystemConfig{
			ConfigKey: "gitee_token", ConfigValue: encVal, IsEncrypted: true,
		}).Error)
		svc2 := NewGitService(enc)
		_, _, token, _, _, _, err := svc2.getCurrentGitConfig()
		require.NoError(t, err)
		assert.Equal(t, "secret-token", token)
	})

	t.Run("gitee各配置缺失分别报错", func(t *testing.T) {
		keys := []string{"gitee_repo_url", "gitee_username", "gitee_token", "gitee_email", "gitee_local_path"}
		for _, miss := range keys {
			kv := map[string]string{}
			for _, k := range keys {
				if k != miss {
					kv[k] = "v"
				}
			}
			db2 := newSvcTestDB(t)
			withGlobalDB(t, db2)
			gitSvcCfg(t, db2, kv)
			_, _, _, _, _, _, err := svc.getCurrentGitConfig()
			require.Error(t, err, "缺少 %s 应报错", miss)
			assert.Contains(t, err.Error(), "配置失败")
		}
		withGlobalDB(t, db)
	})

	t.Run("github各配置缺失分别报错", func(t *testing.T) {
		keys := []string{"github_repo_url", "github_username", "github_token", "github_local_path"}
		for _, miss := range keys {
			kv := map[string]string{"git_repository_type": "github"}
			for _, k := range keys {
				if k != miss {
					kv[k] = "v"
				}
			}
			db2 := newSvcTestDB(t)
			withGlobalDB(t, db2)
			gitSvcCfg(t, db2, kv)
			_, _, _, _, _, _, err := svc.getCurrentGitConfig()
			require.Error(t, err, "缺少 %s 应报错", miss)
			assert.Contains(t, err.Error(), "配置失败")
		}
		withGlobalDB(t, db)
	})
}

// --------------------------------------------------------------------
// InitRepository
// --------------------------------------------------------------------

func TestGitService_InitRepository(t *testing.T) {
	gitSvcSkip(t)

	t.Run("Token认证克隆成功", func(t *testing.T) {
		svc, db, _ := newGitSvcEnv(t)
		origin := gitSvcOriginMain(t)
		local := filepath.Join(t.TempDir(), "repo")
		gitSvcGiteeCfg(t, db, origin, local)

		require.NoError(t, svc.InitRepository())
		assert.Equal(t, local, svc.repoPath)
		assert.NotNil(t, svc.repo)
		assert.DirExists(t, filepath.Join(local, ".git"))
		// 克隆得到 origin 的 images.txt
		data, err := os.ReadFile(filepath.Join(local, "images.txt"))
		require.NoError(t, err)
		assert.Equal(t, "old-image:1", strings.TrimSpace(string(data)))
	})

	t.Run("用户名密码认证克隆成功", func(t *testing.T) {
		svc, db, _ := newGitSvcEnv(t)
		origin := gitSvcOriginMain(t)
		local := filepath.Join(t.TempDir(), "repo")
		gitSvcGiteeCfg(t, db, origin, local)
		// token 行存在但值为空 -> 走密码认证分支（本地 file origin 忽略凭证）
		gitSvcCfg(t, db, map[string]string{"gitee_token": "", "gitee_password": "pw-123"})

		require.NoError(t, svc.InitRepository())
		assert.NotNil(t, svc.repo)
	})

	t.Run("已存在仓库直接打开", func(t *testing.T) {
		svc, db, _ := newGitSvcEnv(t)
		origin := gitSvcOriginMain(t)
		local := filepath.Join(t.TempDir(), "repo")
		gitSvcRun(t, "", "clone", origin, local)
		gitSvcGiteeCfg(t, db, origin, local)

		require.NoError(t, svc.InitRepository())
		assert.NotNil(t, svc.repo)
		// 不会重新克隆：工作区文件保留（无重建过程，仅断言 .git 存在且打开成功）
		assert.DirExists(t, filepath.Join(local, ".git"))
	})

	t.Run("已存在目录但.git损坏打开失败", func(t *testing.T) {
		svc, db, _ := newGitSvcEnv(t)
		local := filepath.Join(t.TempDir(), "repo")
		require.NoError(t, os.MkdirAll(local, 0o755))
		// .git 是普通文件而非目录：os.Stat 成功但 PlainOpen 失败
		require.NoError(t, os.WriteFile(filepath.Join(local, ".git"), []byte("fake"), 0o644))
		gitSvcGiteeCfg(t, db, gitSvcOriginMain(t), local)

		err := svc.InitRepository()
		require.Error(t, err)
		assert.Contains(t, err.Error(), "打开现有仓库失败")
	})

	t.Run("配置缺失", func(t *testing.T) {
		svc, _, _ := newGitSvcEnv(t)
		err := svc.InitRepository()
		require.Error(t, err)
		assert.Contains(t, err.Error(), "获取Git配置失败")
	})

	t.Run("GitHub缺少Token", func(t *testing.T) {
		svc, db, _ := newGitSvcEnv(t)
		gitSvcCfg(t, db, map[string]string{
			"git_repository_type": "github",
			"github_repo_url":     gitSvcOriginMain(t),
			"github_username":     "u",
			"github_local_path":   filepath.Join(t.TempDir(), "r"),
		})
		err := svc.InitRepository()
		require.Error(t, err)
		// token 行缺失：配置读取阶段即报错
		assert.Contains(t, err.Error(), "获取GitHub令牌配置失败")

		// token 行存在但值为空 -> 到达令牌校验分支
		gitSvcCfg(t, db, map[string]string{"github_token": ""})
		err = svc.InitRepository()
		require.Error(t, err)
		assert.Contains(t, err.Error(), "GitHub仓库必须配置访问令牌")
	})

	t.Run("Gitee密码缺失", func(t *testing.T) {
		svc, db, _ := newGitSvcEnv(t)
		local := filepath.Join(t.TempDir(), "r")
		gitSvcCfg(t, db, map[string]string{
			"git_repository_type": "gitee",
			"gitee_repo_url":      gitSvcOriginMain(t),
			"gitee_username":      "u",
			"gitee_email":         "e",
			"gitee_local_path":    local,
			"gitee_token":         "", // 行存在但值为空 -> 密码分支，且无密码
		})
		err := svc.InitRepository()
		require.Error(t, err)
		assert.Contains(t, err.Error(), "获取Gitee密码失败")
	})

	t.Run("URL解析失败", func(t *testing.T) {
		svc, db, _ := newGitSvcEnv(t)
		gitSvcGiteeCfg(t, db, "://bad-url", filepath.Join(t.TempDir(), "r"))
		err := svc.InitRepository()
		require.Error(t, err)
		assert.Contains(t, err.Error(), "解析Git仓库URL失败")
	})

	t.Run("父目录创建失败", func(t *testing.T) {
		svc, db, _ := newGitSvcEnv(t)
		blocker := filepath.Join(t.TempDir(), "blocker")
		require.NoError(t, os.WriteFile(blocker, []byte("x"), 0o644))
		gitSvcGiteeCfg(t, db, gitSvcOriginMain(t), filepath.Join(blocker, "repo"))

		err := svc.InitRepository()
		require.Error(t, err)
		assert.Contains(t, err.Error(), "创建目录失败")
	})

	t.Run("克隆失败_远端不存在", func(t *testing.T) {
		svc, db, _ := newGitSvcEnv(t)
		missing := filepath.Join(t.TempDir(), "not-exist.git")
		gitSvcGiteeCfg(t, db, missing, filepath.Join(t.TempDir(), "r"))
		err := svc.InitRepository()
		require.Error(t, err)
		assert.Contains(t, err.Error(), "克隆仓库失败")
	})
}

// --------------------------------------------------------------------
// UpdateImagesFile e2e
// --------------------------------------------------------------------

func TestGitService_UpdateImagesFile_E2E(t *testing.T) {
	gitSvcSkip(t)

	t.Run("gitee全流程成功_历史注释保留", func(t *testing.T) {
		svc, db, _ := newGitSvcEnv(t)
		origin := gitSvcOriginMain(t)
		local := filepath.Join(t.TempDir(), "repo")
		gitSvcGiteeCfg(t, db, origin, local)

		sha, err := svc.UpdateImagesFile(context.Background(), []string{"nginx:1.25", "redis:7"})
		require.NoError(t, err)
		assert.NotEmpty(t, sha)

		content := gitSvcOriginFileContent(t, origin, "main", "images.txt")
		lines := strings.Split(strings.ReplaceAll(content, "\r\n", "\n"), "\n")
		assert.Equal(t, "nginx:1.25", lines[0])
		assert.Equal(t, "redis:7", lines[1])
		assert.Contains(t, content, "# 历史镜像记录")
		assert.Contains(t, content, "# old-image:1")

		// origin 提交信息
		logMsg := gitSvcRun(t, origin, "log", "-1", "--pretty=%s", "main")
		assert.Contains(t, logMsg, "Add 2 new images")
	})

	t.Run("二次更新_增量追加", func(t *testing.T) {
		svc, db, _ := newGitSvcEnv(t)
		origin := gitSvcOriginMain(t)
		local := filepath.Join(t.TempDir(), "repo")
		gitSvcGiteeCfg(t, db, origin, local)

		_, err := svc.UpdateImagesFile(context.Background(), []string{"a:1"})
		require.NoError(t, err)
		_, err = svc.UpdateImagesFile(context.Background(), []string{"b:2"})
		require.NoError(t, err)

		content := gitSvcOriginFileContent(t, origin, "main", "images.txt")
		assert.Contains(t, content, "b:2")
		assert.Contains(t, content, "# a:1")
	})

	t.Run("远端先行提交_拉取合并后成功", func(t *testing.T) {
		svc, db, _ := newGitSvcEnv(t)
		origin := gitSvcOriginMain(t)
		local := filepath.Join(t.TempDir(), "repo")
		gitSvcGiteeCfg(t, db, origin, local)
		repo, _ := gitSvcOpen(t, origin, local)
		svc.repo, svc.repoPath = repo, local

		// 另一个克隆推送新提交，使本地落后
		sibling := filepath.Join(t.TempDir(), "sibling")
		gitSvcRun(t, "", "clone", origin, sibling)
		require.NoError(t, os.WriteFile(filepath.Join(sibling, "README.md"), []byte("advance\n"), 0o644))
		gitSvcRun(t, sibling, "add", ".")
		gitSvcRun(t, sibling, "-c", "user.name=t", "-c", "user.email=t@t.io", "commit", "-m", "advance")
		gitSvcRun(t, sibling, "push", "origin", "main:main")

		sha, err := svc.UpdateImagesFile(context.Background(), []string{"c:3"})
		require.NoError(t, err)
		assert.NotEmpty(t, sha)
		content := gitSvcOriginFileContent(t, origin, "main", "images.txt")
		assert.Contains(t, content, "c:3")
	})

	t.Run("GitHub配置全流程成功_默认noreply邮箱", func(t *testing.T) {
		svc, db, _ := newGitSvcEnv(t)
		origin := gitSvcOriginMain(t)
		local := filepath.Join(t.TempDir(), "repo")
		gitSvcGitHubCfg(t, db, origin, local)

		sha, err := svc.UpdateImagesFile(context.Background(), []string{"gh:1"})
		require.NoError(t, err)
		assert.NotEmpty(t, sha)

		// GitHub 未配置 email 时使用 username@users.noreply.github.com
		email := gitSvcRun(t, origin, "log", "-1", "--pretty=%ae", "main")
		assert.Equal(t, "tester@users.noreply.github.com", strings.TrimSpace(email))
	})

	t.Run("origin无images文件_空内容起步", func(t *testing.T) {
		svc, db, _ := newGitSvcEnv(t)
		origin := gitSvcOrigin(t, map[string]string{"README.md": "hi\n"}, "main")
		local := filepath.Join(t.TempDir(), "repo")
		gitSvcGiteeCfg(t, db, origin, local)

		sha, err := svc.UpdateImagesFile(context.Background(), []string{"first:1"})
		require.NoError(t, err)
		assert.NotEmpty(t, sha)
		content := gitSvcOriginFileContent(t, origin, "main", "images.txt")
		assert.Contains(t, content, "first:1")
	})
}

func TestGitService_UpdateImagesFile_FailureAndRecovery(t *testing.T) {
	gitSvcSkip(t)

	t.Run("远程损坏_清理重建后成功", func(t *testing.T) {
		svc, db, _ := newGitSvcEnv(t)
		origin := gitSvcOriginMain(t)
		local := filepath.Join(t.TempDir(), "repo")
		repo, _ := gitSvcOpen(t, origin, local)
		gitSvcGiteeCfg(t, db, origin, local)
		svc.repo, svc.repoPath = repo, local
		// 本地 remote 指向坏地址 → pullLatest 失败 → CleanRepository+重新克隆（用配置里的正确 origin）
		gitSvcRun(t, local, "remote", "set-url", "origin", filepath.Join(t.TempDir(), "gone.git"))

		sha, err := svc.UpdateImagesFile(context.Background(), []string{"recovered:1"})
		require.NoError(t, err)
		assert.NotEmpty(t, sha)
		content := gitSvcOriginFileContent(t, origin, "main", "images.txt")
		assert.Contains(t, content, "recovered:1")
	})

	t.Run("重建失败_重新初始化报错", func(t *testing.T) {
		svc, db, _ := newGitSvcEnv(t)
		// 本地为空目录且 origin 不存在 -> 重克隆必然失败
		local := filepath.Join(t.TempDir(), "empty-repo")
		require.NoError(t, os.MkdirAll(local, 0o755))
		missing := filepath.Join(t.TempDir(), "missing-origin.git")
		gitSvcGiteeCfg(t, db, missing, local)
		svc.repo, svc.repoPath = nil, local

		_, err := svc.UpdateImagesFile(context.Background(), []string{"x:1"})
		require.Error(t, err)
		assert.Contains(t, err.Error(), "克隆仓库失败")
	})

	t.Run("仓库未初始化且配置缺失", func(t *testing.T) {
		svc, _, _ := newGitSvcEnv(t)
		_, err := svc.UpdateImagesFile(context.Background(), []string{"x:1"})
		require.Error(t, err)
		assert.Contains(t, err.Error(), "获取Git配置失败")
	})

	t.Run("写入文件失败", func(t *testing.T) {
		svc, db, _ := newGitSvcEnv(t)
		origin := gitSvcOriginMain(t)
		local := filepath.Join(t.TempDir(), "repo")
		repo, _ := gitSvcOpen(t, origin, local)
		gitSvcGiteeCfg(t, db, origin, local)
		svc.repo = repo
		// repoPath 指向普通文件下的子路径 → os.Create 失败
		blocker := filepath.Join(t.TempDir(), "blocker")
		require.NoError(t, os.WriteFile(blocker, []byte("x"), 0o644))
		svc.repoPath = filepath.Join(blocker, "sub")

		_, err := svc.UpdateImagesFile(context.Background(), []string{"x:1"})
		require.Error(t, err)
		assert.Contains(t, err.Error(), "写入images.txt失败")
	})

	t.Run("暂存Add失败", func(t *testing.T) {
		svc, db, _ := newGitSvcEnv(t)
		// origin 无 images.txt，克隆工作区自然没有该文件 → worktree.Add 失败
		origin := gitSvcOrigin(t, map[string]string{"README.md": "x\n"}, "main")
		local := filepath.Join(t.TempDir(), "repo")
		repo, _ := gitSvcOpen(t, origin, local)
		gitSvcGiteeCfg(t, db, origin, local)
		svc.repo = repo
		// repoPath 指向可写的其他目录（writeImagesFile 成功，但工作区里没有 images.txt）
		svc.repoPath = t.TempDir()

		_, err := svc.UpdateImagesFile(context.Background(), []string{"x:1"})
		require.Error(t, err)
		assert.Contains(t, err.Error(), "提交更改失败")
	})
}

// --------------------------------------------------------------------
// readImagesFile / writeImagesFile
// --------------------------------------------------------------------

func TestGitService_ReadWriteImagesFile(t *testing.T) {
	svc, _, _ := newGitSvcEnv(t)

	t.Run("读写往返", func(t *testing.T) {
		path := filepath.Join(t.TempDir(), "images.txt")
		require.NoError(t, svc.writeImagesFile(path, []string{"a:1", "", "# 注释"}))
		lines, err := svc.readImagesFile(path)
		require.NoError(t, err)
		assert.Equal(t, []string{"a:1", "", "# 注释"}, lines)
	})

	t.Run("读取不存在文件报错", func(t *testing.T) {
		_, err := svc.readImagesFile(filepath.Join(t.TempDir(), "nope.txt"))
		require.Error(t, err)
	})

	t.Run("写入失败_父路径是文件", func(t *testing.T) {
		blocker := filepath.Join(t.TempDir(), "blocker")
		require.NoError(t, os.WriteFile(blocker, []byte("x"), 0o644))
		err := svc.writeImagesFile(filepath.Join(blocker, "sub", "images.txt"), []string{"a"})
		require.Error(t, err)
	})
}

// --------------------------------------------------------------------
// pullLatest / forcePullLatest
// --------------------------------------------------------------------

func TestGitService_PullLatest(t *testing.T) {
	gitSvcSkip(t)

	t.Run("已是最新", func(t *testing.T) {
		svc, db, _ := newGitSvcEnv(t)
		origin := gitSvcOriginMain(t)
		local := filepath.Join(t.TempDir(), "repo")
		repo, _ := gitSvcOpen(t, origin, local)
		gitSvcGiteeCfg(t, db, origin, local)
		svc.repo, svc.repoPath = repo, local
		require.NoError(t, svc.pullLatest())
	})

	t.Run("落后时快进拉取", func(t *testing.T) {
		svc, db, _ := newGitSvcEnv(t)
		origin := gitSvcOriginMain(t)
		local := filepath.Join(t.TempDir(), "repo")
		repo, _ := gitSvcOpen(t, origin, local)
		gitSvcGiteeCfg(t, db, origin, local)
		svc.repo, svc.repoPath = repo, local

		sibling := filepath.Join(t.TempDir(), "sibling")
		gitSvcRun(t, "", "clone", origin, sibling)
		require.NoError(t, os.WriteFile(filepath.Join(sibling, "README.md"), []byte("v2\n"), 0o644))
		gitSvcRun(t, sibling, "add", ".")
		gitSvcRun(t, sibling, "-c", "user.name=t", "-c", "user.email=t@t.io", "commit", "-m", "v2")
		gitSvcRun(t, sibling, "push", "origin", "main:main")

		require.NoError(t, svc.pullLatest())
		data, err := os.ReadFile(filepath.Join(local, "README.md"))
		require.NoError(t, err)
		assert.Equal(t, "v2", strings.TrimSpace(string(data)))
	})

	t.Run("分叉时强制同步_丢弃本地提交", func(t *testing.T) {
		svc, db, _ := newGitSvcEnv(t)
		origin := gitSvcOriginMain(t)
		local := filepath.Join(t.TempDir(), "repo")
		repo, _ := gitSvcOpen(t, origin, local)
		gitSvcGiteeCfg(t, db, origin, local)
		svc.repo, svc.repoPath = repo, local

		// 远端推进
		sibling := filepath.Join(t.TempDir(), "sibling")
		gitSvcRun(t, "", "clone", origin, sibling)
		require.NoError(t, os.WriteFile(filepath.Join(sibling, "README.md"), []byte("remote-win\n"), 0o644))
		gitSvcRun(t, sibling, "add", ".")
		gitSvcRun(t, sibling, "-c", "user.name=t", "-c", "user.email=t@t.io", "commit", "-m", "remote")
		gitSvcRun(t, sibling, "push", "origin", "main:main")
		// 本地提交 → 分叉
		require.NoError(t, os.WriteFile(filepath.Join(local, "images.txt"), []byte("local-change\n"), 0o644))
		gitSvcRun(t, local, "add", ".")
		gitSvcRun(t, local, "-c", "user.name=t", "-c", "user.email=t@t.io", "commit", "-m", "local")

		require.NoError(t, svc.pullLatest()) // 普通 Pull 失败 → forcePullLatest 硬重置
		remoteTip := gitSvcRun(t, origin, "rev-parse", "main")
		localTip := gitSvcRun(t, local, "rev-parse", "HEAD")
		assert.Equal(t, strings.TrimSpace(remoteTip), strings.TrimSpace(localTip))
	})

	t.Run("用户名密码认证拉取", func(t *testing.T) {
		svc, db, _ := newGitSvcEnv(t)
		origin := gitSvcOriginMain(t)
		local := filepath.Join(t.TempDir(), "repo")
		repo, _ := gitSvcOpen(t, origin, local)
		gitSvcGiteeCfg(t, db, origin, local)
		// token 行存在但值为空 -> 密码认证分支
		gitSvcCfg(t, db, map[string]string{"gitee_token": "", "gitee_password": "pw-123"})
		svc.repo, svc.repoPath = repo, local
		require.NoError(t, svc.pullLatest())
	})

	t.Run("裸仓库worktree失败", func(t *testing.T) {
		svc, db, _ := newGitSvcEnv(t)
		barePath := filepath.Join(t.TempDir(), "bare.git")
		bare, err := git.PlainInit(barePath, true)
		require.NoError(t, err)
		gitSvcGiteeCfg(t, db, gitSvcOriginMain(t), filepath.Join(t.TempDir(), "r"))
		svc.repo = bare
		err = svc.pullLatest()
		require.Error(t, err)
		assert.Contains(t, err.Error(), "bare") // ErrIsBareRepository
	})

	t.Run("配置失败", func(t *testing.T) {
		svc, db, _ := newGitSvcEnv(t)
		origin := gitSvcOriginMain(t)
		local := filepath.Join(t.TempDir(), "repo")
		repo, _ := gitSvcOpen(t, origin, local)
		gitSvcGiteeCfg(t, db, origin, local)
		require.NoError(t, db.Exec("DELETE FROM system_configs").Error)
		svc.repo = repo
		err := svc.pullLatest()
		require.Error(t, err)
		assert.Contains(t, err.Error(), "获取Git配置失败")
	})

	t.Run("GitHub无Token", func(t *testing.T) {
		svc, db, _ := newGitSvcEnv(t)
		origin := gitSvcOriginMain(t)
		local := filepath.Join(t.TempDir(), "repo")
		repo, _ := gitSvcOpen(t, origin, local)
		gitSvcCfg(t, db, map[string]string{
			"git_repository_type": "github",
			"github_repo_url":     origin,
			"github_username":     "u",
			"github_local_path":   local,
			"github_token":        "",
		})
		svc.repo = repo
		err := svc.pullLatest()
		require.Error(t, err)
		assert.Contains(t, err.Error(), "GitHub仓库必须配置访问令牌")
	})

	t.Run("Gitee密码缺失", func(t *testing.T) {
		svc, db, _ := newGitSvcEnv(t)
		origin := gitSvcOriginMain(t)
		local := filepath.Join(t.TempDir(), "repo")
		repo, _ := gitSvcOpen(t, origin, local)
		gitSvcCfg(t, db, map[string]string{
			"git_repository_type": "gitee",
			"gitee_repo_url":      origin,
			"gitee_username":      "u",
			"gitee_email":         "e",
			"gitee_local_path":    local,
			"gitee_token":         "",
		})
		svc.repo = repo
		err := svc.pullLatest()
		require.Error(t, err)
		assert.Contains(t, err.Error(), "获取Gitee密码失败")
	})
}

func TestGitService_ForcePullLatest(t *testing.T) {
	gitSvcSkip(t)

	t.Run("分叉强制重置成功", func(t *testing.T) {
		svc, db, _ := newGitSvcEnv(t)
		origin := gitSvcOriginMain(t)
		local := filepath.Join(t.TempDir(), "repo")

		// 先推远端新提交，再克隆出落后的本地、取新句柄，
		// 规避 go-git file 传输的 "reference has changed concurrently" 竞态
		sibling := filepath.Join(t.TempDir(), "sibling")
		gitSvcRun(t, "", "clone", origin, sibling)
		require.NoError(t, os.WriteFile(filepath.Join(sibling, "README.md"), []byte("forced\n"), 0o644))
		gitSvcRun(t, sibling, "add", ".")
		gitSvcRun(t, sibling, "-c", "user.name=t", "-c", "user.email=t@t.io", "commit", "-m", "remote")
		gitSvcRun(t, sibling, "push", "origin", "main:main")

		gitSvcRun(t, "", "clone", origin, local)
		repo, err := git.PlainOpen(local)
		require.NoError(t, err)
		gitSvcGiteeCfg(t, db, origin, local)
		svc.repo, svc.repoPath = repo, local

		require.NoError(t, svc.forcePullLatest())
	})

	t.Run("无origin远程", func(t *testing.T) {
		svc, db, _ := newGitSvcEnv(t)
		origin := gitSvcOriginMain(t)
		local := filepath.Join(t.TempDir(), "repo")
		repo, _ := gitSvcOpen(t, origin, local)
		gitSvcGiteeCfg(t, db, origin, local)
		svc.repo = repo
		gitSvcRun(t, local, "remote", "remove", "origin")

		err := svc.forcePullLatest()
		require.Error(t, err)
		assert.Contains(t, err.Error(), "获取远程仓库失败")
	})

	t.Run("配置失败", func(t *testing.T) {
		svc, db, _ := newGitSvcEnv(t)
		origin := gitSvcOriginMain(t)
		local := filepath.Join(t.TempDir(), "repo")
		repo, _ := gitSvcOpen(t, origin, local)
		gitSvcGiteeCfg(t, db, origin, local)
		require.NoError(t, db.Exec("DELETE FROM system_configs").Error)
		svc.repo = repo
		err := svc.forcePullLatest()
		require.Error(t, err)
		assert.Contains(t, err.Error(), "获取Git配置失败")
	})

	t.Run("fetch失败_远端不存在", func(t *testing.T) {
		svc, db, _ := newGitSvcEnv(t)
		origin := gitSvcOriginMain(t)
		local := filepath.Join(t.TempDir(), "repo")
		repo, _ := gitSvcOpen(t, origin, local)
		gitSvcGiteeCfg(t, db, origin, local)
		svc.repo = repo
		gitSvcRun(t, local, "remote", "set-url", "origin", filepath.Join(t.TempDir(), "gone.git"))

		err := svc.forcePullLatest()
		require.Error(t, err)
		assert.Contains(t, err.Error(), "获取远程引用失败")
	})

	t.Run("远端无main分支", func(t *testing.T) {
		svc, db, _ := newGitSvcEnv(t)
		// origin 只有 master 分支
		origin := gitSvcOrigin(t, map[string]string{"images.txt": "x\n"}, "master")
		local := filepath.Join(t.TempDir(), "repo")
		repo, _ := gitSvcOpen(t, origin, local)
		gitSvcGiteeCfg(t, db, origin, local)
		svc.repo = repo

		err := svc.forcePullLatest()
		require.Error(t, err)
		assert.Contains(t, err.Error(), "获取远程main分支失败")
	})

	t.Run("裸仓库无worktree", func(t *testing.T) {
		svc, db, _ := newGitSvcEnv(t)
		origin := gitSvcOriginMain(t)
		barePath := filepath.Join(t.TempDir(), "bare.git")
		bare, err := git.PlainInit(barePath, true)
		require.NoError(t, err)
		gitSvcGiteeCfg(t, db, origin, filepath.Join(t.TempDir(), "r"))
		svc.repo = bare
		_, err = bare.CreateRemote(&config.RemoteConfig{Name: "origin", URLs: []string{origin}})
		require.NoError(t, err)

		err = svc.forcePullLatest()
		require.Error(t, err)
		assert.Contains(t, err.Error(), "bare")
	})
}

// --------------------------------------------------------------------
// commitAndPush / pushWithRetry
// --------------------------------------------------------------------

func TestGitService_CommitAndPush(t *testing.T) {
	gitSvcSkip(t)

	t.Run("成功提交并推送", func(t *testing.T) {
		svc, db, _ := newGitSvcEnv(t)
		origin := gitSvcOriginMain(t)
		local := filepath.Join(t.TempDir(), "repo")
		repo, _ := gitSvcOpen(t, origin, local)
		gitSvcGiteeCfg(t, db, origin, local)
		svc.repo, svc.repoPath = repo, local

		require.NoError(t, os.WriteFile(filepath.Join(local, "images.txt"), []byte("pushed:1\n"), 0o644))
		sha, err := svc.commitAndPush([]string{"pushed:1"})
		require.NoError(t, err)
		assert.NotEmpty(t, sha)
		content := gitSvcOriginFileContent(t, origin, "main", "images.txt")
		assert.Contains(t, content, "pushed:1")
	})

	t.Run("无更改返回当前HEAD", func(t *testing.T) {
		svc, db, _ := newGitSvcEnv(t)
		origin := gitSvcOriginMain(t)
		local := filepath.Join(t.TempDir(), "repo")
		repo, _ := gitSvcOpen(t, origin, local)
		gitSvcGiteeCfg(t, db, origin, local)
		svc.repo, svc.repoPath = repo, local

		sha, err := svc.commitAndPush([]string{"x"})
		require.NoError(t, err)
		want := strings.TrimSpace(gitSvcRun(t, local, "rev-parse", "HEAD"))
		assert.Equal(t, want, sha)
	})

	t.Run("裸仓库worktree失败", func(t *testing.T) {
		svc, db, _ := newGitSvcEnv(t)
		bare, err := git.PlainInit(filepath.Join(t.TempDir(), "bare.git"), true)
		require.NoError(t, err)
		gitSvcGiteeCfg(t, db, gitSvcOriginMain(t), filepath.Join(t.TempDir(), "r"))
		svc.repo = bare
		_, err = svc.commitAndPush([]string{"x"})
		require.Error(t, err)
	})

	t.Run("工作区无images文件Add失败", func(t *testing.T) {
		svc, db, _ := newGitSvcEnv(t)
		origin := gitSvcOrigin(t, map[string]string{"README.md": "x\n"}, "main")
		local := filepath.Join(t.TempDir(), "repo")
		repo, _ := gitSvcOpen(t, origin, local)
		gitSvcGiteeCfg(t, db, origin, local)
		svc.repo = repo
		_, err := svc.commitAndPush([]string{"x"})
		require.Error(t, err)
	})

	t.Run("配置失败", func(t *testing.T) {
		svc, db, _ := newGitSvcEnv(t)
		origin := gitSvcOriginMain(t)
		local := filepath.Join(t.TempDir(), "repo")
		repo, _ := gitSvcOpen(t, origin, local)
		gitSvcGiteeCfg(t, db, origin, local)
		require.NoError(t, db.Exec("DELETE FROM system_configs").Error)
		svc.repo, svc.repoPath = repo, local
		// 制造未提交更改，越过 IsClean 分支后在读取配置处失败
		require.NoError(t, os.WriteFile(filepath.Join(local, "images.txt"), []byte("dirty\n"), 0o644))

		_, err := svc.commitAndPush([]string{"x"})
		require.Error(t, err)
		assert.Contains(t, err.Error(), "获取Git配置失败")
	})
}

func TestGitService_PushWithRetry(t *testing.T) {
	gitSvcSkip(t)

	t.Run("直接推送成功", func(t *testing.T) {
		svc, db, _ := newGitSvcEnv(t)
		origin := gitSvcOriginMain(t)
		local := filepath.Join(t.TempDir(), "repo")
		repo, _ := gitSvcOpen(t, origin, local)
		gitSvcGiteeCfg(t, db, origin, local)
		svc.repo = repo
		// 本地先提交一个 commit 使 push 有内容
		require.NoError(t, os.WriteFile(filepath.Join(local, "README.md"), []byte("x\n"), 0o644))
		gitSvcRun(t, local, "add", ".")
		gitSvcRun(t, local, "-c", "user.name=t", "-c", "user.email=t@t.io", "commit", "-m", "plain")
		require.NoError(t, svc.pushWithRetry("sha-x"))
	})

	t.Run("配置失败", func(t *testing.T) {
		svc, _, _ := newGitSvcEnv(t)
		bare, err := git.PlainInit(filepath.Join(t.TempDir(), "bare.git"), true)
		require.NoError(t, err)
		svc.repo = bare
		err = svc.pushWithRetry("sha-x")
		require.Error(t, err)
		assert.Contains(t, err.Error(), "获取Git配置失败")
	})

	t.Run("推送被拒_重拉重提交后成功", func(t *testing.T) {
		svc, db, _ := newGitSvcEnv(t)
		origin := gitSvcOriginMain(t)
		// pre-receive 钩子：仅放行 "Retry commit" 前缀的提交
		hooksDir := filepath.Join(origin, "hooks")
		require.NoError(t, os.MkdirAll(hooksDir, 0o755))
		script := "#!/bin/sh\nwhile read old new ref; do\n  msg=$(git log -1 --pretty=%s \"$new\")\n  case \"$msg\" in\n  \"Retry commit\"*) exit 0;;\n  *) echo rejected-by-hook; exit 1;;\n  esac\ndone\nexit 0\n"
		require.NoError(t, os.WriteFile(filepath.Join(hooksDir, "pre-receive"), []byte(script), 0o755))

		local := filepath.Join(t.TempDir(), "repo")
		repo, _ := gitSvcOpen(t, origin, local)
		gitSvcGiteeCfg(t, db, origin, local)
		svc.repo, svc.repoPath = repo, local

		// 本地提交普通消息 commit（会被钩子拒绝），并保持工作区有未提交修改（触发重提交分支）
		require.NoError(t, os.WriteFile(filepath.Join(local, "images.txt"), []byte("hook:1\n"), 0o644))
		gitSvcRun(t, local, "add", ".")
		gitSvcRun(t, local, "-c", "user.name=t", "-c", "user.email=t@t.io", "commit", "-m", "Add 1 new images")
		require.NoError(t, os.WriteFile(filepath.Join(local, "images.txt"), []byte("hook:1\ndirty\n"), 0o644))

		require.NoError(t, svc.pushWithRetry("sha-y"))
		last := gitSvcRun(t, origin, "log", "-1", "--pretty=%s", "main")
		assert.Contains(t, last, "Retry commit: sha-y")
	})

	t.Run("远端不可达_重试耗尽", func(t *testing.T) {
		svc, db, _ := newGitSvcEnv(t)
		origin := gitSvcOriginMain(t)
		local := filepath.Join(t.TempDir(), "repo")
		repo, _ := gitSvcOpen(t, origin, local)
		gitSvcGiteeCfg(t, db, origin, local)
		svc.repo = repo
		require.NoError(t, os.RemoveAll(origin)) // 删除远端：push/pull 均失败
		require.NoError(t, os.WriteFile(filepath.Join(local, "README.md"), []byte("x\n"), 0o644))
		gitSvcRun(t, local, "add", ".")
		gitSvcRun(t, local, "-c", "user.name=t", "-c", "user.email=t@t.io", "commit", "-m", "plain")

		err := svc.pushWithRetry("sha-z")
		require.Error(t, err)
		assert.Contains(t, err.Error(), "推送到Gitee失败")
		assert.Contains(t, err.Error(), "已重试3次")
	})
}

// --------------------------------------------------------------------
// CleanRepository
// --------------------------------------------------------------------

func TestGitService_CleanRepository(t *testing.T) {
	svc, _, _ := newGitSvcEnv(t)

	t.Run("空路径直接成功", func(t *testing.T) {
		require.NoError(t, svc.CleanRepository(context.Background()))
	})

	t.Run("删除仓库目录", func(t *testing.T) {
		dir := filepath.Join(t.TempDir(), "repo")
		require.NoError(t, os.MkdirAll(filepath.Join(dir, ".git"), 0o755))
		svc.repoPath = dir
		require.NoError(t, svc.CleanRepository(context.Background()))
		assert.NoDirExists(t, dir)
	})
}
