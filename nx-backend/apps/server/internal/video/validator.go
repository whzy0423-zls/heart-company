package video

import "fmt"

type ValidationError struct {
	Code               string        `json:"code"`
	Field              string        `json:"field"`
	Message            string        `json:"message"`
	Fix                string        `json:"fix"`
	LatestCapabilities *Capabilities `json:"latestCapabilities,omitempty"`
}

func (e *ValidationError) Error() string {
	if e == nil {
		return ""
	}
	if e.Message != "" {
		return e.Message
	}
	return e.Code
}

type ValidationWarning struct {
	Code    string `json:"code"`
	Field   string `json:"field"`
	Message string `json:"message"`
	Fix     string `json:"fix"`
}

type ValidationReport struct {
	Warnings []ValidationWarning `json:"warnings"`
}

func ValidateGenerateRequest(req GenerateRequest, caps Capabilities) error {
	_, err := ValidateGenerateRequestWithWarnings(req, caps)
	return err
}

func ValidateGenerateRequestWithWarnings(req GenerateRequest, caps Capabilities) (ValidationReport, error) {
	report := ValidationReport{Warnings: []ValidationWarning{}}

	if req.CapabilityVersion != caps.CapabilityVersion {
		latest := caps
		return report, validationError(
			"capability_version_stale",
			"capabilityVersion",
			"视频模型能力已更新，请刷新配置后重试。",
			"重新获取最新能力并使用新的 capabilityVersion 提交。",
			&latest,
		)
	}
	if req.Model != caps.Model {
		return report, validationError(
			"model_mismatch",
			"model",
			"请求模型与当前能力档案不一致。",
			"选择当前能力档案中的模型后重试。",
			nil,
		)
	}
	if req.Seed != nil && !caps.SupportsSeed {
		return report, unsupportedFieldError("seed_unsupported", "seed", "随机种子")
	}
	if req.CameraFixed != nil && !caps.SupportsCameraFixed {
		return report, unsupportedFieldError("camera_fixed_unsupported", "cameraFixed", "固定镜头")
	}
	if req.Resolution != "" && (!caps.SupportsResolution || !containsString(caps.Resolutions, req.Resolution)) {
		return report, unsupportedFieldError("resolution_unsupported", "resolution", "分辨率")
	}
	if req.GenerateAudio != nil && !caps.SupportsGenerateAudio {
		return report, unsupportedFieldError("generate_audio_unsupported", "generateAudio", "生成音频开关")
	}
	if !durationSupported(req.Duration, caps) {
		return report, validationError(
			"duration_unsupported",
			"duration",
			fmt.Sprintf("当前模型不支持 %d 秒时长。", req.Duration),
			"从最新能力的 supportedDurations 中选择时长。",
			nil,
		)
	}
	if !containsString(caps.AspectRatios, req.AspectRatio) {
		return report, validationError(
			"aspect_ratio_unsupported",
			"aspectRatio",
			fmt.Sprintf("当前模型不支持画幅 %q。", req.AspectRatio),
			"从最新能力的 aspectRatios 中选择画幅。",
			nil,
		)
	}
	if !knownTaskMode(req.TaskMode) || !containsString(caps.TaskModes, req.TaskMode) {
		return report, validationError(
			"task_mode_unsupported",
			"taskMode",
			fmt.Sprintf("当前模型不支持任务模式 %q。", req.TaskMode),
			"从最新能力的 taskModes 中选择任务模式。",
			nil,
		)
	}

	if err := validateTargetPredicates(req); err != nil {
		return report, err
	}

	imageCount := 0
	videoCount := 0
	audioCount := 0
	videoSeconds := 0.0
	audioSeconds := 0.0
	for index, reference := range req.References {
		if !containsString(caps.ReferenceRoles, reference.Role) {
			return report, validationError(
				"reference_role_unsupported",
				fmt.Sprintf("references[%d].role", index),
				fmt.Sprintf("当前模型不支持引用角色 %q。", reference.Role),
				"删除该引用，或改用最新能力 referenceRoles 中允许的角色。",
				nil,
			)
		}
		if !knownReferenceKind(reference.Kind) {
			return report, validationError(
				"reference_kind_unsupported",
				fmt.Sprintf("references[%d].kind", index),
				fmt.Sprintf("不支持引用素材类型 %q。", reference.Kind),
				"引用素材类型只能是 image、video 或 audio。",
				nil,
			)
		}
		if !roleMatchesKind(reference.Role, reference.Kind) {
			return report, validationError(
				"reference_kind_role_mismatch",
				fmt.Sprintf("references[%d]", index),
				"引用素材类型与用途不匹配。",
				"让图片、视频或音频使用与其类型匹配的引用角色。",
				nil,
			)
		}

		switch reference.Kind {
		case "image":
			imageCount++
		case "video":
			videoCount++
			if reference.DurationSeconds == nil {
				report.Warnings = append(report.Warnings, missingDurationWarning(index, reference.Kind))
			} else {
				videoSeconds += *reference.DurationSeconds
			}
		case "audio":
			audioCount++
			if reference.DurationSeconds == nil {
				report.Warnings = append(report.Warnings, missingDurationWarning(index, reference.Kind))
			} else {
				audioSeconds += *reference.DurationSeconds
			}
		}
	}

	if imageCount > caps.Limits.MaxImages {
		return report, limitError("max_images_exceeded", fmt.Sprintf("图片引用数量为 %d，超过上限 %d。", imageCount, caps.Limits.MaxImages), "减少图片引用数量。")
	}
	if videoCount > caps.Limits.MaxVideos {
		return report, limitError("max_videos_exceeded", fmt.Sprintf("视频引用数量为 %d，超过上限 %d。", videoCount, caps.Limits.MaxVideos), "减少视频引用数量。")
	}
	if audioCount > caps.Limits.MaxAudios {
		return report, limitError("max_audios_exceeded", fmt.Sprintf("音频引用数量为 %d，超过上限 %d。", audioCount, caps.Limits.MaxAudios), "减少音频引用数量。")
	}
	if videoSeconds > caps.Limits.MaxVideoSecondsTotal {
		return report, limitError("max_video_seconds_total_exceeded", fmt.Sprintf("已知视频总时长为 %.2f 秒，超过上限 %.2f 秒。", videoSeconds, caps.Limits.MaxVideoSecondsTotal), "缩短或删除视频引用。")
	}
	if audioSeconds > caps.Limits.MaxAudioSecondsTotal {
		return report, limitError("max_audio_seconds_total_exceeded", fmt.Sprintf("已知音频总时长为 %.2f 秒，超过上限 %.2f 秒。", audioSeconds, caps.Limits.MaxAudioSecondsTotal), "缩短或删除音频引用。")
	}

	return report, nil
}

