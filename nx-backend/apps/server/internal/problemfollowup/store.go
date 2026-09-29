package problemfollowup

import (
	"context"
	"crypto/rand"
	"database/sql"
	"encoding/hex"
	"errors"
	"fmt"
	"strings"
	"time"
)

type Store struct{ db *sql.DB }

func NewStore(database *sql.DB) *Store { return &Store{db: database} }

type mainState struct {
	CardID int64
	Level  string
	Expiry sql.NullTime
}

// Every operation that can establish/cancel/publish a follow-up takes locks in
// the same order: user, primary card/session, activity, job. Model calls and
// external push delivery never run while these database locks are held.
func lockMain(ctx context.Context, tx *sql.Tx, userID, sessionID int64) (mainState, bool, error) {
	var state mainState
	var status string
	err := tx.QueryRowContext(ctx, `SELECT status,member_level,member_expires_at FROM app_users WHERE id=$1 FOR UPDATE`, userID).Scan(&status, &state.Level, &state.Expiry)
	if errors.Is(err, sql.ErrNoRows) {
		return state, false, nil
	}
	if err != nil {
		return state, false, err
	}
	if status != "active" {
		return state, false, nil
	}
	err = tx.QueryRowContext(ctx, `SELECT c.id FROM app_chat_sessions s JOIN app_user_cards c ON c.id=s.card_id
	WHERE s.id=$1 AND s.app_user_id=$2 AND c.app_user_id=$2 AND s.scene='chat' AND c.card_type='primary' AND c.status='active'
	FOR SHARE OF c,s`, sessionID, userID).Scan(&state.CardID)
	if errors.Is(err, sql.ErrNoRows) {
		return state, false, nil
	}
	return state, err == nil, err
}

func eligible(ctx context.Context, tx *sql.Tx, state mainState, now time.Time) (bool, error) {
	level := strings.ToLower(strings.TrimSpace(state.Level))
	if level != "svip" && level != "svip_month" && level != "svip_quarter" && level != "svip_year" {
		return false, nil
	}
	if !state.Expiry.Valid {
		if level != "svip" {
			return false, nil
		}
	} else if !state.Expiry.Time.After(now) {
		return false, nil
	}
	var enabled bool
	err := tx.QueryRowContext(ctx, `SELECT CASE WHEN jsonb_typeof(feature_flags)<>'object' THEN false
	WHEN feature_flags ? 'problemFollowup' THEN feature_flags->'problemFollowup'='true'::jsonb ELSE true END FROM app_plans WHERE code='svip'`).Scan(&enabled)
	if errors.Is(err, sql.ErrNoRows) {
		return true, nil
	}
	return enabled, err
}

func (s *Store) BeginTurn(ctx context.Context, userID, sessionID int64, now time.Time) (Turn, error) {
	if s == nil || s.db == nil {
		return Turn{}, errors.New("problem followup: store unavailable")
	}
	if userID <= 0 || sessionID <= 0 {
		return Turn{}, nil
	}
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return Turn{}, err
	}
	defer tx.Rollback()
	state, ok, err := lockMain(ctx, tx, userID, sessionID)
	if err != nil || !ok {
		return Turn{}, err
	}
	turn := Turn{AppUserID: userID, SessionID: sessionID, CardID: state.CardID}
	err = tx.QueryRowContext(ctx, `INSERT INTO app_problem_followup_activity(app_user_id,session_id,revision,last_activity_at)
	VALUES($1,$2,1,$3) ON CONFLICT(app_user_id) DO UPDATE SET session_id=EXCLUDED.session_id,
	revision=app_problem_followup_activity.revision+1,last_activity_at=EXCLUDED.last_activity_at RETURNING revision`, userID, sessionID, now).Scan(&turn.Revision)
	if err != nil {
		return Turn{}, err
	}
	if _, err = tx.ExecContext(ctx, `UPDATE app_problem_followup_jobs SET status='cancelled',claim_token='',lease_until=NULL,updated_at=$2
	WHERE app_user_id=$1 AND status IN ('pending','classifying','waiting','delivering')`, userID, now); err != nil {
		return Turn{}, err
	}
	if err = tx.Commit(); err != nil {
		return Turn{}, err
	}
	return turn, nil
}

