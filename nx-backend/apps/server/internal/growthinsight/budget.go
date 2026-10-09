package growthinsight

import (
	"context"
	"database/sql"
	"errors"
	"time"
)

const (
	DefaultDailyAttemptLimit     = 100
	DefaultUserDailyAttemptLimit = 3
)

type BudgetLimits struct {
	DailyAttempts     int
	UserDailyAttempts int
}

// A committed reservation is never refunded, including provider errors and later consent withdrawal.
func (s *Store) reserveAttempt(ctx context.Context, j job, now time.Time) error {
	if s.budgetLimits.DailyAttempts <= 0 || s.budgetLimits.UserDailyAttempts <= 0 {
		return ErrInvalid
	}
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()
	cardID, revision, err := authorizedTx(ctx, tx, j.userID)
	if errors.Is(err, ErrDisabled) || errors.Is(err, ErrNotFound) {
		return ErrStale
	}
	if err != nil {
		return err
	}
	var token string
	var lease time.Time
	err = tx.QueryRowContext(ctx, `SELECT claim_token,lease_until FROM app_growth_insight_jobs WHERE app_user_id=$1 AND status='analyzing' FOR UPDATE`, j.userID).Scan(&token, &lease)
	if errors.Is(err, sql.ErrNoRows) {
		return ErrStale
	}
	if err != nil {
		return err
	}
	if token != j.token || cardID != j.cardID || revision != j.revision {
		return ErrStale
	}
	// One shared lock also covers reservations that wait across UTC midnight.
	// It is released before any model call.
	if _, err = tx.ExecContext(ctx, `SELECT pg_advisory_xact_lock(hashtextextended('growth_insight_budget:'||current_schema(),0))`); err != nil {
		return err
	}
	if !j.claimedAt.IsZero() {
		now = now.Add(time.Since(j.claimedAt))
	}
	if !lease.After(now) {
		return ErrStale
	}
	dayStart := budgetDayStart(now)
	day := dayStart.Format("2006-01-02")
	if _, err = tx.ExecContext(ctx, `DELETE FROM app_growth_insight_attempts WHERE budget_day<$1::date`, dayStart.AddDate(0, 0, -6).Format("2006-01-02")); err != nil {
		return err
	}
	var exists bool
	if err = tx.QueryRowContext(ctx, `SELECT EXISTS(SELECT 1 FROM app_growth_insight_attempts WHERE claim_token=$1 AND app_user_id=$2)`, j.token, j.userID).Scan(&exists); err != nil {
		return err
	}
	if exists {
		return tx.Commit()
	}
	var total, user int
	if err = tx.QueryRowContext(ctx, `SELECT count(*),count(*) FILTER(WHERE app_user_id=$2) FROM app_growth_insight_attempts WHERE budget_day=$1::date`, day, j.userID).Scan(&total, &user); err != nil {
		return err
	}
	code := ""
	if user >= s.budgetLimits.UserDailyAttempts {
		code = "user_daily_budget_exhausted"
	}
	if total >= s.budgetLimits.DailyAttempts {
		code = "global_daily_budget_exhausted"
	}
	if code != "" {
		if _, err = tx.ExecContext(ctx, `UPDATE app_growth_insight_jobs SET status='pending',claim_token='',lease_until=NULL,error_code=$3,due_at=$4,attempts=greatest(attempts-1,0),updated_at=$5 WHERE app_user_id=$1 AND claim_token=$2`, j.userID, j.token, code, dayStart.AddDate(0, 0, 1), now); err != nil {
			return err
		}
		if _, err = tx.ExecContext(ctx, `UPDATE app_growth_insight_consents SET status='pending' WHERE app_user_id=$1`, j.userID); err != nil {
			return err
		}
		if err = tx.Commit(); err != nil {
			return err
		}
		return ErrBudgetExhausted
	}
	if _, err = tx.ExecContext(ctx, `INSERT INTO app_growth_insight_attempts(claim_token,app_user_id,budget_day,reserved_at) VALUES($1,$2,$3::date,$4)`, j.token, j.userID, day, now); err != nil {
		return err
	}
	return tx.Commit()
}

func budgetDayStart(now time.Time) time.Time {
	utc := now.UTC()
	return time.Date(utc.Year(), utc.Month(), utc.Day(), 0, 0, 0, 0, time.UTC)
}

func (s *Store) pruneAttempts(ctx context.Context, now time.Time) error {
	_, err := s.db.ExecContext(ctx, `DELETE FROM app_growth_insight_attempts WHERE budget_day<$1::date`, budgetDayStart(now).AddDate(0, 0, -6).Format("2006-01-02"))
	return err
}
