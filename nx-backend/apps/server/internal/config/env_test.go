package config

import (
	"errors"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
)

func TestLoadDefaultsVideoGatewayContract(t *testing.T) {
	t.Setenv("ENV_FILE", filepath.Join(t.TempDir(), "missing.env"))
	t.Setenv("VIDEO_MODEL_PROFILE", "")
	t.Setenv("VIDEO_GATEWAY_CONTRACT", "")
	t.Setenv("VIDEO_GATEWAY_CONTRACT_VERSION", "")
	t.Setenv("VIDEO_GATEWAY_CONTRACT_JSON", "")

	env := Load()

	if env.Video.GatewayContract.Name != "legacy_flat_v1" {
		t.Fatalf("got %#v", env.Video.GatewayContract)
	}
	if env.Video.GatewayContract.Version != "1" {
		t.Fatal("expected contract version 1")
	}
	if env.Video.GatewayContract.Limits.MaxVideos != 3 {
		t.Fatalf("documented intermediary limit is 3 videos, got %+v", env.Video.GatewayContract.Limits)
	}
}

func TestLegacyVideoGatewayContractReturnsIndependentCopies(t *testing.T) {
	first := LegacyVideoGatewayContract()
	second := LegacyVideoGatewayContract()
	if !reflect.DeepEqual(first, second) {
		t.Fatalf("legacy constructors differ:\nfirst: %#v\nsecond: %#v", first, second)
	}

	first.DeclaredModes[0] = "edit"
	first.References.SupportsRoles[0] = "first_frame"
	third := LegacyVideoGatewayContract()
	if !reflect.DeepEqual(second, third) {
		t.Fatalf("mutating one legacy contract polluted future copies:\nsecond: %#v\nthird: %#v", second, third)
	}
	if third.DeclaredModes[0] != "reference" || third.References.SupportsRoles[0] != "reference_image" {
		t.Fatalf("legacy constructor returned mutated values: %#v", third)
	}
}

func TestValidateGatewayContractRejectsInvalidModesAndNamespaces(t *testing.T) {
	tests := []struct {
		name      string
		contract  func() GatewayContractConfig
		mutate    func(*GatewayContractConfig)
		wantCode  string
		wantField string
	}{
		{
			name:     "unknown declared mode",
			contract: validContentItemsGatewayContractForTest,
			mutate: func(contract *GatewayContractConfig) {
				contract.DeclaredModes = []string{"reference", "future"}
			},
			wantCode:  "invalid_declared_mode",
			wantField: "declaredModes[1]",
		},
		{
			name:     "duplicate declared mode",
			contract: validContentItemsGatewayContractForTest,
			mutate: func(contract *GatewayContractConfig) {
				contract.DeclaredModes = []string{"reference", "edit", "edit"}
			},
			wantCode:  "duplicate_declared_mode",
			wantField: "declaredModes[2]",
		},
		{
			name:     "unknown reference mode",
			contract: validContentItemsGatewayContractForTest,
			mutate: func(contract *GatewayContractConfig) {
				contract.References.Mode = "future_items"
			},
			wantCode:  "invalid_reference_mode",
			wantField: "references.mode",
		},
		{
			name:     "duplicate supported role",
			contract: validContentItemsGatewayContractForTest,
			mutate: func(contract *GatewayContractConfig) {
				contract.References.SupportsRoles = append(contract.References.SupportsRoles, "reference_image")
			},
			wantCode:  "duplicate_reference_role",
			wantField: "references.supportsRoles[7]",
		},
		{
			name:     "scalar collides with fixed model",
			contract: validContentItemsGatewayContractForTest,
			mutate: func(contract *GatewayContractConfig) {
				contract.Duration.Name = "model"
			},
			wantCode:  "duplicate_gateway_field",
			wantField: "duration.name",
		},
		{
			name:     "scalar fields collide",
			contract: validContentItemsGatewayContractForTest,
			mutate: func(contract *GatewayContractConfig) {
				contract.AspectRatio.Name = contract.Duration.Name
			},
			wantCode:  "duplicate_gateway_field",
			wantField: "aspectRatio.name",
		},
		{
			name:     "scalar collides with fixed content items",
			contract: validContentItemsGatewayContractForTest,
			mutate: func(contract *GatewayContractConfig) {
				contract.TaskMode.Name = "content_items"
			},
			wantCode:  "duplicate_gateway_field",
			wantField: "taskMode.name",
		},
		{
			name:     "flat media collides with prompt",
			contract: LegacyVideoGatewayContract,
			mutate: func(contract *GatewayContractConfig) {
				contract.References.ImageField = "prompt"
			},
			wantCode:  "duplicate_gateway_field",
			wantField: "references.imageField",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			contract := tt.contract()
			tt.mutate(&contract)
			assertGatewayContractValidationError(t, ValidateGatewayContract(contract), tt.wantCode, tt.wantField)
		})
	}
}

