# 免费语音模型接入设计

## 目标

让后台能够快速配置并验证免费额度语音模型，使普通聊天语音与芯之力实时语音分别使用适配各自协议的模型，并保持后台可独立配置。

## 两条语音链路

- 普通聊天/批量语音：HTTP ASR 使用硅基流动 `FunAudioLLM/SenseVoiceSmall`，通过 OpenAI 兼容的 `/audio/transcriptions` 接口。
- 芯之力实时语音：实时 ASR 继续使用阿里云百炼 Paraformer WebSocket（`wss://dashscope.aliyuncs.com/api-ws/v1/inference`、`paraformer-realtime-v2`）。实时 WebSocket 协议与批量 HTTP ASR 不兼容，不能把 SenseVoiceSmall 填入芯之力 ASR 字段。
- 芯之力语音回复：TTS 使用硅基流动 `FunAudioLLM/CosyVoice2-0.5B`，通过 OpenAI 兼容的 `/audio/speech` 接口。

## 模型配置

- 普通聊天 ASR：`FunAudioLLM/SenseVoiceSmall`
- 芯之力实时 ASR：阿里云百炼 `paraformer-realtime-v2`（WebSocket）
- 芯之力 TTS：`FunAudioLLM/CosyVoice2-0.5B`
- TTS 默认音色：`FunAudioLLM/CosyVoice2-0.5B:alex`
- API Base：`https://api.siliconflow.cn/v1`
- Provider：`openai-compatible`
- 批量 ASR 语言：`zh`
- TTS 格式：`mp3`

API Key 不写入代码或版本库。后台预设只填充非敏感字段，管理员输入硅基流动 Key 后保存。

## 后台交互

芯之力模型配置区增加“填充免费额度 TTS 预设”操作，只填入 TTS 协议、地址、模型、音色和格式；实时 ASR 的阿里云百炼配置不被改动，已有 ASR/TTS API Key 也不被覆盖。普通聊天模型配置区另提供批量 SenseVoiceSmall 预设。新环境需由管理员分别填写对应服务商的 Key。

## 验证

- 前端契约测试确认真实 `/settings/xinzhili-model` 页面中的 TTS 预设值完整、实时 ASR 仍是 Paraformer WebSocket 且不会填入密钥。
- 后端模型配置测试确认 ASR/TTS 配置可保存、读取和保留密钥。
- 运行后台前端测试、Go 相关测试及全量测试。
