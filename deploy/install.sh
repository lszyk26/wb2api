#!/usr/bin/env bash
# One-shot deploy: install Docker -> generate .env (random secrets) -> start hub + this app -> print credentials
# Usage: bash deploy/install.sh
set -euo pipefail

cd "$(dirname "$0")/.."

say() { printf '\033[1;33m%s\033[0m\n' "$*"; }
die() { printf '\033[1;31mX %s\033[0m\n' "$*" >&2; exit 1; }

# ---------- 0. Preflight ----------
[ "$(id -u)" = "0" ] || die "Run as root: sudo bash deploy/install.sh"
command -v curl >/dev/null || die "curl is required"

# ---------- 1. Docker ----------
if ! command -v docker >/dev/null 2>&1; then
  say "-> Docker not found, installing (official script)"
  curl -fsSL https://get.docker.com | sh
  systemctl enable --now docker
fi
if docker compose version >/dev/null 2>&1; then
  DC="docker compose"
elif command -v docker-compose >/dev/null 2>&1; then
  DC="docker-compose"
else
  say "-> Installing the docker compose plugin"
  if command -v apt-get >/dev/null 2>&1; then
    apt-get update -qq && apt-get install -y docker-compose-plugin
  elif command -v dnf >/dev/null 2>&1; then
    dnf install -y docker-compose-plugin
  elif command -v yum >/dev/null 2>&1; then
    yum install -y docker-compose-plugin
  fi
  docker compose version >/dev/null 2>&1 \
    || die "compose plugin still missing; install it manually then re-run: https://docs.docker.com/compose/install/linux/"
  DC="docker compose"
fi
say "OK: Docker ready ($DC)"

# ---------- 2. .env ----------
rnd() { openssl rand -hex 32 2>/dev/null || head -c 32 /dev/urandom | od -An -tx1 | tr -d ' \n'; }
randpass() { tr -dc 'A-Za-z0-9' </dev/urandom | head -c 16; }

if [ -f .env ]; then
  say "-> .env exists, leaving it alone (delete it to start over)"
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
  # Shared by this app and the hub
  sed -i "s|^WB2_UPSTREAM_KEY=.*|WB2_UPSTREAM_KEY=${UP_KEY}|" .env
  sed -i "s|^# WB2_HUB_PANEL_PASS=.*|WB2_HUB_PANEL_PASS=${HUB_PASS}|" .env
  chmod 600 .env
  say "OK: .env written (public URL ${PUBLIC_URL})"
fi

mkdir -p data hub/accounts hub/usage

# ---------- 3. Start everything ----------
say "-> Building and starting (first build takes 2-5 min; the hub image is pulled)"
$DC up -d --build

# ---------- 4. Wait for health ----------
say "-> Waiting for services"
for i in $(seq 1 60); do
  curl -fsS --max-time 2 http://127.0.0.1:8078/api/health >/dev/null 2>&1 && { READY=1; break; }
  sleep 3
done
[ "${READY:-0}" = "1" ] || die "The app did not come up within 60s; check: $DC logs -f wb2"

HUB_READY=0
for i in $(seq 1 20); do
  curl -fsS --max-time 2 http://127.0.0.1:8788/ >/dev/null 2>&1 && { HUB_READY=1; break; }
  sleep 3
done
[ "${HUB_READY:-0}" = "1" ] || say "WARN: hub not ready yet; give it a moment or check: $DC logs -f hub"

ADMIN_PASS_SHOW="$(grep '^WB2_ADMIN_PASS=' .env | cut -d= -f2-)"
PUBLIC_SHOW="$(grep '^WB2_PUBLIC_URL=' .env | cut -d= -f2-)"
HUB_PASS_SHOW="$(grep '^WB2_HUB_PANEL_PASS=' .env | cut -d= -f2-)"
IP_PORT="${PUBLIC_SHOW%:8078}"
cat <<EOF

────────────────────────────────────────────
 Done. Both this app and the account hub are running.

 Site          ${PUBLIC_SHOW}
 Admin panel   sidebar -> "Admin" after login
 Admin user    admin
 Password      ${ADMIN_PASS_SHOW}

 Hub panel     ${IP_PORT}:8788/
 Hub password  ${HUB_PASS_SHOW}

 Next: open the hub panel -> sign in -> "Login new account"
       authorize one WorkBuddy (international) or CodeBuddy (CN) account.
       (Alternatively use "OAuth one-click" on this site's Accounts page.)
       Until an account is added, /v1 returns 502 — that is expected.

 Optional (edit .env then $DC up -d):
   WB2_RESEND_KEY     unset -> email verification disabled
   WB2_PAY_*          unset -> manual transfer + admin confirmation only
   WB2_GH_ID/SECRET   unset -> GitHub login hidden

 Logs: $DC logs -f wb2   /   $DC logs -f hub
────────────────────────────────────────────
EOF
