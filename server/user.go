package main

// user.go — 用户侧接口：概览 / 用量 / 日志 / 密钥 / 兑换 / 订单 / 授权账号

import (
	"encoding/json"
	"io"
	"net/http"
	"strconv"
	"strings"
	"time"
)

var _ = io.Discard

func nowISO() string { return time.Now().Format(time.RFC3339) }

func ftoa(f float64) string { return strconv.FormatFloat(f, 'f', -1, 64) }

// ---------------- /api/me ----------------
func handleMe(w http.ResponseWriter, r *http.Request) {
	u := currentUser(r)
	quota := dailyQuota(u)
	rollDay(u)

	var keyCount int64
	db.Model(&APIKey{}).Where("user_id = ? AND revoked = ?", u.ID, false).Count(&keyCount)
	var accCount int64
	db.Model(&ContribAccount{}).Where("user_id = ? AND state IN ?", u.ID, []string{"ok", "pending", "cool"}).Count(&accCount)

	conc, rpm, logDays := limitsOf(u)
	out := publicUser(u)
	pi := planInfoOf(u)
	out["contrib_plan"] = pi.Contrib.Code
	out["contrib_accounts"] = okContribCount(u.ID)
	out["sponsor_plan"] = pi.Sponsor.Code
	out["sponsor_until"] = pi.SponsorTill
	out["stacked"] = pi.Stacked
	out["quota"] = map[string]interface{}{
		"daily":          quota,
		"used_today":     round2(u.PointsToday),
		"left_today":     round2(max0(quota - u.PointsToday)),
		"bonus":          round2(u.BonusPoints),
		"lottery_points": round2(u.LotteryPoints),
		"bonus_until":    u.BonusUntil,
		"reset_at":       nextMidnight(),
		"soft_limit":     round2(quota * 1.5),
		"concurrency":    conc,
		"rpm":            rpm,
		"log_days":       logDays,
	}
	out["counts"] = map[string]interface{}{
		"keys":     keyCount,
		"accounts": accCount,
	}
	ok(w, out)
}

func nextMidnight() string {
	n := time.Now()
	t := time.Date(n.Year(), n.Month(), n.Day(), 0, 0, 0, 0, n.Location()).AddDate(0, 0, 1)
	return t.Format(time.RFC3339)
}

// ---------------- /api/me/overview ----------------
func handleOverview(w http.ResponseWriter, r *http.Request) {
	u := currentUser(r)
	rollDay(u)
	quota := dailyQuota(u)

	// 近 24 小时按小时聚合
	now := time.Now()
	since := now.Add(-24 * time.Hour)
	type agg struct {
		Hour   string  `json:"hour"`
		Calls  int64   `json:"calls"`
		Points float64 `json:"points"`
		Errors int64   `json:"errors"`
	}
	var rows []agg
	db.Model(&LogRow{}).
		Select("strftime('%H', created_at) as hour, count(*) as calls, sum(points) as points, sum(case when status >= 400 then 1 else 0 end) as errors").
		Where("user_id = ? AND created_at >= ?", u.ID, since).
		Group("hour").Order("hour").Scan(&rows)
	byHour := map[string]agg{}
	for _, a := range rows {
		byHour[a.Hour] = a
	}
	series := make([]map[string]interface{}, 0, 24)
	for i := 23; i >= 0; i-- {
		t := now.Add(-time.Duration(i) * time.Hour)
		k := t.Format("15")
		a := byHour[k]
		series = append(series, map[string]interface{}{
			"hour": t.Format("01-02 15:00"), "calls": a.Calls, "points": round2(a.Points), "errors": a.Errors,
		})
	}

	// 今日 / 昨日
	var todayCalls, todayErr int64
	var todayPts float64
	dayStart := time.Date(now.Year(), now.Month(), now.Day(), 0, 0, 0, 0, now.Location())
	db.Model(&LogRow{}).Where("user_id = ? AND created_at >= ?", u.ID, dayStart).
		Select("count(*) as c, coalesce(sum(points),0) as p, sum(case when status>=400 then 1 else 0 end) as e").
		Row().Scan(&todayCalls, &todayPts, &todayErr)

	var ydayCalls int64
	var ydayPts float64
	db.Model(&LogRow{}).Where("user_id = ? AND created_at >= ? AND created_at < ?", u.ID, dayStart.AddDate(0, 0, -1), dayStart).
		Select("count(*) as c, coalesce(sum(points),0) as p").Row().Scan(&ydayCalls, &ydayPts)

	// 按模型
	type mrow struct {
		Model  string  `json:"model"`
		Calls  int64   `json:"calls"`
		Points float64 `json:"points"`
		Tokens int64   `json:"tokens"`
	}
	var byModel []mrow
	db.Model(&LogRow{}).Where("user_id = ? AND created_at >= ?", u.ID, dayStart.AddDate(0, 0, -7)).
		Select("model, count(*) as calls, coalesce(sum(points),0) as points, coalesce(sum(prompt_tokens+completion_tokens),0) as tokens").
		Group("model").Order("calls desc").Limit(8).Scan(&byModel)

	// 最近日志
	logs := recentLogs(u, 8)

	// 池状态
	pool := upstreamHealth()

	ok(w, map[string]interface{}{
		"user":     publicUser(u),
		"quota":    map[string]interface{}{"daily": quota, "used_today": round2(u.PointsToday), "left_today": round2(max0(quota - u.PointsToday)), "bonus": round2(u.BonusPoints), "lottery_points": round2(u.LotteryPoints), "bonus_until": u.BonusUntil, "reset_at": nextMidnight()},
		"today":    map[string]interface{}{"calls": todayCalls, "points": round2(todayPts), "errors": todayErr, "prev_calls": ydayCalls, "prev_points": round2(ydayPts)},
		"series":   series,
		"by_model": byModel,
		"logs":     logs,
		"pool":     pool,
	})
}

