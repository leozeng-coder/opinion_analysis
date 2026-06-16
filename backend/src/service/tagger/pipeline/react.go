package pipeline

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
)

// reactReasonPrompt 指导 Reason 步骤：评估已有证据的充分性，并严格判断是否需要补充检索。
const reactReasonPrompt = `你是一名舆情分析的推理规划师，正在用「边推理边检索」的方式收集证据来回答用户问题。
给定用户问题、已检索的角度、以及已获得的内容摘要，请对当前证据做一次有深度的评估，并决定是否需要补充检索。

输出 JSON（无其它文字）：
{
  "assessment": "对已掌握信息的分析：覆盖了哪些方面、揭示了什么倾向或分歧、能支撑回答的哪些部分（2-3句，要具体，引用实际内容而非空话）",
  "gap": "要完整回答用户问题，当前仍缺失的关键信息；若已足够则填 \"无\"",
  "sufficient": true,
  "next_queries": ["补充检索词"]
}

判断原则（务必严格遵守）：
- 默认倾向于停止。只有当存在「会实质改变最终结论」的关键信息缺口时，才设 sufficient=false。
- 若现有内容已能对用户问题给出有依据、较完整的回答，必须设 sufficient=true，不要为了检索而检索。
- 信息只是"还能更多"但不影响结论方向时，应判定为充分（sufficient=true）。
- sufficient=true 时 next_queries 必须为空数组。
- next_queries 最多 2 个，必须是与已检索角度明显不同的新方向，每个不超过 30 字。
- 只输出 JSON，不要任何解释。`

// reactDecision 是 Reason 步骤解析出的决策。
type reactDecision struct {
	Assessment  string   `json:"assessment"`
	Gap         string   `json:"gap"`
	Sufficient  bool     `json:"sufficient"`
	NextQueries []string `json:"next_queries"`
}

// ReActNode 实现「推理↔检索」交替循环（ReAct），并支持渐进式扩容：
//
//	首轮用 Intent 拆解的 sub_queries、以大模型评估的 InitialTopK 条/角度检索 → 打分提炼 → 累积；
//	之后由 Reason 评估充分性。当「补充检索的新增率明显下降，但证据仍不足」时，
//	逐级扩大检索量并放宽筛选门槛（把背景类内容也纳入），直到充分或达到上限。
//
// 步骤事件使用唯一 step key 兼容前端去重，但展示文案不含"第N轮"字样。
type ReActNode struct {
	search       RAGSearchFn
	llmCall      LLMCallFn
	maxRounds    int
	baseMinScore int     // 默认筛选门槛
	maxExpandLvl int     // 最大扩容级别
	lowNewRate   float64 // 触发扩容的新增率阈值
}

func NewReActNode(search RAGSearchFn, llmCall LLMCallFn) *ReActNode {
	return &ReActNode{
		search:       search,
		llmCall:      llmCall,
		maxRounds:    4,
		baseMinScore: 3,
		maxExpandLvl: 2,
		lowNewRate:   0.4,
	}
}

// expandParams 按扩容级别返回该级别的检索量与筛选门槛。
// level 0 用大模型评估的 base 值；逐级放大检索量、放宽门槛以纳入背景内容。
func expandParams(level, baseTopK int) (perQuery, minScore int) {
	switch level {
	case 0:
		return baseTopK, 3
	case 1:
		return clampInt(baseTopK*2, 8, 24), 2
	default: // 2+
		return clampInt(baseTopK*3, 12, 30), 2
	}
}

func clampInt(v, lo, hi int) int {
	if v < lo {
		return lo
	}
	if v > hi {
		return hi
	}
	return v
}

func (n *ReActNode) Name() string  { return "react" }
func (n *ReActNode) Title() string { return "推理与检索" }

