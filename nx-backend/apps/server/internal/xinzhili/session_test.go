package xinzhili

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/json"
	"errors"
	"fmt"
	"log"
	"slices"
	"strings"
	"sync"
	"testing"
	"time"

	"nine-xing/nx-backend/apps/server/internal/rag"
	"nine-xing/nx-backend/apps/server/internal/voice"
)

func TestModelIdentityRealtimeBypassesGeneratorAndCompletesVoiceDelivery(t *testing.T) {
	for _, configured := range []bool{false, true} {
		configured := configured
		name := "without_generator"
		if configured {
			name = "with_generator"
		}
		t.Run(name, func(t *testing.T) {
			fixture := newSessionFixture(t)
			fixture.synth.segments = nil
			if !configured {
				fixture.session.Close()
				fixture.deps.Generator = nil
				fixture.session = NewSession(fixture.deps)
			}

			const question = "你是什么模型？"
			if err := fixture.session.StartTurn(context.Background(), fixture.input("turn-model-identity")); err != nil {
				t.Fatal(err)
			}
			fixture.asr.emit(ASREvent{Kind: ASREventFinal, Final: question, Stable: true})

			segment := fixture.sink.waitAudio(t)
			if segment.DeliveryText() != rag.ModelIdentityReply {
				t.Fatalf("TTS delivery text=%q", segment.DeliveryText())
			}
			fixture.store.waitAssistantCount(t, 1)
			if err := fixture.session.HandlePlaybackAck(context.Background(), PlaybackAck{TurnID: "turn-model-identity", SegmentSeq: segment.Seq}); err != nil {
				t.Fatal(err)
			}
			fixture.store.waitCompleted(t)
			fixture.store.waitDelivered(t, rag.ModelIdentityReply)
			fixture.store.waitCompleteAck(t)

			users, assistants, completed := fixture.store.contents()
			if len(users) != 1 || users[0] != question {
				t.Fatalf("hidden user persistence=%q", users)
			}
			if len(assistants) != 1 || assistants[0] != rag.ModelIdentityReply || completed != rag.ModelIdentityReply {
				t.Fatalf("assistant drafts=%q completed=%q", assistants, completed)
			}
			if got := fixture.synth.synthesizedTexts(); len(got) != 1 || got[0] != rag.ModelIdentityReply {
				t.Fatalf("synthesized texts=%q", got)
			}
			if fixture.generator.calls() != 0 {
				t.Fatalf("generator calls=%d", fixture.generator.calls())
			}
			fixture.sink.assertNoErrorControl(t)
		})
	}
}

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

func TestStartTurnRejectsZeroTurnKey(t *testing.T) {
	fixture := newSessionFixture(t)
	input := fixture.input("turn-zero-key")
	input.TurnKey = 0
	if err := fixture.session.StartTurn(context.Background(), input); err == nil {
		t.Fatal("StartTurn accepted zero turn key")
	}
}

func TestStreamSentenceChunkerPrioritizesPlayableFirstChunk(t *testing.T) {
	t.Run("short weak punctuation waits", func(t *testing.T) {
		var chunker streamSentenceChunker
		if got := chunker.Push("你好，"); len(got) != 0 {
			t.Fatalf("chunks=%q", got)
		}
		if got := chunker.Flush(); !slices.Equal(got, []string{"你好，"}) {
			t.Fatalf("flush=%q", got)
		}
	})

	t.Run("tiny strong first sentence waits for a playable phrase", func(t *testing.T) {
		var chunker streamSentenceChunker
		if got := chunker.Push("你好。"); len(got) != 0 {
			t.Fatalf("tiny strong sentence emitted choppy chunk=%q", got)
		}
		next := "我还在认真听你说完，这样第一段声音不会太碎一点点，"
		want := "你好。" + next
		if got := chunker.Push(next); !slices.Equal(got, []string{want}) {
			t.Fatalf("chunks=%q want %q", got, []string{want})
		}
	})

	t.Run("weak punctuation waits for longer phrase to avoid choppy audio clips", func(t *testing.T) {
		var shortChunker streamSentenceChunker
		tooShort := strings.Repeat("甲", firstTTSChunkMinRunes-2) + "，"
		if got := shortChunker.Push(tooShort + "后续"); len(got) != 0 {
			t.Fatalf("tiny chunk emitted=%q", got)
		}

		var chunker streamSentenceChunker
		prefix := strings.Repeat("甲", firstTTSChunkMinRunes-1) + "，"
		if got := chunker.Push(prefix + "后续"); !slices.Equal(got, []string{prefix}) {
			t.Fatalf("chunks=%q", got)
		}
		if got := chunker.Flush(); !slices.Equal(got, []string{"后续"}) {
			t.Fatalf("flush=%q", got)
		}
	})

	t.Run("fourteen rune comma stays buffered because it sounds choppy as standalone audio", func(t *testing.T) {
		var chunker streamSentenceChunker
		earlyComma := strings.Repeat("甲", 13) + "，"
		if got := chunker.Push(earlyComma + "后续"); len(got) != 0 {
			t.Fatalf("early comma emitted choppy chunk=%q", got)
		}
	})

	t.Run("first chunk hard cap still avoids very short playback fragments", func(t *testing.T) {
		var chunker streamSentenceChunker
		first := strings.Repeat("甲", firstTTSChunkMaxRunes)
		if got := chunker.Push(first + "后续"); !slices.Equal(got, []string{first}) {
			t.Fatalf("chunks=%q", got)
		}
	})

	t.Run("later chunks keep longer cap to reduce playback gaps", func(t *testing.T) {
		var chunker streamSentenceChunker
		first := strings.Repeat("甲", firstTTSChunkMinRunes) + "。"
		if got := chunker.Push(first); !slices.Equal(got, []string{first}) {
			t.Fatalf("first=%q", got)
		}
		if got := chunker.Push(strings.Repeat("乙", streamTTSChunkMaxRunes-1)); len(got) != 0 {
			t.Fatalf("early later chunk=%q", got)
		}
		later := strings.Repeat("乙", streamTTSChunkMaxRunes)
		if got := chunker.Push("乙"); !slices.Equal(got, []string{later}) {
			t.Fatalf("later=%q", got)
		}
	})

	t.Run("later tiny strong sentence also waits for a playable phrase", func(t *testing.T) {
		var chunker streamSentenceChunker
		first := strings.Repeat("甲", firstTTSChunkMinRunes) + "。"
		if got := chunker.Push(first); !slices.Equal(got, []string{first}) {
			t.Fatalf("first=%q", got)
		}
		if got := chunker.Push("好。"); len(got) != 0 {
			t.Fatalf("later tiny strong emitted choppy chunk=%q", got)
		}
		next := "后面继续补足一段自然长度，让播放不要一顿一顿，保持连贯，"
		want := "好。" + next
		if got := chunker.Push(next); !slices.Equal(got, []string{want}) {
			t.Fatalf("later=%q want %q", got, []string{want})
		}
	})

	t.Run("hard limit does not split an English token", func(t *testing.T) {
		var chunker streamSentenceChunker
		prefix := strings.Repeat("甲", firstTTSChunkMaxRunes-10)
		if got := chunker.Push(prefix + " emotional-support 后续"); !slices.Equal(got, []string{prefix}) {
			t.Fatalf("chunks=%q want prefix boundary", got)
		}
	})
}

func TestSessionGenerationNormalizesRealtimeChineseBeforeTTSAndPersistence(t *testing.T) {
	fixture := newSessionFixture(t)
	raw := "OK，我陪你做 1 2 3 次呼吸，不用 worry。"
	want := voice.NormalizeStrictChineseTTSInput(raw)
	fixture.generator.deltas = []string{"OK，我陪你做 1 2 ", "3 次呼吸，不用 worry。"}
	fixture.generator.answer = raw
	fixture.knowledge.docs = []rag.Document{{ID: "knowledge:strict-chinese", Title: "来源", Content: "中文语音输出"}}
	fixture.synth.segments = nil

	if err := fixture.session.StartTurn(context.Background(), fixture.input("turn-strict-realtime")); err != nil {
		t.Fatal(err)
	}
	fixture.asr.emit(ASREvent{Kind: ASREventFinal, Final: "我们先做 1 2 3 次呼吸", Stable: true})
	segment := fixture.sink.waitAudio(t)
	if segment.DeliveryText() != want {
		t.Fatalf("delivery text=%q want %q", segment.DeliveryText(), want)
	}
	assertNoASCIIText(t, segment.DeliveryText())
	if got := fixture.synth.synthesizedTexts(); !slices.Equal(got, []string{want}) {
		t.Fatalf("synthesized=%q want %q", got, []string{want})
	}
	if err := fixture.session.HandlePlaybackAck(context.Background(), PlaybackAck{TurnID: "turn-strict-realtime", SegmentSeq: segment.Seq}); err != nil {
		t.Fatal(err)
	}
	fixture.store.waitCompleted(t)
	fixture.store.waitDelivered(t, want)
	fixture.store.waitCompleteAck(t)
	_, assistants, completed := fixture.store.contents()
	if len(assistants) != 1 || assistants[0] != want || completed != want {
		t.Fatalf("assistant=%q completed=%q want %q", assistants, completed, want)
	}
}

func TestSpeechStartedEmitsASRActivity(t *testing.T) {
	fixture := newSessionFixture(t)
	if err := fixture.session.StartTurn(context.Background(), fixture.input("turn-asr-activity")); err != nil {
		t.Fatal(err)
	}
	fixture.asr.emit(ASREvent{Kind: ASREventSpeechStarted})
	event := fixture.sink.waitControl(t, EventASRActivity)
	if event.TurnID == nil || *event.TurnID != "turn-asr-activity" {
		t.Fatalf("asr.activity turnId = %v", event.TurnID)
	}
}

