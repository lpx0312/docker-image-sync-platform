package services

import (
	"encoding/base64"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// newTestBearerClient 构造注入 TLS resty 客户端的 bearerV2Client
func newTestBearerClient(name string) *bearerV2Client {
	c := newBearerV2Client(name)
	c.client = svcTLSRestyClient()
	return c
}

func TestParseBearerChallenge(t *testing.T) {
	// 标准形式
	info := parseBearerChallenge(`Bearer realm="https://auth.example.com/token",service="registry.example.com"`)
	require.NotNil(t, info)
	assert.Equal(t, "https://auth.example.com/token", info.Realm)
	assert.Equal(t, "registry.example.com", info.Service)

	// 小写 bearer + 多余空格 + 无引号值
	info = parseBearerChallenge(`bearer realm=https://a.example.com/t, service=svc, extra`)
	require.NotNil(t, info)
	assert.Equal(t, "https://a.example.com/t", info.Realm)
	assert.Equal(t, "svc", info.Service)

	// 缺 realm → nil
	assert.Nil(t, parseBearerChallenge(`Bearer service="only-service"`))
	// 非 Bearer → nil
	assert.Nil(t, parseBearerChallenge(`Basic realm="x"`))
	assert.Nil(t, parseBearerChallenge(""))
	// 含无 '=' 的片段应被跳过且不影响解析
	info = parseBearerChallenge(`Bearer garbage, realm="https://r.example.com/t"`)
	require.NotNil(t, info)
	assert.Equal(t, "https://r.example.com/t", info.Realm)
	assert.Empty(t, info.Service)
}

func TestBearerTokenCacheKey(t *testing.T) {
	assert.Equal(t, "reg:ns/repo", bearerTokenCacheKey("reg", "ns", "repo"))
}

func TestNewBearerV2ClientDefaults(t *testing.T) {
	c := newBearerV2Client("SWR")
	assert.Equal(t, "SWR", c.name)
	assert.NotNil(t, c.client)
	assert.NotNil(t, c.tokenCache)
	assert.NotNil(t, c.authCache)
}

func TestBearerDiscoverAuthChallenge(t *testing.T) {
	// 401 + challenge → 解析并缓存（第二次不再请求）
	var hits int32
	var realmURL string
	ts := newSvcTLSServer(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		atomic.AddInt32(&hits, 1)
		w.Header().Set("WWW-Authenticate", fmt.Sprintf(`Bearer realm="%s/token",service="svc"`, realmURL))
		w.WriteHeader(http.StatusUnauthorized)
	}))
	realmURL = ts.URL
	c := newTestBearerClient("SWR")

	info := c.discoverAuth(registryHostOf(ts))
	require.NotNil(t, info)
	assert.Equal(t, ts.URL+"/token", info.Realm)
	assert.Equal(t, "svc", info.Service)

	info2 := c.discoverAuth(registryHostOf(ts))
	assert.Equal(t, info, info2)
	assert.Equal(t, int32(1), atomic.LoadInt32(&hits))
}

func TestBearerDiscoverAuthNoChallenge(t *testing.T) {
	// 200（无 challenge）→ 空信息
	ts := newSvcTLSServer(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))
	c := newTestBearerClient("SWR")
	info := c.discoverAuth(registryHostOf(ts))
	require.NotNil(t, info)
	assert.Empty(t, info.Realm)

	// 网络错误 → 空信息
	c2 := newTestBearerClient("SWR")
	info2 := c2.discoverAuth("127.0.0.1:1")
	require.NotNil(t, info2)
	assert.Empty(t, info2.Realm)
}

