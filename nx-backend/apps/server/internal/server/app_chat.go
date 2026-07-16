package server

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strconv"
	"strings"
	"time"

	"nine-xing/nx-backend/apps/server/internal/chat"
	"nine-xing/nx-backend/apps/server/internal/httpx"
	"nine-xing/nx-backend/apps/server/internal/rag"
	"nine-xing/nx-backend/apps/server/internal/userpreference"
)

const appChatHistoryLimit = 12

const appChatFallbackHistoryLimit = 20

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

	ctx, cancel := context.WithTimeout(r.Context(), s.chatTimeout)
	defer cancel()
	preferences, directives, extraction, err := s.prepareAppChatPreferences(ctx, userInfo.ID, body.Question)
	if err != nil {
		httpx.Fail(w, http.StatusInternalServerError, "偏好读取失败，请重试")
		return
	}

	docs, _ := s.retrieveAppDocsForQuery(ctx, body.Question, 6)
	profile := rag.UserProfile{}
	if appUser, err := s.appUsers.FindByID(ctx, userInfo.ID); err == nil {
		profile.Nickname = appUser.Nickname
	}
	// 注入用户主型，供检索加权与 AI 个性化作答（未测/无主卡时为 0，rag 仅在 >0 时使用）。
	if card, err := s.quiz.PrimaryCard(ctx, userInfo.ID); err == nil {
		profile.MainType = card.MainType
	}
	if memories, err := s.appChatMemoriesForPrompt(ctx, userInfo.ID, sess.CardID, 6); err == nil {
		profile.Memories = memories
	}
	generator := s.generator()
	promptContext := s.appChatContextForPrompt(ctx, sessionID, generator)

	ans, err := rag.NewService(docs, rag.WithGenerator(generator)).Ask(ctx, rag.AskInput{
		History:             promptContext.History,
		ConversationSummary: promptContext.Summary,
		Question:            body.Question,
		UserProfile:         profile,
		UserPreferences:     preferences,
		CurrentDirectives:   directives,
	})
	if err != nil {
		httpx.Fail(w, http.StatusInternalServerError, "回答生成失败，请重试")
		return
	}

	sourcesJSON, _ := json.Marshal(ans.Sources)
	messageID, saveErr := s.appChat.SavePair(ctx, sessionID, body.Question, ans.Answer, sourcesJSON)
	if saveErr != nil {
		httpx.Fail(w, http.StatusInternalServerError, "回答保存失败，请重试")
		return
	}
	if err := s.persistAppChatPreferences(ctx, userInfo.ID, extraction); err != nil {
		httpx.Fail(w, http.StatusInternalServerError, "偏好保存失败，请重试")
		return
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

	ctx, cancel := context.WithTimeout(r.Context(), s.chatTimeout)
	defer cancel()
	preferences, directives, extraction, err := s.prepareAppChatPreferences(ctx, userInfo.ID, body.Question)
	if err != nil {
		httpx.Fail(w, http.StatusInternalServerError, "偏好读取失败，请重试")
		return
	}

	docs, _ := s.retrieveAppDocsForQuery(ctx, body.Question, 6)
	profile := rag.UserProfile{}
	if s.appUsers != nil {
		if appUser, err := s.appUsers.FindByID(ctx, userInfo.ID); err == nil {
			profile.Nickname = appUser.Nickname
		}
	}
	if s.quiz != nil {
		if card, err := s.quiz.PrimaryCard(ctx, userInfo.ID); err == nil {
			profile.MainType = card.MainType
		}
	}
	if memories, err := s.appChatMemoriesForPrompt(ctx, userInfo.ID, sess.CardID, 6); err == nil {
		profile.Memories = memories
	}
	generator := s.generator()
	promptContext := s.appChatContextForPrompt(ctx, sessionID, generator)

	w.Header().Set("Content-Type", "text/event-stream; charset=utf-8")
	w.Header().Set("Cache-Control", "no-cache")
	w.Header().Set("Connection", "keep-alive")
	w.Header().Set("X-Accel-Buffering", "no")
	if err := writeAppChatSSEComment(w, flusher, "ping"); err != nil {
		return
	}

	type streamResult struct {
		answer rag.Answer
		err    error
	}
	generationCtx, cancelGeneration := context.WithCancel(ctx)
	defer cancelGeneration()
	deltas := make(chan string)
	result := make(chan streamResult, 1)
	go func() {
		answer, streamErr := rag.NewService(docs, rag.WithGenerator(generator)).AskStream(generationCtx, rag.AskInput{
			History:             promptContext.History,
			ConversationSummary: promptContext.Summary,
			Question:            body.Question,
			UserProfile:         profile,
			UserPreferences:     preferences,
			CurrentDirectives:   directives,
		}, func(delta string) error {
			select {
			case deltas <- delta:
				return nil
			case <-generationCtx.Done():
				return generationCtx.Err()
			}
		})
		result <- streamResult{answer: answer, err: streamErr}
	}()

	heartbeatInterval := s.chatHeartbeatInterval
	if heartbeatInterval <= 0 {
		heartbeatInterval = 12 * time.Second
	}
	heartbeat := time.NewTicker(heartbeatInterval)
	defer heartbeat.Stop()

	var ans rag.Answer
	for {
		select {
		case delta := <-deltas:
			if err := writeAppChatSSE(w, flusher, "delta", map[string]string{"content": delta}); err != nil {
				cancelGeneration()
				return
			}
		case completed := <-result:
			ans = completed.answer
			err = completed.err
			goto generationComplete
		case <-heartbeat.C:
			if err := writeAppChatSSEComment(w, flusher, "ping"); err != nil {
				cancelGeneration()
				return
			}
		case <-ctx.Done():
			cancelGeneration()
			return
		}
	}

generationComplete:
	if err != nil {
		_ = writeAppChatSSE(w, flusher, "error", map[string]string{"message": "回答生成失败，请重试"})
		return
	}

	sourcesJSON, _ := json.Marshal(ans.Sources)
	messageID, saveErr := s.appChat.SavePair(ctx, sessionID, body.Question, ans.Answer, sourcesJSON)
	if saveErr != nil {
		_ = writeAppChatSSE(w, flusher, "error", map[string]string{"message": "回答保存失败，请重试"})
		return
	}
	if err := s.persistAppChatPreferences(ctx, userInfo.ID, extraction); err != nil {
		_ = writeAppChatSSE(w, flusher, "error", map[string]string{"message": "偏好保存失败，请重试"})
		return
	}
	s.rememberChatAnswer(ctx, userInfo.ID, sess.CardID, body.Question, ans.Answer)
	if messageID > 0 {
		s.recordAppProfileEvidenceAsync(userInfo.ID, sess.CardID, "chat", messageID, body.Question)
	}

	_ = writeAppChatSSE(w, flusher, "done", askResponse{Answer: ans, MessageID: messageID})
}

