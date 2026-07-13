package videoproject

import (
	"context"
	"errors"
	"strings"
	"testing"

	"nine-xing/nx-backend/apps/server/internal/llm"
	"nine-xing/nx-backend/apps/server/internal/video"
)

func TestCreateStoryboardDraftPersistsDependenciesAndNewVersions(t *testing.T) {
	repository := &recordingStoryboardDraftRepository{
		snapshot: StoryboardDesignSnapshot{
			ProjectID:            "15",
			Script:               "第一场：阿宁在雨夜车站捡到旧相机。",
			ScriptRevision:       4,
			BreakdownID:          "23",
			AssetRevision:        8,
			BaselineStoryboardID: "6",
			Assets: []llm.ProjectAssetSummary{
				{Key: "character-aning", Type: "character", Name: "阿宁"},
				{Key: "scene-station", Type: "scene", Name: "雨夜车站"},
			},
		},
	}
	generator := &recordingProjectStoryboardGenerator{
		result: llm.ProjectStoryboardResult{Shots: []llm.ProjectStoryboardShot{{
			SourceKey: "shot-01", Name: "发现相机", Enabled: true, Duration: 10,
			SceneKey: "scene-station", CharacterKeys: []string{"character-aning"},
			Action: "阿宁捡起相机", Camera: "中景推进", TaskMode: "reference",
		}}},
		raw: `{"shots":[{"sourceKey":"shot-01"}]}`,
	}
	service := NewStoryboardWorkflowService(repository, generator)
	input := StoryboardDraftInput{
		ProjectID:   "15",
		AspectRatio: "9:16",
		Capabilities: video.Capabilities{
			Model:              "video-ds-2.0",
			CapabilityVersion:  "seedance-capability-v4",
			SupportedDurations: []int{5, 10, 15},
			TaskModes:          []string{"reference"},
			ReferenceRoles:     []string{"reference_image", "reference_video", "reference_audio"},
		},
	}

	first, err := service.CreateStoryboardDraft(context.Background(), input)
	if err != nil {
		t.Fatalf("CreateStoryboardDraft returned error: %v", err)
	}
	second, err := service.CreateStoryboardDraft(context.Background(), input)
	if err != nil {
		t.Fatalf("second CreateStoryboardDraft returned error: %v", err)
	}

	if first.Version != 1 || second.Version != 2 || first.ID == second.ID {
		t.Fatalf("each design request must create a new version, first=%+v second=%+v", first, second)
	}
	if first.Revision != 1 || first.Status != "draft" || first.SourceScriptRevision != 4 || first.SourceBreakdownID != "23" || first.SourceAssetRevision != 8 {
		t.Fatalf("unexpected dependency snapshot: %+v", first)
	}
	if first.SourceCapabilityVersion != "seedance-capability-v4" || first.BaselineStoryboardID != "6" {
		t.Fatalf("expected capability and confirmed baseline snapshot, got %+v", first)
	}
	if len(first.Shots) != 1 || first.Shots[0].SourceKey != "shot-01" || first.RawResult != generator.raw {
		t.Fatalf("expected cleaned shots and raw output, got %+v", first)
	}
	if len(generator.inputs) != 2 {
		t.Fatalf("expected two model calls, got %d", len(generator.inputs))
	}
	modelInput := generator.inputs[0]
	if modelInput.ScriptRevision != 4 || modelInput.BreakdownID != "23" || modelInput.AssetRevision != 8 || modelInput.CapabilityVersion != "seedance-capability-v4" {
		t.Fatalf("expected all dependencies in model input, got %+v", modelInput)
	}
	if modelInput.Model != "video-ds-2.0" || modelInput.AspectRatio != "9:16" || len(modelInput.Assets) != 2 {
		t.Fatalf("expected capabilities and confirmed assets in model input, got %+v", modelInput)
	}
	if len(repository.writes) != 2 || repository.writes[0].Status != "draft" || repository.writes[0].ErrorMessage != "" {
		t.Fatalf("unexpected storyboard writes: %+v", repository.writes)
	}
}

