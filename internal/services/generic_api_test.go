package services

import (
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

func newTestGenericService() *GenericAPIService {
	s := NewGenericAPIService()
	s.client = svcTLSRestyClient()
	return s
}

// joinQuoted 把字符串列表拼成 "a","b" 形式（构造 JSON 数组体）
func joinQuoted(items []string) string {
	quoted := make([]string, 0, len(items))
	for _, it := range items {
		quoted = append(quoted, fmt.Sprintf("%q", it))
	}
	return strings.Join(quoted, ",")
}

func nowPlusHour() time.Time { return time.Now().Add(time.Hour) }

func TestGenericTokenCacheKeyAndInvalidate(t *testing.T) {
	assert.Equal(t, "reg|scope-x", genericTokenCacheKey("reg", "scope-x"))

	s := newTestGenericService()
	s.tokenCache["reg|scope"] = &tokenCacheItem{token: "tk"}
	s.invalidateTokenCache("reg", "scope")
	assert.NotContains(t, s.tokenCache, "reg|scope")
}

func TestGenericCachedAuth(t *testing.T) {
	s := newTestGenericService()
	assert.Nil(t, s.cachedAuth("unknown.example.com"))

	s.authCache["r1"] = &genericAuthInfo{Realm: "https://r1/token"}
	got := s.cachedAuth("r1")
	require.NotNil(t, got)
	assert.Equal(t, "https://r1/token", got.Realm)
}

func TestGenericDiscoverAuth(t *testing.T) {
	// 401 + challenge → 解析
	var ts *httptest.Server
	var hits int32
	ts = newSvcTLSServer(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		atomic.AddInt32(&hits, 1)
		w.Header().Set("WWW-Authenticate", fmt.Sprintf(`Bearer realm="%s/token",service="svc"`, ts.URL))
		w.WriteHeader(http.StatusUnauthorized)
	}))
	s := newTestGenericService()
	info := s.discoverAuth(registryHostOf(ts), "u", "p")
	require.NotNil(t, info)
	assert.Equal(t, ts.URL+"/token", info.Realm)
	assert.Equal(t, "svc", info.Service)

	// 命中缓存，不再探测
	_ = s.discoverAuth(registryHostOf(ts), "u", "p")
	assert.Equal(t, int32(1), atomic.LoadInt32(&hits))

	// 非 401 → 空 info
	ts2 := newSvcTLSServer(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))
	s2 := newTestGenericService()
	info2 := s2.discoverAuth(registryHostOf(ts2), "u", "p")
	require.NotNil(t, info2)
	assert.Empty(t, info2.Realm)
}

