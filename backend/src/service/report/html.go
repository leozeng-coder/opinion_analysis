package report

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"html/template"
	"log"
	"sort"
	"strings"
	"time"

	"opinion-analysis/config"
)

type topArticle struct {
	Title     string
	Platform  string
	Sentiment string
	SentScore float64
}

type topicCard struct {
	Topic   string
	Count   int
	Summary string
	Pos     int
	Neu     int
	Neg     int
	PosRate float64
	NeuRate float64
	NegRate float64
}

type htmlData struct {
	Theme             ReportTheme
	CrawlerRunID      uint
	GeneratedAt       string
	TimeRange         string
	ArticleCount      int
	CommentCount      int
	Platforms         string
	Topics            string
	SentimentPos      int
	SentimentNeu      int
	SentimentNeg      int
	SentPosRate       float64
	SentNeuRate       float64
	SentNegRate       float64
	RiskAlert         bool
	RiskLevel         string
	TopArticlesJSON   template.JS
	PlatformJSON      template.JS
	SentimentJSON     template.JS
	TopTagsJSON       template.JS
	PlatformSentJSON  template.JS
	TopicSentJSON     template.JS
	DailyTrendJSON    template.JS
	ScoreBucketJSON   template.JS
	RadarJSON         template.JS
	ChartColorsJSON   template.JS
	TopicCards        []topicCard
	Conclusion        string
	TopicBubbleJSON   template.JS
	TagCloudJSON      template.JS
	ChartVariantJSON  template.JS
}

func (s *Service) buildHTML(ctx context.Context, stats crawlStats, groupSummaries map[string]string, crawlerRunID uint, platforms []string, topics []string, cfg config.TaggerConfig, apiKeySet bool, htmlTheme string) (string, error) {
	theme := pickReportTheme(htmlTheme)

	var conclusion string
	if apiKeySet {
		var err error
		conclusion, err = s.buildConclusion(ctx, stats, groupSummaries, cfg)
		if err != nil {
			log.Printf("[ReportService] conclusion LLM failed: %v", err)
			conclusion = "（LLM 生成结论暂时不可用）"
		}
	}

	var cards []topicCard
	for _, g := range stats.TopGroups {
		ts := stats.TopicSentiment[g.Topic]
		pos, neu, neg := ts["positive"], ts["neutral"], ts["negative"]
		cards = append(cards, topicCard{
			Topic:   g.Topic,
			Count:   g.Count,
			Summary: groupSummaries[g.Topic],
			Pos:     pos,
			Neu:     neu,
			Neg:     neg,
			PosRate: pct(pos, g.Count),
			NeuRate: pct(neu, g.Count),
			NegRate: pct(neg, g.Count),
		})
	}

	var topArticles []topArticle
	for _, a := range stats.TopArticles {
		topArticles = append(topArticles, topArticle{
			Title:     a.Title,
			Platform:  a.Platform,
			Sentiment: a.Sentiment,
			SentScore: a.SentScore,
		})
	}

	pos := stats.SentimentDist["positive"]
	neg := stats.SentimentDist["negative"]
	neu := stats.ArticleCount - pos - neg
	negRate := pct(neg, stats.ArticleCount)

	riskLevel := "低风险"
	if negRate > 40 {
		riskLevel = "高风险"
	} else if negRate > 25 {
		riskLevel = "中风险"
	}

	d := htmlData{
		Theme:             theme,
		CrawlerRunID:      crawlerRunID,
		GeneratedAt:       time.Now().Format("2006-01-02 15:04:05"),
		TimeRange:         formatTimeRange(stats.TimeRange),
		ArticleCount:      stats.ArticleCount,
		CommentCount:      stats.CommentCount,
		Platforms:         strings.Join(platforms, "、"),
		Topics:            strings.Join(topics, "、"),
		SentimentPos:      pos,
		SentimentNeu:      neu,
		SentimentNeg:      neg,
		SentPosRate:       pct(pos, stats.ArticleCount),
		SentNeuRate:       pct(neu, stats.ArticleCount),
		SentNegRate:       negRate,
		RiskAlert:         negRate > 40,
		RiskLevel:         riskLevel,
		TopArticlesJSON:   template.JS(buildTopArticlesJSON(topArticles)),
		PlatformJSON:      template.JS(buildPlatformJSON(stats.Platforms)),
		SentimentJSON:     template.JS(buildSentimentJSON(stats.SentimentDist, stats.ArticleCount)),
		TopTagsJSON:       template.JS(buildTopTagsJSON(stats.TagFreq, 10)),
		PlatformSentJSON:  template.JS(buildPlatformSentimentJSON(stats.PlatformSentiment)),
		TopicSentJSON:     template.JS(buildTopicSentimentJSON(stats.TopicSentiment, stats.TopGroups)),
		DailyTrendJSON:    template.JS(buildDailyTrendJSON(stats.DailyTrend)),
		ScoreBucketJSON:   template.JS(buildScoreBucketJSON(stats.SentScoreBuckets)),
		RadarJSON:         template.JS(buildRadarJSON(stats.PlatformAvgScore)),
		ChartColorsJSON:   template.JS(mustJSON(theme.ChartColors)),
		TopicCards:        cards,
		Conclusion:        conclusion,
		TopicBubbleJSON:   template.JS(buildTopicBubbleJSON(stats.TopGroups, stats.TopicSentiment)),
		TagCloudJSON:      template.JS(buildTopTagsJSON(stats.TagFreq, 25)),
		ChartVariantJSON:  template.JS(mustJSON(theme.Variant)),
	}

	tmpl, err := template.New("report").Funcs(template.FuncMap{
		"css": func(s string) template.CSS { return template.CSS(s) },
	}).Parse(htmlTemplate)
	if err != nil {
		return "", fmt.Errorf("parse html template: %w", err)
	}
	var buf bytes.Buffer
	if err := tmpl.Execute(&buf, d); err != nil {
		return "", fmt.Errorf("render html: %w", err)
	}
	return buf.String(), nil
}

