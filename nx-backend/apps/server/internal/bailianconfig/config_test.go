package bailianconfig

import (
	"context"
	"database/sql"
	"errors"
	"net"
	"net/url"
	"os"
	"strings"
	"testing"
	"time"

	_ "github.com/jackc/pgx/v5/stdlib"
)

var (
	errTestDatabaseUnavailable = errors.New("set a localhost TEST_DATABASE_URL to run bailian credential store integration tests")
	errTestDatabaseRequired    = errors.New("CI requires TEST_DATABASE_URL for bailian credential store integration tests")
)

func TestReadReturnsNotFoundWhenSharedCredentialHasNeverBeenSaved(t *testing.T) {
	database := openTestDB(t)

	got, found, err := Read(context.Background(), database)
	if err != nil {
		t.Fatalf("Read: %v", err)
	}
	if found {
		t.Fatalf("found=%v config=%#v, want no shared credential record", found, got)
	}
	if got != (Config{}) {
		t.Fatalf("config=%#v want zero Config", got)
	}
}

func TestUpdateCreatesVersionedSharedCredential(t *testing.T) {
	database := openTestDB(t)

	created, err := Update(context.Background(), database, "sk-first", 0, false)
	if err != nil {
		t.Fatalf("Update: %v", err)
	}
	if want := (Config{Version: 1, APIKey: "sk-first"}); created != want {
		t.Fatalf("created=%#v want=%#v", created, want)
	}

	stored, found, err := Read(context.Background(), database)
	if err != nil || !found {
		t.Fatalf("Read found=%v err=%v", found, err)
	}
	if stored != created {
		t.Fatalf("stored=%#v want=%#v", stored, created)
	}
}

func TestUpdateEmptyKeyPreservesExistingSharedCredential(t *testing.T) {
	database := openTestDB(t)
	created, err := Update(context.Background(), database, "sk-original", 0, false)
	if err != nil {
		t.Fatal(err)
	}

	updated, err := Update(context.Background(), database, "", created.Version, false)
	if err != nil {
		t.Fatal(err)
	}
	if want := (Config{Version: 2, APIKey: "sk-original"}); updated != want {
		t.Fatalf("updated=%#v want=%#v", updated, want)
	}
}

func TestUpdateExplicitClearCreatesAndKeepsEmptySharedCredentialRecord(t *testing.T) {
	database := openTestDB(t)
	created, err := Update(context.Background(), database, "sk-original", 0, false)
	if err != nil {
		t.Fatal(err)
	}

	cleared, err := Update(context.Background(), database, "", created.Version, true)
	if err != nil {
		t.Fatal(err)
	}
	if want := (Config{Version: 2}); cleared != want {
		t.Fatalf("cleared=%#v want=%#v", cleared, want)
	}

	stored, found, err := Read(context.Background(), database)
	if err != nil || !found {
		t.Fatalf("Read after clear found=%v err=%v", found, err)
	}
	if stored != cleared {
		t.Fatalf("stored=%#v want=%#v", stored, cleared)
	}
}

func TestUpdateExplicitClearCreatesEmptyRecordOnFirstSave(t *testing.T) {
	database := openTestDB(t)

	cleared, err := Update(context.Background(), database, "", 0, true)
	if err != nil {
		t.Fatal(err)
	}
	if want := (Config{Version: 1}); cleared != want {
		t.Fatalf("cleared=%#v want=%#v", cleared, want)
	}
	_, found, err := Read(context.Background(), database)
	if err != nil || !found {
		t.Fatalf("Read after first clear found=%v err=%v", found, err)
	}
}

func TestUpdateEmptyFirstSaveIsNoOpAndDoesNotDisableLegacyFallback(t *testing.T) {
	database := openTestDB(t)

	updated, err := Update(context.Background(), database, "", 0, false)
	if err != nil {
		t.Fatalf("Update: %v", err)
	}
	if updated != (Config{}) {
		t.Fatalf("updated=%#v want zero Config for no-op", updated)
	}
	_, found, err := Read(context.Background(), database)
	if err != nil {
		t.Fatal(err)
	}
	if found {
		t.Fatal("empty first save must not create a record because legacy fallback must remain available")
	}
}

