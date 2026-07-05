package server

import (
	"encoding/json"
	"fmt"
	"net/http"
	"regexp"
	"strconv"
	"strings"
	"sync"
	"time"

	"nine-xing/nx-backend/apps/server/internal/articlestore"
	"nine-xing/nx-backend/apps/server/internal/httpx"
)

const (
	publicArticleReferenceCacheMaxEntries = 1024
	publicArticleReferenceCacheTTL        = 30 * time.Second
)

var publicArticleReferenceCache = newPublicArticleReferenceCacheStore()

type publicArticleReferenceCacheEntry struct {
	expiresAt time.Time
	value     bool
}

type publicArticleReferenceCacheStore struct {
	mu    sync.Mutex
	items map[string]publicArticleReferenceCacheEntry
}

func newPublicArticleReferenceCacheStore() *publicArticleReferenceCacheStore {
	return &publicArticleReferenceCacheStore{
		items: make(map[string]publicArticleReferenceCacheEntry),
	}
}

func (c *publicArticleReferenceCacheStore) Load(key string) (publicArticleReferenceCacheEntry, bool) {
	c.mu.Lock()
	defer c.mu.Unlock()
	entry, ok := c.items[key]
	return entry, ok
}

func (c *publicArticleReferenceCacheStore) Store(key string, entry publicArticleReferenceCacheEntry) {
	c.mu.Lock()
	defer c.mu.Unlock()
	now := time.Now()
	for itemKey, item := range c.items {
		if now.After(item.expiresAt) {
			delete(c.items, itemKey)
		}
	}
	for len(c.items) >= publicArticleReferenceCacheMaxEntries {
		for itemKey := range c.items {
			delete(c.items, itemKey)
			break
		}
	}
	c.items[key] = entry
}

func (c *publicArticleReferenceCacheStore) Delete(key string) {
	c.mu.Lock()
	defer c.mu.Unlock()
	delete(c.items, key)
}

func (c *publicArticleReferenceCacheStore) Clear() {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.items = make(map[string]publicArticleReferenceCacheEntry)
}

func (c *publicArticleReferenceCacheStore) Len() int {
	c.mu.Lock()
	defer c.mu.Unlock()
	return len(c.items)
}

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
		clearPublicArticleReferenceCache()
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
		clearPublicArticleReferenceCache()
		httpx.OK(w, result)
	case http.MethodDelete:
		ok, err := s.articles.DeleteArticle(r.Context(), id)
		if err != nil {
			httpx.Fail(w, http.StatusBadRequest, err.Error())
			return
		}
		if ok {
			clearPublicArticleReferenceCache()
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
	for i := range result.Items {
		result.Items[i].Cover = publicArticleAssetURL(result.Items[i].Cover)
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
	doc.Cover = publicArticleAssetURL(doc.Cover)
	doc.AudioURL = publicArticleAssetURL(doc.AudioURL)
	doc.Content = publicArticleContentAssetURLs(doc.Content)
	httpx.OK(w, doc)
}

func (s *Server) publicArticleAsset(w http.ResponseWriter, r *http.Request) {
	idText := strings.Trim(strings.TrimPrefix(r.URL.Path, "/api/public/article-assets/"), "/")
	id, err := strconv.ParseInt(idText, 10, 64)
	if err != nil || id <= 0 {
		http.NotFound(w, r)
		return
	}
	referenced, err := cachedPublicArticleReference(fmt.Sprintf("asset:%d", id), func() (bool, error) {
		return s.articles.PublicAssetReferenced(r.Context(), id)
	})
	if err != nil || !referenced {
		http.NotFound(w, r)
		return
	}
	asset, err := s.uploads.Find(r.Context(), id)
	if err != nil {
		http.NotFound(w, r)
		return
	}
	writePublicUploadAsset(w, asset)
}

func (s *Server) publicArticleUpload(w http.ResponseWriter, r *http.Request) {
	rel := publicUploadRelativePath(r.URL.Path, "/api/public/article-uploads/")
	if rel == "" {
		http.NotFound(w, r)
		return
	}
	privateURL := "/api/uploads/" + rel
	referenced, err := cachedPublicArticleReference("upload:"+privateURL, func() (bool, error) {
		return s.articles.PublicLocalUploadReferenced(r.Context(), privateURL)
	})
	if err != nil || !referenced {
		http.NotFound(w, r)
		return
	}
	s.servePublicLocalUpload(w, r, rel)
}

func cachedPublicArticleReference(key string, load func() (bool, error)) (bool, error) {
	now := time.Now()
	if entry, ok := publicArticleReferenceCache.Load(key); ok {
		if now.Before(entry.expiresAt) {
			return entry.value, nil
		}
		publicArticleReferenceCache.Delete(key)
	}
	value, err := load()
	if err != nil {
		return false, err
	}
	ttl := publicArticleReferenceCacheTTL
	if !value {
		ttl = 5 * time.Second
	}
	publicArticleReferenceCache.Store(key, publicArticleReferenceCacheEntry{
		expiresAt: now.Add(ttl),
		value:     value,
	})
	return value, nil
}

func clearPublicArticleReferenceCache() {
	publicArticleReferenceCache.Clear()
}

func publicArticleAssetURL(raw string) string {
	if id, ok := uploadAssetIDFromURL(raw); ok {
		return fmt.Sprintf("/api/public/article-assets/%d", id)
	}
	if rel, ok := localUploadRelativePath(raw); ok {
		return "/api/public/article-uploads/" + rel
	}
	return strings.TrimSpace(raw)
}

var (
	privateArticleAssetURLInContent  = regexp.MustCompile(`(^|[\s"'(=])(/api/upload-assets/[1-9][0-9]*)($|[\s"')>\]])`)
	privateArticleUploadURLInContent = regexp.MustCompile(`(^|[\s"'(=])(/api/uploads/[^\s"'<>\])]+)($|[\s"')>\]])`)
)

func publicArticleContentAssetURLs(content string) string {
	if strings.TrimSpace(content) == "" {
		return content
	}
	content = rewriteArticleContentPrivateURLs(content, privateArticleAssetURLInContent)
	content = rewriteArticleContentPrivateURLs(content, privateArticleUploadURLInContent)
	return content
}

func rewriteArticleContentPrivateURLs(content string, pattern *regexp.Regexp) string {
	return pattern.ReplaceAllStringFunc(content, func(match string) string {
		parts := pattern.FindStringSubmatch(match)
		if len(parts) != 4 {
			return match
		}
		return parts[1] + publicArticleAssetURL(parts[2]) + parts[3]
	})
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
