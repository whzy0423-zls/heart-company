package server

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"strconv"
	"strings"
	"time"

	"nine-xing/nx-backend/apps/server/internal/httpx"
	"nine-xing/nx-backend/apps/server/internal/rag"
	"nine-xing/nx-backend/apps/server/internal/skillcatalog"
	"nine-xing/nx-backend/apps/server/internal/skillchat"
)

type skillChatRuntimeGenerator struct{ server *Server }

func (g skillChatRuntimeGenerator) Generate(ctx context.Context, input rag.GenerateInput) (string, error) {
	if g.server == nil {
		return "", errors.New("skill chat generator unavailable")
	}
	generator, _ := g.server.chatRuntime()
	if generator == nil {
		return "", errors.New("skill chat generator unavailable")
	}
	return generator.Generate(ctx, input)
}

func (g skillChatRuntimeGenerator) GenerateStream(ctx context.Context, input rag.GenerateInput, emit rag.StreamEmitter) (string, error) {
	if g.server == nil {
		return "", errors.New("skill chat generator unavailable")
	}
	generator, _ := g.server.chatRuntime()
	if generator == nil {
		return "", errors.New("skill chat generator unavailable")
	}
	if streaming, ok := generator.(rag.StreamingGenerator); ok {
		emittedContent := false
		answer, err := streaming.GenerateStream(ctx, input, func(delta string) error {
			if strings.TrimSpace(delta) != "" {
				emittedContent = true
			}
			if emit == nil {
				return nil
			}
			return emit(delta)
		})
		if err == nil || emittedContent {
			return answer, err
		}
		// Some OpenAI-compatible gateways accept stream=true but fail before
		// producing content. A one-shot fallback preserves SSE semantics without
		// risking duplicate text after a partial stream.
		answer, err = generator.Generate(ctx, input)
		if err == nil && strings.TrimSpace(answer) != "" && emit != nil {
			err = emit(answer)
		}
		return answer, err
	}
	answer, err := generator.Generate(ctx, input)
	if err == nil && strings.TrimSpace(answer) != "" && emit != nil {
		err = emit(answer)
	}
	return answer, err
}

func appPathID(path, prefix, suffix string) (int64, bool) {
	if !strings.HasPrefix(path, prefix) || (suffix != "" && !strings.HasSuffix(path, suffix)) {
		return 0, false
	}
	rest := strings.TrimPrefix(path, prefix)
	if suffix != "" {
		rest = strings.TrimSuffix(rest, suffix)
	}
	id, err := strconv.ParseInt(strings.Trim(rest, "/"), 10, 64)
	return id, err == nil && id > 0
}

func (s *Server) appSkillLibrariesRouter(w http.ResponseWriter, r *http.Request) {
	path := r.URL.Path
	switch {
	case path == "/api/app/skill-libraries" && r.Method == http.MethodGet:
		items, err := s.skillCatalog.ListLibraries(r.Context())
		if err != nil {
			httpx.Fail(w, http.StatusInternalServerError, "技能库读取失败")
			return
		}
		httpx.OK(w, items)
	case strings.HasSuffix(path, "/categories") && r.Method == http.MethodGet:
		libraryID, ok := appPathID(path, "/api/app/skill-libraries/", "/categories")
		if !ok {
			httpx.Fail(w, http.StatusBadRequest, "技能库编号无效")
			return
		}
		items, err := s.skillCatalog.ListCategories(r.Context(), libraryID)
		if err != nil {
			httpx.Fail(w, http.StatusInternalServerError, "技能分类读取失败")
			return
		}
		httpx.OK(w, items)
	case strings.HasSuffix(path, "/skills") && r.Method == http.MethodGet:
		libraryID, ok := appPathID(path, "/api/app/skill-libraries/", "/skills")
		if !ok {
			httpx.Fail(w, http.StatusBadRequest, "技能库编号无效")
			return
		}
		categoryID, _ := strconv.ParseInt(r.URL.Query().Get("categoryId"), 10, 64)
		limit, _ := strconv.Atoi(r.URL.Query().Get("limit"))
		cursorSort, cursorID, err := skillcatalog.DecodeCursor(r.URL.Query().Get("cursor"))
		if err != nil {
			httpx.Fail(w, http.StatusBadRequest, "分页游标无效")
			return
		}
		page, err := s.skillCatalog.ListSkills(r.Context(), skillcatalog.SkillFilter{
			LibraryID: libraryID, CategoryID: categoryID, Query: r.URL.Query().Get("query"),
			CursorSort: cursorSort, CursorID: cursorID, Limit: limit,
		})
		if err != nil {
			httpx.Fail(w, http.StatusInternalServerError, "技能目录读取失败")
			return
		}
		httpx.OK(w, page)
	default:
		httpx.Fail(w, http.StatusMethodNotAllowed, "method not allowed")
	}
}

