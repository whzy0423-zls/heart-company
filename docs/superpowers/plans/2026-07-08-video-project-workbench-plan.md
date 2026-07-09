# Video Project Workbench Implementation Plan

> **For agentic workers:** REQUIRED: Use superpowers:subagent-driven-development (if subagents available) or superpowers:executing-plans to implement this plan. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Turn the `docs/video*.md` design into a working video project workflow: project list, project workbench, character/scene/shot CRUD, prompt preview, shot generation, batch generation, and FFmpeg composition.

**Architecture:** Keep the backend `internal/videoproject` package as the domain boundary for project entities, prompt building, batch generation, frame extraction, and composition. Keep the frontend API contract in `src/api/core/videoproject.ts` as the single source of TypeScript truth, and make both workbench pages consume the backend's camelCase JSON contract. Prioritize a reliable MVP closure before optional transition/music/subtitle polish.

**Tech Stack:** Go `net/http` + PostgreSQL schema in `internal/db/schema.sql`, existing `video.Store` generation API, FFmpeg/ffprobe for frame extraction and composition, Vue 3 + TypeScript + Ant Design Vue, Vben backend-access dynamic routes.

---

## Current Evidence and Root Cause

- `docs/video-generation-redesign.md` requires project-based workflow: `/video/projects`, `/video/projects/:id`, project/character/scene/shot CRUD, prompt builder, single-shot generation, batch generation.
- `docs/video-workbench-design.md` requires a workbench integrating assets, prompt preview, frame inheritance, batch monitoring.
- `docs/video-composition-solution.md` requires FFmpeg server-side composition with status/progress and final video URL.
- Backend smoke check: `PATH=/opt/homebrew/bin:/usr/local/go/bin:/usr/local/bin:$PATH go test ./internal/videoproject ./internal/server` passes.
- Frontend typecheck currently fails with many errors. Root cause: `src/api/core/videoproject.ts` still defines old snake_case models (`project_id`, `video_url`, `batch_id`, `compose_id`) while backend returns camelCase (`projectId`, `videoUrl`, `totalShots`, `shotResults`, `finalVideoUrl`). Workbench templates are mostly written for camelCase, so the API types are the source of most TS errors.
- Additional root causes:
  - Duplicate pages `src/views/video/projects.vue`, `src/views/video/projects/index.vue`, `src/views/video/workbench.vue`, and `src/views/video/projects/workbench.vue` are all included in `vue-tsc`, so even unreferenced legacy copies must compile or be removed/proxied.
  - `batchGenerateShotsApi` and `composeProjectVideoApi` frontend types expect async job IDs, but backend currently returns direct synchronous result objects.
  - `projects/workbench.vue` imports many Ant Design components only used as auto-resolved template tags, creating `noUnusedLocals` failures.
  - `router/routes/modules/video.ts` imports `$t` but does not use it.

## Scope Decision

Implement the MVP closure first:

1. Backend contract remains camelCase to match existing Go JSON tags and neighboring `video.ts` conventions.
2. Frontend type definitions and UI logic are aligned to that backend contract.
3. Character/scene/shot create, update, delete work from the UI (no “开发中” placeholders for CRUD).
4. Prompt preview displays the backend preview as-is.
5. Batch generation and composition work with current backend semantics first; if time permits, add real async compose-job persistence using `video_compose_jobs`.
6. Optional docs items (advanced transitions, music upload picker, subtitles UI, visual continuity scoring, asset drag/drop binding) stay as extension points unless needed to make the core workflow usable.

---

## Chunk 1: Stabilize the frontend API contract and route/page duplication

### Task 1: Replace stale `videoproject.ts` types with backend camelCase types

**Files:**
- Modify: `nx-backend/apps/web-antd/src/api/core/videoproject.ts`

- [ ] **Step 1: Write a type-level contract test**

Create or update a lightweight TypeScript check by running the existing full typecheck first:

```bash
cd nx-backend/apps/web-antd
PATH=/opt/homebrew/bin:/usr/local/bin:$PATH pnpm -s typecheck
```

Expected before fix: fails with fields like `referenceImageUrl`, `videoUrl`, `actionDescription`, `generatedPrompt` missing from `Character`, `Scene`, and `Shot`.

- [ ] **Step 2: Update interfaces to match backend JSON tags**

Use these shapes in `videoproject.ts`:

