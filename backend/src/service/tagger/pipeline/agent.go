package pipeline

import (
	"context"
	"fmt"
	"strings"
)

// agentSystemPrompt 指导 Agent 如何分阶段使用工具收集证据。
const agentSystemPrompt = `你是「舆情分析系统」的智能助手，正在为回答用户问题收集证据。你可以调用工具检索本地舆情知识库或联网搜索。

请遵循「本地优先、按需补充」的分阶段策略：

第一步——本地优先：
先用 search_local_knowledge 检索本系统已采集的舆情数据。这是首选来源，覆盖各平台的文章正文与用户评论。

第二步——评估缺口：
拿到本地结果后，对照用户意图判断信息是否充分。重点检查这些维度是否被覆盖：
- 事件/话题的背景与来龙去脉
- 多方观点与玩家/用户的真实声音
- 最新进展、近期动态
- 情感倾向与争议焦点
如果本地数据已能充分回答，就不要再调用工具，直接结束。

第三步——有针对性地联网补充：
仅当本地存在明确缺口时，才用 web_search 做针对性补充，而不是泛泛地搜。
- 联网时优先关注主流自媒体与游戏社区上的真实用户声音，如小红书、贴吧、抖音、TapTap、小黑盒等。
- 针对缺口设计查询（例如本地缺“玩家评价”，就专门搜玩家口碑/吐槽/评测）。
- 用 count 表达本次大约要补多少条：缺口小就少补，缺口大可多补；系统会在预算内放行。
- 不同缺口分多次、分角度调用，每次换不同查询词。

通用要求：
- 不要为了调用而调用：信息已足够时立即停止（无需在这一步写出完整答案，后续有专门环节作答）。
- 始终围绕用户的话题与意图，不要偏题到无关事件。`

// AgentNode 用真正的工具调用（function calling）循环替换原 ReActNode。
// LLM 自主决定调用哪些工具、调几轮；工具产出沉淀进 state.Observations / state.WebResults，
// 交给后续 Synthesize / Generate 节点。每次工具调用都 emit 成 ThinkStep 推到前端思考链。
type AgentNode struct {
	registry *Registry
	toolCall ToolCallFn
	maxSteps int
}

// NewAgentNode 创建 Agent 节点。maxSteps<=0 时默认 5。
func NewAgentNode(registry *Registry, toolCall ToolCallFn, maxSteps int) *AgentNode {
	if maxSteps <= 0 {
		maxSteps = 5
	}
	return &AgentNode{registry: registry, toolCall: toolCall, maxSteps: maxSteps}
}

func (n *AgentNode) Name() string  { return "agent" }
func (n *AgentNode) Title() string { return "工具调用" }

