package main

// gateway.go — OpenAI 兼容入口
//
// 流程：sk-wb-* 密钥 → 用户 → 额度/并发/限速校验 → 转发到上游 workbuddy 网关
//       → 流式透传并解析 usage → 计费扣点 → 写日志

import (
	"bufio"
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"io"
	"log"
	"net/http"
	"strconv"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"gorm.io/gorm"
)

// ---------------- 内存限流 ----------------
type bucket struct {
	conc   int
	stamps []int64
	mu     sync.Mutex
}

var buckets sync.Map // uid -> *bucket

func getBucket(uid uint) *bucket {
	v, _ := buckets.LoadOrStore(uid, &bucket{})
	return v.(*bucket)
}

func (b *bucket) tryAcquire(maxConc, rpm int) (release func(), errMsg string) {
	b.mu.Lock()
	defer b.mu.Unlock()
	if maxConc > 0 && b.conc >= maxConc {
		return nil, "并发已达上限（同时进行中的请求），请稍后重试"
	}
	if rpm > 0 {
		now := time.Now().Unix()
		keep := b.stamps[:0]
		for _, t := range b.stamps {
			if now-t < 60 {
				keep = append(keep, t)
			}
		}
		b.stamps = keep
		if len(b.stamps) >= rpm {
			return nil, "超出每分钟请求上限，请稍后重试"
		}
		b.stamps = append(b.stamps, now)
	}
	b.conc++
	return func() {
		b.mu.Lock()
		if b.conc > 0 {
			b.conc--
		}
		b.mu.Unlock()
	}, ""
}

// ---------------- 密钥 ----------------
func hashKey(k string) string {
	s := sha256.Sum256([]byte(k))
	return hex.EncodeToString(s[:])
}

func keyBySecret(secret string) (*APIKey, *User) {
	var k APIKey
	if err := db.First(&k, "hash = ? AND revoked = ?", hashKey(secret), false).Error; err != nil {
		return nil, nil
	}
	var u User
	if err := db.First(&u, k.UserID).Error; err != nil {
		return nil, nil
	}
	return &k, &u
}

// ---------------- 额度 ----------------
func rollDay(u *User) {
	today := time.Now().Format("2006-01-02")
	if u.PointsDate != today {
		u.PointsDate = today
		u.PointsToday = 0
		db.Model(&User{}).Where("id = ?", u.ID).Updates(map[string]interface{}{
			"points_date": today, "points_today": 0,
		})
	}
	// 过期加油包清零
	if u.BonusUntil != nil && time.Now().After(*u.BonusUntil) && u.BonusPoints > 0 {
		db.Model(&User{}).Where("id = ?", u.ID).Update("bonus_points", 0)
		u.BonusPoints = 0
	}
	// 赞助过期 → 掉回游客
	if u.Plan != "" && u.PlanUntil != nil && time.Now().After(*u.PlanUntil) {
		db.Model(&User{}).Where("id = ?", u.ID).Updates(map[string]interface{}{"plan": "", "plan_until": nil})
		u.Plan = ""
	}
}

// quotaCheck 返回 允许 / 降速 / 拒绝
func quotaState(u *User) (leftDaily, leftBonus, leftLottery, quota float64) {
	rollDay(u)
	quota = dailyQuota(u)
	leftDaily = max0(quota - u.PointsToday)
	leftBonus = u.BonusPoints
	leftLottery = u.LotteryPoints
	return
}

// charge 计费扣点。
//
// 扣减顺序（改动前务必确认三处调用方）：
//  1. 日额度池（points_today，每天重置）
//  2. lottery_points（抽奖赠点，永久有效）—— 赠点先花，付费余额留到最后
//  3. bonus_points（付费点数，有 bonus_until）
//
// 三处都不够时，超出部分仍然记进 points_today（沿用原有「超额度记账」行为，
// 由 handleChatCompletions 的 1.5 倍软上限拦截）。
func charge(u *User, pts float64) {
	rollDay(u)
	quota := dailyQuota(u)

	fromDaily := pts
	if quota > 0 {
		if left := max0(quota - u.PointsToday); fromDaily > left {
			fromDaily = left
		}
	} else {
		fromDaily = 0
	}
	rest := pts - fromDaily

	fromLottery := 0.0
	if rest > 0 && u.LotteryPoints > 0 {
		fromLottery = rest
		if fromLottery > u.LotteryPoints {
			fromLottery = u.LotteryPoints
		}
		rest -= fromLottery
	}

	fromBonus := 0.0
	if rest > 0 && u.BonusPoints > 0 {
		fromBonus = rest
		if fromBonus > u.BonusPoints {
			fromBonus = u.BonusPoints
		}
		rest -= fromBonus
	}

	// 余额都不够 → 多出来的仍计入 points_today（保持原有行为）
	leftover := rest
	if leftover > 0 {
		fromDaily += leftover
	}

	if fromDaily != 0 {
		db.Model(&User{}).Where("id = ?", u.ID).
			UpdateColumn("points_today", gorm.Expr("points_today + ?", fromDaily))
		u.PointsToday += fromDaily
	}
	if fromLottery > 0 {
		db.Model(&User{}).Where("id = ?", u.ID).
			UpdateColumn("lottery_points", gorm.Expr("CASE WHEN COALESCE(lottery_points,0) >= ? THEN lottery_points - ? ELSE 0 END", fromLottery, fromLottery))
		u.LotteryPoints -= fromLottery
	}
	if fromBonus > 0 {
		db.Model(&User{}).Where("id = ?", u.ID).
			UpdateColumn("bonus_points", gorm.Expr("CASE WHEN COALESCE(bonus_points,0) >= ? THEN bonus_points - ? ELSE 0 END", fromBonus, fromBonus))
		u.BonusPoints -= fromBonus
	}
}

