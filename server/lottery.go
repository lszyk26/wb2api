package main

// lottery.go — 抽奖 v2
//
// 设计要点（改动前请先读这段）：
//  1. 付费池扣 bonus_points（付费点数），奖品统一发 lottery_points（赠点）。
//     资金流单向：赠点永远不能回流到抽奖，天然防「中奖 → 再抽 → 再中」。
//  2. 抽奖入口第一件事必须调 rollDay(u)。
//     bonus_until 是懒清零（只在有请求经过 rollDay 时才清），
//     不先 roll 就会消费到「已过期但还没被清掉」的余额。
//  3. 库存按天分区（lottery_prize_days），天然每日重置，不需要定时任务。
//  4. 随机数用 crypto/rand，禁止 math/rand（种子可预测 = 可算中奖）。
//  5. 全流程在 db.Transaction 内，事务内一律用 tx，禁止用全局 db。

import (
	"crypto/rand"
	"encoding/json"
	"fmt"
	"log"
	"math"
	"math/big"
	"net/http"
	"strconv"
	"strings"
	"sync"
	"time"

	"gorm.io/gorm"
)

// 每个奖池启用奖品的权重合计必须等于这个数
const lotteryWeightTotal = 10000

const lotteryDefaultAnnounce = "抽奖所得赠点永久有效，只能用于 API 消费，不能再次抽奖；大奖每日限量，先到先得。"

// ---------------- 模型 ----------------

// LotteryPrize 奖品（奖池 = paid / free）
type LotteryPrize struct {
	ID         uint    `gorm:"primarykey" json:"id"`
	Pool       string  `gorm:"size:8;index" json:"pool"` // paid / free
	Name       string  `gorm:"size:32" json:"name"`
	Kind       string  `gorm:"size:16" json:"kind"` // lottery_points / bonus_points / none
	Value      float64 `json:"value"`
	Weight     int     `json:"weight"`
	DailyStock int     `json:"daily_stock"` // -1 = 不限量
	Enabled    bool    `json:"enabled"`
	Sort       int     `json:"sort"`
}

// LotteryPrizeDay 每日库存占用（按天分区，天然重置）
type LotteryPrizeDay struct {
	ID      uint   `gorm:"primarykey" json:"id"`
	PrizeID uint   `gorm:"uniqueIndex:idx_lpd_day" json:"prize_id"`
	Day     string `gorm:"size:10;uniqueIndex:idx_lpd_day" json:"day"`
	Claimed int    `json:"claimed"`
}

// LotteryDraw 抽奖流水（记 random_no / weight，便于审计复现）
type LotteryDraw struct {
	ID            uint      `gorm:"primarykey" json:"draw_id"`
	UserID        uint      `gorm:"index;uniqueIndex:idx_draw_idem" json:"user_id"`
	Username      string    `gorm:"size:64" json:"username"`
	Pool          string    `gorm:"size:8;index" json:"pool"` // paid / free / clawback
	PrizeID       uint      `json:"prize_id"`
	PrizeName     string    `gorm:"size:64" json:"prize_name"`
	PrizeType     string    `gorm:"size:16" json:"prize_type"`
	PrizeValue    float64   `json:"prize_value"`
	CostPoints    float64   `json:"cost_points"`
	RandomNo      int       `json:"random_no"`
	Weight        int       `json:"weight"`
	BonusBefore   float64   `json:"bonus_before"`
	BonusAfter    float64   `json:"bonus_after"`
	LotteryBefore float64   `json:"lottery_before"`
	LotteryAfter  float64   `json:"lottery_after"`
	IdemKey       string    `gorm:"size:64;uniqueIndex:idx_draw_idem" json:"-"`
	IP            string    `gorm:"size:64" json:"ip"`
	UA            string    `gorm:"size:200" json:"ua"`
	Revoked       bool      `json:"revoked"`
	Note          string    `gorm:"size:128" json:"note"`
	CreatedAt     time.Time `gorm:"index" json:"created_at"`
}

// ---------------- 小工具 ----------------

func todayKey() string { return time.Now().Format("2006-01-02") }

func startOfDay() time.Time {
	n := time.Now()
	return time.Date(n.Year(), n.Month(), n.Day(), 0, 0, 0, 0, n.Location())
}

func settingF(k, def string) float64 {
	var f float64
	if _, err := fmtSscan(getSetting(k, def), &f); err == nil {
		return f
	}
	var d float64
	if _, err := fmtSscan(def, &d); err == nil {
		return d
	}
	return 0
}

func settingI(k, def string) int { return int(settingF(k, def)) }

func settingB(k, def string) bool { return getSetting(k, def) == "1" }

// cryptoRandInt 返回 [0,n) 的密码学安全随机数
func cryptoRandInt(n int) (int, error) {
	if n <= 1 {
		return 0, nil
	}
	v, err := rand.Int(rand.Reader, big.NewInt(int64(n)))
	if err != nil {
		return 0, err
	}
	return int(v.Int64()), nil
}

// lotRound 抽奖专用取整。
// 注意：不要用 plans.go 里的 round2 —— 它对 <0.01 会返回 0.01（原本给金额用），
// 会把「0 点」和「0 赠点」显示成 0.01，属于必须避开的坑。
func lotRound(v float64) float64 {
	return math.Round(v*100) / 100
}

// probStr 权重（合计 10000）→ 百分比字符串
func probStr(w int) string {
	p := float64(w) / 100
	if p == float64(int(p)) {
		return strconv.Itoa(int(p)) + "%"
	}
	return strconv.FormatFloat(p, 'f', 2, 64) + "%"
}

// ---------------- 错误码 ----------------

