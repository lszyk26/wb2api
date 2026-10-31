package main

// oauth.go — 直接 OAuth 授权：转发 wb hub 的浏览器登录流程
//   start: hub /accounts/login/start 拿 authUrl，前端新标签打开
//   poll : hub /accounts/login/poll，ok 时账号已进池，这里补贡献记录并升级档位
// hub 面板接口要 X-Panel-Token，用 WB2_HUB_PANEL_PASS 换

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"os"
	"strings"
	"sync"
	"time"
)

var (
	panelTokMu  sync.Mutex
	panelTok    string
	panelTokGot time.Time
)

func hubPanelToken(force bool) (string, error) {
	pass := strings.TrimSpace(os.Getenv("WB2_HUB_PANEL_PASS"))
	if pass == "" {
		return "", errors.New("未配置 WB2_HUB_PANEL_PASS（wb 面板密码），OAuth 授权暂不可用")
	}
	panelTokMu.Lock()
	defer panelTokMu.Unlock()
	if !force && panelTok != "" && time.Since(panelTokGot) < 6*time.Hour {
		return panelTok, nil
	}
	body, _ := json.Marshal(map[string]string{"password": pass})
	req, err := http.NewRequest("POST", cfg.UpstreamURL+"/panel/login", bytes.NewReader(body))
	if err != nil {
		return "", err
	}
	req.Header.Set("Content-Type", "application/json")
	cl := &http.Client{Timeout: 20 * time.Second}
	resp, err := cl.Do(req)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()
	raw, _ := io.ReadAll(io.LimitReader(resp.Body, 4096))
	if resp.StatusCode != 200 {
		return "", errors.New("面板登录失败：" + strings.TrimSpace(string(raw)))
	}
	var out map[string]interface{}
	if err := json.Unmarshal(raw, &out); err != nil {
		return "", errors.New("面板登录返回异常")
	}
	tok, _ := out["token"].(string)
	if tok == "" {
		return "", errors.New("面板登录未返回 token")
	}
	panelTok, panelTokGot = tok, time.Now()
	return panelTok, nil
}

func hubPanel(method, path string, body []byte) (int, []byte, error) {
	for attempt := 0; attempt < 2; attempt++ {
		tok, err := hubPanelToken(attempt > 0)
		if err != nil {
			return 0, nil, err
		}
		var rd io.Reader
		if body != nil {
			rd = bytes.NewReader(body)
		}
		req, err := http.NewRequest(method, cfg.UpstreamURL+path, rd)
		if err != nil {
			return 0, nil, err
		}
		req.Header.Set("Content-Type", "application/json")
		req.Header.Set("X-Panel-Token", tok)
		cl := &http.Client{Timeout: 30 * time.Second}
		resp, err := cl.Do(req)
		if err != nil {
			return 0, nil, err
		}
		raw, _ := io.ReadAll(io.LimitReader(resp.Body, 16384))
		resp.Body.Close()
		if resp.StatusCode == 401 && attempt == 0 {
			continue
		}
		return resp.StatusCode, raw, nil
	}
	return 0, nil, errors.New("hub 鉴权失败")
}

// POST /api/me/account/oauth/start  {realm}
func handleOAuthStart(w http.ResponseWriter, r *http.Request) {
	u := currentUser(r)
	var in map[string]interface{}
	_ = json.NewDecoder(r.Body).Decode(&in)
	realm := strings.ToLower(strings.TrimSpace(defaultStr(strVal(in["realm"]), "intl")))
	if realm != "intl" && realm != "cn" {
		writeErr(w, 400, "realm 只能是 intl 或 cn")
		return
	}
	body, _ := json.Marshal(map[string]string{"realm": realm, "platform": "CLI"})
	code, raw, err := hubPanel("POST", "/accounts/login/start", body)
	if err != nil {
		writeErr(w, 502, "启动授权失败："+err.Error())
		return
	}
	if code != 200 {
		writeErr(w, 502, "启动授权失败："+strings.TrimSpace(string(raw)))
		return
	}
	var out map[string]interface{}
	if err := json.Unmarshal(raw, &out); err != nil {
		writeErr(w, 502, "上游返回无法解析")
		return
	}
	state, _ := out["state"].(string)
	authURL, _ := out["authUrl"].(string)
	if state == "" || authURL == "" {
		writeErr(w, 502, "上游没有返回授权地址")
		return
	}
	ok(w, map[string]interface{}{"ok": true, "state": state, "authUrl": authURL, "realm": realm})
	_ = u
}

