package services

import (
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func newTestAcrService() *AcrAPIService {
	s := NewAcrAPIService()
	s.client = svcTLSRestyClient()
	return s
}

func TestGetRegionFromRegistry(t *testing.T) {
	assert.Equal(t, "cn-hangzhou", getRegionFromRegistry("registry.cn-hangzhou.aliyuncs.com"))
	assert.Equal(t, "cn-hangzhou", getRegionFromRegistry("crpi-abc123.cn-hangzhou.personal.cr.aliyuncs.com"))
	assert.Equal(t, "us-west-1", getRegionFromRegistry("registry.us-west-1.aliyuncs.com"))
	assert.Equal(t, "eu-central-1", getRegionFromRegistry("registry.eu-central-1.aliyuncs.com"))
	assert.Equal(t, "ap-southeast-1", getRegionFromRegistry("registry.ap-southeast-1.aliyuncs.com"))
	assert.Equal(t, "cn-hangzhou", getRegionFromRegistry("example.com"), "无 region 时回退默认值")
}

func TestGetDefaultAuthServer(t *testing.T) {
	assert.Equal(t, "dockerauth.cn-hangzhou.aliyuncs.com",
		getDefaultAuthServer("registry.cn-hangzhou.aliyuncs.com"))
	assert.Equal(t, "dockerauth.us-west-1.aliyuncs.com",
		getDefaultAuthServer("registry.us-west-1.aliyuncs.com"))
}

func TestAcrTokenCacheKeyAndInvalidate(t *testing.T) {
	assert.Equal(t, "reg:ns/repo", tokenCacheKey("reg", "ns", "repo"))
	s := newTestAcrService()
	s.tokenCache["reg:ns/repo"] = &tokenCacheItem{token: "tk"}
	s.invalidateTokenCache("reg", "ns", "repo")
	assert.NotContains(t, s.tokenCache, "reg:ns/repo")
}

// newAcrAuthServer 构造 ACR token 端点（POST /auth）
func newAcrAuthServer(t *testing.T, status int, body string) *httptest.Server {
	t.Helper()
	var ts *httptest.Server
	ts = newSvcTLSServer(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		require.Equal(t, "/auth", r.URL.Path)
		require.Equal(t, http.MethodPost, r.Method)
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(status)
		_, _ = w.Write([]byte(body))
	}))
	return ts
}

