package repository

import (
	"strconv"
	"strings"

	"gorm.io/gorm"
	"opinion-analysis/src/model"
)

// SmtpConfigData SMTP 邮件发送配置（system_settings: smtp.*）。
type SmtpConfigData struct {
	Host, Username, Password, From string
	Port                           int
	UseTLS                         bool
}

// AlertConfigData 告警引擎全局配置（system_settings: alert.*）。
type AlertConfigData struct {
	OnCrawl bool
}

var smtpConfigDescs = map[string]string{
	"smtp.host": "SMTP 服务器", "smtp.port": "SMTP 端口",
	"smtp.username": "SMTP 用户名", "smtp.password": "SMTP 授权码",
	"smtp.from": "发件人", "smtp.use_tls": "587 STARTTLS",
}

func smtpDefaults() map[string]string {
	return map[string]string{
		"smtp.host": "", "smtp.port": "465", "smtp.username": "",
		"smtp.password": "", "smtp.from": "", "smtp.use_tls": "false",
	}
}

func (r *SystemRepository) GetSmtpConfig() (SmtpConfigData, error) {
	m := smtpDefaults()
	rows, err := r.loadSettingsByPrefix("smtp.")
	if err != nil {
		return SmtpConfigData{}, err
	}
	for k, v := range rows {
		m[k] = v
	}
	from := strings.TrimSpace(m["smtp.from"])
	if from == "" {
		from = strings.TrimSpace(m["smtp.username"])
	}
	return SmtpConfigData{
		Host: strings.TrimSpace(m["smtp.host"]), Port: parseIntSetting(m["smtp.port"], 465),
		Username: strings.TrimSpace(m["smtp.username"]), Password: m["smtp.password"],
		From: from, UseTLS: parseBoolSetting(m["smtp.use_tls"]),
	}, nil
}

func (r *SystemRepository) SaveSmtpConfig(cfg SmtpConfigData, updatedBy uint) error {
	updates := map[string]string{
		"smtp.host": strings.TrimSpace(cfg.Host), "smtp.port": strconv.Itoa(cfg.Port),
		"smtp.username": strings.TrimSpace(cfg.Username), "smtp.from": strings.TrimSpace(cfg.From),
		"smtp.use_tls": boolToSetting(cfg.UseTLS),
	}
	if cfg.Password != "" {
		updates["smtp.password"] = cfg.Password
	}
	return r.db.Transaction(func(tx *gorm.DB) error {
		for key, val := range updates {
			if err := upsertSettingTx(tx, key, val, smtpConfigDescs[key], updatedBy); err != nil {
				return err
			}
		}
		return nil
	})
}

func (r *SystemRepository) GetAlertConfig() AlertConfigData {
	s, err := r.GetByKey("alert.on_crawl")
	if err != nil {
		return AlertConfigData{OnCrawl: true}
	}
	return AlertConfigData{OnCrawl: parseBoolSetting(s.Value)}
}

func (r *SystemRepository) loadSettingsByPrefix(prefix string) (map[string]string, error) {
	var rows []model.SystemSetting
	if err := r.db.Where("`key` LIKE ?", prefix+"%").Find(&rows).Error; err != nil {
		return nil, err
	}
	out := make(map[string]string, len(rows))
	for _, row := range rows {
		out[row.Key] = row.Value
	}
	return out, nil
}

func upsertSettingTx(tx *gorm.DB, key, val, desc string, updatedBy uint) error {
	var existing model.SystemSetting
	err := tx.Where("`key` = ?", key).First(&existing).Error
	if IsNotFound(err) {
		return tx.Create(&model.SystemSetting{Key: key, Value: val, Desc: desc, UpdatedBy: updatedBy}).Error
	}
	if err != nil {
		return err
	}
	return tx.Model(&model.SystemSetting{}).Where("`key` = ?", key).
		Updates(map[string]interface{}{"value": val, "updated_by": updatedBy}).Error
}
