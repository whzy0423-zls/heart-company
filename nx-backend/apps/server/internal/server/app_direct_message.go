package server

import (
	"encoding/json"
	"errors"
	"net/http"
	"strconv"
	"strings"

	"nine-xing/nx-backend/apps/server/internal/directmessage"
	"nine-xing/nx-backend/apps/server/internal/httpx"
)

func (s *Server) appDirectMessageRouter(w http.ResponseWriter, r *http.Request) {
	user, ok := appUserFromContext(r)
	if !ok {
		httpx.Fail(w, http.StatusUnauthorized, "unauthorized")
		return
	}
	path := strings.TrimPrefix(r.URL.Path, "/api/app/direct/")
	switch {
	case strings.HasPrefix(path, "conversations/") && strings.HasSuffix(path, "/appearance"):
		s.appChatAppearanceRouter(w, r)
	case path == "conversations" && r.Method == http.MethodGet:
		items, err := s.directMessages.ListConversations(r.Context(), user.ID)
		if err != nil {
			mapDirectMessageError(w, err)
			return
		}
		httpx.OK(w, map[string]any{"items": items})
	case path == "conversations" && r.Method == http.MethodPost:
		var body struct {
			PeerID int64 `json:"peerId"`
		}
		if json.NewDecoder(r.Body).Decode(&body) != nil {
			httpx.Fail(w, http.StatusBadRequest, "direct_message.invalid_conversation")
			return
		}
		item, err := s.directMessages.GetOrCreateConversation(r.Context(), user.ID, body.PeerID)
		if err != nil {
			mapDirectMessageError(w, err)
			return
		}
		httpx.JSON(w, http.StatusCreated, map[string]any{"code": 0, "data": item, "error": nil, "message": "ok"})
	case strings.HasPrefix(path, "conversations/") && strings.HasSuffix(path, "/messages") && r.Method == http.MethodGet:
		id, ok := parseDirectPathID(path, "conversations/", "/messages")
		if !ok {
			httpx.Fail(w, http.StatusBadRequest, "direct_message.invalid_conversation")
			return
		}
		before := positiveDirectQuery(r, "beforeSequence")
		after := positiveDirectQuery(r, "afterSequence")
		cursor, err := directmessage.NormalizeHistoryCursor(before, after)
		if err != nil {
			mapDirectMessageError(w, err)
			return
		}
		items, err := s.directMessages.History(r.Context(), user.ID, id, cursor, int(positiveDirectQuery(r, "limit")))
		if err != nil {
			mapDirectMessageError(w, err)
			return
		}
		httpx.OK(w, map[string]any{"items": items, "beforeSequence": before, "afterSequence": after})
	case strings.HasPrefix(path, "conversations/") && strings.HasSuffix(path, "/messages") && r.Method == http.MethodPost:
		id, ok := parseDirectPathID(path, "conversations/", "/messages")
		if !ok {
			httpx.Fail(w, http.StatusBadRequest, "direct_message.invalid_conversation")
			return
		}
		var body struct {
			ClientMessageID string `json:"clientMessageId"`
			MessageType     string `json:"messageType"`
			Body            string `json:"body"`
			MediaID         *int64 `json:"mediaId"`
		}
		if json.NewDecoder(r.Body).Decode(&body) != nil {
			httpx.Fail(w, http.StatusBadRequest, "direct_message.invalid_conversation")
			return
		}
		item, err := s.directMessages.Send(r.Context(), directmessage.SendInput{ConversationID: id, SenderID: user.ID, ClientMessageID: body.ClientMessageID, MessageType: body.MessageType, Body: body.Body, MediaID: body.MediaID})
		if err != nil {
			mapDirectMessageError(w, err)
			return
		}
		s.directRealtimeHub.Publish(id, map[string]any{"type": "message", "data": item})
		httpx.JSON(w, http.StatusCreated, map[string]any{"code": 0, "data": item, "error": nil, "message": "ok"})
	case strings.HasPrefix(path, "messages/") && strings.HasSuffix(path, "/read") && r.Method == http.MethodPost:
		id, ok := parseDirectPathID(path, "messages/", "/read")
		if !ok {
			httpx.Fail(w, http.StatusBadRequest, "direct_message.invalid_conversation")
			return
		}
		var body struct {
			ConversationID int64 `json:"conversationId"`
			Sequence       int64 `json:"sequence"`
		}
		if json.NewDecoder(r.Body).Decode(&body) != nil {
			httpx.Fail(w, http.StatusBadRequest, "direct_message.cursor_conflict")
			return
		}
		if body.ConversationID <= 0 {
			body.ConversationID = id
		}
		if err := s.directMessages.MarkRead(r.Context(), user.ID, body.ConversationID, body.Sequence); err != nil {
			mapDirectMessageError(w, err)
			return
		}
		httpx.OK(w, nil)
	case strings.HasPrefix(path, "messages/") && strings.HasSuffix(path, "/recall") && r.Method == http.MethodPost:
		id, ok := parseDirectPathID(path, "messages/", "/recall")
		if !ok {
			httpx.Fail(w, http.StatusBadRequest, "direct_message.message_not_found")
			return
		}
		item, err := s.directMessages.Recall(r.Context(), user.ID, id)
		if err != nil {
			mapDirectMessageError(w, err)
			return
		}
		s.directRealtimeHub.Publish(item.ConversationID, map[string]any{"type": "message", "data": item})
		httpx.OK(w, item)
	default:
		httpx.Fail(w, http.StatusNotFound, "direct_message.not_found")
	}
}

func parseDirectPathID(path, prefix, suffix string) (int64, bool) {
	raw := strings.TrimSuffix(strings.TrimPrefix(path, prefix), suffix)
	id, err := strconv.ParseInt(raw, 10, 64)
	return id, err == nil && id > 0 && !strings.Contains(raw, "/")
}
func positiveDirectQuery(r *http.Request, key string) int64 {
	v, err := strconv.ParseInt(strings.TrimSpace(r.URL.Query().Get(key)), 10, 64)
	if err != nil || v < 0 {
		return 0
	}
	return v
}
func mapDirectMessageError(w http.ResponseWriter, err error) {
	status := http.StatusInternalServerError
	switch {
	case errors.Is(err, directmessage.ErrInvalidConversation), errors.Is(err, directmessage.ErrCursorConflict):
		status = http.StatusBadRequest
	case errors.Is(err, directmessage.ErrBlocked), errors.Is(err, directmessage.ErrNotFriend), errors.Is(err, directmessage.ErrNotParticipant):
		status = http.StatusForbidden
	case errors.Is(err, directmessage.ErrMessageNotFound):
		status = http.StatusNotFound
	case errors.Is(err, directmessage.ErrPayloadConflict):
		status = http.StatusConflict
	case errors.Is(err, directmessage.ErrRecallWindow):
		status = http.StatusUnprocessableEntity
	}
	httpx.Fail(w, status, err.Error())
}
