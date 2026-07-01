package db

import (
	"context"
	"database/sql"
	"os"
	"testing"
	"time"
)

func TestSeedSelfHealsAdminRoleAndBinding(t *testing.T) {
	dsn := os.Getenv("TEST_DATABASE_URL")
	if dsn == "" {
		t.Skip("set TEST_DATABASE_URL to run database seed integration test")
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