func pct(n, total int) float64 {
	if total == 0 {
		return 0
	}
	return float64(n) / float64(total) * 100
}

func mustJSON(v any) string {
	b, _ := json.Marshal(v)
	return string(b)
}

func buildPlatformJSON(platforms map[string]int) string {
	type item struct {
		Name  string `json:"name"`
		Value int    `json:"value"`
	}
	var items []item
	for k, v := range platforms {
		items = append(items, item{Name: k, Value: v})
	}
	sort.Slice(items, func(i, j int) bool { return items[i].Value > items[j].Value })
	return mustJSON(items)
}

func buildSentimentJSON(dist map[string]int, total int) string {
	type item struct {
		Name  string  `json:"name"`
		Value int     `json:"value"`
		Rate  float64 `json:"rate"`
	}
	items := []item{
		{Name: "正面", Value: dist["positive"], Rate: pct(dist["positive"], total)},
		{Name: "中性", Value: dist["neutral"] + dist[""], Rate: pct(dist["neutral"]+dist[""], total)},
		{Name: "负面", Value: dist["negative"], Rate: pct(dist["negative"], total)},
	}
	return mustJSON(items)
}

func buildTopTagsJSON(tagFreq map[string]int, top int) string {
	type kv struct {
		Tag   string
		Count int
	}
	var list []kv
	for k, v := range tagFreq {
		list = append(list, kv{k, v})
	}
	sort.Slice(list, func(i, j int) bool { return list[i].Count > list[j].Count })
	if len(list) > top {
		list = list[:top]
	}
	type item struct {
		Name  string `json:"name"`
		Value int    `json:"value"`
	}
	var items []item
	for _, kv := range list {
		items = append(items, item{Name: kv.Tag, Value: kv.Count})
	}
	return mustJSON(items)
}

