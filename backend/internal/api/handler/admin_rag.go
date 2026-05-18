package handler

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/gin-gonic/gin"
	"gorm.io/gorm"
	"opinion-analysis/config"
	"opinion-analysis/internal/model"
	"opinion-analysis/pkg/response"
)

// AdminRagHandler 向量知识库（RAG）同步管理与状态。
type AdminRagHandler struct {
	db *gorm.DB
}

func NewAdminRagHandler(db *gorm.DB) *AdminRagHandler {
	return &AdminRagHandler{db: db}
}

func ragServiceURL() string {
	if config.Cfg == nil {
		return ""
	}
	return strings.TrimSpace(config.Cfg.RAG.EmbeddingServiceURL)
}

// proxyGet 向 RAG Python 服务发 GET 请求，将响应体 JSON 回传给前端（包裹标准 response 格式）。
func proxyGet(c *gin.Context, path string) {
	url := ragServiceURL()
	if url == "" {
		response.Fail(c, 400, "未配置 rag.embedding_service_url")
		return
	}
	client := &http.Client{Timeout: 8 * time.Second}
	req, err := http.NewRequestWithContext(c.Request.Context(), http.MethodGet, strings.TrimRight(url, "/")+path, nil)
	if err != nil {
		response.Fail(c, 502, err.Error())
		return
	}
	resp, err := client.Do(req)
	if err != nil {
		response.Fail(c, 502, "RAG 服务不可达: "+err.Error())
		return
	}
	defer resp.Body.Close()
	body, _ := io.ReadAll(resp.Body)
	if resp.StatusCode/100 != 2 {
		response.Fail(c, 502, string(body))
		return
	}
	var result any
	if err := json.Unmarshal(body, &result); err != nil {
		response.Fail(c, 502, "RAG 服务返回非法 JSON")
		return
	}
	response.OK(c, result)
}

// proxyPut 向 RAG Python 服务发 PUT 请求，转发请求体。
func proxyPut(c *gin.Context, path string) {
	url := ragServiceURL()
	if url == "" {
		response.Fail(c, 400, "未配置 rag.embedding_service_url")
		return
	}
	payload, err := io.ReadAll(c.Request.Body)
	if err != nil {
		response.Fail(c, 400, "读请求体失败")
		return
	}
	client := &http.Client{Timeout: 8 * time.Second}
	req, err := http.NewRequestWithContext(c.Request.Context(), http.MethodPut,
		strings.TrimRight(url, "/")+path, bytes.NewReader(payload))
	if err != nil {
		response.Fail(c, 502, err.Error())
		return
	}
	req.Header.Set("Content-Type", "application/json")
	resp, err := client.Do(req)
	if err != nil {
		response.Fail(c, 502, "RAG 服务不可达: "+err.Error())
		return
	}
	defer resp.Body.Close()
	body, _ := io.ReadAll(resp.Body)
	if resp.StatusCode/100 != 2 {
		response.Fail(c, 502, string(body))
		return
	}
	var result any
	if err := json.Unmarshal(body, &result); err != nil {
		response.Fail(c, 502, "RAG 服务返回非法 JSON")
		return
	}
	response.OK(c, result)
}

// proxyDelete 向 RAG Python 服务发 DELETE 请求。
func proxyDelete(c *gin.Context, path string) {
	url := ragServiceURL()
	if url == "" {
		response.Fail(c, 400, "未配置 rag.embedding_service_url")
		return
	}
	client := &http.Client{Timeout: 8 * time.Second}
	req, err := http.NewRequestWithContext(c.Request.Context(), http.MethodDelete,
		strings.TrimRight(url, "/")+path, nil)
	if err != nil {
		response.Fail(c, 502, err.Error())
		return
	}
	resp, err := client.Do(req)
	if err != nil {
		response.Fail(c, 502, "RAG 服务不可达: "+err.Error())
		return
	}
	defer resp.Body.Close()
	body, _ := io.ReadAll(resp.Body)
	if resp.StatusCode/100 != 2 {
		response.Fail(c, 502, string(body))
		return
	}
	var result any
	if err := json.Unmarshal(body, &result); err != nil {
		response.Fail(c, 502, "RAG 服务返回非法 JSON")
		return
	}
	response.OK(c, result)
}

