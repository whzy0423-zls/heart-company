package server

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log"
	"net/http"
	"strconv"
	"strings"

	"nine-xing/nx-backend/apps/server/internal/friends"
	"nine-xing/nx-backend/apps/server/internal/httpx"
	"nine-xing/nx-backend/apps/server/internal/push"
)

func (s *Server) appSocialRouter(w http.ResponseWriter, r *http.Request) {
	user, ok := appUserFromContext(r)
	if !ok {
		httpx.Fail(w, http.StatusUnauthorized, "unauthorized")
		return
	}
	path := strings.TrimPrefix(r.URL.Path, "/api/app/")
	switch {
	case path == "social/invite" && r.Method == http.MethodGet:
		code, err := s.friends.GetOrCreateInviteCode(r.Context(), user.ID)
		if err != nil {
			mapFriendError(w, err)
			return
		}
		httpx.OK(w, map[string]any{"inviteCode": code})
	case path == "social/invite/rotate" && r.Method == http.MethodPost:
		code, err := s.friends.RotateInviteCode(r.Context(), user.ID)
		if err != nil {
			mapFriendError(w, err)
			return
		}
		httpx.OK(w, map[string]any{"inviteCode": code})
	case path == "social/search" && r.Method == http.MethodGet:
		item, err := s.friends.Search(r.Context(), user.ID, r.URL.Query().Get("q"))
		if err != nil {
			mapFriendError(w, err)
			return
		}
		httpx.OK(w, item)
	case path == "friends" && r.Method == http.MethodGet:
		items, err := s.friends.ListFriends(r.Context(), user.ID)
		if err != nil {
			mapFriendError(w, err)
			return
		}
		httpx.OK(w, map[string]any{"items": items})
	case path == "friend-requests" && r.Method == http.MethodGet:
		incoming := strings.ToLower(r.URL.Query().Get("direction")) != "outgoing"
		items, err := s.friends.ListRequests(r.Context(), user.ID, incoming)
		if err != nil {
			mapFriendError(w, err)
			return
		}
		httpx.OK(w, map[string]any{"items": items})
	case path == "friend-requests" && r.Method == http.MethodPost:
		var body struct {
			AddresseeID int64  `json:"addresseeId"`
			Message     string `json:"message"`
		}
		if json.NewDecoder(r.Body).Decode(&body) != nil {
			httpx.Fail(w, http.StatusBadRequest, "friend.invalid_request")
			return
		}
		item, err := s.friends.CreateRequest(r.Context(), user.ID, body.AddresseeID, body.Message)
		if err != nil {
			mapFriendError(w, err)
			return
		}
		requesterName := strings.TrimSpace(user.RealName)
		if requesterName == "" {
			requesterName = strings.TrimSpace(user.Username)
		}
		s.notifyFriendRequest(r.Context(), item, requesterName)
		httpx.JSON(w, http.StatusCreated, map[string]any{"code": 0, "data": item, "error": nil, "message": "ok"})
	case strings.HasPrefix(path, "friend-requests/") && r.Method == http.MethodPost:
		rest := strings.TrimPrefix(path, "friend-requests/")
		parts := strings.Split(strings.Trim(rest, "/"), "/")
		if len(parts) != 2 {
			httpx.Fail(w, http.StatusNotFound, "friend.not_found")
			return
		}
		id, err := strconv.ParseInt(parts[0], 10, 64)
		if err != nil || id <= 0 {
			httpx.Fail(w, http.StatusBadRequest, "friend.invalid_request")
			return
		}
		accept := parts[1] == "accept"
		if !accept && parts[1] != "reject" {
			httpx.Fail(w, http.StatusNotFound, "friend.not_found")
			return
		}
		item, err := s.friends.RespondRequest(r.Context(), user.ID, id, accept)
		if err != nil {
			mapFriendError(w, err)
			return
		}
		httpx.OK(w, item)
	case strings.HasPrefix(path, "friends/") && r.Method == http.MethodDelete:
		id, err := strconv.ParseInt(strings.Trim(strings.TrimPrefix(path, "friends/"), "/"), 10, 64)
		if err != nil || id <= 0 {
			httpx.Fail(w, http.StatusBadRequest, "friend.invalid_user_id")
			return
		}
		if err := s.friends.DeleteFriend(r.Context(), user.ID, id); err != nil {
			mapFriendError(w, err)
			return
		}
		httpx.OK(w, nil)
	case path == "social/personality-visibility" && (r.Method == http.MethodPut || r.Method == http.MethodPatch):
		var body struct {
			Visibility string `json:"visibility"`
		}
		if json.NewDecoder(r.Body).Decode(&body) != nil {
			httpx.Fail(w, http.StatusBadRequest, "friend.invalid_state")
			return
		}
		visibility, version, err := s.friends.SetVisibility(r.Context(), user.ID, body.Visibility)
		if err != nil {
			mapFriendError(w, err)
			return
		}
		httpx.OK(w, map[string]any{"visibility": visibility, "version": version})
	case path == "blocks" && r.Method == http.MethodPost:
		var body struct {
			UserID int64  `json:"userId"`
			Reason string `json:"reason"`
		}
		if json.NewDecoder(r.Body).Decode(&body) != nil {
			httpx.Fail(w, http.StatusBadRequest, "friend.invalid_request")
			return
		}
		if err := s.friends.Block(r.Context(), user.ID, body.UserID, body.Reason); err != nil {
			mapFriendError(w, err)
			return
		}
		httpx.OK(w, nil)
	case strings.HasPrefix(path, "blocks/") && r.Method == http.MethodDelete:
		id, err := strconv.ParseInt(strings.Trim(strings.TrimPrefix(path, "blocks/"), "/"), 10, 64)
		if err != nil || id <= 0 {
			httpx.Fail(w, http.StatusBadRequest, "friend.invalid_user_id")
			return
		}
		if err := s.friends.Unblock(r.Context(), user.ID, id); err != nil {
			mapFriendError(w, err)
			return
		}
		httpx.OK(w, nil)
	case (path == "reports" || path == "social/reports") && r.Method == http.MethodPost:
		var body struct {
			UserID  int64  `json:"userId"`
			Reason  string `json:"reason"`
			Details string `json:"details"`
		}
		if json.NewDecoder(r.Body).Decode(&body) != nil {
			httpx.Fail(w, http.StatusBadRequest, "friend.invalid_request")
			return
		}
		id, err := s.friends.Report(r.Context(), user.ID, body.UserID, body.Reason, body.Details)
		if err != nil {
			mapFriendError(w, err)
			return
		}
		httpx.JSON(w, http.StatusCreated, map[string]any{"id": id})
	default:
		httpx.Fail(w, http.StatusNotFound, "friend.not_found")
	}
}

