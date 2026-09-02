package db

import (
	"context"
	"database/sql"
	"fmt"
	"os"
	"strings"
	"testing"
	"time"

	"nine-xing/nx-backend/apps/server/internal/testutil"

	_ "github.com/jackc/pgx/v5/stdlib"
)

func TestSchemaDoesNotAlterAppChatMessagesBeforeCreate(t *testing.T) {
	raw, err := os.ReadFile("schema.sql")
	if err != nil {
		t.Fatal(err)
	}
	sql := string(raw)

	createIndex := strings.Index(sql, "CREATE TABLE IF NOT EXISTS app_chat_messages")
	if createIndex < 0 {
		t.Fatal("schema is missing app_chat_messages CREATE TABLE")
	}

	for _, statement := range []string{
		"ALTER TABLE app_chat_messages ADD COLUMN IF NOT EXISTS favorite",
		"ALTER TABLE app_chat_messages ADD COLUMN IF NOT EXISTS feedback",
	} {
		alterIndex := strings.Index(sql, statement)
		if alterIndex < 0 {
			continue
		}
		if alterIndex < createIndex {
			t.Fatalf("%q appears before app_chat_messages is created", statement)
		}
	}
}

func TestSchemaMigratesExistingQuizSubmissionWingType(t *testing.T) {
	raw, err := os.ReadFile("schema.sql")
	if err != nil {
		t.Fatal(err)
	}
	sql := string(raw)
	statement := "ALTER TABLE app_quiz_submissions ADD COLUMN IF NOT EXISTS wing_type"
	if !strings.Contains(sql, statement) {
		t.Fatalf("schema is missing old-database migration %q", statement)
	}
}

func TestSchemaMigratesChatSessionContextSummary(t *testing.T) {
	raw, err := os.ReadFile("schema.sql")
	if err != nil {
		t.Fatal(err)
	}
	sql := string(raw)
	for _, statement := range []string{
		"ALTER TABLE app_chat_sessions ADD COLUMN IF NOT EXISTS context_summary",
		"ALTER TABLE app_chat_sessions ADD COLUMN IF NOT EXISTS context_summary_through_message_id",
	} {
		if !strings.Contains(sql, statement) {
			t.Fatalf("schema is missing old-database migration %q", statement)
		}
	}
}

func TestSchemaMigratesChatSessionScene(t *testing.T) {
	raw, err := os.ReadFile("schema.sql")
	if err != nil {
		t.Fatal(err)
	}
	sql := string(raw)
	for _, statement := range []string{
		"ALTER TABLE app_chat_sessions ADD COLUMN IF NOT EXISTS scene",
		"idx_app_chat_sessions_scene",
	} {
		if !strings.Contains(sql, statement) {
			t.Fatalf("schema is missing chat scene migration %q", statement)
		}
	}
}

func TestMigrateSchemaPreparesLegacyVideoColumnsBeforeFullSchema(t *testing.T) {
	raw, err := os.ReadFile("db.go")
	if err != nil {
		t.Fatal(err)
	}
	source := string(raw)
	compatibilityExec := strings.Index(source, "preSchemaCompatibilitySQL")
	fullSchemaExec := strings.Index(source, "tx.ExecContext(ctx, schemaSQL)")
	if compatibilityExec < 0 || fullSchemaExec < 0 || compatibilityExec > fullSchemaExec {
		t.Fatal("legacy video compatibility SQL must run before the full embedded schema")
	}
	for _, table := range []string{
		"ALTER TABLE IF EXISTS video_project_characters",
		"ALTER TABLE IF EXISTS video_project_scenes",
		"ALTER TABLE IF EXISTS video_project_assets",
	} {
		if !strings.Contains(source, table) {
			t.Fatalf("missing pre-schema compatibility migration %q", table)
		}
	}
	if strings.Count(source, "ADD COLUMN IF NOT EXISTS breakdown_item_key") < 3 {
		t.Fatal("all legacy video project tables must add breakdown_item_key before the full schema")
	}
}

func TestSchemaDefinesOnlinePaymentOrderColumns(t *testing.T) {
	raw, err := os.ReadFile("schema.sql")
	if err != nil {
		t.Fatal(err)
	}
	sql := string(raw)
	createStart := strings.Index(sql, "CREATE TABLE IF NOT EXISTS app_orders")
	if createStart < 0 {
		t.Fatal("schema is missing app_orders CREATE TABLE")
	}
	createEnd := strings.Index(sql[createStart:], ");")
	if createEnd < 0 {
		t.Fatal("schema has an unterminated app_orders CREATE TABLE")
	}
	create := sql[createStart : createStart+createEnd]
	for _, fragment := range []string{
		"payment_provider TEXT NOT NULL DEFAULT 'manual'",
		"pay_channel", "gateway_id", "provider_trade_no", "provider_status",
		"pay_url", "last_query_at", "payment_error",
	} {
		if !strings.Contains(create, fragment) {
			t.Errorf("app_orders CREATE TABLE is missing %q", fragment)
		}
	}
}

