package services

import (
	"fmt"

	"docker-image-sync-platform/internal/database"
	"docker-image-sync-platform/internal/models"

	"github.com/google/uuid"
	"gorm.io/gorm"
)

type ProcessSyncFunc func(batchID string)

type SyncBatchService struct {
	db        *gorm.DB
	processor ProcessSyncFunc
}

func NewSyncBatchService(db *gorm.DB, processor ProcessSyncFunc) *SyncBatchService {
	return &SyncBatchService{db: db, processor: processor}
}

func (s *SyncBatchService) CreateRetryBatch(recordID uint) (string, error) {
	var old models.SyncRecord
	if err := s.db.First(&old, recordID).Error; err != nil {
		return "", fmt.Errorf("镜像记录不存在")
	}
	if old.Status != models.SyncStatusFailed {
		return "", fmt.Errorf("只有失败的镜像才能重试")
	}
	if old.AcrRegistryID == 0 {
		return "", fmt.Errorf("记录缺少 ACR 配置，无法重试")
	}
	batchID := uuid.New().String()
	err := s.db.Transaction(func(tx *gorm.DB) error {
		if err := tx.Create(&models.SyncBatch{ID: batchID, Description: fmt.Sprintf("重试: %s", old.OriginalInput)}).Error; err != nil {
			return err
		}
		return tx.Create(&models.SyncRecord{
			BatchID: batchID, AcrRegistryID: old.AcrRegistryID,
			OriginalImage: old.OriginalImage, Tag: old.Tag, Architecture: old.Architecture,
			OriginalInput: old.OriginalInput, InputOrder: 1, Status: models.SyncStatusPending,
			Description: old.Description,
		}).Error
	})
	return batchID, err
}

func (s *SyncBatchService) StartProcessing(batchID string) {
	if s.processor != nil {
		go s.processor(batchID)
	}
}

func UpsertWorkflowRun(batchID string, acrRegistryID uint, runID, runURL, commitSHA string) error {
	var existing models.SyncWorkflowRun
	err := database.DB.Where("batch_id = ? AND acr_registry_id = ?", batchID, acrRegistryID).First(&existing).Error
	if err == gorm.ErrRecordNotFound {
		return database.DB.Create(&models.SyncWorkflowRun{
			BatchID: batchID, AcrRegistryID: acrRegistryID,
			GitHubRunID: runID, GitHubActionURL: runURL, CommitSHA: commitSHA,
		}).Error
	}
	if err != nil {
		return err
	}
	return database.DB.Model(&existing).Updates(map[string]interface{}{
		"github_run_id": runID, "github_action_url": runURL, "commit_sha": commitSHA,
	}).Error
}

func EnrichRecordsWithWorkflowRuns(records []models.SyncRecord) {
	if len(records) == 0 {
		return
	}
	ids := make([]string, 0)
	seen := map[string]struct{}{}
	for _, r := range records {
		if _, ok := seen[r.BatchID]; !ok {
			seen[r.BatchID] = struct{}{}
			ids = append(ids, r.BatchID)
		}
	}
	var runs []models.SyncWorkflowRun
	database.DB.Where("batch_id IN ?", ids).Find(&runs)
	m := map[string]models.SyncWorkflowRun{}
	for _, run := range runs {
		m[fmt.Sprintf("%s:%d", run.BatchID, run.AcrRegistryID)] = run
	}
	for i := range records {
		if run, ok := m[fmt.Sprintf("%s:%d", records[i].BatchID, records[i].AcrRegistryID)]; ok {
			records[i].GitHubRunID = run.GitHubRunID
			records[i].GitHubRunURL = run.GitHubActionURL
		}
	}
}
