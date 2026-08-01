package videoproject

import (
	"context"
	"fmt"
	"strconv"
	"strings"

	"nine-xing/nx-backend/apps/server/internal/video"
)

// PromptBuilder 按 Seedance 2.0 规则把结构化分镜和规范素材编译为提示词，
// 同时保持素材编号、网关载荷顺序和新手诊断一致。
//
// 提示词结构公式：主体 + 动作 + 环境 + 风格 + 光照 + 镜头 + 质量词
// 参考素材优先级：上一镜头尾帧 > 角色标准照 > 场景参考图（图片最多 4 张，视频最多 2 个）
type PromptBuilder struct {
	loadShot       func(context.Context, string) (Shot, error)
	loadProject    func(context.Context, string) (Project, error)
	listCharacters func(context.Context, string) ([]Character, error)
	loadScene      func(context.Context, string) (Scene, error)
	previousShot   func(context.Context, string, int) (Shot, bool, error)
	capabilities   func(string) video.Capabilities
}

func NewPromptBuilder(store *Store, capabilityProviders ...func(string) video.Capabilities) *PromptBuilder {
	builder := &PromptBuilder{
		loadShot:       store.GetShot,
		loadProject:    store.GetProject,
		listCharacters: store.ListCharacters,
		loadScene:      store.getScene,
		previousShot:   store.PreviousShot,
		capabilities: func(model string) video.Capabilities {
			return video.ResolveCapabilities(video.CapabilityConfig{
				Model:           strings.TrimSpace(model),
				GatewayContract: video.LegacyFlatContract(),
			})
		},
	}
	if len(capabilityProviders) > 0 && capabilityProviders[0] != nil {
		builder.capabilities = capabilityProviders[0]
	}
	return builder
}

// ShotPreview 是生成前的完整预览：提示词、参考素材、校验结果与预估成功率。
type ShotPreview struct {
	Audios               []string           `json:"audios"`
	EstimatedSuccessRate int                `json:"estimatedSuccessRate"`
	Images               []string           `json:"images"`
	Prompt               string             `json:"prompt"`
	PromptVersion        string             `json:"promptVersion"`
	RequestHash          string             `json:"requestHash"`
	DiagnosticsHash      string             `json:"diagnosticsHash"`
	Diagnostics          []PromptDiagnostic `json:"diagnostics"`
	References           []video.Reference  `json:"references"`
	Validation           struct {
		Errors   []string `json:"errors"`
		IsValid  bool     `json:"isValid"`
		Warnings []string `json:"warnings"`
	} `json:"validation"`
	Videos []string `json:"videos"`
}

// BuildPreview 组装分镜的完整生成参数（不实际提交生成）。
func (b *PromptBuilder) BuildPreview(ctx context.Context, shotID string) (ShotPreview, error) {
	shot, err := b.loadShot(ctx, shotID)
	if err != nil {
		return ShotPreview{}, err
	}

	project, err := b.loadProject(ctx, shot.ProjectID)
	if err != nil {
		return ShotPreview{}, err
	}

	characters, err := b.resolveCharacters(ctx, shot)
	if err != nil {
		return ShotPreview{}, err
	}

	var scene *Scene
	if shot.SceneID != "" {
		sc, err := b.loadScene(ctx, shot.SceneID)
		if err == nil {
			scene = &sc
		}
	}

	var prevShot *Shot
	if prev, ok, err := b.previousShot(ctx, shot.ProjectID, shot.OrderNum); err == nil && ok {
		prevShot = &prev
	}

	capabilities := b.capabilities(shot.VideoModel)
	canonical, err := b.buildCanonicalReferences(shot, characters, scene, prevShot, capabilities)
	if err != nil {
		return ShotPreview{}, err
	}
	compiled := CompileSeedancePrompt(promptInputFromShot(shot, characters, scene, project.StyleGuide, canonical), capabilities)
	preview := ShotPreview{
		Audios:          []string{},
		Diagnostics:     []PromptDiagnostic{},
		Images:          []string{},
		Prompt:          compiled.Prompt,
		PromptVersion:   compiled.PromptVersion,
		RequestHash:     compiled.RequestHash,
		DiagnosticsHash: compiled.DiagnosticsHash,
		References:      []video.Reference{},
		Videos:          []string{},
	}
	preview.Diagnostics = append(preview.Diagnostics, compiled.Diagnostics...)
	preview.References = append(preview.References, compiled.OrderedReferences...)
	preview.Validation.Errors = []string{}
	preview.Validation.Warnings = []string{}
	preview.Images, preview.Videos, preview.Audios = promptPreviewURLs(canonical)
	for _, diagnostic := range compiled.Diagnostics {
		switch diagnostic.Level {
		case "error":
			preview.Validation.Errors = append(preview.Validation.Errors, diagnostic.Message)
		case "warning":
			preview.Validation.Warnings = append(preview.Validation.Warnings, diagnostic.Message)
		}
	}
	request, _, _, _, requestErr := buildShotGenerateRequest(shot, preview, capabilities, GenerateShotInput{
		CapabilityVersion: capabilities.CapabilityVersion,
	})
	if requestErr != nil {
		appendPromptDiagnostic(&preview, PromptDiagnostic{
			Level:   "error",
			Code:    "shot_generation_parameters_invalid",
			Message: requestErr.Error(),
			Fix:     "返回分镜设置，按提示修改视频生成参数。",
		})
	} else {
		for _, diagnostic := range validatePreviewGenerateRequest(request, capabilities) {
			appendPromptDiagnostic(&preview, diagnostic)
		}
	}
	preview.DiagnosticsHash = hashPromptValue(struct {
		RequestHash string             `json:"requestHash"`
		Diagnostics []PromptDiagnostic `json:"diagnostics"`
	}{RequestHash: preview.RequestHash, Diagnostics: preview.Diagnostics})
	preview.Validation.IsValid = len(preview.Validation.Errors) == 0

	preview.EstimatedSuccessRate = b.estimateSuccessRate(shot, characters, preview)
	return preview, nil
}