func TestSpeechStartedDoesNotRegressProcessingState(t *testing.T) {
	sink := newFakeSessionSink()
	s := &session{deps: SessionDependencies{Sink: sink}}
	turn := &activeTurn{
		input: StartTurnInput{TurnID: "turn-already-processing"},
		ctx:   context.Background(), processing: true,
	}
	s.handleASREvent(turn, ASREvent{Kind: ASREventSpeechStarted})
	sink.assertNoControl(t)
}

func TestStableTranscriptEmitsTurnProcessing(t *testing.T) {
	fixture := newSessionFixture(t)
	if err := fixture.session.StartTurn(context.Background(), fixture.input("turn-processing")); err != nil {
		t.Fatal(err)
	}
	fixture.asr.emit(ASREvent{Kind: ASREventFinal, Final: "我想聊一聊", Stable: true})
	event := fixture.sink.waitControl(t, EventTurnProcessing)
	if event.TurnID == nil || *event.TurnID != "turn-processing" {
		t.Fatalf("turn.processing turnId = %v", event.TurnID)
	}
}

func TestSessionStrategyRoutesASRSignalsAndTicksBeforeEndpointing(t *testing.T) {
	fixture := newSessionFixture(t)
	engine := newScriptedStrategyEngine()
	fixture.session.Close()
	fixture.deps.EngineFactory = func(Mode, TimingConfig, Clock) StrategyEngine { return engine }
	fixture.session = NewSession(fixture.deps)

	if err := fixture.session.StartTurn(context.Background(), fixture.input("turn-strategy-tick")); err != nil {
		t.Fatal(err)
	}
	fixture.asr.emit(ASREvent{Kind: ASREventSpeechStarted})
	engine.waitApply(t, SignalSpeechStarted)
	fixture.asr.emit(ASREvent{Kind: ASREventPartial, Partial: "我想先说", Stable: true})
	engine.waitApply(t, SignalPartial)
	fixture.asr.emit(ASREvent{Kind: ASREventFinal, Final: "我想先说完整", Stable: true})
	engine.waitApply(t, SignalStableText)
	engine.waitApply(t, SignalSilence)
	if fixture.generator.calls() != 0 || fixture.store.userCount() != 0 || fixture.asr.finishCount() != 0 {
		t.Fatalf("stable final bypassed strategy: generator=%d users=%d finish=%d", fixture.generator.calls(), fixture.store.userCount(), fixture.asr.finishCount())
	}

	engine.setTickActions(Action{Kind: ActionEndpoint})
	fixture.generator.waitCalled(t)
	if fixture.asr.finishCount() != 1 {
		t.Fatalf("FinishInput calls=%d want=1", fixture.asr.finishCount())
	}
	input := fixture.generator.lastInput()
	if input.Question != "我想先说完整" {
		t.Fatalf("question=%q", input.Question)
	}
}

func TestSessionStrategyKeepsMultipleFinalSegmentsUntilEndpoint(t *testing.T) {
	fixture := newSessionFixture(t)
	engine := newScriptedStrategyEngine()
	var silences int
	engine.applyActions = func(signal Signal) []Action {
		if signal.Kind == SignalSilence {
			silences++
			if silences == 2 {
				return []Action{{Kind: ActionEndpoint}}
			}
		}
		return nil
	}
	fixture.session.Close()
	fixture.deps.EngineFactory = func(Mode, TimingConfig, Clock) StrategyEngine { return engine }
	fixture.session = NewSession(fixture.deps)

	if err := fixture.session.StartTurn(context.Background(), fixture.input("turn-multiple-finals")); err != nil {
		t.Fatal(err)
	}
	fixture.asr.emit(ASREvent{Kind: ASREventSpeechStarted})
	engine.waitApply(t, SignalSpeechStarted)
	fixture.asr.emit(ASREvent{Kind: ASREventFinal, Final: "第一段。", Stable: true})
	engine.waitApply(t, SignalStableText)
	engine.waitApply(t, SignalSilence)
	if fixture.generator.calls() != 0 {
		t.Fatal("first stable final must remain inside the configured silence window")
	}

	fixture.asr.emit(ASREvent{Kind: ASREventSpeechStarted})
	engine.waitApply(t, SignalSpeechStarted)
	fixture.asr.emit(ASREvent{Kind: ASREventFinal, Final: "第二段。", Stable: true})
	fixture.generator.waitCalled(t)
	if got := fixture.generator.lastInput().Question; got != "第一段。第二段。" {
		t.Fatalf("combined question=%q", got)
	}
	if fixture.asr.finishCount() != 1 {
		t.Fatalf("FinishInput calls=%d want=1", fixture.asr.finishCount())
	}
}

func TestSessionStrategyFinishInputFailureConvergesToErrorAndDone(t *testing.T) {
	fixture := newSessionFixture(t)
	engine := newScriptedStrategyEngine()
	engine.applyActions = func(signal Signal) []Action {
		if signal.Kind == SignalSilence {
			return []Action{{Kind: ActionEndpoint}}
		}
		return nil
	}
	fixture.asr.finishErr = errors.New("finish failed")
	fixture.session.Close()
	fixture.deps.EngineFactory = func(Mode, TimingConfig, Clock) StrategyEngine { return engine }
	fixture.session = NewSession(fixture.deps)

	if err := fixture.session.StartTurn(context.Background(), fixture.input("turn-finish-failed")); err != nil {
		t.Fatal(err)
	}
	fixture.asr.emit(ASREvent{Kind: ASREventFinal, Final: "请听我说", Stable: true})
	event := fixture.sink.waitControl(t, EventError)
	var payload ErrorPayload
	if err := json.Unmarshal(event.Payload, &payload); err != nil {
		t.Fatal(err)
	}
	if payload.Code != "asr_finish_failed" {
		t.Fatalf("error code=%q", payload.Code)
	}
	fixture.sink.waitControl(t, EventAssistantDone)
	if fixture.generator.calls() != 0 || fixture.store.userCount() != 0 {
		t.Fatalf("failed FinishInput generated=%d users=%d", fixture.generator.calls(), fixture.store.userCount())
	}
}

func TestSessionStrategyDrainsTailFinalBeforeProcessing(t *testing.T) {
	fixture := newSessionFixture(t)
	engine := newScriptedStrategyEngine()
	engine.applyActions = func(signal Signal) []Action {
		if signal.Kind == SignalSilence {
			return []Action{{Kind: ActionEndpoint}}
		}
		return nil
	}
	fixture.asr.finishHook = func(asr *fakeASRSession) {
		asr.emit(ASREvent{Kind: ASREventFinal, Final: "尾部补充。", Stable: true})
		asr.emit(ASREvent{Kind: ASREventTaskFinished})
	}
	fixture.session.Close()
	fixture.deps.EngineFactory = func(Mode, TimingConfig, Clock) StrategyEngine { return engine }
	fixture.session = NewSession(fixture.deps)

	if err := fixture.session.StartTurn(context.Background(), fixture.input("turn-tail-final")); err != nil {
		t.Fatal(err)
	}
	fixture.asr.emit(ASREvent{Kind: ASREventFinal, Final: "主体内容。", Stable: true})
	fixture.generator.waitCalled(t)
	if got := fixture.generator.lastInput().Question; got != "主体内容。尾部补充。" {
		t.Fatalf("drained question=%q", got)
	}
}

func TestSessionStrategyPartialOnlyFinishesOnceAndPromotesAtTermination(t *testing.T) {
	fixture := newSessionFixture(t)
	engine := newScriptedStrategyEngine()
	engine.applyActions = func(signal Signal) []Action {
		if signal.Kind == SignalPartial {
			return []Action{{Kind: ActionEndpoint}, {Kind: ActionEndpoint}}
		}
		return nil
	}
	fixture.session.Close()
	fixture.deps.EngineFactory = func(Mode, TimingConfig, Clock) StrategyEngine { return engine }
	fixture.session = NewSession(fixture.deps)

	if err := fixture.session.StartTurn(context.Background(), fixture.input("turn-partial-only")); err != nil {
		t.Fatal(err)
	}
	fixture.asr.emit(ASREvent{Kind: ASREventPartial, Partial: "在吗在吗"})
	fixture.generator.waitCalled(t)
	if got := fixture.generator.lastInput().Question; got != "在吗在吗" {
		t.Fatalf("promoted partial=%q", got)
	}
	if fixture.asr.finishCount() != 1 {
		t.Fatalf("FinishInput calls=%d want=1", fixture.asr.finishCount())
	}
	users, _, _ := fixture.store.contents()
	if len(users) != 1 || users[0] != "在吗在吗" {
		t.Fatalf("persisted users=%q", users)
	}
}

func TestSessionStrategyPartialWaitsForFinishReturnAndTaskFinishedBarriers(t *testing.T) {
	fixture := newSessionFixture(t)
	engine := newScriptedStrategyEngine()
	engine.applyActions = func(signal Signal) []Action {
		if signal.Kind == SignalPartial {
			return []Action{{Kind: ActionEndpoint}}
		}
		return nil
	}
	finishStarted := make(chan struct{})
	releaseFinish := make(chan struct{})
	fixture.asr.finishHook = func(asr *fakeASRSession) {
		close(finishStarted)
		asr.emit(ASREvent{Kind: ASREventTaskFinished})
		<-releaseFinish
	}
	fixture.session.Close()
	fixture.deps.EngineFactory = func(Mode, TimingConfig, Clock) StrategyEngine { return engine }
	fixture.session = NewSession(fixture.deps)

	if err := fixture.session.StartTurn(context.Background(), fixture.input("turn-partial-barriers")); err != nil {
		t.Fatal(err)
	}
	fixture.asr.emit(ASREvent{Kind: ASREventPartial, Partial: "你在吗"})
	select {
	case <-finishStarted:
	case <-time.After(time.Second):
		t.Fatal("FinishInput did not start")
	}
	time.Sleep(30 * time.Millisecond)
	if fixture.generator.calls() != 0 || fixture.store.userCount() != 0 {
		t.Fatal("task-finished alone crossed the endpoint barrier")
	}
	close(releaseFinish)
	fixture.generator.waitCalled(t)
}

