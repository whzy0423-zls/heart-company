# 引导式视频制片工作台设计

## 背景与现状

当前视频项目工作台已经具备项目、角色、场景、分镜、参考素材、单镜生成、批量生成、视频版本和成片合成能力，但这些能力集中在一个超过 5500 行的 Vue 页面中，并同时铺在三栏界面里。

现有五步流程 `剧本录入 -> 资产分析 -> 创建资产 -> 分镜设计 -> 剪辑合成` 只改变顶部说明和按钮动作，下面的角色/场景栏、分镜卡和详情栏始终不变。用户看到的是流程导航，操作体验仍是功能集合。入口页还要求用户先理解“项目制”和尚未开放的“短片制”，进一步增加了决策成本。

从现有页面和运行截图可以确认以下主要问题：

- 信息密度失控：工作台顶部、左侧资产库、中间分镜三列、右侧详情和卡片内部三列同时竞争注意力。
- 主操作不明确：项目设置、添加分镜、批量生成、单镜生成、刷新版本和合成视频同时出现。
- 流程与能力脱节：剧本步骤没有项目级剧本编辑器，资产分析没有真正的分析结果，生成和版本管理被塞在分镜卡内。
- 参数过早暴露：模型、分辨率、音画模式等高级参数对每个镜头始终可见。
- 状态反馈分散：生成状态、轮询状态、版本状态、批量进度和合成状态位于不同区域，失败后缺少统一恢复入口。
- 视觉语言冲突：深色流程头、浅色后台容器、深色视频表单和多种边框/圆角混用，页面显得重且碎。
- 移动和窄屏退化明显：三栏只能纵向堆叠，流程与操作之间距离很长。

## 目标

本次改造将现有功能重组为一套“系统带着用户完成视频”的引导式制片流程。

成功标准：

1. 用户进入项目后，能在 10 秒内判断当前处于哪一步、还缺什么、下一步做什么。
2. 每个步骤只保留一个视觉主操作，次要操作不会与主操作竞争。
3. 剧本、素材、分镜、生成、导出成为真实的五个工作区，而不是同一页面的五个说明状态。
4. 普通用户无需理解模型、分辨率和版本管理即可完成首次生成；熟练用户仍可展开高级设置。
5. 生成前明确展示“可生成、待完善、生成中、已完成”的镜头数量，并只提交准备完成的镜头。
6. 异步操作有持续可见的进度、成功反馈、失败原因和就地恢复动作。
7. 桌面端不再出现固定三栏挤压；窄屏下没有横向滚动，核心操作保持可达。

## 非目标

- 不重写现有视频模型、提示词组装、OSS 上传、轮询和合成后端。
- 不引入完整非线性时间线编辑器、多人协作、预算积分或复杂权限体系。
- 不在本次实现 AI 剧本解析服务。剧本转分镜先使用可预测、可编辑的段落拆分。
- 不保留无后端能力支撑的“资产分析”伪步骤。
- 不实现仍处于占位状态的短片制独立工作台。

## 规格权威性与旧方案兼容

本规格是当前 `/video/projects/:id/workbench` 用户体验和交付范围的权威来源，明确取代 `2026-07-10-seedance2-novice-video-workflow-design.md` 中“七步默认向导、七步前端路由和一次性交付全部 AI 编排能力”的部分。原因是当前后端没有完成 AI 拆解、AI 资产候选和 AI 分镜版本链，继续把这些步骤作为默认入口会产生不可执行的空流程。

旧规格和旧实施计划中的以下安全基础仍然有效，可独立实施或被本规格复用：

- Seedance 模型能力注册表和服务端统一参数校验。
- 提示词编译、素材编号与参考顺序一致性。
- 付费生成请求幂等、未知结果对账和 POST 不自动重试。
- 生成结果选择、内容过期判定和合成输入指纹。
- 老项目双读兼容与不删除历史生成结果。

其中付费生成提交安全不是可选后续，而是本工作台开放“生成”步骤的发布前置。本次实施原样采用旧计划 Task 4A/4B 的请求键状态机，再接入生成 UI。

权威状态迁移：

