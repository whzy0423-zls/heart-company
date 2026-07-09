package server

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"net/http"
	"strconv"
	"strings"

	"nine-xing/nx-backend/apps/server/internal/httpx"
	"nine-xing/nx-backend/apps/server/internal/profilecalibration"
)

// appDailyQuizService is the narrow App-facing contract implemented by
// internal/profilecalibration. Each method must enforce ownership using
// appUserID so a user can only access their own card/batch resources.
type appDailyQuizService interface {
	TodayBatch(ctx context.Context, appUserID, cardID int64) (any, error)
	Progress(ctx context.Context, appUserID, cardID int64) (any, error)
	SubmitAnswer(ctx context.Context, appUserID, batchID, questionID int64, optionID string) (any, error)
	CompleteBatch(ctx context.Context, appUserID, batchID int64) (any, error)
}

type appDailyQuizAnswerRequest struct {
	BatchID    int64  `json:"batchId"`
	QuestionID int64  `json:"questionId"`
	OptionID   string `json:"optionId"`
}

type appDailyQuizCompleteRequest struct {
	BatchID int64 `json:"batchId"`
}

func newAppProfileCalibrationServices(database *sql.DB) (appDailyQuizService, appReassessmentService, appDailyQuizReminderService) {
	if database == nil {
		return nil, nil, nil
	}
	store := profilecalibration.NewStore(database)
	return store, store, store
}

func (s *Server) appDailyQuizToday(w http.ResponseWriter, r *http.Request) {
	userInfo, ok := appUserFromContext(r)
	if !ok {
		httpx.Fail(w, http.StatusUnauthorized, "unauthorized")
		return
	}
	cardID, ok := appCalibrationPositiveQueryID(w, r, "cardId", "invalid card id")
	if !ok {
		return
	}
	if s.appDailyQuiz == nil {
		httpx.Fail(w, http.StatusServiceUnavailable, "profile calibration unavailable")
		return
	}
	batch, err := s.appDailyQuiz.TodayBatch(r.Context(), userInfo.ID, cardID)
	if appCalibrationFail(w, err, "query failed", false) {
		return
	}
	httpx.OK(w, batch)
}

func (s *Server) appDailyQuizProgress(w http.ResponseWriter, r *http.Request) {
	userInfo, ok := appUserFromContext(r)
	if !ok {
		httpx.Fail(w, http.StatusUnauthorized, "unauthorized")
		return
	}
	cardID, ok := appCalibrationPositiveQueryID(w, r, "cardId", "invalid card id")
	if !ok {
		return
	}
	if s.appDailyQuiz == nil {
		httpx.Fail(w, http.StatusServiceUnavailable, "profile calibration unavailable")
		return
	}
	progress, err := s.appDailyQuiz.Progress(r.Context(), userInfo.ID, cardID)
	if appCalibrationFail(w, err, "query failed", false) {
		return
	}
	httpx.OK(w, progress)
}

func (s *Server) appDailyQuizAnswer(w http.ResponseWriter, r *http.Request) {
	userInfo, ok := appUserFromContext(r)
	if !ok {
		httpx.Fail(w, http.StatusUnauthorized, "unauthorized")
		return
	}
	var input appDailyQuizAnswerRequest
	if err := json.NewDecoder(r.Body).Decode(&input); err != nil {
		httpx.Fail(w, http.StatusBadRequest, "invalid request body")
		return
	}
	input.OptionID = strings.TrimSpace(input.OptionID)
	if input.BatchID <= 0 {
		httpx.Fail(w, http.StatusBadRequest, "invalid batch id")
		return
	}
	if input.QuestionID <= 0 {
		httpx.Fail(w, http.StatusBadRequest, "invalid question id")
		return
	}
	if input.OptionID == "" {
		httpx.Fail(w, http.StatusBadRequest, "option id required")
		return
	}
	if s.appDailyQuiz == nil {
		httpx.Fail(w, http.StatusServiceUnavailable, "profile calibration unavailable")
		return
	}
	result, err := s.appDailyQuiz.SubmitAnswer(r.Context(), userInfo.ID, input.BatchID, input.QuestionID, input.OptionID)
	if appCalibrationFail(w, err, "submit failed", false) {
		return
	}
	httpx.OK(w, result)
}

func (s *Server) appDailyQuizComplete(w http.ResponseWriter, r *http.Request) {
	userInfo, ok := appUserFromContext(r)
	if !ok {
		httpx.Fail(w, http.StatusUnauthorized, "unauthorized")
		return
	}
	var input appDailyQuizCompleteRequest
	if err := json.NewDecoder(r.Body).Decode(&input); err != nil {
		httpx.Fail(w, http.StatusBadRequest, "invalid request body")
		return
	}
	if input.BatchID <= 0 {
		httpx.Fail(w, http.StatusBadRequest, "invalid batch id")
		return
	}
	if s.appDailyQuiz == nil {
		httpx.Fail(w, http.StatusServiceUnavailable, "profile calibration unavailable")
		return
	}
	result, err := s.appDailyQuiz.CompleteBatch(r.Context(), userInfo.ID, input.BatchID)
	if appCalibrationFail(w, err, "complete failed", false) {
		return
	}
	httpx.OK(w, result)
}

func appCalibrationPositiveQueryID(w http.ResponseWriter, r *http.Request, key, message string) (int64, bool) {
	id, err := strconv.ParseInt(strings.TrimSpace(r.URL.Query().Get(key)), 10, 64)
	if err != nil || id <= 0 {
		httpx.Fail(w, http.StatusBadRequest, message)
		return 0, false
	}
	return id, true
}

func appCalibrationFail(w http.ResponseWriter, err error, fallback string, notFoundAsOKNil bool) bool {
	if err == nil {
		return false
	}
	if appCalibrationIsNotFound(err) {
		if notFoundAsOKNil {
			httpx.OK(w, nil)
		} else {
			httpx.Fail(w, http.StatusNotFound, "not found")
		}
		return true
	}
	if status, message, ok := appCalibrationPublicError(err); ok {
		httpx.Fail(w, status, message)
		return true
	}
	httpx.Fail(w, http.StatusInternalServerError, fallback)
	return true
}

func appCalibrationIsNotFound(err error) bool {
	if errors.Is(err, sql.ErrNoRows) {
		return true
	}
	message := strings.ToLower(strings.TrimSpace(err.Error()))
	return message == "profilecalibration: not found" ||
		message == "profile calibration: not found" ||
		strings.Contains(message, "not found")
}

func appCalibrationPublicError(err error) (int, string, bool) {
	if err == nil {
		return 0, "", false
	}
	if statusErr, ok := err.(interface {
		StatusCode() int
		Error() string
	}); ok {
		status := statusErr.StatusCode()
		if status >= 400 && status < 500 {
			return status, statusErr.Error(), true
		}
	}
	message := strings.TrimSpace(err.Error())
	lower := strings.ToLower(message)
	switch {
	case strings.HasPrefix(lower, "profilecalibration: invalid"),
		strings.HasPrefix(lower, "profile calibration: invalid"),
		strings.Contains(lower, "bad request"):
		return http.StatusBadRequest, message, true
	case strings.HasPrefix(lower, "profilecalibration: conflict"),
		strings.HasPrefix(lower, "profile calibration: conflict"):
		return http.StatusConflict, message, true
	}
	return 0, "", false
}
