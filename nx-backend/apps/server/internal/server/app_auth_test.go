package server_test

import (
	"encoding/json"
	"fmt"
	"net/http"
	"testing"

	"nine-xing/nx-backend/apps/server/internal/appuser"
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

func TestAppAuthLogout(t *testing.T) {
	handler, _ := newTestServer(t)

	_, refreshToken := appLogin(t, handler, "13800000004")

	r := perform(handler, http.MethodPost, "/api/app/auth/logout", "", map[string]string{
		"refreshToken": refreshToken,
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
