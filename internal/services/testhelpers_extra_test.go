package services

// 本文件为组1（registry/acr/swr/ccr/generic/config/auth/user/role/encryption 等）
// 测试提供共享脚手架。不修改 testsupport_test.go（其中的 newTestDB 在 sqlite 上
// 因 ImageSyncRecord/SyncTask 的 MySQL enum 标签必然失败，此处提供兼容版本）。

import (
	"crypto/tls"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"docker-image-sync-platform/internal/config"
	"docker-image-sync-platform/internal/models"

	"github.com/glebarez/sqlite"
	"github.com/go-resty/resty/v2"
	"github.com/sirupsen/logrus"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"
	gl "gorm.io/gorm/logger"
)

// newSvcTestDB 打开内存 SQLite 并完成全部模型迁移。
//
// ImageSyncRecord/SyncTask 带 MySQL enum 标签，sqlite 解析不了
// enum('pending',...) 列类型，参照 internal/database/main_test.go 的
// preCreateMySQLCompatTables 方案先预建兼容表，再迁移其余模型。
func newSvcTestDB(t *testing.T) *gorm.DB {
	t.Helper()
	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{
		Logger:                                   gl.Default.LogMode(gl.Silent),
		DisableForeignKeyConstraintWhenMigrating: true,
	})
	require.NoError(t, err)
	sqlDB, err := db.DB()
	require.NoError(t, err)
	sqlDB.SetMaxOpenConns(1)

	require.NoError(t, db.Exec(`CREATE TABLE IF NOT EXISTS image_sync_records (
		id integer PRIMARY KEY AUTOINCREMENT,
		original_image varchar(500) NOT NULL,
		acr_image varchar(500),
		tag varchar(100),
		architecture varchar(50) DEFAULT "",
		acr_architectures text,
		acr_registry_id integer,
		original_input varchar(600),
		input_order integer DEFAULT 0,
		sync_status enum DEFAULT "pending",
		error_message text,
		description varchar(500),
		task_id varchar(100),
		priority integer DEFAULT 0,
		started_at datetime,
		completed_at datetime,
		duration integer DEFAULT 0,
		image_size integer DEFAULT 0,
		created_at datetime,
		updated_at datetime,
		deleted_at datetime
	)`).Error)
	require.NoError(t, db.Exec(`CREATE TABLE IF NOT EXISTS sync_tasks (
		id integer PRIMARY KEY AUTOINCREMENT,
		task_id varchar(100) NOT NULL,
		images_json text,
		status enum DEFAULT "pending",
		github_action_url varchar(500),
		github_run_id varchar(100),
		commit_sha varchar(100),
		started_at datetime,
		completed_at datetime,
		error_message text,
		acr_registry_id integer DEFAULT 0,
		description varchar(500),
		max_concurrent integer DEFAULT 3,
		total_images integer DEFAULT 0,
		completed_images integer DEFAULT 0,
		failed_images integer DEFAULT 0,
		progress decimal(5,2) DEFAULT 0.00,
		created_at datetime,
		updated_at datetime,
		deleted_at datetime
	)`).Error)

	require.NoError(t, db.AutoMigrate(
		&models.Permission{}, &models.Role{},
		&models.SystemConfig{},
		&models.User{}, &models.LoginLog{},
		&models.AcrRegistry{}, &models.AcrRepository{},
	))
	return db
}

// svcClosedDB 返回底层连接已关闭的 DB，用于构造“查询/写入失败”分支
func svcClosedDB(t *testing.T) *gorm.DB {
	t.Helper()
	db := newSvcTestDB(t)
	sqlDB, err := db.DB()
	require.NoError(t, err)
	require.NoError(t, sqlDB.Close())
	return db
}

// newSilentLogger 输出丢弃的 logrus logger
func newSilentLogger() *logrus.Logger {
	lg := logrus.New()
	lg.SetOutput(io.Discard)
	return lg
}

// newSvcEncryptionService 构造加密服务（可指定 ENCRYPTION_KEY）
func newSvcEncryptionService(t *testing.T, envKey string) *EncryptionService {
	t.Helper()
	t.Setenv("ENCRYPTION_KEY", envKey)
	t.Setenv("GIN_MODE", "")
	svc, err := NewEncryptionService(newSilentLogger())
	require.NoError(t, err)
	return svc
}

// withAppConfig 临时替换全局 config.AppConfig
func withAppConfig(t *testing.T, cfg *config.Config) {
	t.Helper()
	saved := config.AppConfig
	config.AppConfig = cfg
	t.Cleanup(func() { config.AppConfig = saved })
}

// withHTTPDefaultTransport 临时替换 http.DefaultTransport。
// SWR/CCR 管理面在函数内新建 http.Client（Transport 为 nil → 请求时取
// http.DefaultTransport），替换后可拦截对华为云/腾讯云域名的请求。
func withHTTPDefaultTransport(t *testing.T, rt http.RoundTripper) {
	t.Helper()
	saved := http.DefaultTransport
	http.DefaultTransport = rt
	t.Cleanup(func() { http.DefaultTransport = saved })
}

// stubRoundTripper 将任意 http.Handler 当作 RoundTripper 使用
type stubRoundTripper struct {
	handler http.Handler
}

