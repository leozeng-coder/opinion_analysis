package admin

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/gin-gonic/gin"
	"opinion-analysis/config"
	"opinion-analysis/pkg/response"
	"opinion-analysis/src/model"
	"opinion-analysis/src/repository"
	"opinion-analysis/src/service/milvus"
	"opinion-analysis/src/service/ragprocess"
)

type RAGHandler struct {
	rag    *repository.RAGRepository
	proc   *ragprocess.Manager
	milvus *milvus.Service
	embed  *milvus.EmbedderClient
	syncer *milvus.Syncer
}

func NewRAGHandler(
	store *repository.Store,
	proc *ragprocess.Manager,
	milvusSvc *milvus.Service,
	embed *milvus.EmbedderClient,
	syncer *milvus.Syncer,
) *RAGHandler {
	return &RAGHandler{
		rag:    store.RAG,
		proc:   proc,
		milvus: milvusSvc,
		embed:  embed,
		syncer: syncer,
	}
}

// Status GET /api/admin/rag/status — 所有远程探测并发执行，不阻塞模型加载。
func (h *RAGHandler) Status(c *gin.Context) {
	out := gin.H{
		"ragEnabled":              false,
		"embeddingServiceUrl":     "",
		"serviceReachable":        false,
		"embedModel":              "",
		"embedDim":                0,
		"milvusUri":               "",
		"collection":              "",
		"note":                    "句向量用于 RAG 检索，与「大模型配置」中的对话 API 不是同一个模型。",
		"syncIntervalSecondsHint": 120,
	}
	if config.Cfg != nil {
		out["ragEnabled"] = config.Cfg.RAG.Enabled
		out["embeddingServiceUrl"] = config.Cfg.RAG.EmbeddingServiceURL
		out["milvusUri"] = config.Cfg.RAG.MilvusURI
		out["collection"] = config.Cfg.RAG.MilvusCollection
	}
	if h.proc != nil {
		out["processManaged"] = h.proc.ManagedEnabled()
		out["processRunning"] = h.proc.IsRunning()
		if pid := h.proc.PID(); pid > 0 {
			out["processPid"] = pid
		}
	}

	// 并发探测：embedding 服务 + Milvus，各自独立超时，互不阻塞
	var wg sync.WaitGroup
	var mu sync.Mutex

	if h.embed != nil {
		wg.Add(1)
		go func() {
			defer wg.Done()
			// 只读 /health，不触发模型加载（dim=0 表示模型还未加载）
			type healthResp struct {
				EmbedDimension int    `json:"embed_dimension"`
				EmbedModel     string `json:"embed_model"`
				EmbedProvider  string `json:"embed_provider"`
				EmbedderReady  bool   `json:"embedder_ready"`
			}
			client := &http.Client{Timeout: 3 * time.Second}
			resp, err := client.Get(h.embed.BaseURL() + "/health")
			if err != nil {
				mu.Lock()
				out["serviceError"] = err.Error()
				mu.Unlock()
				return
			}
			defer resp.Body.Close()
			if resp.StatusCode/100 != 2 {
				mu.Lock()
				out["serviceError"] = "http " + resp.Status
				mu.Unlock()
				return
			}
			var hr healthResp
			if err := json.NewDecoder(resp.Body).Decode(&hr); err == nil {
				mu.Lock()
				out["serviceReachable"] = true
				out["embedDim"] = hr.EmbedDimension
				out["embedderReady"] = hr.EmbedderReady
				if hr.EmbedModel != "" {
					out["embedModel"] = hr.EmbedModel
				}
				if hr.EmbedProvider != "" {
					out["embedProvider"] = hr.EmbedProvider
				}
				mu.Unlock()
			}
		}()
	}

	if h.milvus != nil {
		wg.Add(1)
		go func() {
			defer wg.Done()
			if err := h.milvus.Ping(3 * time.Second); err == nil {
				ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
				defer cancel()
				exists, _ := h.milvus.HasCollection(ctx)
				mu.Lock()
				out["milvusReachable"] = true
				out["collectionExists"] = exists
				mu.Unlock()
			} else {
				mu.Lock()
				out["milvusError"] = err.Error()
				mu.Unlock()
			}
		}()
	}

	wg.Wait()

	// sync_enabled + embedding 配置（纯 DB 读，极快）
	if ss, err := h.rag.GetSyncEnabledSetting(); err == nil && ss != nil {
		out["syncEnabled"] = strings.ToLower(strings.TrimSpace(ss.Value)) == "true"
	} else {
		out["syncEnabled"] = true
	}
	if cfg, err := h.rag.GetRagConfig(); err == nil {
		if out["embedModel"] == "" || out["embedModel"] == nil {
			out["embedModel"] = cfg.EmbedModel
		}
		if out["embedProvider"] == nil || out["embedProvider"] == "" {
			out["embedProvider"] = cfg.EmbedProvider
		}
		out["syncIntervalSecondsHint"] = cfg.SyncIntervalSec
	}

	response.OK(c, out)
}

