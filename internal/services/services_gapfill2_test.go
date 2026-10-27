package services

// services_gapfill2_test.go：本地 TLS fixture（/auth + tags + manifests）
// 覆盖 CleanInvalid/BatchCreate/SyncFromRecords/GetTagDetail 成功路径
// 以及 DetectNetworkQuality / setConfigWithTx 等分支。

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"docker-image-sync-platform/internal/database"
	"docker-image-sync-platform/internal/models"

	"crypto/tls"
	"github.com/go-resty/resty/v2"
	"github.com/sirupsen/logrus"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"time"
)

// svcCatalogEnv 注入 ACR 单例客户端 + 本地 TLS registry：
// repo "keep" 有 Tag（存在）、"gone" 404（不存在）、"boom" 500（检查失败）、manifests 返回 OCI index
func svcCatalogEnv(t *testing.T) (*AcrRepositoryService, *models.AcrRegistry, *EncryptionService) {
	t.Helper()
	const indexManifest = `{"schemaVersion":2,"mediaType":"application/vnd.oci.image.index.v1+json","manifests":[
		{"mediaType":"application/vnd.oci.image.manifest.v1+json","digest":"sha256:1111111111111111111111111111111111111111111111111111111111111111","size":714,"platform":{"architecture":"amd64","os":"linux"}},
		{"mediaType":"application/vnd.oci.image.manifest.v1+json","digest":"sha256:2222222222222222222222222222222222222222222222222222222222222222","size":714,"platform":{"architecture":"arm64","os":"linux"}}]}`

	ts := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		switch {
		case r.URL.Path == "/auth":
			_, _ = w.Write([]byte(`{"token":"tk","expires_in":3600}`))
		case strings.Contains(r.URL.Path, "/tags/list"):
			switch {
			case strings.Contains(r.URL.Path, "/keep"), strings.Contains(r.URL.Path, "/newrepo"):
				_, _ = w.Write([]byte(`{"name":"x","tags":["1.0","2.0"]}`))
			case strings.Contains(r.URL.Path, "/gone"):
				w.WriteHeader(http.StatusNotFound)
				_, _ = w.Write([]byte(`{"errors":[{"code":"NAME_UNKNOWN","message":"not found"}]}`))
			default:
				w.WriteHeader(http.StatusInternalServerError)
			}
		case strings.Contains(r.URL.Path, "/manifests/"):
			w.Header().Set("Docker-Content-Digest", "sha256:3333333333333333333333333333333333333333333333333333333333333333")
			_, _ = w.Write([]byte(indexManifest))
		default:
			w.WriteHeader(http.StatusNotFound)
		}
	}))
	t.Cleanup(ts.Close)
	host := strings.TrimPrefix(ts.URL, "https://")

	savedClient := acrClientInstance
	injected := NewAcrAPIService()
	injected.client = resty.New().SetTLSClientConfig(&tls.Config{InsecureSkipVerify: true}).SetTimeout(5 * time.Second)
	acrClientInstance = injected
	t.Cleanup(func() { acrClientInstance = savedClient })

	db := newSvcTestDB(t)
	withGlobalDB(t, db)
	enc := svcEnc(t)
	acr := &models.AcrRegistry{
		RegistryURL: host, Namespace: "ns", Alias: "fixture", AuthServer: host,
		Username: "u", Password: mustEnc(t, enc), RegistryType: models.RegistryTypeACR,
	}
	require.NoError(t, db.Create(acr).Error)
	return NewAcrRepositoryService(db, enc), acr, enc
}

