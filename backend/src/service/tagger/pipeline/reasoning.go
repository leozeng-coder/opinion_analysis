package pipeline

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
)

const synthesizeSystemPrompt = `你是一位资深舆情分析师。给定用户问题、推理过程，以及经检索筛选后累积的相关内容，
请先洞察用户的真实需求，再做一次深度综合分析。

输出 JSON（无其它文字）：
{
  "user_need": "分析用户提这个问题的真实意图：他深层想了解什么、关注的核心是什么（1-2句，要具体）",
  "dimensions": ["回答应覆盖的分析维度1", "维度2", "维度3"],
  "insights": ["关键洞察1", "关键洞察2", "..."]
}

要求：
- user_need 要超越问题字面，推断用户的真实关切（如问"口碑"可能关心是否值得信任/购买）
- dimensions 给出 3-4 个回答该问题应展开的角度（如：舆论倾向、争议焦点、情绪强度、传播趋势等），需贴合本问题
- insights 输出 4-6 条关键洞察，每条一句话、直接支撑回答；综合多条来源，指出共识、分歧、矛盾与趋势，而非罗列
- 区分"文章立场"与"评论观点"；涉及数据/比例时尽量具体；证据不足时点明不确定性
- 只输出 JSON，不要任何额外说明`

// synthesizeResult 是 Synthesize 节点解析出的结构化结果。
type synthesizeResult struct {
	UserNeed   string   `json:"user_need"`
	Dimensions []string `json:"dimensions"`
	Insights   []string `json:"insights"`
}

// SynthesizeNode 是 ReAct 循环之后的综合推理节点（前身为 ReasoningNode）。
// 它把累积的全部 observations 交给 LLM 做一次深度综合，产出关键洞察，
// 并连同 ReAct 推理链一起组装进 FinalContext，供生成节点作答。
type SynthesizeNode struct {
	llmCall LLMCallFn
}

func NewSynthesizeNode(llmCall LLMCallFn) *SynthesizeNode {
	return &SynthesizeNode{llmCall: llmCall}
}

func (n *SynthesizeNode) Name() string  { return "synthesize" }
func (n *SynthesizeNode) Title() string { return "综合推理" }

func (n *SynthesizeNode) Execute(ctx context.Context, state *PipelineState, emit EmitFn) error {
	if len(state.Observations) == 0 {
		emit(ThinkStep{Step: n.Name(), Title: n.Title(), Content: "无检索内容，跳过综合推理", Status: StatusSkipped})
		buildDeepContext(state)
		return nil
	}

	emit(ThinkStep{Step: n.Name(), Title: n.Title(), Content: "正在综合全部检索证据，提炼关键洞察…", Status: StatusRunning})

	// 组装综合分析的输入：用户问题 + 推理链 + 各条观察的提炼摘要
	var sb strings.Builder
	sb.WriteString("用户问题：")
	sb.WriteString(state.UserQuestion)

	if len(state.ReasoningChain) > 0 {
		sb.WriteString("\n\n推理过程：\n")
		for _, r := range state.ReasoningChain {
			sb.WriteString("- ")
			sb.WriteString(r)
			sb.WriteString("\n")
		}
	}

	sb.WriteString("\n相关内容（共 ")
	sb.WriteString(fmt.Sprintf("%d", len(state.Observations)))
	sb.WriteString(" 条，按相关度提炼）：\n")
	const maxRunes = 9000
	total := 0
	for i, o := range state.Observations {
		text := o.Extract
		if text == "" {
			text = truncateStr(o.Chunk, 300)
		}
		entry := fmt.Sprintf("%d. [相关度%d] %s\n", i+1, o.Score, text)
		r := []rune(entry)
		if total+len(r) > maxRunes {
			break
		}
		sb.WriteString(entry)
		total += len(r)
	}

	msgs := []map[string]string{
		{"role": "system", "content": synthesizeSystemPrompt},
		{"role": "user", "content": sb.String()},
	}

	raw, err := n.llmCall(ctx, msgs)
	if err != nil {
		emit(ThinkStep{Step: n.Name(), Title: n.Title(), Content: "综合推理失败，将直接使用检索内容作答", Status: StatusDone})
		buildDeepContext(state)
		return nil
	}

	res, perr := parseSynthesizeResult(raw)
	if perr != nil || len(res.Insights) == 0 {
		// 解析失败时降级：按旧的项目符号列表解析，至少拿到洞察
		insights := parseBulletList(raw)
		state.KeyInsights = insights
		state.KeyFacts = insights
		buildDeepContext(state)
		emit(ThinkStep{Step: n.Name(), Title: n.Title(), Content: strings.Join(insights, "\n"), Status: StatusDone})
		return nil
	}

	state.UserNeed = strings.TrimSpace(res.UserNeed)
	state.AnswerDimensions = cleanStrings(res.Dimensions)
	state.KeyInsights = cleanStrings(res.Insights)
	state.KeyFacts = state.KeyInsights // 兼容旧字段
	buildDeepContext(state)

	// 思考卡片展示：用户需求 + 分析维度 + 关键洞察，让用户看到 AI 理解了他要什么
	var disp strings.Builder
	if state.UserNeed != "" {
		disp.WriteString("【理解你的需求】")
		disp.WriteString(state.UserNeed)
		disp.WriteString("\n")
	}
	if len(state.AnswerDimensions) > 0 {
		disp.WriteString("【分析维度】")
		disp.WriteString(strings.Join(state.AnswerDimensions, "、"))
		disp.WriteString("\n")
	}
	if len(state.KeyInsights) > 0 {
		disp.WriteString("【关键洞察】\n")
		disp.WriteString(strings.Join(state.KeyInsights, "\n"))
	}
	emit(ThinkStep{Step: n.Name(), Title: n.Title(), Content: disp.String(), Status: StatusDone})
	return nil
}

