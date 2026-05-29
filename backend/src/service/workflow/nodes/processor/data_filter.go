package processor

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net/http"
	"regexp"
	"strings"
	"time"
	"unicode/utf8"

	"gorm.io/gorm"

	"opinion-analysis/config"
	"opinion-analysis/src/model"
	"opinion-analysis/src/service/workflow/nodes"
)

// DataFilterNode 数据过滤节点。
// 对上游传入的 articleIds 做过滤，输出保留下来的 articleIds（不删除数据库记录）。
// 两道过滤可独立启用，执行顺序固定为：先正则，再 AI。
//   - 正则过滤：按关键词（正则/包含）与字数范围筛选
//   - AI 过滤：把过滤需求描述交给大模型逐条判断是否保留
//
// AI 调用配置直接读取平台级 LLM 配置 config.Cfg.Tagger（与平台设置统一、随其变更），
// 不依赖打标服务实例，也不与其它节点耦合。
type DataFilterNode struct {
	*nodes.BaseNode
	db     *gorm.DB
	client *http.Client
}

// NewDataFilterNode 创建数据过滤节点
func NewDataFilterNode(db *gorm.DB) *DataFilterNode {
	return &DataFilterNode{
		BaseNode: nodes.NewBaseNode("data_filter"),
		db:       db,
		client:   &http.Client{Timeout: 60 * time.Second},
	}
}

// Validate 校验配置
func (n *DataFilterNode) Validate(config map[string]interface{}) error {
	return nil
}

// Execute 执行过滤
func (n *DataFilterNode) Execute(ctx context.Context, cfg map[string]interface{}, input map[string]interface{}) (map[string]interface{}, error) {
	enableRegex := n.GetBool(cfg, "enableRegex", false)
	enableAI := n.GetBool(cfg, "enableAI", false)

	articleIds := n.GetArticleIDs(input)
	inputCount := len(articleIds)

	log.Printf("[DataFilterNode] start: inputCount=%d enableRegex=%v enableAI=%v", inputCount, enableRegex, enableAI)

	if inputCount == 0 {
		log.Printf("[DataFilterNode] no upstream articleIds, skip")
		return n.MergeOutput(input, map[string]interface{}{
			"articleIds":         nodes.PackArticleIDs(articleIds),
			"filterInputCount":   0,
			"filterOutputCount":  0,
			"regexRemovedCount":  0,
			"aiRemovedCount":     0,
			"success":            true,
		}), nil
	}

	// 未启用任何过滤：原样透传
	if !enableRegex && !enableAI {
		return n.MergeOutput(input, map[string]interface{}{
			"articleIds":         nodes.PackArticleIDs(articleIds),
			"filterInputCount":   inputCount,
			"filterOutputCount":  inputCount,
			"regexRemovedCount":  0,
			"aiRemovedCount":     0,
			"success":            true,
		}), nil
	}

	// 加载文章
	var articles []model.Article
	if err := n.db.WithContext(ctx).Where("id IN ?", articleIds).Find(&articles).Error; err != nil {
		return nil, n.WrapError("加载文章失败", err)
	}

	survivors := articles
	regexRemoved := 0
	aiRemoved := 0

	// 1) 先正则过滤
	if enableRegex {
		kept, removed, err := n.applyRegexFilter(cfg, survivors)
		if err != nil {
			return nil, n.WrapError("正则过滤失败", err)
		}
		regexRemoved = removed
		survivors = kept
		log.Printf("[DataFilterNode] regex filter: removed=%d remaining=%d", removed, len(survivors))
	}

	// 2) 再 AI 过滤
	if enableAI && len(survivors) > 0 {
		kept, removed, err := n.applyAIFilter(ctx, cfg, survivors)
		if err != nil {
			return nil, n.WrapError("AI 过滤失败", err)
		}
		aiRemoved = removed
		survivors = kept
		log.Printf("[DataFilterNode] ai filter: removed=%d remaining=%d", removed, len(survivors))
	}

	keptIDs := make([]int64, 0, len(survivors))
	keptSet := make(map[uint]struct{}, len(survivors))
	for _, a := range survivors {
		keptIDs = append(keptIDs, int64(a.ID))
		keptSet[a.ID] = struct{}{}
	}

	// 被过滤掉的文章：在已加载文章中、但未通过过滤的
	rejectedIDs := make([]int64, 0)
	for _, a := range articles {
		if _, ok := keptSet[a.ID]; !ok {
			rejectedIDs = append(rejectedIDs, int64(a.ID))
		}
	}

	// 默认把被过滤掉的文章从库里移除（软删除），保证库内/下游/统计只保留过滤后的数据。
	// 可通过 deleteFiltered=false 关闭（仅缩小下游 articleIds，不动数据库）。
	deleteFiltered := n.GetBool(cfg, "deleteFiltered", true)
	deletedCount := 0
	if deleteFiltered && len(rejectedIDs) > 0 {
		if err := n.db.WithContext(ctx).Where("id IN ?", rejectedIDs).Delete(&model.Article{}).Error; err != nil {
			return nil, n.WrapError("删除被过滤文章失败", err)
		}
		deletedCount = len(rejectedIDs)
		log.Printf("[DataFilterNode] removed %d filtered articles from DB", deletedCount)
	}

	log.Printf("[DataFilterNode] done: input=%d output=%d (regexRemoved=%d aiRemoved=%d deleted=%d)",
		inputCount, len(keptIDs), regexRemoved, aiRemoved, deletedCount)

	return n.MergeOutput(input, map[string]interface{}{
		"articleIds":        nodes.PackArticleIDs(keptIDs),
		"filterInputCount":  inputCount,
		"filterOutputCount": len(keptIDs),
		"regexRemovedCount": regexRemoved,
		"aiRemovedCount":    aiRemoved,
		"deletedCount":      deletedCount,
		"success":           true,
	}), nil
}

