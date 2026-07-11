package testutil

import (
	"fmt"
	"net/url"
	"strings"

	"github.com/jackc/pgx/v5"
)

// ValidateIsolatedPostgresDSN prevents integration tests from mutating a
// database that is not explicitly named as a disposable test database.
func ValidateIsolatedPostgresDSN(dsn string) error {
	dsn = strings.TrimSpace(dsn)
	if dsn == "" {
		return fmt.Errorf("test database URL is empty")
	}
	parsed, err := url.Parse(dsn)
	if err != nil {
		return fmt.Errorf("parse test database URL: %w", err)
	}
	if parsed.Scheme != "postgres" && parsed.Scheme != "postgresql" {
		return fmt.Errorf("test database URL must use postgres or postgresql")
	}
	config, err := pgx.ParseConfig(dsn)
	if err != nil {
		return fmt.Errorf("parse test database URL as PostgreSQL config: %w", err)
	}
	databaseName := strings.TrimSpace(config.Database)
	if databaseName == "" {
		return fmt.Errorf("test database URL must include a database name")
	}
	if !strings.HasSuffix(strings.ToLower(databaseName), "_test") {
		return fmt.Errorf("test database name %q must end with _test", databaseName)
	}
	return nil
}
