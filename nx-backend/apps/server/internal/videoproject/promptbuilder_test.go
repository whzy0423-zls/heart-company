package videoproject

import (
	"context"
	"encoding/json"
	"os"
	"reflect"
	"strings"
	"testing"

	"nine-xing/nx-backend/apps/server/internal/video"
)

func TestBuildPreviewUsesCanonicalReferenceOrder(t *testing.T) {
	capabilities := fullPromptCapabilities()
	shot := Shot{
		ID:                 "9",
		ProjectID:          "3",
		VideoModel:         "video-ds-2.0",
		Duration:           10,
		AspectRatio:        "16:9",
		DynamicDescription: "将天空修改为黄昏",
		ShotAssets: []ShotAsset{
			{ID: "30", ShotID: "9", AssetType: "video", ReferenceRole: "reference_video", SortOrder: 1, ObjectURL: "https://cdn.example.com/camera.mp4", SourceType: "camera", SourceID: "camera-1"},
			{ID: "20", ShotID: "9", AssetType: "image", ReferenceRole: "reference_image", SortOrder: 1, ObjectURL: "https://cdn.example.com/station.png", SourceType: "scene", SourceID: "雨夜车站"},
			{ID: "10", ShotID: "9", AssetType: "image", ReferenceRole: "reference_image", SortOrder: 1, ObjectURL: "https://cdn.example.com/character.png", SourceType: "character", SourceID: "小夏"},
			{ID: "40", ShotID: "9", AssetType: "audio", ReferenceRole: "reference_audio", SortOrder: 0, ObjectURL: "https://cdn.example.com/voice.mp3", SourceType: "voice", SourceID: "voice-1"},
			{ID: "50", ShotID: "9", AssetType: "video", ReferenceRole: "edit_target", SortOrder: 2, ObjectURL: "https://cdn.example.com/edit.mp4", SourceType: "upload", SourceID: "edit-1"},
		},
	}
	builder := &PromptBuilder{
		loadShot: func(context.Context, string) (Shot, error) { return shot, nil },
		loadProject: func(context.Context, string) (Project, error) {
			return Project{ID: "3", StyleGuide: "电影感蓝色霓虹"}, nil
		},
		listCharacters: func(context.Context, string) ([]Character, error) { return nil, nil },
		loadScene:      func(context.Context, string) (Scene, error) { return Scene{}, nil },
		previousShot:   func(context.Context, string, int) (Shot, bool, error) { return Shot{}, false, nil },
		capabilities:   func(string) video.Capabilities { return capabilities },
	}

	preview, err := builder.BuildPreview(context.Background(), "9")
	if err != nil {
		t.Fatal(err)
	}
	wantURLs := []string{
		"https://cdn.example.com/voice.mp3",
		"https://cdn.example.com/character.png",
		"https://cdn.example.com/station.png",
		"https://cdn.example.com/camera.mp4",
		"https://cdn.example.com/edit.mp4",
	}
	gotURLs := make([]string, 0, len(preview.References))
	for _, reference := range preview.References {
		gotURLs = append(gotURLs, reference.URL)
	}
	if !reflect.DeepEqual(gotURLs, wantURLs) {
		t.Fatalf("preview reference URLs = %#v, want %#v", gotURLs, wantURLs)
	}
	if !strings.HasPrefix(preview.Prompt, "严格编辑视频2，将天空修改为黄昏") {
		t.Fatalf("edit target numbering drifted in prompt: %q", preview.Prompt)
	}
	if preview.PromptVersion != SeedancePromptVersion || preview.RequestHash == "" || preview.DiagnosticsHash == "" {
		t.Fatalf("preview prompt metadata = %+v", preview)
	}
	if !preview.Validation.IsValid {
		t.Fatalf("preview validation = %+v", preview.Validation)
	}
}

