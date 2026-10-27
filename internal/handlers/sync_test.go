package handlers

// sync_test.go：镜像同步 Handler 测试（单元 + 边界 + 路由级 + 异步执行链路）。
// GitHub/Gitee 客户端硬编码域名无法指向 httptest，工作流触发链路依赖
// 测试库中缺失的 Git 配置自然失败（确定性错误路径），不发起真实同步；
// 镜像存在性检测借助 hLocalRegistry（go-containerregistry 对 127.0.0.1 走明文 http）离线覆盖。

import (
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"docker-image-sync-platform/internal/config"
	"docker-image-sync-platform/internal/models"
	"docker-image-sync-platform/internal/services"

	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"
)

// syncSetup 构造真实依赖的 SyncHandler 并挂好全局 DB，测试结束 Shutdown 收尾后台协程
func syncSetup(t *testing.T) (*SyncHandler, *gorm.DB) {
	t.Helper()
	db := newTestDB(t)
	withGlobalDB(t, db)
	enc := hNewEncSvc(t)
	factory := services.NewGitServiceFactory(enc)
	h := NewSyncHandler(factory)
	t.Cleanup(h.Shutdown)
	return h, db
}

// syncRouter 注册同步相关路由
func syncRouter(h *SyncHandler) *gin.Engine {
	r := hNewEngine()
	r.POST("/sync/submit", h.SubmitSync)
	r.POST("/sync/batch", h.SubmitBatchSync)
	r.POST("/sync/batch/mock", h.SubmitMockBatchSync)
	r.GET("/sync/status/:taskId", h.GetSyncStatus)
	r.GET("/sync/batch/status/:taskId", h.GetBatchSyncStatus)
	r.GET("/sync/history", h.GetSyncHistory)
	r.GET("/sync/suggest-acr", h.SuggestAcr)
	r.POST("/sync/check-acr", h.CheckAcr)
	r.POST("/images/:id/retry", h.RetrySync)
	return r
}

// syncFailInsertsOn 创建 BEFORE INSERT 触发器让指定表的 INSERT 全部失败
func syncFailInsertsOn(t *testing.T, db *gorm.DB, table string) {
	t.Helper()
	trig := fmt.Sprintf("trig_fail_ins_%s", table)
	require.NoError(t, db.Exec(fmt.Sprintf(
		"CREATE TRIGGER %s BEFORE INSERT ON %s BEGIN SELECT RAISE(ABORT, 'forced insert failure'); END",
		trig, table)).Error)
	t.Cleanup(func() { _ = db.Exec(fmt.Sprintf("DROP TRIGGER IF EXISTS %s", trig)).Error })
}

// syncSeedTask 直插任务（status 空则默认 pending）
func syncSeedTask(t *testing.T, db *gorm.DB, taskID string, mutate func(*models.SyncTask)) *models.SyncTask {
	t.Helper()
	task := &models.SyncTask{TaskID: taskID, Status: models.TaskStatusPending, TotalImages: 0, MaxConcurrent: 3}
	if mutate != nil {
		mutate(task)
	}
	if task.Status == "" {
		task.Status = models.TaskStatusPending
	}
	require.NoError(t, db.Create(task).Error)
	return task
}

// syncTaskByTaskID 查询任务（返回零值表示不存在）
func syncTaskByTaskID(t *testing.T, db *gorm.DB, taskID string) models.SyncTask {
	t.Helper()
	var task models.SyncTask
	err := db.Where("task_id = ?", taskID).First(&task).Error
	require.NoError(t, err)
	return task
}

func syncRecordsOfTask(t *testing.T, db *gorm.DB, taskID string) []models.ImageSyncRecord {
	t.Helper()
	var records []models.ImageSyncRecord
	require.NoError(t, db.Where("task_id = ?", taskID).Order("input_order ASC").Find(&records).Error)
	return records
}

// syncManyImageItems 生成 n 个批量镜像项的 JSON
func syncManyImageItems(n int) string {
	items := make([]map[string]string, n)
	for i := range items {
		items[i] = map[string]string{"source_image": fmt.Sprintf("img%d", i)}
	}
	b, _ := json.Marshal(items)
	return string(b)
}

// ---------------------------------------------------------------------------
// 纯函数单元测试
// ---------------------------------------------------------------------------

func TestSyncHandler_splitImageAndTag(t *testing.T) {
	image, tag := splitImageAndTag("nginx")
	assert.Equal(t, "nginx", image)
	assert.Equal(t, "latest", tag)

	image, tag = splitImageAndTag("nginx:1.25")
	assert.Equal(t, "nginx", image)
	assert.Equal(t, "1.25", tag)

	// 带端口号的 registry：最后一个 ':' 后含 '/'，不是 tag
	image, tag = splitImageAndTag("registry:5000/repo/img")
	assert.Equal(t, "registry:5000/repo/img", image)
	assert.Equal(t, "latest", tag)

	image, tag = splitImageAndTag("registry:5000/repo/img:v1")
	assert.Equal(t, "registry:5000/repo/img", image)
	assert.Equal(t, "v1", tag)

	image, tag = splitImageAndTag("  nginx:1.26  ")
	assert.Equal(t, "nginx", image)
	assert.Equal(t, "1.26", tag)
}

func TestSyncHandler_parseImageInfo(t *testing.T) {
	image, tag, arch := parseImageInfo("nginx:1.25")
	assert.Equal(t, "nginx", image)
	assert.Equal(t, "1.25", tag)
	assert.Empty(t, arch)

	image, tag, arch = parseImageInfo("--platform=linux/arm64 nginx:1.25")
	assert.Equal(t, "nginx", image)
	assert.Equal(t, "1.25", tag)
	assert.Equal(t, "linux/arm64", arch)

	// 不含 --platform 前缀时按普通镜像处理
	image, tag, arch = parseImageInfo("redis")
	assert.Equal(t, "redis", image)
	assert.Equal(t, "latest", tag)
	assert.Empty(t, arch)
}

func TestSyncHandler_parseImageNameAndTag(t *testing.T) {
	h, _ := syncSetup(t)
	image, tag := h.parseImageNameAndTag("quay.io/org/app:v2")
	assert.Equal(t, "quay.io/org/app", image)
	assert.Equal(t, "v2", tag)
}

func TestSyncHandler_normalizeBatchSyncRequest(t *testing.T) {
	// 合法值保留
	req := &models.BatchSyncRequest{MaxConcurrent: 5}
	normalizeBatchSyncRequest(req)
	assert.Equal(t, 5, req.MaxConcurrent)

	// 0 与越界值回退全局配置（init 中 MaxConcurrentJobs=3）
	req = &models.BatchSyncRequest{}
	normalizeBatchSyncRequest(req)
	assert.Equal(t, 3, req.MaxConcurrent)

	req = &models.BatchSyncRequest{MaxConcurrent: 11}
	normalizeBatchSyncRequest(req)
	assert.Equal(t, 3, req.MaxConcurrent)

	// 配置值在 1-10 内时作为默认并发
	hWithAppConfig(t, func(c *config.Config) { c.Sync.MaxConcurrentJobs = 6 })
	req = &models.BatchSyncRequest{}
	normalizeBatchSyncRequest(req)
	assert.Equal(t, 6, req.MaxConcurrent)
	// 配置值本身越界时兜底为 3
	hWithAppConfig(t, func(c *config.Config) { c.Sync.MaxConcurrentJobs = 99 })
	req = &models.BatchSyncRequest{}
	normalizeBatchSyncRequest(req)
	assert.Equal(t, 3, req.MaxConcurrent)
}

func TestSyncHandler_calculateEstimatedTime(t *testing.T) {
	h, _ := syncSetup(t)
	assert.Equal(t, 75, h.calculateEstimatedTime(5, 2))
	// 并发摊薄后小于单镜像耗时时按 30 秒兜底
	assert.Equal(t, 30, h.calculateEstimatedTime(1, 10))
}

func TestSyncHandler_groupRecordsByAcrRegistry(t *testing.T) {
	records := []models.ImageSyncRecord{
		{ID: 1, AcrRegistryID: 1},
		{ID: 2, AcrRegistryID: 2},
		{ID: 3, AcrRegistryID: 0}, // 回退 fallback
	}
	groups := groupRecordsByAcrRegistry(records, 7)
	assert.Len(t, groups[1], 1)
	assert.Len(t, groups[2], 1)
	assert.Len(t, groups[7], 1, "ACR ID 为 0 的记录应归入任务级 fallback")
}

func TestSyncHandler_buildImageLineFromRecord(t *testing.T) {
	assert.Equal(t, "nginx", buildImageLineFromRecord(models.ImageSyncRecord{OriginalImage: "nginx"}))
	assert.Equal(t, "nginx:1.25", buildImageLineFromRecord(models.ImageSyncRecord{OriginalImage: "nginx", Tag: "1.25"}))
}

