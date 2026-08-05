package server

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"sort"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"nine-xing/nx-backend/apps/server/internal/auth"
	"nine-xing/nx-backend/apps/server/internal/rag"
	"nine-xing/nx-backend/apps/server/internal/userpreference"
)

func TestAppChatPreferenceMutationIsStoredBeforeStreamingModelAndDone(t *testing.T) {
	var orderMu sync.Mutex
	var order []string
	preferences := newFakeAppChatPreferenceStore()
	preferences.onApply = func() {
		orderMu.Lock()
		order = append(order, "preference")
		orderMu.Unlock()
	}
	generator := &controlledAppChatStreamingGenerator{
		generateStream: func(_ context.Context, input rag.GenerateInput, emit rag.StreamEmitter) (string, error) {
			orderMu.Lock()
			order = append(order, "model")
			orderMu.Unlock()
			if strings.Join(input.UserPreferences, "|") != "回答简短，避免长篇大论" {
				t.Fatalf("new durable preference missing from current generation: %+v", input)
			}
			if strings.Join(input.CurrentDirectives, "|") != "回答简短，避免长篇大论" {
				t.Fatalf("current directive missing: %+v", input)
			}
			if err := emit("简短回答"); err != nil {
				return "", err
			}
			return "简短回答", nil
		},
	}
	chatStore := newFakeAppChatStreamStore()
	chatStore.onSave = func() {
		orderMu.Lock()
		order = append(order, "save")
		orderMu.Unlock()
	}
	s := newAppChatStreamServer(chatStore, generator)
	s.userPreferences = preferences

	body := performPreferenceStreamRequest(t, s, 7, 42, "以后回答短一点，怎么处理？", context.Background())

	orderMu.Lock()
	gotOrder := strings.Join(order, ",")
	orderMu.Unlock()
	if gotOrder != "preference,model,save" {
		t.Fatalf("operation order = %q, want preference,model,save", gotOrder)
	}
	if !strings.Contains(body, "event: done\n") {
		t.Fatalf("missing done event: %q", body)
	}
}

func TestAppChatAskPassesSavedPreferencesAndCurrentDirectives(t *testing.T) {
	preferences := newFakeAppChatPreferenceStore()
	if err := preferences.Apply(context.Background(), 7, []userpreference.Mutation{{Upsert: &userpreference.Preference{
		Category: "length", Slot: "length.detail_level", Instruction: "回答简短，避免长篇大论",
	}}}); err != nil {
		t.Fatal(err)
	}
	generator := &capturingNonStreamingAppChatGenerator{answer: "详细但精炼的回答"}
	s := newAppChatStreamServer(newFakeAppChatStreamStore(), generator)
	s.userPreferences = preferences
	s.chatLimiter = newFixedWindowRateLimiter(100, time.Minute)
	writer := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "/api/app/chat/sessions/42/ask", strings.NewReader(`{"question":"这次详细说"}`))
	req = req.WithContext(context.WithValue(req.Context(), appContextKey{}, auth.UserInfo{ID: 7}))

	s.appChatRouter(writer, req)

	if writer.Code != http.StatusOK {
		t.Fatalf("status = %d body=%s", writer.Code, writer.Body.String())
	}
	if strings.Join(generator.input.UserPreferences, "|") != "回答简短，避免长篇大论" ||
		strings.Join(generator.input.CurrentDirectives, "|") != "回答更详细" {
		t.Fatalf("non-streaming input missing preference overlay: %+v", generator.input)
	}
}

func TestAppChatAskPassesCurrentConversationCardToGenerator(t *testing.T) {
	store := newFakeAppChatStreamStore()
	store.cardID = 88
	generator := &capturingNonStreamingAppChatGenerator{answer: "结合当前人物卡回答"}
	s := newAppChatStreamServer(store, generator)
	s.db = newAppAnalyticsUnitDB(t, "overview_error")
	s.chatLimiter = newFixedWindowRateLimiter(100, time.Minute)
	s.appChatProfilesForCardOverride = func(_ context.Context, userID, cardID int64) (rag.UserProfile, rag.ConversationCard) {
		if userID != 7 || cardID != 88 {
			t.Fatalf("profile lookup userID=%d cardID=%d, want 7/88", userID, cardID)
		}
		return rag.UserProfile{Nickname: "小林", MainType: 9}, rag.ConversationCard{
			CardType: "secondary",
			Name:     "妈妈",
			Relation: "家人",
			MainType: 2,
			WingType: 1,
			Profile:  `{"primaryMotivation":"希望被需要"}`,
		}
	}

	writer := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "/api/app/chat/sessions/42/ask", strings.NewReader(`{"question":"她为什么总替我做决定？"}`))
	req = req.WithContext(context.WithValue(req.Context(), appContextKey{}, auth.UserInfo{ID: 7}))
	s.appChatRouter(writer, req)

	if writer.Code != http.StatusOK {
		t.Fatalf("status = %d body=%s", writer.Code, writer.Body.String())
	}
	card := generator.input.ConversationCard
	if card.CardType != "secondary" || card.Name != "妈妈" || card.Relation != "家人" || card.MainType != 2 || card.WingType != 1 || !strings.Contains(card.Profile, "希望被需要") {
		t.Fatalf("generator conversation card = %+v, want current secondary card", card)
	}
}

func TestAppChatAskClearsPrimaryTypeWhenCurrentConversationCardTypeIsUnavailableOrInvalid(t *testing.T) {
	for _, currentType := range []int{0, 10} {
		t.Run(fmt.Sprintf("current_type_%d", currentType), func(t *testing.T) {
			store := newFakeAppChatStreamStore()
			store.cardID = 88
			generator := &capturingNonStreamingAppChatGenerator{answer: "不应该根据主卡猜测"}
			s := newAppChatStreamServer(store, generator)
			s.db = newAppAnalyticsUnitDB(t, "overview_error")
			s.chatLimiter = newFixedWindowRateLimiter(100, time.Minute)
			s.appChatProfilesForCardOverride = func(_ context.Context, userID, cardID int64) (rag.UserProfile, rag.ConversationCard) {
				if userID != 7 || cardID != 88 {
					t.Fatalf("profile lookup userID=%d cardID=%d, want 7/88", userID, cardID)
				}
				return rag.UserProfile{Nickname: "小林", MainType: 9}, rag.ConversationCard{MainType: currentType}
			}

			writer := httptest.NewRecorder()
			req := httptest.NewRequest(http.MethodPost, "/api/app/chat/sessions/42/ask", strings.NewReader(`{"question":"她为什么总替我做决定？"}`))
			req = req.WithContext(context.WithValue(req.Context(), appContextKey{}, auth.UserInfo{ID: 7}))
			s.appChatRouter(writer, req)

			if writer.Code != http.StatusOK {
				t.Fatalf("status = %d body=%s", writer.Code, writer.Body.String())
			}
			if got := generator.input.ConversationCard.MainType; got >= 1 && got <= 9 {
				t.Fatalf("conversation card main type = %d, want no valid current type", got)
			}
			if generator.input.UserProfile.MainType != 0 {
				t.Fatalf("user profile main type = %d, want 0 when current conversation card type is unavailable", generator.input.UserProfile.MainType)
			}
		})
	}
}

