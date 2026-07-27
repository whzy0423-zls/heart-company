# 小程序老师课堂实施计划

> **For agentic workers:** REQUIRED: Use superpowers:subagent-driven-development (if subagents available) to execute this plan. Steps use checkbox (`- [ ]`) syntax.

**Goal:** 建立面向小程序用户的“老师课堂”视频/音频内容链路，支持系列与独立内容、分片上传、发布、权限、系列/单课购买、签名播放和基础继续学习。

**Architecture:** 新增独立 classroom 领域，不复用站点 JSON、视频制作资产库或 20MiB 通用上传。媒体文件通过 OSS multipart 直传，数据库只存媒体元数据；后台通过独立权限管理内容，小程序通过公开元数据 API 和可选匿名/JWT 播放鉴权消费内容。

**Tech Stack:** Go server、PostgreSQL schema/store、现有 OSS storage、现有微信支付/orders 回调、Vue 3 + Ant Design Vue 后台、uni-app/Vue3 小程序、Node assertion tests、Vitest、Go tests。

**Spec:** `docs/superpowers/specs/2026-07-26-miniapp-teacher-classroom-design.md`

---

## Chunk 1: 基础模型、会员字段与媒体存储边界

### Task 1: 新增 classroom 数据模型与迁移

**Files:**
- Create: `nx-backend/apps/server/internal/classroom/models.go`
- Create: `nx-backend/apps/server/internal/classroom/store.go`
- Create: `nx-backend/apps/server/internal/classroom/store_test.go`
- Modify: `nx-backend/apps/server/internal/db/schema.sql`
- Create: `nx-backend/apps/server/internal/db/schema_classroom_test.go`
- Modify: `nx-backend/apps/server/internal/db/schema_order_test.go`

- [ ] **Step 1: Write failing model/store tests**
  - Cover series/content status transitions, `show_as_standalone`, `access_level=inherit`, price validation, `playback_blocked`, media-asset metadata, upload-task unique draft binding, and series/content entitlement target exclusivity.
  - Cover content publish rejection unless media is ready and parent series is published.
- [ ] **Step 2: Run RED**
  - Run `cd nx-backend/apps/server && go test ./internal/classroom ./internal/db`.
  - Expected: package/schema symbols and tables are missing.
- [ ] **Step 3: Add schema and domain types**
  - Add `classroom_series`, `classroom_contents`, `classroom_media_assets`, `classroom_upload_tasks`, `classroom_entitlements`, `classroom_progress`.
  - Use `teacher_key` + `teacher_name_snapshot` only; no `teacher_id` in the first migration.
  - Add checks/indexes for status, access, price, sort order, expiry, target exclusivity, and `content_id` upload-task uniqueness.
  - Keep media object key/size/checksum in `classroom_media_assets`; do not reference `upload_assets.data` for long media.
- [ ] **Step 4: Implement store methods minimally**
  - Add create/update/get/list methods needed by later HTTP handlers.
  - Enforce state transitions and optimistic update timestamps in the store/service boundary.
- [ ] **Step 5: Run GREEN and commit**
  - Run `go test ./internal/classroom ./internal/db`.
  - Commit: `feat: add classroom content persistence`

### Task 2: Make miniapp membership expiry authoritative

**Files:**
- Modify: `nx-backend/apps/server/internal/db/schema.sql`
- Modify: `nx-backend/apps/server/internal/miniapp/orders.go`
- Modify: `nx-backend/apps/server/internal/miniapp/orders_test.go`
- Modify: `nx-backend/apps/server/internal/server/payment_handlers.go`
- Create: `nx-backend/apps/server/internal/server/miniapp_membership_test.go`
- Modify: `nx-backend/apps/server/internal/miniapp/service.go`

- [ ] **Step 1: Add failing expiry tests**
  - Cover `wx_users.member_started_at/member_expires_at`, legacy lifetime membership (`expires_at IS NULL`), expired member rejection, renewal, duplicate callback idempotency, and refund/revoke.
- [ ] **Step 2: Run RED**
  - Run `go test ./internal/miniapp ./internal/server -run 'Member|Membership|Order'`.
