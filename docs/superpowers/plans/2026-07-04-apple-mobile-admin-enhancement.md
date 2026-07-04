# Apple Mobile Admin Enhancement Implementation Plan

> **For agentic workers:** REQUIRED: Use superpowers:subagent-driven-development (if subagents available) or superpowers:executing-plans to implement this plan. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** 将现有 `miniapp` 打磨成 Apple/iOS 风格移动端原型，并补齐后台 App 数据看板、用户 360 与推送受众预估能力。

**Architecture:** 采用增量式实现：移动端只新增样式 tokens 和页面视图优化，不重写业务逻辑；后台新增独立 API 与页面，复用现有 `app_users`、`app_quiz_submissions`、`app_memories`、`app_user_cards`、`app_chat_*`、`app_compatibility_reports`、`app_device_tokens` 表；权限通过现有菜单种子和 `requirePermission` 控制。

**Tech Stack:** Go 1.22 + net/http + PostgreSQL；Vue 3 + Vben Admin + Ant Design Vue + Vitest；uni-app + Vue3。

---

## File Structure

- Create: `miniapp/src/styles/apple-mobile.css` — App/iOS 风格全局 tokens、safe-area、按钮、卡片、section、辅助类。
- Modify: `miniapp/src/App.vue` — 引入统一样式 tokens 并保留原全局样式语义。
- Modify: `miniapp/src/pages/index/index.vue` — 首页 Apple 风格信息架构与入口优化。
- Modify: `miniapp/src/pages/result/result.vue` — 结果页移动端可读性优化。
- Modify: `miniapp/src/pages/chat/chat.vue` — 聊天页安全区、输入区、消息卡片优化。
- Modify: `miniapp/src/pages/profile/profile.vue` — 我的页会员/成长入口视觉优化。
- Create: `nx-backend/apps/server/internal/appanalytics/overview.go` — 后台 App 数据看板聚合 store。
- Create: `nx-backend/apps/server/internal/appanalytics/overview_test.go` — 纯 SQL/参数辅助测试。
- Modify: `nx-backend/apps/server/internal/server/server.go` — 注入 store、注册 `/api/app-analytics/overview` 和菜单。
- Create/Modify: `nx-backend/apps/server/internal/server/admin_app_analytics.go` — HTTP handler。
- Modify: `nx-backend/apps/server/internal/push/store.go` — 增加 audience count 查询。
- Modify: `nx-backend/apps/server/internal/server/admin_push.go` — 增加 `/api/push/audience-count` handler。
- Create/Modify: `nx-backend/apps/server/internal/server/admin_push_test.go` — 参数校验/HTTP 测试。
- Create: `nx-backend/apps/web-antd/src/api/core/app-analytics.ts` — 后台看板 API 类型与请求。
- Modify: `nx-backend/apps/web-antd/src/api/core/push.ts` — audience count API。
- Modify: `nx-backend/apps/web-antd/src/api/core/index.ts` — 导出新增 API。
- Create: `nx-backend/apps/web-antd/src/views/dashboard/app.vue` — App 数据看板页面。
- Create: `nx-backend/apps/web-antd/src/views/dashboard/app-analytics.test.ts` / `.ts` — 展示辅助函数测试。
- Modify: `nx-backend/apps/web-antd/src/views/customer/app-users.vue` — 增加用户 360 入口，复用 insights。
- Modify: `nx-backend/apps/web-antd/src/views/message/push.vue` — 发送弹窗增加模板与受众预估。
- Modify/Create: `nx-backend/apps/web-antd/src/views/message/push-target.ts` / `.test.ts` — 受众预估和模板 helper。

## Chunk 1: Documentation and Baseline

### Task 1: Design and implementation plan

**Files:**
- Create: `docs/superpowers/specs/2026-07-04-apple-mobile-admin-enhancement-design.md`
- Create: `docs/superpowers/plans/2026-07-04-apple-mobile-admin-enhancement.md`

- [ ] **Step 1: Write design document**

Describe current no-native-iOS constraint, UI principles, admin capabilities, API design, test strategy, and no-business-logic-change statement.

- [ ] **Step 2: Write implementation plan**

Use checkbox tasks and exact file paths.

- [ ] **Step 3: Baseline status**

Run:
```bash
git status --short
```
Expected: Existing uncommitted changes remain; do not revert them.

## Chunk 2: Miniapp Apple UI

### Task 2: Add mobile design tokens and homepage polish

**Files:**
- Create: `miniapp/src/styles/apple-mobile.css`
- Modify: `miniapp/src/App.vue`
- Modify: `miniapp/src/pages/index/index.vue`

- [ ] **Step 1: Add style-only smoke test**

Run current config tests before change:
```bash
cd miniapp && npm run test:config
```
Expected: current tests pass or failures are documented as pre-existing.

- [ ] **Step 2: Create design token stylesheet**

Add CSS variables/classes for `--nx-bg`, `--nx-primary`, `.ios-page`, `.ios-card`, `.ios-button`, `.ios-section`, `.ios-safe-bottom`.

- [ ] **Step 3: Import stylesheet**

In `App.vue`, add `@import './styles/apple-mobile.css';`, preserving existing `.card`, `.btn-primary`, `.wrap` classes.

- [ ] **Step 4: Update homepage template/style**

Add top safe-area hero, quick action cards, stats strip, and keep `startTest/goLearn/goChat/goRelation` unchanged.

- [ ] **Step 5: Verify miniapp config tests**

Run:
```bash
cd miniapp && npm run test:config
```
Expected: PASS.

### Task 3: Polish result/chat/profile screens without changing logic