func TestSessionArgumentRepeatedFillerPartialDoesNotEnterEndpoint(t *testing.T) {
	fixture := newSessionFixture(t)
	fixture.session.Close()
	fixture.deps.Clock = wallSessionClock{}
	fixture.deps.EngineFactory = func(mode Mode, timing TimingConfig, clock Clock) StrategyEngine {
		return NewEngine(mode, timing, clock)
	}
	fixture.session = NewSession(fixture.deps)

	input := fixture.input("turn-repeated-partial")
	input.Mode = ModeArgument
	input.Timing = strategyTiming()
	if err := fixture.session.StartTurn(context.Background(), input); err != nil {
		t.Fatal(err)
	}
	fixture.asr.emit(ASREvent{Kind: ASREventPartial, Partial: "嗯嗯嗯"})
	time.Sleep(1800 * time.Millisecond)
	if fixture.asr.finishCount() != 0 {
		t.Fatalf("FinishInput calls=%d want=0", fixture.asr.finishCount())
	}
	fixture.sink.assertNoControl(t)
	select {
	case segment := <-fixture.sink.audio:
		t.Fatalf("unexpected prompt audio=%q", segment.DeliveryText())
	default:
	}
}

func TestSessionArgumentQualifiedPartialEntersEndpointOnce(t *testing.T) {
	fixture := newSessionFixture(t)
	fixture.session.Close()
	fixture.deps.Clock = wallSessionClock{}
	fixture.deps.EngineFactory = func(mode Mode, timing TimingConfig, clock Clock) StrategyEngine {
		return NewEngine(mode, timing, clock)
	}
	fixture.session = NewSession(fixture.deps)

	input := fixture.input("turn-qualified-partial")
	input.Mode = ModeArgument
	input.Timing = strategyTiming()
	if err := fixture.session.StartTurn(context.Background(), input); err != nil {
		t.Fatal(err)
	}
	fixture.asr.emit(ASREvent{Kind: ASREventPartial, Partial: "你在吗"})
	time.Sleep(1400 * time.Millisecond)
	if fixture.asr.finishCount() != 0 {
		t.Fatalf("FinishInput crossed 1600ms safe window early: calls=%d", fixture.asr.finishCount())
	}
	waitASRFinishCount(t, fixture.asr, 1, 2*time.Second)
	fixture.generator.waitCalled(t)
	if fixture.asr.finishCount() != 1 {
		t.Fatalf("FinishInput calls=%d want=1", fixture.asr.finishCount())
	}
}

func TestSessionCompletedEndpointWithoutAnyTranscriptUsesDefensivePrompt(t *testing.T) {
	sink := newFakeSessionSink()
	synth := &fakeSynthesizer{}
	store := &fakeConversationStore{}
	s := &session{
		deps:   SessionDependencies{Sink: sink, Synthesizer: synth, Conversations: store},
		events: make(chan sessionEvent, 4),
		done:   make(chan struct{}),
	}
	ctx, cancel := context.WithCancel(context.Background())
	turn := &activeTurn{
		input:           StartTurnInput{TurnID: "turn-defensive-prompt", TurnKey: TurnKey("turn-defensive-prompt")},
		turnKey:         TurnKey("turn-defensive-prompt"),
		ctx:             ctx,
		cancel:          cancel,
		endpointing:     true,
		finishReturned:  true,
		asrTaskFinished: true,
		segments:        map[uint32]string{},
		lastAck:         -1,
	}
	s.completeStrategyEndpoint(turn)

	current := turn
	for i := 0; i < 2; i++ {
		select {
		case event := <-s.events:
			s.handleEvent(&current, event)
		case <-time.After(time.Second):
			t.Fatal("defensive prompt did not finish")
		}
	}
	segment := sink.waitAudio(t)
	if segment.DeliveryText() != "环境有些嘈杂，我没听清，请再说一次" {
		t.Fatalf("prompt=%q", segment.DeliveryText())
	}
	sink.waitControl(t, EventAssistantDone)
	if store.assistantCount() != 0 {
		t.Fatalf("terminal prompt assistants=%d want=0", store.assistantCount())
	}
	_, _, completed := store.contents()
	if completed != "" {
		t.Fatalf("terminal prompt completed assistant=%q", completed)
	}
}

func TestSessionStrategyStopAssistantEmitsPlaybackInterrupt(t *testing.T) {
	fixture := newSessionFixture(t)
	engine := newScriptedStrategyEngine()
	engine.applyActions = func(signal Signal) []Action {
		if signal.Kind == SignalSpeechStarted {
			return []Action{{Kind: ActionStopAssistant}}
		}
		return nil
	}
	fixture.session.Close()
	fixture.deps.EngineFactory = func(Mode, TimingConfig, Clock) StrategyEngine { return engine }
	fixture.session = NewSession(fixture.deps)

	if err := fixture.session.StartTurn(context.Background(), fixture.input("turn-stop-assistant")); err != nil {
		t.Fatal(err)
	}
	fixture.asr.emit(ASREvent{Kind: ASREventSpeechStarted})
	event := fixture.sink.waitControl(t, EventPlaybackInterrupt)
	if event.TurnID == nil || *event.TurnID != "turn-stop-assistant" {
		t.Fatalf("playback.interrupt turnId=%v", event.TurnID)
	}
}

func TestSessionStrategyCancelPendingCancelsEndpointBeforeGeneration(t *testing.T) {
	fixture := newSessionFixture(t)
	engine := newScriptedStrategyEngine()
	var endpointStarted bool
	engine.applyActions = func(signal Signal) []Action {
		switch signal.Kind {
		case SignalSilence:
			endpointStarted = true
			return []Action{{Kind: ActionEndpoint}}
		case SignalSpeechStarted:
			if endpointStarted {
				return []Action{{Kind: ActionCancelPending}}
			}
		}
		return nil
	}
	finishStarted := make(chan struct{})
	releaseFinish := make(chan struct{})
	fixture.asr.finishHook = func(asr *fakeASRSession) {
		close(finishStarted)
		<-releaseFinish
		asr.emit(ASREvent{Kind: ASREventTaskFinished})
	}
	fixture.session.Close()
	fixture.deps.EngineFactory = func(Mode, TimingConfig, Clock) StrategyEngine { return engine }
	fixture.session = NewSession(fixture.deps)

	if err := fixture.session.StartTurn(context.Background(), fixture.input("turn-cancel-pending")); err != nil {
		t.Fatal(err)
	}
	fixture.asr.emit(ASREvent{Kind: ASREventFinal, Final: "第一段", Stable: true})
	select {
	case <-finishStarted:
	case <-time.After(time.Second):
		t.Fatal("FinishInput did not start")
	}
	fixture.asr.emit(ASREvent{Kind: ASREventSpeechStarted})
	fixture.sink.waitControl(t, EventTurnCancelled)
	fixture.sink.waitControl(t, EventAssistantDone)
	close(releaseFinish)
	time.Sleep(30 * time.Millisecond)
	if fixture.generator.calls() != 0 || fixture.store.userCount() != 0 {
		t.Fatalf("cancelled endpoint generated=%d users=%d", fixture.generator.calls(), fixture.store.userCount())
	}
}

func TestSessionStrategyComfortPromptUsesExistingTTSDelivery(t *testing.T) {
	fixture := newSessionFixture(t)
	fixture.synth.segments = nil
	engine := newScriptedStrategyEngine()
	fixture.session.Close()
	fixture.deps.EngineFactory = func(Mode, TimingConfig, Clock) StrategyEngine { return engine }
	fixture.session = NewSession(fixture.deps)

	input := fixture.input("turn-comfort-prompt")
	input.Mode = ModeComfort
	if err := fixture.session.StartTurn(context.Background(), input); err != nil {
		t.Fatal(err)
	}
	engine.setTickActions(Action{Kind: ActionComfortPrompt, TextKey: "comfort.first_silence"})
	segment := fixture.sink.waitAudio(t)
	if segment.DeliveryText() != "我在这里，你可以慢慢说。" {
		t.Fatalf("comfort prompt=%q", segment.DeliveryText())
	}
	fixture.sink.waitControl(t, EventAssistantDone)
}

func TestSessionStrategyMidSentencePromptResumesSameInputTurn(t *testing.T) {
	fixture := newSessionFixture(t)
	fixture.synth.segments = nil
	engine := newScriptedStrategyEngine()
	engine.applyActions = func(signal Signal) []Action {
		if signal.Kind == SignalSilence {
			return []Action{{Kind: ActionEndpoint}}
		}
		return nil
	}
	fixture.session.Close()
	fixture.deps.EngineFactory = func(Mode, TimingConfig, Clock) StrategyEngine { return engine }
	fixture.session = NewSession(fixture.deps)

	input := fixture.input("turn-mid-sentence-resume")
	input.Mode = ModeComfort
	if err := fixture.session.StartTurn(context.Background(), input); err != nil {
		t.Fatal(err)
	}
	engine.setTickActions(Action{Kind: ActionComfortPrompt, TextKey: "comfort.mid_sentence"})
	fixture.sink.waitAudio(t)
	fixture.sink.waitControl(t, EventAssistantDone)

	fixture.asr.emit(ASREvent{Kind: ASREventPartial, Partial: "我还想继续", Stable: true})
	engine.waitApply(t, SignalPartial)
	fixture.asr.emit(ASREvent{Kind: ASREventFinal, Final: "我还想继续说完", Stable: true})
	fixture.generator.waitCalled(t)
	if got := fixture.generator.lastInput().Question; got != "我还想继续说完" {
		t.Fatalf("resumed question=%q", got)
	}
	fixture.sink.waitAudio(t)
	fixture.sink.waitControl(t, EventAssistantDone)
}

