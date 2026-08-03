# 无限画布能力独立模型配置实施计划

> **For agentic workers:** REQUIRED: Use superpowers:subagent-driven-development (if subagents available) or superpowers:executing-plans to implement this plan. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** 将无限画布的图片、视频、文本、音频模型配置拆成四套独立的浏览器本地配置，并让各类节点只使用对应能力的 API Base、API Key、协议和模型 ID。

**Architecture:** 在 iframe 内的 React 应用增加能力配置模型和配置弹窗；Zustand 负责版本化 localStorage 迁移和独立更新；所有生成服务通过显式 capability resolver 获取请求配置。节点默认继承能力配置，旧节点的 `channelId::model` 覆盖值只做兼容解码，不再携带凭据。

**Tech Stack:** React 19、TypeScript、Zustand persist、Ant Design、Vitest、Vite。

---

## 文件地图

- Modify: `nx-backend/apps/infinite-canvas/src/stores/use-config-store.ts` — 能力配置类型、默认值、版本迁移、能力解析器和协议校验。
- Create: `nx-backend/apps/infinite-canvas/src/components/capability-model-config-dialog.tsx` — 四类能力配置 UI、密钥输入、脚本高级编辑和保存反馈。
- Modify: `nx-backend/apps/infinite-canvas/src/pages/canvas/project.tsx` — 挂载配置弹窗、传递目标能力、节点生成入口使用能力解析。
- Modify: `nx-backend/apps/infinite-canvas/src/components/model-picker.tsx` — 从渠道列表改为当前能力模型显示/切换兼容逻辑。
- Modify: `nx-backend/apps/infinite-canvas/src/components/canvas/canvas-node-prompt-panel.tsx` — 图片/视频/文本/音频节点的能力配置入口和缺失提示。
- Modify: `nx-backend/apps/infinite-canvas/src/components/canvas/canvas-config-node-panel.tsx` — 配置节点的能力配置入口和模型显示。
- Modify: `nx-backend/apps/infinite-canvas/src/services/api/image.ts` — 显式接收 image 能力配置，保留脚本与协议处理。
- Modify: `nx-backend/apps/infinite-canvas/src/services/api/video.ts` — 显式接收 video 能力配置，校验视频协议和轮询快照。
- Modify: `nx-backend/apps/infinite-canvas/src/services/api/audio.ts` — 显式接收 audio 能力配置，拒绝 Gemini。
- Modify: `nx-backend/apps/infinite-canvas/src/lib/seedance-video.ts` — 使用 video 能力解析结果。
- Modify: `nx-backend/apps/infinite-canvas/src/services/api/model-plugin.ts` — 脚本调用接收能力级请求配置。
- Modify: `nx-backend/apps/infinite-canvas/src/lib/canvas/canvas-generation-helpers.ts` — `buildGenerationConfig` 改为能力感知解析。
- Modify: `nx-backend/apps/infinite-canvas/src/lib/canvas/canvas-node-factory.ts` — 节点元数据默认模型改为能力模型 ID。
- Modify: `nx-backend/apps/infinite-canvas/src/stores/canvas/use-canvas-store.ts` — 本地项目 rehydrate 时迁移 `channelId::model`。
- Modify: `nx-backend/apps/infinite-canvas/src/pages/canvas/index.tsx` — 导入项目时执行旧模型覆盖值归一化。
- Modify: `nx-backend/apps/infinite-canvas/src/types/canvas.ts` — 明确旧模型覆盖值与新能力默认值的兼容语义。
- Test: `nx-backend/apps/infinite-canvas/src/stores/use-config-store.test.ts` — 独立更新、迁移、解析、协议校验。
- Test: `nx-backend/apps/infinite-canvas/src/components/capability-model-config-dialog.test.tsx` — 四个区域独立编辑和保存。
- Test: `nx-backend/apps/infinite-canvas/src/services/api/capability-config-isolation.test.ts` — 异步请求配置快照和跨能力隔离。
- Test: `nx-backend/apps/infinite-canvas/src/lib/canvas/project-config-export.test.ts` — 导出不含 API Key，旧节点模型覆盖可恢复。
- Test: `nx-backend/apps/infinite-canvas/src/stores/canvas/use-canvas-store.test.ts` — 本地 rehydrate 和导入归一化。

## Chunk 1: Store 数据模型、迁移和解析器

### Task 1: 写能力配置的失败测试

**Files:**
- Create: `nx-backend/apps/infinite-canvas/src/stores/use-config-store.test.ts`

