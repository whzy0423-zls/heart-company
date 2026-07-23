package db

import (
	"context"
	"database/sql"
	"fmt"
	"os"
	"testing"
	"time"

	"nine-xing/nx-backend/apps/server/internal/testutil"
)

func TestSeedCustomerMiniappMenuBindingsUpgradesOnlyExistingCustomerReaders(t *testing.T) {
	dsn := os.Getenv("TEST_DATABASE_URL")
	if dsn == "" {
		t.Skip("set TEST_DATABASE_URL to run customer menu binding integration test")
	}
	if err := testutil.ValidateIsolatedPostgresDSN(dsn); err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
	defer cancel()

	adminDatabase, err := sql.Open("pgx", dsn)
	if err != nil {
		t.Fatal(err)
	}
	schemaName := fmt.Sprintf("customer_menu_binding_%d", time.Now().UnixNano())
	if _, err := adminDatabase.ExecContext(ctx, "CREATE SCHEMA "+schemaName); err != nil {
		_ = adminDatabase.Close()
		t.Fatalf("create isolated schema: %v", err)
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

	if _, err := database.ExecContext(ctx, `
		CREATE TABLE roles (
			id BIGSERIAL PRIMARY KEY,
			code TEXT NOT NULL UNIQUE,
			name TEXT NOT NULL,
			remark TEXT NOT NULL DEFAULT '',
			status INT NOT NULL DEFAULT 1,
			create_time TIMESTAMPTZ NOT NULL DEFAULT now()
		);
		CREATE TABLE menus (
			id BIGSERIAL PRIMARY KEY,
			pid BIGINT NOT NULL DEFAULT 0,
			name TEXT NOT NULL,
			path TEXT NOT NULL DEFAULT '',
			component TEXT NOT NULL DEFAULT '',
			auth_code TEXT NOT NULL DEFAULT '',
			type TEXT NOT NULL DEFAULT 'menu',
			status INT NOT NULL DEFAULT 1,
			sort INT NOT NULL DEFAULT 0,
			meta JSONB NOT NULL DEFAULT '{}'::jsonb
		);
		CREATE TABLE role_menus (
			role_id BIGINT NOT NULL REFERENCES roles(id) ON DELETE CASCADE,
			menu_id BIGINT NOT NULL REFERENCES menus(id) ON DELETE CASCADE,
			PRIMARY KEY (role_id, menu_id)
		)`); err != nil {
		t.Fatalf("create seed test tables: %v", err)
	}
	if err := seedMenus(ctx, database); err != nil {
		t.Fatalf("seed menus: %v", err)
	}

	var eligibleRoleID, ineligibleRoleID int64
	if err := database.QueryRowContext(ctx,
		`INSERT INTO roles (code,name) VALUES ('customer_reader_test','客户只读测试') RETURNING id`,
	).Scan(&eligibleRoleID); err != nil {
		t.Fatal(err)
	}
	if err := database.QueryRowContext(ctx,
		`INSERT INTO roles (code,name) VALUES ('other_reader_test','其他只读测试') RETURNING id`,
	).Scan(&ineligibleRoleID); err != nil {
		t.Fatal(err)
	}
	if _, err := database.ExecContext(ctx,
		`INSERT INTO role_menus (role_id,menu_id) VALUES ($1,501),($1,502),($2,503)`,
		eligibleRoleID, ineligibleRoleID); err != nil {
		t.Fatal(err)
	}

	if err := seedRoles(ctx, database); err != nil {
		t.Fatalf("seed roles: %v", err)
	}
	if err := seedCustomerMiniappMenuBindings(ctx, database); err != nil {
		t.Fatalf("seed miniapp customer bindings: %v", err)
	}
	if err := seedCustomerMiniappMenuBindings(ctx, database); err != nil {
		t.Fatalf("seed miniapp customer bindings should be idempotent: %v", err)
	}

	assertRoleMenuBindingCount(t, ctx, database, "customer_reader_test", 511, 1)
	assertRoleMenuBindingCount(t, ctx, database, "other_reader_test", 511, 0)
	assertRoleMenuBindingCount(t, ctx, database, "admin", 511, 1)
}

func assertRoleMenuBindingCount(t *testing.T, ctx context.Context, database *sql.DB, roleCode string, menuID int64, want int) {
	t.Helper()
	var got int
	if err := database.QueryRowContext(ctx,
		`SELECT count(*) FROM role_menus rm JOIN roles r ON r.id=rm.role_id WHERE r.code=$1 AND rm.menu_id=$2`,
		roleCode, menuID,
	).Scan(&got); err != nil {
		t.Fatal(err)
	}
	if got != want {
		t.Fatalf("role %q menu %d binding count = %d, want %d", roleCode, menuID, got, want)
	}
}

func TestSeedSelfHealsAdminRoleAndBinding(t *testing.T) {
	dsn := os.Getenv("TEST_DATABASE_URL")
	if dsn == "" {
		t.Skip("set TEST_DATABASE_URL to run database seed integration test")
	}
	if err := testutil.ValidateIsolatedPostgresDSN(dsn); err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
	defer cancel()
	database, err := sql.Open("pgx", dsn)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = database.Close() })
	if _, err := database.ExecContext(ctx, schemaSQL); err != nil {
		t.Fatalf("migrate: %v", err)
	}

	adminUser := "admin_self_heal_test"
	_, _ = database.ExecContext(ctx, `DELETE FROM user_roles WHERE user_id IN (SELECT id FROM users WHERE username=$1)`, adminUser)
	_, _ = database.ExecContext(ctx, `DELETE FROM users WHERE username=$1`, adminUser)

	if err := seedRoles(ctx, database); err != nil {
		t.Fatalf("seed roles: %v", err)
	}
	if err := seedRoles(ctx, database); err != nil {
		t.Fatalf("seed roles should be idempotent: %v", err)
	}
	if _, err := database.ExecContext(ctx,
		`INSERT INTO users (username, password_hash, nickname, status) VALUES ($1,'x',$2,1)`,
		adminUser, "admin self heal"); err != nil {
		t.Fatal(err)
	}

	if err := seedAdmin(ctx, database, adminUser, "123456"); err != nil {
		t.Fatalf("seed admin: %v", err)
	}

	var count int
	if err := database.QueryRowContext(ctx,
		`SELECT count(*)
		   FROM users u
		   JOIN user_roles ur ON ur.user_id=u.id
		   JOIN roles r ON r.id=ur.role_id
		  WHERE u.username=$1 AND r.code='admin'`,
		adminUser,
	).Scan(&count); err != nil {
		t.Fatal(err)
	}
	if count != 1 {
		t.Fatalf("expected existing admin user to be bound to admin role, got %d", count)
	}
}
