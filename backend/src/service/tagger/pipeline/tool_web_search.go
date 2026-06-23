package pipeline

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
)

// WebSearchFn 依赖注入的联网搜索函数，解耦 pipeline 与具体服务商（如博查 Bocha）。
type WebSearchFn func(ctx context.Context, query string, count int) ([]WebResult, error)

// WebSearchTool 把联网搜索包装成一个 Agent 工具。
// 当本地知识库覆盖不足、需要补充实时动态或外部声音时，由 LLM 自主调用。
//
// 三个特性配合需求设计：
//   - 话题关联：持有 topics，每次查询确定性地并入话题词，避免联网结果跑题（不依赖 LLM 自觉）。
//   - 渐进式扩容：budget 为后台配置的累计上限；工具内记录已取条数 used，
//     单次实际取数 = clamp(LLM 评估的 count, 1, 剩余预算)，累计永不超过 budget。
//   - 平台偏好：在系统提示与查询提示中引导优先主流自媒体/游戏社区（小红书/贴吧/抖音/taptap/小黑盒等）。
//
// 内部按 URL 去重，产出 []WebResult 沉淀给后续综合/生成节点。
type WebSearchTool struct {
	search WebSearchFn
	topics []string            // 当前会话的话题，用于把联网查询锚定到话题
	budget int                 // 累计结果上限（后台配置 webSearchCount）
	used   int                 // 已累计返回的结果数（渐进式扩容游标）
	seen   map[string]struct{} // 跨调用按 URL 去重
}

// NewWebSearchTool 创建联网搜索工具。budget<=0 时默认 5；topics 为当前会话选定的话题。
func NewWebSearchTool(search WebSearchFn, budget int, topics []string) *WebSearchTool {
	if budget <= 0 {
		budget = 5
	}
	return &WebSearchTool{
		search: search,
		topics: topics,
		budget: budget,
		seen:   make(map[string]struct{}),
	}
}

func (t *WebSearchTool) Name() string { return "web_search" }

func (t *WebSearchTool) Description() string {
	return "联网搜索实时信息，补充本地知识库未覆盖的内容。" +
		"**必须在query中明确包含当前话题名称**，避免泛化搜索导致结果偏离主题。" +
		"适用场景：最新动态、外部资料、主流社区（小红书/贴吧/B站/TapTap）的玩家声音。"
}

func (t *WebSearchTool) Parameters() map[string]any {
	return map[string]any{
		"type": "object",
		"properties": map[string]any{
			"query": map[string]any{
				"type": "string",
				"description": "搜索查询词。**必须包含话题名称**。" +
					"错误：'S1赛季对比'；正确：'赛尔号 S1赛季 S2赛季 对比'",
			},
			"count": map[string]any{
				"type":        "integer",
				"description": "需要的结果数（系统会在预算内自动调整）",
			},
		},
		"required": []string{"query"},
	}
}

type webSearchArgs struct {
	Query string `json:"query"`
	Count int    `json:"count"`
}

