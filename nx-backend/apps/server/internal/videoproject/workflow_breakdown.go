package videoproject

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"strings"
	"time"

	"nine-xing/nx-backend/apps/server/internal/llm"
)

type ProjectScriptSnapshot struct {
	ProjectID string
	Revision  int
	Content   string
}

type BreakdownDraftWrite struct {
	ProjectID            string
	Status               string
	SourceScriptRevision int
	ScriptSnapshot       string
	Characters           []BreakdownItem
	Scenes               []BreakdownItem
	Props                []BreakdownItem
	Outfits              []BreakdownItem
	Styles               []BreakdownItem
	StoryBeats           []StoryBeat
	RawResult            string
	ErrorMessage         string
}

type projectBreakdownGenerator interface {
	BreakdownVideoProjectScript(ctx context.Context, script string) (llm.ProjectBreakdownResult, string, error)
}

type breakdownDraftRepository interface {
	LoadConfirmedProjectScript(ctx context.Context, projectID string) (ProjectScriptSnapshot, error)
	SaveBreakdownDraft(ctx context.Context, write BreakdownDraftWrite) (BreakdownVersion, error)
}

type BreakdownWorkflowService struct {
	repository breakdownDraftRepository
	generator  projectBreakdownGenerator
}

func NewBreakdownWorkflowService(repository breakdownDraftRepository, generator projectBreakdownGenerator) *BreakdownWorkflowService {
	return &BreakdownWorkflowService{repository: repository, generator: generator}
}

func (service *BreakdownWorkflowService) CreateBreakdownDraft(ctx context.Context, projectID string) (BreakdownVersion, error) {
	if service == nil || service.repository == nil || service.generator == nil {
		return BreakdownVersion{}, fmt.Errorf("剧本拆解服务尚未配置")
	}
	snapshot, err := service.repository.LoadConfirmedProjectScript(ctx, projectID)
	if err != nil {
		return BreakdownVersion{}, err
	}
	result, rawResult, generationErr := service.generator.BreakdownVideoProjectScript(ctx, snapshot.Content)
	write := BreakdownDraftWrite{
		ProjectID:            snapshot.ProjectID,
		Status:               "draft",
		SourceScriptRevision: snapshot.Revision,
		ScriptSnapshot:       snapshot.Content,
		RawResult:            rawResult,
	}
	if generationErr != nil {
		write.Status = "failed"
		write.ErrorMessage = generationErr.Error()
	} else {
		write.Characters = breakdownItemsFromLLM(result.Characters)
		write.Scenes = breakdownItemsFromLLM(result.Scenes)
		write.Props = breakdownItemsFromLLM(result.Props)
		write.Outfits = breakdownItemsFromLLM(result.Outfits)
		write.Styles = breakdownItemsFromLLM(result.Styles)
		write.StoryBeats = storyBeatsFromLLM(result.StoryBeats)
	}
	draft, saveErr := service.repository.SaveBreakdownDraft(ctx, write)
	if saveErr != nil {
		return BreakdownVersion{}, saveErr
	}
	if generationErr != nil {
		return draft, generationErr
	}
	return draft, nil
}

func breakdownItemsFromLLM(items []llm.BreakdownItem) []BreakdownItem {
	result := make([]BreakdownItem, 0, len(items))
	for _, item := range items {
		result = append(result, BreakdownItem{
			Key:          item.Key,
			Name:         item.Name,
			Description:  item.Description,
			VisualPrompt: item.VisualPrompt,
			UsageNote:    item.UsageNote,
			Required:     item.Required,
			Decision:     item.Decision,
		})
	}
	return result
}

func storyBeatsFromLLM(items []llm.ProjectStoryBeat) []StoryBeat {
	result := make([]StoryBeat, 0, len(items))
	for _, item := range items {
		result = append(result, StoryBeat{
			Key:         item.Key,
			Title:       item.Title,
			Description: item.Description,
			SceneKeys:   append([]string{}, item.SceneKeys...),
			AssetKeys:   append([]string{}, item.AssetKeys...),
		})
	}
	return result
}

func (store *Store) LoadConfirmedProjectScript(ctx context.Context, projectID string) (ProjectScriptSnapshot, error) {
	parsedProjectID, err := parseID(projectID)
	if err != nil {
		return ProjectScriptSnapshot{}, err
	}
	var snapshot ProjectScriptSnapshot
	var confirmedRevision int
	if err := store.db.QueryRowContext(ctx,
		`SELECT id::text, script_revision, confirmed_script_revision, script_content
		   FROM video_projects
		  WHERE id=$1`,
		parsedProjectID,
	).Scan(&snapshot.ProjectID, &snapshot.Revision, &confirmedRevision, &snapshot.Content); err != nil {
		if err == sql.ErrNoRows {
			return ProjectScriptSnapshot{}, fmt.Errorf("项目不存在")
		}
		return ProjectScriptSnapshot{}, err
	}
	snapshot.Content = strings.TrimSpace(snapshot.Content)
	if snapshot.Content == "" {
		return ProjectScriptSnapshot{}, fmt.Errorf("请先填写剧本内容")
	}
	if snapshot.Revision <= 0 || confirmedRevision != snapshot.Revision {
		return ProjectScriptSnapshot{}, fmt.Errorf("请先确认当前剧本，再进行 AI 拆解")
	}
	return snapshot, nil
}

