# 官网与小程序平台数据及后台消息提醒设计

## 目标

在现有后台“数据概览”中直接展示官网端与微信小程序端的用户量、业务提交量和趋势；让小程序新用户、测评提交、预约咨询以及官网报名能够产生可见的后台未读提醒；修复消息管理中的“查看业务”按钮，使其跳转后自动打开对应业务详情。

## 已确认范围

- 采用方案 A：直接升级现有“数据概览”，不新增独立平台数据菜单。
- 官网端统计口径：
  - 用户量为 `site_visits.visitor_id` 去重后的访客数。
  - 业务量为官网报名咨询 `signups` 数量。
- 微信小程序端统计口径：
  - 用户量为 `wx_users` 数量。
  - 业务量包括测评提交 `test_records` 和预约咨询 `bookings`。
- 平台看板提供总量、今日新增、最近 7/30 天趋势和最近业务动态。
- 后台采用 15 秒轮询，不引入 WebSocket。
- 本轮不把 App 手机号用户体系 `app_users` 混入“微信小程序端”统计。

## 当前问题与根因

### 小程序提交后后台没有提醒

当前管理后台已经存在一套“官网报名”全局提醒：`basic.vue` 连接 `/api/signups/events` SSE，并用 60 秒轮询回退，依靠本地 `localStorage` 保存最后提醒 ID。它只覆盖报名事件，不覆盖小程序新用户和测评；消息管理页面本身也没有统一的全局未读数量查询。若在此基础上再增加 `messages` 轮询，会造成官网报名重复弹窗，因此本轮必须停用前端旧 signup SSE/轮询，统一改为 `messages` 未读提醒。

本地调试时，小程序默认请求线上 API，而本地管理后台连接本地 `nx_admin`，因此线上提交不会自动出现在本地数据库。这是环境隔离，不应通过跨环境读库规避；后台需明确显示当前运行环境。

### “查看业务”没有反应

现有报名消息的 `target_path` 为 `/message/management?type=signup`，仍指向消息管理自身。点击后路由没有实质变化。报名信息页面目前也只支持按状态筛选，不能根据业务 ID 自动打开详情。

### 缺少平台维度

现有官网数据概览只聚合 `site_visits` 和 `signups`；App 数据看板只聚合 `app_users` 等 App 体系数据。微信小程序的 `wx_users`、`test_records`、`bookings` 没有与官网端形成统一的运营对比视图。

## 总体架构

新增三个边界清晰的能力：

1. `businessmessage` 领域服务：统一生成业务消息、规范业务类型和跳转目标。
2. 平台运营统计查询：只读聚合官网与微信小程序已有数据表，不复制业务数据。
3. 后台全局提醒客户端：轻量轮询未读摘要，只有发现新 ID 时弹窗，避免重复提醒。

业务写入和消息写入必须共享事务。统计查询与消息查询保持只读，不反向修改业务状态。

## 数据模型

### messages 增量字段

在现有 `messages` 表增加：

- `platform`：`website`、`miniapp`、`system`。
- `event_key`：稳定事件标识，如 `signup.created`、`miniapp.user.created`、`miniapp.quiz.submitted`、`miniapp.booking.created`。

保留已有 `type`、`business_id`、`business_type`、`target_path`，兼容历史数据。增加唯一约束 `UNIQUE(event_key, business_type, business_id)`，消息写入采用 `ON CONFLICT` 幂等处理，防止接口重试产生重复提醒。为未读轮询增加 `(is_read, id DESC)` 索引，为平台筛选增加 `(platform, create_time DESC)` 索引。

迁移时回填历史消息：根据现有 `business_type`、标题和目标路径补齐 `platform`、`event_key` 与正确的 `target_path`；无法可靠识别的历史系统消息标记为 `system`，不得猜测业务归属。

### signups 来源字段

在 `signups` 增加非空字段 `source_platform`，取值仅允许 `website` 或 `miniapp`，历史默认 `website`。迁移时根据 `bookings.signup_id` 将对应报名线索回填为 `miniapp`。此后：

