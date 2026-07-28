# Classroom Cover Management Implementation Plan

> **For agentic workers:** REQUIRED: Use superpowers:subagent-driven-development (if subagents available) or superpowers:executing-plans to implement this plan. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Add cloud-backed manual courseware covers, first-frame video fallback, audio default fallback, and selectable `16:9` / `9:16` / `1:1` display ratios across the admin and miniapp.

**Architecture:** Persist only ownership metadata and the selected ratio on `classroom_contents`; keep generated video covers on the media asset. Resolve one effective `coverUrl` on the server using a shared priority rule and short-lived OSS URLs, so clients never need object keys. Use dedicated admin upload/delete endpoints and keep ratio changes in the normal content update flow.

**Tech Stack:** Go 1.x, PostgreSQL schema SQL, Alibaba Cloud OSS storage interfaces, FFmpeg/ffprobe, net/http, Vue 3 + Ant Design Vue + TypeScript, uni-app miniapp, Go tests and Node/Vue tests.

**Design:** `docs/superpowers/specs/2026-07-28-classroom-cover-management-design.md`

**Workspace constraint:** Work in `/Users/wohenzaiyi/Desktop/nine-xing` on `detail-tuning-video-management`. The pre-existing classroom workflow fixes were verified and isolated in commit `5dcb8f0`; do not rewrite that commit. Stage only files belonging to the current task.

**Cover-management API contract:** Cover operations are independent from ordinary metadata editing, so they are allowed for `draft`, `processing`, `ready`, `published`, and `offline` content without changing lifecycle state.

```text
POST   /api/admin/classroom/contents/{id}/cover
       multipart: file, expectedUpdatedAt
DELETE /api/admin/classroom/contents/{id}/cover?expectedUpdatedAt=<RFC3339>
PUT    /api/admin/classroom/contents/{id}/cover-settings
       JSON: { coverAspectRatio, expectedUpdatedAt }
```

Every operation returns the latest content DTO and `updatedAt`; the client must replace its local version before issuing the next cover operation. New content must be saved as a draft before its row-level “封面管理” action is enabled, because object ownership requires a content ID.

---

## Chunk 1: Backend persistence and cover resolution

### Task 1: Persist cover ownership and aspect ratio

**Files:**
- Modify: `nx-backend/apps/server/internal/db/schema.sql`
- Modify: `nx-backend/apps/server/internal/db/schema_classroom_test.go`
- Modify: `nx-backend/apps/server/internal/classroom/models.go`
- Modify: `nx-backend/apps/server/internal/classroom/store.go`
- Modify: `nx-backend/apps/server/internal/classroom/store_test.go`

- [ ] **Step 1: Add failing schema tests**

Assert that `classroom_contents` contains `manual_cover_object_key TEXT NOT NULL DEFAULT ''`, `cover_aspect_ratio TEXT NOT NULL DEFAULT '16:9'`, an idempotent upgrade path for existing databases, and a three-value check constraint.

- [ ] **Step 2: Run the schema test and confirm RED**

Run:

```bash
cd nx-backend/apps/server
go test ./internal/db -run 'TestSchema.*Classroom' -count=1
```

Expected: FAIL because the new columns and constraint do not exist.

- [ ] **Step 3: Add failing model/store tests**

Cover these behaviors:

- empty ratio normalizes to `16:9`;
- all three ratios validate;
- another value fails validation;
- create, get, list, and update scan/write `manual_cover_object_key` and `cover_aspect_ratio`;
- the existing `tags=[]` behavior remains intact.

- [ ] **Step 4: Run model/store tests and confirm RED**

Run:

```bash
go test ./internal/classroom -run 'Test(Content.*Cover|Store.*Content|CreateContent|UpdateContent|ListContents)' -count=1
```

Expected: FAIL on missing model fields or SQL columns.

- [ ] **Step 5: Implement minimal schema and persistence changes**

