package repository

import (
	"time"

	"gorm.io/gorm"
	"opinion-analysis/src/model"
)

type AlertRepository struct {
	db *gorm.DB
}

func NewAlertRepository(db *gorm.DB) *AlertRepository {
	return &AlertRepository{db: db}
}

func (r *AlertRepository) ListRules() ([]model.AlertRule, error) {
	var list []model.AlertRule
	err := r.db.Find(&list).Error
	return list, err
}

func (r *AlertRepository) CreateRule(rule *model.AlertRule) error {
	return r.db.Create(rule).Error
}

func (r *AlertRepository) DeleteRule(id string) error {
	return r.db.Delete(&model.AlertRule{}, id).Error
}

func (r *AlertRepository) ListRecords(page, pageSize int, startAt *time.Time) ([]model.AlertRecord, int64, error) {
	q := r.db.Model(&model.AlertRecord{})
	if startAt != nil {
		q = q.Where("created_at >= ?", *startAt)
	}
	var total int64
	q.Count(&total)
	var list []model.AlertRecord
	offset := (page - 1) * pageSize
	err := q.Preload("Rule").Order("created_at desc").Offset(offset).Limit(pageSize).Find(&list).Error
	return list, total, err
}