func TestSyncHandler_buildACRImageForRecord(t *testing.T) {
	h, db := syncSetup(t)
	acrID := hSeedAcrRegistry(t, db, "reg.example.com", "ns", true)
	rec := models.ImageSyncRecord{AcrRegistryID: acrID, OriginalImage: "library/nginx", Tag: "1.25"}
	assert.Equal(t, "reg.example.com/ns/nginx:1.25", h.buildACRImageForRecord(rec))

	// ACR ID 为 0 时走系统默认配置（无配置时使用内置默认 registry/namespace）
	rec = models.ImageSyncRecord{AcrRegistryID: 0, OriginalImage: "nginx", Tag: ""}
	assert.Contains(t, h.buildACRImageForRecord(rec), "/nginx:latest")
}

// ---------------------------------------------------------------------------
// SubmitSync
// ---------------------------------------------------------------------------

func TestSyncHandler_SubmitSync(t *testing.T) {
	t.Run("坏JSON_400", func(t *testing.T) {
		h, _ := syncSetup(t)
		r := syncRouter(h)
		w, _ := hPerformJSON(t, r, http.MethodPost, "/sync/submit", `not-json`, nil)
		assert.Equal(t, http.StatusBadRequest, w.Code)
	})

	t.Run("空镜像列表_400", func(t *testing.T) {
		h, _ := syncSetup(t)
		r := syncRouter(h)
		w, _ := hPerformJSON(t, r, http.MethodPost, "/sync/submit", `{"images":[]}`, nil)
		assert.Equal(t, http.StatusBadRequest, w.Code)
	})

	t.Run("超过单次上限_400", func(t *testing.T) {
		h, _ := syncSetup(t)
		r := syncRouter(h)
		images := make([]string, maxBatchSyncImages+1)
		for i := range images {
			images[i] = fmt.Sprintf("img%d", i)
		}
		b, _ := json.Marshal(images)
		w, body := hPerformJSON(t, r, http.MethodPost, "/sync/submit",
			fmt.Sprintf(`{"images":%s}`, b), nil)
		require.Equal(t, http.StatusBadRequest, w.Code)
		assert.Contains(t, body["error"], "单次最多同步")
	})

	t.Run("任务创建失败_500", func(t *testing.T) {
		h, db := syncSetup(t)
		r := syncRouter(h)
		syncFailInsertsOn(t, db, "sync_tasks")
		w, _ := hPerformJSON(t, r, http.MethodPost, "/sync/submit", `{"images":["nginx"]}`, nil)
		assert.Equal(t, http.StatusInternalServerError, w.Code)
	})

	t.Run("镜像记录创建失败回滚_500", func(t *testing.T) {
		h, db := syncSetup(t)
		r := syncRouter(h)
		syncFailInsertsOn(t, db, "image_sync_records")
		w, _ := hPerformJSON(t, r, http.MethodPost, "/sync/submit", `{"images":["nginx"]}`, nil)
		assert.Equal(t, http.StatusInternalServerError, w.Code)
		var taskCount int64
		db.Model(&models.SyncTask{}).Count(&taskCount)
		assert.EqualValues(t, 0, taskCount, "事务应整体回滚，不留任务残留")
	})

	t.Run("成功_异步处理失败收尾", func(t *testing.T) {
		h, db := syncSetup(t)
		r := syncRouter(h)
		w, body := hPerformJSON(t, r, http.MethodPost, "/sync/submit",
			`{"images":["nginx:1.25","redis"],"description":"测试任务","architecture":"arm64"}`, nil)
		require.Equal(t, http.StatusOK, w.Code, w.Body.String())
		taskID, _ := body["task_id"].(string)
		assert.NotEmpty(t, taskID)
		assert.Equal(t, models.TaskStatusPending, body["status"])
		assert.EqualValues(t, 2, body["total_images"])
		assert.Contains(t, body["estimated_completion"], "-")

		records := syncRecordsOfTask(t, db, taskID)
		require.Len(t, records, 2)
		assert.Equal(t, "nginx", records[0].OriginalImage)
		assert.Equal(t, "1.25", records[0].Tag)
		assert.Equal(t, "arm64", records[0].Architecture)
		assert.Equal(t, "redis", records[1].OriginalImage)
		assert.Equal(t, "latest", records[1].Tag)
		assert.EqualValues(t, 1, records[0].InputOrder)
		assert.EqualValues(t, 2, records[1].InputOrder)

		task := syncTaskByTaskID(t, db, taskID)
		assert.Equal(t, "nginx:1.25\nredis", task.ImagesJSON)
		assert.Equal(t, 1, task.MaxConcurrent, "单镜像同步固定顺序执行")
		assert.Equal(t, "测试任务", task.Description)

		// 等待后台协程收尾：测试库没有 Git 配置，workflow 触发必然失败 → 任务转 failed
		h.Shutdown()
		task = syncTaskByTaskID(t, db, taskID)
		assert.Equal(t, models.TaskStatusFailed, task.Status)
		assert.NotNil(t, task.CompletedAt)
		assert.NotEmpty(t, task.ErrorMessage)
		records = syncRecordsOfTask(t, db, taskID)
		for _, rec := range records {
			assert.Equal(t, models.SyncStatusFailed, rec.SyncStatus)
			assert.NotEmpty(t, rec.ErrorMessage)
		}
	})

	t.Run("注入身份走完整路由", func(t *testing.T) {
		h, db := syncSetup(t)
		r := syncRouter(h)
		api := r.Group("", hWithIdentity(7, "alice"))
		api.POST("/e2e/sync/submit", h.SubmitSync)
		w, body := hPerformJSON(t, r, http.MethodPost, "/e2e/sync/submit", `{"images":["alpine:3.19"]}`, nil)
		require.Equal(t, http.StatusOK, w.Code, w.Body.String())
		taskID, _ := body["task_id"].(string)
		h.Shutdown()
		assert.Equal(t, models.TaskStatusFailed, syncTaskByTaskID(t, db, taskID).Status)
	})
}

// ---------------------------------------------------------------------------
// SubmitBatchSync / SubmitMockBatchSync
// ---------------------------------------------------------------------------

