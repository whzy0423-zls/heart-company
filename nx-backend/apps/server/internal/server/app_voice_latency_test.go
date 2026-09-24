package server

import (
	"bytes"
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"nine-xing/nx-backend/apps/server/internal/rag"
	"nine-xing/nx-backend/apps/server/internal/skillchat"
)

func TestVoiceBroadcastStartsShortCompleteSentenceBeforeClose(t *testing.T) {
	emitted := make(chan voiceBroadcastSegment, 2)
	stream := newVoiceBroadcastStream(context.Background(), "short-reply", recordingVoiceBroadcastProvider{}, func(segment voiceBroadcastSegment) error {
		emitted <- segment
		return nil
	})
	defer func() { stream.Cancel(); _ = stream.Wait() }()

	if err := stream.Push("好的，我们慢慢说。"); err != nil {
		t.Fatal(err)
	}
	select {
	case segment := <-emitted:
		if string(segment.Audio) != "好的，我们慢慢说。" || segment.Final {
			t.Fatalf("first segment = %+v", segment)
		}
	case <-time.After(time.Second):
		t.Fatal("complete short sentence waited for text persistence and stream close")
	}
}

func TestVoiceBroadcastStartsLongPhraseBeforeTextSentenceCompletes(t *testing.T) {
	emitted := make(chan voiceBroadcastSegment, 2)
	stream := newVoiceBroadcastStream(context.Background(), "long-reply", recordingVoiceBroadcastProvider{}, func(segment voiceBroadcastSegment) error {
		emitted <- segment
		return nil
	})
	defer func() { stream.Cancel(); _ = stream.Wait() }()

	if err := stream.Push(strings.Repeat("说", 42)); err != nil {
		t.Fatal(err)
	}
	select {
	case segment := <-emitted:
		if string(segment.Audio) != strings.Repeat("说", 18) || segment.Final {
			t.Fatalf("first segment = %+v", segment)
		}
	case <-time.After(time.Second):
		t.Fatal("long phrase waited for the 96-rune limit")
	}
}

func TestAppChatStartsVoiceBeforeTextSentenceCompletes(t *testing.T) {
	providerStarted := make(chan struct{})
	var providerOnce sync.Once
	store := newFakeAppChatStreamStore()
	generator := &controlledAppChatStreamingGenerator{
		generateStream: func(ctx context.Context, _ rag.GenerateInput, emit rag.StreamEmitter) (string, error) {
			for _, delta := range []string{"第一段内容正在", "继续输出更多内容", "再说吧"} {
				if err := emit(delta); err != nil {
					return "", err
				}
			}
			select {
			case <-ctx.Done():
				return "", ctx.Err()
			case <-time.After(2 * time.Second):
				return "第一段内容正在继续输出更多内容再说吧。", nil
			}
		},
	}
	preferences := newMemoryVoiceBroadcastPreferenceStore()
	if err := preferences.Set(context.Background(), 7, true); err != nil {
		t.Fatal(err)
	}
	s := newAppChatStreamServer(store, generator)
	s.voiceBroadcastPreferences = preferences
	s.voiceBroadcastConfigLoader = enabledVoiceBroadcastConfig
	s.voiceBroadcastSynthesizerFactory = func(context.Context) (voiceBroadcastSynthesizer, error) {
		return voiceBroadcastProviderFunc(func(context.Context, string) ([]byte, string, error) {
			providerOnce.Do(func() { close(providerStarted) })
			return []byte("audio"), "audio/mpeg", nil
		}), nil
	}
	writer := newAppChatBlockingStreamWriter()
	request := newAppChatStreamRequest(context.Background())
	done := make(chan struct{})
	go func() {
		s.appChatRouter(writer, request)
		close(done)
	}()
	select {
	case <-providerStarted:
	case <-time.After(500 * time.Millisecond):
		t.Fatal("voice TTS did not start while text sentence was still incomplete")
	}
	select {
	case <-done:
	case <-time.After(3 * time.Second):
		t.Fatal("chat stream did not finish")
	}
}

type voiceBroadcastProviderFunc func(context.Context, string) ([]byte, string, error)

func (f voiceBroadcastProviderFunc) Synthesize(ctx context.Context, text string) ([]byte, string, error) {
	return f(ctx, text)
}