func validateTargetPredicates(req GenerateRequest) error {
	editTargets := 0
	extendTargets := 0
	for _, reference := range req.References {
		switch reference.Role {
		case "edit_target":
			editTargets++
		case "extend_target":
			extendTargets++
		}
	}

	if editTargets > 0 && extendTargets > 0 {
		return validationError(
			"mixed_target_roles",
			"references",
			"一次请求不能同时包含编辑目标和延长目标。",
			"只保留 edit_target 或 extend_target 中的一种。",
			nil,
		)
	}

	switch req.TaskMode {
	case "reference":
		if editTargets > 0 || extendTargets > 0 {
			return validationError(
				"target_role_not_allowed",
				"references",
				"参考生成模式不能包含编辑或延长目标。",
				"删除目标角色，或切换到对应的 edit/extend 模式。",
				nil,
			)
		}
	case "edit":
		if editTargets == 0 {
			return validationError(
				"edit_target_required",
				"references",
				"编辑模式必须包含一个 edit_target。",
				"选择一个待编辑视频并标记为 edit_target。",
				nil,
			)
		}
		if editTargets > 1 {
			return validationError(
				"multiple_edit_targets",
				"references",
				"编辑模式只能包含一个 edit_target。",
				"只保留一个待编辑视频。",
				nil,
			)
		}
	case "extend":
		if extendTargets == 0 {
			return validationError(
				"extend_target_required",
				"references",
				"延长模式必须包含一个 extend_target。",
				"选择一个待延长视频并标记为 extend_target。",
				nil,
			)
		}
		if extendTargets > 1 {
			return validationError(
				"multiple_extend_targets",
				"references",
				"延长模式只能包含一个 extend_target。",
				"只保留一个待延长视频。",
				nil,
			)
		}
	}
	return nil
}

func validationError(code, field, message, fix string, latest *Capabilities) *ValidationError {
	return &ValidationError{
		Code:               code,
		Field:              field,
		Message:            message,
		Fix:                fix,
		LatestCapabilities: latest,
	}
}

func unsupportedFieldError(code, field, label string) *ValidationError {
	return validationError(
		code,
		field,
		fmt.Sprintf("当前模型或网关不支持%s参数。", label),
		fmt.Sprintf("移除%s参数，并按最新能力重新提交。", label),
		nil,
	)
}

func limitError(code, message, fix string) *ValidationError {
	return validationError(code, "references", message, fix, nil)
}

func missingDurationWarning(index int, kind string) ValidationWarning {
	return ValidationWarning{
		Code:    "media_duration_missing",
		Field:   fmt.Sprintf("references[%d].durationSeconds", index),
		Message: fmt.Sprintf("第 %d 个%s素材缺少时长，系统不会猜测其时长。", index+1, kind),
		Fix:     "读取素材元数据并补充 durationSeconds，才能完整校验总时长。",
	}
}

func durationSupported(duration int, caps Capabilities) bool {
	if duration == -1 && caps.SupportsSmartDuration {
		return true
	}
	return containsInt(caps.SupportedDurations, duration)
}

func knownTaskMode(mode string) bool {
	return mode == "reference" || mode == "edit" || mode == "extend"
}

func knownReferenceKind(kind string) bool {
	return kind == "image" || kind == "video" || kind == "audio"
}

func roleMatchesKind(role, kind string) bool {
	switch role {
	case "reference_image", "first_frame", "last_frame":
		return kind == "image"
	case "reference_video", "edit_target", "extend_target":
		return kind == "video"
	case "reference_audio":
		return kind == "audio"
	default:
		return false
	}
}
