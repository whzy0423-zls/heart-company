# 芯之力 App/后台兼容性修复 Implementation Plan

> **For agentic workers:** REQUIRED: Use superpowers:subagent-driven-development (if subagents available) or superpowers:executing-plans to implement this plan. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** 修复当前后台与手机 App 芯之力实时语音的 capabilities、generation、v1 解码和 provider 默认值兼容问题。

**Architecture:** 在现有 Go server/mux、xinzhili protocol 和 realtime sink 边界内做最小增量修改，不替换既有 TTS 或 App 协议。通过服务端能力接口预检，连接级 generation 贯穿 JSON/二进制事件，协议层对已发布 v1 做向后兼容。

**Tech Stack:** Go、net/http、gorilla/websocket、encoding/json、Go test；Flutter App 现有协议测试作为客户端回归。

---

### Task 1: Capabilities endpoint

**Files:**
- Create: `nx-backend/apps/server/internal/xinzhili/capabilities.go`
- Create: `nx-backend/apps/server/internal/server/app_xinzhili_capabilities.go`
- Modify: `nx-backend/apps/server/internal/server/server.go`
- Test: `nx-backend/apps/server/internal/server/app_xinzhili_capabilities_test.go`
- Test: `nx-backend/apps/server/internal/xinzhili/capabilities_test.go`

- [ ] Write route and payload failing tests.
- [ ] Run focused tests and confirm missing route/payload failure.
- [ ] Implement `DefaultRealtimeCapabilities` and authenticated GET handler.
- [ ] Register `/api/app/xinzhili/realtime/capabilities` before WebSocket route.
- [ ] Run focused tests and commit.

### Task 2: Connection generation propagation

**Files:**
- Modify: `nx-backend/apps/server/internal/server/app_xinzhili_realtime.go`
- Test: `nx-backend/apps/server/internal/server/app_xinzhili_realtime_test.go`

- [ ] Add failing tests asserting session.ready and assistant MP3 generation equals client generation.
- [ ] Run tests and confirm failure.
- [ ] Store the client generation at session start and inject it into all server control/audio events.
- [ ] Run focused tests and commit.

### Task 3: Published v1 decoder compatibility

**Files:**
- Modify: `nx-backend/apps/server/internal/xinzhili/protocol.go`
- Test: `nx-backend/apps/server/internal/xinzhili/protocol_test.go`
- Modify: `nx-backend/apps/server/internal/server/app_xinzhili_realtime.go` (sanitized protocol error logging/message if required)
- Test: `nx-backend/apps/server/internal/server/app_xinzhili_realtime_test.go`

- [ ] Add failing tests for omitted/defaultable fields and unknown top-level fields.
- [ ] Run tests and confirm failure.
- [ ] Implement minimal defaulting/unknown-field compatibility while preserving core validation.
- [ ] Run focused tests and commit.

### Task 4: Provider defaults and fallback audit

**Files:**
- Modify: `nx-backend/apps/server/internal/server/xinzhili_realtime_config.go` if needed.
- Modify: `nx-backend/apps/server/internal/voice/voice.go` if needed.
- Test: Existing focused Go tests.

- [ ] Add/adjust failing test for Bailian default model.
- [ ] Implement provider-specific defaults only where current behavior is wrong.
- [ ] Verify old MiniMax defaults remain unchanged.

### Task 5: Full verification

- [ ] Run backend focused and package tests.
- [ ] Run App Xinzhili Flutter tests.
- [ ] Inspect diff and route/protocol contract.
- [ ] Commit final compatibility changes without touching `main` or `test`.
