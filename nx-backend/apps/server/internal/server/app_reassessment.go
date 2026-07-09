package server

import (
	"context"
	"net/http"
	"strconv"
	"strings"

	"nine-xing/nx-backend/apps/server/internal/httpx"
)

// appReassessmentService is the narrow App-facing contract implemented by
// internal/profilecalibration. Detail/Accept/Reject must scope by appUserID so
// foreign report IDs are returned as not found.
type appReassessmentService interface {
	Latest(ctx context.Context, appUserID, cardID int64) (any, error)
	Detail(ctx context.Context, appUserID, id int64) (any, error)
	Accept(ctx context.Context, appUserID, id int64) (any, error)
	Reject(ctx context.Context, appUserID, id int64) (any, error)
}

func (s *Server) appReassessmentLatest(w http.ResponseWriter, r *http.Request) {
	userInfo, ok := appUserFromContext(r)
	if !ok {
		httpx.Fail(w, http.StatusUnauthorized, "unauthorized")
		return
	}
	cardID, ok := appCalibrationPositiveQueryID(w, r, "cardId", "invalid card id")
	if !ok {
		return
	}
	if s.appReassessment == nil {
		httpx.Fail(w, http.StatusServiceUnavailable, "profile calibration unavailable")
		return
	}
	report, err := s.appReassessment.Latest(r.Context(), userInfo.ID, cardID)
	if appCalibrationFail(w, err, "query failed", true) {
		return
	}
	httpx.OK(w, report)
}

func (s *Server) appReassessmentRouter(w http.ResponseWriter, r *http.Request) {
	path := strings.TrimRight(r.URL.Path, "/")
	idText, action, ok := parseAppReassessmentPath(path)
	if !ok {
		httpx.Fail(w, http.StatusNotFound, "not found")
		return
	}
	if action == "" {
		if r.Method != http.MethodGet {
			httpx.Fail(w, http.StatusMethodNotAllowed, "method not allowed")
			return
		}
		s.appReassessmentDetail(w, r, idText)
		return
	}
	if r.Method != http.MethodPost {
		httpx.Fail(w, http.StatusMethodNotAllowed, "method not allowed")
		return
	}
	switch action {
	case "accept":
		s.appReassessmentAccept(w, r, idText)
	case "reject":
		s.appReassessmentReject(w, r, idText)
	default:
		httpx.Fail(w, http.StatusNotFound, "not found")
	}
}

func (s *Server) appReassessmentDetail(w http.ResponseWriter, r *http.Request, idText string) {
	userInfo, ok := appUserFromContext(r)
	if !ok {
		httpx.Fail(w, http.StatusUnauthorized, "unauthorized")
		return
	}
	id, ok := appCalibrationParsePositiveID(w, idText, "invalid reassessment id")
	if !ok {
		return
	}
	if s.appReassessment == nil {
		httpx.Fail(w, http.StatusServiceUnavailable, "profile calibration unavailable")
		return
	}
	report, err := s.appReassessment.Detail(r.Context(), userInfo.ID, id)
	if appCalibrationFail(w, err, "query failed", false) {
		return
	}
	httpx.OK(w, report)
}

func (s *Server) appReassessmentAccept(w http.ResponseWriter, r *http.Request, idText string) {
	userInfo, ok := appUserFromContext(r)
	if !ok {
		httpx.Fail(w, http.StatusUnauthorized, "unauthorized")
		return
	}
	id, ok := appCalibrationParsePositiveID(w, idText, "invalid reassessment id")
	if !ok {
		return
	}
	if s.appReassessment == nil {
		httpx.Fail(w, http.StatusServiceUnavailable, "profile calibration unavailable")
		return
	}
	result, err := s.appReassessment.Accept(r.Context(), userInfo.ID, id)
	if appCalibrationFail(w, err, "accept failed", false) {
		return
	}
	httpx.OK(w, result)
}

func (s *Server) appReassessmentReject(w http.ResponseWriter, r *http.Request, idText string) {
	userInfo, ok := appUserFromContext(r)
	if !ok {
		httpx.Fail(w, http.StatusUnauthorized, "unauthorized")
		return
	}
	id, ok := appCalibrationParsePositiveID(w, idText, "invalid reassessment id")
	if !ok {
		return
	}
	if s.appReassessment == nil {
		httpx.Fail(w, http.StatusServiceUnavailable, "profile calibration unavailable")
		return
	}
	result, err := s.appReassessment.Reject(r.Context(), userInfo.ID, id)
	if appCalibrationFail(w, err, "reject failed", false) {
		return
	}
	httpx.OK(w, result)
}

func parseAppReassessmentPath(path string) (idText string, action string, ok bool) {
	rest := strings.Trim(strings.TrimPrefix(path, "/api/app/reassessment/"), "/")
	if rest == "" || rest == path {
		return "", "", false
	}
	parts := strings.Split(rest, "/")
	switch len(parts) {
	case 1:
		return parts[0], "", true
	case 2:
		return parts[0], parts[1], true
	default:
		return "", "", false
	}
}

func appCalibrationParsePositiveID(w http.ResponseWriter, raw, message string) (int64, bool) {
	id, err := strconv.ParseInt(strings.TrimSpace(raw), 10, 64)
	if err != nil || id <= 0 {
		httpx.Fail(w, http.StatusBadRequest, message)
		return 0, false
	}
	return id, true
}
