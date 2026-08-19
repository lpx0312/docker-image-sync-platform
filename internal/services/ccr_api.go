package services

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"
)

// CCR 个人版管理面 API 的区域。个人版数据全局（ccr.ccs.tencentyun.com 无区域），
// 腾讯云 API 网关需要区域参数，参考官方 SDK 默认使用 ap-guangzhou。
const ccrManageRegion = "ap-guangzhou"

const (
	tcrAPIVersion  = "2019-09-24"
	tcrAPIActionListRepos = "DescribeRepositoryFilterPersonal"
)

// CcrAPIService 腾讯云 CCR（个人版）客户端
//
// 数据面（Tag/Manifest/存在性检查）由通用 bearerV2Client 提供，认证流程：
//  1. GET https://ccr.ccs.tencentyun.com/v2/（不带认证）→ 401，
//     WWW-Authenticate: Bearer realm="https://ccr.ccs.tencentyun.com/service/token",service="token-service"
//  2. GET {realm}?service=...&scope=repository:{命名空间}/{仓库}:pull（Basic 认证）
//     → {"token":"<JWT>","expires_in":7200}
//
// 注意：
//   - docker login 凭证为腾讯云账号数字 ID + 仓库密码（与 SecretId/SecretKey 无关）
//   - 不存在的仓库 tags/list 返回标准 404 NAME_UNKNOWN
//   - /v2/_catalog 不被支持，列举仓库走管理面 tcr API（见 ListRepositories）
type CcrAPIService struct {
	*bearerV2Client
}

var _ RegistryAPIClient = (*CcrAPIService)(nil)

// NewCcrAPIService 创建 CCR API 服务实例
func NewCcrAPIService() *CcrAPIService {
	return &CcrAPIService{bearerV2Client: newBearerV2Client("CCR")}
}

// callTcrAPI 调用腾讯云 tcr API（TC3 签名 POST），返回响应体与状态码
func (s *CcrAPIService) callTcrAPI(action string, payload map[string]interface{}, secretID, secretKey string) ([]byte, int, error) {
	host := fmt.Sprintf("tcr.%s.tencentcloudapi.com", ccrManageRegion)
	body, err := json.Marshal(payload)
	if err != nil {
		return nil, 0, err
	}

	req, err := http.NewRequest(http.MethodPost, "https://"+host+"/", bytes.NewReader(body))
	if err != nil {
		return nil, 0, err
	}
	now := time.Now()
	signer := tc3Signer{SecretID: secretID, SecretKey: secretKey}
	req.Header.Set("Content-Type", "application/json; charset=utf-8")
	req.Header.Set("Authorization", signer.tc3AuthHeader(host, "tcr", action, body, now))
	req.Header.Set("X-TC-Action", action)
	req.Header.Set("X-TC-Timestamp", fmt.Sprintf("%d", now.Unix()))
	req.Header.Set("X-TC-Version", tcrAPIVersion)
	req.Header.Set("X-TC-Region", ccrManageRegion)

	resp, err := (&http.Client{Timeout: 30 * time.Second}).Do(req)
	if err != nil {
		return nil, 0, fmt.Errorf("请求腾讯云 API 失败: %w", err)
	}
	defer resp.Body.Close()
	respBody, _ := io.ReadAll(io.LimitReader(resp.Body, 8<<20))
	return respBody, resp.StatusCode, nil
}

// parseTcrResponse 解析腾讯云响应：业务错误时返回错误（腾讯云 API 出错通常仍返回 HTTP 200）
func parseTcrResponse(body []byte) (json.RawMessage, error) {
	var parsed struct {
		Response struct {
			Error *struct {
				Code    string `json:"Code"`
				Message string `json:"Message"`
			} `json:"Error"`
			Data json.RawMessage `json:"Data"`
			// 部分接口直接挂在 Response 下（DescribeRepositoryFilterPersonal 的 Data 已含所需字段）
		} `json:"Response"`
	}
	if err := json.Unmarshal(body, &parsed); err != nil {
		return nil, fmt.Errorf("解析腾讯云响应失败: %w", err)
	}
	if parsed.Response.Error != nil {
		code := parsed.Response.Error.Code
		msg := parsed.Response.Error.Message
		if strings.Contains(code, "AuthFailure") || strings.Contains(code, "Unauthorized") {
			return nil, fmt.Errorf("SecretId/SecretKey 验证失败（%s）: %s", code, msg)
		}
		return nil, fmt.Errorf("腾讯云 API 错误（%s）: %s", code, msg)
	}
	return parsed.Response.Data, nil
}

