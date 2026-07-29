# MiniMax 音色复制到阿里百炼 Implementation Plan

> **For agentic workers:** REQUIRED: Use superpowers:subagent-driven-development (if subagents available) or superpowers:executing-plans to implement this plan. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** 让已有 MiniMax 人声档案复用原音频样本，一键生成阿里百炼托管 MiniMax 音色，并在芯之力 TTS 配置中选择。

**Architecture:** 新增后端专用复制接口，后端按样本资产做事务级去重，复用现有 `CreateProfile`/`CloneProfile` 和 Bailian clone client；前端在人声列表提供按行复制操作，芯之力现有 provider 过滤逻辑继续消费 `GET /voice/options`。原 MiniMax 档案始终保留。

**Tech Stack:** Go HTTP server、PostgreSQL、Vue 3 + Ant Design Vue、Vitest、Go tests。

---

## Task 1: 后端复制服务与接口

**Files:**
- Modify: `nx-backend/apps/server/internal/voice/voice.go`
- Modify: `nx-backend/apps/server/internal/server/server.go`
- Test: `nx-backend/apps/server/internal/server/voice_profile_copy_test.go`
- Test: `nx-backend/apps/server/internal/voice/voice_test.go`

- [ ] **Step 1: 写失败测试**
  - 为 `POST /api/voice/profiles/{id}/copy-to-bailian` 增加路由与权限契约测试。
  - 覆盖源档案为 MiniMax、样本资产被复用、原档案保持不变、返回新档案 provider 为 `bailian`。
  - 覆盖同一 `sample_asset_id` 重复请求只返回已有百炼档案。
  - 覆盖百炼档案 `failed` 时复用原档案重试。

- [ ] **Step 2: 运行后端失败测试**

  Run: `cd nx-backend/apps/server && go test ./internal/server ./internal/voice -run 'CopyProfile|copy-to-bailian'`

  Expected: FAIL，因为复制路由和 Store 方法尚不存在。

- [ ] **Step 3: 实现 Store 复制方法**
  - 新增 `CopyProfileToBailian(ctx, sourceID)`。
  - 校验源档案 provider 为 `minimax`、样本资产存在且可读取。
  - 使用事务和按样本资产的数据库锁完成“查询现有百炼档案 / 插入新档案”去重。
  - 新档案复制 `sample_asset_id`、`sample_url`、`sample_name`，名称追加“（百炼）”，provider 为 `bailian`。
  - 对 `draft`/`failed` 档案调用现有克隆流程，`ready`/`cloning` 返回当前记录。
  - 不修改源 MiniMax 档案。

- [ ] **Step 4: 实现 HTTP 路由**
  - 在 `server.go` 注册 `/api/voice/profiles/{id}/copy-to-bailian`，权限为 `Voice:Profile:Manage`。
  - 保留原 `/api/voice/profiles/{id}` POST 重试克隆行为。
  - 返回统一 JSON 错误和档案对象。

- [ ] **Step 5: 运行后端测试**

  Run: `cd nx-backend/apps/server && go test ./internal/server ./internal/voice -run 'CopyProfile|copy-to-bailian|BailianClone'`

  Expected: PASS。

- [ ] **Step 6: Commit**

  ```bash
  git add nx-backend/apps/server/internal/voice/voice.go nx-backend/apps/server/internal/server/server.go nx-backend/apps/server/internal/server/voice_profile_copy_test.go nx-backend/apps/server/internal/voice/voice_test.go
  git commit -m "feat: copy minimax voice profiles to bailian"
  ```

## Task 2: 人声管理 UI 入口

**Files:**
- Modify: `nx-backend/apps/web-antd/src/api/core/voice.ts`
- Modify: `nx-backend/apps/web-antd/src/views/voice/profiles.vue`
- Test: `nx-backend/apps/web-antd/src/views/voice/profiles.provider-platform.test.ts`

