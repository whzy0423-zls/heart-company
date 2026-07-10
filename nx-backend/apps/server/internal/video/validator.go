package video

import (
	"fmt"
	"math"
	"net"
	"net/url"
	"path"
	"strings"

	"nine-xing/nx-backend/apps/server/internal/netguard"
)

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
	if strings.TrimSpace(req.Prompt) == "" {
		return report, validationError(
			"prompt_required",
			"prompt",
			"视频提示词不能为空。",
			"填写希望生成的视频内容后重试。",
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
	if len(req.References) > 5 {
		report.Warnings = append(report.Warnings, ValidationWarning{
			Code:    "reference_count_above_recommended",
			Field:   "references",
			Message: fmt.Sprintf("当前选择了 %d 个引用素材，超过建议数量 5。", len(req.References)),
			Fix:     "优先保留最关键的 5 个素材，减少引用冲突。",
		})
	}
	for index, reference := range req.References {
		if !knownReferenceKind(reference.Kind) {
			return report, validationError(
				"reference_kind_unsupported",
				fmt.Sprintf("references[%d].kind", index),
				fmt.Sprintf("不支持引用素材类型 %q。", reference.Kind),
				"引用素材类型只能是 image、video 或 audio。",
				nil,
			)
		}
		if !containsString(caps.ReferenceRoles, reference.Role) {
			return report, validationError(
				"reference_role_unsupported",
				fmt.Sprintf("references[%d].role", index),
				fmt.Sprintf("当前模型不支持引用角色 %q。", reference.Role),
				"删除该引用，或改用最新能力 referenceRoles 中允许的角色。",
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
		if err := validateReferenceURL(index, reference.URL); err != nil {
			return report, err
		}
		if reference.DurationSeconds != nil {
			if _, ok := durationNanoseconds(*reference.DurationSeconds); !ok {
				return report, validationError(
					"media_duration_invalid",
					fmt.Sprintf("references[%d].durationSeconds", index),
					"素材时长必须是大于 0 且可安全表示的有限秒数。",
					"重新读取素材元数据并填写有效 durationSeconds。",
					nil,
				)
			}
		}
	}

	if err := validateTargetPredicates(req); err != nil {
		return report, err
	}

	imageCount := 0
	videoCount := 0
	audioCount := 0
	videoNanoseconds := int64(0)
	audioNanoseconds := int64(0)
	for index, reference := range req.References {
		switch reference.Kind {
		case "image":
			imageCount++
		case "video":
			videoCount++
			if reference.DurationSeconds == nil {
				report.Warnings = append(report.Warnings, missingDurationWarning(index, reference.Kind))
			} else {
				nanoseconds, _ := durationNanoseconds(*reference.DurationSeconds)
				videoNanoseconds = saturatingAddNanoseconds(videoNanoseconds, nanoseconds)
			}
		case "audio":
			audioCount++
			if reference.DurationSeconds == nil {
				report.Warnings = append(report.Warnings, missingDurationWarning(index, reference.Kind))
			} else {
				nanoseconds, _ := durationNanoseconds(*reference.DurationSeconds)
				audioNanoseconds = saturatingAddNanoseconds(audioNanoseconds, nanoseconds)
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
	if videoNanoseconds > limitNanoseconds(caps.Limits.MaxVideoSecondsTotal) {
		return report, limitError("max_video_seconds_total_exceeded", fmt.Sprintf("已知视频总时长为 %.9f 秒，超过上限 %.2f 秒。", float64(videoNanoseconds)/nanosecondsPerSecond, caps.Limits.MaxVideoSecondsTotal), "缩短或删除视频引用。")
	}
	if audioNanoseconds > limitNanoseconds(caps.Limits.MaxAudioSecondsTotal) {
		return report, limitError("max_audio_seconds_total_exceeded", fmt.Sprintf("已知音频总时长为 %.9f 秒，超过上限 %.2f 秒。", float64(audioNanoseconds)/nanosecondsPerSecond, caps.Limits.MaxAudioSecondsTotal), "缩短或删除音频引用。")
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

const nanosecondsPerSecond = 1_000_000_000

func durationNanoseconds(seconds float64) (int64, bool) {
	const maxInt64 = int64(^uint64(0) >> 1)
	maxNanosecondsFloat := math.Nextafter(float64(maxInt64), 0)
	maxSeconds := maxNanosecondsFloat / nanosecondsPerSecond
	if math.IsNaN(seconds) || math.IsInf(seconds, 0) || seconds <= 0 || seconds > maxSeconds {
		return 0, false
	}
	nanoseconds := math.Round(seconds * nanosecondsPerSecond)
	if nanoseconds > maxNanosecondsFloat {
		return 0, false
	}
	if nanoseconds < 1 {
		nanoseconds = 1
	}
	result := int64(nanoseconds)
	if result <= 0 {
		return 0, false
	}
	return result, true
}

func limitNanoseconds(seconds float64) int64 {
	nanoseconds, ok := durationNanoseconds(seconds)
	if !ok {
		return 0
	}
	return nanoseconds
}

func saturatingAddNanoseconds(total, value int64) int64 {
	const maxInt64 = int64(^uint64(0) >> 1)
	if value > maxInt64-total {
		return maxInt64
	}
	return total + value
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

func validateReferenceURL(index int, raw string) error {
	raw = strings.TrimSpace(raw)
	field := fmt.Sprintf("references[%d].url", index)
	if raw == "" {
		return validationError(
			"reference_url_required",
			field,
			"引用素材地址不能为空。",
			"重新上传或选择素材，并补充可访问地址。",
			nil,
		)
	}
	if isDocumentedTemporaryAssetPath(raw) || isSafePublicReferenceURL(raw) {
		return nil
	}
	return validationError(
		"reference_url_invalid",
		field,
		"引用素材地址必须是安全的公网 http(s) 地址，或站内 /pg/assets/ 临时素材路径。",
		"移除本地、私网、带凭据或路径穿越地址，并重新选择素材。",
		nil,
	)
}

func isDocumentedTemporaryAssetPath(raw string) bool {
	parsed, err := url.ParseRequestURI(raw)
	if err != nil || parsed.IsAbs() || parsed.Host != "" || parsed.User != nil || parsed.Fragment != "" {
		return false
	}
	if !strings.HasPrefix(parsed.Path, "/pg/assets/") || strings.Contains(parsed.Path, `\`) {
		return false
	}
	if path.Clean(parsed.Path) != parsed.Path {
		return false
	}
	return strings.TrimPrefix(parsed.Path, "/pg/assets/") != ""
}

func isSafePublicReferenceURL(raw string) bool {
	parsed, err := url.ParseRequestURI(raw)
	if err != nil || parsed.User != nil || parsed.Fragment != "" || !netguard.IsPublicHTTPURL(raw) {
		return false
	}
	if path.Clean(parsed.Path) != parsed.Path || strings.Contains(parsed.Path, `\`) {
		return false
	}
	host := strings.TrimSuffix(strings.ToLower(strings.TrimSpace(parsed.Hostname())), ".")
	if host == "" || strings.HasSuffix(host, ".local") {
		return false
	}
	if ip := net.ParseIP(host); ip != nil {
		return netguard.IsPublicIP(ip)
	}
	return isValidPublicHostname(host)
}

func isValidPublicHostname(host string) bool {
	if len(host) > 253 || !strings.Contains(host, ".") {
		return false
	}
	labels := strings.Split(host, ".")
	for _, label := range labels {
		if len(label) == 0 || len(label) > 63 || label[0] == '-' || label[len(label)-1] == '-' {
			return false
		}
		for _, char := range label {
			if (char < 'a' || char > 'z') && (char < '0' || char > '9') && char != '-' {
				return false
			}
		}
	}
	topLevelHasLetter := false
	for _, char := range labels[len(labels)-1] {
		if char >= 'a' && char <= 'z' {
			topLevelHasLetter = true
			break
		}
	}
	if !topLevelHasLetter {
		return false
	}
	return true
}
