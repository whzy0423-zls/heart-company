# 百炼公共语音凭证 Implementation Plan

> **For agentic workers:** REQUIRED: Use superpowers:subagent-driven-development (if subagents available) or superpowers:executing-plans to implement this plan. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** 在人声管理和芯之力配置中共用一个百炼 API Key，支持先克隆人声再选择音色，并移除语音相关的硅基流动预设。

**Architecture:** 新增独立 `bailian_shared_credentials` 配置和版本化 API；服务端为克隆、实时 ASR、百炼 TTS 统一解析公共 Key，并仅在公共记录不存在时回退旧百炼配置。后台复用一个公共凭证组件，人声管理作为主要入口，芯之力页面保留同一配置入口但移除 ASR/TTS 重复 Key 输入。

**Tech Stack:** Go、PostgreSQL JSONB、Vue 3、TypeScript、Ant Design Vue、Vitest、Go testing。

---

## Chunk 1: 后端公共凭证与运行时数据流

### Task 1: 公共百炼凭证存储

**Files:**
- Create: `nx-backend/apps/server/internal/bailianconfig/config.go`
- Create: `nx-backend/apps/server/internal/bailianconfig/config_test.go`

- [ ] **Step 1: 写公共凭证存储红测**

覆盖：无记录、创建、已有记录空 Key 保留、显式清空、expectedVersion 冲突，以及“存在空记录时 found=true”。额外覆盖：公共记录不存在时，`apiKey=""` 且 `clearApiKey=false` 为 no-op，不创建空记录，避免意外关闭旧配置回退。

- [ ] **Step 2: 运行红测**

Run:

```bash
cd nx-backend/apps/server
go test ./internal/bailianconfig -count=1
```

Expected: FAIL，包或 Read/Update 尚不存在。

- [ ] **Step 3: 实现版本化存储**

实现：

```go
const ConfigKey = "bailian_shared_credentials"

type Config struct {
    Version int64  `json:"version"`
    APIKey  string `json:"apiKey"`
}

var ErrConflict = errors.New("bailian credentials version conflict")
```

使用独立 advisory lock；已有记录时空输入保留旧 Key，`clearAPIKey` 清空；公共记录不存在时空输入为 no-op；显式清空时即使 Key 为空也创建并保留配置记录。

- [ ] **Step 4: 运行绿测**

Run: `go test ./internal/bailianconfig -count=1`

Expected: PASS。

- [ ] **Step 5: 提交**

```bash
git add nx-backend/apps/server/internal/bailianconfig
git commit -m "feat(voice): store shared bailian credentials"
```

### Task 2: 公共 Key 解析、旧配置回退与安全视图

**Files:**
- Create: `nx-backend/apps/server/internal/server/bailian_credentials.go`
- Create: `nx-backend/apps/server/internal/server/bailian_credentials_test.go`
- Modify: `nx-backend/apps/server/internal/server/server.go`
- Modify: `nx-backend/apps/server/internal/server/server_unit_test.go`

- [ ] **Step 1: 写解析与路由红测**

覆盖：

- 公共记录优先。
- 公共空记录禁止旧 Key 回退。
- 旧百炼/DashScope TTS Key 可回退。
- MiniMax/硅基流动 TTS Key不参与回退。
- 旧阿里百炼 ASR Key 可回退。
- 官方 DashScope endpoint 使用 URL 解析并精确校验 hostname 和支持路径；`dashscope.aliyuncs.com.example` 等相似域名不得参与回退。
- GET 不返回明文，仅返回 `version/apiKeySet/apiKeySuffix/source`。
- PUT 要求 `expectedVersion`，409 不覆盖。
- 首次 PUT 空 `apiKey` 且未显式清空时不创建公共记录，GET 仍可继续读取旧百炼回退状态。
- 两次合法 CAS 更新即使运行时注入完成顺序相反，克隆客户端最终也必须使用数据库最新版本的 Key。
- 路由允许 `Voice:Profile:Manage` 或 `System:XinzhiliModel:Config`。

