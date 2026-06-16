package report

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"sort"
	"strings"
	"sync"

	"opinion-analysis/config"
	"opinion-analysis/src/model"
)

const deepBatchSize = 5
const deepConcurrency = 5

// TokenCounter 线程安全的 token 计数器
type TokenCounter struct {
	mu               sync.Mutex
	promptTokens     int
	completionTokens int
}

func (tc *TokenCounter) Add(usage LLMUsage) {
	tc.mu.Lock()
	tc.promptTokens += usage.PromptTokens
	tc.completionTokens += usage.CompletionTokens
	tc.mu.Unlock()
}

func (tc *TokenCounter) Total() int {
	tc.mu.Lock()
	defer tc.mu.Unlock()
	return tc.promptTokens + tc.completionTokens
}

func (tc *TokenCounter) Summary() string {
	tc.mu.Lock()
	defer tc.mu.Unlock()
	total := tc.promptTokens + tc.completionTokens
	if total >= 1000 {
		return fmt.Sprintf("%.1fk tokens (输入 %dk + 输出 %dk)", float64(total)/1000, tc.promptTokens/1000, tc.completionTokens/1000)
	}
	return fmt.Sprintf("%d tokens (输入 %d + 输出 %d)", total, tc.promptTokens, tc.completionTokens)
}

// ArticleInsight 深度模式每篇文章的完整洞察
type ArticleInsight struct {
	ArticleID      uint     `json:"articleId"`
	Title          string   `json:"title"`
	Platform       string   `json:"platform"`
	CoreNeed       string   `json:"coreNeed"`
	PainPoints     []string `json:"painPoints"`
	Suggestions    []string `json:"suggestions"`
	Sentiment      string   `json:"sentiment"`
	Intensity      int      `json:"intensity"`
	Evidence       []string `json:"evidence"`
	CommentSignals []string `json:"commentSignals"`
	DecisionValue  int      `json:"decisionValue"`
	Endorsement    float64  `json:"endorsement"`
	AgeDays        int      `json:"ageDays"`
	TimeWeight     float64  `json:"timeWeight"`
}

// InsightCluster 聚类后的诉求话题组
type InsightCluster struct {
	Topic       string           `json:"topic"`
	CoreVerdict string           `json:"coreVerdict"`
	Insights    []ArticleInsight `json:"insights"`
	RiskLevel   string           `json:"riskLevel"`
	ActionItem  string           `json:"actionItem"`
	KeyFindings []string         `json:"keyFindings"`
}

// PLACEHOLDER_DEEP_METHODS

// callLLMTracked 带 token 统计的 LLM 调用（同时累加到 context 计数器）
func callLLMTracked(ctx context.Context, prompt string, cfg config.TaggerConfig, maxTokens int, tc *TokenCounter) (string, error) {
	content, usage, err := callLLMWithUsage(ctx, prompt, cfg, maxTokens)
	if err == nil {
		if tc != nil {
			tc.Add(usage)
		}
		if ctxTC, ok := ctx.Value(ctxKeyTokenCounter).(*TokenCounter); ok && ctxTC != nil && ctxTC != tc {
			ctxTC.Add(usage)
		}
	}
	return content, err
}
func (s *Service) deepAnalyzeAll(ctx context.Context, articles []model.Article, comments []model.ArticleComment, stats crawlStats, cfg config.TaggerConfig, tc *TokenCounter) []ArticleInsight {
	// 建立 articleID → comments 索引
	commentMap := make(map[uint][]model.ArticleComment)
	for _, c := range comments {
		commentMap[c.ArticleID] = append(commentMap[c.ArticleID], c)
	}
	// 每个文章的评论按点赞排序，取 Top 10
	for aid, cs := range commentMap {
		sort.Slice(cs, func(i, j int) bool { return cs[i].LikeCount > cs[j].LikeCount })
		if len(cs) > 10 {
			cs = cs[:10]
		}
		commentMap[aid] = cs
	}

	// 分批
	type batch struct {
		start int
		arts  []model.Article
	}
	var batches []batch
	for i := 0; i < len(articles); i += deepBatchSize {
		end := i + deepBatchSize
		if end > len(articles) {
			end = len(articles)
		}
		batches = append(batches, batch{start: i, arts: articles[i:end]})
	}

	results := make([][]ArticleInsight, len(batches))
	var wg sync.WaitGroup
	sem := make(chan struct{}, deepConcurrency)

	for bi, b := range batches {
		wg.Add(1)
		sem <- struct{}{}
		go func(idx int, bt batch) {
			defer wg.Done()
			defer func() { <-sem }()
			results[idx] = s.deepAnalyzeBatch(ctx, bt.arts, commentMap, stats, cfg, tc)
		}(bi, b)
	}
	wg.Wait()

	var all []ArticleInsight
	for _, r := range results {
		all = append(all, r...)
	}

	// 计算认同度
	for i := range all {
		aid := all[i].ArticleID
		var endorsement float64
		if cs, ok := commentMap[aid]; ok {
			for _, c := range cs {
				endorsement += float64(c.LikeCount + 1)
			}
		}
		all[i].Endorsement = endorsement
	}

	log.Printf("[DeepAnalysis] 全量分析完成 — %d 篇文章产出 %d 条洞察", len(articles), len(all))
	return all
}