func TestAppChatPreferenceWriteFailureDoesNotCallModel(t *testing.T) {
	preferences := newFakeAppChatPreferenceStore()
	preferences.applyErr = errors.New("database unavailable")
	var modelCalls atomic.Int32
	generator := &controlledAppChatStreamingGenerator{
		generateStream: func(context.Context, rag.GenerateInput, rag.StreamEmitter) (string, error) {
			modelCalls.Add(1)
			return "unexpected", nil
		},
	}
	s := newAppChatStreamServer(newFakeAppChatStreamStore(), generator)
	s.userPreferences = preferences

	body := performPreferenceStreamRequest(t, s, 7, 42, "以后回答短一点", context.Background())

	if modelCalls.Load() != 0 {
		t.Fatalf("model called %d times after durable write failure", modelCalls.Load())
	}
	if !strings.Contains(body, "偏好保存失败，请重试") || strings.Contains(body, "event: done\n") {
		t.Fatalf("write failure was not explicit and terminal: %q", body)
	}
}

func TestAppChatGenerationFailureKeepsDurablePreference(t *testing.T) {
	preferences := newFakeAppChatPreferenceStore()
	generator := &controlledAppChatStreamingGenerator{
		generateStream: func(context.Context, rag.GenerateInput, rag.StreamEmitter) (string, error) {
			return "", errors.New("provider unavailable")
		},
	}
	s := newAppChatStreamServer(newFakeAppChatStreamStore(), generator)
	s.userPreferences = preferences

	body := performPreferenceStreamRequest(t, s, 7, 42, "以后回答短一点", context.Background())

	stored, err := preferences.List(context.Background(), 7)
	if err != nil {
		t.Fatal(err)
	}
	if len(stored) != 1 || stored[0].Instruction != "回答简短，避免长篇大论" {
		t.Fatalf("durable setting was lost after generation failure: %+v", stored)
	}
	if !strings.Contains(body, "event: done\n") || strings.Contains(body, "已经记住") {
		t.Fatalf("fallback answer must not undo or falsely promise the setting: %q", body)
	}
}

func TestAppChatPreferencesAreGlobalPerUserButIsolatedBetweenUsers(t *testing.T) {
	preferences := newFakeAppChatPreferenceStore()
	var mu sync.Mutex
	var inputs []rag.GenerateInput
	generator := &controlledAppChatStreamingGenerator{
		generateStream: func(_ context.Context, input rag.GenerateInput, emit rag.StreamEmitter) (string, error) {
			mu.Lock()
			inputs = append(inputs, input)
			mu.Unlock()
			if err := emit("回答"); err != nil {
				return "", err
			}
			return "回答", nil
		},
	}
	s := newAppChatStreamServer(newFakeAppChatStreamStore(), generator)
	s.userPreferences = preferences

	performPreferenceStreamRequest(t, s, 7, 42, "以后回答短一点", context.Background())
	performPreferenceStreamRequest(t, s, 8, 43, "怎么做？", context.Background())
	performPreferenceStreamRequest(t, s, 7, 99, "换个会话继续", context.Background())

	mu.Lock()
	defer mu.Unlock()
	if len(inputs) != 3 {
		t.Fatalf("model inputs = %d, want 3", len(inputs))
	}
	if len(inputs[1].UserPreferences) != 0 {
		t.Fatalf("user A preference leaked to user B: %+v", inputs[1].UserPreferences)
	}
	if strings.Join(inputs[2].UserPreferences, "|") != "回答简短，避免长篇大论" {
		t.Fatalf("same user's preference did not cross session/card: %+v", inputs[2].UserPreferences)
	}
}

func TestAppChatPreferenceFallbackSkipsImmediatelyWhenSlotFull(t *testing.T) {
	extractor := &fakeAppChatPreferenceExtractor{}
	s := &Server{
		userPreferences:        newFakeAppChatPreferenceStore(),
		preferenceExtractor:    extractor,
		preferenceAsyncSlots:   make(chan struct{}, 1),
		preferenceAsyncTimeout: time.Second,
	}
	s.preferenceAsyncSlots <- struct{}{}
	turn := s.beginAppChatPreferenceTurn(7)
	defer s.finishAppChatPreferenceTurn(turn)

	started := time.Now()
	accepted := s.scheduleAppChatPreferenceFallback(turn, "以后回答语气更成熟一点")
	if accepted {
		t.Fatal("full slot unexpectedly accepted fallback")
	}
	if elapsed := time.Since(started); elapsed > 50*time.Millisecond {
		t.Fatalf("full slot did not skip immediately: %s", elapsed)
	}
	if extractor.calls.Load() != 0 {
		t.Fatalf("extractor called %d times despite full slot", extractor.calls.Load())
	}
}

func TestAppChatPreferenceFallbackReservationReleasesOwnershipExactlyOnce(t *testing.T) {
	s := &Server{
		userPreferences:      newFakeAppChatPreferenceStore(),
		preferenceExtractor:  &fakeAppChatPreferenceExtractor{},
		preferenceAsyncSlots: make(chan struct{}, 1),
	}
	turn := s.beginAppChatPreferenceTurn(7)
	reservation := s.reserveAppChatPreferenceFallback(turn, "以后回答语气更成熟一点")
	if reservation == nil {
		t.Fatal("fallback reservation was not acquired")
	}
	if turn.state.pending.Load() != 1 || len(s.preferenceAsyncSlots) != 1 {
		t.Fatalf("reservation ownership = pending:%d slots:%d, want 1/1", turn.state.pending.Load(), len(s.preferenceAsyncSlots))
	}
	reservation.release()
	reservation.release()
	if reservation.start() {
		t.Fatal("released reservation started an async fallback")
	}
	if turn.state.pending.Load() != 0 || len(s.preferenceAsyncSlots) != 0 {
		t.Fatalf("released ownership = pending:%d slots:%d, want 0/0", turn.state.pending.Load(), len(s.preferenceAsyncSlots))
	}
	s.finishAppChatPreferenceTurn(turn)
	waitForPreferenceTurnCleanup(t, s, 7)
}

