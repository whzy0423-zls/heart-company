# Apple 移动端与后台运营增强设计

日期：2026-07-04  
仓库：`/Users/wohenzaiyi/Desktop/nine-xing`

## 背景与约束

当前仓库未发现原生 iOS / Xcode / Swift 工程。移动端可落地对象是 `miniapp/`（uni-app + Vue3，可构建小程序和 H5），因此本轮先把 `miniapp` 打磨成 Apple/iOS 风格移动端原型，同时完善后台 App 运营能力。核心业务逻辑（测评、聊天、学习、关系合盘、登录、支付等）不重写，只做 UI、聚合接口、运营工具和高可用兜底。

## 设计目标

1. Apple/iOS 风格移动端：safe-area、44pt 触控、柔和玻璃卡片、明确层级、加载/空态反馈、移动端可读性。
2. 后台 App 运营：管理员可看 App 数据概览、用户 360、用户提炼数据、App 订单、推送记录与发送。
3. 推送更可控：发送前能看到受众预估，提供常用模板，降低误发风险。
4. 高可用：接口失败有明确提示；后台页面不白屏；查询分页和默认值安全；生产配置不放宽。
5. 原逻辑保护：所有新增接口/页面均为增量，不改变现有 App 公共知识库 + 个人知识库沉淀路径。

## Apple 移动端 UI 原则

参考本轮 UI 设计系统输出：产品为 AI/九型人格/成长工具，视觉采用 iOS 风格的极简、高对比、柔和蓝紫渐变与卡片层级。

- 布局：`page` 使用 `safe-area-inset-*`；内容底部预留 tabBar/手势区域。
- 触控：主要按钮和卡片可点击区域不低于 88rpx（约 44pt）；相邻触控间距 >= 16rpx。
- 层级：首页先给核心 CTA（开始测试/AI 对话/关系合盘），再展示成长洞察和功能入口。
- 反馈：按钮点击有轻量态；列表/数据为空时显示引导；加载中不白屏。
- 组件化：新增 `miniapp/src/styles/apple-mobile.css` 存放 tokens、通用卡片、按钮、section、safe-area 等能力，页面只组合样式。
- 字体：使用系统字体栈，不引入 Google 字体外链。

## 后台功能设计

### App 数据看板

新增后台页面：`/dashboard/app`，菜单仅管理员/有权限可见。用于 App 运营总览：

- 总用户、活跃用户、VIP/SVIP、禁用用户。
- 测评提交、聊天会话/消息、记忆、卡片、合盘报告。
- 最近 7 天新增用户、最近 7 天测评、最近 7 天聊天消息。
- 会员等级分布、用户状态分布。
- 最近用户、最近提炼用户。

后端新增：`GET /api/app-analytics/overview`（后台鉴权），聚合现有 App 表。

### App 用户 360 详情

在 `App 客户` 列表增加「360」入口或在详情抽屉扩展：

- 基础资料：手机号、昵称、状态、会员等级、注册/最近登录。
- 测评画像：主型、副型、翼型、性别、最近测评时间、得分标签、中心摘要。
- 沉淀数据：卡片数、记忆数、最近记忆、会话数、消息数、合盘摘要。
- 操作：保持已有状态/会员等级编辑能力。

优先复用现有 `/api/app-users/insights` 聚合数据，避免重复 SQL。

### 推送管理增强

现有 `推送管理` 保留发送与记录。新增：

- `GET /api/push/audience-count?targetType=all|level&targetValue=vip`：发送前预估注册设备数/去重用户数。
- 常用模板：成长周报、每日练习、关系合盘、会员权益。
- 发送弹窗实时显示受众预估；目标非法或无受众时提示。

## 后端 API 设计

- `GET /api/app-analytics/overview`
  - 权限：`Analytics:App:Overview`
  - 返回：`cards`、`memberLevels`、`statusDistribution`、`recentUsers`、`recentInsights`。
- `GET /api/push/audience-count`
  - 权限：`Push:Manage`
  - 参数：`targetType` 默认 `all`；`targetValue` 会员等级。
  - 返回：`targetType`、`targetValue`、`deviceCount`、`userCount`。
- `GET /api/app-users/insights`
  - 已存在，继续作为用户提炼数据和 360 的主要数据源。

## 数据库/权限菜单

新增菜单：

- `DashboardAppAnalytics`：`/dashboard/app`，组件 `/dashboard/app`，权限 `Analytics:App:Overview`。

Push audience count 不需要新菜单，复用 `Push:Manage`。

## 测试与验证策略

后端：

- 单元/HTTP 测试覆盖 App analytics overview 与 push audience count 参数校验。
- `go test ./...`
- `go vet ./...`

后台前端：

- API helper/纯函数测试（Vitest）。
- `pnpm exec vitest run --dom`
- `pnpm --filter @vben/web-antd typecheck`
- `pnpm --filter @vben/web-antd build`

miniapp：

- `npm run test:config`
- `VITE_API_BASE=https://api.nine-xing.com/api npm run prebuild:mp-weixin`
- `npm run build:h5`

## 不改动声明

本设计不改变：

- App 测评题目和结果算法。
- AI 聊天公共知识库检索与个人记忆沉淀主流程。
- 登录、支付、隐私、推送注册已有接口语义。
- 后台现有页面路径与权限，仅新增菜单/入口和聚合展示。
