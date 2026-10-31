package main

// admin.go — 管理端：用户 / 订单 / 卡密 / 账号审核 / 档位 / 池状态

import (
	"encoding/json"
	"net/http"
	"strconv"
	"strings"
	"time"
)

var _ = strconv.Itoa

func handleAdminStats(w http.ResponseWriter, r *http.Request) {
	var users, banned, keys, orders int64
	db.Model(&User{}).Count(&users)
	db.Model(&User{}).Where("banned = ?", true).Count(&banned)
	db.Model(&APIKey{}).Where("revoked = ?", false).Count(&keys)
	db.Model(&Order{}).Where("status = ?", "pending").Count(&orders)

	var calls int64
	var points float64
	dayStart := time.Now().Truncate(24 * time.Hour)
	db.Model(&LogRow{}).Where("created_at >= ?", dayStart).
		Select("count(*), coalesce(sum(points),0)").Row().Scan(&calls, &points)

	var paidAmount float64
	db.Model(&Order{}).Where("status = ?", "paid").Select("coalesce(sum(amount),0)").Scan(&paidAmount)

	var pendingAcc int64
	db.Model(&ContribAccount{}).Where("state = ?", "pending").Count(&pendingAcc)

	// 近 7 天注册
	type drow struct {
		Day string `json:"day"`
		N   int64  `json:"n"`
	}
	var regs []drow
	db.Model(&User{}).Where("created_at >= ?", time.Now().AddDate(0, 0, -7)).
		Select("strftime('%m-%d', created_at) as day, count(*) as n").Group("day").Order("day").Scan(&regs)

	ok(w, map[string]interface{}{
		"users": users, "banned": banned, "keys": keys,
		"pending_orders": orders, "pending_accounts": pendingAcc,
		"today_calls": calls, "today_points": round2(points),
		"revenue": round2(paidAmount), "registrations": regs,
		"pool": upstreamHealth(),
	})
}

func handleAdminUsers(w http.ResponseWriter, r *http.Request) {
	q := strings_Trim(r.URL.Query().Get("q"))
	page, _ := strconv.Atoi(r.URL.Query().Get("page"))
	if page < 1 {
		page = 1
	}
	dbq := db.Model(&User{})
	if q != "" {
		dbq = dbq.Where("username LIKE ? OR email LIKE ? OR invite_code = ?", "%"+q+"%", "%"+q+"%", q)
	}
	var total int64
	dbq.Count(&total)
	var rows []User
	dbq.Order("id desc").Offset((page - 1) * 30).Limit(30).Find(&rows)
	list := make([]map[string]interface{}, 0, len(rows))
	for i := range rows {
		u := &rows[i]
		var keys int64
		db.Model(&APIKey{}).Where("user_id = ? AND revoked = ?", u.ID, false).Count(&keys)
		list = append(list, map[string]interface{}{
			"id": u.ID, "username": u.Username, "email": u.Email, "plan": u.Plan,
			"plan_until": u.PlanUntil, "is_admin": u.IsAdmin, "banned": u.Banned,
			"ban_reason": u.BanReason, "points_today": round2(u.PointsToday),
			"bonus_points": round2(u.BonusPoints), "student": u.Student,
			"lottery_points": round2(u.LotteryPoints), "unlimited": u.Unlimited,
			"invite_code": u.InviteCode, "created_at": u.CreatedAt,
			"last_login": u.LastLogin, "keys": keys,
		})
	}
	ok(w, map[string]interface{}{"total": total, "page": page, "users": list})
}

func handleAdminSetPlan(w http.ResponseWriter, r *http.Request) {
	var in struct {
		ID     uint   `json:"id"`
		Plan   string `json:"plan"`
		Months int    `json:"months"`
	}
	_ = json.NewDecoder(r.Body).Decode(&in)
	var u User
	if err := db.First(&u, in.ID).Error; err != nil {
		writeErr(w, 404, "用户不存在")
		return
	}
	if in.Plan == "" {
		db.Model(&User{}).Where("id = ?", u.ID).Updates(map[string]interface{}{"plan": "", "plan_until": nil})
		ok(w, map[string]interface{}{"ok": true, "plan": ""})
		return
	}
	p := planOf(in.Plan)
	if p.Code == "" {
		writeErr(w, 400, "档位不存在")
		return
	}
	m := in.Months
	if m < 1 {
		m = 1
	}
	base := time.Now()
	if u.PlanUntil != nil && u.PlanUntil.After(base) {
		base = *u.PlanUntil
	}
	until := base.AddDate(0, m, 0)
	db.Model(&User{}).Where("id = ?", u.ID).Updates(map[string]interface{}{"plan": p.Code, "plan_until": until})
	ok(w, map[string]interface{}{"ok": true, "plan": p.Code, "until": until.Format("2006-01-02")})
}