func TestSessionStrategyQueuesInterruptionPrefixForNextAnswer(t *testing.T) {
	fixture := newSessionFixture(t)
	fixture.synth.segments = nil
	engine := newScriptedStrategyEngine()
	engine.applyActions = func(signal Signal) []Action {
		switch signal.Kind {
		case SignalStableText:
			return []Action{{Kind: ActionQueueInterruptionPrefix, TextKey: "argument.important_interruption"}}
		case SignalSilence:
			return []Action{{Kind: ActionEndpoint}}
		default:
			return nil
		}
	}
	fixture.session.Close()
	fixture.deps.EngineFactory = func(Mode, TimingConfig, Clock) StrategyEngine { return engine }
	fixture.session = NewSession(fixture.deps)

	if err := fixture.session.StartTurn(context.Background(), fixture.input("turn-prefix")); err != nil {
		t.Fatal(err)
	}
	fixture.asr.emit(ASREvent{Kind: ASREventFinal, Final: "这点对我很重要", Stable: true})
	segment := fixture.sink.waitAudio(t)
	if segment.DeliveryText() != "我听到了，这一点很重要。" {
		t.Fatalf("interruption prefix=%q", segment.DeliveryText())
	}
}

func TestSessionStrategyActionFailureConvergesToErrorAndDone(t *testing.T) {
	fixture := newSessionFixture(t)
	engine := newScriptedStrategyEngine()
	engine.applyActions = func(signal Signal) []Action {
		if signal.Kind == SignalSpeechStarted {
			return []Action{{Kind: ActionStopAssistant}}
		}
		return nil
	}
	fixture.sink.failKind = EventPlaybackInterrupt
	fixture.session.Close()
	fixture.deps.EngineFactory = func(Mode, TimingConfig, Clock) StrategyEngine { return engine }
	fixture.session = NewSession(fixture.deps)

	if err := fixture.session.StartTurn(context.Background(), fixture.input("turn-action-failed")); err != nil {
		t.Fatal(err)
	}
	fixture.asr.emit(ASREvent{Kind: ASREventSpeechStarted})
	event := fixture.sink.waitControl(t, EventError)
	var payload ErrorPayload
	if err := json.Unmarshal(event.Payload, &payload); err != nil {
		t.Fatal(err)
	}
	if payload.Code != "strategy_action_failed" {
		t.Fatalf("error code=%q", payload.Code)
	}
	fixture.sink.waitControl(t, EventAssistantDone)
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
	if payload.TurnID == nil || *payload.TurnID != "turn-model-missing" {
		t.Fatalf("payload turnId=%v", payload.TurnID)
	}
	if event.TurnID != nil || event.TurnSeq != nil {
		t.Fatalf("session-level error carried turn fields: turnId=%v turnSeq=%v", event.TurnID, event.TurnSeq)
	}
	done := fixture.sink.waitControl(t, EventAssistantDone)
	if done.TurnID == nil || *done.TurnID != "turn-model-missing" {
		t.Fatalf("assistant.done turnId=%v", done.TurnID)
	}
	var donePayload struct {
		SegmentCount int `json:"segmentCount"`
	}
	if err := json.Unmarshal(done.Payload, &donePayload); err != nil {
		t.Fatal(err)
	}
	if donePayload.SegmentCount != 0 {
		t.Fatalf("assistant.done segmentCount=%d want=0", donePayload.SegmentCount)
	}
	sessionID, sessionSeq := "session-test", uint64(0)
	event.SessionID = &sessionID
	event.SessionSeq = &sessionSeq
	if _, err := EncodeEnvelope(event, DirectionServer, true); err != nil {
		t.Fatalf("error envelope cannot be encoded: %v", err)
	}
	if fixture.store.userCount() != 0 {
		t.Fatal("model missing must not persist a user turn")
	}
}

func TestConversationIsolationAndRelevanceUseIndependentRetrievers(t *testing.T) {
	fixture := newSessionFixture(t)
	input := fixture.input("turn-retrieval")
	input.KnowledgeTopK, input.KnowledgeMinScore = 3, 0.21
	input.TheoryTopK, input.TheoryMinScore = 7, 0.63
	fixture.knowledge.docs = []rag.Document{{ID: "knowledge:1", Title: "关系知识", Content: "关系冲突"}}
	fixture.theory.docs = []rag.Document{{ID: "theory:1", Title: "理论卡", Content: "非暴力沟通"}}
	if err := fixture.session.StartTurn(context.Background(), input); err != nil {
		t.Fatal(err)
	}
	fixture.asr.emit(ASREvent{Kind: ASREventFinal, Final: "和伴侣冲突怎么办", Stable: true})
	fixture.sink.waitAudio(t)

	if fixture.knowledge.calls != 1 || fixture.theory.calls != 1 {
		t.Fatalf("knowledge calls=%d theory calls=%d", fixture.knowledge.calls, fixture.theory.calls)
	}
	if fixture.knowledge.topK != 3 || fixture.knowledge.minScore != 0.21 || fixture.theory.topK != 7 || fixture.theory.minScore != 0.63 {
		t.Fatalf("knowledge=(%d,%v) theory=(%d,%v)", fixture.knowledge.topK, fixture.knowledge.minScore, fixture.theory.topK, fixture.theory.minScore)
	}
	generated := fixture.generator.lastInput()
	if len(generated.Sources) != 2 || fixture.store.resolved.Scene != SceneXinzhiliVoice || fixture.store.resolved.CardID != 22 {
		t.Fatalf("input=%+v conversation=%+v", generated, fixture.store.resolved)
	}
}

func TestRealtimeGenerationUsesCurrentCardLayeredKnowledgeAndPersistsTrace(t *testing.T) {
	fixture := newSessionFixture(t)
	fixture.session.Close()
	layered := &fakeLayeredKnowledgeRetriever{
		documents: []rag.Document{{ID: "type-6", Title: "6号型号库", Content: "先区分现实风险和想象风险"}},
		trace:     KnowledgeTrace{CardID: 22, EnneagramType: intPointer(6), CardRevision: 9, LayerHits: json.RawMessage(`{"enneagram_type":{"chunk_ids":["type-6"]}}`)},
	}
	fixture.deps.LayeredKnowledge = layered
	fixture.session = NewSession(fixture.deps)

	input := fixture.input("turn-layered-knowledge")
	if err := fixture.session.StartTurn(context.Background(), input); err != nil {
		t.Fatal(err)
	}
	fixture.asr.emit(ASREvent{Kind: ASREventFinal, Final: "担心风险时怎么办", Stable: true})
	fixture.sink.waitAudio(t)
	fixture.store.waitCompleted(t)

	if layered.calls != 1 || layered.userID != 11 || layered.conversationID != 33 || layered.cardID != 22 || layered.query != "担心风险时怎么办" {
		t.Fatalf("layered retrieval calls=%d input=%d/%d/%d %q", layered.calls, layered.userID, layered.conversationID, layered.cardID, layered.query)
	}
	if fixture.knowledge.calls != 0 || fixture.theory.calls != 0 {
		t.Fatalf("legacy retrievers called knowledge=%d theory=%d", fixture.knowledge.calls, fixture.theory.calls)
	}
	trace := fixture.store.completedKnowledgeTrace()
	if trace == nil || trace.CardID != 22 || trace.CardRevision != 9 || trace.EnneagramType == nil || *trace.EnneagramType != 6 {
		t.Fatalf("completed knowledge trace = %+v", trace)
	}
	if !strings.Contains(string(trace.LayerHits), "type-6") {
		t.Fatalf("completed layer hits = %s", trace.LayerHits)
	}
	if sources := fixture.generator.lastInput().Sources; len(sources) != 1 || sources[0].ID != "type-6" {
		t.Fatalf("layered generator sources = %+v", sources)
	}
}

func TestGenerationRetrievesKnowledgeBeforeTheory(t *testing.T) {
	fixture := newSessionFixture(t)
	fixture.session.Close()
	order := newOrderedRetrievalProbe([]rag.Document{{ID: "knowledge:ordered", Title: "知识", Content: "知识内容"}}, []rag.Document{{ID: "theory:ordered", Title: "理论", Content: "理论内容"}})
	fixture.deps.Knowledge = order.knowledge()
	fixture.deps.Theory = order.theory()
	fixture.session = NewSession(fixture.deps)

	if err := fixture.session.StartTurn(context.Background(), fixture.input("turn-ordered-retrieval")); err != nil {
		t.Fatal(err)
	}
	fixture.asr.emit(ASREvent{Kind: ASREventFinal, Final: "什么是九型", Stable: true})

	order.waitKnowledgeStarted(t)
	if order.theoryStartedBeforeKnowledgeDone(150 * time.Millisecond) {
		order.releaseKnowledge()
		t.Fatal("theory retrieval started before knowledge retrieval completed")
	}
	order.releaseKnowledge()
	fixture.generator.waitCalled(t)

	input := fixture.generator.lastInput()
	if len(input.Sources) != 2 || input.Sources[0].ID != "knowledge:ordered" || input.Sources[1].ID != "theory:ordered" {
		t.Fatalf("sources=%+v", input.Sources)
	}
}

