package engagement

import (
	"context"
	"database/sql"
	"net/url"
	"os"
	"strings"
	"testing"
	"time"

	_ "github.com/jackc/pgx/v5/stdlib"

	"nine-xing/nx-backend/apps/server/internal/testutil"
)

func TestMessageSummaryRedactsIdentifiers(t *testing.T) {
	got := messageSummary(`openid="secret-openid" unionid=secret-unionid 手机号:13800138000`)
	for _, forbidden := range []string{"secret-openid", "secret-unionid", "13800138000"} {
		if strings.Contains(got, forbidden) {
			t.Fatalf("summary leaked %q: %q", forbidden, got)
		}
	}
}

func TestParsePositiveMessageIDRejectsUnsafeValues(t *testing.T) {
	for _, value := range []string{"", "0", "-1", "1.0", "9223372036854775808"} {
		if _, err := parsePositiveDecimalID(value); err == nil {
			t.Fatalf("expected %q to be rejected", value)
		}
	}
}

func TestBuildMessageWhereFiltersByBusinessTypeWithoutReplacingType(t *testing.T) {
	condition, args, err := buildMessageWhere(url.Values{
		"businessType": {"signup"},
		"type":         {"miniapp"},
	})
	if err != nil {
		t.Fatalf("buildMessageWhere returned error: %v", err)
	}
	if condition != "1=1 AND type=$1 AND business_type=$2" {
		t.Fatalf("condition = %q", condition)
	}
	if len(args) != 2 || args[0] != "miniapp" || args[1] != "signup" {
		t.Fatalf("args = %#v", args)
	}
}

func TestGameOverviewFallsBackToCenterNameFromKey(t *testing.T) {
	dsn := os.Getenv("TEST_DATABASE_URL")
	if dsn == "" {
		t.Skip("set TEST_DATABASE_URL to run engagement database integration tests")
	}
	if err := testutil.ValidateIsolatedPostgresDSN(dsn); err != nil {
		t.Fatal(err)
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