- [ ] **Step 1: 测试四类配置的独立更新**

断言更新 image 的 `apiKey`、`modelId` 不改变 video/text/audio 的字段。

- [ ] **Step 2: 测试旧渠道配置迁移**

覆盖：旧 `channels`、无渠道时的扁平 `baseUrl/apiKey/apiFormat/model`、`channelId::model`、自定义 `script`、缺失字段和重复执行迁移。

- [ ] **Step 3: 测试显式 capability resolver**

断言 image/video/text/audio 分别返回自身的 `apiBase/apiKey/apiFormat/model`，且不会按裸模型名回退到其他能力。

- [ ] **Step 4: 测试协议矩阵**

断言音频 Gemini、视频 Gemini 等持久化非法组合得到能力级协议错误，而不是静默使用其他能力配置。

- [ ] **Step 5: 运行测试确认失败**

Run: `pnpm --dir nx-backend/apps/infinite-canvas exec vitest run --dom src/stores/use-config-store.test.ts`

Expected: FAIL，因为能力配置类型、迁移和 resolver 尚未实现。

### Task 2: 实现能力配置和版本化迁移

**Files:**
- Modify: `nx-backend/apps/infinite-canvas/src/stores/use-config-store.ts`

- [ ] **Step 1: 增加 `CapabilityModelConfig` 与 `CapabilityConfigs` 类型**

字段包含 `apiBase`、`apiKey`、`apiFormat`、`modelId` 和兼容用 `script`；保留各能力原有图片、视频、文本、音频参数。

- [ ] **Step 2: 增加四类默认配置和能力标签类型**

默认空密钥、合理默认协议；禁止把一个能力的默认配置对象引用给其他能力。

- [ ] **Step 3: 增加 Zustand persist `version`/`migrate`**

迁移新结构、旧渠道结构和旧扁平结构；迁移 `channelId::model` 为原始模型 ID；保留 WebDAV；迁移必须幂等且能处理部分损坏数据。

- [ ] **Step 4: 增加能力解析器**

实现 `resolveCapabilityRequestConfig(config, capability, modelOverride?)`，明确返回当前能力的凭据、协议、模型名和脚本。模型覆盖只影响模型名，不改变凭据来源。

- [ ] **Step 5: 增加能力级校验和错误类型**

提供缺 API Base、缺 API Key、缺模型 ID、协议不支持四类错误；错误中带 capability，不发生跨能力回退。

- [ ] **Step 6: 运行 store 测试确认通过**

Run: `pnpm --dir nx-backend/apps/infinite-canvas exec vitest run --dom src/stores/use-config-store.test.ts`

Expected: PASS。

- [ ] **Step 7: Commit**

```bash
git add nx-backend/apps/infinite-canvas/src/stores/use-config-store.ts nx-backend/apps/infinite-canvas/src/stores/use-config-store.test.ts
git commit -m "feat: add capability-specific canvas model config"
```

## Chunk 2: 配置弹窗和画布入口

### Task 3: 写配置弹窗失败测试

**Files:**
- Create: `nx-backend/apps/infinite-canvas/src/components/capability-model-config-dialog.test.tsx`
- Modify if missing: `nx-backend/apps/infinite-canvas/package.json` — UI 测试所需 happy-dom/testing library 依赖。

- [ ] **Step 1: 测试四个配置区域可独立填写**

渲染弹窗，分别填写图片、视频、文本、音频字段，断言保存回调收到四套独立对象。

- [ ] **Step 2: 测试目标能力定位**

传入 `targetCapability="video"` 时默认打开视频区域；缺失配置提示能够打开对应区域。

- [ ] **Step 3: 测试密钥和脚本字段**

API Key 使用密码控件；高级脚本默认折叠但可编辑；保存后回调不泄漏到页面文本或错误提示。

- [ ] **Step 4: 运行测试确认失败**

Run: `pnpm --dir nx-backend/apps/infinite-canvas exec vitest run --dom src/components/capability-model-config-dialog.test.tsx`

Expected: FAIL，因为组件尚未存在。

### Task 4: 实现配置弹窗并挂载

**Files:**
- Create: `nx-backend/apps/infinite-canvas/src/components/capability-model-config-dialog.tsx`
- Modify: `nx-backend/apps/infinite-canvas/src/pages/canvas/project.tsx`
- Modify: `nx-backend/apps/infinite-canvas/src/stores/use-config-store.ts`

