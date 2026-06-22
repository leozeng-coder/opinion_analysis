package report

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"math"
	"sort"
	"strings"
	"sync"

	"opinion-analysis/config"
	"opinion-analysis/src/model"
	"opinion-analysis/src/service/workflow/nodes"
)

const deepBatchSize = 3 // 需求挖掘每批文章数。挖掘是全流水线输出最重的调用（每篇~600-1000 token），
// 批量过大极易被 max_tokens 截断（unexpected end of JSON input）。3 篇控制单批输出规模，
// 且单批失败只降级 3 篇而非 5 篇。
const deepConcurrency = 5

// jsonArrayContract 强制 LLM 直接输出 JSON 数组、无前言/解释/markdown。
// 防止模型在 [ 之前吐说明文字占掉 max_tokens 预算导致第一个对象就被截断（unexpected end of JSON input）。
const jsonArrayContract = "\n⚠️严格输出要求：直接输出JSON数组本身，第一个字符必须是 [，最后一个字符必须是 ]。" +
	"禁止任何前言、解释、说明、markdown代码块标记(```)。不要输出「以下是分析结果」之类的话。\n"

// jsonObjectContract 同上，针对返回单个 JSON 对象的阶段。
const jsonObjectContract = "\n⚠️严格输出要求：直接输出JSON对象本身，第一个字符必须是 {，最后一个字符必须是 }。" +
	"禁止任何前言、解释、说明、markdown代码块标记(```)。\n"

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

// scaledTokens 按输入单元数缩放 max_tokens，避免大输入被截断。
// base 基线，perItem 每单元增量，n 单元数，cap 上限（DeepSeek 输出上限 8K）。
func scaledTokens(base, perItem, n, cap int) int {
	v := base + perItem*n
	if v > cap {
		v = cap
	}
	if v < base {
		v = base
	}
	return v
}

// llmJSONResult 一次带重试/抢救的 LLM JSON 调用结果
type llmJSONResult struct {
	OK      bool
	Retried int
	Raw     string
}

// callLLMJSON 调用 LLM 并尝试解析 JSON，失败时打印原始响应并重试一次。
// salvage 负责从原始响应中提取/抢救 JSON 子串，parse 负责反序列化（成功返回 nil）。
// 任何失败都会：① Go 控制台输出完整诊断（原始响应、prompt 规模、token 预算）；
// ② 通过 nodes.ProgressFunc(ctx) 推送一行精简告警到前端工作流控制台（前端按 ✗/⚠ 着色）。
// 重试时放大 maxTokens（×1.6，上限 8000），治本「空响应/被截断」——小预算是空响应的主因。
func callLLMJSON(ctx context.Context, prompt string, cfg config.TaggerConfig, maxTokens int, tc *TokenCounter,
	stage string, salvage func(string) string, parse func(string) error) llmJSONResult {

	front := nodes.ProgressFunc(ctx) // 前端工作流控制台通道（未注入时为 no-op）
	res := llmJSONResult{}
	budget := maxTokens
	for attempt := 1; attempt <= 2; attempt++ {
		resp, err := callLLMTracked(ctx, prompt, cfg, budget, tc)
		res.Raw = resp
		if err != nil {
			log.Printf("[DeepAnalysis][%s] LLM 调用失败(尝试%d/2): %v | prompt=%d字 maxTokens=%d",
				stage, attempt, err, len([]rune(prompt)), budget)
			front(fmt.Sprintf("✗ %s LLM调用失败(第%d/2次): %s", stage, attempt, truncRunes(err.Error(), 80)))
			res.Retried = attempt - 1
			budget = growTokens(budget)
			continue
		}
		if strings.TrimSpace(resp) == "" {
			// 空响应：salvage 救不了，几乎都是 token 预算被推理/系统耗尽。直接放大预算重试。
			log.Printf("[DeepAnalysis][%s] LLM 返回空响应(尝试%d/2) | prompt=%d字 maxTokens=%d（疑似输出预算过小，下次放大至%d）",
				stage, attempt, len([]rune(prompt)), budget, growTokens(budget))
			front(fmt.Sprintf("⚠ %s 返回空响应(第%d/2次)，放大输出预算重试", stage, attempt))
			res.Retried = attempt - 1
			budget = growTokens(budget)
			continue
		}
		if perr := parse(salvage(resp)); perr != nil {
			log.Printf("[DeepAnalysis][%s] JSON 解析失败(尝试%d/2): %v | prompt=%d字 maxTokens=%d | 原始响应: %.500s",
				stage, attempt, perr, len([]rune(prompt)), budget, resp)
			front(fmt.Sprintf("⚠ %s JSON解析失败(第%d/2次): %s", stage, attempt, truncRunes(perr.Error(), 60)))
			res.Retried = attempt - 1
			budget = growTokens(budget)
			continue
		}
		res.OK = true
		res.Retried = attempt - 1
		return res
	}
	// 两次都失败：明确告警到前端控制台（红色），让用户知道是哪个环节彻底失败。
	log.Printf("[DeepAnalysis][%s] 两次尝试均失败，进入兜底 | prompt=%d字", stage, len([]rune(prompt)))
	front(fmt.Sprintf("✗ %s 解析生成失败：2次尝试均未成功，已启用兜底", stage))
	res.Retried = 1
	return res
}

