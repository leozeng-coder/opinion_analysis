package report

import "strings"

// stripFences 去掉 ```json ... ``` 之类的 markdown 代码围栏
func stripFences(s string) string {
	s = strings.TrimSpace(s)
	for _, fence := range []string{"```json", "```JSON", "```"} {
		if idx := strings.Index(s, fence); idx >= 0 {
			s = s[idx+len(fence):]
			break
		}
	}
	if idx := strings.LastIndex(s, "```"); idx >= 0 {
		s = s[:idx]
	}
	return strings.TrimSpace(s)
}

// extractJSONObject 从可能含前后缀/围栏的文本中提取首个 { 到最后一个 } 的子串
func extractJSONObject(s string) string {
	s = stripFences(s)
	start := strings.IndexByte(s, '{')
	end := strings.LastIndexByte(s, '}')
	if start < 0 || end <= start {
		return s
	}
	return s[start : end+1]
}

// extractJSONArray 提取首个 [ 到最后一个 ] 的子串
func extractJSONArray(s string) string {
	s = stripFences(s)
	start := strings.IndexByte(s, '[')
	end := strings.LastIndexByte(s, ']')
	if start < 0 || end <= start {
		return s
	}
	return s[start : end+1]
}

// salvageArray 抢救被截断的 JSON 数组：当输出在中途被 max_tokens 截断、
// 末尾的 ] 缺失时，回退到「最后一个完整闭合的顶层元素」并补上 ]。
// 仅处理对象元素数组（[{...},{...}]），这是本服务所有数组响应的形态。
func salvageArray(s string) string {
	s = stripFences(s)
	start := strings.IndexByte(s, '[')
	if start < 0 {
		return s
	}
	// 已经能正常闭合则直接返回标准提取
	if end := strings.LastIndexByte(s, ']'); end > start {
		return s[start : end+1]
	}
	// 截断场景：扫描找出最后一个在顶层闭合的 '}'
	depth := 0
	inStr := false
	esc := false
	lastComplete := -1
	for i := start; i < len(s); i++ {
		c := s[i]
		if esc {
			esc = false
			continue
		}
		switch {
		case c == '\\' && inStr:
			esc = true
		case c == '"':
			inStr = !inStr
		case inStr:
			// 字符串内部，忽略括号
		case c == '{':
			depth++
		case c == '}':
			depth--
			if depth == 0 {
				lastComplete = i
			}
		}
	}
	if lastComplete < 0 {
		return s[start:] // 无完整元素，交给上层判定失败
	}
	return s[start:lastComplete+1] + "]"
}

// salvageObject 抢救被截断的 JSON 对象：缺失末尾 } 时，回退到最后一个
// 顶层完整闭合的位置。用于 {"clusters":[...]} 这类响应在数组尾部被截断的情况。
func salvageObject(s string) string {
	s = stripFences(s)
	start := strings.IndexByte(s, '{')
	if start < 0 {
		return s
	}
	if end := strings.LastIndexByte(s, '}'); end > start {
		// 校验括号是否平衡；不平衡说明被截断
		if balanced(s[start : end+1]) {
			return s[start : end+1]
		}
	}
	return s[start:]
}

// balanced 判断 JSON 片段的花括号/方括号是否平衡（字符串内不计）
func balanced(s string) bool {
	depth := 0
	inStr := false
	esc := false
	for i := 0; i < len(s); i++ {
		c := s[i]
		if esc {
			esc = false
			continue
		}
		switch {
		case c == '\\' && inStr:
			esc = true
		case c == '"':
			inStr = !inStr
		case inStr:
		case c == '{', c == '[':
			depth++
		case c == '}', c == ']':
			depth--
		}
	}
	return depth == 0 && !inStr
}
