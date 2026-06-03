package milvus

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"
)

// EmbedderClient HTTP 客户端，调用 Python embedding 服务的 /v1/embed 接口。
type EmbedderClient struct {
	baseURL string
	http    *http.Client
}

// NewEmbedderClient 创建 embedding 客户端。
func NewEmbedderClient(baseURL string) *EmbedderClient {
	return &EmbedderClient{
		baseURL: strings.TrimRight(baseURL, "/"),
		http:    &http.Client{Timeout: 90 * time.Second},
	}
}

type embedReq struct {
	Texts     []string `json:"texts"`
	Normalize bool     `json:"normalize"`
}

type embedResp struct {
	Vectors [][]float32 `json:"vectors"`
	Dim     int         `json:"dim"`
}

// Encode 将 texts 编码为向量，normalize=true。
func (c *EmbedderClient) Encode(texts []string) ([][]float32, error) {
	if len(texts) == 0 {
		return nil, nil
	}
	payload, err := json.Marshal(embedReq{Texts: texts, Normalize: true})
	if err != nil {
		return nil, err
	}
	req, err := http.NewRequest(http.MethodPost, c.baseURL+"/v1/embed", bytes.NewReader(payload))
	if err != nil {
		return nil, err
	}
	req.Header.Set("Content-Type", "application/json")
	resp, err := c.http.Do(req)
	if err != nil {
		return nil, fmt.Errorf("embedding service unreachable: %w", err)
	}
	defer resp.Body.Close()
	body, _ := io.ReadAll(resp.Body)
	if resp.StatusCode/100 != 2 {
		return nil, fmt.Errorf("embedding service status=%d body=%s", resp.StatusCode, body)
	}
	var out embedResp
	if err := json.Unmarshal(body, &out); err != nil {
		return nil, fmt.Errorf("embedding decode: %w", err)
	}
	if len(out.Vectors) != len(texts) {
		return nil, fmt.Errorf("embedding returned %d vectors for %d inputs", len(out.Vectors), len(texts))
	}
	return out.Vectors, nil
}

// Dim 返回当前模型向量维度。
// 先尝试 /health（已加载时快速返回），若维度为 0 则实际 encode 一次以触发模型加载。
func (c *EmbedderClient) Dim() (int, error) {
	resp, err := c.http.Get(c.baseURL + "/health")
	if err != nil {
		return 0, fmt.Errorf("embedding health: %w", err)
	}
	defer resp.Body.Close()
	body, _ := io.ReadAll(resp.Body)
	var h struct {
		EmbedDimension int `json:"embed_dimension"`
	}
	if err := json.Unmarshal(body, &h); err != nil {
		return 0, err
	}
	if h.EmbedDimension > 0 {
		return h.EmbedDimension, nil
	}
	// 模型尚未加载（延迟加载），触发一次真实 encode 来获取维度
	vecs, err := c.Encode([]string{"probe"})
	if err != nil {
		return 0, fmt.Errorf("probe embed to get dim: %w", err)
	}
	if len(vecs) == 0 || len(vecs[0]) == 0 {
		return 0, fmt.Errorf("embed probe returned empty vector")
	}
	return len(vecs[0]), nil
}

// BaseURL 返回 embedding 服务地址（供 handler 直接拼接 /health 等路径）。
func (c *EmbedderClient) BaseURL() string { return c.baseURL }

// HealthOK 检查 embedding 服务是否可达。
func (c *EmbedderClient) HealthOK() bool {
	client := &http.Client{Timeout: 5 * time.Second}
	resp, err := client.Get(c.baseURL + "/health")
	if err != nil {
		return false
	}
	defer resp.Body.Close()
	return resp.StatusCode/100 == 2
}
