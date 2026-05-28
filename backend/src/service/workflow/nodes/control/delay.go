package control

import (
	"context"
	"time"

	"opinion-analysis/src/service/workflow/nodes"
)

// DelayNode 延迟节点
type DelayNode struct {
	*nodes.BaseNode
}

// NewDelayNode 创建延迟节点
func NewDelayNode() *DelayNode {
	return &DelayNode{
		BaseNode: nodes.NewBaseNode("delay"),
	}
}

// Validate 验证配置
func (n *DelayNode) Validate(config map[string]interface{}) error {
	// duration 是可选的，有默认值
	return nil
}

// Execute 执行延迟
func (n *DelayNode) Execute(ctx context.Context, config map[string]interface{}, input map[string]interface{}) (map[string]interface{}, error) {
	duration := n.GetInt(config, "duration", 1)

	select {
	case <-time.After(time.Duration(duration) * time.Second):
		// 延迟完成
	case <-ctx.Done():
		return nil, n.WrapError("delay cancelled", ctx.Err())
	}

	// 直接传递输入到输出
	return input, nil
}
