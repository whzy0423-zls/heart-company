# Enterprise Training Promotion Miniapp Implementation Plan

> **For agentic workers:** REQUIRED: Use superpowers:subagent-driven-development (if subagents available) or superpowers:executing-plans to implement this plan. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Reposition the miniapp as Han Teacher's enterprise-training promotion funnel with real training cases, customizable training solutions, controlled promotional video playback, and attributable enterprise consultation leads.

**Architecture:** Add an isolated `enterprisepromotion` backend domain for cases, solutions, trainers, consent, leads, attribution, and homepage snapshots. Extract only neutral multipart/media concepts from the unmerged classroom branch into a promotion-owned media boundary; do not merge classroom payments, entitlements, membership, or progress. Replace the miniapp's four primary tabs while preserving old deep links as secondary compatibility routes.

**Tech Stack:** Go 1.22, PostgreSQL, existing `net/http` server, Aliyun OSS signed/multipart uploads, Vue 3 + Vben Admin + TypeScript, uni-app Vue 3 miniapp, Node `.mjs` contract tests, Python/FFmpeg media audit scripts.

**Design spec:** `docs/superpowers/specs/2026-07-27-enterprise-training-promotion-design.md`

---

## File Structure

### Backend domain

- Create `nx-backend/apps/server/internal/enterprisepromotion/models.go` — domain records, enums, validation, safe public projections.
- Create `nx-backend/apps/server/internal/enterprisepromotion/store.go` — cases, trainers, solutions, topics, homepage settings, ordered associations.
- Create `nx-backend/apps/server/internal/enterprisepromotion/publication.go` — consent links, claims/testimonial review, publication gate.
- Create `nx-backend/apps/server/internal/enterprisepromotion/consultations.go` — encrypted consultation persistence, notes, idempotency, privacy requests.
- Create `nx-backend/apps/server/internal/enterprisepromotion/analytics.go` — promotion sessions, events, share tokens, funnel queries.
- Create `nx-backend/apps/server/internal/promotionmedia/models.go` — neutral media assets, attempts, upload tasks and ready/quarantine state machine.
- Create `nx-backend/apps/server/internal/promotionmedia/service.go` — multipart lifecycle, probing, processing attempts and signed playback.
- Create `nx-backend/apps/server/internal/promotionmedia/store.go` — media assets, upload tasks, processing attempts, leases and lineage persistence.
- Create `nx-backend/apps/server/internal/promotionmedia/ffmpeg.go` — ffprobe/full-decode command boundary and structured failure codes.
- Create `nx-backend/apps/server/internal/promotionmedia/processor.go` — normalized MP4/H.264/AAC transcode and validation pipeline.
- Create `nx-backend/apps/server/internal/promotionmedia/worker.go` — leased background runner for probing, transcoding and validation attempts.

### Backend HTTP

- Create `nx-backend/apps/server/internal/server/enterprise_promotion_public.go` — public home/cases/solutions/trainers/consultations/events/play tickets.
- Create `nx-backend/apps/server/internal/server/enterprise_promotion_admin.go` — admin CRUD, review, publish, offline, preview and analytics.
- Create `nx-backend/apps/server/internal/server/enterprise_promotion_media.go` — admin multipart uploads, processing attempts and consent links.
- Create `nx-backend/apps/server/internal/server/enterprise_promotion_privacy.go` — SMS verification and privacy-request workflow.
- Modify `nx-backend/apps/server/internal/server/server.go` — dependencies and route registration.
- Modify `nx-backend/apps/server/internal/db/schema.sql` — enterprise promotion schema.
- Modify `nx-backend/apps/server/internal/db/db.go` — permission/menu seeds.
- Modify `nx-backend/apps/server/internal/config/env.go` — promotion media and privacy-request configuration.

### Admin

- Create `nx-backend/apps/web-antd/src/api/core/enterprise-promotion.ts` — typed API contract.
- Modify `nx-backend/apps/web-antd/src/api/core/index.ts` — export API.
- Create `nx-backend/apps/web-antd/src/router/routes/modules/enterprise-promotion.ts` — admin routes.
- Create `nx-backend/apps/web-antd/src/views/enterprise-promotion/` — dashboard/settings, cases, solutions, trainers, media, consents, leads and analytics.
- Port neutral upload helpers from committed `feature/miniapp-teacher-classroom` files into `enterprise-promotion/media/`; do not import classroom order/entitlement code.

### Miniapp

- Create `miniapp/src/api/enterprisePromotion.js` — public promotion API methods.
- Create `miniapp/src/utils/enterprisePromotion.js` — safe normalization, CTA context and event helpers.
- Create `miniapp/src/utils/enterpriseConsultation.js` — form draft, idempotency and consent helpers.
- Modify `miniapp/src/pages.json` — four new tabs plus enterprise subpackage routes; retain old pages.
- Rewrite `miniapp/src/pages/index/index.vue` — enterprise promotion home.
- Create `miniapp/src/pages/training-cases/index.vue` — case list tab.
- Create `miniapp/src/pages/enterprise-solutions/index.vue` — solution list tab.
- Create `miniapp/src/pages/enterprise-consultation/index.vue` — consultation overview tab.
- Create `miniapp/src/pages-enterprise/` — case detail, solution detail, trainer detail, consultation form and privacy notice.

### Media tooling/docs

- Create `scripts/enterprise-media/build_manifest.py` — source IDs, relative paths, size, SHA-256 and probe results.
- Create `scripts/enterprise-media/validate_media.py` — ffprobe/full-decode validation and JSON report.
- Create `docs/deployment/enterprise-promotion-media.md` — OSS/CORS/FFmpeg/consent/release procedure.

---

## Chunk 1: Domain, Database, Media and Publication Safety

### Task 1: Add enterprise-promotion schema and domain models

**Files:**
- Create: `nx-backend/apps/server/internal/enterprisepromotion/models.go`
- Create: `nx-backend/apps/server/internal/enterprisepromotion/models_test.go`
- Modify: `nx-backend/apps/server/internal/db/schema.sql`
- Create: `nx-backend/apps/server/internal/db/schema_enterprise_promotion_test.go`
- Create: `nx-backend/apps/server/internal/db/schema_enterprise_promotion_postgres_test.go`

- [ ] **Step 1: Write failing schema tests**

Assert the schema contains the complete table list for trainers, cases, solutions, topics, ordered joins, testimonials, claims, settings, consultations, notes, consent records/links, promotion sessions/events/share tokens, privacy requests, media assets, multipart upload tasks and processing attempts. Assert named snippets for trainer FKs, `training_case_media.id`, unique slugs/keys, stable sort keys, versions, consultation reference hashes, event idempotency, media lineage/attempt constraints and persisted QA result/reviewer/time/note fields required by the ready gate.

```go
func TestSchemaContainsEnterprisePromotionTables(t *testing.T) {
    schema := readSchema(t)
    for _, table := range []string{
        "enterprise_trainers", "training_cases", "enterprise_solutions",
        "training_case_media", "publication_consents",
        "promotion_media_assets", "promotion_media_upload_tasks",
        "promotion_media_processing_attempts",
        "enterprise_consultations", "promotion_events",
    } {
        if !strings.Contains(schema, "CREATE TABLE IF NOT EXISTS "+table) {
            t.Fatalf("missing %s", table)
        }
    }
}
```

- [ ] **Step 2: Run RED**

Run: `cd nx-backend/apps/server && go test ./internal/db ./internal/enterprisepromotion`

Expected: FAIL because package/tables do not exist.

- [ ] **Step 3: Add a failing real-PostgreSQL migration test**

Using `TEST_DATABASE_URL`, require a loopback host and an isolated test database name. Add named tests `TestEnterprisePromotionPostgres` and `TestEnterprisePromotionRollbackPostgres`: apply `schema.sql` twice in a temporary schema, exercise FK/UNIQUE/CHECK/RESTRICT behavior, then rehearse the documented non-destructive rollback/forward-recovery sequence and verify retained consultation/audit rows. Tests must clean up temporary schemas and skip only when `TEST_DATABASE_URL` is absent; they must reject non-loopback or non-test database targets.

- [ ] **Step 4: Run the PostgreSQL migration test RED**

Run: `cd nx-backend/apps/server && TEST_DATABASE_URL="$TEST_DATABASE_URL" go test ./internal/db -run 'TestEnterprisePromotion(Postgres|RollbackPostgres)$' -v`

Expected: FAIL against a configured isolated loopback test database because the new schema objects and constraints do not exist. A skipped test does not satisfy this RED step.

- [ ] **Step 5: Implement schema constraints**

Include stable unique slugs/keys, `training_case_media.id`, FK/RESTRICT behavior, stable sort keys, `version`, trainer FKs on cases and solutions, fixed topic keys, consultation reference hash, encrypted PII columns and event idempotency uniqueness. Add `promotion_media_assets`, `promotion_media_upload_tasks` and `promotion_media_processing_attempts` with source/derived lineage, server-generated object keys, SHA-256, probe/derived metadata, state, `qa_result`, `qa_approved_by`, `qa_approved_at`, `qa_note`, attempt number, lease owner/expiry, structured error code/log excerpt, retry timestamps and FK/RESTRICT rules. Enforce that ready state is written only in the same transaction as a passing QA record.

