# Upstream contract

**English** | [简体中文](../UPSTREAM.md)

This app is a **facade for an account pool**: users, quotas, billing, keys and payments live here; the actual model calls go to an upstream gateway (`WB2_UPSTREAM`).

**What happens without it**: you can still sign up, log in, create keys and browse pages, but you **cannot add a single account** (the authorize button does nothing, pasted credentials are never loaded) and every `/v1` call returns 502. Accounts only enter the pool through the upstream — this app does not implement the login protocol.

As long as your upstream implements the contract below, everything works.

**Reference implementation**: [ardeyouxipianyi/workbuddy2api-hub](https://github.com/ardeyouxipianyi/workbuddy2api-hub) (Python, image `ghcr.io/ardeyouxipianyi/workbuddy2api-hub:latest`, port 8788). Its script already contains `/accounts/*`, `/panel/login` and `X-Realm`, so it plugs in without code changes.

## Bring the upstream up first

```bash
docker run -d --name wb-hub --restart unless-stopped \
  -p 8788:8788 \
  -e API_KEY=your-upstream-key \
  -e TZ=Asia/Shanghai \
  -v $PWD/hub/accounts:/app/accounts \
  -v $PWD/hub/usage:/app/usage \
  ghcr.io/ardeyouxipianyi/workbuddy2api-hub:latest \
  python wb_proxy.py --host 0.0.0.0 --lan --panel-password your-panel-password
```

Things that bite if you copy blindly (all verified on the real image):

- `--host 0.0.0.0` is mandatory. The image's default command includes it, but the moment you override `command` you must spell it out, or connections from outside the container are reset.
- **The panel password is not an environment variable** — only `--panel-password` (or change it in the panel afterwards). Default is `admin`.
- The panel lives at the **root path `/`**; `/panel/*` are API routes (`/panel/login`, …). Hitting `/panel` directly is a 404.
- The password hash is stored in `accounts/settings.json`, so `accounts/` must be persistent.

Then wire the three values together:

| Upstream | This app |
|---|---|
| `API_KEY` (env) | `WB2_UPSTREAM_KEY` |
| value of `--panel-password` | `WB2_HUB_PANEL_PASS` |
| container port `8788` | `WB2_UPSTREAM` (`http://127.0.0.1:8788` same host, `http://hub:8788` in compose) |

Also point `WB2_HUB_DIR` at the mounted `hub/` directory so this app can see `accounts/` and `usage/`.

### Does the stock image work?

Yes. Tested against `:latest` with no patches: `/panel/login` returns a token, `/accounts/login/start` returns `{state, authUrl}` (this app reads `authUrl`), `/accounts/test` and `/accounts/set` respond, `/accounts/delete` works, and `/v1/models` only answers with a valid `Bearer` key.

## Authentication

```
Authorization: Bearer <WB2_UPSTREAM_KEY>
```

Panel routes (account authorization) use a panel token instead:

```
POST /panel/login   {"password": "<WB2_HUB_PANEL_PASS>"}  →  {"token": "..."}
subsequent requests: X-Panel-Token: <token>     # retried automatically once on 401
```

## 1. Model list

```
GET /v1/models
GET /v1/models?realm=cn
```

OpenAI-style response:

```json
{ "data": [ { "id": "deepseek-v4.1-flash", "object": "model" } ] }
```

This app caches it (`refreshModels()`) and filters it through `settings.models_disabled` before exposing it to users.

## 2. Chat

```
POST /v1/chat/completions      # identical to OpenAI
POST /v1/messages              # Anthropic style, converted to chat/completions
X-Realm: cn | intl             # optional, pick an account pool
Accept: text/event-stream      # streaming
```

- **Streaming must pass SSE through verbatim** (including `data: [DONE]`); this app forwards and parses `usage` while doing so
- If the client disconnects early, `usage` may never arrive and this app falls back to token estimation
- Upstream error semantics this app translates:
  - `11140` account restricted → 503 `account_restricted`
  - `11158` model not allowed for this account type → 400 `model_not_supported_by_pool`, plus a 30-minute block on that model
  - `429 model busy` → treated as daily quota exhausted; retry on another account or the other realm

## 3. Account management (panel routes)

| Method | Path | Body | Notes |
|---|---|---|---|
| POST | `/accounts/login/start` | `{"realm":"intl\|cn","platform":"CLI"}` | returns `{"state":"...","authUrl":"..."}` — open in a new window |
| GET | `/accounts/login/poll?state=` | — | `{"status":"pending\|ok\|fail","uid":"...","label":"..."}` |
| POST | `/accounts/test` | `{"uid":"...","model":"deepseek-v4.1-flash"}` | `{"ok":true}` or `{"ok":false,"note":"..."}` |
| POST | `/accounts/set` | `{"uid":"...","proxySlot":"slot-1"}` | bind an egress slot |
| POST | `/accounts/delete` | `{"uid":"..."}` | remove from pool |

## 4. Files on disk

Besides HTTP, this app reads and writes the upstream data directory (`WB2_HUB_DIR`) directly:

```
$WB2_HUB_DIR/accounts/<uid>.json      # written when a user pastes credentials; the hub's file watcher picks it up
$WB2_HUB_DIR/usage/usage.jsonl        # upstream usage log, imported incrementally every 60s
```

Each line of `usage.jsonl` is a JSON object containing at least `at`, `model` and token counts. Import is deduplicated by `at+model+sha` with a stored offset, so re-importing never double-bills.

## 5. Realm splitting

The upstream pools accounts by realm (`intl` = workbuddy.ai, `cn` = codebuddy.cn). This app's strategy:

- Models in the whitelist (`cnRealmModels`) are sent with `X-Realm: cn`
- International-only models (`gpt-*` / `gemini-*` / `grok-*`) go to `intl`
- Everything else is split **per conversation**: sha256 of the first user message decides the side (`settings.cn_split`, 70% to cn by default), so a conversation never flips sides and prefix caching stays intact
- If one side fails, the other is retried once and the failing side is penalized for 60s

If your upstream has a single pool, just ignore `X-Realm` — all requests land in the same pool.