// ---------------- 模型缓存 ----------------
// modelsRefreshing 防止并发刷新模型缓存（一次刷新要打上游，别叠罗汉）
var modelsRefreshing int32

func modelsCacheAge() time.Duration {
	v := getSetting("models_cached_at", "")
	if v == "" {
		return 24 * time.Hour
	}
	t, err := time.Parse(time.RFC3339, v)
	if err != nil {
		return 24 * time.Hour
	}
	return time.Since(t)
}

func upstreamDo(method, path string, body []byte, stream bool) (*http.Response, error) {
	return upstreamDoRealm(method, path, body, stream, "")
}

// upstreamDoRealm 同上，但可指定上游出口（X-Realm: cn / intl）
func upstreamDoRealm(method, path string, body []byte, stream bool, realm string) (*http.Response, error) {
	req, err := http.NewRequest(method, cfg.UpstreamURL+path, bytes.NewReader(body))
	if err != nil {
		return nil, err
	}
	req.Header.Set("Authorization", "Bearer "+cfg.UpstreamKey)
	if realm != "" {
		req.Header.Set("X-Realm", realm)
	}
	req.Header.Set("Content-Type", "application/json")
	if stream {
		req.Header.Set("Accept", "text/event-stream")
	}
	cl := &http.Client{Timeout: 0}
	if !stream {
		cl.Timeout = 300 * time.Second
	}
	return cl.Do(req)
}

// ---------------- 国内版（cn）出口 ----------------
// hub 的调度按 realm 分池：非专属模型一律走 WB_PROXY_DEFAULT_REALM（=intl），
// 而站内 10 个账号是 codebuddy.cn 的国内版号，永远轮不到。这里把「只在国内版
// 提供的模型」显式打上 X-Realm: cn，让它们落到 cn 池；其余模型行为不变。
// 已逐个用 /accounts/test 实测可用的模型才放进来（glm-5.1 实测 11133，未收）。
var cnRealmModels = map[string]bool{
	"deepseek-v4-pro":    true,
	"kimi-k3-1":          true,
	"minimax-m3":         true,
	"minimax-m3.1-flash": true,
	"minimax-m2.7":       true,
	"hy3-x":              true,
	"kimi-k2.7":          true,
	"glm-5v-turbo":       true,
	"space-bunny":        true,
	"mimo-v2.6-pro":      true,
}

// realmOfModel 返回该模型要求的出口（"cn" 或 "" = 跟随默认 intl）
func realmOfModel(model string) string {
	m := strings.ToLower(strings.TrimSpace(model))
	if cnRealmModels[m] {
		return "cn"
	}
	return ""
}

// mergeCNCatalog 把国内版模型目录并进国际版目录（按 id 去重），
// 让 /v1/models 同时列出两个出口能跑的模型。
func mergeCNCatalog(intlBody []byte) []byte {
	if len(intlBody) == 0 {
		return intlBody
	}
	var intl map[string]interface{}
	if json.Unmarshal(intlBody, &intl) != nil {
		return intlBody
	}
	data, _ := intl["data"].([]interface{})
	seen := map[string]bool{}
	for _, it := range data {
		if m, ok := it.(map[string]interface{}); ok {
			if id, ok := m["id"].(string); ok {
				seen[id] = true
			}
		}
	}
	resp, err := upstreamDoRealm("GET", "/v1/models", nil, false, "cn")
	if err != nil {
		return intlBody
	}
	defer resp.Body.Close()
	if resp.StatusCode != 200 {
		return intlBody
	}
	body, _ := io.ReadAll(io.LimitReader(resp.Body, 512<<10))
	var cn struct {
		Data []map[string]interface{} `json:"data"`
	}
	if json.Unmarshal(body, &cn) != nil || len(cn.Data) == 0 {
		return intlBody
	}
	added := 0
	for _, m := range cn.Data {
		id, _ := m["id"].(string)
		if !cnRealmModels[id] {
			continue
		}
		if ok := id != ""; ok && !seen[id] {
			m["realm"] = "cn"
			data = append(data, m)
			seen[id] = true
			added++
		}
	}
	if added == 0 {
		return intlBody
	}
	intl["data"] = data
	intl["cn_model_count"] = added
	out, err := json.Marshal(intl)
	if err != nil {
		return intlBody
	}
	return out
}

