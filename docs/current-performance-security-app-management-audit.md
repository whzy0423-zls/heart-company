# 当前项目启动性能、防御与 App 后台管理审计

审计日期：2026-07-04  
主项目：`/Users/wohenzaiyi/Desktop/nine-xing`  
App 项目：`/Users/wohenzaiyi/Desktop/nine-xing-app`

## 1. 审计范围

本次检查覆盖：

- 后台管理端 `nx-backend/apps/web-antd`
- Go API 服务 `nx-backend/apps/server`
- 官网 `website-react`
- 阅读 H5 `reading-h5`
- Flutter App `nine-xing-app`
- Docker Compose 暴露面与生产配置
- App 侧接口是否已有后台管理入口

未执行破坏性操作；未提交 Git。

## 2. 实测启动与构建性能

本机命令环境需要补充 Homebrew 路径：

```bash
export PATH=/opt/homebrew/bin:/usr/local/go/bin:$PATH
```

| 模块 | 验证命令 / 场景 | 结果 |
|---|---:|---|
| 后台管理开发启动 | `pnpm -F @vben/web-antd run dev` | Vite ready 约 2.96s |
| 官网开发启动 | `npm run dev` | Vite ready 约 0.37s |
| 阅读 H5 开发启动 | `npm run dev` | Vite ready 约 0.37s |
| 后台管理生产构建 | `pnpm -F @vben/web-antd run build` | 6.97s；`dist` 5.1M；`dist.zip` 2.0M |
| 官网生产构建 | `npm run build` | 2.01s；但 `dist` 约 232M |
| 阅读 H5 构建 | `npm run build` | 0.91s；`dist` 248K |
| Go 服务编译 | `go build ./cmd/server` | 3.00s |
| Go 服务测试 | `go test ./...` | 3.40s，通过 |
| Flutter App 静态检查 | `flutter analyze` | 8.77s，无问题 |

### 2.1 性能结论

- 后台管理端开发启动和构建速度可接受，不是主要瓶颈。
- Go 后端编译和测试速度良好。
- 官网构建本身快，但产物异常大，主要来自视频、音频和上传文件。
- App 静态检查正常，但 App 资产图片存在明显压缩空间。

## 3. 主要性能问题与优化建议

### P0：官网 `dist` 约 232M，静态资源过重

定位到的大文件主要包括：

- `website-react/public/assets/videos/*.mp4`
- `website-react/public/assets/uploads/video/analysis/*.mp4`
- `website-react/public/assets/uploads/video/*.mp4`
- `website-react/public/assets/audio/*.m4a`
- `website-react/public/assets/avatars/*.png`

风险：

- Docker 镜像变大，部署变慢。
- 每次构建都会复制历史上传和视频资产。
- 静态资源与代码生命周期耦合，不利于回滚、迁移和 CDN 缓存。

建议：

1. 将 `website-react/public/assets/uploads` 从源码和镜像构建中剥离，改走 OSS / CDN / 挂载卷。
2. 固定课程视频也建议迁移到 CDN，前端只保留配置 URL。
3. 对头像、Logo、封面图统一压缩为 WebP/AVIF。
4. 后端上传默认不要再落入官网源码目录，生产使用 `UPLOAD_DIR=/data/uploads` 或 OSS。

### P1：App 图片资产偏大

App 头像位于：

- `/Users/wohenzaiyi/Desktop/nine-xing-app/assets/images/avatars/*.png`

多张头像约 650K～1.1M，Logo 约 273K。

建议：

1. 头像按实际展示尺寸裁剪。
2. 转 WebP。
3. 如果需要高清，区分 2x/3x，不要全量使用原始大 PNG。
4. 首屏不需要的素材延迟加载。

### P1：后台最大 chunk 可进一步懒加载

后台 `dist` 最大文件：

- `installCanvasRenderer-*.js` 约 501K
- `basic-*.js` 约 314K
- `api-*.js` 约 161K

建议：

1. ECharts / canvas renderer 只在看板页懒加载。
2. 视频、语音、富文本、表格增强能力按路由懒加载。
3. 压缩后台 `logo.png` / `favicon.png`，当前各约 273K。

## 4. 防御与安全现状

### 4.1 已有防御点

后端已有以下安全设计：

- HTTP 服务设置了：
  - `ReadHeaderTimeout: 5s`
  - `ReadTimeout: 20s`
  - `WriteTimeout: 120s`
  - `IdleTimeout: 60s`