// LockTurnTx must run before chat inserts take foreign-key/message locks.
// Legacy callers without an accepted main-chat turn do no additional work.
func LockTurnTx(ctx context.Context, tx *sql.Tx, sessionID int64) error {
	turn := turnFrom(ctx)
	if turn.Revision <= 0 {
		return nil
	}
	if turn.SessionID != sessionID {
		return ErrStale
	}
	_, _, err := lockMain(ctx, tx, turn.AppUserID, sessionID)
	return err
}

func currentActivity(ctx context.Context, tx *sql.Tx, turn Turn) (bool, error) {
	var revision, sessionID int64
	err := tx.QueryRowContext(ctx, `SELECT revision,session_id FROM app_problem_followup_activity WHERE app_user_id=$1 FOR UPDATE`, turn.AppUserID).Scan(&revision, &sessionID)
	if errors.Is(err, sql.ErrNoRows) {
		return false, nil
	}
	if err != nil {
		return false, err
	}
	return revision == turn.Revision && sessionID == turn.SessionID, nil
}

func latestAnswer(ctx context.Context, tx *sql.Tx, sessionID, assistantID, revision int64) (bool, error) {
	var latest bool
	err := tx.QueryRowContext(ctx, `SELECT EXISTS(SELECT 1 FROM app_chat_messages m
	WHERE m.id=$1 AND m.session_id=$2 AND m.role='assistant' AND btrim(m.content)<>'' AND m.problem_followup_revision=$3
	AND NOT EXISTS(SELECT 1 FROM app_chat_messages newer WHERE newer.session_id=$2 AND newer.id>m.id
	 AND (newer.problem_followup_revision=0 OR newer.problem_followup_revision >= $3))
	AND NOT EXISTS(SELECT 1 FROM app_problem_followup_jobs prior WHERE prior.followup_message_id=m.id))`, assistantID, sessionID, revision).Scan(&latest)
	return latest, err
}

// EnqueueTx is called only after a successful answer has been inserted in the
// same transaction. An accepted turn with a newer activity revision wins even
// when an older provider request happens to finish later.
func EnqueueTx(ctx context.Context, tx *sql.Tx, sessionID, assistantID int64, now time.Time) error {
	turn := turnFrom(ctx)
	if turn.Revision <= 0 {
		return nil
	}
	if turn.SessionID != sessionID {
		return ErrStale
	}
	state, ok, err := lockMain(ctx, tx, turn.AppUserID, sessionID)
	if err != nil || !ok {
		return err
	}
	if state.CardID != turn.CardID {
		return nil
	}
	// Mark even a late obsolete turn. Its messages must not cancel a newer
	// user's valid candidate merely because the old provider finished last.
	// Atomic text/voice pairs share PostgreSQL's transaction create_time.
	_, err = tx.ExecContext(ctx, `UPDATE app_chat_messages m SET problem_followup_revision=$3
	WHERE m.session_id=$1 AND (m.id=$2 OR m.id=(
	 SELECT u.id FROM app_chat_messages u JOIN app_chat_messages a ON a.id=$2 AND a.session_id=$1
	 WHERE u.session_id=$1 AND u.role='user' AND u.id<a.id AND u.create_time=a.create_time
	 ORDER BY u.id DESC LIMIT 1))`, sessionID, assistantID, turn.Revision)
	if err != nil {
		return err
	}
	if ok, err = currentActivity(ctx, tx, turn); err != nil || !ok {
		return err
	}
	if ok, err = eligible(ctx, tx, state, now); err != nil || !ok {
		return err
	}
	if ok, err = latestAnswer(ctx, tx, sessionID, assistantID, turn.Revision); err != nil || !ok {
		return err
	}
	_, err = tx.ExecContext(ctx, `INSERT INTO app_problem_followup_jobs
	(app_user_id,session_id,card_id,assistant_message_id,activity_revision,due_at,next_attempt_at,created_at,updated_at)
	VALUES($1,$2,$3,$4,$5,$6,$7,$7,$7) ON CONFLICT(assistant_message_id) DO NOTHING`, turn.AppUserID, sessionID, turn.CardID, assistantID, turn.Revision, now.Add(Delay), now)
	return err
}