// ListRuns GET /api/admin/rag/runs
func (h *RAGHandler) ListRuns(c *gin.Context) {
	page, _ := strconv.Atoi(c.DefaultQuery("page", "1"))
	pageSize, _ := strconv.Atoi(c.DefaultQuery("pageSize", "10"))
	if page < 1 {
		page = 1
	}
	if pageSize < 1 || pageSize > 50 {
		pageSize = 10
	}
	logs, total, err := h.rag.ListSyncLogs(page, pageSize)
	if err != nil {
		response.ServerError(c)
		return
	}
	response.OKPage(c, total, logs)
}

// TriggerSync POST /api/admin/rag/sync — 在 goroutine 中执行一次同步并立即返回 syncLogId。
func (h *RAGHandler) TriggerSync(c *gin.Context) {
	logRow := model.RagSyncLog{
		Status:    "running",
		Progress:  0,
		Mode:      "manual",
		StartedAt: time.Now(),
	}
	if err := h.rag.CreateSyncLog(&logRow); err != nil {
		response.ServerError(c)
		return
	}
	go func() {
		ctx, cancel := context.WithTimeout(context.Background(), 30*time.Minute)
		defer cancel()
		if _, err := h.syncer.RunOnce(ctx, logRow.ID); err != nil {
			_ = h.rag.FailSyncLog(&logRow, err.Error())
		}
	}()
	response.OK(c, gin.H{
		"syncLogId": logRow.ID,
		"message":   "已提交同步任务，请在下方列表查看进度",
	})
}

// GetConfig GET /api/admin/rag/config
func (h *RAGHandler) GetConfig(c *gin.Context) {
	cfg, err := h.rag.GetRagConfig()
	if err != nil {
		response.Fail(c, 500, "读取配置失败: "+err.Error())
		return
	}
	response.OK(c, repository.BuildRagConfigResponse(cfg, nil, false, ""))
}

// UpdateConfig PUT /api/admin/rag/config
func (h *RAGHandler) UpdateConfig(c *gin.Context) {
	var req updateRagConfigReq
	if err := c.ShouldBindJSON(&req); err != nil {
		response.Fail(c, 400, "请求体解析失败: "+err.Error())
		return
	}
	cur, err := h.rag.GetRagConfig()
	if err != nil {
		response.Fail(c, 500, "读取当前配置失败: "+err.Error())
		return
	}
	merged := cur
	if req.SyncEnabled != nil {
		merged.SyncEnabled = *req.SyncEnabled
	}
	if req.EmbedProvider != nil {
		merged.EmbedProvider = strings.TrimSpace(*req.EmbedProvider)
	}
	if req.EmbedModel != nil {
		merged.EmbedModel = strings.TrimSpace(*req.EmbedModel)
	}
	if req.EmbedAPIBase != nil {
		merged.EmbedAPIBase = strings.TrimSpace(*req.EmbedAPIBase)
	}
	if req.EmbedAPIKey != nil && *req.EmbedAPIKey != "" {
		merged.EmbedAPIKey = strings.TrimSpace(*req.EmbedAPIKey)
	}
	if req.ChunkMaxChars != nil {
		merged.ChunkMaxChars = *req.ChunkMaxChars
	}
	if req.ChunkOverlap != nil {
		merged.ChunkOverlap = *req.ChunkOverlap
	}
	if req.SyncIntervalSec != nil {
		merged.SyncIntervalSec = *req.SyncIntervalSec
	}
	if req.SyncBatch != nil {
		merged.SyncBatch = *req.SyncBatch
	}
	p := strings.ToLower(strings.TrimSpace(merged.EmbedProvider))
	if p == "" {
		p = "local"
	}
	if p != "local" && p != "api" {
		response.Fail(c, 400, "embed_provider 必须是 local 或 api")
		return
	}
	merged.EmbedProvider = p
	if merged.EmbedModel == "" {
		response.Fail(c, 400, "embed_model 不能为空")
		return
	}
	if merged.ChunkMaxChars < 128 || merged.ChunkMaxChars > 2000 {
		response.Fail(c, 400, "chunk_max_chars 应在 128~2000")
		return
	}
	if merged.SyncIntervalSec < 30 || merged.SyncIntervalSec > 86400 {
		response.Fail(c, 400, "sync_interval_sec 应在 30~86400")
		return
	}
	if merged.SyncBatch < 1 || merged.SyncBatch > 2000 {
		response.Fail(c, 400, "sync_batch 应在 1~2000")
		return
	}

	var cu uint
	if uid, ok := c.Get("userID"); ok {
		if id, ok2 := uid.(uint); ok2 {
			cu = id
		}
	}
	actorName := ""
	if uname, ok := c.Get("username"); ok {
		if name, ok2 := uname.(string); ok2 {
			actorName = name
		}
	}
	if err := h.rag.SaveRagConfig(merged, cu, actorName); err != nil {
		response.Fail(c, 500, "持久化失败: "+err.Error())
		return
	}

	// 热更新 syncer 配置
	if h.syncer != nil {
		h.syncer.UpdateConfig(milvus.SyncConfig{
			Enabled:         merged.SyncEnabled,
			ChunkMaxChars:   merged.ChunkMaxChars,
			ChunkOverlap:    merged.ChunkOverlap,
			SyncIntervalSec: merged.SyncIntervalSec,
			SyncBatch:       merged.SyncBatch,
		})
	}
	response.OK(c, repository.BuildRagConfigResponse(merged, nil, true, ""))
}