- 生产环境配置会拒绝明显弱配置：默认 JWT、默认后台密码、占位 DB 密码等。
- 后台 token、App token、小程序 token 通过 `tokenKind` 区分。
- App access token 为短时 token，refresh token 支持轮换。
- 后台登录有用户名 + IP 维度限流。
- App 短信发送有手机号和 IP 限流。
- App 短信验证有手机号和 IP 限流。
- 公开报名、统计、小游戏接口有 IP 限流。
- 上传接口有大小限制；头像自助上传校验图片类型。
- SSRF 防护：`netguard` 阻止私网、localhost、link-local 等地址。
- Docker Compose 中 Go server 不直接暴露到宿主机；后台和官网绑定 `127.0.0.1`。
- CORS 在生产环境未配置白名单时不会回写任意 Origin。
- App 推送 deep link 做了路径白名单和 scheme/authority 拦截。

### 4.2 防御缺口与建议

#### P0：限流当前是进程内存级

现状：固定窗口限流存在于单进程内存中。多实例部署、容器重启、横向扩容时限流状态会丢失。

建议：

- 登录、短信、公开写入接口接入 Redis 或数据库级限流。
- 增加每日发送上限、设备维度上限、网段维度上限。
- 增加连续失败锁定策略。

#### P0：后台 token 缺少可吊销机制

现状：后台 JWT 有过期时间，但没有 `jti`、`iat`、`token_version` 这类服务端吊销依据。

建议：

- 用户表增加 `token_version` 或 `password_changed_at`。
- 修改密码、禁用账号、角色权限变化后可强制旧 token 失效。

#### P1：高风险后台操作缺少审计

建议新增后台操作审计表，记录：

- 操作者 ID / 用户名
- IP / User-Agent
- 操作对象
- 操作类型
- 变更前后摘要
- 时间

优先覆盖：

- 修改会员等级
- 禁用 / 启用 App 用户
- 模型配置
- 推送群发
- 上传 / 删除资源
- 站点配置修改
- 系统用户、角色、菜单变更

#### P1：上传类型策略可更细

建议按业务目录限制 MIME / 扩展名 / 魔数，例如：

- 头像：只允许 JPEG/PNG/WebP
- 音频：只允许 m4a/mp3/wav 等业务需要类型
- 视频：只允许 mp4/webm 等业务需要类型
- 文档：单独白名单

## 5. App 侧与后台管理覆盖度

Flutter App 当前功能模块包括：

- 手机号登录
- 健康检查
- 测评问卷
- 命运卡 / 主副卡
- 对话会话、消息、收藏、搜索、反馈
- 私库记忆
- 关系合盘
- 每日练习、任务、成长画像、趋势
- 周报
- 会员权益、订单
- 隐私导出 / 删除
- 推送注册与 deep link
- 语音识别

后台当前已有管理能力：

| App 能力 | 后台覆盖情况 |
|---|---|
| App 用户列表 | 已有 `/api/app-users/list` 与页面 |
| App 用户详情 | 已有 |
| 会员等级维护 | 已有 |
| 禁用 / 启用用户 | 已有 |
| 用户画像/记忆/对话数量汇总 | 已有“用户提炼数据” |
| 用户合盘摘要 | 已在洞察数据中展示摘要 |
| 推送发送 | 已有 |
| 按会员等级推送 | 已有 |
| 推送历史 | 已有 |
| 模型配置 | 已有 |
| 测评题库 | 后端 API 已有，后台入口仍偏弱 |
| 命运卡查看 | 后端只读 API 已有，后台入口有限 |
| 聊天明细 | 后端 App API 已有，后台运营入口不足 |
| 记忆明细 | App API 已有，后台运营入口不足 |
| 订单 / 权益 | App API 已有，后台运营入口不足 |
| 每日练习 / 任务 / 周报 / 趋势 | App API 已有，后台配置与运营入口不足 |
| 隐私导出 / 删除 | App API 已有，后台应只做审计，不建议随意代操作 |

## 6. 建议新增的后台管理模块

### P0：App 订单与权益管理

能力建议：

- 订单列表、订单详情、支付状态查询。
- 按用户查看权益快照。
- 手动补发权益、调整会员等级和有效期。
- 记录补发原因和操作者。
- 异常订单标记。

### P0：聊天与消息质检

能力建议：

- 按用户、卡片、时间、关键词筛选会话。
- 查看问题、回答、耗时、来源摘要、反馈、收藏状态。
- 标记低质量回答。
- 统计高频问题和负反馈。

### P0：私库记忆管理

能力建议：

- 按用户、卡片、状态筛选记忆。
- 查看记忆来源消息。
- 停用 / 恢复 / 删除异常记忆。
- 标记低质量记忆。

### P1：每日练习 / 任务 / 周报运营

