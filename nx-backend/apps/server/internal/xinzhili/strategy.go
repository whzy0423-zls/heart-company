package xinzhili

import (
	"strings"
	"time"
	"unicode"
	"unicode/utf8"
)

const (
	minimumArgumentPartialRunes = 4
	comfortMidSentencePause     = 2500 * time.Millisecond
)

type ActionKind int

const (
	ActionEndpoint ActionKind = iota
	ActionCancelPending
	ActionStopAssistant
	ActionComfortPrompt
	ActionQueueInterruptionPrefix
)

type Action struct {
	Kind    ActionKind
	TextKey string
}

type Clock interface {
	Now() time.Time
}

type SignalKind int

const (
	SignalUnknown SignalKind = iota
	SignalSpeechStarted
	SignalPartial
	SignalStableText
	SignalSilence
	SignalAssistantStarted
	SignalAssistantStopped
	SignalTurnReset
	SignalSessionReset
)

// Signal carries acoustic and semantic facts already decided by upstream
// components. The strategy deliberately does not infer risk or intent from
// transcript keywords.
type Signal struct {
	Kind                  SignalKind
	Transcript            string
	Stable                bool
	Noise                 bool
	ArgumentCandidate     bool
	HighRisk              bool
	ImportantInterruption bool
}

type Engine struct {
	mode    Mode
	timing  TimingConfig
	clock   Clock
	lastNow time.Time

	sessionStartedAt time.Time
	proactiveCount   int
	hasEverSpoken    bool
	deepPrompted     bool

	assistantPlaying         bool
	pendingGeneration        bool
	proactivePending         bool
	hasValidText             bool
	hasStableText            bool
	highRisk                 bool
	acousticSilenceAt        *time.Time
	deepSilenceAt            *time.Time
	partialFallbackText      string
	partialFallbackFirstAt   time.Time
	partialFallbackChangedAt time.Time

	partialPrefix         string
	partialFirstSeenAt    time.Time
	partialSeen           bool
	argumentCandidateSeen bool
	argumentCandidate     bool
	candidateEvaluated    bool
	midSentencePrompted   bool
}

func NewEngine(mode Mode, timing TimingConfig, clock Clock) *Engine {
	if clock == nil {
		panic("xinzhili: strategy Clock is required")
	}
	if !knownMode(mode) {
		panic("xinzhili: strategy mode is unknown")
	}
	applyTimingDefaults(&timing)
	if err := validateTiming(timing); err != nil {
		panic(err)
	}
	now := clock.Now()
	engine := &Engine{
		mode:             mode,
		timing:           timing,
		clock:            clock,
		lastNow:          now,
		sessionStartedAt: now,
	}
	if mode == ModeDeepListening {
		engine.deepSilenceAt = &now
	}
	return engine
}

func (e *Engine) Apply(signal Signal) []Action {
	switch signal.Kind {
	case SignalSpeechStarted:
		return e.applySpeechStarted(signal)
	case SignalPartial:
		e.applyPartial(signal)
	case SignalStableText:
		e.applyStableText(signal)
	case SignalSilence:
		e.applySilence()
	case SignalAssistantStarted:
		e.assistantPlaying = true
		e.pendingGeneration = false
		e.deepSilenceAt = nil
	case SignalAssistantStopped:
		e.assistantPlaying = false
		e.pendingGeneration = false
		e.proactivePending = false
		e.startDeepSilence()
	case SignalTurnReset:
		e.resetTurn()
	case SignalSessionReset:
		e.resetSession()
	}
	return nil
}

func (e *Engine) Tick() []Action {
	now := e.now()
	if action := e.endpointAction(now); action != nil {
		return action
	}
	if action := e.proactiveAction(now); action != nil {
		return action
	}
	return nil
}

func (e *Engine) applySpeechStarted(signal Signal) []Action {
	if signal.Noise {
		return nil
	}
	e.hasEverSpoken = true
	hadPending := e.acousticSilenceAt != nil
	hadPending = hadPending || e.pendingGeneration || e.proactivePending
	interruptedAssistant := e.assistantPlaying
	e.acousticSilenceAt = nil
	e.deepSilenceAt = nil
	e.midSentencePrompted = false
	e.pendingGeneration = false
	e.proactivePending = false

	var actions []Action
	if e.assistantPlaying {
		actions = append(actions, Action{Kind: ActionStopAssistant})
		e.assistantPlaying = false
		hadPending = true
	}
	if hadPending {
		actions = append(actions, Action{Kind: ActionCancelPending})
	}
	if e.mode == ModeArgument && interruptedAssistant && signal.ImportantInterruption && !signal.HighRisk {
		actions = append(actions, Action{
			Kind:    ActionQueueInterruptionPrefix,
			TextKey: "argument.important_interruption",
		})
	}
	return actions
}

