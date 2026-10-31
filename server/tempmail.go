package main

// tempmail.go — 临时 / 一次性邮箱识别（2026-10-17）
//
// 用途：临时邮箱仍然允许注册与登录，但**不发放注册礼包 / 邀请奖励**，
// 避免用一次性邮箱批量注册薅点数。
//
// 判定方式：精确域名表 + 子域后缀匹配 + 关键词兜底（如 10minutemail.xx）。
// 宁可少判，不做过度猜测 —— 误判会让正常用户白丢奖励。

import "strings"

// 常见一次性 / 临时邮箱域名
var tempMailDomains = map[string]bool{
	// 国际主流
	"temp-mail.org":       true,
	"temp-mail.io":        true,
	"tempmail.com":        true,
	"tempmail.net":        true,
	"tempmail.plus":       true,
	"tempmailo.com":       true,
	"tempr.email":         true,
	"tempail.com":         true,
	"tempinbox.com":       true,
	"tempmailaddress.com": true,
	"guerrillamail.com":   true,
	"guerrillamail.net":   true,
	"guerrillamail.org":   true,
	"guerrillamail.de":    true,
	"sharklasers.com":     true,
	"grr.la":              true,
	"spam4.me":            true,
	"mailinator.com":      true,
	"mailinator.net":      true,
	"mailinator2.com":     true,
	"yopmail.com":         true,
	"yopmail.fr":          true,
	"yopmail.net":         true,
	"throwaway.email":     true,
	"throwawaymail.com":   true,
	"dispostable.com":     true,
	"trashmail.com":       true,
	"trashmail.de":        true,
	"trash-mail.com":      true,
	"fakeinbox.com":       true,
	"maildrop.cc":         true,
	"getnada.com":         true,
	"nada.email":          true,
	"burnermail.io":       true,
	"emailondeck.com":     true,
	"mintemail.com":       true,
	"spamgourmet.com":     true,
	"mytemp.email":        true,
	"moakt.com":           true,
	"mail7.io":            true,
	"mailnesia.com":       true,
	"mailcatch.com":       true,
	"tempinbox.xyz":       true,
	"emailtemp.org":       true,
	"10mail.org":          true,
	"10minutemail.com":    true,
	"10minutemail.net":    true,
	"10minutemail.org":    true,
	"20minutemail.com":    true,
	"minuteinbox.com":     true,
	"mohmal.com":          true,
	"inboxkitten.com":     true,
	"dropmail.me":         true,
	"smailpro.com":        true,
	"mailto.plus":         true,
	"etempmail.com":       true,
	"tempmail.ninja":      true,
	"tmpmail.org":         true,
	"tmpmail.net":         true,
	"1secmail.com":        true,
	"1secmail.net":        true,
	"1secmail.org":        true,
	"instant-mail.de":     true,
	"byom.de":             true,
	"mailnull.com":        true,
	"spambox.us":          true,
	"spamex.com":          true,
	"trbvm.com":           true,
	"vomoto.com":          true,
	"mailsac.com":         true,
	"emailfake.com":       true,
	"generator.email":     true,
	"mvrht.net":           true,
	"harakirimail.com":    true,
	"discard.email":       true,
	"discardmail.com":     true,
	"cock.li":             true,
	"tutanota.de":         false, // 正规加密邮箱，明确排除
	// 国内常见临时邮箱
	"linshiyouxiang.net": true,
	"linshiyou.com":      true,
	"24mail.chacuo.net":  true,
	"chacuo.net":         true,
	"mailx.top":          true,
	"yunmail.top":        true,
	"aiot.zone":          true,
}

// 关键词兜底（仅匹配域名里出现这些组合时判为临时邮箱）
var tempMailKeywords = []string{
	"tempmail", "temp-mail", "tempail", "tempinbox", "tempr.email",
	"10minutemail", "20minutemail", "minuteinbox", "1secmail",
	"guerrillamail", "sharklasers", "mailinator", "yopmail",
	"trashmail", "trash-mail", "throwaway", "fakeinbox",
	"linshiyou", "24mail", "dropmail", "maildrop", "mohmal",
	"emailfake", "generator.email", "burnermail", "mytemp.email",
}

// domainOf 取邮箱域名（小写）
func domainOf(email string) string {
	i := strings.LastIndex(email, "@")
	if i < 0 || i == len(email)-1 {
		return ""
	}
	return strings.ToLower(strings.TrimSpace(email[i+1:]))
}

// isTempEmail 是否临时 / 一次性邮箱
func isTempEmail(email string) bool {
	d := domainOf(normalizeEmail(email))
	if d == "" {
		return false
	}
	if v, hit := tempMailDomains[d]; hit {
		return v
	}
	// 子域：xxx.mailinator.com 之类
	for dom, on := range tempMailDomains {
		if on && strings.HasSuffix(d, "."+dom) {
			return true
		}
	}
	for _, kw := range tempMailKeywords {
		if strings.Contains(d, kw) {
			return true
		}
	}
	return false
}
