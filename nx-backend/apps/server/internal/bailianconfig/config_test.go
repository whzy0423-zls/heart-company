package bailianconfig

import (
	"context"
	"database/sql"
	"errors"
	"net"
	"net/url"
	"os"
	"strings"
	"sync"
	"testing"
	"time"

	"nine-xing/nx-backend/apps/server/internal/testutil"

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

func TestReadRejectsInvalidPersistedCredentialVersions(t *testing.T) {
	tests := []struct {
		name string
		body string
	}{
		{name: "json null", body: `null`},
		{name: "empty object", body: `{}`},
		{name: "missing version", body: `{"apiKey":"sk-missing"}`},
		{name: "zero version", body: `{"version":0,"apiKey":"sk-zero"}`},
		{name: "negative version", body: `{"version":-1,"apiKey":"sk-negative"}`},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			database := openTestDB(t)
			if _, err := database.Exec(`INSERT INTO site_configs (key, config, update_time) VALUES ($1, $2::jsonb, now())`, ConfigKey, tt.body); err != nil {
				t.Fatal(err)
			}

			_, found, err := Read(context.Background(), database)
			if err == nil {
				t.Fatal("Read accepted invalid stored credential")
			}
			if found {
				t.Fatalf("Read found=%v with invalid stored credential, want false", found)
			}
		})
	}
}

func TestUpdateConcurrentFirstSaveHasOneWinner(t *testing.T) {
	database := openTestDB(t)
	start := make(chan struct{})
	errs := make(chan error, 2)
	var group sync.WaitGroup
	for _, apiKey := range []string{"sk-first-a", "sk-first-b"} {
		group.Add(1)
		go func(apiKey string) {
			defer group.Done()
			<-start
			_, err := Update(context.Background(), database, apiKey, 0, false)
			errs <- err
		}(apiKey)
	}
	close(start)
	group.Wait()
	close(errs)

	assertOneSuccessAndOneConflict(t, errs)
	stored, found, err := Read(context.Background(), database)
	if err != nil || !found {
		t.Fatalf("Read found=%v err=%v", found, err)
	}
	if stored.Version != 1 || (stored.APIKey != "sk-first-a" && stored.APIKey != "sk-first-b") {
		t.Fatalf("stored=%#v want one first-write winner at version 1", stored)
	}
}

func TestUpdateConcurrentSameVersionHasOneWinner(t *testing.T) {
	database := openTestDB(t)
	created, err := Update(context.Background(), database, "sk-initial", 0, false)
	if err != nil {
		t.Fatal(err)
	}

	start := make(chan struct{})
	errs := make(chan error, 2)
	var group sync.WaitGroup
	for _, apiKey := range []string{"sk-update-a", "sk-update-b"} {
		group.Add(1)
		go func(apiKey string) {
			defer group.Done()
			<-start
			_, err := Update(context.Background(), database, apiKey, created.Version, false)
			errs <- err
		}(apiKey)
	}
	close(start)
	group.Wait()
	close(errs)

	assertOneSuccessAndOneConflict(t, errs)
	stored, found, err := Read(context.Background(), database)
	if err != nil || !found {
		t.Fatalf("Read found=%v err=%v", found, err)
	}
	if stored.Version != 2 || (stored.APIKey != "sk-update-a" && stored.APIKey != "sk-update-b") {
		t.Fatalf("stored=%#v want one version-2 update winner", stored)
	}
}

func TestUpdateFirstEmptyNoOpReleasesCredentialLock(t *testing.T) {
	database := openTestDB(t)

	if _, err := Update(context.Background(), database, "", 0, false); err != nil {
		t.Fatalf("empty first Update: %v", err)
	}
	ctx, cancel := context.WithTimeout(context.Background(), 250*time.Millisecond)
	defer cancel()
	created, err := Update(ctx, database, "sk-after-noop", 0, false)
	if err != nil {
		t.Fatalf("Update after no-op blocked instead of acquiring released lock: %v", err)
	}
	if want := (Config{Version: 1, APIKey: "sk-after-noop"}); created != want {
		t.Fatalf("created=%#v want=%#v", created, want)
	}
}

func assertOneSuccessAndOneConflict(t *testing.T, errs <-chan error) {
	t.Helper()
	var successes, conflicts int
	for err := range errs {
		switch {
		case err == nil:
			successes++
		case errors.Is(err, ErrConflict):
			conflicts++
		default:
			t.Fatalf("unexpected concurrent update error: %v", err)
		}
	}
	if successes != 1 || conflicts != 1 {
		t.Fatalf("successes=%d conflicts=%d, want one of each", successes, conflicts)
	}
}

func testDSN(getenv func(string) string) (string, error) {
	if dsn := strings.TrimSpace(getenv("TEST_DATABASE_URL")); dsn != "" {
		if err := validateTestDSN(dsn); err != nil {
			return "", err
		}
		return dsn, nil
	}
	if strings.TrimSpace(getenv("CI")) != "" {
		return "", errTestDatabaseRequired
	}
	return "", errTestDatabaseUnavailable
}

func validateTestDSN(dsn string) error {
	if err := testutil.ValidateIsolatedPostgresDSN(dsn); err != nil {
		return err
	}
	parsed, err := url.Parse(dsn)
	if err != nil {
		return err
	}
	host := parsed.Hostname()
	if host != "localhost" && !net.ParseIP(host).IsLoopback() {
		return errors.New("test database URL must point to localhost")
	}
	return nil
}

func TestTestDSNRejectsNonSuffixTestNames(t *testing.T) {
	tests := []struct {
		name    string
		dsn     string
		wantErr bool
	}{
		{name: "isolated suffix accepted", dsn: "postgres://nx:nx@localhost:5432/bailian_credentials_test?sslmode=disable"},
		{name: "remote isolated database is rejected", dsn: "postgres://nx:nx@db.example.com:5432/bailian_credentials_test?sslmode=disable", wantErr: true},
		{name: "contest is not a test database", dsn: "postgres://nx:nx@localhost:5432/contest?sslmode=disable", wantErr: true},
		{name: "latest data is not a test database", dsn: "postgres://nx:nx@localhost:5432/latest_data?sslmode=disable", wantErr: true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, err := testDSN(func(key string) string {
				if key == "TEST_DATABASE_URL" {
					return tt.dsn
				}
				return ""
			})
			if (err != nil) != tt.wantErr {
				t.Fatalf("testDSN(%q) err=%v wantErr=%v", tt.dsn, err, tt.wantErr)
			}
		})
	}
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
