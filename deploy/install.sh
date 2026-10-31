#!/usr/bin/env bash
# 一键部署：装 Docker → 生成 .env（随机密钥）→ 构建 → 起站 → 打印账号
# 用法：bash deploy/install.sh
set -euo pipefail

cd "$(dirname "$0")/.."

say() { printf '\033[1;33m%s\033[0m\n' "$*"; }
die() { printf '\033[1;31m✗ %s\033[0m\n' "$*" >&2; exit 1; }

# ---------- 0. 前置检查 ----------
[ "$(id -u)" = "0" ] || die "请用 root 运行：sudo bash deploy/install.sh"
command -v curl >/dev/null || die "缺少 curl"

# ---------- 1. Docker ----------
if ! command -v docker >/dev/null 2>&1; then
  say "→ 未检测到 Docker，开始安装（官方脚本）"
  curl -fsSL https://get.docker.com | sh
  systemctl enable --now docker
fi
if docker compose version >/dev/null 2>&1; then
  DC="docker compose"
elif command -v docker-compose >/dev/null 2>&1; then
  DC="docker-compose"
else
  say "→ 安装 docker compose 插件"
  apt-get update -qq && apt-get install -y docker-compose-plugin
  DC="docker compose"
fi
say "✓ Docker 就绪（$DC）"

# ---------- 2. .env ----------
rnd() { openssl rand -hex 32 2>/dev/null || head -c 32 /dev/urandom | od -An -tx1 | tr -d ' \n'; }
randpass() { tr -dc 'A-Za-z0-9' </dev/urandom | head -c 16; }

if [ -f .env ]; then
  say "→ 已有 .env，保留不动（要重来就删掉它）"
else
  PUBLIC_URL="http://$(curl -fsS --max-time 5 https://api.ipify.org 2>/dev/null || echo 127.0.0.1):8078"
  ADMIN_PASS="$(randpass)"
  cp .env.example .env
  sed -i "s|^WB2_PUBLIC_URL=.*|WB2_PUBLIC_URL=${PUBLIC_URL}|" .env
  sed -i "s|^WB2_ADMIN_PASS=.*|WB2_ADMIN_PASS=${ADMIN_PASS}|" .env
  sed -i "s|^WB2_JWT_SECRET=.*|WB2_JWT_SECRET=$(rnd)|" .env
  sed -i "s|^WB2_ENC_KEY=.*|WB2_ENC_KEY=$(rnd)|" .env
  chmod 600 .env
  say "✓ 已生成 .env（访问地址 ${PUBLIC_URL}）"
fi

mkdir -p data hub

# ---------- 3. 起站 ----------
say "→ 构建并启动（首次编译 2~5 分钟）"
$DC up -d --build

# ---------- 4. 等健康检查 ----------
say "→ 等待服务就绪"
for i in $(seq 1 60); do
  if curl -fsS --max-time 2 http://127.0.0.1:8078/api/health >/dev/null 2>&1; then
    READY=1; break
  fi
  sleep 3
done
[ "${READY:-0}" = "1" ] || die "60 秒内没起来，看日志：$DC logs -f wb2"

ADMIN_PASS_SHOW="$(grep '^WB2_ADMIN_PASS=' .env | cut -d= -f2-)"
PUBLIC_SHOW="$(grep '^WB2_PUBLIC_URL=' .env | cut -d= -f2-)"
cat <<EOF

────────────────────────────────────────────
 ✓ 起来了

 站点      ${PUBLIC_SHOW}
 管理台    登录后侧边栏「管理台」
 管理员    admin
 密码      ${ADMIN_PASS_SHOW}

 下一步（不做也没有功能问题，只是接口会 502）：
   1. 编辑 .env 填 WB2_UPSTREAM / WB2_UPSTREAM_KEY（账号池网关）
   2. 需要邮箱验证码就填 WB2_RESEND_KEY，不需要就跳过（自动放行）
   3. 需要支付就填 WB2_PAY_*，不填则走「手动转账 + 管理台确认」
   改完重启：$DC up -d

 日志：$DC logs -f wb2
────────────────────────────────────────────
EOF
