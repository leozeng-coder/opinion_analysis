package crawler

import (
	"bytes"
	"context"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net/http"
	"time"

	"opinion-analysis/config"
	"opinion-analysis/src/model"
	"opinion-analysis/src/repository"
)

// Service 爬虫服务
type Service struct {
	repo           *repository.CrawlerRepository
	httpClient     *http.Client
	apiBaseURL     string
	proxySecretKey string
}

// NewService 创建爬虫服务
func NewService(repo *repository.CrawlerRepository) *Service {
	apiURL := config.Cfg.Crawler.ApiURL
	if apiURL == "" {
		apiURL = "http://127.0.0.1:8085" // 默认端口
	}

	secretKey := config.Cfg.Crawler.ProxySecretKey
	if secretKey == "" {
		secretKey = "your-secret-key-change-in-production"
	}

	return &Service{
		repo:           repo,
		httpClient:     &http.Client{Timeout: 30 * time.Second},
		apiBaseURL:     apiURL,
		proxySecretKey: secretKey,
	}
}

// TriggerParams 触发爬虫的参数
type TriggerParams struct {
	Spiders           []string // 兼容旧工作流；优先使用 Platform
	Platform          string   // 单平台代码（xhs/dy/ks/bili/wb/tieba/zhihu）
	CrawlerType       string   // search / detail / creator，留空默认 search
	Keywords          []string
	SpecifiedIds      string // detail 模式，逗号分隔
	CreatorIds        string // creator 模式，逗号分隔
	LoginType         string // cookie / qrcode，留空默认 cookie
	SaveOption        string // db / json / csv / ..，留空默认 db
	StartPage         int    // 起始页，0 表示使用默认值 1
	EnableComments    bool
	EnableSubComments bool
	Headless          bool // 无头模式；建议工作流节点默认 true
	Topics            []string
	StartAt           string
	EndAt             string
	TimeoutMinutes    int
}

type mediaCrawlerStatusResponse struct {
	Status       string  `json:"status"`
	ErrorMessage *string `json:"error_message"`
}

// TriggerResult 触发结果
type TriggerResult struct {
	RunID     uint      `json:"runId"`
	Spiders   []string  `json:"spiders"`
	Status    string    `json:"status"`
	StartedAt time.Time `json:"startedAt"`
}

// MediaCrawlerStartRequest MediaCrawler API 请求格式
type MediaCrawlerStartRequest struct {
	Platform          string `json:"platform"`
	LoginType         string `json:"login_type"`
	CrawlerType       string `json:"crawler_type"`
	Keywords          string `json:"keywords"`
	SpecifiedIds      string `json:"specified_ids,omitempty"`
	CreatorIds        string `json:"creator_ids,omitempty"`
	StartPage         int    `json:"start_page"`
	SaveOption        string `json:"save_option"`
	Headless          bool   `json:"headless"`
	EnableComments    bool   `json:"enable_comments"`
	EnableSubComments bool   `json:"enable_sub_comments"`
}

