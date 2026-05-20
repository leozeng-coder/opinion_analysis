package user

import (
	"math"
	"time"

	"github.com/gin-gonic/gin"
	"opinion-analysis/pkg/response"
	"opinion-analysis/src/repository"
)

type DashboardHandler struct {
	articles *repository.ArticleRepository
	alerts   *repository.AlertRepository
	crawler  *repository.CrawlerRepository
	system   *repository.SystemRepository
	digest   *repository.DigestRepository
}

func NewDashboardHandler(store *repository.Store) *DashboardHandler {
	return &DashboardHandler{
		articles: store.Article,
		alerts:   store.Alert,
		crawler:  store.Crawler,
		system:   store.System,
		digest:   store.Digest,
	}
}

type kpiMetric struct {
	Count         int64    `json:"count"`
	ChangePercent *float64 `json:"changePercent,omitempty"`
}

type negativeRatioMetric struct {
	Percent      float64  `json:"percent"`
	ChangePoints *float64 `json:"changePoints,omitempty"`
}

func pctChange(current, previous int64) *float64 {
	if previous == 0 {
		if current == 0 {
			return nil
		}
		v := 100.0
		return &v
	}
	v := math.Round(float64(current-previous)/float64(previous)*1000) / 10
	return &v
}

func negativeRatioFromDist(dist []repository.SentDist) float64 {
	var total, neg int64
	for _, d := range dist {
		total += d.Count
		if d.Sentiment == "negative" {
			neg = d.Count
		}
	}
	if total == 0 {
		return 0
	}
	return math.Round(float64(neg)/float64(total)*1000) / 10
}

func (h *DashboardHandler) Overview(c *gin.Context) {
	now := time.Now()
	loc := now.Location()
	todayStart := time.Date(now.Year(), now.Month(), now.Day(), 0, 0, 0, 0, loc)
	todayEnd := todayStart.Add(24 * time.Hour)
	yesterdayStart := todayStart.Add(-24 * time.Hour)
	trendStart := todayStart.Add(-6 * 24 * time.Hour)

	fmtRFC := func(t time.Time) string { return t.Format(time.RFC3339) }
	fmtDate := todayStart.Format("2006-01-02")

	todayNew, _ := h.articles.CountBetween(fmtRFC(todayStart), fmtRFC(todayEnd))
	yesterdayNew, _ := h.articles.CountBetween(fmtRFC(yesterdayStart), fmtRFC(todayStart))

	todayAlerts, _ := h.alerts.CountRecordsBetween(todayStart, todayEnd)
	yesterdayAlerts, _ := h.alerts.CountRecordsBetween(yesterdayStart, todayStart)

	todaySent, _ := h.articles.SentimentDist(fmtRFC(todayStart), fmtRFC(todayEnd))
	yesterdaySent, _ := h.articles.SentimentDist(fmtRFC(yesterdayStart), fmtRFC(todayStart))
	todayNegRatio := negativeRatioFromDist(todaySent)
	yesterdayNegRatio := negativeRatioFromDist(yesterdaySent)
	var negChange *float64
	if todayNegRatio != 0 || yesterdayNegRatio != 0 {
		v := math.Round((todayNegRatio-yesterdayNegRatio)*10) / 10
		negChange = &v
	}

	threshold := int64(2)
	if s, err := h.system.GetByKey("dashboard.hot_topic_threshold"); err == nil {
		threshold = ParseHotTopicThreshold(s.Value, 2)
	}
	hotTopicCount := h.articles.CountHotTopics(threshold)

	sentimentTrend, _ := h.articles.SentimentTrend(fmtRFC(trendStart), fmtRFC(todayEnd))
	platform, _ := h.articles.PlatformDist(fmtRFC(trendStart), fmtRFC(todayEnd))
	hotTags, _ := h.articles.TagCounts(fmtRFC(trendStart), fmtRFC(todayEnd), "", 20)

	recentAlerts, _, _ := h.alerts.ListRecords(1, 5, nil)
	recentNegative, _, _ := h.articles.List(repository.ArticleListFilter{
		Page: 1, PageSize: 5, Sentiment: "negative",
	})

	pendingTag, _ := h.articles.CountPendingTagging()
	lastRun, _ := h.crawler.LastRun()
	latestAt, _ := h.articles.LatestPublishedAt()

	var summaryBlock gin.H
	if h.digest != nil {
		if d, err := h.digest.Get(fmtDate); err == nil && d != nil {
			summaryBlock = gin.H{
				"date":     d.Date,
				"text":     d.Text,
				"keywords": d.Keywords,
			}
		}
	}

	response.OK(c, gin.H{
		"summary": summaryBlock,
		"kpi": gin.H{
			"todayNew":      kpiMetric{Count: todayNew, ChangePercent: pctChange(todayNew, yesterdayNew)},
			"hotTopics":     kpiMetric{Count: hotTopicCount},
			"todayAlerts":   kpiMetric{Count: todayAlerts, ChangePercent: pctChange(todayAlerts, yesterdayAlerts)},
			"negativeRatio": negativeRatioMetric{Percent: todayNegRatio, ChangePoints: negChange},
		},
		"sentimentTrend": sentimentTrend,
		"hotTags":        hotTags,
		"recentAlerts":   recentAlerts,
		"recentNegative": recentNegative,
		"platform":       platform,
		"status": gin.H{
			"lastCrawlerRun":  lastRun,
			"pendingTagging":  pendingTag,
			"latestArticleAt": latestAt,
		},
	})
}
