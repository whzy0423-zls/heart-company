package db

import (
	"os"
	"strings"
	"testing"
)

func TestLifeStorySchemaContract(t *testing.T) {
	raw, err := os.ReadFile("schema.sql")
	if err != nil {
		t.Fatal(err)
	}
	schema := strings.Join(strings.Fields(string(raw)), " ")
	for _, table := range []string{
		"app_life_stories", "app_life_story_materials", "app_life_story_versions",
		"app_life_story_jobs", "app_story_quota_periods", "app_story_quota_ledger", "app_life_story_outbox",
		"app_life_story_token_maps", "app_life_story_progress",
	} {
		if !strings.Contains(schema, "CREATE TABLE IF NOT EXISTS "+table+" (") {
			t.Fatalf("schema missing %s", table)
		}
	}
	for _, fragment := range []string{
		"app_user_id BIGINT NOT NULL REFERENCES app_users(id) ON DELETE CASCADE",
		"UNIQUE (app_user_id, request_key)",
		"payload_hash TEXT NOT NULL",
		"snapshot_hash TEXT NOT NULL",
		"source_version_id BIGINT",
		"worker_id TEXT NOT NULL",
		"retry_after TIMESTAMPTZ",
		"story_style TEXT NOT NULL DEFAULT 'realistic'",
		"CHECK (story_style IN ('realistic','novel','fairy_tale','myth'))",
		"ALTER TABLE app_life_story_versions ADD COLUMN IF NOT EXISTS story_style TEXT NOT NULL DEFAULT 'realistic'",
		"DROP CONSTRAINT IF EXISTS app_life_story_versions_story_style_check",
		"CHECK (status IN ('draft', 'outline_ready', 'queued', 'generating', 'completed', 'failed', 'cancelled', 'safety_blocked'))",
		"CHECK (source_type IN ('text', 'voice'))",
		"duration_ms INTEGER NOT NULL",
		"byte_length INTEGER NOT NULL",
		"UNIQUE (job_id, entry_type)",
		"source_key TEXT NOT NULL",
		"claim_token TEXT NOT NULL",
		"lease_until TIMESTAMPTZ",
		"next_attempt_at TIMESTAMPTZ",
		"idx_app_life_stories_user_updated",
		"idx_app_life_story_jobs_claimable",
		"idx_app_life_story_jobs_one_active_per_user",
	} {
		if !strings.Contains(schema, fragment) {
			t.Fatalf("schema missing contract %q", fragment)
		}
	}
}
