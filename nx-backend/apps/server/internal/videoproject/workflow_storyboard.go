package videoproject

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"strings"
	"time"

	"nine-xing/nx-backend/apps/server/internal/llm"
	"nine-xing/nx-backend/apps/server/internal/video"
)

type StoryboardDesignSnapshot struct {
	ProjectID            string
	Script               string
	ScriptRevision       int
	BreakdownID          string
	AssetRevision        int
	BaselineStoryboardID string
	Assets               []llm.ProjectAssetSummary
}

type StoryboardDraftInput struct {
	ProjectID    string
	AspectRatio  string
	Capabilities video.Capabilities
}

type StoryboardDraftWrite struct {
	ProjectID               string
	Status                  string
	SourceScriptRevision    int
	SourceBreakdownID       string
	SourceAssetRevision     int
	SourceCapabilityVersion string
	BaselineStoryboardID    string
	Shots                   []StoryboardShot
	RawResult               string
	ErrorMessage            string
}

type projectStoryboardGenerator interface {
	DesignVideoProjectStoryboard(ctx context.Context, input llm.ProjectStoryboardInput) (llm.ProjectStoryboardResult, string, error)
}

type storyboardDraftRepository interface {
	LoadStoryboardDesignSnapshot(ctx context.Context, projectID string) (StoryboardDesignSnapshot, error)
	SaveStoryboardDraft(ctx context.Context, write StoryboardDraftWrite) (StoryboardVersion, error)
}

type StoryboardWorkflowService struct {
	repository storyboardDraftRepository
	generator  projectStoryboardGenerator
}

func NewStoryboardWorkflowService(repository storyboardDraftRepository, generator projectStoryboardGenerator) *StoryboardWorkflowService {
	return &StoryboardWorkflowService{repository: repository, generator: generator}
}

func (service *StoryboardWorkflowService) CreateStoryboardDraft(ctx context.Context, input StoryboardDraftInput) (StoryboardVersion, error) {
	if service == nil || service.repository == nil || service.generator == nil {
		return StoryboardVersion{}, fmt.Errorf("AI 分镜服务尚未配置")
	}
	snapshot, err := service.repository.LoadStoryboardDesignSnapshot(ctx, input.ProjectID)
	if err != nil {
		return StoryboardVersion{}, err
	}
	modelInput := llm.ProjectStoryboardInput{
		Script:            snapshot.Script,
		ScriptRevision:    snapshot.ScriptRevision,
		BreakdownID:       snapshot.BreakdownID,
		AssetRevision:     snapshot.AssetRevision,
		CapabilityVersion: strings.TrimSpace(input.Capabilities.CapabilityVersion),
		Model:             strings.TrimSpace(input.Capabilities.Model),
		AspectRatio:       strings.TrimSpace(input.AspectRatio),
		AllowedDurations:  append([]int{}, input.Capabilities.SupportedDurations...),
		TaskModes:         append([]string{}, input.Capabilities.TaskModes...),
		ReferenceRoles:    append([]string{}, input.Capabilities.ReferenceRoles...),
		Assets:            append([]llm.ProjectAssetSummary{}, snapshot.Assets...),
	}
	result, rawResult, generationErr := service.generator.DesignVideoProjectStoryboard(ctx, modelInput)
	write := StoryboardDraftWrite{
		ProjectID:               snapshot.ProjectID,
		Status:                  "draft",
		SourceScriptRevision:    snapshot.ScriptRevision,
		SourceBreakdownID:       snapshot.BreakdownID,
		SourceAssetRevision:     snapshot.AssetRevision,
		SourceCapabilityVersion: modelInput.CapabilityVersion,
		BaselineStoryboardID:    snapshot.BaselineStoryboardID,
		RawResult:               rawResult,
	}
	if generationErr != nil {
		write.Status = "failed"
		write.ErrorMessage = generationErr.Error()
	} else {
		write.Shots = storyboardShotsFromWorkflowLLM(result.Shots)
	}
	draft, saveErr := service.repository.SaveStoryboardDraft(ctx, write)
	if saveErr != nil {
		return StoryboardVersion{}, saveErr
	}
	if generationErr != nil {
		return draft, generationErr
	}
	return draft, nil
}

