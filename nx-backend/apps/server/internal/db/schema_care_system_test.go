package db

import (
	"os"
	"strings"
	"testing"
)

func TestSchemaIncludesCareSystemContract(t *testing.T) {
	raw, err := os.ReadFile("schema.sql")
	if err != nil {
		t.Fatal(err)
	}
	sqlText := strings.Join(strings.Fields(string(raw)), " ")
	for _, column := range []string{
		"care_level SMALLINT",
		"care_label TEXT",
		"care_summary TEXT",
		"care_trend TEXT",
		"care_data_status TEXT",
		"care_evaluated_at TIMESTAMPTZ",
		"care_knowledge_version TEXT",
		"care_evaluation_version TEXT",
	} {
		if !strings.Contains(sqlText, column) {
			t.Errorf("app_users care column missing %q", column)
		}
	}
	for _, table := range []string{"care_evaluations", "care_evaluation_queue"} {
		if !strings.Contains(sqlText, "CREATE TABLE IF NOT EXISTS "+table) {
			t.Errorf("care table missing %q", table)
		}
	}
	for _, fragment := range []string{
		"CHECK (care_level IS NULL OR care_level BETWEEN 1 AND 10)",
		"app_user_id BIGINT NOT NULL REFERENCES app_users(id) ON DELETE CASCADE",
		"signals JSONB NOT NULL DEFAULT '{}'::jsonb",
		"idx_app_users_care_level",
		"idx_care_evaluation_queue_status",
	} {
		if !strings.Contains(sqlText, fragment) {
			t.Errorf("care schema missing %q", fragment)
		}
	}
}