// ---------------- 双出口分流（cn 池 / intl 池） ----------------
// 站内 11 个国内版号长期空转、intl 池只有 1 个号扛全部真实流量，这是反复
// 11140 的根子。这里把「两边都能跑」的模型按比例分给 cn 池，intl 专属模型
// 保持原样；任一侧报错就自动换另一侧重试一次。
const cnSplitDefault = 70

var cnPenaltyUntil int64   // unix 秒：cn 侧刚失败过 → 短暂不再分流过去
var intlPenaltyUntil int64 // unix 秒：intl 侧刚失败过（常见是 429 限额）→ 短暂全给 cn

// isIntlExclusive 只有国际版出口提供的模型（hub 侧 exclusive_realm 同口径）
func isIntlExclusive(model string) bool {
	m := strings.ToLower(strings.TrimSpace(model))
	if strings.HasPrefix(m, "gpt-") || strings.HasPrefix(m, "gemini-") {
		return true
	}
	return m == "grok-4.7"
}

func cnSplitPercent() int {
	raw := strings.TrimSpace(getSetting("cn_split", ""))
	if raw == "" {
		return cnSplitDefault
	}
	n, err := strconv.Atoi(raw)
	if err != nil || n < 0 || n > 100 {
		return cnSplitDefault
	}
	return n
}

// pickDualRealmFor 按「会话」而不是「请求」分出口：同一段对话里第一条用户消息
// 决定走 cn 还是 intl，后续轮次跟着走，避免来回切换把上游的 prefix 缓存打散。
// 返回 "" = 默认 intl，"cn" = 国内版。
func pickDualRealmFor(raw []byte) string {
	now := time.Now().Unix()
	cnOut := now < atomic.LoadInt64(&cnPenaltyUntil)
	intlOut := now < atomic.LoadInt64(&intlPenaltyUntil)
	if cnOut && !intlOut {
		return ""
	}
	if intlOut && !cnOut {
		return "cn"
	}
	if cnOut && intlOut {
		return ""
	}
	p := effectiveCNSplitPercent() // 配置比例 + 两侧可用号数，取对 cn 更有利的
	if p <= 0 {
		return ""
	}
	if p >= 100 {
		return "cn"
	}
	var pl struct {
		Messages []struct {
			Role    string          `json:"role"`
			Content json.RawMessage `json:"content"`
		} `json:"messages"`
	}
	seed := ""
	if json.Unmarshal(raw, &pl) == nil {
		for _, m := range pl.Messages {
			if m.Role == "user" && len(m.Content) > 0 {
				seed = string(m.Content)
				break
			}
		}
	}
	if seed == "" {
		seed = string(raw[:min(len(raw), 512)])
	}
	sum := sha256.Sum256([]byte(seed))
	if int(sum[0])*100/256 < p {
		return "cn"
	}
	return ""
}

func altRealm(realm string) string {
	if realm == "cn" {
		return ""
	}
	return "cn"
}

// realmFailoverWorthy 这一侧「明显不是模型本身的问题」，值得换另一侧重试
func realmFailoverWorthy(status int, body []byte) bool {
	if status >= 500 || status == 429 {
		return true
	}
	t := string(body)
	for _, kw := range []string{"11140", "账号存在异常行为", "no usable account", "accounts_ready", "cooling", "冷却"} {
		if strings.Contains(t, kw) {
			return true
		}
	}
	return false
}

// penalize 某一侧刚失败（cn 池没号 / intl 被限流），300 秒内优先走另一侧。
// 上游「no usable account / 熔断」通常以几十分钟计，惩罚太短会让每个新会话都白撞一枪。
func penalize(realm string) {
	until := time.Now().Unix() + 300
	if realm == "cn" {
		atomic.StoreInt64(&cnPenaltyUntil, until)
	} else {
		atomic.StoreInt64(&intlPenaltyUntil, until)
	}
}

