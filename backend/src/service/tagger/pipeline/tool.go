package pipeline

import (
	"context"
	"encoding/json"
	"fmt"
	"sync"
)

// Tool 是 Agent 可调用的一个工具。新增工具只需实现此接口并注册到 Registry，
// 无需改动 AgentNode。Name 必须全局唯一。
type Tool interface {
	// Name 工具唯一标识，如 "web_search"。
	Name() string
	// Description 给 LLM 看的用途描述，决定 LLM 何时选择调用本工具。
	Description() string
	// Parameters 返回入参的 JSON Schema（OpenAI function parameters 格式）。
	Parameters() map[string]any
	// Invoke 执行工具。args 为 LLM 给出的 JSON 入参；返回结果统一回灌给 LLM，
	// 并可携带结构化产出（Observations / WebResults）沉淀到 PipelineState。
	Invoke(ctx context.Context, args json.RawMessage) (ToolResult, error)
}

// ToolResult 是工具执行结果。
type ToolResult struct {
	// Content 回灌给 LLM 的文本（作为 role=tool 消息）。
	Content string
	// Observations 本地检索类工具产出的去重观察，供 Synthesize/Generate 复用。
	Observations []Observation
	// WebResults 联网搜索类工具产出的结果。
	WebResults []WebResult
	// Display 给前端思考链展示的简短摘要（emit 进 ThinkStep）；为空时由 AgentNode 兜底生成。
	Display string
}

// ToolSpec 是注册表导出的工具描述，用于组装 LLM 的 tools 数组或 prompt 清单。
type ToolSpec struct {
	Name        string         `json:"name"`
	Description string         `json:"description"`
	Parameters  map[string]any `json:"parameters"`
}

// ToolCall 是 LLM 决定发起的一次工具调用（原生 tool_calls，或 JSON 兑底解析得到）。
type ToolCall struct {
	ID        string          `json:"id"`        // 原生模式下的 tool_call_id，用于回灌对应；兑底模式可为空
	Name      string          `json:"name"`      // 工具名
	Arguments json.RawMessage `json:"arguments"` // 工具入参（JSON）
}

// Message 是工具调用循环中传递的一条消息（OpenAI 兼容，扩展 tool 角色字段）。
type Message struct {
	Role       string     `json:"role"`                   // system / user / assistant / tool
	Content    string     `json:"content"`                // 文本内容
	ToolCalls  []ToolCall `json:"tool_calls,omitempty"`   // assistant 发起的工具调用
	ToolCallID string     `json:"tool_call_id,omitempty"` // role=tool 时对应的调用 ID
	Name       string     `json:"name,omitempty"`         // role=tool 时的工具名
}

// ToolCallResp 是一次 LLM 调用的结果：要么给出文本（结束），要么发起工具调用。
type ToolCallResp struct {
	Content   string     // LLM 文本输出（无工具调用时即其阶段性结论）
	ToolCalls []ToolCall // 原生 tool_calls；JSON 兑底模式下解析得到
}

// ToolCallFn 是支持工具调用的 LLM 调用（非流式）。
// specs 为本轮可用工具清单；实现可选择原生 tools 协议或 prompt+JSON 兑底。
type ToolCallFn func(ctx context.Context, msgs []Message, specs []ToolSpec) (ToolCallResp, error)

// Registry 持有所有可用工具，按 name 索引，并发安全。
type Registry struct {
	mu    sync.RWMutex
	order []string
	tools map[string]Tool
}

// NewRegistry 创建空注册表。
func NewRegistry() *Registry {
	return &Registry{tools: make(map[string]Tool)}
}

// Register 注册一个工具；重名会被覆盖（后注册者生效）。
func (r *Registry) Register(t Tool) {
	if t == nil {
		return
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	name := t.Name()
	if _, exists := r.tools[name]; !exists {
		r.order = append(r.order, name)
	}
	r.tools[name] = t
}

// Len 返回已注册工具数。
func (r *Registry) Len() int {
	r.mu.RLock()
	defer r.mu.RUnlock()
	return len(r.tools)
}

// Specs 按注册顺序返回所有工具的描述，供组装 LLM 请求。
func (r *Registry) Specs() []ToolSpec {
	r.mu.RLock()
	defer r.mu.RUnlock()
	specs := make([]ToolSpec, 0, len(r.order))
	for _, name := range r.order {
		t := r.tools[name]
		specs = append(specs, ToolSpec{
			Name:        t.Name(),
			Description: t.Description(),
			Parameters:  t.Parameters(),
		})
	}
	return specs
}

// Invoke 按 name 执行工具；工具不存在时返回错误。
func (r *Registry) Invoke(ctx context.Context, name string, args json.RawMessage) (ToolResult, error) {
	r.mu.RLock()
	t, ok := r.tools[name]
	r.mu.RUnlock()
	if !ok {
		return ToolResult{}, fmt.Errorf("unknown tool: %s", name)
	}
	return t.Invoke(ctx, args)
}
