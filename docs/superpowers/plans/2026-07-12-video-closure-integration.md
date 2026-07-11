# Video Closure Integration Implementation Plan

> **For agentic workers:** REQUIRED: Use superpowers:subagent-driven-development (if subagents available) or superpowers:executing-plans to implement this plan. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Preserve the current chat and test-safety work, merge the verified guided zero-charge video workflow, and make the normal E2E command execute the browser suite.

**Architecture:** First convert every intentional dirty-worktree change into a reviewable commit, then merge the verified video branch in an isolated integration worktree. Resolve shared schema and server files by composition, and make Playwright a real Turbo workspace task with worktree-local output.

**Tech Stack:** Go, PostgreSQL, Vue 3, Vitest, Turbo, Playwright, FFmpeg/ffprobe, Git worktrees

---

## Chunk 1: Preserve Current Work

### Task 0: Commit the approved implementation plan

**Files:**
- Create: `docs/superpowers/plans/2026-07-12-video-closure-integration.md`

- [ ] **Step 1: Commit the plan before changing implementation**

Stage only this plan, run `git diff --cached --check` and `git diff --cached --name-status`, then commit with `git commit -m "docs(video): plan closure integration"`.

### Task 1: Audit and commit conversation context work

**Files:**
- Modify: `nx-backend/apps/server/internal/chat/store.go`
- Create: `nx-backend/apps/server/internal/chat/conversation_context_test.go`
- Create: `nx-backend/apps/server/internal/chat/store_test.go`
- Modify: `nx-backend/apps/server/internal/db/schema.sql`
- Modify: `nx-backend/apps/server/internal/db/schema_order_test.go`
- Modify: `nx-backend/apps/server/internal/llm/minimax.go`
- Modify: `nx-backend/apps/server/internal/llm/minimax_test.go`
- Modify: `nx-backend/apps/server/internal/rag/rag.go`
- Modify: `nx-backend/apps/server/internal/rag/rag_test.go`
- Modify: `nx-backend/apps/server/internal/server/app_chat.go`
- Modify: `nx-backend/apps/server/internal/server/app_chat_test.go`

- [ ] **Step 1: Confirm focused behavior tests pass**

Run: `cd nx-backend/apps/server && go test ./internal/chat ./internal/llm ./internal/rag ./internal/server -run 'Conversation|Chat|History|Memory' -count=1`

Expected: PASS.

- [ ] **Step 2: Audit the staged file list**

Stage only the files listed in this task, then run `git diff --cached --name-status`.

Expected: only conversation context, memory, schema, generator, RAG, and app-chat files.

- [ ] **Step 3: Commit the feature**

Run: `git commit -m "feat(chat): preserve long conversation context"`

### Task 2: Commit isolated database-test safety and reliability fixes

**Files:**
- Create: `nx-backend/apps/server/internal/testutil/postgres.go`
- Create: `nx-backend/apps/server/internal/testutil/postgres_test.go`
- Modify: `nx-backend/apps/server/internal/appuser/store_test.go`
- Modify: `nx-backend/apps/server/internal/db/seed_test.go`
- Modify: `nx-backend/apps/server/internal/engagement/engagement_test.go`
- Modify: `nx-backend/apps/server/internal/server/app_auth_test.go`
- Modify: `nx-backend/apps/server/internal/server/app_privacy_test.go`
- Modify: `nx-backend/apps/server/internal/server/app_profile_calibration_integration_test.go`
- Modify: `nx-backend/apps/server/internal/server/server_test.go`
- Modify: `nx-backend/apps/server/internal/server/upload_test.go`

- [ ] **Step 1: Prove unsafe DSNs are rejected**

Run: `cd nx-backend/apps/server && go test ./internal/testutil -count=1 -v`

Expected: PASS for `_test` databases and rejection of empty, production, unsupported, and malformed DSNs.

- [ ] **Step 2: Run all integration tests against a disposable database**

From `nx-backend/apps/server`, run:

```bash
db="nine_xing_preserve_$(date +%s)_test"
cleanup() { /opt/homebrew/bin/dropdb --if-exists "$db"; }
trap cleanup EXIT
/opt/homebrew/bin/createdb -O nx "$db"
TEST_DATABASE_URL="postgres://nx:nx@localhost:5432/${db}?sslmode=disable" \
  go test ./... -count=1 -p=1 -v
```

Expected: database-backed tests visibly run and pass, and the trap removes the database on success or failure.

- [ ] **Step 3: Audit and commit**

Stage only the files listed in this task, inspect `git diff --cached --name-status`, then run:

`git commit -m "test(server): isolate database integration suites"`

### Task 3: Preserve dependency catalog work and audit status

**Files:**
- Modify: `nx-backend/pnpm-workspace.yaml`

