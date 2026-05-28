package nodes

import (
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
	if val, ok := config[key].([]interface{}); ok {
		result := make([]string, 0, len(val))
		for _, v := range val {
			if str, ok := v.(string); ok {
				result = append(result, str)
			}
		}
		return result
	}
	return []string{}
}

// GetArticleIDs 从输入中获取文章ID列表
func (n *BaseNode) GetArticleIDs(input map[string]interface{}) []int64 {
	if val, ok := input["articleIds"].([]int64); ok {
		return val
	}
	if val, ok := input["articleIds"].([]interface{}); ok {
		result := make([]int64, 0, len(val))
		for _, v := range val {
			switch id := v.(type) {
			case float64:
				result = append(result, int64(id))
			case int64:
				result = append(result, id)
			case int:
				result = append(result, int64(id))
			}
		}
		return result
	}
	return []int64{}
}

// SetArticleIDs 设置文章ID列表到输出
func (n *BaseNode) SetArticleIDs(output map[string]interface{}, articleIds []int64) {
	output["articleIds"] = articleIds
}

// MergeOutput 合并输入和输出（责任链模式）
func (n *BaseNode) MergeOutput(input, output map[string]interface{}) map[string]interface{} {
	result := make(map[string]interface{})

	// 先复制输入
	for k, v := range input {
		result[k] = v
	}

	// 再覆盖输出
	for k, v := range output {
		result[k] = v
	}

	return result
}

// WrapError 包装错误信息
func (n *BaseNode) WrapError(message string, err error) error {
	if err != nil {
		return fmt.Errorf("%s: %w", message, err)
	}
	return fmt.Errorf("%s", message)
}