func recentLogs(u *User, limit int) []map[string]interface{} {
	var rows []LogRow
	_, _, logDays := limitsOf(u)
	since := time.Now().AddDate(0, 0, -maxInt(logDays, 1))
	db.Where("user_id = ? AND created_at >= ?", u.ID, since).Order("id desc").Limit(limit).Find(&rows)
	out := make([]map[string]interface{}, 0, len(rows))
	for _, r := range rows {
		out = append(out, map[string]interface{}{
			"id": r.ID, "model": r.Model, "prompt_tokens": r.PromptTok, "completion_tokens": r.ComplTok,
			"points": round2(r.Points), "status": r.Status, "error": r.Error,
			"ms": r.Ms, "ttft_ms": r.TTFT, "stream": r.Stream, "at": r.CreatedAt.Format("01-02 15:04:05"),
			"ago": humanAgo(r.CreatedAt),
		})
	}
	return out
}

func maxInt(a, b int) int {
	if a > b {
		return a
	}
	return b
}

func humanAgo(t time.Time) string {
	d := time.Since(t)
	switch {
	case d < time.Minute:
		return "刚刚"
	case d < time.Hour:
		return strconv.Itoa(int(d.Minutes())) + " 分钟前"
	case d < 24*time.Hour:
		return strconv.Itoa(int(d.Hours())) + " 小时前"
	}
	return strconv.Itoa(int(d.Hours()/24)) + " 天前"
}

// ---------------- /api/me/usage ----------------
func handleUsage(w http.ResponseWriter, r *http.Request) {
	u := currentUser(r)
	days := 7
	if v := r.URL.Query().Get("days"); v != "" {
		if n, err := strconv.Atoi(v); err == nil && n >= 1 && n <= 90 {
			days = n
		}
	}
	since := time.Now().AddDate(0, 0, -days)
	type drow struct {
		Day    string  `json:"day"`
		Calls  int64   `json:"calls"`
		Points float64 `json:"points"`
		Tokens int64   `json:"tokens"`
		Errors int64   `json:"errors"`
	}
	var rows []drow
	db.Model(&LogRow{}).Where("user_id = ? AND created_at >= ?", u.ID, since).
		Select("strftime('%Y-%m-%d', created_at) as day, count(*) as calls, coalesce(sum(points),0) as points, coalesce(sum(prompt_tokens+completion_tokens),0) as tokens, sum(case when status>=400 then 1 else 0 end) as errors").
		Group("day").Order("day").Scan(&rows)

	var total struct {
		Calls  int64   `json:"calls"`
		Points float64 `json:"points"`
		Tokens int64   `json:"tokens"`
	}
	db.Model(&LogRow{}).Where("user_id = ? AND created_at >= ?", u.ID, since).
		Select("count(*) as calls, coalesce(sum(points),0) as points, coalesce(sum(prompt_tokens+completion_tokens),0) as tokens").
		Scan(&total)

	type mrow struct {
		Model  string  `json:"model"`
		Calls  int64   `json:"calls"`
		Points float64 `json:"points"`
		Tokens int64   `json:"tokens"`
	}
	var byModel []mrow
	db.Model(&LogRow{}).Where("user_id = ? AND created_at >= ?", u.ID, since).
		Select("model, count(*) as calls, coalesce(sum(points),0) as points, coalesce(sum(prompt_tokens+completion_tokens),0) as tokens").
		Group("model").Order("points desc").Limit(12).Scan(&byModel)

	ok(w, map[string]interface{}{
		"days":     rows,
		"total":    total,
		"by_model": byModel,
		"range":    days,
	})
}

