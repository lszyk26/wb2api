package main

// oauth_github.go — 用户侧 GitHub OAuth 登录 / 自动注册
//
// 注意与 oauth.go 区分：那个是「用户授权自己的 WorkBuddy 上游账号」，
// 本文件是「用户用 GitHub 登录本站」。
//
// 流程：
//   GET  /api/auth/github/start     拿到 GitHub 授权地址（state 一次性、10 分钟）
//   GET  /api/auth/github/callback  GitHub 回跳：校验 state → 换 access_token → 拉用户 → 登录或建号
//   POST /api/auth/github/exchange  用一次性 ticket 换本站令牌（令牌不落 URL）

import (
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"sync"
	"time"
)

var (
	ghStates  sync.Map
	ghTickets sync.Map
	ghHTTP    = &http.Client{Timeout: 15 * time.Second}

	errGHBanned     = errors.New("账号已被停用")
	errGHEmailTaken = errors.New("邮箱已绑定其他 GitHub 账号")
)

// ---------------- 运行期状态（进程内，重启失效可接受） ----------------

type ghStateVal struct {
	exp    time.Time
	invite string
}

type ghTicketVal struct {
	exp   time.Time
	sess  string
	user  map[string]interface{}
	isNew bool
}

// ghReady 配置齐全才启用（未配 Secret 时前端按钮自动隐藏）
func ghReady() bool { return cfg.GitHubID != "" && cfg.GitHubSecret != "" }

func ghGC() {
	now := time.Now()
	ghStates.Range(func(k, v interface{}) bool {
		if v.(ghStateVal).exp.Before(now) {
			ghStates.Delete(k)
		}
		return true
	})
	ghTickets.Range(func(k, v interface{}) bool {
		if v.(ghTicketVal).exp.Before(now) {
			ghTickets.Delete(k)
		}
		return true
	})
}

// ghBase 站点根地址，从回调地址推导，避免多一个配置项
func ghBase() string {
	s := cfg.GitHubRedirect
	if i := strings.Index(s, "/api/"); i > 0 {
		return s[:i]
	}
	return cfg.PublicURL
}

func ghFail(w http.ResponseWriter, r *http.Request, code string) {
	http.Redirect(w, r, ghBase()+"/?gh_err="+url.QueryEscape(code), http.StatusFound)
}

// ---------------- 1. 发起授权 ----------------

func handleGitHubStart(w http.ResponseWriter, r *http.Request) {
	if cors(w, r) {
		return
	}
	if !methodGuard(w, r, "GET") {
		return
	}
	if !ghReady() {
		writeErr(w, 503, "GitHub 登录暂未启用")
		return
	}
	ghGC()
	invite := strings.TrimSpace(r.URL.Query().Get("invite"))
	if len(invite) > 16 {
		invite = invite[:16]
	}
	state := randHex(20)
	ghStates.Store(state, ghStateVal{exp: time.Now().Add(10 * time.Minute), invite: invite})

	q := url.Values{}
	q.Set("client_id", cfg.GitHubID)
	q.Set("redirect_uri", cfg.GitHubRedirect)
	q.Set("scope", "read:user user:email")
	q.Set("state", state)
	q.Set("allow_signup", "true")
	ok(w, map[string]string{"url": "https://github.com/login/oauth/authorize?" + q.Encode()})
}

// ---------------- 2. GitHub 回跳 ----------------

func handleGitHubCallback(w http.ResponseWriter, r *http.Request) {
	q := r.URL.Query()
	if q.Get("error") != "" {
		ghFail(w, r, "denied")
		return
	}
	st := q.Get("state")
	v, found := ghStates.Load(st)
	if !found || st == "" {
		ghFail(w, r, "state")
		return
	}
	ghStates.Delete(st) // 一次性，无论后续成败
	sv, _ := v.(ghStateVal)
	if time.Now().After(sv.exp) {
		ghFail(w, r, "state")
		return
	}
	code := strings.TrimSpace(q.Get("code"))
	if code == "" {
		ghFail(w, r, "nocode")
		return
	}

	tok, err := ghExchangeToken(code)
	if err != nil {
		log.Printf("[gh] 换 token 失败: %v ip=%s", err, clientIP(r))
		ghFail(w, r, "token")
		return
	}
	var gu ghUser
	if err := ghAPIGet("/user", tok, &gu); err != nil || gu.ID == 0 {
		log.Printf("[gh] 拉取用户失败: %v", err)
		ghFail(w, r, "user")
		return
	}
	var mails []ghEmail
	_ = ghAPIGet("/user/emails", tok, &mails)
	email, verified := ghPrimaryEmail(&gu, mails)

	u, isNew, err := ghProvisionUser(&gu, email, verified, sv.invite)
	if err != nil {
		switch err {
		case errGHBanned:
			ghFail(w, r, "banned")
		case errGHEmailTaken:
			ghFail(w, r, "email_taken")
		default:
			log.Printf("[gh] 建号失败: %v user=%s", err, gu.Login)
			ghFail(w, r, "create")
		}
		return
	}

	now := time.Now()
	u.LastLogin = &now
	db.Model(&User{}).Where("id = ?", u.ID).Update("last_login", now)

	ticket := randHex(24)
	tv := ghTicketVal{
		exp:   time.Now().Add(90 * time.Second),
		user:  publicUser(&u),
		isNew: isNew,
	}
	sess := signToken(u.ID, u.IsAdmin)
	tv.sess = sess
	ghTickets.Store(ticket, tv)
	http.Redirect(w, r, ghBase()+"/?gh_ticket="+ticket, http.StatusFound)
}

