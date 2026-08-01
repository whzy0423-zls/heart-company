package videoproject

import (
	"encoding/json"
	"errors"
	"strings"
	"testing"
)

func TestWorkflowModelsKeepEmptyCollectionsAsArrays(t *testing.T) {
	overview := NewWorkflowOverview()
	encoded, err := json.Marshal(overview)
	if err != nil {
		t.Fatal(err)
	}
	for _, field := range []string{"steps", "blockers", "warnings"} {
		if !strings.Contains(string(encoded), `"`+field+`":[]`) {
			t.Fatalf("workflow field %q must be a JSON array: %s", field, encoded)
		}
	}
}

func TestWorkflowModelJSONContract(t *testing.T) {
	breakdown := NewBreakdownVersion()
	breakdown.Characters = append(breakdown.Characters, BreakdownItem{
		Key: "character-1", Name: "小夏", Required: true, Decision: "pending",
	})
	storyboard := NewStoryboardVersion()
	storyboard.Shots = append(storyboard.Shots, StoryboardShot{
		SourceKey: "shot-1",
		References: []StoryboardReferenceIntent{{
			AssetKey: "character-1", Role: "reference_image", SortOrder: 1,
		}},
	})
	payload := struct {
		Breakdown  BreakdownVersion  `json:"breakdown"`
		Storyboard StoryboardVersion `json:"storyboard"`
	}{Breakdown: breakdown, Storyboard: storyboard}
	encoded, err := json.Marshal(payload)
	if err != nil {
		t.Fatal(err)
	}
	for _, fragment := range []string{
		`"sourceScriptRevision":0`,
		`"characters":[{"key":"character-1"`,
		`"decision":"pending"`,
		`"sourceKey":"shot-1"`,
		`"references":[{"assetKey":"character-1"`,
		`"role":"reference_image"`,
	} {
		if !strings.Contains(string(encoded), fragment) {
			t.Fatalf("workflow JSON missing %q: %s", fragment, encoded)
		}
	}
}

func TestWorkflowConflictErrorIsTyped(t *testing.T) {
	err := &WorkflowConflictError{
		Code:            "workflow_revision_conflict",
		Message:         "内容已被更新，请刷新后重试。",
		CurrentRevision: 3,
	}
	var conflict *WorkflowConflictError
	if !errors.As(err, &conflict) {
		t.Fatalf("error = %T, want *WorkflowConflictError", err)
	}
	if conflict.Error() != err.Message || conflict.Code == "" || conflict.CurrentRevision != 3 {
		t.Fatalf("conflict = %+v", conflict)
	}
}
