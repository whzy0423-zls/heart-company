# 部署说明（Docker Compose 三容器）

整套包含四个服务，由仓库根的 `docker-compose.yml` 编排：

| 服务 | 内容 | 宿主机端口 | 说明 |
|------|------|-----------|------|
| `db` | PostgreSQL 16 | 不对外 | 用户/角色/菜单持久化，数据存卷 `pgdata` |
| `server` | Go 后台服务 | 不对外 | 仅集群内 `:5320`，经各 nginx 反代 `/api` |
| `admin` | 后台管理(web-antd) | `8080` | 静态页 + 反代 `/api` 到 server |
| `website` | 官网(website-react) | `8000` | 静态页 + 反代 `/api/public/*` 到 server |

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

首次启动时 server 会自动建表并播种一个超级管理员（账号取 `ADMIN_USERNAME`，密码 `ADMIN_PASSWORD`，bcrypt 加密入库）。之后即可在「系统管理」里增删用户/角色/菜单，全部落库永久生效。

---

## 一、首次部署

在仓库根 `nine-xing/` 下：

```bash
# 1) 设置生产密钥（强烈建议，不设则用不安全的默认值）
export JWT_SECRET="$(openssl rand -hex 32)"
export ADMIN_USERNAME="admin"
export ADMIN_PASSWORD="你的强密码"

# 2) 构建并启动
docker compose up -d --build

# 3) 查看状态/日志
docker compose ps
docker compose logs -f server
```

启动后：
- 后台管理：http://服务器IP:8080 （账号见上面的 ADMIN_*）
- 官网：http://服务器IP:8000

> 也可把上面的环境变量写进仓库根的 `.env` 文件（compose 会自动读取），避免每次 export。

## 二、更新发布

```bash
# 改了代码后重新构建对应服务
docker compose up -d --build admin      # 只更新后台
docker compose up -d --build website    # 只更新官网
docker compose up -d --build server     # 只更新后端

# 全部更新
docker compose up -d --build
```

## 三、域名 + HTTPS（生产建议）

容器只监听 HTTP。生产建议在最外层再加一个反向代理（宿主机 nginx 或 Traefik / Caddy）做域名分发与证书：

```
admin.example.com  → 127.0.0.1:8080
www.example.com    → 127.0.0.1:8000
```

此时无需改容器：后台/官网的 `/api` 已由各自容器内 nginx 反代到 server，外层只做 80/443 → 8080/8000 的转发即可。

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
```

## 五、关键配置位置

| 配置 | 文件 |
|------|------|
| 后台接口地址 | `nx-backend/apps/web-antd/.env.production` → `VITE_GLOB_API_URL=/api` |
| 后台缓存命名空间/密钥 | `nx-backend/apps/web-antd/.env` |
| 官网接口地址 | `website-react/.env.production` → `VITE_API_BASE_URL=/api` |
| 后端账号/密钥/端口/数据库 | `docker-compose.yml` 的 `server.environment` 或根 `.env` |
| 数据库账号 | 根 `.env` 的 `POSTGRES_*` |
| 站点默认配置（首启播种） | `shared/site-config.json` |
| 数据库表结构/初始数据 | `nx-backend/apps/server/internal/db/` |

## 六、本地非容器调试

需要本地有一个 PostgreSQL（或先 `docker compose up -d db` 只起数据库）。

```bash
# 后端（连本地或容器里的 db）
cd nx-backend/apps/server
DATABASE_URL='postgres://nx:nx@localhost:5432/nx_admin?sslmode=disable' go run ./cmd/server
# 后台（另开终端）
cd nx-backend && pnpm dev:antd
# 官网（另开终端）
cd website-react && npm run dev
```

三者的 `/api` 在 dev 下都会代理到本地 `:5320`。