func (n *ReActNode) Execute(ctx context.Context, state *PipelineState, emit EmitFn) error {
	if !state.NeedRetrieval || n.search == nil {
		emit(ThinkStep{Step: n.Name(), Title: "知识库检索", Content: "无需检索本地知识库", Status: StatusSkipped})
		return nil
	}

	seen := make(map[int]struct{})         // 已纳入 observations 的 article id，跨轮去重
	usedQueries := make(map[string]struct{}) // 已检索过的角度
	var allAngles []string                   // 所有用过的角度（扩容时重检索用）

	queries := state.SubQueries
	if len(queries) == 0 {
		queries = []string{state.RetrievalQuery}
	}

	baseTopK := state.InitialTopK
	if baseTopK <= 0 {
		baseTopK = 8
	}
	expandLevel := 0 // 当前扩容级别

	for round := 1; round <= n.maxRounds; round++ {
		if err := ctx.Err(); err != nil {
			return err
		}

		expanding := expandLevel > 0 && round > 1
		perQuery, minScore := expandParams(expandLevel, baseTopK)

		// 扩容轮重检索所有历史角度（靠 seen 去重只取新文章）；常规轮检索新角度
		var roundQueries []string
		if expanding {
			roundQueries = uniqueStrings(allAngles)
		} else {
			roundQueries = dedupQueries(queries, usedQueries)
			allAngles = append(allAngles, roundQueries...)
		}
		if len(roundQueries) == 0 {
			break
		}

		retrievalStep := fmt.Sprintf("retrieval_%d", round)
		filterStep := fmt.Sprintf("filter_%d", round)
		retrievalTitle, filterTitle := "知识库检索", "相关性筛选"
		if expanding {
			retrievalTitle, filterTitle = "扩大检索范围", "扩容内容筛选"
		} else if round > 1 {
			retrievalTitle, filterTitle = "补充检索", "补充内容筛选"
		}

		// --- Act: 检索 ---
		runHint := "检索中：" + strings.Join(roundQueries, " / ")
		if expanding {
			runHint = fmt.Sprintf("正在扩大搜索范围（每角度 %d 条、相关门槛放宽至 %d 分）：%s",
				perQuery, minScore, strings.Join(roundQueries, " / "))
		}
		emit(ThinkStep{Step: retrievalStep, Title: retrievalTitle, Content: runHint, Status: StatusRunning})

		roundChunks, returned := n.retrieveRound(ctx, roundQueries, state.Topics, perQuery, seen)

		// 新增率：去重后的新内容 / 检索返回总数
		newRate := 1.0
		if returned > 0 {
			newRate = float64(len(roundChunks)) / float64(returned)
		}

		if len(roundChunks) == 0 {
			emit(ThinkStep{Step: retrievalStep, Title: retrievalTitle, Content: "未检索到新的相关内容", Status: StatusDone})
			break
		}

		titles := []string{fmt.Sprintf("检索到 %d 条新内容（去重后新增率 %.0f%%）", len(roundChunks), newRate*100)}
		for _, c := range roundChunks {
			if t := extractTitle(c); t != "" {
				titles = append(titles, t)
			}
		}
		emit(ThinkStep{Step: retrievalStep, Title: retrievalTitle, Content: strings.Join(titles, "\n"), Status: StatusDone})

		// --- 打分提炼 ---
		emit(ThinkStep{Step: filterStep, Title: filterTitle, Content: fmt.Sprintf("正在评估 %d 条内容的相关性…", len(roundChunks)), Status: StatusRunning})

		kept := n.scoreAndAccumulate(ctx, state, roundChunks, minScore, round, seen)
		keepRate := 0.0
		if len(roundChunks) > 0 {
			keepRate = float64(kept) / float64(len(roundChunks))
		}

		filterDone := fmt.Sprintf("保留 %d 条高相关内容（采纳率 %.0f%%，共掌握 %d 条证据）", kept, keepRate*100, len(state.Observations))
		emit(ThinkStep{Step: filterStep, Title: filterTitle, Content: filterDone, Status: StatusDone})

		if round == n.maxRounds {
			break
		}

		// --- Reason: 评估充分性 ---
		decision := n.reason(ctx, state, emit, round)
		if decision.Sufficient {
			break
		}

		// --- 决定下一步：渐进式扩容 or 换新角度 ---
		// 触发扩容：新增率明显下降（常规检索已挖空）且证据仍不足，且未到扩容上限
		if round > 1 && newRate < n.lowNewRate && expandLevel < n.maxExpandLvl {
			expandLevel++
			nextPer, nextMin := expandParams(expandLevel, baseTopK)
			emit(ThinkStep{
				Step:  fmt.Sprintf("expand_%d", round),
				Title: "扩大搜索范围",
				Content: fmt.Sprintf("新增内容明显减少（新增率 %.0f%% < %.0f%%），但背景信息仍不足；\n正在扩大搜索范围：检索量提升至每角度 %d 条，相关门槛放宽至 %d 分，纳入更多背景内容。",
					newRate*100, n.lowNewRate*100, nextPer, nextMin),
				Status: StatusDone,
			})
			continue // 下一轮以新级别重检索历史角度
		}

		// 否则换大模型给出的新角度（不扩容）
		if len(decision.NextQueries) == 0 {
			break
		}
		queries = decision.NextQueries
		expandLevel = 0 // 新角度回到常规检索量
	}

	// 把累积的 observations 同步到兼容字段，供后续节点（Synthesize/Generate）使用
	state.RetrievedChunks = state.RetrievedChunks[:0]
	for _, o := range state.Observations {
		state.RetrievedChunks = append(state.RetrievedChunks, o.Chunk)
	}
	return nil
}

