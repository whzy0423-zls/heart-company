package server

import (
	"encoding/json"
	"net/http"
	"strings"

	"nine-xing/nx-backend/apps/server/internal/httpx"
	"nine-xing/nx-backend/apps/server/internal/mindquote"
)

// adminMindGroups handles list (GET) / upsert (POST) / delete (DELETE ?id=) for 心语分组。
func (s *Server) adminMindGroups(w http.ResponseWriter, r *http.Request) {
	switch r.Method {
	case http.MethodGet:
		groups, err := s.mindquotes.ListGroups(r.Context())
		if err != nil {
			httpx.Fail(w, http.StatusInternalServerError, err.Error())
			return
		}
		httpx.OK(w, map[string]any{"items": groups})
	case http.MethodPost:
		r.Body = http.MaxBytesReader(w, r.Body, 64*1024)
		var body mindquote.Group
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
			httpx.Fail(w, http.StatusBadRequest, "Invalid JSON payload")
			return
		}
		result, err := s.mindquotes.SaveGroup(r.Context(), body)
		if err != nil {
			httpx.Fail(w, http.StatusBadRequest, err.Error())
			return
		}
		httpx.OK(w, result)
	case http.MethodDelete:
		id := strings.TrimSpace(r.URL.Query().Get("id"))
		if id == "" {
			httpx.Fail(w, http.StatusBadRequest, "id is required")
			return
		}
		ok, err := s.mindquotes.DeleteGroup(r.Context(), id)
		if err != nil {
			httpx.Fail(w, http.StatusBadRequest, err.Error())
			return
		}
		httpx.OK(w, ok)
	default:
		httpx.Fail(w, http.StatusMethodNotAllowed, "Method Not Allowed")
	}
}

// adminMindQuotes handles list (GET) and create (POST) for 心语。
func (s *Server) adminMindQuotes(w http.ResponseWriter, r *http.Request) {
	switch r.Method {
	case http.MethodGet:
		result, err := s.mindquotes.ListQuotes(r.Context(), queryMap(r))
		if err != nil {
			httpx.Fail(w, http.StatusInternalServerError, err.Error())
			return
		}
		httpx.OK(w, result)
	case http.MethodPost:
		r.Body = http.MaxBytesReader(w, r.Body, 256*1024)
		var body mindquote.Quote
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
			httpx.Fail(w, http.StatusBadRequest, "Invalid JSON payload")
			return
		}
		result, err := s.mindquotes.SaveQuote(r.Context(), body)
		if err != nil {
			httpx.Fail(w, http.StatusBadRequest, err.Error())
			return
		}
		httpx.OK(w, result)
	default:
		httpx.Fail(w, http.StatusMethodNotAllowed, "Method Not Allowed")
	}
}

// adminMindQuoteByID handles detail (GET) / update (PUT) / delete (DELETE) by trailing id.
func (s *Server) adminMindQuoteByID(w http.ResponseWriter, r *http.Request) {
	id := strings.Trim(strings.TrimPrefix(r.URL.Path, "/api/mind-quotes/"), "/")
	if id == "" {
		httpx.Fail(w, http.StatusBadRequest, "id is required")
		return
	}
	switch r.Method {
	case http.MethodGet:
		q, ok, err := s.mindquotes.GetQuote(r.Context(), id)
		if err != nil {
			httpx.Fail(w, http.StatusBadRequest, err.Error())
			return
		}
		if !ok {
			httpx.Fail(w, http.StatusNotFound, "心语不存在")
			return
		}
		httpx.OK(w, q)
	case http.MethodPut:
		r.Body = http.MaxBytesReader(w, r.Body, 256*1024)
		var body mindquote.Quote
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
			httpx.Fail(w, http.StatusBadRequest, "Invalid JSON payload")
			return
		}
		body.ID = id
		result, err := s.mindquotes.SaveQuote(r.Context(), body)
		if err != nil {
			httpx.Fail(w, http.StatusBadRequest, err.Error())
			return
		}
		httpx.OK(w, result)
	case http.MethodDelete:
		ok, err := s.mindquotes.DeleteQuote(r.Context(), id)
		if err != nil {
			httpx.Fail(w, http.StatusBadRequest, err.Error())
			return
		}
		httpx.OK(w, ok)
	default:
		httpx.Fail(w, http.StatusMethodNotAllowed, "Method Not Allowed")
	}
}

// publicMindGroups serves enabled groups + their lightweight quotes to the website (no auth).
func (s *Server) publicMindGroups(w http.ResponseWriter, r *http.Request) {
	groups, err := s.mindquotes.PublicGroups(r.Context())
	if err != nil {
		httpx.Fail(w, http.StatusInternalServerError, err.Error())
		return
	}
	httpx.OK(w, map[string]any{"items": groups})
}

// publicMindQuoteDetail serves one enabled quote's full content (website detail page).
func (s *Server) publicMindQuoteDetail(w http.ResponseWriter, r *http.Request) {
	id := strings.Trim(strings.TrimPrefix(r.URL.Path, "/api/public/mind-quotes/"), "/")
	if id == "" {
		httpx.Fail(w, http.StatusBadRequest, "id is required")
		return
	}
	q, ok, err := s.mindquotes.PublicDetail(r.Context(), id)
	if err != nil {
		httpx.Fail(w, http.StatusBadRequest, err.Error())
		return
	}
	if !ok {
		httpx.Fail(w, http.StatusNotFound, "心语不存在或已下架")
		return
	}
	httpx.OK(w, q)
}
