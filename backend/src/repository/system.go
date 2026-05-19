package repository

import (
	"strings"

	"gorm.io/gorm"
	"opinion-analysis/pkg/utils"
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

func isSensitiveSettingKey(key string) bool {
	k := strings.ToLower(key)
	return strings.Contains(k, "api_key") || strings.Contains(k, "password") || strings.Contains(k, "secret")
}

func maskSettingValue(key, val string) string {
	if isSensitiveSettingKey(key) {
		return utils.MaskString(val)
	}
	return val
}

// RecordSettingHistory 写入单条配置变更历史。
func (r *SystemRepository) RecordSettingHistory(key, oldVal, newVal string, updatedBy uint, updatedByName, source string) error {
	if oldVal == newVal {
		return nil
	}
	row := model.SystemSettingHistory{
		SettingKey:    key,
		OldValue:      oldVal,
		NewValue:      newVal,
		UpdatedBy:     updatedBy,
		UpdatedByName: updatedByName,
		Source:        source,
	}
	return r.db.Create(&row).Error
}

// ListSettingHistory 按 key 前缀分页查询配置变更历史。
func (r *SystemRepository) ListSettingHistory(prefix string, page, pageSize int) ([]model.SystemSettingHistory, int64, error) {
	if page < 1 {
		page = 1
	}
	if pageSize < 1 || pageSize > 100 {
		pageSize = 20
	}
	q := r.db.Model(&model.SystemSettingHistory{})
	if p := strings.TrimSpace(prefix); p != "" {
		q = q.Where("setting_key LIKE ?", p+"%")
	}
	var total int64
	if err := q.Count(&total).Error; err != nil {
		return nil, 0, err
	}
	var list []model.SystemSettingHistory
	err := q.Order("id DESC").Offset((page - 1) * pageSize).Limit(pageSize).Find(&list).Error
	for i := range list {
		list[i].OldValue = maskSettingValue(list[i].SettingKey, list[i].OldValue)
		list[i].NewValue = maskSettingValue(list[i].SettingKey, list[i].NewValue)
	}
	return list, total, err
}

// GetSettingHistoryByID 读取单条历史（不脱敏，供重新应用使用）。
func (r *SystemRepository) GetSettingHistoryByID(id uint) (*model.SystemSettingHistory, error) {
	var row model.SystemSettingHistory
	err := r.db.First(&row, id).Error
	if err != nil {
		return nil, err
	}
	return &row, nil
}

// DeleteSettingHistoryByID 删除单条配置变更历史。
func (r *SystemRepository) DeleteSettingHistoryByID(id uint) error {
	return r.db.Delete(&model.SystemSettingHistory{}, id).Error
}
