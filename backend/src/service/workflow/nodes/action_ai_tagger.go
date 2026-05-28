package nodes

import (
	"context"
	"fmt"

	"opinion-analysis/src/service/tagger"
)

// AITaggerNode AI打标节点
type AITaggerNode struct {
	taggerSvc *tagger.Service
}

func NewAITaggerNode(taggerSvc *tagger.Service) *AITaggerNode {
	return &AITaggerNode{taggerSvc: taggerSvc}
}

func (n *AITaggerNode) Type() string {
	return "ai_tagger"
}

func (n *AITaggerNode) Validate(config map[string]interface{}) error {
	// batchSize 是可选的，有默认值
	return nil
}

func (n *AITaggerNode) Execute(ctx context.Context, config map[string]interface{}, input map[string]interface{}) (map[string]interface{}, error) {
	// 执行AI打标逻辑
	tagged, err := n.taggerSvc.RunOnce(ctx)
	if err != nil {
		return nil, fmt.Errorf("ai tagger failed: %w", err)
	}

	// 返回输出数据（供下游节点使用）
	return map[string]interface{}{
		"taggedCount": tagged,
		"success":     true,
	}, nil
}
