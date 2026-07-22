package server

import (
	"bytes"
	"context"
	"database/sql"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"testing"
	"time"

	"nine-xing/nx-backend/apps/server/internal/config"
	"nine-xing/nx-backend/apps/server/internal/db"
	"nine-xing/nx-backend/apps/server/internal/testutil"
)

func TestAppPrivacyPolicyReturnsFixedText(t *testing.T) {
	s := &Server{}
	req := httptest.NewRequest(http.MethodGet, "/api/app/privacy/policy", nil)
	res := httptest.NewRecorder()

	s.appPrivacyPolicy(res, req)

	if res.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d body=%s", res.Code, res.Body.String())
	}
	var body struct {
		Code int `json:"code"`
		Data struct {
			Title       string `json:"title"`
			Content     string `json:"content"`
			EffectiveAt string `json:"effectiveAt"`
		} `json:"data"`
	}
	if err := json.Unmarshal(res.Body.Bytes(), &body); err != nil {
		t.Fatal(err)
	}
	if body.Code != 0 || body.Data.Title == "" || body.Data.Content == "" || body.Data.EffectiveAt == "" {
		t.Fatalf("expected structured privacy policy, got %+v", body)
	}
}

func TestAppPrivacyPolicyDisclosesXinzhiliVoiceDataHandling(t *testing.T) {
	s := &Server{}
	req := httptest.NewRequest(http.MethodGet, "/api/app/privacy/policy", nil)
	res := httptest.NewRecorder()

	s.appPrivacyPolicy(res, req)

	if res.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d body=%s", res.Code, res.Body.String())
	}
	var body struct {
		Data struct {
			Content string `json:"content"`
		} `json:"data"`
	}
	if err := json.Unmarshal(res.Body.Bytes(), &body); err != nil {
		t.Fatal(err)
	}

	required := []string{
		"临时处理",
		"不持久化保存原始录音",
		"转写文字",
		"AI 回答",
		"回答来源",
		"沟通偏好",
		"对话记忆",
		"页面不展示文字",
		"语音识别（ASR）",
		"语音合成（TTS）",
		"模型服务",
		"第三方",
		"停用账号",
		"撤销登录凭证",
		"匿名化",
		"解除关联",
	}
	for _, phrase := range required {
		if !strings.Contains(body.Data.Content, phrase) {
			t.Errorf("privacy policy missing %q; content=%q", phrase, body.Data.Content)
		}
	}
}

func TestAppPrivacyRoutesRequireAuth(t *testing.T) {
	s := &Server{mux: http.NewServeMux()}
	s.routes()

	tests := []struct {
		method string
		path   string
	}{
		{http.MethodPost, "/api/app/privacy/export"},
		{http.MethodDelete, "/api/app/privacy/memories"},
		{http.MethodDelete, "/api/app/privacy/account"},
	}
	for _, tt := range tests {
		res := performAppAPI(s.mux, tt.method, tt.path, "", nil)
		if res.Code != http.StatusUnauthorized {
			t.Fatalf("%s %s expected 401, got %d body=%s", tt.method, tt.path, res.Code, res.Body.String())
		}
	}
}

func TestAppPrivacyExportAndMemoryDeletionAreScopedToCurrentUser(t *testing.T) {
	handler, database := newAppAPITestServer(t)
	token, _, userID := appAPILogin(t, handler, "13800009001")
	otherToken, _, otherUserID := appAPILogin(t, handler, "13800009002")
	_ = otherToken

	cardID := insertAppAPICard(t, database, userID, "本人")
	otherCardID := insertAppAPICard(t, database, otherUserID, "其他用户")
	insertAppAPIMemory(t, database, userID, cardID, "current user memory")
	insertAppAPIMemory(t, database, otherUserID, otherCardID, "other user memory")
	insertAppAPIPreference(t, database, userID, "length", "length.detail_level", "回答简短，避免长篇大论")
	insertAppAPIPreference(t, database, otherUserID, "tone", "tone.direct", "表达直接，少说教")
	insertAppAPIChatPair(t, database, userID, cardID)

	export := performAppAPI(handler, http.MethodPost, "/api/app/privacy/export", token, nil)
	if export.Code != http.StatusOK {
		t.Fatalf("expected export 200, got %d body=%s", export.Code, export.Body.String())
	}
	var exportBody struct {
		Code int `json:"code"`
		Data struct {
			User struct {
				ID int64 `json:"id"`
			} `json:"user"`
			Cards       []any `json:"cards"`
			Memories    []any `json:"memories"`
			Preferences []struct {
				Category    string `json:"category"`
				Slot        string `json:"slot"`
				Instruction string `json:"instruction"`
				SourceText  string `json:"sourceText"`
			} `json:"preferences"`
			SessionCount int `json:"sessionCount"`
			MessageCount int `json:"messageCount"`
		} `json:"data"`
	}
	if err := json.Unmarshal(export.Body.Bytes(), &exportBody); err != nil {
		t.Fatal(err)
	}
	if exportBody.Code != 0 || exportBody.Data.User.ID != userID || len(exportBody.Data.Cards) == 0 || len(exportBody.Data.Memories) != 1 || len(exportBody.Data.Preferences) != 1 || exportBody.Data.SessionCount != 1 || exportBody.Data.MessageCount != 2 {
		t.Fatalf("unexpected export data: %+v", exportBody.Data)
	}
	if exportBody.Data.Preferences[0].Instruction != "回答简短，避免长篇大论" {
		t.Fatalf("unexpected exported preferences: %+v", exportBody.Data.Preferences)
	}

	clear := performAppAPI(handler, http.MethodDelete, "/api/app/privacy/memories", token, nil)
	if clear.Code != http.StatusOK {
		t.Fatalf("expected clear memories 200, got %d body=%s", clear.Code, clear.Body.String())
	}
	if got := countAppAPIRows(t, database, `SELECT count(*) FROM app_memories WHERE app_user_id = $1`, userID); got != 0 {
		t.Fatalf("expected current user's memories to be cleared, got %d", got)
	}
	if got := countAppAPIRows(t, database, `SELECT count(*) FROM app_memories WHERE app_user_id = $1`, otherUserID); got != 1 {
		t.Fatalf("expected other user's memories to remain, got %d", got)
	}
	if got := countAppAPIRows(t, database, `SELECT count(*) FROM app_user_preferences WHERE app_user_id = $1`, userID); got != 0 {
		t.Fatalf("expected current user's communication preferences to be cleared, got %d", got)
	}
	if got := countAppAPIRows(t, database, `SELECT count(*) FROM app_user_preferences WHERE app_user_id = $1`, otherUserID); got != 1 {
		t.Fatalf("expected other user's communication preferences to remain, got %d", got)
	}
}

