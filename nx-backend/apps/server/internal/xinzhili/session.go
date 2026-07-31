package xinzhili

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"sync"
	"time"
	"unicode"

	"nine-xing/nx-backend/apps/server/internal/rag"
)

const SceneXinzhiliVoice = "xinzhili_voice"

const strategyTickInterval = 20 * time.Millisecond

var ErrSessionClosed = errors.New("xinzhili: session closed")

type Card struct {
	ID       int64
	Name     string
	Relation string
	MainType int
	WingType int
	Profile  string
}

type Conversation struct {
	ID     int64
	UserID int64
	CardID int64
	Scene  string
}

type CardProvider interface {
	OwnedCard(ctx context.Context, userID, cardID int64) (Card, error)
}

type ConversationStore interface {
	Resolve(ctx context.Context, userID, cardID int64, scene string, conversationID int64) (Conversation, error)
	History(ctx context.Context, conversation Conversation, limit int) ([]rag.Message, string, error)
	SaveUser(ctx context.Context, conversation Conversation, text string, mode Mode) (int64, error)
	CreateAssistant(ctx context.Context, conversation Conversation, content string, mode Mode) (int64, error)
	AcknowledgeAssistant(ctx context.Context, messageID int64, deliveredText string, complete bool) error
	CompleteAssistant(ctx context.Context, messageID int64, content string, sources json.RawMessage) error
}

type PreferenceProvider interface {
	PromptPreferences(ctx context.Context, userID int64) ([]string, error)
}

type MemoryProvider interface {
	PromptMemories(ctx context.Context, userID, cardID int64) ([]string, error)
}

type KnowledgeRetriever interface {
	Search(ctx context.Context, query string, topK int, minScore float64) ([]rag.Document, error)
}

type TheoryRetriever interface {
	Search(ctx context.Context, query string, topK int, minScore float64) ([]rag.Document, error)
}

type ChatGenerator interface {
	GenerateStream(ctx context.Context, input rag.GenerateInput, emit rag.StreamEmitter) (string, error)
}

type SpeechSynthesizer interface {
	Synthesize(ctx context.Context, cfg TTSConfig, text string, emit func(AudioSegment) error) error
}

type SessionSink interface {
	SendControl(ctx context.Context, event Envelope) error
	SendAudio(ctx context.Context, segment AudioSegment) error
}

type StrategyEngine interface {
	Apply(signal Signal) []Action
	Tick() []Action
}

type EngineFactory func(mode Mode, timing TimingConfig, clock Clock) StrategyEngine

type SessionDependencies struct {
	Cards         CardProvider
	Conversations ConversationStore
	Preferences   PreferenceProvider
	Memories      MemoryProvider
	Knowledge     KnowledgeRetriever
	Theory        TheoryRetriever
	Generator     ChatGenerator
	ASRFactory    ASRFactory
	Synthesizer   SpeechSynthesizer
	EngineFactory EngineFactory
	Sink          SessionSink
	Clock         Clock
}

type StartTurnInput struct {
	UserID            int64
	CardID            int64
	ConversationID    int64
	TurnID            string
	TurnKey           uint64
	Mode              Mode
	ASRConfig         RealtimeASRConfig
	TTSConfig         TTSConfig
	Timing            TimingConfig
	CommonPrompt      string
	ModePrompt        string
	KnowledgeTopK     int
	KnowledgeMinScore float64
	TheoryTopK        int
	TheoryMinScore    float64
}

type PCMFrame struct {
	TurnID string
	Data   []byte
}

type PlaybackAck struct {
	TurnID     string
	SegmentSeq uint32
}

type TurnSession interface {
	StartTurn(ctx context.Context, input StartTurnInput) error
	PushPCM(ctx context.Context, frame PCMFrame) error
	HandlePlaybackAck(ctx context.Context, ack PlaybackAck) error
	Interrupt(ctx context.Context, turnID string) error
	Cancel(ctx context.Context, turnID string) error
	Close() error
}

type session struct {
	deps      SessionDependencies
	commands  chan sessionCommand
	events    chan sessionEvent
	done      chan struct{}
	closeOnce sync.Once
}

type sessionCommand struct {
	kind  sessionCommandKind
	ctx   context.Context
	start StartTurnInput
	pcm   PCMFrame
	ack   PlaybackAck
	turn  string
	reply chan error
}

type sessionCommandKind uint8

const (
	commandStart sessionCommandKind = iota
	commandPCM
	commandAck
	commandInterrupt
	commandCancel
	commandClose
)

type sessionEvent struct {
	kind       sessionEventKind
	turnID     string
	asr        ASREvent
	err        error
	answer     string
	sources    []rag.Source
	segment    AudioSegment
	segmentAck chan error
}

type sessionEventKind uint8

const (
	eventASR sessionEventKind = iota
	eventASRClosed
	eventASRInputFinished
	eventGenerationDelta
	eventGenerationDone
	eventTTSSegment
	eventTTSDone
)

