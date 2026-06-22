package engagement

import (
	"context"
	"database/sql"
	"os"
	"testing"
	"time"

	_ "github.com/jackc/pgx/v5/stdlib"
)

func TestGameOverviewFallsBackToCenterNameFromKey(t *testing.T) {
	dsn := os.Getenv("TEST_DATABASE_URL")
	if dsn == "" {
		t.Skip("set TEST_DATABASE_URL to run engagement database integration tests")
	}

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	database, err := sql.Open("pgx", dsn)
	if err != nil {
		t.Fatalf("open db: %v", err)
	}
	t.Cleanup(func() { _ = database.Close() })

	var id int64
	if err := database.QueryRowContext(ctx, `
		INSERT INTO game_results (visitor_id, gender, result_type, second_type, score, centers)
		VALUES ('test-missing-center-name', 'male', 1, 2, '{}'::jsonb, '[{"key":"gut","pct":60}]'::jsonb)
		RETURNING id`).Scan(&id); err != nil {
		t.Fatalf("insert game result: %v", err)
	}
	t.Cleanup(func() {
		cleanupCtx, cleanupCancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cleanupCancel()
		_, _ = database.ExecContext(cleanupCtx, `DELETE FROM game_results WHERE id=$1`, id)
	})

	overview, err := NewStore(database).GameOverview(ctx)
	if err != nil {
		t.Fatalf("GameOverview returned error: %v", err)
	}
	found := false
	for _, item := range overview.CenterItems {
		if item.Name == "本能中心" {
			found = true
			if item.Value < 1 {
				t.Fatalf("expected center count to include fallback item: %+v", item)
			}
		}
	}
	if !found {
		t.Fatalf("expected missing center name to fall back from key: %+v", overview.CenterItems)
	}
}