func TestSchemaMigratesOnlinePaymentOrderColumns(t *testing.T) {
	raw, err := os.ReadFile("schema.sql")
	if err != nil {
		t.Fatal(err)
	}
	sql := string(raw)
	for _, column := range []string{
		"payment_provider", "pay_channel", "gateway_id", "provider_trade_no",
		"provider_status", "pay_url", "last_query_at", "payment_error",
	} {
		statement := "ALTER TABLE app_orders ADD COLUMN IF NOT EXISTS " + column
		if !strings.Contains(sql, statement) {
			t.Errorf("schema is missing old-database migration %q", statement)
		}
	}
}

func TestSchemaAddsOnlinePaymentOrderUniqueness(t *testing.T) {
	raw, err := os.ReadFile("schema.sql")
	if err != nil {
		t.Fatal(err)
	}
	sql := string(raw)
	for _, fragment := range []string{
		"CREATE UNIQUE INDEX IF NOT EXISTS idx_app_orders_provider_trade_no",
		"ON app_orders(provider_trade_no)",
		"WHERE provider_trade_no IS NOT NULL AND provider_trade_no <> ''",
		"CREATE UNIQUE INDEX IF NOT EXISTS idx_app_orders_one_active_online_per_user",
		"ON app_orders(app_user_id)",
		"WHERE payment_provider = 'xzn' AND status IN ('pending', 'paying')",
	} {
		if !strings.Contains(sql, fragment) {
			t.Errorf("schema is missing online-payment uniqueness fragment %q", fragment)
		}
	}
	if !strings.Contains(sql, "payment_provider TEXT NOT NULL DEFAULT 'manual'") {
		t.Fatal("legacy orders must default to manual so pending_confirmation remains outside the online-order index")
	}
}

func TestAppOrderOnlinePaymentIndexesInPostgres(t *testing.T) {
	dsn := strings.TrimSpace(os.Getenv("TEST_DATABASE_URL"))
	if dsn == "" {
		t.Skip("set TEST_DATABASE_URL to run app order payment index integration test")
	}
	if err := testutil.ValidateIsolatedPostgresDSN(dsn); err != nil {
		t.Fatal(err)
	}

	adminDatabase, err := sql.Open("pgx", dsn)
	if err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithTimeout(context.Background(), 90*time.Second)
	defer cancel()
	schemaName := fmt.Sprintf("app_order_payment_%d", time.Now().UnixNano())
	if _, err := adminDatabase.ExecContext(ctx, "CREATE SCHEMA "+schemaName); err != nil {
		_ = adminDatabase.Close()
		t.Fatalf("create isolated test schema: %v", err)
	}
	scopedDSN, err := postgresDSNWithSearchPath(dsn, schemaName)
	if err != nil {
		t.Fatal(err)
	}
	database, err := sql.Open("pgx", scopedDSN)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		_ = database.Close()
		_, _ = adminDatabase.ExecContext(context.Background(), "DROP SCHEMA IF EXISTS "+schemaName+" CASCADE")
		_ = adminDatabase.Close()
	})

	if _, err := database.ExecContext(ctx, schemaSQL); err != nil {
		t.Fatalf("apply schema: %v", err)
	}
	var firstUserID, secondUserID int64
	if err := database.QueryRowContext(ctx, `INSERT INTO app_users (phone) VALUES ('13800000001') RETURNING id`).Scan(&firstUserID); err != nil {
		t.Fatal(err)
	}
	if err := database.QueryRowContext(ctx, `INSERT INTO app_users (phone) VALUES ('13800000002') RETURNING id`).Scan(&secondUserID); err != nil {
		t.Fatal(err)
	}

	insertOrder := func(outTradeNo string, userID int64, provider, status, providerTradeNo string) error {
		_, err := database.ExecContext(ctx, `
			INSERT INTO app_orders (out_trade_no, app_user_id, payment_provider, status, provider_trade_no)
			VALUES ($1, $2, $3, $4, NULLIF($5, ''))
		`, outTradeNo, userID, provider, status, providerTradeNo)
		return err
	}

	if err := insertOrder("provider-first", firstUserID, "xzn", "paid", "provider-duplicate"); err != nil {
		t.Fatalf("insert first provider trade number: %v", err)
	}
	if err := insertOrder("provider-second", secondUserID, "xzn", "paid", "provider-duplicate"); err == nil {
		t.Fatal("duplicate non-empty provider_trade_no was accepted")
	}

	if err := insertOrder("active-first", firstUserID, "xzn", "pending", "provider-active-1"); err != nil {
		t.Fatalf("insert first active online order: %v", err)
	}
	if err := insertOrder("active-second", firstUserID, "xzn", "paying", "provider-active-2"); err == nil {
		t.Fatal("second active online order for one user was accepted")
	}
	if err := insertOrder("manual-confirmation", firstUserID, "manual", "pending_confirmation", ""); err != nil {
		t.Fatalf("manual pending_confirmation order must remain allowed: %v", err)
	}
	if err := insertOrder("manual-pending", firstUserID, "manual", "pending", ""); err != nil {
		t.Fatalf("manual pending order must remain allowed: %v", err)
	}
	if err := insertOrder("other-user-active", secondUserID, "xzn", "pending", "provider-active-3"); err != nil {
		t.Fatalf("different users must be allowed concurrent online orders: %v", err)
	}
}
