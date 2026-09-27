package server

import (
	"context"
	"crypto/tls"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"log"
	"net"
	"net/http"
	"net/smtp"
	"strconv"
	"strings"
	"time"

	"nine-xing/nx-backend/apps/server/internal/appuser"
	"nine-xing/nx-backend/apps/server/internal/config"
	"nine-xing/nx-backend/apps/server/internal/httpx"
)

const appEmailConfigKey = "app_email_config"

type appEmailConfig struct {
	Enabled  bool   `json:"enabled"`
	Host     string `json:"host"`
	Port     int    `json:"port"`
	Username string `json:"username"`
	Password string `json:"password,omitempty"`
	From     string `json:"from"`
	FromName string `json:"fromName"`
}

func defaultAppEmailConfig() appEmailConfig {
	return appEmailConfig{Port: 587}
}

func normalizeAppEmail(raw string) string {
	return strings.ToLower(strings.TrimSpace(raw))
}

func validateAppEmail(raw string) error {
	return appuser.ValidateEmail(raw)
}

func (s *Server) loadAppEmailConfig(ctx context.Context) (appEmailConfig, error) {
	cfg := defaultAppEmailConfig()
	if s == nil || s.db == nil {
		return cfg, nil
	}
	var raw []byte
	err := s.db.QueryRowContext(ctx, `SELECT config FROM site_configs WHERE key=$1`, appEmailConfigKey).Scan(&raw)
	if errors.Is(err, sql.ErrNoRows) {
		return cfg, nil
	}
	if err != nil {
		return cfg, err
	}
	if err := json.Unmarshal(raw, &cfg); err != nil {
		return cfg, err
	}
	if cfg.Port == 0 {
		cfg.Port = 587
	}
	return cfg, nil
}

func (s *Server) saveAppEmailConfig(ctx context.Context, cfg appEmailConfig) error {
	cfg.Host = strings.TrimSpace(cfg.Host)
	cfg.Username = strings.TrimSpace(cfg.Username)
	cfg.From = strings.TrimSpace(cfg.From)
	cfg.FromName = strings.TrimSpace(cfg.FromName)
	if cfg.Port <= 0 || cfg.Port > 65535 {
		return errors.New("SMTP 端口不正确")
	}
	if cfg.Enabled && (cfg.Host == "" || cfg.From == "") {
		return errors.New("启用 SMTP 时必须填写服务器和发件人")
	}
	if cfg.From != "" {
		if err := validateAppEmail(cfg.From); err != nil {
			return errors.New("发件人邮箱格式不正确")
		}
	}
	raw, err := json.Marshal(cfg)
	if err != nil {
		return err
	}
	_, err = s.db.ExecContext(ctx, `
		INSERT INTO site_configs (key, config, update_time)
		VALUES ($1, $2::jsonb, now())
		ON CONFLICT (key) DO UPDATE SET config=EXCLUDED.config, update_time=now()
	`, appEmailConfigKey, string(raw))
	return err
}

func (s *Server) appEmailConfigHandler(w http.ResponseWriter, r *http.Request) {
	switch r.Method {
	case http.MethodGet:
		cfg, err := s.loadAppEmailConfig(r.Context())
		if err != nil {
			httpx.Fail(w, http.StatusInternalServerError, "读取 SMTP 配置失败")
			return
		}
		cfg.Password = ""
		httpx.OK(w, cfg)
	case http.MethodPut:
		var input appEmailConfig
		if err := json.NewDecoder(http.MaxBytesReader(w, r.Body, 8<<10)).Decode(&input); err != nil {
			httpx.Fail(w, http.StatusBadRequest, "配置格式不正确")
			return
		}
		if input.Password == "" {
			current, err := s.loadAppEmailConfig(r.Context())
			if err == nil {
				input.Password = current.Password
			}
		}
		if err := s.saveAppEmailConfig(r.Context(), input); err != nil {
			httpx.Fail(w, http.StatusBadRequest, err.Error())
			return
		}
		input.Password = ""
		httpx.OK(w, input)
	default:
		w.WriteHeader(http.StatusMethodNotAllowed)
	}
}

