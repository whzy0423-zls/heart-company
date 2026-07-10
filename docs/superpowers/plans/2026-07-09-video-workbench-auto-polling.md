# Video Workbench Auto Polling Implementation Plan

> **For agentic workers:** REQUIRED: Use superpowers:subagent-driven-development (if subagents available) or superpowers:executing-plans to implement this plan. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** 视频工作台在单个分镜生成、重新生成、批量生成后自动轮询刷新版本列表，直到生成视频回显或任务结束。

**Architecture:** 复用现有 `refreshShotVideoVersionsApi` 和 `handleRefreshShotVideoVersions`，新增分镜级轮询状态与 timer 管理。轮询以 silent/force 模式绕过普通按钮 busy 限制，不打断用户手动操作，并在组件卸载时清理所有 timer。

**Tech Stack:** Vue 3 Composition API、Vben web-antd、Vitest 静态迁移测试、Go 后端现有接口。

---

### Task 1: 前端自动轮询状态与生命周期

**Files:**
- Test: `nx-backend/apps/web-antd/src/views/video/project-workbench-production-flow.test.ts`
- Modify: `nx-backend/apps/web-antd/src/views/video/projects/workbench.vue`

- [ ] **Step 1: Write the failing test**
  - 在迁移测试中断言存在 `shotGenerationPollingIds`、`shotGenerationPollingTimers`、`startShotGenerationPolling`、`stopShotGenerationPolling`、`stopAllShotGenerationPolling`、`isShotGenerationPolling`、`window.setTimeout`、`onBeforeUnmount`。
- [ ] **Step 2: Run test to verify it fails**
  - Run: `pnpm exec vitest run apps/web-antd/src/views/video/project-workbench-production-flow.test.ts`
  - Expected: FAIL because polling symbols are not implemented.
- [ ] **Step 3: Implement minimal polling helpers**
  - Import `onBeforeUnmount`.
  - Add `Set`/`Map` refs for polling shot ids and timers.
  - Implement start/stop/is helpers and lifecycle cleanup.
- [ ] **Step 4: Run test to verify it passes**
  - Run the focused vitest command above.

### Task 2: 生成动作接入自动刷新

**Files:**
- Test: `nx-backend/apps/web-antd/src/views/video/project-workbench-production-flow.test.ts`
- Modify: `nx-backend/apps/web-antd/src/views/video/projects/workbench.vue`

- [ ] **Step 1: Extend failing test**
  - 断言 `handleRefreshShotVideoVersions(shot, { force: true, silent: true })`、`startShotGenerationPolling(shot.id)` 以及批量生成后的轮询启动逻辑存在。
- [ ] **Step 2: Run test to verify it fails**
  - Run: `pnpm exec vitest run apps/web-antd/src/views/video/project-workbench-production-flow.test.ts`
  - Expected: FAIL because generate handlers do not start polling.
- [ ] **Step 3: Implement generate integration**
  - `handleRefreshShotVideoVersions` 增加 `{ force, silent }` options。
  - `generateShot` 成功后启动当前分镜轮询。
  - `handleRegenerateShotVideoVersion` 成功后启动当前分镜轮询。
  - `generateAllShots` 成功后对 generating/成功分镜启动轮询。
- [ ] **Step 4: Run focused tests and typecheck**
  - `pnpm exec vitest run apps/web-antd/src/views/video/project-workbench-production-flow.test.ts`
  - `pnpm --filter @vben/web-antd run typecheck`

### Task 3: Regression verification

**Files:**
- Existing Go tests and front-end tests.

- [ ] **Step 1: Run backend tests**
  - `cd nx-backend/apps/server && go test ./internal/videoproject ./internal/server -count=1`
- [ ] **Step 2: Report evidence**
  - Summarize changed files and exact verification output.
