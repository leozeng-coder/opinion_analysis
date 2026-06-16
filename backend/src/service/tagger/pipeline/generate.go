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
你已经过多轮检索与综合推理，请基于上方【用户需求洞察】【综合洞察】【推理过程】【知识库证据】给出有深度的回答：
1. 紧扣【用户需求洞察】，回答用户真正关心的问题，并尽量覆盖其中建议的分析维度
2. 先用一段话给出直接结论
3. 再分点展开论证，每个论点都用知识库证据中的具体内容或数据支撑，标注来源倾向（文章/评论）
4. 主动指出信息中的分歧、矛盾或趋势变化，不要回避不确定性
5. 最后给出一句话的总结或可操作建议
回答要充分、有条理、多角度，优先使用【综合洞察】中的结论，不要简单复述原文。`

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
	if len(state.Observations) > 0 || len(state.KeyInsights) > 0 {
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