func appendPromptDiagnostic(preview *ShotPreview, diagnostic PromptDiagnostic) {
	for _, existing := range preview.Diagnostics {
		if existing.Code == diagnostic.Code {
			return
		}
	}
	preview.Diagnostics = append(preview.Diagnostics, diagnostic)
	switch diagnostic.Level {
	case "error":
		preview.Validation.Errors = append(preview.Validation.Errors, diagnostic.Message)
	case "warning":
		preview.Validation.Warnings = append(preview.Validation.Warnings, diagnostic.Message)
	}
}

func validatePreviewGenerateRequest(request video.GenerateRequest, capabilities video.Capabilities) []PromptDiagnostic {
	working := request
	diagnostics := make([]PromptDiagnostic, 0)
	for attempts := 0; attempts < len(request.References)+16; attempts++ {
		report, err := video.ValidateGenerateRequestWithWarnings(working, capabilities)
		for _, warning := range report.Warnings {
			diagnostics = appendUniquePromptDiagnostic(diagnostics, PromptDiagnostic{
				Level: "warning", Code: warning.Code, Message: warning.Message, Fix: warning.Fix,
			})
		}
		if err == nil {
			break
		}
		validationError, ok := err.(*video.ValidationError)
		if !ok {
			diagnostics = appendUniquePromptDiagnostic(diagnostics, PromptDiagnostic{
				Level: "error", Code: "generation_request_invalid", Message: err.Error(), Fix: "返回分镜设置检查生成参数。",
			})
			break
		}
		code := promptCodeForValidationError(validationError.Code)
		diagnostics = appendUniquePromptDiagnostic(diagnostics, PromptDiagnostic{
			Level: "error", Code: code, Message: validationError.Message, Fix: validationError.Fix,
		})
		if !repairPreviewValidationRequest(&working, capabilities, validationError) {
			break
		}
	}
	return diagnostics
}

func appendUniquePromptDiagnostic(diagnostics []PromptDiagnostic, diagnostic PromptDiagnostic) []PromptDiagnostic {
	for _, existing := range diagnostics {
		if existing.Code == diagnostic.Code {
			return diagnostics
		}
	}
	return append(diagnostics, diagnostic)
}

func promptCodeForValidationError(code string) string {
	switch code {
	case "task_mode_unsupported":
		return "unsupported_task_mode"
	case "edit_target_required":
		return "missing_edit_target"
	case "extend_target_required":
		return "missing_extend_target"
	default:
		return code
	}
}