// Status GET /api/admin/rag/status — 合并后端开关、RAG 服务健康与「句向量模型」说明（非对话 LLM）。
func (h *AdminRagHandler) Status(c *gin.Context) {
	out := gin.H{
		"ragEnabled":              false,
		"embeddingServiceUrl":     "",
		"serviceReachable":        false,
		"embedModel":              "",
		"embedDim":                0,
		"milvusUri":               "",
		"collection":              "",
		"note":                    "智能对话检索使用的是本地句向量模型（Sentence-Transformers），与「大模型配置」中的对话 API（如 DeepSeek）不是同一个模型。",
		"syncIntervalSecondsHint": 120,
	}
	var url string
	if config.Cfg != nil {
		out["ragEnabled"] = config.Cfg.RAG.Enabled
		url = strings.TrimSpace(config.Cfg.RAG.EmbeddingServiceURL)
		out["embeddingServiceUrl"] = url
	}
	if url == "" {
		response.OK(c, out)
		return
	}
	client := &http.Client{Timeout: 5 * time.Second}
	healthURL := strings.TrimRight(url, "/") + "/health"
	req, err := http.NewRequestWithContext(c.Request.Context(), http.MethodGet, healthURL, nil)
	if err != nil {
		response.OK(c, out)
		return
	}
	resp, err := client.Do(req)
	if err != nil {
		out["serviceError"] = err.Error()
		response.OK(c, out)
		return
	}
	defer resp.Body.Close()
	b, _ := io.ReadAll(resp.Body)
	if resp.StatusCode/100 != 2 {
		out["serviceError"] = string(b)
		response.OK(c, out)
		return
	}
	out["serviceReachable"] = true
	var remote map[string]any
	if json.Unmarshal(b, &remote) == nil {
		if v, ok := remote["embed_model"].(string); ok {
			out["embedModel"] = v
		}
		if v, ok := remote["embed_dimension"].(float64); ok {
			out["embedDim"] = int(v)
		}
		if v, ok := remote["milvus_uri"].(string); ok {
			out["milvusUri"] = v
		}
		if v, ok := remote["collection"].(string); ok {
			out["collection"] = v
		}
		if v, ok := remote["sync_interval_sec"].(float64); ok {
			out["syncIntervalSecondsHint"] = int(v)
		}
	}

	// 读取 sync_enabled 开关
	var ss model.SystemSetting
	if h.db.Where("`key` = ?", "rag.sync_enabled").Limit(1).Find(&ss).Error == nil && ss.Key != "" {
		out["syncEnabled"] = strings.ToLower(strings.TrimSpace(ss.Value)) == "true"
	} else {
		out["syncEnabled"] = true
	}

	response.OK(c, out)
}

// ListRuns GET /api/admin/rag/runs
func (h *AdminRagHandler) ListRuns(c *gin.Context) {
	page, _ := strconv.Atoi(c.DefaultQuery("page", "1"))
	pageSize, _ := strconv.Atoi(c.DefaultQuery("pageSize", "10"))
	if page < 1 {
		page = 1
	}
	if pageSize < 1 || pageSize > 50 {
		pageSize = 10
	}
	var total int64
	if err := h.db.Model(&model.RagSyncLog{}).Count(&total).Error; err != nil {
		response.ServerError(c)
		return
	}
	var logs []model.RagSyncLog
	offset := (page - 1) * pageSize
	if err := h.db.Order("id DESC").Offset(offset).Limit(pageSize).Find(&logs).Error; err != nil {
		response.ServerError(c)
		return
	}
	response.OKPage(c, total, logs)
}

