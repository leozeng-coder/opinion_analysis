package handler

import (
	"log"
	"strings"
	"time"

	"github.com/gin-gonic/gin"
	"gorm.io/gorm"
	"opinion-analysis/config"
	"opinion-analysis/internal/model"
	"opinion-analysis/internal/service/rag"
	"opinion-analysis/internal/service/tagger"
	"opinion-analysis/pkg/response"
)

type ChatSessionHandler struct {
	db        *gorm.DB
	taggerSvc *tagger.Service
	ragClient *rag.Client
}

func NewChatSessionHandler(db *gorm.DB, taggerSvc *tagger.Service) *ChatSessionHandler {
	h := &ChatSessionHandler{db: db, taggerSvc: taggerSvc}
	if config.Cfg != nil && config.Cfg.RAG.Enabled && strings.TrimSpace(config.Cfg.RAG.EmbeddingServiceURL) != "" {
		h.ragClient = &rag.Client{BaseURL: strings.TrimSpace(config.Cfg.RAG.EmbeddingServiceURL)}
	}
	return h
}

func currentUserID(c *gin.Context) (uint, bool) {
	v, ok := c.Get("userID")
	if !ok {
		return 0, false
	}
	uid, ok := v.(uint)
	return uid, ok
}

// POST /api/ai/sessions 创建空会话（侧栏「新对话」）
func (h *ChatSessionHandler) CreateSession(c *gin.Context) {
	uid, ok := currentUserID(c)
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
	if err := h.db.Create(&session).Error; err != nil {
		response.ServerError(c)
		return
	}
	response.OK(c, gin.H{"session": session})
}

// GET /api/ai/sessions
func (h *ChatSessionHandler) ListSessions(c *gin.Context) {
	uid, ok := currentUserID(c)
	if !ok {
		response.ServerError(c)
		return
	}
	var sessions []model.ChatSession
	h.db.Where("user_id = ?", uid).Order("updated_at DESC").Limit(100).Find(&sessions)
	response.OK(c, gin.H{"list": sessions})
}

// GET /api/ai/sessions/:id
func (h *ChatSessionHandler) GetSession(c *gin.Context) {
	uid, ok := currentUserID(c)
	if !ok {
		response.ServerError(c)
		return
	}
	var session model.ChatSession
	if err := h.db.Where("id = ? AND user_id = ?", c.Param("id"), uid).First(&session).Error; err != nil {
		response.Fail(c, 404, "会话不存在")
		return
	}
	var messages []model.ChatMessage
	h.db.Where("session_id = ?", session.ID).Order("created_at ASC").Find(&messages)
	response.OK(c, gin.H{"session": session, "messages": messages})
}

// DELETE /api/ai/sessions/:id
func (h *ChatSessionHandler) DeleteSession(c *gin.Context) {
	uid, ok := currentUserID(c)
	if !ok {
		response.ServerError(c)
		return
	}
	var session model.ChatSession
	if err := h.db.Where("id = ? AND user_id = ?", c.Param("id"), uid).First(&session).Error; err != nil {
		response.Fail(c, 404, "会话不存在")
		return
	}
	h.db.Where("session_id = ?", session.ID).Delete(&model.ChatMessage{})
	h.db.Delete(&session)
	response.OK(c, nil)
}

// PATCH /api/ai/sessions/:id
func (h *ChatSessionHandler) RenameSession(c *gin.Context) {
	uid, ok := currentUserID(c)
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
	result := h.db.Model(&model.ChatSession{}).
		Where("id = ? AND user_id = ?", c.Param("id"), uid).
		Updates(map[string]interface{}{
			"title":      strings.TrimSpace(req.Title),
			"updated_at": time.Now(),
		})
	if result.RowsAffected == 0 {
		response.Fail(c, 404, "会话不存在")
		return
	}
	response.OK(c, nil)
}

// POST /api/ai/sessions/chat
// sessionId 为 null 时自动创建新会话，标题取首条消息前 36 个字符。
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
	uid, ok := currentUserID(c)
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

	// 获取或新建会话
	var session model.ChatSession
	if req.SessionID != nil {
		if err := h.db.Where("id = ? AND user_id = ?", *req.SessionID, uid).First(&session).Error; err != nil {
			response.Fail(c, 404, "会话不存在")
			return
		}
	} else {
		session = model.ChatSession{UserID: uid, Title: sessionTitle(content)}
		if err := h.db.Create(&session).Error; err != nil {
			response.ServerError(c)
			return
		}
	}

	// 从 DB 加载历史消息
	var dbMsgs []model.ChatMessage
	h.db.Where("session_id = ?", session.ID).Order("created_at ASC").Find(&dbMsgs)

	hist := make([]tagger.ChatMessage, 0, len(dbMsgs)+1)
	for _, m := range dbMsgs {
		hist = append(hist, tagger.ChatMessage{Role: m.Role, Content: m.Content})
	}
	hist = append(hist, tagger.ChatMessage{Role: "user", Content: content})

	// RAG 检索
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

	// 调用大模型
	reply, err := h.taggerSvc.ChatCompletion(
		c.Request.Context(), hist, strings.TrimSpace(req.PageHint), retrievalCtx,
	)
	if err != nil {
		if strings.Contains(err.Error(), "api key not configured") {
			response.Fail(c, 503, "大模型未配置：请在管理后台「系统状态」中配置 API Key")
			return
		}
		response.Fail(c, 502, "模型调用失败："+truncateErrMsg(err.Error(), 200))
		return
	}

	// 持久化两条消息
	h.db.Create(&model.ChatMessage{SessionID: session.ID, Role: "user", Content: content})
	h.db.Create(&model.ChatMessage{SessionID: session.ID, Role: "assistant", Content: reply})

	outTitle := session.Title
	updates := map[string]interface{}{"updated_at": time.Now()}
	if req.SessionID != nil && len(dbMsgs) == 0 {
		t := sessionTitle(content)
		updates["title"] = t
		outTitle = t
	}
	if err := h.db.Model(&model.ChatSession{}).Where("id = ?", session.ID).Updates(updates).Error; err != nil {
		log.Printf("[session-chat] bump session: %v", err)
	}

	response.OK(c, gin.H{
		"sessionId": session.ID,
		"title":     outTitle,
		"reply":     reply,
	})
}
