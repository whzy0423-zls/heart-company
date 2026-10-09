package growthinsight

import (
	"context"
	"crypto/rand"
	"database/sql"
	"encoding/hex"
	"encoding/json"
	"errors"
	"time"
)

const (
	debounce          = 30 * time.Minute
	cadence           = 24 * time.Hour
	publicationPeriod = 7 * 24 * time.Hour
	leaseDuration     = 10 * time.Minute
	maxAttempts       = 3
)

type job struct {
	userID, cardID, revision int64
	fingerprint, token       string
	attempts                 int
	claimedAt                time.Time
}

func (s *Store) Enqueue(ctx context.Context, userID int64) error {
	return s.enqueue(ctx, userID, time.Now().UTC(), true)
}

func (s *Store) enqueue(ctx context.Context, userID int64, now time.Time, manual bool) error {
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()
	cardID, revision, err := authorizedTx(ctx, tx, userID)
	if err != nil {
		return err
	}
	if _, err = tx.ExecContext(ctx, `WITH exhausted AS (
UPDATE app_growth_insight_jobs SET status='failed',claim_token='',lease_until=NULL,error_code='lease_exhausted'
WHERE app_user_id=$1 AND status='analyzing' AND lease_until<=$2 AND attempts>=$3 RETURNING app_user_id)
UPDATE app_growth_insight_consents SET status='failed' WHERE app_user_id IN (SELECT app_user_id FROM exhausted)`, userID, now, maxAttempts); err != nil {
		return err
	}
	var previous string
	var lastSuccess sql.NullTime
	if err = tx.QueryRowContext(ctx, `SELECT last_fingerprint,last_success_at FROM app_growth_insight_consents WHERE app_user_id=$1`, userID).Scan(&previous, &lastSuccess); err != nil {
		return err
	}
	sources, err := collect(ctx, tx, userID, cardID, now)
	if err != nil {
		return err
	}
	if _, err = tx.ExecContext(ctx, `UPDATE app_growth_insight_consents SET last_scanned_at=$2 WHERE app_user_id=$1`, userID, now); err != nil {
		return err
	}
	if sources.Fingerprint == previous || (previous != "" && lastSuccess.Valid && !sources.Latest.After(lastSuccess.Time)) {
		if err = tx.Commit(); err != nil {
			return err
		}
		if manual {
			return ErrNoNewEvidence
		}
		return nil
	}
	due := sources.Latest.Add(debounce)
	if lastSuccess.Valid && lastSuccess.Time.Add(cadence).After(due) {
		due = lastSuccess.Time.Add(cadence)
	}
	if sources.Latest.IsZero() {
		due = now
	}
	_, err = tx.ExecContext(ctx, `INSERT INTO app_growth_insight_jobs(app_user_id,card_id,revision,fingerprint,due_at,updated_at)
VALUES($1,$2,$3,$4,$5,$6)
ON CONFLICT(app_user_id) DO UPDATE SET card_id=EXCLUDED.card_id,revision=EXCLUDED.revision,fingerprint=EXCLUDED.fingerprint,
 due_at=CASE WHEN app_growth_insight_jobs.error_code IN ('user_daily_budget_exhausted','global_daily_budget_exhausted') AND app_growth_insight_jobs.due_at>$6 THEN app_growth_insight_jobs.due_at
 WHEN app_growth_insight_jobs.fingerprint=EXCLUDED.fingerprint AND NOT $7 THEN greatest(app_growth_insight_jobs.due_at,EXCLUDED.due_at) ELSE EXCLUDED.due_at END,
 status='pending',attempts=CASE WHEN app_growth_insight_jobs.fingerprint<>EXCLUDED.fingerprint OR $7 THEN 0 ELSE app_growth_insight_jobs.attempts END,
 claim_token='',lease_until=NULL,error_code=CASE WHEN app_growth_insight_jobs.error_code IN ('user_daily_budget_exhausted','global_daily_budget_exhausted') AND app_growth_insight_jobs.due_at>$6 THEN app_growth_insight_jobs.error_code ELSE '' END,updated_at=EXCLUDED.updated_at
WHERE (app_growth_insight_jobs.status<>'analyzing' OR app_growth_insight_jobs.lease_until<=$6)
 AND (app_growth_insight_jobs.fingerprint<>EXCLUDED.fingerprint OR $7 OR app_growth_insight_jobs.status='pending' OR (app_growth_insight_jobs.status='analyzing' AND app_growth_insight_jobs.attempts<$8))`, userID, cardID, revision, sources.Fingerprint, due, now, manual, maxAttempts)
	if err != nil {
		return err
	}
	_, err = tx.ExecContext(ctx, `UPDATE app_growth_insight_consents SET status='pending' WHERE app_user_id=$1 AND EXISTS(SELECT 1 FROM app_growth_insight_jobs j WHERE j.app_user_id=$1 AND j.status='pending')`, userID)
	if err != nil {
		return err
	}
	return tx.Commit()
}

