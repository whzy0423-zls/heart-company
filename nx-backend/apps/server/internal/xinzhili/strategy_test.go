package xinzhili

import (
	"reflect"
	"testing"
	"time"
)

type strategyFakeClock struct{ now time.Time }

func newStrategyFakeClock() *strategyFakeClock {
	return &strategyFakeClock{now: time.Date(2026, 7, 23, 12, 0, 0, 0, time.UTC)}
}

func (c *strategyFakeClock) Now() time.Time          { return c.now }
func (c *strategyFakeClock) Advance(d time.Duration) { c.now = c.now.Add(d) }

func (c *strategyFakeClock) Rewind(d time.Duration) { c.now = c.now.Add(-d) }

func strategyTiming() TimingConfig {
	return TimingConfig{
		PartialStableMs: 150, ArgumentCandidateSilenceMs: 350, NormalEndSilenceMs: 700,
		ComfortEndSilenceMs: 1200, DeepListeningEndSilenceMs: 1500,
		ComfortFirstPromptMs: 5000, ComfortSecondPromptMs: 12000,
		DeepListeningPromptMs: 12000, MaxProactivePrompts: 2,
	}
}

func assertStrategyActions(t *testing.T, got []Action, want ...Action) {
	t.Helper()
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("actions = %#v, want %#v", got, want)
	}
}

func TestStrategyNormalEndpointsAfterStableSpeechAnd700msSilence(t *testing.T) {
	clock := newStrategyFakeClock()
	engine := NewEngine(ModeNormal, strategyTiming(), clock)
	assertStrategyActions(t, engine.Apply(Signal{Kind: SignalStableText, Transcript: "我想认真聊聊"}))
	assertStrategyActions(t, engine.Apply(Signal{Kind: SignalSilence}))
	clock.Advance(699 * time.Millisecond)
	assertStrategyActions(t, engine.Tick())
	clock.Advance(time.Millisecond)
	assertStrategyActions(t, engine.Tick(), Action{Kind: ActionEndpoint})
	assertStrategyActions(t, engine.Tick())
}

func TestStrategyNormalContinuedSpeechCancelsEndpointTimer(t *testing.T) {
	clock := newStrategyFakeClock()
	engine := NewEngine(ModeNormal, strategyTiming(), clock)
	engine.Apply(Signal{Kind: SignalStableText, Transcript: "第一句"})
	engine.Apply(Signal{Kind: SignalSilence})
	clock.Advance(500 * time.Millisecond)
	assertStrategyActions(t, engine.Apply(Signal{Kind: SignalSpeechStarted}), Action{Kind: ActionCancelPending})
	clock.Advance(time.Second)
	assertStrategyActions(t, engine.Tick())
}

func TestStrategyNormalIgnoresBlankAndNoise(t *testing.T) {
	clock := newStrategyFakeClock()
	engine := NewEngine(ModeNormal, strategyTiming(), clock)
	engine.Apply(Signal{Kind: SignalStableText, Transcript: " \n\t"})
	engine.Apply(Signal{Kind: SignalPartial, Transcript: "背景声", Stable: true, Noise: true})
	engine.Apply(Signal{Kind: SignalSilence})
	clock.Advance(2 * time.Second)
	assertStrategyActions(t, engine.Tick())
}

func TestStrategyQualifiedPartialEndpointsAfterSafeStableWindow(t *testing.T) {
	clock := newStrategyFakeClock()
	engine := NewEngine(ModeNormal, strategyTiming(), clock)
	engine.Apply(Signal{Kind: SignalPartial, Transcript: "在吗在吗"})
	clock.Advance(1599 * time.Millisecond)
	assertStrategyActions(t, engine.Tick())
	clock.Advance(time.Millisecond)
	assertStrategyActions(t, engine.Tick(), Action{Kind: ActionEndpoint})
	assertStrategyActions(t, engine.Tick())
}

func TestStrategyRepeatedQualifiedPartialDoesNotResetStableWindow(t *testing.T) {
	clock := newStrategyFakeClock()
	engine := NewEngine(ModeNormal, strategyTiming(), clock)
	engine.Apply(Signal{Kind: SignalPartial, Transcript: "你在吗"})
	clock.Advance(time.Second)
	engine.Apply(Signal{Kind: SignalPartial, Transcript: "你在吗"})
	clock.Advance(599 * time.Millisecond)
	assertStrategyActions(t, engine.Tick())
	clock.Advance(time.Millisecond)
	assertStrategyActions(t, engine.Tick(), Action{Kind: ActionEndpoint})
}

