package video

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"strconv"
	"strings"

	"nine-xing/nx-backend/apps/server/internal/config"
)

const officialCapabilityRegistryVersion = "seedance2_official_profiles_2026-07-10.v1"

type CapabilityConfig struct {
	Model           string
	ModelProfile    string
	GatewayContract config.GatewayContractConfig
}

type CapabilitySource struct {
	OfficialProfile        string `json:"officialProfile"`
	OfficialProfileVersion string `json:"officialProfileVersion"`
	Selection              string `json:"selection"`
	GatewayContract        string `json:"gatewayContract"`
	GatewayContractVersion string `json:"gatewayContractVersion"`
}

type CapabilityDegradation struct {
	Feature string `json:"feature"`
	Reason  string `json:"reason"`
}

type Capabilities struct {
	Model                       string                  `json:"model"`
	ModelProfile                string                  `json:"modelProfile"`
	MinDurationSeconds          int                     `json:"minDurationSeconds"`
	MaxDurationSeconds          int                     `json:"maxDurationSeconds"`
	SupportedDurations          []int                   `json:"supportedDurations"`
	SupportsSmartDuration       bool                    `json:"supportsSmartDuration"`
	AspectRatios                []string                `json:"aspectRatios"`
	Resolutions                 []string                `json:"resolutions"`
	TaskModes                   []string                `json:"taskModes"`
	ReferenceRoles              []string                `json:"referenceRoles"`
	SupportsResolution          bool                    `json:"supportsResolution"`
	SupportsGenerateAudio       bool                    `json:"supportsGenerateAudio"`
	SupportsMultimodalReference bool                    `json:"supportsMultimodalReference"`
	SupportsEdit                bool                    `json:"supportsEdit"`
	SupportsExtend              bool                    `json:"supportsExtend"`
	SupportsSeed                bool                    `json:"supportsSeed"`
	SupportsCameraFixed         bool                    `json:"supportsCameraFixed"`
	Limits                      config.MediaLimits      `json:"limits"`
	CapabilityVersion           string                  `json:"capabilityVersion"`
	Source                      CapabilitySource        `json:"source"`
	Degradations                []CapabilityDegradation `json:"degradations"`
}

type officialCapabilityProfile struct {
	name                 string
	version              string
	durations            []int
	smartDuration        bool
	aspectRatios         []string
	resolutions          []string
	resolutionUnverified bool
	generateAudio        bool
	taskModes            []string
	referenceRoles       []string
	supportsSeed         bool
	supportsCameraFixed  bool
	limits               config.MediaLimits
}

// LegacyFlatContract returns the built-in, currently proven flat-array contract.
// It lives in video until config exposes its internal default constructor.
func LegacyFlatContract() config.GatewayContractConfig {
	return config.GatewayContractConfig{
		Name:          "legacy_flat_v1",
		Version:       "1",
		DeclaredModes: []string{"reference"},
		Duration: config.FieldEncoding{
			Name:      "seconds",
			ValueType: "string",
		},
		AspectRatio: config.FieldEncoding{
			Name:      "aspect_ratio",
			ValueType: "string",
		},
		References: config.ReferenceEncoding{
			Mode:          "flat_arrays",
			ImageField:    "images",
			VideoField:    "videos",
			AudioField:    "audios",
			SupportsRoles: []string{"reference_image", "reference_video", "reference_audio"},
		},
		Limits: config.MediaLimits{
			MaxImages:            4,
			MaxVideos:            3,
			MaxAudios:            1,
			MaxVideoSecondsTotal: 15,
			MaxAudioSecondsTotal: 15,
		},
	}
}

