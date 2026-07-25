package server

import (
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestPlatformAnalyticsOverviewRejectsInvalidDays(t *testing.T) {
	w := httptest.NewRecorder()
	(&Server{}).platformAnalyticsOverview(w, httptest.NewRequest(http.MethodGet, "/api/analytics/platform-overview?days=8", nil))
	if w.Code != http.StatusBadRequest {
		t.Fatalf("status=%d body=%s", w.Code, w.Body.String())
	}
}
