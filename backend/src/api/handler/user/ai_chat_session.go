package user

import (
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/gin-gonic/gin"
	"opinion-analysis/config"
	"opinion-analysis/pkg/response"
	"opinion-analysis/src/model"
	"opinion-analysis/src/repository"
	"opinion-analysis/src/service/rag"
	"opinion-analysis/src/service/tagger"
)

type ChatSessionHandler struct {
	chat      *repository.ChatRepository
	taggerSvc *tagger.Service
	ragClient *rag.Client
}

func NewChatSessionHandler(store *repository.Store, taggerSvc *tagger.Service) *ChatSessionHandler {
	h := &ChatSessionHandler{chat: store.Chat, taggerSvc: taggerSvc}
	if config.Cfg != nil && config.Cfg.RAG.Enabled && strings.TrimSpace(config.Cfg.RAG.EmbeddingServiceURL) != "" {
		h.ragClient = &rag.Client{BaseURL: strings.TrimSpace(config.Cfg.RAG.EmbeddingServiceURL)}
	}
	return h
}

func (h *ChatSessionHandler) CreateSession(c *gin.Context) {
	uid, ok := CurrentUserID(c)
	if !ok {
		response.ServerError(c)
		return
	}
	var req struct {
		Title string `json:"title"`
	}
	_ = c.ShouldBindJSON(&req)
	title := strings.TrimSpace(req.Title)
	if title == "" {
		title = "新对话"
	}
	session := model.ChatSession{UserID: uid, Title: title}
	if err := h.chat.CreateSession(&session); err != nil {
		response.ServerError(c)
		return
	}
	response.OK(c, gin.H{"session": session})
}

func (h *ChatSessionHandler) ListSessions(c *gin.Context) {
	uid, ok := CurrentUserID(c)
	if !ok {
		response.ServerError(c)
		return
	}
	sessions, err := h.chat.ListSessionsByUser(uid, 100)
	if err != nil {
		response.ServerError(c)
		return
	}
	response.OK(c, gin.H{"list": sessions})
}

func (h *ChatSessionHandler) GetSession(c *gin.Context) {
	uid, ok := CurrentUserID(c)
	if !ok {
		response.ServerError(c)
		return
	}
	sessionID, _ := strconv.ParseUint(c.Param("id"), 10, 32)
	session, err := h.chat.FindSessionForUser(uint(sessionID), uid)
	if err != nil {
		response.Fail(c, 404, "会话不存在")
		return
	}
	messages, err := h.chat.ListMessages(session.ID)
	if err != nil {
		response.ServerError(c)
		return
	}
	response.OK(c, gin.H{"session": session, "messages": messages})
}

func (h *ChatSessionHandler) DeleteSession(c *gin.Context) {
	uid, ok := CurrentUserID(c)
	if !ok {
		response.ServerError(c)
		return
	}
	sessionID, _ := strconv.ParseUint(c.Param("id"), 10, 32)
	session, err := h.chat.FindSessionForUser(uint(sessionID), uid)
	if err != nil {
		response.Fail(c, 404, "会话不存在")
		return
	}
	_ = h.chat.DeleteMessagesBySession(session.ID)
	_ = h.chat.DeleteSession(session)
	response.OK(c, nil)
}

func (h *ChatSessionHandler) RenameSession(c *gin.Context) {
	uid, ok := CurrentUserID(c)
	if !ok {
		response.ServerError(c)
		return
	}
	var req struct {
		Title string `json:"title"`
	}
	if err := c.ShouldBindJSON(&req); err != nil || strings.TrimSpace(req.Title) == "" {
		response.Fail(c, 400, "title required")
		return
	}
	sessionID, _ := strconv.ParseUint(c.Param("id"), 10, 32)
	rows, err := h.chat.RenameSession(uint(sessionID), uid, strings.TrimSpace(req.Title))
	if err != nil {
		response.ServerError(c)
		return
	}
	if rows == 0 {
		response.Fail(c, 404, "会话不存在")
		return
	}
	response.OK(c, nil)
}

