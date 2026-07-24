package xinzhili

import (
	"context"
	"encoding/json"
	"errors"
	"strings"
	"sync"
	"testing"
	"time"

	"nine-xing/nx-backend/apps/server/internal/rag"
)

func TestConversationBlankASRDoesNotCallModelOrPersist(t *testing.T) {
	fixture := newSessionFixture(t)
	if err := fixture.session.StartTurn(context.Background(), fixture.input("turn-blank")); err != nil {
		t.Fatal(err)
	}
	fixture.asr.emit(ASREvent{Kind: ASREventFinal, Final: "   ", Stable: true})
	time.Sleep(30 * time.Millisecond)
	if fixture.generator.calls() != 0 || fixture.store.userCount() != 0 {
		t.Fatalf("blank transcript generated=%d users=%d", fixture.generator.calls(), fixture.store.userCount())
	}
}

func TestConversationWithoutChatModelReturnsStableError(t *testing.T) {
	fixture := newSessionFixture(t)
	fixture.session.Close()
	fixture.deps.Generator = nil
	fixture.session = NewSession(fixture.deps)
	if err := fixture.session.StartTurn(context.Background(), fixture.input("turn-model-missing")); err != nil {
		t.Fatal(err)
	}
	fixture.asr.emit(ASREvent{Kind: ASREventFinal, Final: "我有点难受", Stable: true})
	event := fixture.sink.waitControl(t, EventError)
	var payload ErrorPayload
	if err := json.Unmarshal(event.Payload, &payload); err != nil {
		t.Fatal(err)
	}
	if payload.Code != "chat_model_not_configured" {
		t.Fatalf("code=%q", payload.Code)
	}
	if fixture.store.userCount() != 0 {
		t.Fatal("model missing must not persist a user turn")
	}
}

func TestConversationIsolationAndRelevanceUseIndependentRetrievers(t *testing.T) {
	fixture := newSessionFixture(t)
	fixture.knowledge.docs = []rag.Document{{ID: "knowledge:1", Title: "关系知识", Content: "关系冲突"}}
	fixture.theory.docs = []rag.Document{{ID: "theory:1", Title: "理论卡", Content: "非暴力沟通"}}
	if err := fixture.session.StartTurn(context.Background(), fixture.input("turn-retrieval")); err != nil {
		t.Fatal(err)
	}
	fixture.asr.emit(ASREvent{Kind: ASREventFinal, Final: "和伴侣冲突怎么办", Stable: true})
	fixture.sink.waitAudio(t)

	if fixture.knowledge.calls != 1 || fixture.theory.calls != 1 {
		t.Fatalf("knowledge calls=%d theory calls=%d", fixture.knowledge.calls, fixture.theory.calls)
	}
	input := fixture.generator.lastInput()
	if len(input.Sources) != 2 || fixture.store.resolved.Scene != SceneXinzhiliVoice || fixture.store.resolved.CardID != 22 {
		t.Fatalf("input=%+v conversation=%+v", input, fixture.store.resolved)
	}
}

func TestDeliveryCreatesAssistantAfterFirstAudioAndAcknowledgesExactPrefixes(t *testing.T) {
	fixture := newSessionFixture(t)
	fixture.synth.segments = []AudioSegment{
		{Seq: 0, Audio: []byte{1}, MIME: "audio/mpeg", deliveryText: "先呼吸。"},
		{Seq: 1, Audio: []byte{2}, MIME: "audio/mpeg", deliveryText: "再感受脚底。"},
	}
	if err := fixture.session.StartTurn(context.Background(), fixture.input("turn-delivery")); err != nil {
		t.Fatal(err)
	}
	fixture.asr.emit(ASREvent{Kind: ASREventFinal, Final: "我很焦虑", Stable: true})
	first := fixture.sink.waitAudio(t)
	if first.Seq != 0 || fixture.store.assistantCount() != 1 {
		t.Fatalf("first=%+v assistants=%d", first, fixture.store.assistantCount())
	}
	fixture.sink.waitAudio(t)
	if err := fixture.session.HandlePlaybackAck(context.Background(), PlaybackAck{TurnID: "turn-delivery", SegmentSeq: 0}); err != nil {
		t.Fatal(err)
	}
	if err := fixture.session.HandlePlaybackAck(context.Background(), PlaybackAck{TurnID: "turn-delivery", SegmentSeq: 1}); err != nil {
		t.Fatal(err)
	}
	fixture.store.waitDelivered(t, "先呼吸。再感受脚底。")
	fixture.store.waitCompleted(t)
}