// Trigger 触发爬虫任务
func (s *Service) Trigger(ctx context.Context, params TriggerParams) (*TriggerResult, error) {
	// 默认使用知乎
	if len(params.Spiders) == 0 {
		params.Spiders = []string{"zhihu"}
	}

	// 构建过滤参数
	filter := make(map[string]interface{})
	if len(params.Keywords) > 0 {
		filter["keywords"] = params.Keywords
	}
	if len(params.Topics) > 0 {
		filter["topics"] = params.Topics
	}
	if params.StartAt != "" {
		filter["startAt"] = params.StartAt
	}
	if params.EndAt != "" {
		filter["endAt"] = params.EndAt
	}

	mode := "basic"
	paramsJSON := "{}"
	if len(filter) > 0 {
		mode = "advanced"
		b, _ := json.Marshal(filter)
		paramsJSON = string(b)
	}

	spiderArg := ""
	for i, s := range params.Spiders {
		if i > 0 {
			spiderArg += ","
		}
		spiderArg += s
	}

	// 创建运行日志
	logRow := model.CrawlerRunLog{
		Spiders:        spiderArg,
		Mode:           mode,
		Params:         paramsJSON,
		Status:         "running",
		Progress:       0,
		ProgressDetail: fmt.Sprintf(`{"phase":"queued","totalSpiders":%d}`, len(params.Spiders)),
		TriggeredBy:    0, // 工作流触发
		StartedAt:      time.Now(),
	}

	if err := s.repo.CreateRunLog(&logRow); err != nil {
		return nil, fmt.Errorf("failed to create run log: %w", err)
	}

	log.Printf("[CrawlerService] Triggered crawler task: runId=%d, spiders=%s, mode=%s",
		logRow.ID, spiderArg, mode)

	// 调用 MediaCrawler API（后台执行，用独立 background context，避免 HTTP 请求 ctx 结束就断掉）
	// 注意：工作流取消走 WaitForCompletion 的 ctx，会显式调 StopCrawler 来停 MediaCrawler
	if config.Cfg.Crawler.Enabled {
		log.Printf("[CrawlerService] Calling MediaCrawler API for runId=%d", logRow.ID)
		timeoutMinutes := params.TimeoutMinutes
		if timeoutMinutes <= 0 {
			timeoutMinutes = 30
		}
		bgCtx := context.Background()
		go s.callMediaCrawlerAPI(bgCtx, logRow.ID, params, time.Duration(timeoutMinutes)*time.Minute)
	} else {
		log.Printf("[CrawlerService] Crawler is disabled in config")
		s.finishRunLog(logRow.ID, "failed", "Crawler is disabled in configuration")
	}

	return &TriggerResult{
		RunID:     logRow.ID,
		Spiders:   params.Spiders,
		Status:    "running",
		StartedAt: logRow.StartedAt,
	}, nil
}

// callMediaCrawlerAPI 调用 MediaCrawler FastAPI 并等待爬虫进程结束（数据同步由 platform_sync 节点负责）
func (s *Service) callMediaCrawlerAPI(ctx context.Context, logID uint, params TriggerParams, timeout time.Duration) {
	t0 := time.Now()

	// 平台：优先使用 Platform 字段（新节点），兜底取 Spiders[0]（旧工作流兼容）
	platform := params.Platform
	if platform == "" && len(params.Spiders) > 0 {
		platform = s.mapSpiderToPlatform(params.Spiders[0])
	}
	if platform == "" {
		platform = "zhihu"
	}

	// 爬取类型
	crawlerType := params.CrawlerType
	if crawlerType == "" {
		crawlerType = "search"
	}

	// 登录方式
	loginType := params.LoginType
	if loginType == "" {
		loginType = "cookie"
	}

	// 存储方式
	saveOption := params.SaveOption
	if saveOption == "" {
		saveOption = "db"
	}

	// 起始页
	startPage := params.StartPage
	if startPage <= 0 {
		startPage = 1
	}

	// 合并关键词
	keywords := ""
	if len(params.Keywords) > 0 {
		keywords = params.Keywords[0]
		for i := 1; i < len(params.Keywords); i++ {
			keywords += " " + params.Keywords[i]
		}
	}

	// 构建请求
	reqBody := MediaCrawlerStartRequest{
		Platform:          platform,
		LoginType:         loginType,
		CrawlerType:       crawlerType,
		Keywords:          keywords,
		SpecifiedIds:      params.SpecifiedIds,
		CreatorIds:        params.CreatorIds,
		StartPage:         startPage,
		SaveOption:        saveOption,
		Headless:          params.Headless,
		EnableComments:    params.EnableComments,
		EnableSubComments: params.EnableSubComments,
	}

	jsonData, _ := json.Marshal(reqBody)
	log.Printf("[CrawlerService] Request to MediaCrawler API: POST %s/api/crawler/start, body: %s", s.apiBaseURL, string(jsonData))

	req, err := http.NewRequest("POST", s.apiBaseURL+"/api/crawler/start", bytes.NewBuffer(jsonData))
	if err != nil {
		log.Printf("[CrawlerService] Failed to create request: %v", err)
		s.finishRunLog(logID, "failed", fmt.Sprintf("Failed to create request: %v", err))
		return
	}

	req.Header.Set("Content-Type", "application/json")
	s.setProxyAuthHeaders(req)

	// 发送请求
	resp, err := s.httpClient.Do(req)
	if err != nil {
		log.Printf("[CrawlerService] Failed to call MediaCrawler API: %v", err)
		s.finishRunLog(logID, "failed", fmt.Sprintf("Failed to call MediaCrawler API: %v", err))
		return
	}
	defer resp.Body.Close()

	body, _ := io.ReadAll(resp.Body)
	elapsed := time.Since(t0)

	if resp.StatusCode != 200 {
		log.Printf("[CrawlerService] MediaCrawler API returned error: status=%d, body=%s", resp.StatusCode, string(body))
		s.finishRunLog(logID, "failed", fmt.Sprintf("MediaCrawler API error (status %d): %s", resp.StatusCode, string(body)))
		return
	}

	log.Printf("[CrawlerService] MediaCrawler API call succeeded: runId=%d, elapsed=%v, waiting for completion...", logID, elapsed)

	if err := s.waitMediaCrawlerIdle(timeout); err != nil {
		log.Printf("[CrawlerService] MediaCrawler did not finish: %v", err)
		s.finishRunLog(logID, "failed", err.Error())
		return
	}

	s.finishRunLog(logID, "success", "Crawler completed successfully via MediaCrawler API")
}