Add a `CoverAspectRatio` enum with constants for `16:9`, `9:16`, and `1:1`. Add a normalization helper used before validation/persistence, extend all content SQL column lists and scans consistently, and update existing database upgrade SQL without rebuilding the table.

- [ ] **Step 6: Run focused and package tests and confirm GREEN**

```bash
go test ./internal/db -run 'TestSchema.*Classroom' -count=1
go test ./internal/classroom -count=1
```

- [ ] **Step 7: Commit only Task 1 files**

```bash
git add nx-backend/apps/server/internal/db/schema.sql \
  nx-backend/apps/server/internal/db/schema_classroom_test.go \
  nx-backend/apps/server/internal/classroom/models.go \
  nx-backend/apps/server/internal/classroom/store.go \
  nx-backend/apps/server/internal/classroom/store_test.go
git commit -m "feat: persist classroom cover settings"
```

### Task 2: Resolve one effective cover URL for admin and public DTOs

**Files:**
- Create: `nx-backend/apps/server/internal/classroom/cover.go`
- Create: `nx-backend/apps/server/internal/classroom/cover_test.go`
- Modify: `nx-backend/apps/server/internal/server/server.go`
- Modify: `nx-backend/apps/server/internal/config/env.go`
- Modify: `nx-backend/apps/server/internal/config/env_test.go`
- Modify: `nx-backend/apps/server/internal/server/classroom_admin.go`
- Modify: `nx-backend/apps/server/internal/server/classroom_admin_test.go`
- Modify: `nx-backend/apps/server/internal/server/classroom_public.go`
- Modify: `nx-backend/apps/server/internal/server/classroom_public_test.go`
- Modify: `nx-backend/apps/server/internal/server/classroom_progress.go`
- Modify: `nx-backend/apps/server/internal/server/classroom_progress_test.go`
- Modify: `nx-backend/apps/server/internal/server/classroom_vertical_contract_test.go`
- Create: `nx-backend/apps/server/internal/server/classroom_audio_cover.svg`
- Create: `nx-backend/apps/server/internal/server/classroom_audio_cover.go`

- [ ] **Step 1: Write failing resolver tests**

Define and test a small resolver that accepts content type, manual object key, generated object key, legacy URL, and an `storage.ObjectSigner`. Assert exact priority: manual → generated → legacy → audio default → empty. Assert object keys are presigned and never returned raw, while legacy URLs remain unchanged.

- [ ] **Step 2: Run resolver test and confirm RED**

```bash
cd nx-backend/apps/server
go test ./internal/classroom -run 'TestResolve.*Cover' -count=1
```

- [ ] **Step 3: Write failing DTO/query tests**

Extend admin/public/progress/vertical-contract test fixtures and SQL expectations so DTOs return `coverUrl` and `coverAspectRatio`, and admin DTOs additionally return `manualCoverObjectKey` plus `coverSource`. For admin lists, specify a batch cover-context query keyed by the returned content IDs; it may presign each object but must not query media/series once per row. Public list/detail SQL must join the media cover key directly.

- [ ] **Step 4: Run server tests and confirm RED**

```bash
go test ./internal/server -run 'Test.*Classroom.*(Content|Progress|Public|Cover)' -count=1
```

- [ ] **Step 5: Implement shared resolution and wire the signer**

Use the existing classroom OSS object signer created in `server.go`. Add a bounded cover URL TTL setting (default 1800 seconds), avoid N+1 media lookups by extending the relevant content queries or introducing focused joined projections, and serve an embedded SVG through one stable public audio-cover path shared by admin and miniapp. Because signed URLs expire, mark metadata responses containing them `Cache-Control: private, no-store` rather than allowing stale `304` responses. Map signing failures to the existing classroom service error handling.

- [ ] **Step 6: Run focused tests and confirm GREEN**

```bash
go test ./internal/classroom -count=1
go test ./internal/server -run 'Test.*Classroom' -count=1
```

