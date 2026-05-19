package repository

import (
	"strconv"
	"strings"

	"gorm.io/gorm"
	"opinion-analysis/pkg/utils"
	"opinion-analysis/src/model"
)

// RagConfigData RAG Embedding 运行时配置（来自 system_settings）。
type RagConfigData struct {
	SyncEnabled     bool
	EmbedProvider   string
	EmbedModel      string
	EmbedAPIBase    string
	EmbedAPIKey     string
	ChunkMaxChars   int
	ChunkOverlap    int
	SyncIntervalSec int
	SyncBatch       int
}

var ragConfigKeys = []string{
	"rag.sync_enabled",
	"rag.embed_provider",
	"rag.embed_model",
	"rag.embed_api_base",
	"rag.embed_api_key",
	"rag.chunk_max_chars",
	"rag.chunk_overlap",
	"rag.sync_interval_sec",
	"rag.sync_batch",
}

var ragConfigDescs = map[string]string{
	"rag.sync_enabled":     "RAG 向量同步定时任务是否启用",
	"rag.embed_provider":   "RAG 句向量来源：local=本地模型，api=OpenAI 兼容 API",
	"rag.embed_model":      "RAG 句向量模型名（本地 HuggingFace id 或 API model）",
	"rag.embed_api_base":   "RAG Embedding API Base URL（OpenAI 兼容）",
	"rag.embed_api_key":    "RAG Embedding API Key（敏感）",
	"rag.chunk_max_chars":  "RAG 切块最大字符数",
	"rag.chunk_overlap":    "RAG 切块重叠字符数",
	"rag.sync_interval_sec": "RAG 定时增量同步间隔（秒）",
	"rag.sync_batch":       "RAG 单次同步最多处理文章数",
}

func ragConfigDefaults() map[string]string {
	return map[string]string{
		"rag.sync_enabled":      "true",
		"rag.embed_provider":    "local",
		"rag.embed_model":       "paraphrase-multilingual-MiniLM-L12-v2",
		"rag.embed_api_base":    "",
		"rag.embed_api_key":     "",
		"rag.chunk_max_chars":   "420",
		"rag.chunk_overlap":     "72",
		"rag.sync_interval_sec": "120",
		"rag.sync_batch":        "100",
	}
}

func (r *RAGRepository) loadRagSettingMap() (map[string]string, error) {
	defaults := ragConfigDefaults()
	out := make(map[string]string, len(defaults))
	for k, v := range defaults {
		out[k] = v
	}
	var rows []model.SystemSetting
	if err := r.db.Where("`key` LIKE ?", "rag.%").Find(&rows).Error; err != nil {
		return nil, err
	}
	for _, row := range rows {
		out[row.Key] = row.Value
	}
	return out, nil
}

func parseBoolSetting(s string) bool {
	v := strings.ToLower(strings.TrimSpace(s))
	return v == "true" || v == "1" || v == "yes" || v == "on"
}

func parseIntSetting(s string, fallback int) int {
	n, err := strconv.Atoi(strings.TrimSpace(s))
	if err != nil {
		return fallback
	}
	return n
}

func mapToRagConfigData(m map[string]string) RagConfigData {
	d := ragConfigDefaults()
	for k, v := range m {
		d[k] = v
	}
	return RagConfigData{
		SyncEnabled:     parseBoolSetting(d["rag.sync_enabled"]),
		EmbedProvider:   strings.TrimSpace(d["rag.embed_provider"]),
		EmbedModel:      strings.TrimSpace(d["rag.embed_model"]),
		EmbedAPIBase:    strings.TrimSpace(d["rag.embed_api_base"]),
		EmbedAPIKey:     d["rag.embed_api_key"],
		ChunkMaxChars:   parseIntSetting(d["rag.chunk_max_chars"], 420),
		ChunkOverlap:    parseIntSetting(d["rag.chunk_overlap"], 72),
		SyncIntervalSec: parseIntSetting(d["rag.sync_interval_sec"], 120),
		SyncBatch:       parseIntSetting(d["rag.sync_batch"], 100),
	}
}

// GetRagConfig 从 system_settings 读取 RAG 配置。
func (r *RAGRepository) GetRagConfig() (RagConfigData, error) {
	m, err := r.loadRagSettingMap()
	if err != nil {
		return RagConfigData{}, err
	}
	return mapToRagConfigData(m), nil
}

