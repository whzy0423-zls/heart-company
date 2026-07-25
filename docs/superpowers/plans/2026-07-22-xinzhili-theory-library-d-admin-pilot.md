# 芯之力理论库 D：后台审核与有界资料试点 Implementation Plan

> **For agentic workers:** REQUIRED: Use superpowers:subagent-driven-development (if subagents available) or superpowers:executing-plans to implement this plan. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** 提供资料目录、理论卡审核和检索复盘后台，并完成最多 24 个文件、2,000 页面、300 OCR 页的首批导入试点。

**Architecture:** 后端 admin API 只操作 `theorystore` 和 trace 查询服务；Vue 页面按资料、卡片、检索三个工作区拆分。`theoryingest` 只验证并导入标准抽取包，不运行 OCR。所有 AI 草稿保持 draft，人工审核后才能进入 release。

**Tech Stack:** Go 1.22, PostgreSQL, Vue 3, TypeScript, Ant Design Vue/Vben, Vitest, pnpm, external OCR package contract.

**Depends on:** Milestones A–C complete.

---

## Chunk 1: Admin review and bounded ingestion pilot

### Task 1: Add admin APIs for source catalog

**Files:**
- Create: `nx-backend/apps/server/internal/theorystore/admin_catalog.go`
- Create: `nx-backend/apps/server/internal/theorystore/admin_catalog_test.go`
- Create: `nx-backend/apps/server/internal/server/admin_theory_sources.go`
- Create: `nx-backend/apps/server/internal/server/admin_theory_sources_test.go`
- Modify: `nx-backend/apps/server/internal/server/server.go`

- [ ] **Step 1: Write failing route and behavior tests**

Cover `theorystore` paginated works/files queries, filters for extraction class/status/duplicate, detail view, metadata correction and retry-status action. Handler tests use a store interface and require `RAG:Theory:Manage`; handlers contain no SQL.

- [ ] **Step 2: Verify failure**

Run: `cd nx-backend/apps/server && go test ./internal/theorystore ./internal/server -run 'AdminCatalog|AdminTheorySources' -count=1`

- [ ] **Step 3: Implement focused handlers**

Routes:

```text
GET  /api/theory/sources/works
GET  /api/theory/sources/files
GET  /api/theory/sources/files/{id}
PUT  /api/theory/sources/files/{id}
POST /api/theory/sources/files/{id}/retry
```

Never return full extracted book text; return metadata, status, quality and short evidence previews only.

- [ ] **Step 4: Run and commit**

Run: `cd nx-backend/apps/server && go test ./internal/theorystore ./internal/server -run 'AdminCatalog|AdminTheorySources' -count=1`

```bash
git add nx-backend/apps/server/internal/theorystore/admin_catalog.go nx-backend/apps/server/internal/theorystore/admin_catalog_test.go nx-backend/apps/server/internal/server/admin_theory_sources.go nx-backend/apps/server/internal/server/admin_theory_sources_test.go nx-backend/apps/server/internal/server/server.go
git commit -m "feat(admin): manage theory source catalog"
```

### Task 2: Add theory card review and release APIs

**Files:**
- Create: `nx-backend/apps/server/internal/theorystore/admin_cards.go`
- Create: `nx-backend/apps/server/internal/theorystore/admin_cards_test.go`
- Create: `nx-backend/apps/server/internal/server/admin_theory_cards.go`
- Create: `nx-backend/apps/server/internal/server/admin_theory_cards_test.go`
- Create: `nx-backend/apps/server/internal/server/admin_theory_releases.go`
- Create: `nx-backend/apps/server/internal/server/admin_theory_releases_test.go`
- Modify: `nx-backend/apps/server/internal/server/server.go`

- [ ] **Step 1: Write failing workflow tests**

Cover store-backed list/detail queries plus atomic draft-aggregate editing of card fields, sources/page evidence, relations and practices; test add/update/remove child rows, stale child IDs and rollback. Also cover send to review, return, publish validation, supersede, retire, build release, activation conflict and rollback. Handler tests must prove all SQL/workflow ownership stays in `theorystore`.

- [ ] **Step 2: Verify failure**

Run: `cd nx-backend/apps/server && go test ./internal/theorystore ./internal/server -run 'AdminCards|AdminTheoryCards|AdminTheoryReleases' -count=1`

- [ ] **Step 3: Implement APIs**

Use `PUT /api/theory/cards/{id}` with a typed aggregate payload containing `card`, `sources`, `relations` and `practices`; save it through one `theorystore.SaveDraftAggregate` transaction. Use separate endpoints for state changes; never accept an arbitrary status field in general update. Return validation errors with field names for source, evidence and safety omissions.