// parseSynthesizeResult 容错解析 Synthesize 节点的 JSON 输出。
func parseSynthesizeResult(raw string) (synthesizeResult, error) {
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
		return synthesizeResult{}, fmt.Errorf("no JSON object found")
	}
	s = s[start : end+1]
	var r synthesizeResult
	if err := json.Unmarshal([]byte(s), &r); err != nil {
		return synthesizeResult{}, err
	}
	return r, nil
}

// cleanStrings 去除空白项并 trim。
func cleanStrings(in []string) []string {
	var out []string
	for _, s := range in {
		if t := strings.TrimSpace(s); t != "" {
			out = append(out, t)
		}
	}
	return out
}

// buildDeepContext 组装注入生成节点的最终上下文：
// 用户需求洞察 → 分析维度 → 关键洞察 → 推理过程 → 知识库证据（原文前附 Extract 提炼）。
func buildDeepContext(state *PipelineState) {
	var sb strings.Builder

	// 用户需求洞察 + 建议维度：让生成模型从更丰富的角度组织回答
	if state.UserNeed != "" {
		sb.WriteString("【用户需求洞察】\n")
		sb.WriteString(state.UserNeed)
		sb.WriteString("\n")
		if len(state.AnswerDimensions) > 0 {
			sb.WriteString("建议从以下维度展开回答：")
			sb.WriteString(strings.Join(state.AnswerDimensions, "、"))
			sb.WriteString("\n")
		}
		sb.WriteString("\n")
	}

	if len(state.KeyInsights) > 0 {
		sb.WriteString("【综合洞察 — 关键结论】\n")
		for i, f := range state.KeyInsights {
			sb.WriteString(fmt.Sprintf("%d. %s\n", i+1, f))
		}
		sb.WriteString("\n")
	}

	if len(state.ReasoningChain) > 0 {
		sb.WriteString("【推理过程】\n")
		for _, r := range state.ReasoningChain {
			sb.WriteString("- ")
			sb.WriteString(r)
			sb.WriteString("\n")
		}
		sb.WriteString("\n")
	}

	if len(state.Observations) > 0 {
		sb.WriteString("【知识库证据】（以 --- 分隔，每条先给相关性提炼，再附原文）\n")
		const maxChunkRunes = 11000
		total := 0
		for _, o := range state.Observations {
			// 方案A：原文前附上打分阶段已提炼的 Extract（相关性摘要），让模型先抓要点再看原文
			var head string
			if o.Extract != "" {
				head = fmt.Sprintf("[相关度%d] 要点：%s\n", o.Score, o.Extract)
			} else {
				head = fmt.Sprintf("[相关度%d]\n", o.Score)
			}
			block := head + o.Chunk
			r := []rune(block)
			if total+len(r) > maxChunkRunes {
				remaining := maxChunkRunes - total
				if remaining > 20 {
					sb.WriteString("---\n")
					sb.WriteString(string(r[:remaining]))
					sb.WriteString("…\n")
				}
				break
			}
			sb.WriteString("---\n")
			sb.WriteString(block)
			sb.WriteString("\n")
			total += len(r)
		}
	}

	state.FinalContext = sb.String()
}

// parseBulletList splits a "• item\n• item" style response into a slice.
func parseBulletList(s string) []string {
	var out []string
	for _, line := range strings.Split(s, "\n") {
		line = strings.TrimSpace(line)
		if line == "" {
			continue
		}
		// Accept •, -, *, or numbered prefixes
		for _, prefix := range []string{"• ", "- ", "* "} {
			if strings.HasPrefix(line, prefix) {
				line = strings.TrimPrefix(line, prefix)
				break
			}
		}
		if line != "" {
			out = append(out, line)
		}
	}
	return out
}

func min(a, b int) int {
	if a < b {
		return a
	}
	return b
}
