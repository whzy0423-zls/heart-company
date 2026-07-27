package db

import (
	"context"
	"database/sql"
	"strings"
	"testing"
	"time"

	"nine-xing/nx-backend/apps/server/internal/testdb"
)

func TestEnterprisePromotionPostgres(t *testing.T) {
	db, schemaName := openEnterprisePromotionSchema(t)
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	assertEnterprisePromotionCatalog(t, ctx, db)

	var trainerID int64
	if err := db.QueryRowContext(ctx, `INSERT INTO enterprise_trainers(key,name,status) VALUES ('trainer-one','Trainer','published') RETURNING id`).Scan(&trainerID); err != nil {
		t.Fatal(err)
	}
	if _, err := db.ExecContext(ctx, `INSERT INTO enterprise_trainers(key,name) VALUES ('trainer-one','Duplicate')`); err == nil {
		t.Fatal("trainer key UNIQUE constraint did not reject duplicate")
	}
	if _, err := db.ExecContext(ctx, `INSERT INTO training_cases(slug,title,trainer_id,trainer_name_snapshot) VALUES ('bad-fk','Bad',999999,'Missing')`); err == nil {
		t.Fatal("case trainer FK did not reject missing trainer")
	}
	var caseID int64
	if err := db.QueryRowContext(ctx, `INSERT INTO training_cases(slug,title,trainer_id,trainer_name_snapshot) VALUES ('case-one','Case', $1,'Trainer') RETURNING id`, trainerID).Scan(&caseID); err != nil {
		t.Fatal(err)
	}
	if _, err := db.ExecContext(ctx, `DELETE FROM enterprise_trainers WHERE id=$1`, trainerID); err == nil {
		t.Fatal("trainer RESTRICT constraint allowed referenced delete")
	}
	if _, err := db.ExecContext(ctx, `UPDATE training_cases SET status='unknown' WHERE id=$1`, caseID); err == nil {
		t.Fatal("case status CHECK accepted unknown state")
	}

	var assetID, assetVersion int64
	if err := db.QueryRowContext(ctx, `INSERT INTO promotion_media_assets(asset_key,kind,object_key,sha256,state) VALUES ('asset-one','video','promotion/source/asset-one','aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa','uploaded') RETURNING id,version`).Scan(&assetID, &assetVersion); err != nil {
		t.Fatal(err)
	}
	if assetVersion != 1 {
		t.Fatalf("initial asset version=%d", assetVersion)
	}
	if _, err := db.ExecContext(ctx, `UPDATE promotion_media_assets SET state='qa_pending',qa_result='failed',qa_note='stale review' WHERE id=$1`, assetID); err != nil {
		t.Fatal(err)
	}
	var assetState, assetQAResult, assetQANote string
	if err := db.QueryRowContext(ctx, `UPDATE promotion_media_assets SET probe_metadata='{"duration":10}' WHERE id=$1 RETURNING version,state,qa_result,qa_note`, assetID).Scan(&assetVersion, &assetState, &assetQAResult, &assetQANote); err != nil {
		t.Fatal(err)
	}
	if assetVersion != 2 || assetState != "uploaded" || assetQAResult != "pending" || assetQANote != "" {
		t.Fatalf("identity mutation snapshot: version=%d state=%q qa=%q note=%q", assetVersion, assetState, assetQAResult, assetQANote)
	}
	if _, err := db.ExecContext(ctx, `UPDATE promotion_media_assets SET state='qa_pending' WHERE id=$1`, assetID); err != nil {
		t.Fatalf("qa_pending state rejected: %v", err)
	}
	if _, err := db.ExecContext(ctx, `UPDATE promotion_media_assets SET state='ready' WHERE id=$1`, assetID); err == nil {
		t.Fatal("ready gate accepted an asset without passing QA")
	}
	var reviewerID int64
	if err := db.QueryRowContext(ctx, `INSERT INTO users(username,password_hash) VALUES ('promotion-reviewer','hash') RETURNING id`).Scan(&reviewerID); err != nil {
		t.Fatal(err)
	}
	var attemptID int64
	const assetSHA = "aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa"
	if err := db.QueryRowContext(ctx, `INSERT INTO promotion_media_processing_attempts(asset_id,attempt_number,state,output_sha256) VALUES ($1,1,'succeeded',$2) RETURNING id`, assetID, assetSHA).Scan(&attemptID); err != nil {
		t.Fatal(err)
	}
	stampTx, err := db.BeginTx(ctx, nil)
	if err != nil {
		t.Fatal(err)
	}
	var expectedApprovalTxID, stampedReviewID int64
	if err := stampTx.QueryRowContext(ctx, `SELECT txid_current()`).Scan(&expectedApprovalTxID); err != nil {
		_ = stampTx.Rollback()
		t.Fatal(err)
	}
	if err := stampTx.QueryRowContext(ctx, `INSERT INTO promotion_media_qa_reviews(asset_id,attempt_id,qa_result,approved_by,qa_note,approval_txid,asset_version,output_sha256) VALUES ($1,$2,'failed',$3,'tamper check',1,999,$4) RETURNING id`, assetID, attemptID, reviewerID, strings.Repeat("b", 64)).Scan(&stampedReviewID); err != nil {
		_ = stampTx.Rollback()
		t.Fatal(err)
	}
	if err := stampTx.Commit(); err != nil {
		t.Fatal(err)
	}
	var storedApprovalTxID, storedAssetVersion int64
	var storedOutputSHA string
	if err := db.QueryRowContext(ctx, `SELECT approval_txid,asset_version,output_sha256 FROM promotion_media_qa_reviews WHERE id=$1`, stampedReviewID).Scan(&storedApprovalTxID, &storedAssetVersion, &storedOutputSHA); err != nil {
		t.Fatal(err)
	}
	if storedApprovalTxID != expectedApprovalTxID || storedAssetVersion != assetVersion || storedOutputSHA != assetSHA {
		t.Fatalf("QA snapshot was caller-controlled: tx=%d version=%d sha=%q", storedApprovalTxID, storedAssetVersion, storedOutputSHA)
	}
	var oldReviewID int64
	var oldApprovedAt time.Time
	if err := db.QueryRowContext(ctx, `INSERT INTO promotion_media_qa_reviews(asset_id,attempt_id,qa_result,approved_by,qa_note) VALUES ($1,$2,'passed',$3,'first review') RETURNING id,approved_at`, assetID, attemptID, reviewerID).Scan(&oldReviewID, &oldApprovedAt); err != nil {
		t.Fatal(err)
	}
	if _, err := db.ExecContext(ctx, `UPDATE promotion_media_assets SET state='ready', ready_qa_review_id=$2, ready_attempt_id=$3, qa_result='passed', qa_approved_by=$4, qa_approved_at=$5, qa_note='first review' WHERE id=$1`, assetID, oldReviewID, attemptID, reviewerID, oldApprovedAt); err == nil {
		t.Fatal("ready gate accepted a QA review committed by an earlier transaction")
	}
	if _, err := db.ExecContext(ctx, `UPDATE promotion_media_qa_reviews SET qa_note='rewritten' WHERE id=$1`, oldReviewID); err == nil {
		t.Fatal("append-only QA review was mutable")
	}
	mismatchTx, err := db.BeginTx(ctx, nil)
	if err != nil {
		t.Fatal(err)
	}
	var mismatchReviewID int64
	var mismatchApprovedAt time.Time
	if err := mismatchTx.QueryRowContext(ctx, `INSERT INTO promotion_media_qa_reviews(asset_id,attempt_id,qa_result,approved_by,qa_note) VALUES ($1,$2,'passed',$3,'exact note') RETURNING id,approved_at`, assetID, attemptID, reviewerID).Scan(&mismatchReviewID, &mismatchApprovedAt); err != nil {
		_ = mismatchTx.Rollback()
		t.Fatal(err)
	}
	if _, err := mismatchTx.ExecContext(ctx, `UPDATE promotion_media_assets SET state='ready', ready_qa_review_id=$2, ready_attempt_id=$3, qa_result='passed', qa_approved_by=$4, qa_approved_at=$5, qa_note='wrong note' WHERE id=$1`, assetID, mismatchReviewID, attemptID, reviewerID, mismatchApprovedAt); err == nil {
		_ = mismatchTx.Rollback()
		t.Fatal("ready gate accepted QA snapshot fields that did not exactly match review")
	}
	_ = mismatchTx.Rollback()

	tx, err := db.BeginTx(ctx, nil)
	if err != nil {
		t.Fatal(err)
	}
	var readyReviewID int64
	var readyApprovedAt time.Time
	if err := tx.QueryRowContext(ctx, `INSERT INTO promotion_media_qa_reviews(asset_id,attempt_id,qa_result,approved_by,qa_note) VALUES ($1,$2,'passed',$3,'release approval') RETURNING id,approved_at`, assetID, attemptID, reviewerID).Scan(&readyReviewID, &readyApprovedAt); err != nil {
		_ = tx.Rollback()
		t.Fatal(err)
	}
	if _, err := tx.ExecContext(ctx, `UPDATE promotion_media_assets SET state='ready', ready_qa_review_id=$2, ready_attempt_id=$3, qa_result='passed', qa_approved_by=$4, qa_approved_at=$5, qa_note='release approval' WHERE id=$1`, assetID, readyReviewID, attemptID, reviewerID, readyApprovedAt); err != nil {
		_ = tx.Rollback()
		t.Fatalf("same-transaction ready + QA review rejected: %v", err)
	}
	if err := tx.Commit(); err != nil {
		t.Fatal(err)
	}
	if _, err := db.ExecContext(ctx, `UPDATE promotion_media_assets SET qa_note='rewritten snapshot' WHERE id=$1`, assetID); err == nil {
		t.Fatal("ready asset QA snapshot was mutable without a current-transaction review")
	}
	if _, err := db.ExecContext(ctx, `UPDATE promotion_media_assets SET object_key='promotion/source/replaced' WHERE id=$1`, assetID); err == nil {
		t.Fatal("ready asset identity was mutable")
	}
	if _, err := db.ExecContext(ctx, `UPDATE promotion_media_assets SET kind='audio' WHERE id=$1`, assetID); err == nil {
		t.Fatal("ready asset kind was mutable")
	}
	if _, err := db.ExecContext(ctx, `UPDATE promotion_media_processing_attempts SET output_sha256=$2 WHERE id=$1`, attemptID, strings.Repeat("c", 64)); err == nil {
		t.Fatal("QA-bound processing attempt identity was mutable")
	}
	var snapshotReviewID, snapshotAttemptID, snapshotVersion int64
	var snapshotSHA, snapshotNote string
	var snapshotApprovedAt time.Time
	if err := db.QueryRowContext(ctx, `SELECT ready_qa_review_id,ready_attempt_id,version,sha256,qa_note,qa_approved_at FROM promotion_media_assets WHERE id=$1`, assetID).Scan(&snapshotReviewID, &snapshotAttemptID, &snapshotVersion, &snapshotSHA, &snapshotNote, &snapshotApprovedAt); err != nil {
		t.Fatal(err)
	}
	if snapshotReviewID != readyReviewID || snapshotAttemptID != attemptID || snapshotVersion != assetVersion || snapshotSHA != assetSHA || snapshotNote != "release approval" || !snapshotApprovedAt.Equal(readyApprovedAt) {
		t.Fatal("ready asset QA snapshot does not exactly match approved content version")
	}
	if _, err := db.ExecContext(ctx, `INSERT INTO promotion_media_processing_attempts(asset_id,attempt_number,state) VALUES ($1,1,'queued'),($1,1,'queued')`, assetID); err == nil {
		t.Fatal("processing attempt uniqueness did not reject duplicate")
	}

	var consentID int64
	if err := db.QueryRowContext(ctx, `INSERT INTO publication_consents(subject_type,subject_reference,status) VALUES ('media_asset','asset-one','approved') RETURNING id`).Scan(&consentID); err != nil {
		t.Fatal(err)
	}
	if _, err := db.ExecContext(ctx, `INSERT INTO training_case_consent_links(consent_id,media_asset_id,subject_type,subject_id,use_scope,requirement_key) VALUES ($1,$2,'media_asset',$2,'public_playback','media-publication')`, consentID, assetID); err != nil {
		t.Fatalf("media-only consent link rejected: %v", err)
	}
	if _, err := db.ExecContext(ctx, `INSERT INTO training_case_consent_links(consent_id,media_asset_id,subject_type,subject_id,use_scope,requirement_key) VALUES ($1,$2,'person',99,'public_playback','wrong-type')`, consentID, assetID); err == nil {
		t.Fatal("consent/link subject type mismatch accepted")
	}
	var testimonialConsentID, testimonialID int64
	if err := db.QueryRowContext(ctx, `INSERT INTO publication_consents(subject_type,subject_reference,status) VALUES ('testimonial','quote-one','approved') RETURNING id`).Scan(&testimonialConsentID); err != nil {
		t.Fatal(err)
	}
	if err := db.QueryRowContext(ctx, `INSERT INTO training_case_testimonials(case_id,quote,provenance,consent_id,position) VALUES ($1,'Quote','recording',$2,0) RETURNING id`, caseID, testimonialConsentID).Scan(&testimonialID); err != nil {
		t.Fatal(err)
	}
	if _, err := db.ExecContext(ctx, `INSERT INTO training_case_consent_links(case_id,consent_id,testimonial_id,subject_type,subject_id,use_scope,requirement_key) VALUES ($1,$2,$3,'testimonial',$4,'public_quote','quote')`, caseID, testimonialConsentID, testimonialID, testimonialID+1); err == nil {
		t.Fatal("testimonial consent link subject_id mismatch accepted")
	}

	var sessionID int64
	if err := db.QueryRowContext(ctx, `INSERT INTO promotion_sessions(session_key,first_touch, last_touch) VALUES ('session-one','{}','{}') RETURNING id`).Scan(&sessionID); err != nil {
		t.Fatal(err)
	}
	if _, err := db.ExecContext(ctx, `INSERT INTO promotion_events(session_id,event_type,page_path,idempotency_key) VALUES ($1,'page_view','/cases','event-one'),($1,'page_view','/cases','event-one')`, sessionID); err == nil {
		t.Fatal("event idempotency uniqueness did not reject duplicate")
	}
	var secondSessionID int64
	if err := db.QueryRowContext(ctx, `INSERT INTO promotion_sessions(session_key,first_touch,last_touch) VALUES ('session-two','{}','{}') RETURNING id`).Scan(&secondSessionID); err != nil {
		t.Fatal(err)
	}
	if _, err := db.ExecContext(ctx, `INSERT INTO promotion_events(session_id,event_type,page_path,idempotency_key) VALUES ($1,'page_view','/cases','global-event'),($2,'page_view','/cases','global-event')`, sessionID, secondSessionID); err == nil {
		t.Fatal("event idempotency key was not globally unique")
	}
	if _, err := db.ExecContext(ctx, enterprisePromotionMigrationSQL(t)); err != nil {
		t.Fatalf("idempotent forward migration with persisted QA/consent/event data: %v", err)
	}

	var gotSchema string
	if err := db.QueryRowContext(ctx, `SELECT current_schema()`).Scan(&gotSchema); err != nil || gotSchema != schemaName {
		t.Fatalf("current schema=%q err=%v, want %q", gotSchema, err, schemaName)
	}
}