- [ ] **Step 3: Add fields and callback writes**
  - Extend the existing miniapp `member` order branch reached through `payment_handlers.go → miniapp.MarkOrderPaid` to update level and validity atomically.
  - Make legacy member callbacks idempotent and define refund/revoke behavior here; Task 7 must reuse this callback/service boundary for classroom products rather than add a second callback path.
  - Keep existing membership source authoritative; classroom entitlement code reads these fields instead of duplicating member rows.
- [ ] **Step 4: Run GREEN and commit**
  - Run focused miniapp/server tests plus `go test ./internal/db`.
  - Commit: `feat: track miniapp membership validity`

---

## Chunk 2: Multipart media upload and validation

### Task 3: Add OSS multipart classroom upload service

**Files:**
- Modify: `nx-backend/apps/server/internal/storage/storage.go`
- Create: `nx-backend/apps/server/internal/classroom/upload.go`
- Create: `nx-backend/apps/server/internal/classroom/upload_test.go`
- Modify: `nx-backend/apps/server/internal/server/server.go`
- Create: `nx-backend/apps/server/internal/server/classroom_upload.go`
- Create: `nx-backend/apps/server/internal/server/classroom_upload_test.go`
- Modify: `nx-backend/apps/server/internal/config/env.go`
- Modify: `nx-backend/apps/server/internal/config/env_test.go`
- Create: `docs/deployment/classroom-media.md`

- [ ] **Step 1: Write failing upload tests**
  - Cover initiate, part-sign, complete, abort, ownership, server-generated object prefix, expected-size/checksum/ETag validation, duplicate complete, expired task, retry count, and permission rejection.
  - Use a fake multipart signer/uploader; never require real OSS in unit tests.
- [ ] **Step 2: Run RED**
  - Run `go test ./internal/classroom ./internal/server -run 'ClassroomUpload|Multipart'`.
- [ ] **Step 3: Implement storage interface**
  - Add multipart initiate/sign/complete/abort/list/head abstractions while preserving existing `ObjectUploader` callers.
  - Add explicit endpoint/bucket/region/part-size/max-parts/credential-TTL/media-limit configuration.
  - Treat OSS Bucket CORS as deployment configuration: document allowed origins/methods/headers/exposed ETag and verification steps in `docs/deployment/classroom-media.md`, not as a secret or runtime service credential field.
- [ ] **Step 4: Implement classroom upload handlers**
  - Use `/api/admin/classroom/uploads/...` routes and classroom upload permission only.
  - Bind each task to one content draft; make complete and abort idempotent; store only metadata.
- [ ] **Step 5: Add media validation**
  - Validate MP4 H.264/AAC and MP3/M4A AAC using a testable media-probe boundary; write duration/size/checksum/cover and transition to `ready` or `failed`.
- [ ] **Step 6: Run GREEN and commit**
  - Run focused Go tests and `go test ./internal/storage ./internal/classroom ./internal/server`.
  - Commit: `feat: add classroom multipart media uploads`

---

## Chunk 3: Admin permissions, APIs and management UI

### Task 4: Add classroom permissions, menu seeds and admin API

**Files:**
- Modify: `nx-backend/apps/server/internal/db/db.go`
- Modify: `nx-backend/apps/server/internal/db/menu_test.go`
- Modify: `nx-backend/apps/server/internal/system/menu_routes_test.go`
- Modify: `nx-backend/apps/server/internal/server/miniapp_admin.go`
- Modify: `nx-backend/apps/server/internal/server/server.go` (register admin routes)
- Create: `nx-backend/apps/server/internal/server/classroom_admin.go`
- Create: `nx-backend/apps/server/internal/server/classroom_admin_test.go`
- Create: `nx-backend/apps/web-antd/src/api/core/classroom.ts`

- [ ] **Step 1: Write failing permission/API contract tests**
  - Require `Miniapp:Classroom:List/Write/Upload/Publish/Price` menu/role seeds.
  - Cover actual HTTP route registration, list filters, safe metadata, series CRUD, content draft CRUD, publish/offline, price validation, and audit records.
- [ ] **Step 2: Run RED**
  - Run `go test ./internal/db ./internal/server -run 'Classroom|Menu'`. The admin UI Vitest contract is created and run in Task 5.
