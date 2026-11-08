package main

// invite.go — 邀请码 / 邀请返利
//   注册八邀请码：邀请人 +500 点（在 auth.go 里已实现）
//   被邀请人付费赞助：按邀请人档位返 8% / 12% / 15%（本文件实现）
//   前端 GET /api/me/invite 拿自己的邀请码、已邀人数、累计返利

import (
	"encoding/json"
	"net/http"
	"strings"
	"time"

	"gorm.io/gorm"
)

// InviteReward 邀请返利流水
type InviteReward struct {
	ID        uint      `gorm:"primarykey" json:"id"`
	InviterID uint      `gorm:"index" json:"inviter_id"`
	InviteeID uint      `json:"invitee_id"`
	Invitee   string    `gorm:"size:64" json:"invitee"`
	OrderNo   string    `gorm:"size:40" json:"order_no"`
	Plan      string    `gorm:"size:16" json:"plan"`
	Amount    float64   `json:"amount"`
	Rate      float64   `json:"rate"`
	Points    float64   `json:"points"`
	CreatedAt time.Time `json:"created_at"`
}

// 邀请返利比例：按「邀请人」的档位算
func inviteRate(plan string) float64 {
	switch strings.ToLower(strings.TrimSpace(plan)) {
	case "c1", "lv1":
		return 0.08
	case "c2", "lv2":
		return 0.12
	case "c3", "lv3", "c4":
		return 0.15
	}
	return 0.05
}

// payInviteRebate 被邀请人付费后，给邀请人返点（写进加油包，30 天有效）
func payInviteRebate(o *Order) {
	if o == nil || o.Amount <= 0 {
		return
	}
	var buyer User
	if err := db.First(&buyer, o.UserID).Error; err != nil {
		return
	}
	if strings.TrimSpace(buyer.InvitedBy) == "" || buyer.EmailIsTemp {
		return
	}
	var inv User
	if err := db.First(&inv, "invite_code = ?", buyer.InvitedBy).Error; err != nil {
		return
	}
	if inv.EmailIsTemp || inv.ID == buyer.ID {
		return
	}
	rate := inviteRate(inv.Plan)
	pts := round2(o.Amount * cfg.PointPerYuan * rate)
	if pts <= 0 {
		return
	}
	// 同一笔订单只返一次（重复回调 / 重复核销都挡住）
	var dup int64
	db.Model(&InviteReward{}).Where("order_no = ?", o.OrderNo).Count(&dup)
	if dup > 0 {
		return
	}
	// 同上：SQL 自增，避免并发时用内存快照回写
	db.Model(&User{}).Where("id = ?", inv.ID).
		UpdateColumn("bonus_points", gorm.Expr("COALESCE(bonus_points,0) + ?", pts))
	if inv.BonusUntil == nil || inv.BonusUntil.Before(time.Now().AddDate(0, 0, 20)) {
		until := time.Now().AddDate(0, 0, 30)
		db.Model(&User{}).Where("id = ? AND (bonus_until IS NULL OR bonus_until < ?)", inv.ID, until).
			UpdateColumn("bonus_until", until)
	}
	db.Create(&InviteReward{
		InviterID: inv.ID, InviteeID: buyer.ID, Invitee: buyer.Username,
		OrderNo: o.OrderNo, Plan: o.Plan, Amount: o.Amount, Rate: rate,
		Points: pts, CreatedAt: time.Now(),
	})
}

// GET /api/me/invite
func handleMyInvite(w http.ResponseWriter, r *http.Request) {
	u := currentUser(r)
	var invitees []User
	db.Where("invited_by = ?", u.InviteCode).Order("id desc").Limit(100).Find(&invitees)
	list := make([]map[string]interface{}, 0, len(invitees))
	for i := range invitees {
		v := &invitees[i]
		list = append(list, map[string]interface{}{
			"username":      v.Username,
			"plan":          v.Plan,
			"created_at":    v.CreatedAt,
			"paid":          false,
			"email_is_temp": v.EmailIsTemp,
		})
	}
	// 标记谁付过费（有 paid 订单）
	for _, it := range list {
		un, _ := it["username"].(string)
		var n int64
		db.Model(&Order{}).Where("username = ? AND status = ?", un, "paid").Count(&n)
		it["paid"] = n > 0
	}
	var total float64
	db.Model(&InviteReward{}).Where("inviter_id = ?", u.ID).Select("coalesce(sum(points),0)").Scan(&total)
	var rewards []InviteReward
	db.Where("inviter_id = ?", u.ID).Order("id desc").Limit(20).Find(&rewards)

	ok(w, map[string]interface{}{
		"code":         u.InviteCode,
		"url":          cfg.PublicURL + "/?invite=" + u.InviteCode,
		"count":        len(invitees),
		"earned":       round2(total),
		"signup_bonus": 500,
		"rate":         inviteRate(u.Plan),
		"rate_next":    inviteRate(nextPlanOf(u.Plan)),
		"invitees":     list,
		"rewards":      rewards,
	})
}

// nextPlanOf 给前端提示「再升一档返利更高」
func nextPlanOf(plan string) string {
	switch strings.ToLower(strings.TrimSpace(plan)) {
	case "":
		return "lv1"
	case "lv1", "c1":
		return "lv2"
	case "lv2", "c2":
		return "lv3"
	}
	return ""
}

var _ = json.Marshal
