package services

import (
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"net/http"
	"sync/atomic"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestInferArchFromTag(t *testing.T) {
	assert.Equal(t, "arm64", inferArchFromTag("app-ARM64-v2"))
	assert.Equal(t, "amd64", inferArchFromTag("app-amd64"))
	assert.Equal(t, "", inferArchFromTag("latest"))
	assert.Equal(t, "", inferArchFromTag(""))
}

func TestSortArchitectures(t *testing.T) {
	assert.Equal(t, []string{"amd64", "arm64"}, sortArchitectures([]string{"arm64", "amd64"}))
	assert.Equal(t, []string{"amd64", "arm64", "riscv64"}, sortArchitectures([]string{"riscv64", "arm64", "amd64"}))
	// 未知架构按字典序排在已知架构之后
	assert.Equal(t, []string{"amd64", "loong64", "mips"}, sortArchitectures([]string{"mips", "loong64", "amd64"}))
	// 不修改原切片
	src := []string{"arm64", "amd64"}
	_ = sortArchitectures(src)
	assert.Equal(t, []string{"arm64", "amd64"}, src)
	assert.Empty(t, sortArchitectures(nil))
}

func TestParseHTTPTime(t *testing.T) {
	assert.Empty(t, parseHTTPTime(""))

	// RFC1123（GMT 带 0 偏移）
	got := parseHTTPTime("Mon, 02 Jan 2006 15:04:05 GMT")
	assert.Equal(t, "2006-01-02T15:04:05Z", got)

	// RFC1123Z（显式时区偏移）
	got = parseHTTPTime("Mon, 02 Jan 2006 15:04:05 +0800")
	assert.Equal(t, "2006-01-02T15:04:05+08:00", got)

	// RFC3339
	got = parseHTTPTime("2006-01-02T15:04:05Z")
	assert.Equal(t, "2006-01-02T15:04:05Z", got)

	// RFC3339Nano 可解析，但统一按 RFC3339 输出（小数秒被丢弃）
	got = parseHTTPTime("2006-01-02T15:04:05.123456789Z")
	assert.Equal(t, "2006-01-02T15:04:05Z", got)

	// 无法解析时原样返回
	assert.Equal(t, "not-a-time", parseHTTPTime("not-a-time"))
}

func TestFirstNonEmpty(t *testing.T) {
	assert.Equal(t, "a", firstNonEmpty("a", "b"))
	assert.Equal(t, "b", firstNonEmpty("", "b"))
	assert.Empty(t, firstNonEmpty("", ""))
	assert.Empty(t, firstNonEmpty())
}

func TestEmptyTagDetail(t *testing.T) {
	d := emptyTagDetail("v1")
	assert.Equal(t, "v1", d.Tag)
	assert.Empty(t, d.Architectures)
	assert.NotNil(t, d.Digests)
	assert.NotNil(t, d.Sizes)
	assert.NotNil(t, d.PushedAt)
}

func TestComputeManifestDigest(t *testing.T) {
	body := []byte(`{"schemaVersion":2}`)
	sum := sha256.Sum256(body)
	want := "sha256:" + hex.EncodeToString(sum[:])
	assert.Equal(t, want, computeManifestDigest(body))
}

// ---------- fetchImageArch ----------

func TestFetchImageArchSkipsWithoutDigestOrToken(t *testing.T) {
	client := svcTLSRestyClient()
	ts := newSvcTLSServer(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		t.Error("不应发起任何请求")
	}))
	assert.Empty(t, fetchImageArch(client, registryHostOf(ts), "tok", "ns", "repo", ""))
	assert.Empty(t, fetchImageArch(client, registryHostOf(ts), "", "ns", "repo", "sha256:cfg"))
}

