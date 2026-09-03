package server

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"time"

	"nine-xing/nx-backend/apps/server/internal/httpx"
	"nine-xing/nx-backend/apps/server/internal/lifestory"
)

const lifeStoryBodyLimit = 768 * 1024

type lifeStoryErrorCode string

const (
	lifeStoryErrorUnauthorized        lifeStoryErrorCode = "life_story.unauthorized"
	lifeStoryErrorUnavailable         lifeStoryErrorCode = "life_story.unavailable"
	lifeStoryErrorInvalidID           lifeStoryErrorCode = "life_story.invalid_id"
	lifeStoryErrorInvalidBody         lifeStoryErrorCode = "life_story.invalid_body"
	lifeStoryErrorNotFound            lifeStoryErrorCode = "life_story.not_found"
	lifeStoryErrorConflict            lifeStoryErrorCode = "life_story.conflict"
	lifeStoryErrorIdempotencyConflict lifeStoryErrorCode = "life_story.idempotency_conflict"
	lifeStoryErrorInvalidState        lifeStoryErrorCode = "life_story.invalid_state"
	lifeStoryErrorQuotaExhausted      lifeStoryErrorCode = "life_story.quota_exhausted"
	lifeStoryErrorAccountInactive     lifeStoryErrorCode = "life_story.account_inactive"
	lifeStoryErrorQuestionLimit       lifeStoryErrorCode = "life_story.question_limit"
	lifeStoryErrorValidationFailed    lifeStoryErrorCode = "life_story.validation_failed"
	lifeStoryErrorRateLimited         lifeStoryErrorCode = "life_story.rate_limited"
	lifeStoryErrorMethodNotAllowed    lifeStoryErrorCode = "life_story.method_not_allowed"
	lifeStoryErrorInternal            lifeStoryErrorCode = "life_story.internal"
)

var lifeStoryErrorMessages = map[lifeStoryErrorCode]string{
	lifeStoryErrorUnauthorized:        "登录状态已失效，请重新登录",
	lifeStoryErrorUnavailable:         "故事服务暂不可用，请稍后再试",
	lifeStoryErrorInvalidID:           "请求的故事信息无效",
	lifeStoryErrorInvalidBody:         "请求内容格式有误",
	lifeStoryErrorNotFound:            "故事或相关内容不存在",
	lifeStoryErrorConflict:            "内容已更新，请刷新后重试",
	lifeStoryErrorIdempotencyConflict: "本次提交内容已变化，请重新提交",
	lifeStoryErrorInvalidState:        "当前故事状态不允许此操作",
	lifeStoryErrorQuotaExhausted:      "故事生成次数已用完",
	lifeStoryErrorAccountInactive:     "账号已停用，请重新登录",
	lifeStoryErrorQuestionLimit:       "补问最多三条",
	lifeStoryErrorValidationFailed:    "提交内容有误，请检查后重试",
	lifeStoryErrorRateLimited:         "素材保存太频繁，请稍后再试",
	lifeStoryErrorMethodNotAllowed:    "当前请求方式不受支持",
	lifeStoryErrorInternal:            "故事操作失败，请稍后重试",
}

func lifeStoryFail(w http.ResponseWriter, status int, code lifeStoryErrorCode) {
	message := lifeStoryErrorMessages[code]
	if message == "" {
		code = lifeStoryErrorInternal
		message = lifeStoryErrorMessages[code]
	}
	httpx.JSON(w, status, map[string]any{
		"code": -1, "data": nil, "error": message,
		"message": message, "errorCode": string(code),
	})
}

type lifeStoryCreateRequest struct {
	Title     string               `json:"title"`
	Materials []lifestory.Material `json:"materials,omitempty"`
}

type lifeStoryDraftRequest struct {
	Title            string               `json:"title"`
	Materials        []lifestory.Material `json:"materials,omitempty"`
	DraftVersion     int64                `json:"draftVersion"`
	DraftVersionAlt  int64                `json:"draft_version"`
	ExpectedRevision int64                `json:"expectedRevision"`
	Revision         int64                `json:"revision"`
}

type lifeStoryQuestionsRequest struct {
	Questions        []lifestory.Question `json:"questions,omitempty"`
	QuestionSetID    string               `json:"questionSetId"`
	QuestionSetIDAlt string               `json:"question_set_id"`
	QuestionID       string               `json:"questionId"`
	Answer           string               `json:"answer"`
	Skip             bool                 `json:"skip"`
	ExpectedRevision int64                `json:"expectedRevision"`
}

type lifeStoryFactsRequest struct {
	FactCard         lifestory.FactCard `json:"factCard"`
	Facts            lifestory.FactCard `json:"facts"`
	FactsVersion     int64              `json:"factsVersion"`
	FactsVersionAlt  int64              `json:"facts_version"`
	ExpectedRevision int64              `json:"expectedRevision"`
	Revision         int64              `json:"revision"`
}

type lifeStoryOutlineRequest struct {
	Outline           lifestory.Outline `json:"outline"`
	OutlineVersion    int64             `json:"outlineVersion"`
	OutlineVersionAlt int64             `json:"outline_version"`
	ExpectedRevision  int64             `json:"expectedRevision"`
	Revision          int64             `json:"revision"`
}