func TestValidateGatewayContractEnforcesContentItemsSchema(t *testing.T) {
	tests := []struct {
		name      string
		mutate    func(*GatewayContractConfig)
		wantCode  string
		wantField string
	}{
		{
			name: "missing role field",
			mutate: func(contract *GatewayContractConfig) {
				delete(contract.References.RoleFields, "first_frame")
			},
			wantCode:  "reference_role_fields_mismatch",
			wantField: "references.roleFields",
		},
		{
			name: "extra role field",
			mutate: func(contract *GatewayContractConfig) {
				contract.References.SupportsRoles = contract.References.SupportsRoles[:6]
			},
			wantCode:  "reference_role_fields_mismatch",
			wantField: "references.roleFields",
		},
		{
			name: "duplicate role field values",
			mutate: func(contract *GatewayContractConfig) {
				contract.References.RoleFields["first_frame"] = contract.References.RoleFields["reference_image"]
			},
			wantCode:  "duplicate_reference_role_field",
			wantField: "references.roleFields.first_frame",
		},
		{
			name: "missing media field for role",
			mutate: func(contract *GatewayContractConfig) {
				contract.References.AudioField = ""
			},
			wantCode:  "missing_reference_media_field",
			wantField: "references.audioField",
		},
		{
			name: "media field conflicts with role field",
			mutate: func(contract *GatewayContractConfig) {
				contract.References.RoleFields["reference_video"] = contract.References.VideoField
			},
			wantCode:  "reference_field_conflict",
			wantField: "references.roleFields.reference_video",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			contract := validContentItemsGatewayContractForTest()
			tt.mutate(&contract)
			assertGatewayContractValidationError(t, ValidateGatewayContract(contract), tt.wantCode, tt.wantField)
		})
	}
}

func TestValidateGatewayContractEnforcesFlatArraysSchema(t *testing.T) {
	tests := []struct {
		name      string
		mutate    func(*GatewayContractConfig)
		wantCode  string
		wantField string
	}{
		{
			name: "role fields are forbidden",
			mutate: func(contract *GatewayContractConfig) {
				contract.References.RoleFields = map[string]string{"reference_image": "role"}
			},
			wantCode:  "flat_role_fields_not_allowed",
			wantField: "references.roleFields",
		},
		{
			name: "frame role is forbidden",
			mutate: func(contract *GatewayContractConfig) {
				contract.References.SupportsRoles = append(contract.References.SupportsRoles, "first_frame")
			},
			wantCode:  "flat_reference_role_not_supported",
			wantField: "references.supportsRoles[3]",
		},
		{
			name: "declared role requires media field",
			mutate: func(contract *GatewayContractConfig) {
				contract.References.VideoField = ""
			},
			wantCode:  "missing_reference_media_field",
			wantField: "references.videoField",
		},
		{
			name: "media fields must be distinct",
			mutate: func(contract *GatewayContractConfig) {
				contract.References.AudioField = contract.References.VideoField
			},
			wantCode:  "duplicate_reference_media_field",
			wantField: "references.audioField",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			contract := LegacyVideoGatewayContract()
			tt.mutate(&contract)
			assertGatewayContractValidationError(t, ValidateGatewayContract(contract), tt.wantCode, tt.wantField)
		})
	}
}

func TestEncodeGatewayFieldValueUsesSharedStringIntBoolSemantics(t *testing.T) {
	tests := []struct {
		name   string
		field  FieldEncoding
		source string
		want   any
	}{
		{
			name:   "direct string",
			field:  FieldEncoding{Name: "value", ValueType: "string"},
			source: "portrait",
			want:   "portrait",
		},
		{
			name:   "mapped smart string",
			field:  FieldEncoding{Name: "value", ValueType: "string", ValueMap: map[string]string{"smart": "auto"}},
			source: "smart",
			want:   "auto",
		},
		{
			name:   "direct integer",
			field:  FieldEncoding{Name: "value", ValueType: "int"},
			source: "12",
			want:   12,
		},
		{
			name:   "mapped integer",
			field:  FieldEncoding{Name: "value", ValueType: "int", ValueMap: map[string]string{"1080P": "1080"}},
			source: "1080P",
			want:   1080,
		},
		{
			name:   "direct bool",
			field:  FieldEncoding{Name: "value", ValueType: "bool"},
			source: "true",
			want:   true,
		},
		{
			name:   "mapped bool",
			field:  FieldEncoding{Name: "value", ValueType: "bool", ValueMap: map[string]string{"false": "1"}},
			source: "false",
			want:   true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := EncodeGatewayFieldValue(tt.field, tt.source)
			if err != nil {
				t.Fatalf("EncodeGatewayFieldValue() error = %v", err)
			}
			if !reflect.DeepEqual(got, tt.want) {
				t.Fatalf("EncodeGatewayFieldValue() = %#v, want %#v", got, tt.want)
			}
		})
	}
}

