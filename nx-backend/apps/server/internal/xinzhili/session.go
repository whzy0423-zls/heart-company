package xinzhili

import (
	"context"
	"encoding/json"
	"errors"
	"strings"
	"sync"
	"time"
	"unicode"

	"nine-xing/nx-backend/apps/server/internal/rag"
)

const SceneXinzhiliVoice = "xinzhili_voice"

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
	UserID         int64
	CardID         int64
	ConversationID int64
	TurnID         string
	Mode           Mode
	ASRConfig      RealtimeASRConfig
	TTSConfig      TTSConfig
	Timing         TimingConfig
	CommonPrompt   string
	ModePrompt     string
	TopK           int
	MinScore       float64
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
	eventGenerationDelta
	eventGenerationDone
	eventTTSSegment
	eventTTSDone
)

type activeTurn struct {
	input          StartTurnInput
	conversation   Conversation
	card           Card
	asr            ASRSession
	engine         StrategyEngine
	ctx            context.Context
	cancel         context.CancelFunc
	hasSpeech      bool
	processing     bool
	draft          string
	answer         string
	sources        []rag.Source
	assistantID    int64
	segments       map[uint32]string
	lastAck        int64
	completionDone bool
	generationErr  error
	chunker        streamSentenceChunker
	ttsJobs        chan ttsStreamJob
	nextTTSSeq     uint32
}

type ttsStreamJob struct {
	seq  uint32
	text string
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
		}
	}
}

