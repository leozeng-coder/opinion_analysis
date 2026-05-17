package utils

import (
	"encoding/json"
	"strings"
)

var sensitiveKeys = []string{
	"password", "secret", "token", "cookie", "cookies",
	"apikey", "api_key", "accesskey", "access_key",
	"deepseekapikey", "deepseek_api_key", "authorization",
}

func isSensitive(key string) bool {
	k := strings.ToLower(key)
	for _, s := range sensitiveKeys {
		if strings.Contains(k, s) {
			return true
		}
	}
	return false
}

// MaskSensitive 把 JSON 字节中的敏感字段值替换为 ***。
// 解析失败时返回原文（已截断），避免阻塞业务。
func MaskSensitive(raw []byte) string {
	const maxLen = 4096
	if len(raw) == 0 {
		return ""
	}
	var v any
	if err := json.Unmarshal(raw, &v); err != nil {
		s := string(raw)
		if len(s) > maxLen {
			s = s[:maxLen] + "...(truncated)"
		}
		return s
	}
	masked := maskValue(v)
	out, err := json.Marshal(masked)
	if err != nil {
		return ""
	}
	s := string(out)
	if len(s) > maxLen {
		s = s[:maxLen] + "...(truncated)"
	}
	return s
}

func maskValue(v any) any {
	switch x := v.(type) {
	case map[string]any:
		out := make(map[string]any, len(x))
		for k, val := range x {
			if isSensitive(k) {
				if _, ok := val.(string); ok {
					out[k] = "***"
					continue
				}
				out[k] = "***"
				continue
			}
			out[k] = maskValue(val)
		}
		return out
	case []any:
		for i := range x {
			x[i] = maskValue(x[i])
		}
		return x
	default:
		return v
	}
}

// MaskString 保留首4位+末4位（足够辨识但隐藏完整值）。
func MaskString(s string) string {
	if s == "" {
		return ""
	}
	if len(s) <= 8 {
		return "***"
	}
	return s[:4] + "***" + s[len(s)-4:]
}

// MaskDSN 对 user:password@tcp(...) 形式的 MySQL DSN 隐藏密码段。
func MaskDSN(dsn string) string {
	at := strings.LastIndex(dsn, "@")
	if at <= 0 {
		return dsn
	}
	cred := dsn[:at]
	colon := strings.Index(cred, ":")
	if colon < 0 {
		return dsn
	}
	return cred[:colon+1] + "***" + dsn[at:]
}
