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

// DB 暴露内部 db 供需要复杂查询的 handler 使用。
func (r *RAGRepository) DB() *gorm.DB { return r.db }

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

// FindArticlesForSync 查未同步或内容有变更的文章（未同步的优先，其次按最新更新）。
func (r *RAGRepository) FindArticlesForSync(batchSize int) ([]model.Article, error) {
	if batchSize <= 0 {
		batchSize = 100
	}
	var articles []model.Article
	err := r.db.
		Where("deleted_at IS NULL").
		Order("embedding_synced_at IS NULL DESC, updated_at DESC").
		Limit(batchSize).
		Find(&articles).Error
	return articles, err
}

// FindDeletedArticleIDs 查软删文章的 ID（用于从 Milvus 清除 chunk）。
func (r *RAGRepository) FindDeletedArticleIDs() ([]int64, error) {
	var ids []int64
	err := r.db.Unscoped().
		Model(&model.Article{}).
		Where("deleted_at IS NOT NULL").
		Limit(400).
		Pluck("id", &ids).Error
	return ids, err
}

// UpdateEmbeddingSync 写入文章的向量同步标记。
func (r *RAGRepository) UpdateEmbeddingSync(id uint, hash string, syncedAt time.Time) error {
	return r.db.Model(&model.Article{}).
		Where("id = ?", id).
		Updates(map[string]any{
			"embedding_content_hash": hash,
			"embedding_synced_at":    syncedAt,
		}).Error
}

// ResetAllEmbeddingSync 清空所有文章的向量同步标记（rebuild 用）。
func (r *RAGRepository) ResetAllEmbeddingSync() (int64, error) {
	tx := r.db.Model(&model.Article{}).
		Where("deleted_at IS NULL").
		Updates(map[string]any{
			"embedding_content_hash": nil,
			"embedding_synced_at":    nil,
		})
	return tx.RowsAffected, tx.Error
}

// ResetArticleEmbeddingSync 清空单篇文章的向量同步标记。
func (r *RAGRepository) ResetArticleEmbeddingSync(id uint) error {
	return r.db.Model(&model.Article{}).
		Where("id = ?", id).
		Updates(map[string]any{
			"embedding_content_hash": nil,
			"embedding_synced_at":    nil,
		}).Error
}

// UpdateSyncLogProgress 更新同步日志进度（由 Syncer goroutine 调用）。
func (r *RAGRepository) UpdateSyncLogProgress(id uint, progress int, detail string, articles, chunksUp, chunksDel int) error {
	return r.db.Model(&model.RagSyncLog{}).
		Where("id = ?", id).
		Updates(map[string]any{
			"progress":           progress,
			"progress_detail":    detail,
			"articles_processed": articles,
			"chunks_upserted":    chunksUp,
			"chunks_deleted":     chunksDel,
		}).Error
}

// FinishSyncLog 标记同步日志完成或失败。
func (r *RAGRepository) FinishSyncLog(id uint, ok bool, message string, articles, chunksUp, chunksDel int) error {
	status := "success"
	if !ok {
		status = "failed"
	}
	now := time.Now()
	return r.db.Model(&model.RagSyncLog{}).
		Where("id = ?", id).
		Updates(map[string]any{
			"status":             status,
			"message":            message,
			"articles_processed": articles,
			"chunks_upserted":    chunksUp,
			"chunks_deleted":     chunksDel,
			"finished_at":        now,
			"progress":           100,
		}).Error
}

