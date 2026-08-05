package server

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log"
	"net/http"
	"strconv"
	"strings"
	"sync"
	"sync/atomic"
	"time"
	"unicode/utf8"

	"nine-xing/nx-backend/apps/server/internal/answerhygiene"
	"nine-xing/nx-backend/apps/server/internal/chat"
	"nine-xing/nx-backend/apps/server/internal/httpx"
	"nine-xing/nx-backend/apps/server/internal/quiz"
	"nine-xing/nx-backend/apps/server/internal/rag"
	"nine-xing/nx-backend/apps/server/internal/userpreference"
)

const appChatHistoryLimit = 12

const appChatFallbackHistoryLimit = 20

var (
	errInvalidAppChatTier            = errors.New("invalid app chat tier")
	errAppChatTierRequiresMembership = errors.New("app chat tier requires membership")
)

func resolveAppChatTier(requested, memberLevel string) (string, error) {
	tier := strings.ToLower(strings.TrimSpace(requested))
	if tier == "" {
		tier = "basic"
	}
	if tier != "basic" && tier != "deep" && tier != "companion" {
		return "", errInvalidAppChatTier
	}
	if tier != "basic" && (strings.TrimSpace(memberLevel) == "" || strings.EqualFold(strings.TrimSpace(memberLevel), "free")) {
		return "", errAppChatTierRequiresMembership
	}
	return tier, nil
}

func (s *Server) appChatTierForUser(ctx context.Context, appUserID int64, requested string) (string, error) {
	tier, err := resolveAppChatTier(requested, "free")
	if err == nil || errors.Is(err, errInvalidAppChatTier) {
		return tier, err
	}
	if s.appUsers == nil {
		return "", errors.New("app user store unavailable")
	}
	user, err := s.appUsers.FindByID(ctx, appUserID)
	if err != nil {
		return "", err
	}
	return resolveAppChatTier(requested, user.MemberLevel)
}

func failAppChatTier(w http.ResponseWriter, err error) {
	switch {
	case errors.Is(err, errInvalidAppChatTier):
		httpx.Fail(w, http.StatusBadRequest, "不支持的对话模式")
	case errors.Is(err, errAppChatTierRequiresMembership):
		httpx.Fail(w, http.StatusForbidden, "当前对话模式需要会员权益")
	default:
		httpx.Fail(w, http.StatusInternalServerError, "会员权益读取失败，请重试")
	}
}

func buildAppChatConversationCard(card quiz.Card) rag.ConversationCard {
	profile := strings.TrimSpace(string(card.Profile))
	if runes := []rune(profile); len(runes) > 2000 {
		profile = string(runes[:2000])
	}
	return rag.ConversationCard{CardType: card.CardType, Name: card.Name, Relation: card.Relation, MainType: card.MainType, WingType: card.WingType, Profile: profile}
}

func normalizeAppChatProfileForConversationCard(profile rag.UserProfile, card rag.ConversationCard) (rag.UserProfile, rag.ConversationCard) {
	if card.MainType < 1 || card.MainType > 9 {
		profile.MainType = 0
	}
	return profile, card
}

func appChatModelIdentityAnswer(question string) (rag.Answer, bool) {
	question = strings.TrimSpace(question)
	if question == "" || utf8.RuneCountInString(question) > 300 || !rag.IsModelIdentityQuestion(question) {
		return rag.Answer{}, false
	}
	return rag.Answer{
		Answer:      rag.ModelIdentityReply,
		Sources:     []rag.Source{},
		Suggestions: []string{},
	}, true
}

func (s *Server) appChatProfilesForCard(ctx context.Context, appUserID, cardID int64) (rag.UserProfile, rag.ConversationCard) {
	if s.appChatProfilesForCardOverride != nil {
		profile, cardContext := s.appChatProfilesForCardOverride(ctx, appUserID, cardID)
		return normalizeAppChatProfileForConversationCard(profile, cardContext)
	}
	profile := rag.UserProfile{}
	if s.appUsers != nil {
		if appUser, err := s.appUsers.FindByID(ctx, appUserID); err == nil {
			profile.Nickname = appUser.Nickname
		}
	}
	cardContext := rag.ConversationCard{}
	if s.quiz != nil {
		if primary, err := s.quiz.PrimaryCard(ctx, appUserID); err == nil {
			profile.MainType = primary.MainType
		}
		if card, err := s.quiz.GetCard(ctx, appUserID, cardID); err == nil {
			cardContext = buildAppChatConversationCard(card)
		}
	}
	return normalizeAppChatProfileForConversationCard(profile, cardContext)
}

const (
	defaultAppChatHeartbeatInterval = 15 * time.Second
	defaultAppChatProviderIdle      = 20 * time.Second
	defaultAppChatPostSaveTimeout   = 2 * time.Second
	appChatStreamEventBuffer        = 16
)

type appChatStreamEventKind uint8

const (
	appChatStreamProviderStarted appChatStreamEventKind = iota + 1
	appChatStreamProviderProgress
	appChatStreamDelta
	appChatStreamDone
	appChatStreamError
	appChatStreamPersistenceStarted
)

type appChatStreamEvent struct {
	kind        appChatStreamEventKind
	delta       string
	writeResult chan error
	response    askResponse
	publicError string
	errorPhase  string
}

type appChatStreamLifecycle struct {
	phase atomic.Int32
}

const (
	appChatStreamGenerating int32 = iota
	appChatStreamPersisting
	appChatStreamStopped
)

func (l *appChatStreamLifecycle) beginPersistence() bool {
	return l.phase.CompareAndSwap(appChatStreamGenerating, appChatStreamPersisting)
}

func (l *appChatStreamLifecycle) stopBeforePersistence() bool {
	return l.phase.CompareAndSwap(appChatStreamGenerating, appChatStreamStopped)
}

func (l *appChatStreamLifecycle) persistenceStarted() bool {
	return l.phase.Load() == appChatStreamPersisting
}

type appChatPromptContext struct {
	Summary                     string
	History                     []rag.Message
	SummaryThroughMessageID     int64
	ShouldPersistUpdatedSummary bool
}

type appChatContextStore interface {
	GetConversationState(ctx context.Context, sessionID int64) (chat.ConversationState, error)
	ListMessagesAfter(ctx context.Context, sessionID, afterMessageID int64) ([]chat.Message, error)
	ListRecentMessages(ctx context.Context, sessionID int64, limit int) ([]chat.Message, error)
	UpdateConversationSummary(ctx context.Context, sessionID, expectedThroughMessageID int64, summary string, throughMessageID int64) (bool, error)
}