type lotErr struct {
	Status int
	Code   string
	Msg    string
}

func (e *lotErr) Error() string { return e.Msg }

func lotErrOf(status int, code, msg string) error { return &lotErr{status, code, msg} }

func writeLotErr(w http.ResponseWriter, err error) {
	if le, isLot := err.(*lotErr); isLot {
		writeJSON(w, le.Status, map[string]interface{}{
			"error": map[string]interface{}{"message": le.Msg, "code": le.Code},
		})
		return
	}
	log.Printf("[lottery] internal: %v", err)
	writeJSON(w, 500, map[string]interface{}{
		"error": map[string]interface{}{"message": "抽奖失败，请稍后重试", "code": "LOTTERY_INTERNAL"},
	})
}

// ---------------- 限频（独立于 /v1 的桶） ----------------

type lotLimiter struct {
	mu   sync.Mutex
	hits map[string][]int64
}

func newLotLimiter() *lotLimiter { return &lotLimiter{hits: map[string][]int64{}} }

func (l *lotLimiter) allow(key string, perMin int) bool {
	if perMin <= 0 {
		return true
	}
	now := time.Now().Unix()
	l.mu.Lock()
	defer l.mu.Unlock()
	keep := l.hits[key][:0]
	for _, t := range l.hits[key] {
		if now-t < 60 {
			keep = append(keep, t)
		}
	}
	if len(keep) >= perMin {
		l.hits[key] = keep
		return false
	}
	l.hits[key] = append(keep, now)
	// 防内存膨胀
	if len(l.hits) > 5000 {
		for k, v := range l.hits {
			if len(v) == 0 || now-v[len(v)-1] >= 60 {
				delete(l.hits, k)
			}
		}
	}
	return true
}

var (
	lotUserLimiter = newLotLimiter() // 每用户 10 次/分钟
	lotIPLimiter   = newLotLimiter() // 每 IP 60 次/分钟
)

// ---------------- 奖池读取 ----------------

func loadPoolPrizes(tx *gorm.DB, pool string) ([]LotteryPrize, int) {
	var ps []LotteryPrize
	tx.Where("pool = ? AND enabled = ?", pool, true).Order("sort asc, id asc").Find(&ps)
	sum := 0
	for _, p := range ps {
		sum += p.Weight
	}
	return ps, sum
}

func prizeLabel(p LotteryPrize) string {
	if strings.TrimSpace(p.Name) != "" {
		return p.Name
	}
	if p.Kind == "none" || p.Value <= 0 {
		return "0点"
	}
	return strconv.FormatFloat(p.Value, 'f', -1, 64) + "点"
}

func claimsOfDay(prizes []LotteryPrize) map[uint]int {
	claims := map[uint]int{}
	if len(prizes) == 0 {
		return claims
	}
	ids := make([]uint, 0, len(prizes))
	for _, p := range prizes {
		ids = append(ids, p.ID)
	}
	var rows []LotteryPrizeDay
	db.Where("day = ? AND prize_id IN ?", todayKey(), ids).Find(&rows)
	for _, r := range rows {
		claims[r.PrizeID] = r.Claimed
	}
	return claims
}

func freeUsedToday(uid uint) int64 {
	var n int64
	db.Model(&LotteryDraw{}).Where("user_id = ? AND pool = ? AND created_at >= ?", uid, "free", startOfDay()).Count(&n)
	return n
}

func paidDrawsToday(uid uint) int64 {
	var n int64
	db.Model(&LotteryDraw{}).Where("user_id = ? AND pool = ? AND created_at >= ?", uid, "paid", startOfDay()).Count(&n)
	return n
}

func freeRemainingToday(uid uint) int {
	limit := settingI("lottery_daily_free", "1")
	left := limit - int(freeUsedToday(uid))
	if left < 0 {
		return 0
	}
	return left
}

// 启动/保存时的权重自检
func lotteryWeightOK(pool string) (int, bool) {
	var ps []LotteryPrize
	db.Where("pool = ? AND enabled = ?", pool, true).Find(&ps)
	sum := 0
	for _, p := range ps {
		sum += p.Weight
	}
	return sum, sum == lotteryWeightTotal && len(ps) > 0
}

func logLotteryWeightState() {
	for _, pool := range []string{"paid", "free"} {
		sum, okp := lotteryWeightOK(pool)
		if !okp {
			log.Printf("[lottery] ⚠️ 奖池 %s 权重合计 = %d，必须等于 %d，抽奖将被禁用", pool, sum, lotteryWeightTotal)
		} else {
			log.Printf("[lottery] 奖池 %s 权重合计 = %d ✓", pool, sum)
		}
	}
}

// ---------------- GET /api/lottery/config ----------------

func poolPublic(pool string, showProb bool, claims map[uint]int, sumOut *int) []map[string]interface{} {
	var ps []LotteryPrize
	db.Where("pool = ? AND enabled = ?", pool, true).Order("sort asc, id asc").Find(&ps)
	sum := 0
	out := make([]map[string]interface{}, 0, len(ps))
	for _, p := range ps {
		sum += p.Weight
		remain := -1
		if p.DailyStock >= 0 {
			remain = p.DailyStock - claims[p.ID]
			if remain < 0 {
				remain = 0
			}
		}
		item := map[string]interface{}{
			"id":              p.ID,
			"pool":            pool,
			"name":            prizeLabel(p),
			"kind":            p.Kind,
			"points":          lotRound(p.Value),
			"weight":          p.Weight,
			"daily_stock":     p.DailyStock,
			"remaining_stock": remain,
		}
		if showProb {
			item["probability"] = probStr(p.Weight)
		}
		out = append(out, item)
	}
	if sumOut != nil {
		*sumOut = sum
	}
	return out
}

