package db

import (
	"strings"
	"testing"
)

func TestXinzhiliVoiceSchemaDefinesVersionedVoiceConfigTables(t *testing.T) {
	for _, want := range []string{
		"CREATE TABLE IF NOT EXISTS app_xinzhili_voice_configs",
		"version BIGINT NOT NULL CHECK (version > 0)",
		"status TEXT NOT NULL CHECK (status IN ('draft', 'active', 'inactive', 'archived'))",
		"api_key_ciphertext TEXT NOT NULL DEFAULT ''",
		"api_key_suffix TEXT NOT NULL DEFAULT ''",
		"config JSONB NOT NULL DEFAULT '{}'::jsonb",
		"CREATE UNIQUE INDEX IF NOT EXISTS idx_app_xinzhili_voice_configs_single_draft",
		"CREATE UNIQUE INDEX IF NOT EXISTS idx_app_xinzhili_voice_configs_single_active",
		"CREATE TABLE IF NOT EXISTS app_xinzhili_voice_cleanup_jobs",
		"remote_voice_id TEXT NOT NULL DEFAULT ''",
		"status TEXT NOT NULL CHECK (status IN ('pending', 'processing', 'done', 'failed'))",
	} {
		if !strings.Contains(schemaSQL, want) {
			t.Fatalf("schema missing %q", want)
		}
	}
}
