package server

import (
	"encoding/json"
	"net/http"
	"strconv"
	"strings"

	"nine-xing/nx-backend/apps/server/internal/httpx"
	"nine-xing/nx-backend/apps/server/internal/quiz"
)

// adminQuizQuestions handles list (GET) and create (POST) of quiz questions for
// the admin question-bank manager.
func (s *Server) adminQuizQuestions(w http.ResponseWriter, r *http.Request) {
	switch r.Method {
	case http.MethodGet:
		result, err := s.quiz.ListQuestionsAdmin(r.Context())
		if err != nil {
			httpx.Fail(w, http.StatusInternalServerError, err.Error())
			return
		}
		httpx.OK(w, result)
	case http.MethodPost:
		r.Body = http.MaxBytesReader(w, r.Body, 256*1024)
		var body quiz.QuestionInput
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
			httpx.Fail(w, http.StatusBadRequest, "Invalid JSON payload")
			return
		}
		result, err := s.quiz.CreateQuestion(r.Context(), body)
		if err != nil {
			httpx.Fail(w, http.StatusBadRequest, err.Error())
			return
		}
		httpx.OK(w, result)
	default:
		httpx.Fail(w, http.StatusMethodNotAllowed, "Method Not Allowed")
	}
}

// adminQuizQuestionByID handles update (PUT) and delete (DELETE) of a single
// quiz question identified by its numeric id.
func (s *Server) adminQuizQuestionByID(w http.ResponseWriter, r *http.Request) {
	idText := strings.Trim(strings.TrimPrefix(r.URL.Path, "/api/quiz/questions/"), "/")
	id, err := strconv.ParseInt(idText, 10, 64)
	if err != nil || id <= 0 {
		httpx.Fail(w, http.StatusBadRequest, "invalid id")
		return
	}
	switch r.Method {
	case http.MethodPut:
		r.Body = http.MaxBytesReader(w, r.Body, 256*1024)
		var body quiz.QuestionInput
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
			httpx.Fail(w, http.StatusBadRequest, "Invalid JSON payload")
			return
		}
		result, err := s.quiz.UpdateQuestion(r.Context(), id, body)
		if err != nil {
			httpx.Fail(w, http.StatusBadRequest, err.Error())
			return
		}
		httpx.OK(w, result)
	case http.MethodDelete:
		if err := s.quiz.DeleteQuestion(r.Context(), id); err != nil {
			httpx.Fail(w, http.StatusBadRequest, err.Error())
			return
		}
		httpx.OK(w, true)
	default:
		httpx.Fail(w, http.StatusMethodNotAllowed, "Method Not Allowed")
	}
}

// adminQuizCards lists the命运卡片 belonging to one app user (admin read-only
// view). The target user is given via the ?appUserId= query parameter.
func (s *Server) adminQuizCards(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		httpx.Fail(w, http.StatusMethodNotAllowed, "Method Not Allowed")
		return
	}
	appUserID, err := strconv.ParseInt(strings.TrimSpace(r.URL.Query().Get("appUserId")), 10, 64)
	if err != nil || appUserID <= 0 {
		httpx.Fail(w, http.StatusBadRequest, "appUserId is required")
		return
	}
	result, err := s.quiz.ListCardsAdmin(r.Context(), appUserID)
	if err != nil {
		httpx.Fail(w, http.StatusInternalServerError, err.Error())
		return
	}
	httpx.OK(w, result)
}
