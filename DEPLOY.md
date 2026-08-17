# 部署说明（Docker Compose 四服务）

整套包含四个服务，由仓库根的 `docker-compose.yml` 编排：

| 服务 | 内容 | 宿主机端口 | 说明 |
|------|------|-----------|------|
| `db` | PostgreSQL 16 | 不对外 | 用户/角色/菜单持久化，数据存卷 `pgdata` |
| `server` | Go 后台服务 | 不对外 | 仅集群内 `:5320`，经各 nginx 反代 `/api` |
| `admin` | 后台管理(web-antd) | `127.0.0.1:8080` | 静态页 + 反代 `/api` 到 server |
| `website` | 官网(website-react) | `127.0.0.1:8000` | 静态页 + 反代 `/api/public/*` 到 server |

数据流：
```
浏览器 → admin:8080  → / 静态后台
                      → /api/* 反代 → server:5320 → PostgreSQL(用户/角色/菜单)
                                                  → /data/site-config.json(官网配置)
浏览器 → website:8000 → / 静态官网
                      → 启动时 fetch /api/public/site-config 反代 → server:5320
```

持久化：

- **用户 / 角色 / 菜单**：PostgreSQL，存 docker 卷 `pgdata`，永久保存。
- **官网站点配置**：JSON 文件，存 docker 卷 `site-config`（容器内 `/data`），后台保存后**刷新官网即生效**。
- **上传文件**：优先使用 OSS；若根 `.env` 未配置任何 `OSS_*`，则保存到 docker 卷 `uploads`（容器内 `/data/uploads`）。

首次启动时 server 会自动建表并播种一个超级管理员（账号取 `ADMIN_USERNAME`，密码 `ADMIN_PASSWORD`，bcrypt 加密入库）。之后即可在「系统管理」里增删用户/角色/菜单，全部落库永久生效。

---

## 一、首次部署

在仓库根 `nine-xing/` 下：

```bash
# 1) 设置生产密钥（必填；未设置时 docker compose 会拒绝启动）
export JWT_SECRET="$(openssl rand -hex 32)"
export ADMIN_USERNAME="admin"
export ADMIN_PASSWORD="你的强密码"
export POSTGRES_PASSWORD="$(openssl rand -hex 24)"

# 2) 如果后台/官网/API 不同源，配置浏览器允许访问 API 的生产域名
# export CORS_ALLOWED_ORIGINS="https://admin.example.com,https://www.example.com"
# 如后端前面有反向代理且需要按真实客户端 IP 限流，显式配置可信代理来源。
# docker-compose 默认固定 nx 网络为 172.28.0.0/24；如你改了 compose 网络或外层代理链，请同步调整。
export TRUSTED_PROXY_CIDRS="${TRUSTED_PROXY_CIDRS:-172.28.0.0/24}"

# 3) 如果要使用人声管理/声音测试，必须配置 MiniMax 中文站 API Key
export MINIMAX_API_KEY="你的 MiniMax API Key"
export MINIMAX_API_BASE="https://api.minimaxi.com"
# 如控制台要求 GroupId，再设置：
# export MINIMAX_GROUP_ID="你的 GroupId"

# 4) 如使用 OSS 上传，请补齐全部 OSS 配置；不使用 OSS 就保持所有 OSS_* 为空
# export OSS_ACCESS_KEY_ID="..."
# export OSS_ACCESS_KEY_SECRET="..."
# export OSS_BUCKET="..."
# export OSS_REGION="cn-beijing"
# export OSS_ENDPOINT="https://oss-cn-beijing.aliyuncs.com"
# export OSS_PUBLIC_URL="https://your-bucket.oss-cn-beijing.aliyuncs.com"

# 5) 如启用视频/图片生成网关，必须显式配置对应 API_BASE；
# 如启用视频生成/视频分析网关（VIDEO_API_KEY 非空），还必须配置后端公网根地址；
# 外部视频网关只会使用 PUBLIC_BASE_URL 补全本地相对资源，不会从 Host/X-Forwarded-Host 推断。
# export VIDEO_API_BASE="https://video-api.example.com"
# export IMAGE_API_BASE="https://image-api.example.com"
# export PUBLIC_BASE_URL="https://api.example.com"

# 6) 如启用微信支付/推送，请补齐对应生产配置
# export WXPAY_MCH_ID="..."
# export WXPAY_APPID="..."
# export WXPAY_API_V3_KEY="..."
# export WXPAY_SERIAL_NO="..."
# 将证书放到宿主机 ./certs/wxpay/，compose 会只读挂载到 /run/secrets/wxpay/
# export WXPAY_PRIVATE_KEY_PATH="/run/secrets/wxpay/apiclient_key.pem"
# export WXPAY_PLATFORM_CERT_PATH="/run/secrets/wxpay/wechatpay_platform.pem"
# export WXPAY_NOTIFY_URL="https://api.example.com/api/pay/notify"
# export JPUSH_APP_KEY="..."
# export JPUSH_MASTER_SECRET="..."

# 7) 构建并启动
docker compose up -d --build

# 8) 查看状态/日志
docker compose ps
docker compose logs -f server
```

