# App 普通会话 ASR 配置兼容设计

## 背景

App 普通会话语音接口 `/api/app/chat/sessions/{id}/voice` 当前只读取服务启动时的 `ASR_*` 环境变量。后台“模型配置”页面已经存在 `xinzhiliVoice.asr` 的 OpenAI-compatible ASR 配置，但保存后不会被普通会话读取，因此用户看到“已配置”与运行时仍提示 ASR 未配置的不一致。

独立的“芯之力模型配置”页面保存的是阿里百炼 Paraformer WebSocket 实时 ASR。该协议不能直接当作普通会话使用的 `/audio/transcriptions` HTTP 接口，必须继续保持独立，避免破坏现有实时语音。

## 方案

普通会话 ASR 在每次识别前解析当前配置：

1. 优先读取 `site_configs.key = model_config` 中完整的 `xinzhiliVoice.asr`。
2. 只有当 provider 为 OpenAI-compatible，且 `apiBase`、`apiKey`、`model` 均完整时才采用后台配置。
3. 后台配置缺失、不完整或数据库暂时读取失败时，回退现有 `ASR_API_BASE`、`ASR_API_KEY`、`ASR_MODEL`、`ASR_TIMEOUT_SECONDS` 环境变量。
4. 不读取、改写或替换 `xinzhili_model_config.realtimeAsr`，现有 WebSocket 实时语音链路保持原样。

## 数据流

```text
App 普通会话录音
  -> POST /api/app/chat/sessions/{id}/voice
  -> recognizeSpeech
  -> 读取 model_config.xinzhiliVoice.asr
       -> 完整：使用后台配置
       -> 缺失/异常：使用 ASR_* 环境变量
  -> OpenAI-compatible /audio/transcriptions
```

## 错误处理

- 两个来源都没有完整密钥时，继续返回 ASR 未配置错误。
- 数据库读取失败时不让已有环境变量能力下线。
- 上游 HTTP、响应解析、超时与地址防护继续沿用现有实现。
- 日志和接口响应不回显 API Key。

## 验收

- 后台保存完整的 `xinzhiliVoice.asr` 后，普通会话无需重启即可使用新配置。
- 后台 ASR 优先于环境变量。
- 后台 ASR 不完整时，原有环境变量仍可使用。
- 芯之力 Paraformer WebSocket 实时语音相关配置和测试不受影响。

