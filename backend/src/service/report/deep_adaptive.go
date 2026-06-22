package report

import (
	"context"
	"encoding/json"
	"fmt"
	"math"
	"sort"
	"strings"
	"sync"
	"time"
	"unicode"

	"opinion-analysis/config"
	"opinion-analysis/src/model"
)

// ═══════════════════════════════════════════════════════════════════════════
// 深度分析自适应预算架构
// 阶段0: 质量预过滤 (零成本规则)
// 阶段1: 轻量打分 (规则+模糊区LLM)
// 阶段3: 预算分配器 (按重要性加权, 上限200k)
// ═══════════════════════════════════════════════════════════════════════════

// ── 阶段0: 质量预过滤 ──

// prefilterArticles 文章质量预过滤（零LLM，纯规则快速丢弃噪音）
func prefilterArticles(articles []model.Article, refDate time.Time) ([]model.Article, StageStat) {
	stat := StageStat{Name: "预过滤(文章)", Kind: "filter", Attempted: len(articles)}

	filtered := make([]model.Article, 0, len(articles))
	var tooThin, tooOld, noContent int

	for _, a := range articles {
		contentLen := len([]rune(a.Content))
		titleLen := len([]rune(a.Title))
		ageDays := int(refDate.Sub(a.PublishedAt).Hours() / 24)

		// 规则A: 内容<30字 且 标题<10字
		if contentLen < 30 && titleLen < 10 {
			tooThin++
			continue
		}

		// 规则B: 内容几乎为空 (全是空白/标点)
		if contentLen < 15 && titleLen == 0 {
			noContent++
			continue
		}

		// 规则C: 发布>365天 且 内容稀薄
		if ageDays > 365 && contentLen < 50 {
			tooOld++
			continue
		}

		filtered = append(filtered, a)
	}

	stat.Succeeded = len(filtered)
	var parts []string
	if tooThin > 0 {
		parts = append(parts, "过短"+itoa(tooThin))
	}
	if noContent > 0 {
		parts = append(parts, "无内容"+itoa(noContent))
	}
	if tooOld > 0 {
		parts = append(parts, "陈旧"+itoa(tooOld))
	}
	stat.Note = strings.Join(parts, "，")

	return filtered, stat
}

// prefilterComments 评论质量预过滤（增强版，加纯emoji/复读机检测）
func prefilterComments(comments []model.ArticleComment) ([]model.ArticleComment, StageStat) {
	stat := StageStat{Name: "预过滤(评论)", Kind: "filter", Attempted: len(comments)}

	filtered := make([]model.ArticleComment, 0, len(comments))
	seen := make(map[string]struct{})
	var noise, emoji, repeat int

	for _, c := range comments {
		// 规则1: 现有噪音规则
		if isNoiseComment(c.Content) {
			noise++
			continue
		}

		// 规则2: 纯emoji判断（>50%字符是emoji/符号）
		runes := []rune(c.Content)
		letterCount := 0
		for _, r := range runes {
			if unicode.IsLetter(r) || unicode.IsNumber(r) {
				letterCount++
			}
		}
		if len(runes) > 0 && float64(letterCount)/float64(len(runes)) < 0.5 {
			emoji++
			continue
		}

		// 规则3: 复读机（normalizeComment后精确去重）
		key := normalizeComment(c.Content)
		if _, ok := seen[key]; ok {
			repeat++
			continue
		}
		seen[key] = struct{}{}

		filtered = append(filtered, c)
	}

	stat.Succeeded = len(filtered)
	var parts []string
	if noise > 0 {
		parts = append(parts, "停用词"+itoa(noise))
	}
	if emoji > 0 {
		parts = append(parts, "纯表情"+itoa(emoji))
	}
	if repeat > 0 {
		parts = append(parts, "重复"+itoa(repeat))
	}
	stat.Note = strings.Join(parts, "，")

	return filtered, stat
}

// ── 阶段1: 轻量打分 ──

