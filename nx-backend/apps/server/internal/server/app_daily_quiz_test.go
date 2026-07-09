package server

import (
	"bytes"
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"

	"nine-xing/nx-backend/apps/server/internal/auth"
)

func TestNewAppProfileCalibrationServicesCreatesCoreAdapters(t *testing.T) {
	registerAppQuizTestDriver()
	db, err := sql.Open(appQuizTestDriverName, "profile_calibration")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = db.Close() })

	daily, reassessment, reminders := newAppProfileCalibrationServices(db)

	if daily == nil || reassessment == nil || reminders == nil {
		t.Fatalf("expected non-nil daily quiz, reassessment, and reminder adapters")
	}
}

func TestAppDailyQuizTodayPassesAuthenticatedUserAndCard(t *testing.T) {
	service := &fakeDailyQuizService{
		today: map[string]any{
			"id":     int64(10),
			"cardId": int64(123),
			"date":   "2026-07-09",
			"questions": []map[string]any{{
				"id":      int64(501),
				"body":    "今天你更像哪种反应？",
				"options": []map[string]any{{"id": "A", "label": "A", "text": "先行动"}},
			}},
			"progress": map[string]any{"answered": 7, "total": 100, "todayAnswered": 0, "todayTotal": 5},
		},
	}
	s := &Server{appDailyQuiz: service}

	response := performAppCalibrationRequest(t, s.appDailyQuizToday, http.MethodGet, "/api/app/daily-quiz/today?cardId=123", nil)

	if response.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d body=%s", response.Code, response.Body.String())
	}
	if service.todayUserID != 7 || service.todayCardID != 123 {
		t.Fatalf("expected service to receive user=7 card=123, got user=%d card=%d", service.todayUserID, service.todayCardID)
	}
	body := decodeAppCalibrationResponse(t, response)
	data := body.Data.(map[string]any)
	if data["date"] != "2026-07-09" {
		t.Fatalf("expected batch payload to pass through, got %+v", data)
	}
}

func TestAppDailyQuizTodayRejectsInvalidCardID(t *testing.T) {
	s := &Server{appDailyQuiz: &fakeDailyQuizService{}}

	response := performAppCalibrationRequest(t, s.appDailyQuizToday, http.MethodGet, "/api/app/daily-quiz/today?cardId=0", nil)

	if response.Code != http.StatusBadRequest {
		t.Fatalf("expected 400 for invalid card id, got %d body=%s", response.Code, response.Body.String())
	}
}

func TestAppDailyQuizAnswerPassesAuthenticatedUserAndBody(t *testing.T) {
	service := &fakeDailyQuizService{answer: map[string]any{"accepted": true}}
	s := &Server{appDailyQuiz: service}

	response := performAppCalibrationRequest(t, s.appDailyQuizAnswer, http.MethodPost, "/api/app/daily-quiz/answer", map[string]any{
		"batchId":    88,
		"questionId": 501,
		"optionId":   "B",
	})

	if response.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d body=%s", response.Code, response.Body.String())
	}
	if service.answerUserID != 7 || service.answerBatchID != 88 || service.answerQuestionID != 501 || service.answerOptionID != "B" {
		t.Fatalf("service received wrong answer input: user=%d batch=%d question=%d option=%q", service.answerUserID, service.answerBatchID, service.answerQuestionID, service.answerOptionID)
	}
}

func TestAppDailyQuizCompleteMapsForeignBatchToNotFound(t *testing.T) {
	service := &fakeDailyQuizService{completeErr: errors.New("profilecalibration: not found")}
	s := &Server{appDailyQuiz: service}

	response := performAppCalibrationRequest(t, s.appDailyQuizComplete, http.MethodPost, "/api/app/daily-quiz/complete", map[string]any{"batchId": 88})

	if response.Code != http.StatusNotFound {
		t.Fatalf("expected 404 for foreign/missing batch, got %d body=%s", response.Code, response.Body.String())
	}
}

func TestAppDailyQuizProgressPassesAuthenticatedUserAndCard(t *testing.T) {
	service := &fakeDailyQuizService{progress: map[string]any{"answered": 42, "total": 100, "todayAnswered": 3, "todayTotal": 5, "latestReportId": 66}}
	s := &Server{appDailyQuiz: service}

	response := performAppCalibrationRequest(t, s.appDailyQuizProgress, http.MethodGet, "/api/app/daily-quiz/progress?cardId=123", nil)

	if response.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d body=%s", response.Code, response.Body.String())
	}
	if service.progressUserID != 7 || service.progressCardID != 123 {
		t.Fatalf("expected service to receive user=7 card=123, got user=%d card=%d", service.progressUserID, service.progressCardID)
	}
}

type fakeDailyQuizService struct {
	today       any
	todayErr    error
	todayUserID int64
	todayCardID int64

	progress       any
	progressErr    error
	progressUserID int64
	progressCardID int64

	answer           any
	answerErr        error
	answerUserID     int64
	answerBatchID    int64
	answerQuestionID int64
	answerOptionID   string

	complete        any
	completeErr     error
	completeUserID  int64
	completeBatchID int64
}

func (f *fakeDailyQuizService) TodayBatch(_ context.Context, appUserID, cardID int64) (any, error) {
	f.todayUserID = appUserID
	f.todayCardID = cardID
	return f.today, f.todayErr
}

func (f *fakeDailyQuizService) Progress(_ context.Context, appUserID, cardID int64) (any, error) {
	f.progressUserID = appUserID
	f.progressCardID = cardID
	return f.progress, f.progressErr
}

func (f *fakeDailyQuizService) SubmitAnswer(_ context.Context, appUserID, batchID, questionID int64, optionID string) (any, error) {
	f.answerUserID = appUserID
	f.answerBatchID = batchID
	f.answerQuestionID = questionID
	f.answerOptionID = optionID
	return f.answer, f.answerErr
}

func (f *fakeDailyQuizService) CompleteBatch(_ context.Context, appUserID, batchID int64) (any, error) {
	f.completeUserID = appUserID
	f.completeBatchID = batchID
	return f.complete, f.completeErr
}

func performAppCalibrationRequest(t *testing.T, handler http.HandlerFunc, method, path string, payload any) *httptest.ResponseRecorder {
	t.Helper()
	var body bytes.Buffer
	if payload != nil {
		if err := json.NewEncoder(&body).Encode(payload); err != nil {
			t.Fatal(err)
		}
	}
	request := httptest.NewRequest(method, path, &body)
	request = request.WithContext(contextWithAppUser(request.Context(), auth.UserInfo{ID: 7, Phone: "13800000000"}))
	response := httptest.NewRecorder()
	handler(response, request)
	return response
}

func decodeAppCalibrationResponse(t *testing.T, response *httptest.ResponseRecorder) struct {
	Code    int    `json:"code"`
	Data    any    `json:"data"`
	Message string `json:"message"`
} {
	t.Helper()
	var body struct {
		Code    int    `json:"code"`
		Data    any    `json:"data"`
		Message string `json:"message"`
	}
	if err := json.Unmarshal(response.Body.Bytes(), &body); err != nil {
		t.Fatal(err)
	}
	return body
}
