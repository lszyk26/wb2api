package main

// realmhealth.go — 双出口「按可用号数」动态配比
//
// 背景：cn 池和 intl 池的号数长期严重不对等（13 : 2），固定 70/30 其实一直
// 偏袒 intl 池：cn 每个号摊 5.4% 流量，intl 每个号摊 15%，intl 那两个号更容易
// 被上游判异常（11140）或熔断。而一旦某一侧可用号归零，固定比例还会白白让那
// 部分请求去撞一次 503 再故障转移。
//
// 这里改成：每分钟问一次 hub 面板「每个 realm 有几个号真能用」，
// 分流比例取 max(配置比例, cn 池的承载占比)。
//   - intl 全灭        → 100% 走 cn（自动止血，不用人肉改设置）
//   - 两边都健康        → 13:2 → 86:14，每个号的负载基本拉平
//   - 手动把 cn_split 设成 0/100 → 仍然可以强制单边
// hub 查不到时（网络/密码变更）自动退回配置比例，不会因为面板挂了就不能分流。

import (
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net/http"
	"strconv"
	"strings"
	"sync/atomic"
	"time"
)

type realmCount struct {
	CN    int
	Intl  int
	Known bool
	At    int64
}

var realmUsableState atomic.Value // realmCount

const realmHealthTTL = 60 * time.Second

// hubRealmUsable 问 hub 面板该 realm 有几个「真的能用」的号
func hubRealmUsable(realm string) (int, error) {
	// 注意：不能用 oauth.go 里的 hubPanel —— 它把响应截在 16KB，
	// 而 13 个账号的 /accounts 列表远超这个大小，会被截断成非法 JSON。
	tok, err := hubPanelToken(false)
	if err != nil {
		return 0, err
	}
	req, err := http.NewRequest("GET", cfg.UpstreamURL+"/accounts?realm="+realm, nil)
	if err != nil {
		return 0, err
	}
	req.Header.Set("X-Panel-Token", tok)
	cl := &http.Client{Timeout: 8 * time.Second}
	resp, err := cl.Do(req)
	if err != nil {
		return 0, err
	}
	defer resp.Body.Close()
	raw, err := io.ReadAll(io.LimitReader(resp.Body, 4<<20))
	if err != nil {
		return 0, err
	}
	if resp.StatusCode != 200 {
		return 0, fmt.Errorf("hub /accounts?realm=%s → %d", realm, resp.StatusCode)
	}
	// 注意字段类型：hub 的 expiresIn 是字符串（"27 days"），
	// breakerFor / cooldownFor 是可空整数。类型写错会让整个反序列化失败。
	type acc struct {
		Enabled           bool   `json:"enabled"`
		InCooldown        bool   `json:"inCooldown"`
		BreakerFor        *int64 `json:"breakerFor"`
		CooldownFor       *int64 `json:"cooldownFor"`
		DailyLimitBlocked bool   `json:"dailyLimitBlocked"`
		ExpiresIn         string `json:"expiresIn"`
	}
	var rows []acc
	if err := json.Unmarshal(raw, &rows); err != nil {
		// 兼容 {"accounts":[...]} 外壳
		var wrap struct {
			Accounts []acc `json:"accounts"`
		}
		if err2 := json.Unmarshal(raw, &wrap); err2 != nil {
			return 0, err
		}
		rows = wrap.Accounts
	}
	n := 0
	for _, a := range rows {
		if !a.Enabled || a.InCooldown || a.DailyLimitBlocked {
			continue
		}
		if a.BreakerFor != nil && *a.BreakerFor > 0 {
			continue // 熔断中
		}
		if a.CooldownFor != nil && *a.CooldownFor > 0 {
			continue // 冷却中
		}
		if !expiresInPositive(a.ExpiresIn) {
			continue // 凭据过期
		}
		n++
	}
	return n, nil
}

// expiresInPositive 解析 hub 的 "27 days" 这种字符串，判断凭据还没过期
func expiresInPositive(s string) bool {
	s = strings.TrimSpace(s)
	if s == "" {
		return false
	}
	i := 0
	for i < len(s) && (s[i] == '-' || (s[i] >= '0' && s[i] <= '9')) {
		i++
	}
	if i == 0 {
		return true // 认不出来的格式，不因为解析问题把号判死
	}
	v, err := strconv.ParseFloat(s[:i], 64)
	if err != nil {
		return true
	}
	return v > 0
}

// refreshRealmHealth 刷新两侧可用号数；两侧都失败时保留上一次的结果
func refreshRealmHealth() {
	cur, _ := realmUsableState.Load().(realmCount)
	cn, e1 := hubRealmUsable("cn")
	intl, e2 := hubRealmUsable("intl")
	if e1 != nil && e2 != nil {
		log.Printf("[realm] 可用号数查询失败：cn=%v intl=%v", e1, e2)
		return
	}
	if e1 != nil {
		log.Printf("[realm] cn 查询失败：%v", e1)
	}
	if e2 != nil {
		log.Printf("[realm] intl 查询失败：%v", e2)
	}
	if e1 != nil {
		cn = cur.CN
	}
	if e2 != nil {
		intl = cur.Intl
	}
	next := realmCount{CN: cn, Intl: intl, Known: true, At: time.Now().Unix()}
	if !cur.Known || cur.CN != cn || cur.Intl != intl {
		log.Printf("[realm] 可用号数更新：cn=%d intl=%d → cn 分流 %d%%", cn, intl, effectiveCNSplitPercentWith(next))
	}
	realmUsableState.Store(next)
}

func startRealmHealthLoop() {
	refreshRealmHealth()
	go func() {
		for {
			time.Sleep(realmHealthTTL)
			refreshRealmHealth()
		}
	}()
}

// effectiveCNSplitPercentWith 纯函数版本，便于单测
func effectiveCNSplitPercentWith(h realmCount) int {
	p := cnSplitPercent()
	if p <= 0 || p >= 100 {
		return p // 0/100 视为强制单边，不受容量干预
	}
	if !h.Known {
		return p
	}
	if h.CN+h.Intl == 0 {
		return p
	}
	if h.Intl == 0 && h.CN > 0 {
		return 100
	}
	if h.CN == 0 && h.Intl > 0 {
		return 0
	}
	capCn := h.CN * 100 / (h.CN + h.Intl)
	if capCn > p {
		return capCn
	}
	return p
}

func effectiveCNSplitPercent() int {
	h, _ := realmUsableState.Load().(realmCount)
	return effectiveCNSplitPercentWith(h)
}

// realmUsableSnapshot 给 /api/status 用
func realmUsableSnapshot() map[string]interface{} {
	h, _ := realmUsableState.Load().(realmCount)
	return map[string]interface{}{
		"cn_usable":    h.CN,
		"intl_usable":  h.Intl,
		"known":        h.Known,
		"cn_split_eff": effectiveCNSplitPercent(),
	}
}