// SaveRagConfig 写入 RAG 配置并记录完整配置快照。
func (r *RAGRepository) SaveRagConfig(
	cfg RagConfigData,
	updatedBy uint,
	updatedByName string,
) error {
	pairs := map[string]string{
		"rag.sync_enabled":      boolToSetting(cfg.SyncEnabled),
		"rag.embed_provider":    strings.TrimSpace(cfg.EmbedProvider),
		"rag.embed_model":       strings.TrimSpace(cfg.EmbedModel),
		"rag.embed_api_base":    strings.TrimSpace(cfg.EmbedAPIBase),
		"rag.embed_api_key":     cfg.EmbedAPIKey,
		"rag.chunk_max_chars":   strconv.Itoa(cfg.ChunkMaxChars),
		"rag.chunk_overlap":     strconv.Itoa(cfg.ChunkOverlap),
		"rag.sync_interval_sec": strconv.Itoa(cfg.SyncIntervalSec),
		"rag.sync_batch":        strconv.Itoa(cfg.SyncBatch),
	}
	return r.db.Transaction(func(tx *gorm.DB) error {
		for _, key := range ragConfigKeys {
			newVal := pairs[key]
			var existing model.SystemSetting
			err := tx.Where("`key` = ?", key).First(&existing).Error
			if err == gorm.ErrRecordNotFound {
				if err := tx.Create(&model.SystemSetting{
					Key: key, Value: newVal, Desc: ragConfigDescs[key], UpdatedBy: updatedBy,
				}).Error; err != nil {
					return err
				}
				continue
			}
			if err != nil {
				return err
			}
			if existing.Value != newVal {
				if err := tx.Model(&model.SystemSetting{}).Where("`key` = ?", key).
					Updates(map[string]any{
						"value": newVal, "desc": ragConfigDescs[key], "updated_by": updatedBy,
					}).Error; err != nil {
					return err
				}
			}
		}
		payloadJSON, err := MarshalRagSnapshot(cfg)
		if err != nil {
			return err
		}
		sys := &SystemRepository{db: tx}
		return sys.createConfigSnapshotIfChangedTx("rag", payloadJSON, updatedBy, updatedByName)
	})
}

func boolToSetting(b bool) string {
	if b {
		return "true"
	}
	return "false"
}

// RagConfigResponse 管理端 API 响应。
type RagConfigResponse struct {
	SyncEnabled     bool     `json:"sync_enabled"`
	EmbedProvider   string   `json:"embed_provider"`
	EmbedModel      string   `json:"embed_model"`
	EmbedAPIBase    string   `json:"embed_api_base"`
	EmbedAPIKey     string   `json:"embed_api_key"`
	APIKeySet       bool     `json:"api_key_set"`
	ChunkMaxChars   int      `json:"chunk_max_chars"`
	ChunkOverlap    int      `json:"chunk_overlap"`
	SyncIntervalSec int      `json:"sync_interval_sec"`
	SyncBatch       int      `json:"sync_batch"`
	EnvOverrides    []string `json:"env_overrides"`
	Note            string   `json:"note"`
	OK              bool     `json:"ok"`
	ServiceApplied  bool     `json:"service_applied,omitempty"`
	Warning         string   `json:"warning,omitempty"`
	Warnings        []string `json:"warnings,omitempty"`
}

func BuildRagConfigResponse(cfg RagConfigData, envOverrides []string, serviceApplied bool, warning string) RagConfigResponse {
	if cfg.EmbedProvider == "" {
		cfg.EmbedProvider = "local"
	}
	keySet := strings.TrimSpace(cfg.EmbedAPIKey) != ""
	note := "配置持久化在 system_settings；RAG 服务重启或可达时会自动加载。"
	if len(envOverrides) > 0 {
		note = "带 env_overrides 的项由 RAG 进程环境变量锁定；其余项已写入数据库。"
	}
	if warning != "" {
		note = warning + " " + note
	}
	return RagConfigResponse{
		SyncEnabled:     cfg.SyncEnabled,
		EmbedProvider:   cfg.EmbedProvider,
		EmbedModel:      cfg.EmbedModel,
		EmbedAPIBase:    cfg.EmbedAPIBase,
		EmbedAPIKey:     utils.MaskString(cfg.EmbedAPIKey),
		APIKeySet:       keySet,
		ChunkMaxChars:   cfg.ChunkMaxChars,
		ChunkOverlap:    cfg.ChunkOverlap,
		SyncIntervalSec: cfg.SyncIntervalSec,
		SyncBatch:       cfg.SyncBatch,
		EnvOverrides:    envOverrides,
		Note:            note,
		OK:              true,
		ServiceApplied:  serviceApplied,
		Warning:         warning,
	}
}