// articleScoreRule 规则打分（0-10分，不依赖评论数/点赞数）
// 基于：内容充分度 + 时效 + 情感强度 + 标签丰富度
func articleScoreRule(a model.Article, refDate time.Time) float64 {
	contentLen := float64(len([]rune(a.Content)))
	titleLen := float64(len([]rune(a.Title)))
	ageDays := int(refDate.Sub(a.PublishedAt).Hours() / 24)
	tw := timeWeight(ageDays)

	// 内容充分度 (0-4分): 长内容更可能有观点
	contentScore := math.Min(4.0, (contentLen/200.0)*2.0+(titleLen/20.0)*0.5)

	// 时效 (0-2分): 新内容优先
	timeScore := tw * 2.0

	// 情感强度 (0-3分): 明确正面/负面比中性有价值
	sentScore := 0.0
	switch a.Sentiment {
	case "positive":
		sentScore = 1.5 + (a.SentScore * 1.5) // 0.8分→2.7, 0.6→2.4
	case "negative":
		sentScore = 1.5 + ((1.0 - a.SentScore) * 1.5) // 0.2分→2.7, 0.3→2.55
	default:
		sentScore = 0.5 // 中性基础分
	}

	// 标签丰富度 (0-1分): 多标签说明内容信息量大
	tagScore := 0.0
	if a.AITags != nil && *a.AITags != "" {
		var tags []string
		if json.Unmarshal([]byte(*a.AITags), &tags) == nil {
			tagScore = math.Min(1.0, float64(len(tags))*0.25)
		}
	}

	total := contentScore + timeScore + sentScore + tagScore
	return math.Min(10.0, total)
}

// lightweightScoreArticles 轻量打分：规则粗筛 + 模糊区LLM细化
func (s *Service) lightweightScoreArticles(ctx context.Context, articles []model.Article, refDate time.Time, cfg config.TaggerConfig, tc *TokenCounter, metrics *PipelineMetrics) []float64 {
	n := len(articles)
	scores := make([]float64, n)

	// 阶段1: 全量规则打分
	ruleScores := make([]float64, n)
	for i, a := range articles {
		ruleScores[i] = articleScoreRule(a, refDate)
	}

	// 阶段2: 识别模糊区 (3.5-6.5分之间的40%数据)
	sorted := make([]float64, n)
	copy(sorted, ruleScores)
	sort.Float64s(sorted)

	fuzzyLow := 3.5
	fuzzyHigh := 6.5
	if n > 10 {
		p30 := sorted[n*3/10]
		p70 := sorted[n*7/10]
		if p30 < fuzzyLow {
			fuzzyLow = p30
		}
		if p70 > fuzzyHigh {
			fuzzyHigh = p70
		}
	}

	var fuzzyIndices []int
	for i, rs := range ruleScores {
		if rs >= fuzzyLow && rs <= fuzzyHigh {
			fuzzyIndices = append(fuzzyIndices, i)
		}
	}

	// 阶段3: 模糊区LLM细化（并发批量，每批20篇）
	llmBonus := make(map[int]float64) // articleIndex → +0到+2的加分
	if len(fuzzyIndices) > 0 {
		const batchSize = 20
		type batchJob struct{ start, end int }
		var batches []batchJob
		for i := 0; i < len(fuzzyIndices); i += batchSize {
			end := i + batchSize
			if end > len(fuzzyIndices) {
				end = len(fuzzyIndices)
			}
			batches = append(batches, batchJob{i, end})
		}

		var wg sync.WaitGroup
		var mu sync.Mutex
		sem := make(chan struct{}, 5)
		okCount := 0

		for _, bt := range batches {
			wg.Add(1)
			sem <- struct{}{}
			go func(b batchJob) {
				defer wg.Done()
				defer func() { <-sem }()

				batchIndices := fuzzyIndices[b.start:b.end]
				batchArts := make([]model.Article, len(batchIndices))
				for j, idx := range batchIndices {
					batchArts[j] = articles[idx]
				}

				bonus, ok := s.llmScoreFuzzyBatch(ctx, batchArts, cfg, tc)
				mu.Lock()
				if ok {
					for j, idx := range batchIndices {
						llmBonus[idx] = bonus[j]
					}
					okCount++
				}
				mu.Unlock()
			}(bt)
		}
		wg.Wait()

		if metrics != nil {
			metrics.Record(StageStat{
				Name:      "轻量打分(LLM细化)",
				Attempted: len(batches),
				Succeeded: okCount,
				Note:      fmt.Sprintf("规则全量+LLM细化%d/%d篇", len(fuzzyIndices), n),
			})
		}
	}

	// 阶段4: 合成最终分数
	for i := range scores {
		scores[i] = ruleScores[i]
		if bonus, ok := llmBonus[i]; ok {
			scores[i] += bonus
			if scores[i] > 10.0 {
				scores[i] = 10.0
			}
		}
	}

	return scores
}

