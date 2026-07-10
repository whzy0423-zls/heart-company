package videoproject

import (
	"fmt"
	"math"
	"strings"

	"nine-xing/nx-backend/apps/server/internal/video"
)

var workflowStepKeys = []string{"script", "breakdown", "assets", "storyboard", "prompt", "generate", "compose"}

type WorkflowAssetState struct {
	ID                string
	Kind              string
	ItemKey           string
	SourceBreakdownID string
	Required          bool
	Status            string
	Selected          bool
	SelectedURL       string
}

type WorkflowShotState struct {
	ID                            string
	SourceKey                     string
	Enabled                       bool
	Duration                      int
	ValidationErrors              []WorkflowMessage
	SavedDiagnosticsHash          string
	CurrentDiagnosticsHash        string
	CurrentRequestHash            string
	SelectedGenerationID          string
	SelectedGenerationStatus      string
	SelectedGenerationRequestHash string
	SelectedGenerationAckHash     string
}

type WorkflowComposeState struct {
	Status           string
	VideoURL         string
	SavedInputHash   string
	CurrentInputHash string
}

type WorkflowStatusInput struct {
	Project             Project
	Script              ProjectScriptState
	ConfirmedBreakdown  *BreakdownVersion
	Assets              []WorkflowAssetState
	AssetRevision       int
	ConfirmedStoryboard *StoryboardVersion
	Shots               []WorkflowShotState
	Capabilities        video.Capabilities
	TargetDuration      int
	Compose             WorkflowComposeState
	LegacyAssetCount    int
	LegacyShotCount     int
}

func ComputeWorkflowOverview(input WorkflowStatusInput) WorkflowOverview {
	overview := NewWorkflowOverview()
	overview.Project = input.Project
	overview.Capabilities = input.Capabilities

	script := computeScriptStep(input)
	breakdown := computeBreakdownStep(input, script)
	assets := computeAssetsStep(input, breakdown)
	storyboard := computeStoryboardStep(input, assets)
	prompt := computePromptStep(input, storyboard)
	generate := computeGenerateStep(input, prompt)
	compose := computeComposeStep(input, generate)
	overview.Steps = []WorkflowStepStatus{script, breakdown, assets, storyboard, prompt, generate, compose}

	completed := 0
	overview.CurrentStep = "compose"
	for _, step := range overview.Steps {
		if workflowStatusComplete(step.Status) {
			completed++
			continue
		}
		if overview.CurrentStep == "compose" {
			overview.CurrentStep = step.Key
		}
		overview.Blockers = append(overview.Blockers, step.Blockers...)
	}
	for _, step := range overview.Steps {
		overview.Warnings = append(overview.Warnings, step.Warnings...)
	}
	overview.Overall = int(math.Round(float64(completed) / float64(len(workflowStepKeys)) * 100))
	return overview
}

func computeScriptStep(input WorkflowStatusInput) WorkflowStepStatus {
	step := NewWorkflowStepStatus("script")
	step.NextAction = "填写或导入剧本"
	step.Evidence["scriptRevision"] = input.Script.Revision
	step.Evidence["confirmedScriptRevision"] = input.Script.ConfirmedRevision
	contentPresent := strings.TrimSpace(input.Script.Content) != ""
	switch {
	case !contentPresent && input.LegacyShotCount > 0:
		step.Status = "skipped_existing"
		step.Progress = 100
		step.NextAction = "继续使用已有项目，或补录剧本"
		step.Evidence["legacyShotCount"] = input.LegacyShotCount
	case !contentPresent:
		step.Status = "blocked"
		step.Blockers = []WorkflowMessage{workflowBlocker("script_empty", "还没有剧本", "写一句创意，AI 会帮你整理成完整剧本。", "script", input.Project.ID)}
	case input.Script.ConfirmedRevision != input.Script.Revision || input.Script.Revision <= 0:
		step.Status = "stale"
		step.Progress = 50
		step.NextAction = "确认当前剧本"
		step.Blockers = []WorkflowMessage{workflowBlocker("script_unconfirmed", "剧本修改后还没有确认", "确认当前版本后才能继续拆解。", "script", input.Project.ID)}
	default:
		step.Status = "completed"
		step.Progress = 100
		step.NextAction = "查看剧本拆解"
	}
	return step
}

