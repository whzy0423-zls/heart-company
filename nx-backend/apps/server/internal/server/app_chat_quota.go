package server

import (
	"context"
	"crypto/rand"
	"database/sql"
	"encoding/hex"
	"errors"
	"fmt"
	"time"
)

var errAppChatQuotaExhausted = errors.New("app chat daily quota exhausted")

type appChatQuotaSnapshot struct {
	Limit                 int    `json:"limit"`
	Reserved              int    `json:"reserved"`
	Consumed              int    `json:"consumed"`
	Remaining             int    `json:"remaining"`
	TrialRemaining        int    `json:"trialRemaining"`
	TotalRemaining        int    `json:"totalRemaining"`
	TrialNearestExpiresAt string `json:"trialNearestExpiresAt,omitempty"`
}

func newAppChatQuotaSnapshot(limit, reserved, consumed int) appChatQuotaSnapshot {
	if limit < 0 {
		return appChatQuotaSnapshot{Limit: -1, Remaining: -1, TotalRemaining: -1}
	}
	remaining := limit - reserved - consumed
	if remaining < 0 {
		remaining = 0
	}
	return appChatQuotaSnapshot{Limit: limit, Reserved: reserved, Consumed: consumed, Remaining: remaining, TotalRemaining: remaining}
}

func newAppChatQuotaSnapshotWithTrial(limit, reserved, consumed, trialRemaining int, nearestExpiry string) appChatQuotaSnapshot {
	snapshot := newAppChatQuotaSnapshot(limit, reserved, consumed)
	snapshot.TrialRemaining = max(trialRemaining, 0)
	snapshot.TrialNearestExpiresAt = nearestExpiry
	if snapshot.Remaining >= 0 {
		snapshot.TotalRemaining = snapshot.Remaining + snapshot.TrialRemaining
	}
	return snapshot
}

func appChatQuotaDate(now time.Time) string {
	location, err := time.LoadLocation("Asia/Shanghai")
	if err != nil {
		location = time.FixedZone("Asia/Shanghai", 8*60*60)
	}
	return now.In(location).Format("2006-01-02")
}

type appChatQuotaManager interface {
	Reserve(context.Context, int64, int, time.Time) (string, appChatQuotaSnapshot, error)
	Commit(context.Context, string) (appChatQuotaSnapshot, error)
	Release(context.Context, string) (appChatQuotaSnapshot, error)
	Snapshot(context.Context, int64, int, time.Time) (appChatQuotaSnapshot, error)
}

type databaseAppChatQuotaManager struct{ db *sql.DB }

func newDatabaseAppChatQuotaManager(db *sql.DB) appChatQuotaManager {
	if db == nil {
		return nil
	}
	return &databaseAppChatQuotaManager{db: db}
}

func newAppChatQuotaReservationKey() (string, error) {
	var value [16]byte
	if _, err := rand.Read(value[:]); err != nil {
		return "", err
	}
	return hex.EncodeToString(value[:]), nil
}