func repairPreviewValidationRequest(request *video.GenerateRequest, capabilities video.Capabilities, validationError *video.ValidationError) bool {
	switch validationError.Code {
	case "capability_version_stale":
		request.CapabilityVersion = capabilities.CapabilityVersion
	case "model_mismatch":
		request.Model = capabilities.Model
	case "prompt_required":
		request.Prompt = "待完善的视频内容"
	case "seed_unsupported":
		request.Seed = nil
	case "camera_fixed_unsupported":
		request.CameraFixed = nil
	case "resolution_unsupported":
		request.Resolution = ""
	case "generate_audio_unsupported":
		request.GenerateAudio = nil
	case "duration_unsupported":
		if len(capabilities.SupportedDurations) == 0 {
			return false
		}
		request.Duration = capabilities.SupportedDurations[0]
	case "aspect_ratio_unsupported":
		if len(capabilities.AspectRatios) == 0 {
			return false
		}
		request.AspectRatio = capabilities.AspectRatios[0]
	case "task_mode_unsupported":
		if len(capabilities.TaskModes) == 0 {
			return false
		}
		request.TaskMode = capabilities.TaskModes[0]
	case "edit_target_required", "extend_target_required":
		request.TaskMode = "reference"
	case "mixed_target_roles":
		request.References = removePromptReferencesByRole(request.References, "extend_target")
	case "multiple_edit_targets":
		request.References = keepFirstPromptReferenceRole(request.References, "edit_target")
	case "multiple_extend_targets":
		request.References = keepFirstPromptReferenceRole(request.References, "extend_target")
	case "target_role_not_allowed":
		request.References = removePromptReferencesByRole(request.References, "edit_target", "extend_target")
	default:
		index, ok := promptReferenceIndex(validationError.Field)
		if !ok || index < 0 || index >= len(request.References) {
			return false
		}
		request.References = append(request.References[:index:index], request.References[index+1:]...)
	}
	return true
}

func promptReferenceIndex(field string) (int, bool) {
	start := strings.Index(field, "references[")
	if start == -1 {
		return 0, false
	}
	start += len("references[")
	end := strings.Index(field[start:], "]")
	if end == -1 {
		return 0, false
	}
	index, err := strconv.Atoi(field[start : start+end])
	return index, err == nil
}

func removePromptReferencesByRole(references []video.Reference, roles ...string) []video.Reference {
	blocked := make(map[string]struct{}, len(roles))
	for _, role := range roles {
		blocked[role] = struct{}{}
	}
	result := make([]video.Reference, 0, len(references))
	for _, reference := range references {
		if _, exists := blocked[reference.Role]; !exists {
			result = append(result, reference)
		}
	}
	return result
}

func keepFirstPromptReferenceRole(references []video.Reference, role string) []video.Reference {
	kept := false
	result := make([]video.Reference, 0, len(references))
	for _, reference := range references {
		if reference.Role != role {
			result = append(result, reference)
			continue
		}
		if !kept {
			result = append(result, reference)
			kept = true
		}
	}
	return result
}

func (b *PromptBuilder) resolveCharacters(ctx context.Context, shot Shot) ([]Character, error) {
	if len(shot.CharacterIDs) == 0 {
		return nil, nil
	}
	all, err := b.listCharacters(ctx, shot.ProjectID)
	if err != nil {
		return nil, err
	}
	wanted := make(map[string]bool, len(shot.CharacterIDs))
	for _, id := range shot.CharacterIDs {
		wanted[id] = true
	}
	selected := []Character{}
	for _, c := range all {
		if wanted[c.ID] {
			selected = append(selected, c)
		}
	}
	return selected, nil
}