- [ ] **Step 6: Implement domain enums and validation**

```go
type CaseStatus string
const (
    CaseDraft CaseStatus = "draft"
    CaseReview CaseStatus = "review"
    CasePublished CaseStatus = "published"
    CaseOffline CaseStatus = "offline"
)

func (c TrainingCase) ValidateForReview() error {
    if strings.TrimSpace(c.Title) == "" || strings.TrimSpace(c.Slug) == "" {
        return ErrInvalidCase
    }
    if c.TrainerID <= 0 { return ErrTrainerRequired }
    return nil
}
```

- [ ] **Step 7: Run GREEN**

Run: `cd nx-backend/apps/server && go test ./internal/db ./internal/enterprisepromotion`

Expected: PASS. With a configured safe `TEST_DATABASE_URL`, the migration test must execute rather than skip.

- [ ] **Step 8: Commit**

```bash
git add nx-backend/apps/server/internal/enterprisepromotion nx-backend/apps/server/internal/db/schema.sql nx-backend/apps/server/internal/db/schema_enterprise_promotion_test.go nx-backend/apps/server/internal/db/schema_enterprise_promotion_postgres_test.go
git commit -m "feat: add enterprise promotion domain schema"
```

### Task 2: Implement stores, public projections and optimistic concurrency

**Files:**
- Create: `nx-backend/apps/server/internal/enterprisepromotion/store.go`
- Create: `nx-backend/apps/server/internal/enterprisepromotion/store_test.go`
- Create: `nx-backend/apps/server/internal/enterprisepromotion/store_postgres_test.go`

- [ ] **Step 1: RED/GREEN trainer and fixed-topic persistence**

Write focused tests for trainer CRUD, fixed topic keys, stable ordering and RESTRICT deletion. Run `cd nx-backend/apps/server && go test ./internal/enterprisepromotion -run 'Trainer|Topic'` and confirm RED; implement the narrow methods and rerun to GREEN.

- [ ] **Step 2: RED/GREEN case aggregate persistence**

Write tests for case create/update, ordered media/topics, transaction rollback and version conflicts. Run `cd nx-backend/apps/server && go test ./internal/enterprisepromotion -run 'CaseStore|CaseMedia|CaseTopic'` and confirm RED; implement minimal methods and rerun to GREEN.

- [ ] **Step 3: RED/GREEN solution aggregate persistence**

Write tests for solution CRUD, trainer FK, ordered `training_case_solutions` case relations, stable ordering and version conflicts. Run `cd nx-backend/apps/server && go test ./internal/enterprisepromotion -run 'SolutionStore'` and confirm RED; implement minimal methods and rerun to GREEN.

- [ ] **Step 4: Define and implement narrow repository interfaces**

```go
type Store interface {
    ListPublicCases(ctx context.Context, q PublicCaseQuery) ([]PublicCaseCard, error)
    GetPublicCaseBySlug(ctx context.Context, slug string) (PublicCaseDetail, error)
    UpdateCase(ctx context.Context, in UpdateCaseInput) (TrainingCase, error)
    ReplaceCaseMedia(ctx context.Context, caseID int64, version int64, items []CaseMediaInput) error
}
```

Use transactions for aggregate updates. Return `ErrVersionConflict`, not generic DB errors.

- [ ] **Step 5: RED/GREEN public projections**

Add reflection/JSON tests proving published-only projections preserve stable ordering/topic filtering and omit internal company name, encrypted PII, consent evidence and source asset keys. Run `cd nx-backend/apps/server && go test ./internal/enterprisepromotion -run PublicProjection` RED, implement projections, then rerun GREEN.

- [ ] **Step 6: RED/GREEN PostgreSQL concurrency and constraints**

Using only safe `TEST_DATABASE_URL`, test concurrent updates, optimistic conflict mapping, aggregate rollback and RESTRICT behavior against PostgreSQL. Run `cd nx-backend/apps/server && TEST_DATABASE_URL="$TEST_DATABASE_URL" go test ./internal/enterprisepromotion -run Postgres -v` and confirm RED before implementation, then GREEN. Reject unsafe database URLs; do not use `DATABASE_URL`.

- [ ] **Step 7: Run the complete package suite**

Run: `cd nx-backend/apps/server && go test ./internal/enterprisepromotion`

Expected: PASS. PostgreSQL tests may skip only when `TEST_DATABASE_URL` is absent; Chunk 1 acceptance requires one explicit run with a configured isolated test database.

- [ ] **Step 8: Commit**

```bash
git add nx-backend/apps/server/internal/enterprisepromotion
git commit -m "feat: persist enterprise promotion content"
```

### Task 3: Extract neutral promotion media boundary

**Files:**
- Create: `nx-backend/apps/server/internal/promotionmedia/models.go`
- Create: `nx-backend/apps/server/internal/promotionmedia/service.go`
- Create: `nx-backend/apps/server/internal/promotionmedia/store.go`
- Create: `nx-backend/apps/server/internal/promotionmedia/ffmpeg.go`
- Create: `nx-backend/apps/server/internal/promotionmedia/processor.go`
- Create: `nx-backend/apps/server/internal/promotionmedia/worker.go`
- Create: `nx-backend/apps/server/internal/promotionmedia/service_test.go`
- Create: `nx-backend/apps/server/internal/promotionmedia/store_test.go`
- Create: `nx-backend/apps/server/internal/promotionmedia/store_postgres_test.go`
- Create: `nx-backend/apps/server/internal/promotionmedia/ffmpeg_test.go`
- Create: `nx-backend/apps/server/internal/promotionmedia/processor_test.go`
- Create: `nx-backend/apps/server/internal/promotionmedia/worker_test.go`
- Modify: `nx-backend/apps/server/internal/storage/storage.go`
- Modify: `nx-backend/apps/server/internal/config/env.go`
- Modify: `nx-backend/apps/server/internal/config/env_test.go`

- [ ] **Step 1: Use classroom code only as a read-only reference**

Use the already pinned classroom reference commit and review committed files with:

```bash
CLASSROOM_REF=8d41254b6cc08aa4b1381412acd18f49f9cbec01
git show "$CLASSROOM_REF":nx-backend/apps/server/internal/classroom/upload.go
```

Do not merge the classroom branch and do not copy payment, entitlement, membership or progress types.

- [ ] **Step 2: RED/GREEN state machine and persistent attempts**

Write tests for valid transitions, quarantine as a branch, source/derived lineage, new immutable processing attempts, attempt numbers, task ownership and cleanup leases. Run `cd nx-backend/apps/server && go test ./internal/promotionmedia -run 'State|Store|Attempt|Lease'` and confirm RED; implement models/store and rerun GREEN.

- [ ] **Step 3: RED/GREEN multipart and storage boundary**

Write tests for initiate/sign-part/complete/abort idempotency, checksum mismatch, server-controlled object keys, size/part limits and abandoned-upload cleanup. Run `cd nx-backend/apps/server && go test ./internal/promotionmedia ./internal/storage -run 'Multipart|Upload|ObjectKey'` and confirm RED; implement the minimal service/storage changes and rerun GREEN.

- [ ] **Step 4: Implement promotion-owned media state transitions**

```go
func (s AssetStatus) CanTransition(next AssetStatus) bool {
    allowed := map[AssetStatus]map[AssetStatus]bool{
        StatusProbing: {StatusTranscoding: true, StatusQuarantined: true, StatusFailed: true},
        StatusTranscoding: {StatusValidating: true, StatusQuarantined: true, StatusFailed: true},
        StatusValidating: {StatusQAPending: true, StatusQuarantined: true, StatusFailed: true},
        StatusQAPending: {StatusReady: true, StatusQuarantined: true},
        StatusQuarantined: {StatusRejected: true},
    }
    return allowed[s][next]
}
```

Reprocess creates a new attempt starting at probing; it does not mutate quarantine directly to ready.

- [ ] **Step 5: RED/GREEN FFmpeg transcode and validation pipeline**

Write command-construction and fixture-runner tests, run `cd nx-backend/apps/server && go test ./internal/promotionmedia -run 'FFmpeg|Processor|Validation'` and confirm RED. Implement cancellable commands with timeouts: source `ffprobe`; normalized MP4/H.264/AAC transcode; loudness normalization; derived `ffprobe`; full null-output decode; timestamp, duration and A/V-sync tolerance checks; SHA-256 and derived metadata persistence; HTTP Range/206 validation. Parse errors into `NAL_CORRUPT`, `AAC_CORRUPT`, `NO_AUDIO`, `UNSUPPORTED_CODEC`, `TIMESTAMP_INVALID`, `AV_SYNC_INVALID`, `RANGE_INVALID` and `DECODE_FAILED`, then rerun GREEN.

- [ ] **Step 6: RED/GREEN leased processing worker and ready gate**

