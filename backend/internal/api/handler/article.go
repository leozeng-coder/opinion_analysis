package handler

import (
	"strconv"

	"github.com/gin-gonic/gin"
	"opinion-analysis/internal/model"
	"opinion-analysis/pkg/response"
	"gorm.io/gorm"
)

type ArticleHandler struct {
	db *gorm.DB
}

func NewArticleHandler(db *gorm.DB) *ArticleHandler {
	return &ArticleHandler{db: db}
}

// List 分页查询舆情列表
func (h *ArticleHandler) List(c *gin.Context) {
	page, _ := strconv.Atoi(c.DefaultQuery("page", "1"))
	pageSize, _ := strconv.Atoi(c.DefaultQuery("pageSize", "20"))
	platform := c.Query("platform")
	sentiment := c.Query("sentiment")
	keyword := c.Query("keyword")
	startAt := c.Query("startAt")
	endAt := c.Query("endAt")

	query := h.db.Model(&model.Article{}).Preload("Source").
		Where("platform != ?", "github-trending-today")
	if platform != "" {
		query = query.Where("platform = ?", platform)
	}
	if sentiment != "" {
		query = query.Where("sentiment = ?", sentiment)
	}
	if keyword != "" {
		query = query.Where("title LIKE ? OR content LIKE ?", "%"+keyword+"%", "%"+keyword+"%")
	}
	if startAt != "" {
		query = query.Where("published_at >= ?", startAt)
	}
	if endAt != "" {
		query = query.Where("published_at <= ?", endAt)
	}

	var total int64
	query.Count(&total)

	var list []model.Article
	offset := (page - 1) * pageSize
	if err := query.Order("published_at desc").Offset(offset).Limit(pageSize).Find(&list).Error; err != nil {
		response.ServerError(c)
		return
	}
	response.OKPage(c, total, list)
}

// Detail 获取单条舆情详情
func (h *ArticleHandler) Detail(c *gin.Context) {
	id := c.Param("id")
	var article model.Article
	if err := h.db.Preload("Source").First(&article, id).Error; err != nil {
		response.Fail(c, 404, "记录不存在")
		return
	}
	response.OK(c, article)
}

// Platforms 返回数据库中实际存在的平台列表
func (h *ArticleHandler) Platforms(c *gin.Context) {
	var platforms []string
	if err := h.db.Model(&model.Article{}).
		Distinct("platform").
		Where("platform != '' AND platform != ?", "github-trending-today").
		Order("platform asc").
		Pluck("platform", &platforms).Error; err != nil {
		response.ServerError(c)
		return
	}
	response.OK(c, platforms)
}

// Stats 舆情统计（情感分布、平台分布、时间趋势）
func (h *ArticleHandler) Stats(c *gin.Context) {
	startAt := c.DefaultQuery("startAt", "")
	endAt := c.DefaultQuery("endAt", "")

	query := h.db.Model(&model.Article{})
	if startAt != "" {
		query = query.Where("published_at >= ?", startAt)
	}
	if endAt != "" {
		query = query.Where("published_at <= ?", endAt)
	}

	type SentDist struct {
		Sentiment string `json:"sentiment"`
		Count     int64  `json:"count"`
	}
	var sentDist []SentDist
	query.Select("sentiment, count(*) as count").Group("sentiment").Scan(&sentDist)

	type PlatformDist struct {
		Platform string `json:"platform"`
		Count    int64  `json:"count"`
	}
	var platDist []PlatformDist
	query.Select("platform, count(*) as count").Group("platform").Scan(&platDist)

	type TrendPoint struct {
		Date  string `json:"date"`
		Count int64  `json:"count"`
	}
	var trend []TrendPoint
	query.Select("DATE(published_at) as date, count(*) as count").
		Group("DATE(published_at)").Order("date asc").Scan(&trend)

	response.OK(c, gin.H{
		"sentiment": sentDist,
		"platform":  platDist,
		"trend":     trend,
	})
}
