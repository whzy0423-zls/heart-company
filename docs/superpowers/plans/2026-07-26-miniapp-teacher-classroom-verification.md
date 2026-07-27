# 老师课堂 Task 12 发布验收记录

- 验收日期：2026-07-27（Asia/Shanghai）
- 分支：`feature/miniapp-teacher-classroom`
- 验收起点：`2c5844ff fix: guard classroom purchase lifecycle`
- 生产小程序 API：`https://xn--9iq9az5uo8fz16d.com/api`
- 生产微信 AppID：`wx7d12bddbec8e17f7`

## 1. 小程序发布产物

| 检查 | 命令/证据 | 结果 |
| --- | --- | --- |
| 全套配置与运行态回归 | `cd miniapp && npm run test:config` | 通过；新增 `scripts/release-regression-flow.test.mjs` 也在门禁内 |
| 本地微信构建 | `VITE_API_BASE=http://127.0.0.1:5320/api npm run dev:mp-weixin` | 输出 `DONE Build complete. Watching for changes...`；检查完产物后主动 `Ctrl-C` 终止 watcher |
| 本地产物路由 | `dist/dev/mp-weixin/app.json` | 主包 4 页；课堂列表和课件详情均为独立分包 |
| 本地产物页面 | `find dist/dev/mp-weixin/pages/classroom* -type f` | 两个页面的 `.js/.json/.wxml/.wxss` 均存在 |
| 本地产物体积 | `du -sk dist/dev/mp-weixin` | 928 KiB；课堂列表 40 KiB，详情 44 KiB |
| 本地 API 注入 | `grep -RIl 'http://127.0.0.1:5320/api' dist/dev/mp-weixin` | 仅生成的 `config.js` 命中 |
| 生产微信构建 | `VITE_API_BASE=https://xn--9iq9az5uo8fz16d.com/api npm run build:mp-weixin` | prebuild、编译、postbuild 全部通过 |
| 生产验证器 | `node scripts/verify-production-api-base.mjs`；`node scripts/verify-built-wechat-appid.mjs` | 真实 HTTPS API、AppID、无占位域名均通过 |
| 生产路由/AppID | 解析 `dist/build/mp-weixin/app.json`、`project.config.json` | 课堂两分包存在；AppID 为 `wx7d12bddbec8e17f7` |
| 生产包体 | `du -sk dist/build/mp-weixin` | 652 KiB；课堂两个分包各 28 KiB |
| 禁止主机 | 扫描 localhost、私网/占位 API host | 无命中 |
| 永久媒体 URL | 扫描 `aliyuncs.com/myqcloud.com/amazonaws.com/myhuaweicloud.com` 及 OSS/COS/S3/OBS URL | 开发和生产产物均无命中 |

`app.json` 的课堂路由为：

```text
pages/classroom/classroom
pages/classroom-detail/classroom-detail
```

## 2. 后台与服务端门禁

| 门禁 | 命令 | 结果 |
| --- | --- | --- |
| 指定后台测试 | `pnpm exec vitest run apps/web-antd/src/views/miniapp/home.test.ts apps/web-antd/src/views/classroom/classroom.test.ts apps/web-antd/src/views/site-config/teacher.test.ts apps/web-antd/src/views/site-config/courses.test.ts` | 4 files、37 tests 通过 |
| Web 类型检查 | `pnpm run check:type --filter=@vben/web-antd` | 1/1 task 通过 |
| Web 生产构建 | `pnpm run build:antd` | 11/11 tasks 通过；质量修复后的 fresh 构建生成 `apps/web-antd/dist.zip`（1,575,614 bytes） |
| Go 全仓 | `cd apps/server && go test -count=1 ./...` | 全部通过 |
| 课堂纵向合同 | `go test ./internal/server -run ClassroomVerticalContract -count=1 -v` | `TestClassroomVerticalContractAdminPublishToPublicPlayback` 通过 |
| Race | `go test -race ./internal/classroom ./internal/server ./internal/storage` | 全部通过 |
| Vet | `go vet ./...` | 通过，无输出 |
| Go 格式 | `gofmt` 后 `gofmt -l` 检查本次 Go 文件 | 无输出 |
| 前端格式 | `oxfmt --check` 检查本次 JSON/MJS loader 与 regression 文件；Vue 以原始格式保留并检查最小语义 diff | 通过 |
| 空白错误 | `git diff --check` | 通过 |

### Typecheck RED → GREEN

首次全量 typecheck 确认以下真实门禁失败：

- `customer/miniapp-users.vue`：未使用的 `Space`，以及 Ant Table slot 的 `record` 类型不匹配。
- `miniapp/home.vue`：同一 SFC 两个 script block 重复导入 `MiniappHomeIconKey/MiniappHomeThemeKey`。

完成最小类型修复后，`vue-tsc --noEmit --skipLibCheck` 退出码为 0，生产构建也通过。

质量复审时已撤回 `miniapp-users.vue` 的整文件格式化；相对原文件只保留删除未使用导入，以及 `unknown` record ID 校验后从 typed source list 重新取值的最小语义 diff，不再使用 `Record<string, any>` 和强制类型断言。

