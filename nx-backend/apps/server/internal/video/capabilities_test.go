package video

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"reflect"
	"strconv"
	"strings"
	"testing"

	"nine-xing/nx-backend/apps/server/internal/config"
)

var seedanceDurationValues = []int{4, 5, 6, 7, 8, 9, 10, 11, 12, 13, 14, 15}

var seedanceAspectRatios = []string{
	"adaptive", "21:9", "16:9", "4:3", "1:1", "3:4", "9:16",
}

var seedanceReferenceRoles = []string{
	"reference_image",
	"first_frame",
	"last_frame",
	"reference_video",
	"reference_audio",
	"edit_target",
	"extend_target",
}

func TestResolveCapabilitiesUsesOfficialProfileIntersection(t *testing.T) {
	got := ResolveCapabilities(CapabilityConfig{
		Model:           "video-ds-2.0",
		GatewayContract: LegacyFlatContract(),
	})
	if got.ModelProfile != "standard" {
		t.Fatal(got.ModelProfile)
	}
	if got.SupportsResolution {
		t.Fatal("legacy contract must hide resolution")
	}
	if got.SupportsEdit || got.SupportsExtend {
		t.Fatal("flat arrays cannot encode targets")
	}
	if got.CapabilityVersion == "" {
		t.Fatal("missing version")
	}
	if got.Limits.MaxImages != 4 {
		t.Fatalf("legacy contract should keep conservative proven limit, got %+v", got.Limits)
	}
}

func TestResolveCapabilitiesUnknownModelUsesGenericProfile(t *testing.T) {
	got := ResolveCapabilities(CapabilityConfig{Model: "custom-video"})
	if got.Source.OfficialProfile != "generic_unknown" {
		t.Fatal(got.Source)
	}
	if got.SupportsSmartDuration {
		t.Fatal("unknown capability must fail closed")
	}
}

func TestLegacyFlatContractMatchesIntermediaryDocumentedLimits(t *testing.T) {
	got := LegacyFlatContract()
	want := config.MediaLimits{
		MaxImages:            4,
		MaxVideos:            3,
		MaxAudios:            1,
		MaxVideoSecondsTotal: 15,
		MaxAudioSecondsTotal: 15,
	}
	if !reflect.DeepEqual(got.Limits, want) {
		t.Fatalf("legacy intermediary limits = %+v, want %+v", got.Limits, want)
	}
}

func TestLegacyFlatContractUsesConfigSourceOfTruth(t *testing.T) {
	want := config.LegacyVideoGatewayContract()
	got := LegacyFlatContract()
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("video legacy wrapper diverged from config source:\n got: %#v\nwant: %#v", got, want)
	}

	got.DeclaredModes[0] = "edit"
	got.References.SupportsRoles[0] = "edit_target"
	if fresh := LegacyFlatContract(); !reflect.DeepEqual(fresh, want) {
		t.Fatalf("mutating wrapper result polluted source: %#v", fresh)
	}
}

func TestResolveCapabilitiesExactASFastAliasStaysFailClosed(t *testing.T) {
	got := ResolveCapabilities(CapabilityConfig{
		Model:           "as-sd2.0-fast",
		GatewayContract: configuredSeedanceContract(),
	})
	if got.ModelProfile != "fast" || got.Source.Selection != "exact_model" {
		t.Fatalf("exact intermediary alias resolved incorrectly: profile=%q source=%+v", got.ModelProfile, got.Source)
	}
	if got.SupportsResolution || len(got.Resolutions) != 0 {
		t.Fatalf("fast alias must keep resolution fail-closed: supported=%v values=%#v", got.SupportsResolution, got.Resolutions)
	}
	assertDegradation(t, got, "resolution", "official_profile_unverified")
}

func TestResolveCapabilitiesUnknownReferenceEncodingModeFailsClosed(t *testing.T) {
	for _, mode := range []string{"", "content_item", "future_items"} {
		name := mode
		if name == "" {
			name = "empty"
		}
		t.Run(name, func(t *testing.T) {
			contract := configuredSeedanceContract()
			contract.References.Mode = mode

			got := ResolveCapabilities(CapabilityConfig{Model: "video-ds-2.0", GatewayContract: contract})
			if len(got.ReferenceRoles) != 0 {
				t.Fatalf("unknown encoding mode %q exposed roles: %#v", mode, got.ReferenceRoles)
			}
			if got.SupportsEdit || got.SupportsExtend {
				t.Fatalf("unknown encoding mode %q exposed edit/extend: %v/%v", mode, got.SupportsEdit, got.SupportsExtend)
			}
			assertDegradation(t, got, "task_mode.edit", "unknown_reference_encoding_mode")
			assertDegradation(t, got, "task_mode.extend", "unknown_reference_encoding_mode")
			assertDegradation(t, got, "reference_role.first_frame", "unknown_reference_encoding_mode")
			assertDegradation(t, got, "reference_role.edit_target", "unknown_reference_encoding_mode")
		})
	}
}