func TestFetchImageArchSuccessAndRetries(t *testing.T) {
	var hits int32
	ts := newSvcTLSServer(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch n := atomic.AddInt32(&hits, 1); {
		case r.URL.Path == "/v2/ns/repo/blobs/sha256:cfg" && n == 1:
			w.WriteHeader(http.StatusTooManyRequests) // 429 → 重试
		case r.URL.Path == "/v2/ns/repo/blobs/sha256:cfg":
			w.Header().Set("Content-Type", "application/json")
			_, _ = w.Write([]byte(`{"architecture":"arm64","os":"linux"}`))
		default:
			t.Errorf("意外请求: %s", r.URL.Path)
		}
	}))
	client := svcTLSRestyClient()

	// 429 后重试成功
	assert.Equal(t, "arm64", fetchImageArch(client, registryHostOf(ts), "tok", "ns", "repo", "sha256:cfg"))
	require.Equal(t, int32(2), atomic.LoadInt32(&hits))
}

func TestFetchImageArchFailures(t *testing.T) {
	// 非 linux OS
	ts := newSvcTLSServer(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"architecture":"amd64","os":"windows"}`))
	}))
	client := svcTLSRestyClient()
	assert.Empty(t, fetchImageArch(client, registryHostOf(ts), "tok", "ns", "repo", "sha256:cfg"))

	// 非 200
	ts2 := newSvcTLSServer(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusForbidden)
	}))
	assert.Empty(t, fetchImageArch(client, registryHostOf(ts2), "tok", "ns", "repo", "sha256:cfg"))

	// 畸形 JSON
	ts3 := newSvcTLSServer(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte(`{invalid`))
	}))
	assert.Empty(t, fetchImageArch(client, registryHostOf(ts3), "tok", "ns", "repo", "sha256:cfg"))

	// 网络错误
	assert.Empty(t, fetchImageArch(client, "127.0.0.1:1", "tok", "ns", "repo", "sha256:cfg"))
}

// ---------- fetchResourceLastModified / fetchManifestLastModified / fetchBlobLastModified ----------

func TestFetchResourceLastModified(t *testing.T) {
	client := svcTLSRestyClient()

	// 无 token 直接跳过
	assert.Empty(t, fetchResourceLastModified(client, "https://example.com/x", ""))

	ts := newSvcTLSServer(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodHead {
			t.Errorf("应使用 HEAD，实际 %s", r.Method)
		}
		w.Header().Set("Last-Modified", "Mon, 02 Jan 2006 15:04:05 GMT")
		w.WriteHeader(http.StatusOK)
	}))
	host := registryHostOf(ts)
	assert.Equal(t, "2006-01-02T15:04:05Z", fetchResourceLastModified(client, ts.URL+"/x", "tok"))
	assert.Equal(t, "2006-01-02T15:04:05Z",
		fetchManifestLastModified(client, host, "tok", "ns", "repo", "sha256:ref"))

	// 仅 Date 头兜底
	ts2 := newSvcTLSServer(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Date", "Mon, 02 Jan 2006 15:04:05 GMT")
		w.WriteHeader(http.StatusOK)
	}))
	assert.Equal(t, "2006-01-02T15:04:05Z", fetchResourceLastModified(client, ts2.URL+"/x", "tok"))

	// 非 200 → 空
	ts3 := newSvcTLSServer(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusNotFound)
	}))
	assert.Empty(t, fetchResourceLastModified(client, ts3.URL+"/x", "tok"))

	// 网络错误 → 空
	assert.Empty(t, fetchResourceLastModified(client, "https://127.0.0.1:1/x", "tok"))

	// 空 reference / digest 短路
	assert.Empty(t, fetchManifestLastModified(client, host, "tok", "ns", "repo", ""))
	assert.Empty(t, fetchBlobLastModified(client, host, "tok", "ns", "repo", ""))
}

// ---------- parseManifestToTagDetail ----------

const rcManifestList = `{
  "schemaVersion": 2,
  "mediaType": "application/vnd.docker.distribution.manifest.list.v2+json",
  "manifests": [
    {"mediaType":"application/vnd.docker.distribution.manifest.v2+json","size":1234,"digest":"sha256:aaa","platform":{"architecture":"amd64","os":"linux"}},
    {"mediaType":"application/vnd.docker.distribution.manifest.v2+json","size":2345,"digest":"sha256:bbb","platform":{"architecture":"arm64","os":"linux"}},
    {"mediaType":"application/vnd.docker.distribution.manifest.v2+json","size":99,"digest":"sha256:win","platform":{"architecture":"amd64","os":"windows"}}
  ]
}`

func TestParseManifestToTagDetailList(t *testing.T) {
	client := svcTLSRestyClient()

	// manifestLastModified 已提供：注意 Go 实参求值是急切的，
	// firstNonEmpty 的后续 HEAD 请求仍会发出，只是结果不被采用
	ts := newSvcTLSServer(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))
	detail, err := parseManifestToTagDetail(client, "v1", registryHostOf(ts), "ns", "repo", "tok",
		[]byte(rcManifestList), "sha256:list", "2006-01-02T15:04:05Z")
	require.NoError(t, err)
	assert.Equal(t, "v1", detail.Tag)
	assert.Equal(t, []string{"amd64", "arm64"}, detail.Architectures)
	assert.Equal(t, "sha256:aaa", detail.Digests["amd64"])
	assert.Equal(t, "sha256:bbb", detail.Digests["arm64"])
	assert.Equal(t, int64(1234), detail.Sizes["amd64"])
	assert.Equal(t, int64(2345), detail.Sizes["arm64"])
	assert.Equal(t, "2006-01-02T15:04:05Z", detail.PushedAt["amd64"])
	assert.Equal(t, "2006-01-02T15:04:05Z", detail.PushedAt["arm64"])

	// manifestLastModified 为空 → 每个 linux 子 manifest 触发 HEAD（manifest+blob 兜底链）
	headCount := 0
	ts2 := newSvcTLSServer(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		headCount++
		if r.Method != http.MethodHead {
			t.Errorf("应使用 HEAD，实际 %s", r.Method)
		}
		w.Header().Set("Last-Modified", "Tue, 03 Jan 2006 10:00:00 GMT")
		w.WriteHeader(http.StatusOK)
	}))
	detail2, err := parseManifestToTagDetail(client, "v1", registryHostOf(ts2), "ns", "repo", "tok",
		[]byte(rcManifestList), "sha256:list", "")
	require.NoError(t, err)
	assert.Equal(t, "2006-01-03T10:00:00Z", detail2.PushedAt["amd64"])
	// 2 个 linux 架构 × 2（实参急切求值：manifest HEAD + blob HEAD 各一次）= 4 次
	assert.Equal(t, 4, headCount, "仅对 linux 架构子 manifest 发起 HEAD")
}

const rcSingleManifest = `{
  "schemaVersion": 2,
  "mediaType": "application/vnd.docker.distribution.manifest.v2+json",
  "config": {"mediaType":"application/vnd.docker.container.image.v1+json","size":500,"digest":"sha256:cfg"},
  "layers": [
    {"mediaType":"application/vnd.docker.image.rootfs.diff.tar.gzip","size":100,"digest":"sha256:l1"},
    {"mediaType":"application/vnd.docker.image.rootfs.diff.tar.gzip","size":250,"digest":"sha256:l2"}
  ]
}`

func TestParseManifestToTagDetailSingleArch(t *testing.T) {
	client := svcTLSRestyClient()

	// 1) tag 自带架构信息（无需拉 config blob）
	ts := newSvcTLSServer(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Last-Modified", "Wed, 04 Jan 2006 08:00:00 GMT")
		w.WriteHeader(http.StatusOK)
	}))
	detail, err := parseManifestToTagDetail(client, "app-arm64", registryHostOf(ts), "ns", "repo", "tok",
		[]byte(rcSingleManifest), "sha256:single", "")
	require.NoError(t, err)
	assert.Equal(t, []string{"arm64"}, detail.Architectures)
	assert.Equal(t, "sha256:single", detail.Digests["arm64"])
	assert.Equal(t, int64(350), detail.Sizes["arm64"])
	assert.Equal(t, "2006-01-04T08:00:00Z", detail.PushedAt["arm64"])

	// 2) tag 无架构信息 → 拉取 config blob 判断架构
	ts2 := newSvcTLSServer(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case r.URL.Path == "/v2/ns/repo/blobs/sha256:cfg":
			w.Header().Set("Content-Type", "application/json")
			_, _ = w.Write([]byte(`{"architecture":"amd64","os":"linux"}`))
		default:
			w.Header().Set("Last-Modified", "Wed, 04 Jan 2006 08:00:00 GMT")
			w.WriteHeader(http.StatusOK)
		}
	}))
	detail2, err := parseManifestToTagDetail(client, "latest", registryHostOf(ts2), "ns", "repo", "tok",
		[]byte(rcSingleManifest), "", "")
	require.NoError(t, err)
	assert.Equal(t, []string{"amd64"}, detail2.Architectures)
	// contentDigest 为空时按 body 计算
	sum := sha256.Sum256([]byte(rcSingleManifest))
	assert.Equal(t, "sha256:"+hex.EncodeToString(sum[:]), detail2.Digests["amd64"])
	assert.Equal(t, int64(350), detail2.Sizes["amd64"])

	// 3) 无法推断架构 → 空架构详情
	ts3 := newSvcTLSServer(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"architecture":"amd64","os":"windows"}`))
	}))
	detail3, err := parseManifestToTagDetail(client, "latest", registryHostOf(ts3), "ns", "repo", "tok",
		[]byte(rcSingleManifest), "sha256:x", "")
	require.NoError(t, err)
	assert.Empty(t, detail3.Architectures)
	assert.Empty(t, detail3.Digests)
}

