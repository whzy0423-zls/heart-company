package server

import (
	"errors"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"testing"

	"nine-xing/nx-backend/apps/server/internal/directmessage"
)

func TestDirectMessageRoutesAreRegistered(t *testing.T) {
	raw, err := os.ReadFile("server.go")
	if err != nil {
		t.Fatal(err)
	}
	source := string(raw)
	for _, route := range []string{"/api/app/direct/conversations", "/api/app/direct/messages/"} {
		if !strings.Contains(source, `s.mux.HandleFunc("`+route+`"`) {
			t.Fatalf("missing route %s", route)
		}
	}
}

func TestAppDirectMessageRouterRequiresAuthentication(t *testing.T) {
	server := &Server{}
	response := httptest.NewRecorder()
	server.appDirectMessageRouter(response, httptest.NewRequest(http.MethodGet, "/api/app/direct/conversations", nil))
	if response.Code != http.StatusUnauthorized {
		t.Fatalf("expected unauthorized, got %d", response.Code)
	}
}

func TestMapDirectMessageErrorUsesStableStatuses(t *testing.T) {
	cases := []struct {
		err    error
		status int
	}{{directmessage.ErrCursorConflict, http.StatusBadRequest}, {directmessage.ErrPayloadConflict, http.StatusConflict}, {directmessage.ErrBlocked, http.StatusForbidden}, {directmessage.ErrRecallWindow, http.StatusUnprocessableEntity}, {errors.New("db"), http.StatusInternalServerError}}
	for _, tc := range cases {
		response := httptest.NewRecorder()
		mapDirectMessageError(response, tc.err)
		if response.Code != tc.status {
			t.Errorf("%v: expected %d got %d", tc.err, tc.status, response.Code)
		}
	}
}