func TestStrategyGrowingPartialResetsStableWindow(t *testing.T) {
	clock := newStrategyFakeClock()
	engine := NewEngine(ModeNormal, strategyTiming(), clock)
	engine.Apply(Signal{Kind: SignalPartial, Transcript: "我想说"})
	clock.Advance(time.Second)
	engine.Apply(Signal{Kind: SignalPartial, Transcript: "我想说一件事"})
	clock.Advance(1599 * time.Millisecond)
	assertStrategyActions(t, engine.Tick())
	clock.Advance(time.Millisecond)
	assertStrategyActions(t, engine.Tick(), Action{Kind: ActionEndpoint})
}

func TestStrategyShortPauseBeforePartialGrowthDoesNotEndpoint(t *testing.T) {
	clock := newStrategyFakeClock()
	engine := NewEngine(ModeNormal, strategyTiming(), clock)
	engine.Apply(Signal{Kind: SignalPartial, Transcript: "我想说"})
	clock.Advance(1200 * time.Millisecond)
	engine.Apply(Signal{Kind: SignalPartial, Transcript: "我想说一件事"})
	clock.Advance(400 * time.Millisecond)
	assertStrategyActions(t, engine.Tick())
}

func TestStrategyPartialFallbackRejectsNoiseSingleAndRepeatedCharacters(t *testing.T) {
	for _, signal := range []Signal{
		{Kind: SignalPartial, Transcript: "在"},
		{Kind: SignalPartial, Transcript: "嗯嗯嗯"},
		{Kind: SignalPartial, Transcript: "在吗", Noise: true},
	} {
		clock := newStrategyFakeClock()
		engine := NewEngine(ModeNormal, strategyTiming(), clock)
		engine.Apply(signal)
		clock.Advance(20 * time.Second)
		assertStrategyActions(t, engine.Tick())
	}
}

func TestStrategyContinuouslyChangingPartialHitsBoundedHardLimit(t *testing.T) {
	clock := newStrategyFakeClock()
	engine := NewEngine(ModeNormal, strategyTiming(), clock)
	engine.Apply(Signal{Kind: SignalPartial, Transcript: "我想说"})
	for _, text := range []string{"我想说一", "我想说一件", "我想说一件很", "我想说一件很长"} {
		clock.Advance(time.Second)
		engine.Apply(Signal{Kind: SignalPartial, Transcript: text})
		assertStrategyActions(t, engine.Tick())
	}
	clock.Advance(time.Second)
	engine.Apply(Signal{Kind: SignalPartial, Transcript: "我想说一件很长的事"})
	assertStrategyActions(t, engine.Tick(), Action{Kind: ActionEndpoint})
}

func TestStrategyStableFinalUsesFinalSilenceAndDoesNotDuplicatePartialEndpoint(t *testing.T) {
	clock := newStrategyFakeClock()
	engine := NewEngine(ModeNormal, strategyTiming(), clock)
	engine.Apply(Signal{Kind: SignalPartial, Transcript: "我想聊聊"})
	clock.Advance(time.Second)
	engine.Apply(Signal{Kind: SignalStableText, Transcript: "我想聊聊近况"})
	engine.Apply(Signal{Kind: SignalSilence})
	clock.Advance(700 * time.Millisecond)
	assertStrategyActions(t, engine.Tick(), Action{Kind: ActionEndpoint})
	clock.Advance(10 * time.Second)
	assertStrategyActions(t, engine.Tick())
}

func TestStrategyNormalKeepsAcousticSilenceBeforeDelayedFinal(t *testing.T) {
	clock := newStrategyFakeClock()
	engine := NewEngine(ModeNormal, strategyTiming(), clock)
	engine.Apply(Signal{Kind: SignalSpeechStarted})
	engine.Apply(Signal{Kind: SignalSilence})
	clock.Advance(300 * time.Millisecond)
	engine.Apply(Signal{Kind: SignalStableText, Transcript: "延迟到达的最终文本"})

	clock.Advance(399 * time.Millisecond)
	assertStrategyActions(t, engine.Tick())
	clock.Advance(time.Millisecond)
	assertStrategyActions(t, engine.Tick(), Action{Kind: ActionEndpoint})
}