func (n *AgentNode) Execute(ctx context.Context, state *PipelineState, emit EmitFn) error {
	if n.registry == nil || n.registry.Len() == 0 || n.toolCall == nil {
		emit(ThinkStep{Step: n.Name(), Title: "工具调用", Content: "无可用工具，跳过检索", Status: StatusSkipped})
		return nil
	}
	if !state.NeedRetrieval {
		emit(ThinkStep{Step: n.Name(), Title: "工具调用", Content: "本次无需检索外部信息", Status: StatusSkipped})
		return nil
	}

	specs := n.registry.Specs()

	// 初始消息：系统提示 + 意图提示 + 用户问题
	msgs := []Message{{Role: "system", Content: agentSystemPrompt}}
	if hint := buildAgentUserHint(state); hint != "" {
		msgs = append(msgs, Message{Role: "system", Content: hint})
	}
	msgs = append(msgs, Message{Role: "user", Content: state.UserQuestion})

	callIdx := 0
	for step := 1; step <= n.maxSteps; step++ {
		if err := ctx.Err(); err != nil {
			return err
		}

		decideStep := fmt.Sprintf("agent_decide_%d", step)
		emit(ThinkStep{Step: decideStep, Title: "分析下一步", Content: "正在判断需要调用哪些工具…", Status: StatusRunning})

		resp, err := n.toolCall(ctx, msgs, specs)
		if err != nil {
			emit(ThinkStep{Step: decideStep, Title: "分析下一步", Content: "工具决策失败，基于已有信息继续", Status: StatusDone})
			break
		}

		// 无工具调用 → LLM 认为信息已足够，结束循环
		if len(resp.ToolCalls) == 0 {
			emit(ThinkStep{Step: decideStep, Title: "分析下一步", Content: "信息已足够，结束检索，进入综合分析", Status: StatusDone})
			break
		}

		// 记录 assistant 的工具调用消息（供回灌时上下文完整）
		emit(ThinkStep{Step: decideStep, Title: "分析下一步", Content: summarizeToolCalls(resp.ToolCalls), Status: StatusDone})
		msgs = append(msgs, Message{Role: "assistant", Content: resp.Content, ToolCalls: resp.ToolCalls})

		// 逐个执行工具
		for _, tc := range resp.ToolCalls {
			callIdx++
			toolStep := fmt.Sprintf("tool_%d", callIdx)
			title := toolTitle(tc.Name)
			emit(ThinkStep{Step: toolStep, Title: title, Content: "执行中：" + truncateStr(string(tc.Arguments), 80), Status: StatusRunning})

			result, ierr := n.registry.Invoke(ctx, tc.Name, tc.Arguments)
			if ierr != nil {
				// 工具不存在等错误：回灌错误信息，让 LLM 调整
				errText := fmt.Sprintf("工具 %s 执行出错：%v", tc.Name, ierr)
				msgs = append(msgs, toolResultMsg(tc, errText))
				emit(ThinkStep{Step: toolStep, Title: title, Content: errText, Status: StatusDone})
				continue
			}

			// 沉淀结构化产出
			if len(result.Observations) > 0 {
				state.Observations = append(state.Observations, result.Observations...)
			}
			if len(result.WebResults) > 0 {
				state.WebResults = append(state.WebResults, result.WebResults...)
			}

			// 回灌给 LLM
			msgs = append(msgs, toolResultMsg(tc, result.Content))

			display := result.Display
			if display == "" {
				display = truncateStr(result.Content, 200)
			}
			emit(ThinkStep{Step: toolStep, Title: title, Content: display, Status: StatusDone})
		}
	}

	// 同步兼容字段供后续节点使用
	state.RetrievedChunks = state.RetrievedChunks[:0]
	for _, o := range state.Observations {
		state.RetrievedChunks = append(state.RetrievedChunks, o.Chunk)
	}
	return nil
}

// buildAgentUserHint 把意图节点的产出与当前话题作为提示注入（辅助 LLM 选工具、锚定话题）。
func buildAgentUserHint(state *PipelineState) string {
	var sb strings.Builder
	if len(state.Topics) > 0 {
		sb.WriteString("当前分析话题：")
		sb.WriteString(strings.Join(state.Topics, "、"))
		sb.WriteString("（所有检索与联网搜索都应紧扣这些话题，不要偏题到无关事件）\n")
	}
	if state.Intent != "" {
		sb.WriteString("用户意图：")
		sb.WriteString(state.Intent)
		sb.WriteString("\n")
	}
	if len(state.Entities) > 0 {
		sb.WriteString("关键实体：")
		sb.WriteString(strings.Join(state.Entities, "、"))
		sb.WriteString("\n")
	}
	return strings.TrimSpace(sb.String())
}

func toolResultMsg(tc ToolCall, content string) Message {
	return Message{
		Role:       "tool",
		Content:    content,
		ToolCallID: tc.ID,
		Name:       tc.Name,
	}
}

// toolTitle 给前端展示用的工具中文标题。
func toolTitle(name string) string {
	switch name {
	case "search_local_knowledge":
		return "知识库检索"
	case "web_search":
		return "联网搜索"
	default:
		return "工具：" + name
	}
}

func summarizeToolCalls(calls []ToolCall) string {
	parts := make([]string, 0, len(calls))
	for _, c := range calls {
		parts = append(parts, "调用 "+toolTitle(c.Name))
	}
	return "决定" + strings.Join(parts, "、")
}