func TestAcrGetToken(t *testing.T) {
	t.Run("成功并缓存", func(t *testing.T) {
		var hits int32
		var ts *httptest.Server
		var gotAuth, gotScope, gotService string
		ts = newSvcTLSServer(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			atomic.AddInt32(&hits, 1)
			gotAuth = r.Header.Get("Authorization")
			gotScope = r.PostFormValue("scope")
			gotService = r.PostFormValue("service")
			w.Header().Set("Content-Type", "application/json")
			_, _ = w.Write([]byte(`{"token":"atk","expires_in":7200}`))
		}))
		s := newTestAcrService()
		host := registryHostOf(ts)

		tok, err := s.GetToken("registry.example.com", "user", "pass", "ns", "repo", host, "")
		require.NoError(t, err)
		assert.Equal(t, "atk", tok)
		// Basic 认证 + form 表单
		assert.True(t, strings.HasPrefix(gotAuth, "Basic "))
		assert.Equal(t, "repository:ns/repo:pull", gotScope)
		assert.Equal(t, "registry.aliyuncs.com:cn-hangzhou:26842", gotService,
			"dockerService 未配置时使用默认值")

		// 第二次命中缓存
		tok2, err := s.GetToken("registry.example.com", "user", "pass", "ns", "repo", host, "")
		require.NoError(t, err)
		assert.Equal(t, "atk", tok2)
		assert.Equal(t, int32(1), atomic.LoadInt32(&hits))
	})

	t.Run("authServer缺省时自动推断", func(t *testing.T) {
		// authServer 传空 → getDefaultAuthServer(registry) 推断出 dockerauth.{region}.aliyuncs.com
		// 用 stub RoundTripper 拦截，避免访问真实外网
		var gotURL string
		s := newTestAcrService()
		s.client.SetTransport(roundTripperFunc(func(req *http.Request) (*http.Response, error) {
			gotURL = req.URL.String()
			return cannedJSONResponse(req, http.StatusOK, `{"token":"atk-default","expires_in":7200}`), nil
		}))

		tok, err := s.GetToken("registry.cn-hangzhou.aliyuncs.com", "u", "p", "ns", "repo", "", "")
		require.NoError(t, err)
		assert.Equal(t, "atk-default", tok)
		assert.True(t, strings.HasPrefix(gotURL, "https://dockerauth.cn-hangzhou.aliyuncs.com/auth"),
			"应请求推断出的默认认证服务器，实际 %s", gotURL)
	})

	t.Run("自定义service", func(t *testing.T) {
		var ts *httptest.Server
		var gotService string
		ts = newSvcTLSServer(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			require.NoError(t, r.ParseForm())
			gotService = r.PostFormValue("service")
			w.Header().Set("Content-Type", "application/json")
			_, _ = w.Write([]byte(`{"token":"atk2"}`))
		}))
		s := newTestAcrService()
		_, err := s.GetToken("reg.example.com", "u", "p", "ns", "repo", registryHostOf(ts), "my-service:cn-x:1")
		require.NoError(t, err)
		assert.Equal(t, "my-service:cn-x:1", gotService)
	})

	t.Run("非200", func(t *testing.T) {
		ts := newAcrAuthServer(t, http.StatusUnauthorized, `{}`)
		s := newTestAcrService()
		_, err := s.GetToken("reg.example.com", "u", "p", "ns", "repo", registryHostOf(ts), "")
		require.Error(t, err)
		assert.Contains(t, err.Error(), "获取Token失败: HTTP 401")
	})

	t.Run("空Token", func(t *testing.T) {
		ts := newAcrAuthServer(t, http.StatusOK, `{"token":""}`)
		s := newTestAcrService()
		_, err := s.GetToken("reg.example.com", "u", "p", "ns", "repo", registryHostOf(ts), "")
		require.Error(t, err)
		assert.Contains(t, err.Error(), "返回空Token")
	})

	t.Run("网络错误", func(t *testing.T) {
		s := newTestAcrService()
		_, err := s.GetToken("reg.example.com", "u", "p", "ns", "repo", "127.0.0.1:1", "")
		require.Error(t, err)
		assert.Contains(t, err.Error(), "获取Token失败")
	})
}

// newAcrFullServer ACR 数据面 + token 端点：/auth 发 token，/v2/... 按 tagsStatus 响应
func newAcrFullServer(t *testing.T, tagsStatus int, tagsBody string) *httptest.Server {
	t.Helper()
	var ts *httptest.Server
	ts = newSvcTLSServer(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case r.URL.Path == "/auth":
			w.Header().Set("Content-Type", "application/json")
			_, _ = w.Write([]byte(`{"token":"atk","expires_in":7200}`))
		default:
			if tagsStatus != http.StatusOK {
				w.WriteHeader(tagsStatus)
			}
			w.Header().Set("Content-Type", "application/json")
			_, _ = w.Write([]byte(tagsBody))
		}
	}))
	return ts
}

