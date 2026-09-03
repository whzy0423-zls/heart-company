package server

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"net/netip"
	"strings"
	"sync"
	"testing"
	"time"
	"unicode"

	"nine-xing/nx-backend/apps/server/internal/auth"
	"nine-xing/nx-backend/apps/server/internal/config"
	"nine-xing/nx-backend/apps/server/internal/location"
)

type fakeLocationService struct {
	mu          sync.Mutex
	searchInput []location.SearchInput
	reverseIn   []location.ReverseInput
	searchItems []location.Candidate
	reverseItem *location.Candidate
	searchErr   error
	reverseErr  error
}

func (f *fakeLocationService) Search(_ context.Context, input location.SearchInput) ([]location.Candidate, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.searchInput = append(f.searchInput, input)
	return f.searchItems, f.searchErr
}

func (f *fakeLocationService) Reverse(_ context.Context, input location.ReverseInput) (*location.Candidate, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.reverseIn = append(f.reverseIn, input)
	return f.reverseItem, f.reverseErr
}

func withLocationUser(req *http.Request, id int64) *http.Request {
	return req.WithContext(contextWithAppUser(req.Context(), auth.UserInfo{ID: id, TokenKind: auth.TokenKindApp}))
}

func decodeLocationResponse(t *testing.T, recorder *httptest.ResponseRecorder) map[string]any {
	t.Helper()
	var payload map[string]any
	if err := json.Unmarshal(recorder.Body.Bytes(), &payload); err != nil {
		t.Fatalf("decode response: %v body=%s", err, recorder.Body.String())
	}
	return payload
}

func assertLocationError(t *testing.T, recorder *httptest.ResponseRecorder, status int, code, message string) {
	t.Helper()
	if recorder.Code != status {
		t.Fatalf("status=%d want=%d body=%s", recorder.Code, status, recorder.Body.String())
	}
	payload := decodeLocationResponse(t, recorder)
	if payload["errorCode"] != code {
		t.Fatalf("errorCode=%v want=%s body=%s", payload["errorCode"], code, recorder.Body.String())
	}
	if payload["message"] != message || payload["error"] != message {
		t.Fatalf("message/error=%v/%v want=%q", payload["message"], payload["error"], message)
	}
	hasHan := false
	for _, r := range message {
		if unicode.Is(unicode.Han, r) {
			hasHan = true
			break
		}
	}
	if !hasHan {
		t.Fatalf("test message must be Chinese: %q", message)
	}
	if got := recorder.Header().Get("Cache-Control"); got != "private, no-store" {
		t.Fatalf("Cache-Control=%q want private, no-store", got)
	}
}

func TestAppLocationSearchReturnsBoundedCandidatesWithoutCaching(t *testing.T) {
	service := &fakeLocationService{searchItems: []location.Candidate{{
		Name: "学校", Address: "主路", Province: "广东省", City: "深圳市", District: "南山区",
		Latitude: 22.59, Longitude: 113.97, CoordinateSystem: location.CoordinateSystemGCJ02,
	}}}
	s := &Server{locationService: service}
	req := withLocationUser(httptest.NewRequest(http.MethodPost, "/api/app/locations/search", strings.NewReader(`{"query":" 深圳\n大学 ","city":" 深圳市 "}`)), 7)
	res := httptest.NewRecorder()
	s.appLocationSearch(res, req)
	if res.Code != http.StatusOK {
		t.Fatalf("status=%d body=%s", res.Code, res.Body.String())
	}
	if got := res.Header().Get("Cache-Control"); got != "private, no-store" {
		t.Fatalf("Cache-Control=%q", got)
	}
	payload := decodeLocationResponse(t, res)
	data := payload["data"].(map[string]any)
	items := data["items"].([]any)
	if len(items) != 1 || items[0].(map[string]any)["coordinateSystem"] != location.CoordinateSystemGCJ02 {
		t.Fatalf("unexpected items: %#v", items)
	}
	service.mu.Lock()
	defer service.mu.Unlock()
	if len(service.searchInput) != 1 || service.searchInput[0].Query != "深圳大学" || service.searchInput[0].City != "深圳市" {
		t.Fatalf("service input=%+v", service.searchInput)
	}
}

