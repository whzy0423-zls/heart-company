package server

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"testing"
	"time"
	"unicode"

	"nine-xing/nx-backend/apps/server/internal/auth"
	"nine-xing/nx-backend/apps/server/internal/config"
	"nine-xing/nx-backend/apps/server/internal/lifestory"
)

type lifeStoryErrorPayload struct {
	Code      int    `json:"code"`
	ErrorCode string `json:"errorCode"`
	Message   string `json:"message"`
	Error     string `json:"error"`
}

func decodeLifeStoryErrorPayload(t *testing.T, recorder *httptest.ResponseRecorder) lifeStoryErrorPayload {
	t.Helper()
	var payload lifeStoryErrorPayload
	if err := json.Unmarshal(recorder.Body.Bytes(), &payload); err != nil {
		t.Fatalf("decode error response: %v body=%s", err, recorder.Body.String())
	}
	if payload.Code != -1 || payload.ErrorCode == "" {
		t.Fatalf("unstable error envelope: %+v", payload)
	}
	if payload.Message == "" || payload.Error != payload.Message {
		t.Fatalf("error/message mismatch: %+v", payload)
	}
	hasHan := false
	for _, r := range payload.Message {
		if unicode.Is(unicode.Han, r) {
			hasHan = true
			break
		}
	}
	if !hasHan {
		t.Fatalf("life story message is not Chinese: %+v", payload)
	}
	return payload
}

func TestLifeStoryResponsesUsePrivateNoStoreCachePolicy(t *testing.T) {
	s := &Server{}
	req := httptest.NewRequest(http.MethodGet, "/api/app/life-stories", nil)
	res := httptest.NewRecorder()

	s.appLifeStoryRouter(res, req)

	if got := res.Header().Get("Cache-Control"); got != "private, no-store" {
		t.Fatalf("Cache-Control=%q want private, no-store", got)
	}
}

func TestLifeStoryMuxAuthenticationUsesDedicatedErrorContract(t *testing.T) {
	const secret = "life-story-auth-contract-secret"
	expiredToken, err := auth.SignWithExpiry(auth.UserInfo{ID: 7, TokenKind: auth.TokenKindApp}, secret, -time.Hour)
	if err != nil {
		t.Fatal(err)
	}
	tests := []struct {
		name          string
		authorization string
	}{
		{name: "missing token"},
		{name: "expired token", authorization: "Bearer " + expiredToken},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			server := &Server{env: config.Env{JWTSecret: secret}, mux: http.NewServeMux()}
			server.routes()
			request := httptest.NewRequest(http.MethodGet, "/api/app/life-stories", nil)
			request.Header.Set("Authorization", tt.authorization)
			response := httptest.NewRecorder()

			server.mux.ServeHTTP(response, request)

			payload := decodeLifeStoryErrorPayload(t, response)
			if response.Code != http.StatusUnauthorized || payload.ErrorCode != "life_story.unauthorized" {
				t.Fatalf("status=%d payload=%+v body=%s", response.Code, payload, response.Body.String())
			}
			if got := response.Header().Get("Cache-Control"); got != "private, no-store" {
				t.Fatalf("Cache-Control=%q want private, no-store", got)
			}
		})
	}
}

func TestOtherAppMuxRoutesKeepGenericAuthenticationContract(t *testing.T) {
	server := &Server{env: config.Env{JWTSecret: "other-app-auth-secret"}, mux: http.NewServeMux()}
	server.routes()
	response := httptest.NewRecorder()

	server.mux.ServeHTTP(response, httptest.NewRequest(http.MethodGet, "/api/app/me", nil))

	var payload map[string]any
	if err := json.Unmarshal(response.Body.Bytes(), &payload); err != nil {
		t.Fatal(err)
	}
	if response.Code != http.StatusUnauthorized || payload["message"] != "unauthorized" {
		t.Fatalf("status=%d payload=%+v", response.Code, payload)
	}
	if _, ok := payload["errorCode"]; ok {
		t.Fatalf("non-Life Story auth response unexpectedly changed: %+v", payload)
	}
}