- [ ] **Step 7: Commit Task 2**

```bash
git add nx-backend/apps/server/internal/classroom/cover.go \
  nx-backend/apps/server/internal/classroom/cover_test.go \
  nx-backend/apps/server/internal/server/server.go \
  nx-backend/apps/server/internal/config/env.go \
  nx-backend/apps/server/internal/config/env_test.go \
  nx-backend/apps/server/internal/server/classroom_admin.go \
  nx-backend/apps/server/internal/server/classroom_admin_test.go \
  nx-backend/apps/server/internal/server/classroom_public.go \
  nx-backend/apps/server/internal/server/classroom_public_test.go \
  nx-backend/apps/server/internal/server/classroom_progress.go \
  nx-backend/apps/server/internal/server/classroom_progress_test.go \
  nx-backend/apps/server/internal/server/classroom_vertical_contract_test.go \
  nx-backend/apps/server/internal/server/classroom_audio_cover.svg \
  nx-backend/apps/server/internal/server/classroom_audio_cover.go
git commit -m "feat: resolve effective classroom covers"
```

## Chunk 2: Cover object lifecycle and automatic generation

### Task 3: Add manual cover upload and deletion APIs

**Files:**
- Create: `nx-backend/apps/server/internal/classroom/cover_service.go`
- Create: `nx-backend/apps/server/internal/classroom/cover_service_test.go`
- Modify: `nx-backend/apps/server/internal/classroom/store.go`
- Modify: `nx-backend/apps/server/internal/classroom/store_test.go`
- Modify: `nx-backend/apps/server/internal/server/classroom_admin.go`
- Modify: `nx-backend/apps/server/internal/server/classroom_admin_test.go`
- Modify: `nx-backend/apps/server/internal/server/server.go`

- [ ] **Step 1: Write failing service tests**

Test upload, replacement, deletion, idempotent deletion, DB failure cleanup, old-object cleanup after successful replacement, already-gone deletion, invalid MIME, and size limit. Confirm deleting a manual cover never touches the media generated cover key. Use a narrow `classroomCoverStorage` interface composed of `storage.ObjectUploader`, `storage.ObjectSigner`, and `DeleteObject(context.Context, string) error`; inject the same classroom `MultipartStorage` configured with `CLASSROOM_MEDIA_BUCKET`, not the generic upload bucket.

- [ ] **Step 2: Run service tests and confirm RED**

```bash
cd nx-backend/apps/server
go test ./internal/classroom -run 'TestCoverService' -count=1
```

- [ ] **Step 3: Implement the cover service minimally**

Use the injected narrow cover storage for `classroom/covers/manual/{contentID}/...`, actual-content MIME detection, an explicit image byte limit, and store methods that atomically swap/clear `manual_cover_object_key` with the expected content timestamp.

- [ ] **Step 4: Write failing HTTP tests**

Cover:

- `POST /api/admin/classroom/contents/{id}/cover` multipart field `file`;
- POST multipart field `expectedUpdatedAt` is mandatory;
- `DELETE /api/admin/classroom/contents/{id}/cover?expectedUpdatedAt=<RFC3339>`;
- write permission routing;
- 200 response with updated DTO and fallback after deletion;
- 400 invalid image, 404 content, 409 conflict, 503 unavailable storage.

- [ ] **Step 5: Run HTTP tests and confirm RED**

```bash
go test ./internal/server -run 'TestClassroomContentCover' -count=1
```

- [ ] **Step 6: Implement routes and handlers**

Extend the existing `/contents/` action parser without interfering with publish/offline/price actions. Limit the multipart body before reading it, call the cover service, write the existing audit log, and return `toContentDTO`. Permit cover operations for every non-deleted lifecycle state and return 409 on a stale timestamp.

- [ ] **Step 7: Add deletion-cleanup tests and implementation**

