# Automatic Care System Implementation Plan

> **For agentic workers:** REQUIRED: Use superpowers:executing-plans to implement this plan. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Build an automatic 1-10 care-level system that evaluates main-assistant and one-to-one friend conversations with the active knowledge base, shows the result in the App portrait, and marks/sorts App customers in the admin console.

**Architecture:** Add an idempotent PostgreSQL care snapshot/history/queue schema. A server-side evaluator gathers the 30-day main and direct-message corpus plus historical baseline and enabled knowledge metadata, produces a bounded structured result, and persists it asynchronously with a retry-safe queue. Existing App `/me` and admin App-user list/detail payloads expose the snapshot; Flutter and Vue render the same contract.

**Tech Stack:** Go 1.x, PostgreSQL, `database/sql`, existing chat/directmessage/appknowledge stores, Flutter/Dart, Vue 3 + Ant Design Vue, Vitest/Go tests.

---

## Chunk 1: Contract and database foundation

### Task 1: Define failing schema contract tests

**Files:**
- Create: `nx-backend/apps/server/internal/db/schema_care_system_test.go`
- Modify: `nx-backend/apps/server/internal/db/schema.sql`

- [ ] **Step 1: Write tests for idempotent care schema fragments**

Assert `app_users` has `care_level`, `care_label`, `care_summary`, `care_trend`, `care_data_status`, `care_evaluated_at`, `care_knowledge_version`, and `care_evaluation_version`; assert `care_evaluations` and `care_evaluation_queue` exist with user foreign keys, bounded level checks, JSON signal storage, status/indexes, and a unique pending queue constraint.

- [ ] **Step 2: Run the focused test and verify RED**

Run: `cd nx-backend/apps/server && go test ./internal/db -run Care -count=1`
Expected: FAIL because the schema does not contain the care columns/tables.

- [ ] **Step 3: Add idempotent schema SQL**

Add `ALTER TABLE app_users ADD COLUMN IF NOT EXISTS ...` and `CREATE TABLE IF NOT EXISTS` definitions near the existing App-user/chat schema. Add indexes for `(care_level DESC, care_evaluated_at DESC)`, queue status/next attempt, and evaluation user/time. Keep all existing columns and migration ordering intact.

- [ ] **Step 4: Run the focused test and verify GREEN**

Run the same command; expected PASS.

- [ ] **Step 5: Commit**

`git add nx-backend/apps/server/internal/db/schema.sql nx-backend/apps/server/internal/db/schema_care_system_test.go && git commit -m "feat: add care system schema"`

### Task 2: Add care domain model and bounded scoring rules

**Files:**
- Create: `nx-backend/apps/server/internal/caresystem/models.go`
- Create: `nx-backend/apps/server/internal/caresystem/scoring.go`
- Create: `nx-backend/apps/server/internal/caresystem/scoring_test.go`

- [ ] **Step 1: Write failing scoring tests**

Cover data-insufficient output, lower/middle/high/maximum level boundaries, single-message cap (one message cannot jump to level 10), trend comparison against prior level, and stable labels for levels 1-10.

- [ ] **Step 2: Run `go test ./internal/caresystem -run TestScore -count=1` and verify RED**

Expected: package/functions missing.

- [ ] **Step 3: Implement minimal pure scoring functions**

Define `Signals`, `HistoricalBaseline`, `Evaluation`, `Score`, `LabelForLevel`, and `TrendFor`. Clamp output to 1..10, return `DataStatusInsufficient` when corpus evidence is below the configured threshold, and keep medical-diagnosis language out of labels.

- [ ] **Step 4: Run focused tests and verify GREEN**

Run: `go test ./internal/caresystem -run TestScore -count=1`.

- [ ] **Step 5: Commit**

`git add nx-backend/apps/server/internal/caresystem && git commit -m "feat: add bounded care scoring rules"`

---

## Chunk 2: Corpus collection, persistence, and automatic evaluation

### Task 3: Add corpus collector for main and friend conversations

**Files:**
- Create: `nx-backend/apps/server/internal/caresystem/collector.go`
- Create: `nx-backend/apps/server/internal/caresystem/collector_test.go`

- [ ] **Step 1: Write failing collector tests**

Use a recording SQL driver to assert the collector reads `app_chat_sessions`/`app_chat_messages` for the user’s main chat and joins `direct_conversations`/`direct_messages` for every one-to-one conversation involving the user. Assert the 30-day cutoff, historical baseline query, ordering, and bounded message count.

- [ ] **Step 2: Run focused collector tests and verify RED**

Run: `go test ./internal/caresystem -run TestCollect -count=1`.

- [ ] **Step 3: Implement collector**

Return normalized `ConversationEvidence` records with source (`main` or `friend`), role/sender, content, and timestamp. Exclude recalled/empty media-only messages, cap corpus size deterministically, and never return raw corpus in API response.

