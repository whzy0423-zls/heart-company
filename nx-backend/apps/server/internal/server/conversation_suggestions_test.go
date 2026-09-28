package server

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"nine-xing/nx-backend/apps/server/internal/auth"
	"nine-xing/nx-backend/apps/server/internal/chat"
)

const validSuggestionsJSON = `{"suggestions":["我能怎样安排今天的练习？","怎样判断自己在进步？","遇到挫折时怎么调整？","能给我一个具体例子吗？","如何把练习用在工作中？"]}`

type suggestionTurnReaderFunc func(context.Context, int64, int64, int64, string) (chat.SuggestionTurn, error)

func (f suggestionTurnReaderFunc) GetSuggestionTurn(ctx context.Context, userID, sessionID, messageID int64, scene string) (chat.SuggestionTurn, error) {
	return f(ctx, userID, sessionID, messageID, scene)
}

func suggestionRequest(body string, authenticated bool) *http.Request {
	r := httptest.NewRequest(http.MethodPost, "/api/app/conversation-suggestions", strings.NewReader(body))
	if authenticated {
		r = r.WithContext(contextWithAppUser(r.Context(), auth.UserInfo{ID: 7}))
	}
	return r
}

func TestConversationSuggestionsReadsOwnedTurnForEachScene(t *testing.T) {
	for _, tt := range []struct {
		body, scene string
		session     int64
	}{
		{`{"scene":"chat","sessionId":42,"messageId":99,"exclude":[]}`, "chat", 42},
		{`{"scene":"chat","sessionId":42,"messageId":99,"enneagramType":9}`, "enneagram_9", 42},
		{`{"scene":"skill","sessionId":42,"messageId":99}`, "skill_chat", 42},
		{`{"scene":"xinzhili","sessionId":0,"messageId":99}`, "xinzhili_voice", 0},
	} {
		t.Run(tt.scene, func(t *testing.T) {
			reads, completions := 0, 0
			s := &Server{
				conversationSuggestionReader: suggestionTurnReaderFunc(func(_ context.Context, userID, sessionID, messageID int64, scene string) (chat.SuggestionTurn, error) {
					reads++
					if userID != 7 || sessionID != tt.session || messageID != 99 || scene != tt.scene {
						t.Fatalf("lookup=(%d,%d,%d,%s)", userID, sessionID, messageID, scene)
					}
					return chat.SuggestionTurn{SessionID: 42, Question: "我总是拖延怎么办？", Answer: "先做一个十分钟动作。"}, nil
				}),
				conversationSuggestionComplete: func(_ context.Context, system, user string, _ int) (string, error) {
					completions++
					if !strings.Contains(user, "先做一个十分钟动作。") || strings.Contains(system, "先做一个十分钟动作。") {
						t.Fatalf("wrong completion context")
					}
					return validSuggestionsJSON, nil
				},
			}
			w := httptest.NewRecorder()
			s.appConversationSuggestions(w, suggestionRequest(tt.body, true))
			var output struct {
				Data struct {
					Suggestions []string `json:"suggestions"`
				} `json:"data"`
			}
			if err := json.Unmarshal(w.Body.Bytes(), &output); err != nil {
				t.Fatal(err)
			}
			if w.Code != http.StatusOK || reads != 1 || completions != 1 || len(output.Data.Suggestions) != 5 {
				t.Fatalf("status=%d reads=%d completions=%d body=%s", w.Code, reads, completions, w.Body.String())
			}
		})
	}
}

func TestConversationSuggestionsRejectsUnownedOrWrongSceneBeforeModel(t *testing.T) {
	s := &Server{
		conversationSuggestionReader: suggestionTurnReaderFunc(func(context.Context, int64, int64, int64, string) (chat.SuggestionTurn, error) {
			return chat.SuggestionTurn{}, chat.ErrNotFound
		}),
		conversationSuggestionComplete: func(context.Context, string, string, int) (string, error) {
			t.Fatal("model called for inaccessible turn")
			return "", nil
		},
	}
	w := httptest.NewRecorder()
	s.appConversationSuggestions(w, suggestionRequest(`{"scene":"chat","sessionId":42,"messageId":99}`, true))
	if w.Code != http.StatusNotFound {
		t.Fatalf("status=%d body=%s", w.Code, w.Body.String())
	}
}

