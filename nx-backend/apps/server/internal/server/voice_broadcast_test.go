package server

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
	"time"

	"nine-xing/nx-backend/apps/server/internal/auth"
	"nine-xing/nx-backend/apps/server/internal/bailianconfig"
	"nine-xing/nx-backend/apps/server/internal/config"
	"nine-xing/nx-backend/apps/server/internal/voicebroadcastconfig"
	"nine-xing/nx-backend/apps/server/internal/xinzhili"
)

func TestVoiceBroadcastPreferenceDefaultsOffPersistsAndIsUserScoped(t *testing.T) {
	store := newMemoryVoiceBroadcastPreferenceStore()
	s := &Server{
		voiceBroadcastPreferences: store,
		voiceBroadcastConfigLoader: func(context.Context) (voiceBroadcastConfig, error) {
			return voiceBroadcastConfig{Enabled: true, Provider: voiceBroadcastProviderBailian, APIKey: "sk-test", Model: "qwen3-tts-instruct-flash", Voice: "Cherry"}, nil
		},
	}

	get := func(userID int64) voiceBroadcastCapability {
		r := httptest.NewRequest(http.MethodGet, "/api/app/voice-broadcast", nil)
		r = r.WithContext(contextWithAppUser(r.Context(), auth.UserInfo{ID: userID}))
		w := httptest.NewRecorder()
		s.appVoiceBroadcast(w, r)
		if w.Code != http.StatusOK {
			t.Fatalf("GET user %d: status=%d body=%s", userID, w.Code, w.Body.String())
		}
		var envelope struct {
			Data voiceBroadcastCapability `json:"data"`
		}
		if err := json.Unmarshal(w.Body.Bytes(), &envelope); err != nil {
			t.Fatal(err)
		}
		return envelope.Data
	}
	put := func(userID int64, enabled bool) {
		body := strings.NewReader(`{"enabled":true}`)
		if !enabled {
			body = strings.NewReader(`{"enabled":false}`)
		}
		r := httptest.NewRequest(http.MethodPut, "/api/app/voice-broadcast", body)
		r = r.WithContext(contextWithAppUser(r.Context(), auth.UserInfo{ID: userID}))
		w := httptest.NewRecorder()
		s.appVoiceBroadcast(w, r)
		if w.Code != http.StatusOK {
			t.Fatalf("PUT user %d: status=%d body=%s", userID, w.Code, w.Body.String())
		}
	}

	if got := get(7); got.UserEnabled {
		t.Fatalf("default user preference = true, want false")
	}
	put(7, true)
	if got := get(7); !got.UserEnabled || !got.Enabled || got.Provider != xinzhili.TTSProviderBailian || got.Voice != "Cherry" {
		t.Fatalf("persisted capability = %+v", got)
	}
	if got := get(8); got.UserEnabled {
		t.Fatalf("user preference leaked across users: %+v", got)
	}
}

func TestVoiceBroadcastRouteIsRegistered(t *testing.T) {
	handler := New(config.Env{JWTSecret: "test-secret"}, nil)
	if shutdowner, ok := handler.(interface{ Shutdown() }); ok {
		defer shutdowner.Shutdown()
	}
	req := httptest.NewRequest(http.MethodGet, voiceBroadcastPreferencePath, nil)
	res := httptest.NewRecorder()
	handler.ServeHTTP(res, req)
	if res.Code != http.StatusUnauthorized {
		t.Fatalf("voice broadcast route status=%d body=%s, want unauthorized", res.Code, res.Body.String())
	}
}

