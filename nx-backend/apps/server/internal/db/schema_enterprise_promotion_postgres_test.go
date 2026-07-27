package db

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"net/url"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/stdlib"
)

func TestEnterprisePromotionTestDSNRejectsRoutingOverrides(t *testing.T) {
	tests := []string{
		"postgres://postgres:secret@127.0.0.1/nx_test?host=remote.example",
		"postgres://postgres:secret@127.0.0.1/nx_test?hostaddr=203.0.113.10",
		"postgres://postgres:secret@127.0.0.1/nx_test?service=production",
		"host=127.0.0.1 dbname=nx_test service=production",
	}
	for _, dsn := range tests {
		if _, err := validateEnterprisePromotionTestDSN(dsn); err == nil {
			t.Errorf("routing override accepted: %q", dsn)
		}
	}
}

func TestEnterprisePromotionTestDSNAcceptsFinalLoopbackTestTarget(t *testing.T) {
	config, err := validateEnterprisePromotionTestDSN("postgres://postgres:secret@127.0.0.1:5432/nx_enterprise_test?sslmode=disable")
	if err != nil {
		t.Fatal(err)
	}
	if config.Host != "127.0.0.1" || config.Database != "nx_enterprise_test" {
		t.Fatalf("final target host=%q database=%q", config.Host, config.Database)
	}
}

func validateEnterprisePromotionTestDSN(dsn string) (*pgx.ConnConfig, error) {
	lower := strings.ToLower(dsn)
	if strings.HasPrefix(lower, "postgres://") || strings.HasPrefix(lower, "postgresql://") {
		parsed, err := url.Parse(dsn)
		if err != nil {
			return nil, fmt.Errorf("parse TEST_DATABASE_URL: %w", err)
		}
		for _, key := range []string{"host", "hostaddr", "service", "servicefile"} {
			if _, exists := parsed.Query()[key]; exists {
				return nil, fmt.Errorf("TEST_DATABASE_URL must not use %s routing override", key)
			}
		}
	} else {
		for _, field := range strings.Fields(lower) {
			for _, key := range []string{"hostaddr=", "service=", "servicefile="} {
				if strings.HasPrefix(field, key) {
					return nil, fmt.Errorf("TEST_DATABASE_URL must not use %s routing override", strings.TrimSuffix(key, "="))
				}
			}
		}
	}
	config, err := pgx.ParseConfig(dsn)
	if err != nil {
		return nil, fmt.Errorf("parse final TEST_DATABASE_URL config: %w", err)
	}
	host := strings.ToLower(strings.TrimSpace(config.Host))
	if host != "localhost" && host != "127.0.0.1" && host != "::1" {
		return nil, fmt.Errorf("TEST_DATABASE_URL final host must be loopback, got %q", host)
	}
	if !strings.Contains(strings.ToLower(config.Database), "test") {
		return nil, fmt.Errorf("TEST_DATABASE_URL final database must be isolated test database, got %q", config.Database)
	}
	if config.Database == "" {
		return nil, errors.New("TEST_DATABASE_URL final database is empty")
	}
	return config, nil
}

func TestEnterprisePromotionPostgres(t *testing.T) {
	db, schemaName := openEnterprisePromotionSchema(t)
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

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

	var assetID int64
	if err := db.QueryRowContext(ctx, `INSERT INTO promotion_media_assets(asset_key,kind,object_key,sha256,state) VALUES ('asset-one','video','promotion/source/asset-one','aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa','uploaded') RETURNING id`).Scan(&assetID); err != nil {
		t.Fatal(err)
	}
	if _, err := db.ExecContext(ctx, `UPDATE promotion_media_assets SET state='ready' WHERE id=$1`, assetID); err == nil {
		t.Fatal("ready gate accepted an asset without passing QA")
	}
	var reviewerID int64
	if err := db.QueryRowContext(ctx, `INSERT INTO users(username,password_hash) VALUES ('promotion-reviewer','hash') RETURNING id`).Scan(&reviewerID); err != nil {
		t.Fatal(err)
	}
	var oldReviewID int64
	if err := db.QueryRowContext(ctx, `INSERT INTO promotion_media_qa_reviews(asset_id,qa_result,approved_by,qa_note) VALUES ($1,'passed',$2,'first review') RETURNING id`, assetID, reviewerID).Scan(&oldReviewID); err != nil {
		t.Fatal(err)
	}
	if _, err := db.ExecContext(ctx, `UPDATE promotion_media_assets SET state='ready', qa_result='passed', qa_approved_by=$2, qa_approved_at=now() WHERE id=$1`, assetID, reviewerID); err == nil {
		t.Fatal("ready gate accepted a QA review committed by an earlier transaction")
	}
	if _, err := db.ExecContext(ctx, `UPDATE promotion_media_qa_reviews SET qa_note='rewritten' WHERE id=$1`, oldReviewID); err == nil {
		t.Fatal("append-only QA review was mutable")
	}
	tx, err := db.BeginTx(ctx, nil)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := tx.ExecContext(ctx, `INSERT INTO promotion_media_qa_reviews(asset_id,qa_result,approved_by,qa_note) VALUES ($1,'passed',$2,'release approval')`, assetID, reviewerID); err != nil {
		_ = tx.Rollback()
		t.Fatal(err)
	}
	if _, err := tx.ExecContext(ctx, `UPDATE promotion_media_assets SET state='ready', qa_result='passed', qa_approved_by=$2, qa_approved_at=now(), qa_note='release approval' WHERE id=$1`, assetID, reviewerID); err != nil {
		_ = tx.Rollback()
		t.Fatalf("same-transaction ready + QA review rejected: %v", err)
	}
	if err := tx.Commit(); err != nil {
		t.Fatal(err)
	}
	if _, err := db.ExecContext(ctx, `UPDATE promotion_media_assets SET qa_note='rewritten snapshot' WHERE id=$1`, assetID); err == nil {
		t.Fatal("ready asset QA snapshot was mutable without a current-transaction review")
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
	dsn := strings.TrimSpace(os.Getenv("TEST_DATABASE_URL"))
	if dsn == "" {
		t.Skip("TEST_DATABASE_URL is absent; PostgreSQL enterprise promotion tests skipped without connecting to any development database")
	}
	config, err := validateEnterprisePromotionTestDSN(dsn)
	if err != nil {
		t.Fatal(err)
	}
	adminDB := stdlib.OpenDB(*config)
	ctx, cancel := context.WithTimeout(context.Background(), 45*time.Second)
	t.Cleanup(cancel)
	if err := adminDB.PingContext(ctx); err != nil {
		_ = adminDB.Close()
		t.Fatalf("connect to isolated test database: %v", err)
	}
	schemaName := fmt.Sprintf("enterprise_promotion_%d", time.Now().UnixNano())
	if _, err := adminDB.ExecContext(ctx, `CREATE SCHEMA `+schemaName); err != nil {
		_ = adminDB.Close()
		t.Fatal(err)
	}
	t.Cleanup(func() {
		_, _ = adminDB.Exec(`DROP SCHEMA IF EXISTS ` + schemaName + ` CASCADE`)
		_ = adminDB.Close()
	})

	scoped := config.Copy()
	if scoped.RuntimeParams == nil {
		scoped.RuntimeParams = make(map[string]string)
	}
	scoped.RuntimeParams["search_path"] = schemaName + ",public"
	database := stdlib.OpenDB(*scoped)
	t.Cleanup(func() { _ = database.Close() })
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