type lifeStoryGenerationRequest struct {
	RequestKey        string `json:"requestKey"`
	Instruction       string `json:"instruction,omitempty"`
	FactsVersion      int64  `json:"factsVersion"`
	FactsVersionAlt   int64  `json:"facts_version"`
	OutlineVersion    int64  `json:"outlineVersion"`
	OutlineVersionAlt int64  `json:"outline_version"`
	SourceVersionID   int64  `json:"sourceVersionId"`
	SourceVersion     int64  `json:"sourceVersion"`
}

type lifeStoryProgressRequest struct {
	VersionID       int64  `json:"versionId"`
	ChapterIndex    *int   `json:"chapterIndex"`
	ChapterOrder    *int   `json:"chapterOrder"`
	CharacterOffset int    `json:"characterOffset"`
	Completed       bool   `json:"completed"`
	ClientUpdatedAt string `json:"clientUpdatedAt"`
}

type lifeStoryMetaRequest struct {
	Title      *string `json:"title"`
	IsFavorite *bool   `json:"isFavorite"`
	Favorite   *bool   `json:"favorite"`
}

type lifeStoryGenerationResponse struct {
	Job            lifestory.Job `json:"job"`
	StoryRemaining int           `json:"storyRemaining"`
}

// lifeStoryRuntimeCompleter resolves the active model at execution time. This
// keeps queued jobs compatible with model-config changes made after startup
// and turns a missing provider into an explicit failed job instead of leaving
// the queue stalled indefinitely.
type lifeStoryRuntimeCompleter struct{ server *Server }

func (c lifeStoryRuntimeCompleter) CompleteJSON(ctx context.Context, system, user string, maxTokens int) (string, error) {
	if c.server == nil {
		return "", errors.New("life story model is unavailable")
	}
	return c.server.completePreferenceJSON(ctx, system, user, maxTokens)
}

func (s *Server) requireLifeStoryAuth(next http.HandlerFunc) http.HandlerFunc {
	return s.requireAppAuthWithFailure(next, func(w http.ResponseWriter) {
		w.Header().Set("Cache-Control", "private, no-store")
		lifeStoryFail(w, http.StatusUnauthorized, lifeStoryErrorUnauthorized)
	})
}

func (s *Server) appLifeStoryRouter(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Cache-Control", "private, no-store")
	user, ok := appUserFromContext(r)
	if !ok {
		lifeStoryFail(w, http.StatusUnauthorized, lifeStoryErrorUnauthorized)
		return
	}
	if s.lifeStories == nil {
		lifeStoryFail(w, http.StatusServiceUnavailable, lifeStoryErrorUnavailable)
		return
	}
	path := strings.TrimPrefix(r.URL.Path, "/api/app/life-stories")
	path = strings.Trim(path, "/")
	parts := []string{}
	if path != "" {
		parts = strings.Split(path, "/")
	}
	if len(parts) == 0 {
		s.appLifeStoryCollection(w, r, user.ID)
		return
	}
	storyID, err := strconv.ParseInt(parts[0], 10, 64)
	if err != nil || storyID <= 0 {
		lifeStoryFail(w, http.StatusBadRequest, lifeStoryErrorInvalidID)
		return
	}
	if len(parts) == 1 {
		s.appLifeStoryItem(w, r, user.ID, storyID)
		return
	}
	s.appLifeStorySubroute(w, r, user.ID, storyID, parts[1:])
}

func (s *Server) appLifeStoryCollection(w http.ResponseWriter, r *http.Request, userID int64) {
	switch r.Method {
	case http.MethodGet:
		items, err := s.lifeStories.List(r.Context(), userID)
		if err != nil {
			lifeStoryFail(w, http.StatusInternalServerError, lifeStoryErrorInternal)
			return
		}
		s.decorateLifeStories(r.Context(), userID, items)
		quota := s.lifeStoryQuota(r.Context(), userID)
		httpx.OK(w, map[string]any{"items": items, "storyRemaining": quota.Remaining})
	case http.MethodPost:
		var input lifeStoryCreateRequest
		if !decodeLifeStoryOptionalBody(w, r, &input) {
			return
		}
		if input.Materials != nil && !s.allowLifeStoryMaterialWrite(w, userID) {
			return
		}
		story, err := s.lifeStories.CreateStory(r.Context(), userID, lifestory.CreateStoryInput{Title: input.Title, Materials: input.Materials})
		if err != nil {
			lifeStoryWriteError(w, err)
			return
		}
		story.StoryRemaining = s.lifeStoryQuota(r.Context(), userID).Remaining
		httpx.OK(w, story)
	default:
		lifeStoryFail(w, http.StatusMethodNotAllowed, lifeStoryErrorMethodNotAllowed)
	}
}

