package server_test

import (
	"database/sql"
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
