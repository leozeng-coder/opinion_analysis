package tagger

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"

	"opinion-analysis/src/service/tagger/pipeline"
)

// makeToolCallFn 构造支持工具调用的 LLM 调用函数：
// 默认走 OpenAI 兼容原生 tools/tool_calls 协议；当 useNative=false 或原生解析失败时，
// 降级为 prompt+JSON 兑底（把工具清单写进 system，让模型输出指定 JSON 指明调用）。
func (s *Service) makeToolCallFn(baseURL, apiKey, model string) pipeline.ToolCallFn {
	return func(ctx context.Context, msgs []pipeline.Message, specs []pipeline.ToolSpec) (pipeline.ToolCallResp, error) {
		// 先尝试原生 function calling
		resp, err := s.nativeToolCall(ctx, baseURL, apiKey, model, msgs, specs)
		if err == nil {
			return resp, nil
		}
		// 原生失败（模型不支持/返回异常）→ JSON 兑底
		return s.jsonFallbackToolCall(ctx, baseURL, apiKey, model, msgs, specs)
	}
}

// ---- 原生 function calling ----

// openAITool 是 OpenAI 兼容的 tools 数组元素。
type openAITool struct {
	Type     string `json:"type"`
	Function struct {
		Name        string         `json:"name"`
		Description string         `json:"description"`
		Parameters  map[string]any `json:"parameters"`
	} `json:"function"`
}

func specsToOpenAITools(specs []pipeline.ToolSpec) []openAITool {
	out := make([]openAITool, 0, len(specs))
	for _, sp := range specs {
		var t openAITool
		t.Type = "function"
		t.Function.Name = sp.Name
		t.Function.Description = sp.Description
		t.Function.Parameters = sp.Parameters
		out = append(out, t)
	}
	return out
}

// msgsToWire 把 pipeline.Message 转成线上 JSON（处理 tool_calls / tool 角色）。
func msgsToWire(msgs []pipeline.Message) []map[string]any {
	out := make([]map[string]any, 0, len(msgs))
	for _, m := range msgs {
		wm := map[string]any{"role": m.Role}
		// content 始终带上（tool/assistant 可能为空字符串，但字段需存在）
		wm["content"] = m.Content
		if len(m.ToolCalls) > 0 {
			tcs := make([]map[string]any, 0, len(m.ToolCalls))
			for _, tc := range m.ToolCalls {
				tcs = append(tcs, map[string]any{
					"id":   tc.ID,
					"type": "function",
					"function": map[string]any{
						"name":      tc.Name,
						"arguments": string(tc.Arguments),
					},
				})
			}
			wm["tool_calls"] = tcs
		}
		if m.ToolCallID != "" {
			wm["tool_call_id"] = m.ToolCallID
		}
		if m.Name != "" {
			wm["name"] = m.Name
		}
		out = append(out, wm)
	}
	return out
}

func (s *Service) nativeToolCall(ctx context.Context, baseURL, apiKey, model string, msgs []pipeline.Message, specs []pipeline.ToolSpec) (pipeline.ToolCallResp, error) {
	reqBody := map[string]any{
		"model":       model,
		"messages":    msgsToWire(msgs),
		"temperature": 0.3,
		"max_tokens":  1500,
	}
	if len(specs) > 0 {
		reqBody["tools"] = specsToOpenAITools(specs)
		reqBody["tool_choice"] = "auto"
	}
	payload, _ := json.Marshal(reqBody)

	url := strings.TrimRight(baseURL, "/") + "/chat/completions"
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, url, bytes.NewReader(payload))
	if err != nil {
		return pipeline.ToolCallResp{}, err
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+apiKey)

	resp, err := s.client.Do(req)
	if err != nil {
		return pipeline.ToolCallResp{}, err
	}
	defer resp.Body.Close()
	body, _ := io.ReadAll(resp.Body)
	if resp.StatusCode/100 != 2 {
		return pipeline.ToolCallResp{}, fmt.Errorf("llm status=%d body=%s", resp.StatusCode, truncate(string(body), 300))
	}

	var apiResp struct {
		Choices []struct {
			Message struct {
				Content   string `json:"content"`
				ToolCalls []struct {
					ID       string `json:"id"`
					Function struct {
						Name      string `json:"name"`
						Arguments string `json:"arguments"`
					} `json:"function"`
				} `json:"tool_calls"`
			} `json:"message"`
		} `json:"choices"`
	}
	if err := json.Unmarshal(body, &apiResp); err != nil {
		return pipeline.ToolCallResp{}, fmt.Errorf("decode tool call response: %w", err)
	}
	if len(apiResp.Choices) == 0 {
		return pipeline.ToolCallResp{}, fmt.Errorf("empty llm choices")
	}

	msg := apiResp.Choices[0].Message
	out := pipeline.ToolCallResp{Content: strings.TrimSpace(msg.Content)}
	for _, tc := range msg.ToolCalls {
		args := tc.Function.Arguments
		if strings.TrimSpace(args) == "" {
			args = "{}"
		}
		out.ToolCalls = append(out.ToolCalls, pipeline.ToolCall{
			ID:        tc.ID,
			Name:      tc.Function.Name,
			Arguments: json.RawMessage(args),
		})
	}
	return out, nil
}