func TestVoiceBroadcastKeepsTinyAcknowledgmentWithNextSentence(t *testing.T) {
	emitted := make(chan voiceBroadcastSegment, 2)
	stream := newVoiceBroadcastStream(context.Background(), "tiny-reply", recordingVoiceBroadcastProvider{}, func(segment voiceBroadcastSegment) error {
		emitted <- segment
		return nil
	})
	defer func() { stream.Cancel(); _ = stream.Wait() }()

	if err := stream.Push("嗯。"); err != nil {
		t.Fatal(err)
	}
	select {
	case segment := <-emitted:
		t.Fatalf("tiny acknowledgment was split into its own TTS request: %+v", segment)
	default:
	}
	if err := stream.Push("我们可以先慢慢说。"); err != nil {
		t.Fatal(err)
	}
	select {
	case segment := <-emitted:
		if string(segment.Audio) != "嗯。我们可以先慢慢说。" {
			t.Fatalf("combined segment = %+v", segment)
		}
	case <-time.After(time.Second):
		t.Fatal("combined sentence did not start TTS")
	}
}

func TestSkillChatDeliversFirstVoiceWhileLaterSegmentIsSynthesizing(t *testing.T) {
	provider := &twoStageVoiceProvider{
		firstStarted: make(chan struct{}), firstRelease: make(chan struct{}),
		secondStarted: make(chan struct{}), secondRelease: make(chan struct{}),
	}
	store := &slowVoiceSkillRuntimeStore{}
	preferences := newMemoryVoiceBroadcastPreferenceStore()
	if err := preferences.Set(context.Background(), 7, true); err != nil {
		t.Fatal(err)
	}
	s := &Server{
		skillChatRuntime: skillchat.NewRuntime(store, slowVoiceSkillSearcher{}, twoSentenceSkillGenerator{}),
		chatTimeout:      2 * time.Second, voiceBroadcastPreferences: preferences,
		voiceBroadcastConfigLoader: enabledVoiceBroadcastConfig,
		voiceBroadcastSynthesizerFactory: func(context.Context) (voiceBroadcastSynthesizer, error) {
			return provider, nil
		},
		voiceBroadcastDrainTimeout: time.Second,
	}
	requestContext, cancel := context.WithCancel(context.Background())
	defer cancel()
	request := httptest.NewRequest(http.MethodPost, "/api/app/skill-sessions/42/ask/stream", strings.NewReader(`{"question":"请回答"}`)).WithContext(requestContext)
	writer := newVoiceNotifyingWriter()
	finished := make(chan struct{})
	go func() {
		s.appSkillSessionAskStream(writer, request, 7)
		close(finished)
	}()
	defer func() {
		provider.releaseAll()
		select {
		case <-finished:
		case <-time.After(time.Second):
		}
	}()

	select {
	case <-provider.firstStarted:
	case <-time.After(time.Second):
		t.Fatal("first TTS job never started")
	}
	deadline := time.Now().Add(time.Second)
	for store.saves.Load() == 0 && time.Now().Before(deadline) {
		time.Sleep(time.Millisecond)
	}
	if store.saves.Load() == 0 {
		t.Fatal("skill text answer was not saved")
	}
	// The answer is complete while the first TTS job is still held. The next
	// segment must not delay delivery of the first synthesized segment.
	time.Sleep(20 * time.Millisecond)
	provider.releaseFirst()
	select {
	case <-provider.secondStarted:
	case <-time.After(time.Second):
		t.Fatal("second TTS job never started")
	}
	select {
	case <-writer.voiceWritten:
	case <-time.After(time.Second):
		t.Fatal("first voice segment was held behind later TTS work")
	}
	provider.releaseSecond()
	select {
	case <-finished:
	case <-time.After(time.Second):
		t.Fatal("skill stream did not finish")
	}
	if body := writer.BodyString(); strings.Index(body, "event: voice\n") > strings.Index(body, "event: done\n") {
		t.Fatalf("voice was delivered after done: %q", body)
	}
}

func TestSkillChatPartialVoiceFailureFinishesAudioInsteadOfStoppingPlayback(t *testing.T) {
	provider := &partialVoiceProvider{secondStarted: make(chan struct{})}
	s := newPartialVoiceSkillServer(t, provider, time.Second)
	request := httptest.NewRequest(http.MethodPost, "/api/app/skill-sessions/42/ask/stream", strings.NewReader(`{"question":"请回答"}`))
	writer := newVoiceNotifyingWriter()
	s.appSkillSessionAskStream(writer, request, 7)
	assertPartialVoiceTerminatesNormally(t, writer.BodyString())
}