func computeBreakdownStep(input WorkflowStatusInput, script WorkflowStepStatus) WorkflowStepStatus {
	step := NewWorkflowStepStatus("breakdown")
	step.NextAction = "让 AI 拆解人物、场景和资产"
	if input.ConfirmedBreakdown != nil {
		step.Evidence["breakdownId"] = input.ConfirmedBreakdown.ID
		step.Evidence["breakdownRevision"] = input.ConfirmedBreakdown.Revision
		step.Evidence["sourceScriptRevision"] = input.ConfirmedBreakdown.SourceScriptRevision
	}
	if script.Status == "skipped_existing" && input.LegacyAssetCount > 0 {
		step.Status = "skipped_existing"
		step.Progress = 100
		step.NextAction = "继续使用已有资产"
		step.Evidence["legacyAssetCount"] = input.LegacyAssetCount
		return step
	}
	if script.Status != "completed" {
		if input.ConfirmedBreakdown != nil {
			return staleByPreviousStep(step, "剧本已变化，现有拆解需要重新确认", "script")
		}
		return blockedByPreviousStep(step, "请先完成并确认剧本", "script")
	}
	breakdown := input.ConfirmedBreakdown
	switch {
	case breakdown == nil:
		step.Status = "blocked"
		step.Blockers = []WorkflowMessage{workflowBlocker("breakdown_missing", "还没有确认剧本拆解", "让 AI 拆解后，逐项确认或忽略。", "breakdown", input.Project.ID)}
	case breakdown.Status != "confirmed" || breakdown.SourceScriptRevision != input.Script.ConfirmedRevision:
		step.Status = "stale"
		step.Progress = 50
		step.NextAction = "重新拆解并确认"
		step.Blockers = []WorkflowMessage{workflowBlocker("breakdown_stale", "拆解对应的是旧剧本", "基于当前剧本重新生成并确认拆解。", "breakdown", breakdown.ID)}
	case breakdownHasPendingItems(*breakdown):
		step.Status = "stale"
		step.Progress = 75
		step.NextAction = "处理所有待确认项目"
		step.Blockers = []WorkflowMessage{workflowBlocker("breakdown_pending", "还有拆解项目没有处理", "为每一项选择确认或忽略。", "breakdown", breakdown.ID)}
	default:
		step.Status = "completed"
		step.Progress = 100
		step.NextAction = "准备人物与场景参考图"
	}
	return step
}

func computeAssetsStep(input WorkflowStatusInput, breakdown WorkflowStepStatus) WorkflowStepStatus {
	step := NewWorkflowStepStatus("assets")
	step.NextAction = "为必需资产选择参考图"
	step.Evidence["assetRevision"] = input.AssetRevision
	currentBreakdownID := ""
	if input.ConfirmedBreakdown != nil {
		currentBreakdownID = input.ConfirmedBreakdown.ID
	}
	requiredCount := 0
	readyCount := 0
	currentSourceCount := 0
	for _, asset := range input.Assets {
		if !asset.Required {
			continue
		}
		requiredCount++
		if asset.SourceBreakdownID == "" || asset.SourceBreakdownID == currentBreakdownID {
			currentSourceCount++
		}
		if asset.Selected && asset.Status == "ready" && publicWorkflowURL(asset.SelectedURL) {
			readyCount++
		} else {
			step.Blockers = append(step.Blockers, workflowBlocker(
				"asset_reference_missing", fmt.Sprintf("%s还没有可用的主参考图", workflowAssetLabel(asset)),
				"生成、上传或从资产库选择一张公网可访问的参考图。", asset.Kind, asset.ID,
			))
		}
	}
	step.Evidence["requiredCount"] = requiredCount
	step.Evidence["readyCount"] = readyCount
	step.Evidence["currentSourceCount"] = currentSourceCount
	if breakdown.Status != "completed" && breakdown.Status != "skipped_existing" {
		if len(input.Assets) > 0 {
			return staleByPreviousStep(step, "剧本拆解已变化，现有资产需要重新核对", "breakdown")
		}
		return blockedByPreviousStep(step, "请先完成剧本拆解", "breakdown")
	}
	if requiredCount == readyCount && requiredCount == currentSourceCount {
		step.Status = "completed"
		step.Progress = 100
		step.NextAction = "设计分镜"
		return step
	}
	step.Status = "stale"
	if requiredCount != currentSourceCount {
		step.Blockers = append(step.Blockers, workflowBlocker("asset_breakdown_stale", "现有资产来自旧的剧本拆解", "核对新拆解并准备对应参考图，旧资产仍会保留。", "assets", input.Project.ID))
	}
	if requiredCount > 0 {
		step.Progress = readyCount * 100 / requiredCount
	}
	return step
}

