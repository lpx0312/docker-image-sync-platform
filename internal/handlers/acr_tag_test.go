package handlers

// acr_tag_test.go：镜像 Tag 查询 Handler 测试。
// registry 客户端硬编码 https 无法指向 httptest：成功路径在 services 包测，
// 这里覆盖参数校验/记录不存在/解密失败/网络不可达分支。

import (
	"net/http"
	"strconv"
	"testing"

	"docker-image-sync-platform/internal/models"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestAcrTagHandler_GetTags(t *testing.T) {
	db := newTestDB(t)
	withGlobalDB(t, db)
	enc := hNewEncSvc(t)
	h := NewAcrTagHandler(enc)
	r := hNewEngine()
	r.GET("/acr-tags", h.GetTags)

	// 正常配置（地址不可达，覆盖网络错误 500）
	ok := hEncAcrRecord(t, enc)
	okID := strconv.FormatUint(uint64(ok.ID), 10)
	// 密文损坏（解密失败 500）
	bad := hEncAcrRecord(t, enc, func(a *models.AcrRegistry) { a.Password = "ENC:!!!not-base64!!!" })
	badID := strconv.FormatUint(uint64(bad.ID), 10)

	t.Run("缺参数400", func(t *testing.T) {
		w, _ := hPerformJSON(t, r, http.MethodGet, "/acr-tags", "", nil)
		assert.Equal(t, http.StatusBadRequest, w.Code)
		w, _ = hPerformJSON(t, r, http.MethodGet, "/acr-tags?acr_registry_id="+okID, "", nil)
		assert.Equal(t, http.StatusBadRequest, w.Code)
	})

	t.Run("非法ID400", func(t *testing.T) {
		w, _ := hPerformJSON(t, r, http.MethodGet, "/acr-tags?acr_registry_id=abc&repository_name=nginx", "", nil)
		assert.Equal(t, http.StatusBadRequest, w.Code)
	})

	t.Run("配置不存在404", func(t *testing.T) {
		w, body := hPerformJSON(t, r, http.MethodGet, "/acr-tags?acr_registry_id=424242&repository_name=nginx", "", nil)
		assert.Equal(t, http.StatusNotFound, w.Code)
		assert.Contains(t, body["message"], "不存在")
	})

	t.Run("解密失败500_提示ENCRYPTION_KEY", func(t *testing.T) {
		w, body := hPerformJSON(t, r, http.MethodGet, "/acr-tags?acr_registry_id="+badID+"&repository_name=nginx", "", nil)
		assert.Equal(t, http.StatusInternalServerError, w.Code)
		assert.Contains(t, body["message"], "ENCRYPTION_KEY")
	})

	t.Run("网络不可达500", func(t *testing.T) {
		w, _ := hPerformJSON(t, r, http.MethodGet, "/acr-tags?acr_registry_id="+okID+"&repository_name=nginx", "", nil)
		assert.Equal(t, http.StatusInternalServerError, w.Code)
	})
}

func TestAcrTagHandler_GetTagsDetails(t *testing.T) {
	db := newTestDB(t)
	withGlobalDB(t, db)
	enc := hNewEncSvc(t)
	h := NewAcrTagHandler(enc)
	r := hNewEngine()
	r.GET("/acr-tags/details", h.GetTagsDetails)

	ok := hEncAcrRecord(t, enc)
	okID := strconv.FormatUint(uint64(ok.ID), 10)

	t.Run("缺参数400", func(t *testing.T) {
		w, _ := hPerformJSON(t, r, http.MethodGet, "/acr-tags/details?acr_registry_id=1", "", nil)
		assert.Equal(t, http.StatusBadRequest, w.Code)
	})

	t.Run("非法ID400", func(t *testing.T) {
		w, _ := hPerformJSON(t, r, http.MethodGet, "/acr-tags/details?acr_registry_id=x&repository_name=a&tags=1", "", nil)
		assert.Equal(t, http.StatusBadRequest, w.Code)
	})

	t.Run("tags参数无效400", func(t *testing.T) {
		// 逗号分隔后全为空
		w, body := hPerformJSON(t, r, http.MethodGet, "/acr-tags/details?acr_registry_id="+okID+"&repository_name=a&tags=,,", "", nil)
		assert.Equal(t, http.StatusBadRequest, w.Code)
		assert.Contains(t, body["message"], "tags")
	})

	t.Run("tags空白修剪后有效", func(t *testing.T) {
		// 带空格的 tags 应被修剪，进入后续流程（配置可达性无关，404/500 均可）
		w, _ := hPerformJSON(t, r, http.MethodGet, "/acr-tags/details?acr_registry_id=424242&repository_name=a&tags=+1.0+,+2.0+", "", nil)
		assert.Equal(t, http.StatusNotFound, w.Code)
	})

	t.Run("网络不可达_返回空详情200", func(t *testing.T) {
		// 当前行为：批量详情对拉取失败的 Tag 返回空详情条目（architectures 为空）而非整体报错
		w, body := hPerformJSON(t, r, http.MethodGet, "/acr-tags/details?acr_registry_id="+okID+"&repository_name=nginx&tags=1.25", "", nil)
		require.Equal(t, http.StatusOK, w.Code)
		data := body["data"].([]interface{})
		require.Len(t, data, 1)
		item := data[0].(map[string]interface{})
		assert.Equal(t, "1.25", item["tag"])
		arch, _ := item["architectures"].([]interface{})
		assert.Empty(t, arch)
	})
}

func TestAcrTagHandler_GetTagDetail(t *testing.T) {
	db := newTestDB(t)
	withGlobalDB(t, db)
	enc := hNewEncSvc(t)
	h := NewAcrTagHandler(enc)
	r := hNewEngine()
	r.GET("/acr-tags/detail", h.GetTagDetail)

	ok := hEncAcrRecord(t, enc)
	okID := strconv.FormatUint(uint64(ok.ID), 10)

	t.Run("缺参数400", func(t *testing.T) {
		w, _ := hPerformJSON(t, r, http.MethodGet, "/acr-tags/detail?acr_registry_id=1&repository_name=a", "", nil)
		assert.Equal(t, http.StatusBadRequest, w.Code)
	})

	t.Run("非法ID400", func(t *testing.T) {
		w, _ := hPerformJSON(t, r, http.MethodGet, "/acr-tags/detail?acr_registry_id=zz&repository_name=a&tag=t", "", nil)
		assert.Equal(t, http.StatusBadRequest, w.Code)
	})

	t.Run("配置不存在404", func(t *testing.T) {
		w, _ := hPerformJSON(t, r, http.MethodGet, "/acr-tags/detail?acr_registry_id=424242&repository_name=a&tag=t", "", nil)
		assert.Equal(t, http.StatusNotFound, w.Code)
	})

	t.Run("网络不可达500", func(t *testing.T) {
		w, _ := hPerformJSON(t, r, http.MethodGet, "/acr-tags/detail?acr_registry_id="+okID+"&repository_name=nginx&tag=1.25", "", nil)
		assert.Equal(t, http.StatusInternalServerError, w.Code)
	})
}
