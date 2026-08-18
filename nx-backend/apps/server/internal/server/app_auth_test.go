package server_test

import (
	"crypto/hmac"
	"crypto/sha256"
	"database/sql"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"sync"
	"testing"
	"time"

	"nine-xing/nx-backend/apps/server/internal/appuser"
	"nine-xing/nx-backend/apps/server/internal/testutil"
)

func TestAppAuthSendSMS(t *testing.T) {
	handler, _ := newTestServer(t)

	t.Run("rejects short phone", func(t *testing.T) {
		r := perform(handler, http.MethodPost, "/api/app/auth/send-sms", "", map[string]string{"phone": "1380000"})
		if r.Code != http.StatusBadRequest {
			t.Fatalf("expected 400, got %d", r.Code)
		}
	})

	t.Run("returns devCode in dev mode", func(t *testing.T) {
		r := perform(handler, http.MethodPost, "/api/app/auth/send-sms", "", map[string]string{"phone": "13800000001"})
		if r.Code != http.StatusOK {
			t.Fatalf("expected 200, got %d body=%s", r.Code, r.Body.String())
		}
		var resp struct {
			Code int            `json:"code"`
			Data map[string]any `json:"data"`
		}
		if err := json.Unmarshal(r.Body.Bytes(), &resp); err != nil {
			t.Fatal(err)
		}
		if fmt.Sprint(resp.Data["devCode"]) == "" {
			t.Fatal("expected devCode in dev mode")
		}
	})
}

func TestAppAuthVerifySMS(t *testing.T) {
	handler, _ := newTestServer(t)

	phone := "13800000002"

	// send code
	sendResp := perform(handler, http.MethodPost, "/api/app/auth/send-sms", "", map[string]string{"phone": phone})
	if sendResp.Code != http.StatusOK {
		t.Fatalf("send-sms failed: %d %s", sendResp.Code, sendResp.Body.String())
	}
	var sendBody struct {
		Data map[string]any `json:"data"`
	}
	_ = json.Unmarshal(sendResp.Body.Bytes(), &sendBody)
	devCode, _ := sendBody.Data["devCode"].(string)
	if devCode == "" {
		t.Skip("no devCode — SMS provider configured, skipping")
	}

	t.Run("wrong code returns 401", func(t *testing.T) {
		r := perform(handler, http.MethodPost, "/api/app/auth/verify-sms", "", map[string]any{
			"phone": phone,
			"code":  "000000",
		})
		if r.Code != http.StatusUnauthorized {
			t.Fatalf("expected 401, got %d", r.Code)
		}
	})

	t.Run("correct code returns tokens and user", func(t *testing.T) {
		r := perform(handler, http.MethodPost, "/api/app/auth/verify-sms", "", map[string]any{
			"phone":      phone,
			"code":       devCode,
			"deviceInfo": "test-device",
		})
		if r.Code != http.StatusOK {
			t.Fatalf("expected 200, got %d %s", r.Code, r.Body.String())
		}
		var resp struct {
			Code int `json:"code"`
			Data struct {
				AccessToken  string       `json:"accessToken"`
				RefreshToken string       `json:"refreshToken"`
				User         appuser.User `json:"user"`
			} `json:"data"`
		}
		if err := json.Unmarshal(r.Body.Bytes(), &resp); err != nil {
			t.Fatal(err)
		}
		if resp.Data.AccessToken == "" {
			t.Fatal("expected accessToken")
		}
		if resp.Data.RefreshToken == "" {
			t.Fatal("expected refreshToken")
		}
		if resp.Data.User.Phone != phone {
			t.Fatalf("expected phone %s, got %s", phone, resp.Data.User.Phone)
		}
	})

	t.Run("code cannot be reused", func(t *testing.T) {
		r := perform(handler, http.MethodPost, "/api/app/auth/verify-sms", "", map[string]any{
			"phone": phone,
			"code":  devCode,
		})
		if r.Code != http.StatusUnauthorized {
			t.Fatalf("expected 401 on reuse, got %d", r.Code)
		}
	})
}

func TestAppPasswordRegistrationAndLogin(t *testing.T) {
	handler, _ := newTestServer(t)
	database := openAppAuthContractDatabase(t)
	const (
		phone          = "13900000801"
		account        = "task8primary"
		unknownAccount = "task8unknown"
		nickname       = "Task 8 用户"
		password       = "fixture-secret-8"
	)
	cleanupAppAuthContractFixtures(t, database, []string{phone}, []string{account, unknownAccount})
	t.Cleanup(func() {
		cleanupAppAuthContractFixtures(t, database, []string{phone}, []string{account, unknownAccount})
	})

	registrationCode := requestAppDevCode(t, handler, phone)
	registrationResponse := perform(handler, http.MethodPost, "/api/app/auth/register", "", map[string]string{
		"nickname":   nickname,
		"account":    account,
		"password":   password,
		"phone":      phone,
		"code":       registrationCode,
		"deviceInfo": "task8-registration",
	})
	registration := decodeAppPasswordSessionResponse(t, registrationResponse, "password registration")
	if registration.Data.User.ID <= 0 {
		t.Fatal("password registration returned an invalid user ID")
	}
	if registration.Data.User.Phone != phone || registration.Data.User.Account != account || registration.Data.User.Nickname != nickname {
		t.Fatalf("password registration returned unexpected user identity: id=%d account=%q nickname=%q", registration.Data.User.ID, registration.Data.User.Account, registration.Data.User.Nickname)
	}

	accountLoginResponse := perform(handler, http.MethodPost, "/api/app/auth/login", "", map[string]string{
		"account":    strings.ToUpper(account),
		"password":   password,
		"deviceInfo": "task8-account-login",
	})
	accountLogin := decodeAppPasswordSessionResponse(t, accountLoginResponse, "case-insensitive account login")
	if accountLogin.Data.User.ID != registration.Data.User.ID {
		t.Fatalf("account login user ID=%d want=%d", accountLogin.Data.User.ID, registration.Data.User.ID)
	}

	phoneLoginResponse := perform(handler, http.MethodPost, "/api/app/auth/login", "", map[string]string{
		"account":    phone,
		"password":   password,
		"deviceInfo": "task8-phone-login",
	})
	phoneLogin := decodeAppPasswordSessionResponse(t, phoneLoginResponse, "phone password login")
	if phoneLogin.Data.User.ID != registration.Data.User.ID {
		t.Fatalf("phone login user ID=%d want=%d", phoneLogin.Data.User.ID, registration.Data.User.ID)
	}

	wrongPasswordResponse := perform(handler, http.MethodPost, "/api/app/auth/login", "", map[string]string{
		"account":  account,
		"password": "fixture-wrong-password",
	})
	wrongPasswordEnvelope := assertAppPasswordInvalidCredentialsResponse(t, wrongPasswordResponse, "wrong password")
	unknownAccountResponse := perform(handler, http.MethodPost, "/api/app/auth/login", "", map[string]string{
		"account":  unknownAccount,
		"password": password,
	})
	unknownAccountEnvelope := assertAppPasswordInvalidCredentialsResponse(t, unknownAccountResponse, "unknown account")
	if wrongPasswordEnvelope != unknownAccountEnvelope {
		t.Fatalf("wrong password and unknown account envelopes differ: wrong=%+v unknown=%+v", wrongPasswordEnvelope, unknownAccountEnvelope)
	}

	refreshResponse := perform(handler, http.MethodPost, "/api/app/auth/refresh", "", map[string]string{
		"refreshToken": accountLogin.Data.RefreshToken,
	})
	if refreshResponse.Code != http.StatusOK {
		t.Fatalf("password login refresh returned status %d", refreshResponse.Code)
	}
	var refreshBody struct {
		Code int `json:"code"`
		Data struct {
			AccessToken  string `json:"accessToken"`
			RefreshToken string `json:"refreshToken"`
		} `json:"data"`
		Message string `json:"message"`
	}
	if err := json.Unmarshal(refreshResponse.Body.Bytes(), &refreshBody); err != nil {
		t.Fatalf("decode password login refresh response: %v", err)
	}
	if refreshBody.Code != 0 || refreshBody.Message != "ok" || refreshBody.Data.AccessToken == "" || refreshBody.Data.RefreshToken == "" {
		t.Fatal("password login refresh response does not match the Flutter token shape")
	}
	if refreshBody.Data.RefreshToken == accountLogin.Data.RefreshToken {
		t.Fatal("password login refresh token was not rotated")
	}
	oldRefreshResponse := perform(handler, http.MethodPost, "/api/app/auth/refresh", "", map[string]string{
		"refreshToken": accountLogin.Data.RefreshToken,
	})
	if oldRefreshResponse.Code != http.StatusUnauthorized {
		t.Fatalf("rotated refresh token returned status %d, want 401", oldRefreshResponse.Code)
	}
}

