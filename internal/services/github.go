// Package services 提供核心业务服务实现
//
// github.go 文件实现了GitHub API集成服务，主要负责：
// - GitHub Actions工作流的监控和管理
// - 与GitHub API的安全通信
// - 工作流运行状态的实时跟踪
// - API速率限制的监控和管理
//
// GitHub服务功能：
// - 支持GitHub Actions工作流的查询和监控
// - 提供工作流运行状态的实时更新
// - 实现API调用的重试和错误处理机制
// - 支持多种GitHub仓库URL格式的解析
// - 提供API速率限制的监控功能
//
// 主要业务场景：
// - 镜像同步任务的CI/CD流水线监控
// - GitHub Actions工作流状态的实时跟踪
// - 同步任务完成状态的自动化检测
// - 与Gitee代码推送的联动触发
//
// 作者: Docker镜像同步平台开发团队
// 版本: v1.0.0
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

// GitHubService GitHub API集成服务
//
// 负责管理Docker镜像同步平台与GitHub的集成，包括：
// - GitHub Actions工作流的监控和状态查询
// - 与GitHub API的安全认证和通信
// - 工作流运行结果的实时跟踪
// - API调用的错误处理和重试机制
//
// 核心功能：
//   - 自动化的工作流状态监控
//   - 智能的API调用重试策略
//   - 多格式GitHub仓库URL的解析支持
//   - API速率限制的监控和管理
//   - 与镜像同步流程的无缝集成
type GitHubService struct {
	client   *resty.Client // HTTP客户端，用于API调用
	baseURL  string        // GitHub API基础URL
	owner    string        // 仓库所有者
	repo     string        // 仓库名称
}

// WorkflowRun GitHub Actions工作流运行信息
//
// 表示GitHub Actions中单次工作流运行的详细信息，包含：
// - 运行的唯一标识和状态信息
// - 关联的提交SHA和时间戳
// - 工作流的执行结果和访问链接
//
// 字段说明：
//   - ID: 工作流运行的唯一标识符
//   - Status: 当前运行状态（queued, in_progress, completed等）
//   - Conclusion: 运行结论（success, failure, cancelled等）
//   - HTMLURL: 工作流运行的GitHub页面链接
//   - HeadSHA: 触发工作流的提交SHA值
//   - CreatedAt: 工作流创建时间
//   - UpdatedAt: 工作流最后更新时间
type WorkflowRun struct {
	ID         int64  `json:"id"`         // 工作流运行ID
	Status     string `json:"status"`     // 运行状态
	Conclusion string `json:"conclusion"` // 运行结论
	HTMLURL    string `json:"html_url"`   // GitHub页面链接
	HeadSHA    string `json:"head_sha"`   // 提交SHA
	CreatedAt  string `json:"created_at"` // 创建时间
	UpdatedAt  string `json:"updated_at"` // 更新时间
}

// WorkflowRunsResponse GitHub Actions工作流运行列表响应
//
// 表示GitHub API返回的工作流运行列表，包含：
// - 总数统计信息
// - 工作流运行的详细列表
//
// 字段说明：
//   - TotalCount: 符合条件的工作流运行总数
//   - WorkflowRuns: 工作流运行详细信息列表
//
// 使用场景：
//   - 分页查询工作流运行历史
//   - 统计工作流执行情况
//   - 批量处理工作流数据
type WorkflowRunsResponse struct {
	TotalCount   int           `json:"total_count"`   // 总数
	WorkflowRuns []WorkflowRun `json:"workflow_runs"` // 工作流运行列表
}

