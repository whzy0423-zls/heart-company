package server

import (
	"context"
	"crypto/rand"
	"database/sql"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"sort"
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
	SkillID           int64  `json:"skillId,omitempty"`
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
	return c.server.completeStoryJSON(ctx, system, user, maxTokens)
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
		if err := s.ensureLifeStoryCreationAllowed(r.Context(), userID); err != nil {
			lifeStoryWriteError(w, err)
			return
		}
		story, err := s.lifeStories.CreateStory(r.Context(), userID, lifestory.CreateStoryInput{Title: input.Title, Materials: input.Materials})
		if err != nil {
			lifeStoryWriteError(w, err)
			return
		}
		s.decorateLifeStory(r.Context(), userID, &story)
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
		s.decorateLifeStory(r.Context(), userID, &story)
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
		if err := s.ensureLifeStoryWritable(r.Context(), userID, storyID); err != nil {
			lifeStoryWriteError(w, err)
			return
		}
		var input lifeStoryMetaRequest
		if !decodeLifeStoryBody(w, r, &input) {
			return
		}
		story, err := s.lifeStories.UpdateMeta(r.Context(), userID, storyID, input.Title, firstBool(input.IsFavorite, input.Favorite))
		if err != nil {
			lifeStoryWriteError(w, err)
			return
		}
		s.decorateLifeStory(r.Context(), userID, &story)
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
	payload := map[string]any{
		"storyId":      story.ID,
		"facts":        story.FactCard,
		"factCard":     story.FactCard,
		"factsVersion": story.FactCard.Version,
		// Keep the canonical event collection explicit for lightweight clients
		// and import tooling that only needs map coordinates.
		"events": story.FactCard.Events,
	}
	addLifeStoryAccessMetadata(payload, story)
	return payload
}

func lifeStoryOutlineEnvelope(story lifestory.Story) map[string]any {
	payload := map[string]any{
		"storyId":        story.ID,
		"outline":        story.Outline,
		"outlineVersion": story.Outline.Version,
	}
	addLifeStoryAccessMetadata(payload, story)
	return payload
}

func addLifeStoryAccessMetadata(payload map[string]any, story lifestory.Story) {
	if payload == nil {
		return
	}
	if strings.TrimSpace(story.AccessState) != "" {
		payload["accessState"] = story.AccessState
	}
	if strings.TrimSpace(story.RequiredPlanLevel) != "" {
		payload["requiredPlanLevel"] = story.RequiredPlanLevel
	}
	if strings.TrimSpace(story.AccessReason) != "" {
		payload["accessReason"] = story.AccessReason
	}
}

