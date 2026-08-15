package db

import (
	"context"
	"database/sql"
	"os"
	"strings"
	"testing"
	"time"

	"nine-xing/nx-backend/apps/server/internal/testdb"
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
	appUsersEndOffset := strings.Index(schema[appUsersIndex:], ");")
	if appUsersEndOffset < 0 {
		t.Fatal("app_users CREATE TABLE is not terminated")
	}
	appUsersTerminatorIndex := appUsersIndex + appUsersEndOffset
	appUsersTable := schema[appUsersIndex : appUsersTerminatorIndex+len(");")]

	for _, column := range []string{
		"account",
		"password_hash",
	} {
		assertNullableTextColumnDefinition(t, appUsersTable, column)
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
	normalizedEndOffset := strings.Index(normalizedSchema[normalizedCreateIndex:], ");")
	if normalizedEndOffset < 0 {
		t.Fatal("normalized app_users CREATE TABLE is not terminated")
	}
	normalizedTerminatorIndex := normalizedCreateIndex + normalizedEndOffset
	for _, statement := range required[:3] {
		statementIndex := strings.Index(normalizedSchema, statement)
		if statementIndex <= normalizedTerminatorIndex {
			t.Fatalf("%q must occur after the app_users CREATE TABLE terminator", statement)
		}
	}
}

func TestSchemaAppPasswordCredentialsPostgres(t *testing.T) {
	t.Run("fresh schema", func(t *testing.T) {
		database, _ := testdb.OpenEnvIsolatedSchema(t, "app_password_fresh")
		ctx, cancel := context.WithTimeout(context.Background(), 90*time.Second)
		defer cancel()

		applyAppPasswordSchemaTwice(t, ctx, database)
		assertAppPasswordCredentialColumnsNullable(t, ctx, database)

		if _, err := database.ExecContext(ctx, `
			INSERT INTO app_users (phone, nickname) VALUES
				('13800000001', 'legacy user one'),
				('13800000002', 'legacy user two')
		`); err != nil {
			t.Fatalf("insert legacy-style app users: %v", err)
		}

		var legacyUsers int
		if err := database.QueryRowContext(ctx, `
			SELECT count(*)
			FROM app_users
			WHERE phone IN ('13800000001', '13800000002')
			  AND account IS NULL
			  AND password_hash IS NULL
		`).Scan(&legacyUsers); err != nil {
			t.Fatalf("count legacy-style app users: %v", err)
		}
		if legacyUsers != 2 {
			t.Fatalf("legacy-style users with NULL credentials=%d, want 2", legacyUsers)
		}

		if _, err := database.ExecContext(ctx, `
			INSERT INTO app_users (phone, account, password_hash)
			VALUES ('13800000003', 'CaseUser', 'hash-one')
		`); err != nil {
			t.Fatalf("insert first non-empty account: %v", err)
		}
		if _, err := database.ExecContext(ctx, `
			INSERT INTO app_users (phone, account, password_hash)
			VALUES ('13800000004', 'caseuser', 'hash-two')
		`); err == nil {
			t.Fatal("case-insensitive duplicate non-empty account should be rejected")
		}
	})

	t.Run("legacy schema migration", func(t *testing.T) {
		database, _ := testdb.OpenEnvIsolatedSchema(t, "app_password_legacy")
		ctx, cancel := context.WithTimeout(context.Background(), 90*time.Second)
		defer cancel()

		if _, err := database.ExecContext(ctx, `
			CREATE TABLE app_users (
				id BIGSERIAL PRIMARY KEY,
				phone TEXT NOT NULL UNIQUE,
				nickname TEXT NOT NULL DEFAULT '',
				avatar TEXT NOT NULL DEFAULT '',
				status TEXT NOT NULL DEFAULT 'active',
				member_level TEXT NOT NULL DEFAULT 'free',
				member_started_at TIMESTAMPTZ,
				member_expires_at TIMESTAMPTZ,
				register_source TEXT NOT NULL DEFAULT 'sms',
				last_login_at TIMESTAMPTZ,
				create_time TIMESTAMPTZ NOT NULL DEFAULT now(),
				update_time TIMESTAMPTZ NOT NULL DEFAULT now()
			)
		`); err != nil {
			t.Fatalf("create pre-feature app_users table: %v", err)
		}

		var legacyUserID int64
		if err := database.QueryRowContext(ctx, `
			INSERT INTO app_users (
				phone,
				nickname,
				avatar,
				status,
				member_level,
				member_started_at,
				member_expires_at,
				register_source,
				last_login_at,
				create_time,
				update_time
			) VALUES (
				'13800000010',
				'Existing SMS User',
				'legacy-avatar.png',
				'active',
				'premium',
				TIMESTAMPTZ '2026-01-15 08:00:00+00',
				TIMESTAMPTZ '2027-01-15 08:00:00+00',
				'sms',
				TIMESTAMPTZ '2026-08-14 08:00:00+00',
				TIMESTAMPTZ '2026-01-15 08:00:00+00',
				TIMESTAMPTZ '2026-08-14 08:00:00+00'
			)
			RETURNING id
		`).Scan(&legacyUserID); err != nil {
			t.Fatalf("insert pre-feature app user: %v", err)
		}

		applyAppPasswordSchemaTwice(t, ctx, database)
		assertAppPasswordCredentialColumnsNullable(t, ctx, database)

		var (
			gotID        int64
			gotPhone     string
			gotNickname  string
			account      sql.NullString
			passwordHash sql.NullString
		)
		if err := database.QueryRowContext(ctx, `
			SELECT id, phone, nickname, account, password_hash
			FROM app_users
			WHERE id=$1
		`, legacyUserID).Scan(&gotID, &gotPhone, &gotNickname, &account, &passwordHash); err != nil {
			t.Fatalf("read migrated pre-feature app user: %v", err)
		}
		if gotID != legacyUserID || gotPhone != "13800000010" || gotNickname != "Existing SMS User" {
			t.Fatalf("pre-feature app user changed during migration: id=%d phone=%q nickname=%q", gotID, gotPhone, gotNickname)
		}
		if account.Valid || passwordHash.Valid {
			t.Fatalf("pre-feature app user credentials must remain NULL: account=%v password_hash=%v", account, passwordHash)
		}
	})
}

func assertNullableTextColumnDefinition(t *testing.T, tableSQL, column string) {
	t.Helper()
	want := column + " TEXT,"
	for _, line := range strings.Split(tableSQL, "\n") {
		normalized := strings.Join(strings.Fields(line), " ")
		if !strings.HasPrefix(normalized, column+" ") {
			continue
		}
		if normalized != want {
			t.Fatalf("app_users fresh-table definition for %s=%q, want %q", column, normalized, want)
		}
		return
	}
	t.Fatalf("app_users fresh-table definition is missing %q", want)
}

func applyAppPasswordSchemaTwice(t *testing.T, ctx context.Context, database *sql.DB) {
	t.Helper()
	for pass := 1; pass <= 2; pass++ {
		if _, err := database.ExecContext(ctx, schemaSQL); err != nil {
			t.Fatalf("apply app password schema pass %d: %v", pass, err)
		}
	}
}

func assertAppPasswordCredentialColumnsNullable(t *testing.T, ctx context.Context, database *sql.DB) {
	t.Helper()
	for _, column := range []string{"account", "password_hash"} {
		var dataType, nullable string
		if err := database.QueryRowContext(ctx, `
			SELECT data_type, is_nullable
			FROM information_schema.columns
			WHERE table_schema=current_schema()
			  AND table_name='app_users'
			  AND column_name=$1
		`, column).Scan(&dataType, &nullable); err != nil {
			t.Fatalf("read app_users.%s catalog entry: %v", column, err)
		}
		if dataType != "text" || nullable != "YES" {
			t.Fatalf("app_users.%s type=%q nullable=%q, want text/YES", column, dataType, nullable)
		}
	}
}
