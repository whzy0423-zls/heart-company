# 芯之力实时语音后台与管理端 Implementation Plan

> **For agentic workers:** REQUIRED: Use superpowers:subagent-driven-development (if subagents available) or superpowers:executing-plans to implement this plan. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** 提供可配置、可测试、可取消且会尽早返回首段语音的 ASR→检索→LLM→TTS SSE 服务。

**Architecture:** 扩展现有 `modelconfig` 保存脱敏的芯之力 ASR/TTS 配置；新增 OpenAI-compatible TTS 客户端、分句器和芯之力 turn orchestrator。HTTP handler 复用 App 鉴权、会员判断、主卡画像、RAG 与流式 generator，并在请求取消时停止后续任务。

**Tech Stack:** Go 1.24、net/http、PostgreSQL、Vue 3、Ant Design Vue、Vitest。

---

## Chunk 1: 配置和基础组件

### Task 1: 芯之力模型配置

**Files:**
- Modify: `nx-backend/apps/server/internal/modelconfig/model_config.go`
- Modify: `nx-backend/apps/server/internal/modelconfig/model_config_test.go`
- Modify: `nx-backend/apps/server/internal/server/server.go`
- Modify: `nx-backend/apps/server/internal/server/model_config_test.go`

- [ ] 先写失败测试：默认值、trim、空密钥保留、脱敏视图、必填配置校验。
- [ ] 运行 `go test ./internal/modelconfig ./internal/server -run 'Xinzhili|ModelConfig'` 并确认因字段缺失失败。
- [ ] 最小实现 `XinzhiliVoiceConfig`、ASR/TTS/交互子配置及 HTTP view。
- [ ] 重跑测试并提交 `feat: add xinzhili voice model config`。

### Task 2: OpenAI-compatible ASR/TTS 和分句器

**Files:**
- Create: `nx-backend/apps/server/internal/voice/openai_compatible.go`
- Create: `nx-backend/apps/server/internal/voice/openai_compatible_test.go`
- Create: `nx-backend/apps/server/internal/voice/sentence_chunker.go`
- Create: `nx-backend/apps/server/internal/voice/sentence_chunker_test.go`

- [ ] 先写失败测试：端点拼接、multipart ASR、`/audio/speech` JSON TTS、响应类型、超时/取消和中文标点分句。
- [ ] 运行定向测试确认 RED。
- [ ] 实现最小客户端和保持顺序的句子累积器。
- [ ] 重跑 `go test ./internal/voice` 并提交 `feat: add compatible speech adapters`。

## Chunk 2: SSE 编排与持久化

### Task 3: 独立芯之力会话场景

**Files:**
- Modify: `nx-backend/apps/server/internal/db/schema.sql`
- Modify: `nx-backend/apps/server/internal/chat/store.go`
- Modify: `nx-backend/apps/server/internal/chat/store_test.go`

- [ ] 先写失败测试：同一主卡的普通聊天和 `xinzhili_voice` 返回不同 session，场景内可恢复。
- [ ] 添加 `scene` 列、唯一/查询索引和 `GetOrCreateSceneSession`。
- [ ] 运行 chat/db 测试并提交 `feat: isolate xinzhili voice sessions`。

### Task 4: 芯之力流式 turn endpoint

**Files:**
- Create: `nx-backend/apps/server/internal/server/app_xinzhili_voice.go`
- Create: `nx-backend/apps/server/internal/server/app_xinzhili_voice_test.go`
- Modify: `nx-backend/apps/server/internal/server/server.go`

- [ ] 先写失败测试：会员限制、未配置、空白转写、事件顺序、首个 audio 早于 LLM 完成、TTS 顺序、客户端取消不保存、成功后保存。
- [ ] 注册 `POST /api/app/xinzhili/turns/stream`，先 flush `ready`。
- [ ] 复用主卡画像/记忆/偏好，显式发出知识库和理论库状态，调用 `AskStream`。
- [ ] 分句后串行 TTS，SSE 返回 base64 音频段；完成后保存独立场景问答。
- [ ] 运行 server 测试并提交 `feat: stream xinzhili voice turns`。

### Task 5: 配置测试接口和观测

**Files:**
- Create: `nx-backend/apps/server/internal/server/xinzhili_voice_config_test.go`
- Modify: `nx-backend/apps/server/internal/server/app_xinzhili_voice.go`
- Modify: `nx-backend/apps/server/internal/server/server.go`

- [ ] 先写失败测试：ASR、TTS、完整链路测试接口不会泄露密钥。
- [ ] 实现 `/api/model-config/test-xinzhili-asr`、`test-xinzhili-tts`、`test-xinzhili-chain`。
- [ ] 添加各阶段毫秒日志和稳定错误码。
- [ ] 运行定向测试并提交 `feat: add xinzhili voice diagnostics`。

## Chunk 3: 管理端

### Task 6: 芯之力语音配置表单

**Files:**
- Modify: `nx-backend/apps/web-antd/src/api/core/model-config.ts`
- Modify: `nx-backend/apps/web-antd/src/views/settings/model.vue`
- Modify/Create: `nx-backend/apps/web-antd/src/views/settings/model.test.ts`

- [ ] 先写失败测试：读取/保存/masking/protocol 验证和三个测试按钮。
- [ ] 扩展类型、默认表单和保存 payload；密钥保存后清空输入并刷新 `apiKeySet`。
- [ ] 增加 ASR、TTS、交互、提示词和诊断区域。
- [ ] 运行 `pnpm --filter @vben/web-antd test:unit -- model` 和类型检查。
- [ ] 提交 `feat: configure xinzhili voice models`。

## Chunk 4: 后台验收

### Task 7: 全量验证

- [ ] 运行 `go test ./...`。
- [ ] 运行后台 lint/typecheck/unit/build。
- [ ] 检查 `git diff --check` 和密钥无回显。
- [ ] 更新设计文档中的实测协议差异并提交。