启动后：
- 后台管理：服务器本机 `http://127.0.0.1:8080`（账号见上面的 `ADMIN_*`）
- 官网：服务器本机 `http://127.0.0.1:8000`

`docker-compose.yml` 默认只绑定本机回环地址，公网访问请走下一节的宿主机 Nginx / Caddy / Traefik 反代，或临时用 SSH 隧道调试。

> 也可把上面的环境变量写进仓库根的 `.env` 文件（compose 会自动读取），避免每次 export。

稳定性参数建议从以下初始值开始，并根据 `/api/app/health` 中的 TTS 等待时间和数据库 `waitCount` 调整：

```bash
DB_MAX_OPEN_CONNS=20
DB_MAX_IDLE_CONNS=5
XINZHILI_TTS_MAX_CONCURRENT=8
XINZHILI_MAX_CONNECTIONS=50
```

## 二、更新发布

### 后端 `server`：芯之力会话锁升级约束

当前版本使用 PostgreSQL transaction advisory lock 保证同一用户、卡片的 `xinzhili_voice` 场景只创建一个会话。这个锁是所有新版 `server` 实例共同遵守的应用协议，旧版实例不会获取该锁。因此发布本版本时，**禁止旧版和新版 `server` 实例混跑**，不能直接对 `server` 执行常规零停机滚动更新。

后端发布按下面顺序执行；多主机或编排平台需在所有节点完成同一步骤后再进入下一步：

1. 在摘流前构建并标记待发布镜像，记录镜像 tag 或 digest；不要在停服窗口内临时修改代码。
2. 从负载均衡、网关或服务发现中摘除全部旧 `server` 实例，停止向它们分配新请求。
3. 通过负载均衡监控、访问日志或连接指标确认旧实例的活跃请求已经归零；芯之力流式请求最长可能持续数分钟，应等待其自然结束，不要强杀仍在处理请求的进程。
4. 停止所有旧 `server` 容器或进程。逐台执行只针对 `server` 的停止操作，禁止使用会删除数据库卷或无差别删除容器/进程的命令。
5. 用只读检查确认所有节点均不存在旧版运行实例，再启动全部新版 `server` 实例。
6. 检查容器状态和日志，并请求 `GET /api/app/health`；全部新版实例健康后再恢复负载均衡流量。

单机 Docker Compose 可参考以下检查清单。构建发生在摘流前；摘流和确认活跃请求需在实际使用的负载均衡或网关上完成：

```bash
# 摘流前：只构建 server，并记录最终镜像 ID
docker compose build server
docker compose images server

# 已摘流且活跃请求归零后：只停止 server，不影响 db/admin/website 和数据卷
docker compose stop server

# 只读确认：Compose 中 server 未运行，且没有同项目遗留的运行中 server 容器
docker compose ps server
docker ps --filter 'label=com.docker.compose.service=server' --format 'table {{.ID}}\t{{.Image}}\t{{.Status}}\t{{.Names}}'

# 所有旧实例均停止后，启动已构建的新版 server
docker compose up -d --no-deps server

# 恢复流量前检查状态、近期日志和健康接口
docker compose ps server
docker compose logs --since=10m server
curl --fail --silent --show-error http://127.0.0.1:8080/api/app/health
```

若业务必须采用旧版、新版实例混跑的零停机滚动发布，本版本的 advisory lock 方案**不保证混跑期间的会话唯一性**。在执行这种发布方式前，必须先单独设计和验证仅作用于 `xinzhili_voice` 的 partial unique 约束，以及历史重复会话的消息归并、去重和安全迁移方案；不得直接增加覆盖普通 `chat` 的全局唯一约束。

```bash
# 改了代码后重新构建对应服务
docker compose up -d --build admin      # 只更新后台
docker compose up -d --build website    # 只更新官网
docker compose up -d --build server     # 只更新后端（本版本须遵循上面的摘流停旧流程）

# 全部更新（如包含 server，仍须先完成上面的摘流、停旧和检查步骤）
docker compose up -d --build
```

