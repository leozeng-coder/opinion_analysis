package repository

import (
	"log"
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

func (r *AlertRepository) ListActiveRules() ([]model.AlertRule, error) {
	var list []model.AlertRule
	err := r.db.Where("status = ?", 1).Find(&list).Error
	log.Printf("[alert] ListActiveRules found %d rules with status=1", len(list))
	for i := range list {
		log.Printf("[alert]   rule %d: name=%s status=%d", list[i].ID, list[i].Name, list[i].Status)
	}
	return list, err
}

func (r *AlertRepository) CreateRule(rule *model.AlertRule) error {
	return r.db.Create(rule).Error
}

func (r *AlertRepository) FindRule(id string) (*model.AlertRule, error) {
	var rule model.AlertRule
	err := r.db.First(&rule, id).Error
	if err == gorm.ErrRecordNotFound {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	return &rule, nil
}

func (r *AlertRepository) UpdateRule(id string, fields map[string]interface{}) error {
	return r.db.Model(&model.AlertRule{}).Where("id = ?", id).Updates(fields).Error
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

func (r *AlertRepository) CountRecordsBetween(start, end time.Time) (int64, error) {
	var count int64
	err := r.db.Model(&model.AlertRecord{}).
		Where("created_at >= ? AND created_at < ?", start, end).
		Count(&count).Error
	return count, err
}

func (r *AlertRepository) CreateRecord(record *model.AlertRecord) error {
	return r.db.Create(record).Error
}

func (r *AlertRepository) ExistsByDedupKey(key string) (bool, error) {
	if key == "" {
		return false, nil
	}
	var count int64
	err := r.db.Model(&model.AlertRecord{}).Where("dedup_key = ?", key).Count(&count).Error
	return count > 0, err
}

func (r *AlertRepository) UpdateLastTriggered(ruleID uint, t time.Time) error {
	return r.db.Model(&model.AlertRule{}).Where("id = ?", ruleID).
		Update("last_triggered_at", t).Error
}

func (r *AlertRepository) FindRecord(id string) (*model.AlertRecord, error) {
	var record model.AlertRecord
	err := r.db.Preload("Rule").First(&record, id).Error
	if err == gorm.ErrRecordNotFound {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	return &record, nil
}

func (r *AlertRepository) MarkAsRead(id string) error {
	return r.db.Model(&model.AlertRecord{}).Where("id = ?", id).
		Update("status", "read").Error
}
