package videoproject

import (
	"context"
	"errors"
	"strings"
	"sync"
	"testing"
	"time"

	imagegen "nine-xing/nx-backend/apps/server/internal/image"
)

func TestAssetCandidateStoreKeepsGeneratingFailedAndReadyHistory(t *testing.T) {
	repository := newMemoryAssetCandidateRepository()
	service := NewAssetWorkflowService(repository, nil)

	first, err := service.CreateExternalCandidate(context.Background(), ExternalAssetCandidateInput{
		ProjectID: "7", TargetKind: "character", TargetID: "11", Prompt: "短发女性标准照",
		ImageAssetID: "101", ImageURL: "https://cdn.example.com/a.png", Source: "upload",
	})
	if err != nil {
		t.Fatal(err)
	}
	repository.forceCandidate(AssetCandidate{ProjectID: "7", TargetType: "character", TargetID: "11", Prompt: "失败尝试", Source: "generated", Status: "failed", ErrorMessage: "模型超时"})
	repository.forceCandidate(AssetCandidate{ProjectID: "7", TargetType: "character", TargetID: "11", Prompt: "正在生成", Source: "generated", Status: "generating"})

	items, err := service.ListCandidates(context.Background(), "7", "character", "11")
	if err != nil {
		t.Fatal(err)
	}
	if len(items) != 3 || first.Status != "ready" || items[0].Status != "ready" || items[1].Status != "failed" || items[2].Status != "generating" {
		t.Fatalf("expected full candidate history, first=%+v items=%+v", first, items)
	}
}

func TestSelectAssetCandidateIsAtomicIdempotentAndValidatesReadiness(t *testing.T) {
	repository := newMemoryAssetCandidateRepository()
	service := NewAssetWorkflowService(repository, nil)
	first := repository.forceCandidate(AssetCandidate{ProjectID: "7", TargetType: "scene", TargetID: "12", ImageURL: "https://cdn.example.com/scene-a.png", Source: "upload", Status: "ready"})
	second := repository.forceCandidate(AssetCandidate{ProjectID: "7", TargetType: "scene", TargetID: "12", ImageURL: "https://cdn.example.com/scene-b.png", Source: "generated", Status: "ready"})
	failed := repository.forceCandidate(AssetCandidate{ProjectID: "7", TargetType: "scene", TargetID: "12", Source: "generated", Status: "failed"})
	private := repository.forceCandidate(AssetCandidate{ProjectID: "7", TargetType: "scene", TargetID: "12", ImageURL: "/api/upload-assets/4", Source: "upload", Status: "ready"})

	selected, err := service.SelectCandidate(context.Background(), "7", first.ID)
	if err != nil {
		t.Fatal(err)
	}
	if !selected.Selected || repository.assetRevision != 1 || repository.compatibilityURL != first.ImageURL {
		t.Fatalf("unexpected first selection: candidate=%+v revision=%d url=%q", selected, repository.assetRevision, repository.compatibilityURL)
	}
	if _, err := service.SelectCandidate(context.Background(), "7", first.ID); err != nil {
		t.Fatal(err)
	}
	if repository.assetRevision != 1 {
		t.Fatalf("reselecting the same candidate must be idempotent, revision=%d", repository.assetRevision)
	}
	if _, err := service.SelectCandidate(context.Background(), "7", second.ID); err != nil {
		t.Fatal(err)
	}
	if repository.assetRevision != 2 || repository.selectedCount("scene", "12") != 1 || repository.compatibilityURL != second.ImageURL {
		t.Fatalf("selection must atomically switch one candidate, revision=%d selected=%d url=%q", repository.assetRevision, repository.selectedCount("scene", "12"), repository.compatibilityURL)
	}
	if _, err := service.SelectCandidate(context.Background(), "7", failed.ID); err == nil || !strings.Contains(err.Error(), "生成完成") {
		t.Fatalf("expected failed candidate rejection, got %v", err)
	}
	if _, err := service.SelectCandidate(context.Background(), "7", private.ID); err == nil || !strings.Contains(err.Error(), "公网") {
		t.Fatalf("expected private URL rejection, got %v", err)
	}
}

