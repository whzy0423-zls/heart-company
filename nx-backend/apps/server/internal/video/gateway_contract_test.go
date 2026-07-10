package video

import (
	"encoding/json"
	"reflect"
	"strings"
	"testing"

	"nine-xing/nx-backend/apps/server/internal/config"
)

func TestMapLegacyGatewayPayload(t *testing.T) {
	generateAudio := true
	canonical, err := CanonicalizeReferences([]Reference{
		{ID: "50", Kind: "video", Role: "reference_video", URL: "v2", SortOrder: 5},
		{ID: "20", Kind: "image", Role: "reference_image", URL: "i2", SortOrder: 2},
		{ID: "40", Kind: "audio", Role: "reference_audio", URL: "a1", SortOrder: 4},
		{ID: "10", Kind: "image", Role: "reference_image", URL: "i1", SortOrder: 1},
		{ID: "30", Kind: "video", Role: "reference_video", URL: "v1", SortOrder: 3},
	})
	if err != nil {
		t.Fatalf("CanonicalizeReferences() error = %v", err)
	}

	payload, err := MapGatewayPayload(GenerateRequest{
		Model:         "video-ds-2.0-fast",
		Prompt:        "雨夜车站",
		Duration:      15,
		AspectRatio:   "9:16",
		Resolution:    "1080P",
		GenerateAudio: &generateAudio,
		TaskMode:      "reference",
		RequestKey:    "must-not-enter-body",
	}, canonical, LegacyFlatContract())
	if err != nil {
		t.Fatalf("MapGatewayPayload() error = %v", err)
	}
	raw, err := json.Marshal(payload)
	if err != nil {
		t.Fatalf("json.Marshal() error = %v", err)
	}

	const want = `{"aspect_ratio":"9:16","audios":["a1"],"images":["i1","i2"],"model":"video-ds-2.0-fast","prompt":"雨夜车站","seconds":"15","videos":["v1","v2"]}`
	if string(raw) != want {
		t.Fatalf("legacy payload = %s, want %s", raw, want)
	}
}

func TestMapConfiguredGatewayPayloadUsesDeclaredFieldsAndCanonicalReferences(t *testing.T) {
	generateAudio := true
	rawReferences := []Reference{
		{ID: "10", Kind: "video", Role: "reference_video", URL: "shared", SortOrder: 0},
		{ID: "20", Kind: "image", Role: "first_frame", URL: "i1", SortOrder: 1},
		{ID: "30", Kind: "video", Role: "edit_target", URL: "shared", SortOrder: 2},
		{ID: "40", Kind: "audio", Role: "reference_audio", URL: "a1", SortOrder: 3},
	}
	canonical, err := CanonicalizeReferences(rawReferences)
	if err != nil {
		t.Fatalf("CanonicalizeReferences() error = %v", err)
	}
	if got := canonicalLabels(canonical); !reflect.DeepEqual(got, []string{"视频1", "图片1", "视频2", "音频1"}) {
		t.Fatalf("canonical labels = %#v", got)
	}

	contract := configuredMapperContract()
	contract.References.RequiresTargetFirst = false
	prompt := "参考视频1的运镜，严格编辑视频2，参考图片1的首帧和音频1的声音"
	payload, err := MapGatewayPayload(GenerateRequest{
		Model:             "video-ds-2.0",
		Prompt:            prompt,
		Duration:          12,
		AspectRatio:       "9:16",
		Resolution:        "1080P",
		GenerateAudio:     &generateAudio,
		TaskMode:          "edit",
		References:        rawReferences,
		RequestKey:        "must-not-enter-body",
		CapabilityVersion: "must-not-enter-body",
	}, canonical, contract)
	if err != nil {
		t.Fatalf("MapGatewayPayload() error = %v", err)
	}
	raw, err := json.Marshal(payload)
	if err != nil {
		t.Fatalf("json.Marshal() error = %v", err)
	}

	const want = `{"aspect_ratio":"portrait","content_items":[{"reference_video":true,"video_url":"shared"},{"first_frame":true,"image_url":"i1"},{"edit_target":true,"video_url":"shared"},{"audio_url":"a1","reference_audio":true}],"duration_seconds":12,"model":"video-ds-2.0","operation":"replace","prompt":"参考视频1的运镜，严格编辑视频2，参考图片1的首帧和音频1的声音","resolution_code":"full_hd","sound_enabled":false}`
	if string(raw) != want {
		t.Fatalf("configured payload = %s, want %s", raw, want)
	}
}