func TestGenerationLoadsContextInParallelExceptOrderedRetrieval(t *testing.T) {
	fixture := newSessionFixture(t)
	fixture.session.Close()
	barrier := newContextLoadBarrier()
	fixture.knowledge.docs = []rag.Document{{ID: "knowledge:1", Title: "知识", Content: "知识内容"}}
	fixture.theory.docs = []rag.Document{{ID: "theory:1", Title: "理论", Content: "理论内容"}}
	fixture.deps.Conversations = barrierConversationStore{ConversationStore: fixture.store, barrier: barrier}
	fixture.deps.Preferences = barrierPreferenceProvider{barrier: barrier}
	fixture.deps.Memories = barrierMemoryProvider{barrier: barrier}
	fixture.deps.Knowledge = barrierRetriever{name: "knowledge", barrier: barrier, delegate: fixture.knowledge}
	fixture.deps.Theory = barrierRetriever{name: "theory", barrier: barrier, delegate: fixture.theory}
	fixture.session = NewSession(fixture.deps)

	if err := fixture.session.StartTurn(context.Background(), fixture.input("turn-parallel-context")); err != nil {
		t.Fatal(err)
	}
	fixture.asr.emit(ASREvent{Kind: ASREventFinal, Final: "帮我分析一下", Stable: true})

	started := make(map[string]bool, 4)
	deadline := time.NewTimer(300 * time.Millisecond)
	defer deadline.Stop()
	for len(started) < 4 {
		select {
		case name := <-barrier.started:
			started[name] = true
		case <-deadline.C:
			close(barrier.release)
			t.Fatalf("context loads started=%v, want history/preferences/memories/knowledge before ordered theory", started)
		}
	}
	if started["theory"] {
		close(barrier.release)
		t.Fatalf("theory retrieval started before knowledge barrier released: %v", started)
	}
	close(barrier.release)
	fixture.generator.waitCalled(t)

	input := fixture.generator.lastInput()
	if input.ConversationSummary != "历史摘要" || !slices.Equal(input.UserPreferences, []string{"并行偏好"}) || !slices.Equal(input.UserProfile.Memories, []string{"并行记忆"}) {
		t.Fatalf("generated context=%+v", input)
	}
	if len(input.Sources) != 2 || input.Sources[0].ID != "knowledge:1" || input.Sources[1].ID != "theory:1" {
		t.Fatalf("sources=%+v", input.Sources)
	}
}

