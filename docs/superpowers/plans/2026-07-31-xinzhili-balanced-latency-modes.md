# 芯之力均衡提速与四模式恢复 Implementation Plan

> **For agentic workers:** REQUIRED: Use superpowers:subagent-driven-development (if subagents available) or superpowers:executing-plans to implement this plan. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** 恢复芯之力四种模式，并在保持多轮语音稳定性的前提下降低端点判定、首句生成和 TTS 首段播放延迟。

**Architecture:** 后台配置继续作为模式和时间参数的唯一权威来源；实时会话在流式生成阶段采用首块优先分句，并用有界、按序提交的 TTS 调度器合成短块。App 播放队列使用显式 `turnKey` 隔离轮次，服务端二进制帧也使用所属轮次的固定 `turnKey`，避免跨轮状态污染。

**Tech Stack:** Go、PostgreSQL、Vue 3/TypeScript、Flutter/Dart、WebSocket、Paraformer、DashScope TTS。

---

## Chunk 1: 多轮音频稳定性

### Task 1: App 播放队列支持新轮片段序号复位

**Files:**
- Modify: `/Users/wohenzaiyi/Desktop/nine-xing-app/lib/features/xinzhili/services/xinzhili_playback_queue.dart`
- Modify: `/Users/wohenzaiyi/Desktop/nine-xing-app/lib/features/xinzhili/services/xinzhili_realtime_session.dart`
- Test: `/Users/wohenzaiyi/Desktop/nine-xing-app/test/features/xinzhili/services/xinzhili_playback_queue_test.dart`
- Test: `/Users/wohenzaiyi/Desktop/nine-xing-app/test/features/xinzhili/services/xinzhili_realtime_session_test.dart`

- [ ] **Step 1: 用显式轮次测试替换现有启发式测试**

当前工作区已有“空闲时 seq0 自动复位”的启发式修复和同名测试。保留该测试意图但升级为两个不同 `turnKey` 的轮次；增加旧轮仍在播放、旧完成回调迟到的场景。调用新一轮 startTurn 后，不触发旧播放器自然完成回调，旧 playback future 也必须立即变为 `interrupted`，随后新轮可进入播放并 ACK 新轮。新测试在启发式实现上必须失败。

- [ ] **Step 2: 验证测试因第二轮被判旧片段而失败**

Run:

```bash
cd /Users/wohenzaiyi/Desktop/nine-xing-app
flutter test test/features/xinzhili/services/xinzhili_playback_queue_test.dart \
  --plain-name 'realtime queue accepts segment zero again for the next turn'
flutter test test/features/xinzhili/services/xinzhili_realtime_session_test.dart \
  --plain-name 'acks the new turn when the previous playback callback arrives late'
```

Expected: FAIL，启发式无法在旧轮仍活跃时可靠切换，或迟到回调污染新轮。

- [ ] **Step 3: 最小实现显式轮次边界**

给 `XinzhiliMemoryAudioSegment` 增加 `turnKey`。RealtimeSession 在 startTurn 时递增播放 epoch 并主动调用 `_playback.stop()`，保存轮次切换 barrier；旧 `_drainPlayback` 因 interrupted 退出。新轮音频可以先进入 `_completed`，但 drain 在 enqueue 前必须等待该 barrier，并再次核对 epoch/turnId。队列按新 key 清空旧 pending、复位 `_nextSeq`；旧 stop 或播放完成的迟到回调不得影响新轮。RealtimeSession 构造音频段和发送 ACK 时保留该轮 turnId/turnKey。

- [ ] **Step 4: 跑播放队列与实时会话测试**

```bash
cd /Users/wohenzaiyi/Desktop/nine-xing-app
flutter test \
  test/features/xinzhili/services/xinzhili_playback_queue_test.dart \
  test/features/xinzhili/services/xinzhili_realtime_session_test.dart
```

Expected: PASS。

- [ ] **Step 5: 提交 App 修复**

```bash
cd /Users/wohenzaiyi/Desktop/nine-xing-app
git add lib/features/xinzhili/services/xinzhili_playback_queue.dart \
  lib/features/xinzhili/services/xinzhili_realtime_session.dart \
  test/features/xinzhili/services/xinzhili_playback_queue_test.dart \
  test/features/xinzhili/services/xinzhili_realtime_session_test.dart
git commit -m '修复：隔离芯之力多轮内存播放状态'
```

### Task 2: 服务端音频帧绑定固定轮次标识

