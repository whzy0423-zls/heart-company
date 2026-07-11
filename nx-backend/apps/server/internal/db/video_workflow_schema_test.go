package db

import (
	"os"
	"strings"
	"testing"
)

func TestVideoWorkflowSchema(t *testing.T) {
	raw, err := os.ReadFile("schema.sql")
	if err != nil {
		t.Fatalf("read schema: %v", err)
	}
	schema := string(raw)

	required := []string{
		"script_content TEXT NOT NULL DEFAULT ''",
		"script_revision INT NOT NULL DEFAULT 0",
		"final_video_input_hash TEXT NOT NULL DEFAULT ''",
		"generation_revision INT NOT NULL DEFAULT 0",
		"selected_generation_id BIGINT REFERENCES video_generations(id) ON DELETE SET NULL",
		"source_key TEXT NOT NULL DEFAULT ''",
		"source_script_revision INT NOT NULL DEFAULT 0",
		"sort_order INT NOT NULL DEFAULT 0",
		"shot_revision INT NOT NULL DEFAULT 0",
		"compose_input_hash TEXT NOT NULL DEFAULT ''",
		"compose_input_snapshot JSONB NOT NULL DEFAULT '{}'::jsonb",
		"progress INT NOT NULL DEFAULT 0",
		"CREATE TABLE IF NOT EXISTS video_generation_submissions",
		"CREATE UNIQUE INDEX IF NOT EXISTS idx_video_generation_submissions_active_shot",
		"ON video_generation_submissions(shot_id)",
		"WHERE status IN ('prepared','submitting','accepted','unknown_outcome','reconciled')",
		"CREATE UNIQUE INDEX IF NOT EXISTS idx_video_compose_jobs_active_project",
		"ON video_compose_jobs(project_id) WHERE status IN ('queued','processing')",
		"ON video_shots(project_id, source_key) WHERE source_key <> ''",
	}
	for _, fragment := range required {
		if !strings.Contains(schema, fragment) {
			t.Errorf("schema missing workflow contract %q", fragment)
		}
	}

	submissionTable := schemaStatement(t, schema, "CREATE TABLE IF NOT EXISTS video_generation_submissions")
	for _, fragment := range []string{
		"request_key UUID NOT NULL UNIQUE",
		"shot_id BIGINT NOT NULL REFERENCES video_shots(id) ON DELETE CASCADE",
		"generation_id BIGINT REFERENCES video_generations(id) ON DELETE SET NULL",
		"task_id TEXT NOT NULL DEFAULT ''",
		"status TEXT NOT NULL DEFAULT 'prepared'",
		"request_snapshot JSONB NOT NULL DEFAULT '{}'::jsonb",
		"error_message TEXT NOT NULL DEFAULT ''",
		"create_time TIMESTAMPTZ NOT NULL DEFAULT now()",
		"update_time TIMESTAMPTZ NOT NULL DEFAULT now()",
		"CHECK (status IN ('prepared','submitting','accepted','unknown_outcome','reconciled','completed','failed','cancelled'))",
	} {
		if !strings.Contains(submissionTable, fragment) {
			t.Errorf("submission table missing contract %q", fragment)
		}
	}

	compatibleColumns := []string{
		"ALTER TABLE video_projects ADD COLUMN IF NOT EXISTS script_content TEXT NOT NULL DEFAULT '';",
		"ALTER TABLE video_projects ADD COLUMN IF NOT EXISTS script_revision INT NOT NULL DEFAULT 0;",
		"ALTER TABLE video_projects ADD COLUMN IF NOT EXISTS final_video_input_hash TEXT NOT NULL DEFAULT '';",
		"ALTER TABLE video_shots ADD COLUMN IF NOT EXISTS generation_revision INT NOT NULL DEFAULT 0;",
		"ALTER TABLE video_shots ADD COLUMN IF NOT EXISTS selected_generation_id BIGINT REFERENCES video_generations(id) ON DELETE SET NULL;",
		"ALTER TABLE video_shots ADD COLUMN IF NOT EXISTS source_key TEXT NOT NULL DEFAULT '';",
		"ALTER TABLE video_shots ADD COLUMN IF NOT EXISTS source_script_revision INT NOT NULL DEFAULT 0;",
		"ALTER TABLE video_shot_assets ADD COLUMN IF NOT EXISTS sort_order INT NOT NULL DEFAULT 0;",
		"ALTER TABLE video_generations ADD COLUMN IF NOT EXISTS shot_revision INT NOT NULL DEFAULT 0;",
		"ALTER TABLE video_compose_jobs ADD COLUMN IF NOT EXISTS compose_input_hash TEXT NOT NULL DEFAULT '';",
		"ALTER TABLE video_compose_jobs ADD COLUMN IF NOT EXISTS compose_input_snapshot JSONB NOT NULL DEFAULT '{}'::jsonb;",
		"ALTER TABLE video_compose_jobs ADD COLUMN IF NOT EXISTS progress INT NOT NULL DEFAULT 0;",
	}
	for _, statement := range compatibleColumns {
		if !strings.Contains(schema, statement) {
			t.Errorf("schema missing compatible migration %q", statement)
		}
	}

	generationCreate := strings.Index(schema, "CREATE TABLE IF NOT EXISTS video_generations")
	selectedGenerationAlter := strings.Index(schema, "ALTER TABLE video_shots ADD COLUMN IF NOT EXISTS selected_generation_id")
	if generationCreate < 0 || selectedGenerationAlter < 0 || selectedGenerationAlter < generationCreate {
		t.Error("selected_generation_id must be added after video_generations exists")
	}

	assetBackfill := "ROW_NUMBER() OVER (PARTITION BY shot_id ORDER BY create_time, id) - 1"
	if !strings.Contains(schema, assetBackfill) {
		t.Errorf("schema missing deterministic asset sort backfill %q", assetBackfill)
	}

	selectedBackfill := schemaStatement(t, schema, "UPDATE video_shots AS shots")
	for _, fragment := range []string{
		"UPDATE video_shots AS shots",
		"SET selected_generation_id = generations.id",
		"generations.id = shots.generation_id",
		"generations.status IN ('completed','succeeded')",
		"generations.video_url <> ''",
	} {
		if !strings.Contains(selectedBackfill, fragment) {
			t.Errorf("schema missing safe selected generation backfill %q", fragment)
		}
	}
	for _, forbiddenStatus := range []string{"queued", "processing", "failed"} {
		if strings.Contains(selectedBackfill, forbiddenStatus) {
			t.Errorf("selected generation backfill must exclude %q generations", forbiddenStatus)
		}
	}
}

func schemaStatement(t *testing.T, schema, start string) string {
	t.Helper()
	startIndex := strings.Index(schema, start)
	if startIndex < 0 {
		t.Fatalf("schema missing statement starting with %q", start)
	}
	endIndex := strings.Index(schema[startIndex:], ";")
	if endIndex < 0 {
		t.Fatalf("schema statement starting with %q is not terminated", start)
	}
	return schema[startIndex : startIndex+endIndex+1]
}