// ghProvisionUser 命中已有账号（github_id 优先、已验证邮箱次之），否则新建
// 返回：用户、是否新注册、错误
func ghProvisionUser(gu *ghUser, email string, verified bool, invite string) (User, bool, error) {
	var u User

	// ① 已绑定过这个 GitHub 账号
	if err := db.First(&u, "github_id = ?", gu.ID).Error; err == nil {
		if u.Banned {
			return u, false, errGHBanned
		}
		if u.GitHubLogin != gu.Login {
			db.Model(&User{}).Where("id = ?", u.ID).Update("github_login", gu.Login)
			u.GitHubLogin = gu.Login
		}
		return u, false, nil
	}

	// ② 邮箱命中已有账号 → 自动绑定，避免同一人开两个号刷注册礼
	if email != "" && verified {
		var byMail User
		if err := db.First(&byMail, "email = ?", email).Error; err == nil {
			if byMail.GitHubID != 0 && byMail.GitHubID != gu.ID {
				return byMail, false, errGHEmailTaken
			}
			if byMail.Banned {
				return byMail, false, errGHBanned
			}
			upd := map[string]interface{}{"github_id": gu.ID, "github_login": gu.Login}
			// GitHub 已确认该邮箱归属 → 顺手把站内邮箱标为已验证
			if !byMail.EmailVerified {
				upd["email_verified"] = true
				byMail.EmailVerified = true
			}
			if err := db.Model(&User{}).Where("id = ?", byMail.ID).Updates(upd).Error; err != nil {
				return byMail, false, err
			}
			byMail.GitHubID, byMail.GitHubLogin = gu.ID, gu.Login
			log.Printf("[gh] 邮箱命中，自动绑定 uid=%d user=%s", byMail.ID, byMail.Username)
			return byMail, false, nil
		}
	}

	// ③ 全新用户 → 建号（复用注册礼 / 邀请返点规则）
	tmp := verified && isTempEmail(email)
	bonus := !tmp
	// 防刷闸：刚创建且空内容的 GitHub 号不发礼包（管理台 setting gh_newbie_guard=0 可关）
	if bonus && getSetting("gh_newbie_guard", "1") == "1" && !gu.CreatedAt.IsZero() &&
		time.Since(gu.CreatedAt) < 7*24*time.Hour && gu.PublicRepos == 0 && gu.Followers == 0 {
		bonus = false
	}
	nu := User{
		Username:      ghUniqueUsername(gu.Login),
		Email:         email,
		EmailVerified: verified,
		EmailIsTemp:   tmp,
		GitHubID:      gu.ID,
		GitHubLogin:   gu.Login,
		InviteCode:    genInviteCode(),
		InvitedBy:     strings.TrimSpace(invite),
	}
	if bonus {
		nu.BonusPoints = cfg.SignupPoints
		until := time.Now().AddDate(0, 0, cfg.SignupDays)
		nu.BonusUntil = &until
	}
	if err := db.Create(&nu).Error; err != nil {
		return nu, false, err
	}
	// 邀请返点（与 handleRegister 一致：临时邮箱不发）
	if nu.InvitedBy != "" && bonus {
		var inv User
		if err := db.First(&inv, "invite_code = ?", nu.InvitedBy).Error; err == nil && !inv.EmailIsTemp && inv.ID != nu.ID {
			inv.BonusPoints += 500
			db.Save(&inv)
		}
	}
	log.Printf("[gh] 注册成功 uid=%d user=%s email=%s bonus=%v ip=oauth", nu.ID, nu.Username, maskEmail(email), bonus)
	return nu, true, nil
}

// ---------------- 3. ticket 换令牌 ----------------

