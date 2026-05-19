package user

import (
	"strconv"

	"github.com/gin-gonic/gin"
	"opinion-analysis/pkg/response"
	"opinion-analysis/src/repository"
)

type ArticleHandler struct {
	articles *repository.ArticleRepository
	system   *repository.SystemRepository
}

func NewArticleHandler(store *repository.Store) *ArticleHandler {
	return &ArticleHandler{articles: store.Article, system: store.System}
}

func (h *ArticleHandler) List(c *gin.Context) {
	page, _ := strconv.Atoi(c.DefaultQuery("page", "1"))
	pageSize, _ := strconv.Atoi(c.DefaultQuery("pageSize", "20"))

	list, total, err := h.articles.List(repository.ArticleListFilter{
		Page:      page,
		PageSize:  pageSize,
		Platform:  c.Query("platform"),
		Sentiment: c.Query("sentiment"),
		Keyword:   c.Query("keyword"),
		StartAt:   c.Query("startAt"),
		EndAt:     c.Query("endAt"),
		Tags:      SplitNonEmpty(c.Query("tags"), ","),
	})
	if err != nil {
		response.ServerError(c)
		return
	}
	response.OKPage(c, total, list)
}

func (h *ArticleHandler) Detail(c *gin.Context) {
	article, err := h.articles.FindByID(c.Param("id"))
	if err != nil {
		response.Fail(c, 404, "记录不存在")
		return
	}
	response.OK(c, article)
}

func (h *ArticleHandler) Platforms(c *gin.Context) {
	platforms, err := h.articles.DistinctPlatforms()
	if err != nil {
		response.ServerError(c)
		return
	}
	response.OK(c, platforms)
}

func (h *ArticleHandler) Stats(c *gin.Context) {
	startAt := c.DefaultQuery("startAt", "")
	endAt := c.DefaultQuery("endAt", "")

	sentDist, err := h.articles.SentimentDist(startAt, endAt)
	if err != nil {
		response.ServerError(c)
		return
	}
	platDist, err := h.articles.PlatformDist(startAt, endAt)
	if err != nil {
		response.ServerError(c)
		return
	}
	trend, err := h.articles.Trend(startAt, endAt)
	if err != nil {
		response.ServerError(c)
		return
	}

	threshold := int64(2)
	if s, err := h.system.GetByKey("dashboard.hot_topic_threshold"); err == nil {
		threshold = ParseHotTopicThreshold(s.Value, 2)
	}
	hotTopicCount := h.articles.CountHotTopics(threshold)

	response.OK(c, gin.H{
		"sentiment":     sentDist,
		"platform":      platDist,
		"trend":         trend,
		"hotTopicCount": hotTopicCount,
	})
}

func (h *ArticleHandler) Tags(c *gin.Context) {
	limit, _ := strconv.Atoi(c.DefaultQuery("limit", "80"))
	if limit <= 0 || limit > 500 {
		limit = 80
	}
	rows, err := h.articles.TagCounts(c.Query("startAt"), c.Query("endAt"), c.Query("platform"), limit)
	if err != nil {
		response.ServerError(c)
		return
	}
	response.OK(c, rows)
}
