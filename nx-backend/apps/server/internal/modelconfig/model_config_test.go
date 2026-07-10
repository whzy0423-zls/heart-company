package modelconfig

import (
	"context"
	"encoding/json"
	"errors"
	"reflect"
	"testing"

	"nine-xing/nx-backend/apps/server/internal/config"
)

func testContentItemsGatewayContract() config.GatewayContractConfig {
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
			ValueMap:  map[string]string{"1080P": "1080p", "4K": "4k"},
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
			SupportsRoles:       []string{"reference_image", "first_frame", "last_frame", "reference_video", "reference_audio", "edit_target", "extend_target"},
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

func TestVideoGatewayContractRoundTrip(t *testing.T) {
	want := VideoConfig{
		APIBase:         "https://gateway.example.com/v1",
		APIKey:          "secret-key",
		Model:           "video-ds-2.0",
		ModelProfile:    "standard",
		GatewayContract: testContentItemsGatewayContract(),
	}

	raw, err := json.Marshal(want)
	if err != nil {
		t.Fatal(err)
	}
	const exact = `{"apiBase":"https://gateway.example.com/v1","apiKey":"secret-key","model":"video-ds-2.0","modelProfile":"standard","gatewayContract":{"name":"seedance2_configured_v1","version":"3","declaredModes":["reference","edit","extend"],"duration":{"name":"duration","valueType":"int","valueMap":{"smart":"-1"}},"aspectRatio":{"name":"aspect_ratio","valueType":"string"},"resolution":{"name":"resolution","valueType":"string","valueMap":{"1080P":"1080p","4K":"4k"}},"generateAudio":{"name":"generate_audio","valueType":"bool"},"taskMode":{"name":"task_mode","valueType":"string"},"references":{"mode":"content_items","imageField":"image_url","videoField":"video_url","audioField":"audio_url","roleFields":{"edit_target":"edit_target","extend_target":"extend_target","first_frame":"first_frame","last_frame":"last_frame","reference_audio":"reference_audio","reference_image":"reference_image","reference_video":"reference_video"},"supportsRoles":["reference_image","first_frame","last_frame","reference_video","reference_audio","edit_target","extend_target"],"requiresTargetFirst":true},"limits":{"maxImages":9,"maxVideos":3,"maxAudios":3,"maxVideoSecondsTotal":15,"maxAudioSecondsTotal":15},"idempotency":{"header":"X-Request-Key"},"reconciliation":{"lookupByRequestKey":true,"method":"GET","pathTemplate":"/v1/videos/by-request/{requestKey}","taskIdPaths":["data.task_id","task_id"],"statusPaths":["data.status","status"]}}}`
	if string(raw) != exact {
		t.Fatalf("unexpected JSON:\n got: %s\nwant: %s", raw, exact)
	}

	var got VideoConfig
	if err := json.Unmarshal(raw, &got); err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("round trip mismatch:\n got: %#v\nwant: %#v", got, want)
	}
}

func TestMergeIncomingVideoContract(t *testing.T) {
	storedContract := testContentItemsGatewayContract()
	storedContract.Name = "stored_contract"
	storedContract.Version = "1"
	current := Config{Video: VideoConfig{
		APIKey:          "stored-secret",
		ModelProfile:    "fast",
		GatewayContract: storedContract,
	}}
	incomingContract := testContentItemsGatewayContract()
	incomingContract.Version = "4"
	incoming := Config{Video: VideoConfig{
		APIBase:         "https://new-gateway.example.com/v1",
		Model:           "video-ds-2.0",
		ModelProfile:    "standard",
		GatewayContract: incomingContract,
	}}

	got := current.MergeIncoming(incoming)

	if got.Video.APIKey != "stored-secret" {
		t.Fatalf("expected empty incoming API key to preserve stored secret, got %q", got.Video.APIKey)
	}
	if got.Video.ModelProfile != "standard" {
		t.Fatalf("expected incoming model profile to replace stored profile, got %q", got.Video.ModelProfile)
	}
	if !reflect.DeepEqual(got.Video.GatewayContract, incomingContract) {
		t.Fatalf("expected incoming non-secret contract to replace stored contract:\n got: %#v\nwant: %#v", got.Video.GatewayContract, incomingContract)
	}
}

