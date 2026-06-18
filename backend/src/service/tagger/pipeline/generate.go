package pipeline

import (
	"context"
)

// StreamFn streams LLM-generated text chunks to the caller via a channel.
// It mirrors the signature used by tagger.Service.ChatCompletionStream.
type StreamFn func(ctx context.Context, msgs []map[string]string) (<-chan string, <-chan error)

// GenerateNode is the final node. It builds the enriched system prompt
// (injecting the reasoning context) and streams the response back to
// the HTTP handler via the provided channels.
type GenerateNode struct {
	stream     StreamFn
	contentCh  chan<- string
	systemBase string // the base system prompt shared with the normal chat endpoint
}

func NewGenerateNode(stream StreamFn, contentCh chan<- string, systemBase string) *GenerateNode {
	return &GenerateNode{stream: stream, contentCh: contentCh, systemBase: systemBase}
}

func (n *GenerateNode) Name() string  { return "generate" }
func (n *GenerateNode) Title() string { return "生成回答" }

func (n *GenerateNode) Execute(ctx context.Context, state *PipelineState, emit EmitFn) error {
	emit(ThinkStep{Step: n.Name(), Title: n.Title(), Content: "正在生成回答…", Status: StatusRunning})

	msgs := buildDeepChatMessages(state, n.systemBase)

	chunkCh, errCh := n.stream(ctx, msgs)

	for {
		select {
		case chunk, ok := <-chunkCh:
			if !ok {
				emit(ThinkStep{Step: n.Name(), Title: n.Title(), Status: StatusDone})
				return nil
			}
			select {
			case n.contentCh <- chunk:
			case <-ctx.Done():
				return ctx.Err()
			}

		case err := <-errCh:
			if err != nil {
				return err
			}

		case <-ctx.Done():
			return ctx.Err()
		}
	}
}

// deepAnswerDirective 强制生成节点产出结构化、有深度的回答。
const deepAnswerDirective = `

## 深度回答要求（本次为深度思考模式）
你已经过多轮检索与综合推理，现在要基于上方【用户需求洞察】【综合洞察】【推理过程】【知识库证据】【联网搜索结果】，输出一篇有深度、成体系的舆情分析，而不是简短答复。

写作框架（用 Markdown，分小标题组织，不要从头到尾一段话）：
1. **核心结论**：开篇用 2-4 句直接回答用户最关心的问题，先给判断和总体态势。
2. **分维度展开**：紧扣【用户需求洞察】建议的分析维度逐一展开（如舆论倾向、争议焦点、情绪强度、传播趋势、各方立场等）。每个维度：
   - 给出该维度下的判断；
   - 用知识库证据或联网结果中的具体内容、数据、典型言论来支撑，标注来源倾向（文章观点 / 用户评论 / 联网来源）；
   - 有数据或比例时具体引用，避免"很多人""普遍认为"这类空泛表述。
3. **分歧与矛盾**：主动指出不同声音、对立观点、前后变化或与预期不符之处，不要回避不确定性；证据不足的地方明确说明。
4. **趋势与影响**：在证据支持下，分析事态的发展趋势、潜在影响或值得关注的信号。
5. **总结与建议**：最后给出凝练的总体研判，并视情况给出 1-3 条可操作建议或值得持续关注的点。

质量底线：
- 优先使用【综合洞察】中的结论作为骨架，再用证据充实细节，不要简单复述原文片段。
- 论点要有层次和递进，覆盖多角度，做到充分、全面、有条理。
- 涉及联网搜索结果时，引用处注明来源（站点或链接）。
- 内容要详实，但每一句都要有依据，宁可点明"现有信息不足以判断"也不要编造。`

// buildDeepChatMessages assembles the message list for the generation call.
// It enriches the system prompt with reasoning context and injects history.
func buildDeepChatMessages(state *PipelineState, systemBase string) []map[string]string {
	sys := systemBase

	if state.FinalContext != "" {
		sys += "\n\n" + state.FinalContext
	}
	if state.PageHint != "" {
		sys += "\n\n当前页面上下文：" + truncateStr(state.PageHint, 800)
	}
	// 仅当确实检索到内容时才追加深度作答指令（避免无依据时强行编造）
	if len(state.Observations) > 0 || len(state.KeyInsights) > 0 || len(state.WebResults) > 0 {
		sys += deepAnswerDirective
	}

	msgs := []map[string]string{{"role": "system", "content": sys}}

	// Inject conversation history (without the final user turn)
	const maxHistRunes = 6000
	history := state.History
	if len(history) > 24 {
		history = history[len(history)-24:]
	}
	for _, m := range history {
		role := m.Role
		content := truncateStr(m.Content, maxHistRunes)
		if role == "user" || role == "assistant" {
			msgs = append(msgs, map[string]string{"role": role, "content": content})
		}
	}

	// Append the current user question
	msgs = append(msgs, map[string]string{"role": "user", "content": state.UserQuestion})

	return msgs
}
