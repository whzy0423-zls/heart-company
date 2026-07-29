# Teacher Card Compact Layout Implementation Plan

> **For agentic workers:** REQUIRED: Use superpowers:subagent-driven-development (if subagents available) or superpowers:executing-plans to implement this plan. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** 消除老师简介卡片中头像下方的大块空白，同时保留现有后台内容、图片容错与暖白黑金视觉风格。

**Architecture:** 将单层左右双栏拆成顶部媒体信息行和底部通栏详情区。只修改学一学页面模板与局部样式，并用静态契约测试锁定 DOM 层级和关键 CSS。

**Tech Stack:** Vue 3 SFC、uni-app、微信小程序 WXML/WXSS、Node.js `assert` 契约测试

---

## Chunk 1: Teacher card layout

### Task 1: Add the layout regression contract

**Files:**
- Modify: `src/pages/learn/learn.content-state.test.mjs`
- Modify: `scripts/ui-compat.test.mjs`

- [ ] **Step 1: Write a failing test**

断言老师卡片含 `teacher-card__header` 和 `teacher-card__details`，并确认简介位于通栏详情区。

- [ ] **Step 2: Verify the test fails**

Run: `node src/pages/learn/learn.content-state.test.mjs`

Expected: FAIL because the current card still keeps biography inside `teacher-card__body` beside the avatar.

### Task 2: Implement the compact card

**Files:**
- Modify: `src/pages/learn/learn.vue`

- [ ] **Step 1: Update markup**

增加顶部 `teacher-card__header`，将头像和 `teacher-card__identity` 放入其中；增加 `teacher-card__details` 并把简介、标签移入该区域。

- [ ] **Step 2: Update styles**

卡片改为纵向容器，顶部信息行保持横排，详情区宽度为 `100%`；继续使用现有设计变量。

- [ ] **Step 3: Run focused tests**

Run: `node src/pages/learn/learn.content-state.test.mjs && node scripts/ui-compat.test.mjs`

Expected: PASS.

### Task 3: Verify and commit

**Files:**
- Verify: `dist/dev/mp-weixin/pages/learn/learn.wxml`
- Verify: `dist/dev/mp-weixin/pages/learn/learn.wxss`

- [ ] **Step 1: Run full tests**

Run: `npm run test:config`

Expected: PASS with zero failing tests.

- [ ] **Step 2: Check compiled structure and git diff**

Run: `git diff --check && git status --short`

- [ ] **Step 3: Commit**

```bash
git add src/pages/learn/learn.vue src/pages/learn/learn.content-state.test.mjs scripts/ui-compat.test.mjs
git commit -m "fix: compact teacher profile layout"
```

