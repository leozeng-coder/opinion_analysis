package nodes

import (
	"context"
	"fmt"
	"strconv"
	"strings"

	"github.com/Knetic/govaluate"
)

// ConditionNode 条件判断节点
type ConditionNode struct{}

func NewConditionNode() *ConditionNode {
	return &ConditionNode{}
}

func (n *ConditionNode) Type() string {
	return "condition_if"
}

func (n *ConditionNode) Validate(config map[string]interface{}) error {
	if _, ok := config["condition"]; !ok {
		return fmt.Errorf("condition is required")
	}
	return nil
}

func (n *ConditionNode) Execute(ctx context.Context, config map[string]interface{}, input map[string]interface{}) (map[string]interface{}, error) {
	conditionStr := config["condition"].(string)

	// 使用 govaluate 库解析和执行表达式
	expression, err := govaluate.NewEvaluableExpression(conditionStr)
	if err != nil {
		return nil, fmt.Errorf("invalid condition expression: %w", err)
	}

	// 将 input 转换为 govaluate 可用的参数
	parameters := make(map[string]interface{})
	for k, v := range input {
		// 处理嵌套的 map，例如 output.count
		if strings.Contains(k, ".") {
			parts := strings.Split(k, ".")
			if len(parts) == 2 {
				parameters[parts[1]] = v
			}
		} else {
			parameters[k] = v
		}
	}

	// 执行表达式
	result, err := expression.Evaluate(parameters)
	if err != nil {
		return nil, fmt.Errorf("failed to evaluate condition: %w", err)
	}

	// 转换结果为布尔值
	var conditionMet bool
	switch v := result.(type) {
	case bool:
		conditionMet = v
	case float64:
		conditionMet = v != 0
	case string:
		conditionMet, _ = strconv.ParseBool(v)
	default:
		conditionMet = false
	}

	return map[string]interface{}{
		"conditionMet": conditionMet,
		"branch":       getBranch(conditionMet),
	}, nil
}

func getBranch(conditionMet bool) string {
	if conditionMet {
		return "true"
	}
	return "false"
}
