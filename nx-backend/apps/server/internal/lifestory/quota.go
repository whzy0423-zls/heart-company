package lifestory

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"strings"
	"time"
)

const DefaultQuotaPeriod = "current_month"

const firstGenerationGrantKey = "grant:first_generation"

type QuotaSnapshot struct {
	PeriodKey string `json:"periodKey"`
	Limit     int    `json:"limit"`
	Reserved  int    `json:"reserved"`
	Consumed  int    `json:"consumed"`
	Remaining int    `json:"remaining"`
}

type QuotaStore struct{ db *sql.DB }

func NewQuotaStore(db *sql.DB) *QuotaStore { return &QuotaStore{db: db} }

func (s *Store) QuotaStore() *QuotaStore {
	if s == nil {
		return &QuotaStore{}
	}
	return &QuotaStore{db: s.db}
}

func quotaPeriod(period string) string {
	period = strings.TrimSpace(period)
	if period == "" || period == DefaultQuotaPeriod {
		return time.Now().UTC().Format("2006-01")
	}
	if len([]rune(period)) > 64 {
		return string([]rune(period)[:64])
	}
	return period
}

func (s *QuotaStore) ensureDB() error {
	if s == nil || s.db == nil {
		return ErrNilDB
	}
	return nil
}

type quotaTX interface {
	ExecContext(context.Context, string, ...any) (sql.Result, error)
	QueryRowContext(context.Context, string, ...any) *sql.Row
}

func ensureQuotaPeriod(ctx context.Context, tx quotaTX, appUserID int64, period string) (int64, error) {
	period = quotaPeriod(period)
	var id int64
	err := tx.QueryRowContext(ctx, `INSERT INTO app_story_quota_periods
 (app_user_id,period_key,quota_limit) VALUES($1,$2,0)
 ON CONFLICT(app_user_id,period_key) DO UPDATE SET updated_at=now()
 RETURNING id`, appUserID, period).Scan(&id)
	return id, err
}

// ensureFirstGenerationGrantTx inserts the lifetime grant before increasing
// the period limit. Only the transaction that inserted the unique ledger row
// is allowed to increase the limit, so concurrent first requests cannot double
// grant the account.
func ensureFirstGenerationGrantTx(ctx context.Context, tx quotaTX, appUserID, periodID int64) error {
	result, err := tx.ExecContext(ctx, `INSERT INTO app_story_quota_ledger
 (app_user_id,period_id,entry_type,amount,idempotency_key)
 VALUES($1,$2,'grant',1,$3) ON CONFLICT(app_user_id,idempotency_key) DO NOTHING`, appUserID, periodID, firstGenerationGrantKey)
	if err != nil {
		return err
	}
	inserted, _ := result.RowsAffected()
	if inserted != 1 {
		return nil
	}
	_, err = tx.ExecContext(ctx, `UPDATE app_story_quota_periods
 SET quota_limit=quota_limit+1,updated_at=now() WHERE id=$1`, periodID)
	return err
}

func quotaSnapshotByID(ctx context.Context, tx quotaTX, appUserID, periodID int64) (QuotaSnapshot, error) {
	var q QuotaSnapshot
	err := tx.QueryRowContext(ctx, `SELECT period_key,quota_limit,reserved,consumed
 FROM app_story_quota_periods WHERE id=$1 AND app_user_id=$2`, periodID, appUserID).
		Scan(&q.PeriodKey, &q.Limit, &q.Reserved, &q.Consumed)
	if err != nil {
		return QuotaSnapshot{}, err
	}
	q.Remaining = q.Limit - q.Reserved - q.Consumed
	if q.Remaining < 0 {
		q.Remaining = 0
	}
	return q, nil
}

func quotaSnapshotTx(ctx context.Context, tx quotaTX, appUserID int64, period string) (QuotaSnapshot, error) {
	period = quotaPeriod(period)
	var q QuotaSnapshot
	q.PeriodKey = period
	err := tx.QueryRowContext(ctx, `SELECT quota_limit,reserved,consumed
 FROM app_story_quota_periods WHERE app_user_id=$1 AND period_key=$2`, appUserID, period).
		Scan(&q.Limit, &q.Reserved, &q.Consumed)
	if err != nil {
		return QuotaSnapshot{}, err
	}
	q.Remaining = q.Limit - q.Reserved - q.Consumed
	if q.Remaining < 0 {
		q.Remaining = 0
	}
	return q, nil
}