func storyboardShotsFromWorkflowLLM(shots []llm.ProjectStoryboardShot) []StoryboardShot {
	result := make([]StoryboardShot, 0, len(shots))
	for _, shot := range shots {
		references := make([]StoryboardReferenceIntent, 0, len(shot.References))
		for _, reference := range shot.References {
			references = append(references, StoryboardReferenceIntent{
				AssetKey:  reference.AssetKey,
				Role:      reference.Role,
				SortOrder: reference.SortOrder,
				UsageNote: reference.UsageNote,
			})
		}
		result = append(result, StoryboardShot{
			SourceKey:     shot.SourceKey,
			Name:          shot.Name,
			Enabled:       shot.Enabled,
			Duration:      shot.Duration,
			SceneKey:      shot.SceneKey,
			CharacterKeys: append([]string{}, shot.CharacterKeys...),
			AssetKeys:     append([]string{}, shot.AssetKeys...),
			Action:        shot.Action,
			Camera:        shot.Camera,
			Composition:   shot.Composition,
			Lighting:      shot.Lighting,
			Audio:         shot.Audio,
			Dialogue:      shot.Dialogue,
			TaskMode:      shot.TaskMode,
			References:    references,
		})
	}
	return result
}

func (store *Store) LoadStoryboardDesignSnapshot(ctx context.Context, projectID string) (StoryboardDesignSnapshot, error) {
	parsedProjectID, err := parseID(projectID)
	if err != nil {
		return StoryboardDesignSnapshot{}, err
	}
	var snapshot StoryboardDesignSnapshot
	var currentScriptRevision, confirmedScriptRevision int
	if err := store.db.QueryRowContext(ctx,
		`SELECT p.id::text, p.script_content, p.script_revision, p.confirmed_script_revision,
		        p.asset_revision,
		        COALESCE((
		          SELECT b.id::text
		            FROM video_project_breakdowns b
		           WHERE b.project_id=p.id AND b.status='confirmed'
		           LIMIT 1
		        ),''),
		        COALESCE((
		          SELECT storyboard.id::text
		            FROM video_project_storyboard_versions storyboard
		           WHERE storyboard.project_id=p.id AND storyboard.status='confirmed'
		           LIMIT 1
		        ),'')
		   FROM video_projects p
		  WHERE p.id=$1`,
		parsedProjectID,
	).Scan(
		&snapshot.ProjectID, &snapshot.Script, &currentScriptRevision, &confirmedScriptRevision,
		&snapshot.AssetRevision, &snapshot.BreakdownID, &snapshot.BaselineStoryboardID,
	); err != nil {
		if err == sql.ErrNoRows {
			return StoryboardDesignSnapshot{}, fmt.Errorf("项目不存在")
		}
		return StoryboardDesignSnapshot{}, err
	}
	snapshot.Script = strings.TrimSpace(snapshot.Script)
	if snapshot.Script == "" || currentScriptRevision <= 0 || confirmedScriptRevision != currentScriptRevision {
		return StoryboardDesignSnapshot{}, fmt.Errorf("请先确认当前剧本")
	}
	if snapshot.BreakdownID == "" {
		return StoryboardDesignSnapshot{}, fmt.Errorf("请先确认剧本拆解结果")
	}
	snapshot.ScriptRevision = confirmedScriptRevision

	breakdownID, err := parseID(snapshot.BreakdownID)
	if err != nil {
		return StoryboardDesignSnapshot{}, fmt.Errorf("已确认的剧本拆解版本无效")
	}
	rows, err := store.db.QueryContext(ctx,
		`SELECT asset_key, asset_type, name, description, visual_prompt, reference_image_url, required
		   FROM (
		     SELECT c.breakdown_item_key AS asset_key, 'character'::text AS asset_type,
		            c.name, c.description, c.visual_prompt, c.reference_image_url, c.required, c.id, 1 AS category_order
		       FROM video_project_characters c
		      WHERE c.project_id=$1 AND c.source_breakdown_id=$2 AND c.status<>'detached'
		     UNION ALL
		     SELECT scene.breakdown_item_key, 'scene'::text,
		            scene.name, scene.description, scene.visual_prompt, scene.reference_image_url, scene.required, scene.id, 2
		       FROM video_project_scenes scene
		      WHERE scene.project_id=$1 AND scene.source_breakdown_id=$2 AND scene.status<>'detached'
		     UNION ALL
		     SELECT asset.breakdown_item_key, asset.type,
		            asset.name, asset.description, asset.visual_prompt, asset.reference_image_url, asset.required, asset.id,
		            CASE asset.type WHEN 'prop' THEN 3 WHEN 'outfit' THEN 4 ELSE 5 END
		       FROM video_project_assets asset
		      WHERE asset.project_id=$1 AND asset.source_breakdown_id=$2 AND asset.status<>'detached'
		   ) confirmed_assets
		  WHERE asset_key<>''
		  ORDER BY category_order, id`,
		parsedProjectID, breakdownID,
	)
	if err != nil {
		return StoryboardDesignSnapshot{}, err
	}
	defer rows.Close()
	snapshot.Assets = []llm.ProjectAssetSummary{}
	for rows.Next() {
		var asset llm.ProjectAssetSummary
		if err := rows.Scan(
			&asset.Key, &asset.Type, &asset.Name, &asset.Description, &asset.VisualPrompt,
			&asset.ReferenceImageURL, &asset.Required,
		); err != nil {
			return StoryboardDesignSnapshot{}, err
		}
		snapshot.Assets = append(snapshot.Assets, asset)
	}
	if err := rows.Err(); err != nil {
		return StoryboardDesignSnapshot{}, err
	}
	if len(snapshot.Assets) == 0 {
		return StoryboardDesignSnapshot{}, fmt.Errorf("当前拆解还没有可用于分镜的已确认资产")
	}
	return snapshot, nil
}