func TestDeliveryCancellationBeforeFirstAudioDoesNotCreateAssistant(t *testing.T) {
	fixture := newSessionFixture(t)
	fixture.synth.block = make(chan struct{})
	if err := fixture.session.StartTurn(context.Background(), fixture.input("turn-cancel")); err != nil {
		t.Fatal(err)
	}
	fixture.asr.emit(ASREvent{Kind: ASREventFinal, Final: "先别回答", Stable: true})
	fixture.generator.waitCalled(t)
	if err := fixture.session.Cancel(context.Background(), "turn-cancel"); err != nil {
		t.Fatal(err)
	}
	close(fixture.synth.block)
	time.Sleep(30 * time.Millisecond)
	if fixture.store.assistantCount() != 0 {
		t.Fatal("cancel before first playable audio must not create assistant")
	}
}

func TestDeliveryGenerationFailureKeepsPlayedPrefixAndExcludesDraft(t *testing.T) {
	fixture := newSessionFixture(t)
	fixture.generator.answer = ""
	fixture.generator.deltas = []string{"我陪你呼吸。", "这段没有播放"}
	fixture.generator.pauseAfter = 1
	fixture.generator.paused = make(chan struct{})
	fixture.generator.resume = make(chan struct{})
	fixture.generator.err = errors.New("model stream lost")
	fixture.synth.segments = nil
	if err := fixture.session.StartTurn(context.Background(), fixture.input("turn-partial-fail")); err != nil {
		t.Fatal(err)
	}
	fixture.asr.emit(ASREvent{Kind: ASREventFinal, Final: "我很难受", Stable: true})
	select {
	case <-fixture.generator.paused:
	case <-time.After(time.Second):
		t.Fatal("generator did not pause after first sentence")
	}
	first := fixture.sink.waitAudio(t)
	if first.DeliveryText() != "我陪你呼吸。" {
		t.Fatalf("first delivery text=%q", first.DeliveryText())
	}
	if err := fixture.session.HandlePlaybackAck(context.Background(), PlaybackAck{TurnID: "turn-partial-fail", SegmentSeq: first.Seq}); err != nil {
		t.Fatal(err)
	}
	close(fixture.generator.resume)
	fixture.sink.waitControl(t, EventError)
	fixture.store.waitDelivered(t, "我陪你呼吸。")
	fixture.store.mu.Lock()
	assistantDraft := fixture.store.assistants[0]
	fixture.store.mu.Unlock()
	if assistantDraft != "我陪你呼吸。" {
		t.Fatalf("assistant initial content=%q", assistantDraft)
	}
	time.Sleep(30 * time.Millisecond)
	fixture.store.mu.Lock()
	completedContent := fixture.store.completedContent
	fixture.store.mu.Unlock()
	if completedContent != "" {
		t.Fatalf("failed generation must not complete unplayed draft: %q", completedContent)
	}
}

func TestConversationASRDisconnectReopensOnlyBeforeSpeech(t *testing.T) {
	fixture := newSessionFixture(t)
	if err := fixture.session.StartTurn(context.Background(), fixture.input("turn-reopen")); err != nil {
		t.Fatal(err)
	}
	fixture.asr.fail(ErrASRDisconnected)
	fixture.factory.waitOpens(t, 2)

	second := fixture.factory.latest()
	second.emit(ASREvent{Kind: ASREventSpeechStarted})
	second.fail(ErrASRDisconnected)
	event := fixture.sink.waitControl(t, EventError)
	var payload ErrorPayload
	if err := json.Unmarshal(event.Payload, &payload); err != nil {
		t.Fatal(err)
	}
	if payload.Code != "asr_turn_lost" || fixture.factory.openCount() != 2 {
		t.Fatalf("payload=%+v opens=%d", payload, fixture.factory.openCount())
	}
}