func TestCreateStoryboardDraftPersistsFailedVersionAndRawResult(t *testing.T) {
	repository := &recordingStoryboardDraftRepository{
		snapshot: StoryboardDesignSnapshot{
			ProjectID: "18", Script: "阿宁走进车站。", ScriptRevision: 2,
			BreakdownID: "31", AssetRevision: 3, BaselineStoryboardID: "9",
		},
	}
	generator := &recordingProjectStoryboardGenerator{
		raw: "模型这次没有返回 JSON",
		err: errors.New("AI 分镜模型未返回有效 JSON，请重新生成"),
	}
	service := NewStoryboardWorkflowService(repository, generator)

	draft, err := service.CreateStoryboardDraft(context.Background(), StoryboardDraftInput{
		ProjectID: "18",
		Capabilities: video.Capabilities{
			Model: "video-ds-2.0", CapabilityVersion: "capability-v2",
			SupportedDurations: []int{5, 10, 15}, TaskModes: []string{"reference"},
		},
	})
	if err == nil || !strings.Contains(err.Error(), "有效 JSON") {
		t.Fatalf("expected model error to reach caller, got %v", err)
	}
	if draft.Status != "failed" || draft.Version != 1 || draft.Revision != 1 {
		t.Fatalf("expected failed storyboard version, got %+v", draft)
	}
	if draft.RawResult != generator.raw || draft.ErrorMessage != generator.err.Error() {
		t.Fatalf("expected raw failure details to be persisted, got %+v", draft)
	}
	if draft.SourceBreakdownID != "31" || draft.SourceAssetRevision != 3 || draft.BaselineStoryboardID != "9" || draft.SourceCapabilityVersion != "capability-v2" {
		t.Fatalf("expected failed version dependency snapshot, got %+v", draft)
	}
}

func TestUpdateStoryboardDraftRequiresRevisionAndIncrementsOnce(t *testing.T) {
	draft := NewStoryboardVersion()
	draft.ID = "30"
	draft.ProjectID = "1"
	draft.Revision = 2
	draft.Status = "draft"
	draft.Shots = []StoryboardShot{{SourceKey: "shot-a", Name: "开场", Enabled: true, Duration: 5, Action: "阿宁走进车站", TaskMode: "reference"}}

	nextShots := []StoryboardShot{{SourceKey: "shot-a", Name: "开场", Enabled: true, Duration: 10, Action: "阿宁走进车站", TaskMode: "reference"}}
	updated, changed, err := applyStoryboardDraftUpdate(draft, UpdateStoryboardDraftInput{ExpectedRevision: 2, Shots: nextShots})
	if err != nil {
		t.Fatal(err)
	}
	if !changed || updated.Revision != 3 || updated.Shots[0].Duration != 10 {
		t.Fatalf("unexpected draft update: changed=%v draft=%+v", changed, updated)
	}

	again, changed, err := applyStoryboardDraftUpdate(updated, UpdateStoryboardDraftInput{ExpectedRevision: 3, Shots: nextShots})
	if err != nil {
		t.Fatal(err)
	}
	if changed || again.Revision != 3 {
		t.Fatalf("no-op update must be idempotent: changed=%v draft=%+v", changed, again)
	}

	_, _, err = applyStoryboardDraftUpdate(updated, UpdateStoryboardDraftInput{ExpectedRevision: 2, Shots: nextShots})
	assertWorkflowConflictCode(t, err, "workflow_revision_conflict")
}

func TestUpdateStoryboardDraftRejectsEmptyAndDuplicateSourceKeys(t *testing.T) {
	draft := NewStoryboardVersion()
	draft.ID = "30"
	draft.ProjectID = "1"
	draft.Revision = 1
	draft.Status = "draft"

	for _, test := range []struct {
		name  string
		shots []StoryboardShot
	}{
		{"empty", []StoryboardShot{{Name: "开场", Enabled: true}}},
		{"duplicate", []StoryboardShot{{SourceKey: "shot-a", Name: "开场", Enabled: true}, {SourceKey: "shot-a", Name: "结尾", Enabled: true}}},
	} {
		t.Run(test.name, func(t *testing.T) {
			_, _, err := applyStoryboardDraftUpdate(draft, UpdateStoryboardDraftInput{ExpectedRevision: 1, Shots: test.shots})
			var validation *WorkflowValidationError
			if !errors.As(err, &validation) || validation.Code != "storyboard_source_key_invalid" {
				t.Fatalf("error = %T %v, want storyboard_source_key_invalid", err, err)
			}
		})
	}
}

