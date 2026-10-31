package main

// payments.go — 支付通道对接
//
// 目前支持两种驱动：
//   epay  = 彩虹易支付（submit.php / mapi.php / api.php），ziyuanpay.top 实测可用
//   mupa  = 木瓦公益码支付（wsk-me.com），协议待确认，先留桩
//
// 所有密钥从 data/env 读取，不落代码。

import (
	"crypto/md5"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"sort"
	"strconv"
	"strings"
	"time"
)

type PayChannel struct {
	Code    string // alipay / alipay2 / wechat
	Name    string
	Driver  string // epay / mupa
	Gateway string
	PID     string
	Secret  string
	Enabled bool
}

func loadChannels() []PayChannel {
	mk := func(code, name, driver, base, pid, secret string) PayChannel {
		return PayChannel{
			Code: code, Name: name, Driver: driver,
			Gateway: strings.TrimRight(base, "/"),
			PID:     pid, Secret: secret,
			Enabled: base != "" && pid != "" && secret != "",
		}
	}
	out := []PayChannel{
		mk("alipay", "支付宝", "mupa",
			env("WB2_PAY_ALIPAY_URL", ""), env("WB2_PAY_ALIPAY_PID", ""), env("WB2_PAY_ALIPAY_KEY", "")),
		mk("alipay2", "支付宝（备用）", "epay",
			env("WB2_PAY_ALIPAY2_URL", ""), env("WB2_PAY_ALIPAY2_PID", ""), env("WB2_PAY_ALIPAY2_KEY", "")),
		mk("wechat", "微信支付", "epay",
			env("WB2_PAY_WECHAT_URL", ""), env("WB2_PAY_WECHAT_PID", ""), env("WB2_PAY_WECHAT_KEY", "")),
	}
	// 同一家网关的两个商户号，微信/支付宝共用一套协议
	for i := range out {
		if out[i].Code == "alipay2" && out[i].Enabled {
			out[i].Driver = "epay"
		}
	}
	return out
}

func channelByCode(code string) PayChannel {
	for _, c := range loadChannels() {
		if c.Code == code {
			return c
		}
	}
	return PayChannel{}
}

// ---------------- 易支付签名 ----------------
// 规则：参数按 ASCII 字典序排序，排除 sign / sign_type / 空值，
//      拼成 a=b&c=d 后直接追加商户密钥，MD5 小写。
func epaySign(params map[string]string, key string) string {
	keys := make([]string, 0, len(params))
	for k, v := range params {
		if k == "sign" || k == "sign_type" || v == "" {
			continue
		}
		keys = append(keys, k)
	}
	sort.Strings(keys)
	var b strings.Builder
	for i, k := range keys {
		if i > 0 {
			b.WriteByte('&')
		}
		b.WriteString(k)
		b.WriteByte('=')
		b.WriteString(params[k])
	}
	b.WriteString(key)
	sum := md5.Sum([]byte(b.String()))
	return hex.EncodeToString(sum[:])
}

func (c PayChannel) verifyNotify(p map[string]string) bool {
	if c.Secret == "" {
		return false
	}
	sign := strings.ToLower(p["sign"])
	if sign == "" {
		return false
	}
	want := strings.ToLower(epaySign(p, c.Secret))
	if sign != want {
		return false
	}
	// 商户号必须匹配
	if p["pid"] != "" && p["pid"] != c.PID {
		return false
	}
	return true
}

// ---------------- 创建支付 ----------------
type PayResult struct {
	OrderNo  string `json:"order_no"`
	PayURL   string `json:"pay_url"`
	QRCode   string `json:"qrcode"`
	Scheme   string `json:"urlscheme"`
	Channel  string `json:"channel"`
	Amount   float64 `json:"amount"`
	ExpireAt string `json:"expire_at"`
}

func createPayment(o *Order, ch PayChannel, payType, clientIP string) (*PayResult, error) {
	if !ch.Enabled {
		return nil, errors.New("该支付通道未配置")
	}
	notifyBase := env("WB2_PAY_NOTIFY_BASE", cfg.PublicURL)
	notify := notifyBase + "/api/pay/notify/" + ch.Code
	ret := notifyBase + "/api/pay/return"
	name := fmt.Sprintf("WorkBuddy2 %s %d个月", strings.ToUpper(o.Plan), o.Months)
	money := strconv.FormatFloat(o.Amount, 'f', 2, 64)

	switch ch.Driver {
	case "epay":
		return epayCreate(o, ch, payType, notify, ret, name, money, clientIP)
	case "mupa":
		return mupaCreate(o, ch, payType, notify, ret, name, money, clientIP)
	}
	return nil, fmt.Errorf("未知支付驱动：%s", ch.Driver)
}