func (s *session) startTurn(ctx context.Context, input StartTurnInput) (*activeTurn, error) {
	input.TurnID = strings.TrimSpace(input.TurnID)
	if input.UserID <= 0 || input.CardID <= 0 || input.ConversationID < 0 || input.TurnID == "" || !knownMode(input.Mode) {
		return nil, errors.New("xinzhili: invalid turn start")
	}
	if s.deps.Cards == nil || s.deps.Conversations == nil || s.deps.ASRFactory == nil || s.deps.Sink == nil {
		return nil, errors.New("xinzhili: session dependencies missing")
	}
	if input.TopK <= 0 {
		input.TopK = 4
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
	turn := &activeTurn{input: input, conversation: conversation, card: card, asr: asr, ctx: turnCtx, cancel: cancel, segments: map[uint32]string{}, lastAck: -1}
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
	if turn == nil || ack.TurnID != turn.input.TurnID || turn.assistantID == 0 {
		return errors.New("xinzhili: assistant delivery not active")
	}
	if int64(ack.SegmentSeq) <= turn.lastAck {
		return nil
	}
	var builder strings.Builder
	for seq := uint32(0); seq <= ack.SegmentSeq; seq++ {
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
		if event.err != nil && !errors.Is(event.err, context.Canceled) {
			s.sendError(current, "tts_failed", "语音回复生成失败，请重试", true)
			return
		}
		current.completionDone = true
		if current.assistantID > 0 && current.generationErr == nil && current.answer != "" {
			sources, _ := json.Marshal(current.sources)
			_ = s.deps.Conversations.CompleteAssistant(current.ctx, current.assistantID, current.answer, sources)
		}
	}
}

func (s *session) handleASREvent(turn *activeTurn, event ASREvent) {
	if event.Kind == ASREventSpeechStarted {
		turn.hasSpeech = true
		if turn.engine != nil {
			turn.engine.Apply(Signal{Kind: SignalSpeechStarted})
		}
	}
	if event.Kind != ASREventFinal || !event.Stable || turn.processing {
		return
	}
	text := strings.TrimSpace(event.Final)
	if !hasMeaningfulTranscript(text) {
		return
	}
	turn.hasSpeech = true
	if turn.engine != nil {
		turn.engine.Apply(Signal{Kind: SignalStableText, Transcript: text, Stable: true})
	}
	if s.deps.Generator == nil {
		s.sendError(turn, "chat_model_not_configured", "请配置好会话模型后再重试", false)
		return
	}
	if _, err := s.deps.Conversations.SaveUser(turn.ctx, turn.conversation, text, turn.input.Mode); err != nil {
		s.sendError(turn, "conversation_save_failed", "会话保存失败，请重试", true)
		return
	}
	turn.processing = true
	turn.ttsJobs = make(chan ttsStreamJob, 128)
	s.startSynthesisWorker(turn)
	s.startGeneration(turn, text)
}

func (s *session) startGeneration(turn *activeTurn, question string) {
	go func() {
		history, summary, _ := s.deps.Conversations.History(turn.ctx, turn.conversation, 20)
		preferences := []string(nil)
		if s.deps.Preferences != nil {
			preferences, _ = s.deps.Preferences.PromptPreferences(turn.ctx, turn.input.UserID)
		}
		memories := []string(nil)
		if s.deps.Memories != nil {
			memories, _ = s.deps.Memories.PromptMemories(turn.ctx, turn.input.UserID, turn.input.CardID)
		}
		documents := make([]rag.Document, 0, turn.input.TopK*2)
		if s.deps.Knowledge != nil {
			if docs, err := s.deps.Knowledge.Search(turn.ctx, question, turn.input.TopK, turn.input.MinScore); err == nil {
				documents = appendUniqueDocuments(documents, docs)
			}
		}
		if s.deps.Theory != nil {
			if docs, err := s.deps.Theory.Search(turn.ctx, question, turn.input.TopK, turn.input.MinScore); err == nil {
				documents = appendUniqueDocuments(documents, docs)
			}
		}
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
	if s.deps.Synthesizer == nil {
		s.sendError(turn, "tts_not_configured", "请配置好语音模型后再重试", false)
		return
	}
	go func() {
		var workerErr error
		for job := range turn.ttsJobs {
			emitted := false
			workerErr = s.deps.Synthesizer.Synthesize(turn.ctx, turn.input.TTSConfig, job.text, func(segment AudioSegment) error {
				if emitted {
					return errors.New("xinzhili: streamed TTS chunk produced multiple segments")
				}
				emitted = true
				segment.Seq = job.seq
				segment.deliveryText = job.text
				response := make(chan error, 1)
				if !s.postEvent(sessionEvent{kind: eventTTSSegment, turnID: turn.input.TurnID, segment: segment, segmentAck: response}) {
					return ErrSessionClosed
				}
				select {
				case err := <-response:
					return err
				case <-turn.ctx.Done():
					return turn.ctx.Err()
				}
			})
			if workerErr != nil {
				break
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
	text := segment.DeliveryText()
	if text == "" {
		return errors.New("xinzhili: TTS segment delivery text missing")
	}
	if _, exists := turn.segments[segment.Seq]; exists {
		return errors.New("xinzhili: duplicate TTS segment")
	}
	if err := s.deps.Sink.SendAudio(turn.ctx, segment); err != nil {
		return err
	}
	turn.segments[segment.Seq] = text
	if turn.assistantID == 0 {
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

func (s *session) sendError(turn *activeTurn, code, message string, retryable bool) {
	payload, _ := json.Marshal(ErrorPayload{Code: code, Message: message, Retryable: retryable, Fatal: false})
	turnID := turn.input.TurnID
	seq := uint64(1)
	turnSeq := uint64(1)
	_ = s.deps.Sink.SendControl(turn.ctx, Envelope{
		ProtocolVersion: ProtocolVersion, Type: EventError, TurnID: &turnID,
		SessionSeq: &seq, TurnSeq: &turnSeq, TimestampMs: time.Now().UnixMilli(), Payload: payload,
	})
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

type streamSentenceChunker struct{ buffer []rune }

func (c *streamSentenceChunker) Push(delta string) []string {
	c.buffer = append(c.buffer, []rune(delta)...)
	return c.take(false)
}

func (c *streamSentenceChunker) Flush() []string { return c.take(true) }

func (c *streamSentenceChunker) take(flush bool) []string {
	var chunks []string
	for len(c.buffer) > 0 {
		cut := 0
		limit := min(len(c.buffer), maxTTSSentenceRunes)
		for index := 0; index < limit; index++ {
			if isStrongSentenceEndAt(c.buffer, index) {
				cut = index + 1
				for cut < limit && isClosingQuote(c.buffer[cut]) {
					cut++
				}
				break
			}
		}
		if cut == 0 && len(c.buffer) >= maxTTSSentenceRunes {
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
		}
		c.buffer = trimLeftSpaceRunes(c.buffer[cut:])
	}
	return chunks
}
