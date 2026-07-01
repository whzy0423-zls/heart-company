package server

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"strconv"
	"strings"
	"time"

	"nine-xing/nx-backend/apps/server/internal/httpx"
	"nine-xing/nx-backend/apps/server/internal/rag"
)

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
	idText := strings.Trim(strings.TrimPrefix(r.URL.Path, "/api/app/chat/sessions/"), "/ask")
	sessionID, err := strconv.ParseInt(idText, 10, 64)
	if err != nil || sessionID == 0 {
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

	docs, _ := s.retrieveDocsForQuery(ctx, body.Question, 6)
	profile := rag.UserProfile{}
	if appUser, err := s.appUsers.FindByID(ctx, userInfo.ID); err == nil {
		profile.Nickname = appUser.Nickname
	}
	// 注入用户主型，供检索加权与 AI 个性化作答（未测/无主卡时为 0，rag 仅在 >0 时使用）。
	if card, err := s.quiz.PrimaryCard(ctx, userInfo.ID); err == nil {
		profile.MainType = card.MainType
	}

	ans, err := rag.NewService(docs, rag.WithGenerator(s.generator())).Ask(ctx, rag.AskInput{
		History:     body.History,
		Question:    body.Question,
		UserProfile: profile,
	})
	if err != nil {
		httpx.Fail(w, http.StatusInternalServerError, "回答生成失败，请重试")
		return
	}

	sourcesJSON, _ := json.Marshal(ans.Sources)
	messageID, saveErr := s.appChat.SavePair(ctx, sessionID, body.Question, ans.Answer, sourcesJSON)
	if saveErr != nil {
		_ = saveErr
	}
	s.rememberChatAnswer(ctx, userInfo.ID, sess.CardID, body.Question, ans.Answer)

	httpx.OK(w, askResponse{Answer: ans, MessageID: messageID})
}

func (s *Server) rememberChatAnswer(ctx context.Context, appUserID, cardID int64, question, answer string) {
	question = strings.TrimSpace(question)
	answer = strings.TrimSpace(answer)
	if cardID <= 0 || question == "" || answer == "" {
		return
	}
	if len([]rune(question)) < 8 {
		return
	}
	content := "用户曾问：" + question
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
