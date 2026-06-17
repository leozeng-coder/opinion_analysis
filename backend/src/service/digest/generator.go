package digest

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net/http"
	"strings"
	"time"

	"gorm.io/gorm"
	"opinion-analysis/config"
	"opinion-analysis/pkg/platform"
	"opinion-analysis/src/model"
	"opinion-analysis/src/repository"
	"opinion-analysis/src/service/tagger"
)

// Generator 每日摘要生成器
type Generator struct {
	db     *gorm.DB
	digest *repository.DigestRepository
	tagger *tagger.Service
}

// NewGenerator 创建摘要生成器
func NewGenerator(db *gorm.DB, digestRepo *repository.DigestRepository, taggerSvc *tagger.Service) *Generator {
	return &Generator{
		db:     db,
		digest: digestRepo,
		tagger: taggerSvc,
	}
}

// FilterOptions 摘要生成过滤条件（均为可选）
type FilterOptions struct {
	Topics    []string // 话题过滤（OR 关系）
	Platforms []string // 平台过滤（OR 关系），可传短码或入库值，内部统一归一
	StartDate string   // 格式 "2006-01-02"，为空则不限
	EndDate   string   // 格式 "2006-01-02"，为空则不限
	Limit     int      // 最多取多少条，0 则默认 500
}

// GenerateRecentDigest 生成近期舆情AI分析摘要（基于最近500条数据，无过滤条件）
func (g *Generator) GenerateRecentDigest(ctx context.Context) error {
	return g.GenerateWithFilters(ctx, FilterOptions{})
}

// GenerateWithFilters 按指定过滤条件生成摘要并保存到 Redis
func (g *Generator) GenerateWithFilters(ctx context.Context, opts FilterOptions) error {
	if g.digest == nil {
		log.Printf("[digest] redis not available, skip digest generation")
		return nil
	}

	limit := opts.Limit
	if limit <= 0 || limit > 500 {
		limit = 500
	}

	log.Printf("[digest] 开始生成舆情AI分析摘要 (limit=%d, platforms=%v, topics=%v, start=%s, end=%s)...",
		limit, opts.Platforms, opts.Topics, opts.StartDate, opts.EndDate)

	query := g.db.Model(&model.Article{}).Order("published_at DESC").Limit(limit)

	if opts.StartDate != "" {
		query = query.Where("published_at >= ?", opts.StartDate+" 00:00:00")
	}
	if opts.EndDate != "" {
		query = query.Where("published_at <= ?", opts.EndDate+" 23:59:59")
	}
	if len(opts.Platforms) > 0 {
		cols := platform.ResolveArticleValues(opts.Platforms)
		if len(cols) == 1 {
			query = query.Where("platform = ?", cols[0])
		} else {
			query = query.Where("platform IN ?", cols)
		}
	}
	if len(opts.Topics) == 1 {
		query = query.Where("topic = ?", opts.Topics[0])
	} else if len(opts.Topics) > 1 {
		query = query.Where("topic IN ?", opts.Topics)
	}

	var articles []model.Article
	if err := query.Find(&articles).Error; err != nil {
		return fmt.Errorf("query articles: %w", err)
	}

	if len(articles) == 0 {
		log.Printf("[digest] 没有符合条件的文章，跳过摘要生成")
		return fmt.Errorf("no articles found for given filters")
	}

	log.Printf("[digest] 查询到 %d 条文章，开始分析...", len(articles))

	stats := g.analyzeArticles(articles)

	summary, keywords, err := g.generateSummaryWithLLM(ctx, articles, stats)
	if err != nil {
		log.Printf("[digest] LLM生成摘要失败: %v，使用统计摘要", err)
		summary = g.generateStatsSummary(stats)
		keywords = stats.TopKeywords
	}

	today := time.Now().Format("2006-01-02")
	digestData := &repository.DailyDigest{
		Date:     today,
		Text:     summary,
		Keywords: keywords,
	}

	if err := g.digest.Set(digestData); err != nil {
		return fmt.Errorf("save digest to redis: %w", err)
	}

	log.Printf("[digest] ✅ 摘要生成成功，长度: %d 字符, 关键词: %v", len(summary), keywords)
	return nil
}

// ArticleStats 文章统计数据
type ArticleStats struct {
	TotalCount       int
	PositiveCount    int
	NegativeCount    int
	NeutralCount     int
	PlatformDist     map[string]int
	TopKeywords      []string
	TimeRange        string
	PositivePercent  float64
	NegativePercent  float64
	NeutralPercent   float64
	TopPlatform      string
	TopPlatformCount int
}

