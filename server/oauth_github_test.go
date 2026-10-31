package main

// oauth_github_test.go — GitHub 建号 / 绑定 / 防刷 / ticket 的离线单测（不联网）

import (
	"net/http/httptest"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func TestGitHubProvision(t *testing.T) {
	openDB(filepath.Join(t.TempDir(), "gh.db"))
	cfg.SignupPoints = 3000
	cfg.SignupDays = 7

	// ---- 工具函数 ----
	if got := ghSlug("Lszyk-26_X"); got != "lszyk-26_x" {
		t.Fatalf("ghSlug = %q", got)
	}
	gu0 := &ghUser{ID: 7, Login: "octo"}
	if e, okv := ghPrimaryEmail(gu0, []ghEmail{{Email: "A@B.com", Primary: true, Verified: true}}); e != "a@b.com" || !okv {
		t.Fatalf("primary email = %q %v", e, okv)
	}
	if e, okv := ghPrimaryEmail(gu0, nil); okv || e != "7+octo@users.noreply.github.com" {
		t.Fatalf("fallback email = %q %v", e, okv)
	}

	old := time.Now().AddDate(-3, 0, 0)
	inviter := User{Username: "inviter", Email: "inv@x.com", InviteCode: "ABC123"}
	if err := db.Create(&inviter).Error; err != nil {
		t.Fatal(err)
	}

	// ---- ① 新用户建号 + 邀请返点 ----
	gu1 := &ghUser{ID: 1001, Login: "Octo-Cat", CreatedAt: old, PublicRepos: 5}
	u1, isNew, err := ghProvisionUser(gu1, "octo@x.com", true, "ABC123")
	if err != nil || !isNew {
		t.Fatalf("建号失败 new=%v err=%v", isNew, err)
	}
	if u1.Username != "octo-cat" || u1.Email != "octo@x.com" || !u1.EmailVerified || u1.GitHubID != 1001 {
		t.Fatalf("建号字段不对: %+v", u1)
	}
	if u1.BonusPoints != 3000 || u1.BonusUntil == nil {
		t.Fatalf("注册礼没发: %v", u1.BonusPoints)
	}
	var inv User
	db.First(&inv, inviter.ID)
	if inv.BonusPoints != 500 {
		t.Fatalf("邀请返点 = %v", inv.BonusPoints)
	}

	// ---- ② 同一 GitHub 再登录：不新开号，login 改名要同步 ----
	gu1.Login = "octo-cat2"
	u2, isNew2, err := ghProvisionUser(gu1, "octo@x.com", true, "")
	if err != nil || isNew2 || u2.ID != u1.ID {
		t.Fatalf("复登异常 id=%d new=%v err=%v", u2.ID, isNew2, err)
	}
	if u2.GitHubLogin != "octo-cat2" {
		t.Fatalf("改名未同步: %q", u2.GitHubLogin)
	}

	// ---- ③ 邮箱命中老账号 → 自动绑定，不新开号，并标记邮箱已验证 ----
	plain := User{Username: "plainuser", Email: "plain@x.com", PassHash: hashPass("secret1"), InviteCode: "PLAIN1"}
	if err := db.Create(&plain).Error; err != nil {
		t.Fatal(err)
	}
	gu3 := &ghUser{ID: 2002, Login: "plainuser", CreatedAt: old, PublicRepos: 1}
	u3, isNew3, err := ghProvisionUser(gu3, "plain@x.com", true, "")
	if err != nil || isNew3 || u3.ID != plain.ID {
		t.Fatalf("绑定失败 id=%d new=%v err=%v", u3.ID, isNew3, err)
	}
	if u3.GitHubID != 2002 || !u3.EmailVerified {
		t.Fatalf("绑定字段不对: %+v", u3)
	}
	var cnt int64
	db.Model(&User{}).Where("email = ?", "plain@x.com").Count(&cnt)
	if cnt != 1 {
		t.Fatalf("邮箱重复建号 cnt=%d", cnt)
	}

	// ---- ④ 用户名撞车 ----
	gu4 := &ghUser{ID: 3003, Login: "plainuser", CreatedAt: old}
	u4, isNew4, err := ghProvisionUser(gu4, "", false, "")
	if err != nil || !isNew4 {
		t.Fatalf("撞名建号失败: %v", err)
	}
	if u4.Username != "plainuser_2" {
		t.Fatalf("撞名后缀 = %q", u4.Username)
	}

	// ---- ⑤ 防刷闸：刚建的空 GitHub 号不发礼包 ----
	gu5 := &ghUser{ID: 4004, Login: "brandnew", CreatedAt: time.Now().Add(-2 * time.Hour)}
	u5, _, err := ghProvisionUser(gu5, "new@x.com", true, "")
	if err != nil {
		t.Fatalf("guard 建号失败: %v", err)
	}
	if u5.BonusPoints != 0 {
		t.Fatalf("防刷闸无效: %v", u5.BonusPoints)
	}

	// ---- ⑥ 封禁账号拦截 ----
	db.Model(&User{}).Where("id = ?", u1.ID).Update("banned", true)
	if _, _, err := ghProvisionUser(gu1, "octo@x.com", true, ""); err != errGHBanned {
		t.Fatalf("封禁未拦截: %v", err)
	}
}

// TestGitHubExchangeTicket ticket 必须一次性、过期要拦
func TestGitHubExchangeTicket(t *testing.T) {
	openDB(filepath.Join(t.TempDir(), "gh2.db"))
	cfg.JWTSecret = "test-only-placeholder-not-a-real-secret"

	u := User{Username: "tickuser", InviteCode: "T1"}
	if err := db.Create(&u).Error; err != nil {
		t.Fatal(err)
	}
	tk := randHex(24)
	ghTickets.Store(tk, ghTicketVal{
		exp:   time.Now().Add(time.Minute),
		sess:  signToken(u.ID, false),
		user:  map[string]interface{}{"username": "tickuser"},
		isNew: true,
	})

	call := func() *httptest.ResponseRecorder {
		rec := httptest.NewRecorder()
		req := httptest.NewRequest("POST", "/api/auth/github/exchange",
			strings.NewReader(`{"ticket":"`+tk+`"}`))
		req.Header.Set("Content-Type", "application/json")
		handleGitHubExchange(rec, req)
		return rec
	}

	if rec := call(); rec.Code != 200 || !strings.Contains(rec.Body.String(), "tickuser") {
		t.Fatalf("首次交换失败 code=%d body=%s", rec.Code, rec.Body.String())
	}
	if rec := call(); rec.Code != 400 {
		t.Fatalf("ticket 未一次性生效: %d", rec.Code)
	}

	// 过期 ticket
	tk2 := randHex(24)
	ghTickets.Store(tk2, ghTicketVal{exp: time.Now().Add(-time.Second), sess: "x", user: map[string]interface{}{}})
	rec := httptest.NewRecorder()
	handleGitHubExchange(rec, httptest.NewRequest("POST", "/api/auth/github/exchange",
		strings.NewReader(`{"ticket":"`+tk2+`"}`)))
	if rec.Code != 400 {
		t.Fatalf("过期 ticket 未拦截: %d", rec.Code)
	}
}