func handleLotteryConfig(w http.ResponseWriter, r *http.Request) {
	u := currentUser(r)

	showProb := settingB("lottery_show_probability", "1")
	// 库存展示用两个池的全部奖品 id 一次取
	var all []LotteryPrize
	db.Where("enabled = ?", true).Find(&all)
	claims := claimsOfDay(all)

	var paidSum, freeSum int
	paid := poolPublic("paid", showProb, claims, &paidSum)
	free := poolPublic("free", showProb, claims, &freeSum)

	reason := ""
	if !settingB("lottery_enabled", "1") {
		reason = "off"
	} else if paidSum != lotteryWeightTotal || freeSum != lotteryWeightTotal {
		reason = "weight_invalid"
	}

	out := map[string]interface{}{
		"enabled":          reason == "",
		"disabled_reason":  reason,
		"cost_points":      lotRound(settingF("lottery_cost_points", "2000")),
		"daily_free_limit": settingI("lottery_daily_free", "1"),
		"point_per_yuan":   cfg.PointPerYuan,
		"show_probability": showProb,
		"announcement":     getSetting("lottery_announcement", lotteryDefaultAnnounce),
		"prizes":           paid,
		"free_prizes":      free,
		// 未登录时下列字段为 0
		"free_remaining_today": 0,
		"paid_draws_today":     0,
		"bonus_points":         0,
		"lottery_points":       0,
		"bonus_until":          nil,
		"bonus_expired":        false,
		"temp_email":           false,
		"newbie_locked":        false,
		"unlimited":            false,
	}
	if u != nil {
		bonusExpired := u.BonusUntil != nil && time.Now().After(*u.BonusUntil)
		newbieHours := settingI("lottery_newbie_paid_hours", "24")
		out["free_remaining_today"] = freeRemainingToday(u.ID)
		out["paid_draws_today"] = paidDrawsToday(u.ID)
		out["bonus_points"] = lotRound(u.BonusPoints)
		out["lottery_points"] = lotRound(u.LotteryPoints)
		out["bonus_until"] = u.BonusUntil
		out["bonus_expired"] = bonusExpired
		out["temp_email"] = u.EmailIsTemp
		out["newbie_locked"] = newbieHours > 0 && time.Since(u.CreatedAt) < time.Duration(newbieHours)*time.Hour
		out["unlimited"] = u.Unlimited
		if u.Unlimited {
			out["free_remaining_today"] = 999999
		}
	}
	ok(w, out)
}

// ---------------- POST /api/lottery/draw ----------------