- [ ] **Step 4: Run focused tests and verify GREEN**

Run the collector test command again.

- [ ] **Step 5: Commit**

`git add nx-backend/apps/server/internal/caresystem/collector* && git commit -m "feat: collect care evaluation conversation evidence"`

### Task 4: Persist snapshots, history, and retry queue

**Files:**
- Create: `nx-backend/apps/server/internal/caresystem/store.go`
- Create: `nx-backend/apps/server/internal/caresystem/store_test.go`

- [ ] **Step 1: Write failing store tests**

Test enqueue deduplication, successful snapshot transaction, history insertion, failure preserving prior snapshot, retry backoff state, and loading current snapshot.

- [ ] **Step 2: Run `go test ./internal/caresystem -run TestStore -count=1` and verify RED**

- [ ] **Step 3: Implement store methods**

Implement `Enqueue`, `ClaimBatch`, `SaveEvaluation`, `SaveFailure`, and `Current`. Use transactions, `FOR UPDATE SKIP LOCKED`, bounded retries, and `care_data_status` values `ready`, `insufficient`, `stale`, `failed`.

- [ ] **Step 4: Run focused tests and verify GREEN**

- [ ] **Step 5: Commit**

`git add nx-backend/apps/server/internal/caresystem/store* && git commit -m "feat: persist care snapshots and retry queue"`

### Task 5: Add evaluator worker and message enqueue hooks

**Files:**
- Create: `nx-backend/apps/server/internal/caresystem/evaluator.go`
- Create: `nx-backend/apps/server/internal/caresystem/evaluator_test.go`
- Modify: `nx-backend/apps/server/internal/server/server.go`
- Modify: `nx-backend/apps/server/internal/server/app_chat.go`
- Modify: `nx-backend/apps/server/internal/server/app_direct_message.go`

- [ ] **Step 1: Write failing evaluator/worker tests**

Test that a claimed user combines main and friend evidence, active knowledge version is attached, scoring output is saved, one user is not evaluated concurrently twice, and model/knowledge failure leaves the prior snapshot intact while queueing retry.

- [ ] **Step 2: Run focused evaluator tests and verify RED**

Run: `go test ./internal/caresystem ./internal/server -run 'Care|care' -count=1`.

- [ ] **Step 3: Implement evaluator and worker lifecycle**

Use existing configured JSON-capable generator/RAG knowledge metadata rather than introducing a second LLM client. Start a bounded worker with server shutdown cancellation and a periodic sweep fallback. After successful main-chat or direct-message persistence, enqueue the app user without delaying the message response.

- [ ] **Step 4: Run focused tests and verify GREEN**

- [ ] **Step 5: Commit**

`git add nx-backend/apps/server/internal/caresystem nx-backend/apps/server/internal/server/server.go nx-backend/apps/server/internal/server/app_chat.go nx-backend/apps/server/internal/server/app_direct_message.go && git commit -m "feat: evaluate care level after conversations"`

---

## Chunk 3: App and admin API contracts

### Task 6: Expose care snapshot from App `/me`

**Files:**
- Create: `nx-backend/apps/server/internal/server/app_care.go`
- Modify: `nx-backend/apps/server/internal/server/server.go`
- Modify: `nx-backend/apps/server/internal/appuser/store.go`
- Create: `nx-backend/apps/server/internal/server/app_care_test.go`

- [ ] **Step 1: Write failing endpoint tests**

Assert authenticated `GET /api/app/me` includes the care fields, data-insufficient state is represented without a fake level, and unauthorized requests are rejected.

- [ ] **Step 2: Run focused endpoint tests and verify RED**

- [ ] **Step 3: Implement response mapping and route integration**

Return the existing AppUser object plus camelCase care fields. Keep summary bounded and avoid raw chat/knowledge payloads.

- [ ] **Step 4: Run focused tests and verify GREEN**

- [ ] **Step 5: Commit**

`git add nx-backend/apps/server/internal/server/app_care* nx-backend/apps/server/internal/server/server.go nx-backend/apps/server/internal/appuser/store.go && git commit -m "feat: expose app care snapshot"`

### Task 7: Extend admin App-customer list/detail and sorting/filtering

**Files:**
- Modify: `nx-backend/apps/server/internal/appuser/store.go`
- Modify: `nx-backend/apps/server/internal/appuser/handlers.go`
- Create: `nx-backend/apps/server/internal/appuser/care_test.go`
- Modify: `nx-backend/apps/web-antd/src/api/core/app-customer.ts`
- Modify: `nx-backend/apps/web-antd/src/api/core/app-customer.test.ts`

- [ ] **Step 1: Write failing Go and TypeScript API tests**

