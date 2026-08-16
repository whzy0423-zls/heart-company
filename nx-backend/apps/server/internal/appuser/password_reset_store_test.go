package appuser

import (
	"context"
	"database/sql"
	"errors"
	"testing"
	"time"

	"golang.org/x/crypto/bcrypt"
)

func TestStorePasswordResetCodeIfEligibleOnlyStoresForActivePasswordUser(t *testing.T) {
	database := openRegisterWithPasswordFixture(t)
	ctx := context.Background()
	passwordHash := mustPasswordHash(t, "old-secret")
	for _, fixture := range []struct {
		phone        string
		passwordHash any
		status       string
		wantEligible bool
	}{
		{phone: "13800000301", passwordHash: passwordHash, status: "active", wantEligible: true},
		{phone: "13800000302", passwordHash: nil, status: "active", wantEligible: false},
		{phone: "13800000303", passwordHash: passwordHash, status: "disabled", wantEligible: false},
	} {
		if _, err := database.ExecContext(ctx, `
			INSERT INTO app_users (phone, account, password_hash, status)
			VALUES ($1, $2, $3, $4)
		`, fixture.phone, "reset_"+fixture.phone[8:], fixture.passwordHash, fixture.status); err != nil {
			t.Fatalf("seed reset user: %v", err)
		}
		eligible, err := NewStore(database).StorePasswordResetCodeIfEligible(
			ctx, fixture.phone, "reset-code-hash", "127.0.0.1", time.Now().Add(time.Hour),
		)
		if err != nil {
			t.Fatalf("StorePasswordResetCodeIfEligible() error = %v", err)
		}
		if eligible != fixture.wantEligible {
			t.Fatalf("phone %s eligible=%t want=%t", fixture.phone, eligible, fixture.wantEligible)
		}
		var rows int
		if err := database.QueryRowContext(ctx, `SELECT count(*) FROM app_password_reset_codes WHERE phone=$1`, fixture.phone).Scan(&rows); err != nil {
			t.Fatal(err)
		}
		if rows != boolCount(fixture.wantEligible) {
			t.Fatalf("phone %s reset rows=%d want=%d", fixture.phone, rows, boolCount(fixture.wantEligible))
		}
	}

	eligible, err := NewStore(database).StorePasswordResetCodeIfEligible(
		ctx, "13800000309", "unknown-code-hash", "127.0.0.1", time.Now().Add(time.Hour),
	)
	if err != nil || eligible {
		t.Fatalf("unknown phone eligible=%t error=%v", eligible, err)
	}
}

func TestResetPasswordConsumesLatestCodeUpdatesHashAndRevokesSessions(t *testing.T) {
	database := openRegisterWithPasswordFixture(t)
	ctx := context.Background()
	const phone = "13800000311"
	oldHash := mustPasswordHash(t, "old-secret")
	var userID int64
	if err := database.QueryRowContext(ctx, `
		INSERT INTO app_users (phone, account, password_hash)
		VALUES ($1, 'reset_success', $2)
		RETURNING id
	`, phone, oldHash).Scan(&userID); err != nil {
		t.Fatal(err)
	}
	for _, token := range []string{"reset-token-a", "reset-token-b"} {
		if _, err := database.ExecContext(ctx, `
			INSERT INTO app_refresh_tokens (app_user_id, token_hash, expires_at)
			VALUES ($1, $2, now() + interval '1 day')
		`, userID, token); err != nil {
			t.Fatal(err)
		}
	}
	stamp := time.Now().Add(-time.Minute)
	olderID := insertPasswordResetCode(t, database, phone, "valid-reset-hash", time.Now().Add(time.Hour), false, stamp)
	newerID := insertPasswordResetCode(t, database, phone, "valid-reset-hash", time.Now().Add(time.Hour), false, stamp)
	registrationCodeID := insertRegisterSMSCode(t, database, phone, "valid-reset-hash", time.Now().Add(time.Hour), false, stamp)

	err := NewStore(database).ResetPassword(ctx, ResetPasswordInput{
		Phone: phone, CodeHash: "valid-reset-hash", Password: "new-secret",
	})
	if err != nil {
		t.Fatalf("ResetPassword() error = %v", err)
	}

	var storedHash string
	if err := database.QueryRowContext(ctx, `SELECT password_hash FROM app_users WHERE id=$1`, userID).Scan(&storedHash); err != nil {
		t.Fatal(err)
	}
	if bcrypt.CompareHashAndPassword([]byte(storedHash), []byte("new-secret")) != nil {
		t.Fatal("new password does not match stored hash")
	}
	if bcrypt.CompareHashAndPassword([]byte(storedHash), []byte("old-secret")) == nil {
		t.Fatal("old password still matches stored hash")
	}
	if passwordResetCodeUsed(t, database, olderID) {
		t.Fatal("older reset code was consumed")
	}
	if !passwordResetCodeUsed(t, database, newerID) {
		t.Fatal("latest reset code was not consumed")
	}
	if registerSMSCodeUsed(t, database, registrationCodeID) {
		t.Fatal("password reset consumed a registration/login code")
	}
	var activeTokens int
	if err := database.QueryRowContext(ctx, `SELECT count(*) FROM app_refresh_tokens WHERE app_user_id=$1 AND revoked=false`, userID).Scan(&activeTokens); err != nil {
		t.Fatal(err)
	}
	if activeTokens != 0 {
		t.Fatalf("active refresh tokens=%d want=0", activeTokens)
	}
}