type appChatStore interface {
	appChatContextStore
	ListSessions(ctx context.Context, appUserID int64) ([]chat.Session, error)
	GetOrCreateSession(ctx context.Context, appUserID, cardID int64) (chat.Session, error)
	GetSession(ctx context.Context, appUserID, sessionID int64) (chat.Session, error)
	ListMessages(ctx context.Context, sessionID int64) ([]chat.Message, error)
	SavePair(ctx context.Context, sessionID int64, question, answer string, sources json.RawMessage) (int64, error)
	SetFeedback(ctx context.Context, appUserID, messageID int64, feedback string) error
	ToggleFavorite(ctx context.Context, appUserID, messageID int64) (bool, error)
	ListFavorites(ctx context.Context, appUserID, cardID int64) ([]chat.FavoriteItem, error)
	SearchMessages(ctx context.Context, appUserID, cardID int64, keyword string) ([]chat.SearchResult, error)
}

type appChatPreferenceStore interface {
	List(ctx context.Context, userID int64) ([]userpreference.Preference, error)
	Apply(ctx context.Context, userID int64, mutations []userpreference.Mutation) error
}

type appChatPreferenceExtractor interface {
	Extract(ctx context.Context, message string) userpreference.Extraction
}

type appChatPreferenceTurnState struct {
	mu           sync.Mutex
	nextTicket   atomic.Uint64
	latestBySlot map[string]uint64
	active       atomic.Int32
	pending      atomic.Int32
}

type appChatPreferenceTurn struct {
	userID int64
	state  *appChatPreferenceTurnState
	ticket uint64
}

type appChatPreferenceFallbackReservation struct {
	server   *Server
	turn     appChatPreferenceTurn
	question string
	claim    sync.Once
}

var (
	errAppChatPreferenceSave  = errors.New("偏好保存失败，请重试")
	errAppChatPreferenceRead  = errors.New("偏好读取失败，请重试")
	errAppChatPreferenceStale = errors.New("请求已被新的消息更新，请重试")
)

func compactAppChatContext(ctx context.Context, previousSummary string, messages []chat.Message, summarizer rag.ConversationSummarizer) appChatPromptContext {
	validChatMessages := validAppChatMessages(messages)
	validMessages := appChatHistoryFromMessages(validChatMessages)
	if len(validMessages) <= appChatFallbackHistoryLimit {
		return appChatPromptContext{Summary: strings.TrimSpace(previousSummary), History: validMessages}
	}
	if summarizer == nil {
		return appChatPromptContext{
			Summary: strings.TrimSpace(previousSummary),
			History: validMessages[len(validMessages)-appChatFallbackHistoryLimit:],
		}
	}

	oldCount := len(validMessages) - appChatHistoryLimit
	updatedSummary, err := summarizer.SummarizeConversation(ctx, strings.TrimSpace(previousSummary), validMessages[:oldCount])
	updatedSummary = strings.TrimSpace(updatedSummary)
	if err != nil || updatedSummary == "" {
		return appChatPromptContext{
			Summary: strings.TrimSpace(previousSummary),
			History: validMessages[len(validMessages)-appChatFallbackHistoryLimit:],
		}
	}
	throughID := validChatMessages[oldCount-1].ID
	return appChatPromptContext{
		Summary:                     updatedSummary,
		History:                     validMessages[oldCount:],
		SummaryThroughMessageID:     throughID,
		ShouldPersistUpdatedSummary: true,
	}
}

func buildAppChatPromptContext(ctx context.Context, sessionID int64, store appChatContextStore, summarizer rag.ConversationSummarizer) appChatPromptContext {
	state, err := store.GetConversationState(ctx, sessionID)
	if err != nil {
		return fallbackAppChatPromptContext(ctx, sessionID, store, "")
	}
	messages, err := store.ListMessagesAfter(ctx, sessionID, state.SummaryThroughMessageID)
	if err != nil {
		return fallbackAppChatPromptContext(ctx, sessionID, store, state.Summary)
	}
	promptContext := compactAppChatContext(ctx, state.Summary, messages, summarizer)
	if promptContext.ShouldPersistUpdatedSummary {
		_, _ = store.UpdateConversationSummary(
			ctx,
			sessionID,
			state.SummaryThroughMessageID,
			promptContext.Summary,
			promptContext.SummaryThroughMessageID,
		)
	}
	return promptContext
}

func fallbackAppChatPromptContext(ctx context.Context, sessionID int64, store appChatContextStore, summary string) appChatPromptContext {
	messages, err := store.ListRecentMessages(ctx, sessionID, appChatFallbackHistoryLimit)
	if err != nil {
		return appChatPromptContext{Summary: strings.TrimSpace(summary)}
	}
	return appChatPromptContext{Summary: strings.TrimSpace(summary), History: appChatHistoryFromMessages(messages)}
}

// appChatSessions GET /api/app/chat/sessions — list user's sessions.
func (s *Server) appChatSessions(w http.ResponseWriter, r *http.Request) {
	userInfo, ok := appUserFromContext(r)
	if !ok {
		httpx.Fail(w, http.StatusUnauthorized, "unauthorized")
		return
	}
	sessions, err := s.appChat.ListSessions(r.Context(), userInfo.ID)
	if err != nil {
		httpx.Fail(w, http.StatusInternalServerError, "server error")
		return
	}
	httpx.OK(w, sessions)
}

// appChatGetOrCreate POST /api/app/chat/sessions — get or create session for a card.
func (s *Server) appChatGetOrCreate(w http.ResponseWriter, r *http.Request) {
	userInfo, ok := appUserFromContext(r)
	if !ok {
		httpx.Fail(w, http.StatusUnauthorized, "unauthorized")
		return
	}
	var body struct {
		CardID int64 `json:"cardId"`
	}
	if err := json.NewDecoder(io.LimitReader(r.Body, 8<<10)).Decode(&body); err != nil || body.CardID == 0 {
		httpx.Fail(w, http.StatusBadRequest, "cardId required")
		return
	}
	if _, err := s.quiz.GetCard(r.Context(), userInfo.ID, body.CardID); err != nil {
		httpx.Fail(w, http.StatusNotFound, "card not found")
		return
	}
	sess, err := s.appChat.GetOrCreateSession(r.Context(), userInfo.ID, body.CardID)
	if err != nil {
		httpx.Fail(w, http.StatusInternalServerError, "server error")
		return
	}
	httpx.OK(w, sess)
}

// appChatMessages GET /api/app/chat/sessions/{id}/messages — list messages in session.
func (s *Server) appChatMessages(w http.ResponseWriter, r *http.Request) {
	userInfo, ok := appUserFromContext(r)
	if !ok {
		httpx.Fail(w, http.StatusUnauthorized, "unauthorized")
		return
	}
	idText := strings.Trim(strings.TrimPrefix(r.URL.Path, "/api/app/chat/sessions/"), "/messages")
	sessionID, err := strconv.ParseInt(idText, 10, 64)
	if err != nil || sessionID == 0 {
		httpx.Fail(w, http.StatusBadRequest, "invalid session id")
		return
	}
	// verify ownership
	if _, err := s.appChat.GetSession(r.Context(), userInfo.ID, sessionID); err != nil {
		httpx.Fail(w, http.StatusNotFound, "session not found")
		return
	}
	msgs, err := s.appChat.ListMessages(r.Context(), sessionID)
	if err != nil {
		httpx.Fail(w, http.StatusInternalServerError, "server error")
		return
	}
	httpx.OK(w, msgs)
}

