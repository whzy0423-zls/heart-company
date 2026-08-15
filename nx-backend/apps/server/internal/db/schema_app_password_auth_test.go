package db

import (
	"os"
	"strings"
	"testing"
)

func TestSchemaDefinesAppPasswordCredentials(t *testing.T) {
	raw, err := os.ReadFile("schema.sql")
	if err != nil {
		t.Fatal(err)
	}
	schema := string(raw)
	normalizedSchema := strings.Join(strings.Fields(schema), " ")

	appUsersCreate := "CREATE TABLE IF NOT EXISTS app_users"
	appUsersIndex := strings.Index(schema, appUsersCreate)
	if appUsersIndex < 0 {
		t.Fatal("schema is missing app_users")
	}
	appUsersEnd := strings.Index(schema[appUsersIndex:], ");")
	if appUsersEnd < 0 {
		t.Fatal("app_users CREATE TABLE is not terminated")
	}
	appUsersTable := strings.Join(strings.Fields(schema[appUsersIndex:appUsersIndex+appUsersEnd]), " ")

	for _, column := range []string{
		"account TEXT",
		"password_hash TEXT",
	} {
		if !strings.Contains(appUsersTable, column) {
			t.Fatalf("app_users fresh-table definition is missing %q", column)
		}
	}

	required := []string{
		"ALTER TABLE app_users ADD COLUMN IF NOT EXISTS account TEXT",
		"ALTER TABLE app_users ADD COLUMN IF NOT EXISTS password_hash TEXT",
		"CREATE UNIQUE INDEX IF NOT EXISTS idx_app_users_account_unique",
		"ON app_users (lower(account))",
		"WHERE account IS NOT NULL AND btrim(account) <> ''",
	}
	for _, fragment := range required {
		if !strings.Contains(normalizedSchema, fragment) {
			t.Fatalf("schema is missing app password credential migration %q", fragment)
		}
	}

	normalizedCreateIndex := strings.Index(normalizedSchema, appUsersCreate)
	for _, statement := range required[:3] {
		statementIndex := strings.Index(normalizedSchema, statement)
		if statementIndex < normalizedCreateIndex {
			t.Fatalf("%q must occur after app_users CREATE TABLE", statement)
		}
	}
}