type activeTurn struct {
	input                    StartTurnInput
	turnKey                  uint64
	conversation             Conversation
	card                     Card
	asr                      ASRSession
	engine                   StrategyEngine
	ctx                      context.Context
	cancel                   context.CancelFunc
	hasSpeech                bool
	processing               bool
	endpointing              bool
	finishReturned           bool
	asrTaskFinished          bool
	endpointCancelled        bool
	pendingCancellationCount uint64
	transcripts              []string
	latestQualifiedPartial   string
	interruptionPrefix       string
	proactivePrompt          bool
	terminalPrompt           bool
	responseStartSeq         uint32
	draft                    string
	answer                   string
	sources                  []rag.Source
	assistantID              int64
	segments                 map[uint32]string
	lastAck                  int64
	completionDone           bool
	generationErr            error
	chunker                  streamSentenceChunker
	ttsJobs                  chan ttsStreamJob
	nextTTSSeq               uint32
	ttsAudioBytes            int
	nextTurnSeq              uint64
	doneSent                 bool
}

type ttsStreamJob struct {
	seq  uint32
	text string
}

type ttsStreamResult struct {
	job     ttsStreamJob
	segment AudioSegment
	err     error
}

func NewSession(deps SessionDependencies) TurnSession {
	s := &session{deps: deps, commands: make(chan sessionCommand), events: make(chan sessionEvent, 32), done: make(chan struct{})}
	go s.loop()
	return s
}

func (s *session) StartTurn(ctx context.Context, input StartTurnInput) error {
	return s.request(sessionCommand{kind: commandStart, ctx: ctx, start: input})
}

func (s *session) PushPCM(ctx context.Context, frame PCMFrame) error {
	frame.Data = append([]byte(nil), frame.Data...)
	return s.request(sessionCommand{kind: commandPCM, ctx: ctx, pcm: frame})
}

func (s *session) HandlePlaybackAck(ctx context.Context, ack PlaybackAck) error {
	return s.request(sessionCommand{kind: commandAck, ctx: ctx, ack: ack})
}

func (s *session) Interrupt(ctx context.Context, turnID string) error {
	return s.request(sessionCommand{kind: commandInterrupt, ctx: ctx, turn: turnID})
}

func (s *session) Cancel(ctx context.Context, turnID string) error {
	return s.request(sessionCommand{kind: commandCancel, ctx: ctx, turn: turnID})
}

func (s *session) Close() error {
	var result error
	s.closeOnce.Do(func() { result = s.request(sessionCommand{kind: commandClose, ctx: context.Background()}) })
	return result
}

func (s *session) request(command sessionCommand) error {
	if command.ctx == nil {
		command.ctx = context.Background()
	}
	command.reply = make(chan error, 1)
	select {
	case <-s.done:
		return ErrSessionClosed
	case s.commands <- command:
	case <-command.ctx.Done():
		return command.ctx.Err()
	}
	select {
	case err := <-command.reply:
		return err
	case <-s.done:
		if command.kind == commandClose {
			return nil
		}
		return ErrSessionClosed
	case <-command.ctx.Done():
		return command.ctx.Err()
	}
}

func (s *session) loop() {
	var turn *activeTurn
	ticker := time.NewTicker(strategyTickInterval)
	defer ticker.Stop()
	defer close(s.done)
	for {
		select {
		case command := <-s.commands:
			var err error
			switch command.kind {
			case commandStart:
				if turn != nil {
					s.stopTurn(turn)
				}
				turn, err = s.startTurn(command.ctx, command.start)
			case commandPCM:
				err = s.pushPCM(command.ctx, turn, command.pcm)
			case commandAck:
				err = s.playbackAck(command.ctx, turn, command.ack)
			case commandInterrupt, commandCancel:
				err = s.cancelTurn(turn, command.turn)
				if err == nil {
					turn = nil
				}
			case commandClose:
				if turn != nil {
					s.stopTurn(turn)
				}
				command.reply <- nil
				return
			}
			command.reply <- err
		case event := <-s.events:
			if turn == nil || event.turnID != turn.input.TurnID {
				if event.segmentAck != nil {
					event.segmentAck <- context.Canceled
				}
				continue
			}
			s.handleEvent(&turn, event)
		case <-ticker.C:
			if turn != nil && turn.engine != nil && !turn.processing && !turn.endpointing {
				s.executeStrategyActions(turn, turn.engine.Tick())
			}
		}
	}
}

func (s *session) startTurn(ctx context.Context, input StartTurnInput) (*activeTurn, error) {
	input.TurnID = strings.TrimSpace(input.TurnID)
	if input.UserID <= 0 || input.CardID <= 0 || input.ConversationID < 0 || input.TurnID == "" || input.TurnKey == 0 || !knownMode(input.Mode) {
		return nil, errors.New("xinzhili: invalid turn start")
	}
	if s.deps.Cards == nil || s.deps.Conversations == nil || s.deps.ASRFactory == nil || s.deps.Sink == nil {
		return nil, errors.New("xinzhili: session dependencies missing")
	}
	if input.KnowledgeTopK <= 0 {
		input.KnowledgeTopK = 4
	}
	if input.TheoryTopK <= 0 {
		input.TheoryTopK = 4
	}
	card, err := s.deps.Cards.OwnedCard(ctx, input.UserID, input.CardID)
	if err != nil {
		return nil, err
	}
	conversation, err := s.deps.Conversations.Resolve(ctx, input.UserID, input.CardID, SceneXinzhiliVoice, input.ConversationID)
	if err != nil {
		return nil, err
	}
	turnCtx, cancel := context.WithCancel(context.Background())
	asr, err := s.deps.ASRFactory.Open(turnCtx, input.ASRConfig)
	if err != nil {
		cancel()
		return nil, err
	}
	turn := &activeTurn{input: input, turnKey: input.TurnKey, conversation: conversation, card: card, asr: asr, ctx: turnCtx, cancel: cancel, segments: map[uint32]string{}, lastAck: -1}
	if s.deps.EngineFactory != nil && s.deps.Clock != nil {
		turn.engine = s.deps.EngineFactory(input.Mode, input.Timing, s.deps.Clock)
	}
	s.watchASR(turn.input.TurnID, asr)
	return turn, nil
}

