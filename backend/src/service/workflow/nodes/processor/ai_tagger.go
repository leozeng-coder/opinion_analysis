package processor

import (
	"context"
	"log"

	"opinion-analysis/src/service/tagger"
	"opinion-analysis/src/service/workflow/nodes"
)

// AITaggerNode AI打标节点
// 只处理上游传入的文章ID列表
type AITaggerNode struct {
	*nodes.BaseNode
	taggerSvc *tagger.Service
}

// NewAITaggerNode 创建AI打标节点
func NewAITaggerNode(taggerSvc *tagger.Service) *AITaggerNode {
	return &AITaggerNode{
		BaseNode:  nodes.NewBaseNode("ai_tagger"),
		taggerSvc: taggerSvc,
	}
}

// Validate 验证配置
func (n *AITaggerNode) Validate(config map[string]interface{}) error {
	// batchSize 是可选的，有默认值
	return nil
}

// Execute 执行AI打标
func (n *AITaggerNode) Execute(ctx context.Context, config map[string]interface{}, input map[string]interface{}) (map[string]interface{}, error) {
	// 从配置获取参数
	batchSize := n.GetInt(config, "batchSize", 20)

	// 从上游节点获取文章ID列表（责任链传递）
	articleIds := n.GetArticleIDs(input)

	log.Printf("[AITaggerNode] Starting AI tagging: %d articles, batchSize=%d",
		len(articleIds), batchSize)

	var taggedCount int
	var err error

	if len(articleIds) > 0 {
		// 只处理指定的文章ID
		taggedCount, err = n.taggerSvc.TagArticlesByIDs(ctx, articleIds)
		if err != nil {
			return nil, n.WrapError("AI tagging failed", err)
		}
		log.Printf("[AITaggerNode] Tagged %d articles from provided IDs", taggedCount)
	} else {
		// 如果没有文章ID，处理所有未打标的文章（兼容旧逻辑）
		log.Printf("[AITaggerNode] No articleIds provided, processing all pending articles")
		taggedCount, err = n.taggerSvc.RunOnce(ctx)
		if err != nil {
			return nil, n.WrapError("AI tagging failed", err)
		}
		log.Printf("[AITaggerNode] Tagged %d pending articles", taggedCount)
	}

	// 构造输出（继承输入 + 新增字段）
	output := n.MergeOutput(input, map[string]interface{}{
		"taggedCount": taggedCount,           // 本次打标的文章数量
		"success":     true,                  // 执行成功
		"batchSize":   batchSize,             // 使用的批次大小
	})

	log.Printf("[AITaggerNode] AI tagging completed: %d articles tagged", taggedCount)

	return output, nil
}