**Files:**
- Modify: `nx-backend/apps/server/internal/xinzhili/session.go`
- Modify: `nx-backend/apps/server/internal/xinzhili/tts.go`
- Modify: `nx-backend/apps/server/internal/server/app_xinzhili_realtime.go`
- Test: `nx-backend/apps/server/internal/server/app_xinzhili_realtime_test.go`
- Test: `nx-backend/apps/server/internal/xinzhili/session_test.go`

- [ ] **Step 1: 写失败测试模拟连接全局 turnKey 改变**

测试构造旧轮音频片段，在发送前改变连接当前 `turnKey`，期望二进制帧仍携带片段所属旧轮固定 key。

- [ ] **Step 2: 运行测试确认当前读取全局值而失败**

```bash
cd nx-backend/apps/server
go test ./internal/server -run 'TestXinzhili.*Audio.*TurnKey' -count=1
```

Expected: FAIL，actual key 等于连接当前 key。

- [ ] **Step 3: 将 turnKey 放入进程内音频段元数据**

为 `StartTurnInput` 增加 TurnKey，由 WebSocket startTurn 传入；实时会话内部音频段携带不可序列化的固定 key，`acceptAudioSegment` 使用 active turn 写入，`xinzhiliWSSink.SendAudio` 直接编码片段携带值，不再读取 `conn.turnKey`。

- [ ] **Step 4: 运行相关 Go 测试和 race**

```bash
go test ./internal/xinzhili ./internal/server -count=1
go test -race ./internal/xinzhili ./internal/server -count=1
```

Expected: PASS。

- [ ] **Step 5: 提交后台稳定性修复**

```bash
git add nx-backend/apps/server/internal/xinzhili \
  nx-backend/apps/server/internal/server/app_xinzhili_realtime.go \
  nx-backend/apps/server/internal/server/app_xinzhili_realtime_test.go
git commit -m '修复：固定芯之力音频片段所属轮次'
```

## Chunk 2: 模式恢复与端点提速

### Task 3: 四模式、均衡时间和配置热更新

**Files:**
- Modify: `nx-backend/apps/server/internal/xinzhili/config.go`
- Test: `nx-backend/apps/server/internal/xinzhili/config_test.go`
- Test: `nx-backend/apps/server/internal/xinzhili/config_store_test.go`
- Modify: `nx-backend/apps/server/internal/server/app_xinzhili_realtime.go`
- Modify: `nx-backend/apps/server/internal/server/xinzhili_model_config.go`
- Test: `nx-backend/apps/server/internal/server/xinzhili_model_config_test.go`
- Test: `nx-backend/apps/server/internal/server/app_xinzhili_realtime_test.go`
- Modify: `/Users/wohenzaiyi/Desktop/nine-xing-app/lib/features/xinzhili/services/xinzhili_realtime_session.dart`
- Test: `/Users/wohenzaiyi/Desktop/nine-xing-app/test/features/xinzhili/services/xinzhili_realtime_session_test.dart`
- Modify: `nx-backend/apps/web-antd/src/views/settings/xinzhili-model.vue`
- Test: `nx-backend/apps/web-antd/src/views/settings/xinzhili-model.test.ts`

- [ ] **Step 1: 写默认编辑表单和配置规范化失败测试**

期望新配置编辑表单包含四种模式，并使用 120/300/500/900/1200ms 均衡参数；已有配置仍按保存值加载。另写保存后活动连接收到 `session.config_changed`、漏广播连接在下一次 startTurn 补发新版本的失败测试。覆盖播放期间禁用当前模式：活动轮不中断，requested/pending 先回退 normal，effective 保持旧轮真实模式，下一轮再归一 normal。

- [ ] **Step 2: 运行 Go 与前端定向测试确认失败**

```bash
cd nx-backend/apps/server
go test ./internal/xinzhili -run 'Test.*Default.*Timing|Test.*Default.*Modes' -count=1
go test ./internal/server -run 'TestXinzhili.*ConfigChanged|TestXinzhili.*ConfigBroadcast|TestXinzhili.*StartTurn.*Config' -count=1
cd /Users/wohenzaiyi/Desktop/nine-xing/nx-backend
pnpm test:unit -- apps/web-antd/src/views/settings/xinzhili-model.test.ts
cd /Users/wohenzaiyi/Desktop/nine-xing-app
flutter test test/features/xinzhili/services/xinzhili_realtime_session_test.dart \
  --plain-name 'keeps active effective mode until the next turn after config change'
```

