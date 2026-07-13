# 双协议聊天、真实流式与全局用户偏好设计

## 目标

统一后台和 App 的聊天模型链路，只支持 OpenAI-compatible 与 Anthropic-compatible 两种协议；移除聊天路径中的 MiniMax 专用配置和请求格式；保证 Flutter 在模型尚未生成完时即可显示首个增量；让用户对称呼、长度、语气、格式和互动方式的明确指示立即生效，并跨会话长期保存。

默认回答应简短、自然、拟人化：普通问题通常 1–3 句，复杂问题使用必要的短段落或短列表，只有用户明确要求详细说明时才展开。

## 范围与边界

本次修改涉及两个仓库：

- 后端与管理后台：`/Users/wohenzaiyi/Desktop/nine-xing`
- Flutter App：`/Users/wohenzaiyi/Desktop/nine-xing-app`

本次只替换“聊天模型”链路。已有管理任务、每日题目、Embedding 等模型能力不因为本设计被强制迁移。聊天配置中不再展示、保存或解释 MiniMax Group ID，也不再调用 `/v1/text/chatcompletion_v2`。

旧聊天配置如果没有 `provider` 字段，必须显示为未配置并要求管理员重新选择协议；不得静默回退到 MiniMax，也不得把旧字段猜测成 OpenAI 配置。

## 方案选择

采用后端双适配器与统一流事件方案：

- OpenAI 适配器使用 `/v1/chat/completions` 和 OpenAI Chat Completions 请求、响应及 SSE 事件。
- Anthropic 适配器使用 `/v1/messages` 和 Anthropic Messages 请求、响应及 SSE 事件。
- 两个适配器实现同一个聊天生成接口，但各自保留原生协议，不通过外部网关互相转换，也不把 Anthropic 请求强塞进 OpenAI 数据结构。
- 后端对 App 只输出稳定的 `delta`、`done`、`error` 三类 SSE 事件，使 App 不依赖上游供应商协议。

不采用“统一成 OpenAI 格式后再兼容 Anthropic”的方案，因为该方案会隐藏 Anthropic 的鉴权、消息结构、结束原因和事件语义差异，测试与故障定位也更困难。

## 后台配置

聊天配置模型改为：

```text
provider: openai-compatible | anthropic-compatible
apiBase: string
apiKey: string
model: string
timeoutSeconds: integer
systemPrompt: optional string
```

管理后台聊天配置增加协议选择框，选项仅为 OpenAI 协议和 Anthropic 协议。移除 Group ID 输入框和 MiniMax 文案。`apiBase` 保持可配置，以支持官方端点和兼容网关；保存时去掉末尾斜杠，适配器负责可靠拼接路径，避免重复 `/v1` 或双斜杠。

保存配置后，服务端通过工厂方法构造对应聊天生成器并原子替换运行时实例。配置校验与“测试连接”必须走同一个工厂，避免测试成功而运行时使用另一套协议。未知 provider、空 provider、缺少 API key/model 或不合法超时均返回明确错误，旧运行实例在新配置验证失败时保持不变。

## Provider 适配器

### 公共接口

聊天生成器继续支持同步和流式入口：

```go
Generate(ctx, input) (string, error)
GenerateStream(ctx, input, emit) (string, error)
Ping(ctx) PingResult
```

公共层负责输入上下文、最终文本累积和错误分类；适配器只负责请求构造、鉴权、上游响应解析和原生错误提取。

### OpenAI-compatible

- 请求路径：在 `apiBase` 上拼接 `/chat/completions`；如果配置已经以 `/v1` 结尾，不重复添加版本段。
- 鉴权：`Authorization: Bearer <apiKey>`。
- 请求使用 `model`、`messages`、`stream` 和受控的输出 token 上限。
- 流式解析 `data:` 事件中的 `choices[].delta.content`，忽略 usage-only 事件和空增量，以 `[DONE]` 或明确结束原因完成。
- 非 2xx、错误对象、畸形事件、超大事件和上下文取消都返回结构化错误，不把错误正文作为回答。

### Anthropic-compatible

- 请求路径：在 `apiBase` 上拼接 `/messages`。
- 鉴权：`x-api-key: <apiKey>`，并发送所需 `anthropic-version`。
- system 内容使用顶层 `system`，历史与当前问题使用 Anthropic `messages`。
- 流式解析 `content_block_delta` 中的 `text_delta.text`；正确处理 `message_start`、`content_block_start/stop`、`message_delta`、`message_stop` 和 `error`。
- 不把 Anthropic event source 映射为 OpenAI choices 后再解析。

### 统一输出

适配器每解析到有效文本就立即调用 `emit(delta)`，同时累积完整回答。App HTTP handler 将其编码为：

```text
event: delta
data: {"content":"..."}

event: done
data: {"messageId":"...", ...}

event: error
data: {"message":"..."}
```

若上游在产生部分文本后失败，不保存这轮问答；App 临时保留已显示文本并标记失败。只有上游完整结束且 `SavePair` 成功后才发送 `done`。

## SSE 端到端交付

Go handler 在写响应头后立即 `Flush()`，并在等待上游首段或较长间隔时发送 SSE 注释心跳，例如 `: ping\n\n`。心跳不改变业务事件，但可尽早建立响应并降低代理、移动网络或 Dio 缓冲整段响应的概率。Nginx 继续对 `/api/app/chat/` 关闭 buffering、cache 和 gzip。

Flutter 的 `postStream` 对 SSE 请求禁用普通响应体 `receiveTimeout`，直接消费字节流并增量 UTF-8 解码。Repository 解析器保留跨网络 chunk、CRLF、单 chunk 多事件和注释行支持。Notifier 在每个 `delta` 到达时更新同一个 pending 助手气泡并立即 `notifyListeners()`；`done` 只补齐服务端消息 ID、来源和完成状态，不重复追加文本。

