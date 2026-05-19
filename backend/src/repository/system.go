package repository

import (
	"strings"

	"gorm.io/gorm"
	"opinion-analysis/src/model"
)

type SystemRepository struct {
	db *gorm.DB
}

func NewSystemRepository(db *gorm.DB) *SystemRepository {
	return &SystemRepository{db: db}
}

func (r *SystemRepository) ListSettings() ([]model.SystemSetting, error) {
	var list []model.SystemSetting
	err := r.db.Order("`key`").Find(&list).Error
	return list, err
}

func (r *SystemRepository) GetByKey(key string) (*model.SystemSetting, error) {
	var s model.SystemSetting
	err := r.db.Where("`key` = ?", key).First(&s).Error
	if err != nil {
		return nil, err
	}
	return &s, nil
}

func (r *SystemRepository) UpsertSetting(key, value string, updatedBy uint) (*model.SystemSetting, error) {
	var existing model.SystemSetting
	err := r.db.Where("`key` = ?", key).First(&existing).Error
	if IsNotFound(err) {
		s := model.SystemSetting{Key: key, Value: value, UpdatedBy: updatedBy}
		if err := r.db.Create(&s).Error; err != nil {
			return nil, err
		}
		return &s, nil
	}
	if err != nil {
		return nil, err
	}
	if err := r.db.Model(&model.SystemSetting{}).Where("`key` = ?", key).
		Updates(map[string]interface{}{"value": value, "updated_by": updatedBy}).Error; err != nil {
		return nil, err
	}
	return r.GetByKey(key)
}

func (r *SystemRepository) RegistrationEnabled() bool {
	s, err := r.GetByKey("registration_enabled")
	if err != nil {
		return true
	}
	v := strings.ToLower(strings.TrimSpace(s.Value))
	return v == "true" || v == "1" || v == "yes" || v == "on"
}
