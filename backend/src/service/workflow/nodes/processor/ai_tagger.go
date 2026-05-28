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
	batchSize := n.GetInt(config, "batchSize", 20)
	onlyProvided := n.GetBool(config, "onlyProvidedIds", true)

	articleIds := n.GetArticleIDs(input)

	log.Printf("[AITaggerNode] Input payload: articleIds field=%v (type=%T)",
		input["articleIds"], input["articleIds"])
	log.Printf("[AITaggerNode] Starting AI tagging: %d articleIds, batchSize=%d onlyProvided=%v",
		len(articleIds), batchSize, onlyProvided)

	var taggedCount int
	var err error

	if len(articleIds) > 0 {
		taggedCount, err = n.taggerSvc.TagArticlesByIDs(ctx, articleIds)
		if err != nil {
			return nil, n.WrapError("AI tagging failed", err)
		}
	} else if onlyProvided {
		log.Printf("[AITaggerNode] No articleIds in upstream payload, skipping (onlyProvidedIds=true)")
		taggedCount = 0
	} else {
		log.Printf("[AITaggerNode] No articleIds provided, processing all pending articles")
		taggedCount, err = n.taggerSvc.RunOnce(ctx)
		if err != nil {
			return nil, n.WrapError("AI tagging failed", err)
		}
	}

	output := n.MergeOutput(input, map[string]interface{}{
		"taggedCount": taggedCount,
		"success":     true,
		"batchSize":   batchSize,
	})

	log.Printf("[AITaggerNode] AI tagging completed: %d articles tagged", taggedCount)
	return output, nil
}