func handleLotteryDraw(w http.ResponseWriter, r *http.Request) {
	if !methodGuard(w, r, http.MethodPost) {
		return
	}
	u := currentUser(r)
	if u == nil {
		writeErr(w, 401, "请先登录")
		return
	}
	if !settingB("lottery_enabled", "1") {
		writeLotErr(w, lotErrOf(403, "LOTTERY_DISABLED", "抽奖活动未开启"))
		return
	}
	if u.EmailIsTemp {
		writeLotErr(w, lotErrOf(403, "LOTTERY_TEMP_EMAIL_FORBIDDEN", "临时邮箱账号不能参与抽奖"))
		return
	}
	var in struct {
		Pool    string `json:"pool"`
		IdemKey string `json:"idempotency_key"`
	}
	_ = json.NewDecoder(r.Body).Decode(&in)
	pool := strings.ToLower(strings.TrimSpace(in.Pool))
	if pool != "paid" && pool != "free" {
		writeLotErr(w, lotErrOf(400, "LOTTERY_POOL_INVALID", "只有 free / paid 两个奖池"))
		return
	}
	key := strings.TrimSpace(in.IdemKey)
	if key == "" {
		writeLotErr(w, lotErrOf(400, "LOTTERY_IDEMPOTENCY_CONFLICT", "缺少幂等键 idempotency_key"))
		return
	}
	if len(key) > 64 {
		key = key[:64]
	}
	ip := clientIP(r)
	dev := u.Unlimited
	if !dev && !lotUserLimiter.allow(fmt.Sprintf("u%d", u.ID), 10) {
		writeLotErr(w, lotErrOf(429, "LOTTERY_RATE_LIMITED", "操作太快了，每分钟最多抽 10 次"))
		return
	}
	if !dev && !lotIPLimiter.allow(ip, 60) {
		writeLotErr(w, lotErrOf(429, "LOTTERY_RATE_LIMITED", "当前网络请求过于频繁，请稍后再试"))
		return
	}

	// 关键：先滚日期 + 清过期加油包。bonus_until 是懒清零，
	// 不先 roll 就会消费到「已过期但还没被清掉」的余额。
	rollDay(u)

	cost := settingF("lottery_cost_points", "2000")
	paySrc := getSetting("lottery_pay_source", "bonus_points")
	if paySrc != "bonus_points" {
		writeLotErr(w, lotErrOf(403, "LOTTERY_DISABLED", "抽奖支付源配置无效（目前只支持 bonus_points）"))
		return
	}
	freeLimit := settingI("lottery_daily_free", "1")
	budget := settingF("lottery_daily_budget_points", "0")
	newbieHours := settingI("lottery_newbie_paid_hours", "24")
	ua := r.UserAgent()
	if len(ua) > 200 {
		ua = ua[:200]
	}

	var out map[string]interface{}
	err := db.Transaction(func(tx *gorm.DB) error {
		var uu User
		if e := tx.First(&uu, u.ID).Error; e != nil {
			return e
		}

		// 幂等：同 (user_id, idem_key) 只扣一次
		var old LotteryDraw
		if e := tx.Where("user_id = ? AND idem_key = ?", u.ID, key).First(&old).Error; e == nil {
			if old.Pool != pool {
				return lotErrOf(409, "LOTTERY_IDEMPOTENCY_CONFLICT", "同一幂等键不能用于不同奖池")
			}
			out = drawResult(&old)
			return nil
		}

		prizes, sum := loadPoolPrizes(tx, pool)
		if len(prizes) == 0 || sum != lotteryWeightTotal {
			log.Printf("[lottery] 奖池 %s 权重异常 sum=%d", pool, sum)
			return lotErrOf(400, "LOTTERY_POOL_WEIGHT_INVALID", "奖池权重配置异常（合计必须 10000），已暂停抽奖")
		}

		day := todayKey()
		dayStart := startOfDay()

		if pool == "free" {
			// 开发者账号：免费次数不设上限
			if !uu.Unlimited {
				var used int64
				tx.Model(&LotteryDraw{}).Where("user_id = ? AND pool = ? AND created_at >= ?", u.ID, "free", dayStart).Count(&used)
				if int(used) >= freeLimit {
					return lotErrOf(403, "LOTTERY_FREE_LIMIT_REACHED", "今天的免费次数已经用完了")
				}
			}
		} else if !uu.Unlimited {
			if newbieHours > 0 && time.Since(uu.CreatedAt) < time.Duration(newbieHours)*time.Hour {
				return lotErrOf(403, "LOTTERY_NEWBIE_LOCKED", fmt.Sprintf("注册满 %d 小时后才能抽付费池", newbieHours))
			}
			if uu.BonusPoints < cost {
				return lotErrOf(400, "LOTTERY_INSUFFICIENT_BONUS", fmt.Sprintf("付费点数不足，抽一次需要 %.0f 点", cost))
			}
		}

		ids := make([]uint, 0, len(prizes))
		for _, p := range prizes {
			ids = append(ids, p.ID)
		}
		claims := map[uint]int{}
		var dayRows []LotteryPrizeDay
		tx.Where("day = ? AND prize_id IN ?", day, ids).Find(&dayRows)
		for _, d := range dayRows {
			claims[d.PrizeID] = d.Claimed
		}

		// 每日返奖预算：超额后大奖（>1000 点）权重并入 0 点
		budgetHit := false
		if pool == "paid" && budget > 0 {
			var back float64
			tx.Model(&LotteryDraw{}).Where("pool = ? AND created_at >= ? AND prize_value > 0 AND note <> ?", "paid", dayStart, "dev").
				Select("coalesce(sum(prize_value),0)").Scan(&back)
			if back >= budget {
				budgetHit = true
			}
		}

		// 组装候选：售罄 / 超预算的权重并入 none 档，权重总额恒为 10000
		type cand struct {
			P LotteryPrize
			W int
		}
		cands := make([]cand, 0, len(prizes))
		fold := 0
		noneIdx := -1
		for _, p := range prizes {
			exhausted := p.DailyStock >= 0 && claims[p.ID] >= p.DailyStock
			if budgetHit && p.Value > 1000 {
				exhausted = true
			}
			if exhausted {
				if p.Kind != "none" {
					fold += p.Weight
				}
				continue
			}
			if p.Kind == "none" {
				noneIdx = len(cands)
			}
			cands = append(cands, cand{p, p.Weight})
		}
		if noneIdx >= 0 {
			cands[noneIdx].W += fold
		} else if fold > 0 {
			cands = append(cands, cand{LotteryPrize{Kind: "none", Name: "0点", Weight: fold}, fold})
		}

		n, e := cryptoRandInt(lotteryWeightTotal)
		if e != nil {
			return e
		}
		acc := 0
		chosen := cands[len(cands)-1]
		for _, c := range cands {
			acc += c.W
			if n < acc {
				chosen = c
				break
			}
		}

		// 库存原子占用（条件写：抢空则降级为 0 点）
		if chosen.P.DailyStock >= 0 && chosen.P.ID != 0 {
			res := tx.Exec(`INSERT INTO lottery_prize_days (prize_id, day, claimed) VALUES (?,?,1)
				ON CONFLICT(prize_id, day) DO UPDATE SET claimed = claimed + 1 WHERE claimed < ?`,
				chosen.P.ID, day, chosen.P.DailyStock)
			if res.Error != nil {
				return res.Error
			}
			if res.RowsAffected == 0 {
				for _, c := range cands {
					if c.P.Kind == "none" {
						chosen = c
						break
					}
				}
			}
		}

		// 扣款：条件更新，防并发扣成负数（开发者账号不扣点）
		if pool == "paid" && cost > 0 && !uu.Unlimited {
			res := tx.Model(&User{}).Where("id = ? AND bonus_points >= ?", u.ID, cost).
				UpdateColumn("bonus_points", gorm.Expr("bonus_points - ?", cost))
			if res.Error != nil {
				return res.Error
			}
			if res.RowsAffected == 0 {
				return lotErrOf(400, "LOTTERY_INSUFFICIENT_BONUS", "付费点数不足")
			}
		}

		// 发奖：一律进 lottery_points（bonus_points 奖品仅小额兜底）
		if chosen.P.Value > 0 {
			switch chosen.P.Kind {
			case "lottery_points":
				if e := tx.Model(&User{}).Where("id = ?", u.ID).
					UpdateColumn("lottery_points", gorm.Expr("COALESCE(lottery_points,0) + ?", chosen.P.Value)).Error; e != nil {
					return e
				}
			case "bonus_points":
				if e := tx.Model(&User{}).Where("id = ?", u.ID).
					UpdateColumn("bonus_points", gorm.Expr("COALESCE(bonus_points,0) + ?", chosen.P.Value)).Error; e != nil {
					return e
				}
				until := time.Now().AddDate(0, 0, 30)
				tx.Model(&User{}).Where("id = ? AND (bonus_until IS NULL OR bonus_until < ?)", u.ID, until).
					UpdateColumn("bonus_until", until)
			}
		}

		row := LotteryDraw{
			UserID: u.ID, Username: uu.Username, Pool: pool,
			PrizeID: chosen.P.ID, PrizeName: prizeLabel(chosen.P),
			PrizeType: chosen.P.Kind, PrizeValue: chosen.P.Value,
			RandomNo: n, Weight: chosen.W,
			BonusBefore: uu.BonusPoints, LotteryBefore: uu.LotteryPoints,
			IdemKey: key, IP: ip, UA: ua, CreatedAt: time.Now(),
		}
		if pool == "paid" && !uu.Unlimited {
			row.CostPoints = cost
		}
		if uu.Unlimited {
			row.Note = "dev"
		}
		if e := tx.Create(&row).Error; e != nil {
			// 幂等键撞车（并发同 key）：取回已有记录，本次事务内的扣减随回滚一起撤销
			var dup LotteryDraw
			if tx.Where("user_id = ? AND idem_key = ?", u.ID, key).First(&dup).Error == nil {
				out = drawResult(&dup)
				return lotErrOf(409, "LOTTERY_IDEMPOTENCY_CONFLICT", "重复请求，请刷新后查看结果")
			}
			return e
		}
		var after User
		if e := tx.First(&after, u.ID).Error; e != nil {
			return e
		}
		row.BonusAfter = after.BonusPoints
		row.LotteryAfter = after.LotteryPoints
		tx.Model(&LotteryDraw{}).Where("id = ?", row.ID).Updates(map[string]interface{}{
			"bonus_after": row.BonusAfter, "lottery_after": row.LotteryAfter,
		})
		out = drawResult(&row)
		return nil
	})
	if err != nil {
		writeLotErr(w, err)
		return
	}
	if out != nil {
		if u.Unlimited {
			out["free_remaining_today"] = 999999 // 开发者账号：不限次
		} else {
			out["free_remaining_today"] = freeRemainingToday(u.ID)
		}
		out["paid_draws_today"] = paidDrawsToday(u.ID)
	}
	ok(w, out)
}