func TestGenericGetToken(t *testing.T) {
	t.Run("成功并缓存", func(t *testing.T) {
		var ts *httptest.Server
		var hits int32
		var captured *http.Request
		ts = newSvcTLSServer(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			atomic.AddInt32(&hits, 1)
			captured = r
			w.Header().Set("Content-Type", "application/json")
			_, _ = w.Write([]byte(`{"token":"gtok","expires_in":7200}`))
		}))
		s := newTestGenericService()
		s.authCache[registryHostOf(ts)] = &genericAuthInfo{Realm: ts.URL + "/token", Service: "svc"}

		tok, err := s.getToken(registryHostOf(ts), "user", "pass", "ns", "repo", "repository:ns/repo:pull")
		require.NoError(t, err)
		assert.Equal(t, "gtok", tok)
		require.NotNil(t, captured)
		// 携带 Basic 凭证
		assert.True(t, strings.HasPrefix(captured.Header.Get("Authorization"), "Basic "))
		q := captured.URL.Query()
		assert.Equal(t, "svc", q.Get("service"))
		assert.Equal(t, "repository:ns/repo:pull", q.Get("scope"))

		// 缓存命中
		tok2, err := s.getToken(registryHostOf(ts), "user", "pass", "ns", "repo", "repository:ns/repo:pull")
		require.NoError(t, err)
		assert.Equal(t, "gtok", tok2)
		assert.Equal(t, int32(1), atomic.LoadInt32(&hits))
	})

	t.Run("匿名请求不带Basic", func(t *testing.T) {
		var ts *httptest.Server
		var captured *http.Request
		ts = newSvcTLSServer(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			captured = r
			w.Header().Set("Content-Type", "application/json")
			_, _ = w.Write([]byte(`{"token":"anon-tok"}`))
		}))
		s := newTestGenericService()
		s.authCache[registryHostOf(ts)] = &genericAuthInfo{Realm: ts.URL + "/token"}

		tok, err := s.getToken(registryHostOf(ts), "", "", "ns", "repo", "repository:ns/repo:pull")
		require.NoError(t, err)
		assert.Equal(t, "anon-tok", tok)
		require.NotNil(t, captured)
		assert.Empty(t, captured.Header.Get("Authorization"))
	})

	t.Run("无认证端点", func(t *testing.T) {
		s := newTestGenericService()
		ts := newSvcTLSServer(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			w.WriteHeader(http.StatusOK)
		}))
		_, err := s.getToken(registryHostOf(ts), "u", "p", "ns", "repo", "scope")
		require.Error(t, err)
		assert.Contains(t, err.Error(), "未返回 Bearer 认证信息")
	})

	t.Run("401认证被拒绝", func(t *testing.T) {
		ts := newSvcTLSServer(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			w.WriteHeader(http.StatusUnauthorized)
		}))
		s := newTestGenericService()
		s.authCache[registryHostOf(ts)] = &genericAuthInfo{Realm: ts.URL + "/token"}
		_, err := s.getToken(registryHostOf(ts), "u", "p", "ns", "repo", "scope")
		require.Error(t, err)
		assert.Contains(t, err.Error(), "认证被拒绝")
	})

	t.Run("非200", func(t *testing.T) {
		ts := newSvcTLSServer(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			w.WriteHeader(http.StatusBadGateway)
		}))
		s := newTestGenericService()
		s.authCache[registryHostOf(ts)] = &genericAuthInfo{Realm: ts.URL + "/token"}
		_, err := s.getToken(registryHostOf(ts), "u", "p", "ns", "repo", "scope")
		require.Error(t, err)
		assert.Contains(t, err.Error(), "HTTP 502")
	})

	t.Run("网络错误", func(t *testing.T) {
		s := newTestGenericService()
		s.authCache["bad.example.com"] = &genericAuthInfo{Realm: "https://127.0.0.1:1/token"}
		_, err := s.getToken("bad.example.com", "u", "p", "ns", "repo", "scope")
		require.Error(t, err)
		assert.Contains(t, err.Error(), "获取Token失败")
	})

	t.Run("空Token", func(t *testing.T) {
		ts := newSvcTLSServer(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			w.Header().Set("Content-Type", "application/json")
			_, _ = w.Write([]byte(`{"token":""}`))
		}))
		s := newTestGenericService()
		s.authCache[registryHostOf(ts)] = &genericAuthInfo{Realm: ts.URL + "/token"}
		_, err := s.getToken(registryHostOf(ts), "u", "p", "ns", "repo", "scope")
		require.Error(t, err)
		assert.Contains(t, err.Error(), "返回空Token")
	})
}

