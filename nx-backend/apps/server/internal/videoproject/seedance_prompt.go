package videoproject

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"unicode/utf8"

	"nine-xing/nx-backend/apps/server/internal/video"
)

const SeedancePromptVersion = "seedance2_v2"

type PromptDiagnostic struct {
	Level   string `json:"level"`
	Code    string `json:"code"`
	Message string `json:"message"`
	Fix     string `json:"fix"`
}

type CompiledPrompt struct {
	Prompt            string             `json:"prompt"`
	PromptVersion     string             `json:"promptVersion"`
	RequestHash       string             `json:"requestHash"`
	DiagnosticsHash   string             `json:"diagnosticsHash"`
	Diagnostics       []PromptDiagnostic `json:"diagnostics"`
	OrderedReferences []video.Reference  `json:"orderedReferences"`
}

type PromptInput struct {
	Mode                  string                    `json:"mode"`
	Subject               string                    `json:"subject"`
	Action                string                    `json:"action"`
	Scene                 string                    `json:"scene"`
	Camera                string                    `json:"camera"`
	Composition           string                    `json:"composition"`
	Lighting              string                    `json:"lighting"`
	VisualStyle           string                    `json:"visualStyle"`
	Quality               string                    `json:"quality"`
	Dialogue              string                    `json:"dialogue"`
	SoundEffect           string                    `json:"soundEffect"`
	Music                 string                    `json:"music"`
	Subtitle              string                    `json:"subtitle"`
	AdditionalInstruction string                    `json:"additionalInstruction"`
	EditInstruction       string                    `json:"editInstruction"`
	ExtendInstruction     string                    `json:"extendInstruction"`
	References            video.CanonicalReferences `json:"references"`
}

var (
	promptReferencePattern = regexp.MustCompile(`(图片|视频|音频)([0-9]+)`)
	exactTimeRangePattern  = regexp.MustCompile(`[0-9]+(?:\.[0-9]+)?\s*(?:-|–|—|~|～|至|到)\s*[0-9]+(?:\.[0-9]+)?\s*秒`)
)

func CompileSeedancePrompt(input PromptInput, capabilities video.Capabilities) CompiledPrompt {
	input = normalizePromptInput(input)
	orderedReferences := copyPromptReferences(input.References)
	diagnostics := promptCapabilityDiagnostics(input, capabilities)
	diagnostics = append(diagnostics, promptTargetDiagnostics(input)...)

	prompt := compileSeedancePromptText(input)
	diagnostics = append(diagnostics, promptTextDiagnostics(input, prompt)...)

	requestHash := hashPromptValue(struct {
		Version           string            `json:"version"`
		CapabilityVersion string            `json:"capabilityVersion"`
		Mode              string            `json:"mode"`
		Prompt            string            `json:"prompt"`
		References        []video.Reference `json:"references"`
	}{
		Version:           SeedancePromptVersion,
		CapabilityVersion: capabilities.CapabilityVersion,
		Mode:              input.Mode,
		Prompt:            prompt,
		References:        orderedReferences,
	})
	diagnosticsHash := hashPromptValue(struct {
		RequestHash string             `json:"requestHash"`
		Diagnostics []PromptDiagnostic `json:"diagnostics"`
	}{RequestHash: requestHash, Diagnostics: diagnostics})

	return CompiledPrompt{
		Prompt:            prompt,
		PromptVersion:     SeedancePromptVersion,
		RequestHash:       requestHash,
		DiagnosticsHash:   diagnosticsHash,
		Diagnostics:       diagnostics,
		OrderedReferences: orderedReferences,
	}
}

func normalizePromptInput(input PromptInput) PromptInput {
	input.Mode = strings.ToLower(strings.TrimSpace(input.Mode))
	if input.Mode == "" {
		input.Mode = "reference"
	}
	for _, field := range []*string{
		&input.Subject,
		&input.Action,
		&input.Scene,
		&input.Camera,
		&input.Composition,
		&input.Lighting,
		&input.VisualStyle,
		&input.Quality,
		&input.Dialogue,
		&input.SoundEffect,
		&input.Music,
		&input.Subtitle,
		&input.AdditionalInstruction,
		&input.EditInstruction,
		&input.ExtendInstruction,
	} {
		*field = strings.TrimSpace(*field)
	}
	return input
}

