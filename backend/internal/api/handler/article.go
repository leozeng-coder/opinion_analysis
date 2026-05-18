package handler

import (
	"encoding/json"
	"sort"
	"strconv"
	"strings"

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
	tagsParam := c.Query("tags") // 逗号分隔；多个标签为 OR 关系

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
	if tags := splitNonEmpty(tagsParam, ","); len(tags) > 0 {
		// MySQL 8: JSON_CONTAINS(ai_tags, JSON_QUOTE('xxx'))；任一命中即返回
		conds := make([]string, 0, len(tags))
		args := make([]any, 0, len(tags))
		for _, t := range tags {
			conds = append(conds, "JSON_CONTAINS(ai_tags, JSON_QUOTE(?))")
			args = append(args, t)
		}
		query = query.Where("("+strings.Join(conds, " OR ")+")", args...)
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

// Stats 舆情统计（情感分布、平台分布、时间趋势、热点话题数）
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

	// 热点话题：读取阈值配置，统计出现次数 ≥ 阈值的 AI 标签数
	threshold := int64(2)
	var threshSetting model.SystemSetting
	if h.db.Where("`key` = ?", "dashboard.hot_topic_threshold").First(&threshSetting).Error == nil {
		if n, err := strconv.ParseInt(strings.TrimSpace(threshSetting.Value), 10, 64); err == nil && n > 0 {
			threshold = n
		}
	}
	hotTopicCount := countHotTopics(h.db, threshold)

	response.OK(c, gin.H{
		"sentiment":     sentDist,
		"platform":      platDist,
		"trend":         trend,
		"hotTopicCount": hotTopicCount,
	})
}

// countHotTopics 统计出现次数 ≥ threshold 的 AI 标签数量
func countHotTopics(db *gorm.DB, threshold int64) int64 {
	// 尝试 JSON_TABLE（MySQL 8.0+）
	sql := `
		SELECT COUNT(*) AS hot_count
		FROM (
			SELECT jt.tag, COUNT(*) AS cnt
			FROM articles a,
			     JSON_TABLE(a.ai_tags, '$[*]' COLUMNS (tag VARCHAR(64) PATH '$')) jt
			WHERE a.ai_tags IS NOT NULL
			  AND a.deleted_at IS NULL
			  AND a.platform != 'github-trending-today'
			GROUP BY jt.tag
			HAVING cnt >= ?
		) sub`
	var count int64
	if err := db.Raw(sql, threshold).Scan(&count).Error; err != nil {
		// 回退：内存聚合
		return countHotTopicsInMemory(db, threshold)
	}
	return count
}

func countHotTopicsInMemory(db *gorm.DB, threshold int64) int64 {
	type row struct {
		AITags *string
	}
	var raws []row
	db.Table("articles").
		Select("ai_tags").
		Where("ai_tags IS NOT NULL AND deleted_at IS NULL AND platform != ?", "github-trending-today").
		Scan(&raws)

	counter := map[string]int64{}
	for _, r := range raws {
		if r.AITags == nil {
			continue
		}
		var tags []string
		if err := json.Unmarshal([]byte(*r.AITags), &tags); err != nil {
			continue
		}
		for _, t := range tags {
			t = strings.TrimSpace(t)
			if t != "" {
				counter[t]++
			}
		}
	}
	hotCount := int64(0)
	for _, cnt := range counter {
		if cnt >= threshold {
			hotCount++
		}
	}
	return hotCount
}

// Tags 聚合 AI 标签词频，用于词云 / 下拉筛选
// 查询参数: startAt, endAt, platform, limit (默认 80)
// 实现说明：ai_tags 是 JSON 数组，使用 JSON_TABLE 在数据库侧展开聚合，避免拉全表到内存。
func (h *ArticleHandler) Tags(c *gin.Context) {
	startAt := c.Query("startAt")
	endAt := c.Query("endAt")
	platform := c.Query("platform")
	limit, _ := strconv.Atoi(c.DefaultQuery("limit", "80"))
	if limit <= 0 || limit > 500 {
		limit = 80
	}

	// 先尝试 JSON_TABLE（MySQL 8.0+），失败回退到内存聚合
	var rows []TagCount
	sql := `
		SELECT jt.tag AS tag, COUNT(*) AS count
		FROM articles a,
		     JSON_TABLE(a.ai_tags, '$[*]' COLUMNS (tag VARCHAR(64) PATH '$')) jt
		WHERE a.ai_tags IS NOT NULL
		  AND a.deleted_at IS NULL
		  AND a.platform != 'github-trending-today'`
	args := []any{}
	if startAt != "" {
		sql += " AND a.published_at >= ?"
		args = append(args, startAt)
	}
	if endAt != "" {
		sql += " AND a.published_at <= ?"
		args = append(args, endAt)
	}
	if platform != "" {
		sql += " AND a.platform = ?"
		args = append(args, platform)
	}
	sql += " GROUP BY jt.tag ORDER BY count DESC LIMIT ?"
	args = append(args, limit)

	if err := h.db.Raw(sql, args...).Scan(&rows).Error; err != nil {
		// 回退路径：把 ai_tags 拉到内存解析（兼容老版本 MySQL）
		rows = aggregateTagsInMemory(h.db, startAt, endAt, platform, limit)
	}
	response.OK(c, rows)
}

// TagCount 标签计数对象
type TagCount struct {
	Tag   string `json:"tag"`
	Count int64  `json:"count"`
}

func aggregateTagsInMemory(db *gorm.DB, startAt, endAt, platform string, limit int) []TagCount {
	type row struct {
		AITags *string
	}
	q := db.Table("articles").
		Select("ai_tags as ai_tags").
		Where("ai_tags IS NOT NULL AND deleted_at IS NULL AND platform != ?", "github-trending-today")
	if startAt != "" {
		q = q.Where("published_at >= ?", startAt)
	}
	if endAt != "" {
		q = q.Where("published_at <= ?", endAt)
	}
	if platform != "" {
		q = q.Where("platform = ?", platform)
	}
	var raws []row
	q.Scan(&raws)

	counter := map[string]int64{}
	for _, r := range raws {
		if r.AITags == nil {
			continue
		}
		var tags []string
		if err := json.Unmarshal([]byte(*r.AITags), &tags); err != nil {
			continue
		}
		for _, t := range tags {
			t = strings.TrimSpace(t)
			if t == "" {
				continue
			}
			counter[t]++
		}
	}
	out := make([]TagCount, 0, len(counter))
	for k, v := range counter {
		out = append(out, TagCount{Tag: k, Count: v})
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Count > out[j].Count })
	if len(out) > limit {
		out = out[:limit]
	}
	return out
}

func splitNonEmpty(s, sep string) []string {
	if s == "" {
		return nil
	}
	parts := strings.Split(s, sep)
	out := make([]string, 0, len(parts))
	for _, p := range parts {
		if p = strings.TrimSpace(p); p != "" {
			out = append(out, p)
		}
	}
	return out
}
