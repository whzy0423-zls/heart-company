package db

import (
	"context"
	"database/sql"
	"fmt"
	"os"
	"strings"
	"testing"
	"time"
)

func TestSchemaIncludesIdempotentSuggestionTurnAssociation(t *testing.T) {
	raw, err := os.ReadFile("schema.sql")
	if err != nil {
		t.Fatal(err)
	}
	schema := strings.Join(strings.Fields(string(raw)), " ")
	required := "ALTER TABLE app_chat_messages ADD COLUMN IF NOT EXISTS reply_to_message_id BIGINT REFERENCES app_chat_messages(id) ON DELETE SET NULL"
	if !strings.Contains(schema, required) {
		t.Fatalf("schema missing explicit turn association %q", required)
	}
	if !strings.Contains(schema, "CREATE INDEX IF NOT EXISTS idx_app_chat_messages_reply_to ON app_chat_messages(reply_to_message_id) WHERE reply_to_message_id IS NOT NULL") {
		t.Fatal("schema missing partial index for turn association foreign key")
	}
}

func TestSuggestionTurnSchemaMigratesCleanAndLegacyIdempotently(t *testing.T) {
	dsn := strings.TrimSpace(os.Getenv("TEST_DATABASE_URL"))
	if dsn == "" {
		t.Skip("set TEST_DATABASE_URL to run suggestion turn migration tests")
	}
	if !strings.Contains(strings.ToLower(dsn), "test") || (!strings.Contains(dsn, "127.0.0.1") && !strings.Contains(dsn, "localhost")) {
		t.Fatal("TEST_DATABASE_URL must be a loopback isolated test database")
	}
	db, err := sql.Open("pgx", dsn)
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	raw, err := os.ReadFile("schema.sql")
	if err != nil {
		t.Fatal(err)
	}
	schema := string(raw)
	start := strings.Index(schema, "CREATE TABLE IF NOT EXISTS app_chat_messages (")
	if start < 0 {
		t.Fatal("message migration block not found")
	}
	end := start + strings.Index(schema[start:], "\nDO $$")
	if start < 0 || end <= start {
		t.Fatal("message migration block not found")
	}
	for _, legacy := range []bool{false, true} {
		t.Run(fmt.Sprint("legacy=", legacy), func(t *testing.T) {
			ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
			defer cancel()
			conn, err := db.Conn(ctx)
			if err != nil {
				t.Fatal(err)
			}
			defer conn.Close()
			name := fmt.Sprintf("suggestion_turn_migration_%d", time.Now().UnixNano())
			if _, err = conn.ExecContext(ctx, `CREATE SCHEMA `+name+`;SET search_path TO `+name+`,public;CREATE TABLE app_chat_sessions(id BIGSERIAL PRIMARY KEY);CREATE TABLE upload_assets(id BIGSERIAL PRIMARY KEY);`); err != nil {
				t.Fatal(err)
			}
			t.Cleanup(func() { _, _ = db.Exec(`DROP SCHEMA IF EXISTS ` + name + ` CASCADE`) })
			if legacy {
				if _, err = conn.ExecContext(ctx, `CREATE TABLE app_chat_messages(id BIGSERIAL PRIMARY KEY,session_id BIGINT NOT NULL REFERENCES app_chat_sessions(id) ON DELETE CASCADE,role TEXT NOT NULL,content TEXT NOT NULL DEFAULT '',sources JSONB NOT NULL DEFAULT '[]',create_time TIMESTAMPTZ NOT NULL DEFAULT now());INSERT INTO app_chat_sessions(id) VALUES(1);INSERT INTO app_chat_messages(session_id,role,content) VALUES(1,'user','保留旧问题'),(1,'assistant','保留旧回答');`); err != nil {
					t.Fatal(err)
				}
			}
			for pass := 1; pass <= 2; pass++ {
				if _, err = conn.ExecContext(ctx, schema[start:end]); err != nil {
					t.Fatalf("migration pass %d: %v", pass, err)
				}
			}
			var exists bool
			if err = conn.QueryRowContext(ctx, `SELECT EXISTS(SELECT 1 FROM information_schema.columns WHERE table_schema=$1 AND table_name='app_chat_messages' AND column_name='reply_to_message_id' AND is_nullable='YES')`, name).Scan(&exists); err != nil || !exists {
				t.Fatalf("nullable association missing: exists=%v err=%v", exists, err)
			}
			if legacy {
				var content string
				var reply sql.NullInt64
				if err = conn.QueryRowContext(ctx, `SELECT content,reply_to_message_id FROM app_chat_messages WHERE role='assistant'`).Scan(&content, &reply); err != nil || content != "保留旧回答" || reply.Valid {
					t.Fatalf("legacy changed: content=%s reply=%v err=%v", content, reply, err)
				}
			}
		})
	}
}
