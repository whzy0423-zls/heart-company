# heart-company

## 项目概览

`heart-company` 是一个官网 + 后台管理 + Go 服务端的整合项目。

- `website-react/`：React 官网，默认运行在 `http://localhost:8000`。
- `nx-backend/apps/web-antd/`：Vben Admin 后台管理，默认运行在 `http://localhost:8080`。
- `nx-backend/apps/server/`：Go API 服务，容器内端口 `5320`，由官网和后台通过 `/api` 反向代理访问。
- `shared/site-config.json`：官网默认配置数据。

后台主要用于管理官网配置、系统用户/角色/菜单、客户报名信息、后台品牌信息和图片上传。

## 本地启动

先复制环境变量文件：

```bash
cp .env.example .env
```

然后按需修改 `.env` 中的管理员密码、数据库密码、JWT 密钥和 OSS 配置。

启动全部服务：

```bash
docker compose up -d --build
```

访问地址：

- 官网：`http://localhost:8000`
- 后台管理：`http://localhost:8080`

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

- `JWT_SECRET`：JWT 签名密钥，生产环境必须换成强随机串。
- `ADMIN_USERNAME` / `ADMIN_PASSWORD`：后台管理员初始账号密码。
- `POSTGRES_USER` / `POSTGRES_PASSWORD` / `POSTGRES_DB`：PostgreSQL 配置。
- `OSS_ACCESS_KEY_ID` / `OSS_ACCESS_KEY_SECRET` / `OSS_BUCKET` / `OSS_ENDPOINT` / `OSS_REGION`：阿里云 OSS 上传配置。
- `OSS_PUBLIC_URL`：OSS 或 CDN 公网访问域名。
- `OSS_PREFIX`：上传文件在 OSS 中的目录前缀，默认 `uploads`。
- `UPLOAD_MAX_MB`：单文件上传大小限制。

注意：`.env` 包含真实密钥，已经在 `.gitignore` 中忽略，不能提交到仓库。

## 图片上传与预览

后台图片上传会先写入 OSS，并在数据库 `upload_assets` 表中记录上传资产。

由于 OSS bucket 可能是私有读，前端展示使用后端代理地址：

```text
/api/upload-assets/{id}
```

这样后台和官网都可以稳定预览图片。真实 OSS 地址会保存在数据库的 `object_url` 字段中，便于后续追踪和迁移。

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

本仓库地址：

```text
https://github.com/whzy0423-zls/heart-company.git
```

提交前请确认以下内容不要进入仓库：

- `.env`
- `node_modules/`
- `dist/`
- `.DS_Store`
- 本地工具配置目录，如 `.claude/`、`.codex/`