func (s *Store) Claim(ctx context.Context, now time.Time) (*Job, error) {
	if s == nil || s.db == nil {
		return nil, errors.New("problem followup: store unavailable")
	}
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return nil, err
	}
	defer tx.Rollback()
	// A process may die on its final attempt; expire the lease into a terminal
	// state rather than retaining an unclaimable processing row forever.
	if _, err = tx.ExecContext(ctx, `UPDATE app_problem_followup_jobs SET status='failed',claim_token='',lease_until=NULL,updated_at=$1
	WHERE attempts >= $2 AND status IN ('classifying','delivering') AND lease_until <= $1`, now, MaxAttempts); err != nil {
		return nil, err
	}
	var job Job
	err = tx.QueryRowContext(ctx, `SELECT id,app_user_id,session_id,card_id,assistant_message_id,activity_revision,status,due_at,attempts
	FROM app_problem_followup_jobs WHERE attempts < $2 AND next_attempt_at <= $1 AND (
	 status='pending' OR (status='waiting' AND due_at <= $1) OR
	 (status IN ('classifying','delivering') AND lease_until <= $1))
	ORDER BY next_attempt_at,id LIMIT 1 FOR UPDATE SKIP LOCKED`, now, MaxAttempts).Scan(&job.ID, &job.AppUserID, &job.SessionID, &job.CardID, &job.AssistantMessageID, &job.Revision, &job.Status, &job.DueAt, &job.Attempts)
	if errors.Is(err, sql.ErrNoRows) {
		if err = tx.Commit(); err != nil {
			return nil, err
		}
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	if job.Status == "pending" {
		job.Status = "classifying"
	}
	if job.Status == "waiting" {
		job.Status = "delivering"
	}
	var token [24]byte
	if _, err = rand.Read(token[:]); err != nil {
		return nil, err
	}
	job.ClaimToken = hex.EncodeToString(token[:])
	job.Attempts++
	_, err = tx.ExecContext(ctx, `UPDATE app_problem_followup_jobs SET status=$2,claim_token=$3,lease_until=$4,attempts=$5,updated_at=$6 WHERE id=$1`, job.ID, job.Status, job.ClaimToken, now.Add(Lease), job.Attempts, now)
	if err != nil {
		return nil, err
	}
	if err = tx.Commit(); err != nil {
		return nil, err
	}
	return &job, nil
}

// validateClaim cancels an ineligible live claim as part of the caller's
// transaction. The caller commits that cancellation before returning ErrStale.
func validateClaim(ctx context.Context, tx *sql.Tx, job Job, now time.Time) (bool, error) {
	state, mainOK, err := lockMain(ctx, tx, job.AppUserID, job.SessionID)
	if err != nil {
		return false, err
	}
	activityOK := false
	if mainOK {
		activityOK, err = currentActivity(ctx, tx, Turn{AppUserID: job.AppUserID, SessionID: job.SessionID, CardID: job.CardID, Revision: job.Revision})
		if err != nil {
			return false, err
		}
	}
	var exists bool
	err = tx.QueryRowContext(ctx, `SELECT true FROM app_problem_followup_jobs WHERE id=$1 AND app_user_id=$2 AND session_id=$3 AND card_id=$4
	AND assistant_message_id=$5 AND activity_revision=$6 AND status=$7 AND claim_token=$8 AND lease_until>$9 FOR UPDATE`,
		job.ID, job.AppUserID, job.SessionID, job.CardID, job.AssistantMessageID, job.Revision, job.Status, job.ClaimToken, now).Scan(&exists)
	if errors.Is(err, sql.ErrNoRows) {
		return false, nil
	}
	if err != nil {
		return false, err
	}
	ok := mainOK && activityOK && state.CardID == job.CardID
	if ok {
		ok, err = eligible(ctx, tx, state, now)
		if err != nil {
			return false, err
		}
	}
	if ok {
		ok, err = latestAnswer(ctx, tx, job.SessionID, job.AssistantMessageID, job.Revision)
		if err != nil {
			return false, err
		}
	}
	if !ok {
		_, err = tx.ExecContext(ctx, `UPDATE app_problem_followup_jobs SET status='cancelled',claim_token='',lease_until=NULL,updated_at=$2 WHERE id=$1`, job.ID, now)
	}
	return ok, err
}

