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
		"version_name TEXT NOT NULL",
		"version_code BIGINT NOT NULL CHECK (version_code > 0)",
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
	} {
		if !strings.Contains(schema, fragment) {
			t.Fatalf("expected app release schema to contain %q", fragment)
		}
	}
}
