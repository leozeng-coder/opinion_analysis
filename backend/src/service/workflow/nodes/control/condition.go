package control

import (
	"context"
	"fmt"

	"opinion-analysis/src/service/workflow/nodes"
)

// ConditionNode 条件判断节点
type ConditionNode struct {
	*nodes.BaseNode
}

// NewConditionNode 创建条件节点
func NewConditionNode() *ConditionNode {
	return &ConditionNode{
		BaseNode: nodes.NewBaseNode("condition"),
	}
}

// Validate 验证配置
func (n *ConditionNode) Validate(config map[string]interface{}) error {
	// expression 是可选的，有默认逻辑
	return nil
}

// Execute 执行条件判断
func (n *ConditionNode) Execute(ctx context.Context, config map[string]interface{}, input map[string]interface{}) (map[string]interface{}, error) {
	expression := n.GetString(config, "expression", "")

	// TODO: 实现表达式求值
	// 目前简单实现：检查 articleIds 是否为空
	articleIds := n.GetArticleIDs(input)
	result := len(articleIds) > 0

	output := n.MergeOutput(input, map[string]interface{}{
		"conditionResult": result,
		"expression":      expression,
	})

	return output, nil
}

// evaluateExpression 求值表达式（简化实现）
func (n *ConditionNode) evaluateExpression(expression string, input map[string]interface{}) (bool, error) {
	// TODO: 实现完整的表达式求值
	// 例如：input.taggedCount > 10
	// 目前返回固定值
	return true, fmt.Errorf("expression evaluation not implemented")
}