func TestVoiceBroadcastCapabilityReportsProviderUnavailableWithoutSecrets(t *testing.T) {
	s := &Server{
		voiceBroadcastPreferences: newMemoryVoiceBroadcastPreferenceStore(),
		voiceBroadcastConfigLoader: func(context.Context) (voiceBroadcastConfig, error) {
			return voiceBroadcastConfig{}, errors.New("provider unavailable")
		},
	}
	r := httptest.NewRequest(http.MethodGet, "/api/app/voice-broadcast", nil)
	r = r.WithContext(contextWithAppUser(r.Context(), auth.UserInfo{ID: 7}))
	w := httptest.NewRecorder()
	s.appVoiceBroadcast(w, r)
	if w.Code != http.StatusOK {
		t.Fatalf("status=%d body=%s", w.Code, w.Body.String())
	}
	var envelope struct {
		Data voiceBroadcastCapability `json:"data"`
	}
	if err := json.Unmarshal(w.Body.Bytes(), &envelope); err != nil {
		t.Fatal(err)
	}
	if envelope.Data.Enabled || envelope.Data.ProviderAvailable {
		t.Fatalf("unavailable capability = %+v", envelope.Data)
	}
}

func TestVoiceBroadcastCapabilityNormalizesAdminBailianProvider(t *testing.T) {
	preferences := newMemoryVoiceBroadcastPreferenceStore()
	if err := preferences.Set(context.Background(), 8, true); err != nil {
		t.Fatal(err)
	}
	s := &Server{
		voiceBroadcastPreferences: preferences,
		voiceBroadcastConfigLoader: func(context.Context) (voiceBroadcastConfig, error) {
			return voiceBroadcastConfig{
				Enabled: true, Provider: "aliyun-bailian", APIKey: "sk-test",
				Model: voiceBroadcastDefaultModel, Voice: voiceBroadcastDefaultVoice,
			}, nil
		},
	}
	capability := s.voiceBroadcastCapabilityForUser(context.Background(), 8)
	if capability.Provider != xinzhili.TTSProviderBailian || !capability.ProviderAvailable {
		t.Fatalf("capability=%+v, want normalized available Bailian", capability)
	}
}

func TestXinzhiliRealtimeVoiceBroadcastRejectsNonBailianProvider(t *testing.T) {
	preferences := newMemoryVoiceBroadcastPreferenceStore()
	if err := preferences.Set(context.Background(), 7, true); err != nil {
		t.Fatal(err)
	}
	s := &Server{
		voiceBroadcastPreferences: preferences,
		voiceBroadcastConfigLoader: func(context.Context) (voiceBroadcastConfig, error) {
			return voiceBroadcastConfig{
				Enabled: true, Provider: "minimax", APIKey: "sk-test",
				Model: "speech-02", Voice: "female",
			}, nil
		},
	}
	_, enabled, controlled := s.xinzhiliRealtimeVoiceBroadcastConfig(context.Background(), 7)
	if !controlled || enabled {
		t.Fatalf("controlled=%t enabled=%t, want controlled=true enabled=false", controlled, enabled)
	}
}

func TestNormalizeVoiceBroadcastProviderAcceptsAdminAlias(t *testing.T) {
	for _, raw := range []string{"bailian", "aliyun-bailian", "aliyun_bailian", "dashscope"} {
		if got := normalizeVoiceBroadcastProvider(raw); got != voiceBroadcastProviderBailian {
			t.Fatalf("provider %q normalized to %q", raw, got)
		}
	}
}