func ResolveCapabilities(input CapabilityConfig) Capabilities {
	model := strings.TrimSpace(input.Model)
	explicitProfile := strings.TrimSpace(input.ModelProfile)
	contract := config.TrimGatewayContract(input.GatewayContract)
	profile, selection := selectOfficialProfile(model, explicitProfile)

	capabilities := Capabilities{
		Model:               model,
		ModelProfile:        profile.name,
		SupportsSeed:        profile.supportsSeed,
		SupportsCameraFixed: profile.supportsCameraFixed,
		Source: CapabilitySource{
			OfficialProfile:        profile.name,
			OfficialProfileVersion: profile.version,
			Selection:              selection,
			GatewayContract:        contract.Name,
			GatewayContractVersion: contract.Version,
		},
	}

	capabilities.SupportedDurations = intersectDurationValues(profile, contract)
	if len(capabilities.SupportedDurations) > 0 {
		capabilities.MinDurationSeconds = capabilities.SupportedDurations[0]
		capabilities.MaxDurationSeconds = capabilities.SupportedDurations[len(capabilities.SupportedDurations)-1]
	}
	capabilities.SupportsSmartDuration = profile.smartDuration && supportsSmartDuration(contract.Duration)
	capabilities.AspectRatios = intersectAspectRatios(profile.aspectRatios, contract)
	capabilities.Resolutions = intersectMappedValues(profile.resolutions, contract.Resolution, true)
	capabilities.SupportsResolution = len(capabilities.Resolutions) > 0
	capabilities.SupportsGenerateAudio = profile.generateAudio && supportsBooleanField(contract.GenerateAudio)

	rawRoles := intersectReferenceRoles(profile.referenceRoles, contract.References)
	capabilities.TaskModes = intersectTaskModes(profile.taskModes, contract, rawRoles)
	capabilities.SupportsEdit = containsString(capabilities.TaskModes, "edit")
	capabilities.SupportsExtend = containsString(capabilities.TaskModes, "extend")
	capabilities.ReferenceRoles = filterRolesForModes(rawRoles, capabilities.TaskModes)
	capabilities.Limits = intersectMediaLimits(profile.limits, contract.Limits, capabilities.ReferenceRoles)
	capabilities.SupportsMultimodalReference = supportsMultipleReferenceKinds(capabilities.ReferenceRoles, capabilities.Limits)

	capabilities.Degradations = explainDegradations(profile, selection, contract, capabilities)
	capabilities.CapabilityVersion = capabilityVersion(profile.version, model, explicitProfile, contract)
	return capabilities
}

func selectOfficialProfile(model, explicitProfile string) (officialCapabilityProfile, string) {
	if exactProfile, ok := exactModelProfile(model); ok {
		if explicitProfile == "" || explicitProfile == exactProfile {
			return officialProfile(exactProfile), "exact_model"
		}
		if isAllowedExplicitProfile(explicitProfile) {
			return officialProfile("generic_unknown"), "profile_conflict"
		}
		return officialProfile("generic_unknown"), "invalid_explicit_profile"
	}

	if explicitProfile != "" {
		if isAllowedExplicitProfile(explicitProfile) {
			return officialProfile(explicitProfile), "explicit_profile"
		}
		return officialProfile("generic_unknown"), "invalid_explicit_profile"
	}
	return officialProfile("generic_unknown"), "generic_fallback"
}

func exactModelProfile(model string) (string, bool) {
	switch model {
	case "video-ds-2.0":
		return "standard", true
	case "video-ds-2.0-fast", "as-sd2.0-fast":
		return "fast", true
	default:
		return "", false
	}
}

func isAllowedExplicitProfile(profile string) bool {
	switch profile {
	case "standard", "fast", "mini":
		return true
	default:
		return false
	}
}

func officialProfile(name string) officialCapabilityProfile {
	seedanceDurations := integerRange(4, 15)
	seedanceAspects := []string{"adaptive", "21:9", "16:9", "4:3", "1:1", "3:4", "9:16"}
	seedanceModes := []string{"reference", "edit", "extend"}
	seedanceRoles := []string{
		"reference_image",
		"first_frame",
		"last_frame",
		"reference_video",
		"reference_audio",
		"edit_target",
		"extend_target",
	}
	seedanceLimits := config.MediaLimits{
		MaxImages:            9,
		MaxVideos:            3,
		MaxAudios:            3,
		MaxVideoSecondsTotal: 15,
		MaxAudioSecondsTotal: 15,
	}

	switch name {
	case "standard":
		return officialCapabilityProfile{
			name:           "standard",
			version:        officialCapabilityRegistryVersion,
			durations:      seedanceDurations,
			smartDuration:  true,
			aspectRatios:   seedanceAspects,
			resolutions:    []string{"480P", "720P", "1080P", "4K"},
			generateAudio:  true,
			taskModes:      seedanceModes,
			referenceRoles: seedanceRoles,
			limits:         seedanceLimits,
		}
	case "fast", "mini":
		return officialCapabilityProfile{
			name:                 name,
			version:              officialCapabilityRegistryVersion,
			durations:            seedanceDurations,
			smartDuration:        true,
			aspectRatios:         seedanceAspects,
			resolutionUnverified: true,
			generateAudio:        true,
			taskModes:            seedanceModes,
			referenceRoles:       seedanceRoles,
			limits:               seedanceLimits,
		}
	default:
		return officialCapabilityProfile{
			name:           "generic_unknown",
			version:        officialCapabilityRegistryVersion,
			durations:      []int{5, 10, 15},
			aspectRatios:   []string{"16:9", "9:16", "1:1"},
			taskModes:      []string{"reference"},
			referenceRoles: []string{"reference_image", "reference_video", "reference_audio"},
			limits: config.MediaLimits{
				MaxImages:            4,
				MaxVideos:            2,
				MaxAudios:            1,
				MaxVideoSecondsTotal: 15,
				MaxAudioSecondsTotal: 15,
			},
		}
	}
}

