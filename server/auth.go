package main

// auth.go — 注册 / 登录 / 令牌 / 中间件

import (
	"context"
	"crypto/hmac"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"strconv"
	"strings"
	"time"

	"golang.org/x/crypto/bcrypt"
)

// ---------------- 口令 ----------------
func hashPass(p string) string {
	b, _ := bcrypt.GenerateFromPassword([]byte(p), bcrypt.DefaultCost)
	return string(b)
}

func checkPass(h, p string) bool {
	return bcrypt.CompareHashAndPassword([]byte(h), []byte(p)) == nil
}

// ---------------- 令牌（自实现 HMAC，不引三方） ----------------
func signToken(uid uint, isAdmin bool) string {
	exp := time.Now().Add(30 * 24 * time.Hour).Unix()
	payload := fmt.Sprintf("%d|%t|%d", uid, isAdmin, exp)
	mac := hmac.New(sha256.New, []byte(cfg.JWTSecret))
	mac.Write([]byte(payload))
	sig := base64.RawURLEncoding.EncodeToString(mac.Sum(nil))
	return base64.RawURLEncoding.EncodeToString([]byte(payload)) + "." + sig
}

func parseToken(tok string) (uint, bool, error) {
	parts := strings.Split(tok, ".")
	if len(parts) != 2 {
		return 0, false, fmt.Errorf("bad token")
	}
	raw, err := base64.RawURLEncoding.DecodeString(parts[0])
	if err != nil {
		return 0, false, err
	}
	mac := hmac.New(sha256.New, []byte(cfg.JWTSecret))
	mac.Write(raw)
	want := base64.RawURLEncoding.EncodeToString(mac.Sum(nil))
	if !hmac.Equal([]byte(want), []byte(parts[1])) {
		return 0, false, fmt.Errorf("bad signature")
	}
	fields := strings.Split(string(raw), "|")
	if len(fields) != 3 {
		return 0, false, fmt.Errorf("bad payload")
	}
	uid, _ := strconv.ParseUint(fields[0], 10, 64)
	adm := fields[1] == "true"
	exp, _ := strconv.ParseInt(fields[2], 10, 64)
	if time.Now().Unix() > exp {
		return 0, false, fmt.Errorf("expired")
	}
	return uint(uid), adm, nil
}

// ---------------- 上下文 ----------------
type ctxKey struct{ name string }

var userKey = ctxKey{"user"}

func authMiddleware(next http.HandlerFunc, adminOnly bool) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if cors(w, r) {
			return
		}
		h := r.Header.Get("Authorization")
		tok := strings.TrimPrefix(h, "Bearer ")
		if tok == "" || tok == h {
			tok = r.URL.Query().Get("token")
		}
		uid, adm, err := parseToken(tok)
		if err != nil {
			writeErr(w, 401, "请先登录")
			return
		}
		var u User
		if err := db.First(&u, uid).Error; err != nil {
			writeErr(w, 401, "账号不存在")
			return
		}
		if u.Banned {
			writeErr(w, 403, "账号已被停用："+u.BanReason)
			return
		}
		if adminOnly && !u.IsAdmin {
			writeErr(w, 403, "需要管理员权限")
			return
		}
		_ = adm
		r = withUser(r, &u)
		next(w, r)
	}
}

// optionalAuth 允许未登录访问（抽奖概率公示等）；带 token 时注入当前用户
func optionalAuth(next http.HandlerFunc) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if cors(w, r) {
			return
		}
		h := r.Header.Get("Authorization")
		tok := strings.TrimPrefix(h, "Bearer ")
		if tok == "" || tok == h {
			tok = r.URL.Query().Get("token")
		}
		if tok != "" {
			if uid, _, err := parseToken(tok); err == nil {
				var u User
				if db.First(&u, uid).Error == nil && !u.Banned {
					r = withUser(r, &u)
				}
			}
		}
		next(w, r)
	}
}

func currentUser(r *http.Request) *User {
	u, _ := r.Context().Value(userKey).(*User)
	return u
}

func withUser(r *http.Request, u *User) *http.Request {
	return r.WithContext(context.WithValue(r.Context(), userKey, u))
}

