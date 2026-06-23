package tagger

import (
	"bufio"
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
	// 当历史消息超过此轮数时，保留最近的消息 + 首轮摘要
	contextWindowTurns = 8
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
		"model":       model,
		"messages":    msgs,
		"temperature": 0.7,
		"max_tokens":  3000,
		"top_p":       0.9,
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

// ChatCompletionStream 流式调用大模型，通过 channel 返回增量内容。
// 调用方需要从 channel 读取直到关闭，或者 context 取消。
func (s *Service) ChatCompletionStream(ctx context.Context, history []ChatMessage, pageHint string, retrievalContext string) (<-chan string, <-chan error) {
	contentCh := make(chan string, 10)
	errCh := make(chan error, 1)

	go func() {
		defer close(contentCh)
		defer close(errCh)

		cfg, apiKey := s.snapshot()
		if strings.TrimSpace(apiKey) == "" {
			errCh <- fmt.Errorf("llm api key not configured")
			return
		}

		msgs := buildChatMessages(history, pageHint, retrievalContext)
		if len(msgs) == 0 {
			errCh <- fmt.Errorf("no messages")
			return
		}

		model := cfg.LLMModel
		if strings.TrimSpace(model) == "" {
			model = "deepseek-chat"
		}
		reqBody := map[string]any{
			"model":       model,
			"messages":    msgs,
			"temperature": 0.7,
			"max_tokens":  3000,
			"top_p":       0.9,
			"stream":      true,
		}
		payload, _ := json.Marshal(reqBody)

		baseURL := cfg.LLMBaseURL
		if strings.TrimSpace(baseURL) == "" {
			baseURL = "https://api.deepseek.com"
		}
		url := strings.TrimRight(baseURL, "/") + "/chat/completions"
		req, err := http.NewRequestWithContext(ctx, http.MethodPost, url, bytes.NewReader(payload))
		if err != nil {
			errCh <- err
			return
		}
		req.Header.Set("Content-Type", "application/json")
		req.Header.Set("Authorization", "Bearer "+apiKey)
		req.Header.Set("Accept", "text/event-stream")

		resp, err := s.client.Do(req)
		if err != nil {
			errCh <- err
			return
		}
		defer resp.Body.Close()

		if resp.StatusCode/100 != 2 {
			body, _ := io.ReadAll(resp.Body)
			errCh <- fmt.Errorf("llm status=%d body=%s", resp.StatusCode, truncate(string(body), 400))
			return
		}

		// 解析 SSE 流
		scanner := bufio.NewScanner(resp.Body)
		for scanner.Scan() {
			line := strings.TrimSpace(scanner.Text())

			// 跳过空行
			if line == "" {
				continue
			}

			// 跳过注释行
			if strings.HasPrefix(line, ":") {
				continue
			}

			// 解析 data: 行
			if !strings.HasPrefix(line, "data: ") {
				continue
			}

			data := strings.TrimPrefix(line, "data: ")
			if data == "[DONE]" {
				break
			}

			var chunk struct {
				Choices []struct {
					Delta struct {
						Content string `json:"content"`
					} `json:"delta"`
					FinishReason *string `json:"finish_reason"`
				} `json:"choices"`
			}
			if err := json.Unmarshal([]byte(data), &chunk); err != nil {
				// 解析失败，记录但继续
				continue
			}

			if len(chunk.Choices) > 0 {
				content := chunk.Choices[0].Delta.Content
				if content != "" {
					select {
					case contentCh <- content:
					case <-ctx.Done():
						return
					}
				}

				// 检查是否结束
				if chunk.Choices[0].FinishReason != nil {
					break
				}
			}
		}

		if err := scanner.Err(); err != nil {
			errCh <- err
		}
	}()

	return contentCh, errCh
}

func buildChatMessages(history []ChatMessage, pageHint string, retrievalContext string) []map[string]string {
	sys := `你是「舆情分析系统」内置的智能助手，专注于解读舆情数据、分析公众情绪和提供决策建议。

## 核心原则
1. **优先引用知识库**：每个事实必须来自下方检索结果
2. **不知道就说不知道**：信息缺失时坦诚告知，严禁推测、编造、基于常识否定
3. **自然语言引用**：用"根据小红书上的分析..."而非"【#1】(id=123)"
4. **禁止技术标记**：不要复制【#序号】、【W数字】、(id=数字)、【综合洞察】等内部标记

## 引用示例
❌ 错误：根据【#1】(id=4421)，S2赛季暗改了13项...
✓ 正确：根据小红书上一篇关于S2赛季的分析，官方暗改了13项美术表现。

❌ 错误："S1赛季没有主题名。"（否定性推测）
✓ 正确："知识库中关于S1赛季主题的记载较少，建议补充资料。"（坦诚缺失）

## 多维度分析（适用于评论数据）
根据具体问题选择相关维度：舆论倾向、情绪强度、争议焦点、关键诉求、传播风险

## 对话策略
- 意图不明时主动澄清
- 先给核心答案，再展开解释
- 遇到复杂问题展示推理步骤
- 保持专业但友好的语气`

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
		sys += "\n\n【知识库检索摘录】（来自本地舆情向量库，包含文章正文和用户评论，条目以 --- 分隔）\n" + rc
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
	// 硬截断：超过最大轮数时，只保留最近的消息
	if len(in) > maxChatUserTurns*2 {
		in = in[len(in)-maxChatUserTurns*2:]
	}

	// 智能压缩：如果消息数超过上下文窗口，生成早期对话摘要
	if len(in) > contextWindowTurns*2 {
		// 保留最近 contextWindowTurns 轮对话
		recentMsgs := in[len(in)-contextWindowTurns*2:]
		// 早期消息生成摘要
		earlyMsgs := in[:len(in)-contextWindowTurns*2]
		summary := summarizeEarlyContext(earlyMsgs)

		// 将摘要作为第一条消息插入
		result := []ChatMessage{{Role: "assistant", Content: summary}}
		result = append(result, recentMsgs...)
		return result
	}

	return in
}

// summarizeEarlyContext 将早期对话压缩为摘要
func summarizeEarlyContext(msgs []ChatMessage) string {
	if len(msgs) == 0 {
		return ""
	}

	var topics []string
	var lastUserQ string

	for _, m := range msgs {
		role := strings.ToLower(strings.TrimSpace(m.Role))
		content := strings.TrimSpace(m.Content)
		if content == "" {
			continue
		}

		if role == "user" {
			// 提取用户问题的关键词（简单实现：取前50字符）
			r := []rune(content)
			if len(r) > 50 {
				lastUserQ = string(r[:50]) + "..."
			} else {
				lastUserQ = content
			}
			if lastUserQ != "" {
				topics = append(topics, lastUserQ)
			}
		}
	}

	if len(topics) == 0 {
		return "[早期对话摘要：用户进行了一些初步咨询]"
	}

	// 限制摘要中的话题数量
	if len(topics) > 3 {
		topics = topics[:3]
	}

	return fmt.Sprintf("[早期对话摘要：用户询问了以下话题：%s]", strings.Join(topics, "；"))
}