func (b *PromptBuilder) buildCanonicalReferences(shot Shot, characters []Character, scene *Scene, prevShot *Shot, capabilities video.Capabilities) (video.CanonicalReferences, error) {
	references := make([]video.Reference, 0, len(shot.ShotAssets)+len(characters)+3)
	maxSortOrder := -1
	counts := map[string]int{"image": 0, "video": 0, "audio": 0}
	for index, asset := range shot.ShotAssets {
		role := strings.TrimSpace(asset.ReferenceRole)
		if role == "" {
			role = defaultShotReferenceRole(asset.AssetType)
		}
		id := strings.TrimSpace(asset.ID)
		if id == "" {
			id = fmt.Sprintf("shot-asset-%06d", index+1)
		}
		sourceType := strings.TrimSpace(asset.SourceType)
		if sourceType == "" {
			sourceType = "shot_asset"
		}
		sourceID := strings.TrimSpace(asset.SourceID)
		if sourceID == "" {
			sourceID = id
		}
		kind := strings.TrimSpace(asset.AssetType)
		references = append(references, video.Reference{
			ID:         id,
			Kind:       kind,
			Role:       role,
			URL:        strings.TrimSpace(asset.ObjectURL),
			SortOrder:  asset.SortOrder,
			SourceType: sourceType,
			SourceID:   sourceID,
			UsageNote:  strings.TrimSpace(asset.UsageNote),
		})
		counts[kind]++
		if asset.SortOrder > maxSortOrder {
			maxSortOrder = asset.SortOrder
		}
	}
	if len(shot.ShotAssets) > 0 {
		return video.CanonicalizeReferences(references)
	}

	nextSortOrder := maxSortOrder + 1
	addAutomatic := func(kind, role, rawURL, sourceType, sourceID, usageNote string) {
		url := strings.TrimSpace(rawURL)
		if url == "" || counts[kind] >= promptReferenceKindLimit(capabilities, kind) {
			return
		}
		references = append(references, video.Reference{
			ID:         fmt.Sprintf("auto-%s-%s", sourceType, sourceID),
			Kind:       kind,
			Role:       role,
			URL:        url,
			SortOrder:  nextSortOrder,
			SourceType: sourceType,
			SourceID:   sourceID,
			UsageNote:  usageNote,
		})
		nextSortOrder++
		counts[kind]++
	}

	modes := make(map[string]bool, len(shot.ImageReferenceModes))
	for _, mode := range shot.ImageReferenceModes {
		modes[mode] = true
	}
	if modes["prev_frame"] && prevShot != nil {
		role := "reference_image"
		if stringInPromptList(capabilities.ReferenceRoles, "first_frame") {
			role = "first_frame"
		}
		addAutomatic("image", role, prevShot.EndFrameURL, "previous_shot", stablePromptSourceID(prevShot.ID, "previous"), "承接上一镜头画面")
	}
	if modes["character_ref"] {
		for _, character := range orderedPromptCharacters(characters) {
			addAutomatic("image", "reference_image", character.ReferenceImageURL, "character", stablePromptSourceID(character.ID, character.Name), fmt.Sprintf("角色“%s”外观", character.Name))
		}
	}
	if modes["scene_ref"] && scene != nil {
		addAutomatic("image", "reference_image", scene.ReferenceImageURL, "scene", stablePromptSourceID(scene.ID, scene.Name), fmt.Sprintf("场景“%s”环境", scene.Name))
	}
	switch shot.VideoReferenceMode {
	case "prev_video":
		if prevShot != nil {
			addAutomatic("video", "reference_video", prevShot.VideoURL, "previous_shot", stablePromptSourceID(prevShot.ID, "previous"), "承接上一镜头动作与运镜")
		}
	case "scene_demo":
		if scene != nil {
			addAutomatic("video", "reference_video", scene.ReferenceVideoURL, "scene", stablePromptSourceID(scene.ID, scene.Name), fmt.Sprintf("场景“%s”动作与运镜", scene.Name))
		}
	}
	return video.CanonicalizeReferences(references)
}

func promptReferenceKindLimit(capabilities video.Capabilities, kind string) int {
	switch kind {
	case "image":
		return capabilities.Limits.MaxImages
	case "video":
		return capabilities.Limits.MaxVideos
	case "audio":
		return capabilities.Limits.MaxAudios
	default:
		return 0
	}
}

func stablePromptSourceID(id, fallback string) string {
	if id = strings.TrimSpace(id); id != "" {
		return id
	}
	return strings.TrimSpace(fallback)
}

func orderedPromptCharacters(characters []Character) []Character {
	ordered := make([]Character, 0, len(characters))
	for _, character := range characters {
		if character.IsMain {
			ordered = append(ordered, character)
		}
	}
	for _, character := range characters {
		if !character.IsMain {
			ordered = append(ordered, character)
		}
	}
	return ordered
}

func promptInputFromShot(shot Shot, characters []Character, scene *Scene, styleGuide string, references video.CanonicalReferences) PromptInput {
	mode := promptModeFromReferences(references)
	action := strings.TrimSpace(shot.DynamicDescription)
	if action == "" {
		action = strings.TrimSpace(shot.ActionDescription)
	}
	subjects := make([]string, 0, len(characters))
	for _, character := range characters {
		if name := strings.TrimSpace(character.Name); name != "" {
			subjects = append(subjects, name)
		}
	}
	sceneDescription := ""
	if scene != nil {
		sceneDescription = strings.TrimSpace(scene.Description)
		if sceneDescription == "" {
			sceneDescription = strings.TrimSpace(scene.Name)
		}
	}
	input := PromptInput{
		Mode:        mode,
		Subject:     strings.Join(subjects, "和"),
		Action:      action,
		Scene:       sceneDescription,
		Camera:      strings.TrimSpace(shot.CameraMovement),
		VisualStyle: strings.TrimSpace(styleGuide),
		References:  references,
	}
	switch mode {
	case "edit":
		input.Action = ""
		input.EditInstruction = action
	case "extend":
		input.Action = ""
		input.ExtendInstruction = action
	}
	return input
}

func promptModeFromReferences(references video.CanonicalReferences) string {
	for _, reference := range references.References {
		if reference.Role == "edit_target" {
			return "edit"
		}
	}
	for _, reference := range references.References {
		if reference.Role == "extend_target" {
			return "extend"
		}
	}
	return "reference"
}

