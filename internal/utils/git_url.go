// Package utils 提供通用的工具函数
package utils

import (
	"fmt"
	"strings"
)

// ParseGitHubRepoURL 解析 GitHub 仓库 URL，提取 owner 和 repo
//
// 支持的 URL 格式:
//   - https://github.com/owner/repo
//   - https://github.com/owner/repo.git
//   - http://github.com/owner/repo.git
//   - git@github.com:owner/repo.git
//   - owner/repo (简写格式)
//
// 参数:
//   - repoURL: GitHub 仓库 URL
//
// 返回值:
//   - owner: 仓库所有者
//   - repo: 仓库名称
//   - error: 解析错误
func ParseGitHubRepoURL(repoURL string) (owner, repo string, err error) {
	if repoURL == "" {
		return "", "", fmt.Errorf("仓库 URL 不能为空")
	}

	// 保存原始 URL 用于错误信息
	originalURL := repoURL

	// 移除协议部分（http://、https://）
	if idx := strings.Index(repoURL, "://"); idx != -1 {
		repoURL = repoURL[idx+3:]
	}

	// 处理 SSH 格式 (git@github.com:owner/repo.git)
	if strings.HasPrefix(repoURL, "git@") {
		repoURL = repoURL[4:]
		// 替换 SSH 格式中的冒号为斜杠
		repoURL = strings.Replace(repoURL, ":", "/", 1)
	}

	// 移除 github.com/ 前缀
	if strings.HasPrefix(repoURL, "github.com/") {
		repoURL = repoURL[11:]
	}

	// 移除 .git 后缀
	repoURL = strings.TrimSuffix(repoURL, ".git")

	// 分割 owner 和 repo
	parts := strings.Split(repoURL, "/")
	if len(parts) != 2 {
		return "", "", fmt.Errorf("无效的 GitHub 仓库 URL: %s", originalURL)
	}

	owner = strings.TrimSpace(parts[0])
	repo = strings.TrimSpace(parts[1])

	if owner == "" || repo == "" {
		return "", "", fmt.Errorf("无法从 URL 解析 owner 或 repo: %s", originalURL)
	}

	return owner, repo, nil
}

// ParseGitHubRepoURLSimple 解析 GitHub 仓库 URL（简单版本，不返回错误）
//
// 当解析失败时返回空字符串，适用于不需要错误处理的场景
//
// 参数:
//   - repoURL: GitHub 仓库 URL
//
// 返回值:
//   - owner: 仓库所有者（失败时为空字符串）
//   - repo: 仓库名称（失败时为空字符串）
func ParseGitHubRepoURLSimple(repoURL string) (owner, repo string) {
	owner, repo, _ = ParseGitHubRepoURL(repoURL)
	return owner, repo
}

// ParseGiteeRepoURL 解析 Gitee 仓库 URL，提取 owner 和 repo
//
// 支持的 URL 格式:
//   - https://gitee.com/owner/repo
//   - https://gitee.com/owner/repo.git
//   - git@gitee.com:owner/repo.git
//
// 参数:
//   - repoURL: Gitee 仓库 URL
//
// 返回值:
//   - owner: 仓库所有者
//   - repo: 仓库名称
//   - error: 解析错误
func ParseGiteeRepoURL(repoURL string) (owner, repo string, err error) {
	if repoURL == "" {
		return "", "", fmt.Errorf("仓库 URL 不能为空")
	}

	originalURL := repoURL

	// 移除协议部分
	if idx := strings.Index(repoURL, "://"); idx != -1 {
		repoURL = repoURL[idx+3:]
	}

	// 处理 SSH 格式
	if strings.HasPrefix(repoURL, "git@") {
		repoURL = repoURL[4:]
		repoURL = strings.Replace(repoURL, ":", "/", 1)
	}

	// 移除 gitee.com/ 前缀
	if strings.HasPrefix(repoURL, "gitee.com/") {
		repoURL = repoURL[10:]
	}

	// 移除 .git 后缀
	repoURL = strings.TrimSuffix(repoURL, ".git")

	parts := strings.Split(repoURL, "/")
	if len(parts) != 2 {
		return "", "", fmt.Errorf("无效的 Gitee 仓库 URL: %s", originalURL)
	}

	owner = strings.TrimSpace(parts[0])
	repo = strings.TrimSpace(parts[1])

	if owner == "" || repo == "" {
		return "", "", fmt.Errorf("无法从 URL 解析 owner 或 repo: %s", originalURL)
	}

	return owner, repo, nil
}

// ParseGitRepoURL 自动识别并解析 Git 仓库 URL（支持 GitHub 和 Gitee）
//
// 参数:
//   - repoURL: Git 仓库 URL
//
// 返回值:
//   - owner: 仓库所有者
//   - repo: 仓库名称
//   - error: 解析错误
func ParseGitRepoURL(repoURL string) (owner, repo string, err error) {
	if strings.Contains(repoURL, "github.com") {
		return ParseGitHubRepoURL(repoURL)
	}
	if strings.Contains(repoURL, "gitee.com") {
		return ParseGiteeRepoURL(repoURL)
	}
	return "", "", fmt.Errorf("不支持的 Git 仓库 URL: %s", repoURL)
}
