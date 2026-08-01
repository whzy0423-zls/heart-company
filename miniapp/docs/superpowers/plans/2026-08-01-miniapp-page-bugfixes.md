# Miniapp Page Bugfixes Implementation Plan

> **For agentic workers:** REQUIRED: Use superpowers:subagent-driven-development (if subagents available) or superpowers:executing-plans to implement this plan. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** 修复学习页跳转、课堂播放恢复、关系页定时跳转，以及窄屏页面布局问题。

**Architecture:** 保持现有 Vue/uni-app 页面结构不变，在对应页面内做局部生命周期与路由修正。使用现有契约测试补充回归断言，并通过完整测试和双端构建验证。

**Tech Stack:** Vue 3、uni-app、Node.js assert 测试、Vite。

---

### Task 1: 学习页导航
- [ ] 补充底部 CTA 直达测试页、推荐项直达具体内容的失败测试。
- [ ] 修改 `src/pages/learn/learn.vue` 路由处理并运行学习页测试。

### Task 2: 页面生命周期
- [ ] 补充关系页定时器清理与课件播放恢复的失败测试。
- [ ] 修改 `relation.vue` 与 `classroom-detail.vue` 并运行对应测试。

### Task 3: 窄屏样式
- [ ] 补充预约场景和身份选择的响应式契约测试。
- [ ] 增加 360px 以下布局规则并运行页面测试。

### Task 4: 完整验证
- [ ] 运行完整测试、H5 构建与微信小程序构建。
