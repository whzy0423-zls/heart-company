# SQLite Public Knowledge Import Implementation Plan

> **For agentic workers:** Use superpowers:subagent-driven-development for the independent implementation tasks and review; use test-driven-development and verification-before-completion. Do not commit without a user request.

**Goal:** Make all SQLite source categories selectable in backend management, with bounded cleaned imports and growth-category activation defaults.

**Architecture:** Read-only SQLite catalog/export CLI feeds the existing PostgreSQL knowledge service. A source catalog controls live retrieval visibility; the existing knowledge admin page gains a separate source tab. Existing manual/theory/skill content remains unchanged.

**Tech Stack:** Python 3.12, SQLite, psycopg/PostgreSQL, Go, Vue 3/Ant Design Vue, pytest, Go testing, Vitest.

## Chunk 1: Independent Implementation

### Task 1: Source Catalog and Go Admin

Files: `nx-backend/apps/server/internal/db/schema.sql`; new `internal/publicknowledge` store/tests; new `internal/server/public_knowledge_admin.go` and tests; targeted `internal/server/server.go` integration.

- [x] Write failing tests for catalog schema, list filters/status validation, authenticated routes and disabled-source exclusion.
- [x] Add catalog schema and document association/index columns following the spec contracts.
- [x] Implement bounded catalog list, atomic activation, preview and indexed local public search; wire permission-protected routes.
- [x] Run focused Go tests and package build. Keep existing manual public search and caches unchanged.

### Task 2: Streaming SQLite Import and Knowledge Repository

Files: new `services/knowledge-service/app/ingestion/sqlite_public.py` and focused helper modules as needed; tests under `tests/ingestion`; targeted `app/repositories/documents.py`; `README.md`.

- [x] Write failing fixture tests for complete catalog/defaults, cleaning, read-only access, stable IDs, limits/resume, dedup and re-import.
- [x] Implement `python -m app.ingestion.sqlite_public` subcommands `catalog`, `export`, `register`, `import`, `rollback`. Preview/export operate without PostgreSQL or API secrets.
- [x] Register sources idempotently without overwriting admin enabled choices; import streamed cleaned JSONL into managed public documents. Enforce dataset/source association, immutable batch provenance, limits, atomic batch writes and rollback.
- [x] Add managed-source live filters to vector/lexical queries; use indexed CJK-bigram search for imported text, leaving legacy search behavior unchanged.
- [x] Run pytest and fixture PostgreSQL tests; document exact preview, import/backfill and rollback commands.

### Task 3: Admin Source Selector

Files: new source API/component and tests in `nx-backend/apps/web-antd/src`; targeted `views/rag/knowledge.vue` and API exports.

- [x] Write failing frontend tests for API contracts, filters, preview and activation state/errors.
- [x] Add source directory tab with category/status search, pagination, per-source and batch activation, imported/source counts and quality badges.
- [x] Preserve manual document controls and existing regression tests. Use the current design system and responsive overflow handling.
- [x] Run focused Vitest and TypeScript checks. No server-side file paths/secrets are exposed in the UI.

## Chunk 2: Integration and Verification

- [x] Review spec/plan and independent changes for contract consistency.
- [x] Test on an isolated local PostgreSQL database with pgvector if available; otherwise report the exact integration gap.
- [x] Run real read-only catalog preview for all categories and bounded body cleaning preview; record counts and rejected examples without modifying source.
- [x] Verify frontend view at desktop/mobile viewport when local admin server is practical.
- [x] Run relevant Python/Go/frontend suites, inspect diffs, document verified behavior and pending production import.

## Verified Local State (2026-09-28)

- Implementation repository: `/Users/wohenzaiyi/Desktop/nine-xing`; the App repository is unchanged.
- Real read-only catalog: 4,756 sources, 30 populated primary categories, 711 default-enabled psychology/growth sources. Secondary categories are retained and filterable.
- The isolated local preview database contains this catalog and 130 imported excerpts: 100 psychology excerpts and 30 Enneagram excerpts. These are bounded samples, not a complete book/body import.
- Exact repeated import inserted 0 and skipped 100; rolling back the duplicate batch deleted 0 and retained the original documents. The advertisement sample was rejected with coordinates in its report.
- Python suite with local PostgreSQL: 67 passed. Focused frontend verification: 17 tests across 7 files; final source-directory rerun: 13 tests across 3 files. Frontend typecheck and Go build passed. Relevant Go publicknowledge/db/server PostgreSQL tests passed; the same packages also passed without database integration enabled.
- Playwright verified desktop/mobile catalog, source activation and excerpt drawer, with no page errors. The mobile viewport and document width both measured 390px; the preview drawer measured 366px and remained within the viewport.
- Broad PostgreSQL regression caveat: `TestTheoryVerticalSliceSeedExecutesTwice` fails with immutable snapshot `P0001`, also reproduced on baseline `HEAD 9c04d15` in an independent database. A concurrent ownership test produced one `40P01`; it did not recur in 102 baseline or 20 current repetitions, so its cause remains unconfirmed. Do not describe the entire PostgreSQL regression suite as passing.
- No production writes, embedding API calls, original SQLite modifications or commits were performed. Approximate/semantic deduplication remains deferred; suspected OCR/page headers are preserved with review flags.

## Production Handoff

Use `services/knowledge-service/README.md` for the explicit CLI commands. Deploy/apply the backend schema, register the complete catalog, then export/import bounded reviewed batches with stable dataset and cleaning-version identities. Catalog activation alone does not import body text. Begin with the default growth categories, inspect rejection/review reports and previews, then opt into additional categories. Paid embedding backfill is optional and remains a separate explicit operation.

Local admin preview: `http://127.0.0.1:4319/rag/knowledge`. The preview database and processes are temporary local fixtures; the complete cleaned production corpus is still pending.