- [ ] **Step 2: 运行红测**

Run:

```bash
go test ./internal/server -run 'BailianCredentials|BailianCredentialRoute' -count=1
```

Expected: FAIL。

- [ ] **Step 3: 实现接口与解析器**

新增：

```text
GET /api/voice/bailian-credentials
PUT /api/voice/bailian-credentials
```

PUT body：

```json
{"expectedVersion":0,"apiKey":"sk-...","clearApiKey":false}
```

旧配置识别使用 `net/url` 解析并精确匹配官方 DashScope hostname/支持路径，不使用模糊字符串包含判断。保存后刷新 `ConfigureBailianCopy`，固定使用 DashScope endpoint 和 `qwen3-tts-vc-2026-01-22`，不再依赖当前 TTS provider；刷新前重读最新公共记录，或用 mutex + version guard 丢弃旧版本注入，保证并发 CAS 下运行时配置与数据库最新版本一致。

- [ ] **Step 4: 运行绿测**

Run: `go test ./internal/server -run 'BailianCredentials|BailianCredentialRoute' -count=1`

Expected: PASS。

- [ ] **Step 5: 提交**

```bash
git add nx-backend/apps/server/internal/server
git commit -m "feat(voice): expose shared bailian credentials"
```

### Task 3: 克隆客户端启动注入与芯之力运行时覆盖

**Files:**
- Modify: `nx-backend/apps/server/internal/server/server.go`
- Modify: `nx-backend/apps/server/internal/server/xinzhili_model_config.go`
- Modify: `nx-backend/apps/server/internal/server/xinzhili_model_config_test.go`
- Modify: `nx-backend/apps/server/internal/server/app_xinzhili_realtime.go`
- Modify: `nx-backend/apps/server/internal/server/app_xinzhili_realtime_test.go`
- Modify: `nx-backend/apps/server/internal/xinzhili/config.go`
- Modify: `nx-backend/apps/server/internal/xinzhili/config_test.go`

- [ ] **Step 1: 写启动、保存和运行时红测**

覆盖：

- 服务重启后公共 Key 注入克隆客户端。
- TTS 为 MiniMax 时公共 Key 仍可用于百炼克隆，但不覆盖 MiniMax TTS Key。
- 每轮 Paraformer ASR 使用公共 Key。
- 百炼 TTS 使用公共 Key。
- 启用芯之力且公共 Key 缺失时保存失败。
- disabled 配置允许 voice 为空并先保存其他字段。
- 公共记录存在时，保存芯之力清理旧百炼 Key；公共记录不存在时保留旧回退 Key。

- [ ] **Step 2: 运行红测**

Run:

```bash
go test ./internal/server ./internal/xinzhili -run 'Bailian|Xinzhili.*Credential|RuntimeCredential' -count=1
```

Expected: FAIL。

- [ ] **Step 3: 实现运行时凭证覆盖**

增加服务层 helper：读取公共凭证和旧回退，生成仅用于当前请求/轮次的 `xinzhili.Config`。结构校验不再要求百炼 Key 存在于芯之力 JSON；启用保存由 handler 根据 provider 校验公共或历史私有 Key。

- [ ] **Step 4: 改造启动注入**

替换 `applyStoredXinzhiliBailianCopyConfig` 的 provider 绑定逻辑，启动时直接使用公共凭证解析器配置 `voice.Store`。

- [ ] **Step 5: 运行绿测**

Run:

```bash
go test ./internal/server ./internal/xinzhili ./internal/voice -count=1
```

Expected: PASS。

- [ ] **Step 6: 提交**

```bash
git add nx-backend/apps/server/internal/server nx-backend/apps/server/internal/xinzhili
git commit -m "fix(xinzhili): use shared bailian credentials"
```

## Chunk 2: 后台统一配置入口与硅基流动清理

### Task 4: 公共凭证前端 API 与复用组件

