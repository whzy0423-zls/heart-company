package server

import (
	"bytes"
	"context"
	"database/sql"
	"database/sql/driver"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log"
	"mime/multipart"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"nine-xing/nx-backend/apps/server/internal/appuser"
	"nine-xing/nx-backend/apps/server/internal/auth"
	"nine-xing/nx-backend/apps/server/internal/config"
	"nine-xing/nx-backend/apps/server/internal/db"
	"nine-xing/nx-backend/apps/server/internal/rag"
	"nine-xing/nx-backend/apps/server/internal/testutil"
	"nine-xing/nx-backend/apps/server/internal/userpreference"
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
			Version     string `json:"version"`
			Content     string `json:"content"`
			EffectiveAt string `json:"effectiveAt"`
		} `json:"data"`
	}
	if err := json.Unmarshal(res.Body.Bytes(), &body); err != nil {
		t.Fatal(err)
	}
	if body.Code != 0 || body.Data.Title == "" || body.Data.Version == "" || body.Data.EffectiveAt == "" || body.Data.Content == "" {
		t.Fatalf("expected structured privacy policy, got %+v", body)
	}
	if !bytes.Contains([]byte(body.Data.Content), []byte("学习到的沟通偏好")) {
		t.Fatalf("privacy policy must name learned communication preferences, got %q", body.Data.Content)
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
			Version     string `json:"version"`
			EffectiveAt string `json:"effectiveAt"`
			Content     string `json:"content"`
		} `json:"data"`
	}
	if err := json.Unmarshal(res.Body.Bytes(), &body); err != nil {
		t.Fatal(err)
	}
	if body.Data.Version != "2026-07-23" || body.Data.EffectiveAt != "2026-07-23" {
		t.Fatalf("privacy policy dates = version %q effective %q", body.Data.Version, body.Data.EffectiveAt)
	}

	content := normalizePrivacyPolicy(body.Data.Content)
	requiredParagraphs := []string{
		`使用芯之力语音对话时，服务会临时处理你提交的音频，用于语音识别（ASR）、生成回答和语音合成（TTS）。我们的自有后台不持久化保存原始录音。即使页面不展示文字，成功完成并保存的芯之力对话仍会将转写文字、AI回答和回答来源保存为隐藏对话历史。服务可能保存你明确表达的沟通偏好，并在回答时使用已有对话历史和已有专属记忆（如适用），以保持上下文并提供更贴合你的后续回答。`,
		`根据后台配置，完成语音识别（ASR）、语音合成（TTS）或回答生成所必需的音频、文字及上下文可能会发送给相应的第三方模型服务提供方处理。第三方的处理范围和保留规则受我们与其约定及其适用政策约束；我们不会将“页面不展示文字”等同于“不处理或不保存文字”。`,
		`注销后，主业务库中的聊天会话及消息、专属记忆、沟通偏好、兼容报告、签到记录和设备令牌会被删除；成长卡片仅标记删除，相关数据行可能为保持关联一致性而保留，但不再正常展示或使用。刷新令牌会被撤销，账号会停用并匿名化登录标识，分析记录会与账号解除关联。`,
		`订单、会员和交易记录，以及依法或履约需留存的其他记录，可能继续保留；备份和安全运维日志也可能按适用法规和实际系统配置的周期保留，期满后删除或去标识。你可以通过隐私渠道查询当前适用的保留期限。`,
	}
	for _, paragraph := range requiredParagraphs {
		if !strings.Contains(content, normalizePrivacyPolicy(paragraph)) {
			t.Errorf("privacy policy missing paragraph %q; content=%q", paragraph, body.Data.Content)
		}
	}

	prohibited := []string{
		"永久保存原始录音",
		"页面不展示所以不处理文字",
		"芯之力会创建专属记忆",
		"注销立即删除全部数据",
		"注销后全部数据立即删除",
	}
	for _, statement := range prohibited {
		if strings.Contains(content, normalizePrivacyPolicy(statement)) {
			t.Errorf("privacy policy contains prohibited overclaim %q", statement)
		}
	}
}

func normalizePrivacyPolicy(value string) string {
	return strings.Join(strings.Fields(value), "")
}

