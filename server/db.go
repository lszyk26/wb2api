package main

// db.go — 数据模型与迁移

import (
	"log"
	"time"

	"github.com/glebarez/sqlite"
	"gorm.io/gorm"
	"gorm.io/gorm/logger"
)

var db *gorm.DB

type User struct {
	ID            uint   `gorm:"primarykey" json:"id"`
	Username      string `gorm:"uniqueIndex;size:64" json:"username"`
	Email         string `gorm:"size:128;index" json:"email"`
	EmailVerified bool   `json:"email_verified"`                                  // 邮箱已验证（验证码通过）
	EmailIsTemp   bool   `json:"email_is_temp"`                                   // 临时邮箱（不发注册礼包）
	GitHubID      int64  `gorm:"column:github_id;index" json:"github_id"`         // GitHub 用户 ID（0 = 未绑定）
	GitHubLogin   string `gorm:"column:github_login;size:64" json:"github_login"` // GitHub 登录名（仅展示）
	PassHash      string `json:"-"`
	IsAdmin       bool   `json:"is_admin"`
	Banned        bool   `json:"banned"`
	BanReason     string `json:"ban_reason"`

	// 档位："" 游客 / lv1 lv2 lv3 / c1 c2 c3 / stack(叠加)
	Plan      string     `gorm:"size:16" json:"plan"`
	PlanUntil *time.Time `json:"plan_until"`

	// 额度
	DailyPoints float64    `json:"daily_points"` // 每日额度（0 = 用档位默认）
	PointsToday float64    `json:"points_today"` // 今日已用
	PointsDate  string     `gorm:"size:10" json:"points_date"`
	BonusPoints float64    `json:"bonus_points"` // 加油包 / 卡密剩余点数
	BonusUntil  *time.Time `json:"bonus_until"`
	// 抽奖赠点：永久有效、只能用于 API 消费（charge 的第二个扣减源），
	// 永不进入抽奖扣款白名单 —— 资金流单向，天然防「中奖→再抽」循环
	LotteryPoints float64 `gorm:"default:0" json:"lottery_points"`
	// Unlimited = 开发者账号：额度/并发/抽奖次数全部不设上限，抽奖不扣点
	Unlimited bool `gorm:"default:0" json:"unlimited"`

	// 授权贡献
	ContribRatio float64 `json:"contrib_ratio"` // 实际生效折算比例
	Student      bool    `json:"student"`       // 学生认证

	InviteCode string `gorm:"uniqueIndex;size:16" json:"invite_code"`
	InvitedBy  string `gorm:"size:16" json:"invited_by"`

	CreatedAt time.Time  `json:"created_at"`
	LastLogin *time.Time `json:"last_login"`
}

type APIKey struct {
	ID        uint       `gorm:"primarykey" json:"id"`
	UserID    uint       `gorm:"index" json:"user_id"`
	Name      string     `gorm:"size:64" json:"name"`
	Prefix    string     `gorm:"size:16;index" json:"prefix"`
	Hash      string     `gorm:"size:64;uniqueIndex" json:"-"` // sha256(完整 key)
	Secret    string     `json:"-"`                            // 加密后的完整 key（用于再次展示）
	Revoked   bool       `json:"revoked"`
	LastUsed  *time.Time `json:"last_used"`
	Calls     int64      `json:"calls"`
	CreatedAt time.Time  `json:"created_at"`
}

type LogRow struct {
	ID        uint      `gorm:"primarykey" json:"id"`
	UserID    uint      `gorm:"index" json:"-"`
	KeyID     uint      `gorm:"index" json:"key_id"`
	Model     string    `gorm:"size:64;index" json:"model"`
	PromptTok int       `json:"prompt_tokens"`
	ComplTok  int       `json:"completion_tokens"`
	Points    float64   `json:"points"`
	Cost      float64   `json:"cost"` // 上游 credit
	Status    int       `json:"status"`
	Error     string    `gorm:"size:255" json:"error"`
	Ms        int       `json:"ms"`
	TTFT      int       `json:"ttft_ms"`
	Stream    bool      `json:"stream"`
	IP        string    `gorm:"size:64" json:"ip"`
	CreatedAt time.Time `gorm:"index" json:"created_at"`
}

