package db

import (
	"context"
	"database/sql"
	"fmt"
	"os"
	"strings"
	"testing"
	"time"

	"nine-xing/nx-backend/apps/server/internal/testutil"

	_ "github.com/jackc/pgx/v5/stdlib"
)

func TestSchemaIncludesProfileCalibrationTables(t *testing.T) {
	raw, err := os.ReadFile("schema.sql")
	if err != nil {
		t.Fatal(err)
	}
	schema := string(raw)
	for _, required := range []string{
		"CREATE TABLE IF NOT EXISTS app_daily_quiz_questions",
		"CREATE TABLE IF NOT EXISTS app_daily_quiz_sets",
		"CREATE TABLE IF NOT EXISTS app_daily_quiz_question_versions",
		"CREATE TABLE IF NOT EXISTS app_daily_quiz_batches",
		"CREATE TABLE IF NOT EXISTS app_daily_quiz_answers",
		"CREATE TABLE IF NOT EXISTS app_profile_evidence",
		"CREATE TABLE IF NOT EXISTS app_reassessment_jobs",
		"CREATE TABLE IF NOT EXISTS app_profile_versions",
		"idx_app_daily_quiz_batches_card_date_round",
		"idx_app_daily_quiz_answers_batch_question",
		"idx_app_daily_quiz_sets_date",
		"idx_app_daily_quiz_question_versions_active",
		"idx_app_reassessment_jobs_open",
		"idx_app_profile_versions_active",
		"push_claimed_at",
		"push_sent_at   TIMESTAMPTZ",
		"push_sent_at         TIMESTAMPTZ",
		"slot_no",
		"version_no",
		"raw_response",
	} {
		if !strings.Contains(schema, required) {
			t.Fatalf("expected schema to include %s", required)
		}
	}
}

func TestSchemaMigratesLegacyDailyQuizQuestionsTypeWeights(t *testing.T) {
	raw, err := os.ReadFile("schema.sql")
	if err != nil {
		t.Fatal(err)
	}
	schema := strings.Join(strings.Fields(string(raw)), " ")
	for _, required := range []string{
		"ALTER TABLE app_daily_quiz_questions ADD COLUMN IF NOT EXISTS type_weights JSONB",
		"UPDATE app_daily_quiz_questions SET type_weights = '{}'::jsonb WHERE type_weights IS NULL",
		"ALTER TABLE app_daily_quiz_questions ALTER COLUMN type_weights SET DEFAULT '{}'::jsonb",
		"ALTER TABLE app_daily_quiz_questions ALTER COLUMN type_weights SET NOT NULL",
	} {
		if !strings.Contains(schema, required) {
			t.Fatalf("legacy daily quiz migration missing %q", required)
		}
	}
}