## 三、域名 + HTTPS（生产建议）

容器只在宿主机 `127.0.0.1` 监听 HTTP。生产建议在最外层再加一个反向代理（宿主机 nginx 或 Traefik / Caddy）做域名分发与证书：

```
admin.example.com  → 127.0.0.1:8080
www.example.com    → 127.0.0.1:8000
```

此时无需改容器：后台/官网的 `/api` 已由各自容器内 nginx 反代到 server，外层只做 80/443 → 8080/8000 的转发即可。

### 芯之力语音流式接口的外层代理检查清单

芯之力的 `POST /api/app/xinzhili/turns/stream` 同时包含音频上传和 SSE 响应。容器内 nginx 已为该路径设置
`client_max_body_size 11m`、关闭请求/响应缓冲并使用 180 秒超时；宿主机 nginx、宝塔和 CDN 等外层代理也必须为同一精确路径配置：

- 为便于阅读和配置审计，约定将精确路由写在通用 `/api/` 规则前；nginx 的 exact location 匹配语义本身不依赖书写顺序。上传大小限制不得小于 **11 MiB**。
- nginx/宝塔关闭请求缓冲（`proxy_request_buffering off`），并关闭响应缓冲、缓存和压缩，避免该代理层先整包缓冲请求或聚合 SSE 增量。
- 保留 HTTP/1.1、清空上游 `Connection` 请求头，并将连接超时设置为 30 秒，读取和发送超时设置为至少 180 秒。
- CDN/WAF 必须确认支持流式请求体和 SSE；该路径绕过缓存后，再用实际音频验证上传与应用层解析完成后的首个 SSE 事件能及时到达。
- 发布后检查最终生效配置，而不只检查面板表单；nginx 可用 `nginx -T` 确认精确路由及其指令实际存在，并位于约定的审计位置。

宿主机 nginx 或宝塔可按下面模板配置；将 `<port>` 替换成该域名对应的容器入口端口（后台 `8080`，官网 `8000`）：

```nginx
location = /api/app/xinzhili/turns/stream {
    client_max_body_size 11m;
    proxy_pass http://127.0.0.1:<port>;
    proxy_http_version 1.1;
    proxy_set_header Connection "";
    proxy_request_buffering off;
    proxy_buffering off;
    proxy_cache off;
    gzip off;
    proxy_connect_timeout 30s;
    proxy_read_timeout 180s;
    proxy_send_timeout 180s;
    proxy_set_header Host $host;
    proxy_set_header X-Real-IP $remote_addr;
    proxy_set_header X-Forwarded-For $proxy_add_x_forwarded_for;
    proxy_set_header X-Forwarded-Proto $scheme;
}
```

这里的 `proxy_pass` 必须只有 `http://127.0.0.1:<port>`，不能附加 URI，也不能在端口后写尾随 `/`；否则 nginx 会把 exact location 的原始请求路径替换成 `/`，导致第二层 nginx 或 Go 无法匹配 `/api/app/xinzhili/turns/stream`。

关闭代理请求缓冲只消除当前代理层的整包缓冲，不代表全链路不使用临时文件，也不代表客户端上传过程中就会收到 SSE。Go handler 仍通过 `ParseMultipartForm` 解析请求，超过内存阈值的 multipart 内容可能写入系统临时目录，并在 handler 结束时调用 `MultipartForm.RemoveAll` 清理；首个 SSE 事件应在上传和应用层解析完成后验证。

如果某一层代理确实无法关闭请求缓冲，发布说明必须明确披露：音频会先完整写入该层临时目录，首个 SSE 事件会相应延迟；同时记录临时目录位置、磁盘容量告警和清理责任。nginx 正常完成或中止请求时会清理请求体临时文件；清理疑似遗留文件前必须先摘流/停止对应代理并确认没有活跃请求，禁止在运行中直接删除正在使用的临时文件。

## 四、常用运维