func TestMapConfiguredGatewayPayloadKeepsDeclaredFieldNameLiteral(t *testing.T) {
	contract := configuredMapperContract()
	contract.Resolution.Name = "options.resolution"
	payload, err := MapGatewayPayload(GenerateRequest{
		Model:       "video-ds-2.0",
		Prompt:      "测试",
		Duration:    12,
		AspectRatio: "9:16",
		Resolution:  "1080P",
		TaskMode:    "reference",
	}, mustCanonicalReferences(t, nil), contract)
	if err != nil {
		t.Fatalf("MapGatewayPayload() error = %v", err)
	}
	if got := payload["options.resolution"]; got != "full_hd" {
		t.Fatalf("literal declared key value = %#v, want full_hd", got)
	}
	if _, nested := payload["options"]; nested {
		t.Fatalf("mapper constructed an undeclared nested object: %#v", payload["options"])
	}
}

func TestMapGatewayPayloadAcceptsZeroCanonicalReferencesAsEmpty(t *testing.T) {
	_, err := MapGatewayPayload(GenerateRequest{
		Model:       "video-ds-2.0",
		Prompt:      "测试",
		Duration:    12,
		AspectRatio: "9:16",
		TaskMode:    "reference",
	}, CanonicalReferences{}, configuredMapperContract())
	if err != nil {
		t.Fatalf("zero canonical references should represent an empty set: %v", err)
	}
}

func TestMapGatewayPayloadUsesSharedFieldEncodingSemantics(t *testing.T) {
	generateAudio := true
	contract := configuredMapperContract()
	contract.Duration = config.FieldEncoding{Name: "duration", ValueType: "string", ValueMap: map[string]string{"smart": "auto"}}
	contract.AspectRatio = config.FieldEncoding{Name: "aspect", ValueType: "int", ValueMap: map[string]string{"9:16": "916"}}
	contract.Resolution = config.FieldEncoding{Name: "resolution", ValueType: "int", ValueMap: map[string]string{"1080P": "1080"}}
	contract.GenerateAudio = config.FieldEncoding{Name: "audio", ValueType: "string", ValueMap: map[string]string{"true": "on", "false": "off"}}
	contract.TaskMode = config.FieldEncoding{Name: "mode", ValueType: "int", ValueMap: map[string]string{"reference": "1", "edit": "2", "extend": "3"}}

	payload, err := MapGatewayPayload(GenerateRequest{
		Model:         "video-ds-2.0",
		Prompt:        "测试",
		Duration:      -1,
		AspectRatio:   "9:16",
		Resolution:    "1080P",
		GenerateAudio: &generateAudio,
		TaskMode:      "reference",
	}, mustCanonicalReferences(t, nil), contract)
	if err != nil {
		t.Fatalf("MapGatewayPayload() error = %v", err)
	}
	for field, want := range map[string]any{
		"duration":   "auto",
		"aspect":     916,
		"resolution": 1080,
		"audio":      "on",
		"mode":       1,
	} {
		if got := payload[field]; !reflect.DeepEqual(got, want) {
			t.Fatalf("payload[%q] = %#v, want %#v", field, got, want)
		}
	}
}

func TestMapGatewayPayloadUsesSharedBoolParsing(t *testing.T) {
	generateAudio := true
	contract := configuredMapperContract()
	contract.GenerateAudio = config.FieldEncoding{
		Name:      "audio",
		ValueType: "bool",
		ValueMap:  map[string]string{"true": "1", "false": "0"},
	}

	payload, err := MapGatewayPayload(GenerateRequest{
		Model:         "video-ds-2.0",
		Prompt:        "测试",
		Duration:      12,
		AspectRatio:   "9:16",
		GenerateAudio: &generateAudio,
		TaskMode:      "reference",
	}, mustCanonicalReferences(t, nil), contract)
	if err != nil {
		t.Fatalf("MapGatewayPayload() error = %v", err)
	}
	if got := payload["audio"]; got != true {
		t.Fatalf("payload audio = %#v, want true", got)
	}
}

func TestMapGatewayPayloadUsesDirectStringValueWhenNoOverrideExists(t *testing.T) {
	contract := configuredMapperContract()
	contract.Resolution.ValueMap = nil
	payload, err := MapGatewayPayload(GenerateRequest{
		Model:       "video-ds-2.0",
		Prompt:      "测试",
		Duration:    12,
		AspectRatio: "9:16",
		Resolution:  "1080P",
		TaskMode:    "reference",
	}, CanonicalReferences{}, contract)
	if err != nil {
		t.Fatalf("MapGatewayPayload() error = %v", err)
	}
	if got := payload[contract.Resolution.Name]; got != "1080P" {
		t.Fatalf("direct resolution = %#v, want 1080P", got)
	}
}