func TestEnterprisePromotionRollbackPostgres(t *testing.T) {
	db, _ := openEnterprisePromotionSchema(t)
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	var consultationID int64
	if err := db.QueryRowContext(ctx, `
		INSERT INTO enterprise_consultations(
			consultation_reference_hash, request_idempotency_hash, company_name_encrypted, contact_name_encrypted,
			phone_encrypted, privacy_notice_version, consented_at, consent_source
		) VALUES ('reference-hash', 'request-hash', '\x01', '\x02', '\x03', 'v1', now(), 'miniapp') RETURNING id
	`).Scan(&consultationID); err != nil {
		t.Fatal(err)
	}
	var auditID int64
	if err := db.QueryRowContext(ctx, `INSERT INTO enterprise_promotion_audit_logs(entity_type,entity_id,action,detail) VALUES ('consultation',$1,'created','{}') RETURNING id`, consultationID).Scan(&auditID); err != nil {
		t.Fatal(err)
	}

	if _, err := db.ExecContext(ctx, `INSERT INTO enterprise_promotion_settings(key,draft_config,published_config) VALUES ('feature-state','{"enabled":false}','{"enabled":false}')`); err != nil {
		t.Fatal(err)
	}
	var legacyStatus string
	if err := db.QueryRowContext(ctx, `SELECT status FROM enterprise_consultations WHERE id=$1`, consultationID).Scan(&legacyStatus); err != nil || legacyStatus != "new" {
		t.Fatalf("feature-disabled compatibility read status=%q err=%v", legacyStatus, err)
	}
	if _, err := db.ExecContext(ctx, enterprisePromotionMigrationSQL(t)); err != nil {
		t.Fatalf("forward recovery schema application: %v", err)
	}

	var consultationCount, auditCount int
	if err := db.QueryRowContext(ctx, `SELECT count(*) FROM enterprise_consultations WHERE id=$1`, consultationID).Scan(&consultationCount); err != nil {
		t.Fatal(err)
	}
	if err := db.QueryRowContext(ctx, `SELECT count(*) FROM enterprise_promotion_audit_logs WHERE id=$1`, auditID).Scan(&auditCount); err != nil {
		t.Fatal(err)
	}
	if consultationCount != 1 || auditCount != 1 {
		t.Fatalf("rollback/forward recovery lost records: consultations=%d audit=%d", consultationCount, auditCount)
	}
}