// forwardChat 把请求转给上游，双出口模型带自动分流与故障转移。
// 返回的 resp 若为错误响应，body 已缓冲好(可直接读)；若为成功响应则原样透传。
func forwardChat(model string, raw []byte, stream bool) (*http.Response, error) {
	realm := realmOfModel(model)
	dual := false
	if realm == "" && !isIntlExclusive(model) {
		dual = true
		realm = pickDualRealmFor(raw)
	}
	resp, err := upstreamDoRealm("POST", "/v1/chat/completions", raw, stream, realm)
	if !dual {
		return resp, err
	}
	if err != nil {
		penalize(realm)
		return upstreamDoRealm("POST", "/v1/chat/completions", raw, stream, altRealm(realm))
	}
	if resp.StatusCode < 400 {
		return resp, nil
	}
	body, _ := io.ReadAll(io.LimitReader(resp.Body, 64<<10))
	resp.Body.Close()
	log.Printf("[split] primary realm=%q status=%d failover_worthy=%v", realm, resp.StatusCode, realmFailoverWorthy(resp.StatusCode, body))
	if !realmFailoverWorthy(resp.StatusCode, body) {
		resp.Body = io.NopCloser(bytes.NewReader(body))
		return resp, nil
	}
	penalize(realm)
	r2, err2 := upstreamDoRealm("POST", "/v1/chat/completions", raw, stream, altRealm(realm))
	log.Printf("[split] failover realm=%q err=%v status=%d", altRealm(realm), err2, func() int {
		if r2 == nil {
			return -1
		}
		return r2.StatusCode
	}())
	if err2 != nil {
		resp.Body = io.NopCloser(bytes.NewReader(body))
		return resp, nil
	}
	if r2.StatusCode >= 400 {
		b2, _ := io.ReadAll(io.LimitReader(r2.Body, 64<<10))
		r2.Body.Close()
		r2.Body = io.NopCloser(bytes.NewReader(b2))
	}
	return r2, nil
}

// ---------------- /v1/models ----------------
func handleV1Models(w http.ResponseWriter, r *http.Request) {
	if cors(w, r) {
		return
	}
	// 缓存优先：/v1/models 若同步透传上游，偶尔会卡 15~17 秒。
	// 客户端（尤其手机端）启动/保存配置时都会拉一次，卡住就体感「服务挂了」。
	// 所以有缓存就直接返回，过期只在后台异步刷新，绝不阻塞客户端。
	if raw := getSetting("models_cache", ""); raw != "" {
		if modelsCacheAge() > 5*time.Minute && atomic.CompareAndSwapInt32(&modelsRefreshing, 0, 1) {
			go func() {
				defer atomic.StoreInt32(&modelsRefreshing, 0)
				refreshModels()
			}()
		}
		w.Header().Set("Content-Type", "application/json")
		w.Header().Set("X-WB2-Cache", "hit")
		w.Write(filterBlockedModels([]byte(raw)))
		return
	}
	// 无缓存时才同步拉一次
	h := r.Header.Get("Authorization")
	secret := strings.TrimSpace(strings.TrimPrefix(h, "Bearer "))
	k, _ := keyBySecret(secret)
	if k == nil {
		if raw := getSetting("models_cache", ""); raw != "" {
			w.Header().Set("Content-Type", "application/json")
			w.Header().Set("X-WB2-Cache", "hit")
			w.Write(filterBlockedModels([]byte(raw)))
			return
		}
	}
	resp, err := upstreamDo("GET", "/v1/models", nil, false)
	if err != nil {
		writeErr(w, 502, "上游不可达："+err.Error())
		return
	}
	defer resp.Body.Close()
	body, _ := io.ReadAll(resp.Body)

	// 上游网关 key 失效时不要 401 直出：退回缓存，至少模型列表可用
	if resp.StatusCode == 401 {
		if raw := getSetting("models_cache", ""); raw != "" {
			w.Header().Set("Content-Type", "application/json")
			w.Header().Set("X-WB2-Cache", "stale")
			w.Write(filterBlockedModels([]byte(raw)))
			return
		}
	}

	// 缓存一次，用于模型白名单
	if resp.StatusCode == 200 && len(body) > 100 {
		body = mergeCNCatalog(body)
		setSetting("models_cache", string(body))
		setSetting("models_cached_at", nowISO())
		body = filterBlockedModels(body)
	}
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(resp.StatusCode)
	w.Write(body)
}

func allowedModels() map[string]bool {
	var out struct {
		Data []struct {
			ID string `json:"id"`
		} `json:"data"`
	}
	raw := getSetting("models_cache", "")
	if raw != "" {
		_ = json.Unmarshal([]byte(raw), &out)
	}
	set := map[string]bool{}
	for _, m := range out.Data {
		set[m.ID] = true
	}
	// 兜底：缓存为空时放行（不因为缓存问题阻断服务）
	if len(set) == 0 {
		return nil
	}
	return set
}

// ---------------- /v1/chat/completions ----------------
func handleChatCompletions(w http.ResponseWriter, r *http.Request) {
	if cors(w, r) {
		return
	}
	if r.Method != http.MethodPost {
		writeErr(w, 405, "只支持 POST")
		return
	}

	// 1) 鉴权
	h := r.Header.Get("Authorization")
	secret := strings.TrimSpace(strings.TrimPrefix(h, "Bearer "))
	if secret == "" || !strings.HasPrefix(secret, "sk-wb-") {
		writeErr(w, 401, "缺少或无效的 API Key（格式 sk-wb-…）")
		return
	}
	key, u := keyBySecret(secret)
	if key == nil {
		writeErr(w, 401, "API Key 无效或已吊销")
		return
	}
	chatCore(w, r, u, key)
}