func TestAppPasswordRecovery(t *testing.T) {
	handler, _ := newTestServer(t)
	database := openAppAuthContractDatabase(t)
	const (
		phone        = "13900000831"
		unknownPhone = "13900000832"
		smsOnlyPhone = "13900000833"
		account      = "task8recovery"
		oldPassword  = "old-recovery-secret"
		newPassword  = "new-recovery-secret"
	)
	phones := []string{phone, unknownPhone, smsOnlyPhone}
	cleanupAppAuthContractFixtures(t, database, phones, []string{account})
	t.Cleanup(func() { cleanupAppAuthContractFixtures(t, database, phones, []string{account}) })

	registrationCode := "831246"
	if _, err := database.Exec(`
		INSERT INTO app_sms_codes (phone, code_hash, expires_at)
		VALUES ($1, $2, now() + interval '10 minutes')
	`, phone, appuser.HashToken(registrationCode)); err != nil {
		t.Fatalf("seed recovery registration code: %v", err)
	}
	registration := decodeAppPasswordSessionResponse(t, perform(handler, http.MethodPost, "/api/app/auth/register", "", map[string]string{
		"nickname": "Recovery User", "account": account, "password": oldPassword, "phone": phone, "code": registrationCode,
	}), "recovery fixture registration")
	if _, err := database.Exec(`INSERT INTO app_users (phone) VALUES ($1)`, smsOnlyPhone); err != nil {
		t.Fatal(err)
	}

	for _, candidate := range []string{unknownPhone, smsOnlyPhone} {
		response := perform(handler, http.MethodPost, "/api/app/auth/send-sms", "", map[string]string{
			"phone": candidate, "purpose": "password_reset",
		})
		if response.Code != http.StatusBadRequest {
			t.Fatalf("password reset request for %s returned %d body=%s", candidate, response.Code, response.Body.String())
		}
		var body struct {
			Code    int    `json:"code"`
			Data    any    `json:"data"`
			Message string `json:"message"`
		}
		if err := json.Unmarshal(response.Body.Bytes(), &body); err != nil {
			t.Fatal(err)
		}
		if body.Code != -1 || body.Message != "该手机号尚未注册，请先注册账号" || body.Data != nil {
			t.Fatalf("ineligible reset response did not explain registration requirement: %s", response.Body.String())
		}
		if rows := countAppAuthContractRows(t, database, `SELECT count(*) FROM app_password_reset_codes WHERE phone=$1`, candidate); rows != 0 {
			t.Fatalf("ineligible phone %s stored %d reset codes", candidate, rows)
		}
	}

	resetSend := perform(handler, http.MethodPost, "/api/app/auth/send-sms", "", map[string]string{
		"phone": phone, "purpose": "password_reset",
	})
	if resetSend.Code != http.StatusOK {
		t.Fatalf("eligible reset request returned %d body=%s", resetSend.Code, resetSend.Body.String())
	}
	var resetSendBody struct {
		Data map[string]any `json:"data"`
	}
	if err := json.Unmarshal(resetSend.Body.Bytes(), &resetSendBody); err != nil {
		t.Fatal(err)
	}
	resetCode, _ := resetSendBody.Data["devCode"].(string)
	if resetCode == "" {
		t.Fatal("eligible development reset did not return devCode")
	}

	resetResponse := perform(handler, http.MethodPost, "/api/app/auth/reset-password", "", map[string]string{
		"phone": phone, "code": resetCode, "password": newPassword,
	})
	if resetResponse.Code != http.StatusOK {
		t.Fatalf("reset password returned %d body=%s", resetResponse.Code, resetResponse.Body.String())
	}
	if refresh := perform(handler, http.MethodPost, "/api/app/auth/refresh", "", map[string]string{
		"refreshToken": registration.Data.RefreshToken,
	}); refresh.Code != http.StatusUnauthorized {
		t.Fatalf("pre-reset refresh token returned %d want 401", refresh.Code)
	}
	assertAppPasswordInvalidCredentialsResponse(t, perform(handler, http.MethodPost, "/api/app/auth/login", "", map[string]string{
		"account": account, "password": oldPassword,
	}), "old password after reset")
	decodeAppPasswordSessionResponse(t, perform(handler, http.MethodPost, "/api/app/auth/login", "", map[string]string{
		"account": account, "password": newPassword,
	}), "new password after reset")
	if reuse := perform(handler, http.MethodPost, "/api/app/auth/reset-password", "", map[string]string{
		"phone": phone, "code": resetCode, "password": "another-secret",
	}); reuse.Code != http.StatusUnauthorized {
		t.Fatalf("reused reset code returned %d want 401", reuse.Code)
	}
}

