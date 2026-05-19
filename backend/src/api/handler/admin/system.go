package admin

import (
	"context"
	"net/http"
	"strings"
	"time"

	"github.com/gin-gonic/gin"
	"gorm.io/gorm"
	"opinion-analysis/config"
	"opinion-analysis/pkg/response"
	"opinion-analysis/pkg/utils"
	"opinion-analysis/src/repository"
	"opinion-analysis/src/service/tagger"
)

type SystemHandler struct {
	db         *gorm.DB
	taggerSvc  *tagger.Service
	articles   *repository.ArticleRepository
	crawler    *repository.CrawlerRepository
}

func NewSystemHandler(db *gorm.DB, store *repository.Store, taggerSvc *tagger.Service) *SystemHandler {
	return &SystemHandler{
		db:        db,
		taggerSvc: taggerSvc,
		articles:  store.Article,
		crawler:   store.Crawler,
	}
}

// Config 返回当前生效的大模型（tagger）配置。
// apiKey 以脱敏形式返回，并附带 apiKeySet 标志：前端编辑表单时如果留空，后端会保留原有值。
func (h *SystemHandler) Config(c *gin.Context) {
	if h.taggerSvc == nil {
		response.ServerError(c)
		return
	}
	cfg, keySet := h.taggerSvc.GetConfig()
	out := gin.H{
		"tagger": gin.H{
			"enabled":         cfg.Enabled,
			"llmModel":        cfg.LLMModel,
			"llmBaseUrl":      cfg.LLMBaseURL,
			"llmApiKey":       utils.MaskString(cfg.LLMApiKey),
			"apiKeySet":       keySet,
			"intervalSeconds": cfg.IntervalSeconds,
			"batchSize":       cfg.BatchSize,
			"maxPerTick":      cfg.MaxPerTick,
		},
		"note": "修改后立即对后台 AI 自动打标任务生效；变更持久化到 system_settings 表，重启不丢失。",
	}
	response.OK(c, out)
}

// updateTaggerReq 接收前端表单。LLMApiKey 为指针：
//   - nil 或省略 → 保留旧值
//   - 空字符串    → 清空
//   - 非空        → 覆盖
type updateTaggerReq struct {
	Enabled         *bool   `json:"enabled"`
	LLMModel        *string `json:"llmModel"`
	LLMBaseURL      *string `json:"llmBaseUrl"`
	LLMApiKey       *string `json:"llmApiKey"`
	IntervalSeconds *int    `json:"intervalSeconds"`
	BatchSize       *int    `json:"batchSize"`
	MaxPerTick      *int    `json:"maxPerTick"`
}

