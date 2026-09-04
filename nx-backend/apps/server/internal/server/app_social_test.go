package server

import (
	"errors"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"testing"

	"nine-xing/nx-backend/apps/server/internal/friends"
)

func TestSocialRoutesAreRegistered(t *testing.T) {
	raw, err := os.ReadFile("server.go")
	if err != nil {
		t.Fatal(err)
	}
	source := string(raw)
	for _, route := range []string{
		"/api/app/social",
		"/api/app/friends",
		"/api/app/friend-requests",
		"/api/app/blocks",
		"/api/app/social/reports",
	} {
		if !strings.Contains(source, `s.mux.HandleFunc("`+route+`"`) {
			t.Fatalf("social route %s is not registered", route)
		}
	}
}

func TestAppSocialRouterRequiresAuthenticationContext(t *testing.T) {
	server := &Server{}
	req := httptest.NewRequest(http.MethodGet, "/api/app/friends", nil)
	response := httptest.NewRecorder()
	server.appSocialRouter(response, req)
	if response.Code != http.StatusUnauthorized {
		t.Fatalf("expected unauthorized, got %d", response.Code)
	}
}

func TestMapFriendErrorUsesStableStatuses(t *testing.T) {
	cases := []struct {
		err    error
		status int
	}{
		{friends.ErrInvalidUserID, http.StatusBadRequest},
		{friends.ErrSelfRelation, http.StatusBadRequest},
		{friends.ErrBlocked, http.StatusForbidden},
		{friends.ErrNotFound, http.StatusNotFound},
		{errors.New("db down"), http.StatusInternalServerError},
	}
	for _, tc := range cases {
		response := httptest.NewRecorder()
		mapFriendError(response, tc.err)
		if response.Code != tc.status {
			t.Errorf("%v: expected %d, got %d", tc.err, tc.status, response.Code)
		}
	}
}