func (s *Server) appSkillsRouter(w http.ResponseWriter, r *http.Request) {
	path := r.URL.Path
	user, ok := appUserFromContext(r)
	if !ok {
		httpx.Fail(w, http.StatusUnauthorized, "unauthorized")
		return
	}
	switch {
	case strings.HasSuffix(path, "/sessions/latest") && r.Method == http.MethodGet:
		skillID, valid := appPathID(path, "/api/app/skills/", "/sessions/latest")
		if !valid {
			httpx.Fail(w, http.StatusBadRequest, "技能编号无效")
			return
		}
		session, err := s.skillChat.LatestSession(r.Context(), user.ID, skillID)
		if errors.Is(err, skillchat.ErrNotFound) {
			httpx.Fail(w, http.StatusNotFound, "暂无技能会话")
			return
		}
		if err != nil {
			httpx.Fail(w, http.StatusInternalServerError, "技能会话读取失败")
			return
		}
		httpx.OK(w, session)
	case strings.HasSuffix(path, "/sessions") && r.Method == http.MethodGet:
		skillID, valid := appPathID(path, "/api/app/skills/", "/sessions")
		if !valid {
			httpx.Fail(w, http.StatusBadRequest, "技能编号无效")
			return
		}
		items, err := s.skillChat.ListSessions(r.Context(), user.ID, skillID)
		if err != nil {
			httpx.Fail(w, http.StatusInternalServerError, "技能会话读取失败")
			return
		}
		httpx.OK(w, items)
	case strings.HasSuffix(path, "/sessions") && r.Method == http.MethodPost:
		skillID, valid := appPathID(path, "/api/app/skills/", "/sessions")
		if !valid {
			httpx.Fail(w, http.StatusBadRequest, "技能编号无效")
			return
		}
		var body struct {
			Title string `json:"title"`
		}
		if r.Body != nil {
			_ = json.NewDecoder(io.LimitReader(r.Body, 8<<10)).Decode(&body)
		}
		session, err := s.skillChat.CreateSession(r.Context(), user.ID, skillID, body.Title)
		if errors.Is(err, skillchat.ErrNotFound) {
			httpx.Fail(w, http.StatusNotFound, "技能不存在或尚未发布")
			return
		}
		if err != nil {
			httpx.Fail(w, http.StatusInternalServerError, "技能会话创建失败")
			return
		}
		httpx.OK(w, session)
	case r.Method == http.MethodGet:
		skillID, valid := appPathID(path, "/api/app/skills/", "")
		if !valid {
			httpx.Fail(w, http.StatusBadRequest, "技能编号无效")
			return
		}
		item, err := s.skillCatalog.GetSkill(r.Context(), skillID)
		if errors.Is(err, skillcatalog.ErrNotFound) {
			httpx.Fail(w, http.StatusNotFound, "技能不存在或尚未发布")
			return
		}
		if err != nil {
			httpx.Fail(w, http.StatusInternalServerError, "技能读取失败")
			return
		}
		httpx.OK(w, item)
	default:
		httpx.Fail(w, http.StatusMethodNotAllowed, "method not allowed")
	}
}