func compileSeedancePromptText(input PromptInput) string {
	var sections []string
	switch input.Mode {
	case "edit":
		target := targetReference(input.References, "edit_target")
		label := "目标视频"
		if target != nil {
			label = target.Label
		}
		instruction := strings.TrimSpace(input.EditInstruction)
		if instruction == "" {
			instruction = "按镜头要求修改目标内容"
		}
		sections = append(sections, fmt.Sprintf("严格编辑%s，%s，其余内容保持不变。", label, trimSentenceEnd(instruction)))
	case "extend":
		target := targetReference(input.References, "extend_target")
		label := "目标视频"
		if target != nil {
			label = target.Label
		}
		instruction := strings.TrimSpace(input.ExtendInstruction)
		if instruction == "" {
			instruction = "画面自然延续"
		}
		sections = append(sections, fmt.Sprintf("向后延长%s，生成%s。", label, trimSentenceEnd(instruction)))
	default:
		if guidance := compileReferenceGuidance(input); guidance != "" {
			sections = append(sections, guidance+"。")
		}
		if body := compileReferenceBody(input); body != "" {
			sections = append(sections, body+"。")
		}
	}
	sections = append(sections, compileDefaultConstraints(input.Subtitle))
	return strings.Join(sections, "")
}

func compileReferenceGuidance(input PromptInput) string {
	groups := map[string][]string{"image": {}, "video": {}, "audio": {}}
	for _, reference := range input.References.References {
		switch reference.Role {
		case "reference_image":
			groups["image"] = append(groups["image"], imageReferenceGuidance(reference))
		case "first_frame":
			groups["image"] = append(groups["image"], fmt.Sprintf("以%s作为首帧", reference.Label))
		case "last_frame":
			groups["image"] = append(groups["image"], fmt.Sprintf("以%s作为尾帧", reference.Label))
		case "reference_video":
			guidance := fmt.Sprintf("参考%s的动作和运镜", reference.Label)
			if input.Camera != "" {
				guidance = fmt.Sprintf("参考%s的%s运镜", reference.Label, input.Camera)
			}
			groups["video"] = append(groups["video"], guidance)
		case "reference_audio":
			groups["audio"] = append(groups["audio"], fmt.Sprintf("参考%s的音色", reference.Label))
		}
	}
	parts := make([]string, 0, len(input.References.References))
	for _, kind := range []string{"image", "video", "audio"} {
		parts = append(parts, groups[kind]...)
	}
	return strings.Join(parts, "，")
}

func imageReferenceGuidance(reference video.CanonicalReference) string {
	sourceType := strings.ToLower(strings.TrimSpace(reference.SourceType))
	sourceID := strings.TrimSpace(reference.SourceID)
	switch sourceType {
	case "character", "person", "role":
		if sourceID != "" {
			return fmt.Sprintf("参考%s中的角色“%s”外观", reference.Label, sourceID)
		}
		return fmt.Sprintf("参考%s中的角色外观", reference.Label)
	case "scene", "location":
		if sourceID != "" {
			return fmt.Sprintf("参考%s中的%s", reference.Label, sourceID)
		}
		return fmt.Sprintf("参考%s中的场景", reference.Label)
	case "outfit", "costume":
		if sourceID != "" {
			return fmt.Sprintf("参考%s中的%s服饰", reference.Label, sourceID)
		}
		return fmt.Sprintf("参考%s中的服饰", reference.Label)
	case "prop", "item":
		if sourceID != "" {
			return fmt.Sprintf("参考%s中的道具“%s”", reference.Label, sourceID)
		}
		return fmt.Sprintf("参考%s中的道具", reference.Label)
	case "style":
		return fmt.Sprintf("参考%s的视觉风格", reference.Label)
	default:
		return fmt.Sprintf("参考%s的主体与画面", reference.Label)
	}
}