func (s *Store) LoadContext(ctx context.Context, job Job) (Input, error) {
	if job.Status != "classifying" {
		return Input{}, ErrStale
	}
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return Input{}, err
	}
	defer tx.Rollback()
	ok, err := validateClaim(ctx, tx, job, time.Now())
	if err != nil {
		return Input{}, err
	}
	if !ok {
		if err = tx.Commit(); err != nil {
			return Input{}, err
		}
		return Input{}, ErrStale
	}
	rows, err := tx.QueryContext(ctx, `SELECT m.role,CASE WHEN m.message_type='voice' THEN m.transcript ELSE m.content END
	FROM app_chat_messages m WHERE m.session_id=$1 AND m.id<=$2 AND m.role IN ('user','assistant')
	AND NOT EXISTS(SELECT 1 FROM app_problem_followup_jobs j WHERE j.followup_message_id=m.id)
	ORDER BY m.id DESC LIMIT 12`, job.SessionID, job.AssistantMessageID)
	if err != nil {
		return Input{}, err
	}
	var input Input
	for rows.Next() {
		var message Message
		if err = rows.Scan(&message.Role, &message.Content); err != nil {
			rows.Close()
			return Input{}, err
		}
		message.Content = bound(strings.TrimSpace(message.Content), 2000)
		input.Messages = append(input.Messages, message)
	}
	if err = rows.Err(); err != nil {
		rows.Close()
		return Input{}, err
	}
	rows.Close()
	for i, j := 0, len(input.Messages)-1; i < j; i, j = i+1, j-1 {
		input.Messages[i], input.Messages[j] = input.Messages[j], input.Messages[i]
	}
	// Read the exact atomic pair, not an unrelated earlier user's message.
	err = tx.QueryRowContext(ctx, `SELECT a.content,COALESCE((
	 SELECT CASE WHEN u.message_type='voice' THEN u.transcript ELSE u.content END FROM app_chat_messages u
	 WHERE u.session_id=a.session_id AND u.role='user' AND u.id<a.id AND u.create_time=a.create_time
	 ORDER BY u.id DESC LIMIT 1),'') FROM app_chat_messages a WHERE a.id=$1 AND a.session_id=$2`,
		job.AssistantMessageID, job.SessionID).Scan(&input.Answer, &input.Question)
	if err != nil {
		return Input{}, err
	}
	input.Question = bound(strings.TrimSpace(input.Question), 1200)
	input.Answer = bound(strings.TrimSpace(input.Answer), 6000)
	if input.Question == "" || input.Answer == "" {
		if _, err = tx.ExecContext(ctx, `UPDATE app_problem_followup_jobs SET status='skipped',claim_token='',lease_until=NULL,updated_at=now() WHERE id=$1`, job.ID); err != nil {
			return Input{}, err
		}
		if err = tx.Commit(); err != nil {
			return Input{}, err
		}
		return Input{}, ErrStale
	}
	if err = tx.Commit(); err != nil {
		return Input{}, err
	}
	return input, nil
}

func (s *Store) SaveDecision(ctx context.Context, job Job, decision Decision, now time.Time) error {
	if job.Status != "classifying" {
		return ErrStale
	}
	if err := ValidateDecision(decision); err != nil {
		return err
	}
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()
	ok, err := validateClaim(ctx, tx, job, now)
	if err != nil {
		return err
	}
	if !ok {
		if err = tx.Commit(); err != nil {
			return err
		}
		return ErrStale
	}
	status := "skipped"
	if decision.ShouldFollowUp {
		status = "waiting"
	}
	_, err = tx.ExecContext(ctx, `UPDATE app_problem_followup_jobs SET status=$2,problem_summary=$3,followup_text=$4,claim_token='',lease_until=NULL,
	attempts=0,next_attempt_at=due_at,updated_at=$5 WHERE id=$1`, job.ID, status, decision.ProblemSummary, decision.Message, now)
	if err != nil {
		return err
	}
	return tx.Commit()
}

