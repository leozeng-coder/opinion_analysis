package nodes

import (
	"strconv"
	"time"
)

// 工作流节点间传递的标准字段：
//   crawlerRunId, crawlerStartedAt, platforms, syncPlatformCodes,
//   articleIds, articlesCount, taggedCount, syncNewCount

// GetUint 从 input 读取无符号整数（兼容 JSON 反序列化后的 float64）
func GetUint(input map[string]interface{}, key string) uint {
	if input == nil {
		return 0
	}
	switch v := input[key].(type) {
	case float64:
		return uint(v)
	case int:
		return uint(v)
	case int64:
		return uint(v)
	case uint:
		return v
	case uint64:
		return uint(v)
	case string:
		n, _ := strconv.ParseUint(v, 10, 64)
		return uint(n)
	}
	return 0
}

// GetTime 从 input 读取 RFC3339 时间
func GetTime(input map[string]interface{}, key string) time.Time {
	if input == nil {
		return time.Time{}
	}
	switch v := input[key].(type) {
	case string:
		t, err := time.Parse(time.RFC3339, v)
		if err == nil {
			return t
		}
	case time.Time:
		return v
	}
	return time.Time{}
}

// GetStringSliceFromInput 从 input 读取字符串数组（兼容 []string / []interface{}）
func GetStringSliceFromInput(input map[string]interface{}, key string) []string {
	if input == nil {
		return nil
	}
	if val, ok := input[key].([]string); ok {
		return val
	}
	if val, ok := input[key].([]interface{}); ok {
		out := make([]string, 0, len(val))
		for _, item := range val {
			if s, ok := item.(string); ok {
				out = append(out, s)
			}
		}
		return out
	}
	return nil
}

// PackArticleIDs 将 int64 切片转为 []interface{} 供下游节点消费
// 注意：为了兼容 JSON 序列化，统一转为 float64
func PackArticleIDs(ids []int64) []interface{} {
	out := make([]interface{}, len(ids))
	for i, id := range ids {
		out[i] = float64(id) // JSON 序列化后会变成 float64
	}
	return out
}

// UnpackArticleIDs 从 interface{} 解包为 []int64
func UnpackArticleIDs(val interface{}) []int64 {
	if val == nil {
		return []int64{}
	}

	// 直接是 []int64
	if ids, ok := val.([]int64); ok {
		return ids
	}

	// []interface{} 格式（最常见）
	if arr, ok := val.([]interface{}); ok {
		result := make([]int64, 0, len(arr))
		for _, v := range arr {
			switch id := v.(type) {
			case float64:
				result = append(result, int64(id))
			case int64:
				result = append(result, id)
			case int:
				result = append(result, int64(id))
			case uint:
				result = append(result, int64(id))
			}
		}
		return result
	}

	return []int64{}
}

// CarryForward 合并上游 input 与当前节点产出，保证责任链字段向下传递
func CarryForward(input, produced map[string]interface{}) map[string]interface{} {
	result := make(map[string]interface{})
	for k, v := range input {
		result[k] = v
	}
	for k, v := range produced {
		result[k] = v
	}
	return result
}