func TestAppLocationSearchEmptyResultIsSuccessfulEmptyArray(t *testing.T) {
	s := &Server{locationService: &fakeLocationService{searchItems: nil}}
	req := withLocationUser(httptest.NewRequest(http.MethodPost, "/api/app/locations/search", strings.NewReader(`{"query":"不存在的地点"}`)), 7)
	res := httptest.NewRecorder()
	s.appLocationSearch(res, req)
	if res.Code != http.StatusOK {
		t.Fatalf("status=%d body=%s", res.Code, res.Body.String())
	}
	payload := decodeLocationResponse(t, res)
	items := payload["data"].(map[string]any)["items"].([]any)
	if items == nil || len(items) != 0 {
		t.Fatalf("items=%#v want empty array", items)
	}
}

func TestAppLocationReverseReturnsNullableCandidate(t *testing.T) {
	s := &Server{locationService: &fakeLocationService{reverseItem: nil}}
	req := withLocationUser(httptest.NewRequest(http.MethodPost, "/api/app/locations/reverse", strings.NewReader(`{"latitude":22.59,"longitude":113.97,"coordinateSystem":"gcj02"}`)), 8)
	res := httptest.NewRecorder()
	s.appLocationReverse(res, req)
	if res.Code != http.StatusOK {
		t.Fatalf("status=%d body=%s", res.Code, res.Body.String())
	}
	payload := decodeLocationResponse(t, res)
	if candidate := payload["data"].(map[string]any)["candidate"]; candidate != nil {
		t.Fatalf("candidate=%#v want null", candidate)
	}
}

func TestAppLocationRejectsMalformedTrailingAndOversizedBodies(t *testing.T) {
	tests := []struct {
		name string
		body string
	}{
		{name: "empty", body: ""},
		{name: "trailing json", body: `{"query":"学校"}{"query":"别处"}`},
		{name: "trailing garbage", body: `{"query":"学校"} garbage`},
		{name: "oversized", body: `{"query":"` + strings.Repeat("字", 4100) + `"}`},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			s := &Server{locationService: &fakeLocationService{}}
			req := withLocationUser(httptest.NewRequest(http.MethodPost, "/api/app/locations/search", strings.NewReader(tt.body)), 7)
			res := httptest.NewRecorder()
			s.appLocationSearch(res, req)
			assertLocationError(t, res, http.StatusBadRequest, "LOCATION_INVALID_REQUEST", "地点信息有误，请检查后重试")
		})
	}
}

func TestAppLocationRejectsInvalidSearchAndReverseBoundaries(t *testing.T) {
	searchBodies := []string{
		`{"query":""}`,
		`{"query":"` + strings.Repeat("字", 81) + `"}`,
		`{"query":"学校","city":"` + strings.Repeat("市", 41) + `"}`,
	}
	for _, body := range searchBodies {
		s := &Server{locationService: &fakeLocationService{}}
		res := httptest.NewRecorder()
		s.appLocationSearch(res, withLocationUser(httptest.NewRequest(http.MethodPost, "/api/app/locations/search", strings.NewReader(body)), 7))
		assertLocationError(t, res, http.StatusBadRequest, "LOCATION_INVALID_REQUEST", "地点信息有误，请检查后重试")
	}

	reverseBodies := []string{
		`{"latitude":91,"longitude":0,"coordinateSystem":"gcj02"}`,
		`{"latitude":0,"longitude":181,"coordinateSystem":"gcj02"}`,
		`{"latitude":0,"longitude":0,"coordinateSystem":"wgs84"}`,
		`{"latitude":NaN,"longitude":0,"coordinateSystem":"gcj02"}`,
	}
	for _, body := range reverseBodies {
		s := &Server{locationService: &fakeLocationService{}}
		res := httptest.NewRecorder()
		s.appLocationReverse(res, withLocationUser(httptest.NewRequest(http.MethodPost, "/api/app/locations/reverse", strings.NewReader(body)), 7))
		assertLocationError(t, res, http.StatusBadRequest, "LOCATION_INVALID_REQUEST", "地点信息有误，请检查后重试")
	}
}