func TestProfileCalibrationSchemaMigratesLegacyTypeWeightsIdempotently(t *testing.T) {
	dsn := strings.TrimSpace(os.Getenv("TEST_DATABASE_URL"))
	if dsn == "" {
		t.Skip("set TEST_DATABASE_URL to run profile calibration schema migration test")
	}
	if err := testutil.ValidateIsolatedPostgresDSN(dsn); err != nil {
		t.Fatal(err)
	}

	adminDatabase, err := sql.Open("pgx", dsn)
	if err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithTimeout(context.Background(), 90*time.Second)
	defer cancel()
	schemaName := fmt.Sprintf("profile_calibration_legacy_%d", time.Now().UnixNano())
	if _, err := adminDatabase.ExecContext(ctx, "CREATE SCHEMA "+schemaName); err != nil {
		_ = adminDatabase.Close()
		t.Fatalf("create isolated test schema: %v", err)
	}
	scopedDSN, err := postgresDSNWithSearchPath(dsn, schemaName)
	if err != nil {
		t.Fatal(err)
	}
	database, err := sql.Open("pgx", scopedDSN)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		_ = database.Close()
		_, _ = adminDatabase.ExecContext(context.Background(), "DROP SCHEMA IF EXISTS "+schemaName+" CASCADE")
		_ = adminDatabase.Close()
	})

	for run := 1; run <= 2; run++ {
		if _, err := database.ExecContext(ctx, schemaSQL); err != nil {
			t.Fatalf("apply full schema to fresh database pass %d: %v", run, err)
		}
	}
	var freshTypeWeightsColumns, chatSceneColumns int
	if err := database.QueryRowContext(ctx, `
		SELECT count(*)
		FROM information_schema.columns
		WHERE table_schema=current_schema()
		  AND table_name='app_daily_quiz_questions'
		  AND column_name='type_weights'
	`).Scan(&freshTypeWeightsColumns); err != nil {
		t.Fatalf("read fresh type_weights column: %v", err)
	}
	if freshTypeWeightsColumns != 1 {
		t.Fatalf("fresh type_weights column count=%d, want 1", freshTypeWeightsColumns)
	}
	if err := database.QueryRowContext(ctx, `
		SELECT count(*)
		FROM information_schema.columns
		WHERE table_schema=current_schema()
		  AND table_name='app_chat_sessions'
		  AND column_name='scene'
	`).Scan(&chatSceneColumns); err != nil {
		t.Fatalf("read fresh app_chat_sessions.scene column: %v", err)
	}
	if chatSceneColumns != 1 {
		t.Fatalf("fresh app_chat_sessions.scene column count=%d, want 1", chatSceneColumns)
	}

	if _, err := adminDatabase.ExecContext(ctx, "DROP SCHEMA "+schemaName+" CASCADE; CREATE SCHEMA "+schemaName); err != nil {
		t.Fatalf("reset test schema for legacy migration: %v", err)
	}
	if _, err := database.ExecContext(ctx, `
		CREATE TABLE app_daily_quiz_questions (
			id BIGSERIAL PRIMARY KEY,
			sort INT NOT NULL DEFAULT 0,
			body TEXT NOT NULL DEFAULT '',
			options JSONB NOT NULL DEFAULT '[]'::jsonb,
			dimension TEXT NOT NULL DEFAULT '',
			status TEXT NOT NULL DEFAULT 'active',
			create_time TIMESTAMPTZ NOT NULL DEFAULT now(),
			update_time TIMESTAMPTZ NOT NULL DEFAULT now()
		);
		INSERT INTO app_daily_quiz_questions(sort, body, options, dimension)
		VALUES (10, '旧题目', '[]'::jsonb, 'legacy');
	`); err != nil {
		t.Fatalf("create legacy daily quiz schema: %v", err)
	}

	for run := 1; run <= 2; run++ {
		if _, err := database.ExecContext(ctx, schemaSQL); err != nil {
			t.Fatalf("apply full schema pass %d: %v", run, err)
		}
	}

	var nullable, defaultValue string
	if err := database.QueryRowContext(ctx, `
		SELECT is_nullable, column_default
		FROM information_schema.columns
		WHERE table_schema=current_schema()
		  AND table_name='app_daily_quiz_questions'
		  AND column_name='type_weights'
	`).Scan(&nullable, &defaultValue); err != nil {
		t.Fatalf("read migrated type_weights column: %v", err)
	}
	if nullable != "NO" {
		t.Fatalf("type_weights nullable=%q, want NO", nullable)
	}
	if !strings.Contains(defaultValue, "'{}'::jsonb") {
		t.Fatalf("type_weights default=%q, want empty json object", defaultValue)
	}

	var legacyWeights string
	if err := database.QueryRowContext(ctx, `SELECT type_weights::text FROM app_daily_quiz_questions WHERE body='旧题目'`).Scan(&legacyWeights); err != nil {
		t.Fatalf("read legacy row type_weights: %v", err)
	}
	if legacyWeights != "{}" {
		t.Fatalf("legacy type_weights=%q, want {}", legacyWeights)
	}

	var defaultWeights string
	if err := database.QueryRowContext(ctx, `
		INSERT INTO app_daily_quiz_questions(sort, body, options, dimension)
		VALUES (20, '迁移后题目', '[]'::jsonb, 'new')
		RETURNING type_weights::text
	`).Scan(&defaultWeights); err != nil {
		t.Fatalf("insert row using migrated default: %v", err)
	}
	if defaultWeights != "{}" {
		t.Fatalf("default type_weights=%q, want {}", defaultWeights)
	}
	if _, err := database.ExecContext(ctx, `
		INSERT INTO app_daily_quiz_questions(sort, body, options, dimension, type_weights)
		VALUES (30, '非法空权重', '[]'::jsonb, 'invalid', NULL)
	`); err == nil {
		t.Fatal("type_weights should reject NULL after migration")
	}
}
