package user

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"strconv"
	"strings"
	"time"

	"github.com/gin-gonic/gin"
	"opinion-analysis/pkg/response"
	"opinion-analysis/src/model"
	"opinion-analysis/src/repository"
	"opinion-analysis/src/service/alertengine"
)

type AlertHandler struct {
	alerts *repository.AlertRepository
	engine *alertengine.Engine
}

func NewAlertHandler(store *repository.Store, engine *alertengine.Engine) *AlertHandler {
	return &AlertHandler{alerts: store.Alert, engine: engine}
}

type alertRuleReq struct {
	Name           string   `json:"name" binding:"required"`
	Remark         string   `json:"remark"`
	KeywordListAnd []string `json:"keywordListAnd"`
	KeywordListOr  []string `json:"keywordListOr"`
	Sentiment      string   `json:"sentiment"`
	Threshold      int      `json:"threshold"`
	Interval       int      `json:"interval"`
	TimeRangeDays  *int     `json:"timeRangeDays"`
	NotifyType     string   `json:"notifyType"`
	NotifyEmail    string   `json:"notifyEmail"`
	NotifyWebhook  string   `json:"notifyWebhook"`
	NotifyPhone    string   `json:"notifyPhone"`
	NotifyConf     string   `json:"notifyConf"`
	Status         *int8    `json:"status"`
}

func normalizeSentiment(raw string) string {
	s := strings.TrimSpace(raw)
	if s == "" || s == "all" {
		return ""
	}
	return s
}

func normalizeKeywordList(list []string) string {
	parts := make([]string, 0, len(list))
	for _, k := range list {
		if k = strings.TrimSpace(k); k != "" {
			parts = append(parts, k)
		}
	}
	if len(parts) == 0 {
		return "[]"
	}
	b, _ := json.Marshal(parts)
	return string(b)
}

func normalizeAlertNotifyConf(raw string) string {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return "{}"
	}
	if json.Valid([]byte(raw)) {
		return raw
	}
	b, _ := json.Marshal(map[string]string{"value": raw})
	return string(b)
}

func ruleFieldsFromReq(req alertRuleReq) (map[string]interface{}, error) {
	if req.Threshold <= 0 {
		req.Threshold = 10
	}
	if req.Interval <= 0 {
		req.Interval = 60
	}
	if strings.TrimSpace(req.NotifyType) == "" {
		return nil, fmt.Errorf("notifyType is required")
	}
	notifyConf, err := buildNotifyConf(req)
	if err != nil {
		return nil, err
	}
	fields := map[string]interface{}{
		"name":         strings.TrimSpace(req.Name),
		"remark":       strings.TrimSpace(req.Remark),
		"keywords_and": normalizeKeywordList(req.KeywordListAnd),
		"keywords_or":  normalizeKeywordList(req.KeywordListOr),
		"sentiment":    normalizeSentiment(req.Sentiment),
		"threshold":    req.Threshold,
		"interval":     req.Interval,
		"notify_type":  req.NotifyType,
		"notify_conf":  notifyConf,
	}
	timeRange := 3 // default
	if req.TimeRangeDays != nil && *req.TimeRangeDays > 0 {
		timeRange = *req.TimeRangeDays
	}
	fields["time_range_days"] = timeRange
	if req.Status != nil {
		fields["status"] = *req.Status
	}
	return fields, nil
}

func formatRuleResponse(rule *model.AlertRule) {
	rule.KeywordsAnd = formatAlertKeywords(rule.KeywordsAnd)
	rule.KeywordsOr = formatAlertKeywords(rule.KeywordsOr)
	rule.NotifyConf = formatNotifyConf(rule.NotifyType, rule.NotifyConf)
}

func buildNotifyConf(req alertRuleReq) (string, error) {
	if conf := strings.TrimSpace(req.NotifyConf); conf != "" && json.Valid([]byte(conf)) {
		return normalizeAlertNotifyConf(conf), nil
	}
	switch strings.TrimSpace(req.NotifyType) {
	case "email":
		email := strings.TrimSpace(req.NotifyEmail)
		if email == "" {
			return "", fmt.Errorf("请填写通知邮箱")
		}
		b, _ := json.Marshal(map[string]string{"email": email})
		return string(b), nil
	case "webhook":
		url := strings.TrimSpace(req.NotifyWebhook)
		if url == "" {
			return "", fmt.Errorf("请填写 Webhook 地址")
		}
		b, _ := json.Marshal(map[string]string{"url": url})
		return string(b), nil
	case "sms":
		phone := strings.TrimSpace(req.NotifyPhone)
		if phone == "" {
			return "", fmt.Errorf("请填写手机号")
		}
		b, _ := json.Marshal(map[string]string{"phone": phone})
		return string(b), nil
	default:
		return "{}", nil
	}
}