- 官网接口创建报名时显式写入 `website`。
- 小程序预约创建报名线索时显式写入 `miniapp`。
- 所有官网报名数量、报名转化率和最近动态查询都必须过滤 `source_platform='website'`。
- 小程序预约只按 `bookings` 统计，不能再把其关联 signup 算入官网报名，避免重复统计。

迁移前检查 `bookings.signup_id` 孤儿数据；能够确定的先修复，无法确定的设为空并记录迁移日志，然后为 `bookings.signup_id` 增加指向 `signups(id)` 的外键。

### 消息类型与跳转

| 事件 | platform | businessType | targetPath |
| --- | --- | --- | --- |
| 官网报名 | website | signup | `/customer/signups?leadId={id}&open=detail` |
| 小程序预约 | miniapp | signup | `/customer/signups?leadId={signupId}&open=detail` |
| 小程序新用户 | miniapp | miniapp-user | `/customer/miniapp-users?userId={id}&open=detail` |
| 小程序测评 | miniapp | miniapp-test-record | `/customer/miniapp-users?userId={userId}&testRecordId={id}&open=test` |

小程序用户管理页面若当前不存在，则在“客户管理”下增加只读页面，展示 `wx_users`、最近测评和预约。它与现有 `App 客户` 页面保持区分，避免混淆两个用户体系。页面和 API 使用 `Customer:Miniapp:List` 权限；RBAC 菜单迁移将该权限授予现有具备客户查看权限的管理员角色。消息目标对应页面无权限时，统一跳转器显示“无权查看该业务”，不静默失败。

## 写入流程与事务

### 官网报名

引入最小 `DBTX` 接口，使 store 方法同时接受 `*sql.DB` 和 `*sql.Tx`。事务只能由领域 service 协调：service 调用 `BeginTx`，把同一个 tx 传给 signup、booking 和 businessmessage store，所有步骤成功后 `Commit`，失败统一 `Rollback`。handler 不允许吞掉 signup 创建错误后继续创建 booking。任何页面刷新/SSE 兼容广播只能在 commit 成功后执行。

`signup.Service.CreateWebsiteSignup` 在同一数据库事务中：

1. 写入 `signups`，`source_platform=website`。
2. 写入 `messages`，平台为 `website`，目标为该报名详情。
3. 提交事务。

### 小程序新用户

`miniapp.Service.UpsertUser` 调用 tx-aware store 并返回“是否新建”。首次创建用户时，在同一事务中写入 `miniapp.user.created` 消息；重复登录只更新登录时间，不重复提醒。

### 小程序测评

`miniapp.Service.SaveTestRecord` 在同一事务中写入测评记录和 `miniapp.quiz.submitted` 消息。消息包含用户昵称或脱敏标识、主型和提交时间。

### 小程序预约

预约、报名线索和提醒由一个服务事务协调：

1. 写入 `source_platform=miniapp` 的报名线索，但不生成官网报名消息。
2. 写入 `bookings` 并绑定 `signup_id`。
3. 生成一条 `miniapp.booking.created` 消息，目标为报名详情。
4. 提交后再广播页面实时刷新事件。

任何一步失败都整体回滚，不留下“有业务无提醒”或“有提醒无业务”的半状态。业务消息写入发生冲突时视为幂等成功，但仍需确认对应业务记录存在。

## 未读提醒 API

新增需要 `Message:Manage:List` 权限的 `GET /api/messages/unread-summary?afterId={decimalString}`，返回：

```json
{
  "count": 3,
  "latestId": "128",
  "items": [
    {
      "id": "128",
      "title": "新的小程序预约",
      "platform": "miniapp",
      "businessType": "signup",
      "targetPath": "/customer/signups?leadId=42&open=detail",
      "createTime": "2026-07-23 17:00:00"
    }
  ],
  "hasMore": false,
  "nextAfterId": "128"
}
```

接口返回总未读计数、当前最大 ID，以及 `afterId` 之后的未读消息，按 ID 升序分页拉取并返回 `hasMore`/`nextAfterId`，避免 15 秒内产生超过展示窗口数量的消息时漏提醒。所有 BIGSERIAL ID 在 JSON 和前端状态中都使用十进制字符串；前端使用 `BigInt` 或十进制字符串比较，禁止 `Number`、`parseInt`、`Math.max`，避免超过 JavaScript 安全整数后去重失效。