Extend content deletion tests so the handler deletes the database record first and then removes only its manual cover object. A database failure must not delete the object; an already-gone object is success; post-commit cleanup failure is logged as an orphan without resurrecting the deleted record.

- [ ] **Step 8: Run focused tests and confirm GREEN**

```bash
go test ./internal/classroom -count=1
go test ./internal/server -run 'Test.*Classroom' -count=1
```

- [ ] **Step 9: Commit Task 3**

```bash
git add nx-backend/apps/server/internal/classroom/cover_service.go \
  nx-backend/apps/server/internal/classroom/cover_service_test.go \
  nx-backend/apps/server/internal/classroom/store.go \
  nx-backend/apps/server/internal/classroom/store_test.go \
  nx-backend/apps/server/internal/server/classroom_admin.go \
  nx-backend/apps/server/internal/server/classroom_admin_test.go \
  nx-backend/apps/server/internal/server/server.go
git commit -m "feat: manage manual classroom covers"
```

### Task 4: Generate first-frame covers at the selected ratio

**Files:**
- Modify: `nx-backend/apps/server/internal/classroom/upload.go`
- Modify: `nx-backend/apps/server/internal/classroom/upload_test.go`
- Modify: `nx-backend/apps/server/internal/classroom/models.go`
- Modify: `nx-backend/apps/server/internal/classroom/cover_service.go`
- Modify: `nx-backend/apps/server/internal/classroom/cover_service_test.go`
- Modify: `nx-backend/apps/server/internal/server/classroom_admin.go`
- Modify: `nx-backend/apps/server/internal/server/classroom_admin_test.go`
- Modify: `nx-backend/apps/server/internal/server/server.go`

- [ ] **Step 1: Write failing extractor command tests**

For every ratio assert the command:

- does not contain `-ss 00:00:01`;
- selects one first decodable frame;
- uses scale-up plus centered crop;
- outputs `1280x720`, `720x1280`, or `1080x1080` respectively.

- [ ] **Step 2: Run extractor tests and confirm RED**

```bash
cd nx-backend/apps/server
go test ./internal/classroom -run 'TestFFmpegCoverExtractor' -count=1
```

- [ ] **Step 3: Change extractor contract and upload completion**

Pass `content.CoverAspectRatio` into extraction, keep the current OSS upload and failure cleanup semantics, and preserve upload completion idempotency.

- [ ] **Step 4: Add failing ratio-change regeneration tests**

Add HTTP/service orchestration tests for `PUT /api/admin/classroom/contents/{id}/cover-settings`. When a video content ratio changes and generated media exists, create a new generated cover using the new ratio, atomically swap `media.cover_object_key`, then delete the old generated object. Test new-object cleanup when the DB swap fails, stale `expectedUpdatedAt` returns 409, and the response carries the new `updatedAt`. Confirm manual covers remain selected and lifecycle status is unchanged.

- [ ] **Step 5: Implement regeneration and confirm GREEN**

```bash
go test ./internal/classroom -count=1
go test ./internal/server -run 'Test.*Classroom' -count=1
```

- [ ] **Step 6: Commit Task 4**

```bash
git add nx-backend/apps/server/internal/classroom/upload.go \
  nx-backend/apps/server/internal/classroom/upload_test.go \
  nx-backend/apps/server/internal/classroom/models.go \
  nx-backend/apps/server/internal/classroom/cover_service.go \
  nx-backend/apps/server/internal/classroom/cover_service_test.go \
  nx-backend/apps/server/internal/server/classroom_admin.go \
  nx-backend/apps/server/internal/server/classroom_admin_test.go \
  nx-backend/apps/server/internal/server/server.go
git commit -m "feat: generate classroom covers from first frame"
```

## Chunk 3: Admin and miniapp user experience

### Task 5: Add admin cover controls

