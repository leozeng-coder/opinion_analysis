package nodes

import (
	"context"
	"fmt"

	"opinion-analysis/src/service/ragprocess"
)

// RAGVectorizeNode RAG向量化节点
type RAGVectorizeNode struct {
	ragProc *ragprocess.Manager
}

func NewRAGVectorizeNode(ragProc *ragprocess.Manager) *RAGVectorizeNode {
	return &RAGVectorizeNode{ragProc: ragProc}
}

func (n *RAGVectorizeNode) Type() string {
	return "rag_vectorize"
}

func (n *RAGVectorizeNode) Validate(config map[string]interface{}) error {
	// collectionName 可选，使用默认值
	return nil
}

func (n *RAGVectorizeNode) Execute(ctx context.Context, config map[string]interface{}, input map[string]interface{}) (map[string]interface{}, error) {
	// 确保 RAG 服务已启动
	if !n.ragProc.IsRunning() {
		if err := n.ragProc.EnsureStarted(ctx); err != nil {
			return nil, fmt.Errorf("failed to start RAG service: %w", err)
		}
	}

	return map[string]interface{}{
		"success": true,
		"message": "RAG service is running",
	}, nil
}
