package services

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// newTestCcrService 数据面走 TLS resty，管理面（callTcrAPI）走 http.DefaultTransport stub
func newTestCcrService() *CcrAPIService {
	s := NewCcrAPIService()
	s.bearerV2Client.client = svcTLSRestyClient()
	return s
}

// withCcrManageStub 拦截腾讯云 tcr API（tcr.ap-guangzhou.tencentcloudapi.com）
func withCcrManageStub(t *testing.T, h http.Handler) {
	t.Helper()
	withHTTPDefaultTransport(t, stubRoundTripper{handler: h})
}

func TestCcrCallTcrAPI(t *testing.T) {
	t.Run("请求头与响应", func(t *testing.T) {
		var gotAuth, gotAction, gotVersion, gotRegion, gotMethod, gotCT string
		var gotBody map[string]interface{}
		withCcrManageStub(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			gotAuth = r.Header.Get("Authorization")
			gotAction = r.Header.Get("X-TC-Action")
			gotVersion = r.Header.Get("X-TC-Version")
			gotRegion = r.Header.Get("X-TC-Region")
			gotMethod = r.Method
			gotCT = r.Header.Get("Content-Type")
			body, _ := io.ReadAll(r.Body)
			_ = json.Unmarshal(body, &gotBody)
			w.Header().Set("Content-Type", "application/json")
			_, _ = w.Write([]byte(`{"Response":{"Data":{}}}`))
		}))

		s := newTestCcrService()
		body, status, err := s.callTcrAPI("DescribeRepositoryFilterPersonal",
			map[string]interface{}{"Namespace": "ns", "Offset": 0, "Limit": 100}, "SID", "SK")
		require.NoError(t, err)
		assert.Equal(t, http.StatusOK, status)
		assert.Contains(t, string(body), `"Data"`)

		assert.True(t, strings.HasPrefix(gotAuth, "TC3-HMAC-SHA256 Credential=SID/"),
			"应携带 TC3 签名头，实际 %q", gotAuth)
		assert.Equal(t, "DescribeRepositoryFilterPersonal", gotAction)
		assert.Equal(t, "2019-09-24", gotVersion)
		assert.Equal(t, "ap-guangzhou", gotRegion)
		assert.Equal(t, http.MethodPost, gotMethod)
		assert.Contains(t, gotCT, "application/json")
		assert.Equal(t, "ns", gotBody["Namespace"])
		assert.Equal(t, float64(100), gotBody["Limit"])
	})

	t.Run("payload序列化失败", func(t *testing.T) {
		s := newTestCcrService()
		_, _, err := s.callTcrAPI("Act", map[string]interface{}{"f": func() {}}, "SID", "SK")
		require.Error(t, err)
	})

	t.Run("网络错误", func(t *testing.T) {
		withHTTPDefaultTransport(t, errRoundTripper{err: fmt.Errorf("boom")})
		s := newTestCcrService()
		_, _, err := s.callTcrAPI("Act", map[string]interface{}{}, "SID", "SK")
		require.Error(t, err)
		assert.Contains(t, err.Error(), "请求腾讯云 API 失败")
	})
}

func TestParseTcrResponse(t *testing.T) {
	// 正常数据
	data, err := parseTcrResponse([]byte(`{"Response":{"Data":{"RepoInfo":[],"TotalCount":0}}}`))
	require.NoError(t, err)
	assert.NotNil(t, data)

	// AuthFailure → SecretId/SecretKey 错误
	_, err = parseTcrResponse([]byte(`{"Response":{"Error":{"Code":"AuthFailure.SignatureFailure","Message":"bad sign"}}}`))
	require.Error(t, err)
	assert.Contains(t, err.Error(), "SecretId/SecretKey 验证失败")

	_, err = parseTcrResponse([]byte(`{"Response":{"Error":{"Code":"UnauthorizedOperation","Message":"no perm"}}}`))
	require.Error(t, err)
	assert.Contains(t, err.Error(), "SecretId/SecretKey 验证失败")

	// 其他业务错误
	_, err = parseTcrResponse([]byte(`{"Response":{"Error":{"Code":"InternalError","Message":"oops"}}}`))
	require.Error(t, err)
	assert.Contains(t, err.Error(), "腾讯云 API 错误")

	// 畸形 JSON
	_, err = parseTcrResponse([]byte(`{bad`))
	require.Error(t, err)
	assert.Contains(t, err.Error(), "解析腾讯云响应失败")
}

