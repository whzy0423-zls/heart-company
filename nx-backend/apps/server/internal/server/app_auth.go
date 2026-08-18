package server

import (
	"context"
	"crypto/rand"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"math/big"
	"net/http"
	"strings"
	"time"

	"nine-xing/nx-backend/apps/server/internal/appuser"
	"nine-xing/nx-backend/apps/server/internal/auth"
	"nine-xing/nx-backend/apps/server/internal/config"
	"nine-xing/nx-backend/apps/server/internal/httpx"
	"nine-xing/nx-backend/apps/server/internal/realip"
)

const (
	appAccessTokenDuration  = 15 * time.Minute
	appRefreshTokenDuration = 30 * 24 * time.Hour
	smsCodeExpiry           = 10 * time.Minute
)

func (s *Server) appSendSMS(w http.ResponseWriter, r *http.Request) {
	r.Body = http.MaxBytesReader(w, r.Body, 4*1024)
	var body struct {
		Phone   string `json:"phone"`
		Purpose string `json:"purpose"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		httpx.Fail(w, http.StatusBadRequest, "invalid request body")
		return
	}
	phone := strings.TrimSpace(body.Phone)
	purpose := strings.TrimSpace(body.Purpose)
	if !isMainlandPhone(phone) {
		httpx.Fail(w, http.StatusBadRequest, "invalid phone number")
		return
	}

	now := time.Now()
	ip := s.clientIP(r)

	if !s.smsIPLimiter.Allow(ip, now) {
		httpx.Fail(w, http.StatusTooManyRequests, "发送过于频繁，请稍后再试")
		return
	}
	if !s.smsPhoneLimiter.Allow(phone, now) {
		httpx.Fail(w, http.StatusTooManyRequests, "发送过于频繁，请稍后再试")
		return
	}
	provider := strings.TrimSpace(s.env.SMS.Provider)
	appEnv := config.NormalizeAppEnv(s.env.AppEnv)
	if provider == "" && appEnv != "dev" && appEnv != "test" {
		httpx.Fail(w, http.StatusServiceUnavailable, "SMS provider is not configured")
		return
	}

	code, err := generateSMSCode()
	if err != nil {
		httpx.Fail(w, http.StatusInternalServerError, "failed to generate code")
		return
	}
	codeHash := appuser.HashToken(code)

	if purpose == "password_reset" {
		eligible, err := s.appUsers.StorePasswordResetCodeIfEligible(r.Context(), phone, codeHash, ip, now.Add(smsCodeExpiry))
		if err != nil {
			httpx.Fail(w, http.StatusInternalServerError, "failed to store code")
			return
		}
		if !eligible {
			httpx.Fail(w, http.StatusBadRequest, "当前手机号未注册，请注册账号")
			return
		}
	} else {
		if purpose != "" && purpose != "register" {
			httpx.Fail(w, http.StatusBadRequest, "invalid SMS purpose")
			return
		}
		if err := s.appUsers.StoreSMSCode(r.Context(), phone, codeHash, ip, now.Add(smsCodeExpiry)); err != nil {
			httpx.Fail(w, http.StatusInternalServerError, "failed to store code")
			return
		}
	}

	if provider == "" {
		log.Print("[SMS-DEV] local response issued")
		httpx.OK(w, map[string]any{"devCode": code})
		return
	}

	if err := s.smsSender.Send(r.Context(), phone, code); err != nil {
		log.Printf("[SMS] send error phone=%s: %v", phone, err)
		httpx.Fail(w, http.StatusInternalServerError, "短信发送失败")
		return
	}
	log.Printf("[SMS] sent to %s", phone)
	if s.smsReporter != nil {
		// The report is best-effort and runs outside the request context so a
		// client disconnect cannot cancel the audit notification.
		go s.reportSMSDelivery(phone, purpose)
	}
	httpx.OK(w, nil)
}

func (s *Server) reportSMSDelivery(phone, purpose string) {
	label := "注册"
	if purpose == "password_reset" {
		label = "找回密码"
	}
	content := fmt.Sprintf("验证码短信已提交：手机号 %s，用途 %s", maskSMSPhone(phone), label)
	ctx, cancel := context.WithTimeout(context.Background(), 6*time.Second)
	defer cancel()
	if err := s.smsReporter.Report(ctx, "芯之力验证码已发送", content, "text", ""); err != nil {
		log.Printf("[SMS-REPORT] send error phone=%s: %v", maskSMSPhone(phone), err)
		return
	}
	log.Printf("[SMS-REPORT] sent phone=%s", maskSMSPhone(phone))
}

func maskSMSPhone(phone string) string {
	phone = strings.TrimSpace(phone)
	if len(phone) <= 7 {
		return "***"
	}
	return phone[:3] + "****" + phone[len(phone)-4:]
}

func (s *Server) appVerifySMS(w http.ResponseWriter, r *http.Request) {
	r.Body = http.MaxBytesReader(w, r.Body, 8*1024)
	var body struct {
		Phone      string `json:"phone"`
		Code       string `json:"code"`
		DeviceInfo string `json:"deviceInfo"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		httpx.Fail(w, http.StatusBadRequest, "invalid request body")
		return
	}
	phone := strings.TrimSpace(body.Phone)
	code := strings.TrimSpace(body.Code)
	if !isMainlandPhone(phone) || len(code) != 6 {
		httpx.Fail(w, http.StatusBadRequest, "invalid phone or code")
		return
	}
	if !s.allowSMSVerifyAttempt(phone, s.clientIP(r), time.Now()) {
		httpx.Fail(w, http.StatusTooManyRequests, "验证码验证过于频繁，请稍后再试")
		return
	}

	codeHash := appuser.HashToken(code)
	valid, err := s.appUsers.VerifyAndUseSMSCode(r.Context(), phone, codeHash)
	if err != nil {
		httpx.Fail(w, http.StatusInternalServerError, "verify failed")
		return
	}
	if !valid {
		httpx.Fail(w, http.StatusUnauthorized, "验证码错误或已过期")
		return
	}

	user, err := s.appUsers.FindOrCreateByPhone(r.Context(), phone)
	if err != nil {
		httpx.Fail(w, http.StatusInternalServerError, "login failed")
		return
	}
	if user.Status != "active" {
		httpx.Fail(w, http.StatusForbidden, "账号已被禁用")
		return
	}
	if err := s.writeAppSession(w, r, user, body.DeviceInfo); err != nil {
		httpx.Fail(w, http.StatusInternalServerError, "token error")
		return
	}
}