// analyzeArticles 分析文章数据
func (g *Generator) analyzeArticles(articles []model.Article) ArticleStats {
	stats := ArticleStats{
		TotalCount:   len(articles),
		PlatformDist: make(map[string]int),
	}

	keywordCount := make(map[string]int)

	for _, article := range articles {
		// 统计情感分布
		switch article.Sentiment {
		case "positive":
			stats.PositiveCount++
		case "negative":
			stats.NegativeCount++
		default:
			stats.NeutralCount++
		}

		// 统计平台分布
		stats.PlatformDist[article.Platform]++

		// 统计关键词
		if article.AITags != nil && *article.AITags != "" {
			var tags []string
			if err := json.Unmarshal([]byte(*article.AITags), &tags); err == nil {
				for _, tag := range tags {
					keywordCount[tag]++
				}
			}
		}
	}

	// 计算百分比
	if stats.TotalCount > 0 {
		stats.PositivePercent = float64(stats.PositiveCount) / float64(stats.TotalCount) * 100
		stats.NegativePercent = float64(stats.NegativeCount) / float64(stats.TotalCount) * 100
		stats.NeutralPercent = float64(stats.NeutralCount) / float64(stats.TotalCount) * 100
	}

	// 找出最多的平台
	maxCount := 0
	for platform, count := range stats.PlatformDist {
		if count > maxCount {
			maxCount = count
			stats.TopPlatform = platform
			stats.TopPlatformCount = count
		}
	}

	// 提取Top 10关键词
	type kv struct {
		Key   string
		Value int
	}
	var kvList []kv
	for k, v := range keywordCount {
		kvList = append(kvList, kv{k, v})
	}
	// 简单排序（冒泡排序，因为数据量不大）
	for i := 0; i < len(kvList); i++ {
		for j := i + 1; j < len(kvList); j++ {
			if kvList[j].Value > kvList[i].Value {
				kvList[i], kvList[j] = kvList[j], kvList[i]
			}
		}
	}
	for i := 0; i < 10 && i < len(kvList); i++ {
		stats.TopKeywords = append(stats.TopKeywords, kvList[i].Key)
	}

	// 时间范围
	if len(articles) > 0 {
		latest := articles[0].PublishedAt
		oldest := articles[len(articles)-1].PublishedAt
		stats.TimeRange = fmt.Sprintf("%s 至 %s",
			oldest.Format("01-02"),
			latest.Format("01-02"))
	}

	return stats
}

// generateSummaryWithLLM 使用LLM生成摘要
func (g *Generator) generateSummaryWithLLM(ctx context.Context, articles []model.Article, stats ArticleStats) (string, []string, error) {
	if g.tagger == nil {
		return "", nil, fmt.Errorf("tagger service not available")
	}

	// 获取tagger配置
	cfg, apiKeySet := g.tagger.GetConfig()
	if !apiKeySet {
		return "", nil, fmt.Errorf("LLM API key not configured")
	}

	// 构建提示词
	prompt := g.buildPrompt(articles, stats)

	// 调用LLM
	response, err := g.callLLM(ctx, prompt, cfg)
	if err != nil {
		return "", nil, err
	}

	// 解析响应
	summary, keywords := g.parseResponse(response)

	return summary, keywords, nil
}

// callLLM 调用LLM API
func (g *Generator) callLLM(ctx context.Context, prompt string, cfg config.TaggerConfig) (string, error) {
	apiKey := strings.TrimSpace(cfg.LLMApiKey)
	if apiKey == "" {
		return "", fmt.Errorf("API key is empty")
	}

	model := cfg.LLMModel
	if strings.TrimSpace(model) == "" {
		model = "deepseek-chat"
	}

	baseURL := cfg.LLMBaseURL
	if strings.TrimSpace(baseURL) == "" {
		baseURL = "https://api.deepseek.com"
	}

	reqBody := map[string]any{
		"model": model,
		"messages": []map[string]string{
			{"role": "system", "content": "你是专业的舆情分析师，擅长总结和提炼关键信息。"},
			{"role": "user", "content": prompt},
		},
		"temperature": 0.7,
		"max_tokens":  1000,
	}

	payload, _ := json.Marshal(reqBody)
	url := strings.TrimRight(baseURL, "/") + "/chat/completions"

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, url, bytes.NewReader(payload))
	if err != nil {
		return "", err
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+apiKey)

	client := &http.Client{Timeout: 60 * time.Second}
	resp, err := client.Do(req)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()

	respBody, _ := io.ReadAll(resp.Body)
	if resp.StatusCode/100 != 2 {
		return "", fmt.Errorf("LLM API error: status=%d body=%s", resp.StatusCode, string(respBody))
	}

	var apiResp struct {
		Choices []struct {
			Message struct {
				Content string `json:"content"`
			} `json:"message"`
		} `json:"choices"`
	}

	if err := json.Unmarshal(respBody, &apiResp); err != nil {
		return "", fmt.Errorf("decode response: %w", err)
	}

	if len(apiResp.Choices) == 0 {
		return "", fmt.Errorf("empty response")
	}

	return apiResp.Choices[0].Message.Content, nil
}

