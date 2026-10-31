package main

// emailcode.go — 邮箱验证码体系（2026-10-17）
//
// 覆盖三条链路，共用一套「发码 / 验码 / 限流」：
//   register  注册（POST /api/auth/code?purpose=register）
//   login     邮箱验证码登录（POST /api/auth/email-login）
//   reset     忘记密码重置（POST /api/auth/reset）
//
// 安全设计：
//   · 6 位数字，HMAC-SHA256(邮箱+用途+验证码, JWT_SECRET) 存储，不存明文
//   · 10 分钟有效、一次性使用、错 5 次作废
//   · 同邮箱同用途 60s 冷却；同邮箱 1 小时 6 条 / 24 小时 15 条；
//     同 IP 1 小时 15 条 / 24 小时 40 条

import (
	"crypto/hmac"
	"crypto/rand"
	"crypto/sha256"
	"crypto/subtle"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"log"
	"math/big"
	"net/http"
	"strings"
	"time"
)

const (
	emailCodeTTL      = 10 * time.Minute
	emailCodeCooldown = 60 * time.Second
	codePerEmailHour  = 6
	codePerEmailDay   = 15
	codePerIPHour     = 15
	codePerIPDay      = 40
	emailCodeMaxTries = 5
)

// EmailCode 验证码记录
type EmailCode struct {
	ID        uint      `gorm:"primarykey" json:"id"`
	Email     string    `gorm:"size:128;index" json:"email"`
	Purpose   string    `gorm:"size:16;index" json:"purpose"`
	CodeHash  string    `gorm:"size:64" json:"-"`
	Attempts  int       `json:"attempts"`
	Used      bool      `json:"used"`
	IP        string    `gorm:"size:64" json:"ip"`
	ExpiresAt time.Time `gorm:"index" json:"expires_at"`
	CreatedAt time.Time `json:"created_at"`
}

// ---------------- 基础工具 ----------------

func normalizeEmail(s string) string {
	return strings.ToLower(strings.TrimSpace(s))
}

// validEmail 轻量格式校验
func validEmail(email string) bool {
	if len(email) < 5 || len(email) > 128 || strings.ContainsAny(email, " \t\r\n,;") {
		return false
	}
	i := strings.LastIndex(email, "@")
	if i <= 0 || i == len(email)-1 {
		return false
	}
	domain := email[i+1:]
	if !strings.Contains(domain, ".") || strings.HasPrefix(domain, ".") || strings.HasSuffix(domain, ".") {
		return false
	}
	return !strings.ContainsAny(domain, "@")
}

// maskEmail 脱敏：ab***@example.com
func maskEmail(email string) string {
	i := strings.Index(email, "@")
	if i <= 0 {
		return email
	}
	name, domain := email[:i], email[i:]
	keep := 2
	if len(name) < keep {
		keep = len(name)
	}
	return name[:keep] + "***" + domain
}

func hashEmailCode(email, purpose, code string) string {
	mac := hmac.New(sha256.New, []byte(cfg.JWTSecret))
	mac.Write([]byte(email + "|" + purpose + "|" + code))
	return hex.EncodeToString(mac.Sum(nil))
}

func genEmailCode() string {
	n, err := rand.Int(rand.Reader, big.NewInt(1000000))
	if err != nil {
		return fmt.Sprintf("%06d", time.Now().UnixNano()%1000000)
	}
	return fmt.Sprintf("%06d", n.Int64())
}

func emailPurposeText(purpose string) (title, sub, subject string) {
	switch purpose {
	case "register":
		return "注册验证码",
			"你正在注册 WorkBuddy 2.0 账号，请在注册页填入下面的验证码完成邮箱验证。",
			"WorkBuddy 2.0 注册验证码"
	case "login":
		return "登录验证码",
			"你正在使用邮箱验证码登录 WorkBuddy 2.0，请在登录页填入下面的验证码。",
			"WorkBuddy 2.0 登录验证码"
	case "reset":
		return "重置密码验证码",
			"你正在重置 WorkBuddy 2.0 账号密码，请填入下面的验证码。若非本人操作请忽略本邮件。",
			"WorkBuddy 2.0 密码重置验证码"
	}
	return "", "", ""
}

func sendCodeMail(to, purpose, code string) error {
	title, sub, subject := emailPurposeText(purpose)
	if subject == "" {
		return fmt.Errorf("未知用途")
	}
	text := fmt.Sprintf("%s\n\n验证码：%s\n\n%d 分钟内有效，仅可使用一次。\n如果不是你本人的操作，请忽略此邮件。\n\n—— WorkBuddy 2.0",
		title, code, int(emailCodeTTL.Minutes()))
	return sendMail(to, subject, text, mailCodeHTML(title, sub, code, int(emailCodeTTL.Minutes())))
}

// ---------------- 账号查询 ----------------