// deepAnalyzeBatch 单批深度分析
func (s *Service) deepAnalyzeBatch(ctx context.Context, arts []model.Article, commentMap map[uint][]model.ArticleComment, stats crawlStats, cfg config.TaggerConfig, tc *TokenCounter) []ArticleInsight {
	var sb strings.Builder
	sb.WriteString("你是资深用户研究专家。请对以下每篇内容（含其评论）做深度需求挖掘。\n\n")
	sb.WriteString("要求：\n")
	sb.WriteString("- 基于内容和评论的真实表述，提炼用户的潜在需求和痛点\n")
	sb.WriteString("- evidence 字段必须引用原文/评论中的原话片段（10-30字），用于验证结论真实性\n")
	sb.WriteString("- 如果评论中有不同于文章的补充信号（需求、抱怨、建议），提取到 commentSignals\n")
	sb.WriteString("- 不要推测没有依据的内容，只从原文和评论中提取\n\n")
	sb.WriteString("以JSON数组返回，每篇格式：\n")
	sb.WriteString(`{"id":序号,"coreNeed":"核心诉求(25-40字)","painPoints":["痛点1","痛点2"],"suggestions":["建议1","建议2"],"sentiment":"positive/neutral/negative","intensity":1-5,"evidence":["原文引用1","原文引用2"],"commentSignals":["评论信号1"]}`)
	sb.WriteString("\n\n")

	for i, a := range arts {
		content := a.Content
		if len([]rune(content)) > 400 {
			content = string([]rune(content)[:400])
		}
		sb.WriteString(fmt.Sprintf("--- 文章 %d ---\n", i+1))
		sb.WriteString(fmt.Sprintf("标题：%s\n平台：%s\n情感：%s（%.2f）\n", a.Title, a.Platform, a.Sentiment, a.SentScore))
		if content != "" {
			sb.WriteString(fmt.Sprintf("内容：%s\n", content))
		}

		if cs, ok := commentMap[a.ID]; ok && len(cs) > 0 {
			sb.WriteString("评论：\n")
			for ci, c := range cs {
				cContent := c.Content
				if len([]rune(cContent)) > 100 {
					cContent = string([]rune(cContent)[:100])
				}
				sb.WriteString(fmt.Sprintf("  %d. [赞%d] %s\n", ci+1, c.LikeCount, cContent))
			}
		}
		sb.WriteString("\n")
	}

	resp, err := callLLMTracked(ctx, sb.String(), cfg, 1500, tc)
	if err != nil {
		log.Printf("[DeepAnalysis] batch LLM failed: %v", err)
		return s.buildFallbackInsights(arts, stats)
	}

	resp = strings.TrimSpace(resp)
	if idx := strings.Index(resp, "["); idx >= 0 {
		resp = resp[idx:]
	}
	if idx := strings.LastIndex(resp, "]"); idx >= 0 {
		resp = resp[:idx+1]
	}

	var parsed []struct {
		ID             int      `json:"id"`
		CoreNeed       string   `json:"coreNeed"`
		PainPoints     []string `json:"painPoints"`
		Suggestions    []string `json:"suggestions"`
		Sentiment      string   `json:"sentiment"`
		Intensity      int      `json:"intensity"`
		Evidence       []string `json:"evidence"`
		CommentSignals []string `json:"commentSignals"`
	}
	if err := json.Unmarshal([]byte(resp), &parsed); err != nil {
		log.Printf("[DeepAnalysis] parse batch JSON failed: %v", err)
		return s.buildFallbackInsights(arts, stats)
	}

	insightMap := make(map[int]*ArticleInsight, len(parsed))
	for _, p := range parsed {
		insightMap[p.ID] = &ArticleInsight{
			CoreNeed:       p.CoreNeed,
			PainPoints:     p.PainPoints,
			Suggestions:    p.Suggestions,
			Sentiment:      p.Sentiment,
			Intensity:      p.Intensity,
			Evidence:       p.Evidence,
			CommentSignals: p.CommentSignals,
		}
	}

	insights := make([]ArticleInsight, 0, len(arts))
	for i, a := range arts {
		ageDays := int(stats.RefDate.Sub(a.PublishedAt).Hours() / 24)
		ins := ArticleInsight{
			ArticleID:  a.ID,
			Title:      a.Title,
			Platform:   a.Platform,
			Sentiment:  a.Sentiment,
			AgeDays:    ageDays,
			TimeWeight: timeWeight(ageDays),
		}
		if p, ok := insightMap[i+1]; ok {
			ins.CoreNeed = p.CoreNeed
			ins.PainPoints = p.PainPoints
			ins.Suggestions = p.Suggestions
			if p.Sentiment != "" {
				ins.Sentiment = p.Sentiment
			}
			ins.Intensity = p.Intensity
			ins.Evidence = p.Evidence
			ins.CommentSignals = p.CommentSignals
		} else {
			ins.CoreNeed = a.Title
		}
		insights = append(insights, ins)
	}
	return insights
}

