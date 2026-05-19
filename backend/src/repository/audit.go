package repository

import (
	"time"

	"gorm.io/gorm"
	"opinion-analysis/src/model"
)

type AuditRepository struct {
	db *gorm.DB
}

func NewAuditRepository(db *gorm.DB) *AuditRepository {
	return &AuditRepository{db: db}
}

type AuditListFilter struct {
	ActorName string
	Action    string
	Resource  string
	StartAt   *time.Time
	EndAt     *time.Time
	Page      int
	PageSize  int
}

func (r *AuditRepository) List(filter AuditListFilter) ([]model.AuditLog, int64, error) {
	q := r.db.Model(&model.AuditLog{})
	if filter.ActorName != "" {
		q = q.Where("actor_name LIKE ?", "%"+filter.ActorName+"%")
	}
	if filter.Action != "" {
		q = q.Where("action = ?", filter.Action)
	}
	if filter.Resource != "" {
		q = q.Where("resource = ?", filter.Resource)
	}
	if filter.StartAt != nil {
		q = q.Where("created_at >= ?", *filter.StartAt)
	}
	if filter.EndAt != nil {
		q = q.Where("created_at <= ?", *filter.EndAt)
	}
	var total int64
	if err := q.Count(&total).Error; err != nil {
		return nil, 0, err
	}
	var list []model.AuditLog
	err := q.Order("id desc").Offset((filter.Page - 1) * filter.PageSize).Limit(filter.PageSize).Find(&list).Error
	return list, total, err
}
