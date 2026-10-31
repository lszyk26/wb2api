#!/usr/bin/env bash
# 一键部署：装 Docker → 生成 .env（随机密钥）→ 起 hub + 本站 → 打印账号
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
  if command -v apt-get >/dev/null 2>&1; then
    apt-get update -qq && apt-get install -y docker-compose-plugin
  elif command -v dnf >/dev/null 2>&1; then
    dnf install -y docker-compose-plugin
  elif command -v yum >/dev/null 2>&1; then
    yum install -y docker-compose-plugin
  fi
  docker compose version >/dev/null 2>&1 \
    || die "没装上 docker compose 插件，手动装一下再重跑：https://docs.docker.com/compose/install/linux/"
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
  UP_KEY="sk-hub-$(rnd)"
  HUB_PASS="$(randpass)"
  cp .env.example .env
  sed -i "s|^WB2_PUBLIC_URL=.*|WB2_PUBLIC_URL=${PUBLIC_URL}|" .env
  sed -i "s|^WB2_ADMIN_PASS=.*|WB2_ADMIN_PASS=${ADMIN_PASS}|" .env
  sed -i "s|^WB2_JWT_SECRET=.*|WB2_JWT_SECRET=$(rnd)|" .env
  sed -i "s|^WB2_ENC_KEY=.*|WB2_ENC_KEY=$(rnd)|" .env
  # 本站与 hub 共用同一个上游密钥
  sed -i "s|^WB2_UPSTREAM_KEY=.*|WB2_UPSTREAM_KEY=${UP_KEY}|" .env
  sed -i "s|^# WB2_HUB_PANEL_PASS=.*|WB2_HUB_PANEL_PASS=${HUB_PASS}|" .env
  chmod 600 .env
  say "✓ 已生成 .env（访问地址 ${PUBLIC_URL}）"
fi

mkdir -p data hub/accounts hub/usage

# ---------- 3. 起站（hub + 本站）----------
say "→ 构建并启动（本站首次编译 2~5 分钟，hub 直接拉镜像）"
$DC up -d --build

# ---------- 4. 等健康检查 ----------
say "→ 等待服务就绪"
for i in $(seq 1 60); do
  curl -fsS --max-time 2 http://127.0.0.1:8078/api/health >/dev/null 2>&1 && { READY=1; break; }
  sleep 3
done
[ "${READY:-0}" = "1" ] || die "60 秒内本站没起来，看日志：$DC logs -f wb2"

HUB_READY=0
for i in $(seq 1 20); do
  curl -fsS --max-time 2 http://127.0.0.1:8788/ >/dev/null 2>&1 && { HUB_READY=1; break; }
  sleep 3
done
[ "${HUB_READY:-0}" = "1" ] || say "⚠ hub 还没就绪，稍等片刻或看日志：$DC logs -f hub"

ADMIN_PASS_SHOW="$(grep '^WB2_ADMIN_PASS=' .env | cut -d= -f2-)"
PUBLIC_SHOW="$(grep '^WB2_PUBLIC_URL=' .env | cut -d= -f2-)"
HUB_PASS_SHOW="$(grep '^WB2_HUB_PANEL_PASS=' .env | cut -d= -f2-)"
IP_PORT="${PUBLIC_SHOW%:8078}"
cat <<EOF

────────────────────────────────────────────
 ✓ 起来了（本站 + 账号池 hub 都已启动）

 站点        ${PUBLIC_SHOW}
 管理台      登录后侧边栏「管理台」
 管理员      admin
 密码        ${ADMIN_PASS_SHOW}

 账号池面板  ${IP_PORT}:8788/
 面板密码    ${HUB_PASS_SHOW}

 下一步：打开上面的「账号池面板」→ 登录 → Login new account
        加一个 WorkBuddy（国际版）或 CodeBuddy（国内版）账号
        （也可以回本站「账号」页点「OAuth 一键授权」）
        没加账号前，对话接口会返回 502，别慌，这是正常的。

 可选配置（改 .env 后 $DC up -d 生效）：
   WB2_RESEND_KEY     不填 → 关闭邮箱验证码
   WB2_PAY_*          不填 → 只能手动转账 + 管理台确认
   WB2_GH_ID/SECRET   不填 → 隐藏 GitHub 登录

 日志：$DC logs -f wb2    /    $DC logs -f hub
────────────────────────────────────────────
EOF
