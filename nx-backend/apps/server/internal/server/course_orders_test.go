package server

import (
	"net/http/httptest"
	"strings"
	"testing"

	"nine-xing/nx-backend/apps/server/internal/miniapp"
)

func TestParseBookingID(t *testing.T) {
	for _, raw := range []string{"", "0", "-1", "x", "1.2"} {
		if _, err := parseBookingID(raw); err == nil {
			t.Fatalf("parseBookingID(%q) accepted", raw)
		}
	}
	if got, err := parseBookingID(" 42 "); err != nil || got != 42 {
		t.Fatalf("got %d err %v", got, err)
	}
}

func TestCourseOrderErrorMapping(t *testing.T) {
	tests := []struct {
		err    error
		status int
		body   string
	}{
		{errCourseBookingPriceChanged, 409, "课程价格已调整"},
		{miniapp.ErrOrderAlreadyOwned, 409, "已支付"},
	}
	for _, tt := range tests {
		rr := httptest.NewRecorder()
		writeCourseOrderError(rr, tt.err)
		if rr.Code != tt.status || !strings.Contains(rr.Body.String(), tt.body) {
			t.Fatalf("err=%v status=%d body=%s", tt.err, rr.Code, rr.Body.String())
		}
	}
}