Test lease acquisition/renewal, crash recovery, retry limits, attempt history, and the complete `probing → transcoding → validating → qa_pending → ready` flow. A derived asset becomes `ready` only after automated checks pass and an explicit QA approval record exists; failures become quarantined/failed with structured diagnostics. Run `cd nx-backend/apps/server && go test ./internal/promotionmedia -run 'Worker|ReadyGate|CrashRecovery'` and confirm RED, implement the runner, then rerun GREEN.

- [ ] **Step 7: RED/GREEN PostgreSQL media persistence and leases**

Using only safe `TEST_DATABASE_URL`, test lineage FK/RESTRICT, attempt uniqueness, concurrent lease acquisition/renewal, expired-lease crash recovery and atomic QA-to-ready transition. Run `cd nx-backend/apps/server && TEST_DATABASE_URL="$TEST_DATABASE_URL" go test ./internal/promotionmedia -run Postgres -v` RED and then GREEN. Chunk 1 acceptance requires this test to execute against an isolated database, not skip.

- [ ] **Step 8: Run complete media tests**

Run: `cd nx-backend/apps/server && go test ./internal/promotionmedia ./internal/config ./internal/storage`

Expected: PASS.

- [ ] **Step 9: Commit**

```bash
git add nx-backend/apps/server/internal/promotionmedia nx-backend/apps/server/internal/storage nx-backend/apps/server/internal/config
git commit -m "feat: add promotion media processing boundary"
```

### Task 4: Add consent, claim review and publication gate

**Files:**
- Create: `nx-backend/apps/server/internal/enterprisepromotion/publication.go`
- Create: `nx-backend/apps/server/internal/enterprisepromotion/publication_test.go`
- Create: `nx-backend/apps/server/internal/enterprisepromotion/publication_store.go`
- Create: `nx-backend/apps/server/internal/enterprisepromotion/publication_store_test.go`
- Create: `nx-backend/apps/server/internal/enterprisepromotion/publication_store_postgres_test.go`

- [ ] **Step 1: Write failing gate matrix**

Cover company, person, logo, voice, screen/document, media and testimonial consent; expired/revoked consent; unreviewed claims; missing cover; missing promo/highlight video; media not ready.

- [ ] **Step 2: Run RED**

Run: `cd nx-backend/apps/server && go test ./internal/enterprisepromotion -run Publication`

Expected: FAIL.

- [ ] **Step 3: Implement structured gate failures**

```go
type GateFailure struct { Code, Subject string }

func EvaluatePublication(a Aggregate) []GateFailure {
    // deterministic ordered failures for admin UI and tests
}
```

- [ ] **Step 4: Write failing consent persistence and revocation transaction tests**

Cover consent-link CRUD, source consent inherited by derived assets, concurrent revoke/publish attempts, querying every affected case, transactional non-publication, and calls to cache invalidation and playback-ticket authorization fakes. Run `cd nx-backend/apps/server && go test ./internal/enterprisepromotion -run 'ConsentStore|Revocation'` and confirm the unit/fake tests are RED.

- [ ] **Step 5: Run the PostgreSQL revocation tests RED**

Run: `cd nx-backend/apps/server && TEST_DATABASE_URL="$TEST_DATABASE_URL" go test ./internal/enterprisepromotion -run 'ConsentStorePostgres|RevocationPostgres' -v`

Expected: FAIL while connected to an isolated loopback test database because persistence/transactions are not implemented. A skipped test does not satisfy this RED step.

- [ ] **Step 6: Implement consent revocation effect**

Implement the repository transaction and narrow `CacheInvalidator` / `PlaybackAuthorizer` interfaces. Revocation must mark every affected case non-public, clear public cache tags and prevent new playback tickets, including derived assets linked to a revoked source consent.

- [ ] **Step 7: Run GREEN and commit**

```bash
(cd nx-backend/apps/server && go test ./internal/enterprisepromotion)
(cd nx-backend/apps/server && TEST_DATABASE_URL="$TEST_DATABASE_URL" go test ./internal/enterprisepromotion -run 'ConsentStorePostgres|RevocationPostgres' -v)
git add nx-backend/apps/server/internal/enterprisepromotion
git commit -m "feat: enforce enterprise case publication consent"
```

Expected: both commands PASS, and the PostgreSQL command executes against an isolated loopback test database rather than skipping.

---

## Chunk 2: HTTP Contracts, Leads, Attribution and Admin

### Task 5: Register permissions, dependencies and public read APIs

**Files:**
- Create: `nx-backend/apps/server/internal/server/enterprise_promotion_public.go`
- Create: `nx-backend/apps/server/internal/server/enterprise_promotion_public_test.go`
- Modify: `nx-backend/apps/server/internal/server/server.go`
- Modify: `nx-backend/apps/server/internal/db/db.go`
- Modify: `nx-backend/apps/server/internal/db/menu_test.go`

- [ ] **Step 1: Write failing route, permission and public-contract tests**

Require `EnterprisePromotion:List|Write|Media|Publish|Consent|Leads|Analytics|Export` and the public home/case/solution/trainer endpoints. Cover methods, bounded pagination, fixed topic keys, published/authorized filtering, generic unavailable responses for offline content, cache/ETag behavior, and reflection/JSON assertions that internal company names, consent evidence, storage keys, drafts and PII never appear. Add a cross-layer cache test proving consent revoke, publish/offline and ordered-association changes invalidate only the affected tagged home/list/detail snapshots so previously cached content is not served.

- [ ] **Step 2: Run RED**

Run: `cd nx-backend/apps/server && go test ./internal/db ./internal/server -run 'EnterprisePromotion|PromotionPublic'`

- [ ] **Step 3: Implement safe public handlers**

Handlers accept bounded pagination and fixed topic keys, return published/authorized DTOs, set cache/ETag headers and map offline/unauthorized cases to a generic unavailable response. Implement the concrete tagged cache and inject its `CacheInvalidator` into the publication/revocation service introduced in Chunk 1.

- [ ] **Step 4: Run GREEN and commit**

```bash
(cd nx-backend/apps/server && go test ./internal/db ./internal/server -run 'EnterprisePromotion|PromotionPublic')
git add nx-backend/apps/server/internal/db nx-backend/apps/server/internal/server
git commit -m "feat: expose enterprise promotion public content"
```

### Task 6: Add media upload, processing and case-media playback APIs

**Files:**
- Create: `nx-backend/apps/server/internal/server/enterprise_promotion_media.go`
- Create: `nx-backend/apps/server/internal/server/enterprise_promotion_media_test.go`
- Modify: `nx-backend/apps/server/internal/server/server.go`

- [ ] **Step 1: Write failing multipart and playback tests**

Define and test the route/permission matrix: upload/assets/reprocess/attempts and asset `qa-approve`/`qa-reject` require `EnterprisePromotion:Media`; media consent-link GET/POST/DELETE require `EnterprisePromotion:Consent`; public ticket/refresh are anonymous but rate-limited and protected by allowed-origin/referrer policy where available. Test upload ownership, retries, list filtering, reprocess, attempt history, consent-link CRUD, QA reviewer/note/version validation, atomic `qa_pending → ready|quarantined` transition and audit, `case_media_id` ownership, slug/case-media mismatch, disallowed media roles, ready/authorized checks, revoked consent, media-version changes, nonce/expiry, short ticket claims, refresh-at-current-second behavior and rate limits.

- [ ] **Step 2: Run RED**

Run: `cd nx-backend/apps/server && go test ./internal/server -run PromotionMedia`

- [ ] **Step 3: Register and implement admin upload/consent handlers**

Expose initiate/sign-part/complete/abort/assets/reprocess/attempts plus `POST /api/admin/promotion-media/assets/:id/qa-approve` and `.../qa-reject`. Require server-generated object keys, configured size/part limits, authenticated reviewer identity, note/version checks and append-only QA audit.

- [ ] **Step 4: Register and implement public ticket handlers**

Ticket claims bind `case_media_id`, media version, expiry and nonce. Never accept raw asset IDs. Refresh verifies the same case-media association and returns resume instructions.

- [ ] **Step 5: Run GREEN and commit**

```bash
(cd nx-backend/apps/server && go test ./internal/server -run PromotionMedia)
git add nx-backend/apps/server/internal/server/enterprise_promotion_media*
git add nx-backend/apps/server/internal/server/server.go
git commit -m "feat: serve controlled enterprise case media"
```

### Task 7: Implement consultation, privacy and attribution services

**Files:**
- Create: `nx-backend/apps/server/internal/enterprisepromotion/consultations.go`
- Create: `nx-backend/apps/server/internal/enterprisepromotion/consultations_test.go`
- Create: `nx-backend/apps/server/internal/enterprisepromotion/analytics.go`
- Create: `nx-backend/apps/server/internal/enterprisepromotion/analytics_test.go`
- Create: `nx-backend/apps/server/internal/enterprisepromotion/consultations_postgres_test.go`
- Create: `nx-backend/apps/server/internal/enterprisepromotion/analytics_postgres_test.go`
- Create: `nx-backend/apps/server/internal/server/enterprise_promotion_privacy.go`
- Create: `nx-backend/apps/server/internal/server/enterprise_promotion_privacy_test.go`
- Modify: `nx-backend/apps/server/internal/server/enterprise_promotion_public.go`
- Modify: `nx-backend/apps/server/internal/server/enterprise_promotion_public_test.go`
- Modify: `nx-backend/apps/server/internal/server/server.go`
- Modify: `nx-backend/apps/server/internal/sms/sms.go`

