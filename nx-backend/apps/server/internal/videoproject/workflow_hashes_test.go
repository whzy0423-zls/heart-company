package videoproject

import (
	"testing"

	"nine-xing/nx-backend/apps/server/internal/video"
)

func TestShotRequestHashUsesExactNormalizedRequestExceptRequestKey(t *testing.T) {
	seed := 42
	cameraFixed := true
	request := video.GenerateRequest{
		Model: "video-ds-2.0", Prompt: "雨夜车站，阿宁捡起相机", Duration: 10,
		AspectRatio: "9:16", Resolution: "", TaskMode: "reference",
		References: []video.Reference{
			{ID: "1", Kind: "image", Role: "reference_image", URL: "https://cdn.example.com/a.png", SortOrder: 1, SourceType: "character", SourceID: "7", UsageNote: "保持人物一致"},
			{ID: "2", Kind: "image", Role: "reference_image", URL: "https://cdn.example.com/b.png", SortOrder: 2, SourceType: "scene", SourceID: "8"},
		},
		RequestKey: "11111111-1111-4111-8111-111111111111", CapabilityVersion: "capability-v1",
		Seed: &seed, CameraFixed: &cameraFixed,
	}
	base := ShotRequestHash(request)
	if base == "" {
		t.Fatal("expected non-empty request hash")
	}

	sameIntent := cloneGenerateRequestForHashTest(request)
	sameIntent.RequestKey = "22222222-2222-4222-8222-222222222222"
	if got := ShotRequestHash(sameIntent); got != base {
		t.Fatalf("request key must not change request intent hash: base=%s got=%s", base, got)
	}

	mutations := []struct {
		name   string
		mutate func(*video.GenerateRequest)
	}{
		{"model", func(value *video.GenerateRequest) { value.Model = "video-ds-2.0-fast" }},
		{"prompt", func(value *video.GenerateRequest) { value.Prompt += "，镜头推进" }},
		{"duration", func(value *video.GenerateRequest) { value.Duration = 15 }},
		{"aspect ratio", func(value *video.GenerateRequest) { value.AspectRatio = "16:9" }},
		{"resolution", func(value *video.GenerateRequest) { value.Resolution = "1080P" }},
		{"generate audio", func(value *video.GenerateRequest) { enabled := true; value.GenerateAudio = &enabled }},
		{"task mode", func(value *video.GenerateRequest) { value.TaskMode = "edit" }},
		{"capability", func(value *video.GenerateRequest) { value.CapabilityVersion = "capability-v2" }},
		{"seed", func(value *video.GenerateRequest) { next := 43; value.Seed = &next }},
		{"camera fixed", func(value *video.GenerateRequest) { next := false; value.CameraFixed = &next }},
		{"reference order", func(value *video.GenerateRequest) {
			value.References[0], value.References[1] = value.References[1], value.References[0]
		}},
		{"reference url", func(value *video.GenerateRequest) { value.References[0].URL = "https://cdn.example.com/new.png" }},
		{"reference role", func(value *video.GenerateRequest) { value.References[0].Role = "first_frame" }},
		{"reference source", func(value *video.GenerateRequest) { value.References[0].SourceID = "99" }},
		{"reference usage", func(value *video.GenerateRequest) { value.References[0].UsageNote = "只参考服装" }},
	}
	for _, mutation := range mutations {
		t.Run(mutation.name, func(t *testing.T) {
			changed := cloneGenerateRequestForHashTest(request)
			mutation.mutate(&changed)
			if got := ShotRequestHash(changed); got == base {
				t.Fatalf("mutation %q did not change request hash", mutation.name)
			}
		})
	}
}