func (s *Service) waitMediaCrawlerIdle(timeout time.Duration) error {
	deadline := time.Now().Add(timeout)

	runningDeadline := time.Now().Add(30 * time.Second)
	sawRunning := false
	for !sawRunning && time.Now().Before(runningDeadline) {
		status, err := s.fetchMediaCrawlerStatus()
		if err != nil {
			log.Printf("[CrawlerService] Failed to fetch MediaCrawler status: %v", err)
		} else if status.Status == "running" || status.Status == "stopping" {
			sawRunning = true
			log.Printf("[CrawlerService] MediaCrawler entered %s state", status.Status)
			break
		} else if status.Status == "error" {
			return fmt.Errorf("MediaCrawler error: %s", mediaCrawlerErrorMessage(status))
		}
		time.Sleep(2 * time.Second)
	}
	if !sawRunning {
		return fmt.Errorf("MediaCrawler did not enter running state within 30s")
	}

	for time.Now().Before(deadline) {
		status, err := s.fetchMediaCrawlerStatus()
		if err != nil {
			log.Printf("[CrawlerService] Failed to fetch MediaCrawler status: %v", err)
		} else {
			switch status.Status {
			case "idle":
				log.Printf("[CrawlerService] MediaCrawler finished (idle)")
				return nil
			case "error":
				return fmt.Errorf("MediaCrawler error: %s", mediaCrawlerErrorMessage(status))
			case "running", "stopping":
				log.Printf("[CrawlerService] MediaCrawler still %s...", status.Status)
			}
		}
		time.Sleep(5 * time.Second)
	}

	return fmt.Errorf("timeout waiting for MediaCrawler to finish")
}

func mediaCrawlerErrorMessage(status *mediaCrawlerStatusResponse) string {
	if status.ErrorMessage != nil && *status.ErrorMessage != "" {
		return *status.ErrorMessage
	}
	return "unknown error"
}

// StopCrawler 向 MediaCrawler 发送停止信号并标记 run log 为 cancelled。
// 设计为幂等：若 MediaCrawler 已 idle，stop 请求会被 400 拒绝，直接忽略。
func (s *Service) StopCrawler(runID uint) {
	log.Printf("[CrawlerService] Stopping MediaCrawler for runId=%d", runID)

	req, err := http.NewRequest("POST", s.apiBaseURL+"/api/crawler/stop", nil)
	if err != nil {
		log.Printf("[CrawlerService] Failed to build stop request: %v", err)
	} else {
		s.setProxyAuthHeaders(req)
		resp, err := s.httpClient.Do(req)
		if err != nil {
			log.Printf("[CrawlerService] Stop request failed: %v", err)
		} else {
			body, _ := io.ReadAll(resp.Body)
			resp.Body.Close()
			log.Printf("[CrawlerService] Stop response: status=%d body=%s", resp.StatusCode, string(body))
		}
	}

	if runID > 0 {
		s.finishRunLog(runID, "failed", "workflow cancelled by user")
	}
}