func drawResult(row *LotteryDraw) map[string]interface{} {
	return map[string]interface{}{
		"draw_id":              row.ID,
		"pool":                 row.Pool,
		"prize_name":           row.PrizeName,
		"prize_type":           row.PrizeType,
		"prize_value":          lotRound(row.PrizeValue),
		"cost_points":          lotRound(row.CostPoints),
		"bonus_points_before":  lotRound(row.BonusBefore),
		"bonus_points_after":   lotRound(row.BonusAfter),
		"lottery_points_after": lotRound(row.LotteryAfter),
	}
}

// ---------------- GET /api/lottery/history ----------------

func handleLotteryHistory(w http.ResponseWriter, r *http.Request) {
	u := currentUser(r)
	if u == nil {
		writeErr(w, 401, "请先登录")
		return
	}
	page, _ := strconv.Atoi(r.URL.Query().Get("page"))
	if page < 1 {
		page = 1
	}
	size, _ := strconv.Atoi(r.URL.Query().Get("page_size"))
	if size < 1 || size > 100 {
		size = 20
	}
	var total int64
	db.Model(&LotteryDraw{}).Where("user_id = ?", u.ID).Count(&total)
	var rows []LotteryDraw
	db.Where("user_id = ?", u.ID).Order("id desc").Offset((page - 1) * size).Limit(size).Find(&rows)
	items := make([]map[string]interface{}, 0, len(rows))
	for _, x := range rows {
		items = append(items, map[string]interface{}{
			"draw_id":     x.ID,
			"pool":        x.Pool,
			"prize_name":  x.PrizeName,
			"prize_value": lotRound(x.PrizeValue),
			"cost_points": lotRound(x.CostPoints),
			"created_at":  x.CreatedAt,
		})
	}
	ok(w, map[string]interface{}{"draws": items, "total": total, "page": page, "page_size": size})
}

// ---------------- 管理端 ----------------

func handleAdminLotteryPrizes(w http.ResponseWriter, r *http.Request) {
	pool := r.URL.Query().Get("pool")
	q := db.Order("pool asc, sort asc, id asc")
	if pool != "" {
		q = q.Where("pool = ?", pool)
	}
	var rows []LotteryPrize
	q.Find(&rows)
	claims := claimsOfDay(rows)
	items := make([]map[string]interface{}, 0, len(rows))
	for _, p := range rows {
		remain := -1
		if p.DailyStock >= 0 {
			remain = p.DailyStock - claims[p.ID]
			if remain < 0 {
				remain = 0
			}
		}
		items = append(items, map[string]interface{}{
			"id": p.ID, "pool": p.Pool, "name": p.Name, "kind": p.Kind,
			"value": p.Value, "weight": p.Weight, "daily_stock": p.DailyStock,
			"remaining_stock": remain, "enabled": p.Enabled, "sort": p.Sort,
			"probability": probStr(p.Weight),
		})
	}
	paidSum, _ := lotteryWeightOK("paid")
	freeSum, _ := lotteryWeightOK("free")
	ok(w, map[string]interface{}{
		"prizes": items, "paid_weight_sum": paidSum, "free_weight_sum": freeSum,
		"weight_total": lotteryWeightTotal,
	})
}