func TestLifeStoryWriteErrorMapsQuestionLimitToUnprocessableEntity(t *testing.T) {
	recorder := httptest.NewRecorder()
	lifeStoryWriteError(recorder, errors.Join(lifestory.ErrTooManyQuestions, errors.New("at most three questions")))
	if recorder.Code != http.StatusUnprocessableEntity {
		t.Fatalf("status=%d body=%s", recorder.Code, recorder.Body.String())
	}
}

func TestLifeStoryWriteErrorMapsPublicContractStatuses(t *testing.T) {
	_, oversizedVoiceErr := lifestory.NewStore(nil).CreateStory(context.Background(), 7, lifestory.CreateStoryInput{
		Materials: []lifestory.Material{{
			SourceType: lifestory.MaterialVoice,
			Transcript: "一段口述素材",
			ByteLength: 10*1024*1024 + 1,
		}},
	})
	missingMaterialsErr := lifestory.ValidateSnapshot(lifestory.StorySnapshot{StoryID: 7})
	if oversizedVoiceErr == nil || missingMaterialsErr == nil {
		t.Fatalf("validation fixtures must fail: oversizedVoice=%v missingMaterials=%v", oversizedVoiceErr, missingMaterialsErr)
	}
	tests := []struct {
		name     string
		err      error
		want     int
		wantCode string
	}{
		{name: "not found", err: lifestory.ErrNotFound, want: http.StatusNotFound, wantCode: "life_story.not_found"},
		{name: "revision conflict", err: lifestory.ErrConflict, want: http.StatusConflict, wantCode: "life_story.conflict"},
		{name: "invalid parameter", err: errors.New("chapter index must be non-negative"), want: http.StatusBadRequest, wantCode: "life_story.validation_failed"},
		{name: "payload conflict", err: lifestory.ErrPayloadConflict, want: http.StatusConflict, wantCode: "life_story.idempotency_conflict"},
		{name: "invalid state", err: lifestory.ErrInvalidState, want: http.StatusConflict, wantCode: "life_story.invalid_state"},
		{name: "quota exhausted", err: lifestory.ErrQuotaExhausted, want: http.StatusPaymentRequired, wantCode: "life_story.quota_exhausted"},
		{name: "inactive user", err: lifestory.ErrInactiveUser, want: http.StatusUnauthorized, wantCode: "life_story.account_inactive"},
		{name: "question limit", err: lifestory.ErrTooManyQuestions, want: http.StatusUnprocessableEntity, wantCode: "life_story.question_limit"},
		{name: "oversized voice material", err: oversizedVoiceErr, want: http.StatusBadRequest, wantCode: "life_story.validation_failed"},
		{name: "snapshot without materials", err: missingMaterialsErr, want: http.StatusBadRequest, wantCode: "life_story.validation_failed"},
		{name: "internal", err: errors.New("database exploded"), want: http.StatusInternalServerError, wantCode: "life_story.internal"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			recorder := httptest.NewRecorder()
			lifeStoryWriteError(recorder, tt.err)
			if recorder.Code != tt.want {
				t.Fatalf("status=%d want=%d body=%s", recorder.Code, tt.want, recorder.Body.String())
			}
			payload := decodeLifeStoryErrorPayload(t, recorder)
			if payload.ErrorCode != tt.wantCode {
				t.Fatalf("errorCode=%q want=%q body=%s", payload.ErrorCode, tt.wantCode, recorder.Body.String())
			}
			if strings.Contains(payload.Message, tt.err.Error()) {
				t.Fatalf("response leaked internal error %q: %s", tt.err, recorder.Body.String())
			}
		})
	}
}