// appChatAsk POST /api/app/chat/sessions/{id}/ask — send question, get AI answer, persist pair.
func (s *Server) appChatAsk(w http.ResponseWriter, r *http.Request) {
	userInfo, ok := appUserFromContext(r)
	if !ok {
		httpx.Fail(w, http.StatusUnauthorized, "unauthorized")
		return
	}
	sessionID, ok := appChatSessionIDFromPath(r.URL.Path, "/ask")
	if !ok {
		httpx.Fail(w, http.StatusBadRequest, "invalid session id")
		return
	}

	if !s.chatLimiter.Allow(userInfo.ID, time.Now()) {
		httpx.Fail(w, http.StatusTooManyRequests, "请求过于频繁，请稍后再试")
		return
	}

	sess, err := s.appChat.GetSession(r.Context(), userInfo.ID, sessionID)
	if err != nil {
		httpx.Fail(w, http.StatusNotFound, "session not found")
		return
	}

	var body struct {
		Question string        `json:"question"`
		History  []rag.Message `json:"history"`
	}
	if err := json.NewDecoder(io.LimitReader(r.Body, 32<<10)).Decode(&body); err != nil || strings.TrimSpace(body.Question) == "" {
		httpx.Fail(w, http.StatusBadRequest, "question required")
		return
	}
	if answer, ok := appChatModelIdentityAnswer(body.Question); ok {
		_, chatTimeout := s.chatRuntime()
		ctx, cancel := context.WithTimeout(r.Context(), chatTimeout)
		defer cancel()
		answer.Answer = answerhygiene.Clean(body.Question, answer.Answer)
		sourcesJSON, _ := json.Marshal(answer.Sources)
		messageID, saveErr := s.appChat.SavePair(ctx, sessionID, body.Question, answer.Answer, sourcesJSON)
		if saveErr != nil {
			httpx.Fail(w, http.StatusInternalServerError, "回答保存失败，请重试")
			return
		}
		s.rememberChatAnswer(ctx, userInfo.ID, sess.CardID, body.Question, answer.Answer)
		if messageID > 0 {
			s.recordAppProfileEvidenceAsync(userInfo.ID, sess.CardID, "chat", messageID, body.Question)
		}
		httpx.OK(w, askResponse{Answer: answer, MessageID: messageID})
		return
	}
	preferenceExtraction := userpreference.Extract(body.Question)
	preferenceTurn := appChatPreferenceTurn{userID: userInfo.ID}
	if len(preferenceExtraction.Mutations) > 0 || userpreference.NeedsLLMFallback(body.Question) {
		preferenceTurn = s.beginAppChatPreferenceTurn(userInfo.ID)
	}
	defer s.finishAppChatPreferenceTurn(preferenceTurn)

	generator, chatTimeout := s.chatRuntime()
	ctx, cancel := context.WithTimeout(r.Context(), chatTimeout)
	defer cancel()
	preferences, directives, err := s.prepareAppChatPreferences(ctx, preferenceTurn, preferenceExtraction)
	if err != nil {
		httpx.Fail(w, http.StatusInternalServerError, appChatPreferencePublicError(err))
		return
	}

	docs, _ := s.retrieveAppDocsForQuery(ctx, body.Question, 6)
	profile, conversationCard := s.appChatProfilesForCard(ctx, userInfo.ID, sess.CardID)
	if memories, err := s.appChatMemoriesForPrompt(ctx, userInfo.ID, sess.CardID, 6); err == nil {
		profile.Memories = memories
	}
	promptContext := s.appChatContextForPrompt(ctx, sessionID, generator)

	ans, err := rag.NewService(docs, rag.WithGenerator(generator)).Ask(ctx, rag.AskInput{
		History:             promptContext.History,
		ConversationSummary: promptContext.Summary,
		Question:            body.Question,
		UserProfile:         profile,
		ConversationCard:    conversationCard,
		UserPreferences:     preferences,
		CurrentDirectives:   directives,
	})
	if err != nil {
		httpx.Fail(w, http.StatusInternalServerError, "回答生成失败，请重试")
		return
	}
	ans.Answer = answerhygiene.Clean(body.Question, ans.Answer)

	sourcesJSON, _ := json.Marshal(ans.Sources)
	messageID, saveErr := s.appChat.SavePair(ctx, sessionID, body.Question, ans.Answer, sourcesJSON)
	if saveErr == nil {
		s.scheduleAppChatPreferenceFallback(preferenceTurn, body.Question)
	}
	s.rememberChatAnswer(ctx, userInfo.ID, sess.CardID, body.Question, ans.Answer)
	if messageID > 0 {
		s.recordAppProfileEvidenceAsync(userInfo.ID, sess.CardID, "chat", messageID, body.Question)
	}

	httpx.OK(w, askResponse{Answer: ans, MessageID: messageID})
}

// appChatAskStream POST /api/app/chat/sessions/{id}/ask/stream — send question, stream AI answer, persist pair.
func (s *Server) appChatAskStream(w http.ResponseWriter, r *http.Request) {
	userInfo, ok := appUserFromContext(r)
	if !ok {
		httpx.Fail(w, http.StatusUnauthorized, "unauthorized")
		return
	}
	sessionID, ok := appChatSessionIDFromPath(r.URL.Path, "/ask/stream")
	if !ok {
		httpx.Fail(w, http.StatusBadRequest, "invalid session id")
		return
	}
	flusher, ok := w.(http.Flusher)
	if !ok {
		httpx.Fail(w, http.StatusInternalServerError, "streaming unsupported")
		return
	}

	if s.chatLimiter != nil && !s.chatLimiter.Allow(userInfo.ID, time.Now()) {
		httpx.Fail(w, http.StatusTooManyRequests, "请求过于频繁，请稍后再试")
		return
	}

	sess, err := s.appChat.GetSession(r.Context(), userInfo.ID, sessionID)
	if err != nil {
		httpx.Fail(w, http.StatusNotFound, "session not found")
		return
	}

	var body struct {
		Question string        `json:"question"`
		History  []rag.Message `json:"history"`
	}
	if err := json.NewDecoder(io.LimitReader(r.Body, 32<<10)).Decode(&body); err != nil || strings.TrimSpace(body.Question) == "" {
		httpx.Fail(w, http.StatusBadRequest, "question required")
		return
	}
	fixedAnswer, isModelIdentity := appChatModelIdentityAnswer(body.Question)
	preferenceExtraction := userpreference.Extraction{}
	preferenceTurn := appChatPreferenceTurn{userID: userInfo.ID}
	if !isModelIdentity {
		preferenceExtraction = userpreference.Extract(body.Question)
		if len(preferenceExtraction.Mutations) > 0 || userpreference.NeedsLLMFallback(body.Question) {
			preferenceTurn = s.beginAppChatPreferenceTurn(userInfo.ID)
		}
	}
	defer s.finishAppChatPreferenceTurn(preferenceTurn)

	generator, chatTimeout := s.chatRuntime()
	ctx, cancel := context.WithTimeout(r.Context(), chatTimeout)
	defer cancel()
	streamStartedAt := time.Now()
	w.Header().Set("Content-Type", "text/event-stream; charset=utf-8")
	w.Header().Set("Cache-Control", "no-cache")
	w.Header().Set("Connection", "keep-alive")
	w.Header().Set("X-Accel-Buffering", "no")
	if err := writeAppChatSSEComment(w, flusher, "connected"); err != nil {
		logAppChatStreamTiming("error", userInfo.ID, sessionID, streamStartedAt, "connected_write")
		return
	}
	logAppChatStreamTiming("connected", userInfo.ID, sessionID, streamStartedAt, "")

	events := make(chan appChatStreamEvent, appChatStreamEventBuffer)
	lifecycle := &appChatStreamLifecycle{}
	go s.runAppChatStreamPipeline(ctx, events, appChatStreamPipelineInput{
		userID:               userInfo.ID,
		sessionID:            sessionID,
		cardID:               sess.CardID,
		question:             body.Question,
		preferenceTurn:       preferenceTurn,
		preferenceExtraction: preferenceExtraction,
		generator:            generator,
		lifecycle:            lifecycle,
		fixedAnswer:          fixedAnswer,
		isModelIdentity:      isModelIdentity,
	})
	s.pumpAppChatStream(ctx, cancel, r.Context(), w, flusher, events, lifecycle, userInfo.ID, sessionID, streamStartedAt, chatTimeout)
}