- [ ] **Step 3: Implement admin routes**
  - Add paginated series/content/task endpoints with server-side permission checks.
  - Ensure paid content returns safe metadata for discovery while media URLs remain protected.
  - Enforce `inherit` resolution, `show_as_standalone`, series publish prerequisite, normal offline vs `playback_blocked`, and CNY price constraints.
- [ ] **Step 4: Add admin API client types**
  - Define request/response types, upload task progress, content/series payloads, and error normalization.
- [ ] **Step 5: Run GREEN and commit**
  - Commit: `feat: add classroom admin permissions and APIs`

### Task 5: Build admin classroom management UI

**Files:**
- Create: `nx-backend/apps/web-antd/src/router/routes/modules/classroom.ts`
- Create: `nx-backend/apps/web-antd/src/views/classroom/index.vue`
- Create: `nx-backend/apps/web-antd/src/views/classroom/series.vue`
- Create: `nx-backend/apps/web-antd/src/views/classroom/upload-tasks.vue`
- Create: `nx-backend/apps/web-antd/src/views/classroom/components/content-editor.vue`
- Create: `nx-backend/apps/web-antd/src/views/classroom/classroom.test.ts`

- [ ] **Step 1: Write failing UI contract tests**
  - Require tabs for content/series/upload tasks, video/audio type selector, series/standalone selector, permission/price controls, upload progress/retry, draft/publish/offline actions, and playback-blocked control.
  - Require route authority metadata and independent Upload/Publish/Price button access codes; unauthorized controls must be hidden or disabled.
  - Validate `show_as_standalone=true + inherit→paid` has an explicit “购买系列” CTA strategy or requires a single-course paid override before publication.
- [ ] **Step 2: Run RED**
  - Run `pnpm exec vitest run apps/web-antd/src/views/classroom/classroom.test.ts`.
- [ ] **Step 3: Implement list and editor**
  - Follow existing site-config/editor-shell and upload/image component patterns without reusing the video-production asset editor.
  - Make paid metadata discoverable, but hide protected media object keys.
- [ ] **Step 4: Implement upload task UI**
  - Show initiate/progress/complete/processing/ready/failed states, retry and abort, and preserve task association to the draft.
- [ ] **Step 5: Run GREEN and commit**
  - Run focused Vitest and format check.
  - Commit: `feat: add classroom admin management UI`

---

## Chunk 4: Public APIs, purchase and playback authorization

### Task 6: Add public listing/detail and optional-auth playback API

**Files:**
- Create: `nx-backend/apps/server/internal/server/classroom_public.go`
- Create: `nx-backend/apps/server/internal/server/classroom_public_test.go`
- Modify: `nx-backend/apps/server/internal/server/server.go` (register public/play routes)
- Modify: `nx-backend/apps/server/internal/server/context.go`
- Modify: `nx-backend/apps/server/internal/server/rate_limiter.go`
- Modify: `nx-backend/apps/server/internal/httpx/response.go` if needed

- [ ] **Step 1: Write failing API tests**
  - Cover actual HTTP route registration, series/standalone lists, detail, effective access, paid cards visible with `canPlay=false`, ETag/Vary behavior, published/media filtering, series offline + standalone semantics, and `playback_blocked`.
  - Cover anonymous ticket claims, five-minute TTL, IP/device rate limit, JWT path, expired signature and cross-content replay rejection.
- [ ] **Step 2: Run RED**
  - Run `go test ./internal/server -run 'ClassroomPublic|ClassroomPlayback'`.
- [ ] **Step 3: Implement public metadata endpoints**
  - Add pagination/sorting/filtering, cache invalidation hooks, and explicit user-state separation.
- [ ] **Step 4: Implement optional-auth playback**
  - Add middleware that accepts anonymous signed tickets or miniapp JWT; do not apply current mandatory `requireMiniapp` blindly.
  - Return short-lived signed OSS/CDN URL only after access checks; support Range/206 through storage/CDN.
- [ ] **Step 5: Run GREEN and commit**
  - Commit: `feat: add classroom public playback authorization`

### Task 7: Extend existing order flow for classroom purchases