func TestAppPrivacyXinzhiliVoiceHandlerLogsExcludeConversationContent(t *testing.T) {
	const (
		filename          = "privacy-filename-canary.wav"
		transcript        = "隐私转写金丝雀"
		answer            = "隐私回答金丝雀。"
		providerErrorBody = "privacy-provider-error-body-canary"
	)
	audio := []byte("privacy-raw-audio-canary")
	sensitive := []string{filename, transcript, answer, string(audio), base64.StdEncoding.EncodeToString(audio), providerErrorBody}
	tmpDir := t.TempDir()
	t.Setenv("TMPDIR", tmpDir)

	var logs bytes.Buffer
	originalWriter, originalFlags, originalPrefix := log.Writer(), log.Flags(), log.Prefix()
	log.SetOutput(&logs)
	log.SetFlags(0)
	log.SetPrefix("")
	t.Cleanup(func() {
		log.SetOutput(originalWriter)
		log.SetFlags(originalFlags)
		log.SetPrefix(originalPrefix)
	})

	request := func() *http.Request {
		req := newPrivacyXinzhiliMultipartRequest(t, filename, audio)
		return req.WithContext(contextWithAppUser(req.Context(), auth.UserInfo{ID: 7}))
	}
	assertASRInput := func(gotAudio []byte, gotFilename string) {
		if !bytes.Equal(gotAudio, audio) || gotFilename != filename {
			t.Fatalf("ASR input filename=%q audio=%q", gotFilename, gotAudio)
		}
	}
	run := func(s *Server, req *http.Request, maxMemory int64) string {
		res := httptest.NewRecorder()
		s.appXinzhiliVoiceTurnStreamWithMultipartMemory(res, req, maxMemory)
		return res.Body.String()
	}

	success := newSuccessfulXinzhiliVoiceServer(t)
	success.xinzhiliTranscribe = func(_ context.Context, gotAudio []byte, gotFilename string) (string, error) {
		assertASRInput(gotAudio, gotFilename)
		return transcript, nil
	}
	success.ragGen = xinzhiliStreamingGeneratorFunc(func(_ context.Context, _ rag.GenerateInput, emit rag.StreamEmitter) (string, error) {
		if err := emit(answer); err != nil {
			return "", err
		}
		return answer, nil
	})
	if body := run(success, request(), xinzhiliMultipartMemory); !strings.Contains(body, "event: done") {
		t.Fatalf("success path did not complete: %s", body)
	}

	asrFailure := newSuccessfulXinzhiliVoiceServer(t)
	asrFailure.xinzhiliTranscribe = func(_ context.Context, gotAudio []byte, gotFilename string) (string, error) {
		assertASRInput(gotAudio, gotFilename)
		return "", errors.New(providerErrorBody)
	}
	if body := run(asrFailure, request(), xinzhiliMultipartMemory); !strings.Contains(body, `"code":"asr_failed"`) {
		t.Fatalf("ASR failure path did not run: %s", body)
	}

	modelFailure := newSuccessfulXinzhiliVoiceServer(t)
	modelFailure.xinzhiliTranscribe = func(_ context.Context, gotAudio []byte, gotFilename string) (string, error) {
		assertASRInput(gotAudio, gotFilename)
		return transcript, nil
	}
	modelFailure.ragGen = xinzhiliStreamingGeneratorFunc(func(_ context.Context, _ rag.GenerateInput, emit rag.StreamEmitter) (string, error) {
		if err := emit(answer); err != nil {
			return "", err
		}
		return "", errors.New(providerErrorBody)
	})
	if body := run(modelFailure, request(), xinzhiliMultipartMemory); !strings.Contains(body, `"code":"generation_failed"`) {
		t.Fatalf("model failure path did not run: %s", body)
	}

	cleanupFailure := newSuccessfulXinzhiliVoiceServer(t)
	cleanupReq := request()
	cleanupFailure.xinzhiliTranscribe = func(_ context.Context, gotAudio []byte, gotFilename string) (string, error) {
		assertASRInput(gotAudio, gotFilename)
		if cleanupReq.MultipartForm == nil {
			t.Fatal("handler did not parse multipart form before ASR")
		}
		temporaryFiles, err := filepath.Glob(filepath.Join(tmpDir, "multipart-*"))
		if err != nil || len(temporaryFiles) != 1 {
			t.Fatalf("multipart temporary files = %v, err=%v", temporaryFiles, err)
		}
		if err := os.Remove(temporaryFiles[0]); err != nil {
			t.Fatalf("remove multipart temporary file: %v", err)
		}
		if err := os.Mkdir(temporaryFiles[0], 0o700); err != nil {
			t.Fatalf("replace multipart temporary file with directory: %v", err)
		}
		if err := os.WriteFile(filepath.Join(temporaryFiles[0], "keep"), []byte("keep"), 0o600); err != nil {
			t.Fatalf("make replacement directory non-empty: %v", err)
		}
		return transcript, nil
	}
	cleanupFailure.ragGen = success.ragGen
	if body := run(cleanupFailure, cleanupReq, 1); !strings.Contains(body, "event: done") {
		t.Fatalf("cleanup failure path did not otherwise complete: %s", body)
	}

	logged := logs.String()
	if !strings.Contains(logged, "xinzhili multipart cleanup failed") {
		t.Fatalf("expected full handler cleanup warning, logs=%q", logged)
	}
	for _, value := range sensitive {
		if strings.Contains(logged, value) {
			t.Errorf("xinzhili handler logs leaked sensitive canary %q: %s", value, logged)
		}
	}
}

