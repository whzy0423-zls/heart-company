# SQLite Public Knowledge Production Import

User approved complete production synchronization after expanding storage.
All 4,756 sources/30 primary categories are registered; 711 growth sources
default to enabled. Other categories remain selectable but disabled.

## Cleaning

Consumed 4,537,749 input fragments and exported 3,645,921 source-scoped unique
segments in 455 batches. Rejected 832,756 duplicates, 58,677 garbage fragments
and 395 advertisements. Both growth/other exports completed. Original SQLite
was read-only and unchanged. OCR/header uncertainty remains flagged.

Artifacts: `var/knowledge/sqlite-import/production-20260928/` locally;
`/opt/heart-company/.deploy/sqlite-public-20260928` on the server. Wire archives
may omit only reconstructed `search_text`; manifests preserve hashes/counts.
The root-private 518 MB pre-import PostgreSQL backup has a verified archive list.
Baseline public/Enneagram/skill counts: 83,072 / 186 / 1,626.

## Status

- [x] Complete cleaning and source registration.
- [x] Deploy LangChain activation gates/COPY importer, Go fallback/catalog routes and admin assets.
- [x] Verify initial imports and idempotent retries; preserve unrelated server changes.
- [x] Deploy/verify nullable ranking cache and server-local resumable runner.
- [x] Verify 455 successful batches, 3,645,921 managed documents and unchanged baseline.
- [x] Verify final enabled/disabled retrieval, service health and disk usage.
- [x] Restore temporary PostgreSQL settings.

Completed on September 29, 2026 (Asia/Shanghai). The server published
`completion-verified.json` after exact count checks and settings restoration.
All 3,645,921 managed documents have a populated search cache; catalog chunk
totals match. The 711 enabled growth sources contain 452,669 documents. Baseline
public/Enneagram/skill counts remain exactly 83,072 / 186 / 1,626.

Final database size: 36 GB. Server filesystem: 197 GB total, 126 GB used,
64 GB available (67% used). Gateway health and knowledge readiness returned
success. Authenticated hybrid retrieval for `创伤后成长` returned 30 documents,
including five imported managed documents, in 5.22 seconds. All returned managed
sources were independently verified enabled. Live enabled/disabled and private
scope checks passed without modifying administrator choices.

## Recovery

Session-local import tuning preserves durable WAL/full-page writes. Temporary
global `max_wal_size=8GB` and `checkpoint_timeout=15min` were restored to
their original `1024MB` and `300s`, with `fsync`/`full_page_writes` still enabled.
Original settings/auto-conf were backed up.
Legacy lexical indexing is restricted to unmanaged rows; managed Chinese
retrieval uses a bigram GIN index on `COALESCE(public_search_vector,
to_tsvector('simple',search_text))` and a nullable ranking cache. The cache was
fully populated before imports resumed. Replacing the old expression index
concurrently also reclaimed its backfill bloat (1.3 GB to 280 MB at 292,773
managed documents). Legacy title/content substring GIN indexes also cover only
unmanaged rows. Earlier 449 ms/3.03 s measurements did not hold under bulk GIN
maintenance: subsequent retrieval timed out, with WAL writes and random TOAST
reads dominating the storage load.

During import of the disabled-category corpus, an independently verified 478 MB
temporary GIN index covers the enabled-source snapshot. Lexical queries fetch
current enabled IDs on each connection and bind an indexed `ANY(text[])` filter,
while retaining the live `EXISTS` gate. No activation choices are rewritten.
The full managed GIN was deferred until finalization. It is now rebuilt and
independently verified ready/valid (3,454 MB); the temporary enabled-source GIN
and `full-index-deferred` marker were removed only after validation. Activation
choices are no longer tied to the temporary source snapshot.

Sequential read-only cache warming of the existing 4.4 GB TOAST relation helped
complete index construction. A production lexical query returned 12 new managed
IDs in 4.86 s; an authenticated real hybrid endpoint returned five new managed
IDs in 9.19 s. These are smoke results, not a latency guarantee. Subsequent bulk
batches measured roughly 8-10 s instead of the earlier 169-470 s spikes.

The temporary `autovacuum_analyze_scale_factor=5.0` table option reduced repeated
wide-row analysis; vacuum itself remained enabled. `backups/document-analyze-before.json`
and `tune_analyze.py` preserve the original explicit option (none). Restoration
skips values changed by other administrators. The heap and TOAST options are now
independently verified restored to their original inherited defaults.

