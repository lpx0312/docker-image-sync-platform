package utils

import (
	"crypto/aes"
	"crypto/cipher"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"io"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"reflect"
	"strconv"
	"strings"
	"testing"

	"docker-image-sync-platform/internal/database"
	"docker-image-sync-platform/internal/logger"
	"docker-image-sync-platform/internal/models"

	"github.com/glebarez/sqlite"
	"go.uber.org/zap"
	"gorm.io/gorm"
	gormlogger "gorm.io/gorm/logger"
)

// ---------------------------------------------------------------------------
// 轻量断言助手（go.mod 中 testify 仅是 indirect 依赖且缺少完整的 go.sum 条目，
// 按任务约束不允许修改 go.mod / go.sum，因此使用标准库实现断言）
// ---------------------------------------------------------------------------

func noErr(t *testing.T, err error) {
	t.Helper()
	if err != nil {
		t.Fatalf("期望无错误, 实际: %v", err)
	}
}

func isErr(t *testing.T, err error) {
	t.Helper()
	if err == nil {
		t.Fatalf("期望有错误, 实际为 nil")
	}
}

func eq(t *testing.T, want, got interface{}) {
	t.Helper()
	if !reflect.DeepEqual(want, got) {
		t.Fatalf("不相等:\n期望: %#v\n实际: %#v", want, got)
	}
}

func neq(t *testing.T, want, got interface{}) {
	t.Helper()
	if reflect.DeepEqual(want, got) {
		t.Fatalf("不应相等: %#v", want)
	}
}

func contains(t *testing.T, s, sub string) {
	t.Helper()
	if !strings.Contains(s, sub) {
		t.Fatalf("%q 未包含 %q", s, sub)
	}
}

func errContains(t *testing.T, err error, sub string) {
	t.Helper()
	if err == nil {
		t.Fatalf("期望包含 %q 的错误, 实际为 nil", sub)
	}
	if !strings.Contains(err.Error(), sub) {
		t.Fatalf("错误 %q 未包含 %q", err.Error(), sub)
	}
}

func isTrue(t *testing.T, v bool) {
	t.Helper()
	if !v {
		t.Fatalf("期望 true, 实际 false")
	}
}

func isFalse(t *testing.T, v bool) {
	t.Helper()
	if v {
		t.Fatalf("期望 false, 实际 true")
	}
}

func isEmpty(t *testing.T, s string) {
	t.Helper()
	if s != "" {
		t.Fatalf("期望空字符串, 实际: %q", s)
	}
}

// ---------------------------------------------------------------------------
// 测试基础设施
// ---------------------------------------------------------------------------

// setupTestDB 为每个测试准备独立的 sqlite 数据库，并临时替换全局 database.DB 与 logger.Logger，
// 测试结束后恢复，避免测试间全局状态污染。
func setupTestDB(t *testing.T) *gorm.DB {
	t.Helper()

	oldDB := database.DB
	oldLogger := logger.Logger

	db, err := gorm.Open(sqlite.Open(filepath.Join(t.TempDir(), "test.db")), &gorm.Config{
		Logger: gormlogger.Default.LogMode(gormlogger.Silent),
	})
	noErr(t, err)
	noErr(t, db.AutoMigrate(&models.SystemConfig{}, &models.AcrRegistry{}))

	database.DB = db
	logger.Logger = zap.NewNop()

	t.Cleanup(func() {
		database.DB = oldDB
		logger.Logger = oldLogger
		if sqlDB, err := db.DB(); err == nil {
			_ = sqlDB.Close()
		}
	})
	return db
}

// encryptForTest 使用与生产一致的 AES-GCM 算法加密明文（ENC: 前缀格式），
// 便于测试 DecryptSystemConfigValue 及凭据解密链路。
func encryptForTest(t *testing.T, plaintext string) string {
	t.Helper()
	key, err := encryptionKeyBytes()
	noErr(t, err)

	block, err := aes.NewCipher(key)
	noErr(t, err)
	gcm, err := cipher.NewGCM(block)
	noErr(t, err)

	nonce := make([]byte, gcm.NonceSize())
	_, err = io.ReadFull(rand.Reader, nonce)
	noErr(t, err)

	sealed := gcm.Seal(nonce, nonce, []byte(plaintext), nil)
	return "ENC:" + base64.StdEncoding.EncodeToString(sealed)
}