核心验收不是“最终内容正确”，而是本地真实延迟 HTTP 服务先发送第一段、阻塞第二段时，Repository 和 Notifier 已经向 UI 暴露第一段。

## 全局用户偏好

### 数据模型

新增用户级偏好存储，与会话、卡片和具体模型无关。每条有效偏好至少包含：

```text
userId
category: addressing | length | tone | format | interaction | custom
instruction
normalizedKey
sourceText
createdAt
updatedAt
```

同一用户、同一 `category + normalizedKey` 唯一。明显冲突的新偏好替换旧偏好；取消指令删除对应偏好。偏好需要纳入用户隐私导出和删除流程。

### 即时服从与异步记忆

当前消息中的明确指令直接进入本轮 prompt，因此首次说“不要叫我亲爱的”“以后回答短一点”时，本轮回答就必须遵守，不能等待异步提取完成。

回答完成后异步提取可长期复用的交流偏好。常见明确表达使用本地确定性规则兜底，包括：

- 不要/请叫我某个称呼。
- 回答短一点、详细一点、不要长篇大论。
- 语气正式、随意、温柔、直接、少说教。
- 不要列表、使用列表、先给结论。
- 不要反问、少追问、一次只问一个问题。
- 忘掉、取消或恢复某项偏好。

模型提取失败不能影响主回答；确定性规则识别到的常见偏好仍应保存。对事实陈述、一次性任务格式和含糊表达不得擅自提升为长期偏好。

### 优先级

Prompt 按以下顺序组装，越靠后越具体：

1. 安全、真实性和产品硬边界。
2. 默认人格与简短回答规则。
3. 已保存的全局用户偏好。
4. 会话摘要和近期历史。
5. 当前用户消息及其中的明确指令。

当前明确指令优先于默认风格和已保存偏好，但不能覆盖安全与真实性边界。例如用户曾要求“回答详细”，本轮说“只告诉我结果”时，本轮只给结果；若本轮表达“以后都简短”，则同时更新全局偏好。

### 默认回复契约

- 普通确认、简单问题和情绪回应：通常 1–3 句。
- 复杂分析：使用少量短段落或短列表，只保留解决问题需要的信息。
- 只有用户明确要求“详细解释、展开、完整分析”等才长答。
- 避免固定开场、机械复述、课程式总结、无必要建议和连续追问。
- 语气像自然朋友，但不默认使用“亲爱的”等亲昵称呼。

## 错误处理与可观测性

- 配置错误在保存或测试连接时返回具体字段错误。
- 上游非 2xx 日志记录 provider、model、状态码和脱敏错误摘要，不记录 API key 或完整私人对话。
- SSE 建连、首段到达、完成、取消和错误记录耗时指标，便于区分上游慢、服务端缓冲、代理缓冲和 App 渲染问题。
- 心跳写失败或客户端取消时立即取消上游请求。
- 偏好提取失败只记录内部错误，不阻断聊天；存储失败不伪装为已记住。

## 测试设计

### 后端单元与集成测试

- 配置序列化、脱敏返回、旧配置无 provider 时未配置、未知 provider 拒绝。
- 工厂分别构造 OpenAI 与 Anthropic 适配器，完全拒绝 MiniMax provider。
- 两种协议的请求路径、鉴权、system/history 映射、同步结果与原生错误解析。
- 两种协议真实延迟 SSE：第一段必须在第二段释放前到达 emitter。
- LF/CRLF、跨 read 拆分、多事件、空 delta、usage、结束、畸形 JSON、超大事件和取消。
- App chat handler 的 header、初始 flush、心跳、delta/done/error、完整后保存与部分失败不保存。
- 偏好新增、替换、取消、跨会话读取、用户隔离、隐私导出与删除。
- 当前指令覆盖旧偏好，默认 1–3 句契约进入 prompt，常见本地规则在提取器失败时仍保存。
- 管理后台类型检查、单测和生产构建。

### Flutter 测试

- Repository 解析真实 HTTP 分段字节流，而不仅是 mock 字符串 Stream。
- 本地延迟服务发送第一段后阻塞；断言第二段未发送时 Repository 已产出 delta。
- Notifier 收到第一段后，pending 气泡内容立即可见且仍处于生成状态。
- `done` 不重复文本；部分失败保留文本但无可持久化 messageId。
- SSE 请求不受普通 `receiveTimeout` 中断，取消订阅能终止请求。
- 全量 `flutter test`、`flutter analyze` 和 Android 构建验证。

### 端到端验收

分别用 OpenAI-compatible 和 Anthropic-compatible 的受控测试服务完成一轮聊天，确认后台测试连接与实际聊天使用同一配置。真实流探针必须证明首个 `delta` 在上游结束前到达 App 层。使用新会话验证“不要叫我亲爱的”“回答短一点”“以后先给结论”等偏好跨会话生效，并验证取消偏好后恢复默认。

## 发布与迁移

先发布后端和管理后台，再发布 App。后端上线后，旧聊天配置会显示为未配置；管理员必须选择协议并通过测试连接后才能启用新运行实例。部署前保留原配置数据但不再读取 Group ID，便于审计和人工迁移，不提供静默兼容路径。

若 App 版本仍使用既有 `delta/done/error` 协议，可在后端切换后继续工作；新版 App 主要补足字节级实时性、超时和真实延迟测试。发布后执行 OpenAI、Anthropic、偏好跨会话和真机首段时序四类冒烟测试。
