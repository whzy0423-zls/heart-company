// Package testdb provides guarded PostgreSQL fixtures for integration tests.
package testdb

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"net/url"
	"os"
	"regexp"
	"strings"
	"testing"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/stdlib"
)

var schemaPrefixPattern = regexp.MustCompile(`^[a-z][a-z0-9_]*$`)

func ParseSafeConfig(dsn string) (*pgx.ConnConfig, error) {
	lower := strings.ToLower(strings.TrimSpace(dsn))
	if strings.HasPrefix(lower, "postgres://") || strings.HasPrefix(lower, "postgresql://") {
		parsed, err := url.Parse(dsn)
		if err != nil {
			return nil, fmt.Errorf("parse TEST_DATABASE_URL: %w", err)
		}
		for _, key := range []string{"host", "hostaddr", "service", "servicefile"} {
			if _, exists := parsed.Query()[key]; exists {
				return nil, fmt.Errorf("TEST_DATABASE_URL must not use %s routing override", key)
			}
		}
	} else {
		for _, field := range strings.Fields(lower) {
			for _, key := range []string{"hostaddr=", "service=", "servicefile="} {
				if strings.HasPrefix(field, key) {
					return nil, fmt.Errorf("TEST_DATABASE_URL must not use %s routing override", strings.TrimSuffix(key, "="))
				}
			}
		}
	}
	config, err := pgx.ParseConfig(dsn)
	if err != nil {
		return nil, fmt.Errorf("parse final TEST_DATABASE_URL config: %w", err)
	}
	if !isLoopbackHost(config.Host) {
		return nil, fmt.Errorf("TEST_DATABASE_URL final host must be loopback, got %q", config.Host)
	}
	for _, fallback := range config.Fallbacks {
		if fallback == nil || !isLoopbackHost(fallback.Host) {
			var host string
			if fallback != nil {
				host = fallback.Host
			}
			return nil, fmt.Errorf("TEST_DATABASE_URL fallback host must be loopback, got %q", host)
		}
	}
	if config.Database == "" {
		return nil, errors.New("TEST_DATABASE_URL final database is empty")
	}
	if !isTestDatabase(config.Database) {
		return nil, fmt.Errorf("TEST_DATABASE_URL final database must follow the isolated test naming convention, got %q", config.Database)
	}
	return config, nil
}

func OpenIsolatedSchema(t testing.TB, dsn, prefix string) (*sql.DB, string) {
	t.Helper()
	config, err := ParseSafeConfig(strings.TrimSpace(dsn))
	if err != nil {
		t.Fatal(err)
	}
	name, err := schemaName(prefix, time.Now().UnixNano())
	if err != nil {
		t.Fatal(err)
	}
	adminDB := stdlib.OpenDB(*config)
	ctx, cancel := context.WithTimeout(context.Background(), 45*time.Second)
	t.Cleanup(cancel)
	if err := adminDB.PingContext(ctx); err != nil {
		_ = adminDB.Close()
		t.Fatalf("connect to isolated test database: %v", err)
	}
	if _, err := adminDB.ExecContext(ctx, `CREATE SCHEMA `+name); err != nil {
		_ = adminDB.Close()
		t.Fatal(err)
	}
	t.Cleanup(func() {
		_, _ = adminDB.Exec(`DROP SCHEMA IF EXISTS ` + name + ` CASCADE`)
		_ = adminDB.Close()
	})
	scoped := config.Copy()
	if scoped.RuntimeParams == nil {
		scoped.RuntimeParams = make(map[string]string)
	}
	scoped.RuntimeParams["search_path"] = name + ",public"
	database := stdlib.OpenDB(*scoped)
	t.Cleanup(func() { _ = database.Close() })
	return database, name
}

func OpenEnvIsolatedSchema(t testing.TB, prefix string) (*sql.DB, string) {
	t.Helper()
	dsn := strings.TrimSpace(os.Getenv("TEST_DATABASE_URL"))
	if dsn == "" {
		t.Skip("TEST_DATABASE_URL is absent; PostgreSQL test skipped without connecting to any development database")
	}
	return OpenIsolatedSchema(t, dsn, prefix)
}

func schemaName(prefix string, suffix int64) (string, error) {
	if !schemaPrefixPattern.MatchString(prefix) {
		return "", fmt.Errorf("unsafe test schema prefix %q", prefix)
	}
	return fmt.Sprintf("%s_%d", prefix, suffix), nil
}

func isLoopbackHost(host string) bool {
	switch strings.ToLower(strings.TrimSpace(host)) {
	case "localhost", "127.0.0.1", "::1":
		return true
	default:
		return false
	}
}

func isTestDatabase(database string) bool {
	name := strings.ToLower(strings.TrimSpace(database))
	return name == "test" || strings.HasPrefix(name, "test_") || strings.HasSuffix(name, "_test") || strings.Contains(name, "_test_")
}