func TestStrategyComfortDelayedFinalAfterThresholdEndpointsImmediately(t *testing.T) {
	clock := newStrategyFakeClock()
	engine := NewEngine(ModeComfort, strategyTiming(), clock)
	engine.Apply(Signal{Kind: SignalSpeechStarted})
	engine.Apply(Signal{Kind: SignalSilence})
	clock.Advance(1500 * time.Millisecond)
	engine.Apply(Signal{Kind: SignalStableText, Transcript: "现在才收到最终文本"})

	assertStrategyActions(t, engine.Tick(), Action{Kind: ActionEndpoint})
}

func TestStrategyArgumentEndpointsCandidateAfterStablePartialAnd350ms(t *testing.T) {
	clock := newStrategyFakeClock()
	engine := NewEngine(ModeArgument, strategyTiming(), clock)
	engine.Apply(Signal{Kind: SignalPartial, Transcript: "我总是做不到", Stable: true, ArgumentCandidate: true})
	clock.Advance(150 * time.Millisecond)
	engine.Apply(Signal{Kind: SignalPartial, Transcript: "我总是做不到任何事", Stable: true})
	engine.Apply(Signal{Kind: SignalSilence})
	clock.Advance(349 * time.Millisecond)
	assertStrategyActions(t, engine.Tick())
	clock.Advance(time.Millisecond)
	assertStrategyActions(t, engine.Tick(), Action{Kind: ActionEndpoint})
	assertStrategyActions(t, engine.Tick())
}

func TestStrategyArgumentCandidateIsEvaluatedOnlyOncePerPause(t *testing.T) {
	clock := newStrategyFakeClock()
	engine := NewEngine(ModeArgument, strategyTiming(), clock)
	engine.Apply(Signal{Kind: SignalPartial, Transcript: "我肯定永远不行", Stable: true})
	clock.Advance(150 * time.Millisecond)
	engine.Apply(Signal{Kind: SignalPartial, Transcript: "我肯定永远不行了", Stable: true})
	engine.Apply(Signal{Kind: SignalSilence})
	clock.Advance(350 * time.Millisecond)
	assertStrategyActions(t, engine.Tick())
	engine.Apply(Signal{Kind: SignalPartial, Transcript: "我肯定永远不行了", Stable: true, ArgumentCandidate: true})
	clock.Advance(350 * time.Millisecond)
	assertStrategyActions(t, engine.Tick(), Action{Kind: ActionEndpoint})
}

func TestStrategyArgumentMissFallsBackToNormal700ms(t *testing.T) {
	clock := newStrategyFakeClock()
	engine := NewEngine(ModeArgument, strategyTiming(), clock)
	engine.Apply(Signal{Kind: SignalPartial, Transcript: "这是一段完整表达", Stable: true})
	clock.Advance(150 * time.Millisecond)
	engine.Apply(Signal{Kind: SignalPartial, Transcript: "这是一段完整表达内容", Stable: true})
	engine.Apply(Signal{Kind: SignalSilence})
	clock.Advance(350 * time.Millisecond)
	assertStrategyActions(t, engine.Tick())
	clock.Advance(349 * time.Millisecond)
	assertStrategyActions(t, engine.Tick())
	clock.Advance(time.Millisecond)
	assertStrategyActions(t, engine.Tick(), Action{Kind: ActionEndpoint})
}

func TestStrategyArgumentRequiresTwoMatchingStablePartialsFor150ms(t *testing.T) {
	tests := []struct {
		name, first string
		advance     time.Duration
		second      Signal
	}{
		{name: "only one partial", first: "我真的完全做不到", advance: 150 * time.Millisecond},
		{name: "too soon", first: "我真的完全做不到", second: Signal{Kind: SignalPartial, Transcript: "我真的完全做不到", Stable: true, ArgumentCandidate: true}},
		{name: "too short", first: "我不行", advance: 150 * time.Millisecond, second: Signal{Kind: SignalPartial, Transcript: "我不行", Stable: true, ArgumentCandidate: true}},
		{name: "unstable", first: "我真的完全做不到", advance: 150 * time.Millisecond, second: Signal{Kind: SignalPartial, Transcript: "我真的完全做不到", ArgumentCandidate: true}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			clock := newStrategyFakeClock()
			engine := NewEngine(ModeArgument, strategyTiming(), clock)
			engine.Apply(Signal{Kind: SignalPartial, Transcript: tt.first, Stable: true, ArgumentCandidate: true})
			clock.Advance(tt.advance)
			if tt.second.Kind != SignalUnknown {
				engine.Apply(tt.second)
			}
			engine.Apply(Signal{Kind: SignalSilence})
			clock.Advance(350 * time.Millisecond)
			assertStrategyActions(t, engine.Tick())
		})
	}
}