func TestAcrGetTags(t *testing.T) {
	t.Run("成功", func(t *testing.T) {
		var ts *httptest.Server
		ts = newSvcTLSServer(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			switch r.URL.Path {
			case "/auth":
				w.Header().Set("Content-Type", "application/json")
				_, _ = w.Write([]byte(`{"token":"atk","expires_in":7200}`))
			case "/v2/ns/repo/tags/list":
				assert.Equal(t, "Bearer atk", r.Header.Get("Authorization"))
				w.Header().Set("Content-Type", "application/json")
				_, _ = w.Write([]byte(`{"name":"ns/repo","tags":["v1","v2"]}`))
			}
		}))
		s := newTestAcrService()
		tags, err := s.GetTags(registryHostOf(ts), "u", "p", "ns", "repo", registryHostOf(ts), "")
		require.NoError(t, err)
		assert.Equal(t, []string{"v1", "v2"}, tags)

		// GetTagNames 为别名
		tags2, err := s.GetTagNames(registryHostOf(ts), "u", "p", "ns", "repo", registryHostOf(ts), "")
		require.NoError(t, err)
		assert.Equal(t, []string{"v1", "v2"}, tags2)
	})

	t.Run("404返回空列表", func(t *testing.T) {
		ts := newAcrFullServer(t, http.StatusNotFound, `{}`)
		s := newTestAcrService()
		tags, err := s.GetTags(registryHostOf(ts), "u", "p", "ns", "repo", registryHostOf(ts), "")
		require.NoError(t, err)
		assert.Empty(t, tags)
	})

	t.Run("401后刷新token重试成功", func(t *testing.T) {
		var ts *httptest.Server
		var tokenCalls, tagsCalls int32
		ts = newSvcTLSServer(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			switch r.URL.Path {
			case "/auth":
				n := atomic.AddInt32(&tokenCalls, 1)
				tok := "stale"
				if n >= 2 {
					tok = "fresh"
				}
				w.Header().Set("Content-Type", "application/json")
				_, _ = w.Write([]byte(fmt.Sprintf(`{"token":%q,"expires_in":7200}`, tok)))
			case "/v2/ns/repo/tags/list":
				if r.Header.Get("Authorization") == "Bearer stale" {
					atomic.AddInt32(&tagsCalls, 1)
					w.WriteHeader(http.StatusUnauthorized)
					return
				}
				w.Header().Set("Content-Type", "application/json")
				_, _ = w.Write([]byte(`{"name":"ns/repo","tags":["v1"]}`))
			}
		}))
		s := newTestAcrService()
		tags, err := s.GetTags(registryHostOf(ts), "u", "p", "ns", "repo", registryHostOf(ts), "")
		require.NoError(t, err)
		assert.Equal(t, []string{"v1"}, tags)
		assert.Equal(t, int32(2), atomic.LoadInt32(&tokenCalls))
		assert.Equal(t, int32(1), atomic.LoadInt32(&tagsCalls))
	})

	t.Run("持续401最终失败", func(t *testing.T) {
		ts := newAcrFullServer(t, http.StatusUnauthorized, `{}`)
		s := newTestAcrService()
		_, err := s.GetTags(registryHostOf(ts), "u", "p", "ns", "repo", registryHostOf(ts), "")
		require.Error(t, err)
		assert.Contains(t, err.Error(), "HTTP 401")
	})

	t.Run("500", func(t *testing.T) {
		ts := newAcrFullServer(t, http.StatusInternalServerError, `oops`)
		s := newTestAcrService()
		_, err := s.GetTags(registryHostOf(ts), "u", "p", "ns", "repo", registryHostOf(ts), "")
		require.Error(t, err)
		assert.Contains(t, err.Error(), "HTTP 500")
	})

	t.Run("数据面网络错误", func(t *testing.T) {
		// token 正常，tags 端点不可达：registry 指向 127.0.0.1:1
		ts := newAcrAuthServer(t, http.StatusOK, `{"token":"atk","expires_in":7200}`)
		s := newTestAcrService()
		_, err := s.GetTags("127.0.0.1:1", "u", "p", "ns", "repo", registryHostOf(ts), "")
		require.Error(t, err)
		assert.Contains(t, err.Error(), "获取Tag列表失败")
	})

	t.Run("token失败上抛", func(t *testing.T) {
		ts := newAcrAuthServer(t, http.StatusForbidden, `{}`)
		s := newTestAcrService()
		_, err := s.GetTags(registryHostOf(ts), "u", "p", "ns", "repo", registryHostOf(ts), "")
		require.Error(t, err)
		assert.Contains(t, err.Error(), "获取Token失败: HTTP 403")
	})
}

