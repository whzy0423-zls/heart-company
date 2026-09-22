package db

import (
	"os"
	"strings"
	"testing"
)

func TestSchemaIncludesVoiceBroadcastConfigContract(t *testing.T) {
	raw, err := os.ReadFile("schema.sql")
	if err != nil {
		t.Fatal(err)
	}
	schema := string(raw)
	start := strings.Index(schema, "CREATE TABLE IF NOT EXISTS app_voice_broadcast_configs")
	if start < 0 {
		t.Fatal("schema is missing app_voice_broadcast_configs")
	}
	end := strings.Index(schema[start:], ");")
	if end < 0 {
		t.Fatal("voice broadcast table definition is not terminated")
	}
	table := strings.Join(strings.Fields(schema[start:start+end]), " ")
	for _, required := range []string{
		"id SMALLINT PRIMARY KEY CHECK (id = 1)",
		"version BIGINT NOT NULL CHECK (version > 0)",
		"enabled BOOLEAN NOT NULL DEFAULT false",
		"provider TEXT NOT NULL DEFAULT 'aliyun-bailian'",
		"model TEXT NOT NULL DEFAULT 'qwen3-tts-instruct-flash'",
		"default_voice TEXT NOT NULL DEFAULT 'Cherry'",
		"current_voice TEXT NOT NULL DEFAULT 'Cherry'",
		"api_key_ciphertext TEXT NOT NULL DEFAULT ''",
		"api_key_suffix TEXT NOT NULL DEFAULT ''",
		"health JSONB NOT NULL",
	} {
		if !strings.Contains(table, required) {
			t.Fatalf("voice broadcast schema missing %q", required)
		}
	}
}
