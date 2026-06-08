package admin

import (
	"context"
	"fmt"
	"net/http"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/redis/go-redis/v9"
	"gorm.io/gorm"
	"opinion-analysis/config"
	"opinion-analysis/pkg/response"
	"opinion-analysis/pkg/utils"
	"opinion-analysis/src/repository"
	"opinion-analysis/src/service/alertengine"
	"opinion-analysis/src/service/tagger"
)

type SystemHandler struct {
	db        *gorm.DB
	rdb       *redis.Client
	taggerSvc *tagger.Service
	articles  *repository.ArticleRepository
	crawler   *repository.CrawlerRepository
	system    *repository.SystemRepository
	rag       *repository.RAGRepository
}

func NewSystemHandler(db *gorm.DB, store *repository.Store, taggerSvc *tagger.Service, rdb *redis.Client) *SystemHandler {
	return &SystemHandler{
		db:        db,
		rdb:       rdb,
		taggerSvc: taggerSvc,
		articles:  store.Article,
		crawler:   store.Crawler,
		system:    store.System,
		rag:       store.RAG,
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
	uname, _ := c.Get("username")
	actorName, _ := uname.(string)

	if err := tagger.SaveConfig(h.db, merged, cu, actorName); err != nil {
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

type platformEntry struct {
	code       string
	table      string
	artPlatform string
}

// platformOrder 固定顺序，确保 health 接口返回的 platformDiffs 顺序稳定
var platformOrder = []platformEntry{
	{"xhs",   "xhs_note",       "xhs"},
	{"dy",    "douyin_aweme",   "douyin"},
	{"ks",    "kuaishou_video", "kuaishou"},
	{"bili",  "bilibili_video", "bilibili"},
	{"wb",    "weibo_note",     "weibo"},
	{"tieba", "tieba_note",     "tieba"},
	{"zhihu", "zhihu_content",  "zhihu"},
}

// Health 并发探测 MySQL / Redis / LLM，并返回待处理指标和平台数据差异。
func (h *SystemHandler) Health(c *gin.Context) {
	var (
		dbProbe    healthProbe
		redisProbe healthProbe
		llmProbe   healthProbe
	)

	var wg sync.WaitGroup
	wg.Add(3)

	// MySQL
	go func() {
		defer wg.Done()
		t := time.Now()
		if sqlDB, err := h.db.DB(); err != nil {
			dbProbe.Message = err.Error()
		} else {
			ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
			defer cancel()
			if err := sqlDB.PingContext(ctx); err != nil {
				dbProbe.Message = err.Error()
			} else {
				dbProbe.OK = true
			}
		}
		dbProbe.Latency = time.Since(t).Milliseconds()
	}()

	// Redis
	go func() {
		defer wg.Done()
		t := time.Now()
		if h.rdb == nil {
			redisProbe.Message = "未配置"
		} else {
			ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
			defer cancel()
			if err := h.rdb.Ping(ctx).Err(); err != nil {
				redisProbe.Message = err.Error()
			} else {
				redisProbe.OK = true
			}
		}
		redisProbe.Latency = time.Since(t).Milliseconds()
	}()

	// LLM
	go func() {
		defer wg.Done()
		t := time.Now()
		cfg, keySet := h.taggerSvc.GetConfig()
		dsURL := cfg.LLMBaseURL
		if dsURL == "" {
			dsURL = "https://api.deepseek.com"
		}
		if !keySet {
			llmProbe.Message = "no api key configured"
		} else {
			ctx, cancel := context.WithTimeout(context.Background(), 4*time.Second)
			defer cancel()
			req, _ := http.NewRequestWithContext(ctx, http.MethodGet, dsURL+"/v1/models", nil)
			req.Header.Set("Authorization", "Bearer "+cfg.LLMApiKey)
			client := &http.Client{Timeout: 4 * time.Second}
			resp, err := client.Do(req)
			if err != nil {
				llmProbe.Message = err.Error()
			} else {
				defer resp.Body.Close()
				if resp.StatusCode >= 200 && resp.StatusCode < 300 {
					llmProbe.OK = true
				} else {
					llmProbe.Message = "http " + http.StatusText(resp.StatusCode)
				}
			}
		}
		llmProbe.Latency = time.Since(t).Milliseconds()
	}()

	wg.Wait()

	pendingTag, _ := h.articles.CountPendingTagging()
	last, _ := h.crawler.LastRun()

	// 待向量化（embedding_synced_at 为空的文章）
	var pendingEmbed int64
	h.db.Table("articles").Where("embedding_synced_at IS NULL AND deleted_at IS NULL").Count(&pendingEmbed)

	// 总文章数
	var totalArticles int64
	h.db.Table("articles").Where("deleted_at IS NULL").Count(&totalArticles)

	// 各平台表行数 vs 中心表行数
	type platformDiff struct {
		Code      string `json:"code"`
		TableName string `json:"table"`
		Source    int64  `json:"source"`
		Central   int64  `json:"central"`
		Diff      int64  `json:"diff"`
	}
	var diffs []platformDiff
	for _, p := range platformOrder {
		var srcCount int64
		if err := h.db.Raw("SELECT COUNT(*) FROM `"+p.table+"`").Scan(&srcCount).Error; err != nil {
			srcCount = -1
		}
		var centCount int64
		h.db.Table("articles").Where("platform = ? AND deleted_at IS NULL", p.artPlatform).Count(&centCount)
		diffs = append(diffs, platformDiff{
			Code:      p.code,
			TableName: p.table,
			Source:    srcCount,
			Central:   centCount,
			Diff:      srcCount - centCount,
		})
	}

	response.OK(c, gin.H{
		"database":       dbProbe,
		"redis":          redisProbe,
		"llm":            llmProbe,
		"pendingTagging": pendingTag,
		"pendingEmbed":   pendingEmbed,
		"totalArticles":  totalArticles,
		"platformDiffs":  diffs,
		"lastCrawlerRun": last,
		"timestamp":      time.Now(),
	})
}

// ListSettingHistory GET /api/admin/system/settings/history — 配置快照列表（domain=rag|tagger）。
func (h *SystemHandler) ListSettingHistory(c *gin.Context) {
	domain := strings.TrimSpace(c.Query("domain"))
	if domain == "" {
		prefix := strings.TrimSpace(c.Query("prefix"))
		switch {
		case strings.HasPrefix(prefix, "rag"):
			domain = "rag"
		case strings.HasPrefix(prefix, "tagger"):
			domain = "tagger"
		}
	}
	if domain != "rag" && domain != "tagger" {
		response.Fail(c, 400, "domain 必须是 rag 或 tagger")
		return
	}
	page, _ := strconv.Atoi(c.DefaultQuery("page", "1"))
	pageSize, _ := strconv.Atoi(c.DefaultQuery("pageSize", "20"))
	list, total, err := h.system.ListConfigSnapshots(domain, page, pageSize)
	if err != nil {
		response.Fail(c, 500, err.Error())
		return
	}
	response.OK(c, gin.H{
		"list":  list,
		"total": total,
		"page":  page,
	})
}

// DeleteSettingHistory DELETE /api/admin/system/settings/history/:id
func (h *SystemHandler) DeleteSettingHistory(c *gin.Context) {
	id, err := strconv.ParseUint(c.Param("id"), 10, 64)
	if err != nil || id == 0 {
		response.Fail(c, 400, "invalid id")
		return
	}
	if _, err := h.system.GetConfigSnapshotByID(uint(id)); err != nil {
		if repository.IsNotFound(err) {
			response.Fail(c, 404, "历史记录不存在")
			return
		}
		response.Fail(c, 500, err.Error())
		return
	}
	if err := h.system.DeleteConfigSnapshot(uint(id)); err != nil {
		response.Fail(c, 500, err.Error())
		return
	}
	response.OK(c, gin.H{"ok": true, "message": "已删除"})
}

// ReapplySettingHistory POST /api/admin/system/settings/history/:id/reapply — 应用整条配置快照。
func (h *SystemHandler) ReapplySettingHistory(c *gin.Context) {
	id, err := strconv.ParseUint(c.Param("id"), 10, 64)
	if err != nil || id == 0 {
		response.Fail(c, 400, "invalid id")
		return
	}
	row, err := h.system.GetConfigSnapshotByID(uint(id))
	if err != nil {
		if repository.IsNotFound(err) {
			response.Fail(c, 404, "历史记录不存在")
			return
		}
		response.Fail(c, 500, err.Error())
		return
	}

	var cu uint
	if uid, ok := c.Get("userID"); ok {
		if idu, ok := uid.(uint); ok {
			cu = idu
		}
	}
	actorName := ""
	if uname, ok := c.Get("username"); ok {
		if name, ok := uname.(string); ok {
			actorName = name
		}
	}

	var warn string
	var pyWarnings []string

	switch row.Domain {
	case "rag":
		payload, err := repository.ParseRagSnapshotPayload(row.Payload)
		if err != nil {
			response.Fail(c, 400, "快照数据无效")
			return
		}
		if err := h.rag.SaveRagConfig(payload.ToRagConfigData(), cu, actorName); err != nil {
			response.Fail(c, 500, "应用失败: "+err.Error())
			return
		}
		warn, pyWarnings = "", nil
	case "tagger":
		if h.taggerSvc == nil {
			response.Fail(c, 500, "tagger 服务不可用")
			return
		}
		payload, err := repository.ParseTaggerSnapshotPayload(row.Payload)
		if err != nil {
			response.Fail(c, 400, "快照数据无效")
			return
		}
		cfg := payload.ToTaggerConfig()
		if err := tagger.SaveConfig(h.db, cfg, cu, actorName); err != nil {
			response.Fail(c, 500, "持久化失败: "+err.Error())
			return
		}
		h.taggerSvc.UpdateConfig(cfg)
		if config.Cfg != nil {
			config.Cfg.Tagger = cfg
		}
	default:
		response.Fail(c, 400, fmt.Sprintf("不支持的配置域: %s", row.Domain))
		return
	}

	if len(pyWarnings) > 0 {
		warn = strings.Join(pyWarnings, "；")
	}
	msg := "已应用该条历史配置"
	if warn != "" {
		msg += "；" + warn
	}
	response.OK(c, gin.H{
		"ok":      true,
		"message": msg,
		"domain":  row.Domain,
		"warning": warn,
	})
}

type updateSmtpReq struct {
	Host     *string `json:"host"`
	Port     *int    `json:"port"`
	Username *string `json:"username"`
	Password *string `json:"password"`
	From     *string `json:"from"`
	UseTLS   *bool   `json:"useTls"`
	OnCrawl  *bool   `json:"onCrawl"`
}

type testSmtpReq struct {
	To string `json:"to" binding:"required,email"`
}

// GetSmtp 返回 SMTP 配置（密码脱敏）。
func (h *SystemHandler) GetSmtp(c *gin.Context) {
	cfg, err := h.system.GetSmtpConfig()
	if err != nil {
		response.ServerError(c)
		return
	}
	alertCfg := h.system.GetAlertConfig()
	response.OK(c, gin.H{
		"host":        cfg.Host,
		"port":        cfg.Port,
		"username":    cfg.Username,
		"from":        cfg.From,
		"useTls":      cfg.UseTLS,
		"passwordSet": cfg.Password != "",
		"password":    utils.MaskString(cfg.Password),
		"onCrawl":     alertCfg.OnCrawl,
	})
}

// UpdateSmtp 保存 SMTP 与告警全局配置。
func (h *SystemHandler) UpdateSmtp(c *gin.Context) {
	var req updateSmtpReq
	if err := c.ShouldBindJSON(&req); err != nil {
		response.Fail(c, 400, err.Error())
		return
	}
	cur, err := h.system.GetSmtpConfig()
	if err != nil {
		response.ServerError(c)
		return
	}
	merged := cur
	if req.Host != nil {
		merged.Host = strings.TrimSpace(*req.Host)
	}
	if req.Port != nil && *req.Port > 0 {
		merged.Port = *req.Port
	}
	if req.Username != nil {
		merged.Username = strings.TrimSpace(*req.Username)
	}
	if req.Password != nil && strings.TrimSpace(*req.Password) != "" {
		merged.Password = strings.TrimSpace(*req.Password)
	}
	if req.From != nil {
		merged.From = strings.TrimSpace(*req.From)
	}
	if req.UseTLS != nil {
		merged.UseTLS = *req.UseTLS
	}
	uid := uint(0)
	if v, ok := c.Get("userID"); ok {
		if id, ok2 := v.(uint); ok2 {
			uid = id
		}
	}
	if err := h.system.SaveSmtpConfig(merged, uid); err != nil {
		response.ServerError(c)
		return
	}
	if req.OnCrawl != nil {
		val := "false"
		if *req.OnCrawl {
			val = "true"
		}
		if _, err := h.system.UpsertSetting("alert.on_crawl", val, uid); err != nil {
			response.ServerError(c)
			return
		}
	}
	response.OK(c, gin.H{"message": "告警邮件配置已保存"})
}

// TestSmtp 发送测试邮件（需先保存配置）。
func (h *SystemHandler) TestSmtp(c *gin.Context) {
	var req testSmtpReq
	if err := c.ShouldBindJSON(&req); err != nil {
		response.Fail(c, 400, err.Error())
		return
	}
	cfg, err := h.system.GetSmtpConfig()
	if err != nil {
		response.ServerError(c)
		return
	}
	if err := alertengine.SendTestMail(cfg, strings.TrimSpace(req.To)); err != nil {
		response.Fail(c, 400, err.Error())
		return
	}
	response.OK(c, gin.H{"message": "测试邮件已发送"})
}

// GetCrawlerConfig 返回当前爬虫配置（敏感字段脱敏）。
func (h *SystemHandler) GetCrawlerConfig(c *gin.Context) {
	cfg, err := h.system.GetCrawlerConfig()
	if err != nil {
		response.ServerError(c)
		return
	}
	response.OK(c, repository.BuildCrawlerConfigResponse(cfg))
}

// GetCrawlerLimits 返回爬虫配置上限（供工作流编辑器动态限制用）。
func (h *SystemHandler) GetCrawlerLimits(c *gin.Context) {
	cfg, err := h.system.GetCrawlerConfig()
	if err != nil {
		response.ServerError(c)
		return
	}
	response.OK(c, gin.H{
		"maxNotesCount": cfg.MaxNotesCount,
	})
}

// updateCrawlerReq 使用指针字段实现部分更新：nil 表示不修改。
type updateCrawlerReq struct {
	MaxNotesCount     *int    `json:"maxNotesCount"`
	MaxConcurrency    *int    `json:"maxConcurrency"`
	SleepSecMin       *int    `json:"sleepSecMin"`
	SleepSecMax       *int    `json:"sleepSecMax"`
	EnableIPProxy     *bool   `json:"enableIPProxy"`
	IPProxyPoolCount  *int    `json:"ipProxyPoolCount"`
	IPProxyProvider   *string `json:"ipProxyProvider"`
	ProxyKdlSecretID  *string `json:"proxyKdlSecretId"`
	ProxyKdlSignature *string `json:"proxyKdlSignature"`
	ProxyKdlUsername  *string `json:"proxyKdlUsername"`
	ProxyKdlPassword  *string `json:"proxyKdlPassword"`
	ProxyWandouAppKey *string `json:"proxyWandouAppKey"`
	XhsSortType       *string `json:"xhsSortType"`
	WeiboSearchType   *string `json:"weiboSearchType"`
	DySortType        *int    `json:"dySortType"`
	ZhihuSort         *string `json:"zhihuSort"`
	ZhihuSearchTime   *string `json:"zhihuSearchTime"`
	// Cookie 按平台独立更新；key 为平台代码（xhs/dy/ks/bili/wb/tieba/zhihu）
	Cookies map[string]string `json:"cookies"`
}

// UpdateCrawlerConfig 合并并持久化爬虫配置。
func (h *SystemHandler) UpdateCrawlerConfig(c *gin.Context) {
	var req updateCrawlerReq
	if err := c.ShouldBindJSON(&req); err != nil {
		response.Fail(c, 400, err.Error())
		return
	}

	cur, err := h.system.GetCrawlerConfig()
	if err != nil {
		response.ServerError(c)
		return
	}
	merged := cur

	if req.MaxNotesCount != nil {
		if *req.MaxNotesCount < 1 || *req.MaxNotesCount > 500 {
			response.Fail(c, 400, "maxNotesCount 应在 1~500 之间")
			return
		}
		merged.MaxNotesCount = *req.MaxNotesCount
	}
	if req.MaxConcurrency != nil {
		if *req.MaxConcurrency < 1 || *req.MaxConcurrency > 10 {
			response.Fail(c, 400, "maxConcurrency 应在 1~10 之间")
			return
		}
		merged.MaxConcurrency = *req.MaxConcurrency
	}
	if req.SleepSecMin != nil {
		merged.SleepSecMin = *req.SleepSecMin
	}
	if req.SleepSecMax != nil {
		if req.SleepSecMin != nil && *req.SleepSecMax < *req.SleepSecMin {
			response.Fail(c, 400, "sleepSecMax 不能小于 sleepSecMin")
			return
		}
		merged.SleepSecMax = *req.SleepSecMax
	}
	if req.EnableIPProxy != nil {
		merged.EnableIPProxy = *req.EnableIPProxy
	}
	if req.IPProxyPoolCount != nil {
		merged.IPProxyPoolCount = *req.IPProxyPoolCount
	}
	if req.IPProxyProvider != nil {
		merged.IPProxyProvider = strings.TrimSpace(*req.IPProxyProvider)
	}
	if req.ProxyKdlSecretID != nil {
		merged.ProxyKdlSecretID = strings.TrimSpace(*req.ProxyKdlSecretID)
	}
	if req.ProxyKdlSignature != nil {
		merged.ProxyKdlSignature = strings.TrimSpace(*req.ProxyKdlSignature)
	}
	if req.ProxyKdlUsername != nil {
		merged.ProxyKdlUsername = strings.TrimSpace(*req.ProxyKdlUsername)
	}
	if req.ProxyKdlPassword != nil {
		merged.ProxyKdlPassword = strings.TrimSpace(*req.ProxyKdlPassword)
	}
	if req.ProxyWandouAppKey != nil {
		merged.ProxyWandouAppKey = strings.TrimSpace(*req.ProxyWandouAppKey)
	}
	if req.XhsSortType != nil {
		merged.XhsSortType = strings.TrimSpace(*req.XhsSortType)
	}
	if req.WeiboSearchType != nil {
		merged.WeiboSearchType = strings.TrimSpace(*req.WeiboSearchType)
	}
	if req.DySortType != nil {
		merged.DySortType = *req.DySortType
	}
	if req.ZhihuSort != nil {
		merged.ZhihuSort = strings.TrimSpace(*req.ZhihuSort)
	}
	if req.ZhihuSearchTime != nil {
		merged.ZhihuSearchTime = strings.TrimSpace(*req.ZhihuSearchTime)
	}
	// Cookie 按 key 单独合并，空字符串意味着清空
	for platform, cookie := range req.Cookies {
		switch platform {
		case "xhs":
			merged.CookieXhs = cookie
		case "dy":
			merged.CookieDy = cookie
		case "ks":
			merged.CookieKs = cookie
		case "bili":
			merged.CookieBili = cookie
		case "wb":
			merged.CookieWb = cookie
		case "tieba":
			merged.CookieTieba = cookie
		case "zhihu":
			merged.CookieZhihu = cookie
		}
	}

	uid := uint(0)
	if v, ok := c.Get("userID"); ok {
		if id, ok2 := v.(uint); ok2 {
			uid = id
		}
	}
	if err := h.system.SaveCrawlerConfig(merged, uid); err != nil {
		response.Fail(c, 500, "持久化失败: "+err.Error())
		return
	}
	response.OK(c, repository.BuildCrawlerConfigResponse(merged))
}