| 当前状态 | 允许目标 | 是否占用镜头活动锁 |
| --- | --- | --- |
| `prepared` | `submitting`、`cancelled` | 是 |
| `submitting` | `accepted`、`unknown_outcome` | 是 |
| `unknown_outcome` | `reconciled` | 是 |
| `reconciled` | `completed`、`failed` | 是，继续轮询关联任务 |
| `accepted` | `completed`、`failed` | 是，继续轮询关联任务 |
| `completed` | 无 | 否 |
| `failed` | 无 | 否 |
| `cancelled` | 无 | 否 |

- 活动状态固定为 `prepared/submitting/accepted/unknown_outcome/reconciled`；终态固定为 `completed/failed/cancelled`。只有终态释放同一镜头的唯一活动锁。
- `requestKey` 由客户端在一次明确的用户生成动作开始时生成 UUID，并随生成请求提交；服务端不提供“申请请求键”接口。
- 相同请求键重复提交返回同一 submission/generation，不重复 POST；新请求键在镜头有活动提交时被拒绝。
- `prepared` 仅在尚未调用上游时允许取消。`submitting/unknown_outcome/reconciled/accepted` 不允许由用户放弃或自动取消。
- 上游 POST 不做网络层自动重试；响应不确定进入 `unknown_outcome`，只能以相同请求键和已知/人工提供的 task ID 调用幂等 `Reconcile`，不能换键重投。
- `Reconcile(requestKey, taskId)` 重复附加相同 task ID 成功且只产生一条 generation；不同 task ID 返回冲突；成功后进入 `reconciled` 并恢复普通任务轮询。
- 生成请求输入至少为 `{ requestKey, shotId }`；响应为 `{ submissionId, requestKey, status, generationId?, taskId? }`。刷新/对账使用 `submissionId/requestKey`，选片只使用 `generationId`。
- UI 在 `unknown_outcome` 时只显示“检查任务状态/人工对账”，不显示普通“重试生成”。明确“再生成一个版本”只能在原提交终态后创建新 UUID。
- 发布门禁包含完整迁移表拒绝用例、重复点击、并发新键、prepared 取消、未知结果、相同/冲突 task ID 对账、本地关联失败、进程恢复、终态释放锁和明确新版本测试；任何一项未通过都不能启用付费生成按钮。

路由决策：

- `/video/projects/:id/workbench` 挂载本规格的五步 `workflow.vue`，作为默认工作台。
- `/video/projects/:id/advanced` 保留当前 `workbench.vue`，作为高级工作台和完整旧能力逃生口。
- 现有 `/video/projects/:id` 别名继续跳转默认五步工作台。
- 新工作台可跳转高级工作台；高级工作台可返回五步工作台。两者读取相同项目、素材、分镜和版本数据，不复制业务记录。

短片制入口和模板在有可执行后端前完全移除，不展示禁用或“即将上线”选项。

## 方案比较

### 方案 A：引导式任务工作台（采用）

把能力重新映射为 `创作设定 -> 素材准备 -> 分镜脚本 -> 视频生成 -> 成片导出`，每一步渲染独立工作区。高级功能通过抽屉、折叠面板和详情弹窗按需展开。

优点是直接解决流程、认知和信息密度问题，同时可以复用全部现有 API。代价是需要调整工作台模板和少量项目数据结构。

### 方案 B：保留三栏，仅做视觉整理

缩小间距、统一颜色、优化卡片样式，不改变信息架构。

实现成本低，但五步仍是装饰导航，用户仍需自己理解所有能力，不满足本次目标。

### 方案 C：专业剪辑器式自由画布

使用素材栏、预览画布、检查器和时间线，提供最高自由度。

适合专业剪辑，但首次使用成本高、开发量大，也偏离当前以 AI 分镜生成和项目合成为主的产品能力。

## 核心设计原则

- 任务优先：界面标题描述用户要完成的任务，而不是系统模块名。
- 渐进披露：常用字段默认展示，高级参数、版本操作和素材细节按需展开。
- 状态即导航：步骤完成度由真实数据计算，用户随时知道阻塞项。
- 就地恢复：错误出现在对应镜头或操作附近，并提供明确的编辑或重试动作。
- 保留控制权：自动拆分和批量生成都先展示范围，已存在内容时必须确认，不静默覆盖。
- 一个主操作：每个步骤只有一个高强调按钮，其他操作使用普通按钮或文本操作。

## 信息架构

### 制片入口 `/video/production`

入口页由“模式选择”改为“创作中心”：

