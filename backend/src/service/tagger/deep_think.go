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

	"opinion-analysis/src/service/rag"
	"opinion-analysis/src/service/tagger/pipeline"
)

// baseSystemPrompt is the same prompt used by the normal chat endpoint.
// It is inlined here so the generate node can enrich it with reasoning context.
const baseSystemPrompt = `你是「舆情分析系统」内置的智能助手，专注于帮助用户理解和分析舆情数据。

## 核心能力
- 解读舆情趋势、热点话题、情感分析结果
- 基于知识库检索提供数据支持的回答（知识库包含文章正文和用户评论）
- 从评论中提炼公众情绪、观点倾向、争议焦点
- 协助用户理解系统功能和数据含义
- 提供可操作的分析建议

## 多维度分析框架
当知识库检索结果中包含「用户评论」时，请从以下维度综合分析：
1. **舆论倾向**：评论整体偏正面/负面/中性，主流观点是什么
2. **情绪强度**：评论者的情绪激烈程度，是否有极端言论
3. **争议焦点**：评论中的分歧点、对立观点
4. **关键诉求**：用户反复提及的需求或不满
5. **传播风险**：是否有可能引发更大范围讨论的敏感点

不需要每次都覆盖所有维度，根据用户问题和数据特征选择最相关的角度。

## 对话策略
1. 先给出核心答案，再展开解释，包含背景信息、数据解读、实际意义
2. 优先使用【推理摘要】和【知识库检索摘录】中的事实
3. 明确区分"系统中的实际数据"和"基于经验的分析建议"
4. 遇到复杂问题时，展示推理步骤

## 限制
- 不编造系统中不存在的具体数据
- 不输出有害、违法内容`

// DeepChatResult is returned by RunDeepPipeline.
type DeepChatResult struct {
	ContentCh <-chan string    // streams the final LLM tokens
	StepCh    <-chan pipeline.ThinkStep // streams think_step events
	ErrCh     <-chan error
}

// RunDeepPipeline assembles and executes the deep-thinking pipeline.
// It returns channels that the HTTP handler consumes to write SSE events.
func (s *Service) RunDeepPipeline(
	ctx context.Context,
	history []ChatMessage,
	userQuestion string,
	pageHint string,
	topics []string,
	ragClient *rag.Client,
) DeepChatResult {
	contentCh := make(chan string, 20)
	stepCh := make(chan pipeline.ThinkStep, 16)
	errCh := make(chan error, 1)

	go func() {
		defer close(contentCh)
		defer close(stepCh)
		defer close(errCh)

		cfg, apiKey := s.snapshot()
		if strings.TrimSpace(apiKey) == "" {
			errCh <- fmt.Errorf("llm api key not configured")
			return
		}

		baseURL := cfg.LLMBaseURL
		if strings.TrimSpace(baseURL) == "" {
			baseURL = "https://api.deepseek.com"
		}
		model := cfg.LLMModel
		if strings.TrimSpace(model) == "" {
			model = "deepseek-chat"
		}

		// Build dependency-injected functions
		lightLLM := pipeline.NewLightLLMCall(baseURL, apiKey, model, s.client)
		filterLLM := pipeline.NewFilterLLMCall(baseURL, apiKey, model, s.client)

		var ragSearch pipeline.RAGSearchFn
		if ragClient != nil {
			ragSearch = func(ctx context.Context, query string, topK int, t []string) ([]string, error) {
				chunks, err := ragClient.Search(ctx, query, topK, t)
				if err != nil {
					return nil, err
				}
				// Convert []rag.Chunk to []string (formatted text)
				out := make([]string, 0, len(chunks))
				for _, ch := range chunks {
					out = append(out, formatChunk(ch))
				}
				return out, nil
			}
		}

		streamFn := s.makePipelineStreamFn(baseURL, apiKey, model)

		// Convert history type
		hist := make([]pipeline.ChatMessage, len(history))
		for i, m := range history {
			hist[i] = pipeline.ChatMessage{Role: m.Role, Content: m.Content}
		}

		state := &pipeline.PipelineState{
			History:      hist,
			UserQuestion: userQuestion,
			PageHint:     pageHint,
			Topics:       topics,
		}

		emit := func(step pipeline.ThinkStep) {
			select {
			case stepCh <- step:
			case <-ctx.Done():
			}
		}

		p := pipeline.NewPipeline(
			pipeline.NewIntentNode(lightLLM),
			pipeline.NewReActNode(ragSearch, filterLLM),
			pipeline.NewSynthesizeNode(filterLLM),
			pipeline.NewGenerateNode(streamFn, contentCh, baseSystemPrompt),
		)

		if err := p.Run(ctx, state, emit); err != nil {
			if ctx.Err() == nil {
				errCh <- err
			}
		}
	}()

	return DeepChatResult{ContentCh: contentCh, StepCh: stepCh, ErrCh: errCh}
}

// makePipelineStreamFn creates the StreamFn used by the generate node.
// It reuses the same SSE-parsing logic as ChatCompletionStream but accepts
// a pre-built message slice (no extra buildChatMessages wrapping).
func (s *Service) makePipelineStreamFn(baseURL, apiKey, model string) pipeline.StreamFn {
	return func(ctx context.Context, msgs []map[string]string) (<-chan string, <-chan error) {
		contentCh := make(chan string, 10)
		errCh := make(chan error, 1)

		go func() {
			defer close(contentCh)
			defer close(errCh)

			reqBody := map[string]any{
				"model":       model,
				"messages":    msgs,
				"temperature": 0.6,
				"max_tokens":  4000,
				"top_p":       0.9,
				"stream":      true,
			}
			payload, _ := json.Marshal(reqBody)

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

			scanner := bufio.NewScanner(resp.Body)
			for scanner.Scan() {
				line := strings.TrimSpace(scanner.Text())
				if line == "" || strings.HasPrefix(line, ":") || !strings.HasPrefix(line, "data: ") {
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
					continue
				}
				if len(chunk.Choices) > 0 {
					if c := chunk.Choices[0].Delta.Content; c != "" {
						select {
						case contentCh <- c:
						case <-ctx.Done():
							return
						}
					}
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
}

// formatChunk converts a rag.Chunk to a plain-text string for the pipeline nodes.
func formatChunk(ch rag.Chunk) string {
	chType := ch.ChunkType
	if chType == "" {
		chType = "content"
	}
	label := "正文"
	if chType == "comment" {
		label = "用户评论"
	}
	return fmt.Sprintf("[id=%d platform=%s type=%s score=%.4f src=%s]\n标题：%s\n%s：%s",
		ch.ArticleID, ch.Platform, label, ch.Score, ch.Source, ch.Title, label, ch.Snippet)
}