func TestAppChatPreferenceFallbackUsesIndependentBoundedContextAndIgnoresCurrentDirectives(t *testing.T) {
	started := make(chan context.Context, 1)
	release := make(chan struct{})
	extractor := &fakeAppChatPreferenceExtractor{
		extract: func(ctx context.Context, _ string) userpreference.Extraction {
			started <- ctx
			<-release
			return userpreference.Extraction{
				CurrentDirectives: []string{"注入当前回答"},
				Mutations: []userpreference.Mutation{{Upsert: &userpreference.Preference{
					Category: "tone", Slot: "tone.warmth", Instruction: "语气沉稳冷静", SourceText: "以后回答语气更成熟一点",
				}}},
			}
		},
	}
	preferences := newFakeAppChatPreferenceStore()
	var modelInput rag.GenerateInput
	generator := &controlledAppChatStreamingGenerator{
		generateStream: func(_ context.Context, input rag.GenerateInput, emit rag.StreamEmitter) (string, error) {
			modelInput = input
			if err := emit("当前回答"); err != nil {
				return "", err
			}
			return "当前回答", nil
		},
	}
	s := newAppChatStreamServer(newFakeAppChatStreamStore(), generator)
	s.userPreferences = preferences
	s.preferenceExtractor = extractor
	s.preferenceAsyncSlots = make(chan struct{}, 1)
	s.preferenceAsyncTimeout = 200 * time.Millisecond
	requestCtx, cancel := context.WithCancel(context.Background())

	performPreferenceStreamRequest(t, s, 7, 42, "以后回答语气更成熟一点", requestCtx)
	var asyncCtx context.Context
	select {
	case asyncCtx = <-started:
	case <-time.After(time.Second):
		t.Fatal("async extractor did not start")
	}
	cancel()
	if asyncCtx.Err() != nil {
		t.Fatalf("async context was canceled with request: %v", asyncCtx.Err())
	}
	if _, ok := asyncCtx.Deadline(); !ok {
		t.Fatal("async context is not bounded")
	}
	close(release)
	preferences.waitForApply(t, 1)

	if strings.Contains(strings.Join(modelInput.CurrentDirectives, "|"), "注入当前回答") {
		t.Fatalf("LLM fallback directive reached current reply: %+v", modelInput.CurrentDirectives)
	}
	stored, _ := preferences.List(context.Background(), 7)
	if len(stored) != 1 || stored[0].Instruction != "语气沉稳冷静" {
		t.Fatalf("async mutations were not applied: %+v", stored)
	}
}

func TestAppChatPreferenceFallbackDoesNotOverwriteNewerDurablePreference(t *testing.T) {
	started := make(chan struct{})
	release := make(chan struct{})
	extractor := &fakeAppChatPreferenceExtractor{
		extract: func(context.Context, string) userpreference.Extraction {
			close(started)
			<-release
			return userpreference.Extraction{Mutations: []userpreference.Mutation{{Upsert: &userpreference.Preference{
				Category: "length", Slot: "length.detail_level", Instruction: "回答简短，避免长篇大论",
			}}}}
		},
	}
	preferences := newFakeAppChatPreferenceStore()
	s := newAppChatStreamServer(newFakeAppChatStreamStore(), successfulAppChatGenerator("回答"))
	s.userPreferences = preferences
	s.preferenceExtractor = extractor
	s.preferenceAsyncSlots = make(chan struct{}, 1)

	performPreferenceStreamRequest(t, s, 7, 42, "以后回答风格更凝练一点", context.Background())
	select {
	case <-started:
	case <-time.After(time.Second):
		t.Fatal("older async extraction did not start")
	}
	performPreferenceStreamRequest(t, s, 7, 43, "以后回答详细一点", context.Background())
	close(release)
	waitForPreferenceTurnCleanup(t, s, 7)

	stored, err := preferences.List(context.Background(), 7)
	if err != nil {
		t.Fatal(err)
	}
	if len(stored) != 1 || stored[0].Instruction != "回答更详细" {
		t.Fatalf("older async extraction overwrote newer durable setting: %+v", stored)
	}
}

func TestAppChatPreferenceCommittedFallbackKeepsOrderingAfterClientDisconnect(t *testing.T) {
	store := &firstSaveBlockingAppChatStore{
		fakeAppChatStreamStore: newFakeAppChatStreamStore(),
		firstStarted:           make(chan struct{}),
		releaseFirst:           make(chan struct{}),
		firstCommitted:         make(chan struct{}),
	}
	released := false
	defer func() {
		if !released {
			close(store.releaseFirst)
		}
	}()
	extractor := &fakeAppChatPreferenceExtractor{
		extract: func(context.Context, string) userpreference.Extraction {
			return userpreference.Extraction{Mutations: []userpreference.Mutation{{Upsert: &userpreference.Preference{
				Category: "length", Slot: "length.detail_level", Instruction: "回答简短，避免长篇大论",
			}}}}
		},
	}
	preferences := newFakeAppChatPreferenceStore()
	s := newAppChatStreamServer(store, successfulAppChatGenerator("回答"))
	s.userPreferences = preferences
	s.preferenceExtractor = extractor
	s.preferenceAsyncSlots = make(chan struct{}, 1)

	firstCtx, cancelFirst := context.WithCancel(context.Background())
	firstDone := make(chan struct{})
	go func() {
		defer close(firstDone)
		performPreferenceStreamRequest(t, s, 7, 42, "以后回答风格更凝练一点", firstCtx)
	}()
	select {
	case <-store.firstStarted:
	case <-time.After(time.Second):
		t.Fatal("first SavePair did not enter its may-commit window")
	}
	s.preferenceTurnsMu.Lock()
	firstState := s.preferenceTurns[7]
	s.preferenceTurnsMu.Unlock()
	if firstState == nil {
		t.Fatal("first preference turn state was missing")
	}
	cancelFirst()
	select {
	case <-firstDone:
	case <-time.After(time.Second):
		t.Fatal("first handler did not return after client disconnect")
	}
	s.preferenceTurnsMu.Lock()
	retainedState := s.preferenceTurns[7]
	s.preferenceTurnsMu.Unlock()
	if retainedState != firstState {
		t.Fatal("may-commit fallback ownership did not retain the original turn state")
	}

	performPreferenceStreamRequest(t, s, 7, 43, "以后回答详细一点", context.Background())
	preferences.waitForApply(t, 1)
	close(store.releaseFirst)
	released = true
	select {
	case <-store.firstCommitted:
	case <-time.After(time.Second):
		t.Fatal("first SavePair did not report its committed result")
	}
	waitForPreferenceTurnCleanup(t, s, 7)

	stored, err := preferences.List(context.Background(), 7)
	if err != nil {
		t.Fatal(err)
	}
	if len(stored) != 1 || stored[0].Instruction != "回答更详细" {
		t.Fatalf("disconnected older fallback escaped ticket ordering: %+v", stored)
	}
}

