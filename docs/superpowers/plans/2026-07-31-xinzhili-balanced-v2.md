# 芯之力均衡提速 V2 Implementation Plan

> **For agentic workers:** REQUIRED: Use superpowers:subagent-driven-development (if subagents available) or superpowers:executing-plans to implement this plan. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** 缩短芯之力从用户停说到首段回复播放的耗时，同时保持四种模式语义和现有 WebSocket/TTS 顺序协议。

**Architecture:** 在策略引擎内增加完整句状态并降低默认端点阈值；在会话生成入口并行聚合五类上下文；仅在流式 TTS chunker 中采用 10/20/36 分块。所有变化局限于 `internal/xinzhili`，保留既有协议层和双路有序合成器。

**Tech Stack:** Go、标准库 `sync`、现有 xinzhili 单元测试与 fake providers。

---

## Chunk 1: 策略端点

### Task 1: 默认阈值与完整句快速端点

**Files:**
- Modify: `nx-backend/apps/server/internal/xinzhili/config.go`
- Modify: `nx-backend/apps/server/internal/xinzhili/config_test.go`
- Modify: `nx-backend/apps/server/internal/xinzhili/strategy.go`
- Modify: `nx-backend/apps/server/internal/xinzhili/strategy_test.go`

- [ ] **Step 1: 写失败测试**

更新默认 timing 断言为 250/350/700/1000；增加正常/抬杠完整句在收到静音后立即端点、安慰/深度倾听仍等待阈值、闭合引号识别、重置后不残留完整句状态的测试。

- [ ] **Step 2: 验证 RED**

Run: `go test ./internal/xinzhili -run 'TestDefault|CompleteSentence|StrongSentence' -count=1`

Expected: FAIL，当前默认值较慢且策略没有完整句快速端点。

- [ ] **Step 3: 最小实现**

将默认 timing 改为目标值；在 `Engine` 增加完整句布尔状态，稳定文本时识别强结束符与闭合引号，在 `endpointAction` 中仅对 normal/argument 且已收到 silence 的完整句立即返回 `ActionEndpoint`，在 turn 清理时复位。

- [ ] **Step 4: 验证 GREEN**

Run: `go test ./internal/xinzhili -count=1`

- [ ] **Step 5: 提交**

```bash
git add nx-backend/apps/server/internal/xinzhili/{config.go,config_test.go,strategy.go,strategy_test.go}
git commit -m "优化：缩短芯之力模式端点等待"
```

## Chunk 2: 回答前准备

### Task 2: 并行读取生成上下文

**Files:**
- Modify: `nx-backend/apps/server/internal/xinzhili/session.go`
- Modify: `nx-backend/apps/server/internal/xinzhili/session_test.go`

- [ ] **Step 1: 写失败测试**

增加带启动通知和统一释放 barrier 的 fake providers。启动一轮生成后，断言历史、偏好、记忆、知识库、理论库五个调用都能在 barrier 释放前开始，并验证传给 generator 的内容及知识库/理论库顺序不变。

- [ ] **Step 2: 验证 RED**

Run: `go test ./internal/xinzhili -run TestGenerationLoadsContextInParallel -count=1`

Expected: FAIL 或超时，因为当前串行调用会阻塞在第一个 provider。

- [ ] **Step 3: 最小实现**

在 `startGeneration` goroutine 内以五个 goroutine 和 `sync.WaitGroup` 分别读取数据，错误保持为空结果，等待全部结束后按原顺序构造 documents、sources 和 `rag.GenerateInput`。

- [ ] **Step 4: 验证 GREEN 与 race**

Run: `go test -race ./internal/xinzhili -run TestGenerationLoadsContextInParallel -count=1`

- [ ] **Step 5: 提交**

```bash
git add nx-backend/apps/server/internal/xinzhili/session.go nx-backend/apps/server/internal/xinzhili/session_test.go
git commit -m "优化：并行加载芯之力回答上下文"
```

## Chunk 3: 首段语音

### Task 3: 调整流式 TTS 分块

**Files:**
- Modify: `nx-backend/apps/server/internal/xinzhili/session.go`
- Modify: `nx-backend/apps/server/internal/xinzhili/session_test.go`

- [ ] **Step 1: 写失败测试**

将 chunker 测试改为：10 字后弱标点切分、首段 20 字硬切、后续 36 字硬切，并保留短弱标点等待和强句末优先行为。

- [ ] **Step 2: 验证 RED**

Run: `go test ./internal/xinzhili -run TestStreamSentenceChunkerPrioritizesPlayableFirstChunk -count=1`

Expected: FAIL，当前值为 14/28/42。

- [ ] **Step 3: 最小实现**

设置 `firstTTSChunkMinRunes = 10`、`firstTTSChunkMaxRunes = 20`、`streamTTSChunkMaxRunes = 36`，后续流式 chunk 使用专用上限。

- [ ] **Step 4: 验证 GREEN**

Run: `go test ./internal/xinzhili -count=1`

- [ ] **Step 5: 提交**

```bash
git add nx-backend/apps/server/internal/xinzhili/session.go nx-backend/apps/server/internal/xinzhili/session_test.go
git commit -m "优化：提前生成芯之力首段语音"
```

## Chunk 4: 集成验证

### Task 4: 全量验证与集成

**Files:**
- Verify: `nx-backend/apps/server/internal/xinzhili`
- Verify: `nx-backend/apps/server/internal/server`

- [ ] **Step 1: 格式化和局部测试**

Run: `gofmt -w internal/xinzhili/*.go && go test ./internal/xinzhili -count=1`

- [ ] **Step 2: race 验证**

Run: `go test -race ./internal/xinzhili ./internal/server -count=1`

- [ ] **Step 3: 全仓验证**

Run: `go test ./... -count=1 && go vet ./...`

- [ ] **Step 4: 合并和配置同步**

在后台 `main` 合并本分支；通过现有配置 GET 获取完整配置并 PUT 回写，仅将 `argumentCandidateSilenceMs/normalEndSilenceMs/comfortEndSilenceMs/deepListeningEndSilenceMs` 更新为 `250/350/700/1000`，保留密钥和其他字段。

- [ ] **Step 5: 非 Docker 重启与真机验证**

使用 `.local-logs/run-local-dev.sh` 重启本地服务，确认 5320/5666 可访问；真机依次连续测试正常、抬杠、安慰、深度倾听模式，检查首段速度、无片段超时且播放顺序完整。

- [ ] **Step 6: 推送**

Run: `git push origin main`