func (s *Server) appLifeStoryItem(w http.ResponseWriter, r *http.Request, userID, storyID int64) {
	switch r.Method {
	case http.MethodGet:
		story, err := s.lifeStories.Get(r.Context(), userID, storyID)
		if errors.Is(err, lifestory.ErrNotFound) {
			lifeStoryFail(w, http.StatusNotFound, lifeStoryErrorNotFound)
			return
		}
		if err != nil {
			lifeStoryFail(w, http.StatusInternalServerError, lifeStoryErrorInternal)
			return
		}
		story.StoryRemaining = s.lifeStoryQuota(r.Context(), userID).Remaining
		if progress, progressErr := s.lifeStories.GetProgress(r.Context(), userID, storyID); progressErr == nil {
			// Keep the response shape stable while exposing reader state to clients.
			_ = progress
		}
		httpx.OK(w, story)
	case http.MethodDelete:
		if err := s.lifeStories.DeleteStory(r.Context(), userID, storyID); err != nil {
			lifeStoryWriteError(w, err)
			return
		}
		httpx.OK(w, map[string]bool{"deleted": true})
	case http.MethodPatch:
		var input lifeStoryMetaRequest
		if !decodeLifeStoryBody(w, r, &input) {
			return
		}
		story, err := s.lifeStories.UpdateMeta(r.Context(), userID, storyID, input.Title, firstBool(input.IsFavorite, input.Favorite))
		if err != nil {
			lifeStoryWriteError(w, err)
			return
		}
		httpx.OK(w, story)
	default:
		lifeStoryFail(w, http.StatusMethodNotAllowed, lifeStoryErrorMethodNotAllowed)
	}
}

func firstBool(a, b *bool) *bool {
	if a != nil {
		return a
	}
	return b
}

func firstPositiveInt64(values ...int64) int64 {
	for _, value := range values {
		if value > 0 {
			return value
		}
	}
	return 0
}

func isEmptyFactCard(facts lifestory.FactCard) bool {
	return facts.Version == 0 && !facts.Confirmed &&
		len(facts.Characters) == 0 && len(facts.Events) == 0 &&
		len(facts.Timeline) == 0 && len(facts.Questions) == 0 &&
		strings.TrimSpace(facts.QuestionSetID) == "" &&
		strings.TrimSpace(facts.Setting) == "" &&
		strings.TrimSpace(facts.Conflict) == "" &&
		strings.TrimSpace(facts.TurningPoint) == "" &&
		strings.TrimSpace(facts.CentralQuestion) == "" &&
		strings.TrimSpace(facts.Ending) == "" &&
		strings.TrimSpace(facts.Unresolved) == "" &&
		facts.Perspective == "" && facts.Tone == ""
}

func normalizeLifeStoryQuestions(questions []lifestory.Question) []lifestory.Question {
	for i := range questions {
		if questions[i].Sequence <= 0 {
			questions[i].Sequence = i + 1
		}
	}
	return questions
}

func resolveLifeStoryPreparationQuestions(existing, analyzed []lifestory.Question) ([]lifestory.Question, bool) {
	questions := existing
	replaceQuestionSet := false
	if isUnansweredLifeStoryFallback(existing) && len(analyzed) > 0 && !isLifeStoryFallback(analyzed) {
		questions = analyzed
		replaceQuestionSet = true
	} else if len(questions) == 0 {
		questions = analyzed
	}
	if len(questions) == 0 {
		questions = []lifestory.Question{
			{ID: "turning_point", Prompt: "这段经历中，哪个瞬间让你决定做出改变？"},
			{ID: "ending", Prompt: "事情最后如何结束？现在回头看最重要的收获是什么？"},
		}
	}
	if len(questions) > 3 {
		questions = questions[:3]
	}
	return append([]lifestory.Question(nil), questions...), replaceQuestionSet
}

func isUnansweredLifeStoryFallback(questions []lifestory.Question) bool {
	if !isLifeStoryFallback(questions) {
		return false
	}
	for _, question := range questions {
		if strings.TrimSpace(question.Answer) != "" || question.Skipped || strings.TrimSpace(question.AnsweredAt) != "" {
			return false
		}
	}
	return true
}

func isLifeStoryFallback(questions []lifestory.Question) bool {
	return len(questions) == 2 &&
		questions[0].ID == "turning_point" && questions[0].Prompt == "这段经历中，哪个瞬间让你决定做出改变？" &&
		questions[1].ID == "ending" && questions[1].Prompt == "事情最后如何结束？现在回头看最重要的收获是什么？"
}

func lifeStoryFactsEnvelope(story lifestory.Story) map[string]any {
	return map[string]any{
		"storyId":      story.ID,
		"facts":        story.FactCard,
		"factCard":     story.FactCard,
		"factsVersion": story.FactCard.Version,
	}
}

func lifeStoryOutlineEnvelope(story lifestory.Story) map[string]any {
	return map[string]any{
		"storyId":        story.ID,
		"outline":        story.Outline,
		"outlineVersion": story.Outline.Version,
	}
}

