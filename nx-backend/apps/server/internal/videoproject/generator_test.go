package videoproject

import (
	"context"
	"errors"
	"testing"

	"nine-xing/nx-backend/apps/server/internal/video"
)

func TestGenerateShotUsesNormalizedRequest(t *testing.T) {
	generateAudio := true
	videoStore := &recordingNormalizedVideoStore{
		capabilities: video.Capabilities{
			Model:                 "video-ds-2.0",
			CapabilityVersion:     "capability-v1",
			SupportsResolution:    true,
			Resolutions:           []string{"1080P"},
			SupportsGenerateAudio: true,
		},
		generation: video.Generation{ID: "42", TaskID: "task-42", Status: "queued"},
	}
	marked := false
	generator := &Generator{
		videoStore: videoStore,
		buildPreview: func(context.Context, string) (ShotPreview, error) {
			preview := ShotPreview{Prompt: "seedance prompt"}
			preview.Validation.IsValid = true
			preview.References = []video.Reference{
				{ID: "image-1", Kind: "image", Role: "reference_image", URL: "https://oss.example.com/character.png", SortOrder: 1},
				{ID: "video-1", Kind: "video", Role: "reference_video", URL: "https://oss.example.com/camera.mp4", SortOrder: 2},
				{ID: "audio-1", Kind: "audio", Role: "reference_audio", URL: "https://oss.example.com/voice.mp3", SortOrder: 3},
			}
			return preview, nil
		},
		loadShot: func(context.Context, string) (Shot, error) {
			return Shot{
				ID:                      "9",
				ProjectID:               "3",
				VideoModel:              "video-ds-2.0",
				Duration:                10,
				AspectRatio:             "9:16",
				VideoResolution:         "1080p",
				SoundAndPictureTogether: "enabled",
			}, nil
		},
		markShotGenerating: func(_ context.Context, shotID, generationID, prompt string, images, videos, audios []string) error {
			marked = true
			if shotID != "9" || generationID != "42" || prompt != "seedance prompt" {
				t.Fatalf("mark args = %q %q %q", shotID, generationID, prompt)
			}
			if len(images) != 1 || len(videos) != 1 || len(audios) != 1 {
				t.Fatalf("mark references = %#v %#v %#v", images, videos, audios)
			}
			return nil
		},
		startMonitor: func(string, string) {},
	}

	generation, err := generator.GenerateShotWithInput(context.Background(), "9", GenerateShotInput{
		RequestKey:        "11111111-1111-4111-8111-111111111111",
		CapabilityVersion: "capability-v1",
	})
	if err != nil {
		t.Fatal(err)
	}
	if generation.ID != "42" || !marked {
		t.Fatalf("generation=%+v marked=%v", generation, marked)
	}
	request := videoStore.request
	if request.Model != "video-ds-2.0" || request.Prompt != "seedance prompt" || request.Duration != 10 || request.AspectRatio != "9:16" {
		t.Fatalf("normalized request basics = %+v", request)
	}
	if request.Resolution != "1080P" || request.GenerateAudio == nil || *request.GenerateAudio != generateAudio {
		t.Fatalf("normalized advanced fields = %+v", request)
	}
	if request.TaskMode != "reference" || request.RequestKey != "11111111-1111-4111-8111-111111111111" || request.CapabilityVersion != "capability-v1" {
		t.Fatalf("normalized request identity = %+v", request)
	}
	if len(request.References) != 3 || request.References[0].URL != "https://oss.example.com/character.png" || request.References[2].Kind != "audio" {
		t.Fatalf("normalized references = %+v", request.References)
	}
	if videoStore.scope.ProjectID != "3" || videoStore.scope.ShotID != "9" {
		t.Fatalf("generation scope = %+v", videoStore.scope)
	}
}

