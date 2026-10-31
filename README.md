# WorkBuddy 2.0 · Student API

Turn a pool of WorkBuddy / CodeBuddy accounts into **one OpenAI-compatible endpoint**.

Single Go binary + Vue 3 frontend + SQLite. Ships with signup/login, API keys, usage metering, plans & top-ups, referral rewards, online payments, and account-pool health/routing. The Go binary serves the built frontend itself, so one process is a whole site.

> AGPL-3.0. If you run a modified version, you have to publish your changes.

**English** | [简体中文](README.zh-CN.md)

![Landing page](docs/screenshots/landing.png)

---

## Deploy in 30 seconds

```bash
git clone https://github.com/lszyk26/wb2api.git && cd wb2api
bash deploy/install.sh          # generates secrets → writes .env → starts hub + this app
```

Then open `http://YOUR_SERVER_IP:8078`. Admin user is `admin`, password is in `.env` as `WB2_ADMIN_PASS` (the script generates and prints it).

Doing it by hand:

```bash
cp .env.example .env && vi .env   # at minimum: WB2_PUBLIC_URL, WB2_UPSTREAM, WB2_UPSTREAM_KEY
docker compose up -d --build
```

No Docker? See [docs/en/DEPLOY.md](docs/en/DEPLOY.md) (systemd + nginx + manual build).

---

## You need an upstream

> **No hub, no accounts.** This project does not implement the WorkBuddy / CodeBuddy login protocol and cannot talk to the provider directly.
> "OAuth one-click" just forwards the hub's login flow to your browser; "paste credentials" just drops a file into `WB2_HUB_DIR/accounts/` for the hub to load.
> **With no hub running, the accounts page stays empty and every `/v1` call returns 502.**

**This project does not own any accounts; it is a facade in front of an account pool.** You must point it at an OpenAI-compatible upstream gateway (default `WB2_UPSTREAM=http://127.0.0.1:8788`). This app handles users, quotas, billing, keys, payments, and **which upstream egress a request goes to**.

Minimum contract the upstream must implement (`Bearer` auth):

| Method | Path | Purpose |
|---|---|---|
| GET | `/v1/models` | Model list, supports `?realm=cn` |
| POST | `/v1/chat/completions` | Chat, SSE passthrough |
| POST | `/accounts/test` | Account self-test (`{uid, model}`) |
| POST | `/accounts/set` | Bind egress (`{uid, proxySlot}`) |
| POST | `/accounts/delete` | Remove from pool |
| POST | `/accounts/login/start` `…/poll` | OAuth authorization |

Full field definitions: [docs/en/UPSTREAM.md](docs/en/UPSTREAM.md). The upstream account directory (`WB2_HUB_DIR/accounts/*.json`) and usage file (`WB2_USAGE_PATH`) are also read/written for account submission and usage reconciliation.

---

## Dependencies

**Build machine** (skip all of this if you only use Docker)

| Need | Version | Used for |
|---|---|---|
| Node.js | ≥ 18 (22 recommended) | `vite build` only |
| Go | ≥ 1.22 | Backend compile only |
| Docker + compose | any recent | All you need for one-click deploy |

**Third-party libraries**: everything is pinned in `package-lock.json` / `go.sum`. No CDN, no web fonts, no cgo.

- Frontend: 3 direct deps — `vue`, `vite`, `@vitejs/plugin-vue` (`npm ci` installs 32 packages in ~6s)
- Backend: 3 direct deps — `gorm.io/gorm`, `github.com/glebarez/sqlite` (pure-Go SQLite, **no gcc / cgo**), `golang.org/x/crypto` (bcrypt). Everything else is stdlib.

**Required at runtime**