func TestAppLocationMapsProviderErrorsToStableChineseResponses(t *testing.T) {
	tests := []struct {
		name    string
		err     error
		status  int
		code    string
		message string
		reverse bool
	}{
		{name: "not configured", err: location.ErrNotConfigured, status: http.StatusServiceUnavailable, code: "LOCATION_NOT_CONFIGURED", message: "地点服务暂未配置，你可以先手动填写"},
		{name: "timeout", err: location.ErrTimeout, status: http.StatusGatewayTimeout, code: "LOCATION_UPSTREAM_TIMEOUT", message: "地点服务暂时不可用，你可以手动填写地点名称", reverse: true},
		{name: "unavailable", err: location.ErrUnavailable, status: http.StatusServiceUnavailable, code: "LOCATION_UPSTREAM_UNAVAILABLE", message: "地点服务暂时不可用，你可以手动填写地点名称"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			service := &fakeLocationService{searchErr: tt.err, reverseErr: tt.err}
			s := &Server{locationService: service}
			var res *httptest.ResponseRecorder
			if tt.reverse {
				res = httptest.NewRecorder()
				s.appLocationReverse(res, withLocationUser(httptest.NewRequest(http.MethodPost, "/api/app/locations/reverse", strings.NewReader(`{"latitude":0,"longitude":0,"coordinateSystem":"gcj02"}`)), 7))
			} else {
				res = httptest.NewRecorder()
				s.appLocationSearch(res, withLocationUser(httptest.NewRequest(http.MethodPost, "/api/app/locations/search", strings.NewReader(`{"query":"学校"}`)), 7))
			}
			assertLocationError(t, res, tt.status, tt.code, tt.message)
			if strings.Contains(res.Body.String(), "location provider") || strings.Contains(res.Body.String(), "TOKEN") {
				t.Fatalf("provider detail leaked: %s", res.Body.String())
			}
		})
	}
}

func TestAppLocationMethodAndAuthFailuresAreNoStore(t *testing.T) {
	s := &Server{env: config.Env{JWTSecret: "location-auth-secret"}, mux: http.NewServeMux()}
	s.routes()
	for _, tt := range []struct {
		name   string
		method string
		path   string
		body   string
		status int
	}{
		{name: "missing auth search", method: http.MethodPost, path: "/api/app/locations/search", body: `{"query":"学校"}`, status: http.StatusUnauthorized},
		{name: "missing auth reverse", method: http.MethodPost, path: "/api/app/locations/reverse", body: `{"latitude":0,"longitude":0,"coordinateSystem":"gcj02"}`, status: http.StatusUnauthorized},
		{name: "wrong method", method: http.MethodGet, path: "/api/app/locations/search", status: http.StatusMethodNotAllowed},
	} {
		t.Run(tt.name, func(t *testing.T) {
			res := httptest.NewRecorder()
			s.mux.ServeHTTP(res, httptest.NewRequest(tt.method, tt.path, strings.NewReader(tt.body)))
			if res.Code != tt.status {
				t.Fatalf("status=%d want=%d body=%s", res.Code, tt.status, res.Body.String())
			}
			if got := res.Header().Get("Cache-Control"); got != "private, no-store" {
				t.Fatalf("Cache-Control=%q want private, no-store", got)
			}
		})
	}
}

