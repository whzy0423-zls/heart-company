package server

import (
	"database/sql"
	"encoding/json"
	"errors"
	"net/http"
	"strconv"
	"strings"

	"nine-xing/nx-backend/apps/server/internal/httpx"
	"nine-xing/nx-backend/apps/server/internal/quiz"
)

func (s *Server) appQuizQuestions(w http.ResponseWriter, r *http.Request) {
	qs, err := s.quiz.ListQuestions(r.Context())
	if err != nil {
		httpx.Fail(w, http.StatusInternalServerError, "failed to load questions")
		return
	}
	httpx.OK(w, qs)
}

func (s *Server) appQuizSubmit(w http.ResponseWriter, r *http.Request) {
	userInfo, ok := appUserFromContext(r)
	if !ok {
		httpx.Fail(w, http.StatusUnauthorized, "unauthorized")
		return
	}
	var input quiz.SubmitInput
	if err := json.NewDecoder(r.Body).Decode(&input); err != nil {
		httpx.Fail(w, http.StatusBadRequest, "invalid request body")
		return
	}
	if len(input.Answers) == 0 {
		httpx.Fail(w, http.StatusBadRequest, "answers required")
		return
	}
	sub, err := s.quiz.Submit(r.Context(), userInfo.ID, input)
	if err != nil {
		if strings.HasPrefix(err.Error(), "quiz: ") {
			httpx.Fail(w, http.StatusBadRequest, err.Error())
			return
		}
		httpx.Fail(w, http.StatusInternalServerError, "submit failed")
		return
	}
	httpx.OK(w, sub)
}

func (s *Server) appQuizSubmission(w http.ResponseWriter, r *http.Request) {
	userInfo, ok := appUserFromContext(r)
	if !ok {
		httpx.Fail(w, http.StatusUnauthorized, "unauthorized")
		return
	}
	sub, err := s.quiz.LatestSubmission(r.Context(), userInfo.ID)
	if errors.Is(err, sql.ErrNoRows) {
		httpx.OK(w, nil)
		return
	}
	if err != nil {
		httpx.Fail(w, http.StatusInternalServerError, "query failed")
		return
	}
	httpx.OK(w, sub)
}

func (s *Server) appCards(w http.ResponseWriter, r *http.Request) {
	userInfo, ok := appUserFromContext(r)
	if !ok {
		httpx.Fail(w, http.StatusUnauthorized, "unauthorized")
		return
	}
	switch r.Method {
	case http.MethodGet:
		cards, err := s.quiz.ListCards(r.Context(), userInfo.ID)
		if err != nil {
			httpx.Fail(w, http.StatusInternalServerError, "query failed")
			return
		}
		httpx.OK(w, cards)
	case http.MethodPost:
		var input quiz.CardInput
		if err := json.NewDecoder(r.Body).Decode(&input); err != nil {
			httpx.Fail(w, http.StatusBadRequest, "invalid request body")
			return
		}
		user, err := s.appUsers.FindByID(r.Context(), userInfo.ID)
		if err != nil {
			httpx.Fail(w, http.StatusNotFound, "user not found")
			return
		}
		created, err := s.quiz.CreateCard(r.Context(), userInfo.ID, user.MemberLevel, input)
		if err != nil {
			httpx.Fail(w, http.StatusInternalServerError, "create failed")
			return
		}
		httpx.OK(w, created)
	default:
		httpx.Fail(w, http.StatusMethodNotAllowed, "method not allowed")
	}
}

func (s *Server) appCardPrimary(w http.ResponseWriter, r *http.Request) {
	userInfo, ok := appUserFromContext(r)
	if !ok {
		httpx.Fail(w, http.StatusUnauthorized, "unauthorized")
		return
	}
	card, err := s.quiz.PrimaryCard(r.Context(), userInfo.ID)
	if errors.Is(err, sql.ErrNoRows) {
		httpx.OK(w, nil)
		return
	}
	if err != nil {
		httpx.Fail(w, http.StatusInternalServerError, "query failed")
		return
	}
	httpx.OK(w, card)
}

// appCardByID handles GET/PUT/DELETE on a single card owned by the current user.
// Exact route "/api/app/cards/primary" still wins over this subtree match.
func (s *Server) appCardByID(w http.ResponseWriter, r *http.Request) {
	userInfo, ok := appUserFromContext(r)
	if !ok {
		httpx.Fail(w, http.StatusUnauthorized, "unauthorized")
		return
	}
	idText := strings.Trim(strings.TrimPrefix(r.URL.Path, "/api/app/cards/"), "/")
	// 子路径分发：/api/app/cards/:id/portrait → 成长画像。
	if rest, ok := strings.CutSuffix(idText, "/portrait"); ok {
		s.appCardPortrait(w, r, userInfo.ID, strings.Trim(rest, "/"))
		return
	}
	if rest, ok := strings.CutSuffix(idText, "/memories"); ok {
		s.appCardMemories(w, r, userInfo.ID, strings.Trim(rest, "/"))
		return
	}
	id, err := strconv.ParseInt(idText, 10, 64)
	if err != nil || id <= 0 {
		httpx.Fail(w, http.StatusBadRequest, "invalid id")
		return
	}
	switch r.Method {
	case http.MethodGet:
		card, err := s.quiz.GetCard(r.Context(), userInfo.ID, id)
		if errors.Is(err, quiz.ErrNotFound) {
			httpx.Fail(w, http.StatusNotFound, "not found")
			return
		}
		if err != nil {
			httpx.Fail(w, http.StatusInternalServerError, "query failed")
			return
		}
		httpx.OK(w, card)
	case http.MethodPut:
		var input quiz.CardInput
		if err := json.NewDecoder(r.Body).Decode(&input); err != nil {
			httpx.Fail(w, http.StatusBadRequest, "invalid request body")
			return
		}
		card, err := s.quiz.UpdateCard(r.Context(), userInfo.ID, id, input)
		if errors.Is(err, quiz.ErrNotFound) {
			httpx.Fail(w, http.StatusNotFound, "not found")
			return
		}
		if err != nil {
			httpx.Fail(w, http.StatusInternalServerError, "update failed")
			return
		}
		httpx.OK(w, card)
	case http.MethodDelete:
		if err := s.quiz.DeleteCard(r.Context(), userInfo.ID, id); err != nil {
			if errors.Is(err, quiz.ErrNotFound) {
				httpx.Fail(w, http.StatusNotFound, "not found")
				return
			}
			httpx.Fail(w, http.StatusInternalServerError, "delete failed")
			return
		}
		httpx.OK(w, true)
	default:
		httpx.Fail(w, http.StatusMethodNotAllowed, "method not allowed")
	}
}