func TestResolveCapabilitiesReferenceModeDoesNotRequireReferenceRoles(t *testing.T) {
	tests := []struct {
		name       string
		references config.ReferenceEncoding
		wantRoles  []string
	}{
		{
			name:      "no reference encoding",
			wantRoles: []string{},
		},
		{
			name: "first and last frames only",
			references: config.ReferenceEncoding{
				Mode:       "content_items",
				ImageField: "image_url",
				RoleFields: map[string]string{
					"first_frame": "first_frame",
					"last_frame":  "last_frame",
				},
				SupportsRoles: []string{"first_frame", "last_frame"},
			},
			wantRoles: []string{"first_frame", "last_frame"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			contract := configuredSeedanceContract()
			contract.DeclaredModes = []string{"reference"}
			contract.References = tt.references

			got := ResolveCapabilities(CapabilityConfig{Model: "video-ds-2.0", GatewayContract: contract})
			if !reflect.DeepEqual(got.TaskModes, []string{"reference"}) {
				t.Fatalf("basic text-to-video reference mode was hidden: %#v", got.TaskModes)
			}
			if !reflect.DeepEqual(got.ReferenceRoles, tt.wantRoles) {
				t.Fatalf("reference roles = %#v, want %#v", got.ReferenceRoles, tt.wantRoles)
			}
			if got.SupportsEdit || got.SupportsExtend {
				t.Fatalf("undeclared advanced modes were exposed: edit=%v extend=%v", got.SupportsEdit, got.SupportsExtend)
			}
			assertNoDegradation(t, got, "task_mode.reference")
		})
	}
}

func TestResolveCapabilitiesRejectsBlankEnumMappings(t *testing.T) {
	contract := configuredSeedanceContract()
	contract.Resolution.ValueMap = map[string]string{"1080P": "   "}
	contract.AspectRatio.ValueMap = map[string]string{
		"16:9": " ",
		"9:16": "portrait",
	}

	got := ResolveCapabilities(CapabilityConfig{Model: "video-ds-2.0", GatewayContract: contract})
	if got.SupportsResolution || len(got.Resolutions) != 0 {
		t.Fatalf("blank resolution mapping was exposed: supported=%v values=%#v", got.SupportsResolution, got.Resolutions)
	}
	if !reflect.DeepEqual(got.AspectRatios, []string{"9:16"}) {
		t.Fatalf("blank aspect mapping was exposed: %#v", got.AspectRatios)
	}
	assertDegradation(t, got, "resolution.1080P", "gateway_contract_value_not_encodable")
	assertDegradation(t, got, "aspect_ratio.16:9", "gateway_contract_value_not_encodable")
}

func TestResolveCapabilitiesClassifiesReferenceEncodingDegradations(t *testing.T) {
	tests := []struct {
		name           string
		mutate         func(*config.GatewayContractConfig)
		wantTaskReason string
		wantRoleReason string
	}{
		{
			name: "mode not declared",
			mutate: func(contract *config.GatewayContractConfig) {
				contract.DeclaredModes = []string{"reference"}
			},
			wantTaskReason: "gateway_contract_mode_not_declared",
			wantRoleReason: "gateway_contract_mode_not_declared",
		},
		{
			name: "flat arrays cannot encode target",
			mutate: func(contract *config.GatewayContractConfig) {
				contract.References.Mode = "flat_arrays"
			},
			wantTaskReason: "gateway_contract_cannot_encode_target",
			wantRoleReason: "gateway_contract_cannot_encode_role",
		},
		{
			name: "content items target field missing",
			mutate: func(contract *config.GatewayContractConfig) {
				contract.References.RoleFields["edit_target"] = " "
			},
			wantTaskReason: "gateway_contract_cannot_encode_target",
			wantRoleReason: "gateway_contract_cannot_encode_role",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			contract := configuredSeedanceContract()
			tt.mutate(&contract)
			got := ResolveCapabilities(CapabilityConfig{Model: "video-ds-2.0", GatewayContract: contract})
			assertDegradation(t, got, "task_mode.edit", tt.wantTaskReason)
			assertDegradation(t, got, "reference_role.edit_target", tt.wantRoleReason)
		})
	}
}

func TestResolveCapabilitiesExactModelProfileConflictFailsClosed(t *testing.T) {
	got := ResolveCapabilities(CapabilityConfig{
		Model:           "video-ds-2.0-fast",
		ModelProfile:    "standard",
		GatewayContract: configuredSeedanceContract(),
	})
	if got.ModelProfile != "generic_unknown" {
		t.Fatalf("conflicting exact model profile = %q, want generic_unknown", got.ModelProfile)
	}
	if got.Source.Selection != "profile_conflict" || got.Source.OfficialProfile != "generic_unknown" {
		t.Fatalf("conflict source = %+v", got.Source)
	}
	if got.SupportsResolution || len(got.Resolutions) != 0 {
		t.Fatalf("conflicting fast model must not gain standard resolution: supported=%v values=%#v", got.SupportsResolution, got.Resolutions)
	}
	assertDegradation(t, got, "model_profile", "profile_conflict")
}

func TestResolveCapabilitiesExactModelAcceptsMatchingProfile(t *testing.T) {
	got := ResolveCapabilities(CapabilityConfig{
		Model:           "video-ds-2.0-fast",
		ModelProfile:    "fast",
		GatewayContract: configuredSeedanceContract(),
	})
	if got.ModelProfile != "fast" || got.Source.Selection != "exact_model" {
		t.Fatalf("matching exact profile resolved incorrectly: profile=%q source=%+v", got.ModelProfile, got.Source)
	}
	if got.SupportsResolution || len(got.Resolutions) != 0 {
		t.Fatalf("fast profile must remain resolution fail-closed: supported=%v values=%#v", got.SupportsResolution, got.Resolutions)
	}
	assertDegradation(t, got, "resolution", "official_profile_unverified")
}