func TestDeliveryCreatesAssistantAfterFirstAudioAndAcknowledgesExactPrefixes(t *testing.T) {
	fixture := newSessionFixture(t)
	const turnKey uint64 = 42
	fixture.synth.segments = []AudioSegment{
		{Seq: 0, Audio: []byte{1}, MIME: "audio/mpeg", deliveryText: "先呼吸。"},
		{Seq: 1, Audio: []byte{2}, MIME: "audio/mpeg", deliveryText: "再感受脚底。"},
	}
	input := fixture.input("turn-delivery")
	input.TurnKey = turnKey
	if err := fixture.session.StartTurn(context.Background(), input); err != nil {
		t.Fatal(err)
	}
	fixture.asr.emit(ASREvent{Kind: ASREventFinal, Final: "我很焦虑", Stable: true})
	first := fixture.sink.waitAudio(t)
	fixture.store.waitAssistantCount(t, 1)
	if first.Seq != 0 || first.TurnKey != turnKey {
		t.Fatalf("first=%+v", first)
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
	fixture.store.waitCompleteAck(t)
}

func TestDeliveryWrapsAudioWithStartAndEndControls(t *testing.T) {
	fixture := newSessionFixture(t)
	audio := []byte("complete-mp3-segment")
	digest := sha256.Sum256(audio)
	fixture.synth.segments = []AudioSegment{{
		Seq: 0, Audio: audio, MIME: "audio/mpeg", SHA256: digest, ByteLength: len(audio), deliveryText: "先慢慢呼吸。",
	}}
	if err := fixture.session.StartTurn(context.Background(), fixture.input("turn-audio-envelope")); err != nil {
		t.Fatal(err)
	}
	fixture.asr.emit(ASREvent{Kind: ASREventFinal, Final: "我有点紧张", Stable: true})

	var start fakeSessionWireEvent
	for start.control == nil || start.control.Type != EventAssistantAudioStart {
		start = <-fixture.sink.wire
		if start.audio != nil {
			t.Fatalf("audio arrived before assistant.audio_start: %+v", start)
		}
	}
	var metadata struct {
		SegmentSeq uint32 `json:"segmentSeq"`
		MIMEType   string `json:"mimeType"`
		ByteLength int    `json:"byteLength"`
		SHA256     string `json:"sha256"`
	}
	if err := json.Unmarshal(start.control.Payload, &metadata); err != nil {
		t.Fatal(err)
	}
	if metadata.SegmentSeq != 0 || metadata.MIMEType != "audio/mpeg" || metadata.ByteLength != len(audio) || metadata.SHA256 != fmt.Sprintf("%x", digest) {
		t.Fatalf("audio metadata = %+v", metadata)
	}

	binary := <-fixture.sink.wire
	if binary.audio == nil || binary.audio.Seq != 0 || string(binary.audio.Audio) != string(audio) {
		t.Fatalf("second wire event = %+v, want MP3 segment", binary)
	}

	end := <-fixture.sink.wire
	if end.control == nil || end.control.Type != EventAssistantAudioEnd {
		t.Fatalf("third wire event = %+v, want assistant.audio_end", end)
	}
	var ended struct {
		SegmentSeq uint32 `json:"segmentSeq"`
	}
	if err := json.Unmarshal(end.control.Payload, &ended); err != nil {
		t.Fatal(err)
	}
	if ended.SegmentSeq != 0 {
		t.Fatalf("audio end segmentSeq = %d", ended.SegmentSeq)
	}
}

func TestDeliverySignalsAssistantDoneAfterFinalAudioSegment(t *testing.T) {
	fixture := newSessionFixture(t)
	if err := fixture.session.StartTurn(context.Background(), fixture.input("turn-assistant-done")); err != nil {
		t.Fatal(err)
	}
	fixture.asr.emit(ASREvent{Kind: ASREventFinal, Final: "请回答我", Stable: true})
	fixture.sink.waitAudio(t)
	done := fixture.sink.waitControl(t, EventAssistantDone)
	if done.TurnID == nil || *done.TurnID != "turn-assistant-done" {
		t.Fatalf("assistant.done turnId = %v", done.TurnID)
	}
}

func TestDeliveryOrdersMultipleAudioSegmentsBeforeAssistantDone(t *testing.T) {
	fixture := newSessionFixture(t)
	fixture.synth.segments = []AudioSegment{
		{Seq: 0, Audio: []byte{1}, MIME: "audio/mpeg", deliveryText: "先呼吸。"},
		{Seq: 1, Audio: []byte{2}, MIME: "audio/mpeg", deliveryText: "再感受脚底。"},
	}
	if err := fixture.session.StartTurn(context.Background(), fixture.input("turn-multi-segment")); err != nil {
		t.Fatal(err)
	}
	fixture.asr.emit(ASREvent{Kind: ASREventFinal, Final: "请分两段回答", Stable: true})

	got := make([]string, 0, 7)
	turnSeqs := make([]uint64, 0, 5)
	deadline := time.After(time.Second)
	for len(got) < 7 {
		select {
		case event := <-fixture.sink.wire:
			if event.audio != nil {
				got = append(got, fmt.Sprintf("binary:%d", event.audio.Seq))
				continue
			}
			if event.control == nil {
				continue
			}
			var payload struct {
				SegmentSeq   uint32 `json:"segmentSeq"`
				SegmentCount int    `json:"segmentCount"`
			}
			_ = json.Unmarshal(event.control.Payload, &payload)
			switch event.control.Type {
			case EventAssistantAudioStart:
				got = append(got, fmt.Sprintf("start:%d", payload.SegmentSeq))
			case EventAssistantAudioEnd:
				got = append(got, fmt.Sprintf("end:%d", payload.SegmentSeq))
			case EventAssistantDone:
				got = append(got, fmt.Sprintf("done:%d", payload.SegmentCount))
			default:
				continue
			}
			if event.control.TurnSeq == nil {
				t.Fatalf("%s missing turnSeq", event.control.Type)
			}
			turnSeqs = append(turnSeqs, *event.control.TurnSeq)
		case <-deadline:
			t.Fatalf("wire order incomplete: %v", got)
		}
	}
	if want := []string{"start:0", "binary:0", "end:0", "start:1", "binary:1", "end:1", "done:2"}; !slices.Equal(got, want) {
		t.Fatalf("wire order=%v want=%v", got, want)
	}
	if want := []uint64{1, 2, 3, 4, 5}; !slices.Equal(turnSeqs, want) {
		t.Fatalf("turnSeqs=%v want=%v", turnSeqs, want)
	}
}

func TestSessionTTSSynthesizesChunksSequentially(t *testing.T) {
	synth := newControlledSynthesizer("第一段", "第二段", "第三段")
	s := &session{
		deps:   SessionDependencies{Synthesizer: synth},
		events: make(chan sessionEvent, 16),
		done:   make(chan struct{}),
	}
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	turn := &activeTurn{
		input:   StartTurnInput{TurnID: "turn-tts-concurrent"},
		ctx:     ctx,
		cancel:  cancel,
		ttsJobs: make(chan ttsStreamJob, 3),
	}

	s.startSynthesisWorker(turn)
	turn.ttsJobs <- ttsStreamJob{seq: 0, text: "第一段"}
	turn.ttsJobs <- ttsStreamJob{seq: 1, text: "第二段"}
	turn.ttsJobs <- ttsStreamJob{seq: 2, text: "第三段"}
	close(turn.ttsJobs)

	started := map[string]bool{}
	select {
	case text := <-synth.started:
		started[text] = true
	case <-time.After(time.Second):
		t.Fatal("first chunk did not start")
	}
	if !started["第一段"] {
		t.Fatalf("initial starts=%v want first chunk only", started)
	}
	select {
	case text := <-synth.started:
		t.Fatalf("chunk %q dispatched before first chunk completed", text)
	case <-time.After(30 * time.Millisecond):
	}

	synth.release("第一段")
	for wantSeq := uint32(0); wantSeq < 3; wantSeq++ {
		if wantSeq > 0 {
			select {
			case text := <-synth.started:
				wantText := []string{"第一段", "第二段", "第三段"}[wantSeq]
				if text != wantText {
					t.Fatalf("start=%q want=%q", text, wantText)
				}
			case <-time.After(time.Second):
				t.Fatalf("chunk %d was not dispatched", wantSeq)
			}
			synth.release([]string{"第一段", "第二段", "第三段"}[wantSeq])
		}
		event := waitSessionEvent(t, s.events, eventTTSSegment)
		if event.segment.Seq != wantSeq {
			t.Fatalf("segment seq=%d want=%d", event.segment.Seq, wantSeq)
		}
		event.segmentAck <- nil
	}
	done := waitSessionEvent(t, s.events, eventTTSDone)
	if done.err != nil {
		t.Fatal(done.err)
	}
	if synth.maximumConcurrency() != 1 {
		t.Fatalf("maximum concurrency=%d want=1", synth.maximumConcurrency())
	}
}

func TestQueueTTSChunkKeepsPlayableRealtimeChunkTogether(t *testing.T) {
	s := &session{}
	turn := &activeTurn{ttsJobs: make(chan ttsStreamJob, 4)}

	s.queueTTSChunk(turn, "好。后面继续补足一段自然长度。")
	close(turn.ttsJobs)

	var got []ttsStreamJob
	for job := range turn.ttsJobs {
		got = append(got, job)
	}
	want := []ttsStreamJob{{seq: 0, text: "好。后面继续补足一段自然长度。"}}
	if !slices.Equal(got, want) {
		t.Fatalf("jobs=%+v want=%+v", got, want)
	}
}

func TestSessionTTSRejectsProviderSentenceSplitsInsideOneRealtimeChunk(t *testing.T) {
	synth := splittingSynthesizer{}
	s := &session{
		deps:   SessionDependencies{Synthesizer: synth},
		events: make(chan sessionEvent, 16),
		done:   make(chan struct{}),
	}
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	turn := &activeTurn{
		input:            StartTurnInput{TurnID: "turn-split-provider"},
		ctx:              ctx,
		cancel:           cancel,
		responseStartSeq: 7,
		ttsJobs:          make(chan ttsStreamJob, 1),
	}

	s.startSynthesisWorker(turn)
	turn.ttsJobs <- ttsStreamJob{seq: 7, text: "好。后面继续补足一段自然长度。"}
	close(turn.ttsJobs)

	done := waitSessionEvent(t, s.events, eventTTSDone)
	if done.err == nil || !strings.Contains(done.err.Error(), "multiple MP3 segments") {
		t.Fatalf("done err=%v, want multiple MP3 segment guard", done.err)
	}
}

func TestSessionTTSFailureDrainsQueuedGenerationAndStillCompletes(t *testing.T) {
	fixture := newSessionFixture(t)
	deltas := make([]string, 140)
	for index := range deltas {
		deltas[index] = fmt.Sprintf("第%d段。", index)
	}
	fixture.generator.deltas = deltas
	fixture.generator.answer = strings.Join(deltas, "")
	fixture.synth.failAt = 1

	if err := fixture.session.StartTurn(context.Background(), fixture.input("turn-tts-drain")); err != nil {
		t.Fatal(err)
	}
	fixture.asr.emit(ASREvent{Kind: ASREventFinal, Final: "请生成很多段", Stable: true})

	errorEvent := fixture.sink.waitControl(t, EventError)
	var errorPayload ErrorPayload
	if err := json.Unmarshal(errorEvent.Payload, &errorPayload); err != nil {
		t.Fatal(err)
	}
	if errorPayload.Code != "tts_failed" {
		t.Fatalf("error code=%q", errorPayload.Code)
	}
	done := fixture.sink.waitControl(t, EventAssistantDone)
	if done.TurnID == nil || *done.TurnID != "turn-tts-drain" {
		t.Fatalf("assistant.done turnId=%v", done.TurnID)
	}
	time.Sleep(30 * time.Millisecond)
	for {
		select {
		case event := <-fixture.sink.controls:
			if event.Type == EventAssistantDone {
				t.Fatal("assistant.done emitted more than once")
			}
		default:
			return
		}
	}
}

func TestSessionLogsTTSFailureWithoutUserContent(t *testing.T) {
	originalWriter, originalFlags := log.Writer(), log.Flags()
	var logs bytes.Buffer
	log.SetOutput(&logs)
	log.SetFlags(0)
	t.Cleanup(func() {
		log.SetOutput(originalWriter)
		log.SetFlags(originalFlags)
	})

	fixture := newSessionFixture(t)
	fixture.generator.deltas = []string{"私密回答第一段。"}
	fixture.generator.answer = "私密回答第一段。"
	fixture.synth.failText = "私密回答第一段。"

	if err := fixture.session.StartTurn(context.Background(), fixture.input("turn-tts-log")); err != nil {
		t.Fatal(err)
	}
	fixture.asr.emit(ASREvent{Kind: ASREventFinal, Final: "用户私密问题", Stable: true})

	_ = fixture.sink.waitControl(t, EventError)
	got := logs.String()
	for _, required := range []string{"xinzhili tts failed", "user_id=11", "turn_id=\"turn-tts-log\"", "segment_count=0", "audio_bytes=0", "err=tts failed"} {
		if !strings.Contains(got, required) {
			t.Fatalf("log missing %q: %s", required, got)
		}
	}
	for _, secret := range []string{"用户私密问题", "私密回答第一段"} {
		if strings.Contains(got, secret) {
			t.Fatalf("log leaked %q: %s", secret, got)
		}
	}
}

func TestDeliveryCompletesAuditWhenLaterTTSChunkFails(t *testing.T) {
	fixture := newSessionFixture(t)
	fixture.generator.answer = "第一段。第二段。"
	fixture.knowledge.docs = []rag.Document{{ID: "knowledge:audit", Title: "审计来源", Content: "支持回答的内容"}}
	fixture.synth.failText = "第二段。"
	if err := fixture.session.StartTurn(context.Background(), fixture.input("turn-tts-fail")); err != nil {
		t.Fatal(err)
	}
	fixture.asr.emit(ASREvent{Kind: ASREventFinal, Final: "请分两段回答", Stable: true})
	first := fixture.sink.waitAudio(t)
	if err := fixture.session.HandlePlaybackAck(context.Background(), PlaybackAck{TurnID: "turn-tts-fail", SegmentSeq: first.Seq}); err != nil {
		t.Fatal(err)
	}
	errorEvent := fixture.sink.waitControl(t, EventError)
	var errorPayload ErrorPayload
	if err := json.Unmarshal(errorEvent.Payload, &errorPayload); err != nil {
		t.Fatal(err)
	}
	if errorPayload.TurnID == nil || *errorPayload.TurnID != "turn-tts-fail" {
		t.Fatalf("tts error payload turnId=%v", errorPayload.TurnID)
	}
	done := fixture.sink.waitControl(t, EventAssistantDone)
	if done.TurnID == nil || *done.TurnID != "turn-tts-fail" {
		t.Fatalf("assistant.done turnId=%v", done.TurnID)
	}
	var donePayload struct {
		SegmentCount int `json:"segmentCount"`
	}
	if err := json.Unmarshal(done.Payload, &donePayload); err != nil {
		t.Fatal(err)
	}
	if donePayload.SegmentCount != 1 {
		t.Fatalf("assistant.done segmentCount=%d want=1", donePayload.SegmentCount)
	}
	fixture.store.waitCompleted(t)
	fixture.store.mu.Lock()
	content, delivered := fixture.store.completedContent, fixture.store.delivered[len(fixture.store.delivered)-1]
	sources := append(json.RawMessage(nil), fixture.store.sources...)
	fixture.store.mu.Unlock()
	if content != "第一段。第二段。" || delivered != "第一段。" || !strings.Contains(string(sources), "knowledge:audit") {
		t.Fatalf("content=%q delivered=%q sources=%s", content, delivered, sources)
	}
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
	played := "我陪你慢慢呼吸，把注意力放回当下，先一起稳定下来，不着急。"
	fixture.generator.deltas = []string{played, "这段没有播放"}
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
	if first.DeliveryText() != played {
		t.Fatalf("first delivery text=%q", first.DeliveryText())
	}
	if err := fixture.session.HandlePlaybackAck(context.Background(), PlaybackAck{TurnID: "turn-partial-fail", SegmentSeq: first.Seq}); err != nil {
		t.Fatal(err)
	}
	close(fixture.generator.resume)
	fixture.sink.waitControl(t, EventError)
	fixture.store.waitDelivered(t, played)
	fixture.store.mu.Lock()
	assistantDraft := fixture.store.assistants[0]
	fixture.store.mu.Unlock()
	if assistantDraft != played {
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

func TestGenerationProviderFailureReportsSpecificError(t *testing.T) {
	fixture := newSessionFixture(t)
	fixture.generator.answer = ""
	fixture.generator.err = errors.New("upstream returned html")
	if err := fixture.session.StartTurn(context.Background(), fixture.input("turn-provider-fail")); err != nil {
		t.Fatal(err)
	}
	fixture.asr.emit(ASREvent{Kind: ASREventFinal, Final: "你在吗", Stable: true})

	event := fixture.sink.waitControl(t, EventError)
	var payload ErrorPayload
	if err := json.Unmarshal(event.Payload, &payload); err != nil {
		t.Fatal(err)
	}
	if payload.Code != "provider_generation_failed" {
		t.Fatalf("error code=%q", payload.Code)
	}
	if payload.Message != "会话模型连接异常，请稍后重试" {
		t.Fatalf("error message=%q", payload.Message)
	}
}

func TestGenerationEmptyAnswerReportsSpecificError(t *testing.T) {
	fixture := newSessionFixture(t)
	fixture.generator.answer = ""
	if err := fixture.session.StartTurn(context.Background(), fixture.input("turn-empty-answer")); err != nil {
		t.Fatal(err)
	}
	fixture.asr.emit(ASREvent{Kind: ASREventFinal, Final: "你在吗", Stable: true})

	event := fixture.sink.waitControl(t, EventError)
	var payload ErrorPayload
	if err := json.Unmarshal(event.Payload, &payload); err != nil {
		t.Fatal(err)
	}
	if payload.Code != "empty_generation" {
		t.Fatalf("error code=%q", payload.Code)
	}
	if payload.Message != "会话模型没有返回有效回答，请重试" {
		t.Fatalf("error message=%q", payload.Message)
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
			return immediateEndpointStrategyEngine{}
		},
		Sink: sink, Clock: fixedSessionClock{now: time.Unix(100, 0)},
	}
	fixture := &sessionFixture{deps: deps, store: store, generator: generator, knowledge: knowledge, theory: theory, synth: synth, sink: sink, factory: factory, asr: asr}
	fixture.session = NewSession(deps)
	t.Cleanup(func() { _ = fixture.session.Close() })
	return fixture
}

func (f *sessionFixture) input(turnID string) StartTurnInput {
	return StartTurnInput{UserID: 11, CardID: 22, ConversationID: 33, TurnID: turnID, TurnKey: TurnKey(turnID), Mode: ModeNormal, ASRConfig: RealtimeASRConfig{}, TTSConfig: TTSConfig{}, KnowledgeTopK: 4, KnowledgeMinScore: 0.35, TheoryTopK: 4, TheoryMinScore: 0.35}
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
	completeAcks     []bool
	sources          json.RawMessage
	completedContent string
	knowledgeTrace   *KnowledgeTrace
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
func (s *fakeConversationStore) AcknowledgeAssistant(_ context.Context, _ int64, delivered string, complete bool) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.delivered = append(s.delivered, delivered)
	s.completeAcks = append(s.completeAcks, complete)
	return nil
}
func (s *fakeConversationStore) waitCompleteAck(t *testing.T) {
	t.Helper()
	deadline := time.After(time.Second)
	for {
		s.mu.Lock()
		complete := len(s.completeAcks) > 0 && s.completeAcks[len(s.completeAcks)-1]
		s.mu.Unlock()
		if complete {
			return
		}
		select {
		case <-deadline:
			t.Fatal("final acknowledged segment was not marked complete")
		default:
			time.Sleep(time.Millisecond)
		}
	}
}
func (s *fakeConversationStore) CompleteAssistant(_ context.Context, _ int64, content string, sources json.RawMessage) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.completedContent = content
	s.sources = append(json.RawMessage(nil), sources...)
	return nil
}

func (s *fakeConversationStore) CompleteAssistantWithKnowledgeTrace(_ context.Context, _ int64, content string, sources json.RawMessage, trace KnowledgeTrace) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.completedContent = content
	s.sources = append(json.RawMessage(nil), sources...)
	copyTrace := trace
	copyTrace.LayerHits = append(json.RawMessage(nil), trace.LayerHits...)
	s.knowledgeTrace = &copyTrace
	return nil
}