Expected: FAIL，仍为仅 normal 和旧时间参数。

- [ ] **Step 3: 更新新配置默认值和管理端空表单默认值**

仅调整“首次配置/空配置”的默认值；读取已保存配置时继续尊重数据库。PUT 持久化成功后广播新模式快照；startTurn 比较版本并兜底刷新。配置变更不取消活动轮，requested/pending 可先回退 normal，effective 在活动轮结束前保留旧模式；App 允许这一过渡状态但禁止重新选择已禁用模式。

- [ ] **Step 4: GET、合并并完整 PUT 本地权威配置**

先 GET `/api/xinzhili-model-config`，以返回完整 view 为基线，仅替换 `enabledModes` 和 5 个时间参数；保留 `enabled`、`realtimeAsr`、`tts`、`commonPrompt`、`modePrompts` 以及 timing 中的主动提示字段。提交当前 `expectedVersion` 的完整 PUT，apiKey 留空沿用已存密钥，最后再次 GET 验证。

目标差异：

```json
{
  "enabledModes": ["normal", "argument", "comfort", "deep_listening"],
  "timing": {
    "partialStableMs": 120,
    "argumentCandidateSilenceMs": 300,
    "normalEndSilenceMs": 500,
    "comfortEndSilenceMs": 900,
    "deepListeningEndSilenceMs": 1200
  }
}
```

不得把该 JSON 片段直接作为 PUT body。

- [ ] **Step 5: 验证 ready 和 config_changed 返回四模式**

新建真机 WebSocket 会话，确认 `session.ready.enabledModes` 包含四种模式；保持连接后再次保存配置，确认收到 `session.config_changed` 且 App 无需重启即可刷新模式选择器。

- [ ] **Step 6: 分别提交后台和 App 配置状态机改动**

```bash
cd /Users/wohenzaiyi/Desktop/nine-xing
git add nx-backend/apps/server/internal/xinzhili/config.go \
  nx-backend/apps/server/internal/xinzhili/config_test.go \
  nx-backend/apps/server/internal/xinzhili/config_store_test.go \
  nx-backend/apps/server/internal/server/app_xinzhili_realtime.go \
  nx-backend/apps/server/internal/server/app_xinzhili_realtime_test.go \
  nx-backend/apps/server/internal/server/xinzhili_model_config.go \
  nx-backend/apps/server/internal/server/xinzhili_model_config_test.go \
  nx-backend/apps/web-antd/src/views/settings/xinzhili-model.vue \
  nx-backend/apps/web-antd/src/views/settings/xinzhili-model.test.ts
git commit -m '功能：恢复芯之力四模式均衡参数'

cd /Users/wohenzaiyi/Desktop/nine-xing-app
git add lib/features/xinzhili/services/xinzhili_realtime_session.dart \
  test/features/xinzhili/services/xinzhili_realtime_session_test.dart
git commit -m '功能：兼容芯之力模式配置过渡状态'
```

## Chunk 3: 首句和 TTS 提速

### Task 4: 流式回答首块优先分句

**Files:**
- Modify: `nx-backend/apps/server/internal/xinzhili/session.go`
- Test: `nx-backend/apps/server/internal/xinzhili/session_test.go`

- [ ] **Step 1: 写首块软标点和无标点上限失败测试**

覆盖：少于 14 rune 的弱标点不切；达到 14 rune 后在 `，,；;：:` 切首块；无标点达到 28 rune 切首块；后续强句号优先且上限 42 rune；Flush 不丢文本。

- [ ] **Step 2: 运行分块测试确认当前只认强句号/42 字**

```bash
cd nx-backend/apps/server
go test ./internal/xinzhili -run 'TestStreamSentenceChunker.*First' -count=1
```

Expected: FAIL。

- [ ] **Step 3: 实现首块优先规则**

为 `streamSentenceChunker` 记录是否已发首块。首块采用 14 rune 最小弱标点长度和 28 rune 硬上限；后续维持强句号优先和 42 rune 上限。

- [ ] **Step 4: 跑全部 xinzhili 测试**

```bash
go test ./internal/xinzhili -count=1
```

Expected: PASS。

- [ ] **Step 5: 提交首句提速**

```bash
git add nx-backend/apps/server/internal/xinzhili/session.go \
  nx-backend/apps/server/internal/xinzhili/session_test.go
git commit -m '优化：提前生成芯之力首段语音'
```

