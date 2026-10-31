# 部署

三种方式，任选其一。

## 一、Docker（推荐）

```bash
git clone https://github.com/lszyk26/wb2api.git && cd wb2api
bash deploy/install.sh
```

脚本做了：装 Docker → 生成 `.env`（随机 JWT / 加密密钥 / 管理员密码）→ `docker compose up -d --build` → 轮询 `/api/health` → 打印访问地址与密码。

手动版：

```bash
cp .env.example .env
vi .env
docker compose up -d --build
docker compose logs -f wb2
```

数据在 `./data`（SQLite + 密钥），备份这个目录就行。`.env` 里的 `WB2_ENC_KEY` 一旦有用户入库就不要改，否则存量凭据解不开。

## 二、systemd + 源码编译

```bash
# 依赖：Go >= 1.22、Node >= 18
git clone https://github.com/lszyk26/wb2api.git /opt/wb2api
cd /opt/wb2api

# 前端
npm ci && npm run build          # 产物 dist/

# 后端（纯 Go SQLite，无需 cgo）
cd server
CGO_ENABLED=0 go build -o /opt/wb2api/bin/wb2api .
cd ..

# 配置
cp .env.example /opt/wb2api/.env
vi /opt/wb2api/.env              # WB2_DATA_DIR=/opt/wb2api/data 等
chmod 600 .env

# 服务
cp deploy/wb2api.service /etc/systemd/system/
systemctl daemon-reload
systemctl enable --now wb2api
journalctl -u wb2api -f
```

## 三、前面挂 nginx

`deploy/nginx.conf` 是可直接改域名用的片段，两个必须遵守的点：

1. `location /v1/` 里 `proxy_buffering off` —— 否则 SSE 会被攒成一整块再返回
2. `index.html` 发 `Cache-Control: no-store`，`assets/` 发长缓存 —— 否则旧 HTML 指向已删除的 JS，白屏

如果由 Go 直接托管前端（`WB2_STATIC_DIR` 指向 `dist/`），nginx 只做 `proxy_pass` 即可，不用管静态缓存。

## 首次启动后

1. 用 `admin` + `.env` 里的 `WB2_ADMIN_PASS` 登录，进「管理台」
2. **加账号**（这步不做，账号页是空的、`/v1` 全 502）
   - `docker compose` 已经把 hub 一起起好了：打开 `http://服务器IP:8788/`，密码是 `.env` 里的 `WB2_HUB_PANEL_PASS`
   - 面板里 `Login new account` 授权一个 WorkBuddy / CodeBuddy 账号，或在本站账号页点「OAuth 一键授权」
   - 手动起 hub（不用 compose）见 [UPSTREAM.md](UPSTREAM.md#先把上游跑起来)
3. 配发信：`WB2_RESEND_KEY` 或 `WB2_SMTP_*`；都不配则自动关闭邮箱验证码
4. 配支付：`WB2_PAY_*`；不配则用户在「定价」页只能选手动转账，管理台确认到账
5. 改档位价格：管理台 → 档位，实时生效，不用重启

## 把账号池面板藏起来（建议）

`docker compose` 默认把 hub 的面板暴露在 `0.0.0.0:8788`，方便你加号，但等于把「账号池后台」挂在公网，只有一层面板密码。

收紧两步：

1. `docker-compose.yml` 里把端口改成只绑本机：

   ```yaml
   ports:
     - "127.0.0.1:8788:8788"
   ```

2. 用 `deploy/nginx-hub.conf` 反代出来，按需开 IP 白名单和 Basic Auth：

   ```bash
   apt install -y apache2-utils
   htpasswd -c /etc/nginx/wb-hub.htpasswd admin
   cp deploy/nginx-hub.conf /etc/nginx/conf.d/wb-hub.conf
   # 改 server_name / 证书路径 / allow 的 IP，然后
   nginx -t && systemctl reload nginx
   ```

   白名单里别忘 `allow` 你自己的出口 IP；套了 Cloudflare 要先配 `real_ip_header`，不然白名单等于全放行。

只做第 1 步也行：加号时用 `ssh -L 8788:127.0.0.1:8788 root@服务器`，本地开 `http://127.0.0.1:8788/`。

## 升级

```bash
git pull
docker compose up -d --build          # Docker
# 或：npm ci && npm run build && cd server && go build -o ../bin/wb2api . && systemctl restart wb2api
```

SQLite 表结构在启动时自动迁移，`data/` 备份好即可回滚。