func intersectDurationValues(profile officialCapabilityProfile, contract config.GatewayContractConfig) []int {
	if !supportsDurationField(contract.Duration) {
		return nil
	}
	if isLegacyFlatContract(contract) {
		return intersectInts(profile.durations, []int{5, 10, 15})
	}
	return append([]int(nil), profile.durations...)
}

func supportsDurationField(field config.FieldEncoding) bool {
	return field.Name != "" && (field.ValueType == "int" || field.ValueType == "string")
}

func supportsSmartDuration(field config.FieldEncoding) bool {
	if !supportsDurationField(field) {
		return false
	}
	return field.ValueMap["smart"] == "-1"
}

func supportsBooleanField(field config.FieldEncoding) bool {
	return field.Name != "" && field.ValueType == "bool"
}

func intersectAspectRatios(official []string, contract config.GatewayContractConfig) []string {
	if isLegacyFlatContract(contract) {
		if contract.AspectRatio.Name == "" || contract.AspectRatio.ValueType != "string" {
			return nil
		}
		return intersectStrings([]string{"16:9", "9:16", "1:1"}, official)
	}
	return intersectMappedValues(official, contract.AspectRatio, false)
}

func intersectMappedValues(official []string, field config.FieldEncoding, requireExplicitMap bool) []string {
	if len(official) == 0 || field.Name == "" || field.ValueType != "string" {
		return nil
	}
	if len(field.ValueMap) == 0 {
		if requireExplicitMap {
			return nil
		}
		return append([]string(nil), official...)
	}

	result := make([]string, 0, len(official))
	for _, value := range official {
		if _, ok := field.ValueMap[value]; ok {
			result = append(result, value)
		}
	}
	return result
}

func isLegacyFlatContract(contract config.GatewayContractConfig) bool {
	return contract.Name == "legacy_flat_v1" && contract.Version == "1"
}

func intersectInts(left, right []int) []int {
	result := make([]int, 0, len(left))
	for _, value := range left {
		for _, candidate := range right {
			if value == candidate {
				result = append(result, value)
				break
			}
		}
	}
	return result
}

func intersectStrings(left, right []string) []string {
	result := make([]string, 0, len(left))
	for _, value := range left {
		if containsString(right, value) {
			result = append(result, value)
		}
	}
	return result
}

func intersectReferenceRoles(official []string, contract config.ReferenceEncoding) []string {
	result := make([]string, 0, len(official))
	for _, role := range official {
		if !containsString(contract.SupportsRoles, role) || !canEncodeReferenceRole(contract, role) {
			continue
		}
		result = append(result, role)
	}
	return result
}

func canEncodeReferenceRole(contract config.ReferenceEncoding, role string) bool {
	var mediaField string
	switch role {
	case "reference_image", "first_frame", "last_frame":
		mediaField = contract.ImageField
	case "reference_video", "edit_target", "extend_target":
		mediaField = contract.VideoField
	case "reference_audio":
		mediaField = contract.AudioField
	default:
		return false
	}
	if mediaField == "" {
		return false
	}

	switch role {
	case "reference_image", "reference_video", "reference_audio":
		if contract.Mode == "flat_arrays" {
			return true
		}
		return contract.RoleFields[role] != ""
	default:
		return contract.Mode != "flat_arrays" && contract.RoleFields[role] != ""
	}
}

func intersectTaskModes(official []string, contract config.GatewayContractConfig, roles []string) []string {
	result := make([]string, 0, len(official))
	for _, mode := range official {
		if !containsString(contract.DeclaredModes, mode) {
			continue
		}
		switch mode {
		case "reference":
			if hasGeneralReferenceRole(roles) {
				result = append(result, mode)
			}
		case "edit":
			if containsString(roles, "edit_target") && modeCanBeEncoded(contract, "edit_target") {
				result = append(result, mode)
			}
		case "extend":
			if containsString(roles, "extend_target") && modeCanBeEncoded(contract, "extend_target") {
				result = append(result, mode)
			}
		}
	}
	return result
}