func TestStoryboardDiffTokenComputesCreateUpdateArchiveAndUnchanged(t *testing.T) {
	draft := NewStoryboardVersion()
	draft.ID = "30"
	draft.ProjectID = "1"
	draft.Revision = 4
	draft.Status = "draft"
	draft.SourceScriptRevision = 6
	draft.SourceBreakdownID = "20"
	draft.SourceAssetRevision = 8
	draft.SourceCapabilityVersion = "capability-v2"
	draft.BaselineStoryboardID = "25"
	draft.Shots = []StoryboardShot{
		{SourceKey: "shot-a", Name: "保持镜头", Enabled: true, Duration: 5, Action: "阿宁抬头", TaskMode: "reference"},
		{SourceKey: "shot-b", Name: "修改镜头", Enabled: true, Duration: 10, Action: "阿宁捡起相机", TaskMode: "reference"},
		{SourceKey: "shot-d", Name: "新增镜头", Enabled: true, Duration: 5, Action: "相机亮起", TaskMode: "reference"},
	}
	context := StoryboardDiffContext{
		CurrentScriptRevision:        6,
		CurrentBreakdownID:           "20",
		CurrentAssetRevision:         8,
		CurrentCapabilityVersion:     "capability-v2",
		CurrentConfirmedStoryboardID: "25",
		LiveShots: []MaterializedStoryboardShot{
			{ID: "40", Shot: StoryboardShot{SourceKey: "shot-a", Name: "保持镜头", Enabled: true, Duration: 5, Action: "阿宁抬头", TaskMode: "reference"}},
			{ID: "41", Shot: StoryboardShot{SourceKey: "shot-b", Name: "修改镜头", Enabled: true, Duration: 5, Action: "阿宁捡起相机", TaskMode: "reference"}},
			{ID: "42", Shot: StoryboardShot{SourceKey: "shot-c", Name: "归档镜头", Enabled: true, Duration: 5, Action: "列车驶过", TaskMode: "reference"}},
		},
	}

	diff, err := previewStoryboardDiff(draft, context)
	if err != nil {
		t.Fatalf("previewStoryboardDiff returned error: %v", err)
	}
	if diff.StoryboardID != "30" || diff.Revision != 4 || diff.DiffToken == "" {
		t.Fatalf("unexpected diff metadata: %+v", diff)
	}
	want := map[string]string{"shot-a": "unchanged", "shot-b": "update", "shot-c": "archive", "shot-d": "create"}
	if len(diff.Items) != len(want) {
		t.Fatalf("unexpected diff items: %+v", diff.Items)
	}
	for _, item := range diff.Items {
		if want[item.SourceKey] != item.Operation {
			t.Fatalf("source %s operation = %q, want %q; items=%+v", item.SourceKey, item.Operation, want[item.SourceKey], diff.Items)
		}
		if item.Operation != "create" && item.ShotID == "" {
			t.Fatalf("existing operation must include live shot ID: %+v", item)
		}
	}
}

func TestStoryboardDiffTokenChangesWithDependenciesAndOperations(t *testing.T) {
	draft := NewStoryboardVersion()
	draft.ID = "30"
	draft.ProjectID = "1"
	draft.Revision = 2
	draft.Status = "draft"
	draft.SourceScriptRevision = 4
	draft.SourceBreakdownID = "20"
	draft.SourceAssetRevision = 7
	draft.SourceCapabilityVersion = "capability-v1"
	draft.BaselineStoryboardID = "25"
	draft.Shots = []StoryboardShot{{SourceKey: "shot-a", Name: "开场", Enabled: true, Duration: 5, Action: "阿宁走进车站", TaskMode: "reference"}}
	context := StoryboardDiffContext{
		CurrentScriptRevision: 4, CurrentBreakdownID: "20", CurrentAssetRevision: 7,
		CurrentCapabilityVersion: "capability-v1", CurrentConfirmedStoryboardID: "25",
	}
	base, err := previewStoryboardDiff(draft, context)
	if err != nil {
		t.Fatal(err)
	}

	tests := []struct {
		name   string
		mutate func(*StoryboardVersion, *StoryboardDiffContext)
	}{
		{"draft revision", func(value *StoryboardVersion, _ *StoryboardDiffContext) { value.Revision++ }},
		{"baseline", func(value *StoryboardVersion, context *StoryboardDiffContext) {
			value.BaselineStoryboardID = "26"
			context.CurrentConfirmedStoryboardID = "26"
		}},
		{"script", func(value *StoryboardVersion, context *StoryboardDiffContext) {
			value.SourceScriptRevision++
			context.CurrentScriptRevision++
		}},
		{"breakdown", func(value *StoryboardVersion, context *StoryboardDiffContext) {
			value.SourceBreakdownID = "21"
			context.CurrentBreakdownID = "21"
		}},
		{"assets", func(value *StoryboardVersion, context *StoryboardDiffContext) {
			value.SourceAssetRevision++
			context.CurrentAssetRevision++
		}},
		{"capability", func(value *StoryboardVersion, context *StoryboardDiffContext) {
			value.SourceCapabilityVersion = "capability-v2"
			context.CurrentCapabilityVersion = "capability-v2"
		}},
		{"operation", func(value *StoryboardVersion, _ *StoryboardDiffContext) { value.Shots[0].Duration = 10 }},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			changedDraft := draft
			changedDraft.Shots = append([]StoryboardShot{}, draft.Shots...)
			changedContext := context
			test.mutate(&changedDraft, &changedContext)
			changed, err := previewStoryboardDiff(changedDraft, changedContext)
			if err != nil {
				t.Fatal(err)
			}
			if changed.DiffToken == base.DiffToken {
				t.Fatalf("mutation %q did not change diff token", test.name)
			}
		})
	}
}

