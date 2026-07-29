# 小程序运营文案配置化实施计划

> **For agentic workers:** REQUIRED: Use superpowers:subagent-driven-development (if subagents available) or superpowers:executing-plans to implement this plan. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** 在不改变当前首页视觉、路由和课件 API 的前提下，把学习页固定文案、企业服务方式与合作流程接入后台 `site-config`。

**Architecture:** 小程序从 `home.miniappLearn` 读取学习页专属文案，从现有 `home.enterprise.items/processSteps` 读取企业预约数据；页面只消费经过默认值保护的规范化结果。后台复用现有 `/site-config` JSON 保存接口，在“小程序管理 → 学习页管理”和“官网管理 → 企业课程”增加结构化配置区，旧配置无需迁移即可继续显示当前默认内容。

**Tech Stack:** uni-app/Vue 3、微信小程序 WXSS、Go site-config API、Ant Design Vue 管理后台、Node/Vitest 契约测试。

---

### Task 1: 建立小程序学习页文案配置模型

**Files:**
- Create: `src/utils/miniappLearn.js`
- Create: `src/utils/miniappLearn.test.mjs`

- [x] 写表驱动测试并实现 `normalizeMiniappLearn(config)`：空配置、部分配置、空白/畸形字段、meta 上限、输入不变异均覆盖。
- [x] 运行 `node src/utils/miniappLearn.test.mjs`，确认 RED→GREEN。
- [x] 结果和默认值不共享引用，未知字段不影响旧配置。
- [x] 提交 `8373c44 feat(miniapp): add learn copy normalizer`。

### Task 2: 接入学习页

**Files:**
- Modify: `src/pages/learn/learn.vue`
- Modify: `src/pages/learn/learn.content-state.test.mjs`
- Modify: `src/utils/siteConfig.js`
- Modify: `src/utils/siteConfig.test.mjs`
- Modify: `package.json`

- [ ] 先补页面源码契约，要求 Hero、课堂精选、区块标题、空态和底部 CTA 从 `miniappLearn` 读取。
- [ ] 补 `learningSources()` 对 `home.miniappLearn` 的识别测试并确认 RED。
- [ ] 接入配置，保留课堂 API、已有课程/老师/语录数据、固定路由和当前视觉布局。
- [ ] 将 normalizer 测试加入 `test:config`，运行专项测试与全量配置测试。

### Task 3: 接入企业预约页

**Files:**
- Modify: `src/utils/personalExpertHome.js`
- Modify: `src/utils/personalExpertHome.test.mjs`
- Modify: `src/pages/booking/booking.vue`
- Modify: `src/pages/booking/booking.enterprise.test.mjs`

- [ ] 先补 `enterprise.items/processSteps` 的配置优先、过滤、上限、默认回退和输入不变异测试并确认 RED。
- [ ] 在 `personalExpertServices()` 中返回规范化的 `serviceModes/processSteps`。
- [ ] 让预约页展示配置数组，并在 `onShow` 读取缓存配置；保留表单草稿和 intent，重排后按标题重新匹配选中项。
- [ ] 运行企业预约专项测试与全量配置测试。

### Task 4: 接入管理后台

**Files:**
- Modify: `../nx-backend/apps/web-antd/src/api/core/site-config.ts`
- Create: `../nx-backend/apps/web-antd/src/views/miniapp/learn.vue`
- Create: `../nx-backend/apps/web-antd/src/views/miniapp/learn.test.ts`
- Modify: `../nx-backend/apps/web-antd/src/views/site-config/enterprise.vue`
- Modify: corresponding route/menu registration and tests
- Modify: `../shared/site-config.json`

- [ ] 扩展 `home.miniappLearn`、enterprise service/process item 类型和旧配置默认初始化。
- [ ] 新增“学习页管理”入口，编辑 Hero、课堂精选、区块标题、空态与底部 CTA；实际视频/音频继续由老师课堂管理维护。
- [ ] 在企业课程页增加服务方式与合作流程数组的新增、删除、排序与逐条编辑。
- [ ] 补后台渲染、保存、round-trip 和旧配置兼容测试。
- [ ] 运行 web-antd 相关 Vitest 测试。

### Task 5: 集成验证

- [ ] 运行小程序专项测试、微信样式兼容测试和 `npm run test:config`。
- [ ] 运行 `npm run build:mp-weixin`。
- [ ] 确认 `npm run dev:mp-weixin` 持续运行，并在微信开发者工具验证学习页与企业预约页。
- [ ] 检查 diff、提交实现，并保留用户此前可回退标记。
