// Package tagger 提供后台 AI 自动打标服务：周期性扫描 articles 表中 ai_tags 为空的记录，
// 批量调用 DeepSeek 生成 1~4 个标签，写回数据库。
//
// 设计要点：
//   - 增量：以 ai_tags IS NULL 作为待处理标记，无需额外状态表
//   - 批量：单次 LLM 调用处理 batchSize 条，显著降低 token 与延迟开销
//   - 节流：单次 tick 处理上限 maxPerTick，避免历史积压时把后台 goroutine 占满
//   - 失败隔离:单批失败仅跳过该批次，剩余批次继续；写回时按条容错
//   - 配置热更新：通过 UpdateConfig 可在运行期替换 cfg/apiKey；Start 循环每 tick 读取最新值
package tagger

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net/http"
	"os"
	"regexp"
	"strings"
	"sync"
	"time"

	"gorm.io/gorm"

	"opinion-analysis/config"
	"opinion-analysis/internal/model"
)

type Service struct {
	db     *gorm.DB
	client *http.Client

	mu     sync.RWMutex
	cfg    config.TaggerConfig
	apiKey string
}

func New(db *gorm.DB, cfg config.TaggerConfig) *Service {
	s := &Service{
		db:     db,
		client: &http.Client{Timeout: 60 * time.Second},
	}
	s.applyConfig(cfg)
	return s
}

// snapshot 返回当前生效的配置和已解析的 apiKey 拷贝。
func (s *Service) snapshot() (config.TaggerConfig, string) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.cfg, s.apiKey
}

// GetConfig 给 handler 用：返回当前生效配置以及 apiKey 是否已配置。
func (s *Service) GetConfig() (cfg config.TaggerConfig, apiKeySet bool) {
	cfg, key := s.snapshot()
	return cfg, key != ""
}

// UpdateConfig 替换当前配置。下一轮 tick 自动生效。
func (s *Service) UpdateConfig(cfg config.TaggerConfig) {
	s.applyConfig(cfg)
}

func (s *Service) applyConfig(cfg config.TaggerConfig) {
	apiKey := strings.TrimSpace(cfg.DeepseekAPIKey)
	if apiKey == "" {
		apiKey = strings.TrimSpace(os.Getenv("DEEPSEEK_API_KEY"))
	}
	s.mu.Lock()
	s.cfg = cfg
	s.apiKey = apiKey
	s.mu.Unlock()
}

// Start 启动后台定时任务。ctx 取消时优雅退出。
// 当前 Enabled=false 或 apiKey 缺失时 goroutine 不退出，仅跳过本轮，
// 以便管理员后续通过 UpdateConfig 启用时无需重启服务。
func (s *Service) Start(ctx context.Context) {
	log.Printf("[tagger] service goroutine started")

	timer := time.NewTimer(5 * time.Second)
	defer timer.Stop()

	for {
		select {
		case <-ctx.Done():
			log.Printf("[tagger] stopped: %v", ctx.Err())
			return
		case <-timer.C:
			cfg, apiKey := s.snapshot()
			interval := time.Duration(cfg.IntervalSeconds) * time.Second
			if interval <= 0 {
				interval = 2 * time.Minute
			}
			if !cfg.Enabled {
				timer.Reset(interval)
				continue
			}
			if apiKey == "" {
				log.Printf("[tagger] skip tick: deepseekApiKey 未配置")
				timer.Reset(interval)
				continue
			}
			n, err := s.RunOnce(ctx)
			if err != nil {
				log.Printf("[tagger] tick error: %v", err)
			} else if n > 0 {
				log.Printf("[tagger] tagged %d articles", n)
			}
			timer.Reset(interval)
		}
	}
}