func TestAppPrivacyAccountDeletionDisablesCurrentUserAndRevokesTokens(t *testing.T) {
	handler, database := newAppAPITestServer(t)
	accessToken, refreshToken, userID := appAPILogin(t, handler, "13800009003")
	_, _, otherUserID := appAPILogin(t, handler, "13800009004")
	cardID := insertAppAPICard(t, database, userID, "本人")
	otherCardID := insertAppAPICard(t, database, otherUserID, "其他用户")
	insertAppAPIMemory(t, database, userID, cardID, "current user memory")
	insertAppAPIMemory(t, database, otherUserID, otherCardID, "other user memory")
	insertAppAPIPreference(t, database, userID, "length", "length.detail_level", "回答简短，避免长篇大论")
	insertAppAPIPreference(t, database, otherUserID, "tone", "tone.direct", "表达直接，少说教")

	res := performAppAPI(handler, http.MethodDelete, "/api/app/privacy/account", accessToken, nil)
	if res.Code != http.StatusOK {
		t.Fatalf("expected account delete 200, got %d body=%s", res.Code, res.Body.String())
	}

	info := performAppAPI(handler, http.MethodGet, "/api/app/user/info", accessToken, nil)
	if info.Code != http.StatusUnauthorized {
		t.Fatalf("expected old access token to be rejected, got %d body=%s", info.Code, info.Body.String())
	}
	refresh := performAppAPI(handler, http.MethodPost, "/api/app/auth/refresh", "", map[string]string{
		"refreshToken": refreshToken,
	})
	if refresh.Code != http.StatusUnauthorized {
		t.Fatalf("expected refresh token to be revoked, got %d body=%s", refresh.Code, refresh.Body.String())
	}
	if got := countAppAPIRows(t, database, `SELECT count(*) FROM app_users WHERE id = $1 AND status = 'disabled' AND phone LIKE 'deleted-%'`, userID); got != 1 {
		t.Fatalf("expected current user to be anonymized and disabled, got %d", got)
	}
	if got := countAppAPIRows(t, database, `SELECT count(*) FROM app_memories WHERE app_user_id = $1`, userID); got != 0 {
		t.Fatalf("expected current user's memories to be removed, got %d", got)
	}
	if got := countAppAPIRows(t, database, `SELECT count(*) FROM app_user_preferences WHERE app_user_id = $1`, userID); got != 0 {
		t.Fatalf("expected current user's communication preferences to be removed, got %d", got)
	}
	if got := countAppAPIRows(t, database, `SELECT count(*) FROM app_user_preferences WHERE app_user_id = $1`, otherUserID); got != 1 {
		t.Fatalf("expected other user's communication preferences to remain, got %d", got)
	}
	if got := countAppAPIRows(t, database, `SELECT count(*) FROM app_users WHERE id = $1 AND status = 'active'`, otherUserID); got != 1 {
		t.Fatalf("expected other user to remain active, got %d", got)
	}
}