func emailUserCount(email string) int64 {
	var n int64
	db.Model(&User{}).Where("LOWER(email) = ?", email).Count(&n)
	return n
}

// userByEmail 取邮箱对应的账号（多个时取最早创建的）
func userByEmail(email string) (*User, int64) {
	var n int64
	db.Model(&User{}).Where("LOWER(email) = ?", email).Count(&n)
	if n == 0 {
		return nil, 0
	}
	var u User
	if err := db.Where("LOWER(email) = ?", email).Order("id asc").First(&u).Error; err != nil {
		return nil, n
	}
	return &u, n
}

// verifyEmailCode 校验验证码；成功即标记已用
func verifyEmailCode(email, purpose, code string) (bool, string) {
	var rec EmailCode
	if err := db.Where("email = ? AND purpose = ? AND used = ?", email, purpose, false).
		Order("id desc").First(&rec).Error; err != nil {
		return false, "请先获取验证码"
	}
	if time.Now().After(rec.ExpiresAt) {
		return false, "验证码已过期，请重新获取"
	}
	if rec.Attempts >= emailCodeMaxTries {
		return false, "错误次数过多，请重新获取验证码"
	}
	if subtle.ConstantTimeCompare([]byte(rec.CodeHash), []byte(hashEmailCode(email, purpose, code))) != 1 {
		db.Model(&EmailCode{}).Where("id = ?", rec.ID).Update("attempts", rec.Attempts+1)
		left := emailCodeMaxTries - rec.Attempts - 1
		if left > 0 && left <= 2 {
			return false, fmt.Sprintf("验证码不正确，还可尝试 %d 次", left)
		}
		return false, "验证码不正确"
	}
	db.Model(&EmailCode{}).Where("id = ?", rec.ID).Updates(map[string]interface{}{"used": true})
	return true, ""
}

// ---------------- 发码：POST /api/auth/code ----------------

func handleSendCode(w http.ResponseWriter, r *http.Request) {
	if !methodGuard(w, r, http.MethodPost) {
		return
	}
	if cors(w, r) {
		return
	}
	var in struct {
		Email   string `json:"email"`
		Purpose string `json:"purpose"`
	}
	if err := json.NewDecoder(r.Body).Decode(&in); err != nil {
		writeErr(w, 400, "参数错误")
		return
	}
	email := normalizeEmail(in.Email)
	purpose := strings.ToLower(strings.TrimSpace(in.Purpose))
	if purpose == "" {
		purpose = "register"
	}
	if _, _, subject := emailPurposeText(purpose); subject == "" {
		writeErr(w, 400, "验证码用途不正确")
		return
	}
	if !validEmail(email) {
		writeErr(w, 400, "邮箱格式不正确")
		return
	}
	if !mailReady() {
		writeErr(w, 503, "邮件服务尚未配置，请联系管理员")
		return
	}

	// 用途相关的前置校验
	switch purpose {
	case "register":
		if emailUserCount(email) > 0 {
			writeErr(w, 409, "该邮箱已注册，请直接登录")
			return
		}
	case "login":
		if u, _ := userByEmail(email); u == nil {
			writeErr(w, 404, "该邮箱还没有注册，请先注册")
			return
		} else if u.Banned {
			writeErr(w, 403, "账号已被停用："+u.BanReason)
			return
		}
	case "reset":
		if u, _ := userByEmail(email); u == nil {
			writeErr(w, 404, "该邮箱还没有注册")
			return
		}
	}

	now := time.Now()
	ip := clientIP(r)

	// 冷却
	var recent int64
	db.Model(&EmailCode{}).
		Where("email = ? AND purpose = ? AND created_at > ?", email, purpose, now.Add(-emailCodeCooldown)).
		Count(&recent)
	if recent > 0 {
		writeErr(w, 429, "验证码刚发过，请 60 秒后再试")
		return
	}
	// 邮箱维度限流
	var eHour, eDay int64
	db.Model(&EmailCode{}).Where("email = ? AND created_at > ?", email, now.Add(-time.Hour)).Count(&eHour)
	db.Model(&EmailCode{}).Where("email = ? AND created_at > ?", email, now.Add(-24*time.Hour)).Count(&eDay)
	if eHour >= codePerEmailHour || eDay >= codePerEmailDay {
		writeErr(w, 429, "该邮箱今日获取验证码次数过多，请稍后再试")
		return
	}
	// IP 维度限流
	var iHour, iDay int64
	db.Model(&EmailCode{}).Where("ip = ? AND created_at > ?", ip, now.Add(-time.Hour)).Count(&iHour)
	db.Model(&EmailCode{}).Where("ip = ? AND created_at > ?", ip, now.Add(-24*time.Hour)).Count(&iDay)
	if iHour >= codePerIPHour || iDay >= codePerIPDay {
		writeErr(w, 429, "请求过于频繁，请稍后再试")
		return
	}

	code := genEmailCode()
	rec := EmailCode{
		Email:     email,
		Purpose:   purpose,
		CodeHash:  hashEmailCode(email, purpose, code),
		IP:        ip,
		ExpiresAt: now.Add(emailCodeTTL),
	}
	if err := db.Create(&rec).Error; err != nil {
		writeErr(w, 500, "验证码生成失败")
		return
	}
	if err := sendCodeMail(email, purpose, code); err != nil {
		db.Delete(&EmailCode{}, rec.ID)
		log.Printf("[emailcode] 发送失败 to=%s purpose=%s: %v", email, purpose, err)
		writeErr(w, 502, "验证码邮件发送失败，请稍后再试")
		return
	}

	resp := map[string]interface{}{
		"ok":           true,
		"email":        maskEmail(email),
		"purpose":      purpose,
		"ttl":          int(emailCodeTTL.Seconds()),
		"resend_after": int(emailCodeCooldown.Seconds()),
		"temp_mail":    isTempEmail(email),
		"channel":      mailChannel(),
	}
	// 临时邮箱：注册可继续，但不会得到注册礼包
	if purpose == "register" {
		if resp["temp_mail"].(bool) {
			resp["signup_points"] = 0
			resp["hint"] = "临时邮箱可以注册，但不发放注册礼包与邀请奖励"
		} else {
			resp["signup_points"] = cfg.SignupPoints
		}
	}
	ok(w, resp)
}