// ---------------- 注册 / 登录 ----------------
func handleRegister(w http.ResponseWriter, r *http.Request) {
	var in struct {
		Username string `json:"username"`
		Password string `json:"password"`
		Email    string `json:"email"`
		Invite   string `json:"invite"`
		Code     string `json:"code"` // 邮箱验证码
	}
	if err := json.NewDecoder(r.Body).Decode(&in); err != nil {
		writeErr(w, 400, "参数错误")
		return
	}
	in.Username = strings.TrimSpace(in.Username)
	if len(in.Username) < 3 || len(in.Username) > 24 {
		writeErr(w, 400, "用户名 3–24 个字符")
		return
	}
	if len(in.Password) < 6 {
		writeErr(w, 400, "密码至少 6 位")
		return
	}
	var cnt int64
	db.Model(&User{}).Where("username = ?", in.Username).Count(&cnt)
	if cnt > 0 {
		writeErr(w, 409, "用户名已被占用")
		return
	}

	// ---- 邮箱验证码（注册必填；邮件通道未配置时放行并记日志） ----
	email := normalizeEmail(in.Email)
	verifyRequired := getSetting("email_verify_required", "1") == "1" && mailReady()
	tempMail := false
	if verifyRequired {
		if !validEmail(email) {
			writeErr(w, 400, "请填写正确的邮箱并获取验证码")
			return
		}
		if emailUserCount(email) > 0 {
			writeErr(w, 409, "该邮箱已注册，请直接登录")
			return
		}
		if len(strings.TrimSpace(in.Code)) != 6 {
			writeErr(w, 400, "请输入 6 位邮箱验证码")
			return
		}
		if okc, msg := verifyEmailCode(email, "register", strings.TrimSpace(in.Code)); !okc {
			writeErr(w, 400, msg)
			return
		}
		tempMail = isTempEmail(email)
	} else if email != "" {
		if !validEmail(email) {
			writeErr(w, 400, "邮箱格式不正确")
			return
		}
		if emailUserCount(email) > 0 {
			writeErr(w, 409, "该邮箱已注册，请直接登录")
			return
		}
		tempMail = isTempEmail(email)
	}
	u := User{
		Username:      in.Username,
		Email:         email,
		EmailVerified: verifyRequired,
		EmailIsTemp:   tempMail,
		PassHash:      hashPass(in.Password),
		InviteCode:    genInviteCode(),
		InvitedBy:     strings.TrimSpace(in.Invite),
	}
	// 注册礼：3000 点，7 天有效（写进 bonus，不占用每日额度）
	// ⚠️ 临时邮箱不赠送：可正常注册使用，但不发礼包，也不给邀请人返点
	if !tempMail {
		u.BonusPoints = cfg.SignupPoints
		until := time.Now().AddDate(0, 0, cfg.SignupDays)
		u.BonusUntil = &until
	}
	if err := db.Create(&u).Error; err != nil {
		writeErr(w, 500, "创建失败")
		return
	}
	// 邀请奖励：给邀请人 500 点（被邀请人是临时邮箱、或邀请人本身是临时邮箱都不发）
	if u.InvitedBy != "" && !tempMail {
		var inv User
		if err := db.First(&inv, "invite_code = ?", u.InvitedBy).Error; err == nil && !inv.EmailIsTemp {
			inv.BonusPoints += 500
			db.Save(&inv)
		}
	}
	log.Printf("[auth] 注册成功 uid=%d user=%s email=%s temp=%v ip=%s", u.ID, u.Username, maskEmail(email), tempMail, clientIP(r))
	resp := map[string]interface{}{
		"token": signToken(u.ID, u.IsAdmin),
		"user":  publicUser(&u),
	}
	if tempMail {
		resp["notice"] = "检测到临时邮箱：账号已创建，但不发放注册礼包与邀请奖励"
	}
	writeJSON(w, 200, resp)
}

func handleLogin(w http.ResponseWriter, r *http.Request) {
	var in struct {
		Username string `json:"username"`
		Password string `json:"password"`
	}
	if err := json.NewDecoder(r.Body).Decode(&in); err != nil {
		writeErr(w, 400, "参数错误")
		return
	}
	var u User
	if err := db.First(&u, "username = ? OR email = ?", strings.TrimSpace(in.Username), strings.TrimSpace(in.Username)).Error; err != nil {
		writeErr(w, 401, "用户名或密码错误")
		return
	}
	if u.PassHash == "" {
		writeErr(w, 400, "该账号使用 GitHub 登录，请点「使用 GitHub 继续」，或用邮箱验证码重置密码后再设密码")
		return
	}
	if !checkPass(u.PassHash, in.Password) {
		writeErr(w, 401, "用户名或密码错误")
		return
	}
	if u.Banned {
		writeErr(w, 403, "账号已被停用："+u.BanReason)
		return
	}
	now := time.Now()
	u.LastLogin = &now
	db.Save(&u)
	writeJSON(w, 200, map[string]interface{}{
		"token": signToken(u.ID, u.IsAdmin),
		"user":  publicUser(&u),
	})
}

func publicUser(u *User) map[string]interface{} {
	if u == nil {
		return nil
	}
	quota := dailyQuota(u)
	rollDay(u)
	return map[string]interface{}{
		"id":             u.ID,
		"username":       u.Username,
		"email":          u.Email,
		"email_verified": u.EmailVerified,
		"email_is_temp":  u.EmailIsTemp,
		"is_admin":       u.IsAdmin,
		"plan":           u.Plan,
		"plan_until":     u.PlanUntil,
		"daily_points":   quota,
		"points_today":   u.PointsToday,
		"points_left":    max0(quota - u.PointsToday),
		"bonus_points":   u.BonusPoints,
		"bonus_until":    u.BonusUntil,
		"lottery_points": u.LotteryPoints,
		"contrib_ratio":  u.ContribRatio,
		"student":        u.Student,
		"invite_code":    u.InviteCode,
		"github_login":   u.GitHubLogin,
		"created_at":     u.CreatedAt,
	}
}

func genInviteCode() string {
	const alphabet = "abcdefghjkmnpqrstuvwxyz23456789"
	for {
		b := make([]byte, 8)
		rand.Read(b)
		for i := range b {
			b[i] = alphabet[int(b[i])%len(alphabet)]
		}
		code := string(b)
		var c int64
		db.Model(&User{}).Where("invite_code = ?", code).Count(&c)
		if c == 0 {
			return code
		}
	}
}

func randHex(n int) string {
	b := make([]byte, n)
	rand.Read(b)
	return hex.EncodeToString(b)
}

func max0(v float64) float64 {
	if v < 0 {
		return 0
	}
	return v
}
