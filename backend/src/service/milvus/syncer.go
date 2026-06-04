package milvus

import (
	"context"
	"crypto/sha256"
	"fmt"
	"log"
	"strings"
	"sync"
	"time"

	"gorm.io/gorm"
	"opinion-analysis/src/model"
	"opinion-analysis/src/repository"
)

// Syncer 增量向量同步服务，等价于 Python incremental_sync 调度器。
type Syncer struct {
	db      *gorm.DB
	milvus  *Service
	embed   *EmbedderClient
	rag     *repository.RAGRepository

	mu     sync.RWMutex
	cfg    SyncConfig
	notify chan struct{}

	cancelFn context.CancelFunc
	wg       sync.WaitGroup
}

// SyncConfig 运行时可热更新的同步配置。
type SyncConfig struct {
	Enabled         bool
	ChunkMaxChars   int
	ChunkOverlap    int
	SyncIntervalSec int
	SyncBatch       int
}

func defaultSyncConfig() SyncConfig {
	return SyncConfig{
		Enabled:         true,
		ChunkMaxChars:   420,
		ChunkOverlap:    72,
		SyncIntervalSec: 120,
		SyncBatch:       100,
	}
}

// NewSyncer 创建同步服务。
func NewSyncer(db *gorm.DB, milvus *Service, embed *EmbedderClient, rag *repository.RAGRepository) *Syncer {
	return &Syncer{
		db:     db,
		milvus: milvus,
		embed:  embed,
		rag:    rag,
		cfg:    defaultSyncConfig(),
		notify: make(chan struct{}, 1),
	}
}

// UpdateConfig 热更新配置；若 goroutine 正在 pause 等待，立即唤醒。
func (s *Syncer) UpdateConfig(cfg SyncConfig) {
	s.mu.Lock()
	s.cfg = cfg
	s.mu.Unlock()
	select {
	case s.notify <- struct{}{}:
	default:
	}
}

func (s *Syncer) snapshot() SyncConfig {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.cfg
}

// Start 启动后台 goroutine（ctx 取消时退出）。
func (s *Syncer) Start(ctx context.Context) {
	s.wg.Add(1)
	go func() {
		defer s.wg.Done()
		s.run(ctx)
	}()
}

// Restart 停止并重启 goroutine（用于 /rag/restart 接口）。
func (s *Syncer) Restart(ctx context.Context) {
	if s.cancelFn != nil {
		s.cancelFn()
		s.wg.Wait()
	}
	child, cancel := context.WithCancel(ctx)
	s.cancelFn = cancel
	s.Start(child)
}

func (s *Syncer) run(ctx context.Context) {
	log.Printf("[milvus-syncer] goroutine started")
	timer := time.NewTimer(10 * time.Second)
	defer timer.Stop()

	for {
		select {
		case <-ctx.Done():
			log.Printf("[milvus-syncer] stopped: %v", ctx.Err())
			return
		case <-timer.C:
			cfg := s.snapshot()
			interval := time.Duration(cfg.SyncIntervalSec) * time.Second
			if interval <= 0 {
				interval = 2 * time.Minute
			}
			if !cfg.Enabled {
				log.Printf("[milvus-syncer] paused: waiting for re-enable")
				select {
				case <-ctx.Done():
					return
				case <-s.notify:
				}
				timer.Reset(5 * time.Second)
				continue
			}
			if _, err := s.RunOnce(ctx, 0); err != nil {
				log.Printf("[milvus-syncer] sync error: %v", err)
			}
			timer.Reset(interval)
		}
	}
}

