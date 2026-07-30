# 课堂管理稳定性与百炼音色统一 Implementation Plan

> **For agentic workers:** REQUIRED: Use superpowers:subagent-driven-development (if subagents available) or superpowers:executing-plans to implement this plan. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** 修复课堂管理数据、上传、发布、封面和最近更新问题，并将新音色克隆统一迁移到阿里百炼 Qwen。

**Architecture:** 在现有课堂 API、上传状态机、系列模型和声音管理 provider 抽象上做兼容扩展。危险删除采用归档，批量发布采用单事务，耗时媒体完成采用独立有界上下文，首页最近更新采用服务端聚合。

**Tech Stack:** Go、PostgreSQL、Vue 3、Ant Design Vue、TypeScript、uni-app、Node test、Vitest。

---

## Chunk 1: 编辑器与上传稳定性

### Task 1: 连续新建课件状态重置

**Files:**
- Modify: `nx-backend/apps/web-antd/src/views/classroom/index.vue`
- Modify/Test: `nx-backend/apps/web-antd/src/views/classroom/editor-model.test.ts`

- [ ] 写连续两次新建仍调用 Create 的失败测试。
- [ ] 运行测试并确认因编辑器实例复用而失败。
- [ ] 为弹窗增加销毁策略、create generation key 和关闭重置。
- [ ] 运行课堂前端测试。

### Task 2: 上传完成超时与状态对账

**Files:**
- Modify: `nx-backend/apps/web-antd/src/api/core/classroom.ts`
- Modify: `nx-backend/apps/web-antd/src/views/classroom/upload-flow.ts`
- Modify/Test: `nx-backend/apps/web-antd/src/views/classroom/upload-flow.test.ts`
- Modify: `nx-backend/apps/server/internal/server/classroom_upload.go`
- Modify: `nx-backend/apps/server/internal/classroom/upload.go`
- Modify/Test: `nx-backend/apps/server/internal/server/classroom_upload_test.go`

- [ ] 写 complete 超时后查询状态、completing 不 abort 的失败测试。
- [ ] 写请求取消后仍能完成/失败落库的后端失败测试。
- [ ] 实现 180 秒超时、状态对账及独立有界上下文。
- [ ] 运行上传相关前后端测试。

### Task 3: 上传任务静默轮询

**Files:**
- Modify: `nx-backend/apps/web-antd/src/views/classroom/upload-tasks.vue`
- Modify/Test: `nx-backend/apps/web-antd/src/views/classroom/upload-tasks-model.ts`
- Test: `nx-backend/apps/web-antd/src/views/classroom/upload-tasks-model.test.ts`

- [ ] 写旧响应丢弃、首次骨架与静默刷新失败测试。
- [ ] 提取轮询状态模型并实现请求序号控制。
- [ ] 用递归定时器替换重叠 interval，并按页面可见性/终态暂停。
- [ ] 运行上传任务前端测试。

## Chunk 2: 删除与发布

### Task 4: 下架课件归档删除

**Files:**
- Modify: `nx-backend/apps/server/internal/classroom/models.go`
- Modify: `nx-backend/apps/server/internal/classroom/store.go`
- Modify: `nx-backend/apps/server/internal/server/classroom_admin.go`
- Modify/Test: `nx-backend/apps/server/internal/server/classroom_admin_test.go`
- Modify: `nx-backend/apps/web-antd/src/views/classroom/index.vue`

- [ ] 写 offline 删除转 archived、公开查询排除 archived 的失败测试。
- [ ] 实现状态常量、存储过滤和删除分支。
- [ ] 为 offline 状态展示删除操作。
- [ ] 运行课堂后端及前端测试。

### Task 5: 原子批量发布

**Files:**
- Modify: `nx-backend/apps/server/internal/server/routes.go`
- Modify: `nx-backend/apps/server/internal/server/classroom_admin.go`
- Modify/Test: `nx-backend/apps/server/internal/server/classroom_admin_test.go`
- Modify: `nx-backend/apps/web-antd/src/api/core/classroom.ts`
- Modify: `nx-backend/apps/web-antd/src/views/classroom/index.vue`