// llmScoreFuzzyBatch 对模糊区文章批量调用轻量LLM，判断是否有明确观点/痛点
func (s *Service) llmScoreFuzzyBatch(ctx context.Context, arts []model.Article, cfg config.TaggerConfig, tc *TokenCounter) ([]float64, bool) {
	var sb strings.Builder
	sb.WriteString("你是内容质量评估员。对以下文章逐一判断：是否包含**明确的观点/痛点/建议**？\n")
	sb.WriteString("有明确观点→1分，内容笼统/纯转发/无观点→0分。\n")
	sb.WriteString("返回JSON数组：[{\"id\":序号,\"hasOpinion\":0或1}]\n")
	sb.WriteString(jsonArrayContract)
	sb.WriteString("\n")

	for i, a := range arts {
		content := a.Content
		if len([]rune(content)) > 150 {
			content = string([]rune(content)[:150]) + "..."
		}
		sb.WriteString(fmt.Sprintf("%d. [%s] %s", i+1, a.Platform, a.Title))
		if content != "" {
			sb.WriteString(" — " + content)
		}
		sb.WriteString("\n")
	}

	var parsed []struct {
		ID         int `json:"id"`
		HasOpinion int `json:"hasOpinion"`
	}
	// 输出预算：每篇需要~30 tokens ({"id":1,"hasOpinion":1},)，基础600留prompt开销
	outputBudget := 600 + len(arts)*35
	if outputBudget < 800 {
		outputBudget = 800
	}
	if outputBudget > 2000 {
		outputBudget = 2000 // 20篇上限：600+20×35=1300，安全
	}
	r := callLLMJSON(ctx, sb.String(), cfg, outputBudget, tc, "模糊区评估",
		salvageArray, func(s string) error { return json.Unmarshal([]byte(s), &parsed) })

	if !r.OK {
		// 失败时给中等加分(+1分)，不惩罚也不过度奖励
		bonus := make([]float64, len(arts))
		for i := range bonus {
			bonus[i] = 1.0
		}
		return bonus, false
	}

	opMap := make(map[int]int)
	for _, p := range parsed {
		opMap[p.ID] = p.HasOpinion
	}

	bonus := make([]float64, len(arts))
	for i := range arts {
		if opMap[i+1] == 1 {
			bonus[i] = 2.0 // 有观点加2分
		} else {
			bonus[i] = 0.0 // 无观点不加分
		}
	}
	return bonus, true
}

// ── 阶段3: 预算分配器 ──

// topicBudget 单个话题的预算分配结果
type topicBudget struct {
	topicIndex       int
	importance       float64
	articleCount     int
	avgScore         float64
	avgTimeWeight    float64
	allocatedTokens  int // 分配的总token
	deepDiveCount    int // 深挖文章数
	tokensPerArticle int // 每篇文章token预算
	commentSample    int // 评论采样上限
}

