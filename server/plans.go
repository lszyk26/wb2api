package main

// plans.go — 运营档位（后端唯一真源），前端通过 /api/plans 读取
//
// 改价格 / 额度只需要改这里（或在数据库里改 Plan 表），全站生效。

import (
	"encoding/json"
	"log"
	"strings"
	"time"
)

type planSeed struct {
	Plan
}

var seedRows = []Plan{
	// ---------------- 赞助档 ----------------
	{Code: "lv1", Kind: "sponsor", Name: "支持者", Tagline: "够用就好，按月随停",
		Price: 9, YearPrice: 90, Daily: 12000, Conc: 2, RPM: 30, LogDays: 7, Sort: 1, Enabled: true,
		Perks: jsonStr([]string{
			"每日 1.2 万算力点（约 3000 次轻量对话）",
			"aio · glm-5.3-flash 通道",
			"并发 2 · 30 请求/分钟",
			"调用日志保留 7 天",
			"邀请返利 8%",
		})},
	{Code: "lv2", Kind: "sponsor", Name: "常驻", Tagline: "把这里当主力，多数人停在这档",
		Price: 29, YearPrice: 290, Daily: 45000, Conc: 5, RPM: 80, LogDays: 30, Sort: 2, Hot: true, Enabled: true,
		Perks: jsonStr([]string{
			"每日 4.5 万算力点（约 1.1 万次轻量对话）",
			"全部文本模型 + 视觉 + 工具调用",
			"kimi-k3 / gpt-5.6 等长上下文",
			"并发 5 · 80 请求/分钟",
			"日志 30 天 + CSV 导出",
			"最多 5 把 API 密钥",
			"邀请返利 12%",
		})},
	{Code: "lv3", Kind: "sponsor", Name: "共建", Tagline: "给班级 / 社团 / 重度自用",
		Price: 79, YearPrice: 790, Daily: 150000, Conc: 12, RPM: 200, LogDays: 90, Sort: 3, Enabled: true,
		Perks: jsonStr([]string{
			"每日 15 万算力点（软上限，防脚本滥用）",
			"全部模型 + 新模型内测抢先",
			"并发 12 · 200 请求/分钟",
			"独立调度队列，不与他人抢",
			"日志 90 天 + CSV/JSON 导出",
			"可签发 5 个下游子密钥",
			"共建名单署名（可匿名）",
			"专属技术支持通道",
			"邀请返利 15%",
		})},

	// ---------------- 贡献档 ----------------
	{Code: "c1", Kind: "contrib", Name: "贡献者", Tagline: "交 1 个账号就够",
		Ratio: 0.30, MinCredits: 500, MinUptime: 90, Accounts: 1, Daily: 15000, Conc: 3, RPM: 40, LogDays: 7, Sort: 1, Enabled: true,
		Perks: jsonStr([]string{
			"账号余额 30% 每月折算成你的额度",
			"保底 = LV1 全部权益的 120%（每日 1.5 万点）",
			"并发 3 · 40 请求/分钟",
			"可随时撤回账号，额度按日结算",
		}),
		Need: jsonStr([]string{"1 个有效账号", "积分 ≥ 500", "7 天在线率 ≥ 90%"})},
	{Code: "c2", Kind: "contrib", Name: "主力贡献", Tagline: "两个账号，或一个大号",
		Ratio: 0.45, MinCredits: 2000, MinUptime: 92, Accounts: 2, Daily: 60000, Conc: 6, RPM: 100, LogDays: 30, Sort: 2, Hot: true, Enabled: true,
		Perks: jsonStr([]string{
			"账号余额 45% 每月折算成你的额度",
			"含 LV2 全部权益，额度再 ×1.2（每日 6 万点）",
			"并发 6 · 100 请求/分钟",
			"优先调度：冷却账号自动跳过",
		}),
		Need: jsonStr([]string{"2 个账号 或 单账号积分 ≥ 2000", "7 天在线率 ≥ 92%"})},
	{Code: "c3", Kind: "contrib", Name: "共建贡献", Tagline: "池子的主力，说话有分量",
		Ratio: 0.55, MinCredits: 3000, MinUptime: 95, Accounts: 3, Daily: 180000, Conc: 10, RPM: 180, LogDays: 90, Sort: 3, Enabled: true,
		Perks: jsonStr([]string{
			"账号余额 55% 每月折算成你的额度",
			"含 LV3 全部权益（每日 18 万点保底）",
			"独立队列 + 共建名单署名",
			"可提名新模型 / 优先修 bug",
		}),
		Need: jsonStr([]string{"3 个账号", "30 天在线率 ≥ 95%"})},
}