func (s stubRoundTripper) RoundTrip(req *http.Request) (*http.Response, error) {
	rec := httptest.NewRecorder()
	s.handler.ServeHTTP(rec, req)
	return rec.Result(), nil
}

// errRoundTripper 对所有请求返回错误
type errRoundTripper struct{ err error }

func (e errRoundTripper) RoundTrip(*http.Request) (*http.Response, error) {
	return nil, e.err
}

// roundTripperFunc 函数式 RoundTripper（resty 客户端 SetTransport 用，
// 可在不依赖 DNS 的情况下拦截任意 URL）
type roundTripperFunc func(*http.Request) (*http.Response, error)

func (f roundTripperFunc) RoundTrip(r *http.Request) (*http.Response, error) { return f(r) }

// cannedJSONResponse 构造固定 JSON 响应
func cannedJSONResponse(req *http.Request, status int, body string) *http.Response {
	return &http.Response{
		StatusCode: status,
		Header:     http.Header{"Content-Type": []string{"application/json"}},
		Body:       io.NopCloser(strings.NewReader(body)),
		Request:    req,
	}
}

// newSvcTLSServer 启动 TLS httptest 服务（registry 数据面 URL 均硬编码 https://）
func newSvcTLSServer(t *testing.T, h http.Handler) *httptest.Server {
	t.Helper()
	ts := httptest.NewTLSServer(h)
	t.Cleanup(ts.Close)
	return ts
}

// svcTLSRestyClient 构造跳过证书校验的 resty 客户端（配合 httptest TLS 服务）
func svcTLSRestyClient() *resty.Client {
	return resty.New().SetTLSClientConfig(&tls.Config{InsecureSkipVerify: true})
}

// registryHostOf 从 httptest 服务 URL 提取 host:port 作为 registry 地址
func registryHostOf(ts *httptest.Server) string {
	return strings.TrimPrefix(ts.URL, "https://")
}

// fakeRegistryAPIClient RegistryAPIClient 的可编程假实现，
// 用于台账/配置服务的远程存在性校验与列表导入逻辑测试
type fakeRegistryAPIClient struct {
	tags        []string
	tagsErr     error
	exists      bool
	existsErr   error
	notFoundErr bool // IsRepositoryNotFound 的返回值
	details     []*TagDetail
	detailByTag map[string]*TagDetail
	detailsErr  error
	list        []string
	listErr     error
	testResult  *RegistryTestResult
}

func (f *fakeRegistryAPIClient) GetTags(registry, username, password, namespace, repo, authServer, dockerService string) ([]string, error) {
	return f.tags, f.tagsErr
}

func (f *fakeRegistryAPIClient) GetTagNames(registry, username, password, namespace, repo, authServer, dockerService string) ([]string, error) {
	return f.tags, f.tagsErr
}

func (f *fakeRegistryAPIClient) GetTagDetail(registry, username, password, namespace, repo, tag, authServer, dockerService string) (*TagDetail, error) {
	if f.detailByTag != nil {
		if d, ok := f.detailByTag[tag]; ok {
			return d, nil
		}
	}
	if len(f.details) > 0 {
		return f.details[0], f.detailsErr
	}
	return nil, f.detailsErr
}

func (f *fakeRegistryAPIClient) GetTagsDetailsBatch(registry, username, password, namespace, repo, authServer, dockerService string, tagNames []string) ([]*TagDetail, error) {
	return f.details, f.detailsErr
}

func (f *fakeRegistryAPIClient) GetTagsWithDetails(registry, username, password, namespace, repo, authServer, dockerService string) ([]*TagDetail, error) {
	return f.details, f.detailsErr
}

func (f *fakeRegistryAPIClient) RepositoryExists(registry, username, password, namespace, repo, authServer, dockerService string) (bool, error) {
	return f.exists, f.existsErr
}

func (f *fakeRegistryAPIClient) IsRepositoryNotFound(err error) bool {
	return f.notFoundErr
}

func (f *fakeRegistryAPIClient) ListRepositories(registry, username, password, accessKey, secretKey, namespace, authServer, dockerService string) ([]string, error) {
	return f.list, f.listErr
}

func (f *fakeRegistryAPIClient) TestConnection(registry, username, password, accessKey, secretKey, namespace, authServer, dockerService string) *RegistryTestResult {
	if f.testResult == nil {
		return &RegistryTestResult{RegistryType: "fake"}
	}
	return f.testResult
}

// withRegistryClientInstance 临时替换 NewRegistryAPIService 的包级单例
func withRegistryClientInstance(t *testing.T, registryType string, fake RegistryAPIClient) {
	t.Helper()
	savedAcr, savedSwr, savedCcr, savedGeneric := acrClientInstance, swrClientInstance, ccrClientInstance, genericClientInstance
	t.Cleanup(func() {
		acrClientInstance, swrClientInstance, ccrClientInstance, genericClientInstance = savedAcr, savedSwr, savedCcr, savedGeneric
	})
	switch registryType {
	case models.RegistryTypeSWR:
		swrClientInstance = fake
	case models.RegistryTypeCCR:
		ccrClientInstance = fake
	case models.RegistryTypeHarbor, models.RegistryTypeGeneric:
		genericClientInstance = fake
	default:
		acrClientInstance = fake
	}
}