// budgetAllocator 预算分配器：按话题重要性(文章数×质量×时效)分配200k预算
func budgetAllocator(clusters []InsightCluster, scores map[uint]float64, globalCap int) []topicBudget {
	if len(clusters) == 0 {
		return nil
	}

	// 步骤1: 计算每个话题的重要性
	type topicMeta struct {
		idx        int
		importance float64
		count      int
		avgScore   float64
		avgTW      float64
	}
	metas := make([]topicMeta, len(clusters))
	totalImportance := 0.0

	for i, cl := range clusters {
		if len(cl.Insights) == 0 {
			continue
		}

		var sumScore, sumTW float64
		for _, ins := range cl.Insights {
			if s, ok := scores[ins.ArticleID]; ok {
				sumScore += s
			} else {
				sumScore += 5.0 // 兜底中等分
			}
			sumTW += ins.TimeWeight
		}
		avgScore := sumScore / float64(len(cl.Insights))
		avgTW := sumTW / float64(len(cl.Insights))

		// 重要性 = 文章数 × 平均分数 × 平均时效权重
		importance := float64(len(cl.Insights)) * avgScore * (avgTW + 0.1)

		metas[i] = topicMeta{i, importance, len(cl.Insights), avgScore, avgTW}
		totalImportance += importance
	}

	// 步骤2: 按重要性比例分配预算，单话题上限50k
	const singleTopicCap = 50000
	budgets := make([]topicBudget, len(clusters))
	usedTokens := 0

	for i, m := range metas {
		if m.count == 0 {
			continue
		}

		rawAlloc := int(float64(globalCap) * (m.importance / totalImportance))
		if rawAlloc > singleTopicCap {
			rawAlloc = singleTopicCap
		}

		// 每篇深挖文章预算：基础3000 + 质量加成(高分→更多token)
		tokensPerArt := 3000 + int(m.avgScore*200) // 10分→5000, 5分→4000, 0分→3000

		// 深挖文章数 = 预算 / 单篇token，但最少3篇(保证小话题不消失)
		deepCount := rawAlloc / tokensPerArt
		if deepCount < 3 {
			deepCount = 3
		}
		if deepCount > m.count {
			deepCount = m.count // 不超过话题总文章数
		}

		actualAlloc := deepCount * tokensPerArt
		usedTokens += actualAlloc

		// 评论采样数：大话题多采样，小话题保底10
		commentSample := 10 + (deepCount * 8) // 3篇→34条, 10篇→90条
		if commentSample > 100 {
			commentSample = 100
		}

		budgets[i] = topicBudget{
			topicIndex:       i,
			importance:       m.importance,
			articleCount:     m.count,
			avgScore:         m.avgScore,
			avgTimeWeight:    m.avgTW,
			allocatedTokens:  actualAlloc,
			deepDiveCount:    deepCount,
			tokensPerArticle: tokensPerArt,
			commentSample:    commentSample,
		}
	}

	// 步骤3: 超限兜底 — 线性缩放，保证每话题≥2篇且单篇≥800 tokens
	if usedTokens > globalCap {
		scale := float64(globalCap) / float64(usedTokens)
		for i := range budgets {
			if budgets[i].deepDiveCount == 0 {
				continue
			}

			budgets[i].deepDiveCount = int(float64(budgets[i].deepDiveCount) * scale)
			if budgets[i].deepDiveCount < 2 {
				budgets[i].deepDiveCount = 2
			}

			budgets[i].tokensPerArticle = int(float64(budgets[i].tokensPerArticle) * scale)
			if budgets[i].tokensPerArticle < 800 {
				budgets[i].tokensPerArticle = 800
			}

			budgets[i].allocatedTokens = budgets[i].deepDiveCount * budgets[i].tokensPerArticle

			// 评论采样同步缩放
			budgets[i].commentSample = int(float64(budgets[i].commentSample) * scale)
			if budgets[i].commentSample < 10 {
				budgets[i].commentSample = 10
			}
		}
	}

	return budgets
}

// ── 阶段5: 评论自适应采样 ──