**Files:**
- Modify: `miniapp/src/pages/result/result.vue`
- Modify: `miniapp/src/pages/chat/chat.vue`
- Modify: `miniapp/src/pages/profile/profile.vue`

- [ ] **Step 1: Inspect existing script blocks**

Confirm no changes to API calls, navigation helpers, or computed business rules.

- [ ] **Step 2: Update templates/classes only where possible**

Apply `.ios-page`, `.ios-card`, `.ios-section`, `.ios-button` and safe bottom spacing.

- [ ] **Step 3: Improve empty/loading states**

Add non-blocking empty guidance for missing result/profile/chat history.

- [ ] **Step 4: Verify H5 build**

Run:
```bash
cd miniapp && npm run build:h5
```
Expected: build exits 0.

## Chunk 3: Backend App Analytics and Push Audience

### Task 4: Add App analytics overview API

**Files:**
- Create: `nx-backend/apps/server/internal/appanalytics/overview.go`
- Create: `nx-backend/apps/server/internal/appanalytics/overview_test.go`
- Create: `nx-backend/apps/server/internal/server/admin_app_analytics.go`
- Modify: `nx-backend/apps/server/internal/server/server.go`
- Modify: `nx-backend/apps/server/internal/db/db.go`

- [ ] **Step 1: Write failing tests for query parameter normalization**

Test default day window and limit bounds in `overview_test.go`.

- [ ] **Step 2: Implement overview store**

Provide `Overview(ctx, query)` returning cards, distributions, recent users, recent insights.

- [ ] **Step 3: Add HTTP handler**

Register `GET /api/app-analytics/overview` with permission `Analytics:App:Overview`.

- [ ] **Step 4: Add menu seed**

Add `/dashboard/app` menu item without changing existing `/dashboard/analytics`.

- [ ] **Step 5: Run backend tests**

Run:
```bash
cd nx-backend/apps/server && go test ./...
```
Expected: PASS.

### Task 5: Add push audience count API

**Files:**
- Modify: `nx-backend/apps/server/internal/push/store.go`
- Modify: `nx-backend/apps/server/internal/server/admin_push.go`
- Modify: `nx-backend/apps/server/internal/server/server.go`
- Modify: `nx-backend/apps/server/internal/server/admin_push_test.go`

- [ ] **Step 1: Write failing handler tests**

Cover invalid target type, missing level target, and valid all/level response shape.

- [ ] **Step 2: Implement store count method**

Count active tokens and distinct users for `all` and `level`.

- [ ] **Step 3: Implement handler**

Validate target using same rules as send; return `{ targetType, targetValue, deviceCount, userCount }`.

- [ ] **Step 4: Run targeted tests**

Run:
```bash
cd nx-backend/apps/server && go test ./internal/server ./internal/push
```
Expected: PASS.

## Chunk 4: Admin Frontend

### Task 6: Add App analytics dashboard page

**Files:**
- Create: `nx-backend/apps/web-antd/src/api/core/app-analytics.ts`
- Modify: `nx-backend/apps/web-antd/src/api/core/index.ts`
- Create: `nx-backend/apps/web-antd/src/views/dashboard/app-analytics.ts`
- Create: `nx-backend/apps/web-antd/src/views/dashboard/app-analytics.test.ts`
- Create: `nx-backend/apps/web-antd/src/views/dashboard/app.vue`

- [ ] **Step 1: Write helper tests**

Test number formatting, member labels, empty data fallback.

- [ ] **Step 2: Add API types**

Match Go JSON response shape.

- [ ] **Step 3: Build dashboard page**

Use Ant Design cards/table/tags; include load error and refresh.

- [ ] **Step 4: Run Vitest target**

Run:
```bash
cd nx-backend && pnpm exec vitest run apps/web-antd/src/views/dashboard/app-analytics.test.ts --dom
```
Expected: PASS.

### Task 7: Add user 360 entry and push audience UX

**Files:**
- Modify: `nx-backend/apps/web-antd/src/views/customer/app-users.vue`
- Modify: `nx-backend/apps/web-antd/src/views/message/push-target.ts`
- Modify: `nx-backend/apps/web-antd/src/views/message/push-target.test.ts`
- Modify: `nx-backend/apps/web-antd/src/views/message/push.vue`
- Modify: `nx-backend/apps/web-antd/src/api/core/push.ts`

- [ ] **Step 1: Extend push helper tests**

Test template application, audience summary, and invalid level behavior.

- [ ] **Step 2: Add audience API helper**

Expose `getPushAudienceCountApi`.

- [ ] **Step 3: Update push modal**

Add template select and audience preview, loading/error states.

- [ ] **Step 4: Update App users**

Add user 360 button linking/opening insight detail, without removing existing detail/edit.

- [ ] **Step 5: Run frontend unit tests**

Run:
```bash
cd nx-backend && pnpm exec vitest run --dom
```
Expected: PASS.

## Chunk 5: Full Verification

### Task 8: Full regression checks

**Files:**
- No functional files unless fixing verification failures.

- [ ] **Step 1: Backend full test/vet**

```bash
cd nx-backend/apps/server && go test ./... && go vet ./...
```

- [ ] **Step 2: Admin frontend typecheck/build**

```bash
cd nx-backend && pnpm --filter @vben/web-antd typecheck && pnpm --filter @vben/web-antd build
```

- [ ] **Step 3: Miniapp checks**

```bash
cd miniapp && npm run test:config && VITE_API_BASE=https://api.nine-xing.com/api npm run prebuild:mp-weixin && npm run build:h5
```

- [ ] **Step 4: Final diff review**

```bash
git diff --stat && git status --short
```

- [ ] **Step 5: Report**

Summarize changed files, verification evidence, and confirm original business logic was preserved.
