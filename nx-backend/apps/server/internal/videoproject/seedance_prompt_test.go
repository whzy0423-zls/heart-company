package videoproject

import (
	"reflect"
	"strings"
	"testing"
	"unicode/utf8"

	"nine-xing/nx-backend/apps/server/internal/config"
	"nine-xing/nx-backend/apps/server/internal/video"
)

func TestSeedancePromptReferenceGolden(t *testing.T) {
	in := PromptInput{
		Mode:        "reference",
		Subject:     "小夏",
		Action:      "在雨夜车站快步回头",
		Scene:       "蓝色霓虹灯下的站台",
		Camera:      "缓慢跟拍",
		Dialogue:    "别走",
		SoundEffect: "远处列车鸣笛",
		References:  canonicalReferencePromptFixture(t),
	}

	got := CompileSeedancePrompt(in, fullPromptCapabilities())
	want := "参考图片1中的角色“小夏”外观，参考图片2中的雨夜车站，参考视频2的缓慢跟拍运镜，参考音频1的音色。小夏在蓝色霓虹灯下的站台快步回头，镜头缓慢跟拍，{别走}，<远处列车鸣笛>。保持无字幕、不要生成 Logo、不要生成水印。"
	if got.Prompt != want {
		t.Fatalf("prompt = %q, want %q", got.Prompt, want)
	}
	assertCompiledPromptMetadata(t, got, in.References)
}

func TestSeedancePromptEditGolden(t *testing.T) {
	references := canonicalPromptReferences(t, []video.Reference{
		{ID: "10", Kind: "video", Role: "reference_video", URL: "https://cdn.example.com/reference.mp4", SortOrder: 1},
		{ID: "20", Kind: "video", Role: "edit_target", URL: "https://cdn.example.com/edit.mp4", SortOrder: 2},
	})
	in := PromptInput{
		Mode:            "edit",
		EditInstruction: "将天空修改为黄昏",
		References:      references,
	}

	got := CompileSeedancePrompt(in, fullPromptCapabilities())
	want := "严格编辑视频2，将天空修改为黄昏，其余内容保持不变。保持无字幕、不要生成 Logo、不要生成水印。"
	if got.Prompt != want {
		t.Fatalf("prompt = %q, want %q", got.Prompt, want)
	}
	if strings.Contains(got.Prompt, "参考视频") {
		t.Fatalf("edit prompt must not describe its target as a reference video: %q", got.Prompt)
	}
}

func TestSeedancePromptExtendGolden(t *testing.T) {
	references := canonicalPromptReferences(t, []video.Reference{
		{ID: "10", Kind: "video", Role: "reference_video", URL: "https://cdn.example.com/reference.mp4", SortOrder: 1},
		{ID: "20", Kind: "video", Role: "extend_target", URL: "https://cdn.example.com/extend.mp4", SortOrder: 2},
	})
	in := PromptInput{
		Mode:              "extend",
		ExtendInstruction: "角色继续走入站台深处",
		References:        references,
	}

	got := CompileSeedancePrompt(in, fullPromptCapabilities())
	want := "向后延长视频2，生成角色继续走入站台深处。保持无字幕、不要生成 Logo、不要生成水印。"
	if got.Prompt != want {
		t.Fatalf("prompt = %q, want %q", got.Prompt, want)
	}
	if strings.Contains(got.Prompt, "参考视频") {
		t.Fatalf("extend prompt must not describe its target as a reference video: %q", got.Prompt)
	}
}