func TestAppPasswordInvalidCredentialsEnvelopeComparisonIgnoresFormattingAndExtraFields(t *testing.T) {
	wrongPasswordResponse := httptest.NewRecorder()
	wrongPasswordResponse.Code = http.StatusUnauthorized
	wrongPasswordResponse.Body.WriteString(`{"code":-1,"data":null,"error":"账号或密码错误","message":"账号或密码错误","traceId":"fixture-a"}`)
	unknownAccountResponse := httptest.NewRecorder()
	unknownAccountResponse.Code = http.StatusUnauthorized
	unknownAccountResponse.Body.WriteString("{\n  \"message\": \"账号或密码错误\",\n  \"error\": \"账号或密码错误\",\n  \"data\": null,\n  \"code\": -1,\n  \"metadata\": {\"fixture\": true}\n}\n")

	wrongPasswordEnvelope := assertAppPasswordInvalidCredentialsResponse(t, wrongPasswordResponse, "wrong password fixture")
	unknownAccountEnvelope := assertAppPasswordInvalidCredentialsResponse(t, unknownAccountResponse, "unknown account fixture")
	if wrongPasswordEnvelope != unknownAccountEnvelope {
		t.Fatalf("stable invalid-credentials envelopes differ: wrong=%+v unknown=%+v", wrongPasswordEnvelope, unknownAccountEnvelope)
	}
}

func TestAppPasswordRegistrationBindsLegacySMSUser(t *testing.T) {
	legacyHandler, _ := newTestServer(t)
	database := openAppAuthContractDatabase(t)
	const (
		phone    = "13900000802"
		account  = "task8legacy"
		nickname = "Task 8 旧用户"
		password = "fixture-secret-8"
	)
	cleanupAppAuthContractFixtures(t, database, []string{phone}, []string{account})
	t.Cleanup(func() {
		cleanupAppAuthContractFixtures(t, database, []string{phone}, []string{account})
	})

	legacyCode := requestAppDevCode(t, legacyHandler, phone)
	legacyLoginResponse := perform(legacyHandler, http.MethodPost, "/api/app/auth/verify-sms", "", map[string]string{
		"phone":      phone,
		"code":       legacyCode,
		"deviceInfo": "task8-legacy-sms-login",
	})
	legacyLogin := decodeAppPasswordSessionResponse(t, legacyLoginResponse, "legacy SMS login")
	if legacyLogin.Data.User.ID <= 0 {
		t.Fatal("legacy SMS login returned an invalid user ID")
	}

	registrationHandler, _ := newTestServer(t)
	registrationCode := requestAppDevCode(t, registrationHandler, phone)
	registrationResponse := perform(registrationHandler, http.MethodPost, "/api/app/auth/register", "", map[string]string{
		"nickname":   nickname,
		"account":    account,
		"password":   password,
		"phone":      phone,
		"code":       registrationCode,
		"deviceInfo": "task8-legacy-password-bind",
	})
	registration := decodeAppPasswordSessionResponse(t, registrationResponse, "legacy user password binding")
	if registration.Data.User.ID != legacyLogin.Data.User.ID {
		t.Fatalf("legacy user ID changed from %d to %d", legacyLogin.Data.User.ID, registration.Data.User.ID)
	}
	if registration.Data.User.Account != account || registration.Data.User.Phone != phone || registration.Data.User.Nickname != nickname {
		t.Fatalf("legacy user binding returned unexpected identity: id=%d account=%q nickname=%q", registration.Data.User.ID, registration.Data.User.Account, registration.Data.User.Nickname)
	}
	if got := countAppAuthContractRows(t, database, `SELECT count(*) FROM app_users WHERE id=$1 AND phone=$2 AND account=$3 AND password_hash IS NOT NULL`, legacyLogin.Data.User.ID, phone, account); got != 1 {
		t.Fatalf("legacy user credentials were not bound in place, matching rows=%d", got)
	}
}

func TestAppPasswordRegistrationDuplicateAccountRollsBackSMSCode(t *testing.T) {
	handler, _ := newTestServer(t)
	database := openAppAuthContractDatabase(t)
	const (
		ownerPhone       = "13900000803"
		candidatePhone   = "13900000804"
		occupiedAccount  = "task8duplicate"
		availableAccount = "task8available"
		password         = "fixture-secret-8"
	)
	cleanupAppAuthContractFixtures(t, database, []string{ownerPhone, candidatePhone}, []string{occupiedAccount, availableAccount})
	t.Cleanup(func() {
		cleanupAppAuthContractFixtures(t, database, []string{ownerPhone, candidatePhone}, []string{occupiedAccount, availableAccount})
	})

	ownerCode := requestAppDevCode(t, handler, ownerPhone)
	ownerResponse := perform(handler, http.MethodPost, "/api/app/auth/register", "", map[string]string{
		"nickname": "Task 8 占用账号",
		"account":  occupiedAccount,
		"password": password,
		"phone":    ownerPhone,
		"code":     ownerCode,
	})
	_ = decodeAppPasswordSessionResponse(t, ownerResponse, "duplicate account owner registration")

	candidateCode := requestAppDevCode(t, handler, candidatePhone)
	duplicateResponse := perform(handler, http.MethodPost, "/api/app/auth/register", "", map[string]string{
		"nickname": "Task 8 候选用户",
		"account":  occupiedAccount,
		"password": password,
		"phone":    candidatePhone,
		"code":     candidateCode,
	})
	if duplicateResponse.Code != http.StatusConflict {
		t.Fatalf("duplicate account registration returned status %d, want 409", duplicateResponse.Code)
	}
	var duplicateBody struct {
		Code    int    `json:"code"`
		Message string `json:"message"`
		Error   string `json:"error"`
	}
	if err := json.Unmarshal(duplicateResponse.Body.Bytes(), &duplicateBody); err != nil {
		t.Fatalf("decode duplicate account response: %v", err)
	}
	if duplicateBody.Code != -1 || duplicateBody.Message != "用户名已存在" || duplicateBody.Error != "用户名已存在" {
		t.Fatal("duplicate account response did not use the expected conflict message")
	}
	if got := countAppAuthContractRows(t, database, `SELECT count(*) FROM app_users WHERE phone=$1`, candidatePhone); got != 0 {
		t.Fatalf("duplicate account attempt partially created %d user rows", got)
	}
	var codeUsed bool
	if err := database.QueryRow(`SELECT used FROM app_sms_codes WHERE phone=$1 ORDER BY create_time DESC, id DESC LIMIT 1`, candidatePhone).Scan(&codeUsed); err != nil {
		t.Fatalf("read candidate SMS code state: %v", err)
	}
	if codeUsed {
		t.Fatal("duplicate account attempt consumed the SMS code")
	}

	retryResponse := perform(handler, http.MethodPost, "/api/app/auth/register", "", map[string]string{
		"nickname": "Task 8 候选用户",
		"account":  availableAccount,
		"password": password,
		"phone":    candidatePhone,
		"code":     candidateCode,
	})
	retry := decodeAppPasswordSessionResponse(t, retryResponse, "duplicate account retry")
	if retry.Data.User.Phone != candidatePhone || retry.Data.User.Account != availableAccount {
		t.Fatalf("duplicate account retry returned unexpected identity: id=%d account=%q", retry.Data.User.ID, retry.Data.User.Account)
	}
}

