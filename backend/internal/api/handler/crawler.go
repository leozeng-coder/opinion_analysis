package handler

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"log"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strconv"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/gin-gonic/gin"
	"gorm.io/gorm"

	"opinion-analysis/config"
	"opinion-analysis/internal/model"
	"opinion-analysis/pkg/response"
)

var allowedSpiderKeys = map[string]struct{}{
	"broad-topic": {}, "deep-sentiment": {},
}

// defaultBasicSpiders 用于「立即运行全部」（不包含 deep-sentiment，避免没有关键词时跑空）
var defaultBasicSpiders = []string{"broad-topic"}

type CrawlerHandler struct {
	db *gorm.DB
}

func NewCrawlerHandler(db *gorm.DB) *CrawlerHandler {
	return &CrawlerHandler{db: db}
}

func (h *CrawlerHandler) ListSpiders(c *gin.Context) {
	var list []model.CrawlerSpiderConfig
	if err := h.db.Order("id").Find(&list).Error; err != nil {
		response.ServerError(c)
		return
	}
	response.OK(c, list)
}

type putSpidersBody struct {
	Spiders []struct {
		SpiderKey       string `json:"spiderKey" binding:"required"`
		IntervalMinutes int    `json:"intervalMinutes" binding:"required"`
		Enabled         int8   `json:"enabled" binding:"required"`
	} `json:"spiders" binding:"required"`
}

func (h *CrawlerHandler) PutSpiders(c *gin.Context) {
	var body putSpidersBody
	if err := c.ShouldBindJSON(&body); err != nil {
		response.Fail(c, 400, err.Error())
		return
	}
	for _, it := range body.Spiders {
		k := strings.ToLower(strings.TrimSpace(it.SpiderKey))
		if _, ok := allowedSpiderKeys[k]; !ok {
			response.Fail(c, 400, "unknown spider: "+k)
			return
		}
		if it.IntervalMinutes < 1 || it.IntervalMinutes > 10080 {
			response.Fail(c, 400, "intervalMinutes must be between 1 and 10080")
			return
		}
		if it.Enabled != 0 && it.Enabled != 1 {
			response.Fail(c, 400, "enabled must be 0 or 1")
			return
		}
		if err := h.db.Model(&model.CrawlerSpiderConfig{}).Where("spider_key = ?", k).Updates(map[string]interface{}{
			"interval_minutes": it.IntervalMinutes,
			"enabled":          it.Enabled,
		}).Error; err != nil {
			response.ServerError(c)
			return
		}
	}
	var list []model.CrawlerSpiderConfig
	if err := h.db.Order("id").Find(&list).Error; err != nil {
		response.ServerError(c)
		return
	}
	response.OK(c, list)
}

type runNowBody struct {
	Spiders  []string `json:"spiders"`
	Keywords []string `json:"keywords"`
	Topics   []string `json:"topics"`
	StartAt  string   `json:"startAt"` // RFC3339 / ISO8601；为空表示不限
	EndAt    string   `json:"endAt"`
}

// crawlerFilter 是落到日志和子进程的过滤参数
type crawlerFilter struct {
	Keywords []string `json:"keywords,omitempty"`
	Topics   []string `json:"topics,omitempty"`
	StartAt  string   `json:"startAt,omitempty"`
	EndAt    string   `json:"endAt,omitempty"`
}

