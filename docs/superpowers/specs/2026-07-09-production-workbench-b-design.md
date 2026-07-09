# 制片工作台 B 方案迁移设计

## 背景

用户要把 `/Users/wohenzaiyi/dd-liuguang-web` 的“制片工作台”能力迁移到当前项目 `/Users/wohenzaiyi/Desktop/nine-xing`，并确认按 B 方案执行：

- 新增制片工作台模式入口。
- 项目制复用目标项目已有 `/video/projects` 和 `/video/projects/:id/workbench`。
- 短片制先留占位。
- 在现有项目工作台中加入源项目式五步生产流程：`剧本录入 → 资产分析 → 创建资产 → 分镜设计 → 剪辑合成`。
- 不直接复制源项目 Element Plus 页面，而是按目标项目 Vue + Ant Design Vue / Vben 体系落地。

## 目标

第一阶段交付一个可进入、可理解、可继续扩展的制片工作台：用户从“制片工作台”入口选择项目制或短片制；项目制进入现有视频项目列表；进入项目工作台后看到五步生产流程，并能在每个步骤下使用或跳转到现有项目、资产、分镜、批量生成、合成能力。

## 非目标

第一阶段不迁移源项目全部重功能：

- 不新增剧集 / 分集后端模型。
- 不把 Element Plus 组件整页复制进目标项目。
- 不一次性迁移源项目的高级分镜工作台、视频版本管理、批量九宫格、预算积分、团队成员、复杂资产编辑弹窗。
- 短片制只做路由、页面和后续规划占位，不接后端业务。

## 用户流程

### 入口流程

```text
/video/production
  ├─ 项目制：进入 /video/projects
  └─ 短片制：进入 /video/production/short，占位提示“功能规划中”
```

### 项目制流程

```text
/video/projects
  └─ 新建或选择项目
      └─ /video/projects/:id/workbench
          ├─ 剧本录入
          ├─ 资产分析
          ├─ 创建资产
          ├─ 分镜设计
          └─ 剪辑合成
```

## 页面与组件设计

### 1. 制片工作台入口页

新增 `apps/web-antd/src/views/video/production/index.vue`。

职责：

- 展示“项目制”和“短片制”两张模式卡片。
- 项目制主按钮跳转 `/video/projects`。
- 短片制主按钮跳转 `/video/production/short`。
- 文案明确项目制适合完整项目流程，短片制用于单条短片快速制作且当前占位。

UI 原则：

- 使用 Ant Design Vue 卡片、按钮、标签。
- 保持 Vben 后台页风格。
- 触控/点击区域不低于 44px，避免小图标裸点。
- 路由入口在菜单中显示为“制片工作台”。

### 2. 短片制占位页

新增 `apps/web-antd/src/views/video/production/short.vue`。

职责：

- 展示短片制工作台占位。
- 明确后续方向：脚本输入、资产选择、快速分镜、一键生成、成片导出。
- 提供返回制片模式入口和进入项目制的按钮。

### 3. 路由改造

修改 `apps/web-antd/src/router/routes/modules/video.ts`。

新增：

- `/video/production`：制片工作台入口，菜单可见。
- `/video/production/short`：短片制占位，菜单隐藏，activeMenu 指向 `/video/production`。

保留：

- `/video/projects`：项目列表，项目制核心入口。
- `/video/projects/:id/workbench`：项目工作台。

### 4. 项目工作台五步流程

修改 `apps/web-antd/src/views/video/projects/workbench.vue`。

新增顶部或主内容区的流程步骤条，步骤为：

1. 剧本录入
2. 资产分析
3. 创建资产
4. 分镜设计
5. 剪辑合成

每个步骤对应一个引导面板：

- **剧本录入**：第一阶段提供文本录入/导入占位区与“从剧本拆解资产和分镜”的说明。若目标后端暂未支持剧本文档持久化，先用前端状态，不影响现有项目保存。
- **资产分析**：展示源项目“分析角色、场景、物品、画面”等语义；第一阶段桥接到现有资产库状态，显示当前角色数、场景数、分镜数，提供刷新/进入创建资产。
- **创建资产**：聚合现有左侧资产库能力，提示人物、场景、物品、音频、视频、风格等资产统一上传到阿里云 OSS 文件桶；保留已有生成参考图/上传资产能力。
- **分镜设计**：激活现有分镜列表、分镜编辑、绑定资产、单镜生成、批量生成能力。
- **剪辑合成**：聚合已有“合成视频”、合成进度和最终成片链接能力。

现有三栏工作台结构尽量保留，避免一次性大重构；五步流程作为工作台的生产导航和上下文说明层加入。

## 数据流设计

第一阶段尽量复用现有 API：

- 项目：`getProjectApi`、`listProjectsApi`、`createProjectApi`。
- 资产：现有角色、场景、项目资产接口。
- 分镜：`listShotsApi`、`createShotApi`、`updateShotApi`、`generateShotApi`、`batchGenerateShotsApi`。
- 合成：`composeProjectVideoApi`、`getComposeStatusApi`。

新增前端状态：

```ts
const productionStep = ref<'script' | 'analysis' | 'assets' | 'storyboard' | 'compose'>('script');
const scriptDraft = ref('');
```

第一阶段不新增数据库字段。后续如果要持久化剧本和生产步骤，可新增后端字段或独立 production session 表。

## 错误处理

- 入口页和短片占位页没有远程请求，主要保证路由稳定。
- 工作台沿用现有 API 错误提示机制。
- 五步面板中触发现有异步能力时，继续使用 loading、disabled、message 提示。
- 对短片制占位明确说明“暂未开放”，避免用户误以为功能失败。

## 测试设计

采用 TDD，先写失败测试，再实现：

1. 路由测试：验证存在 `/video/production` 和 `/video/production/short`。
2. 页面源码测试：验证入口页包含项目制、短片制、跳转路径、占位文案。
3. 工作台源码测试：验证五步文案存在，且包含资产统一上传到阿里云 OSS 文件桶的提示。
4. 原有工作台残留测试：若 `workbench-redesign.test.ts` 是前一任务遗留失败测试，需要评估是否纳入本次修复或调整为当前工作台布局要求。
5. 构建验证：至少运行相关 vitest；尝试 `pnpm -F @vben/web-antd run build`。若 typecheck 存在既有无关错误，记录具体文件和错误，不把它当成本次失败。

## 迁移边界

本设计把源项目能力迁移为“目标项目可承载的产品流程和页面结构”，而不是代码级复制。这样可以保留目标项目已有后端、OSS、资产、分镜和合成能力，也给短片制留下清晰扩展点。
