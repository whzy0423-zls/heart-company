package db

import (
	"context"
	"database/sql"
	"fmt"
	"os"
	"strings"
	"testing"
	"time"

	_ "github.com/jackc/pgx/v5/stdlib"
)

func TestXinzhiliSchemaMigratesCleanAndLegacyDatabasesIdempotently(t *testing.T) {
	dsn := strings.TrimSpace(os.Getenv("TEST_DATABASE_URL"))
	if dsn == "" {
		t.Skip("set TEST_DATABASE_URL to run xinzhili schema migration test")
	}
	if !strings.Contains(strings.ToLower(dsn), "test") || (!strings.Contains(dsn, "127.0.0.1") && !strings.Contains(dsn, "localhost")) {
		t.Fatal("TEST_DATABASE_URL must be a loopback isolated test database")
	}
	database, err := sql.Open("pgx", dsn)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = database.Close() })

	for _, legacy := range []bool{false, true} {
		name := "clean"
		if legacy {
			name = "legacy"
		}
		t.Run(name, func(t *testing.T) {
			ctx, cancel := context.WithTimeout(context.Background(), 45*time.Second)
			defer cancel()
			conn, err := database.Conn(ctx)
			if err != nil {
				t.Fatal(err)
			}
			defer conn.Close()
			schemaName := fmt.Sprintf("task2_xinzhili_%s_%d", name, time.Now().UnixNano())
			if _, err := conn.ExecContext(ctx, `CREATE SCHEMA `+schemaName); err != nil {
				t.Fatal(err)
			}
			t.Cleanup(func() { _, _ = database.Exec(`DROP SCHEMA IF EXISTS ` + schemaName + ` CASCADE`) })
			if _, err := conn.ExecContext(ctx, `SET search_path TO `+schemaName+`, public`); err != nil {
				t.Fatal(err)
			}

			if _, err := conn.ExecContext(ctx, `
				CREATE TABLE app_users(id BIGSERIAL PRIMARY KEY, phone TEXT NOT NULL DEFAULT '');
				CREATE TABLE app_user_cards(
					id BIGSERIAL PRIMARY KEY,
					app_user_id BIGINT NOT NULL REFERENCES app_users(id) ON DELETE CASCADE,
					card_type TEXT NOT NULL DEFAULT 'primary', name TEXT NOT NULL DEFAULT '',
					relation TEXT NOT NULL DEFAULT '', enneagram INT NOT NULL DEFAULT 0,
					wing INT NOT NULL DEFAULT 0, profile JSONB NOT NULL DEFAULT '{}',
					status TEXT NOT NULL DEFAULT 'active'
				);
				CREATE TABLE upload_assets(id BIGSERIAL PRIMARY KEY);
			`); err != nil {
				t.Fatalf("prerequisite schema: %v", err)
			}
			var legacySessionID int64
			if legacy {
				if _, err := conn.ExecContext(ctx, `
					CREATE TABLE app_chat_sessions (
						id BIGSERIAL PRIMARY KEY,
						app_user_id BIGINT NOT NULL REFERENCES app_users(id) ON DELETE CASCADE,
						card_id BIGINT NOT NULL REFERENCES app_user_cards(id) ON DELETE CASCADE,
						title TEXT NOT NULL DEFAULT '',
						updated_at TIMESTAMPTZ NOT NULL DEFAULT now(),
						create_time TIMESTAMPTZ NOT NULL DEFAULT now()
					);
					CREATE TABLE app_chat_messages (
						id BIGSERIAL PRIMARY KEY,
						session_id BIGINT NOT NULL REFERENCES app_chat_sessions(id) ON DELETE CASCADE,
						role TEXT NOT NULL, content TEXT NOT NULL DEFAULT '', sources JSONB NOT NULL DEFAULT '[]',
						create_time TIMESTAMPTZ NOT NULL DEFAULT now()
					);
				`); err != nil {
					t.Fatal(err)
				}
				var userID, cardID int64
				if err := conn.QueryRowContext(ctx, `INSERT INTO app_users(phone) VALUES ($1) RETURNING id`, fmt.Sprintf("task2-%d", time.Now().UnixNano())).Scan(&userID); err != nil {
					t.Fatal(err)
				}
				if err := conn.QueryRowContext(ctx, `INSERT INTO app_user_cards(app_user_id, card_type, name, relation, enneagram, wing, profile, status) VALUES ($1,'primary','测试卡','self',1,2,'{}','active') RETURNING id`, userID).Scan(&cardID); err != nil {
					t.Fatal(err)
				}
				if err := conn.QueryRowContext(ctx, `INSERT INTO app_chat_sessions(app_user_id, card_id) VALUES ($1,$2) RETURNING id`, userID, cardID).Scan(&legacySessionID); err != nil {
					t.Fatal(err)
				}
			}

			migration := xinzhiliChatMigrationSQL(t)
			for i := 0; i < 2; i++ {
				if _, err := conn.ExecContext(ctx, migration); err != nil {
					t.Fatalf("migration pass %d: %v", i+1, err)
				}
			}
			assertXinzhiliSchema(t, ctx, conn, legacySessionID)
		})
	}
}