func TestResolveCapabilitiesSelectsOnlyExactOrExplicitProfiles(t *testing.T) {
	contract := configuredSeedanceContract()
	tests := []struct {
		name              string
		input             CapabilityConfig
		wantProfile       string
		wantSelection     string
		wantDurations     []int
		wantAspects       []string
		wantResolutions   []string
		wantRoles         []string
		wantLimits        config.MediaLimits
		wantSmart         bool
		wantGenerateAudio bool
		wantEdit          bool
		wantExtend        bool
		wantDegradation   CapabilityDegradation
	}{
		{
			name:              "exact standard model",
			input:             CapabilityConfig{Model: "video-ds-2.0", GatewayContract: contract},
			wantProfile:       "standard",
			wantSelection:     "exact_model",
			wantDurations:     seedanceDurationValues,
			wantAspects:       seedanceAspectRatios,
			wantResolutions:   []string{"480P", "720P", "1080P", "4K"},
			wantRoles:         seedanceReferenceRoles,
			wantLimits:        config.MediaLimits{MaxImages: 9, MaxVideos: 3, MaxAudios: 3, MaxVideoSecondsTotal: 15, MaxAudioSecondsTotal: 15},
			wantSmart:         true,
			wantGenerateAudio: true,
			wantEdit:          true,
			wantExtend:        true,
		},
		{
			name:              "exact fast model",
			input:             CapabilityConfig{Model: "video-ds-2.0-fast", GatewayContract: contract},
			wantProfile:       "fast",
			wantSelection:     "exact_model",
			wantDurations:     seedanceDurationValues,
			wantAspects:       seedanceAspectRatios,
			wantRoles:         seedanceReferenceRoles,
			wantLimits:        config.MediaLimits{MaxImages: 9, MaxVideos: 3, MaxAudios: 3, MaxVideoSecondsTotal: 15, MaxAudioSecondsTotal: 15},
			wantSmart:         true,
			wantGenerateAudio: true,
			wantEdit:          true,
			wantExtend:        true,
			wantDegradation:   CapabilityDegradation{Feature: "resolution", Reason: "official_profile_unverified"},
		},
		{
			name:              "explicit mini profile on custom model",
			input:             CapabilityConfig{Model: "private-video-model", ModelProfile: "mini", GatewayContract: contract},
			wantProfile:       "mini",
			wantSelection:     "explicit_profile",
			wantDurations:     seedanceDurationValues,
			wantAspects:       seedanceAspectRatios,
			wantRoles:         seedanceReferenceRoles,
			wantLimits:        config.MediaLimits{MaxImages: 9, MaxVideos: 3, MaxAudios: 3, MaxVideoSecondsTotal: 15, MaxAudioSecondsTotal: 15},
			wantSmart:         true,
			wantGenerateAudio: true,
			wantEdit:          true,
			wantExtend:        true,
			wantDegradation:   CapabilityDegradation{Feature: "resolution", Reason: "official_profile_unverified"},
		},
		{
			name:              "explicit standard profile on custom model",
			input:             CapabilityConfig{Model: "private-standard-model", ModelProfile: "standard", GatewayContract: contract},
			wantProfile:       "standard",
			wantSelection:     "explicit_profile",
			wantDurations:     seedanceDurationValues,
			wantAspects:       seedanceAspectRatios,
			wantResolutions:   []string{"480P", "720P", "1080P", "4K"},
			wantRoles:         seedanceReferenceRoles,
			wantLimits:        config.MediaLimits{MaxImages: 9, MaxVideos: 3, MaxAudios: 3, MaxVideoSecondsTotal: 15, MaxAudioSecondsTotal: 15},
			wantSmart:         true,
			wantGenerateAudio: true,
			wantEdit:          true,
			wantExtend:        true,
		},
		{
			name:            "mini-looking model is not inferred",
			input:           CapabilityConfig{Model: "video-ds-2.0-mini", GatewayContract: contract},
			wantProfile:     "generic_unknown",
			wantSelection:   "generic_fallback",
			wantDurations:   []int{5, 10, 15},
			wantAspects:     []string{"16:9", "9:16", "1:1"},
			wantRoles:       []string{"reference_image", "reference_video", "reference_audio"},
			wantLimits:      config.MediaLimits{MaxImages: 4, MaxVideos: 2, MaxAudios: 1, MaxVideoSecondsTotal: 15, MaxAudioSecondsTotal: 15},
			wantDegradation: CapabilityDegradation{Feature: "model_profile", Reason: "unknown_model"},
		},
		{
			name:            "invalid explicit profile fails closed",
			input:           CapabilityConfig{Model: "video-ds-2.0", ModelProfile: "ultra", GatewayContract: contract},
			wantProfile:     "generic_unknown",
			wantSelection:   "invalid_explicit_profile",
			wantDurations:   []int{5, 10, 15},
			wantAspects:     []string{"16:9", "9:16", "1:1"},
			wantRoles:       []string{"reference_image", "reference_video", "reference_audio"},
			wantLimits:      config.MediaLimits{MaxImages: 4, MaxVideos: 2, MaxAudios: 1, MaxVideoSecondsTotal: 15, MaxAudioSecondsTotal: 15},
			wantDegradation: CapabilityDegradation{Feature: "model_profile", Reason: "invalid_explicit_profile"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := ResolveCapabilities(tt.input)
			if got.Model != tt.input.Model {
				t.Fatalf("model = %q, want %q", got.Model, tt.input.Model)
			}
			if got.ModelProfile != tt.wantProfile {
				t.Fatalf("profile = %q, want %q", got.ModelProfile, tt.wantProfile)
			}
			if got.Source.OfficialProfile != tt.wantProfile || got.Source.Selection != tt.wantSelection {
				t.Fatalf("source = %+v, want profile %q selected by %q", got.Source, tt.wantProfile, tt.wantSelection)
			}
			if got.Source.OfficialProfileVersion == "" {
				t.Fatal("official profile source must be versioned")
			}
			if !reflect.DeepEqual(got.SupportedDurations, tt.wantDurations) {
				t.Fatalf("durations = %#v, want %#v", got.SupportedDurations, tt.wantDurations)
			}
			if !reflect.DeepEqual(got.AspectRatios, tt.wantAspects) {
				t.Fatalf("aspects = %#v, want %#v", got.AspectRatios, tt.wantAspects)
			}
			if !reflect.DeepEqual(got.Resolutions, tt.wantResolutions) {
				t.Fatalf("resolutions = %#v, want %#v", got.Resolutions, tt.wantResolutions)
			}
			if got.SupportsResolution != (len(tt.wantResolutions) > 0) {
				t.Fatalf("SupportsResolution = %v, resolutions = %#v", got.SupportsResolution, got.Resolutions)
			}
			if !reflect.DeepEqual(got.ReferenceRoles, tt.wantRoles) {
				t.Fatalf("roles = %#v, want %#v", got.ReferenceRoles, tt.wantRoles)
			}
			if !reflect.DeepEqual(got.Limits, tt.wantLimits) {
				t.Fatalf("limits = %+v, want %+v", got.Limits, tt.wantLimits)
			}
			if got.SupportsSmartDuration != tt.wantSmart || got.SupportsGenerateAudio != tt.wantGenerateAudio {
				t.Fatalf("smart/audio = %v/%v, want %v/%v", got.SupportsSmartDuration, got.SupportsGenerateAudio, tt.wantSmart, tt.wantGenerateAudio)
			}
			if got.SupportsEdit != tt.wantEdit || got.SupportsExtend != tt.wantExtend {
				t.Fatalf("edit/extend = %v/%v, want %v/%v", got.SupportsEdit, got.SupportsExtend, tt.wantEdit, tt.wantExtend)
			}
			if got.SupportsSeed || got.SupportsCameraFixed {
				t.Fatal("Seedance profiles must not advertise seed or camera_fixed")
			}
			if tt.wantDegradation.Feature != "" {
				assertDegradation(t, got, tt.wantDegradation.Feature, tt.wantDegradation.Reason)
			}
		})
	}
}

