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
```

现有 `assist.enabled` 与 `assist.systemPrompt` 结构保留，避免丢失已配置人设或改变关闭 AI 辅助的语义。默认安全、简短和指令优先规则始终存在，后台 `systemPrompt` 只作为补充人设，不能完全覆盖这些硬规则。

管理后台聊天配置增加协议选择框，选项仅为 OpenAI 协议和 Anthropic 协议。移除 Group ID 输入框和 MiniMax 文案。`apiBase` 定义为“已经包含版本段和可选自定义前缀的 API 根”，官方示例分别为 `https://api.openai.com/v1` 与 `https://api.anthropic.com/v1`。保存时去掉末尾斜杠，适配器只追加资源路径 `/chat/completions` 或 `/messages`，不自行添加第二个 `/v1`。

保存顺序固定为：合并输入并校验 → 构造候选生成器并执行最小探测 → 持久化配置 → 在锁内进行不会失败的实例交换。任何前置步骤失败时，数据库与运行时实例均保持旧值。provider 发生变化时必须重新提交 API key，不得继承上一供应商的密钥。配置校验与“测试连接”必须走同一个工厂。未知 provider、空 provider、缺少 API key/model 或不合法超时均返回明确错误。

## Provider 适配器

### 公共接口

聊天生成器继续支持同步、流式、长会话摘要和提示词润色入口：

```go
Generate(ctx, input) (string, error)
GenerateStream(ctx, input, emit) (string, error)
Ping(ctx) PingResult
SummarizeConversation(ctx, input) (string, error)
PolishPrompt(ctx, prompt) (string, error)
```

公共层负责输入上下文、最终文本累积和错误分类；适配器只负责请求构造、鉴权、上游响应解析和原生错误提取。现有 `rag.ConversationSummarizer` 和提示词润色调用必须改为 provider-neutral 能力，不能继续断言 `*MiniMaxGenerator`。

### OpenAI-compatible

- 请求路径：在已包含版本段的 `apiBase` 上拼接 `/chat/completions`。
- 鉴权：`Authorization: Bearer <apiKey>`。
- 请求使用 `model`、`messages`、`stream` 和受控的输出 token 上限。
- 流式解析 `data:` 事件中的 `choices[].delta.content`，忽略 usage-only 事件和空增量，以 `[DONE]` 或明确结束原因完成。
- 同步响应只读取 `choices[].message.content`；非 2xx、错误对象、畸形事件、超大事件和上下文取消都返回结构化错误，不把错误正文作为回答。

### Anthropic-compatible

- 请求路径：在已包含版本段的 `apiBase` 上拼接 `/messages`。
- 鉴权：`x-api-key: <apiKey>`，并发送所需 `anthropic-version`。
- system 内容使用顶层 `system`，历史与当前问题使用 Anthropic `messages`，请求必须包含受控的 `max_tokens`。
- 流式解析 `content_block_delta` 中的 `text_delta.text`；正确处理 `message_start`、`content_block_start/stop`、`message_delta`、`message_stop` 和 `error`。
- 同步响应只拼接 `type=text` 的 content blocks；不把 Anthropic event source 映射为 OpenAI choices 后再解析。

### 网络与超时安全

动态 `apiBase` 必须继续使用现有 `netguard` 公网 URL 校验和受保护 transport，拒绝 localhost、私网地址、危险重定向和 DNS rebinding。测试通过注入 HTTP client/transport 使用本地 `httptest.Server`，不得为测试放宽生产校验。

超时分为三层：provider 请求总上限、聊天 handler 总业务上限、SSE 空闲上限。Flutter 不使用普通 response receive timeout，但后端仍保留有限的总时长和空闲时长；配置优先级必须明确，不能让旧的 `MINIAPP_CHAT_TIMEOUT_SECONDS` 静默覆盖后台模型超时。

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

Go handler 完成鉴权、请求体与 session 归属校验后，立即写 `: connected\n\n` 并 `Flush()`；资料检索、用户画像、偏好加载和可能触发模型调用的会话摘要都在流建立后执行。生成和慢操作在派生 context 中运行，通过 channel 把 delta/result/error 交给唯一 writer pump；heartbeat、delta、done、error 只由 handler writer goroutine 串行写入，禁止多个 goroutine 并发操作 `ResponseWriter`。写失败立即取消派生 context。每条连接只能发送一次终态：`done` 或 `error`，终态后立即关闭。

在等待上游首段或较长间隔时发送 SSE 注释心跳，例如 `: ping\n\n`。心跳不改变业务事件，但可降低代理、移动网络或 Dio 缓冲整段响应的概率。Nginx 继续对 `/api/app/chat/` 关闭 buffering、cache 和 gzip。

Flutter 的 `ApiClient` 增加可注入 `baseUrl`/Dio 的测试接缝。`postStream` 对 SSE 请求禁用普通响应体 `receiveTimeout`，使用显式 `CancelToken`，直接消费字节流并增量 UTF-8 解码。Repository 解析器保留跨网络 chunk、CRLF、单 chunk 多事件和注释行支持。Notifier 在每个 `delta` 到达时更新同一个 pending 助手气泡并立即 `notifyListeners()`；`done` 只补齐服务端消息 ID、来源和完成状态，不重复追加文本。页面销毁或切换会话时要真正取消在途 HTTP，而不只是忽略迟到结果；消息内容增长时保持合理的自动滚动体验。

