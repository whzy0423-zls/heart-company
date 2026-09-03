package server

import (
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"time"

	"nine-xing/nx-backend/apps/server/internal/httpx"
	"nine-xing/nx-backend/apps/server/internal/location"
)

const appLocationBodyLimit int64 = 4 * 1024

const (
	locationErrorInvalidRequest      = "LOCATION_INVALID_REQUEST"
	locationErrorNotConfigured       = "LOCATION_NOT_CONFIGURED"
	locationErrorRateLimited         = "LOCATION_RATE_LIMITED"
	locationErrorUpstreamTimeout     = "LOCATION_UPSTREAM_TIMEOUT"
	locationErrorUpstreamUnavailable = "LOCATION_UPSTREAM_UNAVAILABLE"
)

const (
	locationSearchUserLimit   = 30
	locationReverseUserLimit  = 60
	locationSearchIPLimit     = 120
	locationReverseIPLimit    = 240
	locationRateLimitWindow   = time.Minute
	locationRateLimiterMaxKey = 20_000
)

// Exported aliases keep the public error vocabulary available to integration
// tests and clients without exposing provider-specific implementation types.
const (
	LocationErrorInvalidRequest      = locationErrorInvalidRequest
	LocationErrorNotConfigured       = locationErrorNotConfigured
	LocationErrorRateLimited         = locationErrorRateLimited
	LocationErrorUpstreamTimeout     = locationErrorUpstreamTimeout
	LocationErrorUpstreamUnavailable = locationErrorUpstreamUnavailable
)

var appLocationErrorMessages = map[string]string{
	locationErrorInvalidRequest:      "地点信息有误，请检查后重试",
	locationErrorNotConfigured:       "地点服务暂未配置，你可以先手动填写",
	locationErrorRateLimited:         "操作有些频繁，请稍后再试",
	locationErrorUpstreamTimeout:     "地点服务暂时不可用，你可以手动填写地点名称",
	locationErrorUpstreamUnavailable: "地点服务暂时不可用，你可以手动填写地点名称",
}

// appLocationNoStore sets the cache policy before authentication and method
// checks run, so error responses cannot be cached by a proxy.
func appLocationNoStore(next http.HandlerFunc) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Cache-Control", "private, no-store")
		next(w, r)
	}
}

func appLocationFail(w http.ResponseWriter, status int, code string) {
	w.Header().Set("Cache-Control", "private, no-store")
	message := appLocationErrorMessages[code]
	if message == "" {
		code = locationErrorUpstreamUnavailable
		message = appLocationErrorMessages[code]
	}
	httpx.JSON(w, status, map[string]any{
		"code":      -1,
		"data":      nil,
		"error":     message,
		"errorCode": code,
		"message":   message,
	})
}

func appLocationUnauthorized(w http.ResponseWriter) {
	w.Header().Set("Cache-Control", "private, no-store")
	httpx.Fail(w, http.StatusUnauthorized, "unauthorized")
}

func (s *Server) appLocationSearch(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Cache-Control", "private, no-store")
	if r == nil {
		appLocationFail(w, http.StatusBadRequest, locationErrorInvalidRequest)
		return
	}
	if r.Method != http.MethodPost {
		w.Header().Set("Allow", http.MethodPost)
		appLocationFail(w, http.StatusMethodNotAllowed, locationErrorInvalidRequest)
		return
	}
	user, ok := appUserFromContext(r)
	if !ok || user.ID <= 0 {
		appLocationUnauthorized(w)
		return
	}

	var input location.SearchInput
	if !decodeAppLocationBody(w, r, &input) {
		return
	}
	normalized, err := location.NormalizeSearchInput(input)
	if err != nil {
		appLocationFail(w, http.StatusBadRequest, locationErrorInvalidRequest)
		return
	}
	service := s.appLocationService()
	if service == nil {
		appLocationFail(w, http.StatusServiceUnavailable, locationErrorNotConfigured)
		return
	}
	if !s.allowAppLocationSearch(user.ID, s.clientIP(r), s.locationNow()) {
		appLocationFail(w, http.StatusTooManyRequests, locationErrorRateLimited)
		return
	}
	items, err := service.Search(r.Context(), normalized)
	if err != nil {
		s.writeAppLocationProviderError(w, err)
		return
	}
	// Normalize a second time at the app boundary.  This protects the handler
	// even when an injected service is not the bundled AMap implementation.
	items = location.NormalizeCandidates(items)
	if items == nil {
		items = []location.Candidate{}
	}
	httpx.OK(w, map[string]any{"items": items})
}