func TestResolveCapabilitiesIntersectsValuesRolesAndLimits(t *testing.T) {
	contract := configuredSeedanceContract()
	contract.DeclaredModes = []string{"reference", "edit"}
	contract.Duration.ValueMap = nil
	contract.AspectRatio.ValueMap = map[string]string{"16:9": "16:9", "9:16": "9:16"}
	contract.Resolution.ValueMap = map[string]string{"1080P": "1080p", "2K": "2k"}
	contract.GenerateAudio = config.FieldEncoding{}
	contract.References.SupportsRoles = []string{"reference_image", "reference_video", "edit_target"}
	contract.References.RoleFields = map[string]string{
		"reference_image": "reference_image",
		"reference_video": "reference_video",
		"edit_target":     "edit_target",
	}
	contract.References.AudioField = ""
	contract.Limits = config.MediaLimits{
		MaxImages:            20,
		MaxVideos:            1,
		MaxAudios:            5,
		MaxVideoSecondsTotal: 7,
		MaxAudioSecondsTotal: 15,
	}

	got := ResolveCapabilities(CapabilityConfig{Model: "video-ds-2.0", GatewayContract: contract})
	if !reflect.DeepEqual(got.SupportedDurations, seedanceDurationValues) || got.SupportsSmartDuration {
		t.Fatalf("unexpected duration intersection: values=%#v smart=%v", got.SupportedDurations, got.SupportsSmartDuration)
	}
	if !reflect.DeepEqual(got.AspectRatios, []string{"16:9", "9:16"}) {
		t.Fatalf("aspects = %#v", got.AspectRatios)
	}
	if !reflect.DeepEqual(got.Resolutions, []string{"1080P"}) || !got.SupportsResolution {
		t.Fatalf("resolution intersection = %#v (supported=%v)", got.Resolutions, got.SupportsResolution)
	}
	if !reflect.DeepEqual(got.TaskModes, []string{"reference", "edit"}) || !got.SupportsEdit || got.SupportsExtend {
		t.Fatalf("task modes = %#v, edit=%v extend=%v", got.TaskModes, got.SupportsEdit, got.SupportsExtend)
	}
	if !reflect.DeepEqual(got.ReferenceRoles, []string{"reference_image", "reference_video", "edit_target"}) {
		t.Fatalf("roles = %#v", got.ReferenceRoles)
	}
	if got.SupportsGenerateAudio {
		t.Fatal("missing gateway field must hide generateAudio")
	}
	wantLimits := config.MediaLimits{MaxImages: 9, MaxVideos: 1, MaxAudios: 0, MaxVideoSecondsTotal: 7, MaxAudioSecondsTotal: 0}
	if !reflect.DeepEqual(got.Limits, wantLimits) {
		t.Fatalf("limits = %+v, want %+v", got.Limits, wantLimits)
	}

	wantDegradations := map[string]string{
		"smart_duration":                 "gateway_contract_missing_smart_mapping",
		"aspect_ratio.adaptive":          "gateway_contract_value_not_declared",
		"aspect_ratio.21:9":              "gateway_contract_value_not_declared",
		"aspect_ratio.4:3":               "gateway_contract_value_not_declared",
		"aspect_ratio.1:1":               "gateway_contract_value_not_declared",
		"aspect_ratio.3:4":               "gateway_contract_value_not_declared",
		"resolution.480P":                "gateway_contract_value_not_declared",
		"resolution.720P":                "gateway_contract_value_not_declared",
		"resolution.4K":                  "gateway_contract_value_not_declared",
		"generate_audio":                 "gateway_contract_missing_field",
		"task_mode.extend":               "gateway_contract_mode_not_declared",
		"reference_role.first_frame":     "gateway_contract_role_not_declared",
		"reference_role.last_frame":      "gateway_contract_role_not_declared",
		"reference_role.reference_audio": "gateway_contract_role_not_declared",
		"reference_role.extend_target":   "gateway_contract_role_not_declared",
		"limits.max_videos":              "gateway_contract_limit",
		"limits.max_audios":              "gateway_contract_reference_unavailable",
		"limits.max_video_seconds_total": "gateway_contract_limit",
		"limits.max_audio_seconds_total": "gateway_contract_reference_unavailable",
	}
	assertExactDegradations(t, got, wantDegradations)
}

