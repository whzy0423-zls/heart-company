package videoproject

import (
	"os"
	"strings"
	"testing"
)

func TestWorkflowSchemaContract(t *testing.T) {
	raw, err := os.ReadFile("../db/schema.sql")
	if err != nil {
		t.Fatal(err)
	}
	schema := normalizeSchemaSQL(string(raw))

	assertSchemaFragments(t, schema, []string{
		"ALTER TABLE video_projects ADD COLUMN IF NOT EXISTS script_content TEXT NOT NULL DEFAULT '';",
		"ALTER TABLE video_projects ADD COLUMN IF NOT EXISTS script_summary TEXT NOT NULL DEFAULT '';",
		"ALTER TABLE video_projects ADD COLUMN IF NOT EXISTS script_revision INT NOT NULL DEFAULT 0;",
		"ALTER TABLE video_projects ADD COLUMN IF NOT EXISTS confirmed_script_revision INT NOT NULL DEFAULT 0;",
		"ALTER TABLE video_projects ADD COLUMN IF NOT EXISTS workflow_step TEXT NOT NULL DEFAULT 'script';",
		"ALTER TABLE video_projects ADD COLUMN IF NOT EXISTS workflow_mode TEXT NOT NULL DEFAULT 'guided' CHECK (workflow_mode IN ('guided','autopilot'));",
		"ALTER TABLE video_projects ADD COLUMN IF NOT EXISTS workflow_settings JSONB NOT NULL DEFAULT '{}'::jsonb;",
		"ALTER TABLE video_projects ADD COLUMN IF NOT EXISTS workflow_settings_revision INT NOT NULL DEFAULT 0;",
		"ALTER TABLE video_projects ADD COLUMN IF NOT EXISTS asset_revision INT NOT NULL DEFAULT 0;",
		"ALTER TABLE video_projects ADD COLUMN IF NOT EXISTS script_confirmed_at TIMESTAMPTZ;",
		"ALTER TABLE video_projects ADD COLUMN IF NOT EXISTS breakdown_confirmed_at TIMESTAMPTZ;",
		"ALTER TABLE video_projects ADD COLUMN IF NOT EXISTS storyboard_confirmed_at TIMESTAMPTZ;",
	})

	assertSchemaFragments(t, schema, []string{
		"CREATE TABLE IF NOT EXISTS video_project_breakdowns (",
		"project_id BIGINT NOT NULL REFERENCES video_projects(id) ON DELETE CASCADE",
		"version INT NOT NULL CHECK (version > 0)",
		"revision INT NOT NULL DEFAULT 1 CHECK (revision > 0)",
		"status TEXT NOT NULL DEFAULT 'draft' CHECK (status IN ('draft','confirmed','superseded','failed'))",
		"source_script_revision INT NOT NULL DEFAULT 0",
		"script_snapshot TEXT NOT NULL DEFAULT ''",
		"characters JSONB NOT NULL DEFAULT '[]'::jsonb",
		"scenes JSONB NOT NULL DEFAULT '[]'::jsonb",
		"props JSONB NOT NULL DEFAULT '[]'::jsonb",
		"outfits JSONB NOT NULL DEFAULT '[]'::jsonb",
		"styles JSONB NOT NULL DEFAULT '[]'::jsonb",
		"story_beats JSONB NOT NULL DEFAULT '[]'::jsonb",
		"raw_result TEXT NOT NULL DEFAULT ''",
		"error_message TEXT NOT NULL DEFAULT ''",
		"CREATE UNIQUE INDEX IF NOT EXISTS uq_video_project_breakdowns_version ON video_project_breakdowns(project_id, version);",
		"CREATE UNIQUE INDEX IF NOT EXISTS uq_video_project_breakdowns_confirmed ON video_project_breakdowns(project_id) WHERE status='confirmed';",
	})

	for _, table := range []string{"video_project_characters", "video_project_scenes"} {
		assertSchemaFragments(t, schema, []string{
			"ALTER TABLE " + table + " ADD COLUMN IF NOT EXISTS visual_prompt TEXT NOT NULL DEFAULT '';",
			"ALTER TABLE " + table + " ADD COLUMN IF NOT EXISTS source TEXT NOT NULL DEFAULT 'legacy' CHECK (source IN ('ai','manual','library','legacy'));",
			"ALTER TABLE " + table + " ADD COLUMN IF NOT EXISTS status TEXT NOT NULL DEFAULT 'draft' CHECK (status IN ('draft','confirmed','generating','ready','failed','detached'));",
			"ALTER TABLE " + table + " ADD COLUMN IF NOT EXISTS required BOOLEAN NOT NULL DEFAULT false;",
			"ALTER TABLE " + table + " ADD COLUMN IF NOT EXISTS breakdown_item_key TEXT NOT NULL DEFAULT '';",
			"ALTER TABLE " + table + " ADD COLUMN IF NOT EXISTS source_breakdown_id BIGINT REFERENCES video_project_breakdowns(id) ON DELETE SET NULL;",
		})
	}
	assertSchemaFragments(t, schema, []string{
		"CREATE UNIQUE INDEX IF NOT EXISTS uq_video_project_characters_breakdown_key ON video_project_characters(project_id, breakdown_item_key) WHERE breakdown_item_key<>'';",
		"CREATE UNIQUE INDEX IF NOT EXISTS uq_video_project_scenes_breakdown_key ON video_project_scenes(project_id, breakdown_item_key) WHERE breakdown_item_key<>'';",
	})

	assertSchemaFragments(t, schema, []string{
		"CREATE TABLE IF NOT EXISTS video_project_assets (",
		"type TEXT NOT NULL CHECK (type IN ('prop','outfit','style'))",
		"breakdown_item_key TEXT NOT NULL DEFAULT ''",
		"source_breakdown_id BIGINT REFERENCES video_project_breakdowns(id) ON DELETE SET NULL",
		"visual_prompt TEXT NOT NULL DEFAULT ''",
		"usage_note TEXT NOT NULL DEFAULT ''",
		"required BOOLEAN NOT NULL DEFAULT false",
		"global_asset_id BIGINT REFERENCES video_assets(id) ON DELETE SET NULL",
		"reference_image_url TEXT NOT NULL DEFAULT ''",
		"source TEXT NOT NULL DEFAULT 'manual' CHECK (source IN ('ai','manual','library','legacy'))",
		"status TEXT NOT NULL DEFAULT 'draft' CHECK (status IN ('draft','confirmed','generating','ready','failed','detached'))",
		"metadata JSONB NOT NULL DEFAULT '{}'::jsonb",
		"CREATE UNIQUE INDEX IF NOT EXISTS uq_video_project_assets_breakdown_key ON video_project_assets(project_id, breakdown_item_key) WHERE breakdown_item_key<>'';",
	})

	assertSchemaFragments(t, schema, []string{
		"CREATE TABLE IF NOT EXISTS video_project_asset_candidates (",
		"target_type TEXT NOT NULL CHECK (target_type IN ('character','scene','prop','outfit','style'))",
		"target_id BIGINT NOT NULL",
		"prompt TEXT NOT NULL DEFAULT ''",
		"image_asset_id BIGINT REFERENCES upload_assets(id) ON DELETE SET NULL",
		"image_url TEXT NOT NULL DEFAULT ''",
		"source TEXT NOT NULL DEFAULT 'generated' CHECK (source IN ('generated','upload','library','legacy'))",
		"generation_request_id TEXT NOT NULL DEFAULT ''",
		"status TEXT NOT NULL DEFAULT 'queued' CHECK (status IN ('queued','generating','ready','failed'))",
		"selected BOOLEAN NOT NULL DEFAULT false",
		"CREATE UNIQUE INDEX IF NOT EXISTS uq_video_project_asset_candidates_selected ON video_project_asset_candidates(target_type, target_id) WHERE selected=true;",
	})

	assertSchemaFragments(t, schema, []string{
		"CREATE TABLE IF NOT EXISTS video_project_storyboard_versions (",
		"version INT NOT NULL CHECK (version > 0)",
		"revision INT NOT NULL DEFAULT 1 CHECK (revision > 0)",
		"status TEXT NOT NULL DEFAULT 'draft' CHECK (status IN ('draft','confirmed','superseded','failed'))",
		"source_script_revision INT NOT NULL DEFAULT 0",
		"source_breakdown_id BIGINT REFERENCES video_project_breakdowns(id) ON DELETE SET NULL",
		"source_asset_revision INT NOT NULL DEFAULT 0",
		"source_capability_version TEXT NOT NULL DEFAULT ''",
		"baseline_storyboard_id BIGINT REFERENCES video_project_storyboard_versions(id) ON DELETE SET NULL",
		"shots JSONB NOT NULL DEFAULT '[]'::jsonb",
		"CREATE UNIQUE INDEX IF NOT EXISTS uq_video_project_storyboard_versions_version ON video_project_storyboard_versions(project_id, version);",
		"CREATE UNIQUE INDEX IF NOT EXISTS uq_video_project_storyboard_versions_confirmed ON video_project_storyboard_versions(project_id) WHERE status='confirmed';",
	})

	assertSchemaFragments(t, schema, []string{
		"ALTER TABLE video_shots ADD COLUMN IF NOT EXISTS generation_mode TEXT NOT NULL DEFAULT 'reference' CHECK (generation_mode IN ('reference','edit','extend'));",
		"ALTER TABLE video_shots ADD COLUMN IF NOT EXISTS prompt_override TEXT NOT NULL DEFAULT '';",
		"ALTER TABLE video_shots ADD COLUMN IF NOT EXISTS prompt_version TEXT NOT NULL DEFAULT '';",
		"ALTER TABLE video_shots ADD COLUMN IF NOT EXISTS audio_mode TEXT NOT NULL DEFAULT '';",
		"ALTER TABLE video_shots ADD COLUMN IF NOT EXISTS prompt_diagnostics JSONB NOT NULL DEFAULT '{}'::jsonb;",
		"ALTER TABLE video_shots ADD COLUMN IF NOT EXISTS source_key TEXT NOT NULL DEFAULT '';",
		"ALTER TABLE video_shots ADD COLUMN IF NOT EXISTS archived_at TIMESTAMPTZ;",
		"ALTER TABLE video_shots ADD COLUMN IF NOT EXISTS selected_generation_id BIGINT REFERENCES video_generations(id) ON DELETE SET NULL;",
		"ALTER TABLE video_shots ADD COLUMN IF NOT EXISTS selected_generation_ack_hash TEXT NOT NULL DEFAULT '';",
		"CREATE UNIQUE INDEX IF NOT EXISTS uq_video_shots_source_key ON video_shots(project_id, source_key) WHERE source_key<>'' AND archived_at IS NULL;",
		"ALTER TABLE video_compose_jobs ADD COLUMN IF NOT EXISTS compose_input_hash TEXT NOT NULL DEFAULT '';",
		"CREATE INDEX IF NOT EXISTS idx_video_shot_assets_order ON video_shot_assets(shot_id, sort_order, id);",
	})
}

func normalizeSchemaSQL(raw string) string {
	return strings.Join(strings.Fields(raw), " ")
}

func assertSchemaFragments(t *testing.T, schema string, fragments []string) {
	t.Helper()
	for _, fragment := range fragments {
		normalized := normalizeSchemaSQL(fragment)
		if !strings.Contains(schema, normalized) {
			t.Fatalf("schema missing %q", fragment)
		}
	}
}