// notifyFriendRequest fans out the durable in-app notification and the
// best-effort device push after the friend request transaction commits.
// Delivery failures must not turn a successfully-created request into an API
// error or make the requester retry and create duplicate state.
func (s *Server) notifyFriendRequest(ctx context.Context, request friends.FriendRequest, requesterNickname string) {
	title := "收到新的好友申请"
	content := fmt.Sprintf("%s 请求添加你为好友", strings.TrimSpace(requesterNickname))
	if strings.TrimSpace(requesterNickname) == "" {
		content = "有人请求添加你为好友"
	}
	const deepLink = "/friends/requests"
	source := fmt.Sprintf("friend-request:%d", request.ID)
	if s != nil && s.appNotifications != nil {
		if _, err := s.appNotifications.CreateForUser(
			ctx,
			request.AddresseeID,
			"friend_request",
			title,
			content,
			deepLink,
			source,
		); err != nil {
			log.Printf("friend request notification failed request=%d: %v", request.ID, err)
		}
	}
	if s == nil || s.pushStore == nil || s.pushStore.Pusher() == nil {
		return
	}
	registrationIDs, err := s.pushStore.GetRegistrationIDsByUserIDs(ctx, []int64{request.AddresseeID})
	if err != nil {
		log.Printf("friend request device lookup failed request=%d: %v", request.ID, err)
		return
	}
	if len(registrationIDs) == 0 {
		return
	}
	if _, err := s.pushStore.Pusher().Push(ctx, registrationIDs, push.Message{
		Title: title, Content: content, DeepLink: deepLink,
	}); err != nil {
		log.Printf("friend request push failed request=%d: %v", request.ID, err)
	}
}

func mapFriendError(w http.ResponseWriter, err error) {
	status := http.StatusInternalServerError
	switch {
	case errors.Is(err, friends.ErrInvalidUserID), errors.Is(err, friends.ErrSelfRelation), errors.Is(err, friends.ErrInvalidState):
		status = http.StatusBadRequest
	case errors.Is(err, friends.ErrBlocked), errors.Is(err, friends.ErrForbidden):
		status = http.StatusForbidden
	case errors.Is(err, friends.ErrNotFound):
		status = http.StatusNotFound
	}
	httpx.Fail(w, status, err.Error())
}