func (s *session) pushPCM(ctx context.Context, turn *activeTurn, frame PCMFrame) error {
	if turn == nil || strings.TrimSpace(frame.TurnID) != turn.input.TurnID {
		return errors.New("xinzhili: turn not active")
	}
	if len(frame.Data) == 0 {
		return ErrASREmptyPCM
	}
	return turn.asr.WritePCM(ctx, frame.Data)
}

func (s *session) cancelTurn(turn *activeTurn, turnID string) error {
	if turn == nil || strings.TrimSpace(turnID) != turn.input.TurnID {
		return errors.New("xinzhili: turn not active")
	}
	s.stopTurn(turn)
	return nil
}

func (s *session) stopTurn(turn *activeTurn) {
	turn.cancel()
	if turn.asr != nil {
		_ = turn.asr.Close()
	}
}

func (s *session) playbackAck(ctx context.Context, turn *activeTurn, ack PlaybackAck) error {
	if turn == nil || ack.TurnID != turn.input.TurnID {
		return errors.New("xinzhili: assistant delivery not active")
	}
	if turn.assistantID == 0 {
		if ack.SegmentSeq < turn.responseStartSeq {
			return nil
		}
		return errors.New("xinzhili: assistant delivery not active")
	}
	if int64(ack.SegmentSeq) <= turn.lastAck {
		return nil
	}
	var builder strings.Builder
	for seq := turn.responseStartSeq; seq <= ack.SegmentSeq; seq++ {
		text, ok := turn.segments[seq]
		if !ok {
			return errors.New("xinzhili: playback ack references unsent segment")
		}
		builder.WriteString(text)
	}
	delivered := builder.String()
	complete := turn.completionDone && delivered == turn.answer
	if err := s.deps.Conversations.AcknowledgeAssistant(ctx, turn.assistantID, delivered, complete); err != nil {
		return err
	}
	turn.lastAck = int64(ack.SegmentSeq)
	return nil
}

func (s *session) watchASR(turnID string, asr ASRSession) {
	go func() {
		for event := range asr.Events() {
			if !s.postEvent(sessionEvent{kind: eventASR, turnID: turnID, asr: event}) {
				return
			}
		}
		s.postEvent(sessionEvent{kind: eventASRClosed, turnID: turnID, err: asr.Err()})
	}()
}

func (s *session) postEvent(event sessionEvent) bool {
	select {
	case s.events <- event:
		return true
	case <-s.done:
		return false
	}
}

func (s *session) handleEvent(turn **activeTurn, event sessionEvent) {
	current := *turn
	switch event.kind {
	case eventASR:
		s.handleASREvent(current, event.asr)
	case eventASRInputFinished:
		if !current.endpointing || current.processing {
			return
		}
		if event.err != nil {
			s.sendError(current, "asr_finish_failed", "语音识别结束失败，请重新说一次", true)
			_ = s.sendAssistantDone(current)
			current.processing = true
			current.cancel()
			return
		}
		current.finishReturned = true
		s.completeStrategyEndpoint(current)
	case eventASRClosed:
		if event.err == nil || errors.Is(event.err, context.Canceled) {
			return
		}
		if !current.hasSpeech && !current.processing {
			asr, err := s.deps.ASRFactory.Open(current.ctx, current.input.ASRConfig)
			if err == nil {
				current.asr = asr
				s.watchASR(current.input.TurnID, asr)
				return
			}
		}
		s.sendError(current, "asr_turn_lost", "语音连接中断，请重新说一次", true)
		_ = s.sendAssistantDone(current)
		s.stopTurn(current)
		*turn = nil
	case eventGenerationDone:
		s.handleGenerationDone(current, event)
	case eventGenerationDelta:
		current.draft += event.answer
		for _, chunk := range current.chunker.Push(event.answer) {
			s.queueTTSChunk(current, chunk)
		}
	case eventTTSSegment:
		err := s.acceptAudioSegment(current, event.segment)
		if event.segmentAck != nil {
			event.segmentAck <- err
		}
	case eventTTSDone:
		proactivePrompt := current.proactivePrompt
		terminalPrompt := current.terminalPrompt
		current.completionDone = true
		if current.engine != nil {
			s.executeStrategyActions(current, current.engine.Apply(Signal{Kind: SignalAssistantStopped}))
		}
		if current.assistantID > 0 && current.generationErr == nil && current.answer != "" {
			sources, _ := json.Marshal(current.sources)
			_ = s.deps.Conversations.CompleteAssistant(current.ctx, current.assistantID, current.answer, sources)
			s.confirmCompletedPlayback(current)
		}
		if event.err != nil && !errors.Is(event.err, context.Canceled) {
			s.sendError(current, "tts_failed", "语音回复生成失败，请重试", true)
		}
		if event.err == nil || !errors.Is(event.err, context.Canceled) {
			if err := s.sendAssistantDone(current); err != nil {
				current.cancel()
			}
		}
		if proactivePrompt {
			s.resetProactivePromptDelivery(current)
		}
		if terminalPrompt {
			current.cancel()
		}
	}
}

