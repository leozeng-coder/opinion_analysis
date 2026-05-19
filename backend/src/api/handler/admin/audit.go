package admin

import (
	"strconv"
	"time"

	"github.com/gin-gonic/gin"
	"opinion-analysis/pkg/response"
	"opinion-analysis/src/repository"
)

type AuditHandler struct {
	audit *repository.AuditRepository
}

func NewAuditHandler(store *repository.Store) *AuditHandler {
	return &AuditHandler{audit: store.Audit}
}

func (h *AuditHandler) List(c *gin.Context) {
	page, _ := strconv.Atoi(c.DefaultQuery("page", "1"))
	pageSize, _ := strconv.Atoi(c.DefaultQuery("pageSize", "30"))
	if page < 1 {
		page = 1
	}
	if pageSize < 1 || pageSize > 200 {
		pageSize = 30
	}

	filter := repository.AuditListFilter{
		ActorName: c.Query("actorName"),
		Action:    c.Query("action"),
		Resource:  c.Query("resource"),
		Page:      page,
		PageSize:  pageSize,
	}
	if startStr := c.Query("startAt"); startStr != "" {
		if t, err := time.Parse(time.RFC3339, startStr); err == nil {
			filter.StartAt = &t
		}
	}
	if endStr := c.Query("endAt"); endStr != "" {
		if t, err := time.Parse(time.RFC3339, endStr); err == nil {
			filter.EndAt = &t
		}
	}

	list, total, err := h.audit.List(filter)
	if err != nil {
		response.ServerError(c)
		return
	}
	response.OKPage(c, total, list)
}