func TestCcrListRepositories(t *testing.T) {
	t.Run("未配置SecretKey", func(t *testing.T) {
		s := newTestCcrService()
		_, err := s.ListRepositories("ccr.ccs.tencentyun.com", "u", "p", "", "", "ns", "", "")
		require.Error(t, err)
		assert.Contains(t, err.Error(), "未配置腾讯云管理面 SecretId/SecretKey")
	})

	t.Run("成功单页", func(t *testing.T) {
		withCcrManageStub(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			w.Header().Set("Content-Type", "application/json")
			_, _ = w.Write([]byte(`{"Response":{"Data":{"RepoInfo":[{"RepoName":"ns/app1"},{"RepoName":"ns/app2"},{"RepoName":"plain"},{"RepoName":"ns/  "}],"TotalCount":3}}}`))
		}))
		s := newTestCcrService()
		repos, err := s.ListRepositories("ccr.ccs.tencentyun.com", "u", "p", "SID", "SK", "ns", "", "")
		require.NoError(t, err)
		// "命名空间/仓库" 去前缀，空白跳过
		assert.Equal(t, []string{"app1", "app2", "plain"}, repos)
	})

	t.Run("分页", func(t *testing.T) {
		page1 := make([]string, 0, 100)
		for i := 0; i < 100; i++ {
			page1 = append(page1, fmt.Sprintf(`{"RepoName":%q}`, fmt.Sprintf("ns/r%03d", i)))
		}
		var offsets []float64
		withCcrManageStub(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			body, _ := io.ReadAll(r.Body)
			var payload map[string]interface{}
			_ = json.Unmarshal(body, &payload)
			offsets = append(offsets, payload["Offset"].(float64))
			if payload["Offset"].(float64) == 0 {
				w.Header().Set("Content-Type", "application/json")
				_, _ = w.Write([]byte(fmt.Sprintf(`{"Response":{"Data":{"RepoInfo":[%s],"TotalCount":150}}}`, strings.Join(page1, ","))))
				return
			}
			w.Header().Set("Content-Type", "application/json")
			// 第二页返回剩余 2 个，TotalCount 标记为累计已获取数以结束分页
			_, _ = w.Write([]byte(`{"Response":{"Data":{"RepoInfo":[{"RepoName":"ns/tail1"},{"RepoName":"ns/tail2"}],"TotalCount":102}}}`))
		}))
		s := newTestCcrService()
		repos, err := s.ListRepositories("ccr.ccs.tencentyun.com", "u", "p", "SID", "SK", "ns", "", "")
		require.NoError(t, err)
		assert.Len(t, repos, 102)
		require.Len(t, offsets, 2)
		assert.Equal(t, float64(0), offsets[0])
		assert.Equal(t, float64(100), offsets[1])
	})

	t.Run("空页立即停止", func(t *testing.T) {
		var calls int
		withCcrManageStub(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			calls++
			w.Header().Set("Content-Type", "application/json")
			_, _ = w.Write([]byte(`{"Response":{"Data":{"RepoInfo":[],"TotalCount":99}}}`))
		}))
		s := newTestCcrService()
		repos, err := s.ListRepositories("ccr.ccs.tencentyun.com", "u", "p", "SID", "SK", "ns", "", "")
		require.NoError(t, err)
		assert.Empty(t, repos)
		assert.Equal(t, 1, calls, "RepoInfo 为空应立即结束分页")
	})

	t.Run("HTTP非200", func(t *testing.T) {
		withCcrManageStub(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			w.WriteHeader(http.StatusForbidden)
			_, _ = w.Write([]byte(strings.Repeat("z", 300)))
		}))
		s := newTestCcrService()
		_, err := s.ListRepositories("ccr.ccs.tencentyun.com", "u", "p", "SID", "SK", "ns", "", "")
		require.Error(t, err)
		assert.Contains(t, err.Error(), "获取 CCR 仓库列表失败: HTTP 403")
	})

	t.Run("网络错误", func(t *testing.T) {
		withHTTPDefaultTransport(t, errRoundTripper{err: fmt.Errorf("down")})
		s := newTestCcrService()
		_, err := s.ListRepositories("ccr.ccs.tencentyun.com", "u", "p", "SID", "SK", "ns", "", "")
		require.Error(t, err)
		assert.Contains(t, err.Error(), "请求腾讯云 API 失败")
	})

	t.Run("响应解析失败", func(t *testing.T) {
		withCcrManageStub(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			w.Header().Set("Content-Type", "application/json")
			_, _ = w.Write([]byte(`{bad json`))
		}))
		s := newTestCcrService()
		_, err := s.ListRepositories("ccr.ccs.tencentyun.com", "u", "p", "SID", "SK", "ns", "", "")
		require.Error(t, err)
		assert.Contains(t, err.Error(), "解析腾讯云响应失败")
	})

	t.Run("Data字段非法", func(t *testing.T) {
		withCcrManageStub(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			w.Header().Set("Content-Type", "application/json")
			_, _ = w.Write([]byte(`{"Response":{"Data":"not-an-object"}}`))
		}))
		s := newTestCcrService()
		_, err := s.ListRepositories("ccr.ccs.tencentyun.com", "u", "p", "SID", "SK", "ns", "", "")
		require.Error(t, err)
		assert.Contains(t, err.Error(), "解析 CCR 仓库列表失败")
	})
}