接口只返回提醒所需字段，不返回完整消息正文、openid 或 unionid。消息列表接口增加 `platform` 和 `eventKey` 字段及平台筛选参数。

### 未读生命周期

- 单条“查看业务”前先调用 `PUT /api/messages/read` 标记对应消息已读，成功后更新顶部数量并跳转。
- 消息管理的“全部已读”调用同一接口的批量语义，服务端成功后才清空本地未读状态。
- 顶部通知下拉的“清空”不删除数据库消息，语义为“将当前通知全部标记已读并清空下拉展示”；它必须先调用 `/api/messages/read` 的全部已读语义，服务端成功后才清空本地数组。消息管理页本轮不提供删除消息能力。
- 任一请求失败时回滚乐观 UI，保留原未读数量并提示一次可恢复错误。

## 后台全局提醒

- 登录后立即查询一次未读摘要。
- 每 15 秒重新查询；页面不可见时暂停，恢复可见后立即查询。
- 顶部 Notification 组件从单纯红点升级为未读数字徽标，超过 99 显示 `99+`。
- 客户端仅在当前登录会话内保存 `afterId` 游标；只有游标之后的未读消息才弹窗，并持续分页直到 `hasMore=false`。
- 首次加载只显示未读数量，不为历史积压消息逐条弹窗。
- 网络错误保留最近一次成功数量，不错误归零；下一轮自动恢复。
- 点击弹窗或消息管理“查看业务”调用统一跳转方法。
- 删除 `basic.vue` 对 `signup-events.ts`、`signup-notice.ts` 的连接和轮询调用，并删除或迁移 `nx-signup-notice-last-id:v2` 状态；前端不再访问 `/api/signups/events`。后端端点本轮可兼容保留，供旧版管理端短期使用。

## “查看业务”与详情自动打开

消息管理不再信任任意外部 URL，只接受站内白名单路径：

- `/customer/signups`
- `/customer/miniapp-users`
- `/customer/user-insights`

报名信息页面监听 `leadId` 和 `open=detail`：加载列表后查找目标；若不在当前页则直接调用详情 API，再打开抽屉。

小程序用户页面监听 `userId`、`testRecordId` 和 `open`，打开用户详情或测评详情。重复点击同一路由消息也必须触发打开动作，不能因为 URL 未变化而无反应。

统一跳转器在路由表或权限表中发现目标不可访问时，显示“无权查看该业务”；目标记录已删除时显示“业务记录不存在或已删除”，不能表现为按钮无反应。

## 平台数据接口

新增 `GET /api/analytics/platform-overview?days=7`，`days` 只允许 7 或 30。返回：

- `website`
  - `totalUsers`：全量去重 visitor。
  - `newUsersToday`：今日首次出现的 visitor，页面统一命名为“今日新增访客”。
  - `activeUsersToday`：今日出现过的独立 visitor，页面统一命名为“今日活跃访客”。
  - `totalSubmissions`：`source_platform=website` 的报名总数。
  - `submissionsToday`：今日 `source_platform=website` 的报名数。
  - 保留现有 PV、今日 PV 和报名转化率指标；转化率分子只统计官网报名。
- `miniapp`
  - `totalUsers`：`wx_users` 总数。
  - `newUsersToday`：今日新增微信用户。
  - `totalTests`、`testsToday`。
  - `totalBookings`、`bookingsToday`。
- `series`
  - 每日官网活跃访客、新增小程序用户、官网报名、小程序测评、小程序预约。
- `recentActivities`
  - 最近的官网报名、小程序新用户、测评和预约，最多 10 条。小程序预约关联的 signup 不得再次作为官网报名返回。

统计按 `Asia/Shanghai` 自然日计算。官网每日用户趋势为当日独立访客；总用户为全周期独立 visitor，两者语义在页面上明确标注。

## 首页方案 A

在现有数据概览顶部增加：

1. 官网端平台卡：累计访客、今日新增访客、今日活跃访客、PV、今日 PV、累计报名、今日报名和报名转化率。
2. 微信小程序平台卡：累计用户、今日新增、累计测评、累计预约。
3. 7 天 / 30 天切换。
4. 双平台用户趋势图。
5. 双平台业务提交趋势图。
6. 最近动态列表，带平台标签和“查看业务”。