func (e *Engine) applyPartial(signal Signal) {
	text := normalizeStrategyText(signal.Transcript)
	if signal.Noise || text == "" {
		return
	}
	e.hasEverSpoken = true
	e.hasValidText = true
	e.highRisk = e.highRisk || signal.HighRisk
	e.deepSilenceAt = nil
	e.recordPartialFallback(text)

	if e.mode != ModeArgument || !signal.Stable || utf8.RuneCountInString(text) < minimumArgumentPartialRunes {
		e.clearPartialCandidate()
		return
	}

	now := e.now()
	if !e.partialSeen || !hasStableLiteralPrefix(e.partialPrefix, text) {
		e.partialPrefix = text
		e.partialFirstSeenAt = now
		e.partialSeen = true
		e.argumentCandidateSeen = signal.ArgumentCandidate
		e.argumentCandidate = false
		return
	}
	e.argumentCandidateSeen = e.argumentCandidateSeen || signal.ArgumentCandidate

	if now.Sub(e.partialFirstSeenAt) < time.Duration(e.timing.PartialStableMs)*time.Millisecond {
		return
	}
	e.partialPrefix = text
	if e.argumentCandidateSeen && !e.highRisk && !e.candidateEvaluated {
		e.argumentCandidate = true
	}
}

func (e *Engine) applyStableText(signal Signal) {
	text := normalizeStrategyText(signal.Transcript)
	if signal.Noise || text == "" {
		return
	}
	e.hasEverSpoken = true
	e.hasValidText = true
	e.hasStableText = true
	e.clearPartialFallback()
	e.highRisk = e.highRisk || signal.HighRisk
	if e.acousticSilenceAt == nil {
		e.deepSilenceAt = nil
	}
	if e.mode == ModeArgument && e.partialSeen && !hasStableLiteralPrefix(e.partialPrefix, text) {
		e.clearPartialCandidate()
	}
}

func (e *Engine) applySilence() {
	e.startDeepSilence()
	if e.acousticSilenceAt != nil {
		return
	}
	now := e.now()
	e.acousticSilenceAt = &now
}

func (e *Engine) endpointAction(now time.Time) []Action {
	if e.partialFallbackText != "" {
		safeWindow := e.partialFallbackSafeWindow()
		if now.Sub(e.partialFallbackChangedAt) >= safeWindow ||
			now.Sub(e.partialFallbackFirstAt) >= partialFallbackHardLimit(safeWindow) {
			e.endpointTurn()
			return []Action{{Kind: ActionEndpoint}}
		}
	}
	if e.acousticSilenceAt == nil {
		return nil
	}
	elapsed := now.Sub(*e.acousticSilenceAt)

	if e.mode == ModeArgument && !e.candidateEvaluated &&
		elapsed >= time.Duration(e.timing.ArgumentCandidateSilenceMs)*time.Millisecond {
		e.candidateEvaluated = true
		if e.argumentCandidate && !e.highRisk {
			e.endpointTurn()
			return []Action{{Kind: ActionEndpoint}}
		}
	}

	threshold, eligible := e.endpointThreshold()
	if eligible && elapsed >= threshold {
		e.endpointTurn()
		return []Action{{Kind: ActionEndpoint}}
	}
	return nil
}

func (e *Engine) endpointThreshold() (time.Duration, bool) {
	switch e.mode {
	case ModeArgument:
		return time.Duration(e.timing.NormalEndSilenceMs) * time.Millisecond, e.hasValidText
	case ModeComfort:
		return time.Duration(e.timing.ComfortEndSilenceMs) * time.Millisecond, e.hasStableText
	case ModeDeepListening:
		return time.Duration(e.timing.DeepListeningEndSilenceMs) * time.Millisecond, e.hasStableText
	default:
		return time.Duration(e.timing.NormalEndSilenceMs) * time.Millisecond, e.hasStableText
	}
}

func (e *Engine) proactiveAction(now time.Time) []Action {
	if e.assistantPlaying || e.pendingGeneration || e.proactivePending ||
		e.timing.MaxProactivePrompts <= 0 || e.proactiveCount >= e.timing.MaxProactivePrompts {
		return nil
	}

	switch e.mode {
	case ModeComfort:
		if e.acousticSilenceAt != nil && e.hasValidText && !e.hasStableText && !e.midSentencePrompted &&
			now.Sub(*e.acousticSilenceAt) >= comfortMidSentencePause {
			e.midSentencePrompted = true
			e.proactiveCount++
			e.proactivePending = true
			return []Action{{Kind: ActionComfortPrompt, TextKey: "comfort.mid_sentence"}}
		}
		if e.hasEverSpoken {
			return nil
		}
		elapsed := now.Sub(e.sessionStartedAt)
		if e.proactiveCount == 0 && elapsed >= time.Duration(e.timing.ComfortFirstPromptMs)*time.Millisecond {
			e.proactiveCount++
			e.proactivePending = true
			return []Action{{Kind: ActionComfortPrompt, TextKey: "comfort.first_silence"}}
		}
		if e.proactiveCount == 1 && elapsed >= time.Duration(e.timing.ComfortSecondPromptMs)*time.Millisecond {
			e.proactiveCount++
			e.proactivePending = true
			return []Action{{Kind: ActionComfortPrompt, TextKey: "comfort.second_silence"}}
		}
	case ModeDeepListening:
		if !e.deepPrompted && !e.assistantPlaying && e.deepSilenceAt != nil &&
			now.Sub(*e.deepSilenceAt) >= time.Duration(e.timing.DeepListeningPromptMs)*time.Millisecond {
			e.deepPrompted = true
			e.proactiveCount++
			e.proactivePending = true
			return []Action{{Kind: ActionComfortPrompt, TextKey: "deep_listening.silence"}}
		}
	}
	return nil
}

