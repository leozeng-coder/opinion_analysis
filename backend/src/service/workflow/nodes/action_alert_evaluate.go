package nodes

import (
	"context"
	"fmt"

	"opinion-analysis/src/service/alertengine"
)

// AlertEvaluateNode 告警评估节点
type AlertEvaluateNode struct {
	alertEngine *alertengine.Engine
}

func NewAlertEvaluateNode(alertEngine *alertengine.Engine) *AlertEvaluateNode {
	return &AlertEvaluateNode{alertEngine: alertEngine}
}

func (n *AlertEvaluateNode) Type() string {
	return "alert_evaluate"
}

func (n *AlertEvaluateNode) Validate(config map[string]interface{}) error {
	// 无需配置参数
	return nil
}

func (n *AlertEvaluateNode) Execute(ctx context.Context, config map[string]interface{}, input map[string]interface{}) (map[string]interface{}, error) {
	// 执行告警评估
	result, err := n.alertEngine.EvaluateAll(ctx, "workflow")
	if err != nil {
		return nil, fmt.Errorf("alert evaluation failed: %w", err)
	}

	return map[string]interface{}{
		"success":   true,
		"triggered": result.Triggered,
		"evaluated": result.Evaluated,
		"skipped":   result.Skipped,
		"message":   "Alert evaluation completed",
	}, nil
}
