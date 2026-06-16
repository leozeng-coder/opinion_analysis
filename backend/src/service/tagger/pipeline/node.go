package pipeline

import (
	"context"
	"encoding/json"
)

// StepStatus represents the current state of a pipeline step.
type StepStatus string

const (
	StatusRunning StepStatus = "running"
	StatusDone    StepStatus = "done"
	StatusSkipped StepStatus = "skipped"
	StatusError   StepStatus = "error"
)

// ThinkStep is an SSE event emitted by a node to the client.
type ThinkStep struct {
	Step    string     `json:"step"`
	Title   string     `json:"title"`
	Content string     `json:"content,omitempty"`
	Status  StepStatus `json:"status"`
}

// EmitFn is the function nodes call to push a ThinkStep event over SSE.
type EmitFn func(step ThinkStep)

// PipelineState is the shared bag of state passed between nodes.
// Nodes read from earlier fields and write to later fields.
type PipelineState struct {
	// Input (set before pipeline runs)
	History      []ChatMessage
	UserQuestion string
	PageHint     string
	Topics       []string

	// Node 1 output – intent analysis
	Intent         string
	Entities       []string
	NeedRetrieval  bool
	RetrievalQuery string
	SubQueries     []string // 拆解出的子查询，作为 ReAct 首轮检索词
	InitialTopK    int      // 大模型评估的初始检索量（每个子查询召回条数，4~12）

	// ReAct loop accumulation
	// Observations 跨轮累积、按 article 去重的检索观察（每条含原文与提炼摘要）。
	Observations []Observation
	// ReasoningChain 记录每一轮 Reason 的思考，供综合与展示。
	ReasoningChain []string

	// 兼容字段：单轮检索结果（ReAct 内部每轮临时填充）
	RetrievedChunks []string

	// Synthesize 节点输出
	KeyFacts         []string // 兼容旧字段
	KeyInsights      []string // 综合推理得出的关键洞察
	UserNeed         string   // 对用户真实需求/意图的洞察（深层想了解什么）
	AnswerDimensions []string // 建议回答覆盖的分析维度

	// Final assembled context injected into the generation system prompt
	FinalContext string
}

// Observation 是 ReAct 循环中一条去重后的检索观察。
type Observation struct {
	ArticleID int    // 来源文章 ID，用于跨轮去重（0 表示未知）
	Chunk     string // 格式化后的原文片段
	Extract   string // LLM 提炼的与问题相关的摘要
	Score     int    // 相关性打分 1-5
	Round     int    // 第几轮检索得到
}

// ChatMessage mirrors tagger.ChatMessage to avoid a circular import.
type ChatMessage struct {
	Role    string
	Content string
}

// IntentResult is the structured JSON produced by Node 1.
type IntentResult struct {
	Intent         string   `json:"intent"`
	Entities       []string `json:"entities"`
	NeedRetrieval  bool     `json:"need_retrieval"`
	RetrievalQuery string   `json:"retrieval_query"`
	SubQueries     []string `json:"sub_queries"`
	InitialTopK    int      `json:"initial_topk"`
}

// ParseIntent attempts to unmarshal the LLM text as IntentResult.
// It is tolerant of markdown code fences the model may emit.
func ParseIntent(raw string) (IntentResult, error) {
	// strip possible ```json ... ``` wrappers
	cleaned := raw
	for _, fence := range []string{"```json", "```"} {
		if idx := indexOf(cleaned, fence); idx >= 0 {
			cleaned = cleaned[idx+len(fence):]
		}
	}
	if idx := indexOf(cleaned, "```"); idx >= 0 {
		cleaned = cleaned[:idx]
	}

	var r IntentResult
	if err := json.Unmarshal([]byte(cleaned), &r); err != nil {
		return r, err
	}
	return r, nil
}

func indexOf(s, sub string) int {
	for i := 0; i <= len(s)-len(sub); i++ {
		if s[i:i+len(sub)] == sub {
			return i
		}
	}
	return -1
}

// Node is the interface every pipeline step must implement.
// Execute reads from state, does work (possibly calling LLM or RAG),
// emits ThinkStep events via emit, and writes results back to state.
type Node interface {
	// Name returns the short identifier used in ThinkStep.step.
	Name() string
	// Title returns the human-readable label shown in the UI.
	Title() string
	// Execute runs the node. It may emit 0..N ThinkStep events.
	// Returning a non-nil error causes the pipeline to abort.
	Execute(ctx context.Context, state *PipelineState, emit EmitFn) error
}

// Pipeline is a linear chain of Nodes. Nodes are executed in order.
// Adding or removing a node requires only a single-line change here.
type Pipeline struct {
	nodes []Node
}

// NewPipeline creates a pipeline with the provided nodes.
func NewPipeline(nodes ...Node) *Pipeline {
	return &Pipeline{nodes: nodes}
}

// Run executes every node in order, passing state and emit along.
// Execution stops on the first error.
func (p *Pipeline) Run(ctx context.Context, state *PipelineState, emit EmitFn) error {
	for _, n := range p.nodes {
		if err := ctx.Err(); err != nil {
			return err
		}
		if err := n.Execute(ctx, state, emit); err != nil {
			return err
		}
	}
	return nil
}