func TestStrategyArgumentPartialRevisionCancelsCandidate(t *testing.T) {
	clock := newStrategyFakeClock()
	engine := NewEngine(ModeArgument, strategyTiming(), clock)
	engine.Apply(Signal{Kind: SignalPartial, Transcript: "我永远都做不好", Stable: true, ArgumentCandidate: true})
	clock.Advance(150 * time.Millisecond)
	engine.Apply(Signal{Kind: SignalPartial, Transcript: "我永远都做不好这件事", Stable: true, ArgumentCandidate: true})
	engine.Apply(Signal{Kind: SignalPartial, Transcript: "其实我只是今天有点累", Stable: true})
	engine.Apply(Signal{Kind: SignalSilence})
	clock.Advance(350 * time.Millisecond)
	assertStrategyActions(t, engine.Tick())
	clock.Advance(350 * time.Millisecond)
	assertStrategyActions(t, engine.Tick(), Action{Kind: ActionEndpoint})
}

func TestStrategyArgumentFinalRevisionCancelsCandidate(t *testing.T) {
	clock := newStrategyFakeClock()
	engine := NewEngine(ModeArgument, strategyTiming(), clock)
	engine.Apply(Signal{Kind: SignalPartial, Transcript: "我永远都做不好", Stable: true, ArgumentCandidate: true})
	clock.Advance(150 * time.Millisecond)
	engine.Apply(Signal{Kind: SignalPartial, Transcript: "我永远都做不好这件事", Stable: true})
	engine.Apply(Signal{Kind: SignalStableText, Transcript: "其实我只是今天有点累"})
	engine.Apply(Signal{Kind: SignalSilence})

	clock.Advance(350 * time.Millisecond)
	assertStrategyActions(t, engine.Tick())
	clock.Advance(350 * time.Millisecond)
	assertStrategyActions(t, engine.Tick(), Action{Kind: ActionEndpoint})
}

func TestStrategyArgumentMatchingFinalKeepsCandidate(t *testing.T) {
	clock := newStrategyFakeClock()
	engine := NewEngine(ModeArgument, strategyTiming(), clock)
	engine.Apply(Signal{Kind: SignalPartial, Transcript: "我永远都做不好", Stable: true, ArgumentCandidate: true})
	clock.Advance(150 * time.Millisecond)
	engine.Apply(Signal{Kind: SignalPartial, Transcript: "我永远都做不好这件事", Stable: true})
	engine.Apply(Signal{Kind: SignalStableText, Transcript: "我永远都做不好这件事情"})
	engine.Apply(Signal{Kind: SignalSilence})

	clock.Advance(350 * time.Millisecond)
	assertStrategyActions(t, engine.Tick(), Action{Kind: ActionEndpoint})
}

func TestStrategyArgumentPunctuationRevisionCancelsLiteralPrefixCandidate(t *testing.T) {
	clock := newStrategyFakeClock()
	engine := NewEngine(ModeArgument, strategyTiming(), clock)
	engine.Apply(Signal{Kind: SignalPartial, Transcript: "我永远做不好", Stable: true, ArgumentCandidate: true})
	clock.Advance(150 * time.Millisecond)
	engine.Apply(Signal{Kind: SignalPartial, Transcript: "我永远做不好这件事", Stable: true})
	engine.Apply(Signal{Kind: SignalStableText, Transcript: "我永远，做不好这件事"})
	engine.Apply(Signal{Kind: SignalSilence})

	clock.Advance(350 * time.Millisecond)
	assertStrategyActions(t, engine.Tick())
	clock.Advance(350 * time.Millisecond)
	assertStrategyActions(t, engine.Tick(), Action{Kind: ActionEndpoint})
}

