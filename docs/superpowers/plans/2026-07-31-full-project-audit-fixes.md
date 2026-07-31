# Full Project Audit Fixes Implementation Plan

> **For agentic workers:** REQUIRED: Use superpowers:subagent-driven-development (if subagents available) or superpowers:executing-plans to implement this plan. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** 修复全量审计确认的数据库、芯之力、课堂、人声、推送和后台 E2E 缺陷。

**Architecture:** 保持现有模块边界，在数据库 schema、实时会话 orchestrator、课堂 service、voice service、push provider 和后台 GenerationStep 内做最小修复。每项先补回归测试，再实现并独立提交。

**Tech Stack:** Go 1.x、PostgreSQL、Gin/WebSocket、Vue 3、Vitest、Playwright、Flutter 协议测试。

---

### Task 1: Legacy daily quiz migration

**Files:**
- Modify: `nx-backend/apps/server/internal/db/schema.sql`
- Modify: `nx-backend/apps/server/internal/db/schema_profile_calibration_test.go`
- Create/Modify: `nx-backend/apps/server/internal/db/*profile_calibration*_integration_test.go`

- [ ] 写旧表缺 `type_weights` 的失败迁移测试。
- [ ] 运行测试确认 SQLSTATE 42703/缺列。
- [ ] 添加幂等 ALTER、回填、default、NOT NULL。
- [ ] 运行 db 测试和目标 profile calibration 测试。
- [ ] 提交。

### Task 2: Xinzhili strategy orchestration

**Files:**
- Modify: `nx-backend/apps/server/internal/xinzhili/session.go`
- Modify: `nx-backend/apps/server/internal/xinzhili/strategy.go`（仅在接口需要时）
- Modify/Create: `nx-backend/apps/server/internal/xinzhili/*test.go`

- [ ] 写 partial、silence、Tick、FinishInput、Action 执行的失败测试。
- [ ] 接入 ticker 和统一 Action executor。
- [ ] 保证首段 stable 不会绕过配置静音策略。
- [ ] 跑 xinzhili 测试与 race 测试。
- [ ] 提交。

### Task 3: Realtime config version and session race

**Files:**
- Modify: `nx-backend/apps/server/internal/server/app_xinzhili_realtime.go`
- Modify/Create: `nx-backend/apps/server/internal/server/app_xinzhili_realtime*_test.go`

- [ ] 写配置热更新事件和旧连接替换竞态测试。
- [ ] 每轮同步 `configVersion` 并广播 `session.config_changed`/mode snapshot。
- [ ] 锁内原子安装 session，关闭后拒绝迟到安装。
- [ ] 跑 server 测试与 race 测试。
- [ ] 提交。

### Task 4: Classroom consistency and cleanup

**Files:**
- Modify: `nx-backend/apps/server/internal/server/classroom_admin.go`
- Modify: `nx-backend/apps/server/internal/classroom/upload.go`
- Modify/Create: 对应测试文件

- [ ] 写审计失败不反转成功、幂等查询错误传播、poison cleanup 继续处理测试。
- [ ] 将事后审计改为记录失败但不改变已提交业务响应。
- [ ] 传播完成路径 repo 错误。
- [ ] 清理器继续处理并聚合错误。
- [ ] 跑课堂/server 测试。
- [ ] 提交。

### Task 5: Voice clone concurrency and failure semantics

**Files:**
- Modify: `nx-backend/apps/server/internal/voice/voice.go`
- Modify: voice repository/store files if CAS helper is missing
- Modify/Create: `nx-backend/apps/server/internal/voice/*test.go`

- [ ] 写并发双请求仅一次供应商调用测试。
- [ ] 写供应商失败不会返回假成功测试。
- [ ] 复用/扩展原子 claim。
- [ ] 返回稳定可机读错误与 failed profile 状态。
- [ ] 跑 voice 测试与 race 测试。
- [ ] 提交。

### Task 6: Push production routing and invalid tokens

**Files:**
- Modify: `nx-backend/apps/server/internal/push/*`
- Modify/Create: push tests

- [ ] 写 APNs production payload 测试。
- [ ] 写无效 registration ID 隔离/清退测试。
- [ ] 实现环境标记和错误解析。
- [ ] 跑 push 测试。
- [ ] 提交。

### Task 7: Admin version drawer focus

**Files:**
- Modify: `nx-backend/apps/web-antd/src/views/video/projects/workflow/GenerationStep.vue`
- Modify/Create: component test
- Modify: `nx-backend/apps/web-antd/e2e/guided-video-workflow.spec.ts` only if assertion needs stable identity

- [ ] 写筛选切换后焦点回退测试。
- [ ] 保存触发元素并实现可见按钮 fallback。
- [ ] 跑 Vitest、Playwright 目标用例重复 3 次、类型检查和构建。
- [ ] 提交。

### Task 8: Deployment defaults and full verification

**Files:**
- Modify: `.env.example`
- Modify: `docker-compose.yml`
- Modify: `DEPLOY.md`
- Modify: `nx-backend/apps/server/.env.example`
- Modify/Create: deployment contract tests

- [ ] 写生产默认值不得引用硅基流动/旧 MiniMax 克隆路径的失败测试。
- [ ] 更新默认配置和部署说明为百炼公共凭证方案。
- [ ] 执行 Go 全量测试、vet、race、后台单测/构建/E2E、小程序测试/构建、官网/H5 测试/构建。
- [ ] 做最终代码审查并提交。