```ts
export interface Project {
  id: string;
  name: string;
  description: string;
  theme: string;
  styleGuide: string;
  status: string;
  composeStatus: 'pending' | 'composing' | 'completed' | 'failed' | string;
  finalVideoAssetId: string;
  finalVideoUrl: string;
  createTime: string;
  updateTime: string;
  characterCount: number;
  sceneCount: number;
  totalShots: number;
  completedShots: number;
}

export interface Character {
  id: string;
  projectId: string;
  assetId: string;
  name: string;
  description: string;
  referenceImageUrl: string;
  isMain: boolean;
  createTime: string;
}

export interface Scene {
  id: string;
  projectId: string;
  assetId: string;
  name: string;
  description: string;
  referenceImageUrl: string;
  referenceVideoUrl: string;
  createTime: string;
}

export interface Shot {
  id: string;
  projectId: string;
  orderNum: number;
  name: string;
  actionDescription: string;
  duration: 5 | 10 | 15 | number;
  aspectRatio: '16:9' | '9:16' | '1:1' | string;
  characterIds: string[];
  sceneId: string;
  imageReferenceModes: string[];
  videoReferenceMode: string;
  cameraMovement: string;
  generationId: string;
  generatedPrompt: string;
  usedImages: string[];
  usedVideos: string[];
  endFrameUrl: string;
  status: 'draft' | 'generating' | 'completed' | 'failed' | string;
  errorMessage: string;
  createTime: string;
  updateTime: string;
  videoUrl: string;
}

export interface ShotPreview {
  prompt: string;
  images: string[];
  videos: string[];
  estimatedSuccessRate: number;
  validation: {
    isValid: boolean;
    errors: string[];
    warnings: string[];
  };
}

export interface BatchGenerateResponse {
  projectId: string;
  totalShots: number;
  successCount: number;
  failedCount: number;
  shotResults: Array<{
    shotId: string;
    shotName: string;
    orderNum: number;
    status: 'success' | 'failed' | 'skipped' | string;
    generationId: string;
    errorMessage: string;
  }>;
}

export interface BatchProgressResponse {
  projectId: string;
  total: number;
  completed: number;
  generating: number;
  failed: number;
  pending: number;
  progress: number;
}

export interface ComposeProjectInput {
  transition?: string;
  musicUrl?: string;
  enableSubtitles?: boolean;
}

export interface ComposeVideoResponse {
  projectId: string;
  videoUrl: string;
  duration: number;
  fileSize: number;
  shotCount: number;
  status: 'completed' | 'failed' | string;
  errorMessage?: string;
}

export interface ComposeStatusResponse {
  projectId: string;
  composeStatus: 'pending' | 'composing' | 'completed' | 'failed' | string;
  finalVideoUrl: string;
  totalShots: number;
  completedShots: number;
  canCompose: boolean;
}
```

- [ ] **Step 3: Align API return types**

Change:

```ts
export function generateShotApi(shotId: string | number) {
  return requestClient.post<VideoGeneration>(`/video/shots-generate/${shotId}`, {});
}

export function previewShotPromptApi(shotId: string | number) {
  return requestClient.get<ShotPreview>(`/video/shots-preview/${shotId}`);
}

export function composeProjectVideoApi(projectId: string | number, data: ComposeProjectInput = {}) {
  return requestClient.post<ComposeVideoResponse>(`/video/projects-compose/${projectId}`, data, {
    timeout: 180_000,
  });
}

export function getComposeStatusApi(projectId: string | number) {
  return requestClient.get<ComposeStatusResponse>(`/video/projects-compose-status/${projectId}`);
}
```

Import `VideoGeneration` from `./video` or duplicate only the minimal fields if import cycles appear.

- [ ] **Step 4: Run typecheck and capture remaining failures**

Run:

```bash
cd nx-backend/apps/web-antd
PATH=/opt/homebrew/bin:/usr/local/bin:$PATH pnpm -s typecheck
```

Expected: errors shift away from stale field names and toward page-local unused imports/logic mismatches.

### Task 2: Decide canonical pages and make duplicates compile

**Files:**
- Modify: `nx-backend/apps/web-antd/src/views/video/projects.vue`
- Modify: `nx-backend/apps/web-antd/src/views/video/projects/index.vue`
- Modify: `nx-backend/apps/web-antd/src/views/video/workbench.vue`
- Modify: `nx-backend/apps/web-antd/src/views/video/projects/workbench.vue`
- Modify: `nx-backend/apps/web-antd/src/router/routes/modules/video.ts`