// RunOnce 处理一轮：查询待打标文章 → 批量调用 LLM → 写回。返回成功打标的条数。
func (s *Service) RunOnce(ctx context.Context) (int, error) {
	cfg, apiKey := s.snapshot()
	if apiKey == "" {
		return 0, fmt.Errorf("deepseek api key not configured")
	}

	batchSize := cfg.BatchSize
	if batchSize <= 0 {
		batchSize = 20
	}
	maxPerTick := cfg.MaxPerTick
	if maxPerTick <= 0 {
		maxPerTick = 200
	}

	var pending []model.Article
	if err := s.db.WithContext(ctx).
		Where("ai_tags IS NULL").
		Order("id ASC").
		Limit(maxPerTick).
		Find(&pending).Error; err != nil {
		return 0, fmt.Errorf("query pending: %w", err)
	}
	if len(pending) == 0 {
		return 0, nil
	}

	totalDone := 0
	for start := 0; start < len(pending); start += batchSize {
		end := start + batchSize
		if end > len(pending) {
			end = len(pending)
		}
		chunk := pending[start:end]
		tagsByIndex, err := s.callDeepSeek(ctx, chunk, cfg, apiKey)
		if err != nil {
			log.Printf("[tagger] batch %d-%d LLM failed: %v", start, end, err)
			continue
		}
		for i, art := range chunk {
			tags := tagsByIndex[i]
			payload, _ := json.Marshal(tags)
			s.db.WithContext(ctx).
				Model(&model.Article{}).
				Where("id = ?", art.ID).
				Update("ai_tags", string(payload))
			totalDone++
		}
	}
	return totalDone, nil
}

// callDeepSeek 让模型对一批文章打标，返回 [][]string，外层下标 = chunk 的下标。
func (s *Service) callDeepSeek(ctx context.Context, chunk []model.Article, cfg config.TaggerConfig, apiKey string) ([][]string, error) {
	if len(chunk) == 0 {
		return nil, nil
	}

	var b strings.Builder
	for i, art := range chunk {
		text := art.Title
		if c := strings.TrimSpace(art.Content); c != "" {
			if len([]rune(c)) > 200 {
				c = string([]rune(c)[:200])
			}
			text = text + "。" + c
		}
		fmt.Fprintf(&b, "%d. %s\n", i+1, sanitizeForPrompt(text))
	}

	userPrompt := fmt.Sprintf(`你是一名中文舆情内容标签助手。请为下列每条新闻/帖子生成 1~4 个简洁的话题标签（每个 2~6 个汉字，名词或短语，
用于概括其核心内容、领域或事件主体）。要求：
- 标签必须能高度概括内容，便于检索与分类（如：科技创新、社会民生、资本市场、明星八卦、灾害事故、政策法规）
- 不要使用过于宽泛的词（如：新闻、消息、事件、报道）
- 同一条目内的标签不重复
- 严格按 JSON 数组输出，元素顺序对应编号，不要包含其它文字

待打标条目（共 %d 条）：
%s

输出格式示例：[{"i":1,"tags":["科技创新","人工智能"]},{"i":2,"tags":["资本市场","政策利好"]}]`, len(chunk), b.String())

	model := cfg.Model
	if strings.TrimSpace(model) == "" {
		model = "deepseek-chat"
	}
	reqBody := map[string]any{
		"model": model,
		"messages": []map[string]string{
			{"role": "system", "content": "你是专业的中文内容打标助手，只输出严格的 JSON。"},
			{"role": "user", "content": userPrompt},
		},
		"temperature":     0.2,
		"max_tokens":      2000,
		"response_format": map[string]string{"type": "json_object"},
	}
	payload, _ := json.Marshal(reqBody)

	baseURL := cfg.DeepseekBaseURL
	if strings.TrimSpace(baseURL) == "" {
		baseURL = "https://api.deepseek.com"
	}
	url := strings.TrimRight(baseURL, "/") + "/chat/completions"
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, url, bytes.NewReader(payload))
	if err != nil {
		return nil, err
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+apiKey)

	resp, err := s.client.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	respBody, _ := io.ReadAll(resp.Body)
	if resp.StatusCode/100 != 2 {
		return nil, fmt.Errorf("status=%d body=%s", resp.StatusCode, truncate(string(respBody), 300))
	}

	var apiResp struct {
		Choices []struct {
			Message struct {
				Content string `json:"content"`
			} `json:"message"`
		} `json:"choices"`
	}
	if err := json.Unmarshal(respBody, &apiResp); err != nil {
		return nil, fmt.Errorf("decode api: %w", err)
	}
	if len(apiResp.Choices) == 0 {
		return nil, fmt.Errorf("empty choices")
	}
	return parseTagsResponse(apiResp.Choices[0].Message.Content, len(chunk)), nil
}

