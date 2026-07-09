package server

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"testing"

	"nine-xing/nx-backend/apps/server/internal/profilecalibration"
)

func TestAdminDailyQuizPushStatsReturnsPushAndAnswerCounts(t *testing.T) {
	service := &fakeDailyQuizPushAdminService{
		stats: profilecalibration.DailyQuizPushStats{
			Date:                       "2026-07-09",
			EligibleUsers:              12,
			PushedUsers:                9,
			AnsweredUsers:              4,
			CompletedUsers:             2,
			TotalAnswers:               17,
			PendingReassessmentReports: 3,
		},
	}
	s := &Server{appDailyQuizPushAdmin: service}
	req := httptest.NewRequest(http.MethodGet, "/api/push/daily-quiz/stats?date=2026-07-09", nil)
	res := httptest.NewRecorder()

	s.adminDailyQuizPushStats(res, req)

	if res.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d body=%s", res.Code, res.Body.String())
	}
	if service.statsDate != "2026-07-09" {
		t.Fatalf("expected service date 2026-07-09, got %q", service.statsDate)
	}
	var body struct {
		Data profilecalibration.DailyQuizPushStats `json:"data"`
	}
	if err := json.Unmarshal(res.Body.Bytes(), &body); err != nil {
		t.Fatal(err)
	}
	if body.Data.PushedUsers != 9 || body.Data.AnsweredUsers != 4 || !body.Data.Pushed {
		t.Fatalf("unexpected stats payload: %+v", body.Data)
	}
}

func TestAdminDailyQuizPushRecordsReturnsPagedBatchRows(t *testing.T) {
	service := &fakeDailyQuizPushAdminService{
		records: []profilecalibration.DailyQuizPushRecord{{
			AppUserID:     7,
			Phone:         "13800000000",
			Nickname:      "测试用户",
			CardID:        123,
			CardName:      "本人人格卡",
			QuizDate:      "2026-07-09",
			BatchID:       88,
			Pushed:        true,
			PushSentAt:    "2026/07/09 09:00:00",
			AnsweredCount: 5,
			Completed:     true,
			CompletedAt:   "2026/07/09 09:05:00",
		}},
		recordsTotal: 1,
	}
	s := &Server{appDailyQuizPushAdmin: service}
	req := httptest.NewRequest(http.MethodGet, "/api/push/daily-quiz/records?date=2026-07-09&page=2&pageSize=10", nil)
	res := httptest.NewRecorder()

	s.adminDailyQuizPushRecords(res, req)

	if res.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d body=%s", res.Code, res.Body.String())
	}
	if service.recordsDate != "2026-07-09" || service.recordsPage != 2 || service.recordsPageSize != 10 {
		t.Fatalf("unexpected service input date=%q page=%d pageSize=%d", service.recordsDate, service.recordsPage, service.recordsPageSize)
	}
	var body struct {
		Data struct {
			Items []profilecalibration.DailyQuizPushRecord `json:"items"`
			Total int                                      `json:"total"`
		} `json:"data"`
	}
	if err := json.Unmarshal(res.Body.Bytes(), &body); err != nil {
		t.Fatal(err)
	}
	if body.Data.Total != 1 || len(body.Data.Items) != 1 || body.Data.Items[0].BatchID != 88 {
		t.Fatalf("unexpected records payload: %+v", body.Data)
	}
}

func TestAdminDailyQuizPushRejectsInvalidDate(t *testing.T) {
	s := &Server{appDailyQuizPushAdmin: &fakeDailyQuizPushAdminService{}}
	req := httptest.NewRequest(http.MethodGet, "/api/push/daily-quiz/stats?date=2026/07/09", nil)
	res := httptest.NewRecorder()

	s.adminDailyQuizPushStats(res, req)

	if res.Code != http.StatusBadRequest {
		t.Fatalf("expected invalid date to return 400, got %d body=%s", res.Code, res.Body.String())
	}
}

func TestAdminDailyQuizPushReturnsServiceUnavailableWhenStoreMissing(t *testing.T) {
	s := &Server{}
	req := httptest.NewRequest(http.MethodGet, "/api/push/daily-quiz/records?date=2026-07-09", nil)
	res := httptest.NewRecorder()

	s.adminDailyQuizPushRecords(res, req)

	if res.Code != http.StatusInternalServerError {
		t.Fatalf("expected missing service to return 500, got %d body=%s", res.Code, res.Body.String())
	}
}

func TestAdminDailyQuizPushRouteRegistration(t *testing.T) {
	raw, err := os.ReadFile("server.go")
	if err != nil {
		t.Fatalf("read server.go: %v", err)
	}
	source := string(raw)
	for _, route := range []string{
		`"/api/push/daily-quiz/stats"`,
		`"/api/push/daily-quiz/records"`,
	} {
		if !strings.Contains(source, route) {
			t.Fatalf("expected server routes to include %s", route)
		}
	}
	if !strings.Contains(source, `requirePermission("ProfileCalibration:DailyQuiz:Manage", s.adminDailyQuizPushStats)`) {
		t.Fatal("daily quiz push stats route must require ProfileCalibration:DailyQuiz:Manage")
	}
	if !strings.Contains(source, `requirePermission("ProfileCalibration:DailyQuiz:Manage", s.adminDailyQuizPushRecords)`) {
		t.Fatal("daily quiz push records route must require ProfileCalibration:DailyQuiz:Manage")
	}
}

type fakeDailyQuizPushAdminService struct {
	stats     profilecalibration.DailyQuizPushStats
	statsErr  error
	statsDate string

	records         []profilecalibration.DailyQuizPushRecord
	recordsTotal    int
	recordsErr      error
	recordsDate     string
	recordsPage     int
	recordsPageSize int
}

func (f *fakeDailyQuizPushAdminService) DailyQuizPushStats(_ context.Context, date string) (profilecalibration.DailyQuizPushStats, error) {
	f.statsDate = date
	if f.statsErr != nil {
		return profilecalibration.DailyQuizPushStats{}, f.statsErr
	}
	return f.stats, nil
}

func (f *fakeDailyQuizPushAdminService) ListDailyQuizPushRecords(_ context.Context, date string, page, pageSize int) ([]profilecalibration.DailyQuizPushRecord, int, error) {
	f.recordsDate = date
	f.recordsPage = page
	f.recordsPageSize = pageSize
	if f.recordsErr != nil {
		return nil, 0, f.recordsErr
	}
	return f.records, f.recordsTotal, nil
}

func TestAdminDailyQuizPushStatsMapsServiceErrors(t *testing.T) {
	s := &Server{appDailyQuizPushAdmin: &fakeDailyQuizPushAdminService{statsErr: errors.New("db down")}}
	req := httptest.NewRequest(http.MethodGet, "/api/push/daily-quiz/stats?date=2026-07-09", nil)
	res := httptest.NewRecorder()

	s.adminDailyQuizPushStats(res, req)

	if res.Code != http.StatusInternalServerError {
		t.Fatalf("expected service error to return 500, got %d body=%s", res.Code, res.Body.String())
	}
}
