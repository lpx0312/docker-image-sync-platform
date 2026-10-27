package utils

import (
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"strings"
	"sync"
	"testing"

	"docker-image-sync-platform/internal/models"

	"github.com/google/go-containerregistry/pkg/authn"
	"github.com/google/go-containerregistry/pkg/name"
	v1 "github.com/google/go-containerregistry/pkg/v1"
	"github.com/google/go-containerregistry/pkg/v1/types"
)

// ---------------------------------------------------------------------------
// systemConfigValue / authFromCredentials / getACRAuth*
// ---------------------------------------------------------------------------

func TestSystemConfigValue(t *testing.T) {
	setupTestDB(t)
	writeSystemConfig(t, "some_key", "some_value", false)

	eq(t, "some_value", systemConfigValue("some_key"))

	// 不存在返回空串
	isEmpty(t, systemConfigValue("missing"))
}

func TestAuthFromCredentials(t *testing.T) {
	setupTestDB(t)

	// 用户名为空 -> 匿名
	eq(t, authn.Anonymous, authFromCredentials("", "whatever"))

	// 密码为空 -> 匿名
	eq(t, authn.Anonymous, authFromCredentials("user", ""))

	// 明文密码（无 ENC: 前缀，原样使用）
	basic, ok := authFromCredentials("user", "plain-pass").(*authn.Basic)
	isTrue(t, ok)
	eq(t, "user", basic.Username)
	eq(t, "plain-pass", basic.Password)

	// 加密密码成功解密
	enc := encryptForTest(t, "secret-pass")
	basic, ok = authFromCredentials("user", enc).(*authn.Basic)
	isTrue(t, ok)
	eq(t, "user", basic.Username)
	eq(t, "secret-pass", basic.Password)

	// 解密失败（非法 ENC: 密文）回退为把密文当明文使用（当前实际行为）
	basic, ok = authFromCredentials("user", "ENC:!!!invalid").(*authn.Basic)
	isTrue(t, ok)
	eq(t, "user", basic.Username)
	eq(t, "ENC:!!!invalid", basic.Password)

	// release 模式无 ENCRYPTION_KEY： 解密报错同样走明文回退
	t.Setenv("GIN_MODE", "release")
	basic, ok = authFromCredentials("user", "ENC:any").(*authn.Basic)
	isTrue(t, ok)
	eq(t, "ENC:any", basic.Password)
}

func TestGetACRAuth(t *testing.T) {
	setupTestDB(t)

	// 无配置 -> 匿名
	eq(t, authn.Anonymous, getACRAuth())

	// 系统默认配置生效
	writeSystemConfig(t, "aliyun_username", "sysuser", false)
	writeSystemConfig(t, "aliyun_password", encryptForTest(t, "syspass"), false)

	basic, ok := getACRAuth().(*authn.Basic)
	isTrue(t, ok)
	eq(t, "sysuser", basic.Username)
	eq(t, "syspass", basic.Password)
}

func TestGetACRAuthByRegistryID(t *testing.T) {
	db := setupTestDB(t)

	// 合法 ACR 记录： 使用该记录的凭据
	row := models.AcrRegistry{
		RegistryURL: "registry.custom.io", Namespace: "ns",
		Username: "acruser", Password: encryptForTest(t, "acrpass"),
	}
	noErr(t, db.Create(&row).Error)

	basic, ok := getACRAuthByRegistryID(row.ID).(*authn.Basic)
	isTrue(t, ok)
	eq(t, "acruser", basic.Username)
	eq(t, "acrpass", basic.Password)

	// 记录不存在的 ID： 回退到系统默认配置
	writeSystemConfig(t, "aliyun_username", "sysuser", false)
	writeSystemConfig(t, "aliyun_password", "syspass", false)

	basic, ok = getACRAuthByRegistryID(424242).(*authn.Basic)
	isTrue(t, ok)
	eq(t, "sysuser", basic.Username)

	// ID=0： 直接使用系统默认配置
	basic, ok = getACRAuthByRegistryID(0).(*authn.Basic)
	isTrue(t, ok)
	eq(t, "sysuser", basic.Username)
	eq(t, "syspass", basic.Password)
}

