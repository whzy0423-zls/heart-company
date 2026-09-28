# Public Knowledge Online Query Budgets

The imported corpus has 3,645,921 managed segments. These segments currently
have no bulk-generated embeddings; existing vectors and new lexical results
are combined. All source activation and library/safety filters remain live.

## Exact Vector Access

After import, 97.74% of rows have no vector. The existing exact vector query
keeps deterministic `ORDER BY embedding <=> query_vector, id`; switching to
HNSW-only ordering produced a cold query longer than 27 seconds despite very
fast warm results, so this release retains exact ranking.

Run this independently of a transaction, after checking PostgreSQL/vector
readiness:

```sql
CREATE INDEX CONCURRENTLY IF NOT EXISTS idx_knowledge_documents_embedded_scope
ON knowledge_documents(library_kind, release_id)
WHERE embedding IS NOT NULL;
```

Check `pg_index.indisvalid` and `indisready` before using the new access path.
If a concurrent build is interrupted, inspect and drop only its invalid index
concurrently before retrying. Never drop a valid index just to force a rebuild.
This index contains no activation snapshot and needs no refresh when a source
is enabled or disabled. It supports all library scopes and does not duplicate
large vector values in an `INCLUDE` clause. Keep it during code rollback.

## Request Budgets

- Online embedding requests use zero retries and a 2-second HTTP phase timeout.
- Exact vector SQL has a transaction-local 5-second statement timeout; failure
  retains lexical results through the existing semantic fallback.
- Lexical SQL has a transaction-local 8-second statement timeout and 64 MB
  bitmap memory. Live source IDs are refreshed on every connection.
- The Go managed-public fallback has its own 5-second child context and never
  extends a shorter caller deadline.
- The production release Compose override sets the remote gateway retrieval
  budget to 15,000 ms. This is a bounded operational allowance, not a P95 SLA.

These are HTTP phase and SQL statement budgets, not a strict whole-call
Python deadline. The live source-ID and vector-dimension metadata queries
run separately; the Go gateway enforces the outer request budget.

Batch ingestion/backfill clients retain their existing retry defaults. No bulk
embedding requests are made by this release. Query-level limits do not change
global PostgreSQL settings, catalog choices, ranking, source provenance, or
private/library isolation.

The release tests cover online versus batch retry policy, SQL timeout locality,
lexical fallback after vector cancellation/provider failure, source filtering,
and Go deadline propagation. Production checks must additionally verify an
actual 8-result App-shaped request and the non-null partial-index plan.