func firstGrantExists(ctx context.Context, tx quotaTX, appUserID int64) (bool, error) {
	var value int
	err := tx.QueryRowContext(ctx, `SELECT 1 FROM app_story_quota_ledger WHERE app_user_id=$1 AND idempotency_key=$2`, appUserID, firstGenerationGrantKey).Scan(&value)
	if errors.Is(err, sql.ErrNoRows) {
		return false, nil
	}
	return err == nil, err
}

// Reserve is the transaction primitive used by job creation and worker
// recovery. It records the period on the reservation row and always returns
// that period, even if the caller later passes a different current month.
func reserveQuotaTx(ctx context.Context, tx quotaTX, appUserID, jobID int64, period string) (QuotaSnapshot, error) {
	var jobUser int64
	var jobStatus JobStatus
	if err := tx.QueryRowContext(ctx, `SELECT app_user_id,status FROM app_life_story_jobs WHERE id=$1 FOR UPDATE`, jobID).Scan(&jobUser, &jobStatus); errors.Is(err, sql.ErrNoRows) {
		return QuotaSnapshot{}, ErrNotFound
	} else if err != nil {
		return QuotaSnapshot{}, err
	}
	if jobUser != appUserID {
		return QuotaSnapshot{}, ErrConflict
	}
	var existingPeriod int64
	err := tx.QueryRowContext(ctx, `SELECT period_id FROM app_story_quota_ledger WHERE app_user_id=$1 AND job_id=$2 AND entry_type='reserve' FOR UPDATE`, appUserID, jobID).Scan(&existingPeriod)
	if err == nil {
		return quotaSnapshotByID(ctx, tx, appUserID, existingPeriod)
	}
	if !errors.Is(err, sql.ErrNoRows) {
		return QuotaSnapshot{}, err
	}
	if jobStatus != JobQueued && jobStatus != JobRunning {
		return QuotaSnapshot{}, ErrInvalidState
	}
	period = quotaPeriod(period)
	periodID, err := ensureQuotaPeriod(ctx, tx, appUserID, period)
	if err != nil {
		return QuotaSnapshot{}, err
	}
	if err := ensureFirstGenerationGrantTx(ctx, tx, appUserID, periodID); err != nil {
		return QuotaSnapshot{}, err
	}
	var available int
	if err := tx.QueryRowContext(ctx, `UPDATE app_story_quota_periods
 SET reserved=reserved+1,updated_at=now()
 WHERE id=$1 AND reserved+consumed < quota_limit RETURNING reserved`, periodID).Scan(&available); errors.Is(err, sql.ErrNoRows) {
		return QuotaSnapshot{}, ErrQuotaExhausted
	} else if err != nil {
		return QuotaSnapshot{}, err
	}
	if _, err := tx.ExecContext(ctx, `INSERT INTO app_story_quota_ledger
 (app_user_id,period_id,job_id,entry_type,amount,idempotency_key)
 VALUES($1,$2,$3,'reserve',1,$4)`, appUserID, periodID, jobID, fmt.Sprintf("story:%d:reserve", jobID)); err != nil {
		return QuotaSnapshot{}, err
	}
	return quotaSnapshotByID(ctx, tx, appUserID, periodID)
}

func (s *QuotaStore) Snapshot(ctx context.Context, appUserID int64, period string) (QuotaSnapshot, error) {
	if err := s.ensureDB(); err != nil {
		return QuotaSnapshot{}, err
	}
	period = quotaPeriod(period)
	var q QuotaSnapshot
	q.PeriodKey = period
	err := s.db.QueryRowContext(ctx, `SELECT quota_limit,reserved,consumed
 FROM app_story_quota_periods WHERE app_user_id=$1 AND period_key=$2`, appUserID, period).
		Scan(&q.Limit, &q.Reserved, &q.Consumed)
	if errors.Is(err, sql.ErrNoRows) {
		granted, grantErr := firstGrantExists(ctx, s.db, appUserID)
		if grantErr != nil {
			return QuotaSnapshot{}, grantErr
		}
		if !granted {
			// The first generation grant is intentionally visible before the
			// first job transaction creates the period row.
			q.Limit, q.Remaining = 1, 1
		}
		return q, nil
	}
	if err != nil {
		return QuotaSnapshot{}, err
	}
	q.Remaining = q.Limit - q.Reserved - q.Consumed
	if q.Remaining < 0 {
		q.Remaining = 0
	}
	return q, nil
}