func (s *Service) buildFallbackInsights(arts []model.Article, stats crawlStats) []ArticleInsight {
	insights := make([]ArticleInsight, len(arts))
	for i, a := range arts {
		ageDays := int(stats.RefDate.Sub(a.PublishedAt).Hours() / 24)
		insights[i] = ArticleInsight{
			ArticleID:  a.ID,
			Title:      a.Title,
			Platform:   a.Platform,
			CoreNeed:   a.Title,
			Sentiment:  a.Sentiment,
			AgeDays:    ageDays,
			TimeWeight: timeWeight(ageDays),
		}
	}
	return insights
}

// clusterInsights 语义聚类（步骤③）
func (s *Service) clusterInsights(ctx context.Context, insights []ArticleInsight, cfg config.TaggerConfig, tc *TokenCounter) []InsightCluster {
	var sb strings.Builder
	sb.WriteString("你是资深用户研究专家。以下是用户的诉求/痛点洞察列表，请按「用户核心诉求」将它们聚类为5-8个话题组。\n\n")
	sb.WriteString("话题命名规则：\n")
	sb.WriteString("- 以用户诉求/痛点命名，如「XX功能体验差急需优化」「XX机制不合理引发不满」\n")
	sb.WriteString("- 话题名8-15字，直接体现诉求本质\n")
	sb.WriteString("- 每条洞察只属于一个话题\n\n")
	sb.WriteString("以JSON返回：{\"clusters\":[{\"topic\":\"话题名\",\"ids\":[序号...],\"verdict\":\"一句话核心结论\",\"action\":\"建议行动\"}]}\n\n")
	sb.WriteString("洞察列表：\n")

	for i, ins := range insights {
		if i >= 80 {
			sb.WriteString(fmt.Sprintf("...（省略剩余%d条）\n", len(insights)-80))
			break
		}
		sb.WriteString(fmt.Sprintf("%d. [%s/%s] %s\n", i+1, ins.Platform, ins.Sentiment, ins.CoreNeed))
	}

	resp, err := callLLMTracked(ctx, sb.String(), cfg, 1200, tc)
	if err != nil {
		log.Printf("[DeepAnalysis] cluster LLM failed: %v", err)
		return s.fallbackCluster(insights)
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
			IDs     []int  `json:"ids"`
			Verdict string `json:"verdict"`
			Action  string `json:"action"`
		} `json:"clusters"`
	}
	if err := json.Unmarshal([]byte(resp), &parsed); err != nil {
		log.Printf("[DeepAnalysis] parse cluster JSON failed: %v", err)
		return s.fallbackCluster(insights)
	}

	var clusters []InsightCluster
	for _, cl := range parsed.Clusters {
		if cl.Topic == "" || len(cl.IDs) == 0 {
			continue
		}
		cluster := InsightCluster{
			Topic:       cl.Topic,
			CoreVerdict: cl.Verdict,
			ActionItem:  cl.Action,
		}
		for _, idx := range cl.IDs {
			if idx >= 1 && idx <= len(insights) {
				cluster.Insights = append(cluster.Insights, insights[idx-1])
			}
		}
		clusters = append(clusters, cluster)
	}

	sort.Slice(clusters, func(i, j int) bool { return len(clusters[i].Insights) > len(clusters[j].Insights) })
	log.Printf("[DeepAnalysis] 聚类完成 — %d 个话题组", len(clusters))
	return clusters
}