func xinzhiliChatMigrationSQL(t *testing.T) string {
	t.Helper()
	start := strings.Index(schemaSQL, "-- ----- App 问答会话")
	end := strings.Index(schemaSQL[start:], "-- ----- App 专属记忆")
	if start < 0 || end < 0 {
		t.Fatal("chat migration section not found")
	}
	return schemaSQL[start : start+end]
}

func assertXinzhiliSchema(t *testing.T, ctx context.Context, conn *sql.Conn, legacySessionID int64) {
	t.Helper()
	if _, err := conn.ExecContext(ctx, `INSERT INTO app_users(phone) SELECT 'schema-assert' WHERE NOT EXISTS (SELECT 1 FROM app_users)`); err != nil {
		t.Fatal(err)
	}
	var sceneDefault string
	if err := conn.QueryRowContext(ctx, `SELECT column_default FROM information_schema.columns WHERE table_schema=current_schema() AND table_name='app_chat_sessions' AND column_name='scene'`).Scan(&sceneDefault); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(sceneDefault, "chat") {
		t.Fatalf("scene default=%q", sceneDefault)
	}
	if legacySessionID != 0 {
		var scene string
		if err := conn.QueryRowContext(ctx, `SELECT scene FROM app_chat_sessions WHERE id=$1`, legacySessionID).Scan(&scene); err != nil || scene != "chat" {
			t.Fatalf("legacy scene=%q err=%v", scene, err)
		}
	}
	for _, column := range []string{"delivery_status", "delivered_text", "xinzhili_mode"} {
		var nullable string
		if err := conn.QueryRowContext(ctx, `SELECT is_nullable FROM information_schema.columns WHERE table_schema=current_schema() AND table_name='app_chat_messages' AND column_name=$1`, column).Scan(&nullable); err != nil || nullable != "YES" {
			t.Fatalf("column %s nullable=%q err=%v", column, nullable, err)
		}
	}
	var indexDef string
	if err := conn.QueryRowContext(ctx, `SELECT indexdef FROM pg_indexes WHERE schemaname=current_schema() AND indexname='idx_app_chat_sessions_user_card_scene'`).Scan(&indexDef); err != nil {
		t.Fatal(err)
	}
	for _, part := range []string{"app_user_id", "card_id", "scene", "updated_at DESC"} {
		if !strings.Contains(indexDef, part) {
			t.Fatalf("index missing %q: %s", part, indexDef)
		}
	}
	if _, err := conn.ExecContext(ctx, `INSERT INTO app_xinzhili_mode_preferences(app_user_id, requested_mode, revision) SELECT id, 'invalid', 1 FROM app_users LIMIT 1`); err == nil {
		t.Fatal("invalid mode should violate CHECK")
	}
	if _, err := conn.ExecContext(ctx, `INSERT INTO app_xinzhili_mode_preferences(app_user_id, requested_mode, revision) SELECT id, 'normal', 0 FROM app_users LIMIT 1`); err == nil {
		t.Fatal("zero revision should violate CHECK")
	}
	var cascadeUserID int64
	if err := conn.QueryRowContext(ctx, `SELECT id FROM app_users ORDER BY id LIMIT 1`).Scan(&cascadeUserID); err != nil {
		t.Fatal(err)
	}
	if _, err := conn.ExecContext(ctx, `INSERT INTO app_xinzhili_mode_preferences(app_user_id, requested_mode, revision) VALUES ($1,'normal',1)`, cascadeUserID); err != nil {
		t.Fatal(err)
	}
	if _, err := conn.ExecContext(ctx, `DELETE FROM app_users WHERE id=$1`, cascadeUserID); err != nil {
		t.Fatal(err)
	}
	var preferenceCount int
	if err := conn.QueryRowContext(ctx, `SELECT count(*) FROM app_xinzhili_mode_preferences WHERE app_user_id=$1`, cascadeUserID).Scan(&preferenceCount); err != nil || preferenceCount != 0 {
		t.Fatalf("mode preference cascade count=%d err=%v", preferenceCount, err)
	}
}