func (s *Server) appLifeStorySubroute(w http.ResponseWriter, r *http.Request, userID, storyID int64, parts []string) {
	action := strings.ToLower(strings.TrimSpace(parts[0]))
	switch action {
	case "draft":
		if r.Method != http.MethodPatch && r.Method != http.MethodPut {
			lifeStoryFail(w, http.StatusMethodNotAllowed, lifeStoryErrorMethodNotAllowed)
			return
		}
		var input lifeStoryDraftRequest
		if !decodeLifeStoryBody(w, r, &input) {
			return
		}
		expected := input.DraftVersion
		if expected == 0 {
			expected = input.DraftVersionAlt
		}
		if expected == 0 {
			expected = input.ExpectedRevision
		}
		if expected == 0 {
			expected = input.Revision
		}
		if input.Materials != nil && !s.allowLifeStoryMaterialWrite(w, userID) {
			return
		}
		story, err := s.lifeStories.SaveDraft(r.Context(), userID, storyID, expected, lifestory.DraftInput{Title: input.Title, Materials: input.Materials})
		if err != nil {
			lifeStoryWriteError(w, err)
			return
		}
		httpx.OK(w, story)
	case "prepare":
		if r.Method != http.MethodPost {
			lifeStoryFail(w, http.StatusMethodNotAllowed, lifeStoryErrorMethodNotAllowed)
			return
		}
		s.appLifeStoryPrepare(w, r, userID, storyID)
	case "questions":
		if r.Method != http.MethodPost {
			lifeStoryFail(w, http.StatusMethodNotAllowed, lifeStoryErrorMethodNotAllowed)
			return
		}
		if len(parts) >= 3 && (parts[2] == "answer" || parts[2] == "skip") {
			s.appLifeStoryAnswerQuestion(w, r, userID, storyID, parts[1], parts[2] == "skip")
			return
		}
		// Compatibility with the first internal route shape:
		// /questions/answer with questionId in the body.
		if len(parts) == 2 && (parts[1] == "answer" || parts[1] == "skip") {
			s.appLifeStoryAnswerQuestion(w, r, userID, storyID, "", parts[1] == "skip")
			return
		}
		lifeStoryFail(w, http.StatusNotFound, lifeStoryErrorNotFound)
	case "facts", "fact-card":
		if r.Method == http.MethodGet {
			story, err := s.lifeStories.Get(r.Context(), userID, storyID)
			if err != nil {
				lifeStoryWriteError(w, err)
				return
			}
			httpx.OK(w, lifeStoryFactsEnvelope(story))
			return
		}
		if r.Method != http.MethodPatch && r.Method != http.MethodPost {
			lifeStoryFail(w, http.StatusMethodNotAllowed, lifeStoryErrorMethodNotAllowed)
			return
		}
		input, ok := decodeLifeStoryFactsBody(w, r)
		if !ok {
			return
		}
		var story lifestory.Story
		var err error
		facts := input.FactCard
		if isEmptyFactCard(facts) && !isEmptyFactCard(input.Facts) {
			facts = input.Facts
		}
		expected := firstPositiveInt64(input.FactsVersion, input.FactsVersionAlt, input.ExpectedRevision, input.Revision, facts.Version)
		confirm := (len(parts) > 1 && parts[1] == "confirm") || r.URL.Query().Get("confirm") == "1" || facts.Confirmed
		if confirm {
			story, err = s.lifeStories.ConfirmFacts(r.Context(), userID, storyID, facts, expected)
		} else {
			story, err = s.lifeStories.SaveFactCard(r.Context(), userID, storyID, facts, expected)
		}
		if err != nil {
			lifeStoryWriteError(w, err)
			return
		}
		httpx.OK(w, lifeStoryFactsEnvelope(story))
	case "outline":
		if r.Method == http.MethodGet {
			story, err := s.lifeStories.Get(r.Context(), userID, storyID)
			if err != nil {
				lifeStoryWriteError(w, err)
				return
			}
			httpx.OK(w, lifeStoryOutlineEnvelope(story))
			return
		}
		if len(parts) > 1 && parts[1] == "confirm" {
			if r.Method != http.MethodPost && r.Method != http.MethodPatch {
				lifeStoryFail(w, http.StatusMethodNotAllowed, lifeStoryErrorMethodNotAllowed)
				return
			}
			input, ok := decodeLifeStoryOutlineBody(w, r)
			if !ok {
				return
			}
			expected := firstPositiveInt64(input.OutlineVersion, input.OutlineVersionAlt, input.ExpectedRevision, input.Revision, input.Outline.Version)
			var story lifestory.Story
			var err error
			if len(input.Outline.Chapters) == 0 {
				story, err = s.lifeStories.ConfirmStoredOutline(r.Context(), userID, storyID, expected)
			} else {
				story, err = s.lifeStories.ConfirmOutline(r.Context(), userID, storyID, input.Outline, expected)
			}
			if err != nil {
				lifeStoryWriteError(w, err)
				return
			}
			httpx.OK(w, lifeStoryOutlineEnvelope(story))
			return
		}
		if r.Method != http.MethodPatch && r.Method != http.MethodPost {
			lifeStoryFail(w, http.StatusMethodNotAllowed, lifeStoryErrorMethodNotAllowed)
			return
		}
		input, ok := decodeLifeStoryOutlineBody(w, r)
		if !ok {
			return
		}
		expected := firstPositiveInt64(input.OutlineVersion, input.OutlineVersionAlt, input.ExpectedRevision, input.Revision, input.Outline.Version)
		story, err := s.lifeStories.SaveOutline(r.Context(), userID, storyID, input.Outline, expected)
		if err != nil {
			lifeStoryWriteError(w, err)
			return
		}
		httpx.OK(w, lifeStoryOutlineEnvelope(story))
	case "generations", "generate":
		if len(parts) > 1 && parts[1] == "cancel" {
			s.appLifeStoryCancel(w, r, userID, storyID, 0)
			return
		}
		if r.Method != http.MethodPost {
			lifeStoryFail(w, http.StatusMethodNotAllowed, lifeStoryErrorMethodNotAllowed)
			return
		}
		var input lifeStoryGenerationRequest
		if !decodeLifeStoryBody(w, r, &input) {
			return
		}
		requestKey, keyErr := resolveLifeStoryRequestKey(r, input.RequestKey)
		if keyErr != nil {
			lifeStoryWriteError(w, keyErr)
			return
		}
		factsVersion := firstPositiveInt64(input.FactsVersion, input.FactsVersionAlt)
		outlineVersion := firstPositiveInt64(input.OutlineVersion, input.OutlineVersionAlt)
		sourceVersion := firstPositiveInt64(input.SourceVersionID, input.SourceVersion)
		job, _, err := s.lifeStories.CreateGenerationJobWithInput(r.Context(), userID, storyID, lifestory.GenerationInput{
			RequestKey: requestKey, FactsVersion: factsVersion, OutlineVersion: outlineVersion,
			SourceVersionID: sourceVersion, Instruction: input.Instruction,
		})
		if err != nil {
			lifeStoryWriteError(w, err)
			return
		}
		quota := s.lifeStoryQuota(r.Context(), userID)
		httpx.OK(w, lifeStoryGenerationResponse{Job: job, StoryRemaining: quota.Remaining})
	case "jobs":
		if len(parts) < 2 {
			lifeStoryFail(w, http.StatusBadRequest, lifeStoryErrorInvalidID)
			return
		}
		jobID, parseErr := strconv.ParseInt(parts[1], 10, 64)
		if parseErr != nil || jobID <= 0 {
			lifeStoryFail(w, http.StatusBadRequest, lifeStoryErrorInvalidID)
			return
		}
		if len(parts) > 2 && parts[2] == "cancel" {
			s.appLifeStoryCancel(w, r, userID, storyID, jobID)
			return
		}
		if r.Method != http.MethodGet {
			lifeStoryFail(w, http.StatusMethodNotAllowed, lifeStoryErrorMethodNotAllowed)
			return
		}
		job, err := s.lifeStories.GetJob(r.Context(), userID, storyID, jobID)
		if errors.Is(err, lifestory.ErrNotFound) {
			lifeStoryFail(w, http.StatusNotFound, lifeStoryErrorNotFound)
			return
		}
		if err != nil {
			lifeStoryFail(w, http.StatusInternalServerError, lifeStoryErrorInternal)
			return
		}
		httpx.OK(w, job)
	case "versions":
		if len(parts) < 2 {
			lifeStoryFail(w, http.StatusBadRequest, lifeStoryErrorInvalidID)
			return
		}
		versionID, parseErr := strconv.ParseInt(parts[1], 10, 64)
		if parseErr != nil || versionID < 0 {
			lifeStoryFail(w, http.StatusBadRequest, lifeStoryErrorInvalidID)
			return
		}
		if r.Method != http.MethodGet {
			lifeStoryFail(w, http.StatusMethodNotAllowed, lifeStoryErrorMethodNotAllowed)
			return
		}
		version, err := s.lifeStories.GetVersion(r.Context(), userID, storyID, versionID)
		if errors.Is(err, lifestory.ErrNotFound) {
			lifeStoryFail(w, http.StatusNotFound, lifeStoryErrorNotFound)
			return
		}
		if err != nil {
			lifeStoryFail(w, http.StatusInternalServerError, lifeStoryErrorInternal)
			return
		}
		httpx.OK(w, version)
	case "revisions":
		if r.Method != http.MethodPost {
			lifeStoryFail(w, http.StatusMethodNotAllowed, lifeStoryErrorMethodNotAllowed)
			return
		}
		var input lifeStoryGenerationRequest
		if !decodeLifeStoryBody(w, r, &input) {
			return
		}
		requestKey, keyErr := resolveLifeStoryRequestKey(r, input.RequestKey)
		if keyErr != nil {
			lifeStoryWriteError(w, keyErr)
			return
		}
		factsVersion := firstPositiveInt64(input.FactsVersion, input.FactsVersionAlt)
		outlineVersion := firstPositiveInt64(input.OutlineVersion, input.OutlineVersionAlt)
		sourceVersion := firstPositiveInt64(input.SourceVersionID, input.SourceVersion)
		job, _, err := s.lifeStories.CreateGenerationJobWithInput(r.Context(), userID, storyID, lifestory.GenerationInput{
			RequestKey: requestKey, FactsVersion: factsVersion, OutlineVersion: outlineVersion,
			SourceVersionID: sourceVersion, Instruction: input.Instruction,
		})
		if err != nil {
			lifeStoryWriteError(w, err)
			return
		}
		quota := s.lifeStoryQuota(r.Context(), userID)
		httpx.OK(w, lifeStoryGenerationResponse{Job: job, StoryRemaining: quota.Remaining})
	case "progress":
		if r.Method == http.MethodGet {
			progress, err := s.lifeStories.GetProgress(r.Context(), userID, storyID)
			if err != nil {
				lifeStoryWriteError(w, err)
				return
			}
			httpx.OK(w, progress)
			return
		}
		if r.Method != http.MethodPut && r.Method != http.MethodPatch {
			lifeStoryFail(w, http.StatusMethodNotAllowed, lifeStoryErrorMethodNotAllowed)
			return
		}
		var input lifeStoryProgressRequest
		if !decodeLifeStoryBody(w, r, &input) {
			return
		}
		progressInput := lifestory.ReadingProgress{
			VersionID: input.VersionID, CharacterOffset: input.CharacterOffset,
			Completed: input.Completed, ClientUpdatedAt: input.ClientUpdatedAt,
		}
		if input.ChapterIndex != nil {
			progressInput.ChapterIndex = *input.ChapterIndex
		} else if input.ChapterOrder != nil {
			progressInput.ChapterOrder = *input.ChapterOrder
		}
		progress, err := s.lifeStories.SaveProgress(r.Context(), userID, storyID, progressInput)
		if err != nil {
			lifeStoryWriteError(w, err)
			return
		}
		httpx.OK(w, progress)
	case "cancel":
		s.appLifeStoryCancel(w, r, userID, storyID, 0)
	default:
		lifeStoryFail(w, http.StatusNotFound, lifeStoryErrorNotFound)
	}
}