func TestConfirmStoryboardRejectsStaleDependencies(t *testing.T) {
	draft, context := confirmationStoryboardFixture()
	diff, err := previewStoryboardDiff(draft, context)
	if err != nil {
		t.Fatal(err)
	}

	tests := []struct {
		name   string
		mutate func(*StoryboardVersion, *StoryboardDiffContext, *ConfirmStoryboardInput)
		code   string
	}{
		{"script", func(_ *StoryboardVersion, value *StoryboardDiffContext, _ *ConfirmStoryboardInput) {
			value.CurrentScriptRevision++
		}, "workflow_dependency_conflict"},
		{"breakdown", func(_ *StoryboardVersion, value *StoryboardDiffContext, _ *ConfirmStoryboardInput) {
			value.CurrentBreakdownID = "21"
		}, "workflow_dependency_conflict"},
		{"asset revision", func(_ *StoryboardVersion, value *StoryboardDiffContext, _ *ConfirmStoryboardInput) {
			value.CurrentAssetRevision++
		}, "workflow_dependency_conflict"},
		{"capability", func(_ *StoryboardVersion, value *StoryboardDiffContext, _ *ConfirmStoryboardInput) {
			value.CurrentCapabilityVersion = "capability-v2"
		}, "workflow_dependency_conflict"},
		{"baseline", func(_ *StoryboardVersion, value *StoryboardDiffContext, _ *ConfirmStoryboardInput) {
			value.CurrentConfirmedStoryboardID = "26"
		}, "workflow_dependency_conflict"},
		{"draft revision", func(value *StoryboardVersion, _ *StoryboardDiffContext, _ *ConfirmStoryboardInput) { value.Revision++ }, "workflow_diff_conflict"},
		{"input revision", func(_ *StoryboardVersion, _ *StoryboardDiffContext, input *ConfirmStoryboardInput) {
			input.ExpectedRevision--
		}, "workflow_revision_conflict"},
		{"token", func(_ *StoryboardVersion, _ *StoryboardDiffContext, input *ConfirmStoryboardInput) {
			input.DiffToken = "old-token"
		}, "workflow_diff_conflict"},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			changedDraft := draft
			changedDraft.Shots = append([]StoryboardShot{}, draft.Shots...)
			changedContext := context
			input := ConfirmStoryboardInput{ExpectedRevision: draft.Revision, DiffToken: diff.DiffToken}
			test.mutate(&changedDraft, &changedContext, &input)
			_, err := buildStoryboardMaterializationPlan(changedDraft, changedContext, input)
			assertWorkflowConflictCode(t, err, test.code)
		})
	}
}

