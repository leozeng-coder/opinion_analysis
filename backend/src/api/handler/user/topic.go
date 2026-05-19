package user

import (
	"strconv"

	"github.com/gin-gonic/gin"
	"opinion-analysis/pkg/response"
	"opinion-analysis/src/repository"
)

type TopicHandler struct {
	topics *repository.TopicRepository
}

func NewTopicHandler(store *repository.Store) *TopicHandler {
	return &TopicHandler{topics: store.Topic}
}

func (h *TopicHandler) List(c *gin.Context) {
	page, _ := strconv.Atoi(c.DefaultQuery("page", "1"))
	pageSize, _ := strconv.Atoi(c.DefaultQuery("pageSize", "20"))

	list, total, err := h.topics.List(page, pageSize)
	if err != nil {
		response.ServerError(c)
		return
	}
	response.OKPage(c, total, list)
}

func (h *TopicHandler) Detail(c *gin.Context) {
	topic, err := h.topics.FindByID(c.Param("id"))
	if err != nil {
		response.Fail(c, 404, "话题不存在")
		return
	}
	response.OK(c, topic)
}
