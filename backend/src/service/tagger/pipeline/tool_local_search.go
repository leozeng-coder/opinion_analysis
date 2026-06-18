package pipeline

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
)

// LocalSearchTool 把本地舆情知识库检索包装成一个 Agent 工具。
// 它复用现有 RAGSearchFn 做检索、scoreChunks 做相关性打分与提炼，
// 内部按 article id 去重，产出 []Observation 沉淀给后续综合/生成节点。
type LocalSearchTool struct {
	search   RAGSearchFn // 注入的向量检索函数
	llmCall  LLMCallFn   // 打分用 LLM
	topics   []string    // 话题过滤（来自请求）
	intent   string      // 用户意图（用于打分上下文）
	question string      // 用户原始问题
	minScore int         // 保留门槛
	// seen 跨工具多次调用去重已纳入的 article id。
	seen map[int]struct{}
}

// NewLocalSearchTool 创建本地检索工具。seen 在整个 Agent 会话内共享以跨轮去重。
func NewLocalSearchTool(search RAGSearchFn, llmCall LLMCallFn, topics []string, intent, question string, seen map[int]struct{}) *LocalSearchTool {
	if seen == nil {
		seen = make(map[int]struct{})
	}
	return &LocalSearchTool{
		search:   search,
		llmCall:  llmCall,
		topics:   topics,
		intent:   intent,
		question: question,
		minScore: 3,
		seen:     seen,
	}
}

func (t *LocalSearchTool) Name() string { return "search_local_knowledge" }

func (t *LocalSearchTool) Description() string {
	return "检索本系统内部的舆情知识库（包含各社交平台抓取的文章正文与用户评论）。" +
		"当用户的问题涉及本系统已采集的舆情数据、话题趋势、公众评论、情感倾向时，使用本工具。" +
		"可多次调用以补充不同检索角度。"
}

func (t *LocalSearchTool) Parameters() map[string]any {
	return map[string]any{
		"type": "object",
		"properties": map[string]any{
			"query": map[string]any{
				"type":        "string",
				"description": "检索查询词，提炼出最能命中相关文章/评论的核心词，不超过 40 字",
			},
			"top_k": map[string]any{
				"type":        "integer",
				"description": "召回条数，4~12，简单问题取小、宽泛问题取大",
			},
		},
		"required": []string{"query"},
	}
}

type localSearchArgs struct {
	Query string `json:"query"`
	TopK  int    `json:"top_k"`
}

func (t *LocalSearchTool) Invoke(ctx context.Context, raw json.RawMessage) (ToolResult, error) {
	if t.search == nil {
		return ToolResult{Content: "本地知识库不可用。", Display: "本地知识库未启用"}, nil
	}
	var args localSearchArgs
	_ = json.Unmarshal(raw, &args)
	query := strings.TrimSpace(args.Query)
	if query == "" {
		query = t.question
	}
	topK := args.TopK
	if topK < 4 {
		topK = 6
	} else if topK > 12 {
		topK = 12
	}

	chunks, err := t.search(ctx, query, topK, t.topics)
	if err != nil {
		return ToolResult{
			Content: fmt.Sprintf("检索「%s」失败：%v。请尝试其他方式或基于已有信息回答。", query, err),
			Display: "本地检索失败：" + truncateStr(query, 30),
		}, nil
	}

	// 按 article id 去重，排除此前已纳入的
	var fresh []string
	roundSeen := make(map[int]struct{})
	for _, c := range chunks {
		id := extractArticleID(c)
		if id != 0 {
			if _, ok := t.seen[id]; ok {
				continue
			}
			if _, ok := roundSeen[id]; ok {
				continue
			}
			roundSeen[id] = struct{}{}
		}
		fresh = append(fresh, c)
	}

	if len(fresh) == 0 {
		return ToolResult{
			Content: fmt.Sprintf("检索「%s」未找到新的相关内容。", query),
			Display: "未检索到新内容：" + truncateStr(query, 30),
		}, nil
	}

	// 复用 scoreChunks 打分提炼
	obs := t.scoreToObservations(ctx, fresh)

	// 组装回灌给 LLM 的文本
	var sb strings.Builder
	sb.WriteString(fmt.Sprintf("检索「%s」得到 %d 条相关内容：\n", query, len(obs)))
	for i, o := range obs {
		line := o.Extract
		if line == "" {
			line = truncateStr(stripMetaHeader(o.Chunk), 160)
		}
		sb.WriteString(fmt.Sprintf("%d. [相关度%d] %s\n", i+1, o.Score, line))
	}

	titles := make([]string, 0, len(obs))
	for _, o := range obs {
		if tl := extractTitle(o.Chunk); tl != "" {
			titles = append(titles, "• "+tl)
		}
	}
	display := fmt.Sprintf("检索「%s」→ 采纳 %d 条", truncateStr(query, 30), len(obs))
	if len(titles) > 0 {
		display += "\n" + strings.Join(titles, "\n")
	}

	return ToolResult{
		Content:      sb.String(),
		Observations: obs,
		Display:      display,
	}, nil
}

// scoreToObservations 对片段打分，保留 >= minScore 的，转成 Observation 并登记去重。
// 打分失败或全部越界时降级保留全部。
func (t *LocalSearchTool) scoreToObservations(ctx context.Context, chunks []string) []Observation {
	items, err := scoreChunks(ctx, t.llmCall, t.intent, t.question, chunks)
	if err != nil || len(items) == 0 {
		return t.accumulateAll(chunks)
	}
	var out []Observation
	validMapped := 0
	for _, item := range items {
		idx := item.ID - 1
		if idx < 0 || idx >= len(chunks) {
			continue
		}
		validMapped++
		if item.Score < t.minScore {
			continue
		}
		c := chunks[idx]
		id := extractArticleID(c)
		markSeen(t.seen, id)
		out = append(out, Observation{
			ArticleID: id,
			Chunk:     c,
			Extract:   item.Extract,
			Score:     item.Score,
		})
	}
	if validMapped == 0 {
		return t.accumulateAll(chunks)
	}
	return out
}

// accumulateAll 降级：全部以中性分纳入。
func (t *LocalSearchTool) accumulateAll(chunks []string) []Observation {
	out := make([]Observation, 0, len(chunks))
	for _, c := range chunks {
		id := extractArticleID(c)
		markSeen(t.seen, id)
		out = append(out, Observation{ArticleID: id, Chunk: c, Score: t.minScore})
	}
	return out
}
