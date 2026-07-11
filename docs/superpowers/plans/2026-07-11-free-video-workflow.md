# Free Video Workflow Implementation Plan

> **For agentic workers:** REQUIRED: Use superpowers:subagent-driven-development (if subagents available) or superpowers:executing-plans to implement this plan. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Make the real guided video workflow complete locally by default while proving that no paid video-provider request occurs.

**Architecture:** Add a fail-closed generation mode to server configuration and branch inside the shared video store before any gateway call. Demo mode renders and uploads valid local MP4 files, preserves request-key/version semantics, and exposes its mode through workflow status. Extend the composer to resolve authenticated local upload URLs directly from disk, then verify the full browser flow with provider-host network rejection.

**Tech Stack:** Go 1.22, PostgreSQL/database/sql, FFmpeg/ffprobe, existing local object uploader, Vue 3, TypeScript, Vitest, Playwright.

---

## File Structure

- Modify `nx-backend/apps/server/internal/config/env.go`: fail-closed video generation mode parsing.
- Modify `nx-backend/apps/server/internal/config/env_test.go`: default, malformed, and explicit paid-mode contracts.
- Modify `nx-backend/apps/server/.env.example`: document demo default and paid acknowledgement.
- Create `nx-backend/apps/server/internal/video/demo.go`: FFmpeg demo renderer and metadata.
- Create `nx-backend/apps/server/internal/video/demo_test.go`: real MP4 renderer checks.
- Modify `nx-backend/apps/server/internal/video/video.go`: mode gate, demo persistence, zero-gateway path, mode accessor.
- Modify `nx-backend/apps/server/internal/video/video_test.go`: idempotent demo generation and paid regression tests.
- Modify `nx-backend/apps/server/internal/video/submission.go`: demo-only proven-failure cancellation helper.
- Modify `nx-backend/apps/server/internal/video/submission_test.go`: cancellation cannot weaken paid transitions.
- Modify `nx-backend/apps/server/internal/videoproject/generator.go`: skip public reference validation in demo mode.
- Modify `nx-backend/apps/server/internal/videoproject/composer.go`: traversal-safe local upload source resolution.
- Modify `nx-backend/apps/server/internal/videoproject/projectcomposer.go`: pass upload root into composer.
- Modify `nx-backend/apps/server/internal/videoproject/projectcomposer_test.go`: compose real local demo MP4 files.
- Modify `nx-backend/apps/server/internal/server/videoproject_routes.go`: inject upload root and expose workflow generation mode.
- Modify `nx-backend/apps/server/internal/server/video_workflow_test.go`: workflow mode response contract.
- Modify `nx-backend/apps/web-antd/src/api/core/videoproject.ts`: workflow generation mode type.
- Modify `nx-backend/apps/web-antd/src/views/video/projects/workflow.vue`: pass mode into generation step.
- Modify `nx-backend/apps/web-antd/src/views/video/projects/workflow/GenerationStep.vue`: visible demo/paid safety banner.
- Modify frontend source/API tests for the new contract.
- Modify `nx-backend/apps/web-antd/e2e/guided-video-workflow.spec.ts`: assert demo banner and reject provider network.
- Create `docs/verification/2026-07-11-free-video-workflow.md`: zero-charge closure evidence.

## Chunk 1: Fail-Closed Mode And Local Generation

### Task 1: Add the fail-closed server configuration

- [ ] Write failing table tests requiring default/malformed/incomplete configuration to produce `demo`, and only `paid` plus `ALLOW_PAID_VIDEO_GENERATION` to produce `paid`.
- [ ] Run `cd nx-backend/apps/server && go test ./internal/config -run TestVideoGenerationMode -count=1` and confirm RED.
- [ ] Add `Mode` to `VideoConfig`, parse both environment variables, and preserve the field through `modelconfig.Config.ApplyVideo`.
- [ ] Add `.env.example` documentation with demo defaults and the exact paid acknowledgement.
- [ ] Run the focused config/modelconfig tests and confirm GREEN.
- [ ] Commit as `feat(video): default generation to free demo mode`.