func (s *fakeConversationStore) completedKnowledgeTrace() *KnowledgeTrace {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.knowledgeTrace == nil {
		return nil
	}
	copyTrace := *s.knowledgeTrace
	copyTrace.LayerHits = append(json.RawMessage(nil), s.knowledgeTrace.LayerHits...)
	return &copyTrace
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
func (s *fakeConversationStore) contents() (users, assistants []string, completed string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	return append([]string(nil), s.users...), append([]string(nil), s.assistants...), s.completedContent
}
func (s *fakeConversationStore) waitAssistantCount(t *testing.T, want int) {
	t.Helper()
	deadline := time.After(time.Second)
	for {
		if s.assistantCount() == want {
			return
		}
		select {
		case <-deadline:
			t.Fatalf("assistants=%d want=%d", s.assistantCount(), want)
		default:
			time.Sleep(time.Millisecond)
		}
	}
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

type contextLoadBarrier struct {
	started chan string
	release chan struct{}
}

func newContextLoadBarrier() *contextLoadBarrier {
	return &contextLoadBarrier{started: make(chan string, 5), release: make(chan struct{})}
}

func (b *contextLoadBarrier) wait(ctx context.Context, name string) error {
	b.started <- name
	select {
	case <-b.release:
		return nil
	case <-ctx.Done():
		return ctx.Err()
	}
}

type barrierConversationStore struct {
	ConversationStore
	barrier *contextLoadBarrier
}

func (s barrierConversationStore) History(ctx context.Context, _ Conversation, _ int) ([]rag.Message, string, error) {
	if err := s.barrier.wait(ctx, "history"); err != nil {
		return nil, "", err
	}
	return []rag.Message{{Role: "user", Content: "历史消息"}}, "历史摘要", nil
}

type barrierPreferenceProvider struct{ barrier *contextLoadBarrier }

func (p barrierPreferenceProvider) PromptPreferences(ctx context.Context, _ int64) ([]string, error) {
	if err := p.barrier.wait(ctx, "preferences"); err != nil {
		return nil, err
	}
	return []string{"并行偏好"}, nil
}

type barrierMemoryProvider struct{ barrier *contextLoadBarrier }

func (p barrierMemoryProvider) PromptMemories(ctx context.Context, _, _ int64) ([]string, error) {
	if err := p.barrier.wait(ctx, "memories"); err != nil {
		return nil, err
	}
	return []string{"并行记忆"}, nil
}

type orderedRetrievalProbe struct {
	knowledgeDocs    []rag.Document
	theoryDocs       []rag.Document
	knowledgeStarted chan struct{}
	knowledgeRelease chan struct{}
	knowledgeDone    chan struct{}
	theoryStarted    chan struct{}
}

func newOrderedRetrievalProbe(knowledgeDocs, theoryDocs []rag.Document) *orderedRetrievalProbe {
	return &orderedRetrievalProbe{
		knowledgeDocs:    knowledgeDocs,
		theoryDocs:       theoryDocs,
		knowledgeStarted: make(chan struct{}),
		knowledgeRelease: make(chan struct{}),
		knowledgeDone:    make(chan struct{}),
		theoryStarted:    make(chan struct{}),
	}
}

func (p *orderedRetrievalProbe) knowledge() KnowledgeRetriever {
	return orderedKnowledgeRetriever{probe: p}
}

func (p *orderedRetrievalProbe) theory() TheoryRetriever {
	return orderedTheoryRetriever{probe: p}
}

func (p *orderedRetrievalProbe) waitKnowledgeStarted(t *testing.T) {
	t.Helper()
	select {
	case <-p.knowledgeStarted:
	case <-time.After(time.Second):
		t.Fatal("knowledge retrieval did not start")
	}
}

func (p *orderedRetrievalProbe) theoryStartedBeforeKnowledgeDone(timeout time.Duration) bool {
	select {
	case <-p.theoryStarted:
		select {
		case <-p.knowledgeDone:
			return false
		default:
			return true
		}
	case <-time.After(timeout):
		return false
	}
}

func (p *orderedRetrievalProbe) releaseKnowledge() {
	select {
	case <-p.knowledgeRelease:
	default:
		close(p.knowledgeRelease)
	}
}

type orderedKnowledgeRetriever struct{ probe *orderedRetrievalProbe }

func (r orderedKnowledgeRetriever) Search(ctx context.Context, _ string, _ int, _ float64) ([]rag.Document, error) {
	close(r.probe.knowledgeStarted)
	select {
	case <-r.probe.knowledgeRelease:
	case <-ctx.Done():
		return nil, ctx.Err()
	}
	close(r.probe.knowledgeDone)
	return append([]rag.Document(nil), r.probe.knowledgeDocs...), nil
}

type orderedTheoryRetriever struct{ probe *orderedRetrievalProbe }

func (r orderedTheoryRetriever) Search(ctx context.Context, _ string, _ int, _ float64) ([]rag.Document, error) {
	close(r.probe.theoryStarted)
	select {
	case <-r.probe.knowledgeDone:
	case <-ctx.Done():
		return nil, ctx.Err()
	}
	return append([]rag.Document(nil), r.probe.theoryDocs...), nil
}

type barrierRetriever struct {
	name     string
	barrier  *contextLoadBarrier
	delegate *fakeRetriever
}

func (r barrierRetriever) Search(ctx context.Context, query string, topK int, minScore float64) ([]rag.Document, error) {
	if err := r.barrier.wait(ctx, r.name); err != nil {
		return nil, err
	}
	return r.delegate.Search(ctx, query, topK, minScore)
}

type fakeRetriever struct {
	calls    int
	docs     []rag.Document
	topK     int
	minScore float64
}

type fakeLayeredKnowledgeRetriever struct {
	calls          int
	userID         int64
	conversationID int64
	cardID         int64
	query          string
	documents      []rag.Document
	trace          KnowledgeTrace
}

func (r *fakeLayeredKnowledgeRetriever) Retrieve(_ context.Context, userID, conversationID, cardID int64, query string) (LayeredKnowledgeResult, error) {
	r.calls++
	r.userID, r.conversationID, r.cardID, r.query = userID, conversationID, cardID, query
	return LayeredKnowledgeResult{Documents: append([]rag.Document(nil), r.documents...), Trace: &r.trace}, nil
}

func intPointer(value int) *int { return &value }

func (r *fakeRetriever) Search(_ context.Context, _ string, topK int, minScore float64) ([]rag.Document, error) {
	r.calls++
	r.topK, r.minScore = topK, minScore
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
	texts    []string
	segments []AudioSegment
	block    chan struct{}
	failAt   int
	failText string
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
	s.texts = append(s.texts, text)
	segments := append([]AudioSegment(nil), s.segments...)
	s.mu.Unlock()
	if (s.failAt > 0 && index == s.failAt) || (s.failText != "" && text == s.failText) {
		return errors.New("tts failed")
	}
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

type splittingSynthesizer struct{}

func (splittingSynthesizer) Synthesize(ctx context.Context, _ TTSConfig, text string, emit func(AudioSegment) error) error {
	for _, sentence := range SplitSentences(text) {
		select {
		case <-ctx.Done():
			return ctx.Err()
		default:
		}
		if err := emit(AudioSegment{Audio: []byte("mp3:" + sentence), MIME: "audio/mpeg", deliveryText: sentence}); err != nil {
			return err
		}
	}
	return nil
}

type controlledSynthesizer struct {
	mu        sync.Mutex
	active    int
	maxActive int
	gates     map[string]chan struct{}
	started   chan string
}

func newControlledSynthesizer(blocked ...string) *controlledSynthesizer {
	s := &controlledSynthesizer{gates: make(map[string]chan struct{}), started: make(chan string, len(blocked)+4)}
	for _, text := range blocked {
		s.gates[text] = make(chan struct{})
	}
	return s
}

func (s *controlledSynthesizer) Synthesize(ctx context.Context, _ TTSConfig, text string, emit func(AudioSegment) error) error {
	s.mu.Lock()
	s.active++
	if s.active > s.maxActive {
		s.maxActive = s.active
	}
	gate := s.gates[text]
	s.mu.Unlock()
	s.started <- text
	defer func() {
		s.mu.Lock()
		s.active--
		s.mu.Unlock()
	}()
	if gate != nil {
		select {
		case <-gate:
		case <-ctx.Done():
			return ctx.Err()
		}
	}
	return emit(AudioSegment{Audio: []byte(text), MIME: "audio/mpeg", deliveryText: text})
}

func (s *controlledSynthesizer) release(text string) { close(s.gates[text]) }

func (s *controlledSynthesizer) maximumConcurrency() int {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.maxActive
}

func waitSessionEvent(t *testing.T, events <-chan sessionEvent, kind sessionEventKind) sessionEvent {
	t.Helper()
	select {
	case event := <-events:
		if event.kind != kind {
			t.Fatalf("event kind=%d want=%d", event.kind, kind)
		}
		return event
	case <-time.After(time.Second):
		t.Fatalf("timed out waiting for event kind=%d", kind)
		return sessionEvent{}
	}
}

func assertNoASCIIText(t *testing.T, text string) {
	t.Helper()
	for _, r := range text {
		if r >= 'a' && r <= 'z' || r >= 'A' && r <= 'Z' || r >= '0' && r <= '9' {
			t.Fatalf("text still contains ASCII rune %q in %q", r, text)
		}
	}
}

func (s *fakeSynthesizer) synthesizedTexts() []string {
	s.mu.Lock()
	defer s.mu.Unlock()
	return append([]string(nil), s.texts...)
}

type fakeSessionSink struct {
	controls chan Envelope
	audio    chan AudioSegment
	wire     chan fakeSessionWireEvent
	failKind EventType
}

type fakeSessionWireEvent struct {
	control *Envelope
	audio   *AudioSegment
}

func newFakeSessionSink() *fakeSessionSink {
	return &fakeSessionSink{
		controls: make(chan Envelope, 16),
		audio:    make(chan AudioSegment, 16),
		wire:     make(chan fakeSessionWireEvent, 64),
	}
}
func (s *fakeSessionSink) SendControl(_ context.Context, event Envelope) error {
	if event.Type == s.failKind {
		s.failKind = ""
		return errors.New("control failed")
	}
	s.controls <- event
	copy := event
	s.wire <- fakeSessionWireEvent{control: &copy}
	return nil
}
func (s *fakeSessionSink) SendAudio(_ context.Context, segment AudioSegment) error {
	s.audio <- segment
	copy := segment
	s.wire <- fakeSessionWireEvent{audio: &copy}
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
func (s *fakeSessionSink) assertNoControl(t *testing.T) {
	t.Helper()
	select {
	case event := <-s.controls:
		t.Fatalf("unexpected control event: type=%s payload=%s", event.Type, event.Payload)
	default:
	}
}

func (s *fakeSessionSink) assertNoErrorControl(t *testing.T) {
	t.Helper()
	for {
		select {
		case event := <-s.controls:
			if event.Type == EventError {
				t.Fatalf("unexpected error control: payload=%s", event.Payload)
			}
		default:
			return
		}
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
	events      chan ASREvent
	done        chan struct{}
	mu          sync.Mutex
	err         error
	finishErr   error
	finishCalls int
	finishHook  func(*fakeASRSession)
	once        sync.Once
}

func newFakeASRSession() *fakeASRSession {
	return &fakeASRSession{events: make(chan ASREvent, 16), done: make(chan struct{})}
}
func (s *fakeASRSession) WritePCM(context.Context, []byte) error { return nil }
func (s *fakeASRSession) FinishInput(context.Context) error {
	s.mu.Lock()
	s.finishCalls++
	err, hook := s.finishErr, s.finishHook
	s.mu.Unlock()
	if hook != nil {
		hook(s)
	} else if err == nil {
		s.emit(ASREvent{Kind: ASREventTaskFinished})
	}
	return err
}
func (s *fakeASRSession) Events() <-chan ASREvent { return s.events }
func (s *fakeASRSession) Err() error              { s.mu.Lock(); defer s.mu.Unlock(); return s.err }
func (s *fakeASRSession) Close() error            { s.once.Do(func() { close(s.done) }); return nil }
func (s *fakeASRSession) emit(event ASREvent)     { s.events <- event }
func (s *fakeASRSession) fail(err error)          { s.mu.Lock(); s.err = err; s.mu.Unlock(); close(s.events) }
func (s *fakeASRSession) finishCount() int {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.finishCalls
}

func waitASRFinishCount(t *testing.T, asr *fakeASRSession, want int, timeout time.Duration) {
	t.Helper()
	deadline := time.After(timeout)
	for {
		if asr.finishCount() == want {
			return
		}
		select {
		case <-deadline:
			t.Fatalf("FinishInput calls=%d want=%d", asr.finishCount(), want)
		default:
			time.Sleep(10 * time.Millisecond)
		}
	}
}

type scriptedStrategyEngine struct {
	applyCh      chan Signal
	tickCh       chan struct{}
	mu           sync.Mutex
	tickActions  []Action
	applyActions func(Signal) []Action
}

type immediateEndpointStrategyEngine struct{}

func (immediateEndpointStrategyEngine) Apply(signal Signal) []Action {
	if signal.Kind == SignalSilence {
		return []Action{{Kind: ActionEndpoint}}
	}
	return nil
}

func (immediateEndpointStrategyEngine) Tick() []Action { return nil }

func newScriptedStrategyEngine() *scriptedStrategyEngine {
	return &scriptedStrategyEngine{applyCh: make(chan Signal, 16), tickCh: make(chan struct{}, 16)}
}

func (e *scriptedStrategyEngine) Apply(signal Signal) []Action {
	e.applyCh <- signal
	e.mu.Lock()
	defer e.mu.Unlock()
	if e.applyActions == nil {
		return nil
	}
	return append([]Action(nil), e.applyActions(signal)...)
}

func (e *scriptedStrategyEngine) Tick() []Action {
	select {
	case e.tickCh <- struct{}{}:
	default:
	}
	e.mu.Lock()
	defer e.mu.Unlock()
	actions := append([]Action(nil), e.tickActions...)
	e.tickActions = nil
	return actions
}

func (e *scriptedStrategyEngine) setTickActions(actions ...Action) {
	e.mu.Lock()
	e.tickActions = append([]Action(nil), actions...)
	e.mu.Unlock()
}

func (e *scriptedStrategyEngine) waitApply(t *testing.T, kind SignalKind) Signal {
	t.Helper()
	deadline := time.After(time.Second)
	for {
		select {
		case signal := <-e.applyCh:
			if signal.Kind == kind {
				return signal
			}
		case <-deadline:
			t.Fatalf("strategy signal %v not received", kind)
		}
	}
}

type fixedSessionClock struct{ now time.Time }

func (c fixedSessionClock) Now() time.Time { return c.now }

type wallSessionClock struct{}

func (wallSessionClock) Now() time.Time { return time.Now() }