// 站内演练场：用登录令牌调用，不需要用户自带 key（同样计费）
func handlePlaygroundChat(w http.ResponseWriter, r *http.Request) {
	if cors(w, r) {
		return
	}
	if r.Method != http.MethodPost {
		writeErr(w, 405, "只支持 POST")
		return
	}
	u := currentUser(r)
	virtual := &APIKey{ID: 0, UserID: u.ID, Name: "playground"}
	chatCore(w, r, u, virtual)
}

func chatCore(w http.ResponseWriter, r *http.Request, u *User, key *APIKey) {
	start := time.Now()
	if u.Banned {
		writeErr(w, 403, "账号已停用："+u.BanReason)
		return
	}

	// 2) 读请求体
	raw, err := io.ReadAll(io.LimitReader(r.Body, 32<<20))
	if err != nil {
		writeErr(w, 400, "读取请求体失败")
		return
	}
	var payload map[string]interface{}
	if err := json.Unmarshal(raw, &payload); err != nil {
		writeErr(w, 400, "请求体不是合法 JSON")
		return
	}
	model, _ := payload["model"].(string)
	if model == "" {
		writeErr(w, 400, "缺少 model 字段")
		return
	}
	if allow := allowedModels(); allow != nil && !allow[model] {
		writeErr(w, 404, "模型不存在或当前不可用："+model)
		return
	}
	// 池内账号类型不支持的模型：直接给人话，别把上游 11158 甩出去
	if isModelBlocked(model) {
		writeJSON(w, 400, map[string]interface{}{
			"error": map[string]interface{}{
				"message":      "模型 " + model + " 当前池内账号类型不支持（上游按账号类型放行），请改用 glm-5.3-flash、deepseek-v4.1-flash 等；等有支持该模型的账号接入后会自动恢复。",
				"type":         "invalid_request_error",
				"code":         400,
				"gateway_hint": "model_not_supported_by_pool",
			},
		})
		return
	}

	// 按档位限制模型：LV1/游客 走 aio·glm 通道；LV2 起全量
	if pm := planModels(u.Plan); pm != nil && !pm[model] {
		writeErr(w, 403, "当前档位不含该模型："+model+"。LV1／游客可用 "+planModelNames(u.Plan)+"，升级 LV2 解锁全部文本模型")
		return
	}
	stream, _ := payload["stream"].(bool)
	if stream {
		// 让上游回 usage，我们才能精确计费
		payload["stream_options"] = map[string]interface{}{"include_usage": true}
		b, _ := json.Marshal(payload)
		raw = b
	}

	// 3) 额度 / 限速
	leftDaily, leftBonus, leftLottery, quota := quotaState(u)
	if leftDaily <= 0 && leftBonus <= 0 && leftLottery <= 0 {
		writeJSON(w, 402, map[string]interface{}{
			"error": map[string]interface{}{
				"message": "今日额度、抽奖赠点与加油包都已用完。第二天 0 点额度自动恢复；也可以赞助档位或兑换卡密立即加量。",
				"code":    402,
				"hint":    "quota_exhausted",
				"quota":   quota,
				"used":    u.PointsToday,
			},
		})
		return
	}
	// 软上限：日额度用超后限速 1/3；超过 1.5 倍且无加油包则拒绝
	conc, rpm, _ := limitsOf(u)
	soft := ""
	if leftDaily <= 0 && (leftBonus > 0 || leftLottery > 0) {
		soft = "bonus"
	} else if quota > 0 && u.PointsToday > quota {
		over := u.PointsToday / quota
		if over >= 1.5 && leftBonus <= 0 && leftLottery <= 0 {
			writeErr(w, 402, "已超出日额度 150%，为保护池子暂停到明日 0 点")
			return
		}
		soft = "throttle"
		if rpm > 3 {
			rpm = rpm / 3
		}
	}
	_ = soft

	release, msg := getBucket(u.ID).tryAcquire(conc, rpm)
	if release == nil {
		writeErr(w, 429, msg)
		return
	}
	defer release()

	// 4) 转发
	resp, err := forwardChat(model, raw, stream)
	if err != nil {
		logRow(u, key, model, 0, 0, 0, 502, "上游不可达："+err.Error(), start, 0, stream, r)
		writeErr(w, 502, "上游不可达，请稍后重试")
		return
	}
	defer resp.Body.Close()

	if resp.StatusCode >= 400 {
		body, _ := io.ReadAll(io.LimitReader(resp.Body, 8192))
		writeLogFail(u, key, model, resp.StatusCode, body, start, stream, r)
		// 上游网关自身鉴权失效（API key 被换/停用）→ 503 人话，别让调用方以为是自己的 key 坏了
		if resp.StatusCode == 401 && strings.Contains(string(body), "invalid api key") {
			writeJSON(w, 503, map[string]interface{}{
				"error": map[string]interface{}{
					"message":      "上游网关鉴权异常（管理员正在处理），请稍后重试。",
					"type":         "server_error",
					"code":         503,
					"gateway_hint": "upstream_auth_broken",
				},
			})
			return
		}
		// 上游按账号类型拒绝该模型（11158）：把这个模型标记为不可用 30 分钟并回人话
		if modelNotAllowed(body) {
			markModelBlocked(model, 30)
			writeJSON(w, 400, map[string]interface{}{
				"error": map[string]interface{}{
					"message":      "模型 " + model + " 当前池内账号类型不支持（上游账号为免费账号，不支持该模型），请改用 glm-5.3-flash、deepseek-v4.1-flash 等。",
					"type":         "invalid_request_error",
					"code":         400,
					"gateway_hint": "model_not_supported_by_pool",
				},
			})
			return
		}
		// 账号被上游限制（11140 异常行为）：翻译成人话，别把英文堆栈甩给调用方
		if msg, hint, okr := restrictedMessage(body); okr {
			writeJSON(w, 503, map[string]interface{}{
				"error": map[string]interface{}{
					"message":      msg,
					"type":         "server_error",
					"code":         503,
					"gateway_hint": hint,
				},
			})
			return
		}
		// 池子没有可用账号时，给调用方一句人话，而不是把上游的英文堆栈直接甩回去
		if msg, ok := poolDownMessage(body); ok {
			writeJSON(w, resp.StatusCode, map[string]interface{}{
				"error": map[string]interface{}{
					"message":      msg,
					"type":         "server_error",
					"code":         resp.StatusCode,
					"gateway_hint": "pool_recovering",
				},
			})
			return
		}
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(resp.StatusCode)
		w.Write(body)
		return
	}

	// 5) 流式 / 非流式
	if stream {
		proxyStream(w, resp, u, key, model, start, r)
		return
	}
	proxyOnce(w, resp, u, key, model, start, r)
}