func promptPreviewURLs(references video.CanonicalReferences) (images, videos, audios []string) {
	images = []string{}
	videos = []string{}
	audios = []string{}
	for _, reference := range references.References {
		switch reference.Kind {
		case "image":
			images = append(images, reference.URL)
		case "video":
			videos = append(videos, reference.URL)
		case "audio":
			audios = append(audios, reference.URL)
		}
	}
	return images, videos, audios
}

// buildReferenceImages 按优先级组装参考图片（最多 4 张）：
// 分镜显式参考图（用户上传/资产库选择） > 上一镜头尾帧（连贯性） > 角色标准照（人物一致性） > 场景参考图（环境约束）。
func (b *PromptBuilder) buildReferenceImages(shot Shot, characters []Character, scene *Scene, prevShot *Shot) []string {
	modes := make(map[string]bool, len(shot.ImageReferenceModes))
	for _, m := range shot.ImageReferenceModes {
		modes[m] = true
	}

	images := []string{}
	seen := map[string]bool{}
	add := func(url string) {
		url = strings.TrimSpace(url)
		if url == "" || seen[url] || len(images) >= 4 {
			return
		}
		seen[url] = true
		images = append(images, url)
	}

	// 优先级0：分镜级图片素材。用户显式上传/选择的分镜参考图必须参与生成，
	// 避免被自动角色/场景参考图挤出 4 张上限。
	for _, asset := range shot.ShotAssets {
		if asset.AssetType == "image" {
			add(asset.ObjectURL)
		}
	}

	// 优先级1：上一镜头尾帧。
	if modes["prev_frame"] && prevShot != nil {
		add(prevShot.EndFrameURL)
	}

	// 优先级2：角色标准照（主角优先）。
	if modes["character_ref"] {
		for _, char := range characters {
			if char.IsMain {
				add(char.ReferenceImageURL)
			}
		}
		for _, char := range characters {
			if !char.IsMain {
				add(char.ReferenceImageURL)
			}
		}
	}

	// 优先级3：场景参考图。
	if modes["scene_ref"] && scene != nil {
		add(scene.ReferenceImageURL)
	}

	return images
}

// buildReferenceVideos 组装参考视频（最多 2 个）：
// 分镜显式参考视频（用户上传/资产库选择） > 上一镜头视频/场景示例视频。
func (b *PromptBuilder) buildReferenceVideos(shot Shot, scene *Scene, prevShot *Shot) []string {
	videos := []string{}
	seen := map[string]bool{}
	add := func(url string) {
		url = strings.TrimSpace(url)
		if url == "" || seen[url] || len(videos) >= 2 {
			return
		}
		seen[url] = true
		videos = append(videos, url)
	}

	for _, asset := range shot.ShotAssets {
		if asset.AssetType == "video" {
			add(asset.ObjectURL)
		}
	}

	switch shot.VideoReferenceMode {
	case "prev_video":
		if prevShot != nil {
			add(prevShot.VideoURL)
		}
	case "scene_demo":
		if scene != nil {
			add(scene.ReferenceVideoURL)
		}
	}
	return videos
}

func (b *PromptBuilder) buildReferenceAudios(shot Shot) []string {
	audios := []string{}
	seen := map[string]bool{}
	for _, asset := range shot.ShotAssets {
		if asset.AssetType != "audio" {
			continue
		}
		url := strings.TrimSpace(asset.ObjectURL)
		if url == "" || seen[url] {
			continue
		}
		seen[url] = true
		audios = append(audios, url)
	}
	return audios
}

// estimateSuccessRate 预估成功率：基础 50%，参考素材逐项加成，封顶 95%。
func (b *PromptBuilder) estimateSuccessRate(shot Shot, characters []Character, preview ShotPreview) int {
	rate := 50

	// 上一帧继承 +20（连贯性最强的约束）。
	for _, m := range shot.ImageReferenceModes {
		if m == "prev_frame" && shot.OrderNum > 1 && len(preview.Images) > 0 {
			rate += 20
			break
		}
	}

	// 角色标准照 +15。
	hasCharRef := false
	for _, char := range characters {
		if strings.TrimSpace(char.ReferenceImageURL) != "" {
			hasCharRef = true
			break
		}
	}
	if hasCharRef {
		rate += 15
	}

	// 视频参考 +10。
	if len(preview.Videos) > 0 {
		rate += 10
	}

	// 角色有详细描述 +5。
	for _, char := range characters {
		if strings.TrimSpace(char.Description) != "" {
			rate += 5
			break
		}
	}

	if rate > 95 {
		rate = 95
	}
	return rate
}