- 页面首要操作为“新建视频项目”。
- 次要操作为“继续最近项目”，展示最近项目及真实进度。
- 新建操作直接创建当前可用的视频项目，不再出现快速短片/系列项目模式选择。
- 尚未开放的短片制不再出现在入口页、模板或工作台中。

项目列表 `/video/projects` 继续作为完整管理页，不重复承担产品介绍。

### 项目工作台 `/video/projects/:id/workbench`

页面由三个稳定区域组成：

1. **项目栏**：返回、项目名称、保存/同步状态、项目总进度和设置入口。
2. **真实步骤导航**：五步、完成标记、阻塞数量；当前步骤写入 URL 查询参数 `step`，刷新和返回时保留位置。
3. **步骤内容区**：一次只显示当前任务需要的界面。

不再在项目内部显示“项目制/短片制”开关。模式属于创建项目时的选择，不属于项目制作中的频繁操作。

五个步骤键和 URL 值固定为：

| 顺序 | 步骤 | `step` 查询参数 |
| --- | --- | --- |
| 1 | 创作设定 | `brief` |
| 2 | 素材准备 | `assets` |
| 3 | 分镜脚本 | `storyboard` |
| 4 | 视频生成 | `generate` |
| 5 | 成片导出 | `export` |

用户可自由返回已访问步骤。向前点击不被硬禁用，但目标步骤顶部必须展示阻断原因和返回修复操作。未知 `step` 值回退到系统计算的推荐步骤并使用 `router.replace` 修正 URL。

## 五步工作流

### 1. 创作设定

目标是给后续分镜提供稳定上下文。

默认展示：

- 项目名称
- 创作主题
- 项目剧本
- 整体视觉风格

项目剧本新增持久化字段 `scriptContent`。输入区显示字数和预计段落数，支持保存草稿。主题和风格预设保留，但由大段标签改为紧凑选择器。

主操作为“保存并准备素材”。若剧本为空，按钮保持可用但在点击后就地提示需要补充内容。

### 2. 素材准备

目标是准备视频中反复出现的角色与场景，保障一致性。

页面使用两组并列但不嵌套的资源区：角色、场景。每组显示数量、缺图数量和添加操作。资源项以缩略图、名称、用途和状态为主，编辑与删除进入菜单。

全局视频资产库通过“从资产库选择”抽屉接入；上传仍走现有 OSS API。页面明确显示素材会在分镜中复用，但不再展示内部存储说明作为视觉主信息。

主操作为“素材已准备，开始分镜”。角色或场景为空不强制阻塞，因为无人物/纯场景视频同样成立。

### 3. 分镜脚本

目标是把项目剧本变成可生成的镜头清单。

首次进入且没有分镜时，显示两个路径：

- “从剧本创建分镜”：按空行/换行拆分有效段落，先显示将创建的镜头数量；确认后调用现有创建分镜 API，生成可编辑草稿。
- “手动添加分镜”：进入单镜编辑。

已有分镜时使用双区布局：左侧为稳定宽度的紧凑镜头目录，右侧为当前镜头编辑器。目录只显示序号、名称、内容摘要和准备状态，不在每张卡里重复完整表单。

编辑器默认展示分镜原文、动作描述、角色、场景、时长与画幅。参考素材作为独立区，可从资产库选择或上传。生成提示词和模型参数不在此步骤干扰用户。

主操作为“保存并检查生成条件”。切换镜头前保存当前编辑；失败时保留输入并在字段附近显示错误。

### 4. 视频生成

目标是检查条件、提交任务并持续跟踪结果。

顶部生成概览展示四个可点击状态：

- 可生成
- 待完善
- 生成中
- 已完成

主操作文案包含范围，例如“生成 6 个可用镜头”。待完善镜头不会被提交，并可点击直接回到对应分镜修复。

镜头使用生成任务卡，而不是编辑表单：左侧显示脚本摘要和参考素材，中间显示预览/生成状态，右侧仅显示当前版本摘要。模型、分辨率、音画同出和动态提示词放入“高级设置”折叠区。

视频版本改为按镜头打开的抽屉，提供预览、设为当前、备份、重新编辑、复制、抽帧、去字幕、超分和删除。页面主层级不再同时显示所有版本按钮。