## 3. 场景矩阵

| 场景 | 权威可执行证据 | 结论 |
| --- | --- | --- |
| 上传成功 | `TestClassroomUploadCompleteValidatesPartsHeadChecksumAndProbe`、`TestClassroomUploadTransitionsMediaProcessingThenReadyWithCover` | 通过 |
| 上传失败 | `TestClassroomUploadCompleteMarksFailedOnHeadOrProbeMismatch`、`TestClassroomUploadFailureWritesFailedMediaAndContent` | 通过 |
| 上传 abort | `TestClassroomUploadAbortIsIdempotentAndOwned`、两实例 abort 幂等测试 | 通过 |
| 上传 expiry | `TestClassroomUploadCleanupExpiredAbortsOrphans`、过期 completing 维护测试 | 通过 |
| 非法媒体 | `TestValidateMediaProbeSupportsRequiredFormats`、`TestClassroomUploadRequiresChecksumAndAAC`、MP3 codec 校验测试 | 通过 |
| draft/publish/offline | `TestClassroomAdminPublishOfflineAndDraftCRUD`、系列/课件状态转换测试 | 通过 |
| hard-stop | `TestClassroomPlaybackRejectsParentHardBlock`、public parent hard block 测试 | 通过 |
| anonymous/login/member/paid | `TestClassroomPublicEffectiveAccessAndPurchaseStates`、播放访问矩阵测试 | 通过 |
| series/content purchase | `TestClassroomOrderRoutesCreateSeriesAndReadContentStatus`、系列/单课 entitlement 测试 | 通过 |
| callback duplicate | `TestMarkOrderPaidDuplicateClassroomCallbackIsIdempotent` | 通过 |
| refund | `TestRefundClassroomOrderRevokesOnlyItsEntitlementAndWritesAudit` | 通过 |
| signed URL expiry | `TestClassroomAnonymousTicketExpiresAndRefreshesThroughNoStoreEndpoint`、跨内容/媒体版本 replay 测试 | 通过 |
| Range/seek | `TestOSSPlaybackPresignLeavesRangeHeaderUnsignedForMediaSeeking`、`TestOSSClassroomPlaybackPresignedURLServesByteRange`；小程序播放器 seek/timeupdate 合同 | 独立 OSS V4 verifier 接受生产签名和动态 Range，返回 206；篡改签名及错误 Range 约束均被拒绝 |
| progress throttle/flush | `classroomProgress.test.mjs`、`classroom-progress-order.test.mjs`、详情页 hide/unload 测试、服务端限流测试 | 通过 |
| cache invalidation | `TestClassroomPublicContentCacheInvalidatesOnHiddenMediaVersion`、`TestClassroomPublicListCacheInvalidatesOnPriceAndOfflineVisibilityChanges` | 旧 ETag 在媒体版本、价格或可见性变化后返回新 200；内部版本不泄漏 |
| permission denial | `TestClassroomUploadRoutesRequireDedicatedPermission`、`TestClassroomAdminListPublishAndPriceRoutesStopAtPermissionDenial` | list/publish/price/upload 均在业务 handler 前 403 |

### Cache invalidation TDD

`TestClassroomPublicContentCacheInvalidatesOnHiddenMediaVersion` 首次运行因生产 DTO 没有隐藏 cache version 而 RED。实现将媒体 ETag 只纳入缓存指纹、不写入 JSON DTO 后：

- 媒体版本改变会生成新 HTTP ETag；
- 旧 `If-None-Match` 得到 200；
- 响应仍不包含 media ETag、object key 或永久 URL。

### Range/206 TDD

`TestOSSClassroomPlaybackPresignedURLServesByteRange` 使用生产 `OSSUploader.PresignGetURL` 和课堂纵向合同中的对象路径 `classroom/private/content-21.mp4` 生成短时签名地址，再向本地可重复的 OSS-compatible integration fixture 发出 Range GET。Fixture 独立解析 credential/scope/date/expiry/additional headers，重建 canonical URI、canonical query、canonical request、string-to-sign 和 `aliyun_v4_request` HMAC signing key，并以常量时序比较签名：

- RED 1：origin 忽略 Range 时返回 200 和完整正文，测试按预期失败；
- RED 2：切换到独立签名 verifier 前，测试因缺少 `verifyOSSQueryV4` 编译失败；
- GREEN：原始生产签名通过验证，`Range: bytes=4-8` 返回 206；
- `Content-Range` 精确为 `bytes 4-8/16`；
- 响应正文精确为 `45678`；
- 同一生产签名改发 `bytes=9-11` 仍返回 206、`bytes 9-11/16` 和 `9ab`，证明 Range 保持动态；
- 篡改 `x-oss-signature` 返回 403；控制组故意把 Range 纳入 `x-oss-additional-headers` 并重签后，原 Range 可用但改变 Range 返回 403，证明错误签名约束会被测试捕获。