**Files:**
- Modify: `nx-backend/apps/web-antd/src/api/core/voice.ts`
- Create: `nx-backend/apps/web-antd/src/views/voice/bailian-credentials-card.vue`
- Create: `nx-backend/apps/web-antd/src/views/voice/bailian-credentials-card.test.ts`

- [ ] **Step 1: 写组件红测**

覆盖：加载安全状态、只显示一个 Key 输入、空值保留、保存成功刷新版本、409 提示重新加载、显式清空确认、读取失败显示错误与“重新加载”入口并可恢复。

- [ ] **Step 2: 运行红测**

Run:

```bash
cd nx-backend
pnpm exec vitest run apps/web-antd/src/views/voice/bailian-credentials-card.test.ts --dom
```

Expected: FAIL。

- [ ] **Step 3: 实现 API 和组件**

组件文案明确：同一个 Key 用于 Paraformer、Qwen 克隆和 Qwen TTS。组件通过事件通知父页面 Key 状态变化；加载失败时保持不可用状态并提供显式重试，不把失败误判为“未配置”。

- [ ] **Step 4: 运行绿测**

Run: `pnpm exec vitest run apps/web-antd/src/views/voice/bailian-credentials-card.test.ts --dom`

Expected: PASS。

- [ ] **Step 5: 提交**

```bash
git add nx-backend/apps/web-antd/src/api/core/voice.ts nx-backend/apps/web-antd/src/views/voice/bailian-credentials-card*
git commit -m "feat(admin): add shared bailian credential card"
```

### Task 5: 人声管理改为先配置 Key 再克隆

**Files:**
- Modify: `nx-backend/apps/web-antd/src/views/voice/profiles.vue`
- Modify: `nx-backend/apps/web-antd/src/views/voice/profiles.provider-platform.test.ts`

- [ ] **Step 1: 写人声管理红测**

要求页面包含公共凭证卡，不再出现“请先在芯之力模型配置中保存百炼 API Key”；Key 未配置或读取失败时禁用克隆，读取失败时可重试，配置后允许上传并克隆。

- [ ] **Step 2: 运行红测**

Run: `pnpm exec vitest run apps/web-antd/src/views/voice/profiles.provider-platform.test.ts --dom`

Expected: FAIL。

- [ ] **Step 3: 改造页面**

把公共凭证卡放在新增人声表单之前；`canSubmit` 同时要求 `apiKeySet`；提示流程改为“保存公共 Key → 上传样本 → 克隆 → 芯之力选择”。

- [ ] **Step 4: 运行绿测并提交**

```bash
pnpm exec vitest run apps/web-antd/src/views/voice --dom
git add nx-backend/apps/web-antd/src/views/voice
git commit -m "feat(admin): configure bailian before voice cloning"
```

### Task 6: 芯之力页面统一 Key 并移除硅基流动

**Files:**
- Modify: `nx-backend/apps/web-antd/src/views/settings/xinzhili-model.vue`
- Modify: `nx-backend/apps/web-antd/src/views/settings/xinzhili-model.test.ts`
- Modify: `nx-backend/apps/web-antd/src/views/settings/xinzhili-model.free-preset.test.ts`
- Modify: `nx-backend/apps/web-antd/src/views/settings/model.xinzhili-voice.test.ts`
- Modify: `nx-backend/apps/web-antd/src/views/settings/model.tts-voice-reuse.test.ts`

- [ ] **Step 1: 将硅基流动旧断言改成红测**

断言：

- 页面不含 `api.siliconflow.cn`、`applyFreeTtsPreset`、免费额度按钮。
- 百炼场景不再渲染 ASR/TTS 两个重复 API Key 输入。
- TTS provider 为 MiniMax 或其他非百炼 provider 时，仍条件式渲染该 provider 自身的私有 API Key 输入，并保持留空不修改。
- 页面渲染同一个 `BailianCredentialsCard`。
- 保留已有音色选择和手动音色 ID。
- 具备 `Voice:Profile:Manage` 权限时显示“前往人声管理克隆音色”，只有 `System:XinzhiliModel:Config` 权限时不显示该跳转但仍可保存公共 Key。
- 公共凭证读取失败时显示重试入口，恢复后可继续选择音色和保存。

