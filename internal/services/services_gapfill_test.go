package services

// services_gapfill_test.go：services 包最后一轮补测——
// BatchCreate 矩阵 / CreateGitFileService / TestSimpleConnection / ImportFromRegistry（注入单例客户端）

import (
	"crypto/tls"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"docker-image-sync-platform/internal/models"

	"github.com/go-resty/resty/v2"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestAcrRepositoryService_BatchCreate(t *testing.T) {
	db := newSvcTestDB(t)
	enc := svcEnc(t)
	svc := NewAcrRepositoryService(db, enc)
	acr := svcAcr(t, db, enc, "batch", "ns", true, "")

	t.Run("配置不存在", func(t *testing.T) {
		_, err := svc.BatchCreate(&models.AcrRepositoryBatchRequest{AcrRegistryID: 999, RepositoryNames: []string{"a"}})
		require.Error(t, err)
		assert.Contains(t, err.Error(), "配置不存在")
	})

	t.Run("解密失败", func(t *testing.T) {
		bad := &models.AcrRegistry{RegistryURL: "127.0.0.1:1", Namespace: "n", Alias: "badenc9", Username: "u", Password: "ENC:^^^", RegistryType: models.RegistryTypeACR}
		require.NoError(t, db.Create(bad).Error)
		_, err := svc.BatchCreate(&models.AcrRepositoryBatchRequest{AcrRegistryID: bad.ID, RepositoryNames: []string{"a"}})
		require.Error(t, err)
		assert.Contains(t, err.Error(), "解密")
	})

	t.Run("输入去重_本地已存在_远程检查失败", func(t *testing.T) {
		require.NoError(t, db.Create(&models.AcrRepository{AcrRegistryID: acr.ID, RepositoryName: "exists"}).Error)
		result, err := svc.BatchCreate(&models.AcrRepositoryBatchRequest{
			AcrRegistryID:   acr.ID,
			RepositoryNames: []string{" exists ", "newone", "newone", "  "},
		})
		require.NoError(t, err)
		assert.Equal(t, 1, result.AlreadyExist)
		assert.Contains(t, result.AlreadyExistNames, "exists")
		assert.Equal(t, []string{"newone"}, result.DuplicateInInput, "输入重复去重并排序")
		// 远程不可达 → check_failed
		assert.Equal(t, []string{"newone"}, result.CheckFailedNames)
		assert.Equal(t, 0, result.Created)
	})
}

func TestAcrRepositoryService_ImportFromRegistry_WithCatalog(t *testing.T) {
	// 注入 ACR 单例客户端指向 TLS httptest：/auth 发 token，/v2/_catalog 列出 ns/ 前缀仓库
	host := ""
	ts := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		if r.URL.Path == "/auth" {
			_, _ = w.Write([]byte(`{"token":"fake-token","expires_in":3600}`))
			return
		}
		// ACR 客户端按 namespace 前缀过滤 catalog 条目
		_, _ = w.Write([]byte(`{"repositories":["ns/app1","ns/app2","other/x"]}`))
	}))
	t.Cleanup(ts.Close)
	host = strings.TrimPrefix(ts.URL, "https://")

	savedClient := acrClientInstance
	injected := NewAcrAPIService()
	injected.client = resty.New().SetTLSClientConfig(&tls.Config{InsecureSkipVerify: true}).SetTimeout(5 * time.Second)
	acrClientInstance = injected
	t.Cleanup(func() { acrClientInstance = savedClient })

	db := newSvcTestDB(t)
	enc := svcEnc(t)
	svc := NewAcrRepositoryService(db, enc)
	acr := &models.AcrRegistry{
		RegistryURL: host, Namespace: "ns", Alias: "catalog", AuthServer: host,
		Username: "u", Password: mustEnc(t, enc), RegistryType: models.RegistryTypeACR,
	}
	require.NoError(t, db.Create(acr).Error)
	// 预置一条已存在
	require.NoError(t, db.Create(&models.AcrRepository{AcrRegistryID: acr.ID, RepositoryName: "app1"}).Error)

	result, err := svc.ImportFromRegistry(acr.ID)
	require.NoError(t, err)
	assert.Equal(t, 1, result.Created)
	assert.Contains(t, result.CreatedNames, "app2")
	assert.Equal(t, 1, result.AlreadyExist)
	assert.Contains(t, result.AlreadyExistNames, "app1")
}