func computeStoryboardStep(input WorkflowStatusInput, assets WorkflowStepStatus) WorkflowStepStatus {
	step := NewWorkflowStepStatus("storyboard")
	step.NextAction = "让 AI 设计分镜"
	storyboard := input.ConfirmedStoryboard
	if storyboard != nil {
		step.Evidence["storyboardId"] = storyboard.ID
		step.Evidence["sourceScriptRevision"] = storyboard.SourceScriptRevision
		step.Evidence["sourceBreakdownId"] = storyboard.SourceBreakdownID
		step.Evidence["sourceAssetRevision"] = storyboard.SourceAssetRevision
		step.Evidence["capabilityVersion"] = storyboard.SourceCapabilityVersion
	}
	if input.Script.Content == "" && input.LegacyShotCount > 0 && storyboard == nil {
		step.Status = "skipped_existing"
		step.Progress = 100
		step.NextAction = "继续检查已有分镜提示词"
		step.Evidence["legacyShotCount"] = input.LegacyShotCount
		return step
	}
	if assets.Status != "completed" {
		if storyboard != nil {
			return staleByPreviousStep(step, "资产已变化，现有分镜需要重新确认", "assets")
		}
		return blockedByPreviousStep(step, "请先准备必需参考图", "assets")
	}
	currentBreakdownID := ""
	if input.ConfirmedBreakdown != nil {
		currentBreakdownID = input.ConfirmedBreakdown.ID
	}
	switch {
	case storyboard == nil:
		step.Status = "blocked"
		step.Blockers = []WorkflowMessage{workflowBlocker("storyboard_missing", "还没有确认分镜", "让 AI 设计镜头后，检查并确认。", "storyboard", input.Project.ID)}
	case storyboard.Status != "confirmed" ||
		storyboard.SourceScriptRevision != input.Script.ConfirmedRevision ||
		storyboard.SourceBreakdownID != currentBreakdownID ||
		storyboard.SourceAssetRevision != input.AssetRevision ||
		storyboard.SourceCapabilityVersion != input.Capabilities.CapabilityVersion:
		step.Status = "stale"
		step.Progress = 50
		step.NextAction = "基于当前资产重新设计分镜"
		step.Blockers = []WorkflowMessage{workflowBlocker("storyboard_stale", "分镜依赖的剧本、资产或模型能力已变化", "重新设计并确认分镜，旧分镜和视频仍会保留。", "storyboard", storyboard.ID)}
	case len(enabledWorkflowShots(input.Shots)) == 0:
		step.Status = "stale"
		step.Blockers = []WorkflowMessage{workflowBlocker("storyboard_empty", "没有启用的分镜", "至少启用一个镜头。", "storyboard", storyboard.ID)}
	case workflowShotValidationErrors(input.Shots) > 0:
		step.Status = "stale"
		step.Progress = 75
		step.Blockers = append(step.Blockers, collectWorkflowShotErrors(input.Shots)...)
	default:
		step.Status = "completed"
		step.Progress = 100
		step.NextAction = "检查 Seedance 2.0 提示词"
	}
	appendTargetDurationWarning(&step, input)
	return step
}

func computePromptStep(input WorkflowStatusInput, storyboard WorkflowStepStatus) WorkflowStepStatus {
	step := NewWorkflowStepStatus("prompt")
	step.NextAction = "逐镜检查提示词建议"
	step.Evidence["capabilityVersion"] = input.Capabilities.CapabilityVersion
	if storyboard.Status != "completed" && storyboard.Status != "skipped_existing" {
		if len(input.Shots) > 0 {
			return staleByPreviousStep(step, "分镜已变化，现有提示词需要重新检查", "storyboard")
		}
		return blockedByPreviousStep(step, "请先确认当前分镜", "storyboard")
	}
	shots := enabledWorkflowShots(input.Shots)
	validated := 0
	for _, shot := range shots {
		if shot.SavedDiagnosticsHash != "" && shot.SavedDiagnosticsHash == shot.CurrentDiagnosticsHash && len(shot.ValidationErrors) == 0 {
			validated++
			continue
		}
		step.Blockers = append(step.Blockers, workflowBlocker("prompt_stale", "有镜头的提示词还没有按当前素材检查", "打开镜头提示词，按建议修正并重新检查。", "shot", shot.ID))
	}
	step.Evidence["validatedCount"] = validated
	step.Evidence["enabledShotCount"] = len(shots)
	if len(shots) > 0 && validated == len(shots) {
		step.Status = "completed"
		step.Progress = 100
		step.NextAction = "生成并选择视频版本"
		return step
	}
	if len(shots) > 0 {
		step.Status = "pending"
		for _, shot := range shots {
			if shot.SavedDiagnosticsHash != "" {
				step.Status = "stale"
				break
			}
		}
		step.Progress = validated * 100 / len(shots)
	} else {
		step.Status = "blocked"
	}
	return step
}

