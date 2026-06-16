package report

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"math"
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
	"opinion-analysis/src/service/workflow/nodes"
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
	ArticleID   uint
	Title       string
	Platform    string
	Sentiment   string
	SentScore   float64
	Tags        []string
	LikeCount   int
	PublishedAt time.Time
	AgeDays     int
	TimeWeight  float64
}

// groupStats 单个话题组统计
type groupStats struct {
	Topic         string
	Articles      []articleSummary
	Count         int            // 实际文章数
	WeightedScore float64        // 时效加权热度（用于排序和气泡大小）
	OldCount      int            // >180 天的文章数
	MedianAgeDays int            // 中位数文章年龄（天）
	EarliestDate  time.Time
	LatestDate    time.Time
}

// ArticleBriefing 分析：每篇文章的快速摘要
type ArticleBriefing struct {
	ArticleID   uint      `json:"articleId"`
	Opinion     string    `json:"opinion"`     // LLM 提炼的 1 句核心观点
	PublishedAt time.Time `json:"publishedAt"`
	AgeDays     int       `json:"ageDays"`
	TimeWeight  float64   `json:"timeWeight"`
	Tags        []string  `json:"tags"`
}

// TopicSummaryStructured Phase 3 结构化话题摘要
type TopicSummaryStructured struct {
	CoreSummary string   `json:"coreSummary"` // 一句话核心结论
	KeyFindings []string `json:"keyFindings"` // 3-4 条要点
	RiskLevel   string   `json:"riskLevel"`   // high/medium/low
	RiskNote    string   `json:"riskNote"`    // 风险说明
	Opportunity string   `json:"opportunity"` // 正面机遇信号
}

// GlobalInsight Phase 1.5 全局矛盾/风险聚合
type GlobalInsight struct {
	TopRisks       []string `json:"topRisks"`       // 最突出的风险信号
	Contradictions []string `json:"contradictions"` // 观点矛盾点
	Trends         []string `json:"trends"`         // 趋势信号
}

// ConclusionStructured 结构化结论
type ConclusionStructured struct {
	Situation    string   `json:"situation"`    // 整体态势(1句)
	Risks        []string `json:"risks"`        // 风险信号
	Opportunities []string `json:"opportunities"` // 正面机遇
	Actions      []string `json:"actions"`      // 行动建议
}

