package pipeline

import (
	"context"
	"strings"
)

// RAGSearchFn is a dependency-injected function that queries the vector store.
type RAGSearchFn func(ctx context.Context, query string, topK int, topics []string) ([]string, error)

// RetrievalNode decides whether to search the knowledge base (based on Node 1's
// NeedRetrieval flag) and populates state.RetrievedChunks.
type RetrievalNode struct {
	search RAGSearchFn
}

func NewRetrievalNode(search RAGSearchFn) *RetrievalNode {
	return &RetrievalNode{search: search}
}

func (n *RetrievalNode) Name() string  { return "retrieval" }
func (n *RetrievalNode) Title() string { return "知识库检索" }

func (n *RetrievalNode) Execute(ctx context.Context, state *PipelineState, emit EmitFn) error {
	if !state.NeedRetrieval || n.search == nil {
		emit(ThinkStep{Step: n.Name(), Title: n.Title(), Content: "无需检索本地知识库", Status: StatusSkipped})
		return nil
	}

	emit(ThinkStep{Step: n.Name(), Title: n.Title(), Content: "检索中：" + truncateStr(state.RetrievalQuery, 40), Status: StatusRunning})

	chunks, err := n.search(ctx, state.RetrievalQuery, 20, state.Topics)
	if err != nil {
		emit(ThinkStep{Step: n.Name(), Title: n.Title(), Content: "检索失败，将基于通用知识作答", Status: StatusDone})
		return nil
	}

	state.RetrievedChunks = chunks

	if len(chunks) == 0 {
		emit(ThinkStep{Step: n.Name(), Title: n.Title(), Content: "未检索到相关内容，将基于通用知识作答", Status: StatusDone})
	} else {
		// First line: summary; subsequent lines: article titles for display
		lines := []string{"检索到 " + itoa(len(chunks)) + " 条相关内容"}
		for _, chunk := range state.RetrievedChunks {
			for _, line := range strings.Split(chunk, "\n") {
				line = strings.TrimSpace(line)
				if strings.HasPrefix(line, "标题：") {
					title := strings.TrimPrefix(line, "标题：")
					if title != "" {
						lines = append(lines, title)
					}
					break
				}
			}
		}
		emit(ThinkStep{Step: n.Name(), Title: n.Title(), Content: strings.Join(lines, "\n"), Status: StatusDone})
	}
	return nil
}

// truncateStr trims a string to at most max runes, appending … if cut.
func truncateStr(s string, max int) string {
	r := []rune(s)
	if len(r) <= max {
		return s
	}
	return string(r[:max]) + "…"
}

// itoa converts a small non-negative int to its decimal string.
func itoa(n int) string {
	if n == 0 {
		return "0"
	}
	digits := []byte{}
	for n > 0 {
		digits = append([]byte{byte('0' + n%10)}, digits...)
		n /= 10
	}
	return string(digits)
}