func TestSyncHandler_SubmitBatchSync(t *testing.T) {
	t.Run("坏JSON_400", func(t *testing.T) {
		h, _ := syncSetup(t)
		r := syncRouter(h)
		w, _ := hPerformJSON(t, r, http.MethodPost, "/sync/batch", ``, nil)
		assert.Equal(t, http.StatusBadRequest, w.Code)
	})

	t.Run("超过单次上限_400", func(t *testing.T) {
		h, _ := syncSetup(t)
		r := syncRouter(h)
		w, body := hPerformJSON(t, r, http.MethodPost, "/sync/batch",
			fmt.Sprintf(`{"images":%s}`, syncManyImageItems(maxBatchSyncImages+1)), nil)
		require.Equal(t, http.StatusBadRequest, w.Code)
		assert.Contains(t, body["error"], "单次最多同步")
	})

	t.Run("并发参数归一化", func(t *testing.T) {
		h, _ := syncSetup(t)
		r := syncRouter(h)

		for _, tc := range []struct {
			name     string
			body     string
			expected int
		}{
			{"未传用配置默认", `{"images":[{"source_image":"a"}]}`, 3},
			{"越界回退默认", `{"images":[{"source_image":"a"}],"max_concurrent":11}`, 3},
			{"合法值保留", `{"images":[{"source_image":"a"}],"max_concurrent":5}`, 5},
		} {
			t.Run(tc.name, func(t *testing.T) {
				w, body := hPerformJSON(t, r, http.MethodPost, "/sync/batch", tc.body, nil)
				require.Equal(t, http.StatusOK, w.Code, w.Body.String())
				assert.EqualValues(t, tc.expected, body["max_concurrent"])
				// 等待后台任务收尾，避免 goroutine 泄漏到其他测试
				h.Shutdown()
			})
		}
	})

	t.Run("配置值非法时兜底为3", func(t *testing.T) {
		hWithAppConfig(t, func(c *config.Config) { c.Sync.MaxConcurrentJobs = 42 })
		req := &models.BatchSyncRequest{}
		normalizeBatchSyncRequest(req)
		assert.Equal(t, 3, req.MaxConcurrent)
	})

	t.Run("ACR归属解析与字段覆盖", func(t *testing.T) {
		h, db := syncSetup(t)
		r := syncRouter(h)
		acrID := hSeedAcrRegistry(t, db, "reg.example.com", "ns-a", true)
		require.NoError(t, db.Create(&models.AcrRepository{AcrRegistryID: acrID, RepositoryName: "affimg"}).Error)

		w, body := hPerformJSON(t, r, http.MethodPost, "/sync/batch", fmt.Sprintf(`{
			"images":[
				{"source_image":"affimg:2.0","target_tag":"v2","architecture":"arm64","priority":9,"description":"带亲和"},
				{"source_image":"--platform=linux/arm64 plain:1.0","description":"默认归属"}
			],
			"max_concurrent":2
		}`), nil)
		require.Equal(t, http.StatusOK, w.Code, w.Body.String())
		assert.EqualValues(t, 2, body["total_images"])
		taskID, _ := body["task_id"].(string)

		records := syncRecordsOfTask(t, db, taskID)
		require.Len(t, records, 2)

		// 仓库台账存在 affimg 归属 → 解析到该 ACR；target_tag/architecture 覆盖
		assert.EqualValues(t, acrID, records[0].AcrRegistryID)
		assert.Equal(t, "affimg", records[0].OriginalImage)
		assert.Equal(t, "v2", records[0].Tag)
		assert.Equal(t, "arm64", records[0].Architecture)
		assert.Equal(t, 9, records[0].Priority)
		assert.Equal(t, "带亲和", records[0].Description)
		assert.Equal(t, "affimg:v2", records[0].OriginalInput)

		// 无归属 → 默认 ACR
		assert.EqualValues(t, acrID, records[1].AcrRegistryID)
		assert.Equal(t, "plain", records[1].OriginalImage)
		assert.Equal(t, "1.0", records[1].Tag)
		assert.Equal(t, "linux/arm64", records[1].Architecture)

		task := syncTaskByTaskID(t, db, taskID)
		// 实际格式：源镜像:源tag:目标tag 与 --platform 前缀原样保留
		// TODO(疑似bug): "affimg:2.0:v2" 双冒号不是合法镜像引用，待确认 images.txt 生成逻辑
		assert.Equal(t, "affimg:2.0:v2\n--platform=linux/arm64 plain:1.0", task.ImagesJSON)
		assert.Equal(t, 2, task.MaxConcurrent)
		assert.EqualValues(t, 2, task.TotalImages)

		h.Shutdown()
		task = syncTaskByTaskID(t, db, taskID)
		assert.Equal(t, models.TaskStatusFailed, task.Status, "无 Git 配置时 workflow 触发失败")
	})

	t.Run("显式指定ACR时不解析", func(t *testing.T) {
		h, db := syncSetup(t)
		r := syncRouter(h)
		acrA := hSeedAcrRegistry(t, db, "reg-a.example.com", "ns-a", true)
		acrB := hSeedAcrRegistry(t, db, "reg-b.example.com", "ns-b", false)

		w, body := hPerformJSON(t, r, http.MethodPost, "/sync/batch",
			fmt.Sprintf(`{"images":[{"source_image":"img:1"}],"acr_registry_id":%d}`, acrB), nil)
		require.Equal(t, http.StatusOK, w.Code, w.Body.String())
		taskID, _ := body["task_id"].(string)
		records := syncRecordsOfTask(t, db, taskID)
		require.Len(t, records, 1)
		assert.EqualValues(t, acrB, records[0].AcrRegistryID, "显式指定 ACR 优先于默认")
		assert.NotEqual(t, acrA, records[0].AcrRegistryID)
		h.Shutdown()
	})

	t.Run("归属解析失败_500", func(t *testing.T) {
		h, db := syncSetup(t)
		r := syncRouter(h)
		// 仓库台账表缺失 → ResolveTargetAcr 内部查询报错 → 事务失败
		hDropTable(t, db, &models.AcrRepository{}, "acr_repositories")
		w, _ := hPerformJSON(t, r, http.MethodPost, "/sync/batch", `{"images":[{"source_image":"img:1"}]}`, nil)
		assert.Equal(t, http.StatusInternalServerError, w.Code)
		var taskCount int64
		db.Model(&models.SyncTask{}).Count(&taskCount)
		assert.EqualValues(t, 0, taskCount, "解析失败时任务应回滚")
	})

	t.Run("任务创建失败_500", func(t *testing.T) {
		h, db := syncSetup(t)
		r := syncRouter(h)
		syncFailInsertsOn(t, db, "sync_tasks")
		w, _ := hPerformJSON(t, r, http.MethodPost, "/sync/batch", `{"images":[{"source_image":"a"}]}`, nil)
		assert.Equal(t, http.StatusInternalServerError, w.Code)
	})

	t.Run("镜像记录创建失败_500", func(t *testing.T) {
		h, db := syncSetup(t)
		r := syncRouter(h)
		syncFailInsertsOn(t, db, "image_sync_records")
		w, _ := hPerformJSON(t, r, http.MethodPost, "/sync/batch", `{"images":[{"source_image":"a"}]}`, nil)
		assert.Equal(t, http.StatusInternalServerError, w.Code)
		var taskCount int64
		db.Model(&models.SyncTask{}).Count(&taskCount)
		assert.EqualValues(t, 0, taskCount)
	})
}

func TestSyncHandler_SubmitMockBatchSync(t *testing.T) {
	t.Run("坏JSON_400", func(t *testing.T) {
		h, _ := syncSetup(t)
		r := syncRouter(h)
		w, _ := hPerformJSON(t, r, http.MethodPost, "/sync/batch/mock", `x`, nil)
		assert.Equal(t, http.StatusBadRequest, w.Code)
	})

	t.Run("任务创建失败_500", func(t *testing.T) {
		h, db := syncSetup(t)
		r := syncRouter(h)
		syncFailInsertsOn(t, db, "sync_tasks")
		w, _ := hPerformJSON(t, r, http.MethodPost, "/sync/batch/mock", `{"images":[{"source_image":"a"}]}`, nil)
		assert.Equal(t, http.StatusInternalServerError, w.Code)
	})

	t.Run("目标存在_模拟同步成功", func(t *testing.T) {
		h, db := syncSetup(t)
		ts := hLocalRegistry(t, "ok")
		acrID := hSeedAcrRegistry(t, db, hLocalRegistryHost(ts), "ns", true)
		// 指向 generic 类型以便 Check/Detect 走明文 http
		require.NoError(t, db.Model(&models.AcrRegistry{}).Where("id = ?", acrID).
			Update("registry_type", models.RegistryTypeGeneric).Error)

		r := syncRouter(h)
		w, body := hPerformJSON(t, r, http.MethodPost, "/sync/batch/mock",
			`{"images":[{"source_image":"nginx:1.25"}],"max_concurrent":1}`, nil)
		require.Equal(t, http.StatusOK, w.Code, w.Body.String())
		assert.Equal(t, "模拟批量同步任务已提交，正在处理中...", body["message"])
		assert.EqualValues(t, 1, body["image_count"])
		assert.Equal(t, "30秒", body["estimated_time"])
		taskID, _ := body["task_id"].(string)

		// 模拟同步每镜像固定 sleep 2-5 秒，轮询等待终态（不依赖真实 sleep 固定值）
		require.Eventually(t, func() bool {
			var status string
			db.Raw("SELECT status FROM sync_tasks WHERE task_id = ?", taskID).Scan(&status)
			return status == models.TaskStatusSuccess
		}, 15*time.Second, 200*time.Millisecond)

		records := syncRecordsOfTask(t, db, taskID)
		require.Len(t, records, 1)
		rec := records[0]
		assert.Equal(t, models.SyncStatusSuccess, rec.SyncStatus)
		assert.Equal(t, fmt.Sprintf("%s/ns/nginx:1.25", hLocalRegistryHost(ts)), rec.ACRImage)
		assert.Contains(t, rec.AcrArchitectures, "amd64")
		assert.Contains(t, rec.AcrArchitectures, "arm64")
		assert.Greater(t, rec.Duration, int64(0))

		// 同步成功后自动登记仓库台账
		var repoCount int64
		db.Model(&models.AcrRepository{}).Where("acr_registry_id = ? AND repository_name = ?", acrID, "nginx").Count(&repoCount)
		assert.EqualValues(t, 1, repoCount)

		h.Shutdown()
	})

	t.Run("目标不存在_模拟同步失败", func(t *testing.T) {
		h, db := syncSetup(t)
		ts := hLocalRegistry(t, "missing")
		hSeedAcrRegistry(t, db, hLocalRegistryHost(ts), "ns", true)
		require.NoError(t, db.Model(&models.AcrRegistry{}).Where("id = ?", 1).
			Update("registry_type", models.RegistryTypeGeneric).Error)

		r := syncRouter(h)
		w, body := hPerformJSON(t, r, http.MethodPost, "/sync/batch/mock",
			`{"images":[{"source_image":"ghost:9.9"}]}`, nil)
		require.Equal(t, http.StatusOK, w.Code)
		taskID, _ := body["task_id"].(string)

		require.Eventually(t, func() bool {
			var status string
			db.Raw("SELECT status FROM sync_tasks WHERE task_id = ?", taskID).Scan(&status)
			return status == models.TaskStatusFailed
		}, 15*time.Second, 200*time.Millisecond)

		records := syncRecordsOfTask(t, db, taskID)
		require.Len(t, records, 1)
		assert.Equal(t, models.SyncStatusFailed, records[0].SyncStatus)
		assert.Contains(t, records[0].ErrorMessage, "目标镜像不存在于注册表中")

		task := syncTaskByTaskID(t, db, taskID)
		assert.Contains(t, task.ErrorMessage, "全部失败")
		h.Shutdown()
	})
}