**Files:**
- Modify: `nx-backend/apps/server/internal/miniapp/orders.go`
- Modify: `nx-backend/apps/server/internal/miniapp/orders_test.go`
- Modify: `nx-backend/apps/server/internal/server/payment_handlers.go`
- Create: `nx-backend/apps/server/internal/server/classroom_orders.go`
- Create: `nx-backend/apps/server/internal/server/classroom_orders_test.go`
- Modify: `nx-backend/apps/server/internal/server/server.go` (register order routes)
- Create: `nx-backend/apps/server/internal/classroom/entitlements.go`
- Create: `nx-backend/apps/server/internal/classroom/entitlements_test.go`

- [ ] **Step 1: Write failing purchase tests**
  - Cover actual POST create/GET status routes, series and single-content order creation, amount/title snapshots, one pending order per target, dev/production payment parameter boundaries, callback idempotency, entitlement issuance, future series lessons, moved-out lessons, refunds, manual grants, price changes, and member access.
- [ ] **Step 2: Run RED**
  - Run `go test ./internal/miniapp ./internal/server ./internal/classroom -run 'ClassroomOrder|Entitlement|Payment'`.
- [ ] **Step 3: Reuse `orders.product`**
  - Add classroom product values to current order handlers; validate `ref_id` target type and sale state; do not create a parallel payment system.
- [ ] **Step 4: Add transactional entitlement issuance**
  - Process successful callback once, issue series/content entitlement, preserve order snapshots, and handle revoke/refund audit.
- [ ] **Step 5: Run GREEN and commit**
  - Commit: `feat: support classroom series and lesson purchases`

---

## Chunk 5: Miniapp classroom experience and progress

### Task 8: Add miniapp classroom APIs and state utilities

**Files:**
- Modify: `miniapp/src/api/index.js`
- Modify: `miniapp/src/api/index.test.mjs`
- Modify: `miniapp/package.json` (register focused tests in `test:config`)
- Create: `miniapp/src/utils/classroomDisplay.js`
- Create: `miniapp/src/utils/classroomDisplay.test.mjs`
- Create: `miniapp/src/utils/classroomProgress.js`
- Create: `miniapp/src/utils/classroomProgress.test.mjs`
- Create: `docs/superpowers/fixtures/classroom-public-response.json` (shared API fixture)

- [ ] **Step 1: Write failing utility/API tests**
  - Cover safe metadata normalization, effective access labels, purchase state, content-type routing, anonymous/JWT playback response, expired URL retry, local anonymous progress, and throttled logged-in progress updates.
- [ ] **Step 2: Run RED**
  - Run `node src/api/index.test.mjs`, `node src/utils/classroomDisplay.test.mjs`, and `node src/utils/classroomProgress.test.mjs`.
- [ ] **Step 3: Add API methods**
  - Add list/detail/play/order/progress/continue-learning methods with `userErrorMessage` normalization and no permanent object URL exposure.
- [ ] **Step 4: Implement pure display/progress helpers**
  - Keep access/CTA logic pure; use 90% server completion semantics; store anonymous progress locally only.
- [ ] **Step 5: Run GREEN and commit**
  - Add `classroomDisplay.test.mjs` and `classroomProgress.test.mjs` to `miniapp/package.json` `test:config`.
  - Commit: `feat: add miniapp classroom data layer`

### Task 9: Add classroom list, detail and playback pages

**Files:**
- Modify: `miniapp/src/pages/learn/learn.vue`
- Create: `miniapp/src/pages/classroom/classroom.vue`
- Create: `miniapp/src/pages/classroom-detail/classroom-detail.vue`
- Create: `miniapp/src/pages/classroom/classroom.test.mjs`
- Create: `miniapp/src/pages/classroom-detail/classroom-detail.test.mjs`
- Modify: `miniapp/src/pages.json`
- Modify: `miniapp/scripts/project-config.test.mjs`
- Modify: `miniapp/scripts/ui-compat.test.mjs`
- Modify: `miniapp/package.json` (register page tests in `test:config`)

