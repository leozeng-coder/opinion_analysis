package report

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net/http"
	"sort"
	"strings"
	"sync"
	"time"

	"github.com/google/uuid"
	"github.com/redis/go-redis/v9"
	"gorm.io/gorm"
	"opinion-analysis/config"
	"opinion-analysis/src/model"
	"opinion-analysis/src/service/tagger"
)

const reportKeyPrefix = "analysis_report:"
const reportTTL = 7 * 24 * time.Hour

// Format 报告格式
type Format string

const (
	FormatMarkdown Format = "markdown"
	FormatHTML     Format = "html"
)

// ReportMeta 报告元数据（存入 Redis）
type ReportMeta struct {
	Format       Format    `json:"format"`
	Content      string    `json:"content"`
	CrawlerRunID uint      `json:"crawlerRunId"`
	ArticleCount int       `json:"articleCount"`
	CommentCount int       `json:"commentCount"`
	Platforms    []string  `json:"platforms"`
	Topics       []string  `json:"topics"`
	CreatedAt    time.Time `json:"createdAt"`
}

// articleSummary 用于 LLM 分析的轻量文章摘要
type articleSummary struct {
	Title     string
	Platform  string
	Sentiment string
	SentScore float64
	Tags      []string
	LikeCount int
}

// groupStats 单个话题组统计
type groupStats struct {
	Topic    string
	Articles []articleSummary
	Count    int
}

// dailyTrendPoint 按日发布量与情感分布
type dailyTrendPoint struct {
	Date     string `json:"date"`
	Total    int    `json:"total"`
	Positive int    `json:"positive"`
	Negative int    `json:"negative"`
	Neutral  int    `json:"neutral"`
}

// crawlStats 本次爬取的统计数据（程序算，不走 LLM）
type crawlStats struct {
	ArticleCount      int
	CommentCount      int
	Platforms         map[string]int
	SentimentDist     map[string]int
	TagFreq           map[string]int
	PlatformSentiment map[string]map[string]int
	TopicSentiment    map[string]map[string]int
	DailyTrend        []dailyTrendPoint
	SentScoreBuckets  [5]int
	PlatformAvgScore  map[string]float64
	TopGroups         []groupStats
	TopArticles       []model.Article
	TimeRange         [2]time.Time
}

// Service 报告服务
type Service struct {
	db        *gorm.DB
	rdb       *redis.Client
	taggerSvc *tagger.Service
}

func NewService(db *gorm.DB, rdb *redis.Client, taggerSvc *tagger.Service) *Service {
	return &Service{db: db, rdb: rdb, taggerSvc: taggerSvc}
}

// Generate 生成分析报告，返回 reportID
func (s *Service) Generate(ctx context.Context, articleIDs []int64, crawlerRunID uint, platforms []string, topics []string, format Format, htmlTheme string, sampleSize int, maxGroups int) (string, error) {
	if sampleSize <= 0 {
		sampleSize = 8
	}
	if maxGroups <= 0 {
		maxGroups = 5
	}

	// Step1: 批量查文章
	var articles []model.Article
	if err := s.db.WithContext(ctx).Where("id IN ?", articleIDs).Find(&articles).Error; err != nil {
		return "", fmt.Errorf("query articles: %w", err)
	}
	if len(articles) == 0 {
		return "", fmt.Errorf("no articles found for the given IDs")
	}

	// Step1: 统计评论数
	var commentCount int64
	var articleIDsUint []uint
	for _, id := range articleIDs {
		articleIDsUint = append(articleIDsUint, uint(id))
	}
	s.db.WithContext(ctx).Model(&model.ArticleComment{}).Where("article_id IN ?", articleIDsUint).Count(&commentCount)

	// Step1: 程序统计
	stats := s.computeStats(articles, int(commentCount), sampleSize, maxGroups)

	// Step2&3: 分层 LLM 分析
	cfg, apiKeySet := s.taggerSvc.GetConfig()
	groupSummaries := make(map[string]string, len(stats.TopGroups))
	if apiKeySet {
		groupSummaries = s.summarizeGroups(ctx, stats.TopGroups, cfg)
	}

	// Step4: 生成最终报告
	var content string
	var genErr error
	switch format {
	case FormatHTML:
		content, genErr = s.buildHTML(ctx, stats, groupSummaries, crawlerRunID, platforms, topics, cfg, apiKeySet, htmlTheme)
	default:
		content, genErr = s.buildMarkdown(ctx, stats, groupSummaries, crawlerRunID, platforms, topics, cfg, apiKeySet)
	}
	if genErr != nil {
		return "", genErr
	}

	// 保存到 Redis
	reportID := uuid.New().String()
	meta := &ReportMeta{
		Format:       format,
		Content:      content,
		CrawlerRunID: crawlerRunID,
		ArticleCount: stats.ArticleCount,
		CommentCount: stats.CommentCount,
		Platforms:    platforms,
		Topics:       topics,
		CreatedAt:    time.Now(),
	}
	if err := s.save(ctx, reportID, meta); err != nil {
		return "", fmt.Errorf("save report: %w", err)
	}

	return reportID, nil
}