func TestEncodeGatewayFieldValueReturnsSafeTypedErrors(t *testing.T) {
	field := FieldEncoding{
		Name:      "value",
		ValueType: "int",
		ValueMap:  map[string]string{"private-source": "https://secret.example/token"},
	}
	_, err := EncodeGatewayFieldValue(field, "private-source")
	var validationErr *GatewayContractValidationError
	if !errors.As(err, &validationErr) {
		t.Fatalf("expected typed encoding error, got %T: %v", err, err)
	}
	if validationErr.Code != "field_value_not_encodable" || validationErr.Field != "value" {
		t.Fatalf("encoding error = code %q field %q", validationErr.Code, validationErr.Field)
	}
	if strings.Contains(err.Error(), "private-source") || strings.Contains(err.Error(), "secret.example") {
		t.Fatalf("encoding error leaked configured values: %v", err)
	}
}

func TestValidateGatewayContractEnforcesFieldEncodingSemantics(t *testing.T) {
	tests := []struct {
		name      string
		mutate    func(*GatewayContractConfig)
		wantCode  string
		wantField string
	}{
		{
			name: "empty value map key",
			mutate: func(contract *GatewayContractConfig) {
				contract.Duration.ValueMap = map[string]string{" ": "0"}
			},
			wantCode:  "empty_value_map_key",
			wantField: "duration.valueMap",
		},
		{
			name: "empty value map value",
			mutate: func(contract *GatewayContractConfig) {
				contract.Resolution.ValueMap = map[string]string{"1080P": " "}
			},
			wantCode:  "empty_value_map_value",
			wantField: "resolution.valueMap",
		},
		{
			name: "mapped integer must parse",
			mutate: func(contract *GatewayContractConfig) {
				contract.Duration.ValueMap = map[string]string{"smart": "auto"}
			},
			wantCode:  "invalid_mapped_value",
			wantField: "duration.valueMap",
		},
		{
			name: "mapped bool must parse",
			mutate: func(contract *GatewayContractConfig) {
				contract.GenerateAudio.ValueMap = map[string]string{"true": "enabled"}
			},
			wantCode:  "invalid_mapped_value",
			wantField: "generateAudio.valueMap",
		},
		{
			name: "bool results must differ",
			mutate: func(contract *GatewayContractConfig) {
				contract.GenerateAudio.ValueMap = map[string]string{"true": "true", "false": "1"}
			},
			wantCode:  "indistinguishable_boolean_mapping",
			wantField: "generateAudio.valueMap",
		},
		{
			name: "integer task mode must encode every declared mode",
			mutate: func(contract *GatewayContractConfig) {
				contract.TaskMode = FieldEncoding{Name: "task_mode", ValueType: "int", ValueMap: map[string]string{"reference": "1", "edit": "2"}}
			},
			wantCode:  "declared_mode_not_encodable",
			wantField: "taskMode",
		},
		{
			name: "string audio requires explicit boolean mappings",
			mutate: func(contract *GatewayContractConfig) {
				contract.GenerateAudio = FieldEncoding{Name: "generate_audio", ValueType: "string"}
			},
			wantCode:  "missing_boolean_mapping",
			wantField: "generateAudio.valueMap",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			contract := validContentItemsGatewayContractForTest()
			tt.mutate(&contract)
			assertGatewayContractValidationError(t, ValidateGatewayContract(contract), tt.wantCode, tt.wantField)
		})
	}
}

func TestValidateGatewayContractAcceptsSharedFieldEncodingSemantics(t *testing.T) {
	contract := validContentItemsGatewayContractForTest()
	contract.Duration = FieldEncoding{Name: "duration", ValueType: "string", ValueMap: map[string]string{"smart": "auto"}}
	contract.AspectRatio = FieldEncoding{Name: "aspect_ratio", ValueType: "int", ValueMap: map[string]string{"9:16": "916"}}
	contract.Resolution = FieldEncoding{Name: "resolution", ValueType: "int", ValueMap: map[string]string{"1080P": "1080"}}
	contract.GenerateAudio = FieldEncoding{Name: "generate_audio", ValueType: "string", ValueMap: map[string]string{"true": "on", "false": "off"}}
	contract.TaskMode = FieldEncoding{Name: "task_mode", ValueType: "int", ValueMap: map[string]string{"reference": "1", "edit": "2", "extend": "3"}}

	if err := ValidateGatewayContract(contract); err != nil {
		t.Fatalf("expected shared encoding contract to pass, got %v", err)
	}
}

func TestValidateGatewayContractRejectsDuplicateTaskModeEncodings(t *testing.T) {
	tests := []struct {
		name  string
		field FieldEncoding
	}{
		{
			name: "string",
			field: FieldEncoding{
				Name:      "task_mode",
				ValueType: "string",
				ValueMap:  map[string]string{"reference": "same", "edit": "same", "extend": "append"},
			},
		},
		{
			name: "int",
			field: FieldEncoding{
				Name:      "task_mode",
				ValueType: "int",
				ValueMap:  map[string]string{"reference": "1", "edit": "1", "extend": "2"},
			},
		},
		{
			name: "bool",
			field: FieldEncoding{
				Name:      "task_mode",
				ValueType: "bool",
				ValueMap:  map[string]string{"reference": "true", "edit": "1", "extend": "false"},
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			contract := validContentItemsGatewayContractForTest()
			contract.TaskMode = tt.field
			assertGatewayContractValidationError(t, ValidateGatewayContract(contract), "duplicate_task_mode_encoding", "taskMode")
		})
	}
}

