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

	// OnCancel 取消回调：当工作流被用户取消时，engine 会调此方法通知当前节点。
	//
	// 实现策略（两选一，节点自行选择）：
	//   - 立即终止：向外部服务发停止信号（如 crawler stop），需要自行处理 ctx 的最终状态。
	//   - 完成当前再退出：不做任何操作，依赖 ctx.Done() 在下一个检查点自然退出（BaseNode 默认行为）。
	//
	// 本方法应快速返回，不应阻塞；需要的异步操作请用 goroutine。
	OnCancel(ctx context.Context)
}

type progressKey struct{}

// WithProgressFunc 把进度回调注入 ctx，由 engine 在执行节点前调用
func WithProgressFunc(ctx context.Context, fn func(string)) context.Context {
	return context.WithValue(ctx, progressKey{}, fn)
}

// ProgressFunc 从 ctx 读取进度回调；若未注入则返回 no-op
func ProgressFunc(ctx context.Context) func(string) {
	if fn, ok := ctx.Value(progressKey{}).(func(string)); ok {
		return fn
	}
	return func(string) {}
}

