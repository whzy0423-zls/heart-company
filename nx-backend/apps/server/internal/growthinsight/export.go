package growthinsight

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"time"
)

type CorrectionExport struct {
	ReportID  int64     `json:"reportId"`
	Kind      string    `json:"kind"`
	Note      string    `json:"note"`
	CreatedAt time.Time `json:"createdAt"`
}

type ExportData struct {
	Consent     ConsentState       `json:"consent"`
	Reports     []UserView         `json:"reports"`
	Actions     []Action           `json:"actions"`
	Corrections []CorrectionExport `json:"corrections"`
}

// Export includes all own derived projections as a data-rights exception to the publication cadence.
// It is independent of membership and never includes internal evidence or uncertainties.
func (s *Store) Export(ctx context.Context, userID int64) (ExportData, error) {
	data := ExportData{Reports: []UserView{}, Actions: []Action{}, Corrections: []CorrectionExport{}}
	tx, err := s.db.BeginTx(ctx, &sql.TxOptions{Isolation: sql.LevelRepeatableRead})
	if err != nil {
		return data, err
	}
	defer tx.Rollback()
	var active bool
	if err = tx.QueryRowContext(ctx, `SELECT status='active' FROM app_users WHERE id=$1 FOR SHARE`, userID).Scan(&active); err != nil {
		return data, storeError(err)
	}
	if !active {
		return data, tx.Commit()
	}
	err = tx.QueryRowContext(ctx, `SELECT enabled,COALESCE(published_report_id,0),disclosure_version FROM app_growth_insight_consents WHERE app_user_id=$1 FOR SHARE`, userID).Scan(&data.Consent.Enabled, &data.Consent.ReportID, &data.Consent.DisclosureVersion)
	if errors.Is(err, sql.ErrNoRows) {
		return data, tx.Commit()
	}
	if err != nil {
		return data, err
	}
	if !data.Consent.Enabled {
		return data, tx.Commit()
	}
	rows, err := tx.QueryContext(ctx, `SELECT r.id,r.payload FROM app_growth_insight_reports r
JOIN app_user_cards c ON c.id=r.card_id AND c.app_user_id=r.app_user_id AND c.card_type='primary' AND c.status='active'
WHERE r.app_user_id=$1 ORDER BY r.version`, userID)
	if err != nil {
		return data, err
	}
	byReport := map[int64]int{}
	for rows.Next() {
		var id int64
		var body []byte
		var r Report
		if err = rows.Scan(&id, &body); err != nil {
			rows.Close()
			return data, err
		}
		if err = json.Unmarshal(body, &r); err != nil {
			rows.Close()
			return data, err
		}
		r.ID = id
		byReport[id] = len(data.Reports)
		data.Reports = append(data.Reports, project(r, nil))
	}
	err = rows.Err()
	rows.Close()
	if err != nil {
		return data, err
	}
	rows, err = tx.QueryContext(ctx, `SELECT `+actionColumns+` FROM app_growth_insight_actions WHERE app_user_id=$1 ORDER BY id`, userID)
	if err != nil {
		return data, err
	}
	for rows.Next() {
		a, err := scanAction(rows)
		if err != nil {
			rows.Close()
			return data, err
		}
		if index, ok := byReport[a.ReportID]; ok {
			data.Actions = append(data.Actions, a)
			data.Reports[index].Actions = append(data.Reports[index].Actions, a)
		}
	}
	err = rows.Err()
	rows.Close()
	if err != nil {
		return data, err
	}
	rows, err = tx.QueryContext(ctx, `SELECT report_id,kind,note,created_at FROM app_growth_insight_corrections WHERE app_user_id=$1 ORDER BY id`, userID)
	if err != nil {
		return data, err
	}
	for rows.Next() {
		var c CorrectionExport
		if err = rows.Scan(&c.ReportID, &c.Kind, &c.Note, &c.CreatedAt); err != nil {
			rows.Close()
			return data, err
		}
		if _, ok := byReport[c.ReportID]; ok {
			data.Corrections = append(data.Corrections, c)
		}
	}
	err = rows.Err()
	rows.Close()
	if err != nil {
		return data, err
	}
	return data, tx.Commit()
}
