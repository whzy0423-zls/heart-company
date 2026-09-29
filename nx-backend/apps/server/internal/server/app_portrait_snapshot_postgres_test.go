package server

import (
	"context"
	"os"
	"testing"

	"nine-xing/nx-backend/apps/server/internal/quiz"
	"nine-xing/nx-backend/apps/server/internal/testdb"
)

func TestPortraitSnapshotPostgresPersistsGeneratedAndNextTimes(t *testing.T) {
	db, _ := testdb.OpenEnvIsolatedSchema(t, "portrait_snapshot")
	schema, err := os.ReadFile("../db/schema.sql")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := db.Exec(string(schema)); err != nil {
		t.Fatalf("initialize schema: %v", err)
	}
	if _, err := db.Exec(`
		INSERT INTO app_users(id, phone) VALUES (8801, 'portrait-snapshot-8801');
		INSERT INTO app_user_cards(id, app_user_id, card_type, name, relation, enneagram)
		VALUES (8801, 8801, 'primary', '本人', 'self', 1);
	`); err != nil {
		t.Fatal(err)
	}

	server := &Server{db: db}
	card := quiz.Card{ID: 8801, MainType: 1, UpdateTime: "2026/09/01 10:00:00"}
	first, err := server.refreshPortraitSnapshot(context.Background(), 8801, card)
	if err != nil {
		t.Fatalf("refresh portrait: %v", err)
	}
	if first.UpdatedAt == "" || first.NextUpdateAt == "" {
		t.Fatalf("snapshot times missing: %+v", first)
	}

	second, err := server.portraitForCard(context.Background(), 8801, card)
	if err != nil {
		t.Fatalf("read portrait: %v", err)
	}
	if second.UpdatedAt != first.UpdatedAt || second.NextUpdateAt != first.NextUpdateAt {
		t.Fatalf("snapshot was not reused: first=%+v second=%+v", first, second)
	}

	var count int
	if err := db.QueryRow(`SELECT count(*) FROM app_growth_portrait_snapshots WHERE card_id=8801 AND app_user_id=8801`).Scan(&count); err != nil {
		t.Fatal(err)
	}
	if count != 1 {
		t.Fatalf("snapshot count=%d, want 1", count)
	}
}
