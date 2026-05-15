package handler

import (
	"github.com/gin-gonic/gin"
	"opinion-analysis/internal/model"
	"opinion-analysis/pkg/response"
	"gorm.io/gorm"
	"strconv"
)

type TopicHandler struct {
	db *gorm.DB
}

func NewTopicHandler(db *gorm.DB) *TopicHandler {
	return &TopicHandler{db: db}
}

func (h *TopicHandler) List(c *gin.Context) {
	page, _ := strconv.Atoi(c.DefaultQuery("page", "1"))
	pageSize, _ := strconv.Atoi(c.DefaultQuery("pageSize", "20"))

	var total int64
	h.db.Model(&model.Topic{}).Count(&total)

	var list []model.Topic
	offset := (page - 1) * pageSize
	if err := h.db.Order("heat_score desc").Offset(offset).Limit(pageSize).Find(&list).Error; err != nil {
		response.ServerError(c)
		return
	}
	response.OKPage(c, total, list)
}

func (h *TopicHandler) Detail(c *gin.Context) {
	id := c.Param("id")
	var topic model.Topic
	if err := h.db.First(&topic, id).Error; err != nil {
		response.Fail(c, 404, "话题不存在")
		return
	}
	response.OK(c, topic)
}
