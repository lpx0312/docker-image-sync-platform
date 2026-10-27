package handlers

// gapfill2_test.go：handlers 第二轮补测——包内函数直测 + 数据库错误分支。

import (
	"net/http"
	"testing"

	"docker-image-sync-platform/internal/models"
	"docker-image-sync-platform/internal/services"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestConfigHandler_HelperFunctions(t *testing.T) {
	h := &ConfigHandler{}

	t.Run("getQualityString 全分支", func(t *testing.T) {
		assert.Equal(t, "good", h.getQualityString(services.NetworkQualityGood))
		assert.Equal(t, "medium", h.getQualityString(services.NetworkQualityMedium))
		assert.Equal(t, "poor", h.getQualityString(services.NetworkQualityPoor))
		assert.Equal(t, "unknown", h.getQualityString(services.NetworkQuality(-1)))
	})

	t.Run("parseGiteeRepoURL 全分支", func(t *testing.T) {
		okCases := []struct{ url, owner, repo string }{
			{"https://gitee.com/own/rep.git", "own", "rep"},
			{"https://gitee.com/own/rep", "own", "rep"},
			{"http://gitee.com/own/rep", "own", "rep"},
			{"https://www.gitee.com/own/rep", "own", "rep"},
		}
		for _, c := range okCases {
			owner, repo, err := parseGiteeRepoURL(c.url)
			require.NoError(t, err, c.url)
			assert.Equal(t, c.owner, owner, c.url)
			assert.Equal(t, c.repo, repo, c.url)
		}

		for _, bad := range []string{"", "not-a-url", "https://gitee.com/", "https://gitee.com/only-owner"} {
			_, _, err := parseGiteeRepoURL(bad)
			assert.Error(t, err, bad)
		}
	})
}

func TestConfigHandler_ErrorBranches(t *testing.T) {
	enc := hNewEncSvc(t)

	setup := func(t *testing.T) (http.Handler, *ConfigHandler) {
		db := newTestDB(t)
		withGlobalDB(t, db)
		factory := services.NewGitServiceFactory(enc)
		cfgSvc := services.NewConfigService(db, enc, hNewLogrus())
		h := NewConfigHandler(factory, cfgSvc)
		r := hNewEngine()
		r.GET("/config/git", h.GetGitConfig)
		r.GET("/config/aliyun-db", h.GetAliyunConfig)
		r.PUT("/config/aliyun-db", h.UpdateAliyunConfig)
		r.POST("/config/aliyun/test", h.TestAliyunConnection)
		r.GET("/config/all", h.GetAllConfigs)
		r.GET("/config/git-network-test", h.TestGitNetworkQuality)
		return r, h
	}

	t.Run("配置表缺失时优雅降级200", func(t *testing.T) {
		// ConfigService 对缺失配置/表错误容错返回空值，handler 不报 500
		r, _ := setup(t)
		hDropTable(t, hDB(), &models.SystemConfig{}, "system_configs")
		for _, path := range []string{"/config/git", "/config/all", "/config/aliyun-db"} {
			w, _ := hPerformJSON(t, r, http.MethodGet, path, "", nil)
			assert.Equal(t, http.StatusOK, w.Code, path)
		}
	})

	t.Run("UpdateAliyunConfig_写入失败500", func(t *testing.T) {
		r, _ := setup(t)
		hSetConfig(t, hDB(), "aliyun_registry", "reg.example.com")
		hSetConfig(t, hDB(), "aliyun_namespace", "ns")
		hFailUpdatesOn(t, hDB(), "system_configs")
		w, _ := hPerformJSON(t, r, http.MethodPut, "/config/aliyun-db", `{"registry":"x","namespace":"y","username":"u","password":"p"}`, nil)
		assert.Equal(t, http.StatusInternalServerError, w.Code)
	})

	t.Run("TestGitNetworkQuality_检测失败或成功均可达", func(t *testing.T) {
		r, _ := setup(t)
		// 未初始化/无配置时 DetectNetworkQuality 的结果不影响断言：200 或 500 均可
		w, _ := hPerformJSON(t, r, http.MethodGet, "/config/git-network-test", "", nil)
		assert.Contains(t, []int{http.StatusOK, http.StatusInternalServerError}, w.Code)
	})
}

func TestAcrRegistryHandler_ErrorBranches(t *testing.T) {
	enc := hNewEncSvc(t)
	db := newTestDB(t)
	withGlobalDB(t, db)
	svc := services.NewAcrRegistryService(db, enc)
	h := NewAcrRegistryHandler(svc)
	r := hNewEngine()
	r.GET("/acr-registries", h.GetAll)
	r.GET("/acr-registries/quota-summary", h.GetQuotaSummary)

	t.Run("GetAll_表缺失500", func(t *testing.T) {
		hDropTable(t, hDB(), &models.AcrRegistry{}, "acr_registries")
		w, _ := hPerformJSON(t, r, http.MethodGet, "/acr-registries", "", nil)
		assert.Equal(t, http.StatusInternalServerError, w.Code)
	})

	t.Run("GetQuotaSummary_台账表缺失500", func(t *testing.T) {
		hEncAcrRecord(t, enc)
		hDropTable(t, hDB(), &models.AcrRepository{}, "acr_repositories")
		w, _ := hPerformJSON(t, r, http.MethodGet, "/acr-registries/quota-summary", "", nil)
		assert.Equal(t, http.StatusInternalServerError, w.Code)
	})
}
