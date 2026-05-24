// Package sentiment 提供基于 LLM 的情感分析服务。
// 支持从 system_settings 表动态加载配置（与 tagger 共用 LLM 配置）。
package sentiment

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"math"
	"net/http"
	"os"
	"strconv"
	"strings"
	"sync"
	"time"
	"unicode/utf8"

	"gorm.io/gorm"
	"opinion-analysis/src/model"
)

// Analyzer 情感分析器
type Analyzer struct {
	db     *gorm.DB
	client *http.Client
	mu     sync.RWMutex
	cfg    Config
}

// Config 情感分析配置（复用 tagger 的 LLM 配置）
type Config struct {
	Enabled    bool
	LLMApiKey  string
	LLMBaseURL string
	LLMModel   string
}

// New 创建情感分析器
func New(db *gorm.DB) *Analyzer {
	a := &Analyzer{
		db:     db,
		client: &http.Client{Timeout: 30 * time.Second},
	}
	a.loadConfig()
	return a
}

// loadConfig 从 system_settings 表加载配置
func (a *Analyzer) loadConfig() {
	cfg := Config{
		Enabled:    true,
		LLMBaseURL: "https://api.deepseek.com",
		LLMModel:   "deepseek-chat",
	}

	// 从数据库读取 tagger.* 配置（与打标服务共用）
	var rows []model.SystemSetting
	if err := a.db.Where("`key` LIKE ?", "tagger.%").Find(&rows).Error; err == nil {
		for _, r := range rows {
			switch r.Key {
			case "tagger.enabled":
				cfg.Enabled = parseBool(r.Value)
			case "tagger.llm_api_key":
				cfg.LLMApiKey = strings.TrimSpace(r.Value)
			case "tagger.llm_base_url":
				if v := strings.TrimSpace(r.Value); v != "" {
					cfg.LLMBaseURL = v
				}
			case "tagger.llm_model":
				if v := strings.TrimSpace(r.Value); v != "" {
					cfg.LLMModel = v
				}
			}
		}
	}

	// 环境变量优先级最高
	if key := strings.TrimSpace(os.Getenv("DEEPSEEK_API_KEY")); key != "" {
		cfg.LLMApiKey = key
	}

	a.mu.Lock()
	a.cfg = cfg
	a.mu.Unlock()
}

// ReloadConfig 重新加载配置（供外部调用，如配置更新后）
func (a *Analyzer) ReloadConfig() {
	a.loadConfig()
}

// snapshot 返回当前配置的副本
func (a *Analyzer) snapshot() Config {
	a.mu.RLock()
	defer a.mu.RUnlock()
	return a.cfg
}

// Result 情感分析结果
type Result struct {
	Sentiment string  // positive/negative/neutral
	Score     float64 // 0.000-1.000，保留3位小数
}

// Analyze 分析文本情感
// 返回情感类别和分数（0.000-1.000）
func (a *Analyzer) Analyze(ctx context.Context, text string) (Result, error) {
	cfg := a.snapshot()

	// 检查配置
	if !cfg.Enabled {
		return Result{Sentiment: "neutral", Score: 0.500}, nil
	}
	if cfg.LLMApiKey == "" {
		return Result{Sentiment: "neutral", Score: 0.500}, nil
	}

	// 文本预处理：截断过长内容
	text = strings.TrimSpace(text)
	if text == "" {
		return Result{Sentiment: "neutral", Score: 0.500}, nil
	}
	if utf8.RuneCountInString(text) > 500 {
		text = string([]rune(text)[:500])
	}

	// 调用 LLM 进行情感分析
	sentiment, score, err := a.callLLM(ctx, text, cfg)
	if err != nil {
		log.Printf("[sentiment] LLM call failed: %v, fallback to neutral", err)
		return Result{Sentiment: "neutral", Score: 0.500}, nil
	}

	// 确保分数精确到3位小数
	score = roundTo3Decimals(score)

	return Result{
		Sentiment: sentiment,
		Score:     score,
	}, nil
}