func (s *Server) appLifeStorySubroute(w http.ResponseWriter, r *http.Request, userID, storyID int64, parts []string) {
	action := strings.ToLower(strings.TrimSpace(parts[0]))
	// Generation/revision routes validate the idempotency body before checking
	// membership so malformed requests keep their stable 400/409 contract.
	// The membership gate is applied immediately after that validation below.
	if lifeStorySubrouteWrites(action, r) && action != "generations" && action != "generate" && action != "revisions" {
		if err := s.ensureLifeStoryWritable(r.Context(), userID, storyID); err != nil {
			lifeStoryWriteError(w, err)
			return
		}
	}
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
		s.decorateLifeStory(r.Context(), userID, &story)
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
			s.decorateLifeStory(r.Context(), userID, &story)
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
			s.decorateLifeStory(r.Context(), userID, &story)
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
		if err := s.ensureLifeStoryWritable(r.Context(), userID, storyID); err != nil {
			lifeStoryWriteError(w, err)
			return
		}
		factsVersion := firstPositiveInt64(input.FactsVersion, input.FactsVersionAlt)
		outlineVersion := firstPositiveInt64(input.OutlineVersion, input.OutlineVersionAlt)
		sourceVersion := firstPositiveInt64(input.SourceVersionID, input.SourceVersion)
		instruction, skillErr := s.lifeStorySkillInstruction(r.Context(), userID, storyID, input.SkillID, input.Instruction)
		if skillErr != nil {
			lifeStoryFail(w, http.StatusBadRequest, lifeStoryErrorValidationFailed)
			return
		}
		s.ensureLifeStoryMembershipQuota(r.Context(), userID)
		job, _, err := s.lifeStories.CreateGenerationJobWithInput(r.Context(), userID, storyID, lifestory.GenerationInput{
			RequestKey: requestKey, FactsVersion: factsVersion, OutlineVersion: outlineVersion,
			SourceVersionID: sourceVersion, Instruction: instruction,
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
		redactLockedLifeStoryJob(&job, s.lifeStoryAccessState(r.Context(), userID, storyID))
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
		redactLockedLifeStoryVersion(&version, s.lifeStoryAccessState(r.Context(), userID, storyID))
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
		if err := s.ensureLifeStoryWritable(r.Context(), userID, storyID); err != nil {
			lifeStoryWriteError(w, err)
			return
		}
		factsVersion := firstPositiveInt64(input.FactsVersion, input.FactsVersionAlt)
		outlineVersion := firstPositiveInt64(input.OutlineVersion, input.OutlineVersionAlt)
		sourceVersion := firstPositiveInt64(input.SourceVersionID, input.SourceVersion)
		instruction, skillErr := s.lifeStorySkillInstruction(r.Context(), userID, storyID, input.SkillID, input.Instruction)
		if skillErr != nil {
			lifeStoryFail(w, http.StatusBadRequest, lifeStoryErrorValidationFailed)
			return
		}
		s.ensureLifeStoryMembershipQuota(r.Context(), userID)
		job, _, err := s.lifeStories.CreateGenerationJobWithInput(r.Context(), userID, storyID, lifestory.GenerationInput{
			RequestKey: requestKey, FactsVersion: factsVersion, OutlineVersion: outlineVersion,
			SourceVersionID: sourceVersion, Instruction: instruction,
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
			redactLockedLifeStoryProgress(&progress, s.lifeStoryAccessState(r.Context(), userID, storyID))
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

func (s *Server) lifeStorySkillInstruction(ctx context.Context, userID, storyID, skillID int64, userInstruction string) (string, error) {
	userInstruction = strings.TrimSpace(userInstruction)
	if skillID <= 0 {
		return userInstruction, nil
	}
	story, err := s.lifeStories.Get(ctx, userID, storyID)
	if err != nil {
		return "", err
	}
	style, err := lifestory.NormalizeStoryStyle(story.Outline.StoryStyle)
	if err != nil {
		return "", err
	}
	var category, instructions string
	err = s.db.QueryRowContext(ctx, `
		SELECT category.key,version.instructions
		FROM app_skills skill
		JOIN app_skill_categories category ON category.id=skill.category_id AND category.status='enabled'
		JOIN app_skill_libraries library ON library.id=category.library_id AND library.key='story-skills' AND library.status='enabled'
		JOIN app_skill_versions version ON version.id=skill.latest_published_version_id AND version.status='published'
		WHERE skill.id=$1 AND skill.status='enabled'`, skillID).Scan(&category, &instructions)
	if err != nil {
		return "", errors.New("故事技能不存在或尚未发布")
	}
	if category != string(style) {
		return "", errors.New("故事技能与当前故事类型不匹配")
	}
	combined := "【已发布故事技能规则】\n" + strings.TrimSpace(instructions)
	if userInstruction != "" {
		combined += "\n【用户补充要求】\n" + userInstruction
	}
	runes := []rune(combined)
	if len(runes) > 6000 {
		combined = string(runes[:6000])
	}
	return combined, nil
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
	var upgradeErr *membershipUpgradeError
	if errors.As(err, &upgradeErr) {
		writeLifeStoryMembershipUpgradeRequired(w, upgradeErr.RequiredPlanLevel, upgradeErr.AccessState)
		return
	}
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

func writeLifeStoryMembershipUpgradeRequired(w http.ResponseWriter, requiredPlanLevel, accessState string) {
	message := "历史故事已保留，请升级会员后继续编辑或生成"
	if strings.TrimSpace(accessState) == resourceAccessLockedUpgrade {
		message = "该故事需要升级会员后才能继续操作"
	}
	httpx.JSON(w, http.StatusForbidden, map[string]any{
		"code":              "membership_upgrade_required",
		"errorCode":         "membership_upgrade_required",
		"error":             message,
		"message":           message,
		"requiredPlanLevel": requiredPlanLevel,
		"accessState":       accessState,
	})
}

func (s *Server) lifeStoryQuotaStore() *lifestory.QuotaStore {
	if s == nil || s.lifeStories == nil {
		return nil
	}
	return s.lifeStories.QuotaStore()
}

func (s *Server) lifeStoryQuota(ctx context.Context, userID int64) lifestory.QuotaSnapshot {
	planCode := "free"
	if s != nil && s.db != nil {
		var memberLevel string
		var expiresAt sql.NullTime
		if err := s.db.QueryRowContext(ctx, `SELECT member_level,member_expires_at FROM app_users WHERE id=$1 AND status='active'`, userID).Scan(&memberLevel, &expiresAt); err == nil {
			var membershipExpiry *time.Time
			if expiresAt.Valid {
				membershipExpiry = &expiresAt.Time
			}
			planCode = appEffectivePlanCode(memberLevel, membershipExpiry, time.Now())
		}
	}
	return s.lifeStoryQuotaForPlan(ctx, userID, planCode)
}

func (s *Server) ensureLifeStoryMembershipQuota(ctx context.Context, userID int64) {
	_ = s.lifeStoryQuota(ctx, userID)
}

func (s *Server) lifeStoryQuotaForPlan(ctx context.Context, userID int64, planCode string) lifestory.QuotaSnapshot {
	store := s.lifeStoryQuotaStore()
	if store == nil {
		return lifestory.QuotaSnapshot{PeriodKey: lifestory.DefaultQuotaPeriod}
	}
	benefits := s.appPlan(ctx, planCode)
	if planCode != "free" {
		period := time.Now().UTC().Format("2006-01")
		key := fmt.Sprintf("membership:%s:%s:minimum:%d", planCode, period, benefits.StoryMonthlyLimit)
		if quota, err := store.EnsureMinimum(ctx, userID, benefits.StoryMonthlyLimit, period, key); err == nil {
			return quota
		}
	}
	quota, err := store.Snapshot(ctx, userID, "")
	if err != nil {
		return lifestory.QuotaSnapshot{PeriodKey: lifestory.DefaultQuotaPeriod}
	}
	return quota
}

func (s *Server) decorateLifeStories(ctx context.Context, userID int64, stories []lifestory.Story) {
	plan := s.currentAppMembershipPlan(ctx, userID)
	decorateLifeStoriesForPlan(stories, plan.PlanLevel)
	remaining := s.lifeStoryQuota(ctx, userID).Remaining
	for i := range stories {
		stories[i].StoryRemaining = remaining
	}
}

const lifeStoryAccessReason = "超出当前会员等级的人生故事额度，历史内容已保留"

// redactLockedLifeStoryContent retains the story's identity and lightweight
// history fields while removing user-provided source material and generated
// content from a downgraded response. The same helper is used for list and
// detail responses so a client cannot recover the body by switching routes.
func redactLockedLifeStoryContent(story *lifestory.Story) {
	if story == nil || story.AccessState == resourceAccessActive {
		return
	}
	story.Materials = nil
	story.FactCard = lifestory.FactCard{}
	story.Outline = lifestory.Outline{}
	story.CurrentVersionID = 0
	story.CurrentVersion = nil
	story.Versions = nil
	story.LatestJob = nil
	story.Jobs = nil
	story.Progress = nil
	story.DraftVersion = 0
}

func redactLockedLifeStoryVersion(version *lifestory.Version, access membershipResourceMetadata) {
	if version == nil {
		return
	}
	version.AccessState = access.State
	version.RequiredPlanLevel = access.RequiredPlanLevel
	version.AccessReason = access.Reason
	if !membershipContentLocked(access) {
		return
	}
	version.Chapters = nil
	version.Reflection = ""
	version.CharacterCount = 0
	version.WordCount = 0
	version.Model = ""
	version.GenerationConfig = nil
}

func redactLockedLifeStoryJob(job *lifestory.Job, access membershipResourceMetadata) {
	if job == nil {
		return
	}
	job.AccessState = access.State
	job.RequiredPlanLevel = access.RequiredPlanLevel
	job.AccessReason = access.Reason
	if !membershipContentLocked(access) {
		return
	}
	// Keep status/progress for polling UI, but do not expose identifiers that
	// can be used to retrieve a locked generated version or private failures.
	job.SourceVersionID = 0
	job.VersionID = 0
	job.ErrorMessage = ""
}

func redactLockedLifeStoryProgress(progress *lifestory.ReadingProgress, access membershipResourceMetadata) {
	if progress == nil {
		return
	}
	progress.AccessState = access.State
	progress.RequiredPlanLevel = access.RequiredPlanLevel
	progress.AccessReason = access.Reason
	if !membershipContentLocked(access) {
		return
	}
	progress.VersionID = 0
	progress.ChapterIndex = 0
	progress.ChapterOrder = 0
	progress.CharacterOffset = 0
	progress.Completed = false
}

// lifeStoryHistoryLimit is the number of historical story records that remain
// fully active at a membership level. It is deliberately separate from
// StoryMonthlyLimit, which belongs to the generation quota ledger and resets
// with the quota period.
func lifeStoryHistoryLimit(planLevel string) int {
	switch normalizeMembershipLevel(planLevel) {
	case "svip":
		return 12
	case "vip":
		return 3
	default:
		return 1
	}
}

func lifeStoryRequiredPlanForRank(rank int) string {
	switch {
	case rank <= 1:
		return "free"
	case rank <= 3:
		return "vip"
	default:
		return "svip"
	}
}

// decorateLifeStoriesForPlan annotates historical stories without removing
// them. Stories keep their oldest-first priority when a user downgrades;
// newer stories become readable history but reject content mutations.
func decorateLifeStoriesForPlan(stories []lifestory.Story, planLevel string) {
	if len(stories) == 0 {
		return
	}
	order := make([]int, len(stories))
	for i := range stories {
		order[i] = i
	}
	sort.SliceStable(order, func(i, j int) bool {
		left, leftErr := time.Parse(time.RFC3339Nano, strings.TrimSpace(stories[order[i]].CreatedAt))
		right, rightErr := time.Parse(time.RFC3339Nano, strings.TrimSpace(stories[order[j]].CreatedAt))
		if leftErr == nil && rightErr == nil && !left.Equal(right) {
			return left.Before(right)
		}
		if leftErr == nil && rightErr != nil {
			return true
		}
		if leftErr != nil && rightErr == nil {
			return false
		}
		if stories[order[i]].ID != stories[order[j]].ID {
			return stories[order[i]].ID < stories[order[j]].ID
		}
		return order[i] < order[j]
	})
	planLevel = normalizeMembershipLevel(planLevel)
	limit := lifeStoryHistoryLimit(planLevel)
	for rank, index := range order {
		rank++
		required := lifeStoryRequiredPlanForRank(rank)
		state := resourceAccessActive
		reason := ""
		if rank > limit {
			state = resourceAccessReadOnlyOverLimit
			reason = lifeStoryAccessReason
		}
		stories[index].AccessState = state
		stories[index].RequiredPlanLevel = required
		stories[index].AccessReason = reason
		redactLockedLifeStoryContent(&stories[index])
	}
}

func (s *Server) decorateLifeStory(ctx context.Context, userID int64, story *lifestory.Story) {
	if story == nil {
		return
	}
	plan := s.currentAppMembershipPlan(ctx, userID)
	if s != nil && s.lifeStories != nil {
		all, err := s.lifeStories.List(ctx, userID)
		if err != nil {
			// Do not fall back to rank one when the authoritative list is
			// unavailable: that would expose a downgraded story body during a
			// transient database failure. Keep identity metadata and redact the
			// private content until access can be resolved.
			story.AccessState = resourceAccessLockedUpgrade
			story.RequiredPlanLevel = "svip"
			story.AccessReason = "会员资源状态暂不可用，请稍后重试或升级会员"
			redactLockedLifeStoryContent(story)
			return
		}
		decorateLifeStoriesForPlan(all, plan.PlanLevel)
		for _, item := range all {
			if item.ID == story.ID {
				story.AccessState = item.AccessState
				story.RequiredPlanLevel = item.RequiredPlanLevel
				story.AccessReason = item.AccessReason
				redactLockedLifeStoryContent(story)
				return
			}
		}
	}
	items := []lifestory.Story{*story}
	decorateLifeStoriesForPlan(items, plan.PlanLevel)
	story.AccessState = items[0].AccessState
	story.RequiredPlanLevel = items[0].RequiredPlanLevel
	story.AccessReason = items[0].AccessReason
	redactLockedLifeStoryContent(story)
}

func (s *Server) lifeStoryAccessState(ctx context.Context, userID, storyID int64) membershipResourceMetadata {
	plan := s.currentAppMembershipPlan(ctx, userID)
	if s != nil && s.lifeStories != nil {
		all, err := s.lifeStories.List(ctx, userID)
		if err != nil {
			return membershipResourceMetadata{
				State:             resourceAccessLockedUpgrade,
				RequiredPlanLevel: "svip",
				Reason:            "会员资源状态暂不可用，请稍后重试或升级会员",
				UpgradeRequired:   true,
			}
		}
		decorateLifeStoriesForPlan(all, plan.PlanLevel)
		for _, story := range all {
			if story.ID == storyID {
				return membershipResourceMetadata{State: story.AccessState, RequiredPlanLevel: story.RequiredPlanLevel, Reason: story.AccessReason, UpgradeRequired: story.AccessState != resourceAccessActive}
			}
		}
	}
	return membershipResourceMetadataForPlan(plan.PlanLevel, "free", "")
}

func (s *Server) ensureLifeStoryWritable(ctx context.Context, userID, storyID int64) error {
	if s == nil || s.lifeStories == nil || storyID <= 0 {
		return nil
	}
	access := s.lifeStoryAccessState(ctx, userID, storyID)
	if access.State == resourceAccessReadOnlyOverLimit || access.State == resourceAccessLockedUpgrade {
		return &membershipUpgradeError{RequiredPlanLevel: access.RequiredPlanLevel, AccessState: access.State}
	}
	return nil
}

func (s *Server) ensureLifeStoryCreationAllowed(ctx context.Context, userID int64) error {
	if s == nil || s.lifeStories == nil {
		return nil
	}
	// Creating a story draft does not consume a generation. Historical story
	// count is an access-decoration concern only; using it here would make a
	// user who reached a plan's limit in an earlier month permanently unable to
	// start another story. The generation job transaction calls reserveQuotaTx,
	// which applies the current month's quota atomically and returns
	// ErrQuotaExhausted when the allowance is spent.
	//
	// Keep this hook so callers can retain a single policy boundary and so a
	// future draft-specific entitlement can be added without reintroducing a
	// history-count gate.
	_ = ctx
	_ = userID
	return nil
}

func lifeStorySubrouteWrites(action string, r *http.Request) bool {
	if r == nil {
		return false
	}
	if action == "cancel" || (action == "generations" || action == "jobs") && len(r.URL.Path) > 0 && strings.Contains(r.URL.Path, "/cancel") {
		return false
	}
	switch action {
	case "draft", "prepare", "questions", "facts", "outline", "generations", "generate", "revisions", "progress":
		return r.Method != http.MethodGet
	default:
		return false
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
	if len([]rune(bodyKey)) > 128 || len([]rune(headerKey)) > 128 {
		return "", errors.New("request key is required")
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