Assert list query supports `careLevel`, default SQL ordering is level DESC then evaluation time DESC, response fields map to camelCase, and detail includes the same snapshot. Assert TS API preserves filter params and types.

- [ ] **Step 2: Run focused tests and verify RED**

Run Go package tests and `pnpm vitest run apps/web-antd/src/api/core/app-customer.test.ts` from `nx-backend`.

- [ ] **Step 3: Implement query/model extensions**

Join/read `app_users` care columns, add optional `careLevel` filter validation `1..10`, and retain existing pagination/search/status/member filters. Add `careDataStatus` for insufficient/failed states.

- [ ] **Step 4: Run focused tests and verify GREEN**

- [ ] **Step 5: Commit**

`git add nx-backend/apps/server/internal/appuser nx-backend/apps/web-antd/src/api/core/app-customer* && git commit -m "feat: expose sortable care levels to admin"`

---

## Chunk 4: UI presentation and verification

### Task 8: Render care card in Flutter App portrait

**Files:**
- Modify: `/Users/wohenzaiyi/Desktop/nine-xing-app/lib/features/auth/app_user.dart`
- Modify: `/Users/wohenzaiyi/Desktop/nine-xing-app/lib/features/growth/models/growth_portrait.dart`
- Modify: `/Users/wohenzaiyi/Desktop/nine-xing-app/lib/features/growth/growth_repository.dart`
- Modify: `/Users/wohenzaiyi/Desktop/nine-xing-app/lib/features/growth/screens/growth_portrait_screen.dart`
- Create: `/Users/wohenzaiyi/Desktop/nine-xing-app/test/features/care/care_level_card_test.dart`

- [ ] **Step 1: Write failing widget/model tests**

Assert JSON parsing for all care fields, data-insufficient empty state, level 1/5/10 color labels, summary and trend rendering, and that no raw conversation text is rendered.

- [ ] **Step 2: Run focused Flutter test and verify RED**

Run: `flutter test test/features/care/care_level_card_test.dart`.

- [ ] **Step 3: Implement care model and portrait card**

Add a reusable `CareSnapshot` model, load it with the portrait response or `/me` refresh, and render a compact Hifi card with accessible semantic labels. Refresh after returning from chat without blocking portrait content.

- [ ] **Step 4: Run focused test and verify GREEN**

- [ ] **Step 5: Commit**

`cd /Users/wohenzaiyi/Desktop/nine-xing-app && git add lib test && git commit -m "feat: show care level in app portrait"`

### Task 9: Render care level in admin customer list/detail

**Files:**
- Modify: `nx-backend/apps/web-antd/src/views/customer/app-users.vue`
- Create: `nx-backend/apps/web-antd/src/views/customer/app-user-care.ts`
- Create: `nx-backend/apps/web-antd/src/views/customer/app-user-care.test.ts`

- [ ] **Step 1: Write failing Vue helper tests**

Assert level-to-label/color mapping, high-level emphasis for 8-10, insufficient-state label, and stable sort/filter query serialization.

- [ ] **Step 2: Run focused Vitest test and verify RED**

- [ ] **Step 3: Implement table column, filter, and detail block**

Add a care-level Select filter, table column with Tag/progress emphasis, default server-side ordering, and detail descriptions for level, trend, summary, data window, and evaluated time. Do not add notification UI.

- [ ] **Step 4: Run focused tests and verify GREEN**

- [ ] **Step 5: Commit**

`git add nx-backend/apps/web-antd/src/views/customer && git commit -m "feat: mark care levels in admin customers"`

### Task 10: Full verification and documentation

**Files:**
- Modify: `docs/superpowers/specs/2026-09-11-care-system-design.md` (verification notes only)
- Modify: `README.md` or existing deployment docs if the worker/config needs an operator note

- [ ] **Step 1: Run backend tests and format**

Run: `cd nx-backend/apps/server && gofmt -w internal/caresystem internal/appuser internal/server && go test ./...`.

- [ ] **Step 2: Run admin tests/build**

Run: `cd nx-backend && pnpm vitest run apps/web-antd/src/api/core/app-customer.test.ts apps/web-antd/src/views/customer/app-user-care.test.ts && pnpm --filter @vben/web-antd typecheck` (use the repository’s documented equivalent if the script name differs).

- [ ] **Step 3: Run Flutter tests/analyze/format**

Run: `cd /Users/wohenzaiyi/Desktop/nine-xing-app && dart format --set-exit-if-changed lib test && flutter analyze && flutter test`.

- [ ] **Step 4: Review diff and migration idempotency**

Run `git diff --check`, inspect both repositories’ `git status --short`, and verify schema migration can run twice without altering existing customer or message data.

- [ ] **Step 5: Commit verification/docs**

Commit only the feature changes and verification documentation; keep unrelated user edits untouched.