func TestGenerateShotOmitsUnsupportedLegacyAdvancedSettings(t *testing.T) {
	capabilities := legacyProjectGenerationCapabilities()
	videoStore := &recordingNormalizedVideoStore{
		capabilities: capabilities,
		generation:   video.Generation{ID: "42", TaskID: "task-42", Status: "queued"},
	}
	generator := &Generator{
		videoStore: videoStore,
		buildPreview: func(context.Context, string) (ShotPreview, error) {
			preview := ShotPreview{Prompt: "legacy project prompt"}
			preview.Validation.IsValid = true
			return preview, nil
		},
		loadShot: func(context.Context, string) (Shot, error) {
			return Shot{
				ID:                      "9",
				ProjectID:               "3",
				VideoModel:              "video-ds-2.0",
				Duration:                10,
				AspectRatio:             "16:9",
				VideoResolution:         "720p",
				SoundAndPictureTogether: "enabled",
			}, nil
		},
		markShotGenerating: func(context.Context, string, string, string, []string, []string, []string) error {
			return nil
		},
		startMonitor: func(string, string) {},
	}

	if _, err := generator.GenerateShotWithInput(context.Background(), "9", GenerateShotInput{}); err != nil {
		t.Fatal(err)
	}
	if videoStore.request.Resolution != "" {
		t.Fatalf("legacy unsupported resolution = %q, want omitted", videoStore.request.Resolution)
	}
	if videoStore.request.GenerateAudio != nil {
		t.Fatalf("legacy unsupported generateAudio = %v, want omitted", *videoStore.request.GenerateAudio)
	}
}

func TestGenerateShotRejectsUnsupportedReferenceRoleBeforeCreateRequest(t *testing.T) {
	capabilities := legacyProjectGenerationCapabilities()
	videoStore := &validatingNormalizedVideoStore{capabilities: capabilities}
	marked := false
	generator := &Generator{
		videoStore: videoStore,
		buildPreview: func(context.Context, string) (ShotPreview, error) {
			preview := ShotPreview{Prompt: "unsupported role"}
			preview.Validation.IsValid = true
			preview.References = []video.Reference{{
				ID: "image-1", Kind: "image", Role: "first_frame", URL: "https://oss.example.com/frame.png",
			}}
			return preview, nil
		},
		loadShot: func(context.Context, string) (Shot, error) {
			return Shot{ID: "9", ProjectID: "3", VideoModel: "video-ds-2.0", Duration: 10, AspectRatio: "16:9"}, nil
		},
		markShotGenerating: func(context.Context, string, string, string, []string, []string, []string) error {
			marked = true
			return nil
		},
		startMonitor: func(string, string) {},
	}

	_, err := generator.GenerateShotWithInput(context.Background(), "9", GenerateShotInput{
		RequestKey:        "11111111-1111-4111-8111-111111111111",
		CapabilityVersion: capabilities.CapabilityVersion,
	})
	var validationErr *video.ValidationError
	if !errors.As(err, &validationErr) || validationErr.Code != "reference_role_unsupported" {
		t.Fatalf("error = %T %v, want reference_role_unsupported", err, err)
	}
	if videoStore.createRequests != 0 || marked {
		t.Fatalf("create requests = %d, marked = %v", videoStore.createRequests, marked)
	}
}

func legacyProjectGenerationCapabilities() video.Capabilities {
	return video.ResolveCapabilities(video.CapabilityConfig{
		Model:           "video-ds-2.0",
		GatewayContract: video.LegacyFlatContract(),
	})
}

type recordingNormalizedVideoStore struct {
	capabilities video.Capabilities
	generation   video.Generation
	request      video.GenerateRequest
	scope        video.GenerationContext
	calls        int
}

type validatingNormalizedVideoStore struct {
	capabilities   video.Capabilities
	createRequests int
}

func (s *validatingNormalizedVideoStore) Capabilities(string) video.Capabilities {
	return s.capabilities
}

func (s *validatingNormalizedVideoStore) GenerateNormalized(_ context.Context, request video.GenerateRequest, _ video.GenerationContext) (video.Generation, error) {
	if err := video.ValidateGenerateRequest(request, s.capabilities); err != nil {
		return video.Generation{}, err
	}
	s.createRequests++
	return video.Generation{ID: "42", TaskID: "task-42", Status: "queued"}, nil
}

func (s *validatingNormalizedVideoStore) Refresh(context.Context, string) (video.Generation, error) {
	return video.Generation{}, nil
}

func (s *recordingNormalizedVideoStore) Capabilities(string) video.Capabilities {
	return s.capabilities
}

func (s *recordingNormalizedVideoStore) GenerateNormalized(_ context.Context, request video.GenerateRequest, scope video.GenerationContext) (video.Generation, error) {
	s.calls++
	s.request = request
	s.scope = scope
	return s.generation, nil
}

func (s *recordingNormalizedVideoStore) Refresh(context.Context, string) (video.Generation, error) {
	return s.generation, nil
}