func TestBearerGetToken(t *testing.T) {
	t.Run("成功并缓存", func(t *testing.T) {
		var hits int32
		var captured *http.Request
		ts := newSvcTLSServer(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			atomic.AddInt32(&hits, 1)
			captured = r
			w.Header().Set("Content-Type", "application/json")
			_, _ = w.Write([]byte(`{"token":"tok-1","expires_in":7200}`))
		}))
		c := newTestBearerClient("SWR")
		c.authCache[registryHostOf(ts)] = &registryAuthInfo{Realm: ts.URL + "/token", Service: "svc"}

		tok, err := c.GetToken(registryHostOf(ts), "user", "pass", "ns", "repo")
		require.NoError(t, err)
		assert.Equal(t, "tok-1", tok)
		require.NotNil(t, captured)
		// Basic 认证头
		want := "Basic " + base64.StdEncoding.EncodeToString([]byte("user:pass"))
		assert.Equal(t, want, captured.Header.Get("Authorization"))
		// scope / service 查询参数
		q := captured.URL.Query()
		assert.Equal(t, "svc", q.Get("service"))
		assert.Equal(t, "repository:ns/repo:pull", q.Get("scope"))

		// 第二次命中缓存
		tok2, err := c.GetToken(registryHostOf(ts), "user", "pass", "ns", "repo")
		require.NoError(t, err)
		assert.Equal(t, "tok-1", tok2)
		assert.Equal(t, int32(1), atomic.LoadInt32(&hits))
	})

	t.Run("expires_in缺失按默认缓存", func(t *testing.T) {
		ts := newSvcTLSServer(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			w.Header().Set("Content-Type", "application/json")
			_, _ = w.Write([]byte(`{"token":"tok-2"}`))
		}))
		c := newTestBearerClient("SWR")
		c.authCache[registryHostOf(ts)] = &registryAuthInfo{Realm: ts.URL + "/token"}
		tok, err := c.GetToken(registryHostOf(ts), "u", "p", "ns", "repo")
		require.NoError(t, err)
		assert.Equal(t, "tok-2", tok)
		item := c.tokenCache[bearerTokenCacheKey(registryHostOf(ts), "ns", "repo")]
		require.NotNil(t, item)
		// expires_in<=0 → 600s，缓存 TTL=max(600-60,60)=540s
		assert.True(t, item.expiresAt.After(time.Now().Add(500*time.Second)))
	})

	t.Run("401认证被拒绝", func(t *testing.T) {
		ts := newSvcTLSServer(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			w.WriteHeader(http.StatusUnauthorized)
		}))
		c := newTestBearerClient("SWR")
		c.authCache[registryHostOf(ts)] = &registryAuthInfo{Realm: ts.URL + "/token"}
		_, err := c.GetToken(registryHostOf(ts), "u", "p", "ns", "repo")
		require.Error(t, err)
		assert.Contains(t, err.Error(), "获取SWR Token失败")
		assert.Contains(t, err.Error(), "认证被拒绝")
	})

	t.Run("非200状态码", func(t *testing.T) {
		ts := newSvcTLSServer(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			w.WriteHeader(http.StatusInternalServerError)
		}))
		c := newTestBearerClient("CCR")
		c.authCache[registryHostOf(ts)] = &registryAuthInfo{Realm: ts.URL + "/token"}
		_, err := c.GetToken(registryHostOf(ts), "u", "p", "ns", "repo")
		require.Error(t, err)
		assert.Contains(t, err.Error(), "获取CCR Token失败: HTTP 500")
	})

	t.Run("网络错误", func(t *testing.T) {
		c := newTestBearerClient("SWR")
		c.authCache["bad.example.com"] = &registryAuthInfo{Realm: "https://127.0.0.1:1/token"}
		_, err := c.GetToken("bad.example.com", "u", "p", "ns", "repo")
		require.Error(t, err)
		assert.Contains(t, err.Error(), "获取SWR Token失败")
	})

	t.Run("空Token", func(t *testing.T) {
		ts := newSvcTLSServer(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			w.Header().Set("Content-Type", "application/json")
			_, _ = w.Write([]byte(`{"token":"","expires_in":600}`))
		}))
		c := newTestBearerClient("SWR")
		c.authCache[registryHostOf(ts)] = &registryAuthInfo{Realm: ts.URL + "/token"}
		_, err := c.GetToken(registryHostOf(ts), "u", "p", "ns", "repo")
		require.Error(t, err)
		assert.Contains(t, err.Error(), "返回空Token")
	})

	t.Run("无认证端点", func(t *testing.T) {
		c := newTestBearerClient("SWR")
		ts := newSvcTLSServer(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			w.WriteHeader(http.StatusOK) // 无 challenge
		}))
		_, err := c.GetToken(registryHostOf(ts), "u", "p", "ns", "repo")
		require.Error(t, err)
		assert.Contains(t, err.Error(), "无法获取认证端点")
	})
}

func TestBearerInvalidateTokenCache(t *testing.T) {
	c := newTestBearerClient("SWR")
	c.tokenCache["reg:ns/repo"] = &tokenCacheItem{token: "tok", expiresAt: time.Now().Add(time.Hour)}
	c.invalidateTokenCache("reg", "ns", "repo")
	assert.NotContains(t, c.tokenCache, "reg:ns/repo")
}