func (h *CrawlerHandler) RunNow(c *gin.Context) {
	if !config.Cfg.Crawler.Enabled {
		response.Fail(c, 4003, "crawler trigger is disabled (set crawler.enabled=true and ensure Python venv exists)")
		return
	}
	root, py, err := resolveCrawlerExec()
	if err != nil {
		response.Fail(c, 4003, err.Error())
		return
	}
	var body runNowBody
	if c.Request.ContentLength > 0 {
		if err := c.ShouldBindJSON(&body); err != nil {
			response.Fail(c, 400, err.Error())
			return
		}
	}
	filter := normalizeFilter(body)
	hasFilter := len(filter.Keywords) > 0 || len(filter.Topics) > 0 || filter.StartAt != "" || filter.EndAt != ""

	keys := normalizeSpiderKeys(body.Spiders, hasFilter)
	if len(keys) == 0 {
		response.Fail(c, 400, "no valid spiders (allowed: broad-topic, deep-sentiment)")
		return
	}
	if hasFilter && filter.StartAt != "" && filter.EndAt != "" {
		st, e1 := time.Parse(time.RFC3339, filter.StartAt)
		ed, e2 := time.Parse(time.RFC3339, filter.EndAt)
		if e1 == nil && e2 == nil && st.After(ed) {
			response.Fail(c, 400, "startAt must not be after endAt")
			return
		}
	}

	userID, ok := c.Get("userID")
	if !ok {
		response.ServerError(c)
		return
	}
	uid, ok := userID.(uint)
	if !ok {
		response.ServerError(c)
		return
	}
	spiderArg := strings.Join(keys, ",")
	mode := "basic"
	paramsJSON := "{}"
	if hasFilter {
		mode = "advanced"
		b, _ := json.Marshal(filter)
		paramsJSON = string(b)
	}
	logRow := model.CrawlerRunLog{
		Spiders:        spiderArg,
		Mode:           mode,
		Params:         paramsJSON,
		Status:         "running",
		Progress:       0,
		ProgressDetail: fmt.Sprintf(`{"phase":"queued","totalSpiders":%d}`, len(keys)),
		TriggeredBy:    uid,
		StartedAt:      time.Now(),
	}
	if err := h.db.Create(&logRow).Error; err != nil {
		log.Printf("crawler: create run log failed: %v", err)
		response.ServerError(c)
		return
	}
	hasFilterPayload := paramsJSON != "" && paramsJSON != "{}"
	log.Printf("[crawler task] run_created run_id=%d mode=%s spiders=%s user_id=%d has_filter=%v",
		logRow.ID, mode, spiderArg, uid, hasFilterPayload)
	go h.runSubprocess(logRow.ID, root, py, spiderArg, paramsJSON)

	response.OK(c, gin.H{"id": logRow.ID})
}

func (h *CrawlerHandler) runSubprocess(logID uint, root, py, spiders, filterJSON string) {
	t0 := time.Now()
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Minute)
	defer cancel()

	var filterFile string
	hasFilterPayload := filterJSON != "" && filterJSON != "{}"
	if hasFilterPayload {
		tmp, err := os.CreateTemp("", "crawler-filter-*.json")
		if err != nil {
			log.Printf("[crawler task] filter_temp_create_failed run_id=%d err=%v", logID, err)
		} else {
			if _, werr := tmp.Write([]byte(filterJSON)); werr != nil {
				log.Printf("[crawler task] filter_temp_write_failed run_id=%d err=%v", logID, werr)
				_ = tmp.Close()
				_ = os.Remove(tmp.Name())
			} else {
				_ = tmp.Close()
				filterFile = tmp.Name()
				defer func() { _ = os.Remove(filterFile) }()
			}
		}
	}

	log.Printf("[crawler task] subprocess_start run_id=%d python=%s crawler_root=%s spiders=%s filter_file=%v",
		logID, py, root, spiders, filterFile != "")

	script := filepath.Join(root, "run_once.py")
	cmd := exec.CommandContext(ctx, py, script, "--spiders", spiders)
	cmd.Dir = root
	env := append(os.Environ(), "DATABASE_DSN="+config.Cfg.Database.DSN)
	env = append(env, fmt.Sprintf("CRAWLER_RUN_LOG_ID=%d", logID))
	env = append(env, "CRAWLER_SPIDER_NAMES="+spiders)
	env = append(env, "PYTHONIOENCODING=utf-8")
	env = append(env, "PYTHONUTF8=1")
	if filterFile != "" {
		env = append(env, "CRAWLER_FILTER_FILE="+filterFile)
	} else if hasFilterPayload {
		log.Printf("[crawler task] filter_file_missing run_id=%d (advanced filter not passed to child; check temp file errors)", logID)
	}
	cmd.Env = env
	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr
	err := cmd.Run()
	elapsed := time.Since(t0)
	now := time.Now()
	msg := buildRunMessage(stdout.String(), stderr.String(), err)
	status := "success"
	if err != nil {
		status = "failed"
		if ctx.Err() == context.DeadlineExceeded {
			msg = "爬虫进程超时（超过10分钟）\n" + msg
		}
	}
	finish := map[string]interface{}{
		"status":            status,
		"message":           msg,
		"finished_at":       &now,
		"progress":          100,
		"progress_detail":   `{"phase":"failed"}`,
	}
	if status == "success" {
		finish["progress_detail"] = `{"phase":"finished"}`
	}
	tx := h.db.Model(&model.CrawlerRunLog{}).Where("id = ? AND status = ?", logID, "running").Updates(finish)
	if tx.Error != nil {
		log.Printf("[crawler task] subprocess_finish_db_error run_id=%d status=%s elapsed=%v err=%v", logID, status, elapsed, tx.Error)
	} else if tx.RowsAffected == 0 {
		log.Printf("[crawler task] subprocess_finish_skip run_id=%d status=%s elapsed=%v (row not in running)", logID, status, elapsed)
	} else {
		log.Printf("[crawler task] subprocess_finish run_id=%d status=%s elapsed=%v", logID, status, elapsed)
	}
}