批量生成继续使用现有 API 和轮询机制。轮询在离开页面或任务结束时停止；失败卡显示服务返回原因与“重试”或“返回分镜修改”。

### 5. 成片导出

目标是确认镜头完整性并生成最终视频。

页面展示镜头顺序、完成数量、缺失镜头和预计总时长。未完成镜头不会静默忽略；用户可以返回生成步骤，或明确选择“仅合成已完成镜头”。

转场、音乐和字幕作为有限的合成设置。主操作为“合成成片”。合成中显示持续进度，完成后直接显示播放器、视频地址和下载/复制操作；失败时保留设置并提供重试。

## 状态模型

步骤完成度由项目真实数据计算，不以“访问过页面”作为完成证据。统一状态为 `pending | current | complete | optional | skipped_existing | blocked | stale`。

精确谓词：

- `brief complete`：项目名称非空且 `scriptContent.trim()` 非空。
- `brief skipped_existing`：剧本为空，但项目已存在任一角色、场景、分镜、生成版本或成片。
- `assets complete`：至少存在一个角色或场景。
- `assets optional`：没有角色/场景，但用户可以直接创建无人物/纯场景分镜；该状态不阻断后续。
- `assets skipped_existing`：没有角色/场景但已经存在分镜、生成版本或成片。
- `storyboard complete`：至少一个分镜，且至少一个镜头状态为 `ready | generating | completed | stale`。
- `storyboard blocked`：没有分镜，或所有镜头都缺少动作描述。
- `generate complete`：项目内所有未删除镜头都有与当前生成修订一致的成功选中版本。临时部分合成不会改变该步骤完成度。
- `generate stale`：存在已选成功版本，但其 `shotRevision` 落后于当前镜头 `generationRevision`。
- `export complete`：最终视频存在，且 `finalVideoInputHash` 等于当前合成输入哈希。
- `export stale`：最终视频存在但输入哈希不同，旧成片仍可预览，不能显示为当前成片。

推荐初始步骤按最靠后的可继续位置计算：当前成片 -> `export`；有生成中/成功/失败版本 -> `generate`；有分镜 -> `storyboard`；有角色或场景 -> `assets`；否则 -> `brief`。老项目不会因缺少剧本被送回第一步。

### 镜头生成状态契约

纯函数接口：

```ts
type ShotReadiness =
  | { kind: 'incomplete'; canGenerate: false; reason: string }
  | { kind: 'ready'; canGenerate: true; reason: '' }
  | { kind: 'generating'; canGenerate: false; reason: string }
  | { kind: 'recovery'; canGenerate: false; reason: string }
  | { kind: 'completed'; canGenerate: false; reason: '' }
  | { kind: 'stale'; canGenerate: true; reason: string }
  | { kind: 'failed'; canGenerate: true; reason: string };

interface ShotReadinessInput {
  actionDescription: string;
  generationRevision: number;
  latestSubmissionStatus: string;
  linkedTaskStatus: string;
  latestTaskError: string;
  selectedVersion?: {
    generationId: string;
    shotRevision: number;
    status: string;
    videoUrl: string;
  };
}
```

`latestSubmissionStatus` 来自付费提交状态机，`linkedTaskStatus/latestTaskError` 来自已关联 generation 的网关任务；`selectedVersion` 只来自 `selected_generation_id`。三者是独立关系，开始或失败一次新提交不能改变已选版本。

判定优先级从高到低：

| 条件 | 输出 | 批量生成 |
| --- | --- | --- |
| 提交为 `unknown_outcome` | `recovery` | 排除，只允许对账 |
| 提交为 `prepared/submitting/accepted/reconciled` | `generating` | 排除 |
| 已关联任务为 `queued/pending/generating/processing` | `generating` | 排除 |
| 动作描述为空 | `incomplete` | 排除 |
| 选中版本成功、有 URL 且版本修订等于当前修订 | `completed` | 排除 |
| 选中版本成功但版本修订落后 | `stale` | 默认包含，提交前明确提示会产生新版本 |
| 最新任务失败且没有当前有效选中版本 | `failed` | 默认包含重试 |
| 其他情况 | `ready` | 包含 |