func TestAppChatPreferenceHeartbeatFailureBeforePersistenceStopsLateSave(t *testing.T) {
	hookReached := make(chan struct{})
	releaseHook := make(chan struct{})
	store := newFakeAppChatStreamStore()
	saveCalls := make(chan struct{}, 4)
	store.onSave = func() { saveCalls <- struct{}{} }
	extractor := &fakeAppChatPreferenceExtractor{
		extract: func(context.Context, string) userpreference.Extraction {
			return userpreference.Extraction{Mutations: []userpreference.Mutation{{Upsert: &userpreference.Preference{
				Category: "length", Slot: "length.detail_level", Instruction: "回答简短，避免长篇大论",
			}}}}
		},
	}
	preferences := newFakeAppChatPreferenceStore()
	s := newAppChatStreamServer(store, successfulAppChatGenerator("回答"))
	s.userPreferences = preferences
	s.preferenceExtractor = extractor
	s.preferenceAsyncSlots = make(chan struct{}, 1)
	s.chatHeartbeatInterval = 10 * time.Millisecond
	s.chatProviderIdleTimeout = time.Second
	var hookCalls atomic.Int32
	s.chatPersistHook = func() {
		if hookCalls.Add(1) != 1 {
			return
		}
		close(hookReached)
		<-releaseHook
	}

	writer := &failingFrameAppChatStreamWriter{header: make(http.Header), failFrame: ": ping\n\n"}
	req := httptest.NewRequest(http.MethodPost, "/api/app/chat/sessions/42/ask/stream", strings.NewReader(`{"question":"以后回答风格更凝练一点"}`))
	req = req.WithContext(context.WithValue(context.Background(), appContextKey{}, auth.UserInfo{ID: 7}))
	handlerDone := make(chan struct{})
	go func() {
		defer close(handlerDone)
		s.appChatRouter(writer, req)
	}()
	select {
	case <-hookReached:
	case <-time.After(time.Second):
		close(releaseHook)
		t.Fatal("pipeline did not reach the pre-persistence window")
	}
	select {
	case <-handlerDone:
	case <-time.After(time.Second):
		close(releaseHook)
		t.Fatal("heartbeat write failure did not return the handler")
	}

	performPreferenceStreamRequest(t, s, 7, 43, "以后回答详细一点", context.Background())
	preferences.waitForApply(t, 1)
	select {
	case <-saveCalls:
	case <-time.After(time.Second):
		close(releaseHook)
		t.Fatal("newer turn was not saved")
	}
	close(releaseHook)
	select {
	case <-saveCalls:
		t.Fatal("pre-persistence heartbeat failure still entered SavePair")
	case <-time.After(50 * time.Millisecond):
	}
	if extractor.calls.Load() != 0 {
		t.Fatalf("pre-persistence heartbeat failure started %d fallbacks", extractor.calls.Load())
	}
	stored, err := preferences.List(context.Background(), 7)
	if err != nil {
		t.Fatal(err)
	}
	if len(stored) != 1 || stored[0].Instruction != "回答更详细" {
		t.Fatalf("late pre-persistence worker overwrote the newer preference: %+v", stored)
	}
}

func TestAppChatPreferenceHeartbeatFailureAfterPersistenceKeepsOrdering(t *testing.T) {
	store := &firstSaveBlockingAppChatStore{
		fakeAppChatStreamStore: newFakeAppChatStreamStore(),
		firstStarted:           make(chan struct{}),
		releaseFirst:           make(chan struct{}),
		firstCommitted:         make(chan struct{}),
	}
	released := false
	defer func() {
		if !released {
			close(store.releaseFirst)
		}
	}()
	extractor := &fakeAppChatPreferenceExtractor{
		extract: func(context.Context, string) userpreference.Extraction {
			return userpreference.Extraction{Mutations: []userpreference.Mutation{{Upsert: &userpreference.Preference{
				Category: "length", Slot: "length.detail_level", Instruction: "回答简短，避免长篇大论",
			}}}}
		},
	}
	preferences := newFakeAppChatPreferenceStore()
	s := newAppChatStreamServer(store, successfulAppChatGenerator("回答"))
	s.userPreferences = preferences
	s.preferenceExtractor = extractor
	s.preferenceAsyncSlots = make(chan struct{}, 1)
	s.chatHeartbeatInterval = 10 * time.Millisecond
	s.chatProviderIdleTimeout = time.Second

	writer := &failingFrameAppChatStreamWriter{header: make(http.Header), failFrame: ": ping\n\n"}
	req := httptest.NewRequest(http.MethodPost, "/api/app/chat/sessions/42/ask/stream", strings.NewReader(`{"question":"以后回答风格更凝练一点"}`))
	req = req.WithContext(context.WithValue(context.Background(), appContextKey{}, auth.UserInfo{ID: 7}))
	handlerDone := make(chan struct{})
	go func() {
		defer close(handlerDone)
		s.appChatRouter(writer, req)
	}()
	select {
	case <-store.firstStarted:
	case <-time.After(time.Second):
		t.Fatal("SavePair did not enter persistence")
	}
	s.preferenceTurnsMu.Lock()
	firstState := s.preferenceTurns[7]
	s.preferenceTurnsMu.Unlock()
	select {
	case <-handlerDone:
	case <-time.After(time.Second):
		t.Fatal("heartbeat write failure did not return during persistence")
	}
	s.preferenceTurnsMu.Lock()
	retainedState := s.preferenceTurns[7]
	s.preferenceTurnsMu.Unlock()
	if firstState == nil || retainedState != firstState {
		t.Fatal("persistence ownership was released after heartbeat failure")
	}

	performPreferenceStreamRequest(t, s, 7, 43, "以后回答详细一点", context.Background())
	preferences.waitForApply(t, 1)
	close(store.releaseFirst)
	released = true
	select {
	case <-store.firstCommitted:
	case <-time.After(time.Second):
		t.Fatal("older SavePair did not commit")
	}
	waitForPreferenceTurnCleanup(t, s, 7)
	stored, err := preferences.List(context.Background(), 7)
	if err != nil {
		t.Fatal(err)
	}
	if len(stored) != 1 || stored[0].Instruction != "回答更详细" {
		t.Fatalf("older committed fallback escaped ordering after heartbeat failure: %+v", stored)
	}
}