func TestAcrRepositoryExists(t *testing.T) {
	s := newTestAcrService()
	ts := newAcrFullServer(t, http.StatusOK, `{"name":"ns/repo","tags":["v1"]}`)
	ok, err := s.RepositoryExists(registryHostOf(ts), "u", "p", "ns", "repo", registryHostOf(ts), "")
	require.NoError(t, err)
	assert.True(t, ok)

	ts2 := newAcrFullServer(t, http.StatusOK, `{"name":"ns/repo","tags":[]}`)
	ok2, err2 := s.RepositoryExists(registryHostOf(ts2), "u", "p", "ns", "repo", registryHostOf(ts2), "")
	require.NoError(t, err2)
	assert.False(t, ok2)

	ts3 := newAcrFullServer(t, http.StatusInternalServerError, `{}`)
	_, err3 := s.RepositoryExists(registryHostOf(ts3), "u", "p", "ns", "repo", registryHostOf(ts3), "")
	require.Error(t, err3)
}

func TestAcrGetTagDetail(t *testing.T) {
	var ts *httptest.Server
	ts = newSvcTLSServer(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/auth":
			w.Header().Set("Content-Type", "application/json")
			_, _ = w.Write([]byte(`{"token":"atk","expires_in":7200}`))
		case "/v2/ns/repo/manifests/v1":
			w.Header().Set("Docker-Content-Digest", "sha256:acr-manifest")
			w.Header().Set("Last-Modified", "Mon, 02 Jan 2006 15:04:05 GMT")
			w.Header().Set("Content-Type", "application/json")
			_, _ = w.Write([]byte(rcManifestList))
		default:
			w.Header().Set("Last-Modified", "Mon, 02 Jan 2006 15:04:05 GMT")
			w.WriteHeader(http.StatusOK)
		}
	}))
	s := newTestAcrService()
	host := registryHostOf(ts)

	detail, err := s.GetTagDetail(host, "u", "p", "ns", "repo", "v1", host, "")
	require.NoError(t, err)
	assert.Equal(t, "v1", detail.Tag)
	assert.Equal(t, []string{"amd64", "arm64"}, detail.Architectures)
	// 多架构 manifest：Digests 记录各架构子 manifest 的 digest
	assert.Equal(t, "sha256:aaa", detail.Digests["amd64"])
	assert.Equal(t, "sha256:bbb", detail.Digests["arm64"])
	assert.Equal(t, "2006-01-02T15:04:05Z", detail.PushedAt["amd64"])

	// manifest 失败分支
	detail2, err := s.GetTagDetail(host, "u", "p", "ns", "repo", "missing", host, "")
	require.Error(t, err) // handler 对未知 manifest 返回空 200 → JSON 解析失败
	require.Nil(t, detail2)
}