提交活动/恢复状态必须在选片和失败判断之前求值，任何活动锁存在时都不能再次生成。若终态失败任务之外仍有当前有效的选中版本，镜头保持 `completed` 并在版本抽屉显示最近一次尝试失败，不把可用结果降级。批量生成范围严格等于 `canGenerate=true` 的镜头 ID 列表，即 `ready + stale + failed`，服务端再次使用相同规则校验，不能只信任前端。主操作文案按范围显示“生成 N 个可用/需更新/可重试镜头”，提交确认中分组列明三类数量。

### 生成与合成过期

- `video_shots.generation_revision` 初始为 0。动作描述、动态描述、角色、场景、时长、画幅、模型、分辨率、音画模式、参考素材顺序或项目风格发生变化时递增。
- 每条 `video_generations` 保存提交时的 `shot_revision`。新增 `video_shots.selected_generation_id` 作为显式选中版本；现有 `generation_id` 仅表示最近一次关联任务并保留旧接口兼容，不能再用于判断当前选片。
- 老镜头和老版本修订默认都为 0，因此在未编辑前保持有效；第一次相关编辑后旧版本自然变为 `stale`。
- 老数据迁移：`generation_id` 指向成功且有 URL 的生成记录时回填 `selected_generation_id`；指向 queued/processing/failed 或不存在记录时不回填。迁移不把失败/活动任务误选为当前版本。
- 默认合成参与者为项目内按 `order_num,id` 排序的全部未删除镜头。若用户明确选择“仅合成已完成镜头”，本次任务参与者为提交确认框列出的有效选片镜头；未参与镜头仍在 `excludedShotIds` 中持久化。
- 合成时对有序的 `shotId:selectedGenerationId:shotRevision`、转场、字幕和音乐 URL 计算 SHA-256，写入合成任务 `compose_input_hash`，成功后同步到项目 `final_video_input_hash`。
- 每个合成任务同时保存 `compose_input_snapshot JSONB`，结构为 `{ includedShots: [{ shotId, generationId, shotRevision, orderNum }], excludedShotIds, transition, enableSubtitles, musicUrl, partialAcknowledged }`。状态页和历史详情从快照展示当次参与/排除清单，不尝试从哈希反推。
- 镜头顺序、选中版本、镜头修订或合成设置变化后，旧成片不删除，只标记为 `stale`。
- “仅合成已完成镜头”必须在提交确认框中列出包含/排除镜头，并将排除列表纳入合成输入哈希；该确认只对本次提交有效。

## 数据与接口变化

### 项目剧本

为 `video_projects` 增加 `script_content TEXT NOT NULL DEFAULT ''`，并在 schema 中使用 `ALTER TABLE ... ADD COLUMN IF NOT EXISTS` 兼容现有数据库。

以下契约增加 `scriptContent`：

- Go `videoproject.Project`
- Go `videoproject.ProjectInput`
- 项目创建、查询、列表和更新 SQL
- TypeScript `Project`
- 项目创建/更新表单

项目剧本每次内容变化递增 `script_revision`，用于生成稳定的分镜来源键。

### 生成和合成修订字段

新增兼容字段：

- `video_projects.script_revision INT NOT NULL DEFAULT 0`
- `video_projects.final_video_input_hash TEXT NOT NULL DEFAULT ''`
- `video_shots.generation_revision INT NOT NULL DEFAULT 0`
- `video_shots.selected_generation_id BIGINT REFERENCES video_generations(id) ON DELETE SET NULL`
- `video_shots.source_key TEXT NOT NULL DEFAULT ''`
- `video_shots.source_script_revision INT NOT NULL DEFAULT 0`
- `video_generations.shot_revision INT NOT NULL DEFAULT 0`
- `video_compose_jobs.compose_input_hash TEXT NOT NULL DEFAULT ''`
- `video_compose_jobs.compose_input_snapshot JSONB NOT NULL DEFAULT '{}'::jsonb`

为 `(project_id, source_key)` 建立 `source_key <> ''` 的唯一索引，支持剧本分镜幂等创建。

### 剧本拆分

前端纯函数 `splitScriptIntoShots(scriptContent)`：

- 将连续空行视为优先分段；没有空行时按非空行分段。
- 去除首尾空白和空段。
- 不执行语义改写，不推断角色或场景。
- 返回可预览的段落数组。

确认后调用单一批量接口 `POST /video/projects-shots/from-script/:projectId`，请求包含 `scriptRevision` 和段落列表。每段来源键为 `SHA-256(projectId + ':' + scriptRevision + ':' + zeroBasedIndex + ':' + normalizedParagraph)`。