1. **Upstream account hub `wb-proxy-hub`** — reference implementation: [ardeyouxipianyi/workbuddy2api-hub](https://github.com/ardeyouxipianyi/workbuddy2api-hub) (Python, image `ghcr.io/ardeyouxipianyi/workbuddy2api-hub:latest`). **Without it you cannot add accounts and `/v1` returns 502** (pages, signup and the admin panel still work).
2. **Accounts**: WorkBuddy (international) or CodeBuddy (China), ones you legitimately hold, added **through the hub**.

**Shortest working path** — `bash deploy/install.sh` already starts the hub for you; the only step left is adding an account.

1. Open `http://YOUR_SERVER_IP:8788/` (account-pool panel), password is `WB2_HUB_PANEL_PASS` in `.env`
2. Sign in → `Login new account` → pick international / China → finish the flow in your browser
3. Back on this site's Accounts page you'll see it; until then `/v1` returning 502 is expected

Starting the hub manually instead:

```bash
docker run -d --name wb-hub --restart unless-stopped -p 8788:8788 \
  -e API_KEY=your-upstream-key -e TZ=Asia/Shanghai \
  -v $PWD/hub/accounts:/app/accounts -v $PWD/hub/usage:/app/usage \
  ghcr.io/ardeyouxipianyi/workbuddy2api-hub:latest \
  python wb_proxy.py --host 0.0.0.0 --lan --panel-password your-panel-password
```

Two things people get wrong:

- `--host 0.0.0.0` is required, otherwise connections from outside the container are reset
- **The panel password is only a CLI flag (`--panel-password`), there is no env var for it**; the default is `admin`, so leaving it is a wide-open door

Then configure this app: `WB2_UPSTREAM` (`http://127.0.0.1:8788` on the same host, `http://hub:8788` inside compose), `WB2_UPSTREAM_KEY=<the same API_KEY>`, `WB2_HUB_PANEL_PASS=<panel password>`, `WB2_HUB_DIR=<the mounted hub/>`.

---

## Features

- **Users**: signup / email-code login / GitHub OAuth, password reset, referral rewards, lottery, built-in playground
- **API keys**: `sk-wb-` prefix, count limited by plan, only a hash plus an encrypted copy is stored, rotatable
- **OpenAI-compatible**: `/v1/chat/completions` (streaming and non-streaming), `/v1/models`, `/v1/messages` (Anthropic style)
- **Billing**: points = (input×2 + output×8)/1000 × model multiplier; daily quota resets every day; top-up points are consumed after the daily quota
- **Plans**: sponsor tiers (LV1–LV3) and contributor tiers (C1–C4, computed from authorized accounts) — whichever daily quota is higher wins; C1–C3 stack 1.5×, capped at 300k points/day
- **Account pool**: OAuth authorize → self-test → join pool → rotate egress binding; session-level cn/intl split with automatic failover to the other side
- **Payments**: two drivers (epay / mupa) with signed async callbacks
- **Admin panel**: users, orders, redeem codes, account review, live plan pricing, lottery prizes, upstream health, settlement
- **Frontend**: iOS-style dark glass, 50+ animations, four performance presets, mobile drawer; distinct empty / error / offline states

---

## Configuration

Everything is configured through `.env` (or the file `WB2_ENV_FILE` points at); restart after changes. Common keys:

| Variable | Default | Notes |
|---|---|---|
| `WB2_LISTEN` | `127.0.0.1:8078` | Listen address; use `0.0.0.0:8078` in containers |
| `WB2_PUBLIC_URL` | `http://localhost:8078` | **Required**: invite links, payment callbacks, email footers |
| `WB2_DATA_DIR` | `./data` | Data dir (SQLite + uploads + hub mount point) |
| `WB2_STATIC_DIR` | `./dist` | Built frontend; leave empty to serve API only |
| `WB2_JWT_SECRET` | random | Session token secret; **unset means everyone is logged out on restart** |
| `WB2_ENC_KEY` | same as JWT | Credential encryption key; **changing it makes stored credentials unreadable** |
| `WB2_ADMIN_USER` / `WB2_ADMIN_PASS` | `admin` / random, logged | First admin account |
| `WB2_UPSTREAM` / `WB2_UPSTREAM_KEY` | `http://127.0.0.1:8788` / empty | Upstream gateway and its key |
| `WB2_HUB_DIR` | `$WB2_DATA_DIR/hub` | Upstream account / usage directory |
| `WB2_RESEND_KEY` / `WB2_SMTP_*` | empty | Email (Resend first, SMTP fallback); unset disables email verification |
| `WB2_GH_ID` / `WB2_GH_SECRET` | empty | GitHub OAuth login; unset hides the entry |
| `WB2_PAY_*` | empty | Payment channels; unset means manual bank transfer + admin confirmation |

Full list with comments: [.env.example](.env.example).

---

## Layout

```
server/          Go backend (net/http + GORM + pure-Go SQLite, no cgo)
  gateway.go     /v1 forwarding, SSE passthrough, cn/intl split + failover
  plans.go       plans, quotas, stacking rules
  payments.go    payment drivers
  upstream.go    upstream account dir / usage reconciliation
src/             Vue 3 frontend (Vite, no vue-router — App.vue switches pages)
  pages/         Landing / Dashboard / Keys / Usage / Pricing / Accounts / Admin …
  styles/        design tokens, dark glass, motion, performance tiers
deploy/          install.sh / systemd unit / nginx snippets
docs/            deployment and upstream contract
```

---

## Gotchas (worth reading before touching the code)

- SSE on `/v1` **requires** `proxy_buffering off`, otherwise streaming arrives in one chunk
- `index.html` must be `no-store` while hashed assets get long cache; otherwise a stale HTML points at deleted JS → blank page
- Don't name Vue scoped root classes after utility classes (`.lg/.sm/.pri/.icon/.glass`) — they leak onto global buttons
- `position:fixed` modals must be `Teleport`ed to body; an ancestor's `will-change:transform` hijacks the containing block
- Animate only `transform`/`opacity`; low-end devices fall back to flat backgrounds

Code comments are in Chinese — that's a known limitation, PRs adding English comments are welcome.

---

## Disclaimer

This project is an **admin panel and billing gateway** for an account pool. It ships with and distributes no account credentials. Before using it, make sure the accounts and upstream service you connect to permit this kind of usage; account bans, rate limits, and any legal or compliance consequences are the deployer's responsibility.

## License

[AGPL-3.0](LICENSE)