- [ ] **Step 1: RED/GREEN consultation domain and public HTTP contract**

Write domain and HTTP tests for `POST /api/public/enterprise-consultations`: required `Idempotency-Key`, 24-hour idempotency TTL, timeout then same-key retry, `contact_consent=true`, current privacy notice version and `PRIVACY_NOTICE_OUTDATED`, encrypted PII, suspected-duplicate marking without silent loss, honeypot/minimum-submit-time/IP/device rate limits and verification escalation. Run `cd nx-backend/apps/server && go test ./internal/enterprisepromotion ./internal/server -run 'Consultation(Service|PublicHTTP)'` RED; implement the service and registered handler; rerun GREEN. A successful consultation remains committed if event ingestion fails, with an independent event error/status.

- [ ] **Step 2: RED/GREEN attribution sessions, events and share tokens**

Write domain and HTTP tests for `POST /api/public/promotion/sessions` and `/events`: trusted share-token reconstruction; expired/disabled token rejection; first-touch never overwritten; last-touch updated only by valid sources; 90-day sessions; event ID uniqueness; 25/50/90 playback deduplication; bounded payloads and rate limits. Run `cd nx-backend/apps/server && go test ./internal/enterprisepromotion ./internal/server -run 'Promotion(Session|Event|ShareToken)'` RED; implement registered handlers/services; rerun GREEN.

- [ ] **Step 3: RED/GREEN privacy-request workflow**

Write tests for reference hashing/rotation, SMS code TTL and one-time use, IP/device/reference rate limits, access/correction/deletion states, encrypted correction payloads, irreversible anonymization retaining minimal audit fields and append-only status history. Run `cd nx-backend/apps/server && go test ./internal/enterprisepromotion ./internal/server ./internal/sms -run 'ConsultationPrivacy|PrivacySMS'` RED; implement `enterprise_promotion_privacy.go` and SMS changes; rerun GREEN.

- [ ] **Step 4: RED/GREEN PostgreSQL concurrency and transaction behavior**

Using safe `TEST_DATABASE_URL`, test concurrent consultation idempotency, 24-hour expiry, encrypted columns, suspected duplicates, append-only notes, event uniqueness, first/last touch, reference hashes and privacy-request status transitions. Run `cd nx-backend/apps/server && TEST_DATABASE_URL="$TEST_DATABASE_URL" go test ./internal/enterprisepromotion -run 'ConsultationPostgres|AnalyticsPostgres|PrivacyPostgres' -v` RED and GREEN. Chunk 2 acceptance requires execution against an isolated loopback test database, not skip.

- [ ] **Step 5: Run complete GREEN and commit**

```bash
(cd nx-backend/apps/server && go test ./internal/enterprisepromotion ./internal/server ./internal/sms)
(cd nx-backend/apps/server && TEST_DATABASE_URL="$TEST_DATABASE_URL" go test ./internal/enterprisepromotion -run 'ConsultationPostgres|AnalyticsPostgres|PrivacyPostgres' -v)
git add nx-backend/apps/server/internal/enterprisepromotion nx-backend/apps/server/internal/server/enterprise_promotion_privacy* nx-backend/apps/server/internal/sms
git add nx-backend/apps/server/internal/server/enterprise_promotion_public* nx-backend/apps/server/internal/server/server.go
git commit -m "feat: capture attributable enterprise consultations"
```

### Task 8: Build admin APIs and typed client

**Files:**
- Create: `nx-backend/apps/server/internal/server/enterprise_promotion_admin.go`
- Create: `nx-backend/apps/server/internal/server/enterprise_promotion_admin_test.go`
- Create: `nx-backend/apps/server/internal/server/enterprise_promotion_pii_postgres_test.go`
- Create: `nx-backend/apps/web-antd/src/api/core/enterprise-promotion.ts`
- Create: `nx-backend/apps/web-antd/src/api/core/enterprise-promotion.test.ts`
- Modify: `nx-backend/apps/web-antd/src/api/core/index.ts`
- Modify: `nx-backend/apps/server/internal/server/server.go`

- [ ] **Step 1: RED/GREEN content CRUD contracts**

Cover registered list/detail/create/update for settings, trainers, cases and solutions, including method, permission and version checks. Run `(cd nx-backend/apps/server && go test ./internal/server -run 'PromotionAdminContent')` and `(cd nx-backend && pnpm test:unit -- apps/web-antd/src/api/core/enterprise-promotion.test.ts -t 'content CRUD')` RED; implement the narrow handlers/client methods; rerun GREEN.

- [ ] **Step 2: RED/GREEN media-management typed client contracts**

In the admin client test, cover explicit methods for initiate/sign-part/complete/abort, asset list/detail, attempt history, reprocess, media consent-link GET/POST/DELETE and QA approve/reject. Run `(cd nx-backend && pnpm test:unit -- apps/web-antd/src/api/core/enterprise-promotion.test.ts -t 'media management')` RED; implement promotion-owned typed methods; rerun GREEN. Do not expose a generic free-form endpoint caller.

- [ ] **Step 3: RED/GREEN publication and consent contracts**

Cover review/preview/publish/offline, consent links, testimonials/claims, media QA approve/reject typed methods, stable gate error codes, preview cache isolation, precise tagged-cache invalidation after publish/offline/reorder and `EnterprisePromotion:Publish|Consent|Media` 401/403 behavior. Run `(cd nx-backend/apps/server && go test ./internal/server -run 'PromotionAdminPublication|PromotionMediaQA')` and `(cd nx-backend && pnpm test:unit -- apps/web-antd/src/api/core/enterprise-promotion.test.ts -t 'publication and consent')` RED; implement; rerun GREEN.

- [ ] **Step 4: RED/GREEN leads, privacy and export contracts**

Cover leads/notes/export plus privacy-request list/detail/approve/reject/complete, `EnterprisePromotion:Leads|Export` permissions, verification-before-approval, legal state transitions and audit records. Add named PostgreSQL tests `TestPromotionPIIRBACPostgres`, `TestPromotionPIIDecryptAuditPostgres` and `TestPromotionExportAuditPostgres` proving masked lists, permission-gated decryption and complete export audit. Run `(cd nx-backend/apps/server && go test ./internal/server -run 'PromotionAdminLead|PromotionAdminPrivacy|PromotionAdminExport')`, `(cd nx-backend/apps/server && TEST_DATABASE_URL="$TEST_DATABASE_URL" go test ./internal/server -run 'TestPromotion(PIIRBAC|PIIDecryptAudit|ExportAudit)Postgres$' -v)` and `(cd nx-backend && pnpm test:unit -- apps/web-antd/src/api/core/enterprise-promotion.test.ts -t 'leads privacy export')` RED; implement; rerun GREEN. The PostgreSQL command must execute on an isolated loopback database and must not skip.

- [ ] **Step 5: RED/GREEN share-token and analytics contracts**

Cover share-token create/disable/list and analytics funnel queries with `EnterprisePromotion:Analytics`. Run `(cd nx-backend/apps/server && go test ./internal/server -run 'PromotionAdminShare|PromotionAdminAnalytics')` and `(cd nx-backend && pnpm test:unit -- apps/web-antd/src/api/core/enterprise-promotion.test.ts -t 'share and analytics')` RED; implement explicit typed client methods and handlers; rerun GREEN. Across all loops map version conflict to HTTP 409 and publication failures to stable codes (`CONSENT_MISSING`, `MEDIA_NOT_READY`, `CLAIM_UNREVIEWED`); preview uses a short admin token and never warms public cache.

- [ ] **Step 6: Run complete GREEN, typecheck and commit**

```bash
(cd nx-backend/apps/server && go test ./internal/server -run PromotionAdmin)
(cd nx-backend && pnpm test:unit -- apps/web-antd/src/api/core/enterprise-promotion.test.ts)
(cd nx-backend && pnpm -F @vben/web-antd run typecheck)
git add nx-backend/apps/server/internal/server/enterprise_promotion_admin* nx-backend/apps/web-antd/src/api/core
git add nx-backend/apps/server/internal/server/server.go
git commit -m "feat: add enterprise promotion admin APIs"
```

### Task 9: Build admin management UI

