# Guided Video Production Workbench Implementation Plan

> **For agentic workers:** REQUIRED: Use superpowers:subagent-driven-development (if subagents available) or superpowers:executing-plans to implement this plan. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Replace the crowded default video workbench with a beginner-friendly five-step workflow while preserving the existing advanced workbench and making paid generation, version selection, stale-result detection, and composition recoverable and safe.

**Architecture:** Preserve the information architecture and feature surface of `projects/workbench.vue` as the advanced route, changing only its paid-generation request-key contract, and mount a new modular `projects/workflow.vue` at the default route. Add backward-compatible project/shot/generation/compose revision fields, an idempotent paid-submission state machine, workflow status/readiness services, and focused APIs. The Vue container owns requests, polling, dirty navigation, and routing; step components are presentational and communicate through typed props/emits.

**Tech Stack:** Go 1.22, PostgreSQL schema migrations, `database/sql`, existing New API video gateway, Vue 3 `<script setup>`, TypeScript, Vue Router, Ant Design Vue/Vben, Vitest, Playwright/browser inspection.

**Design:** `docs/superpowers/specs/2026-07-11-guided-video-production-workbench-design.md`

---

## File Structure

### Backend safety and workflow domain

- Modify `nx-backend/apps/server/internal/db/schema.sql`: compatible workflow, revision, selection, submission, and compose snapshot fields/indexes.
- Create `nx-backend/apps/server/internal/db/video_workflow_schema_test.go`: static schema contract tests.
- Create `nx-backend/apps/server/internal/video/submission.go`: paid submission states, compare-and-swap transitions, activity lock, reconciliation.
- Create `nx-backend/apps/server/internal/video/submission_test.go`: state machine and idempotency tests.
- Modify `nx-backend/apps/server/internal/video/video.go`: request-key-aware one-shot POST and submission terminal synchronization.
- Modify `nx-backend/apps/server/internal/video/video_test.go`: duplicate/unknown/linkage/reconcile integration tests.
- Modify `nx-backend/apps/server/internal/videoproject/videoproject.go`: project script fields, shot revisions, selected version, source keys.
- Create `nx-backend/apps/server/internal/videoproject/workflow.go`: readiness/step status, script splitting keys, batch import, compose input model.
- Create `nx-backend/apps/server/internal/videoproject/workflow_test.go`: pure and store contract tests.
- Modify `nx-backend/apps/server/internal/videoproject/generator.go`: request key, shot revision snapshot, no selection overwrite.
- Modify `nx-backend/apps/server/internal/videoproject/batchgenerator.go`: exact server-computed `canGenerate` scope.
- Modify `nx-backend/apps/server/internal/videoproject/projectcomposer.go`: selected-version participants, partial acknowledgement, input hash/snapshot.
- Modify `nx-backend/apps/server/internal/server/videoproject_routes.go`: script import, workflow status, safe generation, selection, compose responses.
- Modify `nx-backend/apps/server/internal/server/server.go`: register focused workflow endpoints.
- Create `nx-backend/apps/server/internal/server/video_workflow_test.go`: route/source contract tests.

### Frontend workflow

- Create `nx-backend/apps/web-antd/src/views/video/projects/workflow/workflow.ts`: pure step/readiness/script/CTA helpers.
- Create `nx-backend/apps/web-antd/src/views/video/projects/workflow/workflow.test.ts`: pure helper tests.
- Create `nx-backend/apps/web-antd/src/views/video/projects/workflow/useWorkflowNavigation.ts`: URL step sync and dirty-action queue.
- Create `nx-backend/apps/web-antd/src/views/video/projects/workflow/useWorkflowPolling.ts`: generation/compose polling ownership and cleanup.
- Create `nx-backend/apps/web-antd/src/views/video/projects/workflow/WorkflowStepper.vue`: five real step buttons.
- Create `nx-backend/apps/web-antd/src/views/video/projects/workflow/BriefStep.vue`: project settings and persistent script.
- Create `nx-backend/apps/web-antd/src/views/video/projects/workflow/AssetsStep.vue`: character/scene preparation and asset-library entry.
- Create `nx-backend/apps/web-antd/src/views/video/projects/workflow/StoryboardStep.vue`: compact navigator, editor, script import result.
- Create `nx-backend/apps/web-antd/src/views/video/projects/workflow/GenerationStep.vue`: preflight groups, batch/single submission, advanced settings.
- Create `nx-backend/apps/web-antd/src/views/video/projects/workflow/VersionDrawer.vue`: selected/current/history version actions.
- Create `nx-backend/apps/web-antd/src/views/video/projects/workflow/ExportStep.vue`: include/exclude review, compose progress, current/stale result.
- Create `nx-backend/apps/web-antd/src/views/video/projects/workflow.vue`: data orchestration, dialogs, one-primary-action shell.
- Create `nx-backend/apps/web-antd/src/views/video/projects/workflow.source.test.ts`: route, component, accessibility and responsive source contracts.
- Modify `nx-backend/apps/web-antd/src/api/core/videoproject.ts`: workflow fields and endpoint types.
- Modify `nx-backend/apps/web-antd/src/router/routes/modules/video.ts`: default workflow and `/advanced` route.
- Modify `nx-backend/apps/web-antd/src/views/video/production/index.vue`: creation center, no unavailable short mode.
- Modify `nx-backend/apps/web-antd/src/views/video/production.test.ts`: new entry/route expectations.
- Modify `nx-backend/apps/server/internal/db/db.go`: remove unavailable short mode menu seed and add advanced hidden route.

### Verification

- Create `nx-backend/apps/web-antd/e2e/guided-video-workflow.spec.ts`: mandatory responsive/browser flow with mocked APIs.
- Preserve `artifacts/`; write new screenshots under `artifacts/guided-workflow-*` without staging the directory.

## Chunk 1: Paid Generation Safety and Revision Foundation

### Task 1: Add backward-compatible workflow schema contracts

**Files:**
- Create: `nx-backend/apps/server/internal/db/video_workflow_schema_test.go`
- Modify: `nx-backend/apps/server/internal/db/schema.sql`

- [ ] **Step 1: Write the failing schema test**

Read `schema.sql` and require exact fragments for:

```go
required := []string{
  "script_content TEXT NOT NULL DEFAULT ''",
  "script_revision INT NOT NULL DEFAULT 0",
  "final_video_input_hash TEXT NOT NULL DEFAULT ''",
  "generation_revision INT NOT NULL DEFAULT 0",
  "selected_generation_id BIGINT",
  "source_key TEXT NOT NULL DEFAULT ''",
  "source_script_revision INT NOT NULL DEFAULT 0",
  "sort_order INT NOT NULL DEFAULT 0",
  "shot_revision INT NOT NULL DEFAULT 0",
  "compose_input_hash TEXT NOT NULL DEFAULT ''",
  "compose_input_snapshot JSONB NOT NULL DEFAULT '{}'::jsonb",
  "progress INT NOT NULL DEFAULT 0",
  "CREATE TABLE IF NOT EXISTS video_generation_submissions",
  "request_key UUID NOT NULL UNIQUE",
  "idx_video_generation_submissions_active_shot",
  "idx_video_compose_jobs_active_project",
}
```

