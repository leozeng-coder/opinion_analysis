package repository

import (
	"encoding/json"
	"sort"
	"strings"
	"time"

	"gorm.io/gorm"
	"opinion-analysis/src/model"
)

type ArticleRepository struct {
	db *gorm.DB
}

func NewArticleRepository(db *gorm.DB) *ArticleRepository {
	return &ArticleRepository{db: db}
}

type ArticleListFilter struct {
	Page      int
	PageSize  int
	Platform  string
	Sentiment string
	Keyword   string
	StartAt   string
	EndAt     string
	Tags      []string
}

func (r *ArticleRepository) List(filter ArticleListFilter) ([]model.Article, int64, error) {
	query := r.db.Model(&model.Article{}).Preload("Source").
		Where("platform != ?", "github-trending-today")
	if filter.Platform != "" {
		query = query.Where("platform = ?", filter.Platform)
	}
	if filter.Sentiment != "" {
		query = query.Where("sentiment = ?", filter.Sentiment)
	}
	if filter.Keyword != "" {
		query = query.Where("title LIKE ? OR content LIKE ?", "%"+filter.Keyword+"%", "%"+filter.Keyword+"%")
	}
	if filter.StartAt != "" {
		query = query.Where("published_at >= ?", filter.StartAt)
	}
	if filter.EndAt != "" {
		query = query.Where("published_at <= ?", filter.EndAt)
	}
	if len(filter.Tags) > 0 {
		conds := make([]string, 0, len(filter.Tags))
		args := make([]any, 0, len(filter.Tags))
		for _, t := range filter.Tags {
			conds = append(conds, "JSON_CONTAINS(ai_tags, JSON_QUOTE(?))")
			args = append(args, t)
		}
		query = query.Where("("+strings.Join(conds, " OR ")+")", args...)
	}
	var total int64
	query.Count(&total)
	var list []model.Article
	offset := (filter.Page - 1) * filter.PageSize
	err := query.Order("published_at desc").Offset(offset).Limit(filter.PageSize).Find(&list).Error
	return list, total, err
}

func (r *ArticleRepository) FindByID(id string) (*model.Article, error) {
	var article model.Article
	err := r.db.Preload("Source").First(&article, id).Error
	if err != nil {
		return nil, err
	}
	return &article, nil
}

func (r *ArticleRepository) DistinctPlatforms() ([]string, error) {
	var platforms []string
	err := r.db.Model(&model.Article{}).
		Distinct("platform").
		Where("platform != ''").
		Order("platform asc").
		Pluck("platform", &platforms).Error
	return platforms, err
}

type SentDist struct {
	Sentiment string `json:"sentiment"`
	Count     int64  `json:"count"`
}

type PlatformDist struct {
	Platform string `json:"platform"`
	Count    int64  `json:"count"`
}

type TrendPoint struct {
	Date  string `json:"date"`
	Count int64  `json:"count"`
}

type SentimentTrendRow struct {
	Date      string `json:"date"`
	Sentiment string `json:"sentiment"`
	Count     int64  `json:"count"`
}

type SentimentTrendPoint struct {
	Date     string `json:"date"`
	Positive int64  `json:"positive"`
	Neutral  int64  `json:"neutral"`
	Negative int64  `json:"negative"`
	Total    int64  `json:"total"`
}

func (r *ArticleRepository) baseStatsQuery(startAt, endAt string) *gorm.DB {
	query := r.db.Model(&model.Article{})
	if startAt != "" {
		query = query.Where("published_at >= ?", startAt)
	}
	if endAt != "" {
		query = query.Where("published_at <= ?", endAt)
	}
	return query
}

func (r *ArticleRepository) SentimentDist(startAt, endAt string) ([]SentDist, error) {
	var sentDist []SentDist
	err := r.baseStatsQuery(startAt, endAt).Select("sentiment, count(*) as count").Group("sentiment").Scan(&sentDist).Error
	return sentDist, err
}

func (r *ArticleRepository) PlatformDist(startAt, endAt string) ([]PlatformDist, error) {
	var platDist []PlatformDist
	err := r.baseStatsQuery(startAt, endAt).Select("platform, count(*) as count").Group("platform").Scan(&platDist).Error
	return platDist, err
}

func (r *ArticleRepository) Trend(startAt, endAt string) ([]TrendPoint, error) {
	var trend []TrendPoint
	err := r.baseStatsQuery(startAt, endAt).
		Select("DATE(published_at) as date, count(*) as count").
		Group("DATE(published_at)").Order("date asc").Scan(&trend).Error
	return trend, err
}

func (r *ArticleRepository) CountBetween(startAt, endAt string) (int64, error) {
	var count int64
	err := r.baseStatsQuery(startAt, endAt).Count(&count).Error
	return count, err
}

