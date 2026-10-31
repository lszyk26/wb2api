# ---------- 1. 前端 ----------
FROM node:22-alpine AS web
WORKDIR /src
COPY package.json package-lock.json ./
RUN npm ci
COPY index.html vite.config.js ./
COPY src ./src
RUN npm run build

# ---------- 2. 后端 ----------
FROM golang:1.22-alpine AS api
WORKDIR /src
COPY server/go.mod server/go.sum ./
RUN go mod download
COPY server ./
RUN CGO_ENABLED=0 GOFLAGS=-trimpath go build -ldflags "-s -w" -o /out/wb2api .

# ---------- 3. 运行 ----------
FROM alpine:3.20
RUN apk add --no-cache ca-certificates tzdata \
 && adduser -D -H -s /sbin/nologin wb2
WORKDIR /app
COPY --from=api /out/wb2api /app/wb2api
COPY --from=web /src/dist /app/dist
RUN mkdir -p /data /hub && chown -R wb2:wb2 /app /data /hub
USER wb2
ENV WB2_LISTEN=0.0.0.0:8078 \
    WB2_DATA_DIR=/data \
    WB2_STATIC_DIR=/app/dist \
    WB2_HUB_DIR=/hub
EXPOSE 8078
VOLUME ["/data"]
ENTRYPOINT ["/app/wb2api"]