func TestSkillChatPartialVoiceTimeoutFinishesAudioInsteadOfStoppingPlayback(t *testing.T) {
	provider := &partialVoiceProvider{secondStarted: make(chan struct{}), waitForCancel: true}
	s := newPartialVoiceSkillServer(t, provider, 200*time.Millisecond)
	request := httptest.NewRequest(http.MethodPost, "/api/app/skill-sessions/42/ask/stream", strings.NewReader(`{"question":"请回答"}`))
	writer := newVoiceNotifyingWriter()
	finished := make(chan struct{})
	go func() {
		s.appSkillSessionAskStream(writer, request, 7)
		close(finished)
	}()
	select {
	case <-writer.voiceWritten:
	case <-time.After(time.Second):
		t.Fatal("first synthesized voice was not delivered")
	}
	select {
	case <-provider.secondStarted:
	case <-time.After(time.Second):
		t.Fatal("later TTS job did not start")
	}
	select {
	case <-finished:
	case <-time.After(time.Second):
		t.Fatal("skill stream did not finish after voice timeout")
	}
	assertPartialVoiceTerminatesNormally(t, writer.BodyString())
}

func TestSkillChatCanceledVoiceDoesNotSendTerminalEvents(t *testing.T) {
	provider := &partialVoiceProvider{secondStarted: make(chan struct{}), waitForCancel: true}
	s := newPartialVoiceSkillServer(t, provider, time.Second)
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	request := httptest.NewRequest(http.MethodPost, "/api/app/skill-sessions/42/ask/stream", strings.NewReader(`{"question":"请回答"}`)).WithContext(ctx)
	writer := newVoiceNotifyingWriter()
	finished := make(chan struct{})
	go func() {
		s.appSkillSessionAskStream(writer, request, 7)
		close(finished)
	}()
	select {
	case <-writer.voiceWritten:
	case <-time.After(time.Second):
		t.Fatal("first synthesized voice was not delivered")
	}
	select {
	case <-provider.secondStarted:
	case <-time.After(time.Second):
		t.Fatal("later TTS job did not start")
	}
	cancel()
	select {
	case <-finished:
	case <-time.After(time.Second):
		t.Fatal("skill stream did not stop after request cancellation")
	}
	body := writer.BodyString()
	if strings.Contains(body, "event: voice_error\n") || strings.Contains(body, `"final":true`) || strings.Contains(body, "event: done\n") {
		t.Fatalf("canceled request emitted terminal events: %q", body)
	}
}

func TestSkillChatVoiceFailureBeforeAudioKeepsError(t *testing.T) {
	s := newPartialVoiceSkillServer(t, failingVoiceBroadcastProvider{err: errors.New("first TTS failed")}, time.Second)
	request := httptest.NewRequest(http.MethodPost, "/api/app/skill-sessions/42/ask/stream", strings.NewReader(`{"question":"请回答"}`))
	writer := newVoiceNotifyingWriter()
	s.appSkillSessionAskStream(writer, request, 7)
	body := writer.BodyString()
	if !strings.Contains(body, `"code":"synthesis_failed"`) || strings.Contains(body, "event: voice\n") || !strings.Contains(body, "event: done\n") {
		t.Fatalf("failed first TTS did not preserve the error event: %q", body)
	}
}

func TestVoiceBroadcastQueueKeepsEarliestChunkWhenFull(t *testing.T) {
	stream := &voiceBroadcastStream{runCtx: context.Background(), jobs: make(chan string, voiceBroadcastQueueSize)}
	chunks := make([]string, voiceBroadcastQueueSize+1)
	for i := range chunks {
		chunks[i] = strings.Repeat("句", i+1)
	}
	if err := stream.enqueue(chunks); err != nil {
		t.Fatal(err)
	}
	for i := 0; i < voiceBroadcastQueueSize; i++ {
		if got := <-stream.jobs; got != chunks[i] {
			t.Fatalf("queued chunk %d = %q, want earliest %q", i, got, chunks[i])
		}
	}
}

func newPartialVoiceSkillServer(t *testing.T, provider voiceBroadcastSynthesizer, drainTimeout time.Duration) *Server {
	t.Helper()
	preferences := newMemoryVoiceBroadcastPreferenceStore()
	if err := preferences.Set(context.Background(), 7, true); err != nil {
		t.Fatal(err)
	}
	return &Server{
		skillChatRuntime: skillchat.NewRuntime(&slowVoiceSkillRuntimeStore{}, slowVoiceSkillSearcher{}, twoSentenceSkillGenerator{}),
		chatTimeout:      2 * time.Second, voiceBroadcastPreferences: preferences,
		voiceBroadcastConfigLoader: enabledVoiceBroadcastConfig,
		voiceBroadcastSynthesizerFactory: func(context.Context) (voiceBroadcastSynthesizer, error) {
			return provider, nil
		},
		voiceBroadcastDrainTimeout: drainTimeout,
	}
}

