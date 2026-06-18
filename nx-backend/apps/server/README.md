# Nine Xing Vben Go Server

适配 Vben Admin 5.7.0 的最小 Go 服务端。

## 接口

- `POST /api/auth/login`
- `POST /api/auth/logout`
- `POST /api/auth/refresh`
- `GET /api/user/info`
- `GET /api/auth/codes`
- `GET /api/menu/all`
- `POST /api/upload` — 认证后上传 `multipart/form-data` 文件字段 `file`，可选 query `dir`
- `GET /api/site-config`
- `PUT /api/site-config`  — 保存配置；成功后自动触发官网重新构建（若已配置 `BUILD_SCRIPT`）
- `GET /api/site-config/build-status`  — 轮询官网构建状态（`idle/pending/building/success/failed/disabled`）

响应结构与 Vben 默认请求拦截器一致：

```json
{
  "code": 0,
  "data": {},
  "error": null,
  "message": "ok"
}
```

## 启动

```bash
cd nx-backend/apps/server
PORT=5320 SITE_CONFIG_PATH=/Users/wohenzaiyi/Desktop/nine-xing/shared/site-config.json go run ./cmd/server
```

启用阿里云 OSS 上传需要配置：

```bash
OSS_ACCESS_KEY_ID=...
OSS_ACCESS_KEY_SECRET=...
OSS_BUCKET=...
OSS_ENDPOINT=oss-cn-hangzhou.aliyuncs.com
OSS_REGION=cn-hangzhou
OSS_PUBLIC_URL=https://你的访问域名
OSS_PREFIX=uploads
UPLOAD_MAX_MB=20
```

上传成功响应中的 `data.url` 会写入站点配置里的图片路径字段。

默认账号：

```text
admin / 123456
```

## 测试

```bash
go test ./...
```