func (store *Store) SaveStoryboardDraft(ctx context.Context, write StoryboardDraftWrite) (StoryboardVersion, error) {
	projectID, err := parseID(write.ProjectID)
	if err != nil {
		return StoryboardVersion{}, err
	}
	breakdownID, err := parseID(write.SourceBreakdownID)
	if err != nil {
		return StoryboardVersion{}, fmt.Errorf("无效的剧本拆解版本")
	}
	if write.Status != "draft" && write.Status != "failed" {
		return StoryboardVersion{}, fmt.Errorf("无效的分镜草稿状态")
	}
	var baselineStoryboardID any
	if strings.TrimSpace(write.BaselineStoryboardID) != "" {
		parsedBaselineID, err := parseID(write.BaselineStoryboardID)
		if err != nil {
			return StoryboardVersion{}, fmt.Errorf("无效的分镜对比基线")
		}
		baselineStoryboardID = parsedBaselineID
	}

	transaction, err := store.db.BeginTx(ctx, nil)
	if err != nil {
		return StoryboardVersion{}, err
	}
	defer transaction.Rollback()
	var lockedProjectID int64
	if err := transaction.QueryRowContext(ctx,
		`SELECT id FROM video_projects WHERE id=$1 FOR UPDATE`,
		projectID,
	).Scan(&lockedProjectID); err != nil {
		if err == sql.ErrNoRows {
			return StoryboardVersion{}, fmt.Errorf("项目不存在")
		}
		return StoryboardVersion{}, err
	}
	var nextVersion int
	if err := transaction.QueryRowContext(ctx,
		`SELECT COALESCE(MAX(version),0)+1 FROM video_project_storyboard_versions WHERE project_id=$1`,
		projectID,
	).Scan(&nextVersion); err != nil {
		return StoryboardVersion{}, err
	}
	shots, _ := json.Marshal(nonNilStoryboardShots(write.Shots))

	version := NewStoryboardVersion()
	var rawShots []byte
	var createTime, updateTime time.Time
	if err := transaction.QueryRowContext(ctx,
		`INSERT INTO video_project_storyboard_versions (
		     project_id, version, revision, status, source_script_revision, source_breakdown_id,
		     source_asset_revision, source_capability_version, baseline_storyboard_id,
		     shots, raw_result, error_message
		 ) VALUES ($1,$2,1,$3,$4,$5,$6,$7,$8,$9::jsonb,$10,$11)
		 RETURNING id::text, project_id::text, version, revision, status, source_script_revision,
		           source_breakdown_id::text, source_asset_revision, source_capability_version,
		           COALESCE(baseline_storyboard_id::text,''), shots, raw_result, error_message,
		           create_time, update_time`,
		projectID, nextVersion, write.Status, write.SourceScriptRevision, breakdownID,
		write.SourceAssetRevision, strings.TrimSpace(write.SourceCapabilityVersion), baselineStoryboardID,
		string(shots), write.RawResult, write.ErrorMessage,
	).Scan(
		&version.ID, &version.ProjectID, &version.Version, &version.Revision, &version.Status,
		&version.SourceScriptRevision, &version.SourceBreakdownID, &version.SourceAssetRevision,
		&version.SourceCapabilityVersion, &version.BaselineStoryboardID, &rawShots,
		&version.RawResult, &version.ErrorMessage, &createTime, &updateTime,
	); err != nil {
		return StoryboardVersion{}, err
	}
	if err := json.Unmarshal(rawShots, &version.Shots); err != nil {
		return StoryboardVersion{}, err
	}
	version.CreateTime = formatTime(createTime)
	version.UpdateTime = formatTime(updateTime)
	if err := transaction.Commit(); err != nil {
		return StoryboardVersion{}, err
	}
	return version, nil
}

func nonNilStoryboardShots(shots []StoryboardShot) []StoryboardShot {
	if shots == nil {
		return []StoryboardShot{}
	}
	return shots
}
