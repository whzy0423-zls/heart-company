# Video Closure Integration Design

## Goal

Integrate the verified guided and zero-charge video workflow into the current
`detail-tuning-video-management` line without losing the uncommitted chat
context, memory, database-test safety, or integration-test fixes already in the
workspace.

## Integration Strategy

Preserve the current work as reviewable commits before merging video work:

1. Commit conversation summary, stable-memory, and related schema changes as
   one coherent feature change.
2. Commit isolated PostgreSQL test validation and integration-test reliability
   fixes separately.
3. Audit every entry in `git status` and assign it to one of the feature,
   test-safety, or dependency/integration commits. This includes new chat tests,
   `internal/testutil`, and workspace catalog changes. No existing change may
   disappear merely because it does not fit the first two groups.
4. Exclude local `artifacts/` and generated output from every commit.
5. Create a new isolated git worktree and integration branch from the resulting
   current branch.
6. Merge `feature/guided-video-workbench` without rewriting its verified
   history.

This retains attribution and makes schema or server conflicts reviewable. A
single snapshot commit would obscure ownership, while rebasing the video branch
would rewrite an already verified implementation.

## Conflict Policy

Conflicts are resolved by preserving both feature sets. In particular:

- `schema.sql` must contain both conversation-summary columns and the complete
  video workflow schema in dependency-safe order.
- server construction must initialize both chat summarization and guided video
  workflow dependencies.
- video routes and configuration follow the verified video branch unless the
  current branch contains a newer compatible safety fix.
- tests are never deleted merely to resolve a conflict; assertions are updated
  only when the combined behavior requires it.

Each resolved conflict is checked with focused tests before the full suite.

## E2E Gate

The guided Chromium suite becomes part of the normal workspace E2E command.
The root `pnpm run test:e2e` must invoke a real workspace task and fail when no
tests are collected. The dedicated Playwright configuration remains the source
of browser fixtures and server lifecycle behavior.

All browser artifacts must use paths derived from the active worktree or
Playwright configuration. Tests must not contain an absolute path to another
checkout. `test-results`, traces, screenshots, and temporary downloads must be
ignored or removed by the test lifecycle before the final cleanliness check.

The gate must cover the five-step guided workflow, recovery states, responsive
layouts, explicit version selection, composition retry, and the zero-charge
safety signal that rejects paid-provider requests.

## Verification

The integrated branch is accepted only when all of the following pass from a
clean test run:

- Go tests with a uniquely named PostgreSQL database ending in `_test`.
- `go vet ./...` and Go formatting checks.
- The real HTTP zero-charge workflow with FFmpeg/ffprobe and
  `provider_calls=0`.
- Frontend Vitest, Vue typecheck, and production build.
- Root `pnpm run test:e2e` with a non-zero collected test count.
- A repository search confirming the E2E suite has no hard-coded path to the
  desktop checkout or another worktree.
- Website, reading H5, and miniapp tests/builds.
- `git diff --check`, no generated Playwright output in `git status`, and a
  clean integration worktree.

Existing repository-wide lint, package-publication, dependency-cycle, and
large-asset debt is reported separately unless it directly blocks the combined
video workflow. Mass formatting is out of scope because it would obscure the
functional integration.

## Delivery

No changes are pushed or merged into `main`. The result is delivered as an
integration branch with reviewable commits and exact verification evidence.