func (s *QuotaStore) Reserve(ctx context.Context, appUserID, jobID int64, period string) (QuotaSnapshot, error) {
	if err := s.ensureDB(); err != nil {
		return QuotaSnapshot{}, err
	}
	if appUserID <= 0 || jobID <= 0 {
		return QuotaSnapshot{}, fmt.Errorf("invalid quota identifiers")
	}
	tx, err := s.db.BeginTx(ctx, &sql.TxOptions{Isolation: sql.LevelSerializable})
	if err != nil {
		return QuotaSnapshot{}, err
	}
	defer tx.Rollback()
	// A user lock serializes reserve with account deletion and other jobs.
	var status string
	if err := tx.QueryRowContext(ctx, `SELECT status FROM app_users WHERE id=$1 FOR UPDATE`, appUserID).Scan(&status); errors.Is(err, sql.ErrNoRows) {
		return QuotaSnapshot{}, ErrNotFound
	} else if err != nil {
		return QuotaSnapshot{}, err
	}
	if strings.TrimSpace(status) != "active" {
		return QuotaSnapshot{}, ErrInactiveUser
	}
	q, err := reserveQuotaTx(ctx, tx, appUserID, jobID, period)
	if err != nil {
		return QuotaSnapshot{}, err
	}
	if err := tx.Commit(); err != nil {
		return QuotaSnapshot{}, err
	}
	return q, nil
}

// commitQuotaTx consumes the exact period recorded by the reservation. The
// caller must hold the user and job locks (FinalizeJob does); the helper also
// works as an idempotent primitive for retries.
func commitQuotaTx(ctx context.Context, tx quotaTX, appUserID, jobID, versionID int64) (QuotaSnapshot, error) {
	var periodID int64
	err := tx.QueryRowContext(ctx, `SELECT period_id FROM app_story_quota_ledger
 WHERE app_user_id=$1 AND job_id=$2 AND entry_type='reserve' FOR UPDATE`, appUserID, jobID).Scan(&periodID)
	if errors.Is(err, sql.ErrNoRows) {
		var committedPeriod int64
		if scanErr := tx.QueryRowContext(ctx, `SELECT period_id FROM app_story_quota_ledger
 WHERE app_user_id=$1 AND job_id=$2 AND entry_type='commit'`, appUserID, jobID).Scan(&committedPeriod); scanErr == nil {
			return quotaSnapshotByID(ctx, tx, appUserID, committedPeriod)
		}
		return QuotaSnapshot{}, ErrInvalidState
	}
	if err != nil {
		return QuotaSnapshot{}, err
	}
	var done int
	if err := tx.QueryRowContext(ctx, `SELECT 1 FROM app_story_quota_ledger
 WHERE app_user_id=$1 AND job_id=$2 AND entry_type='release'`, appUserID, jobID).Scan(&done); err == nil {
		return QuotaSnapshot{}, ErrInvalidState
	} else if !errors.Is(err, sql.ErrNoRows) {
		return QuotaSnapshot{}, err
	}
	result, err := tx.ExecContext(ctx, `INSERT INTO app_story_quota_ledger
 (app_user_id,period_id,job_id,version_id,entry_type,amount,idempotency_key)
 VALUES($1,$2,$3,$4,'commit',1,$5) ON CONFLICT(app_user_id,idempotency_key) DO NOTHING`,
		appUserID, periodID, jobID, versionID, fmt.Sprintf("story:%d:commit", jobID))
	if err != nil {
		return QuotaSnapshot{}, err
	}
	inserted, _ := result.RowsAffected()
	if inserted == 1 {
		result, err = tx.ExecContext(ctx, `UPDATE app_story_quota_periods
 SET reserved=reserved-1,consumed=consumed+1,updated_at=now()
 WHERE id=$1 AND app_user_id=$2 AND reserved > 0`, periodID, appUserID)
		if err != nil {
			return QuotaSnapshot{}, err
		}
		updated, _ := result.RowsAffected()
		if updated != 1 {
			return QuotaSnapshot{}, ErrInvalidState
		}
	}
	return quotaSnapshotByID(ctx, tx, appUserID, periodID)
}