func TestAppLocationRateLimitsByUserAndTrustedClientIP(t *testing.T) {
	now := time.Date(2026, 8, 31, 10, 0, 0, 0, time.UTC)
	service := &fakeLocationService{searchItems: []location.Candidate{}}
	s := &Server{
		locationService:         service,
		locationSearchLimiter:   newFixedWindowRateLimiter(1, time.Minute),
		locationSearchIPLimiter: newBoundedStrRateLimiter(1, time.Minute, 10),
		now:                     func() time.Time { return now },
		trustedProxyCIDRs:       []netip.Prefix{netip.MustParsePrefix("127.0.0.1/32")},
	}
	request := func(userID int64, forwarded string) *httptest.ResponseRecorder {
		req := withLocationUser(httptest.NewRequest(http.MethodPost, "/api/app/locations/search", strings.NewReader(`{"query":"学校"}`)), userID)
		req.RemoteAddr = "127.0.0.1:1234"
		req.Header.Set("X-Forwarded-For", forwarded)
		res := httptest.NewRecorder()
		s.appLocationSearch(res, req)
		return res
	}
	if first := request(1, "198.51.100.1"); first.Code != http.StatusOK {
		t.Fatalf("first request status=%d body=%s", first.Code, first.Body.String())
	}
	second := request(1, "198.51.100.2")
	assertLocationError(t, second, http.StatusTooManyRequests, "LOCATION_RATE_LIMITED", "操作有些频繁，请稍后再试")
	third := request(2, "198.51.100.1")
	assertLocationError(t, third, http.StatusTooManyRequests, "LOCATION_RATE_LIMITED", "操作有些频繁，请稍后再试")
	// A different trusted client IP can pass the IP limiter, but the user
	// limiter still blocks the original user. This also proves forwarded IP is
	// considered only through the configured trusted proxy.
	if got := s.clientIP(httptest.NewRequest(http.MethodPost, "/", nil)); got == "198.51.100.1" {
		t.Fatal("untrusted request unexpectedly used forwarded IP")
	}
}

func TestAppLocationLimiterUsesSeparateReverseBudget(t *testing.T) {
	now := time.Date(2026, 8, 31, 10, 0, 0, 0, time.UTC)
	s := &Server{
		locationService:          &fakeLocationService{reverseItem: nil},
		locationReverseLimiter:   newFixedWindowRateLimiter(1, time.Minute),
		locationReverseIPLimiter: newBoundedStrRateLimiter(1, time.Minute, 10),
		now:                      func() time.Time { return now },
	}
	request := func() *httptest.ResponseRecorder {
		req := withLocationUser(httptest.NewRequest(http.MethodPost, "/api/app/locations/reverse", strings.NewReader(`{"latitude":0,"longitude":0,"coordinateSystem":"gcj02"}`)), 9)
		req.RemoteAddr = "198.51.100.9:1234"
		res := httptest.NewRecorder()
		s.appLocationReverse(res, req)
		return res
	}
	if first := request(); first.Code != http.StatusOK {
		t.Fatalf("first reverse status=%d body=%s", first.Code, first.Body.String())
	}
	assertLocationError(t, request(), http.StatusTooManyRequests, "LOCATION_RATE_LIMITED", "操作有些频繁，请稍后再试")
}

func TestAppLocationNilServiceReturnsNotConfigured(t *testing.T) {
	s := &Server{}
	res := httptest.NewRecorder()
	s.appLocationSearch(res, withLocationUser(httptest.NewRequest(http.MethodPost, "/api/app/locations/search", strings.NewReader(`{"query":"学校"}`)), 7))
	assertLocationError(t, res, http.StatusServiceUnavailable, "LOCATION_NOT_CONFIGURED", "地点服务暂未配置，你可以先手动填写")
}

func TestAppLocationErrorMappingDoesNotExposeArbitraryErrors(t *testing.T) {
	secretErr := fmt.Errorf("upstream key TOKEN and query 学校")
	s := &Server{locationService: &fakeLocationService{searchErr: secretErr}}
	res := httptest.NewRecorder()
	s.appLocationSearch(res, withLocationUser(httptest.NewRequest(http.MethodPost, "/api/app/locations/search", strings.NewReader(`{"query":"学校"}`)), 7))
	if res.Code != http.StatusServiceUnavailable {
		t.Fatalf("status=%d body=%s", res.Code, res.Body.String())
	}
	if strings.Contains(res.Body.String(), "TOKEN") || strings.Contains(res.Body.String(), "学校") {
		t.Fatalf("sensitive provider error leaked: %s", res.Body.String())
	}
}