- [ ] **Step 1: 实现四个标签/折叠区域**

每个区域编辑 API Base、API Key、协议、模型 ID、对应能力参数和高级脚本。

- [ ] **Step 2: 扩展 `openConfigDialog` 状态**

增加 `targetCapability`；打开时定位图片/视频/文本/音频；保存事件只提交被编辑的能力，其他三类配置保持原对象和原值不变。

- [ ] **Step 3: 在无限画布应用根部挂载弹窗**

从顶部画布菜单增加“模型配置”；生成缺配置时打开弹窗并定位对应能力，保存后恢复原操作提示。

- [ ] **Step 4: 运行 UI 测试确认通过**

Run: `pnpm --dir nx-backend/apps/infinite-canvas exec vitest run --dom src/components/capability-model-config-dialog.test.tsx`

Expected: PASS。

- [ ] **Step 5: Commit**

```bash
git add nx-backend/apps/infinite-canvas/src/components/capability-model-config-dialog.tsx nx-backend/apps/infinite-canvas/src/components/capability-model-config-dialog.test.tsx nx-backend/apps/infinite-canvas/src/pages/canvas/project.tsx nx-backend/apps/infinite-canvas/src/stores/use-config-store.ts
git commit -m "feat: add canvas capability model settings dialog"
```

## Chunk 3: 节点选择和生成服务接入

### Task 5: 改造节点模型显示和能力入口

**Files:**
- Modify: `nx-backend/apps/infinite-canvas/src/components/model-picker.tsx`
- Modify: `nx-backend/apps/infinite-canvas/src/components/canvas/canvas-node-prompt-panel.tsx`
- Modify: `nx-backend/apps/infinite-canvas/src/components/canvas/canvas-config-node-panel.tsx`

- [ ] **Step 1: 写节点能力选择测试**

断言图片节点只读取 image 配置，视频节点只读取 video 配置；旧 `channelId::model` 显示为原始模型名。

- [ ] **Step 2: 运行测试确认失败**

Run: `pnpm --dir nx-backend/apps/infinite-canvas exec vitest run --dom src/components/canvas`

Expected: FAIL，因为节点仍使用渠道模型列表。

- [ ] **Step 3: 将模型选择改为能力默认模型 + 兼容覆盖**

新节点默认使用能力配置的 `modelId`；旧节点覆盖值可显示和重置；节点 UI 不展示其他能力的模型或密钥。

- [ ] **Step 4: 补齐缺配置入口**

`onMissingConfig` 传入明确 capability，打开对应配置区域。

- [ ] **Step 5: 运行节点测试**

Run: `pnpm --dir nx-backend/apps/infinite-canvas exec vitest run --dom src/components/canvas`

Expected: PASS。

### Task 6: 改造图片、视频、文本、音频服务

**Files:**
- Modify: `nx-backend/apps/infinite-canvas/src/services/api/image.ts`
- Modify: `nx-backend/apps/infinite-canvas/src/services/api/video.ts`
- Modify: `nx-backend/apps/infinite-canvas/src/services/api/audio.ts`
- Modify: `nx-backend/apps/infinite-canvas/src/lib/seedance-video.ts`
- Modify: `nx-backend/apps/infinite-canvas/src/services/api/model-plugin.ts`
- Modify: `nx-backend/apps/infinite-canvas/src/pages/canvas/project.tsx`
- Modify: `nx-backend/apps/infinite-canvas/src/lib/canvas/canvas-generation-helpers.ts`
- Modify: `nx-backend/apps/infinite-canvas/src/lib/canvas/canvas-node-factory.ts`

- [ ] **Step 1: 写服务隔离失败测试**

构造四类配置并发起生成，断言每个请求使用对应能力的 API Base、API Key、协议和模型 ID。

覆盖 `project.tsx` 的图片生成、图片编辑/问答、批量生成、视频创建、视频轮询、文本流式响应、音频生成和 Seedance 路径；所有路径都通过 `buildGenerationConfig` 或能力 resolver 传递 capability。

同时断言鉴权错误、插件异常和轮询异常中不包含完整 API Key。

- [ ] **Step 2: 运行测试确认失败**

Run: `pnpm --dir nx-backend/apps/infinite-canvas exec vitest run --dom src/services/api/capability-config-isolation.test.ts`

Expected: FAIL，因为服务仍依赖渠道解析。

- [ ] **Step 3: 替换裸模型/渠道解析**

