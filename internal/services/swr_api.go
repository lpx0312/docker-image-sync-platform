package services

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"time"
)

// SwrAPIService 华为云 SWR 客户端
//
// 数据面（Tag/Manifest/存在性检查）由通用 bearerV2Client 提供，认证流程：
//  1. GET https://{registry}/v2/（不带认证）→ 401，响应头
//     WWW-Authenticate: Bearer realm="https://{registry}/swr/auth/v2/registry/auth/",service="dockyard"
//  2. GET {realm}?service=...&scope=repository:{组织}/{仓库}:pull（Basic 认证）→ {"token":"<JWT>"}（约 900s 有效）
//
// 注意：
//   - Basic 直连 /v2 会被拒绝（401），必须走 token 流程
//   - scope 必须携带具体 repository:{ns}/{repo}:pull，无 scope 的 token 无仓库访问权限
//   - 组织不存在时 token 端点直接返回 401 DENIED（"Image organization does not exist"）
//   - /v2/_catalog 不被支持（404），列举仓库走管理面 API（见 ListRepositories）
//   - 不存在的仓库 tags/list 返回 200 且 tags 为 null，RepositoryExists 依此判断
type SwrAPIService struct {
	*bearerV2Client
}

var _ RegistryAPIClient = (*SwrAPIService)(nil)

// NewSwrAPIService 创建 SWR API 服务实例
func NewSwrAPIService() *SwrAPIService {
	return &SwrAPIService{bearerV2Client: newBearerV2Client("SWR")}
}

// ListRepositories 通过 SWR 管理面 API（swr-api.{region}.myhuaweicloud.com）
// 列出组织内全部镜像仓库，用于「从仓库导入」。SWR 数据面不支持 /v2/_catalog。
//
// 管理面凭证为 IAM AK/SK（与数据面登录凭证相互独立）：accessKey/secretKey
// 来自镜像仓库配置中单独填写的 Access Key / Secret Key；accessKey 为空时
// 兜底从登录用户名（区域@AK）中提取。
func (s *SwrAPIService) ListRepositories(registry, username, password, accessKey, secretKey, namespace, authServer, dockerService string) ([]string, error) {
	if secretKey == "" {
		return nil, fmt.Errorf("未配置 SWR 管理面 Secret Key（AK/SK），无法获取镜像列表，请在镜像仓库配置中填写")
	}
	region := swrRegion(registry, username)
	if region == "" {
		return nil, fmt.Errorf("无法从仓库地址 %s 或用户名 %s 推断 SWR 区域", registry, username)
	}
	ak := accessKey
	if ak == "" {
		ak = swrAKFromUsername(username)
	}
	host := fmt.Sprintf("swr-api.%s.myhuaweicloud.com", region)
	signer := swrSigner{AccessKey: ak, SecretKey: secretKey}

	httpClient := &http.Client{Timeout: 30 * time.Second}
	repos := make([]string, 0, 64)
	const limit = 100

	for offset := 0; ; offset += limit {
		u := &url.URL{
			Scheme: "https",
			Host:   host,
			Path:   "/v2/manage/repos",
			RawQuery: url.Values{
				"namespace": []string{namespace},
				"limit":     []string{fmt.Sprintf("%d", limit)},
				"offset":    []string{fmt.Sprintf("%d", offset)},
			}.Encode(),
		}
		req, err := http.NewRequest(http.MethodGet, u.String(), nil)
		if err != nil {
			return nil, err
		}
		signer.sign(req, time.Now())

		resp, err := httpClient.Do(req)
		if err != nil {
			return nil, fmt.Errorf("请求 SWR 管理面失败: %w", err)
		}
		body, _ := io.ReadAll(io.LimitReader(resp.Body, 8<<20))
		resp.Body.Close()

		if resp.StatusCode == http.StatusUnauthorized {
			return nil, fmt.Errorf("SWR 管理面 AK/SK 签名验证失败，请检查镜像仓库配置中的 Access Key / Secret Key（华为云控制台「我的凭证 → 访问密钥」）")
		}
		if resp.StatusCode != http.StatusOK {
			detail := string(body)
			if len(detail) > 200 {
				detail = detail[:200]
			}
			return nil, fmt.Errorf("获取 SWR 仓库列表失败: HTTP %d: %s", resp.StatusCode, detail)
		}

		items, err := parseSwrRepoList(body)
		if err != nil {
			return nil, err
		}
		for _, it := range items {
			name := it
			if idx := strings.LastIndex(name, "/"); idx >= 0 {
				name = name[idx+1:]
			}
			if name = strings.TrimSpace(name); name != "" {
				repos = append(repos, name)
			}
		}
		if len(items) < limit {
			break
		}
	}

	return repos, nil
}