func TestAppPasswordSessionPersistenceFailureResponses(t *testing.T) {
	handler, _ := newTestServer(t)
	dsn := os.Getenv("TEST_DATABASE_URL")
	if err := testutil.ValidateIsolatedPostgresDSN(dsn); err != nil {
		t.Fatal(err)
	}
	database, err := sql.Open("pgx", dsn)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = database.Close() })

	const (
		deviceInfo        = "task6-refresh-token-failure"
		registrationPhone = "13900000601"
		smsLoginPhone     = "13900000602"
		account           = "task6sessionuser"
		password          = "secret1"
	)
	cleanupAppPasswordSessionFailureFixtures(t, database, registrationPhone, smsLoginPhone, account)
	t.Cleanup(func() {
		cleanupAppPasswordSessionFailureFixtures(t, database, registrationPhone, smsLoginPhone, account)
	})
	installAppRefreshTokenFailureTrigger(t, database, deviceInfo)

	registrationCode := requestAppDevCode(t, handler, registrationPhone)
	registration := perform(handler, http.MethodPost, "/api/app/auth/register", "", map[string]string{
		"nickname":   "心之力用户",
		"account":    account,
		"password":   password,
		"phone":      registrationPhone,
		"code":       registrationCode,
		"deviceInfo": deviceInfo,
	})
	if registration.Code != http.StatusInternalServerError {
		t.Fatalf("expected committed registration with refresh-token failure to return 500, got %d body=%s", registration.Code, registration.Body.String())
	}
	if !strings.Contains(registration.Body.String(), "注册已成功，请使用账号密码登录") {
		t.Fatalf("expected committed registration recovery message, got body=%s", registration.Body.String())
	}

	var (
		storedAccount      string
		storedPasswordHash string
	)
	if err := database.QueryRow(`SELECT account, password_hash FROM app_users WHERE phone=$1`, registrationPhone).Scan(&storedAccount, &storedPasswordHash); err != nil {
		t.Fatalf("expected registration transaction to remain committed: %v", err)
	}
	if storedAccount != account || storedPasswordHash == "" {
		t.Fatalf("expected committed account credentials, got account=%q hashEmpty=%t", storedAccount, storedPasswordHash == "")
	}

	passwordLogin := perform(handler, http.MethodPost, "/api/app/auth/login", "", map[string]string{
		"account":    account,
		"password":   password,
		"deviceInfo": deviceInfo,
	})
	if passwordLogin.Code != http.StatusInternalServerError || !strings.Contains(passwordLogin.Body.String(), "token error") {
		t.Fatalf("expected password login refresh-token failure to remain token error, got %d body=%s", passwordLogin.Code, passwordLogin.Body.String())
	}

	smsCode := requestAppDevCode(t, handler, smsLoginPhone)
	smsLogin := perform(handler, http.MethodPost, "/api/app/auth/verify-sms", "", map[string]string{
		"phone":      smsLoginPhone,
		"code":       smsCode,
		"deviceInfo": deviceInfo,
	})
	if smsLogin.Code != http.StatusInternalServerError || !strings.Contains(smsLogin.Body.String(), "token error") {
		t.Fatalf("expected SMS login refresh-token failure to remain token error, got %d body=%s", smsLogin.Code, smsLogin.Body.String())
	}
}

func TestAppPasswordLoginAttemptLimiterEnforcesDatabaseAccountLimit(t *testing.T) {
	handler, _ := newTestServer(t)
	database := openAppPasswordLoginLimiterAssertionDatabase(t)
	const (
		account = " Task7Account "
		ip      = "203.0.113.31"
	)

	for attempt := 1; attempt <= 5; attempt++ {
		response := performAppPasswordLoginFromIP(handler, account, ip)
		if response.Code != http.StatusUnauthorized {
			t.Fatalf("account attempt %d: expected 401, got %d body=%s", attempt, response.Code, response.Body.String())
		}
	}
	response := performAppPasswordLoginFromIP(handler, account, ip)
	if response.Code != http.StatusTooManyRequests {
		t.Fatalf("account attempt 6: expected 429, got %d body=%s", response.Code, response.Body.String())
	}

	assertRequestRateLimitRow(t, database, "app_password_account", appPasswordLoginLimiterTestKey("account", account), 6)
	assertRequestRateLimitRow(t, database, "app_password_ip", appPasswordLoginLimiterTestKey("ip", ip), 6)
	assertRequestRateLimitScopeRows(t, database, "admin_login", 0)
}

func TestAppPasswordLoginAttemptLimiterEnforcesDatabaseIPLimitBeforeAccount(t *testing.T) {
	handler, _ := newTestServer(t)
	database := openAppPasswordLoginLimiterAssertionDatabase(t)
	const ip = "203.0.113.32"

	for attempt := 1; attempt <= 30; attempt++ {
		account := fmt.Sprintf("task7ipuser%02d", attempt)
		response := performAppPasswordLoginFromIP(handler, account, ip)
		if response.Code != http.StatusUnauthorized {
			t.Fatalf("IP attempt %d: expected 401, got %d body=%s", attempt, response.Code, response.Body.String())
		}
	}
	rejectedAccount := "task7ipuser31"
	response := performAppPasswordLoginFromIP(handler, rejectedAccount, ip)
	if response.Code != http.StatusTooManyRequests {
		t.Fatalf("IP attempt 31: expected 429, got %d body=%s", response.Code, response.Body.String())
	}

	assertRequestRateLimitRow(t, database, "app_password_ip", appPasswordLoginLimiterTestKey("ip", ip), 31)
	var rejectedAccountRows int
	if err := database.QueryRow(`SELECT COUNT(*) FROM request_rate_limits WHERE scope='app_password_account' AND key=$1`, appPasswordLoginLimiterTestKey("account", rejectedAccount)).Scan(&rejectedAccountRows); err != nil {
		t.Fatalf("count rejected account limiter row: %v", err)
	}
	if rejectedAccountRows != 0 {
		t.Fatalf("expected rejected IP not to create account DB key, got %d rows", rejectedAccountRows)
	}
	assertRequestRateLimitScopeRows(t, database, "admin_login", 0)
}