func TestSeedancePromptTargetDiagnostics(t *testing.T) {
	tests := []struct {
		name       string
		mode       string
		references []video.Reference
		wantCode   string
	}{
		{name: "edit target missing", mode: "edit", wantCode: "missing_edit_target"},
		{
			name: "multiple edit targets", mode: "edit", wantCode: "multiple_edit_targets",
			references: []video.Reference{
				{ID: "1", Kind: "video", Role: "edit_target", URL: "https://cdn.example.com/1.mp4"},
				{ID: "2", Kind: "video", Role: "edit_target", URL: "https://cdn.example.com/2.mp4"},
			},
		},
		{name: "extend target missing", mode: "extend", wantCode: "missing_extend_target"},
		{
			name: "multiple extend targets", mode: "extend", wantCode: "multiple_extend_targets",
			references: []video.Reference{
				{ID: "1", Kind: "video", Role: "extend_target", URL: "https://cdn.example.com/1.mp4"},
				{ID: "2", Kind: "video", Role: "extend_target", URL: "https://cdn.example.com/2.mp4"},
			},
		},
		{
			name: "target roles mixed", mode: "edit", wantCode: "mixed_target_roles",
			references: []video.Reference{
				{ID: "1", Kind: "video", Role: "edit_target", URL: "https://cdn.example.com/edit.mp4"},
				{ID: "2", Kind: "video", Role: "extend_target", URL: "https://cdn.example.com/extend.mp4"},
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := CompileSeedancePrompt(PromptInput{
				Mode:              tt.mode,
				EditInstruction:   "修改天空",
				ExtendInstruction: "继续向前走",
				References:        canonicalPromptReferences(t, tt.references),
			}, fullPromptCapabilities())
			assertPromptDiagnostic(t, got.Diagnostics, "error", tt.wantCode)
		})
	}
}

func TestSeedancePromptDiagnostics(t *testing.T) {
	t.Run("missing reference number", func(t *testing.T) {
		got := CompileSeedancePrompt(PromptInput{
			Mode:       "reference",
			Action:     "参考图片9的服饰，人物向前走",
			References: canonicalPromptReferences(t, []video.Reference{{ID: "1", Kind: "image", Role: "reference_image", URL: "https://cdn.example.com/1.png"}}),
		}, fullPromptCapabilities())
		assertPromptDiagnostic(t, got.Diagnostics, "error", "reference_number_missing")
	})

	t.Run("multiple camera movements", func(t *testing.T) {
		got := CompileSeedancePrompt(PromptInput{Mode: "reference", Subject: "人物", Action: "向前走", Camera: "先推镜，再环绕人物"}, fullPromptCapabilities())
		assertPromptDiagnostic(t, got.Diagnostics, "warning", "multiple_camera_movements")
	})

	t.Run("exact time segments", func(t *testing.T) {
		got := CompileSeedancePrompt(PromptInput{Mode: "reference", Action: "0-3秒走近镜头，3至5秒回头"}, fullPromptCapabilities())
		assertPromptDiagnostic(t, got.Diagnostics, "warning", "exact_time_segments_unstable")
	})

	t.Run("unsupported reference role", func(t *testing.T) {
		caps := fullPromptCapabilities()
		caps.ReferenceRoles = []string{"reference_image"}
		got := CompileSeedancePrompt(PromptInput{
			Mode:       "reference",
			Action:     "向前走",
			References: canonicalPromptReferences(t, []video.Reference{{ID: "1", Kind: "video", Role: "reference_video", URL: "https://cdn.example.com/1.mp4"}}),
		}, caps)
		assertPromptDiagnostic(t, got.Diagnostics, "error", "unsupported_reference_role")
	})

	t.Run("diagnostics explain repair", func(t *testing.T) {
		got := CompileSeedancePrompt(PromptInput{Mode: "edit"}, fullPromptCapabilities())
		if len(got.Diagnostics) == 0 {
			t.Fatal("expected diagnostics")
		}
		for _, diagnostic := range got.Diagnostics {
			if strings.TrimSpace(diagnostic.Message) == "" || strings.TrimSpace(diagnostic.Fix) == "" {
				t.Fatalf("diagnostic must explain the problem and repair: %+v", diagnostic)
			}
		}
	})
}