// parseSwrRepoList 兼容数组与 {"body":[...]} 两种响应形态
func parseSwrRepoList(body []byte) ([]string, error) {
	var arr []struct {
		Name string `json:"name"`
		Path string `json:"path"`
	}
	if err := json.Unmarshal(body, &arr); err == nil {
		names := make([]string, 0, len(arr))
		for _, it := range arr {
			if it.Name != "" {
				names = append(names, it.Name)
			} else if it.Path != "" {
				names = append(names, it.Path)
			}
		}
		return names, nil
	}

	var wrapped struct {
		Body []struct {
			Name string `json:"name"`
			Path string `json:"path"`
		} `json:"body"`
	}
	if err := json.Unmarshal(body, &wrapped); err == nil {
		names := make([]string, 0, len(wrapped.Body))
		for _, it := range wrapped.Body {
			if it.Name != "" {
				names = append(names, it.Name)
			} else if it.Path != "" {
				names = append(names, it.Path)
			}
		}
		return names, nil
	}

	return nil, fmt.Errorf("解析 SWR 仓库列表响应失败")
}

// TestConnection 测试 SWR 配置连通性：数据面登录凭证（推送/拉取）+ 管理面 AK/SK（获取镜像列表）
func (s *SwrAPIService) TestConnection(registry, username, password, accessKey, secretKey, namespace, authServer, dockerService string) *RegistryTestResult {
	result := &RegistryTestResult{RegistryType: "swr"}

	// 数据面：用登录凭证换取 scoped token（与 docker login 等价），组织不存在/无权限会报错
	if _, err := s.GetToken(registry, username, password, namespace, "connection-test"); err != nil {
		result.LoginMessage = err.Error()
	} else {
		result.LoginOK = true
		result.LoginMessage = "登录凭证可用（可推送/拉取镜像）"
	}

	// 管理面：AK/SK 签名查询组织内仓库（limit=1，仅验证连通与权限）
	if secretKey == "" {
		result.ManageSkipped = true
		result.ManageMessage = "未配置 AK/SK（可选；不配置则「从仓库导入」不可用）"
		return result
	}
	if err := s.testManageAccess(registry, username, accessKey, secretKey, namespace); err != nil {
		result.ManageMessage = err.Error()
	} else {
		result.ManageOK = true
		result.ManageMessage = "AK/SK 可用（可获取镜像列表）"
	}
	return result
}

// testManageAccess 用 AK/SK 签名调用管理面验证连通性（GET /v2/manage/repos?namespace=xx&limit=1）
func (s *SwrAPIService) testManageAccess(registry, username, accessKey, secretKey, namespace string) error {
	region := swrRegion(registry, username)
	if region == "" {
		return fmt.Errorf("无法从仓库地址 %s 或用户名 %s 推断 SWR 区域", registry, username)
	}
	host := fmt.Sprintf("swr-api.%s.myhuaweicloud.com", region)
	ak := accessKey
	if ak == "" {
		ak = swrAKFromUsername(username)
	}

	u := &url.URL{
		Scheme:   "https",
		Host:     host,
		Path:     "/v2/manage/repos",
		RawQuery: url.Values{"namespace": []string{namespace}, "limit": []string{"1"}}.Encode(),
	}
	req, err := http.NewRequest(http.MethodGet, u.String(), nil)
	if err != nil {
		return err
	}
	swrSigner{AccessKey: ak, SecretKey: secretKey}.sign(req, time.Now())

	resp, err := (&http.Client{Timeout: 30 * time.Second}).Do(req)
	if err != nil {
		return fmt.Errorf("请求 SWR 管理面失败: %w", err)
	}
	defer resp.Body.Close()
	body, _ := io.ReadAll(io.LimitReader(resp.Body, 4096))

	switch resp.StatusCode {
	case 200:
		return nil
	case 401:
		return fmt.Errorf("AK/SK 签名验证失败（请检查 Access Key / Secret Key）")
	case 404:
		return fmt.Errorf("组织 %s 不存在", namespace)
	default:
		detail := string(body)
		if len(detail) > 200 {
			detail = detail[:200]
		}
		return fmt.Errorf("HTTP %d: %s", resp.StatusCode, detail)
	}
}

// swrRegion 从仓库地址（swr.{region}.myhuaweicloud.com）或用户名（{region}@AK）推断区域
func swrRegion(registry, username string) string {
	if parts := strings.Split(registry, "."); len(parts) >= 2 && parts[0] == "swr" {
		return parts[1]
	}
	if idx := strings.Index(username, "@"); idx > 0 {
		return username[:idx]
	}
	return ""
}
