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

func TestSchemaClassroomContentsPersistCoverOwnershipAndAspectRatio(t *testing.T) {
	raw, err := os.ReadFile("schema.sql")
	if err != nil {
		t.Fatal(err)
	}
	sql := string(raw)
	contents := extractClassroomCreateTable(sql, "classroom_contents")
	for _, fragment := range []string{
		"manual_cover_object_key TEXT NOT NULL DEFAULT ''",
		"cover_aspect_ratio TEXT NOT NULL DEFAULT '16:9'",
		"CHECK (cover_aspect_ratio IN ('16:9','9:16','1:1'))",
	} {
		if !strings.Contains(contents, fragment) {
			t.Errorf("classroom_contents is missing cover setting %q", fragment)
		}
	}
	for _, migration := range []string{
		"ALTER TABLE classroom_contents ADD COLUMN IF NOT EXISTS manual_cover_object_key TEXT NOT NULL DEFAULT ''",
		"ALTER TABLE classroom_contents ADD COLUMN IF NOT EXISTS cover_aspect_ratio TEXT NOT NULL DEFAULT '16:9'",
	} {
		if !strings.Contains(sql, migration) {
			t.Errorf("schema is missing idempotent cover migration %q", migration)
		}
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
	if strings.Contains(sql, "CREATE UNIQUE INDEX IF NOT EXISTS uq_classroom_entitlement_active_series") || strings.Contains(sql, "CREATE UNIQUE INDEX IF NOT EXISTS uq_classroom_entitlement_active_content") {
		t.Fatal("partial unique indexes permanently block renewal after an entitlement expires; renewal idempotency belongs in the transactional entitlement service")
	}
}

func TestClassroomEntitlementSchemaDropsLegacyRenewalBlockingIndexes(t *testing.T) {
	raw, err := os.ReadFile("schema.sql")
	if err != nil {
		t.Fatal(err)
	}
	sql := string(raw)
	for _, statement := range []string{
		"DROP INDEX IF EXISTS uq_classroom_entitlement_active_series",
		"DROP INDEX IF EXISTS uq_classroom_entitlement_active_content",
	} {
		if !strings.Contains(sql, statement) {
			t.Fatalf("schema is missing legacy entitlement index cleanup %q", statement)
		}
	}
}

func TestClassroomEntitlementsHaveOrderIdempotencyIndex(t *testing.T) {
	raw, err := os.ReadFile("schema.sql")
	if err != nil {
		t.Fatal(err)
	}
	sql := string(raw)
	statement := "CREATE UNIQUE INDEX IF NOT EXISTS uq_classroom_entitlements_order ON classroom_entitlements(order_id) WHERE order_id IS NOT NULL"
	if !strings.Contains(sql, statement) {
		t.Fatalf("schema is missing order entitlement idempotency index %q", statement)
	}
}

func TestSchemaClassroomUploadCleanupStatusUsesCleanedValue(t *testing.T) {
	raw, err := os.ReadFile("schema.sql")
	if err != nil {
		t.Fatal(err)
	}
	fragment := extractClassroomCreateTable(string(raw), "classroom_upload_tasks")
	if !strings.Contains(fragment, "'cleaned'") || strings.Contains(fragment, "'clean'\")") {
		t.Fatalf("cleanup status constraint must use cleaned: %s", fragment)
	}
}

func TestSchemaClassroomUploadSupportsCompletionClaimState(t *testing.T) {
	raw, err := os.ReadFile("schema.sql")
	if err != nil {
		t.Fatal(err)
	}
	fragment := extractClassroomCreateTable(string(raw), "classroom_upload_tasks")
	if !strings.Contains(fragment, "'completing'") {
		t.Fatalf("missing completing claim state: %s", fragment)
	}
}

func TestSchemaClassroomUploadSupportsCleanupClaimState(t *testing.T) {
	raw, err := os.ReadFile("schema.sql")
	if err != nil {
		t.Fatal(err)
	}
	fragment := extractClassroomCreateTable(string(raw), "classroom_upload_tasks")
	if !strings.Contains(fragment, "'cleaning'") {
		t.Fatalf("missing cleaning claim state: %s", fragment)
	}
}

func TestSchemaClassroomUploadSupportsInitiatingReservationState(t *testing.T) {
	raw, err := os.ReadFile("schema.sql")
	if err != nil {
		t.Fatal(err)
	}
	fragment := extractClassroomCreateTable(string(raw), "classroom_upload_tasks")
	if !strings.Contains(fragment, "'initiating'") {
		t.Fatalf("missing initiating reservation state: %s", fragment)
	}
}

func TestSchemaClassroomUploadPersistsRetryIdentityAndProgress(t *testing.T) {
	raw, err := os.ReadFile("schema.sql")
	if err != nil {
		t.Fatal(err)
	}
	fragment := extractClassroomCreateTable(string(raw), "classroom_upload_tasks")
	for _, column := range []string{"original_filename", "completed_parts", "completed_bytes"} {
		if !strings.Contains(fragment, column) {
			t.Errorf("missing upload task column %q", column)
		}
	}
	for _, migration := range []string{"ADD COLUMN IF NOT EXISTS original_filename", "ADD COLUMN IF NOT EXISTS completed_parts", "ADD COLUMN IF NOT EXISTS completed_bytes"} {
		if !strings.Contains(string(raw), migration) {
			t.Errorf("missing upgrade migration %q", migration)
		}
	}
}

func TestSchemaMigratesExistingClassroomUploadStatusConstraint(t *testing.T) {
	raw, err := os.ReadFile("schema.sql")
	if err != nil {
		t.Fatal(err)
	}
	sql := string(raw)
	for _, fragment := range []string{"DROP CONSTRAINT IF EXISTS classroom_upload_tasks_status_check", "ADD CONSTRAINT classroom_upload_tasks_status_check", "'initiating'", "'completing'", "'cleaning'"} {
		if !strings.Contains(sql, fragment) {
			t.Errorf("missing migration fragment %q", fragment)
		}
	}
}
