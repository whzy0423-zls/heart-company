package videoproject

import (
	"context"
	"database/sql"
	"fmt"
	"strings"
	"time"
)

type ProjectScriptState struct {
	ProjectID         string    `json:"projectId"`
	Content           string    `json:"content"`
	Summary           string    `json:"summary"`
	Style             string    `json:"style"`
	Revision          int       `json:"revision"`
	ConfirmedRevision int       `json:"confirmedRevision"`
	ConfirmedAt       time.Time `json:"-"`
	ConfirmedAtText   string    `json:"confirmedAt"`
}

type SaveProjectScriptInput struct {
	ExpectedRevision int    `json:"expectedRevision"`
	Content          string `json:"content"`
	Summary          string `json:"summary"`
	Style            string `json:"style"`
}

func applyProjectScriptSave(current ProjectScriptState, input SaveProjectScriptInput) (ProjectScriptState, bool, error) {
	if input.ExpectedRevision != current.Revision {
		return ProjectScriptState{}, false, revisionConflict(current.Revision, "剧本已在其他页面更新，请刷新后再保存")
	}
	next := current
	next.Content = strings.TrimSpace(input.Content)
	next.Summary = strings.TrimSpace(input.Summary)
	next.Style = strings.TrimSpace(input.Style)
	if next == current {
		return current, false, nil
	}
	if next.Content != current.Content {
		next.Revision++
	}
	return next, true, nil
}

func applyProjectScriptConfirm(current ProjectScriptState, expectedRevision int, confirmedAt time.Time) (ProjectScriptState, bool, error) {
	if expectedRevision != current.Revision {
		return ProjectScriptState{}, false, revisionConflict(current.Revision, "剧本版本已变化，请查看最新内容后再确认")
	}
	if strings.TrimSpace(current.Content) == "" {
		return ProjectScriptState{}, false, &WorkflowValidationError{
			Code:    "script_empty",
			Field:   "content",
			Message: "剧本内容不能为空",
			Fix:     "请填写一句创意或完整剧本后再确认。",
			Details: map[string]any{},
		}
	}
	if current.ConfirmedRevision == current.Revision {
		return current, false, nil
	}
	next := current
	next.ConfirmedRevision = next.Revision
	next.ConfirmedAt = confirmedAt
	next.ConfirmedAtText = formatWorkflowTime(confirmedAt)
	return next, true, nil
}

func revisionConflict(currentRevision int, message string) *WorkflowConflictError {
	return &WorkflowConflictError{
		Code:            "workflow_revision_conflict",
		Message:         message,
		CurrentRevision: currentRevision,
		Details:         map[string]any{"currentRevision": currentRevision},
	}
}

func (store *Store) GetProjectScript(ctx context.Context, projectID string) (ProjectScriptState, error) {
	parsedProjectID, err := parseID(projectID)
	if err != nil {
		return ProjectScriptState{}, err
	}
	var state ProjectScriptState
	var confirmedAt sql.NullTime
	if err := store.db.QueryRowContext(ctx,
		`SELECT id::text, script_content, script_summary, style_guide,
		        script_revision, confirmed_script_revision, script_confirmed_at
		   FROM video_projects
		  WHERE id=$1`,
		parsedProjectID,
	).Scan(
		&state.ProjectID, &state.Content, &state.Summary, &state.Style,
		&state.Revision, &state.ConfirmedRevision, &confirmedAt,
	); err != nil {
		if err == sql.ErrNoRows {
			return ProjectScriptState{}, fmt.Errorf("项目不存在")
		}
		return ProjectScriptState{}, err
	}
	if confirmedAt.Valid {
		state.ConfirmedAt = confirmedAt.Time
		state.ConfirmedAtText = formatWorkflowTime(confirmedAt.Time)
	}
	return state, nil
}

