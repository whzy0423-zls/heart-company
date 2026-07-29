# 芯之力 App/后台兼容性修复设计

## 目标

让当前后台分支与手机 App `main` 的芯之力实时语音链路达到端到端协议一致，保留现有 TTS/百炼克隆音色配置能力，并兼容已发布的 v1 客户端。

## 已确认根因

1. App 建立 WebSocket 前调用 `/api/app/xinzhili/realtime/capabilities`，当前后台没有该 GET 路由。
2. App 使用连接 `generation` 丢弃旧连接事件；当前后台服务端控制信封与 MP3 二进制帧未透传当前连接 generation。
3. 当前后台 v1 解码器要求所有新字段且拒绝未知顶层字段，已发布客户端存在字段缺省/扩展差异。
4. 当前后台没有注册旧 `/api/app/xinzhili/turns/stream` 兜底链路；需要确认是否保留兼容处理，不能影响当前 App 首选 WebSocket。
5. 百炼 TTS 配置缺省模型必须使用 `MiniMax/speech-2.8-turbo`，MiniMax 继续使用 `speech-02-hd`。

## 设计

- 新增受 App JWT 保护的 capabilities GET 接口，返回协议版本、能力列表和最低 App 构建号。
- WebSocket 连接建立时记录客户端 generation；所有服务端 JSON 事件和 MP3 帧使用该 generation。
- v1 信封解码保留核心字段校验，但允许已发布客户端缺省 generation/sessionSeq/configVersion/payload，并忽略未知顶层字段；规范化为默认值后继续走既有方向和序列校验。
- 保留现有 `/api/app/xinzhili/realtime` 主链路；对旧 HTTP 路由先做明确状态检查，不擅自改动普通聊天接口。
- 按 provider 选择 TTS 缺省模型，防止百炼音色落到 MiniMax 旧模型。
- 以 Go 单测覆盖路由、generation、宽容解码和 provider 默认值；运行现有后台测试与 App 芯之力测试。

## 验收标准

- App 能成功读取 capabilities 并继续建立 WebSocket。
- `session.ready` 的 generation 等于 App 当前连接 generation。
- 服务端 MP3 帧 generation 等于当前连接 generation，App 能进入 ready 并播放音频。
- 缺省/扩展 v1 信封可被解析，真正错误仍返回协议错误。
- 百炼配置缺省模型为 `MiniMax/speech-2.8-turbo`。