func (s *Server) appLocationReverse(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Cache-Control", "private, no-store")
	if r == nil {
		appLocationFail(w, http.StatusBadRequest, locationErrorInvalidRequest)
		return
	}
	if r.Method != http.MethodPost {
		w.Header().Set("Allow", http.MethodPost)
		appLocationFail(w, http.StatusMethodNotAllowed, locationErrorInvalidRequest)
		return
	}
	user, ok := appUserFromContext(r)
	if !ok || user.ID <= 0 {
		appLocationUnauthorized(w)
		return
	}

	var input location.ReverseInput
	if !decodeAppLocationBody(w, r, &input) {
		return
	}
	normalized, err := location.NormalizeReverseInput(input)
	if err != nil {
		appLocationFail(w, http.StatusBadRequest, locationErrorInvalidRequest)
		return
	}
	service := s.appLocationService()
	if service == nil {
		appLocationFail(w, http.StatusServiceUnavailable, locationErrorNotConfigured)
		return
	}
	if !s.allowAppLocationReverse(user.ID, s.clientIP(r), s.locationNow()) {
		appLocationFail(w, http.StatusTooManyRequests, locationErrorRateLimited)
		return
	}
	candidate, err := service.Reverse(r.Context(), normalized)
	if err != nil {
		s.writeAppLocationProviderError(w, err)
		return
	}
	if candidate != nil {
		normalizedCandidate, normalizeErr := location.NormalizeCandidate(*candidate)
		if normalizeErr != nil {
			// A malformed provider item is an unavailable upstream response, not
			// a reason to emit an invalid JSON shape to the app.
			appLocationFail(w, http.StatusServiceUnavailable, locationErrorUpstreamUnavailable)
			return
		}
		candidate = &normalizedCandidate
	}
	httpx.OK(w, map[string]any{"candidate": candidate})
}

func decodeAppLocationBody(w http.ResponseWriter, r *http.Request, destination any) bool {
	if r == nil {
		appLocationFail(w, http.StatusBadRequest, locationErrorInvalidRequest)
		return false
	}
	body := r.Body
	if body == nil {
		body = http.NoBody
	}
	r.Body = http.MaxBytesReader(w, body, appLocationBodyLimit)
	decoder := json.NewDecoder(r.Body)
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(destination); err != nil {
		appLocationFail(w, http.StatusBadRequest, locationErrorInvalidRequest)
		return false
	}
	// A second JSON value or any non-whitespace trailing bytes are rejected.
	var trailing any
	if err := decoder.Decode(&trailing); !errors.Is(err, io.EOF) {
		appLocationFail(w, http.StatusBadRequest, locationErrorInvalidRequest)
		return false
	}
	return true
}

func (s *Server) appLocationService() location.Service {
	if s == nil {
		return nil
	}
	return s.locationService
}

func (s *Server) locationNow() time.Time {
	if s == nil {
		return time.Now()
	}
	return s.nowTime()
}

func (s *Server) allowAppLocationSearch(userID int64, ip string, now time.Time) bool {
	if s == nil {
		return true
	}
	// Check IP first so a single abusive source cannot consume user capacity
	// across many accounts.  A failed user check intentionally leaves the IP
	// accounting in place for the current window.
	if s.locationSearchIPLimiter != nil && !s.locationSearchIPLimiter.Allow(ip, now) {
		return false
	}
	if s.locationSearchLimiter != nil && !s.locationSearchLimiter.Allow(userID, now) {
		return false
	}
	return true
}

func (s *Server) allowAppLocationReverse(userID int64, ip string, now time.Time) bool {
	if s == nil {
		return true
	}
	if s.locationReverseIPLimiter != nil && !s.locationReverseIPLimiter.Allow(ip, now) {
		return false
	}
	if s.locationReverseLimiter != nil && !s.locationReverseLimiter.Allow(userID, now) {
		return false
	}
	return true
}

func (s *Server) writeAppLocationProviderError(w http.ResponseWriter, err error) {
	switch {
	case errors.Is(err, location.ErrInvalidRequest):
		appLocationFail(w, http.StatusBadRequest, locationErrorInvalidRequest)
	case errors.Is(err, location.ErrNotConfigured):
		appLocationFail(w, http.StatusServiceUnavailable, locationErrorNotConfigured)
	case errors.Is(err, location.ErrTimeout):
		appLocationFail(w, http.StatusGatewayTimeout, locationErrorUpstreamTimeout)
	case errors.Is(err, location.ErrUnavailable):
		appLocationFail(w, http.StatusServiceUnavailable, locationErrorUpstreamUnavailable)
	default:
		// Do not include provider details, request values, or credentials in the
		// public response.  The provider client already classifies known errors.
		appLocationFail(w, http.StatusServiceUnavailable, locationErrorUpstreamUnavailable)
	}
}

// appLocationPathHandler keeps route registration compact while ensuring the
// no-store header is present for method and authentication failures alike.
func (s *Server) appLocationPathHandler(method string, next http.HandlerFunc) http.HandlerFunc {
	authenticated := s.requireAppAuth(next)
	return appLocationNoStore(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != method {
			w.Header().Set("Allow", method)
			appLocationFail(w, http.StatusMethodNotAllowed, locationErrorInvalidRequest)
			return
		}
		authenticated(w, r)
	})
}
