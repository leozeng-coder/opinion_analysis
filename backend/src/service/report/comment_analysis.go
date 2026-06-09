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

type CommentSentiment struct {
	Positive int     `json:"positive"`
	Neutral  int     `json:"neutral"`
	Negative int     `json:"negative"`
	PosRate  float64 `json:"posRate"`
	NeuRate  float64 `json:"neuRate"`
	NegRate  float64 `json:"negRate"`
}

type TopicCommentView struct {
	Topic        string           `json:"topic"`
	CommentCount int              `json:"commentCount"`
	Sentiment    CommentSentiment `json:"sentiment"`
	KeyOpinions  []string         `json:"keyOpinions"`
}

type HotComment struct {
	Content   string `json:"content"`
	Nickname  string `json:"nickname"`
	Platform  string `json:"platform"`
	LikeCount int    `json:"likeCount"`
	Sentiment string `json:"sentiment"`
}

type commentTrendPoint struct {
	Date     string `json:"date"`
	Total    int    `json:"total"`
	Positive int    `json:"positive"`
	Negative int    `json:"negative"`
	Neutral  int    `json:"neutral"`
}

type CommentAnalysis struct {
	OverallSentiment   CommentSentiment   `json:"overallSentiment"`
	TopicComments      []TopicCommentView `json:"topicComments"`
	HotComments        []HotComment       `json:"hotComments"`
	DailyTrend         []commentTrendPoint `json:"dailyTrend"`
	PlatformCount      map[string]int     `json:"platformCount"`
}