func TestParseManifestToTagDetailMalformed(t *testing.T) {
	client := svcTLSRestyClient()
	ts := newSvcTLSServer(t, http.NotFoundHandler())
	_, err := parseManifestToTagDetail(client, "v1", registryHostOf(ts), "ns", "repo", "tok",
		[]byte(`{broken`), "", "")
	require.Error(t, err)
	assert.Contains(t, err.Error(), "解析Manifest失败")
}

// ---------- fetchManifestWithRetry ----------

func rcTokenFunc(token string, err error, forceCalls *int) func(bool) (string, error) {
	return func(forceRefresh bool) (string, error) {
		if forceRefresh && forceCalls != nil {
			*forceCalls++
		}
		return token, err
	}
}

func TestFetchManifestWithRetrySuccess(t *testing.T) {
	var authHeaders []string
	ts := newSvcTLSServer(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		require.Equal(t, "/v2/ns/repo/manifests/v1", r.URL.Path)
		authHeaders = append(authHeaders, r.Header.Get("Authorization"))
		w.Header().Set("Docker-Content-Digest", "sha256:manifest")
		w.Header().Set("Last-Modified", "Mon, 02 Jan 2006 15:04:05 GMT")
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(rcManifestList))
	}))
	client := svcTLSRestyClient()

	body, digest, lastMod, status, err := fetchManifestWithRetry(client, registryHostOf(ts), "ns", "repo", "v1",
		rcTokenFunc("tok123", nil, nil))
	require.NoError(t, err)
	assert.Equal(t, http.StatusOK, status)
	assert.Equal(t, []byte(rcManifestList), body)
	assert.Equal(t, "sha256:manifest", digest)
	assert.Equal(t, "2006-01-02T15:04:05Z", lastMod)
	require.NotEmpty(t, authHeaders)
	assert.Equal(t, "Bearer tok123", authHeaders[0])
}

