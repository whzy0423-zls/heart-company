# 芯之力实时语音对话设计

## 目标

用户点击粒子球后直接讲话；App 在检测到有效语音后的短暂停顿时自动提交本轮。服务端依次完成 ASR、知识库与九型理论资料检索、对话模型生成和分句 TTS，并通过同一个 SSE 响应尽早把文字增量与可播放音频段返回。AI 播放期间再次点击球体会立即停止播放、取消服务端请求并重新进入聆听。

## 首版协议

使用 `POST multipart/form-data /api/app/xinzhili/turns/stream`，字段包含 `audio`、`durationMs`，返回 `text/event-stream`。首版不使用 WebSocket：本地已经具备静音断句，ASR 仍是批量文件接口，HTTP + SSE 可以复用现有鉴权、刷新、代理配置和取消链路。

事件顺序如下：

1. `ready`：服务端已经接受请求。
2. `state`：`transcribing`、`retrieving_knowledge`、`retrieving_theory`、`thinking`、`speaking`。
3. `transcript`：ASR 最终文本。
4. `text_delta`：对话模型的原始文本增量。
5. `audio`：`index`、`contentType`、`audioBase64`、`text`。
6. `done`：最终文字、来源和持久化消息标识。
7. `error`：可直接展示给用户的错误消息和稳定错误码。

连接期间每 12 秒发送 SSE 注释心跳。客户端断开时，服务端取消 LLM 和尚未开始的 TTS，不保存伪造的完整回答。

## 低延迟策略

- App 以 16kHz、单声道 Float32 PCM 采集，同时驱动粒子动画和本地 VAD。
- 检测到至少 300ms 有效声音后，连续约 700ms 低于退出阈值即提交；最长单轮 60 秒。
- App 只把当前轮编码为 PCM16 WAV，不先转 AAC，减少端侧编码延迟和格式不确定性。
- LLM 文字边生成边发给 App；服务端按 `。！？；\n` 或安全长度阈值切句。
- TTS 严格按句子序号串行生成，第一句生成完成即发送并播放，不等待全文。
- App 使用单播放队列；播放一段时可同时接收下一段。
- 记录 ASR、检索、首个 LLM delta、首段 TTS 和首段发送的耗时，便于线上定位慢点。

## 模型配置

在现有模型配置中增加 `xinzhiliVoice`：

- `enabled`
- ASR：OpenAI-compatible `apiBase/apiKey/model/language/timeoutSeconds`
- TTS：OpenAI-compatible `apiBase/apiKey/model/voice/speed/responseFormat/timeoutSeconds`
- 交互：`endSilenceMs/minSpeechMs/maxTurnSeconds/autoRelisten/tapToInterrupt`
- 专属系统提示词

对话模型继续复用现有 `chat` 的 OpenAI-compatible 或 Anthropic-compatible 配置。Anthropic 只用于中间的文本生成，不用于 ASR/TTS。密钥只以 `apiKeySet` 回显，空密钥保存表示保留旧值。任一必需配置缺失时，App 获得“请先在后台配置好芯之力语音模型后再重试”。

## 检索和上下文

服务端明确发出知识库、理论库两个状态。实现继续复用 `retrieveAppDocsForQuery` 的统一候选集，但在内部按 `kb-` 文档和站点/理论文档分组后再合并交给 RAG，保证事件语义与实际顺序一致。生成时复用主卡画像、用户记忆、沟通偏好与当前指令；不读取副卡会话上下文。

## 会话与隐私

- 原始芯之力录音默认不持久化。
- 只有完整生成成功后才保存 ASR 文本和最终 AI 文本。
- 使用 `xinzhili_voice` 场景的独立会话，不混入主卡文字聊天或任一副卡聊天。
- 被用户打断或网络中断的回答不作为完整回答保存。

## App 状态机

`idle → connecting → listening → processing → aiSpeaking → listening`。

- `listening`：采集、VAD 和球体输入动画。
- `processing`：停止麦克风，显示正在理解/检索。
- `aiSpeaking`：播放音频队列，并用输出音量驱动球体。
- `aiSpeaking` 点击：取消请求、清空队列、重新 `listening`。
- 无有效声音：继续聆听，不上传空白音频。
- 后台、退出页面、权限失败或鉴权失效：释放麦克风、网络和播放器资源。

## 错误语义

- 未配置：明确提示后台配置模型。
- 没听清：语音提示/界面提示用户靠近手机重新说，不生成心理分析。
- 网络异常：保留当前页面并允许点击球体重试。
- TTS 某一后续分段失败：已到达的语音可以播完，同时展示文字全文和失败提示。

## 验收指标

- 静音结束到 transcript 可观测。
- transcript 到首个文本 delta 可观测。
- 首个文本 delta 到第一段可播放音频可观测。
- 在正常中转站延迟下，目标是静音结束后约 2–4 秒开始听到首句；具体以真实 ASR/LLM/TTS 配置压测为准。
- 点击打断的本地停止反馈目标小于 150ms。