func (t *WebSearchTool) Invoke(ctx context.Context, raw json.RawMessage) (ToolResult, error) {
	if t.search == nil {
		return ToolResult{Content: "联网搜索不可用。", Display: "联网搜索未启用"}, nil
	}
	var args webSearchArgs
	_ = json.Unmarshal(raw, &args)
	query := strings.TrimSpace(args.Query)
	if query == "" {
		return ToolResult{Content: "联网搜索需要提供 query 参数。", Display: "联网搜索缺少查询词"}, nil
	}

	// 渐进式预算：本次可取 = clamp(LLM 评估的 count, 1, 剩余预算)。
	remaining := t.budget - t.used
	if remaining <= 0 {
		return ToolResult{
			Content: fmt.Sprintf("联网搜索次数已用尽（上限 %d 条），请基于已有信息回答。", t.budget),
			Display: "联网搜索次数已用尽",
		}, nil
	}
	want := args.Count
	if want <= 0 {
		want = 3 // LLM 未给出评估时，保守地小步补充
	}
	if want > remaining {
		want = remaining
	}

	// 话题关联：确定性地把当前话题并入查询，避免联网结果跑题。
	effectiveQuery := t.composeQuery(query)

	results, err := t.search(ctx, effectiveQuery, want)
	if err != nil {
		// 失败降级：不阻断，告知 LLM 可基于已有信息回答
		return ToolResult{
			Content: fmt.Sprintf("联网搜索「%s」失败：%v。请基于已有信息回答或尝试其他角度。", effectiveQuery, err),
			Display: "联网搜索失败：" + truncateStr(query, 30),
		}, nil
	}

	// 话题相关性过滤：移除标题中不包含任何话题关键词的结果
	var filtered []WebResult
	if len(t.topics) > 0 {
		for _, r := range results {
			titleLower := strings.ToLower(r.Title)
			var matched bool
			for _, topic := range t.topics {
				topicLower := strings.ToLower(strings.TrimSpace(topic))
				if topicLower != "" && strings.Contains(titleLower, topicLower) {
					matched = true
					break
				}
			}
			if matched {
				filtered = append(filtered, r)
			}
		}
	} else {
		filtered = results
	}

	// 按 URL 去重
	var fresh []WebResult
	for _, r := range filtered {
		key := strings.TrimSpace(r.URL)
		if key != "" {
			if _, ok := t.seen[key]; ok {
				continue
			}
			t.seen[key] = struct{}{}
		}
		fresh = append(fresh, r)
	}
	t.used += len(fresh)

	if len(fresh) == 0 {
		return ToolResult{
			Content: fmt.Sprintf("联网搜索「%s」未返回新结果。", effectiveQuery),
			Display: "联网搜索无新结果：" + truncateStr(query, 30),
		}, nil
	}

	// 组装回灌文本
	var sb strings.Builder
	sb.WriteString(fmt.Sprintf("联网搜索「%s」得到 %d 条结果（已用 %d/%d 条预算）：\n", effectiveQuery, len(fresh), t.used, t.budget))
	for i, r := range fresh {
		sb.WriteString(fmt.Sprintf("%d. %s", i+1, r.Title))
		if r.SiteName != "" {
			sb.WriteString(" （来源：" + r.SiteName + "）")
		}
		if r.Published != "" {
			sb.WriteString(" [" + r.Published + "]")
		}
		sb.WriteString("\n")
		summary := r.Summary
		if summary != "" {
			sb.WriteString("   " + truncateStr(summary, 300) + "\n")
		}
		if r.URL != "" {
			sb.WriteString("   链接：" + r.URL + "\n")
		}
	}
	if t.used >= t.budget {
		sb.WriteString("（联网搜索次数已用尽，如仍不足请基于已有信息作答）\n")
	}

	titles := make([]string, 0, len(fresh))
	for _, r := range fresh {
		if r.Title != "" {
			titles = append(titles, r.Title)
		}
	}
	display := fmt.Sprintf("联网搜索「%s」→ %d 条（累计 %d/%d）", truncateStr(query, 30), len(fresh), t.used, t.budget)
	if len(titles) > 0 {
		display += "\n" + strings.Join(titles, "\n")
	}

	return ToolResult{
		Content:    sb.String(),
		WebResults: fresh,
		Display:    display,
	}, nil
}

// composeQuery 把当前会话话题并入查询词，确保联网结果锚定在话题上。
// 已包含话题名的查询不重复追加。
// 同时添加引号强制精确匹配，避免通用词（如"S1 S2"）被过度发散。
func (t *WebSearchTool) composeQuery(query string) string {
	if len(t.topics) == 0 {
		return query
	}
	var missing []string
	for _, tp := range t.topics {
		tp = strings.TrimSpace(tp)
		if tp != "" && !strings.Contains(query, tp) {
			missing = append(missing, tp)
		}
	}
	if len(missing) == 0 {
		return query
	}

	// 话题名加引号强制精确匹配，避免发散
	// 例如："洛克王国世界" S1赛季 S2赛季 对比
	// 而非：洛克王国世界 S1赛季 S2赛季 对比（会匹配到所有游戏的S1 S2）
	quotedTopics := make([]string, len(missing))
	for i, tp := range missing {
		// 如果话题名包含空格或多个词，用引号括起来
		if strings.Contains(tp, " ") || len([]rune(tp)) > 4 {
			quotedTopics[i] = `"` + tp + `"`
		} else {
			quotedTopics[i] = tp
		}
	}

	return strings.Join(quotedTopics, " ") + " " + query
}
