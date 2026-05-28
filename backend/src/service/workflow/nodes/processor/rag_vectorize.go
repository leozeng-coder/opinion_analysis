package processor

import (
	"context"
	"log"

	"opinion-analysis/src/service/ragprocess"
	"opinion-analysis/src/service/workflow/nodes"
)

// RAGVectorizeNode RAG向量化节点
type RAGVectorizeNode struct {
	*nodes.BaseNode
	ragProc *ragprocess.Manager
}

// NewRAGVectorizeNode 创建RAG向量化节点
func NewRAGVectorizeNode(ragProc *ragprocess.Manager) *RAGVectorizeNode {
	return &RAGVectorizeNode{
		BaseNode: nodes.NewBaseNode("rag_vectorize"),
		ragProc:  ragProc,
	}
}

// Validate 验证配置
func (n *RAGVectorizeNode) Validate(config map[string]interface{}) error {
	// 无需配置参数
	return nil
}

// Execute 执行RAG向量化
func (n *RAGVectorizeNode) Execute(ctx context.Context, config map[string]interface{}, input map[string]interface{}) (map[string]interface{}, error) {
	// 从上游获取文章ID
	articleIds := n.GetArticleIDs(input)

	log.Printf("[RAGVectorizeNode] Ensuring RAG service is running for %d articles", len(articleIds))

	// 确保 RAG 服务运行
	err := n.ragProc.EnsureStarted(ctx)
	if err != nil {
		return nil, n.WrapError("RAG service start failed", err)
	}

	output := n.MergeOutput(input, map[string]interface{}{
		"ragServiceRunning": true,
		"success":           true,
	})

	log.Printf("[RAGVectorizeNode] RAG service is running")

	return output, nil
}