func TestApplyVideoUsesStoredProfileAndContract(t *testing.T) {
	baseContract := testContentItemsGatewayContract()
	baseContract.Name = "legacy_flat_v1"
	baseContract.Version = "1"
	base := config.VideoConfig{
		ModelProfile:    "fast",
		GatewayContract: baseContract,
	}
	storedContract := testContentItemsGatewayContract()
	storedContract.Version = "4"
	cfg := Config{Video: VideoConfig{
		ModelProfile:    " standard ",
		GatewayContract: storedContract,
	}}

	got := cfg.ApplyVideo(base)

	if got.ModelProfile != "standard" {
		t.Fatalf("expected stored model profile override, got %q", got.ModelProfile)
	}
	if !reflect.DeepEqual(got.GatewayContract, storedContract) {
		t.Fatalf("expected stored gateway contract override:\n got: %#v\nwant: %#v", got.GatewayContract, storedContract)
	}
}

func TestTrimmedVideoGatewayContract(t *testing.T) {
	raw := testContentItemsGatewayContract()
	raw.Name = " seedance2_configured_v1 "
	raw.Version = " 3 "
	raw.DeclaredModes = []string{" reference ", " edit ", " extend "}
	raw.Duration = config.FieldEncoding{
		Name:      " duration ",
		ValueType: " int ",
		ValueMap:  map[string]string{" smart ": " -1 "},
	}
	raw.References.Mode = " content_items "
	raw.References.ImageField = " image_url "
	raw.References.VideoField = " video_url "
	raw.References.AudioField = " audio_url "
	raw.References.RoleFields = map[string]string{" edit_target ": " target_video "}
	raw.References.SupportsRoles = []string{" reference_image ", " edit_target "}
	raw.Idempotency.Header = " X-Request-Key "
	raw.Reconciliation.Method = " GET "
	raw.Reconciliation.PathTemplate = " /v1/videos/by-request/{requestKey} "
	raw.Reconciliation.TaskIDPaths = []string{" data.task_id ", " task_id "}
	raw.Reconciliation.StatusPaths = []string{" data.status ", " status "}

	got := (Config{Video: VideoConfig{
		ModelProfile:    " standard ",
		GatewayContract: raw,
	}}).trimmed().Video
	want := raw
	want.Name = "seedance2_configured_v1"
	want.Version = "3"
	want.DeclaredModes = []string{"reference", "edit", "extend"}
	want.Duration = config.FieldEncoding{
		Name:      "duration",
		ValueType: "int",
		ValueMap:  map[string]string{"smart": "-1"},
	}
	want.References.Mode = "content_items"
	want.References.ImageField = "image_url"
	want.References.VideoField = "video_url"
	want.References.AudioField = "audio_url"
	want.References.RoleFields = map[string]string{"edit_target": "target_video"}
	want.References.SupportsRoles = []string{"reference_image", "edit_target"}
	want.Idempotency.Header = "X-Request-Key"
	want.Reconciliation.Method = "GET"
	want.Reconciliation.PathTemplate = "/v1/videos/by-request/{requestKey}"
	want.Reconciliation.TaskIDPaths = []string{"data.task_id", "task_id"}
	want.Reconciliation.StatusPaths = []string{"data.status", "status"}

	if got.ModelProfile != "standard" {
		t.Fatalf("expected trimmed model profile, got %q", got.ModelProfile)
	}
	if !reflect.DeepEqual(got.GatewayContract, want) {
		t.Fatalf("unexpected trimmed contract:\n got: %#v\nwant: %#v", got.GatewayContract, want)
	}
}