func handleAdminLotteryPrizeSave(w http.ResponseWriter, r *http.Request) {
	if !methodGuard(w, r, http.MethodPost) {
		return
	}
	var in struct {
		ID         uint    `json:"id"`
		Pool       string  `json:"pool"`
		Name       string  `json:"name"`
		Kind       string  `json:"kind"`
		Value      float64 `json:"value"`
		Weight     int     `json:"weight"`
		DailyStock int     `json:"daily_stock"`
		Enabled    *bool   `json:"enabled"`
		Sort       int     `json:"sort"`
	}
	_ = json.NewDecoder(r.Body).Decode(&in)
	pool := strings.ToLower(strings.TrimSpace(in.Pool))
	if pool != "paid" && pool != "free" {
		writeErr(w, 400, "pool 只能是 paid / free")
		return
	}
	switch in.Kind {
	case "lottery_points", "bonus_points", "none":
	default:
		writeErr(w, 400, "kind 只能是 lottery_points / bonus_points / none")
		return
	}
	if in.Weight < 0 || in.Weight > lotteryWeightTotal {
		writeErr(w, 400, "权重必须在 0–10000 之间")
		return
	}
	var saved LotteryPrize
	var sum int
	err := db.Transaction(func(tx *gorm.DB) error {
		p := LotteryPrize{}
		if in.ID != 0 {
			if e := tx.First(&p, in.ID).Error; e != nil {
				return lotErrOf(404, "NOT_FOUND", "奖品不存在")
			}
		} else {
			p.Enabled = true
		}
		p.Pool = pool
		p.Kind = in.Kind
		p.Name = strings.TrimSpace(in.Name)
		if p.Name == "" {
			p.Name = "0点"
		}
		p.Value = in.Value
		if in.Kind == "none" {
			p.Value = 0
		}
		p.Weight = in.Weight
		p.DailyStock = in.DailyStock
		p.Sort = in.Sort
		if in.Enabled != nil {
			p.Enabled = *in.Enabled
		}
		if e := tx.Save(&p).Error; e != nil {
			return e
		}
		// 同一个事务里立刻校验权重，不合规整笔回滚（不会留下脏数据）
		tx.Model(&LotteryPrize{}).Where("pool = ? AND enabled = ?", pool, true).
			Select("coalesce(sum(weight),0)").Scan(&sum)
		if sum != lotteryWeightTotal {
			return lotErrOf(400, "LOTTERY_POOL_WEIGHT_INVALID",
				fmt.Sprintf("保存后奖池 %s 的权重合计 = %d，必须等于 %d，已回滚", pool, sum, lotteryWeightTotal))
		}
		saved = p
		return nil
	})
	if err != nil {
		writeLotErr(w, err)
		return
	}
	ok(w, map[string]interface{}{"prize": saved, "weight_sum": sum, "weight_total": lotteryWeightTotal})
}

func handleAdminLotteryPrizeToggle(w http.ResponseWriter, r *http.Request) {
	if !methodGuard(w, r, http.MethodPost) {
		return
	}
	var in struct {
		ID      uint `json:"id"`
		Enabled bool `json:"enabled"`
	}
	_ = json.NewDecoder(r.Body).Decode(&in)
	var sum int
	var poolCode string
	err := db.Transaction(func(tx *gorm.DB) error {
		var p LotteryPrize
		if e := tx.First(&p, in.ID).Error; e != nil {
			return lotErrOf(404, "NOT_FOUND", "奖品不存在")
		}
		if e := tx.Model(&LotteryPrize{}).Where("id = ?", p.ID).Update("enabled", in.Enabled).Error; e != nil {
			return e
		}
		tx.Model(&LotteryPrize{}).Where("pool = ? AND enabled = ?", p.Pool, true).
			Select("coalesce(sum(weight),0)").Scan(&sum)
		poolCode = p.Pool
		if sum != lotteryWeightTotal {
			return lotErrOf(400, "LOTTERY_POOL_WEIGHT_INVALID",
				fmt.Sprintf("启停后奖池 %s 权重合计 = %d ≠ %d，已撤销", p.Pool, sum, lotteryWeightTotal))
		}
		return nil
	})
	if err != nil {
		writeLotErr(w, err)
		return
	}
	ok(w, map[string]interface{}{"ok": true, "pool": poolCode, "weight_sum": sum})
}

func handleAdminLotteryStockReset(w http.ResponseWriter, r *http.Request) {
	if !methodGuard(w, r, http.MethodPost) {
		return
	}
	var in struct {
		Pool string `json:"pool"`
	}
	_ = json.NewDecoder(r.Body).Decode(&in)
	day := todayKey()
	q := db.Where("day = ?", day)
	if in.Pool == "paid" || in.Pool == "free" {
		var ids []uint
		db.Model(&LotteryPrize{}).Where("pool = ?", in.Pool).Pluck("id", &ids)
		if len(ids) == 0 {
			ok(w, map[string]interface{}{"ok": true, "cleared": 0})
			return
		}
		q = q.Where("prize_id IN ?", ids)
	}
	res := q.Delete(&LotteryPrizeDay{})
	ok(w, map[string]interface{}{"ok": true, "cleared": res.RowsAffected, "day": day})
}

