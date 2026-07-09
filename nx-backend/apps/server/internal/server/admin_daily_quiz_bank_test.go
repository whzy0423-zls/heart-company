package server

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"testing"

	"nine-xing/nx-backend/apps/server/internal/profilecalibration"
)

func TestAdminDailyQuizBankTodayReturnsGeneratedSet(t *testing.T) {
	service := &fakeDailyQuizBankAdminService{
		set: profilecalibration.DailyQuizSet{
			ID:          10,
			Date:        "2026-07-09",
			Status:      "generated",
			Source:      "ai",
			QuestionIDs: []int64{1, 2, 3, 4, 5},
			Questions: []profilecalibration.DailyQuizQuestionVersion{{
				SlotNo:    1,
				VersionNo: 1,
				Question:  profilecalibration.Question{ID: 1, Body: "今天更接近哪种反应？"},
			}},
		},
	}
	s := &Server{appDailyQuizBankAdmin: service}
	req := httptest.NewRequest(http.MethodGet, "/api/daily-quiz/admin/sets/today", nil)
	res := httptest.NewRecorder()

	s.adminDailyQuizSetToday(res, req)

	if res.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d body=%s", res.Code, res.Body.String())
	}
	if service.getDate == "" {
		t.Fatal("expected handler to query today's business date")
	}
	var body struct {
		Data profilecalibration.DailyQuizSet `json:"data"`
	}
	if err := json.Unmarshal(res.Body.Bytes(), &body); err != nil {
		t.Fatal(err)
	}
	if body.Data.ID != 10 || len(body.Data.Questions) != 1 {
		t.Fatalf("unexpected daily quiz set payload: %+v", body.Data)
	}
}

func TestAdminDailyQuizBankGenerateUsesRequestedDate(t *testing.T) {
	service := &fakeDailyQuizBankAdminService{set: profilecalibration.DailyQuizSet{ID: 11, Date: "2026-07-09"}}
	s := &Server{appDailyQuizBankAdmin: service}
	req := httptest.NewRequest(http.MethodPost, "/api/daily-quiz/admin/sets/generate", strings.NewReader(`{"date":"2026-07-09"}`))
	res := httptest.NewRecorder()

	s.adminDailyQuizSetGenerate(res, req)

	if res.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d body=%s", res.Code, res.Body.String())
	}
	if service.generateDate != "2026-07-09" {
		t.Fatalf("expected generate date 2026-07-09, got %q", service.generateDate)
	}
}

func TestAdminDailyQuizBankReplaceSingleQuestionVersion(t *testing.T) {
	service := &fakeDailyQuizBankAdminService{set: profilecalibration.DailyQuizSet{ID: 10, Date: "2026-07-09"}}
	s := &Server{appDailyQuizBankAdmin: service}
	req := httptest.NewRequest(http.MethodPost, "/api/daily-quiz/admin/sets/10/questions/3/replace", strings.NewReader(`{"reason":"题目表达不够精准"}`))
	res := httptest.NewRecorder()

	s.adminDailyQuizQuestionReplace(res, req)

	if res.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d body=%s", res.Code, res.Body.String())
	}
	if service.replaceSetID != 10 || service.replaceSlotNo != 3 {
		t.Fatalf("expected replace set=10 slot=3, got set=%d slot=%d", service.replaceSetID, service.replaceSlotNo)
	}
	if service.replaceReason != "题目表达不够精准" {
		t.Fatalf("unexpected replace reason %q", service.replaceReason)
	}
}

func TestAdminDailyQuizBankRoutesAreRegistered(t *testing.T) {
	raw, err := os.ReadFile("server.go")
	if err != nil {
		t.Fatalf("read server.go: %v", err)
	}
	source := string(raw)
	for _, route := range []string{
		`"/api/daily-quiz/admin/sets/today"`,
		`"/api/daily-quiz/admin/sets"`,
		`"/api/daily-quiz/admin/sets/generate"`,
		`"/api/daily-quiz/admin/sets/"`,
	} {
		if !strings.Contains(source, route) {
			t.Fatalf("expected server routes to include %s", route)
		}
	}
	if !strings.Contains(source, `requirePermission("ProfileCalibration:DailyQuiz:Manage"`) {
		t.Fatal("daily quiz bank routes must require ProfileCalibration:DailyQuiz:Manage")
	}
}

type fakeDailyQuizBankAdminService struct {
	set profilecalibration.DailyQuizSet
	err error

	getDate      string
	generateDate string

	replaceSetID  int64
	replaceSlotNo int
	replaceReason string
}

func (f *fakeDailyQuizBankAdminService) GetDailyQuizSet(_ context.Context, date string) (profilecalibration.DailyQuizSet, error) {
	f.getDate = date
	if f.err != nil {
		return profilecalibration.DailyQuizSet{}, f.err
	}
	return f.set, nil
}

func (f *fakeDailyQuizBankAdminService) GenerateDailyQuizSet(_ context.Context, date string) (profilecalibration.DailyQuizSet, error) {
	f.generateDate = date
	if f.err != nil {
		return profilecalibration.DailyQuizSet{}, f.err
	}
	if f.set.Date == "" {
		f.set.Date = date
	}
	return f.set, nil
}

func (f *fakeDailyQuizBankAdminService) ReplaceDailyQuizQuestion(_ context.Context, setID int64, slotNo int, reason, operator string) (profilecalibration.DailyQuizSet, error) {
	f.replaceSetID = setID
	f.replaceSlotNo = slotNo
	f.replaceReason = reason
	if f.err != nil {
		return profilecalibration.DailyQuizSet{}, f.err
	}
	return f.set, nil
}
