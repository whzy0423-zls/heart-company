package videoproject

import (
	"testing"

	"nine-xing/nx-backend/apps/server/internal/video"
)

func TestWorkflowStatusComputesSevenAuthoritativeSteps(t *testing.T) {
	complete := completeWorkflowStatusInput()

	tests := []struct {
		name        string
		input       WorkflowStatusInput
		wantCurrent string
		wantOverall int
		wantStatus  map[string]string
	}{
		{
			name:        "complete current workflow",
			input:       complete,
			wantCurrent: "compose",
			wantOverall: 100,
			wantStatus: map[string]string{
				"script": "completed", "breakdown": "completed", "assets": "completed",
				"storyboard": "completed", "prompt": "completed", "generate": "completed", "compose": "completed",
			},
		},
		{
			name: "empty new project",
			input: WorkflowStatusInput{
				Project:      Project{ID: "2", Name: "新项目"},
				Capabilities: video.Capabilities{Model: "video-ds-2.0", CapabilityVersion: "capability-v1"},
			},
			wantCurrent: "script",
			wantOverall: 0,
			wantStatus: map[string]string{
				"script": "blocked", "breakdown": "blocked", "assets": "blocked",
				"storyboard": "blocked", "prompt": "blocked", "generate": "blocked", "compose": "blocked",
			},
		},
		{
			name: "legacy project starts from existing shots",
			input: WorkflowStatusInput{
				Project:          Project{ID: "3", Name: "旧项目"},
				LegacyAssetCount: 2,
				LegacyShotCount:  1,
				AssetRevision:    0,
				Capabilities:     video.Capabilities{Model: "video-ds-2.0", CapabilityVersion: "capability-v1"},
				Assets:           []WorkflowAssetState{{ID: "8", Kind: "character", Required: true, Status: "ready", Selected: true, SelectedURL: "https://cdn.example.com/a.png"}},
				Shots:            []WorkflowShotState{{ID: "9", Enabled: true, CurrentDiagnosticsHash: "diag-current"}},
			},
			wantCurrent: "prompt",
			wantOverall: 57,
			wantStatus: map[string]string{
				"script": "skipped_existing", "breakdown": "skipped_existing", "assets": "completed",
				"storyboard": "skipped_existing", "prompt": "pending", "generate": "blocked", "compose": "blocked",
			},
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			overview := ComputeWorkflowOverview(test.input)
			if overview.CurrentStep != test.wantCurrent || overview.Overall != test.wantOverall {
				t.Fatalf("current/overall = %s/%d, want %s/%d; overview=%+v", overview.CurrentStep, overview.Overall, test.wantCurrent, test.wantOverall, overview)
			}
			if len(overview.Steps) != 7 {
				t.Fatalf("expected seven steps, got %+v", overview.Steps)
			}
			for key, want := range test.wantStatus {
				step := workflowStepForTest(t, overview, key)
				if step.Status != want {
					t.Fatalf("step %s status = %q, want %q; step=%+v", key, step.Status, want, step)
				}
			}
		})
	}
}

func TestWorkflowStatusEvidenceContainsExactDependencies(t *testing.T) {
	overview := ComputeWorkflowOverview(completeWorkflowStatusInput())

	script := workflowStepForTest(t, overview, "script")
	if script.Evidence["scriptRevision"] != 4 || script.Evidence["confirmedScriptRevision"] != 4 {
		t.Fatalf("unexpected script evidence: %+v", script.Evidence)
	}
	breakdown := workflowStepForTest(t, overview, "breakdown")
	if breakdown.Evidence["breakdownId"] != "20" || breakdown.Evidence["sourceScriptRevision"] != 4 {
		t.Fatalf("unexpected breakdown evidence: %+v", breakdown.Evidence)
	}
	assets := workflowStepForTest(t, overview, "assets")
	if assets.Evidence["assetRevision"] != 7 || assets.Evidence["requiredCount"] != 2 || assets.Evidence["readyCount"] != 2 {
		t.Fatalf("unexpected asset evidence: %+v", assets.Evidence)
	}
	storyboard := workflowStepForTest(t, overview, "storyboard")
	if storyboard.Evidence["storyboardId"] != "30" || storyboard.Evidence["sourceBreakdownId"] != "20" || storyboard.Evidence["sourceAssetRevision"] != 7 || storyboard.Evidence["capabilityVersion"] != "capability-v1" {
		t.Fatalf("unexpected storyboard evidence: %+v", storyboard.Evidence)
	}
	prompt := workflowStepForTest(t, overview, "prompt")
	if prompt.Evidence["capabilityVersion"] != "capability-v1" || prompt.Evidence["validatedCount"] != 2 {
		t.Fatalf("unexpected prompt evidence: %+v", prompt.Evidence)
	}
	generate := workflowStepForTest(t, overview, "generate")
	if generate.Evidence["selectedCount"] != 2 || generate.Evidence["currentCount"] != 2 {
		t.Fatalf("unexpected selection evidence: %+v", generate.Evidence)
	}
	compose := workflowStepForTest(t, overview, "compose")
	if compose.Evidence["savedInputHash"] != "compose-hash" || compose.Evidence["currentInputHash"] != "compose-hash" {
		t.Fatalf("unexpected compose evidence: %+v", compose.Evidence)
	}
}