func TestSelectAssetCandidateConcurrentSelectionsLeaveOneWinner(t *testing.T) {
	repository := newMemoryAssetCandidateRepository()
	service := NewAssetWorkflowService(repository, nil)
	first := repository.forceCandidate(AssetCandidate{ProjectID: "7", TargetType: "prop", TargetID: "13", ImageURL: "https://cdn.example.com/prop-a.png", Source: "generated", Status: "ready"})
	second := repository.forceCandidate(AssetCandidate{ProjectID: "7", TargetType: "prop", TargetID: "13", ImageURL: "https://cdn.example.com/prop-b.png", Source: "generated", Status: "ready"})

	start := make(chan struct{})
	errorsCh := make(chan error, 2)
	for _, candidateID := range []string{first.ID, second.ID} {
		go func(id string) {
			<-start
			_, err := service.SelectCandidate(context.Background(), "7", id)
			errorsCh <- err
		}(candidateID)
	}
	close(start)
	for range 2 {
		if err := <-errorsCh; err != nil {
			t.Fatal(err)
		}
	}
	if count := repository.selectedCount("prop", "13"); count != 1 {
		t.Fatalf("concurrent selection left %d selected candidates", count)
	}
}

func TestGenerateAssetCandidateLifecycleCreatesRowBeforeProviderCall(t *testing.T) {
	repository := newMemoryAssetCandidateRepository()
	provider := &recordingProjectImageGenerator{result: imagegen.Result{AssetID: 101, ObjectURL: "https://cdn.example.com/generated.png", URL: "https://cdn.example.com/generated.png"}}
	service := NewAssetWorkflowService(repository, provider)
	provider.beforeGenerate = func() {
		items, _ := repository.ListAssetCandidates(context.Background(), "7", "character", "11")
		if len(items) != 1 || items[0].Status != "generating" {
			t.Fatalf("candidate must exist as generating before provider call, got %+v", items)
		}
	}

	ready, err := service.GenerateCandidate(context.Background(), GenerateAssetCandidateInput{
		ProjectID: "7", TargetKind: "character", TargetID: "11", Prompt: "短发女性标准照", Model: "gpt-image-2", Size: "1024x1024",
	})
	if err != nil {
		t.Fatal(err)
	}
	if ready.Status != "ready" || ready.ImageAssetID != "101" || ready.ImageURL != "https://cdn.example.com/generated.png" || provider.calls != 1 {
		t.Fatalf("unexpected generated candidate: %+v calls=%d", ready, provider.calls)
	}
}

func TestGenerateAssetCandidateFailureUpdatesSameRow(t *testing.T) {
	repository := newMemoryAssetCandidateRepository()
	provider := &recordingProjectImageGenerator{err: errors.New("图片模型暂时不可用")}
	service := NewAssetWorkflowService(repository, provider)

	failed, err := service.GenerateCandidate(context.Background(), GenerateAssetCandidateInput{
		ProjectID: "7", TargetKind: "outfit", TargetID: "14", Prompt: "红色风衣标准照",
	})
	if err == nil || !strings.Contains(err.Error(), "暂时不可用") {
		t.Fatalf("expected provider error, got %v", err)
	}
	if failed.Status != "failed" || failed.ErrorMessage != provider.err.Error() {
		t.Fatalf("expected same candidate to record failure, got %+v", failed)
	}
	items, _ := repository.ListAssetCandidates(context.Background(), "7", "outfit", "14")
	if len(items) != 1 || items[0].ID != failed.ID {
		t.Fatalf("failure must update the prepared row instead of creating another, got %+v", items)
	}
}

