package growthinsight

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"strings"
	"time"
	"unicode/utf8"
)

type Store struct{ db *sql.DB }

func NewStore(db *sql.DB) *Store { return &Store{db: db} }

func (s *Store) Consent(ctx context.Context, userID int64) (bool, error) {
	v, err := s.ConsentMetadata(ctx, userID)
	return v.Enabled, err
}

func (s *Store) ConsentMetadata(ctx context.Context, userID int64) (ConsentState, error) {
	var v ConsentState
	err := s.db.QueryRowContext(ctx, `SELECT COALESCE(c.enabled,false),COALESCE(c.published_report_id,0),COALESCE(c.disclosure_version,'')
FROM app_users u LEFT JOIN app_growth_insight_consents c ON c.app_user_id=u.id WHERE u.id=$1`, userID).Scan(&v.Enabled, &v.ReportID, &v.DisclosureVersion)
	return v, storeError(err)
}

func (s *Store) SetConsent(ctx context.Context, userID int64, enabled bool) (bool, error) {
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return false, err
	}
	defer tx.Rollback()
	var active bool
	if err = tx.QueryRowContext(ctx, `SELECT status='active' FROM app_users WHERE id=$1 FOR SHARE`, userID).Scan(&active); err != nil {
		return false, storeError(err)
	}
	if enabled && !active {
		return false, ErrNotFound
	}
	if _, err = tx.ExecContext(ctx, `INSERT INTO app_growth_insight_consents(app_user_id) VALUES($1) ON CONFLICT DO NOTHING`, userID); err != nil {
		return false, err
	}
	var wasEnabled bool
	if err = tx.QueryRowContext(ctx, `SELECT enabled FROM app_growth_insight_consents WHERE app_user_id=$1 FOR UPDATE`, userID).Scan(&wasEnabled); err != nil {
		return false, err
	}
	if wasEnabled != enabled {
		if _, err = tx.ExecContext(ctx, `UPDATE app_growth_insight_consents SET enabled=$2,disclosure_version=$3,updated_at=now() WHERE app_user_id=$1`, userID, enabled, DisclosureVersion); err != nil {
			return false, err
		}
		if _, err = tx.ExecContext(ctx, `SELECT growth_insight_invalidate($1)`, userID); err != nil {
			return false, err
		}
	}
	return enabled, tx.Commit()
}

func (s *Store) View(ctx context.Context, userID, cardID int64) (UserView, error) {
	v := emptyView(cardID)
	var primary bool
	err := s.db.QueryRowContext(ctx, `SELECT card_type='primary' AND status='active' FROM app_user_cards WHERE id=$1 AND app_user_id=$2`, cardID, userID).Scan(&primary)
	if err != nil {
		return v, storeError(err)
	}
	if !primary {
		return v, nil
	}
	var reportID int64
	var publishedAt sql.NullTime
	err = s.db.QueryRowContext(ctx, `SELECT enabled,status,COALESCE(published_report_id,0),published_at FROM app_growth_insight_consents WHERE app_user_id=$1`, userID).Scan(&v.Enabled, &v.Status, &reportID, &publishedAt)
	if errors.Is(err, sql.ErrNoRows) {
		return v, nil
	}
	if err != nil {
		return v, err
	}
	if !v.Enabled {
		return emptyView(cardID), nil
	}
	if reportID == 0 {
		return v, nil
	}
	r, err := s.Report(ctx, reportID)
	if errors.Is(err, ErrNotFound) {
		v.Status = "pending"
		return v, nil
	}
	if err != nil {
		return v, err
	}
	if r.CardID != cardID || r.AppUserID != userID {
		return emptyView(cardID), nil
	}
	actions, err := s.actions(ctx, reportID)
	if err != nil {
		return v, err
	}
	v = project(r, actions)
	if publishedAt.Valid {
		v.NextUpdateAt = stamp(publishedAt.Time.Add(publicationPeriod))
	}
	return v, nil
}

