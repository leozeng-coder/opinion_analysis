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