- [ ] **Step 1: Remove unused `$t` import**

In `router/routes/modules/video.ts`, delete:

```ts
import { $t } from '#/locales';
```

- [ ] **Step 2: Use one canonical list implementation**

Keep `src/views/video/projects.vue` as the dynamic backend-menu target (`Component: /video/projects`) and make `src/views/video/projects/index.vue` a thin proxy:

```vue
<script setup lang="ts">
import ProjectsPage from '../projects.vue';
</script>

<template>
  <ProjectsPage />
</template>
```

This preserves both static module route and backend-menu dynamic route.

- [ ] **Step 3: Use one canonical workbench implementation**

Keep `src/views/video/projects/workbench.vue` as the canonical implementation. Make `src/views/video/workbench.vue` a thin proxy:

```vue
<script setup lang="ts">
import WorkbenchPage from './projects/workbench.vue';
</script>

<template>
  <WorkbenchPage />
</template>
```

- [ ] **Step 4: Fix `projects.vue` table typing**

If keeping the table version as canonical, type columns with Ant Design's table type:

```ts
import type { TableColumnsType } from 'ant-design-vue';
const columns: TableColumnsType<Project> = [
  // fixed: 'right' as const when needed
];
```

Also ensure `editingId` is `ref<string | number | ''>('')` and `getProgress` handles optional counts:

```ts
function getProgress(record: Project) {
  const total = record.totalShots || 0;
  if (!total) return 0;
  return Math.round(((record.completedShots || 0) / total) * 100);
}
```

- [ ] **Step 5: Re-run typecheck**

Run:

```bash
cd nx-backend/apps/web-antd
PATH=/opt/homebrew/bin:/usr/local/bin:$PATH pnpm -s typecheck
```

Expected: remaining errors only inside canonical `projects/workbench.vue`.

---

## Chunk 2: Make workbench CRUD and preview match the backend

### Task 3: Remove unused imports and align local types in `projects/workbench.vue`

**Files:**
- Modify: `nx-backend/apps/web-antd/src/views/video/projects/workbench.vue`

- [ ] **Step 1: Remove unused Ant Design component imports**

For `<a-button>`, `<a-form>`, etc., no script import is needed. Keep only `message`, `Modal`, and any runtime values actually referenced in script (`Empty`). Remove unused `router`, `updateShotApi` only if Task 5 does not use it.

- [ ] **Step 2: Use imported `ShotPreview` from API**

Delete local `interface ShotPreview` and import `type ShotPreview` from `#/api/core/videoproject`.

- [ ] **Step 3: Make preview modal null-safe only if needed**

After API type fix, `shotPreview.validation` is required. Template can remain as-is.

- [ ] **Step 4: Fix duration controls to match backend**

Backend allows only `5/10/15` seconds. Replace the `<a-input-number>` with a select:

```vue
<a-select v-model:value="shotForm.duration">
  <a-select-option :value="5">5 秒</a-select-option>
  <a-select-option :value="10">10 秒</a-select-option>
  <a-select-option :value="15">15 秒</a-select-option>
</a-select>
```

Default `shotForm.duration` should be `15` unless the UI intentionally defaults to 5.

- [ ] **Step 5: Fix shot order default**

When creating a shot, omit `orderNum` or use `shots.value.length + 1`, not `shots.value.length`.

```ts
await createShotApi(projectId.value, {
  ...shotForm.value,
  orderNum: shots.value.length + 1,
});
```

- [ ] **Step 6: Re-run typecheck**

Expected: no stale model errors remain.

### Task 4: Implement character and scene update flows

**Files:**
- Modify: `nx-backend/apps/web-antd/src/views/video/projects/workbench.vue`
- Existing API: `updateCharacterApi`, `updateSceneApi`

- [ ] **Step 1: Add editing IDs**

```ts
const editingCharacterId = ref<string>('');
const editingSceneId = ref<string>('');
```

- [ ] **Step 2: Update modal titles**

```vue
<a-modal :title="editingCharacterId ? '编辑角色' : '添加角色'" ...>
<a-modal :title="editingSceneId ? '编辑场景' : '添加场景'" ...>
```

- [ ] **Step 3: Fill forms on edit**