// ---------------- /api/me/logs ----------------
func handleLogs(w http.ResponseWriter, r *http.Request) {
	u := currentUser(r)
	_, _, logDays := limitsOf(u)
	page, _ := strconv.Atoi(r.URL.Query().Get("page"))
	if page < 1 {
		page = 1
	}
	limit := 30
	since := time.Now().AddDate(0, 0, -maxInt(logDays, 1))
	q := db.Where("user_id = ? AND created_at >= ?", u.ID, since)
	if m := r.URL.Query().Get("model"); m != "" {
		q = q.Where("model = ?", m)
	}
	if s := r.URL.Query().Get("status"); s == "error" {
		q = q.Where("status >= 400")
	} else if s == "ok" {
		q = q.Where("status < 400")
	}
	var total int64
	q.Model(&LogRow{}).Count(&total)
	var rows []LogRow
	q.Order("id desc").Offset((page - 1) * limit).Limit(limit).Find(&rows)
	list := make([]map[string]interface{}, 0, len(rows))
	for _, l := range rows {
		list = append(list, map[string]interface{}{
			"id": l.ID, "model": l.Model, "prompt_tokens": l.PromptTok, "completion_tokens": l.ComplTok,
			"points": round2(l.Points), "status": l.Status, "error": l.Error, "ms": l.Ms,
			"ttft_ms": l.TTFT, "stream": l.Stream, "at": l.CreatedAt.Format("2006-01-02 15:04:05"),
		})
	}
	ok(w, map[string]interface{}{
		"total": total, "page": page, "limit": limit, "logs": list, "retain_days": logDays,
	})
}

// ---------------- 密钥 ----------------
func handleKeys(w http.ResponseWriter, r *http.Request) {
	u := currentUser(r)
	if r.Method == http.MethodPost {
		// 密钥数量按档位：游客 15 把，赞助/贡献档逐级加
		maxKeys := keyLimit(u.Plan)
		var n int64
		db.Model(&APIKey{}).Where("user_id = ? AND revoked = ?", u.ID, false).Count(&n)
		if n >= maxKeys {
			writeErr(w, 400, "当前档位最多 "+strconv.FormatInt(maxKeys, 10)+" 把密钥，请先吊销旧的")
			return
		}
		var in struct {
			Name string `json:"name"`
		}
		_ = json.NewDecoder(r.Body).Decode(&in)
		if in.Name == "" {
			in.Name = "默认密钥"
		}
		secret := "sk-wb-" + randHex(24)
		k := APIKey{
			UserID: u.ID, Name: in.Name,
			Prefix: secret[:14], Hash: hashKey(secret), Secret: encrypt(secret),
		}
		if err := db.Create(&k).Error; err != nil {
			writeErr(w, 500, "创建失败")
			return
		}
		ok(w, map[string]interface{}{"key": publicKey(&k), "secret": secret})
		return
	}
	var rows []APIKey
	db.Where("user_id = ?", u.ID).Order("id desc").Find(&rows)
	list := make([]map[string]interface{}, 0, len(rows))
	for i := range rows {
		list = append(list, publicKey(&rows[i]))
	}
	ok(w, map[string]interface{}{"keys": list, "max": keyLimit(u.Plan)})
}

