package middleware

import (
	"bytes"
	"io"
	"log"
	"time"

	"github.com/gin-gonic/gin"
	"gorm.io/gorm"
	"opinion-analysis/internal/model"
	"opinion-analysis/pkg/response"
	"opinion-analysis/pkg/utils"
)

func RequireAdmin() gin.HandlerFunc {
	return func(c *gin.Context) {
		role, _ := c.Get("role")
		if role != "admin" {
			response.Forbidden(c)
			c.Abort()
			return
		}
		c.Next()
	}
}

// Audit 写操作审计中间件：在响应完成后异步写入 audit_logs。
// resource: 资源类型；action: 动作。请求体经 MaskSensitive 脱敏后落库。
func Audit(db *gorm.DB, resource, action string) gin.HandlerFunc {
	return func(c *gin.Context) {
		var body []byte
		if c.Request.Body != nil && c.Request.ContentLength > 0 && c.Request.ContentLength < 32*1024 {
			b, err := io.ReadAll(c.Request.Body)
			if err == nil {
				body = b
				c.Request.Body = io.NopCloser(bytes.NewBuffer(b))
			}
		}

		c.Next()

		actorID, _ := c.Get("userID")
		actorName, _ := c.Get("username")
		uid, _ := actorID.(uint)
		uname, _ := actorName.(string)

		entry := model.AuditLog{
			ActorID:    uid,
			ActorName:  uname,
			Action:     action,
			Resource:   resource,
			ResourceID: c.Param("id"),
			Method:     c.Request.Method,
			Path:       c.FullPath(),
			Status:     c.Writer.Status(),
			Payload:    utils.MaskSensitive(body),
			IP:         c.ClientIP(),
			UserAgent:  c.Request.UserAgent(),
			CreatedAt:  time.Now(),
		}

		go func(e model.AuditLog) {
			if err := db.Create(&e).Error; err != nil {
				log.Printf("[audit] write failed: %v", err)
			}
		}(entry)
	}
}