func TestConfirmStoryboardMaterializesDiffWithoutDeletingGenerations(t *testing.T) {
	draft, context := confirmationStoryboardFixture()
	diff, err := previewStoryboardDiff(draft, context)
	if err != nil {
		t.Fatal(err)
	}
	plan, err := buildStoryboardMaterializationPlan(draft, context, ConfirmStoryboardInput{ExpectedRevision: draft.Revision, DiffToken: diff.DiffToken})
	if err != nil {
		t.Fatalf("buildStoryboardMaterializationPlan returned error: %v", err)
	}
	if len(plan.Creates) != 1 || plan.Creates[0].SourceKey != "shot-c" {
		t.Fatalf("unexpected creates: %+v", plan.Creates)
	}
	if len(plan.Updates) != 1 || plan.Updates[0].ID != "41" || plan.Updates[0].Shot.SourceKey != "shot-b" {
		t.Fatalf("unexpected updates: %+v", plan.Updates)
	}
	if plan.Updates[0].SelectedGenerationID != "501" || plan.Updates[0].SelectedGenerationAckHash != "old-ack" {
		t.Fatalf("updated shot must preserve old selection for explicit stale acknowledgement, got %+v", plan.Updates[0])
	}
	if len(plan.Unchanged) != 1 || plan.Unchanged[0].ID != "40" || plan.Unchanged[0].SelectedGenerationID != "500" {
		t.Fatalf("unchanged shot and user selection must stay current, got %+v", plan.Unchanged)
	}
	if len(plan.Archives) != 1 || plan.Archives[0].ID != "42" || plan.Archives[0].GenerationCount != 3 {
		t.Fatalf("old shot must archive with generation history untouched, got %+v", plan.Archives)
	}
	if len(plan.Creates[0].References) != 2 || plan.Creates[0].References[0].Role != "edit_target" || plan.Creates[0].References[1].Role != "reference_image" {
		t.Fatalf("reference intentions must retain ordered roles, got %+v", plan.Creates[0].References)
	}
	if plan.Creates[0].VideoModel != "video-ds-2.0" || plan.Creates[0].AspectRatio != "9:16" || plan.Creates[0].VideoResolution != "" {
		t.Fatalf("new shot must receive validated project defaults, got %+v", plan.Creates[0])
	}
	if plan.Creates[0].SceneID != "12" || len(plan.Creates[0].CharacterIDs) != 1 || plan.Creates[0].CharacterIDs[0] != "11" {
		t.Fatalf("stable asset keys must resolve to live IDs, got %+v", plan.Creates[0])
	}
}

func TestConfirmStoryboardMaterializationRejectsUnresolvedAssetKeys(t *testing.T) {
	draft, context := confirmationStoryboardFixture()
	draft.Shots[0].SceneKey = "scene-missing"
	diff, err := previewStoryboardDiff(draft, context)
	if err != nil {
		t.Fatal(err)
	}
	_, err = buildStoryboardMaterializationPlan(draft, context, ConfirmStoryboardInput{ExpectedRevision: draft.Revision, DiffToken: diff.DiffToken})
	var validation *WorkflowValidationError
	if !errors.As(err, &validation) || validation.Code != "storyboard_asset_unresolved" || validation.Field != "shots[0].sceneKey" {
		t.Fatalf("error = %T %v, want exact unresolved scene field", err, err)
	}
}