能力建议：

- 配置每日练习模板。
- 查看每日打卡率。
- 查看任务完成率。
- 查看周报生成状态。
- 手动重生成周报。

### P1：App 配置中心

能力建议：

- 首页模块开关与排序。
- Onboarding 文案。
- 会员页文案和价格展示。
- 强制升级 / 最低版本。
- 活动 Banner。
- 灰度开关。

### P1：推送人群增强

当前支持全部与会员等级，建议增加：

- 未测评用户
- 已测评未聊天用户
- 7 天未活跃用户
- 某主型用户
- 某会员等级 + 行为标签组合

## 7. 推荐落地顺序

### 第一阶段：性能和生产风险

1. 剥离官网 `public/assets/uploads`，上传资产迁移到 OSS/CDN/挂载卷。
2. 压缩官网与 App 图片资源。
3. 增加后台操作审计。
4. 限流从内存升级为 Redis/DB。
5. 后台 token 加可吊销机制。

### 第二阶段：App 运营后台补齐

1. 订单 / 权益管理。
2. 聊天记录与反馈质检。
3. 私库记忆管理。
4. 测评题库后台入口完善。
5. 主副卡后台查看与筛选。

### 第三阶段：增长运营

1. App 配置中心。
2. 每日练习 / 任务 / 周报管理。
3. App 数据看板。
4. 更细的人群推送。
5. 后台 chunk 懒加载优化。

## 8. 验证命令记录

已执行并通过的命令：

```bash
export PATH=/opt/homebrew/bin:/usr/local/go/bin:$PATH

cd /Users/wohenzaiyi/Desktop/nine-xing/nx-backend
pnpm -F @vben/web-antd run build

cd /Users/wohenzaiyi/Desktop/nine-xing/website-react
npm run build

cd /Users/wohenzaiyi/Desktop/nine-xing/reading-h5
npm run build

cd /Users/wohenzaiyi/Desktop/nine-xing/nx-backend/apps/server
go build ./cmd/server
go test ./...

cd /Users/wohenzaiyi/Desktop/nine-xing-app
flutter analyze
```

结果摘要：

- 后台构建通过。
- 官网构建通过。
- 阅读 H5 构建通过。
- Go 编译通过。
- Go 测试通过。
- Flutter analyze 无问题。

## 9. 注意事项

- 当前两个仓库已有较多未提交改动，本报告未对现有业务源码做功能性修改。
- 本次构建命令可能刷新已有 `dist` 产物。
- `docs/app-backend-admin-plan.md` 当前显示的阶段状态已经落后于实际代码，建议后续单独更新为“历史计划 + 当前完成矩阵”。

## 10. 执行记录（2026-07-04）

本轮已执行不依赖外部服务的本地改造项：

1. 官网上传资产剥离
   - `website-react` 构建后自动删除 `dist/assets/uploads`。
   - `.dockerignore` 排除 `website-react/public/assets/uploads/` 和 `website-react/dist/assets/uploads/`，避免上传历史进入镜像上下文。
   - 验证后 `website-react/dist/assets/uploads` 不存在。

2. 资源体积持续审计
   - 新增 `scripts/audit-large-assets.mjs`。
   - `website-react` 新增 `npm run audit:assets`。
   - 可持续列出官网/App 中超过阈值的大资源。

3. 后台关键操作审计
   - 新增 `admin_operation_logs` 表。
   - 新增后端 `auditlog.Store` 与测试。
   - 已接入 App 客户更新、模型配置更新、App 推送发送。
   - 新增 `/api/audit-logs/list` 查询接口。
   - 后台系统管理新增“操作审计”页面。

4. App 订单后台入口
   - 新增 `/api/app-orders/list` 只读接口。
   - 后台客户管理新增“App 订单”页面。
   - 支持订单号/手机号/昵称关键词、状态、商品筛选。

验证结果：

```bash
cd nx-backend/apps/server && go test ./...
# 通过

cd nx-backend && pnpm -F @vben/web-antd run build
# 通过；仍有上游 @vueuse/core PURE 注释 warning，不影响构建

cd website-react && npm run build
# 通过；构建后 dist/assets/uploads 已剥离
```

产物变化：

- 官网 `dist` 从约 232M 降到约 168M。
- 后台 `dist` 约 5.0M。

仍需外部资源或后续专项推进：

- 固定课程视频迁移 CDN/OSS 后，官网 `dist` 还能继续大幅下降。
- Redis/DB 分布式限流需要确定部署组件。
- 后台 token 吊销机制建议单独做一次鉴权专项。
- 订单补发权益、聊天质检、私库记忆管理可继续作为下一批后台运营模块。

