package user

import (
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"strings"
	"time"

	"github.com/gin-gonic/gin"
	"opinion-analysis/pkg/response"
	"opinion-analysis/src/model"
	"opinion-analysis/src/service/rag"
	"opinion-analysis/src/service/tagger"
	"opinion-analysis/src/service/tagger/pipeline"
)

// deepChatReq extends sessionChatReq with deep-think specific fields.
type deepChatReq struct {
	SessionID    *uint    `json:"sessionId"`
	Content      string   `json:"content"`
	PageHint     string   `json:"pageHint"`
	UseRAG       *bool    `json:"useRag"`
	WebSearch    *bool    `json:"webSearch"`
	Topics       []string `json:"topics"`
	IsRegenerate bool     `json:"isRegenerate"`
}

// Capabilities 返回深度思考模式下用户可用的能力开关，供前端决定是否展示对应入口。
// 目前包含联网搜索：仅当管理员启用总开关且已配置 API Key 时为 true。
func (h *ChatSessionHandler) Capabilities(c *gin.Context) {
	webSearch := false
	if h.taggerSvc != nil {
		cfg, _ := h.taggerSvc.GetConfig()
		// 联网搜索可用 = 管理员启用总开关 且 已配置博查 API Key。
		webSearch = cfg.WebSearchEnabled && strings.TrimSpace(cfg.WebSearchApiKey) != ""
	}
	response.OK(c, gin.H{
		"webSearch": webSearch,
	})
}

// DeepChat is the SSE handler for the /chat/deep endpoint.
// It runs the deep-thinking pipeline and emits think_step + content events.
//
// SSE event types emitted:
//
//	event: session    — session info (same as normal chat)
//	event: think_step — pipeline step progress
//	data: {...}       — incremental content chunks (no event: prefix = default)
//	event: done       — completion
//	event: error      — error (terminal)
func (h *ChatSessionHandler) DeepChat(c *gin.Context) {
	if h.taggerSvc == nil {
		response.ServerError(c)
		return
	}
	uid, ok := CurrentUserID(c)
	if !ok {
		response.ServerError(c)
		return
	}

	var req deepChatReq
	if err := c.ShouldBindJSON(&req); err != nil {
		response.Fail(c, 400, "invalid json")
		return
	}
	content := strings.TrimSpace(req.Content)
	if content == "" {
		response.Fail(c, 400, "content required")
		return
	}

	// Resolve or create session
	var session model.ChatSession
	if req.SessionID != nil {
		s, err := h.chat.FindSessionForUser(*req.SessionID, uid)
		if err != nil {
			response.Fail(c, 404, "会话不存在")
			return
		}
		session = *s
	} else {
		session = model.ChatSession{UserID: uid, Title: sessionTitle(content)}
		if err := h.chat.CreateSession(&session); err != nil {
			response.ServerError(c)
			return
		}
	}

	dbMsgs, err := h.chat.ListMessages(session.ID)
	if err != nil {
		response.ServerError(c)
		return
	}

	hist := make([]tagger.ChatMessage, 0, len(dbMsgs))
	for _, m := range dbMsgs {
		hist = append(hist, tagger.ChatMessage{Role: m.Role, Content: m.Content})
	}

	useRAG := true
	if req.UseRAG != nil {
		useRAG = *req.UseRAG
	}
	var ragClient *rag.Client
	if useRAG {
		ragClient = h.ragClient
	}

	// 用户本次是否请求联网搜索；实际是否生效还取决于管理员后台总开关与 Key（在 pipeline 内判定）。
	webSearch := false
	if req.WebSearch != nil {
		webSearch = *req.WebSearch
	}

	// Set SSE headers
	c.Header("Content-Type", "text/event-stream")
	c.Header("Cache-Control", "no-cache")
	c.Header("Connection", "keep-alive")
	c.Header("X-Accel-Buffering", "no")

	flusher, ok := c.Writer.(http.Flusher)
	if !ok {
		response.ServerError(c)
		return
	}

	// Emit session event
	sessionInfo := map[string]interface{}{
		"sessionId": session.ID,
		"title":     session.Title,
		"ragUsed":   useRAG && ragClient != nil,
		"deepThink": true,
	}
	sessionJSON, _ := json.Marshal(sessionInfo)
	fmt.Fprintf(c.Writer, "event: session\ndata: %s\n\n", sessionJSON)
	flusher.Flush()

	result := h.taggerSvc.RunDeepPipeline(
		c.Request.Context(),
		hist,
		content,
		strings.TrimSpace(req.PageHint),
		req.Topics,
		ragClient,
		webSearch,
	)

	var fullReply strings.Builder

	for {
		select {
		// Pipeline step events
		case step, ok := <-result.StepCh:
			if !ok {
				result.StepCh = nil
				continue
			}
			stepJSON, _ := json.Marshal(step)
			fmt.Fprintf(c.Writer, "event: think_step\ndata: %s\n\n", stepJSON)
			flusher.Flush()

		// Content chunks from the generation node
		case chunk, ok := <-result.ContentCh:
			if !ok {
				goto StreamEnd
			}
			fullReply.WriteString(chunk)
			chunkData := map[string]string{"content": chunk}
			chunkJSON, _ := json.Marshal(chunkData)
			fmt.Fprintf(c.Writer, "data: %s\n\n", chunkJSON)
			flusher.Flush()

		// Error from any node
		case err := <-result.ErrCh:
			if err != nil {
				errMsg := err.Error()
				if strings.Contains(errMsg, "api key not configured") {
					errMsg = "大模型未配置：请在管理后台「系统状态」中配置 API Key"
				} else {
					errMsg = "深度思考失败：" + TruncateErrMsg(errMsg, 200)
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

StreamEnd:
	// Drain remaining step events before closing (the generate node emits done)
	for len(result.StepCh) > 0 {
		step := <-result.StepCh
		stepJSON, _ := json.Marshal(step)
		fmt.Fprintf(c.Writer, "event: think_step\ndata: %s\n\n", stepJSON)
	}
	flusher.Flush()

	// Persist messages
	reply := fullReply.String()
	if !req.IsRegenerate {
		_ = h.chat.CreateMessage(&model.ChatMessage{SessionID: session.ID, Role: "user", Content: content})
	}
	_ = h.chat.CreateMessage(&model.ChatMessage{SessionID: session.ID, Role: "assistant", Content: reply})

	outTitle := session.Title
	updates := map[string]interface{}{"updated_at": time.Now()}
	if req.SessionID != nil && len(dbMsgs) == 0 {
		t := sessionTitle(content)
		updates["title"] = t
		outTitle = t
	}
	if err := h.chat.UpdateSession(session.ID, updates); err != nil {
		log.Printf("[deep-chat] bump session: %v", err)
	}

	doneData := map[string]interface{}{
		"done":  true,
		"title": outTitle,
	}
	doneJSON, _ := json.Marshal(doneData)
	fmt.Fprintf(c.Writer, "event: done\ndata: %s\n\n", doneJSON)
	flusher.Flush()

	_ = pipeline.StatusDone // ensure import used
}
