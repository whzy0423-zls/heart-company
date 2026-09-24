package db

import (
	"os"
	"strings"
	"testing"
)

func TestSchemaDefinesAppReleaseConstraints(t *testing.T) {
	raw, err := os.ReadFile("schema.sql")
	if err != nil {
		t.Fatal(err)
	}
	schema := string(raw)

	for _, fragment := range []string{
		"CREATE TABLE IF NOT EXISTS app_releases",
		"id BIGSERIAL PRIMARY KEY",
		"platform TEXT NOT NULL CHECK (platform IN ('android'))",
		"app_name TEXT NOT NULL DEFAULT ''",
		"package_name TEXT NOT NULL DEFAULT ''",
		"icon_path TEXT NOT NULL DEFAULT ''",
		"version_name TEXT NOT NULL",
		"version_code BIGINT NOT NULL CHECK (version_code > 0)",
		"min_supported_version_code BIGINT NOT NULL DEFAULT 0 CHECK (min_supported_version_code >= 0)",
		"force_update BOOLEAN NOT NULL DEFAULT false",
		"rollout_percentage INTEGER NOT NULL DEFAULT 100 CHECK (rollout_percentage BETWEEN 1 AND 100)",
		"release_notes TEXT NOT NULL DEFAULT ''",
		"file_name TEXT NOT NULL",
		"file_path TEXT NOT NULL",
		"file_size BIGINT NOT NULL CHECK (file_size > 0)",
		"sha256 TEXT NOT NULL",
		"status TEXT NOT NULL DEFAULT 'draft' CHECK (status IN ('draft', 'published', 'archived'))",
		"created_at TIMESTAMPTZ NOT NULL DEFAULT now()",
		"published_at TIMESTAMPTZ",
		"UNIQUE(platform, version_code)",
		"CREATE UNIQUE INDEX IF NOT EXISTS idx_app_releases_one_published_per_platform",
		"ON app_releases(platform)",
		"WHERE status = 'published'",
		"ALTER TABLE app_releases ADD COLUMN IF NOT EXISTS app_name TEXT NOT NULL DEFAULT ''",
		"ALTER TABLE app_releases ADD COLUMN IF NOT EXISTS package_name TEXT NOT NULL DEFAULT ''",
		"ALTER TABLE app_releases ADD COLUMN IF NOT EXISTS icon_path TEXT NOT NULL DEFAULT ''",
		"ALTER TABLE app_releases ADD COLUMN IF NOT EXISTS min_supported_version_code BIGINT NOT NULL DEFAULT 0 CHECK (min_supported_version_code >= 0)",
		"ALTER TABLE app_releases ADD COLUMN IF NOT EXISTS force_update BOOLEAN NOT NULL DEFAULT false",
		"ALTER TABLE app_releases ADD COLUMN IF NOT EXISTS rollout_percentage INTEGER NOT NULL DEFAULT 100 CHECK (rollout_percentage BETWEEN 1 AND 100)",
	} {
		if !strings.Contains(schema, fragment) {
			t.Fatalf("expected app release schema to contain %q", fragment)
		}
	}
}
