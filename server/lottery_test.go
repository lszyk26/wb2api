package main

// lottery_test.go — 抽奖 v2 自测（临时副本里跑，不进生产目录）
//
// 覆盖：charge 三段扣减顺序 / 免费次数 / 幂等 / 余额不足 / 临时邮箱 /
//       权重校验 / 库存售罄转 0 点 / 扣回 / 奖池互不影响

import (
	"encoding/json"
	"fmt"
	"net/http/httptest"
	"os"
	"strings"
	"testing"
	"time"
)

func setupTestDB(t *testing.T) {
	t.Helper()
	for _, f := range []string{"/tmp/ltest.db", "/tmp/ltest.db-wal", "/tmp/ltest.db-shm"} {
		_ = os.Remove(f)
	}
	openDB("/tmp/ltest.db")
	seedSettings()
	seedLottery()
	cfg.PointPerYuan = 1000
	lotUserLimiter = newLotLimiter()
	lotIPLimiter = newLotLimiter()
}

func newUser(t *testing.T, name string, bonus, lottery float64) *User {
	t.Helper()
	u := &User{
		Username: name, Email: name + "@test.local", PassHash: "x",
		DailyPoints: 100, PointsToday: 0, PointsDate: todayKey(),
		BonusPoints: bonus, LotteryPoints: lottery, InviteCode: name,
		CreatedAt: time.Now().AddDate(0, 0, -30),
	}
	if err := db.Create(u).Error; err != nil {
		t.Fatalf("create user: %v", err)
	}
	return u
}

func draw(t *testing.T, u *User, pool, key string) (int, map[string]interface{}) {
	t.Helper()
	body := fmt.Sprintf(`{"pool":%q,"idempotency_key":%q}`, pool, key)
	req := httptest.NewRequest("POST", "/api/lottery/draw", strings.NewReader(body))
	req = withUser(req, u)
	w := httptest.NewRecorder()
	handleLotteryDraw(w, req)
	var out map[string]interface{}
	_ = json.Unmarshal(w.Body.Bytes(), &out)
	return w.Code, out
}

func errCode(out map[string]interface{}) string {
	if e, ok := out["error"].(map[string]interface{}); ok {
		if c, ok := e["code"].(string); ok {
			return c
		}
	}
	return ""
}

func reload(id uint) User {
	var u User
	db.First(&u, id)
	return u
}

// 1) charge 三段扣减顺序：日额度 → lottery_points → bonus_points
func TestChargeOrder(t *testing.T) {
	setupTestDB(t)
	u := newUser(t, "c1", 1000, 50)
	u.PointsToday = 90
	u.PointsDate = todayKey()
	db.Save(u)

	// 扣 30：日额度剩 10，其余 20 走 lottery_points，bonus 不动
	charge(u, 30)
	if u.PointsToday != 100 {
		t.Fatalf("points_today = %v, want 100", u.PointsToday)
	}
	if u.LotteryPoints != 30 {
		t.Fatalf("lottery_points = %v, want 30", u.LotteryPoints)
	}
	if u.BonusPoints != 1000 {
		t.Fatalf("bonus_points = %v, want 1000（不应动付费余额）", u.BonusPoints)
	}

	// 落库确认
	uu := reload(u.ID)
	if uu.LotteryPoints != 30 || uu.BonusPoints != 1000 || uu.PointsToday != 100 {
		t.Fatalf("DB 落库不一致: %+v", uu)
	}

	// 再扣 200：lottery 30 用完 → bonus 扣 170
	charge(u, 200)
	uu = reload(u.ID)
	if uu.LotteryPoints != 0 {
		t.Fatalf("lottery_points = %v, want 0", uu.LotteryPoints)
	}
	if uu.BonusPoints != 830 {
		t.Fatalf("bonus_points = %v, want 830", uu.BonusPoints)
	}
	if uu.PointsToday != 100 {
		t.Fatalf("points_today = %v, want 100", uu.PointsToday)
	}

	// 三处都不够：多出来的仍计入 points_today（沿用旧行为）
	u3 := newUser(t, "c3", 100, 0)
	u3.PointsToday = 100
	u3.PointsDate = todayKey()
	db.Save(u3)
	charge(u3, 500)
	u3r := reload(u3.ID)
	if u3r.BonusPoints != 0 || u3r.LotteryPoints != 0 {
		t.Fatalf("余额应清零: %+v", u3r)
	}
	if u3r.PointsToday != 500 {
		t.Fatalf("points_today = %v, want 500（超额度记账行为需保留）", u3r.PointsToday)
	}
}

// 2) 免费池：每日 1 次，第二次被拒
func TestFreeLimit(t *testing.T) {
	setupTestDB(t)
	u := newUser(t, "f1", 0, 0)

	code, out := draw(t, u, "free", "f-1")
	if code != 200 {
		t.Fatalf("免费抽奖失败 %d %v", code, out)
	}
	if out["free_remaining_today"].(float64) != 0 {
		t.Fatalf("free_remaining_today = %v, want 0", out["free_remaining_today"])
	}
	code, out = draw(t, u, "free", "f-2")
	if code != 403 || errCode(out) != "LOTTERY_FREE_LIMIT_REACHED" {
		t.Fatalf("第二次免费应为 FREE_LIMIT_REACHED，got %d %v", code, out)
	}
	// 免费池不能动 bonus
	if reload(u.ID).BonusPoints != 0 {
		t.Fatal("免费池不应扣 bonus_points")
	}
}