**Files:**
- Create: `nx-backend/apps/web-antd/src/router/routes/modules/enterprise-promotion.ts`
- Create: `nx-backend/apps/web-antd/src/views/enterprise-promotion/index.vue`
- Create: `nx-backend/apps/web-antd/src/views/enterprise-promotion/settings.vue`
- Create: `nx-backend/apps/web-antd/src/views/enterprise-promotion/cases.vue`
- Create: `nx-backend/apps/web-antd/src/views/enterprise-promotion/case-editor.vue`
- Create: `nx-backend/apps/web-antd/src/views/enterprise-promotion/solutions.vue`
- Create: `nx-backend/apps/web-antd/src/views/enterprise-promotion/trainers.vue`
- Create: `nx-backend/apps/web-antd/src/views/enterprise-promotion/media.vue`
- Create: `nx-backend/apps/web-antd/src/views/enterprise-promotion/media/upload-flow.ts`
- Create: `nx-backend/apps/web-antd/src/views/enterprise-promotion/media/upload-flow.test.ts`
- Create: `nx-backend/apps/web-antd/src/views/enterprise-promotion/media/upload-checksum.ts`
- Create: `nx-backend/apps/web-antd/src/views/enterprise-promotion/media/upload-checksum.test.ts`
- Create: `nx-backend/apps/web-antd/src/views/enterprise-promotion/consents.vue`
- Create: `nx-backend/apps/web-antd/src/views/enterprise-promotion/leads.vue`
- Create: `nx-backend/apps/web-antd/src/views/enterprise-promotion/analytics.vue`
- Create: `nx-backend/apps/web-antd/src/views/enterprise-promotion/enterprise-promotion.integration.test.ts`

- [ ] **Step 1: RED/GREEN settings and trainer screens**

Write route/menu and integration tests for permission-gated settings/trainers, version conflicts and validation. Run `cd nx-backend && pnpm test:unit -- apps/web-antd/src/views/enterprise-promotion/enterprise-promotion.integration.test.ts -t 'settings and trainers'` RED, implement, then GREEN.

- [ ] **Step 2: RED/GREEN case and solution screens**

Test list/editor separation, ordered associations, preview and deterministic publication failures. Run `cd nx-backend && pnpm test:unit -- apps/web-antd/src/views/enterprise-promotion/enterprise-promotion.integration.test.ts -t 'cases and solutions'` RED, implement, then GREEN.

- [ ] **Step 3: RED/GREEN media and consent screens**

Test upload resume, processing attempts, QA approve/reject actions with reviewer note/version, and consent gaps. Run `cd nx-backend && pnpm test:unit -- apps/web-antd/src/views/enterprise-promotion/enterprise-promotion.integration.test.ts -t 'media and consents'` RED. Reuse only the pinned commit with these exact read-only commands:

```bash
CLASSROOM_REF=8d41254b6cc08aa4b1381412acd18f49f9cbec01
git show "$CLASSROOM_REF":nx-backend/apps/web-antd/src/views/classroom/upload-flow.ts
git show "$CLASSROOM_REF":nx-backend/apps/web-antd/src/views/classroom/upload-checksum.ts
```

Create promotion-owned `media/upload-flow.ts` and `media/upload-checksum.ts` plus unit tests; do not import classroom modules. Run `cd nx-backend && pnpm test:unit -- apps/web-antd/src/views/enterprise-promotion/media/upload-flow.test.ts apps/web-antd/src/views/enterprise-promotion/media/upload-checksum.test.ts` RED, implement only against promotion APIs/permissions, then rerun both focused and integration tests GREEN. Do not read or modify the classroom worktree and do not merge/cherry-pick the classroom branch.

- [ ] **Step 4: RED/GREEN lead and privacy screens**

Test PII masking, notes, privacy-state actions and export audit. Run `cd nx-backend && pnpm test:unit -- apps/web-antd/src/views/enterprise-promotion/enterprise-promotion.integration.test.ts -t 'leads and privacy'` RED, implement and rerun GREEN.

- [ ] **Step 5: RED/GREEN analytics screen**

Test funnel/date filters and share-token state. Run `cd nx-backend && pnpm test:unit -- apps/web-antd/src/views/enterprise-promotion/enterprise-promotion.integration.test.ts -t 'analytics'` RED, implement and rerun GREEN.

- [ ] **Step 6: Run complete tests and typecheck**

Run:

```bash
cd nx-backend
pnpm test:unit -- apps/web-antd/src/views/enterprise-promotion/enterprise-promotion.integration.test.ts
pnpm -F @vben/web-antd run typecheck
```

Expected: PASS.

- [ ] **Step 7: Commit**

```bash
git add nx-backend/apps/web-antd/src/router/routes/modules/enterprise-promotion.ts nx-backend/apps/web-antd/src/views/enterprise-promotion
git commit -m "feat: manage enterprise promotion content"
```

---

## Chunk 3: Miniapp Navigation, Content Funnel and Consultation UX

### Task 10: Add miniapp API and pure normalization utilities

**Files:**
- Create: `miniapp/src/api/enterprisePromotion.js`
- Modify: `miniapp/src/api/index.js`
- Create: `miniapp/src/api/enterprisePromotion.test.mjs`
- Modify: `miniapp/src/api/request.js`
- Modify: `miniapp/src/api/request.test.mjs`
- Create: `miniapp/src/utils/enterprisePromotion.js`
- Create: `miniapp/src/utils/enterprisePromotion.test.mjs`
- Create: `miniapp/src/utils/enterpriseConsultation.js`
- Create: `miniapp/src/utils/enterpriseConsultation.test.mjs`
- Modify: `miniapp/package.json`

- [ ] **Step 1: Write failing API URL and normalization tests**

Test public endpoints, bounded topic keys, safe empty states, case-media playback URLs, CTA context expiry, first/last touch, form drafts, privacy version and idempotency-key reuse. Preserve every existing request/auth/error assertion in `request.test.mjs`, then incrementally add tests proving controlled custom headers reach `uni.request`, `Idempotency-Key` is sent on consultation submission, and callers cannot override the managed Authorization header.

- [ ] **Step 2: Run RED**

Run: `cd miniapp && node src/api/request.test.mjs && node src/api/enterprisePromotion.test.mjs && node src/utils/enterprisePromotion.test.mjs && node src/utils/enterpriseConsultation.test.mjs`

- [ ] **Step 3: Implement minimal modules**

Keep network methods in API, pure transformations in utils and storage keys/versioning in consultation helper. Extend `request.js` with an allow-listed custom-header option while preserving server URL, content type and managed authorization behavior.

- [ ] **Step 4: Add tests to `test:config` and run GREEN**

Run: `cd miniapp && npm run test:config`

- [ ] **Step 5: Commit**

```bash
git add miniapp/src/api miniapp/src/utils miniapp/package.json
git commit -m "feat: add enterprise promotion miniapp data layer"
```

### Task 11: Replace primary navigation while preserving old routes

**Files:**
- Modify: `miniapp/src/pages.json`
- Create: `miniapp/src/static/tabbar/home.png`
- Create: `miniapp/src/static/tabbar/home-active.png`
- Create: `miniapp/src/static/tabbar/cases.png`
- Create: `miniapp/src/static/tabbar/cases-active.png`
- Create: `miniapp/src/static/tabbar/solutions.png`
- Create: `miniapp/src/static/tabbar/solutions-active.png`
- Create: `miniapp/src/static/tabbar/consultation.png`
- Create: `miniapp/src/static/tabbar/consultation-active.png`
- Modify: `miniapp/scripts/ui-compat.test.mjs`
- Create: `miniapp/src/utils/enterpriseNavigation.js`
- Create: `miniapp/src/utils/enterpriseNavigation.test.mjs`
- Modify: `miniapp/src/utils/homeMenu.js`
- Modify: `miniapp/src/utils/homeMenu.test.mjs`
- Modify: `miniapp/src/pages/result/result.vue`
- Modify: `miniapp/src/pages/booking-records/booking-records.vue`
- Modify: `miniapp/src/pages/booking-records/booking-records.session.test.mjs`
- Modify: `miniapp/src/pages/booking-detail/booking-detail.vue`
- Modify: `miniapp/src/pages/booking-detail/booking-detail.session.test.mjs`
- Modify: `miniapp/src/pages/profile-edit/profile-edit.vue`
- Modify: `miniapp/package.json`
- Create: `miniapp/src/pages/training-cases/index.vue`
- Create: `miniapp/src/pages/enterprise-solutions/index.vue`
- Create: `miniapp/src/pages/enterprise-consultation/index.vue`
- Create: `miniapp/src/pages-enterprise/training-case/detail.vue`
- Create: `miniapp/src/pages-enterprise/enterprise-solution/detail.vue`
- Create: `miniapp/src/pages-enterprise/teacher/detail.vue`
- Create: `miniapp/src/pages-enterprise/consultation/form.vue`
- Create: `miniapp/src/pages-enterprise/privacy/consultation.vue`

- [ ] **Step 1: Write failing route contract tests**

Assert exact tabs `首页/培训案例/培训方案/咨询合作`, enterprise subpackage paths and continued registration of test/relation/learn/booking/profile/privacy routes. Scan all legacy navigation call sites and prove removed tabs use `navigateTo`/`redirectTo` through a compatibility helper, while only the four current tabs use `switchTab`.

- [ ] **Step 2: Run RED**

Run: `cd miniapp && node scripts/ui-compat.test.mjs && node src/utils/enterpriseNavigation.test.mjs && node src/utils/homeMenu.test.mjs`