```ts
function editCharacter(char: Character) {
  editingCharacterId.value = char.id;
  characterForm.value = {
    name: char.name,
    description: char.description,
    referenceImageUrl: char.referenceImageUrl,
    isMain: char.isMain,
  };
  characterModalVisible.value = true;
}

function editScene(scene: Scene) {
  editingSceneId.value = scene.id;
  sceneForm.value = {
    name: scene.name,
    description: scene.description,
    referenceImageUrl: scene.referenceImageUrl,
    referenceVideoUrl: scene.referenceVideoUrl,
  };
  sceneModalVisible.value = true;
}
```

- [ ] **Step 4: Branch create/update in handlers**

```ts
if (editingCharacterId.value) {
  await updateCharacterApi(editingCharacterId.value, characterForm.value);
  message.success('更新成功');
} else {
  await createCharacterApi(projectId.value, characterForm.value);
  message.success('添加成功');
}
```

Same pattern for scenes.

- [ ] **Step 5: Reset editing IDs in add handlers and after submit**

`showAddCharacter()` and `showAddScene()` must clear their editing IDs.

- [ ] **Step 6: Run typecheck**

Expected: update APIs are used and no unused edit parameter errors remain.

### Task 5: Implement shot edit/update flow and backend preview display

**Files:**
- Modify: `nx-backend/apps/web-antd/src/views/video/projects/workbench.vue`
- Existing API: `updateShotApi`, `previewShotPromptApi`

- [ ] **Step 1: Add shot editing ID**

```ts
const editingShotId = ref<string>('');
```

- [ ] **Step 2: Use dynamic modal title**

```vue
<a-modal :title="editingShotId ? '编辑分镜' : '添加分镜'" ...>
```

- [ ] **Step 3: Populate shot form on edit**

```ts
function editShot(shot: Shot) {
  editingShotId.value = shot.id;
  shotForm.value = {
    name: shot.name,
    actionDescription: shot.actionDescription,
    duration: shot.duration || 15,
    aspectRatio: shot.aspectRatio || '16:9',
    characterIds: [...(shot.characterIds || [])],
    sceneId: shot.sceneId || '',
    cameraMovement: shot.cameraMovement || '',
  };
  shotModalVisible.value = true;
}
```

- [ ] **Step 4: Branch create/update in `handleAddShot`**

```ts
const payload = { ...shotForm.value };
if (editingShotId.value) {
  await updateShotApi(editingShotId.value, payload);
  message.success('更新成功');
} else {
  await createShotApi(projectId.value, { ...payload, orderNum: shots.value.length + 1 });
  message.success('添加成功');
}
```

- [ ] **Step 5: Display backend preview directly**

```ts
const result = await previewShotPromptApi(shot.id);
shotPreview.value = result;
```

Do not overwrite `images`, `validation`, or `estimatedSuccessRate` with placeholders.

- [ ] **Step 6: Update selected shot after reload**

After `loadShots()`, preserve selection:

```ts
if (selectedShot.value) {
  selectedShot.value = shots.value.find((s) => s.id === selectedShot.value?.id) || null;
}
```

- [ ] **Step 7: Run typecheck**

Expected: workbench compiles.

---

## Chunk 3: Align batch generation and composition UX with actual backend behavior

### Task 6: Make batch generation UX correct for synchronous backend MVP

**Files:**
- Modify: `nx-backend/apps/web-antd/src/views/video/projects/workbench.vue`
- Modify: `nx-backend/apps/web-antd/src/api/core/videoproject.ts`

- [ ] **Step 1: Remove fake `batchId` dependency for MVP**

Backend `GenerateAllShots` returns a final `BatchGenerateResponse`; it does not return `batch_id`.

- [ ] **Step 2: Update `generateAllShots`**

```ts
async function generateAllShots() {
  generating.value = true;
  try {
    const result = await batchGenerateShotsApi(projectId.value);
    const skipped = result.shotResults.filter((r) => r.status === 'skipped').length;
    if (result.failedCount > 0) {
      message.warning(`批量生成完成：成功 ${result.successCount} 个，跳过 ${skipped} 个，失败 ${result.failedCount} 个`);
    } else {
      message.success(`批量生成完成：成功 ${result.successCount} 个，跳过 ${skipped} 个`);
    }
    await loadShots();
  } catch (error) {
    message.error('批量生成失败');
  } finally {
    generating.value = false;
  }
}
```

- [ ] **Step 3: Optionally keep passive progress refresh**

If the backend is later made async, reintroduce polling. For MVP, remove `batchId`, `pollBatchProgress`, and `getBatchProgressApi` usage from the UI unless displaying a manual refresh.

