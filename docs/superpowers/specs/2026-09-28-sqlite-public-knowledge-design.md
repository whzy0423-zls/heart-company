# SQLite Public Knowledge Import

## Approved Scope

All categories in the provided SQLite book database enter an admin-selectable source catalog. Body text is cleaned and imported in bounded batches. Psychology, personality, communication, learning, emotional growth and counselling categories default to enabled. Literature, history, philosophy and story categories default to disabled. Selection is admin-only; no App UI changes or production database writes in this task.

The initial local-only scope above was subsequently expanded by the user to
production synchronization of the complete corpus after increasing server
storage. See `docs/deployment/sqlite-public-import-2026-09-28.md` for that
explicitly approved deployment, batch ledger and verification status. Category
activation defaults remain unchanged; bulk embedding generation remains out of scope.

## Architecture

- Read the original SQLite database in read-only mode, including `books`, `book_categories`, `library_categories`, and `story_library_sources`. Stable identifiers namespace the explicit dataset ID and original record ID.
- Keep a `public_knowledge_sources` PostgreSQL catalog separate from manual `rag_documents`. Preserve all categories, original extraction statuses and locators. Catalog refreshes must preserve administrator activation choices.
- Associate imported `knowledge_documents` with nullable `public_source_id`, `import_batch_id`, and indexed `search_text`. Existing documents with no catalog association retain their current behavior. Catalog activation is checked live in both lexical and vector retrieval and in the Go local public search.
- Stream SQLite chunks through conservative normalization, advertisement/empty/garbage rejection, bounded splitting and content-hash deduplication. Preserve provenance; do not reinterpret text as instructions, reconstruct missing content, or treat keyword tags as evidence of personality type.
- Use a CLI to preview the complete catalog, export bounded cleaned JSONL batches, register the catalog, import into PostgreSQL, optionally generate embeddings, and roll back a named batch. Writing or paid embedding calls require explicit CLI commands, not preview.
- Add a book catalog tab to existing admin knowledge management: category/status filters, pagination, selection activation, and source/chunk preview. Reuse existing authentication and `RAG:Knowledge:Manage` permission.

## Shared Contracts

Table `public_knowledge_sources`: `id TEXT PRIMARY KEY`, `dataset_id TEXT`, `source_kind TEXT` (`book` or `story`), `source_record_id BIGINT`, `title TEXT`, `category TEXT`, `file_format TEXT`, `extract_status TEXT`, `source_chunks INT`, `text_chars BIGINT`, `enabled BOOLEAN`, `quality_status TEXT` (`pending`, `ready`, `needs_review`), `imported_chunks INT`, `metadata JSONB`, `create_time`, `update_time`.

Managed document fields: `public_source_id TEXT NULL REFERENCES public_knowledge_sources(id)`, `import_batch_id TEXT NULL`, `search_text TEXT NOT NULL DEFAULT ''`. Managed document IDs include dataset/source/chunk coordinates. Batch IDs are immutable per import and recorded for rollback. Catalog enabled defaults are applied only on insert, not refresh.

Deduplication is source-scoped for managed imports: independently selectable sources must each retain their usable passages. Restrict the existing global embedding identity unique index to unmanaged documents and add a managed `(public_source_id, content_hash, index_version)` unique index. Same-ID/same-content re-import is a no-op preserving the first import batch and any existing vector. Changed content with the same ID is rejected; a new cleaning version produces new IDs. Rollback deletes only rows originally created by the named batch and recomputes affected source counts.

Admin endpoints:
- `GET /api/rag/sources?keyword=&category=&enabled=&qualityStatus=&datasetId=&page=1&pageSize=20`: `{items,total,page,pageSize,categories:[{name,count}]}`. Source responses use camelCase field names.
- `POST /api/rag/sources/status`: `{ids:string[],enabled:boolean}`; bounded, validated, atomic selection update; returns `{updated:number}`.
- `GET /api/rag/sources/{id}/chunks`: `{items:[{id,title,content,locator,importBatchId}],total:number}`; returns a small preview, not the full source body.

## Cleaning and Scale

Full catalog construction does not scan all body text. Export uses source and chunk indices, `--limit-sources` and `--limit-chunks`, explicit source/category filters and a resumable source/chunk cursor. Reject counts and examples are reported separately. Exact duplicate detection uses source-scoped disk-backed state for exported batches; OCR uncertainty and suspected page/title headers are flagged rather than silently rewritten. Approximate/semantic duplicate detection is deferred to a later quality review stage. Export defaults to enabled growth categories; other categories can be explicitly selected without implicitly enabling them.

Managed Chinese lexical text contains CJK bigrams and Latin/numeric words so PostgreSQL simple full-text search can use a GIN index. No full-library process preload and no unbounded substring scan for managed body text. Embeddings are optional at first: imported text is keyword-searchable immediately, then backfilled with the configured model and vector dimension.

The Python dependency factory must support a lexical-only retriever with just `DATABASE_URL`; embedding failure must still permit lexical results. Use the nullable live gate `public_source_id IS NULL OR EXISTS(enabled matching source)` rather than an inner join or an orphan-permissive default. Integrate managed local retrieval at `retrieveAppDocsForQuery`, including direct caller/fallback paths, outside any cached legacy candidate list.

## Validation

SQLite fixtures cover categories, read-only source access, status handling, cleaning, deterministic provenance, bounded pagination and duplicates. PostgreSQL tests cover idempotent registration/import, administrator choice preservation, enabled/disabled retrieval parity, rollback and unaffected legacy documents. Go tests cover permissions/contracts and local search filters. Frontend tests cover filters, pagination, activation and failure states. Run a catalog and small cleaning preview against the real database; preserve the original file and do not contact production services.