// keyLimit 各档位可持有的 API 密钥数量上限（2026-10-24：普通用户 1 → 15）
func keyLimit(plan string) int64 {
	switch strings.ToLower(strings.TrimSpace(plan)) {
	case "":
		return 15 // 游客 / 普通用户
	case "lv1", "c1":
		return 30
	case "lv2", "c2":
		return 80
	case "lv3", "c3":
		return 200
	case "c4":
		return 500
	}
	return 15
}

func publicKey(k *APIKey) map[string]interface{} {
	return map[string]interface{}{
		"id": k.ID, "name": k.Name, "prefix": k.Prefix,
		"revoked": k.Revoked, "calls": k.Calls,
		"last_used": k.LastUsed, "created_at": k.CreatedAt,
	}
}

func handleKeyDelete(w http.ResponseWriter, r *http.Request) {
	u := currentUser(r)
	var in struct {
		ID uint `json:"id"`
	}
	_ = json.NewDecoder(r.Body).Decode(&in)
	db.Model(&APIKey{}).Where("id = ? AND user_id = ?", in.ID, u.ID).Update("revoked", true)
	ok(w, map[string]interface{}{"ok": true})
}

func handleKeyRotate(w http.ResponseWriter, r *http.Request) {
	u := currentUser(r)
	var in struct {
		ID uint `json:"id"`
	}
	_ = json.NewDecoder(r.Body).Decode(&in)
	var k APIKey
	if err := db.First(&k, "id = ? AND user_id = ?", in.ID, u.ID).Error; err != nil {
		writeErr(w, 404, "密钥不存在")
		return
	}
	secret := "sk-wb-" + randHex(24)
	db.Model(&APIKey{}).Where("id = ?", k.ID).Updates(map[string]interface{}{
		"prefix": secret[:14], "hash": hashKey(secret), "secret": encrypt(secret),
	})
	ok(w, map[string]interface{}{"secret": secret, "prefix": secret[:14]})
}

// ---------------- 卡密兑换 ----------------
func handleRedeem(w http.ResponseWriter, r *http.Request) {
	u := currentUser(r)
	var in struct {
		Code string `json:"code"`
	}
	_ = json.NewDecoder(r.Body).Decode(&in)
	code := strings.ToUpper(strings.TrimSpace(in.Code))
	var c RedeemCode
	if err := db.First(&c, "upper(code) = ?", code).Error; err != nil {
		writeErr(w, 404, "卡密不存在")
		return
	}
	if c.UsedBy != 0 {
		writeErr(w, 400, "这张卡密已被使用")
		return
	}
	if c.ExpireAt != nil && time.Now().After(*c.ExpireAt) {
		writeErr(w, 400, "卡密已过期")
		return
	}
	now := time.Now()
	db.Model(&RedeemCode{}).Where("id = ?", c.ID).Updates(map[string]interface{}{"used_by": u.ID, "used_at": now})
	switch c.Kind {
	case "points":
		u.BonusPoints += c.Points
		if u.BonusUntil == nil || u.BonusUntil.Before(now.AddDate(0, 0, 30)) {
			until := now.AddDate(0, 0, 90)
			u.BonusUntil = &until
		}
		db.Save(u)
		ok(w, map[string]interface{}{"ok": true, "kind": "points", "points": c.Points})
	case "plan":
		months := c.Months
		if months < 1 {
			months = 1
		}
		base := now
		if u.PlanUntil != nil && u.PlanUntil.After(now) {
			base = *u.PlanUntil
		}
		until := base.AddDate(0, months, 0)
		db.Model(&User{}).Where("id = ?", u.ID).Updates(map[string]interface{}{"plan": c.Plan, "plan_until": until})
		ok(w, map[string]interface{}{"ok": true, "kind": "plan", "plan": c.Plan, "months": months, "until": until.Format("2006-01-02")})
	default:
		writeErr(w, 400, "未知卡密类型")
	}
}

// ---------------- 订单 ----------------
func handleMyOrders(w http.ResponseWriter, r *http.Request) {
	u := currentUser(r)
	var rows []Order
	db.Where("user_id = ?", u.ID).Order("id desc").Limit(50).Find(&rows)
	ok(w, map[string]interface{}{"orders": rows})
}