func TestLifeStoryDirectErrorsUseStableChineseCodes(t *testing.T) {
	t.Run("invalid body", func(t *testing.T) {
		recorder := httptest.NewRecorder()
		request := httptest.NewRequest(http.MethodPost, "/api/app/life-stories/1/draft", strings.NewReader("{"))
		var input lifeStoryDraftRequest
		if decodeLifeStoryBody(recorder, request, &input) {
			t.Fatal("malformed body was accepted")
		}
		payload := decodeLifeStoryErrorPayload(t, recorder)
		if payload.ErrorCode != "life_story.invalid_body" {
			t.Fatalf("errorCode=%q body=%s", payload.ErrorCode, recorder.Body.String())
		}
	})

	t.Run("method not allowed", func(t *testing.T) {
		recorder := httptest.NewRecorder()
		request := httptest.NewRequest(http.MethodGet, "/api/app/life-stories/1/draft", nil)
		new(Server).appLifeStorySubroute(recorder, request, 1, 1, []string{"draft"})
		payload := decodeLifeStoryErrorPayload(t, recorder)
		if recorder.Code != http.StatusMethodNotAllowed || payload.ErrorCode != "life_story.method_not_allowed" {
			t.Fatalf("unexpected method response: status=%d payload=%+v", recorder.Code, payload)
		}
	})

	t.Run("rate limited", func(t *testing.T) {
		now := time.Date(2026, 8, 30, 12, 0, 0, 0, time.UTC)
		server := &Server{lifeStoryMaterialLimiter: newFixedWindowRateLimiter(1, time.Minute), now: func() time.Time { return now }}
		if !server.allowLifeStoryMaterialWrite(httptest.NewRecorder(), 1) {
			t.Fatal("first write was unexpectedly limited")
		}
		recorder := httptest.NewRecorder()
		if server.allowLifeStoryMaterialWrite(recorder, 1) {
			t.Fatal("second write was not limited")
		}
		payload := decodeLifeStoryErrorPayload(t, recorder)
		if payload.ErrorCode != "life_story.rate_limited" {
			t.Fatalf("errorCode=%q body=%s", payload.ErrorCode, recorder.Body.String())
		}
	})
}

func TestLifeStoryRoutesDoNotBypassStructuredErrorResponses(t *testing.T) {
	raw, err := os.ReadFile("app_life_story.go")
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(raw), "httpx.Fail(") {
		t.Fatal("app_life_story.go still contains unstructured httpx.Fail calls")
	}
}

func TestDecodeLifeStoryOptionalBodyAcceptsEmptyCreateRequest(t *testing.T) {
	recorder := httptest.NewRecorder()
	request := httptest.NewRequest(http.MethodPost, "/api/app/life-stories", nil)
	var input lifeStoryCreateRequest
	if !decodeLifeStoryOptionalBody(recorder, request, &input) {
		t.Fatalf("empty create body was rejected: status=%d body=%s", recorder.Code, recorder.Body.String())
	}
	if input.Title != "" || input.Materials != nil {
		t.Fatalf("unexpected decoded create input: %+v", input)
	}
}

func TestDecodeLifeStoryBodyRejectsTrailingContent(t *testing.T) {
	tests := []struct {
		name string
		body string
	}{
		{name: "second JSON value", body: `{"draftVersion":1}{"draftVersion":2}`},
		{name: "trailing garbage", body: `{"draftVersion":1} trailing`},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			response := httptest.NewRecorder()
			request := httptest.NewRequest(http.MethodPatch, "/api/app/life-stories/1/draft", strings.NewReader(tt.body))
			var input lifeStoryDraftRequest

			if decodeLifeStoryBody(response, request, &input) {
				t.Fatalf("invalid body was accepted: %q", tt.body)
			}
			payload := decodeLifeStoryErrorPayload(t, response)
			if response.Code != http.StatusBadRequest || payload.ErrorCode != "life_story.invalid_body" {
				t.Fatalf("status=%d payload=%+v", response.Code, payload)
			}
		})
	}
}

func TestDecodeLifeStoryOptionalBodyRejectsTrailingContent(t *testing.T) {
	tests := []struct {
		name string
		body string
	}{
		{name: "second JSON value", body: `{"title":"first"}{"title":"second"}`},
		{name: "trailing garbage", body: `{"title":"first"} trailing`},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			response := httptest.NewRecorder()
			request := httptest.NewRequest(http.MethodPost, "/api/app/life-stories", strings.NewReader(tt.body))
			var input lifeStoryCreateRequest

			if decodeLifeStoryOptionalBody(response, request, &input) {
				t.Fatalf("invalid optional body was accepted: %q", tt.body)
			}
			payload := decodeLifeStoryErrorPayload(t, response)
			if response.Code != http.StatusBadRequest || payload.ErrorCode != "life_story.invalid_body" {
				t.Fatalf("status=%d payload=%+v", response.Code, payload)
			}
		})
	}
}