func TestAcrGetTagsDetailsBatch(t *testing.T) {
	t.Run("空列表", func(t *testing.T) {
		s := newTestAcrService()
		details, err := s.GetTagsDetailsBatch("reg", "u", "p", "ns", "repo", "", "", nil)
		require.NoError(t, err)
		assert.Empty(t, details)
	})

	t.Run("超过上限", func(t *testing.T) {
		s := newTestAcrService()
		many := make([]string, tagDetailsBatchMaxSize+1)
		for i := range many {
			many[i] = fmt.Sprintf("v%d", i)
		}
		_, err := s.GetTagsDetailsBatch("reg", "u", "p", "ns", "repo", "", "", many)
		require.Error(t, err)
		assert.Contains(t, err.Error(), "单次最多查询")
	})

	t.Run("初始token失败", func(t *testing.T) {
		ts := newAcrAuthServer(t, http.StatusUnauthorized, `{}`)
		s := newTestAcrService()
		_, err := s.GetTagsDetailsBatch(registryHostOf(ts), "u", "p", "ns", "repo", registryHostOf(ts), "", []string{"v1"})
		require.Error(t, err)
	})

	t.Run("成功与失败混合", func(t *testing.T) {
		var ts *httptest.Server
		ts = newSvcTLSServer(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			switch {
			case r.URL.Path == "/auth":
				w.Header().Set("Content-Type", "application/json")
				_, _ = w.Write([]byte(`{"token":"atk","expires_in":7200}`))
			case r.URL.Path == "/v2/ns/repo/manifests/bad":
				w.WriteHeader(http.StatusInternalServerError)
			case strings.HasPrefix(r.URL.Path, "/v2/ns/repo/manifests/"):
				w.Header().Set("Docker-Content-Digest", "sha256:ok")
				w.Header().Set("Content-Type", "application/json")
				_, _ = w.Write([]byte(rcManifestList))
			default:
				w.Header().Set("Last-Modified", "Mon, 02 Jan 2006 15:04:05 GMT")
				w.WriteHeader(http.StatusOK)
			}
		}))
		s := newTestAcrService()
		host := registryHostOf(ts)
		details, err := s.GetTagsDetailsBatch(host, "u", "p", "ns", "repo", host, "", []string{"ok1", "bad"})
		require.NoError(t, err)
		require.Len(t, details, 2)
		assert.Equal(t, []string{"amd64", "arm64"}, details[0].Architectures)
		assert.Equal(t, "bad", details[1].Tag)
		assert.Empty(t, details[1].Architectures)
	})
}

func TestAcrGetTagsWithDetails(t *testing.T) {
	var ts *httptest.Server
	ts = newSvcTLSServer(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case r.URL.Path == "/auth":
			w.Header().Set("Content-Type", "application/json")
			_, _ = w.Write([]byte(`{"token":"atk","expires_in":7200}`))
		case r.URL.Path == "/v2/ns/repo/tags/list":
			w.Header().Set("Content-Type", "application/json")
			_, _ = w.Write([]byte(`{"name":"ns/repo","tags":["v1"]}`))
		case r.URL.Path == "/v2/ns/repo/manifests/v1":
			w.Header().Set("Docker-Content-Digest", "sha256:ok")
			w.Header().Set("Content-Type", "application/json")
			_, _ = w.Write([]byte(rcManifestList))
		default:
			w.Header().Set("Last-Modified", "Mon, 02 Jan 2006 15:04:05 GMT")
			w.WriteHeader(http.StatusOK)
		}
	}))
	s := newTestAcrService()
	host := registryHostOf(ts)
	details, err := s.GetTagsWithDetails(host, "u", "p", "ns", "repo", host, "")
	require.NoError(t, err)
	require.Len(t, details, 1)
	assert.Equal(t, []string{"amd64", "arm64"}, details[0].Architectures)

	// tags 拉取失败上抛
	_, err = s.GetTagsWithDetails("127.0.0.1:1", "u", "p", "ns", "repo", "127.0.0.1:1", "")
	require.Error(t, err)
}