func TestWorkflowStatusTargetDurationVarianceIsWarningOnly(t *testing.T) {
	input := completeWorkflowStatusInput()
	input.TargetDuration = 60
	input.Shots[0].Duration = 5
	input.Shots[1].Duration = 5

	overview := ComputeWorkflowOverview(input)
	storyboard := workflowStepForTest(t, overview, "storyboard")
	if storyboard.Status != "completed" {
		t.Fatalf("target duration variance must not block storyboard, got %+v", storyboard)
	}
	if len(storyboard.Warnings) == 0 || storyboard.Warnings[0].Code != "target_duration_variance" {
		t.Fatalf("expected duration warning, got %+v", storyboard.Warnings)
	}
}

func TestWorkflowInvalidationPreservesResultsAndMarksOnlyDependenciesStale(t *testing.T) {
	tests := []struct {
		name       string
		mutate     func(*WorkflowStatusInput)
		wantStale  []string
		wantRemain []string
	}{
		{
			name: "script edit invalidates all downstream",
			mutate: func(input *WorkflowStatusInput) {
				input.Script.Revision = 5
				input.Script.Content = "修改后的剧本"
			},
			wantStale: []string{"script", "breakdown", "assets", "storyboard", "prompt", "generate", "compose"},
		},
		{
			name: "new breakdown revision invalidates assets onward",
			mutate: func(input *WorkflowStatusInput) {
				input.ConfirmedBreakdown.ID = "21"
				input.AssetRevision = 8
			},
			wantStale:  []string{"assets", "storyboard", "prompt", "generate", "compose"},
			wantRemain: []string{"script", "breakdown"},
		},
		{
			name: "candidate selection invalidates storyboard onward",
			mutate: func(input *WorkflowStatusInput) {
				input.AssetRevision = 8
			},
			wantStale:  []string{"storyboard", "prompt", "generate", "compose"},
			wantRemain: []string{"script", "breakdown", "assets"},
		},
		{
			name: "prompt diagnostics hash invalidates prompt onward",
			mutate: func(input *WorkflowStatusInput) {
				input.Shots[0].CurrentDiagnosticsHash = "diag-new"
				input.Shots[0].CurrentRequestHash = "request-new"
			},
			wantStale:  []string{"prompt", "generate", "compose"},
			wantRemain: []string{"script", "breakdown", "assets", "storyboard"},
		},
		{
			name: "stale selected generation requires acknowledgement",
			mutate: func(input *WorkflowStatusInput) {
				input.Shots[0].CurrentRequestHash = "request-new"
			},
			wantStale:  []string{"generate", "compose"},
			wantRemain: []string{"script", "breakdown", "assets", "storyboard", "prompt"},
		},
		{
			name: "compose settings change only invalidates compose",
			mutate: func(input *WorkflowStatusInput) {
				input.Compose.CurrentInputHash = "compose-new"
			},
			wantStale:  []string{"compose"},
			wantRemain: []string{"script", "breakdown", "assets", "storyboard", "prompt", "generate"},
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			input := completeWorkflowStatusInput()
			test.mutate(&input)
			overview := ComputeWorkflowOverview(input)
			for _, key := range test.wantStale {
				status := workflowStepForTest(t, overview, key).Status
				if status != "stale" {
					t.Fatalf("step %s status = %q, want stale; overview=%+v", key, status, overview.Steps)
				}
			}
			for _, key := range test.wantRemain {
				status := workflowStepForTest(t, overview, key).Status
				if status != "completed" {
					t.Fatalf("step %s status = %q, want completed; overview=%+v", key, status, overview.Steps)
				}
			}
			if input.Compose.VideoURL == "" || len(input.Shots) != 2 || input.Shots[0].SelectedGenerationID == "" {
				t.Fatal("invalidation evaluation must not delete existing results")
			}
		})
	}
}