func TestAppPasswordLoginAttemptLimiterStoresPseudonymousDatabaseKeys(t *testing.T) {
	handler, _ := newTestServer(t)
	database := openAppPasswordLoginLimiterAssertionDatabase(t)
	const (
		account = "ReviewUser0816"
		ip      = "203.0.113.81"
	)

	response := performAppPasswordLoginFromIP(handler, account, ip)
	if response.Code != http.StatusUnauthorized {
		t.Fatalf("expected 401, got %d body=%s", response.Code, response.Body.String())
	}

	for _, scope := range []string{"app_password_account", "app_password_ip"} {
		var key string
		if err := database.QueryRow(`SELECT key FROM request_rate_limits WHERE scope=$1`, scope).Scan(&key); err != nil {
			t.Fatalf("read %s limiter key: %v", scope, err)
		}
		if strings.Contains(strings.ToLower(key), strings.ToLower(account)) || strings.Contains(key, ip) {
			t.Fatalf("%s limiter persisted a direct identifier: %q", scope, key)
		}
		if !strings.HasPrefix(key, "v1:") || len(key) != len("v1:")+64 {
			t.Fatalf("%s limiter key=%q, want versioned SHA-256 digest", scope, key)
		}
	}
}

func TestAppPasswordLoginAttemptLimiterNewTestServerClearsOnlyOwnedScopes(t *testing.T) {
	database := openAppPasswordLoginLimiterAssertionDatabase(t)
	preservedScope := fmt.Sprintf("task7_preserved_%d", time.Now().UnixNano())
	fixtureKey := fmt.Sprintf("task7-fixture-%d", time.Now().UnixNano())
	t.Run("server lifecycle", func(t *testing.T) {
		started := time.Now()
		_, _ = newTestServer(t)
		_, _ = newTestServer(t)
		if elapsed := time.Since(started); elapsed > 5*time.Second {
			t.Fatalf("repeated newTestServer calls took too long: %s", elapsed)
		}
		for _, scope := range []string{"app_password_account", "app_password_ip", preservedScope} {
			if _, err := database.Exec(`
				INSERT INTO request_rate_limits(scope, key, count, expires_at)
				VALUES($1, $2, 1, now() + interval '1 minute')
				ON CONFLICT(scope, key) DO UPDATE SET count=EXCLUDED.count, expires_at=EXCLUDED.expires_at
			`, scope, fixtureKey); err != nil {
				t.Fatalf("insert rate limiter fixture for %s: %v", scope, err)
			}
		}
	})
	t.Cleanup(func() {
		_, _ = database.Exec(`DELETE FROM request_rate_limits WHERE scope=$1`, preservedScope)
	})

	assertRequestRateLimitKeyRows(t, database, "app_password_account", fixtureKey, 0)
	assertRequestRateLimitKeyRows(t, database, "app_password_ip", fixtureKey, 0)
	assertRequestRateLimitKeyRows(t, database, preservedScope, fixtureKey, 1)
}

func performAppPasswordLoginFromIP(handler http.Handler, account, ip string) *httptest.ResponseRecorder {
	body, _ := json.Marshal(map[string]string{
		"account":  account,
		"password": "secret1",
	})
	request := httptest.NewRequest(http.MethodPost, "/api/app/auth/login", strings.NewReader(string(body)))
	request.Header.Set("Content-Type", "application/json")
	request.RemoteAddr = ip + ":43210"
	response := httptest.NewRecorder()
	handler.ServeHTTP(response, request)
	return response
}

func appPasswordLoginLimiterTestKey(dimension, value string) string {
	normalized := strings.ToLower(strings.TrimSpace(value))
	mac := hmac.New(sha256.New, []byte("test-secret"))
	_, _ = mac.Write([]byte("app-password-login:" + dimension + ":" + normalized))
	return "v1:" + hex.EncodeToString(mac.Sum(nil))
}

func openAppPasswordLoginLimiterAssertionDatabase(t *testing.T) *sql.DB {
	t.Helper()
	dsn := strings.TrimSpace(os.Getenv("TEST_DATABASE_URL"))
	if dsn == "" {
		t.Skip("set TEST_DATABASE_URL to run app password limiter integration tests")
	}
	if err := testutil.ValidateIsolatedPostgresDSN(dsn); err != nil {
		t.Fatal(err)
	}
	database, err := sql.Open("pgx", dsn)
	if err != nil {
		t.Fatalf("open app password limiter assertion database: %v", err)
	}
	t.Cleanup(func() { _ = database.Close() })
	return database
}

func assertRequestRateLimitRow(t *testing.T, database *sql.DB, scope, key string, want int) {
	t.Helper()
	var got int
	if err := database.QueryRow(`SELECT count FROM request_rate_limits WHERE scope=$1 AND key=$2`, scope, key).Scan(&got); err != nil {
		t.Fatalf("read request rate limit %s/%s: %v", scope, key, err)
	}
	if got != want {
		t.Fatalf("request rate limit %s/%s count=%d want=%d", scope, key, got, want)
	}
}

func assertRequestRateLimitScopeRows(t *testing.T, database *sql.DB, scope string, want int) {
	t.Helper()
	var got int
	if err := database.QueryRow(`SELECT COUNT(*) FROM request_rate_limits WHERE scope=$1`, scope).Scan(&got); err != nil {
		t.Fatalf("count request rate limit scope %s: %v", scope, err)
	}
	if got != want {
		t.Fatalf("request rate limit scope %s rows=%d want=%d", scope, got, want)
	}
}

func assertRequestRateLimitKeyRows(t *testing.T, database *sql.DB, scope, key string, want int) {
	t.Helper()
	var got int
	if err := database.QueryRow(`SELECT COUNT(*) FROM request_rate_limits WHERE scope=$1 AND key=$2`, scope, key).Scan(&got); err != nil {
		t.Fatalf("count request rate limit key %s/%s: %v", scope, key, err)
	}
	if got != want {
		t.Fatalf("request rate limit key %s/%s rows=%d want=%d", scope, key, got, want)
	}
}