Also assert every new column has an `ALTER TABLE ... ADD COLUMN IF NOT EXISTS` path and that `selected_generation_id` is added after `video_generations` exists. The RED contract must require the full FK text `selected_generation_id BIGINT REFERENCES video_generations(id) ON DELETE SET NULL`, `video_shot_assets.sort_order`, `video_compose_jobs.progress`, the partial unique source index `ON video_shots(project_id, source_key) WHERE source_key <> ''`, the compose activity index `ON video_compose_jobs(project_id) WHERE status IN ('queued','processing')`, and a backfill predicate containing successful terminal status plus `video_url <> ''`; a name-only fragment is insufficient. Asset sort backfill is deterministic by `(create_time,id)` within each shot.

- [ ] **Step 2: Run RED**

Run: `cd nx-backend/apps/server && go test ./internal/db -run TestVideoWorkflowSchema -count=1`

Expected: FAIL listing the first missing field.

- [ ] **Step 3: Add compatible schema changes**

Add project/shot/generation/compose fields plus:

```sql
CREATE TABLE IF NOT EXISTS video_generation_submissions (
  id BIGSERIAL PRIMARY KEY,
  request_key UUID NOT NULL UNIQUE,
  shot_id BIGINT NOT NULL REFERENCES video_shots(id) ON DELETE CASCADE,
  generation_id BIGINT REFERENCES video_generations(id) ON DELETE SET NULL,
  task_id TEXT NOT NULL DEFAULT '',
  status TEXT NOT NULL DEFAULT 'prepared',
  request_snapshot JSONB NOT NULL DEFAULT '{}'::jsonb,
  error_message TEXT NOT NULL DEFAULT '',
  create_time TIMESTAMPTZ NOT NULL DEFAULT now(),
  update_time TIMESTAMPTZ NOT NULL DEFAULT now(),
  CHECK (status IN ('prepared','submitting','accepted','unknown_outcome','reconciled','completed','failed','cancelled'))
);

CREATE UNIQUE INDEX IF NOT EXISTS idx_video_generation_submissions_active_shot
ON video_generation_submissions(shot_id)
WHERE status IN ('prepared','submitting','accepted','unknown_outcome','reconciled');

CREATE UNIQUE INDEX IF NOT EXISTS idx_video_compose_jobs_active_project
ON video_compose_jobs(project_id)
WHERE status IN ('queued','processing');
```

Use a partial unique index for non-empty `(project_id, source_key)`. Backfill `selected_generation_id` only from successful generations with a non-empty video URL.

- [ ] **Step 4: Run GREEN and schema regression**

Run: `cd nx-backend/apps/server && go test ./internal/db -count=1`

Expected: PASS.

- [ ] **Step 5: Commit**

```bash
git add nx-backend/apps/server/internal/db/schema.sql nx-backend/apps/server/internal/db/video_workflow_schema_test.go
git commit -m "feat(video): add guided workflow schema"
```

### Task 2: Implement the paid submission state machine

**Files:**
- Create: `nx-backend/apps/server/internal/video/submission.go`
- Create: `nx-backend/apps/server/internal/video/submission_test.go`

- [ ] **Step 1: Write failing transition and idempotency tests**

Table-test only these transitions:

```go
var allowedSubmissionTransitions = map[string]map[string]bool{
  "prepared":       {"submitting": true, "cancelled": true},
  "submitting":     {"accepted": true, "unknown_outcome": true},
  "unknown_outcome": {"reconciled": true},
  "accepted":       {"completed": true, "failed": true},
  "reconciled":     {"completed": true, "failed": true},
}
```

Generate the Cartesian product of all eight statuses and assert that every edge outside the map is rejected, including cancellation from `submitting/accepted/reconciled/unknown_outcome` and every transition out of terminal states. Test same request key reuse, database-enforced concurrent new-key rejection under the partial unique activity index, terminal release, repeated same-task reconciliation, conflicting task rejection, and reconstructing a new `SubmissionStore` over the same database after a simulated process restart. The restarted store must find the active row, reject a new key, and permit polling/reconciliation with the original key.

- [ ] **Step 2: Run RED**

Run: `cd nx-backend/apps/server && go test ./internal/video -run TestSubmission -count=1`

Expected: FAIL because `SubmissionStore` is undefined.

- [ ] **Step 3: Implement focused store and typed errors**

Create types `Submission`, `PrepareSubmissionInput`, `UnknownOutcomeError`, and methods `Prepare`, `Transition`, `GetByRequestKey`, `FindActiveByShot`, `AttachAccepted`, `Reconcile`. Use SQL compare-and-swap status predicates. Never expose request snapshots to unauthorised routes or logs.

- [ ] **Step 4: Run GREEN**

Run: `cd nx-backend/apps/server && go test ./internal/video -run TestSubmission -count=1`

Expected: PASS.

- [ ] **Step 5: Commit**

```bash
git add nx-backend/apps/server/internal/video/submission.go nx-backend/apps/server/internal/video/submission_test.go
git commit -m "feat(video): persist paid submission states"
```

### Task 3A: Make duplicate and active gateway submission idempotent

**Files:**
- Modify: `nx-backend/apps/server/internal/video/video.go`
- Modify: `nx-backend/apps/server/internal/video/video_test.go`
- Modify: `nx-backend/apps/server/internal/video/submission.go`

- [ ] **Step 1: Write failing duplicate/active integration cases**

Add tests for:

```text
same requestKey twice -> one upstream POST and same generation
different key while active -> zero second POST and conflict
```

- [ ] **Step 2: Run RED**

Run: `cd nx-backend/apps/server && go test ./internal/video -run 'TestGenerateReusesRequestKey|TestGenerateRejectsNewKeyWhileActive' -count=1`

Expected: FAIL because `GenerateInput.RequestKey/ShotID/ShotRevision` and submission wiring do not exist.

- [ ] **Step 3: Extend `GenerateInput` and `Generation`**

Add `RequestKey`, `ShotID`, `ShotRevision`, and submission metadata. Generate must validate UUID/non-empty shot ID for paid project calls, prepare before POST, transition to submitting, perform exactly one `CreateTask`, and attach the task/generation idempotently.

- [ ] **Step 4: Run focused GREEN**

Run the same focused command. Expected: PASS with exactly one upstream POST.

- [ ] **Step 5: Commit**

```bash
git add nx-backend/apps/server/internal/video/video.go nx-backend/apps/server/internal/video/video_test.go nx-backend/apps/server/internal/video/submission.go
git commit -m "feat(video): make paid submission idempotent"
```

### Task 3B: Preserve ambiguous outcomes without a paid retry

**Files:**
- Modify: `nx-backend/apps/server/internal/video/video.go`
- Modify: `nx-backend/apps/server/internal/video/video_test.go`
- Modify: `nx-backend/apps/server/internal/video/submission.go`

- [ ] **Step 1: Write failing ambiguity cases**

Add separate tests for connection interruption, unparseable success response, and HTTP success with an empty `task_id`. Each must assert one upstream POST, persisted `unknown_outcome`, original request-key activity lock, `UnknownOutcomeError{RequestKey, TaskID?}`, and no ordinary retry/new-key path.

