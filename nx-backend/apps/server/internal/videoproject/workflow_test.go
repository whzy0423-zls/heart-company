package videoproject

import (
	"context"
	"sync"
	"testing"

	"nine-xing/nx-backend/apps/server/internal/video"
)

func TestShotReadiness(t *testing.T) {
	current := &SelectedVersionFacts{Status: "completed", VideoURL: "https://cdn/video.mp4", ShotRevision: 3}
	stale := &SelectedVersionFacts{Status: "completed", VideoURL: "https://cdn/video.mp4", ShotRevision: 2}
	failedSelection := &SelectedVersionFacts{Status: "failed", VideoURL: "", ShotRevision: 3}
	cases := []struct {
		name  string
		facts ShotWorkflowFacts
		want  ShotReadiness
	}{
		{name: "unknown outcome wins", facts: ShotWorkflowFacts{ActionDescription: "action", SubmissionStatus: "unknown_outcome", Selected: current}, want: ReadinessRecovery},
		{name: "prepared active", facts: ShotWorkflowFacts{ActionDescription: "action", SubmissionStatus: "prepared"}, want: ReadinessGenerating},
		{name: "submitting active", facts: ShotWorkflowFacts{ActionDescription: "action", SubmissionStatus: "submitting"}, want: ReadinessGenerating},
		{name: "accepted active", facts: ShotWorkflowFacts{ActionDescription: "action", SubmissionStatus: "accepted"}, want: ReadinessGenerating},
		{name: "reconciled active", facts: ShotWorkflowFacts{ActionDescription: "action", SubmissionStatus: "reconciled"}, want: ReadinessGenerating},
		{name: "linked active", facts: ShotWorkflowFacts{ActionDescription: "action", LinkedTaskActive: true}, want: ReadinessGenerating},
		{name: "missing action", facts: ShotWorkflowFacts{}, want: ReadinessIncomplete},
		{name: "selected missing url", facts: ShotWorkflowFacts{ActionDescription: "action", GenerationRevision: 3, Selected: &SelectedVersionFacts{Status: "completed", ShotRevision: 3}}, want: ReadinessFailed},
		{name: "current selection", facts: ShotWorkflowFacts{ActionDescription: "action", GenerationRevision: 3, Selected: current}, want: ReadinessCompleted},
		{name: "stale selection", facts: ShotWorkflowFacts{ActionDescription: "action", GenerationRevision: 3, Selected: stale}, want: ReadinessStale},
		{name: "terminal failure", facts: ShotWorkflowFacts{ActionDescription: "action", LatestStatus: "failed", Selected: failedSelection}, want: ReadinessFailed},
		{name: "failed attempt with valid selection", facts: ShotWorkflowFacts{ActionDescription: "action", GenerationRevision: 3, LatestStatus: "failed", Selected: current}, want: ReadinessCompleted},
		{name: "ready", facts: ShotWorkflowFacts{ActionDescription: "action", GenerationRevision: 3}, want: ReadinessReady},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := ComputeShotReadiness(tc.facts); got != tc.want {
				t.Fatalf("ComputeShotReadiness=%s, want %s", got, tc.want)
			}
		})
	}
}

func TestWorkflowStepPredicates(t *testing.T) {
	cases := []struct {
		name  string
		facts WorkflowFacts
		step  WorkflowStep
		want  WorkflowStepState
	}{
		{name: "brief complete", facts: WorkflowFacts{ScriptContent: "script"}, step: StepBrief, want: StepComplete},
		{name: "brief legacy skipped", facts: WorkflowFacts{ShotReadiness: []ShotReadiness{ReadinessReady}}, step: StepBrief, want: StepSkippedExisting},
		{name: "assets optional", facts: WorkflowFacts{}, step: StepAssets, want: StepOptional},
		{name: "assets complete", facts: WorkflowFacts{AssetCount: 1}, step: StepAssets, want: StepComplete},
		{name: "assets legacy skipped", facts: WorkflowFacts{ShotReadiness: []ShotReadiness{ReadinessReady}}, step: StepAssets, want: StepSkippedExisting},
		{name: "storyboard blocked", facts: WorkflowFacts{}, step: StepStoryboard, want: StepBlocked},
		{name: "storyboard complete", facts: WorkflowFacts{ShotReadiness: []ShotReadiness{ReadinessReady}}, step: StepStoryboard, want: StepComplete},
		{name: "generate stale", facts: WorkflowFacts{ShotReadiness: []ShotReadiness{ReadinessCompleted, ReadinessStale}}, step: StepGenerate, want: StepStale},
		{name: "generate complete", facts: WorkflowFacts{ShotReadiness: []ShotReadiness{ReadinessCompleted}}, step: StepGenerate, want: StepComplete},
		{name: "export stale", facts: WorkflowFacts{FinalVideoURL: "url", FinalVideoCurrent: false}, step: StepExport, want: StepStale},
		{name: "export complete", facts: WorkflowFacts{FinalVideoURL: "url", FinalVideoCurrent: true}, step: StepExport, want: StepComplete},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := ComputeWorkflowStepState(tc.facts, tc.step); got != tc.want {
				t.Fatalf("ComputeWorkflowStepState=%s, want %s", got, tc.want)
			}
		})
	}
}