// newCcrDataPlaneOKService 数据面可用的 CCR 服务（/v2/ challenge + /token）
func newCcrDataPlaneOKService(t *testing.T) (*CcrAPIService, string) {
	t.Helper()
	var ts *httptest.Server
	ts = newSvcTLSServer(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/v2/":
			w.Header().Set("WWW-Authenticate", fmt.Sprintf(`Bearer realm="%s/service/token",service="token-service"`, ts.URL))
			w.WriteHeader(http.StatusUnauthorized)
		case "/service/token":
			w.Header().Set("Content-Type", "application/json")
			_, _ = w.Write([]byte(`{"token":"ccr-tok","expires_in":7200}`))
		default:
			w.WriteHeader(http.StatusOK)
		}
	}))
	s := newTestCcrService()
	return s, registryHostOf(ts)
}

func TestCcrTestConnection(t *testing.T) {
	t.Run("数据面失败", func(t *testing.T) {
		s := newTestCcrService()
		ts := newSvcTLSServer(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			w.WriteHeader(http.StatusOK) // 无 challenge
		}))
		res := s.TestConnection(registryHostOf(ts), "u", "p", "", "", "ns", "", "")
		require.NotNil(t, res)
		assert.False(t, res.LoginOK)
		assert.Equal(t, "ccr", res.RegistryType)
	})

	t.Run("未配置SecretKey跳过管理面", func(t *testing.T) {
		withCcrManageStub(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			t.Error("未配置 SecretKey 不应请求管理面")
		}))
		s, registry := newCcrDataPlaneOKService(t)
		res := s.TestConnection(registry, "10001", "p", "", "", "ns", "", "")
		assert.True(t, res.LoginOK)
		assert.True(t, res.ManageSkipped)
		assert.Contains(t, res.ManageMessage, "未配置 SecretId/SecretKey")
	})

	t.Run("管理面可用", func(t *testing.T) {
		withCcrManageStub(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			w.Header().Set("Content-Type", "application/json")
			_, _ = w.Write([]byte(`{"Response":{"Data":{"RepoInfo":[],"TotalCount":0}}}`))
		}))
		s, registry := newCcrDataPlaneOKService(t)
		res := s.TestConnection(registry, "10001", "p", "SID", "SK", "ns", "", "")
		assert.True(t, res.LoginOK)
		assert.True(t, res.ManageOK)
	})

	t.Run("管理面HTTP错误", func(t *testing.T) {
		withCcrManageStub(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			w.WriteHeader(http.StatusForbidden)
		}))
		s, registry := newCcrDataPlaneOKService(t)
		res := s.TestConnection(registry, "10001", "p", "SID", "SK", "ns", "", "")
		assert.True(t, res.LoginOK)
		assert.False(t, res.ManageOK)
		assert.Contains(t, res.ManageMessage, "HTTP 403")
	})

	t.Run("管理面凭证错误", func(t *testing.T) {
		withCcrManageStub(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			w.Header().Set("Content-Type", "application/json")
			_, _ = w.Write([]byte(`{"Response":{"Error":{"Code":"AuthFailure","Message":"bad key"}}}`))
		}))
		s, registry := newCcrDataPlaneOKService(t)
		res := s.TestConnection(registry, "10001", "p", "SID", "SK", "ns", "", "")
		assert.Contains(t, res.ManageMessage, "SecretId/SecretKey 验证失败")
	})
}
