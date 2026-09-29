# SQLite Public Knowledge Cleanup

Completed on September 29, 2026 after explicit approval to remove unusable
sources from the public catalog.

## Removed

- 69 `pending` sources with no exportable body fragments.
- 7 `needs_review` book sources with zero imported chunks and no usable body.
- 9 clearly corrupted story sources: eight high-ratio binary/encoding
  failures and one reversed OCR-only PDF. Their 59 imported fragments were
  removed in the same transaction.

## Verification

- Catalog: 4,671 sources.
- Quality status: 4,666 `ready`, 5 `needs_review`, 0 `pending`.
- Managed public documents: 3,645,862.
- Deleted source/document residuals: 0.
- Knowledge service and PostgreSQL containers: healthy.

The remaining five review sources are mostly readable bodies with isolated OCR
markers and remain excluded from retrieval. The targeted rollback archive is
stored on the server at `/opt/heart-company/.deploy/cleanup-targets.dump`.