// writeSystemConfig 写入一条 system_configs 记录
func writeSystemConfig(t *testing.T, key, value string, encrypted bool) {
	t.Helper()
	noErr(t, database.DB.Create(&models.SystemConfig{
		ConfigKey:   key,
		ConfigValue: value,
		IsEncrypted: encrypted,
	}).Error)
}

// newTestRegistry 启动模拟 OCI Registry 的 httptest 服务器，返回 host（127.0.0.1:port）
func newTestRegistry(t *testing.T, h http.Handler) string {
	t.Helper()
	srv := httptest.NewServer(h)
	t.Cleanup(srv.Close)
	return strings.TrimPrefix(srv.URL, "http://")
}

// redirectHTTPTransport 把生产代码中硬编码的 https 请求重定向到本地 httptest 服务器。
// 用于测试 detectArchitecturesHTTPFallback（该函数手拼 https:// URL）。
func redirectHTTPTransport(t *testing.T, host string) {
	t.Helper()
	orig := http.DefaultTransport
	http.DefaultTransport = redirectRoundTripper{host: host, base: orig}
	t.Cleanup(func() { http.DefaultTransport = orig })
}

type redirectRoundTripper struct {
	host string
	base http.RoundTripper
}

func (r redirectRoundTripper) RoundTrip(req *http.Request) (*http.Response, error) {
	clone := req.Clone(req.Context())
	clone.URL.Scheme = "http"
	clone.URL.Host = r.host
	return r.base.RoundTrip(clone)
}

// sha256Digest 计算 docker 格式摘要 sha256:xxx
func sha256Digest(b []byte) string {
	h := sha256.Sum256(b)
	return "sha256:" + hex.EncodeToString(h[:])
}

// manifestListBody 构造一个多架构 manifest list 响应体
func manifestListBody(t *testing.T) []byte {
	t.Helper()
	return []byte(`{
  "schemaVersion": 2,
  "mediaType": "application/vnd.docker.distribution.manifest.list.v2+json",
  "manifests": [
    {"digest":"sha256:` + strings.Repeat("a", 64) + `","size":10,"mediaType":"application/vnd.docker.distribution.manifest.v2+json","platform":{"architecture":"arm64","os":"linux"}},
    {"digest":"sha256:` + strings.Repeat("b", 64) + `","size":10,"mediaType":"application/vnd.docker.distribution.manifest.v2+json","platform":{"architecture":"amd64","os":"linux"}},
    {"digest":"sha256:` + strings.Repeat("c", 64) + `","size":10,"mediaType":"application/vnd.docker.distribution.manifest.v2+json","platform":{"architecture":"amd64","os":"linux"}},
    {"digest":"sha256:` + strings.Repeat("d", 64) + `","size":10,"mediaType":"application/vnd.docker.distribution.manifest.v2+json","platform":{"architecture":"arm64","os":"unknown"}},
    {"digest":"sha256:` + strings.Repeat("e", 64) + `","size":10,"mediaType":"application/vnd.docker.distribution.manifest.v2+json","platform":{"architecture":"ppc64le","os":"linux"}},
    {"digest":"sha256:` + strings.Repeat("f", 64) + `","size":10,"mediaType":"application/vnd.docker.distribution.manifest.v2+json","platform":{"architecture":"amd64","os":"windows"}},
    {"digest":"sha256:` + strings.Repeat("1", 64) + `","size":10,"mediaType":"application/vnd.docker.distribution.manifest.v2+json","platform":{"architecture":"unknown","os":"unknown"}},
    {"digest":"sha256:` + strings.Repeat("2", 64) + `","size":10,"mediaType":"application/vnd.docker.distribution.manifest.v2+json"}
  ]
}`)
}

// writeJSON 写 JSON 响应
func writeJSON(w http.ResponseWriter, status int, body []byte, contentType string) {
	if contentType == "" {
		contentType = "application/json"
	}
	w.Header().Set("Content-Type", contentType)
	w.Header().Set("Content-Length", strconv.Itoa(len(body)))
	w.WriteHeader(status)
	_, _ = w.Write(body)
}