- [ ] **Step 2: 运行红测**

Run: `pnpm exec vitest run apps/web-antd/src/views/settings --dom`

Expected: FAIL。

- [ ] **Step 3: 改造芯之力页面**

删除硅基流动预设函数、Alert 和按钮；删除百炼 ASR/TTS 两个重复 Key 表单项并插入公共凭证卡；对 MiniMax/其他非百炼 TTS 保留条件式私有 Key 输入，保存时沿用留空不修改。根据权限条件显示“前往人声管理克隆音色”，公共凭证加载失败时提供重试入口。

- [ ] **Step 4: 运行绿测**

Run: `pnpm exec vitest run apps/web-antd/src/views/settings --dom`

Expected: PASS。

- [ ] **Step 5: 提交**

```bash
git add nx-backend/apps/web-antd/src/views/settings
git commit -m "fix(admin): simplify xinzhili bailian setup"
```

### Task 7: 清理旧模型配置页的硅基流动语音预设

**Files:**
- Modify: `nx-backend/apps/web-antd/src/views/settings/model.vue`
- Modify: `nx-backend/apps/web-antd/src/views/settings/model.xinzhili-voice.test.ts`

- [ ] **Step 1: 写红测**

断言旧模型配置页不再包含 `applySiliconFlowVoicePreset`、硅基流动免费语音按钮及相关说明，其他模型配置表单保持存在。

- [ ] **Step 2: 运行红测**

Run: `pnpm exec vitest run apps/web-antd/src/views/settings/model.xinzhili-voice.test.ts --dom`

Expected: FAIL。

- [ ] **Step 3: 删除语音预设并运行绿测**

Run: `pnpm exec vitest run apps/web-antd/src/views/settings/model*.test.ts --dom`

Expected: PASS。

- [ ] **Step 4: 提交**

```bash
git add nx-backend/apps/web-antd/src/views/settings/model.vue nx-backend/apps/web-antd/src/views/settings/model.xinzhili-voice.test.ts
git commit -m "chore(admin): remove siliconflow voice presets"
```

## Chunk 3: 回归验证与本地验收

### Task 8: 全链路验证

**Files:**
- Verify only.

- [ ] **Step 1: 后端全量测试**

```bash
cd nx-backend/apps/server
go test ./... -count=1
```

Expected: PASS。

- [ ] **Step 2: 后台测试、类型检查和构建**

```bash
cd nx-backend
pnpm exec vitest run apps/web-antd --dom
pnpm --filter @vben/web-antd run typecheck
pnpm --filter @vben/web-antd run build
```

Expected: PASS；仅允许已知 Node engine/pure annotation warning。

- [ ] **Step 3: App 芯之力回归**

```bash
cd /Users/wohenzaiyi/Desktop/nine-xing-app/.worktrees/xinzhili-bailian-compat-20260730
flutter test test/features/xinzhili/services/xinzhili_realtime_session_test.dart test/features/xinzhili/services/xinzhili_socket_test.dart test/features/xinzhili/xinzhili_controller_test.dart
```

Expected: PASS。

- [ ] **Step 4: 本地非 Docker 启动**

```bash
cd /Users/wohenzaiyi/Desktop/nine-xing/.worktrees/classroom-admin-stability-20260730
bash .local-logs/run-local-dev.sh
```

验收：

1. 人声管理只填一个公共 Key。
2. 不进入芯之力页面即可创建克隆音色。
3. 芯之力页面选择 ready 音色。
4. 两个语音页面没有硅基流动预设。
5. `main` 和 `test` 保持不变。

- [ ] **Step 5: 最终提交状态检查**

```bash
git diff --check
git status --short --branch
```

Expected: 工作树干净，所有实现位于 `fix/classroom-admin-stability-20260730`。