func (m *databaseAppChatQuotaManager) Reserve(ctx context.Context, userID int64, limit int, now time.Time) (string, appChatQuotaSnapshot, error) {
	if limit < 0 {
		return "", newAppChatQuotaSnapshot(-1, 0, 0), nil
	}
	if m == nil || m.db == nil || userID <= 0 {
		return "", newAppChatQuotaSnapshot(max(limit, 0), 0, 0), errAppChatQuotaExhausted
	}
	key, err := newAppChatQuotaReservationKey()
	if err != nil {
		return "", appChatQuotaSnapshot{}, err
	}
	date := appChatQuotaDate(now)
	tx, err := m.db.BeginTx(ctx, &sql.TxOptions{Isolation: sql.LevelSerializable})
	if err != nil {
		return "", appChatQuotaSnapshot{}, err
	}
	defer tx.Rollback()

	if _, err := tx.ExecContext(ctx, `INSERT INTO app_chat_daily_quotas(app_user_id,quota_date,quota_limit)
		VALUES($1,$2::date,$3) ON CONFLICT(app_user_id,quota_date) DO UPDATE
		SET quota_limit=GREATEST(EXCLUDED.quota_limit,app_chat_daily_quotas.reserved+app_chat_daily_quotas.consumed),update_time=now()`, userID, date, limit); err != nil {
		return "", appChatQuotaSnapshot{}, err
	}
	if _, err := tx.ExecContext(ctx, `UPDATE app_chat_trial_credit_grants
		SET status='expired',remaining=reserved,update_time=now()
		WHERE app_user_id=$1 AND status='active' AND expires_at <= $2`, userID, now); err != nil {
		return "", appChatQuotaSnapshot{}, err
	}
	if err := releaseStaleAppChatReservations(ctx, tx, userID, date); err != nil {
		return "", appChatQuotaSnapshot{}, err
	}
	var quotaLimit, reserved, consumed int
	err = tx.QueryRowContext(ctx, `UPDATE app_chat_daily_quotas SET reserved=reserved+1,update_time=now()
		WHERE app_user_id=$1 AND quota_date=$2::date AND reserved+consumed < quota_limit
		RETURNING quota_limit,reserved,consumed`, userID, date).Scan(&quotaLimit, &reserved, &consumed)
	if err == nil {
		if _, err := tx.ExecContext(ctx, `INSERT INTO app_chat_quota_reservations(reservation_key,app_user_id,quota_date,source,status)
			VALUES($1,$2,$3::date,'daily','reserved')`, key, userID, date); err != nil {
			return "", appChatQuotaSnapshot{}, err
		}
		snapshot, err := appChatQuotaSnapshotInTx(ctx, tx, userID, date, limit, now)
		if err != nil {
			return "", appChatQuotaSnapshot{}, err
		}
		if err := tx.Commit(); err != nil {
			return "", appChatQuotaSnapshot{}, err
		}
		return key, snapshot, nil
	}
	if !errors.Is(err, sql.ErrNoRows) {
		return "", appChatQuotaSnapshot{}, err
	}

	var grantID int64
	err = tx.QueryRowContext(ctx, `WITH candidate AS (
		SELECT id FROM app_chat_trial_credit_grants
		WHERE app_user_id=$1 AND status='active' AND expires_at>$2 AND remaining>reserved
		ORDER BY expires_at,id FOR UPDATE SKIP LOCKED LIMIT 1)
		UPDATE app_chat_trial_credit_grants g SET reserved=g.reserved+1,update_time=now()
		FROM candidate WHERE g.id=candidate.id RETURNING g.id`, userID, now).Scan(&grantID)
	if errors.Is(err, sql.ErrNoRows) {
		snapshot, snapshotErr := appChatQuotaSnapshotInTx(ctx, tx, userID, date, limit, now)
		if snapshotErr != nil {
			return "", appChatQuotaSnapshot{}, snapshotErr
		}
		if err := tx.Commit(); err != nil {
			return "", appChatQuotaSnapshot{}, err
		}
		return "", snapshot, errAppChatQuotaExhausted
	}
	if err != nil {
		return "", appChatQuotaSnapshot{}, err
	}
	if _, err := tx.ExecContext(ctx, `INSERT INTO app_chat_quota_reservations(reservation_key,app_user_id,quota_date,source,trial_grant_id,status)
		VALUES($1,$2,$3::date,'trial',$4,'reserved')`, key, userID, date, grantID); err != nil {
		return "", appChatQuotaSnapshot{}, err
	}
	snapshot, err := appChatQuotaSnapshotInTx(ctx, tx, userID, date, limit, now)
	if err != nil {
		return "", appChatQuotaSnapshot{}, err
	}
	if err := tx.Commit(); err != nil {
		return "", appChatQuotaSnapshot{}, err
	}
	return key, snapshot, nil
}

func releaseStaleAppChatReservations(ctx context.Context, tx *sql.Tx, userID int64, date string) error {
	rows, err := tx.QueryContext(ctx, `UPDATE app_chat_quota_reservations SET status='released',update_time=now()
		WHERE app_user_id=$1 AND (quota_date=$2::date OR source='trial') AND status='reserved' AND create_time < now()-interval '10 minutes'
		RETURNING source,trial_grant_id`, userID, date)
	if err != nil {
		return err
	}
	defer rows.Close()
	dailyCount := 0
	grantCounts := map[int64]int{}
	for rows.Next() {
		var source string
		var grantID sql.NullInt64
		if err := rows.Scan(&source, &grantID); err != nil {
			return err
		}
		if source == "trial" && grantID.Valid {
			grantCounts[grantID.Int64]++
		} else {
			dailyCount++
		}
	}
	if err := rows.Err(); err != nil {
		return err
	}
	if dailyCount > 0 {
		if _, err := tx.ExecContext(ctx, `UPDATE app_chat_daily_quotas SET reserved=GREATEST(0,reserved-$3),update_time=now()
			WHERE app_user_id=$1 AND quota_date=$2::date`, userID, date, dailyCount); err != nil {
			return err
		}
	}
	for grantID, count := range grantCounts {
		if _, err := tx.ExecContext(ctx, `UPDATE app_chat_trial_credit_grants
			SET remaining=CASE WHEN status='active' AND expires_at>now() THEN remaining ELSE GREATEST(0,reserved-$2) END,
				reserved=GREATEST(0,reserved-$2),update_time=now()
			WHERE id=$1`, grantID, count); err != nil {
			return err
		}
	}
	return nil
}