服务端在事务内逐项以来源键幂等创建，但允许单项校验失败，返回：

```ts
interface CreateShotsFromScriptResult {
  items: Array<{
    index: number;
    sourceKey: string;
    status: 'created' | 'existing' | 'failed';
    shot?: Shot;
    error?: string;
  }>;
}
```

网络结果不确定时，客户端使用同一请求重试；`existing` 项不会重复创建。部分失败后页面始终显示“剧本导入结果”面板，失败项可单独重试，成功项可直接编辑。该面板可由当前剧本重新计算全部来源键，并与现有分镜 `sourceKey` 对照重建，无需依赖浏览器内存。修改剧本会递增修订并产生新的来源键；系统不会自动删除旧分镜，导入前明确提示新增数量。

### 未保存编辑契约

- 工作台容器拥有 `dirtyScope`，值为 `brief | shot:<id> | compose | null`；子组件只发出 `dirty-change` 和 `save` 事件。
- 切换镜头、步骤、进入高级工作台或浏览器路由离开时，若存在 dirty 状态，弹出“保存并继续 / 放弃更改 / 取消”。
- “保存并继续”失败时停留在原步骤和原镜头，保留输入、聚焦第一个错误字段，不执行原导航动作。
- “放弃更改”从最近一次服务端数据恢复表单，再执行目标动作。
- “取消”关闭提示并保持当前焦点与选择。
- 打开版本抽屉或素材抽屉不触发离开；若抽屉动作会改写当前表单引用，先走同一保护流程。
- 浏览器刷新/关闭使用 `beforeunload` 原生提示；成功保存后立即清除 dirty 状态。

## 前端组件边界

现有页面保留数据编排职责，逐步提取以下可独立理解的组件/纯逻辑：

- `ProductionStepper.vue`：步骤展示、完成/阻塞状态和选择事件。
- `ProjectBriefPanel.vue`：创作设定表单，不直接请求 API。
- `AssetPreparationPanel.vue`：角色/场景列表与用户操作事件。
- `ShotNavigator.vue`：镜头目录与选择事件。
- `ShotEditorPanel.vue`：当前镜头基础编辑和参考素材。
- `GenerationOverview.vue`：镜头状态汇总、筛选与批量操作事件。
- `ShotVersionDrawer.vue`：单镜版本列表和版本动作事件。
- `ComposePanel.vue`：成片检查、设置、进度和结果。
- `workflow.ts`：剧本拆分、镜头准备状态、步骤完成度等纯函数。

API 请求、轮询 timer、项目级弹窗和跨步骤跳转先保留在工作台容器中，避免在本次改造中同时迁移所有业务状态。组件通过明确 props/emits 交互，不直接依赖路由或全局 store。

默认入口新建 `projects/workflow.vue` 作为容器，当前 `projects/workbench.vue` 不在本次重写，改挂到高级路由。核心接口约束：

- 步骤组件接收领域数据和 `busy/errors`，只发 `save/next/back/dirty-change`。
- `ShotNavigator` 接收轻量镜头状态数组，发 `select/create/reorder`，不持有镜头表单。
- `GenerationOverview` 接收已经由 `workflow.ts` 计算的状态，不自行判断付费提交范围。
- `ShotVersionDrawer` 发版本动作，容器调用现有 API 后重新加载镜头和版本。
- `ComposePanel` 接收服务端返回的当前性、包含/排除清单和合成状态，不在前端猜测成片是否过期。
- 所有 timer、路由同步、dirty 导航队列和 API 请求归容器/composable 管理，组件卸载不遗留后台轮询。

## 视觉规范

整体采用克制的浅色创作台，而不是深色流程横幅与浅色后台混搭：

- 页面背景：冷中性浅灰；内容面：白色；视频预览区可使用深灰。
- 主色：现有品牌蓝，用于唯一主操作、当前步骤和键盘焦点。
- 辅助状态：薄荷绿表示完成，琥珀色表示待处理，红色表示失败；状态始终配文字/图标。
- 卡片圆角不超过 8px；不使用装饰性渐变、光晕和大面积阴影。
- 采用 4/8px 间距体系；正文不小于 14px，输入控件和主要点击区不低于 44px。
- 图标统一使用项目现有 Ant Design/Lucide 图标，不使用 emoji。
- 动效仅用于步骤切换、抽屉和状态反馈，时长 150-250ms，并尊重 `prefers-reduced-motion`。

