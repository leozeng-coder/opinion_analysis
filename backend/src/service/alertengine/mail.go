package alertengine

import (
	"crypto/tls"
	"encoding/base64"
	"fmt"
	"net"
	"net/smtp"
	"strings"
	"time"

	"opinion-analysis/src/repository"
)

// SendTestMail 发送测试邮件（管理后台用）。
func SendTestMail(cfg repository.SmtpConfigData, to string) error {
	return sendMail(cfg, to, "Alert Test", "SMTP configuration is working.")
}

func sendMail(cfg repository.SmtpConfigData, to, subject, body string) error {
	if err := validateSmtpConfig(cfg); err != nil {
		return err
	}
	from := resolveFrom(cfg)
	return sendSMTP(cfg, from, to, buildMimeMessage(from, to, subject, body))
}

func validateSmtpConfig(cfg repository.SmtpConfigData) error {
	var missing []string
	if strings.TrimSpace(cfg.Host) == "" {
		missing = append(missing, "SMTP 服务器")
	}
	if strings.TrimSpace(cfg.Username) == "" {
		missing = append(missing, "用户名")
	}
	if strings.TrimSpace(cfg.Password) == "" {
		missing = append(missing, "密码")
	}
	if len(missing) > 0 {
		return fmt.Errorf("SMTP 配置不完整：%s", strings.Join(missing, "、"))
	}
	return nil
}

func resolveFrom(cfg repository.SmtpConfigData) string {
	from := strings.TrimSpace(cfg.From)
	if from == "" {
		return strings.TrimSpace(cfg.Username)
	}
	if emailDomain(from) != "" && emailDomain(from) != emailDomain(cfg.Username) {
		return strings.TrimSpace(cfg.Username)
	}
	return from
}

func emailDomain(addr string) string {
	if i := strings.LastIndex(strings.TrimSpace(addr), "@"); i >= 0 {
		return strings.ToLower(addr[i+1:])
	}
	return ""
}

func smtpHost(host string) string {
	host = strings.TrimSpace(host)
	if i := strings.Index(host, ":"); i > 0 {
		return host[:i]
	}
	return host
}

func sendSMTP(cfg repository.SmtpConfigData, from, to, msg string) error {
	addr := fmt.Sprintf("%s:%d", cfg.Host, cfg.Port)
	host := smtpHost(cfg.Host)
	auths := smtpAuths(cfg)

	var lastErr error
	for _, auth := range auths {
		var err error
		switch {
		case cfg.Port == 465:
			err = sendOverSSL(addr, host, auth, from, to, msg)
		case cfg.Port == 587 || cfg.UseTLS:
			err = sendOverSTARTTLS(addr, host, auth, from, to, msg)
		default:
			return fmt.Errorf("不支持的 SMTP 端口 %d，请使用 465（SSL）或 587（STARTTLS）", cfg.Port)
		}
		if err == nil {
			return nil
		}
		lastErr = err
	}
	return fmt.Errorf("邮件发送失败: %w", lastErr)
}

func sendOverSSL(addr, host string, auth smtp.Auth, from, to, msg string) error {
	conn, err := tls.Dial("tcp", addr, &tls.Config{ServerName: host, MinVersion: tls.VersionTLS12})
	if err != nil {
		return err
	}
	client, err := smtp.NewClient(conn, host)
	if err != nil {
		conn.Close()
		return err
	}
	defer client.Close()
	return submit(client, auth, from, to, msg)
}

func sendOverSTARTTLS(addr, host string, auth smtp.Auth, from, to, msg string) error {
	conn, err := net.DialTimeout("tcp", addr, 20*time.Second)
	if err != nil {
		return err
	}
	client, err := smtp.NewClient(conn, host)
	if err != nil {
		conn.Close()
		return err
	}
	defer client.Close()
	if ok, _ := client.Extension("STARTTLS"); ok {
		if err := client.StartTLS(&tls.Config{ServerName: host, MinVersion: tls.VersionTLS12}); err != nil {
			return err
		}
	}
	return submit(client, auth, from, to, msg)
}

func submit(client *smtp.Client, auth smtp.Auth, from, to, msg string) error {
	if ok, _ := client.Extension("AUTH"); ok {
		if err := client.Auth(auth); err != nil {
			return err
		}
	}
	if err := client.Mail(from); err != nil {
		return err
	}
	if err := client.Rcpt(to); err != nil {
		return err
	}
	w, err := client.Data()
	if err != nil {
		return err
	}
	if _, err := w.Write([]byte(msg)); err != nil {
		return err
	}
	return w.Close()
}

func smtpAuths(cfg repository.SmtpConfigData) []smtp.Auth {
	host := smtpHost(cfg.Host)
	plain := smtp.PlainAuth("", cfg.Username, cfg.Password, host)
	login := loginAuth{username: cfg.Username, password: cfg.Password}
	domain := emailDomain(cfg.Username)
	if strings.Contains(host, "163.") || domain == "163.com" || domain == "126.com" {
		return []smtp.Auth{&login, plain}
	}
	return []smtp.Auth{plain, &login}
}

type loginAuth struct {
	username, password string
}

func (a *loginAuth) Start(_ *smtp.ServerInfo) (string, []byte, error) {
	return "LOGIN", nil, nil
}

func (a *loginAuth) Next(fromServer []byte, more bool) ([]byte, error) {
	if !more {
		return nil, nil
	}
	if strings.HasPrefix(string(fromServer), "334 ") {
		raw, _ := base64.StdEncoding.DecodeString(strings.TrimSpace(string(fromServer)[4:]))
		ch := strings.ToLower(string(raw))
		if strings.Contains(ch, "user") {
			return []byte(base64.StdEncoding.EncodeToString([]byte(a.username))), nil
		}
		if strings.Contains(ch, "pass") {
			return []byte(base64.StdEncoding.EncodeToString([]byte(a.password))), nil
		}
	}
	return nil, fmt.Errorf("unexpected LOGIN prompt: %q", fromServer)
}

func buildMimeMessage(from, to, subject, body string) string {
	for _, r := range subject {
		if r > 127 {
			subject = fmt.Sprintf("=?UTF-8?B?%s?=", base64.StdEncoding.EncodeToString([]byte(subject)))
			break
		}
	}
	return "From: " + from + "\r\n" +
		"To: " + to + "\r\n" +
		"Subject: " + subject + "\r\n" +
		"MIME-Version: 1.0\r\n" +
		"Content-Type: text/plain; charset=UTF-8\r\n\r\n" +
		body
}