func (s *Server) appSendEmailCode(w http.ResponseWriter, r *http.Request) {
	var body struct {
		Email string `json:"email"`
	}
	if err := json.NewDecoder(http.MaxBytesReader(w, r.Body, 4<<10)).Decode(&body); err != nil {
		httpx.Fail(w, http.StatusBadRequest, "invalid request body")
		return
	}
	email := normalizeAppEmail(body.Email)
	if err := validateAppEmail(email); err != nil {
		httpx.Fail(w, http.StatusBadRequest, "邮箱格式不正确")
		return
	}
	now := time.Now()
	ip := s.clientIP(r)
	if s.smsIPLimiter != nil && !s.smsIPLimiter.Allow(ip, now) {
		httpx.Fail(w, http.StatusTooManyRequests, "发送过于频繁，请稍后再试")
		return
	}
	if s.smsPhoneLimiter != nil && !s.smsPhoneLimiter.Allow(email, now) {
		httpx.Fail(w, http.StatusTooManyRequests, "发送过于频繁，请稍后再试")
		return
	}
	if s.appUsers == nil || s.db == nil {
		httpx.Fail(w, http.StatusServiceUnavailable, "认证服务不可用")
		return
	}
	code, err := generateSMSCode()
	if err != nil {
		httpx.Fail(w, http.StatusInternalServerError, "验证码生成失败")
		return
	}
	eligible, err := s.appUsers.StoreEmailResetCodeIfEligible(r.Context(), email, appuser.HashToken(code), ip, now.Add(smsCodeExpiry))
	if err != nil {
		httpx.Fail(w, http.StatusInternalServerError, "验证码存储失败")
		return
	}
	if !eligible {
		httpx.Fail(w, http.StatusBadRequest, "该邮箱尚未绑定 App 用户")
		return
	}
	cfg, err := s.loadAppEmailConfig(r.Context())
	if err != nil {
		httpx.Fail(w, http.StatusInternalServerError, "读取 SMTP 配置失败")
		return
	}
	if !cfg.Enabled || cfg.Host == "" {
		if config.NormalizeAppEnv(s.env.AppEnv) == "dev" || config.NormalizeAppEnv(s.env.AppEnv) == "test" {
			httpx.OK(w, map[string]any{"devCode": code})
			return
		}
		httpx.Fail(w, http.StatusServiceUnavailable, "SMTP 尚未配置，请联系管理员")
		return
	}
	if err := sendConfiguredAppEmail(r.Context(), cfg, email, "芯之力密码找回验证码", fmt.Sprintf("你的密码找回验证码是：%s\n验证码 10 分钟内有效。如非本人操作，请忽略此邮件。", code)); err != nil {
		log.Printf("[EMAIL] send error email=%s: %v", email, err)
		httpx.Fail(w, http.StatusInternalServerError, "邮件发送失败")
		return
	}
	httpx.OK(w, nil)
}

func (s *Server) appResetPasswordEmail(w http.ResponseWriter, r *http.Request) {
	var body struct {
		Email    string `json:"email"`
		Code     string `json:"code"`
		Password string `json:"password"`
	}
	if err := json.NewDecoder(http.MaxBytesReader(w, r.Body, 8<<10)).Decode(&body); err != nil {
		httpx.Fail(w, http.StatusBadRequest, "invalid request body")
		return
	}
	email := normalizeAppEmail(body.Email)
	if err := validateAppEmail(email); err != nil || !isSixDigitCode(strings.TrimSpace(body.Code)) {
		httpx.Fail(w, http.StatusBadRequest, "邮箱或验证码格式不正确")
		return
	}
	if err := appuser.ValidatePassword(body.Password); err != nil {
		httpx.Fail(w, http.StatusBadRequest, "密码格式不正确")
		return
	}
	if s.appUsers == nil || s.db == nil {
		httpx.Fail(w, http.StatusServiceUnavailable, "认证服务不可用")
		return
	}
	if err := s.appUsers.ResetPasswordByEmail(r.Context(), email, appuser.HashToken(strings.TrimSpace(body.Code)), body.Password); err != nil {
		if errors.Is(err, appuser.ErrInvalidCredentials) {
			httpx.Fail(w, http.StatusUnauthorized, "验证码错误或已过期")
			return
		}
		httpx.Fail(w, http.StatusInternalServerError, "重置密码失败")
		return
	}
	httpx.OK(w, nil)
}

func sendConfiguredAppEmail(ctx context.Context, cfg appEmailConfig, recipient, subject, body string) error {
	if ctx == nil {
		ctx = context.Background()
	}
	address := net.JoinHostPort(cfg.Host, strconv.Itoa(cfg.Port))
	header := "From: " + cfg.From + "\r\n"
	if cfg.FromName != "" {
		header = fmt.Sprintf("From: %s <%s>\r\n", cfg.FromName, cfg.From)
	}
	message := []byte(header + "To: " + recipient + "\r\nSubject: " + subject + "\r\nContent-Type: text/plain; charset=UTF-8\r\n\r\n" + body)
	var auth smtp.Auth
	if cfg.Username != "" {
		auth = smtp.PlainAuth("", cfg.Username, cfg.Password, cfg.Host)
	}
	// smtp.SendMail negotiates STARTTLS when the server advertises it.
	if cfg.Port == 465 {
		return sendImplicitTLSAppEmail(cfg, address, recipient, message, auth)
	}
	return smtp.SendMail(address, auth, cfg.From, []string{recipient}, message)
}

func sendImplicitTLSAppEmail(cfg appEmailConfig, address, recipient string, message []byte, auth smtp.Auth) error {
	conn, err := tls.Dial("tcp", address, &tls.Config{ServerName: cfg.Host, MinVersion: tls.VersionTLS12})
	if err != nil {
		return err
	}
	client, err := smtp.NewClient(conn, cfg.Host)
	if err != nil {
		return err
	}
	defer client.Close()
	if auth != nil {
		if err := client.Auth(auth); err != nil {
			return err
		}
	}
	if err := client.Mail(cfg.From); err != nil {
		return err
	}
	if err := client.Rcpt(recipient); err != nil {
		return err
	}
	w, err := client.Data()
	if err != nil {
		return err
	}
	if _, err := w.Write(message); err != nil {
		return err
	}
	if err := w.Close(); err != nil {
		return err
	}
	return client.Quit()
}