所有服务入口显式接收 capability，调用 `resolveCapabilityRequestConfig`；脚本从当前能力配置读取。

- [ ] **Step 4: 固定异步请求配置快照**

生成任务开始时复制当前能力请求配置；视频轮询、重试和回调继续使用该快照，不读取后续被修改的其他配置。

- [ ] **Step 5: 增加能力级协议校验**

视频/音频拒绝 Gemini；不支持的组合返回当前能力错误；不得回退到其他能力。

- [ ] **Step 6: 运行隔离测试**

Run: `pnpm --dir nx-backend/apps/infinite-canvas exec vitest run --dom src/services/api/capability-config-isolation.test.ts`

Expected: PASS。

- [ ] **Step 6: Commit**

```bash
git add nx-backend/apps/infinite-canvas/src/services/api nx-backend/apps/infinite-canvas/src/lib/seedance-video.ts nx-backend/apps/infinite-canvas/src/pages/canvas/project.tsx nx-backend/apps/infinite-canvas/src/services/api/capability-config-isolation.test.ts
git commit -m "feat: route canvas generation by capability config"
```

## Chunk 4: 导入导出、回归和交付验证

### Task 7: 保护项目导入导出边界

**Files:**
- Modify: `nx-backend/apps/infinite-canvas/src/lib/canvas/canvas-export.ts`
- Modify: `nx-backend/apps/infinite-canvas/src/stores/canvas/use-canvas-store.ts`
- Modify: `nx-backend/apps/infinite-canvas/src/pages/canvas/index.tsx`
- Modify: `nx-backend/apps/infinite-canvas/src/types/canvas-export.ts`
- Test: `nx-backend/apps/infinite-canvas/src/lib/canvas/project-config-export.test.ts`
- Test: `nx-backend/apps/infinite-canvas/src/stores/canvas/use-canvas-store.test.ts`

- [ ] **Step 1: 写失败测试**

导出包含模型覆盖值但不包含四类 API Key、API Base 或完整配置对象；使用递归字段扫描验证任意节点嵌套字段也不含 `apiKey`；导入旧节点 `channelId::model` 后得到原始模型 ID。覆盖 `use-canvas-store.ts` rehydrate 和 `pages/canvas/index.tsx` 导入两条路径。

- [ ] **Step 2: 运行测试确认失败**

Run: `pnpm --dir nx-backend/apps/infinite-canvas exec vitest run --dom src/lib/canvas/project-config-export.test.ts src/stores/canvas/use-canvas-store.test.ts`

Expected: FAIL，因为当前导出保留完整节点 JSON，导入也未统一归一化旧模型 ID。

- [ ] **Step 3: 实现脱敏导出和兼容导入**

过滤凭据字段，保留必要的节点模型元数据；对未知旧覆盖值按能力默认模型回退。

- [ ] **Step 4: 运行导入导出测试**

Run: `pnpm --dir nx-backend/apps/infinite-canvas exec vitest run --dom src/lib/canvas/project-config-export.test.ts src/stores/canvas/use-canvas-store.test.ts`

Expected: PASS。

### Task 8: 全量验证和生产资源构建

**Files:**
- No new source files; verify all modified files.

- [ ] **Step 1: 运行无限画布完整测试**

Run: `pnpm --dir nx-backend/apps/infinite-canvas exec vitest run --dom`

Expected: PASS；若环境缺少 DOM 测试环境，先按项目现有测试配置修正，不跳过失败测试。

- [ ] **Step 2: 运行类型检查**

Run: `pnpm --dir nx-backend/apps/infinite-canvas run typecheck`

Expected: exit 0。

- [ ] **Step 3: 构建并更新后台 iframe 静态资源**

Run: `pnpm --dir nx-backend/apps/infinite-canvas run build`

Expected: Vite 生成 `nx-backend/apps/web-antd/public/infinite-canvas/`，无编译错误。

- [ ] **Step 4: 手工验证四类配置**

打开后台无限画布：分别填写四类 API Base、API Key、模型 ID；执行图片、视频、文本、音频节点，确认请求地址和模型互不串用；刷新页面确认本地配置仍在。

- [ ] **Step 5: 检查密钥边界**

验证项目导出文件、节点元数据、URL、控制台和错误提示均不包含完整 API Key。

- [ ] **Step 6: Commit**

```bash
git add nx-backend/apps/infinite-canvas nx-backend/apps/web-antd/public/infinite-canvas
git commit -m "feat: complete independent canvas model configuration"
```