func handleAdminBan(w http.ResponseWriter, r *http.Request) {
	var in struct {
		ID     uint   `json:"id"`
		Banned bool   `json:"banned"`
		Reason string `json:"reason"`
	}
	_ = json.NewDecoder(r.Body).Decode(&in)
	if in.Reason == "" {
		in.Reason = "违反使用规则"
	}
	db.Model(&User{}).Where("id = ?", in.ID).Updates(map[string]interface{}{"banned": in.Banned, "ban_reason": in.Reason})
	ok(w, map[string]interface{}{"ok": true})
}

func handleAdminPoints(w http.ResponseWriter, r *http.Request) {
	var in struct {
		ID     uint    `json:"id"`
		Points float64 `json:"points"` // 正数增加加油包，负数扣
		Note   string  `json:"note"`
	}
	_ = json.NewDecoder(r.Body).Decode(&in)
	var u User
	if err := db.First(&u, in.ID).Error; err != nil {
		writeErr(w, 404, "用户不存在")
		return
	}
	v := u.BonusPoints + in.Points
	if v < 0 {
		v = 0
	}
	update := map[string]interface{}{"bonus_points": v}
	if in.Points > 0 {
		until := time.Now().AddDate(0, 0, 90)
		update["bonus_until"] = until
	}
	db.Model(&User{}).Where("id = ?", u.ID).Updates(update)
	ok(w, map[string]interface{}{"ok": true, "bonus_points": round2(v)})
}

func handleAdminOrders(w http.ResponseWriter, r *http.Request) {
	st := r.URL.Query().Get("status")
	q := db.Model(&Order{})
	if st != "" {
		q = q.Where("status = ?", st)
	}
	var rows []Order
	q.Order("id desc").Limit(100).Find(&rows)
	ok(w, map[string]interface{}{"orders": rows})
}

func handleAdminOrderReview(w http.ResponseWriter, r *http.Request) {
	var in struct {
		ID     uint   `json:"id"`
		Action string `json:"action"` // pay / cancel
		Note   string `json:"note"`
	}
	_ = json.NewDecoder(r.Body).Decode(&in)
	var o Order
	if err := db.First(&o, in.ID).Error; err != nil {
		writeErr(w, 404, "订单不存在")
		return
	}
	if in.Action == "cancel" {
		db.Model(&Order{}).Where("id = ?", o.ID).Update("status", "cancelled")
		ok(w, map[string]interface{}{"ok": true, "status": "cancelled"})
		return
	}
	markOrderPaid(&o, in.Note)
	ok(w, map[string]interface{}{"ok": true, "status": "paid"})
}

func handleAdminCodes(w http.ResponseWriter, r *http.Request) {
	var rows []RedeemCode
	db.Order("id desc").Limit(200).Find(&rows)
	ok(w, map[string]interface{}{"codes": rows})
}

func handleAdminGenCode(w http.ResponseWriter, r *http.Request) {
	var in struct {
		Kind   string  `json:"kind"` // points / plan
		Points float64 `json:"points"`
		Plan   string  `json:"plan"`
		Months int     `json:"months"`
		Count  int     `json:"count"`
		Days   int     `json:"expire_days"`
		Note   string  `json:"note"`
	}
	_ = json.NewDecoder(r.Body).Decode(&in)
	if in.Count < 1 {
		in.Count = 1
	}
	if in.Count > 200 {
		in.Count = 200
	}
	if in.Kind == "" {
		in.Kind = "points"
	}
	made := make([]string, 0, in.Count)
	var expire *time.Time
	if in.Days > 0 {
		t := time.Now().AddDate(0, 0, in.Days)
		expire = &t
	}
	for i := 0; i < in.Count; i++ {
		code := "WB2-" + strings.ToUpper(randHex(4)) + "-" + strings.ToUpper(randHex(4))
		c := RedeemCode{
			Code: code, Kind: in.Kind, Points: in.Points,
			Plan: in.Plan, Months: in.Months, ExpireAt: expire, Note: in.Note,
		}
		if err := db.Create(&c).Error; err == nil {
			made = append(made, code)
		}
	}
	ok(w, map[string]interface{}{"ok": true, "codes": made})
}

func handleAdminAccounts(w http.ResponseWriter, r *http.Request) {
	st := r.URL.Query().Get("state")
	q := db.Model(&ContribAccount{})
	if st != "" {
		q = q.Where("state = ?", st)
	}
	var rows []ContribAccount
	q.Order("id desc").Limit(200).Find(&rows)
	list := make([]map[string]interface{}, 0, len(rows))
	for _, a := range rows {
		list = append(list, map[string]interface{}{
			"id": a.ID, "username": a.Username, "user_id": a.UserID, "realm": a.Realm,
			"label": a.Label, "uid_ref": a.UIDRef, "credits": a.Credits,
			"state": a.State, "note": a.Note, "created_at": a.CreatedAt,
		})
	}
	ok(w, map[string]interface{}{"accounts": list})
}

