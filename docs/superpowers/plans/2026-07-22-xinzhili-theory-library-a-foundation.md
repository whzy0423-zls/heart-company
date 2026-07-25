# 芯之力理论库 A：Schema、目录与审核纵切 Implementation Plan

> **For agentic workers:** REQUIRED: Use superpowers:subagent-driven-development (if subagents available) or superpowers:executing-plans to implement this plan. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** 建立理论库的数据库、来源目录、理论卡审核和原子发布基础，并跑通“一份来源 → 一张卡 → 一个 chunk → 一个 active release”的最小纵切。

**Architecture:** PostgreSQL 保存规范作品、物理文件、版本化理论卡、来源证据、关系、检索块、embedding 元数据和发布快照。`theory_release_cards` 是 release 与 chunk 的唯一映射来源，`theory_chunks` 不重复保存 release ID。`internal/theorystore` 按 catalog、card、release 三个职责拆文件。

**Tech Stack:** Go 1.22, PostgreSQL 16, pgvector, `database/sql`, existing idempotent `schema.sql`, Go tests.

**Spec:** `docs/superpowers/specs/2026-07-22-xinzhili-theory-library-design.md`

---

## Chunk 1: Foundation vertical slice

### Task 1: Lock the database contract with failing schema tests

**Files:**
- Create: `nx-backend/apps/server/internal/db/schema_theory_library_test.go`
- Modify: `nx-backend/apps/server/internal/db/schema.sql`

- [ ] **Step 1: Write the failing schema contract test**

Add table-driven assertions for foundation tables from design sections 5.1–5.9 and the relevant 5.12 constraints. Retrieval audit tables are explicitly deferred to B; chat-message source tables are deferred to C.

```go
func TestSchemaIncludesTheoryLibraryFoundation(t *testing.T) {
    raw, err := os.ReadFile("schema.sql")
    if err != nil { t.Fatal(err) }
    sqlText := strings.Join(strings.Fields(string(raw)), " ")
    required := []string{
        "CREATE TABLE IF NOT EXISTS theory_libraries",
        "CREATE TABLE IF NOT EXISTS theory_library_releases",
        "CREATE TABLE IF NOT EXISTS theory_source_works",
        "CREATE TABLE IF NOT EXISTS theory_source_files",
        "CREATE TABLE IF NOT EXISTS theory_cards",
        "CREATE TABLE IF NOT EXISTS theory_practices",
        "CREATE TABLE IF NOT EXISTS theory_card_relations",
        "CREATE TABLE IF NOT EXISTS theory_card_sources",
        "CREATE TABLE IF NOT EXISTS theory_chunks",
        "CREATE TABLE IF NOT EXISTS theory_chunk_embeddings",
        "CREATE TABLE IF NOT EXISTS theory_release_cards",
        "CREATE UNIQUE INDEX IF NOT EXISTS uq_theory_active_release",
        "CHECK (from_card_id <> to_card_id)",
        "CHECK (page_end IS NULL OR page_start IS NULL OR page_end >= page_start)",
    }
    for _, fragment := range required {
        if !strings.Contains(sqlText, fragment) { t.Errorf("missing %q", fragment) }
    }
}
```

Also assert `theory_chunks` does not contain `release_id`, and `theory_release_cards` has unique `(release_id, chunk_id)`.

- [ ] **Step 2: Run the focused test and verify failure**

Run: `cd nx-backend/apps/server && go test ./internal/db -run TheoryLibrary -count=1`

Expected: FAIL with missing theory table fragments.

- [ ] **Step 3: Add idempotent DDL**

Add the foundation schema from design sections 5.1–5.9 plus its 5.12 constraints. Use explicit `CHECK` constraints for statuses, score ranges, page ranges, card kinds, evidence levels, safety levels, and `authority_level BETWEEN 1 AND 5`. Add indexes on every FK and common filter.

Create `theory_chunk_embeddings` first as a metadata table without an `embedding` column. In the existing exception-safe pgvector `DO $$` block, conditionally run:

```sql
ALTER TABLE theory_chunk_embeddings ADD COLUMN IF NOT EXISTS embedding vector(1536);
CREATE INDEX IF NOT EXISTS idx_theory_chunk_embeddings_hnsw
  ON theory_chunk_embeddings USING hnsw (embedding vector_cosine_ops);
```

Plain PostgreSQL therefore keeps catalog/review/release features and reports vector capability as unavailable; retrieval code in B must detect the optional column.

Use these invariants exactly:

```sql
CREATE UNIQUE INDEX IF NOT EXISTS uq_theory_active_release
  ON theory_library_releases(library_id) WHERE status = 'active';

CREATE UNIQUE INDEX IF NOT EXISTS uq_theory_published_card_key
  ON theory_cards(library_id, canonical_key) WHERE status = 'published';

CREATE UNIQUE INDEX IF NOT EXISTS uq_theory_release_chunk
  ON theory_release_cards(release_id, chunk_id);
```