func TestMapConfiguredGatewayPayloadRejectsUndeclaredRole(t *testing.T) {
	contract := configuredMapperContract()
	delete(contract.References.RoleFields, "first_frame")
	canonical := mustCanonicalReferences(t, []Reference{
		{ID: "1", Kind: "image", Role: "first_frame", URL: "i1"},
	})

	err := mapGatewayPayloadError(GenerateRequest{
		Model:       "video-ds-2.0",
		Prompt:      "测试",
		Duration:    12,
		AspectRatio: "9:16",
		TaskMode:    "reference",
	}, canonical, contract)
	validationErr := assertValidationCode(t, err, "gateway_reference_role_unsupported")
	if validationErr.Field != "references[0].role" {
		t.Fatalf("validation field = %q, want references[0].role", validationErr.Field)
	}
}

func TestTargetFirstContractRejectsEditTargetAtVideo2(t *testing.T) {
	contract := configuredMapperContract()
	contract.References.RequiresTargetFirst = true
	canonical := mustCanonicalReferences(t, []Reference{
		{ID: "1", Kind: "image", Role: "first_frame", URL: "i1", SortOrder: 0},
		{ID: "2", Kind: "video", Role: "reference_video", URL: "v1", SortOrder: 1},
		{ID: "3", Kind: "video", Role: "edit_target", URL: "v2", SortOrder: 2},
	})

	err := mapGatewayPayloadError(GenerateRequest{
		Model:       "video-ds-2.0",
		Prompt:      "严格编辑视频2",
		Duration:    12,
		AspectRatio: "9:16",
		TaskMode:    "edit",
	}, canonical, contract)
	validationErr := assertValidationCode(t, err, "gateway_target_not_first")
	if validationErr.Field != "references[2]" || !strings.Contains(validationErr.Message, "视频2") {
		t.Fatalf("target-first error must identify 视频2: %+v", validationErr)
	}
}

func TestMapGatewayPayloadRejectsStaleCanonicalReferences(t *testing.T) {
	duration := 6.5
	rawReferences := []Reference{
		{ID: "1", Kind: "video", Role: "reference_video", URL: "v1", SortOrder: 1, DurationSeconds: &duration},
		{ID: "2", Kind: "video", Role: "edit_target", URL: "v2", SortOrder: 2},
	}
	base := mustCanonicalReferences(t, rawReferences)

	tests := []struct {
		name   string
		mutate func(*CanonicalReferences)
	}{
		{
			name: "forged target ordinal",
			mutate: func(references *CanonicalReferences) {
				references.References[1].Ordinal = 1
			},
		},
		{
			name: "swapped items",
			mutate: func(references *CanonicalReferences) {
				references.References[0], references.References[1] = references.References[1], references.References[0]
			},
		},
		{
			name: "modified label",
			mutate: func(references *CanonicalReferences) {
				references.References[0].Label = "视频9"
			},
		},
		{
			name: "modified duration value",
			mutate: func(references *CanonicalReferences) {
				forged := 9.5
				references.References[0].DurationSeconds = &forged
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			forged := cloneCanonicalReferencesForTest(base)
			tt.mutate(&forged)
			contract := configuredMapperContract()
			contract.References.RequiresTargetFirst = true

			err := mapGatewayPayloadError(GenerateRequest{
				Model:       "video-ds-2.0",
				Prompt:      "严格编辑视频2",
				Duration:    12,
				AspectRatio: "9:16",
				TaskMode:    "edit",
				References:  rawReferences,
			}, forged, contract)
			validationErr := assertValidationCode(t, err, "canonical_references_stale")
			if validationErr.Field != "references" {
				t.Fatalf("validation field = %q, want references", validationErr.Field)
			}
		})
	}
}