func TestValidateGatewayReferenceURLsRejectsLocalUploadProxyURLs(t *testing.T) {
	err := validateGatewayReferenceURLs(
		[]string{"https://oss.example.com/shot.jpg"},
		[]string{"/api/upload-assets/12"},
		[]string{"https://oss.example.com/audio.mp3"},
	)

	if err == nil {
		t.Fatal("expected local upload proxy reference to be rejected before submitting to video gateway")
	}
	if got := err.Error(); got != "参考视频需要阿里云 OSS 文件桶公网 http(s) 地址，请重新上传到文件桶后再生成: /api/upload-assets/12" {
		t.Fatalf("unexpected error: %q", got)
	}
}

func TestValidateGatewayReferenceURLsAcceptsPublicOSSURLs(t *testing.T) {
	if err := validateGatewayReferenceURLs(
		[]string{"https://oss.example.com/shot.jpg"},
		[]string{"https://oss.example.com/ref.mp4"},
		[]string{"https://oss.example.com/audio.mp3"},
	); err != nil {
		t.Fatalf("expected public OSS URLs to be accepted, got %v", err)
	}
}

func TestRequirePublicAssetObjectURLRejectsMissingOrLocalProxyURL(t *testing.T) {
	for _, raw := range []string{"", "/api/upload-assets/9"} {
		if _, err := requirePublicAssetObjectURL("视频抽帧", raw); err == nil {
			t.Fatalf("expected %q to be rejected as non-public objectUrl", raw)
		}
	}
}

func TestRequirePublicAssetObjectURLAcceptsPublicOSSURL(t *testing.T) {
	got, err := requirePublicAssetObjectURL("视频抽帧", "https://oss.example.com/video/frame.jpg")
	if err != nil {
		t.Fatalf("expected public objectUrl to be accepted: %v", err)
	}
	if got != "https://oss.example.com/video/frame.jpg" {
		t.Fatalf("got %q", got)
	}
}

func TestNormalizeShotAssetInputRejectsLocalProxyObjectURL(t *testing.T) {
	input := ShotAssetInput{
		AssetType: "image",
		ObjectURL: "/api/upload-assets/12",
		Name:      "本地代理图片",
	}

	err := normalizeShotAssetInput(&input)

	if err == nil {
		t.Fatal("expected local upload proxy URL to be rejected for shot reference assets")
	}
	if got := err.Error(); got != "分镜参考素材需要阿里云 OSS 文件桶公网 objectUrl，请配置 OSS_PUBLIC_URL/文件桶公网访问后重新上传" {
		t.Fatalf("unexpected error: %q", got)
	}
}

func TestNormalizeShotAssetInputAcceptsPublicObjectURL(t *testing.T) {
	input := ShotAssetInput{
		AssetType: "image",
		ObjectURL: "https://oss.example.com/shot.jpg",
		Name:      "公网图片",
	}

	if err := normalizeShotAssetInput(&input); err != nil {
		t.Fatalf("expected public objectUrl to be accepted: %v", err)
	}
}

func TestNormalizeShotAssetInputDefaultsLegacyRoleAndPreservesCanonicalMetadata(t *testing.T) {
	input := ShotAssetInput{
		AssetType:  "video",
		ObjectURL:  "https://oss.example.com/shot.mp4",
		SortOrder:  3,
		SourceType: " upload ",
		SourceID:   " asset-7 ",
		UsageNote:  " 参考运镜 ",
	}

	if err := normalizeShotAssetInput(&input); err != nil {
		t.Fatal(err)
	}
	if input.ReferenceRole != "reference_video" || input.SortOrder != 3 {
		t.Fatalf("normalized role/order = %+v", input)
	}
	if input.SourceType != "upload" || input.SourceID != "asset-7" || input.UsageNote != "参考运镜" {
		t.Fatalf("normalized metadata = %+v", input)
	}
}

func TestNormalizeShotAssetInputRejectsRoleKindMismatch(t *testing.T) {
	input := ShotAssetInput{
		AssetType:     "image",
		ObjectURL:     "https://oss.example.com/shot.png",
		ReferenceRole: "edit_target",
	}

	if err := normalizeShotAssetInput(&input); err == nil || err.Error() != "分镜素材类型与用途不匹配" {
		t.Fatalf("error = %v, want role-kind mismatch", err)
	}
}
