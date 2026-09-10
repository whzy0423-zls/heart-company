package caresystem

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
)

type Store struct{ db *sql.DB }

func NewStore(db *sql.DB) *Store { return &Store{db: db} }

func (s *Store) Enqueue(ctx context.Context, appUserID int64) error {
	if s == nil || s.db == nil || appUserID <= 0 {
		return fmt.Errorf("care store unavailable")
	}
	_, err := s.db.ExecContext(ctx, `
		INSERT INTO care_evaluation_queue(app_user_id)
		VALUES($1)
		ON CONFLICT (app_user_id) WHERE status IN ('pending','processing') DO UPDATE
		SET status='pending', next_attempt_at=now(), update_time=now()`, appUserID)
	return err
}

func (s *Store) ClaimDue(ctx context.Context, limit int) ([]int64, error) {
	if limit <= 0 {
		limit = 20
	}
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return nil, err
	}
	defer tx.Rollback()
	rows, err := tx.QueryContext(ctx, `
		SELECT id, app_user_id FROM care_evaluation_queue
		WHERE status IN ('pending','failed') AND next_attempt_at <= now()
		ORDER BY id LIMIT $1 FOR UPDATE SKIP LOCKED`, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var ids, queueIDs []int64
	for rows.Next() {
		var qid, uid int64
		if err := rows.Scan(&qid, &uid); err != nil {
			return nil, err
		}
		queueIDs = append(queueIDs, qid)
		ids = append(ids, uid)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	for _, qid := range queueIDs {
		if _, err := tx.ExecContext(ctx, `UPDATE care_evaluation_queue SET status='processing', attempts=attempts+1, update_time=now() WHERE id=$1`, qid); err != nil {
			return nil, err
		}
	}
	if err := tx.Commit(); err != nil {
		return nil, err
	}
	return ids, nil
}

func (s *Store) SaveFailure(ctx context.Context, appUserID int64, evalErr error) error {
	message := "evaluation failed"
	if evalErr != nil {
		message = evalErr.Error()
	}
	_, err := s.db.ExecContext(ctx, `UPDATE care_evaluation_queue SET status='failed', last_error=$1, next_attempt_at=now() + LEAST((POWER(2, attempts) * INTERVAL '1 minute'), INTERVAL '6 hours'), update_time=now() WHERE app_user_id=$2 AND status IN ('pending','processing')`, message, appUserID)
	return err
}

func (s *Store) Current(ctx context.Context, appUserID int64) (Evaluation, error) {
	var e Evaluation
	var level sql.NullInt64
	var evaluated sql.NullTime
	var status, trend string
	err := s.db.QueryRowContext(ctx, `SELECT care_level, care_label, care_summary, care_trend, care_data_status, care_evaluated_at, care_knowledge_version, care_evaluation_version FROM app_users WHERE id=$1`, appUserID).
		Scan(&level, &e.Label, &e.Summary, &trend, &status, &evaluated, &e.KnowledgeVersion, &e.EvaluationVersion)
	if err != nil {
		return Evaluation{}, err
	}
	e.AppUserID = appUserID
	e.DataStatus = DataStatus(status)
	e.Trend = Trend(trend)
	if level.Valid {
		value := int(level.Int64)
		e.Level = &value
	}
	if evaluated.Valid {
		e.WindowEnd = evaluated.Time
	}
	return e, nil
}

func (s *Store) Save(ctx context.Context, evaluation Evaluation) error {
	if evaluation.AppUserID <= 0 {
		return fmt.Errorf("care user id is required")
	}
	signals, err := json.Marshal(evaluation.Signals)
	if err != nil {
		return err
	}
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()
	_, err = tx.ExecContext(ctx, `
		INSERT INTO care_evaluations(app_user_id, care_level, care_label, care_summary, care_trend, data_status, signals, source_window_start, source_window_end, knowledge_version, evaluation_version, error_message)
		VALUES($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11,$12)`, evaluation.AppUserID, evaluation.Level, evaluation.Label, evaluation.Summary, evaluation.Trend, evaluation.DataStatus, signals, evaluation.WindowStart, evaluation.WindowEnd, evaluation.KnowledgeVersion, evaluation.EvaluationVersion, evaluation.ErrorMessage)
	if err != nil {
		return err
	}
	_, err = tx.ExecContext(ctx, `
		UPDATE app_users SET care_level=$1, care_label=$2, care_summary=$3, care_trend=$4, care_data_status=$5, care_evaluated_at=now(), care_knowledge_version=$6, care_evaluation_version=$7, update_time=now() WHERE id=$8`, evaluation.Level, evaluation.Label, evaluation.Summary, evaluation.Trend, evaluation.DataStatus, evaluation.KnowledgeVersion, evaluation.EvaluationVersion, evaluation.AppUserID)
	if err != nil {
		return err
	}
	_, err = tx.ExecContext(ctx, `UPDATE care_evaluation_queue SET status='done', update_time=now() WHERE app_user_id=$1 AND status IN ('pending','processing')`, evaluation.AppUserID)
	if err != nil {
		return err
	}
	return tx.Commit()
}