func openEnterprisePromotionSchema(t *testing.T) (*sql.DB, string) {
	t.Helper()
	database, schemaName := testdb.OpenEnvIsolatedSchema(t, "enterprise_promotion")
	ctx, cancel := context.WithTimeout(context.Background(), 45*time.Second)
	defer cancel()
	if _, err := database.ExecContext(ctx, `
		CREATE TABLE users (
			id BIGSERIAL PRIMARY KEY,
			username TEXT NOT NULL UNIQUE,
			password_hash TEXT NOT NULL
		)
	`); err != nil {
		t.Fatalf("create isolated prerequisite schema: %v", err)
	}
	migration := enterprisePromotionMigrationSQL(t)
	for pass := 1; pass <= 2; pass++ {
		if _, err := database.ExecContext(ctx, migration); err != nil {
			t.Fatalf("apply schema pass %d: %v", pass, err)
		}
	}
	return database, schemaName
}

func enterprisePromotionMigrationSQL(t *testing.T) string {
	t.Helper()
	const marker = "-- ----- 企业培训推广：媒体、案例、方案、授权、线索和基础归因 -----"
	start := strings.Index(schemaSQL, marker)
	if start < 0 {
		t.Fatal("enterprise promotion migration section not found in schema.sql")
	}
	return schemaSQL[start:]
}

