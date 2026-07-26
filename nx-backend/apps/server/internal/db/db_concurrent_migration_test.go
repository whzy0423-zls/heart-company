package db

import (
	"context"
	"database/sql"
	"fmt"
	"net/url"
	"os"
	"strings"
	"sync"
	"testing"
	"time"

	"nine-xing/nx-backend/apps/server/internal/testutil"

	_ "github.com/jackc/pgx/v5/stdlib"
)

func TestOpenSerializesConcurrentFullSchemaMigration(t *testing.T) {
	sourceDSN := strings.TrimSpace(os.Getenv("TEST_DATABASE_URL"))
	if sourceDSN == "" {
		t.Skip("set TEST_DATABASE_URL to run concurrent migration test")
	}
	if err := testutil.ValidateIsolatedPostgresDSN(sourceDSN); err != nil {
		t.Fatal(err)
	}
	parsed, err := url.Parse(sourceDSN)
	if err != nil {
		t.Fatal(err)
	}
	databaseName := fmt.Sprintf("nine_xing_migration_%d_test", time.Now().UnixNano())
	adminURL := *parsed
	adminURL.Path = "/postgres"
	adminQuery := adminURL.Query()
	adminQuery.Del("search_path")
	adminQuery.Del("dbname")
	adminQuery.Del("database")
	adminURL.RawQuery = adminQuery.Encode()
	adminDB, err := sql.Open("pgx", adminURL.String())
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = adminDB.Close() })
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Minute)
	defer cancel()
	if _, err := adminDB.ExecContext(ctx, `CREATE DATABASE `+databaseName); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _, _ = adminDB.Exec(`DROP DATABASE IF EXISTS ` + databaseName + ` WITH (FORCE)`) })
	testURL := *parsed
	testURL.Path = "/" + databaseName
	testDSN := testURL.String()

	start := make(chan struct{})
	errs := make(chan error, 2)
	var wg sync.WaitGroup
	for i := 0; i < 2; i++ {
		wg.Add(1)
		go func(index int) {
			defer wg.Done()
			<-start
			database, err := Open(ctx, testDSN, fmt.Sprintf("migration-admin-%d", index), "task2-password")
			if database != nil {
				_ = database.Close()
			}
			errs <- err
		}(i)
	}
	close(start)
	wg.Wait()
	close(errs)
	for err := range errs {
		if err != nil {
			t.Fatalf("concurrent Open failed: %v", err)
		}
	}
}
