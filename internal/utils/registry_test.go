package utils

import (
	"encoding/base64"
	"net/http"
	"strings"
	"sync"
	"testing"

	"docker-image-sync-platform/internal/models"
)

// manifestHeadHandler 返回一个对 HEAD manifests 返回 200 的模拟 registry handler
func manifestHeadHandler(digestBody []byte, mu *sync.Mutex, gotAuth *string) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		switch {
		case r.URL.Path == "/v2/" || r.URL.Path == "/v2":
			w.WriteHeader(http.StatusOK)
		case r.Method == http.MethodHead && strings.Contains(r.URL.Path, "/manifests/"):
			if mu != nil && gotAuth != nil {
				mu.Lock()
				*gotAuth = r.Header.Get("Authorization")
				mu.Unlock()
			}
			w.Header().Set("Content-Type", "application/vnd.docker.distribution.manifest.v2+json")
			w.Header().Set("Docker-Content-Digest", sha256Digest(digestBody))
			w.Header().Set("Content-Length", "1024")
			w.WriteHeader(http.StatusOK)
		default:
			http.NotFound(w, r)
		}
	}
}

func TestCheckImageExistsInRegistryWithErr_Exists(t *testing.T) {
	setupTestDB(t)
	body := manifestListBody(t)
	host := newTestRegistry(t, manifestHeadHandler(body, nil, nil))

	exists, err := CheckImageExistsInRegistryWithErr(host+"/myrepo:v1", 0)
	noErr(t, err)
	isTrue(t, exists)
}

func TestCheckImageExistsInRegistryWithErr_NotFound(t *testing.T) {
	setupTestDB(t)
	host := newTestRegistry(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case r.URL.Path == "/v2/" || r.URL.Path == "/v2":
			w.WriteHeader(http.StatusOK)
		default:
			// 404 + manifest unknown： 生产代码识别为“不存在”而非错误
			writeJSON(w, http.StatusNotFound,
				[]byte(`{"errors":[{"code":"MANIFEST_UNKNOWN","message":"manifest unknown"}]}`), "application/json")
		}
	}))

	exists, err := CheckImageExistsInRegistryWithErr(host+"/missing:v1", 0)
	noErr(t, err)
	isFalse(t, exists)
}

func TestCheckImageExistsInRegistryWithErr_Errors(t *testing.T) {
	setupTestDB(t)

	t.Run("非法镜像引用", func(t *testing.T) {
		exists, err := CheckImageExistsInRegistryWithErr("in valid!!", 0)
		isErr(t, err)
		isFalse(t, exists)
	})

	t.Run("服务器 500 错误", func(t *testing.T) {
		host := newTestRegistry(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			switch {
			case r.URL.Path == "/v2/" || r.URL.Path == "/v2":
				w.WriteHeader(http.StatusOK)
			default:
				writeJSON(w, http.StatusInternalServerError, []byte("internal error"), "")
			}
		}))
		exists, err := CheckImageExistsInRegistryWithErr(host+"/myrepo:v1", 0)
		errContains(t, err, "500")
		isFalse(t, exists)
	})

	t.Run("连接拒绝", func(t *testing.T) {
		exists, err := CheckImageExistsInRegistryWithErr("127.0.0.1:1/repo:v1", 0)
		isErr(t, err)
		isFalse(t, exists)
	})
}

func TestCheckImageExistsInRegistry_Wrapper(t *testing.T) {
	setupTestDB(t)

	t.Run("存在", func(t *testing.T) {
		host := newTestRegistry(t, manifestHeadHandler(manifestListBody(t), nil, nil))
		isTrue(t, CheckImageExistsInRegistry(host+"/repo:v1", 0))
	})

	t.Run("不存在", func(t *testing.T) {
		host := newTestRegistry(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			switch {
			case r.URL.Path == "/v2/" || r.URL.Path == "/v2":
				w.WriteHeader(http.StatusOK)
			default:
				writeJSON(w, http.StatusNotFound,
					[]byte(`{"errors":[{"code":"MANIFEST_UNKNOWN","message":"manifest unknown"}]}`), "application/json")
			}
		}))
		isFalse(t, CheckImageExistsInRegistry(host+"/repo:none", 0))
	})

	t.Run("错误时安全返回 false", func(t *testing.T) {
		isFalse(t, CheckImageExistsInRegistry("127.0.0.1:1/repo:v1", 0))
		isFalse(t, CheckImageExistsInRegistry("bad ref!!", 0))
	})
}

// e2e: 通过 401 Basic 质询验证 ACR 凭据（含解密）真实用于 registry 请求
func TestCheckImageExistsInRegistryWithErr_BasicAuthE2E(t *testing.T) {
	db := setupTestDB(t)

	row := models.AcrRegistry{
		RegistryURL: "registry.custom.io",
		Namespace:   "ns",
		Username:    "e2e-user",
		Password:    encryptForTest(t, "e2e-pass"),
	}
	noErr(t, db.Create(&row).Error)

	var mu sync.Mutex
	var gotAuth string
	body := manifestListBody(t)
	host := newTestRegistry(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case r.URL.Path == "/v2/" || r.URL.Path == "/v2":
			// Basic 质询： 触发 basicTransport 携带凭据
			w.Header().Set("WWW-Authenticate", `Basic realm="test"`)
			w.WriteHeader(http.StatusUnauthorized)
		case r.Method == http.MethodHead && strings.Contains(r.URL.Path, "/manifests/"):
			mu.Lock()
			gotAuth = r.Header.Get("Authorization")
			mu.Unlock()
			if gotAuth == "" {
				w.WriteHeader(http.StatusUnauthorized)
				return
			}
			w.Header().Set("Content-Type", "application/vnd.docker.distribution.manifest.v2+json")
			w.Header().Set("Docker-Content-Digest", sha256Digest(body))
			w.Header().Set("Content-Length", "1024")
			w.WriteHeader(http.StatusOK)
		default:
			http.NotFound(w, r)
		}
	}))

	exists, err := CheckImageExistsInRegistryWithErr(host+"/authrepo:v1", row.ID)
	noErr(t, err)
	isTrue(t, exists)

	mu.Lock()
	defer mu.Unlock()
	if gotAuth == "" {
		t.Fatalf("应携带 Basic Authorization 头")
	}
	raw, err := base64.StdEncoding.DecodeString(strings.TrimPrefix(gotAuth, "Basic "))
	noErr(t, err)
	// 验证解密链路： DB 中的 ENC: 密文被正确解密后用于认证
	eq(t, "e2e-user:e2e-pass", string(raw))
}