// ---------------------------------------------------------------------------
// 状态查询类接口
// ---------------------------------------------------------------------------

func TestSyncHandler_GetSyncStatus(t *testing.T) {
	t.Run("任务不存在_404", func(t *testing.T) {
		h, _ := syncSetup(t)
		r := syncRouter(h)
		w, _ := hPerformJSON(t, r, http.MethodGet, "/sync/status/no-such-task", "", nil)
		assert.Equal(t, http.StatusNotFound, w.Code)
	})

	t.Run("空任务ID_400", func(t *testing.T) {
		h, _ := syncSetup(t)
		w := httptest.NewRecorder()
		c, _ := gin.CreateTestContext(w)
		c.Request = httptest.NewRequest(http.MethodGet, "/sync/status/", nil)
		c.Params = gin.Params{{Key: "taskId", Value: ""}}
		h.GetSyncStatus(c)
		assert.Equal(t, http.StatusBadRequest, w.Code)
	})

	t.Run("镜像记录查询失败_500", func(t *testing.T) {
		h, db := syncSetup(t)
		r := syncRouter(h)
		syncSeedTask(t, db, "task-err", nil)
		hDropTable(t, db, &models.ImageSyncRecord{}, "image_sync_records")
		w, _ := hPerformJSON(t, r, http.MethodGet, "/sync/status/task-err", "", nil)
		assert.Equal(t, http.StatusInternalServerError, w.Code)
	})

	t.Run("成功_含各状态计数", func(t *testing.T) {
		h, db := syncSetup(t)
		r := syncRouter(h)
		syncSeedTask(t, db, "task-ok", func(task *models.SyncTask) {
			task.Status = models.TaskStatusRunning
			task.TotalImages = 4
			task.GitHubRunID = "123"
			task.GitHubActionURL = "https://github.com/o/r/actions/runs/123"
			task.Progress = 50.0
		})
		hSeedSyncRecord(t, db, &models.ImageSyncRecord{TaskID: "task-ok", OriginalImage: "a", SyncStatus: models.SyncStatusPending, InputOrder: 1})
		hSeedSyncRecord(t, db, &models.ImageSyncRecord{TaskID: "task-ok", OriginalImage: "b", SyncStatus: models.SyncStatusSyncing, InputOrder: 2})
		hSeedSyncRecord(t, db, &models.ImageSyncRecord{TaskID: "task-ok", OriginalImage: "c", SyncStatus: models.SyncStatusSuccess, InputOrder: 3})
		hSeedSyncRecord(t, db, &models.ImageSyncRecord{TaskID: "task-ok", OriginalImage: "d", SyncStatus: models.SyncStatusFailed, InputOrder: 4})

		w, body := hPerformJSON(t, r, http.MethodGet, "/sync/status/task-ok", "", nil)
		require.Equal(t, http.StatusOK, w.Code, w.Body.String())
		assert.Equal(t, "task-ok", body["task_id"])
		assert.Equal(t, models.TaskStatusRunning, body["status"])
		assert.Equal(t, "123", body["github_run_id"])
		images := body["images"].(map[string]interface{})
		assert.EqualValues(t, 1, images["pending"])
		assert.EqualValues(t, 1, images["syncing"])
		assert.EqualValues(t, 1, images["success"])
		assert.EqualValues(t, 1, images["failed"])
		assert.Len(t, images["records"], 4)
	})
}

func TestSyncHandler_GetBatchSyncStatus(t *testing.T) {
	h, _ := syncSetup(t)
	r := syncRouter(h)
	w, body := hPerformJSON(t, r, http.MethodGet, "/sync/batch/status/whatever", "", nil)
	require.Equal(t, http.StatusGone, w.Code)
	assert.Equal(t, "FEATURE_DEPRECATED", body["code"])
	assert.Contains(t, body["error"], "已废弃")
}

func TestSyncHandler_GetSyncHistory(t *testing.T) {
	h, db := syncSetup(t)
	r := syncRouter(h)
	syncSeedTask(t, db, "t1", func(task *models.SyncTask) { task.Status = models.TaskStatusCompleted })
	syncSeedTask(t, db, "t2", func(task *models.SyncTask) { task.Status = models.TaskStatusFailed })
	syncSeedTask(t, db, "t3", func(task *models.SyncTask) { task.Status = models.TaskStatusCompleted })

	t.Run("默认分页", func(t *testing.T) {
		w, body := hPerformJSON(t, r, http.MethodGet, "/sync/history", "", nil)
		require.Equal(t, http.StatusOK, w.Code)
		assert.EqualValues(t, 3, body["total"])
		assert.EqualValues(t, 1, body["page"])
		assert.EqualValues(t, 20, body["page_size"])
		assert.Len(t, body["data"], 3)
	})

	t.Run("分页参数", func(t *testing.T) {
		w, body := hPerformJSON(t, r, http.MethodGet, "/sync/history?page=2&page_size=2", "", nil)
		require.Equal(t, http.StatusOK, w.Code)
		assert.EqualValues(t, 3, body["total"])
		assert.EqualValues(t, 2, body["page"])
		assert.EqualValues(t, 2, body["page_size"])
		assert.Len(t, body["data"], 1)
	})

	t.Run("非法分页参数回退默认", func(t *testing.T) {
		w, body := hPerformJSON(t, r, http.MethodGet, "/sync/history?page=abc&page=-1&page_size=999&page_size=0", "", nil)
		require.Equal(t, http.StatusOK, w.Code)
		assert.EqualValues(t, 1, body["page"])
		assert.EqualValues(t, 20, body["page_size"], "超过 100 的 page_size 应被忽略")
	})

	t.Run("状态过滤", func(t *testing.T) {
		w, body := hPerformJSON(t, r, http.MethodGet, "/sync/history?status=failed", "", nil)
		require.Equal(t, http.StatusOK, w.Code)
		assert.EqualValues(t, 1, body["total"])
		data := body["data"].([]interface{})
		assert.Equal(t, "t2", data[0].(map[string]interface{})["task_id"])
	})

	t.Run("无匹配过滤", func(t *testing.T) {
		w, body := hPerformJSON(t, r, http.MethodGet, "/sync/history?status=paused", "", nil)
		require.Equal(t, http.StatusOK, w.Code)
		assert.EqualValues(t, 0, body["total"])
		assert.Empty(t, body["data"])
	})

	t.Run("查询失败_500", func(t *testing.T) {
		h2, db2 := syncSetup(t)
		r2 := syncRouter(h2)
		hDropTable(t, db2, &models.SyncTask{}, "sync_tasks")
		w, _ := hPerformJSON(t, r2, http.MethodGet, "/sync/history", "", nil)
		assert.Equal(t, http.StatusInternalServerError, w.Code)
	})

	t.Run("注入身份查询", func(t *testing.T) {
		r3 := hNewEngine()
		r3.GET("/history", hWithIdentity(3, "bob"), h.GetSyncHistory)
		w, body := hPerformJSON(t, r3, http.MethodGet, "/history", "", nil)
		require.Equal(t, http.StatusOK, w.Code)
		assert.EqualValues(t, 3, body["total"])
	})
}

// ---------------------------------------------------------------------------
// ACR 建议与冲突检查
// ---------------------------------------------------------------------------

func TestSyncHandler_SuggestAcr(t *testing.T) {
	t.Run("缺少image参数_400", func(t *testing.T) {
		h, _ := syncSetup(t)
		r := syncRouter(h)
		w, _ := hPerformJSON(t, r, http.MethodGet, "/sync/suggest-acr", "", nil)
		assert.Equal(t, http.StatusBadRequest, w.Code)
	})

	t.Run("空白image参数_400", func(t *testing.T) {
		h, _ := syncSetup(t)
		r := syncRouter(h)
		w, _ := hPerformJSON(t, r, http.MethodGet, "/sync/suggest-acr?image=%20%20", "", nil)
		assert.Equal(t, http.StatusBadRequest, w.Code)
	})

	t.Run("无ACR时回退建议", func(t *testing.T) {
		h, _ := syncSetup(t)
		r := syncRouter(h)
		w, body := hPerformJSON(t, r, http.MethodGet, "/sync/suggest-acr?image=nginx:1.25", "", nil)
		require.Equal(t, http.StatusOK, w.Code, w.Body.String())
		assert.Equal(t, "success", body["status"])
		data := body["data"].(map[string]interface{})
		assert.EqualValues(t, 0, data["suggested_acr_id"])
		// 无 ACR 时当前返回空 reason
		assert.Empty(t, data["suggestion_reason"])
	})

	t.Run("有归属时建议原ACR", func(t *testing.T) {
		h, db := syncSetup(t)
		r := syncRouter(h)
		acrID := hSeedAcrRegistry(t, db, "reg.example.com", "ns-x", true)
		require.NoError(t, db.Create(&models.AcrRepository{AcrRegistryID: acrID, RepositoryName: "nginx"}).Error)

		w, body := hPerformJSON(t, r, http.MethodGet, "/sync/suggest-acr?image=nginx:1.25", "", nil)
		require.Equal(t, http.StatusOK, w.Code)
		data := body["data"].(map[string]interface{})
		assert.EqualValues(t, acrID, data["suggested_acr_id"])
		assert.Equal(t, "affinity", data["suggestion_reason"])
		assert.Equal(t, "ns-x", data["suggested_namespace"])
		assert.NotNil(t, data["quota_summary"], "应返回配额摘要")
	})

	t.Run("解析失败_500", func(t *testing.T) {
		h, db := syncSetup(t)
		r := syncRouter(h)
		hDropTable(t, db, &models.AcrRepository{}, "acr_repositories")
		w, _ := hPerformJSON(t, r, http.MethodGet, "/sync/suggest-acr?image=nginx", "", nil)
		assert.Equal(t, http.StatusInternalServerError, w.Code)
	})
}