func jsonStr(v interface{}) string {
	b, _ := json.Marshal(v)
	return string(b)
}

func seedPlans() {
	for _, p := range seedRows {
		var exist Plan
		if err := db.First(&exist, "code = ?", p.Code).Error; err == nil {
			continue
		}
		row := p
		if err := db.Create(&row).Error; err != nil {
			log.Printf("[plans] seed %s: %v", p.Code, err)
		}
	}
}

// ---------------- 额度换算 ----------------
// 计费：点数 = (prompt*2 + completion*8) / 1000 * 模型系数
// 1 元 = 1000 点 → 一次轻量对话（300 in / 200 out）标准系数下 ≈ 4.4 点

func modelMultiplier(model string) float64 {
	switch model {
	case "glm-5.3-flash", "deepseek-v4.1-flash", "gemini-3.5-flash", "gemini-3.8-flash", "glm-5.2":
		return 1.0
	case "hy4-preview", "hy3", "glm-5.3", "kimi-k3", "kimi-k2.6", "kimi-k2.8-preview",
		"gpt-5.5", "gpt-5.6-sol", "gpt-5.6-terra", "gpt-5.6-luna", "gpt-5.4", "grok-4.7", "step-5-preview":
		return 1.8
	case "gpt-6-astra", "gpt-6.1-sol", "gpt-6-sol", "gpt-6-luna":
		return 4.0
	case "minimax-m3.1-flash":
		return 1.0
	case "deepseek-v4-pro", "kimi-k3-1", "minimax-m3", "hy3-x", "kimi-k2.7", "glm-5v-turbo",
		"minimax-m2.7", "space-bunny", "mimo-v2.6-pro":
		return 1.8
	}
	return 2.0
}

func pointsFor(model string, promptTok, complTok int) float64 {
	base := (float64(promptTok)*2 + float64(complTok)*8) / 1000
	return round2(base * modelMultiplier(model))
}

func round2(v float64) float64 {
	if v < 0.01 {
		return 0.01
	}
	return float64(int(v*100+0.5)) / 100
}

// ---------------- 贡献 / 赞助 双轨判定 ----------------
// users.plan 只有一个字段，付费曾把贡献档直接覆盖掉（2026-10-24 事故）。
// 现在：贡献档按「有效账号数」现算，赞助档按「已支付订单」现算，users.plan 只做展示。
func okContribCount(uid uint) int64 {
	var n int64
	db.Model(&ContribAccount{}).Where("user_id = ? AND state = ?", uid, "ok").Count(&n)
	return n
}

func contribPlanByCount(n int64) Plan {
	code := ""
	switch {
	case n >= 4:
		code = "c4"
	case n >= 3:
		code = "c3"
	case n >= 2:
		code = "c2"
	case n >= 1:
		code = "c1"
	default:
		return Plan{}
	}
	return planOf(code)
}

// sponsorPlanOf 从已支付订单推导当前赞助档（订单是唯一真源）
func sponsorPlanOf(uid uint) (Plan, *time.Time) {
	var o Order
	if err := db.Where("user_id = ? AND status = ? AND months > 0", uid, "paid").
		Order("paid_at desc").First(&o).Error; err != nil {
		return Plan{}, nil
	}
	if o.PaidAt == nil {
		return Plan{}, nil
	}
	until := o.PaidAt.AddDate(0, o.Months, 0)
	if time.Now().After(until) {
		return Plan{}, nil
	}
	p := planOf(o.Plan)
	if p.Kind != "sponsor" {
		return Plan{}, nil
	}
	return p, &until
}

// effectivePlans 返回 (贡献档, 赞助档, 赞助到期)
func effectivePlans(u *User) (Plan, Plan, *time.Time) {
	c := contribPlanByCount(okContribCount(u.ID))
	// 原规则：单个账号余额 ≥2000 也算主力贡献（升 c2）
	if c.Code == "c1" {
		var biggest float64
		db.Model(&ContribAccount{}).Where("user_id = ? AND state = ?", u.ID, "ok").
			Select("coalesce(max(credits),0)").Scan(&biggest)
		if biggest >= 2000 {
			c = planOf("c2")
		}
	}
	s, until := sponsorPlanOf(u.ID)
	return c, s, until
}

// 每日额度封顶（叠加后也不超过这个值）
const maxDailyQuota = 300000.0

// planInfo 档位合并结果
type planInfo struct {
	Contrib     Plan
	Sponsor     Plan
	SponsorTill *time.Time
	Daily       float64
	Stacked     bool // 是否触发了 1.5 倍叠加
}