// 审核授权账号：通过 → 尝试写入上游 accounts 目录并热加载
func handleAdminAccountReview(w http.ResponseWriter, r *http.Request) {
	var in struct {
		ID     uint   `json:"id"`
		Action string `json:"action"` // approve / reject
		Note   string `json:"note"`
	}
	_ = json.NewDecoder(r.Body).Decode(&in)
	var a ContribAccount
	if err := db.First(&a, in.ID).Error; err != nil {
		writeErr(w, 404, "账号不存在")
		return
	}
	if in.Action == "reject" {
		db.Model(&ContribAccount{}).Where("id = ?", a.ID).Updates(map[string]interface{}{"state": "rejected", "note": defaultStr(in.Note, "审核未通过")})
		ok(w, map[string]interface{}{"ok": true, "state": "rejected"})
		return
	}
	cred, err := decrypt(a.Credential)
	if err != nil {
		writeErr(w, 500, "凭据解密失败")
		return
	}
	// 写入上游 accounts 目录（按 uid.json 命名），由用户手工确认后再改 state=ok
	path, werr := writeUpstreamAccount(cred, a.UIDRef)
	if werr != nil {
		db.Model(&ContribAccount{}).Where("id = ?", a.ID).Updates(map[string]interface{}{"state": "pending", "note": "写入上游失败：" + werr.Error()})
		writeErr(w, 500, "写入上游失败："+werr.Error())
		return
	}
	db.Model(&ContribAccount{}).Where("id = ?", a.ID).Updates(map[string]interface{}{"state": "ok", "note": defaultStr(in.Note, "已接入，文件："+path)})

	// 达标的用户自动升到对应贡献档
	autoUpgradeContrib(a.UserID)
	ok(w, map[string]interface{}{"ok": true, "state": "ok", "path": path})
}

func autoUpgradeContrib(uid uint) {
	var u User
	if err := db.First(&u, uid).Error; err != nil {
		return
	}
	// 统一走 recomputePlan：贡献档按有效账号数、赞助档按已支付订单，取较高者
	recomputePlan(&u)
}

func handleAdminPlans(w http.ResponseWriter, r *http.Request) {
	var rows []Plan
	db.Order("kind, sort").Find(&rows)
	ok(w, map[string]interface{}{"plans": rows})
}

func handleAdminPlanSave(w http.ResponseWriter, r *http.Request) {
	var in Plan
	if err := json.NewDecoder(r.Body).Decode(&in); err != nil {
		writeErr(w, 400, "参数错误")
		return
	}
	var exist Plan
	if err := db.First(&exist, "code = ?", in.Code).Error; err != nil {
		writeErr(w, 404, "档位不存在")
		return
	}
	updates := map[string]interface{}{
		"name": in.Name, "tagline": in.Tagline,
		"price": in.Price, "year_price": in.YearPrice, "daily": in.Daily,
		"conc": in.Conc, "rpm": in.RPM, "log_days": in.LogDays,
		"ratio": in.Ratio, "hot": in.Hot, "enabled": in.Enabled,
	}
	if in.Perks != "" {
		updates["perks"] = in.Perks
	}
	db.Model(&Plan{}).Where("code = ?", in.Code).Updates(updates)
	ok(w, map[string]interface{}{"ok": true})
}

func handleAdminPool(w http.ResponseWriter, r *http.Request) {
	ok(w, map[string]interface{}{
		"upstream": upstreamHealth(),
		"models":   getSetting("models_cached_at", ""),
		"usage":    upstreamUsageSummary(),
	})
}

func handleAdminSettings(w http.ResponseWriter, r *http.Request) {
	if r.Method == http.MethodPost {
		var in map[string]string
		_ = json.NewDecoder(r.Body).Decode(&in)
		for k, v := range in {
			setSetting(k, v)
		}
		ok(w, map[string]interface{}{"ok": true})
		return
	}
	keys := []string{"guest_daily", "student_discount", "stack_multiplier", "cors_origin", "announcement"}
	out := map[string]string{}
	for _, k := range keys {
		out[k] = getSetting(k, "")
	}
	ok(w, out)
}

func strings_Trim(s string) string {
	for len(s) > 0 && (s[0] == ' ' || s[0] == '\t') {
		s = s[1:]
	}
	for len(s) > 0 && (s[len(s)-1] == ' ' || s[len(s)-1] == '\t') {
		s = s[:len(s)-1]
	}
	return s
}