func handleAdminLotteryDraws(w http.ResponseWriter, r *http.Request) {
	qp := r.URL.Query()
	page, _ := strconv.Atoi(qp.Get("page"))
	if page < 1 {
		page = 1
	}
	size, _ := strconv.Atoi(qp.Get("page_size"))
	if size < 1 || size > 200 {
		size = 50
	}
	q := db.Model(&LotteryDraw{})
	if v := qp.Get("user_id"); v != "" {
		if n, err := strconv.Atoi(v); err == nil {
			q = q.Where("user_id = ?", n)
		}
	}
	if v := qp.Get("pool"); v != "" {
		q = q.Where("pool = ?", v)
	}
	if v := qp.Get("from"); v != "" {
		if t, err := time.Parse("2006-01-02", v); err == nil {
			q = q.Where("created_at >= ?", t)
		}
	}
	if v := qp.Get("to"); v != "" {
		if t, err := time.Parse("2006-01-02", v); err == nil {
			q = q.Where("created_at < ?", t.AddDate(0, 0, 1))
		}
	}
	var total int64
	q.Count(&total)
	var rows []LotteryDraw
	q.Order("id desc").Offset((page - 1) * size).Limit(size).Find(&rows)
	ok(w, map[string]interface{}{"draws": rows, "total": total, "page": page, "page_size": size})
}

func handleAdminLotteryStats(w http.ResponseWriter, r *http.Request) {
	qp := r.URL.Query()
	from := qp.Get("from")
	days := 14
	if from != "" {
		if t, err := time.Parse("2006-01-02", from); err == nil {
			days = int(time.Since(t).Hours()/24) + 1
		}
	}
	if days < 1 {
		days = 1
	}
	if days > 180 {
		days = 180
	}
	type row struct {
		Day        string  `json:"day"`
		Pool       string  `json:"pool"`
		Draws      int64   `json:"draws"`
		CostPoints float64 `json:"cost_points"`
		PrizeValue float64 `json:"prize_value"`
	}
	var rows []row
	db.Model(&LotteryDraw{}).
		Select("strftime('%Y-%m-%d', created_at) as day, pool, count(*) as draws, coalesce(sum(cost_points),0) as cost_points, coalesce(sum(prize_value),0) as prize_value").
		Where("created_at >= ?", startOfDay().AddDate(0, 0, -(days-1))).
		Group("day, pool").Order("day desc").Scan(&rows)

	total := map[string]interface{}{}
	var paidDraws, freeDraws int64
	var paidCost, paidBack, freeBack float64
	for _, x := range rows {
		switch x.Pool {
		case "paid":
			paidDraws += x.Draws
			paidCost += x.CostPoints
			paidBack += x.PrizeValue
		case "free":
			freeDraws += x.Draws
			freeBack += x.PrizeValue
		}
	}
	rtp := 0.0
	if paidCost > 0 {
		rtp = lotRound(paidBack / paidCost * 100)
	}
	total["paid_draws"] = paidDraws
	total["free_draws"] = freeDraws
	total["paid_cost_points"] = lotRound(paidCost)
	total["paid_back_points"] = lotRound(paidBack)
	total["free_back_points"] = lotRound(freeBack)
	total["paid_rtp_percent"] = rtp
	total["profit_points"] = lotRound(paidCost - paidBack)
	budget := settingF("lottery_daily_budget_points", "0")
	usedToday := 0.0
	db.Model(&LotteryDraw{}).Where("pool = ? AND created_at >= ? AND prize_value > 0 AND note <> ?", "paid", startOfDay(), "dev").
		Select("coalesce(sum(prize_value),0)").Scan(&usedToday)
	ok(w, map[string]interface{}{
		"days": rows, "total": total,
		"budget_points": lotRound(budget), "budget_used_today": lotRound(usedToday),
	})
}

// 扣回：写负向流水，不改原记录（项目没有退款回滚工具，这个入口必须有）
func handleAdminLotteryClawback(w http.ResponseWriter, r *http.Request) {
	if !methodGuard(w, r, http.MethodPost) {
		return
	}
	var in struct {
		DrawID uint   `json:"draw_id"`
		Note   string `json:"note"`
	}
	_ = json.NewDecoder(r.Body).Decode(&in)
	var d LotteryDraw
	if err := db.First(&d, in.DrawID).Error; err != nil {
		writeErr(w, 404, "流水不存在")
		return
	}
	if d.Revoked {
		writeErr(w, 400, "这条流水已经扣回过")
		return
	}
	if d.PrizeValue <= 0 || d.PrizeType == "none" {
		writeErr(w, 400, "这条流水没有可扣回的奖励")
		return
	}
	var u User
	if err := db.First(&u, d.UserID).Error; err != nil {
		writeErr(w, 404, "用户不存在")
		return
	}
	var out map[string]interface{}
	err := db.Transaction(func(tx *gorm.DB) error {
		switch d.PrizeType {
		case "lottery_points":
			if e := tx.Model(&User{}).Where("id = ?", u.ID).
				UpdateColumn("lottery_points", gorm.Expr("CASE WHEN COALESCE(lottery_points,0) >= ? THEN lottery_points - ? ELSE 0 END", d.PrizeValue, d.PrizeValue)).Error; e != nil {
				return e
			}
		case "bonus_points":
			if e := tx.Model(&User{}).Where("id = ?", u.ID).
				UpdateColumn("bonus_points", gorm.Expr("CASE WHEN COALESCE(bonus_points,0) >= ? THEN bonus_points - ? ELSE 0 END", d.PrizeValue, d.PrizeValue)).Error; e != nil {
				return e
			}
		}
		tx.Model(&LotteryDraw{}).Where("id = ?", d.ID).Updates(map[string]interface{}{
			"revoked": true, "note": in.Note,
		})
		var after User
		if e := tx.First(&after, u.ID).Error; e != nil {
			return e
		}
		neg := LotteryDraw{
			UserID: d.UserID, Username: d.Username, Pool: "clawback",
			PrizeID: d.PrizeID, PrizeName: "扣回：" + d.PrizeName,
			PrizeType: d.PrizeType, PrizeValue: -d.PrizeValue,
			BonusBefore: after.BonusPoints, BonusAfter: after.BonusPoints,
			LotteryBefore: after.LotteryPoints + pickLottery(d.PrizeType, d.PrizeValue),
			LotteryAfter:  after.LotteryPoints,
			IdemKey:       fmt.Sprintf("cb-%d-%d", d.ID, time.Now().UnixNano()),
			Revoked:       true, Note: in.Note, CreatedAt: time.Now(),
		}
		if e := tx.Create(&neg).Error; e != nil {
			return e
		}
		out = map[string]interface{}{
			"ok": true, "draw_id": d.ID,
			"lottery_points": lotRound(after.LotteryPoints), "bonus_points": lotRound(after.BonusPoints),
			"clawback_draw_id": neg.ID,
		}
		return nil
	})
	if err != nil {
		writeLotErr(w, err)
		return
	}
	ok(w, out)
}

