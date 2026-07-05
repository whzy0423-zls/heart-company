# nine-xing

## 项目概览

`nine-xing` 是「九型芯之力」官网、阅读 H5、微信小程序、后台管理与 Go 服务端的整合项目。

- `website-react/`：React 官网，Docker/生产入口默认是 `http://localhost:8000`。
- `reading-h5/`：React 阅读 H5，展示后台发布的文章、封面与听书音频。
- `miniapp/`：uni-app + Vue3 微信小程序/H5，包含测评、AI 对话、学习、合盘、预约、我的档案等端侧功能。
- `nx-backend/apps/web-antd/`：Vben Admin 后台管理，Docker/生产入口默认是 `http://localhost:8080`，本地 Vite 开发入口默认是 `http://localhost:5666`。
- `nx-backend/apps/server/`：Go API 服务，容器内端口 `5320`，由官网和后台通过 `/api` 反向代理访问。
- `shared/site-config.json`：官网默认配置数据。

后台主要用于管理官网配置、阅读文章、人声/视频/模型配置、系统用户/角色/菜单、客户报名信息、App 用户、用户提炼数据、推送管理、后台品牌信息和图片上传。后端同时提供 App 用户画像/测评/记忆/成长报告、JPush 设备注册与推送、公开资源安全代理、微信支付回调、隐私导出/清除/注销等接口。

## 本地启动

先复制环境变量文件：

```bash
cp .env.example .env
```

然后修改 `.env` 中的 `JWT_SECRET`、`ADMIN_PASSWORD`、`POSTGRES_PASSWORD` 等生产必填项；OSS、微信支付、JPush、模型与短信配置按实际功能开启。

启动全部服务：

```bash
docker compose up -d --build
```

Docker Compose 访问地址：

- 官网：`http://localhost:8000`
- 后台管理：`http://localhost:8080`

Docker Compose 默认把官网/后台端口绑定到宿主机 `127.0.0.1`，生产公网访问请使用宿主机 Nginx/Caddy/Traefik 反代到上述本机端口。

如果是本地开发方式启动后台（`cd nx-backend && pnpm dev:antd`），请访问：

- 后台管理开发地址：`http://localhost:5666`
- 后端 API：`http://localhost:5320/api/status`

查看服务状态：

```bash
docker compose ps
```

停止服务：

```bash
docker compose down
```

## 环境变量

关键环境变量在 `.env.example` 中有示例。

- `APP_ENV`：必须显式配置为 `dev` / `test` / `staging` / `production`，生产建议固定 `production`。
- `JWT_SECRET`：JWT 签名密钥，生产环境必须换成强随机串。
- `ADMIN_USERNAME` / `ADMIN_PASSWORD`：后台管理员初始账号密码。
- `POSTGRES_USER` / `POSTGRES_PASSWORD` / `POSTGRES_DB`：PostgreSQL 配置。
- `OSS_ACCESS_KEY_ID` / `OSS_ACCESS_KEY_SECRET` / `OSS_BUCKET` / `OSS_ENDPOINT` / `OSS_REGION`：阿里云 OSS 上传配置。
- `OSS_PUBLIC_URL`：OSS 或 CDN 公网访问域名。
- `OSS_PREFIX`：上传文件在 OSS 中的目录前缀，默认 `uploads`。
- `UPLOAD_MAX_MB`：单文件上传大小限制。
- `VIDEO_API_BASE` / `IMAGE_API_BASE`：生产下只要对应 API Key 非空，就必须显式配置公网 API 地址。
- `PUBLIC_BASE_URL`：启用视频网关时必须配置公网 HTTPS API 根地址，用于外部服务回调/拉取本地相对资源。
- `JPUSH_APP_KEY` / `JPUSH_MASTER_SECRET`：启用 JPush 推送时配置。

注意：`.env` 包含真实密钥，已经在 `.gitignore` 中忽略，不能提交到仓库。

## 图片上传与预览

后台图片上传会写入 OSS；未配置 OSS 时写入本地上传卷，并在数据库 `upload_assets` 表中记录上传资产。

由于 OSS bucket 或本地上传目录可能是私有读，后台预览使用带鉴权的后端代理地址：

```text
/api/upload-assets/{id}
```

官网、阅读 H5、登录前后台品牌会由公开只读接口把“已被配置引用”的上传资源重写为专用 public 代理地址，例如 `/api/public/site-assets/{id}`、`/api/public/article-assets/{id}`。真实 OSS 或本地对象地址会保存在数据库的 `object_url` 字段中，便于后续追踪和迁移。

## 官网配置

后台“官网管理”保存配置后，Go 服务会把配置写入数据库/配置存储。官网运行时通过：

```text
/api/public/site-config
```

读取最新配置。

如果只改 `shared/site-config.json`，需要重新构建或重新初始化相关配置；运行中的官网优先读取服务端公开接口返回的数据。

## 报名信息

官网报名表提交到：

```text
POST /api/public/signups
```

后台在客户管理菜单下查看报名数据。手机号模式会校验手机号格式；微信号模式只要求填写内容。

## Git 与安全

提交前请确认以下内容不要进入仓库：

- `.env`
- `node_modules/`
- `dist/`
- `.DS_Store`
- 本地工具配置目录，如 `.claude/`、`.codex/`
