package db

import (
	"os"
	"strings"
	"testing"
)

func TestVoiceProfilesPersistSynthesisModelAndMigrateLegacyBailianRows(t *testing.T) {
	raw, err := os.ReadFile("schema.sql")
	if err != nil {
		t.Fatal(err)
	}
	schema := string(raw)
	for _, expected := range []string{
		"model           TEXT NOT NULL DEFAULT ''",
		"ALTER TABLE voice_profiles ADD COLUMN IF NOT EXISTS model TEXT NOT NULL DEFAULT ''",
		"MiniMax/speech-2.8-turbo",
	} {
		if !strings.Contains(schema, expected) {
			t.Fatalf("schema missing %q", expected)
		}
	}
}
