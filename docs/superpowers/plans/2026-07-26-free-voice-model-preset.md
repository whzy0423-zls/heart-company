# 免费语音模型预设 Implementation Plan

> **For agentic workers:** REQUIRED: Use superpowers:subagent-driven-development (if subagents available) or superpowers:executing-plans to implement this plan. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** 在后台芯之力语音配置中提供经过验证的 SiliconFlow 免费额度 ASR/TTS 一键预设，并保留现有密钥安全语义。

**Architecture:** 预设逻辑放在后台页面的纯函数中，按钮只写入非敏感表单字段。后端继续使用现有 OpenAI 兼容 SpeechModelConfig 和密钥保留逻辑，不引入新的协议分支。

**Tech Stack:** Vue 3、Ant Design Vue、Vitest/Node source-contract tests、Go。

---

## Chunk 1: 预设与验证

### Task 1: 免费语音预设纯函数

**Files:**
- Modify: `nx-backend/apps/web-antd/src/views/settings/model.vue`
- Test: `nx-backend/apps/web-antd/src/views/settings/model.xinzhili-voice.test.ts`

- [ ] 写一个失败测试，要求页面声明 SiliconFlow ASR、TTS、音色和 API Base 预设，且不会覆盖 API Key。
- [ ] 运行测试并确认因预设缺失而失败。
- [ ] 实现最小预设常量、应用函数和“一键使用免费预设”按钮。
- [ ] 运行测试确认通过。

### Task 2: 后端配置契约回归

**Files:**
- Test: `nx-backend/apps/server/internal/modelconfig/model_config_test.go`

- [ ] 增加免费预设配置可通过 Normalize/ValidateReady 且局部更新保留 ASR/TTS Key 的测试。
- [ ] 运行测试确认当前后端契约通过；如发现缺口，再做最小修复。

### Task 3: 全量验证与提交

- [ ] 运行 `go test ./internal/modelconfig ./internal/server`。
- [ ] 运行后台相关前端测试或静态契约测试。
- [ ] 运行 `go test ./...` 和 `git diff --check`。
- [ ] 提交并推送功能分支。