// newBearerFullServer 标准三层端点：/v2/ challenge、/token 200、tags 端点自定状态与响应
func newBearerFullServer(t *testing.T, tagsBody string, tagsStatus ...int) *httptest.Server {
	t.Helper()
	status := http.StatusOK
	if len(tagsStatus) > 0 {
		status = tagsStatus[0]
	}
	var ts *httptest.Server
	ts = newSvcTLSServer(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/v2/":
			w.Header().Set("WWW-Authenticate", fmt.Sprintf(`Bearer realm="%s/token",service="svc"`, ts.URL))
			w.WriteHeader(http.StatusUnauthorized)
		case "/token":
			w.Header().Set("Content-Type", "application/json")
			_, _ = w.Write([]byte(`{"token":"tk","expires_in":7200}`))
		default:
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(status)
			_, _ = w.Write([]byte(tagsBody))
		}
	}))
	return ts
}

func TestBearerGetTags(t *testing.T) {
	t.Run("成功并过滤空白项", func(t *testing.T) {
		c := newTestBearerClient("SWR")
		var ts *httptest.Server
		ts = newSvcTLSServer(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			switch r.URL.Path {
			case "/v2/":
				w.Header().Set("WWW-Authenticate", fmt.Sprintf(`Bearer realm="%s/token",service="svc"`, ts.URL))
				w.WriteHeader(http.StatusUnauthorized)
			case "/token":
				w.Header().Set("Content-Type", "application/json")
				_, _ = w.Write([]byte(`{"token":"tk","expires_in":7200}`))
			case "/v2/ns/repo/tags/list":
				assert.Equal(t, "Bearer tk", r.Header.Get("Authorization"))
				w.Header().Set("Content-Type", "application/json")
				_, _ = w.Write([]byte(`{"name":"ns/repo","tags":["v1"," v2 ","","v3"]}`))
			default:
				t.Errorf("意外请求 %s", r.URL.Path)
			}
		}))
		tags, err := c.GetTags(registryHostOf(ts), "u", "p", "ns", "repo", "", "")
		require.NoError(t, err)
		assert.Equal(t, []string{"v1", "v2", "v3"}, tags)
	})

	t.Run("tags为null返回空列表", func(t *testing.T) {
		c := newTestBearerClient("SWR")
		ts := newBearerFullServer(t, `{"name":"ns/repo","tags":null}`)
		tags, err := c.GetTags(registryHostOf(ts), "u", "p", "ns", "repo", "", "")
		require.NoError(t, err)
		assert.Empty(t, tags)
	})

	t.Run("404返回空列表", func(t *testing.T) {
		c := newTestBearerClient("SWR")
		ts := newBearerFullServer(t, `{}`, http.StatusNotFound)
		tags, err := c.GetTags(registryHostOf(ts), "u", "p", "ns", "repo", "", "")
		require.NoError(t, err)
		assert.Empty(t, tags)
	})

	t.Run("401后刷新token重试成功", func(t *testing.T) {
		c := newTestBearerClient("SWR")
		var tokenCalls int32
		var ts *httptest.Server
		ts = newSvcTLSServer(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			switch r.URL.Path {
			case "/v2/":
				w.Header().Set("WWW-Authenticate", fmt.Sprintf(`Bearer realm="%s/token",service="svc"`, ts.URL))
				w.WriteHeader(http.StatusUnauthorized)
			case "/token":
				n := atomic.AddInt32(&tokenCalls, 1)
				token := "tk-stale"
				if n >= 2 {
					token = "tk-fresh" // 刷新后返回新 token
				}
				w.Header().Set("Content-Type", "application/json")
				_, _ = w.Write([]byte(fmt.Sprintf(`{"token":%q,"expires_in":7200}`, token)))
			case "/v2/ns/repo/tags/list":
				if r.Header.Get("Authorization") == "Bearer tk-stale" {
					w.WriteHeader(http.StatusUnauthorized) // 旧 token 失效一次
					return
				}
				w.Header().Set("Content-Type", "application/json")
				_, _ = w.Write([]byte(`{"name":"ns/repo","tags":["v1"]}`))
			}
		}))
		tags, err := c.GetTags(registryHostOf(ts), "u", "p", "ns", "repo", "", "")
		require.NoError(t, err)
		assert.Equal(t, []string{"v1"}, tags)
		assert.Equal(t, int32(2), atomic.LoadInt32(&tokenCalls), "应触发一次 token 刷新")
	})

	t.Run("持续401最终失败", func(t *testing.T) {
		c := newTestBearerClient("SWR")
		ts := newBearerFullServer(t, `{}`, http.StatusUnauthorized)
		_, err := c.GetTags(registryHostOf(ts), "u", "p", "ns", "repo", "", "")
		require.Error(t, err)
		assert.Contains(t, err.Error(), "HTTP 401")
	})

	t.Run("500报错", func(t *testing.T) {
		c := newTestBearerClient("SWR")
		ts := newBearerFullServer(t, `oops`, http.StatusInternalServerError)
		_, err := c.GetTags(registryHostOf(ts), "u", "p", "ns", "repo", "", "")
		require.Error(t, err)
		assert.Contains(t, err.Error(), "获取Tag列表失败: HTTP 500")
	})

	t.Run("网络错误", func(t *testing.T) {
		c := newTestBearerClient("SWR")
		c.authCache["127.0.0.1:1"] = &registryAuthInfo{Realm: "https://127.0.0.1:1/token"}
		_, err := c.GetTags("127.0.0.1:1", "u", "p", "ns", "repo", "", "")
		require.Error(t, err)
	})

	t.Run("token失败上抛", func(t *testing.T) {
		c := newTestBearerClient("SWR")
		ts := newSvcTLSServer(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			w.WriteHeader(http.StatusUnauthorized) // /v2/ 与 /token 均 401
		}))
		_, err := c.GetTags(registryHostOf(ts), "u", "p", "ns", "repo", "", "")
		require.Error(t, err)
		assert.Contains(t, err.Error(), "获取SWR Token失败")
	})
}

