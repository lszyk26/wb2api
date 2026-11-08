package main

// payments_test.go — 支付回调幂等性
//
// 支付平台会重复投递同一笔通知（同一单的 GET + POST、超时重试、人工补单），
// markOrderPaid 必须保证「同一笔订单只到账一次」，否则点数/档位/邀请返利会被重复发放。

import "testing"

func TestMarkOrderPaidIdempotent(t *testing.T) {
	setupTestDB(t)
	seedPlans()
	if err := db.Create(&Plan{Code: "p1", Kind: "points", Name: "加油包", Daily: 12000, Price: 10, Enabled: true}).Error; err != nil {
		t.Fatalf("seed points plan: %v", err)
	}
	u := newUser(t, "buyer", 0, 0)
	o := Order{OrderNo: "TEST-ORDER-1", UserID: u.ID, Username: u.Username, Plan: "p1", Months: 1, Amount: 10, Status: "pending"}
	if err := db.Create(&o).Error; err != nil {
		t.Fatalf("create order: %v", err)
	}

	markOrderPaid(&o, "首次回调")
	markOrderPaid(&o, "重复回调")
	markOrderPaid(&o, "又一次重投")

	var got User
	if err := db.First(&got, u.ID).Error; err != nil {
		t.Fatalf("读回用户: %v", err)
	}
	if got.BonusPoints != 12000 {
		t.Fatalf("重复回调重复到账：bonus=%v（应只加一次 12000）", got.BonusPoints)
	}
	var oc Order
	db.First(&oc, o.ID)
	if oc.Status != "paid" {
		t.Fatalf("订单状态应为 paid，实际 %q", oc.Status)
	}
}

func TestPayInviteRebateOnce(t *testing.T) {
	setupTestDB(t)
	seedPlans()

	inviter := newUser(t, "inviter", 0, 0)
	inviter.InviteCode = "INVCODE1"
	if err := db.Save(inviter).Error; err != nil {
		t.Fatalf("save inviter: %v", err)
	}
	buyer := newUser(t, "invitee", 0, 0)
	buyer.InvitedBy = "INVCODE1"
	if err := db.Save(buyer).Error; err != nil {
		t.Fatalf("save buyer: %v", err)
	}

	o := Order{OrderNo: "TEST-ORDER-2", UserID: buyer.ID, Username: buyer.Username, Plan: "lv1", Months: 1, Amount: 10, Status: "pending"}
	if err := db.Create(&o).Error; err != nil {
		t.Fatalf("create order: %v", err)
	}
	markOrderPaid(&o, "第一次")
	markOrderPaid(&o, "重复回调")

	var n int64
	db.Model(&InviteReward{}).Where("order_no = ?", o.OrderNo).Count(&n)
	if n != 1 {
		t.Fatalf("返利流水 %d 条（同一笔订单应只有 1 条）", n)
	}
	var iv User
	db.First(&iv, inviter.ID)
	want := round2(10 * cfg.PointPerYuan * inviteRate(inviter.Plan))
	if iv.BonusPoints != want {
		t.Fatalf("邀请人返利 %v（应 %v），说明重复发放了", iv.BonusPoints, want)
	}
}
