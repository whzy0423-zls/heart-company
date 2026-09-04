package db

import (
	"os"
	"strings"
	"testing"
)

func TestSchemaIncludesRealtimeTicketContract(t *testing.T) {
	raw, err := os.ReadFile("schema.sql")
	if err != nil {
		t.Fatal(err)
	}
	schema := strings.Join(strings.Fields(string(raw)), " ")
	for _, fragment := range []string{"CREATE TABLE IF NOT EXISTS direct_realtime_tickets", "token_hash TEXT NOT NULL UNIQUE", "expires_at TIMESTAMPTZ NOT NULL", "consumed_at TIMESTAMPTZ", "idx_direct_realtime_tickets_active", "WHERE consumed_at IS NULL"} {
		if !strings.Contains(schema, fragment) {
			t.Fatalf("schema missing realtime ticket contract %q", fragment)
		}
	}
}