type sessionFixture struct {
	deps      SessionDependencies
	session   TurnSession
	store     *fakeConversationStore
	generator *fakeChatGenerator
	knowledge *fakeRetriever
	theory    *fakeRetriever
	synth     *fakeSynthesizer
	sink      *fakeSessionSink
	factory   *fakeASRFactory
	asr       *fakeASRSession
}

func newSessionFixture(t *testing.T) *sessionFixture {
	t.Helper()
	asr := newFakeASRSession()
	factory := &fakeASRFactory{sessions: []*fakeASRSession{asr}}
	store := &fakeConversationStore{}
	generator := &fakeChatGenerator{answer: "先呼吸。再感受脚底。"}
	knowledge := &fakeRetriever{}
	theory := &fakeRetriever{}
	synth := &fakeSynthesizer{segments: []AudioSegment{{Seq: 0, Audio: []byte{1}, MIME: "audio/mpeg", deliveryText: "先呼吸。再感受脚底。"}}}
	sink := newFakeSessionSink()
	deps := SessionDependencies{
		Cards: &fakeCardProvider{}, Conversations: store, Preferences: fakePreferenceProvider{}, Memories: fakeMemoryProvider{},
		Knowledge: knowledge, Theory: theory, Generator: generator, ASRFactory: factory, Synthesizer: synth,
		EngineFactory: func(mode Mode, timing TimingConfig, clock Clock) StrategyEngine {
			return NewEngine(mode, timing, clock)
		},
		Sink: sink, Clock: fixedSessionClock{now: time.Unix(100, 0)},
	}
	fixture := &sessionFixture{deps: deps, store: store, generator: generator, knowledge: knowledge, theory: theory, synth: synth, sink: sink, factory: factory, asr: asr}
	fixture.session = NewSession(deps)
	t.Cleanup(func() { _ = fixture.session.Close() })
	return fixture
}

func (f *sessionFixture) input(turnID string) StartTurnInput {
	return StartTurnInput{UserID: 11, CardID: 22, ConversationID: 33, TurnID: turnID, Mode: ModeNormal, ASRConfig: RealtimeASRConfig{}, TTSConfig: TTSConfig{}, TopK: 4, MinScore: 0.35}
}

type fakeCardProvider struct{}

func (*fakeCardProvider) OwnedCard(context.Context, int64, int64) (Card, error) {
	return Card{ID: 22, Name: "小林", Relation: "朋友", MainType: 6, WingType: 5}, nil
}

type fakeConversationStore struct {
	mu               sync.Mutex
	resolved         Conversation
	users            []string
	assistants       []string
	delivered        []string
	sources          json.RawMessage
	completedContent string
}