// ---------------------------------------------------------------------------
// platformsFromIndex
// ---------------------------------------------------------------------------

type fakeIndex struct {
	m   *v1.IndexManifest
	err error
}

func (f *fakeIndex) MediaType() (types.MediaType, error) { return types.DockerManifestList, nil }
func (f *fakeIndex) Digest() (v1.Hash, error) {
	return v1.NewHash("sha256:" + strings.Repeat("0", 64))
}
func (f *fakeIndex) Size() (int64, error)           { return 0, nil }
func (f *fakeIndex) RawManifest() ([]byte, error)   { return json.Marshal(f.m) }
func (f *fakeIndex) IndexManifest() (*v1.IndexManifest, error) {
	if f.err != nil {
		return nil, f.err
	}
	return f.m, nil
}
func (f *fakeIndex) Image(v1.Hash) (v1.Image, error)           { return nil, errors.New("not implemented") }
func (f *fakeIndex) ImageIndex(v1.Hash) (v1.ImageIndex, error) { return nil, errors.New("not implemented") }

var descSeq int

func desc(arch, os string) v1.Descriptor {
	descSeq++
	return v1.Descriptor{
		Digest:    v1.Hash{Algorithm: "sha256", Hex: strings.Repeat(fmt.Sprintf("%02x", descSeq%256), 32)},
		MediaType: types.DockerManifestSchema2,
		Platform:  &v1.Platform{Architecture: arch, OS: os},
	}
}

func TestPlatformsFromIndex(t *testing.T) {
	idx := &fakeIndex{m: &v1.IndexManifest{
		SchemaVersion: 2,
		MediaType:     types.DockerManifestList,
		Manifests: []v1.Descriptor{
			desc("arm64", "linux"),
			desc("amd64", "linux"),
			desc("amd64", "linux"),                   // 重复架构应去重
			{MediaType: types.DockerManifestSchema2}, // platform 为 nil 跳过
			desc("unknown", "unknown"),               // attestation 跳过
			desc("amd64", "windows"),                 // 非 linux 跳过
			desc("", "linux"),                        // 空架构跳过
			desc("unknown", "linux"),                 // unknown 架构跳过
			desc("arm", "linux"),
		},
	}}

	out, err := platformsFromIndex(idx)
	noErr(t, err)
	eq(t, []string{"amd64", "arm", "arm64"}, out)

	// 空索引
	out, err = platformsFromIndex(&fakeIndex{m: &v1.IndexManifest{}})
	noErr(t, err)
	eq(t, 0, len(out))

	// IndexManifest 出错
	_, err = platformsFromIndex(&fakeIndex{err: errors.New("fetch failed")})
	isErr(t, err)
}

// ---------------------------------------------------------------------------
// parseArchitecturesFromManifestJSON
// ---------------------------------------------------------------------------