// releaseQuotaTx returns a reservation to its original period. It does not
// create a quota period for a missing reservation, which is important during
// account deletion and late worker callbacks.
func releaseQuotaTx(ctx context.Context, tx quotaTX, appUserID, jobID int64) (QuotaSnapshot, bool, error) {
	var periodID int64
	err := tx.QueryRowContext(ctx, `SELECT period_id FROM app_story_quota_ledger
 WHERE app_user_id=$1 AND job_id=$2 AND entry_type='reserve' FOR UPDATE`, appUserID, jobID).Scan(&periodID)
	if errors.Is(err, sql.ErrNoRows) {
		var finishedPeriod int64
		if scanErr := tx.QueryRowContext(ctx, `SELECT period_id FROM app_story_quota_ledger
 WHERE app_user_id=$1 AND job_id=$2 AND entry_type IN ('release','commit') ORDER BY id DESC LIMIT 1`, appUserID, jobID).Scan(&finishedPeriod); scanErr == nil {
			q, qErr := quotaSnapshotByID(ctx, tx, appUserID, finishedPeriod)
			return q, false, qErr
		}
		return QuotaSnapshot{}, false, nil
	}
	if err != nil {
		return QuotaSnapshot{}, false, err
	}
	var done int
	if err := tx.QueryRowContext(ctx, `SELECT 1 FROM app_story_quota_ledger
 WHERE app_user_id=$1 AND job_id=$2 AND entry_type IN ('release','commit')`, appUserID, jobID).Scan(&done); err == nil {
		q, qErr := quotaSnapshotByID(ctx, tx, appUserID, periodID)
		return q, false, qErr
	} else if !errors.Is(err, sql.ErrNoRows) {
		return QuotaSnapshot{}, false, err
	}
	result, err := tx.ExecContext(ctx, `INSERT INTO app_story_quota_ledger
 (app_user_id,period_id,job_id,entry_type,amount,idempotency_key)
 VALUES($1,$2,$3,'release',1,$4) ON CONFLICT(app_user_id,idempotency_key) DO NOTHING`,
		appUserID, periodID, jobID, fmt.Sprintf("story:%d:release", jobID))
	if err != nil {
		return QuotaSnapshot{}, false, err
	}
	inserted, _ := result.RowsAffected()
	if inserted == 1 {
		result, err = tx.ExecContext(ctx, `UPDATE app_story_quota_periods
 SET reserved=reserved-1,updated_at=now() WHERE id=$1 AND app_user_id=$2 AND reserved > 0`, periodID, appUserID)
		if err != nil {
			return QuotaSnapshot{}, false, err
		}
		updated, _ := result.RowsAffected()
		if updated != 1 {
			return QuotaSnapshot{}, false, ErrInvalidState
		}
	}
	q, err := quotaSnapshotByID(ctx, tx, appUserID, periodID)
	return q, inserted == 1, err
}

// Commit consumes the exact period stored on the reservation ledger. It never
// creates a new period for a stale/unknown job.
func (s *QuotaStore) Commit(ctx context.Context, appUserID, jobID, versionID int64, _ string) (QuotaSnapshot, error) {
	if err := s.ensureDB(); err != nil {
		return QuotaSnapshot{}, err
	}
	tx, err := s.db.BeginTx(ctx, &sql.TxOptions{Isolation: sql.LevelSerializable})
	if err != nil {
		return QuotaSnapshot{}, err
	}
	defer tx.Rollback()
	var userStatus string
	if err := tx.QueryRowContext(ctx, `SELECT status FROM app_users WHERE id=$1 FOR UPDATE`, appUserID).Scan(&userStatus); errors.Is(err, sql.ErrNoRows) {
		return QuotaSnapshot{}, ErrNotFound
	} else if err != nil {
		return QuotaSnapshot{}, err
	}
	if strings.TrimSpace(userStatus) != "active" {
		return QuotaSnapshot{}, ErrInactiveUser
	}
	var jobUser int64
	if err := tx.QueryRowContext(ctx, `SELECT app_user_id FROM app_life_story_jobs WHERE id=$1 FOR UPDATE`, jobID).Scan(&jobUser); errors.Is(err, sql.ErrNoRows) {
		return QuotaSnapshot{}, ErrNotFound
	} else if err != nil {
		return QuotaSnapshot{}, err
	}
	if jobUser != appUserID {
		return QuotaSnapshot{}, ErrConflict
	}
	q, err := commitQuotaTx(ctx, tx, appUserID, jobID, versionID)
	if err != nil {
		return QuotaSnapshot{}, err
	}
	if err := tx.Commit(); err != nil {
		return QuotaSnapshot{}, err
	}
	return q, nil
}

