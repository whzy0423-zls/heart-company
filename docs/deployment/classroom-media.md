# 老师课堂媒体上传部署

老师课堂视频/音频使用私有 OSS Bucket 的 multipart 直传，不复用通用 `/api/upload`（20MiB）接口。

## 环境变量

服务端读取以下课堂媒体配置：

- `CLASSROOM_MEDIA_ENDPOINT`：OSS endpoint；为空时回退 `OSS_ENDPOINT`
- `CLASSROOM_MEDIA_BUCKET`：私有 Bucket；为空时回退 `OSS_BUCKET`
- `CLASSROOM_MEDIA_REGION`：Bucket 所在地域；为空时回退 `OSS_REGION`
- `CLASSROOM_MEDIA_PART_SIZE_MB`：分片大小，默认 8MiB
- `CLASSROOM_MEDIA_MAX_PARTS`：最大分片数，默认 10000
- `CLASSROOM_MEDIA_CREDENTIAL_TTL_SECONDS`：分片签名有效期，默认 900 秒
- `CLASSROOM_MEDIA_MAX_VIDEO_MB`：视频大小上限，默认 4096MiB
- `CLASSROOM_MEDIA_MAX_AUDIO_MB`：音频大小上限，默认 512MiB

现有 `OSS_ACCESS_KEY_ID`、`OSS_ACCESS_KEY_SECRET` 仅由服务端读取，浏览器只获得短时分片签名 URL。

## Bucket CORS

在 OSS 控制台为私有 Bucket 配置 CORS（不要将 CORS 写入环境变量或代码）：

- Allowed origins：后台实际域名（开发时可加入 `http://localhost:5173`）
- Allowed methods：`GET`, `HEAD`, `PUT`, `POST`
- Allowed request headers：`*`（生产可收窄为 `Content-Type`, `Content-Length`, `Content-MD5`, `x-oss-*`）
- Exposed response headers：`ETag`, `x-oss-hash-crc64ecma`
- 是否允许凭证：按后台域名认证方案设置；签名 URL 本身不依赖 Cookie

## Bucket 未完成分片生命周期（上线必配）

数据库会先写入 `initiating` 预占，再请求 OSS 创建 multipart。若进程在 OSS 已创建 upload、但真实 upload ID 尚未确认回数据库的极小窗口内硬退出，应用维护任务只能看到预占 ID，无法定位该次 OSS multipart。因此私有课堂 Bucket 必须配置 OSS 生命周期规则，自动终止未完成的分片上传。

- Prefix：`classroom/`
- 动作：`AbortMultipartUpload`
- Days：`1`（最长不要超过 3 天）
- Status：`Enabled`

生命周期 XML 示例：

```xml
<LifecycleConfiguration>
  <Rule>
    <ID>abort-incomplete-classroom-multipart</ID>
    <Prefix>classroom/</Prefix>
    <Status>Enabled</Status>
    <AbortMultipartUpload>
      <Days>1</Days>
    </AbortMultipartUpload>
  </Rule>
</LifecycleConfiguration>
```

可在 OSS 控制台的 Bucket 生命周期页面配置；使用 `ossutil` 时，先按当前客户端版本执行 lifecycle put，再执行 lifecycle get，确认返回内容同时包含 `classroom/`、`AbortMultipartUpload` 和 `Days=1`。部署验收还需创建一个测试 multipart、保持未完成，并在生命周期窗口后确认它已从未完成上传列表消失。

## 上传与校验流程

1. 后台调用 `POST /api/admin/classroom/uploads/initiate`，任务绑定一个草稿课件。
2. 服务端生成 `classroom/{video|audio}/YYYY/MM/DD/content-{id}/...` object key，并创建 multipart upload。
3. 后台按分片请求 `.../{taskId}/parts/{partNumber}/sign`，浏览器直接 PUT 到 OSS。
4. `.../{taskId}/complete` 会重新列出分片，校验 ETag/总大小，再执行 Complete、HeadObject、checksum 校验和媒体 probe。前端应提交 `crc64:<value>` 作为 expected checksum，服务端会与 OSS Complete/Head 实际返回的 CRC64 比对。
5. 仅 MP4(H.264/AAC)、MP3、M4A(AAC) 进入 `ready`；失败任务标记 `failed`，可在达到重试上限前重新上传。
6. 取消使用 `.../{taskId}/abort`；完成和取消均幂等。

## 运行检查

```bash
curl -s "$API/api/health" >/dev/null
# 登录后台后，确认 OPTIONS/PUT 预检响应包含 Access-Control-Allow-Origin、Access-Control-Expose-Headers: ETag
```

定期清理过期或待补偿的上传任务，并调用 OSS AbortMultipartUpload；数据库只保存媒体元数据，不保存长视频二进制。Bucket 生命周期是应用维护任务的兜底，不能省略。

## 运行依赖与清理调度

- API 服务主机必须安装 `ffprobe` 与 `ffmpeg`。视频 probe 通过后，服务端使用 `ffmpeg` 在约 1 秒处抽取 JPEG 封面并上传到同一私有课堂 Bucket。
- API 服务启动后会立即运行一次课堂上传维护任务，并每 15 分钟调用 `CleanupPending(ctx, limit)`。维护任务会处理过期的 `initiating/initiated/uploading/completing`、租约超时的 `cleaning`，以及 `failed/expired/aborted + cleanup pending/failed` 记录：中止 multipart、删除完整对象与抽取封面，成功写入 `cleanup_status=cleaned`，失败保留原 object key/upload id 并在下轮重试。
- `MaxAttempts` 表示实际发起 multipart upload 的次数；一次失败不会额外消耗次数，只有重新 initiate 才会递增。