func validContentItemsGatewayContractForTest() GatewayContractConfig {
	return GatewayContractConfig{
		Name:          "seedance2_configured_v1",
		Version:       "1",
		DeclaredModes: []string{"reference", "edit", "extend"},
		Duration:      FieldEncoding{Name: "duration", ValueType: "int", ValueMap: map[string]string{"smart": "-1"}},
		AspectRatio:   FieldEncoding{Name: "aspect_ratio", ValueType: "string"},
		Resolution:    FieldEncoding{Name: "resolution", ValueType: "string", ValueMap: map[string]string{"1080P": "1080p"}},
		GenerateAudio: FieldEncoding{Name: "generate_audio", ValueType: "bool"},
		TaskMode:      FieldEncoding{Name: "task_mode", ValueType: "string"},
		References: ReferenceEncoding{
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
			SupportsRoles: []string{"reference_image", "first_frame", "last_frame", "reference_video", "reference_audio", "edit_target", "extend_target"},
		},
	}
}

func assertGatewayContractValidationError(t *testing.T, err error, wantCode, wantField string) {
	t.Helper()
	var validationErr *GatewayContractValidationError
	if !errors.As(err, &validationErr) {
		t.Fatalf("expected GatewayContractValidationError, got %T: %v", err, err)
	}
	if validationErr.Code != wantCode || validationErr.Field != wantField {
		t.Fatalf("validation error = code %q field %q, want code %q field %q", validationErr.Code, validationErr.Field, wantCode, wantField)
	}
}

func TestLoadVideoGatewayContractFailsClosedForIncompleteIdentity(t *testing.T) {
	cases := []struct {
		name     string
		contract string
		version  string
		body     string
	}{
		{
			name: "body without name",
			body: `{"duration":{"name":"seconds","valueType":"int"}}`,
		},
		{
			name:     "name without version",
			contract: "configured_contract",
			body:     `{"duration":{"name":"seconds","valueType":"int"}}`,
		},
		{
			name:    "version without name",
			version: "2",
			body:    `{"duration":{"name":"seconds","valueType":"int"}}`,
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Setenv("ENV_FILE", filepath.Join(t.TempDir(), "missing.env"))
			t.Setenv("VIDEO_MODEL_PROFILE", "")
			t.Setenv("VIDEO_GATEWAY_CONTRACT", tc.contract)
			t.Setenv("VIDEO_GATEWAY_CONTRACT_VERSION", tc.version)
			t.Setenv("VIDEO_GATEWAY_CONTRACT_JSON", tc.body)

			env := Load()

			if !reflect.DeepEqual(env.Video.GatewayContract, GatewayContractConfig{}) {
				t.Fatalf("expected incomplete explicit contract to fail closed, got %#v", env.Video.GatewayContract)
			}
		})
	}
}

func TestLoadExplicitLegacyGatewayJSONNeverFallsBackToBuiltIn(t *testing.T) {
	tests := []struct {
		name string
		raw  string
	}{
		{name: "malformed JSON", raw: "{"},
		{name: "invalid parsed JSON", raw: `{"references":{"mode":"future_items"}}`},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Setenv("ENV_FILE", filepath.Join(t.TempDir(), "missing.env"))
			t.Setenv("VIDEO_GATEWAY_CONTRACT", "legacy_flat_v1")
			t.Setenv("VIDEO_GATEWAY_CONTRACT_VERSION", "1")
			t.Setenv("VIDEO_GATEWAY_CONTRACT_JSON", tt.raw)

			got := Load().Video.GatewayContract
			if reflect.DeepEqual(got, LegacyVideoGatewayContract()) {
				t.Fatal("explicit invalid JSON silently fell back to the built-in legacy contract")
			}
			if !reflect.DeepEqual(got, GatewayContractConfig{}) && ValidateGatewayContract(got) == nil {
				t.Fatalf("explicit invalid JSON produced a usable contract: %#v", got)
			}
		})
	}
}

func TestLoadExplicitLegacyIdentityWithoutJSONUsesBuiltIn(t *testing.T) {
	t.Setenv("ENV_FILE", filepath.Join(t.TempDir(), "missing.env"))
	t.Setenv("VIDEO_GATEWAY_CONTRACT", "legacy_flat_v1")
	t.Setenv("VIDEO_GATEWAY_CONTRACT_VERSION", "1")
	t.Setenv("VIDEO_GATEWAY_CONTRACT_JSON", "")

	got := Load().Video.GatewayContract
	if !reflect.DeepEqual(got, LegacyVideoGatewayContract()) {
		t.Fatalf("legacy identity without JSON = %#v", got)
	}
}