// ---------------- 邮箱验证码登录：POST /api/auth/email-login ----------------

func handleEmailLogin(w http.ResponseWriter, r *http.Request) {
	if !methodGuard(w, r, http.MethodPost) {
		return
	}
	if cors(w, r) {
		return
	}
	var in struct {
		Email string `json:"email"`
		Code  string `json:"code"`
	}
	if err := json.NewDecoder(r.Body).Decode(&in); err != nil {
		writeErr(w, 400, "参数错误")
		return
	}
	email := normalizeEmail(in.Email)
	code := strings.TrimSpace(in.Code)
	if !validEmail(email) {
		writeErr(w, 400, "邮箱格式不正确")
		return
	}
	if len(code) != 6 {
		writeErr(w, 400, "请输入 6 位验证码")
		return
	}
	u, n := userByEmail(email)
	if u == nil {
		writeErr(w, 404, "该邮箱还没有注册，请先注册")
		return
	}
	if n > 1 {
		writeErr(w, 409, "该邮箱关联了多个账号，请用用户名密码登录")
		return
	}
	if u.Banned {
		writeErr(w, 403, "账号已被停用："+u.BanReason)
		return
	}
	if okc, msg := verifyEmailCode(email, "login", code); !okc {
		writeErr(w, 401, msg)
		return
	}
	now := time.Now()
	u.LastLogin = &now
	db.Model(&User{}).Where("id = ?", u.ID).Updates(map[string]interface{}{
		"last_login":     now,
		"email_verified": true,
	})
	writeJSON(w, 200, map[string]interface{}{
		"token": signToken(u.ID, u.IsAdmin),
		"user":  publicUser(u),
	})
}

// ---------------- 忘记密码重置：POST /api/auth/reset ----------------

func handleResetPassword(w http.ResponseWriter, r *http.Request) {
	if !methodGuard(w, r, http.MethodPost) {
		return
	}
	if cors(w, r) {
		return
	}
	var in struct {
		Email    string `json:"email"`
		Code     string `json:"code"`
		Password string `json:"password"`
	}
	if err := json.NewDecoder(r.Body).Decode(&in); err != nil {
		writeErr(w, 400, "参数错误")
		return
	}
	email := normalizeEmail(in.Email)
	code := strings.TrimSpace(in.Code)
	if !validEmail(email) {
		writeErr(w, 400, "邮箱格式不正确")
		return
	}
	if len(in.Password) < 6 {
		writeErr(w, 400, "新密码至少 6 位")
		return
	}
	if len(code) != 6 {
		writeErr(w, 400, "请输入 6 位验证码")
		return
	}
	u, n := userByEmail(email)
	if u == nil {
		writeErr(w, 404, "该邮箱还没有注册")
		return
	}
	if n > 1 {
		writeErr(w, 409, "该邮箱关联了多个账号，请联系管理员重置")
		return
	}
	if okc, msg := verifyEmailCode(email, "reset", code); !okc {
		writeErr(w, 401, msg)
		return
	}
	db.Model(&User{}).Where("id = ?", u.ID).Updates(map[string]interface{}{
		"pass_hash":      hashPass(in.Password),
		"email_verified": true,
	})
	log.Printf("[auth] 邮箱验证码重置密码成功 uid=%d email=%s ip=%s", u.ID, maskEmail(email), clientIP(r))
	writeJSON(w, 200, map[string]interface{}{
		"ok":    true,
		"token": signToken(u.ID, u.IsAdmin),
		"user":  publicUser(u),
	})
}