func TestUpdateRejectsExpectedVersionConflict(t *testing.T) {
	database := openTestDB(t)
	created, err := Update(context.Background(), database, "sk-first", 0, false)
	if err != nil {
		t.Fatal(err)
	}

	_, err = Update(context.Background(), database, "sk-second", 0, false)
	if !errors.Is(err, ErrConflict) {
		t.Fatalf("err=%v want ErrConflict", err)
	}

	updated, err := Update(context.Background(), database, "sk-second", created.Version, false)
	if err != nil {
		t.Fatal(err)
	}
	if want := (Config{Version: 2, APIKey: "sk-second"}); updated != want {
		t.Fatalf("updated=%#v want=%#v", updated, want)
	}
}

func TestReadReportsFoundForExistingEmptySharedCredentialRecord(t *testing.T) {
	database := openTestDB(t)
	if _, err := Update(context.Background(), database, "", 0, true); err != nil {
		t.Fatal(err)
	}

	got, found, err := Read(context.Background(), database)
	if err != nil {
		t.Fatal(err)
	}
	if !found {
		t.Fatal("empty stored credential must still report found=true")
	}
	if want := (Config{Version: 1}); got != want {
		t.Fatalf("config=%#v want=%#v", got, want)
	}
}

func testDSN(getenv func(string) string) (string, error) {
	if dsn := strings.TrimSpace(getenv("TEST_DATABASE_URL")); dsn != "" {
		return dsn, nil
	}
	if strings.TrimSpace(getenv("CI")) != "" {
		return "", errTestDatabaseRequired
	}
	return "", errTestDatabaseUnavailable
}

func openTestDB(t *testing.T) *sql.DB {
	t.Helper()
	dsn, err := testDSN(os.Getenv)
	if errors.Is(err, errTestDatabaseUnavailable) {
		t.Skip(err.Error())
	}
	if err != nil {
		t.Fatal(err)
	}
	parsed, err := url.Parse(dsn)
	if err != nil {
		t.Fatalf("parse TEST_DATABASE_URL: %v", err)
	}
	host := parsed.Hostname()
	if host != "localhost" && !net.ParseIP(host).IsLoopback() {
		t.Fatalf("TEST_DATABASE_URL must point to localhost, got host %q", host)
	}
	if parsed.Scheme != "postgres" && parsed.Scheme != "postgresql" {
		t.Fatalf("TEST_DATABASE_URL must use PostgreSQL, got scheme %q", parsed.Scheme)
	}
	if databaseName := strings.TrimPrefix(parsed.Path, "/"); !strings.Contains(strings.ToLower(databaseName), "test") {
		t.Fatalf("TEST_DATABASE_URL must name an isolated test database, got %q", databaseName)
	}
	if parsed.Query().Has("dbname") || parsed.Query().Has("database") {
		t.Fatal("TEST_DATABASE_URL must not override the database name in query parameters")
	}

	database, err := sql.Open("pgx", dsn)
	if err != nil {
		t.Fatalf("open test database: %v", err)
	}
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	if err := database.PingContext(ctx); err != nil {
		_ = database.Close()
		t.Fatalf("ping test database: %v", err)
	}
	if _, err := database.ExecContext(ctx, `CREATE TABLE IF NOT EXISTS site_configs (
		key TEXT PRIMARY KEY,
		config JSONB NOT NULL,
		update_time TIMESTAMPTZ NOT NULL DEFAULT now()
	)`); err != nil {
		_ = database.Close()
		t.Fatalf("ensure site_configs: %v", err)
	}
	if _, err := database.ExecContext(ctx, `DELETE FROM site_configs WHERE key=$1`, ConfigKey); err != nil {
		_ = database.Close()
		t.Fatalf("clean credential fixture: %v", err)
	}
	t.Cleanup(func() {
		_, _ = database.Exec(`DELETE FROM site_configs WHERE key=$1`, ConfigKey)
		_ = database.Close()
	})
	return database
}