func (e *Engine) clearTurnInput() {
	e.hasValidText = false
	e.hasStableText = false
	e.highRisk = false
	e.acousticSilenceAt = nil
	e.partialPrefix = ""
	e.partialFirstSeenAt = time.Time{}
	e.partialSeen = false
	e.argumentCandidateSeen = false
	e.argumentCandidate = false
	e.candidateEvaluated = false
	e.midSentencePrompted = false
	e.clearPartialFallback()
}

func (e *Engine) endpointTurn() {
	e.clearTurnInput()
	e.pendingGeneration = true
}

func (e *Engine) resetTurn() {
	e.clearTurnInput()
	e.pendingGeneration = false
	e.proactivePending = false
	if e.mode == ModeDeepListening {
		if e.assistantPlaying {
			e.deepSilenceAt = nil
		} else {
			e.resetDeepSilence()
		}
	}
}

func (e *Engine) resetSession() {
	e.assistantPlaying = false
	e.resetTurn()
	now := e.now()
	e.sessionStartedAt = now
	if e.mode == ModeDeepListening {
		e.deepSilenceAt = &now
	}
	e.proactiveCount = 0
	e.hasEverSpoken = false
	e.deepPrompted = false
}

func (e *Engine) startDeepSilence() {
	if e.mode != ModeDeepListening || e.deepSilenceAt != nil {
		return
	}
	e.resetDeepSilence()
}

func (e *Engine) resetDeepSilence() {
	now := e.now()
	e.deepSilenceAt = &now
}

func (e *Engine) clearPartialCandidate() {
	e.partialPrefix = ""
	e.partialFirstSeenAt = time.Time{}
	e.partialSeen = false
	e.argumentCandidateSeen = false
	e.argumentCandidate = false
}

func normalizeStrategyText(value string) string {
	return strings.Join(strings.Fields(value), " ")
}

func (e *Engine) recordPartialFallback(text string) {
	if !qualifiedPartialFallback(text) {
		return
	}
	now := e.now()
	if e.partialFallbackText == "" {
		e.partialFallbackText = text
		e.partialFallbackFirstAt = now
		e.partialFallbackChangedAt = now
		return
	}
	if text != e.partialFallbackText {
		e.partialFallbackText = text
		e.partialFallbackChangedAt = now
	}
}

func (e *Engine) clearPartialFallback() {
	e.partialFallbackText = ""
	e.partialFallbackFirstAt = time.Time{}
	e.partialFallbackChangedAt = time.Time{}
}

func (e *Engine) partialFallbackSafeWindow() time.Duration {
	var configured time.Duration
	switch e.mode {
	case ModeComfort:
		configured = time.Duration(e.timing.ComfortEndSilenceMs) * time.Millisecond
	case ModeDeepListening:
		configured = time.Duration(e.timing.DeepListeningEndSilenceMs) * time.Millisecond
	default:
		configured = time.Duration(e.timing.NormalEndSilenceMs) * time.Millisecond
	}
	return max(configured, 1600*time.Millisecond)
}

func partialFallbackHardLimit(safeWindow time.Duration) time.Duration {
	return min(max(3*safeWindow, 5*time.Second), 12*time.Second)
}

func qualifiedPartialFallback(text string) bool {
	seen := make(map[rune]struct{}, 2)
	for _, r := range text {
		if !unicode.IsLetter(r) && !unicode.IsDigit(r) {
			continue
		}
		seen[r] = struct{}{}
		if len(seen) >= 2 {
			return true
		}
	}
	return false
}

// hasStableLiteralPrefix intentionally performs a conservative literal prefix
// comparison. Punctuation or wording revisions cancel an argument candidate;
// semantic classification belongs to the upstream model.
func hasStableLiteralPrefix(previous, current string) bool {
	return strings.HasPrefix(previous, current) || strings.HasPrefix(current, previous)
}

func (e *Engine) now() time.Time {
	now := e.clock.Now()
	if now.Before(e.lastNow) {
		return e.lastNow
	}
	e.lastNow = now
	return now
}