**Files:**
- Modify: `nx-backend/apps/web-antd/src/api/core/classroom.ts`
- Modify: `nx-backend/apps/web-antd/src/views/classroom/editor-model.ts`
- Modify: `nx-backend/apps/web-antd/src/views/classroom/classroom.test.ts`
- Modify: `nx-backend/apps/web-antd/src/views/classroom/classroom.integration.test.ts`
- Modify: `nx-backend/apps/web-antd/src/views/classroom/components/content-editor.vue`
- Create: `nx-backend/apps/web-antd/src/views/classroom/components/content-cover-editor.vue`
- Modify: `nx-backend/apps/web-antd/src/views/classroom/index.vue`
- Modify: `nx-backend/apps/web-antd/src/views/classroom/classroom-view-model.ts`

- [ ] **Step 1: Write failing model/API tests**

Assert new content defaults to `16:9`; a new unsaved draft explains “请先保存课件，再管理封面”; every persisted row, including published/ready/offline rows, has a separate “封面管理” action; ordinary metadata controls remain locked for non-drafts. Assert upload `FormData` contains `file` and `expectedUpdatedAt`, delete uses the latest timestamp query, ratio update uses the latest timestamp JSON, and each returned DTO refreshes the next operation's version.

- [ ] **Step 2: Run frontend tests and confirm RED**

```bash
cd nx-backend
pnpm exec vitest run apps/web-antd/src/views/classroom/classroom.test.ts apps/web-antd/src/views/classroom/classroom.integration.test.ts
```

- [ ] **Step 3: Implement API types and editor controls**

Add ratio/source types and upload/delete/settings functions. Implement a dedicated cover modal so published content does not enter the ordinary metadata editor. Add the three-option selector, aspect-ratio preview, `object-fit: cover`, upload progress/disabled state, source label, and delete-manual-cover button. Keep the current preview on errors, refresh the modal and table row from the returned DTO after every success, and serialize operations to prevent version races.

- [ ] **Step 4: Run tests and typecheck**

```bash
pnpm exec vitest run apps/web-antd/src/views/classroom/classroom.test.ts apps/web-antd/src/views/classroom/classroom.integration.test.ts
pnpm --filter @vben/web-antd typecheck
```

- [ ] **Step 5: Commit Task 5**

```bash
git add nx-backend/apps/web-antd/src/api/core/classroom.ts \
  nx-backend/apps/web-antd/src/views/classroom/editor-model.ts \
  nx-backend/apps/web-antd/src/views/classroom/classroom.test.ts \
  nx-backend/apps/web-antd/src/views/classroom/classroom.integration.test.ts \
  nx-backend/apps/web-antd/src/views/classroom/components/content-editor.vue \
  nx-backend/apps/web-antd/src/views/classroom/components/content-cover-editor.vue \
  nx-backend/apps/web-antd/src/views/classroom/index.vue \
  nx-backend/apps/web-antd/src/views/classroom/classroom-view-model.ts
git commit -m "feat: add classroom cover editor"
```

### Task 6: Honor cover ratios in the miniapp

**Files:**
- Modify: `miniapp/src/api/index.js`
- Modify: `miniapp/src/utils/classroomDisplay.js`
- Modify: `miniapp/src/utils/classroomDisplay.test.mjs`
- Modify: `miniapp/src/pages/classroom/classroom.vue`
- Modify: `miniapp/src/pages/classroom/classroom.test.mjs`
- Modify: `miniapp/src/pages/classroom-detail/classroom-detail.vue`
- Modify: `miniapp/src/pages/classroom-detail/classroom-detail.test.mjs`
- Modify: `miniapp/src/pages/learn/learn.vue`
- Modify: `miniapp/src/pages/learn/learn.content-state.test.mjs`

- [ ] **Step 1: Write failing display tests**

Assert the API/display normalization keeps only the three accepted ratios and falls back to `16:9`; list/detail images use the returned `coverUrl`, apply the matching container ratio, and retain the existing empty-cover placeholder.

- [ ] **Step 2: Run miniapp tests and confirm RED**