// callLLM 调用 LLM API 进行情感分析
func (a *Analyzer) callLLM(ctx context.Context, text string, cfg Config) (string, float64, error) {
	systemPrompt := `你是一个专业的中文情感分析助手。请分析给定文本的情感倾向，并返回 JSON 格式结果。

情感分类规则：
- positive（积极）：表达正面、乐观、赞扬、喜悦等情绪
- negative（消极）：表达负面、悲观、批评、愤怒、失望等情绪
- neutral（中性）：客观陈述、无明显情感倾向

分数规则（0.000-1.000）：
- positive: 0.600-1.000（越积极分数越高）
- neutral: 0.400-0.600（中性偏向）
- negative: 0.000-0.400（越消极分数越低）

输出格式：{"sentiment": "positive|negative|neutral", "score": 0.750}
只输出 JSON，不要其他文字。`

	userPrompt := fmt.Sprintf("请分析以下文本的情感：\n\n%s", text)

	reqBody := map[string]any{
		"model": cfg.LLMModel,
		"messages": []map[string]string{
			{"role": "system", "content": systemPrompt},
			{"role": "user", "content": userPrompt},
		},
		"temperature":     0.3,
		"max_tokens":      200,
		"response_format": map[string]string{"type": "json_object"},
	}

	payload, _ := json.Marshal(reqBody)
	url := strings.TrimRight(cfg.LLMBaseURL, "/") + "/chat/completions"

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, url, bytes.NewReader(payload))
	if err != nil {
		return "", 0, err
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+cfg.LLMApiKey)

	resp, err := a.client.Do(req)
	if err != nil {
		return "", 0, err
	}
	defer resp.Body.Close()

	respBody, _ := io.ReadAll(resp.Body)
	if resp.StatusCode/100 != 2 {
		return "", 0, fmt.Errorf("LLM API error: status=%d body=%s", resp.StatusCode, truncate(string(respBody), 200))
	}

	// 解析 API 响应
	var apiResp struct {
		Choices []struct {
			Message struct {
				Content string `json:"content"`
			} `json:"message"`
		} `json:"choices"`
	}
	if err := json.Unmarshal(respBody, &apiResp); err != nil {
		return "", 0, fmt.Errorf("decode API response: %w", err)
	}
	if len(apiResp.Choices) == 0 {
		return "", 0, fmt.Errorf("empty choices in API response")
	}

	// 解析情感分析结果
	content := strings.TrimSpace(apiResp.Choices[0].Message.Content)
	var result struct {
		Sentiment string  `json:"sentiment"`
		Score     float64 `json:"score"`
	}
	if err := json.Unmarshal([]byte(content), &result); err != nil {
		return "", 0, fmt.Errorf("parse sentiment result: %w (content: %s)", err, truncate(content, 100))
	}

	// 验证和规范化结果
	sentiment := normalizeSentiment(result.Sentiment)
	score := normalizeScore(result.Score, sentiment)

	return sentiment, score, nil
}

// normalizeSentiment 规范化情感类别
func normalizeSentiment(s string) string {
	s = strings.ToLower(strings.TrimSpace(s))
	switch s {
	case "positive", "pos", "积极":
		return "positive"
	case "negative", "neg", "消极":
		return "negative"
	case "neutral", "中性":
		return "neutral"
	default:
		return "neutral"
	}
}

// normalizeScore 规范化分数到合理范围
func normalizeScore(score float64, sentiment string) float64 {
	// 确保分数在 0-1 范围内
	if score < 0 {
		score = 0
	}
	if score > 1 {
		score = 1
	}

	// 根据情感类别调整分数范围
	switch sentiment {
	case "positive":
		// positive 应该在 0.600-1.000
		if score < 0.600 {
			score = 0.600 + (score * 0.4) // 映射到 0.600-1.000
		}
	case "negative":
		// negative 应该在 0.000-0.400
		if score > 0.400 {
			score = score * 0.4 // 映射到 0.000-0.400
		}
	case "neutral":
		// neutral 应该在 0.400-0.600
		if score < 0.400 {
			score = 0.400
		}
		if score > 0.600 {
			score = 0.600
		}
	}

	return roundTo3Decimals(score)
}

// roundTo3Decimals 四舍五入到3位小数
func roundTo3Decimals(f float64) float64 {
	return math.Round(f*1000) / 1000
}

// parseBool 解析布尔值
func parseBool(s string) bool {
	v := strings.ToLower(strings.TrimSpace(s))
	return v == "true" || v == "1" || v == "yes" || v == "on"
}

// truncate 截断字符串
func truncate(s string, n int) string {
	if len(s) <= n {
		return s
	}
	return s[:n] + "..."
}

// AnalyzeBatch 批量分析（带并发控制）
func (a *Analyzer) AnalyzeBatch(ctx context.Context, texts []string, maxConcurrency int) ([]Result, error) {
	if maxConcurrency <= 0 {
		maxConcurrency = 5
	}

	results := make([]Result, len(texts))
	sem := make(chan struct{}, maxConcurrency)
	var wg sync.WaitGroup
	var mu sync.Mutex
	var firstErr error

	for i, text := range texts {
		wg.Add(1)
		go func(idx int, txt string) {
			defer wg.Done()
			sem <- struct{}{}
			defer func() { <-sem }()

			result, err := a.Analyze(ctx, txt)
			mu.Lock()
			results[idx] = result
			if err != nil && firstErr == nil {
				firstErr = err
			}
			mu.Unlock()
		}(i, text)
	}

	wg.Wait()
	return results, firstErr
}

// ParseScore 解析分数字符串（兼容旧数据）
func ParseScore(s string) float64 {
	s = strings.TrimSpace(s)
	if s == "" {
		return 0.500
	}
	f, err := strconv.ParseFloat(s, 64)
	if err != nil {
		return 0.500
	}
	return roundTo3Decimals(f)
}
