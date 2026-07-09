# Production Workbench B Implementation Plan

> **For agentic workers:** REQUIRED: Use superpowers:subagent-driven-development (if subagents available) or superpowers:executing-plans to implement this plan. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Add the confirmed B-scope production workbench migration: a production mode entry, project-mode routing to existing video projects, a short-mode placeholder, and a five-step production flow inside the existing project workbench.

**Architecture:** Keep the target project's Vue 3 + Ant Design Vue/Vben structure. Do not copy Element Plus pages from `dd-liuguang-web`; migrate the product flow semantics onto existing video project, asset, storyboard, generation, and compose APIs. Add isolated route/page tests and source-level workflow tests before production code.

**Tech Stack:** Vue 3 SFC, Vue Router route modules, Ant Design Vue, Vben layout, Vitest source tests, pnpm workspace.

---

## File Structure

- Modify: `nx-backend/apps/web-antd/src/router/routes/modules/video.ts`
  - Add menu-visible `/video/production` and hidden `/video/production/short` child routes.
  - Keep existing `/video/projects` and workbench routes intact.

- Create: `nx-backend/apps/web-antd/src/views/video/production/index.vue`
  - Production mode landing page with Project Mode and Short Mode cards.
  - Project Mode CTA routes to `/video/projects`; Short Mode CTA routes to `/video/production/short`.

- Create: `nx-backend/apps/web-antd/src/views/video/production/short.vue`
  - Short-mode placeholder page with future roadmap and buttons back to production entry/project mode.

- Create: `nx-backend/apps/web-antd/src/views/video/production.test.ts`
  - Source-level tests for routes, backend menu seed, and new page copy/links.

- Modify: `nx-backend/apps/server/internal/db/db.go`
  - Add backend access-mode menu seed entries for `/video/production` and `/video/production/short`.
  - Emit `activePath` metadata for hidden detail routes so Vben menu highlighting works.

- Modify: `nx-backend/apps/web-antd/src/views/video/projects/workbench.vue`
  - Add production step navigation and contextual panels for script, analysis, assets, storyboard, compose.
  - Reuse existing workbench functions for asset creation, shot creation, batch generation, and compose.
  - Keep existing three-column layout and dialogs.

- Create: `nx-backend/apps/web-antd/src/views/video/project-workbench-production-flow.test.ts`
  - Source-level tests verifying five steps, OSS file bucket messaging, and bridge actions exist.

- Existing note: `nx-backend/apps/web-antd/src/views/video/workbench-redesign.test.ts` may be a previous-task residual if present. Do not delete it unless it contradicts the current B scope; if present and failing because of current UI changes, either satisfy it or document it as unrelated.

## Chunk 1: Production Mode Routing and Placeholder Pages

### Task 1: Add route/page tests for production mode

**Files:**
- Create: `nx-backend/apps/web-antd/src/views/video/production.test.ts`
- Read: `nx-backend/apps/web-antd/src/router/routes/modules/video.ts`

- [ ] **Step 1: Write the failing route/page source test**

Create `nx-backend/apps/web-antd/src/views/video/production.test.ts`:

```ts
import { readFileSync } from 'node:fs';
import { dirname, resolve } from 'node:path';
import { fileURLToPath } from 'node:url';

import { describe, expect, it } from 'vitest';

const webRoot = resolve(dirname(fileURLToPath(import.meta.url)), '../../..');
const repoRoot = resolve(webRoot, '../..');
const readWeb = (path: string) => readFileSync(resolve(webRoot, path), 'utf8');
const readRepo = (path: string) => readFileSync(resolve(repoRoot, path), 'utf8');

describe('production workbench mode entry', () => {
  it('registers production mode and short mode routes under /video', () => {
    const routes = readWeb('src/router/routes/modules/video.ts');

    expect(routes).toContain("path: 'production'");
    expect(routes).toContain("name: 'VideoProduction'");
    expect(routes).toContain("#/views/video/production/index.vue");
    expect(routes).toContain("path: 'production/short'");
    expect(routes).toContain("name: 'VideoProductionShort'");
    expect(routes).toContain("#/views/video/production/short.vue");
    expect(routes).toContain("activePath: '/video/production'");
  });

  it('offers project mode and short mode choices with stable target paths', () => {
    const page = readWeb('src/views/video/production/index.vue');

    expect(page).toContain('制片工作台');
    expect(page).toContain('项目制');
    expect(page).toContain('短片制');
    expect(page).toContain('/video/projects');
    expect(page).toContain('/video/production/short');
  });

  it('seeds backend-access menus for production mode', () => {
    const db = readRepo('apps/server/internal/db/db.go');

    expect(db).toContain('VideoProduction');
    expect(db).toContain('/video/production');
    expect(db).toContain('/video/production/index');
    expect(db).toContain('VideoProductionShort');
    expect(db).toContain('/video/production/short');
    expect(db).toContain('ActivePath: "/video/production"');
    expect(db).toContain('metaMap["activePath"]');
  });

  it('keeps short mode as a clear placeholder with project-mode escape route', () => {
    const page = readWeb('src/views/video/production/short.vue');

    expect(page).toContain('短片制工作台');
    expect(page).toContain('功能规划中');
    expect(page).toContain('脚本输入');
    expect(page).toContain('快速分镜');
    expect(page).toContain('/video/production');
    expect(page).toContain('/video/projects');
  });
});
```

- [ ] **Step 2: Run test to verify RED**

Run:

```bash
cd /Users/wohenzaiyi/Desktop/nine-xing/nx-backend
PATH=/opt/homebrew/bin:/usr/local/bin:$PATH pnpm exec vitest run apps/web-antd/src/views/video/production.test.ts
```

Expected: FAIL because `production.test.ts` references routes/pages that do not exist yet.

- [ ] **Step 3: Implement minimal routes, backend menu seed, and pages**

Modify `nx-backend/apps/web-antd/src/router/routes/modules/video.ts` inside the `/video` children array, before `projects`:

```ts
      {
        name: 'VideoProduction',
        path: 'production',
        component: () => import('#/views/video/production/index.vue'),
        meta: {
          icon: 'lucide:clapperboard',
          title: '制片工作台',
        },
      },
      {
        name: 'VideoProductionShort',
        path: 'production/short',
        component: () => import('#/views/video/production/short.vue'),
        meta: {
          hideInMenu: true,
          title: '短片制工作台',
          activePath: '/video/production',
        },
      },
```

Modify `nx-backend/apps/server/internal/db/db.go` so backend access mode can see the new menu:

- Add `ActivePath string` to `seedMenu`.
- When building `metaMap`, write `metaMap["activePath"] = ...`; optionally keep `activeMenu` for compatibility.
- Insert menus under `VideoCenter`:

```go
	{ID: 1006, PID: 1000, Name: "VideoProduction", Path: "/video/production", Component: "/video/production/index", AuthCode: "Video:Project:Manage", Type: "menu", Sort: 6, Icon: "lucide:clapperboard", Title: "制片工作台"},
	{ID: 1007, PID: 1000, Name: "VideoProjects", Path: "/video/projects", Component: "/video/projects", AuthCode: "Video:Project:Manage", Type: "menu", Sort: 7, Icon: "lucide:folder-kanban", Title: "项目列表"},
	{ID: 1008, PID: 1000, Name: "VideoProductionShort", Path: "/video/production/short", Component: "/video/production/short", AuthCode: "Video:Project:Manage", Type: "menu", Sort: 8, Icon: "lucide:badge-play", Title: "短片制工作台", HideInMenu: true, ActivePath: "/video/production"},
	{ID: 1009, PID: 1000, Name: "VideoProjectWorkbench", Path: "/video/projects/:id/workbench", Component: "/video/projects/workbench", AuthCode: "Video:Project:Manage", Type: "menu", Sort: 9, Icon: "lucide:panel-top", Title: "项目工作台详情", HideInMenu: true, ActivePath: "/video/projects"},
```