- [ ] **Step 2: Run RED**

Run: `cd nx-backend/apps/server && go test ./internal/video -run 'TestGenerateStoresUnknownOutcome|TestGenerateMissingTaskIDIsUnknownOutcome|TestGenerateUnparseableSuccessIsUnknownOutcome' -count=1`

Expected: FAIL because current gateway errors write an ordinary failed generation or return without submission state.

- [ ] **Step 3: Implement one-shot ambiguity handling**

Transition `submitting -> unknown_outcome` for every response where the system cannot prove no paid task was created. Preserve any known task ID. Do not create a failed generation as a substitute and do not release the activity lock.

- [ ] **Step 4: Run GREEN and commit**

Run the same command. Expected: PASS with POST count one in every case.

```bash
git add nx-backend/apps/server/internal/video
git commit -m "feat(video): preserve ambiguous generation outcomes"
```

### Task 3C: Reconcile linkage failures and resume after restart

**Files:**
- Modify: `nx-backend/apps/server/internal/video/video.go`
- Modify: `nx-backend/apps/server/internal/video/video_test.go`
- Modify: `nx-backend/apps/server/internal/video/submission.go`

- [ ] **Step 1: Write failing linkage/reconcile cases**

Test that an upstream task ID followed by local generation/link failure rolls back the failed generation/link transaction, then uses an independent persistent recovery update to store that task ID and CAS `submitting -> unknown_outcome`, returning `UnknownOutcomeError{RequestKey, TaskID}`. Then test same-task reconcile twice creates/reuses one generation and moves `unknown_outcome -> reconciled`; a different task returns `reconciliation_task_conflict`. Recreate the store before reconcile to prove process recovery.

- [ ] **Step 2: Run RED**

Run: `cd nx-backend/apps/server && go test ./internal/video -run 'TestGenerateReturnsTaskIDWhenLocalLinkFails|TestReconcileSubmission|TestReconcileAfterStoreRestart' -count=1`

Expected: FAIL on missing persistent recovery semantics.

- [ ] **Step 3: Implement idempotent reconciliation**

Persist task ID and `unknown_outcome` even when generation linkage fails. `Reconcile(ctx, requestKey, taskID)` creates or reuses the generation in one transaction, rejects conflicting task IDs, links the row, transitions to `reconciled`, and leaves the activity lock held for polling.

- [ ] **Step 4: Run GREEN and commit**

Run the same command. Expected: PASS, one generation per request key, no extra POST.

```bash
git add nx-backend/apps/server/internal/video
git commit -m "feat(video): reconcile generation linkage failures"
```

### Task 3D: Release terminal locks and support an explicit new version

**Files:**
- Modify: `nx-backend/apps/server/internal/video/video.go`
- Modify: `nx-backend/apps/server/internal/video/video_test.go`
- Modify: `nx-backend/apps/server/internal/video/submission.go`

- [ ] **Step 1: Write failing terminal/restart/new-version cases**

Test refresh success/failure transitions accepted/reconciled to completed/failed and releases the partial-unique activity lock. Recreate the store mid-poll and prove it resumes the original generation. After terminal state, submit a new explicit user action with a different UUID and assert exactly one second upstream POST and a distinct generation/version.

- [ ] **Step 2: Run RED**

Run: `cd nx-backend/apps/server && go test ./internal/video -run 'TestSubmissionRefreshTerminal|TestSubmissionPollingAfterRestart|TestGenerateExplicitNewVersionAfterTerminal' -count=1`

Expected: FAIL because refresh does not synchronize submissions or release locks.

- [ ] **Step 3: Synchronize terminal refresh**

On refresh completion/failure, transition linked accepted/reconciled submission to terminal. A polling network error leaves submission active. Never retry POST in the HTTP client.

- [ ] **Step 4: Run GREEN and all video tests**

Run: `cd nx-backend/apps/server && go test ./internal/video -count=1`

Expected: PASS. Duplicate-key, active-lock, ambiguity and reconcile scenarios each have POST count 1. `TestGenerateExplicitNewVersionAfterTerminal` has cumulative POST count 2, one per distinct request key, and two distinct generation IDs.

- [ ] **Step 5: Commit**

```bash
git add nx-backend/apps/server/internal/video/video.go nx-backend/apps/server/internal/video/video_test.go nx-backend/apps/server/internal/video/submission.go
git commit -m "feat(video): reconcile paid generation submissions"
```

## Chunk 2: Workflow Backend and Stable Data Contracts

### Task 4: Persist script, shot revision and explicit version selection

**Files:**
- Modify: `nx-backend/apps/server/internal/videoproject/videoproject.go`
- Modify: `nx-backend/apps/server/internal/videoproject/generator.go`
- Modify: `nx-backend/apps/server/internal/videoproject/batchgenerator.go`
- Modify: `nx-backend/apps/server/internal/videoproject/videoproject_test.go`
- Modify: `nx-backend/apps/server/internal/videoproject/generator_test.go`
- Modify: `nx-backend/apps/server/internal/videoproject/batchgenerator_test.go`

- [ ] **Step 1: Write failing persistence tests**

Test project create/update/get/list round-trips `scriptContent/scriptRevision`, and `scriptRevision` increments only when normalized script content actually changes. Table-test each generation-affecting input independently: action description, dynamic description, character IDs, scene ID, duration, aspect ratio, model, resolution, sound/picture mode, reference asset add/delete/reorder, and project style. Assert same normalized values and cosmetic name changes do not increment. Project style increments every affected project shot; character/scene/reference mutations increment only shots that reference the changed entity.

Test explicit selection separately: generation belongs to the shot; only successful records with a non-empty URL may be selected; a successful old-revision version may be stored but readiness is stale; selecting a current-revision version restores completed; new/failed submissions update compatibility `generation_id` without overwriting `selected_generation_id`; legacy reads may show `generation_id`, but workflow readiness uses only `selected_generation_id`.

Test generation revision snapshots: initial submit copies current `generationRevision` to both submission snapshot and `video_generations.shot_revision`; later shot edits never mutate the saved generation revision; reconciliation after a later edit uses the original submission snapshot revision, not current shot state. Test asset order persists through reload, reads by `sort_order,id`, reorder increments only the owning shot revision, and no-op reorder does not increment.

- [ ] **Step 2: Run RED**

Run: `cd nx-backend/apps/server && go test ./internal/videoproject -run 'TestProjectScriptRevision|TestShotGenerationRevisionTriggers|TestReferencedAssetRevisionScope|TestShotAssetOrderPersistence|TestGenerationRevisionSnapshot|TestReconciledGenerationUsesSubmissionRevision|TestSelectedVersionValidity|TestSelectedVersionCompatibility' -count=1`

Expected: FAIL on missing fields/SQL.

- [ ] **Step 3: Implement fields and update semantics**

Extend Go structs/SQL. Compare normalized generation-affecting values before update; increment revision only when one changes. Update character/scene/reference asset mutations to increment only affected shots. Keep `generation_id` as latest task and `selected_generation_id` as user choice.

- [ ] **Step 4: Pass request key/revision into generator**

Change `GenerateShot(ctx, shotID, requestKey)` and batch calls, with one stable request key per shot. Do not call gateway unless server readiness permits it. `MarkShotGenerating` updates latest `generation_id` without changing selection.

