# 会话模型中转站协议设计

## 目标

将手机端聊天使用的会话模型从 MiniMax 专用协议改为通用中转站模式，仅支持 OpenAI 兼容协议和 Anthropic 兼容协议。

## 配置

- 会话模型新增 `provider`：`openai-compatible` 或 `anthropic-compatible`。
- 保留 API Base、Model、API Key。
- 删除会话模型 Group ID。
- 旧配置缺少 provider 时默认按 OpenAI 兼容协议读取。
- API Base 默认展示 `https://coding-play.codes`，保存时去除末尾 `/`。

## 请求协议

- OpenAI：`POST {API Base}/v1/chat/completions`，Bearer Token，支持普通和 SSE 流式返回。
- Anthropic：`POST {API Base}/v1/messages`，`x-api-key` 与 `anthropic-version: 2023-06-01`，支持普通和 SSE 流式返回。
- 两种协议统一接入现有 `rag.Generator`、提示词、tier token 预算、会话摘要与流式输出。

## 管理后台

- 会话模型区域增加协议选择器。
- 删除 Group ID 输入。
- 根据协议显示接口路径说明。
- 连通性测试按当前协议调用，并返回“OpenAI 兼容模型”或“Anthropic 兼容模型”错误，不再出现 MiniMax 专用错误。

## 验证

- OpenAI 普通、流式、探活请求路径与响应解析正确。
- Anthropic 普通、流式、探活请求头、请求体与响应解析正确。
- 配置保存与旧配置兼容。
- 后台表单切换协议不会丢失 API Base、Model、API Key。
- 手机聊天通过中转站正常流式回答。
