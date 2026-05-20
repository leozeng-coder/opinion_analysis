package user

import (
	"encoding/json"
	"fmt"
	"net/http"
	"time"

	"github.com/gin-gonic/gin"
)

// TestStreamHandler 测试 SSE 流式输出
func TestStreamHandler(c *gin.Context) {
	c.Header("Content-Type", "text/event-stream")
	c.Header("Cache-Control", "no-cache")
	c.Header("Connection", "keep-alive")
	c.Header("X-Accel-Buffering", "no")

	flusher, ok := c.Writer.(http.Flusher)
	if !ok {
		c.JSON(500, gin.H{"error": "streaming not supported"})
		return
	}

	// 发送测试消息
	for i := 1; i <= 5; i++ {
		data := map[string]interface{}{
			"content": fmt.Sprintf("测试消息 %d", i),
			"index":   i,
		}
		jsonData, _ := json.Marshal(data)
		fmt.Fprintf(c.Writer, "data: %s\n\n", jsonData)
		flusher.Flush()
		time.Sleep(500 * time.Millisecond)
	}

	// 发送完成事件
	doneData := map[string]bool{"done": true}
	doneJSON, _ := json.Marshal(doneData)
	fmt.Fprintf(c.Writer, "event: done\ndata: %s\n\n", doneJSON)
	flusher.Flush()
}