func (s *Server) appSkillSessionsRouter(w http.ResponseWriter, r *http.Request) {
	path := r.URL.Path
	user, ok := appUserFromContext(r)
	if !ok {
		httpx.Fail(w, http.StatusUnauthorized, "unauthorized")
		return
	}
	switch {
	case strings.HasSuffix(path, "/messages") && r.Method == http.MethodGet:
		sessionID, valid := appPathID(path, "/api/app/skill-sessions/", "/messages")
		if !valid {
			httpx.Fail(w, http.StatusBadRequest, "会话编号无效")
			return
		}
		items, err := s.skillChat.ListMessages(r.Context(), user.ID, sessionID)
		if err != nil {
			httpx.Fail(w, http.StatusInternalServerError, "消息读取失败")
			return
		}
		httpx.OK(w, items)
	case strings.HasSuffix(path, "/ask/stream") && r.Method == http.MethodPost:
		s.appSkillSessionAskStream(w, r, user.ID)
	case strings.HasSuffix(path, "/ask") && r.Method == http.MethodPost:
		sessionID, valid := appPathID(path, "/api/app/skill-sessions/", "/ask")
		if !valid {
			httpx.Fail(w, http.StatusBadRequest, "会话编号无效")
			return
		}
		if s.chatLimiter != nil && !s.chatLimiter.Allow(user.ID, time.Now()) {
			httpx.Fail(w, http.StatusTooManyRequests, "请求过于频繁，请稍后再试")
			return
		}
		question, valid := decodeSkillQuestion(r)
		if !valid {
			httpx.Fail(w, http.StatusBadRequest, "请输入问题")
			return
		}
		_, timeout := s.chatRuntime()
		ctx, cancel := context.WithTimeout(r.Context(), timeout)
		defer cancel()
		result, err := s.skillChatRuntime.Ask(ctx, user.ID, sessionID, question)
		if err != nil {
			failSkillChat(w, err)
			return
		}
		httpx.OK(w, result)
	case strings.HasSuffix(path, "/voice") && r.Method == http.MethodPost:
		s.appSkillSessionVoice(w, r, user.ID)
	case r.Method == http.MethodPatch:
		sessionID, valid := appPathID(path, "/api/app/skill-sessions/", "")
		if !valid {
			httpx.Fail(w, http.StatusBadRequest, "会话编号无效")
			return
		}
		var body struct {
			Title *string `json:"title"`
			Clear bool    `json:"clear"`
		}
		if err := json.NewDecoder(io.LimitReader(r.Body, 8<<10)).Decode(&body); err != nil || (body.Title == nil && !body.Clear) {
			httpx.Fail(w, http.StatusBadRequest, "请输入标题或清空操作")
			return
		}
		session, err := s.skillChat.UpdateSession(r.Context(), user.ID, sessionID, body.Title, body.Clear)
		if err != nil {
			failSkillChat(w, err)
			return
		}
		httpx.OK(w, session)
	case r.Method == http.MethodDelete:
		sessionID, valid := appPathID(path, "/api/app/skill-sessions/", "")
		if !valid {
			httpx.Fail(w, http.StatusBadRequest, "会话编号无效")
			return
		}
		if err := s.skillChat.DeleteSession(r.Context(), user.ID, sessionID); err != nil {
			httpx.Fail(w, http.StatusNotFound, "会话不存在")
			return
		}
		httpx.OK(w, map[string]bool{"deleted": true})
	default:
		httpx.Fail(w, http.StatusMethodNotAllowed, "method not allowed")
	}
}

func decodeSkillQuestion(r *http.Request) (string, bool) {
	var body struct {
		Question string `json:"question"`
	}
	if err := json.NewDecoder(io.LimitReader(r.Body, 32<<10)).Decode(&body); err != nil {
		return "", false
	}
	body.Question = strings.TrimSpace(body.Question)
	return body.Question, body.Question != ""
}

func failSkillChat(w http.ResponseWriter, err error) {
	switch {
	case errors.Is(err, skillchat.ErrNotFound), errors.Is(err, skillchat.ErrVersionUnavailable):
		httpx.Fail(w, http.StatusNotFound, "技能会话不存在或已停用")
	case errors.Is(err, skillchat.ErrInvalidInput):
		httpx.Fail(w, http.StatusBadRequest, "问题格式无效")
	case errors.Is(err, skillchat.ErrSessionChanged):
		httpx.Fail(w, http.StatusConflict, "会话已清空，请重新发送")
	case errors.Is(err, context.DeadlineExceeded):
		httpx.Fail(w, http.StatusGatewayTimeout, "回答生成超时，请重试")
	default:
		httpx.Fail(w, http.StatusInternalServerError, "回答生成失败，请重试")
	}
}

func (s *Server) appSkillSessionAskStream(w http.ResponseWriter, r *http.Request, appUserID int64) {
	sessionID, valid := appPathID(r.URL.Path, "/api/app/skill-sessions/", "/ask/stream")
	if !valid {
		httpx.Fail(w, http.StatusBadRequest, "会话编号无效")
		return
	}
	if s.chatLimiter != nil && !s.chatLimiter.Allow(appUserID, time.Now()) {
		httpx.Fail(w, http.StatusTooManyRequests, "请求过于频繁，请稍后再试")
		return
	}
	question, valid := decodeSkillQuestion(r)
	if !valid {
		httpx.Fail(w, http.StatusBadRequest, "请输入问题")
		return
	}
	flusher, ok := w.(http.Flusher)
	if !ok {
		httpx.Fail(w, http.StatusInternalServerError, "streaming unsupported")
		return
	}
	_, timeout := s.chatRuntime()
	ctx, cancel := context.WithTimeout(r.Context(), timeout)
	defer cancel()
	w.Header().Set("Content-Type", "text/event-stream; charset=utf-8")
	w.Header().Set("Cache-Control", "no-cache")
	w.Header().Set("X-Accel-Buffering", "no")
	_, _ = io.WriteString(w, ": connected\n\n")
	flusher.Flush()
	result, err := s.skillChatRuntime.AskStream(ctx, appUserID, sessionID, question, func(delta string) error {
		return writeAppChatSSE(w, flusher, "delta", map[string]string{"content": delta})
	})
	if err != nil {
		_ = writeAppChatSSE(w, flusher, "error", map[string]string{"message": "回答生成失败，请重试"})
		return
	}
	_ = writeAppChatSSE(w, flusher, "done", result)
}

var _ rag.StreamingGenerator = skillChatRuntimeGenerator{}
