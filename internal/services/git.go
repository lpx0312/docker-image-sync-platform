package services

import (
	"bufio"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"docker-image-sync-platform/internal/config"
	"docker-image-sync-platform/internal/logger"

	"github.com/go-git/go-git/v5"
	"github.com/go-git/go-git/v5/plumbing/object"
	"github.com/go-git/go-git/v5/plumbing/transport/http"
	"go.uber.org/zap"
)

// GitService Git服务
type GitService struct {
	repoPath string
	repo     *git.Repository
}

// NewGitService 创建Git服务
func NewGitService() *GitService {
	return &GitService{
		repoPath: config.AppConfig.Git.LocalRepoPath,
	}
}

// InitRepository 初始化仓库
func (s *GitService) InitRepository() error {
	// 确保目录存在
	if err := os.MkdirAll(filepath.Dir(s.repoPath), 0755); err != nil {
		return fmt.Errorf("创建目录失败: %w", err)
	}

	// 检查仓库是否已存在
	if _, err := os.Stat(filepath.Join(s.repoPath, ".git")); err == nil {
		// 仓库已存在，打开它
		repo, err := git.PlainOpen(s.repoPath)
		if err != nil {
			return fmt.Errorf("打开现有仓库失败: %w", err)
		}
		s.repo = repo
		logger.Logger.Info("使用现有Git仓库", zap.String("path", s.repoPath))
		return nil
	}

	// 克隆仓库
	logger.Logger.Info("开始克隆Gitee仓库", zap.String("url", config.AppConfig.Git.Gitee.RepoURL))
	
	repo, err := git.PlainClone(s.repoPath, false, &git.CloneOptions{
		URL: config.AppConfig.Git.Gitee.RepoURL,
		Auth: &http.BasicAuth{
			Username: config.AppConfig.Git.Gitee.Username,
			Password: config.AppConfig.Git.Gitee.Password,
		},
	})

	if err != nil {
		return fmt.Errorf("克隆仓库失败: %w", err)
	}

	s.repo = repo
	logger.Logger.Info("Git仓库克隆成功", zap.String("path", s.repoPath))
	return nil
}

// UpdateImagesFile 更新images.txt文件
func (s *GitService) UpdateImagesFile(newImages []string) (string, error) {
	if s.repo == nil {
		if err := s.InitRepository(); err != nil {
			return "", err
		}
	}

	// 拉取最新代码
	if err := s.pullLatest(); err != nil {
		logger.Logger.Warn("拉取最新代码失败", zap.Error(err))
	}

	imagesFilePath := filepath.Join(s.repoPath, "images.txt")
	
	// 读取现有的images.txt文件
	existingImages, err := s.readImagesFile(imagesFilePath)
	if err != nil {
		logger.Logger.Warn("读取现有images.txt失败", zap.Error(err))
		existingImages = []string{}
	}

	// 注释掉现有内容
	commentedImages := make([]string, len(existingImages))
	for i, image := range existingImages {
		if !strings.HasPrefix(image, "#") && strings.TrimSpace(image) != "" {
			commentedImages[i] = "# " + image
		} else {
			commentedImages[i] = image
		}
	}

	// 合并新镜像和注释的旧镜像
	var allImages []string
	allImages = append(allImages, newImages...)
	if len(commentedImages) > 0 {
		allImages = append(allImages, "")
		allImages = append(allImages, "# 历史镜像记录")
		allImages = append(allImages, commentedImages...)
	}

	// 写入文件
	if err := s.writeImagesFile(imagesFilePath, allImages); err != nil {
		return "", fmt.Errorf("写入images.txt失败: %w", err)
	}

	// 提交更改
	commitSHA, err := s.commitAndPush(newImages)
	if err != nil {
		return "", fmt.Errorf("提交更改失败: %w", err)
	}

	logger.Logger.Info("images.txt更新成功", 
		zap.String("commit_sha", commitSHA),
		zap.Int("new_images_count", len(newImages)))

	return commitSHA, nil
}