// ---- JSON 兑底（模型不支持原生 tools 时）----

// jsonFallbackToolCall 不使用原生 tools 字段，而是把工具清单写进 system prompt，
// 要求模型输出形如 {"action":"tool"|"final","tool":"...","arguments":{...},"content":"..."} 的 JSON。
func (s *Service) jsonFallbackToolCall(ctx context.Context, baseURL, apiKey, model string, msgs []pipeline.Message, specs []pipeline.ToolSpec) (pipeline.ToolCallResp, error) {
	wire := msgsToWire(msgs)
	if len(specs) > 0 {
		// 在最前面插入工具清单与输出协议说明
		wire = append([]map[string]any{{
			"role":    "system",
			"content": buildFallbackToolPrompt(specs),
		}}, wire...)
	}

	reqBody := map[string]any{
		"model":       model,
		"messages":    wire,
		"temperature": 0.3,
		"max_tokens":  1500,
	}
	payload, _ := json.Marshal(reqBody)

	url := strings.TrimRight(baseURL, "/") + "/chat/completions"
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, url, bytes.NewReader(payload))
	if err != nil {
		return pipeline.ToolCallResp{}, err
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+apiKey)

	resp, err := s.client.Do(req)
	if err != nil {
		return pipeline.ToolCallResp{}, err
	}
	defer resp.Body.Close()
	body, _ := io.ReadAll(resp.Body)
	if resp.StatusCode/100 != 2 {
		return pipeline.ToolCallResp{}, fmt.Errorf("llm status=%d body=%s", resp.StatusCode, truncate(string(body), 300))
	}

	var apiResp struct {
		Choices []struct {
			Message struct {
				Content string `json:"content"`
			} `json:"message"`
		} `json:"choices"`
	}
	if err := json.Unmarshal(body, &apiResp); err != nil || len(apiResp.Choices) == 0 {
		return pipeline.ToolCallResp{}, fmt.Errorf("empty fallback response")
	}
	raw := strings.TrimSpace(apiResp.Choices[0].Message.Content)

	// 无工具或解析不出 JSON → 当作最终文本回答
	if len(specs) == 0 {
		return pipeline.ToolCallResp{Content: raw}, nil
	}
	return parseFallbackDecision(raw), nil
}

func buildFallbackToolPrompt(specs []pipeline.ToolSpec) string {
	var sb strings.Builder
	sb.WriteString("你可以调用以下工具来收集信息。请仅输出一个 JSON 对象（无其它文字）：\n")
	sb.WriteString("- 需要调用工具时：{\"action\":\"tool\",\"tool\":\"工具名\",\"arguments\":{...}}\n")
	sb.WriteString("- 信息已足够、直接作答时：{\"action\":\"final\"}\n\n可用工具：\n")
	for _, sp := range specs {
		paramJSON, _ := json.Marshal(sp.Parameters)
		sb.WriteString(fmt.Sprintf("- %s：%s\n  参数 schema：%s\n", sp.Name, sp.Description, string(paramJSON)))
	}
	sb.WriteString("\n只输出 JSON，不要解释。一次只调用一个工具。")
	return sb.String()
}

// fallbackDecision 兑底模式下模型输出的决策。
type fallbackDecision struct {
	Action    string          `json:"action"`
	Tool      string          `json:"tool"`
	Arguments json.RawMessage `json:"arguments"`
	Content   string          `json:"content"`
}

func parseFallbackDecision(raw string) pipeline.ToolCallResp {
	s := raw
	for _, fence := range []string{"```json", "```"} {
		if idx := strings.Index(s, fence); idx >= 0 {
			s = s[idx+len(fence):]
		}
	}
	if idx := strings.Index(s, "```"); idx >= 0 {
		s = s[:idx]
	}
	start := strings.IndexByte(s, '{')
	end := strings.LastIndexByte(s, '}')
	if start < 0 || end <= start {
		// 解析不出结构 → 当作最终回答
		return pipeline.ToolCallResp{Content: raw}
	}
	var d fallbackDecision
	if err := json.Unmarshal([]byte(s[start:end+1]), &d); err != nil {
		return pipeline.ToolCallResp{Content: raw}
	}
	if d.Action == "tool" && strings.TrimSpace(d.Tool) != "" {
		args := d.Arguments
		if len(strings.TrimSpace(string(args))) == 0 {
			args = json.RawMessage("{}")
		}
		return pipeline.ToolCallResp{
			ToolCalls: []pipeline.ToolCall{{Name: d.Tool, Arguments: args}},
		}
	}
	// final 或未知 → 结束循环（content 可空，由 GenerateNode 正式作答）
	return pipeline.ToolCallResp{Content: d.Content}
}

