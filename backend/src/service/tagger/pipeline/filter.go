package pipeline

import (
	"context"
	"encoding/json"
	"fmt"
	"strconv"
	"strings"
)

// filterSystemPrompt instructs the LLM to score and extract per-article.
const filterSystemPrompt = `你是一名专业的舆情信息筛选员。
给定用户意图和若干检索到的文章/评论片段（每段以 [序号] 开头），请对每段评估相关性并提炼关键信息。

输出要求：
- 返回 JSON 数组，每项包含 id、score、extract 三个字段
- id 必须是片段前方括号中的序号整数（例如片段标记为 [1]，则 id 填 1；[2] 则填 2），从 1 开始
- score 为 1-5 整数：1=完全无关，2=略有相关，3=部分相关，4=较相关，5=高度相关
- extract 为与用户意图直接相关的摘要（1-3句话），score<=2 时可为空字符串
- 必须为每个片段都返回一项，不要遗漏，不要新增不存在的序号
- 只输出 JSON 数组，不要任何额外说明`

// filterItem is the per-article result parsed from the LLM response.
type filterItem struct {
	ID      int    `json:"id"`
	Score   int    `json:"score"`
	Extract string `json:"extract"`
}

// scoreChunks 把候选片段交给 LLM 一次性打分(1-5)并提炼摘要，供 FilterNode 与 ReActNode 复用。
// 返回的 filterItem.ID 为 1-based 序号（对应传入 chunks 的下标 +1）。
func scoreChunks(ctx context.Context, llmCall LLMCallFn, intent, question string, chunks []string) ([]filterItem, error) {
	if len(chunks) == 0 {
		return nil, nil
	}
	var sb strings.Builder
	sb.WriteString("用户意图：")
	sb.WriteString(intent)
	if question != "" {
		sb.WriteString("\n用户问题：")
		sb.WriteString(question)
	}
	sb.WriteString("\n\n以下是检索到的内容片段，请按要求评估并提炼：\n")

	for i, chunk := range chunks {
		sb.WriteString(fmt.Sprintf("\n[%d]\n", i+1))
		// 剥掉片段头部的 [id=... platform=...] 元数据行，避免其中的 id= 与 [序号] 混淆，
		// 同时每篇保留约 700 字上下文，让评论类片段的信息不被过度截断
		sb.WriteString(truncateStr(stripMetaHeader(chunk), 700))
		sb.WriteString("\n")
	}

	msgs := []map[string]string{
		{"role": "system", "content": filterSystemPrompt},
		{"role": "user", "content": sb.String()},
	}
	raw, err := llmCall(ctx, msgs)
	if err != nil {
		return nil, err
	}
	return parseFilterResult(raw)
}

// FilterNode replaces ReasoningNode.
// It sends all candidate chunks to the LLM in one call, asking it to score
// relevance (1-5) and extract relevant info for each. Only articles scoring
// >= minScore are kept; their extracts become the FinalContext for generation.
type FilterNode struct {
	llmCall  LLMCallFn
	minScore int
}

func NewFilterNode(llmCall LLMCallFn) *FilterNode {
	return &FilterNode{llmCall: llmCall, minScore: 3}
}

func (n *FilterNode) Name() string  { return "filter" }
func (n *FilterNode) Title() string { return "相关性筛选" }