func (s *session) confirmCompletedPlayback(turn *activeTurn) {
	if turn.lastAck < 0 {
		return
	}
	var builder strings.Builder
	for seq := turn.responseStartSeq; seq <= uint32(turn.lastAck); seq++ {
		text, ok := turn.segments[seq]
		if !ok {
			return
		}
		builder.WriteString(text)
	}
	if delivered := builder.String(); delivered == turn.answer {
		_ = s.deps.Conversations.AcknowledgeAssistant(turn.ctx, turn.assistantID, delivered, true)
	}
}

func (s *session) handleASREvent(turn *activeTurn, event ASREvent) {
	if turn.processing && event.Kind != ASREventSpeechStarted {
		return
	}
	switch event.Kind {
	case ASREventSpeechStarted:
		firstSpeech := !turn.hasSpeech
		turn.hasSpeech = true
		if turn.engine != nil {
			s.executeStrategyActions(turn, turn.engine.Apply(Signal{Kind: SignalSpeechStarted}))
		}
		if firstSpeech && !turn.processing {
			if err := s.sendTurnControl(turn, EventASRActivity, map[string]any{"state": "speech_started"}); err != nil {
				turn.cancel()
			}
		}
	case ASREventPartial:
		text := strings.TrimSpace(event.Partial)
		if !hasMeaningfulTranscript(text) {
			return
		}
		turn.hasSpeech = true
		if qualifiedPartialFallback(text) {
			turn.latestQualifiedPartial = text
		}
		if turn.engine != nil {
			s.executeStrategyActions(turn, turn.engine.Apply(Signal{Kind: SignalPartial, Transcript: text, Stable: event.Stable}))
		}
	case ASREventFinal:
		if !event.Stable {
			return
		}
		text := strings.TrimSpace(event.Final)
		if !hasMeaningfulTranscript(text) {
			return
		}
		turn.hasSpeech = true
		s.appendStableTranscript(turn, text)
		if turn.engine == nil {
			s.beginProcessing(turn, turnTranscript(turn))
			return
		}
		s.executeStrategyActions(turn, turn.engine.Apply(Signal{Kind: SignalStableText, Transcript: text, Stable: true}))
		s.executeStrategyActions(turn, turn.engine.Apply(Signal{Kind: SignalSilence}))
	case ASREventTaskFinished:
		turn.asrTaskFinished = true
		s.completeStrategyEndpoint(turn)
	}
}

func (s *session) executeStrategyActions(turn *activeTurn, actions []Action) {
	for _, action := range actions {
		var err error
		switch action.Kind {
		case ActionEndpoint:
			s.beginStrategyEndpoint(turn)
		case ActionCancelPending:
			err = s.cancelPendingStrategyWork(turn)
		case ActionStopAssistant:
			err = s.sendTurnControl(turn, EventPlaybackInterrupt, map[string]any{"reason": "speech_started"})
		case ActionComfortPrompt:
			err = s.startStrategyPrompt(turn, action.TextKey)
		case ActionQueueInterruptionPrefix:
			turn.interruptionPrefix, err = strategyActionText(action.TextKey)
		default:
			err = fmt.Errorf("xinzhili: unknown strategy action %d", action.Kind)
		}
		if err != nil {
			s.failStrategyAction(turn)
			return
		}
	}
}

func (s *session) failStrategyAction(turn *activeTurn) {
	if turn == nil {
		return
	}
	s.sendError(turn, "strategy_action_failed", "语音交互动作执行失败，请重试", true)
	_ = s.sendAssistantDone(turn)
	turn.processing = true
	turn.cancel()
}

func (s *session) cancelPendingStrategyWork(turn *activeTurn) error {
	if turn == nil {
		return errors.New("xinzhili: strategy turn missing")
	}
	turn.pendingCancellationCount++
	if !turn.endpointing && !turn.processing {
		return nil
	}
	turn.endpointCancelled = true
	if err := s.sendTurnControl(turn, EventTurnCancelled, map[string]any{"reason": "strategy_cancel_pending"}); err != nil {
		return err
	}
	if err := s.sendAssistantDone(turn); err != nil {
		return err
	}
	turn.processing = true
	turn.cancel()
	return nil
}

func (s *session) startStrategyPrompt(turn *activeTurn, textKey string) error {
	if turn == nil || turn.processing || turn.endpointing {
		return errors.New("xinzhili: strategy prompt conflicts with active work")
	}
	if s.deps.Synthesizer == nil {
		return errors.New("xinzhili: strategy prompt synthesizer missing")
	}
	text, err := strategyActionText(textKey)
	if err != nil {
		return err
	}
	if err := s.sendTurnControl(turn, EventTurnProcessing, map[string]any{"proactive": true, "textKey": textKey}); err != nil {
		return err
	}
	turn.processing = true
	turn.proactivePrompt = true
	turn.draft = text
	turn.answer = text
	turn.ttsJobs = make(chan ttsStreamJob, 1)
	s.startSynthesisWorker(turn)
	s.queueTTSChunk(turn, text)
	close(turn.ttsJobs)
	return nil
}