- [ ] **Step 5: Run GREEN**

Run: `cd nx-backend/apps/server && go test ./internal/videoproject -count=1`

Expected: PASS.

- [ ] **Step 6: Commit**

```bash
git add nx-backend/apps/server/internal/videoproject
git commit -m "feat(video): track workflow revisions and selections"
```

### Task 5A: Add deterministic readiness and five-step status

**Files:**
- Create: `nx-backend/apps/server/internal/videoproject/workflow.go`
- Create: `nx-backend/apps/server/internal/videoproject/workflow_test.go`
- Modify: `nx-backend/apps/server/internal/videoproject/videoproject.go`
- Modify: `nx-backend/apps/server/internal/videoproject/batchgenerator.go`
- Modify: `nx-backend/apps/server/internal/videoproject/batchgenerator_test.go`

- [ ] **Step 1: Write failing pure tests**

Cover exact readiness priority and combinations: unknown outcome recovery; every active submission state; linked active task; missing action; selected success missing URL; current-revision successful selection; old-revision stale selection; terminal failure without a valid selection; terminal failure with a valid current selection remains completed; ready.

Table-test every step predicate: brief complete/skipped_existing; assets complete/optional/skipped_existing; storyboard complete/blocked; generate complete/stale; export complete/stale. Cover legacy recommendation for empty, assets-only, shots, generation and final video projects, including no-script legacy content never returning to brief.

- [ ] **Step 2: Write failing server batch-scope tests**

Assert `ready + stale + failed` included and `incomplete + generating + recovery + completed` excluded. When the client supplies extra shot IDs, server submission is the intersection with `CanGenerate=true`; no excluded shot reaches `GenerateShot`.

- [ ] **Step 3: Run RED**

Run: `cd nx-backend/apps/server && go test ./internal/videoproject -run 'TestShotReadiness|TestWorkflowStepPredicates|TestRecommendedWorkflowStep|TestBatchGenerationReadinessScope' -count=1`

Expected: FAIL because workflow helpers and server scope do not exist.

- [ ] **Step 4: Implement pure contracts and server scope**

Create typed `ShotReadiness`, `WorkflowStepStatus`, and `WorkflowStatus`. Batch generation always reloads authoritative inputs, computes readiness, and filters the requested set.

- [ ] **Step 5: Run GREEN and commit**

Run focused tests and `go test ./internal/videoproject -count=1`. Expected: PASS.

```bash
git add nx-backend/apps/server/internal/videoproject
git commit -m "feat(video): compute guided workflow status"
```

### Task 5B: Add idempotent script import

**Files:**
- Modify: `nx-backend/apps/server/internal/videoproject/workflow.go`
- Modify: `nx-backend/apps/server/internal/videoproject/workflow_test.go`
- Modify: `nx-backend/apps/server/internal/videoproject/videoproject.go`

- [ ] **Step 1: Write failing script import tests**

Use normalized paragraph source key:

```go
func ShotSourceKey(projectID string, revision, index int, paragraph string) string
```

Test a fixed SHA-256 golden, whitespace normalization, stable keys, rejection of stale `scriptRevision`, repeated batch returning `existing`, partial invalid items returning `failed`, retry creating only missing rows, and reconstructing created/existing/failed results after a simulated network retry. Inject a database write failure for the middle valid item and assert the preceding/following valid items are created, the middle is failed, and retry creates only the missing item.

- [ ] **Step 2: Run RED**

Run: `cd nx-backend/apps/server && go test ./internal/videoproject -run 'TestShotSourceKeyGolden|TestCreateShotsFromScript|TestCreateShotsFromStaleScriptRevision' -count=1`

Expected: FAIL because workflow helpers do not exist.

- [ ] **Step 3: Implement source keys and recoverable import**

Create typed `ScriptParagraph` and `CreateShotsFromScriptResult`. Prevalidate all user-data errors before writes. Within the batch transaction, wrap each insert in `SAVEPOINT shot_item_<index>` and `ROLLBACK TO SAVEPOINT` on database error so later items can continue; release each savepoint. Reconstruct import results from source keys after refresh. Reject mismatched project script revision with a conflict.

- [ ] **Step 4: Run GREEN**

Run: `cd nx-backend/apps/server && go test ./internal/videoproject -count=1`

Expected: PASS.

- [ ] **Step 5: Commit**

```bash
git add nx-backend/apps/server/internal/videoproject/workflow.go nx-backend/apps/server/internal/videoproject/workflow_test.go nx-backend/apps/server/internal/videoproject/videoproject.go
git commit -m "feat(video): add guided workflow state"
```

### Task 6: Persist compose participants and current/stale result

**Files:**
- Modify: `nx-backend/apps/server/internal/videoproject/projectcomposer.go`
- Modify: `nx-backend/apps/server/internal/videoproject/videoproject.go`
- Create: `nx-backend/apps/server/internal/videoproject/projectcomposer_test.go`

- [ ] **Step 1: Write failing compose input tests**

Test deterministic SHA-256 over the normalized content input: every non-deleted project shot ordered by `order_num,id`; included `{shotId,generationId,shotRevision,orderNum}`; sorted `excludedShotIds`; transition, subtitles and normalized music URL. `partialAcknowledged` is persisted in the snapshot but deliberately excluded from the hash so identical content does not become stale based on confirmation UI.

Test default mode requires every shot to have an explicit successful, non-empty-URL, current-revision `selected_generation_id` and never falls back to `generation_id`, shot status or shot video URL.

Test partial mode is rejected without per-request acknowledgement, rejects omitted/forged exclusions, foreign-project shots, and invalid/stale selections; server computes and persists exact included/excluded sets. Test reload round-trip. Only successful compose updates `final_video_input_hash`; failed compose preserves the old URL/hash. Order, selection, revision, exclusions or settings changes make the old result stale. Partial acknowledgement is stored on that job only and does not authorize a later request.

Test asynchronous job lifecycle: start returns a job ID in queued state; worker persists milestone progress `10/30/70/90/100` with `processing/completed`; failure persists error, settings and snapshot; status query by job ID round-trips them. Two concurrent inserts for the same project are serialized by the partial unique database index and the loser maps to a domain conflict, not merely a pre-check. A queued/processing job detected after server restart is marked failed with a recoverable interruption message and retained snapshot; retry creates a new job without deleting history.

- [ ] **Step 2: Run RED**

Run: `cd nx-backend/apps/server && go test ./internal/videoproject -run 'TestComposeInputHash|TestComposeRequiresSelectedVersions|TestPartialComposeValidation|TestComposeSnapshotRoundTrip|TestComposeJobProgress|TestComposeActiveJobConstraint|TestComposeRecoveryAfterRestart|TestComposeStatusStale|TestFailedComposePreservesCurrentResult' -count=1`

Expected: FAIL on missing snapshot/hash fields.

- [ ] **Step 3: Implement compose snapshot and status**