func TestLoadParsesVideoGatewayContract(t *testing.T) {
	t.Setenv("ENV_FILE", filepath.Join(t.TempDir(), "missing.env"))
	t.Setenv("VIDEO_MODEL_PROFILE", " standard ")
	t.Setenv("VIDEO_GATEWAY_CONTRACT", " seedance2_configured_v1 ")
	t.Setenv("VIDEO_GATEWAY_CONTRACT_VERSION", " 7 ")
	t.Setenv("VIDEO_GATEWAY_CONTRACT_JSON", `{
		"name":"ignored-json-name",
		"version":"ignored-json-version",
		"declaredModes":["reference","edit","extend"],
		"duration":{"name":"content.duration","valueType":"int","valueMap":{"smart":"-1"}},
		"aspectRatio":{"name":"content.aspect_ratio","valueType":"string"},
		"resolution":{"name":"content.resolution","valueType":"string","valueMap":{"1080P":"1080p"}},
		"generateAudio":{"name":"content.generate_audio","valueType":"bool"},
		"taskMode":{"name":"content.task_mode","valueType":"string"},
		"references":{
			"mode":"content_items",
			"imageField":"image_url",
			"videoField":"video_url",
			"audioField":"audio_url",
			"roleFields":{
				"reference_image":"reference_image",
				"first_frame":"first_frame",
				"last_frame":"last_frame",
				"reference_video":"reference_video",
				"reference_audio":"reference_audio",
				"edit_target":"edit_target",
				"extend_target":"extend_target"
			},
			"supportsRoles":["reference_image","first_frame","last_frame","reference_video","reference_audio","edit_target","extend_target"],
			"requiresTargetFirst":true
		},
		"limits":{"maxImages":9,"maxVideos":3,"maxAudios":3,"maxVideoSecondsTotal":15,"maxAudioSecondsTotal":15},
		"idempotency":{"header":"X-Request-Key"},
		"reconciliation":{
			"lookupByRequestKey":true,
			"method":"GET",
			"pathTemplate":"/v1/videos/by-request/{requestKey}",
			"taskIdPaths":["data.task_id","task_id"],
			"statusPaths":["data.status","status"]
		}
	}`)

	env := Load()

	want := GatewayContractConfig{
		Name:          "seedance2_configured_v1",
		Version:       "7",
		DeclaredModes: []string{"reference", "edit", "extend"},
		Duration: FieldEncoding{
			Name:      "content.duration",
			ValueType: "int",
			ValueMap:  map[string]string{"smart": "-1"},
		},
		AspectRatio: FieldEncoding{Name: "content.aspect_ratio", ValueType: "string"},
		Resolution: FieldEncoding{
			Name:      "content.resolution",
			ValueType: "string",
			ValueMap:  map[string]string{"1080P": "1080p"},
		},
		GenerateAudio: FieldEncoding{Name: "content.generate_audio", ValueType: "bool"},
		TaskMode:      FieldEncoding{Name: "content.task_mode", ValueType: "string"},
		References: ReferenceEncoding{
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
			RequiresTargetFirst: true,
		},
		Limits: MediaLimits{
			MaxImages:            9,
			MaxVideos:            3,
			MaxAudios:            3,
			MaxVideoSecondsTotal: 15,
			MaxAudioSecondsTotal: 15,
		},
		Idempotency: IdempotencyContract{Header: "X-Request-Key"},
		Reconciliation: ReconciliationContract{
			LookupByRequestKey: true,
			Method:             "GET",
			PathTemplate:       "/v1/videos/by-request/{requestKey}",
			TaskIDPaths:        []string{"data.task_id", "task_id"},
			StatusPaths:        []string{"data.status", "status"},
		},
	}
	if env.Video.ModelProfile != "standard" {
		t.Fatalf("expected trimmed model profile, got %q", env.Video.ModelProfile)
	}
	if !reflect.DeepEqual(env.Video.GatewayContract, want) {
		t.Fatalf("unexpected gateway contract:\n got: %#v\nwant: %#v", env.Video.GatewayContract, want)
	}
}

func TestLoadFailsClosedForUnsafeVideoGatewayContractJSON(t *testing.T) {
	t.Setenv("ENV_FILE", filepath.Join(t.TempDir(), "missing.env"))
	t.Setenv("VIDEO_GATEWAY_CONTRACT", "custom_contract")
	t.Setenv("VIDEO_GATEWAY_CONTRACT_VERSION", "2")
	t.Setenv("VIDEO_GATEWAY_CONTRACT_JSON", `{
		"duration":{"name":"content[0]","valueType":"int"},
		"resolution":{"name":"resolution","valueType":"string"}
	}`)

	env := Load()

	if env.Video.GatewayContract.Name != "custom_contract" || env.Video.GatewayContract.Version != "2" {
		t.Fatalf("expected selected contract identity to remain visible, got %#v", env.Video.GatewayContract)
	}
	if env.Video.GatewayContract.Duration.Name != "" || env.Video.GatewayContract.Resolution.Name != "" {
		t.Fatalf("expected unsafe configured fields to fail closed, got %#v", env.Video.GatewayContract)
	}
}

