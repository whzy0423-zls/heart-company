# 芯之力理论库 B：理论检索与 Shadow Trace Implementation Plan

> **For agentic workers:** REQUIRED: Use superpowers:subagent-driven-development (if subagents available) or superpowers:executing-plans to implement this plan. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** 建立类型化的站点/知识/理论检索、意图与风险判断、RRF 和一跳图扩展，并以 shadow 模式记录检索，不改变当前用户回答。

**Architecture:** `internal/grounding` 定义运行契约和编排；`ragstore` 只负责 site/knowledge 候选；`theoryretrieval` 负责 active release 的中文 lexical、向量、RRF 和图扩展；trace store 保存运行与候选。所有空结果、降级和错误显式区分。

**Tech Stack:** Go 1.22, PostgreSQL, pgvector `vector(1536)`, pg_trgm, existing embedding client, Go tests.

**Depends on:** Milestone A complete.

---

## Chunk 1: Retrieval and shadow observability

### Task 1: Define complete typed grounding contracts

**Files:**
- Create: `nx-backend/apps/server/internal/grounding/types.go`
- Create: `nx-backend/apps/server/internal/grounding/types_test.go`
- Create: `nx-backend/apps/server/internal/grounding/classifier.go`
- Create: `nx-backend/apps/server/internal/grounding/classifier_test.go`

- [ ] **Step 1: Write failing type and classification tests**

Cover `ResultStatus`, `IntentKind`, `RiskLevel`, `SiteHit`, `KnowledgeHit`, `TheoryHit`, `GroundingInput`, `TheoryQuery`, `RetrievalTrace`, `GroundingBundle`, and `FinalizeInput`. Assert crisis rules override lower model output and empty is distinct from failed.

- [ ] **Step 2: Verify failure**

Run: `cd nx-backend/apps/server && go test ./internal/grounding -count=1`

Expected: FAIL because package does not exist.

- [ ] **Step 3: Implement exact contracts**

`GroundingInput` must include request/user/session/card IDs, question, summary, history, user main type, conversation-card main type, tier and rollout mode. `GroundingBundle` must include run ID, `SiteHits`, `KnowledgeHits`, `TheoryHits`, risk, intent, fallback, prompt token estimate and trace.

- [ ] **Step 4: Implement deterministic classifiers**

Use the spec keyword groups. Crisis/high rules can only raise the level. Unknown intent stays unknown if neither rules nor optional model classify it.

- [ ] **Step 5: Run tests and commit**

Run: `cd nx-backend/apps/server && go test ./internal/grounding -count=1`

```bash
git add nx-backend/apps/server/internal/grounding
git commit -m "feat(grounding): define retrieval and safety contracts"
```

### Task 2: Replace opaque knowledge retrieval with scored hybrid results

**Files:**
- Modify: `nx-backend/apps/server/internal/ragstore/vector.go`
- Create: `nx-backend/apps/server/internal/ragstore/retrieval.go`
- Create: `nx-backend/apps/server/internal/ragstore/retrieval_test.go`
- Create: `nx-backend/apps/server/internal/grounding/site_retriever.go`
- Create: `nx-backend/apps/server/internal/grounding/site_retriever_test.go`
- Modify: `nx-backend/apps/server/internal/server/miniapp_handlers.go`

- [ ] **Step 1: Write failing retrieval tests**

Prove vector-only hits survive without a lexical match, lexical-only hits survive without vectors, RRF combines both lists, site hits remain typed separately, vector outage yields `degraded` with lexical hits, and no hit yields `empty` without an error.

- [ ] **Step 2: Verify failure**

Run: `cd nx-backend/apps/server && go test ./internal/ragstore ./internal/grounding -run 'Retrieval|SiteRetriever' -count=1`

- [ ] **Step 3: Implement `SiteRetriever` and `KnowledgeRetriever` adapters**

`ragstore.HybridRetriever` owns knowledge lexical SQL, query embedding and vector SQL. Use title/tag exact boost plus trigram threshold `0.18`, cosine threshold `0.72`, RRF `k=60`, final minimum `0.015`; ties sort by exact-title hit, then final score, then stable numeric document ID. Return IDs without the old `kb-` string encoding. Include lexical/vector scores, content hash, duration, index version and stable error code. Keep existing admin CRUD APIs unchanged.

- [ ] **Step 4: Adapt server-owned site configuration without moving it into ragstore**

Expose a read-only `SiteDocumentProvider` from the existing `appRAGDocuments`/site-config assembly and wrap it with `grounding.SiteRetriever`. Milestone B shadow code calls typed site/knowledge retrievers directly. Leave the current live `rag.Service.search` path unchanged until C; add a deprecation comment but do not claim B removed it.

- [ ] **Step 5: Run tests and commit**

Run: `cd nx-backend/apps/server && go test ./internal/ragstore ./internal/grounding ./internal/server -run 'Retrieval|Vector|SiteRetriever' -count=1`