func TestRecommendedWorkflowStep(t *testing.T) {
	cases := []struct {
		name  string
		facts WorkflowFacts
		want  WorkflowStep
	}{
		{name: "empty", facts: WorkflowFacts{}, want: StepBrief},
		{name: "assets only", facts: WorkflowFacts{AssetCount: 2}, want: StepBrief},
		{name: "shots legacy", facts: WorkflowFacts{ShotReadiness: []ShotReadiness{ReadinessIncomplete}}, want: StepStoryboard},
		{name: "ready shots", facts: WorkflowFacts{ShotReadiness: []ShotReadiness{ReadinessReady}}, want: StepGenerate},
		{name: "generated", facts: WorkflowFacts{ShotReadiness: []ShotReadiness{ReadinessCompleted}}, want: StepExport},
		{name: "final", facts: WorkflowFacts{ShotReadiness: []ShotReadiness{ReadinessCompleted}, FinalVideoURL: "url", FinalVideoCurrent: true}, want: StepExport},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := RecommendedWorkflowStep(tc.facts); got != tc.want {
				t.Fatalf("RecommendedWorkflowStep=%s, want %s", got, tc.want)
			}
		})
	}
}

func TestBatchGenerationReadinessScope(t *testing.T) {
	readiness := map[string]ShotReadiness{
		"ready":      ReadinessReady,
		"stale":      ReadinessStale,
		"failed":     ReadinessFailed,
		"incomplete": ReadinessIncomplete,
		"generating": ReadinessGenerating,
		"recovery":   ReadinessRecovery,
		"completed":  ReadinessCompleted,
	}
	requested := []string{"ready", "stale", "failed", "incomplete", "generating", "recovery", "completed", "foreign"}
	got := FilterGeneratableShotIDs(readiness, requested)
	want := []string{"ready", "stale", "failed"}
	if len(got) != len(want) {
		t.Fatalf("FilterGeneratableShotIDs=%v, want %v", got, want)
	}
	for index := range want {
		if got[index] != want[index] {
			t.Fatalf("FilterGeneratableShotIDs=%v, want %v", got, want)
		}
	}
}

func TestBatchGenerationReadinessScopeDoesNotCallExcludedShots(t *testing.T) {
	shots := map[string]Shot{
		"ready":      {ID: "ready", Name: "ready"},
		"stale":      {ID: "stale", Name: "stale"},
		"failed":     {ID: "failed", Name: "failed"},
		"incomplete": {ID: "incomplete", Name: "incomplete"},
		"generating": {ID: "generating", Name: "generating"},
		"recovery":   {ID: "recovery", Name: "recovery"},
		"completed":  {ID: "completed", Name: "completed"},
	}
	readiness := map[string]ShotReadiness{
		"ready": ReadinessReady, "stale": ReadinessStale, "failed": ReadinessFailed,
		"incomplete": ReadinessIncomplete, "generating": ReadinessGenerating,
		"recovery": ReadinessRecovery, "completed": ReadinessCompleted,
	}
	items := []SafeBatchGenerateItem{
		{ShotID: "ready", RequestKey: "key-ready"},
		{ShotID: "stale", RequestKey: "key-stale"},
		{ShotID: "failed", RequestKey: "key-failed"},
		{ShotID: "incomplete", RequestKey: "key-incomplete"},
		{ShotID: "generating", RequestKey: "key-generating"},
		{ShotID: "recovery", RequestKey: "key-recovery"},
		{ShotID: "completed", RequestKey: "key-completed"},
		{ShotID: "foreign", RequestKey: "key-foreign"},
	}
	generator := &recordingShotGenerator{}
	result := executeSafeBatch(context.Background(), generator, "project", shots, readiness, items, true)
	if result.FailedCount != 0 || result.SuccessCount != 3 {
		t.Fatalf("unexpected batch result: %+v", result)
	}
	want := map[string]string{"ready": "key-ready", "stale": "key-stale", "failed": "key-failed"}
	if len(generator.calls) != len(want) {
		t.Fatalf("generator calls=%v, want %v", generator.calls, want)
	}
	for shotID, requestKey := range want {
		if generator.calls[shotID] != requestKey {
			t.Fatalf("shot %s request key=%q, want %q", shotID, generator.calls[shotID], requestKey)
		}
	}
}

type recordingShotGenerator struct {
	mu    sync.Mutex
	calls map[string]string
}

func (g *recordingShotGenerator) GenerateShot(_ context.Context, shotID string, requestKeys ...string) (video.Generation, error) {
	g.mu.Lock()
	defer g.mu.Unlock()
	if g.calls == nil {
		g.calls = map[string]string{}
	}
	requestKey := ""
	if len(requestKeys) > 0 {
		requestKey = requestKeys[0]
	}
	g.calls[shotID] = requestKey
	return video.Generation{ID: "generation-" + shotID}, nil
}