func TestFetchManifestWithRetryGetTokenError(t *testing.T) {
	client := svcTLSRestyClient()
	_, _, _, _, err := fetchManifestWithRetry(client, "reg.example.com", "ns", "repo", "v1",
		rcTokenFunc("", fmt.Errorf("token boom"), nil))
	require.Error(t, err)
	assert.Contains(t, err.Error(), "token boom")
}

func TestFetchManifestWithRetryNetworkError(t *testing.T) {
	client := svcTLSRestyClient()
	_, _, _, _, err := fetchManifestWithRetry(client, "127.0.0.1:1", "ns", "repo", "v1",
		rcTokenFunc("tok", nil, nil))
	require.Error(t, err)
	assert.Contains(t, err.Error(), "获取Manifest失败")
}

func TestFetchManifestWithRetry401Refresh(t *testing.T) {
	var hits int32
	ts := newSvcTLSServer(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if atomic.AddInt32(&hits, 1) == 1 {
			w.WriteHeader(http.StatusUnauthorized) // 首次 401 → 强制刷新 token
			return
		}
		w.Header().Set("Docker-Content-Digest", "sha256:ok")
		_, _ = w.Write([]byte(rcSingleManifest))
	}))
	client := svcTLSRestyClient()

	var forceCalls int
	_, digest, _, _, err := fetchManifestWithRetry(client, registryHostOf(ts), "ns", "repo", "v1",
		rcTokenFunc("tok", nil, &forceCalls))
	require.NoError(t, err)
	assert.Equal(t, "sha256:ok", digest)
	assert.Equal(t, 1, forceCalls, "401 后应强制刷新一次 token")
	require.Equal(t, int32(2), atomic.LoadInt32(&hits))
}