func compileReferenceBody(input PromptInput) string {
	parts := make([]string, 0, 12)
	if visual := composeVisualAction(input.Subject, input.Scene, input.Action); visual != "" {
		parts = append(parts, visual)
	}
	if input.Camera != "" {
		parts = append(parts, "镜头"+input.Camera)
	}
	parts = appendNonEmpty(parts, input.Composition, input.Lighting, input.VisualStyle, input.Quality)
	if input.Dialogue != "" {
		parts = append(parts, wrapPromptNotation(input.Dialogue, "{", "}"))
	}
	if input.SoundEffect != "" {
		parts = append(parts, wrapPromptNotation(input.SoundEffect, "<", ">"))
	}
	if input.Music != "" {
		parts = append(parts, wrapPromptNotation(input.Music, "（", "）"))
	}
	if input.Subtitle != "" {
		parts = append(parts, wrapPromptNotation(input.Subtitle, "【", "】"))
	}
	parts = appendNonEmpty(parts, input.AdditionalInstruction)
	return strings.Join(parts, "，")
}

func composeVisualAction(subject, scene, action string) string {
	action = trimSentenceEnd(action)
	if scene != "" && strings.HasPrefix(action, "在") {
		action = stripLeadingActionLocation(action)
	}
	switch {
	case subject != "" && scene != "":
		return subject + "在" + scene + action
	case subject != "":
		return subject + action
	case scene != "":
		if action == "" {
			return scene
		}
		return "在" + scene + action
	default:
		return action
	}
}

func stripLeadingActionLocation(action string) string {
	bestIndex := earliestPromptKeyword(action, []string{"快步", "缓步", "慢慢", "迅速", "突然", "轻轻", "悄悄", "继续", "奔跑", "回头", "转身", "抬头", "低头", "挥手", "站起", "坐下", "停下"})
	if bestIndex == -1 {
		bestIndex = earliestPromptKeyword(action, []string{"走", "跑", "坐", "说", "拿", "推", "拉", "跳", "停"})
	}
	if bestIndex == -1 {
		return action
	}
	return action[bestIndex:]
}

func earliestPromptKeyword(value string, keywords []string) int {
	bestIndex := -1
	for _, keyword := range keywords {
		index := strings.Index(value, keyword)
		if index > 0 && (bestIndex == -1 || index < bestIndex) {
			bestIndex = index
		}
	}
	return bestIndex
}

func compileDefaultConstraints(subtitle string) string {
	constraints := make([]string, 0, 3)
	if strings.TrimSpace(subtitle) == "" {
		constraints = append(constraints, "保持无字幕")
	}
	constraints = append(constraints, "不要生成 Logo", "不要生成水印")
	return strings.Join(constraints, "、") + "。"
}

func promptCapabilityDiagnostics(input PromptInput, capabilities video.Capabilities) []PromptDiagnostic {
	diagnostics := make([]PromptDiagnostic, 0)
	if !stringInPromptList(capabilities.TaskModes, input.Mode) {
		diagnostics = append(diagnostics, PromptDiagnostic{
			Level: "error", Code: "unsupported_task_mode",
			Message: fmt.Sprintf("当前视频模型不能执行“%s”任务。", promptModeLabel(input.Mode)),
			Fix:     "切换到模型支持的任务方式，或返回分镜设置选择其他模型。",
		})
	}
	seenRoles := make(map[string]struct{})
	for _, reference := range input.References.References {
		if stringInPromptList(capabilities.ReferenceRoles, reference.Role) {
			continue
		}
		if _, duplicate := seenRoles[reference.Role]; duplicate {
			continue
		}
		seenRoles[reference.Role] = struct{}{}
		diagnostics = append(diagnostics, PromptDiagnostic{
			Level: "error", Code: "unsupported_reference_role",
			Message: fmt.Sprintf("当前模型不能把%s用作“%s”。", reference.Label, promptRoleLabel(reference.Role)),
			Fix:     "更换该素材的用途，或选择支持这种素材用途的视频模型。",
		})
	}
	return diagnostics
}