func TestResolveCapabilitiesExplainsEveryLegacyDegradation(t *testing.T) {
	got := ResolveCapabilities(CapabilityConfig{Model: "video-ds-2.0", GatewayContract: LegacyFlatContract()})
	if !reflect.DeepEqual(got.ReferenceRoles, []string{"reference_image", "reference_video", "reference_audio"}) {
		t.Fatalf("legacy roles = %#v", got.ReferenceRoles)
	}
	if !reflect.DeepEqual(got.TaskModes, []string{"reference"}) {
		t.Fatalf("legacy task modes = %#v", got.TaskModes)
	}
	wantLimits := config.MediaLimits{MaxImages: 4, MaxVideos: 3, MaxAudios: 1, MaxVideoSecondsTotal: 15, MaxAudioSecondsTotal: 15}
	if !reflect.DeepEqual(got.Limits, wantLimits) {
		t.Fatalf("legacy limits = %+v, want %+v", got.Limits, wantLimits)
	}

	wantDegradations := map[string]string{
		"duration.4":                   "legacy_contract_value_not_proven",
		"duration.6":                   "legacy_contract_value_not_proven",
		"duration.7":                   "legacy_contract_value_not_proven",
		"duration.8":                   "legacy_contract_value_not_proven",
		"duration.9":                   "legacy_contract_value_not_proven",
		"duration.11":                  "legacy_contract_value_not_proven",
		"duration.12":                  "legacy_contract_value_not_proven",
		"duration.13":                  "legacy_contract_value_not_proven",
		"duration.14":                  "legacy_contract_value_not_proven",
		"smart_duration":               "gateway_contract_missing_smart_mapping",
		"aspect_ratio.adaptive":        "legacy_contract_value_not_proven",
		"aspect_ratio.21:9":            "legacy_contract_value_not_proven",
		"aspect_ratio.4:3":             "legacy_contract_value_not_proven",
		"aspect_ratio.3:4":             "legacy_contract_value_not_proven",
		"resolution":                   "gateway_contract_missing_field",
		"generate_audio":               "gateway_contract_missing_field",
		"task_mode.edit":               "gateway_contract_mode_not_declared",
		"task_mode.extend":             "gateway_contract_mode_not_declared",
		"reference_role.first_frame":   "gateway_contract_role_not_declared",
		"reference_role.last_frame":    "gateway_contract_role_not_declared",
		"reference_role.edit_target":   "gateway_contract_role_not_declared",
		"reference_role.extend_target": "gateway_contract_role_not_declared",
		"limits.max_images":            "gateway_contract_limit",
		"limits.max_audios":            "gateway_contract_limit",
	}
	assertExactDegradations(t, got, wantDegradations)
}