// 3) 付费池：扣 2000 点；同 key 重放只扣一次，返回同一 draw_id
func TestPaidDrawIdempotent(t *testing.T) {
	setupTestDB(t)
	u := newUser(t, "p1", 10000, 0)

	code, out := draw(t, u, "paid", "p-key-1")
	if code != 200 {
		t.Fatalf("付费抽奖失败 %d %v", code, out)
	}
	drawID := out["draw_id"].(float64)
	prize := out["prize_value"].(float64)
	after := reload(u.ID)
	// 奖品一律发 lottery_points：付费点数只扣成本，赠点加奖品
	if after.BonusPoints != 10000-2000 {
		t.Fatalf("bonus 结算错误: %v, want 8000 (prize %v)", after.BonusPoints, prize)
	}
	if after.LotteryPoints != prize {
		t.Fatalf("lottery_points = %v, want %v", after.LotteryPoints, prize)
	}

	// 重放同 key
	code, out2 := draw(t, u, "paid", "p-key-1")
	if code != 200 || out2["draw_id"].(float64) != drawID {
		t.Fatalf("幂等失败: %d %v", code, out2)
	}
	if again := reload(u.ID); again.BonusPoints != after.BonusPoints {
		t.Fatalf("幂等重放重复扣款: %v → %v", after.BonusPoints, again.BonusPoints)
	}
	var n int64
	db.Model(&LotteryDraw{}).Where("user_id = ?", u.ID).Count(&n)
	if n != 1 {
		t.Fatalf("流水条数 = %d, want 1", n)
	}

	// 同 key 换奖池 → 409
	code, out3 := draw(t, u, "free", "p-key-1")
	if code != 409 || errCode(out3) != "LOTTERY_IDEMPOTENCY_CONFLICT" {
		t.Fatalf("同 key 换池应 409，got %d %v", code, out3)
	}
}

// 4) 余额不足 / 临时邮箱 / 缺幂等键
func TestDrawGuards(t *testing.T) {
	setupTestDB(t)
	poor := newUser(t, "g1", 100, 0)
	code, out := draw(t, poor, "paid", "g-1")
	if code != 400 || errCode(out) != "LOTTERY_INSUFFICIENT_BONUS" {
		t.Fatalf("余额不足应 INSUFFICIENT_BONUS，got %d %v", code, out)
	}

	temp := newUser(t, "g2", 10000, 0)
	temp.EmailIsTemp = true
	db.Save(temp)
	code, out = draw(t, temp, "paid", "g-2")
	if code != 403 || errCode(out) != "LOTTERY_TEMP_EMAIL_FORBIDDEN" {
		t.Fatalf("临时邮箱应被拒，got %d %v", code, out)
	}

	code, out = draw(t, poor, "paid", "")
	if code != 400 {
		t.Fatalf("缺幂等键应 400，got %d %v", code, out)
	}
}

// 5) 权重和 ≠ 10000 → 拒绝抽奖
func TestWeightInvalid(t *testing.T) {
	setupTestDB(t)
	u := newUser(t, "w1", 10000, 0)
	var p LotteryPrize
	db.Where("pool = ? AND kind = ?", "paid", "none").First(&p)
	db.Model(&LotteryPrize{}).Where("id = ?", p.ID).Update("weight", p.Weight+1) // 总和变 10001

	code, out := draw(t, u, "paid", "w-1")
	if code != 400 || errCode(out) != "LOTTERY_POOL_WEIGHT_INVALID" {
		t.Fatalf("权重异常应被拒，got %d %v", code, out)
	}
	if reload(u.ID).BonusPoints != 10000 {
		t.Fatal("权重异常时不应扣点")
	}
}

// 6) 大奖售罄 → 权重并入 0 点，抽取结果必然是 0 点
func TestStockExhausted(t *testing.T) {
	setupTestDB(t)
	u := newUser(t, "s1", 20000, 0)
	// 把 paid 池除 none 外全部置为库存 0
	db.Model(&LotteryPrize{}).Where("pool = ? AND kind <> ?", "paid", "none").Update("daily_stock", 0)

	code, out := draw(t, u, "paid", "s-1")
	if code != 200 {
		t.Fatalf("售罄时仍应可抽，got %d %v", code, out)
	}
	if out["prize_value"].(float64) != 0 {
		t.Fatalf("售罄时应只出 0 点，got %v", out["prize_value"])
	}
	if reload(u.ID).BonusPoints != 18000 {
		t.Fatalf("应只扣 2000，got %v", reload(u.ID).BonusPoints)
	}

	// 免费池不受影响
	code, out = draw(t, u, "free", "s-2")
	if code != 200 {
		t.Fatalf("免费池不应受付费池库存影响: %d %v", code, out)
	}
}