func promptTargetDiagnostics(input PromptInput) []PromptDiagnostic {
	editTargets := referencesWithRole(input.References, "edit_target")
	extendTargets := referencesWithRole(input.References, "extend_target")
	if len(editTargets) > 0 && len(extendTargets) > 0 {
		return []PromptDiagnostic{{
			Level: "error", Code: "mixed_target_roles",
			Message: "同一个镜头不能同时编辑视频和延长视频。",
			Fix:     "只保留一个编辑目标或一个延长目标，再重新检查提示词。",
		}}
	}
	switch input.Mode {
	case "edit":
		switch len(editTargets) {
		case 0:
			return []PromptDiagnostic{{
				Level: "error", Code: "missing_edit_target",
				Message: "视频编辑需要先选择一个要修改的视频。",
				Fix:     "添加一个视频素材，并把用途设置为“编辑目标”。",
			}}
		case 1:
			return nil
		default:
			return []PromptDiagnostic{{
				Level: "error", Code: "multiple_edit_targets",
				Message: fmt.Sprintf("当前选择了 %d 个编辑目标，但一次只能编辑一个视频。", len(editTargets)),
				Fix:     "只保留一个“编辑目标”，其他视频改为普通参考或移除。",
			}}
		}
	case "extend":
		switch len(extendTargets) {
		case 0:
			return []PromptDiagnostic{{
				Level: "error", Code: "missing_extend_target",
				Message: "视频延长需要先选择一个要续写的视频。",
				Fix:     "添加一个视频素材，并把用途设置为“延长目标”。",
			}}
		case 1:
			return nil
		default:
			return []PromptDiagnostic{{
				Level: "error", Code: "multiple_extend_targets",
				Message: fmt.Sprintf("当前选择了 %d 个延长目标，但一次只能延长一个视频。", len(extendTargets)),
				Fix:     "只保留一个“延长目标”，其他视频改为普通参考或移除。",
			}}
		}
	}
	return nil
}

func promptTextDiagnostics(input PromptInput, prompt string) []PromptDiagnostic {
	diagnostics := make([]PromptDiagnostic, 0)
	if missing := missingPromptReferenceLabels(prompt, input.References); len(missing) > 0 {
		diagnostics = append(diagnostics, PromptDiagnostic{
			Level: "error", Code: "reference_number_missing",
			Message: fmt.Sprintf("提示词提到了不存在的素材编号：%s。", strings.Join(missing, "、")),
			Fix:     "从素材列表中重新选择编号，或删除这段素材指代。",
		})
	}
	if hasMultipleCameraMovements(input.Camera) {
		diagnostics = append(diagnostics, PromptDiagnostic{
			Level: "warning", Code: "multiple_camera_movements",
			Message: "这个镜头同时安排了多种主要运镜，画面可能不够稳定。",
			Fix:     "优先保留一种主要运镜，其余变化拆到下一个镜头。",
		})
	}
	if exactTimeRangePattern.MatchString(prompt) {
		diagnostics = append(diagnostics, PromptDiagnostic{
			Level: "warning", Code: "exact_time_segments_unstable",
			Message: "提示词使用了精确秒数分段，视频模型可能无法严格按秒执行。",
			Fix:     "改用“镜头1、镜头2、镜头3”描述先后顺序。",
		})
	}
	if utf8.RuneCountInString(prompt) > 500 {
		diagnostics = append(diagnostics, PromptDiagnostic{
			Level: "warning", Code: "prompt_over_500_chinese_chars",
			Message: "提示词超过约 500 个字符，重点可能被稀释。",
			Fix:     "保留主体、动作、场景、一个主要运镜和关键声音，删除重复描述。",
		})
	}
	return diagnostics
}

