package db

import (
	"os"
	"strings"
	"testing"
)

func TestSchemaDefinesClassroomPersistenceBoundaries(t *testing.T) {
	raw, err := os.ReadFile("schema.sql")
	if err != nil {
		t.Fatal(err)
	}
	sql := string(raw)
	for _, table := range []string{"classroom_series", "classroom_contents", "classroom_media_assets", "classroom_upload_tasks", "classroom_entitlements", "classroom_progress"} {
		if !strings.Contains(sql, "CREATE TABLE IF NOT EXISTS "+table) {
			t.Errorf("schema is missing %s", table)
		}
	}
	for _, fragment := range []string{
		"show_as_standalone BOOLEAN NOT NULL DEFAULT false",
		"access_level TEXT NOT NULL DEFAULT 'inherit'",
		"playback_blocked BOOLEAN NOT NULL DEFAULT false",
		"object_key TEXT NOT NULL",
		"checksum TEXT NOT NULL DEFAULT ''",
		"UNIQUE (content_id)",
		"CHECK ((series_id IS NOT NULL)::int + (content_id IS NOT NULL)::int = 1)",
		"CHECK (access_level = 'paid' AND price_cents > 0 OR access_level <> 'paid' AND price_cents = 0)",
	} {
		if !strings.Contains(sql, fragment) {
			t.Errorf("schema is missing classroom constraint %q", fragment)
		}
	}
	if strings.Contains(extractClassroomCreateTable(sql, "classroom_contents"), "teacher_id") || strings.Contains(extractClassroomCreateTable(sql, "classroom_series"), "teacher_id") {
		t.Fatal("first classroom migration must use teacher_key snapshots, not teacher_id")
	}
	if strings.Contains(extractClassroomCreateTable(sql, "classroom_media_assets"), "upload_assets") || strings.Contains(extractClassroomCreateTable(sql, "classroom_media_assets"), " BYTEA") {
		t.Fatal("long classroom media must store object metadata, not upload_assets bytes")
	}
}

func TestSchemaOrdersClassroomForeignKeysAfterDependencies(t *testing.T) {
	raw, err := os.ReadFile("schema.sql")
	if err != nil {
		t.Fatal(err)
	}
	sql := string(raw)
	series := strings.Index(sql, "CREATE TABLE IF NOT EXISTS classroom_series")
	contents := strings.Index(sql, "CREATE TABLE IF NOT EXISTS classroom_contents")
	media := strings.Index(sql, "CREATE TABLE IF NOT EXISTS classroom_media_assets")
	uploads := strings.Index(sql, "CREATE TABLE IF NOT EXISTS classroom_upload_tasks")
	if series < 0 || contents < series || media < contents || uploads < media {
		t.Fatalf("classroom dependency order is invalid: series=%d contents=%d media=%d uploads=%d", series, contents, media, uploads)
	}
}

func extractClassroomCreateTable(sql, table string) string {
	start := strings.Index(sql, "CREATE TABLE IF NOT EXISTS "+table)
	if start < 0 {
		return ""
	}
	rest := sql[start:]
	end := strings.Index(rest, ");")
	if end < 0 {
		return rest
	}
	return rest[:end+2]
}

func TestClassroomEntitlementSchemaAllowsRenewalAfterExpiry(t *testing.T) {
	raw, err := os.ReadFile("schema.sql")
	if err != nil {
		t.Fatal(err)
	}
	sql := string(raw)
	if strings.Contains(sql, "uq_classroom_entitlement_active_series") || strings.Contains(sql, "uq_classroom_entitlement_active_content") {
		t.Fatal("partial unique indexes permanently block renewal after an entitlement expires; renewal idempotency belongs in the transactional entitlement service")
	}
}