func pickLottery(kind string, v float64) float64 {
	if kind == "lottery_points" {
		return v
	}
	return 0
}

func handleAdminLotterySettings(w http.ResponseWriter, r *http.Request) {
	keys := []string{
		"lottery_enabled", "lottery_cost_points", "lottery_daily_free",
		"lottery_daily_budget_points", "lottery_pay_source", "lottery_prize_sink",
		"lottery_show_probability", "lottery_newbie_paid_hours", "lottery_announcement",
	}
	if r.Method == http.MethodGet {
		kv := map[string]string{}
		for _, k := range keys {
			kv[k] = getSetting(k, "")
		}
		paidSum, paidOK := lotteryWeightOK("paid")
		freeSum, freeOK := lotteryWeightOK("free")
		ok(w, map[string]interface{}{
			"settings": kv, "paid_weight_sum": paidSum, "free_weight_sum": freeSum,
			"weight_total": lotteryWeightTotal, "weight_ok": paidOK && freeOK,
		})
		return
	}
	if !methodGuard(w, r, http.MethodPost) {
		return
	}
	var kv map[string]string
	if err := json.NewDecoder(r.Body).Decode(&kv); err != nil {
		writeErr(w, 400, "参数错误")
		return
	}
	allow := map[string]bool{}
	for _, k := range keys {
		allow[k] = true
	}
	saved := map[string]string{}
	for k, v := range kv {
		if !allow[k] {
			continue
		}
		if k == "lottery_pay_source" && v != "bonus_points" {
			writeErr(w, 400, "lottery_pay_source 目前只支持 bonus_points")
			return
		}
		setSetting(k, v)
		saved[k] = v
	}
	ok(w, map[string]interface{}{"ok": true, "saved": saved})
}

// ---------------- 初始化 ----------------

func seedLottery() {
	defaults := map[string]string{
		"lottery_enabled":             "1",
		"lottery_cost_points":         "2000",
		"lottery_daily_free":          "1",
		"lottery_daily_budget_points": "0",
		"lottery_pay_source":          "bonus_points",
		"lottery_prize_sink":          "lottery_points",
		"lottery_show_probability":    "1",
		"lottery_newbie_paid_hours":   "24",
		"lottery_announcement":        lotteryDefaultAnnounce,
	}
	for k, v := range defaults {
		if getSetting(k, "") == "" {
			setSetting(k, v)
		}
	}
	if getSetting("lottery_seeded", "") == "1" {
		logLotteryWeightState()
		return
	}

	type seed struct {
		name   string
		kind   string
		value  float64
		weight int
		stock  int
	}
	paid := []seed{
		{"0点", "none", 0, 4000, -1},
		{"10点", "lottery_points", 10, 1854, -1},
		{"50点", "lottery_points", 50, 1000, -1},
		{"100点", "lottery_points", 100, 800, -1},
		{"200点", "lottery_points", 200, 600, -1},
		{"500点", "lottery_points", 500, 500, -1},
		{"1000点", "lottery_points", 1000, 400, -1},
		{"2000点", "lottery_points", 2000, 300, -1},
		{"5000点", "lottery_points", 5000, 200, 100},
		{"1万点", "lottery_points", 10000, 150, 50},
		{"2万点", "lottery_points", 20000, 100, 20},
		{"5万点", "lottery_points", 50000, 60, 10},
		{"10万点", "lottery_points", 100000, 25, 5},
		{"20万点", "lottery_points", 200000, 10, 2},
		{"50万点", "lottery_points", 500000, 1, 1},
	}
	free := []seed{
		{"0点", "none", 0, 4900, -1},
		{"10点", "lottery_points", 10, 1500, -1},
		{"50点", "lottery_points", 50, 1000, -1},
		{"100点", "lottery_points", 100, 800, -1},
		{"200点", "lottery_points", 200, 600, -1},
		{"500点", "lottery_points", 500, 500, -1},
		{"1000点", "lottery_points", 1000, 400, -1},
		{"2000点", "lottery_points", 2000, 200, -1},
		{"5000点", "lottery_points", 5000, 80, -1},
		{"1万点", "lottery_points", 10000, 20, -1},
	}
	sort := 0
	for _, s := range paid {
		sort++
		db.Create(&LotteryPrize{Pool: "paid", Name: s.name, Kind: s.kind, Value: s.value,
			Weight: s.weight, DailyStock: s.stock, Enabled: true, Sort: sort})
	}
	sort = 0
	for _, s := range free {
		sort++
		db.Create(&LotteryPrize{Pool: "free", Name: s.name, Kind: s.kind, Value: s.value,
			Weight: s.weight, DailyStock: s.stock, Enabled: true, Sort: sort})
	}
	setSetting("lottery_seeded", "1")
	log.Printf("[lottery] 已初始化奖池：paid %d 条 / free %d 条", len(paid), len(free))
	logLotteryWeightState()
}