- [ ] **Step 4: Run and commit**

Run: `cd nx-backend/apps/server && go test ./internal/theorystore ./internal/server -run 'AdminCards|AdminTheoryCards|AdminTheoryReleases' -count=1`

```bash
git add nx-backend/apps/server/internal/theorystore/admin_cards.go nx-backend/apps/server/internal/theorystore/admin_cards_test.go nx-backend/apps/server/internal/server/admin_theory_cards.go nx-backend/apps/server/internal/server/admin_theory_cards_test.go nx-backend/apps/server/internal/server/admin_theory_releases.go nx-backend/apps/server/internal/server/admin_theory_releases_test.go nx-backend/apps/server/internal/server/server.go
git commit -m "feat(admin): review and release theory cards"
```

### Task 3: Add retrieval observation API

**Files:**
- Create: `nx-backend/apps/server/internal/grounding/admin_query.go`
- Create: `nx-backend/apps/server/internal/grounding/admin_query_test.go`
- Create: `nx-backend/apps/server/internal/server/admin_theory_runs.go`
- Create: `nx-backend/apps/server/internal/server/admin_theory_runs_test.go`
- Modify: `nx-backend/apps/server/internal/server/server.go`

- [ ] **Step 1: Write failing trace-query tests**

Cover trace-query-service filters by status/corpus/fallback/risk/time and detail showing candidates, scores, filter reasons, selected sources, message IDs and errors. Handler tests use the service interface and contain no SQL.

- [ ] **Step 2: Verify failure**

Run: `cd nx-backend/apps/server && go test ./internal/grounding ./internal/server -run 'AdminTraceQuery|AdminTheoryRuns' -count=1`

Expected: FAIL because the query service and handlers do not exist.

- [ ] **Step 3: Implement read-only APIs**

Routes:

```text
GET /api/theory/retrieval-runs
GET /api/theory/retrieval-runs/{id}
```

Redact raw user content to authorized staff and cap displayed snippets.

- [ ] **Step 4: Run and commit**

Run: `cd nx-backend/apps/server && go test ./internal/grounding ./internal/server -run 'AdminTraceQuery|AdminTheoryRuns' -count=1`

```bash
git add nx-backend/apps/server/internal/grounding/admin_query.go nx-backend/apps/server/internal/grounding/admin_query_test.go nx-backend/apps/server/internal/server/admin_theory_runs.go nx-backend/apps/server/internal/server/admin_theory_runs_test.go nx-backend/apps/server/internal/server/server.go
git commit -m "feat(admin): inspect grounding retrieval runs"
```

### Task 4: Provision theory RBAC and menu access

**Files:**
- Modify: `nx-backend/apps/server/internal/db/db.go`
- Modify: `nx-backend/apps/server/internal/db/menu_test.go`
- Create: `nx-backend/apps/server/internal/db/theory_menu_integration_test.go`
- Modify: `nx-backend/apps/server/internal/server/server.go`

- [ ] **Step 1: Write failing menu and authorization tests**

Add a `TheoryLibrary` menu under RAG with path `/theory/sources`, component `/theory/sources`, auth code `RAG:Theory:Manage`, plus hidden child routes for cards and retrieval runs sharing the same permission. Verify admin role self-healing receives all menu IDs and an unauthorized role receives HTTP 403 from every theory admin endpoint.

- [ ] **Step 2: Verify failure**

Run: `cd nx-backend/apps/server && go test ./internal/db ./internal/server -run 'TheoryMenu|TheoryPermission' -count=1`

- [ ] **Step 3: Add idempotent menu seeds and guarded routes**

Use stable unused menu IDs in `defaultMenus`; preserve the existing `seedMenus` and admin-role binding behavior. Register every `/api/theory/*` route behind `RAG:Theory:Manage`.

- [ ] **Step 4: Run and commit**

Run: `cd nx-backend/apps/server && go test ./internal/db ./internal/server -run 'TheoryMenu|TheoryPermission|AdminTheory' -count=1`

```bash
git add nx-backend/apps/server/internal/db/db.go nx-backend/apps/server/internal/db/menu_test.go nx-backend/apps/server/internal/db/theory_menu_integration_test.go nx-backend/apps/server/internal/server/server.go
git commit -m "feat(auth): provision theory library permission"
```

### Task 5: Add typed frontend API and routes