func TestAppAuthRefreshToken(t *testing.T) {
	handler, _ := newTestServer(t)

	accessToken, refreshToken := appLogin(t, handler, "13800000003")

	t.Run("invalid token returns 401", func(t *testing.T) {
		r := perform(handler, http.MethodPost, "/api/app/auth/refresh", "", map[string]string{
			"refreshToken": "notavalidtoken",
		})
		if r.Code != http.StatusUnauthorized {
			t.Fatalf("expected 401, got %d", r.Code)
		}
	})

	t.Run("valid token issues new tokens", func(t *testing.T) {
		r := perform(handler, http.MethodPost, "/api/app/auth/refresh", "", map[string]string{
			"refreshToken": refreshToken,
		})
		if r.Code != http.StatusOK {
			t.Fatalf("expected 200, got %d %s", r.Code, r.Body.String())
		}
		var resp struct {
			Data struct {
				AccessToken  string `json:"accessToken"`
				RefreshToken string `json:"refreshToken"`
			} `json:"data"`
		}
		_ = json.Unmarshal(r.Body.Bytes(), &resp)
		if resp.Data.AccessToken == "" || resp.Data.RefreshToken == "" {
			t.Fatal("expected new tokens")
		}
		if resp.Data.RefreshToken == refreshToken {
			t.Fatal("expected new refresh token (rotation)")
		}
	})

	t.Run("old refresh token is revoked after rotation", func(t *testing.T) {
		r := perform(handler, http.MethodPost, "/api/app/auth/refresh", "", map[string]string{
			"refreshToken": refreshToken,
		})
		if r.Code != http.StatusUnauthorized {
			t.Fatalf("expected 401 after rotation, got %d", r.Code)
		}
	})

	_ = accessToken
}

func TestAppAuthRefreshTokenConcurrentRotationAllowsOnlyOneSuccess(t *testing.T) {
	handler, _ := newTestServer(t)

	_, refreshToken := appLogin(t, handler, "13800000013")

	const attempts = 8
	var wg sync.WaitGroup
	statuses := make(chan int, attempts)
	for i := 0; i < attempts; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			r := perform(handler, http.MethodPost, "/api/app/auth/refresh", "", map[string]string{
				"refreshToken": refreshToken,
			})
			statuses <- r.Code
		}()
	}
	wg.Wait()
	close(statuses)

	successes := 0
	for status := range statuses {
		if status == http.StatusOK {
			successes++
		}
	}
	if successes != 1 {
		t.Fatalf("expected exactly one successful refresh rotation, got %d", successes)
	}
}

func TestAppAuthLogout(t *testing.T) {
	handler, _ := newTestServer(t)
	clearAppDeviceTokens(t)

	accessToken, refreshToken := appLogin(t, handler, "13800000004")

	register1 := perform(handler, http.MethodPost, "/api/app/push/register", accessToken, map[string]string{
		"registrationId": "logout-current-device",
		"platform":       "ios",
	})
	if register1.Code != http.StatusOK {
		t.Fatalf("register current device failed: %d %s", register1.Code, register1.Body.String())
	}
	register2 := perform(handler, http.MethodPost, "/api/app/push/register", accessToken, map[string]string{
		"registrationId": "logout-other-device",
		"platform":       "android",
	})
	if register2.Code != http.StatusOK {
		t.Fatalf("register other device failed: %d %s", register2.Code, register2.Body.String())
	}

	r := perform(handler, http.MethodPost, "/api/app/auth/logout", "", map[string]string{
		"refreshToken":   refreshToken,
		"registrationId": "logout-current-device",
	})
	if r.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", r.Code)
	}

	// token should now be revoked
	r2 := perform(handler, http.MethodPost, "/api/app/auth/refresh", "", map[string]string{
		"refreshToken": refreshToken,
	})
	if r2.Code != http.StatusUnauthorized {
		t.Fatalf("expected 401 after logout, got %d", r2.Code)
	}

	adminToken := loginToken(t, handler)
	pushResp := perform(handler, http.MethodPost, "/api/push/send", adminToken, map[string]string{
		"title":   "logout test",
		"content": "logout test",
	})
	if pushResp.Code != http.StatusOK {
		t.Fatalf("push send failed: %d %s", pushResp.Code, pushResp.Body.String())
	}
	sent := waitForPushSendSentCount(t, handler, adminToken, pushResp)
	if sent != 1 {
		t.Fatalf("expected logout with registrationId to keep only other device, sent=%d", sent)
	}
}

func TestAppAuthLogoutWithoutRegistrationIDKeepsDeviceTokens(t *testing.T) {
	handler, _ := newTestServer(t)
	clearAppDeviceTokens(t)

	accessToken, refreshToken := appLogin(t, handler, "13800000014")
	for _, registrationID := range []string{"logout-all-device-1", "logout-all-device-2"} {
		resp := perform(handler, http.MethodPost, "/api/app/push/register", accessToken, map[string]string{
			"registrationId": registrationID,
			"platform":       "ios",
		})
		if resp.Code != http.StatusOK {
			t.Fatalf("register %s failed: %d %s", registrationID, resp.Code, resp.Body.String())
		}
	}

	resp := perform(handler, http.MethodPost, "/api/app/auth/logout", "", map[string]string{
		"refreshToken": refreshToken,
	})
	if resp.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", resp.Code)
	}

	refreshResp := perform(handler, http.MethodPost, "/api/app/auth/refresh", "", map[string]string{
		"refreshToken": refreshToken,
	})
	if refreshResp.Code != http.StatusUnauthorized {
		t.Fatalf("expected refresh token to be revoked after logout, got %d body=%s", refreshResp.Code, refreshResp.Body.String())
	}

	adminToken := loginToken(t, handler)
	pushResp := perform(handler, http.MethodPost, "/api/push/send", adminToken, map[string]string{
		"title":   "logout all test",
		"content": "logout all test",
	})
	if pushResp.Code != http.StatusOK {
		t.Fatalf("push send failed: %d %s", pushResp.Code, pushResp.Body.String())
	}
	sent := waitForPushSendSentCount(t, handler, adminToken, pushResp)
	if sent != 2 {
		t.Fatalf("expected logout without registrationId to keep device tokens, sent=%d", sent)
	}
}

