# 百炼公共语音凭证与克隆优先流程设计

## 目标

将阿里百炼语音能力调整为清晰的单向流程：

1. 在“人声管理”只配置一次百炼 API Key。
2. 上传音频并创建百炼 Qwen 克隆音色。
3. 在“芯之力模型配置”选择已经克隆完成的音色。
4. 实时 ASR、Qwen TTS 和人声克隆复用同一个百炼 API Key。

同时移除后台语音配置里的硅基流动预设，保留普通文字问答等非语音模块的既有配置。

## 当前问题

- 人声克隆实际读取“芯之力模型配置”TTS 区域的 API Key，但页面没有明确说明它也是克隆凭证。
- 芯之力页面同时出现 ASR API Key 和 TTS API Key，阿里百炼场景需要重复填写。
- 用户尚未克隆音色时，芯之力页面已经要求选择音色，造成流程倒置。
- “硅基流动免费额度”预设与当前统一使用百炼 Qwen 的方向冲突。

## 方案选择

### 方案 A：新增独立公共凭证配置（采用）

新建一份 `bailian_shared_credentials` 配置，再由百炼 ASR、百炼 TTS、克隆分别读取。

优点是公共 Key 与具体 TTS provider 解耦，不会覆盖 MiniMax 等历史凭证；缺点是需要增加旧配置回退和实时配置覆盖逻辑。

### 方案 B：复用芯之力配置记录，由人声管理提供公共凭证入口

继续使用现有 `xinzhili_model_config` 作为持久化载体，但新增只负责百炼 API Key 的接口。保存时同步写入 `realtimeAsr.apiKey` 和 `tts.apiKey`，界面只暴露一个公共 Key。

优点是兼容现有加密/留空不修改、配置版本、启动注入和实时运行逻辑；也允许在芯之力尚未启用、音色尚未创建时先保存 Key。

### 方案 C：只在前端复制 ASR Key 到 TTS Key

改动最少，但人声管理仍依赖另一个页面，刷新或多管理员操作时也容易再次产生不一致。

## 后端设计

### 独立公共凭证存储

新增 `internal/bailianconfig`，使用 `site_configs` 的独立 key：

```text
bailian_shared_credentials
```

存储结构：

```json
{
  "version": 1,
  "apiKey": "sk-..."
}
```

公共凭证拥有独立版本和事务锁，不复用芯之力配置的 CAS 版本，因此保存 Key 不会覆盖管理员同时编辑的模式、音色或时序配置。

旧配置回退顺序：

1. 已保存的公共百炼 Key。
2. 仅当旧 TTS provider 为 `bailian`，或 `openai-compatible + 官方 DashScope endpoint` 时，使用旧 TTS Key。
3. 使用旧 `aliyun-bailian` 实时 ASR Key。

MiniMax、硅基流动及其他 provider 的 TTS Key 不参与公共百炼 Key 回退。

旧配置回退只在 `bailian_shared_credentials` 记录从未创建时执行。一旦公共记录存在，即使 Key 被显式清空，也以公共记录为准，不再重新启用旧 Key。

### 公共凭证接口

新增：

- `GET /api/voice/bailian-credentials`
- `PUT /api/voice/bailian-credentials`

访问权限满足任一即可：

- `Voice:Profile:Manage`
- `System:XinzhiliModel:Config`

GET 仅返回安全视图：

```json
{
  "version": 1,
  "apiKeySet": true,
  "apiKeySuffix": "Q9UY",
  "source": "shared"
}
```

PUT 请求：

```json
{
  "expectedVersion": 1,
  "apiKey": "sk-...",
  "clearApiKey": false
}
```

空字符串表示保留原值；显式 `clearApiKey=true` 才清空。版本冲突返回 HTTP 409，前端重新加载后再保存。

### 保存逻辑

保存公共 Key 不创建或修改芯之力模型配置，也不覆盖 MiniMax/其他历史 TTS Key。保存成功后立即刷新人声管理使用的百炼克隆客户端。

服务启动时同样读取“公共 Key + 旧配置回退”，确保重启后克隆客户端仍能取得有效凭证，不再根据当前 TTS provider 决定是否加载克隆 Key。