func (s *session) resetProactivePromptDelivery(turn *activeTurn) {
	turn.processing = false
	turn.proactivePrompt = false
	turn.draft = ""
	turn.answer = ""
	turn.sources = nil
	turn.assistantID = 0
	turn.segments = make(map[uint32]string)
	turn.responseStartSeq = turn.nextTTSSeq
	turn.lastAck = int64(turn.responseStartSeq) - 1
	turn.completionDone = false
	turn.generationErr = nil
	turn.chunker = streamSentenceChunker{}
	turn.ttsJobs = nil
	turn.doneSent = false
}

func strategyActionText(textKey string) (string, error) {
	switch strings.TrimSpace(textKey) {
	case "comfort.first_silence":
		return "我在这里，你可以慢慢说。", nil
	case "comfort.second_silence":
		return "不用着急，想到哪里就说到哪里。", nil
	case "comfort.mid_sentence":
		return "没关系，慢慢来，我在听。", nil
	case "deep_listening.silence":
		return "我会安静陪着你，准备好了再继续。", nil
	case "argument.important_interruption":
		return "我听到了，这一点很重要。", nil
	default:
		return "", fmt.Errorf("xinzhili: unknown strategy text key %q", textKey)
	}
}

func (s *session) beginStrategyEndpoint(turn *activeTurn) {
	if turn == nil || turn.engine == nil || turn.processing || turn.endpointing {
		return
	}
	if !hasMeaningfulTranscript(turnTranscript(turn)) && !qualifiedPartialFallback(turn.latestQualifiedPartial) {
		return
	}
	turn.endpointing = true
	go func(turnID string, asr ASRSession, ctx context.Context) {
		err := asr.FinishInput(ctx)
		s.postEvent(sessionEvent{kind: eventASRInputFinished, turnID: turnID, err: err})
	}(turn.input.TurnID, turn.asr, turn.ctx)
}

func (s *session) completeStrategyEndpoint(turn *activeTurn) {
	if turn == nil || !turn.endpointing || turn.processing || turn.endpointCancelled ||
		!turn.finishReturned || !turn.asrTaskFinished {
		return
	}
	text := turnTranscript(turn)
	if !hasMeaningfulTranscript(text) && qualifiedPartialFallback(turn.latestQualifiedPartial) {
		s.appendStableTranscript(turn, turn.latestQualifiedPartial)
		text = turnTranscript(turn)
	}
	if hasMeaningfulTranscript(text) {
		s.beginProcessing(turn, text)
		return
	}
	s.startUnclearEnvironmentPrompt(turn)
}

func (s *session) startUnclearEnvironmentPrompt(turn *activeTurn) {
	if turn == nil || turn.processing {
		return
	}
	const prompt = "环境有些嘈杂，我没听清，请再说一次"
	if s.deps.Synthesizer == nil {
		s.sendError(turn, "tts_not_configured", "请配置好语音模型后再重试", false)
		_ = s.sendAssistantDone(turn)
		turn.processing = true
		turn.cancel()
		return
	}
	if err := s.sendTurnControl(turn, EventTurnProcessing, map[string]any{"recoverable": true}); err != nil {
		turn.processing = true
		turn.cancel()
		return
	}
	turn.processing = true
	turn.terminalPrompt = true
	turn.draft = prompt
	turn.answer = prompt
	turn.ttsJobs = make(chan ttsStreamJob, 1)
	s.startSynthesisWorker(turn)
	s.queueTTSChunk(turn, prompt)
	close(turn.ttsJobs)
}

func (s *session) beginProcessing(turn *activeTurn, text string) {
	if turn == nil || turn.processing {
		return
	}
	text = strings.TrimSpace(text)
	if !hasMeaningfulTranscript(text) {
		return
	}
	if s.deps.Generator == nil && !rag.IsModelIdentityQuestion(text) {
		s.sendError(turn, "chat_model_not_configured", "请配置好会话模型后再重试", false)
		_ = s.sendAssistantDone(turn)
		turn.processing = true
		turn.cancel()
		return
	}
	if _, err := s.deps.Conversations.SaveUser(turn.ctx, turn.conversation, text, turn.input.Mode); err != nil {
		s.sendError(turn, "conversation_save_failed", "会话保存失败，请重试", true)
		_ = s.sendAssistantDone(turn)
		turn.processing = true
		turn.cancel()
		return
	}
	if s.deps.Synthesizer == nil {
		s.sendError(turn, "tts_not_configured", "请配置好语音模型后再重试", false)
		_ = s.sendAssistantDone(turn)
		turn.processing = true
		turn.cancel()
		return
	}
	if err := s.sendTurnControl(turn, EventTurnProcessing, map[string]any{}); err != nil {
		turn.cancel()
		return
	}
	turn.processing = true
	turn.ttsJobs = make(chan ttsStreamJob, 128)
	s.startSynthesisWorker(turn)
	s.startGeneration(turn, text)
}

func (s *session) appendStableTranscript(turn *activeTurn, text string) {
	text = strings.TrimSpace(text)
	if text == "" {
		return
	}
	if count := len(turn.transcripts); count > 0 {
		last := turn.transcripts[count-1]
		switch {
		case text == last, strings.HasPrefix(last, text):
			return
		case strings.HasPrefix(text, last):
			turn.transcripts[count-1] = text
			return
		}
	}
	turn.transcripts = append(turn.transcripts, text)
}

