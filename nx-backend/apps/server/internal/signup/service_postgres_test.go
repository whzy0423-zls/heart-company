package signup

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"net/http/httptest"
	"net/url"
	"os"
	"strings"
	"testing"
	"time"

	"nine-xing/nx-backend/apps/server/internal/businessmessage"
	"nine-xing/nx-backend/apps/server/internal/dbtx"
	"nine-xing/nx-backend/apps/server/internal/testutil"

	_ "github.com/jackc/pgx/v5/stdlib"
)

func TestWebsiteSignupServicePostgresCommitsSignupAndMessageAtomically(t *testing.T) {
	database := openSignupServicePostgres(t)
	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
	defer cancel()
	request := httptest.NewRequest("POST", "/api/public/signups", nil)
	request.RemoteAddr = "127.0.0.1:12345"
	service := NewService(dbtx.SQLBeginner{DB: database}, NewStore(database), businessmessage.Store{})

	lead, err := service.CreateWebsiteSignup(ctx, LeadInput{
		Name: "张三", ContactType: ContactTypePhone, Contact: "13812345678",
	}, request)

	if err != nil {
		t.Fatalf("CreateWebsiteSignup() error = %v", err)
	}
	var signupCount, messageCount int
	var sourcePlatform, messagePlatform, businessID string
	if err := database.QueryRowContext(ctx, `SELECT count(*),min(source_platform) FROM signups`).Scan(&signupCount, &sourcePlatform); err != nil {
		t.Fatal(err)
	}
	if err := database.QueryRowContext(ctx, `SELECT count(*),min(platform),min(business_id) FROM messages`).Scan(&messageCount, &messagePlatform, &businessID); err != nil {
		t.Fatal(err)
	}
	if signupCount != 1 || sourcePlatform != "website" {
		t.Fatalf("signups = count:%d source:%q, want one website signup", signupCount, sourcePlatform)
	}
	if messageCount != 1 || messagePlatform != "website" || businessID != lead.ID {
		t.Fatalf("messages = count:%d platform:%q business:%q, want one website message for %q", messageCount, messagePlatform, businessID, lead.ID)
	}
}

func TestWebsiteSignupServicePostgresRollsBackSignupWhenMessageFails(t *testing.T) {
	database := openSignupServicePostgres(t)
	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
	defer cancel()
	request := httptest.NewRequest("POST", "/api/public/signups", nil)
	service := NewService(dbtx.SQLBeginner{DB: database}, NewStore(database), failingSignupMessageWriter{})

	_, err := service.CreateWebsiteSignup(ctx, LeadInput{
		Name: "李四", ContactType: ContactTypePhone, Contact: "13912345678",
	}, request)

	if err == nil || !strings.Contains(err.Error(), "create website signup message") {
		t.Fatalf("expected wrapped message failure, got %v", err)
	}
	var signupCount, messageCount int
	if err := database.QueryRowContext(ctx, `SELECT count(*) FROM signups`).Scan(&signupCount); err != nil {
		t.Fatal(err)
	}
	if err := database.QueryRowContext(ctx, `SELECT count(*) FROM messages`).Scan(&messageCount); err != nil {
		t.Fatal(err)
	}
	if signupCount != 0 || messageCount != 0 {
		t.Fatalf("rollback counts = signups:%d messages:%d, want both zero", signupCount, messageCount)
	}
}

type failingSignupMessageWriter struct{}

func (failingSignupMessageWriter) Create(context.Context, dbtx.DBTX, businessmessage.Event) (bool, error) {
	return false, errors.New("injected message failure")
}

func openSignupServicePostgres(t *testing.T) *sql.DB {
	t.Helper()
	dsn := os.Getenv("TEST_DATABASE_URL")
	if dsn == "" {
		t.Skip("set TEST_DATABASE_URL to run signup service PostgreSQL integration tests")
	}
	if err := testutil.ValidateIsolatedPostgresDSN(dsn); err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	adminDB, err := sql.Open("pgx", dsn)
	if err != nil {
		t.Fatal(err)
	}
	schemaName := fmt.Sprintf("signup_service_%d", time.Now().UnixNano())
	if _, err := adminDB.ExecContext(ctx, "CREATE SCHEMA "+schemaName); err != nil {
		_ = adminDB.Close()
		t.Fatalf("create isolated schema: %v", err)
	}
	var database *sql.DB
	t.Cleanup(func() {
		if database != nil {
			_ = database.Close()
		}
		_, _ = adminDB.ExecContext(context.Background(), "DROP SCHEMA IF EXISTS "+schemaName+" CASCADE")
		_ = adminDB.Close()
	})
	scopedDSN, err := signupServiceDSNWithSearchPath(dsn, schemaName)
	if err != nil {
		t.Fatal(err)
	}
	database, err = sql.Open("pgx", scopedDSN)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := database.ExecContext(ctx, `
		CREATE TABLE signups (
		  id BIGSERIAL PRIMARY KEY,
		  name TEXT NOT NULL,
		  contact_type TEXT NOT NULL,
		  contact TEXT NOT NULL,
		  interest TEXT NOT NULL DEFAULT '',
		  message TEXT NOT NULL DEFAULT '',
		  visitor_id TEXT NOT NULL DEFAULT '',
		  source_path TEXT NOT NULL DEFAULT '/',
		  landing_page TEXT NOT NULL DEFAULT '',
		  referrer TEXT NOT NULL DEFAULT '',
		  utm_source TEXT NOT NULL DEFAULT '',
		  utm_medium TEXT NOT NULL DEFAULT '',
		  utm_campaign TEXT NOT NULL DEFAULT '',
		  utm_content TEXT NOT NULL DEFAULT '',
		  utm_term TEXT NOT NULL DEFAULT '',
		  game_result_id BIGINT,
		  ip TEXT NOT NULL DEFAULT '',
		  user_agent TEXT NOT NULL DEFAULT '',
		  source_platform TEXT NOT NULL CHECK (source_platform IN ('website','miniapp')),
		  create_time TIMESTAMPTZ NOT NULL DEFAULT now()
		);
		CREATE TABLE messages (
		  id BIGSERIAL PRIMARY KEY,
		  type TEXT NOT NULL,
		  title TEXT NOT NULL,
		  content TEXT NOT NULL,
		  platform TEXT NOT NULL,
		  event_key TEXT NOT NULL,
		  business_id TEXT NOT NULL,
		  business_type TEXT NOT NULL,
		  target_path TEXT NOT NULL
		);
		CREATE UNIQUE INDEX uq_messages_event_business
		  ON messages(event_key,business_type,business_id);
	`); err != nil {
		t.Fatalf("create signup service fixtures: %v", err)
	}
	return database
}

func signupServiceDSNWithSearchPath(dsn, schemaName string) (string, error) {
	parsed, err := url.Parse(dsn)
	if err != nil {
		return "", err
	}
	query := parsed.Query()
	query.Set("search_path", schemaName)
	parsed.RawQuery = query.Encode()
	return parsed.String(), nil
}