func (s *Server) appLifeStoryPrepare(w http.ResponseWriter, r *http.Request, userID, storyID int64) {
	story, err := s.lifeStories.Get(r.Context(), userID, storyID)
	if err != nil {
		lifeStoryWriteError(w, err)
		return
	}
	if len(story.Materials) == 0 {
		lifeStoryWriteError(w, fmt.Errorf("at least one material is required"))
		return
	}
	text := ""
	for _, material := range story.Materials {
		if strings.TrimSpace(material.Transcript) != "" {
			text += " " + material.Transcript
		} else {
			text += " " + material.Text
		}
	}
	text = strings.TrimSpace(text)
	if len([]rune(text)) > 180 {
		text = string([]rune(text)[:180])
	}
	facts := story.FactCard
	analysis := lifestory.AnalyzePreparation(
		r.Context(), lifeStoryRuntimeCompleter{server: s}, story.Materials,
	)
	facts.Organizations = lifestory.MergeFactOrganizations(facts.Organizations, analysis.Organizations)
	if len(facts.Characters) == 0 {
		facts.Characters = []lifestory.FactCharacter{{ID: "self", Alias: "我", Name: "我", Relation: "自己", RedactionMode: "pseudonym"}}
	}
	if len(facts.Events) == 0 && len(facts.Timeline) == 0 && text != "" {
		facts.Events = []lifestory.FactEvent{{ID: "event-1", Description: text, Confirmed: true, RedactionMode: "blurred"}}
	}
	questions, replaceQuestionSet := resolveLifeStoryPreparationQuestions(facts.Questions, analysis.Questions)
	questions = normalizeLifeStoryQuestions(questions)
	facts.Questions = questions
	outline := story.Outline
	if len(outline.Chapters) == 0 {
		outline.Perspective, outline.Tone = lifestory.PerspectiveFirst, lifestory.ToneWarm
		outline.Chapters = []lifestory.OutlineChapter{{Order: 1, Title: "事情发生以前", Summary: "交代人物与背景"}, {Order: 2, Title: "转折出现", Summary: "呈现冲突与选择"}, {Order: 3, Title: "走过那段路", Summary: "展开行动与变化"}, {Order: 4, Title: "回到今天", Summary: "呈现真实结局"}}
	}
	questionSetID := strings.TrimSpace(facts.QuestionSetID)
	if questionSetID == "" || replaceQuestionSet {
		questionSetID = "qs-" + lifeStoryRequestKey()
	}
	prepared, err := s.lifeStories.SavePrepared(r.Context(), userID, storyID, facts, outline, questionSetID, story.Revision)
	if err != nil {
		lifeStoryWriteError(w, err)
		return
	}
	facts = prepared.FactCard
	outline = prepared.Outline
	questions = facts.Questions
	httpx.OK(w, map[string]any{
		"storyId":        storyID,
		"questionSetId":  facts.QuestionSetID,
		"questions":      questions,
		"facts":          facts,
		"factCard":       facts,
		"factsVersion":   facts.Version,
		"outline":        outline,
		"outlineVersion": outline.Version,
		"materialCount":  len(prepared.Materials),
	})
}

