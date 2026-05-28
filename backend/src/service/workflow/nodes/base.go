package nodes

import (
	"context"
	"fmt"
)

// BaseNode 节点基类，提供通用功能
type BaseNode struct {
	nodeType string
}

// NewBaseNode 创建基础节点
func NewBaseNode(nodeType string) *BaseNode {
	return &BaseNode{nodeType: nodeType}
}

// Type 返回节点类型
func (n *BaseNode) Type() string {
	return n.nodeType
}

// ValidateRequired 验证必填字段
func (n *BaseNode) ValidateRequired(config map[string]interface{}, fields ...string) error {
	for _, field := range fields {
		if _, ok := config[field]; !ok {
			return fmt.Errorf("field '%s' is required", field)
		}
	}
	return nil
}

// GetString 从配置中获取字符串值
func (n *BaseNode) GetString(config map[string]interface{}, key string, defaultValue string) string {
	if val, ok := config[key].(string); ok {
		return val
	}
	return defaultValue
}

// GetInt 从配置中获取整数值
func (n *BaseNode) GetInt(config map[string]interface{}, key string, defaultValue int) int {
	if val, ok := config[key].(float64); ok {
		return int(val)
	}
	return defaultValue
}

// GetBool 从配置中获取布尔值
func (n *BaseNode) GetBool(config map[string]interface{}, key string, defaultValue bool) bool {
	if val, ok := config[key].(bool); ok {
		return val
	}
	return defaultValue
}

// GetStringSlice 从配置中获取字符串数组
func (n *BaseNode) GetStringSlice(config map[string]interface{}, key string) []string {
	if val, ok := config[key].([]string); ok {
		return val
	}
	return GetStringSliceFromInput(config, key)
}

// GetArticleIDs 从输入中获取文章ID列表（使用统一的解包函数）
func (n *BaseNode) GetArticleIDs(input map[string]interface{}) []int64 {
	if input == nil {
		return []int64{}
	}
	return UnpackArticleIDs(input["articleIds"])
}

// SetArticleIDs 设置文章ID列表到输出
func (n *BaseNode) SetArticleIDs(output map[string]interface{}, articleIds []int64) {
	output["articleIds"] = articleIds
}

// MergeOutput 合并输入和输出（责任链模式，同 CarryForward）
func (n *BaseNode) MergeOutput(input, output map[string]interface{}) map[string]interface{} {
	return CarryForward(input, output)
}

// OnCancel 默认实现：空操作，依赖 ctx.Done() 在阻塞点自然退出。
// 节点如需主动停止外部资源（如爬虫进程），应覆盖此方法。
func (n *BaseNode) OnCancel(_ context.Context) {}

// WrapError 包装错误信息
func (n *BaseNode) WrapError(message string, err error) error {
	if err != nil {
		return fmt.Errorf("%s: %w", message, err)
	}
	return fmt.Errorf("%s", message)
}