func assertPartialVoiceTerminatesNormally(t *testing.T, body string) {
	t.Helper()
	if strings.Contains(body, "event: voice_error\n") {
		t.Fatalf("partial audio would be stopped by a voice error: %q", body)
	}
	if got := strings.Count(body, "event: voice\n"); got != 2 {
		t.Fatalf("voice event count = %d, want audio and final marker: %q", got, body)
	}
	first := strings.Index(body, `"segmentSeq":0`)
	final := strings.Index(body, `"segmentSeq":1`)
	done := strings.Index(body, "event: done\n")
	if first < 0 || final < first || done < final || !strings.Contains(body[final:done], `"final":true`) {
		t.Fatalf("partial voice was not terminated before text done: %q", body)
	}
}

type partialVoiceProvider struct {
	calls         atomic.Int32
	secondStarted chan struct{}
	waitForCancel bool
}

func (p *partialVoiceProvider) Synthesize(ctx context.Context, text string) ([]byte, string, error) {
	if p.calls.Add(1) == 1 {
		return []byte(text), "audio/mpeg", nil
	}
	close(p.secondStarted)
	if p.waitForCancel {
		<-ctx.Done()
		return nil, "", ctx.Err()
	}
	return nil, "", errors.New("later TTS segment failed")
}

type twoSentenceSkillGenerator struct{}

func (twoSentenceSkillGenerator) Generate(context.Context, rag.GenerateInput) (string, error) {
	return "第一句内容需要马上播报。第二句内容会稍后完成。", nil
}

func (twoSentenceSkillGenerator) GenerateStream(_ context.Context, _ rag.GenerateInput, emit rag.StreamEmitter) (string, error) {
	const answer = "第一句内容需要马上播报。第二句内容会稍后完成。"
	if err := emit(answer); err != nil {
		return "", err
	}
	return answer, nil
}

type twoStageVoiceProvider struct {
	calls                                                    atomic.Int32
	firstStarted, firstRelease, secondStarted, secondRelease chan struct{}
	firstOnce, secondOnce                                    sync.Once
}

func (p *twoStageVoiceProvider) Synthesize(ctx context.Context, text string) ([]byte, string, error) {
	switch p.calls.Add(1) {
	case 1:
		close(p.firstStarted)
		select {
		case <-p.firstRelease:
		case <-ctx.Done():
			return nil, "", ctx.Err()
		}
	case 2:
		close(p.secondStarted)
		select {
		case <-p.secondRelease:
		case <-ctx.Done():
			return nil, "", ctx.Err()
		}
	}
	return []byte(text), "audio/mpeg", nil
}

func (p *twoStageVoiceProvider) releaseFirst() {
	p.firstOnce.Do(func() { close(p.firstRelease) })
}

func (p *twoStageVoiceProvider) releaseSecond() {
	p.secondOnce.Do(func() { close(p.secondRelease) })
}

func (p *twoStageVoiceProvider) releaseAll() {
	p.releaseFirst()
	p.releaseSecond()
}

type voiceNotifyingWriter struct {
	header       http.Header
	mu           sync.Mutex
	body         bytes.Buffer
	voiceWritten chan struct{}
	voiceOnce    sync.Once
}

func newVoiceNotifyingWriter() *voiceNotifyingWriter {
	return &voiceNotifyingWriter{header: make(http.Header), voiceWritten: make(chan struct{})}
}

func (w *voiceNotifyingWriter) Header() http.Header { return w.header }
func (w *voiceNotifyingWriter) WriteHeader(int)     {}
func (w *voiceNotifyingWriter) Flush()              {}

func (w *voiceNotifyingWriter) Write(p []byte) (int, error) {
	w.mu.Lock()
	defer w.mu.Unlock()
	n, err := w.body.Write(p)
	if strings.Contains(w.body.String(), "event: voice\n") {
		w.voiceOnce.Do(func() { close(w.voiceWritten) })
	}
	return n, err
}

func (w *voiceNotifyingWriter) BodyString() string {
	w.mu.Lock()
	defer w.mu.Unlock()
	return w.body.String()
}