// deepCommentAnalysisAdaptive 自适应评论采样：按话题预算分配的commentSample上限采样
func (s *Service) deepCommentAnalysisAdaptive(ctx context.Context, clusters []InsightCluster, budgets []topicBudget, comments, rawComments []model.ArticleComment, articles []model.Article, cfg config.TaggerConfig, tc *TokenCounter, metrics *PipelineMetrics) *CommentAnalysis {
	if len(rawComments) == 0 {
		return nil
	}

	articleMap := make(map[uint]model.Article, len(articles))
	for _, a := range articles {
		articleMap[a.ID] = a
	}

	result := &CommentAnalysis{PlatformCount: make(map[string]int)}

	// 整体统计（原始全量评论）
	for _, c := range rawComments {
		if a, ok := articleMap[c.ArticleID]; ok {
			result.PlatformCount[a.Platform]++
		}
	}

	// 趋势（原始全量评论）
	dailyMap := make(map[string]*commentTrendPoint)
	for _, c := range rawComments {
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

	// 按cluster归类评论
	clusterArticleIDs := make(map[string]map[uint]bool, len(clusters))
	for _, cl := range clusters {
		ids := make(map[uint]bool)
		for _, ins := range cl.Insights {
			ids[ins.ArticleID] = true
		}
		clusterArticleIDs[cl.Topic] = ids
	}

	topicComments := make(map[string][]model.ArticleComment)
	for _, c := range comments {
		for topic, aids := range clusterArticleIDs {
			if aids[c.ArticleID] {
				topicComments[topic] = append(topicComments[topic], c)
				break
			}
		}
	}

	// LLM分析每个cluster的评论观点（按budgets[i].commentSample自适应采样）
	var views []TopicCommentView
	var wg sync.WaitGroup
	var mu sync.Mutex
	sem := make(chan struct{}, deepConcurrency)
	attempted, succeeded := 0, 0

	for ti, cl := range clusters {
		clComments := topicComments[cl.Topic]
		if len(clComments) == 0 {
			continue
		}
		attempted++

		sampleCap := 10 // 默认兜底
		if ti < len(budgets) && budgets[ti].commentSample > 0 {
			sampleCap = budgets[ti].commentSample
		}

		wg.Add(1)
		sem <- struct{}{}
		go func(topicIdx int, topic string, cs []model.ArticleComment, cap int) {
			defer wg.Done()
			defer func() { <-sem }()
			view, ok := s.analyzeClusterCommentsAdaptive(ctx, topic, cs, cap, cfg, tc)
			mu.Lock()
			views = append(views, view)
			if ok {
				succeeded++
			}
			mu.Unlock()
		}(ti, cl.Topic, clComments, sampleCap)
	}
	wg.Wait()

	if metrics != nil {
		metrics.Record(StageStat{Name: "评论观点提取", Attempted: attempted, Succeeded: succeeded,
			Note: fmt.Sprintf("%d话题/%d评论", attempted, len(comments))})
	}

	// 整体情感（从所有cluster汇总）
	var totalPos, totalNeu, totalNeg int
	for _, v := range views {
		totalPos += v.Sentiment.Positive
		totalNeu += v.Sentiment.Neutral
		totalNeg += v.Sentiment.Negative
	}
	total := totalPos + totalNeu + totalNeg

	if total == 0 {
		sent := s.llmOverallCommentSentiment(ctx, comments, cfg, tc)
		result.OverallSentiment = sent
		total = sent.Positive + sent.Neutral + sent.Negative
		totalPos, totalNeu, totalNeg = sent.Positive, sent.Neutral, sent.Negative
	} else {
		result.OverallSentiment = CommentSentiment{
			Positive: totalPos, Neutral: totalNeu, Negative: totalNeg,
		}
		if total > 0 {
			result.OverallSentiment.PosRate = float64(totalPos) / float64(total) * 100
			result.OverallSentiment.NeuRate = float64(totalNeu) / float64(total) * 100
			result.OverallSentiment.NegRate = float64(totalNeg) / float64(total) * 100
		}
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

	// 热评Top10（原始全量评论）
	sorted := make([]model.ArticleComment, len(rawComments))
	copy(sorted, rawComments)
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
			Content: content, Nickname: c.Nickname, Platform: platform,
			LikeCount: c.LikeCount, Sentiment: "neutral",
		})
	}

	sort.Slice(views, func(i, j int) bool { return views[i].CommentCount > views[j].CommentCount })
	result.TopicComments = views

	return result
}

