# App Chat ASR Config Compatibility Implementation Plan

> **For agentic workers:** REQUIRED: Use superpowers:subagent-driven-development (if subagents available) or superpowers:executing-plans to implement this plan. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** 让 App 普通会话复用后台已保存的 OpenAI-compatible ASR 配置，并保留现有环境变量兜底。

**Architecture:** 在 `recognizeSpeech` 调用上游前增加单一配置解析入口。解析器优先将 `model_config.xinzhiliVoice.asr` 映射为现有 `config.ASRConfig`，配置不完整或存储读取异常时返回 `s.env.ASR`；请求构造、网络防护与响应解析保持不变。

**Tech Stack:** Go、PostgreSQL `site_configs`、现有 `modelconfig` 与 server 单元测试。

---

## Chunk 1: Runtime compatibility

### Task 1: Add regression tests

**Files:**
- Modify: `nx-backend/apps/server/internal/server/app_voice_test.go`

- [ ] 写测试：后台完整 ASR 配置覆盖环境变量，并验证请求使用后台地址、密钥、模型和超时。
- [ ] 写测试：后台配置不完整或读取失败时继续使用环境变量。
- [ ] 运行 `go test ./internal/server -run 'TestRecognizeSpeech.*StoredASR' -count=1`，确认测试因缺少动态解析而失败。

### Task 2: Implement dynamic ASR resolver

**Files:**
- Modify: `nx-backend/apps/server/internal/server/server.go`
- Modify: `nx-backend/apps/server/internal/server/app_voice.go`

- [ ] 为测试和运行时增加窄接口/加载钩子，不改变公开 HTTP 接口。
- [ ] 实现后台 ASR 完整性检查、映射及环境变量回退。
- [ ] 让 `recognizeSpeech` 使用解析后的当前配置。
- [ ] 重新运行定向测试并确认通过。

### Task 3: Regression verification

**Files:**
- Verify only

- [ ] 运行 `go test ./internal/server -run 'TestRecognizeSpeech|TestVoiceChat' -count=1`。
- [ ] 运行 `go test ./internal/modelconfig ./internal/xinzhili ./internal/server -count=1`。
- [ ] 运行 `git diff --check` 并检查未触碰 App WebSocket 实时语音实现。