func (s *Service) analyzeComments(ctx context.Context, comments []model.ArticleComment, articles []model.Article, topGroups []groupStats, cfg config.TaggerConfig) *CommentAnalysis {
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

	// 按平台统计
	for _, c := range comments {
		if a, ok := articleMap[c.ArticleID]; ok {
			result.PlatformCount[a.Platform]++
		}
	}

	// 按日统计趋势
	dailyMap := make(map[string]*commentTrendPoint)
	for _, c := range comments {
		dateKey := c.PublishedAt.Format("01-02")
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

	// 取高赞评论（Top 20 for LLM analysis）
	sorted := make([]model.ArticleComment, len(comments))
	copy(sorted, comments)
	sort.Slice(sorted, func(i, j int) bool { return sorted[i].LikeCount > sorted[j].LikeCount })
	topN := sorted
	if len(topN) > 20 {
		topN = topN[:20]
	}

	// 抽样评论用于 LLM 情感分析（高赞优先 + 随机补充，最多50条）
	sampleForSent := topN
	if len(comments) > 20 {
		step := len(comments) / 30
		if step < 1 {
			step = 1
		}
		for i := 0; i < len(comments) && len(sampleForSent) < 50; i += step {
			found := false
			for _, s := range sampleForSent {
				if s.ID == comments[i].ID {
					found = true
					break
				}
			}
			if !found {
				sampleForSent = append(sampleForSent, comments[i])
			}
		}
	}

	// 按话题分组评论
	topicArticles := make(map[string][]uint)
	for _, g := range topGroups {
		for _, a := range articles {
			if a.AITags != nil && *a.AITags != "" {
				var tags []string
				if json.Unmarshal([]byte(*a.AITags), &tags) == nil {
					for _, tag := range tags {
						if tag == g.Topic {
							topicArticles[g.Topic] = append(topicArticles[g.Topic], a.ID)
							break
						}
					}
				}
			}
		}
	}

	topicComments := make(map[string][]model.ArticleComment)
	for _, c := range comments {
		for topic, aids := range topicArticles {
			for _, aid := range aids {
				if c.ArticleID == aid {
					topicComments[topic] = append(topicComments[topic], c)
					break
				}
			}
		}
	}

	// 并发 LLM 分析
	var wg sync.WaitGroup
	var mu sync.Mutex

	// Task 1: 整体情感分析 + 热评标注
	wg.Add(1)
	go func() {
		defer wg.Done()
		sent, hot := s.llmCommentSentiment(ctx, sampleForSent, topN, articleMap, cfg)
		mu.Lock()
		result.OverallSentiment = sent
		result.HotComments = hot
		mu.Unlock()
	}()

	// Task 2: 话题评论观点提取
	wg.Add(1)
	go func() {
		defer wg.Done()
		views := s.llmTopicCommentOpinions(ctx, topicComments, articleMap, cfg)
		mu.Lock()
		result.TopicComments = views
		mu.Unlock()
	}()

	wg.Wait()

	// 用 LLM 情感结果回填趋势数据
	s.backfillCommentTrend(result, comments, sampleForSent, articleMap)

	return result
}

func (s *Service) llmCommentSentiment(ctx context.Context, sample []model.ArticleComment, topN []model.ArticleComment, articleMap map[uint]model.Article, cfg config.TaggerConfig) (CommentSentiment, []HotComment) {
	var sent CommentSentiment
	var hot []HotComment

	if len(sample) == 0 {
		return sent, hot
	}

	var sb strings.Builder
	sb.WriteString("请对以下评论逐条标注情感倾向（positive/neutral/negative），以JSON数组格式返回，每项格式为{\"id\":序号,\"s\":\"positive\"}。只返回JSON，不要其他文字。\n\n")
	for i, c := range sample {
		content := c.Content
		if len([]rune(content)) > 80 {
			content = string([]rune(content)[:80]) + "..."
		}
		sb.WriteString(fmt.Sprintf("%d. %s\n", i+1, content))
	}

	resp, err := callLLM(ctx, sb.String(), cfg, 800)
	if err != nil {
		log.Printf("[CommentAnalysis] sentiment LLM failed: %v", err)
		return sent, hot
	}

	type sentItem struct {
		ID int    `json:"id"`
		S  string `json:"s"`
	}
	var items []sentItem
	resp = strings.TrimSpace(resp)
	if idx := strings.Index(resp, "["); idx >= 0 {
		resp = resp[idx:]
	}
	if idx := strings.LastIndex(resp, "]"); idx >= 0 {
		resp = resp[:idx+1]
	}
	if err := json.Unmarshal([]byte(resp), &items); err != nil {
		log.Printf("[CommentAnalysis] parse sentiment JSON failed: %v", err)
		return sent, hot
	}

	sentMap := make(map[int]string, len(items))
	for _, item := range items {
		sentMap[item.ID] = item.S
		switch item.S {
		case "positive":
			sent.Positive++
		case "negative":
			sent.Negative++
		default:
			sent.Neutral++
		}
	}

	total := sent.Positive + sent.Neutral + sent.Negative
	if total > 0 {
		sent.PosRate = float64(sent.Positive) / float64(total) * 100
		sent.NeuRate = float64(sent.Neutral) / float64(total) * 100
		sent.NegRate = float64(sent.Negative) / float64(total) * 100
	}

	// 构建热评列表
	for i, c := range topN {
		if i >= 10 {
			break
		}
		platform := ""
		if a, ok := articleMap[c.ArticleID]; ok {
			platform = a.Platform
		}
		sentiment := "neutral"
		for si, sc := range sample {
			if sc.ID == c.ID {
				if s, ok := sentMap[si+1]; ok {
					sentiment = s
				}
				break
			}
		}
		content := c.Content
		if len([]rune(content)) > 150 {
			content = string([]rune(content)[:150]) + "..."
		}
		hot = append(hot, HotComment{
			Content:   content,
			Nickname:  c.Nickname,
			Platform:  platform,
			LikeCount: c.LikeCount,
			Sentiment: sentiment,
		})
	}

	return sent, hot
}

func (s *Service) llmTopicCommentOpinions(ctx context.Context, topicComments map[string][]model.ArticleComment, articleMap map[uint]model.Article, cfg config.TaggerConfig) []TopicCommentView {
	var views []TopicCommentView

	for topic, comments := range topicComments {
		view := TopicCommentView{
			Topic:        topic,
			CommentCount: len(comments),
		}

		// 取代表性评论（高赞优先，最多12条）
		sorted := make([]model.ArticleComment, len(comments))
		copy(sorted, comments)
		sort.Slice(sorted, func(i, j int) bool { return sorted[i].LikeCount > sorted[j].LikeCount })
		sample := sorted
		if len(sample) > 18 {
			sample = sample[:18]
		}

		if len(sample) == 0 {
			views = append(views, view)
			continue
		}

		var sb strings.Builder
		sb.WriteString(fmt.Sprintf("你是资深舆情分析师。以下是「%s」话题下的用户评论样本。请：\n", topic))
		sb.WriteString("1. 判断整体情感倾向分布（positive/neutral/negative各占比）\n")
		sb.WriteString("2. 提炼5-8个代表性观点，每个观点要具体、有信息量（20-30字，说明谁持什么立场、关注什么问题）\n")
		sb.WriteString("3. 观点要覆盖不同立场，让决策者能快速了解用户真实想法\n")
		sb.WriteString("以JSON返回：{\"pos\":正面数,\"neu\":中性数,\"neg\":负面数,\"opinions\":[\"观点1\",\"观点2\",...]}\n\n")
		for i, c := range sample {
			content := c.Content
			if len([]rune(content)) > 80 {
				content = string([]rune(content)[:80]) + "..."
			}
			sb.WriteString(fmt.Sprintf("%d. [赞%d] %s\n", i+1, c.LikeCount, content))
		}

		resp, err := callLLM(ctx, sb.String(), cfg, 500)
		if err != nil {
			log.Printf("[CommentAnalysis] topic opinions LLM failed for %s: %v", topic, err)
			views = append(views, view)
			continue
		}

		var parsed struct {
			Pos      int      `json:"pos"`
			Neu      int      `json:"neu"`
			Neg      int      `json:"neg"`
			Opinions []string `json:"opinions"`
		}
		resp = strings.TrimSpace(resp)
		if idx := strings.Index(resp, "{"); idx >= 0 {
			resp = resp[idx:]
		}
		if idx := strings.LastIndex(resp, "}"); idx >= 0 {
			resp = resp[:idx+1]
		}
		if err := json.Unmarshal([]byte(resp), &parsed); err != nil {
			log.Printf("[CommentAnalysis] parse topic JSON failed for %s: %v", topic, err)
			views = append(views, view)
			continue
		}

		total := parsed.Pos + parsed.Neu + parsed.Neg
		view.Sentiment = CommentSentiment{
			Positive: parsed.Pos,
			Neutral:  parsed.Neu,
			Negative: parsed.Neg,
		}
		if total > 0 {
			view.Sentiment.PosRate = float64(parsed.Pos) / float64(total) * 100
			view.Sentiment.NeuRate = float64(parsed.Neu) / float64(total) * 100
			view.Sentiment.NegRate = float64(parsed.Neg) / float64(total) * 100
		}
		view.KeyOpinions = parsed.Opinions
		views = append(views, view)
	}

	return views
}

func (s *Service) backfillCommentTrend(result *CommentAnalysis, comments []model.ArticleComment, sample []model.ArticleComment, articleMap map[uint]model.Article) {
	// 基于整体情感比例估算每日趋势的情感分布
	total := result.OverallSentiment.Positive + result.OverallSentiment.Neutral + result.OverallSentiment.Negative
	if total == 0 {
		return
	}
	posR := float64(result.OverallSentiment.Positive) / float64(total)
	negR := float64(result.OverallSentiment.Negative) / float64(total)

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