- [ ] 写批量成功、失败回滚、重复 ID 和数量限制测试。
- [ ] 实现显式路由与单事务服务逻辑。
- [ ] 增加表格选择和批量发布按钮。
- [ ] 运行批量发布测试。

## Chunk 3: 系列封面与最近更新

### Task 6: 系列封面管理

**Files:**
- Modify: `nx-backend/apps/server/internal/db/schema.sql`
- Modify: `nx-backend/apps/server/internal/classroom/models.go`
- Modify: `nx-backend/apps/server/internal/classroom/store.go`
- Modify: `nx-backend/apps/server/internal/server/classroom_admin.go`
- Modify: `nx-backend/apps/server/internal/server/classroom_public.go`
- Modify/Test: `nx-backend/apps/server/internal/server/classroom_*_test.go`
- Modify: `nx-backend/apps/web-antd/src/api/core/classroom.ts`
- Modify: `nx-backend/apps/web-antd/src/views/classroom/series.vue`
- Modify: `nx-backend/apps/web-antd/src/views/classroom/series-model.ts`

- [ ] 写封面优先级、比例默认值和上传删除 API 失败测试。
- [ ] 增加数据库字段、模型映射、签名解析及 API。
- [ ] 后台系列弹窗增加预览、上传、删除和比例选择。
- [ ] 运行系列相关测试。

### Task 7: 最近更新聚合

**Files:**
- Modify: `nx-backend/apps/server/internal/classroom/store.go`
- Modify: `nx-backend/apps/server/internal/server/classroom_public.go`
- Modify/Test: `nx-backend/apps/server/internal/server/classroom_public_test.go`
- Modify: `miniapp/src/api/classroom.js`
- Modify: `miniapp/src/pages/index/index.vue`
- Modify: `miniapp/src/pages/classroom/classroom.vue`
- Modify/Test: `miniapp/src/pages/index/index.test.mjs`
- Modify/Test: `miniapp/src/pages/classroom/classroom.test.mjs`

- [ ] 写系列与独立课件按最近发布时间混排的失败测试。
- [ ] 实现 `/api/public/classroom/recent`。
- [ ] 首页根据 itemType 渲染和跳转，课堂页支持 seriesId 自动展开。
- [ ] 运行小程序与公共课堂测试。

## Chunk 4: 百炼 Qwen 音色统一

### Task 8: Qwen 克隆与芯之力 TTS 兼容

**Files:**
- Modify: `nx-backend/apps/server/internal/voice/bailian.go`
- Modify: `nx-backend/apps/server/internal/voice/voice.go`
- Modify/Test: `nx-backend/apps/server/internal/voice/*_test.go`
- Modify: `nx-backend/apps/server/internal/xinzhili/*tts*.go`
- Modify/Test: `nx-backend/apps/server/internal/xinzhili/*tts*_test.go`
- Modify: `nx-backend/apps/web-antd/src/views/voice/profiles.vue`
- Modify: `nx-backend/apps/web-antd/src/views/settings/xinzhili-model/*`

- [ ] 根据官方 Qwen 接口写克隆、合成和迁移失败测试。
- [ ] 新建入口只保留百炼 Qwen，旧 MiniMax 档案保留迁移能力。
- [ ] 新增 Qwen TTS 运行时分支并保留旧 provider 兼容。
- [ ] 芯之力配置保存最终 voiceId、模型和 provider。
- [ ] 运行 voice、xinzhili 和后台配置测试。

## Chunk 5: 集成验证

### Task 9: 全量验证与本地运行

- [ ] 运行 Go 相关包及全量测试。
- [ ] 运行后台课堂、上传、声音管理 Vitest。
- [ ] 运行小程序课堂测试。
- [ ] 运行 `git diff --check` 和构建。
- [ ] 使用本地非 Docker 方式启动后端、后台和小程序，验证关键页面/API。
- [ ] 审查未触碰 `test` 分支和主工作区未提交修改。