func TestOrdinaryChatDoesNotInvalidatePendingDurablePreference(t *testing.T) {
	started := make(chan struct{})
	release := make(chan struct{})
	extractor := &fakeAppChatPreferenceExtractor{
		extract: func(context.Context, string) userpreference.Extraction {
			close(started)
			<-release
			return userpreference.Extraction{Mutations: []userpreference.Mutation{{Upsert: &userpreference.Preference{
				Category: "tone", Slot: "tone.formality", Instruction: "使用正式语气",
			}}}}
		},
	}
	preferences := newFakeAppChatPreferenceStore()
	s := newAppChatStreamServer(newFakeAppChatStreamStore(), successfulAppChatGenerator("回答"))
	s.userPreferences = preferences
	s.preferenceExtractor = extractor
	s.preferenceAsyncSlots = make(chan struct{}, 1)

	performPreferenceStreamRequest(t, s, 7, 42, "以后回答语气更成熟一些", context.Background())
	select {
	case <-started:
	case <-time.After(time.Second):
		t.Fatal("durable async extraction did not start")
	}
	performPreferenceStreamRequest(t, s, 7, 43, "谢谢", context.Background())
	close(release)
	waitForPreferenceTurnCleanup(t, s, 7)

	stored, err := preferences.List(context.Background(), 7)
	if err != nil {
		t.Fatal(err)
	}
	if len(stored) != 1 || stored[0].Instruction != "使用正式语气" {
		t.Fatalf("ordinary chat invalidated pending durable preference: %+v", stored)
	}
}

func TestAsyncPreferenceDifferentSlotSurvivesNewerSynchronousPreference(t *testing.T) {
	started := make(chan struct{})
	release := make(chan struct{})
	extractor := &fakeAppChatPreferenceExtractor{
		extract: func(context.Context, string) userpreference.Extraction {
			close(started)
			<-release
			return userpreference.Extraction{Mutations: []userpreference.Mutation{{Upsert: &userpreference.Preference{
				Category: "custom", Slot: "custom.communication_style", Instruction: "语气幽默自然",
			}}}}
		},
	}
	preferences := newFakeAppChatPreferenceStore()
	s := newAppChatStreamServer(newFakeAppChatStreamStore(), successfulAppChatGenerator("回答"))
	s.userPreferences = preferences
	s.preferenceExtractor = extractor
	s.preferenceAsyncSlots = make(chan struct{}, 1)

	performPreferenceStreamRequest(t, s, 7, 42, "以后回答风格更幽默一些", context.Background())
	select {
	case <-started:
	case <-time.After(time.Second):
		t.Fatal("older async extraction did not start")
	}
	performPreferenceStreamRequest(t, s, 7, 43, "以后回答短一点", context.Background())
	close(release)
	waitForPreferenceTurnCleanup(t, s, 7)

	assertPreferenceInstructions(t, preferences, 7, "回答简短，避免长篇大论", "语气幽默自然")
}

func TestAsyncPreferencesInDifferentSlotsBothPersist(t *testing.T) {
	olderStarted := make(chan struct{})
	releaseOlder := make(chan struct{})
	extractor := &fakeAppChatPreferenceExtractor{
		extract: func(_ context.Context, message string) userpreference.Extraction {
			if strings.Contains(message, "温暖") {
				close(olderStarted)
				<-releaseOlder
				return userpreference.Extraction{Mutations: []userpreference.Mutation{{Upsert: &userpreference.Preference{
					Category: "tone", Slot: "tone.warmth", Instruction: "语气温柔友好",
				}}}}
			}
			return userpreference.Extraction{Mutations: []userpreference.Mutation{{Upsert: &userpreference.Preference{
				Category: "length", Slot: "length.detail_level", Instruction: "回答简短，避免长篇大论",
			}}}}
		},
	}
	preferences := newFakeAppChatPreferenceStore()
	s := newAppChatStreamServer(newFakeAppChatStreamStore(), successfulAppChatGenerator("回答"))
	s.userPreferences = preferences
	s.preferenceExtractor = extractor
	s.preferenceAsyncSlots = make(chan struct{}, 2)

	performPreferenceStreamRequest(t, s, 7, 42, "以后回答风格更温暖一些", context.Background())
	select {
	case <-olderStarted:
	case <-time.After(time.Second):
		t.Fatal("older async extraction did not start")
	}
	performPreferenceStreamRequest(t, s, 7, 43, "以后回答风格更凝练一些", context.Background())
	waitForPreferenceInstruction(t, preferences, 7, "回答简短，避免长篇大论")
	close(releaseOlder)
	waitForPreferenceTurnCleanup(t, s, 7)

	assertPreferenceInstructions(t, preferences, 7, "回答简短，避免长篇大论", "语气温柔友好")
}

func TestAsyncMultiMutationFiltersOnlySlotsTouchedByNewerCancellation(t *testing.T) {
	started := make(chan struct{})
	release := make(chan struct{})
	extractor := &fakeAppChatPreferenceExtractor{
		extract: func(context.Context, string) userpreference.Extraction {
			close(started)
			<-release
			return userpreference.Extraction{Mutations: []userpreference.Mutation{
				{Upsert: &userpreference.Preference{Category: "tone", Slot: "tone.warmth", Instruction: "语气温柔友好"}},
				{Upsert: &userpreference.Preference{Category: "custom", Slot: "custom.communication_style", Instruction: "语气幽默自然"}},
			}}
		},
	}
	preferences := newFakeAppChatPreferenceStore()
	s := newAppChatStreamServer(newFakeAppChatStreamStore(), successfulAppChatGenerator("回答"))
	s.userPreferences = preferences
	s.preferenceExtractor = extractor
	s.preferenceAsyncSlots = make(chan struct{}, 1)

	performPreferenceStreamRequest(t, s, 7, 42, "以后回答风格更温暖幽默一些", context.Background())
	select {
	case <-started:
	case <-time.After(time.Second):
		t.Fatal("older multi-mutation extraction did not start")
	}
	performPreferenceStreamRequest(t, s, 7, 43, "取消所有语气要求", context.Background())
	close(release)
	waitForPreferenceTurnCleanup(t, s, 7)

	assertPreferenceInstructions(t, preferences, 7, "语气幽默自然")
}