- [ ] **Step 1: 写失败测试**
  - 断言 API 暴露 `copyVoiceProfileToBailianApi`。
  - 断言 MiniMax 行存在“复制到百炼”文案、确认提示和调用 API。
  - 断言百炼行不显示该按钮，缺少样本的 MiniMax 行不显示该按钮。

- [ ] **Step 2: 运行 UI 失败测试**

  Run: `cd nx-backend && pnpm exec vitest run --dom apps/web-antd/src/views/voice/profiles.provider-platform.test.ts`

  Expected: FAIL，因为 API 和按钮尚不存在。

- [ ] **Step 3: 实现 API 与交互**
  - 增加 `copyVoiceProfileToBailianApi(id)`，请求 `POST /voice/profiles/{id}/copy-to-bailian`，超时沿用 180 秒。
  - 增加当前行 loading 状态。
  - MiniMax 且有样本的行显示“复制到百炼”；点击确认后调用接口、刷新列表并提示结果。
  - 失败时显示后端错误，保留原列表状态。

- [ ] **Step 4: 运行 UI 测试**

  Run: `cd nx-backend && pnpm exec vitest run --dom apps/web-antd/src/views/voice/profiles.provider-platform.test.ts`

  Expected: PASS。

- [ ] **Step 5: Commit**

  ```bash
  git add nx-backend/apps/web-antd/src/api/core/voice.ts nx-backend/apps/web-antd/src/views/voice/profiles.vue nx-backend/apps/web-antd/src/views/voice/profiles.provider-platform.test.ts
  git commit -m "feat: add bailian voice copy action"
  ```

## Task 3: 芯之力选择链路验收

**Files:**
- Modify: `nx-backend/apps/web-antd/src/views/settings/xinzhili-model.vue` only if an uncovered state is found.
- Test: existing `nx-backend/apps/web-antd/src/views/settings/xinzhili-model.test.ts` or the nearest model-config contract test.

- [ ] **Step 1: 写/补充失败测试**
  - 覆盖选择 `provider=bailian` 的 ready clone 后，`tts.provider` 为 `bailian`、`tts.voice` 使用百炼 voiceId。
  - 覆盖 MiniMax 与百炼选项互相过滤，OpenAI 兼容协议隐藏现有音色选择。

- [ ] **Step 2: 运行测试并按结果补最小修复**

  Run: `cd nx-backend && pnpm exec vitest run --dom apps/web-antd/src/views/settings`

  Expected: 现有逻辑通过；只有发现缺口时才修改页面。

- [ ] **Step 3: Commit（仅有修改时）**

  ```bash
  git add nx-backend/apps/web-antd/src/views/settings
  git commit -m "test: verify bailian voice selection in xinzhili"
  ```

## Task 4: 全量验证与本地页面检查

- [ ] **Step 1: 运行相关前端测试**

  Run: `cd nx-backend && pnpm exec vitest run --dom apps/web-antd/src/views/voice apps/web-antd/src/views/settings`

- [ ] **Step 2: 运行后端相关测试**

  Run: `cd nx-backend/apps/server && go test ./internal/server ./internal/voice ./internal/xinzhili ./internal/modelconfig`

- [ ] **Step 3: 类型检查**

  Run: `cd nx-backend && pnpm --filter @vben/web-antd run typecheck`

- [ ] **Step 4: 浏览器验收**
  - 打开人声管理，确认 MiniMax 档案出现“复制到百炼”。
  - 执行复制后确认新档案平台为阿里百炼、状态最终为可使用。
  - 打开 `/settings/xinzhili-model`，选择“阿里百炼”，确认新音色出现在选择框。
  - 保存后确认 `tts.voice` 为百炼 voiceId，而不是 `clone:<id>`。

- [ ] **Step 5: 检查 Git 状态**

  Run: `git diff --check && git status --short --branch`

  Expected: 本功能文件无未提交改动；并行任务文件不纳入本功能提交。