原有官网询盘、待跟进工作台继续保留，避免影响已有运营流程。

## 小程序用户管理页面

新增只读的“小程序客户”页面：

- 列表：昵称、头像、手机号、渠道、场景、主型、会员级别、注册时间、最近登录。
- 详情：用户资料、测评记录、预约记录。
- 支持按关键词和渠道筛选。
- 列表接口：`GET /api/miniapp/users?page&pageSize&keyword&channel`，最大 `pageSize=100`，返回总数和分页数据。
- 详情接口：`GET /api/miniapp/users/{id}`，返回脱敏用户资料、分页测评记录与预约记录；详情子列表也限制最大页大小。
- 列表和详情均要求 `Customer:Miniapp:List`，响应不得包含 openid、unionid；手机号按现有规则脱敏。
- 本轮不提供删除、封禁、会员修改，避免扩大权限范围。

## 环境提示

后台通过显式配置 `VITE_APP_ENV_LABEL` 和 `VITE_APP_ENV_KIND` 判断并显示环境标识，如“本地开发”；不能只依赖 `import.meta.env.PROD`。帮助文案说明：本地后台只显示本地数据库数据，线上小程序提交不会进入本地库。生产环境可将标签配置为空，不显示冗余提示。

## 错误处理与安全

- 业务消息写入失败时业务事务回滚。
- 未读轮询失败不阻断页面，不连续弹错误通知。
- 统计 API 使用固定 SQL，不接受字段名和排序 SQL 注入。
- `targetPath` 必须通过前端站内白名单校验。
- 消息和最近动态中的手机号只显示脱敏值，不返回 openid、unionid。
- 所有列表分页并限制最大 pageSize。

## 测试与验收

### 后端

- 首次小程序登录生成一条消息，重复登录不重复生成。
- 测评提交与消息原子写入；任一步失败整体回滚。
- 预约只生成一条小程序预约消息，不重复生成官网报名消息。
- 官网报名消息目标包含正确 `leadId`。
- 未读摘要计数、latestId、平台筛选正确。
- `afterId` 多页追赶不会漏消息，BIGSERIAL ID 按字符串保持精度。
- 单条已读、全部已读和顶部通知清空语义与数据库状态一致，失败可重试。
- 平台统计总量、今日值、7/30 天序列和时区边界正确。
- 官网统计只包含 `source_platform=website`，小程序预约不会重复进入官网最近动态。
- `bookings.signup_id` 外键迁移能处理历史孤儿数据。
- 消息、小程序客户和最近动态响应不泄露 openid、unionid 或完整手机号。
- 历史消息缺少 platform/event_key 时仍可读取。

### 前端

- 首次轮询只更新红点，不弹历史消息。
- 后续新 ID 只弹一次，网络失败不归零。
- 页面隐藏暂停轮询，恢复后立即查询。
- “查看业务”只跳站内白名单路径。
- 旧 signup SSE/轮询不再连接，官网报名只弹一条 messages 通知。
- 顶部显示准确数量和 `99+`，单条/全部已读失败时恢复原状态。
- 超过 JavaScript 安全整数的消息 ID 仍能正确比较和去重。
- 同一路由重复点击仍打开详情。
- 报名页面和小程序用户页面能按 URL 参数自动打开详情。
- 数据概览正确渲染两个平台、7/30 天切换和空状态。

### 本地联调

1. 原生启动 PostgreSQL、Go 后端和 Vite 管理后台，不使用 Docker。
2. 使用本地测试接口模拟官网报名、小程序新用户、测评和预约。
3. 验证数据库业务记录与消息数量一致。
4. 验证顶部红点在 15 秒内更新并只弹一次。
5. 验证消息管理和最近动态均可打开正确业务详情。
6. 分别用官网 API 与本地小程序 API 提交数据，确认来源字段、统计与提醒不串平台。

## 不在本轮范围

- WebSocket/SSE 实时推送。
- 跨环境读取生产数据库。
- 合并 `wx_users` 与 `app_users` 两套身份体系。
- 小程序客户写操作、封禁或会员管理。
- 自定义报表导出和任意日期范围分析。
