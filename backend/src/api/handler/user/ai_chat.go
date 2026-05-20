package user

import (
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"strings"

	"github.com/gin-gonic/gin"
	"opinion-analysis/config"
	"opinion-analysis/pkg/response"
	"opinion-analysis/src/service/rag"
	"opinion-analysis/src/service/tagger"
)

type AIChatHandler struct {
	taggerSvc *tagger.Service
	ragClient *rag.Client
}

func NewAIChatHandler(taggerSvc *tagger.Service) *AIChatHandler {
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
	UseRAG   *bool  `json:"useRag"`
}

func lastUserQuestion(hist []tagger.ChatMessage) string {
	for i := len(hist) - 1; i >= 0; i-- {
		if strings.EqualFold(strings.TrimSpace(hist[i].Role), "user") {
			return strings.TrimSpace(hist[i].Content)
		}
	}
	return ""
}

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
	var ragUsed bool
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
				ragUsed = true
			}
		}
	} else if useRAG && h.ragClient == nil {
		log.Printf("[ai-chat] RAG requested but ragClient is nil (check rag.enabled in config.yaml)")
	}

	// 设置 SSE 响应头
	c.Header("Content-Type", "text/event-stream")
	c.Header("Cache-Control", "no-cache")
	c.Header("Connection", "keep-alive")
	c.Header("X-Accel-Buffering", "no")

	flusher, ok := c.Writer.(http.Flusher)
	if !ok {
		response.ServerError(c)
		return
	}

	// 先发送元信息
	metaInfo := map[string]interface{}{
		"ragUsed": ragUsed,
	}
	metaJSON, _ := json.Marshal(metaInfo)
	fmt.Fprintf(c.Writer, "event: meta\ndata: %s\n\n", metaJSON)
	flusher.Flush()

	// 流式调用 LLM
	contentCh, errCh := h.taggerSvc.ChatCompletionStream(
		c.Request.Context(),
		hist,
		strings.TrimSpace(req.PageHint),
		retrievalCtx,
	)

	for {
		select {
		case chunk, ok := <-contentCh:
			if !ok {
				// 流结束，发送完成事件
				doneData := map[string]bool{"done": true}
				doneJSON, _ := json.Marshal(doneData)
				fmt.Fprintf(c.Writer, "event: done\ndata: %s\n\n", doneJSON)
				flusher.Flush()
				return
			}
			// 发送增量内容
			chunkData := map[string]string{"content": chunk}
			chunkJSON, _ := json.Marshal(chunkData)
			fmt.Fprintf(c.Writer, "data: %s\n\n", chunkJSON)
			flusher.Flush()

		case err := <-errCh:
			if err != nil {
				errMsg := err.Error()
				if strings.Contains(errMsg, "api key not configured") {
					errMsg = "大模型未配置：请在管理后台「系统状态」中配置 API Key，或设置环境变量 DEEPSEEK_API_KEY"
				} else {
					errMsg = "模型调用失败：" + TruncateErrMsg(errMsg, 200)
				}
				errData := map[string]string{"error": errMsg}
				errJSON, _ := json.Marshal(errData)
				fmt.Fprintf(c.Writer, "event: error\ndata: %s\n\n", errJSON)
				flusher.Flush()
				return
			}

		case <-c.Request.Context().Done():
			return
		}
	}
}
