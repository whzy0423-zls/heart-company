package server

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"testing"

	"nine-xing/nx-backend/apps/server/internal/auth"
	"nine-xing/nx-backend/apps/server/internal/xinzhili"
)

func TestAppXinzhiliModeSnapshotReturnsPersistedMode(t *testing.T) {
	cfg := xinzhili.DefaultConfig()
	cfg.Enabled = true
	cfg.Version = 12
	cfg.EnabledModes = []xinzhili.Mode{xinzhili.ModeNormal, xinzhili.ModeArgument}
	s := &Server{
		xinzhiliModelConfig: &fakeXinzhiliModelConfigStore{config: cfg, found: true},
		xinzhiliModeStore: &fakeXinzhiliModeStore{
			found: true,
			preference: xinzhili.ModePreference{
				UserID: 7, Requested: xinzhili.ModeArgument, Revision: 4,
			},
		},
	}
	w := httptest.NewRecorder()
	r := httptest.NewRequest(http.MethodGet, "/api/app/xinzhili/mode", nil)
	r = r.WithContext(context.WithValue(r.Context(), appContextKey{}, auth.UserInfo{ID: 7}))

	s.appXinzhiliModeSnapshot(w, r)

	if w.Code != http.StatusOK {
		t.Fatalf("status=%d body=%s", w.Code, w.Body.String())
	}
	var body struct {
		Code int                  `json:"code"`
		Data xinzhiliModeSnapshot `json:"data"`
	}
	if err := json.Unmarshal(w.Body.Bytes(), &body); err != nil {
		t.Fatal(err)
	}
	if body.Code != 0 || body.Data.RequestedMode != xinzhili.ModeArgument ||
		body.Data.PendingMode != xinzhili.ModeArgument || body.Data.EffectiveMode != xinzhili.ModeArgument ||
		body.Data.Revision != 4 || body.Data.ConfigVersion != 12 {
		t.Fatalf("body=%s", w.Body.String())
	}
}

func TestAppXinzhiliModeSnapshotDefaultsToNormalAndNormalizesDisabledMode(t *testing.T) {
	for _, tc := range []struct {
		name  string
		store *fakeXinzhiliModeStore
	}{
		{name: "missing preference", store: &fakeXinzhiliModeStore{}},
		{name: "disabled preference", store: &fakeXinzhiliModeStore{found: true, preference: xinzhili.ModePreference{UserID: 7, Requested: xinzhili.ModeArgument, Revision: 3}}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			cfg := xinzhili.DefaultConfig()
			cfg.Enabled = true
			cfg.EnabledModes = []xinzhili.Mode{xinzhili.ModeNormal, xinzhili.ModeComfort}
			s := &Server{
				xinzhiliModelConfig: &fakeXinzhiliModelConfigStore{config: cfg, found: true},
				xinzhiliModeStore:   tc.store,
			}
			w := httptest.NewRecorder()
			r := httptest.NewRequest(http.MethodGet, "/api/app/xinzhili/mode", nil)
			r = r.WithContext(context.WithValue(r.Context(), appContextKey{}, auth.UserInfo{ID: 7}))

			s.appXinzhiliModeSnapshot(w, r)

			if w.Code != http.StatusOK || !strings.Contains(w.Body.String(), `"requestedMode":"normal"`) {
				t.Fatalf("status=%d body=%s", w.Code, w.Body.String())
			}
		})
	}
}

func TestServerRegistersAppXinzhiliModeSnapshotRoute(t *testing.T) {
	raw, err := os.ReadFile("server.go")
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(raw), `s.mux.HandleFunc("/api/app/xinzhili/mode"`) {
		t.Fatal("app xinzhili mode snapshot route is not registered")
	}
}