func (s *fakeConversationStore) Resolve(_ context.Context, userID, cardID int64, scene string, conversationID int64) (Conversation, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.resolved = Conversation{ID: conversationID, UserID: userID, CardID: cardID, Scene: scene}
	return s.resolved, nil
}
func (s *fakeConversationStore) History(context.Context, Conversation, int) ([]rag.Message, string, error) {
	return nil, "", nil
}
func (s *fakeConversationStore) SaveUser(_ context.Context, _ Conversation, text string, _ Mode) (int64, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.users = append(s.users, text)
	return int64(len(s.users)), nil
}
func (s *fakeConversationStore) CreateAssistant(_ context.Context, _ Conversation, content string, _ Mode) (int64, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.assistants = append(s.assistants, content)
	return int64(100 + len(s.assistants)), nil
}
func (s *fakeConversationStore) AcknowledgeAssistant(_ context.Context, _ int64, delivered string, _ bool) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.delivered = append(s.delivered, delivered)
	return nil
}
func (s *fakeConversationStore) CompleteAssistant(_ context.Context, _ int64, content string, sources json.RawMessage) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.completedContent = content
	s.sources = append(json.RawMessage(nil), sources...)
	return nil
}
func (s *fakeConversationStore) userCount() int {
	s.mu.Lock()
	defer s.mu.Unlock()
	return len(s.users)
}
func (s *fakeConversationStore) assistantCount() int {
	s.mu.Lock()
	defer s.mu.Unlock()
	return len(s.assistants)
}
func (s *fakeConversationStore) completedSources() json.RawMessage {
	s.mu.Lock()
	defer s.mu.Unlock()
	return append(json.RawMessage(nil), s.sources...)
}
func (s *fakeConversationStore) waitCompleted(t *testing.T) {
	t.Helper()
	deadline := time.After(time.Second)
	for {
		if len(s.completedSources()) > 0 {
			return
		}
		select {
		case <-deadline:
			t.Fatal("generation completion must persist sources")
		default:
			time.Sleep(time.Millisecond)
		}
	}
}
func (s *fakeConversationStore) waitDelivered(t *testing.T, want string) {
	t.Helper()
	deadline := time.After(time.Second)
	for {
		s.mu.Lock()
		got := ""
		if len(s.delivered) > 0 {
			got = s.delivered[len(s.delivered)-1]
		}
		s.mu.Unlock()
		if got == want {
			return
		}
		select {
		case <-deadline:
			t.Fatalf("delivered=%q want=%q", got, want)
		default:
			time.Sleep(time.Millisecond)
		}
	}
}

type fakePreferenceProvider struct{}

func (fakePreferenceProvider) PromptPreferences(context.Context, int64) ([]string, error) {
	return []string{"回答简短"}, nil
}

type fakeMemoryProvider struct{}

func (fakeMemoryProvider) PromptMemories(context.Context, int64, int64) ([]string, error) {
	return []string{"喜欢直接表达"}, nil
}

type fakeRetriever struct {
	calls int
	docs  []rag.Document
}

func (r *fakeRetriever) Search(context.Context, string, int, float64) ([]rag.Document, error) {
	r.calls++
	return append([]rag.Document(nil), r.docs...), nil
}

type fakeChatGenerator struct {
	mu         sync.Mutex
	count      int
	answer     string
	input      rag.GenerateInput
	called     chan struct{}
	deltas     []string
	pauseAfter int
	paused     chan struct{}
	resume     chan struct{}
	err        error
}

func (g *fakeChatGenerator) GenerateStream(_ context.Context, input rag.GenerateInput, emit rag.StreamEmitter) (string, error) {
	g.mu.Lock()
	g.count++
	g.input = input
	if g.called != nil {
		close(g.called)
		g.called = nil
	}
	answer := g.answer
	deltas := append([]string(nil), g.deltas...)
	pauseAfter, paused, resume, generationErr := g.pauseAfter, g.paused, g.resume, g.err
	g.mu.Unlock()
	if len(deltas) == 0 {
		deltas = []string{answer}
	}
	for index, delta := range deltas {
		if emit != nil {
			if err := emit(delta); err != nil {
				return "", err
			}
		}
		if pauseAfter > 0 && index+1 == pauseAfter {
			close(paused)
			<-resume
		}
	}
	if answer == "" {
		answer = strings.Join(deltas, "")
	}
	return answer, generationErr
}
func (g *fakeChatGenerator) calls() int { g.mu.Lock(); defer g.mu.Unlock(); return g.count }
func (g *fakeChatGenerator) lastInput() rag.GenerateInput {
	g.mu.Lock()
	defer g.mu.Unlock()
	return g.input
}
func (g *fakeChatGenerator) waitCalled(t *testing.T) {
	t.Helper()
	g.mu.Lock()
	if g.count > 0 {
		g.mu.Unlock()
		return
	}
	if g.called == nil {
		g.called = make(chan struct{})
	}
	ch := g.called
	g.mu.Unlock()
	select {
	case <-ch:
	case <-time.After(time.Second):
		t.Fatal("generator not called")
	}
}

type fakeSynthesizer struct {
	mu       sync.Mutex
	calls    int
	segments []AudioSegment
	block    chan struct{}
}