func waitForPushSendSentCount(t *testing.T, handler http.Handler, adminToken string, pushResp *httptest.ResponseRecorder) int {
	t.Helper()
	var sendBody struct {
		Data struct {
			RecordID int64  `json:"recordId"`
			Status   string `json:"status"`
		} `json:"data"`
	}
	if err := json.Unmarshal(pushResp.Body.Bytes(), &sendBody); err != nil {
		t.Fatal(err)
	}
	if sendBody.Data.RecordID == 0 {
		t.Fatalf("expected async push response recordId, body=%s", pushResp.Body.String())
	}

	deadline := time.Now().Add(2 * time.Second)
	for time.Now().Before(deadline) {
		listResp := perform(handler, http.MethodGet, "/api/push/list?page=1&pageSize=20", adminToken, nil)
		if listResp.Code != http.StatusOK {
			t.Fatalf("push list failed: %d %s", listResp.Code, listResp.Body.String())
		}
		var listBody struct {
			Data struct {
				Items []struct {
					ID           int64  `json:"id"`
					SentCount    int    `json:"sentCount"`
					Status       string `json:"status"`
					ErrorMessage string `json:"errorMessage"`
				} `json:"items"`
			} `json:"data"`
		}
		if err := json.Unmarshal(listResp.Body.Bytes(), &listBody); err != nil {
			t.Fatal(err)
		}
		for _, item := range listBody.Data.Items {
			if item.ID != sendBody.Data.RecordID {
				continue
			}
			switch item.Status {
			case "success":
				return item.SentCount
			case "failed":
				t.Fatalf("push send failed asynchronously: %s", item.ErrorMessage)
			}
		}
		time.Sleep(20 * time.Millisecond)
	}
	t.Fatalf("timed out waiting for push record %d to finish", sendBody.Data.RecordID)
	return 0
}

type appPasswordSessionResponse struct {
	Code int `json:"code"`
	Data struct {
		AccessToken  string       `json:"accessToken"`
		RefreshToken string       `json:"refreshToken"`
		User         appuser.User `json:"user"`
	} `json:"data"`
	Error   any    `json:"error"`
	Message string `json:"message"`
}

func decodeAppPasswordSessionResponse(t *testing.T, response *httptest.ResponseRecorder, operation string) appPasswordSessionResponse {
	t.Helper()
	if response.Code != http.StatusOK {
		t.Fatalf("%s returned status %d", operation, response.Code)
	}
	var body appPasswordSessionResponse
	if err := json.Unmarshal(response.Body.Bytes(), &body); err != nil {
		t.Fatalf("decode %s response: %v", operation, err)
	}
	if body.Code != 0 || body.Message != "ok" || body.Error != nil {
		t.Fatalf("%s returned an invalid response envelope", operation)
	}
	if body.Data.AccessToken == "" || body.Data.RefreshToken == "" || body.Data.User.ID <= 0 {
		t.Fatalf("%s does not match the Flutter session response shape", operation)
	}
	return body
}

type appPasswordInvalidCredentialsEnvelope struct {
	HTTPStatus int
	Code       int
	Message    string
	Error      string
	DataIsNil  bool
}

func assertAppPasswordInvalidCredentialsResponse(t *testing.T, response *httptest.ResponseRecorder, operation string) appPasswordInvalidCredentialsEnvelope {
	t.Helper()
	if response.Code != http.StatusUnauthorized {
		t.Fatalf("%s returned status %d, want 401", operation, response.Code)
	}
	var body struct {
		Code    int    `json:"code"`
		Data    any    `json:"data"`
		Error   string `json:"error"`
		Message string `json:"message"`
	}
	if err := json.Unmarshal(response.Body.Bytes(), &body); err != nil {
		t.Fatalf("decode %s response: %v", operation, err)
	}
	if body.Code != -1 || body.Data != nil || body.Error != "账号或密码错误" || body.Message != "账号或密码错误" {
		t.Fatalf("%s did not return the generic invalid-credentials response", operation)
	}
	return appPasswordInvalidCredentialsEnvelope{
		HTTPStatus: response.Code,
		Code:       body.Code,
		Message:    body.Message,
		Error:      body.Error,
		DataIsNil:  body.Data == nil,
	}
}

func openAppAuthContractDatabase(t *testing.T) *sql.DB {
	t.Helper()
	dsn := strings.TrimSpace(os.Getenv("TEST_DATABASE_URL"))
	if dsn == "" {
		t.Skip("set TEST_DATABASE_URL to run app auth contract tests")
	}
	if err := testutil.ValidateIsolatedPostgresDSN(dsn); err != nil {
		t.Fatal(err)
	}
	database, err := sql.Open("pgx", dsn)
	if err != nil {
		t.Fatalf("open app auth contract database: %v", err)
	}
	t.Cleanup(func() { _ = database.Close() })
	return database
}

func cleanupAppAuthContractFixtures(t *testing.T, database *sql.DB, phones, accounts []string) {
	t.Helper()
	for _, account := range accounts {
		if _, err := database.Exec(`DELETE FROM app_users WHERE lower(account)=lower($1)`, account); err != nil {
			t.Fatalf("clear app auth fixture user by account: %v", err)
		}
	}
	for _, phone := range phones {
		if _, err := database.Exec(`DELETE FROM app_password_reset_codes WHERE phone=$1`, phone); err != nil {
			t.Fatalf("clear app auth fixture password reset codes: %v", err)
		}
		if _, err := database.Exec(`DELETE FROM app_sms_codes WHERE phone=$1`, phone); err != nil {
			t.Fatalf("clear app auth fixture SMS codes: %v", err)
		}
		if _, err := database.Exec(`DELETE FROM app_users WHERE phone=$1`, phone); err != nil {
			t.Fatalf("clear app auth fixture user by phone: %v", err)
		}
	}
}

func countAppAuthContractRows(t *testing.T, database *sql.DB, query string, args ...any) int {
	t.Helper()
	var count int
	if err := database.QueryRow(query, args...).Scan(&count); err != nil {
		t.Fatalf("count app auth contract rows: %v", err)
	}
	return count
}

func requestAppDevCode(t *testing.T, handler http.Handler, phone string) string {
	t.Helper()
	response := perform(handler, http.MethodPost, "/api/app/auth/send-sms", "", map[string]string{"phone": phone})
	if response.Code != http.StatusOK {
		t.Fatalf("send-sms failed for %s: %d %s", phone, response.Code, response.Body.String())
	}
	var body struct {
		Data map[string]any `json:"data"`
	}
	if err := json.Unmarshal(response.Body.Bytes(), &body); err != nil {
		t.Fatal(err)
	}
	code, _ := body.Data["devCode"].(string)
	if code == "" {
		t.Fatal("expected devCode in test mode")
	}
	return code
}