type appChatStreamPipelineInput struct {
	userID               int64
	sessionID            int64
	cardID               int64
	question             string
	preferenceTurn       appChatPreferenceTurn
	preferenceExtraction userpreference.Extraction
	generator            rag.Generator
	lifecycle            *appChatStreamLifecycle
	fixedAnswer          rag.Answer
	isModelIdentity      bool
}

func (s *Server) runAppChatStreamPipeline(ctx context.Context, events chan<- appChatStreamEvent, input appChatStreamPipelineInput) {
	defer close(events)
	send := func(event appChatStreamEvent) bool {
		if ctx.Err() != nil {
			return false
		}
		select {
		case events <- event:
			return true
		case <-ctx.Done():
			return false
		}
	}

	var ans rag.Answer
	if input.isModelIdentity {
		if !send(appChatStreamEvent{kind: appChatStreamProviderStarted}) {
			return
		}
		writeResult := make(chan error, 1)
		if !send(appChatStreamEvent{kind: appChatStreamDelta, delta: input.fixedAnswer.Answer, writeResult: writeResult}) {
			return
		}
		select {
		case err := <-writeResult:
			if err != nil {
				return
			}
		case <-ctx.Done():
			return
		}
		ans = input.fixedAnswer
	} else {
		preferences, directives, err := s.prepareAppChatPreferences(ctx, input.preferenceTurn, input.preferenceExtraction)
		if err != nil {
			send(appChatStreamEvent{kind: appChatStreamError, publicError: appChatPreferencePublicError(err), errorPhase: "preferences"})
			return
		}

		docs, _ := s.retrieveAppDocsForQuery(ctx, input.question, 6)
		profile, conversationCard := s.appChatProfilesForCard(ctx, input.userID, input.cardID)
		if memories, err := s.appChatMemoriesForPrompt(ctx, input.userID, input.cardID, 6); err == nil {
			profile.Memories = memories
		}
		promptContext := s.appChatContextForPrompt(ctx, input.sessionID, input.generator)
		if !send(appChatStreamEvent{kind: appChatStreamProviderStarted}) {
			return
		}

		var sentenceBuffer answerhygiene.SentenceBuffer
		emittedAnswer := false
		emitSafeSentences := func(sentences []string) error {
			for _, sentence := range sentences {
				cleaned := answerhygiene.Clean(input.question, sentence)
				if cleaned == answerhygiene.NeutralDirectAnswerFallback {
					continue
				}
				if emittedAnswer {
					cleaned = "\n" + cleaned
				}
				writeResult := make(chan error, 1)
				if !send(appChatStreamEvent{kind: appChatStreamDelta, delta: cleaned, writeResult: writeResult}) {
					if err := ctx.Err(); err != nil {
						return err
					}
					return context.Canceled
				}
				select {
				case err := <-writeResult:
					if err != nil {
						return err
					}
					emittedAnswer = true
				case <-ctx.Done():
					return ctx.Err()
				}
			}
			return nil
		}

		ans, err = rag.NewService(docs, rag.WithGenerator(input.generator)).AskStream(ctx, rag.AskInput{
			History:             promptContext.History,
			ConversationSummary: promptContext.Summary,
			Question:            input.question,
			UserProfile:         profile,
			ConversationCard:    conversationCard,
			UserPreferences:     preferences,
			CurrentDirectives:   directives,
		}, func(delta string) error {
			if delta == "" {
				return nil
			}
			if !send(appChatStreamEvent{kind: appChatStreamProviderProgress}) {
				if err := ctx.Err(); err != nil {
					return err
				}
				return context.Canceled
			}
			return emitSafeSentences(sentenceBuffer.Push(delta))
		})
		if err != nil {
			send(appChatStreamEvent{kind: appChatStreamError, publicError: "回答生成失败，请重试", errorPhase: "provider"})
			return
		}
		if err := emitSafeSentences(sentenceBuffer.Flush()); err != nil {
			return
		}
		ans.Answer = answerhygiene.Clean(input.question, ans.Answer)
		if !emittedAnswer {
			writeResult := make(chan error, 1)
			if !send(appChatStreamEvent{kind: appChatStreamDelta, delta: ans.Answer, writeResult: writeResult}) {
				return
			}
			select {
			case err := <-writeResult:
				if err != nil {
					return
				}
			case <-ctx.Done():
				return
			}
		}
	}
	if ctx.Err() != nil {
		return
	}
	ans.Answer = answerhygiene.Clean(input.question, ans.Answer)
	if s.chatPersistHook != nil {
		s.chatPersistHook()
	}

	sourcesJSON, _ := json.Marshal(ans.Sources)
	// Reserve the original turn state before SavePair can outlive the handler.
	// This keeps later same-user turns in the same ticket-ordering domain.
	fallback := s.reserveAppChatPreferenceFallback(input.preferenceTurn, input.question)
	// Atomically arbitrate with total/idle timeout before entering SavePair's
	// may-commit window. Once persistence wins, the pump waits for its terminal.
	if !input.lifecycle.beginPersistence() {
		fallback.release()
		return
	}
	events <- appChatStreamEvent{kind: appChatStreamPersistenceStarted}
	messageID, saveErr := s.appChat.SavePair(ctx, input.sessionID, input.question, ans.Answer, sourcesJSON)
	if saveErr != nil {
		fallback.release()
		events <- appChatStreamEvent{kind: appChatStreamError, publicError: "回答保存失败，请重试", errorPhase: "save"}
		return
	}
	// Start the reserved fallback without waiting on the per-user mutation lock,
	// then publish the committed terminal immediately.
	fallback.start()
	events <- appChatStreamEvent{
		kind:     appChatStreamDone,
		response: askResponse{Answer: ans, MessageID: messageID},
	}

	postSaveCtx, cancelPostSave := context.WithTimeout(context.Background(), defaultAppChatPostSaveTimeout)
	s.rememberChatAnswer(postSaveCtx, input.userID, input.cardID, input.question, ans.Answer)
	cancelPostSave()
	if messageID > 0 {
		s.recordAppProfileEvidenceAsync(input.userID, input.cardID, "chat", messageID, input.question)
	}
}

