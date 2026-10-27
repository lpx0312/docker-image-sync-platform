package handlers

// acr_repository_test.go：镜像仓库台账 Handler 测试。

import (
	"net/http"
	"strconv"
	"testing"

	"docker-image-sync-platform/internal/models"
	"docker-image-sync-platform/internal/services"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// unreachableHost 必然拒绝连接的地址：覆盖客户端网络错误分支
const unreachableHost = "127.0.0.1:1"

// hEncAcrRecord 直插一条带加密密码的 ACR 配置（mutate 可覆盖字段）
func hEncAcrRecord(t *testing.T, enc *services.EncryptionService, mutate ...func(*models.AcrRegistry)) *models.AcrRegistry {
	t.Helper()
	encPwd, err := enc.Encrypt("registry-pass")
	require.NoError(t, err)
	acr := &models.AcrRegistry{
		RegistryURL:  unreachableHost,
		Namespace:    "ns-x",
		Alias:        "alias-x",
		Username:     "user",
		Password:     encPwd,
		RegistryType: models.RegistryTypeACR,
	}
	for _, m := range mutate {
		if m != nil {
			m(acr)
		}
	}
	require.NoError(t, hDB().Create(acr).Error)
	return acr
}

func TestAcrRepositoryHandler_All(t *testing.T) {
	db := newTestDB(t)
	withGlobalDB(t, db)
	enc := hNewEncSvc(t)
	svc := services.NewAcrRepositoryService(db, enc)
	h := NewAcrRepositoryHandler(svc)

	r := hNewEngine()
	g := r.Group("/acr-repositories")
	{
		g.GET("", h.GetAll)
		g.GET("/duplicates", h.GetDuplicates)
		g.POST("", h.Create)
		g.POST("/batch", h.BatchCreate)
		g.POST("/batch-delete", h.BatchDelete)
		g.POST("/clean-invalid", h.CleanInvalid)
		g.DELETE("/:id", h.Delete)
		g.POST("/sync-from-records", h.SyncFromRecords)
		g.POST("/import-from-registry", h.ImportFromRegistry)
	}

	acr := hEncAcrRecord(t, enc)
	acrID := strconv.FormatUint(uint64(acr.ID), 10)

	t.Run("GetAll_缺参400", func(t *testing.T) {
		w, _ := hPerformJSON(t, r, http.MethodGet, "/acr-repositories", "", nil)
		assert.Equal(t, http.StatusBadRequest, w.Code)
	})

	t.Run("GetAll_非法参数400", func(t *testing.T) {
		w, _ := hPerformJSON(t, r, http.MethodGet, "/acr-repositories?acr_registry_id=abc", "", nil)
		assert.Equal(t, http.StatusBadRequest, w.Code)
	})

	t.Run("GetAll_成功200", func(t *testing.T) {
		w, _ := hPerformJSON(t, r, http.MethodGet, "/acr-repositories?acr_registry_id="+acrID, "", nil)
		require.Equal(t, http.StatusOK, w.Code)
	})

	t.Run("Create_坏请求400", func(t *testing.T) {
		w, _ := hPerformJSON(t, r, http.MethodPost, "/acr-repositories", `{"repository_name":"x"}`, nil)
		assert.Equal(t, http.StatusBadRequest, w.Code)
	})

	t.Run("Create_仓库配置不存在500", func(t *testing.T) {
		w, _ := hPerformJSON(t, r, http.MethodPost, "/acr-repositories", `{"acr_registry_id":999,"repository_name":"nginx"}`, nil)
		assert.Equal(t, http.StatusInternalServerError, w.Code)
	})

	t.Run("Create_本地已存在_200", func(t *testing.T) {
		require.NoError(t, db.Create(&models.AcrRepository{AcrRegistryID: acr.ID, RepositoryName: "dup"}).Error)
		w, body := hPerformJSON(t, r, http.MethodPost, "/acr-repositories",
			`{"acr_registry_id":`+acrID+`,"repository_name":"dup"}`, nil)
		require.Equal(t, http.StatusOK, w.Code)
		assert.Equal(t, "镜像本地台账已存在，未重复添加", body["message"])
	})

	t.Run("Create_远程校验失败_200_check_failed", func(t *testing.T) {
		// RegistryURL 指向不可达地址 → RepositoryExists 报错（非 NotFound）→ check_failed
		w, body := hPerformJSON(t, r, http.MethodPost, "/acr-repositories",
			`{"acr_registry_id":`+acrID+`,"repository_name":"remote-check-fail"}`, nil)
		require.Equal(t, http.StatusOK, w.Code)
		assert.Equal(t, "检查目标仓库失败，未添加", body["message"])
	})

	t.Run("BatchCreate_坏请求400", func(t *testing.T) {
		w, _ := hPerformJSON(t, r, http.MethodPost, "/acr-repositories/batch", `{}`, nil)
		assert.Equal(t, http.StatusBadRequest, w.Code)
	})

	t.Run("BatchCreate_仓库不存在500", func(t *testing.T) {
		w, _ := hPerformJSON(t, r, http.MethodPost, "/acr-repositories/batch",
			`{"acr_registry_id":999,"repository_names":["a"]}`, nil)
		assert.Equal(t, http.StatusInternalServerError, w.Code)
	})

	t.Run("BatchCreate_输入重复与本地已存在与检查失败", func(t *testing.T) {
		w, body := hPerformJSON(t, r, http.MethodPost, "/acr-repositories/batch",
			`{"acr_registry_id":`+acrID+`,"repository_names":["dup","dup","new-one"]}`, nil)
		require.Equal(t, http.StatusOK, w.Code)
		msg, _ := body["message"].(string)
		assert.Contains(t, msg, "输入重复")
		assert.Contains(t, msg, "本地已存在")
		assert.Contains(t, msg, "检查失败")
	})

	t.Run("BatchDelete_坏请求400", func(t *testing.T) {
		w, _ := hPerformJSON(t, r, http.MethodPost, "/acr-repositories/batch-delete", `{}`, nil)
		assert.Equal(t, http.StatusBadRequest, w.Code)
	})

	t.Run("BatchDelete_成功", func(t *testing.T) {
		rec := models.AcrRepository{AcrRegistryID: acr.ID, RepositoryName: "del-me"}
		require.NoError(t, db.Create(&rec).Error)
		w, body := hPerformJSON(t, r, http.MethodPost, "/acr-repositories/batch-delete",
			`{"ids":[`+strconv.FormatUint(uint64(rec.ID), 10)+`]}`, nil)
		require.Equal(t, http.StatusOK, w.Code)
		assert.Contains(t, body["message"], "成功删除 1 个镜像")
	})

	t.Run("CleanInvalid_坏请求400", func(t *testing.T) {
		w, _ := hPerformJSON(t, r, http.MethodPost, "/acr-repositories/clean-invalid", `{}`, nil)
		assert.Equal(t, http.StatusBadRequest, w.Code)
	})

	t.Run("CleanInvalid_执行成功", func(t *testing.T) {
		require.NoError(t, db.Create(&models.AcrRepository{AcrRegistryID: acr.ID, RepositoryName: "stale"}).Error)
		w, body := hPerformJSON(t, r, http.MethodPost, "/acr-repositories/clean-invalid",
			`{"acr_registry_id":`+acrID+`}`, nil)
		require.Equal(t, http.StatusOK, w.Code)
		assert.Contains(t, body["message"], "清理")
	})

	t.Run("Delete_非法ID400", func(t *testing.T) {
		w, _ := hPerformJSON(t, r, http.MethodDelete, "/acr-repositories/bad", "", nil)
		assert.Equal(t, http.StatusBadRequest, w.Code)
	})

	t.Run("Delete_成功", func(t *testing.T) {
		rec := models.AcrRepository{AcrRegistryID: acr.ID, RepositoryName: "single-del"}
		require.NoError(t, db.Create(&rec).Error)
		w, _ := hPerformJSON(t, r, http.MethodDelete, "/acr-repositories/"+strconv.FormatUint(uint64(rec.ID), 10), "", nil)
		assert.Equal(t, http.StatusOK, w.Code)
	})

	t.Run("SyncFromRecords_坏请求400", func(t *testing.T) {
		w, _ := hPerformJSON(t, r, http.MethodPost, "/acr-repositories/sync-from-records", `{}`, nil)
		assert.Equal(t, http.StatusBadRequest, w.Code)
	})

	t.Run("SyncFromRecords_执行成功", func(t *testing.T) {
		w, body := hPerformJSON(t, r, http.MethodPost, "/acr-repositories/sync-from-records",
			`{"acr_registry_id":`+acrID+`}`, nil)
		require.Equal(t, http.StatusOK, w.Code)
		assert.Contains(t, body["message"], "成功导入")
	})

	t.Run("ImportFromRegistry_坏请求400", func(t *testing.T) {
		w, _ := hPerformJSON(t, r, http.MethodPost, "/acr-repositories/import-from-registry", `{}`, nil)
		assert.Equal(t, http.StatusBadRequest, w.Code)
	})

	t.Run("ImportFromRegistry_远程不可达500", func(t *testing.T) {
		// 远程 _catalog 拉取失败 → 服务返回错误 → 500（成功路径需真实仓库，见 services 包测试）
		w, _ := hPerformJSON(t, r, http.MethodPost, "/acr-repositories/import-from-registry",
			`{"acr_registry_id":`+acrID+`}`, nil)
		assert.Equal(t, http.StatusInternalServerError, w.Code)
	})

	t.Run("GetDuplicates_200", func(t *testing.T) {
		w, _ := hPerformJSON(t, r, http.MethodGet, "/acr-repositories/duplicates", "", nil)
		assert.Equal(t, http.StatusOK, w.Code)
	})
}