func TestExternalAssetCandidatesDoNotCallImageProvider(t *testing.T) {
	repository := newMemoryAssetCandidateRepository()
	provider := &recordingProjectImageGenerator{}
	service := NewAssetWorkflowService(repository, provider)

	for _, source := range []string{"upload", "library"} {
		candidate, err := service.CreateExternalCandidate(context.Background(), ExternalAssetCandidateInput{
			ProjectID: "7", TargetKind: "style", TargetID: "15", Prompt: "参考图",
			ImageAssetID: "201", ImageURL: "https://cdn.example.com/" + source + ".png", Source: source,
		})
		if err != nil {
			t.Fatal(err)
		}
		if candidate.Status != "ready" || candidate.Source != source {
			t.Fatalf("unexpected external candidate: %+v", candidate)
		}
	}
	if provider.calls != 0 {
		t.Fatalf("external candidates must not invoke image provider, calls=%d", provider.calls)
	}
}

func TestRecoverStaleAssetCandidatesKeepsHistoryAndIsIdempotent(t *testing.T) {
	repository := newMemoryAssetCandidateRepository()
	now := time.Date(2026, 7, 11, 10, 0, 0, 0, time.UTC)
	old := repository.forceCandidate(AssetCandidate{ProjectID: "7", TargetType: "character", TargetID: "11", Prompt: "旧生成", Source: "generated", Status: "generating", UpdateTime: now.Add(-2 * time.Hour).Format(time.RFC3339)})
	recent := repository.forceCandidate(AssetCandidate{ProjectID: "7", TargetType: "character", TargetID: "11", Prompt: "新生成", Source: "generated", Status: "generating", UpdateTime: now.Add(-5 * time.Minute).Format(time.RFC3339)})
	service := NewAssetWorkflowService(repository, nil)

	count, err := service.RecoverStaleCandidates(context.Background(), now, 30*time.Minute)
	if err != nil {
		t.Fatal(err)
	}
	if count != 1 || repository.candidates[old.ID].Status != "failed" || !strings.Contains(repository.candidates[old.ID].ErrorMessage, "中断") {
		t.Fatalf("expected old generation recovery, count=%d old=%+v", count, repository.candidates[old.ID])
	}
	if repository.candidates[recent.ID].Status != "generating" || repository.candidates[old.ID].Prompt != "旧生成" {
		t.Fatalf("recent candidate and history must remain intact: old=%+v recent=%+v", repository.candidates[old.ID], repository.candidates[recent.ID])
	}
	count, err = service.RecoverStaleCandidates(context.Background(), now, 30*time.Minute)
	if err != nil || count != 0 {
		t.Fatalf("recovery must be idempotent, count=%d err=%v", count, err)
	}
}

type recordingProjectImageGenerator struct {
	result         imagegen.Result
	err            error
	calls          int
	beforeGenerate func()
}

func (provider *recordingProjectImageGenerator) Generate(context.Context, imagegen.GenerateInput) (imagegen.Result, error) {
	provider.calls++
	if provider.beforeGenerate != nil {
		provider.beforeGenerate()
	}
	return provider.result, provider.err
}

type memoryAssetCandidateRepository struct {
	mu               sync.Mutex
	nextID           int
	candidates       map[string]AssetCandidate
	assetRevision    int
	compatibilityURL string
}

func newMemoryAssetCandidateRepository() *memoryAssetCandidateRepository {
	return &memoryAssetCandidateRepository{candidates: map[string]AssetCandidate{}}
}

func (repository *memoryAssetCandidateRepository) CreateAssetCandidate(_ context.Context, write AssetCandidateWrite) (AssetCandidate, error) {
	repository.mu.Lock()
	defer repository.mu.Unlock()
	repository.nextID++
	candidate := AssetCandidate{
		ID: stringID(int64(repository.nextID)), ProjectID: write.ProjectID, TargetType: write.TargetKind, TargetID: write.TargetID,
		Prompt: write.Prompt, ImageAssetID: write.ImageAssetID, ImageURL: write.ImageURL, Source: write.Source,
		GenerationRequestID: write.GenerationRequestID, Status: write.Status, ErrorMessage: write.ErrorMessage,
	}
	repository.candidates[candidate.ID] = candidate
	return candidate, nil
}

