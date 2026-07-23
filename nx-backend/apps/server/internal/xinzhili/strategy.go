package xinzhili

import (
	"strings"
	"time"
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
	mode   Mode
	timing TimingConfig
	clock  Clock

	sessionStartedAt time.Time
	proactiveCount   int
	hasEverSpoken    bool
	deepPrompted     bool

	assistantPlaying bool
	hasValidText     bool
	hasStableText    bool
	highRisk         bool
	silenceStartedAt *time.Time

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
	applyTimingDefaults(&timing)
	return &Engine{
		mode:             mode,
		timing:           timing,
		clock:            clock,
		sessionStartedAt: clock.Now(),
	}
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
	case SignalAssistantStopped:
		e.assistantPlaying = false
	case SignalTurnReset:
		e.resetTurn()
	case SignalSessionReset:
		e.resetSession()
	}
	return nil
}

func (e *Engine) Tick() []Action {
	now := e.clock.Now()
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
	hadPending := e.silenceStartedAt != nil
	interruptedAssistant := e.assistantPlaying
	e.silenceStartedAt = nil
	e.midSentencePrompted = false

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

	if e.mode != ModeArgument || !signal.Stable || utf8.RuneCountInString(text) < minimumArgumentPartialRunes {
		e.clearPartialCandidate()
		return
	}

	now := e.clock.Now()
	if !e.partialSeen || !sameSemanticPrefix(e.partialPrefix, text) {
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
	e.highRisk = e.highRisk || signal.HighRisk
}

func (e *Engine) applySilence() {
	if !e.hasValidText || e.silenceStartedAt != nil {
		return
	}
	now := e.clock.Now()
	e.silenceStartedAt = &now
}

func (e *Engine) endpointAction(now time.Time) []Action {
	if e.silenceStartedAt == nil {
		return nil
	}
	elapsed := now.Sub(*e.silenceStartedAt)

	if e.mode == ModeArgument && !e.candidateEvaluated &&
		elapsed >= time.Duration(e.timing.ArgumentCandidateSilenceMs)*time.Millisecond {
		e.candidateEvaluated = true
		if e.argumentCandidate && !e.highRisk {
			e.resetTurn()
			return []Action{{Kind: ActionEndpoint}}
		}
	}

	threshold, eligible := e.endpointThreshold()
	if eligible && elapsed >= threshold {
		e.resetTurn()
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
	if e.timing.MaxProactivePrompts <= 0 || e.proactiveCount >= e.timing.MaxProactivePrompts {
		return nil
	}

	switch e.mode {
	case ModeComfort:
		if e.silenceStartedAt != nil && e.hasValidText && !e.hasStableText && !e.midSentencePrompted &&
			now.Sub(*e.silenceStartedAt) >= comfortMidSentencePause {
			e.midSentencePrompted = true
			e.proactiveCount++
			return []Action{{Kind: ActionComfortPrompt, TextKey: "comfort.mid_sentence"}}
		}
		if e.hasEverSpoken {
			return nil
		}
		elapsed := now.Sub(e.sessionStartedAt)
		if e.proactiveCount == 0 && elapsed >= time.Duration(e.timing.ComfortFirstPromptMs)*time.Millisecond {
			e.proactiveCount++
			return []Action{{Kind: ActionComfortPrompt, TextKey: "comfort.first_silence"}}
		}
		if e.proactiveCount == 1 && elapsed >= time.Duration(e.timing.ComfortSecondPromptMs)*time.Millisecond {
			e.proactiveCount++
			return []Action{{Kind: ActionComfortPrompt, TextKey: "comfort.second_silence"}}
		}
	case ModeDeepListening:
		if !e.deepPrompted && now.Sub(e.sessionStartedAt) >= time.Duration(e.timing.DeepListeningPromptMs)*time.Millisecond {
			e.deepPrompted = true
			e.proactiveCount++
			return []Action{{Kind: ActionComfortPrompt, TextKey: "deep_listening.silence"}}
		}
	}
	return nil
}

func (e *Engine) resetTurn() {
	e.assistantPlaying = false
	e.hasValidText = false
	e.hasStableText = false
	e.highRisk = false
	e.silenceStartedAt = nil
	e.partialPrefix = ""
	e.partialFirstSeenAt = time.Time{}
	e.partialSeen = false
	e.argumentCandidateSeen = false
	e.argumentCandidate = false
	e.candidateEvaluated = false
	e.midSentencePrompted = false
}

func (e *Engine) resetSession() {
	e.resetTurn()
	e.sessionStartedAt = e.clock.Now()
	e.proactiveCount = 0
	e.hasEverSpoken = false
	e.deepPrompted = false
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

func sameSemanticPrefix(previous, current string) bool {
	return strings.HasPrefix(previous, current) || strings.HasPrefix(current, previous)
}
