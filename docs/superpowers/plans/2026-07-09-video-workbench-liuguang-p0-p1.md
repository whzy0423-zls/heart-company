# Video Workbench Liuguang P0/P1 Implementation Plan

> **For agentic workers:** REQUIRED: Use superpowers:executing-plans to implement this plan when subagents are unavailable. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Bring nine-xing's project-mode video generation workbench closer to liuguang's storyboard workbench for P0/P1: per-shot board cards, script content, and shot-level image/video/audio references uploaded to OSS with preview.

**Architecture:** Keep the existing nine-xing project-mode model, but extend `video_shots` with liuguang-compatible text fields and add a `video_shot_assets` table for per-shot reference assets. The Vue workbench will stay Ant Design/Vben-based while adopting liuguang's board-card information architecture.

**Tech Stack:** Go net/http + PostgreSQL schema, Vue 3 `<script setup>`, Ant Design Vue, existing `uploadFileApi` and upload asset preview resolver, Vitest static migration tests.

---

## Chunk 1: Tests and data model

### Task 1: Add failing migration tests

**Files:**
- Modify: `nx-backend/apps/web-antd/src/views/video/project-workbench-production-flow.test.ts`
- Test command: `pnpm exec vitest run apps/web-antd/src/views/video/project-workbench-production-flow.test.ts`

- [ ] Add assertions that workbench contains `board-card`, `board-body`, `col-left`, `col-generate`, `col-version`.
- [ ] Add assertions for `scriptOriginalContent` and `handleScriptOriginalContentChange`.
- [ ] Add assertions for shot-level asset upload handlers and API calls: image/video/audio.
- [ ] Run test and confirm it fails because these capabilities are missing.

### Task 2: Extend backend data model

**Files:**
- Modify: `nx-backend/apps/server/internal/db/schema.sql`
- Modify: `nx-backend/apps/server/internal/videoproject/videoproject.go`
- Modify: `nx-backend/apps/server/internal/server/videoproject_routes.go`
- Modify: `nx-backend/apps/server/internal/server/server.go`
- Modify: `nx-backend/apps/web-antd/src/api/core/videoproject.ts`

- [ ] Add `script_original_content`, `dynamic_description`, `grid_storyboard_prompt`, `storyboard_url` columns to `video_shots` via `CREATE TABLE` definition and `ALTER TABLE ADD COLUMN IF NOT EXISTS` for existing DBs.
- [ ] Add `video_shot_assets` table with `shot_id`, `asset_type`, `object_url`, `name`, `mime_type`, `size_bytes`, timestamps.
- [ ] Add Go structs `ShotAsset`, `ShotAssetInput`, fields on `Shot`/`ShotInput`.
- [ ] Add store methods: list/create/delete shot assets.
- [ ] Add API routes under `/api/video/shots-assets/...`.
- [ ] Update TS API/types.

## Chunk 2: Frontend P0/P1 UI

### Task 3: Convert shot list to liuguang-style cards

**Files:**
- Modify: `nx-backend/apps/web-antd/src/views/video/projects/workbench.vue`

- [ ] Rename visual structure to `board-list` / `board-card` / `board-head` / `board-body`.
- [ ] Each card contains left column with title, script textarea, action textarea, metadata, and reference assets.
- [ ] Add middle `col-generate` with generated video preview and generate button.
- [ ] Add right `col-version` placeholder with current generated video and future version-management affordance.
- [ ] Preserve existing inline add/edit behavior, generation, batch generation, drag binding, and project compose.

### Task 4: Add shot-level OSS asset upload and preview

**Files:**
- Modify: `nx-backend/apps/web-antd/src/views/video/projects/workbench.vue`

- [ ] Add image/video/audio upload buttons inside each shot card.
- [ ] Use existing `uploadFileApi` to upload to `video/shot-image`, `video/shot-video`, `video/shot-audio`.
- [ ] After upload, call create shot asset API and reload shots.
- [ ] Render image/video/audio previews from `shot.shotAssets` via `previewReferenceAsset`.
- [ ] Add delete action for shot assets.

## Chunk 3: Verification

### Task 5: Run focused checks

- [ ] `pnpm exec vitest run apps/web-antd/src/views/video/project-workbench-production-flow.test.ts`
- [ ] `pnpm --filter @vben/web-antd run typecheck`
- [ ] `go test ./internal/server ./internal/videoproject -count=1`