- [ ] **Step 1: Write failing page-contract tests**
  - Require dual entry tabs, series/standalone lists, safe loading/empty/error/retry states, video/audio player branches, permission CTA, signed URL refresh/retry, and no direct OSS URL.
  - Leave purchase pending/success/failure and continue-learning/progress contracts to Task 10.
- [ ] **Step 2: Run RED**
  - Run focused page tests and project/UI contract tests.
- [ ] **Step 3: Add routes/subpackage placement**
  - Keep tab pages in the main package; place both `pages/classroom/classroom` and `pages/classroom-detail/classroom-detail` in classroom subpackages and assert both exact routes in `project-config.test.mjs`.
- [ ] **Step 4: Implement learn integration**
  - Execute the compatibility mapping in Task 11 before this step, or keep Task 9 blocked until Task 11 is merged; do not build classroom fallback behavior on the old teacher field mismatch.
  - Preserve existing cached teacher/course/quote behavior; add classroom loading as a separate non-blocking section with retry.
  - Keep existing `home.courses` as compatibility fallback while classroom API is empty or unavailable.
- [ ] **Step 5: Implement details and playback**
  - Use video player for video and page audio player for audio; support play/pause/seek, permission/paywall CTA, signed URL refresh, and failure retry.
- [ ] **Step 6: Run GREEN and commit**
  - Register `classroom.test.mjs` and `classroom-detail.test.mjs` in `miniapp/package.json`, run `npm run test:config`, and commit: `feat: add miniapp teacher classroom experience`

### Task 10: Add classroom progress and order UX polish

**Files:**
- Modify: `miniapp/src/pages/learn/learn.vue`
- Modify: `miniapp/src/pages/classroom/classroom.vue`
- Modify: `miniapp/src/pages/classroom-detail/classroom-detail.vue`
- Modify: `miniapp/src/utils/classroomProgress.js`
- Modify: `miniapp/src/pages/classroom-detail/classroom-detail.test.mjs`
- Modify: `miniapp/scripts/ui-compat.test.mjs`
- Modify: `miniapp/package.json` (register progress/order test)

- [ ] **Step 1: Write failing progress/order UI tests**
  - Cover continue-learning card, 10–15 second progress throttling, pause/unload flush, 90% completion display, order pending/success/failure, and retry.
- [ ] **Step 2: Run RED**
  - Run focused classroom detail tests.
- [ ] **Step 3: Implement minimal progress/order states**
  - Keep anonymous progress local; send logged-in progress through the server endpoint; never trust client completion percentage.
  - Render continue-learning on the classroom list and optionally surface a compact entry from the learning page; keep payment states on the detail page.
- [ ] **Step 4: Run GREEN and commit**
  - Register the progress/order test in `miniapp/package.json`, run `npm run test:config`, and commit: `feat: add classroom progress and purchase states`

---

## Chunk 6: Compatibility, audit and release verification

### Task 11: Repair existing teacher/course compatibility and add end-to-end contracts

**Execution dependency:** Execute this task immediately after Task 8 and before Task 9. It is grouped here for audit documentation only; Task 9 remains blocked until the compatibility commit is merged.

**Files:**
- Modify: `miniapp/src/utils/siteConfig.js`
- Modify: `miniapp/src/utils/teacherCourseware.js`
- Modify: `miniapp/src/utils/siteConfig.test.mjs`
- Modify: `miniapp/src/utils/teacherCourseware.test.mjs`
- Modify: `nx-backend/apps/web-antd/src/views/site-config/teacher.vue`
- Modify: `nx-backend/apps/web-antd/src/views/site-config/courses.vue`
- Modify: `nx-backend/apps/web-antd/src/views/miniapp/home.test.ts`
- Create: `nx-backend/apps/web-antd/src/views/site-config/teacher.test.ts`
- Create: `nx-backend/apps/web-antd/src/views/site-config/courses.test.ts`
- Create: `nx-backend/apps/server/internal/server/classroom_vertical_contract_test.go`

