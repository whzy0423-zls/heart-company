package videoproject

import (
	"context"
	"errors"
	"strings"
	"testing"

	"nine-xing/nx-backend/apps/server/internal/llm"
)

func TestCreateBreakdownDraftPersistsNewVersionAndScriptSnapshot(t *testing.T) {
	repository := &recordingBreakdownDraftRepository{
		snapshot: ProjectScriptSnapshot{
			ProjectID: "12",
			Revision:  3,
			Content:   "第一场：阿宁在雨夜车站捡到旧相机。",
		},
	}
	generator := &recordingProjectBreakdownGenerator{
		result: llm.ProjectBreakdownResult{
			Characters: []llm.BreakdownItem{{Key: "character-a", Name: "阿宁", Decision: "pending"}},
			Scenes:     []llm.BreakdownItem{{Key: "scene-a", Name: "雨夜车站", Decision: "pending"}},
			Props:      []llm.BreakdownItem{},
			Outfits:    []llm.BreakdownItem{},
			Styles:     []llm.BreakdownItem{},
			StoryBeats: []llm.ProjectStoryBeat{},
		},
		raw: `{"characters":[{"key":"character-a","name":"阿宁"}]}`,
	}
	service := NewBreakdownWorkflowService(repository, generator)

	first, err := service.CreateBreakdownDraft(context.Background(), "12")
	if err != nil {
		t.Fatalf("CreateBreakdownDraft returned error: %v", err)
	}
	second, err := service.CreateBreakdownDraft(context.Background(), "12")
	if err != nil {
		t.Fatalf("second CreateBreakdownDraft returned error: %v", err)
	}

	if generator.scripts[0] != repository.snapshot.Content {
		t.Fatalf("expected confirmed script snapshot to be sent to model, got %q", generator.scripts[0])
	}
	if first.Version != 1 || second.Version != 2 || first.ID == second.ID {
		t.Fatalf("each request must create a new version, first=%+v second=%+v", first, second)
	}
	if first.Revision != 1 || first.Status != "draft" || first.SourceScriptRevision != 3 || first.ScriptSnapshot != repository.snapshot.Content {
		t.Fatalf("unexpected persisted draft metadata: %+v", first)
	}
	if len(first.Characters) != 1 || first.Characters[0].Key != "character-a" || first.RawResult != generator.raw {
		t.Fatalf("expected cleaned result and raw output to be persisted, got %+v", first)
	}
	if len(repository.writes) != 2 || repository.writes[0].Status != "draft" || repository.writes[0].ErrorMessage != "" {
		t.Fatalf("unexpected draft writes: %+v", repository.writes)
	}
}

func TestCreateBreakdownDraftPersistsFailedVersionAndRawResult(t *testing.T) {
	repository := &recordingBreakdownDraftRepository{
		snapshot: ProjectScriptSnapshot{ProjectID: "9", Revision: 2, Content: "阿宁走进车站。"},
	}
	generator := &recordingProjectBreakdownGenerator{
		raw: "模型这次没有返回 JSON",
		err: errors.New("剧本拆解模型未返回有效 JSON，请重新生成"),
	}
	service := NewBreakdownWorkflowService(repository, generator)

	draft, err := service.CreateBreakdownDraft(context.Background(), "9")
	if err == nil || !strings.Contains(err.Error(), "有效 JSON") {
		t.Fatalf("expected model error to reach caller, got %v", err)
	}
	if draft.Status != "failed" || draft.Version != 1 || draft.Revision != 1 {
		t.Fatalf("expected failed version to be returned, got %+v", draft)
	}
	if draft.RawResult != generator.raw || draft.ErrorMessage != generator.err.Error() {
		t.Fatalf("expected raw failure details to be persisted, got %+v", draft)
	}
	if draft.ScriptSnapshot != repository.snapshot.Content || draft.SourceScriptRevision != repository.snapshot.Revision {
		t.Fatalf("expected failed version to keep source snapshot, got %+v", draft)
	}
	if len(repository.writes) != 1 || repository.writes[0].Status != "failed" {
		t.Fatalf("expected one failed write, got %+v", repository.writes)
	}
}

type recordingProjectBreakdownGenerator struct {
	result  llm.ProjectBreakdownResult
	raw     string
	err     error
	scripts []string
}

func (generator *recordingProjectBreakdownGenerator) BreakdownVideoProjectScript(_ context.Context, script string) (llm.ProjectBreakdownResult, string, error) {
	generator.scripts = append(generator.scripts, script)
	return generator.result, generator.raw, generator.err
}

type recordingBreakdownDraftRepository struct {
	snapshot ProjectScriptSnapshot
	writes   []BreakdownDraftWrite
	versions []BreakdownVersion
}

func (repository *recordingBreakdownDraftRepository) LoadConfirmedProjectScript(context.Context, string) (ProjectScriptSnapshot, error) {
	return repository.snapshot, nil
}

func (repository *recordingBreakdownDraftRepository) SaveBreakdownDraft(_ context.Context, write BreakdownDraftWrite) (BreakdownVersion, error) {
	repository.writes = append(repository.writes, write)
	version := NewBreakdownVersion()
	version.ID = stringID(int64(len(repository.versions) + 1))
	version.ProjectID = write.ProjectID
	version.Version = len(repository.versions) + 1
	version.Revision = 1
	version.Status = write.Status
	version.SourceScriptRevision = write.SourceScriptRevision
	version.ScriptSnapshot = write.ScriptSnapshot
	version.Characters = write.Characters
	version.Scenes = write.Scenes
	version.Props = write.Props
	version.Outfits = write.Outfits
	version.Styles = write.Styles
	version.StoryBeats = write.StoryBeats
	version.RawResult = write.RawResult
	version.ErrorMessage = write.ErrorMessage
	repository.versions = append(repository.versions, version)
	return version, nil
}