func (s *Server) prepareAppChatPreferences(ctx context.Context, userID int64, question string) ([]string, []string, userpreference.Extraction, error) {
	extraction := userpreference.Extract(question)
	directives := append([]string(nil), extraction.CurrentDirectives...)
	if s.userPreferences == nil {
		return nil, directives, extraction, nil
	}
	stored, err := s.userPreferences.List(ctx, userID)
	if err != nil {
		return nil, nil, extraction, err
	}
	overriddenSlots := appChatDirectiveSlots(directives)
	preferences := make([]string, 0, len(stored))
	for _, preference := range stored {
		if _, overridden := overriddenSlots[preference.Slot]; overridden {
			continue
		}
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

func appChatDirectiveSlots(directives []string) map[string]struct{} {
	slots := make(map[string]struct{}, len(directives))
	for _, directive := range directives {
		switch {
		case directive == "不要使用“亲爱的”等亲昵称呼":
			slots["addressing.avoid_dear"] = struct{}{}
		case strings.HasPrefix(directive, "称呼用户为"):
			slots["addressing.preferred_name"] = struct{}{}
		case directive == "回答简短，避免长篇大论", directive == "回答更详细", directive == "只回答一句":
			slots["length.detail_level"] = struct{}{}
		case directive == "表达直接，少说教":
			slots["tone.direct"] = struct{}{}
		case directive == "不要使用列表":
			slots["format.no_lists"] = struct{}{}
		case directive == "先给结论", directive == "只给结论":
			slots["format.conclusion_first"] = struct{}{}
		case directive == "不要反问或追问":
			slots["interaction.no_followup"] = struct{}{}
		}
	}
	return slots
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
	comment = strings.TrimSpace(strings.ReplaceAll(comment, "\n", " "))
	if comment == "" {
		comment = "ping"
	}
	if _, err := fmt.Fprintf(w, ": %s\n\n", comment); err != nil {
		return err
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
	default:
		httpx.Fail(w, http.StatusMethodNotAllowed, "method not allowed")
	}
}

// appChatMessageRouter dispatches /api/app/chat/messages/{id}/{feedback|favorite}.
func (s *Server) appChatMessageRouter(w http.ResponseWriter, r *http.Request) {
	path := r.URL.Path
	switch {
	case strings.HasSuffix(path, "/feedback") && r.Method == http.MethodPost:
		s.appChatFeedback(w, r)
	case strings.HasSuffix(path, "/favorite") && r.Method == http.MethodPost:
		s.appChatFavorite(w, r)
	default:
		httpx.Fail(w, http.StatusMethodNotAllowed, "method not allowed")
	}
}
