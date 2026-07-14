package server

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"sort"
	"strings"
	"sync"
	"testing"

	"nine-xing/nx-backend/apps/server/internal/auth"
	"nine-xing/nx-backend/apps/server/internal/rag"
	"nine-xing/nx-backend/apps/server/internal/userpreference"
)

func TestAppChatPreferencesApplyImmediatelyThenPersistAfterSavedAnswer(t *testing.T) {
	preferences := newFakeAppChatPreferenceStore()
	chatStore := newFakeAppChatStreamStore()
	var mu sync.Mutex
	var inputs []rag.GenerateInput
	var order []string
	generator := &controlledAppChatStreamingGenerator{
		generateStream: func(_ context.Context, input rag.GenerateInput, emit rag.StreamEmitter) (string, error) {
			mu.Lock()
			inputs = append(inputs, input)
			order = append(order, "model")
			mu.Unlock()
			if err := emit("回答"); err != nil {
				return "", err
			}
			return "回答", nil
		},
	}
	chatStore.onSave = func() {
		mu.Lock()
		order = append(order, "save")
		mu.Unlock()
	}
	preferences.onApply = func() {
		mu.Lock()
		order = append(order, "preference")
		mu.Unlock()
	}
	s := newAppChatStreamServer(chatStore, generator)
	s.userPreferences = preferences

	performAppChatPreferenceRequest(t, s, 7, 42, "以后回答短一点")
	performAppChatPreferenceRequest(t, s, 7, 99, "换个会话继续")
	performAppChatPreferenceRequest(t, s, 8, 43, "另一个用户的问题")

	mu.Lock()
	defer mu.Unlock()
	if len(inputs) != 3 {
		t.Fatalf("model inputs = %d, want 3", len(inputs))
	}
	if strings.Join(inputs[0].CurrentDirectives, "|") != "回答简短，避免长篇大论" {
		t.Fatalf("current correction was not applied immediately: %+v", inputs[0])
	}
	if strings.Join(inputs[1].UserPreferences, "|") != "回答简短，避免长篇大论" {
		t.Fatalf("saved preference did not cross sessions: %+v", inputs[1])
	}
	if len(inputs[2].UserPreferences) != 0 {
		t.Fatalf("preference leaked to another user: %+v", inputs[2])
	}
	if got := strings.Join(order[:3], ","); got != "model,save,preference" {
		t.Fatalf("first request order = %q, want model,save,preference", got)
	}
}

func TestAppChatPreferenceCurrentOnlyDirectiveIsNotPersisted(t *testing.T) {
	preferences := newFakeAppChatPreferenceStore()
	if err := preferences.Apply(context.Background(), 7, []userpreference.Mutation{{Upsert: &userpreference.Preference{
		Category: "length", Slot: "length.detail_level", Instruction: "回答更详细",
	}}}); err != nil {
		t.Fatal(err)
	}
	var captured rag.GenerateInput
	generator := &controlledAppChatStreamingGenerator{
		generateStream: func(_ context.Context, input rag.GenerateInput, emit rag.StreamEmitter) (string, error) {
			captured = input
			if err := emit("一句回答"); err != nil {
				return "", err
			}
			return "一句回答", nil
		},
	}
	s := newAppChatStreamServer(newFakeAppChatStreamStore(), generator)
	s.userPreferences = preferences

	performAppChatPreferenceRequest(t, s, 7, 42, "这次只回答一句")

	if strings.Join(captured.CurrentDirectives, "|") != "只回答一句" {
		t.Fatalf("one-turn correction missing: %+v", captured)
	}
	if len(captured.UserPreferences) != 0 {
		t.Fatalf("conflicting saved length preference was not overridden: %+v", captured)
	}
	stored, err := preferences.List(context.Background(), 7)
	if err != nil {
		t.Fatal(err)
	}
	if len(stored) != 1 || stored[0].Instruction != "回答更详细" {
		t.Fatalf("one-turn correction changed the durable preference: %+v", stored)
	}
}

func TestAppChatPreferenceIsNotPersistedWhenAnswerSaveFails(t *testing.T) {
	preferences := newFakeAppChatPreferenceStore()
	chatStore := newFakeAppChatStreamStore()
	chatStore.saveErr = errors.New("save failed")
	s := newAppChatStreamServer(chatStore, successfulAppChatGenerator("回答"))
	s.userPreferences = preferences

	body := performAppChatPreferenceRequest(t, s, 7, 42, "以后回答短一点")

	stored, err := preferences.List(context.Background(), 7)
	if err != nil {
		t.Fatal(err)
	}
	if len(stored) != 0 {
		t.Fatalf("preference persisted despite SavePair failure: %+v", stored)
	}
	if !strings.Contains(body, "回答保存失败") || strings.Contains(body, "event: done") {
		t.Fatalf("save failure response is wrong: %q", body)
	}
}

func TestAppChatAvoidDearCorrectionReachesCurrentModelRequest(t *testing.T) {
	var captured rag.GenerateInput
	generator := &controlledAppChatStreamingGenerator{
		generateStream: func(_ context.Context, input rag.GenerateInput, emit rag.StreamEmitter) (string, error) {
			captured = input
			if err := emit("好的"); err != nil {
				return "", err
			}
			return "好的", nil
		},
	}
	s := newAppChatStreamServer(newFakeAppChatStreamStore(), generator)
	s.userPreferences = newFakeAppChatPreferenceStore()

	performAppChatPreferenceRequest(t, s, 7, 42, "不要叫我亲爱的")

	if strings.Join(captured.CurrentDirectives, "|") != "不要使用“亲爱的”等亲昵称呼" {
		t.Fatalf("addressing correction missing from current request: %+v", captured)
	}
}

func performAppChatPreferenceRequest(t *testing.T, s *Server, userID, sessionID int64, question string) string {
	t.Helper()
	writer := newAppChatBlockingStreamWriter()
	req := httptest.NewRequest(http.MethodPost, fmt.Sprintf("/api/app/chat/sessions/%d/ask/stream", sessionID), strings.NewReader(fmt.Sprintf(`{"question":%q}`, question)))
	req = req.WithContext(context.WithValue(req.Context(), appContextKey{}, auth.UserInfo{ID: userID}))
	s.appChatRouter(writer, req)
	return writer.BodyString()
}

type fakeAppChatPreferenceStore struct {
	mu      sync.Mutex
	byUser  map[int64]map[string]userpreference.Preference
	onApply func()
}

func newFakeAppChatPreferenceStore() *fakeAppChatPreferenceStore {
	return &fakeAppChatPreferenceStore{byUser: make(map[int64]map[string]userpreference.Preference)}
}

func (s *fakeAppChatPreferenceStore) List(_ context.Context, userID int64) ([]userpreference.Preference, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	values := s.byUser[userID]
	result := make([]userpreference.Preference, 0, len(values))
	for _, preference := range values {
		result = append(result, preference)
	}
	sort.Slice(result, func(i, j int) bool { return result[i].Slot < result[j].Slot })
	return result, nil
}

func (s *fakeAppChatPreferenceStore) Apply(_ context.Context, userID int64, mutations []userpreference.Mutation) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.byUser[userID] == nil {
		s.byUser[userID] = make(map[string]userpreference.Preference)
	}
	for _, mutation := range mutations {
		if mutation.Upsert != nil {
			s.byUser[userID][mutation.Upsert.Slot] = *mutation.Upsert
			continue
		}
		delete(s.byUser[userID], mutation.DeleteSlot)
	}
	if s.onApply != nil {
		s.onApply()
	}
	return nil
}
