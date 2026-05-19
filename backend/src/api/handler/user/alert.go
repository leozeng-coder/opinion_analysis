package user

import (
	"strconv"
	"time"

	"github.com/gin-gonic/gin"
	"opinion-analysis/pkg/response"
	"opinion-analysis/src/model"
	"opinion-analysis/src/repository"
)

type AlertHandler struct {
	alerts *repository.AlertRepository
}

func NewAlertHandler(store *repository.Store) *AlertHandler {
	return &AlertHandler{alerts: store.Alert}
}

type createRuleReq struct {
	Name       string `json:"name" binding:"required"`
	Keywords   string `json:"keywords"`
	Sentiment  string `json:"sentiment"`
	Threshold  int    `json:"threshold"`
	Interval   int    `json:"interval"`
	NotifyType string `json:"notifyType"`
	NotifyConf string `json:"notifyConf"`
}

func (h *AlertHandler) ListRules(c *gin.Context) {
	list, err := h.alerts.ListRules()
	if err != nil {
		response.ServerError(c)
		return
	}
	response.OK(c, list)
}

func (h *AlertHandler) CreateRule(c *gin.Context) {
	var req createRuleReq
	if err := c.ShouldBindJSON(&req); err != nil {
		response.Fail(c, 400, err.Error())
		return
	}
	userID, _ := c.Get("userID")
	rule := model.AlertRule{
		Name:       req.Name,
		Keywords:   req.Keywords,
		Sentiment:  req.Sentiment,
		Threshold:  req.Threshold,
		Interval:   req.Interval,
		NotifyType: req.NotifyType,
		NotifyConf: req.NotifyConf,
		CreatedBy:  userID.(uint),
	}
	if err := h.alerts.CreateRule(&rule); err != nil {
		response.ServerError(c)
		return
	}
	response.OK(c, rule)
}

func (h *AlertHandler) DeleteRule(c *gin.Context) {
	if err := h.alerts.DeleteRule(c.Param("id")); err != nil {
		response.ServerError(c)
		return
	}
	response.OK(c, nil)
}

func (h *AlertHandler) ListRecords(c *gin.Context) {
	page, _ := strconv.Atoi(c.DefaultQuery("page", "1"))
	pageSize, _ := strconv.Atoi(c.DefaultQuery("pageSize", "20"))

	var startAt *time.Time
	if s := c.Query("startAt"); s != "" {
		if t, err := time.Parse(time.RFC3339, s); err == nil {
			startAt = &t
		}
	}

	list, total, err := h.alerts.ListRecords(page, pageSize, startAt)
	if err != nil {
		response.ServerError(c)
		return
	}
	response.OKPage(c, total, list)
}