## 11. 二次执行记录（2026-07-04）

本轮继续落地用户指定的 7 项优化与 App 后台运营能力补齐：

1. 固定课程视频 / 音频迁移 CDN/OSS 准备
   - 新增 `website-react/src/utils/assets.js`，统一通过 `VITE_ASSET_BASE_URL` 生成静态资源地址。
   - 课程视频、封面图、音频引用改为走 `assetUrl(...)`。
   - `.dockerignore` 继续排除 `videos`、`audio`、`uploads` 等重资源目录。
   - `website-react/scripts/prune-dist-uploads.mjs` 扩展为构建后同时剥离 `dist/assets/uploads`、`dist/assets/videos`、`dist/assets/audio`。
   - 构建产物 `website-react/dist` 从上一轮约 168M 继续降到约 9.6M。

2. DB 分布式限流
   - 新增 `request_rate_limits` 表。
   - 新增 `dbRateLimiter`，后台登录限流优先使用数据库级固定窗口。
   - 数据库限流异常时回退内存限流，避免 DB 短暂异常导致后台登录全部失败。
   - 补充回归测试：`TestBackendLoginRateLimitFallsBackToMemoryWhenDBLimiterFails`。

3. 后台 token 吊销机制
   - `users` 表新增 `token_version`。
   - 后台登录 token 写入 `tokenVersion`。
   - 鉴权时比对 token 内版本与数据库当前版本，旧 token / 无版本 token 返回 401。
   - 用户资料更新、密码更新时递增 `token_version`。
   - 补充 token 版本载荷回归测试：`TestTokenVersionRoundTrips`。

4. 订单补发权益
   - 新增 `POST /api/app-orders/{id}/grant`。
   - 可将订单补发为已支付，并对 `vip_*` 商品补发 App 用户会员等级。
   - 写入 `app_order.grant` 操作审计。
   - 后台 App 订单页面新增“补发”按钮与确认操作。

5. 聊天质检后台
   - 新增 `GET /api/app-chat/messages/list`。
   - 支持关键词、消息角色、反馈筛选。
   - 后台新增“聊天质检”页面，可查看用户、卡片、消息、收藏、反馈、来源等信息。

6. 私库记忆后台管理
   - 新增 `GET /api/app-memories/list` 与 `PUT /api/app-memories/{id}/status`。
   - 后台新增“私库记忆”页面，支持关键词/状态筛选、详情查看、启用/停用。
   - 记忆状态变更写入 `app_memory.status` 操作审计。

7. App 图片 WebP 压缩替换
   - App `logo`、`wheel`、9 张头像从 PNG 转为 WebP。
   - 更新 Flutter 代码与图片资产测试引用。
   - App 图片目录体积约 164K。

8. 额外修复
   - 补齐 server 测试内 `push_store_test` fixture 驱动，避免跨包测试驱动不可见导致 `internal/server` 包测试失败。
   - 确认后台推送 audience count handler 与 App 数据概览 handler 有测试覆盖。

本轮关键验证命令：

```bash
cd /Users/wohenzaiyi/Desktop/nine-xing/nx-backend/apps/server
/opt/homebrew/bin/go test ./internal/server -run 'TestBackendLoginRateLimitFallsBackToMemoryWhenDBLimiterFails|TestBackendLoginRateLimitTracksIPAndUsername|TestAdminPushAudienceCount' -count=1
/opt/homebrew/bin/go test ./internal/auth -run TestTokenVersionRoundTrips -count=1
/opt/homebrew/bin/go test ./...

cd /Users/wohenzaiyi/Desktop/nine-xing/nx-backend
pnpm -F @vben/web-antd run build

cd /Users/wohenzaiyi/Desktop/nine-xing/website-react
node src/pages/website-feature-flow.test.mjs
node src/hooks/useMusic.audio-file.test.mjs
npm run build

cd /Users/wohenzaiyi/Desktop/nine-xing-app
flutter test test/assets/image_assets_test.dart
flutter analyze
```

当前产物体积记录：

- `website-react/dist`：约 9.6M。
- `nx-backend/apps/web-antd/dist`：约 5.0M。
- `nine-xing-app/assets/images`：约 164K。

后续仍可优化：

- 若生产环境已有 Redis，建议将 `dbRateLimiter` 抽象为 Redis/DB 双实现，Redis 优先、DB 兜底。
- 后台 token 可继续增加“强制下线指定用户 / 全员下线”按钮，本轮底层版本机制已具备。
- App 运营后台后续可继续补“每日练习 / 任务 / 周报 / 成长趋势”的配置和人工重算入口。
