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
	// seconds 是可选的，有默认值
	return nil
}

// Execute 执行延迟
func (n *DelayNode) Execute(ctx context.Context, config map[string]interface{}, input map[string]interface{}) (map[string]interface{}, error) {
	// 前端表单字段为 seconds；兼容历史配置里使用的 duration。
	seconds := n.GetInt(config, "seconds", 0)
	if seconds <= 0 {
		seconds = n.GetInt(config, "duration", 1)
	}
	if seconds <= 0 {
		seconds = 1
	}

	select {
	case <-time.After(time.Duration(seconds) * time.Second):
		// 延迟完成
	case <-ctx.Done():
		return nil, n.WrapError("delay cancelled", ctx.Err())
	}

	// 直接传递输入到输出
	return input, nil
}
