package admin

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/gin-gonic/gin"
	"opinion-analysis/config"
	"opinion-analysis/pkg/response"
	"opinion-analysis/src/model"
	"opinion-analysis/src/repository"
	"opinion-analysis/src/service/ragprocess"
)

type RAGHandler struct {
	rag  *repository.RAGRepository
	proc *ragprocess.Manager
}

func NewRAGHandler(store *repository.Store, proc *ragprocess.Manager) *RAGHandler {
	return &RAGHandler{rag: store.RAG, proc: proc}
}

func extractRagErrorDetail(body []byte) string {
	s := strings.TrimSpace(string(body))
	if s == "" {
		return "RAG 服务错误"
	}
	var obj map[string]any
	if json.Unmarshal(body, &obj) == nil {
		if d, ok := obj["detail"].(string); ok && d != "" {
			return d
		}
	}
	return s
}

func ragServiceURL() string {
	if config.Cfg == nil {
		return ""
	}
	return strings.TrimSpace(config.Cfg.RAG.EmbeddingServiceURL)
}

func proxyRagPost(c *gin.Context, path string, payload any, timeout time.Duration) (int, []byte, error) {
	url := ragServiceURL()
	if url == "" {
		return 0, nil, fmt.Errorf("未配置 rag.embedding_service_url")
	}
	var bodyReader io.Reader
	if payload != nil {
		b, err := json.Marshal(payload)
		if err != nil {
			return 0, nil, err
		}
		bodyReader = bytes.NewReader(b)
	}
	client := &http.Client{Timeout: timeout}
	req, err := http.NewRequestWithContext(
		c.Request.Context(),
		http.MethodPost,
		strings.TrimRight(url, "/")+path,
		bodyReader,
	)
	if err != nil {
		return 0, nil, err
	}
	if payload != nil {
		req.Header.Set("Content-Type", "application/json")
	}
	resp, err := client.Do(req)
	if err != nil {
		return 0, nil, err
	}
	defer resp.Body.Close()
	body, _ := io.ReadAll(resp.Body)
	return resp.StatusCode, body, nil
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
func (h *RAGHandler) Status(c *gin.Context) {
	out := gin.H{
		"ragEnabled":              false,
		"embeddingServiceUrl":     "",
		"serviceReachable":        false,
		"embedModel":              "",
		"embedDim":                0,
		"milvusUri":               "",
		"collection":              "",
		"note":                    "句向量用于 RAG 检索，与下方「大模型配置」中的对话 API 不是同一个模型；支持本地 Sentence-Transformers 或 OpenAI 兼容 Embedding API。",
		"syncIntervalSecondsHint": 120,
	}
	var url string
	if config.Cfg != nil {
		out["ragEnabled"] = config.Cfg.RAG.Enabled
		url = strings.TrimSpace(config.Cfg.RAG.EmbeddingServiceURL)
		out["embeddingServiceUrl"] = url
	}
	if h.proc != nil {
		out["processManaged"] = h.proc.ManagedEnabled()
		out["processRunning"] = h.proc.IsRunning()
		if pid := h.proc.PID(); pid > 0 {
			out["processPid"] = pid
		}
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
		if v, ok := remote["embed_provider"].(string); ok {
			out["embedProvider"] = v
		}
		if v, ok := remote["embed_model"].(string); ok {
			out["embedModel"] = v
		}
		if v, ok := remote["embed_dimension"].(float64); ok {
			out["embedDim"] = int(v)
		}
		if v, ok := remote["collection_dimension"].(float64); ok {
			out["collectionDim"] = int(v)
		}
		if v, ok := remote["dimension_mismatch"].(bool); ok {
			out["dimensionMismatch"] = v
		}
		if v, ok := remote["embedder_ready"].(bool); ok {
			out["embedderReady"] = v
		}
		if v, ok := remote["embedder_error"].(string); ok && v != "" {
			out["embedderError"] = v
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
	if ss, err := h.rag.GetSyncEnabledSetting(); err == nil && ss != nil {
		out["syncEnabled"] = strings.ToLower(strings.TrimSpace(ss.Value)) == "true"
	} else {
		out["syncEnabled"] = true
	}

	// RAG 不可达时仍从数据库展示当前配置，便于后台修复错误配置
	if !out["serviceReachable"].(bool) {
		if cfg, err := h.rag.GetRagConfig(); err == nil {
			if out["embedModel"] == "" {
				out["embedModel"] = cfg.EmbedModel
			}
			if out["embedProvider"] == nil || out["embedProvider"] == "" {
				out["embedProvider"] = cfg.EmbedProvider
			}
			out["syncIntervalSecondsHint"] = cfg.SyncIntervalSec
		}
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

func (h *RAGHandler) TriggerSync(c *gin.Context) {
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
	if err := h.rag.CreateSyncLog(&logRow); err != nil {
		response.ServerError(c)
		return
	}

	payload, _ := json.Marshal(map[string]any{"sync_log_id": logRow.ID, "async": true})
	client := &http.Client{Timeout: 30 * time.Second}
	req, err := http.NewRequestWithContext(
		c.Request.Context(),
		http.MethodPost,
		strings.TrimRight(url, "/")+"/v1/sync",
		bytes.NewReader(payload),
	)
	if err != nil {
		_ = h.rag.FailSyncLog(&logRow, err.Error())
		response.Fail(c, 502, "发起同步失败: "+err.Error())
		return
	}
	req.Header.Set("Content-Type", "application/json")
	resp, err := client.Do(req)
	if err != nil {
		_ = h.rag.FailSyncLog(&logRow, err.Error())
		response.Fail(c, 502, "RAG 服务不可达: "+err.Error())
		return
	}
	defer resp.Body.Close()
	body, _ := io.ReadAll(resp.Body)
	if resp.StatusCode == 409 {
		msg := extractRagErrorDetail(body)
		_ = h.rag.FailSyncLog(&logRow, msg)
		response.Fail(c, 409, msg)
		return
	}
	if resp.StatusCode/100 != 2 {
		_ = h.rag.FailSyncLog(&logRow, string(body))
		response.Fail(c, 502, "RAG 服务拒绝: "+extractRagErrorDetail(body))
		return
	}

	response.OK(c, gin.H{
		"syncLogId": logRow.ID,
		"message":   "已提交同步任务，请在下方列表查看进度",
		"raw":       json.RawMessage(body),
	})
}

// GetConfig GET /api/admin/rag/config — 从数据库读取；RAG 可达时合并 env_overrides。
func (h *RAGHandler) GetConfig(c *gin.Context) {
	cfg, err := h.rag.GetRagConfig()
	if err != nil {
		response.Fail(c, 500, "读取配置失败: "+err.Error())
		return
	}
	envOverrides := tryRagEnvOverrides()
	resp := repository.BuildRagConfigResponse(cfg, envOverrides, false, "")
	response.OK(c, resp)
}

type updateRagConfigReq struct {
	SyncEnabled      *bool   `json:"sync_enabled"`
	EmbedProvider    *string `json:"embed_provider"`
	EmbedModel       *string `json:"embed_model"`
	EmbedAPIBase     *string `json:"embed_api_base"`
	EmbedAPIKey      *string `json:"embed_api_key"`
	ChunkMaxChars    *int    `json:"chunk_max_chars"`
	ChunkOverlap     *int    `json:"chunk_overlap"`
	SyncIntervalSec  *int    `json:"sync_interval_sec"`
	SyncBatch        *int    `json:"sync_batch"`
}

func pickString(raw map[string]json.RawMessage, keys ...string) (string, bool) {
	for _, k := range keys {
		if v, ok := raw[k]; ok {
			var s string
			if err := json.Unmarshal(v, &s); err == nil {
				return s, true
			}
		}
	}
	return "", false
}

func pickBool(raw map[string]json.RawMessage, keys ...string) (*bool, bool) {
	for _, k := range keys {
		if v, ok := raw[k]; ok {
			var b bool
			if err := json.Unmarshal(v, &b); err == nil {
				return &b, true
			}
		}
	}
	return nil, false
}

func pickInt(raw map[string]json.RawMessage, keys ...string) (*int, bool) {
	for _, k := range keys {
		if v, ok := raw[k]; ok {
			var n int
			if err := json.Unmarshal(v, &n); err == nil {
				return &n, true
			}
			var f float64
			if err := json.Unmarshal(v, &f); err == nil {
				i := int(f)
				return &i, true
			}
		}
	}
	return nil, false
}

func tryRagEnvOverrides() []string {
	// 与 Python _RAG_SETTING_SPECS 环境变量名一致；Go 进程通常未设置，返回空即可。
	return nil
}

func reloadRagService(c *gin.Context) (bool, string, []string) {
	url := ragServiceURL()
	if url == "" {
		return false, "未配置 rag.embedding_service_url，已仅写入数据库", nil
	}
	client := &http.Client{Timeout: 15 * time.Second}
	req, err := http.NewRequestWithContext(c.Request.Context(), http.MethodPost,
		strings.TrimRight(url, "/")+"/v1/rag-config/reload", nil)
	if err != nil {
		return false, "RAG 热更新失败: " + err.Error(), nil
	}
	resp, err := client.Do(req)
	if err != nil {
		return false, "RAG 服务暂不可达，配置已写入数据库，请重启 RAG 服务后生效", nil
	}
	defer resp.Body.Close()
	body, _ := io.ReadAll(resp.Body)
	if resp.StatusCode/100 != 2 {
		return false, "RAG 热更新失败: " + string(body), nil
	}
	var remote map[string]any
	_ = json.Unmarshal(body, &remote)
	var warnings []string
	if ws, ok := remote["warnings"].([]any); ok {
		for _, w := range ws {
			if s, ok := w.(string); ok && s != "" {
				warnings = append(warnings, s)
			}
		}
	}
	return true, "", warnings
}

// UpdateConfig PUT /api/admin/rag/config — 写入数据库并尽力热更新 RAG 服务。
func (h *RAGHandler) UpdateConfig(c *gin.Context) {
	payload, err := io.ReadAll(c.Request.Body)
	if err != nil {
		response.Fail(c, 400, "读请求体失败")
		return
	}
	var req updateRagConfigReq
	_ = json.Unmarshal(payload, &req)
	var raw map[string]json.RawMessage
	_ = json.Unmarshal(payload, &raw)

	cur, err := h.rag.GetRagConfig()
	if err != nil {
		response.Fail(c, 500, "读取当前配置失败: "+err.Error())
		return
	}
	merged := cur

	if req.SyncEnabled != nil {
		merged.SyncEnabled = *req.SyncEnabled
	} else if v, ok := pickBool(raw, "syncEnabled"); ok && v != nil {
		merged.SyncEnabled = *v
	}
	if req.EmbedProvider != nil {
		merged.EmbedProvider = strings.TrimSpace(*req.EmbedProvider)
	} else if s, ok := pickString(raw, "embedProvider"); ok {
		merged.EmbedProvider = strings.TrimSpace(s)
	}
	if req.EmbedModel != nil {
		merged.EmbedModel = strings.TrimSpace(*req.EmbedModel)
	} else if s, ok := pickString(raw, "embedModel"); ok {
		merged.EmbedModel = strings.TrimSpace(s)
	}
	if req.EmbedAPIBase != nil {
		merged.EmbedAPIBase = strings.TrimSpace(*req.EmbedAPIBase)
	} else if s, ok := pickString(raw, "embedApiBase"); ok {
		merged.EmbedAPIBase = strings.TrimSpace(s)
	}
	if v, ok := raw["embed_api_key"]; ok {
		var s string
		if err := json.Unmarshal(v, &s); err == nil {
			merged.EmbedAPIKey = strings.TrimSpace(s)
		}
	} else if v, ok := raw["embedApiKey"]; ok {
		var s string
		if err := json.Unmarshal(v, &s); err == nil {
			merged.EmbedAPIKey = strings.TrimSpace(s)
		}
	}
	if req.ChunkMaxChars != nil {
		merged.ChunkMaxChars = *req.ChunkMaxChars
	} else if v, ok := pickInt(raw, "chunkMaxChars"); ok && v != nil {
		merged.ChunkMaxChars = *v
	}
	if req.ChunkOverlap != nil {
		merged.ChunkOverlap = *req.ChunkOverlap
	} else if v, ok := pickInt(raw, "chunkOverlap"); ok && v != nil {
		merged.ChunkOverlap = *v
	}
	if req.SyncIntervalSec != nil {
		merged.SyncIntervalSec = *req.SyncIntervalSec
	} else if v, ok := pickInt(raw, "syncIntervalSec"); ok && v != nil {
		merged.SyncIntervalSec = *v
	}
	if req.SyncBatch != nil {
		merged.SyncBatch = *req.SyncBatch
	} else if v, ok := pickInt(raw, "syncBatch"); ok && v != nil {
		merged.SyncBatch = *v
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
	if p == "api" {
		if merged.EmbedAPIBase == "" {
			response.Fail(c, 400, "使用 API 模式时必须填写 embed_api_base")
			return
		}
		if strings.TrimSpace(merged.EmbedAPIKey) == "" && strings.TrimSpace(cur.EmbedAPIKey) == "" {
			response.Fail(c, 400, "使用 API 模式时必须填写 embed_api_key")
			return
		}
		if strings.TrimSpace(merged.EmbedAPIKey) == "" {
			merged.EmbedAPIKey = cur.EmbedAPIKey
		}
	}
	if merged.ChunkMaxChars < 128 || merged.ChunkMaxChars > 2000 {
		response.Fail(c, 400, "chunk_max_chars 应在 128~2000")
		return
	}
	if merged.ChunkOverlap < 0 || merged.ChunkOverlap > 500 {
		response.Fail(c, 400, "chunk_overlap 应在 0~500")
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
		if id, ok := uid.(uint); ok {
			cu = id
		}
	}
	actorName := ""
	if uname, ok := c.Get("username"); ok {
		if name, ok := uname.(string); ok {
			actorName = name
		}
	}

	if err := h.rag.SaveRagConfig(merged, cu, actorName); err != nil {
		response.Fail(c, 500, "持久化失败: "+err.Error())
		return
	}

	applied, warn, pyWarnings := reloadRagService(c)
	if len(pyWarnings) > 0 {
		warn = strings.Join(pyWarnings, "；")
	}
	resp := repository.BuildRagConfigResponse(merged, tryRagEnvOverrides(), applied, warn)
	resp.Warnings = pyWarnings
	response.OK(c, resp)
}

// RebuildMilvus POST /api/admin/rag/milvus/rebuild — 重建 Milvus 集合并清空同步标记。
func (h *RAGHandler) RebuildMilvus(c *gin.Context) {
	status, body, err := proxyRagPost(c, "/v1/milvus/rebuild", nil, 120*time.Second)
	if err != nil {
		response.Fail(c, 502, err.Error())
		return
	}
	if status/100 != 2 {
		response.Fail(c, status, extractRagErrorDetail(body))
		return
	}
	var result map[string]any
	if err := json.Unmarshal(body, &result); err != nil {
		response.Fail(c, 502, "RAG 服务返回非法 JSON")
		return
	}
	response.OK(c, result)
}

// RestartService POST /api/admin/rag/restart — 杀旧进程并拉起 RAG 子进程（快速返回，前端轮询 status）。
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
	response.OK(c, result)
}

// ListKBArticles GET /api/admin/rag/articles — 列出文章向量同步状态（代理到 Python 服务）。
func (h *RAGHandler) ListKBArticles(c *gin.Context) {
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
func (h *RAGHandler) DeleteArticleEmbedding(c *gin.Context) {
	id := c.Param("id")
	proxyDelete(c, fmt.Sprintf("/v1/articles/%s/embedding", id))
}
