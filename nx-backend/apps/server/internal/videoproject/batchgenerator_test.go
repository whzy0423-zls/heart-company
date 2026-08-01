package videoproject

import (
	"context"
	"sync"
	"testing"

	"nine-xing/nx-backend/apps/server/internal/video"
)

func TestBatchGenerateUsesPerShotRequestIdentity(t *testing.T) {
	shots := []Shot{
		{ID: "9", ProjectID: "3", Name: "shot 1", OrderNum: 1, Status: "draft"},
		{ID: "10", ProjectID: "3", Name: "shot 2", OrderNum: 2, Status: "draft"},
	}
	shotGenerator := &recordingBatchShotGenerator{}
	batch := NewBatchGenerator(shotGenerator, &staticBatchShotStore{shots: shots})

	result, err := batch.GenerateSelectedShotsWithOptions(context.Background(), "3", []string{"9", "10"}, true, BatchGenerateOptions{
		RequestKeys: map[string]string{
			"9":  "11111111-1111-4111-8111-111111111111",
			"10": "22222222-2222-4222-8222-222222222222",
		},
		CapabilityVersions: map[string]string{
			"9":  "capability-1",
			"10": "capability-2",
		},
	})
	if err != nil {
		t.Fatal(err)
	}
	calls := shotGenerator.snapshot()
	if result.SuccessCount != 2 || len(calls) != 2 {
		t.Fatalf("result=%+v calls=%+v", result, calls)
	}
	byShot := map[string]GenerateShotInput{}
	for _, call := range calls {
		byShot[call.shotID] = call.input
	}
	if byShot["9"].RequestKey != "11111111-1111-4111-8111-111111111111" || byShot["9"].CapabilityVersion != "capability-1" {
		t.Fatalf("shot 9 call = %+v", byShot["9"])
	}
	if byShot["10"].RequestKey != "22222222-2222-4222-8222-222222222222" || byShot["10"].CapabilityVersion != "capability-2" {
		t.Fatalf("shot 10 call = %+v", byShot["10"])
	}
}

func TestBatchGenerateRejectsUnsupportedReferenceRoleBeforeCreateRequest(t *testing.T) {
	capabilities := legacyProjectGenerationCapabilities()
	videoStore := &validatingNormalizedVideoStore{capabilities: capabilities}
	shot := Shot{ID: "9", ProjectID: "3", Name: "shot 1", OrderNum: 1, Status: "draft", VideoModel: "video-ds-2.0", Duration: 10, AspectRatio: "16:9"}
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
			return shot, nil
		},
		markShotGenerating: func(context.Context, string, string, string, []string, []string, []string) error {
			return nil
		},
		startMonitor: func(string, string) {},
	}
	batch := NewBatchGenerator(generator, &staticBatchShotStore{shots: []Shot{shot}})

	result, err := batch.GenerateSelectedShotsWithOptions(context.Background(), "3", []string{"9"}, true, BatchGenerateOptions{
		RequestKeys:        map[string]string{"9": "11111111-1111-4111-8111-111111111111"},
		CapabilityVersions: map[string]string{"9": capabilities.CapabilityVersion},
	})
	if err != nil {
		t.Fatal(err)
	}
	if result.FailedCount != 1 || result.SuccessCount != 0 {
		t.Fatalf("result = %+v", result)
	}
	if videoStore.createRequests != 0 {
		t.Fatalf("create requests = %d, want 0", videoStore.createRequests)
	}
}

type batchShotCall struct {
	shotID string
	input  GenerateShotInput
}

type recordingBatchShotGenerator struct {
	mu    sync.Mutex
	calls []batchShotCall
}

func (g *recordingBatchShotGenerator) GenerateShotWithInput(_ context.Context, shotID string, input GenerateShotInput) (video.Generation, error) {
	g.mu.Lock()
	defer g.mu.Unlock()
	g.calls = append(g.calls, batchShotCall{shotID: shotID, input: input})
	return video.Generation{ID: shotID + "0", Status: "queued"}, nil
}

func (g *recordingBatchShotGenerator) snapshot() []batchShotCall {
	g.mu.Lock()
	defer g.mu.Unlock()
	return append([]batchShotCall(nil), g.calls...)
}

type staticBatchShotStore struct {
	shots []Shot
}

func (s *staticBatchShotStore) ListShots(context.Context, string) ([]Shot, error) {
	return append([]Shot(nil), s.shots...), nil
}

func (s *staticBatchShotStore) GetShot(_ context.Context, shotID string) (Shot, error) {
	for _, shot := range s.shots {
		if shot.ID == shotID {
			return shot, nil
		}
	}
	return Shot{}, nil
}
