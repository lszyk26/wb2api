package main

// config.go — 环境变量与全局配置
//
// 全部配置走 env 文件（默认 ./data/env，可用 WB2_ENV_FILE 覆盖），不落到代码里。

import (
	"bufio"
	"crypto/rand"
	"encoding/hex"
	"log"
	"os"
	"strconv"
	"strings"
)

type Config struct {
	Addr      string
	StaticDir string // 前端产物目录，Go 直接托管（留空则只提供 API）
	DBPath    string
	JWTSecret string
	EncKey    []byte
	AdminUser string
	AdminPass string

	// 上游 workbuddy 网关
	UpstreamURL string
	UpstreamKey string
	UsagePath   string // 上游 usage.jsonl（只读，用于真实用量对账）
	HubDir      string // 上游 hub 数据目录（accounts/ 由本站写入）
	PublicURL   string // 本站对外地址：邀请链接 / 支付回调 / 邮件页脚

	// GitHub OAuth 登录（用户侧一键登录 / 注册）
	GitHubID       string
	GitHubSecret   string
	GitHubRedirect string

	// 运营参数
	PointPerYuan float64
	SignupPoints float64
	SignupDays   int

	// 发信通道（Resend 优先，SMTP 兜底）
	ResendKey  string
	ResendFrom string
	SMTPHost   string
	SMTPPort   string
	SMTPUser   string
	SMTPPass   string
	SMTPFrom   string
}

var cfg Config

// 极简 .env 读取（KEY=VALUE，支持 # 注释与引号）
func loadEnvFile(path string) {
	f, err := os.Open(path)
	if err != nil {
		return
	}
	defer f.Close()
	sc := bufio.NewScanner(f)
	for sc.Scan() {
		line := strings.TrimSpace(sc.Text())
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		eq := strings.Index(line, "=")
		if eq <= 0 {
			continue
		}
		k := strings.TrimSpace(line[:eq])
		v := strings.TrimSpace(line[eq+1:])
		v = strings.Trim(v, `"'`)
		if os.Getenv(k) == "" {
			os.Setenv(k, v)
		}
	}
}

func env(k, def string) string {
	if v := os.Getenv(k); v != "" {
		return v
	}
	return def
}

func envF(k string, def float64) float64 {
	if v := os.Getenv(k); v != "" {
		if f, err := strconv.ParseFloat(v, 64); err == nil {
			return f
		}
	}
	return def
}

func envI(k string, def int) int {
	if v := os.Getenv(k); v != "" {
		if n, err := strconv.Atoi(v); err == nil {
			return n
		}
	}
	return def
}

func loadConfig() {
	dataDir := env("WB2_DATA_DIR", "./data")
	loadEnvFile(env("WB2_ENV_FILE", dataDir+"/env"))
	publicURL := strings.TrimRight(env("WB2_PUBLIC_URL", "http://localhost:8078"), "/")
	hubDir := env("WB2_HUB_DIR", dataDir+"/hub")
	cfg = Config{
		Addr:           env("WB2_LISTEN", "127.0.0.1:8078"),
		StaticDir:      env("WB2_STATIC_DIR", "./dist"),
		DBPath:         env("WB2_DB", dataDir+"/wb2.db"),
		JWTSecret:      env("WB2_JWT_SECRET", ""),
		AdminUser:      env("WB2_ADMIN_USER", "admin"),
		AdminPass:      env("WB2_ADMIN_PASS", ""),
		UpstreamURL:    env("WB2_UPSTREAM", "http://127.0.0.1:8788"),
		UpstreamKey:    env("WB2_UPSTREAM_KEY", ""),
		UsagePath:      env("WB2_USAGE_PATH", hubDir+"/usage/usage.jsonl"),
		HubDir:         hubDir,
		PublicURL:      publicURL,
		GitHubID:       env("WB2_GH_ID", ""),
		GitHubSecret:   env("WB2_GH_SECRET", ""),
		GitHubRedirect: env("WB2_GH_REDIRECT", publicURL+"/api/auth/github/callback"),
		PointPerYuan:   envF("WB2_POINT_PER_YUAN", 1000),
		SignupPoints:   envF("WB2_SIGNUP_POINTS", 3000),
		SignupDays:     envI("WB2_SIGNUP_DAYS", 7),

		ResendKey:  env("WB2_RESEND_KEY", ""),
		ResendFrom: env("WB2_RESEND_FROM", "WorkBuddy2 API <noreply@example.com>"),
		SMTPHost:   env("WB2_SMTP_HOST", ""),
		SMTPPort:   env("WB2_SMTP_PORT", "587"),
		SMTPUser:   env("WB2_SMTP_USER", ""),
		SMTPPass:   env("WB2_SMTP_PASS", ""),
		SMTPFrom:   env("WB2_SMTP_FROM", ""),
	}

	if cfg.JWTSecret == "" {
		b := make([]byte, 32)
		_, _ = rand.Read(b)
		cfg.JWTSecret = hex.EncodeToString(b)
		log.Printf("[config] WB2_JWT_SECRET 未设置，本次使用随机密钥（重启后所有登录失效）")
	}
	initEncryption(env("WB2_ENC_KEY", cfg.JWTSecret))
}