func TestResetPasswordReturnsGenericCredentialsErrorWithoutMutation(t *testing.T) {
	tests := []struct {
		name         string
		phone        string
		passwordHash any
		status       string
		codeHash     string
		expiresAt    time.Time
		used         bool
		seedUser     bool
	}{
		{name: "unknown phone", phone: "13800000321", codeHash: "code", expiresAt: time.Now().Add(time.Hour)},
		{name: "SMS-only user", phone: "13800000322", status: "active", codeHash: "code", expiresAt: time.Now().Add(time.Hour), seedUser: true},
		{name: "disabled user", phone: "13800000323", passwordHash: "set", status: "disabled", codeHash: "code", expiresAt: time.Now().Add(time.Hour), seedUser: true},
		{name: "wrong code", phone: "13800000324", passwordHash: "set", status: "active", codeHash: "stored", expiresAt: time.Now().Add(time.Hour), seedUser: true},
		{name: "expired code", phone: "13800000325", passwordHash: "set", status: "active", codeHash: "code", expiresAt: time.Now().Add(-time.Minute), seedUser: true},
		{name: "used code", phone: "13800000326", passwordHash: "set", status: "active", codeHash: "code", expiresAt: time.Now().Add(time.Hour), used: true, seedUser: true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			database := openRegisterWithPasswordFixture(t)
			ctx := context.Background()
			var beforeHash sql.NullString
			if tt.seedUser {
				passwordHash := tt.passwordHash
				if passwordHash == "set" {
					passwordHash = mustPasswordHash(t, "old-secret")
				}
				if _, err := database.ExecContext(ctx, `
					INSERT INTO app_users (phone, account, password_hash, status)
					VALUES ($1, $2, $3, $4)
				`, tt.phone, "failure_"+tt.phone[8:], passwordHash, tt.status); err != nil {
					t.Fatal(err)
				}
				if err := database.QueryRowContext(ctx, `SELECT password_hash FROM app_users WHERE phone=$1`, tt.phone).Scan(&beforeHash); err != nil {
					t.Fatal(err)
				}
			}
			codeID := insertPasswordResetCode(t, database, tt.phone, tt.codeHash, tt.expiresAt, tt.used, time.Now())
			providedHash := tt.codeHash
			if tt.name == "wrong code" {
				providedHash = "different"
			}
			err := NewStore(database).ResetPassword(ctx, ResetPasswordInput{
				Phone: tt.phone, CodeHash: providedHash, Password: "new-secret",
			})
			if !errors.Is(err, ErrInvalidCredentials) {
				t.Fatalf("ResetPassword() error=%v want ErrInvalidCredentials", err)
			}
			if passwordResetCodeUsed(t, database, codeID) != tt.used {
				t.Fatal("failed reset changed code usage")
			}
			if tt.seedUser {
				var afterHash sql.NullString
				if err := database.QueryRowContext(ctx, `SELECT password_hash FROM app_users WHERE phone=$1`, tt.phone).Scan(&afterHash); err != nil {
					t.Fatal(err)
				}
				if afterHash != beforeHash {
					t.Fatal("failed reset changed password hash")
				}
			}
		})
	}
}

func mustPasswordHash(t *testing.T, password string) string {
	t.Helper()
	hash, err := bcrypt.GenerateFromPassword([]byte(password), bcrypt.DefaultCost)
	if err != nil {
		t.Fatal(err)
	}
	return string(hash)
}

func insertPasswordResetCode(t *testing.T, database *sql.DB, phone, codeHash string, expiresAt time.Time, used bool, createTime time.Time) int64 {
	t.Helper()
	var id int64
	if err := database.QueryRow(`
		INSERT INTO app_password_reset_codes (phone, code_hash, expires_at, used, create_time)
		VALUES ($1, $2, $3, $4, $5)
		RETURNING id
	`, phone, codeHash, expiresAt, used, createTime).Scan(&id); err != nil {
		t.Fatal(err)
	}
	return id
}

func passwordResetCodeUsed(t *testing.T, database *sql.DB, id int64) bool {
	t.Helper()
	var used bool
	if err := database.QueryRow(`SELECT used FROM app_password_reset_codes WHERE id=$1`, id).Scan(&used); err != nil {
		t.Fatal(err)
	}
	return used
}

func boolCount(value bool) int {
	if value {
		return 1
	}
	return 0
}