func TestStrategyArgumentBreathAndNoiseDoNotTrigger(t *testing.T) {
	clock := newStrategyFakeClock()
	engine := NewEngine(ModeArgument, strategyTiming(), clock)
	engine.Apply(Signal{Kind: SignalPartial, Transcript: "嗯", Stable: true, Noise: true, ArgumentCandidate: true})
	clock.Advance(150 * time.Millisecond)
	engine.Apply(Signal{Kind: SignalPartial, Transcript: "呼吸声", Stable: true, Noise: true, ArgumentCandidate: true})
	engine.Apply(Signal{Kind: SignalSilence})
	clock.Advance(time.Second)
	assertStrategyActions(t, engine.Tick())
}

func TestStrategyArgumentHighRiskUsesSafeNormalEndpoint(t *testing.T) {
	clock := newStrategyFakeClock()
	engine := NewEngine(ModeArgument, strategyTiming(), clock)
	engine.Apply(Signal{Kind: SignalPartial, Transcript: "这是需要安全处理的重要内容", Stable: true, ArgumentCandidate: true, HighRisk: true})
	clock.Advance(150 * time.Millisecond)
	engine.Apply(Signal{Kind: SignalPartial, Transcript: "这是需要安全处理的重要内容补充", Stable: true, ArgumentCandidate: true, HighRisk: true})
	engine.Apply(Signal{Kind: SignalSilence})
	clock.Advance(350 * time.Millisecond)
	assertStrategyActions(t, engine.Tick())
	clock.Advance(350 * time.Millisecond)
	assertStrategyActions(t, engine.Tick(), Action{Kind: ActionEndpoint})
}

func TestStrategyImportantAssistantInterruptionQueuesPrefixForNextTurn(t *testing.T) {
	clock := newStrategyFakeClock()
	engine := NewEngine(ModeArgument, strategyTiming(), clock)
	engine.Apply(Signal{Kind: SignalAssistantStarted})
	assertStrategyActions(t, engine.Apply(Signal{Kind: SignalSpeechStarted, ImportantInterruption: true}),
		Action{Kind: ActionStopAssistant}, Action{Kind: ActionCancelPending},
		Action{Kind: ActionQueueInterruptionPrefix, TextKey: "argument.important_interruption"})
}

func TestStrategyHighRiskInterruptionDoesNotQueuePlayfulPrefix(t *testing.T) {
	clock := newStrategyFakeClock()
	engine := NewEngine(ModeArgument, strategyTiming(), clock)
	engine.Apply(Signal{Kind: SignalAssistantStarted})
	assertStrategyActions(t, engine.Apply(Signal{Kind: SignalSpeechStarted, ImportantInterruption: true, HighRisk: true}),
		Action{Kind: ActionStopAssistant}, Action{Kind: ActionCancelPending})
}

func TestStrategyImportantFlagWithoutAssistantPlaybackDoesNotQueuePrefix(t *testing.T) {
	clock := newStrategyFakeClock()
	engine := NewEngine(ModeArgument, strategyTiming(), clock)
	assertStrategyActions(t, engine.Apply(Signal{Kind: SignalSpeechStarted, ImportantInterruption: true}))
}

func TestStrategyComfortEndpointsAfter1200ms(t *testing.T) {
	clock := newStrategyFakeClock()
	engine := NewEngine(ModeComfort, strategyTiming(), clock)
	engine.Apply(Signal{Kind: SignalStableText, Transcript: "我今天心里有点乱"})
	engine.Apply(Signal{Kind: SignalSilence})
	clock.Advance(1199 * time.Millisecond)
	assertStrategyActions(t, engine.Tick())
	clock.Advance(time.Millisecond)
	assertStrategyActions(t, engine.Tick(), Action{Kind: ActionEndpoint})
}

func TestStrategyComfortPromptsAt5And12SecondsAtMostTwice(t *testing.T) {
	clock := newStrategyFakeClock()
	engine := NewEngine(ModeComfort, strategyTiming(), clock)
	clock.Advance(4999 * time.Millisecond)
	assertStrategyActions(t, engine.Tick())
	clock.Advance(time.Millisecond)
	assertStrategyActions(t, engine.Tick(), Action{Kind: ActionComfortPrompt, TextKey: "comfort.first_silence"})
	assertStrategyActions(t, engine.Tick())
	engine.Apply(Signal{Kind: SignalAssistantStarted})
	engine.Apply(Signal{Kind: SignalAssistantStopped})
	clock.Advance(6999 * time.Millisecond)
	assertStrategyActions(t, engine.Tick())
	clock.Advance(time.Millisecond)
	assertStrategyActions(t, engine.Tick(), Action{Kind: ActionComfortPrompt, TextKey: "comfort.second_silence"})
	engine.Apply(Signal{Kind: SignalAssistantStarted})
	engine.Apply(Signal{Kind: SignalAssistantStopped})
	clock.Advance(time.Minute)
	assertStrategyActions(t, engine.Tick())
}