func mustEnc(t *testing.T, enc *EncryptionService) string {
	t.Helper()
	v, err := enc.Encrypt("pw")
	require.NoError(t, err)
	return v
}

func TestCreateGitFileService_All(t *testing.T) {
	db := newSvcTestDB(t)
	withGlobalDB(t, db)
	enc := svcEnc(t)

	t.Run("github完整配置", func(t *testing.T) {
		gitSvcGitHubCfg(t, db, "https://github.com/own/rep.git", "unused")
		svc, err := CreateGitFileService("github", enc)
		require.NoError(t, err)
		assert.NotNil(t, svc)
	})

	t.Run("gitee完整配置", func(t *testing.T) {
		gitSvcCfg(t, db, map[string]string{"git_repository_type": "gitee"})
		giteeFull := map[string]string{
			"gitee_repo_url":   "https://gitee.com/own/rep.git",
			"gitee_username":   "u",
			"gitee_token":      "tk",
			"gitee_email":      "e@t.io",
			"gitee_local_path": "unused",
		}
		gitSvcCfg(t, db, giteeFull)
		svc, err := CreateGitFileService("gitee", enc)
		require.NoError(t, err)
		assert.NotNil(t, svc)
	})

	t.Run("不支持类型", func(t *testing.T) {
		_, err := CreateGitFileService("svn", enc)
		require.Error(t, err)
	})

	t.Run("配置缺失", func(t *testing.T) {
		db2 := newSvcTestDB(t)
		withGlobalDB(t, db2)
		_, err := CreateGitFileService("github", enc)
		require.Error(t, err)
		assert.Contains(t, err.Error(), "获取GitHub配置失败")
	})

	t.Run("URL解析失败", func(t *testing.T) {
		gitSvcCfg(t, db, map[string]string{
			"github_repo_url":   "https://not-github.com/a/b",
			"github_username":   "u",
			"github_token":      "tk",
			"github_branch":     "main",
			"github_local_path": "x",
		})
		_, err := CreateGitFileService("github", enc)
		require.Error(t, err)
	})
}

func TestNewGiteeAPIService_Defaults(t *testing.T) {
	svc := NewGiteeAPIService("tk", "own", "rep", "")
	assert.NotNil(t, svc)
	assert.Equal(t, "main", svc.branch, "空分支默认 main")
	assert.Equal(t, "tk", svc.token)
}

func TestGitHubAPIService_TestSimpleConnection(t *testing.T) {
	// 指向 httptest（apiURL 可注入），成功与失败分支
	ok, _ := ghAPISvc(t, func(w http.ResponseWriter, r *http.Request) {
		ghJSONOk(w, `{"login":"user"}`)
	})
	require.NoError(t, ok.TestSimpleConnection())

	fail, _ := ghAPISvc(t, func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusUnauthorized)
	})
	err := fail.TestSimpleConnection()
	require.Error(t, err)
}

func ghJSONOk(w http.ResponseWriter, body string) {
	w.Header().Set("Content-Type", "application/json")
	_, _ = w.Write([]byte(body))
}

func TestGetGiteeConfig_Encrypted(t *testing.T) {
	db := newSvcTestDB(t)
	withGlobalDB(t, db)
	enc := svcEnc(t)

	encToken, err := enc.Encrypt("secret")
	require.NoError(t, err)
	gitSvcCfg(t, db, map[string]string{
		"gitee_repo_url":   "https://gitee.com/u/r.git",
		"gitee_username":   "u",
		"gitee_email":      "e",
		"gitee_local_path": "p",
	})
	// 带加密标记写入 token
	require.NoError(t, db.Unscoped().Where("config_key = ?", "gitee_token").Delete(&models.SystemConfig{}).Error)
	require.NoError(t, db.Create(&models.SystemConfig{ConfigKey: "gitee_token", ConfigValue: encToken, IsEncrypted: true}).Error)

	cfg, err := getGiteeConfig(enc)
	require.NoError(t, err)
	assert.Equal(t, "secret", cfg.Token)

	// 缺失必备项报错
	require.NoError(t, db.Unscoped().Where("config_key = ?", "gitee_repo_url").Delete(&models.SystemConfig{}).Error)
	_, err = getGiteeConfig(enc)
	require.Error(t, err)
}