func newAppAPITestServer(t *testing.T) (http.Handler, *sql.DB) {
	t.Helper()
	dsn := os.Getenv("TEST_DATABASE_URL")
	if dsn == "" {
		t.Skip("set TEST_DATABASE_URL to run server integration tests")
	}
	if err := testutil.ValidateIsolatedPostgresDSN(dsn); err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
	defer cancel()
	database, err := db.Open(ctx, dsn, "admin", "123456")
	if err != nil {
		t.Fatalf("db open: %v", err)
	}
	t.Cleanup(func() { _ = database.Close() })
	env := config.Env{
		AdminPassword: "123456",
		AdminUsername: "admin",
		AppEnv:        "test",
		JWTSecret:     "test-secret",
		DatabaseURL:   dsn,
	}
	return New(env, database), database
}

func appAPILogin(t *testing.T, handler http.Handler, phone string) (string, string, int64) {
	t.Helper()
	sendResp := performAppAPI(handler, http.MethodPost, "/api/app/auth/send-sms", "", map[string]string{"phone": phone})
	if sendResp.Code != http.StatusOK {
		t.Fatalf("send-sms failed: %d body=%s", sendResp.Code, sendResp.Body.String())
	}
	var sendBody struct {
		Data map[string]any `json:"data"`
	}
	if err := json.Unmarshal(sendResp.Body.Bytes(), &sendBody); err != nil {
		t.Fatal(err)
	}
	devCode, _ := sendBody.Data["devCode"].(string)
	if devCode == "" {
		t.Skip("no devCode; SMS provider configured")
	}
	verifyResp := performAppAPI(handler, http.MethodPost, "/api/app/auth/verify-sms", "", map[string]any{
		"phone":      phone,
		"code":       devCode,
		"deviceInfo": "test-device",
	})
	if verifyResp.Code != http.StatusOK {
		t.Fatalf("verify-sms failed: %d body=%s", verifyResp.Code, verifyResp.Body.String())
	}
	var verifyBody struct {
		Data struct {
			AccessToken  string `json:"accessToken"`
			RefreshToken string `json:"refreshToken"`
			User         struct {
				ID int64 `json:"id"`
			} `json:"user"`
		} `json:"data"`
	}
	if err := json.Unmarshal(verifyResp.Body.Bytes(), &verifyBody); err != nil {
		t.Fatal(err)
	}
	return verifyBody.Data.AccessToken, verifyBody.Data.RefreshToken, verifyBody.Data.User.ID
}

func performAppAPI(handler http.Handler, method, path, token string, payload any) *httptest.ResponseRecorder {
	var body bytes.Buffer
	if payload != nil {
		_ = json.NewEncoder(&body).Encode(payload)
	}
	req := httptest.NewRequest(method, path, &body)
	req.Header.Set("Content-Type", "application/json")
	if token != "" {
		req.Header.Set("Authorization", "Bearer "+token)
	}
	res := httptest.NewRecorder()
	handler.ServeHTTP(res, req)
	return res
}

func insertAppAPICard(t *testing.T, database *sql.DB, userID int64, name string) int64 {
	t.Helper()
	var id int64
	if err := database.QueryRow(
		`INSERT INTO app_user_cards (app_user_id, card_type, name, relation, enneagram, wing, profile, status)
		 VALUES ($1, 'secondary', $2, 'friend', 1, 2, '{}'::jsonb, 'active')
		 RETURNING id`, userID, name).Scan(&id); err != nil {
		t.Fatalf("insert card: %v", err)
	}
	return id
}

func insertAppAPIMemory(t *testing.T, database *sql.DB, userID, cardID int64, content string) {
	t.Helper()
	if _, err := database.Exec(
		`INSERT INTO app_memories (app_user_id, card_id, content) VALUES ($1, $2, $3)`,
		userID, cardID, content); err != nil {
		t.Fatalf("insert memory: %v", err)
	}
}

func insertAppAPIPreference(t *testing.T, database *sql.DB, userID int64, category, slot, instruction string) {
	t.Helper()
	if _, err := database.Exec(
		`INSERT INTO app_user_preferences (app_user_id, category, slot, instruction, source_text)
		 VALUES ($1, $2, $3, $4, 'privacy test')`,
		userID, category, slot, instruction); err != nil {
		t.Fatalf("insert preference: %v", err)
	}
}

func insertAppAPIChatPair(t *testing.T, database *sql.DB, userID, cardID int64) {
	t.Helper()
	var sessionID int64
	if err := database.QueryRow(
		`INSERT INTO app_chat_sessions (app_user_id, card_id, title) VALUES ($1, $2, 'privacy export') RETURNING id`,
		userID, cardID).Scan(&sessionID); err != nil {
		t.Fatalf("insert session: %v", err)
	}
	if _, err := database.Exec(
		`INSERT INTO app_chat_messages (session_id, role, content) VALUES ($1, 'user', 'hello'), ($1, 'assistant', 'hi')`,
		sessionID); err != nil {
		t.Fatalf("insert messages: %v", err)
	}
}

func countAppAPIRows(t *testing.T, database *sql.DB, query string, args ...any) int {
	t.Helper()
	var count int
	if err := database.QueryRow(query, args...).Scan(&count); err != nil {
		t.Fatalf("count rows: %v", err)
	}
	return count
}