func TestBuildPreviewKeepsAutomaticReferencesAlongsideExplicitAssets(t *testing.T) {
	capabilities := fullPromptCapabilities()
	shot := Shot{
		ID:                  "9",
		ProjectID:           "3",
		VideoModel:          "video-ds-2.0",
		Duration:            10,
		AspectRatio:         "16:9",
		ActionDescription:   "小夏走入雨夜车站",
		CharacterIDs:        []string{"7"},
		ImageReferenceModes: []string{"prev_frame", "character_ref", "scene_ref"},
		VideoReferenceMode:  "scene_demo",
		SceneID:             "8",
		ShotAssets: []ShotAsset{
			{ID: "10", AssetType: "image", ReferenceRole: "reference_image", ObjectURL: "https://cdn.example.com/explicit.png", SourceType: "upload", SourceID: "10", UsageNote: "服饰细节"},
			{ID: "20", AssetType: "video", ReferenceRole: "reference_video", ObjectURL: "https://cdn.example.com/explicit.mp4", SourceType: "upload", SourceID: "20", UsageNote: "人物动作"},
		},
	}
	character := Character{ID: "7", Name: "小夏", IsMain: true, ReferenceImageURL: "https://cdn.example.com/character.png"}
	scene := Scene{ID: "8", Name: "雨夜车站", ReferenceImageURL: "https://cdn.example.com/scene.png", ReferenceVideoURL: "https://cdn.example.com/scene.mp4"}
	previous := Shot{ID: "6", EndFrameURL: "https://cdn.example.com/previous-frame.png"}
	builder := &PromptBuilder{
		loadShot:       func(context.Context, string) (Shot, error) { return shot, nil },
		loadProject:    func(context.Context, string) (Project, error) { return Project{ID: "3"}, nil },
		listCharacters: func(context.Context, string) ([]Character, error) { return []Character{character}, nil },
		loadScene:      func(context.Context, string) (Scene, error) { return scene, nil },
		previousShot:   func(context.Context, string, int) (Shot, bool, error) { return previous, true, nil },
		capabilities:   func(string) video.Capabilities { return capabilities },
	}

	preview, err := builder.BuildPreview(context.Background(), "9")
	if err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(preview.Images, []string{
		"https://cdn.example.com/explicit.png",
		"https://cdn.example.com/previous-frame.png",
		"https://cdn.example.com/character.png",
		"https://cdn.example.com/scene.png",
	}) {
		t.Fatalf("preview images = %#v", preview.Images)
	}
	if !reflect.DeepEqual(preview.Videos, []string{
		"https://cdn.example.com/explicit.mp4",
		"https://cdn.example.com/scene.mp4",
	}) {
		t.Fatalf("preview videos = %#v", preview.Videos)
	}
	for _, want := range []string{"图片1", "图片2", "图片3", "图片4", "视频1", "视频2"} {
		if !strings.Contains(preview.Prompt, want) {
			t.Fatalf("prompt missing %q: %q", want, preview.Prompt)
		}
	}
}

func TestBuildPreviewReportsUnsupportedGenerationParameters(t *testing.T) {
	capabilities := legacyProjectGenerationCapabilities()
	shot := Shot{
		ID:                "9",
		ProjectID:         "3",
		VideoModel:        "video-ds-2.0",
		Duration:          7,
		AspectRatio:       "4:3",
		ActionDescription: "人物走入车站",
	}
	builder := &PromptBuilder{
		loadShot:       func(context.Context, string) (Shot, error) { return shot, nil },
		loadProject:    func(context.Context, string) (Project, error) { return Project{ID: "3"}, nil },
		listCharacters: func(context.Context, string) ([]Character, error) { return nil, nil },
		loadScene:      func(context.Context, string) (Scene, error) { return Scene{}, nil },
		previousShot:   func(context.Context, string, int) (Shot, bool, error) { return Shot{}, false, nil },
		capabilities:   func(string) video.Capabilities { return capabilities },
	}

	preview, err := builder.BuildPreview(context.Background(), "9")
	if err != nil {
		t.Fatal(err)
	}
	if preview.Validation.IsValid || len(preview.Validation.Errors) == 0 {
		t.Fatalf("preview must block unsupported generation parameters: %+v", preview.Validation)
	}
	assertPromptDiagnostic(t, preview.Diagnostics, "error", "duration_unsupported")
	assertPromptDiagnostic(t, preview.Diagnostics, "error", "aspect_ratio_unsupported")

	invalidDiagnosticsHash := preview.DiagnosticsHash
	shot.Duration = 10
	shot.AspectRatio = "16:9"
	validPreview, err := builder.BuildPreview(context.Background(), "9")
	if err != nil {
		t.Fatal(err)
	}
	if !validPreview.Validation.IsValid {
		t.Fatalf("valid preview = %+v", validPreview.Validation)
	}
	if validPreview.DiagnosticsHash == invalidDiagnosticsHash {
		t.Fatalf("generation-parameter diagnostics must change diagnostics hash: %q", validPreview.DiagnosticsHash)
	}
}

