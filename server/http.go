package main

// http.go — 响应helper / CORS / 路由表

import (
	"encoding/json"
	"fmt"
	"net/http"
	"os"
	"path"
	"path/filepath"
	"strings"
)

func writeJSON(w http.ResponseWriter, code int, v interface{}) {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.WriteHeader(code)
	_ = json.NewEncoder(w).Encode(v)
}

func writeErr(w http.ResponseWriter, code int, msg string) {
	writeJSON(w, code, map[string]interface{}{
		"error": map[string]interface{}{"message": msg, "code": code},
	})
}

// 统一响应风格：直接返回对象
func ok(w http.ResponseWriter, v interface{}) { writeJSON(w, 200, v) }

func fmtSscan(s string, out *float64) (int, error) {
	var f float64
	n, err := fmt.Sscanf(s, "%f", &f)
	if err == nil {
		*out = f
	}
	return n, err
}

// CORS：同源部署时不需要，但留出本地开发 / 面板调试的能力
func cors(w http.ResponseWriter, r *http.Request) bool {
	origin := r.Header.Get("Origin")
	if origin == "" {
		return false
	}
	allow := getSetting("cors_origin", cfg.PublicURL)
	okOrigin := false
	for _, o := range strings.Split(allow, ",") {
		if strings.TrimSpace(o) == origin {
			okOrigin = true
		}
	}
	if !okOrigin && !strings.HasPrefix(origin, "http://127.0.0.1") && !strings.HasPrefix(origin, "http://localhost") {
		return false
	}
	w.Header().Set("Access-Control-Allow-Origin", origin)
	w.Header().Set("Access-Control-Allow-Credentials", "true")
	w.Header().Set("Access-Control-Allow-Headers", "Authorization,Content-Type,Accept")
	w.Header().Set("Access-Control-Allow-Methods", "GET,POST,PUT,DELETE,OPTIONS")
	w.Header().Set("Vary", "Origin")
	if r.Method == http.MethodOptions {
		w.WriteHeader(204)
		return true
	}
	return false
}

func methodGuard(w http.ResponseWriter, r *http.Request, want ...string) bool {
	for _, m := range want {
		if r.Method == m {
			return true
		}
	}
	writeErr(w, 405, "方法不允许")
	return false
}