Create `nx-backend/apps/web-antd/src/views/video/production/index.vue` using Ant Design Vue cards and `useRouter`:

```vue
<script setup lang="ts">
import { useRouter } from 'vue-router';

const router = useRouter();

function goProjectMode() {
  router.push('/video/projects');
}

function goShortMode() {
  router.push('/video/production/short');
}
</script>

<template>
  <div class="production-page">
    <section class="production-hero">
      <a-tag color="blue">Production Workspace</a-tag>
      <h1>制片工作台</h1>
      <p>
        将剧本、资产、分镜、生成与剪辑合成串成完整生产流。当前先支持项目制完整链路，短片制保留独立入口。
      </p>
    </section>

    <div class="mode-grid">
      <a-card class="mode-card mode-card-primary" hoverable @click="goProjectMode">
        <template #title>项目制</template>
        <template #extra><a-tag color="green">已接入</a-tag></template>
        <p>适合连续剧、系列视频、动画项目等完整制作流程。</p>
        <ul>
          <li>项目管理与项目配置</li>
          <li>资产统一上传到阿里云 OSS 文件桶</li>
          <li>分镜设计、批量生成与成片合成</li>
        </ul>
        <a-button type="primary" block @click.stop="goProjectMode">进入项目制</a-button>
      </a-card>

      <a-card class="mode-card" hoverable @click="goShortMode">
        <template #title>短片制</template>
        <template #extra><a-tag>占位</a-tag></template>
        <p>面向单条短片快速制作，后续会提供更轻量的一站式流程。</p>
        <ul>
          <li>脚本输入</li>
          <li>快速分镜</li>
          <li>一键生成与导出</li>
        </ul>
        <a-button block @click.stop="goShortMode">查看短片制规划</a-button>
      </a-card>
    </div>
  </div>
</template>
```

Add scoped styles with light dashboard surfaces, 4/8px spacing rhythm, no fixed viewport height that blocks scrolling.

Create `nx-backend/apps/web-antd/src/views/video/production/short.vue` with placeholder copy and two CTA buttons:

```vue
<script setup lang="ts">
import { useRouter } from 'vue-router';

const router = useRouter();
</script>

<template>
  <div class="short-mode-page">
    <a-result title="短片制工作台" sub-title="功能规划中：后续用于单条短片快速制作，不走完整项目制流程。">
      <template #extra>
        <a-space wrap>
          <a-button @click="router.push('/video/production')">返回制片工作台</a-button>
          <a-button type="primary" @click="router.push('/video/projects')">先使用项目制</a-button>
        </a-space>
      </template>
    </a-result>

    <a-card title="短片制规划" class="roadmap-card">
      <a-steps direction="vertical" :current="0" status="process">
        <a-step title="脚本输入" description="输入单条短片脚本或创意梗概。" />
        <a-step title="资产选择" description="从人物、物品、场景、风格资产中快速选择。" />
        <a-step title="快速分镜" description="自动拆分 3-8 个关键镜头。" />
        <a-step title="一键生成" description="生成视频并合成为可导出的短片。" />
      </a-steps>
    </a-card>
  </div>
</template>
```

- [ ] **Step 4: Run test to verify GREEN**

Run the same vitest command. Expected: PASS.

- [ ] **Step 5: Self-review**

Confirm:
- No emoji structural icons.
- Click targets are buttons/cards with visible hover.
- No horizontal scroll.
- New pages use `/video/projects`, not source project routes.

## Chunk 2: Project Workbench Five-Step Production Flow

### Task 2: Add tests for five-step flow in existing workbench

