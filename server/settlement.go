package main

// settlement.go — 月度贡献结算 + 账号健康采样
//
// 结算口径（重要）：
//   日额度（dailyQuota）= 用户当天能用的点数，由档位保底决定，每日 0 点重置。
//   月度奖励           = 账号上月实际承载的算力折算成加油包（bonus），不占日额度。
//
// 折算公式：
//   某账号上月 credit 消耗 × credit_to_points(默认1000) × 用户档位比例
//   例：账号上月承载 20 credit（≈20 元算力），C2 用户 45% →
//       20 × 1000 × 0.45 = 9000 点加油包
//
// 为什么按「消耗」而不是按「余额」：余额多不代表贡献多，
// 只有被真实调用掉的部分才是池子实际承担的成本。

import (
	"log"
	"time"
)

// ---------------- 账号健康采样 ----------------
func sampleAccountHealth() {
	var accs []ContribAccount
	db.Where("state IN ?", []string{"ok", "cool"}).Find(&accs)
	if len(accs) == 0 {
		return
	}
	cut := time.Now().Add(-1 * time.Hour).
		Format("2006-01-02 15:04:05")
	for _, a := range accs {
		var calls int64
		db.Model(&UpstreamUsage{}).
			Where("account = ? AND at >= ?", a.UIDRef, cut).
			Count(&calls)
		healthy := calls > 0

		// 连续 3 小时无调用 → 判定离线（写入 note，但不改 state，由管理员决定）
		var recent int64
		db.Model(&UpstreamUsage{}).
			Where("account = ? AND at >= ?", a.UIDRef, time.Now().Add(-3*time.Hour)).
			Count(&recent)

		db.Create(&AccountHealth{
			AccountID: a.ID, UIDRef: a.UIDRef,
			Healthy: healthy, Calls: calls, At: time.Now(),
		})

		if recent == 0 && a.State == "ok" {
			db.Model(&ContribAccount{}).Where("id = ?", a.ID).
				Update("note", "最近 3 小时无调用，请确认账号是否仍被上游使用")
		} else if a.State == "ok" && a.Note != "" && recent > 0 {
			db.Model(&ContribAccount{}).Where("id = ?", a.ID).Update("note", "")
		}
	}
}

// 统计某账号在时间区间内的在线率（0~100）
func uptimeOf(accountID uint, since time.Time) float64 {
	var total, up int64
	db.Model(&AccountHealth{}).
		Where("account_id = ? AND at >= ?", accountID, since).Count(&total)
	db.Model(&AccountHealth{}).
		Where("account_id = ? AND at >= ? AND healthy = ?", accountID, since, true).Count(&up)
	if total == 0 {
		return 0
	}
	return float64(up) / float64(total) * 100
}

// ---------------- 月度结算 ----------------
type SettleItem struct {
	UserID      uint    `json:"user_id"`
	Username    string  `json:"username"`
	AccountID   uint    `json:"account_id"`
	AccountUID  string  `json:"account_uid"`
	Credits     float64 `json:"credits"`
	Credit2Pts  float64 `json:"credit_to_points"`
	Ratio       float64 `json:"ratio"`
	Points      float64 `json:"points"`
}

type SettleReport struct {
	Period    string       `json:"period"`
	Items     []SettleItem `json:"items"`
	TotalPts  float64      `json:"total_points"`
	Users     int          `json:"users"`
	Skipped   int          `json:"skipped"`
	RanAt     string       `json:"ran_at"`
}

// 结算某个月的贡献（month 形如 2026-09；空 = 上个月）
func runSettlement(month string, dryRun bool) (*SettleReport, error) {
	now := time.Now()
	if month == "" {
		month = now.AddDate(0, -1, 0).Format("2006-01")
	}
	start, err := time.Parse("2006-01", month)
	if err != nil {
		return nil, err
	}
	end := start.AddDate(0, 1, 0)

	// 幂等：已结算过就跳过
	if getSetting("settled_"+month, "") != "" && !dryRun {
		return &SettleReport{Period: month, RanAt: now.Format(time.RFC3339), Skipped: 1}, nil
	}

	c2p := envF("WB2_CREDIT_TO_POINTS", 1000)

	var accs []ContribAccount
	db.Where("state = ?", "ok").Find(&accs)

	items := make([]SettleItem, 0, len(accs))
	userTotal := map[uint]float64{}
	userName := map[uint]string{}
	userRatio := map[uint]float64{}
	total := 0.0

	for _, a := range accs {
		var credit float64
		db.Model(&UpstreamUsage{}).
			Where("account = ? AND at >= ? AND at < ?", a.UIDRef, start, end).
			Select("coalesce(sum(credit),0)").Scan(&credit)
		if credit <= 0 {
			continue
		}
		var u User
		if err := db.First(&u, a.UserID).Error; err != nil {
			continue
		}
		ratio := u.ContribRatio
		if ratio <= 0 {
			ratio = planOf(u.Plan).Ratio
		}
		if ratio <= 0 {
			ratio = 0.30
		}
		pts := round2(credit * c2p * ratio)
		items = append(items, SettleItem{
			UserID: u.ID, Username: u.Username, AccountID: a.ID,
			AccountUID: a.UIDRef, Credits: round2(credit),
			Credit2Pts: c2p, Ratio: ratio, Points: pts,
		})
		userTotal[u.ID] += pts
		userName[u.ID] = u.Username
		userRatio[u.ID] = ratio
		total += pts
	}

	rep := &SettleReport{
		Period: month, Items: items, TotalPts: round2(total),
		Users: len(userTotal), RanAt: now.Format(time.RFC3339),
	}

	if dryRun {
		return rep, nil
	}

	// 入账：写加油包（90 天有效）
	until := now.AddDate(0, 0, 90)
	for uid, pts := range userTotal {
		var u User
		if err := db.First(&u, uid).Error; err != nil {
			continue
		}
		newBonus := u.BonusPoints + pts
		updates := map[string]interface{}{"bonus_points": newBonus}
		if u.BonusUntil == nil || u.BonusUntil.Before(until) {
			updates["bonus_until"] = until
		}
		db.Model(&User{}).Where("id = ?", uid).Updates(updates)
		db.Create(&Settlement{
			Period: month, UserID: uid, Username: userName[uid],
			Points: round2(pts), Ratio: userRatio[uid],
			Accounts: len(items), At: now,
		})
		log.Printf("[settle] %s → %s +%.0f 点", month, userName[uid], pts)
	}
	setSetting("settled_"+month, now.Format(time.RFC3339))
	setSetting("settled_last_report", jsonStr(rep))
	return rep, nil
}

// 定时：每月 1 日 00:10 结算上月；另做一个兜底：每小时检查一次「上月是否已结算」
func settlementLoop() {
	time.Sleep(90 * time.Second)
	for {
		now := time.Now()
		lastMonth := now.AddDate(0, -1, 0).Format("2006-01")
		if getSetting("settled_"+lastMonth, "") == "" && now.Day() >= 1 {
			// 等到当月 1 号 00:10 之后
			firstThisMonth := time.Date(now.Year(), now.Month(), 1, 0, 10, 0, 0, now.Location())
			if now.After(firstThisMonth) {
				if _, err := runSettlement(lastMonth, false); err != nil {
					log.Printf("[settle] 失败：%v", err)
				}
			}
		}
		time.Sleep(1 * time.Hour)
	}
}

// 清理 90 天前的心跳采样（保存量）
func pruneHealth() {
	cut := time.Now().AddDate(0, 0, -90)
	db.Where("at < ?", cut).Delete(&AccountHealth{})
	db.Where("at < ?", cut).Delete(&PayLog{})
}