func turnTranscript(turn *activeTurn) string {
	if turn == nil {
		return ""
	}
	return strings.Join(turn.transcripts, "")
}

func (s *session) startGeneration(turn *activeTurn, question string) {
	prefix := turn.interruptionPrefix
	turn.interruptionPrefix = ""
	if rag.IsModelIdentityQuestion(question) {
		go func() {
			if prefix != "" && !s.postEvent(sessionEvent{kind: eventGenerationDelta, turnID: turn.input.TurnID, answer: prefix}) {
				return
			}
			if !s.postEvent(sessionEvent{kind: eventGenerationDelta, turnID: turn.input.TurnID, answer: rag.ModelIdentityReply}) {
				return
			}
			s.postEvent(sessionEvent{kind: eventGenerationDone, turnID: turn.input.TurnID, answer: rag.ModelIdentityReply})
		}()
		return
	}
	go func() {
		if prefix != "" && !s.postEvent(sessionEvent{kind: eventGenerationDelta, turnID: turn.input.TurnID, answer: prefix}) {
			return
		}
		var (
			history       []rag.Message
			summary       string
			preferences   []string
			memories      []string
			knowledgeDocs []rag.Document
			theoryDocs    []rag.Document
			contextLoads  sync.WaitGroup
		)
		contextLoads.Add(5)
		go func() {
			defer contextLoads.Done()
			history, summary, _ = s.deps.Conversations.History(turn.ctx, turn.conversation, 20)
		}()
		go func() {
			defer contextLoads.Done()
			if s.deps.Preferences != nil {
				preferences, _ = s.deps.Preferences.PromptPreferences(turn.ctx, turn.input.UserID)
			}
		}()
		go func() {
			defer contextLoads.Done()
			if s.deps.Memories != nil {
				memories, _ = s.deps.Memories.PromptMemories(turn.ctx, turn.input.UserID, turn.input.CardID)
			}
		}()
		go func() {
			defer contextLoads.Done()
			if s.deps.Knowledge != nil {
				knowledgeDocs, _ = s.deps.Knowledge.Search(turn.ctx, question, turn.input.KnowledgeTopK, turn.input.KnowledgeMinScore)
			}
		}()
		go func() {
			defer contextLoads.Done()
			if s.deps.Theory != nil {
				theoryDocs, _ = s.deps.Theory.Search(turn.ctx, question, turn.input.TheoryTopK, turn.input.TheoryMinScore)
			}
		}()
		contextLoads.Wait()

		documents := make([]rag.Document, 0, turn.input.KnowledgeTopK+turn.input.TheoryTopK)
		documents = appendUniqueDocuments(documents, knowledgeDocs)
		documents = appendUniqueDocuments(documents, theoryDocs)
		sources := documentsToSources(documents)
		directives := make([]string, 0, 2)
		if value := strings.TrimSpace(turn.input.CommonPrompt); value != "" {
			directives = append(directives, value)
		}
		if value := strings.TrimSpace(turn.input.ModePrompt); value != "" {
			directives = append(directives, value)
		}
		input := rag.GenerateInput{
			History: history, ConversationSummary: summary, Question: question, Sources: sources,
			UserProfile:      rag.UserProfile{Memories: memories},
			ConversationCard: rag.ConversationCard{Name: turn.card.Name, Relation: turn.card.Relation, MainType: turn.card.MainType, WingType: turn.card.WingType, Profile: turn.card.Profile},
			UserPreferences:  preferences, CurrentDirectives: directives, Tier: "companion",
		}
		answer, err := s.deps.Generator.GenerateStream(turn.ctx, input, func(delta string) error {
			if delta == "" {
				return nil
			}
			if !s.postEvent(sessionEvent{kind: eventGenerationDelta, turnID: turn.input.TurnID, answer: delta}) {
				return ErrSessionClosed
			}
			return nil
		})
		s.postEvent(sessionEvent{kind: eventGenerationDone, turnID: turn.input.TurnID, answer: answer, sources: sources, err: err})
	}()
}

