package services

import (
	"encoding/json"
	"fmt"
	"net/http"
	"strings"
	"time"

	"docker-image-sync-platform/internal/config"
	"docker-image-sync-platform/internal/logger"

	"github.com/go-resty/resty/v2"
	"go.uber.org/zap"
)

// GitHubService GitHub服务
type GitHubService struct {
	client   *resty.Client
	baseURL  string
	owner    string
	repo     string
}

// WorkflowRun GitHub Actions工作流运行信息
type WorkflowRun struct {
	ID         int64  `json:"id"`
	Status     string `json:"status"`
	Conclusion string `json:"conclusion"`
	HTMLURL    string `json:"html_url"`
	HeadSHA    string `json:"head_sha"`
	CreatedAt  string `json:"created_at"`
	UpdatedAt  string `json:"updated_at"`
}

// WorkflowRunsResponse GitHub Actions工作流运行列表响应
type WorkflowRunsResponse struct {
	TotalCount   int           `json:"total_count"`
	WorkflowRuns []WorkflowRun `json:"workflow_runs"`
}

// NewGitHubService 创建GitHub服务
func NewGitHubService() *GitHubService {
	client := resty.New()
	client.SetTimeout(30 * time.Second)
	client.SetHeader("Accept", "application/vnd.github.v3+json")
	client.SetHeader("User-Agent", "docker-image-sync-platform")
	
	if config.AppConfig.Git.GitHub.Token != "" {
		client.SetAuthToken(config.AppConfig.Git.GitHub.Token)
	}

	// 解析仓库信息
	repoURL := config.AppConfig.Git.GitHub.RepoURL
	owner, repo := parseGitHubRepo(repoURL)

	return &GitHubService{
		client:  client,
		baseURL: "https://api.github.com",
		owner:   owner,
		repo:    repo,
	}
}

// GetWorkflowRun 根据提交SHA获取工作流运行信息
func (s *GitHubService) GetWorkflowRun(commitSHA string) (string, string, error) {
	url := fmt.Sprintf("%s/repos/%s/%s/actions/runs", s.baseURL, s.owner, s.repo)
	
	// 等待一段时间，让GitHub同步Gitee的更改
	maxRetries := 10
	for i := 0; i < maxRetries; i++ {
		resp, err := s.client.R().
			SetQueryParam("head_sha", commitSHA).
			SetQueryParam("per_page", "10").
			Get(url)

		if err != nil {
			logger.Logger.Error("查询GitHub Actions失败", zap.Error(err))
			return "", "", err
		}

		if resp.StatusCode() != http.StatusOK {
			logger.Logger.Error("GitHub API响应错误", 
				zap.Int("status_code", resp.StatusCode()),
				zap.String("response", string(resp.Body())))
			return "", "", fmt.Errorf("GitHub API响应错误: %d", resp.StatusCode())
		}

		var response WorkflowRunsResponse
		if err := json.Unmarshal(resp.Body(), &response); err != nil {
			return "", "", fmt.Errorf("解析响应失败: %w", err)
		}

		// 查找匹配的工作流运行
		for _, run := range response.WorkflowRuns {
			if run.HeadSHA == commitSHA {
				logger.Logger.Info("找到GitHub Actions运行", 
					zap.String("run_id", fmt.Sprintf("%d", run.ID)),
					zap.String("status", run.Status),
					zap.String("url", run.HTMLURL))
				return fmt.Sprintf("%d", run.ID), run.HTMLURL, nil
			}
		}

		// 如果没有找到，等待一段时间后重试
		if i < maxRetries-1 {
			logger.Logger.Info("未找到GitHub Actions运行，等待重试", 
				zap.String("commit_sha", commitSHA),
				zap.Int("retry", i+1))
			time.Sleep(30 * time.Second)
		}
	}

	return "", "", fmt.Errorf("未找到提交 %s 对应的GitHub Actions运行", commitSHA)
}

// GetWorkflowRunStatus 获取工作流运行状态
func (s *GitHubService) GetWorkflowRunStatus(runID string) (string, error) {
	url := fmt.Sprintf("%s/repos/%s/%s/actions/runs/%s", s.baseURL, s.owner, s.repo, runID)
	
	resp, err := s.client.R().Get(url)
	if err != nil {
		return "", err
	}

	if resp.StatusCode() != http.StatusOK {
		return "", fmt.Errorf("GitHub API响应错误: %d", resp.StatusCode())
	}

	var run WorkflowRun
	if err := json.Unmarshal(resp.Body(), &run); err != nil {
		return "", fmt.Errorf("解析响应失败: %w", err)
	}

	return run.Status, nil
}

