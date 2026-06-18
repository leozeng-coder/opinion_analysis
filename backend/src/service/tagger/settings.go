package tagger

import (
	"strconv"
	"strings"

	"gorm.io/gorm"

	"opinion-analysis/config"
	"opinion-analysis/src/model"
	"opinion-analysis/src/repository"
)

const settingPrefix = "tagger."

// SettingKeys 暴露所有受管理的 setting key。
var SettingKeys = []string{
	settingPrefix + "enabled",
	settingPrefix + "llm_api_key",
	settingPrefix + "llm_base_url",
	settingPrefix + "llm_model",
	settingPrefix + "interval_seconds",
	settingPrefix + "batch_size",
	settingPrefix + "max_per_tick",
	settingPrefix + "web_search_enabled",
	settingPrefix + "web_search_api_key",
	settingPrefix + "web_search_count",
}

// LoadConfig 用 system_settings 表里的覆盖值合并 base（来自 config.yaml 默认值）。
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

// SaveConfig 把 cfg 中的字段批量写回 system_settings 表，并记录完整配置快照。
func SaveConfig(db *gorm.DB, cfg config.TaggerConfig, updatedBy uint, updatedByName string) error {
	pairs := map[string]string{
		settingPrefix + "enabled":            boolToStr(cfg.Enabled),
		settingPrefix + "llm_api_key":        cfg.LLMApiKey,
		settingPrefix + "llm_base_url":       cfg.LLMBaseURL,
		settingPrefix + "llm_model":          cfg.LLMModel,
		settingPrefix + "interval_seconds":   strconv.Itoa(cfg.IntervalSeconds),
		settingPrefix + "batch_size":         strconv.Itoa(cfg.BatchSize),
		settingPrefix + "max_per_tick":       strconv.Itoa(cfg.MaxPerTick),
		settingPrefix + "web_search_enabled": boolToStr(cfg.WebSearchEnabled),
		settingPrefix + "web_search_api_key": cfg.WebSearchApiKey,
		settingPrefix + "web_search_count":   strconv.Itoa(cfg.WebSearchCount),
	}
	descs := map[string]string{
		settingPrefix + "enabled":            "AI 自动打标后台任务是否启用",
		settingPrefix + "llm_api_key":        "LLM API Key（敏感）",
		settingPrefix + "llm_base_url":       "LLM API Base URL（OpenAI 兼容）",
		settingPrefix + "llm_model":          "LLM 模型名",
		settingPrefix + "interval_seconds":   "轮询间隔（秒）",
		settingPrefix + "batch_size":         "单次 LLM 请求条数",
		settingPrefix + "max_per_tick":       "单次轮询最多处理条数",
		settingPrefix + "web_search_enabled": "联网搜索工具是否启用（深度思考模式）",
		settingPrefix + "web_search_api_key": "联网搜索 API Key（博查 Bocha，敏感）",
		settingPrefix + "web_search_count":   "联网搜索单次返回结果数",
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
		payloadJSON, err := repository.MarshalTaggerSnapshot(cfg)
		if err != nil {
			return err
		}
		return repository.CreateConfigSnapshotIfChangedTx(tx, "tagger", payloadJSON, updatedBy, updatedByName)
	})
}

// MigrateOldKeys 将旧的 deepseek_* key 迁移到新 llm_* key（幂等）。
func MigrateOldKeys(db *gorm.DB) {
	migrations := [][2]string{
		{"tagger.deepseek_api_key", "tagger.llm_api_key"},
		{"tagger.deepseek_base_url", "tagger.llm_base_url"},
		{"tagger.model", "tagger.llm_model"},
	}
	for _, m := range migrations {
		old, newKey := m[0], m[1]
		var oldRow model.SystemSetting
		if err := db.Where("`key` = ?", old).Limit(1).Find(&oldRow).Error; err != nil || oldRow.Key == "" {
			continue // 旧行不存在，跳过
		}
		var newRow model.SystemSetting
		if db.Where("`key` = ?", newKey).Limit(1).Find(&newRow).Error == nil && newRow.Key != "" {
			// 新 key 已存在，直接删除旧 key
			db.Where("`key` = ?", old).Delete(&model.SystemSetting{})
			continue
		}
		// 复制旧值到新 key
		db.Create(&model.SystemSetting{Key: newKey, Value: oldRow.Value, Desc: oldRow.Desc, UpdatedBy: oldRow.UpdatedBy})
		db.Where("`key` = ?", old).Delete(&model.SystemSetting{})
	}
}

func applySetting(cfg *config.TaggerConfig, key, val string) {
	switch key {
	case settingPrefix + "enabled":
		cfg.Enabled = parseBool(val)
	case settingPrefix + "llm_api_key":
		cfg.LLMApiKey = val
	case settingPrefix + "llm_base_url":
		if v := strings.TrimSpace(val); v != "" {
			cfg.LLMBaseURL = v
		}
	case settingPrefix + "llm_model":
		if v := strings.TrimSpace(val); v != "" {
			cfg.LLMModel = v
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
	case settingPrefix + "web_search_enabled":
		cfg.WebSearchEnabled = parseBool(val)
	case settingPrefix + "web_search_api_key":
		cfg.WebSearchApiKey = val
	case settingPrefix + "web_search_count":
		if n, err := strconv.Atoi(strings.TrimSpace(val)); err == nil && n > 0 {
			cfg.WebSearchCount = n
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

// ApplySettingValue 将 system_settings 键值写入 TaggerConfig。
func ApplySettingValue(cfg *config.TaggerConfig, key, val string) {
	applySetting(cfg, key, val)
}