type updateRagConfigReq struct {
	SyncEnabled     *bool   `json:"sync_enabled"`
	EmbedProvider   *string `json:"embed_provider"`
	EmbedModel      *string `json:"embed_model"`
	EmbedAPIBase    *string `json:"embed_api_base"`
	EmbedAPIKey     *string `json:"embed_api_key"`
	ChunkMaxChars   *int    `json:"chunk_max_chars"`
	ChunkOverlap    *int    `json:"chunk_overlap"`
	SyncIntervalSec *int    `json:"sync_interval_sec"`
	SyncBatch       *int    `json:"sync_batch"`
}

// RebuildMilvus POST /api/admin/rag/milvus/rebuild
func (h *RAGHandler) RebuildMilvus(c *gin.Context) {
	ctx := c.Request.Context()

	// 1. Drop 并重建集合
	if err := h.milvus.DropCollection(ctx); err != nil {
		response.Fail(c, 500, "删除集合失败: "+err.Error())
		return
	}
	dim, err := h.embed.Dim()
	if err != nil {
		response.Fail(c, 502, "获取 embedding 维度失败: "+err.Error())
		return
	}
	if err := h.milvus.EnsureCollection(ctx, dim); err != nil {
		response.Fail(c, 500, "重建集合失败: "+err.Error())
		return
	}

	// 2. 重置 MySQL 同步标记
	n, err := h.rag.ResetAllEmbeddingSync()
	if err != nil {
		response.Fail(c, 500, "重置同步标记失败: "+err.Error())
		return
	}

	response.OK(c, gin.H{
		"ok":                       true,
		"collection":               h.milvus.CollectionName(),
		"embed_dimension":          dim,
		"articles_reset_for_resync": n,
	})
}

// RestartService POST /api/admin/rag/restart — 重启 Python embedding 子进程 + Go syncer。
func (h *RAGHandler) RestartService(c *gin.Context) {
	if h.proc == nil || !h.proc.ManagedEnabled() {
		response.Fail(c, 400, "RAG 进程托管未启用，请在 config.yaml 设置 rag.managed: true")
		return
	}
	ctx, cancel := context.WithTimeout(c.Request.Context(), 45*time.Second)
	defer cancel()

	result, err := h.proc.Restart(ctx)
	if err != nil {
		if result.Message != "" {
			response.Fail(c, 502, result.Message)
			return
		}
		response.Fail(c, 502, err.Error())
		return
	}

	// 重启 Go syncer goroutine
	if h.syncer != nil {
		h.syncer.Restart(context.Background())
	}
	response.OK(c, result)
}