### Task 2: Render a valid local demo MP4

- [ ] Write a failing renderer test that invokes the renderer, checks non-empty MP4 output, and verifies dimensions/duration with ffprobe.
- [ ] Run `go test ./internal/video -run TestFFmpegDemoRenderer -count=1` and confirm RED.
- [ ] Implement a `DemoRenderer` interface and FFmpeg implementation using local lavfi color input, H.264, yuv420p, no network input, aspect-aware dimensions, and bounded context.
- [ ] Run the renderer test and confirm GREEN.
- [ ] Commit as `feat(video): render local demo videos`.

### Task 3: Complete demo submissions without a gateway call

- [ ] Write failing tests for default demo generation, same-key reuse, local upload URL, completed status/provider, zero `CreateTask` calls, and proven local failure lock release.
- [ ] Run focused video tests and confirm RED.
- [ ] Branch inside `video.Store.Generate` before `CreateTask/CreateTaskOnce`; render and upload the MP4, insert a completed `provider='demo'` generation, attach the submission, and synchronize terminal status.
- [ ] Add a narrowly scoped demo cancellation repository method used only before any external operation; keep generic paid transitions unchanged.
- [ ] Expose `GenerationMode()` and skip gateway public-reference validation from `videoproject.Generator` only in demo mode.
- [ ] Run all `internal/video` and `internal/videoproject` tests.
- [ ] Commit as `feat(video): complete demo generation locally`.

## Chunk 2: Real Local Composition And User Signalling

### Task 4: Compose local upload URLs without HTTP loopback

- [ ] Write failing tests for `/api/uploads/video/generated/demo.mp4`, traversal rejection, missing file, and real FFmpeg composition output.
- [ ] Run focused composer tests and confirm RED.
- [ ] Add `uploadRoot` to `Composer`; resolve only normalized `/api/uploads/` paths beneath the root and copy them into the temporary workspace. Keep existing public HTTP behavior unchanged.
- [ ] Pass `env.UploadDir` through `NewProjectComposer` from server routes.
- [ ] Run all project composer tests and confirm GREEN.
- [ ] Commit as `feat(video): compose local demo artifacts`.

### Task 5: Expose and display the free mode

- [ ] Write failing backend response and frontend source/API tests for `generationMode` and the exact no-charge banner.
- [ ] Run focused Go and Vitest commands and confirm RED.
- [ ] Add the workflow response field, typed client contract, mode prop, and persistent banner. Do not add a client-side paid toggle.
- [ ] Run focused tests and typecheck.
- [ ] Commit as `feat(video): show free workflow mode`.

## Chunk 3: Closure Verification

### Task 6: Prove the browser flow makes no paid call

- [ ] Extend the Playwright fixture to return `generationMode: 'demo'`, require the banner, and abort/fail any request whose host/path matches configured video provider creation endpoints.
- [ ] Run the mandatory Playwright command and confirm all scenarios pass.
- [ ] Start the real backend against an isolated test database/upload directory with demo mode and a provider URL pointing to a tripwire HTTP server.
- [ ] Exercise create project, script import, local batch generation, explicit selection, compose polling, and final video retrieval through HTTP; assert tripwire request count is zero and ffprobe accepts generated/final artifacts.
- [ ] Run full Go tests, full frontend Vitest, typecheck, build, and Playwright.
- [ ] Record exact evidence and environment limitations in `docs/verification/2026-07-11-free-video-workflow.md`.
- [ ] Commit verification as `test(video): prove zero-charge workflow closure` and `docs(video): record free workflow verification`.

## Completion Gate

The work is complete only when the real server defaults to demo mode, a tripwire provider receives zero requests, valid local shot MP4 files are explicitly selected, FFmpeg produces a valid final MP4, browser preview/export paths work, and all full suites pass.