func (h *CrawlerHandler) GetRun(c *gin.Context) {
	id, err := strconv.ParseUint(c.Param("id"), 10, 32)
	if err != nil || id == 0 {
		response.Fail(c, 400, "invalid id")
		return
	}
	var row model.CrawlerRunLog
	if err := h.db.First(&row, uint(id)).Error; err != nil {
		if err == gorm.ErrRecordNotFound {
			response.Fail(c, 404, "not found")
			return
		}
		response.ServerError(c)
		return
	}
	response.OK(c, row)
}

func (h *CrawlerHandler) GetRunProgress(c *gin.Context) {
	id, err := strconv.ParseUint(c.Param("id"), 10, 32)
	if err != nil || id == 0 {
		response.Fail(c, 400, "invalid id")
		return
	}
	var row model.CrawlerRunLog
	if err := h.db.First(&row, uint(id)).Error; err != nil {
		if err == gorm.ErrRecordNotFound {
			response.Fail(c, 404, "not found")
			return
		}
		response.ServerError(c)
		return
	}
	var detail any
	raw := row.ProgressDetail
	if raw != "" {
		if err := json.Unmarshal([]byte(raw), &detail); err != nil {
			log.Printf("crawler: progress_detail json id=%d: %v", row.ID, err)
			detail = nil
		}
	}
	response.OK(c, gin.H{
		"id":              row.ID,
		"status":          row.Status,
		"progress":        row.Progress,
		"detail":          detail,
		"progressDetail": raw,
	})
}

func (h *CrawlerHandler) ListRuns(c *gin.Context) {
	page, _ := strconv.Atoi(c.DefaultQuery("page", "1"))
	pageSize, _ := strconv.Atoi(c.DefaultQuery("pageSize", "20"))
	if page < 1 {
		page = 1
	}
	if pageSize < 1 || pageSize > 100 {
		pageSize = 20
	}
	var total int64
	if err := h.db.Model(&model.CrawlerRunLog{}).Count(&total).Error; err != nil {
		response.ServerError(c)
		return
	}
	var list []model.CrawlerRunLog
	offset := (page - 1) * pageSize
	if err := h.db.Order("id desc").Offset(offset).Limit(pageSize).Find(&list).Error; err != nil {
		response.ServerError(c)
		return
	}
	response.OKPage(c, total, list)
}