核心验收不是“最终内容正确”，而是本地真实延迟 HTTP 服务先发送第一段、阻塞第二段时，Repository 和 Notifier 已经向 UI 暴露第一段。

## 全局用户偏好

### 数据模型

新增用户级偏好存储，与会话、卡片和具体模型无关。每条有效偏好至少包含：

```text
userId
category: addressing | length | tone | format | interaction | custom
slot
instruction
sourceText
createdAt
updatedAt
```

同一用户、同一 `slot` 唯一，例如 `length.detail_level`、`addressing.preferred_name`、`format.conclusion_first`。明显冲突的新偏好通过同一 slot 替换旧偏好；可共存偏好使用不同 slot。取消指令删除对应 slot。偏好需要纳入用户隐私导出和删除流程。

### 即时服从与异步记忆

当前消息中的明确指令直接进入本轮 prompt，因此首次说“不要叫我亲爱的”“以后回答短一点”时，本轮回答就必须遵守，不能等待异步提取完成。

常见明确表达由本地确定性规则在生成前识别为本轮 overlay，并同步持久化，至少在发送成功 `done` 前完成；因此 AI 不能在数据库写入失败时伪装“已经记住”。即使本轮模型生成失败，用户明确表达的长期新增、取消或忘记指令仍然保留，因为它本身就是独立的用户设置操作。

模型只异步补充确定性规则无法识别、但明显属于交流风格的偏好；使用独立 bounded context 和小并发槽，不依赖请求 context。模型提取失败不影响主回答，也不得承诺已记住。常见明确表达包括：

- 不要/请叫我某个称呼。
- 回答短一点、详细一点、不要长篇大论。
- 语气正式、随意、温柔、直接、少说教。
- 不要列表、使用列表、先给结论。
- 不要反问、少追问、一次只问一个问题。
- 忘掉、取消或恢复某项偏好。

对事实陈述、一次性任务格式和含糊表达不得擅自提升为长期偏好。`custom` 也只允许交流风格，不允许保存安全绕过、事实主张或任意任务指令。每用户偏好条数、单条长度和注入 prompt 的总字符/token 必须有上限，偏好以明确的数据区块注入，不能被解释成更高优先级系统命令。

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
- App chat handler 的 header、阻塞检索/摘要前初始 flush、单 writer 心跳、帧不交错、唯一终态、delta/done/error、完整后保存与部分失败不保存，并运行相关 race 测试。
- 偏好按 slot 新增、替换、取消、容量限制、跨会话读取、用户隔离、隐私导出与删除。
- 当前指令覆盖旧偏好，默认 1–3 句契约进入 prompt，常见本地规则在提取器失败时仍保存。
- 长会话摘要与提示词润色分别在两种 provider 下可用。
- 无 provider、旧配置、仅设置 MiniMax 环境变量三种启动场景均保持聊天未配置。
- 动态 API Base 的 SSRF、重定向和受保护 transport 回归测试。
- 管理后台类型检查、单测和生产构建。

### Flutter 测试

- Repository 解析真实 HTTP 分段字节流，而不仅是 mock 字符串 Stream。
- 本地延迟服务发送第一段后阻塞；断言第二段未发送时 Repository 已产出 delta。
- Notifier 收到第一段后，pending 气泡内容立即可见且仍处于生成状态。
- `done` 不重复文本；部分失败保留文本但无可持久化 messageId。
- SSE 请求不受普通 `receiveTimeout` 中断，取消订阅、切换会话和页面销毁能终止请求。
- SSE buffer 有尺寸上限，损坏 JSON/UTF-8 返回明确协议错误。
- 内容增长时仅在用户原本接近底部时自动跟随，不能抢走历史阅读位置。
- 全量 `flutter test`、`flutter analyze` 和 Android 构建验证。

### 端到端验收

分别用 OpenAI-compatible 和 Anthropic-compatible 的受控测试服务完成一轮聊天，确认后台测试连接与实际聊天使用同一配置。真实流探针必须证明首个 `delta` 在上游结束前到达 App 层。使用新会话验证“不要叫我亲爱的”“回答短一点”“以后先给结论”等偏好跨会话生效，并验证取消偏好后恢复默认。

## 发布与迁移

先发布后端和管理后台，再发布 App。后端上线后，旧聊天配置会显示为未配置；管理员必须选择协议、重新输入对应 API key 并通过最小探测请求后才能启用新运行实例。探测请求可能产生极少量 token/额度，管理后台不得宣称“完全不消耗额度”。部署前保留原配置数据但不再读取 Group ID，便于审计和人工迁移，不提供静默兼容路径。

`POST /privacy/export` 必须导出 preferences；`DELETE /privacy/memories` 必须在同一事务中删除卡片记忆与全局偏好；当前账号注销采用匿名化而不是物理删除，因此 `/privacy/account` 的注销事务必须显式删除 preferences，不能依赖外键 cascade。同步更新隐私政策版本与文本，并测试事务失败回滚。

若 App 版本仍使用既有 `delta/done/error` 协议，可在后端切换后继续工作；新版 App 主要补足字节级实时性、超时和真实延迟测试。发布后执行 OpenAI、Anthropic、偏好跨会话和真机首段时序四类冒烟测试。