// Get 从 Redis 读取报告
func (s *Service) Get(ctx context.Context, reportID string) (*ReportMeta, error) {
	key := reportKeyPrefix + reportID
	raw, err := s.rdb.Get(ctx, key).Result()
	if err != nil {
		return nil, fmt.Errorf("report not found or expired")
	}
	var meta ReportMeta
	if err := json.Unmarshal([]byte(raw), &meta); err != nil {
		return nil, fmt.Errorf("decode report: %w", err)
	}
	return &meta, nil
}

func (s *Service) save(ctx context.Context, reportID string, meta *ReportMeta) error {
	data, err := json.Marshal(meta)
	if err != nil {
		return err
	}
	return s.rdb.Set(ctx, reportKeyPrefix+reportID, data, reportTTL).Err()
}

func incSentiment(m map[string]int, sentiment string) {
	switch sentiment {
	case "positive", "negative":
		m[sentiment]++
	default:
		m["neutral"]++
	}
}

func scoreBucket(score float64) int {
	switch {
	case score < 0.2:
		return 0
	case score < 0.4:
		return 1
	case score < 0.6:
		return 2
	case score < 0.8:
		return 3
	default:
		return 4
	}
}

// computeStats 纯程序统计，不调 LLM
func (s *Service) computeStats(articles []model.Article, commentCount int, sampleSize int, maxGroups int) crawlStats {
	stats := crawlStats{
		ArticleCount:      len(articles),
		CommentCount:      commentCount,
		Platforms:         make(map[string]int),
		SentimentDist:     make(map[string]int),
		TagFreq:           make(map[string]int),
		PlatformSentiment: make(map[string]map[string]int),
		TopicSentiment:    make(map[string]map[string]int),
		PlatformAvgScore:  make(map[string]float64),
	}

	tagToArticles := make(map[string][]model.Article)
	dailyMap := make(map[string]*dailyTrendPoint)
	platformScoreSum := make(map[string]float64)
	platformScoreCnt := make(map[string]int)

	var minT, maxT time.Time
	for i, a := range articles {
		stats.Platforms[a.Platform]++
		stats.SentimentDist[a.Sentiment]++
		stats.SentScoreBuckets[scoreBucket(a.SentScore)]++

		if stats.PlatformSentiment[a.Platform] == nil {
			stats.PlatformSentiment[a.Platform] = make(map[string]int)
		}
		incSentiment(stats.PlatformSentiment[a.Platform], a.Sentiment)

		platformScoreSum[a.Platform] += a.SentScore
		platformScoreCnt[a.Platform]++

		dateKey := a.PublishedAt.Format("01-02")
		if dailyMap[dateKey] == nil {
			dailyMap[dateKey] = &dailyTrendPoint{Date: dateKey}
		}
		pt := dailyMap[dateKey]
		pt.Total++
		switch a.Sentiment {
		case "positive":
			pt.Positive++
		case "negative":
			pt.Negative++
		default:
			pt.Neutral++
		}

		if a.AITags != nil && *a.AITags != "" {
			var tags []string
			if json.Unmarshal([]byte(*a.AITags), &tags) == nil {
				for _, tag := range tags {
					stats.TagFreq[tag]++
					tagToArticles[tag] = append(tagToArticles[tag], a)
				}
			}
		}

		if i == 0 {
			minT, maxT = a.PublishedAt, a.PublishedAt
		} else {
			if a.PublishedAt.Before(minT) {
				minT = a.PublishedAt
			}
			if a.PublishedAt.After(maxT) {
				maxT = a.PublishedAt
			}
		}
	}
	stats.TimeRange = [2]time.Time{minT, maxT}

	// Top tags → groups
	type tagCount struct {
		tag   string
		count int
	}
	var tagList []tagCount
	for tag, count := range stats.TagFreq {
		tagList = append(tagList, tagCount{tag, count})
	}
	sort.Slice(tagList, func(i, j int) bool { return tagList[i].count > tagList[j].count })

	for i, tc := range tagList {
		if i >= maxGroups {
			break
		}
		grp := groupStats{Topic: tc.tag, Count: tc.count}
		grp.Articles = s.pickRepresentative(tagToArticles[tc.tag], sampleSize)
		stats.TopGroups = append(stats.TopGroups, grp)

		topicSent := make(map[string]int)
		for _, a := range tagToArticles[tc.tag] {
			incSentiment(topicSent, a.Sentiment)
		}
		stats.TopicSentiment[tc.tag] = topicSent
	}

	for plat, sum := range platformScoreSum {
		if cnt := platformScoreCnt[plat]; cnt > 0 {
			stats.PlatformAvgScore[plat] = sum / float64(cnt)
		}
	}

	var dailyKeys []string
	for k := range dailyMap {
		dailyKeys = append(dailyKeys, k)
	}
	sort.Strings(dailyKeys)
	for _, k := range dailyKeys {
		stats.DailyTrend = append(stats.DailyTrend, *dailyMap[k])
	}

	// Top 10 articles by sentiment score
	sorted := make([]model.Article, len(articles))
	copy(sorted, articles)
	sort.Slice(sorted, func(i, j int) bool {
		return sorted[i].SentScore > sorted[j].SentScore
	})
	top := sorted
	if len(top) > 10 {
		top = top[:10]
	}
	stats.TopArticles = top

	return stats
}