```bash
docker compose down            # 停止并移除容器（卷保留，数据不丢）
docker compose down -v         # 连卷一起删（用户/角色/菜单/官网配置全丢，慎用）
docker compose restart server  # 重启后端

# 备份站点配置
docker run --rm -v nine-xing_site-config:/data -v "$PWD":/backup alpine \
  cp /data/site-config.json /backup/site-config.backup.json

# 备份数据库
docker compose exec db pg_dump -U nx nx_admin > nx_admin.backup.sql

## Android App 版本发布

后台上传正式签名 APK 前，配置 `APP_RELEASE_PACKAGE_NAME` 和 APK 签名证书的 SHA-256 指纹
`APP_RELEASE_CERT_SHA256`。指纹是公开元数据；服务器不需要、也不应保存 keystore 或私钥。
可使用 Android build-tools 的 `apksigner verify --verbose --print-certs app-release.apk` 获取指纹。
指纹未配置时允许保存草稿，但发布会失败关闭。

外层 nginx、宝塔或 CDN 必须仅对 `/api/app-releases/upload` 允许 **301 MiB**，并关闭请求缓冲；
两个公开 APK 下载路径应关闭代理缓冲与缓存。普通 `/api/` 仍保留现有限制。

APK 文件持久化在 `/data/uploads/app-releases`。一致性备份时先停止写入和公开服务：

```bash
docker compose stop admin website server
mkdir -p backups
docker compose exec -T db sh -lc 'pg_dump -U "$POSTGRES_USER" -d "$POSTGRES_DB" -Fc' > backups/nx-admin.dump
docker compose run --rm --no-deps --user root -v "$PWD/backups:/backup" server sh -c 'tar -C /data/uploads -czf /backup/app-releases.tgz app-releases'
sha256sum backups/nx-admin.dump backups/app-releases.tgz > backups/SHA256SUMS
```

恢复时数据库和 `app-releases.tgz` 必须使用同一批备份。恢复文件后执行
`docker compose run --rm --no-deps --user root server chown -R app:app /data/uploads`，再启动服务并下载
当前正式版本，对比 API、数据库和 APK 文件的 SHA-256。历史归档包会永久保留，除非运维明确清理。
```

## 五、关键配置位置

| 配置 | 文件 |
|------|------|
| 后台接口地址 | `nx-backend/apps/web-antd/.env.production` → `VITE_GLOB_API_URL=/api` |
| 后台缓存命名空间/密钥 | `nx-backend/apps/web-antd/.env` |
| 官网接口地址 | `website-react/.env.production` → `VITE_API_BASE_URL=/api` |
| 后端账号/密钥/端口/数据库 | `docker-compose.yml` 的 `server.environment` 或根 `.env` |
| 数据库账号 | 根 `.env` 的 `POSTGRES_*` |
| 人声管理 / MiniMax | 根 `.env` 的 `MINIMAX_API_KEY` / `MINIMAX_GROUP_ID` / `MINIMAX_API_BASE` |
| 上传配置 | 根 `.env` 的 `OSS_*` / `UPLOAD_MAX_MB` |
| 站点默认配置（首启播种） | `shared/site-config.json` |
| 数据库表结构/初始数据 | `nx-backend/apps/server/internal/db/` |

## 六、上传与人声管理排查

人声管理的流程是：后台先请求 `POST /api/upload?dir=voice/samples` 上传音频，上传成功后再调用 MiniMax 做声音克隆。

如果服务器提示“内部服务错误”或“音频上传失败”，优先按下面顺序查：

```bash
# 1) 确认 server 容器拿到了关键环境变量
docker compose exec server env | grep -E 'MINIMAX|OSS|UPLOAD'

# 2) 看后端真实错误
docker compose logs -f server

# 3) 如果走外层 nginx / 宝塔 / CDN，确认上传大小限制不小于 UPLOAD_MAX_MB
# 容器内 nginx 默认 client_max_body_size=50m，Go 默认 UPLOAD_MAX_MB=20。
```

常见原因：

- `MINIMAX_API_KEY` 没有传入 `server` 容器：声音克隆会失败。
- `MINIMAX_API_BASE` 配错：中文站 Key 应使用 `https://api.minimaxi.com`。
- OSS 只填了一部分：要么补齐 `OSS_ACCESS_KEY_ID`、`OSS_ACCESS_KEY_SECRET`、`OSS_BUCKET`、`OSS_REGION`，要么清空所有 `OSS_*` 使用本地上传卷。
- 外层 nginx / 面板限制太小：音频文件可能在进入容器前就被拒绝。
- 服务器数据库未迁移到最新版本：重建/更新 `server` 后会自动执行 schema；若仍报错，查看 `docker compose logs server` 的数据库错误。

## 七、本地非容器调试

需要本地有一个 PostgreSQL（或先 `docker compose up -d db` 只起数据库）。

```bash
# 后端（连本地或容器里的 db）
cd nx-backend/apps/server
APP_ENV=dev DATABASE_URL='postgres://nx:nx@localhost:5432/nx_admin?sslmode=disable' go run ./cmd/server
# 后台（另开终端）
cd nx-backend && pnpm dev:antd
# 官网（另开终端）
cd website-react && npm run dev
```

三者的 `/api` 在 dev 下都会代理到本地 `:5320`。