func TestResolveCapabilitiesLegacyKeepsProvenValueSets(t *testing.T) {
	tests := []struct {
		name               string
		input              CapabilityConfig
		wantProfile        string
		wantMaxVideos      int
		wantOfficialHiding bool
	}{
		{
			name:               "standard",
			input:              CapabilityConfig{Model: "video-ds-2.0", GatewayContract: LegacyFlatContract()},
			wantProfile:        "standard",
			wantMaxVideos:      3,
			wantOfficialHiding: true,
		},
		{
			name:               "fast",
			input:              CapabilityConfig{Model: "video-ds-2.0-fast", GatewayContract: LegacyFlatContract()},
			wantProfile:        "fast",
			wantMaxVideos:      3,
			wantOfficialHiding: true,
		},
		{
			name:          "unknown",
			input:         CapabilityConfig{Model: "custom-video", GatewayContract: LegacyFlatContract()},
			wantProfile:   "generic_unknown",
			wantMaxVideos: 2,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := ResolveCapabilities(tt.input)
			if got.ModelProfile != tt.wantProfile {
				t.Fatalf("profile = %q, want %q", got.ModelProfile, tt.wantProfile)
			}
			if got.Source.GatewayContract != "legacy_flat_v1" || got.Source.GatewayContractVersion != "1" {
				t.Fatalf("legacy source = %+v", got.Source)
			}
			if !reflect.DeepEqual(got.SupportedDurations, []int{5, 10, 15}) {
				t.Fatalf("legacy durations = %#v, want only 5/10/15", got.SupportedDurations)
			}
			if !reflect.DeepEqual(got.AspectRatios, []string{"16:9", "9:16", "1:1"}) {
				t.Fatalf("legacy aspect ratios = %#v, want only proven set", got.AspectRatios)
			}
			if got.MinDurationSeconds != 5 || got.MaxDurationSeconds != 15 || got.SupportsSmartDuration {
				t.Fatalf("legacy duration bounds/smart = %d/%d/%v", got.MinDurationSeconds, got.MaxDurationSeconds, got.SupportsSmartDuration)
			}
			wantLimits := config.MediaLimits{MaxImages: 4, MaxVideos: tt.wantMaxVideos, MaxAudios: 1, MaxVideoSecondsTotal: 15, MaxAudioSecondsTotal: 15}
			if !reflect.DeepEqual(got.Limits, wantLimits) {
				t.Fatalf("legacy limits = %+v, want %+v", got.Limits, wantLimits)
			}

			if tt.wantOfficialHiding {
				for _, duration := range []int{4, 6, 7, 8, 9, 11, 12, 13, 14} {
					assertDegradation(t, got, "duration."+strconv.Itoa(duration), "legacy_contract_value_not_proven")
				}
				for _, aspect := range []string{"adaptive", "21:9", "4:3", "3:4"} {
					assertDegradation(t, got, "aspect_ratio."+aspect, "legacy_contract_value_not_proven")
				}
				assertDegradation(t, got, "smart_duration", "gateway_contract_missing_smart_mapping")
			} else {
				assertNoDegradationPrefix(t, got, "duration.")
				assertNoDegradationPrefix(t, got, "aspect_ratio.")
			}
		})
	}
}