// ListKBArticles GET /api/admin/rag/articles — 直接查 MySQL
func (h *RAGHandler) ListKBArticles(c *gin.Context) {
	page, _ := strconv.Atoi(c.DefaultQuery("page", "1"))
	pageSize, _ := strconv.Atoi(c.DefaultQuery("page_size", "20"))
	keyword := strings.TrimSpace(c.Query("keyword"))
	platform := strings.TrimSpace(c.Query("platform"))
	topic := strings.TrimSpace(c.Query("topic"))
	syncedFilter := c.Query("synced") // "yes" | "no" | ""
	if page < 1 {
		page = 1
	}
	if pageSize < 1 || pageSize > 100 {
		pageSize = 20
	}

	db := h.rag.DB()
	q := db.Model(&model.Article{}).Where("deleted_at IS NULL")
	if keyword != "" {
		like := "%" + keyword + "%"
		q = q.Where("title LIKE ? OR content LIKE ?", like, like)
	}
	if platform != "" {
		q = q.Where("platform = ?", platform)
	}
	if topic != "" {
		q = q.Where("topic = ?", topic)
	}
	switch syncedFilter {
	case "yes":
		q = q.Where("embedding_synced_at IS NOT NULL")
	case "no":
		q = q.Where("embedding_synced_at IS NULL")
	}

	var total int64
	q.Count(&total)

	var arts []model.Article
	offset := (page - 1) * pageSize
	q.Select("id, title, platform, topic, published_at, embedding_content_hash, embedding_synced_at").
		Order("id DESC").Offset(offset).Limit(pageSize).Find(&arts)

	type item struct {
		ID               uint    `json:"id"`
		Title            string  `json:"title"`
		Platform         string  `json:"platform"`
		Topic            string  `json:"topic"`
		PublishedAt      *string `json:"publishedAt"`
		EmbeddingHash    *string `json:"embeddingHash"`
		EmbeddingSyncedAt *string `json:"embeddingSyncedAt"`
		Synced           bool    `json:"synced"`
	}
	list := make([]item, len(arts))
	for i, a := range arts {
		it := item{
			ID:       a.ID,
			Title:    a.Title,
			Platform: a.Platform,
			Topic:    a.Topic,
			Synced:   a.EmbeddingSyncedAt != nil,
		}
		if !a.PublishedAt.IsZero() {
			s := a.PublishedAt.Format(time.RFC3339)
			it.PublishedAt = &s
		}
		if a.EmbeddingContentHash != nil {
			it.EmbeddingHash = a.EmbeddingContentHash
		}
		if a.EmbeddingSyncedAt != nil {
			s := a.EmbeddingSyncedAt.Format(time.RFC3339)
			it.EmbeddingSyncedAt = &s
		}
		list[i] = it
	}
	response.OKPage(c, total, list)
}

// GetKBArticleDetail GET /api/admin/rag/articles/:id — MySQL 元数据 + Milvus chunks
func (h *RAGHandler) GetKBArticleDetail(c *gin.Context) {
	idStr := c.Param("id")
	id, err := strconv.Atoi(idStr)
	if err != nil || id <= 0 {
		response.Fail(c, 400, "无效的文章 ID")
		return
	}

	var art model.Article
	if err := h.rag.DB().Where("id = ? AND deleted_at IS NULL", id).First(&art).Error; err != nil {
		response.Fail(c, 404, "文章不存在")
		return
	}

	// 查 Milvus chunks
	type chunkItem struct {
		ChunkPK   string `json:"chunkPk"`
		ChunkIdx  int64  `json:"chunkIdx"`
		Snippet   string `json:"snippet"`
		ChunkType string `json:"chunkType"`
	}
	var chunks []chunkItem
	if h.milvus != nil {
		hits, _ := h.milvus.QueryByArticle(c.Request.Context(), int64(art.ID))
		for _, hit := range hits {
			chunks = append(chunks, chunkItem{
				ChunkPK:   hit.ChunkPK,
				ChunkIdx:  hit.ChunkIdx,
				Snippet:   hit.Snippet,
				ChunkType: hit.ChunkType,
			})
		}
	}
	if chunks == nil {
		chunks = []chunkItem{}
	}

	type artDetail struct {
		ID                uint    `json:"id"`
		Title             string  `json:"title"`
		Platform          string  `json:"platform"`
		Author            string  `json:"author"`
		OriginURL         string  `json:"originUrl"`
		Sentiment         string  `json:"sentiment"`
		SentScore         float64 `json:"sentScore"`
		AITags            *string `json:"aiTags"`
		PublishedAt       *string `json:"publishedAt"`
		EmbeddingSyncedAt *string `json:"embeddingSyncedAt"`
		Synced            bool    `json:"synced"`
	}
	detail := artDetail{
		ID:        art.ID,
		Title:     art.Title,
		Platform:  art.Platform,
		Author:    art.Author,
		OriginURL: art.OriginURL,
		Sentiment: art.Sentiment,
		SentScore: art.SentScore,
		AITags:    art.AITags,
		Synced:    art.EmbeddingSyncedAt != nil,
	}
	if !art.PublishedAt.IsZero() {
		s := art.PublishedAt.Format(time.RFC3339)
		detail.PublishedAt = &s
	}
	if art.EmbeddingSyncedAt != nil {
		s := art.EmbeddingSyncedAt.Format(time.RFC3339)
		detail.EmbeddingSyncedAt = &s
	}

	response.OK(c, gin.H{"article": detail, "chunks": chunks})
}