// retrieveRound 对一组查询各检索 perQuery 条，合并并按 article id 去重（排除已 seen 的）。
// 返回去重后的新片段，以及检索返回的总条数（用于计算新增率）。
func (n *ReActNode) retrieveRound(ctx context.Context, queries []string, topics []string, perQuery int, seen map[int]struct{}) ([]string, int) {
	var out []string
	returned := 0
	roundSeen := make(map[int]struct{})
	for _, q := range queries {
		chunks, err := n.search(ctx, q, perQuery, topics)
		if err != nil {
			continue
		}
		returned += len(chunks)
		for _, c := range chunks {
			id := extractArticleID(c)
			if id != 0 {
				if _, ok := seen[id]; ok {
					continue
				}
				if _, ok := roundSeen[id]; ok {
					continue
				}
				roundSeen[id] = struct{}{}
			}
			out = append(out, c)
		}
	}
	return out, returned
}

// scoreAndAccumulate 对本轮新片段打分，把 >= minScore 的累积进 state.Observations，返回本轮保留数。
func (n *ReActNode) scoreAndAccumulate(ctx context.Context, state *PipelineState, chunks []string, minScore, round int, seen map[int]struct{}) int {
	items, err := scoreChunks(ctx, n.llmCall, state.Intent, state.UserQuestion, chunks)
	if err != nil || len(items) == 0 {
		return n.accumulateAll(state, chunks, minScore, round, seen) // 打分失败：全部纳入，避免丢数据
	}

	kept := 0
	validMapped := 0 // 成功映射到候选片段的 item 数（不论分数）
	for _, item := range items {
		idx := item.ID - 1
		if idx < 0 || idx >= len(chunks) {
			continue
		}
		validMapped++
		if item.Score < minScore {
			continue
		}
		c := chunks[idx]
		id := extractArticleID(c)
		markSeen(seen, id)
		state.Observations = append(state.Observations, Observation{
			ArticleID: id,
			Chunk:     c,
			Extract:   item.Extract,
			Score:     item.Score,
			Round:     round,
		})
		kept++
	}

	// 安全兜底：若没有任何 item 能映射到候选片段（典型为 LLM 把文章 id 当成序号导致全部越界），
	// 说明打分结果不可信，保留全部候选而非全丢——杜绝"检索到却说没检索到"。
	if validMapped == 0 {
		return n.accumulateAll(state, chunks, minScore, round, seen)
	}
	return kept
}

// accumulateAll 把全部片段以中性分纳入 observations（降级路径）。
func (n *ReActNode) accumulateAll(state *PipelineState, chunks []string, minScore, round int, seen map[int]struct{}) int {
	kept := 0
	for _, c := range chunks {
		id := extractArticleID(c)
		markSeen(seen, id)
		state.Observations = append(state.Observations, Observation{
			ArticleID: id, Chunk: c, Score: minScore, Round: round,
		})
		kept++
	}
	return kept
}