func (s *Server) pumpAppChatStream(
	ctx context.Context,
	cancel context.CancelFunc,
	requestCtx context.Context,
	w io.Writer,
	flusher http.Flusher,
	events <-chan appChatStreamEvent,
	lifecycle *appChatStreamLifecycle,
	userID, sessionID int64,
	startedAt time.Time,
	totalTimeout time.Duration,
) {
	heartbeatInterval, providerIdleTimeout := s.appChatStreamTiming(totalTimeout)
	heartbeat := time.NewTicker(heartbeatInterval)
	defer heartbeat.Stop()

	var idleTimer *time.Timer
	var idle <-chan time.Time
	defer func() {
		if idleTimer != nil {
			idleTimer.Stop()
		}
	}()
	resetIdle := func() {
		if idleTimer == nil {
			idleTimer = time.NewTimer(providerIdleTimeout)
			idle = idleTimer.C
			return
		}
		if !idleTimer.Stop() {
			select {
			case <-idleTimer.C:
			default:
			}
		}
		idleTimer.Reset(providerIdleTimeout)
	}
	stopIdle := func() {
		idle = nil
		if idleTimer == nil {
			return
		}
		if !idleTimer.Stop() {
			select {
			case <-idleTimer.C:
			default:
			}
		}
	}
	stopBeforePersistenceAndCancel := func() {
		lifecycle.stopBeforePersistence()
		cancel()
	}

	firstDelta := true
	persistenceStarted := false
	handleEvent := func(event appChatStreamEvent, ok bool) bool {
		if requestCtx.Err() != nil {
			stopBeforePersistenceAndCancel()
			logAppChatStreamTiming("canceled", userID, sessionID, startedAt, "client")
			return true
		}
		if !ok {
			if ctx.Err() != nil {
				_ = writeAppChatSSE(w, flusher, "error", map[string]string{"message": "回答生成超时，请重试"})
				logAppChatStreamTiming("error", userID, sessionID, startedAt, "total_timeout")
				return true
			}
			if writeAppChatSSE(w, flusher, "error", map[string]string{"message": "回答生成失败，请重试"}) != nil {
				stopBeforePersistenceAndCancel()
			}
			logAppChatStreamTiming("error", userID, sessionID, startedAt, "worker_closed")
			return true
		}
		switch event.kind {
		case appChatStreamProviderStarted:
			resetIdle()
		case appChatStreamProviderProgress:
			resetIdle()
		case appChatStreamPersistenceStarted:
			persistenceStarted = true
			stopIdle()
		case appChatStreamDelta:
			if event.delta == "" {
				if event.writeResult != nil {
					event.writeResult <- nil
				}
				return false
			}
			if err := writeAppChatSSE(w, flusher, "delta", map[string]string{"content": event.delta}); err != nil {
				stopBeforePersistenceAndCancel()
				if event.writeResult != nil {
					event.writeResult <- err
				}
				logAppChatStreamTiming("error", userID, sessionID, startedAt, "delta_write")
				return true
			}
			if event.writeResult != nil {
				event.writeResult <- nil
			}
			resetIdle()
			if firstDelta {
				firstDelta = false
				logAppChatStreamTiming("first_delta", userID, sessionID, startedAt, "")
			}
		case appChatStreamDone:
			if err := writeAppChatSSE(w, flusher, "done", event.response); err != nil {
				stopBeforePersistenceAndCancel()
				logAppChatStreamTiming("error", userID, sessionID, startedAt, "done_write")
				return true
			}
			logAppChatStreamTiming("completed", userID, sessionID, startedAt, "")
			return true
		case appChatStreamError:
			if err := writeAppChatSSE(w, flusher, "error", map[string]string{"message": event.publicError}); err != nil {
				stopBeforePersistenceAndCancel()
			}
			logAppChatStreamTiming("error", userID, sessionID, startedAt, event.errorPhase)
			return true
		}
		return false
	}
	drainEvents := func() bool {
		for {
			select {
			case event, ok := <-events:
				if handleEvent(event, ok) {
					return true
				}
			default:
				return false
			}
		}
	}
	totalDone := ctx.Done()
	requestDone := requestCtx.Done()

	for {
		if requestCtx.Err() != nil {
			stopBeforePersistenceAndCancel()
			logAppChatStreamTiming("canceled", userID, sessionID, startedAt, "client")
			return
		}
		if drainEvents() {
			return
		}
		select {
		case event, ok := <-events:
			if handleEvent(event, ok) {
				return
			}
		case <-heartbeat.C:
			if err := writeAppChatSSEComment(w, flusher, "ping"); err != nil {
				stopBeforePersistenceAndCancel()
				logAppChatStreamTiming("error", userID, sessionID, startedAt, "heartbeat_write")
				return
			}
		case <-idle:
			if drainEvents() {
				return
			}
			if persistenceStarted || lifecycle.persistenceStarted() {
				persistenceStarted = true
				stopIdle()
				continue
			}
			if !lifecycle.stopBeforePersistence() {
				continue
			}
			cancel()
			_ = writeAppChatSSE(w, flusher, "error", map[string]string{"message": "回答生成超时，请重试"})
			logAppChatStreamTiming("idle", userID, sessionID, startedAt, "provider")
			return
		case <-requestDone:
			stopBeforePersistenceAndCancel()
			logAppChatStreamTiming("canceled", userID, sessionID, startedAt, "client")
			return
		case <-totalDone:
			if drainEvents() {
				return
			}
			if requestCtx.Err() != nil {
				logAppChatStreamTiming("canceled", userID, sessionID, startedAt, "client")
				return
			}
			if persistenceStarted || lifecycle.persistenceStarted() {
				persistenceStarted = true
				stopIdle()
				totalDone = nil
				continue
			}
			if !lifecycle.stopBeforePersistence() {
				totalDone = nil
				continue
			}
			_ = writeAppChatSSE(w, flusher, "error", map[string]string{"message": "回答生成超时，请重试"})
			logAppChatStreamTiming("error", userID, sessionID, startedAt, "total_timeout")
			return
		}
	}
}

