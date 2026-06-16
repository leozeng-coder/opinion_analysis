package pipeline

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
)

const intentPrompt = `你是一个意图分析助手。给定用户问题，输出 JSON（无其它文字）：
{
  "intent": "一句话描述用户意图（中文）",
  "entities": ["关键实体1", "关键实体2"],
  "need_retrieval": true,
  "retrieval_query": "用于知识库检索的最优查询词",
  "sub_queries": ["子查询1", "子查询2"],
  "initial_topk": 8
}

need_retrieval 为 false 的情况：问候、系统帮助类、纯数学运算等无需参考本地舆情数据的问题。
retrieval_query 应提炼出最能命中相关文章/评论的核心词，不超过 40 字。
sub_queries 把用户问题拆解为 2-3 个互补的检索角度（如对比类问题拆成各方、不同维度），
每个子查询独立可检索、不超过 30 字；若问题很简单，可只给 1 个或留空数组。
initial_topk 是每个检索角度初始召回的条数，请根据问题复杂度评估，取值 4~12：
简单/具体的问题取 4-6，宽泛/需要全面背景的问题取 8-12。
只输出 JSON，不要任何解释。`

// IntentNode calls a lightweight LLM to classify the user's intent
// and decide whether RAG retrieval is needed.
type IntentNode struct {
	llmCall LLMCallFn
}

// LLMCallFn is a dependency-injected function that calls the LLM (non-streaming).
// This keeps the node decoupled from the tagger.Service concrete type.
type LLMCallFn func(ctx context.Context, msgs []map[string]string) (string, error)

func NewIntentNode(llmCall LLMCallFn) *IntentNode {
	return &IntentNode{llmCall: llmCall}
}

func (n *IntentNode) Name() string  { return "intent" }
func (n *IntentNode) Title() string { return "意图分析" }

func (n *IntentNode) Execute(ctx context.Context, state *PipelineState, emit EmitFn) error {
	emit(ThinkStep{Step: n.Name(), Title: n.Title(), Status: StatusRunning})

	msgs := []map[string]string{
		{"role": "system", "content": intentPrompt},
		{"role": "user", "content": state.UserQuestion},
	}

	raw, err := n.llmCall(ctx, msgs)
	if err != nil {
		emit(ThinkStep{Step: n.Name(), Title: n.Title(), Content: "意图分析失败，使用默认检索策略", Status: StatusDone})
		// Graceful fallback: assume retrieval needed with original question
		state.NeedRetrieval = true
		state.RetrievalQuery = state.UserQuestion
		state.SubQueries = []string{state.UserQuestion}
		state.InitialTopK = 8
		state.Intent = "用户问题（意图分析失败）"
		return nil
	}

	result, err := ParseIntent(raw)
	if err != nil {
		// If parsing fails, fall back gracefully
		emit(ThinkStep{Step: n.Name(), Title: n.Title(), Content: "意图解析异常，使用原始问题检索", Status: StatusDone})
		state.NeedRetrieval = true
		state.RetrievalQuery = state.UserQuestion
		state.SubQueries = []string{state.UserQuestion}
		state.InitialTopK = 8
		state.Intent = "用户问题"
		return nil
	}

	state.Intent = result.Intent
	state.Entities = result.Entities
	state.NeedRetrieval = result.NeedRetrieval
	state.RetrievalQuery = result.RetrievalQuery
	if state.RetrievalQuery == "" {
		state.RetrievalQuery = state.UserQuestion
	}
	// 过滤空白子查询；为空时回退到主检索词
	state.SubQueries = state.SubQueries[:0]
	for _, q := range result.SubQueries {
		if s := strings.TrimSpace(q); s != "" {
			state.SubQueries = append(state.SubQueries, s)
		}
	}
	if len(state.SubQueries) == 0 {
		state.SubQueries = []string{state.RetrievalQuery}
	}
	// 大模型评估的初始检索量，限制在 [4,12]，异常时回退默认 8
	state.InitialTopK = clampTopK(result.InitialTopK)

	summary := state.Intent
	if len(state.Entities) > 0 {
		summary += "，关键词：" + strings.Join(state.Entities, "、")
	}
	if len(state.SubQueries) > 0 {
		summary += "\n检索角度：" + strings.Join(state.SubQueries, " / ")
	}
	summary += fmt.Sprintf("\n初始检索量：每个角度 %d 条", state.InitialTopK)
	emit(ThinkStep{Step: n.Name(), Title: n.Title(), Content: summary, Status: StatusDone})
	return nil
}

// clampTopK 把大模型评估的初始检索量限制到 [4,12]，0 或越界时回退到默认 8。
func clampTopK(v int) int {
	if v < 4 {
		if v <= 0 {
			return 8
		}
		return 4
	}
	if v > 12 {
		return 12
	}
	return v
}

// NewLightLLMCall creates an LLMCallFn with a compact token budget (512).
// Suitable for intent analysis and other short-output tasks.
func NewLightLLMCall(baseURL, apiKey, model string, httpClient *http.Client) LLMCallFn {
	return newLLMCall(baseURL, apiKey, model, httpClient, 512)
}

// NewFilterLLMCall creates an LLMCallFn with a larger token budget (1500).
// Used by FilterNode which returns a JSON array over all candidate articles.
func NewFilterLLMCall(baseURL, apiKey, model string, httpClient *http.Client) LLMCallFn {
	return newLLMCall(baseURL, apiKey, model, httpClient, 1500)
}

// newLLMCall is the shared implementation behind the public constructors.
func newLLMCall(baseURL, apiKey, model string, httpClient *http.Client, maxTokens int) LLMCallFn {
	return func(ctx context.Context, msgs []map[string]string) (string, error) {
		if strings.TrimSpace(apiKey) == "" {
			return "", fmt.Errorf("llm api key not configured")
		}
		if strings.TrimSpace(model) == "" {
			model = "deepseek-chat"
		}

		reqBody := map[string]any{
			"model":       model,
			"messages":    msgs,
			"temperature": 0.3,
			"max_tokens":  maxTokens,
		}
		payload, _ := json.Marshal(reqBody)

		url := strings.TrimRight(baseURL, "/") + "/chat/completions"
		req, err := http.NewRequestWithContext(ctx, http.MethodPost, url, bytes.NewReader(payload))
		if err != nil {
			return "", err
		}
		req.Header.Set("Content-Type", "application/json")
		req.Header.Set("Authorization", "Bearer "+apiKey)

		resp, err := httpClient.Do(req)
		if err != nil {
			return "", err
		}
		defer resp.Body.Close()

		body, _ := io.ReadAll(resp.Body)
		if resp.StatusCode/100 != 2 {
			return "", fmt.Errorf("llm status=%d", resp.StatusCode)
		}

		var apiResp struct {
			Choices []struct {
				Message struct {
					Content string `json:"content"`
				} `json:"message"`
			} `json:"choices"`
		}
		if err := json.Unmarshal(body, &apiResp); err != nil || len(apiResp.Choices) == 0 {
			return "", fmt.Errorf("empty llm response")
		}
		return strings.TrimSpace(apiResp.Choices[0].Message.Content), nil
	}
}