- [ ] **Step 1: Verify the catalog entry and lockfile are consistent**

Run `rg -n "@ant-design/icons-vue" nx-backend/apps nx-backend/packages nx-backend/pnpm-lock.yaml nx-backend/pnpm-workspace.yaml`, then run `cd nx-backend && pnpm install --frozen-lockfile`.

Expected: the dependency and catalog definitions are visible, install exits 0, and the lockfile does not change. If the frozen install rejects the catalog state, run `pnpm install --lockfile-only`, inspect the exact lockfile delta, and include that deterministic update in this task.

- [ ] **Step 2: Commit the catalog change**

Stage `nx-backend/pnpm-workspace.yaml` and an audited lockfile change if required. Run `git diff --cached --name-status`, then run `git commit -m "chore(deps): catalog ant design icons"`.

- [ ] **Step 3: Audit every remaining status entry**

Run: `git status --short`.

Expected: only the user-owned `artifacts/` directory remains untracked. The approved design and implementation plan must already be committed as documentation commits. Do not add or delete `artifacts/`.

## Chunk 2: Integrate the Verified Video Branch

### Task 4: Create an isolated integration worktree

**Files:** none

- [ ] **Step 1: Create the branch and worktree**

Use the existing global worktree root and create branch `integrate/video-closure` from the updated `detail-tuning-video-management` HEAD.

Expected path: `~/.config/superpowers/worktrees/nine-xing/video-closure-integration`.

- [ ] **Step 2: Verify baseline**

Run `cd nx-backend && pnpm install --frozen-lockfile`, then `pnpm exec playwright install chromium`. Run Go tests without database configuration and `pnpm run test:unit` in the new worktree.

Expected: PASS before merge.

### Task 5: Merge and resolve shared files

**Files likely to conflict:**
- `nx-backend/apps/server/internal/db/schema.sql`
- `nx-backend/apps/server/internal/server/server.go`
- `nx-backend/apps/server/internal/server/videoproject_routes.go`
- `nx-backend/apps/web-antd/src/api/core/videoproject.ts`
- `nx-backend/apps/web-antd/src/views/video/projects/workbench.vue`
- `nx-backend/pnpm-workspace.yaml`

- [ ] **Step 1: Merge without rewriting history or committing early**

Run: `git merge --no-ff --no-commit feature/guided-video-workbench`.

Expected: merged changes remain staged but uncommitted, or Git reports an explicit conflict list.

- [ ] **Step 2: Resolve conflicts by composition**

Keep conversation-summary columns and all video workflow tables in schema order. Preserve current chat initialization and verified guided-video configuration/routes. Never delete tests to resolve a conflict.

- [ ] **Step 3: Run focused combined tests**

Run: `cd nx-backend/apps/server && go test ./internal/chat ./internal/db ./internal/llm ./internal/rag ./internal/server ./internal/video ./internal/videoproject -count=1`

Run: `cd nx-backend && pnpm exec vitest run apps/web-antd/src/views/video apps/web-antd/src/api/core`

Expected: PASS.

- [ ] **Step 4: Finish the merge commit**

Stage every resolved file, then require `git diff --name-only --diff-filter=U` to return no paths. Run `git diff --cached --check`, inspect `git diff --cached --name-status` and the staged merge diff, then commit the resolved merge.

## Chunk 3: Make E2E a Real Gate

### Task 6: Add failing E2E task and portability checks

**Files:**
- Modify: `nx-backend/apps/web-antd/package.json`
- Modify: `nx-backend/apps/web-antd/e2e/guided-video-workflow.spec.ts`
- Modify: `nx-backend/apps/web-antd/src/views/video/projects/workflow.source.test.ts`
- Modify: `nx-backend/turbo.json`
- Modify: `nx-backend/.gitignore` or the nearest existing ignore file

- [ ] **Step 1: Add a source-contract regression assertion**

Extend the workflow source test to read the E2E spec and assert it does not contain `/Users/`, `/Desktop/nine-xing`, or another checkout path.

- [ ] **Step 2: Verify the portability test fails**

Run the focused Vitest file.

Expected: FAIL because `artifactRoot` is currently an absolute desktop path.

- [ ] **Step 3: Verify the root E2E command is currently empty**

Run: `pnpm run test:e2e`.

Expected before implementation: output contains `0 total` or `No tasks were executed`.

### Task 7: Implement the E2E workspace task and local output

**Files:** same as Task 6

- [ ] **Step 1: Make screenshot output portable**

Remove `artifactRoot`. Accept Playwright `testInfo` in screenshot-producing tests and use `testInfo.outputPath('guided-workflow-<name>.png')`.

- [ ] **Step 2: Add the workspace script**

Add to `@vben/web-antd`:

`"test:e2e": "playwright test guided-video-workflow.spec.ts --config playwright.guided.config.ts --project=chromium --reporter=line"`

Set the root Turbo `test:e2e` task to `"cache": false` so every invocation launches Playwright. Ensure Playwright output directories are ignored without ignoring source fixtures.

- [ ] **Step 3: Verify green**

Run the focused portability Vitest test, then run root `pnpm run test:e2e`.

Expected: portability test PASS; two consecutive root E2E runs both show real Playwright execution and `12 passed`, with no Turbo cache hit replacing browser execution.

- [ ] **Step 4: Commit the gate**

Stage `nx-backend/apps/web-antd/package.json`, the E2E spec, portability test, `nx-backend/turbo.json`, and the selected ignore file. Run `git diff --cached --name-status` and `git diff --cached --check`, then run `git commit -m "test(video): wire guided workflow e2e gate"`.

## Chunk 4: Full Verification and Review

### Task 8: Run the complete backend and zero-charge closure

**Files:** none

- [ ] **Step 1: Full disposable-database suite**

Run from `nx-backend/apps/server`:

```bash
db="nine_xing_integration_$(date +%s)_test"
cleanup() { /opt/homebrew/bin/dropdb --if-exists "$db"; }
trap cleanup EXIT
/opt/homebrew/bin/createdb -O nx "$db"
TEST_DATABASE_URL="postgres://nx:nx@localhost:5432/${db}?sslmode=disable" go test ./... -count=1 -p=1 -v
```

Expected: database-backed tests show `RUN`/`PASS`, none report an unexpected skip, and the trap removes the database.

- [ ] **Step 2: Static verification**

Run `go vet ./...`, then from `nx-backend/apps/server` run:

```bash
unformatted=$(rg --files -g '*.go' -0 | xargs -0 gofmt -l)
test -z "$unformatted" || { print -r -- "$unformatted"; false; }
```

- [ ] **Step 3: Real zero-charge HTTP closure**

Using a fresh database created and trapped exactly as in Step 1, run:

```bash
TEST_DATABASE_URL="postgres://nx:nx@localhost:5432/${db}?sslmode=disable" \
  go test ./internal/server -run '^TestFreeVideoWorkflowRealHTTPClosure$' -count=1 -v
```

Expected: the test visibly runs rather than skips, passes, and logs `provider_calls=0`.

### Task 9: Run the complete frontend and multi-app matrix

**Files:** none

- [ ] **Step 1: Admin frontend**

Run `cd nx-backend && pnpm run test:unit && pnpm run check:type && pnpm run build:antd && pnpm run test:e2e`. Record collected test counts and confirm E2E prints real Playwright execution rather than only a Turbo cache hit.

- [ ] **Step 2: Other applications**

Run the following with each project's committed npm lockfile:

- `cd website-react && npm test && npm run build`
- `cd reading-h5 && npm test && npm run build`
- `cd miniapp && npm run test:config && npm run build:h5 && npm run build:mp-weixin`

- [ ] **Step 3: Repository checks**

Run `git diff --check`, search E2E sources for hard-coded checkout paths, verify no generated test output remains, and inspect `git status --short`.

### Task 10: Two-stage review and delivery

**Files:** all integration changes

- [ ] **Step 1: Spec compliance review**

Verify every design requirement and plan acceptance command against fresh output.

- [ ] **Step 2: Re-run quality-debt baselines and scoped lint**

Run `pnpm run lint`, `pnpm run check:circular`, `pnpm run check:dep`, and `pnpm run publint`; record their fresh exit codes and issue counts even when they reproduce documented repository debt. Separately run:

```bash
pnpm exec oxfmt --check \
  apps/web-antd/e2e/guided-video-workflow.spec.ts \
  apps/web-antd/playwright.guided.config.ts \
  apps/web-antd/src/views/video/projects/workflow.source.test.ts
pnpm exec eslint \
  apps/web-antd/e2e/guided-video-workflow.spec.ts \
  apps/web-antd/playwright.guided.config.ts \
  apps/web-antd/src/views/video/projects/workflow.source.test.ts
```

Also run the repository's configured lint tools on any additional conflict-resolved frontend source files, fixing violations introduced by this integration.

- [ ] **Step 3: Code quality review**

Review conflict resolutions, schema order, fail-closed paid-provider behavior, E2E lifecycle, and accidental file inclusion.

- [ ] **Step 4: Resolve findings and re-run affected verification**

Do not waive correctness findings. Historical lint, publishing, dependency-cycle, and asset-size debt remains separately documented unless integration changed it.

- [ ] **Step 5: Deliver the branch**

Report branch name, commit list, exact test evidence, known warnings, and the recommended merge command. Do not push or merge into `main`.