- [ ] **Step 3: Update routes and icons**

Create minimal compilable skeletons for every newly registered Tab and enterprise subpackage page before editing `pages.json`. Consultation CTAs use `navigateTo('/pages-enterprise/consultation/form?...')`; never call `switchTab` with query parameters. Migrate result, booking-records, booking-detail, profile-edit and home-menu links away from `switchTab` for booking/profile/learn routes that are no longer tabs. Add `enterpriseNavigation.test.mjs` to `test:config` without removing existing tests.

- [ ] **Step 4: Run GREEN and build**

```bash
(cd miniapp && node scripts/ui-compat.test.mjs && node src/utils/enterpriseNavigation.test.mjs && node src/utils/homeMenu.test.mjs)
(cd miniapp && npm run test:config)
(cd miniapp && npm run build:mp-weixin)
```

- [ ] **Step 5: Commit**

```bash
git add miniapp/src/pages.json miniapp/src/static/tabbar miniapp/scripts/ui-compat.test.mjs miniapp/src/utils/enterpriseNavigation* miniapp/src/utils/homeMenu*
git add miniapp/src/pages/result miniapp/src/pages/booking-records miniapp/src/pages/booking-detail miniapp/src/pages/profile-edit
git add miniapp/src/pages/training-cases miniapp/src/pages/enterprise-solutions miniapp/src/pages/enterprise-consultation miniapp/src/pages-enterprise
git add miniapp/package.json
git commit -m "feat: focus miniapp navigation on enterprise training"
```

### Task 12: Build enterprise promotion home

**Files:**
- Rewrite: `miniapp/src/pages/index/index.vue`
- Create: `miniapp/src/pages/index/index.enterprise.test.mjs`
- Modify: `miniapp/scripts/ui-compat.test.mjs`
- Modify: `miniapp/package.json`

- [ ] **Step 1: Write failing page contract tests**

Require Hero, 30–90 second promo video metadata, trustworthy metrics with omission when unavailable, featured cases, solutions, trainer card, authorized text testimonials and both CTAs. Require a secondary “九型工具” section linking test/relation/learn without presenting them as primary tabs. Test cold start/share entry session creation, home impression, featured-case click and consultation-click events. For sharing, assert `onShareAppMessage` returns title/imageUrl/path and `onShareTimeline` returns title/imageUrl/query; both carry a valid share token/channel. Analytics failures never block navigation/rendering.

- [ ] **Step 2: Run RED**

Run: `cd miniapp && node src/pages/index/index.enterprise.test.mjs`

- [ ] **Step 3: Implement cached-then-refresh data flow**

Partial API failure preserves the last published snapshot; a failing media block does not hide cases or consultation CTA. Replace only old-home-specific assertions in `ui-compat.test.mjs` with enterprise-home contracts while preserving generic accessibility and legacy-page checks. Wire session/share/event APIs through the Task 10 helpers.

- [ ] **Step 4: Verify page and config suite**

Run: `cd miniapp && node src/pages/index/index.enterprise.test.mjs && node scripts/ui-compat.test.mjs && npm run test:config`

- [ ] **Step 5: Commit**

```bash
git add miniapp/src/pages/index miniapp/scripts/ui-compat.test.mjs miniapp/package.json
git commit -m "feat: present enterprise training promotion home"
```

### Task 13: Build case list, detail and controlled playback

**Files:**
- Rewrite: `miniapp/src/pages/training-cases/index.vue`
- Rewrite: `miniapp/src/pages-enterprise/training-case/detail.vue`
- Create: `miniapp/src/pages-enterprise/training-case/training-case.test.mjs`
- Create: `miniapp/src/utils/promotionPlayback.js`
- Create: `miniapp/src/utils/promotionPlayback.test.mjs`
- Modify: `miniapp/package.json`

- [ ] **Step 1: Write failing list/detail/playback tests**

Cover fixed topic entry, stable list, share-direct detail, unavailable case, multiple `case_media_id` videos, ticket refresh once, resume second, corrupt/unavailable media fallback and CTA context. Test list/card click, detail share-enter, video start and deduplicated 25/50/90 progress events, CTA click, and case-specific sharing: AppMessage title/imageUrl/path versus Timeline title/imageUrl/query, both with share token/channel. Event failures do not stop playback or navigation.

- [ ] **Step 2: Run RED**

Run: `cd miniapp && node src/pages-enterprise/training-case/training-case.test.mjs && node src/utils/promotionPlayback.test.mjs`

- [ ] **Step 3: Implement pages and playback helper**

Do not expose asset IDs/object keys. Stop refresh loops after one automatic attempt and keep the consultation CTA visible. Add both focused tests to `test:config`.

- [ ] **Step 4: Run GREEN and commit**

```bash
(cd miniapp && node src/pages-enterprise/training-case/training-case.test.mjs && node src/utils/promotionPlayback.test.mjs && npm run test:config)
git add miniapp/src/pages/training-cases miniapp/src/pages-enterprise/training-case miniapp/src/utils/promotionPlayback*
git add miniapp/package.json
git commit -m "feat: showcase enterprise training cases"
```

### Task 14: Build training solutions and trainer pages

**Files:**
- Rewrite: `miniapp/src/pages/enterprise-solutions/index.vue`
- Rewrite: `miniapp/src/pages-enterprise/enterprise-solution/detail.vue`
- Rewrite: `miniapp/src/pages-enterprise/teacher/detail.vue`
- Create: `miniapp/src/pages-enterprise/enterprise-solution/enterprise-solution.test.mjs`
- Modify: `miniapp/package.json`

- [ ] **Step 1: Write failing content and CTA tests**

Require audience, business problem, goal, modules, delivery methods, recommended participants/duration, related cases and “咨询定制方案”; reject purchase/member/progress wording. Test solution/card click, trainer click and CTA click. For solution/trainer shares, assert AppMessage title/imageUrl/path and Timeline title/imageUrl/query with token/channel. Test direct share-entry cold start parses the token, creates or resumes a promotion session and preserves first-touch while updating valid last-touch; analytics failure does not block page rendering/navigation.

- [ ] **Step 2: Run RED**

Run: `cd miniapp && node src/pages-enterprise/enterprise-solution/enterprise-solution.test.mjs`

- [ ] **Step 3: Implement pages**

Use the same published trainer entity across solution and trainer detail. Add the focused test to `test:config`.

- [ ] **Step 4: Run GREEN and commit**

```bash
(cd miniapp && node src/pages-enterprise/enterprise-solution/enterprise-solution.test.mjs && npm run test:config)
git add miniapp/src/pages/enterprise-solutions miniapp/src/pages-enterprise/enterprise-solution miniapp/src/pages-enterprise/teacher
git add miniapp/package.json
git commit -m "feat: explain enterprise training solutions"
```

### Task 15: Build consultation overview, form and privacy request UX

**Files:**
- Rewrite: `miniapp/src/pages/enterprise-consultation/index.vue`
- Rewrite: `miniapp/src/pages-enterprise/consultation/form.vue`
- Rewrite: `miniapp/src/pages-enterprise/privacy/consultation.vue`
- Create: `miniapp/src/pages-enterprise/consultation/enterprise-consultation.test.mjs`
- Create: `miniapp/src/pages-enterprise/privacy/consultation-privacy.test.mjs`
- Modify: `miniapp/package.json`

- [ ] **Step 1: Write failing form-state tests**

Cover generic Tab entry, non-Tab contextual form and 24-hour context expiry. Define required/optional validation for enterprise name, industry, city, participant count, business problem, interested solution, preferred training time, contact name, mobile, WeChat and notes. Cover consent checkbox/version, `PRIVACY_NOTICE_OUTDATED` refresh plus re-confirmation, idempotent retry after timeout, success reference display/storage, duplicate warning, spam challenge, draft preservation, consultation submit-success event and analytics failure isolation. Require Tab-page secondary links for “个人咨询” and “账号与隐私/预约记录” so legacy personal capabilities remain discoverable.

- [ ] **Step 2: Run RED**

Run: `cd miniapp && node src/pages-enterprise/consultation/enterprise-consultation.test.mjs`

- [ ] **Step 3: Write failing privacy-request state tests**

Run: `cd miniapp && node src/pages-enterprise/privacy/consultation-privacy.test.mjs`

Cover initiate by reference + original phone, SMS send countdown, code TTL/error/expired/used/rate-limited states, verify, request-type selection, submit success/reference, retry, reference-lost guidance and return navigation.

- [ ] **Step 4: Implement pages**

The Tab page never expects query params. Contextual CTA uses the non-Tab form. Privacy request UX implements the tested initiate → SMS → verify → request workflow. Add both focused tests to `test:config`.

- [ ] **Step 5: Run GREEN, full miniapp test and build**

```bash
(cd miniapp && node src/pages-enterprise/consultation/enterprise-consultation.test.mjs && node src/pages-enterprise/privacy/consultation-privacy.test.mjs && npm run test:config)
(cd miniapp && npm run build:mp-weixin)
```