func TestMapGatewayPayloadEnforcesTargetPredicatesAtMapperBoundary(t *testing.T) {
	tests := []struct {
		name      string
		mode      string
		refs      []Reference
		wantCode  string
		wantField string
	}{
		{
			name:      "reference rejects edit target",
			mode:      "reference",
			refs:      []Reference{{ID: "1", Kind: "video", Role: "edit_target", URL: "v1"}},
			wantCode:  "target_role_not_allowed",
			wantField: "references",
		},
		{
			name:      "reference rejects extend target",
			mode:      "reference",
			refs:      []Reference{{ID: "1", Kind: "video", Role: "extend_target", URL: "v1"}},
			wantCode:  "target_role_not_allowed",
			wantField: "references",
		},
		{
			name:      "edit requires target",
			mode:      "edit",
			wantCode:  "edit_target_required",
			wantField: "references",
		},
		{
			name: "edit rejects duplicate targets",
			mode: "edit",
			refs: []Reference{
				{ID: "1", Kind: "video", Role: "edit_target", URL: "v1"},
				{ID: "2", Kind: "video", Role: "edit_target", URL: "v2"},
			},
			wantCode:  "multiple_edit_targets",
			wantField: "references",
		},
		{
			name: "mixed targets are rejected",
			mode: "edit",
			refs: []Reference{
				{ID: "1", Kind: "video", Role: "edit_target", URL: "v1"},
				{ID: "2", Kind: "video", Role: "extend_target", URL: "v2"},
			},
			wantCode:  "mixed_target_roles",
			wantField: "references",
		},
		{
			name:      "extend requires target",
			mode:      "extend",
			wantCode:  "extend_target_required",
			wantField: "references",
		},
		{
			name: "extend rejects duplicate targets",
			mode: "extend",
			refs: []Reference{
				{ID: "1", Kind: "video", Role: "extend_target", URL: "v1"},
				{ID: "2", Kind: "video", Role: "extend_target", URL: "v2"},
			},
			wantCode:  "multiple_extend_targets",
			wantField: "references",
		},
		{
			name:      "role kind mismatch",
			mode:      "edit",
			refs:      []Reference{{ID: "1", Kind: "image", Role: "edit_target", URL: "i1"}},
			wantCode:  "reference_kind_role_mismatch",
			wantField: "references[0]",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			canonical := mustCanonicalReferences(t, tt.refs)
			err := mapGatewayPayloadError(GenerateRequest{
				Model:       "video-ds-2.0",
				Prompt:      "测试",
				Duration:    12,
				AspectRatio: "9:16",
				TaskMode:    tt.mode,
				References:  tt.refs,
			}, canonical, configuredMapperContract())
			validationErr := assertValidationCode(t, err, tt.wantCode)
			if validationErr.Field != tt.wantField {
				t.Fatalf("validation field = %q, want %q", validationErr.Field, tt.wantField)
			}
		})
	}
}

