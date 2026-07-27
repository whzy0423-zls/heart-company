package db

import (
	"context"
	"database/sql"
	"fmt"
	"net"
	"net/url"
	"os"
	"path"
	"strings"
	"testing"
	"time"

	_ "github.com/jackc/pgx/v5/stdlib"
)

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
	if _, err := db.ExecContext(ctx, `UPDATE promotion_media_assets SET state='ready', qa_result='passed', qa_approved_by=$2, qa_approved_at=now() WHERE id=$1`, assetID, trainerID); err == nil {
		t.Fatal("QA reviewer FK incorrectly accepted trainer as a users.id")
	}
	var reviewerID int64
	if err := db.QueryRowContext(ctx, `INSERT INTO users(username,password_hash) VALUES ('promotion-reviewer','hash') RETURNING id`).Scan(&reviewerID); err != nil {
		t.Fatal(err)
	}
	if _, err := db.ExecContext(ctx, `UPDATE promotion_media_assets SET state='ready', qa_result='passed', qa_approved_by=$2, qa_approved_at=now() WHERE id=$1`, assetID, reviewerID); err != nil {
		t.Fatalf("same-statement ready + passing QA rejected: %v", err)
	}
	if _, err := db.ExecContext(ctx, `INSERT INTO promotion_media_processing_attempts(asset_id,attempt_number,state) VALUES ($1,1,'queued'),($1,1,'queued')`, assetID); err == nil {
		t.Fatal("processing attempt uniqueness did not reject duplicate")
	}

	var sessionID int64
	if err := db.QueryRowContext(ctx, `INSERT INTO promotion_sessions(session_key,first_touch, last_touch) VALUES ('session-one','{}','{}') RETURNING id`).Scan(&sessionID); err != nil {
		t.Fatal(err)
	}
	if _, err := db.ExecContext(ctx, `INSERT INTO promotion_events(session_id,event_type,page_path,idempotency_key) VALUES ($1,'page_view','/cases','event-one'),($1,'page_view','/cases','event-one')`, sessionID); err == nil {
		t.Fatal("event idempotency uniqueness did not reject duplicate")
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

	tx, err := db.BeginTx(ctx, nil)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := tx.ExecContext(ctx, `DELETE FROM enterprise_promotion_audit_logs; DELETE FROM enterprise_consultations; ALTER TABLE enterprise_consultations RENAME TO enterprise_consultations_rolled_back`); err != nil {
		_ = tx.Rollback()
		t.Fatal(err)
	}
	if err := tx.Rollback(); err != nil {
		t.Fatal(err)
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
	parsed, err := url.Parse(dsn)
	if err != nil {
		t.Fatalf("parse TEST_DATABASE_URL: %v", err)
	}
	if parsed.Scheme != "postgres" && parsed.Scheme != "postgresql" {
		t.Fatalf("TEST_DATABASE_URL must use PostgreSQL, got %q", parsed.Scheme)
	}
	host := strings.ToLower(parsed.Hostname())
	if host != "localhost" && host != "127.0.0.1" && host != "::1" && !net.ParseIP(host).IsLoopback() {
		t.Fatalf("TEST_DATABASE_URL must use a loopback host, got %q", host)
	}
	databaseName := path.Base(strings.TrimSuffix(parsed.Path, "/"))
	if databaseName == "." || databaseName == "/" || !strings.Contains(strings.ToLower(databaseName), "test") {
		t.Fatalf("TEST_DATABASE_URL must name an isolated test database, got %q", databaseName)
	}
	if parsed.Query().Get("database") != "" || parsed.Query().Get("dbname") != "" {
		t.Fatal("TEST_DATABASE_URL must not override the isolated database name in query parameters")
	}

	adminDB, err := sql.Open("pgx", dsn)
	if err != nil {
		t.Fatal(err)
	}
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

	scoped := *parsed
	query := scoped.Query()
	query.Set("search_path", schemaName+",public")
	scoped.RawQuery = query.Encode()
	database, err := sql.Open("pgx", scoped.String())
	if err != nil {
		t.Fatal(err)
	}
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
