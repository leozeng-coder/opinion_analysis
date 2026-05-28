package workflow

import (
	"context"
	"fmt"
)

// NodeExecutor 节点执行器接口
type NodeExecutor interface {
	// Type 返回节点类型标识
	Type() string

	// Validate 验证节点配置
	Validate(config map[string]interface{}) error

	// Execute 执行节点逻辑
	// input: 上游节点的输出数据
	// output: 本节点的输出数据（传递给下游节点）
	Execute(ctx context.Context, config map[string]interface{}, input map[string]interface{}) (output map[string]interface{}, err error)

	// OnCancel 取消回调：engine 在 CancelExecution 时调用，用于通知当前节点做资源清理。
	//
	// 两种策略（节点自行选择）：
	//   - 立即停止：主动终止外部资源（如爬虫），goroutine 内完成。
	//   - 自然退出：空实现，依赖 ctx.Done() 在下一个检查点退出（BaseNode 默认）。
	//
	// 必须快速返回，异步操作须在 goroutine 内完成。
	OnCancel(ctx context.Context)
}

// ExecutionContext 节点执行上下文
type ExecutionContext struct {
	WorkflowID  int64
	ExecutionID int64
	NodeID      string
	Input       map[string]interface{}
	Config      map[string]interface{}
}

// ExecutionResult 节点执行结果
type ExecutionResult struct {
	Success bool
	Output  map[string]interface{}
	Error   error
	Message string
}

// NewExecutionResult 创建执行结果
func NewExecutionResult(output map[string]interface{}, err error) *ExecutionResult {
	result := &ExecutionResult{
		Success: err == nil,
		Output:  output,
		Error:   err,
	}
	if err != nil {
		result.Message = err.Error()
	}
	return result
}

// ValidationError 配置验证错误
type ValidationError struct {
	Field   string
	Message string
}

func (e *ValidationError) Error() string {
	return fmt.Sprintf("validation error: %s - %s", e.Field, e.Message)
}

// ExecutionError 执行错误
type ExecutionError struct {
	NodeType string
	NodeID   string
	Message  string
	Cause    error
}

func (e *ExecutionError) Error() string {
	if e.Cause != nil {
		return fmt.Sprintf("[%s:%s] %s: %v", e.NodeType, e.NodeID, e.Message, e.Cause)
	}
	return fmt.Sprintf("[%s:%s] %s", e.NodeType, e.NodeID, e.Message)
}

func (e *ExecutionError) Unwrap() error {
	return e.Cause
}
