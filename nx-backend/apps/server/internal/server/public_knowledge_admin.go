package server

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"strings"

	"nine-xing/nx-backend/apps/server/internal/httpx"
	"nine-xing/nx-backend/apps/server/internal/publicknowledge"
	"nine-xing/nx-backend/apps/server/internal/rag"
)

type publicKnowledgeStore interface {
	List(context.Context, publicknowledge.ListParams) (publicknowledge.ListResult, error)
	SetEnabled(context.Context, []string, bool) (int, error)
	Preview(context.Context, string) (publicknowledge.PreviewResult, error)
	Search(context.Context, string, int) ([]rag.Document, error)
}

func registerPublicKnowledgeAdminRoutes(mux *http.ServeMux, permission func(string, http.HandlerFunc) http.HandlerFunc, s *Server) {
	mux.HandleFunc("/api/rag/sources", permission("RAG:Knowledge:Manage", s.publicKnowledgeSources))
	mux.HandleFunc("/api/rag/sources/status", permission("RAG:Knowledge:Manage", s.publicKnowledgeSourceStatus))
	mux.HandleFunc("/api/rag/sources/", permission("RAG:Knowledge:Manage", s.publicKnowledgeSourceChunks))
}

func (s *Server) publicKnowledgeSources(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		httpx.Fail(w, http.StatusMethodNotAllowed, "Method Not Allowed")
		return
	}
	params, err := publicknowledge.ParseListParams(r.URL.Query())
	if err != nil {
		publicKnowledgeError(w, err)
		return
	}
	if s.publicKnowledge == nil {
		httpx.Fail(w, http.StatusServiceUnavailable, "Knowledge catalog unavailable")
		return
	}
	result, err := s.publicKnowledge.List(r.Context(), params)
	if err != nil {
		publicKnowledgeError(w, err)
		return
	}
	httpx.OK(w, result)
}

func (s *Server) publicKnowledgeSourceStatus(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		httpx.Fail(w, http.StatusMethodNotAllowed, "Method Not Allowed")
		return
	}
	r.Body = http.MaxBytesReader(w, r.Body, 64*1024)
	var body struct {
		IDs     []string `json:"ids"`
		Enabled *bool    `json:"enabled"`
	}
	decoder := json.NewDecoder(r.Body)
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(&body); err != nil {
		httpx.Fail(w, http.StatusBadRequest, "Invalid JSON payload")
		return
	}
	if err := decoder.Decode(new(any)); err != io.EOF {
		httpx.Fail(w, http.StatusBadRequest, "Invalid JSON payload")
		return
	}
	if body.Enabled == nil {
		httpx.Fail(w, http.StatusBadRequest, "enabled is required")
		return
	}
	if err := publicknowledge.ValidateStatusIDs(body.IDs); err != nil {
		publicKnowledgeError(w, err)
		return
	}
	if s.publicKnowledge == nil {
		httpx.Fail(w, http.StatusServiceUnavailable, "Knowledge catalog unavailable")
		return
	}
	updated, err := s.publicKnowledge.SetEnabled(r.Context(), body.IDs, *body.Enabled)
	if err != nil {
		publicKnowledgeError(w, err)
		return
	}
	httpx.OK(w, map[string]int{"updated": updated})
}

func (s *Server) publicKnowledgeSourceChunks(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		httpx.Fail(w, http.StatusMethodNotAllowed, "Method Not Allowed")
		return
	}
	path := strings.TrimPrefix(r.URL.Path, "/api/rag/sources/")
	parts := strings.Split(path, "/")
	if len(parts) != 2 || parts[0] == "" || parts[1] != "chunks" {
		httpx.Fail(w, http.StatusNotFound, "Not Found")
		return
	}
	if s.publicKnowledge == nil {
		httpx.Fail(w, http.StatusServiceUnavailable, "Knowledge catalog unavailable")
		return
	}
	result, err := s.publicKnowledge.Preview(r.Context(), parts[0])
	if err != nil {
		publicKnowledgeError(w, err)
		return
	}
	httpx.OK(w, result)
}

func publicKnowledgeError(w http.ResponseWriter, err error) {
	switch {
	case errors.Is(err, publicknowledge.ErrValidation):
		httpx.Fail(w, http.StatusBadRequest, err.Error())
	case errors.Is(err, publicknowledge.ErrNotFound):
		httpx.Fail(w, http.StatusNotFound, "Source not found")
	default:
		httpx.Fail(w, http.StatusInternalServerError, "Knowledge catalog operation failed")
	}
}