// 非流式：读全量、解析 usage、计费、原样返回
func proxyOnce(w http.ResponseWriter, resp *http.Response, u *User, key *APIKey, model string, start time.Time, r *http.Request) {
	body, err := io.ReadAll(resp.Body)
	if err != nil {
		writeErr(w, 502, "读取上游响应失败")
		return
	}
	var out struct {
		Usage struct {
			PromptTokens     int     `json:"prompt_tokens"`
			CompletionTokens int     `json:"completion_tokens"`
			Credit           float64 `json:"credit"`
		} `json:"usage"`
	}
	_ = json.Unmarshal(body, &out)
	pt, ct := out.Usage.PromptTokens, out.Usage.CompletionTokens
	if pt == 0 && ct == 0 {
		pt, ct = estimateTokens(body)
	}
	pts := pointsFor(model, pt, ct)
	charge(u, pts)
	logRow(u, key, model, pt, ct, pts, resp.StatusCode, "", start, 0, false, r)

	w.Header().Set("Content-Type", "application/json")
	w.Header().Set("X-WB2-Points", ftoa(pts))
	w.WriteHeader(resp.StatusCode)
	w.Write(body)
}

// 流式：逐行透传，同时解析 usage / 首个 token 时间
func proxyStream(w http.ResponseWriter, resp *http.Response, u *User, key *APIKey, model string, start time.Time, r *http.Request) {
	flusher, okf := w.(http.Flusher)
	h := w.Header()
	h.Set("Content-Type", "text/event-stream; charset=utf-8")
	h.Set("Cache-Control", "no-cache")
	h.Set("Connection", "keep-alive")
	h.Set("X-Accel-Buffering", "no")
	w.WriteHeader(200)

	br := bufio.NewReaderSize(resp.Body, 64<<10)
	var pt, ct int
	var credit float64
	var ttft int
	var errText string
	buf := &bytes.Buffer{}

	for {
		line, err := br.ReadBytes('\n')
		if len(line) > 0 {
			if _, werr := w.Write(line); werr != nil {
				break
			}
			s := strings.TrimSpace(string(line))
			if strings.HasPrefix(s, "data:") && !strings.Contains(s, "[DONE]") {
				js := strings.TrimSpace(strings.TrimPrefix(s, "data:"))
				if strings.Contains(js, "\"usage\"") && !strings.Contains(js, "\"usage\": null") {
					var chunk struct {
						Usage struct {
							PromptTokens     int     `json:"prompt_tokens"`
							CompletionTokens int     `json:"completion_tokens"`
							Credit           float64 `json:"credit"`
						} `json:"usage"`
						Error *struct {
							Message string `json:"message"`
						} `json:"error"`
					}
					if json.Unmarshal([]byte(js), &chunk) == nil {
						if chunk.Usage.PromptTokens > 0 || chunk.Usage.CompletionTokens > 0 {
							pt = chunk.Usage.PromptTokens
							ct = chunk.Usage.CompletionTokens
							credit = chunk.Usage.Credit
						}
						if chunk.Error != nil {
							errText = chunk.Error.Message
						}
					}
				}
				if ttft == 0 && strings.Contains(js, "\"content\"") && !strings.Contains(js, "\"content\":\"\"") {
					ttft = int(time.Since(start).Milliseconds())
				}
			}
			if okf {
				flusher.Flush()
			}
		}
		if err != nil {
			break
		}
	}
	if pt == 0 && ct == 0 {
		pt, ct = estimateTokens(buf.Bytes())
	}
	pts := pointsFor(model, pt, ct)
	charge(u, pts)
	logRow(u, key, model, pt, ct, pts, 200, errText, start, ttft, true, r)
	_ = credit
}