```bash
git add nx-backend/apps/server/internal/ragstore nx-backend/apps/server/internal/grounding/site_retriever.go nx-backend/apps/server/internal/grounding/site_retriever_test.go nx-backend/apps/server/internal/server/miniapp_handlers.go
git commit -m "refactor(rag): expose scored knowledge retrieval"
```

### Task 3: Implement active-release theory hybrid retrieval

**Files:**
- Create: `nx-backend/apps/server/internal/theoryretrieval/retriever.go`
- Create: `nx-backend/apps/server/internal/theoryretrieval/fusion.go`
- Create: `nx-backend/apps/server/internal/theoryretrieval/graph.go`
- Create: `nx-backend/apps/server/internal/theoryretrieval/retriever_test.go`
- Create: `nx-backend/apps/server/internal/theorystore/retrieval_store.go`
- Create: `nx-backend/apps/server/internal/theorystore/retrieval_store_test.go`

- [ ] **Step 1: Write failing RRF and graph tests**

Use defaults `trigram>=0.18`, `cosine>=0.72`, `RRF k=60`, final `>=0.015`, direct expansion `>=0.020`, relation confidence `>=0.75`, graph multiplier `0.60`, graph max 2. Test library key, active release, non-retired card, enabled chunk, ready embedding, matching embedding/content hash and stale exclusion.

- [ ] **Step 2: Verify failure**

Run: `cd nx-backend/apps/server && go test ./internal/theoryretrieval ./internal/theorystore -run 'Retrieval|Fusion|Graph' -count=1`

- [ ] **Step 3: Implement store queries**

Join `theory_libraries(key=$1) → theory_library_releases(status='active') → theory_release_cards → theory_cards(status<>'retired') → theory_chunks(status='enabled') → theory_chunk_embeddings(status='ready')`. Filter embedding model/dimensions from the active release and require embedding `content_hash=theory_chunks.content_hash`. Return work/domain fields needed for diversity deduplication. Lexical query uses pg_trgm when available and returns a degraded flag when falling back to bounded `ILIKE`.

- [ ] **Step 4: Implement fusion and one-hop expansion**

Deduplicate by chunk, then card/work/domain. Graph queries start from a card in the same active release, require relation status `published`, require the target card and target chunk in that release, and enforce confidence/one-hop limits. Direct hits always outrank graph-only hits at equal normalized score.

- [ ] **Step 5: Run tests and commit**

Run: `cd nx-backend/apps/server && go test ./internal/theoryretrieval ./internal/theorystore -count=1`

```bash
git add nx-backend/apps/server/internal/theoryretrieval nx-backend/apps/server/internal/theorystore/retrieval_store.go nx-backend/apps/server/internal/theorystore/retrieval_store_test.go
git commit -m "feat(theory): retrieve active theory releases"
```

### Task 4: Add retrieval run and candidate tracing

**Files:**
- Create: `nx-backend/apps/server/internal/grounding/trace_store.go`
- Create: `nx-backend/apps/server/internal/grounding/trace_store_test.go`
- Modify: `nx-backend/apps/server/internal/db/schema.sql`
- Modify: `nx-backend/apps/server/internal/db/schema_theory_library_test.go`

- [ ] **Step 1: Write failing lifecycle tests**

Assert B's shadow lifecycle `started→retrieved→shadow_completed` with null message IDs, plus `failed/cancelled`; reserve `generating/completed/persist_failed` behavior for C. Validate mutual-exclusion source checks and `ON DELETE RESTRICT` FKs for knowledge/theory sources.

- [ ] **Step 2: Verify failure**

Run: `cd nx-backend/apps/server && go test ./internal/grounding ./internal/db -run 'Trace|RetrievalRun|SourceReference' -count=1`

- [ ] **Step 3: Add trace DDL and store**

Create `rag_retrieval_runs` and `rag_retrieval_hits`. `SaveRetrieved` writes retained candidates in one transaction. In live modes, inability to `Start` is fatal; in `shadow`, the server wrapper logs `trace_start_failed`, skips shadow retrieval and continues the existing answer path. The orchestrator marks successful shadow runs `shadow_completed`; it never uses `generating` in B.

- [ ] **Step 4: Run tests and commit**

Run: `cd nx-backend/apps/server && go test ./internal/grounding ./internal/db -count=1`

```bash
git add nx-backend/apps/server/internal/grounding/trace_store.go nx-backend/apps/server/internal/grounding/trace_store_test.go nx-backend/apps/server/internal/db
git commit -m "feat(grounding): trace retrieval runs and hits"
```

### Task 5: Build the orchestrator and shadow wiring

