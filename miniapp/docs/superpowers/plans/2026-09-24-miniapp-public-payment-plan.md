# 小程序公共支付封装实施计划

> **For agentic workers:** REQUIRED: Use superpowers:subagent-driven-development (if subagents available) or superpowers:executing-plans to implement this plan. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** 将报告和课堂现有微信支付链路统一到可复用的小程序公共方法，并提供后续业务接入文档。

**Architecture:** 在现有支付工具中提供微信支付参数校验、真实调起和开发模拟适配；复用现有有界订单状态控制器，扩展为支持报告 `unlocked` 状态与通用 `isPaid`/`isTerminal` 判定。页面只提供业务下单、开发模拟、状态查询和成功刷新回调，金额继续由后端订单快照决定。

**Tech Stack:** uni-app/Vue 3、Node.js `.mjs` 契约与运行时测试、现有 Bearer API 请求层。

---

### Task 1: 公共支付行为测试

**Files:**
- Modify: `miniapp/src/utils/payment.test.mjs`
- Modify: `miniapp/src/utils/classroom-progress-order.test.mjs`

- [ ] 写失败断言：公共调起校验必需字段，开发模拟只调用注入的业务接口，真实环境调用一次 `uni.requestPayment`。
- [ ] 写失败断言：公共控制器按服务端状态轮询，支持报告 `unlocked`，并保留取消、超时、重复点击和 stop 语义。
- [ ] 运行测试确认在实现前失败。

### Task 2: 公共支付实现

**Files:**
- Modify: `miniapp/src/utils/classroomProgress.js`
- Modify: `miniapp/src/utils/payment.js`

- [ ] 扩展订单控制器支持业务自定义成功/终态判定。
- [ ] 增加 `requestWechatPayment`、`payWechatOrder` 和 `createWechatPaymentController`，统一 dev/production 分支与参数校验。
- [ ] 保留 `payForReport` 兼容入口，并通过报告状态接口确认服务端已解锁后再返回。

### Task 3: 页面接入

**Files:**
- Modify: `miniapp/src/pages/result/result.vue`
- Modify: `miniapp/src/pages/classroom/classroom.vue`
- Modify: `miniapp/src/pages/classroom-detail/classroom-detail.vue`
- Modify: `miniapp/src/pages/profile/profile.vue`

- [ ] 报告、课堂页面改用公共控制器/调起方法，移除重复的 `uni.requestPayment` 包装。
- [ ] 保留页面现有状态展示、权限刷新和测试订单行为。

### Task 4: 文档与验证

**Files:**
- Modify: `miniapp/docs/miniapp-payment-api.md`
- Modify: `miniapp/README.md`

- [ ] 对齐文档中的公共方法签名、状态、字段和报告/课堂接口差异。
- [ ] 运行支付专项测试、完整 `test:config`、生产 API 校验和微信小程序构建。