// 兜底 token 估算（按 4 字符 ≈ 1 token）
func estimateTokens(b []byte) (int, int) {
	s := string(b)
	if i := strings.Index(s, "\"content\":\""); i >= 0 {
		s = s[i:]
	}
	n := len([]rune(s)) / 4
	if n < 1 {
		n = 1
	}
	return n, n
}

func writeLogFail(u *User, key *APIKey, model string, status int, body []byte, start time.Time, stream bool, r *http.Request) {
	msg := strings.TrimSpace(string(body))
	if len(msg) > 240 {
		msg = msg[:240]
	}
	logRow(u, key, model, 0, 0, 0, status, msg, start, 0, stream, r)
}

func logRow(u *User, key *APIKey, model string, pt, ct int, pts float64, status int, errMsg string, start time.Time, ttft int, stream bool, r *http.Request) {
	since := getSetting("log_retain_days", "")
	_ = since
	db.Create(&LogRow{
		UserID: u.ID, KeyID: key.ID, Model: model,
		PromptTok: pt, ComplTok: ct, Points: pts, Status: status, Error: errMsg,
		Ms: int(time.Since(start).Milliseconds()), TTFT: ttft, Stream: stream,
		IP: clientIP(r), CreatedAt: time.Now(),
	})
	db.Model(&APIKey{}).Where("id = ?", key.ID).Updates(map[string]interface{}{
		"calls":     key.Calls + 1,
		"last_used": time.Now(),
	})
}

func clientIP(r *http.Request) string {
	// nginx 用 $remote_addr 覆写 X-Real-IP，客户端无法伪造；
	// CF-Connecting-IP 仅在走 Cloudflare 时才有意义（灰云直连时可被伪造，故排在后面）
	if v := r.Header.Get("X-Real-IP"); v != "" {
		return v
	}
	if v := r.Header.Get("CF-Connecting-IP"); v != "" {
		return v
	}
	if i := strings.LastIndex(r.RemoteAddr, ":"); i > 0 {
		return r.RemoteAddr[:i]
	}
	return r.RemoteAddr
}

// poolDownMessage 识别「上游账号池无可用账号」，翻译成中文提示，并尽量给出预计恢复时间
func poolDownMessage(body []byte) (string, bool) {
	t := string(body)
	if !strings.Contains(t, "no usable account") && !strings.Contains(t, "no healthy account") {
		return "", false
	}
	// 上游会带「剩余 1h45m」这类冷却时间，取最大值作为预计恢复时间
	wait := ""
	rest := t
	for {
		i := strings.Index(rest, "剩余 ")
		if i < 0 {
			break
		}
		rest = rest[i+len("剩余 "):]
		j := strings.IndexAny(rest, "\uff0c\u3001,; ")
		if j < 0 {
			j = len(rest)
		}
		if d := strings.TrimSpace(rest[:j]); d != "" && len(d) < 12 {
			wait = formatWait(d)
		}
	}
	if wait != "" {
		return "上游账号池正在恢复中（预计 " + wait + "后恢复），请稍后再试。", true
	}
	return "上游账号池正在恢复中，请稍后再试。", true
}

// formatWait 把 "1h45m" / "45m" / "1h" 之类转成中文
func formatWait(d string) string {
	h, m := 0, 0
	if i := strings.Index(d, "h"); i >= 0 {
		h = atoiSafe(d[:i])
		d = d[i+1:]
	}
	if i := strings.Index(d, "m"); i >= 0 {
		m = atoiSafe(d[:i])
	}
	if h == 0 && m == 0 {
		return ""
	}
	if h == 0 {
		return itoaSafe(m) + " 分钟"
	}
	if m == 0 {
		return itoaSafe(h) + " 小时"
	}
	return itoaSafe(h) + " 小时 " + itoaSafe(m) + " 分钟"
}

func atoiSafe(s string) int {
	n := 0
	for _, c := range strings.TrimSpace(s) {
		if c < '0' || c > '9' {
			return 0
		}
		n = n*10 + int(c-'0')
	}
	return n
}

func itoaSafe(n int) string {
	if n == 0 {
		return "0"
	}
	var b []byte
	for n > 0 {
		b = append([]byte{byte('0' + n%10)}, b...)
		n /= 10
	}
	return string(b)
}

