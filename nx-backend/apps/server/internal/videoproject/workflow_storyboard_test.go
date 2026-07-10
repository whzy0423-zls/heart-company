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