func modeCanBeEncoded(contract config.GatewayContractConfig, targetRole string) bool {
	if contract.TaskMode.Name != "" && contract.TaskMode.ValueType == "string" {
		return true
	}
	return contract.References.Mode != "flat_arrays" && contract.References.RoleFields[targetRole] != ""
}

func filterRolesForModes(roles, modes []string) []string {
	result := make([]string, 0, len(roles))
	for _, role := range roles {
		if role == "edit_target" && !containsString(modes, "edit") {
			continue
		}
		if role == "extend_target" && !containsString(modes, "extend") {
			continue
		}
		result = append(result, role)
	}
	return result
}

func hasGeneralReferenceRole(roles []string) bool {
	return containsString(roles, "reference_image") || containsString(roles, "reference_video") || containsString(roles, "reference_audio")
}

func intersectMediaLimits(official, gateway config.MediaLimits, roles []string) config.MediaLimits {
	limits := config.MediaLimits{}
	if hasImageRole(roles) {
		limits.MaxImages = intersectPositiveInt(official.MaxImages, gateway.MaxImages)
	}
	if hasVideoRole(roles) {
		limits.MaxVideos = intersectPositiveInt(official.MaxVideos, gateway.MaxVideos)
		limits.MaxVideoSecondsTotal = intersectPositiveFloat(official.MaxVideoSecondsTotal, gateway.MaxVideoSecondsTotal)
	}
	if containsString(roles, "reference_audio") {
		limits.MaxAudios = intersectPositiveInt(official.MaxAudios, gateway.MaxAudios)
		limits.MaxAudioSecondsTotal = intersectPositiveFloat(official.MaxAudioSecondsTotal, gateway.MaxAudioSecondsTotal)
	}
	return limits
}

func hasImageRole(roles []string) bool {
	return containsString(roles, "reference_image") || containsString(roles, "first_frame") || containsString(roles, "last_frame")
}

func hasVideoRole(roles []string) bool {
	return containsString(roles, "reference_video") || containsString(roles, "edit_target") || containsString(roles, "extend_target")
}

func intersectPositiveInt(official, gateway int) int {
	if official <= 0 || gateway <= 0 {
		return 0
	}
	if gateway < official {
		return gateway
	}
	return official
}

func intersectPositiveFloat(official, gateway float64) float64 {
	if official <= 0 || gateway <= 0 {
		return 0
	}
	if gateway < official {
		return gateway
	}
	return official
}

func supportsMultipleReferenceKinds(roles []string, limits config.MediaLimits) bool {
	kinds := 0
	if hasImageRole(roles) && limits.MaxImages > 0 {
		kinds++
	}
	if hasVideoRole(roles) && limits.MaxVideos > 0 {
		kinds++
	}
	if containsString(roles, "reference_audio") && limits.MaxAudios > 0 {
		kinds++
	}
	return kinds >= 2
}