func computeGenerateStep(input WorkflowStatusInput, prompt WorkflowStepStatus) WorkflowStepStatus {
	step := NewWorkflowStepStatus("generate")
	step.NextAction = "生成并选择每个镜头的视频版本"
	if prompt.Status != "completed" {
		if workflowHasSelections(input.Shots) {
			return staleByPreviousStep(step, "提示词已变化，现有选片需要重新核对", "prompt")
		}
		return blockedByPreviousStep(step, "请先完成提示词检查", "prompt")
	}
	shots := enabledWorkflowShots(input.Shots)
	selected := 0
	current := 0
	for _, shot := range shots {
		if shot.SelectedGenerationID == "" || !successfulWorkflowGenerationStatus(shot.SelectedGenerationStatus) {
			step.Blockers = append(step.Blockers, workflowBlocker("shot_selection_missing", "有镜头还没有选择成功的视频版本", "生成视频后，明确选择一个满意版本。", "shot", shot.ID))
			continue
		}
		selected++
		if shot.SelectedGenerationRequestHash == shot.CurrentRequestHash ||
			shot.SelectedGenerationAckHash == SelectionAckHash(shot.CurrentRequestHash, shot.SelectedGenerationID) {
			current++
			continue
		}
		step.Blockers = append(step.Blockers, workflowBlocker("shot_selection_stale", "已选视频来自旧提示词或素材", "重新生成，或明确确认继续使用这个旧版本。", "shot", shot.ID))
	}
	step.Evidence["selectedCount"] = selected
	step.Evidence["currentCount"] = current
	step.Evidence["enabledShotCount"] = len(shots)
	if len(shots) > 0 && current == len(shots) {
		step.Status = "completed"
		step.Progress = 100
		step.NextAction = "合成最终成片"
		return step
	}
	step.Status = "stale"
	if len(shots) > 0 {
		step.Progress = current * 100 / len(shots)
	}
	return step
}

func computeComposeStep(input WorkflowStatusInput, generate WorkflowStepStatus) WorkflowStepStatus {
	step := NewWorkflowStepStatus("compose")
	step.NextAction = "合成最终视频"
	step.Evidence["savedInputHash"] = input.Compose.SavedInputHash
	step.Evidence["currentInputHash"] = input.Compose.CurrentInputHash
	step.Evidence["videoUrl"] = input.Compose.VideoURL
	if generate.Status != "completed" {
		if strings.TrimSpace(input.Compose.VideoURL) != "" {
			step.Status = "stale"
			step.Progress = 50
			step.Blockers = []WorkflowMessage{workflowBlocker("compose_dependencies_stale", "镜头或选片已变化，旧成片仍可下载", "完成当前选片后重新合成。", "compose", input.Project.ID)}
			return step
		}
		return blockedByPreviousStep(step, "请先为每个镜头选择视频版本", "generate")
	}
	if input.Compose.Status == "completed" && publicWorkflowURL(input.Compose.VideoURL) &&
		input.Compose.SavedInputHash != "" && input.Compose.SavedInputHash == input.Compose.CurrentInputHash {
		step.Status = "completed"
		step.Progress = 100
		step.NextAction = "下载或分享成片"
		return step
	}
	if strings.TrimSpace(input.Compose.VideoURL) != "" || strings.TrimSpace(input.Compose.SavedInputHash) != "" {
		step.Status = "stale"
		step.Progress = 50
		step.Blockers = []WorkflowMessage{workflowBlocker("compose_stale", "当前成片对应的是旧镜头或旧设置", "重新合成即可，旧成片不会删除。", "compose", input.Project.ID)}
		return step
	}
	step.Status = "blocked"
	step.Blockers = []WorkflowMessage{workflowBlocker("compose_missing", "还没有合成成片", "确认转场、音乐和字幕设置后开始合成。", "compose", input.Project.ID)}
	return step
}