func TestBearerGetTagNames(t *testing.T) {
	c := newTestBearerClient("SWR")
	ts := newBearerFullServer(t, `{"name":"ns/repo","tags":["a","b"]}`)
	tags, err := c.GetTagNames(registryHostOf(ts), "u", "p", "ns", "repo", "auth", "svc")
	require.NoError(t, err)
	assert.Equal(t, []string{"a", "b"}, tags)
}

func TestBearerRepositoryExists(t *testing.T) {
	c := newTestBearerClient("SWR")

	ts := newBearerFullServer(t, `{"name":"ns/repo","tags":["v1"]}`)
	ok, err := c.RepositoryExists(registryHostOf(ts), "u", "p", "ns", "repo", "", "")
	require.NoError(t, err)
	assert.True(t, ok)

	ts2 := newBearerFullServer(t, `{"name":"ns/repo","tags":null}`)
	ok2, err2 := c.RepositoryExists(registryHostOf(ts2), "u", "p", "ns", "repo", "", "")
	require.NoError(t, err2)
	assert.False(t, ok2)

	c2 := newTestBearerClient("SWR")
	c2.authCache["127.0.0.1:1"] = &registryAuthInfo{Realm: "https://127.0.0.1:1/token"}
	_, err = c2.RepositoryExists("127.0.0.1:1", "u", "p", "ns", "repo", "", "")
	require.Error(t, err)
}

func TestBearerGetTagDetail(t *testing.T) {
	c := newTestBearerClient("SWR")
	var ts *httptest.Server
	ts = newSvcTLSServer(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/v2/":
			w.Header().Set("WWW-Authenticate", fmt.Sprintf(`Bearer realm="%s/token",service="svc"`, ts.URL))
			w.WriteHeader(http.StatusUnauthorized)
		case "/token":
			w.Header().Set("Content-Type", "application/json")
			_, _ = w.Write([]byte(`{"token":"tk","expires_in":7200}`))
		case "/v2/ns/repo/manifests/v1":
			w.Header().Set("Docker-Content-Digest", "sha256:manifest")
			w.Header().Set("Last-Modified", "Mon, 02 Jan 2006 15:04:05 GMT")
			w.Header().Set("Content-Type", "application/json")
			_, _ = w.Write([]byte(rcManifestList))
		default:
			w.Header().Set("Last-Modified", "Mon, 02 Jan 2006 15:04:05 GMT")
			w.WriteHeader(http.StatusOK)
		}
	}))

	detail, err := c.GetTagDetail(registryHostOf(ts), "u", "p", "ns", "repo", "v1", "", "")
	require.NoError(t, err)
	assert.Equal(t, "v1", detail.Tag)
	assert.Equal(t, []string{"amd64", "arm64"}, detail.Architectures)
	assert.Equal(t, "sha256:aaa", detail.Digests["amd64"])
	assert.Equal(t, "2006-01-02T15:04:05Z", detail.PushedAt["amd64"])
}

func TestBearerGetTagDetailErrors(t *testing.T) {
	// token 失败
	c := newTestBearerClient("SWR")
	ts := newSvcTLSServer(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusUnauthorized)
	}))
	_, err := c.GetTagDetail(registryHostOf(ts), "u", "p", "ns", "repo", "v1", "", "")
	require.Error(t, err)

	// manifest 404
	c2 := newTestBearerClient("SWR")
	var ts2 *httptest.Server
	ts2 = newSvcTLSServer(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/v2/":
			w.Header().Set("WWW-Authenticate", fmt.Sprintf(`Bearer realm="%s/token"`, ts2.URL))
			w.WriteHeader(http.StatusUnauthorized)
		case "/token":
			w.Header().Set("Content-Type", "application/json")
			_, _ = w.Write([]byte(`{"token":"tk","expires_in":7200}`))
		default:
			w.WriteHeader(http.StatusNotFound)
		}
	}))
	_, err = c2.GetTagDetail(registryHostOf(ts2), "u", "p", "ns", "repo", "missing", "", "")
	require.Error(t, err)
	assert.Contains(t, err.Error(), "HTTP 404")
}