// 7) 扣回：写负向流水，赠点扣减，原记录标记 revoked
func TestClawback(t *testing.T) {
	setupTestDB(t)
	admin := newUser(t, "ad", 0, 0)
	admin.IsAdmin = true
	db.Save(admin)

	u := newUser(t, "cb1", 10000, 500)
	code, out := draw(t, u, "paid", "cb-1")
	if code != 200 {
		t.Fatalf("抽奖失败 %d %v", code, out)
	}
	prize := out["prize_value"].(float64)
	drawID := uint(out["draw_id"].(float64))

	body := fmt.Sprintf(`{"draw_id":%d,"note":"测试扣回"}`, drawID)
	req := httptest.NewRequest("POST", "/api/admin/lottery/clawback", strings.NewReader(body))
	req = withUser(req, admin)
	w := httptest.NewRecorder()
	handleAdminLotteryClawback(w, req)
	if w.Code != 200 {
		t.Fatalf("扣回失败 %d %s", w.Code, w.Body.String())
	}
	after := reload(u.ID)
	want := 500 + prize - prize // 赠点先加后扣 → 回到 500
	if after.LotteryPoints != want {
		t.Fatalf("扣回后 lottery_points = %v, want %v", after.LotteryPoints, want)
	}
	var d LotteryDraw
	db.First(&d, drawID)
	if !d.Revoked {
		t.Fatal("原流水应标记 revoked")
	}
	var neg LotteryDraw
	db.Where("pool = ?", "clawback").First(&neg)
	if neg.PrizeValue > 0 {
		t.Fatalf("负向流水金额应为负: %v", neg.PrizeValue)
	}

	// 重复扣回 → 拒绝
	req = httptest.NewRequest("POST", "/api/admin/lottery/clawback", strings.NewReader(body))
	req = withUser(req, admin)
	w = httptest.NewRecorder()
	handleAdminLotteryClawback(w, req)
	if w.Code == 200 {
		t.Fatal("重复扣回应被拒绝")
	}
}

// 8) 概率分布抽样：权重映射是否落在正确区间（不校验随机性，只校验边界）
func TestWeightMapping(t *testing.T) {
	setupTestDB(t)
	_, paidSum := loadPoolPrizes(db, "paid")
	_, freeSum := loadPoolPrizes(db, "free")
	if paidSum != lotteryWeightTotal {
		t.Fatalf("paid 权重和 = %d", paidSum)
	}
	if freeSum != lotteryWeightTotal {
		t.Fatalf("free 权重和 = %d", freeSum)
	}
	// 权重累计映射：0..3999 → 0点，4000..5853 → 10点
	var ps []LotteryPrize
	db.Where("pool = ? AND enabled = ?", "paid", true).Order("sort asc, id asc").Find(&ps)
	acc, idx := 0, -1
	for i, p := range ps {
		acc += p.Weight
		if 4600 < acc {
			idx = i
			break
		}
	}
	if idx < 0 || ps[idx].Value != 10 {
		t.Fatalf("随机数 4600 应落在 10 点档，got %v (idx %d)", ps[idx].Value, idx)
	}
}

// 9) 开发者账号：免费无限、付费不扣点、并发额度拉满
func TestDevUnlimited(t *testing.T) {
	setupTestDB(t)
	u := newUser(t, "dev1", 0, 0)
	u.Unlimited = true
	u.CreatedAt = time.Now() // 新号也不该被 24h 锁定
	db.Save(u)

	// 免费连抽 5 次都成功（普通用户每日只有 1 次）
	for i := 0; i < 5; i++ {
		code, out := draw(t, u, "free", fmt.Sprintf("dev-free-%d", i))
		if code != 200 {
			t.Fatalf("开发者免费抽第 %d 次失败 %d %v", i+1, code, out)
		}
	}
	// 付费池：0 余额也能抽，且不扣点
	if dailyQuota(u) < 1e7 {
		t.Fatalf("开发者日额度应视为无限，got %v", dailyQuota(u))
	}
	conc, rpm, _ := limitsOf(u)
	if conc < 64 || rpm < 1000 {
		t.Fatalf("开发者并发/RPM 未拉满: %d / %d", conc, rpm)
	}
	for i := 0; i < 3; i++ {
		code, out := draw(t, u, "paid", fmt.Sprintf("dev-paid-%d", i))
		if code != 200 {
			t.Fatalf("开发者付费抽第 %d 次失败 %d %v", i+1, code, out)
		}
		if out["cost_points"].(float64) != 0 {
			t.Fatalf("开发者抽奖不应扣点，got %v", out["cost_points"])
		}
	}
	if got := reload(u.ID).BonusPoints; got != 0 {
		t.Fatalf("开发者 bonus 不应被扣，got %v", got)
	}
	// 开发者流水标记 dev，统计里要排除
	var n int64
	db.Model(&LotteryDraw{}).Where("user_id = ? AND note = ?", u.ID, "dev").Count(&n)
	if n != 8 {
		t.Fatalf("应写入 8 条 dev 流水，got %d", n)
	}
}
