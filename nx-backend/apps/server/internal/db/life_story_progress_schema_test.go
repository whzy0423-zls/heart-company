package db

import (
	"os"
	"strings"
	"testing"
)

func TestLifeStoryProgressSchemaTracksClientWriteTime(t *testing.T) {
	raw, err := os.ReadFile("schema.sql")
	if err != nil {
		t.Fatal(err)
	}
	schema := string(raw)
	for _, want := range []string{
		"client_updated_at TIMESTAMPTZ",
		"ALTER TABLE app_life_story_progress ADD COLUMN IF NOT EXISTS client_updated_at",
	} {
		if !strings.Contains(schema, want) {
			t.Fatalf("life-story progress schema missing %q", want)
		}
	}
}