## 响应式与可访问性

- 1440px 及以上：镜头目录与编辑器双区展示。
- 768-1439px：目录缩窄，抽屉覆盖式展示高级内容。
- 小于 768px：步骤导航横向可滚动但页面不横向滚动；镜头目录变为下拉选择；底部固定当前步骤主操作并预留安全间距。
- 所有图标按钮有 `aria-label`，步骤使用 `aria-current="step"`，异步状态使用 `aria-live="polite"`。
- 键盘焦点顺序与视觉顺序一致；抽屉关闭后焦点返回触发按钮。
- 错误不只用颜色表达，字段错误紧邻字段，任务错误紧邻对应镜头。

## 错误处理与恢复

- 项目/分镜保存失败：保留本地输入，显示原因和重试，不自动关闭编辑状态。
- 批量创建分镜部分失败：显示成功/失败数量及失败段落，可继续重试失败部分。
- 素材上传失败：在素材区域显示失败文件与重新上传，不只弹全局消息。
- 生成提交失败：镜头卡进入失败状态，保留参数和参考素材。
- 生成轮询超时：显示“仍在处理中，可手动刷新”，不把超时误报为生成失败。
- 合成条件不足：列出具体未完成镜头并提供跳转。
- 页面卸载：清理所有生成与合成轮询 timer。

## 测试设计

### 纯逻辑测试

- 剧本按空行和换行拆分，忽略空段。
- 镜头准备状态覆盖草稿、缺描述、生成中、完成和失败。
- 步骤完成度与下一建议步骤计算正确。
- 批量生成范围等于 `canGenerate=true` 的 `ready + stale + failed` 镜头，并排除所有活动提交与 `unknown_outcome`。
- 镜头修订变化把已选版本变为 `stale`，新版本选择恢复 `completed`。
- 合成输入哈希覆盖镜头顺序、版本、修订和设置。
- 老项目推荐步骤覆盖仅素材、已有分镜、已有生成和已有成片四种形态。
- 剧本分镜来源键稳定，重复请求返回 `existing`。

### 前端组件/源码测试

- 步骤切换真实控制五个内容面板。
- 工作台不再出现项目制/短片制开关和固定三栏结构。
- 高级生成参数默认折叠。
- 版本通过抽屉展示。
- 主操作文案包含当前可执行范围。
- 375px 下无页面级横向滚动，主要触控目标不小于 44px。

### 后端测试

- 项目创建、更新和读取持久化 `scriptContent`。
- 旧数据库迁移能添加 `script_content` 且默认值为空。
- 原有项目、分镜、生成和合成接口回归通过。
- 生成相关编辑递增镜头修订，非相关显示字段不会误增。
- 角色/场景/参考素材变化只使受影响镜头过期。
- 合成输入变化使旧成片过期但不删除 URL。
- 批量剧本建镜头对重复请求和部分失败可恢复。

### 端到端验收

1. 创建项目并填写剧本。
2. 从剧本生成分镜草稿并编辑其中一个镜头。
3. 添加角色/场景和分镜参考素材。
4. 查看生成前检查，只生成准备完成的镜头。
5. 观察生成状态自动刷新并打开版本抽屉。
6. 合成已完成镜头并预览最终视频。
7. 在桌面 1440px、平板 768px 和手机 375px 截图检查无重叠或横向溢出。
8. 打开缺少剧本但已有生成版本的老项目，默认定位生成步骤且历史结果可用。
9. 模拟部分建镜头、轮询超时、生成失败、旧版本过期、合成失败和未保存导航，均可恢复且不重复付费提交。

## 迁移顺序

1. 增加项目剧本字段和纯工作流逻辑。
2. 建立新的工作台壳层和真实步骤路由状态。
3. 迁移创作设定、素材准备和分镜脚本。
4. 迁移视频生成、版本抽屉和批量状态。
5. 迁移成片导出并移除旧三栏/模式占位。
6. 完成响应式、可访问性、回归测试和浏览器截图验证。

该顺序保证每一步都复用现有可运行能力，同时逐步替换旧信息架构，避免同时重写业务 API 和页面交互。