func TestResolveCapabilitiesVersionIsDeterministicAndCoversEveryInput(t *testing.T) {
	baseContract := configuredSeedanceContract()
	base := CapabilityConfig{Model: "private-video", ModelProfile: "standard", GatewayContract: baseContract}
	first := ResolveCapabilities(base)
	second := ResolveCapabilities(base)
	if first.CapabilityVersion == "" || first.CapabilityVersion != second.CapabilityVersion {
		t.Fatalf("same input produced unstable versions: %q vs %q", first.CapabilityVersion, second.CapabilityVersion)
	}
	wantVersion := canonicalCapabilityVersionForTest(t, officialCapabilityRegistryVersion, base)
	if first.CapabilityVersion != wantVersion {
		t.Fatalf("capabilityVersion is not the SHA-256 of canonical input JSON:\n got: %s\nwant: %s", first.CapabilityVersion, wantVersion)
	}
	if decoded, err := hex.DecodeString(first.CapabilityVersion); err != nil || len(decoded) != sha256.Size {
		t.Fatalf("capabilityVersion is not a SHA-256 hex digest: value=%q bytes=%d err=%v", first.CapabilityVersion, len(decoded), err)
	}

	mutations := []struct {
		name   string
		mutate func(*config.GatewayContractConfig)
	}{
		{"name", func(c *config.GatewayContractConfig) { c.Name = "other-contract" }},
		{"version", func(c *config.GatewayContractConfig) { c.Version = "99" }},
		{"declared modes", func(c *config.GatewayContractConfig) { c.DeclaredModes = append(c.DeclaredModes, "preview") }},
		{"duration name", func(c *config.GatewayContractConfig) { c.Duration.Name = "duration_seconds" }},
		{"duration type", func(c *config.GatewayContractConfig) { c.Duration.ValueType = "string" }},
		{"duration map", func(c *config.GatewayContractConfig) { c.Duration.ValueMap["smart"] = "smart" }},
		{"aspect name", func(c *config.GatewayContractConfig) { c.AspectRatio.Name = "ratio" }},
		{"aspect type", func(c *config.GatewayContractConfig) { c.AspectRatio.ValueType = "int" }},
		{"aspect map", func(c *config.GatewayContractConfig) { c.AspectRatio.ValueMap = map[string]string{"16:9": "wide"} }},
		{"resolution name", func(c *config.GatewayContractConfig) { c.Resolution.Name = "size" }},
		{"resolution type", func(c *config.GatewayContractConfig) { c.Resolution.ValueType = "int" }},
		{"resolution map", func(c *config.GatewayContractConfig) { c.Resolution.ValueMap["4K"] = "uhd" }},
		{"audio name", func(c *config.GatewayContractConfig) { c.GenerateAudio.Name = "audio" }},
		{"audio type", func(c *config.GatewayContractConfig) { c.GenerateAudio.ValueType = "string" }},
		{"audio map", func(c *config.GatewayContractConfig) { c.GenerateAudio.ValueMap = map[string]string{"true": "yes"} }},
		{"task mode name", func(c *config.GatewayContractConfig) { c.TaskMode.Name = "mode" }},
		{"task mode type", func(c *config.GatewayContractConfig) { c.TaskMode.ValueType = "int" }},
		{"task mode map", func(c *config.GatewayContractConfig) { c.TaskMode.ValueMap = map[string]string{"edit": "replace"} }},
		{"reference mode", func(c *config.GatewayContractConfig) { c.References.Mode = "role_fields" }},
		{"image field", func(c *config.GatewayContractConfig) { c.References.ImageField = "image" }},
		{"video field", func(c *config.GatewayContractConfig) { c.References.VideoField = "video" }},
		{"audio field", func(c *config.GatewayContractConfig) { c.References.AudioField = "audio" }},
		{"role fields", func(c *config.GatewayContractConfig) { c.References.RoleFields["edit_target"] = "target" }},
		{"supported roles", func(c *config.GatewayContractConfig) {
			c.References.SupportsRoles = c.References.SupportsRoles[:len(c.References.SupportsRoles)-1]
		}},
		{"target order", func(c *config.GatewayContractConfig) { c.References.RequiresTargetFirst = false }},
		{"max images", func(c *config.GatewayContractConfig) { c.Limits.MaxImages-- }},
		{"max videos", func(c *config.GatewayContractConfig) { c.Limits.MaxVideos-- }},
		{"max audios", func(c *config.GatewayContractConfig) { c.Limits.MaxAudios-- }},
		{"max video seconds", func(c *config.GatewayContractConfig) { c.Limits.MaxVideoSecondsTotal-- }},
		{"max audio seconds", func(c *config.GatewayContractConfig) { c.Limits.MaxAudioSecondsTotal-- }},
		{"idempotency header", func(c *config.GatewayContractConfig) { c.Idempotency.Header = "Idempotency-Key" }},
		{"reconcile lookup", func(c *config.GatewayContractConfig) { c.Reconciliation.LookupByRequestKey = false }},
		{"reconcile method", func(c *config.GatewayContractConfig) { c.Reconciliation.Method = "HEAD" }},
		{"reconcile path", func(c *config.GatewayContractConfig) { c.Reconciliation.PathTemplate = "/v2/tasks/{requestKey}" }},
		{"task ID paths", func(c *config.GatewayContractConfig) {
			c.Reconciliation.TaskIDPaths = append(c.Reconciliation.TaskIDPaths, "id")
		}},
		{"status paths", func(c *config.GatewayContractConfig) {
			c.Reconciliation.StatusPaths = append(c.Reconciliation.StatusPaths, "state")
		}},
	}

	for _, tt := range mutations {
		t.Run(tt.name, func(t *testing.T) {
			contract := cloneTestContract(baseContract)
			tt.mutate(&contract)
			got := ResolveCapabilities(CapabilityConfig{Model: base.Model, ModelProfile: base.ModelProfile, GatewayContract: contract})
			if got.CapabilityVersion == first.CapabilityVersion {
				t.Fatalf("contract mutation %q did not change capabilityVersion", tt.name)
			}
		})
	}

	if got := ResolveCapabilities(CapabilityConfig{Model: "other-private-video", ModelProfile: base.ModelProfile, GatewayContract: baseContract}); got.CapabilityVersion == first.CapabilityVersion {
		t.Fatal("changing model must change capabilityVersion")
	}
	if got := ResolveCapabilities(CapabilityConfig{Model: base.Model, ModelProfile: "fast", GatewayContract: baseContract}); got.CapabilityVersion == first.CapabilityVersion {
		t.Fatal("changing explicit profile must change capabilityVersion")
	}

	left := cloneTestContract(baseContract)
	left.Resolution.ValueMap = map[string]string{"480P": "480p", "720P": "720p", "1080P": "1080p", "4K": "4k"}
	right := cloneTestContract(baseContract)
	right.Resolution.ValueMap = make(map[string]string)
	right.Resolution.ValueMap["4K"] = "4k"
	right.Resolution.ValueMap["1080P"] = "1080p"
	right.Resolution.ValueMap["720P"] = "720p"
	right.Resolution.ValueMap["480P"] = "480p"
	leftVersion := ResolveCapabilities(CapabilityConfig{Model: base.Model, ModelProfile: base.ModelProfile, GatewayContract: left}).CapabilityVersion
	rightVersion := ResolveCapabilities(CapabilityConfig{Model: base.Model, ModelProfile: base.ModelProfile, GatewayContract: right}).CapabilityVersion
	if leftVersion != rightVersion {
		t.Fatalf("canonical map ordering changed version: %q vs %q", leftVersion, rightVersion)
	}
}

func TestResolveCapabilitiesReturnsCopiesOfOfficialProfiles(t *testing.T) {
	input := CapabilityConfig{Model: "video-ds-2.0", GatewayContract: configuredSeedanceContract()}
	first := ResolveCapabilities(input)
	first.SupportedDurations[0] = 99
	first.AspectRatios[0] = "mutated"
	first.Resolutions[0] = "mutated"
	first.TaskModes[0] = "mutated"
	first.ReferenceRoles[0] = "mutated"

	second := ResolveCapabilities(input)
	if !reflect.DeepEqual(second.SupportedDurations, seedanceDurationValues) {
		t.Fatalf("official duration profile was mutated: %#v", second.SupportedDurations)
	}
	if !reflect.DeepEqual(second.AspectRatios, seedanceAspectRatios) {
		t.Fatalf("official aspect profile was mutated: %#v", second.AspectRatios)
	}
	if second.Resolutions[0] != "480P" || second.TaskModes[0] != "reference" || second.ReferenceRoles[0] != "reference_image" {
		t.Fatalf("official profile slices were aliased: %+v", second)
	}
}