func (s *Store) Fail(ctx context.Context, job Job, now time.Time) error {
	if job.Status != "classifying" && job.Status != "delivering" {
		return ErrStale
	}
	status := "pending"
	if job.Status == "delivering" {
		status = "waiting"
	}
	if job.Attempts >= MaxAttempts {
		status = "failed"
	}
	result, err := s.db.ExecContext(ctx, `UPDATE app_problem_followup_jobs SET status=$2,claim_token='',lease_until=NULL,
	next_attempt_at=$3,updated_at=$4 WHERE id=$1 AND status=$5 AND claim_token=$6 AND lease_until>$4`, job.ID, status, now.Add(time.Duration(job.Attempts)*time.Minute), now, job.Status, job.ClaimToken)
	if err != nil {
		return err
	}
	n, err := result.RowsAffected()
	if err != nil {
		return err
	}
	if n != 1 {
		return ErrStale
	}
	return nil
}

func (s *Store) Deliver(ctx context.Context, job Job, now time.Time) (*Delivery, error) {
	if job.Status != "delivering" || now.Before(job.DueAt) {
		return nil, ErrStale
	}
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return nil, err
	}
	defer tx.Rollback()
	ok, err := validateClaim(ctx, tx, job, now)
	if err != nil {
		return nil, err
	}
	if !ok {
		if err = tx.Commit(); err != nil {
			return nil, err
		}
		return nil, ErrStale
	}
	var decision Decision
	decision.ShouldFollowUp = true
	if err = tx.QueryRowContext(ctx, `SELECT problem_summary,followup_text FROM app_problem_followup_jobs WHERE id=$1`, job.ID).Scan(&decision.ProblemSummary, &decision.Message); err != nil {
		return nil, err
	}
	if err = ValidateDecision(decision); err != nil {
		return nil, err
	}
	delivery := &Delivery{AppUserID: job.AppUserID, SessionID: job.SessionID, CardID: job.CardID, DeepLink: fmt.Sprintf("/chat/%d", job.SessionID)}
	// This is an assistant-only message, deliberately bypassing SavePair and
	// its enqueue hook. The job association identifies it for context filtering.
	err = tx.QueryRowContext(ctx, `INSERT INTO app_chat_messages(session_id,role,content,sources,message_type,create_time)
	VALUES($1,'assistant',$2,'[]','text',$3) RETURNING id`, job.SessionID, decision.Message, now).Scan(&delivery.MessageID)
	if err != nil {
		return nil, err
	}
	if _, err = tx.ExecContext(ctx, `UPDATE app_chat_sessions SET updated_at=$2 WHERE id=$1`, job.SessionID, now); err != nil {
		return nil, err
	}
	err = tx.QueryRowContext(ctx, `INSERT INTO app_notifications(app_user_id,kind,title,content,deep_link,source_key)
	VALUES($1,'problem_followup','问题解决跟进','你有一条新的对话跟进，点击回到会话查看。',$2,$3)
	ON CONFLICT(app_user_id,source_key) WHERE source_key<>'' DO UPDATE SET source_key=EXCLUDED.source_key RETURNING id`,
		job.AppUserID, delivery.DeepLink, fmt.Sprintf("problem-followup:%d", job.AssistantMessageID)).Scan(&delivery.NotificationID)
	if err != nil {
		return nil, err
	}
	_, err = tx.ExecContext(ctx, `UPDATE app_problem_followup_jobs SET status='sent',sent_at=$2,followup_message_id=$3,notification_id=$4,
	claim_token='',lease_until=NULL,updated_at=$2 WHERE id=$1`, job.ID, now, delivery.MessageID, delivery.NotificationID)
	if err != nil {
		return nil, err
	}
	if err = tx.Commit(); err != nil {
		return nil, err
	}
	return delivery, nil
}