func TestCurrentOnlyDirectiveDoesNotInvalidatePendingDurablePreference(t *testing.T) {
	started := make(chan struct{})
	release := make(chan struct{})
	extractor := &fakeAppChatPreferenceExtractor{
		extract: func(context.Context, string) userpreference.Extraction {
			close(started)
			<-release
			return userpreference.Extraction{Mutations: []userpreference.Mutation{{Upsert: &userpreference.Preference{
				Category: "tone", Slot: "tone.formality", Instruction: "使用正式语气",
			}}}}
		},
	}
	preferences := newFakeAppChatPreferenceStore()
	var mu sync.Mutex
	var inputs []rag.GenerateInput
	generator := &controlledAppChatStreamingGenerator{
		generateStream: func(_ context.Context, input rag.GenerateInput, emit rag.StreamEmitter) (string, error) {
			mu.Lock()
			inputs = append(inputs, input)
			mu.Unlock()
			if err := emit("回答"); err != nil {
				return "", err
			}
			return "回答", nil
		},
	}
	s := newAppChatStreamServer(newFakeAppChatStreamStore(), generator)
	s.userPreferences = preferences
	s.preferenceExtractor = extractor
	s.preferenceAsyncSlots = make(chan struct{}, 1)

	performPreferenceStreamRequest(t, s, 7, 42, "以后回答语气更成熟一些", context.Background())
	select {
	case <-started:
	case <-time.After(time.Second):
		t.Fatal("durable async extraction did not start")
	}
	performPreferenceStreamRequest(t, s, 7, 43, "这次详细说", context.Background())
	close(release)
	waitForPreferenceTurnCleanup(t, s, 7)

	stored, err := preferences.List(context.Background(), 7)
	if err != nil {
		t.Fatal(err)
	}
	if len(stored) != 1 || stored[0].Instruction != "使用正式语气" {
		t.Fatalf("current-only directive invalidated pending durable preference: %+v", stored)
	}
	mu.Lock()
	defer mu.Unlock()
	if len(inputs) != 2 || strings.Join(inputs[1].CurrentDirectives, "|") != "回答更详细" {
		t.Fatalf("current-only directive did not reach its own response: %+v", inputs)
	}
}

func TestNewerUnresolvedDurablePreferenceSupersedesOlderAsyncExtraction(t *testing.T) {
	olderStarted := make(chan struct{})
	releaseOlder := make(chan struct{})
	extractor := &fakeAppChatPreferenceExtractor{
		extract: func(_ context.Context, message string) userpreference.Extraction {
			if strings.Contains(message, "凝练") {
				close(olderStarted)
				<-releaseOlder
				return userpreference.Extraction{Mutations: []userpreference.Mutation{{Upsert: &userpreference.Preference{
					Category: "length", Slot: "length.detail_level", Instruction: "回答简短，避免长篇大论",
				}}}}
			}
			return userpreference.Extraction{Mutations: []userpreference.Mutation{{Upsert: &userpreference.Preference{
				Category: "length", Slot: "length.detail_level", Instruction: "回答更详细",
			}}}}
		},
	}
	preferences := newFakeAppChatPreferenceStore()
	s := newAppChatStreamServer(newFakeAppChatStreamStore(), successfulAppChatGenerator("回答"))
	s.userPreferences = preferences
	s.preferenceExtractor = extractor
	s.preferenceAsyncSlots = make(chan struct{}, 2)

	performPreferenceStreamRequest(t, s, 7, 42, "以后回答风格更凝练一些", context.Background())
	select {
	case <-olderStarted:
	case <-time.After(time.Second):
		t.Fatal("older unresolved extraction did not start")
	}
	performPreferenceStreamRequest(t, s, 7, 43, "以后回答风格更铺陈一些", context.Background())
	waitForPreferenceInstruction(t, preferences, 7, "回答更详细")
	close(releaseOlder)
	waitForPreferenceTurnCleanup(t, s, 7)

	stored, err := preferences.List(context.Background(), 7)
	if err != nil {
		t.Fatal(err)
	}
	if len(stored) != 1 || stored[0].Instruction != "回答更详细" {
		t.Fatalf("older unresolved result overwrote newer unresolved preference: %+v", stored)
	}
}

func TestAppChatPreferenceFallbackDoesNotResurrectNewerCancellation(t *testing.T) {
	started := make(chan struct{})
	release := make(chan struct{})
	extractor := &fakeAppChatPreferenceExtractor{
		extract: func(context.Context, string) userpreference.Extraction {
			close(started)
			<-release
			return userpreference.Extraction{Mutations: []userpreference.Mutation{{Upsert: &userpreference.Preference{
				Category: "tone", Slot: "tone.formality", Instruction: "使用正式语气",
			}}}}
		},
	}
	preferences := newFakeAppChatPreferenceStore()
	if err := preferences.Apply(context.Background(), 7, []userpreference.Mutation{{Upsert: &userpreference.Preference{
		Category: "tone", Slot: "tone.formality", Instruction: "使用正式语气",
	}}}); err != nil {
		t.Fatal(err)
	}
	s := newAppChatStreamServer(newFakeAppChatStreamStore(), successfulAppChatGenerator("回答"))
	s.userPreferences = preferences
	s.preferenceExtractor = extractor
	s.preferenceAsyncSlots = make(chan struct{}, 1)

	performPreferenceStreamRequest(t, s, 7, 42, "以后回答语气更成熟一些", context.Background())
	select {
	case <-started:
	case <-time.After(time.Second):
		t.Fatal("older async extraction did not start")
	}
	performPreferenceStreamRequest(t, s, 7, 43, "取消所有语气要求", context.Background())
	close(release)
	waitForPreferenceTurnCleanup(t, s, 7)

	stored, err := preferences.List(context.Background(), 7)
	if err != nil {
		t.Fatal(err)
	}
	if len(stored) != 0 {
		t.Fatalf("older async extraction resurrected canceled preference: %+v", stored)
	}
}