func TestSyncHandler_CheckAcr(t *testing.T) {
	t.Run("坏JSON_400", func(t *testing.T) {
		h, _ := syncSetup(t)
		r := syncRouter(h)
		w, _ := hPerformJSON(t, r, http.MethodPost, "/sync/check-acr", `bad`, nil)
		assert.Equal(t, http.StatusBadRequest, w.Code)
	})

	t.Run("服务查询失败_500", func(t *testing.T) {
		h, db := syncSetup(t)
		r := syncRouter(h)
		hDropTable(t, db, &models.AcrRegistry{}, "acr_registries")
		w, _ := hPerformJSON(t, r, http.MethodPost, "/sync/check-acr",
			`{"images":["nginx:1"]}`, nil)
		assert.Equal(t, http.StatusInternalServerError, w.Code)
	})

	t.Run("归属冲突检测", func(t *testing.T) {
		h, db := syncSetup(t)
		r := syncRouter(h)
		acrA := hSeedAcrRegistry(t, db, "reg-a.example.com", "ns-a", true)
		acrB := hSeedAcrRegistry(t, db, "reg-b.example.com", "ns-b", false)
		require.NoError(t, db.Create(&models.AcrRepository{AcrRegistryID: acrA, RepositoryName: "shared"}).Error)

		w, body := hPerformJSON(t, r, http.MethodPost, "/sync/check-acr",
			fmt.Sprintf(`{"images":["shared:1","fresh:2"],"acr_registry_id":%d}`, acrB), nil)
		require.Equal(t, http.StatusOK, w.Code, w.Body.String())
		assert.Equal(t, "success", body["status"])
		data := body["data"].(map[string]interface{})
		assert.Equal(t, true, data["has_any_conflict"])
		items := data["items"].([]interface{})
		require.Len(t, items, 2)
		assert.EqualValues(t, 1, data["new_repository_count"])
	})

	t.Run("无冲突", func(t *testing.T) {
		h, db := syncSetup(t)
		r := syncRouter(h)
		acrA := hSeedAcrRegistry(t, db, "reg-a.example.com", "ns-a", true)
		w, body := hPerformJSON(t, r, http.MethodPost, "/sync/check-acr",
			fmt.Sprintf(`{"images":["plain:1"],"acr_registry_id":%d}`, acrA), nil)
		require.Equal(t, http.StatusOK, w.Code)
		data := body["data"].(map[string]interface{})
		assert.Equal(t, false, data["has_any_conflict"])
	})
}

// ---------------------------------------------------------------------------
// RetrySync
// ---------------------------------------------------------------------------

func TestSyncHandler_RetrySync(t *testing.T) {
	newEnv := func(t *testing.T) (*SyncHandler, *gorm.DB, http.Handler, *models.ImageSyncRecord) {
		h, db := syncSetup(t)
		r := syncRouter(h)
		task := syncSeedTask(t, db, "retry-task", nil)
		rec := &models.ImageSyncRecord{
			TaskID: task.TaskID, OriginalImage: "nginx", Tag: "1.25",
			SyncStatus: models.SyncStatusFailed, ErrorMessage: "上次失败",
		}
		hSeedSyncRecord(t, db, rec)
		return h, db, r, rec
	}

	t.Run("非法ID_400", func(t *testing.T) {
		_, _, r, _ := newEnv(t)
		w, _ := hPerformJSON(t, r, http.MethodPost, "/images/abc/retry", "", nil)
		assert.Equal(t, http.StatusBadRequest, w.Code)
	})

	t.Run("镜像不存在_404", func(t *testing.T) {
		_, _, r, _ := newEnv(t)
		w, _ := hPerformJSON(t, r, http.MethodPost, "/images/424242/retry", "", nil)
		assert.Equal(t, http.StatusNotFound, w.Code)
	})

	t.Run("非失败状态不可重试_400", func(t *testing.T) {
		_, db, r, _ := newEnv(t)
		task := syncSeedTask(t, db, "ok-task", nil)
		okRec := hSeedSyncRecord(t, db, &models.ImageSyncRecord{
			TaskID: task.TaskID, OriginalImage: "ok", SyncStatus: models.SyncStatusSuccess,
		})
		w, body := hPerformJSON(t, r, http.MethodPost, fmt.Sprintf("/images/%d/retry", okRec), "", nil)
		require.Equal(t, http.StatusBadRequest, w.Code)
		assert.Contains(t, body["error"], "只有失败的镜像才能重试")
	})

	t.Run("重置状态失败_500", func(t *testing.T) {
		_, db, r, rec := newEnv(t)
		hFailUpdatesOn(t, db, "image_sync_records")
		w, _ := hPerformJSON(t, r, http.MethodPost, fmt.Sprintf("/images/%d/retry", rec.ID), "", nil)
		assert.Equal(t, http.StatusInternalServerError, w.Code)
	})

	t.Run("成功_重新执行同步链路", func(t *testing.T) {
		h, db, r, rec := newEnv(t)
		w, body := hPerformJSON(t, r, http.MethodPost, fmt.Sprintf("/images/%d/retry", rec.ID), "", nil)
		require.Equal(t, http.StatusOK, w.Code, w.Body.String())
		assert.Equal(t, "重试已启动", body["message"])

		// 等待后台协程完成重跑：仍因缺少 Git 配置失败，但错误信息应为新一轮执行的
		h.Shutdown()
		var updated models.ImageSyncRecord
		require.NoError(t, db.First(&updated, rec.ID).Error)
		assert.Equal(t, models.SyncStatusFailed, updated.SyncStatus)
		assert.NotContains(t, updated.ErrorMessage, "上次失败", "应写入新一轮执行错误")
		assert.Empty(t, updated.ACRImage, "重试时已清空上次 ACR 地址")
	})
}

// ---------------------------------------------------------------------------
// processSyncTask 与错误处理链路（直接调用，确定性覆盖）
// ---------------------------------------------------------------------------