**Files:**
- Create: `nx-backend/apps/server/internal/grounding/orchestrator.go`
- Create: `nx-backend/apps/server/internal/grounding/orchestrator_test.go`
- Modify: `nx-backend/apps/server/internal/config/env.go`
- Modify: `nx-backend/apps/server/internal/config/env_test.go`
- Modify: `nx-backend/apps/server/internal/server/server.go`
- Create: `nx-backend/apps/server/internal/server/app_chat_grounding_shadow.go`
- Create: `nx-backend/apps/server/internal/server/app_chat_grounding_shadow_test.go`
- Modify: `nx-backend/apps/server/internal/server/app_chat.go`
- Modify: `nx-backend/apps/server/internal/server/app_chat_stream_test.go`
- Modify: `nx-backend/apps/server/internal/server/app_chat_voice.go`
- Modify: `nx-backend/apps/server/internal/server/app_chat_voice_test.go`
- Modify: `nx-backend/apps/server/internal/server/app_chat_test.go`
- Modify: `.env.example`

- [ ] **Step 1: Write failing orchestration tests**

Assert order site/knowledge → topic expansion → theory, exact quotas, 1,200ms budget, deterministic fallbacks, and that shadow results never enter the current generator prompt.

- [ ] **Step 2: Verify failure**

Run: `cd nx-backend/apps/server && go test ./internal/grounding ./internal/server -run 'Orchestrator|Shadow' -count=1`

- [ ] **Step 3: Implement configurable defaults**

In B, accept only `THEORY_GROUNDING_MODE=off|shadow`, plus thresholds, stage timeouts and library key. Unknown values—including premature `staff|enabled`—must fail configuration validation instead of silently behaving as shadow. C extends the enum after live generation is implemented. Default is `off`.

- [ ] **Step 4: Implement shadow execution**

Call the shadow helper explicitly from sync text and SSE paths in `app_chat.go` and the recognized-transcript path in `app_chat_voice.go`, after complete context assembly and before live generation. Per-entry tests must assert full user/card/history context, exactly one shadow call, no shadow sources in the generator prompt, `shadow_completed` on success, and unchanged HTTP/SSE/voice response on failure.

- [ ] **Step 5: Run verification and commit**

Run: `cd nx-backend/apps/server && go test ./...`

```bash
git add nx-backend/apps/server/internal/grounding nx-backend/apps/server/internal/config nx-backend/apps/server/internal/server .env.example
git commit -m "feat(grounding): run theory retrieval in shadow mode"
```

### Task 6: Milestone B performance and fallback checkpoint

**Files:**
- Create: `nx-backend/apps/server/internal/grounding/orchestrator_budget_test.go`
- Create: `nx-backend/apps/server/internal/grounding/retrieval_fixture_test.go`
- Create: `nx-backend/apps/server/internal/grounding/orchestrator_benchmark_test.go`
- Create: `nx-backend/apps/server/internal/grounding/performance_integration_test.go`

- [ ] **Step 1: Add failing deterministic budget and fixture tests**

Use fake-clock/cancellable retrievers to assert the 450/550/100/100ms stage budgets, fallback and threshold configuration appearing in trace. Add a fixed retrieval corpus that verifies stable ordering and ties. Keep `BenchmarkOrchestratorShadow` informational only.

- [ ] **Step 2: Verify the new tests fail, then implement the missing budget hooks**

Run: `cd nx-backend/apps/server && go test ./internal/grounding -run 'Budget|Fixture' -count=1`

Expected: FAIL before the hooks/fixtures are complete, then PASS after the minimal implementation.

- [ ] **Step 3: Run full backend checks**

Run:

```bash
cd nx-backend/apps/server
gofmt -w internal/grounding/*.go internal/theoryretrieval/*.go internal/theorystore/*.go internal/ragstore/*.go internal/server/app_chat_grounding_shadow*.go internal/config/*.go
go test ./...
go vet ./...
git diff --check
```

Expected: PASS; shadow mode leaves current answer tests unchanged.

- [ ] **Step 4: Add and run a reproducible PostgreSQL performance gate**

With isolated `TEST_DATABASE_URL`, load a deterministic fixture of 5,000 knowledge rows, 500 theory chunks, 50 relations and precomputed 1536-value vectors. Warm up 20 requests, then issue 200 mixed fixture queries at concurrency 10. Sort measured grounding durations in the test and fail unless P95 ≤ 1,200ms and P99 ≤ 2,000ms. Do not include model generation.

Run:

```bash
cd nx-backend/apps/server
THEORY_PERF_TEST=1 go test ./internal/grounding -run TestGroundingPerformanceGate -count=1 -v
go test ./internal/grounding -run '^$' -bench OrchestratorShadow -benchmem
```

Expected: performance test prints fixture size, P50/P95/P99 and PASS; benchmark is informational.

- [ ] **Step 5: Commit**

```bash
git add nx-backend/apps/server/internal/grounding
git commit -m "test(grounding): lock shadow budgets and ranking fixtures"
```