func cleanupAppPasswordSessionFailureFixtures(t *testing.T, database *sql.DB, registrationPhone, smsLoginPhone, account string) {
	t.Helper()
	if _, err := database.Exec(`
		DELETE FROM app_refresh_tokens
		WHERE app_user_id IN (
			SELECT id FROM app_users WHERE phone IN ($1, $2) OR lower(account)=lower($3)
		)
	`, registrationPhone, smsLoginPhone, account); err != nil {
		t.Fatalf("clear task 6 refresh tokens: %v", err)
	}
	if _, err := database.Exec(`DELETE FROM app_sms_codes WHERE phone IN ($1, $2)`, registrationPhone, smsLoginPhone); err != nil {
		t.Fatalf("clear task 6 SMS codes: %v", err)
	}
	if _, err := database.Exec(`DELETE FROM app_users WHERE phone IN ($1, $2) OR lower(account)=lower($3)`, registrationPhone, smsLoginPhone, account); err != nil {
		t.Fatalf("clear task 6 app users: %v", err)
	}
}

func installAppRefreshTokenFailureTrigger(t *testing.T, database *sql.DB, deviceInfo string) {
	t.Helper()
	const (
		triggerName  = "task6_fail_app_refresh_token_insert"
		functionName = "task6_fail_app_refresh_token_insert_fn"
	)
	if _, err := database.Exec(`DROP TRIGGER IF EXISTS ` + triggerName + ` ON app_refresh_tokens`); err != nil {
		t.Fatalf("drop stale task 6 trigger: %v", err)
	}
	if _, err := database.Exec(`DROP FUNCTION IF EXISTS ` + functionName + `()`); err != nil {
		t.Fatalf("drop stale task 6 function: %v", err)
	}
	quotedDeviceInfo := strings.ReplaceAll(deviceInfo, "'", "''")
	createFunction := fmt.Sprintf(`
		CREATE FUNCTION %s() RETURNS trigger AS $$
		BEGIN
			IF NEW.device_info = '%s' THEN
				RAISE EXCEPTION 'forced refresh token persistence failure';
			END IF;
			RETURN NEW;
		END;
		$$ LANGUAGE plpgsql
	`, functionName, quotedDeviceInfo)
	if _, err := database.Exec(createFunction); err != nil {
		t.Fatalf("create task 6 trigger function: %v", err)
	}
	t.Cleanup(func() {
		if _, err := database.Exec(`DROP TRIGGER IF EXISTS ` + triggerName + ` ON app_refresh_tokens`); err != nil {
			t.Errorf("drop task 6 trigger: %v", err)
		}
		if _, err := database.Exec(`DROP FUNCTION IF EXISTS ` + functionName + `()`); err != nil {
			t.Errorf("drop task 6 function: %v", err)
		}
	})
	if _, err := database.Exec(`CREATE TRIGGER ` + triggerName + ` BEFORE INSERT ON app_refresh_tokens FOR EACH ROW EXECUTE FUNCTION ` + functionName + `()`); err != nil {
		t.Fatalf("create task 6 trigger: %v", err)
	}
}

func clearAppDeviceTokens(t *testing.T) {
	t.Helper()
	dsn := os.Getenv("TEST_DATABASE_URL")
	if dsn == "" {
		t.Skip("set TEST_DATABASE_URL to run server integration tests")
	}
	if err := testutil.ValidateIsolatedPostgresDSN(dsn); err != nil {
		t.Fatal(err)
	}
	database, err := sql.Open("pgx", dsn)
	if err != nil {
		t.Fatalf("open db: %v", err)
	}
	defer database.Close()
	if _, err := database.Exec("DELETE FROM app_device_tokens"); err != nil {
		t.Fatalf("clear app_device_tokens: %v", err)
	}
}

func TestAppUserInfo(t *testing.T) {
	handler, _ := newTestServer(t)

	accessToken, _ := appLogin(t, handler, "13800000005")

	t.Run("no token returns 401", func(t *testing.T) {
		r := perform(handler, http.MethodGet, "/api/app/user/info", "", nil)
		if r.Code != http.StatusUnauthorized {
			t.Fatalf("expected 401, got %d", r.Code)
		}
	})

	t.Run("valid token returns user", func(t *testing.T) {
		r := perform(handler, http.MethodGet, "/api/app/user/info", accessToken, nil)
		if r.Code != http.StatusOK {
			t.Fatalf("expected 200, got %d %s", r.Code, r.Body.String())
		}
		var resp struct {
			Data appuser.User `json:"data"`
		}
		_ = json.Unmarshal(r.Body.Bytes(), &resp)
		if resp.Data.Phone != "13800000005" {
			t.Fatalf("unexpected phone: %s", resp.Data.Phone)
		}
	})
}

func TestAppTokenCannotAccessBackendAPI(t *testing.T) {
	handler, _ := newTestServer(t)
	accessToken, _ := appLogin(t, handler, "13800000006")

	r := perform(handler, http.MethodGet, "/api/user/info", accessToken, nil)
	if r.Code != http.StatusUnauthorized {
		t.Fatalf("expected app token to be rejected by backend auth, got %d body=%s", r.Code, r.Body.String())
	}
}

func TestBackendTokenCannotAccessAppAPI(t *testing.T) {
	handler, _ := newTestServer(t)
	token := loginToken(t, handler)

	r := perform(handler, http.MethodGet, "/api/app/user/info", token, nil)
	if r.Code != http.StatusUnauthorized {
		t.Fatalf("expected backend token to be rejected by app auth, got %d body=%s", r.Code, r.Body.String())
	}
}

// appLogin sends an SMS, verifies it, and returns (accessToken, refreshToken).
// Skips the test if not in dev mode (no devCode).
func appLogin(t *testing.T, handler http.Handler, phone string) (string, string) {
	t.Helper()

	sendResp := perform(handler, http.MethodPost, "/api/app/auth/send-sms", "", map[string]string{"phone": phone})
	if sendResp.Code != http.StatusOK {
		t.Fatalf("send-sms failed: %d", sendResp.Code)
	}
	var sendBody struct {
		Data map[string]any `json:"data"`
	}
	_ = json.Unmarshal(sendResp.Body.Bytes(), &sendBody)
	devCode, _ := sendBody.Data["devCode"].(string)
	if devCode == "" {
		t.Skip("no devCode — SMS provider configured, skipping")
	}

	verifyResp := perform(handler, http.MethodPost, "/api/app/auth/verify-sms", "", map[string]any{
		"phone":      phone,
		"code":       devCode,
		"deviceInfo": "test-device",
	})
	if verifyResp.Code != http.StatusOK {
		t.Fatalf("verify-sms failed: %d %s", verifyResp.Code, verifyResp.Body.String())
	}
	var verifyBody struct {
		Data struct {
			AccessToken  string `json:"accessToken"`
			RefreshToken string `json:"refreshToken"`
		} `json:"data"`
	}
	_ = json.Unmarshal(verifyResp.Body.Bytes(), &verifyBody)
	return verifyBody.Data.AccessToken, verifyBody.Data.RefreshToken
}