func TestSyncHandler_processSyncTask(t *testing.T) {
	t.Run("任务不存在_错误收尾", func(t *testing.T) {
		h, db := syncSetup(t)
		h.processSyncTask("missing-task")
		var count int64
		db.Model(&models.SyncTask{}).Where("status = ?", models.TaskStatusFailed).Count(&count)
		assert.EqualValues(t, 0, count, "任务不存在时无从更新")
	})

	t.Run("镜像记录查询失败", func(t *testing.T) {
		h, db := syncSetup(t)
		syncSeedTask(t, db, "q-err", nil)
		hDropTable(t, db, &models.ImageSyncRecord{}, "image_sync_records")
		h.processSyncTask("q-err")
		// 记录表缺失使 handleSyncError 的镜像更新同样失败 → 事务回滚，任务保持 pending
		task := syncTaskByTaskID(t, db, "q-err")
		assert.Equal(t, models.TaskStatusPending, task.Status)
	})

	t.Run("无待同步记录_安静返回", func(t *testing.T) {
		h, db := syncSetup(t)
		syncSeedTask(t, db, "empty", nil)
		hSeedSyncRecord(t, db, &models.ImageSyncRecord{
			TaskID: "empty", OriginalImage: "done", SyncStatus: models.SyncStatusSuccess,
		})
		h.processSyncTask("empty")
		task := syncTaskByTaskID(t, db, "empty")
		assert.Equal(t, models.TaskStatusPending, task.Status, "无可处理记录不应改动任务状态")
	})

	t.Run("正常链路_触发失败收尾", func(t *testing.T) {
		h, db := syncSetup(t)
		acrID := hSeedAcrRegistry(t, db, "reg.example.com", "ns", true)
		syncSeedTask(t, db, "flow", func(task *models.SyncTask) { task.AcrRegistryID = acrID })
		hSeedSyncRecord(t, db, &models.ImageSyncRecord{
			TaskID: "flow", OriginalImage: "nginx", Tag: "1.25", SyncStatus: models.SyncStatusPending, AcrRegistryID: acrID, InputOrder: 1,
		})

		h.processSyncTask("flow")
		task := syncTaskByTaskID(t, db, "flow")
		assert.Equal(t, models.TaskStatusFailed, task.Status)
		assert.Contains(t, task.ErrorMessage, "失败")
		assert.NotNil(t, task.StartedAt, "处理时应记录开始时间")
		records := syncRecordsOfTask(t, db, "flow")
		require.Len(t, records, 1)
		assert.Equal(t, models.SyncStatusFailed, records[0].SyncStatus)
		assert.NotEmpty(t, records[0].ErrorMessage)
	})

	t.Run("任务状态更新失败_回滚", func(t *testing.T) {
		h, db := syncSetup(t)
		syncSeedTask(t, db, "upd-err", nil)
		hSeedSyncRecord(t, db, &models.ImageSyncRecord{
			TaskID: "upd-err", OriginalImage: "nginx", SyncStatus: models.SyncStatusPending,
		})
		hFailUpdatesOn(t, db, "sync_tasks")
		h.processSyncTask("upd-err")
		// handleSyncError 对任务/镜像的更新同样被触发器拦截，全部回滚
		var rec models.ImageSyncRecord
		require.NoError(t, db.Where("task_id = ?", "upd-err").First(&rec).Error)
		assert.Equal(t, models.SyncStatusPending, rec.SyncStatus)
	})

	t.Run("镜像状态批量更新失败_回滚", func(t *testing.T) {
		h, db := syncSetup(t)
		syncSeedTask(t, db, "rec-upd-err", nil)
		hSeedSyncRecord(t, db, &models.ImageSyncRecord{
			TaskID: "rec-upd-err", OriginalImage: "nginx", SyncStatus: models.SyncStatusPending,
		})
		hFailUpdatesOn(t, db, "image_sync_records")
		h.processSyncTask("rec-upd-err")
		// 镜像更新失败：主事务回滚（running 更新撤销），handleSyncError 的镜像更新同样被拦
		// → 错误处理事务也回滚，任务最终保持 pending
		task := syncTaskByTaskID(t, db, "rec-upd-err")
		assert.Equal(t, models.TaskStatusPending, task.Status)
	})
}

func TestSyncHandler_handleSyncError(t *testing.T) {
	t.Run("任务与镜像转失败", func(t *testing.T) {
		h, db := syncSetup(t)
		syncSeedTask(t, db, "err-task", nil)
		hSeedSyncRecord(t, db, &models.ImageSyncRecord{TaskID: "err-task", OriginalImage: "a", SyncStatus: models.SyncStatusPending, InputOrder: 1})
		hSeedSyncRecord(t, db, &models.ImageSyncRecord{TaskID: "err-task", OriginalImage: "b", SyncStatus: models.SyncStatusSyncing, InputOrder: 2})

		h.handleSyncError("err-task", "boom")
		task := syncTaskByTaskID(t, db, "err-task")
		assert.Equal(t, models.TaskStatusFailed, task.Status)
		assert.Equal(t, "boom", task.ErrorMessage)
		records := syncRecordsOfTask(t, db, "err-task")
		assert.Equal(t, models.SyncStatusFailed, records[0].SyncStatus, "pending 应转 failed")
		assert.Equal(t, models.SyncStatusFailed, records[1].SyncStatus, "syncing 应转 failed")
	})

	t.Run("任务更新失败_回滚", func(t *testing.T) {
		h, db := syncSetup(t)
		syncSeedTask(t, db, "err-task2", nil)
		hFailUpdatesOn(t, db, "sync_tasks")
		h.handleSyncError("err-task2", "boom")
		// 无从断言失败状态（更新被拦截），仅确认不 panic 且任务仍是 pending
		assert.Equal(t, models.TaskStatusPending, syncTaskByTaskID(t, db, "err-task2").Status)
	})

	t.Run("镜像更新失败_回滚", func(t *testing.T) {
		h, db := syncSetup(t)
		syncSeedTask(t, db, "err-task3", nil)
		hSeedSyncRecord(t, db, &models.ImageSyncRecord{TaskID: "err-task3", OriginalImage: "a", SyncStatus: models.SyncStatusPending})
		hFailUpdatesOn(t, db, "image_sync_records")
		h.handleSyncError("err-task3", "boom")
		// 同一事务：镜像更新失败整体回滚，任务也保持 pending
		assert.Equal(t, models.TaskStatusPending, syncTaskByTaskID(t, db, "err-task3").Status)
		var rec models.ImageSyncRecord
		require.NoError(t, db.Where("task_id = ?", "err-task3").First(&rec).Error)
		assert.Equal(t, models.SyncStatusPending, rec.SyncStatus, "镜像失败更新被回滚")
	})
}

func TestSyncHandler_verifyAndFinalize(t *testing.T) {
	// syncVerifyEnv 准备本地 registry + ACR + 任务 + 记录
	syncVerifyEnv := func(t *testing.T, mode string) (*SyncHandler, *gorm.DB, string, *models.ImageSyncRecord) {
		h, db := syncSetup(t)
		ts := hLocalRegistry(t, mode)
		acr := &models.AcrRegistry{
			RegistryURL: hLocalRegistryHost(ts), Namespace: "ns", Alias: "local",
			RegistryType: models.RegistryTypeGeneric,
		}
		require.NoError(t, db.Create(acr).Error)
		started := time.Now().Add(-time.Minute)
		task := syncSeedTask(t, db, "verify-task", nil)
		rec := &models.ImageSyncRecord{
			TaskID: task.TaskID, OriginalImage: "nginx", Tag: "1.25",
			SyncStatus: models.SyncStatusSyncing, AcrRegistryID: acr.ID, StartedAt: &started,
		}
		hSeedSyncRecord(t, db, rec)
		return h, db, task.TaskID, rec
	}

	t.Run("handleSyncSuccess_镜像存在_全部成功", func(t *testing.T) {
		h, db, taskID, rec := syncVerifyEnv(t, "ok")
		h.handleSyncSuccess(taskID)
		var updated models.ImageSyncRecord
		require.NoError(t, db.First(&updated, rec.ID).Error)
		assert.Equal(t, models.SyncStatusSuccess, updated.SyncStatus)
		assert.Contains(t, updated.ACRImage, "/ns/nginx:1.25")
		assert.Contains(t, updated.AcrArchitectures, "amd64")
		assert.Greater(t, updated.Duration, int64(0))

		task := syncTaskByTaskID(t, db, taskID)
		assert.Equal(t, models.TaskStatusCompleted, task.Status)
		assert.EqualValues(t, 1, task.CompletedImages)
		assert.EqualValues(t, 0, task.FailedImages)
		assert.Equal(t, 100.0, task.Progress)
	})

	t.Run("handleSyncSuccess_镜像不存在_全部失败", func(t *testing.T) {
		h, db, taskID, rec := syncVerifyEnv(t, "missing")
		h.handleSyncSuccess(taskID)
		var updated models.ImageSyncRecord
		require.NoError(t, db.First(&updated, rec.ID).Error)
		assert.Equal(t, models.SyncStatusFailed, updated.SyncStatus)
		assert.Contains(t, updated.ErrorMessage, "镜像未成功同步到ACR")

		task := syncTaskByTaskID(t, db, taskID)
		assert.Equal(t, models.TaskStatusFailed, task.Status)
		assert.EqualValues(t, 1, task.FailedImages)
	})

	t.Run("handleSyncSuccess_记录查询失败_安静返回", func(t *testing.T) {
		h, db, taskID, _ := syncVerifyEnv(t, "ok")
		hDropTable(t, db, &models.ImageSyncRecord{}, "image_sync_records")
		h.handleSyncSuccess(taskID)
		assert.Equal(t, models.TaskStatusPending, syncTaskByTaskID(t, db, taskID).Status)
	})

	t.Run("handlePartialSyncFailure_部分成功", func(t *testing.T) {
		h, db := syncSetup(t)
		okTS := hLocalRegistry(t, "ok")
		okAcr := &models.AcrRegistry{RegistryURL: hLocalRegistryHost(okTS), Namespace: "ok-ns", Alias: "ok", RegistryType: models.RegistryTypeGeneric}
		require.NoError(t, db.Create(okAcr).Error)
		deadAcr := &models.AcrRegistry{RegistryURL: "127.0.0.1:1", Namespace: "dead-ns", Alias: "dead", RegistryType: models.RegistryTypeGeneric}
		require.NoError(t, db.Create(deadAcr).Error)

		task := syncSeedTask(t, db, "partial-task", nil)
		started := time.Now().Add(-time.Minute)
		okRec := hSeedSyncRecord(t, db, &models.ImageSyncRecord{
			TaskID: task.TaskID, OriginalImage: "nginx", Tag: "1", SyncStatus: models.SyncStatusSyncing,
			AcrRegistryID: okAcr.ID, StartedAt: &started,
		})
		deadRec := hSeedSyncRecord(t, db, &models.ImageSyncRecord{
			TaskID: task.TaskID, OriginalImage: "redis", Tag: "1", SyncStatus: models.SyncStatusSyncing,
			AcrRegistryID: deadAcr.ID, StartedAt: &started,
		})

		h.handlePartialSyncFailure(task.TaskID, "workflow 挂了")
		var r1, r2 models.ImageSyncRecord
		require.NoError(t, db.First(&r1, okRec).Error)
		require.NoError(t, db.First(&r2, deadRec).Error)
		assert.Equal(t, models.SyncStatusSuccess, r1.SyncStatus, "本地 registry 中的镜像应验证成功")
		assert.Equal(t, models.SyncStatusFailed, r2.SyncStatus, "不可达 registry 的镜像应失败")
		assert.Contains(t, r2.ErrorMessage, "workflow 挂了")

		after := syncTaskByTaskID(t, db, task.TaskID)
		assert.Equal(t, models.TaskStatusPartialSuccess, after.Status)
		assert.EqualValues(t, 1, after.CompletedImages)
		assert.EqualValues(t, 1, after.FailedImages)
		assert.Contains(t, after.ErrorMessage, "1个镜像成功同步")
	})

	t.Run("handlePartialSyncFailure_无记录_安静返回", func(t *testing.T) {
		h, db := syncSetup(t)
		syncSeedTask(t, db, "no-rec-task", nil)
		h.handlePartialSyncFailure("no-rec-task", "x")
		assert.Equal(t, models.TaskStatusPending, syncTaskByTaskID(t, db, "no-rec-task").Status)
	})

	t.Run("handlePartialSyncFailure_记录查询失败_安静返回", func(t *testing.T) {
		h, db := syncSetup(t)
		syncSeedTask(t, db, "drop-rec-task", nil)
		hDropTable(t, db, &models.ImageSyncRecord{}, "image_sync_records")
		h.handlePartialSyncFailure("drop-rec-task", "x")
		assert.Equal(t, models.TaskStatusPending, syncTaskByTaskID(t, db, "drop-rec-task").Status)
	})

	t.Run("verifySyncRecords_状态更新失败仅记日志", func(t *testing.T) {
		h, db, taskID, rec := syncVerifyEnv(t, "ok")
		hFailUpdatesOn(t, db, "image_sync_records")
		success, failed := h.verifySyncRecords(taskID, []models.ImageSyncRecord{*rec}, "msg")
		assert.Equal(t, 0, success)
		assert.Equal(t, 0, failed, "更新失败时不应计入成功/失败")
	})

	t.Run("finalizeSyncTask_三种终态", func(t *testing.T) {
		h, db := syncSetup(t)
		syncSeedTask(t, db, "f1", nil)
		assert.Equal(t, models.TaskStatusFailed, h.finalizeSyncTask("f1", 3, 0, 3, "err"))
		syncSeedTask(t, db, "f2", nil)
		assert.Equal(t, models.TaskStatusPartialSuccess, h.finalizeSyncTask("f2", 3, 1, 2, ""))
		syncSeedTask(t, db, "f3", nil)
		assert.Equal(t, models.TaskStatusCompleted, h.finalizeSyncTask("f3", 3, 3, 0, ""))
		task := syncTaskByTaskID(t, db, "f3")
		assert.Equal(t, 100.0, task.Progress)
		assert.EqualValues(t, 3, task.CompletedImages)

		// 更新失败仅记日志，不影响返回值
		syncSeedTask(t, db, "f4", nil)
		hFailUpdatesOn(t, db, "sync_tasks")
		assert.Equal(t, models.TaskStatusCompleted, h.finalizeSyncTask("f4", 1, 1, 0, ""))
	})
}