func TestGenericGetTagsBasicDirect(t *testing.T) {
	var ts *httptest.Server
	ts = newSvcTLSServer(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		require.Equal(t, "/v2/ns/repo/tags/list", r.URL.Path)
		require.True(t, strings.HasPrefix(r.Header.Get("Authorization"), "Basic "),
			"Basic 直连模式应携带 Basic 凭证")
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"name":"ns/repo","tags":["v1","v2"]}`))
	}))
	s := newTestGenericService()

	tags, err := s.GetTags(registryHostOf(ts), "user", "pass", "ns", "repo", "", "")
	require.NoError(t, err)
	assert.Equal(t, []string{"v1", "v2"}, tags)

	// GetTagNames 为别名
	tags2, err := s.GetTagNames(registryHostOf(ts), "user", "pass", "ns", "repo", "", "")
	require.NoError(t, err)
	assert.Equal(t, []string{"v1", "v2"}, tags2)
}

func TestGenericGetTagsBearerFlow(t *testing.T) {
	// /v2/ 401 challenge → /token → Bearer 重试
	var ts *httptest.Server
	ts = newSvcTLSServer(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/v2/":
			w.Header().Set("WWW-Authenticate", fmt.Sprintf(`Bearer realm="%s/token",service="svc"`, ts.URL))
			w.WriteHeader(http.StatusUnauthorized)
		case "/token":
			w.Header().Set("Content-Type", "application/json")
			_, _ = w.Write([]byte(`{"token":"btok","expires_in":7200}`))
		case "/v2/ns/repo/tags/list":
			if !strings.HasPrefix(r.Header.Get("Authorization"), "Bearer ") {
				w.Header().Set("WWW-Authenticate", fmt.Sprintf(`Bearer realm="%s/token",service="svc"`, ts.URL))
				w.WriteHeader(http.StatusUnauthorized)
				return
			}
			w.Header().Set("Content-Type", "application/json")
			_, _ = w.Write([]byte(`{"name":"ns/repo","tags":["v1"]}`))
		default:
			t.Errorf("意外请求 %s", r.URL.Path)
		}
	}))
	s := newTestGenericService()
	tags, err := s.GetTags(registryHostOf(ts), "user", "pass", "ns", "repo", "", "")
	require.NoError(t, err)
	assert.Equal(t, []string{"v1"}, tags)
}

func TestGenericGetTagsBranches(t *testing.T) {
	cases := []struct {
		name    string
		handler http.HandlerFunc
		wantErr string
		want    []string
	}{
		{
			name: "404返回空",
			handler: func(w http.ResponseWriter, r *http.Request) { w.WriteHeader(http.StatusNotFound) },
			want: []string{},
		},
		{
			name: "401无challenge",
			handler: func(w http.ResponseWriter, r *http.Request) {
				w.WriteHeader(http.StatusUnauthorized)
			},
			wantErr: "认证被拒绝（HTTP 401）",
		},
		{
			name: "401有challenge但token失败",
			handler: func(w http.ResponseWriter, r *http.Request) {
				// /v2/ 与数据面均 401 + challenge，token 端点也 401 → getToken 失败 → 返回原 401
				w.Header().Set("WWW-Authenticate", `Bearer realm="/token"`)
				w.WriteHeader(http.StatusUnauthorized)
			},
			wantErr: "认证被拒绝（HTTP 401）",
		},
		{
			name: "403",
			handler: func(w http.ResponseWriter, r *http.Request) { w.WriteHeader(http.StatusForbidden) },
			wantErr: "HTTP 403",
		},
		{
			name: "500",
			handler: func(w http.ResponseWriter, r *http.Request) { w.WriteHeader(http.StatusInternalServerError) },
			wantErr: "HTTP 500",
		},
		{
			name: "畸形JSON",
			handler: func(w http.ResponseWriter, r *http.Request) {
				w.Header().Set("Content-Type", "application/json")
				_, _ = w.Write([]byte(`{broken`))
			},
			wantErr: "解析Tag列表失败",
		},
		{
			name: "tags为null",
			handler: func(w http.ResponseWriter, r *http.Request) {
				w.Header().Set("Content-Type", "application/json")
				_, _ = w.Write([]byte(`{"name":"ns/repo","tags":null}`))
			},
			want: []string{},
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			s := newTestGenericService()
			ts := newSvcTLSServer(t, tc.handler)
			tags, err := s.GetTags(registryHostOf(ts), "user", "pass", "ns", "repo", "", "")
			if tc.wantErr != "" {
				require.Error(t, err)
				assert.Contains(t, err.Error(), tc.wantErr)
				return
			}
			require.NoError(t, err)
			assert.Equal(t, tc.want, tags)
		})
	}

	// 网络错误
	s := newTestGenericService()
	_, err := s.GetTags("127.0.0.1:1", "user", "pass", "ns", "repo", "", "")
	require.Error(t, err)
	assert.Contains(t, err.Error(), "获取Tag列表失败")
}

func TestGenericRepositoryExists(t *testing.T) {
	s := newTestGenericService()
	ts := newSvcTLSServer(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"name":"ns/repo","tags":["v1"]}`))
	}))
	ok, err := s.RepositoryExists(registryHostOf(ts), "u", "p", "ns", "repo", "", "")
	require.NoError(t, err)
	assert.True(t, ok)

	ts2 := newSvcTLSServer(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"name":"ns/repo","tags":[]}`))
	}))
	ok2, err2 := s.RepositoryExists(registryHostOf(ts2), "u", "p", "ns", "repo", "", "")
	require.NoError(t, err2)
	assert.False(t, ok2)
}

func TestGenericFetchManifest(t *testing.T) {
	t.Run("Basic直连成功", func(t *testing.T) {
		s := newTestGenericService()
		ts := newSvcTLSServer(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			require.Equal(t, "/v2/ns/repo/manifests/v1", r.URL.Path)
			w.Header().Set("Docker-Content-Digest", "sha256:gm")
			w.Header().Set("Last-Modified", "Mon, 02 Jan 2006 15:04:05 GMT")
			w.Header().Set("Content-Type", "application/json")
			_, _ = w.Write([]byte(rcManifestList))
		}))
		body, digest, lastMod, token, err := s.fetchManifestGeneric(registryHostOf(ts), "u", "p", "ns", "repo", "v1", false)
		require.NoError(t, err)
		assert.Equal(t, []byte(rcManifestList), body)
		assert.Equal(t, "sha256:gm", digest)
		assert.Equal(t, "2006-01-02T15:04:05Z", lastMod)
		assert.Empty(t, token, "Basic 模式无 token")
	})

	t.Run("Bearer模式返回token", func(t *testing.T) {
		var ts *httptest.Server
		ts = newSvcTLSServer(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			switch r.URL.Path {
			case "/v2/":
				w.Header().Set("WWW-Authenticate", fmt.Sprintf(`Bearer realm="%s/token"`, ts.URL))
				w.WriteHeader(http.StatusUnauthorized)
			case "/token":
				w.Header().Set("Content-Type", "application/json")
				_, _ = w.Write([]byte(`{"token":"mtok","expires_in":7200}`))
			default:
				if !strings.HasPrefix(r.Header.Get("Authorization"), "Bearer ") {
					w.Header().Set("WWW-Authenticate", fmt.Sprintf(`Bearer realm="%s/token"`, ts.URL))
					w.WriteHeader(http.StatusUnauthorized)
					return
				}
				w.Header().Set("Docker-Content-Digest", "sha256:gm2")
				w.Header().Set("Content-Type", "application/json")
				_, _ = w.Write([]byte(rcSingleManifest))
			}
		}))
		s := newTestGenericService()
		_, _, _, token, err := s.fetchManifestGeneric(registryHostOf(ts), "u", "p", "ns", "repo", "v1", false)
		require.NoError(t, err)
		assert.Equal(t, "mtok", token)
	})

	t.Run("强制刷新token缓存", func(t *testing.T) {
		var ts *httptest.Server
		var tokenCalls int32
		ts = newSvcTLSServer(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			switch r.URL.Path {
			case "/v2/":
				w.Header().Set("WWW-Authenticate", fmt.Sprintf(`Bearer realm="%s/token"`, ts.URL))
				w.WriteHeader(http.StatusUnauthorized)
			case "/token":
				atomic.AddInt32(&tokenCalls, 1)
				w.Header().Set("Content-Type", "application/json")
				_, _ = w.Write([]byte(`{"token":"mtok2","expires_in":7200}`))
			default:
				w.Header().Set("Docker-Content-Digest", "sha256:x")
				w.Header().Set("Content-Type", "application/json")
				_, _ = w.Write([]byte(rcSingleManifest))
			}
		}))
		s := newTestGenericService()
		// 预置过期缓存 + auth 缓存，验证 forceTokenRefresh 清理后重新获取
		s.authCache[registryHostOf(ts)] = &genericAuthInfo{Realm: ts.URL + "/token"}
		s.tokenCache[genericTokenCacheKey(registryHostOf(ts), "repository:ns/repo:pull")] =
			&tokenCacheItem{token: "stale", expiresAt: nowPlusHour()}

		_, _, _, token, err := s.fetchManifestGeneric(registryHostOf(ts), "u", "p", "ns", "repo", "v1", true)
		require.NoError(t, err)
		assert.Equal(t, "mtok2", token)
		assert.Equal(t, int32(1), atomic.LoadInt32(&tokenCalls))
	})

	t.Run("429后退避重试成功", func(t *testing.T) {
		var hits int32
		ts := newSvcTLSServer(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			if atomic.AddInt32(&hits, 1) == 1 {
				w.WriteHeader(http.StatusTooManyRequests)
				return
			}
			w.Header().Set("Docker-Content-Digest", "sha256:after429")
			w.Header().Set("Content-Type", "application/json")
			_, _ = w.Write([]byte(rcSingleManifest))
		}))
		s := newTestGenericService()
		_, digest, _, _, err := s.fetchManifestGeneric(registryHostOf(ts), "u", "p", "ns", "repo", "v1", false)
		require.NoError(t, err)
		assert.Equal(t, "sha256:after429", digest)
	})

	t.Run("非200直接报错", func(t *testing.T) {
		ts := newSvcTLSServer(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			w.WriteHeader(http.StatusNotFound)
		}))
		s := newTestGenericService()
		_, _, _, _, err := s.fetchManifestGeneric(registryHostOf(ts), "u", "p", "ns", "repo", "v1", false)
		require.Error(t, err)
		assert.Contains(t, err.Error(), "获取Manifest失败: HTTP 404")
	})

	t.Run("网络错误", func(t *testing.T) {
		s := newTestGenericService()
		_, _, _, _, err := s.fetchManifestGeneric("127.0.0.1:1", "u", "p", "ns", "repo", "v1", false)
		require.Error(t, err)
		assert.Contains(t, err.Error(), "获取Manifest失败")
	})
}

func TestGenericGetTagDetailAndBatches(t *testing.T) {
	s := newTestGenericService()
	ts := newSvcTLSServer(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case r.URL.Path == "/v2/ns/repo/tags/list":
			w.WriteHeader(http.StatusNotFound) // tags/list 404 → GetTags 空列表
		case strings.HasSuffix(r.URL.Path, "/manifests/v1"):
			w.Header().Set("Docker-Content-Digest", "sha256:ok")
			w.Header().Set("Last-Modified", "Mon, 02 Jan 2006 15:04:05 GMT")
			w.Header().Set("Content-Type", "application/json")
			_, _ = w.Write([]byte(rcManifestList))
		case strings.HasSuffix(r.URL.Path, "/manifests/500"):
			w.WriteHeader(http.StatusInternalServerError)
		default:
			w.Header().Set("Last-Modified", "Mon, 02 Jan 2006 15:04:05 GMT")
			w.WriteHeader(http.StatusOK)
		}
	}))

	detail, err := s.GetTagDetail(registryHostOf(ts), "u", "p", "ns", "repo", "v1", "", "")
	require.NoError(t, err)
	assert.Equal(t, []string{"amd64", "arm64"}, detail.Architectures)

	// 空批次
	details, err := s.GetTagsDetailsBatch(registryHostOf(ts), "u", "p", "ns", "repo", "", "", nil)
	require.NoError(t, err)
	assert.Empty(t, details)

	// 超上限
	many := make([]string, tagDetailsBatchMaxSize+1)
	for i := range many {
		many[i] = fmt.Sprintf("v%d", i)
	}
	_, err = s.GetTagsDetailsBatch(registryHostOf(ts), "u", "p", "ns", "repo", "", "", many)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "单次最多查询")

	// 成功 + 失败混合
	details, err = s.GetTagsDetailsBatch(registryHostOf(ts), "u", "p", "ns", "repo", "", "", []string{"v1", "500"})
	require.NoError(t, err)
	require.Len(t, details, 2)
	assert.Equal(t, []string{"amd64", "arm64"}, details[0].Architectures)
	assert.Equal(t, "500", details[1].Tag)
	assert.Empty(t, details[1].Architectures)

	// GetTagsWithDetails：tags/list 404 → 空列表 → 批量为空
	details, err = s.GetTagsWithDetails(registryHostOf(ts), "u", "p", "ns", "repo", "", "")
	require.NoError(t, err)
	assert.Empty(t, details)
}

func TestGenericGetTagsWithDetailsError(t *testing.T) {
	s := newTestGenericService()
	_, err := s.GetTagsWithDetails("127.0.0.1:1", "u", "p", "ns", "repo", "", "")
	require.Error(t, err)
}

func TestGenericListRepositories(t *testing.T) {
	t.Run("成功并按命名空间过滤", func(t *testing.T) {
		var ts *httptest.Server
		ts = newSvcTLSServer(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			switch r.URL.Path {
			case "/v2/":
				w.Header().Set("WWW-Authenticate", fmt.Sprintf(`Bearer realm="%s/token"`, ts.URL))
				w.WriteHeader(http.StatusUnauthorized)
			case "/token":
				w.Header().Set("Content-Type", "application/json")
				_, _ = w.Write([]byte(`{"token":"ctok","expires_in":7200}`))
			default:
				require.Equal(t, "/v2/_catalog", r.URL.Path)
				require.Equal(t, "Bearer ctok", r.Header.Get("Authorization"))
				require.Equal(t, "1000", r.URL.Query().Get("n"))
				w.Header().Set("Content-Type", "application/json")
				_, _ = w.Write([]byte(`{"repositories":["ns/app1","ns/app2","other/app3"]}`))
			}
		}))
		s := newTestGenericService()
		repos, err := s.ListRepositories(registryHostOf(ts), "u", "p", "", "", "ns", "", "")
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
			case "/v2/":
				w.Header().Set("WWW-Authenticate", fmt.Sprintf(`Bearer realm="%s/token"`, ts.URL))
				w.WriteHeader(http.StatusUnauthorized)
			case "/token":
				w.Header().Set("Content-Type", "application/json")
				_, _ = w.Write([]byte(`{"token":"ctok","expires_in":7200}`))
			default:
				atomic.AddInt32(&catalogCalls, 1)
				if r.URL.Query().Get("last") == "" {
					// 首页满 1000 → 触发下一页
					w.Header().Set("Content-Type", "application/json")
					_, _ = w.Write([]byte(fmt.Sprintf(`{"repositories":[%s]}`, joinQuoted(page1))))
					return
				}
				// 第二页：带 last 参数
				assert.Equal(t, "ns/r0999", r.URL.Query().Get("last"))
				w.Header().Set("Content-Type", "application/json")
				_, _ = w.Write([]byte(`{"repositories":["ns/tail"]}`))
			}
		}))
		s := newTestGenericService()
		repos, err := s.ListRepositories(registryHostOf(ts), "u", "p", "", "", "ns", "", "")
		require.NoError(t, err)
		assert.Len(t, repos, 1001)
		assert.Equal(t, "tail", repos[len(repos)-1])
		assert.Equal(t, int32(2), atomic.LoadInt32(&catalogCalls))
	})

	t.Run("404不支持catalog", func(t *testing.T) {
		var ts *httptest.Server
		ts = newSvcTLSServer(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			switch r.URL.Path {
			case "/v2/":
				w.Header().Set("WWW-Authenticate", fmt.Sprintf(`Bearer realm="%s/token"`, ts.URL))
				w.WriteHeader(http.StatusUnauthorized)
			case "/token":
				w.Header().Set("Content-Type", "application/json")
				_, _ = w.Write([]byte(`{"token":"ctok","expires_in":7200}`))
			default:
				w.WriteHeader(http.StatusNotFound)
			}
		}))
		s := newTestGenericService()
		_, err := s.ListRepositories(registryHostOf(ts), "u", "p", "", "", "ns", "", "")
		require.Error(t, err)
		assert.Contains(t, err.Error(), "不支持 /v2/_catalog")
	})

	t.Run("token失败", func(t *testing.T) {
		s := newTestGenericService()
		ts := newSvcTLSServer(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			w.WriteHeader(http.StatusOK) // /v2/ 无 challenge → getToken 失败
		}))
		_, err := s.ListRepositories(registryHostOf(ts), "u", "p", "", "", "ns", "", "")
		require.Error(t, err)
		assert.Contains(t, err.Error(), "获取Catalog权限失败")
	})

	t.Run("401", func(t *testing.T) {
		var ts *httptest.Server
		ts = newSvcTLSServer(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			switch r.URL.Path {
			case "/v2/":
				w.Header().Set("WWW-Authenticate", fmt.Sprintf(`Bearer realm="%s/token"`, ts.URL))
				w.WriteHeader(http.StatusUnauthorized)
			case "/token":
				w.Header().Set("Content-Type", "application/json")
				_, _ = w.Write([]byte(`{"token":"ctok","expires_in":7200}`))
			default:
				w.WriteHeader(http.StatusUnauthorized)
			}
		}))
		s := newTestGenericService()
		_, err := s.ListRepositories(registryHostOf(ts), "u", "p", "", "", "ns", "", "")
		require.Error(t, err)
		assert.Contains(t, err.Error(), "认证被拒绝（HTTP 401）")
	})

	t.Run("500与畸形响应", func(t *testing.T) {
		mk := func(body string, status int) *httptest.Server {
			var ts *httptest.Server
			ts = newSvcTLSServer(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				switch r.URL.Path {
				case "/v2/":
					w.Header().Set("WWW-Authenticate", fmt.Sprintf(`Bearer realm="%s/token"`, ts.URL))
					w.WriteHeader(http.StatusUnauthorized)
				case "/token":
					w.Header().Set("Content-Type", "application/json")
					_, _ = w.Write([]byte(`{"token":"ctok","expires_in":7200}`))
				default:
					if status != http.StatusOK {
						w.WriteHeader(status)
					}
					w.Header().Set("Content-Type", "application/json")
					_, _ = w.Write([]byte(body))
				}
			}))
			return ts
		}

		s := newTestGenericService()
		_, err := s.ListRepositories(registryHostOf(mk(`x`, http.StatusInternalServerError)), "u", "p", "", "", "ns", "", "")
		require.Error(t, err)
		assert.Contains(t, err.Error(), "获取仓库列表失败: HTTP 500")

		s2 := newTestGenericService()
		_, err = s2.ListRepositories(registryHostOf(mk(`{bad`, http.StatusOK)), "u", "p", "", "", "ns", "", "")
		require.Error(t, err)
		assert.Contains(t, err.Error(), "解析仓库列表失败")
	})
}

func TestGenericListRepositoriesErrorBranches(t *testing.T) {
	// catalog 请求返回 500
	func() {
		var ts *httptest.Server
		ts = newSvcTLSServer(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			switch r.URL.Path {
			case "/v2/":
				w.Header().Set("WWW-Authenticate", fmt.Sprintf(`Bearer realm="%s/token"`, ts.URL))
				w.WriteHeader(http.StatusUnauthorized)
			case "/token":
				w.Header().Set("Content-Type", "application/json")
				_, _ = w.Write([]byte(`{"token":"ctok","expires_in":7200}`))
			default:
				w.WriteHeader(http.StatusInternalServerError)
			}
		}))
		s := newTestGenericService()
		_, err := s.ListRepositories(registryHostOf(ts), "u", "p", "", "", "ns", "", "")
		require.Error(t, err)
		assert.Contains(t, err.Error(), "HTTP 500")
	}()

	// 畸形 JSON
	func() {
		var ts *httptest.Server
		ts = newSvcTLSServer(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			switch r.URL.Path {
			case "/v2/":
				w.Header().Set("WWW-Authenticate", fmt.Sprintf(`Bearer realm="%s/token"`, ts.URL))
				w.WriteHeader(http.StatusUnauthorized)
			case "/token":
				w.Header().Set("Content-Type", "application/json")
				_, _ = w.Write([]byte(`{"token":"ctok","expires_in":7200}`))
			default:
				w.Header().Set("Content-Type", "application/json")
				_, _ = w.Write([]byte(`{broken`))
			}
		}))
		s := newTestGenericService()
		_, err := s.ListRepositories(registryHostOf(ts), "u", "p", "", "", "ns", "", "")
		require.Error(t, err)
		assert.Contains(t, err.Error(), "解析仓库列表失败")
	}()

	// 数据面网络错误（token 端点直接不可达）
	func() {
		s := newTestGenericService()
		s.authCache["bad.example.com"] = &genericAuthInfo{Realm: "https://127.0.0.1:1/token"}
		_, err := s.ListRepositories("bad.example.com", "u", "p", "", "", "ns", "", "")
		require.Error(t, err)
		assert.Contains(t, err.Error(), "获取Catalog权限失败")
	}()
}

func TestGenericTestConnection(t *testing.T) {
	t.Run("200直接通过", func(t *testing.T) {
		s := newTestGenericService()
		ts := newSvcTLSServer(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			w.WriteHeader(http.StatusOK)
		}))
		res := s.TestConnection(registryHostOf(ts), "u", "p", "", "", "ns", "", "")
		assert.True(t, res.LoginOK)
		assert.Equal(t, "generic", res.RegistryType)
	})

	t.Run("challenge后token成功", func(t *testing.T) {
		var ts *httptest.Server
		ts = newSvcTLSServer(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			switch r.URL.Path {
			case "/v2/":
				w.Header().Set("WWW-Authenticate", fmt.Sprintf(`Bearer realm="%s/token",service="svc"`, ts.URL))
				w.WriteHeader(http.StatusUnauthorized)
			case "/token":
				w.Header().Set("Content-Type", "application/json")
				_, _ = w.Write([]byte(`{"token":"ct","expires_in":7200}`))
			}
		}))
		s := newTestGenericService()
		res := s.TestConnection(registryHostOf(ts), "u", "p", "", "", "ns", "", "")
		assert.True(t, res.LoginOK)
	})

	t.Run("challenge后token失败", func(t *testing.T) {
		var ts *httptest.Server
		ts = newSvcTLSServer(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			switch r.URL.Path {
			case "/v2/":
				w.Header().Set("WWW-Authenticate", fmt.Sprintf(`Bearer realm="%s/token"`, ts.URL))
				w.WriteHeader(http.StatusUnauthorized)
			case "/token":
				w.WriteHeader(http.StatusUnauthorized)
			}
		}))
		s := newTestGenericService()
		res := s.TestConnection(registryHostOf(ts), "u", "p", "", "", "ns", "", "")
		assert.False(t, res.LoginOK)
		assert.NotEmpty(t, res.LoginMessage)
	})

	t.Run("401无challenge", func(t *testing.T) {
		s := newTestGenericService()
		ts := newSvcTLSServer(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			w.WriteHeader(http.StatusUnauthorized)
		}))
		res := s.TestConnection(registryHostOf(ts), "u", "p", "", "", "ns", "", "")
		assert.False(t, res.LoginOK)
		assert.Contains(t, res.LoginMessage, "认证失败")
	})

	t.Run("异常状态码", func(t *testing.T) {
		s := newTestGenericService()
		ts := newSvcTLSServer(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			w.WriteHeader(http.StatusBadGateway)
		}))
		res := s.TestConnection(registryHostOf(ts), "u", "p", "", "", "ns", "", "")
		assert.False(t, res.LoginOK)
		assert.Contains(t, res.LoginMessage, "异常状态码 502")
	})

	t.Run("网络错误", func(t *testing.T) {
		s := newTestGenericService()
		res := s.TestConnection("127.0.0.1:1", "u", "p", "", "", "ns", "", "")
		assert.False(t, res.LoginOK)
		assert.Contains(t, res.LoginMessage, "连接失败")
	})
}