func TestStrategyComfortProactivePromptWaitsForCompletionBeforeSecond(t *testing.T) {
	clock := newStrategyFakeClock()
	engine := NewEngine(ModeComfort, strategyTiming(), clock)
	clock.Advance(12 * time.Second)

	assertStrategyActions(t, engine.Tick(), Action{Kind: ActionComfortPrompt, TextKey: "comfort.first_silence"})
	assertStrategyActions(t, engine.Tick())
	engine.Apply(Signal{Kind: SignalAssistantStarted})
	assertStrategyActions(t, engine.Tick())
	engine.Apply(Signal{Kind: SignalAssistantStopped})
	assertStrategyActions(t, engine.Tick(), Action{Kind: ActionComfortPrompt, TextKey: "comfort.second_silence"})
	assertStrategyActions(t, engine.Tick())
}

func TestStrategySpeechCancelsUnplayedComfortPromptWithoutFakeStop(t *testing.T) {
	clock := newStrategyFakeClock()
	engine := NewEngine(ModeComfort, strategyTiming(), clock)
	clock.Advance(5 * time.Second)
	assertStrategyActions(t, engine.Tick(), Action{Kind: ActionComfortPrompt, TextKey: "comfort.first_silence"})

	assertStrategyActions(t, engine.Apply(Signal{Kind: SignalSpeechStarted}), Action{Kind: ActionCancelPending})
	clock.Advance(7 * time.Second)
	assertStrategyActions(t, engine.Tick())
}

func TestStrategyComfortQualifiedPartialEndpointsBeforeMidSentencePrompt(t *testing.T) {
	clock := newStrategyFakeClock()
	engine := NewEngine(ModeComfort, strategyTiming(), clock)
	engine.Apply(Signal{Kind: SignalSpeechStarted})
	engine.Apply(Signal{Kind: SignalPartial, Transcript: "我不知道该怎么", Stable: true})
	engine.Apply(Signal{Kind: SignalSilence})
	clock.Advance(1599 * time.Millisecond)
	assertStrategyActions(t, engine.Tick())
	clock.Advance(time.Millisecond)
	assertStrategyActions(t, engine.Tick(), Action{Kind: ActionEndpoint})
	clock.Advance(time.Second)
	assertStrategyActions(t, engine.Tick())
}

func TestStrategySpeechStopsActiveComfortImmediately(t *testing.T) {
	clock := newStrategyFakeClock()
	engine := NewEngine(ModeComfort, strategyTiming(), clock)
	clock.Advance(5 * time.Second)
	assertStrategyActions(t, engine.Tick(), Action{Kind: ActionComfortPrompt, TextKey: "comfort.first_silence"})
	engine.Apply(Signal{Kind: SignalAssistantStarted})
	assertStrategyActions(t, engine.Apply(Signal{Kind: SignalSpeechStarted}),
		Action{Kind: ActionStopAssistant}, Action{Kind: ActionCancelPending})
	clock.Advance(7 * time.Second)
	assertStrategyActions(t, engine.Tick())
}

func TestStrategyDeepListeningEndpointsAfter1500ms(t *testing.T) {
	clock := newStrategyFakeClock()
	engine := NewEngine(ModeDeepListening, strategyTiming(), clock)
	engine.Apply(Signal{Kind: SignalStableText, Transcript: "我想慢慢说完"})
	engine.Apply(Signal{Kind: SignalSilence})
	clock.Advance(1499 * time.Millisecond)
	assertStrategyActions(t, engine.Tick())
	clock.Advance(time.Millisecond)
	assertStrategyActions(t, engine.Tick(), Action{Kind: ActionEndpoint})
}

func TestStrategyDeepListeningLongSilencePromptsOnlyOnceAndNeverInterrupts(t *testing.T) {
	clock := newStrategyFakeClock()
	engine := NewEngine(ModeDeepListening, strategyTiming(), clock)
	clock.Advance(12 * time.Second)
	assertStrategyActions(t, engine.Tick(), Action{Kind: ActionComfortPrompt, TextKey: "deep_listening.silence"})
	engine.Apply(Signal{Kind: SignalAssistantStarted})
	engine.Apply(Signal{Kind: SignalAssistantStopped})
	clock.Advance(time.Minute)
	assertStrategyActions(t, engine.Tick())
}