func formatNotifyConf(notifyType, raw string) string {
	raw = strings.TrimSpace(raw)
	if raw == "" || raw == "{}" {
		return "-"
	}
	var m map[string]string
	if err := json.Unmarshal([]byte(raw), &m); err != nil {
		return raw
	}
	switch notifyType {
	case "email":
		if v := m["email"]; v != "" {
			return v
		}
	case "webhook":
		if v := m["url"]; v != "" {
			return v
		}
	case "sms":
		if v := m["phone"]; v != "" {
			return v
		}
	}
	if v := m["value"]; v != "" {
		return v
	}
	return raw
}

func formatAlertKeywords(raw string) string {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return ""
	}
	if !json.Valid([]byte(raw)) {
		return raw
	}
	var arr []string
	if err := json.Unmarshal([]byte(raw), &arr); err == nil {
		return strings.Join(arr, ", ")
	}
	return raw
}

func (h *AlertHandler) ListRules(c *gin.Context) {
	list, err := h.alerts.ListRules()
	if err != nil {
		response.ServerError(c)
		return
	}
	for i := range list {
		list[i].KeywordsAnd = formatAlertKeywords(list[i].KeywordsAnd)
		list[i].KeywordsOr = formatAlertKeywords(list[i].KeywordsOr)
		list[i].NotifyConf = formatNotifyConf(list[i].NotifyType, list[i].NotifyConf)
	}
	response.OK(c, list)
}

func (h *AlertHandler) CreateRule(c *gin.Context) {
	var req alertRuleReq
	if err := c.ShouldBindJSON(&req); err != nil {
		response.Fail(c, 400, err.Error())
		return
	}
	uid, ok := CurrentUserID(c)
	if !ok {
		response.Unauthorized(c)
		return
	}
	fields, err := ruleFieldsFromReq(req)
	if err != nil {
		response.Fail(c, 400, err.Error())
		return
	}
	status := int8(1)
	if req.Status != nil {
		status = *req.Status
	}
	rule := model.AlertRule{
		Name:          fields["name"].(string),
		Remark:        fields["remark"].(string),
		KeywordsAnd:   fields["keywords_and"].(string),
		KeywordsOr:    fields["keywords_or"].(string),
		Sentiment:     fields["sentiment"].(string),
		Threshold:     fields["threshold"].(int),
		Interval:      fields["interval"].(int),
		TimeRangeDays: fields["time_range_days"].(int),
		NotifyType:    fields["notify_type"].(string),
		NotifyConf:    fields["notify_conf"].(string),
		Status:        status,
		CreatedBy:     uid,
	}
	if err := h.alerts.CreateRule(&rule); err != nil {
		response.ServerError(c)
		return
	}
	formatRuleResponse(&rule)
	response.OK(c, rule)
}

func (h *AlertHandler) UpdateRule(c *gin.Context) {
	var req alertRuleReq
	if err := c.ShouldBindJSON(&req); err != nil {
		response.Fail(c, 400, err.Error())
		return
	}
	existing, err := h.alerts.FindRule(c.Param("id"))
	if err != nil {
		response.ServerError(c)
		return
	}
	if existing == nil {
		response.Fail(c, 404, "规则不存在")
		return
	}
	fields, err := ruleFieldsFromReq(req)
	if err != nil {
		response.Fail(c, 400, err.Error())
		return
	}
	if req.Status == nil {
		fields["status"] = existing.Status
	}
	if err := h.alerts.UpdateRule(c.Param("id"), fields); err != nil {
		response.ServerError(c)
		return
	}
	updated, err := h.alerts.FindRule(c.Param("id"))
	if err != nil || updated == nil {
		response.ServerError(c)
		return
	}
	formatRuleResponse(updated)
	response.OK(c, updated)
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

func (h *AlertHandler) Evaluate(c *gin.Context) {
	if h.engine == nil {
		response.ServerError(c)
		return
	}
	if c.Query("sync") == "1" {
		result, err := h.engine.EvaluateAll(c.Request.Context(), "manual")
		if err != nil {
			response.Fail(c, 500, err.Error())
			return
		}
		response.OK(c, result)
		return
	}
	go func() {
		if _, err := h.engine.EvaluateAll(context.Background(), "manual"); err != nil {
			log.Printf("[alert] manual evaluate: %v", err)
		}
	}()
	response.OK(c, gin.H{"message": "告警评估已在后台启动"})
}

func (h *AlertHandler) GetRecordDetail(c *gin.Context) {
	record, err := h.alerts.FindRecord(c.Param("id"))
	if err != nil {
		response.ServerError(c)
		return
	}
	if record == nil {
		response.Fail(c, 404, "预警记录不存在")
		return
	}
	response.OK(c, record)
}

func (h *AlertHandler) MarkAsRead(c *gin.Context) {
	if err := h.alerts.MarkAsRead(c.Param("id")); err != nil {
		response.ServerError(c)
		return
	}
	response.OK(c, nil)
}