func TestNormalizeLifeStoryQuestionsAssignsStableOneBasedSequence(t *testing.T) {
	questions := normalizeLifeStoryQuestions([]lifestory.Question{
		{ID: "q1", Prompt: "第一问"},
		{ID: "q2", Prompt: "第二问", Sequence: 9, Required: true},
	})
	if questions[0].Sequence != 1 || questions[1].Sequence != 9 || !questions[1].Required {
		t.Fatalf("question metadata was not normalized and preserved: %+v", questions)
	}
}

func TestResolveLifeStoryPreparationQuestionsUsesAnalysisWithoutOverwritingExistingAnswers(t *testing.T) {
	dynamic := []lifestory.Question{
		{ID: "memory_a", Prompt: "你在车站告别时，最想留住的画面是什么？", Sequence: 1},
		{ID: "memory_b", Prompt: "列车开动以后，你当时做出的第一个决定是什么？", Sequence: 2},
		{ID: "memory_c", Prompt: "多年后回看那次离开，它改变了你和家人的哪种关系？", Sequence: 3},
		{ID: "memory_d", Prompt: "第四问不应进入事实卡。", Sequence: 4},
	}

	resolved, replaced := resolveLifeStoryPreparationQuestions(nil, dynamic)
	if len(resolved) != 3 || resolved[0].ID != "memory_a" || resolved[2].ID != "memory_c" || replaced {
		t.Fatalf("resolved dynamic questions=%+v", resolved)
	}

	existing := []lifestory.Question{{
		ID: "answered", Prompt: "原来的问题", Sequence: 1, Answer: "原来的回答", AnsweredAt: "2026-08-30T12:00:00Z",
	}}
	resolved, replaced = resolveLifeStoryPreparationQuestions(existing, dynamic)
	if len(resolved) != 1 || resolved[0] != existing[0] || replaced {
		t.Fatalf("existing answered question was replaced: %+v", resolved)
	}

	fallback := []lifestory.Question{
		{ID: "turning_point", Prompt: "这段经历中，哪个瞬间让你决定做出改变？", Sequence: 1},
		{ID: "ending", Prompt: "事情最后如何结束？现在回头看最重要的收获是什么？", Sequence: 2},
	}
	resolved, replaced = resolveLifeStoryPreparationQuestions(fallback, dynamic)
	if len(resolved) != 3 || resolved[0].ID != "memory_a" || !replaced {
		t.Fatalf("unanswered fallback questions were not upgraded: %+v", resolved)
	}
	fallback[0].Answer = "我决定离开的那一刻"
	resolved, replaced = resolveLifeStoryPreparationQuestions(fallback, dynamic)
	if len(resolved) != 2 || resolved[0].ID != "turning_point" || resolved[0].Answer == "" || replaced {
		t.Fatalf("answered fallback questions were replaced: %+v", resolved)
	}

	resolved, replaced = resolveLifeStoryPreparationQuestions(nil, nil)
	if len(resolved) != 2 || resolved[0].ID != "turning_point" || resolved[1].ID != "ending" || replaced {
		t.Fatalf("fallback questions=%+v", resolved)
	}
}

