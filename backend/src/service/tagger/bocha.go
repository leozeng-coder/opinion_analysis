package tagger

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"

	"opinion-analysis/src/service/tagger/pipeline"
)

// bochaEndpoint 博查 Web Search API。官方文档：https://open.bocha.cn
const bochaEndpoint = "https://api.bocha.cn/v1/web-search"

// bochaReq 博查搜索请求体。字段语义见官方文档「请求体」。
type bochaReq struct {
	Query     string `json:"query"`               // 用户搜索词（必填）
	Summary   bool   `json:"summary"`             // 是否返回正文摘要
	Count     int    `json:"count"`               // 返回结果数，1-50
	Freshness string `json:"freshness,omitempty"` // 时效：noLimit/oneDay/oneWeek/oneMonth/oneYear，或日期范围
}

// bochaResp 博查搜索响应体。成功时 code=200，失败由 HTTP 状态码先行拦截。
type bochaResp struct {
	Code int    `json:"code"`
	Msg  string `json:"msg"`
	Data struct {
		WebPages struct {
			Value []struct {
				Name     string `json:"name"`     // 网页标题
				URL      string `json:"url"`      // 网页 URL
				Snippet  string `json:"snippet"`  // 简短描述
				Summary  string `json:"summary"`  // 正文摘要（summary=true 时）
				SiteName string `json:"siteName"` // 网站名称
				// 发布时间：datePublished 常为空，需回退 dateLastCrawled（其实是发布时间，
				// 且博查把 UTC+8 误标为 Z 后缀，由 resolvePublished 统一纠正）。
				DatePublished   string `json:"datePublished"`
				DateLastCrawled string `json:"dateLastCrawled"`
			} `json:"value"`
		} `json:"webPages"`
	} `json:"data"`
}

// resolvePublished 选取可靠的发布时间。
// 优先 datePublished；为空时回退 dateLastCrawled，并把博查误标的 Z 后缀纠正为 +08:00（UTC+8）。
func resolvePublished(datePublished, dateLastCrawled string) string {
	if s := strings.TrimSpace(datePublished); s != "" {
		return s
	}
	s := strings.TrimSpace(dateLastCrawled)
	if s == "" {
		return ""
	}
	if strings.HasSuffix(s, "Z") {
		s = strings.TrimSuffix(s, "Z") + "+08:00"
	}
	return s
}

// newWebSearchFn 构造注入 pipeline 的联网搜索函数。apiKey 为空时返回 nil（表示工具不可用）。
func (s *Service) newWebSearchFn(apiKey string) pipeline.WebSearchFn {
	apiKey = strings.TrimSpace(apiKey)
	if apiKey == "" {
		return nil
	}
	return func(ctx context.Context, query string, count int) ([]pipeline.WebResult, error) {
		if count <= 0 {
			count = 5
		}
		if count > 50 {
			count = 50 // 博查单次上限
		}
		reqBody := bochaReq{
			Query:     query,
			Summary:   true,
			Count:     count,
			Freshness: "noLimit", // 官方推荐：让搜索算法自动改写时间范围，效果更佳
		}
		payload, _ := json.Marshal(reqBody)

		req, err := http.NewRequestWithContext(ctx, http.MethodPost, bochaEndpoint, bytes.NewReader(payload))
		if err != nil {
			return nil, err
		}
		req.Header.Set("Content-Type", "application/json")
		req.Header.Set("Authorization", "Bearer "+apiKey)

		resp, err := s.client.Do(req)
		if err != nil {
			return nil, err
		}
		defer resp.Body.Close()

		body, _ := io.ReadAll(resp.Body)
		if resp.StatusCode/100 != 2 {
			return nil, fmt.Errorf("bocha status=%d body=%s", resp.StatusCode, truncate(string(body), 300))
		}

		var br bochaResp
		if err := json.Unmarshal(body, &br); err != nil {
			return nil, fmt.Errorf("decode bocha response: %w", err)
		}
		if br.Code != 0 && br.Code != 200 {
			return nil, fmt.Errorf("bocha api code=%d msg=%s", br.Code, br.Msg)
		}

		out := make([]pipeline.WebResult, 0, len(br.Data.WebPages.Value))
		for _, v := range br.Data.WebPages.Value {
			summary := v.Summary
			if summary == "" {
				summary = v.Snippet
			}
			out = append(out, pipeline.WebResult{
				Title:     v.Name,
				URL:       v.URL,
				Summary:   summary,
				SiteName:  v.SiteName,
				Published: resolvePublished(v.DatePublished, v.DateLastCrawled),
			})
		}
		return out, nil
	}
}
