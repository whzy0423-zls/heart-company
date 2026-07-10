package videoproject

import (
	"context"
	"crypto/sha256"
	"database/sql"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"sort"
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

type BreakdownMapping struct {
	ItemKey      string `json:"itemKey"`
	Decision     string `json:"decision"`
	ExistingKind string `json:"existingKind"`
	ExistingID   string `json:"existingId"`
}

type ConfirmBreakdownInput struct {
	ExpectedRevision int                `json:"expectedRevision"`
	DiffToken        string             `json:"diffToken"`
	Mappings         []BreakdownMapping `json:"mappings"`
}

type PreviewBreakdownDiffInput struct {
	ExpectedRevision int                `json:"expectedRevision"`
	Mappings         []BreakdownMapping `json:"mappings"`
}

type ExistingBreakdownAsset struct {
	ID                string `json:"id"`
	ProjectID         string `json:"projectId"`
	Kind              string `json:"kind"`
	ItemKey           string `json:"itemKey"`
	Name              string `json:"name"`
	SourceBreakdownID string `json:"sourceBreakdownId"`
}

type BreakdownDiffContext struct {
	CurrentScriptRevision int
	BaselineBreakdownID   string
	ExistingAssets        []ExistingBreakdownAsset
}

type BreakdownDiffItem struct {
	Operation    string `json:"operation"`
	Kind         string `json:"kind"`
	ItemKey      string `json:"itemKey"`
	Name         string `json:"name"`
	Decision     string `json:"decision"`
	ExistingKind string `json:"existingKind"`
	ExistingID   string `json:"existingId"`
}

type BreakdownDiff struct {
	BreakdownID         string              `json:"breakdownId"`
	Revision            int                 `json:"revision"`
	BaselineBreakdownID string              `json:"baselineBreakdownId"`
	DiffToken           string              `json:"diffToken"`
	Items               []BreakdownDiffItem `json:"items"`
}

type BreakdownMaterializationUpsert struct {
	Kind       string
	Item       BreakdownItem
	ExistingID string
}

type BreakdownMaterializationPlan struct {
	Upserts []BreakdownMaterializationUpsert
	Detach  []ExistingBreakdownAsset
	Ignored []BreakdownItem
}

type typedBreakdownItem struct {
	Kind string
	Item BreakdownItem
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

func previewBreakdownDiff(draft BreakdownVersion, diffContext BreakdownDiffContext, mappings []BreakdownMapping) (BreakdownDiff, error) {
	if draft.Status != "draft" {
		return BreakdownDiff{}, &WorkflowValidationError{
			Code: "breakdown_not_editable", Field: "status", Message: "只有拆解草稿可以确认",
			Fix: "请重新生成一份拆解草稿。", Details: map[string]any{"status": draft.Status},
		}
	}
	if draft.SourceScriptRevision != diffContext.CurrentScriptRevision {
		return BreakdownDiff{}, &WorkflowConflictError{
			Code: "workflow_dependency_conflict", Message: "剧本已变化，这份拆解草稿已过期",
			CurrentRevision: diffContext.CurrentScriptRevision,
			Details: map[string]any{
				"sourceScriptRevision":  draft.SourceScriptRevision,
				"currentScriptRevision": diffContext.CurrentScriptRevision,
			},
		}
	}
	items, err := collectBreakdownItems(draft)
	if err != nil {
		return BreakdownDiff{}, err
	}
	mappingByKey, err := validateBreakdownMappings(draft.ProjectID, items, diffContext.ExistingAssets, mappings)
	if err != nil {
		return BreakdownDiff{}, err
	}
	diff := BreakdownDiff{
		BreakdownID:         draft.ID,
		Revision:            draft.Revision,
		BaselineBreakdownID: strings.TrimSpace(diffContext.BaselineBreakdownID),
		Items:               make([]BreakdownDiffItem, 0, len(items)),
	}
	for _, typedItem := range items {
		mapping := mappingByKey[typedItem.Item.Key]
		operation := "create"
		if mapping.Decision == "ignored" {
			operation = "ignore"
		} else if mapping.ExistingID != "" {
			operation = "link"
		}
		diff.Items = append(diff.Items, BreakdownDiffItem{
			Operation: operation, Kind: typedItem.Kind, ItemKey: typedItem.Item.Key,
			Name: typedItem.Item.Name, Decision: mapping.Decision,
			ExistingKind: mapping.ExistingKind, ExistingID: mapping.ExistingID,
		})
	}
	diff.DiffToken = breakdownDiffToken(draft, diffContext, diff.Items)
	return diff, nil
}

func buildBreakdownMaterializationPlan(draft BreakdownVersion, diffContext BreakdownDiffContext, mappings []BreakdownMapping, expectedToken string) (BreakdownMaterializationPlan, error) {
	diff, err := previewBreakdownDiff(draft, diffContext, mappings)
	if err != nil {
		return BreakdownMaterializationPlan{}, err
	}
	if strings.TrimSpace(expectedToken) == "" || !constantTimeTextEqual(diff.DiffToken, strings.TrimSpace(expectedToken)) {
		return BreakdownMaterializationPlan{}, &WorkflowConflictError{
			Code: "workflow_diff_conflict", Message: "拆解差异已经变化，请重新预览后再确认",
			CurrentRevision: draft.Revision,
			Details:         map[string]any{"currentDiffToken": diff.DiffToken},
		}
	}
	items, _ := collectBreakdownItems(draft)
	mappingByKey, _ := validateBreakdownMappings(draft.ProjectID, items, diffContext.ExistingAssets, mappings)
	plan := BreakdownMaterializationPlan{
		Upserts: []BreakdownMaterializationUpsert{},
		Detach:  []ExistingBreakdownAsset{},
		Ignored: []BreakdownItem{},
	}
	linked := map[string]bool{}
	for _, typedItem := range items {
		mapping := mappingByKey[typedItem.Item.Key]
		item := typedItem.Item
		item.Decision = mapping.Decision
		if mapping.Decision == "ignored" {
			plan.Ignored = append(plan.Ignored, item)
			continue
		}
		plan.Upserts = append(plan.Upserts, BreakdownMaterializationUpsert{
			Kind: typedItem.Kind, Item: item, ExistingID: mapping.ExistingID,
		})
		if mapping.ExistingID != "" {
			linked[mapping.ExistingKind+":"+mapping.ExistingID] = true
		}
	}
	for _, existing := range diffContext.ExistingAssets {
		if existing.SourceBreakdownID != diffContext.BaselineBreakdownID || existing.SourceBreakdownID == "" {
			continue
		}
		if !linked[existing.Kind+":"+existing.ID] {
			plan.Detach = append(plan.Detach, existing)
		}
	}
	return plan, nil
}

func collectBreakdownItems(draft BreakdownVersion) ([]typedBreakdownItem, error) {
	items := []typedBreakdownItem{}
	seen := map[string]bool{}
	for _, group := range []struct {
		kind  string
		items []BreakdownItem
	}{
		{kind: "character", items: draft.Characters},
		{kind: "scene", items: draft.Scenes},
		{kind: "prop", items: draft.Props},
		{kind: "outfit", items: draft.Outfits},
		{kind: "style", items: draft.Styles},
	} {
		for index, item := range group.items {
			item.Key = strings.TrimSpace(item.Key)
			item.Name = strings.TrimSpace(item.Name)
			if item.Key == "" || seen[item.Key] {
				return nil, &WorkflowValidationError{
					Code: "breakdown_item_key_invalid", Field: fmt.Sprintf("%ss[%d].key", group.kind, index),
					Message: "拆解项目缺少唯一标识", Fix: "请重新生成拆解，或为该项目设置不同的稳定 key。",
					Details: map[string]any{"key": item.Key},
				}
			}
			if item.Name == "" {
				return nil, &WorkflowValidationError{
					Code: "breakdown_item_name_required", Field: fmt.Sprintf("%ss[%d].name", group.kind, index),
					Message: "拆解项目名称不能为空", Fix: "请填写一个便于识别的名称。", Details: map[string]any{"key": item.Key},
				}
			}
			seen[item.Key] = true
			items = append(items, typedBreakdownItem{Kind: group.kind, Item: item})
		}
	}
	return items, nil
}

func validateBreakdownMappings(projectID string, items []typedBreakdownItem, existingAssets []ExistingBreakdownAsset, mappings []BreakdownMapping) (map[string]BreakdownMapping, error) {
	itemByKey := map[string]typedBreakdownItem{}
	for _, item := range items {
		itemByKey[item.Item.Key] = item
	}
	existingByIdentity := map[string]ExistingBreakdownAsset{}
	for _, existing := range existingAssets {
		existing.ID = strings.TrimSpace(existing.ID)
		existing.Kind = strings.TrimSpace(existing.Kind)
		if existing.ID != "" && existing.Kind != "" {
			existingByIdentity[existing.Kind+":"+existing.ID] = existing
		}
	}
	mappingByKey := map[string]BreakdownMapping{}
	for index, mapping := range mappings {
		mapping.ItemKey = strings.TrimSpace(mapping.ItemKey)
		mapping.Decision = strings.ToLower(strings.TrimSpace(mapping.Decision))
		mapping.ExistingKind = strings.ToLower(strings.TrimSpace(mapping.ExistingKind))
		mapping.ExistingID = strings.TrimSpace(mapping.ExistingID)
		item, ok := itemByKey[mapping.ItemKey]
		if !ok || mapping.ItemKey == "" || mappingByKey[mapping.ItemKey].ItemKey != "" {
			return nil, breakdownMappingValidation("breakdown_mapping_invalid", fmt.Sprintf("mappings[%d].itemKey", index), "拆解映射包含未知或重复项目", mapping.ItemKey)
		}
		if mapping.Decision != "confirmed" && mapping.Decision != "ignored" {
			return nil, breakdownMappingValidation("breakdown_mapping_invalid", fmt.Sprintf("mappings[%d].decision", index), "请选择确认或忽略", mapping.ItemKey)
		}
		if mapping.Decision == "ignored" {
			if mapping.ExistingKind != "" || mapping.ExistingID != "" {
				return nil, breakdownMappingValidation("breakdown_mapping_invalid", fmt.Sprintf("mappings[%d]", index), "忽略的项目不能再关联已有资产", mapping.ItemKey)
			}
		} else if (mapping.ExistingKind == "") != (mapping.ExistingID == "") {
			return nil, breakdownMappingValidation("breakdown_mapping_invalid", fmt.Sprintf("mappings[%d]", index), "关联已有资产时，类型和 ID 必须同时填写", mapping.ItemKey)
		} else if mapping.ExistingID != "" {
			if mapping.ExistingKind != item.Kind {
				return nil, breakdownMappingValidation("breakdown_mapping_kind_mismatch", fmt.Sprintf("mappings[%d].existingKind", index), "不能把不同类型的资产相互关联", mapping.ItemKey)
			}
			existing, ok := existingByIdentity[mapping.ExistingKind+":"+mapping.ExistingID]
			if !ok || existing.ProjectID != projectID {
				return nil, breakdownMappingValidation("breakdown_mapping_asset_not_found", fmt.Sprintf("mappings[%d].existingId", index), "关联的已有资产不存在或不属于当前项目", mapping.ItemKey)
			}
		}
		mappingByKey[mapping.ItemKey] = mapping
	}
	for _, item := range items {
		if mappingByKey[item.Item.Key].ItemKey == "" {
			return nil, breakdownMappingValidation("breakdown_mapping_required", "mappings", "每个拆解项目都需要明确确认、忽略或关联已有资产", item.Item.Key)
		}
	}
	return mappingByKey, nil
}

func breakdownMappingValidation(code, field, message, itemKey string) *WorkflowValidationError {
	return &WorkflowValidationError{
		Code: code, Field: field, Message: message,
		Fix:     "请在差异预览中为这一项选择明确的处理方式。",
		Details: map[string]any{"itemKey": itemKey},
	}
}

func breakdownDiffToken(draft BreakdownVersion, diffContext BreakdownDiffContext, items []BreakdownDiffItem) string {
	canonical := struct {
		BreakdownID           string
		Revision              int
		BaselineBreakdownID   string
		CurrentScriptRevision int
		SourceScriptRevision  int
		Items                 []BreakdownDiffItem
	}{
		BreakdownID: draft.ID, Revision: draft.Revision,
		BaselineBreakdownID:   strings.TrimSpace(diffContext.BaselineBreakdownID),
		CurrentScriptRevision: diffContext.CurrentScriptRevision,
		SourceScriptRevision:  draft.SourceScriptRevision,
		Items:                 append([]BreakdownDiffItem{}, items...),
	}
	sort.SliceStable(canonical.Items, func(left, right int) bool {
		if canonical.Items[left].Kind != canonical.Items[right].Kind {
			return canonical.Items[left].Kind < canonical.Items[right].Kind
		}
		return canonical.Items[left].ItemKey < canonical.Items[right].ItemKey
	})
	payload, _ := json.Marshal(canonical)
	sum := sha256.Sum256(payload)
	return hex.EncodeToString(sum[:])
}

func constantTimeTextEqual(left, right string) bool {
	leftSum := sha256.Sum256([]byte(left))
	rightSum := sha256.Sum256([]byte(right))
	var difference byte
	for index := range leftSum {
		difference |= leftSum[index] ^ rightSum[index]
	}
	return difference == 0
}

func (store *Store) PreviewBreakdownDiff(ctx context.Context, projectID, breakdownID string, input PreviewBreakdownDiffInput) (BreakdownDiff, error) {
	parsedProjectID, err := parseID(projectID)
	if err != nil {
		return BreakdownDiff{}, err
	}
	parsedBreakdownID, err := parseID(breakdownID)
	if err != nil {
		return BreakdownDiff{}, fmt.Errorf("无效的拆解版本 ID")
	}
	draft, err := store.loadBreakdownVersion(ctx, parsedProjectID, parsedBreakdownID)
	if err != nil {
		return BreakdownDiff{}, err
	}
	if input.ExpectedRevision != draft.Revision {
		return BreakdownDiff{}, revisionConflict(draft.Revision, "拆解草稿已更新，请刷新后重新预览")
	}
	diffContext, err := store.loadBreakdownDiffContext(ctx, parsedProjectID)
	if err != nil {
		return BreakdownDiff{}, err
	}
	return previewBreakdownDiff(draft, diffContext, input.Mappings)
}

func (store *Store) ConfirmBreakdown(ctx context.Context, projectID, breakdownID string, input ConfirmBreakdownInput) (BreakdownVersion, error) {
	parsedProjectID, err := parseID(projectID)
	if err != nil {
		return BreakdownVersion{}, err
	}
	parsedBreakdownID, err := parseID(breakdownID)
	if err != nil {
		return BreakdownVersion{}, fmt.Errorf("无效的拆解版本 ID")
	}
	transaction, err := store.db.BeginTx(ctx, nil)
	if err != nil {
		return BreakdownVersion{}, err
	}
	defer transaction.Rollback()

	var currentScriptRevision, confirmedScriptRevision int
	if err := transaction.QueryRowContext(ctx,
		`SELECT script_revision, confirmed_script_revision
		   FROM video_projects
		  WHERE id=$1
		  FOR UPDATE`,
		parsedProjectID,
	).Scan(&currentScriptRevision, &confirmedScriptRevision); err != nil {
		if err == sql.ErrNoRows {
			return BreakdownVersion{}, fmt.Errorf("项目不存在")
		}
		return BreakdownVersion{}, err
	}
	if confirmedScriptRevision != currentScriptRevision {
		return BreakdownVersion{}, &WorkflowConflictError{
			Code: "workflow_dependency_conflict", Message: "当前剧本尚未确认，不能确认拆解结果",
			CurrentRevision: currentScriptRevision,
			Details:         map[string]any{"confirmedScriptRevision": confirmedScriptRevision},
		}
	}
	draft, err := loadBreakdownVersionTx(ctx, transaction, parsedProjectID, parsedBreakdownID, true)
	if err != nil {
		return BreakdownVersion{}, err
	}
	if input.ExpectedRevision != draft.Revision {
		return BreakdownVersion{}, revisionConflict(draft.Revision, "拆解草稿已更新，请重新预览后再确认")
	}
	diffContext, err := loadBreakdownDiffContextTx(ctx, transaction, parsedProjectID, currentScriptRevision)
	if err != nil {
		return BreakdownVersion{}, err
	}
	plan, err := buildBreakdownMaterializationPlan(draft, diffContext, input.Mappings, input.DiffToken)
	if err != nil {
		return BreakdownVersion{}, err
	}

	if err := prepareBreakdownAssetKeys(ctx, transaction, parsedProjectID, parsedBreakdownID, plan); err != nil {
		return BreakdownVersion{}, err
	}
	for _, upsert := range plan.Upserts {
		if err := materializeBreakdownAsset(ctx, transaction, parsedProjectID, parsedBreakdownID, upsert); err != nil {
			return BreakdownVersion{}, err
		}
	}
	for _, detached := range plan.Detach {
		if err := detachBreakdownAsset(ctx, transaction, parsedProjectID, detached); err != nil {
			return BreakdownVersion{}, err
		}
	}

	confirmed := applyBreakdownDecisions(draft, input.Mappings)
	characters, _ := json.Marshal(confirmed.Characters)
	scenes, _ := json.Marshal(confirmed.Scenes)
	props, _ := json.Marshal(confirmed.Props)
	outfits, _ := json.Marshal(confirmed.Outfits)
	styles, _ := json.Marshal(confirmed.Styles)
	if diffContext.BaselineBreakdownID != "" && diffContext.BaselineBreakdownID != draft.ID {
		if _, err := transaction.ExecContext(ctx,
			`UPDATE video_project_breakdowns
			    SET status='superseded', update_time=now()
			  WHERE project_id=$1 AND id=$2 AND status='confirmed'`,
			parsedProjectID, diffContext.BaselineBreakdownID,
		); err != nil {
			return BreakdownVersion{}, err
		}
	}
	result, err := transaction.ExecContext(ctx,
		`UPDATE video_project_breakdowns
		    SET status='confirmed', characters=$1::jsonb, scenes=$2::jsonb, props=$3::jsonb,
		        outfits=$4::jsonb, styles=$5::jsonb, error_message='', update_time=now()
		  WHERE project_id=$6 AND id=$7 AND status='draft' AND revision=$8`,
		string(characters), string(scenes), string(props), string(outfits), string(styles),
		parsedProjectID, parsedBreakdownID, input.ExpectedRevision,
	)
	if err != nil {
		return BreakdownVersion{}, err
	}
	if affected, rowsErr := result.RowsAffected(); rowsErr != nil {
		return BreakdownVersion{}, rowsErr
	} else if affected != 1 {
		return BreakdownVersion{}, revisionConflict(draft.Revision, "拆解草稿状态已变化，请刷新后重试")
	}
	if _, err := transaction.ExecContext(ctx,
		`UPDATE video_projects
		    SET asset_revision=asset_revision+1, breakdown_confirmed_at=now(), update_time=now()
		  WHERE id=$1`,
		parsedProjectID,
	); err != nil {
		return BreakdownVersion{}, err
	}
	if err := transaction.Commit(); err != nil {
		return BreakdownVersion{}, err
	}
	confirmed.Status = "confirmed"
	confirmed.UpdateTime = formatTime(time.Now())
	return confirmed, nil
}

func applyBreakdownDecisions(draft BreakdownVersion, mappings []BreakdownMapping) BreakdownVersion {
	decisionByKey := map[string]string{}
	for _, mapping := range mappings {
		decisionByKey[strings.TrimSpace(mapping.ItemKey)] = strings.ToLower(strings.TrimSpace(mapping.Decision))
	}
	apply := func(items []BreakdownItem) []BreakdownItem {
		result := append([]BreakdownItem{}, items...)
		for index := range result {
			result[index].Decision = decisionByKey[result[index].Key]
		}
		return result
	}
	draft.Characters = apply(draft.Characters)
	draft.Scenes = apply(draft.Scenes)
	draft.Props = apply(draft.Props)
	draft.Outfits = apply(draft.Outfits)
	draft.Styles = apply(draft.Styles)
	return draft
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

func (store *Store) loadBreakdownVersion(ctx context.Context, projectID, breakdownID int64) (BreakdownVersion, error) {
	return scanBreakdownVersion(store.db.QueryRowContext(ctx, breakdownVersionSelectSQL(false), projectID, breakdownID))
}

func loadBreakdownVersionTx(ctx context.Context, transaction *sql.Tx, projectID, breakdownID int64, forUpdate bool) (BreakdownVersion, error) {
	return scanBreakdownVersion(transaction.QueryRowContext(ctx, breakdownVersionSelectSQL(forUpdate), projectID, breakdownID))
}

func breakdownVersionSelectSQL(forUpdate bool) string {
	query := `SELECT id::text, project_id::text, version, revision, status, source_script_revision,
	                 script_snapshot, characters, scenes, props, outfits, styles, story_beats,
	                 raw_result, error_message, create_time, update_time
	            FROM video_project_breakdowns
	           WHERE project_id=$1 AND id=$2`
	if forUpdate {
		query += ` FOR UPDATE`
	}
	return query
}

func scanBreakdownVersion(row interface{ Scan(...any) error }) (BreakdownVersion, error) {
	version := NewBreakdownVersion()
	var characters, scenes, props, outfits, styles, storyBeats []byte
	var createTime, updateTime time.Time
	if err := row.Scan(
		&version.ID, &version.ProjectID, &version.Version, &version.Revision, &version.Status,
		&version.SourceScriptRevision, &version.ScriptSnapshot,
		&characters, &scenes, &props, &outfits, &styles, &storyBeats,
		&version.RawResult, &version.ErrorMessage, &createTime, &updateTime,
	); err != nil {
		if err == sql.ErrNoRows {
			return BreakdownVersion{}, fmt.Errorf("拆解版本不存在或不属于当前项目")
		}
		return BreakdownVersion{}, err
	}
	if err := decodeBreakdownVersionJSON(&version, characters, scenes, props, outfits, styles, storyBeats); err != nil {
		return BreakdownVersion{}, err
	}
	version.CreateTime = formatTime(createTime)
	version.UpdateTime = formatTime(updateTime)
	return version, nil
}

func (store *Store) loadBreakdownDiffContext(ctx context.Context, projectID int64) (BreakdownDiffContext, error) {
	var currentScriptRevision int
	if err := store.db.QueryRowContext(ctx,
		`SELECT script_revision FROM video_projects WHERE id=$1`,
		projectID,
	).Scan(&currentScriptRevision); err != nil {
		if err == sql.ErrNoRows {
			return BreakdownDiffContext{}, fmt.Errorf("项目不存在")
		}
		return BreakdownDiffContext{}, err
	}
	return loadBreakdownDiffContextFromQueryer(ctx, store.db, projectID, currentScriptRevision)
}

func loadBreakdownDiffContextTx(ctx context.Context, transaction *sql.Tx, projectID int64, currentScriptRevision int) (BreakdownDiffContext, error) {
	return loadBreakdownDiffContextFromQueryer(ctx, transaction, projectID, currentScriptRevision)
}

type breakdownContextQueryer interface {
	QueryRowContext(context.Context, string, ...any) *sql.Row
	QueryContext(context.Context, string, ...any) (*sql.Rows, error)
}

func loadBreakdownDiffContextFromQueryer(ctx context.Context, queryer breakdownContextQueryer, projectID int64, currentScriptRevision int) (BreakdownDiffContext, error) {
	diffContext := BreakdownDiffContext{CurrentScriptRevision: currentScriptRevision, ExistingAssets: []ExistingBreakdownAsset{}}
	if err := queryer.QueryRowContext(ctx,
		`SELECT COALESCE((
		   SELECT id::text FROM video_project_breakdowns
		    WHERE project_id=$1 AND status='confirmed'
		    LIMIT 1
		 ),'')`,
		projectID,
	).Scan(&diffContext.BaselineBreakdownID); err != nil {
		return BreakdownDiffContext{}, err
	}
	rows, err := queryer.QueryContext(ctx,
		`SELECT id::text, project_id::text, kind, item_key, name, source_breakdown_id
		   FROM (
		     SELECT c.id, c.project_id, 'character'::text AS kind, c.breakdown_item_key AS item_key,
		            c.name, COALESCE(c.source_breakdown_id::text,'') AS source_breakdown_id, 1 AS kind_order
		       FROM video_project_characters c WHERE c.project_id=$1
		     UNION ALL
		     SELECT scene.id, scene.project_id, 'scene'::text, scene.breakdown_item_key,
		            scene.name, COALESCE(scene.source_breakdown_id::text,''), 2
		       FROM video_project_scenes scene WHERE scene.project_id=$1
		     UNION ALL
		     SELECT asset.id, asset.project_id, asset.type, asset.breakdown_item_key,
		            asset.name, COALESCE(asset.source_breakdown_id::text,''),
		            CASE asset.type WHEN 'prop' THEN 3 WHEN 'outfit' THEN 4 ELSE 5 END
		       FROM video_project_assets asset WHERE asset.project_id=$1
		   ) assets
		  ORDER BY kind_order, id`,
		projectID,
	)
	if err != nil {
		return BreakdownDiffContext{}, err
	}
	defer rows.Close()
	for rows.Next() {
		var asset ExistingBreakdownAsset
		if err := rows.Scan(&asset.ID, &asset.ProjectID, &asset.Kind, &asset.ItemKey, &asset.Name, &asset.SourceBreakdownID); err != nil {
			return BreakdownDiffContext{}, err
		}
		diffContext.ExistingAssets = append(diffContext.ExistingAssets, asset)
	}
	return diffContext, rows.Err()
}

func prepareBreakdownAssetKeys(ctx context.Context, transaction *sql.Tx, projectID, breakdownID int64, plan BreakdownMaterializationPlan) error {
	for _, upsert := range plan.Upserts {
		if upsert.ExistingID == "" {
			continue
		}
		assetID, err := parseID(upsert.ExistingID)
		if err != nil {
			return fmt.Errorf("无效的已有资产 ID")
		}
		temporaryKey := fmt.Sprintf("pending:%d:%s:%d", breakdownID, upsert.Kind, assetID)
		query, err := breakdownAssetUpdateQuery(upsert.Kind, `breakdown_item_key=$1, update_time=now()`)
		if err != nil {
			return err
		}
		result, err := transaction.ExecContext(ctx, query, temporaryKey, projectID, assetID)
		if err != nil {
			return err
		}
		if affected, rowsErr := result.RowsAffected(); rowsErr != nil {
			return rowsErr
		} else if affected != 1 {
			return breakdownMappingValidation("breakdown_mapping_asset_not_found", "mappings", "关联的已有资产不存在或不属于当前项目", upsert.Item.Key)
		}
	}
	return nil
}

func materializeBreakdownAsset(ctx context.Context, transaction *sql.Tx, projectID, breakdownID int64, upsert BreakdownMaterializationUpsert) error {
	item := upsert.Item
	if upsert.ExistingID != "" {
		assetID, _ := parseID(upsert.ExistingID)
		query, err := breakdownAssetUpdateQuery(upsert.Kind, `name=$1, description=$2, visual_prompt=$3,
			breakdown_item_key=$4, source_breakdown_id=$5, source='ai', status='confirmed', required=$6, update_time=now()`)
		if err != nil {
			return err
		}
		args := []any{item.Name, item.Description, item.VisualPrompt, item.Key, breakdownID, item.Required, projectID, assetID}
		if upsert.Kind == "prop" || upsert.Kind == "outfit" || upsert.Kind == "style" {
			query, err = breakdownAssetUpdateQuery(upsert.Kind, `name=$1, description=$2, visual_prompt=$3, usage_note=$4,
				breakdown_item_key=$5, source_breakdown_id=$6, source='ai', status='confirmed', required=$7, update_time=now()`)
			if err != nil {
				return err
			}
			args = []any{item.Name, item.Description, item.VisualPrompt, item.UsageNote, item.Key, breakdownID, item.Required, projectID, assetID}
		}
		result, err := transaction.ExecContext(ctx, query, args...)
		if err != nil {
			return err
		}
		if affected, rowsErr := result.RowsAffected(); rowsErr != nil {
			return rowsErr
		} else if affected != 1 {
			return breakdownMappingValidation("breakdown_mapping_asset_not_found", "mappings", "关联的已有资产不存在或不属于当前项目", item.Key)
		}
		return nil
	}

	switch upsert.Kind {
	case "character":
		_, err := transaction.ExecContext(ctx,
			`INSERT INTO video_project_characters (
			   project_id, name, description, visual_prompt, source, status, required,
			   breakdown_item_key, source_breakdown_id
			 ) VALUES ($1,$2,$3,$4,'ai','confirmed',$5,$6,$7)`,
			projectID, item.Name, item.Description, item.VisualPrompt, item.Required, item.Key, breakdownID,
		)
		return err
	case "scene":
		_, err := transaction.ExecContext(ctx,
			`INSERT INTO video_project_scenes (
			   project_id, name, description, visual_prompt, source, status, required,
			   breakdown_item_key, source_breakdown_id
			 ) VALUES ($1,$2,$3,$4,'ai','confirmed',$5,$6,$7)`,
			projectID, item.Name, item.Description, item.VisualPrompt, item.Required, item.Key, breakdownID,
		)
		return err
	case "prop", "outfit", "style":
		_, err := transaction.ExecContext(ctx,
			`INSERT INTO video_project_assets (
			   project_id, type, name, description, visual_prompt, usage_note, required,
			   breakdown_item_key, source_breakdown_id, source, status
			 ) VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9,'ai','confirmed')`,
			projectID, upsert.Kind, item.Name, item.Description, item.VisualPrompt, item.UsageNote,
			item.Required, item.Key, breakdownID,
		)
		return err
	default:
		return fmt.Errorf("不支持的拆解资产类型 %q", upsert.Kind)
	}
}

func detachBreakdownAsset(ctx context.Context, transaction *sql.Tx, projectID int64, asset ExistingBreakdownAsset) error {
	assetID, err := parseID(asset.ID)
	if err != nil {
		return fmt.Errorf("无效的已有资产 ID")
	}
	query, err := breakdownAssetUpdateQuery(asset.Kind, `status='detached', required=false, update_time=now()`)
	if err != nil {
		return err
	}
	_, err = transaction.ExecContext(ctx, query, projectID, assetID)
	return err
}

func breakdownAssetUpdateQuery(kind, assignments string) (string, error) {
	table := ""
	switch kind {
	case "character":
		table = "video_project_characters"
	case "scene":
		table = "video_project_scenes"
	case "prop", "outfit", "style":
		table = "video_project_assets"
	default:
		return "", fmt.Errorf("不支持的拆解资产类型 %q", kind)
	}
	if strings.Contains(assignments, ";") {
		return "", fmt.Errorf("无效的资产更新语句")
	}
	return fmt.Sprintf(`UPDATE %s SET %s WHERE project_id=$%d AND id=$%d`, table, assignments, countSQLPlaceholders(assignments)+1, countSQLPlaceholders(assignments)+2), nil
}

func countSQLPlaceholders(value string) int {
	maximum := 0
	for index := 1; index <= 20; index++ {
		if strings.Contains(value, fmt.Sprintf("$%d", index)) {
			maximum = index
		}
	}
	return maximum
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