// ---------------- 档位可用的模型 ----------------
// LV1（及游客）：只放 glm-5.3-flash 与 deepseek-v4.1-flash（2026-10-24 加入 DS，性价比档）
// LV2 / LV3：全部文本模型（getSetting plan_models 可覆盖）
func planModels(plan string) map[string]bool {
	code := strings.ToLower(strings.TrimSpace(plan))
	if code == "" {
		code = "guest"
	}
	// 后台可覆盖：settings.plan_models = {"lv1":["a","b"]}，写 "*" 表示不限
	if raw := strings.TrimSpace(getSetting("plan_models", "")); raw != "" {
		var cfg map[string]interface{}
		if json.Unmarshal([]byte(raw), &cfg) == nil {
			if v, ok := cfg[code]; ok {
				if s, isStr := v.(string); isStr && s == "*" {
					return nil
				}
				if arr, isArr := v.([]interface{}); isArr {
					out := map[string]bool{}
					for _, it := range arr {
						if m, okm := it.(string); okm && m != "" {
							out[m] = true
						}
					}
					if len(out) > 0 {
						return out
					}
				}
			}
		}
	}
	switch code {
	case "guest", "lv1":
		return map[string]bool{"glm-5.3-flash": true, "deepseek-v4.1-flash": true}
	}
	return nil
}

func planModelNames(plan string) string {
	pm := planModels(plan)
	if pm == nil {
		return "全部模型"
	}
	keys := make([]string, 0, len(pm))
	for k := range pm {
		keys = append(keys, k)
	}
	sortStrings(keys)
	return strings.Join(keys, " / ")
}

// restrictedMessage 识别上游 11140「账号存在异常行为，已被限制使用」
func restrictedMessage(body []byte) (string, string, bool) {
	t := string(body)
	if strings.Contains(t, "11140") || strings.Contains(t, "账号存在异常行为") {
		return "上游账号被判定异常行为并限制使用，网关正在切换其他账号，请稍后重试。", "account_restricted", true
	}
	return "", "", false
}

// ---------------- 池内不支持的模型（上游 11158） ----------------
// 上游按「账号类型」放行模型：个人免费号只能用部分模型。
// 一旦某个模型报 11158，就把它拉黑 30 分钟：模型列表里隐藏、调用直接回人话。

func blockedModels() map[string]int64 {
	out := map[string]int64{}
	raw := getSetting("model_blocked", "")
	if raw == "" {
		return out
	}
	var m map[string]int64
	if json.Unmarshal([]byte(raw), &m) != nil {
		return out
	}
	now := time.Now().Unix()
	for k, v := range m {
		if v > now {
			out[k] = v
		}
	}
	return out
}

// disabledModels 永久下线的模型（settings.models_disabled，逗号/空格/换行分隔）。
// 上游账号类型不支持（11158）或者上游已下架的模型放这里：不出现在模型列表里，
// 调用直接回人话，不会打到上游、也不扣点。
func disabledModels() map[string]bool {
	out := map[string]bool{}
	raw := getSetting("models_disabled", "")
	if raw == "" {
		return out
	}
	for _, f := range strings.FieldsFunc(raw, func(r rune) bool {
		return r == ',' || r == ' ' || r == '\n' || r == '\t' || r == ';'
	}) {
		if f != "" {
			out[f] = true
		}
	}
	return out
}

func isModelBlocked(model string) bool {
	if _, ok := blockedModels()[model]; ok {
		return true
	}
	_, off := disabledModels()[model]
	return off
}

// publicModelNames 模型列表（已剔除临时黑名单与永久下线）
func publicModelNames() []string {
	blocked := blockedModels()
	off := disabledModels()
	names := make([]string, 0, 32)
	for k := range allowedModels() {
		if _, bad := blocked[k]; bad {
			continue
		}
		if off[k] {
			continue
		}
		names = append(names, k)
	}
	sortStrings(names)
	return names
}

func markModelBlocked(model string, minutes int) {
	if model == "" {
		return
	}
	m := blockedModels()
	m[model] = time.Now().Unix() + int64(minutes*60)
	b, _ := json.Marshal(m)
	setSetting("model_blocked", string(b))
}

func modelNotAllowed(body []byte) bool {
	t := string(body)
	return strings.Contains(t, "11158") || strings.Contains(t, "not allowed for user type")
}

// filterBlockedModels 从 /v1/models 的 JSON 里剔除被拉黑的模型
func filterBlockedModels(body []byte) []byte {
	blk := blockedModels()
	off := disabledModels()
	if len(blk) == 0 && len(off) == 0 {
		return body
	}
	var root map[string]interface{}
	if json.Unmarshal(body, &root) != nil {
		return body
	}
	data, ok := root["data"].([]interface{})
	if !ok {
		return body
	}
	kept := make([]interface{}, 0, len(data))
	for _, it := range data {
		m, okm := it.(map[string]interface{})
		if !okm {
			continue
		}
		id, _ := m["id"].(string)
		if _, bad := blk[id]; bad {
			continue
		}
		if off[id] {
			continue
		}
		kept = append(kept, it)
	}
	root["data"] = kept
	out, err := json.Marshal(root)
	if err != nil {
		return body
	}
	return out
}