func TestLoadMiniappChatDefaults(t *testing.T) {
	t.Setenv("MINIAPP_CHAT_RATE_LIMIT_PER_MINUTE", "")
	t.Setenv("MINIAPP_CHAT_TIMEOUT_SECONDS", "")

	env := Load()

	if env.MiniappChat.RateLimitPerMinute != 12 {
		t.Fatalf("expected default chat rate limit 12, got %d", env.MiniappChat.RateLimitPerMinute)
	}
	if env.MiniappChat.TimeoutSeconds != 28 {
		t.Fatalf("expected default chat timeout 28, got %d", env.MiniappChat.TimeoutSeconds)
	}
}

func TestLoadMiniappChatOverrides(t *testing.T) {
	t.Setenv("MINIAPP_CHAT_RATE_LIMIT_PER_MINUTE", "5")
	t.Setenv("MINIAPP_CHAT_TIMEOUT_SECONDS", "18")

	env := Load()

	if env.MiniappChat.RateLimitPerMinute != 5 {
		t.Fatalf("expected configured chat rate limit 5, got %d", env.MiniappChat.RateLimitPerMinute)
	}
	if env.MiniappChat.TimeoutSeconds != 18 {
		t.Fatalf("expected configured chat timeout 18, got %d", env.MiniappChat.TimeoutSeconds)
	}
}

func TestLoadReadsSpugSMSConfig(t *testing.T) {
	t.Setenv("SMS_PROVIDER", "spug")
	t.Setenv("SPUG_PUSH_API_BASE", "https://push.spug.cc")
	t.Setenv("SPUG_PUSH_TEMPLATE_CODE", "tmpl123")
	t.Setenv("SPUG_PUSH_TEMPLATE_NAME", "芯之力")
	t.Setenv("SPUG_PUSH_TIMEOUT_SECONDS", "7")

	env := Load()

	if env.SMS.Provider != "spug" {
		t.Fatalf("expected spug provider, got %q", env.SMS.Provider)
	}
	if env.SMS.SpugAPIBase != "https://push.spug.cc" {
		t.Fatalf("expected spug api base, got %q", env.SMS.SpugAPIBase)
	}
	if env.SMS.SpugTemplateCode != "tmpl123" {
		t.Fatalf("expected spug template code, got %q", env.SMS.SpugTemplateCode)
	}
	if env.SMS.SpugTemplateName != "芯之力" {
		t.Fatalf("expected spug template name, got %q", env.SMS.SpugTemplateName)
	}
	if env.SMS.SpugTimeoutSeconds != 7 {
		t.Fatalf("expected spug timeout 7, got %d", env.SMS.SpugTimeoutSeconds)
	}
}

func TestLoadDefaultsASRToSiliconFlowFreeModel(t *testing.T) {
	t.Setenv("ENV_FILE", filepath.Join(t.TempDir(), "missing.env"))
	t.Setenv("ASR_API_BASE", "")
	t.Setenv("ASR_MODEL", "")
	t.Setenv("ASR_TIMEOUT_SECONDS", "")

	env := Load()

	if env.ASR.APIBase != "https://api.siliconflow.cn" {
		t.Fatalf("expected default SiliconFlow ASR base, got %q", env.ASR.APIBase)
	}
	if env.ASR.Model != "FunAudioLLM/SenseVoiceSmall" {
		t.Fatalf("expected default free SiliconFlow ASR model, got %q", env.ASR.Model)
	}
	if env.ASR.TimeoutSeconds != 60 {
		t.Fatalf("expected default ASR timeout 60, got %d", env.ASR.TimeoutSeconds)
	}
}

func TestLoadNormalizesAppEnv(t *testing.T) {
	t.Setenv("APP_ENV", " Production ")

	env := Load()

	if env.AppEnv != "production" {
		t.Fatalf("expected normalized production APP_ENV, got %q", env.AppEnv)
	}
}

func TestValidateProductionTreatsProdAliasAsProduction(t *testing.T) {
	err := ValidateProduction(Env{
		AdminPassword: "123456",
		AppEnv:        "prod",
		DatabaseURL:   "postgres://nx:nx@db:5432/nx_admin?sslmode=disable",
		JWTSecret:     "nine-xing-dev-secret",
	})
	if err == nil {
		t.Fatal("expected prod APP_ENV alias to run production validation")
	}
}

func TestValidateProductionRejectsUnknownAppEnv(t *testing.T) {
	err := ValidateProduction(Env{
		AdminPassword: "a-strong-admin-password",
		AppEnv:        "prd",
		DatabaseURL:   "postgres://nx_app:a-strong-database-password@db:5432/nx_admin?sslmode=disable",
		JWTSecret:     "12345678901234567890123456789012",
	})
	if err == nil {
		t.Fatal("expected unknown APP_ENV to be rejected")
	}
}