func TestNewerPreferenceTurnWaitsForAsyncCheckAndApplyThenWins(t *testing.T) {
	applyStarted := make(chan struct{})
	releaseApply := make(chan struct{})
	preferences := newFakeAppChatPreferenceStore()
	preferences.beforeApply = func(mutations []userpreference.Mutation) {
		if len(mutations) == 1 && mutations[0].Upsert != nil && mutations[0].Upsert.Slot == "tone.formality" {
			close(applyStarted)
			<-releaseApply
		}
	}
	extractor := &fakeAppChatPreferenceExtractor{
		extract: func(context.Context, string) userpreference.Extraction {
			return userpreference.Extraction{Mutations: []userpreference.Mutation{{Upsert: &userpreference.Preference{
				Category: "tone", Slot: "tone.formality", Instruction: "使用正式语气",
			}}}}
		},
	}
	s := newAppChatStreamServer(newFakeAppChatStreamStore(), successfulAppChatGenerator("回答"))
	s.userPreferences = preferences
	s.preferenceExtractor = extractor
	s.preferenceAsyncSlots = make(chan struct{}, 1)

	performPreferenceStreamRequest(t, s, 7, 42, "以后回答语气更成熟一些", context.Background())
	select {
	case <-applyStarted:
	case <-time.After(time.Second):
		t.Fatal("older async apply did not start")
	}
	newerDone := make(chan struct{})
	go func() {
		defer close(newerDone)
		performPreferenceStreamRequest(t, s, 7, 43, "取消所有语气要求", context.Background())
	}()
	select {
	case <-newerDone:
		t.Fatal("newer turn completed while older async apply held the ordering lock")
	case <-time.After(20 * time.Millisecond):
	}
	otherUserDone := make(chan struct{})
	go func() {
		defer close(otherUserDone)
		performPreferenceStreamRequest(t, s, 8, 44, "怎么做？", context.Background())
	}()
	select {
	case <-otherUserDone:
	case <-time.After(100 * time.Millisecond):
		t.Fatal("a slow preference apply for user A blocked user B")
	}
	close(releaseApply)
	select {
	case <-newerDone:
	case <-time.After(time.Second):
		t.Fatal("newer turn did not finish after older apply released")
	}
	waitForPreferenceTurnCleanup(t, s, 7)

	stored, err := preferences.List(context.Background(), 7)
	if err != nil {
		t.Fatal(err)
	}
	if len(stored) != 0 {
		t.Fatalf("newer cancellation did not win after ordered apply: %+v", stored)
	}
}

func TestPreferenceTurnVersionsAreIssuedInArrivalOrderWithoutWaitingForApplyLock(t *testing.T) {
	state := newAppChatPreferenceTurnState()
	s := &Server{preferenceTurns: map[int64]*appChatPreferenceTurnState{7: state}}
	state.mu.Lock()
	locked := true
	defer func() {
		if locked {
			state.mu.Unlock()
		}
	}()

	firstCh := make(chan appChatPreferenceTurn, 1)
	go func() { firstCh <- s.beginAppChatPreferenceTurn(7) }()
	var first appChatPreferenceTurn
	select {
	case first = <-firstCh:
	case <-time.After(50 * time.Millisecond):
		state.mu.Unlock()
		locked = false
		t.Fatal("first turn waited for the apply lock before receiving a version")
	}
	secondCh := make(chan appChatPreferenceTurn, 1)
	go func() { secondCh <- s.beginAppChatPreferenceTurn(7) }()
	var second appChatPreferenceTurn
	select {
	case second = <-secondCh:
	case <-time.After(50 * time.Millisecond):
		state.mu.Unlock()
		locked = false
		t.Fatal("second turn waited for the apply lock before receiving a version")
	}
	state.mu.Unlock()
	locked = false
	defer s.finishAppChatPreferenceTurn(first)
	defer s.finishAppChatPreferenceTurn(second)
	if first.ticket != 1 || second.ticket != 2 {
		t.Fatalf("same-user tickets were inverted: first=%d second=%d", first.ticket, second.ticket)
	}
}

func TestAppChatPreferenceFallbackDoesNotRunAfterSaveFailure(t *testing.T) {
	chatStore := newFakeAppChatStreamStore()
	chatStore.saveErr = errors.New("save failed")
	extractor := &fakeAppChatPreferenceExtractor{}
	s := newAppChatStreamServer(chatStore, successfulAppChatGenerator("回答"))
	s.userPreferences = newFakeAppChatPreferenceStore()
	s.preferenceExtractor = extractor
	s.preferenceAsyncSlots = make(chan struct{}, 1)

	performPreferenceStreamRequest(t, s, 7, 42, "以后回答语气更成熟一点", context.Background())
	time.Sleep(20 * time.Millisecond)
	if extractor.calls.Load() != 0 {
		t.Fatalf("async extractor called %d times after SavePair failure", extractor.calls.Load())
	}
	if len(s.preferenceAsyncSlots) != 0 {
		t.Fatalf("save failure leaked %d fallback slots", len(s.preferenceAsyncSlots))
	}
	waitForPreferenceTurnCleanup(t, s, 7)
}

func TestAppChatPreferenceFallbackDoesNotRunAfterPartialGenerationFailure(t *testing.T) {
	extractor := &fakeAppChatPreferenceExtractor{}
	generator := &controlledAppChatStreamingGenerator{
		generateStream: func(_ context.Context, _ rag.GenerateInput, emit rag.StreamEmitter) (string, error) {
			if err := emit("部分回答"); err != nil {
				return "", err
			}
			return "部分回答", errors.New("stream interrupted")
		},
	}
	s := newAppChatStreamServer(newFakeAppChatStreamStore(), generator)
	s.userPreferences = newFakeAppChatPreferenceStore()
	s.preferenceExtractor = extractor
	s.preferenceAsyncSlots = make(chan struct{}, 1)

	body := performPreferenceStreamRequest(t, s, 7, 42, "以后回答语气更成熟一点", context.Background())
	time.Sleep(20 * time.Millisecond)
	if !strings.Contains(body, "event: error\n") || strings.Contains(body, "event: done\n") {
		t.Fatalf("partial failure terminal events wrong: %q", body)
	}
	if extractor.calls.Load() != 0 {
		t.Fatalf("async extractor called %d times after partial generation failure", extractor.calls.Load())
	}
}

func TestCompletePreferenceJSONUsesActiveGeneratorDuringRuntimeChanges(t *testing.T) {
	a := &preferenceJSONGenerator{name: "a"}
	b := &preferenceJSONGenerator{name: "b"}
	s := &Server{ragGen: a}

	start := make(chan struct{})
	errCh := make(chan error, 8)
	var wg sync.WaitGroup
	wg.Add(1)
	go func() {
		defer wg.Done()
		<-start
		for i := 0; i < 1000; i++ {
			s.modelMu.Lock()
			if i%2 == 0 {
				s.ragGen = b
			} else {
				s.ragGen = a
			}
			s.modelMu.Unlock()
		}
	}()
	for range 4 {
		wg.Add(1)
		go func() {
			defer wg.Done()
			<-start
			for range 250 {
				value, err := s.completePreferenceJSON(context.Background(), "system", "user", 10)
				if err != nil {
					errCh <- err
					return
				}
				if value != "a" && value != "b" {
					errCh <- fmt.Errorf("unexpected active completer result %q", value)
					return
				}
			}
		}()
	}
	close(start)
	wg.Wait()
	close(errCh)
	for err := range errCh {
		t.Error(err)
	}
}