Persist exact JSON snapshot on every job. Default includes all shots and rejects missing valid selection. Partial mode requires `PartialAcknowledged=true` and explicit included/excluded lists. `StartCompose` creates queued job and returns `{jobId,status}`; a worker persists milestone progress while reusing existing compose operations. `GetComposeJob(projectId,jobId)` enforces ownership and returns `isCurrent`, input hash/snapshot, included/excluded shots, status, progress, URL and error. On startup (or first status access), recover stale queued/processing jobs to failed with a retry path.

- [ ] **Step 4: Run GREEN**

Run: `cd nx-backend/apps/server && go test ./internal/videoproject -count=1`

Expected: PASS.

- [ ] **Step 5: Commit**

```bash
git add nx-backend/apps/server/internal/videoproject/projectcomposer.go nx-backend/apps/server/internal/videoproject/videoproject.go nx-backend/apps/server/internal/videoproject/*_test.go
git commit -m "feat(video): make composed results revision aware"
```

### Task 7: Expose focused workflow routes

**Files:**
- Modify: `nx-backend/apps/server/internal/server/videoproject_routes.go`
- Modify: `nx-backend/apps/server/internal/server/server.go`
- Create: `nx-backend/apps/server/internal/server/video_workflow_test.go`
- Modify: `nx-backend/apps/web-antd/src/api/core/videoproject.ts`
- Create: `nx-backend/apps/web-antd/src/api/core/videoproject.workflow.test.ts`
- Modify: `nx-backend/apps/web-antd/src/views/video/projects/workbench.vue`
- Modify: `nx-backend/apps/web-antd/src/views/video/project-workbench-production-flow.test.ts`

- [ ] **Step 1: Write failing route/source tests**

Require handlers for:

```text
GET  /api/video/projects-workflow/:id
GET  /api/video/generation-submissions/:submissionId
POST /api/video/projects-shots/from-script/:id
POST /api/video/shots-generate-safe/:shotId
POST /api/video/projects-batch-generate-safe/:projectId
POST /api/video/generation-submissions/reconcile/:requestKey
POST /api/video/shots-video-versions/set/:shotId/:generationId
POST /api/video/projects-compose-safe/:projectId
GET  /api/video/projects-compose-safe-status/:projectId/:jobId
```

Name focused tests for malformed ID/request key `400`; readiness/import/compose blockers `422`; activity/version ownership conflicts `409`; first accepted or unknown response `202`; same-key reuse `200` with identical identity; same-task reconcile idempotency and different-task conflict; complete workflow GET; submission GET fields `{submissionId,requestKey,status,generationId?,taskId?,taskStatus?,error?}` with project/permission ownership checks; compose status by `{projectId,jobId}` fields `{status,progress,error,inputSnapshot,isCurrent,videoUrl}`; and old project/API regression.

Batch safe input is `{ items: [{ shotId, requestKey }] }`. The client persists one UUID per shot/user action and resends the identical list after a network retry; server reuses each key independently and still intersects with readiness.

- [ ] **Step 2: Run RED**

Run: `cd nx-backend/apps/server && go test ./internal/server -run 'TestVideoWorkflow(Get|Import|Generate|Batch|SubmissionStatus|Reconcile|Select|Compose|ComposeStatus|Legacy)' -count=1`

Expected: FAIL because routes are absent.

- [ ] **Step 3: Implement thin handlers**

Decode typed bodies, call domain methods, preserve HTTP status: `409` for active/version conflicts, `422` for blockers, `202` for accepted/unknown outcomes, `200` for idempotent reuse. Never expose gateway secrets or request snapshots.

Existing `/shots-generate`, `/projects-batch-generate` and `/projects-compose` routes remain for compatibility but internally delegate to the same safe services. Only legacy `/shots-generate` and `/projects-batch-generate` require client generation request keys and fail closed with `400` when missing; the server never invents a paid request key. Legacy `/projects-compose` delegates to `StartCompose`, uses the database active-job conflict, and returns `jobId`; it does not accept or require a generation request key. Migrate the advanced workbench and `videoproject.ts` callers in this task to retain one UUID per user action/shot until a known terminal response, then explicitly create a new UUID for “再生成一个版本”. Add an integration test that resending the same advanced-workbench request after a lost response produces one upstream POST.

- [ ] **Step 3A: Write and run the advanced-workbench RED test**

Extend `project-workbench-production-flow.test.ts` to require per-shot UUID storage, same-key reuse while status is unknown/active, new UUID only after terminal or explicit new-version action, stable key lists for batch, and compose job ID handling without generation keys.

Run: `cd nx-backend && pnpm exec vitest run apps/web-antd/src/views/video/project-workbench-production-flow.test.ts`

Expected: FAIL because advanced callers do not send request keys or compose job IDs.

- [ ] **Step 3B: Migrate callers and run GREEN/typecheck**

Update `videoproject.ts` inputs and the advanced workbench key cache. Network retries reuse the cached key; terminal transitions clear it; “重新生成/再生成一个版本” allocates a new UUID only after no active submission remains. Batch retains a map from shot ID to UUID. Compose stores/polls the returned job ID separately.

Run:

```bash
cd nx-backend
pnpm exec vitest run apps/web-antd/src/views/video/project-workbench-production-flow.test.ts
pnpm --filter @vben/web-antd run typecheck
```

Expected: test PASS and no new type errors.

- [ ] **Step 4: Run GREEN and backend regression**

Run: `cd nx-backend/apps/server && go test ./internal/video ./internal/videoproject ./internal/server ./internal/db -count=1`

Expected: PASS.

- [ ] **Step 5: Commit**

```bash
git add nx-backend/apps/server/internal/server nx-backend/apps/web-antd/src/api/core/videoproject.ts nx-backend/apps/web-antd/src/views/video/projects/workbench.vue nx-backend/apps/web-antd/src/views/video/project-workbench-production-flow.test.ts
git commit -m "feat(video): expose guided workflow APIs"
```

## Chunk 3: Five-Step Vue Workbench and Creation Center

### Task 8: Add typed API and pure frontend workflow logic

**Files:**
- Modify: `nx-backend/apps/web-antd/src/api/core/videoproject.ts`
- Create: `nx-backend/apps/web-antd/src/views/video/projects/workflow/workflow.ts`
- Create: `nx-backend/apps/web-antd/src/views/video/projects/workflow/workflow.test.ts`

- [ ] **Step 1: Write failing helper tests**

Test `splitScriptIntoShots`, step query validation, legacy recommended step, readiness presentation mapping, batch CTA grouping, and compose current/stale labels. Mirror server enums exactly.

Add an API contract test with a mocked `requestClient` for every Task 7 endpoint. Assert method, exact path and payload, including submission GET by ID, compose status with both project/job IDs, batch `{items:[{shotId,requestKey}]}`, reconcile request-key path/body, version selection parameter order, script import revision/items, workflow GET and safe compose.

- [ ] **Step 2: Run RED**

Run: `cd nx-backend && pnpm exec vitest run apps/web-antd/src/views/video/projects/workflow/workflow.test.ts apps/web-antd/src/api/core/videoproject.workflow.test.ts`

Expected: FAIL because module is missing.

- [ ] **Step 3: Implement types/helpers and API calls**

Add workflow response types, request-key fields, source keys, selection/revision, compose snapshot, and functions for all Task 7 routes. Pure helpers must not import Vue or router.

- [ ] **Step 4: Run GREEN**