func (r *ArticleRepository) SentimentTrend(startAt, endAt string) ([]SentimentTrendPoint, error) {
	var rows []SentimentTrendRow
	err := r.baseStatsQuery(startAt, endAt).
		Select("DATE(published_at) as date, sentiment, count(*) as count").
		Group("DATE(published_at), sentiment").
		Order("date asc").
		Scan(&rows).Error
	if err != nil {
		return nil, err
	}
	byDate := map[string]*SentimentTrendPoint{}
	order := make([]string, 0)
	for _, row := range rows {
		pt, ok := byDate[row.Date]
		if !ok {
			pt = &SentimentTrendPoint{Date: row.Date}
			byDate[row.Date] = pt
			order = append(order, row.Date)
		}
		switch row.Sentiment {
		case "positive":
			pt.Positive = row.Count
		case "neutral":
			pt.Neutral = row.Count
		case "negative":
			pt.Negative = row.Count
		}
		pt.Total += row.Count
	}
	out := make([]SentimentTrendPoint, 0, len(order))
	for _, d := range order {
		out = append(out, *byDate[d])
	}
	return out, nil
}

func (r *ArticleRepository) LatestPublishedAt() (*time.Time, error) {
	var ts *time.Time
	err := r.db.Model(&model.Article{}).
		Where("platform != ?", "github-trending-today").
		Select("MAX(published_at)").
		Scan(&ts).Error
	return ts, err
}

func (r *ArticleRepository) CountPendingTagging() (int64, error) {
	var count int64
	err := r.db.Table("articles").Where("ai_tags IS NULL AND deleted_at IS NULL").Count(&count).Error
	return count, err
}

func (r *ArticleRepository) CountHotTopics(threshold int64) int64 {
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
	if err := r.db.Raw(sql, threshold).Scan(&count).Error; err != nil {
		return countHotTopicsInMemory(r.db, threshold)
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
	for _, rw := range raws {
		if rw.AITags == nil {
			continue
		}
		var tags []string
		if err := json.Unmarshal([]byte(*rw.AITags), &tags); err != nil {
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

type TagCount struct {
	Tag   string `json:"tag"`
	Count int64  `json:"count"`
}

func (r *ArticleRepository) TagCounts(startAt, endAt, platform string, limit int) ([]TagCount, error) {
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

	var rows []TagCount
	if err := r.db.Raw(sql, args...).Scan(&rows).Error; err != nil {
		return aggregateTagsInMemory(r.db, startAt, endAt, platform, limit), nil
	}
	return rows, nil
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
	for _, rw := range raws {
		if rw.AITags == nil {
			continue
		}
		var tags []string
		if err := json.Unmarshal([]byte(*rw.AITags), &tags); err != nil {
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

func effectiveArticleTimeSQL() string {
	return "COALESCE(NULLIF(published_at, '0001-01-01 00:00:00'), NULLIF(published_at, '0000-00-00 00:00:00'), created_at)"
}

func (r *ArticleRepository) alertRuleQuery(keywordsAnd []string, keywordsOr []string, sentiment string, since time.Time) *gorm.DB {
	q := r.db.Model(&model.Article{}).
		Where("published_at IS NOT NULL").
		Where("published_at != ?", "0000-00-00 00:00:00").
		Where("published_at != ?", "0001-01-01 00:00:00").
		Where("published_at >= ?", since)
	if sentiment != "" {
		q = q.Where("sentiment = ?", sentiment)
	}
	// AND keywords - all must match (each as separate WHERE clause)
	if len(keywordsAnd) > 0 {
		for _, kw := range keywordsAnd {
			kw = strings.TrimSpace(kw)
			if kw == "" {
				continue
			}
			like := "%" + kw + "%"
			q = q.Where("(title LIKE ? OR content LIKE ? OR keywords LIKE ?)", like, like, like)
		}
	}
	// OR keywords - any can match (combined with OR)
	if len(keywordsOr) > 0 {
		conds := make([]string, 0, len(keywordsOr))
		args := make([]any, 0, len(keywordsOr)*3)
		for _, kw := range keywordsOr {
			kw = strings.TrimSpace(kw)
			if kw == "" {
				continue
			}
			like := "%" + kw + "%"
			conds = append(conds, "(title LIKE ? OR content LIKE ? OR keywords LIKE ?)")
			args = append(args, like, like, like)
		}
		if len(conds) > 0 {
			q = q.Where("("+strings.Join(conds, " OR ")+")", args...)
		}
	}
	return q
}

func (r *ArticleRepository) CountForAlertRule(keywordsAnd, keywordsOr []string, sentiment string, since time.Time) (int64, error) {
	var count int64
	err := r.alertRuleQuery(keywordsAnd, keywordsOr, sentiment, since).Count(&count).Error
	return count, err
}

func (r *ArticleRepository) ListSampleForAlertRule(keywordsAnd, keywordsOr []string, sentiment string, since time.Time, limit int) ([]model.Article, error) {
	if limit < 1 {
		limit = 5
	}
	var list []model.Article
	err := r.alertRuleQuery(keywordsAnd, keywordsOr, sentiment, since).
		Order("published_at desc").
		Limit(limit).
		Find(&list).Error
	return list, err
}