func TestValidateVideoGatewayContractRejectsUnsafeConfig(t *testing.T) {
	cases := []struct {
		name   string
		mutate func(*config.GatewayContractConfig)
		code   string
	}{
		{"unsafe field", func(c *config.GatewayContractConfig) { c.Duration.Name = "content[0]" }, "invalid_field_name"},
		{"bad type", func(c *config.GatewayContractConfig) { c.Duration.ValueType = "object" }, "invalid_value_type"},
		{"bad role", func(c *config.GatewayContractConfig) { c.References.SupportsRoles = []string{"magic_role"} }, "invalid_reference_role"},
		{"authorization header", func(c *config.GatewayContractConfig) { c.Idempotency.Header = "Authorization" }, "reserved_header"},
		{"newline header", func(c *config.GatewayContractConfig) { c.Idempotency.Header = "X-Key\nInjected" }, "invalid_header_name"},
		{"paid reconcile method", func(c *config.GatewayContractConfig) { c.Reconciliation.Method = "POST" }, "invalid_reconciliation_method"},
		{"unsafe reconcile path", func(c *config.GatewayContractConfig) {
			c.Reconciliation.PathTemplate = "https://evil.example/{requestKey}"
		}, "invalid_reconciliation_path"},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			contract := testContentItemsGatewayContract()
			tc.mutate(&contract)

			err := ValidateVideoGatewayContract(contract)
			if err == nil {
				t.Fatalf("expected validation error %q", tc.code)
			}
			var validationErr *GatewayContractValidationError
			if !errors.As(err, &validationErr) {
				t.Fatalf("expected GatewayContractValidationError, got %T: %v", err, err)
			}
			if validationErr.Code != tc.code {
				t.Fatalf("expected code %q, got %q (%v)", tc.code, validationErr.Code, err)
			}
		})
	}
}

func TestValidateVideoGatewayContractAcceptsContentItems(t *testing.T) {
	if err := ValidateVideoGatewayContract(testContentItemsGatewayContract()); err != nil {
		t.Fatalf("expected configured content-items contract to pass, got %v", err)
	}
}

func TestUpsertStoreRejectsUnsafeVideoGatewayContractBeforeDatabase(t *testing.T) {
	contract := testContentItemsGatewayContract()
	contract.Duration.Name = "content[0]"

	err := UpsertStore(context.Background(), nil, Config{Video: VideoConfig{GatewayContract: contract}})

	var validationErr *GatewayContractValidationError
	if !errors.As(err, &validationErr) || validationErr.Code != "invalid_field_name" {
		t.Fatalf("expected invalid_field_name before any database write, got %T: %v", err, err)
	}
}

func TestApplyVideoIgnoresUnsafeStoredContract(t *testing.T) {
	baseContract := testContentItemsGatewayContract()
	baseContract.Name = "legacy_flat_v1"
	baseContract.Version = "1"
	unsafeContract := testContentItemsGatewayContract()
	unsafeContract.Duration.Name = "content[0]"

	got := (Config{Video: VideoConfig{GatewayContract: unsafeContract}}).ApplyVideo(config.VideoConfig{GatewayContract: baseContract})

	if !reflect.DeepEqual(got.GatewayContract, baseContract) {
		t.Fatalf("expected unsafe stored contract to fail closed to the environment baseline:\n got: %#v\nwant: %#v", got.GatewayContract, baseContract)
	}
}

func TestApplyAnalysisUsesVoiceMiniMaxCredentialsAndDefaultM3(t *testing.T) {
	voiceBase := config.MiniMaxConfig{
		APIBase:        "https://api.minimaxi.com",
		APIKey:         "voice-key",
		GroupID:        "voice-group",
		Model:          "abab6.5s-chat",
		TimeoutSeconds: 77,
	}
	cfg := Config{
		Chat: ChatConfig{
			APIBase: "https://coding-play.codes",
			APIKey:  "chat-key",
			GroupID: "chat-group",
			Model:   "gpt-5.5",
		},
		Analysis: AnalysisConfig{
			APIBase: "https://old-analysis.example",
			APIKey:  "old-analysis-key",
			GroupID: "old-analysis-group",
		},
	}

	got := cfg.ApplyAnalysis(voiceBase)

	if got.APIBase != voiceBase.APIBase || got.APIKey != voiceBase.APIKey || got.GroupID != voiceBase.GroupID {
		t.Fatalf("expected analysis to reuse voice MiniMax credentials, got %+v", got)
	}
	if got.Model != DefaultAnalysisModel {
		t.Fatalf("expected default analysis model %q, got %q", DefaultAnalysisModel, got.Model)
	}
	if got.TimeoutSeconds != DefaultAnalysisTimeoutSeconds {
		t.Fatalf("expected analysis timeout %d, got %d", DefaultAnalysisTimeoutSeconds, got.TimeoutSeconds)
	}
}