func TestLoadVoiceBroadcastConfigUsesAdminSingleton(t *testing.T) {
	key := "admin-secret"
	store := &memoryVoiceBroadcastConfigStore{
		cfg: voicebroadcastconfig.Config{
			Version:      3,
			Enabled:      true,
			Provider:     voicebroadcastconfig.DefaultProvider,
			Region:       "cn-shanghai",
			WorkspaceID:  "workspace-1",
			Model:        "qwen3-tts-instruct-flash",
			DefaultVoice: "Cherry",
			CurrentVoice: "Serena",
			APIKey:       key,
		},
		found: true,
	}
	s := &Server{voiceBroadcastConfig: store}

	got, err := s.loadVoiceBroadcastConfig(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if !got.Enabled || got.Provider != voicebroadcastconfig.DefaultProvider || got.APIKey != key {
		t.Fatalf("loaded config = %+v", got)
	}
	if got.Endpoint != voicebroadcastconfig.DefaultEndpoint || got.GroupID != "workspace-1" || got.Voice != "Serena" {
		t.Fatalf("admin fields were not mapped = %+v", got)
	}
	if got.Region != "cn-shanghai" || voiceBroadcastTTSConfig(got).Region != "cn-shanghai" {
		t.Fatalf("admin region was not carried into runtime TTS config = %+v", got)
	}
}

func TestLoadVoiceBroadcastConfigFallsBackToSharedBailianCredential(t *testing.T) {
	store := &memoryVoiceBroadcastConfigStore{
		cfg: voicebroadcastconfig.Config{
			Version:      2,
			Enabled:      true,
			Provider:     voicebroadcastconfig.DefaultProvider,
			WorkspaceID:  "workspace-1",
			Model:        voicebroadcastconfig.DefaultModel,
			CurrentVoice: voicebroadcastconfig.DefaultFemaleVoice,
		},
		found: true,
	}
	s := &Server{
		voiceBroadcastConfig: store,
		bailianCredentials: &memoryBailianCredentialStore{
			cfg:   bailianconfig.Config{Version: 7, APIKey: "sk-shared-voice"},
			found: true,
		},
	}

	got, err := s.loadVoiceBroadcastConfig(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if got.APIKey != "sk-shared-voice" {
		t.Fatalf("APIKey=%q, want shared Bailian credential", got.APIKey)
	}
}

func TestLoadVoiceBroadcastConfigPrefersAdminCredentialOverShared(t *testing.T) {
	s := &Server{
		voiceBroadcastConfig: &memoryVoiceBroadcastConfigStore{
			cfg: voicebroadcastconfig.Config{
				Enabled:      true,
				Provider:     voicebroadcastconfig.DefaultProvider,
				Model:        voicebroadcastconfig.DefaultModel,
				CurrentVoice: voicebroadcastconfig.DefaultFemaleVoice,
				APIKey:       "sk-admin-voice",
			},
			found: true,
		},
		bailianCredentials: &memoryBailianCredentialStore{
			cfg:   bailianconfig.Config{APIKey: "sk-shared-voice"},
			found: true,
		},
	}

	got, err := s.loadVoiceBroadcastConfig(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if got.APIKey != "sk-admin-voice" {
		t.Fatalf("APIKey=%q, want admin credential", got.APIKey)
	}
}

func TestVoiceBroadcastStreamOrdersSegmentsAndFlushesFinalText(t *testing.T) {
	provider := &recordingVoiceBroadcastProvider{}
	var got []voiceBroadcastSegment
	stream := newVoiceBroadcastStream(context.Background(), "reply-1", provider, func(segment voiceBroadcastSegment) error {
		got = append(got, segment)
		return nil
	})
	if err := stream.Push("这是第一句内容，需要播报。"); err != nil {
		t.Fatal(err)
	}
	if err := stream.Push("这是第二句内容，需要播报"); err != nil {
		t.Fatal(err)
	}
	if err := stream.Close(); err != nil {
		t.Fatal(err)
	}
	if len(got) != 2 || got[0].SegmentSeq != 0 || got[1].SegmentSeq != 1 || !got[1].Final {
		t.Fatalf("segments = %+v", got)
	}
	if got[0].ReplyID != "reply-1" || string(got[0].Audio) != "这是第一句内容，需要播报。" {
		t.Fatalf("first segment = %+v", got[0])
	}
}

func TestVoiceBroadcastStreamCancellationInvalidatesQueuedSegments(t *testing.T) {
	provider := &blockingVoiceBroadcastProvider{started: make(chan struct{}), release: make(chan struct{})}
	var mu sync.Mutex
	var got []voiceBroadcastSegment
	stream := newVoiceBroadcastStream(context.Background(), "reply-2", provider, func(segment voiceBroadcastSegment) error {
		mu.Lock()
		got = append(got, segment)
		mu.Unlock()
		return nil
	})
	if err := stream.Push("这是第一句内容，需要播报。第二句内容，需要播报。"); err != nil {
		t.Fatal(err)
	}
	select {
	case <-provider.started:
	case <-time.After(time.Second):
		t.Fatal("provider did not start")
	}
	stream.Cancel()
	close(provider.release)
	if err := stream.Wait(); !errors.Is(err, context.Canceled) {
		t.Fatalf("Wait error=%v, want context.Canceled", err)
	}
	mu.Lock()
	defer mu.Unlock()
	if len(got) != 0 {
		t.Fatalf("canceled stream emitted segments: %+v", got)
	}
}

func TestVoiceBroadcastStreamPushDoesNotBlockWhenProviderIsSlow(t *testing.T) {
	provider := &blockingVoiceBroadcastProvider{started: make(chan struct{}), release: make(chan struct{})}
	stream := newVoiceBroadcastStream(context.Background(), "reply-queue", provider, nil)
	defer func() {
		stream.Cancel()
		close(provider.release)
		_ = stream.Wait()
	}()

	// The provider holds the first item while the remaining sentences fill the
	// bounded queue. Push must still return promptly so text generation can
	// continue independently of optional voice synthesis.
	text := strings.Repeat("这是一个足够长的测试句子。", voiceBroadcastQueueSize+4)
	pushed := make(chan error, 1)
	go func() { pushed <- stream.Push(text) }()
	select {
	case <-provider.started:
	case <-time.After(time.Second):
		t.Fatal("voice provider did not start")
	}
	select {
	case err := <-pushed:
		if err != nil {
			t.Fatalf("Push returned error=%v", err)
		}
	case <-time.After(200 * time.Millisecond):
		t.Fatal("Push blocked behind a full voice queue")
	}
}

func TestVoiceBroadcastStreamCloseAfterCancelReturnsPromptly(t *testing.T) {
	provider := &blockingVoiceBroadcastProvider{started: make(chan struct{}), release: make(chan struct{})}
	stream := newVoiceBroadcastStream(context.Background(), "reply-3", provider, nil)
	if err := stream.Push("这是第一句内容，需要播报。第二句内容，需要播报。"); err != nil {
		t.Fatal(err)
	}
	select {
	case <-provider.started:
	case <-time.After(time.Second):
		t.Fatal("provider did not start")
	}
	stream.Cancel()
	done := make(chan error, 1)
	go func() { done <- stream.Close() }()
	select {
	case err := <-done:
		if !errors.Is(err, context.Canceled) {
			t.Fatalf("Close error=%v, want context.Canceled", err)
		}
	case <-time.After(time.Second):
		t.Fatal("Close blocked after cancellation")
	}
}

func TestVoiceBroadcastSSEPayloadCarriesStableEnvelope(t *testing.T) {
	payload := voiceBroadcastSegment{ReplyID: "r", SegmentSeq: 3, MIME: "audio/mpeg", Audio: []byte("mp3"), Final: true}
	encoded := payload.ssePayload()
	if encoded.AudioBase64 != base64.StdEncoding.EncodeToString([]byte("mp3")) || encoded.ByteLength != 3 || !encoded.Final {
		t.Fatalf("payload = %+v", encoded)
	}
}

func TestAppChatStreamEmitsVoiceSideChannelWithoutChangingTextEvents(t *testing.T) {
	store := newFakeAppChatStreamStore()
	s := newAppChatStreamServer(store, successfulAppChatGenerator("这是第一句内容，需要播报。"))
	s.voiceBroadcastPreferences = newMemoryVoiceBroadcastPreferenceStore()
	if err := s.voiceBroadcastPreferences.Set(context.Background(), 7, true); err != nil {
		t.Fatal(err)
	}
	s.voiceBroadcastConfigLoader = func(context.Context) (voiceBroadcastConfig, error) {
		return voiceBroadcastConfig{Enabled: true, Provider: voiceBroadcastProviderBailian, APIKey: "test", Model: voiceBroadcastDefaultModel, Voice: voiceBroadcastDefaultVoice}, nil
	}
	s.voiceBroadcastSynthesizerFactory = func(context.Context) (voiceBroadcastSynthesizer, error) {
		return recordingVoiceBroadcastProvider{}, nil
	}
	w := newAppChatBlockingStreamWriter()
	s.appChatRouter(w, newAppChatStreamRequest(context.Background()))
	body := w.BodyString()
	if !strings.Contains(body, "event: delta\n") || !strings.Contains(body, "event: done\n") {
		t.Fatalf("text events changed or missing: %q", body)
	}
	if !strings.Contains(body, "event: voice\n") || !strings.Contains(body, `"replyId"`) || !strings.Contains(body, `"audioBase64"`) {
		t.Fatalf("voice side channel missing: %q", body)
	}
	if strings.Index(body, "event: voice\n") > strings.Index(body, "event: done\n") {
		t.Fatalf("voice segment arrived after done: %q", body)
	}
}

func TestAppChatStreamKeepsValidSlowVoiceBeforeDone(t *testing.T) {
	store := newFakeAppChatStreamStore()
	s := newAppChatStreamServer(store, successfulAppChatGenerator("这是第一句内容，需要播报。这是第二句内容，也需要播报。"))
	s.voiceBroadcastPreferences = newMemoryVoiceBroadcastPreferenceStore()
	if err := s.voiceBroadcastPreferences.Set(context.Background(), 7, true); err != nil {
		t.Fatal(err)
	}
	s.voiceBroadcastConfigLoader = func(context.Context) (voiceBroadcastConfig, error) {
		return voiceBroadcastConfig{Enabled: true, Provider: voiceBroadcastProviderBailian, APIKey: "test", Model: voiceBroadcastDefaultModel, Voice: voiceBroadcastDefaultVoice}, nil
	}
	s.voiceBroadcastSynthesizerFactory = func(context.Context) (voiceBroadcastSynthesizer, error) {
		return delayedVoiceBroadcastProvider{delay: 450 * time.Millisecond}, nil
	}

	w := newAppChatBlockingStreamWriter()
	s.appChatRouter(w, newAppChatStreamRequest(context.Background()))
	body := w.BodyString()
	voiceIndex := strings.Index(body, "event: voice\n")
	lastVoiceIndex := strings.LastIndex(body, "event: voice\n")
	doneIndex := strings.Index(body, "event: done\n")
	if voiceIndex < 0 || lastVoiceIndex == voiceIndex {
		t.Fatalf("two slow valid voice segments missing: %q", body)
	}
	if doneIndex < 0 || lastVoiceIndex > doneIndex {
		t.Fatalf("slow valid voice segments must arrive before done: %q", body)
	}
}

func TestAppChatVoiceOutlivesGenerationDeadlineAfterPersistence(t *testing.T) {
	store := newFakeAppChatStreamStore()
	s := newAppChatStreamServer(store, successfulAppChatGenerator("生成已经结束，语音仍需完成。"))
	s.chatTimeout = 100 * time.Millisecond
	s.voiceBroadcastPreferences = newMemoryVoiceBroadcastPreferenceStore()
	if err := s.voiceBroadcastPreferences.Set(context.Background(), 7, true); err != nil {
		t.Fatal(err)
	}
	s.voiceBroadcastConfigLoader = func(context.Context) (voiceBroadcastConfig, error) {
		return voiceBroadcastConfig{Enabled: true, Provider: voiceBroadcastProviderBailian, APIKey: "test", Model: voiceBroadcastDefaultModel, Voice: voiceBroadcastDefaultVoice}, nil
	}
	s.voiceBroadcastSynthesizerFactory = func(context.Context) (voiceBroadcastSynthesizer, error) {
		return delayedVoiceBroadcastProvider{delay: 450 * time.Millisecond}, nil
	}

	w := newAppChatBlockingStreamWriter()
	s.appChatRouter(w, newAppChatStreamRequest(context.Background()))
	body := w.BodyString()
	voiceIndex := strings.Index(body, "event: voice\n")
	doneIndex := strings.Index(body, "event: done\n")
	if voiceIndex < 0 {
		t.Fatalf("voice was canceled by the generation deadline: %q", body)
	}
	if doneIndex < 0 || voiceIndex > doneIndex {
		t.Fatalf("voice must arrive before done after persistence: %q", body)
	}
}

func TestAppChatReportsProviderDeadlineAsVoiceTimeout(t *testing.T) {
	store := newFakeAppChatStreamStore()
	s := newAppChatStreamServer(store, successfulAppChatGenerator("语音服务内部超时。"))
	s.voiceBroadcastPreferences = newMemoryVoiceBroadcastPreferenceStore()
	if err := s.voiceBroadcastPreferences.Set(context.Background(), 7, true); err != nil {
		t.Fatal(err)
	}
	s.voiceBroadcastConfigLoader = func(context.Context) (voiceBroadcastConfig, error) {
		return voiceBroadcastConfig{Enabled: true, Provider: voiceBroadcastProviderBailian, APIKey: "test", Model: voiceBroadcastDefaultModel, Voice: voiceBroadcastDefaultVoice}, nil
	}
	s.voiceBroadcastSynthesizerFactory = func(context.Context) (voiceBroadcastSynthesizer, error) {
		return failingVoiceBroadcastProvider{err: context.DeadlineExceeded}, nil
	}

	w := newAppChatBlockingStreamWriter()
	s.appChatRouter(w, newAppChatStreamRequest(context.Background()))
	body := w.BodyString()
	if !strings.Contains(body, "event: voice_error\n") || !strings.Contains(body, `"code":"synthesis_timeout"`) {
		t.Fatalf("provider deadline must emit voice timeout: %q", body)
	}
}

func TestAppChatStreamBoundsStalledVoiceClose(t *testing.T) {
	store := newFakeAppChatStreamStore()
	s := newAppChatStreamServer(store, successfulAppChatGenerator("这是第一句内容，需要播报。"))
	s.voiceBroadcastPreferences = newMemoryVoiceBroadcastPreferenceStore()
	if err := s.voiceBroadcastPreferences.Set(context.Background(), 7, true); err != nil {
		t.Fatal(err)
	}
	provider := &blockingVoiceBroadcastProvider{started: make(chan struct{}), release: make(chan struct{})}
	s.voiceBroadcastConfigLoader = func(context.Context) (voiceBroadcastConfig, error) {
		return voiceBroadcastConfig{Enabled: true, Provider: voiceBroadcastProviderBailian, APIKey: "test", Model: voiceBroadcastDefaultModel, Voice: voiceBroadcastDefaultVoice}, nil
	}
	s.voiceBroadcastSynthesizerFactory = func(context.Context) (voiceBroadcastSynthesizer, error) {
		return provider, nil
	}
	s.voiceBroadcastDrainTimeout = 50 * time.Millisecond
	w := newAppChatBlockingStreamWriter()
	done := make(chan struct{})
	go func() {
		s.appChatRouter(w, newAppChatStreamRequest(context.Background()))
		close(done)
	}()
	select {
	case <-provider.started:
	case <-time.After(time.Second):
		t.Fatal("voice provider did not start")
	}
	started := time.Now()
	select {
	case <-done:
	case <-time.After(time.Second):
		close(provider.release)
		t.Fatal("text stream remained blocked by slow voice close")
	}
	if elapsed := time.Since(started); elapsed >= time.Second {
		t.Fatalf("text terminal took too long after voice provider stalled: %s", elapsed)
	}
	close(provider.release)
	body := w.BodyString()
	if !strings.Contains(body, "event: done\n") || store.saveCallCount() != 1 {
		t.Fatalf("text terminal/persistence missing after slow voice close: body=%q saves=%d", body, store.saveCallCount())
	}
	if !strings.Contains(body, "event: voice_error\n") || !strings.Contains(body, `"code":"synthesis_timeout"`) {
		t.Fatalf("stalled voice close must emit a timeout error: %q", body)
	}
}

func TestVoiceBroadcastCloseHandleBoundsSlowProvider(t *testing.T) {
	provider := &blockingVoiceBroadcastProvider{started: make(chan struct{}), release: make(chan struct{})}
	stream := newVoiceBroadcastStream(context.Background(), "reply-timeout", provider, nil)
	if err := stream.Push("这是第一句内容，需要播报。第二句内容，需要等待语音合成。"); err != nil {
		t.Fatal(err)
	}
	select {
	case <-provider.started:
	case <-time.After(time.Second):
		t.Fatal("voice provider did not start")
	}
	handle := startVoiceBroadcastClose(stream)
	started := time.Now()
	err, completed := handle.await(20 * time.Millisecond)
	if completed || !errors.Is(err, context.DeadlineExceeded) || time.Since(started) >= time.Second {
		t.Fatalf("close handle result err=%v completed=%t elapsed=%s", err, completed, time.Since(started))
	}
	// abort is part of await's timeout path; releasing the provider also lets
	// the background Close goroutine finish cleanly for the race detector.
	close(provider.release)
}

type memoryVoiceBroadcastPreferenceStore struct {
	mu     sync.Mutex
	values map[int64]bool
}

func newMemoryVoiceBroadcastPreferenceStore() *memoryVoiceBroadcastPreferenceStore {
	return &memoryVoiceBroadcastPreferenceStore{values: make(map[int64]bool)}
}

type memoryVoiceBroadcastConfigStore struct {
	cfg   voicebroadcastconfig.Config
	found bool
}

func (s *memoryVoiceBroadcastConfigStore) Read(context.Context) (voicebroadcastconfig.Config, bool, error) {
	return s.cfg, s.found, nil
}

func (s *memoryVoiceBroadcastConfigStore) Update(context.Context, voicebroadcastconfig.UpdateInput, int64) (voicebroadcastconfig.Config, error) {
	return s.cfg, nil
}

func (s *memoryVoiceBroadcastConfigStore) RecordHealth(context.Context, int64, voicebroadcastconfig.Health) (voicebroadcastconfig.Config, error) {
	return s.cfg, nil
}

func (s *memoryVoiceBroadcastPreferenceStore) Get(_ context.Context, userID int64) (bool, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.values[userID], nil
}

func (s *memoryVoiceBroadcastPreferenceStore) Set(_ context.Context, userID int64, enabled bool) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.values[userID] = enabled
	return nil
}

type recordingVoiceBroadcastProvider struct{}

func (recordingVoiceBroadcastProvider) Synthesize(_ context.Context, text string) ([]byte, string, error) {
	return []byte(text), "audio/mpeg", nil
}

type delayedVoiceBroadcastProvider struct{ delay time.Duration }

func (p delayedVoiceBroadcastProvider) Synthesize(ctx context.Context, text string) ([]byte, string, error) {
	timer := time.NewTimer(p.delay)
	defer timer.Stop()
	select {
	case <-timer.C:
		return []byte(text), "audio/mpeg", nil
	case <-ctx.Done():
		return nil, "", ctx.Err()
	}
}

type failingVoiceBroadcastProvider struct{ err error }

func (p failingVoiceBroadcastProvider) Synthesize(context.Context, string) ([]byte, string, error) {
	return nil, "", p.err
}

type blockingVoiceBroadcastProvider struct {
	started chan struct{}
	release chan struct{}
}

func (p *blockingVoiceBroadcastProvider) Synthesize(ctx context.Context, text string) ([]byte, string, error) {
	select {
	case <-p.started:
	default:
		close(p.started)
	}
	select {
	case <-p.release:
		return []byte(text), "audio/mpeg", nil
	case <-ctx.Done():
		return nil, "", ctx.Err()
	}
}