// buildPrompt 构建LLM提示词
func (g *Generator) buildPrompt(articles []model.Article, stats ArticleStats) string {
	var sb strings.Builder

	sb.WriteString("你是一个专业的舆情分析师。请基于以下数据生成一份简洁的近期舆情分析摘要。\n\n")
	sb.WriteString("## 统计数据\n")
	sb.WriteString(fmt.Sprintf("- 分析时间范围: %s\n", stats.TimeRange))
	sb.WriteString(fmt.Sprintf("- 总数据量: %d 条\n", stats.TotalCount))
	sb.WriteString(fmt.Sprintf("- 情感分布: 正面 %.1f%%, 负面 %.1f%%, 中性 %.1f%%\n",
		stats.PositivePercent, stats.NegativePercent, stats.NeutralPercent))
	sb.WriteString(fmt.Sprintf("- 主要平台: %s (%d条)\n", stats.TopPlatform, stats.TopPlatformCount))
	sb.WriteString(fmt.Sprintf("- 热门话题: %s\n", strings.Join(stats.TopKeywords[:min(5, len(stats.TopKeywords))], "、")))

	sb.WriteString("\n## 部分文章标题示例（最新10条）\n")
	for i := 0; i < 10 && i < len(articles); i++ {
		sb.WriteString(fmt.Sprintf("%d. [%s] %s\n", i+1, articles[i].Platform, articles[i].Title))
	}

	sb.WriteString("\n## 任务要求\n")
	sb.WriteString("请生成一份150-200字的舆情分析摘要，包含以下内容：\n")
	sb.WriteString("1. 整体舆情趋势（正面/负面/中性的主要特征）\n")
	sb.WriteString("2. 主要关注话题（2-3个）\n")
	sb.WriteString("3. 值得注意的舆情动向\n\n")
	sb.WriteString("同时，请提取5-8个最重要的关键词，用逗号分隔。\n\n")
	sb.WriteString("请按以下格式输出：\n")
	sb.WriteString("【摘要】\n")
	sb.WriteString("（你的分析摘要）\n\n")
	sb.WriteString("【关键词】\n")
	sb.WriteString("关键词1,关键词2,关键词3,...\n")

	return sb.String()
}

// parseResponse 解析LLM响应
func (g *Generator) parseResponse(response string) (string, []string) {
	lines := strings.Split(response, "\n")
	var summary strings.Builder
	var keywords []string
	inSummary := false
	inKeywords := false

	for _, line := range lines {
		line = strings.TrimSpace(line)

		if strings.Contains(line, "【摘要】") || strings.Contains(line, "[摘要]") {
			inSummary = true
			inKeywords = false
			continue
		}
		if strings.Contains(line, "【关键词】") || strings.Contains(line, "[关键词]") {
			inSummary = false
			inKeywords = true
			continue
		}

		if inSummary && line != "" {
			if summary.Len() > 0 {
				summary.WriteString(" ")
			}
			summary.WriteString(line)
		}

		if inKeywords && line != "" {
			// 解析关键词（逗号或顿号分隔）
			line = strings.ReplaceAll(line, "、", ",")
			parts := strings.Split(line, ",")
			for _, kw := range parts {
				kw = strings.TrimSpace(kw)
				if kw != "" {
					keywords = append(keywords, kw)
				}
			}
		}
	}

	// 如果解析失败，使用整个响应作为摘要
	summaryText := summary.String()
	if summaryText == "" {
		summaryText = response
	}

	// 限制关键词数量
	if len(keywords) > 8 {
		keywords = keywords[:8]
	}

	return summaryText, keywords
}

// generateStatsSummary 生成基于统计的摘要（LLM失败时的备用方案）
func (g *Generator) generateStatsSummary(stats ArticleStats) string {
	var sb strings.Builder

	sb.WriteString(fmt.Sprintf("近期舆情分析（%s）：", stats.TimeRange))
	sb.WriteString(fmt.Sprintf("共监测到%d条舆情信息。", stats.TotalCount))

	// 情感分析
	if stats.PositivePercent > 50 {
		sb.WriteString(fmt.Sprintf("整体舆情偏向正面（%.1f%%），", stats.PositivePercent))
	} else if stats.NegativePercent > 30 {
		sb.WriteString(fmt.Sprintf("需关注负面舆情（%.1f%%），", stats.NegativePercent))
	} else {
		sb.WriteString("整体舆情较为平稳，")
	}

	// 平台分布
	sb.WriteString(fmt.Sprintf("主要来源于%s平台。", stats.TopPlatform))

	// 热门话题
	if len(stats.TopKeywords) > 0 {
		sb.WriteString(fmt.Sprintf("热门话题包括：%s等。", strings.Join(stats.TopKeywords[:min(3, len(stats.TopKeywords))], "、")))
	}

	return sb.String()
}

func min(a, b int) int {
	if a < b {
		return a
	}
	return b
}