func TestDiagnosticsHashTracksShotReferencesCapabilityAndCompiler(t *testing.T) {
	duration := 10
	input := DiagnosticsHashInput{
		ShotContent: map[string]any{
			"name": "发现相机", "duration": duration, "action": "阿宁捡起旧相机",
		},
		References: video.CanonicalReferences{References: []video.CanonicalReference{{
			Reference: video.Reference{ID: "1", Kind: "image", Role: "reference_image", URL: "https://cdn.example.com/a.png", SortOrder: 1},
			Ordinal:   1, Label: "图片1",
		}}},
		CapabilityVersion: "capability-v1",
		CompilerVersion:   SeedancePromptVersion,
	}
	base := DiagnosticsHash(input)
	if base == "" {
		t.Fatal("expected diagnostics hash")
	}

	reorderedMap := input
	reorderedMap.ShotContent = map[string]any{"action": "阿宁捡起旧相机", "duration": duration, "name": "发现相机"}
	if got := DiagnosticsHash(reorderedMap); got != base {
		t.Fatalf("map insertion order must not change hash: %s != %s", got, base)
	}

	mutations := []struct {
		name   string
		mutate func(*DiagnosticsHashInput)
	}{
		{"shot content", func(value *DiagnosticsHashInput) {
			value.ShotContent = map[string]any{"name": "相机亮起", "duration": 10}
		}},
		{"reference", func(value *DiagnosticsHashInput) {
			value.References.References[0].URL = "https://cdn.example.com/b.png"
		}},
		{"capability", func(value *DiagnosticsHashInput) { value.CapabilityVersion = "capability-v2" }},
		{"compiler", func(value *DiagnosticsHashInput) { value.CompilerVersion = "seedance2_v3" }},
	}
	for _, mutation := range mutations {
		t.Run(mutation.name, func(t *testing.T) {
			changed := cloneDiagnosticsHashInput(input)
			mutation.mutate(&changed)
			if DiagnosticsHash(changed) == base {
				t.Fatalf("mutation %q did not change diagnostics hash", mutation.name)
			}
		})
	}
}

func TestSelectionAckHashIsStableAndBoundToRequestAndGeneration(t *testing.T) {
	base := SelectionAckHash("request-hash-a", "generation-7")
	if base == "" || SelectionAckHash("request-hash-a", "generation-7") != base {
		t.Fatal("selection acknowledgement hash must be stable")
	}
	if SelectionAckHash("request-hash-b", "generation-7") == base {
		t.Fatal("request mutation must invalidate selection acknowledgement")
	}
	if SelectionAckHash("request-hash-a", "generation-8") == base {
		t.Fatal("selection mutation must invalidate acknowledgement")
	}
}

func TestComposeInputHashTracksOrderedSelectionsAndSettings(t *testing.T) {
	settings := ComposeProjectInput{Transition: "fade", MusicURL: "https://cdn.example.com/music.mp3", EnableSubtitles: true}
	base := ComposeInputHash([]string{"generation-1", "generation-2"}, settings)
	if base == "" || ComposeInputHash([]string{"generation-1", "generation-2"}, settings) != base {
		t.Fatal("compose input hash must be stable")
	}
	if ComposeInputHash([]string{"generation-2", "generation-1"}, settings) == base {
		t.Fatal("shot order must change compose hash")
	}
	if ComposeInputHash([]string{"generation-1", "generation-3"}, settings) == base {
		t.Fatal("selected generation must change compose hash")
	}
	changed := settings
	changed.Transition = "none"
	if ComposeInputHash([]string{"generation-1", "generation-2"}, changed) == base {
		t.Fatal("compose settings must change compose hash")
	}
}

func cloneGenerateRequestForHashTest(input video.GenerateRequest) video.GenerateRequest {
	output := input
	output.References = append([]video.Reference{}, input.References...)
	if input.GenerateAudio != nil {
		value := *input.GenerateAudio
		output.GenerateAudio = &value
	}
	if input.Seed != nil {
		value := *input.Seed
		output.Seed = &value
	}
	if input.CameraFixed != nil {
		value := *input.CameraFixed
		output.CameraFixed = &value
	}
	return output
}

func cloneDiagnosticsHashInput(input DiagnosticsHashInput) DiagnosticsHashInput {
	output := input
	output.References.References = append([]video.CanonicalReference{}, input.References.References...)
	return output
}