Do not add `release_id` to `theory_chunks` or `theory_chunk_embeddings`.

Persist `theory_library_releases.retrieval_mode` with `CHECK (retrieval_mode IN ('lexical_only','hybrid'))`. A `lexical_only` release does not require an embedding provider or vector row; a `hybrid` release requires pgvector capability and all vector readiness checks.

- [ ] **Step 4: Run schema and full DB tests**

Run: `cd nx-backend/apps/server && go test ./internal/db -count=1`

Expected: PASS.

- [ ] **Step 5: Commit the schema contract**

```bash
git add nx-backend/apps/server/internal/db/schema.sql nx-backend/apps/server/internal/db/schema_theory_library_test.go
git commit -m "feat(theory): add theory library schema"
```

### Task 2: Define validated theory domain models

**Files:**
- Create: `nx-backend/apps/server/internal/theorystore/models.go`
- Create: `nx-backend/apps/server/internal/theorystore/models_test.go`

- [ ] **Step 1: Write failing validation tests**

Cover invalid status, authority outside 1–5, extraction quality outside 0–1, low-quality source publication, self relation, invalid page range, and missing source on publish.

```go
func TestValidateCardForPublishRequiresSafetyAndSource(t *testing.T) {
    card := Card{CanonicalKey: "observer", CanonicalName: "内在观察者", Status: StatusPublished}
    if err := ValidateCardForPublish(card, nil); err == nil {
        t.Fatal("expected missing evidence/safety/source error")
    }
}
```

- [ ] **Step 2: Verify the tests fail**

Run: `cd nx-backend/apps/server && go test ./internal/theorystore -run Validate -count=1`

Expected: FAIL because package/types do not exist.

- [ ] **Step 3: Implement focused models and validators**

Define constants and structs for `Library`, `Release`, `SourceWork`, `SourceFile`, `Card`, `Practice`, `Relation`, `CardSource`, `Chunk`, and `EmbeddingRecord`. Persist `practice_schema_version` on `theory_practices`; the initial supported identifier is `xinzhili.practice.v1`. Keep JSON fields as `json.RawMessage`; validate safety-critical practice JSON against that schema before publish.

- [ ] **Step 4: Run focused tests**

Run: `cd nx-backend/apps/server && go test ./internal/theorystore -count=1`

Expected: PASS.

- [ ] **Step 5: Commit the models**

```bash
git add nx-backend/apps/server/internal/theorystore
git commit -m "feat(theory): define validated theory models"
```

### Task 3: Implement source catalog registration and deduplication

**Files:**
- Create: `nx-backend/apps/server/internal/theorystore/catalog_store.go`
- Create: `nx-backend/apps/server/internal/theorystore/catalog_store_test.go`

- [ ] **Step 1: Write failing store tests with a recording SQL driver**

Test `RegisterWork`, `RegisterFile`, `FindFileBySHA256`, `MarkDuplicate`, and `UpdateExtractionStatus`. Assert a duplicate SHA is registered as a separate physical file with `duplicate_of_file_id`, not rejected by a unique constraint. Add same-hash/different-work, self-duplicate, duplicate cycle, and different-hash target rejection cases.

- [ ] **Step 2: Verify failure**

Run: `cd nx-backend/apps/server && go test ./internal/theorystore -run 'Catalog|Duplicate' -count=1`

Expected: FAIL because catalog methods are absent.

- [ ] **Step 3: Implement catalog-only SQL methods**

Every method must use a 10-second derived context, trim paths/titles, require 64 lowercase hex SHA-256, and return stable errors such as `ErrInvalidSHA256` and `ErrWorkNotFound`.

- [ ] **Step 4: Run tests**

Run: `cd nx-backend/apps/server && go test ./internal/theorystore -count=1`

Expected: PASS.

- [ ] **Step 5: Commit**

```bash
git add nx-backend/apps/server/internal/theorystore/catalog_store.go nx-backend/apps/server/internal/theorystore/catalog_store_test.go
git commit -m "feat(theory): add source catalog store"
```

### Task 4: Implement card review and atomic release activation

**Files:**
- Create: `nx-backend/apps/server/internal/theorystore/card_store.go`
- Create: `nx-backend/apps/server/internal/theorystore/card_store_test.go`
- Create: `nx-backend/apps/server/internal/theorystore/content_store.go`
- Create: `nx-backend/apps/server/internal/theorystore/content_store_test.go`
- Create: `nx-backend/apps/server/internal/theorystore/release_store.go`
- Create: `nx-backend/apps/server/internal/theorystore/release_store_test.go`

