package server

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"strconv"
	"strings"
	"time"

	"nine-xing/nx-backend/apps/server/internal/httpx"
	"nine-xing/nx-backend/apps/server/internal/profilecalibration"
)

type appDailyQuizBankAdminService interface {
	GetDailyQuizSet(ctx context.Context, date string) (profilecalibration.DailyQuizSet, error)
	GenerateDailyQuizSet(ctx context.Context, date string) (profilecalibration.DailyQuizSet, error)
	ReplaceDailyQuizQuestion(ctx context.Context, setID int64, slotNo int, reason, operator string) (profilecalibration.DailyQuizSet, error)
}

func (s *Server) adminDailyQuizSetToday(w http.ResponseWriter, r *http.Request) {
	s.adminDailyQuizSetByDate(w, r, appCalibrationBusinessDate(time.Now()))
}

func (s *Server) adminDailyQuizSet(w http.ResponseWriter, r *http.Request) {
	date, ok := adminDailyQuizDateParam(w, r)
	if !ok {
		return
	}
	s.adminDailyQuizSetByDate(w, r, date)
}

func (s *Server) adminDailyQuizSetByDate(w http.ResponseWriter, r *http.Request, date string) {
	if s == nil || s.appDailyQuizBankAdmin == nil {
		httpx.Fail(w, http.StatusInternalServerError, "每日题库服务不可用")
		return
	}
	set, err := s.appDailyQuizBankAdmin.GetDailyQuizSet(r.Context(), date)
	if err != nil {
		if errors.Is(err, profilecalibration.ErrInvalidInput) {
			httpx.Fail(w, http.StatusBadRequest, "日期格式错误")
			return
		}
		if errors.Is(err, profilecalibration.ErrNotFound) {
			httpx.Fail(w, http.StatusNotFound, "当日题目尚未生成")
			return
		}
		httpx.Fail(w, http.StatusInternalServerError, "查询失败")
		return
	}
	httpx.OK(w, set)
}

func (s *Server) adminDailyQuizSetGenerate(w http.ResponseWriter, r *http.Request) {
	var input struct {
		Date string `json:"date"`
	}
	if r.Body != nil {
		if err := json.NewDecoder(r.Body).Decode(&input); err != nil {
			httpx.Fail(w, http.StatusBadRequest, "Invalid JSON payload")
			return
		}
	}
	date := strings.TrimSpace(input.Date)
	if date == "" {
		date = appCalibrationBusinessDate(time.Now())
	}
	if _, err := time.Parse("2006-01-02", date); err != nil {
		httpx.Fail(w, http.StatusBadRequest, "日期格式错误")
		return
	}
	if s == nil || s.appDailyQuizBankAdmin == nil {
		httpx.Fail(w, http.StatusInternalServerError, "每日题库服务不可用")
		return
	}
	set, err := s.appDailyQuizBankAdmin.GenerateDailyQuizSet(r.Context(), date)
	if err != nil {
		if errors.Is(err, profilecalibration.ErrInvalidInput) {
			httpx.Fail(w, http.StatusBadRequest, "日期格式错误")
			return
		}
		httpx.Fail(w, http.StatusInternalServerError, "生成失败")
		return
	}
	httpx.OK(w, set)
}

func (s *Server) adminDailyQuizSetRouter(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		httpx.Fail(w, http.StatusMethodNotAllowed, "Method Not Allowed")
		return
	}
	path := strings.TrimPrefix(r.URL.Path, "/api/daily-quiz/admin/sets/")
	parts := strings.Split(strings.Trim(path, "/"), "/")
	if len(parts) != 4 || parts[1] != "questions" || parts[2] == "" || parts[3] != "replace" {
		httpx.Fail(w, http.StatusNotFound, "Not Found")
		return
	}
	setID, err := strconv.ParseInt(parts[0], 10, 64)
	if err != nil || setID <= 0 {
		httpx.Fail(w, http.StatusBadRequest, "invalid set id")
		return
	}
	slotNo, err := strconv.Atoi(parts[2])
	if err != nil || slotNo <= 0 {
		httpx.Fail(w, http.StatusBadRequest, "invalid slot no")
		return
	}
	s.adminDailyQuizQuestionReplaceWithIDs(w, r, setID, slotNo)
}

func (s *Server) adminDailyQuizQuestionReplace(w http.ResponseWriter, r *http.Request) {
	path := strings.TrimPrefix(r.URL.Path, "/api/daily-quiz/admin/sets/")
	parts := strings.Split(strings.Trim(path, "/"), "/")
	if len(parts) != 4 {
		httpx.Fail(w, http.StatusNotFound, "Not Found")
		return
	}
	setID, _ := strconv.ParseInt(parts[0], 10, 64)
	slotNo, _ := strconv.Atoi(parts[2])
	s.adminDailyQuizQuestionReplaceWithIDs(w, r, setID, slotNo)
}

func (s *Server) adminDailyQuizQuestionReplaceWithIDs(w http.ResponseWriter, r *http.Request, setID int64, slotNo int) {
	var input struct {
		Reason string `json:"reason"`
	}
	if r.Body != nil {
		if err := json.NewDecoder(r.Body).Decode(&input); err != nil {
			httpx.Fail(w, http.StatusBadRequest, "Invalid JSON payload")
			return
		}
	}
	if s == nil || s.appDailyQuizBankAdmin == nil {
		httpx.Fail(w, http.StatusInternalServerError, "每日题库服务不可用")
		return
	}
	user := userFromRequest(r)
	operator := firstNonEmpty(strings.TrimSpace(user.RealName), strings.TrimSpace(user.Username), strings.TrimSpace(user.UserID))
	set, err := s.appDailyQuizBankAdmin.ReplaceDailyQuizQuestion(r.Context(), setID, slotNo, strings.TrimSpace(input.Reason), operator)
	if err != nil {
		if errors.Is(err, profilecalibration.ErrInvalidInput) {
			httpx.Fail(w, http.StatusBadRequest, "参数错误")
			return
		}
		if errors.Is(err, profilecalibration.ErrInvalidStatus) {
			httpx.Fail(w, http.StatusConflict, "该题已有用户答题，不能更换")
			return
		}
		if errors.Is(err, profilecalibration.ErrNotFound) {
			httpx.Fail(w, http.StatusNotFound, "题集不存在")
			return
		}
		httpx.Fail(w, http.StatusInternalServerError, "更换失败")
		return
	}
	httpx.OK(w, set)
}

func adminDailyQuizDateParam(w http.ResponseWriter, r *http.Request) (string, bool) {
	date := strings.TrimSpace(r.URL.Query().Get("date"))
	if date == "" {
		return appCalibrationBusinessDate(time.Now()), true
	}
	parsed, err := time.Parse("2006-01-02", date)
	if err != nil || parsed.Format("2006-01-02") != date {
		httpx.Fail(w, http.StatusBadRequest, "日期格式错误")
		return "", false
	}
	return date, true
}
