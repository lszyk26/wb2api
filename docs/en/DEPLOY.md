# Deployment

Three ways. Pick one.

**English** | [简体中文](../DEPLOY.md)

## 1. Docker (recommended)

```bash
git clone https://github.com/lszyk26/wb2api.git && cd wb2api
bash deploy/install.sh
```

The script: installs Docker → generates `.env` (random JWT key, encryption key, admin password, upstream key, hub panel password) → `docker compose up -d --build` → polls `/api/health` → prints the URL, the admin password and the hub panel.

Manual equivalent:

```bash
cp .env.example .env
vi .env
docker compose up -d --build
docker compose logs -f wb2
```

Data lives in `./data` (SQLite + keys) — back that up. The hub's accounts and `settings.json` live in `./hub` — back that up too. Once credentials are stored, do not change `WB2_ENC_KEY`, or they become unreadable.

## 2. systemd, built from source

```bash
# Requirements: Go >= 1.22, Node >= 18
git clone https://github.com/lszyk26/wb2api.git /opt/wb2api
cd /opt/wb2api

# Frontend
npm ci && npm run build          # -> dist/

# Backend (pure Go SQLite, no cgo)
cd server
CGO_ENABLED=0 go build -o /opt/wb2api/bin/wb2api .
cd ..

# Config
cp .env.example /opt/wb2api/.env
vi /opt/wb2api/.env              # set WB2_DATA_DIR=/opt/wb2api/data etc.
chmod 600 .env

# Service
cp deploy/wb2api.service /etc/systemd/system/
systemctl daemon-reload
systemctl enable --now wb2api
journalctl -u wb2api -f
```

Note: on a bare-metal install `WB2_UPSTREAM` must be `http://127.0.0.1:8788` (the compose default `http://hub:8788` only resolves inside the compose network).

## 3. Behind nginx

`deploy/nginx.conf` is a ready-to-edit snippet. Two non-negotiables:

1. `proxy_buffering off` inside `location /v1/` — otherwise SSE is buffered and delivered in one chunk
2. `index.html` served with `Cache-Control: no-store`, `assets/` with long cache — otherwise a stale HTML points at deleted JS and you get a blank page

If the Go binary serves the frontend itself (`WB2_STATIC_DIR` → `dist/`), nginx only needs `proxy_pass`; static caching is handled in Go.

## After first boot

1. Log in as `admin` with `WB2_ADMIN_PASS` from `.env`, open **Admin**
2. **Add an account** (skip this and the accounts page is empty and `/v1` returns 502)
   - `docker compose` already started the hub: open `http://YOUR_SERVER_IP:8788/`, password is `WB2_HUB_PANEL_PASS` from `.env`
   - In the panel click `Login new account` and authorize a WorkBuddy / CodeBuddy account, or use **Accounts → OAuth one-click** on this site
   - Starting the hub without compose: see [UPSTREAM.md](UPSTREAM.md#bring-the-upstream-up-first)
3. Email: `WB2_RESEND_KEY` or `WB2_SMTP_*`; if neither is set, email verification is disabled automatically
4. Payments: `WB2_PAY_*`; if unset, users can only choose "manual transfer" and an admin confirms the order
5. Plan pricing: Admin → Plans, takes effect immediately, no restart

## Locking down the account pool panel (recommended)

`docker compose` exposes the hub panel on `0.0.0.0:8788` so you can add accounts, but that puts the pool backend on the public internet with only a panel password in front of it.

Two steps to tighten it:

1. Bind it to localhost in `docker-compose.yml`:

   ```yaml
   ports:
     - "127.0.0.1:8788:8788"
   ```

2. Reverse-proxy it with `deploy/nginx-hub.conf`, enabling IP allowlist and optionally Basic Auth:

   ```bash
   apt install -y apache2-utils
   htpasswd -c /etc/nginx/wb-hub.htpasswd admin
   cp deploy/nginx-hub.conf /etc/nginx/conf.d/wb-hub.conf
   # edit server_name / cert paths / allowed IPs, then
   nginx -t && systemctl reload nginx
   ```

   Add your own egress IP to `allow`. Behind Cloudflare you must configure `real_ip_header` first, otherwise the allowlist matches CF's IPs and lets everyone in.

Even step 1 alone helps: add accounts over a tunnel, `ssh -L 8788:127.0.0.1:8788 root@your-server`, then open `http://127.0.0.1:8788/` locally.

## Upgrading

```bash
git pull
docker compose up -d --build          # Docker
# or: npm ci && npm run build && cd server && go build -o ../bin/wb2api . && systemctl restart wb2api
```

Schema migrations run automatically on boot; keep `data/` backed up so you can roll back.
