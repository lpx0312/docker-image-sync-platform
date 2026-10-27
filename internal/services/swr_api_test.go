package services

import (
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// newTestSwrService 数据面走 TLS resty，管理面走 http.DefaultTransport stub
func newTestSwrService() *SwrAPIService {
	s := NewSwrAPIService()
	s.bearerV2Client.client = svcTLSRestyClient()
	return s
}

func TestParseSwrRepoList(t *testing.T) {
	// 数组形态
	names, err := parseSwrRepoList([]byte(`[{"name":"app1"},{"path":"group/app2"}]`))
	require.NoError(t, err)
	assert.Equal(t, []string{"app1", "group/app2"}, names)

	// {"body":[...]} 包装形态
	names, err = parseSwrRepoList([]byte(`{"body":[{"name":"b1"},{"path":"b2"}]}`))
	require.NoError(t, err)
	assert.Equal(t, []string{"b1", "b2"}, names)

	// name 优先于 path
	names, err = parseSwrRepoList([]byte(`[{"name":"n","path":"p"}]`))
	require.NoError(t, err)
	assert.Equal(t, []string{"n"}, names)

	// 畸形 JSON
	_, err = parseSwrRepoList([]byte(`{bad`))
	require.Error(t, err)
	assert.Contains(t, err.Error(), "解析 SWR 仓库列表响应失败")
}

func TestSwrRegionInference(t *testing.T) {
	assert.Equal(t, "cn-east-3", swrRegion("swr.cn-east-3.myhuaweicloud.com", ""))
	assert.Equal(t, "cn-north-4", swrRegion("other.example.com", "cn-north-4@AKXXX"))
	assert.Empty(t, swrRegion("other.example.com", "plainuser"))
	assert.Empty(t, swrRegion("@lead", "")) // @ 前为空不算
}

// withSwrManageStub 用指定 handler 拦截 SWR 管理面请求（swr-api.{region}.myhuaweicloud.com）
func withSwrManageStub(t *testing.T, h http.Handler) {
	t.Helper()
	withHTTPDefaultTransport(t, stubRoundTripper{handler: h})
}

func TestSwrListRepositories(t *testing.T) {
	t.Run("未配置SK", func(t *testing.T) {
		s := newTestSwrService()
		_, err := s.ListRepositories("swr.cn-east-3.myhuaweicloud.com", "u", "p", "", "", "ns", "", "")
		require.Error(t, err)
		assert.Contains(t, err.Error(), "未配置 SWR 管理面 Secret Key")
	})

	t.Run("无法推断区域", func(t *testing.T) {
		s := newTestSwrService()
		_, err := s.ListRepositories("registry.example.com", "plainuser", "p", "ak", "sk", "ns", "", "")
		require.Error(t, err)
		assert.Contains(t, err.Error(), "无法从仓库地址")
	})

	t.Run("成功单页", func(t *testing.T) {
		var gotPath, gotQuery, gotAuth string
		withSwrManageStub(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			gotPath = r.URL.Path
			gotQuery = r.URL.RawQuery
			gotAuth = r.Header.Get("Authorization")
			w.Header().Set("Content-Type", "application/json")
			_, _ = w.Write([]byte(`[{"name":"ns/app1"},{"name":"ns/app2"},{"name":""},{"name":"  "}]`))
		}))
		s := newTestSwrService()
		repos, err := s.ListRepositories("swr.cn-east-3.myhuaweicloud.com", "u", "p", "AK123", "SK456", "ns", "", "")
		require.NoError(t, err)
		// name 取 "/" 之后一段，空/空白跳过
		assert.Equal(t, []string{"app1", "app2"}, repos)
		assert.Equal(t, "/v2/manage/repos", gotPath)
		assert.Contains(t, gotQuery, "namespace=ns")
		assert.Contains(t, gotQuery, "limit=100")
		assert.Contains(t, gotQuery, "offset=0")
		assert.Contains(t, gotAuth, "SDK-HMAC-SHA256 Access=AK123")
	})

	t.Run("AK缺省时从用户名提取", func(t *testing.T) {
		var gotAuth string
		withSwrManageStub(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			gotAuth = r.Header.Get("Authorization")
			w.Header().Set("Content-Type", "application/json")
			_, _ = w.Write([]byte(`[]`))
		}))
		s := newTestSwrService()
		repos, err := s.ListRepositories("swr.cn-east-3.myhuaweicloud.com", "cn-east-3@USERAK", "p", "", "SK", "ns", "", "")
		require.NoError(t, err)
		assert.Empty(t, repos)
		assert.Contains(t, gotAuth, "Access=USERAK")
	})

	t.Run("分页", func(t *testing.T) {
		page1Objs := make([]string, 0, 100)
		for i := 0; i < 100; i++ {
			page1Objs = append(page1Objs, fmt.Sprintf(`{"name":%q}`, fmt.Sprintf("ns/r%03d", i)))
		}
		var offsets []string
		withSwrManageStub(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			offsets = append(offsets, r.URL.Query().Get("offset"))
			if r.URL.Query().Get("offset") == "0" {
				w.Header().Set("Content-Type", "application/json")
				_, _ = w.Write([]byte("[" + strings.Join(page1Objs, ",") + "]"))
				return
			}
			w.Header().Set("Content-Type", "application/json")
			_, _ = w.Write([]byte(`[{"name":"ns/tail"}]`))
		}))
		s := newTestSwrService()
		repos, err := s.ListRepositories("swr.cn-east-3.myhuaweicloud.com", "u", "p", "AK", "SK", "ns", "", "")
		require.NoError(t, err)
		assert.Len(t, repos, 101)
		assert.Equal(t, "tail", repos[len(repos)-1])
		assert.Equal(t, []string{"0", "100"}, offsets)
	})

	t.Run("401签名验证失败", func(t *testing.T) {
		withSwrManageStub(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			w.WriteHeader(http.StatusUnauthorized)
		}))
		s := newTestSwrService()
		_, err := s.ListRepositories("swr.cn-east-3.myhuaweicloud.com", "u", "p", "AK", "SK", "ns", "", "")
		require.Error(t, err)
		assert.Contains(t, err.Error(), "AK/SK 签名验证失败")
	})

	t.Run("非200带截断详情", func(t *testing.T) {
		withSwrManageStub(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			w.WriteHeader(http.StatusInternalServerError)
			_, _ = w.Write([]byte(strings.Repeat("x", 500)))
		}))
		s := newTestSwrService()
		_, err := s.ListRepositories("swr.cn-east-3.myhuaweicloud.com", "u", "p", "AK", "SK", "ns", "", "")
		require.Error(t, err)
		assert.Contains(t, err.Error(), "HTTP 500")
		assert.LessOrEqual(t, len(err.Error()), len("获取 SWR 仓库列表失败: HTTP 500: ")+200+20)
	})

	t.Run("畸形响应", func(t *testing.T) {
		withSwrManageStub(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			_, _ = w.Write([]byte(`not-json`))
		}))
		s := newTestSwrService()
		_, err := s.ListRepositories("swr.cn-east-3.myhuaweicloud.com", "u", "p", "AK", "SK", "ns", "", "")
		require.Error(t, err)
		assert.Contains(t, err.Error(), "解析 SWR 仓库列表响应失败")
	})

	t.Run("网络错误", func(t *testing.T) {
		withHTTPDefaultTransport(t, errRoundTripper{err: fmt.Errorf("dial timeout")})
		s := newTestSwrService()
		_, err := s.ListRepositories("swr.cn-east-3.myhuaweicloud.com", "u", "p", "AK", "SK", "ns", "", "")
		require.Error(t, err)
		assert.Contains(t, err.Error(), "请求 SWR 管理面失败")
	})
}