func (n *FilterNode) Execute(ctx context.Context, state *PipelineState, emit EmitFn) error {
	if len(state.RetrievedChunks) == 0 {
		emit(ThinkStep{Step: n.Name(), Title: n.Title(), Content: "无检索内容，跳过筛选", Status: StatusSkipped})
		return nil
	}

	emit(ThinkStep{
		Step:    n.Name(),
		Title:   n.Title(),
		Content: fmt.Sprintf("正在从 %d 篇候选中筛选相关内容…", len(state.RetrievedChunks)),
		Status:  StatusRunning,
	})

	items, err := scoreChunks(ctx, n.llmCall, state.Intent, state.UserQuestion, state.RetrievedChunks)
	if err != nil {
		// Fallback: keep all chunks, no filtering
		emit(ThinkStep{
			Step:    n.Name(),
			Title:   n.Title(),
			Content: fmt.Sprintf("筛选调用失败，保留全部 %d 篇候选内容", len(state.RetrievedChunks)),
			Status:  StatusDone,
		})
		buildFilteredContext(state, nil, state.RetrievedChunks)
		return nil
	}

	if len(items) == 0 {
		// Fallback: keep all chunks
		emit(ThinkStep{
			Step:    n.Name(),
			Title:   n.Title(),
			Content: fmt.Sprintf("结果解析失败，保留全部 %d 篇候选内容", len(state.RetrievedChunks)),
			Status:  StatusDone,
		})
		buildFilteredContext(state, nil, state.RetrievedChunks)
		return nil
	}

	// Filter to relevant items, build summary lines for the ThinkStep
	type keptItem struct {
		chunk   string
		extract string
		score   int
	}
	var kept []keptItem
	validMapped := 0
	for _, item := range items {
		idx := item.ID - 1 // convert 1-based to 0-based
		if idx < 0 || idx >= len(state.RetrievedChunks) {
			continue
		}
		validMapped++
		if item.Score >= n.minScore {
			kept = append(kept, keptItem{
				chunk:   state.RetrievedChunks[idx],
				extract: item.Extract,
				score:   item.Score,
			})
		}
	}

	// 安全兜底：打分结果无一能映射到候选（LLM 把文章 id 当序号导致全部越界），保留全部候选。
	if validMapped == 0 {
		emit(ThinkStep{
			Step:    n.Name(),
			Title:   n.Title(),
			Content: fmt.Sprintf("打分序号异常，保留全部 %d 篇候选内容", len(state.RetrievedChunks)),
			Status:  StatusDone,
		})
		buildFilteredContext(state, nil, state.RetrievedChunks)
		return nil
	}

	// Build emit content: summary line + one line per kept article
	summaryLines := []string{
		fmt.Sprintf("从 %d 篇中筛选出 %d 篇相关内容", len(state.RetrievedChunks), len(kept)),
	}
	for _, k := range kept {
		title := extractTitle(k.chunk)
		if title != "" {
			summaryLines = append(summaryLines, fmt.Sprintf("• %s（相关度 %d/5）", title, k.score))
		}
	}

	emit(ThinkStep{
		Step:    n.Name(),
		Title:   n.Title(),
		Content: strings.Join(summaryLines, "\n"),
		Status:  StatusDone,
	})

	if len(kept) == 0 {
		// Nothing relevant — generation node will answer from general knowledge
		state.FinalContext = ""
		return nil
	}

	// Assemble FinalContext: extracts first, then full chunks for the generator
	var ctx2 strings.Builder
	ctx2.WriteString("【筛选摘要 — 相关内容提炼】\n")
	for i, k := range kept {
		if k.extract != "" {
			ctx2.WriteString(fmt.Sprintf("%d. %s\n", i+1, k.extract))
		}
	}
	ctx2.WriteString("\n【原文摘录】（以 --- 分隔）\n")
	const maxFullRunes = 10000
	totalRunes := 0
	for _, k := range kept {
		r := []rune(k.chunk)
		if totalRunes+len(r) > maxFullRunes {
			remaining := maxFullRunes - totalRunes
			if remaining > 20 {
				ctx2.WriteString("---\n")
				ctx2.WriteString(string(r[:remaining]))
				ctx2.WriteString("…\n")
			}
			break
		}
		ctx2.WriteString("---\n")
		ctx2.WriteString(k.chunk)
		ctx2.WriteString("\n")
		totalRunes += len(r)
	}

	state.FinalContext = ctx2.String()
	return nil
}

// buildFilteredContext is used in fallback paths when filtering fails.
func buildFilteredContext(state *PipelineState, _ []filterItem, chunks []string) {
	const maxRunes = 10000
	var sb strings.Builder
	sb.WriteString("【知识库检索摘录】（以 --- 分隔）\n")
	total := 0
	for _, chunk := range chunks {
		r := []rune(chunk)
		if total+len(r) > maxRunes {
			remaining := maxRunes - total
			if remaining > 20 {
				sb.WriteString("---\n")
				sb.WriteString(string(r[:remaining]))
				sb.WriteString("…\n")
			}
			break
		}
		sb.WriteString("---\n")
		sb.WriteString(chunk)
		sb.WriteString("\n")
		total += len(r)
	}
	state.FinalContext = sb.String()
}

// parseFilterResult extracts the JSON array from the LLM response.
func parseFilterResult(raw string) ([]filterItem, error) {
	// Strip markdown fences
	s := raw
	for _, fence := range []string{"```json", "```"} {
		if idx := indexOf(s, fence); idx >= 0 {
			s = s[idx+len(fence):]
		}
	}
	if idx := indexOf(s, "```"); idx >= 0 {
		s = s[:idx]
	}
	s = strings.TrimSpace(s)

	// Find the outermost JSON array
	start := strings.IndexByte(s, '[')
	end := strings.LastIndexByte(s, ']')
	if start < 0 || end <= start {
		return nil, fmt.Errorf("no JSON array found")
	}
	s = s[start : end+1]

	var items []filterItem
	if err := json.Unmarshal([]byte(s), &items); err != nil {
		return nil, err
	}
	return items, nil
}

// extractTitle pulls the "标题：" line from a formatted chunk string.
func extractTitle(chunk string) string {
	for _, line := range strings.Split(chunk, "\n") {
		line = strings.TrimSpace(line)
		if strings.HasPrefix(line, "标题：") {
			return strings.TrimPrefix(line, "标题：")
		}
	}
	return ""
}

// extractArticleID 从格式化片段头部解析 "id=N"，用于跨轮去重。无法解析时返回 0。
func extractArticleID(chunk string) int {
	idx := strings.Index(chunk, "id=")
	if idx < 0 {
		return 0
	}
	rest := chunk[idx+3:]
	end := 0
	for end < len(rest) && rest[end] >= '0' && rest[end] <= '9' {
		end++
	}
	if end == 0 {
		return 0
	}
	n, err := strconv.Atoi(rest[:end])
	if err != nil {
		return 0
	}
	return n
}

// stripMetaHeader 去掉片段开头的元数据行（形如 "[id=8617 platform=... ]"），
// 只保留标题与正文，避免元数据中的 "id=" 与打分用的 [序号] 混淆。
func stripMetaHeader(chunk string) string {
	s := strings.TrimLeft(chunk, " \t\n")
	if strings.HasPrefix(s, "[id=") {
		if nl := strings.IndexByte(s, '\n'); nl >= 0 {
			return s[nl+1:]
		}
	}
	return chunk
}