// ---------------- 订单确认（支付回调与后台共用） ----------------
func markOrderPaid(o *Order, note string) {
	now := time.Now()
	updates := map[string]interface{}{"status": "paid", "paid_at": now}
	if note != "" {
		updates["note"] = note
	}
	db.Model(&Order{}).Where("id = ?", o.ID).Updates(updates)

	var u User
	if err := db.First(&u, o.UserID).Error; err == nil {
		pl := planOf(o.Plan)
		if pl.Kind == "points" {
			// 加油包：加点数 + 30 天有效，不动档位
			u.BonusPoints += pl.Daily
			until := now.AddDate(0, 0, 30)
			if u.BonusUntil != nil && u.BonusUntil.After(until) {
				until = *u.BonusUntil
			}
			db.Model(&User{}).Where("id = ?", u.ID).Updates(map[string]interface{}{
				"bonus_points": u.BonusPoints, "bonus_until": until,
			})
		} else {
			base := now
			if u.PlanUntil != nil && u.PlanUntil.After(now) {
				base = *u.PlanUntil
			}
			until := base.AddDate(0, o.Months, 0)
			db.Model(&User{}).Where("id = ?", u.ID).Updates(map[string]interface{}{
				"plan": o.Plan, "plan_until": until,
			})
			// 关键：付费不能把更高的贡献档覆盖掉，重新按「贡献 vs 赞助」取较高者
			var uu User
			if err := db.First(&uu, u.ID).Error; err == nil {
				recomputePlan(&uu)
			}
		}
		// 邀请返利：给下单用户的邀请人返点
		payInviteRebate(o)
	}
}

// ---------------- 结算 ----------------
func handleAdminSettle(w http.ResponseWriter, r *http.Request) {
	if r.Method == http.MethodPost {
		var in struct {
			Month   string `json:"month"`
			DryRun  bool   `json:"dry_run"`
			Confirm bool   `json:"confirm"`
		}
		_ = json.NewDecoder(r.Body).Decode(&in)
		rep, err := runSettlement(in.Month, in.DryRun || !in.Confirm)
		if err != nil {
			writeErr(w, 400, err.Error())
			return
		}
		ok(w, map[string]interface{}{"report": rep, "dry_run": in.DryRun || !in.Confirm})
		return
	}
	// 历史结算记录
	var rows []Settlement
	db.Order("id desc").Limit(200).Find(&rows)
	var reports []string
	ok(w, map[string]interface{}{
		"records":          rows,
		"credit_to_points": envF("WB2_CREDIT_TO_POINTS", 1000),
		"last_month":       time.Now().AddDate(0, -1, 0).Format("2006-01"),
		"_":                reports,
	})
}

// ---------------- 支付流水 ----------------
func handleAdminPayLogs(w http.ResponseWriter, r *http.Request) {
	var rows []PayLog
	db.Order("id desc").Limit(200).Find(&rows)
	chans := loadChannels()
	list := make([]map[string]interface{}, 0, len(chans))
	for _, c := range chans {
		list = append(list, map[string]interface{}{
			"code": c.Code, "name": c.Name, "driver": c.Driver,
			"gateway": c.Gateway, "pid": c.PID, "enabled": c.Enabled,
		})
	}
	ok(w, map[string]interface{}{"logs": rows, "channels": list})
}

// ---------------- 账号健康 ----------------
func handleAdminHealth(w http.ResponseWriter, r *http.Request) {
	var accs []ContribAccount
	db.Where("state IN ?", []string{"ok", "cool"}).Find(&accs)
	list := make([]map[string]interface{}, 0, len(accs))
	for _, a := range accs {
		var calls30 int64
		var credit30 float64
		db.Model(&UpstreamUsage{}).
			Where("account = ? AND at >= ?", a.UIDRef, time.Now().AddDate(0, 0, -30)).
			Select("count(*)").Row().Scan(&calls30)
		db.Model(&UpstreamUsage{}).
			Where("account = ? AND at >= ?", a.UIDRef, time.Now().AddDate(0, 0, -30)).
			Select("coalesce(sum(credit),0)").Scan(&credit30)
		list = append(list, map[string]interface{}{
			"id": a.ID, "username": a.Username, "label": a.Label, "uid_ref": a.UIDRef,
			"realm": a.Realm, "state": a.State, "note": a.Note,
			"uptime_7d":  round2(uptimeOf(a.ID, time.Now().AddDate(0, 0, -7))),
			"uptime_30d": round2(uptimeOf(a.ID, time.Now().AddDate(0, 0, -30))),
			"calls_30d":  calls30, "credit_30d": round2(credit30),
		})
	}
	ok(w, map[string]interface{}{"accounts": list})
}