- [ ] **Step 6: Commit**

```bash
git add miniapp/src/pages/enterprise-consultation miniapp/src/pages-enterprise/consultation miniapp/src/pages-enterprise/privacy
git add miniapp/package.json
git commit -m "feat: capture enterprise training consultations"
```

---

## Chunk 4: Source Media Audit, Deployment and Full Verification

### Task 16: Create reproducible source-media manifest and validation tools

**Files:**
- Create: `scripts/enterprise-media/build_manifest.py`
- Create: `scripts/enterprise-media/validate_media.py`
- Create: `scripts/enterprise-media/test_build_manifest.py`
- Create: `scripts/enterprise-media/test_validate_media.py`
- Create: `scripts/enterprise-media/manifest-example.redacted.json`
- Create: `docs/deployment/enterprise-promotion-media.md`
- Modify: `.gitignore`

- [ ] **Step 1: Write failing Python tests**

Use tiny generated valid/corrupt fixtures inside a temporary directory. Test randomized `SOURCE_ID` public output with no source basename/relative path plus a separate ignored mode-0600 private `SOURCE_ID → source-relative-path` mapping used only for local validation, exclusion of `._*`, SHA-256, ffprobe metadata, full decode failure, NAL/AAC classification and JSON report stability. Test safe subprocess construction: `-nostdin`, local-file-only protocol allowlist, timeout/cancellation, bounded concurrency, log truncation, controlled temp/output directories, disk-space guard and no delete/overwrite operation against source files.

- [ ] **Step 2: Run RED**

Run: `python3 scripts/enterprise-media/test_build_manifest.py && python3 scripts/enterprise-media/test_validate_media.py`

Expected: FAIL because scripts do not exist.

- [ ] **Step 3: Implement scripts**

Public manifest output must contain randomized source IDs rather than absolute paths, real relative directories or basenames. `build_manifest.py` writes a separate ignored private mapping with permission 0600; `validate_media.py` requires both `--source-root` and `--private-map`, rejects mappings escaping the source root, and deletes the private map on explicit `--purge-private-map` after QA retention expires. Keep raw media, frames, audio and local reports outside Git; commit only `manifest-example.redacted.json` with synthetic IDs/metadata and no enterprise/person/place identifiers. Add `.gitignore` rules for `tmp/enterprise-media/`, private maps, local manifests/reports/fixtures and common raw media extensions under audit output locations. FFmpeg/ffprobe use read-only inputs, `-nostdin`, network-protocol denial, null output or controlled temp output, per-process timeout/cancellation, low bounded concurrency, disk-space checks and truncated structured logs; timeout/resource exhaustion become failure codes and never mutate/delete source files.

- [ ] **Step 4: Run unit GREEN**

Run: `python3 scripts/enterprise-media/test_build_manifest.py && python3 scripts/enterprise-media/test_validate_media.py`

Expected: PASS.

- [ ] **Step 5: Validate KODAK material with exact read-only commands**

Run from repository root. Source paths are supplied only as shell environment variables from the operator's local session (never written to tracked files). All outputs stay under ignored `tmp/enterprise-media/`, private maps are mode 0600, and inputs are never opened for writing:

```bash
set -euo pipefail
test -n "$SOURCE_DIR_A" && test -n "$SOURCE_DIR_B" && test -n "$SOURCE_DIR_C"
mkdir -p tmp/enterprise-media/{0330,0331,zunyi}
python3 scripts/enterprise-media/build_manifest.py --source "$SOURCE_DIR_A" --output tmp/enterprise-media/0330/manifest.json --private-map tmp/enterprise-media/0330/source-map.json --redact-paths
python3 scripts/enterprise-media/validate_media.py --source-root "$SOURCE_DIR_A" --private-map tmp/enterprise-media/0330/source-map.json --manifest tmp/enterprise-media/0330/manifest.json --output tmp/enterprise-media/0330/report.json --full-decode-ext .mp4 --sample-ext .mov --sample-count 3 --read-only
python3 scripts/enterprise-media/build_manifest.py --source "$SOURCE_DIR_B" --output tmp/enterprise-media/0331/manifest.json --private-map tmp/enterprise-media/0331/source-map.json --redact-paths
python3 scripts/enterprise-media/validate_media.py --source-root "$SOURCE_DIR_B" --private-map tmp/enterprise-media/0331/source-map.json --manifest tmp/enterprise-media/0331/manifest.json --output tmp/enterprise-media/0331/report.json --full-decode-ext .mp4 --sample-ext .mov --sample-count 3 --read-only
python3 scripts/enterprise-media/build_manifest.py --source "$SOURCE_DIR_C" --output tmp/enterprise-media/zunyi/manifest.json --private-map tmp/enterprise-media/zunyi/source-map.json --redact-paths
python3 scripts/enterprise-media/validate_media.py --source-root "$SOURCE_DIR_C" --private-map tmp/enterprise-media/zunyi/source-map.json --manifest tmp/enterprise-media/zunyi/manifest.json --output tmp/enterprise-media/zunyi/report.json --full-decode-ext .mp4 --sample-ext .mov --sample-count 3 --read-only
test "$(stat -f '%Lp' tmp/enterprise-media/0330/source-map.json)" = 600
test "$(stat -f '%Lp' tmp/enterprise-media/0331/source-map.json)" = 600
test "$(stat -f '%Lp' tmp/enterprise-media/zunyi/source-map.json)" = 600
```

Expected: each command exits 0 only when the audit itself completed; media failures are recorded as structured quarantine results rather than hiding the report. Every long compiled MP4 is fully decoded. MOV files may be deterministically sampled for source audit, but no unsampled MOV is described as validated or publish-ready; every later publish candidate must pass the full Chunk 1 pipeline. Damaged aggregate MP4s are quarantined; valid sampled MOV sources remain candidates for re-editing.

- [ ] **Step 6: Stage only code/docs and run privacy checks**

```bash
set -euo pipefail
git add .gitignore scripts/enterprise-media/build_manifest.py scripts/enterprise-media/validate_media.py scripts/enterprise-media/test_build_manifest.py scripts/enterprise-media/test_validate_media.py scripts/enterprise-media/manifest-example.redacted.json docs/deployment/enterprise-promotion-media.md
printf '%s\n' .gitignore docs/deployment/enterprise-promotion-media.md scripts/enterprise-media/build_manifest.py scripts/enterprise-media/manifest-example.redacted.json scripts/enterprise-media/test_build_manifest.py scripts/enterprise-media/test_validate_media.py scripts/enterprise-media/validate_media.py | sort > tmp/enterprise-media/expected-staged.txt
git diff --cached --name-only | sort | diff -u tmp/enterprise-media/expected-staged.txt -
! git diff --cached --name-only | grep -Ei '\.(mov|mp4|m4v|mkv|avi|webm|mts|m2ts|mxf|m4a|aac|mp3|wav|flac|jpg|jpeg|png|heic|tif|tiff|raw|dng)$'
! git diff --cached | grep -E '(/Volumes/|/Users/|[A-Za-z]:\\|SOURCE_DIR_[A-C]=/)'
! git diff --cached | grep -Ei '(enterprise_name|person_name|place_name|real_filename|relative_path)'
if for f in $(git diff --cached --name-only); do file "$f"; done | grep -Evq 'text|JSON|empty'; then exit 1; fi
git diff --cached --check
```

Expected: only named scripts/tests/redacted example/docs are staged; no raw media, extracted frame/audio, real path/name or local report is staged.

- [ ] **Step 7: Commit**

```bash
git commit -m "feat: audit enterprise training media sources"
```

### Task 17: Run complete backend/admin/miniapp verification

**Files:**
- Create: `docs/superpowers/qa/2026-07-27-enterprise-training-promotion/qa.md`

Any verification failure returns to the owning Task: add/extend a regression test, confirm RED, implement the fix, run that Task's GREEN suite and commit the fix separately. Task 17 itself changes only QA evidence.

- [ ] **Step 1: Verify Git state and migration**

Run:

```bash
set -euo pipefail
git diff --check
git status --short
db_tests=$(cd nx-backend/apps/server && go test ./internal/db -list 'TestEnterprisePromotion(Postgres|RollbackPostgres)$')
for name in TestEnterprisePromotionPostgres TestEnterprisePromotionRollbackPostgres; do printf '%s\n' "$db_tests" | grep -qx "$name"; done
(set -euo pipefail; cd nx-backend/apps/server && TEST_DATABASE_URL="$TEST_DATABASE_URL" go test ./internal/db -run 'TestEnterprisePromotion(Postgres|RollbackPostgres)$' -count=1 -v | tee ../../../tmp/enterprise-media/db-migration-tests.log)
! grep -q -- '--- SKIP:' tmp/enterprise-media/db-migration-tests.log
(cd nx-backend/apps/server && go test ./internal/db -run EnterprisePromotion)
```

Expected: no conflict markers/whitespace errors; both named migration tests are listed, run and PASS against an isolated loopback test database with no skip.

- [ ] **Step 2: Run full Go suite**