// POST /api/me/account/oauth/poll  {state}
func handleOAuthPoll(w http.ResponseWriter, r *http.Request) {
	u := currentUser(r)
	var in map[string]interface{}
	_ = json.NewDecoder(r.Body).Decode(&in)
	state := strings.TrimSpace(strVal(in["state"]))
	if state == "" {
		writeErr(w, 400, "缺少 state")
		return
	}
	code, raw, err := hubPanel("GET", "/accounts/login/poll?state="+state, nil)
	if err != nil {
		writeErr(w, 502, "查询授权状态失败："+err.Error())
		return
	}
	if code != 200 {
		writeErr(w, 502, strings.TrimSpace(string(raw)))
		return
	}
	var out map[string]interface{}
	_ = json.Unmarshal(raw, &out)
	status, _ := out["status"].(string)
	switch status {
	case "pending":
		ok(w, map[string]interface{}{"ok": true, "status": "pending", "message": defaultStr(strVal(out["message"]), "等待浏览器完成登录")})
		return
	case "ok":
		acc, _ := out["account"].(map[string]interface{})
		uid := strVal(acc["uid"])
		label := defaultStr(strVal(acc["nickname"]), "授权的账号")
		realm := defaultStr(strVal(acc["realm"]), "intl")

		// 授权完立刻自检一条最小请求：跑得通才留，跑不通直接打回并从池子里删掉
		if uid != "" {
			okTest, note := probeNewAccount(uid)
			if !okTest {
				delBody, _ := json.Marshal(map[string]string{"uid": uid})
				_, _, _ = hubPanel("POST", "/accounts/delete", delBody)
				var rej ContribAccount
				if db.First(&rej, "uid_ref = ?", uid).Error == nil {
					db.Model(&ContribAccount{}).Where("id = ?", rej.ID).Updates(map[string]interface{}{
						"state": "rejected", "user_id": u.ID, "username": u.Username,
						"note": "自检失败已打回：" + note,
					})
				} else {
					db.Create(&ContribAccount{
						UserID: u.ID, Username: u.Username, Realm: realm, Label: label, UIDRef: uid,
						State: "rejected", Note: "自检失败已打回：" + note,
					})
				}
				ok(w, map[string]interface{}{
					"ok": true, "status": "rejected", "uid": uid, "nickname": label,
					"message": "账号自检没通过，已打回并从池子移除：" + note,
				})
				return
			}
			// 通过 → 挂一个独立出口槽位，避免多个账号共用同一出口 IP 被连坐
			bindProxySlot(uid)
		}
		var exist ContribAccount
		if uid != "" && db.First(&exist, "uid_ref = ?", uid).Error == nil {
			db.Model(&ContribAccount{}).Where("id = ?", exist.ID).Updates(map[string]interface{}{
				"state": "ok", "label": label, "user_id": u.ID, "username": u.Username,
				"note": "OAuth 授权接入（" + time.Now().Format("2006-01-02 15:04") + "）",
			})
		} else {
			db.Create(&ContribAccount{
				UserID: u.ID, Username: u.Username, Realm: realm, Label: label, UIDRef: uid,
				State: "ok", Note: "OAuth 授权接入（" + time.Now().Format("2006-01-02 15:04") + "）",
			})
		}
		autoUpgradeContrib(u.ID)
		ok(w, map[string]interface{}{"ok": true, "status": "ok", "uid": uid, "nickname": label, "realm": realm})
		return
	default:
		ok(w, map[string]interface{}{"ok": true, "status": status, "message": strVal(out["message"])})
	}
}

func strVal(v interface{}) string {
	if s, ok := v.(string); ok {
		return s
	}
	return ""
}

// probeNewAccount 用 hub 的 /accounts/test 打一条最小请求，判断新授权的号能不能用
func probeNewAccount(uid string) (bool, string) {
	body, _ := json.Marshal(map[string]string{"uid": uid, "model": "deepseek-v4.1-flash"})
	code, raw, err := hubPanel("POST", "/accounts/test", body)
	if err != nil {
		return true, "" // hub 自身异常时不误杀，交给后面人工复核
	}
	if code != 200 {
		return true, ""
	}
	var out map[string]interface{}
	if json.Unmarshal(raw, &out) != nil {
		return true, ""
	}
	if okv, _ := out["ok"].(bool); okv {
		return true, ""
	}
	reason := strings.TrimSpace(strVal(out["error"]))
	if reason == "" {
		reason = "上游拒绝该账号"
	}
	if len(reason) > 200 {
		reason = reason[:200]
	}
	return false, reason
}

// bindProxySlot 给新账号分配一个独立出口
// 优先用 WB2_PREF_SLOTS 里的住宅出口（默认 slot-11 = 美国 Frontier 住宅），
// 不够用时再轮转机房槽位 slot-1..slot-10
func bindProxySlot(uid string) {
	var used int64
	db.Model(&ContribAccount{}).Where("state = ?", "ok").Count(&used)
	pref := strings.TrimSpace(os.Getenv("WB2_PREF_SLOTS"))
	var slot string
	if pref != "" {
		list := []string{}
		for _, x := range strings.Split(pref, ",") {
			if v := strings.TrimSpace(x); v != "" {
				list = append(list, v)
			}
		}
		if len(list) > 0 {
			slot = list[int(used)%len(list)]
		}
	}
	if slot == "" {
		slot = fmt.Sprintf("slot-%d", int(used)%10+1)
	}
	body, _ := json.Marshal(map[string]interface{}{"uid": uid, "proxySlot": slot})
	_, _, _ = hubPanel("POST", "/accounts/set", body)
}
