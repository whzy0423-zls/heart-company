package server

import (
	"encoding/json"
	"errors"
	"net/http"
	"strconv"
	"strings"

	"nine-xing/nx-backend/apps/server/internal/chatappearance"
	"nine-xing/nx-backend/apps/server/internal/httpx"
)

func (s *Server) appChatAppearanceRouter(w http.ResponseWriter, r *http.Request) {
	user, ok := appUserFromContext(r)
	if !ok {
		httpx.Fail(w, http.StatusUnauthorized, "unauthorized")
		return
	}
	parts := strings.Split(strings.Trim(strings.TrimPrefix(r.URL.Path, "/api/app/direct/conversations/"), "/"), "/")
	if len(parts) != 2 || parts[1] != "appearance" {
		httpx.Fail(w, http.StatusNotFound, "chat_appearance.not_found")
		return
	}
	conversationID, err := strconv.ParseInt(parts[0], 10, 64)
	if err != nil || conversationID <= 0 {
		httpx.Fail(w, http.StatusBadRequest, "chat_appearance.invalid_conversation")
		return
	}
	if r.Method == http.MethodGet {
		item, err := s.chatAppearance.Get(r.Context(), user.ID, conversationID)
		if err != nil {
			if errors.Is(err, chatappearance.ErrNotParticipant) {
				httpx.Fail(w, http.StatusForbidden, err.Error())
				return
			}
			httpx.Fail(w, http.StatusInternalServerError, "chat_appearance.load_failed")
			return
		}
		httpx.OK(w, item)
		return
	}
	if r.Method != http.MethodPut && r.Method != http.MethodPatch {
		httpx.Fail(w, http.StatusMethodNotAllowed, "method not allowed")
		return
	}
	var body struct {
		BackgroundType  string `json:"backgroundType"`
		BackgroundValue string `json:"backgroundValue"`
	}
	if json.NewDecoder(r.Body).Decode(&body) != nil {
		httpx.Fail(w, http.StatusBadRequest, "chat_appearance.invalid_value")
		return
	}
	item, err := s.chatAppearance.Upsert(r.Context(), user.ID, conversationID, body.BackgroundType, body.BackgroundValue)
	if err != nil {
		if errors.Is(err, chatappearance.ErrNotParticipant) {
			httpx.Fail(w, http.StatusForbidden, err.Error())
			return
		}
		httpx.Fail(w, http.StatusBadRequest, err.Error())
		return
	}
	httpx.OK(w, item)
}