func (s *Server) appLifeStoryAnswerQuestion(w http.ResponseWriter, r *http.Request, userID, storyID int64, routeQuestionID string, routeSkip bool) {
	var input lifeStoryQuestionsRequest
	if !decodeLifeStoryBody(w, r, &input) {
		return
	}
	questionID := strings.TrimSpace(routeQuestionID)
	if questionID != "" {
		if decoded, decodeErr := url.PathUnescape(questionID); decodeErr == nil {
			questionID = decoded
		}
	} else {
		questionID = strings.TrimSpace(input.QuestionID)
	}
	if questionID == "" {
		lifeStoryFail(w, http.StatusBadRequest, lifeStoryErrorInvalidID)
		return
	}
	questionSetID := strings.TrimSpace(input.QuestionSetID)
	if questionSetID == "" {
		questionSetID = strings.TrimSpace(input.QuestionSetIDAlt)
	}
	if questionSetID == "" {
		lifeStoryFail(w, http.StatusBadRequest, lifeStoryErrorInvalidID)
		return
	}
	updated, err := s.lifeStories.AnswerQuestion(r.Context(), userID, storyID, questionSetID, questionID, input.Answer, routeSkip || input.Skip)
	if err != nil {
		lifeStoryWriteError(w, err)
		return
	}
	facts := updated.FactCard
	completed := len(facts.Questions) > 0
	for _, question := range facts.Questions {
		if strings.TrimSpace(question.Answer) == "" && !question.Skipped {
			completed = false
			break
		}
	}
	httpx.OK(w, map[string]any{"questionSetId": facts.QuestionSetID, "questions": facts.Questions, "factsVersion": facts.Version, "completed": completed})
}