// planInfoOf 合并「贡献档 + 赞助档」：
//   - 取两者中较高的日额度
//   - 贡献档 C1~C3 且同时有赞助 → 再 ×1.5（C4 已是顶格，不再叠加）
//   - 最终封顶 maxDailyQuota
func planInfoOf(u *User) planInfo {
	c, s, until := effectivePlans(u)
	pi := planInfo{Contrib: c, Sponsor: s, SponsorTill: until}
	base := c.Daily
	if s.Daily > base {
		base = s.Daily
	}
	if c.Daily > 0 && s.Daily > 0 && c.Code != "c4" {
		base *= 1.5
		pi.Stacked = true
	}
	if base > maxDailyQuota {
		base = maxDailyQuota
	}
	pi.Daily = base
	return pi
}

// 每日额度（游客 / 赞助 / 贡献 / 叠加）
func dailyQuota(u *User) float64 {
	// 开发者账号：额度视为无限（1 亿/日，实际用不到）
	if u.Unlimited {
		return 1e8
	}
	if u.DailyPoints > 0 {
		return u.DailyPoints
	}
	if d := planInfoOf(u).Daily; d > 0 {
		return d
	}
	// 兜底：admin 手工指定的 users.plan
	if u.Plan != "" {
		if p := planOf(u.Plan); p.Daily > 0 {
			return p.Daily
		}
	}
	// 新用户宽限期：注册后 N 天给足额度，过后跌回「乞丐档」
	if u.Plan == "" {
		days := 7.0
		var dv float64
		if _, err := fmtSscan(getSetting("newbie_days", "7"), &dv); err == nil && dv > 0 {
			days = dv
		}
		if time.Since(u.CreatedAt) < time.Duration(days*24*float64(time.Hour)) {
			var f float64
			if _, err := fmtSscan(getSetting("newbie_daily", "8000"), &f); err == nil && f > 0 {
				return f
			}
		}
	}
	// 游客（过了宽限期）
	if d := getSetting("guest_daily", "200"); d != "" {
		var f float64
		if _, err := fmtSscan(d, &f); err == nil {
			return f
		}
	}
	return 200
}

// recomputePlan 把「贡献 vs 赞助」较高者写回 users.plan（付费后 / 授权变化后调用）
func recomputePlan(u *User) {
	if u == nil {
		return
	}
	cPlan, sPlan, sUntil := effectivePlans(u)
	if cPlan.Code == "" && sPlan.Code == "" {
		// 没有任何依据（如已撤回账号的老用户）→ 保持现状，不误降档
		return
	}
	// 老用户手动档位更高时不降（撤回账号但档位保留的情况）
	if cPlan.Code == "" && sPlan.Code != "" {
		if cur := planOf(u.Plan); cur.Code != "" && cur.Daily > sPlan.Daily {
			return
		}
	}
	code, until := cPlan.Code, (*time.Time)(nil)
	if sPlan.Code != "" && (cPlan.Code == "" || sPlan.Daily > cPlan.Daily) {
		code, until = sPlan.Code, sUntil
	}
	upd := map[string]interface{}{"plan": code, "plan_until": until}
	if cPlan.Code != "" {
		ratio := 0.30
		switch cPlan.Code {
		case "c4":
			ratio = 0.60
		case "c3":
			ratio = 0.55
		case "c2":
			ratio = 0.45
		}
		upd["contrib_ratio"] = ratio
	}
	db.Model(&User{}).Where("id = ?", u.ID).Updates(upd)
	u.Plan = code
	u.PlanUntil = until
}

func planOf(code string) Plan {
	var p Plan
	// 档位码统一小写匹配（前端历史上会传 LV1 / Lv1 之类）
	db.First(&p, "LOWER(code) = ?", strings.ToLower(strings.TrimSpace(code)))
	return p
}

// 并发 / RPM 限制（贡献 / 赞助取较高档）
func limitsOf(u *User) (int, int, int) {
	// 开发者账号：并发 64 / RPM 1200 / 日志 365 天
	if u.Unlimited {
		return 64, 1200, 365
	}
	cPlan, sPlan, _ := effectivePlans(u)
	p := cPlan
	if sPlan.Conc > p.Conc {
		p = sPlan
	}
	if p.Code == "" && u.Plan != "" {
		p = planOf(u.Plan)
	}
	if p.Code != "" {
		return p.Conc, p.RPM, p.LogDays
	}
	return 1, 20, 3
}