func TestLifeStoryMaterialWritesAreLimitedPerUser(t *testing.T) {
	now := time.Date(2026, 8, 30, 12, 0, 0, 0, time.UTC)
	server := &Server{
		lifeStoryMaterialLimiter: newFixedWindowRateLimiter(10, time.Minute),
		now:                      func() time.Time { return now },
	}
	for i := 0; i < 10; i++ {
		if ok := server.allowLifeStoryMaterialWrite(httptest.NewRecorder(), 7); !ok {
			t.Fatalf("write %d was unexpectedly limited", i+1)
		}
	}
	recorder := httptest.NewRecorder()
	if server.allowLifeStoryMaterialWrite(recorder, 7) {
		t.Fatal("eleventh write should be limited")
	}
	if recorder.Code != http.StatusTooManyRequests || recorder.Header().Get("Retry-After") != "60" {
		t.Fatalf("status=%d retry-after=%q", recorder.Code, recorder.Header().Get("Retry-After"))
	}
	if ok := server.allowLifeStoryMaterialWrite(httptest.NewRecorder(), 8); !ok {
		t.Fatal("another user must have an independent limit")
	}
}

func TestLifeStoryRuntimeCompleterReadsCurrentGenerator(t *testing.T) {
	server := &Server{}
	completer := lifeStoryRuntimeCompleter{server: server}
	if _, err := completer.CompleteJSON(context.Background(), "system", "user", 100); err == nil {
		t.Fatal("expected missing runtime generator error")
	}
	server.ragGen = &preferenceJSONGenerator{name: `{"ok":true}`}
	got, err := completer.CompleteJSON(context.Background(), "system", "user", 100)
	if err != nil {
		t.Fatal(err)
	}
	if got != `{"ok":true}` {
		t.Fatalf("completion=%q", got)
	}
}

func TestLifeStoryGenerationRoutesAcceptRequestKeyFromBodyAndHeader(t *testing.T) {
	tooLong := strings.Repeat("k", 129)
	tests := []struct {
		name   string
		body   string
		header string
	}{
		{name: "body", body: tooLong},
		{name: "header", header: tooLong},
		{name: "matching body and header", body: tooLong, header: tooLong},
	}

	for _, route := range []string{"generations", "revisions"} {
		for _, tt := range tests {
			t.Run(route+"/"+tt.name, func(t *testing.T) {
				body := `{}`
				if tt.body != "" {
					body = `{"requestKey":"` + tt.body + `"}`
				}
				req := httptest.NewRequest(http.MethodPost, "/api/app/life-stories/11/"+route, strings.NewReader(body))
				if tt.header != "" {
					req.Header.Set("Idempotency-Key", tt.header)
				}
				res := httptest.NewRecorder()
				server := &Server{lifeStories: lifestory.NewStore(nil)}

				server.appLifeStorySubroute(res, req, 7, 11, []string{route})

				if res.Code != http.StatusBadRequest {
					t.Fatalf("status=%d want=%d body=%s", res.Code, http.StatusBadRequest, res.Body.String())
				}
			})
		}
	}
}

func TestLifeStoryGenerationRoutesRejectConflictingRequestKeys(t *testing.T) {
	for _, route := range []string{"generations", "revisions"} {
		t.Run(route, func(t *testing.T) {
			req := httptest.NewRequest(http.MethodPost, "/api/app/life-stories/11/"+route, strings.NewReader(`{"requestKey":"body-key"}`))
			req.Header.Set("Idempotency-Key", "header-key")
			res := httptest.NewRecorder()
			server := &Server{lifeStories: lifestory.NewStore(nil)}

			server.appLifeStorySubroute(res, req, 7, 11, []string{route})

			if res.Code != http.StatusConflict {
				t.Fatalf("status=%d want=%d body=%s", res.Code, http.StatusConflict, res.Body.String())
			}
		})
	}
}

func TestLifeStoryGenerationRoutesCreateServerRequestKeyWhenMissing(t *testing.T) {
	for _, route := range []string{"generations", "revisions"} {
		t.Run(route, func(t *testing.T) {
			req := httptest.NewRequest(http.MethodPost, "/api/app/life-stories/11/"+route, strings.NewReader(`{}`))
			res := httptest.NewRecorder()
			server := &Server{lifeStories: lifestory.NewStore(nil)}

			server.appLifeStorySubroute(res, req, 7, 11, []string{route})

			if res.Code == http.StatusBadRequest {
				t.Fatalf("missing request key was not replaced by a server key: body=%s", res.Body.String())
			}
		})
	}
}