func newPrivacyXinzhiliMultipartRequest(t *testing.T, filename string, audio []byte) *http.Request {
	t.Helper()
	var body bytes.Buffer
	writer := multipart.NewWriter(&body)
	part, err := writer.CreateFormFile("audio", filename)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := part.Write(audio); err != nil {
		t.Fatal(err)
	}
	if err := writer.WriteField("durationMs", "1300"); err != nil {
		t.Fatal(err)
	}
	if err := writer.Close(); err != nil {
		t.Fatal(err)
	}
	req := httptest.NewRequest(http.MethodPost, "/api/app/xinzhili/turns/stream", &body)
	req.Header.Set("Content-Type", writer.FormDataContentType())
	return req
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

func TestPrivacyExportIncludesAccountWithoutPasswordHash(t *testing.T) {
	var user appuser.User
	if err := json.Unmarshal([]byte(`{
		"id": 42,
		"phone": "13800000021",
		"account": "privacy_user",
		"password_hash": "secret-hash",
		"passwordHash": "secret-hash"
	}`), &user); err != nil {
		t.Fatal(err)
	}
	payload, err := json.Marshal(appPrivacyExportResponse{User: user})
	if err != nil {
		t.Fatal(err)
	}
	var export struct {
		User map[string]any `json:"user"`
	}
	if err := json.Unmarshal(payload, &export); err != nil {
		t.Fatal(err)
	}
	if export.User["account"] != "privacy_user" {
		t.Fatalf("privacy export account = %v, want privacy_user; payload=%s", export.User["account"], payload)
	}
	for _, key := range []string{"password_hash", "passwordHash"} {
		if _, ok := export.User[key]; ok {
			t.Fatalf("privacy export exposed %q: %s", key, payload)
		}
	}
}

func TestPrivacyDeleteAccountClearsCredentials(t *testing.T) {
	recorder := &appPrivacyDeleteRecorder{}
	database := openAppPrivacyDeleteTestDB(t, recorder)
	s := &Server{db: database}
	req := httptest.NewRequest(http.MethodDelete, "/api/app/privacy/account", nil)
	req = req.WithContext(contextWithAppUser(req.Context(), auth.UserInfo{ID: 42}))
	res := httptest.NewRecorder()

	s.appPrivacyDeleteAccount(res, req)

	if res.Code != http.StatusOK {
		t.Fatalf("expected account delete 200, got %d body=%s", res.Code, res.Body.String())
	}
	var anonymizeQuery string
	for _, query := range recorder.execQueries {
		if strings.Contains(query, "UPDATE app_users") {
			anonymizeQuery = query
			break
		}
	}
	if anonymizeQuery == "" {
		t.Fatalf("account deletion did not execute app_users anonymization: %#v", recorder.execQueries)
	}
	for _, assignment := range []string{"account = NULL", "password_hash = NULL"} {
		if !strings.Contains(anonymizeQuery, assignment) {
			t.Fatalf("account anonymization missing %q in same UPDATE: %s", assignment, anonymizeQuery)
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
	const (
		currentPhone      = "13900000805"
		currentAccount    = "task8privacycurrent"
		currentDeviceInfo = "task8-privacy-current"
		otherPhone        = "13900000806"
		otherAccount      = "task8privacyother"
		otherDeviceInfo   = "task8-privacy-other"
	)
	cleanupAppPrivacyAuthFixtures(t, database,
		[]string{currentPhone, otherPhone},
		[]string{currentAccount, otherAccount},
		[]string{currentDeviceInfo, otherDeviceInfo},
	)
	t.Cleanup(func() {
		cleanupAppPrivacyAuthFixtures(t, database,
			[]string{currentPhone, otherPhone},
			[]string{currentAccount, otherAccount},
			[]string{currentDeviceInfo, otherDeviceInfo},
		)
	})

	currentSession := appAPIPasswordRegister(t, handler, currentPhone, currentAccount, "Task 8 注销用户", currentDeviceInfo)
	currentRotatedSession := appAPIRefreshPasswordSession(t, handler, currentSession.RefreshToken, "current user refresh rotation")
	otherSession := appAPIPasswordRegister(t, handler, otherPhone, otherAccount, "Task 8 其他用户", otherDeviceInfo)
	userID := currentSession.User.ID
	otherUserID := otherSession.User.ID
	cardID := insertAppAPICard(t, database, userID, "本人")
	otherCardID := insertAppAPICard(t, database, otherUserID, "其他用户")
	insertAppAPIMemory(t, database, userID, cardID, "current user memory")
	insertAppAPIMemory(t, database, otherUserID, otherCardID, "other user memory")
	insertAppAPIPreference(t, database, userID, "length", "length.detail_level", "回答简短，避免长篇大论")
	insertAppAPIPreference(t, database, otherUserID, "tone", "tone.direct", "表达直接，少说教")

	res := performAppAPI(handler, http.MethodDelete, "/api/app/privacy/account", currentRotatedSession.AccessToken, nil)
	if res.Code != http.StatusOK {
		t.Fatalf("expected account delete 200, got %d body=%s", res.Code, res.Body.String())
	}

	for label, accessToken := range map[string]string{
		"registration access token": currentSession.AccessToken,
		"rotated access token":      currentRotatedSession.AccessToken,
	} {
		info := performAppAPI(handler, http.MethodGet, "/api/app/user/info", accessToken, nil)
		if info.Code != http.StatusUnauthorized {
			t.Fatalf("expected %s to be rejected, got %d", label, info.Code)
		}
	}
	for label, refreshToken := range map[string]string{
		"registration refresh token": currentSession.RefreshToken,
		"rotated refresh token":      currentRotatedSession.RefreshToken,
	} {
		refresh := performAppAPI(handler, http.MethodPost, "/api/app/auth/refresh", "", map[string]string{
			"refreshToken": refreshToken,
		})
		if refresh.Code != http.StatusUnauthorized {
			t.Fatalf("expected %s to be rejected, got %d", label, refresh.Code)
		}
	}

	var (
		storedPhone        string
		storedAccount      sql.NullString
		storedPasswordHash sql.NullString
		storedStatus       string
	)
	if err := database.QueryRow(`SELECT phone, account, password_hash, status FROM app_users WHERE id=$1`, userID).Scan(
		&storedPhone,
		&storedAccount,
		&storedPasswordHash,
		&storedStatus,
	); err != nil {
		t.Fatalf("read deleted password user: %v", err)
	}
	if storedStatus != "disabled" || storedAccount.Valid || storedPasswordHash.Valid || !strings.HasPrefix(storedPhone, fmt.Sprintf("deleted-%d-", userID)) {
		t.Fatalf("deleted password user retained credentials or identity: status=%q accountValid=%t passwordHashValid=%t phoneAnonymized=%t", storedStatus, storedAccount.Valid, storedPasswordHash.Valid, strings.HasPrefix(storedPhone, "deleted-"))
	}
	var totalRefreshTokens, revokedRefreshTokens int
	if err := database.QueryRow(`
		SELECT count(*), count(*) FILTER (WHERE revoked)
		FROM app_refresh_tokens
		WHERE app_user_id=$1
	`, userID).Scan(&totalRefreshTokens, &revokedRefreshTokens); err != nil {
		t.Fatalf("read deleted user refresh token state: %v", err)
	}
	if totalRefreshTokens < 2 || revokedRefreshTokens != totalRefreshTokens {
		t.Fatalf("deleted user refresh tokens total=%d revoked=%d", totalRefreshTokens, revokedRefreshTokens)
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
	otherInfo := performAppAPI(handler, http.MethodGet, "/api/app/user/info", otherSession.AccessToken, nil)
	if otherInfo.Code != http.StatusOK {
		t.Fatalf("other user's access token returned status %d", otherInfo.Code)
	}
	_ = appAPIRefreshPasswordSession(t, handler, otherSession.RefreshToken, "other user refresh after account deletion")
	var (
		otherStoredPhone        string
		otherStoredAccount      sql.NullString
		otherStoredPasswordHash sql.NullString
		otherStoredStatus       string
	)
	if err := database.QueryRow(`SELECT phone, account, password_hash, status FROM app_users WHERE id=$1`, otherUserID).Scan(
		&otherStoredPhone,
		&otherStoredAccount,
		&otherStoredPasswordHash,
		&otherStoredStatus,
	); err != nil {
		t.Fatalf("read other password user: %v", err)
	}
	if otherStoredPhone != otherPhone || !otherStoredAccount.Valid || otherStoredAccount.String != otherAccount || !otherStoredPasswordHash.Valid || otherStoredPasswordHash.String == "" || otherStoredStatus != "active" {
		t.Fatalf("other password user changed during deletion: status=%q accountValid=%t passwordHashPresent=%t", otherStoredStatus, otherStoredAccount.Valid, otherStoredPasswordHash.Valid && otherStoredPasswordHash.String != "")
	}
}

func TestAppPrivacyMemoryDeletionRollsBackWhenEitherDeleteFails(t *testing.T) {
	tests := []struct {
		name  string
		phone string
		table string
	}{
		{name: "memory delete fails", phone: "13800009005", table: "app_memories"},
		{name: "preference delete fails after memory delete", phone: "13800009006", table: "app_user_preferences"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			handler, database := newAppAPITestServer(t)
			token, _, userID := appAPILogin(t, handler, tt.phone)
			cardID := insertAppAPICard(t, database, userID, "本人")
			insertAppAPIMemory(t, database, userID, cardID, "must survive rollback")
			insertAppAPIPreference(t, database, userID, "length", "length.detail_level", "回答简短一些", "短一点")
			installPrivacyDeleteFailure(t, database, tt.table, userID)

			res := performAppAPI(handler, http.MethodDelete, "/api/app/privacy/memories", token, nil)
			if res.Code != http.StatusInternalServerError {
				t.Fatalf("expected delete failure 500, got %d body=%s", res.Code, res.Body.String())
			}
			if got := countAppAPIRows(t, database, `SELECT count(*) FROM app_memories WHERE app_user_id = $1`, userID); got != 1 {
				t.Fatalf("memory delete failure partially committed, got %d memories", got)
			}
			if got := countAppAPIRows(t, database, `SELECT count(*) FROM app_user_preferences WHERE app_user_id = $1`, userID); got != 1 {
				t.Fatalf("preference delete failure partially committed, got %d preferences", got)
			}
		})
	}
}

func TestAppPrivacyAccountDeletionRollsBackWhenPreferenceCleanupFails(t *testing.T) {
	handler, database := newAppAPITestServer(t)
	const phone = "13800009007"
	cleanupAppPrivacyAuthFixtures(t, database, []string{phone}, nil, nil)
	t.Cleanup(func() {
		cleanupAppPrivacyAuthFixtures(t, database, []string{phone}, nil, nil)
	})
	token, _, userID := appAPILogin(t, handler, phone)
	cardID := insertAppAPICard(t, database, userID, "本人")
	insertAppAPIMemory(t, database, userID, cardID, "must survive rollback")
	insertAppAPIPreference(t, database, userID, "tone", "tone.direct", "直接一点", "直接说")
	installPrivacyDeleteFailure(t, database, "app_user_preferences", userID)

	res := performAppAPI(handler, http.MethodDelete, "/api/app/privacy/account", token, nil)
	if res.Code != http.StatusInternalServerError {
		t.Fatalf("expected account deletion failure 500, got %d body=%s", res.Code, res.Body.String())
	}
	if got := countAppAPIRows(t, database, `SELECT count(*) FROM app_users WHERE id = $1 AND status = 'active'`, userID); got != 1 {
		t.Fatalf("account failure must leave user active, got %d", got)
	}
	if got := countAppAPIRows(t, database, `SELECT count(*) FROM app_memories WHERE app_user_id = $1`, userID); got != 1 {
		t.Fatalf("account failure partially deleted memories, got %d", got)
	}
	if got := countAppAPIRows(t, database, `SELECT count(*) FROM app_user_preferences WHERE app_user_id = $1`, userID); got != 1 {
		t.Fatalf("account failure partially deleted preferences, got %d", got)
	}
}

func TestAppPrivacyDeletionUsesSameUserLockOrderAsPreferenceApply(t *testing.T) {
	handler, database := newAppAPITestServer(t)
	token, _, userID := appAPILogin(t, handler, "13800009008")
	cardID := insertAppAPICard(t, database, userID, "本人")
	insertAppAPIMemory(t, database, userID, cardID, "delete after concurrent preference apply")

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	advisoryKey := userID + 910000
	installPreferenceApplyBlock(t, database, userID, advisoryKey)
	lockConn, err := database.Conn(ctx)
	if err != nil {
		t.Fatalf("get advisory lock connection: %v", err)
	}
	t.Cleanup(func() { _ = lockConn.Close() })
	if _, err := lockConn.ExecContext(ctx, `SELECT pg_advisory_lock($1)`, advisoryKey); err != nil {
		t.Fatalf("acquire advisory lock: %v", err)
	}
	lockHeld := true
	t.Cleanup(func() {
		if lockHeld {
			_, _ = lockConn.ExecContext(context.Background(), `SELECT pg_advisory_unlock($1)`, advisoryKey)
		}
	})

	applyDone := make(chan error, 1)
	go func() {
		applyDone <- userpreference.NewStore(database).Apply(ctx, userID, []userpreference.Mutation{{Upsert: &userpreference.Preference{
			Category:    "length",
			Slot:        "length.detail_level",
			Instruction: "回答简短一些",
			SourceText:  "以后短一点",
		}}})
	}()
	waitForPostgresQueryBlocked(t, database, "INSERT INTO app_user_preferences")

	deleteDone := make(chan *httptest.ResponseRecorder, 1)
	go func() {
		deleteDone <- performAppAPI(handler, http.MethodDelete, "/api/app/privacy/memories", token, nil)
	}()
	waitForPostgresQueryBlocked(t, database, "SELECT id FROM app_users WHERE id = $1 FOR UPDATE")
	select {
	case res := <-deleteDone:
		t.Fatalf("privacy deletion bypassed the user lock and returned early: %d body=%s", res.Code, res.Body.String())
	default:
	}

	if _, err := lockConn.ExecContext(ctx, `SELECT pg_advisory_unlock($1)`, advisoryKey); err != nil {
		t.Fatalf("release advisory lock: %v", err)
	}
	lockHeld = false
	if err := <-applyDone; err != nil {
		t.Fatalf("preference apply failed: %v", err)
	}
	res := <-deleteDone
	if res.Code != http.StatusOK {
		t.Fatalf("privacy deletion failed after preference apply completed: %d body=%s", res.Code, res.Body.String())
	}
	if got := countAppAPIRows(t, database, `SELECT count(*) FROM app_memories WHERE app_user_id = $1`, userID); got != 0 {
		t.Fatalf("expected memories deleted after serialization, got %d", got)
	}
	if got := countAppAPIRows(t, database, `SELECT count(*) FROM app_user_preferences WHERE app_user_id = $1`, userID); got != 0 {
		t.Fatalf("concurrent apply escaped privacy deletion, got %d preferences", got)
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

type appAPIPasswordSession struct {
	AccessToken  string
	RefreshToken string
	User         appuser.User
}

func appAPIPasswordRegister(t *testing.T, handler http.Handler, phone, account, nickname, deviceInfo string) appAPIPasswordSession {
	t.Helper()
	const fixturePassword = "fixture-secret-8"
	sendResponse := performAppAPI(handler, http.MethodPost, "/api/app/auth/send-sms", "", map[string]string{"phone": phone})
	if sendResponse.Code != http.StatusOK {
		t.Fatalf("password registration SMS returned status %d", sendResponse.Code)
	}
	var sendBody struct {
		Code int `json:"code"`
		Data struct {
			DevCode string `json:"devCode"`
		} `json:"data"`
	}
	if err := json.Unmarshal(sendResponse.Body.Bytes(), &sendBody); err != nil {
		t.Fatalf("decode password registration SMS response: %v", err)
	}
	if sendBody.Code != 0 || sendBody.Data.DevCode == "" {
		t.Fatal("password registration SMS response did not include a test code")
	}

	registrationResponse := performAppAPI(handler, http.MethodPost, "/api/app/auth/register", "", map[string]string{
		"nickname":   nickname,
		"account":    account,
		"password":   fixturePassword,
		"phone":      phone,
		"code":       sendBody.Data.DevCode,
		"deviceInfo": deviceInfo,
	})
	if registrationResponse.Code != http.StatusOK {
		t.Fatalf("password registration returned status %d", registrationResponse.Code)
	}
	var registrationBody struct {
		Code int `json:"code"`
		Data struct {
			AccessToken  string       `json:"accessToken"`
			RefreshToken string       `json:"refreshToken"`
			User         appuser.User `json:"user"`
		} `json:"data"`
		Error   any    `json:"error"`
		Message string `json:"message"`
	}
	if err := json.Unmarshal(registrationResponse.Body.Bytes(), &registrationBody); err != nil {
		t.Fatalf("decode password registration response: %v", err)
	}
	if registrationBody.Code != 0 || registrationBody.Message != "ok" || registrationBody.Error != nil || registrationBody.Data.AccessToken == "" || registrationBody.Data.RefreshToken == "" || registrationBody.Data.User.ID <= 0 {
		t.Fatal("password registration response does not match the Flutter session shape")
	}
	if registrationBody.Data.User.Phone != phone || registrationBody.Data.User.Account != account || registrationBody.Data.User.Nickname != nickname {
		t.Fatalf("password registration returned unexpected user identity: id=%d account=%q nickname=%q", registrationBody.Data.User.ID, registrationBody.Data.User.Account, registrationBody.Data.User.Nickname)
	}
	return appAPIPasswordSession{
		AccessToken:  registrationBody.Data.AccessToken,
		RefreshToken: registrationBody.Data.RefreshToken,
		User:         registrationBody.Data.User,
	}
}

func appAPIRefreshPasswordSession(t *testing.T, handler http.Handler, refreshToken, operation string) appAPIPasswordSession {
	t.Helper()
	response := performAppAPI(handler, http.MethodPost, "/api/app/auth/refresh", "", map[string]string{
		"refreshToken": refreshToken,
	})
	if response.Code != http.StatusOK {
		t.Fatalf("%s returned status %d", operation, response.Code)
	}
	var body struct {
		Code int `json:"code"`
		Data struct {
			AccessToken  string `json:"accessToken"`
			RefreshToken string `json:"refreshToken"`
		} `json:"data"`
		Message string `json:"message"`
	}
	if err := json.Unmarshal(response.Body.Bytes(), &body); err != nil {
		t.Fatalf("decode %s response: %v", operation, err)
	}
	if body.Code != 0 || body.Message != "ok" || body.Data.AccessToken == "" || body.Data.RefreshToken == "" {
		t.Fatalf("%s does not match the Flutter token shape", operation)
	}
	if body.Data.RefreshToken == refreshToken {
		t.Fatalf("%s did not rotate the refresh token", operation)
	}
	return appAPIPasswordSession{
		AccessToken:  body.Data.AccessToken,
		RefreshToken: body.Data.RefreshToken,
	}
}

func cleanupAppPrivacyAuthFixtures(t *testing.T, database *sql.DB, phones, accounts, deviceInfos []string) {
	t.Helper()
	for _, deviceInfo := range deviceInfos {
		if _, err := database.Exec(`
			DELETE FROM app_users
			WHERE id IN (
				SELECT app_user_id FROM app_refresh_tokens WHERE device_info=$1
			)
		`, deviceInfo); err != nil {
			t.Fatalf("clear privacy password fixture user by device: %v", err)
		}
	}
	for _, account := range accounts {
		if _, err := database.Exec(`DELETE FROM app_users WHERE lower(account)=lower($1)`, account); err != nil {
			t.Fatalf("clear privacy password fixture user by account: %v", err)
		}
	}
	for _, phone := range phones {
		if _, err := database.Exec(`DELETE FROM app_sms_codes WHERE phone=$1`, phone); err != nil {
			t.Fatalf("clear privacy password fixture SMS codes: %v", err)
		}
		if _, err := database.Exec(`DELETE FROM app_users WHERE phone=$1`, phone); err != nil {
			t.Fatalf("clear privacy password fixture user by phone: %v", err)
		}
	}
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

func insertAppAPIPreference(t *testing.T, database *sql.DB, userID int64, category, slot, instruction string, sourceTexts ...string) {
	sourceText := "privacy test"
	if len(sourceTexts) > 0 {
		sourceText = sourceTexts[0]
	}
	t.Helper()
	if _, err := database.Exec(
		`INSERT INTO app_user_preferences (app_user_id, category, slot, instruction, source_text)
		 VALUES ($1, $2, $3, $4, $5)`,
		userID, category, slot, instruction, sourceText); err != nil {
		t.Fatalf("insert preference: %v", err)
	}
}

func installPrivacyDeleteFailure(t *testing.T, database *sql.DB, table string, userID int64) {
	t.Helper()
	if table != "app_memories" && table != "app_user_preferences" {
		t.Fatalf("unsupported failure table %q", table)
	}
	functionName := "privacy_fail_" + table + "_delete"
	triggerName := functionName + "_trigger"
	functionSQL := fmt.Sprintf(`
		CREATE OR REPLACE FUNCTION %s() RETURNS trigger LANGUAGE plpgsql AS $$
		BEGIN
			IF OLD.app_user_id = %d THEN
				RAISE EXCEPTION 'injected privacy delete failure';
			END IF;
			RETURN OLD;
		END
		$$`, functionName, userID)
	if _, err := database.Exec(functionSQL); err != nil {
		t.Fatalf("create failure function: %v", err)
	}
	if _, err := database.Exec(fmt.Sprintf(
		`CREATE TRIGGER %s BEFORE DELETE ON %s FOR EACH ROW EXECUTE FUNCTION %s()`,
		triggerName, table, functionName,
	)); err != nil {
		t.Fatalf("create failure trigger: %v", err)
	}
	t.Cleanup(func() {
		_, _ = database.Exec(fmt.Sprintf(`DROP TRIGGER IF EXISTS %s ON %s`, triggerName, table))
		_, _ = database.Exec(fmt.Sprintf(`DROP FUNCTION IF EXISTS %s()`, functionName))
	})
}

func installPreferenceApplyBlock(t *testing.T, database *sql.DB, userID, advisoryKey int64) {
	t.Helper()
	const functionName = "privacy_block_preference_apply"
	const triggerName = "privacy_block_preference_apply_trigger"
	functionSQL := fmt.Sprintf(`
		CREATE OR REPLACE FUNCTION %s() RETURNS trigger LANGUAGE plpgsql AS $$
		BEGIN
			IF NEW.app_user_id = %d THEN
				PERFORM pg_advisory_xact_lock(%d);
			END IF;
			RETURN NEW;
		END
		$$`, functionName, userID, advisoryKey)
	if _, err := database.Exec(functionSQL); err != nil {
		t.Fatalf("create preference block function: %v", err)
	}
	if _, err := database.Exec(fmt.Sprintf(
		`CREATE TRIGGER %s BEFORE INSERT OR UPDATE ON app_user_preferences FOR EACH ROW EXECUTE FUNCTION %s()`,
		triggerName, functionName,
	)); err != nil {
		t.Fatalf("create preference block trigger: %v", err)
	}
	t.Cleanup(func() {
		_, _ = database.Exec(`DROP TRIGGER IF EXISTS privacy_block_preference_apply_trigger ON app_user_preferences`)
		_, _ = database.Exec(`DROP FUNCTION IF EXISTS privacy_block_preference_apply()`)
	})
}

func waitForPostgresQueryBlocked(t *testing.T, database *sql.DB, queryFragment string) {
	t.Helper()
	deadline := time.Now().Add(3 * time.Second)
	for time.Now().Before(deadline) {
		var blocked bool
		if err := database.QueryRow(
			`SELECT EXISTS (
				SELECT 1
				  FROM pg_stat_activity
				 WHERE datname = current_database()
				   AND pid <> pg_backend_pid()
				   AND state = 'active'
				   AND wait_event_type = 'Lock'
				   AND position($1 in query) > 0
			)`, queryFragment,
		).Scan(&blocked); err != nil {
			t.Fatalf("inspect postgres activity: %v", err)
		}
		if blocked {
			return
		}
		time.Sleep(10 * time.Millisecond)
	}
	t.Fatalf("timed out waiting for blocked PostgreSQL query containing %q", queryFragment)
}

func countAppAPIRows(t *testing.T, database *sql.DB, query string, args ...any) int {
	t.Helper()
	var count int
	if err := database.QueryRow(query, args...).Scan(&count); err != nil {
		t.Fatalf("count rows: %v", err)
	}
	return count
}

var appPrivacyDeleteDriverSeq atomic.Int64

type appPrivacyDeleteRecorder struct {
	execQueries []string
}

func openAppPrivacyDeleteTestDB(t *testing.T, recorder *appPrivacyDeleteRecorder) *sql.DB {
	t.Helper()
	driverName := fmt.Sprintf("app_privacy_delete_test_%d", appPrivacyDeleteDriverSeq.Add(1))
	sql.Register(driverName, appPrivacyDeleteDriver{recorder: recorder})
	database, err := sql.Open(driverName, "")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = database.Close() })
	return database
}

type appPrivacyDeleteDriver struct {
	recorder *appPrivacyDeleteRecorder
}

func (d appPrivacyDeleteDriver) Open(string) (driver.Conn, error) {
	return &appPrivacyDeleteConn{recorder: d.recorder}, nil
}

type appPrivacyDeleteConn struct {
	recorder *appPrivacyDeleteRecorder
}

func (*appPrivacyDeleteConn) Prepare(string) (driver.Stmt, error) { return nil, driver.ErrSkip }
func (*appPrivacyDeleteConn) Close() error                        { return nil }
func (*appPrivacyDeleteConn) Begin() (driver.Tx, error)           { return appPrivacyDeleteTx{}, nil }

func (*appPrivacyDeleteConn) BeginTx(context.Context, driver.TxOptions) (driver.Tx, error) {
	return appPrivacyDeleteTx{}, nil
}

func (*appPrivacyDeleteConn) QueryContext(_ context.Context, query string, _ []driver.NamedValue) (driver.Rows, error) {
	if !strings.Contains(query, "SELECT id FROM app_users") {
		return nil, fmt.Errorf("unexpected privacy delete query: %s", query)
	}
	return &appPrivacyDeleteRows{}, nil
}

func (c *appPrivacyDeleteConn) ExecContext(_ context.Context, query string, _ []driver.NamedValue) (driver.Result, error) {
	c.recorder.execQueries = append(c.recorder.execQueries, query)
	return driver.RowsAffected(1), nil
}

type appPrivacyDeleteTx struct{}

func (appPrivacyDeleteTx) Commit() error   { return nil }
func (appPrivacyDeleteTx) Rollback() error { return nil }

type appPrivacyDeleteRows struct {
	done bool
}

func (*appPrivacyDeleteRows) Columns() []string { return []string{"id"} }
func (*appPrivacyDeleteRows) Close() error      { return nil }

func (r *appPrivacyDeleteRows) Next(dest []driver.Value) error {
	if r.done {
		return io.EOF
	}
	dest[0] = int64(42)
	r.done = true
	return nil
}

var _ driver.ConnBeginTx = (*appPrivacyDeleteConn)(nil)
var _ driver.ExecerContext = (*appPrivacyDeleteConn)(nil)
var _ driver.QueryerContext = (*appPrivacyDeleteConn)(nil)