// analyzeClusterCommentsAdaptive 自适应采样上限的单话题评论分析
func (s *Service) analyzeClusterCommentsAdaptive(ctx context.Context, topic string, comments []model.ArticleComment, sampleCap int, cfg config.TaggerConfig, tc *TokenCounter) (TopicCommentView, bool) {
	view := TopicCommentView{Topic: topic, CommentCount: len(comments)}

	sorted := make([]model.ArticleComment, len(comments))
	copy(sorted, comments)
	sort.Slice(sorted, func(i, j int) bool { return sorted[i].LikeCount > sorted[j].LikeCount })

	sample := sorted
	if len(sample) > sampleCap {
		sample = sample[:sampleCap]
	}

	// prompt构建与现有analyzeClusterComments一致
	var sb strings.Builder
	sb.WriteString(fmt.Sprintf("你是资深用户研究专家。以下是「%s」话题下的全部%d条评论（按点赞排序，展示前%d条）。\n\n", topic, len(comments), len(sample)))
	sb.WriteString("请分析：\n")
	sb.WriteString("1. 统计样本中 正面/中性/负面 各多少条（整数）\n")
	sb.WriteString("2. 提炼5-8个代表性观点（每条20-30字，具体有信息量）\n")
	sb.WriteString("3. 深度解析3-5条（挖掘评论背后的隐含诉求、矛盾焦点、趋势信号）\n")
	sb.WriteString("4. 主要情绪标签3-5个\n\n")
	sb.WriteString("以JSON返回：{\"pos\":正面条数,\"neu\":中性条数,\"neg\":负面条数,\"opinions\":[...],\"deepInsights\":[...],\"mainEmotions\":[...]}\n")
	sb.WriteString(jsonObjectContract)
	sb.WriteString("\n")

	for i, c := range sample {
		content := c.Content
		if len([]rune(content)) > 100 {
			content = string([]rune(content)[:100]) + "..."
		}
		sb.WriteString(fmt.Sprintf("%d. [赞%d] %s\n", i+1, c.LikeCount, content))
	}

	var parsed struct {
		Pos          float64  `json:"pos"`
		Neu          float64  `json:"neu"`
		Neg          float64  `json:"neg"`
		Opinions     []string `json:"opinions"`
		DeepInsights []string `json:"deepInsights"`
		MainEmotions []string `json:"mainEmotions"`
	}

	// 自适应token：评论观点分析输出很重（deepInsights每条~100字），不能只按样本数线性算
	// 输出预算 = 基础1200 + 每条样本带来的增量20（opinions会更长）
	// 固定开销：情感统计~20 + opinions(6条×50字)~500 + deepInsights(4条×100字)~800 + emotions~50 = 1370
	budget := 1200 + len(sample)*20
	if budget < 1500 {
		budget = 1500 // 最小1500，保证即使少量评论也能完整输出deepInsights
	}
	if budget > 5000 {
		budget = 5000 // 上限5000（100条样本场景）
	}

	r := callLLMJSON(ctx, sb.String(), cfg, budget, tc, "评论观点("+topic+")",
		extractJSONObject, func(s string) error { return json.Unmarshal([]byte(s), &parsed) })
	if !r.OK {
		return view, false
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
			Positive: scaledPos, Neutral: scaledNeu, Negative: scaledNeg,
			PosRate: parsed.Pos / sampleTotal * 100,
			NeuRate: parsed.Neu / sampleTotal * 100,
			NegRate: parsed.Neg / sampleTotal * 100,
		}
	}
	view.KeyOpinions = parsed.Opinions
	view.DeepInsights = parsed.DeepInsights
	view.MainEmotions = parsed.MainEmotions
	return view, true
}