// UpdateTagger 持久化 tagger 配置并热更新后台服务。
func (h *SystemHandler) UpdateTagger(c *gin.Context) {
	if h.taggerSvc == nil {
		response.ServerError(c)
		return
	}
	var req updateTaggerReq
	if err := c.ShouldBindJSON(&req); err != nil {
		response.Fail(c, 400, err.Error())
		return
	}
	cur, _ := h.taggerSvc.GetConfig()
	merged := cur
	if req.Enabled != nil {
		merged.Enabled = *req.Enabled
	}
	if req.LLMModel != nil {
		merged.LLMModel = strings.TrimSpace(*req.LLMModel)
	}
	if req.LLMBaseURL != nil {
		merged.LLMBaseURL = strings.TrimSpace(*req.LLMBaseURL)
	}
	if req.LLMApiKey != nil {
		merged.LLMApiKey = strings.TrimSpace(*req.LLMApiKey)
	}
	if req.IntervalSeconds != nil {
		if *req.IntervalSeconds < 10 {
			response.Fail(c, 400, "intervalSeconds 不能小于 10")
			return
		}
		merged.IntervalSeconds = *req.IntervalSeconds
	}
	if req.BatchSize != nil {
		if *req.BatchSize < 1 || *req.BatchSize > 100 {
			response.Fail(c, 400, "batchSize 应在 1~100 之间")
			return
		}
		merged.BatchSize = *req.BatchSize
	}
	if req.MaxPerTick != nil {
		if *req.MaxPerTick < 1 || *req.MaxPerTick > 10000 {
			response.Fail(c, 400, "maxPerTick 应在 1~10000 之间")
			return
		}
		merged.MaxPerTick = *req.MaxPerTick
	}

	uid, _ := c.Get("userID")
	cu, _ := uid.(uint)

	if err := tagger.SaveConfig(h.db, merged, cu); err != nil {
		response.Fail(c, 500, "持久化失败: "+err.Error())
		return
	}
	h.taggerSvc.UpdateConfig(merged)

	// 同步给 in-memory config.Cfg，让 Health 等只读探针看到最新值
	if config.Cfg != nil {
		config.Cfg.Tagger = merged
	}

	cfg, keySet := h.taggerSvc.GetConfig()
	response.OK(c, gin.H{
		"tagger": gin.H{
			"enabled":         cfg.Enabled,
			"llmModel":        cfg.LLMModel,
			"llmBaseUrl":      cfg.LLMBaseURL,
			"llmApiKey":       utils.MaskString(cfg.LLMApiKey),
			"apiKeySet":       keySet,
			"intervalSeconds": cfg.IntervalSeconds,
			"batchSize":       cfg.BatchSize,
			"maxPerTick":      cfg.MaxPerTick,
		},
	})
}

type healthProbe struct {
	OK      bool   `json:"ok"`
	Message string `json:"message,omitempty"`
	Latency int64  `json:"latencyMs"`
}

// Health 探测 DB / DeepSeek / 待打标条数 / 最近一次爬虫记录。
func (h *SystemHandler) Health(c *gin.Context) {
	out := gin.H{}

	// DB ping
	dbProbe := healthProbe{}
	t := time.Now()
	if sqlDB, err := h.db.DB(); err != nil {
		dbProbe.Message = err.Error()
	} else {
		ctx, cancel := context.WithTimeout(c.Request.Context(), 3*time.Second)
		defer cancel()
		if err := sqlDB.PingContext(ctx); err != nil {
			dbProbe.Message = err.Error()
		} else {
			dbProbe.OK = true
		}
	}
	dbProbe.Latency = time.Since(t).Milliseconds()
	out["database"] = dbProbe

	// LLM ping：读取 tagger 服务当前生效配置（含热更新后的 key）
	dsProbe := healthProbe{}
	t = time.Now()
	cfg, keySet := h.taggerSvc.GetConfig()
	dsURL := cfg.LLMBaseURL
	if dsURL == "" {
		dsURL = "https://api.deepseek.com"
	}
	if !keySet {
		dsProbe.Message = "no api key configured"
	} else {
		ctx, cancel := context.WithTimeout(c.Request.Context(), 4*time.Second)
		defer cancel()
		req, _ := http.NewRequestWithContext(ctx, http.MethodGet, dsURL+"/v1/models", nil)
		req.Header.Set("Authorization", "Bearer "+cfg.LLMApiKey)
		client := &http.Client{Timeout: 4 * time.Second}
		resp, err := client.Do(req)
		if err != nil {
			dsProbe.Message = err.Error()
		} else {
			defer resp.Body.Close()
			if resp.StatusCode >= 200 && resp.StatusCode < 300 {
				dsProbe.OK = true
			} else {
				dsProbe.Message = "http " + http.StatusText(resp.StatusCode)
			}
		}
	}
	dsProbe.Latency = time.Since(t).Milliseconds()
	out["llm"] = dsProbe

	// 业务指标
	pendingTag, _ := h.articles.CountPendingTagging()

	last, _ := h.crawler.LastRun()

	out["pendingTagging"] = pendingTag
	out["lastCrawlerRun"] = last
	out["timestamp"] = time.Now()

	response.OK(c, out)
}