// Tick scans a bounded set and executes one job; callers can run it periodically without chat-request coupling.
func (s *Store) Tick(ctx context.Context, complete CompleteFunc, now time.Time) error {
	tickStartedAt := time.Now()
	now = now.UTC()
	if err := s.pruneAttempts(ctx, now); err != nil {
		return err
	}
	if _, err := s.db.ExecContext(ctx, `UPDATE app_growth_insight_consents c SET published_report_id=latest_report_id,published_at=$1,status='ready'
WHERE c.enabled AND c.latest_report_id IS NOT NULL AND c.latest_report_id IS DISTINCT FROM c.published_report_id
 AND (c.published_at IS NULL OR c.published_at<=$1::timestamptz-interval '7 days')
 AND EXISTS(SELECT 1 FROM app_growth_insight_reports r JOIN app_user_cards card ON card.id=r.card_id
 JOIN app_users u ON u.id=r.app_user_id WHERE r.id=c.latest_report_id AND card.card_type='primary' AND card.status='active' AND card.app_user_id=c.app_user_id AND u.status='active')`, now); err != nil {
		return err
	}
	rows, err := s.db.QueryContext(ctx, `SELECT c.app_user_id FROM app_growth_insight_consents c JOIN app_users u ON u.id=c.app_user_id AND u.status='active'
WHERE c.enabled AND EXISTS(SELECT 1 FROM app_user_cards card WHERE card.app_user_id=c.app_user_id AND card.card_type='primary' AND card.status='active')
ORDER BY c.last_scanned_at,c.app_user_id LIMIT 100`)
	if err != nil {
		return err
	}
	var users []int64
	for rows.Next() {
		var id int64
		if err = rows.Scan(&id); err != nil {
			rows.Close()
			return err
		}
		users = append(users, id)
	}
	err = rows.Err()
	rows.Close()
	if err != nil {
		return err
	}
	for _, id := range users {
		if err = s.enqueue(ctx, id, now, false); err != nil && !errors.Is(err, ErrDisabled) && !errors.Is(err, ErrNotFound) {
			return err
		}
	}
	j, err := s.claim(ctx, now)
	if errors.Is(err, ErrNotFound) {
		return nil
	}
	if err != nil {
		return err
	}
	// The supplied clock anchors this tick, so lease and budget checks include scanning time too.
	j.claimedAt = tickStartedAt
	sources, err := collect(ctx, s.db, j.userID, j.cardID, now)
	if err != nil {
		return s.fail(ctx, j, now, "source_error", err)
	}
	if sources.Fingerprint != j.fingerprint {
		return s.discard(ctx, j)
	}
	if sources.BehaviorCount < 2 {
		return s.finish(ctx, j, sources, Analysis{}, now, true)
	}
	if err = s.reserveAttempt(ctx, j, now); err != nil {
		return err
	}
	modelCtx, cancel := context.WithTimeout(ctx, 5*time.Minute)
	defer cancel()
	analysis, err := generate(modelCtx, complete, sources, now)
	if err != nil {
		return s.fail(ctx, j, now, "generation_failed", err)
	}
	if err = modelCtx.Err(); err != nil {
		return s.fail(ctx, j, now, "generation_timeout", err)
	}
	return s.finish(ctx, j, sources, analysis, now, false)
}

func (s *Store) claim(ctx context.Context, now time.Time) (job, error) {
	j := job{claimedAt: time.Now()}
	token := make([]byte, 24)
	if _, err := rand.Read(token); err != nil {
		return j, err
	}
	j.token = hex.EncodeToString(token)
	// All paths lock consent before a job, including expiry reclamation and finalization.
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return j, err
	}
	defer tx.Rollback()
	err = tx.QueryRowContext(ctx, `SELECT c.app_user_id FROM app_growth_insight_consents c
JOIN app_growth_insight_jobs j ON j.app_user_id=c.app_user_id
WHERE c.enabled AND j.due_at<=$1 AND j.attempts<$2 AND (j.status='pending' OR (j.status='analyzing' AND j.lease_until<=$1))
ORDER BY j.due_at,c.app_user_id FOR UPDATE OF c SKIP LOCKED LIMIT 1`, now, maxAttempts).Scan(&j.userID)
	if err != nil {
		return j, storeError(err)
	}
	err = tx.QueryRowContext(ctx, `UPDATE app_growth_insight_jobs SET status='analyzing',claim_token=$2,lease_until=$3,attempts=attempts+1,updated_at=$4
WHERE app_user_id=$1 RETURNING card_id,revision,fingerprint,attempts`, j.userID, j.token, now.Add(leaseDuration), now).Scan(&j.cardID, &j.revision, &j.fingerprint, &j.attempts)
	if err != nil {
		return j, err
	}
	if _, err = tx.ExecContext(ctx, `UPDATE app_growth_insight_consents SET status='analyzing' WHERE app_user_id=$1`, j.userID); err != nil {
		return j, err
	}
	return j, tx.Commit()
}