// GetWorkflowRunConclusion 获取工作流运行结论
func (s *GitHubService) GetWorkflowRunConclusion(runID string) (string, error) {
	url := fmt.Sprintf("%s/repos/%s/%s/actions/runs/%s", s.baseURL, s.owner, s.repo, runID)
	
	resp, err := s.client.R().Get(url)
	if err != nil {
		return "", err
	}

	if resp.StatusCode() != http.StatusOK {
		return "", fmt.Errorf("GitHub API响应错误: %d", resp.StatusCode())
	}

	var run WorkflowRun
	if err := json.Unmarshal(resp.Body(), &run); err != nil {
		return "", fmt.Errorf("解析响应失败: %w", err)
	}

	return run.Conclusion, nil
}

// GetWorkflowRunDetails 获取工作流运行详细信息
func (s *GitHubService) GetWorkflowRunDetails(runID string) (*WorkflowRun, error) {
	url := fmt.Sprintf("%s/repos/%s/%s/actions/runs/%s", s.baseURL, s.owner, s.repo, runID)
	
	resp, err := s.client.R().Get(url)
	if err != nil {
		return nil, err
	}

	if resp.StatusCode() != http.StatusOK {
		return nil, fmt.Errorf("GitHub API响应错误: %d", resp.StatusCode())
	}

	var run WorkflowRun
	if err := json.Unmarshal(resp.Body(), &run); err != nil {
		return nil, fmt.Errorf("解析响应失败: %w", err)
	}

	return &run, nil
}

// ListWorkflowRuns 列出工作流运行
func (s *GitHubService) ListWorkflowRuns(page, perPage int) (*WorkflowRunsResponse, error) {
	url := fmt.Sprintf("%s/repos/%s/%s/actions/runs", s.baseURL, s.owner, s.repo)
	
	resp, err := s.client.R().
		SetQueryParam("page", fmt.Sprintf("%d", page)).
		SetQueryParam("per_page", fmt.Sprintf("%d", perPage)).
		Get(url)

	if err != nil {
		return nil, err
	}

	if resp.StatusCode() != http.StatusOK {
		return nil, fmt.Errorf("GitHub API响应错误: %d", resp.StatusCode())
	}

	var response WorkflowRunsResponse
	if err := json.Unmarshal(resp.Body(), &response); err != nil {
		return nil, fmt.Errorf("解析响应失败: %w", err)
	}

	return &response, nil
}

// CheckRateLimit 检查API速率限制
func (s *GitHubService) CheckRateLimit() (map[string]interface{}, error) {
	url := fmt.Sprintf("%s/rate_limit", s.baseURL)
	
	resp, err := s.client.R().Get(url)
	if err != nil {
		return nil, err
	}

	if resp.StatusCode() != http.StatusOK {
		return nil, fmt.Errorf("GitHub API响应错误: %d", resp.StatusCode())
	}

	var rateLimit map[string]interface{}
	if err := json.Unmarshal(resp.Body(), &rateLimit); err != nil {
		return nil, fmt.Errorf("解析响应失败: %w", err)
	}

	return rateLimit, nil
}

// parseGitHubRepo 解析GitHub仓库URL
func parseGitHubRepo(repoURL string) (owner, repo string) {
	// 支持多种格式的URL
	// https://github.com/owner/repo
	// https://github.com/owner/repo.git
	// git@github.com:owner/repo.git
	
	if repoURL == "" {
		return "", ""
	}

	// 移除协议部分
	if idx := strings.Index(repoURL, "://"); idx != -1 {
		repoURL = repoURL[idx+3:]
	}

	// 移除git@前缀
	if strings.HasPrefix(repoURL, "git@") {
		repoURL = repoURL[4:]
		// 替换:为/
		repoURL = strings.Replace(repoURL, ":", "/", 1)
	}

	// 移除github.com/前缀
	if strings.HasPrefix(repoURL, "github.com/") {
		repoURL = repoURL[11:]
	}

	// 移除.git后缀
	if strings.HasSuffix(repoURL, ".git") {
		repoURL = repoURL[:len(repoURL)-4]
	}

	// 分割owner和repo
	parts := strings.Split(repoURL, "/")
	if len(parts) >= 2 {
		return parts[0], parts[1]
	}

	return "", ""
}