func blockedByPreviousStep(step WorkflowStepStatus, message, previousKey string) WorkflowStepStatus {
	step.Status = "blocked"
	step.Progress = 0
	step.Blockers = []WorkflowMessage{workflowBlocker("previous_step_incomplete", message, "返回上一步完成必要内容。", previousKey, "")}
	return step
}

func staleByPreviousStep(step WorkflowStepStatus, message, previousKey string) WorkflowStepStatus {
	step.Status = "stale"
	step.Progress = 50
	step.Blockers = []WorkflowMessage{workflowBlocker("previous_step_stale", message, "返回前一步确认最新内容；已有结果不会删除。", previousKey, "")}
	return step
}

func workflowBlocker(code, message, fix, targetType, targetID string) WorkflowMessage {
	return WorkflowMessage{Code: code, Message: message, Fix: fix, TargetType: targetType, TargetID: targetID, Details: map[string]any{}}
}

func workflowStatusComplete(status string) bool {
	return status == "completed" || status == "skipped_existing"
}

func breakdownHasPendingItems(breakdown BreakdownVersion) bool {
	for _, items := range [][]BreakdownItem{breakdown.Characters, breakdown.Scenes, breakdown.Props, breakdown.Outfits, breakdown.Styles} {
		for _, item := range items {
			if item.Decision != "confirmed" && item.Decision != "ignored" {
				return true
			}
		}
	}
	return false
}

func publicWorkflowURL(value string) bool {
	value = strings.TrimSpace(value)
	return strings.HasPrefix(value, "https://") || strings.HasPrefix(value, "http://")
}

func workflowAssetLabel(asset WorkflowAssetState) string {
	switch asset.Kind {
	case "character":
		return "人物"
	case "scene":
		return "场景"
	case "prop":
		return "物品"
	case "outfit":
		return "服饰"
	case "style":
		return "风格"
	default:
		return "资产"
	}
}

func enabledWorkflowShots(shots []WorkflowShotState) []WorkflowShotState {
	result := []WorkflowShotState{}
	for _, shot := range shots {
		if shot.Enabled {
			result = append(result, shot)
		}
	}
	return result
}

func workflowShotValidationErrors(shots []WorkflowShotState) int {
	count := 0
	for _, shot := range enabledWorkflowShots(shots) {
		count += len(shot.ValidationErrors)
	}
	return count
}

func collectWorkflowShotErrors(shots []WorkflowShotState) []WorkflowMessage {
	result := []WorkflowMessage{}
	for _, shot := range enabledWorkflowShots(shots) {
		for _, message := range shot.ValidationErrors {
			if message.TargetType == "" {
				message.TargetType = "shot"
			}
			if message.TargetID == "" {
				message.TargetID = shot.ID
			}
			if message.Details == nil {
				message.Details = map[string]any{}
			}
			result = append(result, message)
		}
	}
	return result
}

func appendTargetDurationWarning(step *WorkflowStepStatus, input WorkflowStatusInput) {
	if step == nil || input.TargetDuration <= 0 {
		return
	}
	actual := 0
	for _, shot := range enabledWorkflowShots(input.Shots) {
		actual += shot.Duration
	}
	if actual == input.TargetDuration {
		return
	}
	step.Warnings = append(step.Warnings, WorkflowMessage{
		Code:       "target_duration_variance",
		Message:    fmt.Sprintf("当前分镜总时长约 %d 秒，目标是 %d 秒", actual, input.TargetDuration),
		Fix:        "可以调整镜头数量或时长；这只是建议，不会阻止继续。",
		TargetType: "storyboard", TargetID: input.Project.ID,
		Details: map[string]any{"actualDuration": actual, "targetDuration": input.TargetDuration},
	})
}

func successfulWorkflowGenerationStatus(status string) bool {
	switch strings.ToLower(strings.TrimSpace(status)) {
	case "completed", "succeeded":
		return true
	default:
		return false
	}
}

func workflowHasSelections(shots []WorkflowShotState) bool {
	for _, shot := range shots {
		if shot.Enabled && strings.TrimSpace(shot.SelectedGenerationID) != "" {
			return true
		}
	}
	return false
}
