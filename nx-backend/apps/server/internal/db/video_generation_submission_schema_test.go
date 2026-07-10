package db

import (
	"os"
	"strings"
	"testing"
)

func TestVideoGenerationSubmissionSchema(t *testing.T) {
	raw, err := os.ReadFile("schema.sql")
	if err != nil {
		t.Fatal(err)
	}
	schema := strings.Join(strings.Fields(string(raw)), " ")

	required := []string{
		"CREATE TABLE IF NOT EXISTS video_generation_submissions",
		"request_key UUID NOT NULL",
		"request_snapshot JSONB NOT NULL",
		"generation_id BIGINT REFERENCES video_generations(id) ON DELETE SET NULL",
		"CREATE UNIQUE INDEX IF NOT EXISTS uq_video_generation_submissions_request_key ON video_generation_submissions(request_key)",
		"CREATE UNIQUE INDEX IF NOT EXISTS uq_video_generation_submissions_active_shot ON video_generation_submissions(shot_id) WHERE shot_id IS NOT NULL AND status IN ('prepared','submitting','accepted','unknown_outcome')",
	}
	for _, statement := range required {
		if !strings.Contains(schema, statement) {
			t.Fatalf("schema is missing %q", statement)
		}
	}
}
