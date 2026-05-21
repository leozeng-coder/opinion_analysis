package alertengine

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"strings"
	"sync"
	"time"

	"opinion-analysis/src/model"
	"opinion-analysis/src/repository"
	"opinion-analysis/src/service/tagger"
)

type AIService interface {
	ChatCompletion(ctx context.Context, history []tagger.ChatMessage, pageHint string, retrievalContext string) (string, error)
}

// Engine 告警评估与通知引擎。
type Engine struct {
	store *repository.Store
	ai    AIService
	mu    sync.Mutex
}

func New(store *repository.Store, ai AIService) *Engine {
	return &Engine{store: store, ai: ai}
}

// EvaluateResult 单次评估汇总。
type EvaluateResult struct {
	Evaluated int          `json:"evaluated"`
	Triggered int          `json:"triggered"`
	Skipped   int          `json:"skipped"`
	Errors    []string     `json:"errors,omitempty"`
	Source    string       `json:"source"`
	Details   []RuleResult `json:"details,omitempty"`
}

// RuleResult 单条规则评估结果。
type RuleResult struct {
	RuleID      uint   `json:"ruleId"`
	RuleName    string `json:"ruleName"`
	Triggered   bool   `json:"triggered"`
	SkipReason  string `json:"skipReason,omitempty"`
	MatchCount  int64  `json:"matchCount,omitempty"`
	Threshold   int    `json:"threshold,omitempty"`
	WindowStart string `json:"windowStart,omitempty"`
}

func (e *Engine) OnCrawlEnabled() bool {
	return e.store.System.GetAlertConfig().OnCrawl
}

// EvaluateAll 评估全部启用中的规则。
func (e *Engine) EvaluateAll(ctx context.Context, source string) (EvaluateResult, error) {
	e.mu.Lock()
	defer e.mu.Unlock()

	result := EvaluateResult{Source: source}
	rules, err := e.store.Alert.ListActiveRules()
	if err != nil {
		return result, err
	}
	result.Evaluated = len(rules)
	log.Printf("[alert] evaluating %d active rules (source=%s)", len(rules), source)

	smtpCfg, _ := e.store.System.GetSmtpConfig()

	for i := range rules {
		select {
		case <-ctx.Done():
			return result, ctx.Err()
		default:
		}
		rule := &rules[i]
		rr, evalErr := e.evaluateRule(ctx, rule, smtpCfg)
		result.Details = append(result.Details, rr)
		if evalErr != nil {
			log.Printf("[alert] rule=%d error: %v", rule.ID, evalErr)
			result.Errors = append(result.Errors, fmt.Sprintf("%s: %v", rule.Name, evalErr))
			continue
		}
		if rr.Triggered {
			result.Triggered++
		} else if rr.SkipReason != "" {
			result.Skipped++
			log.Printf("[alert] rule=%d %s skipped: %s (match=%d threshold=%d)",
				rule.ID, rule.Name, rr.SkipReason, rr.MatchCount, rr.Threshold)
		}
	}
	log.Printf("[alert] done source=%s evaluated=%d triggered=%d skipped=%d",
		source, result.Evaluated, result.Triggered, result.Skipped)
	return result, nil
}

func (e *Engine) evaluateRule(
	ctx context.Context,
	rule *model.AlertRule,
	smtpCfg repository.SmtpConfigData,
) (RuleResult, error) {
	rr := RuleResult{
		RuleID: rule.ID, RuleName: rule.Name, Threshold: rule.Threshold,
	}
	now := time.Now()
	cooldown := time.Duration(ruleIntervalMinutes(rule)) * time.Minute

	if rule.LastTriggeredAt != nil && now.Sub(*rule.LastTriggeredAt) < cooldown {
		rr.SkipReason = fmt.Sprintf("冷却中（距上次触发不足 %d 分钟）", ruleIntervalMinutes(rule))
		return rr, nil
	}

	windowStart := countWindowStart(now, rule)
	rr.WindowStart = windowStart.Format("2006-01-02 15:04")

	keywords := parseRuleKeywords(rule.Keywords)
	sentiment := strings.TrimSpace(rule.Sentiment)

	count, err := e.store.Article.CountForAlertRule(keywords, sentiment, windowStart)
	if err != nil {
		return rr, err
	}
	rr.MatchCount = count

	if int(count) < rule.Threshold {
		rr.SkipReason = fmt.Sprintf("未达阈值（%d/%d 条）", count, rule.Threshold)
		return rr, nil
	}

	dedupKey := fmt.Sprintf("rule:%d:day:%s:cnt:%d", rule.ID, now.Format("20060102"), count)
	exists, err := e.store.Alert.ExistsByDedupKey(dedupKey)
	if err != nil {
		return rr, err
	}
	if exists {
		rr.SkipReason = fmt.Sprintf("今日已告警过相同匹配数（%d 条，去重）", count)
		return rr, nil
	}

	samples, _ := e.store.Article.ListSampleForAlertRule(keywords, sentiment, windowStart, 5)
	title, content := e.buildAlertContent(ctx, rule, count, windowStart, keywords, sentiment, samples)

	record := &model.AlertRecord{
		RuleID: rule.ID, Title: title, Content: content,
		Status: "pending", DedupKey: dedupKey,
	}
	if err := e.store.Alert.CreateRecord(record); err != nil {
		return rr, err
	}
	_ = e.store.Alert.UpdateLastTriggered(rule.ID, now)

	if notifyErr := e.notify(ctx, rule, record, smtpCfg); notifyErr != nil {
		rr.Triggered = true
		return rr, fmt.Errorf("record created, notify failed: %w", notifyErr)
	}
	rr.Triggered = true
	return rr, nil
}