func (s *Server) appChatStreamTiming(totalTimeout time.Duration) (time.Duration, time.Duration) {
	idle := s.chatProviderIdleTimeout
	if idle <= 0 {
		idle = defaultAppChatProviderIdle
	}
	if totalTimeout > 0 {
		maxIdle := totalTimeout * 3 / 4
		if maxIdle <= 0 {
			maxIdle = totalTimeout
		}
		if idle >= totalTimeout || idle > maxIdle {
			idle = maxIdle
		}
	}
	if idle <= 0 {
		idle = time.Second
	}

	heartbeat := s.chatHeartbeatInterval
	if heartbeat <= 0 {
		heartbeat = defaultAppChatHeartbeatInterval
	}
	if heartbeat >= idle {
		heartbeat = idle / 3
	}
	if heartbeat <= 0 {
		heartbeat = time.Millisecond
	}
	return heartbeat, idle
}

func logAppChatStreamTiming(stage string, userID, sessionID int64, startedAt time.Time, phase string) {
	if phase == "" {
		log.Printf("app_chat_stream stage=%s user_id=%d session_id=%d elapsed_ms=%d", stage, userID, sessionID, time.Since(startedAt).Milliseconds())
		return
	}
	log.Printf("app_chat_stream stage=%s phase=%s user_id=%d session_id=%d elapsed_ms=%d", stage, phase, userID, sessionID, time.Since(startedAt).Milliseconds())
}

func (s *Server) beginAppChatPreferenceTurn(userID int64) appChatPreferenceTurn {
	s.preferenceTurnsMu.Lock()
	defer s.preferenceTurnsMu.Unlock()
	if s.preferenceTurns == nil {
		s.preferenceTurns = make(map[int64]*appChatPreferenceTurnState)
	}
	state := s.preferenceTurns[userID]
	if state == nil {
		state = newAppChatPreferenceTurnState()
		s.preferenceTurns[userID] = state
	}
	state.active.Add(1)
	ticket := state.nextTicket.Add(1)
	return appChatPreferenceTurn{userID: userID, state: state, ticket: ticket}
}

func newAppChatPreferenceTurnState() *appChatPreferenceTurnState {
	return &appChatPreferenceTurnState{latestBySlot: make(map[string]uint64)}
}

func (s *Server) finishAppChatPreferenceTurn(turn appChatPreferenceTurn) {
	if turn.state == nil {
		return
	}
	turn.state.active.Add(-1)
	s.cleanupAppChatPreferenceTurn(turn)
}

func (s *Server) cleanupAppChatPreferenceTurn(turn appChatPreferenceTurn) {
	if turn.state == nil {
		return
	}
	s.preferenceTurnsMu.Lock()
	defer s.preferenceTurnsMu.Unlock()
	if s.preferenceTurns[turn.userID] != turn.state {
		return
	}
	if turn.state.active.Load() == 0 && turn.state.pending.Load() == 0 {
		delete(s.preferenceTurns, turn.userID)
	}
}

func (s *Server) prepareAppChatPreferences(ctx context.Context, turn appChatPreferenceTurn, extraction userpreference.Extraction) ([]string, []string, error) {
	if turn.state != nil {
		turn.state.mu.Lock()
		defer turn.state.mu.Unlock()
	} else if len(extraction.Mutations) > 0 {
		return nil, nil, errAppChatPreferenceStale
	}
	if len(extraction.Mutations) > 0 {
		if s.userPreferences == nil {
			return nil, nil, fmt.Errorf("%w: preference store is unavailable", errAppChatPreferenceSave)
		}
		accepted := filterAndMarkAppChatPreferenceMutations(turn.state, turn.ticket, extraction.Mutations)
		if len(accepted) == 0 {
			return nil, nil, errAppChatPreferenceStale
		}
		if err := s.userPreferences.Apply(ctx, turn.userID, accepted); err != nil {
			return nil, nil, fmt.Errorf("%w: %v", errAppChatPreferenceSave, err)
		}
	}
	if s.userPreferences == nil {
		return nil, append([]string(nil), extraction.CurrentDirectives...), nil
	}
	stored, err := s.userPreferences.List(ctx, turn.userID)
	if err != nil {
		return nil, nil, fmt.Errorf("%w: %v", errAppChatPreferenceRead, err)
	}
	preferences := make([]string, 0, len(stored))
	for _, preference := range stored {
		instruction := strings.TrimSpace(preference.Instruction)
		if instruction != "" {
			preferences = append(preferences, instruction)
		}
	}
	return preferences, append([]string(nil), extraction.CurrentDirectives...), nil
}

func (s *Server) prepareAppChatPreferencesLegacy(ctx context.Context, userID int64, question string) ([]string, []string, userpreference.Extraction, error) {
	extraction := userpreference.Extract(question)
	directives := append([]string(nil), extraction.CurrentDirectives...)
	if s.userPreferences == nil {
		return nil, directives, extraction, nil
	}
	stored, err := s.userPreferences.List(ctx, userID)
	if err != nil {
		return nil, nil, extraction, err
	}
	preferences := make([]string, 0, len(stored))
	for _, preference := range stored {
		instruction := strings.TrimSpace(preference.Instruction)
		if instruction != "" {
			preferences = append(preferences, instruction)
		}
	}
	return preferences, directives, extraction, nil
}

func (s *Server) persistAppChatPreferences(ctx context.Context, userID int64, extraction userpreference.Extraction) error {
	if len(extraction.Mutations) == 0 || s.userPreferences == nil {
		return nil
	}
	return s.userPreferences.Apply(ctx, userID, extraction.Mutations)
}

func appChatPreferencePublicError(err error) string {
	if errors.Is(err, errAppChatPreferenceSave) {
		return errAppChatPreferenceSave.Error()
	}
	if errors.Is(err, errAppChatPreferenceStale) {
		return errAppChatPreferenceStale.Error()
	}
	return errAppChatPreferenceRead.Error()
}

func (s *Server) scheduleAppChatPreferenceFallback(turn appChatPreferenceTurn, question string) bool {
	return s.reserveAppChatPreferenceFallback(turn, question).start()
}

func (s *Server) reserveAppChatPreferenceFallback(turn appChatPreferenceTurn, question string) *appChatPreferenceFallbackReservation {
	if turn.userID <= 0 || turn.state == nil || s.userPreferences == nil || s.preferenceExtractor == nil ||
		!userpreference.NeedsLLMFallback(question) {
		return nil
	}
	if s.preferenceAsyncSlots == nil {
		return nil
	}
	select {
	case s.preferenceAsyncSlots <- struct{}{}:
	default:
		return nil
	}
	turn.state.pending.Add(1)
	return &appChatPreferenceFallbackReservation{server: s, turn: turn, question: question}
}

