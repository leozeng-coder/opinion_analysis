// Package rag 调用本机 Python RAG 服务（Milvus Lite + 混合检索）。
package rag

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"
)

// Client HTTP 客户端，EmbeddingServiceURL 为空时 Search 直接返回 nil。
type Client struct {
	BaseURL string
	HTTP    *http.Client
}

// Chunk 单条检索结果。
type Chunk struct {
	ArticleID uint    `json:"article_id"`
	Title     string  `json:"title"`
	Snippet   string  `json:"snippet"`
	Platform  string  `json:"platform"`
	Score     float64 `json:"score"`
	Source    string  `json:"source"` // vector | keyword | hybrid
}

type searchReq struct {
	Query string `json:"query"`
	TopK  int    `json:"top_k"`
}

type searchResp struct {
	Chunks []Chunk `json:"chunks"`
}

// Search 混合检索；baseURL 为空或服务不可用时返回 (nil, nil)。
func (c *Client) Search(ctx context.Context, query string, topK int) ([]Chunk, error) {
	q := strings.TrimSpace(query)
	if c == nil || strings.TrimSpace(c.BaseURL) == "" || q == "" {
		return nil, nil
	}
	if topK <= 0 {
		topK = 8
	}
	if topK > 20 {
		topK = 20
	}

	body, err := json.Marshal(searchReq{Query: q, TopK: topK})
	if err != nil {
		return nil, err
	}
	url := strings.TrimRight(c.BaseURL, "/") + "/v1/search"
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, url, bytes.NewReader(body))
	if err != nil {
		return nil, err
	}
	req.Header.Set("Content-Type", "application/json")

	client := c.HTTP
	if client == nil {
		client = &http.Client{Timeout: 25 * time.Second}
	}
	resp, err := client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("rag search: %w", err)
	}
	defer resp.Body.Close()
	respBody, _ := io.ReadAll(resp.Body)
	if resp.StatusCode/100 != 2 {
		return nil, fmt.Errorf("rag status=%d body=%s", resp.StatusCode, truncate(string(respBody), 200))
	}

	var out searchResp
	if err := json.Unmarshal(respBody, &out); err != nil {
		return nil, err
	}

	// 相关性过滤：移除低分结果
	filtered := FilterByRelevance(out.Chunks, 0.3)
	return filtered, nil
}

func truncate(s string, n int) string {
	r := []rune(s)
	if len(r) <= n {
		return s
	}
	return string(r[:n]) + "…"
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

	// 如果过滤后为空，但原始结果不为空，则保留得分最高的一条（降级策略）
	if len(filtered) == 0 && len(chunks) > 0 {
		best := chunks[0]
		for _, ch := range chunks {
			if ch.Score > best.Score {
				best = ch
			}
		}
		filtered = []Chunk{best}
	}

	return filtered
}

// FormatContext 将检索结果拼成可供 LLM 阅读的文本。
func FormatContext(chunks []Chunk) string {
	if len(chunks) == 0 {
		return ""
	}
	var b strings.Builder

	// 添加检索质量提示
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
		fmt.Fprintf(&b, "[#%d id=%d platform=%s score=%.3f src=%s]\n标题：%s\n摘要：%s",
			i+1, ch.ArticleID, ch.Platform, ch.Score, ch.Source, ch.Title, ch.Snippet)
	}
	return b.String()
}