Repeated insert-triggered heap/TOAST vacuum scans later competed with import
I/O despite zero dead tuples. Both `autovacuum_vacuum_insert_scale_factor`
options temporarily used 5.0; update/delete-triggered vacuum and anti-wraparound
protection remained enabled. `tune_insert_vacuum.py` backs up both original explicit
options and restores only unchanged temporary values. Fixture verification
covered inherited defaults, explicit 0.4/0.6 settings and a concurrent external
0.9 change. Finalization performed one serial `VACUUM (ANALYZE, PARALLEL 0)` before restoration.

The server-local `nine-xing-public-import-20260928.service` reads an immutable
455-batch manifest, verifies archive/ledger hashes, preserves committed batches,
and writes results under `results/` plus `server-sync.log`. It waits for verified
archives while the corpus is uploaded, so SSH loss cannot cancel an active DB
transaction. Final document and batch counts are asserted before writing
`import-complete.json`. All 455 batches are committed and completion is verified.

The persistent unit in `/etc/systemd/system/` is enabled for reboot recovery,
waits for both services, and skips runs once `completion-verified.json` exists.
The current transient unit continues unchanged; its finalization drop-in resets
`ExecStopPost` to prevent duplicate hooks. `TimeoutStopSec=12h` permits the final
concurrent index build. `finalize_import.sh` restores the general managed index
on success or interruption, removes the temporary index only after validation,
cleans GIN pending entries, vacuums/analyzes documents, checks service health and asserts
exact document/batch/catalog counts plus preservation of legacy baseline counts.
An EXIT trap restores temporary table/global settings on error. The final
`completion-verified.json` is published only after settings restoration succeeds.

The first post-import cleanup failed because parallel VACUUM requested a
512 MB shared-memory segment while the Docker container's `/dev/shm` is 64 MB.
All data had already committed; it was not reimported. The independent
`nine-xing-public-import-finalize-20260928.service` recovery unit used
`max_parallel_maintenance_workers=0` and `VACUUM (ANALYZE, PARALLEL 0)`, completed
successfully, reverified exact counts, and restored settings before publishing
the completion marker.

## Final Retrieval Query

After full-corpus analysis, the redundant nullable-source branch in the managed
query gate prevented an efficient semi-join. PostgreSQL selected a source B-tree
scan and the first authenticated smoke request timed out after 120 seconds.
Replacing only the managed branch with direct live `EXISTS` retained activation
semantics and selected the full managed GIN with a hash join. Legacy/vector
gates remain unchanged.

Lexical connections also use `SET LOCAL work_mem='64MB'` to support exact bitmap
scans without changing global configuration. A broad-query candidate-count
probe timed out after 15 seconds at 4 MB, while the 64 MB probe completed in
1.57 seconds with zero lossy heap blocks. Ranking probes for that broad query
still exceeded 15 seconds on cold data; these results are not a latency SLA.
The narrow lexical smoke completed in 0.73 seconds; final authenticated hybrid
retrieval completed in 5.22 seconds. Both query changes have regression tests,
and the full Python suite against isolated PostgreSQL passed: 72 tests.

Only `app/repositories/documents.py` changed relative to the previously deployed
knowledge-service image; server/local SHA-256 matches. The versioned image is
`heart-company-knowledge-service:sqlite-public-final-20260929`, with both the
existing import tag and `latest` updated. The previous image is retained as
`heart-company-knowledge-service:sqlite-public-before-final-20260929` for code-only
rollback. No other services were recreated. Use `PYTHONPATH=/app` when running
`verify_final_retrieval.py` from `/tmp` to verify the same source as the service.
The generated `final-retrieval-verified.json` contains results, not credentials.

Authenticated production source catalog returned 4,756 entries. Python suite
with isolated PostgreSQL: 70 passed, including per-query activation refresh and
enabled/disabled toggle integration. Go publicknowledge/db suites and a server
suite rerun passed on Go 1.22.12; the first server suite run failed transiently,
so it is not evidence that every broad regression is stable.

Re-import uses immutable archive/batch identities and skips committed rows.
Post-commit maintenance failure in batch 12 was retried without duplication.
Rollback only newly created documents with `python -m app.ingestion.sqlite_public
rollback --batch-id BATCH_ID` inside the knowledge-service container. Do not
restore the full backup over subsequent live application activity. Before
reverting to code without activation gates, roll back managed batches or retain
the live-gated repository so disabled sources remain excluded.

No bulk embedding API calls; semantic backfill is a separate cost-bearing step.
New imported text is available through lexical retrieval alongside existing
semantic vectors, not through newly generated document embeddings.
