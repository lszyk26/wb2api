package main

// upstream.go — 上游联动：写入账号文件 / 导入真实用量

import (
	"bufio"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"log"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"time"
)

// 上游 accounts 目录（workbuddy2api-hub 容器挂载的自定义目录，由 WB2_HUB_DIR 指定）
func upstreamAccountsDir() string { return filepath.Join(cfg.HubDir, "accounts") }

// 把用户提交的凭据写成上游可读的账号文件
func writeUpstreamAccount(credJSON, uid string) (string, error) {
	if err := os.MkdirAll(upstreamAccountsDir(), 0o755); err != nil {
		return "", err
	}
	var cred map[string]interface{}
	if err := json.Unmarshal([]byte(credJSON), &cred); err != nil {
		return "", err
	}
	if v, _ := cred["uid"].(string); v == "" {
		cred["uid"] = uid
	}
	// 必要字段兜底
	if _, ok := cred["domain"]; !ok {
		cred["domain"] = "www.workbuddy.ai"
	}
	if _, ok := cred["realm"]; !ok {
		cred["realm"] = "intl"
	}
	if _, ok := cred["platform"]; !ok {
		cred["platform"] = "CLI"
	}
	if _, ok := cred["product"]; !ok {
		cred["product"] = "workbuddy"
	}
	name := uid
	if name == "" {
		name = fmt.Sprintf("user-%d", time.Now().Unix())
	}
	out := filepath.Join(upstreamAccountsDir(), name+".json")
	b, _ := json.MarshalIndent(cred, "", "  ")
	tmp := out + ".tmp"
	if err := os.WriteFile(tmp, b, 0o644); err != nil {
		return "", err
	}
	if err := os.Rename(tmp, out); err != nil {
		return "", err
	}
	return out, nil
}

// ---------------- 上游真实用量导入 ----------------
// usage.jsonl 每行一条，按 at+model+msgs_sha 去重导入，用于真实成本对账

func importUpstreamUsage() {
	f, err := os.Open(cfg.UsagePath)
	if err != nil {
		return
	}
	defer f.Close()

	offset := int64(0)
	if v := getSetting("usage_offset", "0"); v != "" {
		offset, _ = strconv.ParseInt(v, 10, 64)
	}
	st, err := f.Stat()
	if err != nil {
		return
	}
	if st.Size() < offset {
		offset = 0 // 文件被轮转
	}
	if _, err := f.Seek(offset, 0); err != nil {
		return
	}

	sc := bufio.NewScanner(f)
	sc.Buffer(make([]byte, 0, 64<<10), 4<<20)
	n := 0
	for sc.Scan() {
		line := strings.TrimSpace(sc.Text())
		offset += int64(len(line)) + 1
		if line == "" {
			continue
		}
		var r struct {
			At        float64 `json:"at"`
			ISO       string  `json:"iso"`
			Model     string  `json:"model"`
			Account   string  `json:"account"`
			Realm     string  `json:"realm"`
			Prompt    int     `json:"prompt_tokens"`
			Completion int    `json:"completion_tokens"`
			Credit    float64 `json:"credit"`
			Elapsed   int     `json:"elapsed_ms"`
			TTFT      int     `json:"ttft_ms"`
			Outcome   string  `json:"outcome"`
			MsgsSha   string  `json:"msgs_sha"`
		}
		if json.Unmarshal([]byte(line), &r) != nil {
			continue
		}
		sum := sha256.Sum256([]byte(fmt.Sprintf("%.4f|%s|%s|%d", r.At, r.Model, r.MsgsSha, r.Prompt)))
		fp := hex.EncodeToString(sum[:16])
		row := UpstreamUsage{
			At: time.Unix(int64(r.At), 0), Model: r.Model, Account: r.Account, Realm: r.Realm,
			PromptTok: r.Prompt, ComplTok: r.Completion, Credit: r.Credit,
			Elapsed: r.Elapsed, TTFT: r.TTFT, Outcome: r.Outcome, Fingerprint: fp,
		}
		if err := db.Create(&row).Error; err == nil {
			n++
		}
	}
	setSetting("usage_offset", strconv.FormatInt(offset, 10))
	if n > 0 {
		log.Printf("[usage] 导入上游记录 %d 条", n)
	}
}

func upstreamUsageSummary() map[string]interface{} {
	since := time.Now().AddDate(0, 0, -7)
	type mrow struct {
		Model  string  `json:"model"`
		Calls  int64   `json:"calls"`
		Credit float64 `json:"credit"`
		Tokens int64   `json:"tokens"`
	}
	var byModel []mrow
	db.Model(&UpstreamUsage{}).Where("at >= ?", since).
		Select("model, count(*) as calls, coalesce(sum(credit),0) as credit, coalesce(sum(prompt_tokens+completion_tokens),0) as tokens").
		Group("model").Order("credit desc").Limit(20).Scan(&byModel)

	type drow struct {
		Day    string  `json:"day"`
		Calls  int64   `json:"calls"`
		Credit float64 `json:"credit"`
	}
	var byDay []drow
	db.Model(&UpstreamUsage{}).Where("at >= ?", since).
		Select("strftime('%m-%d', at) as day, count(*) as calls, coalesce(sum(credit),0) as credit").
		Group("day").Order("day").Scan(&byDay)

	var total struct {
		Calls  int64
		Credit float64
	}
	db.Model(&UpstreamUsage{}).Where("at >= ?", since).
		Select("count(*) as calls, coalesce(sum(credit),0) as credit").Scan(&total)

	return map[string]interface{}{
		"by_model": byModel, "by_day": byDay,
		"total_calls": total.Calls, "total_credit": round2(total.Credit),
		"range_days": 7,
	}
}

// 上游账号目录一览（不读内容，只列文件）
func listUpstreamAccounts() []map[string]interface{} {
	entries, err := os.ReadDir(upstreamAccountsDir())
	if err != nil {
		return nil
	}
	out := make([]map[string]interface{}, 0, len(entries))
	names := make([]string, 0, len(entries))
	for _, e := range entries {
		if e.IsDir() || !strings.HasSuffix(e.Name(), ".json") || e.Name() == "settings.json" || e.Name() == "model.json" {
			continue
		}
		names = append(names, e.Name())
	}
	sort.Strings(names)
	for _, n := range names {
		full := filepath.Join(upstreamAccountsDir(), n)
		var info map[string]interface{}
		if b, err := os.ReadFile(full); err == nil {
			var m map[string]interface{}
			if json.Unmarshal(b, &m) == nil {
				info = map[string]interface{}{
					"uid": m["uid"], "nickname": m["nickname"],
					"realm": m["realm"], "domain": m["domain"],
				}
			}
		}
		if info == nil {
			info = map[string]interface{}{}
		}
		st, _ := os.Stat(full)
		info["file"] = n
		if st != nil {
			info["size"] = st.Size()
			info["mtime"] = st.ModTime().Format(time.RFC3339)
		}
		out = append(out, info)
	}
	return out
}

