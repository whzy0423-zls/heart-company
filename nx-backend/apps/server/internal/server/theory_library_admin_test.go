package server

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

type fakeTheoryLibraryAdmin struct {
	dashboard theoryLibraryDashboard
	cards     []theoryLibraryCardView
	published theoryLibraryPublishResult
}

func (f *fakeTheoryLibraryAdmin) Dashboard(context.Context) (theoryLibraryDashboard, error) {
	return f.dashboard, nil
}

func (f *fakeTheoryLibraryAdmin) Cards(context.Context, int64) ([]theoryLibraryCardView, error) {
	return f.cards, nil
}

func (f *fakeTheoryLibraryAdmin) Publish(context.Context, int64, int64) (theoryLibraryPublishResult, error) {
	return f.published, nil
}

func TestTheoryLibraryAdminDashboard(t *testing.T) {
	service := &fakeTheoryLibraryAdmin{dashboard: theoryLibraryDashboard{Libraries: []theoryLibrarySummary{{ID: 1, Name: "芯之力理论库", CardCount: 52}}}}
	server := &Server{theoryAdmin: service}
	response := httptest.NewRecorder()
	server.theoryLibrariesHandler(response, httptest.NewRequest(http.MethodGet, "/api/theory-libraries", nil))
	if response.Code != http.StatusOK || !strings.Contains(response.Body.String(), `"cardCount":52`) {
		t.Fatalf("status=%d body=%s", response.Code, response.Body.String())
	}
}

func TestTheoryLibraryAdminRoutesRegistered(t *testing.T) {
	source := readServerSource(t, "server.go")
	for _, route := range []string{"/api/theory-libraries", "/api/theory-libraries/"} {
		if !strings.Contains(source, route) {
			t.Fatalf("missing theory library route %s", route)
		}
	}
	if !strings.Contains(source, `requirePermission("System:TheoryLibrary:Manage"`) {
		t.Fatal("theory library routes must require the management permission")
	}
}