func TestStrategyDeepListeningBoundsQualifiedOngoingPartial(t *testing.T) {
	clock := newStrategyFakeClock()
	engine := NewEngine(ModeDeepListening, strategyTiming(), clock)

	clock.Advance(6 * time.Second)
	engine.Apply(Signal{Kind: SignalSpeechStarted})
	engine.Apply(Signal{Kind: SignalPartial, Transcript: "我还在慢慢讲", Stable: true})
	clock.Advance(1599 * time.Millisecond)
	assertStrategyActions(t, engine.Tick())
	clock.Advance(time.Millisecond)
	assertStrategyActions(t, engine.Tick(), Action{Kind: ActionEndpoint})
}

func TestStrategyDeepListeningDoesNotPromptWhileAssistantIsPlaying(t *testing.T) {
	clock := newStrategyFakeClock()
	engine := NewEngine(ModeDeepListening, strategyTiming(), clock)
	engine.Apply(Signal{Kind: SignalAssistantStarted})

	clock.Advance(12 * time.Second)
	assertStrategyActions(t, engine.Tick())

	engine.Apply(Signal{Kind: SignalAssistantStopped})
	clock.Advance(11999 * time.Millisecond)
	assertStrategyActions(t, engine.Tick())
	clock.Advance(time.Millisecond)
	assertStrategyActions(t, engine.Tick(), Action{Kind: ActionComfortPrompt, TextKey: "deep_listening.silence"})
}

func TestStrategySpeechDuringAssistantPlaybackAlwaysStopsAndCancelsFirst(t *testing.T) {
	for _, mode := range []Mode{ModeNormal, ModeArgument, ModeComfort, ModeDeepListening} {
		t.Run(string(mode), func(t *testing.T) {
			clock := newStrategyFakeClock()
			engine := NewEngine(mode, strategyTiming(), clock)
			engine.Apply(Signal{Kind: SignalAssistantStarted})
			assertStrategyActions(t, engine.Apply(Signal{Kind: SignalSpeechStarted}),
				Action{Kind: ActionStopAssistant}, Action{Kind: ActionCancelPending})
		})
	}
}

func TestStrategySpeechAfterEndpointCancelsPendingGenerationWithoutFakeStop(t *testing.T) {
	for _, mode := range []Mode{ModeNormal, ModeArgument, ModeComfort, ModeDeepListening} {
		t.Run(string(mode), func(t *testing.T) {
			clock := newStrategyFakeClock()
			engine := NewEngine(mode, strategyTiming(), clock)
			engine.Apply(Signal{Kind: SignalStableText, Transcript: "已经完成的一轮表达"})
			engine.Apply(Signal{Kind: SignalSilence})
			threshold := map[Mode]time.Duration{
				ModeNormal:        700 * time.Millisecond,
				ModeArgument:      700 * time.Millisecond,
				ModeComfort:       1200 * time.Millisecond,
				ModeDeepListening: 1500 * time.Millisecond,
			}[mode]
			clock.Advance(threshold)
			assertStrategyActions(t, engine.Tick(), Action{Kind: ActionEndpoint})

			assertStrategyActions(t, engine.Apply(Signal{Kind: SignalSpeechStarted}),
				Action{Kind: ActionCancelPending})
		})
	}
}

func TestStrategyTurnResetKeepsSessionPromptLimit(t *testing.T) {
	clock := newStrategyFakeClock()
	engine := NewEngine(ModeComfort, strategyTiming(), clock)
	clock.Advance(5 * time.Second)
	assertStrategyActions(t, engine.Tick(), Action{Kind: ActionComfortPrompt, TextKey: "comfort.first_silence"})
	engine.Apply(Signal{Kind: SignalTurnReset})
	clock.Advance(7 * time.Second)
	assertStrategyActions(t, engine.Tick(), Action{Kind: ActionComfortPrompt, TextKey: "comfort.second_silence"})
	engine.Apply(Signal{Kind: SignalTurnReset})
	clock.Advance(time.Minute)
	assertStrategyActions(t, engine.Tick())
}