// DeleteArticleEmbedding DELETE /api/admin/rag/articles/:id/embedding
func (h *RAGHandler) DeleteArticleEmbedding(c *gin.Context) {
	idStr := c.Param("id")
	id, err := strconv.Atoi(idStr)
	if err != nil || id <= 0 {
		response.Fail(c, 400, "无效的文章 ID")
		return
	}
	if h.milvus != nil {
		if err := h.milvus.DeleteByArticle(c.Request.Context(), int64(id)); err != nil {
			response.Fail(c, 502, "Milvus 删除失败: "+err.Error())
			return
		}
	}
	if err := h.rag.ResetArticleEmbeddingSync(uint(id)); err != nil {
		response.Fail(c, 500, "重置同步标记失败: "+err.Error())
		return
	}
	response.OK(c, gin.H{"ok": true})
}

// DeleteChunk DELETE /api/admin/rag/chunks?pk=...
func (h *RAGHandler) DeleteChunk(c *gin.Context) {
	pk := c.Query("pk")
	if pk == "" {
		response.Fail(c, 400, "缺少 pk 参数")
		return
	}
	if h.milvus == nil {
		response.Fail(c, 503, "Milvus 服务未初始化")
		return
	}
	if err := h.milvus.DeleteByPK(c.Request.Context(), pk); err != nil {
		response.Fail(c, 502, "删除 chunk 失败: "+err.Error())
		return
	}
	response.OK(c, gin.H{"ok": true})
}

// UpdateChunk PUT /api/admin/rag/chunks?pk=... — 修改 snippet 并重新向量化后写回 Milvus。
func (h *RAGHandler) UpdateChunk(c *gin.Context) {
	pk := c.Query("pk")
	if pk == "" {
		response.Fail(c, 400, "缺少 pk 参数")
		return
	}
	var body struct {
		Snippet string `json:"snippet"`
	}
	if err := c.ShouldBindJSON(&body); err != nil || strings.TrimSpace(body.Snippet) == "" {
		response.Fail(c, 400, "snippet 不能为空")
		return
	}
	if h.milvus == nil || h.embed == nil {
		response.Fail(c, 503, "Milvus/Embedding 服务未初始化")
		return
	}

	// 查旧 chunk 获取 title / articleId 等元数据
	old, err := h.milvus.QueryByPK(c.Request.Context(), pk)
	if err != nil || old == nil {
		response.Fail(c, 404, "chunk 不存在")
		return
	}

	newSnippet := strings.TrimSpace(body.Snippet)
	embedText := old.Title + "\n" + newSnippet
	vecs, err := h.embed.Encode([]string{embedText})
	if err != nil {
		response.Fail(c, 502, "向量化失败: "+err.Error())
		return
	}

	// 删旧、插新（相同 pk）
	if err := h.milvus.DeleteByPK(c.Request.Context(), pk); err != nil {
		response.Fail(c, 502, "删除旧 chunk 失败: "+err.Error())
		return
	}
	newRow := milvus.ChunkRow{
		ChunkPK:   pk,
		ArticleID: old.ArticleID,
		ChunkIdx:  old.ChunkIdx,
		Embedding: vecs[0],
		Title:     old.Title,
		Snippet:   newSnippet,
		Platform:  old.Platform,
		ChunkType: old.ChunkType,
	}
	if err := h.milvus.Insert(c.Request.Context(), []milvus.ChunkRow{newRow}); err != nil {
		response.Fail(c, 502, "写入 Milvus 失败: "+err.Error())
		return
	}
	response.OK(c, gin.H{"ok": true, "snippet": newSnippet})
}

// GetConfig / SaveConfig helpers — keep rag_config.go methods available
var _ = json.Marshal // keep import
var _ = fmt.Sprintf  // keep import