- [ ] **Step 4: Run typecheck**

Expected: no `batch_id` / `total_shots` errors.

### Task 7: Make composition UX correct for synchronous backend MVP

**Files:**
- Modify: `nx-backend/apps/web-antd/src/views/video/projects/workbench.vue`
- Modify: `nx-backend/apps/web-antd/src/api/core/videoproject.ts`

- [ ] **Step 1: Add basic compose options state**

For MVP, use direct concat by default:

```ts
const composeOptions = ref({
  transition: 'none',
  musicUrl: '',
  enableSubtitles: false,
});
```

- [ ] **Step 2: Update `composeVideo` for direct result**

```ts
async function composeVideo() {
  const completedShots = shots.value.filter((s) => s.status === 'completed' && s.videoUrl);
  if (completedShots.length === 0) {
    message.warning('没有已完成的分镜，无法合成视频');
    return;
  }

  composing.value = true;
  try {
    const result = await composeProjectVideoApi(projectId.value, composeOptions.value);
    message.success('视频合成完成');
    Modal.success({
      title: '视频合成成功',
      content: `视频已生成：${result.videoUrl}`,
      onOk: () => result.videoUrl && window.open(result.videoUrl, '_blank'),
    });
    await loadProject();
  } catch (error) {
    message.error('视频合成失败');
  } finally {
    composing.value = false;
    composeProgress.value = 0;
  }
}
```

- [ ] **Step 3: Remove fake `composeId` polling for MVP**

Delete `composeId`, `pollComposeProgress`, and `compose_id` assumptions unless Task 10 implements real async compose jobs.

- [ ] **Step 4: Run typecheck**

Expected: no `compose_id` / `video_url` errors.

---

## Chunk 4: Backend correctness and data safety fixes

### Task 8: Add backend tests for prompt builder behavior

**Files:**
- Create: `nx-backend/apps/server/internal/videoproject/promptbuilder_test.go`
- Modify only if needed: `nx-backend/apps/server/internal/videoproject/promptbuilder.go`

- [ ] **Step 1: Write focused unit tests for pure helper behavior**

Because `BuildPreview` needs DB, start with package-level tests for pure methods:

```go
func TestEnhanceActionUsesLongestDictionaryMatch(t *testing.T) {
    b := &PromptBuilder{}
    got := b.enhanceAction("女孩走进森林")
    if !strings.Contains(got, "walking into") {
        t.Fatalf("expected longest match walking into, got %q", got)
    }
    if strings.Contains(got, "walking slowly") {
        t.Fatalf("used shorter dictionary match: %q", got)
    }
}

func TestBuildReferenceImagesPriorityAndLimit(t *testing.T) {
    b := &PromptBuilder{}
    prev := &Shot{EndFrameURL: "prev.jpg"}
    scene := &Scene{ReferenceImageURL: "scene.jpg"}
    chars := []Character{
        {ReferenceImageURL: "side.jpg"},
        {ReferenceImageURL: "main.jpg", IsMain: true},
    }
    shot := Shot{ImageReferenceModes: []string{"prev_frame", "character_ref", "scene_ref"}}
    got := b.buildReferenceImages(shot, chars, scene, prev)
    want := []string{"prev.jpg", "main.jpg", "side.jpg", "scene.jpg"}
    if !reflect.DeepEqual(got, want) { t.Fatalf("got %#v want %#v", got, want) }
}
```

- [ ] **Step 2: Run tests to verify pass/fail**

```bash
cd nx-backend/apps/server
PATH=/opt/homebrew/bin:/usr/local/go/bin:/usr/local/bin:$PATH go test ./internal/videoproject
```

Expected: pass if current behavior matches; if not, fix minimal helper logic.

### Task 9: Fix binary upload readers in generator/composer

**Files:**
- Modify: `nx-backend/apps/server/internal/videoproject/generator.go`
- Modify: `nx-backend/apps/server/internal/videoproject/projectcomposer.go`

- [ ] **Step 1: Identify binary upload bug**

Current code uses `strings.NewReader(string(data))` for image/video bytes. This can corrupt or inflate binary data.

- [ ] **Step 2: Replace with `bytes.NewReader(data)`**

In both files, import `bytes` and change:

```go
Reader: bytes.NewReader(data),
```

- [ ] **Step 3: Run Go tests**