### Task 5: 会话级 TTS 两路并行且按序发送

**Files:**
- Modify: `nx-backend/apps/server/internal/xinzhili/session.go`
- Test: `nx-backend/apps/server/internal/xinzhili/session_test.go`

- [ ] **Step 1: 写跨回答块并行失败测试**

让 seq0 合成较慢、seq1 较快，断言两个供应商调用重叠，但输出顺序仍为 0、1；首块完成后无需等待后续块。增加失败取消、迟到结果丢弃、pending 不超过 2、跨块累计超过 10MiB、workers 未退出前不发 done 的测试。

- [ ] **Step 2: 运行测试确认当前外层 worker 串行**

```bash
go test ./internal/xinzhili -run 'TestSessionTTS' -count=1
```

Expected: FAIL，最大并发为 1。

- [ ] **Step 3: 实现有界任务池和顺序提交器**

会话 worker 使用派生 TTS context，最多启动 2 个合成任务，并保持 `dispatched-nextEmit <= 2`；结果按 seq 暂存，只有 `nextSeq` 可发送。任一任务失败后取消更高 seq、停止派发并丢弃迟到结果，但继续排空 `ttsJobs` 直到生成端关闭，保证 `queueTTSChunk` 不会因满 channel 卡死事件循环。会话级累计所有音频字节并执行 10MiB 上限；全部 worker 退出后只发一次 `eventTTSDone`。

- [ ] **Step 4: 跑功能、取消和 race 测试**

```bash
go test ./internal/xinzhili -count=1
go test -race ./internal/xinzhili -count=1
```

Expected: PASS，无 goroutine 泄漏、乱序发送或 TTS 早期失败后的 channel 死锁；专门覆盖失败后仍产生超过原 channel 容量的 delta，最终仍收到 `assistant.done`。

- [ ] **Step 5: 提交 TTS 调度优化**

```bash
git add nx-backend/apps/server/internal/xinzhili/session.go \
  nx-backend/apps/server/internal/xinzhili/session_test.go
git commit -m '优化：并行合成芯之力短语音片段'
```

## Chunk 4: 集成验证与交付

### Task 6: 全量验证、真机测速和合并

**Files:**
- Verify: `/Users/wohenzaiyi/Desktop/nine-xing`
- Verify: `/Users/wohenzaiyi/Desktop/nine-xing-app`

- [ ] **Step 1: 后台全量定向验证**

```bash
cd /Users/wohenzaiyi/Desktop/nine-xing/nx-backend/apps/server
go test ./internal/xinzhili ./internal/server -count=1
go test -race ./internal/xinzhili ./internal/server -count=1
cd /Users/wohenzaiyi/Desktop/nine-xing/nx-backend
pnpm test:unit -- apps/web-antd/src/views/settings/xinzhili-model.test.ts
```

- [ ] **Step 2: App 验证**

```bash
cd /Users/wohenzaiyi/Desktop/nine-xing-app
flutter analyze
flutter test test/features/xinzhili
flutter build apk --debug \
  --dart-define=APP_ENV=local \
  --dart-define=API_BASE=http://127.0.0.1:5320/api/app
```

- [ ] **Step 3: 非 Docker 重启本地服务并安装 App**

恢复 `adb reverse tcp:5320 tcp:5320`，覆盖安装 Debug APK。

- [ ] **Step 4: 四模式真机连续验证**

每种模式至少连续 3 轮，记录：

```text
turn.start → turn.processing
turn.processing → first assistant.audio_start
assistant.audio_start → assistant.playback_ack
```

不得出现 `audio_timeout`、`generation_failed`、无 ACK 的 `sent` 助手消息。

- [ ] **Step 5: 清理临时协议诊断日志**

真机确认 fixed turnKey 后，移除当前工作区 `xinzhili_socket.dart` 中临时增加的 turn.start/binary 明细日志，保留原有事件级 Debug 日志；重新运行 Flutter analyze 和定向测试，确保 App 工作区只包含正式修复。

- [ ] **Step 6: 代码审查和最终提交**

使用 `superpowers:requesting-code-review`，修复审查问题后运行 `superpowers:verification-before-completion`。

- [ ] **Step 7: 合并并推送各自 main**

只更新后台仓库和 App 仓库的 `main`，不修改 `test` 分支。确认两个工作区干净，远端提交与本地 HEAD 一致。
