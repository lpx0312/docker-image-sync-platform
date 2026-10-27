package handlers

// acr_registry_test.go：ACR 配置管理 Handler 测试（单元 + 边界 + 路由级）。

import (
	"net/http"
	"strconv"
	"testing"

	"docker-image-sync-platform/internal/models"
	"docker-image-sync-platform/internal/services"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// hNewEncSvc 固定密钥的加密服务
func hNewEncSvc(t *testing.T) *services.EncryptionService {
	t.Helper()
	t.Setenv("ENCRYPTION_KEY", "handlers-test-key")
	t.Setenv("GIN_MODE", "debug")
	svc, err := services.NewEncryptionService(hNewLogrus())
	require.NoError(t, err)
	return svc
}

func TestAcrRegistryHandler_CRUD(t *testing.T) {
	db := newTestDB(t)
	withGlobalDB(t, db)
	enc := hNewEncSvc(t)
	svc := services.NewAcrRegistryService(db, enc)
	h := NewAcrRegistryHandler(svc)

	r := hNewEngine()
	g := r.Group("/acr-registries")
	{
		g.GET("", h.GetAll)
		g.GET("/default", h.GetDefault)
		g.GET("/quota-summary", h.GetQuotaSummary)
		g.GET("/:id", h.GetByID)
		g.POST("", h.Create)
		g.PUT("/:id", h.Update)
		g.DELETE("/:id", h.Delete)
		g.PUT("/:id/default", h.SetDefault)
		g.POST("/:id/test", h.TestConnection)
	}

	t.Run("GetAll_空列表", func(t *testing.T) {
		w, body := hPerformJSON(t, r, http.MethodGet, "/acr-registries", "", nil)
		require.Equal(t, http.StatusOK, w.Code)
		assert.Equal(t, "success", body["status"])
	})

	t.Run("GetDefault_无默认404", func(t *testing.T) {
		w, _ := hPerformJSON(t, r, http.MethodGet, "/acr-registries/default", "", nil)
		assert.Equal(t, http.StatusNotFound, w.Code)
	})

	t.Run("GetByID_非法ID", func(t *testing.T) {
		w, _ := hPerformJSON(t, r, http.MethodGet, "/acr-registries/abc", "", nil)
		assert.Equal(t, http.StatusBadRequest, w.Code)
	})

	t.Run("GetByID_不存在404", func(t *testing.T) {
		w, _ := hPerformJSON(t, r, http.MethodGet, "/acr-registries/999", "", nil)
		assert.Equal(t, http.StatusNotFound, w.Code)
	})

	t.Run("Create_缺必填字段400", func(t *testing.T) {
		w, _ := hPerformJSON(t, r, http.MethodPost, "/acr-registries", `{"namespace":"only-ns"}`, nil)
		assert.Equal(t, http.StatusBadRequest, w.Code)
	})

	t.Run("Create_非法类型500", func(t *testing.T) {
		w, _ := hPerformJSON(t, r, http.MethodPost, "/acr-registries",
			`{"registry_url":"registry.example.com","namespace":"ns-a","username":"u","password":"p","registry_type":"not-a-type"}`, nil)
		assert.Equal(t, http.StatusInternalServerError, w.Code)
	})

	t.Run("Create_首个自动默认_201", func(t *testing.T) {
		w, body := hPerformJSON(t, r, http.MethodPost, "/acr-registries",
			`{"registry_url":"registry.example.com","namespace":"ns-a","alias":"ali-a","username":"u","password":"p","registry_type":"acr"}`, nil)
		require.Equal(t, http.StatusCreated, w.Code, w.Body.String())
		data := body["data"].(map[string]interface{})
		assert.Equal(t, true, data["is_default"], "首个配置应自动设为默认")
		assert.NotEmpty(t, data["id"])
	})

	t.Run("Create_别名冲突500", func(t *testing.T) {
		w, _ := hPerformJSON(t, r, http.MethodPost, "/acr-registries",
			`{"registry_url":"registry2.example.com","namespace":"ns-b","alias":"ali-a","username":"u","password":"p","registry_type":"harbor"}`, nil)
		assert.Equal(t, http.StatusInternalServerError, w.Code)
	})

	t.Run("GetByID_命中200", func(t *testing.T) {
		w, _ := hPerformJSON(t, r, http.MethodGet, "/acr-registries/1", "", nil)
		assert.Equal(t, http.StatusOK, w.Code)
	})

	t.Run("Update_非法ID400", func(t *testing.T) {
		w, _ := hPerformJSON(t, r, http.MethodPut, "/acr-registries/xx", `{"namespace":"n"}`, nil)
		assert.Equal(t, http.StatusBadRequest, w.Code)
	})

	t.Run("Update_不存在500", func(t *testing.T) {
		w, _ := hPerformJSON(t, r, http.MethodPut, "/acr-registries/999",
			`{"registry_url":"r","namespace":"n","username":"u","password":"p"}`, nil)
		assert.Equal(t, http.StatusInternalServerError, w.Code)
	})

	t.Run("Update_成功200", func(t *testing.T) {
		w, body := hPerformJSON(t, r, http.MethodPut, "/acr-registries/1",
			`{"registry_url":"registry.example.com","namespace":"ns-a2","alias":"ali-a","username":"u2","password":"p2","registry_type":"acr"}`, nil)
		require.Equal(t, http.StatusOK, w.Code, w.Body.String())
		data := body["data"].(map[string]interface{})
		assert.Equal(t, "ns-a2", data["namespace"])
	})

	t.Run("SetDefault_非法ID400", func(t *testing.T) {
		w, _ := hPerformJSON(t, r, http.MethodPut, "/acr-registries/bad/default", "", nil)
		assert.Equal(t, http.StatusBadRequest, w.Code)
	})

	t.Run("SetDefault_不存在500", func(t *testing.T) {
		w, _ := hPerformJSON(t, r, http.MethodPut, "/acr-registries/999/default", "", nil)
		assert.Equal(t, http.StatusInternalServerError, w.Code)
	})

	t.Run("SetDefault_成功并保持唯一默认", func(t *testing.T) {
		w, body := hPerformJSON(t, r, http.MethodPost, "/acr-registries",
			`{"registry_url":"registry3.example.com","namespace":"ns-c","alias":"ali-c","username":"u","password":"p","registry_type":"acr"}`, nil)
		require.Equal(t, http.StatusCreated, w.Code, w.Body.String())
		newID := int(body["data"].(map[string]interface{})["id"].(float64))

		w, _ = hPerformJSON(t, r, http.MethodPut, "/acr-registries/"+strconv.Itoa(newID)+"/default", "", nil)
		require.Equal(t, http.StatusOK, w.Code)

		var defCount int64
		db.Model(&models.AcrRegistry{}).Where("is_default = ?", true).Count(&defCount)
		assert.EqualValues(t, 1, defCount, "默认配置应唯一")
	})

	t.Run("TestConnection_非法ID400", func(t *testing.T) {
		w, _ := hPerformJSON(t, r, http.MethodPost, "/acr-registries/bad/test", "", nil)
		assert.Equal(t, http.StatusBadRequest, w.Code)
	})

	t.Run("TestConnection_返回结果200", func(t *testing.T) {
		// 仓库地址不可达 → 返回 login_ok=false 的结果而非 500
		w, body := hPerformJSON(t, r, http.MethodPost, "/acr-registries/1/test", "", nil)
		require.Equal(t, http.StatusOK, w.Code, w.Body.String())
		assert.Equal(t, "success", body["status"])
	})

	t.Run("GetQuotaSummary_200", func(t *testing.T) {
		w, _ := hPerformJSON(t, r, http.MethodGet, "/acr-registries/quota-summary", "", nil)
		assert.Equal(t, http.StatusOK, w.Code)
	})

	t.Run("Delete_非法ID400", func(t *testing.T) {
		w, _ := hPerformJSON(t, r, http.MethodDelete, "/acr-registries/nope", "", nil)
		assert.Equal(t, http.StatusBadRequest, w.Code)
	})

	t.Run("Delete_成功200", func(t *testing.T) {
		w, _ := hPerformJSON(t, r, http.MethodDelete, "/acr-registries/1", "", nil)
		assert.Equal(t, http.StatusOK, w.Code)
		w, _ = hPerformJSON(t, r, http.MethodGet, "/acr-registries/1", "", nil)
		assert.Equal(t, http.StatusNotFound, w.Code)
	})
}
