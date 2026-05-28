package nodes

import (
	"context"
)

// NodeExecutor 节点执行器接口
type NodeExecutor interface {
	// Execute 执行节点逻辑
	// ctx: 上下文
	// config: 节点配置（从JSON解析）
	// input: 上游节点的输出数据
	// 返回: 输出数据、错误
	Execute(ctx context.Context, config map[string]interface{}, input map[string]interface{}) (map[string]interface{}, error)

	// Type 返回节点类型标识（如 "ai_tagger"）
	Type() string

	// Validate 验证节点配置是否合法
	Validate(config map[string]interface{}) error
}