// 彩虹易支付 mapi.php
func epayCreate(o *Order, ch PayChannel, payType, notify, ret, name, money, ip string) (*PayResult, error) {
	p := map[string]string{
		"pid":          ch.PID,
		"type":         payType, // alipay / wxpay
		"out_trade_no": o.OrderNo,
		"notify_url":   notify,
		"return_url":   ret,
		"name":         name,
		"money":        money,
		"clientip":     ip,
		"device":       "pc",
		"sitename":     "WorkBuddy 2.0",
	}
	p["sign"] = epaySign(p, ch.Secret)
	p["sign_type"] = "MD5"

	form := url.Values{}
	for k, v := range p {
		form.Set(k, v)
	}

	// 优先用 mapi.php 拿 JSON（能给二维码），失败再退回 submit.php 跳转
	client := &http.Client{Timeout: 20 * time.Second}
	resp, err := client.PostForm(ch.Gateway+"/mapi.php", form)
	if err == nil {
		defer resp.Body.Close()
		body, _ := io.ReadAll(io.LimitReader(resp.Body, 8192))
		var r struct {
			Code     int    `json:"code"`
			Msg      string `json:"msg"`
			TradeNo  string `json:"trade_no"`
			PayURL   string `json:"payurl"`
			QRCode   string `json:"qrcode"`
			URLScheme string `json:"urlscheme"`
		}
		if json.Unmarshal(body, &r) == nil && r.Code == 1 {
			return &PayResult{
				OrderNo: o.OrderNo, PayURL: r.PayURL, QRCode: r.QRCode,
				Scheme: r.URLScheme, Channel: ch.Code, Amount: o.Amount,
				ExpireAt: time.Now().Add(30 * time.Minute).Format(time.RFC3339),
			}, nil
		}
		// mapi 不可用 → 用页面跳转兜底
		if strings.Contains(string(body), "未配置") || len(body) > 0 {
			q := form.Encode()
			return &PayResult{
				OrderNo: o.OrderNo,
				PayURL:  ch.Gateway + "/submit.php?" + q,
				Channel: ch.Code, Amount: o.Amount,
				ExpireAt: time.Now().Add(30 * time.Minute).Format(time.RFC3339),
			}, nil
		}
	}
	return nil, fmt.Errorf("支付网关不可达：%v", err)
}

// 木瓦公益码支付：协议未公开，先用页面跳转形式（参数名与易支付兼容）
func mupaCreate(o *Order, ch PayChannel, payType, notify, ret, name, money, ip string) (*PayResult, error) {
	p := map[string]string{
		"pid":          ch.PID,
		"type":         payType,
		"out_trade_no": o.OrderNo,
		"notify_url":   notify,
		"return_url":   ret,
		"name":         name,
		"money":        money,
		"sitename":     "WorkBuddy 2.0",
	}
	p["sign"] = epaySign(p, ch.Secret)
	p["sign_type"] = "MD5"
	q := url.Values{}
	for k, v := range p {
		q.Set(k, v)
	}
	return &PayResult{
		OrderNo: o.OrderNo,
		PayURL:  ch.Gateway + "/submit.php?" + q.Encode(),
		Channel: ch.Code, Amount: o.Amount,
		ExpireAt: time.Now().Add(30 * time.Minute).Format(time.RFC3339),
	}, nil
}

// ---------------- 异步回调 ----------------
func handlePayNotify(w http.ResponseWriter, r *http.Request) {
	code := strings.TrimPrefix(r.URL.Path, "/api/pay/notify/")
	ch := channelByCode(code)
	if ch.Code == "" {
		http.Error(w, "channel not found", 404)
		return
	}
	_ = r.ParseForm()
	p := map[string]string{}
	for k := range r.Form {
		p[k] = r.Form.Get(k)
	}
	// GET 也要收
	for k := range r.URL.Query() {
		if _, ok := p[k]; !ok {
			p[k] = r.URL.Query().Get(k)
		}
	}

	if !ch.verifyNotify(p) {
		http.Error(w, "sign error", 400)
		return
	}
	orderNo := p["out_trade_no"]
	status := p["trade_status"]
	if status != "TRADE_SUCCESS" && status != "SUCCESS" {
		w.Write([]byte("success"))
		return
	}
	var o Order
	if err := db.First(&o, "order_no = ?", orderNo).Error; err != nil {
		http.Error(w, "order not found", 404)
		return
	}
	if o.Status == "paid" {
		w.Write([]byte("success"))
		return
	}
	// 金额校验（防篡改）
	if paid, err := strconv.ParseFloat(p["money"], 64); err == nil {
		if paid+0.01 < o.Amount {
			http.Error(w, "amount mismatch", 400)
			return
		}
	}
	markOrderPaid(&o, "自动到账 · "+ch.Name+" "+p["trade_no"])
	logPayEvent(o.OrderNo, ch.Code, "paid", p["trade_no"], o.Amount)
	w.Write([]byte("success"))
}

func handlePayReturn(w http.ResponseWriter, r *http.Request) {
	orderNo := r.URL.Query().Get("out_trade_no")
	u := "/?paid=" + url.QueryEscape(orderNo)
	http.Redirect(w, r, u, http.StatusFound)
}

func logPayEvent(orderNo, channel, action, tradeNo string, amount float64) {
	db.Create(&PayLog{
		OrderNo: orderNo, Channel: channel, Action: action,
		TradeNo: tradeNo, Amount: amount, At: time.Now(),
	})
}
