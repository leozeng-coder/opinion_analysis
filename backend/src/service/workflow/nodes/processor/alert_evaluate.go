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
//
// 配置项：
//   - ruleIds:        []number 指定要评估的规则 id（留空 = 评估全部启用规则）
//   - timeRangeDays:  number   统一覆盖各规则的回溯天数（留空/0 = 走规则各自配置的时间范围）
func (n *AlertEvaluateNode) Execute(ctx context.Context, config map[string]interface{}, input map[string]interface{}) (map[string]interface{}, error) {
	ruleIDs := parseRuleIDs(config["ruleIds"])
	overrideDays := n.GetInt(config, "timeRangeDays", 0)

	log.Printf("[AlertEvaluateNode] Evaluating alerts: ruleIDs=%v overrideDays=%d", ruleIDs, overrideDays)

	result, err := n.alertEngine.EvaluateRules(ctx, "workflow", ruleIDs, overrideDays)
	if err != nil {
		return nil, n.WrapError("alert evaluation failed", err)
	}

	alertCount := result.Triggered

	output := n.MergeOutput(input, map[string]interface{}{
		"alertCount":     alertCount,
		"evaluated":      result.Evaluated,
		"alertRuleIds":   ruleIDs,
		"alertRangeDays": overrideDays,
		"success":        true,
	})

	log.Printf("[AlertEvaluateNode] Alert evaluation completed: evaluated=%d triggered=%d", result.Evaluated, alertCount)

	return output, nil
}

// parseRuleIDs 解析配置中的规则 id 列表（JSON 反序列化后通常为 []interface{}{float64}）。
func parseRuleIDs(raw interface{}) []uint {
	arr, ok := raw.([]interface{})
	if !ok {
		return nil
	}
	out := make([]uint, 0, len(arr))
	for _, v := range arr {
		switch x := v.(type) {
		case float64:
			if x > 0 {
				out = append(out, uint(x))
			}
		case int:
			if x > 0 {
				out = append(out, uint(x))
			}
		case int64:
			if x > 0 {
				out = append(out, uint(x))
			}
		}
	}
	return out
}
