package tagger

import (
	"strconv"
	"strings"

	"gorm.io/gorm"

	"opinion-analysis/config"
	"opinion-analysis/internal/model"
)

// settings 表中 tagger 相关 key 的统一前缀。
const settingPrefix = "tagger."

// SettingKeys 暴露所有受管理的 setting key，前端/审计可用。
var SettingKeys = []string{
	settingPrefix + "enabled",
	settingPrefix + "deepseek_api_key",
	settingPrefix + "deepseek_base_url",
	settingPrefix + "model",
	settingPrefix + "interval_seconds",
	settingPrefix + "batch_size",
	settingPrefix + "max_per_tick",
}

// LoadConfig 用 system_settings 表里的覆盖值合并 base（来自 config.yaml）。
// 表中没有的 key 沿用 base 中的值。
func LoadConfig(db *gorm.DB, base config.TaggerConfig) config.TaggerConfig {
	out := base
	var rows []model.SystemSetting
	if err := db.Where("`key` LIKE ?", settingPrefix+"%").Find(&rows).Error; err != nil {
		return out
	}
	for _, r := range rows {
		applySetting(&out, r.Key, r.Value)
	}
	return out
}

// SaveConfig 把 cfg 中的字段批量写回 system_settings 表。
// updatedBy 用于审计字段。
func SaveConfig(db *gorm.DB, cfg config.TaggerConfig, updatedBy uint) error {
	pairs := map[string]string{
		settingPrefix + "enabled":           boolToStr(cfg.Enabled),
		settingPrefix + "deepseek_api_key":  cfg.DeepseekAPIKey,
		settingPrefix + "deepseek_base_url": cfg.DeepseekBaseURL,
		settingPrefix + "model":             cfg.Model,
		settingPrefix + "interval_seconds":  strconv.Itoa(cfg.IntervalSeconds),
		settingPrefix + "batch_size":        strconv.Itoa(cfg.BatchSize),
		settingPrefix + "max_per_tick":      strconv.Itoa(cfg.MaxPerTick),
	}
	descs := map[string]string{
		settingPrefix + "enabled":           "AI 自动打标后台任务是否启用",
		settingPrefix + "deepseek_api_key":  "DeepSeek API Key（敏感）",
		settingPrefix + "deepseek_base_url": "DeepSeek API base url",
		settingPrefix + "model":             "DeepSeek 模型名",
		settingPrefix + "interval_seconds":  "轮询间隔（秒）",
		settingPrefix + "batch_size":        "单次 LLM 请求条数",
		settingPrefix + "max_per_tick":      "单次轮询最多处理条数",
	}
	return db.Transaction(func(tx *gorm.DB) error {
		for k, v := range pairs {
			var existing model.SystemSetting
			err := tx.Where("`key` = ?", k).First(&existing).Error
			if err == gorm.ErrRecordNotFound {
				if err := tx.Create(&model.SystemSetting{
					Key: k, Value: v, Desc: descs[k], UpdatedBy: updatedBy,
				}).Error; err != nil {
					return err
				}
				continue
			}
			if err != nil {
				return err
			}
			if err := tx.Model(&model.SystemSetting{}).Where("`key` = ?", k).
				Updates(map[string]interface{}{
					"value":      v,
					"desc":       descs[k],
					"updated_by": updatedBy,
				}).Error; err != nil {
				return err
			}
		}
		return nil
	})
}

func applySetting(cfg *config.TaggerConfig, key, val string) {
	switch key {
	case settingPrefix + "enabled":
		cfg.Enabled = parseBool(val)
	case settingPrefix + "deepseek_api_key":
		cfg.DeepseekAPIKey = val
	case settingPrefix + "deepseek_base_url":
		if v := strings.TrimSpace(val); v != "" {
			cfg.DeepseekBaseURL = v
		}
	case settingPrefix + "model":
		if v := strings.TrimSpace(val); v != "" {
			cfg.Model = v
		}
	case settingPrefix + "interval_seconds":
		if n, err := strconv.Atoi(strings.TrimSpace(val)); err == nil && n > 0 {
			cfg.IntervalSeconds = n
		}
	case settingPrefix + "batch_size":
		if n, err := strconv.Atoi(strings.TrimSpace(val)); err == nil && n > 0 {
			cfg.BatchSize = n
		}
	case settingPrefix + "max_per_tick":
		if n, err := strconv.Atoi(strings.TrimSpace(val)); err == nil && n > 0 {
			cfg.MaxPerTick = n
		}
	}
}

func parseBool(s string) bool {
	v := strings.ToLower(strings.TrimSpace(s))
	return v == "true" || v == "1" || v == "yes" || v == "on"
}

func boolToStr(b bool) string {
	if b {
		return "true"
	}
	return "false"
}