func handleCreateOrder(w http.ResponseWriter, r *http.Request) {
	u := currentUser(r)
	var in struct {
		Plan   string `json:"plan"`
		Months int    `json:"months"`
		Method string `json:"method"` // alipay / alipay2 / wechat / manual
		Note   string `json:"note"`
	}
	_ = json.NewDecoder(r.Body).Decode(&in)
	in.Plan = strings.ToLower(strings.TrimSpace(in.Plan))
	in.Method = strings.ToLower(strings.TrimSpace(in.Method))
	p := planOf(in.Plan)
	if p.Code == "" || (p.Kind != "sponsor" && p.Kind != "points") {
		writeErr(w, 400, "档位不存在")
		return
	}
	if p.Kind == "points" {
		in.Months = 0 // 加油包不按月
	} else if in.Months != 12 {
		in.Months = 1
	}
	amount := p.Price
	if p.Kind == "sponsor" && in.Months == 12 {
		amount = p.YearPrice
	}
	// 学生 7 折
	if u.Student {
		if d, err := parseFloat(getSetting("student_discount", "0.7")); err == nil && d > 0 && d < 1 {
			amount = round2(amount * d)
		}
	}
	if amount <= 0 {
		writeErr(w, 400, "该档位不接受下单")
		return
	}

	o := Order{
		OrderNo: genOrderNo(),
		UserID:  u.ID, Username: u.Username, Plan: p.Code, Months: in.Months,
		Amount: amount, Status: "pending", Method: defaultStr(in.Method, "manual"), Note: in.Note,
	}
	db.Create(&o)

	// manual：等管理员确认；其他走真实支付
	if o.Method == "manual" || o.Method == "" {
		ok(w, map[string]interface{}{"order": o, "hint": "订单已创建，请联系管理员确认（或使用卡密兑换立即生效）"})
		return
	}

	ch := channelByCode(o.Method)
	if ch.Code == "" || !ch.Enabled {
		writeErr(w, 400, "该支付方式暂未开通，请用卡密或联系管理员")
		return
	}
	payType := "alipay"
	if ch.Code == "wechat" {
		payType = "wxpay"
	}
	res, err := createPayment(&o, ch, payType, clientIP(r))
	if err != nil {
		writeErr(w, 502, "创建支付失败："+err.Error())
		return
	}
	db.Model(&Order{}).Where("id = ?", o.ID).Update("pay_url", res.PayURL)
	ok(w, map[string]interface{}{"order": o, "pay": res})
}

func genOrderNo() string {
	return "W" + time.Now().Format("20060102150405") + strings.ToUpper(randHex(3))
}

func parseFloat(s string) (float64, error) {
	var f float64
	_, err := fmtSscan(s, &f)
	return f, err
}

// 订单状态轮询（前端扫码后 poll）
func handleOrderStatus(w http.ResponseWriter, r *http.Request) {
	u := currentUser(r)
	no := r.URL.Query().Get("order_no")
	var o Order
	if err := db.First(&o, "order_no = ? AND user_id = ?", no, u.ID).Error; err != nil {
		writeErr(w, 404, "订单不存在")
		return
	}
	ok(w, map[string]interface{}{
		"order_no": o.OrderNo, "status": o.Status, "plan": o.Plan,
		"months": o.Months, "amount": o.Amount, "paid_at": o.PaidAt,
		"pay_url": o.PayURL,
	})
}

// 可用支付通道（前端渲染按钮）
func handlePayChannels(w http.ResponseWriter, r *http.Request) {
	if cors(w, r) {
		return
	}
	out := []map[string]interface{}{}
	for _, c := range loadChannels() {
		if !c.Enabled {
			continue
		}
		out = append(out, map[string]interface{}{
			"code": c.Code, "name": c.Name, "driver": c.Driver,
		})
	}
	ok(w, map[string]interface{}{"channels": out, "manual": true})
}

func defaultStr(a, b string) string {
	if a == "" {
		return b
	}
	return a
}