该 fixture 验证本仓库 OSS SDK 签名和 HTTP Range 契约，不声称替代真实阿里云 Bucket 验证；上线环境仍需执行文末 lifecycle/CORS/实际对象抽查。

## 4. 既有功能回归

| 流程 | 可执行证据 |
| --- | --- |
| 微信登录 | `TestWxLoginUsesMiniappUserServiceAndKeepsResponseCompatible`；小程序 auth/request 测试 |
| expired token | `TestClassroomPublicMetadataRejectsExpiredMiniappJWT` 明确断言 401 且 `GetContent` 调用数为 0；API stale JWT 自动清理并匿名重试测试 |
| test → result → report | `release-regression-flow.test.mjs` 通过 compiler-sfc loader mount 真实生产 `test.vue/result.vue`：测试页实际写 session/storage，结果页消费同一数据，并经共享有状态 request mock 完成匿名上报、存档、报告状态和正文加载；断言生产 import 身份、session schema 与 report DTO；服务端 test-record/report tests |
| relation | `release-regression-flow.test.mjs` 执行选择、分析、结果和 reset；`TestAppCompatibilityCreateReturnsReportWithCompatibleFieldNames` 与 list/detail scope 测试 |
| booking draft/submit | `release-regression-flow.test.mjs` 执行草稿恢复、登录、提交、清草稿和表单复位；`TestMiniappBookingsPostUsesTransactionalServiceBroadcastsAfterSuccessAndReturnsBooking` |
| booking records/detail | `booking-records.session.test.mjs`、`booking-detail.session.test.mjs`、booking display/session tests |
| profile editing | `release-regression-flow.test.mjs` 执行加载、标准化、保存及成功反馈；`TestWxUserInfoPutPersistsProfileAndReturnsUpdatedDTO` 验证服务端 PUT 持久化和 DTO |
| homepage config | 指定 `home.test.ts`，miniapp `siteConfig/homeCarousel/homeMenu` tests |
| learning cache silent refresh | `learn.content-state.test.mjs`、site config cache tests |
| legacy course fallback | `teacherCourseware.test.mjs`、后台 teacher/courses tests、shared fixture/vertical contract |

## 5. 发布审计

### 数据库与回滚

- 课堂表、约束与索引由幂等 `schema.sql` 管理；`schema_classroom_test.go` 验证依赖顺序、升级旧 status constraint、上传状态与 entitlement renewal/index。
- 本次是 additive schema。首选回滚为回退应用版本并保留课堂表，避免破坏订单、权益、学习进度和审计数据。
- 若确认永久卸载且完成数据导出及 OSS 清理，破坏性删除顺序必须是：`classroom_progress` → `classroom_entitlements` → `classroom_upload_tasks` → 移除 content-media FK/`classroom_contents` → `classroom_media_assets` → `classroom_series`。发布回滚不执行此破坏性路径。

### OSS orphan cleanup

- `CleanupPending` 在启动时和每 15 分钟运行，覆盖 expired/failed/aborted/stale-cleaning。
- 清理会 Abort multipart，并删除已完成对象、抽取封面和 failure trace 中的封面；失败记录保留为可重试。
- `docs/deployment/classroom-media.md` 要求 Bucket 配置 `classroom/` 前缀、`AbortMultipartUpload`、Days=1 的生命周期兜底。

### 审计记录

- 系列/课件 create/update/delete/publish/offline/price/hard-stop 写操作均调用课堂审计。
- `TestClassroomAuditCapturesActorActionObjectAndReason` 验证 operator/action/target/reason；手工 entitlement grant/revoke 与审计同事务。

### Secrets / CORS

- OSS AK/SK 只由服务端环境读取；浏览器只获得短时 multipart 或 playback URL。
- 生产环境校验拒绝弱 JWT、弱管理员密码、弱数据库凭据、dev 微信登录/支付和非公网外部 API base。
- Bucket CORS 只允许实际后台 origin；API CORS 通过 allowlist，缓存响应合并 `Vary: Origin, Authorization`。

### 泄漏与仓库卫生

- public DTO allowlist 与测试禁止 `objectKey`、upload ID、永久媒体 URL。
- 生产小程序构建扫描未发现永久 OSS/COS/S3/OBS URL、localhost 或占位域名。
- conflict marker 扫描和 `git diff --check` 必须在交接提交前再次执行。

## 6. 已知环境说明

- 当前 Node 为 `v25.9.0`，仓库声明 Node 22/24；pnpm 会输出 engine warning，但 typecheck、Vitest 和 build 均成功。
- Node 运行 `reportDisplayState.js` 时输出 typeless-package warning；测试成功。
- uni-app 开发/生产构建输出旧依赖 circular warning 和可更新版本提示；构建成功。
- Rolldown 对第三方 `@vueuse/core` 的 PURE annotation 输出 warning；后台生产构建成功。
- OSS 生命周期规则属于部署环境配置。代码、维护任务、自动化测试和操作文档已验收；上线人员仍应按文档执行 `lifecycle get` 并在真实 Bucket 观察一次未完成 multipart 自动消失。