func TestFetchManifestWithRetry401RefreshFails(t *testing.T) {
	ts := newSvcTLSServer(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusUnauthorized)
	}))
	client := svcTLSRestyClient()

	// 刷新本身失败 → HTTP 401
	_, _, _, status, err := fetchManifestWithRetry(client, registryHostOf(ts), "ns", "repo", "v1",
		func(forceRefresh bool) (string, error) {
			if forceRefresh {
				return "", fmt.Errorf("refresh boom")
			}
			return "tok", nil
		})
	require.Error(t, err)
	assert.Equal(t, http.StatusUnauthorized, status)
	assert.Contains(t, err.Error(), "HTTP 401")
}

func TestFetchManifestWithRetryPersistent401(t *testing.T) {
	// 刷新成功但服务端持续 401 → 只重试一次，第二次 401 直接报错
	ts := newSvcTLSServer(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusUnauthorized)
	}))
	client := svcTLSRestyClient()

	var forceCalls int
	_, _, _, status, err := fetchManifestWithRetry(client, registryHostOf(ts), "ns", "repo", "v1",
		rcTokenFunc("tok", nil, &forceCalls))
	require.Error(t, err)
	assert.Equal(t, http.StatusUnauthorized, status)
	assert.Equal(t, 1, forceCalls)
}

func TestFetchManifestWithRetry429ThenSuccess(t *testing.T) {
	var hits int32
	ts := newSvcTLSServer(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if atomic.AddInt32(&hits, 1) == 1 {
			w.WriteHeader(http.StatusTooManyRequests) // 限流 → 退避重试
			return
		}
		w.Header().Set("Docker-Content-Digest", "sha256:after429")
		_, _ = w.Write([]byte(rcSingleManifest))
	}))
	client := svcTLSRestyClient()

	start := time.Now()
	_, digest, _, _, err := fetchManifestWithRetry(client, registryHostOf(ts), "ns", "repo", "v1",
		rcTokenFunc("tok", nil, nil))
	require.NoError(t, err)
	assert.Equal(t, "sha256:after429", digest)
	assert.GreaterOrEqual(t, time.Since(start), 300*time.Millisecond, "429 重试应有基础退避")
	require.Equal(t, int32(2), atomic.LoadInt32(&hits))
}

func TestFetchManifestWithRetryOtherStatus(t *testing.T) {
	ts := newSvcTLSServer(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
	}))
	client := svcTLSRestyClient()

	_, _, _, status, err := fetchManifestWithRetry(client, registryHostOf(ts), "ns", "repo", "v1",
		rcTokenFunc("tok", nil, nil))
	require.Error(t, err)
	assert.Equal(t, http.StatusInternalServerError, status)
	assert.Contains(t, err.Error(), "HTTP 500")
}
