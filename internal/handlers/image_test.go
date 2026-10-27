package handlers

// image_test.go：镜像记录 Handler 测试。
// Check 系列借助 hLocalRegistry（go-containerregistry 对 127.0.0.1 走明文 http）离线覆盖。

import (
	"fmt"
	"net/http"
	"testing"

	"docker-image-sync-platform/internal/models"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// hImageRouter 构造镜像路由与全局 DB
func hImageRouter(t *testing.T) (r http.Handler, db interface {
	Create(value interface{}) interface{ GetError() error }
}) {
	return nil, nil
}

// hImageRecord 直插镜像记录（status 空则默认 pending）
func hImageRecord(t *testing.T, mutate func(*models.ImageSyncRecord)) *models.ImageSyncRecord {
	t.Helper()
	rec := &models.ImageSyncRecord{
		OriginalImage: "nginx",
		Tag:           "1.25",
		SyncStatus:    models.SyncStatusSuccess,
	}
	if mutate != nil {
		mutate(rec)
	}
	require.NoError(t, hDB().Create(rec).Error)
	return rec
}

func TestImageHandler_GetImages(t *testing.T) {
	db := newTestDB(t)
	withGlobalDB(t, db)

	r := hNewEngine()
	h := NewImageHandler()
	r.GET("/images", h.GetImages)

	// 造数据：同组合两条（去重保留最大 ID）+ 一条 amd64 + 一条 arm64
	hImageRecord(t, func(x *models.ImageSyncRecord) { x.OriginalImage = "dup"; x.SyncStatus = models.SyncStatusFailed })
	hImageRecord(t, func(x *models.ImageSyncRecord) {
		x.OriginalImage = "dup"
		x.SyncStatus = models.SyncStatusFailed
		x.ErrorMessage = "second"
	})
	hImageRecord(t, func(x *models.ImageSyncRecord) { x.OriginalImage = "arm-app"; x.Architecture = "arm64" })
	hImageRecord(t, func(x *models.ImageSyncRecord) { x.OriginalImage = "plain"; x.Architecture = "" })

	t.Run("默认查询200", func(t *testing.T) {
		w, body := hPerformJSON(t, r, http.MethodGet, "/images", "", nil)
		require.Equal(t, http.StatusOK, w.Code)
		assert.EqualValues(t, 4, body["total"])
		data := body["data"].([]interface{})
		assert.Len(t, data, 4)
	})

	t.Run("状态过滤", func(t *testing.T) {
		w, body := hPerformJSON(t, r, http.MethodGet, "/images?status=failed", "", nil)
		require.Equal(t, http.StatusOK, w.Code)
		assert.EqualValues(t, 2, body["total"])
	})

	t.Run("搜索多字段命中", func(t *testing.T) {
		w, body := hPerformJSON(t, r, http.MethodGet, "/images?search=arm-app", "", nil)
		require.Equal(t, http.StatusOK, w.Code)
		assert.EqualValues(t, 1, body["total"])
	})

	t.Run("amd64过滤含空架构", func(t *testing.T) {
		w, body := hPerformJSON(t, r, http.MethodGet, "/images?architecture=amd64", "", nil)
		require.Equal(t, http.StatusOK, w.Code)
		// 空架构记录（dup×2 与 plain）按 amd64 兼容计入，共 3 条
		assert.EqualValues(t, 3, body["total"])
	})

	t.Run("arm64过滤", func(t *testing.T) {
		w, body := hPerformJSON(t, r, http.MethodGet, "/images?architecture=arm64", "", nil)
		require.Equal(t, http.StatusOK, w.Code)
		assert.EqualValues(t, 1, body["total"])
	})

	t.Run("分页参数越界兜底", func(t *testing.T) {
		w, body := hPerformJSON(t, r, http.MethodGet, "/images?page=-3&page_size=999", "", nil)
		require.Equal(t, http.StatusOK, w.Code)
		assert.EqualValues(t, 1, body["page"])
		assert.EqualValues(t, 10, body["page_size"])
	})

	t.Run("非法排序字段回退默认", func(t *testing.T) {
		w, body := hPerformJSON(t, r, http.MethodGet, "/images?sort_by=evil_col;drop&sort_order=asc", "", nil)
		require.Equal(t, http.StatusOK, w.Code)
		assert.EqualValues(t, 4, body["total"])
	})

	t.Run("去重模式", func(t *testing.T) {
		w, body := hPerformJSON(t, r, http.MethodGet, "/images?deduplicate=true", "", nil)
		require.Equal(t, http.StatusOK, w.Code)
		assert.EqualValues(t, 3, body["total"], "dup 两条只留一条")
	})

	t.Run("去重模式叠加过滤", func(t *testing.T) {
		w, body := hPerformJSON(t, r, http.MethodGet, "/images?deduplicate=true&status=success", "", nil)
		require.Equal(t, http.StatusOK, w.Code)
		assert.EqualValues(t, 2, body["total"])
	})

	t.Run("ACR地址后处理_空地址重建", func(t *testing.T) {
		hImageRecord(t, func(x *models.ImageSyncRecord) {
			x.OriginalImage = "postprocess-empty"
			x.SyncStatus = models.SyncStatusSuccess
			x.ACRImage = ""
		})
		w, body := hPerformJSON(t, r, http.MethodGet, "/images?search=postprocess-empty", "", nil)
		require.Equal(t, http.StatusOK, w.Code)
		data := body["data"].([]interface{})
		require.Len(t, data, 1)
		item := data[0].(map[string]interface{})
		assert.NotEmpty(t, item["acr_image"], "成功记录的空 ACR 地址应被重建")
	})

	t.Run("ACR地址后处理_补标签", func(t *testing.T) {
		hImageRecord(t, func(x *models.ImageSyncRecord) {
			x.OriginalImage = "postprocess-notag"
			x.SyncStatus = models.SyncStatusSuccess
			x.ACRImage = "reg.example.com/ns/postprocess-notag"
		})
		w, body := hPerformJSON(t, r, http.MethodGet, "/images?search=postprocess-notag", "", nil)
		require.Equal(t, http.StatusOK, w.Code)
		item := body["data"].([]interface{})[0].(map[string]interface{})
		assert.Contains(t, item["acr_image"], ":", "缺标签的 ACR 地址应补 :1.25")
	})

	t.Run("ACR地址后处理_空tag补latest", func(t *testing.T) {
		hImageRecord(t, func(x *models.ImageSyncRecord) {
			x.OriginalImage = "postprocess-latest"
			x.SyncStatus = models.SyncStatusSuccess
			x.Tag = ""
			x.ACRImage = "reg.example.com/ns/postprocess-latest"
		})
		w, body := hPerformJSON(t, r, http.MethodGet, "/images?search=postprocess-latest", "", nil)
		require.Equal(t, http.StatusOK, w.Code)
		item := body["data"].([]interface{})[0].(map[string]interface{})
		assert.Contains(t, item["acr_image"], ":latest")
	})

	t.Run("表缺失500_普通模式", func(t *testing.T) {
		hDropTable(t, hDB(), &models.ImageSyncRecord{}, "image_sync_records")
		w, _ := hPerformJSON(t, r, http.MethodGet, "/images", "", nil)
		assert.Equal(t, http.StatusInternalServerError, w.Code)
	})

	t.Run("表缺失500_去重模式", func(t *testing.T) {
		hDropTable(t, hDB(), &models.ImageSyncRecord{}, "image_sync_records")
		w, _ := hPerformJSON(t, r, http.MethodGet, "/images?deduplicate=true", "", nil)
		assert.Equal(t, http.StatusInternalServerError, w.Code)
	})
}

func TestImageHandler_GetImageAndDelete(t *testing.T) {
	db := newTestDB(t)
	withGlobalDB(t, db)
	h := NewImageHandler()
	r := hNewEngine()
	r.GET("/images/:id", h.GetImage)
	r.DELETE("/images/:id", h.DeleteImage)

	rec := hImageRecord(t, nil)

	t.Run("GetImage_非法ID400", func(t *testing.T) {
		w, _ := hPerformJSON(t, r, http.MethodGet, "/images/xx", "", nil)
		assert.Equal(t, http.StatusBadRequest, w.Code)
	})

	t.Run("GetImage_不存在404", func(t *testing.T) {
		w, _ := hPerformJSON(t, r, http.MethodGet, "/images/424242", "", nil)
		assert.Equal(t, http.StatusNotFound, w.Code)
	})

	t.Run("GetImage_成功记录ACR补全", func(t *testing.T) {
		empty := hImageRecord(t, func(x *models.ImageSyncRecord) { x.ACRImage = ""; x.OriginalImage = "detail-empty" })
		w, body := hPerformJSON(t, r, http.MethodGet, fmt.Sprintf("/images/%d", empty.ID), "", nil)
		require.Equal(t, http.StatusOK, w.Code)
		assert.NotEmpty(t, body["acr_image"])
	})

	t.Run("DeleteImage_非法ID400", func(t *testing.T) {
		w, _ := hPerformJSON(t, r, http.MethodDelete, "/images/bad", "", nil)
		assert.Equal(t, http.StatusBadRequest, w.Code)
	})

	t.Run("DeleteImage_不存在404", func(t *testing.T) {
		w, _ := hPerformJSON(t, r, http.MethodDelete, "/images/424242", "", nil)
		assert.Equal(t, http.StatusNotFound, w.Code)
	})

	t.Run("DeleteImage_成功200", func(t *testing.T) {
		w, _ := hPerformJSON(t, r, http.MethodDelete, fmt.Sprintf("/images/%d", rec.ID), "", nil)
		assert.Equal(t, http.StatusOK, w.Code)
		w, _ = hPerformJSON(t, r, http.MethodGet, fmt.Sprintf("/images/%d", rec.ID), "", nil)
		assert.Equal(t, http.StatusNotFound, w.Code)
	})

	t.Run("DeleteImage_删除失败500", func(t *testing.T) {
		del := hImageRecord(t, func(x *models.ImageSyncRecord) { x.OriginalImage = "del-fail" })
		// GORM 软删除是 UPDATE deleted_at，需阻断 UPDATE 而非 DELETE
		hFailUpdatesOn(t, hDB(), "image_sync_records")
		w, _ := hPerformJSON(t, r, http.MethodDelete, fmt.Sprintf("/images/%d", del.ID), "", nil)
		assert.Equal(t, http.StatusInternalServerError, w.Code)
	})
}

func TestImageHandler_GetImageStats(t *testing.T) {
	db := newTestDB(t)
	withGlobalDB(t, db)
	h := NewImageHandler()
	r := hNewEngine()
	r.GET("/images/stats", h.GetImageStats)

	hImageRecord(t, func(x *models.ImageSyncRecord) { x.SyncStatus = models.SyncStatusPending })
	hImageRecord(t, func(x *models.ImageSyncRecord) { x.SyncStatus = models.SyncStatusSyncing })
	hImageRecord(t, func(x *models.ImageSyncRecord) { x.SyncStatus = models.SyncStatusSuccess })
	hImageRecord(t, func(x *models.ImageSyncRecord) { x.SyncStatus = models.SyncStatusFailed })

	w, body := hPerformJSON(t, r, http.MethodGet, "/images/stats", "", nil)
	require.Equal(t, http.StatusOK, w.Code)
	assert.EqualValues(t, 4, body["total"])
	assert.EqualValues(t, 1, body["pending"])
	assert.EqualValues(t, 1, body["syncing"])
	assert.EqualValues(t, 1, body["success"])
	assert.EqualValues(t, 1, body["failed"])
}

func TestImageHandler_CheckImageExists(t *testing.T) {
	enc := hNewEncSvc(t)

	newEnv := func(t *testing.T, mode string) (http.Handler, *models.ImageSyncRecord, *models.ImageSyncRecord) {
		t.Helper()
		db := newTestDB(t)
		withGlobalDB(t, db)
		ts := hLocalRegistry(t, mode)
		acr := &models.AcrRegistry{
			RegistryURL:  hLocalRegistryHost(ts),
			Namespace:    "ns",
			Alias:        "local",
			Username:     "",
			Password:     "",
			RegistryType: models.RegistryTypeGeneric,
		}
		require.NoError(t, db.Create(acr).Error)

		h := NewImageHandler()
		r := hNewEngine()
		r.POST("/images/:id/check", h.CheckImageExists)

		failedRec := hImageRecord(t, func(x *models.ImageSyncRecord) {
			x.AcrRegistryID = acr.ID
			x.OriginalImage = "nginx"
			x.SyncStatus = models.SyncStatusFailed
		})
		okRec := hImageRecord(t, func(x *models.ImageSyncRecord) {
			x.AcrRegistryID = acr.ID
			x.OriginalImage = "redis"
			x.SyncStatus = models.SyncStatusSuccess
		})
		return r, failedRec, okRec
	}

	t.Run("非法ID400", func(t *testing.T) {
		db := newTestDB(t)
		withGlobalDB(t, db)
		h := NewImageHandler()
		r := hNewEngine()
		r.POST("/images/:id/check", h.CheckImageExists)
		w, _ := hPerformJSON(t, r, http.MethodPost, "/images/xx/check", "", nil)
		assert.Equal(t, http.StatusBadRequest, w.Code)
	})

	t.Run("记录不存在404", func(t *testing.T) {
		db := newTestDB(t)
		withGlobalDB(t, db)
		h := NewImageHandler()
		r := hNewEngine()
		r.POST("/images/:id/check", h.CheckImageExists)
		w, _ := hPerformJSON(t, r, http.MethodPost, "/images/424242/check", "", nil)
		assert.Equal(t, http.StatusNotFound, w.Code)
	})

	t.Run("存在且原状态失败_升级成功", func(t *testing.T) {
		r, failedRec, _ := newEnv(t, "ok")
		w, body := hPerformJSON(t, r, http.MethodPost, fmt.Sprintf("/images/%d/check", failedRec.ID), "", nil)
		require.Equal(t, http.StatusOK, w.Code, w.Body.String())
		assert.Equal(t, true, body["exists"])
		var updated models.ImageSyncRecord
		require.NoError(t, hDB().First(&updated, failedRec.ID).Error)
		assert.Equal(t, models.SyncStatusSuccess, updated.SyncStatus)
		assert.Contains(t, updated.ACRImage, "nginx")
		assert.Contains(t, updated.AcrArchitectures, "amd64")
		assert.Contains(t, updated.AcrArchitectures, "arm64")
	})

	t.Run("存在且原状态非失败_仅刷新架构", func(t *testing.T) {
		r, _, okRec := newEnv(t, "ok")
		w, body := hPerformJSON(t, r, http.MethodPost, fmt.Sprintf("/images/%d/check", okRec.ID), "", nil)
		require.Equal(t, http.StatusOK, w.Code)
		assert.Equal(t, true, body["exists"])
		var updated models.ImageSyncRecord
		require.NoError(t, hDB().First(&updated, okRec.ID).Error)
		assert.Equal(t, models.SyncStatusSuccess, updated.SyncStatus, "非失败状态不应被改动")
	})

	t.Run("不存在_标记失败", func(t *testing.T) {
		r, failedRec, _ := newEnv(t, "missing")
		w, body := hPerformJSON(t, r, http.MethodPost, fmt.Sprintf("/images/%d/check", failedRec.ID), "", nil)
		require.Equal(t, http.StatusOK, w.Code)
		assert.Equal(t, false, body["exists"])
		var updated models.ImageSyncRecord
		require.NoError(t, hDB().First(&updated, failedRec.ID).Error)
		assert.Equal(t, models.SyncStatusFailed, updated.SyncStatus)
		assert.Contains(t, updated.ErrorMessage, "不存在")
	})

	t.Run("检测异常500", func(t *testing.T) {
		r, failedRec, _ := newEnv(t, "error")
		w, _ := hPerformJSON(t, r, http.MethodPost, fmt.Sprintf("/images/%d/check", failedRec.ID), "", nil)
		assert.Equal(t, http.StatusInternalServerError, w.Code)
	})

	_ = enc
}

func TestImageHandler_BatchCheckImages(t *testing.T) {
	setup := func(t *testing.T, mode string) (http.Handler, *models.ImageSyncRecord, *models.ImageSyncRecord) {
		db := newTestDB(t)
		withGlobalDB(t, db)
		ts := hLocalRegistry(t, mode)
		acr := &models.AcrRegistry{
			RegistryURL: hLocalRegistryHost(ts), Namespace: "ns", Alias: "batch",
			RegistryType: models.RegistryTypeGeneric,
		}
		require.NoError(t, db.Create(acr).Error)
		h := NewImageHandler()
		r := hNewEngine()
		r.POST("/images/batch-check", h.BatchCheckImages)
		a := hImageRecord(t, func(x *models.ImageSyncRecord) { x.AcrRegistryID = acr.ID; x.OriginalImage = "nginx" })
		b := hImageRecord(t, func(x *models.ImageSyncRecord) { x.AcrRegistryID = acr.ID; x.OriginalImage = "redis" })
		return r, a, b
	}

	t.Run("坏请求400", func(t *testing.T) {
		db := newTestDB(t)
		withGlobalDB(t, db)
		h := NewImageHandler()
		r := hNewEngine()
		r.POST("/images/batch-check", h.BatchCheckImages)
		w, _ := hPerformJSON(t, r, http.MethodPost, "/images/batch-check", `{}`, nil)
		assert.Equal(t, http.StatusBadRequest, w.Code)
	})

	t.Run("空ID列表400", func(t *testing.T) {
		db := newTestDB(t)
		withGlobalDB(t, db)
		h := NewImageHandler()
		r := hNewEngine()
		r.POST("/images/batch-check", h.BatchCheckImages)
		w, _ := hPerformJSON(t, r, http.MethodPost, "/images/batch-check", `{"ids":[]}`, nil)
		assert.Equal(t, http.StatusBadRequest, w.Code)
	})

	t.Run("超过50个400", func(t *testing.T) {
		db := newTestDB(t)
		withGlobalDB(t, db)
		h := NewImageHandler()
		r := hNewEngine()
		r.POST("/images/batch-check", h.BatchCheckImages)
		ids := make([]int, 51)
		for i := range ids {
			ids[i] = i + 1
		}
		w, _ := hPerformJSON(t, r, http.MethodPost, "/images/batch-check",
			fmt.Sprintf(`{"ids":%v}`, ids), nil)
		assert.Equal(t, http.StatusBadRequest, w.Code)
	})

	t.Run("查询失败500", func(t *testing.T) {
		db := newTestDB(t)
		withGlobalDB(t, db)
		h := NewImageHandler()
		r := hNewEngine()
		r.POST("/images/batch-check", h.BatchCheckImages)
		hDropTable(t, hDB(), &models.ImageSyncRecord{}, "image_sync_records")
		w, _ := hPerformJSON(t, r, http.MethodPost, "/images/batch-check", `{"ids":[1,2]}`, nil)
		assert.Equal(t, http.StatusInternalServerError, w.Code)
	})

	t.Run("存在与不存在混合", func(t *testing.T) {
		r, a, b := setup(t, "ok")
		// a 命中 manifests(ok)，b 不在本地 registry 中（404 → 不存在分支）
		w, body := hPerformJSON(t, r, http.MethodPost, "/images/batch-check",
			fmt.Sprintf(`{"ids":[%d,%d]}`, a.ID, b.ID), nil)
		require.Equal(t, http.StatusOK, w.Code, w.Body.String())
		assert.EqualValues(t, 2, body["total"])
		msg, _ := body["message"].(string)
		assert.Contains(t, msg, "检测完成")
	})

	t.Run("检测异常计入失败", func(t *testing.T) {
		r, a, _ := setup(t, "error")
		w, body := hPerformJSON(t, r, http.MethodPost, "/images/batch-check",
			fmt.Sprintf(`{"ids":[%d]}`, a.ID), nil)
		require.Equal(t, http.StatusOK, w.Code)
		assert.EqualValues(t, 1, body["failed_count"])
		var updated models.ImageSyncRecord
		require.NoError(t, hDB().First(&updated, a.ID).Error)
		assert.Equal(t, models.SyncStatusFailed, updated.SyncStatus)
	})
}
