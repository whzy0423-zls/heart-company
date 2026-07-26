# 免费语音模型接入设计

## 目标

让后台能够快速配置并验证硅基流动免费额度语音模型，使普通聊天语音继续使用 SenseVoiceSmall，芯之力同时具备 ASR 与 TTS 能力。

## 模型配置

- ASR：`FunAudioLLM/SenseVoiceSmall`
- TTS：`FunAudioLLM/CosyVoice2-0.5B`
- TTS 默认音色：`FunAudioLLM/CosyVoice2-0.5B:alex`
- API Base：`https://api.siliconflow.cn/v1`
- Provider：`openai-compatible`
- ASR 语言：`zh`
- TTS 格式：`mp3`

API Key 不写入代码或版本库。后台预设只填充非敏感字段，管理员输入硅基流动 Key 后保存。

## 后台交互

芯之力模型配置区增加“使用硅基流动免费预设”操作，一次填入 ASR、TTS、音色、超时等字段。已有 API Key 保持不变；新环境需填写 Key。普通聊天 ASR 的部署环境继续使用相同的 SiliconFlow Key 和 SenseVoiceSmall。

## 验证

- 前端契约测试确认预设值完整且不会填入密钥。
- 后端模型配置测试确认 ASR/TTS 配置可保存、读取和保留密钥。
- 运行后台前端测试、Go 相关测试及全量测试。