func (s *fakeSynthesizer) Synthesize(ctx context.Context, _ TTSConfig, text string, emit func(AudioSegment) error) error {
	if s.block != nil {
		select {
		case <-s.block:
		case <-ctx.Done():
			return ctx.Err()
		}
	}
	s.mu.Lock()
	index := s.calls
	s.calls++
	segments := append([]AudioSegment(nil), s.segments...)
	s.mu.Unlock()
	segment := AudioSegment{Audio: []byte{1}, MIME: "audio/mpeg", deliveryText: text}
	if len(segments) > 0 {
		if index >= len(segments) {
			index = len(segments) - 1
		}
		segment = segments[index]
	}
	if err := emit(segment); err != nil {
		return err
	}
	return nil
}

type fakeSessionSink struct {
	controls chan Envelope
	audio    chan AudioSegment
}

func newFakeSessionSink() *fakeSessionSink {
	return &fakeSessionSink{controls: make(chan Envelope, 16), audio: make(chan AudioSegment, 16)}
}
func (s *fakeSessionSink) SendControl(_ context.Context, event Envelope) error {
	s.controls <- event
	return nil
}
func (s *fakeSessionSink) SendAudio(_ context.Context, segment AudioSegment) error {
	s.audio <- segment
	return nil
}
func (s *fakeSessionSink) waitControl(t *testing.T, kind EventType) Envelope {
	t.Helper()
	deadline := time.After(time.Second)
	for {
		select {
		case event := <-s.controls:
			if event.Type == kind {
				return event
			}
		case <-deadline:
			t.Fatalf("control %s not received", kind)
		}
	}
}
func (s *fakeSessionSink) waitAudio(t *testing.T) AudioSegment {
	t.Helper()
	select {
	case segment := <-s.audio:
		return segment
	case <-time.After(time.Second):
		t.Fatal("audio not received")
		return AudioSegment{}
	}
}

type fakeASRFactory struct {
	mu       sync.Mutex
	sessions []*fakeASRSession
	opens    int
}

func (f *fakeASRFactory) Open(context.Context, RealtimeASRConfig) (ASRSession, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.opens++
	if f.opens > len(f.sessions) {
		f.sessions = append(f.sessions, newFakeASRSession())
	}
	return f.sessions[f.opens-1], nil
}
func (f *fakeASRFactory) latest() *fakeASRSession {
	f.mu.Lock()
	defer f.mu.Unlock()
	return f.sessions[f.opens-1]
}
func (f *fakeASRFactory) openCount() int { f.mu.Lock(); defer f.mu.Unlock(); return f.opens }
func (f *fakeASRFactory) waitOpens(t *testing.T, want int) {
	t.Helper()
	deadline := time.After(time.Second)
	for {
		if f.openCount() >= want {
			return
		}
		select {
		case <-deadline:
			t.Fatalf("opens=%d want=%d", f.openCount(), want)
		default:
			time.Sleep(time.Millisecond)
		}
	}
}

type fakeASRSession struct {
	events chan ASREvent
	done   chan struct{}
	mu     sync.Mutex
	err    error
	once   sync.Once
}

func newFakeASRSession() *fakeASRSession {
	return &fakeASRSession{events: make(chan ASREvent, 16), done: make(chan struct{})}
}
func (s *fakeASRSession) WritePCM(context.Context, []byte) error { return nil }
func (s *fakeASRSession) FinishInput(context.Context) error      { return nil }
func (s *fakeASRSession) Events() <-chan ASREvent                { return s.events }
func (s *fakeASRSession) Err() error                             { s.mu.Lock(); defer s.mu.Unlock(); return s.err }
func (s *fakeASRSession) Close() error                           { s.once.Do(func() { close(s.done) }); return nil }
func (s *fakeASRSession) emit(event ASREvent)                    { s.events <- event }
func (s *fakeASRSession) fail(err error)                         { s.mu.Lock(); s.err = err; s.mu.Unlock(); close(s.events) }

type fixedSessionClock struct{ now time.Time }

func (c fixedSessionClock) Now() time.Time { return c.now }