// RunOnce 执行一次增量同步；syncLogID > 0 时写 rag_sync_logs 进度。
func (s *Syncer) RunOnce(ctx context.Context, syncLogID uint) (map[string]any, error) {
	cfg := s.snapshot()

	// 1. 确保集合存在（用嵌入模型维度）
	dim, err := s.embed.Dim()
	if err != nil {
		return nil, fmt.Errorf("get embed dim: %w", err)
	}
	if err := s.milvus.EnsureCollection(ctx, dim); err != nil {
		return nil, fmt.Errorf("ensure collection: %w", err)
	}

	processed := 0
	upserted := 0
	chunksUp := 0
	chunksDeleted := 0
	var failedIDs []uint

	tickProgress := func(detail string) {
		if syncLogID == 0 {
			return
		}
		pct := min(99, processed*100/max(1, cfg.SyncBatch))
		_ = s.rag.UpdateSyncLogProgress(syncLogID, pct, detail, processed, chunksUp, chunksDeleted)
	}

	// 2. 清理软删文章的 chunk
	deletedIDs, err := s.rag.FindDeletedArticleIDs()
	if err != nil {
		log.Printf("[milvus-syncer] find deleted: %v", err)
	} else {
		for _, aid := range deletedIDs {
			if e := s.milvus.DeleteByArticle(ctx, aid); e == nil {
				chunksDeleted++
			}
		}
	}
	if syncLogID > 0 {
		tickProgress("清理软删文章对应的向量块")
	}

	// 3. 查待同步文章
	articles, err := s.rag.FindArticlesForSync(cfg.SyncBatch)
	if err != nil {
		return nil, fmt.Errorf("find articles: %w", err)
	}

	for _, art := range articles {
		processed++

		// 查评论
		var comments []model.ArticleComment
		s.db.WithContext(ctx).
			Where("article_id = ? AND deleted_at IS NULL", art.ID).
			Order("published_at ASC").
			Find(&comments)

		// 计算 hash
		hashSrc := BuildFullEmbedText(art.Title, art.Content, 2500)
		if len(comments) > 0 {
			parts := make([]string, 0, len(comments))
			for _, c := range comments {
				if c.Content != "" {
					parts = append(parts, c.Content)
				}
			}
			hashSrc += "\n" + strings.Join(parts, "\n")
		}
		h := sha256Hex(hashSrc)

		if art.EmbeddingContentHash != nil && *art.EmbeddingContentHash == h && art.EmbeddingSyncedAt != nil {
			continue // 无变化，跳过
		}

		// 切块 + embed
		n, err := s.syncArticle(ctx, art, comments, cfg)
		if err != nil {
			log.Printf("[milvus-syncer] article %d sync failed: %v", art.ID, err)
			failedIDs = append(failedIDs, art.ID)
			continue
		}
		chunksUp += n

		now := time.Now()
		if err := s.rag.UpdateEmbeddingSync(art.ID, h, now); err != nil {
			log.Printf("[milvus-syncer] update sync marker article %d: %v", art.ID, err)
		}
		upserted++

		if syncLogID > 0 && (processed%5 == 0 || processed == len(articles)) {
			tickProgress(fmt.Sprintf("已处理 %d/%d，已向量化 %d 篇", processed, len(articles), upserted))
		}
	}

	if syncLogID > 0 {
		ok := len(failedIDs) == 0
		msg := ""
		if !ok {
			ids := make([]string, len(failedIDs))
			for i, id := range failedIDs {
				ids[i] = fmt.Sprintf("%d", id)
			}
			msg = fmt.Sprintf("以下文章向量化失败（%d 篇），其余已成功：article_id=[%s]",
				len(failedIDs), strings.Join(ids, ","))
		}
		_ = s.rag.FinishSyncLog(syncLogID, ok, msg, processed, chunksUp, chunksDeleted)
	}

	result := map[string]any{
		"processed":       processed,
		"upserted":        upserted,
		"chunks_upserted": chunksUp,
		"chunks_deleted":  chunksDeleted,
		"failed":          len(failedIDs),
	}
	return result, nil
}

func (s *Syncer) syncArticle(ctx context.Context, art model.Article, comments []model.ArticleComment, cfg SyncConfig) (int, error) {
	fullText := BuildFullEmbedText(art.Title, art.Content, 2500)
	contentPieces := SemanticChunks(fullText, cfg.ChunkMaxChars, cfg.ChunkOverlap)

	var commentPieces []string
	if len(comments) > 0 {
		lines := make([]string, 0, len(comments))
		for _, c := range comments {
			content := strings.TrimSpace(c.Content)
			if content == "" {
				continue
			}
			nickname := strings.TrimSpace(c.Nickname)
			if nickname == "" {
				nickname = "匿名"
			}
			prefix := "[评论] "
			if c.ParentID != nil {
				prefix = "  └ "
			}
			lines = append(lines, prefix+nickname+": "+content)
		}
		commentText := strings.Join(lines, "\n")
		if commentText != "" {
			commentPieces = SemanticChunks(commentText, cfg.ChunkMaxChars, cfg.ChunkOverlap)
		}
	}

	if len(contentPieces)+len(commentPieces) == 0 {
		_ = s.milvus.DeleteByArticle(ctx, int64(art.ID))
		return 0, nil
	}

	// 构建 embed 输入
	inputs := make([]string, 0, len(contentPieces)+len(commentPieces))
	for _, p := range contentPieces {
		inputs = append(inputs, EmbedChunkText(art.Title, p))
	}
	for _, p := range commentPieces {
		inputs = append(inputs, EmbedChunkText(art.Title, p))
	}

	vecs, err := s.embed.Encode(inputs)
	if err != nil {
		return 0, fmt.Errorf("encode: %w", err)
	}

	rows := make([]ChunkRow, 0, len(inputs))
	idx := 0
	for i, piece := range contentPieces {
		h := sha256Hex(piece)[:12]
		pk := fmt.Sprintf("%d:%d:%s", art.ID, idx, h)
		if len(pk) > 96 {
			pk = pk[:96]
		}
		rows = append(rows, ChunkRow{
			ChunkPK:   pk,
			ArticleID: int64(art.ID),
			ChunkIdx:  int64(idx),
			Embedding: vecs[i],
			Title:     art.Title,
			Snippet:   piece,
			Platform:  art.Platform,
			ChunkType: "content",
			Topic:     art.Topic,
		})
		idx++
	}
	for i, piece := range commentPieces {
		h := sha256Hex(piece)[:12]
		pk := fmt.Sprintf("%d:%d:%s", art.ID, idx, h)
		if len(pk) > 96 {
			pk = pk[:96]
		}
		rows = append(rows, ChunkRow{
			ChunkPK:   pk,
			ArticleID: int64(art.ID),
			ChunkIdx:  int64(idx),
			Embedding: vecs[len(contentPieces)+i],
			Title:     art.Title,
			Snippet:   piece,
			Platform:  art.Platform,
			ChunkType: "comment",
			Topic:     art.Topic,
		})
		idx++
	}

	// 先删旧 chunk，再插入新 chunk
	if err := s.milvus.DeleteByArticle(ctx, int64(art.ID)); err != nil {
		log.Printf("[milvus-syncer] delete old chunks article %d: %v", art.ID, err)
	}
	return len(rows), s.milvus.Insert(ctx, rows)
}

func sha256Hex(s string) string {
	h := sha256.Sum256([]byte(s))
	return fmt.Sprintf("%x", h)
}

func min(a, b int) int {
	if a < b {
		return a
	}
	return b
}
