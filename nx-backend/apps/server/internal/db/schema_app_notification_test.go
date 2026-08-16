package db

import (
	"os"
	"strings"
	"testing"
)

func TestAppNotificationSchema(t *testing.T) {
	raw, err := os.ReadFile("schema.sql")
	if err != nil {
		t.Fatal(err)
	}
	schema := string(raw)
	for _, contract := range []string{
		"CREATE TABLE IF NOT EXISTS app_notifications",
		"app_user_id BIGINT NOT NULL REFERENCES app_users(id) ON DELETE CASCADE",
		"source_key TEXT NOT NULL DEFAULT ''",
		"read_time     TIMESTAMPTZ",
		"idx_app_notifications_user_timeline",
		"idx_app_notifications_user_unread",
		"idx_app_notifications_user_source",
		"WHERE source_key <> ''",
	} {
		if !strings.Contains(schema, contract) {
			t.Fatalf("notification schema missing contract %q", contract)
		}
	}
}