Run: same two-file focused Vitest command.

Expected: PASS.

- [ ] **Step 5: Commit**

```bash
git add nx-backend/apps/web-antd/src/api/core/videoproject.ts nx-backend/apps/web-antd/src/api/core/videoproject.workflow.test.ts nx-backend/apps/web-antd/src/views/video/projects/workflow
git commit -m "feat(video): add workflow client contracts"
```

### Task 9: Build shell, real step navigation and dirty protection

**Files:**
- Create: `nx-backend/apps/web-antd/src/views/video/projects/workflow.vue`
- Create: `nx-backend/apps/web-antd/src/views/video/projects/workflow/WorkflowStepper.vue`
- Create: `nx-backend/apps/web-antd/src/views/video/projects/workflow/useWorkflowNavigation.ts`
- Create: `nx-backend/apps/web-antd/src/views/video/projects/workflow.source.test.ts`
- Create: `nx-backend/apps/web-antd/src/views/video/projects/workflow.interaction.test.ts`
- Modify: `nx-backend/apps/web-antd/src/router/routes/modules/video.ts`

- [ ] **Step 1: Write failing shell/source tests**

The source test asserts five labels and `brief/assets/storyboard/generate/export`, one `v-if` panel per step, `aria-current`, `aria-live`, default `workflow.vue`, hidden `/advanced`, and no project/short mode switch.

The happy-dom interaction test mounts the shell with mocked API/router and asserts: clicking each step renders only its panel and replaces URL; unknown step is replaced by server recommendation; blocked forward step remains enterable and shows blocker/recovery action; busy primary action disables duplicate clicks; loading/fatal/retry states work.

For each of the five steps in normal, blocked, busy and completed fixtures, assert exactly one visually primary command in the page shell, with the correct step-specific label and disabled/loading state. Drawer and secondary commands must not use the primary emphasis class/type.

Test dirty navigation for step change, shot change, advanced route and external router leave. Ordinary drawer open does not prompt; drawer action that mutates current references does. Save success clears dirty and runs the queued action exactly once; save failure retains form/step/shot and focuses first error; discard reloads then continues; cancel keeps focus/selection. `beforeunload` exists only while dirty and is removed after save/unmount.

- [ ] **Step 2: Run RED**

Run: `cd nx-backend && pnpm exec vitest run apps/web-antd/src/views/video/projects/workflow.source.test.ts apps/web-antd/src/views/video/projects/workflow.interaction.test.ts`

Expected: FAIL on missing files/routes.

- [ ] **Step 3: Implement shell and navigation composable**

Use a restrained light shell, 8px radius max, one primary footer action, top project/status bar, five-step nav, loading skeleton, fatal error with retry, advanced-workbench link, and `router.replace({query:{step}})`. Unknown step uses server recommendation.

- [ ] **Step 4: Implement dirty-action queue**

Container owns `dirtyScope` and pending navigation callback. Save failure retains selection/input and focuses first invalid field. Discard reloads latest server state; cancel keeps focus.

- [ ] **Step 5: Run GREEN**

Run the exact two-file Vitest command plus `pnpm --filter @vben/web-antd run typecheck`.

Expected: tests PASS; typecheck has no new errors.

- [ ] **Step 6: Commit**

```bash
git add nx-backend/apps/web-antd/src/views/video/projects/workflow.vue nx-backend/apps/web-antd/src/views/video/projects/workflow nx-backend/apps/web-antd/src/router/routes/modules/video.ts
git commit -m "feat(video): add guided workbench shell"
```

### Task 10: Implement brief, assets and storyboard steps

**Files:**
- Create: `nx-backend/apps/web-antd/src/views/video/projects/workflow/BriefStep.vue`
- Create: `nx-backend/apps/web-antd/src/views/video/projects/workflow/AssetsStep.vue`
- Create: `nx-backend/apps/web-antd/src/views/video/projects/workflow/StoryboardStep.vue`
- Modify: `nx-backend/apps/web-antd/src/views/video/projects/workflow.source.test.ts`
- Modify: `nx-backend/apps/web-antd/src/views/video/projects/workflow.interaction.test.ts`
- Modify: `nx-backend/apps/web-antd/src/views/video/projects/workflow.vue`

- [ ] **Step 1: Extend failing tests**

Mount each step and require:

- Brief: name, theme, script, style, script character count, estimated paragraph count, persistent save, and inline empty-script error.
- Assets: separate role/scene counts and missing-image counts; add/edit/delete; asset-library drawer; upload failure shown next to the item with retry; zero assets remains non-blocking.
- Storyboard: empty-state choices “从剧本创建分镜/手动添加”; import count confirmation; existing shots never overwritten silently; compact navigator and selected-shot editor; save before shot switch; script/action/characters/scene/duration/aspect fields; reference upload/picker; reconstructed created/existing/failed result groups and retry only failed source keys.

Source contracts require 44px primary/touch controls, icon `aria-label`s, visible text errors, mobile shot selector, step-nav-only overflow, safe-area footer padding, no page horizontal overflow, and `prefers-reduced-motion`.

- [ ] **Step 2: Run RED**

Run: `cd nx-backend && pnpm exec vitest run apps/web-antd/src/views/video/projects/workflow.source.test.ts apps/web-antd/src/views/video/projects/workflow.interaction.test.ts`

Expected: FAIL on missing components/behavior.

- [ ] **Step 3: Implement the three steps**

Reuse existing project/character/scene/shot/asset APIs. Keep default forms concise; reference uploads/picker live in a section, not a permanent sidebar. Import uses one batch API and stable source keys. Existing shots remain editable after partial import.

- [ ] **Step 4: Run GREEN and typecheck**

Run the same two-file Vitest command and web typecheck. Expected: PASS/no new errors.

- [ ] **Step 5: Commit**

```bash
git add nx-backend/apps/web-antd/src/views/video/projects/workflow.vue nx-backend/apps/web-antd/src/views/video/projects/workflow
git commit -m "feat(video): guide script assets and storyboard"
```

### Task 11: Implement safe generation, polling and version drawer

**Files:**
- Create: `nx-backend/apps/web-antd/src/views/video/projects/workflow/GenerationStep.vue`
- Create: `nx-backend/apps/web-antd/src/views/video/projects/workflow/VersionDrawer.vue`
- Create: `nx-backend/apps/web-antd/src/views/video/projects/workflow/useWorkflowPolling.ts`
- Modify: `nx-backend/apps/web-antd/src/views/video/projects/workflow.vue`
- Modify: `nx-backend/apps/web-antd/src/views/video/projects/workflow.source.test.ts`
- Modify: `nx-backend/apps/web-antd/src/views/video/projects/workflow.interaction.test.ts`

- [ ] **Step 1: Extend failing tests**

With fake timers and mocked APIs, require detailed ready/stale/failed/incomplete/generating/recovery states, exact `canGenerate` scope, grouped CTA counts, loading/disabled duplicate prevention, collapsed advanced settings, version drawer and explicit selection.

The user-facing overview has exactly four clickable filters: `可生成` maps ready/stale/failed, `待完善` maps incomplete/recovery while recovery keeps its check/reconcile action, `生成中` maps active submission/linked processing, and `已完成` maps a valid selected version. Assert counts, click filtering and selected state. The primary confirmation still breaks可生成 into ready/stale/failed counts.