func (r *appChatPreferenceFallbackReservation) start() bool {
	if r == nil {
		return false
	}
	started := false
	r.claim.Do(func() {
		started = true
		go r.run()
	})
	return started
}

func (r *appChatPreferenceFallbackReservation) release() {
	if r == nil {
		return
	}
	r.claim.Do(r.finish)
}

func (r *appChatPreferenceFallbackReservation) run() {
	s := r.server
	turn := r.turn
	timeout := s.preferenceAsyncTimeout
	if timeout <= 0 {
		timeout = 2 * time.Second
	}
	ctx, cancel := context.WithTimeout(context.Background(), timeout)
	defer cancel()
	defer r.finish()
	extraction := s.preferenceExtractor.Extract(ctx, r.question)
	turn.state.mu.Lock()
	if len(extraction.Mutations) > 0 && ctx.Err() == nil {
		accepted := filterAndMarkAppChatPreferenceMutations(turn.state, turn.ticket, extraction.Mutations)
		if len(accepted) > 0 {
			_ = s.userPreferences.Apply(ctx, turn.userID, accepted)
		}
	}
	turn.state.mu.Unlock()
}

func (r *appChatPreferenceFallbackReservation) finish() {
	r.turn.state.pending.Add(-1)
	<-r.server.preferenceAsyncSlots
	r.server.cleanupAppChatPreferenceTurn(r.turn)
}

// filterAndMarkAppChatPreferenceMutations must be called with state.mu held.
func filterAndMarkAppChatPreferenceMutations(state *appChatPreferenceTurnState, ticket uint64, mutations []userpreference.Mutation) []userpreference.Mutation {
	if state == nil || ticket == 0 || len(mutations) == 0 {
		return nil
	}
	if state.latestBySlot == nil {
		state.latestBySlot = make(map[string]uint64)
	}
	accepted := make([]userpreference.Mutation, 0, len(mutations))
	for _, mutation := range mutations {
		slot := strings.TrimSpace(mutation.DeleteSlot)
		if mutation.Upsert != nil {
			slot = strings.TrimSpace(mutation.Upsert.Slot)
		}
		if slot == "" || state.latestBySlot[slot] > ticket {
			continue
		}
		state.latestBySlot[slot] = ticket
		accepted = append(accepted, mutation)
	}
	return accepted
}

func writeAppChatSSE(w io.Writer, flusher http.Flusher, event string, payload any) error {
	data, err := json.Marshal(payload)
	if err != nil {
		return err
	}
	if _, err := fmt.Fprintf(w, "event: %s\ndata: %s\n\n", event, data); err != nil {
		return err
	}
	flusher.Flush()
	return nil
}

func writeAppChatSSEComment(w io.Writer, flusher http.Flusher, comment string) error {
	frame := ": " + comment + "\n\n"
	written, err := io.WriteString(w, frame)
	if err != nil {
		return err
	}
	if written != len(frame) {
		return io.ErrShortWrite
	}
	flusher.Flush()
	return nil
}

func appChatSessionIDFromPath(path, suffix string) (int64, bool) {
	const prefix = "/api/app/chat/sessions/"
	if !strings.HasPrefix(path, prefix) || !strings.HasSuffix(path, suffix) {
		return 0, false
	}
	rest := strings.TrimPrefix(path, prefix)
	rest = strings.TrimSuffix(rest, suffix)
	id, err := strconv.ParseInt(strings.Trim(rest, "/"), 10, 64)
	if err != nil || id == 0 {
		return 0, false
	}
	return id, true
}

func (s *Server) rememberChatAnswer(ctx context.Context, appUserID, cardID int64, question, answer string) {
	answer = strings.TrimSpace(answer)
	content := chatMemoryContent(question)
	if cardID <= 0 || content == "" || answer == "" {
		return
	}
	if len([]rune(content)) > 160 {
		runes := []rune(content)
		content = string(runes[:160])
	}
	_, _ = s.db.ExecContext(ctx,
		`INSERT INTO app_memories (app_user_id, card_id, content, source_time)
		 SELECT $1, $2, $3, now()
		 WHERE NOT EXISTS (
		   SELECT 1 FROM app_memories
		   WHERE app_user_id = $1 AND card_id = $2 AND content = $3
		 )`,
		appUserID, cardID, content)
}

func (s *Server) appChatContextForPrompt(ctx context.Context, sessionID int64, generator rag.Generator) appChatPromptContext {
	if s.appChat == nil {
		return appChatPromptContext{}
	}
	var summarizer rag.ConversationSummarizer
	if typed, ok := generator.(rag.ConversationSummarizer); ok {
		summarizer = typed
	}
	return buildAppChatPromptContext(ctx, sessionID, s.appChat, summarizer)
}

func appChatHistoryFromMessages(messages []chat.Message) []rag.Message {
	history := make([]rag.Message, 0, len(messages))
	for _, message := range messages {
		role := strings.TrimSpace(message.Role)
		content := message.EffectiveContent()
		if (role != "user" && role != "assistant") || content == "" {
			continue
		}
		history = append(history, rag.Message{Role: role, Content: content})
	}
	return history
}

func validAppChatMessages(messages []chat.Message) []chat.Message {
	valid := make([]chat.Message, 0, len(messages))
	for _, message := range messages {
		role := strings.TrimSpace(message.Role)
		content := message.EffectiveContent()
		if (role != "user" && role != "assistant") || content == "" {
			continue
		}
		message.Role = role
		message.Content = content
		valid = append(valid, message)
	}
	return valid
}

func chatMemoryContent(question string) string {
	question = strings.TrimSpace(question)
	if question == "" || strings.ContainsAny(question, "?？") {
		return ""
	}
	for _, marker := range []string{"不要记", "别记", "不用记", "不必记", "忘掉", "忘记", "删除记忆", "删掉记忆"} {
		if strings.Contains(question, marker) {
			return ""
		}
	}
	for _, marker := range []string{"记住", "请记得", "以后要记得"} {
		if strings.Contains(question, marker) {
			return question
		}
	}
	for _, prefix := range []string{
		"我是", "我叫", "我的孩子", "我的女儿", "我的儿子", "我的丈夫", "我的妻子", "我的妈妈", "我的爸爸",
		"我孩子", "我女儿", "我儿子", "我丈夫", "我妻子", "我妈妈", "我爸爸",
	} {
		if strings.HasPrefix(question, prefix) {
			return question
		}
	}
	return ""
}