func confirmationStoryboardFixture() (StoryboardVersion, StoryboardDiffContext) {
	draft := NewStoryboardVersion()
	draft.ID = "30"
	draft.ProjectID = "1"
	draft.Revision = 4
	draft.Status = "draft"
	draft.SourceScriptRevision = 6
	draft.SourceBreakdownID = "20"
	draft.SourceAssetRevision = 8
	draft.SourceCapabilityVersion = "capability-v1"
	draft.BaselineStoryboardID = "25"
	draft.Shots = []StoryboardShot{
		{SourceKey: "shot-a", Name: "保持镜头", Enabled: true, Duration: 5, SceneKey: "scene-station", CharacterKeys: []string{"character-aning"}, Action: "阿宁抬头", TaskMode: "reference"},
		{SourceKey: "shot-b", Name: "修改镜头", Enabled: true, Duration: 10, SceneKey: "scene-station", CharacterKeys: []string{"character-aning"}, Action: "阿宁捡起相机", TaskMode: "reference"},
		{SourceKey: "shot-c", Name: "新增编辑镜头", Enabled: true, Duration: 5, SceneKey: "scene-station", CharacterKeys: []string{"character-aning"}, AssetKeys: []string{"prop-camera"}, Action: "替换画面中的相机", TaskMode: "edit", References: []StoryboardReferenceIntent{{AssetKey: "video-edit", Role: "edit_target", SortOrder: 1}, {AssetKey: "prop-camera", Role: "reference_image", SortOrder: 2}}},
	}
	context := StoryboardDiffContext{
		CurrentScriptRevision: 6, CurrentBreakdownID: "20", CurrentAssetRevision: 8,
		CurrentCapabilityVersion: "capability-v1", CurrentConfirmedStoryboardID: "25",
		LiveShots: []MaterializedStoryboardShot{
			{ID: "40", Shot: StoryboardShot{SourceKey: "shot-a", Name: "保持镜头", Enabled: true, Duration: 5, SceneKey: "scene-station", CharacterKeys: []string{"character-aning"}, Action: "阿宁抬头", TaskMode: "reference"}, SelectedGenerationID: "500", GenerationCount: 2},
			{ID: "41", Shot: StoryboardShot{SourceKey: "shot-b", Name: "修改镜头", Enabled: true, Duration: 5, SceneKey: "scene-station", CharacterKeys: []string{"character-aning"}, Action: "阿宁捡起相机", TaskMode: "reference"}, SelectedGenerationID: "501", SelectedGenerationAckHash: "old-ack", GenerationCount: 1},
			{ID: "42", Shot: StoryboardShot{SourceKey: "shot-old", Name: "旧镜头", Enabled: true, Duration: 5, SceneKey: "scene-station", CharacterKeys: []string{"character-aning"}, Action: "列车驶过", TaskMode: "reference"}, SelectedGenerationID: "502", GenerationCount: 3},
		},
		Assets: []StoryboardResolvedAsset{
			{Key: "character-aning", Kind: "character", ID: "11", ImageURL: "https://cdn.example.com/character.png"},
			{Key: "scene-station", Kind: "scene", ID: "12", ImageURL: "https://cdn.example.com/scene.png"},
			{Key: "prop-camera", Kind: "prop", ID: "13", ImageURL: "https://cdn.example.com/camera.png"},
			{Key: "video-edit", Kind: "video", ID: "14", ObjectURL: "https://cdn.example.com/edit.mp4"},
		},
		Defaults: StoryboardProjectDefaults{VideoModel: "video-ds-2.0", AspectRatio: "9:16", VideoResolution: "", AudioMode: ""},
		Capabilities: video.Capabilities{
			Model: "video-ds-2.0", CapabilityVersion: "capability-v1", SupportedDurations: []int{5, 10, 15}, AspectRatios: []string{"9:16"},
			TaskModes: []string{"reference", "edit"}, ReferenceRoles: []string{"reference_image", "edit_target"},
		},
	}
	return draft, context
}

type recordingProjectStoryboardGenerator struct {
	result llm.ProjectStoryboardResult
	raw    string
	err    error
	inputs []llm.ProjectStoryboardInput
}

func (generator *recordingProjectStoryboardGenerator) DesignVideoProjectStoryboard(_ context.Context, input llm.ProjectStoryboardInput) (llm.ProjectStoryboardResult, string, error) {
	generator.inputs = append(generator.inputs, input)
	return generator.result, generator.raw, generator.err
}

type recordingStoryboardDraftRepository struct {
	snapshot StoryboardDesignSnapshot
	writes   []StoryboardDraftWrite
	versions []StoryboardVersion
}

func (repository *recordingStoryboardDraftRepository) LoadStoryboardDesignSnapshot(context.Context, string) (StoryboardDesignSnapshot, error) {
	return repository.snapshot, nil
}

func (repository *recordingStoryboardDraftRepository) SaveStoryboardDraft(_ context.Context, write StoryboardDraftWrite) (StoryboardVersion, error) {
	repository.writes = append(repository.writes, write)
	version := NewStoryboardVersion()
	version.ID = stringID(int64(len(repository.versions) + 1))
	version.ProjectID = write.ProjectID
	version.Version = len(repository.versions) + 1
	version.Revision = 1
	version.Status = write.Status
	version.SourceScriptRevision = write.SourceScriptRevision
	version.SourceBreakdownID = write.SourceBreakdownID
	version.SourceAssetRevision = write.SourceAssetRevision
	version.SourceCapabilityVersion = write.SourceCapabilityVersion
	version.BaselineStoryboardID = write.BaselineStoryboardID
	version.Shots = write.Shots
	version.RawResult = write.RawResult
	version.ErrorMessage = write.ErrorMessage
	repository.versions = append(repository.versions, version)
	return version, nil
}