func TestBuildPreviewEmptyCollectionsRemainJSONArrays(t *testing.T) {
	capabilities := legacyProjectGenerationCapabilities()
	shot := Shot{ID: "9", ProjectID: "3", VideoModel: "video-ds-2.0", Duration: 10, AspectRatio: "16:9", ActionDescription: "人物走入车站"}
	builder := &PromptBuilder{
		loadShot:       func(context.Context, string) (Shot, error) { return shot, nil },
		loadProject:    func(context.Context, string) (Project, error) { return Project{ID: "3"}, nil },
		listCharacters: func(context.Context, string) ([]Character, error) { return nil, nil },
		loadScene:      func(context.Context, string) (Scene, error) { return Scene{}, nil },
		previousShot:   func(context.Context, string, int) (Shot, bool, error) { return Shot{}, false, nil },
		capabilities:   func(string) video.Capabilities { return capabilities },
	}

	preview, err := builder.BuildPreview(context.Background(), "9")
	if err != nil {
		t.Fatal(err)
	}
	encoded, err := json.Marshal(preview)
	if err != nil {
		t.Fatal(err)
	}
	for _, field := range []string{"audios", "diagnostics", "images", "references", "videos", "errors", "warnings"} {
		if !strings.Contains(string(encoded), `"`+field+`":[]`) {
			t.Fatalf("preview field %q must be a JSON array: %s", field, encoded)
		}
	}
}

func TestPromptIndicesMatchGatewayPayload(t *testing.T) {
	canonical := canonicalPromptReferences(t, []video.Reference{
		{ID: "30", Kind: "video", Role: "reference_video", URL: "https://cdn.example.com/video-2.mp4", SortOrder: 2},
		{ID: "20", Kind: "image", Role: "reference_image", URL: "https://cdn.example.com/image-2.png", SortOrder: 1},
		{ID: "10", Kind: "image", Role: "reference_image", URL: "https://cdn.example.com/image-1.png", SortOrder: 1},
		{ID: "40", Kind: "audio", Role: "reference_audio", URL: "https://cdn.example.com/audio-1.mp3", SortOrder: 0},
		{ID: "50", Kind: "video", Role: "reference_video", URL: "https://cdn.example.com/video-1.mp4", SortOrder: 1},
	})
	compiled := CompileSeedancePrompt(PromptInput{
		Mode:       "reference",
		Subject:    "小夏",
		Action:     "走入站台",
		References: canonical,
	}, fullPromptCapabilities())
	request := video.GenerateRequest{
		Model:       "video-ds-2.0",
		Prompt:      compiled.Prompt,
		Duration:    10,
		AspectRatio: "16:9",
		TaskMode:    "reference",
		References:  compiled.OrderedReferences,
	}
	payload, err := video.MapGatewayPayload(request, canonical, video.LegacyFlatContract())
	if err != nil {
		t.Fatal(err)
	}
	for _, reference := range canonical.References {
		if !strings.Contains(compiled.Prompt, reference.Label) {
			t.Fatalf("prompt does not mention canonical label %q: %q", reference.Label, compiled.Prompt)
		}
	}
	assertPayloadStringSlice(t, payload, "images", []string{
		"https://cdn.example.com/image-1.png",
		"https://cdn.example.com/image-2.png",
	})
	assertPayloadStringSlice(t, payload, "videos", []string{
		"https://cdn.example.com/video-1.mp4",
		"https://cdn.example.com/video-2.mp4",
	})
	assertPayloadStringSlice(t, payload, "audios", []string{"https://cdn.example.com/audio-1.mp3"})
}

func TestShotReferenceSchemaSupportsCanonicalOrdering(t *testing.T) {
	raw, err := os.ReadFile("../db/schema.sql")
	if err != nil {
		t.Fatal(err)
	}
	schema := string(raw)
	for _, fragment := range []string{
		"ALTER TABLE video_shot_assets ADD COLUMN IF NOT EXISTS reference_role TEXT NOT NULL DEFAULT ''",
		"ALTER TABLE video_shot_assets ADD COLUMN IF NOT EXISTS sort_order INT NOT NULL DEFAULT 0",
		"ALTER TABLE video_shot_assets ADD COLUMN IF NOT EXISTS source_type TEXT NOT NULL DEFAULT ''",
		"ALTER TABLE video_shot_assets ADD COLUMN IF NOT EXISTS source_id TEXT NOT NULL DEFAULT ''",
		"ALTER TABLE video_shot_assets ADD COLUMN IF NOT EXISTS usage_note TEXT NOT NULL DEFAULT ''",
		"CREATE INDEX IF NOT EXISTS idx_video_shot_assets_order ON video_shot_assets(shot_id, sort_order, id)",
	} {
		if !strings.Contains(schema, fragment) {
			t.Fatalf("schema missing %q", fragment)
		}
	}
	storeSource, err := os.ReadFile("videoproject.go")
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(storeSource), "ORDER BY sort_order ASC, id ASC") {
		t.Fatal("shot asset query must use canonical sort_order,id ordering")
	}
}

