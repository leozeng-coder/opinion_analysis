package alertengine

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"

	"opinion-analysis/src/model"
	"opinion-analysis/src/repository"
)

func (e *Engine) notify(
	ctx context.Context,
	rule *model.AlertRule,
	record *model.AlertRecord,
	smtpCfg repository.SmtpConfigData,
) error {
	switch strings.TrimSpace(rule.NotifyType) {
	case "email":
		to := notifyConfValue(rule.NotifyConf, "email")
		if to == "" {
			return fmt.Errorf("规则未配置通知邮箱")
		}
		return sendMail(smtpCfg, to, record.Title, record.Content)
	case "webhook":
		return sendWebhook(ctx, rule, record)
	case "sms":
		return fmt.Errorf("短信通知尚未接入")
	default:
		return nil
	}
}

func notifyConfValue(raw, key string) string {
	var m map[string]string
	if json.Unmarshal([]byte(raw), &m) != nil {
		return ""
	}
	return strings.TrimSpace(m[key])
}

func sendWebhook(ctx context.Context, rule *model.AlertRule, record *model.AlertRecord) error {
	url := notifyConfValue(rule.NotifyConf, "url")
	if url == "" {
		return fmt.Errorf("规则未配置 Webhook 地址")
	}
	payload, _ := json.Marshal(map[string]any{
		"ruleId": rule.ID, "ruleName": rule.Name,
		"title": record.Title, "content": record.Content,
		"recordId": record.ID, "createdAt": record.CreatedAt,
	})
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, url, bytes.NewReader(payload))
	if err != nil {
		return err
	}
	req.Header.Set("Content-Type", "application/json")
	resp, err := (&http.Client{Timeout: 15 * time.Second}).Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	if resp.StatusCode >= 400 {
		body, _ := io.ReadAll(io.LimitReader(resp.Body, 512))
		return fmt.Errorf("webhook HTTP %d: %s", resp.StatusCode, strings.TrimSpace(string(body)))
	}
	return nil
}