func assertEnterprisePromotionCatalog(t *testing.T, ctx context.Context, db *sql.DB) {
	t.Helper()
	columns := map[string][]string{
		"promotion_media_assets":     {"version", "ready_qa_review_id", "ready_attempt_id", "object_key", "sha256", "byte_size", "source_asset_id", "probe_metadata", "derived_metadata"},
		"promotion_media_qa_reviews": {"asset_version", "attempt_id", "output_sha256", "qa_result", "approved_by", "approved_at", "qa_note", "approval_txid"},
		"training_cases":             {"business_challenges", "training_goals", "training_modules", "training_methods", "trainer_id", "version"},
		"enterprise_solutions":       {"audiences", "problems", "goals", "modules", "delivery_methods", "recommended_participants", "recommended_duration", "customizable_items"},
		"enterprise_trainers":        {"specialties", "credentials", "service_industries"},
		"publication_consents":       {"channels", "usage_scopes", "evidence_asset_id", "reviewed_by", "reviewed_at", "revocation_reason"},
		"enterprise_consultations":   {"request_idempotency_hash", "company_name_encrypted", "requirements_encrypted", "contact_name_encrypted", "phone_encrypted", "phone_lookup_hash", "wechat_encrypted", "note_encrypted"},
	}
	for table, names := range columns {
		for _, name := range names {
			var count int
			if err := db.QueryRowContext(ctx, `SELECT count(*) FROM information_schema.columns WHERE table_schema=current_schema() AND table_name=$1 AND column_name=$2`, table, name).Scan(&count); err != nil {
				t.Fatal(err)
			}
			if count != 1 {
				t.Errorf("catalog missing %s.%s", table, name)
			}
		}
	}
	assertColumnCatalog(t, ctx, db, "promotion_media_assets", "version", "bigint", "NO", "1")
	assertColumnCatalog(t, ctx, db, "promotion_media_qa_reviews", "asset_version", "bigint", "NO", "")
	assertColumnCatalog(t, ctx, db, "promotion_media_qa_reviews", "attempt_id", "bigint", "NO", "")
	assertColumnCatalog(t, ctx, db, "promotion_media_qa_reviews", "output_sha256", "text", "NO", "")
	assertColumnCatalog(t, ctx, db, "enterprise_consultations", "phone_encrypted", "bytea", "NO", "")
	assertColumnCatalog(t, ctx, db, "training_case_media", "id", "bigint", "NO", "nextval")

	assertConstraintDefinitionContains(t, ctx, db, "promotion_media_assets", "qa_pending")
	assertConstraintDefinitionContains(t, ctx, db, "promotion_media_assets", "FOREIGN KEY (ready_qa_review_id) REFERENCES promotion_media_qa_reviews(id) ON DELETE RESTRICT")
	assertConstraintDefinitionContains(t, ctx, db, "promotion_media_assets", "FOREIGN KEY (ready_attempt_id) REFERENCES promotion_media_processing_attempts(id) ON DELETE RESTRICT")
	assertConstraintDefinitionContains(t, ctx, db, "training_cases", "FOREIGN KEY (trainer_id) REFERENCES enterprise_trainers(id) ON DELETE RESTRICT")
	assertConstraintDefinitionContains(t, ctx, db, "training_cases", "UNIQUE (slug)")
	assertConstraintDefinitionContains(t, ctx, db, "training_case_media", "draft")
	assertConstraintDefinitionContains(t, ctx, db, "training_case_media", "published")
	assertConstraintDefinitionContains(t, ctx, db, "training_case_media", "offline")
	assertConstraintDefinitionExcludes(t, ctx, db, "training_case_media", "review")
	assertConstraintDefinitionContains(t, ctx, db, "training_case_consent_links", "FOREIGN KEY (consent_id, subject_type) REFERENCES publication_consents(id, subject_type)")
	assertConstraintDefinitionContains(t, ctx, db, "training_case_consent_links", "testimonial_id = subject_id")

	rows, err := db.QueryContext(ctx, `SELECT indexdef FROM pg_indexes WHERE schemaname=current_schema() AND tablename='promotion_events'`)
	if err != nil {
		t.Fatal(err)
	}
	defer rows.Close()
	uniqueIdempotency, sessionLookup := 0, false
	for rows.Next() {
		var definition string
		if err := rows.Scan(&definition); err != nil {
			t.Fatal(err)
		}
		if strings.Contains(definition, "UNIQUE") && strings.Contains(definition, "(idempotency_key)") {
			uniqueIdempotency++
		}
		if !strings.Contains(definition, "UNIQUE") && strings.Contains(definition, "(session_id") {
			sessionLookup = true
		}
	}
	if err := rows.Err(); err != nil {
		t.Fatal(err)
	}
	if uniqueIdempotency != 1 || !sessionLookup {
		t.Fatalf("promotion_events indexes: global idempotency unique=%d session lookup=%v", uniqueIdempotency, sessionLookup)
	}
}