func TestAcrListRepositories(t *testing.T) {
	t.Run("成功并按命名空间过滤", func(t *testing.T) {
		var ts *httptest.Server
		ts = newSvcTLSServer(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			switch r.URL.Path {
			case "/auth":
				w.Header().Set("Content-Type", "application/json")
				_, _ = w.Write([]byte(`{"token":"ctok","expires_in":7200}`))
			default:
				require.Equal(t, "Bearer ctok", r.Header.Get("Authorization"))
				require.Equal(t, "1000", r.URL.Query().Get("n"))
				w.Header().Set("Content-Type", "application/json")
				_, _ = w.Write([]byte(`{"repositories":["ns/app1","ns/app2","other/x"]}`))
			}
		}))
		s := newTestAcrService()
		host := registryHostOf(ts)
		repos, err := s.ListRepositories(host, "u", "p", "", "", "ns", host, "")
		require.NoError(t, err)
		assert.Equal(t, []string{"app1", "app2"}, repos)
	})

	t.Run("分页", func(t *testing.T) {
		var ts *httptest.Server
		page1 := make([]string, 0, 1000)
		for i := 0; i < 1000; i++ {
			page1 = append(page1, fmt.Sprintf("ns/r%04d", i))
		}
		var catalogCalls int32
		ts = newSvcTLSServer(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			switch r.URL.Path {
			case "/auth":
				w.Header().Set("Content-Type", "application/json")
				_, _ = w.Write([]byte(`{"token":"ctok","expires_in":7200}`))
			default:
				atomic.AddInt32(&catalogCalls, 1)
				if r.URL.Query().Get("last") == "" {
					w.Header().Set("Content-Type", "application/json")
					_, _ = w.Write([]byte(fmt.Sprintf(`{"repositories":[%s]}`, joinQuoted(page1))))
					return
				}
				assert.Equal(t, "ns/r0999", r.URL.Query().Get("last"))
				w.Header().Set("Content-Type", "application/json")
				_, _ = w.Write([]byte(`{"repositories":["ns/tail"]}`))
			}
		}))
		s := newTestAcrService()
		host := registryHostOf(ts)
		repos, err := s.ListRepositories(host, "u", "p", "", "", "ns", host, "")
		require.NoError(t, err)
		assert.Len(t, repos, 1001)
		assert.Equal(t, int32(2), atomic.LoadInt32(&catalogCalls))
	})

	t.Run("token失败", func(t *testing.T) {
		ts := newAcrAuthServer(t, http.StatusForbidden, `{}`)
		s := newTestAcrService()
		host := registryHostOf(ts)
		_, err := s.ListRepositories("reg.example.com", "u", "p", "", "", "ns", host, "")
		require.Error(t, err)
		assert.Contains(t, err.Error(), "获取Catalog Token失败")
	})

	t.Run("非200", func(t *testing.T) {
		var ts *httptest.Server
		ts = newSvcTLSServer(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			switch r.URL.Path {
			case "/auth":
				w.Header().Set("Content-Type", "application/json")
				_, _ = w.Write([]byte(`{"token":"ctok","expires_in":7200}`))
			default:
				w.WriteHeader(http.StatusForbidden)
			}
		}))
		s := newTestAcrService()
		host := registryHostOf(ts)
		_, err := s.ListRepositories(host, "u", "p", "", "", "ns", host, "")
		require.Error(t, err)
		assert.Contains(t, err.Error(), "获取仓库列表失败: HTTP 403")
	})

	t.Run("网络错误", func(t *testing.T) {
		s := newTestAcrService()
		_, err := s.ListRepositories("127.0.0.1:1", "u", "p", "", "", "ns", "127.0.0.1:1", "")
		require.Error(t, err)
		assert.Contains(t, err.Error(), "获取Catalog Token失败")
	})
}

func TestAcrTestConnection(t *testing.T) {
	ts := newAcrAuthServer(t, http.StatusOK, `{"token":"ok"}`)
	s := newTestAcrService()
	host := registryHostOf(ts)
	res := s.TestConnection("reg.example.com", "u", "p", "", "", "ns", host, "")
	assert.True(t, res.LoginOK)
	assert.Equal(t, "acr", res.RegistryType)

	ts2 := newAcrAuthServer(t, http.StatusUnauthorized, `{}`)
	s2 := newTestAcrService()
	res2 := s2.TestConnection("reg.example.com", "u", "p", "", "", "ns", registryHostOf(ts2), "")
	assert.False(t, res2.LoginOK)
	assert.Contains(t, res2.LoginMessage, "获取Token失败")
}
