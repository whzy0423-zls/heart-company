# 构建上下文为仓库根 nine-xing/（compose 中 context: .），
# 这样能同时访问 nx-backend/ 与 shared/。
# ---- build stage ----
FROM golang:1.22-alpine AS builder

WORKDIR /src

WORKDIR /src/apps/server

# 拷源码后在构建环境内补全依赖校验和并编译。
# （go.sum 由 go mod tidy 联网生成，构建阶段有网络。）
COPY nx-backend/apps/server/ ./
RUN go mod tidy
RUN CGO_ENABLED=0 GOOS=linux go build -trimpath -ldflags="-s -w" -o /out/server ./cmd/server

# ---- runtime stage ----
FROM alpine:3.20 AS production

RUN apk add --no-cache ca-certificates tzdata && \
    addgroup -S app && adduser -S app -G app

WORKDIR /app
COPY --from=builder /out/server /app/server

# 站点配置持久化在挂载卷里；容器首启时若卷为空，由 entrypoint 用内置默认值播种
COPY shared/site-config.json /app/default-site-config.json
COPY nx-backend/apps/server/docker-entrypoint.sh /app/docker-entrypoint.sh
RUN chmod +x /app/docker-entrypoint.sh && \
    mkdir -p /data && chown -R app:app /app /data

USER app

ENV PORT=5320 \
    SITE_CONFIG_PATH=/data/site-config.json \
    BUILD_SCRIPT=

EXPOSE 5320

ENTRYPOINT ["/app/docker-entrypoint.sh"]
CMD ["/app/server"]