// applyRegexFilter 按关键词（正则/包含）与字数范围过滤。返回保留列表与被移除条数。
func (n *DataFilterNode) applyRegexFilter(cfg map[string]interface{}, articles []model.Article) ([]model.Article, int, error) {
	keywords := n.GetStringSlice(cfg, "regexKeywords")
	keywordMode := n.GetString(cfg, "regexKeywordMode", "keep") // keep=保留命中, exclude=剔除命中
	minLength := n.GetInt(cfg, "minLength", 0)
	maxLength := n.GetInt(cfg, "maxLength", 0)

	// 预编译关键词：能编译成正则用正则，否则退化为子串包含（大小写不敏感）
	type matcher struct {
		re      *regexp.Regexp
		literal string
	}
	matchers := make([]matcher, 0, len(keywords))
	for _, kw := range keywords {
		kw = strings.TrimSpace(kw)
		if kw == "" {
			continue
		}
		if re, err := regexp.Compile("(?i)" + kw); err == nil {
			matchers = append(matchers, matcher{re: re})
		} else {
			matchers = append(matchers, matcher{literal: strings.ToLower(kw)})
		}
	}

	hitKeyword := func(text string) bool {
		if len(matchers) == 0 {
			return false
		}
		lower := strings.ToLower(text)
		for _, m := range matchers {
			if m.re != nil {
				if m.re.MatchString(text) {
					return true
				}
			} else if strings.Contains(lower, m.literal) {
				return true
			}
		}
		return false
	}

	kept := make([]model.Article, 0, len(articles))
	removed := 0
	for _, a := range articles {
		text := strings.TrimSpace(a.Title + " " + a.Content)
		length := utf8.RuneCountInString(strings.TrimSpace(a.Title + a.Content))

		// 字数过滤
		if minLength > 0 && length < minLength {
			removed++
			continue
		}
		if maxLength > 0 && length > maxLength {
			removed++
			continue
		}

		// 关键词过滤（仅当配置了关键词时生效）
		if len(matchers) > 0 {
			hit := hitKeyword(text)
			if keywordMode == "exclude" && hit {
				removed++
				continue
			}
			if keywordMode == "keep" && !hit {
				removed++
				continue
			}
		}

		kept = append(kept, a)
	}
	return kept, removed, nil
}

// applyAIFilter 把过滤需求交给大模型逐条判断是否保留。返回保留列表与被移除条数。
func (n *DataFilterNode) applyAIFilter(ctx context.Context, cfg map[string]interface{}, articles []model.Article) ([]model.Article, int, error) {
	requirement := strings.TrimSpace(n.GetString(cfg, "aiRequirement", ""))
	if requirement == "" {
		return nil, 0, fmt.Errorf("已启用 AI 过滤但未填写过滤需求")
	}

	// 读取平台级 LLM 配置（与平台设置统一，随其变更）
	llmCfg := config.Cfg.Tagger
	apiKey := strings.TrimSpace(llmCfg.LLMApiKey)
	if apiKey == "" {
		return nil, 0, fmt.Errorf("平台未配置大模型 API Key，无法进行 AI 过滤")
	}

	const batchSize = 20
	kept := make([]model.Article, 0, len(articles))
	removed := 0

	for start := 0; start < len(articles); start += batchSize {
		end := start + batchSize
		if end > len(articles) {
			end = len(articles)
		}
		chunk := articles[start:end]

		decisions, err := n.callLLMFilter(ctx, llmCfg, apiKey, requirement, chunk)
		if err != nil {
			return nil, 0, err
		}
		for i, a := range chunk {
			keepIt := true
			if i < len(decisions) {
				keepIt = decisions[i]
			}
			if keepIt {
				kept = append(kept, a)
			} else {
				removed++
			}
		}
	}
	return kept, removed, nil
}