// pickRepresentative 从文章列表中按情感多样性抽取代表样本
func (s *Service) pickRepresentative(articles []model.Article, n int) []articleSummary {
	var pos, neg, neu []model.Article
	for _, a := range articles {
		switch a.Sentiment {
		case "positive":
			pos = append(pos, a)
		case "negative":
			neg = append(neg, a)
		default:
			neu = append(neu, a)
		}
	}
	// 按情感比例取样
	perGroup := n / 3
	if perGroup < 1 {
		perGroup = 1
	}
	var picked []model.Article
	picked = append(picked, head(pos, perGroup)...)
	picked = append(picked, head(neg, perGroup)...)
	picked = append(picked, head(neu, n-len(picked))...)
	if len(picked) > n {
		picked = picked[:n]
	}

	result := make([]articleSummary, len(picked))
	for i, a := range picked {
		sum := articleSummary{
			Title:     a.Title,
			Platform:  a.Platform,
			Sentiment: a.Sentiment,
			SentScore: a.SentScore,
		}
		if a.AITags != nil {
			json.Unmarshal([]byte(*a.AITags), &sum.Tags)
		}
		result[i] = sum
	}
	return result
}

func head(articles []model.Article, n int) []model.Article {
	if n >= len(articles) {
		return articles
	}
	return articles[:n]
}

// summarizeGroups 并发对每个话题组调用 LLM，生成 ~200 字小结
func (s *Service) summarizeGroups(ctx context.Context, groups []groupStats, cfg config.TaggerConfig) map[string]string {
	result := make(map[string]string, len(groups))
	var mu sync.Mutex
	var wg sync.WaitGroup

	for _, g := range groups {
		wg.Add(1)
		go func(grp groupStats) {
			defer wg.Done()
			prompt := buildGroupPrompt(grp)
			summary, err := callLLM(ctx, prompt, cfg, 300)
			if err != nil {
				log.Printf("[ReportService] group LLM failed for topic=%s: %v", grp.Topic, err)
				summary = fmt.Sprintf("（话题「%s」共 %d 篇，LLM分析暂不可用）", grp.Topic, grp.Count)
			}
			mu.Lock()
			result[grp.Topic] = summary
			mu.Unlock()
		}(g)
	}
	wg.Wait()
	return result
}

func buildGroupPrompt(grp groupStats) string {
	var sb strings.Builder
	sb.WriteString(fmt.Sprintf("请对以下「%s」话题的舆情内容做一个简洁分析（150字以内），说明该话题的主要观点、情感倾向和值得关注的内容。\n\n", grp.Topic))
	sb.WriteString(fmt.Sprintf("话题文章共 %d 篇，以下是部分代表性样本：\n", grp.Count))
	for i, a := range grp.Articles {
		sb.WriteString(fmt.Sprintf("%d. [%s][%s] %s\n", i+1, a.Platform, a.Sentiment, a.Title))
	}
	sb.WriteString("\n请直接输出分析内容，不要加标题或格式符号。")
	return sb.String()
}