// TriggerSync POST /api/admin/rag/sync — 创建一条执行记录并通知 Python 后台执行（异步）。
func (h *AdminRagHandler) TriggerSync(c *gin.Context) {
	if config.Cfg == nil {
		response.Fail(c, 500, "配置未加载")
		return
	}
	url := strings.TrimSpace(config.Cfg.RAG.EmbeddingServiceURL)
	if url == "" {
		response.Fail(c, 400, "未配置 rag.embedding_service_url")
		return
	}
	logRow := model.RagSyncLog{
		Status:    "running",
		Progress:  0,
		Mode:      "manual",
		StartedAt: time.Now(),
	}
	if err := h.db.Create(&logRow).Error; err != nil {
		response.ServerError(c)
		return
	}

	payload, _ := json.Marshal(map[string]any{"sync_log_id": logRow.ID, "async": true})
	client := &http.Client{Timeout: 8 * time.Second}
	req, err := http.NewRequestWithContext(
		c.Request.Context(),
		http.MethodPost,
		strings.TrimRight(url, "/")+"/v1/sync",
		bytes.NewReader(payload),
	)
	if err != nil {
		now := time.Now()
		h.db.Model(&logRow).Updates(map[string]any{
			"status": "failed", "message": err.Error(), "finished_at": now,
		})
		response.Fail(c, 502, "发起同步失败: "+err.Error())
		return
	}
	req.Header.Set("Content-Type", "application/json")
	resp, err := client.Do(req)
	if err != nil {
		now := time.Now()
		h.db.Model(&logRow).Updates(map[string]any{
			"status": "failed", "message": err.Error(), "finished_at": now,
		})
		response.Fail(c, 502, "RAG 服务不可达: "+err.Error())
		return
	}
	defer resp.Body.Close()
	body, _ := io.ReadAll(resp.Body)
	if resp.StatusCode/100 != 2 {
		now := time.Now()
		h.db.Model(&logRow).Updates(map[string]any{
			"status": "failed", "message": string(body), "finished_at": now,
		})
		response.Fail(c, 502, "RAG 服务拒绝: "+string(body))
		return
	}

	response.OK(c, gin.H{
		"syncLogId": logRow.ID,
		"message":   "已提交同步任务，请在下方列表查看进度",
		"raw":       json.RawMessage(body),
	})
}

// GetConfig GET /api/admin/rag/config — 读取向量同步开关配置（代理到 Python 服务）。
func (h *AdminRagHandler) GetConfig(c *gin.Context) {
	proxyGet(c, "/v1/rag-config")
}

// UpdateConfig PUT /api/admin/rag/config — 更新向量同步开关配置（代理到 Python 服务）。
func (h *AdminRagHandler) UpdateConfig(c *gin.Context) {
	proxyPut(c, "/v1/rag-config")
}

// ListKBArticles GET /api/admin/rag/articles — 列出文章向量同步状态（代理到 Python 服务）。
func (h *AdminRagHandler) ListKBArticles(c *gin.Context) {
	url := ragServiceURL()
	if url == "" {
		response.Fail(c, 400, "未配置 rag.embedding_service_url")
		return
	}
	// 透传查询参数
	params := c.Request.URL.Query()
	target := strings.TrimRight(url, "/") + "/v1/articles?" + params.Encode()
	client := &http.Client{Timeout: 10 * time.Second}
	req, err := http.NewRequestWithContext(c.Request.Context(), http.MethodGet, target, nil)
	if err != nil {
		response.Fail(c, 502, err.Error())
		return
	}
	resp, err := client.Do(req)
	if err != nil {
		response.Fail(c, 502, "RAG 服务不可达: "+err.Error())
		return
	}
	defer resp.Body.Close()
	body, _ := io.ReadAll(resp.Body)
	if resp.StatusCode/100 != 2 {
		response.Fail(c, 502, string(body))
		return
	}
	var result any
	if err := json.Unmarshal(body, &result); err != nil {
		response.Fail(c, 502, "RAG 服务返回非法 JSON")
		return
	}
	response.OK(c, result)
}

// DeleteArticleEmbedding DELETE /api/admin/rag/articles/:id/embedding — 删除单篇文章的向量。
func (h *AdminRagHandler) DeleteArticleEmbedding(c *gin.Context) {
	id := c.Param("id")
	proxyDelete(c, fmt.Sprintf("/v1/articles/%s/embedding", id))
}