- [ ] **Step 1: Write failing compatibility tests**
  - Cover `home.teacherTeaser → teacher/teachers` mapping with explicit rules: `image/fallbackImage → avatar`, `lead → bio`, combined `title → teacher.name`, `eyebrow → teacher.title` with `九型老师` fallback; structured `teacher/teachers` wins when both exist.
  - Cover legacy home course fallback, independent classroom content not overwriting course products, old JSON round-trip preservation, and teacher/course editor labels separating course products from classroom content.
  - Add a Go vertical contract for admin publish → public list/detail safe metadata → optional-auth play authorization.
  - Make Node `classroomDisplay.test.mjs` consume the shared JSON fixture to verify the miniapp normalization side of the contract.
- [ ] **Step 2: Run RED**
  - Run focused Node/Vitest tests.
- [ ] **Step 3: Implement narrow compatibility mapping**
  - Do not merge classroom content into course-direction configuration; only map legacy fields and label course management clearly.
  - Make structured teacher fields authoritative; use teaser mapping only when structured fields are absent, assigning the combined legacy title to the existing `teacher.name` field rather than introducing `displayName`.
- [ ] **Step 4: Run GREEN and commit**
  - Commit: `fix: bridge legacy teacher and course configuration`

### Task 12: Full verification and release checklist

**Files:**
- Modify: `miniapp/package.json` if new tests need registration
- Modify: `nx-backend/apps/server/internal/db/schema_*_test.go` as needed, including `schema_membership_test.go`
- Create: `docs/superpowers/plans/2026-07-26-miniapp-teacher-classroom-verification.md` only if release checklist needs a durable artifact

- [ ] **Step 1: Run miniapp suites**
  - `cd miniapp && npm run test:config`
  - Build locally with `VITE_API_BASE=http://127.0.0.1:5320/api npm run dev:mp-weixin` and inspect generated `app.json`, routes, classroom assets, and player pages.
  - Run `VITE_API_BASE=<REAL_HTTPS_API_BASE>/api npm run build:mp-weixin` using the repository's real compliant HTTPS production base (never `example.com`, localhost, or a private IP), then inspect production `app.json`, `subPackages`, player routes, package sizes, and absence of permanent OSS URLs.
- [ ] **Step 2: Run backend suites**
  - `cd nx-backend && pnpm exec vitest run apps/web-antd/src/views/miniapp/home.test.ts apps/web-antd/src/views/classroom/classroom.test.ts apps/web-antd/src/views/site-config/teacher.test.ts apps/web-antd/src/views/site-config/courses.test.ts`
  - Run the repository web-app typecheck command and production Ant Design app build (`pnpm run check:type --filter=@vben/web-antd` / `pnpm run build:antd`, adjusted to actual package scripts).
  - `cd nx-backend/apps/server && go test ./...`
  - Run formatting checks for changed Vue/TS/Go files and `git diff --check`.
- [ ] **Step 3: Run scenario matrix**
  - Upload success/failure/abort/expiry; invalid media; draft/publish/offline/hard-stop; anonymous/login/member/paid; series/single purchase; callback duplicate/refund; signed URL expiry; Range/seek; progress throttling; cache invalidation; permission denial.
  - Preserve current regression flows: WeChat login/expired token, test→result→report, relation analysis, booking draft/submit/records/detail, profile editing, homepage config, learning cache silent refresh, and legacy course fallback.
  - Run `go test ./internal/server -run ClassroomVerticalContract` explicitly.
- [ ] **Step 4: Final review and commit**
  - Review database migration rollback, OSS orphan cleanup, audit records, secrets/CORS, and no permanent media URL leakage.
  - Run `git status --short`, conflict-marker scan, and all required suites before handoff.

---

## Execution Notes

- Required task order is `1 → 2 → 3 → 4 → 5 → 6 → 7 → 8 → 11 → 9 → 10 → 12`; Task 11 must land before the learning-page integration.
- Use `superpowers:test-driven-development` for every behavior change.
- Use `superpowers:subagent-driven-development` with a fresh implementer per task, followed by spec and code-quality review.
- Do not implement tasks in parallel when they touch the same schema/API files; parallelize only independent audits or UI/API tasks after their contracts are stable.
- Preserve the existing 20MiB `/api/upload` path for images/small files; classroom media uses the new multipart path.
- Before implementing paid classroom content, confirm the existing WeChat payment callback extension points and migration strategy in Task 7.