func TestApplyAnalysisAllowsModelOverrideOnly(t *testing.T) {
	voiceBase := config.MiniMaxConfig{
		APIBase: "https://api.minimaxi.com",
		APIKey:  "voice-key",
		GroupID: "voice-group",
		Model:   "abab6.5s-chat",
	}
	cfg := Config{
		Analysis: AnalysisConfig{
			APIBase: "https://old-analysis.example",
			APIKey:  "old-analysis-key",
			GroupID: "old-analysis-group",
			Model:   "MiniMax-M3-Preview",
		},
	}

	got := cfg.ApplyAnalysis(voiceBase)

	if got.APIBase != voiceBase.APIBase || got.APIKey != voiceBase.APIKey || got.GroupID != voiceBase.GroupID {
		t.Fatalf("expected only analysis model to override voice credentials, got %+v", got)
	}
	if got.Model != "MiniMax-M3-Preview" {
		t.Fatalf("expected model override, got %q", got.Model)
	}
}

func TestApplyAnalysisIgnoresStaleNonMiniMaxModel(t *testing.T) {
	voiceBase := config.MiniMaxConfig{
		APIBase: "https://api.minimaxi.com",
		APIKey:  "voice-key",
		GroupID: "voice-group",
		Model:   "abab6.5s-chat",
	}
	cfg := Config{
		Analysis: AnalysisConfig{Model: "gpt-5.5"},
	}

	got := cfg.ApplyAnalysis(voiceBase)

	if got.Model != DefaultAnalysisModel {
		t.Fatalf("expected stale non-MiniMax model to fall back to %q, got %q", DefaultAnalysisModel, got.Model)
	}
}

func TestApplyDailyQuizInheritsAdminCompatibleModelConfig(t *testing.T) {
	cfg := Config{
		Admin: CompatibleModelConfig{
			Provider:       " openai-compatible ",
			APIBase:        " https://gateway.example.com/v1 ",
			APIKey:         " admin-key ",
			Model:          " gpt-5.5 ",
			TimeoutSeconds: 31,
		},
		DailyQuiz: CompatibleModelConfig{
			Model:          " gpt-5.5-mini ",
			TimeoutSeconds: 47,
		},
	}

	got := cfg.ApplyDailyQuiz()

	if got.Provider != "openai-compatible" || got.APIBase != "https://gateway.example.com/v1" || got.APIKey != "admin-key" {
		t.Fatalf("expected daily quiz to inherit admin provider/base/key, got %+v", got)
	}
	if got.Model != "gpt-5.5-mini" || got.TimeoutSeconds != 47 {
		t.Fatalf("expected daily quiz model/timeout override, got %+v", got)
	}
}

func TestMergeIncomingPreservesAdminAndDailyQuizAPIKeys(t *testing.T) {
	current := Config{
		Admin:     CompatibleModelConfig{APIKey: "admin-secret", Model: "gpt-old"},
		DailyQuiz: CompatibleModelConfig{APIKey: "quiz-secret", Model: "gpt-quiz-old"},
	}
	incoming := Config{
		Admin:     CompatibleModelConfig{Provider: "openai-compatible", APIBase: "https://admin.example.com/v1", Model: "gpt-new", TimeoutSeconds: 41},
		DailyQuiz: CompatibleModelConfig{Provider: "anthropic-compatible", APIBase: "https://quiz.example.com/v1", Model: "claude-new", TimeoutSeconds: 52},
	}

	got := current.MergeIncoming(incoming)

	if got.Admin.APIKey != "admin-secret" || got.DailyQuiz.APIKey != "quiz-secret" {
		t.Fatalf("expected empty incoming keys to preserve stored secrets, got admin=%q daily=%q", got.Admin.APIKey, got.DailyQuiz.APIKey)
	}
	if got.Admin.Model != "gpt-new" || got.DailyQuiz.Model != "claude-new" {
		t.Fatalf("expected non-secret fields to update, got %+v", got)
	}
}