func (s *session) startSynthesisWorker(turn *activeTurn) {
	const concurrency = 2
	go func() {
		ctx, cancel := context.WithCancel(turn.ctx)
		defer cancel()
		results := make(chan ttsStreamResult, concurrency)
		pendingJobs := make([]ttsStreamJob, 0, concurrency)
		pendingSegments := make(map[uint32]AudioSegment, concurrency)
		jobCancels := make(map[uint32]context.CancelFunc, concurrency)
		jobs := turn.ttsJobs
		nextEmit := turn.responseStartSeq
		inFlight := 0
		var workerErr error
		failed := false
		failedSeq := ^uint32(0)

		startJob := func(job ttsStreamJob) {
			jobCtx, jobCancel := context.WithCancel(ctx)
			jobCancels[job.seq] = jobCancel
			inFlight++
			go func() {
				result := ttsStreamResult{job: job}
				emitted := false
				result.err = s.deps.Synthesizer.Synthesize(jobCtx, turn.input.TTSConfig, job.text, func(segment AudioSegment) error {
					if emitted {
						return errors.New("xinzhili: streamed TTS chunk produced multiple segments")
					}
					emitted = true
					segment.Seq = job.seq
					segment.deliveryText = job.text
					result.segment = segment
					return nil
				})
				if result.err == nil && !emitted {
					result.err = errors.New("xinzhili: streamed TTS chunk produced no segment")
				}
				select {
				case results <- result:
				case <-turn.ctx.Done():
				}
			}()
		}

		fail := func(seq uint32, err error) {
			if failed && seq >= failedSeq {
				return
			}
			failed = true
			failedSeq = seq
			workerErr = err
			pendingJobs = nil
			for pendingSeq := range pendingSegments {
				if pendingSeq >= failedSeq {
					delete(pendingSegments, pendingSeq)
				}
			}
			for runningSeq, stop := range jobCancels {
				if runningSeq > failedSeq {
					stop()
				}
			}
		}

		emitReady := func() {
			for !failed || nextEmit < failedSeq {
				segment, ok := pendingSegments[nextEmit]
				if !ok {
					return
				}
				response := make(chan error, 1)
				if !s.postEvent(sessionEvent{kind: eventTTSSegment, turnID: turn.input.TurnID, segment: segment, segmentAck: response}) {
					fail(nextEmit, ErrSessionClosed)
					return
				}
				select {
				case err := <-response:
					if err != nil {
						fail(nextEmit, err)
						return
					}
				case <-turn.ctx.Done():
					fail(nextEmit, turn.ctx.Err())
					return
				}
				delete(pendingSegments, nextEmit)
				nextEmit++
			}
		}

		for jobs != nil || inFlight > 0 || len(pendingJobs) > 0 {
			for !failed && inFlight < concurrency && len(pendingJobs) > 0 && pendingJobs[0].seq < nextEmit+concurrency {
				job := pendingJobs[0]
				pendingJobs = pendingJobs[1:]
				startJob(job)
			}
			if jobs == nil && inFlight == 0 && len(pendingJobs) > 0 {
				fail(nextEmit, errors.New("xinzhili: TTS chunk sequence gap"))
			}
			select {
			case <-turn.ctx.Done():
				return
			case job, ok := <-jobs:
				if !ok {
					jobs = nil
					continue
				}
				if !failed {
					pendingJobs = append(pendingJobs, job)
				}
			case result := <-results:
				inFlight--
				if stop := jobCancels[result.job.seq]; stop != nil {
					stop()
					delete(jobCancels, result.job.seq)
				}
				if result.err != nil {
					fail(result.job.seq, result.err)
					continue
				}
				if failed && result.job.seq >= failedSeq {
					continue
				}
				turn.ttsAudioBytes += len(result.segment.Audio)
				if turn.ttsAudioBytes > maxTTSTurnBytes {
					fail(result.job.seq, ErrTTSTurnTooLarge)
					continue
				}
				pendingSegments[result.job.seq] = result.segment
				emitReady()
			}
		}
		s.postEvent(sessionEvent{kind: eventTTSDone, turnID: turn.input.TurnID, err: workerErr})
	}()
}

func (s *session) handleGenerationDone(turn *activeTurn, event sessionEvent) {
	rawAnswer := strings.TrimSpace(event.answer)
	if rawAnswer != "" && strings.HasPrefix(rawAnswer, turn.draft) && len(rawAnswer) > len(turn.draft) {
		suffix := rawAnswer[len(turn.draft):]
		turn.draft += suffix
		for _, chunk := range turn.chunker.Push(suffix) {
			s.queueTTSChunk(turn, chunk)
		}
	}
	answer := normalizeGeneratedContent(rawAnswer)
	turn.answer = normalizeGeneratedContent(turn.draft)
	if turn.answer == "" || strings.HasPrefix(answer, turn.answer) {
		turn.answer = answer
	}
	turn.sources = event.sources
	turn.generationErr = event.err
	if event.err == nil {
		for _, chunk := range turn.chunker.Flush() {
			s.queueTTSChunk(turn, chunk)
		}
	}
	close(turn.ttsJobs)
	if event.err != nil || turn.answer == "" {
		s.sendError(turn, "generation_failed", "回答生成失败，请重试", true)
	}
}

func (s *session) queueTTSChunk(turn *activeTurn, chunk string) {
	chunk = strings.TrimSpace(chunk)
	if chunk == "" {
		return
	}
	turn.ttsJobs <- ttsStreamJob{seq: turn.nextTTSSeq, text: chunk}
	turn.nextTTSSeq++
}

func (s *session) acceptAudioSegment(turn *activeTurn, segment AudioSegment) error {
	if turn.turnKey == 0 {
		return errors.New("xinzhili: active turn key missing")
	}
	segment.TurnKey = turn.turnKey
	text := segment.DeliveryText()
	if text == "" {
		return errors.New("xinzhili: TTS segment delivery text missing")
	}
	if _, exists := turn.segments[segment.Seq]; exists {
		return errors.New("xinzhili: duplicate TTS segment")
	}
	digest := sha256.Sum256(segment.Audio)
	if err := s.sendTurnControl(turn, EventAssistantAudioStart, map[string]any{
		"segmentSeq": segment.Seq,
		"mimeType":   "audio/mpeg",
		"byteLength": len(segment.Audio),
		"sha256":     hex.EncodeToString(digest[:]),
	}); err != nil {
		return err
	}
	if err := s.deps.Sink.SendAudio(turn.ctx, segment); err != nil {
		return err
	}
	if err := s.sendTurnControl(turn, EventAssistantAudioEnd, map[string]any{"segmentSeq": segment.Seq}); err != nil {
		return err
	}
	if turn.engine != nil && len(turn.segments) == 0 {
		s.executeStrategyActions(turn, turn.engine.Apply(Signal{Kind: SignalAssistantStarted}))
	}
	turn.segments[segment.Seq] = text
	if turn.assistantID == 0 && !turn.proactivePrompt && !turn.terminalPrompt {
		content := normalizeGeneratedContent(turn.answer)
		if content == "" {
			content = normalizeGeneratedContent(turn.draft)
		}
		messageID, err := s.deps.Conversations.CreateAssistant(turn.ctx, turn.conversation, content, turn.input.Mode)
		if err != nil {
			return err
		}
		turn.assistantID = messageID
	}
	return nil
}

