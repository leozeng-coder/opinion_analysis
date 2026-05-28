package processor

import (
	"context"
	"log"

	"opinion-analysis/src/service/alertengine"
	"opinion-analysis/src/service/workflow/nodes"
)

// AlertEvaluateNode 告警评估节点
type AlertEvaluateNode struct {
	*nodes.BaseNode
	alertEngine *alertengine.Engine
}

// NewAlertEvaluateNode 创建告警评估节点
func NewAlertEvaluateNode(alertEngine *alertengine.Engine) *AlertEvaluateNode {
	return &AlertEvaluateNode{
		BaseNode:    nodes.NewBaseNode("alert_evaluate"),
		alertEngine: alertEngine,
	}
}

// Validate 验证配置
func (n *AlertEvaluateNode) Validate(config map[string]interface{}) error {
	// 无需配置参数
	return nil
}

// Execute 执行告警评估
func (n *AlertEvaluateNode) Execute(ctx context.Context, config map[string]interface{}, input map[string]interface{}) (map[string]interface{}, error) {
	// 从上游获取文章ID
	articleIds := n.GetArticleIDs(input)

	log.Printf("[AlertEvaluateNode] Evaluating alerts for %d articles", len(articleIds))

	// TODO: 实现只对指定文章的告警评估
	// 目前使用原有的评估所有文章的逻辑
	result, err := n.alertEngine.EvaluateAll(ctx, "workflow")
	if err != nil {
		return nil, n.WrapError("alert evaluation failed", err)
	}

	alertCount := result.Triggered

	output := n.MergeOutput(input, map[string]interface{}{
		"alertCount": alertCount,
		"evaluated":  result.Evaluated,
		"success":    true,
	})

	log.Printf("[AlertEvaluateNode] Alert evaluation completed: %d alerts triggered", alertCount)

	return output, nil
}
