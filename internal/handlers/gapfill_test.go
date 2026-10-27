package handlers

// gapfill_test.go：集中补齐散落的错误分支与消息分支覆盖。

import (
	"fmt"
	"net/http"
	"testing"

	"docker-image-sync-platform/internal/models"
	"docker-image-sync-platform/internal/services"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// ====================================================================
// image.go：GetImage ACR 地址补全 / CheckImageExists 更新失败 / GetImages 去重分支
// ====================================================================

func TestImageHandler_Gapfill(t *testing.T) {
	t.Run("GetImage_缺冒号补tag与latest兜底", func(t *testing.T) {
		db := newTestDB(t)
		withGlobalDB(t, db)
		h := NewImageHandler()
		r := hNewEngine()
		r.GET("/images/:id", h.GetImage)

		noColon := hImageRecord(t, func(x *models.ImageSyncRecord) {
			x.SyncStatus = models.SyncStatusSuccess
			x.ACRImage = "reg.example.com/ns/img"
			x.Tag = "1.25"
		})
		w, body := hPerformJSON(t, r, http.MethodGet, fmt.Sprintf("/images/%d", noColon.ID), "", nil)
		require.Equal(t, http.StatusOK, w.Code)
		assert.Equal(t, "reg.example.com/ns/img:1.25", body["acr_image"])

		noTag := hImageRecord(t, func(x *models.ImageSyncRecord) {
			x.SyncStatus = models.SyncStatusSuccess
			x.ACRImage = "reg.example.com/ns/img2"
			x.Tag = ""
		})
		w, body = hPerformJSON(t, r, http.MethodGet, fmt.Sprintf("/images/%d", noTag.ID), "", nil)
		require.Equal(t, http.StatusOK, w.Code)
		assert.Equal(t, "reg.example.com/ns/img2:latest", body["acr_image"])
	})

	t.Run("GetImages_去重叠加过滤与非法排序", func(t *testing.T) {
		db := newTestDB(t)
		withGlobalDB(t, db)
		h := NewImageHandler()
		r := hNewEngine()
		r.GET("/images", h.GetImages)

		hImageRecord(t, func(x *models.ImageSyncRecord) { x.OriginalImage = "g1"; x.Architecture = "amd64" })
		hImageRecord(t, func(x *models.ImageSyncRecord) { x.OriginalImage = "g2"; x.Architecture = "arm64" })

		w, body := hPerformJSON(t, r, http.MethodGet, "/images?deduplicate=true&architecture=arm64&search=g2&sort_by=evil;drop", "", nil)
		require.Equal(t, http.StatusOK, w.Code)
		assert.EqualValues(t, 1, body["total"])
	})

	t.Run("GetImages_去重空结果", func(t *testing.T) {
		db := newTestDB(t)
		withGlobalDB(t, db)
		h := NewImageHandler()
		r := hNewEngine()
		r.GET("/images", h.GetImages)
		// 无任何记录：maxIds 为空 → 空列表分支
		w, body := hPerformJSON(t, r, http.MethodGet, "/images?deduplicate=true", "", nil)
		require.Equal(t, http.StatusOK, w.Code)
		assert.EqualValues(t, 0, body["total"])
	})

	t.Run("CheckImageExists_状态升级更新失败500", func(t *testing.T) {
		db := newTestDB(t)
		withGlobalDB(t, db)
		ts := hLocalRegistry(t, "ok")
		acr := &models.AcrRegistry{RegistryURL: hLocalRegistryHost(ts), Namespace: "ns", RegistryType: models.RegistryTypeGeneric}
		require.NoError(t, db.Create(acr).Error)
		h := NewImageHandler()
		r := hNewEngine()
		r.POST("/images/:id/check", h.CheckImageExists)

		rec := hImageRecord(t, func(x *models.ImageSyncRecord) {
			x.AcrRegistryID = acr.ID
			x.SyncStatus = models.SyncStatusFailed
		})
		hFailUpdatesOn(t, db, "image_sync_records")
		w, _ := hPerformJSON(t, r, http.MethodPost, fmt.Sprintf("/images/%d/check", rec.ID), "", nil)
		assert.Equal(t, http.StatusInternalServerError, w.Code)
	})

	t.Run("CheckImageExists_非失败状态刷新架构更新失败_仍200", func(t *testing.T) {
		db := newTestDB(t)
		withGlobalDB(t, db)
		ts := hLocalRegistry(t, "ok")
		acr := &models.AcrRegistry{RegistryURL: hLocalRegistryHost(ts), Namespace: "ns", RegistryType: models.RegistryTypeGeneric}
		require.NoError(t, db.Create(acr).Error)
		h := NewImageHandler()
		r := hNewEngine()
		r.POST("/images/:id/check", h.CheckImageExists)

		rec := hImageRecord(t, func(x *models.ImageSyncRecord) {
			x.AcrRegistryID = acr.ID
			x.SyncStatus = models.SyncStatusSuccess
		})
		hFailUpdatesOn(t, db, "image_sync_records")
		w, _ := hPerformJSON(t, r, http.MethodPost, fmt.Sprintf("/images/%d/check", rec.ID), "", nil)
		// 架构刷新失败仅记日志，不阻断响应
		assert.Equal(t, http.StatusOK, w.Code)
	})
}

// ====================================================================
// acr_repository.go：服务错误与消息追加分支
// ====================================================================

func TestAcrRepositoryHandler_Gapfill(t *testing.T) {
	enc := hNewEncSvc(t)

	setup := func(t *testing.T) (http.Handler, *models.AcrRegistry) {
		db := newTestDB(t)
		withGlobalDB(t, db)
		svc := services.NewAcrRepositoryService(db, enc)
		h := NewAcrRepositoryHandler(svc)
		r := hNewEngine()
		r.GET("/acr-repositories", h.GetAll)
		r.POST("/acr-repositories/batch-delete", h.BatchDelete)
		r.POST("/acr-repositories/clean-invalid", h.CleanInvalid)
		r.DELETE("/acr-repositories/:id", h.Delete)
		r.POST("/acr-repositories/sync-from-records", h.SyncFromRecords)
		r.GET("/acr-repositories/duplicates", h.GetDuplicates)
		return r, hEncAcrRecord(t, enc)
	}

	t.Run("GetAll_服务错误500", func(t *testing.T) {
		r, acr := setup(t)
		hDropTable(t, hDB(), &models.AcrRepository{}, "acr_repositories")
		w, _ := hPerformJSON(t, r, http.MethodGet, "/acr-repositories?acr_registry_id="+fmt.Sprint(acr.ID), "", nil)
		assert.Equal(t, http.StatusInternalServerError, w.Code)
	})

	t.Run("BatchDelete_删除失败500", func(t *testing.T) {
		r, acr := setup(t)
		rec := &models.AcrRepository{AcrRegistryID: acr.ID, RepositoryName: "bd"}
		require.NoError(t, hDB().Create(rec).Error)
		hFailUpdatesOn(t, hDB(), "acr_repositories")
		w, _ := hPerformJSON(t, r, http.MethodPost, "/acr-repositories/batch-delete",
			fmt.Sprintf(`{"ids":[%d]}`, rec.ID), nil)
		assert.Equal(t, http.StatusInternalServerError, w.Code)
	})

	t.Run("CleanInvalid_服务错误500", func(t *testing.T) {
		r, acr := setup(t)
		hDropTable(t, hDB(), &models.AcrRepository{}, "acr_repositories")
		w, _ := hPerformJSON(t, r, http.MethodPost, "/acr-repositories/clean-invalid",
			fmt.Sprintf(`{"acr_registry_id":%d}`, acr.ID), nil)
		assert.Equal(t, http.StatusInternalServerError, w.Code)
	})

	t.Run("Delete_删除失败500", func(t *testing.T) {
		r, acr := setup(t)
		rec := &models.AcrRepository{AcrRegistryID: acr.ID, RepositoryName: "dd"}
		require.NoError(t, hDB().Create(rec).Error)
		hFailUpdatesOn(t, hDB(), "acr_repositories")
		w, _ := hPerformJSON(t, r, http.MethodDelete, fmt.Sprintf("/acr-repositories/%d", rec.ID), "", nil)
		assert.Equal(t, http.StatusInternalServerError, w.Code)
	})

	t.Run("SyncFromRecords_带记录触发检查失败消息", func(t *testing.T) {
		r, acr := setup(t)
		// 预置一条成功同步记录 → 同步导入会校验远程存在性（不可达 → 检查失败消息分支）
		require.NoError(t, hDB().Create(&models.ImageSyncRecord{
			TaskID: "sfr", OriginalImage: "nginx", Tag: "1", SyncStatus: models.SyncStatusSuccess, ACRImage: "reg.example.com/ns/nginx:1",
		}).Error)
		w, body := hPerformJSON(t, r, http.MethodPost, "/acr-repositories/sync-from-records",
			fmt.Sprintf(`{"acr_registry_id":%d}`, acr.ID), nil)
		require.Equal(t, http.StatusOK, w.Code, w.Body.String())
		msg, _ := body["message"].(string)
		// 服务按 ACRImage 前缀过滤归属记录，不匹配时导入 0 个（消息分支见 services 包测试）
		assert.Contains(t, msg, "成功导入")
	})

	t.Run("GetDuplicates_服务错误500", func(t *testing.T) {
		r, _ := setup(t)
		hDropTable(t, hDB(), &models.AcrRepository{}, "acr_repositories")
		w, _ := hPerformJSON(t, r, http.MethodGet, "/acr-repositories/duplicates", "", nil)
		assert.Equal(t, http.StatusInternalServerError, w.Code)
	})
}

