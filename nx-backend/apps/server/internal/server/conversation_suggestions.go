package server

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"strconv"
	"strings"
	"time"
	"unicode/utf8"

	"nine-xing/nx-backend/apps/server/internal/chat"
	"nine-xing/nx-backend/apps/server/internal/followups"
	"nine-xing/nx-backend/apps/server/internal/httpx"
)

type conversationSuggestionTurnReader interface {
	GetSuggestionTurn(context.Context, int64, int64, int64, string) (chat.SuggestionTurn, error)
}

type conversationSuggestionsRequest struct {
	Scene         string   `json:"scene"`
	SessionID     int64    `json:"sessionId"`
	MessageID     int64    `json:"messageId"`
	EnneagramType int      `json:"enneagramType"`
	Exclude       []string `json:"exclude"`
}

func (body conversationSuggestionsRequest) storageScene() (string, bool) {
	if body.MessageID <= 0 || body.SessionID < 0 || body.EnneagramType < 0 || body.EnneagramType > 9 || len(body.Exclude) > followups.MaxExclusions {
		return "", false
	}
	if body.Scene != "xinzhili" && body.SessionID <= 0 {
		return "", false
	}
	if body.Scene != "chat" && body.EnneagramType != 0 {
		return "", false
	}
	for _, value := range body.Exclude {
		if !utf8.ValidString(value) || utf8.RuneCountInString(value) > followups.MaxQuestionRunes || strings.ContainsAny(value, "\r\n\x00") {
			return "", false
		}
	}
	switch body.Scene {
	case "chat":
		if body.EnneagramType > 0 {
			return fmt.Sprintf("enneagram_%d", body.EnneagramType), true
		}
		return "chat", true
	case "skill":
		return "skill_chat", true
	case "xinzhili":
		return "xinzhili_voice", true
	default:
		return "", false
	}
}

func (s *Server) initConversationSuggestions() {
	s.conversationSuggestionInit.Do(func() {
		if s.conversationSuggestionSlots == nil {
			s.conversationSuggestionSlots = make(chan struct{}, 8)
		}
		if s.conversationSuggestionLimiter == nil {
			s.conversationSuggestionLimiter = newBoundedStrRateLimiter(24, time.Minute, 10_000)
		}
		if s.conversationSuggestionTimeout <= 0 {
			s.conversationSuggestionTimeout = 18 * time.Second
		}
	})
}

func (s *Server) appConversationSuggestions(w http.ResponseWriter, r *http.Request) {
	user, ok := appUserFromContext(r)
	if !ok || user.ID <= 0 {
		httpx.Fail(w, http.StatusUnauthorized, "unauthorized")
		return
	}
	if r.Method != http.MethodPost {
		httpx.Fail(w, http.StatusMethodNotAllowed, "method not allowed")
		return
	}
	var body conversationSuggestionsRequest
	decoder := json.NewDecoder(http.MaxBytesReader(w, r.Body, 16<<10))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(&body); err != nil {
		httpx.Fail(w, http.StatusBadRequest, "推荐请求参数不正确")
		return
	}
	if err := decoder.Decode(new(any)); err != io.EOF {
		httpx.Fail(w, http.StatusBadRequest, "推荐请求参数不正确")
		return
	}
	scene, valid := body.storageScene()
	if !valid {
		httpx.Fail(w, http.StatusBadRequest, "推荐请求参数不正确")
		return
	}
	s.initConversationSuggestions()
	if !s.conversationSuggestionLimiter.Allow(strconv.FormatInt(user.ID, 10), time.Now()) {
		httpx.Fail(w, http.StatusTooManyRequests, "操作太频繁，请稍后再试")
		return
	}
	select {
	case s.conversationSuggestionSlots <- struct{}{}:
		defer func() { <-s.conversationSuggestionSlots }()
	default:
		httpx.Fail(w, http.StatusServiceUnavailable, "问题推荐正在忙，请稍后重试")
		return
	}
	ctx, cancel := context.WithTimeout(r.Context(), s.conversationSuggestionTimeout)
	defer cancel()
	reader := s.conversationSuggestionReader
	if reader == nil {
		reader, _ = s.appChat.(conversationSuggestionTurnReader)
	}
	if reader == nil && s.db != nil {
		reader = chat.NewStore(s.db)
	}
	if reader == nil {
		httpx.Fail(w, http.StatusServiceUnavailable, "问题推荐暂时未就绪，请稍后重试")
		return
	}
	turn, err := reader.GetSuggestionTurn(ctx, user.ID, body.SessionID, body.MessageID, scene)
	if errors.Is(err, chat.ErrNotFound) {
		httpx.Fail(w, http.StatusNotFound, "没有找到对应的回答")
		return
	}
	if err != nil {
		httpx.Fail(w, http.StatusServiceUnavailable, "问题推荐暂时未就绪，请稍后重试")
		return
	}
	complete := s.conversationSuggestionComplete
	if complete == nil {
		complete = s.completePreferenceJSON
	}
	questions, err := followups.New(followups.CompleteFunc(complete)).Generate(ctx, followups.Turn{Question: turn.Question, Answer: turn.Answer, Scene: body.Scene}, body.Exclude)
	if err != nil {
		httpx.Fail(w, http.StatusServiceUnavailable, "问题推荐暂时没有生成成功，请重试")
		return
	}
	httpx.OK(w, map[string]any{"suggestions": questions})
}