// 赞助订单（卡密支付 / 管理员确认）
type Order struct {
	ID        uint       `gorm:"primarykey" json:"id"`
	OrderNo   string     `gorm:"uniqueIndex;size:40" json:"order_no"`
	UserID    uint       `gorm:"index" json:"user_id"`
	Username  string     `gorm:"size:64" json:"username"`
	Plan      string     `gorm:"size:16" json:"plan"`
	Months    int        `json:"months"`
	Amount    float64    `json:"amount"`
	Status    string     `gorm:"size:16;index" json:"status"` // pending / paid / cancelled / expired
	Method    string     `gorm:"size:16" json:"method"`       // alipay / alipay2 / wechat / manual
	Note      string     `gorm:"size:255" json:"note"`
	TradeNo   string     `gorm:"size:64" json:"trade_no"`
	PayURL    string     `gorm:"type:text" json:"-"`
	CreatedAt time.Time  `json:"created_at"`
	PaidAt    *time.Time `json:"paid_at"`
}

// 支付回调流水（对账用）
type PayLog struct {
	ID      uint      `gorm:"primarykey" json:"id"`
	OrderNo string    `gorm:"size:40;index" json:"order_no"`
	Channel string    `gorm:"size:16" json:"channel"`
	Action  string    `gorm:"size:16" json:"action"`
	TradeNo string    `gorm:"size:64" json:"trade_no"`
	Amount  float64   `json:"amount"`
	At      time.Time `gorm:"index" json:"at"`
}

// 账号健康采样（每 10 分钟一条）
type AccountHealth struct {
	ID        uint      `gorm:"primarykey" json:"id"`
	AccountID uint      `gorm:"index" json:"account_id"`
	UIDRef    string    `gorm:"size:64;index" json:"uid_ref"`
	Healthy   bool      `json:"healthy"`
	Calls     int64     `json:"calls"`
	At        time.Time `gorm:"index" json:"at"`
}

// 月度结算记录
type Settlement struct {
	ID       uint      `gorm:"primarykey" json:"id"`
	Period   string    `gorm:"size:10;index" json:"period"`
	UserID   uint      `gorm:"index" json:"user_id"`
	Username string    `gorm:"size:64" json:"username"`
	Points   float64   `json:"points"`
	Ratio    float64   `json:"ratio"`
	Accounts int       `json:"accounts"`
	At       time.Time `json:"at"`
}

// 卡密（充值码 / 档位码）
type RedeemCode struct {
	ID        uint       `gorm:"primarykey" json:"id"`
	Code      string     `gorm:"uniqueIndex;size:32" json:"code"`
	Kind      string     `gorm:"size:16" json:"kind"` // points / plan
	Points    float64    `json:"points"`
	Plan      string     `gorm:"size:16" json:"plan"`
	Months    int        `json:"months"`
	UsedBy    uint       `json:"used_by"`
	UsedAt    *time.Time `json:"used_at"`
	ExpireAt  *time.Time `json:"expire_at"`
	Note      string     `gorm:"size:128" json:"note"`
	CreatedAt time.Time  `json:"created_at"`
}