func resolveCrawlerExec() (root, python string, err error) {
	wd, err := os.Getwd()
	if err != nil {
		return "", "", err
	}
	root = filepath.Clean(filepath.Join(wd, config.Cfg.Crawler.Root))
	if st, e := os.Stat(root); e != nil || !st.IsDir() {
		return "", "", fmt.Errorf("crawler root not found: %s", root)
	}
	if config.Cfg.Crawler.Python != "" {
		python = config.Cfg.Crawler.Python
	} else if runtime.GOOS == "windows" {
		python = filepath.Join(root, ".venv", "Scripts", "python.exe")
	} else {
		python = filepath.Join(root, ".venv", "bin", "python3")
		if _, e := os.Stat(python); e != nil {
			python = filepath.Join(root, ".venv", "bin", "python")
		}
	}
	if _, e := os.Stat(python); e != nil {
		return "", "", fmt.Errorf("Python not found at %s (create MindSpider-main/.venv first)", python)
	}
	script := filepath.Join(root, "run_once.py")
	if _, e := os.Stat(script); e != nil {
		return "", "", fmt.Errorf("missing %s", script)
	}
	return root, python, nil
}

func normalizeSpiderKeys(in []string, hasFilter bool) []string {
	if len(in) == 0 {
		if hasFilter {
			return []string{"deep-sentiment"}
		}
		return append([]string(nil), defaultBasicSpiders...)
	}
	seen := map[string]struct{}{}
	var out []string
	for _, s := range in {
		k := strings.ToLower(strings.TrimSpace(s))
		if k == "" {
			continue
		}
		if _, ok := allowedSpiderKeys[k]; !ok {
			return nil
		}
		if _, dup := seen[k]; dup {
			continue
		}
		seen[k] = struct{}{}
		out = append(out, k)
	}
	return out
}

const filterListMax = 32
const filterTermLen = 64

func normalizeFilter(body runNowBody) crawlerFilter {
	return crawlerFilter{
		Keywords: cleanTermList(body.Keywords),
		Topics:   cleanTermList(body.Topics),
		StartAt:  strings.TrimSpace(body.StartAt),
		EndAt:    strings.TrimSpace(body.EndAt),
	}
}

func cleanTermList(in []string) []string {
	if len(in) == 0 {
		return nil
	}
	seen := map[string]struct{}{}
	out := make([]string, 0, len(in))
	for _, s := range in {
		t := strings.TrimSpace(s)
		if t == "" {
			continue
		}
		if utf8.RuneCountInString(t) > filterTermLen {
			runes := []rune(t)
			t = string(runes[:filterTermLen])
		}
		if _, dup := seen[t]; dup {
			continue
		}
		seen[t] = struct{}{}
		out = append(out, t)
		if len(out) >= filterListMax {
			break
		}
	}
	return out
}

const runLogMessageMax = 12000

func buildRunMessage(out, errOut string, runErr error) string {
	var b strings.Builder
	if runErr != nil {
		b.WriteString("exit: ")
		b.WriteString(runErr.Error())
		b.WriteString("\n")
	}
	if errOut != "" {
		b.WriteString("stderr:\n")
		b.WriteString(errOut)
		b.WriteString("\n")
	}
	if out != "" {
		b.WriteString("stdout:\n")
		b.WriteString(out)
	}
	s := strings.TrimSpace(b.String())
	if s == "" {
		s = "done"
	}
	if utf8.RuneCountInString(s) <= runLogMessageMax {
		return s
	}
	runes := []rune(s)
	return string(runes[:runLogMessageMax]) + "\n…(truncated)"
}

const staleRunMessage = "服务重启或进程异常中断，本条任务已与运行进程失联，已标记为失败。请重新发起抓取。"

// RecoverStaleCrawlerRuns 将仍为 running 且未结束的记录收口为 failed，解除重启后前端长期「运行中」的问题。
func RecoverStaleCrawlerRuns(db *gorm.DB) (int64, error) {
	now := time.Now()
	tx := db.Model(&model.CrawlerRunLog{}).
		Where("status = ? AND finished_at IS NULL", "running").
		Updates(map[string]interface{}{
			"status":          "failed",
			"message":         staleRunMessage,
			"finished_at":     &now,
			"progress":        100,
			"progress_detail": `{"phase":"failed","reason":"orphaned_run"}`,
		})
	return tx.RowsAffected, tx.Error
}
