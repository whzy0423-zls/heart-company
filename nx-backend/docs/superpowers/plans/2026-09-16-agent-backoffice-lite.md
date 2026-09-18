# Agent Backoffice Lite Implementation Plan

> **For agentic workers:** REQUIRED: Use superpowers:subagent-driven-development (if subagents available) or superpowers:executing-plans to implement this plan. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** 让二级/三级代理可以复用 App 账号密码登录后台，并只访问代理管理与经营数据分析，按自身代理链路查看数据、二级代理可添加三级代理。

**Architecture:** 后端扩展后台登录逻辑，允许 App 用户账号密码在其具备代理身份时换发后台 JWT，并在 JWT 中标记代理上下文。新增代理后台专用 API，所有查询用当前代理 id/agent_path 做数据隔离。前端在现有 web-antd 后台增加代理轻量权限分流，复用分销管理页面但根据角色隐藏管理员功能。

**Tech Stack:** Go net/http + PostgreSQL SQL, Vue 3 + Ant Design Vue + ECharts, Vitest source contract tests, Go route/source tests, Flutter widget tests.

---

## Chunk 1: Backend auth and scoped analytics

### Task 1: Contract tests for agent backoffice login and routes

**Files:**
- Modify: `apps/web-antd/src/views/app/distribution-management.test.ts`
- Modify: `apps/server/internal/server/distribution_routes_test.go`
- Inspect: `apps/server/internal/server/auth*.go`, `apps/server/internal/server/server.go`

- [ ] Add failing tests requiring app-user agent login markers and agent-scoped routes.
- [ ] Run Vitest and Go route tests to verify red.
- [ ] Implement route registrations and handler skeletons.
- [ ] Run tests to verify green.

### Task 2: Agent-scoped API implementation

**Files:**
- Modify: `apps/server/internal/server/distribution.go`
- Modify: `apps/server/internal/server/server.go`
- Modify: `apps/web-antd/src/api/core/distribution.ts`

- [ ] Add failing source/contract tests for scoped analytics, time range parameters, child-agent creation constraints.
- [ ] Implement `currentAgentFromRequest`, scoped analytics query, scoped child-agent creation, and self profile endpoint.
- [ ] Preserve admin full analytics behavior.
- [ ] Verify with Go and Vitest tests.

## Chunk 2: Frontend agent backoffice UI

### Task 3: Login and permission branching

**Files:**
- Inspect/Modify: web-antd login API/store/router files.
- Modify API types in `apps/web-antd/src/api/core/distribution.ts` as needed.

- [ ] Add tests/source assertions for app-agent login support and restricted menu access.
- [ ] Implement frontend storage/useAccess role recognition for `agent` sessions.
- [ ] Verify typecheck.

### Task 4: Agent management and analytics UI

**Files:**
- Modify: `apps/web-antd/src/views/app/distribution-management.vue`
- Possibly create/modify: agent analytics view if current page becomes too large.

- [ ] Add tests for 今日/昨日/近7天/自定义时间, chart, summary cards, user/order detail tables.
- [ ] Implement date range controls and scoped API calls.
- [ ] Add二级代理新增三级代理入口; hide for 三级代理.
- [ ] Verify typecheck and Vitest.

## Chunk 3: App money unit

### Task 5: Yuan display in App

**Files:**
- Modify: `/Users/wohenzaiyi/Desktop/nine-xing-app/.worktrees/polish-trend-screen/lib/features/distribution/screens/distribution_screen.dart`
- Test: `/Users/wohenzaiyi/Desktop/nine-xing-app/.worktrees/polish-trend-screen/test/features/distribution/distribution_screen_test.dart`

- [ ] Keep test asserting backend amounts are yuan.
- [ ] Verify Flutter analyze and widget test.

## Final Verification

- [ ] `pnpm --filter @vben/web-antd run typecheck`
- [ ] `pnpm vitest run apps/web-antd/src/views/app/distribution-management.test.ts`
- [ ] `cd apps/server && go test ./internal/server -run 'TestDistributionRouteContract|TestDistributionPostgresFrontendListContract'`
- [ ] `flutter analyze lib/features/distribution/screens/distribution_screen.dart test/features/distribution/distribution_screen_test.dart`
- [ ] `flutter test test/features/distribution/distribution_screen_test.dart`