// buildMarkdown 生成 Markdown 报告（含最终 LLM 综合分析）
func (s *Service) buildMarkdown(ctx context.Context, stats crawlStats, groupSummaries map[string]string, crawlerRunID uint, platforms []string, topics []string, cfg config.TaggerConfig, apiKeySet bool) (string, error) {
	var sb strings.Builder

	timeRange := formatTimeRange(stats.TimeRange)
	sb.WriteString(fmt.Sprintf("# 爬虫分析报告\n\n"))
	sb.WriteString(fmt.Sprintf("**运行 ID：** %d　　**时间范围：** %s　　**生成时间：** %s\n\n",
		crawlerRunID, timeRange, time.Now().Format("2006-01-02 15:04:05")))
	sb.WriteString("---\n\n")

	// 执行摘要
	sb.WriteString("## 执行摘要\n\n")
	sb.WriteString(fmt.Sprintf("- 爬取文章：**%d** 篇\n", stats.ArticleCount))
	sb.WriteString(fmt.Sprintf("- 相关评论：**%d** 条\n", stats.CommentCount))
	sb.WriteString(fmt.Sprintf("- 覆盖平台：**%s**\n", strings.Join(platforms, "、")))
	if len(topics) > 0 {
		sb.WriteString(fmt.Sprintf("- 监控话题：**%s**\n", strings.Join(topics, "、")))
	}
	sb.WriteString(fmt.Sprintf("- 数据时间：%s\n\n", timeRange))

	// 话题全景
	sb.WriteString("## 话题全景\n\n")
	sb.WriteString("| 话题 | 文章数 | 占比 |\n|------|--------|------|\n")
	for _, g := range stats.TopGroups {
		pct := float64(g.Count) / float64(stats.ArticleCount) * 100
		sb.WriteString(fmt.Sprintf("| %s | %d | %.1f%% |\n", g.Topic, g.Count, pct))
	}
	sb.WriteString("\n")

	// 情感态势
	sb.WriteString("## 情感态势\n\n")
	pos := stats.SentimentDist["positive"]
	neg := stats.SentimentDist["negative"]
	neu := stats.SentimentDist["neutral"] + stats.SentimentDist[""]
	total := stats.ArticleCount
	if total > 0 {
		sb.WriteString(fmt.Sprintf("- 正面：**%d** 篇（%.1f%%）\n", pos, float64(pos)/float64(total)*100))
		sb.WriteString(fmt.Sprintf("- 中性：**%d** 篇（%.1f%%）\n", neu, float64(neu)/float64(total)*100))
		sb.WriteString(fmt.Sprintf("- 负面：**%d** 篇（%.1f%%）\n\n", neg, float64(neg)/float64(total)*100))
		if float64(neg)/float64(total) > 0.4 {
			sb.WriteString("> ⚠️ **风险提示：** 负面情感占比超过 40%，请重点关注。\n\n")
		}
	}

	// 平台分布
	sb.WriteString("## 平台分布\n\n")
	sb.WriteString("| 平台 | 文章数 |\n|------|--------|\n")
	for plat, cnt := range stats.Platforms {
		sb.WriteString(fmt.Sprintf("| %s | %d |\n", plat, cnt))
	}
	sb.WriteString("\n")

	// 各话题详细分析
	sb.WriteString("## 各话题深度分析\n\n")
	for _, g := range stats.TopGroups {
		sb.WriteString(fmt.Sprintf("### %s（%d 篇）\n\n", g.Topic, g.Count))
		if summary, ok := groupSummaries[g.Topic]; ok {
			sb.WriteString(summary + "\n\n")
		}
		sb.WriteString("**代表性内容：**\n\n")
		for i, a := range g.Articles {
			if i >= 5 {
				break
			}
			sb.WriteString(fmt.Sprintf("%d. `[%s]` `[%s]` %s\n", i+1, a.Platform, sentimentLabel(a.Sentiment), a.Title))
		}
		sb.WriteString("\n")
	}

	// 高影响力内容
	sb.WriteString("## 高影响力内容 Top 10\n\n")
	sb.WriteString("| # | 标题 | 平台 | 情感 |\n|---|------|------|------|\n")
	for i, a := range stats.TopArticles {
		title := a.Title
		if len([]rune(title)) > 40 {
			title = string([]rune(title)[:40]) + "..."
		}
		sb.WriteString(fmt.Sprintf("| %d | %s | %s | %s |\n", i+1, title, a.Platform, sentimentLabel(a.Sentiment)))
	}
	sb.WriteString("\n")

	// 分析结论（LLM）
	sb.WriteString("## 综合分析结论\n\n")
	if apiKeySet {
		conclusion, err := s.buildConclusion(ctx, stats, groupSummaries, cfg)
		if err != nil {
			log.Printf("[ReportService] conclusion LLM failed: %v", err)
			sb.WriteString("（LLM 生成结论暂时不可用）\n")
		} else {
			sb.WriteString(conclusion + "\n")
		}
	} else {
		sb.WriteString("（未配置 LLM，跳过智能分析结论）\n")
	}

	return sb.String(), nil
}