// 用户提交的授权账号（贡献算力）
type ContribAccount struct {
	ID         uint       `gorm:"primarykey" json:"id"`
	UserID     uint       `gorm:"index" json:"user_id"`
	Username   string     `gorm:"size:64" json:"username"`
	Realm      string     `gorm:"size:8" json:"realm"` // intl / cn
	Label      string     `gorm:"size:64" json:"label"`
	UIDRef     string     `gorm:"size:64" json:"uid_ref"` // 上游账号 uid
	Credits    float64    `json:"credits"`                // 最近观测积分
	Latency    int        `json:"latency_ms"`
	State      string     `gorm:"size:16" json:"state"` // pending / ok / cool / rejected / revoked
	Note       string     `gorm:"size:255" json:"note"`
	Credential string     `json:"-"` // 加密后的凭据 JSON
	LastSeen   *time.Time `json:"last_seen"`
	CreatedAt  time.Time  `json:"created_at"`
}

// 档位（可在线调整，前端 /api/plans 读取）
type Plan struct {
	ID         uint    `gorm:"primarykey" json:"-"`
	Code       string  `gorm:"uniqueIndex;size:16" json:"code"`
	Kind       string  `gorm:"size:16" json:"kind"` // sponsor / contrib
	Name       string  `gorm:"size:32" json:"name"`
	Tagline    string  `gorm:"size:128" json:"tagline"`
	Price      float64 `json:"price"`
	YearPrice  float64 `json:"year_price"`
	Daily      float64 `json:"daily"`
	Conc       int     `json:"conc"`
	RPM        int     `json:"rpm"`
	LogDays    int     `json:"log_days"`
	Ratio      float64 `json:"ratio"` // 贡献档折算比例
	MinCredits float64 `json:"min_credits"`
	MinUptime  float64 `json:"min_uptime"`
	Accounts   int     `json:"accounts"`
	Perks      string  `gorm:"type:text" json:"perks"` // JSON 数组
	Need       string  `gorm:"type:text" json:"need"`
	Hot        bool    `json:"hot"`
	Sort       int     `json:"sort"`
	Enabled    bool    `json:"enabled"`
}

// 系统设置（键值）
type Setting struct {
	K string `gorm:"primarykey;size:64"`
	V string `gorm:"type:text"`
}

// 上游真实用量（从 usage.jsonl 增量导入，用于对账/看板）
type UpstreamUsage struct {
	ID          uint      `gorm:"primarykey" json:"id"`
	At          time.Time `gorm:"index" json:"at"`
	Model       string    `gorm:"size:64;index" json:"model"`
	Account     string    `gorm:"size:64;index" json:"account"`
	Realm       string    `gorm:"size:8" json:"realm"`
	PromptTok   int       `json:"prompt_tokens"`
	ComplTok    int       `json:"completion_tokens"`
	Credit      float64   `json:"credit"`
	Elapsed     int       `json:"elapsed_ms"`
	TTFT        int       `json:"ttft_ms"`
	Outcome     string    `gorm:"size:24" json:"outcome"`
	Fingerprint string    `gorm:"uniqueIndex;size:64" json:"-"` // at+model+sha 去重
}

func openDB(path string) {
	var err error
	db, err = gorm.Open(sqlite.Open(path+"?_pragma=busy_timeout(5000)&_pragma=journal_mode(WAL)"), &gorm.Config{
		Logger: logger.Default.LogMode(logger.Silent),
	})
	if err != nil {
		log.Fatalf("[db] open: %v", err)
	}
	if err := db.AutoMigrate(&User{}, &APIKey{}, &LogRow{}, &Order{}, &RedeemCode{},
		&ContribAccount{}, &Plan{}, &Setting{}, &UpstreamUsage{},
		&PayLog{}, &AccountHealth{}, &Settlement{}, &EmailCode{}, &InviteReward{},
		&LotteryPrize{}, &LotteryPrizeDay{}, &LotteryDraw{}); err != nil {
		log.Fatalf("[db] migrate: %v", err)
	}
	seedPlans()
	log.Printf("[db] ready: %s", path)
}

func getSetting(k, def string) string {
	var s Setting
	if err := db.First(&s, "k = ?", k).Error; err == nil {
		return s.V
	}
	return def
}

func setSetting(k, v string) {
	db.Save(&Setting{K: k, V: v})
}