Run: `(cd nx-backend/apps/server && go test ./...)`

Expected: PASS, zero failures.

- [ ] **Step 3: Run admin tests and typecheck**

```bash
set -euo pipefail
(cd nx-backend && pnpm test:unit)
(cd nx-backend && pnpm -F @vben/web-antd run typecheck)
(cd nx-backend && pnpm build:antd)
```

Expected: PASS.

- [ ] **Step 4: Run miniapp tests and build**

```bash
set -euo pipefail
(cd miniapp && npm run test:config)
(cd miniapp && npm run build:mp-weixin)
```

Expected: PASS and AppID/API verifier success.

- [ ] **Step 5: Verify PII RBAC, decryption and export audit**

Run focused backend/admin integration tests proving unauthenticated/unauthorized users receive 401/403; list views remain masked; decrypted PII detail access requires `EnterprisePromotion:Leads` and creates an audit entry; export requires `EnterprisePromotion:Export`, records requester/filter/count/hash and never includes deleted/anonymized fields.

Run:

```bash
set -euo pipefail
pii_tests=$(cd nx-backend/apps/server && go test ./internal/server -list 'TestPromotion(PIIRBAC|PIIDecryptAudit|ExportAudit)Postgres$')
for name in TestPromotionPIIRBACPostgres TestPromotionPIIDecryptAuditPostgres TestPromotionExportAuditPostgres; do printf '%s\n' "$pii_tests" | grep -qx "$name"; done
(set -euo pipefail; cd nx-backend/apps/server && TEST_DATABASE_URL="$TEST_DATABASE_URL" go test ./internal/server -run 'TestPromotion(PIIRBAC|PIIDecryptAudit|ExportAudit)Postgres$' -count=1 -v | tee ../../../tmp/enterprise-media/pii-audit-tests.log)
! grep -q -- '--- SKIP:' tmp/enterprise-media/pii-audit-tests.log
```

Expected: all three named tests are listed, run and PASS against the isolated loopback database with no skip.

- [ ] **Step 6: Re-run media tooling and record reproducible evidence**

Run:

```bash
set -euo pipefail
python3 scripts/enterprise-media/test_build_manifest.py
python3 scripts/enterprise-media/test_validate_media.py
command -v ffmpeg >/dev/null && command -v ffprobe >/dev/null
ffmpeg -version > tmp/enterprise-media/ffmpeg-version.txt
ffprobe -version > tmp/enterprise-media/ffprobe-version.txt
test -s tmp/enterprise-media/ffmpeg-version.txt && test -s tmp/enterprise-media/ffprobe-version.txt
sed -n '1p' tmp/enterprise-media/ffmpeg-version.txt
sed -n '1p' tmp/enterprise-media/ffprobe-version.txt
shasum -a 256 tmp/enterprise-media/0330/report.json tmp/enterprise-media/0331/report.json tmp/enterprise-media/zunyi/report.json
```

Expected: tests PASS and hashes are recorded in QA evidence. Before referencing a report, verify it contains only randomized source IDs and no absolute/real relative paths or names.

- [ ] **Step 7: Run manual scenario and publish-media matrix**

Document evidence for:

- public home → case → solution → contextual consultation;
- share-direct case landing;
- multiple video ticket/refresh/resume;
- authorization missing/revoked;
- damaged media quarantine;
- consultation timeout + same idempotency key;
- first/last touch attribution;
- privacy access/correction/deletion request;
- old test/relation/booking/profile deep links.

For every public **video asset** referenced by a homepage/case/solution/trainer snapshot, record its randomized source/derived IDs, full-decode PASS result, probe/hash and QA reviewer/time/note. If there are fewer than three public videos, play all of them uninterrupted from beginning to end; otherwise deterministically sample `max(3, ceil(10% of public videos))` for complete playback on target devices, including audio intelligibility and A/V sync. For every remaining public video, record opening/middle/end spot-checks. Public images, covers, logos and trainer portraits instead require hash, MIME/dimension limits, safe image decode, metadata stripping and applicable consent checks; they do not enter the video playback sample. Reject any public reference without its asset-type-specific chain. Separately test five target enterprise users without coaching: within 10 seconds, at least four must correctly state that this is “韩老师企业培训案例与合作咨询小程序”; also record whether they can then locate a real case and the consultation entry. Iterate through the owning UI Task if either threshold fails.

For every scenario, record command/action, UTC+local time, tested commit SHA, tool/app version, exit/result, randomized `SOURCE_ID` and evidence path (JSON/screenshot/log) under ignored local QA artifacts. The committed `qa.md` contains only redacted summaries/hashes. Run the staged privacy scan from Task 16 before commit.

- [ ] **Step 8: Commit QA evidence**

```bash
set -euo pipefail
git add docs/superpowers/qa/2026-07-27-enterprise-training-promotion/qa.md
printf '%s\n' docs/superpowers/qa/2026-07-27-enterprise-training-promotion/qa.md | diff -u - <(git diff --cached --name-only)
! git diff --cached --name-only | grep -Ei '\.(mov|mp4|m4v|mkv|avi|webm|m4a|aac|mp3|wav|flac|jpg|jpeg|png|heic|tif|tiff|raw|dng)$'
! git diff --cached | grep -E '(/Volumes/|/Users/|[A-Za-z]:\\|SOURCE_DIR_[A-C]=/)'
! git diff --cached | grep -Ei '(enterprise_name|person_name|place_name|real_filename|relative_path)'
git diff --cached --check
git commit -m "test: verify enterprise training promotion funnel"
```

### Task 18: Final review, rollout and rollback package

**Files:**
- Modify: `docs/deployment/enterprise-promotion-media.md`
- Create: `docs/deployment/enterprise-promotion-rollout.md`

- [ ] **Step 1: Document rollout order**

1. Database migration.
2. OSS/CORS/media worker configuration.
3. Backend/admin deploy.
4. Upload and approve redacted pilot case.
5. Miniapp review/release.
6. Enable public homepage snapshot.

- [ ] **Step 2: Document rollback**

Separate immediate server rollback from native-navigation rollback. The immediate kill switch returns a maintenance/home snapshot, offlines cases, stops new playback tickets and optionally pauses new consultation intake while preserving consultations and audit records; it does not claim to hide native tabs. To restore the previous tabBar, build and submit an emergency miniapp package containing the last stable `pages.json`; the four tabs remain visible until that client version is reviewed and released. Never delete source media or consent evidence during rollback.

Use forward-compatible, non-destructive schema and service rollback as the default: keep promotion tables/data in place, deploy the previous service version, disable new feature writes/tickets, and verify new consultations/audit rows remain queryable. Rehearse pausing/draining and restoring media worker queues without losing attempts; reverting OSS CORS/lifecycle settings from versioned configuration; disabling/re-enabling upload and ticket issuance; and forward recovery. A database restore/down migration is an exceptional last resort only after maintenance lock, incremental export of every post-backup consultation/audit/consent/event row, checksum/count verification, restore, replay into the compatible schema and post-replay reconciliation; document trigger/approval criteria and abort if any row cannot be preserved. Each rehearsal records commands, configuration version/checksum, health checks and recovery results.

- [ ] **Step 3: Run final fresh verification**

Run the complete commands from Task 17 again after all documentation/config changes.

- [ ] **Step 4: Request code review**

Use `superpowers:requesting-code-review`; address findings with `superpowers:receiving-code-review`.

- [ ] **Step 5: Commit rollout package**

```bash
set -euo pipefail
git add docs/deployment/enterprise-promotion-media.md docs/deployment/enterprise-promotion-rollout.md
printf '%s\n' docs/deployment/enterprise-promotion-media.md docs/deployment/enterprise-promotion-rollout.md | sort > tmp/enterprise-media/expected-rollout-staged.txt
git diff --cached --name-only | sort | diff -u tmp/enterprise-media/expected-rollout-staged.txt -
! git diff --cached --name-only | grep -Ei '\.(mov|mp4|m4v|mkv|avi|webm|m4a|aac|mp3|wav|flac|jpg|jpeg|png|heic|tif|tiff|raw|dng)$'
! git diff --cached | grep -E '(/Volumes/|/Users/|[A-Za-z]:\\|SOURCE_DIR_[A-C]=/)'
! git diff --cached | grep -Ei '(enterprise_name|person_name|place_name|real_filename|relative_path)'
git diff --cached --check
git commit -m "docs: add enterprise promotion rollout plan"
```

---

## Execution Constraints

- Keep `main` and `test` independent; this plan does not modify, merge or synchronize `test`.
- Work only on `feature/enterprise-training-promotion` until review and explicit integration.
- Do not merge the whole `feature/miniapp-teacher-classroom` branch. Reuse only reviewed, committed neutral media ideas/files.
- Do not publish any enterprise/person/logo/voice/testimonial asset without linked approved consent.
- Do not commit raw videos, extracted audio, faces, enterprise names or local source-volume absolute paths.
- Do not implement payments, memberships, classroom entitlements or learning progress in this feature.
- Use TDD for every task and commit after each green task.