func (s *Service) fallbackCluster(insights []ArticleInsight) []InsightCluster {
	return []InsightCluster{{
		Topic:       "综合诉求",
		CoreVerdict: "全量分析聚类失败，以下为未分类洞察",
		Insights:    insights,
		RiskLevel:   "medium",
	}}
}

// evaluateAndRank LLM质量评估+排序（步骤④）
func (s *Service) evaluateAndRank(ctx context.Context, clusters []InsightCluster, cfg config.TaggerConfig, tc *TokenCounter) []InsightCluster {
	var wg sync.WaitGroup
	sem := make(chan struct{}, deepConcurrency)

	for ci := range clusters {
		if len(clusters[ci].Insights) == 0 {
			continue
		}
		wg.Add(1)
		sem <- struct{}{}
		go func(idx int) {
			defer wg.Done()
			defer func() { <-sem }()
			s.evaluateCluster(ctx, &clusters[idx], cfg, tc)
		}(ci)
	}
	wg.Wait()

	// 过滤低质量 + 排序
	for ci := range clusters {
		var filtered []ArticleInsight
		for _, ins := range clusters[ci].Insights {
			if ins.DecisionValue >= 3 {
				filtered = append(filtered, ins)
			}
		}
		sort.Slice(filtered, func(i, j int) bool {
			scoreI := float64(filtered[i].DecisionValue) * filtered[i].Endorsement * filtered[i].TimeWeight
			scoreJ := float64(filtered[j].DecisionValue) * filtered[j].Endorsement * filtered[j].TimeWeight
			return scoreI > scoreJ
		})
		clusters[ci].Insights = filtered
	}

	// 提取 KeyFindings（每组取前3-5个 coreNeed）
	for ci := range clusters {
		var findings []string
		for i, ins := range clusters[ci].Insights {
			if i >= 5 {
				break
			}
			findings = append(findings, ins.CoreNeed)
		}
		clusters[ci].KeyFindings = findings

		// 推断风险等级
		negCount := 0
		for _, ins := range clusters[ci].Insights {
			if ins.Sentiment == "negative" || ins.Intensity >= 4 {
				negCount++
			}
		}
		total := len(clusters[ci].Insights)
		if total > 0 {
			negRate := float64(negCount) / float64(total)
			if negRate > 0.6 {
				clusters[ci].RiskLevel = "high"
			} else if negRate > 0.3 {
				clusters[ci].RiskLevel = "medium"
			} else {
				clusters[ci].RiskLevel = "low"
			}
		}
	}

	// 去掉空 cluster
	var result []InsightCluster
	for _, cl := range clusters {
		if len(cl.Insights) > 0 {
			result = append(result, cl)
		}
	}
	log.Printf("[DeepAnalysis] 评估排序完成 — 保留 %d 个话题组", len(result))
	return result
}