func (s *Store) Report(ctx context.Context, reportID int64) (Report, error) {
	var r Report
	var body []byte
	err := s.db.QueryRowContext(ctx, `SELECT r.id,r.payload,c.published_report_id=r.id
FROM app_growth_insight_reports r JOIN app_growth_insight_consents c ON c.app_user_id=r.app_user_id
JOIN app_users u ON u.id=r.app_user_id AND u.status='active'
JOIN app_user_cards card ON card.id=r.card_id AND card.app_user_id=r.app_user_id AND card.card_type='primary' AND card.status='active'
WHERE r.id=$1 AND c.enabled`, reportID).Scan(&r.ID, &body, &r.Published)
	if err != nil {
		return r, storeError(err)
	}
	id, published := r.ID, r.Published
	if err = json.Unmarshal(body, &r); err != nil {
		return Report{}, err
	}
	r.ID = id
	r.Published = published
	return r, nil
}

func (s *Store) AdminList(ctx context.Context, userID int64) (AdminList, error) {
	v := AdminList{Status: "disabled", Reports: []ReportSummary{}}
	err := s.db.QueryRowContext(ctx, `SELECT COALESCE(c.enabled,false) AND u.status='active',CASE WHEN u.status='active' THEN COALESCE(c.status,'disabled') ELSE 'disabled' END
FROM app_users u LEFT JOIN app_growth_insight_consents c ON c.app_user_id=u.id WHERE u.id=$1`, userID).Scan(&v.Enabled, &v.Status)
	if err != nil {
		return v, storeError(err)
	}
	if !v.Enabled {
		return v, nil
	}
	rows, err := s.db.QueryContext(ctx, `SELECT id,version,generated_at,source_through,evidence_count FROM app_growth_insight_reports WHERE app_user_id=$1 ORDER BY version DESC LIMIT 100`, userID)
	if err != nil {
		return v, err
	}
	defer rows.Close()
	for rows.Next() {
		var r ReportSummary
		if err = rows.Scan(&r.ID, &r.Version, &r.GeneratedAt, &r.SourceThrough, &r.EvidenceCount); err != nil {
			return v, err
		}
		v.Reports = append(v.Reports, r)
	}
	return v, rows.Err()
}

const actionColumns = `id,report_id,card_id,title,detail,status,outcome,note,updated_at`

type scanner interface{ Scan(...any) error }

func scanAction(row scanner) (Action, error) {
	var a Action
	var at time.Time
	err := row.Scan(&a.ID, &a.ReportID, &a.CardID, &a.Title, &a.Detail, &a.Status, &a.Outcome, &a.Note, &at)
	a.UpdatedAt = stamp(at)
	return a, storeError(err)
}