func (s *Server) appChatMemoriesForPrompt(ctx context.Context, appUserID, cardID int64, limit int) ([]string, error) {
	if appUserID <= 0 || cardID <= 0 || limit <= 0 {
		return nil, nil
	}
	if limit > 6 {
		limit = 6
	}
	rows, err := s.db.QueryContext(ctx,
		`SELECT content
		 FROM app_memories
		 WHERE app_user_id = $1 AND card_id = $2 AND status = 'active'
		 ORDER BY update_time DESC, id DESC
		 LIMIT $3`,
		appUserID, cardID, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	memories := make([]string, 0, limit)
	for rows.Next() {
		var content string
		if err := rows.Scan(&content); err != nil {
			return nil, err
		}
		content = strings.TrimSpace(content)
		if content == "" {
			continue
		}
		if len([]rune(content)) > 160 {
			content = string([]rune(content)[:160])
		}
		memories = append(memories, content)
	}
	return memories, rows.Err()
}

// askResponse 在 rag.Answer 基础上附带刚落库的 AI 消息 id，供前端定位反馈 / 收藏。
type askResponse struct {
	rag.Answer
	MessageID int64 `json:"messageId"`
}

// validFeedback 反馈枚举：有帮助 / 不准确 / 想继续问 / 清除。
var validFeedback = map[string]bool{
	"helpful":    true,
	"inaccurate": true,
	"continue":   true,
	"":           true,
}

// messageIDFromPath 从 /api/app/chat/messages/{id}/{action} 中解析消息 id。
func messageIDFromPath(path, action string) (int64, bool) {
	rest := strings.TrimPrefix(path, "/api/app/chat/messages/")
	rest = strings.TrimSuffix(rest, "/"+action)
	id, err := strconv.ParseInt(strings.Trim(rest, "/"), 10, 64)
	if err != nil || id == 0 {
		return 0, false
	}
	return id, true
}

// appChatFeedback POST /api/app/chat/messages/{id}/feedback — 设置某条 AI 回答的反馈。
func (s *Server) appChatFeedback(w http.ResponseWriter, r *http.Request) {
	userInfo, ok := appUserFromContext(r)
	if !ok {
		httpx.Fail(w, http.StatusUnauthorized, "unauthorized")
		return
	}
	messageID, ok := messageIDFromPath(r.URL.Path, "feedback")
	if !ok {
		httpx.Fail(w, http.StatusBadRequest, "invalid message id")
		return
	}
	var body struct {
		Feedback string `json:"feedback"`
	}
	if err := json.NewDecoder(io.LimitReader(r.Body, 4<<10)).Decode(&body); err != nil {
		httpx.Fail(w, http.StatusBadRequest, "invalid body")
		return
	}
	if !validFeedback[body.Feedback] {
		httpx.Fail(w, http.StatusBadRequest, "invalid feedback")
		return
	}
	if err := s.appChat.SetFeedback(r.Context(), userInfo.ID, messageID, body.Feedback); err != nil {
		httpx.Fail(w, http.StatusNotFound, "message not found")
		return
	}
	httpx.OK(w, map[string]string{"feedback": body.Feedback})
}

// appChatFavorite POST /api/app/chat/messages/{id}/favorite — 切换收藏。
func (s *Server) appChatFavorite(w http.ResponseWriter, r *http.Request) {
	userInfo, ok := appUserFromContext(r)
	if !ok {
		httpx.Fail(w, http.StatusUnauthorized, "unauthorized")
		return
	}
	messageID, ok := messageIDFromPath(r.URL.Path, "favorite")
	if !ok {
		httpx.Fail(w, http.StatusBadRequest, "invalid message id")
		return
	}
	favorite, err := s.appChat.ToggleFavorite(r.Context(), userInfo.ID, messageID)
	if err != nil {
		httpx.Fail(w, http.StatusNotFound, "message not found")
		return
	}
	httpx.OK(w, map[string]bool{"favorite": favorite})
}

// appChatFavorites GET /api/app/chat/favorites?cardId= — 收藏列表。
func (s *Server) appChatFavorites(w http.ResponseWriter, r *http.Request) {
	userInfo, ok := appUserFromContext(r)
	if !ok {
		httpx.Fail(w, http.StatusUnauthorized, "unauthorized")
		return
	}
	cardID, _ := strconv.ParseInt(r.URL.Query().Get("cardId"), 10, 64)
	items, err := s.appChat.ListFavorites(r.Context(), userInfo.ID, cardID)
	if err != nil {
		httpx.Fail(w, http.StatusInternalServerError, "server error")
		return
	}
	httpx.OK(w, items)
}

// appChatSearch GET /api/app/chat/search?cardId=&q= — 历史关键词搜索。
func (s *Server) appChatSearch(w http.ResponseWriter, r *http.Request) {
	userInfo, ok := appUserFromContext(r)
	if !ok {
		httpx.Fail(w, http.StatusUnauthorized, "unauthorized")
		return
	}
	keyword := strings.TrimSpace(r.URL.Query().Get("q"))
	if keyword == "" {
		httpx.OK(w, []any{})
		return
	}
	cardID, _ := strconv.ParseInt(r.URL.Query().Get("cardId"), 10, 64)
	items, err := s.appChat.SearchMessages(r.Context(), userInfo.ID, cardID, keyword)
	if err != nil {
		httpx.Fail(w, http.StatusInternalServerError, "server error")
		return
	}
	httpx.OK(w, items)
}

// appChatRouter dispatches /api/app/chat/sessions/* to the correct handler.
func (s *Server) appChatRouter(w http.ResponseWriter, r *http.Request) {
	path := r.URL.Path
	switch {
	case path == "/api/app/chat/sessions" && r.Method == http.MethodGet:
		s.appChatSessions(w, r)
	case path == "/api/app/chat/sessions" && r.Method == http.MethodPost:
		s.appChatGetOrCreate(w, r)
	case strings.HasSuffix(path, "/messages") && r.Method == http.MethodGet:
		s.appChatMessages(w, r)
	case strings.HasSuffix(path, "/ask/stream") && r.Method == http.MethodPost:
		s.appChatAskStream(w, r)
	case strings.HasSuffix(path, "/ask") && r.Method == http.MethodPost:
		s.appChatAsk(w, r)
	case strings.HasSuffix(path, "/voice") && r.Method == http.MethodPost:
		s.appChatVoice(w, r)
	default:
		httpx.Fail(w, http.StatusMethodNotAllowed, "method not allowed")
	}
}

// appChatMessageRouter dispatches /api/app/chat/messages/{id}/{feedback|favorite|audio|transcript}.
func (s *Server) appChatMessageRouter(w http.ResponseWriter, r *http.Request) {
	path := r.URL.Path
	switch {
	case strings.HasSuffix(path, "/feedback") && r.Method == http.MethodPost:
		s.appChatFeedback(w, r)
	case strings.HasSuffix(path, "/favorite") && r.Method == http.MethodPost:
		s.appChatFavorite(w, r)
	case strings.HasSuffix(path, "/audio") && r.Method == http.MethodGet:
		s.appChatVoiceAudio(w, r)
	case strings.HasSuffix(path, "/transcript") && r.Method == http.MethodGet:
		s.appChatVoiceTranscript(w, r)
	default:
		httpx.Fail(w, http.StatusMethodNotAllowed, "method not allowed")
	}
}
