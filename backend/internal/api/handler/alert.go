package handler

import (
	"github.com/gin-gonic/gin"
	"opinion-analysis/internal/model"
	"opinion-analysis/pkg/response"
	"gorm.io/gorm"
	"strconv"
)

type AlertHandler struct {
	db *gorm.DB
}

func NewAlertHandler(db *gorm.DB) *AlertHandler {
	return &AlertHandler{db: db}
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
	var list []model.AlertRule
	if err := h.db.Find(&list).Error; err != nil {
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
	if err := h.db.Create(&rule).Error; err != nil {
		response.ServerError(c)
		return
	}
	response.OK(c, rule)
}

func (h *AlertHandler) DeleteRule(c *gin.Context) {
	id := c.Param("id")
	if err := h.db.Delete(&model.AlertRule{}, id).Error; err != nil {
		response.ServerError(c)
		return
	}
	response.OK(c, nil)
}

func (h *AlertHandler) ListRecords(c *gin.Context) {
	page, _ := strconv.Atoi(c.DefaultQuery("page", "1"))
	pageSize, _ := strconv.Atoi(c.DefaultQuery("pageSize", "20"))

	var total int64
	h.db.Model(&model.AlertRecord{}).Count(&total)

	var list []model.AlertRecord
	offset := (page - 1) * pageSize
	if err := h.db.Preload("Rule").Order("created_at desc").Offset(offset).Limit(pageSize).Find(&list).Error; err != nil {
		response.ServerError(c)
		return
	}
	response.OKPage(c, total, list)
}