func (s *Server) writeAppSession(w http.ResponseWriter, r *http.Request, user appuser.User, deviceInfo string) error {
	if s == nil || s.appUsers == nil {
		return fmt.Errorf("app user store unavailable")
	}
	accessToken, err := s.issueAppAccessToken(user)
	if err != nil {
		return fmt.Errorf("issue app access token: %w", err)
	}

	refreshRaw, err := generateRefreshToken()
	if err != nil {
		return fmt.Errorf("generate app refresh token: %w", err)
	}
	refreshHash := appuser.HashToken(refreshRaw)
	if err := s.appUsers.CreateRefreshToken(r.Context(), user.ID, refreshHash, deviceInfo, time.Now().Add(appRefreshTokenDuration)); err != nil {
		return fmt.Errorf("persist app refresh token: %w", err)
	}

	httpx.OK(w, map[string]any{
		"accessToken":  accessToken,
		"refreshToken": refreshRaw,
		"user":         user,
	})
	return nil
}

func (s *Server) appRefreshToken(w http.ResponseWriter, r *http.Request) {
	r.Body = http.MaxBytesReader(w, r.Body, 4*1024)
	var body struct {
		RefreshToken string `json:"refreshToken"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		httpx.Fail(w, http.StatusBadRequest, "invalid request body")
		return
	}
	if body.RefreshToken == "" {
		httpx.Fail(w, http.StatusBadRequest, "missing refresh token")
		return
	}

	newRefreshRaw, err := generateRefreshToken()
	if err != nil {
		httpx.Fail(w, http.StatusInternalServerError, "token error")
		return
	}
	tokenHash := appuser.HashToken(body.RefreshToken)
	newRefreshHash := appuser.HashToken(newRefreshRaw)
	rt, err := s.appUsers.RotateRefreshToken(r.Context(), tokenHash, newRefreshHash, time.Now().Add(appRefreshTokenDuration))
	if err != nil {
		httpx.Fail(w, http.StatusUnauthorized, "invalid refresh token")
		return
	}

	user, err := s.appUsers.FindByID(r.Context(), rt.AppUserID)
	if err != nil {
		httpx.Fail(w, http.StatusInternalServerError, "user not found")
		return
	}
	if user.Status != "active" {
		httpx.Fail(w, http.StatusForbidden, "账号已被禁用")
		return
	}

	accessToken, err := s.issueAppAccessToken(user)
	if err != nil {
		httpx.Fail(w, http.StatusInternalServerError, "token error")
		return
	}

	httpx.OK(w, map[string]any{
		"accessToken":  accessToken,
		"refreshToken": newRefreshRaw,
	})
}

func (s *Server) appLogout(w http.ResponseWriter, r *http.Request) {
	r.Body = http.MaxBytesReader(w, r.Body, 4*1024)
	var body struct {
		RefreshToken   string `json:"refreshToken"`
		RegistrationID string `json:"registrationId"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		httpx.OK(w, nil)
		return
	}
	body.RefreshToken = strings.TrimSpace(body.RefreshToken)
	body.RegistrationID = strings.TrimSpace(body.RegistrationID)
	if body.RefreshToken != "" {
		tokenHash := appuser.HashToken(body.RefreshToken)
		rt, err := s.appUsers.FindRefreshToken(r.Context(), tokenHash)
		if err == nil && s.pushStore != nil && body.RegistrationID != "" {
			_ = s.pushStore.UnregisterDevice(r.Context(), rt.AppUserID, body.RegistrationID)
		}
		_ = s.appUsers.RevokeRefreshToken(r.Context(), tokenHash)
	}
	httpx.OK(w, nil)
}

