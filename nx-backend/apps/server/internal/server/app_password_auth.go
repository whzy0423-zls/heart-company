package server

import (
	"context"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"strings"
	"time"

	"nine-xing/nx-backend/apps/server/internal/appuser"
	"nine-xing/nx-backend/apps/server/internal/httpx"
)

const (
	appPasswordAuthBodyLimit         = 8 * 1024
	appPasswordLoginDBLimiterTimeout = 500 * time.Millisecond
)

func (s *Server) appRegisterWithPassword(w http.ResponseWriter, r *http.Request) {
	var body struct {
		Nickname   string `json:"nickname"`
		Account    string `json:"account"`
		Password   string `json:"password"`
		Phone      string `json:"phone"`
		Code       string `json:"code"`
		DeviceInfo string `json:"deviceInfo"`
	}
	if err := decodeAppPasswordJSON(w, r, &body); err != nil {
		httpx.Fail(w, http.StatusBadRequest, "invalid request body")
		return
	}

	nickname := strings.TrimSpace(body.Nickname)
	account := strings.TrimSpace(body.Account)
	phone := strings.TrimSpace(body.Phone)
	code := strings.TrimSpace(body.Code)
	deviceInfo := strings.TrimSpace(body.DeviceInfo)

	if err := appuser.ValidateAccount(account); err != nil {
		status, message := appPasswordRegistrationErrorResponse(err)
		httpx.Fail(w, status, message)
		return
	}
	if err := appuser.ValidatePassword(body.Password); err != nil {
		status, message := appPasswordRegistrationErrorResponse(err)
		httpx.Fail(w, status, message)
		return
	}
	if err := appuser.ValidateNickname(nickname); err != nil {
		status, message := appPasswordRegistrationErrorResponse(err)
		httpx.Fail(w, status, message)
		return
	}
	if !isMainlandPhone(phone) {
		httpx.Fail(w, http.StatusBadRequest, "手机号格式不正确")
		return
	}
	if !isSixDigitCode(code) {
		httpx.Fail(w, http.StatusBadRequest, "验证码格式不正确")
		return
	}

	now := time.Now()
	if !s.allowSMSVerifyAttempt(phone, s.clientIP(r), now) {
		httpx.Fail(w, http.StatusTooManyRequests, "验证码验证过于频繁，请稍后再试")
		return
	}
	if s.appUsers == nil || s.db == nil {
		httpx.Fail(w, http.StatusServiceUnavailable, "认证服务不可用")
		return
	}

	user, err := s.appUsers.RegisterWithPassword(r.Context(), appuser.RegisterWithPasswordInput{
		Nickname:    nickname,
		Account:     account,
		Password:    body.Password,
		Phone:       phone,
		SMSCodeHash: appuser.HashToken(code),
	})
	if err != nil {
		status, message := appPasswordRegistrationErrorResponse(err)
		httpx.Fail(w, status, message)
		return
	}

	if err := s.writeAppSession(w, r, user, deviceInfo); err != nil {
		httpx.Fail(w, http.StatusInternalServerError, "注册已成功，请使用账号密码登录")
		return
	}
}

func (s *Server) appLoginWithPassword(w http.ResponseWriter, r *http.Request) {
	var body struct {
		Account    string `json:"account"`
		Password   string `json:"password"`
		DeviceInfo string `json:"deviceInfo"`
	}
	if err := decodeAppPasswordJSON(w, r, &body); err != nil {
		httpx.Fail(w, http.StatusBadRequest, "invalid request body")
		return
	}

	identifier := strings.TrimSpace(body.Account)
	deviceInfo := strings.TrimSpace(body.DeviceInfo)
	if identifier == "" || body.Password == "" {
		httpx.Fail(w, http.StatusBadRequest, "请输入账号和密码")
		return
	}
	if !s.allowAppPasswordLoginAttempt(r.Context(), identifier, s.clientIP(r), time.Now()) {
		httpx.Fail(w, http.StatusTooManyRequests, "登录尝试过于频繁，请稍后再试")
		return
	}
	if s.appUsers == nil || s.db == nil {
		httpx.Fail(w, http.StatusServiceUnavailable, "认证服务不可用")
		return
	}

	user, err := s.appUsers.AuthenticateWithPassword(r.Context(), identifier, body.Password)
	if err != nil {
		status, message := appPasswordLoginErrorResponse(err)
		httpx.Fail(w, status, message)
		return
	}

	if err := s.writeAppSession(w, r, user, deviceInfo); err != nil {
		httpx.Fail(w, http.StatusInternalServerError, "token error")
		return
	}
}