// buildConclusion 最终综合 LLM 调用
func (s *Service) buildConclusion(ctx context.Context, stats crawlStats, groupSummaries map[string]string, cfg config.TaggerConfig) (string, error) {
	var sb strings.Builder
	sb.WriteString("你是专业舆情分析师。请基于以下本次爬取数据的统计摘要和各话题分析，写一份200字以内的综合结论，包括：整体舆情态势、主要风险信号、建议关注点。\n\n")
	sb.WriteString(fmt.Sprintf("统计：文章%d篇，评论%d条，正面%.1f%%，负面%.1f%%\n",
		stats.ArticleCount, stats.CommentCount,
		float64(stats.SentimentDist["positive"])/float64(max1(stats.ArticleCount))*100,
		float64(stats.SentimentDist["negative"])/float64(max1(stats.ArticleCount))*100,
	))
	sb.WriteString("各话题小结：\n")
	for topic, summary := range groupSummaries {
		sb.WriteString(fmt.Sprintf("- %s：%s\n", topic, truncate(summary, 100)))
	}
	sb.WriteString("\n请直接输出结论段落，不要加标题。")
	return callLLM(ctx, sb.String(), cfg, 400)
}

func max1(n int) int {
	if n < 1 {
		return 1
	}
	return n
}

func truncate(s string, n int) string {
	r := []rune(s)
	if len(r) <= n {
		return s
	}
	return string(r[:n]) + "..."
}

func sentimentLabel(s string) string {
	switch s {
	case "positive":
		return "正面"
	case "negative":
		return "负面"
	default:
		return "中性"
	}
}

func formatTimeRange(tr [2]time.Time) string {
	if tr[0].IsZero() {
		return "未知"
	}
	return fmt.Sprintf("%s ~ %s", tr[0].Format("2006-01-02"), tr[1].Format("2006-01-02"))
}

// callLLM 通用 LLM 调用（与 digest 服务保持相同模式）
func callLLM(ctx context.Context, prompt string, cfg config.TaggerConfig, maxTokens int) (string, error) {
	apiKey := strings.TrimSpace(cfg.LLMApiKey)
	if apiKey == "" {
		return "", fmt.Errorf("API key is empty")
	}
	llmModel := cfg.LLMModel
	if strings.TrimSpace(llmModel) == "" {
		llmModel = "deepseek-chat"
	}
	baseURL := cfg.LLMBaseURL
	if strings.TrimSpace(baseURL) == "" {
		baseURL = "https://api.deepseek.com"
	}

	reqBody := map[string]any{
		"model": llmModel,
		"messages": []map[string]string{
			{"role": "system", "content": "你是专业的舆情分析师，擅长从大量数据中提炼关键信息。"},
			{"role": "user", "content": prompt},
		},
		"temperature": 0.5,
		"max_tokens":  maxTokens,
	}
	payload, _ := json.Marshal(reqBody)
	url := strings.TrimRight(baseURL, "/") + "/chat/completions"

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, url, bytes.NewReader(payload))
	if err != nil {
		return "", err
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+apiKey)

	client := &http.Client{Timeout: 90 * time.Second}
	resp, err := client.Do(req)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()

	body, _ := io.ReadAll(resp.Body)
	if resp.StatusCode/100 != 2 {
		return "", fmt.Errorf("LLM API error %d: %s", resp.StatusCode, body)
	}

	var apiResp struct {
		Choices []struct {
			Message struct {
				Content string `json:"content"`
			} `json:"message"`
		} `json:"choices"`
	}
	if err := json.Unmarshal(body, &apiResp); err != nil || len(apiResp.Choices) == 0 {
		return "", fmt.Errorf("invalid LLM response")
	}
	return strings.TrimSpace(apiResp.Choices[0].Message.Content), nil
}
