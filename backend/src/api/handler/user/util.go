package user

import (
	"strconv"
	"strings"

	"github.com/gin-gonic/gin"
)

func CurrentUserID(c *gin.Context) (uint, bool) {
	v, ok := c.Get("userID")
	if !ok {
		return 0, false
	}
	uid, ok := v.(uint)
	return uid, ok
}

func TruncateErrMsg(s string, n int) string {
	r := []rune(s)
	if len(r) <= n {
		return s
	}
	return string(r[:n]) + "…"
}

func SplitNonEmpty(s, sep string) []string {
	if s == "" {
		return nil
	}
	parts := strings.Split(s, sep)
	out := make([]string, 0, len(parts))
	for _, p := range parts {
		if p = strings.TrimSpace(p); p != "" {
			out = append(out, p)
		}
	}
	return out
}

func ParseHotTopicThreshold(dbVal string, defaultVal int64) int64 {
	if n, err := strconv.ParseInt(strings.TrimSpace(dbVal), 10, 64); err == nil && n > 0 {
		return n
	}
	return defaultVal
}