**Files:**
- Create: `nx-backend/apps/web-antd/src/api/core/theory.ts`
- Modify: `nx-backend/apps/web-antd/src/api/core/index.ts`
- Create: `nx-backend/apps/web-antd/src/router/routes/modules/theory.ts`
- Create: `nx-backend/apps/web-antd/src/api/core/theory.test.ts`
- Create: `nx-backend/apps/web-antd/src/router/routes/modules/theory.test.ts`

- [ ] **Step 1: Write failing TypeScript tests**

Assert endpoints, payload types, route names, permission `RAG:Theory:Manage`, and lazy imports for all three workspaces.

- [ ] **Step 2: Verify failure**

Run: `cd nx-backend && pnpm exec vitest run apps/web-antd/src/api/core/theory.test.ts apps/web-antd/src/router/routes/modules/theory.test.ts --dom`

- [ ] **Step 3: Implement API clients and routes**

Keep types specific: no `Record<string, any>` for card safety/evidence/status fields.

- [ ] **Step 4: Run and commit**

Run: `cd nx-backend && pnpm exec vitest run apps/web-antd/src/api/core/theory.test.ts apps/web-antd/src/router/routes/modules/theory.test.ts --dom`

```bash
git add nx-backend/apps/web-antd/src/api/core nx-backend/apps/web-antd/src/router/routes/modules/theory.ts nx-backend/apps/web-antd/src/router/routes/modules/theory.test.ts
git commit -m "feat(web): route theory library administration"
```

### Task 6: Build the three admin workspaces

**Files:**
- Create: `nx-backend/apps/web-antd/src/views/theory/sources.vue`
- Create: `nx-backend/apps/web-antd/src/views/theory/cards.vue`
- Create: `nx-backend/apps/web-antd/src/views/theory/retrieval-runs.vue`
- Create: `nx-backend/apps/web-antd/src/views/theory/components/card-editor.vue`
- Create: `nx-backend/apps/web-antd/src/views/theory/components/source-evidence-editor.vue`
- Create: `nx-backend/apps/web-antd/src/views/theory/sources.test.ts`
- Create: `nx-backend/apps/web-antd/src/views/theory/cards.test.ts`
- Create: `nx-backend/apps/web-antd/src/views/theory/retrieval-runs.test.ts`

- [ ] **Step 1: Write failing view contract tests**

Assert filters, duplicate badges, extraction quality, card status actions, required evidence/safety fields, source page editor, release action, hit-score columns and fallback display.

- [ ] **Step 2: Verify failure**

Run: `cd nx-backend && pnpm exec vitest run apps/web-antd/src/views/theory --dom`

- [ ] **Step 3: Implement sources workspace**

Use a table plus detail drawer. Do not render full book text or expose a raw file download link.

- [ ] **Step 4: Implement card review workspace**

Split the editor into identity/definition, applicability, evidence/safety, relations/practice and sources tabs. State-change buttons call dedicated endpoints.

- [ ] **Step 5: Implement retrieval workspace**

Show site/knowledge/theory separately with lexical/vector/graph/final scores and filter reason.

- [ ] **Step 6: Run tests, typecheck and commit**

Run:

```bash
cd nx-backend
pnpm exec vitest run apps/web-antd/src/views/theory --dom
pnpm --filter @vben/web-antd typecheck
```

```bash
git add nx-backend/apps/web-antd/src/views/theory
git commit -m "feat(web): add theory review workspaces"
```

### Task 7: Implement the external extraction-package importer and pilot ledger

**Files:**
- Create: `nx-backend/apps/server/internal/theoryingest/manifest.go`
- Create: `nx-backend/apps/server/internal/theoryingest/manifest_test.go`
- Create: `nx-backend/apps/server/internal/theoryingest/importer.go`
- Create: `nx-backend/apps/server/internal/theoryingest/importer_test.go`
- Create: `nx-backend/apps/server/internal/theoryingest/batch_store.go`
- Create: `nx-backend/apps/server/internal/theoryingest/batch_store_test.go`
- Create: `nx-backend/apps/server/cmd/theory-ingest/main.go`
- Create: `nx-backend/docs/theory-ingest-package.schema.json`
- Modify: `nx-backend/apps/server/internal/db/schema.sql`
- Modify: `nx-backend/apps/server/internal/db/schema_theory_library_test.go`

- [ ] **Step 1: Write failing package-validation tests**

Cover mismatched SHA-256, missing pages, non-contiguous page numbers, invalid UTF-8, confidence outside 0–1, extractor metadata missing, and low-quality output becoming `review_required`. Add batch-ledger tests for unlisted package, extraction-method mismatch, repeated import idempotency, retry after failure, concurrent imports and exact 24/2,000/300 boundaries.

- [ ] **Step 2: Verify failure**