func (s *Service) fetchMediaCrawlerStatus() (*mediaCrawlerStatusResponse, error) {
	req, err := http.NewRequest("GET", s.apiBaseURL+"/api/crawler/status", nil)
	if err != nil {
		return nil, err
	}
	s.setProxyAuthHeaders(req)

	resp, err := s.httpClient.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	body, _ := io.ReadAll(resp.Body)
	if resp.StatusCode != 200 {
		return nil, fmt.Errorf("status API returned %d: %s", resp.StatusCode, string(body))
	}

	var status mediaCrawlerStatusResponse
	if err := json.Unmarshal(body, &status); err != nil {
		return nil, err
	}
	return &status, nil
}

func (s *Service) setProxyAuthHeaders(req *http.Request) {
	timestamp := time.Now().Unix()
	req.Header.Set("X-Proxy-Timestamp", fmt.Sprintf("%d", timestamp))
	req.Header.Set("X-Proxy-Signature", s.generateProxySignature(timestamp))
}

func (s *Service) generateProxySignature(timestamp int64) string {
	message := fmt.Sprintf("%d", timestamp)
	h := hmac.New(sha256.New, []byte(s.proxySecretKey))
	h.Write([]byte(message))
	return hex.EncodeToString(h.Sum(nil))
}

// mapSpiderToPlatform 映射工作流平台标识到 MediaCrawler API 平台代码
func (s *Service) mapSpiderToPlatform(spider string) string {
	mapping := map[string]string{
		"broad-topic": "zhihu", "deep-sentiment": "zhihu", "zhihu": "zhihu",
		"xiaohongshu": "xhs", "xhs": "xhs",
		"douyin": "dy", "dy": "dy",
		"kuaishou": "ks", "ks": "ks",
		"bilibili": "bili", "bili": "bili",
		"weibo": "wb", "wb": "wb",
		"tieba": "tieba",
	}
	if code, ok := mapping[spider]; ok {
		return code
	}
	return "zhihu"
}

// finishRunLog 完成运行日志
func (s *Service) finishRunLog(logID uint, status, message string) {
	now := time.Now()
	finish := map[string]interface{}{
		"status":      status,
		"message":     message,
		"finished_at": &now,
		"progress":    100,
	}
	if _, err := s.repo.FinishRunLog(logID, finish); err != nil {
		log.Printf("[CrawlerService] Failed to finish run log: %v", err)
	}
}

// GetRunStatus 获取运行状态
func (s *Service) GetRunStatus(ctx context.Context, runID uint) (*model.CrawlerRunLog, error) {
	return s.repo.FindRunLogByID(runID)
}

// WaitForCompletion 等待爬虫完成
func (s *Service) WaitForCompletion(ctx context.Context, runID uint, timeout time.Duration) error {
	deadline := time.Now().Add(timeout)

	for {
		if time.Now().After(deadline) {
			return fmt.Errorf("timeout waiting for crawler completion")
		}

		runLog, err := s.repo.FindRunLogByID(runID)
		if err != nil {
			return fmt.Errorf("failed to get run status: %w", err)
		}

		if runLog.Status == "success" {
			log.Printf("[CrawlerService] Crawler completed successfully: runId=%d", runID)
			return nil
		}

		if runLog.Status == "failed" {
			return fmt.Errorf("crawler failed: %s", runLog.Message)
		}

		// 检查 context 是否取消
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-time.After(5 * time.Second):
			// 继续轮询
			log.Printf("[CrawlerService] Waiting for crawler: runId=%d, status=%s, progress=%d%%",
				runID, runLog.Status, runLog.Progress)
		}
	}
}

