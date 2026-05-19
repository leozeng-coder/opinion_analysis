package repository

import (
	"time"

	"gorm.io/gorm"
	"opinion-analysis/src/model"
)

const staleRunMessage = "服务重启或进程异常中断，本条任务已与运行进程失联，已标记为失败。请重新发起抓取。"

type CrawlerRepository struct {
	db *gorm.DB
}

func NewCrawlerRepository(db *gorm.DB) *CrawlerRepository {
	return &CrawlerRepository{db: db}
}

func (r *CrawlerRepository) DB() *gorm.DB { return r.db }

func (r *CrawlerRepository) ListSpiderConfigs() ([]model.CrawlerSpiderConfig, error) {
	var list []model.CrawlerSpiderConfig
	err := r.db.Order("id").Find(&list).Error
	return list, err
}

func (r *CrawlerRepository) UpdateSpiderConfig(spiderKey string, intervalMinutes int, enabled int8) error {
	return r.db.Model(&model.CrawlerSpiderConfig{}).Where("spider_key = ?", spiderKey).Updates(map[string]interface{}{
		"interval_minutes": intervalMinutes,
		"enabled":          enabled,
	}).Error
}

func (r *CrawlerRepository) CreateRunLog(row *model.CrawlerRunLog) error {
	return r.db.Create(row).Error
}

func (r *CrawlerRepository) FindRunLogByID(id uint) (*model.CrawlerRunLog, error) {
	var row model.CrawlerRunLog
	err := r.db.First(&row, id).Error
	if err != nil {
		return nil, err
	}
	return &row, nil
}

func (r *CrawlerRepository) FinishRunLog(logID uint, finish map[string]interface{}) (int64, error) {
	tx := r.db.Model(&model.CrawlerRunLog{}).Where("id = ? AND status = ?", logID, "running").Updates(finish)
	return tx.RowsAffected, tx.Error
}

func (r *CrawlerRepository) ListRunLogs(page, pageSize int) ([]model.CrawlerRunLog, int64, error) {
	var total int64
	if err := r.db.Model(&model.CrawlerRunLog{}).Count(&total).Error; err != nil {
		return nil, 0, err
	}
	var list []model.CrawlerRunLog
	offset := (page - 1) * pageSize
	err := r.db.Order("id desc").Offset(offset).Limit(pageSize).Find(&list).Error
	return list, total, err
}

type LastCrawlerRun struct {
	ID         uint       `json:"id"`
	Spiders    string     `json:"spiders"`
	Status     string     `json:"status"`
	StartedAt  time.Time  `json:"startedAt"`
	FinishedAt *time.Time `json:"finishedAt"`
}

func (r *CrawlerRepository) LastRun() (LastCrawlerRun, error) {
	var last LastCrawlerRun
	err := r.db.Table("crawler_run_logs").Select("id, spiders, status, started_at, finished_at").
		Order("id desc").Limit(1).Scan(&last).Error
	return last, err
}

func (r *CrawlerRepository) RecoverStaleRuns() (int64, error) {
	now := time.Now()
	tx := r.db.Model(&model.CrawlerRunLog{}).
		Where("status = ? AND finished_at IS NULL", "running").
		Updates(map[string]interface{}{
			"status":          "failed",
			"message":         staleRunMessage,
			"finished_at":     &now,
			"progress":        100,
			"progress_detail": `{"phase":"failed","reason":"orphaned_run"}`,
		})
	return tx.RowsAffected, tx.Error
}

func (r *CrawlerRepository) DeleteSpiderConfigByKey(key string) error {
	return r.db.Where("spider_key = ?", key).Delete(&model.CrawlerSpiderConfig{}).Error
}

func (r *CrawlerRepository) CreateSpiderConfig(cfg *model.CrawlerSpiderConfig) error {
	return r.db.Create(cfg).Error
}

func (r *CrawlerRepository) FindSpiderByKey(key string) (*model.CrawlerSpiderConfig, error) {
	var existing model.CrawlerSpiderConfig
	err := r.db.Where("spider_key = ?", key).First(&existing).Error
	if err != nil {
		return nil, err
	}
	return &existing, nil
}