func (s *Server) appUserInfo(w http.ResponseWriter, r *http.Request) {
	userInfo, ok := appUserFromContext(r)
	if !ok {
		httpx.Fail(w, http.StatusUnauthorized, "unauthorized")
		return
	}
	user, err := s.appUsers.FindByID(r.Context(), userInfo.ID)
	if err != nil {
		httpx.Fail(w, http.StatusNotFound, "user not found")
		return
	}
	httpx.OK(w, user)
}

// --- helpers ---

func (s *Server) issueAppAccessToken(user appuser.User) (string, error) {
	info := auth.UserInfo{
		ID:        user.ID,
		Phone:     user.Phone,
		RealName:  user.Nickname,
		Roles:     []string{"app_user"},
		TokenKind: auth.TokenKindApp,
	}
	return auth.SignWithExpiry(info, s.env.JWTSecret, appAccessTokenDuration)
}

func generateSMSCode() (string, error) {
	return generateSMSCodeFromReader(rand.Reader)
}

func generateSMSCodeFromReader(reader io.Reader) (string, error) {
	n, err := rand.Int(reader, big.NewInt(1000000))
	if err != nil {
		return "", err
	}
	return fmt.Sprintf("%06d", n.Int64()), nil
}

func generateRefreshToken() (string, error) {
	b := make([]byte, 32)
	if _, err := rand.Read(b); err != nil {
		return "", err
	}
	return fmt.Sprintf("%x", b), nil
}

func isMainlandPhone(phone string) bool {
	if len(phone) != 11 || phone[0] != '1' || phone[1] < '3' || phone[1] > '9' {
		return false
	}
	for i := 2; i < len(phone); i++ {
		if phone[i] < '0' || phone[i] > '9' {
			return false
		}
	}
	return true
}

func (s *Server) clientIP(r *http.Request) string {
	if s == nil {
		return realip.RemoteAddr(r)
	}
	return realip.FromRequest(r, s.trustedProxyCIDRs)
}

func (s *Server) allowSMSVerifyAttempt(phone, ip string, now time.Time) bool {
	if s.smsVerifyIPLimiter != nil && !s.smsVerifyIPLimiter.Allow(ip, now) {
		return false
	}
	if s.smsVerifyPhoneLimiter != nil && !s.smsVerifyPhoneLimiter.Allow(phone, now) {
		return false
	}
	return true
}

type appContextKey struct{}

func appUserFromContext(r *http.Request) (auth.UserInfo, bool) {
	u, ok := r.Context().Value(appContextKey{}).(auth.UserInfo)
	return u, ok
}

func (s *Server) requireAppAuth(next http.HandlerFunc) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		tokenUser, err := auth.BearerUserWithKind(r.Header.Get("Authorization"), s.env.JWTSecret, auth.TokenKindApp)
		if err != nil {
			httpx.Fail(w, http.StatusUnauthorized, "unauthorized")
			return
		}
		if s.db == nil {
			httpx.Fail(w, http.StatusUnauthorized, "unauthorized")
			return
		}
		appUser, err := s.appUsers.FindByID(r.Context(), tokenUser.ID)
		if err != nil || appUser.Status != "active" {
			httpx.Fail(w, http.StatusUnauthorized, "unauthorized")
			return
		}
		user := auth.UserInfo{
			ID:       appUser.ID,
			Phone:    appUser.Phone,
			RealName: appUser.Nickname,
			Roles:    []string{"app_user"},
		}
		ctx := r.Context()
		ctx = contextWithAppUser(ctx, user)
		next(w, r.WithContext(ctx))
	}
}

func contextWithAppUser(ctx context.Context, user auth.UserInfo) context.Context {
	return context.WithValue(ctx, appContextKey{}, user)
}