// ---------------- 授权账号 ----------------
func handleMyAccounts(w http.ResponseWriter, r *http.Request) {
	u := currentUser(r)
	var rows []ContribAccount
	db.Where("user_id = ?", u.ID).Order("id desc").Find(&rows)
	list := make([]map[string]interface{}, 0, len(rows))
	for _, a := range rows {
		list = append(list, map[string]interface{}{
			"id": a.ID, "realm": a.Realm, "label": a.Label, "credits": a.Credits,
			"latency_ms": a.Latency, "state": a.State, "note": a.Note,
			"last_seen": a.LastSeen, "created_at": a.CreatedAt,
		})
	}
	// 当前折算：贡献档按有效账号数算、赞助档按已支付订单算（users.plan 只是展示）
	pi := planInfoOf(u)
	var okN int64
	for _, a := range rows {
		if a.State == "ok" {
			okN++
		}
	}
	ratio := u.ContribRatio
	if pi.Contrib.Code != "" {
		ratio = pi.Contrib.Ratio
	}
	ok(w, map[string]interface{}{
		"accounts":      list,
		"ratio":         ratio,
		"tier":          pi.Contrib.Code,
		"ok_accounts":   okN,
		"contrib_plan":  pi.Contrib.Code,
		"contrib_daily": pi.Contrib.Daily,
		"sponsor_plan":  pi.Sponsor.Code,
		"sponsor_until": pi.SponsorTill,
		"daily":         pi.Daily,
		"stacked":       pi.Stacked,
		"requirements": map[string]interface{}{
			"c1": "1 个账号 · 积分 ≥ 500 · 7 天在线率 ≥ 90%",
			"c2": "2 个账号 或 单账号积分 ≥ 2000",
			"c3": "3 个账号 · 30 天在线率 ≥ 95%",
		},
	})
}

// 提交授权账号：支持粘贴上游账号 JSON（含 accessToken/refreshToken），后端加密入库并尝试接入池子
func handleSubmitAccount(w http.ResponseWriter, r *http.Request) {
	u := currentUser(r)
	var in struct {
		Realm string `json:"realm"`
		Label string `json:"label"`
		Raw   string `json:"raw"`
	}
	_ = json.NewDecoder(r.Body).Decode(&in)
	if in.Realm != "intl" && in.Realm != "cn" {
		writeErr(w, 400, "realm 只能是 intl 或 cn")
		return
	}
	raw := strings.TrimSpace(in.Raw)
	if raw == "" {
		writeErr(w, 400, "请粘贴账号凭据 JSON（浏览器授权页导出的内容）")
		return
	}
	var cred struct {
		UID         string `json:"uid"`
		Nickname    string `json:"nickname"`
		Domain      string `json:"domain"`
		Realm       string `json:"realm"`
		AccessToken string `json:"accessToken"`
	}
	if err := json.Unmarshal([]byte(raw), &cred); err != nil {
		writeErr(w, 400, "JSON 解析失败："+err.Error())
		return
	}
	if cred.AccessToken == "" {
		writeErr(w, 400, "缺少 accessToken 字段")
		return
	}
	if cred.UID == "" {
		cred.UID = "user-" + randHex(6)
	}
	label := in.Label
	if label == "" {
		label = defaultStr(cred.Nickname, "账号")
	}
	a := ContribAccount{
		UserID: u.ID, Username: u.Username, Realm: in.Realm, Label: label,
		UIDRef: cred.UID, State: "pending", Credential: encrypt(raw),
		Note: "等待管理员审核接入",
	}
	db.Create(&a)
	ok(w, map[string]interface{}{"ok": true, "id": a.ID, "state": a.State,
		"hint": "已提交，管理员审核通过后开始折算额度"})
}

func handleDeleteAccount(w http.ResponseWriter, r *http.Request) {
	u := currentUser(r)
	var in struct {
		ID uint `json:"id"`
	}
	_ = json.NewDecoder(r.Body).Decode(&in)
	db.Model(&ContribAccount{}).Where("id = ? AND user_id = ?", in.ID, u.ID).
		Updates(map[string]interface{}{"state": "revoked", "note": "用户已撤回"})
	ok(w, map[string]interface{}{"ok": true})
}

// ---------------- 改密码 / 学生认证 ----------------
func handleChangePassword(w http.ResponseWriter, r *http.Request) {
	u := currentUser(r)
	var in struct {
		Old string `json:"old"`
		New string `json:"new"`
	}
	_ = json.NewDecoder(r.Body).Decode(&in)
	if !checkPass(u.PassHash, in.Old) {
		writeErr(w, 400, "原密码不正确")
		return
	}
	if len(in.New) < 6 {
		writeErr(w, 400, "新密码至少 6 位")
		return
	}
	db.Model(&User{}).Where("id = ?", u.ID).Update("pass_hash", hashPass(in.New))
	ok(w, map[string]interface{}{"ok": true})
}