func missingPromptReferenceLabels(prompt string, references video.CanonicalReferences) []string {
	available := make(map[string]struct{}, len(references.References))
	for _, reference := range references.References {
		available[reference.Label] = struct{}{}
	}
	missingSet := make(map[string]struct{})
	for _, match := range promptReferencePattern.FindAllStringSubmatch(prompt, -1) {
		label := match[1] + match[2]
		if _, exists := available[label]; !exists {
			missingSet[label] = struct{}{}
		}
	}
	missing := make([]string, 0, len(missingSet))
	for label := range missingSet {
		missing = append(missing, label)
	}
	sort.Slice(missing, func(left, right int) bool {
		leftKind, leftNumber := splitPromptReferenceLabel(missing[left])
		rightKind, rightNumber := splitPromptReferenceLabel(missing[right])
		if leftKind != rightKind {
			return leftKind < rightKind
		}
		return leftNumber < rightNumber
	})
	return missing
}

func splitPromptReferenceLabel(label string) (int, int) {
	kindOrder := map[string]int{"图片": 0, "视频": 1, "音频": 2}
	for kind, order := range kindOrder {
		if strings.HasPrefix(label, kind) {
			number, _ := strconv.Atoi(strings.TrimPrefix(label, kind))
			return order, number
		}
	}
	return len(kindOrder), 0
}

func hasMultipleCameraMovements(camera string) bool {
	camera = strings.ToLower(strings.TrimSpace(camera))
	if camera == "" {
		return false
	}
	movements := []string{"推镜", "拉镜", "跟拍", "环绕", "摇镜", "移镜", "升降", "俯冲", "旋转", "固定镜头", "手持", "航拍", "zoom", "dolly", "pan", "tilt", "orbit", "tracking"}
	count := 0
	for _, movement := range movements {
		if strings.Contains(camera, movement) {
			count++
		}
	}
	return count > 1
}

func referencesWithRole(references video.CanonicalReferences, role string) []video.CanonicalReference {
	result := make([]video.CanonicalReference, 0)
	for _, reference := range references.References {
		if reference.Role == role {
			result = append(result, reference)
		}
	}
	return result
}

func targetReference(references video.CanonicalReferences, role string) *video.CanonicalReference {
	matches := referencesWithRole(references, role)
	if len(matches) != 1 {
		return nil
	}
	match := matches[0]
	return &match
}

func copyPromptReferences(references video.CanonicalReferences) []video.Reference {
	result := make([]video.Reference, 0, len(references.References))
	for _, canonical := range references.References {
		reference := canonical.Reference
		if reference.DurationSeconds != nil {
			duration := *reference.DurationSeconds
			reference.DurationSeconds = &duration
		}
		result = append(result, reference)
	}
	return result
}

func appendNonEmpty(parts []string, values ...string) []string {
	for _, value := range values {
		if value = strings.TrimSpace(value); value != "" {
			parts = append(parts, trimSentenceEnd(value))
		}
	}
	return parts
}

func wrapPromptNotation(value, left, right string) string {
	value = strings.TrimSpace(value)
	if strings.Contains(value, left) && strings.Contains(value, right) {
		return value
	}
	return left + trimSentenceEnd(value) + right
}

func trimSentenceEnd(value string) string {
	return strings.TrimRight(strings.TrimSpace(value), "。！？!?；;，,")
}

func stringInPromptList(values []string, wanted string) bool {
	for _, value := range values {
		if value == wanted {
			return true
		}
	}
	return false
}

func promptModeLabel(mode string) string {
	switch mode {
	case "reference":
		return "参考生成"
	case "edit":
		return "视频编辑"
	case "extend":
		return "视频延长"
	default:
		return mode
	}
}

func promptRoleLabel(role string) string {
	switch role {
	case "reference_image":
		return "图片参考"
	case "first_frame":
		return "首帧"
	case "last_frame":
		return "尾帧"
	case "reference_video":
		return "视频参考"
	case "reference_audio":
		return "音频参考"
	case "edit_target":
		return "编辑目标"
	case "extend_target":
		return "延长目标"
	default:
		return role
	}
}

func hashPromptValue(value any) string {
	encoded, err := json.Marshal(value)
	if err != nil {
		return ""
	}
	sum := sha256.Sum256(encoded)
	return hex.EncodeToString(sum[:])
}