func TestWorkflowInvalidationAcceptsExplicitStaleSelectionAcknowledgement(t *testing.T) {
	input := completeWorkflowStatusInput()
	input.Shots[0].CurrentRequestHash = "request-new"
	input.Shots[0].SelectedGenerationAckHash = SelectionAckHash("request-new", input.Shots[0].SelectedGenerationID)
	overview := ComputeWorkflowOverview(input)
	if step := workflowStepForTest(t, overview, "generate"); step.Status != "completed" {
		t.Fatalf("explicit acknowledgement should keep old selection, got %+v", step)
	}
}

func completeWorkflowStatusInput() WorkflowStatusInput {
	breakdown := NewBreakdownVersion()
	breakdown.ID = "20"
	breakdown.ProjectID = "1"
	breakdown.Status = "confirmed"
	breakdown.Revision = 2
	breakdown.SourceScriptRevision = 4
	breakdown.Characters = []BreakdownItem{{Key: "character-a", Name: "阿宁", Decision: "confirmed"}}
	breakdown.Scenes = []BreakdownItem{{Key: "scene-a", Name: "雨夜车站", Decision: "confirmed"}}
	breakdown.Props = []BreakdownItem{{Key: "prop-a", Name: "旧相机", Decision: "ignored"}}

	storyboard := NewStoryboardVersion()
	storyboard.ID = "30"
	storyboard.ProjectID = "1"
	storyboard.Status = "confirmed"
	storyboard.SourceScriptRevision = 4
	storyboard.SourceBreakdownID = "20"
	storyboard.SourceAssetRevision = 7
	storyboard.SourceCapabilityVersion = "capability-v1"
	storyboard.Shots = []StoryboardShot{{SourceKey: "shot-a", Enabled: true}, {SourceKey: "shot-b", Enabled: true}}

	return WorkflowStatusInput{
		Project:            Project{ID: "1", Name: "雨夜相机"},
		Script:             ProjectScriptState{ProjectID: "1", Content: "完整剧本", Revision: 4, ConfirmedRevision: 4},
		ConfirmedBreakdown: &breakdown,
		Assets: []WorkflowAssetState{
			{ID: "7", Kind: "character", ItemKey: "character-a", SourceBreakdownID: "20", Required: true, Status: "ready", Selected: true, SelectedURL: "https://cdn.example.com/a.png"},
			{ID: "8", Kind: "scene", ItemKey: "scene-a", SourceBreakdownID: "20", Required: true, Status: "ready", Selected: true, SelectedURL: "https://cdn.example.com/b.png"},
		},
		AssetRevision:       7,
		ConfirmedStoryboard: &storyboard,
		Shots: []WorkflowShotState{
			{
				ID: "40", SourceKey: "shot-a", Enabled: true, Duration: 10,
				SavedDiagnosticsHash: "diag-a", CurrentDiagnosticsHash: "diag-a", CurrentRequestHash: "request-a",
				SelectedGenerationID: "50", SelectedGenerationStatus: "completed", SelectedGenerationRequestHash: "request-a",
			},
			{
				ID: "41", SourceKey: "shot-b", Enabled: true, Duration: 10,
				SavedDiagnosticsHash: "diag-b", CurrentDiagnosticsHash: "diag-b", CurrentRequestHash: "request-b",
				SelectedGenerationID: "51", SelectedGenerationStatus: "succeeded", SelectedGenerationRequestHash: "request-b",
			},
		},
		Capabilities:   video.Capabilities{Model: "video-ds-2.0", CapabilityVersion: "capability-v1"},
		TargetDuration: 20,
		Compose: WorkflowComposeState{
			Status: "completed", VideoURL: "https://cdn.example.com/final.mp4",
			SavedInputHash: "compose-hash", CurrentInputHash: "compose-hash",
		},
	}
}

func workflowStepForTest(t *testing.T, overview WorkflowOverview, key string) WorkflowStepStatus {
	t.Helper()
	for _, step := range overview.Steps {
		if step.Key == key {
			return step
		}
	}
	t.Fatalf("step %q not found in %+v", key, overview.Steps)
	return WorkflowStepStatus{}
}
