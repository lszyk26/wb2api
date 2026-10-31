# 上游契约

本站是**账号池的门面**：用户体系、额度、计费、密钥、支付都在本站；真正调模型的是上游网关（`WB2_UPSTREAM`）。

只要你的上游满足下面这套接口，本站就能完整工作。

**参考实现**：[ardeyouxipianyi/workbuddy2api-hub](https://github.com/ardeyouxipianyi/workbuddy2api-hub)（Python，镜像 `ghcr.io/ardeyouxipianyi/workbuddy2api-hub:latest`，端口 8788）。它的脚本里已经有 `/accounts/*`、`/panel/login`、`X-Realm`，直接对接即可，无需改代码。

## 鉴权

```
Authorization: Bearer <WB2_UPSTREAM_KEY>
```

面板类接口（账号授权）另用面板 token：

```
POST /panel/login   {"password": "<WB2_HUB_PANEL_PASS>"}  →  {"token": "..."}
后续请求：X-Panel-Token: <token>       # 401 时会自动重新登录一次
```

## 1. 模型清单

```
GET /v1/models
GET /v1/models?realm=cn
```

返回 OpenAI 风格：

```json
{ "data": [ { "id": "deepseek-v4.1-flash", "object": "model" } ] }
```

本站会把它缓存下来（`refreshModels()`），并根据 `settings.models_disabled` 过滤后再对用户展示。

## 2. 对话

```
POST /v1/chat/completions      # 与 OpenAI 完全一致
POST /v1/messages              # Anthropic 风格，本站转成 chat/completions
X-Realm: cn | intl             # 可选，指定账号池
Accept: text/event-stream      # 流式
```

- **流式必须原样透传 SSE**（含 `data: [DONE]`），本站边转发边解析 `usage` 计费
- 客户端中途断开时 `usage` 可能收不到，本站会退化到 token 估算
- 常见上游错误语义（本站会翻译成对用户友好的 4xx/5xx）：
  - `11140` 账号被限制 → 503 `account_restricted`
  - `11158` 模型不适用于该账号类型 → 400 `model_not_supported_by_pool`，并把模型拉黑 30 分钟
  - `429 model busy` → 视为当日额度耗尽，换号/换侧重试

## 3. 账号管理（面板接口）

| 方法 | 路径 | 请求体 | 说明 |
|---|---|---|---|
| POST | `/accounts/login/start` | `{"realm":"intl\|cn","platform":"CLI"}` | 返回 `{"url": "...", "state": "..."}`，前端新窗口打开 |
| GET | `/accounts/login/poll?state=` | — | `{"status":"pending\|ok\|fail","uid":"...","label":"..."}` |
| POST | `/accounts/test` | `{"uid":"...","model":"deepseek-v4.1-flash"}` | `{"ok":true}` 或 `{"ok":false,"note":"..."}` |
| POST | `/accounts/set` | `{"uid":"...","proxySlot":"slot-1"}` | 绑定出口槽位 |
| POST | `/accounts/delete` | `{"uid":"..."}` | 移出账号池 |

## 4. 文件级联

除了 HTTP，本站还会直接读写上游的数据目录（`WB2_HUB_DIR`）：

```
$WB2_HUB_DIR/accounts/<uid>.json      # 手动提交凭据时写入，上游文件监听起来就生效
$WB2_HUB_DIR/usage/usage.jsonl        # 上游用量流水，本站每 60s 增量导入做对账
```

`usage.jsonl` 每行一个 JSON，至少包含 `at`、`model`、输入/输出 token；本站按 `at+model+sha` 去重并记录 offset，重复导入不会重复计费。

## 5. 关于 realm 分流

上游按 realm 分池（`intl` = workbuddy.ai，`cn` = codebuddy.cn）。本站的策略：

- 白名单模型（`cnRealmModels`）明确走 `X-Realm: cn`
- 国际专属（`gpt-*` / `gemini-*` / `grok-*`）走 `intl`
- 其余按**会话**分流：第一条 user 消息 sha256 → 固定落一侧（`settings.cn_split`，默认 70% 走 cn），保证同一段对话不来回跳，前缀缓存不失效
- 一侧失败自动换另一侧重试一次，失败侧惩罚 60s

如果你的上游只有一个池，忽略 `X-Realm` 即可，所有请求都进同一个池。