func (s *Server) appLifeStoryCancel(w http.ResponseWriter, r *http.Request, userID, storyID, jobID int64) {
	if r.Method != http.MethodPost {
		lifeStoryFail(w, http.StatusMethodNotAllowed, lifeStoryErrorMethodNotAllowed)
		return
	}
	if jobID == 0 {
		story, err := s.lifeStories.Get(r.Context(), userID, storyID)
		if err != nil || story.LatestJob == nil {
			lifeStoryWriteError(w, lifestory.ErrNotFound)
			return
		}
		jobID = story.LatestJob.ID
	}
	job, err := s.lifeStories.CancelJob(r.Context(), userID, storyID, jobID)
	if err != nil {
		lifeStoryWriteError(w, err)
		return
	}
	httpx.OK(w, job)
}

func decodeLifeStoryBody(w http.ResponseWriter, r *http.Request, target any) bool {
	decoder := json.NewDecoder(http.MaxBytesReader(w, r.Body, lifeStoryBodyLimit))
	if err := decoder.Decode(target); err != nil {
		lifeStoryFail(w, http.StatusBadRequest, lifeStoryErrorInvalidBody)
		return false
	}
	return finishLifeStoryBodyDecode(w, decoder)
}

func (s *Server) allowLifeStoryMaterialWrite(w http.ResponseWriter, userID int64) bool {
	if s == nil || s.lifeStoryMaterialLimiter == nil || s.lifeStoryMaterialLimiter.Allow(userID, s.nowTime()) {
		return true
	}
	w.Header().Set("Retry-After", "60")
	lifeStoryFail(w, http.StatusTooManyRequests, lifeStoryErrorRateLimited)
	return false
}

func decodeLifeStoryOptionalBody(w http.ResponseWriter, r *http.Request, target any) bool {
	decoder := json.NewDecoder(http.MaxBytesReader(w, r.Body, lifeStoryBodyLimit))
	if err := decoder.Decode(target); errors.Is(err, io.EOF) {
		return true
	} else if err != nil {
		lifeStoryFail(w, http.StatusBadRequest, lifeStoryErrorInvalidBody)
		return false
	}
	return finishLifeStoryBodyDecode(w, decoder)
}

func finishLifeStoryBodyDecode(w http.ResponseWriter, decoder *json.Decoder) bool {
	var trailing any
	if err := decoder.Decode(&trailing); !errors.Is(err, io.EOF) {
		lifeStoryFail(w, http.StatusBadRequest, lifeStoryErrorInvalidBody)
		return false
	}
	return true
}

func decodeLifeStoryFactsBody(w http.ResponseWriter, r *http.Request) (lifeStoryFactsRequest, bool) {
	var input lifeStoryFactsRequest
	raw, err := io.ReadAll(http.MaxBytesReader(w, r.Body, lifeStoryBodyLimit))
	if err != nil || len(strings.TrimSpace(string(raw))) == 0 {
		lifeStoryFail(w, http.StatusBadRequest, lifeStoryErrorInvalidBody)
		return input, false
	}
	if err := json.Unmarshal(raw, &input); err != nil {
		lifeStoryFail(w, http.StatusBadRequest, lifeStoryErrorInvalidBody)
		return input, false
	}
	var envelope map[string]json.RawMessage
	if err := json.Unmarshal(raw, &envelope); err == nil {
		if _, ok := envelope["factCard"]; !ok {
			var facts lifestory.FactCard
			if err := json.Unmarshal(raw, &facts); err == nil {
				input.FactCard = facts
			}
		}
		if input.ExpectedRevision == 0 {
			_ = json.Unmarshal(envelope["revision"], &input.ExpectedRevision)
		}
	}
	return input, true
}

func decodeLifeStoryOutlineBody(w http.ResponseWriter, r *http.Request) (lifeStoryOutlineRequest, bool) {
	var input lifeStoryOutlineRequest
	raw, err := io.ReadAll(http.MaxBytesReader(w, r.Body, lifeStoryBodyLimit))
	if err != nil || len(strings.TrimSpace(string(raw))) == 0 {
		lifeStoryFail(w, http.StatusBadRequest, lifeStoryErrorInvalidBody)
		return input, false
	}
	if err := json.Unmarshal(raw, &input); err != nil {
		lifeStoryFail(w, http.StatusBadRequest, lifeStoryErrorInvalidBody)
		return input, false
	}
	var envelope map[string]json.RawMessage
	if err := json.Unmarshal(raw, &envelope); err == nil {
		if _, ok := envelope["outline"]; !ok {
			var outline lifestory.Outline
			if err := json.Unmarshal(raw, &outline); err == nil {
				input.Outline = outline
			}
		}
		if input.ExpectedRevision == 0 {
			_ = json.Unmarshal(envelope["revision"], &input.ExpectedRevision)
		}
	}
	return input, true
}

