package repository

import (
	"encoding/json"
	"strings"

	"gorm.io/gorm"
	"opinion-analysis/config"
	"opinion-analysis/pkg/utils"
	"opinion-analysis/src/model"
)

// RagSnapshotPayload RAG 配置快照 JSON 结构。
type RagSnapshotPayload struct {
	SyncEnabled     bool   `json:"sync_enabled"`
	EmbedProvider   string `json:"embed_provider"`
	EmbedModel      string `json:"embed_model"`
	EmbedAPIBase    string `json:"embed_api_base"`
	EmbedAPIKey     string `json:"embed_api_key"`
	ChunkMaxChars   int    `json:"chunk_max_chars"`
	ChunkOverlap    int    `json:"chunk_overlap"`
	SyncIntervalSec int    `json:"sync_interval_sec"`
	SyncBatch       int    `json:"sync_batch"`
}

// TaggerSnapshotPayload Tagger 配置快照 JSON 结构。
type TaggerSnapshotPayload struct {
	Enabled         bool   `json:"enabled"`
	LLMModel        string `json:"llm_model"`
	LLMBaseURL      string `json:"llm_base_url"`
	LLMApiKey       string `json:"llm_api_key"`
	IntervalSeconds int    `json:"interval_seconds"`
	BatchSize       int    `json:"batch_size"`
	MaxPerTick      int    `json:"max_per_tick"`

	WebSearchEnabled bool   `json:"web_search_enabled"`
	WebSearchApiKey  string `json:"web_search_api_key"`
	WebSearchCount   int    `json:"web_search_count"`
}

func RagConfigToSnapshotPayload(cfg RagConfigData) RagSnapshotPayload {
	p := strings.TrimSpace(cfg.EmbedProvider)
	if p == "" {
		p = "local"
	}
	return RagSnapshotPayload{
		SyncEnabled:     cfg.SyncEnabled,
		EmbedProvider:   p,
		EmbedModel:      strings.TrimSpace(cfg.EmbedModel),
		EmbedAPIBase:    strings.TrimSpace(cfg.EmbedAPIBase),
		EmbedAPIKey:     cfg.EmbedAPIKey,
		ChunkMaxChars:   cfg.ChunkMaxChars,
		ChunkOverlap:    cfg.ChunkOverlap,
		SyncIntervalSec: cfg.SyncIntervalSec,
		SyncBatch:       cfg.SyncBatch,
	}
}

func (p RagSnapshotPayload) ToRagConfigData() RagConfigData {
	return RagConfigData{
		SyncEnabled:     p.SyncEnabled,
		EmbedProvider:   p.EmbedProvider,
		EmbedModel:      p.EmbedModel,
		EmbedAPIBase:    p.EmbedAPIBase,
		EmbedAPIKey:     p.EmbedAPIKey,
		ChunkMaxChars:   p.ChunkMaxChars,
		ChunkOverlap:    p.ChunkOverlap,
		SyncIntervalSec: p.SyncIntervalSec,
		SyncBatch:       p.SyncBatch,
	}
}

func TaggerConfigToSnapshotPayload(cfg config.TaggerConfig) TaggerSnapshotPayload {
	return TaggerSnapshotPayload{
		Enabled:         cfg.Enabled,
		LLMModel:        cfg.LLMModel,
		LLMBaseURL:      cfg.LLMBaseURL,
		LLMApiKey:       cfg.LLMApiKey,
		IntervalSeconds: cfg.IntervalSeconds,
		BatchSize:       cfg.BatchSize,
		MaxPerTick:      cfg.MaxPerTick,

		WebSearchEnabled: cfg.WebSearchEnabled,
		WebSearchApiKey:  cfg.WebSearchApiKey,
		WebSearchCount:   cfg.WebSearchCount,
	}
}

func (p TaggerSnapshotPayload) ToTaggerConfig() config.TaggerConfig {
	return config.TaggerConfig{
		Enabled:         p.Enabled,
		LLMModel:        p.LLMModel,
		LLMBaseURL:      p.LLMBaseURL,
		LLMApiKey:       p.LLMApiKey,
		IntervalSeconds: p.IntervalSeconds,
		BatchSize:       p.BatchSize,
		MaxPerTick:      p.MaxPerTick,

		WebSearchEnabled: p.WebSearchEnabled,
		WebSearchApiKey:  p.WebSearchApiKey,
		WebSearchCount:   p.WebSearchCount,
	}
}

func maskRagSnapshot(p RagSnapshotPayload) RagSnapshotPayload {
	out := p
	out.EmbedAPIKey = utils.MaskString(p.EmbedAPIKey)
	return out
}

func maskTaggerSnapshot(p TaggerSnapshotPayload) TaggerSnapshotPayload {
	out := p
	out.LLMApiKey = utils.MaskString(p.LLMApiKey)
	out.WebSearchApiKey = utils.MaskString(p.WebSearchApiKey)
	return out
}

