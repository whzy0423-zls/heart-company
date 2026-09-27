package appuser

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"strings"
	"time"

	"golang.org/x/crypto/bcrypt"
)

// StoreEmailResetCodeIfEligible stores a one-time code only for an active App
// user with a password and a bound email address.
func (s *Store) StoreEmailResetCodeIfEligible(ctx context.Context, email, codeHash, sendIP string, expiresAt time.Time) (bool, error) {
	if s == nil || s.db == nil {
		return false, errors.New("appuser store unavailable")
	}
	email = NormalizeEmail(email)
	if err := ValidateEmail(email); err != nil {
		return false, err
	}
	var id int64
	err := s.db.QueryRowContext(ctx, `
		WITH invalidated AS (
			UPDATE app_email_reset_codes
			SET used = true
			WHERE email = $1 AND used = false
		)
		INSERT INTO app_email_reset_codes (email, code_hash, expires_at, send_ip)
		SELECT $1, $2, $3, $4
		FROM app_users
		WHERE lower(email) = lower($1)
		  AND status = 'active'
		  AND password_hash IS NOT NULL
		  AND btrim(password_hash) <> ''
		RETURNING id
	`, email, codeHash, expiresAt, sendIP).Scan(&id)
	if errors.Is(err, sql.ErrNoRows) {
		return false, nil
	}
	if err != nil {
		return false, fmt.Errorf("store appuser email reset code: %w", err)
	}
	return true, nil
}

// ResetPasswordByEmail verifies and consumes the latest email code atomically.
func (s *Store) ResetPasswordByEmail(ctx context.Context, email, codeHash, password string) error {
	if s == nil || s.db == nil {
		return errors.New("appuser store unavailable")
	}
	email = NormalizeEmail(email)
	if err := ValidateEmail(email); err != nil {
		return err
	}
	if err := ValidatePassword(password); err != nil {
		return err
	}
	passwordHash, err := bcrypt.GenerateFromPassword([]byte(password), bcrypt.DefaultCost)
	if err != nil {
		return fmt.Errorf("hash appuser email reset password: %w", err)
	}
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return fmt.Errorf("begin appuser email password reset: %w", err)
	}
	defer tx.Rollback()

	var userID int64
	if err := tx.QueryRowContext(ctx, `
		SELECT id
		FROM app_users
		WHERE lower(email) = lower($1)
		  AND status = 'active'
		  AND password_hash IS NOT NULL
		  AND btrim(password_hash) <> ''
		FOR UPDATE
	`, email).Scan(&userID); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return ErrInvalidCredentials
		}
		return fmt.Errorf("lock appuser for email password reset: %w", err)
	}

	var codeID int64
	if err := tx.QueryRowContext(ctx, `
		SELECT id
		FROM app_email_reset_codes
		WHERE email = $1
		  AND code_hash = $2
		  AND used = false
		  AND expires_at > now()
		ORDER BY create_time DESC, id DESC
		LIMIT 1
		FOR UPDATE
	`, email, codeHash).Scan(&codeID); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return ErrInvalidCredentials
		}
		return fmt.Errorf("lock appuser email reset code: %w", err)
	}

	result, err := tx.ExecContext(ctx, `
		UPDATE app_email_reset_codes
		SET used = true
		WHERE id = $1 AND used = false
	`, codeID)
	if err != nil {
		return fmt.Errorf("consume appuser email reset code: %w", err)
	}
	affected, err := result.RowsAffected()
	if err != nil {
		return fmt.Errorf("read appuser email reset code result: %w", err)
	}
	if affected != 1 {
		return ErrInvalidCredentials
	}

	if _, err := tx.ExecContext(ctx, `
		UPDATE app_users
		SET password_hash = $2, update_time = now()
		WHERE id = $1
	`, userID, string(passwordHash)); err != nil {
		return fmt.Errorf("update appuser email reset password: %w", err)
	}
	if _, err := tx.ExecContext(ctx, `
		UPDATE app_refresh_tokens
		SET revoked = true
		WHERE app_user_id = $1 AND revoked = false
	`, userID); err != nil {
		return fmt.Errorf("revoke appuser sessions after email password reset: %w", err)
	}
	if err := tx.Commit(); err != nil {
		return fmt.Errorf("commit appuser email password reset: %w", err)
	}
	return nil
}

func isEmailIdentifier(value string) bool {
	return ValidateEmail(strings.TrimSpace(value)) == nil
}
