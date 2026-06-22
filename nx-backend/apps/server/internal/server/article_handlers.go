package server

import (
	"encoding/json"
	"net/http"
	"strings"

	"nine-xing/nx-backend/apps/server/internal/articlestore"
	"nine-xing/nx-backend/apps/server/internal/httpx"
)

// adminArticles handles list (GET) and create (POST) for the reading admin.
func (s *Server) adminArticles(w http.ResponseWriter, r *http.Request) {
	switch r.Method {
	case http.MethodGet:
		result, err := s.articles.ListArticles(r.Context(), queryMap(r))
		if err != nil {
			httpx.Fail(w, http.StatusInternalServerError, err.Error())
			return
		}
		httpx.OK(w, result)
	case http.MethodPost:
		r.Body = http.MaxBytesReader(w, r.Body, 512*1024)
		var body articlestore.Article
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
			httpx.Fail(w, http.StatusBadRequest, "Invalid JSON payload")
			return
		}
		result, err := s.articles.SaveArticle(r.Context(), body)
		if err != nil {
			httpx.Fail(w, http.StatusBadRequest, err.Error())
			return
		}
		httpx.OK(w, result)
	default:
		httpx.Fail(w, http.StatusMethodNotAllowed, "Method Not Allowed")
	}
}

// adminArticleByID handles detail (GET), update (PUT), delete (DELETE) and
// listen-to-article audio generation (POST .../{id}/audio).
func (s *Server) adminArticleByID(w http.ResponseWriter, r *http.Request) {
	path := strings.Trim(strings.TrimPrefix(r.URL.Path, "/api/articles/"), "/")
	if path == "" {
		httpx.Fail(w, http.StatusBadRequest, "id is required")
		return
	}

	// 子路由：POST /api/articles/{id}/audio 触发听书音频生成。
	if id, ok := strings.CutSuffix(path, "/audio"); ok {
		if r.Method != http.MethodPost {
			httpx.Fail(w, http.StatusMethodNotAllowed, "Method Not Allowed")
			return
		}
		doc, err := s.articles.GenerateAudio(r.Context(), id)
		if err != nil {
			httpx.Fail(w, http.StatusBadRequest, err.Error())
			return
		}
		httpx.OK(w, doc)
		return
	}

	id := path
	switch r.Method {
	case http.MethodGet:
		doc, ok, err := s.articles.GetArticle(r.Context(), id)
		if err != nil {
			httpx.Fail(w, http.StatusBadRequest, err.Error())
			return
		}
		if !ok {
			httpx.Fail(w, http.StatusNotFound, "文章不存在")
			return
		}
		httpx.OK(w, doc)
	case http.MethodPut:
		r.Body = http.MaxBytesReader(w, r.Body, 512*1024)
		var body articlestore.Article
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
			httpx.Fail(w, http.StatusBadRequest, "Invalid JSON payload")
			return
		}
		body.ID = id
		result, err := s.articles.SaveArticle(r.Context(), body)
		if err != nil {
			httpx.Fail(w, http.StatusBadRequest, err.Error())
			return
		}
		httpx.OK(w, result)
	case http.MethodDelete:
		ok, err := s.articles.DeleteArticle(r.Context(), id)
		if err != nil {
			httpx.Fail(w, http.StatusBadRequest, err.Error())
			return
		}
		httpx.OK(w, ok)
	default:
		httpx.Fail(w, http.StatusMethodNotAllowed, "Method Not Allowed")
	}
}

// publicArticles serves the published article list to the H5 (no auth).
func (s *Server) publicArticles(w http.ResponseWriter, r *http.Request) {
	result, err := s.articles.PublicList(r.Context(), queryMap(r))
	if err != nil {
		httpx.Fail(w, http.StatusInternalServerError, err.Error())
		return
	}
	httpx.OK(w, result)
}

// publicArticleDetail serves one published article with full Markdown content.
func (s *Server) publicArticleDetail(w http.ResponseWriter, r *http.Request) {
	id := strings.Trim(strings.TrimPrefix(r.URL.Path, "/api/public/articles/"), "/")
	if id == "" {
		httpx.Fail(w, http.StatusBadRequest, "id is required")
		return
	}
	doc, ok, err := s.articles.PublicDetail(r.Context(), id)
	if err != nil {
		httpx.Fail(w, http.StatusBadRequest, err.Error())
		return
	}
	if !ok {
		httpx.Fail(w, http.StatusNotFound, "文章不存在或已下架")
		return
	}
	httpx.OK(w, doc)
}

// publicArticleCategories lists distinct categories for the H5 filter bar.
func (s *Server) publicArticleCategories(w http.ResponseWriter, r *http.Request) {
	categories, err := s.articles.Categories(r.Context())
	if err != nil {
		httpx.Fail(w, http.StatusInternalServerError, err.Error())
		return
	}
	httpx.OK(w, categories)
}

// readingSettings reads (GET) and updates (PUT) the global 听书 default voice.
func (s *Server) readingSettings(w http.ResponseWriter, r *http.Request) {
	switch r.Method {
	case http.MethodGet:
		voiceKey, err := s.articles.DefaultVoice(r.Context())
		if err != nil {
			httpx.Fail(w, http.StatusInternalServerError, err.Error())
			return
		}
		httpx.OK(w, map[string]string{"voiceKey": voiceKey})
	case http.MethodPut:
		r.Body = http.MaxBytesReader(w, r.Body, 16*1024)
		var body struct {
			VoiceKey string `json:"voiceKey"`
		}
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
			httpx.Fail(w, http.StatusBadRequest, "Invalid JSON payload")
			return
		}
		if err := s.articles.SetDefaultVoice(r.Context(), body.VoiceKey); err != nil {
			httpx.Fail(w, http.StatusBadRequest, err.Error())
			return
		}
		httpx.OK(w, map[string]string{"voiceKey": strings.TrimSpace(body.VoiceKey)})
	default:
		httpx.Fail(w, http.StatusMethodNotAllowed, "Method Not Allowed")
	}
}
