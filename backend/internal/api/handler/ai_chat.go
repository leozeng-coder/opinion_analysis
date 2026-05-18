package handler

import (
	"log"
	"strings"

	"github.com/gin-gonic/gin"
	"gorm.io/gorm"
	"opinion-analysis/config"
	"opinion-analysis/internal/service/rag"
	"opinion-analysis/internal/service/tagger"
	"opinion-analysis/pkg/response"
)

// AIChatHandler 舆情智能对话（走与 tagger 相同的大模型配置；可选 RAG + Milvus Lite）。
type AIChatHandler struct {
	taggerSvc *tagger.Service
	ragClient *rag.Client
}

func NewAIChatHandler(_ *gorm.DB, taggerSvc *tagger.Service) *AIChatHandler {
	h := &AIChatHandler{taggerSvc: taggerSvc}
	if config.Cfg != nil && config.Cfg.RAG.Enabled && strings.TrimSpace(config.Cfg.RAG.EmbeddingServiceURL) != "" {
		h.ragClient = &rag.Client{
			BaseURL: strings.TrimSpace(config.Cfg.RAG.EmbeddingServiceURL),
		}
		log.Printf("[ai-chat] RAG enabled, embedding service: %s", config.Cfg.RAG.EmbeddingServiceURL)
	} else {
		log.Printf("[ai-chat] RAG disabled (enabled=%v url=%q)", config.Cfg != nil && config.Cfg.RAG.Enabled, func() string {
			if config.Cfg != nil {
				return config.Cfg.RAG.EmbeddingServiceURL
			}
			return ""
		}())
	}
	return h
}

type aiChatReq struct {
	Messages []struct {
		Role    string `json:"role"`
		Content string `json:"content"`
	} `json:"messages"`
	PageHint string `json:"pageHint"`
	// UseRAG 默认 true：若 RAG 未启用或服务不可用则自动跳过。
	UseRAG *bool `json:"useRag"`
}

func lastUserQuestion(hist []tagger.ChatMessage) string {
	for i := len(hist) - 1; i >= 0; i-- {
		if strings.EqualFold(strings.TrimSpace(hist[i].Role), "user") {
			return strings.TrimSpace(hist[i].Content)
		}
	}
	return ""
}

// Chat POST /api/ai/chat
func (h *AIChatHandler) Chat(c *gin.Context) {
	if h.taggerSvc == nil {
		response.ServerError(c)
		return
	}
	var req aiChatReq
	if err := c.ShouldBindJSON(&req); err != nil {
		response.Fail(c, 400, "invalid json")
		return
	}
	if len(req.Messages) == 0 {
		response.Fail(c, 400, "messages required")
		return
	}

	hist := make([]tagger.ChatMessage, 0, len(req.Messages))
	for _, m := range req.Messages {
		hist = append(hist, tagger.ChatMessage{
			Role:    strings.TrimSpace(m.Role),
			Content: m.Content,
		})
	}

	useRAG := true
	if req.UseRAG != nil {
		useRAG = *req.UseRAG
	}

	var retrievalCtx string
	if useRAG && h.ragClient != nil {
		q := lastUserQuestion(hist)
		if q != "" {
			chunks, err := h.ragClient.Search(c.Request.Context(), q, 8)
			if err != nil {
				log.Printf("[ai-chat] RAG search error: %v", err)
			} else if len(chunks) == 0 {
				log.Printf("[ai-chat] RAG search returned 0 chunks for query: %q", q)
			} else {
				log.Printf("[ai-chat] RAG search returned %d chunks for query: %q", len(chunks), q)
				retrievalCtx = rag.FormatContext(chunks)
			}
		}
	} else if useRAG && h.ragClient == nil {
		log.Printf("[ai-chat] RAG requested but ragClient is nil (check rag.enabled in config.yaml)")
	}

	reply, err := h.taggerSvc.ChatCompletion(
		c.Request.Context(),
		hist,
		strings.TrimSpace(req.PageHint),
		retrievalCtx,
	)
	if err != nil {
		if strings.Contains(err.Error(), "api key not configured") {
			response.Fail(c, 503, "大模型未配置：请在管理后台「系统状态」中配置 API Key，或设置环境变量 DEEPSEEK_API_KEY")
			return
		}
		response.Fail(c, 502, "模型调用失败："+truncateErrMsg(err.Error(), 200))
		return
	}

	response.OK(c, gin.H{"reply": reply})
}

func truncateErrMsg(s string, n int) string {
	r := []rune(s)
	if len(r) <= n {
		return s
	}
	return string(r[:n]) + "…"
}
