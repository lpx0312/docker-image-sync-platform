// Package services 提供核心业务服务实现
//
// git.go 文件实现了Git版本控制相关的服务，主要负责：
// - Git仓库的初始化和管理
// - 镜像列表文件的版本控制
// - 与Gitee远程仓库的同步操作
// - 代码提交和推送的自动化处理
//
// Git服务功能：
// - 支持自动克隆和初始化Git仓库
// - 提供镜像列表文件的增量更新
// - 实现智能的冲突解决和重试机制
// - 支持历史记录的保留和注释
// - 提供仓库状态查询和清理功能
//
// 主要业务场景：
// - 镜像同步任务的版本控制
// - 镜像列表的持久化存储
// - 与GitHub Actions的集成触发
// - 同步历史的追踪和管理
//
// 作者: Docker镜像同步平台开发团队
// 版本: v1.0.0
package services

import (
	"bufio"
	"fmt"
	"net/url"
	"os"
	"os/exec"
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

// GitService Git版本控制服务
//
// 负责管理Docker镜像同步平台的Git仓库操作，包括：
// - 仓库的初始化、克隆和状态管理
// - 镜像列表文件(images.txt)的版本控制
// - 与Gitee远程仓库的同步和推送
// - 冲突解决和错误恢复机制
//
// 核心功能：
//   - 自动化的仓库管理和同步
//   - 智能的冲突解决策略
//   - 镜像列表的增量更新和历史保留
//   - 与CI/CD流水线的集成支持
type GitService struct {
	repoPath string          // 本地仓库路径
	repo     *git.Repository // Git仓库实例
}

// NewGitService 创建Git服务实例
//
// 功能说明:
//   - 初始化Git服务，配置本地仓库路径
//   - 从应用配置中读取Git相关设置
//   - 为后续的Git操作做准备
//
// 返回值:
//   - *GitService: Git服务实例，包含配置的仓库路径
//
// 使用场景:
//   - 系统启动时初始化Git服务
//   - 镜像同步任务需要版本控制时
//   - 需要与远程仓库交互时
func NewGitService() *GitService {
	return &GitService{
		repoPath: config.AppConfig.Git.LocalRepoPath,
	}
}

// InitRepository 初始化Git仓库
//
// 功能说明:
//   - 检查本地仓库是否存在，如存在则直接使用
//   - 如不存在则从Gitee克隆远程仓库
//   - 自动创建必要的目录结构
//   - 配置仓库的认证信息
//
// 返回值:
//   - error: 初始化过程中的错误，nil表示成功
//
// 错误处理:
//   - 目录创建失败
//   - 仓库克隆失败
//   - 仓库打开失败
//   - 网络连接问题
//
// 使用场景:
//   - 系统首次启动时
//   - 仓库损坏需要重新初始化时
//   - 切换到新的仓库地址时
func (s *GitService) InitRepository() error {
	// ====================================================================
	// 目录准备和检查
	// ====================================================================
	
	// 确保父目录存在，创建必要的目录结构
	if err := os.MkdirAll(filepath.Dir(s.repoPath), 0755); err != nil {
		return fmt.Errorf("创建目录失败: %w", err)
	}

	// ====================================================================
	// 检查现有仓库
	// ====================================================================
	
	// 检查仓库是否已存在（通过.git目录判断）
	if _, err := os.Stat(filepath.Join(s.repoPath, ".git")); err == nil {
		// 仓库已存在，直接打开并使用
		repo, err := git.PlainOpen(s.repoPath)
		if err != nil {
			return fmt.Errorf("打开现有仓库失败: %w", err)
		}
		s.repo = repo
		logger.Logger.Info("使用现有Git仓库", zap.String("path", s.repoPath))
		return nil
	}

	// ====================================================================
	// 克隆远程仓库
	// ====================================================================
	
	// 开始克隆Gitee仓库
	logger.Logger.Info("开始克隆Gitee仓库", zap.String("url", config.AppConfig.Git.Gitee.RepoURL))

	// 构建带有认证信息的URL
	// 优先使用Token认证，如果没有Token则使用用户名密码认证
	parsedURL, err := url.Parse(config.AppConfig.Git.Gitee.RepoURL)
	if err != nil {
		return fmt.Errorf("解析Gitee仓库URL失败: %w", err)
	}
	
	var authURL string
	if config.AppConfig.Git.Gitee.Token != "" {
		// 使用访问令牌认证 (推荐方式)
		// 格式: https://token@gitee.com/username/repo.git
		encodedToken := url.QueryEscape(config.AppConfig.Git.Gitee.Token)
		parsedURL.User = url.User(encodedToken)
		authURL = parsedURL.String()
		logger.Logger.Info("使用Gitee访问令牌进行认证")
	} else {
		// 使用用户名密码认证 (传统方式)
		encodedUsername := url.QueryEscape(config.AppConfig.Git.Gitee.Username)
		encodedPassword := url.QueryEscape(config.AppConfig.Git.Gitee.Password)
		parsedURL.User = url.UserPassword(encodedUsername, encodedPassword)
		authURL = parsedURL.String()
		logger.Logger.Info("使用Gitee用户名密码进行认证")
	}

	// 使用系统git命令执行克隆操作
	// 这种方式比go-git库更稳定，特别是对于大型仓库
	cmd := exec.Command("git", "clone", authURL, s.repoPath)
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	if err := cmd.Run(); err != nil {
		return fmt.Errorf("克隆仓库失败: %w", err)
	}

	// ====================================================================
	// 打开克隆的仓库
	// ====================================================================
	
	// 使用go-git库打开克隆的仓库以进行后续操作
	repo, err := git.PlainOpen(s.repoPath)
	if err != nil {
		return fmt.Errorf("打开克隆的仓库失败: %w", err)
	}

	s.repo = repo
	logger.Logger.Info("Git仓库克隆成功", zap.String("path", s.repoPath))
	return nil
}

// UpdateImagesFile 更新镜像列表文件
//
// 功能说明:
//   - 拉取远程仓库的最新代码
//   - 读取现有的images.txt文件内容
//   - 将旧内容注释化以保留历史记录
//   - 添加新的镜像列表到文件顶部
//   - 提交更改并推送到远程仓库
//
// 参数:
//   - newImages: 新增的镜像列表，每个元素为完整的镜像地址
//
// 返回值:
//   - string: 提交的SHA值，用于后续的CI/CD流水线跟踪
//   - error: 操作过程中的错误，nil表示成功
//
// 文件格式:
//   - 新镜像列表在文件顶部
//   - 历史镜像以注释形式保留
//   - 使用空行分隔不同批次的镜像
//
// 错误处理:
//   - 仓库未初始化时自动初始化
//   - 拉取失败时尝试重新初始化仓库
//   - 提交冲突时自动解决
//
// 使用场景:
//   - 镜像同步任务提交新的镜像列表
//   - 批量镜像同步的版本控制
//   - 触发GitHub Actions工作流
func (s *GitService) UpdateImagesFile(newImages []string) (string, error) {
	// ====================================================================
	// 仓库状态检查和初始化
	// ====================================================================
	
	// 确保仓库已初始化
	if s.repo == nil {
		if err := s.InitRepository(); err != nil {
			return "", err
		}
	}

	// ====================================================================
	// 同步远程代码
	// ====================================================================
	
	// 拉取最新代码以避免冲突
	if err := s.pullLatest(); err != nil {
		logger.Logger.Error("拉取最新代码失败，尝试重新初始化仓库", zap.Error(err))
		
		// 如果拉取失败，可能是仓库状态异常，尝试重新初始化
		if err := s.CleanRepository(); err != nil {
			logger.Logger.Warn("清理仓库失败", zap.Error(err))
		}
		
		if err := s.InitRepository(); err != nil {
			return "", fmt.Errorf("重新初始化仓库失败: %w", err)
		}
		
		logger.Logger.Info("仓库重新初始化成功")
	}

	// ====================================================================
	// 文件内容处理
	// ====================================================================
	
	imagesFilePath := filepath.Join(s.repoPath, "images.txt")

	// 读取现有的images.txt文件
	existingImages, err := s.readImagesFile(imagesFilePath)
	if err != nil {
		logger.Logger.Warn("读取现有images.txt失败", zap.Error(err))
		existingImages = []string{}
	}

	// 注释掉现有内容以保留历史记录
	// 这样可以追踪镜像同步的历史，便于问题排查
	commentedImages := make([]string, len(existingImages))
	for i, image := range existingImages {
		if !strings.HasPrefix(image, "#") && strings.TrimSpace(image) != "" {
			commentedImages[i] = "# " + image
		} else {
			commentedImages[i] = image
		}
	}

	// 合并新镜像和注释的旧镜像
	// 新镜像在顶部，历史镜像在底部
	var allImages []string
	allImages = append(allImages, newImages...)
	if len(commentedImages) > 0 {
		allImages = append(allImages, "")
		allImages = append(allImages, "# 历史镜像记录")
		allImages = append(allImages, commentedImages...)
	}

	// ====================================================================
	// 文件写入和提交
	// ====================================================================
	
	// 写入更新后的文件内容
	if err := s.writeImagesFile(imagesFilePath, allImages); err != nil {
		return "", fmt.Errorf("写入images.txt失败: %w", err)
	}

	// 提交更改并推送到远程仓库
	commitSHA, err := s.commitAndPush(newImages)
	if err != nil {
		return "", fmt.Errorf("提交更改失败: %w", err)
	}

	logger.Logger.Info("images.txt更新成功",
		zap.String("commit_sha", commitSHA),
		zap.Int("new_images_count", len(newImages)))

	return commitSHA, nil
}

// readImagesFile 读取镜像列表文件
//
// 功能说明:
//   - 打开并读取指定路径的images.txt文件
//   - 逐行解析文件内容
//   - 返回文件中的所有行内容
//
// 参数:
//   - filePath: 镜像文件的完整路径
//
// 返回值:
//   - []string: 文件中的所有行内容，包括空行和注释行
//   - error: 文件读取过程中的错误
//
// 错误处理:
//   - 文件不存在
//   - 文件权限不足
//   - 文件读取异常
//
// 使用场景:
//   - 更新镜像文件前读取现有内容
//   - 保留历史镜像记录
//   - 文件内容的版本控制
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

// writeImagesFile 写入镜像列表文件
//
// 功能说明:
//   - 创建或覆盖指定路径的images.txt文件
//   - 将镜像列表逐行写入文件
//   - 确保文件格式的一致性
//
// 参数:
//   - filePath: 目标文件的完整路径
//   - images: 要写入的镜像列表，每个元素为一行内容
//
// 返回值:
//   - error: 文件写入过程中的错误，nil表示成功
//
// 文件格式:
//   - 每行一个镜像地址或注释
//   - 使用Unix风格的换行符(\n)
//   - 支持空行和注释行
//
// 错误处理:
//   - 文件创建失败
//   - 磁盘空间不足
//   - 文件写入权限问题
//
// 使用场景:
//   - 更新镜像列表文件
//   - 保存合并后的镜像内容
//   - 版本控制前的文件准备
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

// pullLatest 拉取远程仓库的最新代码
//
// 功能说明:
//   - 从远程仓库拉取最新的代码更改
//   - 使用配置的认证信息进行安全连接
//   - 如果正常拉取失败，自动尝试强制同步
//   - 处理各种拉取场景和异常情况
//
// 返回值:
//   - error: 拉取过程中的错误，nil表示成功
//
// 拉取策略:
//   - 优先尝试正常的git pull操作
//   - 如果已是最新状态，直接返回成功
//   - 如果有冲突，尝试强制同步策略
//
// 错误处理:
//   - 网络连接问题
//   - 认证失败
//   - 合并冲突
//   - 仓库状态异常
//
// 使用场景:
//   - 更新文件前同步远程代码
//   - 避免提交时的冲突
//   - 保持本地仓库与远程同步
func (s *GitService) pullLatest() error {
	worktree, err := s.repo.Worktree()
	if err != nil {
		return err
	}

	// ====================================================================
	// 尝试正常拉取
	// ====================================================================
	
	// 首先尝试正常拉取，使用配置的认证信息
	var auth *http.BasicAuth
	if config.AppConfig.Git.Gitee.Token != "" {
		// 使用访问令牌认证
		auth = &http.BasicAuth{
			Username: config.AppConfig.Git.Gitee.Token,
			Password: "", // Token认证时密码为空
		}
	} else {
		// 使用用户名密码认证
		auth = &http.BasicAuth{
			Username: config.AppConfig.Git.Gitee.Username,
			Password: config.AppConfig.Git.Gitee.Password,
		}
	}
	
	err = worktree.Pull(&git.PullOptions{
		Auth: auth,
	})

	// 拉取成功或已是最新状态
	if err == nil || err == git.NoErrAlreadyUpToDate {
		return nil
	}

	// ====================================================================
	// 强制同步策略
	// ====================================================================
	
	logger.Logger.Warn("正常拉取失败，尝试强制同步", zap.Error(err))

	// 如果正常拉取失败，尝试强制同步
	// 这通常发生在有本地修改与远程冲突的情况下
	return s.forcePullLatest()
}

// forcePullLatest 强制拉取最新代码，解决冲突
//
// 功能说明:
//   - 获取远程仓库的最新引用
//   - 强制重置本地仓库到远程最新状态
//   - 丢弃本地的未提交更改
//   - 确保本地仓库与远程完全同步
//
// 返回值:
//   - error: 强制同步过程中的错误，nil表示成功
//
// 同步策略:
//   - 使用git fetch获取远程引用
//   - 使用hard reset强制重置到远程状态
//   - 丢弃所有本地未提交的更改
//
// 风险提示:
//   - 会丢失本地未提交的更改
//   - 适用于自动化场景，不适合手动开发
//
// 使用场景:
//   - 正常拉取失败时的备选方案
//   - 解决复杂的合并冲突
//   - 确保仓库状态的一致性
func (s *GitService) forcePullLatest() error {
	// ====================================================================
	// 获取远程引用
	// ====================================================================
	
	// 获取远程仓库的引用
	remote, err := s.repo.Remote("origin")
	if err != nil {
		return fmt.Errorf("获取远程仓库失败: %w", err)
	}

	// 获取远程的最新引用信息
	err = remote.Fetch(&git.FetchOptions{
		Auth: &http.BasicAuth{
			Username: config.AppConfig.Git.Gitee.Username,
			Password: config.AppConfig.Git.Gitee.Password,
		},
	})
	if err != nil && err != git.NoErrAlreadyUpToDate {
		return fmt.Errorf("获取远程引用失败: %w", err)
	}

	// ====================================================================
	// 强制重置到远程状态
	// ====================================================================
	
	// 获取远程main分支的最新提交
	remoteRef, err := s.repo.Reference("refs/remotes/origin/main", true)
	if err != nil {
		return fmt.Errorf("获取远程main分支失败: %w", err)
	}

	worktree, err := s.repo.Worktree()
	if err != nil {
		return err
	}

	// 强制重置到远程最新提交
	// 这会丢弃所有本地更改，确保与远程完全一致
	err = worktree.Reset(&git.ResetOptions{
		Commit: remoteRef.Hash(),
		Mode:   git.HardReset,
	})
	if err != nil {
		return fmt.Errorf("强制重置失败: %w", err)
	}

	logger.Logger.Info("强制同步成功", zap.String("commit", remoteRef.Hash().String()))
	return nil
}

// commitAndPush 提交并推送更改
//
// 功能说明:
//   - 将文件更改添加到Git暂存区
//   - 检查是否有实际的文件更改
//   - 创建包含详细信息的提交记录
//   - 推送提交到远程仓库
//   - 处理推送过程中的各种异常
//
// 参数:
//   - newImages: 新增的镜像列表，用于生成提交信息
//
// 返回值:
//   - string: 提交的SHA值，用于后续跟踪
//   - error: 提交推送过程中的错误
//
// 提交信息格式:
//   - 包含新增镜像的数量
//   - 列出所有新增的镜像地址
//   - 包含提交时间戳
//
// 错误处理:
//   - 文件添加到暂存区失败
//   - 提交创建失败
//   - 推送到远程仓库失败
//
// 使用场景:
//   - 镜像列表更新后的版本控制
//   - 触发CI/CD流水线
//   - 保留镜像同步的历史记录
func (s *GitService) commitAndPush(newImages []string) (string, error) {
	worktree, err := s.repo.Worktree()
	if err != nil {
		return "", err
	}

	// ====================================================================
	// 添加文件到暂存区
	// ====================================================================
	
	// 添加images.txt文件到Git暂存区
	_, err = worktree.Add("images.txt")
	if err != nil {
		return "", err
	}

	// ====================================================================
	// 检查文件更改状态
	// ====================================================================
	
	// 检查是否有实际的文件更改
	status, err := worktree.Status()
	if err != nil {
		return "", err
	}

	// 如果没有更改，跳过提交但返回最新的提交SHA
	if status.IsClean() {
		logger.Logger.Info("没有文件更改，跳过提交")
		// 获取最新提交的SHA
		ref, err := s.repo.Head()
		if err != nil {
			return "", err
		}
		return ref.Hash().String(), nil
	}

	// ====================================================================
	// 创建提交信息
	// ====================================================================
	
	// 创建详细的提交信息，包含镜像列表和时间戳
	commitMsg := fmt.Sprintf("Add %d new images for sync\n\nImages:\n", len(newImages))
	for _, image := range newImages {
		commitMsg += fmt.Sprintf("- %s\n", image)
	}
	commitMsg += fmt.Sprintf("\nCommitted at: %s", time.Now().Format("2006-01-02 15:04:05"))

	// ====================================================================
	// 执行提交操作
	// ====================================================================
	
	// 提交更改，使用配置的用户信息
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

	// ====================================================================
	// 推送到远程仓库
	// ====================================================================
	
	// 推送到Gitee，如果失败则重试
	err = s.pushWithRetry(commit.String())
	if err != nil {
		return "", err
	}

	logger.Logger.Info("代码已推送到Gitee", zap.String("commit", commit.String()))
	return commit.String(), nil
}

// pushWithRetry 推送代码，如果失败则重试
//
// 功能说明:
//   - 尝试推送代码到远程仓库
//   - 如果推送失败，自动重新同步并重试
//   - 支持多次重试机制，提高推送成功率
//   - 处理推送过程中的各种冲突和异常
//
// 参数:
//   - commitSHA: 要推送的提交SHA值，用于日志记录
//
// 返回值:
//   - error: 推送过程中的错误，nil表示成功
//
// 重试策略:
//   - 最多重试3次
//   - 每次重试前重新拉取远程代码
//   - 重新添加文件到暂存区
//   - 如有必要，重新创建提交
//   - 递增的重试间隔时间
//
// 错误处理:
//   - 推送冲突自动解决
//   - 网络异常重试
//   - 认证失败处理
//
// 使用场景:
//   - 多用户环境下的代码推送
//   - 网络不稳定环境的可靠推送
//   - 自动化流程中的错误恢复
func (s *GitService) pushWithRetry(commitSHA string) error {
	maxRetries := 3
	
	for i := 0; i < maxRetries; i++ {
		// ================================================================
		// 尝试推送
		// ================================================================
		
		// 尝试推送到远程仓库
		var auth *http.BasicAuth
		if config.AppConfig.Git.Gitee.Token != "" {
			// 使用访问令牌认证
			auth = &http.BasicAuth{
				Username: config.AppConfig.Git.Gitee.Token,
				Password: "", // Token认证时密码为空
			}
		} else {
			// 使用用户名密码认证
			auth = &http.BasicAuth{
				Username: config.AppConfig.Git.Gitee.Username,
				Password: config.AppConfig.Git.Gitee.Password,
			}
		}
		
		err := s.repo.Push(&git.PushOptions{
			Auth: auth,
		})
		
		// 推送成功，直接返回
		if err == nil {
			return nil
		}
		
		// ================================================================
		// 推送失败处理
		// ================================================================
		
		logger.Logger.Warn("推送失败，尝试重新同步", 
			zap.Int("retry", i+1), 
			zap.Int("max_retries", maxRetries),
			zap.Error(err))
		
		// 如果推送失败，可能是因为远程仓库有新的提交
		// 尝试重新拉取并合并
		if err := s.pullLatest(); err != nil {
			logger.Logger.Error("重新拉取失败", zap.Error(err))
			continue
		}
		
		// ================================================================
		// 重新准备提交
		// ================================================================
		
		// 重新添加文件到暂存区
		worktree, err := s.repo.Worktree()
		if err != nil {
			logger.Logger.Error("获取工作树失败", zap.Error(err))
			continue
		}
		
		_, err = worktree.Add("images.txt")
		if err != nil {
			logger.Logger.Error("添加文件到暂存区失败", zap.Error(err))
			continue
		}
		
		// 检查是否还有更改需要提交
		status, err := worktree.Status()
		if err != nil {
			logger.Logger.Error("检查状态失败", zap.Error(err))
			continue
		}
		
		// 如果有更改，重新提交
		if !status.IsClean() {
			// 重新提交
			commitMsg := fmt.Sprintf("Retry commit: %s", commitSHA)
			_, err = worktree.Commit(commitMsg, &git.CommitOptions{
				Author: &object.Signature{
					Name:  config.AppConfig.Git.Gitee.Username,
					Email: config.AppConfig.Git.Gitee.Email,
					When:  time.Now(),
				},
			})
			if err != nil {
				logger.Logger.Error("重新提交失败", zap.Error(err))
				continue
			}
		}
		
		// ================================================================
		// 重试间隔
		// ================================================================
		
		// 等待一段时间再重试，递增的等待时间
		time.Sleep(time.Duration(i+1) * time.Second)
	}
	
	return fmt.Errorf("推送到Gitee失败，已重试%d次", maxRetries)
}

// GetRepoStatus 获取仓库状态信息
//
// 功能说明:
//   - 获取Git仓库的当前状态信息
//   - 包含工作区状态、最新提交信息等
//   - 提供仓库健康状态的全面视图
//   - 用于监控和调试Git操作
//
// 返回值:
//   - map[string]interface{}: 包含仓库状态的详细信息
//   - error: 获取状态过程中的错误
//
// 状态信息包含:
//   - is_clean: 工作区是否干净（无未提交更改）
//   - last_commit: 最新提交的SHA值
//   - last_message: 最新提交的消息
//   - last_author: 最新提交的作者
//   - last_time: 最新提交的时间
//   - repo_path: 仓库的本地路径
//
// 错误处理:
//   - 仓库未初始化
//   - 获取工作区状态失败
//   - 获取提交信息失败
//
// 使用场景:
//   - 系统状态监控
//   - 调试Git操作问题
//   - 仓库健康检查
//   - 管理界面状态展示
func (s *GitService) GetRepoStatus() (map[string]interface{}, error) {
	// ====================================================================
	// 仓库状态检查
	// ====================================================================
	
	if s.repo == nil {
		return nil, fmt.Errorf("仓库未初始化")
	}

	// ====================================================================
	// 获取工作区状态
	// ====================================================================
	
	worktree, err := s.repo.Worktree()
	if err != nil {
		return nil, err
	}

	status, err := worktree.Status()
	if err != nil {
		return nil, err
	}

	// ====================================================================
	// 获取最新提交信息
	// ====================================================================
	
	// 获取HEAD引用（当前分支的最新提交）
	ref, err := s.repo.Head()
	if err != nil {
		return nil, err
	}

	// 获取提交对象的详细信息
	commit, err := s.repo.CommitObject(ref.Hash())
	if err != nil {
		return nil, err
	}

	// ====================================================================
	// 构建状态信息
	// ====================================================================
	
	return map[string]interface{}{
		"is_clean":     status.IsClean(),        // 工作区是否干净
		"last_commit":  commit.Hash.String(),    // 最新提交SHA
		"last_message": commit.Message,          // 最新提交消息
		"last_author":  commit.Author.Name,      // 最新提交作者
		"last_time":    commit.Author.When,      // 最新提交时间
		"repo_path":    s.repoPath,              // 仓库路径
	}, nil
}

// CleanRepository 清理Git仓库
//
// 功能说明:
//   - 完全删除本地Git仓库目录
//   - 用于重置仓库状态或清理磁盘空间
//   - 删除所有本地文件和Git历史
//   - 为重新初始化仓库做准备
//
// 返回值:
//   - error: 清理过程中的错误，nil表示成功
//
// 清理范围:
//   - 删除整个仓库目录
//   - 包含所有文件和子目录
//   - 包含.git目录和所有历史记录
//
// 风险提示:
//   - 会永久删除所有本地数据
//   - 无法恢复已删除的内容
//   - 适用于自动化场景，谨慎使用
//
// 使用场景:
//   - 仓库状态异常需要重置
//   - 切换到新的仓库地址
//   - 清理磁盘空间
//   - 系统维护和故障恢复
func (s *GitService) CleanRepository() error {
	if s.repoPath != "" {
		return os.RemoveAll(s.repoPath)
	}
	return nil
}
