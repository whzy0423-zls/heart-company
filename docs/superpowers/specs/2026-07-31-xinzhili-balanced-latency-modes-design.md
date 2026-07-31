# 芯之力均衡提速与四模式恢复设计

## 目标

- 恢复 App 端正常、抬杠、安慰、深度倾听四种模式。
- 在不明显抢话的前提下降低语音结束到首段可播放音频的等待时间。
- 保持现有 WebSocket、Paraformer、知识库、流式回答、百炼克隆音色链路兼容。
- 多轮会话连续稳定播放，每段音频均完成 `assistant.playback_ack`。

## 当前原因

- 数据库 `xinzhili_model_config.enabledModes` 目前只有 `normal`，App 按服务端权威配置隐藏其余模式，功能代码并未删除。
- 普通模式端点静音阈值为 700ms，安慰模式为 1200ms，深度倾听为 1500ms。
- 流式回答只有遇到强句号或累计到 42 字才送入 TTS，首句没有明显标点时会延迟首段合成。
- TTS 提供器已有最多 2 路并发能力，但实时会话外层按回答块串行调用，跨块并发没有被充分利用。

## 方案

### 1. 恢复四模式

保存以下权威模式列表：

```json
["normal", "argument", "comfort", "deep_listening"]
```

App 继续使用 `session.ready` 返回的 `enabledModes` 展示模式选择器，不增加客户端硬编码兜底，避免后台禁用模式后 App 仍可误选。

### 2. 均衡端点参数

- `partialStableMs`: 150 → 120
- `argumentCandidateSilenceMs`: 350 → 300
- `normalEndSilenceMs`: 700 → 500
- `comfortEndSilenceMs`: 1200 → 900
- `deepListeningEndSilenceMs`: 1500 → 1200

主动安慰提示时间保持不变，避免为了提速导致系统过早主动说话。

### 3. 首句提前进入 TTS

流式分块采用“自然标点优先、长度上限兜底”：

- 强句号仍立即切块。
- 首块累计至少 14 个 rune 后，允许在 `，,；;：:` 处分块。
- 首块没有标点时到 28 个 rune 强制切块；后续块继续使用强句号优先、42 个 rune 上限。
- 少于 14 个 rune 的弱标点不切块，避免产生过碎、缺少语义的播报。
- 不在字节层切 MP3，每一块仍由 TTS 生成独立完整 MP3。

### 4. TTS 小段有界并行

- 会话级 TTS 调度最多并行 2 个短文本任务。
- 结果允许乱序完成，但必须按 `segmentSeq` 顺序发送。
- 第一块完成后立即发送，不等待整轮回答或后续块。
- 取消、打断、超时沿用当前 context 传播，禁止迟到音频进入下一轮。
- 调度窗口固定为 2，未按序提交的结果最多缓存 2 个；不能因首块慢而无限合成后续块。
- 任一 seq 失败时取消全部更高 seq，丢弃其迟到结果；所有 worker 退出后才发布 `eventTTSDone`，随后才允许 `assistant.done`。
- 10MiB 整轮音频限制由会话调度器跨所有短块累计，不能在每次供应商调用时重新计数。

### 5. 配置热更新

- 后台 PUT 保存成功后向当前进程内的芯之力连接广播 `session.config_changed`，载荷包含新 `configVersion`、`enabledModes` 和模式快照。
- `startTurn` 再次读取权威配置并比较版本，作为多实例或漏广播场景的兜底；版本变化时先更新连接状态并发送 `config_changed`，再开始新轮。
- 配置保存不打断正在录音、生成或播放的活动轮；活动轮继续使用启动时捕获的旧 Mode/Timing/TTS 快照。
- 新配置禁用当前模式时，广播立即把 requested/pending 回退为 `normal`，但 effective 在活动轮结束前保持该轮真实模式；下一轮 startTurn 成功后 effective 再切换为 `normal`。
- App 允许配置切换过渡期的 effectiveMode 暂时不在新 enabledModes 中，只禁止用户再次选择已禁用模式；收到下一轮 processing/模式快照后完成归一。
- 保存继续使用现有 PUT、`expectedVersion` 和“空 apiKey 不修改密钥”语义；当前实例数据必须实际持久化四模式和均衡时间参数。
- Go/Vue 默认值只影响首次配置，不覆盖已有数据库行。

## 稳定性约束

- App 音频段和播放队列增加 `turnKey`；队列以 `turnKey` 而非 `segmentSeq=0` 启发式识别新轮。切换轮次时递增播放 epoch、停止旧播放器、完成旧 future 为 interrupted，并在切换屏障完成后才播放新轮片段。
- RealtimeSession 在 `startTurn` 时主动启动播放切换屏障，而不是等新片段入队后才发现换轮；旧 `_drainPlayback` 因 stop 返回 interrupted 后退出，新轮片段可先进入 completed，但必须等待屏障完成再 enqueue，避免 stop 的迟到完成清掉新轮。
- ACK 始终使用该音频段所属的原始 `turnId/turnKey`；迟到旧轮回调只结束旧 future，不能向当前轮发送 ACK。
- 服务端音频帧必须绑定所属轮次的固定 `turnKey`，不依赖可能变化的连接全局值。
- 不通过单纯延长“语音片段接收超时”掩盖丢帧或状态错位。
- 后台保存配置后，新建轮次立即使用最新模式和时间参数；已建立连接通过配置版本机制更新。

## 验证标准

- 后台保存后 App 可见并可切换四种模式。
- 已连接 App 在保存后收到 `session.config_changed`，无需重启即可看到四模式。
- 配置在 ASR、生成或播放期间保存时不打断活动轮，requested/pending 与 effective 的过渡状态符合上述快照规则；至少有一条状态机测试覆盖播放期间禁用当前模式。
- 首块分句测试覆盖 14 rune 弱标点、28 rune 无标点上限和后续 42 rune 上限。
- 每种模式至少完成 3 轮连续真机对话。
- 每个 `assistant.audio_start` 均对应二进制帧、`assistant.audio_end` 和 `assistant.playback_ack`。
- 普通模式语音结束到 `turn.processing` 的端点等待目标约 500ms。
- 短回答从 `turn.processing` 到首个 `assistant.audio_start` 相比当前链路明显下降，并记录实测数据。
- Go、Dart 单元测试、竞态测试、Flutter analyze 和 Debug APK 构建通过。

## 回滚

- 模式列表可恢复为仅 `normal`。
- 时间参数可恢复为 150/350/700/1200/1500ms。
- 首句分块和并行调度均由独立提交实现，可单独回退且不影响 ASR、模型配置和音色配置。