func assertPayloadStringSlice(t *testing.T, payload map[string]any, field string, want []string) {
	t.Helper()
	got, ok := payload[field].([]string)
	if !ok {
		t.Fatalf("payload[%q] = %T %#v, want []string", field, payload[field], payload[field])
	}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("payload[%q] = %#v, want %#v", field, got, want)
	}
}

func TestBuildReferenceImagesPriorityAndLimit(t *testing.T) {
	b := &PromptBuilder{}
	prev := &Shot{EndFrameURL: "prev.jpg"}
	scene := &Scene{ReferenceImageURL: "scene.jpg"}
	chars := []Character{
		{ReferenceImageURL: "side.jpg"},
		{ReferenceImageURL: "main.jpg", IsMain: true},
	}
	shot := Shot{ImageReferenceModes: []string{"prev_frame", "character_ref", "scene_ref"}}

	got := b.buildReferenceImages(shot, chars, scene, prev)
	want := []string{"prev.jpg", "main.jpg", "side.jpg", "scene.jpg"}

	if !reflect.DeepEqual(got, want) {
		t.Fatalf("got %#v want %#v", got, want)
	}
}

func TestBuildReferenceImagesDeduplicatesAndCapsAtFour(t *testing.T) {
	b := &PromptBuilder{}
	prev := &Shot{EndFrameURL: "same.jpg"}
	scene := &Scene{ReferenceImageURL: "scene.jpg"}
	chars := []Character{
		{ReferenceImageURL: "same.jpg", IsMain: true},
		{ReferenceImageURL: "side-1.jpg"},
		{ReferenceImageURL: "side-2.jpg"},
		{ReferenceImageURL: "side-3.jpg"},
	}
	shot := Shot{ImageReferenceModes: []string{"prev_frame", "character_ref", "scene_ref"}}

	got := b.buildReferenceImages(shot, chars, scene, prev)
	want := []string{"same.jpg", "side-1.jpg", "side-2.jpg", "side-3.jpg"}

	if !reflect.DeepEqual(got, want) {
		t.Fatalf("got %#v want %#v", got, want)
	}
}

func TestBuildReferenceImagesPrioritizesExplicitShotAssets(t *testing.T) {
	b := &PromptBuilder{}
	prev := &Shot{EndFrameURL: "prev.jpg"}
	scene := &Scene{ReferenceImageURL: "scene.jpg"}
	chars := []Character{
		{ReferenceImageURL: "main.jpg", IsMain: true},
		{ReferenceImageURL: "side-1.jpg"},
		{ReferenceImageURL: "side-2.jpg"},
	}
	shot := Shot{
		ImageReferenceModes: []string{"prev_frame", "character_ref", "scene_ref"},
		ShotAssets: []ShotAsset{
			{AssetType: "image", ObjectURL: "explicit-shot-reference.jpg"},
		},
	}

	got := b.buildReferenceImages(shot, chars, scene, prev)

	if !reflect.DeepEqual(got, []string{"explicit-shot-reference.jpg", "prev.jpg", "main.jpg", "side-1.jpg"}) {
		t.Fatalf("explicit shot asset should be kept ahead of automatic references, got %#v", got)
	}
}

func TestBuildReferenceVideosPrioritizesExplicitShotAssets(t *testing.T) {
	b := &PromptBuilder{}
	scene := &Scene{ReferenceVideoURL: "scene-demo.mp4"}
	shot := Shot{
		VideoReferenceMode: "scene_demo",
		ShotAssets: []ShotAsset{
			{AssetType: "video", ObjectURL: "explicit-video-1.mp4"},
			{AssetType: "video", ObjectURL: "explicit-video-2.mp4"},
		},
	}

	got := b.buildReferenceVideos(shot, scene, nil)
	want := []string{"explicit-video-1.mp4", "explicit-video-2.mp4"}

	if !reflect.DeepEqual(got, want) {
		t.Fatalf("explicit shot videos should be kept ahead of automatic video references, got %#v want %#v", got, want)
	}
}