func handleGitHubExchange(w http.ResponseWriter, r *http.Request) {
	if cors(w, r) {
		return
	}
	if !methodGuard(w, r, "POST") {
		return
	}
	var in struct {
		Ticket string `json:"ticket"`
	}
	_ = json.NewDecoder(io.LimitReader(r.Body, 4096)).Decode(&in)
	t := strings.TrimSpace(in.Ticket)
	if t == "" {
		t = r.URL.Query().Get("ticket")
	}
	v, found := ghTickets.LoadAndDelete(t) // 一次性
	if !found {
		writeErr(w, 400, "登录链接已失效，请重新点击 GitHub 登录")
		return
	}
	tv := v.(ghTicketVal)
	if time.Now().After(tv.exp) {
		writeErr(w, 400, "登录链接已过期，请重新点击 GitHub 登录")
		return
	}
	resp := map[string]interface{}{"user": tv.user, "is_new": tv.isNew}
	resp["token"] = tv.sess
	ok(w, resp)
}

// ---------------- GitHub API ----------------

type ghUser struct {
	ID          int64     `json:"id"`
	Login       string    `json:"login"`
	Name        string    `json:"name"`
	Email       string    `json:"email"`
	CreatedAt   time.Time `json:"created_at"`
	PublicRepos int       `json:"public_repos"`
	Followers   int       `json:"followers"`
}

type ghEmail struct {
	Email    string `json:"email"`
	Primary  bool   `json:"primary"`
	Verified bool   `json:"verified"`
}

func ghExchangeToken(code string) (string, error) {
	form := url.Values{}
	form.Set("client_id", cfg.GitHubID)
	form.Set("client_secret", cfg.GitHubSecret)
	form.Set("code", code)
	form.Set("redirect_uri", cfg.GitHubRedirect)
	req, err := http.NewRequest("POST", "https://github.com/login/oauth/access_token", strings.NewReader(form.Encode()))
	if err != nil {
		return "", err
	}
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	req.Header.Set("Accept", "application/json")
	resp, err := ghHTTP.Do(req)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()
	b, _ := io.ReadAll(io.LimitReader(resp.Body, 1<<16))
	var out struct {
		AccessToken string `json:"access_token"`
		Error       string `json:"error"`
		Desc        string `json:"error_description"`
	}
	if err := json.Unmarshal(b, &out); err != nil {
		return "", err
	}
	if out.AccessToken == "" {
		return "", fmt.Errorf("github oauth: %s %s", out.Error, out.Desc)
	}
	return out.AccessToken, nil
}

func ghAPIGet(path, tok string, out interface{}) error {
	req, err := http.NewRequest("GET", "https://api.github.com"+path, nil)
	if err != nil {
		return err
	}
	req.Header.Set("Accept", "application/vnd.github+json")
	req.Header.Set("Authorization", "Bearer "+tok)
	req.Header.Set("User-Agent", "wb2-oauth")
	resp, err := ghHTTP.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	if resp.StatusCode/100 != 2 {
		return fmt.Errorf("github api %s: %d", path, resp.StatusCode)
	}
	b, _ := io.ReadAll(io.LimitReader(resp.Body, 1<<20))
	return json.Unmarshal(b, out)
}

// ghPrimaryEmail 取 primary+verified 的邮箱；没有就退化成唯一的 noreply 地址
func ghPrimaryEmail(gu *ghUser, mails []ghEmail) (string, bool) {
	for _, m := range mails {
		e := normalizeEmail(m.Email)
		if m.Primary && m.Verified && validEmail(e) {
			return e, true
		}
	}
	slug := ghSlug(gu.Login)
	if slug == "" {
		slug = "user"
	}
	return strconv.FormatInt(gu.ID, 10) + "+" + slug + "@users.noreply.github.com", false
}

// ghSlug 把 GitHub login 洗成站内用户名可用的字符
func ghSlug(login string) string {
	var b strings.Builder
	for _, c := range strings.ToLower(login) {
		switch {
		case c >= 'a' && c <= 'z', c >= '0' && c <= '9', c == '_', c == '-':
			b.WriteRune(c)
		default:
			b.WriteByte('_')
		}
	}
	s := b.String()
	if len(s) > 24 {
		s = s[:24]
	}
	return s
}

// ghUniqueUsername 站内用户名 3–24 字符，冲突加 _2/_3…
func ghUniqueUsername(login string) string {
	base := ghSlug(login)
	if len(base) < 3 {
		base = "gh_" + base
	}
	if len(base) > 20 {
		base = base[:20]
	}
	cand := base
	for i := 2; i < 100; i++ {
		var cnt int64
		db.Model(&User{}).Where("username = ?", cand).Count(&cnt)
		if cnt == 0 {
			return cand
		}
		cand = base + "_" + strconv.Itoa(i)
	}
	return base + "_" + randHex(3)
}