func performPreferenceStreamRequest(t *testing.T, s *Server, userID, sessionID int64, question string, ctx context.Context) string {
	t.Helper()
	writer := newAppChatBlockingStreamWriter()
	req := httptest.NewRequest(http.MethodPost, fmt.Sprintf("/api/app/chat/sessions/%d/ask/stream", sessionID), strings.NewReader(fmt.Sprintf(`{"question":%q}`, question)))
	req = req.WithContext(context.WithValue(ctx, appContextKey{}, auth.UserInfo{ID: userID}))
	s.appChatRouter(writer, req)
	return writer.BodyString()
}

type fakeAppChatPreferenceStore struct {
	mu          sync.Mutex
	byUser      map[int64]map[string]userpreference.Preference
	applyErr    error
	onApply     func()
	beforeApply func([]userpreference.Mutation)
	applyCall   chan struct{}
}

type firstSaveBlockingAppChatStore struct {
	*fakeAppChatStreamStore
	calls          atomic.Int32
	firstStarted   chan struct{}
	releaseFirst   chan struct{}
	firstCommitted chan struct{}
}

func (s *firstSaveBlockingAppChatStore) SavePair(_ context.Context, _ int64, _, _ string, _ json.RawMessage) (int64, error) {
	call := s.calls.Add(1)
	if call == 1 {
		close(s.firstStarted)
		<-s.releaseFirst
		close(s.firstCommitted)
	}
	return int64(100 + call), nil
}

func newFakeAppChatPreferenceStore() *fakeAppChatPreferenceStore {
	return &fakeAppChatPreferenceStore{
		byUser:    make(map[int64]map[string]userpreference.Preference),
		applyCall: make(chan struct{}, 16),
	}
}

func (s *fakeAppChatPreferenceStore) List(_ context.Context, userID int64) ([]userpreference.Preference, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	values := s.byUser[userID]
	result := make([]userpreference.Preference, 0, len(values))
	for _, preference := range values {
		result = append(result, preference)
	}
	return result, nil
}

func (s *fakeAppChatPreferenceStore) Apply(_ context.Context, userID int64, mutations []userpreference.Mutation) error {
	if s.beforeApply != nil {
		s.beforeApply(mutations)
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.applyErr != nil {
		return s.applyErr
	}
	if s.byUser[userID] == nil {
		s.byUser[userID] = make(map[string]userpreference.Preference)
	}
	for _, mutation := range mutations {
		if mutation.Upsert != nil {
			s.byUser[userID][mutation.Upsert.Slot] = *mutation.Upsert
		} else {
			delete(s.byUser[userID], mutation.DeleteSlot)
		}
	}
	if s.onApply != nil {
		s.onApply()
	}
	select {
	case s.applyCall <- struct{}{}:
	default:
	}
	return nil
}

func waitForPreferenceTurnCleanup(t *testing.T, s *Server, userID int64) {
	t.Helper()
	deadline := time.Now().Add(time.Second)
	for time.Now().Before(deadline) {
		s.preferenceTurnsMu.Lock()
		_, exists := s.preferenceTurns[userID]
		s.preferenceTurnsMu.Unlock()
		if !exists {
			return
		}
		time.Sleep(time.Millisecond)
	}
	t.Fatal("timed out waiting for preference turn cleanup")
}

func waitForPreferenceInstruction(t *testing.T, store *fakeAppChatPreferenceStore, userID int64, instruction string) {
	t.Helper()
	deadline := time.Now().Add(time.Second)
	for time.Now().Before(deadline) {
		stored, err := store.List(context.Background(), userID)
		if err != nil {
			t.Fatal(err)
		}
		for _, preference := range stored {
			if preference.Instruction == instruction {
				return
			}
		}
		time.Sleep(time.Millisecond)
	}
	t.Fatalf("timed out waiting for preference %q", instruction)
}

func assertPreferenceInstructions(t *testing.T, store *fakeAppChatPreferenceStore, userID int64, want ...string) {
	t.Helper()
	stored, err := store.List(context.Background(), userID)
	if err != nil {
		t.Fatal(err)
	}
	got := make([]string, 0, len(stored))
	for _, preference := range stored {
		got = append(got, preference.Instruction)
	}
	sort.Strings(got)
	sort.Strings(want)
	if strings.Join(got, "|") != strings.Join(want, "|") {
		t.Fatalf("preference instructions = %+v, want %+v", got, want)
	}
}

func (s *fakeAppChatPreferenceStore) waitForApply(t *testing.T, count int) {
	t.Helper()
	for i := 0; i < count; i++ {
		select {
		case <-s.applyCall:
		case <-time.After(time.Second):
			t.Fatal("timed out waiting for preference apply")
		}
	}
}

type fakeAppChatPreferenceExtractor struct {
	calls   atomic.Int32
	extract func(context.Context, string) userpreference.Extraction
}

func (e *fakeAppChatPreferenceExtractor) Extract(ctx context.Context, message string) userpreference.Extraction {
	e.calls.Add(1)
	if e.extract == nil {
		return userpreference.Extraction{}
	}
	return e.extract(ctx, message)
}

type preferenceJSONGenerator struct{ name string }

func (g *preferenceJSONGenerator) Generate(context.Context, rag.GenerateInput) (string, error) {
	return "回答", nil
}

func (g *preferenceJSONGenerator) CompleteJSON(context.Context, string, string, int) (string, error) {
	return g.name, nil
}

type capturingNonStreamingAppChatGenerator struct {
	input  rag.GenerateInput
	answer string
}

func (g *capturingNonStreamingAppChatGenerator) Generate(_ context.Context, input rag.GenerateInput) (string, error) {
	g.input = input
	return g.answer, nil
}

func performAppChatPreferenceRequest(t *testing.T, s *Server, userID, sessionID int64, question string) string {
	t.Helper()
	writer := newAppChatBlockingStreamWriter()
	req := httptest.NewRequest(http.MethodPost, fmt.Sprintf("/api/app/chat/sessions/%d/ask/stream", sessionID), strings.NewReader(fmt.Sprintf(`{"question":%q}`, question)))
	req = req.WithContext(context.WithValue(req.Context(), appContextKey{}, auth.UserInfo{ID: userID}))
	s.appChatRouter(writer, req)
	return writer.BodyString()
}