func (m *databaseAppChatQuotaManager) Commit(ctx context.Context, key string) (appChatQuotaSnapshot, error) {
	return m.finish(ctx, key, true)
}

func (m *databaseAppChatQuotaManager) Release(ctx context.Context, key string) (appChatQuotaSnapshot, error) {
	return m.finish(ctx, key, false)
}

func (m *databaseAppChatQuotaManager) finish(ctx context.Context, key string, consume bool) (appChatQuotaSnapshot, error) {
	if key == "" {
		return newAppChatQuotaSnapshot(-1, 0, 0), nil
	}
	if m == nil || m.db == nil {
		return appChatQuotaSnapshot{}, errors.New("chat quota store unavailable")
	}
	tx, err := m.db.BeginTx(ctx, &sql.TxOptions{Isolation: sql.LevelSerializable})
	if err != nil {
		return appChatQuotaSnapshot{}, err
	}
	defer tx.Rollback()
	status := "released"
	if consume {
		status = "consumed"
	}
	var userID int64
	var date time.Time
	var source string
	var grantID sql.NullInt64
	err = tx.QueryRowContext(ctx, `UPDATE app_chat_quota_reservations SET status=$2,update_time=now()
		WHERE reservation_key=$1 AND status='reserved' RETURNING app_user_id,quota_date,source,trial_grant_id`, key, status).Scan(&userID, &date, &source, &grantID)
	if errors.Is(err, sql.ErrNoRows) {
		return m.snapshotForReservation(ctx, tx, key)
	}
	if err != nil {
		return appChatQuotaSnapshot{}, err
	}
	var limit, reserved, consumed int
	if source == "trial" && grantID.Valid {
		if consume {
			_, err = tx.ExecContext(ctx, `UPDATE app_chat_trial_credit_grants
				SET remaining=CASE WHEN status='active' AND expires_at>now() THEN GREATEST(0,remaining-1) ELSE GREATEST(0,reserved-1) END,
					reserved=GREATEST(0,reserved-1),
					status=CASE WHEN status='active' AND expires_at<=now() THEN 'expired' WHEN status='active' AND remaining<=1 THEN 'exhausted' ELSE status END,
					update_time=now() WHERE id=$1`, grantID.Int64)
		} else {
			_, err = tx.ExecContext(ctx, `UPDATE app_chat_trial_credit_grants
				SET remaining=CASE WHEN status='active' AND expires_at>now() THEN remaining ELSE GREATEST(0,reserved-1) END,
					reserved=GREATEST(0,reserved-1),
					status=CASE WHEN status='active' AND expires_at<=now() THEN 'expired' ELSE status END,
					update_time=now() WHERE id=$1`, grantID.Int64)
		}
		if err != nil {
			return appChatQuotaSnapshot{}, err
		}
		if err := tx.QueryRowContext(ctx, `SELECT quota_limit,reserved,consumed FROM app_chat_daily_quotas
			WHERE app_user_id=$1 AND quota_date=$2`, userID, date).Scan(&limit, &reserved, &consumed); err != nil {
			return appChatQuotaSnapshot{}, err
		}
	} else {
		consumedDelta := 0
		if consume {
			consumedDelta = 1
		}
		if err := tx.QueryRowContext(ctx, `UPDATE app_chat_daily_quotas
			SET reserved=GREATEST(0,reserved-1),consumed=consumed+$3,update_time=now()
			WHERE app_user_id=$1 AND quota_date=$2 RETURNING quota_limit,reserved,consumed`, userID, date, consumedDelta).Scan(&limit, &reserved, &consumed); err != nil {
			return appChatQuotaSnapshot{}, err
		}
	}
	snapshot, err := appChatQuotaSnapshotInTx(ctx, tx, userID, date.Format("2006-01-02"), limit, time.Now())
	if err != nil {
		return appChatQuotaSnapshot{}, err
	}
	if err := tx.Commit(); err != nil {
		return appChatQuotaSnapshot{}, err
	}
	return snapshot, nil
}