// reason 调用 LLM 评估证据充分性，emit 推理过程，并返回是否继续的决策。
func (n *ReActNode) reason(ctx context.Context, state *PipelineState, emit EmitFn, round int) reactDecision {
	const reasonStep = "reason" // 单一推理步骤，不分轮次
	emit(ThinkStep{Step: reasonStep, Title: "推理评估", Content: "正在评估已掌握的证据是否足以回答问题…", Status: StatusRunning})

	var sb strings.Builder
	sb.WriteString("用户问题：")
	sb.WriteString(state.UserQuestion)
	if len(state.SubQueries) > 0 {
		sb.WriteString("\n已检索的角度：")
		sb.WriteString(strings.Join(state.SubQueries, " / "))
	}
	sb.WriteString("\n\n已获得的内容摘要：\n")
	if len(state.Observations) == 0 {
		sb.WriteString("（暂无）\n")
	} else {
		const maxRunes = 4500
		total := 0
		for i, o := range state.Observations {
			line := o.Extract
			if line == "" {
				line = truncateStr(o.Chunk, 140)
			}
			entry := fmt.Sprintf("%d. %s\n", i+1, line)
			r := []rune(entry)
			if total+len(r) > maxRunes {
				break
			}
			sb.WriteString(entry)
			total += len(r)
		}
	}

	msgs := []map[string]string{
		{"role": "system", "content": reactReasonPrompt},
		{"role": "user", "content": sb.String()},
	}
	raw, err := n.llmCall(ctx, msgs)
	if err != nil {
		emit(ThinkStep{Step: reasonStep, Title: "推理评估", Content: "已基于现有证据完成分析判断", Status: StatusDone})
		return reactDecision{Sufficient: true}
	}
	d, perr := parseReactDecision(raw)
	if perr != nil {
		emit(ThinkStep{Step: reasonStep, Title: "推理评估", Content: "已基于现有证据完成分析判断", Status: StatusDone})
		return reactDecision{Sufficient: true}
	}

	// 清洗 next_queries
	var clean []string
	for _, q := range d.NextQueries {
		if s := strings.TrimSpace(q); s != "" {
			clean = append(clean, s)
		}
	}
	d.NextQueries = clean

	// 组装有深度的推理展示：评估 → 缺口 → 决策
	var disp strings.Builder
	if d.Assessment != "" {
		disp.WriteString(d.Assessment)
		// 同步进推理链，供综合节点与最终上下文使用（不带轮次字样）
		state.ReasoningChain = append(state.ReasoningChain, d.Assessment)
	}
	gap := strings.TrimSpace(d.Gap)
	hasGap := gap != "" && gap != "无" && gap != "暂无"

	if d.Sufficient || len(d.NextQueries) == 0 {
		if disp.Len() > 0 {
			disp.WriteString("\n")
		}
		disp.WriteString("证据已足以支撑回答，结束检索，进入综合分析。")
	} else {
		if hasGap {
			disp.WriteString("\n仍需补充：")
			disp.WriteString(gap)
		}
		disp.WriteString("\n继续检索：")
		disp.WriteString(strings.Join(d.NextQueries, " / "))
	}

	emit(ThinkStep{Step: reasonStep, Title: "推理评估", Content: disp.String(), Status: StatusDone})
	return d
}

// parseReactDecision 容错解析 Reason 步骤的 JSON 输出。
func parseReactDecision(raw string) (reactDecision, error) {
	s := raw
	for _, fence := range []string{"```json", "```"} {
		if idx := indexOf(s, fence); idx >= 0 {
			s = s[idx+len(fence):]
		}
	}
	if idx := indexOf(s, "```"); idx >= 0 {
		s = s[:idx]
	}
	start := strings.IndexByte(s, '{')
	end := strings.LastIndexByte(s, '}')
	if start < 0 || end <= start {
		return reactDecision{}, fmt.Errorf("no JSON object found")
	}
	s = s[start : end+1]
	var d reactDecision
	if err := json.Unmarshal([]byte(s), &d); err != nil {
		return reactDecision{}, err
	}
	return d, nil
}

// dedupQueries 过滤掉已用过的查询，并把保留的标记为已用。
func dedupQueries(queries []string, used map[string]struct{}) []string {
	var out []string
	for _, q := range queries {
		key := strings.TrimSpace(q)
		if key == "" {
			continue
		}
		if _, ok := used[key]; ok {
			continue
		}
		used[key] = struct{}{}
		out = append(out, key)
	}
	return out
}

func markSeen(seen map[int]struct{}, id int) {
	if id != 0 {
		seen[id] = struct{}{}
	}
}

// uniqueStrings 对字符串切片去重（保持顺序），用于扩容轮重检索历史角度。
func uniqueStrings(in []string) []string {
	seen := make(map[string]struct{}, len(in))
	var out []string
	for _, s := range in {
		s = strings.TrimSpace(s)
		if s == "" {
			continue
		}
		if _, ok := seen[s]; ok {
			continue
		}
		seen[s] = struct{}{}
		out = append(out, s)
	}
	return out
}
