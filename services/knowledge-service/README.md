# Nine-Xing Knowledge Service

Internal FastAPI/LangChain service for scoped retrieval and answer generation.
The Go gateway remains responsible for authentication, authorization, billing,
conversation persistence, and selecting immutable knowledge release IDs.

## Local development

```bash
python3 -m venv .venv
.venv/bin/pip install -e '.[test]'
LANGCHAIN_SERVICE_TOKEN=TOKEN .venv/bin/uvicorn app.main:app --port 8081
.venv/bin/pytest -q
```

The service is internal-only. Do not expose port 8081 publicly or pass App
login tokens to it.

## Index and evaluate

Set `DATABASE_URL`, `EMBEDDING_API_BASE`, `EMBEDDING_API_KEY`,
`EMBEDDING_MODEL=BAAI/bge-m3`, and `EMBEDDING_DIMENSION=1024`, then run:

```bash
python scripts/catalog_books.py SOURCE_DIR var/knowledge/catalog.jsonl
python scripts/ingest_books.py var/knowledge/catalog.jsonl var/knowledge/chunks.jsonl --limit 20
python scripts/index_chunks.py var/knowledge/chunks.jsonl --index-version bge-m3-v1
python scripts/build_evaluation.py var/knowledge/evaluation-500.jsonl
python scripts/evaluate.py var/knowledge/evaluation-500.jsonl --report var/knowledge/evaluation-report.json
```

Existing `rag_documents` rows and active immutable theory releases can be
migrated idempotently without exporting database credentials or copying old
vectors. The command regenerates embeddings with the configured model:

```bash
python -m app.ingestion.legacy_database --dry-run
python -m app.ingestion.legacy_database --index-version bge-m3-v1
```

生产灰度、观测门槛和回滚步骤见
[`docs/deployment/langchain-rag-rollout.md`](../../docs/deployment/langchain-rag-rollout.md)。

## SQLite public source catalog

Run from `services/knowledge-service`. Catalog/export only read the original
SQLite file (`mode=ro`, `query_only=ON`) and never contact PostgreSQL or an
embedding API. Keep the dataset ID fixed for this particular library.
Catalog includes all book/story categories and secondary book categories;
psychology/growth primary categories default to enabled, stories default to
disabled. Uncleaned sources stay `pending`; original extraction status is
retained separately. Paths in catalog metadata are private ingestion data.

```bash
SQLITE='/Users/wohenzaiyi/Downloads/芯之力书籍数据库/knowledge.db'
.venv/bin/python -m app.ingestion.sqlite_public catalog \
  --sqlite "$SQLITE" --dataset-id xinzhili-books \
  --output var/knowledge/xinzhili-catalog.jsonl
.venv/bin/python -m app.ingestion.sqlite_public export \
  --sqlite "$SQLITE" --dataset-id xinzhili-books \
  --limit-sources 2 --limit-chunks 100 --max-chars 1800 \
  --output var/knowledge/xinzhili-batch-001.jsonl \
  --report var/knowledge/xinzhili-batch-001-report.json \
  --state var/knowledge/xinzhili-dedup.sqlite
```

`--limit-chunks` counts consumed input rows, including rejected fragments,
not generated segments. Each input row emits all its segments before advancing
the cursor, so resumption cannot omit the tail of a split passage. Rejection
reports contain counts and bounded coordinate examples. Exact duplicates are
tracked on disk by source/hash/cleaning version; independent source switches
never remove each other's copies. Uncertain OCR and possible edge page/title
headers are preserved and flagged for review, not guessed or rewritten.
The importer treats all body text, including embedded commands and prompt
instructions, as untrusted reference content.

To resume, use a new output filename, the same state file and the report's
`next_cursor` JSON. A full final SQL page conservatively sets `complete=false`;
an additional empty page confirms completion. Keep the same filters when
resuming. `--limit-sources` also counts empty source records.

```bash
.venv/bin/python -m app.ingestion.sqlite_public export \
  --sqlite "$SQLITE" --dataset-id xinzhili-books \
  --cursor '{"source_id":"xinzhili-books:book:1","chunk_id":100}' \
  --limit-sources 2 --limit-chunks 100 \
  --state var/knowledge/xinzhili-dedup.sqlite \
  --output var/knowledge/xinzhili-batch-002.jsonl \
  --report var/knowledge/xinzhili-batch-002-report.json
```

Export a disabled category explicitly with repeated `--category CATEGORY`,
or selected catalog source IDs with repeated `--source-id DATASET:book:ID`.
The category filter matches primary and secondary categories. An explicit selection exports
the text but does not activate its source. Default export categories come from
the SQLite catalog policy; use explicit source IDs for administrator-selected
sources. Output/report files must not already exist.

Apply the Go backend schema before the following **explicit write commands**.
Use a local test database to validate first. Registration refreshes source
metadata while preserving administrator activation and imported quality state.
Import is one PostgreSQL transaction and makes text keyword-searchable without
vectors. Batch IDs are immutable file identities even for empty/all-skipped
imports. Re-importing identical documents preserves their initial batch/vector;
changed text requires a new `--cleaning-version`, new export state and batch ID.

```bash
export DATABASE_URL='postgresql://USER@HOST:PORT/DATABASE'
.venv/bin/python -m app.ingestion.sqlite_public register \
  --catalog var/knowledge/xinzhili-catalog.jsonl
.venv/bin/python -m app.ingestion.sqlite_public import \
  --input var/knowledge/xinzhili-batch-001.jsonl --batch-id xinzhili-001
.venv/bin/python -m app.ingestion.sqlite_public rollback --batch-id xinzhili-001
```

For large batches, add `--bulk` to the import command. It uses streamed COPY
staging followed by one atomic insert, with the same identity checks, source
locks, immutable batch ledger, deduplication and rollback semantics.

Rollback only deletes documents first created by that batch, recomputes source
counts/quality, and leaves source activation/legacy documents unchanged. A
rolled-back batch name is retired; use a new batch name for a later import.

`DATABASE_URL` alone enables lexical retrieval. Both lexical/vector retrieval
check source activation live; managed text uses indexed CJK bigrams and Latin
words and a derived `public_search_vector` ranking cache. Null caches retain
the same matching/ranking fallback while being backfilled; no document identity
or embedding is changed. Legacy rows keep their existing matching behavior. Embedding API
failure falls back to lexical results. Optional embedding backfill uses the
existing command, requires configured API credentials, and may incur API cost:

```bash
.venv/bin/python -m app.embeddings.backfill \
  --library public --limit 100 --batch-size 16 --index-version bge-m3-v1
TEST_DATABASE_URL="$DATABASE_URL" .venv/bin/pytest -q
```