- [ ] **Step 1: Write failing release transaction tests**

Write three focused RED groups: `TestCardStore*` for card create/update and transitions; `TestContentStore*` for sources/practices/relations/chunks/embedding metadata; `TestReleaseStore*` for locking, activation and rollback. Verify a release cannot map a chunk whose `card_id` differs from the mapped card.

- [ ] **Step 2: Verify failure**

Run: `cd nx-backend/apps/server && go test ./internal/theorystore -run 'Release|Publish' -count=1`

Expected: FAIL.

- [ ] **Step 3: Implement review transitions**

Expose only these transitions: `draft→in_review`, `in_review→draft`, `in_review→published`, `published→superseded`, `superseded→retired`. Validate card sources before publish.

- [ ] **Step 4: Implement release build and activation**

`BuildRelease` inserts `theory_release_cards` only after verifying every chunk belongs to the supplied card. `ActivateRelease` uses one transaction and `SELECT ... FOR UPDATE` on the library.

Before a release can become ready or active, validate all of the following: at least one mapped enabled chunk; every mapped card is publishable; `card_count` and `chunk_count` equal the actual mapping. For `hybrid`, each chunk content hash must match a non-null ready 1536-dimensional vector for the configured model. For `lexical_only`, vector rows are optional and ignored. Reject `hybrid` when vector capability is unavailable.

- [ ] **Step 5: Run package and DB tests**

Run: `cd nx-backend/apps/server && go test ./internal/theorystore ./internal/db -count=1`

Expected: PASS.

- [ ] **Step 6: Commit**

```bash
git add nx-backend/apps/server/internal/theorystore/card_store.go nx-backend/apps/server/internal/theorystore/card_store_test.go nx-backend/apps/server/internal/theorystore/content_store.go nx-backend/apps/server/internal/theorystore/content_store_test.go nx-backend/apps/server/internal/theorystore/release_store.go nx-backend/apps/server/internal/theorystore/release_store_test.go
git commit -m "feat(theory): publish cards through atomic releases"
```

### Task 5: Add and verify the one-card vertical slice

**Files:**
- Create: `nx-backend/scripts/db/seed-xinzhili-theory-vertical-slice.sql`
- Create: `nx-backend/apps/server/internal/db/theory_vertical_slice_test.go`
- Create: `nx-backend/apps/server/internal/theorystore/postgres_integration_test.go`

- [ ] **Step 1: Write a failing seed-content contract test**

Assert the seed contains library `xinzhili`, one source work, one physical file placeholder, one reviewed source link, card `inner_observer`, one enabled chunk and one active `lexical_only` release mapping. The foundation seed is deterministic and never calls an embedding provider.

- [ ] **Step 2: Verify failure**

Run: `cd nx-backend/apps/server && go test ./internal/db -run TheoryVerticalSlice -count=1`

Expected: FAIL because seed file is absent.

- [ ] **Step 3: Add an idempotent seed**

Use `INSERT ... ON CONFLICT ... DO UPDATE` and stable keys. Do not include copyrighted full text; keep the source quotation short and mark it verified only if it came from a checked original page.

- [ ] **Step 4: Add isolated PostgreSQL integration coverage**

Using `TEST_DATABASE_URL` and `testutil.ValidateIsolatedPostgresDSN`, execute `schema.sql` twice, then exercise invalid CHECK/FK constraints, duplicate rules, the theorystore vertical slice, concurrent activation attempts, rollback injection, seed execution and a query from active release back to source/card/chunk. When pgvector is present, separately build a `hybrid` release with an in-test deterministic 1536-value vector and model `test-fixture-1536`; do not use an external embedding API. Skip only when `TEST_DATABASE_URL` is absent.

- [ ] **Step 5: Run unit and PostgreSQL integration tests**

Run: `cd nx-backend/apps/server && go test ./...`

Expected: PASS.

- [ ] **Step 6: Commit**

```bash
git add nx-backend/scripts/db/seed-xinzhili-theory-vertical-slice.sql nx-backend/apps/server/internal/db/theory_vertical_slice_test.go nx-backend/apps/server/internal/theorystore/postgres_integration_test.go
git commit -m "test(theory): prove the vertical publish slice"
```

### Task 6: Milestone A verification checkpoint

- [ ] **Step 1: Run formatting and full verification**

Run:

```bash
cd nx-backend/apps/server
gofmt -w internal/theorystore/*.go internal/db/schema_theory_library_test.go internal/db/theory_vertical_slice_test.go
go test ./...
go vet ./...
git diff --check
```

Expected: all commands exit 0.

- [ ] **Step 2: Inspect only milestone files**

Run: `git status --short && git diff main...HEAD --stat`

Expected: clean worktree; changes limited to schema, theorystore, seed, tests, and this plan/spec lineage.
