package db

import (
	"context"
	"database/sql"
	"os"
	"testing"
	"time"

	"nine-xing/nx-backend/apps/server/internal/testutil"
)

func TestProblemFollowupPlanMigrationPreservesConfigurationPostgres(t *testing.T) {
	dsn := os.Getenv("TEST_DATABASE_URL")
	if dsn == "" {
		t.Skip("set TEST_DATABASE_URL for isolated plan migration test")
	}
	if err := testutil.ValidateIsolatedPostgresDSN(dsn); err != nil {
		t.Fatal(err)
	}
	database, err := sql.Open("pgx", dsn)
	if err != nil {
		t.Fatal(err)
	}
	defer database.Close()
	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
	defer cancel()
	tx, err := database.BeginTx(ctx, nil)
	if err != nil {
		t.Fatal(err)
	}
	defer tx.Rollback()
	// A transaction-local temporary table shadows app_plans. No application
	// schema, existing data, or full seed/migration routine is touched.
	_, err = tx.ExecContext(ctx, `CREATE TEMP TABLE app_plans (
		code text PRIMARY KEY, plan_level text NOT NULL, feature_flags jsonb NOT NULL,
		features jsonb NOT NULL, price_cents int NOT NULL DEFAULT 12345,
		limits jsonb NOT NULL DEFAULT '{"customLimit":17}', update_time timestamptz NOT NULL DEFAULT now()
	) ON COMMIT DROP;
	INSERT INTO app_plans (code,plan_level,feature_flags,features) VALUES
	('svip','svip','{"xinzhili":false}','["我的自定义权益"]'),
	('svip_month','svip','{}','["我的月卡权益"]'),
	('svip_year','svip','{"problemFollowup":false}','["关闭跟进的自定义权益"]'),
	('svip_quarter','svip','{}','["一","二","三","四","五","六","七","八"]'),
	('vip','vip','{}','["VIP自定义权益"]'),
	('free','free','{}','[]');`)
	if err != nil {
		t.Fatal(err)
	}
	migration, err := os.ReadFile("problem_followup_plan_migration.sql")
	if err != nil {
		t.Fatal(err)
	}
	var previous string
	for iteration := 0; iteration < 2; iteration++ {
		if _, err := tx.ExecContext(ctx, string(migration)); err != nil {
			t.Fatal(err)
		}
		var snapshot string
		if err := tx.QueryRowContext(ctx, `SELECT jsonb_agg(to_jsonb(p) ORDER BY code)::text FROM app_plans p`).Scan(&snapshot); err != nil {
			t.Fatal(err)
		}
		if iteration > 0 && snapshot != previous {
			t.Fatal("migration replay changed previously migrated data")
		}
		previous = snapshot
	}
	var correct bool
	err = tx.QueryRowContext(ctx, `SELECT
		(SELECT feature_flags->'problemFollowup' = 'true' AND feature_flags->'xinzhili' = 'false' AND features->>0 = '我的自定义权益' AND jsonb_array_length(features)=2 FROM app_plans WHERE code='svip')
		AND (SELECT feature_flags->'problemFollowup' = 'false' AND jsonb_array_length(features)=1 FROM app_plans WHERE code='svip_year')
		AND (SELECT jsonb_array_length(features)=8 FROM app_plans WHERE code='svip_quarter')
		AND (SELECT feature_flags->'problemFollowup' = 'false' FROM app_plans WHERE code='vip')
		AND (SELECT feature_flags->'problemFollowup' = 'false' FROM app_plans WHERE code='free')
		AND (SELECT bool_and(price_cents=12345 AND limits='{"customLimit":17}'::jsonb) FROM app_plans)
	`).Scan(&correct)
	if err != nil || !correct {
		t.Fatalf("migration policy incorrect: correct=%v err=%v", correct, err)
	}
	// A pre-existing canonical false must not be re-enabled by a subsequent deploy.
	if _, err := tx.ExecContext(ctx, `UPDATE app_plans SET feature_flags='{"problemFollowup":false}',features='["保留文案"]' WHERE code='svip'`); err != nil {
		t.Fatal(err)
	}
	if _, err := tx.ExecContext(ctx, string(migration)); err != nil {
		t.Fatal(err)
	}
	if err := tx.QueryRowContext(ctx, `SELECT feature_flags->'problemFollowup' = 'false' AND features='["保留文案"]'::jsonb FROM app_plans WHERE code='svip'`).Scan(&correct); err != nil || !correct {
		t.Fatalf("canonical false overwritten: correct=%v err=%v", correct, err)
	}
}