// ArticleDeepAnalysis 文章细致分析结果
type ArticleDeepAnalysis struct {
	ArticleID      uint     `json:"articleId"`
	Title          string   `json:"title"`
	Platform       string   `json:"platform"`
	PublishedAt    string   `json:"publishedAt"`
	AgeDays        int      `json:"ageDays"`
	IsOld          bool     `json:"isOld"`
	CoreOpinion    string   `json:"coreOpinion"`
	ContentType    string   `json:"contentType"`
	EmotionProfile string   `json:"emotionProfile"`
	KeyPoints      []string `json:"keyPoints"`
	RiskSignal     string   `json:"riskSignal"`
	TimeNote       string   `json:"timeNote"`
	SentScore      float64  `json:"sentScore"`
	Sentiment      string   `json:"sentiment"`
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
	RefDate           time.Time // 时效计算基准（批次最新文章日期）
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
func (s *Service) Generate(ctx context.Context, articleIDs []int64, crawlerRunID uint, platforms []string, topics []string, format Format, htmlTheme string, sampleSize int, maxGroups int, maxTopicCards int, commentSampleSize int, deepMode bool) (string, error) {
	if sampleSize <= 0 {
		sampleSize = 8
	}
	if maxGroups <= 0 {
		maxGroups = 5
	}
	if maxTopicCards <= 0 {
		maxTopicCards = 8
	}
	if commentSampleSize <= 0 {
		commentSampleSize = 18
	}

	progress := nodes.ProgressFunc(ctx)
	progress(fmt.Sprintf("开始生成报告 — 文章 %d 篇，格式 %s", len(articleIDs), format))
	log.Printf("[Report] 开始生成报告 — 运行ID=%d, 文章数=%d, 格式=%s", crawlerRunID, len(articleIDs), format)

	// 全局 token 计数器（两种模式共用）
	globalTC := &TokenCounter{}
	ctx = WithTokenCounter(ctx, globalTC)

	// Step1: 批量查文章
	var articles []model.Article
	if err := s.db.WithContext(ctx).Where("id IN ?", articleIDs).Find(&articles).Error; err != nil {
		return "", fmt.Errorf("query articles: %w", err)
	}
	if len(articles) == 0 {
		return "", fmt.Errorf("no articles found for the given IDs")
	}

	// 数据预处理：过滤无效数据（两种模式共用）
	articles = filterInvalidArticles(articles)

	// Step1: 统计评论数 + 查评论
	var commentCount int64
	var articleIDsUint []uint
	for _, id := range articleIDs {
		articleIDsUint = append(articleIDsUint, uint(id))
	}
	s.db.WithContext(ctx).Model(&model.ArticleComment{}).Where("article_id IN ?", articleIDsUint).Count(&commentCount)
	var comments []model.ArticleComment
	s.db.WithContext(ctx).Where("article_id IN ?", articleIDsUint).Order("like_count DESC").Find(&comments)
	comments = filterInvalidComments(comments)

	progress(fmt.Sprintf("数据加载完成 — 文章 %d 篇，评论 %d 条", len(articles), len(comments)))
	log.Printf("[Report] 数据查询完成 — 文章 %d 篇, 评论 %d 条", len(articles), len(comments))

	// Step1: 程序统计
	stats := s.computeStats(articles, len(comments), sampleSize, maxGroups)
	log.Printf("[Report] 统计完成 — 初始话题组 %d 个, 正面=%d 中性=%d 负面=%d",
		len(stats.TopGroups),
		stats.SentimentDist["positive"],
		stats.SentimentDist["neutral"]+stats.SentimentDist[""],
		stats.SentimentDist["negative"])

	cfg, apiKeySet := s.taggerSvc.GetConfig()
	groupSummaries := make(map[string]string, len(stats.TopGroups))
	var briefings []ArticleBriefing
	var deepAnalysis []ArticleDeepAnalysis
	var commentAnalysis *CommentAnalysis
	var globalInsight *GlobalInsight
	var topicSummariesStructured map[string]*TopicSummaryStructured
	var conclusionStructured *ConclusionStructured
	var totalTokensUsed int

	if deepMode && apiKeySet {
		// ═══ 深度分析路径 ═══
		log.Printf("[Report] 深度分析模式 — 开始全量挖掘")
		progress("深度分析模式：全量挖掘中...")
		tc := &TokenCounter{}

		// 步骤②：全量深度挖掘
		insights := s.deepAnalyzeAll(ctx, articles, comments, stats, cfg, tc)

		// 步骤③：语义聚类
		progress("语义聚类中...")
		clusters := s.clusterInsights(ctx, insights, cfg, tc)

		// 步骤④：LLM 质量评估+排序
		progress("质量评估与排序...")
		clusters = s.evaluateAndRank(ctx, clusters, cfg, tc)

		// 步骤⑤：转换为渲染结构
		topicSummariesStructured, groupSummaries = s.convertToReportData(clusters, &stats)

		// 评论按 cluster 精确归类分析
		progress("评论深度归类分析...")
		commentAnalysis = s.deepCommentAnalysis(ctx, clusters, comments, articles, cfg, tc)

		// 结论
		progress("生成综合结论...")
		cs, err := s.buildConclusionStructured(ctx, stats, topicSummariesStructured, nil, cfg)
		if err != nil {
			log.Printf("[Report] 深度模式结论失败: %v", err)
		} else {
			conclusionStructured = cs
		}

		progress(fmt.Sprintf("深度分析完成 · 消耗 %s", tc.Summary()))
		log.Printf("[Report] 深度分析完成 — %d 个话题组, %s", len(clusters), tc.Summary())
		totalTokensUsed = tc.Total()

	} else if apiKeySet {
		// Phase 1+2+评论分析 并发执行
		var wg sync.WaitGroup
		var mu sync.Mutex

		// Phase 1: 分析（全量文章）
		wg.Add(1)
		go func() {
			defer wg.Done()
			progress(fmt.Sprintf("分析 %d 篇文章观点...", len(articles)))
			log.Printf("[Report] Phase 1: 分析 %d 篇文章...", len(articles))
			b := s.roughAnalyzeArticles(ctx, articles, stats.RefDate, cfg)
			mu.Lock()
			briefings = b
			mu.Unlock()
			progress(fmt.Sprintf("文章观点提炼完成 — %d 条", len(b)))
			log.Printf("[Report] Phase 1 完成 — 获得 %d 条文章观点", len(b))
		}()

		// 评论分析
		wg.Add(1)
		go func() {
			defer wg.Done()
			progress(fmt.Sprintf("评论分析：处理 %d 条评论...", len(comments)))
			log.Printf("[Report] 评论分析: 处理 %d 条评论...", len(comments))
			ca := s.analyzeComments(ctx, comments, articles, stats.TopGroups, cfg, maxTopicCards, commentSampleSize)
			mu.Lock()
			commentAnalysis = ca
			mu.Unlock()
			if ca != nil {
				progress(fmt.Sprintf("评论分析完成 — %d 个话题卡片，%d 条热评", len(ca.TopicComments), len(ca.HotComments)))
				log.Printf("[Report] 评论分析完成 — 话题卡片 %d 个, 热评 %d 条", len(ca.TopicComments), len(ca.HotComments))
			}
		}()

		wg.Wait()
		log.Printf("[Report] Phase 1+2+评论分析全部完成")

		// Phase 1.5: 全局矛盾/风险聚合（基于 Phase 1 结果）
		if len(briefings) > 0 {
			progress("全局风险与矛盾分析...")
			log.Printf("[Report] Phase 1.5: 全局矛盾检测...")
			globalInsight = s.globalAnalyzeInsights(ctx, briefings, stats, cfg)
			if globalInsight != nil {
				progress(fmt.Sprintf("全局分析完成 — %d 风险, %d 矛盾, %d 趋势",
					len(globalInsight.TopRisks), len(globalInsight.Contradictions), len(globalInsight.Trends)))
				log.Printf("[Report] Phase 1.5 完成 — 风险%d 矛盾%d 趋势%d",
					len(globalInsight.TopRisks), len(globalInsight.Contradictions), len(globalInsight.Trends))
			}
		}

		// Phase 2.5: 基于分析结果，按舆情观点重新聚类话题
		if len(briefings) > 0 {
			progress(fmt.Sprintf("舆情观点聚类 %d 篇文章...", len(articles)))
			log.Printf("[Report] Phase 2.5: 舆情观点聚类 — 对 %d 篇文章重新聚类...", len(articles))
			newGroups, newTopicSent, ok := s.reclusterTopics(ctx, articles, briefings, stats.RefDate, cfg, maxGroups)
			if ok {
				stats.TopGroups = newGroups
				stats.TopicSentiment = newTopicSent
				progress(fmt.Sprintf("聚类完成 — %d 个舆情话题", len(newGroups)))
				log.Printf("[Report] 话题聚类完成 — 生成 %d 个舆情观点话题", len(newGroups))
			} else {
				progress("Phase 2.5 聚类失败，保留原始话题组")
				log.Printf("[Report] 话题聚类失败，保留 AI 标签话题组")
			}
		}

		// Phase 3: 话题深度分析
		progress(fmt.Sprintf("话题深度分析 %d 个话题...", len(stats.TopGroups)))
		log.Printf("[Report] Phase 3: 话题深度分析 %d 个话题...", len(stats.TopGroups))
		topicSummariesStructured = s.summarizeGroupsStructured(ctx, stats.TopGroups, briefings, globalInsight, cfg)
		// 生成纯文本版本用于 Markdown
		for topic, ts := range topicSummariesStructured {
			if ts == nil {
				continue
			}
			var text strings.Builder
			text.WriteString(ts.CoreSummary)
			if len(ts.KeyFindings) > 0 {
				text.WriteString("\n")
				for _, f := range ts.KeyFindings {
					text.WriteString("- " + f + "\n")
				}
			}
			if ts.RiskNote != "" {
				text.WriteString("风险：" + ts.RiskNote + "\n")
			}
			if ts.Opportunity != "" {
				text.WriteString("机遇：" + ts.Opportunity + "\n")
			}
			groupSummaries[topic] = text.String()
		}
		progress("话题分析完成，生成结论...")
		log.Printf("[Report] Phase 3 完成")

		// 结构化结论
		progress("生成综合分析结论...")
		log.Printf("[Report] 生成结构化结论...")
		cs, err := s.buildConclusionStructured(ctx, stats, topicSummariesStructured, globalInsight, cfg)
		if err != nil {
			log.Printf("[Report] 结构化结论失败: %v, 回退旧版", err)
		} else {
			conclusionStructured = cs
		}
		progress("开始渲染报告...")
	}

	// Step4: 生成最终报告
	log.Printf("[Report] 渲染报告内容 (格式=%s)...", format)
	var content string
	var genErr error
	switch format {
	case FormatHTML:
		content, genErr = s.buildHTML(ctx, stats, groupSummaries, crawlerRunID, platforms, topics, cfg, apiKeySet, htmlTheme, commentAnalysis, topicSummariesStructured, conclusionStructured, globalInsight, deepMode)
	default:
		content, genErr = s.buildMarkdown(ctx, stats, groupSummaries, deepAnalysis, crawlerRunID, platforms, topics, cfg, apiKeySet, commentAnalysis)
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
	log.Printf("[Report] 报告生成完成 — ID=%s, 大小=%d KB", reportID, len(content)/1024)

	// 趣味完成话术（公共路径）
	totalTokensUsed = globalTC.Total()
	msg := completionMessage(stats.ArticleCount, stats.CommentCount, totalTokensUsed, deepMode)
	progress(msg)

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

// timeWeight 返回文章基于年龄的时效权重
func timeWeight(ageDays int) float64 {
	switch {
	case ageDays <= 7:
		return 1.0
	case ageDays <= 30:
		return 0.8
	case ageDays <= 90:
		return 0.5
	case ageDays <= 180:
		return 0.3
	case ageDays <= 365:
		return 0.15
	default:
		return 0.05
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

	// 计算 RefDate（批次最新文章发布日期，用于年龄计算）
	var minT, maxT time.Time
	for i, a := range articles {
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
	refDate := maxT
	if refDate.IsZero() {
		refDate = time.Now()
	}
	stats.RefDate = refDate

	for _, a := range articles {
		stats.Platforms[a.Platform]++
		stats.SentimentDist[a.Sentiment]++
		stats.SentScoreBuckets[scoreBucket(a.SentScore)]++

		if stats.PlatformSentiment[a.Platform] == nil {
			stats.PlatformSentiment[a.Platform] = make(map[string]int)
		}
		incSentiment(stats.PlatformSentiment[a.Platform], a.Sentiment)

		platformScoreSum[a.Platform] += a.SentScore
		platformScoreCnt[a.Platform]++

		dateKey := a.PublishedAt.Format("2006-01-02")
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
	}

	// Top tags → groups（按时效加权分数排序）
	type tagCount struct {
		tag           string
		count         int
		weightedScore float64
	}
	var tagList []tagCount
	for tag, arts := range tagToArticles {
		var ws float64
		for _, a := range arts {
			ageDays := int(refDate.Sub(a.PublishedAt).Hours() / 24)
			ws += timeWeight(ageDays)
		}
		tagList = append(tagList, tagCount{tag, len(arts), ws})
	}
	sort.Slice(tagList, func(i, j int) bool { return tagList[i].weightedScore > tagList[j].weightedScore })

	for i, tc := range tagList {
		if i >= maxGroups {
			break
		}
		arts := tagToArticles[tc.tag]

		// 计算话题时间字段
		var earliest, latest time.Time
		ageDaysSlice := make([]int, 0, len(arts))
		oldCount := 0
		for j, a := range arts {
			ageDays := int(refDate.Sub(a.PublishedAt).Hours() / 24)
			ageDaysSlice = append(ageDaysSlice, ageDays)
			if ageDays > 180 {
				oldCount++
			}
			if j == 0 {
				earliest, latest = a.PublishedAt, a.PublishedAt
			} else {
				if a.PublishedAt.Before(earliest) {
					earliest = a.PublishedAt
				}
				if a.PublishedAt.After(latest) {
					latest = a.PublishedAt
				}
			}
		}
		sort.Ints(ageDaysSlice)
		medianAge := 0
		if len(ageDaysSlice) > 0 {
			medianAge = ageDaysSlice[len(ageDaysSlice)/2]
		}

		grp := groupStats{
			Topic:         tc.tag,
			Count:         len(arts), // 使用实际文章数，不用 TagFreq 计数
			WeightedScore: tc.weightedScore,
			OldCount:      oldCount,
			MedianAgeDays: medianAge,
			EarliestDate:  earliest,
			LatestDate:    latest,
		}
		grp.Articles = s.pickRepresentative(arts, sampleSize)
		stats.TopGroups = append(stats.TopGroups, grp)

		topicSent := make(map[string]int)
		for _, a := range arts {
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

	// Top 10 articles by 时效加权情感分数（互动量 × 时效权重）
	type scoredArticle struct {
		art   model.Article
		score float64
	}
	scored := make([]scoredArticle, len(articles))
	for i, a := range articles {
		ageDays := int(refDate.Sub(a.PublishedAt).Hours() / 24)
		tw := timeWeight(ageDays)
		scored[i] = scoredArticle{art: a, score: a.SentScore * tw}
	}
	sort.Slice(scored, func(i, j int) bool { return scored[i].score > scored[j].score })
	top := scored
	if len(top) > 10 {
		top = top[:10]
	}
	stats.TopArticles = make([]model.Article, len(top))
	for i, sa := range top {
		stats.TopArticles[i] = sa.art
	}

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
			ArticleID: a.ID,
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

// reclusterTopics 基于分析观点，用 LLM 按舆情立场重新聚类文章，生成观点驱动的话题组
func (s *Service) reclusterTopics(ctx context.Context, articles []model.Article, briefings []ArticleBriefing, refDate time.Time, cfg config.TaggerConfig, maxGroups int) ([]groupStats, map[string]map[string]int, bool) {
	briefMap := make(map[uint]ArticleBriefing, len(briefings))
	for _, b := range briefings {
		briefMap[b.ArticleID] = b
	}

	var sb strings.Builder
	sb.WriteString("你是资深舆情分析师。以下是来自社交平台的文章列表（含情感标注和AI提炼的核心观点）。\n")
	sb.WriteString(fmt.Sprintf("请按照「用户的情感立场和舆论诉求」将这%d篇文章聚合为%d~%d个话题组。\n\n", len(articles), min2(3, maxGroups), maxGroups))
	sb.WriteString("话题命名规则（重要）：\n")
	sb.WriteString("- 以舆情观点命名，如「运营失公引玩家愤慨」「付费机制遭集体吐槽」「版权争议影响品牌形象」\n")
	sb.WriteString("- 禁止用纯实体名词（如「腾讯」「游戏名」「平台名」）作为话题名\n")
	sb.WriteString("- 话题名5-12个字，直接体现用户的核心立场或情绪诉求\n")
	sb.WriteString("- 有相同怨言、期待、争议点的文章优先归为一组；每篇文章只属于一个话题\n")
	sb.WriteString("- 每个话题至少包含1篇文章\n\n")
	sb.WriteString("以JSON返回（仅JSON，无其他文字）：\n")
	sb.WriteString(`{"clusters":[{"topic":"话题名","ids":[文章序号...],"summary":"一句话归类理由"}]}`)
	sb.WriteString("\n\n文章列表：\n")

	for i, a := range articles {
		opinion := ""
		if b, ok := briefMap[a.ID]; ok && b.Opinion != "" {
			opinion = " — " + b.Opinion
		}
		sb.WriteString(fmt.Sprintf("%d. [%s/%s] %s%s\n", i+1, a.Platform, a.Sentiment, a.Title, opinion))
	}

	resp, err := callLLM(ctx, sb.String(), cfg, 1200)
	if err != nil {
		log.Printf("[Report] reclusterTopics LLM failed: %v", err)
		return nil, nil, false
	}

	resp = strings.TrimSpace(resp)
	if idx := strings.Index(resp, "{"); idx >= 0 {
		resp = resp[idx:]
	}
	if idx := strings.LastIndex(resp, "}"); idx >= 0 {
		resp = resp[:idx+1]
	}

	var parsed struct {
		Clusters []struct {
			Topic   string `json:"topic"`
			IDs     []int  `json:"ids"` // 1-indexed
			Summary string `json:"summary"`
		} `json:"clusters"`
	}
	if err := json.Unmarshal([]byte(resp), &parsed); err != nil {
		log.Printf("[Report] reclusterTopics parse failed: %v — resp: %.200s", err, resp)
		return nil, nil, false
	}
	if len(parsed.Clusters) == 0 {
		return nil, nil, false
	}

	// 按 cluster 大小排序，取前 maxGroups
	clusters := parsed.Clusters
	sort.Slice(clusters, func(i, j int) bool { return len(clusters[i].IDs) > len(clusters[j].IDs) })
	if len(clusters) > maxGroups {
		clusters = clusters[:maxGroups]
	}

	var newGroups []groupStats
	newTopicSent := make(map[string]map[string]int)

	for _, cl := range clusters {
		if len(cl.IDs) == 0 || cl.Topic == "" {
			continue
		}
		var arts []model.Article
		seen := make(map[int]bool)
		for _, idx := range cl.IDs {
			if idx >= 1 && idx <= len(articles) && !seen[idx] {
				arts = append(arts, articles[idx-1])
				seen[idx] = true
			}
		}
		if len(arts) == 0 {
			continue
		}

		var earliest, latest time.Time
		var ws float64
		oldCount := 0
		ageDaysSlice := make([]int, 0, len(arts))
		for j, a := range arts {
			ageDays := int(refDate.Sub(a.PublishedAt).Hours() / 24)
			ws += timeWeight(ageDays)
			ageDaysSlice = append(ageDaysSlice, ageDays)
			if ageDays > 180 {
				oldCount++
			}
			if j == 0 {
				earliest, latest = a.PublishedAt, a.PublishedAt
			} else {
				if a.PublishedAt.Before(earliest) {
					earliest = a.PublishedAt
				}
				if a.PublishedAt.After(latest) {
					latest = a.PublishedAt
				}
			}
		}
		sort.Ints(ageDaysSlice)
		medianAge := 0
		if len(ageDaysSlice) > 0 {
			medianAge = ageDaysSlice[len(ageDaysSlice)/2]
		}

		grp := groupStats{
			Topic:         cl.Topic,
			Count:         len(arts),
			WeightedScore: ws,
			OldCount:      oldCount,
			MedianAgeDays: medianAge,
			EarliestDate:  earliest,
			LatestDate:    latest,
		}
		grp.Articles = s.pickRepresentative(arts, 8)
		newGroups = append(newGroups, grp)

		topicSent := make(map[string]int)
		for _, a := range arts {
			incSentiment(topicSent, a.Sentiment)
		}
		newTopicSent[cl.Topic] = topicSent
	}

	if len(newGroups) == 0 {
		return nil, nil, false
	}
	sort.Slice(newGroups, func(i, j int) bool { return newGroups[i].WeightedScore > newGroups[j].WeightedScore })
	return newGroups, newTopicSent, true
}

func min2(a, b int) int {
	if a < b {
		return a
	}
	return b
}

// summarizeGroupsStructured 并发对每个话题组调用 LLM，生成结构化分析
func (s *Service) summarizeGroupsStructured(ctx context.Context, groups []groupStats, briefings []ArticleBriefing, globalInsight *GlobalInsight, cfg config.TaggerConfig) map[string]*TopicSummaryStructured {
	briefMap := make(map[uint]ArticleBriefing, len(briefings))
	for _, b := range briefings {
		briefMap[b.ArticleID] = b
	}

	result := make(map[string]*TopicSummaryStructured, len(groups))
	var mu sync.Mutex
	var wg sync.WaitGroup

	for _, g := range groups {
		wg.Add(1)
		go func(grp groupStats) {
			defer wg.Done()
			prompt := buildGroupPromptStructured(grp, briefMap, globalInsight)
			resp, err := callLLM(ctx, prompt, cfg, 500)
			if err != nil {
				log.Printf("[ReportService] group LLM failed for topic=%s: %v", grp.Topic, err)
				mu.Lock()
				result[grp.Topic] = &TopicSummaryStructured{
					CoreSummary: fmt.Sprintf("话题「%s」共 %d 篇，LLM分析暂不可用", grp.Topic, grp.Count),
					RiskLevel:   "low",
				}
				mu.Unlock()
				return
			}

			resp = strings.TrimSpace(resp)
			if idx := strings.Index(resp, "{"); idx >= 0 {
				resp = resp[idx:]
			}
			if idx := strings.LastIndex(resp, "}"); idx >= 0 {
				resp = resp[:idx+1]
			}

			var parsed TopicSummaryStructured
			if err := json.Unmarshal([]byte(resp), &parsed); err != nil {
				log.Printf("[ReportService] parse group summary JSON failed for topic=%s: %v", grp.Topic, err)
				parsed = TopicSummaryStructured{
					CoreSummary: fmt.Sprintf("话题「%s」共 %d 篇", grp.Topic, grp.Count),
					RiskLevel:   "low",
				}
			}
			mu.Lock()
			result[grp.Topic] = &parsed
			mu.Unlock()
		}(g)
	}
	wg.Wait()
	return result
}

// summarizeGroups 兼容旧接口：返回纯文本摘要（用于 Markdown 报告）
func (s *Service) summarizeGroups(ctx context.Context, groups []groupStats, briefings []ArticleBriefing, cfg config.TaggerConfig) map[string]string {
	structured := s.summarizeGroupsStructured(ctx, groups, briefings, nil, cfg)
	result := make(map[string]string, len(structured))
	for topic, ts := range structured {
		if ts == nil {
			continue
		}
		var sb strings.Builder
		sb.WriteString(ts.CoreSummary)
		if len(ts.KeyFindings) > 0 {
			sb.WriteString("\n")
			for _, f := range ts.KeyFindings {
				sb.WriteString("- " + f + "\n")
			}
		}
		if ts.RiskNote != "" {
			sb.WriteString("风险：" + ts.RiskNote + "\n")
		}
		if ts.Opportunity != "" {
			sb.WriteString("机遇：" + ts.Opportunity + "\n")
		}
		result[topic] = sb.String()
	}
	return result
}

func buildGroupPromptStructured(grp groupStats, briefMap map[uint]ArticleBriefing, globalInsight *GlobalInsight) string {
	var sb strings.Builder
	sb.WriteString(fmt.Sprintf("你是资深舆情分析师。请对「%s」话题做精准研判，输出结构化JSON。\n\n", grp.Topic))
	sb.WriteString("要求：\n")
	sb.WriteString("- coreSummary：一句话点明核心问题和情感态势（不超过30字，直击要害）\n")
	sb.WriteString("- keyFindings：3-4个关键发现（每条15-25字，有信息量，不泛泛而谈）\n")
	sb.WriteString("- riskLevel：high/medium/low\n")
	sb.WriteString("- riskNote：风险说明（若无风险填空字符串，有则20字以内说明）\n")
	sb.WriteString("- opportunity：正面机遇信号（若无填空字符串，有则20字以内）\n\n")

	if grp.OldCount > 0 {
		oldPct := float64(grp.OldCount) / float64(grp.Count) * 100
		sb.WriteString(fmt.Sprintf("⚠️ 时效：%d篇中%d篇（%.0f%%）超180天旧数据（%s~%s）\n\n",
			grp.Count, grp.OldCount, oldPct,
			grp.EarliestDate.Format("2006-01-02"), grp.LatestDate.Format("2006-01-02")))
	}

	if globalInsight != nil && len(globalInsight.TopRisks) > 0 {
		sb.WriteString("全局风险上下文：" + strings.Join(globalInsight.TopRisks, "；") + "\n\n")
	}

	sb.WriteString(fmt.Sprintf("话题共%d篇，时效热度%.2f，代表性文章：\n", grp.Count, grp.WeightedScore))
	for i, a := range grp.Articles {
		line := fmt.Sprintf("%d. [%s/%s/%.2f] %s", i+1, a.Platform, a.Sentiment, a.SentScore, a.Title)
		if b, ok := briefMap[a.ArticleID]; ok && b.Opinion != "" {
			line += " → " + b.Opinion
		}
		sb.WriteString(line + "\n")
	}
	sb.WriteString("\n仅返回JSON，格式：{\"coreSummary\":\"...\",\"keyFindings\":[...],\"riskLevel\":\"...\",\"riskNote\":\"...\",\"opportunity\":\"...\"}")
	return sb.String()
}

// buildGroupPrompt 旧版 prompt（保留给 Markdown）
func buildGroupPrompt(grp groupStats, briefMap map[uint]ArticleBriefing) string {
	var sb strings.Builder
	sb.WriteString(fmt.Sprintf("你是资深舆情分析师。请对以下「%s」话题的舆情做深度分析（250-350字），要求：\n", grp.Topic))
	sb.WriteString("1. 总结该话题的核心事件/议题是什么\n")
	sb.WriteString("2. 各方主要观点和立场分歧\n")
	sb.WriteString("3. 情感倾向的成因分析（为什么正面/负面）\n")
	sb.WriteString("4. 对决策者的建议：需要关注什么风险、可以采取什么行动\n\n")

	if grp.OldCount > 0 {
		oldPct := float64(grp.OldCount) / float64(grp.Count) * 100
		sb.WriteString(fmt.Sprintf("⚠️ 数据时效提示：该话题 %d 篇中有 %d 篇（%.0f%%）为180天以上的旧数据（发布于 %s ~ %s），分析时请注意区分历史背景与当前热度。\n\n",
			grp.Count, grp.OldCount, oldPct,
			grp.EarliestDate.Format("2006-01-02"), grp.LatestDate.Format("2006-01-02")))
	} else {
		sb.WriteString(fmt.Sprintf("数据时间范围：%s ~ %s（近 %d 天内数据，时效性良好）\n\n",
			grp.EarliestDate.Format("2006-01-02"), grp.LatestDate.Format("2006-01-02"), grp.MedianAgeDays))
	}

	sb.WriteString(fmt.Sprintf("话题文章共 %d 篇，时效加权热度：%.2f，以下是代表性内容及观点摘要：\n", grp.Count, grp.WeightedScore))
	for i, a := range grp.Articles {
		line := fmt.Sprintf("%d. [%s][%s][评分%.2f] %s", i+1, a.Platform, a.Sentiment, a.SentScore, a.Title)
		if b, ok := briefMap[a.ArticleID]; ok && b.Opinion != "" {
			line += "　→ " + b.Opinion
		}
		sb.WriteString(line + "\n")
	}
	sb.WriteString("\n请直接输出分析内容，使用自然段落，不要加标题或markdown格式符号。")
	return sb.String()
}

// roughAnalyzeArticles Phase 1: 全量文章分析，10篇/批并发调用 LLM 提炼核心观点
func (s *Service) roughAnalyzeArticles(ctx context.Context, articles []model.Article, refDate time.Time, cfg config.TaggerConfig) []ArticleBriefing {
	const batchSize = 10

	type batch struct {
		start int
		arts  []model.Article
	}
	var batches []batch
	for i := 0; i < len(articles); i += batchSize {
		end := i + batchSize
		if end > len(articles) {
			end = len(articles)
		}
		batches = append(batches, batch{start: i, arts: articles[i:end]})
	}

	results := make([][]ArticleBriefing, len(batches))
	var wg sync.WaitGroup
	var mu sync.Mutex

	for bi, b := range batches {
		wg.Add(1)
		go func(idx int, bt batch) {
			defer wg.Done()
			briefings := s.roughAnalyzeBatch(ctx, bt.arts, refDate, cfg)
			mu.Lock()
			results[idx] = briefings
			mu.Unlock()
		}(bi, b)
	}
	wg.Wait()

	var all []ArticleBriefing
	for _, r := range results {
		all = append(all, r...)
	}
	return all
}

func (s *Service) roughAnalyzeBatch(ctx context.Context, arts []model.Article, refDate time.Time, cfg config.TaggerConfig) []ArticleBriefing {
	var sb strings.Builder
	sb.WriteString("请对以下每篇文章（含其内容摘要和评论）提炼一句核心观点（25-40字，说明主要立场或反映的问题）。\n")
	sb.WriteString("以JSON数组返回，每项格式：{\"id\":序号,\"opinion\":\"核心观点\"}\n只返回JSON，不要其他文字。\n\n")

	for i, a := range arts {
		title := a.Title
		content := ""
		if len([]rune(a.Content)) > 0 {
			r := []rune(a.Content)
			if len(r) > 120 {
				r = r[:120]
			}
			content = string(r)
		}
		ageDays := int(refDate.Sub(a.PublishedAt).Hours() / 24)
		dateLabel := a.PublishedAt.Format("2006-01-02")
		sb.WriteString(fmt.Sprintf("%d. [%s/%s/%s/+%d天] %s", i+1, a.Platform, a.Sentiment, dateLabel, ageDays, title))
		if content != "" {
			sb.WriteString("　" + content)
		}
		sb.WriteString("\n")
	}

	resp, err := callLLM(ctx, sb.String(), cfg, 600)
	if err != nil {
		log.Printf("[RoughAnalysis] batch LLM failed: %v", err)
		return buildFallbackBriefings(arts, refDate)
	}

	resp = strings.TrimSpace(resp)
	if idx := strings.Index(resp, "["); idx >= 0 {
		resp = resp[idx:]
	}
	if idx := strings.LastIndex(resp, "]"); idx >= 0 {
		resp = resp[:idx+1]
	}

	var items []struct {
		ID      int    `json:"id"`
		Opinion string `json:"opinion"`
	}
	if err := json.Unmarshal([]byte(resp), &items); err != nil {
		log.Printf("[RoughAnalysis] parse JSON failed: %v", err)
		return buildFallbackBriefings(arts, refDate)
	}

	opMap := make(map[int]string, len(items))
	for _, it := range items {
		opMap[it.ID] = it.Opinion
	}

	briefings := make([]ArticleBriefing, len(arts))
	for i, a := range arts {
		ageDays := int(refDate.Sub(a.PublishedAt).Hours() / 24)
		var tags []string
		if a.AITags != nil {
			json.Unmarshal([]byte(*a.AITags), &tags)
		}
		briefings[i] = ArticleBriefing{
			ArticleID:   a.ID,
			Opinion:     opMap[i+1],
			PublishedAt: a.PublishedAt,
			AgeDays:     ageDays,
			TimeWeight:  timeWeight(ageDays),
			Tags:        tags,
		}
	}
	return briefings
}

func buildFallbackBriefings(arts []model.Article, refDate time.Time) []ArticleBriefing {
	result := make([]ArticleBriefing, len(arts))
	for i, a := range arts {
		ageDays := int(refDate.Sub(a.PublishedAt).Hours() / 24)
		var tags []string
		if a.AITags != nil {
			json.Unmarshal([]byte(*a.AITags), &tags)
		}
		result[i] = ArticleBriefing{
			ArticleID:   a.ID,
			Opinion:     a.Title,
			PublishedAt: a.PublishedAt,
			AgeDays:     ageDays,
			TimeWeight:  timeWeight(ageDays),
			Tags:        tags,
		}
	}
	return result
}

// globalAnalyzeInsights Phase 1.5: 全局视角矛盾检测与风险聚合
func (s *Service) globalAnalyzeInsights(ctx context.Context, briefings []ArticleBriefing, stats crawlStats, cfg config.TaggerConfig) *GlobalInsight {
	if len(briefings) == 0 {
		return nil
	}

	var sb strings.Builder
	sb.WriteString("你是资深舆情分析师。以下是本批次所有文章的核心观点列表，请从全局视角分析：\n")
	sb.WriteString("1. topRisks：最突出的2-3个风险信号（每条15-20字，具体到事件）\n")
	sb.WriteString("2. contradictions：2-3个观点矛盾冲突点（不同群体的对立立场）\n")
	sb.WriteString("3. trends：2-3个趋势信号（情绪走向、舆论演变方向）\n\n")
	sb.WriteString(fmt.Sprintf("统计概况：%d篇文章，正面%d/中性%d/负面%d\n\n",
		stats.ArticleCount,
		stats.SentimentDist["positive"],
		stats.SentimentDist["neutral"]+stats.SentimentDist[""],
		stats.SentimentDist["negative"]))
	sb.WriteString("文章观点列表：\n")

	for i, b := range briefings {
		if i >= 60 {
			sb.WriteString(fmt.Sprintf("...（省略剩余%d篇）\n", len(briefings)-60))
			break
		}
		sb.WriteString(fmt.Sprintf("%d. [%s] %s\n", i+1, b.Tags, b.Opinion))
	}
	sb.WriteString("\n仅返回JSON：{\"topRisks\":[...],\"contradictions\":[...],\"trends\":[...]}")

	resp, err := callLLM(ctx, sb.String(), cfg, 800)
	if err != nil {
		log.Printf("[Report] Phase 1.5 globalAnalyze failed: %v", err)
		return nil
	}

	resp = strings.TrimSpace(resp)
	if idx := strings.Index(resp, "{"); idx >= 0 {
		resp = resp[idx:]
	}
	if idx := strings.LastIndex(resp, "}"); idx >= 0 {
		resp = resp[:idx+1]
	}

	var insight GlobalInsight
	if err := json.Unmarshal([]byte(resp), &insight); err != nil {
		log.Printf("[Report] Phase 1.5 parse failed: %v", err)
		return nil
	}
	return &insight
}

// buildConclusionStructured 生成分段式结论（带加粗重点）
func (s *Service) buildConclusionStructured(ctx context.Context, stats crawlStats, topicSummaries map[string]*TopicSummaryStructured, globalInsight *GlobalInsight, cfg config.TaggerConfig) (*ConclusionStructured, error) {
	var sb strings.Builder
	sb.WriteString("你是决策层舆情顾问。基于以下数据撰写综合研判结论。\n\n")
	sb.WriteString("格式要求（严格遵守）：\n")
	sb.WriteString("- 分为4段，每段开头用【】标注段落主题：【整体态势】【风险研判】【正面机遇】【行动建议】\n")
	sb.WriteString("- 每段2-4句话，总计250-350字\n")
	sb.WriteString("- 每段中最关键的短语用 ** 包裹加粗（每段1-2处加粗即可，不要过多）\n")
	sb.WriteString("- 行动建议段用编号列出3-4条具体措施\n")
	sb.WriteString("- 直接输出文字，不要JSON，不要markdown标题\n\n")
	sb.WriteString(fmt.Sprintf("数据：%d篇文章，%d条评论，正面%.1f%%，负面%.1f%%\n\n",
		stats.ArticleCount, stats.CommentCount,
		float64(stats.SentimentDist["positive"])/float64(max1(stats.ArticleCount))*100,
		float64(stats.SentimentDist["negative"])/float64(max1(stats.ArticleCount))*100,
	))

	if globalInsight != nil {
		if len(globalInsight.TopRisks) > 0 {
			sb.WriteString("全局风险：" + strings.Join(globalInsight.TopRisks, "；") + "\n")
		}
		if len(globalInsight.Trends) > 0 {
			sb.WriteString("趋势信号：" + strings.Join(globalInsight.Trends, "；") + "\n")
		}
		sb.WriteString("\n")
	}

	sb.WriteString("各话题研判：\n")
	for topic, ts := range topicSummaries {
		if ts == nil {
			continue
		}
		sb.WriteString(fmt.Sprintf("- %s [%s]：%s\n", topic, ts.RiskLevel, ts.CoreSummary))
	}

	resp, err := callLLM(ctx, sb.String(), cfg, 800)
	if err != nil {
		return nil, err
	}

	// 将纯文本结论存入 Situation 字段，前端直接渲染
	conclusion := &ConclusionStructured{
		Situation: strings.TrimSpace(resp),
	}
	return conclusion, nil
}

// deepAnalyzeTopArticles Phase 2: 对 Top 影响力文章逐篇深度分析
func (s *Service) deepAnalyzeTopArticles(ctx context.Context, articles []model.Article, refDate time.Time, cfg config.TaggerConfig, topN int) []ArticleDeepAnalysis {
	// 按（互动量×时效）综合排序，取 Top N
	type scored struct {
		art   model.Article
		score float64
	}
	candidates := make([]scored, len(articles))
	for i, a := range articles {
		ageDays := int(refDate.Sub(a.PublishedAt).Hours() / 24)
		tw := timeWeight(ageDays)
		candidates[i] = scored{art: a, score: math.Abs(a.SentScore-0.5)*tw + tw}
	}
	sort.Slice(candidates, func(i, j int) bool { return candidates[i].score > candidates[j].score })
	if len(candidates) > topN {
		candidates = candidates[:topN]
	}

	results := make([]ArticleDeepAnalysis, len(candidates))
	var wg sync.WaitGroup
	var mu sync.Mutex

	for ci, c := range candidates {
		wg.Add(1)
		go func(idx int, a model.Article) {
			defer wg.Done()
			ageDays := int(refDate.Sub(a.PublishedAt).Hours() / 24)
			analysis := s.deepAnalyzeSingle(ctx, a, ageDays, cfg)
			mu.Lock()
			results[idx] = analysis
			mu.Unlock()
		}(ci, c.art)
	}
	wg.Wait()

	return results
}

func (s *Service) deepAnalyzeSingle(ctx context.Context, a model.Article, ageDays int, cfg config.TaggerConfig) ArticleDeepAnalysis {
	isOld := ageDays > 180

	content := ""
	if len([]rune(a.Content)) > 0 {
		r := []rune(a.Content)
		if len(r) > 300 {
			r = r[:300]
		}
		content = string(r)
	}

	var sb strings.Builder
	sb.WriteString("你是资深舆情分析师。请对以下文章做多维度深度分析，返回严格JSON格式。\n\n")
	sb.WriteString(fmt.Sprintf("标题：%s\n平台：%s\n发布日期：%s（距今%d天）\n情感标注：%s（评分%.2f）\n",
		a.Title, a.Platform, a.PublishedAt.Format("2006-01-02"), ageDays, a.Sentiment, a.SentScore))
	if isOld {
		sb.WriteString("⚠️ 注意：这是一篇旧文章（发布超过180天），请在timeNote字段明确说明时效限制。\n")
	}
	if content != "" {
		sb.WriteString(fmt.Sprintf("内容摘要：%s\n", content))
	}
	sb.WriteString("\n请返回JSON：\n")
	sb.WriteString(`{"coreOpinion":"核心观点(30-50字)","contentType":"内容类型(评测/攻略/抱怨/分享/新闻等)","emotionProfile":"情绪画像(25字以内，如：愤怒+失望、期待+兴奋)","keyPoints":["要点1","要点2","要点3"],"riskSignal":"风险信号(如无则填无)","timeNote":"时效说明(如为旧数据须说明)"}`)

	resp, err := callLLM(ctx, sb.String(), cfg, 400)

	result := ArticleDeepAnalysis{
		ArticleID:   a.ID,
		Title:       a.Title,
		Platform:    a.Platform,
		PublishedAt: a.PublishedAt.Format("2006-01-02"),
		AgeDays:     ageDays,
		IsOld:       isOld,
		SentScore:   a.SentScore,
		Sentiment:   a.Sentiment,
	}

	if err != nil {
		log.Printf("[DeepAnalysis] LLM failed for article %d: %v", a.ID, err)
		result.CoreOpinion = a.Title
		if isOld {
			result.TimeNote = fmt.Sprintf("该文章发布于 %d 天前，属于旧数据，请注意时效性。", ageDays)
		}
		return result
	}

	resp = strings.TrimSpace(resp)
	if idx := strings.Index(resp, "{"); idx >= 0 {
		resp = resp[idx:]
	}
	if idx := strings.LastIndex(resp, "}"); idx >= 0 {
		resp = resp[:idx+1]
	}

	var parsed struct {
		CoreOpinion    string   `json:"coreOpinion"`
		ContentType    string   `json:"contentType"`
		EmotionProfile string   `json:"emotionProfile"`
		KeyPoints      []string `json:"keyPoints"`
		RiskSignal     string   `json:"riskSignal"`
		TimeNote       string   `json:"timeNote"`
	}
	if err := json.Unmarshal([]byte(resp), &parsed); err != nil {
		log.Printf("[DeepAnalysis] parse JSON failed for article %d: %v", a.ID, err)
		result.CoreOpinion = a.Title
		return result
	}

	result.CoreOpinion = parsed.CoreOpinion
	result.ContentType = parsed.ContentType
	result.EmotionProfile = parsed.EmotionProfile
	result.KeyPoints = parsed.KeyPoints
	result.RiskSignal = parsed.RiskSignal
	result.TimeNote = parsed.TimeNote
	if isOld && result.TimeNote == "" {
		result.TimeNote = fmt.Sprintf("该文章发布于 %d 天前，属于历史数据，请注意时效性。", ageDays)
	}
	return result
}


func (s *Service) buildMarkdown(ctx context.Context, stats crawlStats, groupSummaries map[string]string, deepAnalysis []ArticleDeepAnalysis, crawlerRunID uint, platforms []string, topics []string, cfg config.TaggerConfig, apiKeySet bool, commentAnalysis *CommentAnalysis) (string, error) {
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
		sb.WriteString(fmt.Sprintf("### %s（%d 篇，时效热度%.2f）\n\n", g.Topic, g.Count, g.WeightedScore))
		if g.OldCount > 0 {
			sb.WriteString(fmt.Sprintf("> ⚠️ **时效提示：** 该话题 %d 篇中有 %d 篇为180天以上旧数据，数据时间跨度 %s ~ %s。\n\n",
				g.Count, g.OldCount, g.EarliestDate.Format("2006-01-02"), g.LatestDate.Format("2006-01-02")))
		}
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

	// 文章精读（深度分析）
	if len(deepAnalysis) > 0 {
		sb.WriteString("## 重点文章精读\n\n")
		for i, da := range deepAnalysis {
			if i >= 10 {
				break
			}
			oldMark := ""
			if da.IsOld {
				oldMark = "（旧数据）"
			}
			sb.WriteString(fmt.Sprintf("### %d. %s %s\n\n", i+1, da.Title, oldMark))
			sb.WriteString(fmt.Sprintf("**平台：** %s　**发布：** %s（%d天前）　**情感：** %s（%.2f）\n\n",
				da.Platform, da.PublishedAt, da.AgeDays, sentimentLabel(da.Sentiment), da.SentScore))
			if da.CoreOpinion != "" {
				sb.WriteString(fmt.Sprintf("**核心观点：** %s\n\n", da.CoreOpinion))
			}
			if da.ContentType != "" || da.EmotionProfile != "" {
				sb.WriteString(fmt.Sprintf("**内容类型：** %s　**情绪画像：** %s\n\n", da.ContentType, da.EmotionProfile))
			}
			if len(da.KeyPoints) > 0 {
				sb.WriteString("**关键要点：**\n")
				for _, kp := range da.KeyPoints {
					sb.WriteString(fmt.Sprintf("- %s\n", kp))
				}
				sb.WriteString("\n")
			}
			if da.RiskSignal != "" && da.RiskSignal != "无" {
				sb.WriteString(fmt.Sprintf("**风险信号：** %s\n\n", da.RiskSignal))
			}
			if da.TimeNote != "" {
				sb.WriteString(fmt.Sprintf("> ⏰ %s\n\n", da.TimeNote))
			}
		}
	}

	// 评论深度分析
	if commentAnalysis != nil {
		sb.WriteString("## 评论深度分析\n\n")

		// 情感分布
		sb.WriteString("### 评论情感分布\n\n")
		cs := commentAnalysis.OverallSentiment
		csTotal := cs.Positive + cs.Neutral + cs.Negative
		if csTotal > 0 {
			sb.WriteString(fmt.Sprintf("- 正面：**%d** 条（%.1f%%）\n", cs.Positive, cs.PosRate))
			sb.WriteString(fmt.Sprintf("- 中性：**%d** 条（%.1f%%）\n", cs.Neutral, cs.NeuRate))
			sb.WriteString(fmt.Sprintf("- 负面：**%d** 条（%.1f%%）\n\n", cs.Negative, cs.NegRate))
		}

		// 平台分布
		if len(commentAnalysis.PlatformCount) > 0 {
			sb.WriteString("### 评论平台分布\n\n")
			sb.WriteString("| 平台 | 评论数 |\n|------|--------|\n")
			for plat, cnt := range commentAnalysis.PlatformCount {
				sb.WriteString(fmt.Sprintf("| %s | %d |\n", plat, cnt))
			}
			sb.WriteString("\n")
		}

		// 话题评论观点
		if len(commentAnalysis.TopicComments) > 0 {
			sb.WriteString("### 各话题评论观点\n\n")
			for _, tc := range commentAnalysis.TopicComments {
				sb.WriteString(fmt.Sprintf("#### %s（%d 条评论）\n\n", tc.Topic, tc.CommentCount))
				tcTotal := tc.Sentiment.Positive + tc.Sentiment.Neutral + tc.Sentiment.Negative
				if tcTotal > 0 {
					sb.WriteString(fmt.Sprintf("情感：正面 %d / 中性 %d / 负面 %d\n\n", tc.Sentiment.Positive, tc.Sentiment.Neutral, tc.Sentiment.Negative))
				}
				if len(tc.KeyOpinions) > 0 {
					sb.WriteString("代表性观点：\n")
					for _, op := range tc.KeyOpinions {
						sb.WriteString(fmt.Sprintf("- %s\n", op))
					}
					sb.WriteString("\n")
				}
				if len(tc.DeepInsights) > 0 {
					sb.WriteString("深度解析：\n")
					for _, n := range tc.DeepInsights {
						sb.WriteString(fmt.Sprintf("- %s\n", n))
					}
					sb.WriteString("\n")
				}
				if len(tc.MainEmotions) > 0 {
					sb.WriteString(fmt.Sprintf("主要情绪：%s\n\n", strings.Join(tc.MainEmotions, "、")))
				}
			}
		}

		// 热门评论
		if len(commentAnalysis.HotComments) > 0 {
			sb.WriteString("### 热门评论 Top 10\n\n")
			sb.WriteString("| # | 内容 | 平台 | 点赞 | 情感 |\n|---|------|------|------|------|\n")
			for i, hc := range commentAnalysis.HotComments {
				content := hc.Content
				if len([]rune(content)) > 40 {
					content = string([]rune(content)[:40]) + "..."
				}
				sb.WriteString(fmt.Sprintf("| %d | %s | %s | %d | %s |\n", i+1, content, hc.Platform, hc.LikeCount, sentimentLabel(hc.Sentiment)))
			}
			sb.WriteString("\n")
		}
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
	sb.WriteString("你是专业舆情分析师，为决策层撰写结论。请基于以下数据写一份300-400字的综合研判报告，包括：\n")
	sb.WriteString("1. 整体舆情态势（量级、热度走向、情感结构）\n")
	sb.WriteString("2. 核心风险信号（哪些话题/平台有负面发酵趋势）\n")
	sb.WriteString("3. 机遇与正面信号（可借势传播的内容）\n")
	sb.WriteString("4. 行动建议（优先级排序的具体措施）\n\n")
	sb.WriteString(fmt.Sprintf("统计：文章%d篇，评论%d条，正面%.1f%%，负面%.1f%%\n",
		stats.ArticleCount, stats.CommentCount,
		float64(stats.SentimentDist["positive"])/float64(max1(stats.ArticleCount))*100,
		float64(stats.SentimentDist["negative"])/float64(max1(stats.ArticleCount))*100,
	))
	sb.WriteString("各话题研判：\n")
	for topic, summary := range groupSummaries {
		sb.WriteString(fmt.Sprintf("- %s：%s\n", topic, truncate(summary, 150)))
	}
	sb.WriteString("\n请直接输出结论，使用自然段落，不要加标题或markdown格式。")
	return callLLM(ctx, sb.String(), cfg, 600)
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

// completionMessage 根据工作量生成趣味性完成话术
// 必包含：文章数、评论数、一个emoji；不包含token消耗
func completionMessage(articleCount int, commentCount int, tokenCount int, deepMode bool) string {
	level := 1
	switch {
	case tokenCount > 80000:
		level = 5
	case tokenCount > 40000:
		level = 4
	case tokenCount > 15000:
		level = 3
	case tokenCount > 5000:
		level = 2
	default:
		level = 1
	}

	// 每级 5-6 条话术，必含 {a}=文章数 {c}=评论数
	msgs := map[int][]string{
		1: {
			"✨ {a}篇文章+{c}条评论，小菜一碟！报告已就绪",
			"😎 轻松搞定{a}篇文章和{c}条评论，下次有这种小活还找我",
			"😉 {a}篇+{c}条，一眨眼的功夫就分析完了",
			"😏 秒杀！{a}篇文章{c}条评论，连热身都算不上",
			"🙂 {a}篇文章+{c}条评论已分析，这活儿太轻松了",
		},
		2: {
			"👌 {a}篇文章+{c}条评论分析完毕，还挺顺利的",
			"😁 处理了{a}篇文章和{c}条评论，报告已生成好啦",
			"😮 {a}篇+{c}条，规模适中分析得刚刚好",
			"🙂 {a}篇文章{c}条评论，不多不少，完美完成",
			"😋 分析{a}篇文章+{c}条评论完成，去看看报告吧",
		},
		3: {
			"🫣 认真分析了{a}篇文章和{c}条评论，结果应该挺扎实的",
			"🤔 {a}篇文章+{c}条评论全部过了一遍，建议看看话题分析部分",
			"🫡 {a}篇+{c}条处理完毕，这次挺充实的",
			"😌 {a}篇文章{c}条评论分析完成，数据量中等偏大该看的都看了",
			"🥱 逐一分析了{a}篇文章和{c}条评论，报告已生成",
		},
		4: {
			"💪 {a}篇文章+{c}条评论全部啃完，这次工作量不小",
			"🫣 处理了{a}篇+{c}条评论，总算圆满完成了",
			"🔥 {a}篇文章{c}条评论的大工程完工，含金量不低",
			"😦 这批{a}篇+{c}条够忙活的，不过报告质量有保障",
			"😪 {a}篇文章和{c}条评论挨个翻完了，值得好好看看结果",
		},
		5: {
			"😭 我活下来了！{a}篇文章+{c}条评论全部深挖完毕，我先躺会儿...",
			"🫠 终于完了！{a}篇+{c}条逐条翻了个遍，有事明天再说...",
			"🥵 史诗级工作量！{a}篇文章{c}条评论，这波真拼了老命",
			"😇 {a}篇+{c}条全量分析完成，太多了，我需要冷静一下...",
			"🤯 {a}篇文章和{c}条评论的深度挖掘完工，我去冰箱拿瓶水歇会儿...",
		},
	}

	candidates := msgs[level]
	idx := int(time.Now().UnixNano()/1000) % len(candidates)
	tmpl := candidates[idx]

	result := strings.ReplaceAll(tmpl, "{a}", fmt.Sprintf("%d", articleCount))
	result = strings.ReplaceAll(result, "{c}", fmt.Sprintf("%d", commentCount))
	return result
}

func formatTokenCount(tokens int) string {
	if tokens >= 1000 {
		return fmt.Sprintf("%.1fk tokens", float64(tokens)/1000)
	}
	return fmt.Sprintf("%d tokens", tokens)
}

// filterInvalidArticles 过滤无效文章（共用路径）
func filterInvalidArticles(articles []model.Article) []model.Article {
	var valid []model.Article
	for _, a := range articles {
		content := strings.TrimSpace(a.Content)
		if len([]rune(content)) < 10 && a.Title == "" {
			continue
		}
		valid = append(valid, a)
	}
	if len(valid) == 0 {
		return articles
	}
	return valid
}

// filterInvalidComments 过滤无效评论
func filterInvalidComments(comments []model.ArticleComment) []model.ArticleComment {
	var valid []model.ArticleComment
	for _, c := range comments {
		content := strings.TrimSpace(c.Content)
		if len([]rune(content)) < 4 {
			continue
		}
		valid = append(valid, c)
	}
	return valid
}

// callLLM 通用 LLM 调用（与 digest 服务保持相同模式）
// LLMUsage API 返回的 token 用量
type LLMUsage struct {
	PromptTokens     int `json:"prompt_tokens"`
	CompletionTokens int `json:"completion_tokens"`
	TotalTokens      int `json:"total_tokens"`
}

type ctxKeyType string

const ctxKeyTokenCounter ctxKeyType = "tokenCounter"

// WithTokenCounter 将 TokenCounter 注入 context
func WithTokenCounter(ctx context.Context, tc *TokenCounter) context.Context {
	return context.WithValue(ctx, ctxKeyTokenCounter, tc)
}

func callLLM(ctx context.Context, prompt string, cfg config.TaggerConfig, maxTokens int) (string, error) {
	content, usage, err := callLLMWithUsage(ctx, prompt, cfg, maxTokens)
	if err == nil {
		if tc, ok := ctx.Value(ctxKeyTokenCounter).(*TokenCounter); ok && tc != nil {
			tc.Add(usage)
		}
	}
	return content, err
}

func callLLMWithUsage(ctx context.Context, prompt string, cfg config.TaggerConfig, maxTokens int) (string, LLMUsage, error) {
	var usage LLMUsage
	apiKey := strings.TrimSpace(cfg.LLMApiKey)
	if apiKey == "" {
		return "", usage, fmt.Errorf("API key is empty")
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
		return "", usage, err
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+apiKey)

	client := &http.Client{Timeout: 90 * time.Second}
	resp, err := client.Do(req)
	if err != nil {
		return "", usage, err
	}
	defer resp.Body.Close()

	body, _ := io.ReadAll(resp.Body)
	if resp.StatusCode/100 != 2 {
		return "", usage, fmt.Errorf("LLM API error %d: %s", resp.StatusCode, body)
	}

	var apiResp struct {
		Choices []struct {
			Message struct {
				Content string `json:"content"`
			} `json:"message"`
		} `json:"choices"`
		Usage LLMUsage `json:"usage"`
	}
	if err := json.Unmarshal(body, &apiResp); err != nil || len(apiResp.Choices) == 0 {
		return "", usage, fmt.Errorf("invalid LLM response")
	}
	return strings.TrimSpace(apiResp.Choices[0].Message.Content), apiResp.Usage, nil
}