**Files:**
- Create: `nx-backend/apps/web-antd/src/views/video/project-workbench-production-flow.test.ts`
- Modify: `nx-backend/apps/web-antd/src/views/video/projects/workbench.vue`

- [ ] **Step 1: Write the failing source test**

Create `nx-backend/apps/web-antd/src/views/video/project-workbench-production-flow.test.ts`:

```ts
import { readFileSync } from 'node:fs';
import { dirname, resolve } from 'node:path';
import { fileURLToPath } from 'node:url';

import { describe, expect, it } from 'vitest';

const root = resolve(dirname(fileURLToPath(import.meta.url)), '../../..');
const workbench = () =>
  readFileSync(resolve(root, 'src/views/video/projects/workbench.vue'), 'utf8');

describe('project workbench production flow migration', () => {
  it('exposes the five migrated production steps', () => {
    const source = workbench();

    for (const label of ['剧本录入', '资产分析', '创建资产', '分镜设计', '剪辑合成']) {
      expect(source).toContain(label);
    }

    expect(source).toContain('productionSteps');
    expect(source).toContain('activeProductionStep');
  });

  it('bridges each step to current project-mode capabilities', () => {
    const source = workbench();

    expect(source).toContain('scriptDraft');
    expect(source).toContain('从剧本拆解资产和分镜');
    expect(source).toContain('角色数');
    expect(source).toContain('场景数');
    expect(source).toContain('分镜数');
    expect(source).toContain('showAddCharacter');
    expect(source).toContain('showAddScene');
    expect(source).toContain('showAddShot');
    expect(source).toContain('generateAllShots');
    expect(source).toContain('composeVideo');
  });

  it('makes the Aliyun bucket asset requirement visible in asset creation', () => {
    const source = workbench();

    expect(source).toContain('资产统一上传到阿里云 OSS 文件桶');
    expect(source).toContain('人物');
    expect(source).toContain('物品');
    expect(source).toContain('场景');
    expect(source).toContain('音频');
    expect(source).toContain('视频');
  });
});
```

- [ ] **Step 2: Run test to verify RED**

Run:

```bash
cd /Users/wohenzaiyi/Desktop/nine-xing/nx-backend
PATH=/opt/homebrew/bin:/usr/local/bin:$PATH pnpm exec vitest run apps/web-antd/src/views/video/project-workbench-production-flow.test.ts
```

Expected: FAIL because workbench does not contain the five-step flow state/panels yet.

- [ ] **Step 3: Add minimal state and computed progress**

In `workbench.vue` script setup:

```ts
const activeProductionStep = ref('script');
const scriptDraft = ref('');

const productionSteps = [
  { key: 'script', title: '剧本录入', description: '录入项目剧本，准备拆解资产和分镜' },
  { key: 'analysis', title: '资产分析', description: '分析人物、场景、物品和镜头需求' },
  { key: 'assets', title: '创建资产', description: '生成或上传人物、场景、物品、音频、视频资产' },
  { key: 'storyboard', title: '分镜设计', description: '设计分镜并绑定参考资产' },
  { key: 'compose', title: '剪辑合成', description: '批量生成镜头并合成为成片' },
];

const currentProductionStepIndex = computed(() =>
  productionSteps.findIndex((step) => step.key === activeProductionStep.value),
);

const completedShotCount = computed(
  () => shots.value.filter((shot) => shot.status === 'completed' && shot.videoUrl).length,
);
```

- [ ] **Step 4: Add five-step UI above the existing three-column layout**

In the template, between header and `<a-layout class="workbench-layout">`, add:

```vue
    <section class="production-flow-panel">
      <div class="flow-heading">
        <div>
          <a-tag color="blue">项目制生产流</a-tag>
          <h2>制片工作台流程</h2>
          <p>从剧本录入到资产分析、创建资产、分镜设计和剪辑合成，逐步完成项目制视频生产。</p>
        </div>
        <a-space wrap>
          <a-button @click="activeProductionStep = 'assets'">管理资产</a-button>
          <a-button type="primary" @click="activeProductionStep = 'storyboard'">进入分镜设计</a-button>
        </a-space>
      </div>

      <a-steps
        class="production-steps"
        :current="Math.max(currentProductionStepIndex, 0)"
        responsive
        @change="(index: number) => (activeProductionStep = productionSteps[index]?.key || 'script')"
      >
        <a-step
          v-for="step in productionSteps"
          :key="step.key"
          :title="step.title"
          :description="step.description"
        />
      </a-steps>

      <div class="step-body">
        <a-card v-if="activeProductionStep === 'script'" title="剧本录入" size="small">
          <a-textarea
            v-model:value="scriptDraft"
            :rows="5"
            placeholder="粘贴项目剧本、分场描述或创意梗概。第一阶段先保存在当前页面状态，后续可接入剧本文档持久化。"
          />
          <a-alert
            class="step-alert"
            type="info"
            show-icon
            message="从剧本拆解资产和分镜"
            description="源项目会在这里触发剧本拆解、资产提取和分镜生成；当前先保留入口，并复用下方资产库与分镜列表继续制作。"
          />
        </a-card>

        <a-card v-else-if="activeProductionStep === 'analysis'" title="资产分析" size="small">
          <a-row :gutter="12">
            <a-col :xs="24" :md="8"><a-statistic title="角色数" :value="characters.length" /></a-col>
            <a-col :xs="24" :md="8"><a-statistic title="场景数" :value="scenes.length" /></a-col>
            <a-col :xs="24" :md="8"><a-statistic title="分镜数" :value="shots.length" /></a-col>
          </a-row>
          <a-alert class="step-alert" type="success" show-icon message="分析人物、场景、物品、画面需求" description="先用现有项目资产和分镜数据承接分析结果，后续可接入自动资产分析 Agent。" />
        </a-card>

        <a-card v-else-if="activeProductionStep === 'assets'" title="创建资产" size="small">
          <p class="step-copy">人物、物品、场景、音频、视频、风格等资产统一上传到阿里云 OSS 文件桶，生成和上传后可绑定到分镜。</p>
          <a-space wrap>
            <a-button type="primary" @click="showAddCharacter">添加人物</a-button>
            <a-button @click="showAddScene">添加场景</a-button>
            <a-button @click="leftTab = 'characters'">查看人物资产</a-button>
            <a-button @click="leftTab = 'scenes'">查看场景资产</a-button>
          </a-space>
        </a-card>

        <a-card v-else-if="activeProductionStep === 'storyboard'" title="分镜设计" size="small">
          <p class="step-copy">在下方分镜列表中新建、编辑、绑定资产，并执行单镜头生成或批量生成。</p>
          <a-space wrap>
            <a-button type="primary" @click="showAddShot">添加分镜</a-button>
            <a-button :loading="generating" @click="generateAllShots">批量生成</a-button>
          </a-space>
        </a-card>

        <a-card v-else title="剪辑合成" size="small">
          <a-row :gutter="12">
            <a-col :xs="24" :md="8"><a-statistic title="已完成分镜" :value="completedShotCount" /></a-col>
            <a-col :xs="24" :md="8"><a-statistic title="总分镜" :value="shots.length" /></a-col>
            <a-col :xs="24" :md="8"><a-statistic title="合成进度" :value="composeProgress" suffix="%" /></a-col>
          </a-row>
          <a-space class="step-actions" wrap>
            <a-button type="primary" :loading="composing" :disabled="generating" @click="composeVideo">剪辑合成</a-button>
            <a-button @click="activeProductionStep = 'storyboard'">返回分镜设计</a-button>
          </a-space>
        </a-card>
      </div>
    </section>
```

- [ ] **Step 5: Add scoped CSS for flow panel**

Add styles:

```less
.production-flow-panel {
  margin: 16px 24px 0;
  padding: 16px;
  background: #fff;
  border: 1px solid #eef0f4;
  border-radius: 12px;
  box-shadow: 0 8px 24px rgba(15, 23, 42, 0.06);
}

.flow-heading {
  display: flex;
  align-items: flex-start;
  justify-content: space-between;
  gap: 16px;
  margin-bottom: 16px;

  h2 {
    margin: 8px 0 4px;
    font-size: 20px;
    font-weight: 600;
  }

  p {
    margin: 0;
    color: #64748b;
  }
}

.production-steps {
  margin-bottom: 16px;
}

.step-body {
  .ant-card {
    background: #fbfdff;
  }
}

.step-alert {
  margin-top: 12px;
}

.step-copy {
  margin-bottom: 12px;
  color: #475569;
  line-height: 1.6;
}

.step-actions {
  margin-top: 12px;
}

@media (max-width: 900px) {
  .workbench-container {
    height: auto;
    min-height: 100%;
  }

  .workbench-header,
  .flow-heading,
  .shots-header {
    flex-direction: column;
    align-items: stretch;
  }

  .production-flow-panel {
    margin: 12px;
  }

  .workbench-layout {
    overflow: visible;
  }
}
```

If existing `height: 100%`/`overflow: hidden` causes user-reported no-scroll behavior on small screens, keep desktop behavior but allow mobile/medium screens to scroll using the media override above.

- [ ] **Step 6: Run test to verify GREEN**

Run:

```bash
cd /Users/wohenzaiyi/Desktop/nine-xing/nx-backend
PATH=/opt/homebrew/bin:/usr/local/bin:$PATH pnpm exec vitest run apps/web-antd/src/views/video/project-workbench-production-flow.test.ts
```

Expected: PASS.

- [ ] **Step 7: Self-review**

Confirm:
- Existing asset, shot, generation, compose functions still compile.
- Step navigation does not remove existing sidebars/dialogs.
- Aliyun OSS asset copy is visible.
- Responsive override restores page scroll instead of trapping the user.

## Chunk 3: Integration Verification

### Task 3: Run focused and broader checks

**Files:**
- No production edits expected except fixes discovered by tests.

- [ ] **Step 1: Run focused tests**

```bash
cd /Users/wohenzaiyi/Desktop/nine-xing/nx-backend
PATH=/opt/homebrew/bin:/usr/local/bin:$PATH pnpm exec vitest run \
  apps/web-antd/src/views/video/production.test.ts \
  apps/web-antd/src/views/video/project-workbench-production-flow.test.ts
```

Expected: PASS.

- [ ] **Step 2: Run existing related tests**

```bash
cd /Users/wohenzaiyi/Desktop/nine-xing/nx-backend
PATH=/opt/homebrew/bin:/usr/local/bin:$PATH pnpm exec vitest run \
  apps/web-antd/src/views/video/asset-picker.test.ts \
  apps/web-antd/src/views/video/asset-preview.test.ts \
  apps/web-antd/src/views/video/generation-error.test.ts
```

Expected: PASS.

- [ ] **Step 3: Run build**

```bash
cd /Users/wohenzaiyi/Desktop/nine-xing/nx-backend
PATH=/opt/homebrew/bin:/usr/local/bin:$PATH pnpm -F @vben/web-antd run build
```

Expected: PASS. If build fails, fix current-scope issues.

- [ ] **Step 4: Typecheck if practical**

```bash
cd /Users/wohenzaiyi/Desktop/nine-xing/nx-backend
PATH=/opt/homebrew/bin:/usr/local/bin:$PATH pnpm -F @vben/web-antd run typecheck
```

Expected: It may fail on a known unrelated `src/views/settings/model.vue: provider missing` issue. If so, record it separately and ensure no new production-workbench errors appear.

- [ ] **Step 5: Final status review**

```bash
cd /Users/wohenzaiyi/Desktop/nine-xing
git status --short
```

Expected: Only planned files are changed/created.