// countWindowStart 统计窗口：默认今日 0 点至今（按 published_at）；若今日已触发过则从上次触发时间起算。
func countWindowStart(now time.Time, rule *model.AlertRule) time.Time {
	loc := now.Location()
	todayStart := time.Date(now.Year(), now.Month(), now.Day(), 0, 0, 0, 0, loc)
	if rule.LastTriggeredAt != nil && rule.LastTriggeredAt.After(todayStart) {
		return *rule.LastTriggeredAt
	}
	return todayStart
}

func ruleIntervalMinutes(rule *model.AlertRule) int {
	if rule.Interval <= 0 {
		return 60
	}
	return rule.Interval
}

func parseRuleKeywords(raw string) []string {
	raw = strings.TrimSpace(raw)
	if raw == "" || raw == "[]" {
		return nil
	}
	var arr []string
	if json.Unmarshal([]byte(raw), &arr) == nil {
		out := make([]string, 0, len(arr))
		for _, k := range arr {
			if k = strings.TrimSpace(k); k != "" {
				out = append(out, k)
			}
		}
		return out
	}
	parts := strings.Split(raw, ",")
	out := make([]string, 0, len(parts))
	for _, p := range parts {
		if p = strings.TrimSpace(p); p != "" {
			out = append(out, p)
		}
	}
	return out
}

func (e *Engine) buildAlertContent(
	ctx context.Context,
	rule *model.AlertRule,
	count int64,
	windowStart time.Time,
	keywords []string,
	sentiment string,
	samples []model.Article,
) (title, content string) {
	kwLabel := "全部"
	if len(keywords) > 0 {
		kwLabel = strings.Join(keywords, "、")
	}
	sentLabel := sentiment
	if sentLabel == "" {
		sentLabel = "全部"
	}
	title = fmt.Sprintf("[%s] 舆情预警：%d 条匹配", rule.Name, count)

	var b strings.Builder
	fmt.Fprintf(&b, "规则：%s\n", rule.Name)
	fmt.Fprintf(&b, "时间窗口：%s ~ 现在\n", windowStart.Format("2006-01-02 15:04"))
	fmt.Fprintf(&b, "关键词：%s\n", kwLabel)
	fmt.Fprintf(&b, "情感：%s\n", sentLabel)
	fmt.Fprintf(&b, "匹配条数：%d（阈值 %d）\n\n", count, rule.Threshold)

	// AI 分析
	if e.ai != nil && len(samples) > 0 {
		aiAnalysis := e.generateAIAnalysis(ctx, rule, samples, kwLabel, sentLabel)
		if aiAnalysis != "" {
			fmt.Fprintf(&b, "【AI 分析】\n%s\n\n", aiAnalysis)
		}
	}

	fmt.Fprintf(&b, "【匹配文章】\n")
	for i, a := range samples {
		fmt.Fprintf(&b, "%d. %s\n", i+1, a.Title)
	}
	return title, b.String()
}

func (e *Engine) generateAIAnalysis(ctx context.Context, rule *model.AlertRule, samples []model.Article, kwLabel, sentLabel string) string {
	// 构建文章摘要
	var articleSummary strings.Builder
	for i, a := range samples {
		fmt.Fprintf(&articleSummary, "%d. 标题：%s\n", i+1, a.Title)
		// 使用 Content 的前 200 个字符作为摘要
		if a.Content != "" {
			content := a.Content
			runes := []rune(content)
			if len(runes) > 200 {
				content = string(runes[:200]) + "..."
			}
			fmt.Fprintf(&articleSummary, "   内容摘要：%s\n", content)
		}
		if a.Sentiment != "" {
			fmt.Fprintf(&articleSummary, "   情感：%s\n", a.Sentiment)
		}
	}

	prompt := fmt.Sprintf(`请分析以下舆情预警信息，提供简洁的分析建议（200字以内）：

规则：%s
关键词：%s
情感：%s
匹配文章数：%d

匹配的文章：
%s

请从以下角度简要分析：
1. 舆情趋势和主要关注点
2. 潜在风险或机会
3. 建议采取的行动`, rule.Name, kwLabel, sentLabel, len(samples), articleSummary.String())

	history := []tagger.ChatMessage{
		{Role: "user", Content: prompt},
	}

	// 设置超时
	aiCtx, cancel := context.WithTimeout(ctx, 15*time.Second)
	defer cancel()

	reply, err := e.ai.ChatCompletion(aiCtx, history, "", "")
	if err != nil {
		log.Printf("[alert] AI analysis failed: %v", err)
		return ""
	}
	return reply
}