Request-key lifecycle tests keep keys through `prepared/submitting/accepted/reconciled/unknown_outcome`; repeated click/network retry reuses them; only `completed/failed/cancelled` plus explicit new-version action creates a new UUID. Batch keeps a per-shot key map and clears only terminal entries after partial results.

Polling tests: accepted/reconciled continue; unknown outcome stops normal polling and exposes only check/reconcile; successful reconcile resumes linked task polling; timeout says “仍在处理中，可手动刷新” without failure; transient API errors use capped backoff; terminal, project change and unmount clear timers. Recent failed attempt plus a valid current selection remains completed while drawer shows attempt failure.

Version tests prove a newly successful version does not alter the old selection; only “设为当前” updates `selected_generation_id` and restores completed. Drawer close returns focus to its trigger. No automatic selection branch exists.

- [ ] **Step 2: Run RED**

Run: `cd nx-backend && pnpm exec vitest run apps/web-antd/src/views/video/projects/workflow.source.test.ts apps/web-antd/src/views/video/projects/workflow.interaction.test.ts`

Expected: FAIL.

- [ ] **Step 3: Implement safe submit UX**

Generate one or batch only after confirmation grouping ready/stale/failed. Cache each request key through every active/recovery state until terminal. A second click/network retry reuses the key. `unknown_outcome` exposes reconcile/check only. Display inline blockers with “返回分镜修改”.

- [ ] **Step 4: Implement polling and version drawer**

Poll safe status endpoints with bounded backoff; stop on terminal/unmount/project change. Drawer reuses existing preview/select/backup/edit/copy/frame/subtitle/upscale/delete APIs. New submissions and successful results never change the selected version until the user clicks “设为当前”.

- [ ] **Step 5: Run GREEN and typecheck**

Run the exact two workflow tests, all video view tests, and typecheck. Expected: PASS/no new errors.

- [ ] **Step 6: Commit**

```bash
git add nx-backend/apps/web-antd/src/views/video/projects/workflow.vue nx-backend/apps/web-antd/src/views/video/projects/workflow
git commit -m "feat(video): add safe guided generation"
```

### Task 12: Implement export and simplify creation center

**Files:**
- Create: `nx-backend/apps/web-antd/src/views/video/projects/workflow/ExportStep.vue`
- Modify: `nx-backend/apps/web-antd/src/views/video/projects/workflow.vue`
- Modify: `nx-backend/apps/web-antd/src/views/video/production/index.vue`
- Modify: `nx-backend/apps/web-antd/src/views/video/production.test.ts`
- Create: `nx-backend/apps/web-antd/src/views/video/production.interaction.test.ts`
- Modify: `nx-backend/apps/web-antd/src/views/video/projects/workflow.interaction.test.ts`
- Modify: `nx-backend/apps/web-antd/src/router/routes/modules/video.ts`
- Modify: `nx-backend/apps/server/internal/db/db.go`
- Create: `nx-backend/apps/server/internal/db/video_menu_test.go`

- [ ] **Step 1: Write failing export/entry tests**

Export interaction tests require exact included/excluded lists, explicit per-submit partial acknowledgement, transition/subtitles/music, active conflict, safe compose returning `jobId`, polling `{projectId,jobId}`, progress/error/retry with a new job, current/stale final video, stale video preview without current label, copy/download success and failure feedback, and timer cleanup on step leave/unmount. Failed compose preserves settings; a later partial attempt requires acknowledgement again.

Creation-center interaction tests mock `listProjectsApi/createProjectApi`: loading, empty, failure/retry, recent ordering and real progress; new button busy/duplicate prevention; success routes to `/video/projects/:id/workbench`; failure recovers inline; recent item continues; full list link works. Source/Go tests require short route, card and menu seed fully removed and hidden advanced route present. A router test resolves `/video/production/short` to the normal not-found behavior; neither static nor backend dynamic routes can reintroduce it.

- [ ] **Step 2: Run RED**

Run:

```bash
cd nx-backend
pnpm exec vitest run apps/web-antd/src/views/video/production.test.ts apps/web-antd/src/views/video/production.interaction.test.ts apps/web-antd/src/views/video/projects/workflow.source.test.ts apps/web-antd/src/views/video/projects/workflow.interaction.test.ts
cd apps/server
go test ./internal/db -run TestVideoMenu -count=1
```

Expected: FAIL on old short mode/menu and missing interactions.

- [ ] **Step 3: Implement export step**

Default blocks until all shots have valid selected versions. Partial compose shows exact lists and requires a confirmation checkbox/modal for that submission. Preserve settings after failure. Old stale video remains previewable with a visible “内容已变化，需要重新合成” state.

- [ ] **Step 4: Redesign creation center**

Use one primary new-project action and recent projects with actual progress. Remove unavailable short route/menu/card; keep full project list link. No marketing hero or decorative gradients.

- [ ] **Step 5: Run GREEN and typecheck**

Run the exact RED commands again, all video view tests, and typecheck. Expected: PASS/no new errors.

- [ ] **Step 6: Commit**

```bash
git add nx-backend/apps/web-antd/src/views/video nx-backend/apps/web-antd/src/router/routes/modules/video.ts nx-backend/apps/server/internal/db/db.go nx-backend/apps/server/internal/db/video_menu_test.go
git commit -m "feat(video): finish guided export and entry"
```

## Chunk 4: Regression, Browser Verification and Release Audit

### Task 13: Run automated regression and fix only introduced failures

**Files:**
- Modify only files responsible for introduced failures.

- [ ] **Step 1: Format all changed Go files before final tests**

Build the explicit list from Git and format only existing changed `.go` files:

```bash
cd /Users/wohenzaiyi/Desktop/nine-xing
git diff --name-only --diff-filter=ACMR 69d2ca9..HEAD -- '*.go' | while IFS= read -r file; do gofmt -w "$file"; done
```

Expected: exit 0. Inspect `git diff --check` afterward.

- [ ] **Step 2: Run focused and full backend suites**

Run:

```bash
cd nx-backend/apps/server
go test ./internal/video ./internal/videoproject ./internal/server ./internal/db -count=1
go test ./... -count=1
```

Expected: PASS.

- [ ] **Step 3: Run focused and full frontend suites**

Run:

```bash
cd nx-backend
pnpm exec vitest run apps/web-antd/src/views/video apps/web-antd/src/api/core/videoproject.workflow.test.ts apps/web-antd/src/router
pnpm exec vitest run apps/web-antd/src
```

Expected: PASS.

- [ ] **Step 4: Run typecheck and production build**

Run:

```bash
cd nx-backend
pnpm --filter @vben/web-antd run typecheck
pnpm --filter @vben/web-antd run build
```

Expected: PASS.

If and only if a failure appears unrelated, use the `using-git-worktrees` skill and an isolated path `/tmp/nine-xing-baseline-69d2ca9`:

Create the worktree and shared dependency link, then rerun the matching failed command verbatim from this fixed list:

