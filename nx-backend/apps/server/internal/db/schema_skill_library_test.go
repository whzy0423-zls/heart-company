package db

import (
	"os"
	"strings"
	"testing"
)

func TestSchemaIncludesVersionedSkillCatalog(t *testing.T) {
	raw, err := os.ReadFile("schema.sql")
	if err != nil {
		t.Fatal(err)
	}
	schema := strings.Join(strings.Fields(string(raw)), " ")
	for _, table := range []string{
		"app_skill_libraries",
		"app_skill_categories",
		"app_skills",
		"app_skill_versions",
	} {
		if !strings.Contains(schema, "CREATE TABLE IF NOT EXISTS "+table+" (") {
			t.Fatalf("schema missing %s", table)
		}
	}
	for _, contract := range []string{
		"UNIQUE (library_id, key)",
		"UNIQUE (skill_id, version)",
		"latest_published_version_id BIGINT",
		"theory_release_id BIGINT NOT NULL REFERENCES theory_library_releases(id) ON DELETE RESTRICT",
		"opening_prompts JSONB NOT NULL DEFAULT '[]'::jsonb",
		"CREATE TRIGGER trg_app_skill_versions_immutable",
		"published skill version is immutable",
		"CREATE TRIGGER trg_protect_published_skill_release_mapping",
		"CREATE TRIGGER trg_protect_published_skill_release_chunk",
		"published skill release mapping is immutable",
		"published skill release chunk is immutable",
		"NEW.status = 'retired' AND release_status = 'retired'",
	} {
		if !strings.Contains(schema, contract) {
			t.Fatalf("schema missing skill catalog contract %q", contract)
		}
	}
}

func TestSchemaEnforcesSkillChatSessionShapeAndIndexes(t *testing.T) {
	raw, err := os.ReadFile("schema.sql")
	if err != nil {
		t.Fatal(err)
	}
	schema := strings.Join(strings.Fields(string(raw)), " ")
	for _, contract := range []string{
		"ALTER TABLE app_chat_sessions ALTER COLUMN card_id DROP NOT NULL",
		"ALTER TABLE app_chat_sessions ADD COLUMN IF NOT EXISTS skill_version_id BIGINT REFERENCES app_skill_versions(id) ON DELETE RESTRICT",
		"ALTER TABLE app_chat_sessions ADD COLUMN IF NOT EXISTS generation_revision BIGINT NOT NULL DEFAULT 0",
		"scene = 'skill_chat' AND card_id IS NULL AND skill_version_id IS NOT NULL",
		"CREATE INDEX IF NOT EXISTS idx_app_chat_sessions_user_scene_updated ON app_chat_sessions(app_user_id, scene, updated_at DESC, id DESC)",
		"CREATE INDEX IF NOT EXISTS idx_app_chat_sessions_user_skill_updated ON app_chat_sessions(app_user_id, skill_version_id, updated_at DESC, id DESC)",
		"CREATE INDEX IF NOT EXISTS idx_app_chat_sessions_skill_updated ON app_chat_sessions(skill_version_id, updated_at DESC, id DESC)",
	} {
		if !strings.Contains(schema, contract) {
			t.Fatalf("schema missing skill session contract %q", contract)
		}
	}
}

func TestSchemaPersistsSkillGenerationTraceWithoutUserContent(t *testing.T) {
	raw, err := os.ReadFile("schema.sql")
	if err != nil {
		t.Fatal(err)
	}
	schema := strings.Join(strings.Fields(string(raw)), " ")
	for _, fragment := range []string{
		"CREATE TABLE IF NOT EXISTS app_skill_chat_traces",
		"assistant_message_id BIGINT NOT NULL UNIQUE",
		"generation_revision BIGINT NOT NULL",
		"skill_version_id BIGINT NOT NULL",
		"theory_release_id BIGINT NOT NULL",
		"chunk_ids BIGINT[] NOT NULL",
	} {
		if !strings.Contains(schema, fragment) {
			t.Fatalf("schema missing trace contract %q", fragment)
		}
	}
}