func TestValidateProductionRejectsMissingAppEnv(t *testing.T) {
	err := ValidateProduction(Env{
		AdminPassword: "a-strong-admin-password",
		DatabaseURL:   "postgres://nx_app:a-strong-database-password@db:5432/nx_admin?sslmode=disable",
		JWTSecret:     "12345678901234567890123456789012",
	})
	if err == nil {
		t.Fatal("expected missing APP_ENV to be rejected")
	}
}

func TestLoadReadsDotEnvFromParentDirectory(t *testing.T) {
	for _, key := range []string{"ENV_FILE", "OSS_BUCKET", "OSS_PUBLIC_URL", "OSS_REGION", "OSS_ACCESS_KEY_ID", "OSS_ACCESS_KEY_SECRET"} {
		t.Setenv(key, "")
	}
	root := t.TempDir()
	if err := os.WriteFile(filepath.Join(root, ".env"), []byte("OSS_BUCKET=test-bucket\nOSS_PUBLIC_URL=https://cdn.example.com\nOSS_REGION=cn-test\nOSS_ACCESS_KEY_ID=ak\nOSS_ACCESS_KEY_SECRET=sk\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	nested := filepath.Join(root, "a", "b")
	if err := os.MkdirAll(nested, 0o755); err != nil {
		t.Fatal(err)
	}
	previous, _ := os.Getwd()
	if err := os.Chdir(nested); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = os.Chdir(previous) })

	env := Load()

	if env.OSS.Bucket != "test-bucket" || env.OSS.PublicURL != "https://cdn.example.com" {
		t.Fatalf("expected OSS config from parent .env, got %+v", env.OSS)
	}
}

func TestValidateProductionRejectsDevFallbacks(t *testing.T) {
	err := ValidateProduction(Env{
		AdminPassword: "123456",
		AppEnv:        "production",
		DatabaseURL:   "postgres://nx:nx@db:5432/nx_admin?sslmode=disable",
		JWTSecret:     "nine-xing-dev-secret",
	})
	if err == nil {
		t.Fatal("expected production validation to reject weak defaults")
	}
}

func TestValidateProductionAllowsMissingOptionalIntegrations(t *testing.T) {
	err := ValidateProduction(Env{
		AdminPassword: "a-strong-admin-password",
		AppEnv:        "production",
		DatabaseURL:   "postgres://nx_app:a-strong-database-password@db:5432/nx_admin?sslmode=disable",
		JWTSecret:     "12345678901234567890123456789012",
	})
	if err != nil {
		t.Fatalf("expected production core config to pass without optional integrations, got %v", err)
	}
}

func TestValidateProductionRejectsExplicitDevIntegrations(t *testing.T) {
	base := Env{
		AdminPassword: "a-strong-admin-password",
		AppEnv:        "production",
		DatabaseURL:   "postgres://nx_app:a-strong-database-password@db:5432/nx_admin?sslmode=disable",
		JWTSecret:     "12345678901234567890123456789012",
	}

	withWeChatDev := base
	withWeChatDev.WeChat.LoginDev = true
	if err := ValidateProduction(withWeChatDev); err == nil {
		t.Fatal("expected production validation to reject WECHAT_LOGIN_DEV")
	}

	withWxPayDev := base
	withWxPayDev.WxPay.Dev = true
	if err := ValidateProduction(withWxPayDev); err == nil {
		t.Fatal("expected production validation to reject WXPAY_DEV")
	}
}

func TestValidateProductionRejectsVideoGatewayWithoutPublicBaseURL(t *testing.T) {
	err := ValidateProduction(Env{
		AdminPassword: "a-strong-admin-password",
		AppEnv:        "production",
		DatabaseURL:   "postgres://nx_app:a-strong-database-password@db:5432/nx_admin?sslmode=disable",
		JWTSecret:     "12345678901234567890123456789012",
		Video: VideoConfig{
			APIBase: "https://video.example.com",
			APIKey:  "video-key",
		},
	})
	if err == nil {
		t.Fatal("expected production validation to require PUBLIC_BASE_URL when video gateway is enabled")
	}
}

func TestValidateProductionRequiresExplicitMediaAPIBaseWhenKeyConfigured(t *testing.T) {
	base := Env{
		AdminPassword: "a-strong-admin-password",
		AppEnv:        "production",
		DatabaseURL:   "postgres://nx_app:a-strong-database-password@db:5432/nx_admin?sslmode=disable",
		JWTSecret:     "12345678901234567890123456789012",
		PublicBaseURL: "https://api.example.com",
	}

	withVideoKey := base
	withVideoKey.Video.APIKey = "video-key"
	if err := ValidateProduction(withVideoKey); err == nil {
		t.Fatal("expected production validation to require VIDEO_API_BASE when VIDEO_API_KEY is set")
	}

	withImageKey := base
	withImageKey.Image.APIKey = "image-key"
	if err := ValidateProduction(withImageKey); err == nil {
		t.Fatal("expected production validation to require IMAGE_API_BASE when IMAGE_API_KEY is set")
	}
}

