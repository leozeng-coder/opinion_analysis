// Package rag 提供多路召回检索（Go 直连 Milvus：dense 向量 + BM25 稀疏向量，RRF 融合）。
// Python 侧仅负责 embedding。
package rag

import (
	"context"
	"fmt"
	"sort"
	"strings"

	"gorm.io/gorm"
	"opinion-analysis/src/service/milvus"
)

// Chunk 单条检索结果。
type Chunk struct {
	ArticleID uint    `json:"article_id"`
	Title     string  `json:"title"`
	Snippet   string  `json:"snippet"`
	Platform  string  `json:"platform"`
	Score     float64 `json:"score"`
	Source    string  `json:"source"`     // hybrid | vector | bm25
	ChunkType string  `json:"chunk_type"` // content | comment
}

// Client 多路召回检索客户端。
type Client struct {
	// BaseURL 仍保留（ai_chat handler 用 BaseURL 字段构造），但检索逻辑已迁到 Milvus/Embed。
	BaseURL string
	embed   *milvus.EmbedderClient
	milvus  *milvus.Service
	db      *gorm.DB
}

// NewClient 创建带完整依赖的检索客户端。
func NewClient(baseURL string, embed *milvus.EmbedderClient, milvusSvc *milvus.Service, db *gorm.DB) *Client {
	return &Client{
		BaseURL: baseURL,
		embed:   embed,
		milvus:  milvusSvc,
		db:      db,
	}
}

// Search 多路召回检索；embed 或 milvus 未就绪时返回 (nil, nil)。
// dense 向量与 BM25 稀疏向量两路由 Milvus 内部 RRF 融合，topics 为空时搜全部话题。
func (c *Client) Search(ctx context.Context, query string, topK int, topics []string) ([]Chunk, error) {
	q := strings.TrimSpace(query)
	if c == nil || q == "" {
		return nil, nil
	}
	if c.embed == nil || c.milvus == nil {
		return nil, nil
	}
	if topK <= 0 {
		topK = 8
	}
	if topK > 20 {
		topK = 20
	}

	// 1. 向量化 query（dense 路）；BM25 路由 Milvus 用原始文本分词
	vecs, err := c.embed.Encode([]string{q})
	if err != nil {
		return nil, fmt.Errorf("rag embed query: %w", err)
	}
	var vec []float32
	if len(vecs) > 0 {
		vec = vecs[0]
	}

	// 2. Milvus 多路召回（dense + BM25 → RRF），取 topK*2 候选供按文章去重
	hits, err := c.milvus.Search(ctx, q, vec, topK*2, topics)
	if err != nil {
		return nil, fmt.Errorf("rag milvus search: %w", err)
	}
	if len(hits) == 0 {
		return nil, nil
	}

	// 3. 每篇文章保留最高分 chunk，按 RRF 分数排序取 top-k
	byArticle := make(map[int64]Chunk)
	for _, h := range hits {
		ch := Chunk{
			ArticleID: uint(h.ArticleID),
			Title:     h.Title,
			Snippet:   truncateRunes(h.Snippet, 1500),
			Platform:  h.Platform,
			Score:     round4(float64(h.Score)),
			Source:    "hybrid",
			ChunkType: h.ChunkType,
		}
		if prev, ok := byArticle[h.ArticleID]; !ok || ch.Score > prev.Score {
			byArticle[h.ArticleID] = ch
		}
	}

	out := make([]Chunk, 0, len(byArticle))
	for _, ch := range byArticle {
		out = append(out, ch)
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Score > out[j].Score })
	if len(out) > topK {
		out = out[:topK]
	}
	return out, nil
}

// FormatContext 将检索结果拼成可供 LLM 阅读的文本。
func FormatContext(chunks []Chunk) string {
	if len(chunks) == 0 {
		return ""
	}
	var b strings.Builder
	for i, ch := range chunks {
		if i > 0 {
			b.WriteString("\n---\n")
		}
		chType := ch.ChunkType
		if chType == "" {
			chType = "content"
		}
		label := "正文"
		if chType == "comment" {
			label = "用户评论"
		}
		fmt.Fprintf(&b, "[#%d id=%d platform=%s type=%s score=%.4f src=%s]\n标题：%s\n%s：%s",
			i+1, ch.ArticleID, ch.Platform, label, ch.Score, ch.Source, ch.Title, label, ch.Snippet)
	}
	return b.String()
}

// ---- helpers ----

func round4(f float64) float64 {
	return float64(int(f*10000+0.5)) / 10000
}

func truncateRunes(s string, n int) string {
	r := []rune(s)
	if len(r) <= n {
		return s
	}
	return string(r[:n]) + "…"
}
