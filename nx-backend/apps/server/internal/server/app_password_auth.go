package server

import (
	"encoding/json"
	"errors"
	"net/http"
	"strings"
	"time"

	"nine-xing/nx-backend/apps/server/internal/appuser"
	"nine-xing/nx-backend/apps/server/internal/httpx"
)

const appPasswordAuthBodyLimit = 8 * 1024

func (s *Server) appRegisterWithPassword(w http.ResponseWriter, r *http.Request) {
	r.Body = http.MaxBytesReader(w, r.Body, appPasswordAuthBodyLimit)
	var body struct {
		Nickname   string `json:"nickname"`
		Account    string `json:"account"`
		Password   string `json:"password"`
		Phone      string `json:"phone"`
		Code       string `json:"code"`
		DeviceInfo string `json:"deviceInfo"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
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

	s.writeAppSession(w, r, user, deviceInfo)
}

func (s *Server) appLoginWithPassword(w http.ResponseWriter, r *http.Request) {
	r.Body = http.MaxBytesReader(w, r.Body, appPasswordAuthBodyLimit)
	var body struct {
		Account    string `json:"account"`
		Password   string `json:"password"`
		DeviceInfo string `json:"deviceInfo"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		httpx.Fail(w, http.StatusBadRequest, "invalid request body")
		return
	}

	identifier := strings.TrimSpace(body.Account)
	deviceInfo := strings.TrimSpace(body.DeviceInfo)
	if identifier == "" || body.Password == "" {
		httpx.Fail(w, http.StatusBadRequest, "请输入账号和密码")
		return
	}
	if !s.allowAppPasswordLoginAttempt(identifier, s.clientIP(r), time.Now()) {
		httpx.Fail(w, http.StatusTooManyRequests, "登录尝试过于频繁，请稍后再试")
		return
	}

	user, err := s.appUsers.AuthenticateWithPassword(r.Context(), identifier, body.Password)
	if err != nil {
		status, message := appPasswordLoginErrorResponse(err)
		httpx.Fail(w, status, message)
		return
	}

	s.writeAppSession(w, r, user, deviceInfo)
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

func (s *Server) allowAppPasswordLoginAttempt(identifier, ip string, now time.Time) bool {
	accountKey := strings.ToLower(strings.TrimSpace(identifier))
	if s.appPasswordAccountLimiter != nil && !s.appPasswordAccountLimiter.Allow(accountKey, now) {
		return false
	}
	ipKey := strings.TrimSpace(ip)
	if s.appPasswordIPLimiter != nil && !s.appPasswordIPLimiter.Allow(ipKey, now) {
		return false
	}
	return true
}