// newSwrDataPlaneOKService 返回数据面可用的 SWR 服务（/v2/ challenge + /token 成功），
// registry 即测试服务地址，区域从用户名 {region}@AK 推断（管理面走 stub）
func newSwrDataPlaneOKService(t *testing.T) (*SwrAPIService, string) {
	t.Helper()
	var ts *httptest.Server
	ts = newSvcTLSServer(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/v2/":
			w.Header().Set("WWW-Authenticate", fmt.Sprintf(`Bearer realm="%s/swr/auth",service="dockyard"`, ts.URL))
			w.WriteHeader(http.StatusUnauthorized)
		case "/swr/auth":
			w.Header().Set("Content-Type", "application/json")
			_, _ = w.Write([]byte(`{"token":"swr-tok","expires_in":900}`))
		default:
			w.WriteHeader(http.StatusOK)
		}
	}))
	s := newTestSwrService()
	return s, registryHostOf(ts)
}

func TestSwrTestConnection(t *testing.T) {
	t.Run("数据面失败", func(t *testing.T) {
		s := newTestSwrService()
		ts := newSvcTLSServer(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			w.WriteHeader(http.StatusOK) // 无 challenge → 无法获取认证端点
		}))
		res := s.TestConnection(registryHostOf(ts), "u", "p", "", "", "ns", "", "")
		require.NotNil(t, res)
		assert.False(t, res.LoginOK)
		assert.NotEmpty(t, res.LoginMessage)
		assert.Equal(t, "swr", res.RegistryType)
	})

	t.Run("未配置AK/SK时跳过管理面", func(t *testing.T) {
		withSwrManageStub(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			t.Error("未配置 SK 不应请求管理面")
		}))
		s, registry := newSwrDataPlaneOKService(t)
		res := s.TestConnection(registry, "cn-east-3@AK", "p", "", "", "ns", "", "")
		assert.True(t, res.LoginOK)
		assert.True(t, res.ManageSkipped)
		assert.False(t, res.ManageOK)
		assert.Contains(t, res.ManageMessage, "未配置 AK/SK")
	})

	t.Run("管理面可用", func(t *testing.T) {
		withSwrManageStub(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			require.Equal(t, "/v2/manage/repos", r.URL.Path)
			require.Equal(t, "1", r.URL.Query().Get("limit"))
			w.Header().Set("Content-Type", "application/json")
			_, _ = w.Write([]byte(`[]`))
		}))
		s, registry := newSwrDataPlaneOKService(t)
		res := s.TestConnection(registry, "cn-east-3@AK", "p", "AK1", "SK1", "ns", "", "")
		assert.True(t, res.LoginOK)
		assert.True(t, res.ManageOK)
		assert.False(t, res.ManageSkipped)
	})

	t.Run("管理面签名失败", func(t *testing.T) {
		withSwrManageStub(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			w.WriteHeader(http.StatusUnauthorized)
		}))
		s, registry := newSwrDataPlaneOKService(t)
		res := s.TestConnection(registry, "cn-east-3@AK", "p", "AK1", "SK1", "ns", "", "")
		assert.True(t, res.LoginOK)
		assert.False(t, res.ManageOK)
		assert.Contains(t, res.ManageMessage, "签名验证失败")
	})

	t.Run("管理面组织不存在", func(t *testing.T) {
		withSwrManageStub(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			w.WriteHeader(http.StatusNotFound)
		}))
		s, registry := newSwrDataPlaneOKService(t)
		res := s.TestConnection(registry, "cn-east-3@AK", "p", "AK1", "SK1", "ns", "", "")
		assert.Contains(t, res.ManageMessage, "组织 ns 不存在")
	})

	t.Run("管理面其他错误与截断", func(t *testing.T) {
		withSwrManageStub(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			w.WriteHeader(http.StatusBadGateway)
			_, _ = w.Write([]byte(strings.Repeat("y", 400)))
		}))
		s, registry := newSwrDataPlaneOKService(t)
		res := s.TestConnection(registry, "cn-east-3@AK", "p", "AK1", "SK1", "ns", "", "")
		assert.Contains(t, res.ManageMessage, "HTTP 502")
	})

	t.Run("管理面网络错误", func(t *testing.T) {
		withHTTPDefaultTransport(t, errRoundTripper{err: fmt.Errorf("refused")})
		s, registry := newSwrDataPlaneOKService(t)
		res := s.TestConnection(registry, "cn-east-3@AK", "p", "AK1", "SK1", "ns", "", "")
		assert.Contains(t, res.ManageMessage, "请求 SWR 管理面失败")
	})

	t.Run("管理面区域无法推断", func(t *testing.T) {
		withSwrManageStub(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			t.Error("区域推断失败不应发起管理面请求")
		}))
		s, registry := newSwrDataPlaneOKService(t)
		// registry 非 swr.* 形态、用户名无 @ → 区域推断失败
		res := s.TestConnection(registry, "plainuser", "p", "AK1", "SK1", "ns", "", "")
		assert.True(t, res.LoginOK, "数据面仍应成功（registry 即测试服务）")
		assert.False(t, res.ManageOK)
		assert.Contains(t, res.ManageMessage, "无法从仓库地址")
	})
}