func (store *Store) SaveProjectScript(ctx context.Context, projectID string, input SaveProjectScriptInput) (ProjectScriptState, error) {
	parsedProjectID, err := parseID(projectID)
	if err != nil {
		return ProjectScriptState{}, err
	}
	transaction, err := store.db.BeginTx(ctx, nil)
	if err != nil {
		return ProjectScriptState{}, err
	}
	defer transaction.Rollback()

	current, err := loadProjectScriptForUpdate(ctx, transaction, parsedProjectID)
	if err != nil {
		return ProjectScriptState{}, err
	}
	next, changed, err := applyProjectScriptSave(current, input)
	if err != nil {
		return ProjectScriptState{}, err
	}
	if !changed {
		if err := transaction.Commit(); err != nil {
			return ProjectScriptState{}, err
		}
		return current, nil
	}
	if _, err := transaction.ExecContext(ctx,
		`UPDATE video_projects
		    SET script_content=$1, script_summary=$2, style_guide=$3,
		        script_revision=$4, update_time=now()
		  WHERE id=$5 AND script_revision=$6`,
		next.Content, next.Summary, next.Style, next.Revision, parsedProjectID, current.Revision,
	); err != nil {
		return ProjectScriptState{}, err
	}
	if err := transaction.Commit(); err != nil {
		return ProjectScriptState{}, err
	}
	return next, nil
}

func (store *Store) ConfirmProjectScript(ctx context.Context, projectID string, expectedRevision int) (ProjectScriptState, error) {
	parsedProjectID, err := parseID(projectID)
	if err != nil {
		return ProjectScriptState{}, err
	}
	transaction, err := store.db.BeginTx(ctx, nil)
	if err != nil {
		return ProjectScriptState{}, err
	}
	defer transaction.Rollback()

	current, err := loadProjectScriptForUpdate(ctx, transaction, parsedProjectID)
	if err != nil {
		return ProjectScriptState{}, err
	}
	confirmedAt := time.Now().UTC()
	next, changed, err := applyProjectScriptConfirm(current, expectedRevision, confirmedAt)
	if err != nil {
		return ProjectScriptState{}, err
	}
	if changed {
		if _, err := transaction.ExecContext(ctx,
			`UPDATE video_projects
			    SET confirmed_script_revision=$1, script_confirmed_at=$2, update_time=now()
			  WHERE id=$3 AND script_revision=$1`,
			next.Revision, next.ConfirmedAt, parsedProjectID,
		); err != nil {
			return ProjectScriptState{}, err
		}
	}
	if err := transaction.Commit(); err != nil {
		return ProjectScriptState{}, err
	}
	return next, nil
}

type scriptQueryRow interface {
	Scan(dest ...any) error
}

type scriptQueryer interface {
	QueryRowContext(ctx context.Context, query string, args ...any) *sql.Row
}

func loadProjectScriptForUpdate(ctx context.Context, transaction *sql.Tx, projectID int64) (ProjectScriptState, error) {
	var state ProjectScriptState
	var confirmedAt sql.NullTime
	if err := transaction.QueryRowContext(ctx,
		`SELECT id::text, script_content, script_summary, style_guide,
		        script_revision, confirmed_script_revision, script_confirmed_at
		   FROM video_projects
		  WHERE id=$1
		  FOR UPDATE`,
		projectID,
	).Scan(
		&state.ProjectID, &state.Content, &state.Summary, &state.Style,
		&state.Revision, &state.ConfirmedRevision, &confirmedAt,
	); err != nil {
		if err == sql.ErrNoRows {
			return ProjectScriptState{}, fmt.Errorf("项目不存在")
		}
		return ProjectScriptState{}, err
	}
	if confirmedAt.Valid {
		state.ConfirmedAt = confirmedAt.Time
		state.ConfirmedAtText = formatWorkflowTime(confirmedAt.Time)
	}
	return state, nil
}

func formatWorkflowTime(value time.Time) string {
	if value.IsZero() {
		return ""
	}
	return value.UTC().Format(time.RFC3339)
}