func (s *Service) evaluateCluster(ctx context.Context, cluster *InsightCluster, cfg config.TaggerConfig, tc *TokenCounter) {
	if len(cluster.Insights) == 0 {
		return
	}

	var sb strings.Builder
	sb.WriteString("你是信息质量评审官。以下是同一话题下的洞察列表，请对每条评估其决策价值。\n\n")
	sb.WriteString("评分标准：\n")
	sb.WriteString("5分 = 揭示了明确的用户需求/痛点，有行动指导意义，有原文佐证\n")
	sb.WriteString("4分 = 信息有价值，能帮助理解用户态度\n")
	sb.WriteString("3分 = 一般性信息，价值中等\n")
	sb.WriteString("2分 = 过于笼统或缺乏具体指向\n")
	sb.WriteString("1分 = 噪音/重复/无信息量\n\n")
	sb.WriteString(fmt.Sprintf("话题：%s\n", cluster.Topic))
	sb.WriteString("以JSON数组返回：[{\"id\":序号,\"score\":分数}]\n\n")

	for i, ins := range cluster.Insights {
		evidence := ""
		if len(ins.Evidence) > 0 {
			evidence = " [佐证:" + strings.Join(ins.Evidence, ";") + "]"
		}
		sb.WriteString(fmt.Sprintf("%d. %s%s\n", i+1, ins.CoreNeed, evidence))
	}

	resp, err := callLLMTracked(ctx, sb.String(), cfg, 600, tc)
	if err != nil {
		log.Printf("[DeepAnalysis] evaluate LLM failed for %s: %v", cluster.Topic, err)
		for i := range cluster.Insights {
			cluster.Insights[i].DecisionValue = 3
		}
		return
	}

	resp = strings.TrimSpace(resp)
	if idx := strings.Index(resp, "["); idx >= 0 {
		resp = resp[idx:]
	}
	if idx := strings.LastIndex(resp, "]"); idx >= 0 {
		resp = resp[:idx+1]
	}

	var scores []struct {
		ID    int `json:"id"`
		Score int `json:"score"`
	}
	if err := json.Unmarshal([]byte(resp), &scores); err != nil {
		log.Printf("[DeepAnalysis] parse evaluate JSON failed: %v", err)
		for i := range cluster.Insights {
			cluster.Insights[i].DecisionValue = 3
		}
		return
	}

	scoreMap := make(map[int]int, len(scores))
	for _, s := range scores {
		scoreMap[s.ID] = s.Score
	}
	for i := range cluster.Insights {
		if score, ok := scoreMap[i+1]; ok && score >= 1 && score <= 5 {
			cluster.Insights[i].DecisionValue = score
		} else {
			cluster.Insights[i].DecisionValue = 3
		}
	}
}

// convertToReportData 将深度分析结果转换为现有报告渲染结构
func (s *Service) convertToReportData(clusters []InsightCluster, stats *crawlStats) (map[string]*TopicSummaryStructured, map[string]string) {
	topicSummaries := make(map[string]*TopicSummaryStructured, len(clusters))
	groupSummaries := make(map[string]string, len(clusters))

	// 重建 topGroups 和 topicSentiment
	stats.TopGroups = nil
	stats.TopicSentiment = make(map[string]map[string]int)

	for _, cl := range clusters {
		ts := &TopicSummaryStructured{
			CoreSummary: cl.CoreVerdict,
			KeyFindings: cl.KeyFindings,
			RiskLevel:   cl.RiskLevel,
			Opportunity: cl.ActionItem,
		}
		if cl.RiskLevel == "high" {
			ts.RiskNote = "高风险：大量负面反馈集中"
		}
		topicSummaries[cl.Topic] = ts

		// 文本版
		var text strings.Builder
		text.WriteString(cl.CoreVerdict + "\n")
		for _, f := range cl.KeyFindings {
			text.WriteString("- " + f + "\n")
		}
		if cl.ActionItem != "" {
			text.WriteString("行动建议：" + cl.ActionItem + "\n")
		}
		groupSummaries[cl.Topic] = text.String()

		// 构建 groupStats
		sentMap := make(map[string]int)
		var ws float64
		for _, ins := range cl.Insights {
			incSentiment(sentMap, ins.Sentiment)
			ws += ins.TimeWeight
		}
		stats.TopicSentiment[cl.Topic] = sentMap

		grp := groupStats{
			Topic:         cl.Topic,
			Count:         len(cl.Insights),
			WeightedScore: ws,
		}
		stats.TopGroups = append(stats.TopGroups, grp)
	}

	return topicSummaries, groupSummaries
}

