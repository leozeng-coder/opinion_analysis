// Package rag 提供向量+关键词混合检索（Go 直连 Milvus + Python embedding 服务）。
package rag

import (
	"context"
	"fmt"
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
	Source    string  `json:"source"`    // vector | keyword | hybrid
	ChunkType string  `json:"chunk_type"` // content | comment
}

// Client 混合检索客户端。
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

// Search 混合检索；embed 或 milvus 未就绪时返回 (nil, nil)。
func (c *Client) Search(ctx context.Context, query string, topK int) ([]Chunk, error) {
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

	// 1. 向量化 query
	vecs, err := c.embed.Encode([]string{q})
	if err != nil {
		return nil, fmt.Errorf("rag embed query: %w", err)
	}
	if len(vecs) == 0 {
		return nil, nil
	}

	// 2. Milvus 向量检索（取 top-k*5 供去重/重排）
	limit := min(40, max(topK*5, topK))
	hits, err := c.milvus.Search(ctx, vecs[0], limit)
	if err != nil {
		return nil, fmt.Errorf("rag milvus search: %w", err)
	}

	// 3. 关键词候选（MySQL FULLTEXT / LIKE）
	kwIDs := c.keywordCandidateIDs(ctx, q)
	kwSet := make(map[int64]struct{}, len(kwIDs))
	for _, id := range kwIDs {
		kwSet[id] = struct{}{}
	}

	// 4. 合并：每篇文章保留最高分 chunk，关键词命中加分
	byArticle := make(map[int64]rankedChunk)
	for _, h := range hits {
		sim := float64(cosineToSim(h.Score))
		if _, isKW := kwSet[h.ArticleID]; isKW {
			sim = min64(1.0, sim+0.12)
		}
		src := "vector"
		if _, isKW := kwSet[h.ArticleID]; isKW {
			src = "hybrid"
		}
		ch := Chunk{
			ArticleID: uint(h.ArticleID),
			Title:     h.Title,
			Snippet:   truncateRunes(h.Snippet, 1500),
			Platform:  h.Platform,
			Score:     round4(sim),
			Source:    src,
			ChunkType: h.ChunkType,
		}
		if prev, ok := byArticle[h.ArticleID]; !ok || sim > prev.score {
			byArticle[h.ArticleID] = rankedChunk{score: sim, chunk: ch}
		}
	}

	// 5. 按分数排序取 top-k
	sorted := sortByScore(byArticle)
	out := make([]Chunk, 0, topK)
	seen := make(map[int64]struct{})
	for _, r := range sorted {
		if len(out) >= topK {
			break
		}
		out = append(out, r.chunk)
		seen[int64(r.chunk.ArticleID)] = struct{}{}
	}

	// 6. 若不足 top-k，用关键词候选补齐
	if len(out) < topK && c.db != nil {
		out = c.fillFromKeywords(ctx, out, seen, kwIDs, topK)
	}

	return FilterByRelevance(out, 0.3), nil
}

// keywordCandidateIDs 用 MySQL LIKE 查关键词匹配的文章 ID（补充向量检索的覆盖面）。
func (c *Client) keywordCandidateIDs(ctx context.Context, query string) []int64 {
	if c.db == nil || len([]rune(query)) < 2 {
		return nil
	}
	words := strings.Fields(query)
	if len(words) == 0 {
		return nil
	}
	// 取前 3 个词，各做 LIKE 匹配，取 union（OR 逻辑）
	clauses := make([]string, 0, len(words))
	args := make([]any, 0, len(words)*2)
	for i, w := range words {
		if i >= 3 {
			break
		}
		like := "%" + w + "%"
		clauses = append(clauses, "(title LIKE ? OR content LIKE ?)")
		args = append(args, like, like)
	}
	where := strings.Join(clauses, " OR ")
	var ids []int64
	c.db.WithContext(ctx).
		Table("articles").
		Where("deleted_at IS NULL AND ("+where+")", args...).
		Limit(20).
		Pluck("id", &ids)
	return ids
}

// fillFromKeywords 用关键词候选文章的摘要补足结果。
func (c *Client) fillFromKeywords(ctx context.Context, out []Chunk, seen map[int64]struct{}, kwIDs []int64, topK int) []Chunk {
	type artRow struct {
		ID       uint
		Title    string
		Content  string
		Platform string
	}
	for _, aid := range kwIDs {
		if len(out) >= topK {
			break
		}
		if _, ok := seen[aid]; ok {
			continue
		}
		var row artRow
		if err := c.db.WithContext(ctx).Table("articles").
			Select("id, title, content, platform").
			Where("id = ? AND deleted_at IS NULL", aid).
			First(&row).Error; err != nil {
			continue
		}
		snippet := truncateRunes(row.Content, 500)
		out = append(out, Chunk{
			ArticleID: row.ID,
			Title:     row.Title,
			Snippet:   snippet,
			Platform:  row.Platform,
			Score:     0.3,
			Source:    "keyword",
			ChunkType: "content",
		})
		seen[aid] = struct{}{}
	}
	return out
}

// FilterByRelevance 过滤低相关性结果（基于 score 阈值）。
func FilterByRelevance(chunks []Chunk, minScore float64) []Chunk {
	if len(chunks) == 0 {
		return chunks
	}
	var filtered []Chunk
	for _, ch := range chunks {
		if ch.Score >= minScore {
			filtered = append(filtered, ch)
		}
	}
	if len(filtered) == 0 {
		best := chunks[0]
		for _, ch := range chunks {
			if ch.Score > best.Score {
				best = ch
			}
		}
		return []Chunk{best}
	}
	return filtered
}

// FormatContext 将检索结果拼成可供 LLM 阅读的文本。
func FormatContext(chunks []Chunk) string {
	if len(chunks) == 0 {
		return ""
	}
	var b strings.Builder
	avgScore := 0.0
	for _, ch := range chunks {
		avgScore += ch.Score
	}
	avgScore /= float64(len(chunks))
	if avgScore < 0.5 {
		b.WriteString("[检索提示：以下结果相关性较低，仅供参考]\n\n")
	}
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
		fmt.Fprintf(&b, "[#%d id=%d platform=%s type=%s score=%.3f src=%s]\n标题：%s\n%s：%s",
			i+1, ch.ArticleID, ch.Platform, label, ch.Score, ch.Source, ch.Title, label, ch.Snippet)
	}
	return b.String()
}

// ---- helpers ----

type rankedChunk struct {
	score float64
	chunk Chunk
}

func sortByScore(m map[int64]rankedChunk) []rankedChunk {
	out := make([]rankedChunk, 0, len(m))
	for _, v := range m {
		out = append(out, v)
	}
	// simple insertion sort (small N)
	for i := 1; i < len(out); i++ {
		for j := i; j > 0 && out[j].score > out[j-1].score; j-- {
			out[j], out[j-1] = out[j-1], out[j]
		}
	}
	return out
}

func cosineToSim(d float32) float32 {
	// Milvus COSINE metric already returns similarity in [-1,1]; clamp to [0,1].
	if d < 0 {
		return 0
	}
	if d > 1 {
		return 1
	}
	return d
}

func round4(f float64) float64 {
	return float64(int(f*10000+0.5)) / 10000
}

func min64(a, b float64) float64 {
	if a < b {
		return a
	}
	return b
}

func min(a, b int) int {
	if a < b {
		return a
	}
	return b
}

func max(a, b int) int {
	if a > b {
		return a
	}
	return b
}

func truncateRunes(s string, n int) string {
	r := []rune(s)
	if len(r) <= n {
		return s
	}
	return string(r[:n]) + "…"
}
