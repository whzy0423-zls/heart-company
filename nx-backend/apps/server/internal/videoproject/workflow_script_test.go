package videoproject

import (
	"errors"
	"testing"
	"time"
)

func TestScriptRevisionIncrementsOnlyWhenContentChanges(t *testing.T) {
	current := ProjectScriptState{
		ProjectID:         "12",
		Content:           "第一场：雨夜车站。",
		Summary:           "旧摘要",
		Style:             "旧风格",
		Revision:          2,
		ConfirmedRevision: 2,
	}

	metadataOnly, changed, err := applyProjectScriptSave(current, SaveProjectScriptInput{
		ExpectedRevision: 2,
		Content:          current.Content,
		Summary:          "新摘要",
		Style:            "新风格",
	})
	if err != nil {
		t.Fatal(err)
	}
	if !changed || metadataOnly.Revision != 2 || metadataOnly.ConfirmedRevision != 2 {
		t.Fatalf("metadata-only save must not invalidate the confirmed script: changed=%v state=%+v", changed, metadataOnly)
	}

	edited, changed, err := applyProjectScriptSave(metadataOnly, SaveProjectScriptInput{
		ExpectedRevision: 2,
		Content:          "第一场：清晨车站。",
		Summary:          metadataOnly.Summary,
		Style:            metadataOnly.Style,
	})
	if err != nil {
		t.Fatal(err)
	}
	if !changed || edited.Revision != 3 || edited.ConfirmedRevision != 2 {
		t.Fatalf("content edit must increment only the current revision: changed=%v state=%+v", changed, edited)
	}
}

func TestScriptRevisionNoOpSaveIsIdempotent(t *testing.T) {
	current := ProjectScriptState{ProjectID: "12", Content: "剧本", Summary: "摘要", Style: "风格", Revision: 4, ConfirmedRevision: 3}
	got, changed, err := applyProjectScriptSave(current, SaveProjectScriptInput{
		ExpectedRevision: 4,
		Content:          "  剧本  ",
		Summary:          "摘要",
		Style:            "风格",
	})
	if err != nil {
		t.Fatal(err)
	}
	if changed || got != current {
		t.Fatalf("no-op save must preserve state exactly: changed=%v got=%+v", changed, got)
	}
}

func TestScriptRevisionRejectsStaleSaveAndConfirm(t *testing.T) {
	current := ProjectScriptState{ProjectID: "12", Content: "新剧本", Revision: 5, ConfirmedRevision: 4}
	_, _, err := applyProjectScriptSave(current, SaveProjectScriptInput{ExpectedRevision: 4, Content: "旧页面内容"})
	assertScriptRevisionConflict(t, err, 5)

	_, _, err = applyProjectScriptConfirm(current, 4, time.Now())
	assertScriptRevisionConflict(t, err, 5)
}

func TestScriptRevisionConfirmSetsCurrentRevisionWithoutDeletingDownstream(t *testing.T) {
	current := ProjectScriptState{ProjectID: "12", Content: "已完成剧本", Revision: 5, ConfirmedRevision: 4}
	confirmedAt := time.Date(2026, 7, 11, 8, 30, 0, 0, time.UTC)
	got, changed, err := applyProjectScriptConfirm(current, 5, confirmedAt)
	if err != nil {
		t.Fatal(err)
	}
	if !changed || got.ConfirmedRevision != 5 || !got.ConfirmedAt.Equal(confirmedAt) {
		t.Fatalf("unexpected confirmed script state: changed=%v got=%+v", changed, got)
	}

	again, changed, err := applyProjectScriptConfirm(got, 5, confirmedAt.Add(time.Minute))
	if err != nil {
		t.Fatal(err)
	}
	if changed || !again.ConfirmedAt.Equal(confirmedAt) {
		t.Fatalf("repeated confirmation must be idempotent: changed=%v got=%+v", changed, again)
	}
}

func TestScriptRevisionCannotConfirmEmptyContent(t *testing.T) {
	_, _, err := applyProjectScriptConfirm(ProjectScriptState{ProjectID: "12", Revision: 1}, 1, time.Now())
	var validation *WorkflowValidationError
	if !errors.As(err, &validation) || validation.Code != "script_empty" {
		t.Fatalf("error = %T %v, want script_empty validation", err, err)
	}
}

func assertScriptRevisionConflict(t *testing.T, err error, currentRevision int) {
	t.Helper()
	var conflict *WorkflowConflictError
	if !errors.As(err, &conflict) {
		t.Fatalf("error = %T %v, want *WorkflowConflictError", err, err)
	}
	if conflict.Code != "workflow_revision_conflict" || conflict.CurrentRevision != currentRevision {
		t.Fatalf("unexpected conflict: %+v", conflict)
	}
}