// deepCommentAnalysis 深度模式下按 cluster 精确归类评论，生成 CommentAnalysis
func (s *Service) deepCommentAnalysis(ctx context.Context, clusters []InsightCluster, comments []model.ArticleComment, articles []model.Article, cfg config.TaggerConfig, tc *TokenCounter) *CommentAnalysis {
	if len(comments) == 0 {
		return nil
	}

	articleMap := make(map[uint]model.Article, len(articles))
	for _, a := range articles {
		articleMap[a.ID] = a
	}

	result := &CommentAnalysis{
		PlatformCount: make(map[string]int),
	}

	// 整体统计
	for _, c := range comments {
		if a, ok := articleMap[c.ArticleID]; ok {
			result.PlatformCount[a.Platform]++
		}
	}

	// 按日统计趋势
	dailyMap := make(map[string]*commentTrendPoint)
	for _, c := range comments {
		var dateKey string
		if a, ok := articleMap[c.ArticleID]; ok && !a.PublishedAt.IsZero() {
			dateKey = a.PublishedAt.Format("2006-01-02")
		} else if !c.PublishedAt.IsZero() {
			dateKey = c.PublishedAt.Format("2006-01-02")
		} else {
			dateKey = c.CreatedAt.Format("2006-01-02")
		}
		if dailyMap[dateKey] == nil {
			dailyMap[dateKey] = &commentTrendPoint{Date: dateKey}
		}
		dailyMap[dateKey].Total++
	}
	var dailyKeys []string
	for k := range dailyMap {
		dailyKeys = append(dailyKeys, k)
	}
	sort.Strings(dailyKeys)
	for _, k := range dailyKeys {
		result.DailyTrend = append(result.DailyTrend, *dailyMap[k])
	}

	// 按 cluster 精确归类评论（通过 articleID 关联）
	clusterArticleIDs := make(map[string]map[uint]bool, len(clusters))
	for _, cl := range clusters {
		ids := make(map[uint]bool)
		for _, ins := range cl.Insights {
			ids[ins.ArticleID] = true
		}
		clusterArticleIDs[cl.Topic] = ids
	}

	// 每个 cluster 对应的评论
	topicComments := make(map[string][]model.ArticleComment)
	for _, c := range comments {
		for topic, aids := range clusterArticleIDs {
			if aids[c.ArticleID] {
				topicComments[topic] = append(topicComments[topic], c)
				break // 每条评论只归一个 cluster
			}
		}
	}

	// LLM 分析每个 cluster 的评论观点（全量，不限 sampleSize）
	var views []TopicCommentView
	var wg sync.WaitGroup
	var mu sync.Mutex
	sem := make(chan struct{}, deepConcurrency)

	for _, cl := range clusters {
		clComments := topicComments[cl.Topic]
		if len(clComments) == 0 {
			continue
		}
		wg.Add(1)
		sem <- struct{}{}
		go func(topic string, cs []model.ArticleComment) {
			defer wg.Done()
			defer func() { <-sem }()
			view := s.analyzeClusterComments(ctx, topic, cs, cfg, tc)
			mu.Lock()
			views = append(views, view)
			mu.Unlock()
		}(cl.Topic, clComments)
	}
	wg.Wait()

	// 按评论数排序
	sort.Slice(views, func(i, j int) bool { return views[i].CommentCount > views[j].CommentCount })
	result.TopicComments = views

	// 整体情感（从所有 cluster 评论汇总）
	var totalPos, totalNeu, totalNeg int
	for _, v := range views {
		totalPos += v.Sentiment.Positive
		totalNeu += v.Sentiment.Neutral
		totalNeg += v.Sentiment.Negative
	}
	total := totalPos + totalNeu + totalNeg
	result.OverallSentiment = CommentSentiment{
		Positive: totalPos,
		Neutral:  totalNeu,
		Negative: totalNeg,
	}
	if total > 0 {
		result.OverallSentiment.PosRate = float64(totalPos) / float64(total) * 100
		result.OverallSentiment.NeuRate = float64(totalNeu) / float64(total) * 100
		result.OverallSentiment.NegRate = float64(totalNeg) / float64(total) * 100
	}

	// 回填趋势情感
	if total > 0 {
		posR := float64(totalPos) / float64(total)
		negR := float64(totalNeg) / float64(total)
		for i := range result.DailyTrend {
			pt := &result.DailyTrend[i]
			pt.Positive = int(float64(pt.Total)*posR + 0.5)
			pt.Negative = int(float64(pt.Total)*negR + 0.5)
			pt.Neutral = pt.Total - pt.Positive - pt.Negative
			if pt.Neutral < 0 {
				pt.Neutral = 0
			}
		}
	}

	// 热评 Top 10
	sorted := make([]model.ArticleComment, len(comments))
	copy(sorted, comments)
	sort.Slice(sorted, func(i, j int) bool { return sorted[i].LikeCount > sorted[j].LikeCount })
	if len(sorted) > 10 {
		sorted = sorted[:10]
	}
	for _, c := range sorted {
		platform := ""
		if a, ok := articleMap[c.ArticleID]; ok {
			platform = a.Platform
		}
		content := c.Content
		if len([]rune(content)) > 150 {
			content = string([]rune(content)[:150]) + "..."
		}
		result.HotComments = append(result.HotComments, HotComment{
			Content:   content,
			Nickname:  c.Nickname,
			Platform:  platform,
			LikeCount: c.LikeCount,
			Sentiment: "neutral",
		})
	}

	log.Printf("[DeepAnalysis] 评论归类完成 — %d 个话题, 总评论 %d 条", len(views), len(comments))
	return result
}