func handleStudentVerify(w http.ResponseWriter, r *http.Request) {
	u := currentUser(r)
	var in struct {
		Email string `json:"email"`
	}
	_ = json.NewDecoder(r.Body).Decode(&in)
	// 简化实现：教育邮箱后缀即通过（后续可接学信网 / 邮件验证码）
	edus := []string{".edu.cn", ".edu", ".ac.cn", "edu."}
	low := strings.ToLower(in.Email)
	passed := false
	for _, s := range edus {
		if strings.Contains(low, s) {
			passed = true
		}
	}
	if !passed {
		writeErr(w, 400, "需要教育邮箱（.edu.cn / .edu / .ac.cn）")
		return
	}
	db.Model(&User{}).Where("id = ?", u.ID).Updates(map[string]interface{}{"student": true, "email": in.Email})
	ok(w, map[string]interface{}{"ok": true, "student": true, "hint": "学生认证已通过：赞助 7 折、授权折算 +5%"})
}

// ---------------- 公开状态 ----------------
func handlePlansPublic(w http.ResponseWriter, r *http.Request) {
	if cors(w, r) {
		return
	}
	ok(w, plansPayload())
}

func plansPayload() map[string]interface{} {
	var rows []Plan
	db.Where("enabled = ?", true).Order("kind, sort").Find(&rows)
	var sponsor, contrib, points []map[string]interface{}
	for _, p := range rows {
		item := map[string]interface{}{
			"code": p.Code, "name": p.Name, "tagline": p.Tagline,
			"price": p.Price, "year_price": p.YearPrice, "daily": p.Daily,
			"conc": p.Conc, "rpm": p.RPM, "log_days": p.LogDays,
			"ratio": p.Ratio, "min_credits": p.MinCredits, "min_uptime": p.MinUptime,
			"accounts": p.Accounts, "hot": p.Hot,
			"perks": json.RawMessage(p.Perks), "need": json.RawMessage(defaultStr(p.Need, "[]")),
		}
		if p.Kind == "sponsor" {
			sponsor = append(sponsor, item)
		} else if p.Kind == "points" {
			item["points"] = p.Daily
			points = append(points, item)
		} else {
			contrib = append(contrib, item)
		}
	}
	return map[string]interface{}{
		"sponsor":          sponsor,
		"contrib":          contrib,
		"points":           points,
		"point_per_yuan":   cfg.PointPerYuan,
		"signup_points":    cfg.SignupPoints,
		"signup_days":      cfg.SignupDays,
		"guest_daily":      getSetting("guest_daily", "2000"),
		"student_discount": getSetting("student_discount", "0.7"),
		"stack_multiplier": getSetting("stack_multiplier", "1.5"),
	}
}

func handleStatusPublic(w http.ResponseWriter, r *http.Request) {
	if cors(w, r) {
		return
	}
	up := upstreamHealth()
	names := publicModelNames()
	ok(w, map[string]interface{}{
		"ok":           true,
		"upstream":     up,
		"realm_split":  realmUsableSnapshot(),
		"models":       names,
		"model_count":  len(names),
		"github_login": ghReady(),
		"time":         nowISO(),
	})
}

func sortStrings(s []string) {
	for i := 1; i < len(s); i++ {
		for j := i; j > 0 && s[j] < s[j-1]; j-- {
			s[j], s[j-1] = s[j-1], s[j]
		}
	}
}

// 上游健康：/health 给出账号数、剩余时间
func upstreamHealth() map[string]interface{} {
	resp, err := upstreamDo("GET", "/health", nil, false)
	if err != nil {
		return map[string]interface{}{"ok": false, "error": err.Error()}
	}
	defer resp.Body.Close()
	b, _ := io.ReadAll(io.LimitReader(resp.Body, 4096))
	var m map[string]interface{}
	_ = json.Unmarshal(b, &m)
	if m == nil {
		m = map[string]interface{}{}
	}
	m["ok"] = resp.StatusCode == 200
	m["checked_at"] = nowISO()
	return m
}