// NewGitHubService 创建GitHub服务实例
//
// 功能说明:
//   - 初始化GitHub API客户端，配置认证和超时
//   - 从应用配置中读取GitHub相关设置
//   - 解析GitHub仓库URL，提取所有者和仓库名
//   - 配置HTTP客户端的默认头部和认证信息
//
// 返回值:
//   - *GitHubService: GitHub服务实例，包含配置的API客户端
//
// 配置项:
//   - 30秒的HTTP请求超时
//   - GitHub API v3的Accept头部
//   - 自定义User-Agent标识
//   - 基于Token的认证（如果配置）
//
// 使用场景:
//   - 系统启动时初始化GitHub集成
//   - 镜像同步任务需要监控GitHub Actions时
//   - 需要查询工作流状态时
func NewGitHubService() *GitHubService {
	// ====================================================================
	// HTTP客户端初始化
	// ====================================================================
	
	client := resty.New()
	client.SetTimeout(30 * time.Second)
	client.SetHeader("Accept", "application/vnd.github.v3+json")
	client.SetHeader("User-Agent", "docker-image-sync-platform")
	
	// ====================================================================
	// 认证配置
	// ====================================================================
	
	// 如果配置了GitHub Token，则设置认证
	if config.AppConfig.Git.GitHub.Token != "" {
		client.SetAuthToken(config.AppConfig.Git.GitHub.Token)
	}

	// ====================================================================
	// 仓库信息解析
	// ====================================================================
	
	// 解析GitHub仓库URL，提取所有者和仓库名
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
//
// 功能说明:
//   - 根据Git提交SHA查找对应的GitHub Actions工作流运行
//   - 支持重试机制，等待GitHub同步Gitee的代码推送
//   - 返回工作流运行ID和访问链接
//   - 处理GitHub API的各种响应状态
//
// 参数:
//   - commitSHA: Git提交的SHA值，用于匹配工作流运行
//
// 返回值:
//   - string: 工作流运行ID，用于后续状态查询
//   - string: 工作流运行的GitHub页面链接
//   - error: 查询过程中的错误，nil表示成功
//
// 重试策略:
//   - 最多重试10次，每次间隔30秒
//   - 适应GitHub和Gitee之间的同步延迟
//   - 智能的错误处理和日志记录
//
// 错误处理:
//   - API调用失败
//   - HTTP状态码异常
//   - JSON解析错误
//   - 未找到匹配的工作流运行
//
// 使用场景:
//   - 镜像同步任务提交后查找对应的CI/CD流水线
//   - 监控代码推送触发的自动化工作流
//   - 获取工作流运行链接供用户查看
func (s *GitHubService) GetWorkflowRun(commitSHA string) (string, string, error) {
	url := fmt.Sprintf("%s/repos/%s/%s/actions/runs", s.baseURL, s.owner, s.repo)
	
	// ====================================================================
	// 重试查询机制
	// ====================================================================
	
	// 等待一段时间，让GitHub同步Gitee的更改
	// GitHub和Gitee之间可能存在同步延迟
	maxRetries := 10
	for i := 0; i < maxRetries; i++ {
		// ================================================================
		// API调用
		// ================================================================
		
		resp, err := s.client.R().
			SetQueryParam("head_sha", commitSHA).
			SetQueryParam("per_page", "10").
			Get(url)

		if err != nil {
			logger.Logger.Error("查询GitHub Actions失败", zap.Error(err))
			return "", "", err
		}

		// ================================================================
		// 响应状态检查
		// ================================================================
		
		if resp.StatusCode() != http.StatusOK {
			logger.Logger.Error("GitHub API响应错误", 
				zap.Int("status_code", resp.StatusCode()),
				zap.String("response", string(resp.Body())))
			return "", "", fmt.Errorf("GitHub API响应错误: %d", resp.StatusCode())
		}

		// ================================================================
		// 响应解析
		// ================================================================
		
		var response WorkflowRunsResponse
		if err := json.Unmarshal(resp.Body(), &response); err != nil {
			return "", "", fmt.Errorf("解析响应失败: %w", err)
		}

		// ================================================================
		// 查找匹配的工作流
		// ================================================================
		
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

		// ================================================================
		// 重试等待
		// ================================================================
		
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
//
// 功能说明:
//   - 根据工作流运行ID查询当前运行状态
//   - 返回工作流的实时执行状态
//   - 用于监控工作流的执行进度
//
// 参数:
//   - runID: 工作流运行的唯一标识符
//
// 返回值:
//   - string: 工作流运行状态（queued, in_progress, completed等）
//   - error: 查询过程中的错误，nil表示成功
//
// 状态类型:
//   - queued: 排队等待执行
//   - in_progress: 正在执行中
//   - completed: 执行完成
//
// 错误处理:
//   - API调用失败
//   - HTTP状态码异常
//   - JSON解析错误
//   - 工作流运行不存在
//
// 使用场景:
//   - 实时监控工作流执行进度
//   - 判断镜像同步任务是否完成
//   - 提供用户界面的状态更新
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
//
// 功能说明:
//   - 根据工作流运行ID查询最终执行结论
//   - 返回工作流的执行结果（成功、失败等）
//   - 用于判断镜像同步任务的最终状态
//
// 参数:
//   - runID: 工作流运行的唯一标识符
//
// 返回值:
//   - string: 工作流运行结论（success, failure, cancelled等）
//   - error: 查询过程中的错误，nil表示成功
//
// 结论类型:
//   - success: 执行成功
//   - failure: 执行失败
//   - cancelled: 被取消
//   - skipped: 被跳过
//   - timed_out: 执行超时
//
// 错误处理:
//   - API调用失败
//   - HTTP状态码异常
//   - JSON解析错误
//   - 工作流运行不存在
//
// 使用场景:
//   - 判断镜像同步任务的最终结果
//   - 更新数据库中的任务状态
//   - 发送任务完成通知
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
//
// 功能说明:
//   - 根据工作流运行ID获取完整的运行信息
//   - 返回包含所有字段的工作流运行对象
//   - 提供最全面的工作流状态数据
//
// 参数:
//   - runID: 工作流运行的唯一标识符
//
// 返回值:
//   - *WorkflowRun: 完整的工作流运行信息对象
//   - error: 查询过程中的错误，nil表示成功
//
// 包含信息:
//   - 运行ID、状态和结论
//   - 关联的提交SHA和时间戳
//   - GitHub页面访问链接
//   - 创建和更新时间
//
// 错误处理:
//   - API调用失败
//   - HTTP状态码异常
//   - JSON解析错误
//   - 工作流运行不存在
//
// 使用场景:
//   - 获取工作流的完整状态信息
//   - 生成详细的状态报告
//   - 调试工作流执行问题
//   - 提供管理界面的详细视图
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
//
// 功能说明:
//   - 分页查询仓库的工作流运行历史
//   - 支持自定义页码和每页数量
//   - 返回工作流运行列表和总数统计
//   - 用于历史记录查询和统计分析
//
// 参数:
//   - page: 页码，从1开始
//   - perPage: 每页返回的工作流运行数量
//
// 返回值:
//   - *WorkflowRunsResponse: 包含工作流运行列表和总数的响应对象
//   - error: 查询过程中的错误，nil表示成功
//
// 分页说明:
//   - 支持GitHub API的标准分页机制
//   - 建议每页数量不超过100
//   - 返回结果按时间倒序排列
//
// 错误处理:
//   - API调用失败
//   - HTTP状态码异常
//   - JSON解析错误
//   - 参数验证失败
//
// 使用场景:
//   - 管理界面的工作流历史展示
//   - 统计工作流执行情况
//   - 查找特定时间段的工作流运行
//   - 生成工作流执行报告
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
//
// 功能说明:
//   - 查询当前GitHub API的速率限制状态
//   - 返回剩余调用次数和重置时间
//   - 用于监控API使用情况和避免限制
//   - 提供API调用的健康状态信息
//
// 返回值:
//   - map[string]interface{}: 包含速率限制详细信息的映射
//   - error: 查询过程中的错误，nil表示成功
//
// 限制信息包含:
//   - core.limit: 核心API的总限制数
//   - core.remaining: 核心API的剩余调用数
//   - core.reset: 核心API限制重置的Unix时间戳
//   - search.limit: 搜索API的总限制数
//   - search.remaining: 搜索API的剩余调用数
//   - search.reset: 搜索API限制重置的Unix时间戳
//
// 错误处理:
//   - API调用失败
//   - HTTP状态码异常
//   - JSON解析错误
//   - 认证失败
//
// 使用场景:
//   - 监控API使用情况
//   - 实现智能的API调用策略
//   - 避免触发速率限制
//   - 系统健康状态检查
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
//
// 功能说明:
//   - 解析多种格式的GitHub仓库URL
//   - 提取仓库所有者和仓库名称
//   - 支持HTTPS和SSH格式的URL
//   - 处理各种URL变体和边界情况
//
// 参数:
//   - repoURL: GitHub仓库的URL字符串
//
// 返回值:
//   - owner: 仓库所有者的用户名或组织名
//   - repo: 仓库名称
//
// 支持的URL格式:
//   - https://github.com/owner/repo
//   - https://github.com/owner/repo.git
//   - git@github.com:owner/repo.git
//   - github.com/owner/repo
//
// 解析逻辑:
//   - 移除协议前缀（http://、https://）
//   - 处理SSH格式的git@前缀
//   - 移除github.com域名前缀
//   - 移除.git后缀
//   - 分割路径提取所有者和仓库名
//
// 错误处理:
//   - 空URL返回空字符串
//   - 格式不正确返回空字符串
//   - 路径不完整返回空字符串
//
// 使用场景:
//   - 初始化GitHub服务时解析配置的仓库URL
//   - 支持用户输入的多种URL格式
//   - 配置验证和标准化
func parseGitHubRepo(repoURL string) (owner, repo string) {
	// ====================================================================
	// 输入验证
	// ====================================================================
	
	// 支持多种格式的URL
	// https://github.com/owner/repo
	// https://github.com/owner/repo.git
	// git@github.com:owner/repo.git
	
	if repoURL == "" {
		return "", ""
	}

	// ====================================================================
	// URL标准化处理
	// ====================================================================
	
	// 移除协议部分（http://、https://）
	if idx := strings.Index(repoURL, "://"); idx != -1 {
		repoURL = repoURL[idx+3:]
	}

	// 移除git@前缀并处理SSH格式
	if strings.HasPrefix(repoURL, "git@") {
		repoURL = repoURL[4:]
		// 替换SSH格式中的冒号为斜杠
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

	// ====================================================================
	// 路径分割和提取
	// ====================================================================
	
	// 分割owner和repo
	parts := strings.Split(repoURL, "/")
	if len(parts) >= 2 {
		return parts[0], parts[1]
	}

	return "", ""
}