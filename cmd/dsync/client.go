package main

import (
	"encoding/json"
	"fmt"
	"os"
	"strings"
	"time"

	"github.com/go-resty/resty/v2"
)

// loginResponse POST /auth/login 的响应
type loginResponse struct {
	Token     string         `json:"token"`
	User      map[string]any `json:"user"`
	ExpiresAt time.Time      `json:"expires_at"`
}

// APIError 携带 HTTP 状态码的平台错误，便于调用方按状态码分支处理（如 429 退避）。
type APIError struct {
	StatusCode int
	Message    string
}

func (e *APIError) Error() string {
	if e.Message != "" {
		return e.Message
	}
	return fmt.Sprintf("HTTP %d", e.StatusCode)
}

// Client 平台 API 客户端，封装认证注入、401 自动重登与错误翻译。
type Client struct {
	cfg  *Config
	rest *resty.Client
}

// NewClient 基于 CLI 配置创建 API 客户端。
func NewClient(cfg *Config) *Client {
	r := resty.New()
	r.SetBaseURL(strings.TrimRight(cfg.Server, "/") + "/api/v1")
	r.SetTimeout(60 * time.Second)
	return &Client{cfg: cfg, rest: r}
}

// do 执行一次 API 请求。result 非 nil 时自动反序列化 2xx 响应体。
// 401 时若本地存有密码则自动重登一次并重试。
func (c *Client) do(method, path string, body, result any) error {
	err := c.doOnce(method, path, body, result, false)
	if apiErr, ok := err.(*APIError); ok && apiErr.StatusCode == 401 && c.cfg.Password != "" {
		if loginErr := c.Relogin(); loginErr != nil {
			return loginErr
		}
		return c.doOnce(method, path, body, result, true)
	}
	return err
}

func (c *Client) doOnce(method, path string, body, result any, relogged bool) error {
	req := c.rest.R()
	if c.cfg.Token != "" {
		req.SetAuthToken(c.cfg.Token)
	}
	if body != nil {
		req.SetBody(body)
	}
	if result != nil {
		req.SetResult(result)
	}
	resp, err := req.Execute(method, path)
	if err != nil {
		return fmt.Errorf("无法连接服务器 %s: %w", c.cfg.Server, err)
	}

	switch {
	case resp.StatusCode() == 401:
		if !relogged && c.cfg.Password != "" {
			// 交由上层 do() 处理重登
			return &APIError{StatusCode: 401}
		}
		return &APIError{StatusCode: 401, Message: "登录已过期，请重新执行 dsync login"}
	case resp.StatusCode() == 403:
		return &APIError{StatusCode: 403, Message: "当前账号角色缺少所需权限，请联系管理员（" + extractErrorMessage(resp.Body()) + "）"}
	case resp.StatusCode() >= 400:
		return &APIError{StatusCode: resp.StatusCode(), Message: extractErrorMessage(resp.Body())}
	}
	return nil
}

// Login 用用户名密码登录，成功后将 token 写回配置（不落盘，由调用方决定）。
func (c *Client) Login(username, password string) (*loginResponse, error) {
	var result loginResponse
	req := c.rest.R().SetBody(map[string]string{
		"username": username,
		"password": password,
	}).SetResult(&result)
	resp, err := req.Post("/auth/login")
	if err != nil {
		return nil, fmt.Errorf("无法连接服务器 %s: %w", c.cfg.Server, err)
	}
	if resp.StatusCode() != 200 {
		return nil, &APIError{StatusCode: resp.StatusCode(), Message: extractErrorMessage(resp.Body())}
	}
	c.cfg.Token = result.Token
	c.cfg.TokenExpiresAt = result.ExpiresAt.Format(time.RFC3339)
	return &result, nil
}

// Relogin 用本地保存的凭据重新获取 token。
func (c *Client) Relogin() error {
	if c.cfg.Username == "" || c.cfg.Password == "" {
		return &APIError{StatusCode: 401, Message: "登录已过期，请重新执行 dsync login"}
	}
	if _, err := c.Login(c.cfg.Username, c.cfg.Password); err != nil {
		return fmt.Errorf("自动续期登录失败: %w", err)
	}
	return saveConfig(c.cfg)
}

// postSyncLimited 提交类接口（/sync/submit、/sync/batch）有独立限流：
// 每 IP 每 12 秒 1 次。这里对 429 做指数提示退避重试。
func (c *Client) postSyncLimited(path string, body, result any) error {
	const maxAttempts = 4
	var lastErr error
	for attempt := 1; attempt <= maxAttempts; attempt++ {
		err := c.do("POST", path, body, result)
		if err == nil {
			return nil
		}
		apiErr, ok := err.(*APIError)
		if !ok || apiErr.StatusCode != 429 {
			return err
		}
		lastErr = apiErr
		if attempt < maxAttempts {
			fmt.Fprintf(os.Stderr, "平台提交限流（每 12 秒仅允许 1 次），13 秒后第 %d/%d 次重试...\n", attempt, maxAttempts-1)
			time.Sleep(13 * time.Second)
		}
	}
	return fmt.Errorf("提交仍被限流，请稍后重试: %w", lastErr)
}

// extractErrorMessage 从错误响应体中提取人话信息。
// 平台存在三种错误格式：{"error"}、{"code","message"}、{"status","message"}。
func extractErrorMessage(body []byte) string {
	var m map[string]any
	if err := json.Unmarshal(body, &m); err != nil {
		if len(body) > 0 {
			return string(body)
		}
		return "未知错误"
	}
	for _, key := range []string{"error", "message"} {
		if v, ok := m[key].(string); ok && v != "" {
			return v
		}
	}
	return fmt.Sprintf("HTTP 错误: %s", string(body))
}