func mux() *http.ServeMux {
	m := http.NewServeMux()
	P := func(path string, h http.HandlerFunc) { m.HandleFunc(path, h) }
	A := func(path string, h http.HandlerFunc) { m.HandleFunc(path, authMiddleware(h, false)) }
	AD := func(path string, h http.HandlerFunc) { m.HandleFunc(path, authMiddleware(h, true)) }

	// ---- 公开 ----
	P("/api/health", handleHealth)
	P("/api/plans", handlePlansPublic)
	P("/api/status", handleStatusPublic)
	P("/api/register", handleRegister)
	P("/api/login", handleLogin)
	P("/api/auth/code", handleSendCode)
	P("/api/auth/email-login", handleEmailLogin)
	P("/api/auth/reset", handleResetPassword)
	P("/api/auth/github/start", handleGitHubStart)
	P("/api/auth/github/callback", handleGitHubCallback)
	P("/api/auth/github/exchange", handleGitHubExchange)
	P("/api/pay/channels", handlePayChannels)
	P("/api/pay/notify/", handlePayNotify)
	P("/api/pay/return", handlePayReturn)
	// 抽奖公示：未登录也能看概率与奖池，登录后带个人余额
	P("/api/lottery/config", optionalAuth(handleLotteryConfig))

	// ---- 用户 ----
	A("/api/me", handleMe)
	A("/api/me/overview", handleOverview)
	A("/api/me/usage", handleUsage)
	A("/api/me/logs", handleLogs)
	A("/api/me/keys", handleKeys)
	A("/api/me/keys/delete", handleKeyDelete)
	A("/api/me/keys/rotate", handleKeyRotate)
	A("/api/me/redeem", handleRedeem)
	A("/api/me/orders", handleMyOrders)
	A("/api/me/order/status", handleOrderStatus)
	A("/api/me/order", handleCreateOrder)
	A("/api/me/accounts", handleMyAccounts)
	A("/api/me/account/submit", handleSubmitAccount)
	A("/api/me/account/oauth/start", handleOAuthStart)
	A("/api/me/account/oauth/poll", handleOAuthPoll)
	A("/api/me/invite", handleMyInvite)
	A("/api/me/account/delete", handleDeleteAccount)
	A("/api/me/password", handleChangePassword)
	A("/api/me/student", handleStudentVerify)
	A("/api/playground/chat", handlePlaygroundChat)

	// ---- 抽奖 ----
	A("/api/lottery/draw", handleLotteryDraw)
	A("/api/lottery/history", handleLotteryHistory)

	// ---- OpenAI 兼容 ----
	P("/v1/models", handleV1Models)
	P("/v1/chat/completions", handleChatCompletions)
	P("/v1/completions", handleChatCompletions)
	P("/v1/messages", handleChatCompletions)

	// ---- 管理 ----
	AD("/api/admin/stats", handleAdminStats)
	AD("/api/admin/users", handleAdminUsers)
	AD("/api/admin/user/plan", handleAdminSetPlan)
	AD("/api/admin/user/ban", handleAdminBan)
	AD("/api/admin/user/points", handleAdminPoints)
	AD("/api/admin/orders", handleAdminOrders)
	AD("/api/admin/order/review", handleAdminOrderReview)
	AD("/api/admin/codes", handleAdminCodes)
	AD("/api/admin/code/gen", handleAdminGenCode)
	AD("/api/admin/accounts", handleAdminAccounts)
	AD("/api/admin/account/review", handleAdminAccountReview)
	AD("/api/admin/plans", handleAdminPlans)
	AD("/api/admin/plan/save", handleAdminPlanSave)
	AD("/api/admin/pool", handleAdminPool)
	AD("/api/admin/settings", handleAdminSettings)
	AD("/api/admin/settle", handleAdminSettle)
	AD("/api/admin/paylogs", handleAdminPayLogs)
	AD("/api/admin/health", handleAdminHealth)

	// ---- 抽奖管理 ----
	AD("/api/admin/lottery/prizes", handleAdminLotteryPrizes)
	AD("/api/admin/lottery/prize/save", handleAdminLotteryPrizeSave)
	AD("/api/admin/lottery/prize/toggle", handleAdminLotteryPrizeToggle)
	AD("/api/admin/lottery/stock/reset", handleAdminLotteryStockReset)
	AD("/api/admin/lottery/draws", handleAdminLotteryDraws)
	AD("/api/admin/lottery/stats", handleAdminLotteryStats)
	AD("/api/admin/lottery/clawback", handleAdminLotteryClawback)
	AD("/api/admin/lottery/settings", handleAdminLotterySettings)

	// ---- 静态前端（有 dist 就一并托管，省掉 nginx）----
	P("/", handleStatic)

	return m
}

// handleStatic 托管 dist/，找不到的路径回退 index.html（SPA 路由）
func handleStatic(w http.ResponseWriter, r *http.Request) {
	if strings.HasPrefix(r.URL.Path, "/api/") || strings.HasPrefix(r.URL.Path, "/v1/") {
		writeErr(w, 404, "not_found")
		return
	}
	dir := cfg.StaticDir
	if dir == "" {
		writeErr(w, 404, "not_found")
		return
	}
	p := strings.TrimPrefix(path.Clean(r.URL.Path), "/")
	if p == "" {
		p = "index.html"
	}
	full := filepath.Join(dir, p)
	if st, err := os.Stat(full); err == nil && !st.IsDir() {
		if strings.HasPrefix(p, "assets/") {
			// Vite 产物文件名带 hash，可长缓存
			w.Header().Set("Cache-Control", "public, max-age=31536000, immutable")
		} else {
			w.Header().Set("Cache-Control", "no-cache")
		}
		http.ServeFile(w, r, full)
		return
	}
	idx := filepath.Join(dir, "index.html")
	if _, err := os.Stat(idx); err != nil {
		writeErr(w, 404, "前端未构建：请先 npm run build，或设置 WB2_STATIC_DIR")
		return
	}
	w.Header().Set("Cache-Control", "no-store")
	http.ServeFile(w, r, idx)
}

func handleHealth(w http.ResponseWriter, r *http.Request) {
	up := upstreamHealth()
	ok(w, map[string]interface{}{
		"ok":       true,
		"service":  "wb2api-v2",
		"upstream": up,
		"time":     nowISO(),
	})
}
