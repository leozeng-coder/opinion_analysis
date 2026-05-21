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
)

// Engine 告警评估与通知引擎。
type Engine struct {
	store *repository.Store
	mu    sync.Mutex
}

func New(store *repository.Store) *Engine {
	return &Engine{store: store}
}

// EvaluateResult 单次评估汇总。
type EvaluateResult struct {
	Evaluated int      `json:"evaluated"`
	Triggered int      `json:"triggered"`
	Skipped   int      `json:"skipped"`
	Errors    []string `json:"errors,omitempty"`
	Source    string   `json:"source"`
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

	smtpCfg, _ := e.store.System.GetSmtpConfig()

	for i := range rules {
		select {
		case <-ctx.Done():
			return result, ctx.Err()
		default:
		}
		rule := &rules[i]
		triggered, skipReason, evalErr := e.evaluateRule(ctx, rule, smtpCfg)
		if evalErr != nil {
			log.Printf("[alert] rule=%d error: %v", rule.ID, evalErr)
			result.Errors = append(result.Errors, evalErr.Error())
			continue
		}
		if triggered {
			result.Triggered++
		} else if skipReason != "" {
			result.Skipped++
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
) (triggered bool, skipReason string, err error) {
	now := time.Now()
	interval := time.Duration(ruleIntervalMinutes(rule)) * time.Minute
	windowStart := now.Add(-interval)

	if rule.LastTriggeredAt != nil && now.Sub(*rule.LastTriggeredAt) < interval {
		return false, "cooldown", nil
	}

	keywords := parseRuleKeywords(rule.Keywords)
	sentiment := strings.TrimSpace(rule.Sentiment)

	count, err := e.store.Article.CountForAlertRule(keywords, sentiment, windowStart)
	if err != nil {
		return false, "", err
	}
	if int(count) < rule.Threshold {
		return false, "below threshold", nil
	}

	dedupKey := fmt.Sprintf("rule:%d:win:%s:cnt:%d", rule.ID, windowStart.Truncate(time.Minute).Format("200601021504"), count)
	exists, err := e.store.Alert.ExistsByDedupKey(dedupKey)
	if err != nil {
		return false, "", err
	}
	if exists {
		return false, "dedup", nil
	}

	samples, _ := e.store.Article.ListSampleForAlertRule(keywords, sentiment, windowStart, 5)
	title, content := buildAlertContent(rule, count, windowStart, keywords, sentiment, samples)

	record := &model.AlertRecord{
		RuleID: rule.ID, Title: title, Content: content,
		Status: "pending", DedupKey: dedupKey,
	}
	if err := e.store.Alert.CreateRecord(record); err != nil {
		return false, "", err
	}
	_ = e.store.Alert.UpdateLastTriggered(rule.ID, now)

	if notifyErr := e.notify(ctx, rule, record, smtpCfg); notifyErr != nil {
		return true, "", fmt.Errorf("record created, notify failed: %w", notifyErr)
	}
	return true, "", nil
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

func buildAlertContent(
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
	for i, a := range samples {
		fmt.Fprintf(&b, "%d. %s\n", i+1, a.Title)
	}
	return title, b.String()
}