func (s *Store) finish(ctx context.Context, j job, sources sourceSet, a Analysis, now time.Time, insufficient bool) error {
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
	leaseCheckTime := now
	if !j.claimedAt.IsZero() {
		leaseCheckTime = now.Add(time.Since(j.claimedAt))
	}
	if token != j.token || cardID != j.cardID || revision != j.revision || !lease.After(leaseCheckTime) {
		return ErrStale
	}
	current, err := collect(ctx, tx, j.userID, j.cardID, now)
	if err != nil {
		return err
	}
	if current.Fingerprint != sources.Fingerprint {
		return ErrStale
	}
	if insufficient {
		if _, err = tx.ExecContext(ctx, `UPDATE app_growth_insight_consents SET status='insufficient_data',last_fingerprint=$2 WHERE app_user_id=$1`, j.userID, sources.Fingerprint); err != nil {
			return err
		}
	} else {
		var version int
		var publishedID sql.NullInt64
		var publishedAt sql.NullTime
		if err = tx.QueryRowContext(ctx, `SELECT next_version,published_report_id,published_at FROM app_growth_insight_consents WHERE app_user_id=$1`, j.userID).Scan(&version, &publishedID, &publishedAt); err != nil {
			return err
		}
		start, _ := weeklyPeriod(now)
		end := periodEnd(start, sources.Through)
		r := Report{AppUserID: j.userID, CardID: j.cardID, Version: version, GeneratedAt: now, SourceFrom: sources.From, SourceThrough: sources.Through, PeriodStart: start, PeriodEnd: end, Timezone: "Asia/Shanghai", EvidenceCount: len(sources.Evidence), Coverage: sources.Coverage, Analysis: a, Evidence: sources.Evidence}
		body, err := json.Marshal(r)
		if err != nil {
			return err
		}
		if err = tx.QueryRowContext(ctx, `INSERT INTO app_growth_insight_reports(app_user_id,card_id,version,fingerprint,generated_at,source_through,evidence_count,payload) VALUES($1,$2,$3,$4,$5,$6,$7,$8) RETURNING id`, j.userID, j.cardID, version, sources.Fingerprint, now, sources.Through, len(sources.Evidence), body).Scan(&r.ID); err != nil {
			return err
		}
		for _, action := range a.Actions {
			if _, err = tx.ExecContext(ctx, `INSERT INTO app_growth_insight_actions(report_id,app_user_id,card_id,title,detail,updated_at) VALUES($1,$2,$3,$4,$5,$6)`, r.ID, j.userID, j.cardID, action.Title, action.Detail, now); err != nil {
				return err
			}
		}
		publish := !publishedID.Valid || !publishedAt.Valid || !publishedAt.Time.Add(publicationPeriod).After(now)
		if _, err = tx.ExecContext(ctx, `UPDATE app_growth_insight_consents SET latest_report_id=$2,next_version=next_version+1,last_success_at=$3,last_fingerprint=$4,status='ready',
published_report_id=CASE WHEN $5 THEN $2 ELSE published_report_id END,published_at=CASE WHEN $5 THEN $3 ELSE published_at END WHERE app_user_id=$1`, j.userID, r.ID, now, sources.Fingerprint, publish); err != nil {
			return err
		}
	}
	if _, err = tx.ExecContext(ctx, `UPDATE app_growth_insight_jobs SET status='completed',claim_token='',lease_until=NULL,updated_at=$2 WHERE app_user_id=$1`, j.userID, now); err != nil {
		return err
	}
	return tx.Commit()
}

func (s *Store) fail(ctx context.Context, j job, now time.Time, code string, cause error) error {
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()
	if _, _, err = authorizedTx(ctx, tx, j.userID); err != nil {
		return ErrStale
	}
	status := "pending"
	if j.attempts >= maxAttempts {
		status = "failed"
	}
	result, err := tx.ExecContext(ctx, `UPDATE app_growth_insight_jobs SET status=$3,error_code=$4,claim_token='',lease_until=NULL,due_at=$5,updated_at=$6 WHERE app_user_id=$1 AND claim_token=$2`, j.userID, j.token, status, code, now.Add(time.Duration(j.attempts)*15*time.Minute), now)
	if err != nil {
		return err
	}
	count, err := result.RowsAffected()
	if err != nil {
		return err
	}
	if count == 0 {
		return ErrStale
	}
	if _, err = tx.ExecContext(ctx, `UPDATE app_growth_insight_consents SET status=$2 WHERE app_user_id=$1`, j.userID, status); err != nil {
		return err
	}
	if err = tx.Commit(); err != nil {
		return err
	}
	return cause
}

func (s *Store) discard(ctx context.Context, j job) error {
	_, err := s.db.ExecContext(ctx, `UPDATE app_growth_insight_jobs SET status='pending',claim_token='',lease_until=NULL WHERE app_user_id=$1 AND claim_token=$2`, j.userID, j.token)
	if err != nil {
		return err
	}
	return ErrStale
}
