package handler

import (
	"strconv"
	"time"

	"github.com/gin-gonic/gin"
	"gorm.io/gorm"
	"opinion-analysis/internal/model"
	"opinion-analysis/pkg/response"
)

type AdminAuditHandler struct {
	db *gorm.DB
}

func NewAdminAuditHandler(db *gorm.DB) *AdminAuditHandler {
	return &AdminAuditHandler{db: db}
}

func (h *AdminAuditHandler) List(c *gin.Context) {
	page, _ := strconv.Atoi(c.DefaultQuery("page", "1"))
	pageSize, _ := strconv.Atoi(c.DefaultQuery("pageSize", "30"))
	if page < 1 {
		page = 1
	}
	if pageSize < 1 || pageSize > 200 {
		pageSize = 30
	}

	actorName := c.Query("actorName")
	action := c.Query("action")
	resource := c.Query("resource")
	startStr := c.Query("startAt")
	endStr := c.Query("endAt")

	q := h.db.Model(&model.AuditLog{})
	if actorName != "" {
		q = q.Where("actor_name LIKE ?", "%"+actorName+"%")
	}
	if action != "" {
		q = q.Where("action = ?", action)
	}
	if resource != "" {
		q = q.Where("resource = ?", resource)
	}
	if startStr != "" {
		if t, err := time.Parse(time.RFC3339, startStr); err == nil {
			q = q.Where("created_at >= ?", t)
		}
	}
	if endStr != "" {
		if t, err := time.Parse(time.RFC3339, endStr); err == nil {
			q = q.Where("created_at <= ?", t)
		}
	}

	var total int64
	if err := q.Count(&total).Error; err != nil {
		response.ServerError(c)
		return
	}
	var list []model.AuditLog
	if err := q.Order("id desc").Offset((page - 1) * pageSize).Limit(pageSize).Find(&list).Error; err != nil {
		response.ServerError(c)
		return
	}
	response.OKPage(c, total, list)
}