// growTokens 放大输出预算（×1.6，上限 8000，DeepSeek 输出硬顶）
func growTokens(n int) int {
	v := int(float64(n) * 1.6)
	if v > 8000 {
		v = 8000
	}
	if v <= n {
		v = n + 200
	}
	return v
}

// truncRunes 按 rune 截断，避免前端告警过长
func truncRunes(s string, max int) string {
	r := []rune(strings.TrimSpace(s))
	if len(r) <= max {
		return string(r)
	}
	return string(r[:max]) + "…"
}

// ── 向量工具（embedding 语义聚类用）──

func cosine(a, b []float32) float64 {
	if len(a) != len(b) || len(a) == 0 {
		return 0
	}
	var dot, na, nb float64
	for i := range a {
		dot += float64(a[i]) * float64(b[i])
		na += float64(a[i]) * float64(a[i])
		nb += float64(b[i]) * float64(b[i])
	}
	if na == 0 || nb == 0 {
		return 0
	}
	return dot / (math.Sqrt(na) * math.Sqrt(nb))
}

func cloneVec(v []float32) []float32 {
	out := make([]float32, len(v))
	copy(out, v)
	return out
}

// deepAnalyzeAll 漏斗③批量分析提炼：仅 high 文章送昂贵 LLM 挖掘，low 走 fallback 降级保留。
// commentsForContext 应为粗过滤后的清洗评论（去噪去重），作为挖掘上下文。
func (s *Service) deepAnalyzeAll(ctx context.Context, high, low []model.Article, commentsForContext []model.ArticleComment, stats crawlStats, cfg config.TaggerConfig, tc *TokenCounter, metrics *PipelineMetrics) []ArticleInsight {
	// 建立 articleID → comments 索引（清洗后评论）
	commentMap := make(map[uint][]model.ArticleComment)
	for _, c := range commentsForContext {
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

	// 分批（仅高价值文章进 LLM）
	type batch struct {
		start int
		arts  []model.Article
	}
	var batches []batch
	for i := 0; i < len(high); i += deepBatchSize {
		end := i + deepBatchSize
		if end > len(high) {
			end = len(high)
		}
		batches = append(batches, batch{start: i, arts: high[i:end]})
	}

	results := make([][]ArticleInsight, len(batches))
	batchOK := make([]bool, len(batches))
	var wg sync.WaitGroup
	sem := make(chan struct{}, deepConcurrency)

	for bi, b := range batches {
		wg.Add(1)
		sem <- struct{}{}
		go func(idx int, bt batch) {
			defer wg.Done()
			defer func() { <-sem }()
			results[idx], batchOK[idx] = s.deepAnalyzeBatch(ctx, bt.arts, commentMap, stats, cfg, tc)
		}(bi, b)
	}
	wg.Wait()

	var all []ArticleInsight
	okCount := 0
	for i, r := range results {
		all = append(all, r...)
		if batchOK[i] {
			okCount++
		}
	}

	// 低价值文章降级保留：不进昂贵 LLM，用标题/情感构造 fallback 洞察，仍参与统计与渲染
	if len(low) > 0 {
		all = append(all, s.buildFallbackInsights(low, stats)...)
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

	if metrics != nil {
		metrics.Record(StageStat{
			Name:      "全量需求挖掘",
			Attempted: len(batches),
			Succeeded: okCount,
			Note:      fmt.Sprintf("高价值%d篇→%d条洞察，降级%d篇", len(high), len(all)-len(low), len(low)),
		})
	}
	log.Printf("[DeepAnalysis] 全量分析完成 — 高价值 %d 篇产出洞察，降级 %d 篇（批次成功 %d/%d）", len(high), len(low), okCount, len(batches))
	return all
}

// deepAnalyzeBatch 单批深度分析。返回 (洞察列表, 是否成功解析)。
func (s *Service) deepAnalyzeBatch(ctx context.Context, arts []model.Article, commentMap map[uint][]model.ArticleComment, stats crawlStats, cfg config.TaggerConfig, tc *TokenCounter) ([]ArticleInsight, bool) {
	var sb strings.Builder
	sb.WriteString("你是资深用户研究专家。请对以下每篇内容（含其评论）做深度需求挖掘。\n\n")
	sb.WriteString("要求：\n")
	sb.WriteString("- 基于内容和评论的真实表述，提炼用户的潜在需求和痛点\n")
	sb.WriteString("- evidence 字段必须引用原文/评论中的原话片段（10-30字），用于验证结论真实性\n")
	sb.WriteString("- 如果评论中有不同于文章的补充信号（需求、抱怨、建议），提取到 commentSignals\n")
	sb.WriteString("- 不要推测没有依据的内容，只从原文和评论中提取\n\n")
	sb.WriteString("以JSON数组返回，每篇格式：\n")
	sb.WriteString(`{"id":序号,"coreNeed":"核心诉求(25-40字)","painPoints":["痛点1","痛点2"],"suggestions":["建议1","建议2"],"sentiment":"positive/neutral/negative","intensity":1-5,"evidence":["原文引用1","原文引用2"],"commentSignals":["评论信号1"]}`)
	sb.WriteString("\n")
	sb.WriteString(jsonArrayContract)
	sb.WriteString("\n")

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
	r := callLLMJSON(ctx, sb.String(), cfg, scaledTokens(2000, 1100, len(arts), 8000), tc, "需求挖掘批次",
		salvageArray, func(s string) error { return json.Unmarshal([]byte(s), &parsed) })
	if !r.OK {
		return s.buildFallbackInsights(arts, stats), false
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
	return insights, true
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

// clusterInsights 语义聚类（步骤③）— 入口，按是否有 embedder 分两路。
// metrics 非 nil 时记录本阶段成功率。
func (s *Service) clusterInsights(ctx context.Context, insights []ArticleInsight, cfg config.TaggerConfig, tc *TokenCounter, metrics *PipelineMetrics) []InsightCluster {
	if len(insights) == 0 {
		return nil
	}

	// 主路：embedding 确定性语义聚类
	if s.embed != nil {
		clusters, stat, ok := s.clusterByEmbedding(ctx, insights, cfg, tc)
		if ok {
			if metrics != nil {
				metrics.Record(stat)
			}
			return clusters
		}
		log.Printf("[DeepAnalysis] embedding 聚类不可用，回退 LLM 聚类")
	}

	// 回退路：强化版 LLM 聚类
	return s.clusterByLLM(ctx, insights, cfg, tc, metrics)
}

// clusterByEmbedding 主路：对洞察文本取向量 + 贪心阈值聚类 + LLM 仅命名。
func (s *Service) clusterByEmbedding(ctx context.Context, insights []ArticleInsight, cfg config.TaggerConfig, tc *TokenCounter) ([]InsightCluster, StageStat, bool) {
	stat := StageStat{Name: "语义聚类(embedding)"}

	// 1) 构造每条洞察的可嵌入文本
	texts := make([]string, len(insights))
	for i, ins := range insights {
		var b strings.Builder
		b.WriteString(ins.CoreNeed)
		if len(ins.PainPoints) > 0 {
			b.WriteString(" ")
			b.WriteString(strings.Join(ins.PainPoints, " "))
		}
		texts[i] = strings.TrimSpace(b.String())
	}

	// 2) 批量编码
	vecs, err := s.embed.Encode(texts)
	if err != nil || len(vecs) != len(insights) {
		log.Printf("[DeepAnalysis] embedding 编码失败: %v (期望%d条)", err, len(insights))
		return nil, stat, false
	}

	// 3) 单遍贪心余弦阈值聚类
	groups := greedyClusterByCosine(vecs, 0.78)
	// 合并过小簇（<2）到最近的大簇，避免碎片
	groups = mergeTinyClusters(groups, vecs, 2)

	stat.Attempted = len(groups)
	stat.Note = fmt.Sprintf("%d条洞察→%d簇", len(insights), len(groups))

	// 4) 每簇并发 LLM 命名 + 一句结论（输入小，几乎不截断）
	clusters := make([]InsightCluster, len(groups))
	var wg sync.WaitGroup
	var mu sync.Mutex
	succeeded := 0
	sem := make(chan struct{}, deepConcurrency)

	for gi, idxs := range groups {
		members := make([]ArticleInsight, 0, len(idxs))
		for _, idx := range idxs {
			members = append(members, insights[idx])
		}
		wg.Add(1)
		sem <- struct{}{}
		go func(slot int, mem []ArticleInsight) {
			defer wg.Done()
			defer func() { <-sem }()
			cl, named := s.nameCluster(ctx, mem, cfg, tc)
			clusters[slot] = cl
			if named {
				mu.Lock()
				succeeded++
				mu.Unlock()
			}
		}(gi, members)
	}
	wg.Wait()

	stat.Succeeded = succeeded
	sort.Slice(clusters, func(i, j int) bool { return len(clusters[i].Insights) > len(clusters[j].Insights) })
	log.Printf("[DeepAnalysis] 语义聚类完成 — %d 簇，命名成功 %d/%d", len(clusters), succeeded, len(groups))
	return clusters, stat, true
}

// greedyClusterByCosine 单遍贪心聚类：每条与已有簇心比较，超过阈值则归入最相近簇，否则新建簇。
// 返回每个簇的成员下标列表。
func greedyClusterByCosine(vecs [][]float32, threshold float64) [][]int {
	var groups [][]int
	var centroids [][]float32

	for i := range vecs {
		best := -1
		bestSim := threshold
		for c := range centroids {
			sim := cosine(vecs[i], centroids[c])
			if sim >= bestSim {
				bestSim = sim
				best = c
			}
		}
		if best < 0 {
			groups = append(groups, []int{i})
			centroids = append(centroids, cloneVec(vecs[i]))
		} else {
			groups[best] = append(groups[best], i)
			// 增量更新簇心（均值）
			n := float64(len(groups[best]))
			for d := range centroids[best] {
				centroids[best][d] = float32((float64(centroids[best][d])*(n-1) + float64(vecs[i][d])) / n)
			}
		}
	}
	return groups
}

// mergeTinyClusters 把成员数 < minSize 的小簇并入与其首元素最相近的较大簇。
func mergeTinyClusters(groups [][]int, vecs [][]float32, minSize int) [][]int {
	if len(groups) <= 1 {
		return groups
	}
	// 计算每簇心
	centroid := func(idxs []int) []float32 {
		if len(idxs) == 0 {
			return nil
		}
		c := make([]float32, len(vecs[idxs[0]]))
		for _, id := range idxs {
			for d := range c {
				c[d] += vecs[id][d]
			}
		}
		for d := range c {
			c[d] /= float32(len(idxs))
		}
		return c
	}

	var big [][]int
	var bigC [][]float32
	var tiny [][]int
	for _, g := range groups {
		if len(g) >= minSize {
			big = append(big, g)
			bigC = append(bigC, centroid(g))
		} else {
			tiny = append(tiny, g)
		}
	}
	if len(big) == 0 {
		return groups // 全是小簇，不动
	}
	for _, t := range tiny {
		c := centroid(t)
		best, bestSim := 0, -2.0
		for bi := range bigC {
			if sim := cosine(c, bigC[bi]); sim > bestSim {
				bestSim = sim
				best = bi
			}
		}
		big[best] = append(big[best], t...)
	}
	return big
}

// nameCluster 对一个语义簇调用 LLM 生成话题名 + 核心结论 + 行动建议。
func (s *Service) nameCluster(ctx context.Context, members []ArticleInsight, cfg config.TaggerConfig, tc *TokenCounter) (InsightCluster, bool) {
	cluster := InsightCluster{Insights: members}

	var sb strings.Builder
	sb.WriteString("你是资深用户研究专家。以下是一组语义相近的用户诉求/痛点洞察，请为这一组提炼：\n")
	sb.WriteString("- topic：以用户诉求命名的话题名（8-15字，体现诉求本质，如「XX功能体验差急需优化」）\n")
	sb.WriteString("- verdict：一句话核心结论（30字内）\n")
	sb.WriteString("- action：建议行动（20字内）\n\n")
	sb.WriteString("仅返回JSON：{\"topic\":\"...\",\"verdict\":\"...\",\"action\":\"...\"}\n")
	sb.WriteString(jsonObjectContract)
	sb.WriteString("洞察列表：\n")

	limit := len(members)
	if limit > 25 {
		limit = 25
	}
	for i := 0; i < limit; i++ {
		ins := members[i]
		line := fmt.Sprintf("%d. [%s] %s", i+1, ins.Sentiment, ins.CoreNeed)
		if len(ins.Evidence) > 0 {
			line += "（佐证：" + ins.Evidence[0] + "）"
		}
		sb.WriteString(line + "\n")
	}

	var parsed struct {
		Topic   string `json:"topic"`
		Verdict string `json:"verdict"`
		Action  string `json:"action"`
	}
	r := callLLMJSON(ctx, sb.String(), cfg, scaledTokens(600, 8, limit, 1200), tc, "簇命名",
		extractJSONObject, func(s string) error { return json.Unmarshal([]byte(s), &parsed) })

	if !r.OK || parsed.Topic == "" {
		// 命名失败：用最高频/首条 coreNeed 兜底，不丢数据
		cluster.Topic = fallbackTopicName(members)
		cluster.CoreVerdict = ""
		return cluster, false
	}
	cluster.Topic = parsed.Topic
	cluster.CoreVerdict = parsed.Verdict
	cluster.ActionItem = parsed.Action
	return cluster, true
}

// fallbackTopicName 簇命名失败时的兜底话题名：取簇内首条洞察的核心诉求（截断）。
func fallbackTopicName(members []ArticleInsight) string {
	if len(members) == 0 {
		return "未命名诉求组"
	}
	name := members[0].CoreNeed
	if name == "" {
		name = members[0].Title
	}
	r := []rune(name)
	if len(r) > 15 {
		name = string(r[:15])
	}
	if name == "" {
		return "未命名诉求组"
	}
	return name
}

// clusterByLLM 回退路：强化版单体 LLM 聚类（token 缩放 + 重试 + 抢救）。
func (s *Service) clusterByLLM(ctx context.Context, insights []ArticleInsight, cfg config.TaggerConfig, tc *TokenCounter, metrics *PipelineMetrics) []InsightCluster {
	stat := StageStat{Name: "语义聚类(LLM)", Attempted: 1}

	var sb strings.Builder
	sb.WriteString("你是资深用户研究专家。以下是用户的诉求/痛点洞察列表，请按「用户核心诉求」将它们聚类为5-8个话题组。\n\n")
	sb.WriteString("话题命名规则：\n")
	sb.WriteString("- 以用户诉求/痛点命名，如「XX功能体验差急需优化」「XX机制不合理引发不满」\n")
	sb.WriteString("- 话题名8-15字，直接体现诉求本质\n")
	sb.WriteString("- 每条洞察只属于一个话题\n\n")
	sb.WriteString("以JSON返回：{\"clusters\":[{\"topic\":\"话题名\",\"ids\":[序号...],\"verdict\":\"一句话核心结论\",\"action\":\"建议行动\"}]}\n")
	sb.WriteString(jsonObjectContract)
	sb.WriteString("洞察列表：\n")

	limit := len(insights)
	if limit > 120 {
		limit = 120
		sb.WriteString(fmt.Sprintf("（仅列出前120条，共%d条）\n", len(insights)))
	}
	for i := 0; i < limit; i++ {
		ins := insights[i]
		sb.WriteString(fmt.Sprintf("%d. [%s/%s] %s\n", i+1, ins.Platform, ins.Sentiment, ins.CoreNeed))
	}

	var parsed struct {
		Clusters []struct {
			Topic   string `json:"topic"`
			IDs     []int  `json:"ids"`
			Verdict string `json:"verdict"`
			Action  string `json:"action"`
		} `json:"clusters"`
	}
	r := callLLMJSON(ctx, sb.String(), cfg, scaledTokens(1500, 45, limit, 4000), tc, "LLM聚类",
		salvageObject, func(s string) error { return json.Unmarshal([]byte(s), &parsed) })
	stat.Retried = r.Retried

	if !r.OK || len(parsed.Clusters) == 0 {
		log.Printf("[DeepAnalysis] LLM 聚类失败，按标签退化聚类")
		stat.Note = "LLM失败，按AI标签退化"
		if metrics != nil {
			metrics.Record(stat)
		}
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
		if len(cluster.Insights) > 0 {
			clusters = append(clusters, cluster)
		}
	}
	if len(clusters) == 0 {
		stat.Note = "解析出0有效簇，按AI标签退化"
		if metrics != nil {
			metrics.Record(stat)
		}
		return s.fallbackCluster(insights)
	}

	stat.Succeeded = 1
	if metrics != nil {
		metrics.Record(stat)
	}
	sort.Slice(clusters, func(i, j int) bool { return len(clusters[i].Insights) > len(clusters[j].Insights) })
	log.Printf("[DeepAnalysis] LLM 聚类完成 — %d 个话题组", len(clusters))
	return clusters
}

// fallbackCluster 终极兜底：按已有 AI 标签/平台退化为多个有意义的话题组，
// 而非堆成一个「未分类」单桶。话术不外显刺眼的失败字样。
func (s *Service) fallbackCluster(insights []ArticleInsight) []InsightCluster {
	byTag := make(map[string][]ArticleInsight)
	for _, ins := range insights {
		key := firstNonEmpty(ins.PainPoints)
		if key == "" {
			key = ins.Platform
		}
		if key == "" {
			key = "综合诉求"
		}
		byTag[key] = append(byTag[key], ins)
	}

	var clusters []InsightCluster
	for topic, members := range byTag {
		clusters = append(clusters, InsightCluster{
			Topic:       topic,
			CoreVerdict: "",
			Insights:    members,
			RiskLevel:   "medium",
		})
	}
	sort.Slice(clusters, func(i, j int) bool { return len(clusters[i].Insights) > len(clusters[j].Insights) })
	// 限制组数，过多则合并尾部
	const maxFallback = 8
	if len(clusters) > maxFallback {
		merged := clusters[maxFallback-1]
		merged.Topic = "其他诉求"
		for _, c := range clusters[maxFallback:] {
			merged.Insights = append(merged.Insights, c.Insights...)
		}
		clusters = append(clusters[:maxFallback-1], merged)
	}
	log.Printf("[DeepAnalysis] 退化聚类 — 按标签生成 %d 组", len(clusters))
	return clusters
}

func firstNonEmpty(ss []string) string {
	for _, s := range ss {
		if strings.TrimSpace(s) != "" {
			return s
		}
	}
	return ""
}

// scoreInsights 漏斗④（打分）：在聚类之前对扁平洞察列表做 LLM 价值打分，写回 DecisionValue。
// 固定批 25 并发，统一批大小消除空响应诱因。打分后由调用方排序+切分高低价值。
func (s *Service) scoreInsights(ctx context.Context, insights []ArticleInsight, cfg config.TaggerConfig, tc *TokenCounter, metrics *PipelineMetrics) {
	if len(insights) == 0 {
		return
	}
	const scoreBatch = 25
	type seg struct{ start, end int }
	var segs []seg
	for start := 0; start < len(insights); start += scoreBatch {
		end := start + scoreBatch
		if end > len(insights) {
			end = len(insights)
		}
		segs = append(segs, seg{start, end})
	}

	var wg sync.WaitGroup
	var mu sync.Mutex
	sem := make(chan struct{}, deepConcurrency)
	succeeded := 0
	for _, sg := range segs {
		wg.Add(1)
		sem <- struct{}{}
		go func(g seg) {
			defer wg.Done()
			defer func() { <-sem }()
			if s.scoreInsightBatch(ctx, insights[g.start:g.end], cfg, tc) {
				mu.Lock()
				succeeded++
				mu.Unlock()
			}
		}(sg)
	}
	wg.Wait()

	if metrics != nil {
		metrics.Record(StageStat{Name: "质量评估打分", Attempted: len(segs), Succeeded: succeeded,
			Note: fmt.Sprintf("%d条洞察", len(insights))})
	}
	log.Printf("[DeepAnalysis] 价值打分完成 — %d 条洞察，批次成功 %d/%d", len(insights), succeeded, len(segs))
}

// finalizeClusters 漏斗⑤后处理（纯计算，不调 LLM）：簇内排序、KeyFindings、风险等级、去空簇。
// 低质量过滤(>=3)已在聚类前完成，这里不再过滤。
func (s *Service) finalizeClusters(clusters []InsightCluster) []InsightCluster {
	for ci := range clusters {
		insights := clusters[ci].Insights
		rankInsightsByValue(insights)

		// KeyFindings：每组取前 5 个 coreNeed
		var findings []string
		for i, ins := range insights {
			if i >= 5 {
				break
			}
			findings = append(findings, ins.CoreNeed)
		}
		clusters[ci].KeyFindings = findings

		// 风险等级
		negCount := 0
		for _, ins := range insights {
			if ins.Sentiment == "negative" || ins.Intensity >= 4 {
				negCount++
			}
		}
		if total := len(insights); total > 0 {
			negRate := float64(negCount) / float64(total)
			switch {
			case negRate > 0.6:
				clusters[ci].RiskLevel = "high"
			case negRate > 0.3:
				clusters[ci].RiskLevel = "medium"
			default:
				clusters[ci].RiskLevel = "low"
			}
		}
	}

	var result []InsightCluster
	for _, cl := range clusters {
		if len(cl.Insights) > 0 {
			result = append(result, cl)
		}
	}
	log.Printf("[DeepAnalysis] 合并后处理完成 — 保留 %d 个话题组", len(result))
	return result
}

// scoreInsightBatch 给一批扁平洞察打分，写回 DecisionValue（簇无关，漏斗④用）。
// 失败时本批默认 3 分，不静默丢弃。
func (s *Service) scoreInsightBatch(ctx context.Context, sub []ArticleInsight, cfg config.TaggerConfig, tc *TokenCounter) bool {
	if len(sub) == 0 {
		return true
	}

	var sb strings.Builder
	sb.WriteString("你是信息质量评审官。以下是从舆情内容中提炼的洞察列表，请对每条评估其决策价值。\n\n")
	sb.WriteString("评分标准：\n")
	sb.WriteString("5分 = 揭示了明确的用户需求/痛点，有行动指导意义，有原文佐证\n")
	sb.WriteString("4分 = 信息有价值，能帮助理解用户态度\n")
	sb.WriteString("3分 = 一般性信息，价值中等\n")
	sb.WriteString("2分 = 过于笼统或缺乏具体指向\n")
	sb.WriteString("1分 = 噪音/重复/无信息量\n\n")
	sb.WriteString("以JSON数组返回：[{\"id\":序号,\"score\":分数}]\n")
	sb.WriteString(jsonArrayContract)
	sb.WriteString("\n")

	for i, ins := range sub {
		evidence := ""
		if len(ins.Evidence) > 0 {
			evidence = " [佐证:" + strings.Join(ins.Evidence, ";") + "]"
		}
		sb.WriteString(fmt.Sprintf("%d. %s%s\n", i+1, ins.CoreNeed, evidence))
	}

	var scores []struct {
		ID    int `json:"id"`
		Score int `json:"score"`
	}
	r := callLLMJSON(ctx, sb.String(), cfg, scaledTokens(800, 25, len(sub), 3000), tc, "质量评估",
		salvageArray, func(s string) error { return json.Unmarshal([]byte(s), &scores) })

	if !r.OK {
		for i := range sub {
			sub[i].DecisionValue = 3
		}
		return false
	}

	scoreMap := make(map[int]int, len(scores))
	for _, sc := range scores {
		scoreMap[sc.ID] = sc.Score
	}
	for i := range sub {
		if score, ok := scoreMap[i+1]; ok && score >= 1 && score <= 5 {
			sub[i].DecisionValue = score
		} else {
			sub[i].DecisionValue = 3
		}
	}
	return true
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

// deepCommentAnalysis 深度模式下按 cluster 精确归类评论，生成 CommentAnalysis。
// 设计：纯程序统计（平台分布/趋势/热评/整体情感兜底）无条件产出，与 LLM 观点提取解耦，
// 保证评论区永不全空。
// deepCommentAnalysis 漏斗⑥评论总览。
// comments：粗过滤后的清洗评论，用于 LLM 话题观点提取（去噪去重，信号干净）。
// rawComments：原始全量评论，用于平台分布/趋势/热评/计数（降级保留真实 volume）。
func (s *Service) deepCommentAnalysis(ctx context.Context, clusters []InsightCluster, comments, rawComments []model.ArticleComment, articles []model.Article, cfg config.TaggerConfig, tc *TokenCounter, metrics *PipelineMetrics) *CommentAnalysis {
	if len(rawComments) == 0 {
		return nil
	}

	articleMap := make(map[uint]model.Article, len(articles))
	for _, a := range articles {
		articleMap[a.ID] = a
	}

	result := &CommentAnalysis{
		PlatformCount: make(map[string]int),
	}

	// 整体统计（原始全量评论，保留真实 volume）
	for _, c := range rawComments {
		if a, ok := articleMap[c.ArticleID]; ok {
			result.PlatformCount[a.Platform]++
		}
	}

	// 按日统计趋势（原始全量评论）
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
	attempted, succeeded := 0, 0

	for _, cl := range clusters {
		clComments := topicComments[cl.Topic]
		if len(clComments) == 0 {
			continue
		}
		attempted++
		wg.Add(1)
		sem <- struct{}{}
		go func(topic string, cs []model.ArticleComment) {
			defer wg.Done()
			defer func() { <-sem }()
			view, ok := s.analyzeClusterComments(ctx, topic, cs, cfg, tc)
			mu.Lock()
			views = append(views, view)
			if ok {
				succeeded++
			}
			mu.Unlock()
		}(cl.Topic, clComments)
	}
	wg.Wait()

	if metrics != nil {
		metrics.Record(StageStat{Name: "评论观点提取", Attempted: attempted, Succeeded: succeeded,
			Note: fmt.Sprintf("%d话题/%d评论", attempted, len(comments))})
	}

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

	// 兜底：若按话题的 LLM 情感全部失败（total==0），用高赞样本独立做一次整体情感，
	// 保证评论情感分布图永不空白。
	if total == 0 {
		sent := s.llmOverallCommentSentiment(ctx, comments, cfg, tc)
		result.OverallSentiment = sent
		total = sent.Positive + sent.Neutral + sent.Negative
		totalPos, totalNeu, totalNeg = sent.Positive, sent.Neutral, sent.Negative
	} else {
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

	// 热评 Top 10（原始全量评论，高赞即热，不受去噪影响）
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

// deepCommentSampleCap 每个话题送入 LLM 的评论采样上限（按点赞优先）
const deepCommentSampleCap = 80

// analyzeClusterComments 对单个 cluster 的评论做 LLM 分析。返回 (视图, 是否成功解析)。
func (s *Service) analyzeClusterComments(ctx context.Context, topic string, comments []model.ArticleComment, cfg config.TaggerConfig, tc *TokenCounter) (TopicCommentView, bool) {
	view := TopicCommentView{
		Topic:        topic,
		CommentCount: len(comments),
	}

	// 按点赞排序，取 Top N 做 LLM 分析（提高覆盖，token 随样本量缩放）
	sorted := make([]model.ArticleComment, len(comments))
	copy(sorted, comments)
	sort.Slice(sorted, func(i, j int) bool { return sorted[i].LikeCount > sorted[j].LikeCount })
	sample := sorted
	if len(sample) > deepCommentSampleCap {
		sample = sample[:deepCommentSampleCap]
	}

	var sb strings.Builder
	sb.WriteString(fmt.Sprintf("你是资深用户研究专家。以下是「%s」话题下的全部%d条评论（按点赞排序，展示前%d条）。\n\n", topic, len(comments), len(sample)))
	sb.WriteString("请分析：\n")
	sb.WriteString("1. 统计样本中 正面/中性/负面 各多少条（整数）\n")
	sb.WriteString("2. 提炼5-8个代表性观点（每条20-30字，具体有信息量，说明用户关注什么、持什么立场）\n")
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
	r := callLLMJSON(ctx, sb.String(), cfg, scaledTokens(800, 25, len(sample), 5000), tc, "评论观点("+topic+")",
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
	return view, true
}

// llmOverallCommentSentiment 兜底：当按话题情感全失败时，用高赞样本独立做一次整体情感分析。
func (s *Service) llmOverallCommentSentiment(ctx context.Context, comments []model.ArticleComment, cfg config.TaggerConfig, tc *TokenCounter) CommentSentiment {
	var sent CommentSentiment
	if len(comments) == 0 {
		return sent
	}
	sorted := make([]model.ArticleComment, len(comments))
	copy(sorted, comments)
	sort.Slice(sorted, func(i, j int) bool { return sorted[i].LikeCount > sorted[j].LikeCount })
	sample := sorted
	if len(sample) > 60 {
		sample = sample[:60]
	}

	var sb strings.Builder
	sb.WriteString("请统计以下评论样本中 正面/中性/负面 各多少条（整数）。\n")
	sb.WriteString("以JSON返回：{\"pos\":正面条数,\"neu\":中性条数,\"neg\":负面条数}\n")
	sb.WriteString(jsonObjectContract)
	sb.WriteString("\n")
	for i, c := range sample {
		content := c.Content
		if len([]rune(content)) > 80 {
			content = string([]rune(content)[:80]) + "..."
		}
		sb.WriteString(fmt.Sprintf("%d. %s\n", i+1, content))
	}

	var parsed struct {
		Pos float64 `json:"pos"`
		Neu float64 `json:"neu"`
		Neg float64 `json:"neg"`
	}
	r := callLLMJSON(ctx, sb.String(), cfg, scaledTokens(500, 5, len(sample), 1000), tc, "整体评论情感兜底",
		extractJSONObject, func(s string) error { return json.Unmarshal([]byte(s), &parsed) })
	if !r.OK {
		return sent
	}

	sampleTotal := parsed.Pos + parsed.Neu + parsed.Neg
	if sampleTotal <= 0 {
		return sent
	}
	// 按样本比例放大到全量评论数
	scale := float64(len(comments)) / sampleTotal
	sent.Positive = int(parsed.Pos*scale + 0.5)
	sent.Neutral = int(parsed.Neu*scale + 0.5)
	sent.Negative = len(comments) - sent.Positive - sent.Neutral
	if sent.Negative < 0 {
		sent.Negative = 0
	}
	sent.PosRate = parsed.Pos / sampleTotal * 100
	sent.NeuRate = parsed.Neu / sampleTotal * 100
	sent.NegRate = parsed.Neg / sampleTotal * 100
	return sent
}