```bash
git worktree add --detach /tmp/nine-xing-baseline-69d2ca9 69d2ca9
ln -s /Users/wohenzaiyi/Desktop/nine-xing/nx-backend/node_modules /tmp/nine-xing-baseline-69d2ca9/nx-backend/node_modules
```

```bash
cd /tmp/nine-xing-baseline-69d2ca9/nx-backend/apps/server && go test ./internal/video ./internal/videoproject ./internal/server ./internal/db -count=1
cd /tmp/nine-xing-baseline-69d2ca9/nx-backend/apps/server && go test ./... -count=1
cd /tmp/nine-xing-baseline-69d2ca9/nx-backend && pnpm exec vitest run apps/web-antd/src/views/video apps/web-antd/src/api/core/videoproject.workflow.test.ts apps/web-antd/src/router
cd /tmp/nine-xing-baseline-69d2ca9/nx-backend && pnpm exec vitest run apps/web-antd/src
cd /tmp/nine-xing-baseline-69d2ca9/nx-backend && pnpm --filter @vben/web-antd run typecheck
cd /tmp/nine-xing-baseline-69d2ca9/nx-backend && pnpm --filter @vben/web-antd run build
```

Run only the command corresponding to the observed failure, record output, then clean up:

```bash
cd /Users/wohenzaiyi/Desktop/nine-xing
git worktree remove /tmp/nine-xing-baseline-69d2ca9
```

Record both outputs; do not weaken a new assertion merely because baseline has a different failure.

### Task 14: Start the app and verify the real interface

**Files:**
- Create: `nx-backend/apps/web-antd/playwright.guided.config.ts`.
- Create: `nx-backend/apps/web-antd/e2e/guided-video-workflow.spec.ts`.
- Create untracked screenshots under `artifacts/`.

- [ ] **Step 1: Read browser skill and write the failing mandatory E2E**

Use Playwright route interception for auth/bootstrap and every business API; do not require a real backend/database/login. The config owns a Vite web server at `http://127.0.0.1:4317`:

```ts
webServer: {
  command: 'pnpm -F @vben/web-antd run dev -- --host 127.0.0.1 --port 4317',
  url: 'http://127.0.0.1:4317',
  reuseExistingServer: false,
  timeout: 120_000,
}
```

Playwright starts, waits for and terminates only this child server. If port 4317 is occupied, fail and identify the owner with `lsof`; do not kill an unrelated process. Configure Chromium, screenshot/trace/video on failure, output under `test-results/guided-video-workflow/`.

- [ ] **Step 2: Run E2E RED**

Run:

```bash
cd nx-backend
pnpm exec playwright test apps/web-antd/e2e/guided-video-workflow.spec.ts --config apps/web-antd/playwright.guided.config.ts --project chromium --reporter=line
```

Expected: FAIL on the first missing guided-workflow interaction.

- [ ] **Step 3: Implement route mocks and the complete happy path**

The mandatory scenario performs, in order: open creation center; create a project and reach default workbench; enter/save brief; import script; confirm partial import and edit a shot; add role/scene/reference asset; verify only ready/stale/failed shots are submitted; show a successful version without changing selection; open version drawer and explicitly select it; start compose; poll by job ID; preview current final video. Route mocks mutate in-memory fixture state so reload/navigation prove persistence.

Intercept every generation/submission/compose endpoint. No request may reach a real `/api/video/*generate*` or upstream gateway; assert intercepted paid gateway POST count is 0. Assert same-key duplicate UI actions yield one mocked submission identity.

- [ ] **Step 4: Add fixed recovery fixtures**

Add deterministic browser cases for legacy project recommendation, partial script import, accepted/reconciled polling, unknown outcome with reconcile-only UI, generation failure while valid selection remains completed, stale selection, polling timeout/manual refresh, compose failure/retry, and stale final result. These are all network-intercepted fixtures; no real paid POST is allowed.

- [ ] **Step 5: Add responsive and accessibility assertions**

At 1440x900, 768x1024 and 375x812, assert `document.documentElement.scrollWidth <= clientWidth`, every visible primary/icon control bounding box is at least 44px, fixed footer does not cover the last content element, mobile shot navigator is a select, step nav itself can scroll while page cannot, and no text/button overlaps via bounding boxes. Assert icon `aria-label`, active step `aria-current=step`, async `aria-live`, keyboard tab order, dirty dialog focus, Escape/cancel behavior and drawer focus return. Emulate `reducedMotion: 'reduce'` and assert nonessential transition durations compute to `0s` or animation names are `none`.

Capture:

- `artifacts/guided-workflow-desktop.png`
- `artifacts/guided-workflow-tablet.png`
- `artifacts/guided-workflow-mobile.png`
- `artifacts/guided-workflow-recovery.png`

- [ ] **Step 6: Run E2E GREEN and inspect console/network**

Run the exact Step 2 command. Expected: all named tests PASS, exit 0. Collect page errors, console errors and failed requests in the spec and assert none are attributable to the application. Assert real paid gateway POST count remains 0.

- [ ] **Step 7: Commit the repeatable E2E harness**

```bash
git add nx-backend/apps/web-antd/playwright.guided.config.ts nx-backend/apps/web-antd/e2e/guided-video-workflow.spec.ts
git commit -m "test(video): verify guided workflow in browser"
```

- [ ] **Step 8: Perform in-app Browser skill inspection**

Read and use `browser:control-in-app-browser`. Start the same Vite command on port 4317 in a managed terminal session, open `http://127.0.0.1:4317` in the in-app Browser, and use its inspection/screenshot/console capabilities to repeat the desktop happy-path shell, mobile viewport, dirty dialog focus and recovery-state checks. Stop only the server session started for this inspection. Record Browser-observed URL, viewport, console status, focus result and screenshot paths in the verification evidence document. This manual visual pass supplements, and does not replace, the Playwright assertions.

### Task 15: Completion audit with verification-before-completion

**Files:**
- Create: `docs/verification/2026-07-11-guided-video-workflow.md`

- [ ] **Step 1: Map every design success criterion to evidence**

Use `superpowers:verification-before-completion`. Create an evidence matrix with columns `Requirement | Authoritative evidence | Command/assertion | Exit/result | Artifact`. Cover real five-step panels, one primary action, novice defaults, exact generation scope, recovery, responsive layouts, legacy resume, idempotency, version selection and current compose hash. Include exact command outputs/exit codes and screenshot paths.

- [ ] **Step 2: Inspect final diff and worktree**

Run:

```bash
git status --short
git diff --stat 69d2ca9..HEAD
git diff --check 69d2ca9..HEAD
git diff --cached --name-only
```

Inspect all changed files. Assert `git diff --cached --name-only | rg '^artifacts/'` returns no matches. Confirm unrelated user changes are preserved.

- [ ] **Step 3: Commit final verification-only fixes**

Stage only the evidence document (all source/test commits already occurred):

```bash
git add docs/verification/2026-07-11-guided-video-workflow.md
git diff --cached --name-only
git diff --cached --name-only | rg '^artifacts/' && exit 1 || true
git commit -m "docs(video): record guided workflow verification"
```

- [ ] **Step 4: Mark the goal complete only after all evidence is green**

Do not claim completion with missing browser proof, failing tests, ambiguous paid-submission behavior, or an unverified responsive layout.