func (s *session) sendTurnControl(turn *activeTurn, kind EventType, payload any) error {
	encoded, err := json.Marshal(payload)
	if err != nil {
		return err
	}
	turnID := turn.input.TurnID
	turnSeq := turn.nextTurnSeq
	turn.nextTurnSeq++
	return s.deps.Sink.SendControl(turn.ctx, Envelope{
		ProtocolVersion: ProtocolVersion,
		Type:            kind,
		TurnID:          &turnID,
		TurnSeq:         &turnSeq,
		TimestampMs:     time.Now().UnixMilli(),
		Payload:         encoded,
	})
}

func (s *session) sendError(turn *activeTurn, code, message string, retryable bool) {
	turnID := turn.input.TurnID
	payload, _ := json.Marshal(ErrorPayload{Code: code, Message: message, Retryable: retryable, Fatal: false, TurnID: &turnID})
	if err := s.deps.Sink.SendControl(turn.ctx, Envelope{
		ProtocolVersion: ProtocolVersion, Type: EventError,
		TimestampMs: time.Now().UnixMilli(), Payload: payload,
	}); err != nil {
		turn.cancel()
	}
}

func (s *session) sendAssistantDone(turn *activeTurn) error {
	if turn.doneSent {
		return nil
	}
	turn.doneSent = true
	return s.sendTurnControl(turn, EventAssistantDone, map[string]any{"segmentCount": len(turn.segments)})
}

func appendUniqueDocuments(existing, incoming []rag.Document) []rag.Document {
	seen := make(map[string]struct{}, len(existing)+len(incoming))
	for _, document := range existing {
		seen[document.ID] = struct{}{}
	}
	for _, document := range incoming {
		key := strings.TrimSpace(document.ID)
		if key == "" {
			key = strings.TrimSpace(document.Title) + "\x00" + strings.TrimSpace(document.Content)
		}
		if _, exists := seen[key]; exists {
			continue
		}
		seen[key] = struct{}{}
		existing = append(existing, document)
	}
	return existing
}

func documentsToSources(documents []rag.Document) []rag.Source {
	sources := make([]rag.Source, 0, len(documents))
	for _, document := range documents {
		content := []rune(strings.TrimSpace(document.Content))
		if len(content) > 160 {
			content = content[:160]
		}
		sources = append(sources, rag.Source{ID: document.ID, Title: document.Title, Snippet: string(content)})
	}
	return sources
}

func hasMeaningfulTranscript(text string) bool {
	for _, r := range strings.TrimSpace(text) {
		if unicode.IsLetter(r) || unicode.IsDigit(r) {
			return true
		}
	}
	return false
}

func normalizeGeneratedContent(text string) string {
	return strings.Join(SplitSentences(text), "")
}

const (
	firstTTSChunkMinRunes = 14
	firstTTSChunkMaxRunes = 28
)

type streamSentenceChunker struct {
	buffer  []rune
	emitted bool
}

func (c *streamSentenceChunker) Push(delta string) []string {
	c.buffer = append(c.buffer, []rune(delta)...)
	return c.take(false)
}

func (c *streamSentenceChunker) Flush() []string { return c.take(true) }

func (c *streamSentenceChunker) take(flush bool) []string {
	var chunks []string
	for len(c.buffer) > 0 {
		cut := 0
		chunkLimit := maxTTSSentenceRunes
		if !c.emitted {
			chunkLimit = firstTTSChunkMaxRunes
		}
		limit := min(len(c.buffer), chunkLimit)
		for index := 0; index < limit; index++ {
			if isStrongSentenceEndAt(c.buffer, index) {
				cut = index + 1
				for cut < limit && isClosingQuote(c.buffer[cut]) {
					cut++
				}
				break
			}
			if !c.emitted && index+1 >= firstTTSChunkMinRunes && isSoftSentencePause(c.buffer[index]) {
				cut = index + 1
				break
			}
		}
		if cut == 0 && len(c.buffer) >= chunkLimit {
			cut = limit
		}
		if cut == 0 && flush {
			cut = len(c.buffer)
		}
		if cut == 0 {
			break
		}
		if chunk := strings.TrimSpace(string(c.buffer[:cut])); chunk != "" {
			chunks = append(chunks, chunk)
			c.emitted = true
		}
		c.buffer = trimLeftSpaceRunes(c.buffer[cut:])
	}
	return chunks
}

func isSoftSentencePause(r rune) bool {
	switch r {
	case '，', ',', '；', ';', '：', ':':
		return true
	default:
		return false
	}
}