```bash
cd nx-backend/apps/server
PATH=/opt/homebrew/bin:/usr/local/go/bin:/usr/local/bin:$PATH go test ./internal/videoproject ./internal/server
```

Expected: pass.

### Task 10: Choose whether to implement real async compose jobs now

**Files if implemented:**
- Modify: `nx-backend/apps/server/internal/videoproject/projectcomposer.go`
- Modify: `nx-backend/apps/server/internal/server/videoproject_routes.go`
- Modify: `nx-backend/apps/web-antd/src/api/core/videoproject.ts`
- Modify: `nx-backend/apps/web-antd/src/views/video/projects/workbench.vue`

- [ ] **Step 1: MVP decision**

If the composition command can run within HTTP timeout for expected project sizes, keep synchronous MVP and skip this task.

- [ ] **Step 2: If async is required, add job creation**

Add `StartComposeProject(ctx, projectID, input) (jobID string, error)` that inserts into `video_compose_jobs`, sets project `compose_status='composing'`, and launches goroutine.

- [ ] **Step 3: Add job status endpoint semantics**

Return job status by ID or latest job for project. Include `status`, `progress`, `finalVideoUrl`, `errorMessage`.

- [ ] **Step 4: Update frontend to poll the job**

Only reintroduce `composeId` after backend returns a real `composeId`.

- [ ] **Step 5: Test with Go tests and frontend typecheck**

Expected: route contract and UI match.

---

## Chunk 5: Verification and goal audit

### Task 11: Full local verification

**Files:**
- No source modifications unless verification reveals failures.

- [ ] **Step 1: Run backend tests**

```bash
cd nx-backend/apps/server
PATH=/opt/homebrew/bin:/usr/local/go/bin:/usr/local/bin:$PATH go test ./...
```

Expected: all packages pass.

- [ ] **Step 2: Run frontend typecheck**

```bash
cd nx-backend/apps/web-antd
PATH=/opt/homebrew/bin:/usr/local/bin:$PATH pnpm -s typecheck
```

Expected: passes.

- [ ] **Step 3: Run frontend build if typecheck passes**

```bash
cd nx-backend/apps/web-antd
PATH=/opt/homebrew/bin:/usr/local/bin:$PATH pnpm -s build
```

Expected: production build completes.

- [ ] **Step 4: Verify documented routes exist**

Check:

```bash
rg -n "/api/video/projects|/api/video/shots|projects-compose|projects-batch" nx-backend/apps/server/internal/server
rg -n "path: 'projects'|projects/:id/workbench" nx-backend/apps/web-antd/src/router/routes/modules/video.ts
```

Expected: project list/workbench routes and backend APIs are present.

- [ ] **Step 5: Verify no stale snake_case contract remains in project workflow**

```bash
rg -n "batch_id|compose_id|total_shots|video_url|project_id|shot_number|reference_images|estimated_success_rate" nx-backend/apps/web-antd/src/api/core/videoproject.ts nx-backend/apps/web-antd/src/views/video/projects.vue nx-backend/apps/web-antd/src/views/video/projects/workbench.vue nx-backend/apps/web-antd/src/views/video/workbench.vue
```

Expected: no stale fields, except comments deliberately documenting legacy compatibility.

- [ ] **Step 6: Manual smoke path**

If dev server/database are available:

1. Open `/video/projects`.
2. Create a project with theme/style.
3. Enter workbench.
4. Add one role and one scene.
5. Add one shot with 5/10/15 seconds.
6. Preview prompt; verify prompt, reference images, validation, success rate display.
7. Trigger single-shot generation; verify row status becomes generating.
8. If a completed video exists, trigger composition and verify final URL appears.

Expected: no runtime errors in browser console for the workflow.

### Task 12: Completion audit against `video*.md`

- [ ] **Step 1: Map explicit docs requirements to implementation evidence**

Create a short checklist in the final response covering:

- project workflow pages
- project/character/scene/shot CRUD
- prompt builder and preview
- reference image priority and previous frame inheritance
- single-shot generation using existing API
- batch generation
- end-frame extraction
- FFmpeg composition
- frontend compile/build status

- [ ] **Step 2: Mark optional items honestly**

If not implemented, explicitly label these as optional/future:

- advanced transitions beyond simple pass-through
- background music asset picker/upload UI
- subtitle burn-in UI
- continuity scoring
- drag/drop binding
- asset success-rate analytics

- [ ] **Step 3: Do not mark the goal complete until evidence proves the MVP requirements and verification commands pass.**