func (store *Store) SaveBreakdownDraft(ctx context.Context, write BreakdownDraftWrite) (BreakdownVersion, error) {
	projectID, err := parseID(write.ProjectID)
	if err != nil {
		return BreakdownVersion{}, err
	}
	if write.Status != "draft" && write.Status != "failed" {
		return BreakdownVersion{}, fmt.Errorf("无效的拆解草稿状态")
	}
	transaction, err := store.db.BeginTx(ctx, nil)
	if err != nil {
		return BreakdownVersion{}, err
	}
	defer transaction.Rollback()

	var lockedProjectID int64
	if err := transaction.QueryRowContext(ctx,
		`SELECT id FROM video_projects WHERE id=$1 FOR UPDATE`,
		projectID,
	).Scan(&lockedProjectID); err != nil {
		if err == sql.ErrNoRows {
			return BreakdownVersion{}, fmt.Errorf("项目不存在")
		}
		return BreakdownVersion{}, err
	}
	var nextVersion int
	if err := transaction.QueryRowContext(ctx,
		`SELECT COALESCE(MAX(version),0)+1 FROM video_project_breakdowns WHERE project_id=$1`,
		projectID,
	).Scan(&nextVersion); err != nil {
		return BreakdownVersion{}, err
	}

	characters, _ := json.Marshal(nonNilBreakdownItems(write.Characters))
	scenes, _ := json.Marshal(nonNilBreakdownItems(write.Scenes))
	props, _ := json.Marshal(nonNilBreakdownItems(write.Props))
	outfits, _ := json.Marshal(nonNilBreakdownItems(write.Outfits))
	styles, _ := json.Marshal(nonNilBreakdownItems(write.Styles))
	storyBeats, _ := json.Marshal(nonNilStoryBeats(write.StoryBeats))

	version := NewBreakdownVersion()
	var createTime, updateTime time.Time
	var rawCharacters, rawScenes, rawProps, rawOutfits, rawStyles, rawStoryBeats []byte
	if err := transaction.QueryRowContext(ctx,
		`INSERT INTO video_project_breakdowns (
		     project_id, version, revision, status, source_script_revision, script_snapshot,
		     characters, scenes, props, outfits, styles, story_beats, raw_result, error_message
		 ) VALUES ($1,$2,1,$3,$4,$5,$6::jsonb,$7::jsonb,$8::jsonb,$9::jsonb,$10::jsonb,$11::jsonb,$12,$13)
		 RETURNING id::text, project_id::text, version, revision, status, source_script_revision,
		           script_snapshot, characters, scenes, props, outfits, styles, story_beats,
		           raw_result, error_message, create_time, update_time`,
		projectID, nextVersion, write.Status, write.SourceScriptRevision, write.ScriptSnapshot,
		string(characters), string(scenes), string(props), string(outfits), string(styles), string(storyBeats),
		write.RawResult, write.ErrorMessage,
	).Scan(
		&version.ID, &version.ProjectID, &version.Version, &version.Revision, &version.Status,
		&version.SourceScriptRevision, &version.ScriptSnapshot,
		&rawCharacters, &rawScenes, &rawProps, &rawOutfits, &rawStyles, &rawStoryBeats,
		&version.RawResult, &version.ErrorMessage, &createTime, &updateTime,
	); err != nil {
		return BreakdownVersion{}, err
	}
	if err := decodeBreakdownVersionJSON(&version, rawCharacters, rawScenes, rawProps, rawOutfits, rawStyles, rawStoryBeats); err != nil {
		return BreakdownVersion{}, err
	}
	version.CreateTime = formatTime(createTime)
	version.UpdateTime = formatTime(updateTime)
	if err := transaction.Commit(); err != nil {
		return BreakdownVersion{}, err
	}
	return version, nil
}

func decodeBreakdownVersionJSON(version *BreakdownVersion, characters, scenes, props, outfits, styles, storyBeats []byte) error {
	for _, item := range []struct {
		raw    []byte
		target any
	}{
		{characters, &version.Characters},
		{scenes, &version.Scenes},
		{props, &version.Props},
		{outfits, &version.Outfits},
		{styles, &version.Styles},
		{storyBeats, &version.StoryBeats},
	} {
		if err := json.Unmarshal(item.raw, item.target); err != nil {
			return err
		}
	}
	return nil
}

func nonNilBreakdownItems(items []BreakdownItem) []BreakdownItem {
	if items == nil {
		return []BreakdownItem{}
	}
	return items
}

func nonNilStoryBeats(items []StoryBeat) []StoryBeat {
	if items == nil {
		return []StoryBeat{}
	}
	return items
}