func TestStrategyDeepListeningTurnResetStartsFreshSilenceWindow(t *testing.T) {
	clock := newStrategyFakeClock()
	engine := NewEngine(ModeDeepListening, strategyTiming(), clock)
	clock.Advance(10 * time.Second)
	engine.Apply(Signal{Kind: SignalTurnReset})

	clock.Advance(11999 * time.Millisecond)
	assertStrategyActions(t, engine.Tick())
	clock.Advance(time.Millisecond)
	assertStrategyActions(t, engine.Tick(), Action{Kind: ActionComfortPrompt, TextKey: "deep_listening.silence"})
}

func TestStrategyMonotonicClockClampPreventsRollbackFromRegressingTurnResetBaseline(t *testing.T) {
	clock := newStrategyFakeClock()
	engine := NewEngine(ModeDeepListening, strategyTiming(), clock)
	clock.Advance(6 * time.Second)
	assertStrategyActions(t, engine.Tick())
	clock.Rewind(5 * time.Second)
	engine.Apply(Signal{Kind: SignalTurnReset})

	clock.Advance(12 * time.Second)
	assertStrategyActions(t, engine.Tick())
	clock.Advance(5 * time.Second)
	assertStrategyActions(t, engine.Tick(), Action{Kind: ActionComfortPrompt, TextKey: "deep_listening.silence"})
}

func TestStrategySessionResetRestoresProactivePrompts(t *testing.T) {
	clock := newStrategyFakeClock()
	engine := NewEngine(ModeComfort, strategyTiming(), clock)
	clock.Advance(12 * time.Second)
	assertStrategyActions(t, engine.Tick(), Action{Kind: ActionComfortPrompt, TextKey: "comfort.first_silence"})
	engine.Apply(Signal{Kind: SignalAssistantStarted})
	engine.Apply(Signal{Kind: SignalAssistantStopped})
	assertStrategyActions(t, engine.Tick(), Action{Kind: ActionComfortPrompt, TextKey: "comfort.second_silence"})
	engine.Apply(Signal{Kind: SignalSessionReset})
	clock.Advance(5 * time.Second)
	assertStrategyActions(t, engine.Tick(), Action{Kind: ActionComfortPrompt, TextKey: "comfort.first_silence"})
}

func TestStrategyNewEngineAppliesZeroTimingDefaults(t *testing.T) {
	clock := newStrategyFakeClock()
	engine := NewEngine(ModeNormal, TimingConfig{}, clock)
	engine.Apply(Signal{Kind: SignalStableText, Transcript: "默认阈值"})
	engine.Apply(Signal{Kind: SignalSilence})
	clock.Advance(699 * time.Millisecond)
	assertStrategyActions(t, engine.Tick())
	clock.Advance(time.Millisecond)
	assertStrategyActions(t, engine.Tick(), Action{Kind: ActionEndpoint})
}

func TestStrategyNewEngineUsesValidCustomTiming(t *testing.T) {
	clock := newStrategyFakeClock()
	timing := strategyTiming()
	timing.NormalEndSilenceMs = 900
	engine := NewEngine(ModeNormal, timing, clock)
	engine.Apply(Signal{Kind: SignalStableText, Transcript: "自定义阈值"})
	engine.Apply(Signal{Kind: SignalSilence})
	clock.Advance(899 * time.Millisecond)
	assertStrategyActions(t, engine.Tick())
	clock.Advance(time.Millisecond)
	assertStrategyActions(t, engine.Tick(), Action{Kind: ActionEndpoint})
}

func TestStrategyNewEngineRejectsInvalidDependenciesAndConfiguration(t *testing.T) {
	tests := []struct {
		name   string
		mode   Mode
		timing TimingConfig
		clock  Clock
	}{
		{name: "nil clock", mode: ModeNormal, timing: strategyTiming()},
		{name: "unknown mode", mode: Mode("mystery"), timing: strategyTiming(), clock: newStrategyFakeClock()},
		{name: "invalid timing", mode: ModeNormal, timing: func() TimingConfig {
			v := strategyTiming()
			v.NormalEndSilenceMs = 100
			return v
		}(), clock: newStrategyFakeClock()},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			defer func() {
				if recover() == nil {
					t.Fatal("NewEngine did not panic")
				}
			}()
			NewEngine(tt.mode, tt.timing, tt.clock)
		})
	}
}