```bash
cd miniapp
node src/utils/classroomDisplay.test.mjs
node src/pages/classroom/classroom.test.mjs
node src/pages/classroom-detail/classroom-detail.test.mjs
node src/pages/learn/learn.content-state.test.mjs
```

- [ ] **Step 3: Implement ratio-aware cards and detail hero**

Use `mode="aspectFill"`, a whitelisted ratio-to-class/style helper, and no object-key logic in the client.

- [ ] **Step 4: Run focused and full miniapp tests**

```bash
node src/utils/classroomDisplay.test.mjs
node src/pages/classroom/classroom.test.mjs
node src/pages/classroom-detail/classroom-detail.test.mjs
node src/pages/learn/learn.content-state.test.mjs
npm run test:config
```

- [ ] **Step 5: Commit Task 6**

```bash
git add miniapp/src/api/index.js \
  miniapp/src/utils/classroomDisplay.js \
  miniapp/src/utils/classroomDisplay.test.mjs \
  miniapp/src/pages/classroom/classroom.vue \
  miniapp/src/pages/classroom/classroom.test.mjs \
  miniapp/src/pages/classroom-detail/classroom-detail.vue \
  miniapp/src/pages/classroom-detail/classroom-detail.test.mjs \
  miniapp/src/pages/learn/learn.vue \
  miniapp/src/pages/learn/learn.content-state.test.mjs
git commit -m "feat: display classroom cover ratios"
```

## Chunk 4: Integrated verification

### Task 7: Verify the full feature without changing publication state

**Files:**
- Modify only if a regression test exposes a defect in the files above.

- [ ] **Step 1: Run backend verification**

```bash
cd nx-backend/apps/server
go test ./internal/db ./internal/classroom ./internal/server -count=1
```

- [ ] **Step 2: Run admin verification**

```bash
cd nx-backend
pnpm exec vitest run apps/web-antd/src/views/classroom/classroom.test.ts apps/web-antd/src/views/classroom/classroom.integration.test.ts
pnpm --filter @vben/web-antd typecheck
```

- [ ] **Step 3: Run miniapp verification**

```bash
cd miniapp
npm run test:config
```

- [ ] **Step 4: Run formatting and diff checks**

```bash
gofmt -w \
  nx-backend/apps/server/internal/classroom/models.go \
  nx-backend/apps/server/internal/classroom/store.go \
  nx-backend/apps/server/internal/classroom/cover.go \
  nx-backend/apps/server/internal/classroom/cover_service.go \
  nx-backend/apps/server/internal/classroom/upload.go \
  nx-backend/apps/server/internal/server/classroom_admin.go \
  nx-backend/apps/server/internal/server/classroom_public.go \
  nx-backend/apps/server/internal/server/classroom_progress.go \
  nx-backend/apps/server/internal/server/classroom_audio_cover.go \
  nx-backend/apps/server/internal/server/server.go \
  nx-backend/apps/server/internal/config/env.go
git diff --check
git status --short
```

Review `git diff` line by line. Confirm no unrelated pre-existing edits were overwritten or accidentally staged and no course/series publication endpoint was called.

- [ ] **Step 5: Manual local smoke test**

Using the already running non-Docker backend/admin environment:

1. Confirm the Vite process is listening on port 5666, then open `http://127.0.0.1:5666/classroom`.
2. Create or edit a draft video content and confirm default `16:9`.
3. Select each ratio and verify preview dimensions.
4. Upload a JPEG/PNG/WebP and confirm source is “手动上传”.
5. Delete it and confirm immediate fallback to “视频首帧”.
6. Open an audio item with no manual cover and confirm the default audio cover.
7. Do not publish or offline any content.

- [ ] **Step 6: Final review and optional fix commit**

Dispatch a final spec reviewer and code-quality reviewer. If verification fixes are needed, repeat RED/GREEN for each defect and commit only those fixes:

```bash
git commit -m "fix: complete classroom cover workflow"
```
