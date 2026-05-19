package repository

import (
	"time"

	"gorm.io/gorm"
	"opinion-analysis/src/model"
)

type RAGRepository struct {
	db *gorm.DB
}

func NewRAGRepository(db *gorm.DB) *RAGRepository {
	return &RAGRepository{db: db}
}

func (r *RAGRepository) GetSyncEnabledSetting() (*model.SystemSetting, error) {
	var ss model.SystemSetting
	err := r.db.Where("`key` = ?", "rag.sync_enabled").Limit(1).Find(&ss).Error
	if err != nil {
		return nil, err
	}
	if ss.Key == "" {
		return nil, gorm.ErrRecordNotFound
	}
	return &ss, nil
}

func (r *RAGRepository) ListSyncLogs(page, pageSize int) ([]model.RagSyncLog, int64, error) {
	var total int64
	if err := r.db.Model(&model.RagSyncLog{}).Count(&total).Error; err != nil {
		return nil, 0, err
	}
	var logs []model.RagSyncLog
	offset := (page - 1) * pageSize
	err := r.db.Order("id DESC").Offset(offset).Limit(pageSize).Find(&logs).Error
	return logs, total, err
}

func (r *RAGRepository) CreateSyncLog(row *model.RagSyncLog) error {
	return r.db.Create(row).Error
}

func (r *RAGRepository) FailSyncLog(row *model.RagSyncLog, message string) error {
	now := time.Now()
	return r.db.Model(row).Updates(map[string]any{
		"status": "failed", "message": message, "finished_at": now,
	}).Error
}