Run: `cd nx-backend/apps/server && go test ./internal/theoryingest ./internal/db -run 'Manifest|Batch|TheoryIngest' -count=1`

- [ ] **Step 3: Implement schema and validation**

The package contains `manifest.json` plus `pages/000001.txt` files. Add `theory_ingest_batches`, `theory_ingest_batch_files`, `theory_ingest_imports` and `theory_generated_drafts` with configured caps, allowed package rows, actual cumulative counters and draft idempotency keys. The importer must bind every package SHA/path/method/page range to a registered batch manifest, lock the batch row with `SELECT ... FOR UPDATE`, atomically reserve actual file/page/OCR counts, and reject any cap violation. It never invokes OCR.

- [ ] **Step 4: Implement dry-run and import CLI**

Commands:

```bash
go run ./cmd/theory-ingest register-batch --manifest nx-backend/scripts/theory/xinzhili-pilot-files.json --batch xinzhili-pilot-v1
go run ./cmd/theory-ingest validate --package /path/to/package
go run ./cmd/theory-ingest import --batch xinzhili-pilot-v1 --package /path/to/package --library xinzhili --dry-run
```

`register-batch` atomically creates/updates the batch and allowed package rows, rejects a manifest change after any successful import unless `--new-version` is used, and is idempotent for the same manifest hash.

- [ ] **Step 5: Run and commit**

Run: `cd nx-backend/apps/server && go test ./internal/theoryingest ./internal/db -count=1`

```bash
git add nx-backend/apps/server/internal/theoryingest nx-backend/apps/server/cmd/theory-ingest nx-backend/docs/theory-ingest-package.schema.json nx-backend/apps/server/internal/db
git commit -m "feat(theory): import validated extraction packages"
```

### Task 8: Generate source-linked draft cards without auto-publish

**Files:**
- Create: `nx-backend/apps/server/internal/theoryingest/draft_generator.go`
- Create: `nx-backend/apps/server/internal/theoryingest/draft_generator_test.go`
- Create: `nx-backend/apps/server/internal/theoryingest/draft_schema.go`
- Create: `nx-backend/apps/server/internal/theoryingest/draft_schema_test.go`
- Modify: `nx-backend/apps/server/cmd/theory-ingest/main.go`

- [ ] **Step 1: Write failing draft contract tests**

Given validated pages, assert one atomic draft contains canonical key/name, domain, kind, definition, applicability, epistemic/evidence/safety fields and source links with work/file/page/extraction quality. Reject hallucinated file/page references and any model output requesting `published` status. Re-running the same batch/work/generator-version/source-content-hash must update/reuse the same generated draft set without increasing card or evidence counts; changed source hash or generator version creates an explicit new draft generation.

- [ ] **Step 2: Verify failure**

Run: `cd nx-backend/apps/server && go test ./internal/theoryingest -run 'Draft|SourceLink' -count=1`

- [ ] **Step 3: Implement bounded draft generation/import**

Define a `DraftGenerator` interface so tests use a fake and production uses the configured admin model. Limit input pages/tokens per call. Normalize output through the same card validators and always write `status='draft'`; automatically insert `theory_card_sources` only for page references verified against the imported package ledger. Persist `(batch_id, work_id, generator_version, source_content_hash)` in `theory_generated_drafts` and upsert the complete draft set transactionally so interrupted/repeated runs are resumable and idempotent.

- [ ] **Step 4: Add CLI commands**

```bash
go run ./cmd/theory-ingest draft --batch xinzhili-pilot-v1 --work <canonical-key> --dry-run
go run ./cmd/theory-ingest draft --batch xinzhili-pilot-v1 --work <canonical-key>
```

Output created draft IDs and unresolved evidence warnings. No command in this CLI may publish, build or activate a release.

- [ ] **Step 5: Run and commit**

Run: `cd nx-backend/apps/server && go test ./internal/theoryingest -count=1`

```bash
git add nx-backend/apps/server/internal/theoryingest nx-backend/apps/server/cmd/theory-ingest/main.go
git commit -m "feat(theory): generate review-only source-linked drafts"
```

### Task 9: Create and execute the bounded pilot manifest

**Files:**
- Create: `nx-backend/scripts/theory/xinzhili-pilot-files.json`
- Create: `nx-backend/scripts/theory/README.md`
- Create: `nx-backend/scripts/theory/check-pilot-limits.sh`
- Create: `nx-backend/scripts/theory/check-pilot-limits.test.sh`
- Create: `nx-backend/scripts/theory/fixtures/valid-package/manifest.json`
- Create: `nx-backend/scripts/theory/fixtures/valid-package/pages/000001.txt`
- Create: `nx-backend/scripts/theory/fixtures/invalid-sha-package/manifest.json`
- Create: `nx-backend/scripts/theory/run-pilot.sh`
- Create: `nx-backend/apps/server/internal/theoryingest/acceptance.go`
- Create: `nx-backend/apps/server/internal/theoryingest/acceptance_test.go`
- Modify: `nx-backend/apps/server/cmd/theory-ingest/main.go`