// ListRepositories 通过腾讯云 tcr API（DescribeRepositoryFilterPersonal）
// 列出命名空间下全部镜像仓库，用于「从仓库导入」。CCR 数据面不支持 /v2/_catalog。
//
// 管理面凭证为腾讯云 SecretId/SecretKey（与数据面 docker login 凭证相互独立）。
func (s *CcrAPIService) ListRepositories(registry, username, password, accessKey, secretKey, namespace, authServer, dockerService string) ([]string, error) {
	if secretKey == "" {
		return nil, fmt.Errorf("未配置腾讯云管理面 SecretId/SecretKey，无法获取镜像列表，请在镜像仓库配置中填写")
	}

	repos := make([]string, 0, 64)
	const limit = 100
	for offset := 0; ; offset += limit {
		body, status, err := s.callTcrAPI(tcrAPIActionListRepos, map[string]interface{}{
			"Namespace": namespace,
			"Offset":    offset,
			"Limit":     limit,
		}, accessKey, secretKey)
		if err != nil {
			return nil, err
		}
		if status != http.StatusOK {
			detail := string(body)
			if len(detail) > 200 {
				detail = detail[:200]
			}
			return nil, fmt.Errorf("获取 CCR 仓库列表失败: HTTP %d: %s", status, detail)
		}

		data, err := parseTcrResponse(body)
		if err != nil {
			return nil, err
		}

		var page struct {
			RepoInfo []struct {
				RepoName string `json:"RepoName"`
			} `json:"RepoInfo"`
			TotalCount int `json:"TotalCount"`
		}
		if err := json.Unmarshal(data, &page); err != nil {
			return nil, fmt.Errorf("解析 CCR 仓库列表失败: %w", err)
		}

		for _, it := range page.RepoInfo {
			name := it.RepoName
			// RepoName 形如 "命名空间/仓库名"，去掉前缀
			if idx := strings.LastIndex(name, "/"); idx >= 0 {
				name = name[idx+1:]
			}
			if name = strings.TrimSpace(name); name != "" {
				repos = append(repos, name)
			}
		}

		if len(page.RepoInfo) == 0 || len(repos) >= page.TotalCount {
			break
		}
	}

	return repos, nil
}

// TestConnection 测试 CCR 配置连通性：数据面登录凭证（推送/拉取）+ 管理面 SecretId/SecretKey（获取镜像列表）
func (s *CcrAPIService) TestConnection(registry, username, password, accessKey, secretKey, namespace, authServer, dockerService string) *RegistryTestResult {
	result := &RegistryTestResult{RegistryType: "ccr"}

	// 数据面：用登录凭证换取 scoped token（与 docker login 等价）
	if _, err := s.GetToken(registry, username, password, namespace, "connection-test"); err != nil {
		result.LoginMessage = err.Error()
	} else {
		result.LoginOK = true
		result.LoginMessage = "登录凭证可用（可推送/拉取镜像）"
	}

	// 管理面：SecretId/SecretKey 签名查询命名空间（Limit=1，仅验证连通与权限）
	if secretKey == "" {
		result.ManageSkipped = true
		result.ManageMessage = "未配置 SecretId/SecretKey（可选；不配置则「从仓库导入」不可用）"
		return result
	}
	if err := s.testManageAccess(accessKey, secretKey, namespace); err != nil {
		result.ManageMessage = err.Error()
	} else {
		result.ManageOK = true
		result.ManageMessage = "SecretId/SecretKey 可用（可获取镜像列表）"
	}
	return result
}

// testManageAccess 验证管理面凭证（DescribeRepositoryFilterPersonal，Limit=1）
func (s *CcrAPIService) testManageAccess(secretID, secretKey, namespace string) error {
	body, status, err := s.callTcrAPI(tcrAPIActionListRepos, map[string]interface{}{
		"Namespace": namespace,
		"Offset":    0,
		"Limit":     1,
	}, secretID, secretKey)
	if err != nil {
		return err
	}
	if status != http.StatusOK {
		return fmt.Errorf("HTTP %d", status)
	}
	if _, err := parseTcrResponse(body); err != nil {
		return err
	}
	return nil
}
