# 成长技能库生命周期操作实施计划

> **For agentic workers:** REQUIRED: Use superpowers:subagent-driven-development (if subagents available) or superpowers:executing-plans to implement this plan. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** 为成长技能库增加发布、下架、启用和停用的可用管理操作。

**Architecture:** 后端在现有技能库管理路由下增加四个事务动作，复用 `latest_published_version_id` 作为发布指针；前端通过动作 API 调用并按技能状态渲染操作按钮。版本数据保留，动作只改变生命周期字段。

**Tech Stack:** Go、PostgreSQL、Vue 3、Ant Design Vue、Vitest。

---

### Task 1: 后端动作契约

**Files:**
- Modify: `apps/server/internal/server/skill_library_admin.go`
- Test: `apps/server/internal/server/skill_library_admin_test.go`

- [ ] 写四个动作路由和状态约束的失败测试。
- [ ] 运行 `go test ./internal/server -run SkillLibrary -count=1` 确认测试先失败。
- [ ] 实现动作解析、权限保护及事务更新。
- [ ] 运行专项 Go 测试确认通过。

### Task 2: 前端 API 与操作列

**Files:**
- Modify: `apps/web-antd/src/api/core/skill-library-management.ts`
- Modify: `apps/web-antd/src/views/app/skill-library-management.vue`
- Test: `apps/web-antd/src/api/core/skill-library-management.test.ts`
- Test: `apps/web-antd/src/views/app/skill-library-management.test.ts`

- [ ] 先增加四个 API 调用和按钮契约测试。
- [ ] 运行 `pnpm exec vitest run --dom ...` 确认新增断言失败。
- [ ] 实现 API、确认弹窗、动作 loading、状态条件和刷新。
- [ ] 运行前端专项测试确认通过。

### Task 3: 回归验证

- [ ] 运行后端技能库相关测试。
- [ ] 运行前端技能库 API/页面测试。
- [ ] 检查 `git diff --check` 与工作区状态。