func explainDegradations(profile officialCapabilityProfile, selection string, contract config.GatewayContractConfig, got Capabilities) []CapabilityDegradation {
	degradations := make([]CapabilityDegradation, 0)
	add := func(feature, reason string) {
		for _, degradation := range degradations {
			if degradation.Feature == feature {
				return
			}
		}
		degradations = append(degradations, CapabilityDegradation{Feature: feature, Reason: reason})
	}

	if profile.name == "generic_unknown" {
		reason := "unknown_model"
		switch selection {
		case "invalid_explicit_profile":
			reason = "invalid_explicit_profile"
		case "profile_conflict":
			reason = "profile_conflict"
		}
		add("model_profile", reason)
	}

	if len(profile.durations) > 0 && len(got.SupportedDurations) == 0 {
		add("duration", "gateway_contract_missing_field")
	}
	if isLegacyFlatContract(contract) {
		for _, value := range profile.durations {
			if !containsInt(got.SupportedDurations, value) {
				add("duration."+strconv.Itoa(value), "legacy_contract_value_not_proven")
			}
		}
	}
	if profile.smartDuration && !got.SupportsSmartDuration {
		reason := "gateway_contract_missing_smart_mapping"
		if !supportsDurationField(contract.Duration) {
			reason = "gateway_contract_missing_field"
		}
		add("smart_duration", reason)
	}

	if isLegacyFlatContract(contract) {
		for _, value := range profile.aspectRatios {
			if !containsString(got.AspectRatios, value) {
				add("aspect_ratio."+value, "legacy_contract_value_not_proven")
			}
		}
	} else if len(profile.aspectRatios) > 0 {
		if contract.AspectRatio.Name == "" || contract.AspectRatio.ValueType != "string" {
			add("aspect_ratio", "gateway_contract_missing_field")
		} else if len(contract.AspectRatio.ValueMap) > 0 {
			for _, value := range profile.aspectRatios {
				if !containsString(got.AspectRatios, value) {
					add("aspect_ratio."+value, "gateway_contract_value_not_declared")
				}
			}
		}
	}

	if profile.resolutionUnverified {
		add("resolution", "official_profile_unverified")
	} else if len(profile.resolutions) > 0 {
		switch {
		case contract.Resolution.Name == "" || contract.Resolution.ValueType != "string":
			add("resolution", "gateway_contract_missing_field")
		case len(contract.Resolution.ValueMap) == 0:
			add("resolution", "gateway_contract_missing_enum")
		default:
			for _, value := range profile.resolutions {
				if !containsString(got.Resolutions, value) {
					add("resolution."+value, "gateway_contract_value_not_declared")
				}
			}
		}
	}

	if profile.generateAudio && !got.SupportsGenerateAudio {
		add("generate_audio", "gateway_contract_missing_field")
	}

	for _, mode := range profile.taskModes {
		if containsString(got.TaskModes, mode) {
			continue
		}
		reason := "gateway_contract_mode_not_declared"
		if containsString(contract.DeclaredModes, mode) {
			reason = "gateway_contract_cannot_encode_target"
		}
		add("task_mode."+mode, reason)
	}

	for _, role := range profile.referenceRoles {
		if containsString(got.ReferenceRoles, role) {
			continue
		}
		reason := "gateway_contract_role_not_declared"
		if containsString(contract.References.SupportsRoles, role) {
			reason = "gateway_contract_cannot_encode_role"
			if role == "edit_target" && !containsString(got.TaskModes, "edit") || role == "extend_target" && !containsString(got.TaskModes, "extend") {
				reason = "gateway_contract_mode_not_declared"
			}
		}
		add("reference_role."+role, reason)
	}

	explainLimitDegradation := func(feature string, officialValue, effectiveValue float64, referenceAvailable bool) {
		if officialValue <= effectiveValue {
			return
		}
		reason := "gateway_contract_limit"
		if !referenceAvailable {
			reason = "gateway_contract_reference_unavailable"
		}
		add(feature, reason)
	}
	explainLimitDegradation("limits.max_images", float64(profile.limits.MaxImages), float64(got.Limits.MaxImages), hasImageRole(got.ReferenceRoles))
	explainLimitDegradation("limits.max_videos", float64(profile.limits.MaxVideos), float64(got.Limits.MaxVideos), hasVideoRole(got.ReferenceRoles))
	explainLimitDegradation("limits.max_audios", float64(profile.limits.MaxAudios), float64(got.Limits.MaxAudios), containsString(got.ReferenceRoles, "reference_audio"))
	explainLimitDegradation("limits.max_video_seconds_total", profile.limits.MaxVideoSecondsTotal, got.Limits.MaxVideoSecondsTotal, hasVideoRole(got.ReferenceRoles))
	explainLimitDegradation("limits.max_audio_seconds_total", profile.limits.MaxAudioSecondsTotal, got.Limits.MaxAudioSecondsTotal, containsString(got.ReferenceRoles, "reference_audio"))

	return degradations
}

func capabilityVersion(profileVersion, model, explicitProfile string, contract config.GatewayContractConfig) string {
	contractJSON, err := json.Marshal(contract)
	if err != nil {
		return ""
	}
	var contractBody any
	if err := json.Unmarshal(contractJSON, &contractBody); err != nil {
		return ""
	}
	payload := map[string]any{
		"officialProfileVersion": profileVersion,
		"gatewayContractName":    contract.Name,
		"gatewayContractVersion": contract.Version,
		"gatewayContractBody":    contractBody,
		"model":                  model,
		"explicitProfile":        explicitProfile,
	}
	canonical, err := json.Marshal(payload)
	if err != nil {
		return ""
	}
	hash := sha256.Sum256(canonical)
	return hex.EncodeToString(hash[:])
}

func integerRange(first, last int) []int {
	values := make([]int, 0, last-first+1)
	for value := first; value <= last; value++ {
		values = append(values, value)
	}
	return values
}

func containsString(values []string, want string) bool {
	for _, value := range values {
		if value == want {
			return true
		}
	}
	return false
}

func containsInt(values []int, want int) bool {
	for _, value := range values {
		if value == want {
			return true
		}
	}
	return false
}