func TestSeedancePromptLengthBoundary(t *testing.T) {
	caps := fullPromptCapabilities()
	probe := CompileSeedancePrompt(PromptInput{Mode: "reference", Action: "甲"}, caps)
	overhead := utf8.RuneCountInString(probe.Prompt) - 1
	if overhead >= 500 {
		t.Fatalf("unexpected prompt overhead %d", overhead)
	}

	exact := CompileSeedancePrompt(PromptInput{Mode: "reference", Action: strings.Repeat("甲", 500-overhead)}, caps)
	if got := utf8.RuneCountInString(exact.Prompt); got != 500 {
		t.Fatalf("exact prompt runes = %d, want 500", got)
	}
	assertNoPromptDiagnostic(t, exact.Diagnostics, "prompt_over_500_chinese_chars")

	over := CompileSeedancePrompt(PromptInput{Mode: "reference", Action: strings.Repeat("甲", 501-overhead)}, caps)
	if got := utf8.RuneCountInString(over.Prompt); got != 501 {
		t.Fatalf("over prompt runes = %d, want 501", got)
	}
	assertPromptDiagnostic(t, over.Diagnostics, "warning", "prompt_over_500_chinese_chars")
}

func TestSeedancePromptSubtitleAndDefaultConstraints(t *testing.T) {
	withoutSubtitle := CompileSeedancePrompt(PromptInput{Mode: "reference", Action: "人物走入站台"}, fullPromptCapabilities())
	for _, want := range []string{"保持无字幕", "不要生成 Logo", "不要生成水印"} {
		if !strings.Contains(withoutSubtitle.Prompt, want) {
			t.Fatalf("default prompt missing %q: %q", want, withoutSubtitle.Prompt)
		}
	}

	withSubtitle := CompileSeedancePrompt(PromptInput{Mode: "reference", Action: "人物走入站台", Subtitle: "【字幕】下一站见"}, fullPromptCapabilities())
	if strings.Contains(withSubtitle.Prompt, "保持无字幕") {
		t.Fatalf("subtitle request conflicts with no-subtitle constraint: %q", withSubtitle.Prompt)
	}
	for _, want := range []string{"【字幕】下一站见", "不要生成 Logo", "不要生成水印"} {
		if !strings.Contains(withSubtitle.Prompt, want) {
			t.Fatalf("subtitle prompt missing %q: %q", want, withSubtitle.Prompt)
		}
	}
}

func TestSeedancePromptDoesNotInjectLegacyDefaults(t *testing.T) {
	got := CompileSeedancePrompt(PromptInput{Mode: "reference", Subject: "小夏", Action: "缓慢走近车站"}, fullPromptCapabilities())
	for _, unwanted := range []string{"walking", "medium shot", "static camera", "animation style", "中景", "静态镜头"} {
		if strings.Contains(strings.ToLower(got.Prompt), strings.ToLower(unwanted)) {
			t.Fatalf("prompt injected legacy default %q: %q", unwanted, got.Prompt)
		}
	}
}

func TestSeedancePromptUsesReferenceUsageNote(t *testing.T) {
	canonical := canonicalPromptReferences(t, []video.Reference{{
		ID: "1", Kind: "image", Role: "reference_image", URL: "https://cdn.example.com/outfit.png", UsageNote: "保持红色风衣一致",
	}})
	got := CompileSeedancePrompt(PromptInput{Mode: "reference", Subject: "小夏", Action: "向前走", References: canonical}, fullPromptCapabilities())
	if !strings.Contains(got.Prompt, "用途：保持红色风衣一致") {
		t.Fatalf("prompt omitted usage note: %q", got.Prompt)
	}
}

func TestSeedancePromptHashesAreDeterministicAndContentBound(t *testing.T) {
	in := PromptInput{Mode: "reference", Subject: "小夏", Action: "走入车站"}
	first := CompileSeedancePrompt(in, fullPromptCapabilities())
	second := CompileSeedancePrompt(in, fullPromptCapabilities())
	if first.RequestHash == "" || first.DiagnosticsHash == "" {
		t.Fatalf("hashes must be populated: %+v", first)
	}
	if first.RequestHash != second.RequestHash || first.DiagnosticsHash != second.DiagnosticsHash {
		t.Fatalf("same input produced unstable hashes: %+v / %+v", first, second)
	}
	in.Action = "跑入车站"
	changed := CompileSeedancePrompt(in, fullPromptCapabilities())
	if changed.RequestHash == first.RequestHash || changed.DiagnosticsHash == first.DiagnosticsHash {
		t.Fatalf("changed input must change both fingerprints: %+v / %+v", first, changed)
	}
}