func (s *Server) appResetPassword(w http.ResponseWriter, r *http.Request) {
	var body struct {
		Phone    string `json:"phone"`
		Code     string `json:"code"`
		Password string `json:"password"`
	}
	if err := decodeAppPasswordJSON(w, r, &body); err != nil {
		httpx.Fail(w, http.StatusBadRequest, "invalid request body")
		return
	}

	phone := strings.TrimSpace(body.Phone)
	code := strings.TrimSpace(body.Code)
	if !isMainlandPhone(phone) {
		httpx.Fail(w, http.StatusBadRequest, "手机号格式不正确")
		return
	}
	if !isSixDigitCode(code) {
		httpx.Fail(w, http.StatusBadRequest, "验证码格式不正确")
		return
	}
	if err := appuser.ValidatePassword(body.Password); err != nil {
		httpx.Fail(w, http.StatusBadRequest, "密码格式不正确")
		return
	}
	if !s.allowSMSVerifyAttempt(phone, s.clientIP(r), time.Now()) {
		httpx.Fail(w, http.StatusTooManyRequests, "验证码验证过于频繁，请稍后再试")
		return
	}
	if s.appUsers == nil || s.db == nil {
		httpx.Fail(w, http.StatusServiceUnavailable, "认证服务不可用")
		return
	}

	if err := s.appUsers.ResetPassword(r.Context(), appuser.ResetPasswordInput{
		Phone:    phone,
		CodeHash: appuser.HashToken(code),
		Password: body.Password,
	}); err != nil {
		if errors.Is(err, appuser.ErrInvalidCredentials) {
			httpx.Fail(w, http.StatusUnauthorized, "验证码错误或已过期")
			return
		}
		httpx.Fail(w, http.StatusInternalServerError, "重置密码失败")
		return
	}
	httpx.OK(w, nil)
}

func decodeAppPasswordJSON(w http.ResponseWriter, r *http.Request, destination any) error {
	r.Body = http.MaxBytesReader(w, r.Body, appPasswordAuthBodyLimit)
	decoder := json.NewDecoder(r.Body)
	if err := decoder.Decode(destination); err != nil {
		return err
	}
	var trailing any
	if err := decoder.Decode(&trailing); !errors.Is(err, io.EOF) {
		if err == nil {
			return errors.New("trailing JSON value")
		}
		return err
	}
	return nil
}

func appPasswordRegistrationErrorResponse(err error) (int, string) {
	switch {
	case errors.Is(err, appuser.ErrInvalidAccount):
		return http.StatusBadRequest, "用户名格式不正确"
	case errors.Is(err, appuser.ErrInvalidPassword):
		return http.StatusBadRequest, "密码格式不正确"
	case errors.Is(err, appuser.ErrInvalidNickname):
		return http.StatusBadRequest, "昵称格式不正确"
	case errors.Is(err, appuser.ErrInvalidSMSCode):
		return http.StatusUnauthorized, "验证码错误或已过期"
	case errors.Is(err, appuser.ErrUserDisabled):
		return http.StatusForbidden, "账号已被禁用"
	case errors.Is(err, appuser.ErrAccountTaken):
		return http.StatusConflict, "用户名已存在"
	case errors.Is(err, appuser.ErrPhoneAlreadyRegistered):
		return http.StatusConflict, "该手机号已注册"
	default:
		return http.StatusInternalServerError, "注册失败"
	}
}

func appPasswordLoginErrorResponse(err error) (int, string) {
	switch {
	case errors.Is(err, appuser.ErrInvalidCredentials):
		return http.StatusUnauthorized, "账号或密码错误"
	case errors.Is(err, appuser.ErrUserDisabled):
		return http.StatusForbidden, "账号已被禁用"
	default:
		return http.StatusInternalServerError, "登录失败"
	}
}

func isSixDigitCode(code string) bool {
	if len(code) != 6 {
		return false
	}
	for i := 0; i < len(code); i++ {
		if code[i] < '0' || code[i] > '9' {
			return false
		}
	}
	return true
}

func (s *Server) allowAppPasswordLoginAttempt(ctx context.Context, identifier, ip string, now time.Time) bool {
	if ctx == nil {
		ctx = context.Background()
	}
	ipKey := strings.TrimSpace(ip)
	ipDBKey := appPasswordLoginPersistentKey(s.env.JWTSecret, "ip", ipKey)
	if !allowAppPasswordLoginDimension(ctx, s.appPasswordIPDBLimiter, s.appPasswordIPLimiter, ipDBKey, ipKey, now) {
		return false
	}
	accountKey := strings.ToLower(strings.TrimSpace(identifier))
	accountDBKey := appPasswordLoginPersistentKey(s.env.JWTSecret, "account", accountKey)
	if !allowAppPasswordLoginDimension(ctx, s.appPasswordAccountDBLimiter, s.appPasswordAccountLimiter, accountDBKey, accountKey, now) {
		return false
	}
	return true
}

func appPasswordLoginPersistentKey(secret, dimension, value string) string {
	normalized := strings.ToLower(strings.TrimSpace(value))
	if normalized == "" {
		return ""
	}
	mac := hmac.New(sha256.New, []byte(secret))
	_, _ = mac.Write([]byte("app-password-login:" + dimension + ":" + normalized))
	return "v1:" + hex.EncodeToString(mac.Sum(nil))
}

func allowAppPasswordLoginDimension(ctx context.Context, dbLimiter *dbRateLimiter, memoryLimiter *strRateLimiter, dbKey, memoryKey string, now time.Time) bool {
	if memoryKey == "" {
		return true
	}
	if dbLimiter != nil && dbLimiter.db != nil {
		dbCtx, cancel := context.WithTimeout(ctx, appPasswordLoginDBLimiterTimeout)
		allowed, err := dbLimiter.allow(dbCtx, dbKey, now)
		cancel()
		if err == nil {
			return allowed
		}
	}
	if memoryLimiter != nil {
		return memoryLimiter.Allow(memoryKey, now)
	}
	return true
}