// analyzeClusterComments 对单个 cluster 的全量评论做 LLM 分析
func (s *Service) analyzeClusterComments(ctx context.Context, topic string, comments []model.ArticleComment, cfg config.TaggerConfig, tc *TokenCounter) TopicCommentView {
	view := TopicCommentView{
		Topic:        topic,
		CommentCount: len(comments),
	}

	// 按点赞排序，取 Top 30 做 LLM 分析（全量可能超出单次 token 限制）
	sorted := make([]model.ArticleComment, len(comments))
	copy(sorted, comments)
	sort.Slice(sorted, func(i, j int) bool { return sorted[i].LikeCount > sorted[j].LikeCount })
	sample := sorted
	if len(sample) > 30 {
		sample = sample[:30]
	}

	var sb strings.Builder
	sb.WriteString(fmt.Sprintf("你是资深用户研究专家。以下是「%s」话题下的全部%d条评论（按点赞排序，展示前%d条）。\n\n", topic, len(comments), len(sample)))
	sb.WriteString("请分析：\n")
	sb.WriteString("1. 统计样本中 正面/中性/负面 各多少条（整数）\n")
	sb.WriteString("2. 提炼5-8个代表性观点（每条20-30字，具体有信息量，说明用户关注什么、持什么立场）\n")
	sb.WriteString("3. 深度解析3-5条（挖掘评论背后的隐含诉求、矛盾焦点、趋势信号）\n")
	sb.WriteString("4. 主要情绪标签3-5个\n\n")
	sb.WriteString("以JSON返回：{\"pos\":正面条数,\"neu\":中性条数,\"neg\":负面条数,\"opinions\":[...],\"deepInsights\":[...],\"mainEmotions\":[...]}\n\n")

	for i, c := range sample {
		content := c.Content
		if len([]rune(content)) > 100 {
			content = string([]rune(content)[:100]) + "..."
		}
		sb.WriteString(fmt.Sprintf("%d. [赞%d] %s\n", i+1, c.LikeCount, content))
	}

	resp, err := callLLMTracked(ctx, sb.String(), cfg, 600, tc)
	if err != nil {
		log.Printf("[DeepAnalysis] cluster comment LLM failed for %s: %v", topic, err)
		return view
	}

	var parsed struct {
		Pos          float64  `json:"pos"`
		Neu          float64  `json:"neu"`
		Neg          float64  `json:"neg"`
		Opinions     []string `json:"opinions"`
		DeepInsights []string `json:"deepInsights"`
		MainEmotions []string `json:"mainEmotions"`
	}
	resp = strings.TrimSpace(resp)
	if idx := strings.Index(resp, "{"); idx >= 0 {
		resp = resp[idx:]
	}
	if idx := strings.LastIndex(resp, "}"); idx >= 0 {
		resp = resp[:idx+1]
	}
	if err := json.Unmarshal([]byte(resp), &parsed); err != nil {
		log.Printf("[DeepAnalysis] parse cluster comment JSON failed for %s: %v", topic, err)
		return view
	}

	sampleTotal := parsed.Pos + parsed.Neu + parsed.Neg
	if sampleTotal > 0 {
		scale := float64(view.CommentCount) / sampleTotal
		scaledPos := int(parsed.Pos*scale + 0.5)
		scaledNeu := int(parsed.Neu*scale + 0.5)
		scaledNeg := view.CommentCount - scaledPos - scaledNeu
		if scaledNeg < 0 {
			scaledNeg = 0
		}
		view.Sentiment = CommentSentiment{
			Positive: scaledPos,
			Neutral:  scaledNeu,
			Negative: scaledNeg,
			PosRate:  parsed.Pos / sampleTotal * 100,
			NeuRate:  parsed.Neu / sampleTotal * 100,
			NegRate:  parsed.Neg / sampleTotal * 100,
		}
	}
	view.KeyOpinions = parsed.Opinions
	view.DeepInsights = parsed.DeepInsights
	view.MainEmotions = parsed.MainEmotions
	return view
}