// parseTagsResponse 解析模型输出。容忍以下三种形态：
//
//	1) [{"i":1,"tags":["a","b"]}, ...]
//	2) {"results":[{"i":1,"tags":["a","b"]}, ...]}
//	3) 文本中嵌着上述 JSON
func parseTagsResponse(raw string, expectedLen int) [][]string {
	out := make([][]string, expectedLen)
	for i := range out {
		out[i] = []string{}
	}
	text := strings.TrimSpace(raw)

	var arr []map[string]any
	if err := json.Unmarshal([]byte(text), &arr); err != nil {
		var wrap map[string]any
		if jerr := json.Unmarshal([]byte(text), &wrap); jerr == nil {
			for _, k := range []string{"results", "data", "items", "tags"} {
				if v, ok := wrap[k]; ok {
					if list, ok2 := v.([]any); ok2 {
						arr = coerceArr(list)
						break
					}
				}
			}
		}
		if arr == nil {
			if m := jsonArrayRe.FindString(text); m != "" {
				_ = json.Unmarshal([]byte(m), &arr)
			}
		}
	}
	if arr == nil {
		return out
	}
	for _, item := range arr {
		idx, ok := extractIndex(item)
		if !ok || idx < 0 || idx >= expectedLen {
			continue
		}
		tags := extractTags(item)
		out[idx] = tags
	}
	return out
}

var jsonArrayRe = regexp.MustCompile(`(?s)\[.*\]`)

func coerceArr(list []any) []map[string]any {
	out := make([]map[string]any, 0, len(list))
	for _, v := range list {
		if m, ok := v.(map[string]any); ok {
			out = append(out, m)
		}
	}
	return out
}

func extractIndex(m map[string]any) (int, bool) {
	for _, k := range []string{"i", "index", "id"} {
		if v, ok := m[k]; ok {
			switch x := v.(type) {
			case float64:
				return int(x) - 1, true
			case string:
				var n int
				if _, err := fmt.Sscanf(x, "%d", &n); err == nil {
					return n - 1, true
				}
			}
		}
	}
	return 0, false
}

func extractTags(m map[string]any) []string {
	v, ok := m["tags"]
	if !ok {
		return nil
	}
	list, ok := v.([]any)
	if !ok {
		return nil
	}
	seen := map[string]struct{}{}
	out := make([]string, 0, 4)
	for _, x := range list {
		s, ok := x.(string)
		if !ok {
			continue
		}
		s = strings.TrimSpace(s)
		if s == "" {
			continue
		}
		if _, dup := seen[s]; dup {
			continue
		}
		seen[s] = struct{}{}
		out = append(out, s)
		if len(out) >= 4 {
			break
		}
	}
	return out
}

// sanitizeForPrompt 把换行、回车折叠成空格，避免破坏 prompt 编号格式。
func sanitizeForPrompt(s string) string {
	s = strings.ReplaceAll(s, "\r", " ")
	s = strings.ReplaceAll(s, "\n", " ")
	return strings.TrimSpace(s)
}

func truncate(s string, n int) string {
	if len(s) <= n {
		return s
	}
	return s[:n] + "..."
}