// Release returns a reservation to its original period. Missing reservations
// are a no-op, which prevents cancellation/deletion from creating phantom
// release ledger rows.
func (s *QuotaStore) Release(ctx context.Context, appUserID, jobID int64, period string) (QuotaSnapshot, error) {
	if err := s.ensureDB(); err != nil {
		return QuotaSnapshot{}, err
	}
	tx, err := s.db.BeginTx(ctx, &sql.TxOptions{Isolation: sql.LevelSerializable})
	if err != nil {
		return QuotaSnapshot{}, err
	}
	defer tx.Rollback()
	var userStatus string
	if err := tx.QueryRowContext(ctx, `SELECT status FROM app_users WHERE id=$1 FOR UPDATE`, appUserID).Scan(&userStatus); errors.Is(err, sql.ErrNoRows) {
		return QuotaSnapshot{}, ErrNotFound
	} else if err != nil {
		return QuotaSnapshot{}, err
	}
	var hasReservation int
	reservationErr := tx.QueryRowContext(ctx, `SELECT 1 FROM app_story_quota_ledger
 WHERE app_user_id=$1 AND job_id=$2 AND entry_type='reserve'`, appUserID, jobID).Scan(&hasReservation)
	if errors.Is(reservationErr, sql.ErrNoRows) && strings.TrimSpace(userStatus) != "active" {
		return QuotaSnapshot{}, ErrInactiveUser
	}
	q, _, err := releaseQuotaTx(ctx, tx, appUserID, jobID)
	if err != nil {
		return QuotaSnapshot{}, err
	}
	if err := tx.Commit(); err != nil {
		return QuotaSnapshot{}, err
	}
	if q.PeriodKey == "" && errors.Is(reservationErr, sql.ErrNoRows) {
		// Return the caller's current snapshot only after the transaction is
		// closed; no period row is created by this fallback.
		return s.Snapshot(ctx, appUserID, period)
	}
	return q, nil
}

// Grant adds quota explicitly (for support or test fixtures). Its key is
// user-scoped and idempotent.
func (s *QuotaStore) Grant(ctx context.Context, appUserID int64, amount int, period string, key string) (QuotaSnapshot, error) {
	if err := s.ensureDB(); err != nil {
		return QuotaSnapshot{}, err
	}
	if amount <= 0 {
		return QuotaSnapshot{}, fmt.Errorf("grant amount must be positive")
	}
	period = quotaPeriod(period)
	key = strings.TrimSpace(key)
	if key == "" {
		key = fmt.Sprintf("grant:%d:%d", appUserID, time.Now().UnixNano())
	}
	tx, err := s.db.BeginTx(ctx, &sql.TxOptions{Isolation: sql.LevelSerializable})
	if err != nil {
		return QuotaSnapshot{}, err
	}
	defer tx.Rollback()
	var status string
	if err := tx.QueryRowContext(ctx, `SELECT status FROM app_users WHERE id=$1 FOR UPDATE`, appUserID).Scan(&status); errors.Is(err, sql.ErrNoRows) {
		return QuotaSnapshot{}, ErrNotFound
	} else if err != nil {
		return QuotaSnapshot{}, err
	}
	if strings.TrimSpace(status) != "active" {
		return QuotaSnapshot{}, ErrInactiveUser
	}
	periodID, err := ensureQuotaPeriod(ctx, tx, appUserID, period)
	if err != nil {
		return QuotaSnapshot{}, err
	}
	var exists int
	if err := tx.QueryRowContext(ctx, `SELECT 1 FROM app_story_quota_ledger WHERE app_user_id=$1 AND idempotency_key=$2`, appUserID, key).Scan(&exists); err == nil {
		q, qErr := quotaSnapshotByID(ctx, tx, appUserID, periodID)
		if qErr != nil {
			return QuotaSnapshot{}, qErr
		}
		if err := tx.Commit(); err != nil {
			return QuotaSnapshot{}, err
		}
		return q, nil
	} else if !errors.Is(err, sql.ErrNoRows) {
		return QuotaSnapshot{}, err
	}
	if _, err := tx.ExecContext(ctx, `UPDATE app_story_quota_periods SET quota_limit=quota_limit+$1,updated_at=now() WHERE id=$2`, amount, periodID); err != nil {
		return QuotaSnapshot{}, err
	}
	if _, err := tx.ExecContext(ctx, `INSERT INTO app_story_quota_ledger(app_user_id,period_id,entry_type,amount,idempotency_key) VALUES($1,$2,'grant',$3,$4)`, appUserID, periodID, amount, key); err != nil {
		return QuotaSnapshot{}, err
	}
	q, err := quotaSnapshotByID(ctx, tx, appUserID, periodID)
	if err != nil {
		return QuotaSnapshot{}, err
	}
	if err := tx.Commit(); err != nil {
		return QuotaSnapshot{}, err
	}
	return q, nil
}
