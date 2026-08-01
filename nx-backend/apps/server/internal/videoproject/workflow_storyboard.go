package videoproject

import (
	"context"
	"crypto/sha256"
	"database/sql"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"reflect"
	"sort"
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

type UpdateStoryboardDraftInput struct {
	ExpectedRevision int              `json:"expectedRevision"`
	Shots            []StoryboardShot `json:"shots"`
}

type ConfirmStoryboardInput struct {
	ExpectedRevision int    `json:"expectedRevision"`
	DiffToken        string `json:"diffToken"`
}

type MaterializedStoryboardShot struct {
	ID                        string
	Shot                      StoryboardShot
	SelectedGenerationID      string
	SelectedGenerationAckHash string
	GenerationCount           int
}

type StoryboardResolvedAsset struct {
	Key       string
	Kind      string
	ID        string
	ImageURL  string
	ObjectURL string
}

type StoryboardProjectDefaults struct {
	VideoModel      string
	AspectRatio     string
	VideoResolution string
	AudioMode       string
}

type MaterializedStoryboardReference struct {
	AssetKey   string
	AssetType  string
	ObjectURL  string
	Role       string
	SortOrder  int
	UsageNote  string
	SourceID   string
	SourceType string
}

type StoryboardShotMaterialization struct {
	ID   string
	Shot StoryboardShot `json:"shot"`
	StoryboardShot
	SceneID                   string
	CharacterIDs              []string
	References                []MaterializedStoryboardReference
	VideoModel                string
	AspectRatio               string
	VideoResolution           string
	AudioMode                 string
	SelectedGenerationID      string
	SelectedGenerationAckHash string
	GenerationCount           int
}

type StoryboardMaterializationPlan struct {
	Creates   []StoryboardShotMaterialization
	Updates   []StoryboardShotMaterialization
	Unchanged []StoryboardShotMaterialization
	Archives  []StoryboardShotMaterialization
}

type StoryboardDiffContext struct {
	CurrentScriptRevision        int
	CurrentBreakdownID           string
	CurrentAssetRevision         int
	CurrentCapabilityVersion     string
	CurrentConfirmedStoryboardID string
	LiveShots                    []MaterializedStoryboardShot
	Assets                       []StoryboardResolvedAsset
	Defaults                     StoryboardProjectDefaults
	Capabilities                 video.Capabilities
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

func applyStoryboardDraftUpdate(draft StoryboardVersion, input UpdateStoryboardDraftInput) (StoryboardVersion, bool, error) {
	if input.ExpectedRevision != draft.Revision {
		return StoryboardVersion{}, false, revisionConflict(draft.Revision, "分镜草稿已在其他页面更新，请刷新后重试")
	}
	if draft.Status != "draft" {
		return StoryboardVersion{}, false, &WorkflowValidationError{
			Code: "storyboard_not_editable", Field: "status", Message: "只有分镜草稿可以编辑",
			Fix: "请重新生成一份分镜草稿。", Details: map[string]any{"status": draft.Status},
		}
	}
	shots, err := normalizeStoryboardDraftShots(input.Shots)
	if err != nil {
		return StoryboardVersion{}, false, err
	}
	if reflect.DeepEqual(draft.Shots, shots) {
		return draft, false, nil
	}
	next := draft
	next.Shots = shots
	next.Revision++
	return next, true, nil
}

func normalizeStoryboardDraftShots(shots []StoryboardShot) ([]StoryboardShot, error) {
	result := make([]StoryboardShot, len(shots))
	seen := map[string]bool{}
	for index, shot := range shots {
		shot.SourceKey = strings.TrimSpace(shot.SourceKey)
		if shot.SourceKey == "" || seen[shot.SourceKey] {
			return nil, &WorkflowValidationError{
				Code: "storyboard_source_key_invalid", Field: fmt.Sprintf("shots[%d].sourceKey", index),
				Message: "每个分镜必须有唯一的稳定标识", Fix: "请重新生成分镜，或为重复镜头设置不同的 sourceKey。",
				Details: map[string]any{"sourceKey": shot.SourceKey},
			}
		}
		seen[shot.SourceKey] = true
		shot.Name = strings.TrimSpace(shot.Name)
		shot.SceneKey = strings.TrimSpace(shot.SceneKey)
		shot.CharacterKeys = cleanStoryboardStringList(shot.CharacterKeys)
		shot.AssetKeys = cleanStoryboardStringList(shot.AssetKeys)
		shot.Action = strings.TrimSpace(shot.Action)
		shot.Camera = strings.TrimSpace(shot.Camera)
		shot.Composition = strings.TrimSpace(shot.Composition)
		shot.Lighting = strings.TrimSpace(shot.Lighting)
		shot.Audio = strings.TrimSpace(shot.Audio)
		shot.Dialogue = strings.TrimSpace(shot.Dialogue)
		shot.TaskMode = strings.ToLower(strings.TrimSpace(shot.TaskMode))
		if shot.TaskMode == "" {
			shot.TaskMode = "reference"
		}
		shot.References = normalizeStoryboardReferenceIntents(shot.References)
		result[index] = shot
	}
	return result, nil
}

func previewStoryboardDiff(draft StoryboardVersion, diffContext StoryboardDiffContext) (StoryboardDiff, error) {
	if draft.Status != "draft" {
		return StoryboardDiff{}, &WorkflowValidationError{
			Code: "storyboard_not_editable", Field: "status", Message: "只有分镜草稿可以确认",
			Fix: "请重新生成一份分镜草稿。", Details: map[string]any{"status": draft.Status},
		}
	}
	shots, err := normalizeStoryboardDraftShots(draft.Shots)
	if err != nil {
		return StoryboardDiff{}, err
	}
	draft.Shots = shots
	if err := validateStoryboardDiffDependencies(draft, diffContext); err != nil {
		return StoryboardDiff{}, err
	}
	liveByKey := map[string]MaterializedStoryboardShot{}
	for _, live := range diffContext.LiveShots {
		key := strings.TrimSpace(live.Shot.SourceKey)
		if key == "" || liveByKey[key].ID != "" {
			return StoryboardDiff{}, &WorkflowValidationError{
				Code: "storyboard_source_key_invalid", Field: "liveShots", Message: "现有分镜包含空白或重复的稳定标识",
				Fix: "请先修复项目分镜的 sourceKey。", Details: map[string]any{"sourceKey": key},
			}
		}
		live.Shot, err = normalizeSingleStoryboardShot(live.Shot)
		if err != nil {
			return StoryboardDiff{}, err
		}
		liveByKey[key] = live
	}
	diff := NewStoryboardDiff()
	diff.StoryboardID = draft.ID
	diff.Revision = draft.Revision
	seen := map[string]bool{}
	for _, shot := range draft.Shots {
		seen[shot.SourceKey] = true
		live, exists := liveByKey[shot.SourceKey]
		operation := "create"
		shotID := ""
		before := map[string]any{}
		if exists {
			shotID = live.ID
			before = storyboardShotMap(live.Shot)
			operation = "update"
			if reflect.DeepEqual(live.Shot, shot) {
				operation = "unchanged"
			}
		}
		diff.Items = append(diff.Items, StoryboardDiffItem{
			Operation: operation, SourceKey: shot.SourceKey, ShotID: shotID,
			Before: before, After: storyboardShotMap(shot),
		})
	}
	for _, live := range diffContext.LiveShots {
		if seen[live.Shot.SourceKey] {
			continue
		}
		diff.Items = append(diff.Items, StoryboardDiffItem{
			Operation: "archive", SourceKey: live.Shot.SourceKey, ShotID: live.ID,
			Before: storyboardShotMap(live.Shot), After: map[string]any{},
		})
	}
	diff.DiffToken = storyboardDiffToken(draft, diffContext, diff.Items)
	return diff, nil
}

func validateStoryboardDiffDependencies(draft StoryboardVersion, diffContext StoryboardDiffContext) error {
	if draft.SourceScriptRevision != diffContext.CurrentScriptRevision ||
		draft.SourceBreakdownID != diffContext.CurrentBreakdownID ||
		draft.SourceAssetRevision != diffContext.CurrentAssetRevision ||
		draft.SourceCapabilityVersion != diffContext.CurrentCapabilityVersion ||
		draft.BaselineStoryboardID != diffContext.CurrentConfirmedStoryboardID {
		return &WorkflowConflictError{
			Code: "workflow_dependency_conflict", Message: "剧本、资产、模型能力或当前分镜已经变化，请重新设计或预览",
			CurrentRevision: draft.Revision,
			Details: map[string]any{
				"sourceScriptRevision": draft.SourceScriptRevision, "currentScriptRevision": diffContext.CurrentScriptRevision,
				"sourceBreakdownId": draft.SourceBreakdownID, "currentBreakdownId": diffContext.CurrentBreakdownID,
				"sourceAssetRevision": draft.SourceAssetRevision, "currentAssetRevision": diffContext.CurrentAssetRevision,
				"sourceCapabilityVersion": draft.SourceCapabilityVersion, "currentCapabilityVersion": diffContext.CurrentCapabilityVersion,
				"baselineStoryboardId": draft.BaselineStoryboardID, "currentStoryboardId": diffContext.CurrentConfirmedStoryboardID,
			},
		}
	}
	return nil
}

func storyboardDiffToken(draft StoryboardVersion, diffContext StoryboardDiffContext, items []StoryboardDiffItem) string {
	canonicalItems := append([]StoryboardDiffItem{}, items...)
	sort.SliceStable(canonicalItems, func(left, right int) bool {
		if canonicalItems[left].SourceKey != canonicalItems[right].SourceKey {
			return canonicalItems[left].SourceKey < canonicalItems[right].SourceKey
		}
		return canonicalItems[left].Operation < canonicalItems[right].Operation
	})
	payload, _ := json.Marshal(struct {
		StoryboardID                 string               `json:"storyboardId"`
		Revision                     int                  `json:"revision"`
		BaselineStoryboardID         string               `json:"baselineStoryboardId"`
		CurrentConfirmedStoryboardID string               `json:"currentConfirmedStoryboardId"`
		SourceScriptRevision         int                  `json:"sourceScriptRevision"`
		SourceBreakdownID            string               `json:"sourceBreakdownId"`
		SourceAssetRevision          int                  `json:"sourceAssetRevision"`
		SourceCapabilityVersion      string               `json:"sourceCapabilityVersion"`
		Items                        []StoryboardDiffItem `json:"items"`
	}{
		StoryboardID: draft.ID, Revision: draft.Revision,
		BaselineStoryboardID: draft.BaselineStoryboardID, CurrentConfirmedStoryboardID: diffContext.CurrentConfirmedStoryboardID,
		SourceScriptRevision: draft.SourceScriptRevision, SourceBreakdownID: draft.SourceBreakdownID,
		SourceAssetRevision: draft.SourceAssetRevision, SourceCapabilityVersion: draft.SourceCapabilityVersion,
		Items: canonicalItems,
	})
	sum := sha256.Sum256(payload)
	return hex.EncodeToString(sum[:])
}

func normalizeSingleStoryboardShot(shot StoryboardShot) (StoryboardShot, error) {
	shots, err := normalizeStoryboardDraftShots([]StoryboardShot{shot})
	if err != nil {
		return StoryboardShot{}, err
	}
	return shots[0], nil
}

func normalizeStoryboardReferenceIntents(references []StoryboardReferenceIntent) []StoryboardReferenceIntent {
	result := make([]StoryboardReferenceIntent, 0, len(references))
	for index, reference := range references {
		reference.AssetKey = strings.TrimSpace(reference.AssetKey)
		reference.Role = strings.ToLower(strings.TrimSpace(reference.Role))
		reference.UsageNote = strings.TrimSpace(reference.UsageNote)
		if reference.AssetKey == "" {
			continue
		}
		if reference.Role == "" {
			reference.Role = "reference_image"
		}
		if reference.SortOrder <= 0 {
			reference.SortOrder = index + 1
		}
		result = append(result, reference)
	}
	return result
}

func cleanStoryboardStringList(values []string) []string {
	result := []string{}
	seen := map[string]bool{}
	for _, value := range values {
		value = strings.TrimSpace(value)
		if value == "" || seen[value] {
			continue
		}
		seen[value] = true
		result = append(result, value)
	}
	return result
}

func storyboardShotMap(shot StoryboardShot) map[string]any {
	payload, _ := json.Marshal(shot)
	result := map[string]any{}
	_ = json.Unmarshal(payload, &result)
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

func buildStoryboardMaterializationPlan(draft StoryboardVersion, diffContext StoryboardDiffContext, input ConfirmStoryboardInput) (StoryboardMaterializationPlan, error) {
	if input.ExpectedRevision != draft.Revision {
		return StoryboardMaterializationPlan{}, &WorkflowConflictError{Code: "workflow_revision_conflict", Message: "分镜草稿版本已变化", CurrentRevision: draft.Revision}
	}
	if err := validateStoryboardDiffDependencies(draft, diffContext); err != nil {
		return StoryboardMaterializationPlan{}, err
	}
	diff, err := previewStoryboardDiff(draft, diffContext)
	if err != nil {
		return StoryboardMaterializationPlan{}, err
	}
	if strings.TrimSpace(input.DiffToken) == "" || input.DiffToken != diff.DiffToken {
		return StoryboardMaterializationPlan{}, &WorkflowConflictError{Code: "workflow_diff_conflict", Message: "分镜差异已变化", CurrentRevision: draft.Revision}
	}
	return StoryboardMaterializationPlan{}, nil
}