// ConfigSnapshotItem API 列表项。
type ConfigSnapshotItem struct {
	ID            uint        `json:"id"`
	Domain        string      `json:"domain"`
	Config        interface{} `json:"config"`
	UpdatedBy     uint        `json:"updatedBy"`
	UpdatedByName string      `json:"updatedByName"`
	CreatedAt     string      `json:"createdAt"`
}

func (r *SystemRepository) CreateConfigSnapshotIfChanged(
	domain, payloadJSON string,
	updatedBy uint,
	updatedByName string,
) error {
	return CreateConfigSnapshotIfChangedTx(r.db, domain, payloadJSON, updatedBy, updatedByName)
}

// CreateConfigSnapshotIfChangedTx 与最近一条快照相同时跳过写入。
func CreateConfigSnapshotIfChangedTx(
	tx *gorm.DB,
	domain, payloadJSON string,
	updatedBy uint,
	updatedByName string,
) error {
	domain = strings.TrimSpace(domain)
	var last model.ConfigSnapshot
	err := tx.Where("domain = ?", domain).Order("id DESC").Limit(1).Find(&last).Error
	if err != nil {
		return err
	}
	if last.ID > 0 && last.Payload == payloadJSON {
		return nil
	}
	return tx.Create(&model.ConfigSnapshot{
		Domain:        domain,
		Payload:       payloadJSON,
		UpdatedBy:     updatedBy,
		UpdatedByName: updatedByName,
	}).Error
}

func (r *SystemRepository) createConfigSnapshotIfChangedTx(
	domain, payloadJSON string,
	updatedBy uint,
	updatedByName string,
) error {
	return CreateConfigSnapshotIfChangedTx(r.db, domain, payloadJSON, updatedBy, updatedByName)
}

func (r *SystemRepository) ListConfigSnapshots(domain string, page, pageSize int) ([]ConfigSnapshotItem, int64, error) {
	if page < 1 {
		page = 1
	}
	if pageSize < 1 || pageSize > 100 {
		pageSize = 20
	}
	q := r.db.Model(&model.ConfigSnapshot{})
	if d := strings.TrimSpace(domain); d != "" {
		q = q.Where("domain = ?", d)
	}
	var total int64
	if err := q.Count(&total).Error; err != nil {
		return nil, 0, err
	}
	var rows []model.ConfigSnapshot
	if err := q.Order("id DESC").Offset((page - 1) * pageSize).Limit(pageSize).Find(&rows).Error; err != nil {
		return nil, 0, err
	}
	out := make([]ConfigSnapshotItem, 0, len(rows))
	for _, row := range rows {
		item, err := snapshotRowToItem(row)
		if err != nil {
			continue
		}
		out = append(out, item)
	}
	return out, total, nil
}

func snapshotRowToItem(row model.ConfigSnapshot) (ConfigSnapshotItem, error) {
	cfg, err := parseSnapshotPayload(row.Domain, row.Payload)
	if err != nil {
		return ConfigSnapshotItem{}, err
	}
	return ConfigSnapshotItem{
		ID:            row.ID,
		Domain:        row.Domain,
		Config:        cfg,
		UpdatedBy:     row.UpdatedBy,
		UpdatedByName: row.UpdatedByName,
		CreatedAt:     row.CreatedAt.UTC().Format("2006-01-02T15:04:05Z"),
	}, nil
}

func parseSnapshotPayload(domain, payload string) (interface{}, error) {
	switch strings.TrimSpace(domain) {
	case "rag":
		var p RagSnapshotPayload
		if err := json.Unmarshal([]byte(payload), &p); err != nil {
			return nil, err
		}
		return maskRagSnapshot(p), nil
	case "tagger":
		var p TaggerSnapshotPayload
		if err := json.Unmarshal([]byte(payload), &p); err != nil {
			return nil, err
		}
		return maskTaggerSnapshot(p), nil
	default:
		var raw map[string]interface{}
		if err := json.Unmarshal([]byte(payload), &raw); err != nil {
			return nil, err
		}
		return raw, nil
	}
}

func (r *SystemRepository) GetConfigSnapshotByID(id uint) (*model.ConfigSnapshot, error) {
	var row model.ConfigSnapshot
	err := r.db.First(&row, id).Error
	if err != nil {
		return nil, err
	}
	return &row, nil
}

func (r *SystemRepository) DeleteConfigSnapshot(id uint) error {
	return r.db.Delete(&model.ConfigSnapshot{}, id).Error
}

func ParseRagSnapshotPayload(payload string) (RagSnapshotPayload, error) {
	var p RagSnapshotPayload
	err := json.Unmarshal([]byte(payload), &p)
	return p, err
}

func ParseTaggerSnapshotPayload(payload string) (TaggerSnapshotPayload, error) {
	var p TaggerSnapshotPayload
	err := json.Unmarshal([]byte(payload), &p)
	return p, err
}

func MarshalRagSnapshot(cfg RagConfigData) (string, error) {
	b, err := json.Marshal(RagConfigToSnapshotPayload(cfg))
	return string(b), err
}

func MarshalTaggerSnapshot(cfg config.TaggerConfig) (string, error) {
	b, err := json.Marshal(TaggerConfigToSnapshotPayload(cfg))
	return string(b), err
}