func (repository *memoryAssetCandidateRepository) UpdateAssetCandidate(_ context.Context, candidateID string, update AssetCandidateUpdate) (AssetCandidate, error) {
	repository.mu.Lock()
	defer repository.mu.Unlock()
	candidate, ok := repository.candidates[candidateID]
	if !ok {
		return AssetCandidate{}, errors.New("candidate not found")
	}
	candidate.Status = update.Status
	candidate.ImageAssetID = update.ImageAssetID
	candidate.ImageURL = update.ImageURL
	candidate.ErrorMessage = update.ErrorMessage
	repository.candidates[candidateID] = candidate
	return candidate, nil
}

func (repository *memoryAssetCandidateRepository) ListAssetCandidates(_ context.Context, projectID, targetKind, targetID string) ([]AssetCandidate, error) {
	repository.mu.Lock()
	defer repository.mu.Unlock()
	items := []AssetCandidate{}
	for index := 1; index <= repository.nextID; index++ {
		candidate, ok := repository.candidates[stringID(int64(index))]
		if ok && candidate.ProjectID == projectID && candidate.TargetType == targetKind && candidate.TargetID == targetID {
			items = append(items, candidate)
		}
	}
	return items, nil
}

func (repository *memoryAssetCandidateRepository) SelectAssetCandidate(_ context.Context, projectID, candidateID string) (AssetCandidate, error) {
	repository.mu.Lock()
	defer repository.mu.Unlock()
	candidate, ok := repository.candidates[candidateID]
	if !ok || candidate.ProjectID != projectID {
		return AssetCandidate{}, errors.New("候选图不存在")
	}
	if candidate.Status != "ready" {
		return AssetCandidate{}, errors.New("候选图尚未生成完成")
	}
	if !strings.HasPrefix(candidate.ImageURL, "https://") && !strings.HasPrefix(candidate.ImageURL, "http://") {
		return AssetCandidate{}, errors.New("候选图需要文件桶公网地址")
	}
	if candidate.Selected {
		return candidate, nil
	}
	for id, item := range repository.candidates {
		if item.TargetType == candidate.TargetType && item.TargetID == candidate.TargetID {
			item.Selected = id == candidate.ID
			repository.candidates[id] = item
		}
	}
	candidate = repository.candidates[candidate.ID]
	repository.assetRevision++
	repository.compatibilityURL = candidate.ImageURL
	return candidate, nil
}

func (repository *memoryAssetCandidateRepository) RecoverStaleAssetCandidates(_ context.Context, now time.Time, timeout time.Duration) (int, error) {
	repository.mu.Lock()
	defer repository.mu.Unlock()
	count := 0
	for id, candidate := range repository.candidates {
		updatedAt, _ := time.Parse(time.RFC3339, candidate.UpdateTime)
		if candidate.Status == "generating" && !updatedAt.IsZero() && updatedAt.Before(now.Add(-timeout)) {
			candidate.Status = "failed"
			candidate.ErrorMessage = "图片生成因服务重启或超时中断，请手动重试"
			repository.candidates[id] = candidate
			count++
		}
	}
	return count, nil
}

func (repository *memoryAssetCandidateRepository) forceCandidate(candidate AssetCandidate) AssetCandidate {
	repository.mu.Lock()
	defer repository.mu.Unlock()
	repository.nextID++
	candidate.ID = stringID(int64(repository.nextID))
	if candidate.UpdateTime == "" {
		candidate.UpdateTime = time.Now().UTC().Format(time.RFC3339)
	}
	repository.candidates[candidate.ID] = candidate
	return candidate
}

func (repository *memoryAssetCandidateRepository) selectedCount(kind, targetID string) int {
	repository.mu.Lock()
	defer repository.mu.Unlock()
	count := 0
	for _, candidate := range repository.candidates {
		if candidate.TargetType == kind && candidate.TargetID == targetID && candidate.Selected {
			count++
		}
	}
	return count
}