// readImagesFile 读取images.txt文件
func (s *GitService) readImagesFile(filePath string) ([]string, error) {
	file, err := os.Open(filePath)
	if err != nil {
		return nil, err
	}
	defer file.Close()

	var lines []string
	scanner := bufio.NewScanner(file)
	for scanner.Scan() {
		lines = append(lines, scanner.Text())
	}

	return lines, scanner.Err()
}

// writeImagesFile 写入images.txt文件
func (s *GitService) writeImagesFile(filePath string, images []string) error {
	file, err := os.Create(filePath)
	if err != nil {
		return err
	}
	defer file.Close()

	for _, image := range images {
		if _, err := file.WriteString(image + "\n"); err != nil {
			return err
		}
	}

	return nil
}

// pullLatest 拉取最新代码
func (s *GitService) pullLatest() error {
	worktree, err := s.repo.Worktree()
	if err != nil {
		return err
	}

	err = worktree.Pull(&git.PullOptions{
		Auth: &http.BasicAuth{
			Username: config.AppConfig.Git.Gitee.Username,
			Password: config.AppConfig.Git.Gitee.Password,
		},
	})

	if err != nil && err != git.NoErrAlreadyUpToDate {
		return err
	}

	return nil
}

// commitAndPush 提交并推送更改
func (s *GitService) commitAndPush(newImages []string) (string, error) {
	worktree, err := s.repo.Worktree()
	if err != nil {
		return "", err
	}

	// 添加文件到暂存区
	_, err = worktree.Add("images.txt")
	if err != nil {
		return "", err
	}

	// 检查是否有更改
	status, err := worktree.Status()
	if err != nil {
		return "", err
	}

	if status.IsClean() {
		logger.Logger.Info("没有文件更改，跳过提交")
		// 获取最新提交的SHA
		ref, err := s.repo.Head()
		if err != nil {
			return "", err
		}
		return ref.Hash().String(), nil
	}

	// 创建提交信息
	commitMsg := fmt.Sprintf("Add %d new images for sync\n\nImages:\n", len(newImages))
	for _, image := range newImages {
		commitMsg += fmt.Sprintf("- %s\n", image)
	}
	commitMsg += fmt.Sprintf("\nCommitted at: %s", time.Now().Format("2006-01-02 15:04:05"))

	// 提交更改
	commit, err := worktree.Commit(commitMsg, &git.CommitOptions{
		Author: &object.Signature{
			Name:  config.AppConfig.Git.Gitee.Username,
			Email: config.AppConfig.Git.Gitee.Email,
			When:  time.Now(),
		},
	})

	if err != nil {
		return "", err
	}

	// 推送到Gitee
	err = s.repo.Push(&git.PushOptions{
		Auth: &http.BasicAuth{
			Username: config.AppConfig.Git.Gitee.Username,
			Password: config.AppConfig.Git.Gitee.Password,
		},
	})

	if err != nil {
		return "", fmt.Errorf("推送到Gitee失败: %w", err)
	}

	logger.Logger.Info("代码已推送到Gitee", zap.String("commit", commit.String()))
	return commit.String(), nil
}

// GetRepoStatus 获取仓库状态
func (s *GitService) GetRepoStatus() (map[string]interface{}, error) {
	if s.repo == nil {
		return nil, fmt.Errorf("仓库未初始化")
	}

	worktree, err := s.repo.Worktree()
	if err != nil {
		return nil, err
	}

	status, err := worktree.Status()
	if err != nil {
		return nil, err
	}

	// 获取最新提交信息
	ref, err := s.repo.Head()
	if err != nil {
		return nil, err
	}

	commit, err := s.repo.CommitObject(ref.Hash())
	if err != nil {
		return nil, err
	}

	return map[string]interface{}{
		"is_clean":     status.IsClean(),
		"last_commit":  commit.Hash.String(),
		"last_message": commit.Message,
		"last_author":  commit.Author.Name,
		"last_time":    commit.Author.When,
		"repo_path":    s.repoPath,
	}, nil
}

// CleanRepository 清理仓库
func (s *GitService) CleanRepository() error {
	if s.repoPath != "" {
		return os.RemoveAll(s.repoPath)
	}
	return nil
}