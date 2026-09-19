package db

import (
	"os"
	"strings"
	"testing"
)

func TestSchemaDefinesTeacherProfilesRolesAndReviewEvents(t *testing.T) {
	raw, err := os.ReadFile("schema.sql")
	if err != nil {
		t.Fatal(err)
	}
	sql := string(raw)
	for _, table := range []string{"teacher_profiles", "app_user_roles", "teacher_profile_drafts", "teacher_review_events"} {
		if !strings.Contains(sql, "CREATE TABLE IF NOT EXISTS "+table) {
			t.Errorf("schema missing %s", table)
		}
	}
	for _, fragment := range []string{
		"teacher_key TEXT NOT NULL UNIQUE",
		"role TEXT NOT NULL CHECK (role IN ('teacher','agent'))",
		"review_status TEXT NOT NULL DEFAULT 'draft'",
		"review_reason TEXT NOT NULL DEFAULT ''",
		"pending_review",
		"replaces_content_id",
	} {
		if !strings.Contains(sql, fragment) {
			t.Errorf("schema missing teacher fragment %q", fragment)
		}
	}
	if strings.Contains(sql, "teacher_id BIGINT") {
		t.Fatal("classroom persistence must retain teacher_key snapshots and not add teacher_id")
	}
}