func TestConversationSuggestionsValidatesRequest(t *testing.T) {
	for _, body := range []string{
		`{"scene":"chat","sessionId":0,"messageId":99}`,
		`{"scene":"skill","sessionId":42,"messageId":99,"enneagramType":2}`,
		`{"scene":"chat","sessionId":42,"messageId":99,"enneagramType":10}`,
		`{"scene":"invalid","sessionId":42,"messageId":99}`,
		`{"scene":"chat","sessionId":42,"messageId":99,"answer":"client supplied"}`,
		`{"scene":"chat","sessionId":42,"messageId":99} {}`,
		`{"scene":"chat","sessionId":42,"messageId":99,"exclude":["` + strings.Repeat("问", 41) + `"]}`,
		`{"scene":"chat","sessionId":42,"messageId":99,"exclude":[` + strings.Repeat(`"已经显示的问题？",`, 50) + `"已经显示的问题？"]}`,
	} {
		w := httptest.NewRecorder()
		(&Server{}).appConversationSuggestions(w, suggestionRequest(body, true))
		if w.Code != http.StatusBadRequest {
			t.Fatalf("body=%s status=%d response=%s", body, w.Code, w.Body.String())
		}
	}
	w := httptest.NewRecorder()
	(&Server{}).appConversationSuggestions(w, suggestionRequest(`{}`, false))
	if w.Code != http.StatusUnauthorized {
		t.Fatalf("unauth status=%d", w.Code)
	}
}

func TestConversationSuggestionsCancelsBeforeProvider(t *testing.T) {
	s := &Server{
		conversationSuggestionReader: suggestionTurnReaderFunc(func(context.Context, int64, int64, int64, string) (chat.SuggestionTurn, error) {
			return chat.SuggestionTurn{SessionID: 42, Question: "问题", Answer: "实际回答"}, nil
		}),
		conversationSuggestionComplete: func(context.Context, string, string, int) (string, error) {
			t.Fatal("provider called after cancellation")
			return "", nil
		},
	}
	r := suggestionRequest(`{"scene":"chat","sessionId":42,"messageId":99}`, true)
	ctx, cancel := context.WithCancel(r.Context())
	cancel()
	w := httptest.NewRecorder()
	s.appConversationSuggestions(w, r.WithContext(ctx))
	if w.Code != http.StatusServiceUnavailable {
		t.Fatalf("status=%d", w.Code)
	}
}

func TestConversationSuggestionsTimeoutAndInvalidOutputOnlyFailRecommendations(t *testing.T) {
	for _, tt := range []struct {
		name     string
		complete func(context.Context, string, string, int) (string, error)
	}{
		{"timeout", func(ctx context.Context, _, _ string, _ int) (string, error) { <-ctx.Done(); return "", ctx.Err() }},
		{"invalid", func(context.Context, string, string, int) (string, error) { return `{"suggestions":[]}`, nil }},
	} {
		t.Run(tt.name, func(t *testing.T) {
			s := &Server{conversationSuggestionTimeout: time.Millisecond,
				conversationSuggestionReader: suggestionTurnReaderFunc(func(context.Context, int64, int64, int64, string) (chat.SuggestionTurn, error) {
					return chat.SuggestionTurn{SessionID: 42, Question: "问题", Answer: "已成功保存的回答"}, nil
				}),
				conversationSuggestionComplete: tt.complete,
			}
			w := httptest.NewRecorder()
			s.appConversationSuggestions(w, suggestionRequest(`{"scene":"chat","sessionId":42,"messageId":99}`, true))
			if w.Code != http.StatusServiceUnavailable {
				t.Fatalf("status=%d body=%s", w.Code, w.Body.String())
			}
		})
	}
}

func TestConversationSuggestionsConcurrencyAndRateLimit(t *testing.T) {
	s := &Server{conversationSuggestionSlots: make(chan struct{}, 1), conversationSuggestionLimiter: newBoundedStrRateLimiter(1, time.Minute, 10)}
	s.conversationSuggestionSlots <- struct{}{}
	w := httptest.NewRecorder()
	s.appConversationSuggestions(w, suggestionRequest(`{"scene":"chat","sessionId":42,"messageId":99}`, true))
	if w.Code != http.StatusServiceUnavailable {
		t.Fatalf("busy status=%d", w.Code)
	}
	w = httptest.NewRecorder()
	s.appConversationSuggestions(w, suggestionRequest(`{"scene":"chat","sessionId":42,"messageId":99}`, true))
	if w.Code != http.StatusTooManyRequests {
		t.Fatalf("rate status=%d", w.Code)
	}
}