func TestParseArchitecturesFromManifestJSON(t *testing.T) {
	// manifest list 正常路径 + 过滤/去重
	body := []byte(`{"schemaVersion":2,"manifests":[
		{"digest":"sha256:a","platform":{"os":"linux","architecture":"arm64"}},
		{"digest":"sha256:b","platform":{"os":"linux","architecture":"amd64"}},
		{"digest":"sha256:c","platform":{"os":"linux","architecture":"amd64"}},
		{"digest":"sha256:d","platform":{"os":"windows","architecture":"amd64"}},
		{"digest":"sha256:e","platform":{"os":"unknown","architecture":"unknown"}},
		{"digest":"sha256:f"},
		{"digest":"sha256:g","platform":{"os":"linux","architecture":"unknown"}},
		"not-a-map"
	]}`)
	out, err := parseArchitecturesFromManifestJSON(body)
	noErr(t, err)
	eq(t, []string{"amd64", "arm64"}, out)

	// schema1 单架构镜像
	out, err = parseArchitecturesFromManifestJSON([]byte(`{"schemaVersion":1,"architecture":"amd64"}`))
	noErr(t, err)
	eq(t, []string{"amd64"}, out)

	// schema1 但架构非法 -> 走 manifests 分支并报错
	_, err = parseArchitecturesFromManifestJSON([]byte(`{"schemaVersion":1,"architecture":"unknown"}`))
	isErr(t, err)

	// schema2 顶层 architecture 字段被忽略
	out, err = parseArchitecturesFromManifestJSON([]byte(`{"schemaVersion":2,"architecture":"amd64","manifests":[{"platform":{"os":"linux","architecture":"arm64"}}]}`))
	noErr(t, err)
	eq(t, []string{"arm64"}, out)

	// 非法 JSON
	_, err = parseArchitecturesFromManifestJSON([]byte(`{invalid`))
	isErr(t, err)

	// 无 manifests 数组
	_, err = parseArchitecturesFromManifestJSON([]byte(`{"schemaVersion":2}`))
	errContains(t, err, "manifest 中无 manifests 数组")

	// manifests 数组为空 / 全部被过滤
	_, err = parseArchitecturesFromManifestJSON([]byte(`{"schemaVersion":2,"manifests":[]}`))
	errContains(t, err, "manifest 中未解析到有效架构")

	_, err = parseArchitecturesFromManifestJSON([]byte(`{"schemaVersion":2,"manifests":[{"platform":{"os":"windows","architecture":"amd64"}}]}`))
	errContains(t, err, "manifest 中未解析到有效架构")
}

// ---------------------------------------------------------------------------
// detectArchitecturesHTTPFallback
// ---------------------------------------------------------------------------

func parseRef(t *testing.T, s string) name.Reference {
	t.Helper()
	ref, err := name.ParseReference(s)
	noErr(t, err)
	return ref
}

func TestDetectArchitecturesHTTPFallback_Success(t *testing.T) {
	setupTestDB(t)
	// 生产代码硬编码 https://，这里通过替换 http.DefaultTransport 重定向到本地 httptest
	host := newTestRegistry(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case r.URL.Path == "/v2/":
			w.WriteHeader(http.StatusOK)
		default:
			writeJSON(w, http.StatusOK, manifestListBody(t), "application/vnd.docker.distribution.manifest.list.v2+json")
		}
	}))
	redirectHTTPTransport(t, host)

	out, err := detectArchitecturesHTTPFallback(t.Context(), parseRef(t, host+"/myrepo:tag"), 0)
	noErr(t, err)
	eq(t, []string{"amd64", "arm64", "ppc64le"}, out)
}

