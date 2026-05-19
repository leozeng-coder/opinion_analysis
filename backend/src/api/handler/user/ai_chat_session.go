package user

import (
	"log"
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
	if useRAG && h.ragClient != nil {
		chunks, err := h.ragClient.Search(c.Request.Context(), content, 8)
		if err != nil {
			log.Printf("[session-chat] RAG error: %v", err)
		} else {
			retrievalCtx = rag.FormatContext(chunks)
		}
	}

	reply, err := h.taggerSvc.ChatCompletion(
		c.Request.Context(), hist, strings.TrimSpace(req.PageHint), retrievalCtx,
	)
	if err != nil {
		if strings.Contains(err.Error(), "api key not configured") {
			response.Fail(c, 503, "大模型未配置：请在管理后台「系统状态」中配置 API Key")
			return
		}
		response.Fail(c, 502, "模型调用失败："+TruncateErrMsg(err.Error(), 200))
		return
	}

	_ = h.chat.CreateMessage(&model.ChatMessage{SessionID: session.ID, Role: "user", Content: content})
	_ = h.chat.CreateMessage(&model.ChatMessage{SessionID: session.ID, Role: "assistant", Content: reply})

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

	response.OK(c, gin.H{
		"sessionId": session.ID,
		"title":     outTitle,
		"reply":     reply,
	})
}
