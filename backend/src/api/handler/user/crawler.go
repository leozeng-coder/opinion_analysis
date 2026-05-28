package user

import (
	"strconv"
	"strings"

	"github.com/gin-gonic/gin"

	"opinion-analysis/pkg/response"
	"opinion-analysis/src/repository"
	"opinion-analysis/src/service/alertengine"
)

var allowedSpiderKeys = map[string]struct{}{
	"broad-topic": {}, "deep-sentiment": {},
}

// defaultBasicSpiders 用于「立即运行全部」（不包含 deep-sentiment，避免没有关键词时跑空）
var defaultBasicSpiders = []string{"broad-topic"}

type CrawlerHandler struct {
	crawler *repository.CrawlerRepository
	alerts  *alertengine.Engine
}

func NewCrawlerHandler(store *repository.Store, alerts *alertengine.Engine) *CrawlerHandler {
	return &CrawlerHandler{crawler: store.Crawler, alerts: alerts}
}

func (h *CrawlerHandler) ListSpiders(c *gin.Context) {
	list, err := h.crawler.ListSpiderConfigs()
	if err != nil {
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
		if err := h.crawler.UpdateSpiderConfig(k, it.IntervalMinutes, it.Enabled); err != nil {
			response.ServerError(c)
			return
		}
	}
	list, err := h.crawler.ListSpiderConfigs()
	if err != nil {
		response.ServerError(c)
		return
	}
	response.OK(c, list)
}

func (h *CrawlerHandler) RunNow(c *gin.Context) {
	// 已废弃：现在通过 MediaCrawler FastAPI 代理处理
	response.Fail(c, 410, "This endpoint is deprecated. Use MediaCrawler API proxy instead.")
}

func (h *CrawlerHandler) GetRun(c *gin.Context) {
	idStr := c.Param("id")
	id, err := strconv.ParseUint(idStr, 10, 32)
	if err != nil {
		response.Fail(c, 400, "invalid id")
		return
	}
	row, err := h.crawler.FindRunLogByID(uint(id))
	if err != nil {
		response.Fail(c, 404, "run not found")
		return
	}
	response.OK(c, row)
}

func (h *CrawlerHandler) GetRunProgress(c *gin.Context) {
	idStr := c.Param("id")
	id, err := strconv.ParseUint(idStr, 10, 32)
	if err != nil {
		response.Fail(c, 400, "invalid id")
		return
	}
	row, err := h.crawler.FindRunLogByID(uint(id))
	if err != nil {
		response.Fail(c, 404, "run not found")
		return
	}
	response.OK(c, gin.H{
		"id":             row.ID,
		"status":         row.Status,
		"progress":       row.Progress,
		"progressDetail": row.ProgressDetail,
		"message":        row.Message,
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
	list, total, err := h.crawler.ListRunLogs(page, pageSize)
	if err != nil {
		response.ServerError(c)
		return
	}
	response.OKPage(c, total, list)
}