type sessionChatReq struct {
	SessionID *uint  `json:"sessionId"`
	Content   string `json:"content"`
	PageHint  string `json:"pageHint"`
	UseRAG    *bool  `json:"useRag"`
}

func sessionTitle(content string) string {
	r := []rune(strings.TrimSpace(content))
	if len(r) <= 36 {
		return string(r)
	}
	return string(r[:36]) + "…"
}

func (h *ChatSessionHandler) Chat(c *gin.Context) {
	if h.taggerSvc == nil {
		response.ServerError(c)
		return
	}
	uid, ok := CurrentUserID(c)
	if !ok {
		response.ServerError(c)
		return
	}

	var req sessionChatReq
	if err := c.ShouldBindJSON(&req); err != nil {
		response.Fail(c, 400, "invalid json")
		return
	}
	content := strings.TrimSpace(req.Content)
	if content == "" {
		response.Fail(c, 400, "content required")
		return
	}

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

	hist := make([]tagger.ChatMessage, 0, len(dbMsgs)+1)
	for _, m := range dbMsgs {
		hist = append(hist, tagger.ChatMessage{Role: m.Role, Content: m.Content})
	}
	hist = append(hist, tagger.ChatMessage{Role: "user", Content: content})

	useRAG := true
	if req.UseRAG != nil {
		useRAG = *req.UseRAG
	}
	var retrievalCtx string
	var ragUsed bool
	if useRAG && h.ragClient != nil {
		chunks, err := h.ragClient.Search(c.Request.Context(), content, 8)
		if err != nil {
			log.Printf("[session-chat] RAG error: %v", err)
		} else if len(chunks) > 0 {
			retrievalCtx = rag.FormatContext(chunks)
			ragUsed = true
			log.Printf("[session-chat] RAG returned %d chunks for query", len(chunks))
		} else {
			log.Printf("[session-chat] RAG returned 0 chunks, falling back to general knowledge")
		}
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

	// 先发送会话信息
	sessionInfo := map[string]interface{}{
		"sessionId": session.ID,
		"title":     session.Title,
		"ragUsed":   ragUsed,
	}
	sessionJSON, _ := json.Marshal(sessionInfo)
	fmt.Fprintf(c.Writer, "event: session\ndata: %s\n\n", sessionJSON)
	flusher.Flush()

	// 流式调用 LLM
	contentCh, errCh := h.taggerSvc.ChatCompletionStream(
		c.Request.Context(), hist, strings.TrimSpace(req.PageHint), retrievalCtx,
	)

	var fullReply strings.Builder
	for {
		select {
		case chunk, ok := <-contentCh:
			if !ok {
				// 流结束
				goto StreamEnd
			}
			fullReply.WriteString(chunk)
			// 发送增量内容
			chunkData := map[string]string{"content": chunk}
			chunkJSON, _ := json.Marshal(chunkData)
			fmt.Fprintf(c.Writer, "data: %s\n\n", chunkJSON)
			flusher.Flush()

		case err := <-errCh:
			if err != nil {
				errMsg := err.Error()
				if strings.Contains(errMsg, "api key not configured") {
					errMsg = "大模型未配置：请在管理后台「系统状态」中配置 API Key"
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

StreamEnd:
	// 保存消息到数据库
	reply := fullReply.String()
	_ = h.chat.CreateMessage(&model.ChatMessage{SessionID: session.ID, Role: "user", Content: content})
	_ = h.chat.CreateMessage(&model.ChatMessage{SessionID: session.ID, Role: "assistant", Content: reply})

	// 更新会话
	outTitle := session.Title
	updates := map[string]interface{}{"updated_at": time.Now()}
	if req.SessionID != nil && len(dbMsgs) == 0 {
		t := sessionTitle(content)
		updates["title"] = t
		outTitle = t
	}
	if err := h.chat.UpdateSession(session.ID, updates); err != nil {
		log.Printf("[session-chat] bump session: %v", err)
	}

	// 发送完成事件
	doneData := map[string]interface{}{
		"done":  true,
		"title": outTitle,
	}
	doneJSON, _ := json.Marshal(doneData)
	fmt.Fprintf(c.Writer, "event: done\ndata: %s\n\n", doneJSON)
	flusher.Flush()
}
