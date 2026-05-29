package control

import (
	"context"
	"fmt"
	"strconv"
	"strings"

	"opinion-analysis/src/service/workflow/nodes"
)

// ConditionNode 条件判断节点
//
// 表达式语法（简化版，足以覆盖工作流编排场景）：
//   - 比较：  input.taggedCount > 10 / articlesCount >= 5 / status == "synced"
//     支持运算符 > < >= <= == != ，左侧为上游字段（可带或不带 input. 前缀），
//     右侧为数字或带引号的字符串。
//   - 布尔字段：input.success / hasErrors —— 直接取字段真值。
//   - 留空：    回退为「上游 articleIds 非空」。
//
// 计算结果写入输出字段 conditionResult(bool)。engine 会据此决定是否执行下游节点。
type ConditionNode struct {
	*nodes.BaseNode
}

// NewConditionNode 创建条件节点
func NewConditionNode() *ConditionNode {
	return &ConditionNode{
		BaseNode: nodes.NewBaseNode("condition"),
	}
}

// Validate 验证配置
func (n *ConditionNode) Validate(config map[string]interface{}) error {
	// expression 是可选的，有默认逻辑
	return nil
}

// Execute 执行条件判断
//
// 优先级：结构化规则(conditions) > 表达式(expression，向后兼容) > 默认(上游有文章)。
func (n *ConditionNode) Execute(ctx context.Context, config map[string]interface{}, input map[string]interface{}) (map[string]interface{}, error) {
	rules := parseRules(config["conditions"])
	logic := strings.ToLower(strings.TrimSpace(n.GetString(config, "logic", "and")))
	if logic != "or" {
		logic = "and"
	}
	expression := strings.TrimSpace(n.GetString(config, "expression", ""))

	var result bool
	var err error
	switch {
	case len(rules) > 0:
		result, err = evaluateRules(rules, logic, input)
	case expression != "":
		result, err = evaluateExpression(expression, input)
	default:
		// 默认逻辑：上游有文章才继续
		result = len(n.GetArticleIDs(input)) > 0
	}
	if err != nil {
		return nil, n.WrapError("evaluate condition failed", err)
	}

	output := n.MergeOutput(input, map[string]interface{}{
		"conditionResult": result,
		"conditionLogic":  logic,
	})

	return output, nil
}

// condRule 单条结构化条件规则。
type condRule struct {
	field string
	op    string
	value string
}

// parseRules 解析前端表格录入的 conditions 数组（JSON 反序列化后为 []interface{}）。
func parseRules(raw interface{}) []condRule {
	arr, ok := raw.([]interface{})
	if !ok {
		return nil
	}
	out := make([]condRule, 0, len(arr))
	for _, item := range arr {
		m, ok := item.(map[string]interface{})
		if !ok {
			continue
		}
		field := strings.TrimSpace(toStr(m["field"]))
		op := strings.TrimSpace(toStr(m["op"]))
		value := strings.TrimSpace(toStr(m["value"]))
		if field == "" || op == "" {
			continue
		}
		out = append(out, condRule{field: field, op: op, value: value})
	}
	return out
}

// evaluateRules 按 and/or 组合多条规则。
func evaluateRules(rules []condRule, logic string, input map[string]interface{}) (bool, error) {
	for _, r := range rules {
		ok, err := compare(r.field, r.op, r.value, input)
		if err != nil {
			return false, fmt.Errorf("规则 [%s %s %s]: %w", r.field, r.op, r.value, err)
		}
		if logic == "or" {
			if ok {
				return true, nil
			}
		} else if !ok {
			return false, nil
		}
	}
	// and: 走到这里说明全部满足；or: 走到这里说明无一满足
	return logic != "or", nil
}

// toStr 把 JSON 值转字符串（兼容 string/number/bool）。
func toStr(v interface{}) string {
	switch x := v.(type) {
	case nil:
		return ""
	case string:
		return x
	case float64:
		return strconv.FormatFloat(x, 'f', -1, 64)
	case bool:
		return strconv.FormatBool(x)
	default:
		return fmt.Sprintf("%v", x)
	}
}

// evaluateExpression 求值单条条件表达式。
func evaluateExpression(expression string, input map[string]interface{}) (bool, error) {
	expr := strings.TrimSpace(expression)

	// 比较运算（注意 >= <= == != 要在 > < 之前匹配）
	for _, op := range []string{">=", "<=", "==", "!=", ">", "<"} {
		idx := strings.Index(expr, op)
		if idx < 0 {
			continue
		}
		left := strings.TrimSpace(expr[:idx])
		right := strings.TrimSpace(expr[idx+len(op):])
		return compare(left, op, right, input)
	}

	// 没有运算符：当作布尔字段取真值
	return truthy(fieldValue(expr, input)), nil
}

// compare 执行单个比较运算。
func compare(left, op, right string, input map[string]interface{}) (bool, error) {
	leftVal := fieldValue(left, input)

	// 右侧是带引号字符串 → 字符串比较
	if isQuoted(right) {
		rs := unquote(right)
		ls := fmt.Sprintf("%v", leftVal)
		switch op {
		case "==":
			return ls == rs, nil
		case "!=":
			return ls != rs, nil
		default:
			return false, fmt.Errorf("operator %s not supported for string operand", op)
		}
	}

	// 数字比较
	rf, err := strconv.ParseFloat(right, 64)
	if err != nil {
		return false, fmt.Errorf("right operand %q is not a number", right)
	}
	lf, ok := toFloat(leftVal)
	if !ok {
		// 左值无法转数字时，==/!= 退化为字符串比较，其它运算符判为 false
		ls := fmt.Sprintf("%v", leftVal)
		switch op {
		case "==":
			return ls == right, nil
		case "!=":
			return ls != right, nil
		default:
			return false, nil
		}
	}

	switch op {
	case ">":
		return lf > rf, nil
	case "<":
		return lf < rf, nil
	case ">=":
		return lf >= rf, nil
	case "<=":
		return lf <= rf, nil
	case "==":
		return lf == rf, nil
	case "!=":
		return lf != rf, nil
	}
	return false, fmt.Errorf("unsupported operator %s", op)
}

// fieldValue 读取字段值，支持 input. 前缀。
func fieldValue(token string, input map[string]interface{}) interface{} {
	key := strings.TrimSpace(token)
	key = strings.TrimPrefix(key, "input.")
	if input == nil {
		return nil
	}
	return input[key]
}

func toFloat(v interface{}) (float64, bool) {
	switch n := v.(type) {
	case float64:
		return n, true
	case float32:
		return float64(n), true
	case int:
		return float64(n), true
	case int64:
		return float64(n), true
	case uint:
		return float64(n), true
	case uint64:
		return float64(n), true
	case bool:
		if n {
			return 1, true
		}
		return 0, true
	case string:
		f, err := strconv.ParseFloat(n, 64)
		if err == nil {
			return f, true
		}
	}
	return 0, false
}

func truthy(v interface{}) bool {
	switch n := v.(type) {
	case nil:
		return false
	case bool:
		return n
	case float64:
		return n != 0
	case int:
		return n != 0
	case int64:
		return n != 0
	case string:
		return n != "" && n != "false" && n != "0"
	case []interface{}:
		return len(n) > 0
	}
	return true
}

func isQuoted(s string) bool {
	return len(s) >= 2 && ((s[0] == '"' && s[len(s)-1] == '"') || (s[0] == '\'' && s[len(s)-1] == '\''))
}

func unquote(s string) string {
	if isQuoted(s) {
		return s[1 : len(s)-1]
	}
	return s
}