func buildPlatformSentimentJSON(platformSent map[string]map[string]int) string {
	type result struct {
		Platforms []string `json:"platforms"`
		Series    []struct {
			Name string `json:"name"`
			Data []int  `json:"data"`
		} `json:"series"`
	}

	var platforms []string
	for p := range platformSent {
		platforms = append(platforms, p)
	}
	sort.Slice(platforms, func(i, j int) bool {
		sumI := platformSent[platforms[i]]["positive"] + platformSent[platforms[i]]["neutral"] + platformSent[platforms[i]]["negative"]
		sumJ := platformSent[platforms[j]]["positive"] + platformSent[platforms[j]]["neutral"] + platformSent[platforms[j]]["negative"]
		return sumI > sumJ
	})

	res := result{Platforms: platforms}
	for _, name := range []string{"正面", "中性", "负面"} {
		key := map[string]string{"正面": "positive", "中性": "neutral", "负面": "negative"}[name]
		series := struct {
			Name string `json:"name"`
			Data []int  `json:"data"`
		}{Name: name}
		for _, p := range platforms {
			series.Data = append(series.Data, platformSent[p][key])
		}
		res.Series = append(res.Series, series)
	}
	return mustJSON(res)
}

func buildTopicSentimentJSON(topicSent map[string]map[string]int, groups []groupStats) string {
	type result struct {
		Topics []string `json:"topics"`
		Series []struct {
			Name string `json:"name"`
			Data []int  `json:"data"`
		} `json:"series"`
	}

	var topics []string
	for _, g := range groups {
		topics = append(topics, g.Topic)
	}

	res := result{Topics: topics}
	for _, name := range []string{"正面", "中性", "负面"} {
		key := map[string]string{"正面": "positive", "中性": "neutral", "负面": "negative"}[name]
		series := struct {
			Name string `json:"name"`
			Data []int  `json:"data"`
		}{Name: name}
		for _, t := range topics {
			series.Data = append(series.Data, topicSent[t][key])
		}
		res.Series = append(res.Series, series)
	}
	return mustJSON(res)
}

func buildDailyTrendJSON(trend []dailyTrendPoint) string {
	return mustJSON(trend)
}

func buildScoreBucketJSON(buckets [5]int) string {
	type item struct {
		Name  string `json:"name"`
		Value int    `json:"value"`
	}
	items := []item{
		{Name: "极低(0-0.2)", Value: buckets[0]},
		{Name: "偏低(0.2-0.4)", Value: buckets[1]},
		{Name: "中等(0.4-0.6)", Value: buckets[2]},
		{Name: "偏高(0.6-0.8)", Value: buckets[3]},
		{Name: "极高(0.8-1.0)", Value: buckets[4]},
	}
	return mustJSON(items)
}

func buildRadarJSON(avgScores map[string]float64) string {
	type item struct {
		Name  string  `json:"name"`
		Value float64 `json:"value"`
	}
	var items []item
	for p, v := range avgScores {
		items = append(items, item{Name: p, Value: round2(v * 100)})
	}
	sort.Slice(items, func(i, j int) bool { return items[i].Value > items[j].Value })
	return mustJSON(items)
}

func round2(v float64) float64 {
	return float64(int(v*100+0.5)) / 100
}

func buildTopicBubbleJSON(groups []groupStats, topicSent map[string]map[string]int) string {
	type item struct {
		Name  string    `json:"name"`
		Value []float64 `json:"value"` // [negRate, articleCount]
	}
	var items []item
	for _, g := range groups {
		if g.Count == 0 {
			continue
		}
		ts := topicSent[g.Topic]
		negRate := pct(ts["negative"], g.Count)
		items = append(items, item{
			Name:  g.Topic,
			Value: []float64{round2(negRate), float64(g.Count)},
		})
	}
	return mustJSON(items)
}

func buildTopArticlesJSON(articles []topArticle) string {
	type item struct {
		Title     string  `json:"title"`
		Platform  string  `json:"platform"`
		Sentiment string  `json:"sentiment"`
		SentScore float64 `json:"sentScore"`
	}
	var items []item
	for _, a := range articles {
		t := a.Title
		if len([]rune(t)) > 36 {
			t = string([]rune(t)[:36]) + "..."
		}
		items = append(items, item{
			Title:     t,
			Platform:  a.Platform,
			Sentiment: sentimentLabel(a.Sentiment),
			SentScore: round2(a.SentScore * 100),
		})
	}
	return mustJSON(items)
}
