package nodes

import (
	"context"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"
)

// HTTPRequestNode HTTP请求节点
type HTTPRequestNode struct{}

func NewHTTPRequestNode() *HTTPRequestNode {
	return &HTTPRequestNode{}
}

func (n *HTTPRequestNode) Type() string {
	return "http_request"
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
	url := config["url"].(string)
	method := strings.ToUpper(config["method"].(string))

	var body io.Reader
	if bodyStr, ok := config["body"].(string); ok && bodyStr != "" {
		body = strings.NewReader(bodyStr)
	}

	client := &http.Client{Timeout: 30 * time.Second}
	req, err := http.NewRequestWithContext(ctx, method, url, body)
	if err != nil {
		return nil, fmt.Errorf("failed to create request: %w", err)
	}

	// 设置请求头
	if headers, ok := config["headers"].(map[string]interface{}); ok {
		for k, v := range headers {
			req.Header.Set(k, fmt.Sprintf("%v", v))
		}
	}

	resp, err := client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("http request failed: %w", err)
	}
	defer resp.Body.Close()

	respBody, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("failed to read response: %w", err)
	}

	return map[string]interface{}{
		"statusCode": resp.StatusCode,
		"body":       string(respBody),
		"success":    resp.StatusCode >= 200 && resp.StatusCode < 300,
	}, nil
}