func canonicalReferencePromptFixture(t *testing.T) video.CanonicalReferences {
	t.Helper()
	return canonicalPromptReferences(t, []video.Reference{
		{ID: "40", Kind: "audio", Role: "reference_audio", SourceType: "voice", URL: "https://cdn.example.com/voice.mp3", SortOrder: 0},
		{ID: "10", Kind: "image", Role: "reference_image", SourceType: "character", SourceID: "小夏", URL: "https://cdn.example.com/character.png", SortOrder: 1},
		{ID: "20", Kind: "image", Role: "reference_image", SourceType: "scene", SourceID: "雨夜车站", URL: "https://cdn.example.com/station.png", SortOrder: 1},
		{ID: "50", Kind: "video", Role: "edit_target", SourceType: "edit", URL: "https://cdn.example.com/edit.mp4", SortOrder: 1},
		{ID: "30", Kind: "video", Role: "reference_video", SourceType: "camera", URL: "https://cdn.example.com/camera.mp4", SortOrder: 2},
	})
}

func canonicalPromptReferences(t *testing.T, references []video.Reference) video.CanonicalReferences {
	t.Helper()
	canonical, err := video.CanonicalizeReferences(references)
	if err != nil {
		t.Fatal(err)
	}
	return canonical
}

func fullPromptCapabilities() video.Capabilities {
	return video.Capabilities{
		Model:              "video-ds-2.0",
		CapabilityVersion:  "prompt-capability-v1",
		SupportedDurations: []int{4, 5, 6, 7, 8, 9, 10, 11, 12, 13, 14, 15},
		AspectRatios:       []string{"adaptive", "21:9", "16:9", "4:3", "1:1", "3:4", "9:16"},
		TaskModes:          []string{"reference", "edit", "extend"},
		ReferenceRoles: []string{
			"reference_image", "first_frame", "last_frame", "reference_video", "reference_audio", "edit_target", "extend_target",
		},
		Limits: config.MediaLimits{
			MaxImages:            9,
			MaxVideos:            3,
			MaxAudios:            3,
			MaxVideoSecondsTotal: 15,
			MaxAudioSecondsTotal: 15,
		},
	}
}

func assertCompiledPromptMetadata(t *testing.T, got CompiledPrompt, references video.CanonicalReferences) {
	t.Helper()
	if got.PromptVersion != SeedancePromptVersion || got.RequestHash == "" || got.DiagnosticsHash == "" {
		t.Fatalf("compiled metadata = %+v", got)
	}
	want := make([]video.Reference, 0, len(references.References))
	for _, reference := range references.References {
		want = append(want, reference.Reference)
	}
	if !reflect.DeepEqual(got.OrderedReferences, want) {
		t.Fatalf("ordered references = %+v, want %+v", got.OrderedReferences, want)
	}
}

func assertPromptDiagnostic(t *testing.T, diagnostics []PromptDiagnostic, level, code string) {
	t.Helper()
	for _, diagnostic := range diagnostics {
		if diagnostic.Code == code {
			if diagnostic.Level != level {
				t.Fatalf("diagnostic %q level = %q, want %q", code, diagnostic.Level, level)
			}
			if strings.TrimSpace(diagnostic.Message) == "" || strings.TrimSpace(diagnostic.Fix) == "" {
				t.Fatalf("diagnostic %q lacks novice guidance: %+v", code, diagnostic)
			}
			return
		}
	}
	t.Fatalf("diagnostic %q not found in %+v", code, diagnostics)
}

func assertNoPromptDiagnostic(t *testing.T, diagnostics []PromptDiagnostic, code string) {
	t.Helper()
	for _, diagnostic := range diagnostics {
		if diagnostic.Code == code {
			t.Fatalf("unexpected diagnostic %q in %+v", code, diagnostics)
		}
	}
}
