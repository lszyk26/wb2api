package main

// mail.go — 统一发信出口（2026-10-17）
//
// 通道策略：Resend HTTP API 优先（HTTPS 443，不看出口 IP 归属，机房 IP 也能发），
// SMTP 作为兜底（将来换服务商不必改代码）。
//
// 配置（server/data/env）：
//   WB2_RESEND_KEY   Resend API Key（re_ 开头）
//   WB2_RESEND_FROM  发件人，如 WorkBuddy2 API <noreply@your-domain.com>（域名需在 Resend 验证）
//   WB2_SMTP_HOST / PORT / USER / PASS / FROM   SMTP 兜底
//
// 行为：配置了 Resend 就优先走它；失败时若 SMTP 可用则兜底，并把两条错误都写日志。

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net/http"
	"net/smtp"
	"strings"
	"time"
)

var mailHTTP = &http.Client{Timeout: 20 * time.Second}

func resendConfigured() bool { return strings.TrimSpace(cfg.ResendKey) != "" }

func smtpUsable() bool {
	return cfg.SMTPHost != "" && cfg.SMTPUser != "" && cfg.SMTPPass != ""
}

// mailReady 是否配置了任何可用发信通道
func mailReady() bool { return resendConfigured() || smtpUsable() }

// mailChannel 当前生效通道名（日志/管理端展示）
func mailChannel() string {
	if resendConfigured() {
		return "resend"
	}
	if smtpUsable() {
		return "smtp"
	}
	return "none"
}

func mailFrom() string {
	if f := strings.TrimSpace(cfg.ResendFrom); f != "" {
		return f
	}
	return "WorkBuddy2 API <noreply@example.com>"
}

func sendViaResend(to, subject, text, html string) error {
	payload := map[string]interface{}{
		"from":    mailFrom(),
		"to":      []string{to},
		"subject": subject,
		"text":    text,
	}
	if html != "" {
		payload["html"] = html
	}
	buf, _ := json.Marshal(payload)
	req, err := http.NewRequest(http.MethodPost, "https://api.resend.com/emails", bytes.NewReader(buf))
	if err != nil {
		return err
	}
	req.Header.Set("Authorization", "Bearer "+strings.TrimSpace(cfg.ResendKey))
	req.Header.Set("Content-Type", "application/json")

	resp, err := mailHTTP.Do(req)
	if err != nil {
		return fmt.Errorf("resend 请求失败: %w", err)
	}
	defer resp.Body.Close()
	body, _ := io.ReadAll(io.LimitReader(resp.Body, 4096))
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return fmt.Errorf("resend 返回 %d: %s", resp.StatusCode, strings.TrimSpace(string(body)))
	}
	return nil
}

func sendViaSMTP(to, subject, text, html string) error {
	if !smtpUsable() {
		return fmt.Errorf("SMTP 未配置")
	}
	port := cfg.SMTPPort
	if port == "" {
		port = "587"
	}
	from := cfg.SMTPFrom
	if from == "" {
		from = cfg.SMTPUser
	}

	var body string
	if html == "" {
		body = fmt.Sprintf("From: %s\r\nTo: %s\r\nSubject: %s\r\nMIME-Version: 1.0\r\nContent-Type: text/plain; charset=UTF-8\r\n\r\n%s",
			from, to, subject, text)
	} else {
		bd := "wb2mail" + randHex(6)
		body = fmt.Sprintf("From: %s\r\nTo: %s\r\nSubject: %s\r\nMIME-Version: 1.0\r\n"+
			"Content-Type: multipart/alternative; boundary=%s\r\n\r\n"+
			"--%s\r\nContent-Type: text/plain; charset=UTF-8\r\n\r\n%s\r\n"+
			"--%s\r\nContent-Type: text/html; charset=UTF-8\r\n\r\n%s\r\n--%s--\r\n",
			from, to, subject, bd, bd, text, bd, html, bd)
	}
	auth := smtp.PlainAuth("", cfg.SMTPUser, cfg.SMTPPass, cfg.SMTPHost)
	return smtp.SendMail(cfg.SMTPHost+":"+port, auth, from, []string{to}, []byte(body))
}

// sendMail 统一发信入口
func sendMail(to, subject, text, html string) error {
	if resendConfigured() {
		err := sendViaResend(to, subject, text, html)
		if err == nil {
			log.Printf("[mail] resend ok to=%s subject=%q", to, subject)
			return nil
		}
		log.Printf("[mail] resend FAILED to=%s: %v", to, err)
		if smtpUsable() {
			if e2 := sendViaSMTP(to, subject, text, html); e2 == nil {
				log.Printf("[mail] smtp fallback ok to=%s", to)
				return nil
			} else {
				log.Printf("[mail] smtp fallback also FAILED to=%s: %v", to, e2)
				return fmt.Errorf("resend: %v; smtp: %v", err, e2)
			}
		}
		return err
	}
	if smtpUsable() {
		return sendViaSMTP(to, subject, text, html)
	}
	return fmt.Errorf("发信通道未配置")
}

// mailCodeHTML 验证码邮件模板（暗色 + 橙色强调，与站点一致）
func mailCodeHTML(title, sub, code string, ttlMin int) string {
	return `<!doctype html><html lang="zh-CN"><body style="margin:0;padding:32px 14px;background:#0b0b0b;` +
		`font-family:-apple-system,BlinkMacSystemFont,'Segoe UI',Roboto,'Helvetica Neue',Arial,'PingFang SC','Microsoft YaHei',sans-serif;">` +
		`<table role="presentation" width="100%" cellpadding="0" cellspacing="0" style="max-width:520px;margin:0 auto;">` +
		`<tr><td style="background:#141414;border:1px solid #262626;border-radius:18px;overflow:hidden;">` +
		`<table role="presentation" width="100%" cellpadding="0" cellspacing="0">` +
		`<tr><td style="padding:28px 28px 0;">` +
		`<div style="font-size:12px;letter-spacing:.20em;color:#ff8a3d;font-weight:700;">WORKBUDDY 2.0</div>` +
		`<h1 style="margin:14px 0 0;font-size:21px;line-height:1.35;color:#f6f6f6;font-weight:700;">` + title + `</h1>` +
		`<p style="margin:10px 0 0;font-size:13px;line-height:1.75;color:#b9b9b9;">` + sub + `</p>` +
		`</td></tr>` +
		`<tr><td style="padding:22px 28px 0;">` +
		`<div style="background:#1c1c1c;border:1px solid #2e2e2e;border-radius:14px;padding:20px 16px;text-align:center;">` +
		`<div style="font-family:ui-monospace,SFMono-Regular,Menlo,Consolas,monospace;font-size:34px;line-height:1.1;letter-spacing:.32em;color:#ffb27a;font-weight:700;">` + code + `</div>` +
		`</div></td></tr>` +
		`<tr><td style="padding:20px 28px 26px;">` +
		`<p style="margin:0;font-size:12px;line-height:1.8;color:#7e7e7e;">验证码 <b style="color:#b9b9b9;">` + fmt.Sprint(ttlMin) + ` 分钟</b>内有效，且仅可使用一次。<br>如果不是你本人的操作，请忽略此邮件，你的账号不会被更改。</p>` +
		`</td></tr>` +
		`<tr><td style="padding:15px 28px;border-top:1px solid #232323;">` +
		`<p style="margin:0;font-size:11px;color:#575757;">WorkBuddy 2.0 · ` + cfg.PublicURL + ` · 兼容 OpenAI 的账号网关</p>` +
		`</td></tr></table></td></tr></table></body></html>`
}