func assertColumnCatalog(t *testing.T, ctx context.Context, db *sql.DB, table, column, dataType, nullable, defaultFragment string) {
	t.Helper()
	var gotType, gotNullable string
	var gotDefault sql.NullString
	if err := db.QueryRowContext(ctx, `
		SELECT data_type,is_nullable,column_default
		FROM information_schema.columns
		WHERE table_schema=current_schema() AND table_name=$1 AND column_name=$2
	`, table, column).Scan(&gotType, &gotNullable, &gotDefault); err != nil {
		t.Fatal(err)
	}
	if gotType != dataType || gotNullable != nullable {
		t.Errorf("catalog %s.%s type=%q nullable=%q", table, column, gotType, gotNullable)
	}
	if defaultFragment != "" && (!gotDefault.Valid || !strings.Contains(gotDefault.String, defaultFragment)) {
		t.Errorf("catalog %s.%s default=%q missing %q", table, column, gotDefault.String, defaultFragment)
	}
}

func assertConstraintDefinitionContains(t *testing.T, ctx context.Context, db *sql.DB, table, fragment string) {
	t.Helper()
	definitions := constraintDefinitions(t, ctx, db, table)
	for _, definition := range definitions {
		if strings.Contains(definition, fragment) {
			return
		}
	}
	t.Errorf("catalog constraints for %s missing %q: %v", table, fragment, definitions)
}

func assertConstraintDefinitionExcludes(t *testing.T, ctx context.Context, db *sql.DB, table, fragment string) {
	t.Helper()
	for _, definition := range constraintDefinitions(t, ctx, db, table) {
		if strings.Contains(definition, fragment) {
			t.Errorf("catalog constraint for %s unexpectedly contains %q: %s", table, fragment, definition)
		}
	}
}

func constraintDefinitions(t *testing.T, ctx context.Context, db *sql.DB, table string) []string {
	t.Helper()
	rows, err := db.QueryContext(ctx, `
		SELECT pg_get_constraintdef(c.oid)
		FROM pg_constraint c
		JOIN pg_class r ON r.oid=c.conrelid
		JOIN pg_namespace n ON n.oid=r.relnamespace
		WHERE n.nspname=current_schema() AND r.relname=$1
	`, table)
	if err != nil {
		t.Fatal(err)
	}
	defer rows.Close()
	var definitions []string
	for rows.Next() {
		var definition string
		if err := rows.Scan(&definition); err != nil {
			t.Fatal(err)
		}
		definitions = append(definitions, definition)
	}
	if err := rows.Err(); err != nil {
		t.Fatal(err)
	}
	return definitions
}