func TestSvcGapfix_CleanInvalid_RemotePaths(t *testing.T) {
	svc, acr, _ := svcCatalogEnv(t)
	db := database.DB
	for _, name := range []string{"keep", "gone", "boom"} {
		require.NoError(t, db.Create(&models.AcrRepository{AcrRegistryID: acr.ID, RepositoryName: name}).Error)
	}

	result, err := svc.CleanInvalid(acr.ID)
	require.NoError(t, err)
	assert.Equal(t, 1, result.Cleaned, "gone(404) 应被清理")
	assert.Equal(t, []string{"gone"}, result.CleanedNames)
	assert.Equal(t, []string{"boom"}, result.CheckFailedNames, "boom(500) 检查失败保留")

	var left []models.AcrRepository
	require.NoError(t, db.Where("acr_registry_id = ?", acr.ID).Find(&left).Error)
	names := []string{}
	for _, r := range left {
		names = append(names, r.RepositoryName)
	}
	assert.ElementsMatch(t, []string{"keep", "boom"}, names)
}

func TestSvcGapfix_BatchCreate_RemotePaths(t *testing.T) {
	svc, acr, _ := svcCatalogEnv(t)

	result, err := svc.BatchCreate(&models.AcrRepositoryBatchRequest{
		AcrRegistryID:   acr.ID,
		RepositoryNames: []string{"newrepo", "gone", "boom"},
	})
	require.NoError(t, err)
	assert.Equal(t, 1, result.Created, "newrepo 远程存在 → 创建")
	assert.Contains(t, result.CreatedNames, "newrepo")
	assert.Equal(t, []string{"gone"}, result.MissingInACR, "404 → 远程不存在")
	assert.Equal(t, []string{"boom"}, result.CheckFailedNames, "500 → 检查失败")

	// 幂等：再次导入 newrepo → already_exist
	result2, err := svc.BatchCreate(&models.AcrRepositoryBatchRequest{
		AcrRegistryID: acr.ID, RepositoryNames: []string{"newrepo"},
	})
	require.NoError(t, err)
	assert.Equal(t, 1, result2.AlreadyExist)
}

func TestSvcGapfix_SyncFromRecords_RemotePaths(t *testing.T) {
	svc, acr, _ := svcCatalogEnv(t)
	db := database.DB
	svcRec(t, db, acr.ID, "reg/ns/keep:v1", models.SyncStatusSuccess)
	svcRec(t, db, acr.ID, "reg/ns/gone:v1", models.SyncStatusSuccess)

	result, err := svc.SyncFromRecords(acr.ID)
	require.NoError(t, err)
	assert.Equal(t, 1, result.Created)
	assert.Contains(t, result.CreatedNames, "keep")
	assert.Equal(t, []string{"gone"}, result.MissingInACR)
}

func TestSvcGapfix_TagDetail_RemotePath(t *testing.T) {
	svc, acr, enc := svcCatalogEnv(t)
	_ = svc

	// GetTagDetail（走 manifest 解析，含 amd64/arm64 架构抽取）
	acrSvc := NewAcrRegistryService(database.DB, enc)
	_ = acrSvc

	client := NewRegistryAPIService(models.RegistryTypeACR, acr.RegistryURL)
	detail, err := client.GetTagDetail(acr.RegistryURL, acr.Username, "pw", acr.Namespace, "keep", "1.0", acr.AuthServer, acr.DockerService)
	require.NoError(t, err)
	assert.Equal(t, "1.0", detail.Tag)
	assert.NotEmpty(t, detail.Architectures)
}

func TestGitOptimizedService_DetectNetworkQuality_Real(t *testing.T) {
	// 本机可访问 gitee.com/github.com：探测应成功并返回三档之一
	s := NewGitOptimizedService(svcEnc(t))
	s.config.NetworkDetection = true
	q, err := s.DetectNetworkQuality()
	require.NoError(t, err)
	assert.Contains(t, []NetworkQuality{NetworkQualityGood, NetworkQualityMedium, NetworkQualityPoor}, q)
}