func TestBearerGetTagsDetailsBatch(t *testing.T) {
	t.Run("空列表", func(t *testing.T) {
		c := newTestBearerClient("SWR")
		details, err := c.GetTagsDetailsBatch("reg", "u", "p", "ns", "repo", "", "", nil)
		require.NoError(t, err)
		assert.Empty(t, details)
	})

	t.Run("超过单批上限", func(t *testing.T) {
		c := newTestBearerClient("SWR")
		tags := make([]string, tagDetailsBatchMaxSize+1)
		for i := range tags {
			tags[i] = fmt.Sprintf("v%d", i)
		}
		_, err := c.GetTagsDetailsBatch("reg", "u", "p", "ns", "repo", "", "", tags)
		require.Error(t, err)
		assert.Contains(t, err.Error(), "单次最多查询")
	})

	t.Run("初始token失败", func(t *testing.T) {
		c := newTestBearerClient("SWR")
		ts := newSvcTLSServer(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			w.WriteHeader(http.StatusUnauthorized)
		}))
		_, err := c.GetTagsDetailsBatch(registryHostOf(ts), "u", "p", "ns", "repo", "", "", []string{"v1"})
		require.Error(t, err)
	})

	t.Run("成功与失败混合", func(t *testing.T) {
		c := newTestBearerClient("SWR")
		var ts *httptest.Server
		ts = newSvcTLSServer(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			switch {
			case r.URL.Path == "/v2/":
				w.Header().Set("WWW-Authenticate", fmt.Sprintf(`Bearer realm="%s/token"`, ts.URL))
				w.WriteHeader(http.StatusUnauthorized)
			case r.URL.Path == "/token":
				w.Header().Set("Content-Type", "application/json")
				_, _ = w.Write([]byte(`{"token":"tk","expires_in":7200}`))
			case r.URL.Path == "/v2/ns/repo/manifests/bad":
				w.WriteHeader(http.StatusInternalServerError) // 该 tag 拉取失败 → 空详情
			case strings.HasPrefix(r.URL.Path, "/v2/ns/repo/manifests/"):
				w.Header().Set("Docker-Content-Digest", "sha256:ok")
				w.Header().Set("Content-Type", "application/json")
				_, _ = w.Write([]byte(rcManifestList))
			default:
				w.Header().Set("Last-Modified", "Mon, 02 Jan 2006 15:04:05 GMT")
				w.WriteHeader(http.StatusOK)
			}
		}))
		details, err := c.GetTagsDetailsBatch(registryHostOf(ts), "u", "p", "ns", "repo", "", "", []string{"good1", "bad", "good2"})
		require.NoError(t, err)
		require.Len(t, details, 3)
		// 索引与输入对应
		assert.Equal(t, []string{"amd64", "arm64"}, details[0].Architectures)
		assert.Equal(t, "bad", details[1].Tag)
		assert.Empty(t, details[1].Architectures)
		assert.Equal(t, []string{"amd64", "arm64"}, details[2].Architectures)
	})
}

func TestBearerGetTagsWithDetails(t *testing.T) {
	c := newTestBearerClient("SWR")
	var ts *httptest.Server
	ts = newSvcTLSServer(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case r.URL.Path == "/v2/":
			w.Header().Set("WWW-Authenticate", fmt.Sprintf(`Bearer realm="%s/token"`, ts.URL))
			w.WriteHeader(http.StatusUnauthorized)
		case r.URL.Path == "/token":
			w.Header().Set("Content-Type", "application/json")
			_, _ = w.Write([]byte(`{"token":"tk","expires_in":7200}`))
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
	details, err := c.GetTagsWithDetails(registryHostOf(ts), "u", "p", "ns", "repo", "", "")
	require.NoError(t, err)
	require.Len(t, details, 1)
	assert.Equal(t, "v1", details[0].Tag)

	// tags 拉取失败上抛
	c2 := newTestBearerClient("SWR")
	_, err = c2.GetTagsWithDetails("127.0.0.1:1", "u", "p", "ns", "repo", "", "")
	require.Error(t, err)
}