func (s *Store) actions(ctx context.Context, reportID int64) ([]Action, error) {
	result := []Action{}
	rows, err := s.db.QueryContext(ctx, `SELECT `+actionColumns+` FROM app_growth_insight_actions WHERE report_id=$1 ORDER BY id`, reportID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	for rows.Next() {
		a, err := scanAction(rows)
		if err != nil {
			return nil, err
		}
		result = append(result, a)
	}
	return result, rows.Err()
}

func authorizedTx(ctx context.Context, tx *sql.Tx, userID int64) (int64, int64, error) {
	var active bool
	if err := tx.QueryRowContext(ctx, `SELECT status='active' FROM app_users WHERE id=$1 FOR SHARE`, userID).Scan(&active); err != nil {
		return 0, 0, storeError(err)
	}
	if !active {
		return 0, 0, ErrDisabled
	}
	var cardID int64
	if err := tx.QueryRowContext(ctx, `SELECT id FROM app_user_cards WHERE app_user_id=$1 AND card_type='primary' AND status='active' FOR SHARE`, userID).Scan(&cardID); err != nil {
		return 0, 0, storeError(err)
	}
	var enabled bool
	var revision int64
	err := tx.QueryRowContext(ctx, `SELECT c.enabled,c.revision FROM app_growth_insight_consents c JOIN app_users u ON u.id=c.app_user_id AND u.status='active' WHERE c.app_user_id=$1 FOR UPDATE OF c`, userID).Scan(&enabled, &revision)
	if errors.Is(err, sql.ErrNoRows) {
		return 0, 0, ErrDisabled
	}
	if err != nil {
		return 0, 0, err
	}
	if !enabled {
		return 0, 0, ErrDisabled
	}
	return cardID, revision, nil
}

func (s *Store) FeedbackAction(ctx context.Context, userID, actionID int64, f Feedback) (Action, error) {
	if err := validateFeedback(f); err != nil {
		return Action{}, err
	}
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return Action{}, err
	}
	defer tx.Rollback()
	var owned bool
	if err = tx.QueryRowContext(ctx, `SELECT EXISTS(SELECT 1 FROM app_growth_insight_actions WHERE id=$1 AND app_user_id=$2)`, actionID, userID).Scan(&owned); err != nil {
		return Action{}, err
	}
	if !owned {
		return Action{}, ErrNotFound
	}
	cardID, _, err := authorizedTx(ctx, tx, userID)
	if err != nil {
		return Action{}, err
	}
	a, err := scanAction(tx.QueryRowContext(ctx, `SELECT `+actionColumns+` FROM app_growth_insight_actions WHERE id=$1 AND app_user_id=$2 AND card_id=$3
AND report_id=(SELECT published_report_id FROM app_growth_insight_consents WHERE app_user_id=$2) FOR UPDATE`, actionID, userID, cardID))
	if err != nil {
		return a, err
	}
	if a.Status != f.Status || a.Outcome != f.Outcome || a.Note != f.Note {
		a, err = scanAction(tx.QueryRowContext(ctx, `UPDATE app_growth_insight_actions SET status=$2,outcome=$3,note=$4,updated_at=now() WHERE id=$1 RETURNING `+actionColumns, actionID, f.Status, f.Outcome, f.Note))
		if err != nil {
			return a, err
		}
		if _, err = tx.ExecContext(ctx, `UPDATE app_growth_insight_consents SET revision=revision+1 WHERE app_user_id=$1`, userID); err != nil {
			return a, err
		}
	}
	return a, tx.Commit()
}

func (s *Store) Correct(ctx context.Context, userID, reportID int64, kind, note string) error {
	note = strings.TrimSpace(note)
	if note == "" || utf8.RuneCountInString(note) > 500 || (kind != "inaccurate" && kind != "missing_context" && kind != "other") {
		return ErrInvalid
	}
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()
	var owned bool
	if err = tx.QueryRowContext(ctx, `SELECT EXISTS(SELECT 1 FROM app_growth_insight_reports WHERE id=$1 AND app_user_id=$2)`, reportID, userID).Scan(&owned); err != nil {
		return err
	}
	if !owned {
		return ErrNotFound
	}
	cardID, _, err := authorizedTx(ctx, tx, userID)
	if err != nil {
		return err
	}
	var exists bool
	if err = tx.QueryRowContext(ctx, `SELECT EXISTS(SELECT 1 FROM app_growth_insight_reports WHERE id=$1 AND app_user_id=$2 AND card_id=$3
AND id=(SELECT published_report_id FROM app_growth_insight_consents WHERE app_user_id=$2))`, reportID, userID, cardID).Scan(&exists); err != nil {
		return err
	}
	if !exists {
		return ErrNotFound
	}
	result, err := tx.ExecContext(ctx, `INSERT INTO app_growth_insight_corrections(app_user_id,card_id,report_id,kind,note) VALUES($1,$2,$3,$4,$5) ON CONFLICT DO NOTHING`, userID, cardID, reportID, kind, note)
	if err != nil {
		return err
	}
	count, err := result.RowsAffected()
	if err != nil {
		return err
	}
	if count > 0 {
		if _, err = tx.ExecContext(ctx, `UPDATE app_growth_insight_consents SET revision=revision+1 WHERE app_user_id=$1`, userID); err != nil {
			return err
		}
	}
	return tx.Commit()
}

func storeError(err error) error {
	if errors.Is(err, sql.ErrNoRows) {
		return ErrNotFound
	}
	return err
}