// ---------------------------------------------------------------------------
// 模拟批量执行链路（直接调用）
// ---------------------------------------------------------------------------

func TestSyncHandler_processMockDirect(t *testing.T) {
	t.Run("任务不存在_状态更新无效果后返回", func(t *testing.T) {
		h, db := syncSetup(t)
		h.processMockBatchSyncTask("ghost-task")
		var count int64
		db.Model(&models.SyncTask{}).Count(&count)
		assert.EqualValues(t, 0, count)
	})

	t.Run("状态更新失败_返回", func(t *testing.T) {
		h, db := syncSetup(t)
		syncSeedTask(t, db, "mock-upd-err", nil)
		hFailUpdatesOn(t, db, "sync_tasks")
		h.processMockBatchSyncTask("mock-upd-err")
		assert.Equal(t, models.TaskStatusPending, syncTaskByTaskID(t, db, "mock-upd-err").Status)
	})

	t.Run("镜像记录查询失败_任务转失败", func(t *testing.T) {
		h, db := syncSetup(t)
		syncSeedTask(t, db, "mock-q-err", nil)
		hDropTable(t, db, &models.ImageSyncRecord{}, "image_sync_records")
		h.processMockBatchSyncTask("mock-q-err")
		task := syncTaskByTaskID(t, db, "mock-q-err")
		assert.Equal(t, models.TaskStatusFailed, task.Status)
		assert.Contains(t, task.ErrorMessage, "查询镜像记录失败")
	})

	t.Run("processSingleMockImage_状态更新失败_返回", func(t *testing.T) {
		h, db := syncSetup(t)
		ts := hLocalRegistry(t, "ok")
		acr := &models.AcrRegistry{RegistryURL: hLocalRegistryHost(ts), Namespace: "ns", RegistryType: models.RegistryTypeGeneric}
		require.NoError(t, db.Create(acr).Error)
		recID := hSeedSyncRecord(t, db, &models.ImageSyncRecord{
			TaskID: "mock-img", OriginalImage: "nginx", SyncStatus: models.SyncStatusPending, AcrRegistryID: acr.ID,
		})
		hFailUpdatesOn(t, db, "image_sync_records")
		h.processSingleMockImage("mock-img", models.ImageSyncRecord{ID: recID, AcrRegistryID: acr.ID})
		var updated models.ImageSyncRecord
		require.NoError(t, db.First(&updated, recID).Error)
		assert.Equal(t, models.SyncStatusPending, updated.SyncStatus, "状态更新被拦截，应保持原状")
	})

	t.Run("updateBatchTaskFinalStatus_分支矩阵", func(t *testing.T) {
		h, db := syncSetup(t)
		seed := func(taskID string, statuses ...string) {
			syncSeedTask(t, db, taskID, nil)
			for i, s := range statuses {
				hSeedSyncRecord(t, db, &models.ImageSyncRecord{
					TaskID: taskID, OriginalImage: fmt.Sprintf("%s-%d", taskID, i), SyncStatus: s, InputOrder: i + 1,
				})
			}
		}

		seed("final-success", models.SyncStatusSuccess, models.SyncStatusSuccess)
		h.updateBatchTaskFinalStatus("final-success")
		assert.Equal(t, models.TaskStatusSuccess, syncTaskByTaskID(t, db, "final-success").Status)

		seed("final-partial", models.SyncStatusSuccess, models.SyncStatusFailed)
		h.updateBatchTaskFinalStatus("final-partial")
		task := syncTaskByTaskID(t, db, "final-partial")
		assert.Equal(t, models.TaskStatusPartialSuccess, task.Status)
		assert.Contains(t, task.ErrorMessage, "部分成功")

		seed("final-failed", models.SyncStatusFailed)
		h.updateBatchTaskFinalStatus("final-failed")
		task = syncTaskByTaskID(t, db, "final-failed")
		assert.Equal(t, models.TaskStatusFailed, task.Status)
		assert.Contains(t, task.ErrorMessage, "全部失败")

		seed("final-pending", models.SyncStatusPending)
		h.updateBatchTaskFinalStatus("final-pending")
		task = syncTaskByTaskID(t, db, "final-pending")
		assert.Equal(t, models.TaskStatusRunning, task.Status, "仍有 pending 时任务应保持 running")
		assert.Nil(t, task.CompletedAt, "running 状态不应写完成时间")

		// 更新失败仅记日志
		seed("final-err", models.SyncStatusSuccess)
		hFailUpdatesOn(t, db, "sync_tasks")
		h.updateBatchTaskFinalStatus("final-err")
	})

	t.Run("updateBatchTaskProgress", func(t *testing.T) {
		h, db := syncSetup(t)
		syncSeedTask(t, db, "prog", nil)
		hSeedSyncRecord(t, db, &models.ImageSyncRecord{TaskID: "prog", OriginalImage: "a", SyncStatus: models.SyncStatusSuccess})
		hSeedSyncRecord(t, db, &models.ImageSyncRecord{TaskID: "prog", OriginalImage: "b", SyncStatus: models.SyncStatusPending})
		h.updateBatchTaskProgress("prog")
		assert.Equal(t, 50.0, syncTaskByTaskID(t, db, "prog").Progress)

		hFailUpdatesOn(t, db, "sync_tasks")
		h.updateBatchTaskProgress("prog") // 更新失败仅记日志
	})

	t.Run("handleBatchSyncError", func(t *testing.T) {
		h, db := syncSetup(t)
		syncSeedTask(t, db, "batch-err", nil)
		h.handleBatchSyncError("batch-err", "批量失败原因")
		task := syncTaskByTaskID(t, db, "batch-err")
		assert.Equal(t, models.TaskStatusFailed, task.Status)
		assert.Equal(t, "批量失败原因", task.ErrorMessage)

		syncSeedTask(t, db, "batch-err2", nil)
		hFailUpdatesOn(t, db, "sync_tasks")
		h.handleBatchSyncError("batch-err2", "x") // 更新失败仅记日志
	})

	t.Run("updateTaskStatus", func(t *testing.T) {
		h, db := syncSetup(t)
		syncSeedTask(t, db, "st-completed", nil)
		h.updateTaskStatus("st-completed", "completed", "")
		assert.NotNil(t, syncTaskByTaskID(t, db, "st-completed").CompletedAt)

		syncSeedTask(t, db, "st-failed", nil)
		h.updateTaskStatus("st-failed", "failed", "err-msg")
		task := syncTaskByTaskID(t, db, "st-failed")
		assert.NotNil(t, task.CompletedAt)
		assert.Equal(t, "err-msg", task.ErrorMessage)

		syncSeedTask(t, db, "st-other", nil)
		h.updateTaskStatus("st-other", "running", "")
		task = syncTaskByTaskID(t, db, "st-other")
		assert.Equal(t, "running", task.Status)
		assert.Nil(t, task.CompletedAt)

		syncSeedTask(t, db, "st-err", nil)
		hFailUpdatesOn(t, db, "sync_tasks")
		h.updateTaskStatus("st-err", "completed", "") // 更新失败仅记日志
	})
}