- [ ] **Step 1: Write the failing limit test**

Test acceptance and rejection around 24 files, 2,000 total processed pages and 300 OCR pages.

- [ ] **Step 2: Implement the checker and reviewed manifest**

List only the named priority sources from design section 9. Record relative path, SHA-256, format, planned processed page range, extraction method, canonical work key and package path relative to a runtime `PACKAGES_ROOT`. Register the manifest as batch `xinzhili-pilot-v1` with caps 24/2,000/300. Do not commit extracted copyrighted full text.

- [ ] **Step 3: Run the checker**

Run: `bash nx-backend/scripts/theory/check-pilot-limits.test.sh && bash nx-backend/scripts/theory/check-pilot-limits.sh nx-backend/scripts/theory/xinzhili-pilot-files.json`

Expected: PASS and totals within all three caps.

- [ ] **Step 4: Write and run the failing acceptance-report tests**

Test under-cap but incomplete content, low-quality evidence, missing page links, duplicate work weighting, inactive release and successful 40/12 acceptance.

Run: `cd nx-backend/apps/server && go test ./internal/theoryingest -run Acceptance -count=1`

Expected: FAIL before `acceptance.go` and the CLI subcommand are implemented.

- [ ] **Step 5: Import only validated packages**

Implement `run-pilot.sh` as a manifest-driven resumable batch: call `register-batch` first, validate all packages, dry-run all, import all idempotently, generate drafts work-by-work idempotently, and print ledger totals/warnings. Require `PACKAGES_ROOT`, `TEST_DATABASE_URL` or production DB configuration, and stop on the first invalid package without consuming counters. Add a shell/integration test that interrupts after one work, reruns, and proves imports/draft counts/evidence weights are unchanged except for the remaining work.

Run: `PACKAGES_ROOT=/secure/xinzhili-packages bash nx-backend/scripts/theory/run-pilot.sh`

- [ ] **Step 6: Review content exit criteria**

After human review and release activation, implement the exact acceptance command below. It exits nonzero unless the batch ledger is within caps, at least 40 published theory cards and 12 published practice cards exist in the active release, every card has primary source/page, epistemic/evidence/safety fields, no extraction quality below 0.70, and duplicate canonical works are not counted as independent evidence.

Run: `cd nx-backend/apps/server && go run ./cmd/theory-ingest accept --batch xinzhili-pilot-v1 --min-theory 40 --min-practices 12`

- [ ] **Step 7: Commit the manifest and tooling**

```bash
git add nx-backend/scripts/theory nx-backend/apps/server/internal/theoryingest/acceptance.go nx-backend/apps/server/internal/theoryingest/acceptance_test.go nx-backend/apps/server/cmd/theory-ingest/main.go
git commit -m "chore(theory): define bounded xinzhili pilot corpus"
```

### Task 10: Final safety, admin and rollout verification

**Files:**
- Create: `nx-backend/apps/server/internal/grounding/safety_eval_test.go`
- Create: `docs/theory-library-rollout-checklist.md`

- [ ] **Step 1: Add the fixed safety evaluation set**

Include enneagram labelling, NLP certainty, I Ching prediction, trauma, self-harm, psychosis, domestic violence, medical advice, missing current price and no-source questions. Crisis recall must be 100%; missing-current-fact fabrication must be 0%.

- [ ] **Step 2: Run full verification**

Run:

```bash
cd nx-backend/apps/server
go test ./...
go vet ./...
cd ../../
pnpm exec vitest run apps/web-antd/src/api/core/theory.test.ts apps/web-antd/src/router/routes/modules/theory.test.ts apps/web-antd/src/views/theory --dom
pnpm --filter @vben/web-antd typecheck
```

Expected: PASS.

- [ ] **Step 3: Complete staff rollout checklist**

Record P95 ≤ 1,200ms, P99 ≤ 2,000ms, source trace integrity, knowledge-only emergency fallback, old-client SSE compatibility, reviewer names and active release ID.

- [ ] **Step 4: Commit verification artifacts**

```bash
git add nx-backend/apps/server/internal/grounding/safety_eval_test.go docs/theory-library-rollout-checklist.md
git commit -m "test(theory): verify safe theory rollout"
```