func configuredSeedanceContract() config.GatewayContractConfig {
	return config.GatewayContractConfig{
		Name:          "seedance2_configured_v1",
		Version:       "3",
		DeclaredModes: []string{"reference", "edit", "extend"},
		Duration: config.FieldEncoding{
			Name:      "duration",
			ValueType: "int",
			ValueMap:  map[string]string{"smart": "-1"},
		},
		AspectRatio: config.FieldEncoding{Name: "aspect_ratio", ValueType: "string"},
		Resolution: config.FieldEncoding{
			Name:      "resolution",
			ValueType: "string",
			ValueMap: map[string]string{
				"480P":  "480p",
				"720P":  "720p",
				"1080P": "1080p",
				"4K":    "4k",
			},
		},
		GenerateAudio: config.FieldEncoding{Name: "generate_audio", ValueType: "bool"},
		TaskMode:      config.FieldEncoding{Name: "task_mode", ValueType: "string"},
		References: config.ReferenceEncoding{
			Mode:       "content_items",
			ImageField: "image_url",
			VideoField: "video_url",
			AudioField: "audio_url",
			RoleFields: map[string]string{
				"reference_image": "reference_image",
				"first_frame":     "first_frame",
				"last_frame":      "last_frame",
				"reference_video": "reference_video",
				"reference_audio": "reference_audio",
				"edit_target":     "edit_target",
				"extend_target":   "extend_target",
			},
			SupportsRoles:       append([]string(nil), seedanceReferenceRoles...),
			RequiresTargetFirst: true,
		},
		Limits: config.MediaLimits{
			MaxImages:            9,
			MaxVideos:            3,
			MaxAudios:            3,
			MaxVideoSecondsTotal: 15,
			MaxAudioSecondsTotal: 15,
		},
		Idempotency: config.IdempotencyContract{Header: "X-Request-Key"},
		Reconciliation: config.ReconciliationContract{
			LookupByRequestKey: true,
			Method:             "GET",
			PathTemplate:       "/v1/videos/by-request/{requestKey}",
			TaskIDPaths:        []string{"data.task_id", "task_id"},
			StatusPaths:        []string{"data.status", "status"},
		},
	}
}

func cloneTestContract(in config.GatewayContractConfig) config.GatewayContractConfig {
	out := in
	out.DeclaredModes = append([]string(nil), in.DeclaredModes...)
	out.Duration.ValueMap = cloneTestStringMap(in.Duration.ValueMap)
	out.AspectRatio.ValueMap = cloneTestStringMap(in.AspectRatio.ValueMap)
	out.Resolution.ValueMap = cloneTestStringMap(in.Resolution.ValueMap)
	out.GenerateAudio.ValueMap = cloneTestStringMap(in.GenerateAudio.ValueMap)
	out.TaskMode.ValueMap = cloneTestStringMap(in.TaskMode.ValueMap)
	out.References.RoleFields = cloneTestStringMap(in.References.RoleFields)
	out.References.SupportsRoles = append([]string(nil), in.References.SupportsRoles...)
	out.Reconciliation.TaskIDPaths = append([]string(nil), in.Reconciliation.TaskIDPaths...)
	out.Reconciliation.StatusPaths = append([]string(nil), in.Reconciliation.StatusPaths...)
	return out
}

func cloneTestStringMap(in map[string]string) map[string]string {
	if in == nil {
		return nil
	}
	out := make(map[string]string, len(in))
	for key, value := range in {
		out[key] = value
	}
	return out
}

func canonicalCapabilityVersionForTest(t *testing.T, profileVersion string, input CapabilityConfig) string {
	t.Helper()
	contract := config.TrimGatewayContract(input.GatewayContract)
	contractJSON, err := json.Marshal(contract)
	if err != nil {
		t.Fatal(err)
	}
	var contractBody any
	if err := json.Unmarshal(contractJSON, &contractBody); err != nil {
		t.Fatal(err)
	}
	payload := map[string]any{
		"explicitProfile":        input.ModelProfile,
		"gatewayContractBody":    contractBody,
		"gatewayContractName":    contract.Name,
		"gatewayContractVersion": contract.Version,
		"model":                  input.Model,
		"officialProfileVersion": profileVersion,
	}
	canonical, err := json.Marshal(payload)
	if err != nil {
		t.Fatal(err)
	}
	digest := sha256.Sum256(canonical)
	return hex.EncodeToString(digest[:])
}

func assertDegradation(t *testing.T, got Capabilities, feature, reason string) {
	t.Helper()
	for _, degradation := range got.Degradations {
		if degradation.Feature == feature {
			if degradation.Reason != reason {
				t.Fatalf("degradation %q reason = %q, want %q", feature, degradation.Reason, reason)
			}
			return
		}
	}
	t.Fatalf("missing degradation %q=%q in %#v", feature, reason, got.Degradations)
}

func assertExactDegradations(t *testing.T, got Capabilities, want map[string]string) {
	t.Helper()
	seen := make(map[string]string, len(got.Degradations))
	for _, degradation := range got.Degradations {
		if _, exists := seen[degradation.Feature]; exists {
			t.Fatalf("duplicate degradation for %q: %#v", degradation.Feature, got.Degradations)
		}
		seen[degradation.Feature] = degradation.Reason
	}
	if !reflect.DeepEqual(seen, want) {
		t.Fatalf("degradations mismatch:\n got: %#v\nwant: %#v", seen, want)
	}
}

func assertNoDegradationPrefix(t *testing.T, got Capabilities, prefix string) {
	t.Helper()
	for _, degradation := range got.Degradations {
		if strings.HasPrefix(degradation.Feature, prefix) {
			t.Fatalf("unexpected degradation with prefix %q: %+v", prefix, degradation)
		}
	}
}

func assertNoDegradation(t *testing.T, got Capabilities, feature string) {
	t.Helper()
	for _, degradation := range got.Degradations {
		if degradation.Feature == feature {
			t.Fatalf("unexpected degradation for %q: %+v", feature, degradation)
		}
	}
}
