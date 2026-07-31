package db

import (
	"os"
	"strings"
	"testing"
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