func TestMapConfiguredGatewayPayloadFailsClosed(t *testing.T) {
	baseRequest := GenerateRequest{
		Model:       "video-ds-2.0",
		Prompt:      "测试",
		Duration:    12,
		AspectRatio: "9:16",
		Resolution:  "1080P",
		TaskMode:    "reference",
	}

	tests := []struct {
		name      string
		mutate    func(*config.GatewayContractConfig)
		refs      []Reference
		mode      string
		wantCode  string
		wantField string
	}{
		{
			name: "unknown reference encoding mode",
			mutate: func(contract *config.GatewayContractConfig) {
				contract.References.Mode = "future_items"
			},
			wantCode:  "gateway_reference_encoding_unsupported",
			wantField: "gatewayContract.references.mode",
		},
		{
			name: "blank enum mapping",
			mutate: func(contract *config.GatewayContractConfig) {
				contract.Resolution.ValueMap["1080P"] = " \t "
			},
			wantCode:  "gateway_value_not_encodable",
			wantField: "resolution",
		},
		{
			name: "blank role field",
			mutate: func(contract *config.GatewayContractConfig) {
				contract.References.RoleFields["reference_video"] = " \t "
			},
			refs:      []Reference{{ID: "1", Kind: "video", Role: "reference_video", URL: "v1"}},
			wantCode:  "gateway_reference_role_unsupported",
			wantField: "references[0].role",
		},
		{
			name: "role field conflicts with media field",
			mutate: func(contract *config.GatewayContractConfig) {
				contract.References.RoleFields["reference_video"] = contract.References.VideoField
			},
			refs:      []Reference{{ID: "1", Kind: "video", Role: "reference_video", URL: "v1"}},
			wantCode:  "gateway_reference_field_conflict",
			wantField: "references[0]",
		},
		{
			name: "flat arrays cannot carry target role",
			mutate: func(contract *config.GatewayContractConfig) {
				contract.References.Mode = "flat_arrays"
				contract.References.VideoField = "videos"
			},
			refs:      []Reference{{ID: "1", Kind: "video", Role: "edit_target", URL: "v1"}},
			mode:      "edit",
			wantCode:  "gateway_reference_role_unsupported",
			wantField: "references[0].role",
		},
		{
			name: "flat arrays require role declaration",
			mutate: func(contract *config.GatewayContractConfig) {
				contract.References.Mode = "flat_arrays"
				contract.References.VideoField = "videos"
				contract.References.SupportsRoles = []string{"reference_image", "reference_audio"}
			},
			refs:      []Reference{{ID: "1", Kind: "video", Role: "reference_video", URL: "v1"}},
			wantCode:  "gateway_reference_role_unsupported",
			wantField: "references[0].role",
		},
		{
			name: "invalid declared bool value",
			mutate: func(contract *config.GatewayContractConfig) {
				contract.GenerateAudio.ValueMap["true"] = "yes"
			},
			wantCode:  "gateway_value_not_encodable",
			wantField: "generateAudio",
		},
		{
			name: "declared field collides with fixed model field",
			mutate: func(contract *config.GatewayContractConfig) {
				contract.Duration.Name = "model"
			},
			wantCode:  "gateway_field_conflict",
			wantField: "duration",
		},
		{
			name: "role does not match media kind",
			mutate: func(contract *config.GatewayContractConfig) {
			},
			refs:      []Reference{{ID: "1", Kind: "image", Role: "edit_target", URL: "i1"}},
			wantCode:  "reference_kind_role_mismatch",
			wantField: "references[0]",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			contract := configuredMapperContract()
			tt.mutate(&contract)
			request := baseRequest
			if tt.mode != "" {
				request.TaskMode = tt.mode
			}
			generateAudio := true
			request.GenerateAudio = &generateAudio
			canonical := mustCanonicalReferences(t, tt.refs)
			err := mapGatewayPayloadError(request, canonical, contract)
			validationErr := assertValidationCode(t, err, tt.wantCode)
			if validationErr.Field != tt.wantField {
				t.Fatalf("validation field = %q, want %q", validationErr.Field, tt.wantField)
			}
		})
	}
}

func configuredMapperContract() config.GatewayContractConfig {
	return config.GatewayContractConfig{
		Name:          "seedance2_configured_v1",
		Version:       "1",
		DeclaredModes: []string{"reference", "edit", "extend"},
		Duration: config.FieldEncoding{
			Name:      "duration_seconds",
			ValueType: "int",
			ValueMap:  map[string]string{"smart": "-1"},
		},
		AspectRatio: config.FieldEncoding{
			Name:      "aspect_ratio",
			ValueType: "string",
			ValueMap:  map[string]string{"9:16": "portrait"},
		},
		Resolution: config.FieldEncoding{
			Name:      "resolution_code",
			ValueType: "string",
			ValueMap:  map[string]string{"1080P": "full_hd"},
		},
		GenerateAudio: config.FieldEncoding{
			Name:      "sound_enabled",
			ValueType: "bool",
			ValueMap:  map[string]string{"true": "false", "false": "true"},
		},
		TaskMode: config.FieldEncoding{
			Name:      "operation",
			ValueType: "string",
			ValueMap:  map[string]string{"reference": "create", "edit": "replace", "extend": "append"},
		},
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
			SupportsRoles:       []string{"reference_image", "first_frame", "last_frame", "reference_video", "reference_audio", "edit_target", "extend_target"},
			RequiresTargetFirst: false,
		},
	}
}

func mustCanonicalReferences(t *testing.T, refs []Reference) CanonicalReferences {
	t.Helper()
	canonical, err := CanonicalizeReferences(refs)
	if err != nil {
		t.Fatalf("CanonicalizeReferences() error = %v", err)
	}
	return canonical
}

func cloneCanonicalReferencesForTest(in CanonicalReferences) CanonicalReferences {
	out := CanonicalReferences{References: make([]CanonicalReference, len(in.References))}
	copy(out.References, in.References)
	for index := range out.References {
		if out.References[index].DurationSeconds == nil {
			continue
		}
		duration := *out.References[index].DurationSeconds
		out.References[index].DurationSeconds = &duration
	}
	return out
}

func mapGatewayPayloadError(request GenerateRequest, references CanonicalReferences, contract config.GatewayContractConfig) error {
	_, err := MapGatewayPayload(request, references, contract)
	return err
}
