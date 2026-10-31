package main

// main.go — 启动入口

import (
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"os"
	"path/filepath"
	"os/signal"
	"syscall"
	"time"
)

func main() {
	log.SetFlags(log.LstdFlags)
	loadConfig()

	if err := os.MkdirAll(filepath.Dir(cfg.DBPath), 0o755); err != nil {
		log.Fatalf("[main] mkdir data: %v", err)
	}
	openDB(cfg.DBPath)
	bootstrapAdmin()
	seedSettings()
	seedLottery()
	startRealmHealthLoop() // 每分钟刷新 cn / intl 两侧可用号数，供动态分流使用

	// 后台任务：导入上游用量 / 刷新模型缓存 / 清理过期日志
	go func() {
		time.Sleep(2 * time.Second)
		refreshModels()
		for {
			importUpstreamUsage()
			time.Sleep(60 * time.Second)
		}
	}()
	go func() {
		for {
			time.Sleep(10 * time.Minute)
			refreshModels()
			pruneOldLogs()
		}
	}()
	// 账号健康采样（每 10 分钟）
	go func() {
		time.Sleep(30 * time.Second)
		for {
			sampleAccountHealth()
			time.Sleep(10 * time.Minute)
		}
	}()
	// 月度贡献结算
	go settlementLoop()
	// 清理
	go func() {
		for {
			time.Sleep(6 * time.Hour)
			pruneHealth()
		}
	}()

	m := mux()
	handler := withRecover(withLog(m))

	srv := &http.Server{
		Addr:              cfg.Addr,
		Handler:           handler,
		ReadHeaderTimeout: 15 * time.Second,
		// 流式响应不能设 WriteTimeout
	}
	log.Printf("[main] wb2api-v2 server on %s (upstream %s)", cfg.Addr, cfg.UpstreamURL)

	go func() {
		if err := srv.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			log.Fatalf("[main] listen: %v", err)
		}
	}()

	quit := make(chan os.Signal, 1)
	signal.Notify(quit, syscall.SIGINT, syscall.SIGTERM)
	<-quit
	log.Println("[main] shutting down")
}

func bootstrapAdmin() {
	var n int64
	db.Model(&User{}).Where("is_admin = ?", true).Count(&n)
	if n > 0 {
		return
	}
	pass := cfg.AdminPass
	if pass == "" {
		pass = randHex(8)
		log.Printf("[admin] 未设置 WB2_ADMIN_PASS，本次随机密码：%s（请写入 data/env）", pass)
	}
	u := User{
		Username: cfg.AdminUser, PassHash: hashPass(pass), IsAdmin: true,
		InviteCode: genInviteCode(), DailyPoints: 0,
	}
	if err := db.Create(&u).Error; err != nil {
		log.Printf("[admin] 创建失败: %v", err)
		return
	}
	log.Printf("[admin] 已创建管理员 %s", cfg.AdminUser)
}

func seedSettings() {
	defaults := map[string]string{
		"guest_daily":           "2000",
		"student_discount":      "0.7",
		"stack_multiplier":      "1.5",
		"cn_split":              "70",
		"cors_origin":           cfg.PublicURL,
		"announcement":          "",
		"email_verify_required": "1",
	}
	for k, v := range defaults {
		if getSetting(k, "") == "" {
			setSetting(k, v)
		}
	}
}

func refreshModels() {
	resp, err := upstreamDo("GET", "/v1/models", nil, false)
	if err != nil {
		return
	}
	defer resp.Body.Close()
	b := make([]byte, 0, 32<<10)
	buf := make([]byte, 16<<10)
	for {
		n, err := resp.Body.Read(buf)
		if n > 0 {
			b = append(b, buf[:n]...)
			if len(b) > 512<<10 {
				break
			}
		}
		if err != nil {
			break
		}
	}
	if resp.StatusCode == 200 && len(b) > 50 {
		var probe struct {
			Data []struct{ ID string } `json:"data"`
		}
		if json.Unmarshal(b, &probe) == nil && len(probe.Data) > 0 {
			setSetting("models_cache", string(mergeCNCatalog(b)))
			setSetting("models_cached_at", nowISO())
		}
	}
}

// 清理超出所有档位日志保留期的记录（只保留 90 天）
func pruneOldLogs() {
	cut := time.Now().AddDate(0, 0, -90)
	res := db.Where("created_at < ?", cut).Delete(&LogRow{})
	if res.RowsAffected > 0 {
		log.Printf("[logs] 清理过期日志 %d 条", res.RowsAffected)
	}
	cutU := time.Now().AddDate(0, 0, -30)
	db.Where("at < ?", cutU).Delete(&UpstreamUsage{})
	// 邮箱验证码：过期 24 小时后清理
	db.Where("expires_at < ?", time.Now().Add(-24*time.Hour)).Delete(&EmailCode{})
}

// ---------------- 中间件 ----------------
func withRecover(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		defer func() {
			if e := recover(); e != nil {
				log.Printf("[panic] %s %s: %v", r.Method, r.URL.Path, e)
				writeErr(w, 500, fmt.Sprint("服务内部错误"))
			}
		}()
		next.ServeHTTP(w, r)
	})
}

func withLog(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		start := time.Now()
		w2 := &statusWriter{ResponseWriter: w, status: 200}
		next.ServeHTTP(w2, r)
		if d := time.Since(start); d > 3*time.Second {
			log.Printf("[slow] %s %s %d %s", r.Method, r.URL.Path, w2.status, d)
		}
	})
}

type statusWriter struct {
	http.ResponseWriter
	status int
}

func (s *statusWriter) WriteHeader(c int) {
	s.status = c
	s.ResponseWriter.WriteHeader(c)
}

func (s *statusWriter) Flush() {
	if f, ok := s.ResponseWriter.(http.Flusher); ok {
		f.Flush()
	}
}