func (m *databaseAppChatQuotaManager) snapshotForReservation(ctx context.Context, tx *sql.Tx, key string) (appChatQuotaSnapshot, error) {
	var userID int64
	var date time.Time
	var limit, reserved, consumed int
	err := tx.QueryRowContext(ctx, `SELECT r.app_user_id,r.quota_date,q.quota_limit,q.reserved,q.consumed
		FROM app_chat_quota_reservations r JOIN app_chat_daily_quotas q
		ON q.app_user_id=r.app_user_id AND q.quota_date=r.quota_date WHERE r.reservation_key=$1`, key).Scan(&userID, &date, &limit, &reserved, &consumed)
	if err != nil {
		return appChatQuotaSnapshot{}, err
	}
	snapshot, err := appChatQuotaSnapshotInTx(ctx, tx, userID, date.Format("2006-01-02"), limit, time.Now())
	if err != nil {
		return appChatQuotaSnapshot{}, err
	}
	if err := tx.Commit(); err != nil {
		return appChatQuotaSnapshot{}, err
	}
	return snapshot, nil
}

func (m *databaseAppChatQuotaManager) Snapshot(ctx context.Context, userID int64, limit int, now time.Time) (appChatQuotaSnapshot, error) {
	if m == nil || m.db == nil {
		return newAppChatQuotaSnapshot(limit, 0, 0), nil
	}
	var storedLimit, reserved, consumed int
	err := m.db.QueryRowContext(ctx, `SELECT quota_limit,reserved,consumed FROM app_chat_daily_quotas
		WHERE app_user_id=$1 AND quota_date=$2::date`, userID, appChatQuotaDate(now)).Scan(&storedLimit, &reserved, &consumed)
	if errors.Is(err, sql.ErrNoRows) {
		storedLimit = max(limit, 0)
		reserved = 0
		consumed = 0
	} else if err != nil {
		return appChatQuotaSnapshot{}, fmt.Errorf("chat quota snapshot: %w", err)
	}
	if limit < 0 {
		storedLimit = -1
	} else if limit < storedLimit {
		storedLimit = max(limit, reserved+consumed)
	} else if limit > storedLimit {
		storedLimit = limit
	}
	trialRemaining, nearest, err := appChatTrialSnapshot(ctx, m.db, userID, now)
	if err != nil {
		return appChatQuotaSnapshot{}, fmt.Errorf("trial chat quota snapshot: %w", err)
	}
	return newAppChatQuotaSnapshotWithTrial(storedLimit, reserved, consumed, trialRemaining, nearest), nil
}

type appChatQuotaQueryer interface {
	QueryRowContext(context.Context, string, ...any) *sql.Row
}

func appChatTrialSnapshot(ctx context.Context, queryer appChatQuotaQueryer, userID int64, now time.Time) (int, string, error) {
	var remaining int
	var nearest sql.NullTime
	err := queryer.QueryRowContext(ctx, `SELECT COALESCE(SUM(remaining-reserved),0),MIN(expires_at)
		FROM app_chat_trial_credit_grants
		WHERE app_user_id=$1 AND status='active' AND expires_at>$2 AND remaining>reserved`, userID, now).Scan(&remaining, &nearest)
	if err != nil {
		return 0, "", err
	}
	nearestText := ""
	if nearest.Valid {
		nearestText = nearest.Time.Format(time.RFC3339)
	}
	return remaining, nearestText, nil
}

func appChatQuotaSnapshotInTx(ctx context.Context, tx *sql.Tx, userID int64, date string, fallbackLimit int, now time.Time) (appChatQuotaSnapshot, error) {
	var limit, reserved, consumed int
	err := tx.QueryRowContext(ctx, `SELECT quota_limit,reserved,consumed FROM app_chat_daily_quotas
		WHERE app_user_id=$1 AND quota_date=$2::date`, userID, date).Scan(&limit, &reserved, &consumed)
	if errors.Is(err, sql.ErrNoRows) {
		limit = fallbackLimit
	} else if err != nil {
		return appChatQuotaSnapshot{}, err
	}
	trialRemaining, nearest, err := appChatTrialSnapshot(ctx, tx, userID, now)
	if err != nil {
		return appChatQuotaSnapshot{}, err
	}
	return newAppChatQuotaSnapshotWithTrial(limit, reserved, consumed, trialRemaining, nearest), nil
}
