# 小程序视频课程总入口显隐 Implementation Plan

> **For agentic workers:** REQUIRED: Use superpowers:subagent-driven-development (if subagents available) or superpowers:executing-plans to implement this plan. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** 在后台通过一个总开关控制小程序视频课程入口整体显示或隐藏。

**Architecture:** 使用现有站点配置 `home.miniappLearn.classroom` 承载 `enabled`，旧数据默认开启。后台编辑器负责保存，小程序统一规范化后让首页和学习页按该值渲染。

**Tech Stack:** Vue 3、uni-app、TypeScript、Vitest、Node.js contract tests

---

## Chunk 1: 配置与后台

### Task 1: 增加配置字段和后台开关

**Files:**
- Modify: `nx-backend/apps/web-antd/src/api/core/site-config.ts`
- Modify: `nx-backend/apps/web-antd/src/views/miniapp/learn.vue`
- Test: `nx-backend/apps/web-antd/src/views/miniapp/learn.test.ts`

- [ ] 先增加默认开启、显式关闭和保存开关的失败测试。
- [ ] 运行定向 Vitest，确认因字段和开关缺失而失败。
- [ ] 为 `MiniappLearnClassroom` 增加 `enabled`，规范化时仅接受布尔值，否则默认 `true`。
- [ ] 在“课堂精选”顶部增加“显示视频课程入口”开关和影响说明。
- [ ] 运行定向 Vitest，确认通过。

## Chunk 2: 小程序消费总开关

### Task 2: 统一规范化并隐藏入口

**Files:**
- Modify: `miniapp/src/utils/miniappPages.js`
- Test: `miniapp/src/utils/miniappPages.test.mjs`
- Modify: `miniapp/src/pages/index/index.vue`
- Modify: `miniapp/src/pages/learn/learn.vue`
- Modify: `miniapp/src/pages/result/result.vue`
- Modify: `miniapp/src/pages/booking/booking.vue`
- Test: `miniapp/src/pages/learn.quote-card.test.mjs`

- [ ] 先增加缺失默认开启、显式关闭和页面消费开关的失败测试。
- [ ] 运行定向 Node 测试，确认失败原因正确。
- [ ] 在小程序规范化结果中加入 `classroom.enabled`。
- [ ] 首页按开关隐藏所有课程与课件入口。
- [ ] 学习页按开关隐藏课程和课件分类，并在关闭时回退到语录分类。
- [ ] 测试结果页和预约完成页隐藏老师课堂入口。
- [ ] 运行定向测试和小程序构建。

## Chunk 3: 完整验证

### Task 3: 回归检查

**Files:**
- Verify only

- [ ] 运行后台定向测试和类型检查。
- [ ] 运行小程序配置测试和 H5 构建。
- [ ] 检查 `git diff`，确认没有修改课程数据与发布逻辑。