func TestValidateProductionRejectsPrivatePublicBaseURLWhenVideoGatewayEnabled(t *testing.T) {
	err := ValidateProduction(Env{
		AdminPassword: "a-strong-admin-password",
		AppEnv:        "production",
		DatabaseURL:   "postgres://nx_app:a-strong-database-password@db:5432/nx_admin?sslmode=disable",
		JWTSecret:     "12345678901234567890123456789012",
		PublicBaseURL: "http://127.0.0.1:8080",
		Video: VideoConfig{
			APIBase: "https://video.example.com",
			APIKey:  "video-key",
		},
	})
	if err == nil {
		t.Fatal("expected production validation to reject private PUBLIC_BASE_URL when video gateway is enabled")
	}
}

func TestValidateProductionRejectsInvalidTrustedProxyCIDRs(t *testing.T) {
	err := ValidateProduction(Env{
		AdminPassword:     "a-strong-admin-password",
		AppEnv:            "production",
		DatabaseURL:       "postgres://nx_app:a-strong-database-password@db:5432/nx_admin?sslmode=disable",
		JWTSecret:         "12345678901234567890123456789012",
		TrustedProxyCIDRs: []string{"not-a-cidr"},
	})
	if err == nil {
		t.Fatal("expected production validation to reject invalid TRUSTED_PROXY_CIDRS")
	}
}

func TestValidateProductionRejectsPlaceholderAdminAndDatabasePasswords(t *testing.T) {
	base := Env{
		AdminPassword: "a-strong-admin-password",
		AppEnv:        "production",
		DatabaseURL:   "postgres://nx_app:a-strong-database-password@db:5432/nx_admin?sslmode=disable",
		JWTSecret:     "12345678901234567890123456789012",
	}

	withPlaceholderAdmin := base
	withPlaceholderAdmin.AdminPassword = "change-me-to-a-strong-password"
	if err := ValidateProduction(withPlaceholderAdmin); err == nil {
		t.Fatal("expected production validation to reject placeholder ADMIN_PASSWORD")
	}

	withPlaceholderDB := base
	withPlaceholderDB.DatabaseURL = "postgres://nx_app:change-me-too@db:5432/nx_admin?sslmode=disable"
	if err := ValidateProduction(withPlaceholderDB); err == nil {
		t.Fatal("expected production validation to reject placeholder POSTGRES_PASSWORD in DATABASE_URL")
	}
}

func TestValidateProductionRejectsPrivateExternalAPIBases(t *testing.T) {
	base := Env{
		AdminPassword: "a-strong-admin-password",
		AppEnv:        "production",
		DatabaseURL:   "postgres://nx_app:a-strong-database-password@db:5432/nx_admin?sslmode=disable",
		JWTSecret:     "12345678901234567890123456789012",
	}

	for name, mutate := range map[string]func(*Env){
		"MINIMAX_API_BASE": func(env *Env) { env.MiniMax.APIBase = "http://127.0.0.1:8000" },
		"VIDEO_API_BASE":   func(env *Env) { env.Video.APIBase = "http://10.0.0.2:8000" },
		"IMAGE_API_BASE":   func(env *Env) { env.Image.APIBase = "http://localhost:8000" },
		"ASR_API_BASE":     func(env *Env) { env.ASR.APIBase = "http://192.168.1.2:8000" },
		"EMBEDDING_API_BASE": func(env *Env) {
			env.Embedding.APIBase = "http://169.254.169.254/latest"
		},
	} {
		t.Run(name, func(t *testing.T) {
			env := base
			mutate(&env)
			if err := ValidateProduction(env); err == nil {
				t.Fatalf("expected production validation to reject private %s", name)
			}
		})
	}
}

func TestValidateProductionAcceptsCompleteOptionalConfig(t *testing.T) {
	err := ValidateProduction(Env{
		AdminPassword: "a-strong-admin-password",
		AppEnv:        "production",
		DatabaseURL:   "postgres://nx_app:a-strong-database-password@db:5432/nx_admin?sslmode=disable",
		JWTSecret:     "12345678901234567890123456789012",
		SMS: SMSConfig{
			Provider:  "aliyun",
			APIKey:    "ak",
			APISecret: "sk",
		},
		WeChat: WeChatConfig{
			AppID:  "wx-appid",
			Secret: "wx-secret",
		},
		WxPay: WxPayConfig{
			MchID:            "mch",
			AppID:            "wx-appid",
			APIv3Key:         "12345678901234567890123456789012",
			SerialNo:         "serial",
			PrivateKeyPath:   "/run/secrets/apiclient_key.pem",
			PlatformCertPath: "/run/secrets/wechatpay_platform.pem",
			NotifyURL:        "https://api.example.com/api/pay/notify",
		},
		PublicBaseURL:     "https://api.example.com",
		TrustedProxyCIDRs: []string{"127.0.0.1", "10.0.0.0/24"},
		Video: VideoConfig{
			APIBase: "https://video.example.com",
			APIKey:  "video-key",
		},
	})
	if err != nil {
		t.Fatalf("expected complete production config to pass, got %v", err)
	}
}
