package action

import (
	"context"
	"fmt"
	"io"
	"log"
	"net/http"
	"strings"
	"time"

	"opinion-analysis/src/service/workflow/nodes"
)

// HTTPRequestNode 发送任意 HTTP 请求节点
type HTTPRequestNode struct {
	*nodes.BaseNode
}

func NewHTTPRequestNode() *HTTPRequestNode {
	return &HTTPRequestNode{
		BaseNode: nodes.NewBaseNode("http_request"),
	}
}

func (n *HTTPRequestNode) Validate(config map[string]interface{}) error {
	if _, ok := config["url"]; !ok {
		return fmt.Errorf("url is required")
	}
	if _, ok := config["method"]; !ok {
		return fmt.Errorf("method is required")
	}
	return nil
}

func (n *HTTPRequestNode) Execute(ctx context.Context, config map[string]interface{}, input map[string]interface{}) (map[string]interface{}, error) {
	url := n.GetString(config, "url", "")
	method := strings.ToUpper(n.GetString(config, "method", "GET"))
	timeoutSec := n.GetInt(config, "timeoutSeconds", 30)

	var body io.Reader
	if bodyStr, ok := config["body"].(string); ok && bodyStr != "" {
		body = strings.NewReader(bodyStr)
	}

	client := &http.Client{Timeout: time.Duration(timeoutSec) * time.Second}
	req, err := http.NewRequestWithContext(ctx, method, url, body)
	if err != nil {
		return nil, n.WrapError("build request failed", err)
	}

	if headers, ok := config["headers"].(map[string]interface{}); ok {
		for k, v := range headers {
			req.Header.Set(k, fmt.Sprintf("%v", v))
		}
	}

	log.Printf("[HTTPRequestNode] %s %s", method, url)

	resp, err := client.Do(req)
	if err != nil {
		return nil, n.WrapError("request failed", err)
	}
	defer resp.Body.Close()

	respBody, _ := io.ReadAll(resp.Body)
	success := resp.StatusCode >= 200 && resp.StatusCode < 300

	produced := map[string]interface{}{
		"httpStatusCode": resp.StatusCode,
		"httpBody":       string(respBody),
		"httpSuccess":    success,
	}
	if !success {
		produced["status"] = "partial_success"
	}

	log.Printf("[HTTPRequestNode] Response: status=%d success=%v", resp.StatusCode, success)
	return nodes.CarryForward(input, produced), nil
}