func TestDetectArchitecturesHTTPFallback_Schema1AndErrors(t *testing.T) {
	setupTestDB(t)

	t.Run("schema1 响应", func(t *testing.T) {
		host := newTestRegistry(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			writeJSON(w, http.StatusOK, []byte(`{"schemaVersion":1,"architecture":"arm"}`), "")
		}))
		redirectHTTPTransport(t, host)
		out, err := detectArchitecturesHTTPFallback(t.Context(), parseRef(t, host+"/repo:t"), 0)
		noErr(t, err)
		eq(t, []string{"arm"}, out)
	})

	t.Run("非 200 状态码", func(t *testing.T) {
		host := newTestRegistry(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			writeJSON(w, http.StatusInternalServerError, []byte("boom"), "")
		}))
		redirectHTTPTransport(t, host)
		_, err := detectArchitecturesHTTPFallback(t.Context(), parseRef(t, host+"/repo:t"), 0)
		errContains(t, err, "manifest HTTP 500")
		contains(t, err.Error(), "boom")
	})

	t.Run("连接失败", func(t *testing.T) {
		// 指向无监听的端口，连接拒绝，快速失败
		_, err := detectArchitecturesHTTPFallback(t.Context(), parseRef(t, "127.0.0.1:1/repo:t"), 0)
		isErr(t, err)
	})

	t.Run("响应体读取中断", func(t *testing.T) {
		// 声明较大的 Content-Length，先刷出部分响应再立即断开连接，触发 io.ReadAll 错误
		host := newTestRegistry(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			w.Header().Set("Content-Type", "application/json")
			w.Header().Set("Content-Length", "1024")
			w.WriteHeader(http.StatusOK)
			_, _ = w.Write([]byte(`{"schemaVersion":2,"manifests":[`))
			if f, ok := w.(http.Flusher); ok {
				f.Flush() // 确保部分响应已送达客户端
			}
			panic(http.ErrAbortHandler) // 立即断开连接
		}))
		redirectHTTPTransport(t, host)
		_, err := detectArchitecturesHTTPFallback(t.Context(), parseRef(t, host+"/repo:t"), 0)
		isErr(t, err)
	})

	t.Run("带凭据时携带 Basic Auth 头", func(t *testing.T) {
		db := setupTestDB(t)
		row := models.AcrRegistry{
			RegistryURL: "reg.io", Namespace: "ns",
			Username: "fallback-user", Password: encryptForTest(t, "fallback-pass"),
		}
		noErr(t, db.Create(&row).Error)

		var gotAuth string
		host := newTestRegistry(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			gotAuth = r.Header.Get("Authorization")
			writeJSON(w, http.StatusOK, []byte(`{"schemaVersion":1,"architecture":"amd64"}`), "")
		}))
		redirectHTTPTransport(t, host)

		out, err := detectArchitecturesHTTPFallback(t.Context(), parseRef(t, host+"/repo:t"), row.ID)
		noErr(t, err)
		eq(t, []string{"amd64"}, out)

		// 校验 Basic 头是 username:password
		contains(t, gotAuth, "Basic ")
		raw, err := base64.StdEncoding.DecodeString(strings.TrimPrefix(gotAuth, "Basic "))
		noErr(t, err)
		eq(t, "fallback-user:fallback-pass", string(raw))
	})
}

// ---------------------------------------------------------------------------
// DetectImageArchitecturesInRegistry（e2e： httptest 模拟 OCI registry）
// ---------------------------------------------------------------------------

func TestDetectImageArchitecturesInRegistry_IndexPath(t *testing.T) {
	setupTestDB(t)

	var mu sync.Mutex
	var sawAuthHeader bool
	body := manifestListBody(t)
	host := newTestRegistry(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case r.URL.Path == "/v2/" || r.URL.Path == "/v2":
			w.WriteHeader(http.StatusOK)
		case r.Method == http.MethodGet && strings.Contains(r.URL.Path, "/manifests/"):
			mu.Lock()
			sawAuthHeader = r.Header.Get("Authorization") != ""
			mu.Unlock()
			// 非 fallback 的 Accept（remote.*）才返回 index；fallback 请求返回 500 防止误命中
			if r.Header.Get("Accept") == manifestAcceptFallback {
				writeJSON(w, http.StatusInternalServerError, []byte("should-not-happen"), "")
				return
			}
			writeJSON(w, http.StatusOK, body, "application/vnd.docker.distribution.manifest.list.v2+json")
		default:
			http.NotFound(w, r)
		}
	}))

	out, err := DetectImageArchitecturesInRegistry(host+"/myrepo:v1", 0)
	noErr(t, err)
	eq(t, []string{"amd64", "arm64", "ppc64le"}, out)

	mu.Lock()
	defer mu.Unlock()
	// 匿名认证（无系统配置）不应发送 Authorization 头
	isFalse(t, sawAuthHeader)
}