func lifeStoryWriteError(w http.ResponseWriter, err error) {
	status := http.StatusInternalServerError
	code := lifeStoryErrorInternal
	switch {
	case errors.Is(err, lifestory.ErrNotFound):
		status, code = http.StatusNotFound, lifeStoryErrorNotFound
	case errors.Is(err, lifestory.ErrConflict):
		status, code = http.StatusConflict, lifeStoryErrorConflict
	case errors.Is(err, lifestory.ErrPayloadConflict):
		status, code = http.StatusConflict, lifeStoryErrorIdempotencyConflict
	case errors.Is(err, lifestory.ErrInvalidState):
		status, code = http.StatusConflict, lifeStoryErrorInvalidState
	case errors.Is(err, lifestory.ErrQuotaExhausted):
		status, code = http.StatusPaymentRequired, lifeStoryErrorQuotaExhausted
	case errors.Is(err, lifestory.ErrInactiveUser):
		status, code = http.StatusUnauthorized, lifeStoryErrorAccountInactive
	case errors.Is(err, lifestory.ErrTooManyQuestions):
		status, code = http.StatusUnprocessableEntity, lifeStoryErrorQuestionLimit
	case errors.Is(err, lifestory.ErrValidation):
		status, code = http.StatusBadRequest, lifeStoryErrorValidationFailed
	default:
		if err != nil {
			lower := strings.ToLower(err.Error())
			if strings.Contains(lower, "at most three questions") {
				status, code = http.StatusUnprocessableEntity, lifeStoryErrorQuestionLimit
				break
			}
			if strings.Contains(lower, "required") || strings.Contains(lower, "invalid") ||
				strings.Contains(lower, "too many") || strings.Contains(lower, "too long") ||
				strings.Contains(lower, "must ") || strings.Contains(lower, "outside") ||
				strings.Contains(lower, "at most") {
				status, code = http.StatusBadRequest, lifeStoryErrorValidationFailed
			}
		}
	}
	lifeStoryFail(w, status, code)
}

func (s *Server) lifeStoryQuotaStore() *lifestory.QuotaStore {
	if s == nil || s.lifeStories == nil {
		return nil
	}
	return s.lifeStories.QuotaStore()
}

func (s *Server) lifeStoryQuota(ctx context.Context, userID int64) lifestory.QuotaSnapshot {
	store := s.lifeStoryQuotaStore()
	if store == nil {
		return lifestory.QuotaSnapshot{PeriodKey: lifestory.DefaultQuotaPeriod}
	}
	quota, err := store.Snapshot(ctx, userID, "")
	if err != nil {
		return lifestory.QuotaSnapshot{PeriodKey: lifestory.DefaultQuotaPeriod}
	}
	return quota
}

func (s *Server) decorateLifeStories(ctx context.Context, userID int64, stories []lifestory.Story) {
	remaining := s.lifeStoryQuota(ctx, userID).Remaining
	for i := range stories {
		stories[i].StoryRemaining = remaining
	}
}

func lifeStoryRequestKey() string {
	var buf [16]byte
	if _, err := rand.Read(buf[:]); err == nil {
		return hex.EncodeToString(buf[:])
	}
	return fmt.Sprintf("story-%d", time.Now().UnixNano())
}

func resolveLifeStoryRequestKey(r *http.Request, bodyKey string) (string, error) {
	bodyKey = strings.TrimSpace(bodyKey)
	headerKey := ""
	if r != nil {
		headerKey = strings.TrimSpace(r.Header.Get("Idempotency-Key"))
	}
	if bodyKey != "" && headerKey != "" && bodyKey != headerKey {
		return "", lifestory.ErrPayloadConflict
	}
	if bodyKey != "" {
		return bodyKey, nil
	}
	if headerKey != "" {
		return headerKey, nil
	}
	return lifeStoryRequestKey(), nil
}

func (s *Server) publishLifeStoryCompletion(ctx context.Context, event lifestory.CompletionEvent) error {
	if s == nil || s.appNotifications == nil {
		return errors.New("notification service unavailable")
	}
	source := fmt.Sprintf("life-story:%d:%d", event.Story.ID, event.Version.ID)
	_, err := s.appNotifications.CreateForUser(ctx, event.Story.AppUserID, "life_story", "你的故事已经写好了", "可以打开阅读器，看看这段经历的新视角。", fmt.Sprintf("/life-stories/%d/read", event.Story.ID), source)
	return err
}

func (s *Server) dispatchLifeStoryOutboxEvent(ctx context.Context, event lifestory.OutboxEvent) error {
	if s == nil || s.lifeStories == nil {
		return errors.New("life story store unavailable")
	}
	source := fmt.Sprintf("life-story:%d:%d", event.StoryID, event.VersionID)
	return s.lifeStories.PublishOutboxNotification(
		ctx,
		event,
		"你的故事已经写好了",
		"可以打开阅读器，看看这段经历的新视角。",
		fmt.Sprintf("/life-stories/%d/read", event.StoryID),
		source,
	)
}

func (s *Server) runLifeStoryOutboxLoop(ctx context.Context) {
	if s == nil || s.lifeStories == nil {
		return
	}
	ticker := time.NewTicker(2 * time.Second)
	defer ticker.Stop()
	nextTokenCleanup := time.Time{}
	for {
		select {
		case <-ctx.Done():
			return
		default:
		}
		now := time.Now()
		if !now.Before(nextTokenCleanup) {
			cleanupCtx, cancel := context.WithTimeout(ctx, 10*time.Second)
			_, cleanupErr := s.lifeStories.PurgeExpiredTokenMaps(cleanupCtx, 500)
			cancel()
			if cleanupErr != nil {
				nextTokenCleanup = now.Add(time.Minute)
			} else {
				nextTokenCleanup = now.Add(time.Hour)
			}
		}
		event, err := s.lifeStories.ClaimOutbox(ctx)
		if err == nil {
			_ = s.dispatchLifeStoryOutboxEvent(ctx, event)
		}
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
		}
	}
}