// callLLMFilter 调用 OpenAI 兼容接口，返回每条是否保留的判定（顺序对应 chunk）。
func (n *DataFilterNode) callLLMFilter(ctx context.Context, cfg config.TaggerConfig, apiKey, requirement string, chunk []model.Article) ([]bool, error) {
	var b strings.Builder
	for i, art := range chunk {
		text := art.Title
		if c := strings.TrimSpace(art.Content); c != "" {
			if utf8.RuneCountInString(c) > 200 {
				c = string([]rune(c)[:200])
			}
			text = text + "。" + c
		}
		fmt.Fprintf(&b, "%d. %s\n", i+1, sanitizeFilterText(text))
	}

	userPrompt := fmt.Sprintf(`你是一名中文内容过滤助手。请根据「过滤需求」判断下列每条内容是否应当【保留】。

过滤需求：%s

判定规则：
- 满足保留条件的标记 keep=true，否则 keep=false
- 严格按 JSON 数组输出，元素顺序对应编号，不要输出其它文字

待判定条目（共 %d 条）：
%s

输出格式示例：[{"i":1,"keep":true},{"i":2,"keep":false}]`, requirement, len(chunk), b.String())

	modelName := strings.TrimSpace(cfg.LLMModel)
	if modelName == "" {
		modelName = "deepseek-chat"
	}
	reqBody := map[string]any{
		"model":       modelName,
		"messages":    []map[string]string{{"role": "user", "content": userPrompt}},
		"temperature": 0,
	}
	payload, _ := json.Marshal(reqBody)

	baseURL := strings.TrimSpace(cfg.LLMBaseURL)
	if baseURL == "" {
		baseURL = "https://api.deepseek.com"
	}
	url := strings.TrimRight(baseURL, "/") + "/chat/completions"

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, url, bytes.NewReader(payload))
	if err != nil {
		return nil, err
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+apiKey)

	resp, err := n.client.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	respBody, _ := io.ReadAll(resp.Body)
	if resp.StatusCode/100 != 2 {
		return nil, fmt.Errorf("llm status=%d body=%s", resp.StatusCode, truncateFilter(string(respBody), 400))
	}

	var apiResp struct {
		Choices []struct {
			Message struct {
				Content string `json:"content"`
			} `json:"message"`
		} `json:"choices"`
	}
	if err := json.Unmarshal(respBody, &apiResp); err != nil {
		return nil, fmt.Errorf("解析 llm 响应失败: %w", err)
	}
	if len(apiResp.Choices) == 0 {
		return nil, fmt.Errorf("llm 返回为空")
	}

	content := extractJSONArray(apiResp.Choices[0].Message.Content)
	var parsed []struct {
		I    int  `json:"i"`
		Keep bool `json:"keep"`
	}
	if err := json.Unmarshal([]byte(content), &parsed); err != nil {
		return nil, fmt.Errorf("解析 llm 判定结果失败: %w (raw=%s)", err, truncateFilter(content, 200))
	}

	// 按编号回填，默认保留（避免误删）
	decisions := make([]bool, len(chunk))
	for i := range decisions {
		decisions[i] = true
	}
	for _, p := range parsed {
		idx := p.I - 1
		if idx >= 0 && idx < len(decisions) {
			decisions[idx] = p.Keep
		}
	}
	return decisions, nil
}

func sanitizeFilterText(s string) string {
	s = strings.ReplaceAll(s, "\n", " ")
	s = strings.ReplaceAll(s, "\r", " ")
	return strings.TrimSpace(s)
}

func truncateFilter(s string, max int) string {
	if utf8.RuneCountInString(s) <= max {
		return s
	}
	return string([]rune(s)[:max]) + "…"
}

// extractJSONArray 从可能含有 ```json 包裹或前后缀的文本中提取 JSON 数组子串
func extractJSONArray(s string) string {
	s = strings.TrimSpace(s)
	start := strings.Index(s, "[")
	end := strings.LastIndex(s, "]")
	if start >= 0 && end > start {
		return s[start : end+1]
	}
	return s
}