芯之力每次开始会话/轮次时读取公共凭证并覆盖运行时配置：

- Paraformer ASR 始终使用公共百炼 Key。
- TTS provider 为百炼或旧 DashScope 兼容配置时使用公共百炼 Key。
- MiniMax 等非百炼 TTS 保留自身历史 Key，不被覆盖。

### 芯之力校验与持久化

`xinzhili.Config` 的结构校验继续负责 provider、endpoint、model、voice、format 和模式参数，不再要求百炼 API Key 必须存放在芯之力 JSON 内。

保存启用状态时，由服务层执行凭证校验：

- Paraformer ASR 要求公共百炼 Key 已配置。
- 百炼/DashScope TTS 要求公共百炼 Key 已配置。
- MiniMax 等历史 TTS 仍要求其自身旧 Key，不读取公共百炼 Key。

写入 `xinzhili_model_config` 前：

- 如果公共凭证记录已经存在，`realtimeAsr.apiKey` 清空，不重复保存公共 Key。
- 如果公共凭证记录已经存在，百炼/DashScope TTS 的 `tts.apiKey` 清空。
- 如果公共凭证记录尚未创建，继续保留符合回退条件的旧百炼 Key，避免用户仅修改提示词或时序参数时丢失旧凭证。
- 非百炼历史 TTS 的私有 Key继续按原来的“留空不修改”规则保存。

公共 Key 首次保存后，公共记录立即成为唯一生效来源；旧字段即使暂时存在也不再参与解析，并在下一次芯之力配置保存时清理。运行时通过公共凭证覆盖空的百炼 Key，再交给 ASR/TTS provider。未启用配置允许不选择音色，便于先去人声管理克隆。

## 后台交互设计

### 人声管理

在人声管理“新增人声”区域顶部增加“阿里百炼语音服务”配置卡：

- API Key 输入框。
- 已配置状态与尾号。
- “保存 API Key”按钮。
- 文案说明该 Key 同时用于 Paraformer 实时识别、Qwen 音色克隆和 Qwen TTS。

未配置时禁用“保存并克隆”，并提示先保存百炼 API Key。配置完成后用户直接上传样本并克隆。

### 芯之力模型配置

- 移除 ASR 和 TTS 两个独立 API Key 输入框。
- 显示一个“百炼公共 API Key”配置卡，与人声管理读写同一接口。
- 有人声管理权限时提供“前往人声管理克隆音色”入口；只有芯之力配置权限的管理员也能在当前页保存公共 Key。
- 保留 TTS provider、endpoint、model、已有音色选择和手动音色 ID。
- 未选择音色时允许保存禁用配置；启用芯之力时仍要求 ready 音色。

### 硅基流动清理

移除以下语音相关内容：

- 芯之力模型配置的“硅基流动免费额度 TTS 预设”。
- 旧模型配置页的“硅基流动免费语音预设”。

普通文字问答、旧 ASR 环境默认值等非本次语音界面不改动。

## 错误处理

- Key 保存失败时保留输入内容并展示后端错误。
- Key 版本冲突时提示重新加载，不静默覆盖其他管理员的保存。
- 克隆返回 401 时提示“百炼 API Key 无效或未开通声音复刻服务”。
- 公共凭证读取失败时，人声管理停止克隆按钮并显示重试入口。
- 芯之力音色列表读取失败时仍保留手动音色 ID 输入。

## 测试与验收

- 没有芯之力配置记录时，可在人声管理只填写一个 Key 并保存。
- 保存后百炼 ASR、百炼 TTS 和人声克隆读取同一个 Key。
- MiniMax/其他历史 TTS Key 不被公共百炼 Key 覆盖。
- 只有旧百炼 TTS 或旧百炼 ASR Key 时仍能平滑回退；硅基流动 TTS Key不会被误识别为百炼 Key。
- 未创建音色时，人声管理可以先完成克隆。
- ready 百炼音色出现在芯之力“选择已有音色”。
- 芯之力页面不再显示两个 API Key 输入框。
- 两个语音配置页面不再出现硅基流动语音预设。
- 旧配置 API Key 保留，留空保存不会清除。
- 后端、后台和 App 现有芯之力实时语音测试继续通过。
