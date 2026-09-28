# Full-Stack Public Knowledge Release Plan

> **For agentic workers:** REQUIRED: Use superpowers:subagent-driven-development (if subagents available) or superpowers:executing-plans to implement this plan. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Verify all current code, integrate both repositories into `main`, deploy backend/admin/knowledge services, and publish a newly signed Android APK through the official website.

**Architecture:** The Flutter repository and full-stack repository are independently tested release units. Preserve production operational changes and the imported public corpus; deploy immutable artifacts with image/source/database rollback coverage. APK publication occurs only after online service verification.

**Tech Stack:** Flutter 3.41.7, Go 1.22.12, Python/PostgreSQL, Vue/Vitest, React/Vite, Docker Compose, GitHub protected production signing.

## Task 1: Inventory And Quality Gates

- [x] Fetch both remotes, inspect `main` and dirty working trees.
- [x] Verify all App feature branches are merged; compare historical backend branches with `git cherry` before considering any merge.
- [x] Run knowledge-service suite against isolated PostgreSQL: 72 passed.
- [x] Run Go full suite, isolated PostgreSQL suite serially, race, and vet. Correct actual failures without weakening production invariants.
- [x] Run full admin/canvas unit suites, typechecks, and production build; website, motion website, reading-H5, and miniapp native tests/builds.
- [x] Run App format check, analyze, full tests including golden, and CI non-golden suite. Refresh only visual baselines proven stale by intended committed changes.
- [x] Compare production source drift to tested local `main` and preserve any distinct operational fix.

**Commands:** `GOTOOLCHAIN=go1.22.12 go test -p 1 ./...`, `go test -race ./...`, `go vet ./...`; Python `.venv/bin/python -m pytest tests` with `TEST_DATABASE_URL` pointing only to an isolated test database; native pnpm/npm scripts; `dart format --output=none --set-exit-if-changed .`, `flutter analyze`, `flutter test`, `flutter test --exclude-tags=golden`.

## Task 2: Integrate And Prepare Signed Build

- [x] Review diffs, test fixes and release plan; scan staged files for credentials/generated data.
- [x] Create pre-release rollback refs for both repositories.
- [x] Set a new App version above the live `1.1.25+187` and any current device/test builds; update its version contract test first.
- [x] Restore the protected self-hosted manual signing workflow with `main`/SHA validation, all quality gates, production Dart defines, manifest/signature checks, draft artifact, and unconditional signing-file cleanup.
- [ ] Re-run gates after version/workflow edits, then commit verified changes to `main` and push without rewriting history. Do not add SQLite exports, operational secrets, APKs, caches, or output screenshots. Record the exact final `main` SHA that includes both version and signing workflow.
- [x] Register a temporary local GitHub runner using the registration API without logging tokens. Execute only the trusted `main` revision.

## Task 3: Deploy Services

- [x] Record image IDs, mounts, safe configuration fingerprints, live App release metadata, and source diff.
- [x] Back up production application tables, live schema/catalog/activation/ledger, source overlays, and current images. For knowledge正文, retain the verified pre-import legacy archive plus all 455 hashed wire archives/manifest; check current legacy counts and incremental changes before excluding managed rows from a new dump. Document selective restore and immutable replay, not a whole-database overwrite of live activity. Validate archives.
- [ ] Build tested server/admin/website artifacts and knowledge-service image; preserve runtime utilities, environment, volumes, certificates, website verification files, and unrelated containers.
- [ ] Recreate only affected services with existing production configuration plus versioned image overrides. Do not rebuild against a dirty untested checkout.
- [ ] Verify migration completion, gateway health, knowledge readiness, admin authentication/catalog, public routes, existing follow-up contracts, and a real retrieval with new managed documents.
- [ ] Require 4,756 registered sources, 30 categories, 455 committed batches, 3,645,921 catalog document total, and a valid managed GIN. Retain 711 currently enabled growth sources unless administrators changed them; do not reset choices to satisfy a count assertion.

## Task 4: Publish And Verify APK

- [ ] After online service verification, build the new APK using protected production signing secrets. Verify package/version/certificate, manifest permissions, production origin, size, and SHA-256.
- [ ] Install and launch on the available Android device without clearing user data; verify a non-destructive basic navigation smoke.
- [ ] Upload a new unpublished release via authenticated App release API; compare server-inspected metadata and file hash before publication.
- [ ] Publish with the current non-forced update policy; preserve the previous release for rollback.
- [ ] Verify public latest API, full downloaded APK SHA-256, byte-range response, website download route, and old release availability.
- [ ] Record exact commits/images/APK metadata/tests/rollback details in the deployment report. Close temporary SSH/runner sessions and remove runner registration/signing files.

**Release Gates:** No publication on failing required tests. Do not substitute stale artifacts or identify skipped infrastructure tests as passed. Hosting/billing failures must be distinguished from executed test failures.

## Verified Progress

App final full suite: 2,553 passed including 15 goldens; CI non-golden: 2,538
passed. Analysis has zero issues; format checked 637 files without changes.
App `main`: `6c09646e08240cbe339953e497dad6c1e23ec998`, version `1.1.26+192`.

Frontend: admin/workspace 804 tests, canvas 95, website 80, motion website 65,
reading 8, optimized-assets 1 and 31 miniapp script groups passed. Production
builds and admin/canvas/workspace typechecks passed. Reading was additionally
built with the deployed `/read/` base. Admin E2E configuration refers to a
missing directory; browser smoke is required separately.

Backup archives were validated, current legacy counts remain public 83,072 /
Enneagram 186 / skill 1,626. Source choices before deployment: 4,756 sources,
30 primary categories, 3,645,921 catalog chunks, 711 enabled sources; activation
fingerprint `e2f64d6b49d2c58e9d83fb545ec5d072`. Current release 37 remains
published and non-forced. Selective code/table restoration and immutable
managed-batch replay preserve later live activity.

GitHub hosted quality run `36482917596` did not execute any steps because of
the existing account spending/payment restriction. The temporary ephemeral
macOS ARM64 runner uses the checksum-verified official runner archive and the
production protected signing environment; no signing secret is committed.

Final Go run enables all local PostgreSQL, life-story, workflow migration and
real skill-source integrations under race detection: 5,485 passed, zero failed,
one live-provider performance baseline skipped; 74 packages passed. Vet,
native build and Linux amd64 static cross-build passed. New retire/fallback
regressions failed before the compiler fix and passed afterward, without
relaxing immutable-release triggers. Independent final code review found no
release blocker.

All 455 server-local wire archive hashes match the immutable manifest SHA-256
`55830d54028a4a066cd32443254e78a0516c907322fed73afe56cdff1edd4285`.
Production follow-up/voice/chat overlays match tested local source; remaining
differences are newer local regression tests and the already-tested final
knowledge retrieval optimization.