func TestConfigService_setConfigWithTx_Error(t *testing.T) {
	db := newSvcTestDB(t)
	withGlobalDB(t, db)
	enc := svcEnc(t)
	lg := logrus.New()
	lg.SetLevel(logrus.PanicLevel)
	svc := NewConfigService(db, enc, lg)

	// 触发器阻断 system_configs 写入 → 事务回滚报错
	require.NoError(t, db.Exec("CREATE TRIGGER trig_cfg_ins BEFORE INSERT ON system_configs BEGIN SELECT RAISE(ABORT, 'forced'); END").Error)
	t.Cleanup(func() { _ = db.Exec("DROP TRIGGER IF EXISTS trig_cfg_ins").Error })

	// 公开写路径 SetConfig 走事务，被触发器拦截后报错
	err := svc.SetConfig("new_key", "v", "描述", "default", 0)
	require.Error(t, err)
}

func TestSvcGapfix_DBErrorBranches(t *testing.T) {
	db := newSvcTestDB(t)
	withGlobalDB(t, db)
	enc := svcEnc(t)

	t.Run("AcrRegistry GetAll/Create/SetDefault 表缺失", func(t *testing.T) {
		svc := NewAcrRegistryService(db, enc)
		require.NoError(t, db.Migrator().DropTable("acr_registries"))
		_, err := svc.GetAll()
		assert.Error(t, err)
		_, err = svc.Create(&models.AcrRegistryRequest{RegistryURL: "r", Namespace: "n", Username: "u", Password: "p"})
		assert.Error(t, err)
		err = svc.SetDefault(1)
		assert.Error(t, err)
		require.NoError(t, db.AutoMigrate(&models.AcrRegistry{}))
	})

	t.Run("AcrRepository Create/Delete/BatchDelete 写失败", func(t *testing.T) {
		acr := svcAcr(t, db, enc, "dberr", "ns", true, "")
		svc := NewAcrRepositoryService(db, enc)
		// Create: 本地不存在但加密密码损坏 → 解密失败
		bad := &models.AcrRegistry{RegistryURL: "127.0.0.1:1", Namespace: "n", Alias: "badx", Username: "u", Password: "ENC:!!!", RegistryType: models.RegistryTypeACR}
		require.NoError(t, db.Create(bad).Error)
		_, err := svc.Create(&models.AcrRepositoryRequest{AcrRegistryID: bad.ID, RepositoryName: "x"})
		assert.Error(t, err)

		rec := &models.AcrRepository{AcrRegistryID: acr.ID, RepositoryName: "del"}
		require.NoError(t, db.Create(rec).Error)
		require.NoError(t, db.Exec("CREATE TRIGGER trig_repo_upd BEFORE UPDATE ON acr_repositories BEGIN SELECT RAISE(ABORT, 'forced'); END").Error)
		t.Cleanup(func() { _ = db.Exec("DROP TRIGGER IF EXISTS trig_repo_upd").Error })
		// 软删除是 UPDATE：被拦截 → Delete 报错
		assert.Error(t, svc.Delete(rec.ID))
		// BatchDelete 同样
		_, err = svc.BatchDelete([]uint{rec.ID})
		assert.Error(t, err)
	})

	t.Run("GetTagsDetailsBatch 空列表与远程详情", func(t *testing.T) {
		svc, acr, _ := svcCatalogEnv(t)
		_ = svc
		client := NewRegistryAPIService(models.RegistryTypeACR, acr.RegistryURL)
		empty, err := client.GetTagsDetailsBatch(acr.RegistryURL, acr.Username, "pw", acr.Namespace, "keep", acr.AuthServer, acr.DockerService, nil)
		require.NoError(t, err)
		assert.Empty(t, empty)

		details, err := client.GetTagsDetailsBatch(acr.RegistryURL, acr.Username, "pw", acr.Namespace, "keep", acr.AuthServer, acr.DockerService, []string{"1.0"})
		require.NoError(t, err)
		require.Len(t, details, 1)
		assert.Equal(t, "1.0", details[0].Tag)
		assert.NotEmpty(t, details[0].Architectures)
	})
}
