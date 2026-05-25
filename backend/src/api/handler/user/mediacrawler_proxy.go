package user

import (
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"net/http"
	"net/http/httputil"
	"net/url"
	"time"

	"github.com/gin-gonic/gin"
)

// MediaCrawlerProxyHandler 处理 MediaCrawler FastAPI 的反向代理
type MediaCrawlerProxyHandler struct {
	proxy     *httputil.ReverseProxy
	secretKey string // 用于签名验证的密钥
}

// NewMediaCrawlerProxyHandler 创建 MediaCrawler 代理处理器
func NewMediaCrawlerProxyHandler(fastAPIURL, secretKey string) *MediaCrawlerProxyHandler {
	target, err := url.Parse(fastAPIURL)
	if err != nil {
		panic("Invalid FastAPI URL: " + err.Error())
	}

	proxy := httputil.NewSingleHostReverseProxy(target)

	// 自定义 Director 来修改请求
	originalDirector := proxy.Director
	proxy.Director = func(req *http.Request) {
		originalDirector(req)
		req.Host = target.Host
		req.Header.Set("X-Forwarded-Host", req.Header.Get("Host"))

		// 添加签名头，证明请求来自 Go 后端
		timestamp := time.Now().Unix()
		signature := generateSignature(secretKey, timestamp)
		req.Header.Set("X-Proxy-Signature", signature)
		req.Header.Set("X-Proxy-Timestamp", fmt.Sprintf("%d", timestamp))
	}

	// 自定义错误处理
	proxy.ErrorHandler = func(w http.ResponseWriter, r *http.Request, err error) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusBadGateway)
		w.Write([]byte(`{"error": "FastAPI service unavailable", "detail": "` + err.Error() + `"}`))
	}

	return &MediaCrawlerProxyHandler{
		proxy:     proxy,
		secretKey: secretKey,
	}
}

// generateSignature 生成 HMAC-SHA256 签名
func generateSignature(secret string, timestamp int64) string {
	message := fmt.Sprintf("%d", timestamp)
	h := hmac.New(sha256.New, []byte(secret))
	h.Write([]byte(message))
	return hex.EncodeToString(h.Sum(nil))
}

// ProxyRequest 代理请求到 FastAPI
func (h *MediaCrawlerProxyHandler) ProxyRequest(c *gin.Context) {
	h.proxy.ServeHTTP(c.Writer, c.Request)
}