// ---------------------------------------------------------------------------
// Git workflow 辅助函数（不发起真实同步，只测确定性错误路径）
// ---------------------------------------------------------------------------

func TestSyncHandler_gitWorkflowHelpers(t *testing.T) {
	t.Run("updateImagesFile_任务不存在", func(t *testing.T) {
		h, db := syncSetup(t)
		hSeedSyncRecord(t, db, &models.ImageSyncRecord{
			TaskID: "ghost", OriginalImage: "nginx", Tag: "1",
		})
		_, err := h.updateImagesFile([]models.ImageSyncRecord{{TaskID: "ghost", OriginalImage: "nginx", Tag: "1"}})
		require.Error(t, err)
	})

	t.Run("updateImagesFile_API模式失败回退原服务也失败", func(t *testing.T) {
		h, db := syncSetup(t)
		task := syncSeedTask(t, db, "upd-task", nil)
		hSeedSyncRecord(t, db, &models.ImageSyncRecord{
			TaskID: task.TaskID, OriginalImage: "nginx", Tag: "1",
		})
		// 默认 useAPI=true：API 模式失败后回退原 Git 服务（无 Git 配置同样失败）
		_, err := h.updateImagesFile([]models.ImageSyncRecord{{TaskID: task.TaskID, OriginalImage: "nginx", Tag: "1"}})
		require.Error(t, err)
		assert.Contains(t, err.Error(), "原Git服务更新失败")

		// 关闭 API 模式直接走原服务
		h.gitServiceFactory.SetUseAPI(false)
		_, err = h.updateImagesFile([]models.ImageSyncRecord{{TaskID: task.TaskID, OriginalImage: "nginx", Tag: "1"}})
		require.Error(t, err)
	})

	t.Run("updateImagesFileWithAPI_无Git配置失败", func(t *testing.T) {
		h, _ := syncSetup(t)
		_, err := h.updateImagesFileWithAPI("nginx\n", "msg")
		require.Error(t, err)
	})

	t.Run("updateImagesAndTriggerWorkflow_无Git配置失败", func(t *testing.T) {
		h, _ := syncSetup(t)
		_, _, err := h.updateImagesAndTriggerWorkflow([]string{"nginx"}, map[string]string{}, "docker.yaml")
		require.Error(t, err)
	})

	t.Run("buildAcrWorkflowInputs_默认配置", func(t *testing.T) {
		h, db := syncSetup(t)
		inputs, err := h.buildAcrWorkflowInputs(0)
		require.NoError(t, err)
		assert.Equal(t, "registry.cn-hangzhou.aliyuncs.com", inputs["registry"], "未配置时使用内置默认")
		assert.Equal(t, "lpx03", inputs["namespace"])

		// 数据库配置优先
		hSetConfig(t, db, "aliyun_registry", "reg.example.com")
		hSetConfig(t, db, "aliyun_namespace", "db-ns")
		hSetConfig(t, db, "aliyun_username", "db-user")
		inputs, err = h.buildAcrWorkflowInputs(0)
		require.NoError(t, err)
		assert.Equal(t, "reg.example.com", inputs["registry"])
		assert.Equal(t, "db-ns", inputs["namespace"])
		assert.Equal(t, "db-user", inputs["registry_user"])
	})

	t.Run("buildAcrWorkflowInputs_ACR不存在", func(t *testing.T) {
		h, _ := syncSetup(t)
		_, err := h.buildAcrWorkflowInputs(424242)
		require.Error(t, err)
		assert.Contains(t, err.Error(), "ACR配置不存在")
	})

	t.Run("buildAcrWorkflowInputs_空密码ACR走明文inputs", func(t *testing.T) {
		h, db := syncSetup(t)
		acrID := hSeedAcrRegistry(t, db, "reg.example.com", "ns-a", false)
		inputs, err := h.buildAcrWorkflowInputs(acrID)
		require.NoError(t, err)
		assert.Equal(t, "reg.example.com", inputs["registry"])
		assert.Equal(t, "ns-a", inputs["namespace"])
		_, hasSecret := inputs["registry_password_secret"]
		assert.False(t, hasSecret, "空密码不触发 GitHub secret 写入")
	})

	t.Run("buildRegistryInputs_空密码", func(t *testing.T) {
		h, _ := syncSetup(t)
		inputs, err := h.buildRegistryInputs(0, "r", "ns", "u", "")
		require.NoError(t, err)
		assert.Equal(t, "r", inputs["registry"])
		assert.Empty(t, inputs["registry_password"])
	})

	t.Run("buildRegistryInputs_带密码写secret失败回退明文", func(t *testing.T) {
		h, _ := syncSetup(t)
		// 无有效 GitHub 凭据，secret 写入必然失败 → 回退明文 inputs（兼容旧 workflow）
		inputs, err := h.buildRegistryInputs(6, "r", "ns", "u", "secret-value")
		require.NoError(t, err)
		assert.Equal(t, "r", inputs["registry"])
		assert.Equal(t, "secret-value", inputs["registry_password"], "secret 写入失败应回退明文传递")
	})

	t.Run("waitForWorkflowRun_空runID直接返回", func(t *testing.T) {
		h, _ := syncSetup(t)
		h.waitForWorkflowRun("t", "")
	})

	t.Run("waitForWorkflowRun_收到关闭信号停止", func(t *testing.T) {
		h, _ := syncSetup(t)
		h.cancel() // 预先取消：第一次状态查询（远端失败）后立即走 ctx.Done 分支退出
		done := make(chan struct{})
		go func() {
			h.waitForWorkflowRun("t", "12345")
			close(done)
		}()
		select {
		case <-done:
		case <-time.After(45 * time.Second):
			t.Fatal("waitForWorkflowRun 未在超时内退出")
		}
	})

	t.Run("monitorGitHubActions_收到关闭信号停止", func(t *testing.T) {
		h, _ := syncSetup(t)
		h.cancel()
		done := make(chan struct{})
		go func() {
			h.monitorGitHubActions("t", "deadbeef")
			close(done)
		}()
		select {
		case <-done:
		case <-time.After(5 * time.Second):
			t.Fatal("monitorGitHubActions 未响应关闭信号")
		}
	})

	t.Run("registerRepositoryOnSyncSuccess_边界", func(t *testing.T) {
		h, db := syncSetup(t)
		h.registerRepositoryOnSyncSuccess(nil)                             // nil 记录
		h.registerRepositoryOnSyncSuccess(&models.ImageSyncRecord{})        // ACR ID 0
		acrID := hSeedAcrRegistry(t, db, "reg.example.com", "ns", false)
		h.registerRepositoryOnSyncSuccess(&models.ImageSyncRecord{
			AcrRegistryID: acrID, OriginalImage: "library/nginx", Tag: "1",
		})
		var count int64
		db.Model(&models.AcrRepository{}).Where("acr_registry_id = ? AND repository_name = ?", acrID, "nginx").Count(&count)
		assert.EqualValues(t, 1, count, "成功后应登记仓库台账")

		// 登记失败仅记警告
		syncFailInsertsOn(t, db, "acr_repositories")
		h.registerRepositoryOnSyncSuccess(&models.ImageSyncRecord{
			AcrRegistryID: acrID, OriginalImage: "redis", Tag: "2",
		})
	})
}
