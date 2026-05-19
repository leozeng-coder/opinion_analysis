package tagger

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"unicode/utf8"
)

// ChatMessage 单条对话（OpenAI 兼容 role）。
type ChatMessage struct {
	Role    string `json:"role"`
	Content string `json:"content"`
}

const (
	maxChatUserTurns     = 12
	maxChatContentRunes  = 6000
	systemChatHintRunes  = 800
	maxRetrievalContextR = 12000
)

// ChatCompletion 调用与打标相同的大模型配置，进行多轮对话（非流式）。
// retrievalContext 为 RAG 检索得到的摘录文本，会并入 system（限长）。
func (s *Service) ChatCompletion(ctx context.Context, history []ChatMessage, pageHint string, retrievalContext string) (reply string, err error) {
	cfg, apiKey := s.snapshot()
	if strings.TrimSpace(apiKey) == "" {
		return "", fmt.Errorf("llm api key not configured")
	}

	msgs := buildChatMessages(history, pageHint, retrievalContext)
	if len(msgs) == 0 {
		return "", fmt.Errorf("no messages")
	}

	model := cfg.LLMModel
	if strings.TrimSpace(model) == "" {
		model = "deepseek-chat"
	}
	reqBody := map[string]any{
		"model":    model,
		"messages": msgs,
		"temperature": 0.4,
		"max_tokens":  2000,
	}
	payload, _ := json.Marshal(reqBody)

	baseURL := cfg.LLMBaseURL
	if strings.TrimSpace(baseURL) == "" {
		baseURL = "https://api.deepseek.com"
	}
	url := strings.TrimRight(baseURL, "/") + "/chat/completions"
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, url, bytes.NewReader(payload))
	if err != nil {
		return "", err
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+apiKey)

	resp, err := s.client.Do(req)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()
	respBody, _ := io.ReadAll(resp.Body)
	if resp.StatusCode/100 != 2 {
		return "", fmt.Errorf("llm status=%d body=%s", resp.StatusCode, truncate(string(respBody), 400))
	}

	var apiResp struct {
		Choices []struct {
			Message struct {
				Content string `json:"content"`
			} `json:"message"`
		} `json:"choices"`
	}
	if err := json.Unmarshal(respBody, &apiResp); err != nil {
		return "", fmt.Errorf("decode llm response: %w", err)
	}
	if len(apiResp.Choices) == 0 {
		return "", fmt.Errorf("empty llm choices")
	}
	return strings.TrimSpace(apiResp.Choices[0].Message.Content), nil
}

func buildChatMessages(history []ChatMessage, pageHint string, retrievalContext string) []map[string]string {
	sys := `你是「舆情分析系统」内置的智能助手。用户正在使用一个面向新闻爬虫、热点话题、情感与预警的舆情后台。
请用简洁、专业、可执行的中文回答，不要编造系统中不存在的数据；若需要具体数字，请引导用户到对应页面查看或自行根据用户给出的摘要分析。
若下方提供了「知识库检索摘录」，请优先基于这些摘录回答事实性问题；摘录不足以判断时请明确说明。
禁止输出有害、违法内容。回答不要太长，除非用户明确要求详细说明。`

	if h := strings.TrimSpace(pageHint); h != "" {
		if utf8.RuneCountInString(h) > systemChatHintRunes {
			h = string([]rune(h)[:systemChatHintRunes]) + "…"
		}
		sys += "\n\n当前页面上下文：" + h
	}

	if rc := strings.TrimSpace(retrievalContext); rc != "" {
		if utf8.RuneCountInString(rc) > maxRetrievalContextR {
			rc = string([]rune(rc)[:maxRetrievalContextR]) + "…"
		}
		sys += "\n\n【知识库检索摘录】（来自本地舆情向量库，条目以 --- 分隔）\n" + rc
	}

	out := []map[string]string{{"role": "system", "content": sys}}

	trimmed := normalizeHistory(history)
	for _, m := range trimmed {
		role := strings.ToLower(strings.TrimSpace(m.Role))
		if role != "user" && role != "assistant" {
			continue
		}
		c := strings.TrimSpace(m.Content)
		if c == "" {
			continue
		}
		if utf8.RuneCountInString(c) > maxChatContentRunes {
			c = string([]rune(c)[:maxChatContentRunes]) + "…"
		}
		out = append(out, map[string]string{"role": role, "content": c})
	}
	return out
}

func normalizeHistory(in []ChatMessage) []ChatMessage {
	if len(in) == 0 {
		return nil
	}
	if len(in) > maxChatUserTurns*2 {
		in = in[len(in)-maxChatUserTurns*2:]
	}
	return in
}