func TestDetectImageArchitecturesInRegistry_SingleImagePath(t *testing.T) {
	setupTestDB(t)

	// 单架构镜像： manifest v2 + config blob
	configJSON := []byte(`{"architecture":"arm64","os":"linux","created":"2024-01-01T00:00:00Z"}`)
	configDigest := sha256Digest(configJSON)
	manifest := []byte(fmt.Sprintf(`{"schemaVersion":2,"mediaType":"application/vnd.docker.distribution.manifest.v2+json","config":{"mediaType":"application/vnd.docker.container.image.v1+json","digest":"%s","size":%d},"layers":[]}`, configDigest, len(configJSON)))

	host := newTestRegistry(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case r.URL.Path == "/v2/" || r.URL.Path == "/v2":
			w.WriteHeader(http.StatusOK)
		case strings.Contains(r.URL.Path, "/manifests/"):
			writeJSON(w, http.StatusOK, manifest, "application/vnd.docker.distribution.manifest.v2+json")
		case strings.Contains(r.URL.Path, "/blobs/"):
			writeJSON(w, http.StatusOK, configJSON, "application/octet-stream")
		default:
			http.NotFound(w, r)
		}
	}))

	// remote.Index 对单镜像 manifest 会失败，随后 remote.Image 成功并读取 config 架构
	out, err := DetectImageArchitecturesInRegistry(host+"/single/img:v1", 0)
	noErr(t, err)
	eq(t, []string{"arm64"}, out)
}

func TestDetectImageArchitecturesInRegistry_HTTPFallbackPath(t *testing.T) {
	setupTestDB(t)

	schema1 := []byte(`{"schemaVersion":1,"architecture":"amd64"}`)
	host := newTestRegistry(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case r.URL.Path == "/v2/" || r.URL.Path == "/v2":
			w.WriteHeader(http.StatusOK)
		case strings.Contains(r.URL.Path, "/manifests/"):
			// remote.* 的请求（Accept 与 fallback 常量不同）一律 404；
			// 仅 HTTP fallback 的 Accept 命中 schema1 响应
			if r.Header.Get("Accept") == manifestAcceptFallback {
				writeJSON(w, http.StatusOK, schema1, "application/json")
				return
			}
			writeJSON(w, http.StatusNotFound, []byte(`{"errors":[{"code":"MANIFEST_UNKNOWN","message":"not found"}]}`), "application/json")
		default:
			http.NotFound(w, r)
		}
	}))
	// fallback 内部手拼 https:// URL，需要重定向
	redirectHTTPTransport(t, host)

	out, err := DetectImageArchitecturesInRegistry(host+"/fallback/repo:v1", 0)
	noErr(t, err)
	eq(t, []string{"amd64"}, out)
}

func TestDetectImageArchitecturesInRegistry_Failures(t *testing.T) {
	setupTestDB(t)

	t.Run("非法镜像引用", func(t *testing.T) {
		_, err := DetectImageArchitecturesInRegistry("in valid ref!!", 0)
		isErr(t, err)

		_, err = DetectImageArchitecturesInRegistry("", 0)
		isErr(t, err)
	})

	t.Run("全部探测途径失败", func(t *testing.T) {
		// 端口 1 通常无监听，连接被拒绝，快速失败
		_, err := DetectImageArchitecturesInRegistry("127.0.0.1:1/dead/repo:v1", 0)
		errContains(t, err, "无法解析镜像 manifest 中的架构信息")
	})
}

// ---------------------------------------------------------------------------
// ArchitecturesToJSON
// ---------------------------------------------------------------------------

func TestArchitecturesToJSON(t *testing.T) {
	eq(t, "[]", ArchitecturesToJSON(nil))
	eq(t, "[]", ArchitecturesToJSON([]string{}))
	eq(t, `["amd64","arm64"]`, ArchitecturesToJSON([]string{"amd64", "arm64"}))
	eq(t, `["x86_64","中文"]`, ArchitecturesToJSON([]string{"x86_64", "中文"}))
}
