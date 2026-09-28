package server

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"nine-xing/nx-backend/apps/server/internal/auth"
	"nine-xing/nx-backend/apps/server/internal/chat"
	"nine-xing/nx-backend/apps/server/internal/rag"
)

func TestXinzhiliTextFollowUpIncludesCompleteSourceEvenWhenAudioWasNotDelivered(t *testing.T) {
	for _, delivered := range []string{"", "先慢慢呼吸。"} {
		t.Run(delivered, func(t *testing.T) {
			s := newSuccessfulXinzhiliVoiceServer(t)
			complete := "先慢慢呼吸。然后把注意力放在脚底，并告诉我哪一步最困难。"
			s.appChat = xinzhiliFollowUpContextStore{history: &fakeAppChatContextStore{messages: []chat.Message{
				{ID: 100, Role: "user", Content: "我现在非常紧张，应该怎么做？"},
				{ID: 101, Role: "assistant", Content: delivered},
			}}}
			s.conversationSuggestionReader = suggestionTurnReaderFunc(func(_ context.Context, userID, sessionID, messageID int64, scene string) (chat.SuggestionTurn, error) {
				if userID != 7 || sessionID != 91 || messageID != 101 || scene != "xinzhili_voice" {
					t.Fatalf("source query=%d/%d/%d/%s", userID, sessionID, messageID, scene)
				}
				return chat.SuggestionTurn{SessionID: 91, Question: "我现在非常紧张，应该怎么做？", Answer: complete}, nil
			})
			s.ragGen = generatorFunc(func(_ context.Context, input rag.GenerateInput) (string, error) {
				if len(input.History) < 2 || input.History[len(input.History)-2] != (rag.Message{Role: "user", Content: "我现在非常紧张，应该怎么做？"}) || input.History[len(input.History)-1] != (rag.Message{Role: "assistant", Content: complete}) {
					t.Fatalf("complete source missing from prompt=%+v", input.History)
				}
				return "可以先试一分钟。", nil
			})
			req := httptest.NewRequest(http.MethodPost, "/api/app/xinzhili/text/stream", strings.NewReader(`{"question":"我应该如何把注意力放在脚底？","conversationId":91,"sourceMessageId":101}`))
			req = req.WithContext(contextWithAppUser(req.Context(), auth.UserInfo{ID: 7}))
			res := httptest.NewRecorder()
			s.appXinzhiliTextTurnStream(res, req)
			if !strings.Contains(res.Body.String(), "event: done") {
				t.Fatalf("response=%d %s", res.Code, res.Body.String())
			}
		})
	}
}

func TestXinzhiliTextFollowUpRejectsUnavailableSourceBeforeGeneration(t *testing.T) {
	for _, scenario := range []string{"another owner", "another scene", "incomplete", "wrong session"} {
		t.Run(scenario, func(t *testing.T) {
			s := newSuccessfulXinzhiliVoiceServer(t)
			s.conversationSuggestionReader = suggestionTurnReaderFunc(func(_ context.Context, userID, sessionID, messageID int64, scene string) (chat.SuggestionTurn, error) {
				if userID != 7 || sessionID != 91 || scene != "xinzhili_voice" {
					t.Fatalf("untrusted source lookup=%d/%d/%s", userID, sessionID, scene)
				}
				if scenario == "wrong session" {
					return chat.SuggestionTurn{SessionID: 92, Question: "原问题", Answer: "原回答"}, nil
				}
				return chat.SuggestionTurn{}, chat.ErrNotFound
			})
			s.ragGen = generatorFunc(func(context.Context, rag.GenerateInput) (string, error) {
				t.Fatal("rejected source must not call model")
				return "", nil
			})
			req := httptest.NewRequest(http.MethodPost, "/api/app/xinzhili/text/stream", strings.NewReader(`{"question":"继续","conversationId":91,"sourceMessageId":101}`))
			req = req.WithContext(contextWithAppUser(req.Context(), auth.UserInfo{ID: 7}))
			res := httptest.NewRecorder()
			s.appXinzhiliTextTurnStream(res, req)
			if res.Code != http.StatusNotFound {
				t.Fatalf("response=%d %s", res.Code, res.Body.String())
			}
		})
	}
}

type xinzhiliFollowUpContextStore struct {
	appChatStore
	history *fakeAppChatContextStore
}

func (s xinzhiliFollowUpContextStore) GetConversationState(ctx context.Context, id int64) (chat.ConversationState, error) {
	return s.history.GetConversationState(ctx, id)
}
func (s xinzhiliFollowUpContextStore) ListMessagesAfter(ctx context.Context, id, after int64) ([]chat.Message, error) {
	return s.history.ListMessagesAfter(ctx, id, after)
}
func (s xinzhiliFollowUpContextStore) ListRecentMessages(ctx context.Context, id int64, limit int) ([]chat.Message, error) {
	return s.history.ListRecentMessages(ctx, id, limit)
}
func (s xinzhiliFollowUpContextStore) UpdateConversationSummary(ctx context.Context, id, expected int64, summary string, through int64) (bool, error) {
	return s.history.UpdateConversationSummary(ctx, id, expected, summary, through)
}

func TestXinzhiliTextFollowUpUsesVoiceKnowledgeRuntimeWithoutASR(t *testing.T) {
	s := newSuccessfulXinzhiliVoiceServer(t)
	s.xinzhiliTranscribe = func(context.Context, []byte, string) (string, error) {
		t.Fatal("text follow-up must not transcribe or require audio")
		return "", nil
	}
	var question string
	s.ragGen = generatorFunc(func(_ context.Context, input rag.GenerateInput) (string, error) {
		question = input.Question
		return "先慢慢呼吸。", nil
	})
	req := httptest.NewRequest(http.MethodPost, "/api/app/xinzhili/text/stream", strings.NewReader(`{"question":"怎样慢慢呼吸？"}`))
	req = req.WithContext(contextWithAppUser(req.Context(), auth.UserInfo{ID: 7}))
	res := httptest.NewRecorder()
	s.appXinzhiliTextTurnStream(res, req)
	if question != "怎样慢慢呼吸？" || !strings.Contains(res.Body.String(), "event: done") || !strings.Contains(res.Body.String(), `"sessionId":91`) {
		t.Fatalf("question=%q response=%d %s", question, res.Code, res.Body.String())
	}
}

func TestXinzhiliTextFollowUpRejectsInvalidInput(t *testing.T) {
	for _, input := range []string{`{"question":""}`, `{"question":"ok","conversationId":-1}`, `{"question":"ok","conversationId":4}`, `{"question":"ok","cardId":-1}`, `{"question":"ok"} trailing`} {
		req := httptest.NewRequest(http.MethodPost, "/api/app/xinzhili/text/stream", strings.NewReader(input))
		req = req.WithContext(contextWithAppUser(req.Context(), auth.UserInfo{ID: 7}))
		res := httptest.NewRecorder()
		newSuccessfulXinzhiliVoiceServer(t).appXinzhiliTextTurnStream(res, req)
		if res.Code != http.StatusBadRequest {
			t.Fatalf("input=%s status=%d body=%s", input, res.Code, res.Body.String())
		}
	}
}
