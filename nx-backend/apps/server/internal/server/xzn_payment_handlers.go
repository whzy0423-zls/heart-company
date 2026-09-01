package server

import (
	"context"
	"crypto/aes"
	"crypto/cipher"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"net/url"
	"os"
	"strings"

	"nine-xing/nx-backend/apps/server/internal/httpx"
	"nine-xing/nx-backend/apps/server/internal/xznpay"
)

const xznConfigKey = "xzn_payment"

type xznPaymentConfig struct {
	PID       string `json:"pid"`
	Secret    string `json:"secret"`
	BaseURL   string `json:"baseURL"`
	SignType  string `json:"signType"`
	NotifyURL string `json:"notifyURL"`
	ReturnURL string `json:"returnURL"`
	ChannelID string `json:"channelID"`
}

func (s *Server) xznPayConfig(w http.ResponseWriter, r *http.Request) {
	switch r.Method {
	case http.MethodGet:
		cfg, _ := s.loadXZNConfig(r.Context())
		httpx.OK(w, map[string]any{"configured": cfg.PID != "" && cfg.Secret != "", "pid": cfg.PID, "secretSet": cfg.Secret != "", "baseURL": cfg.BaseURL, "signType": cfg.SignType, "notifyURL": cfg.NotifyURL, "returnURL": cfg.ReturnURL, "channelID": cfg.ChannelID})
	case http.MethodPut:
		var input xznPaymentConfig
		if json.NewDecoder(http.MaxBytesReader(w, r.Body, 32<<10)).Decode(&input) != nil {
			httpx.Fail(w, http.StatusBadRequest, "Invalid JSON payload")
			return
		}
		current, _ := s.loadXZNConfig(r.Context())
		if strings.TrimSpace(input.Secret) == "" {
			input.Secret = current.Secret
		}
		input.PID, input.Secret = strings.TrimSpace(input.PID), strings.TrimSpace(input.Secret)
		input.BaseURL, input.SignType = strings.TrimRight(strings.TrimSpace(input.BaseURL), "/"), strings.ToUpper(strings.TrimSpace(input.SignType))
		if input.BaseURL == "" {
			input.BaseURL = "https://pay.xzncraft.cn/openapi"
		}
		if input.SignType == "" {
			input.SignType = "MD5"
		}
		if input.SignType != "MD5" && input.SignType != "RSA" && input.SignType != "MD5+RSA" {
			httpx.Fail(w, http.StatusBadRequest, "签名方式必须是 MD5、RSA 或 MD5+RSA")
			return
		}
		if input.PID == "" || input.Secret == "" {
			httpx.Fail(w, http.StatusBadRequest, "商户号和商户密钥不能为空")
			return
		}
		encrypted, err := encryptXZNSecret(input.Secret, s.env.JWTSecret)
		if err != nil {
			httpx.Fail(w, http.StatusInternalServerError, "保存密钥失败")
			return
		}
		stored := input
		stored.Secret = encrypted
		raw, _ := json.Marshal(stored)
		_, err = s.db.ExecContext(r.Context(), `INSERT INTO site_configs (key, config, update_time) VALUES ($1,$2::jsonb,now()) ON CONFLICT (key) DO UPDATE SET config=EXCLUDED.config, update_time=now()`, xznConfigKey, raw)
		if err != nil {
			httpx.Fail(w, http.StatusInternalServerError, "保存支付配置失败")
			return
		}
		httpx.OK(w, map[string]any{"configured": true})
	default:
		w.WriteHeader(http.StatusMethodNotAllowed)
	}
}

func (s *Server) xznPayCreate(w http.ResponseWriter, r *http.Request) {
	var in struct{ OutTradeNo, TotalAmount, Subject, PaytypeCode, ChannelID, Attach, ClientIP string }
	if json.NewDecoder(r.Body).Decode(&in) != nil || in.TotalAmount == "" || in.PaytypeCode == "" {
		httpx.Fail(w, 400, "totalAmount and paytypeCode are required")
		return
	}
	c, cfg, err := s.newXZNClient(r.Context())
	if err != nil {
		httpx.Fail(w, 503, "星之柠支付尚未配置，请先保存商户配置")
		return
	}
	channelID := strings.TrimSpace(in.ChannelID)
	if channelID == "" {
		channelID = cfg.ChannelID
	}
	v := url.Values{"out_trade_no": {in.OutTradeNo}, "total_amount": {in.TotalAmount}, "subject": {in.Subject}, "paytype_code": {in.PaytypeCode}, "channel_id": {channelID}, "attach": {in.Attach}, "client_ip": {in.ClientIP}, "notify_url": {cfg.NotifyURL}, "return_url": {cfg.ReturnURL}}
	out, err := c.Post("/pay/create", v)
	if err != nil {
		httpx.Fail(w, 502, err.Error())
		return
	}
	httpx.OK(w, out)
}

func (s *Server) loadXZNConfig(ctx context.Context) (xznPaymentConfig, error) {
	cfg := xznPaymentConfig{PID: os.Getenv("XZN_PID"), Secret: os.Getenv("XZN_KEY"), BaseURL: getenvDefault("XZN_API_BASE", "https://pay.xzncraft.cn/openapi"), SignType: getenvDefault("XZN_SIGN_TYPE", "MD5"), NotifyURL: os.Getenv("XZN_NOTIFY_URL"), ReturnURL: os.Getenv("XZN_RETURN_URL"), ChannelID: os.Getenv("XZN_CHANNEL_ID")}
	var raw []byte
	if err := s.db.QueryRowContext(ctx, `SELECT config FROM site_configs WHERE key=$1`, xznConfigKey).Scan(&raw); err != nil {
		return cfg, nil
	}
	if json.Unmarshal(raw, &cfg) != nil {
		return cfg, errors.New("invalid stored config")
	}
	secret, err := decryptXZNSecret(cfg.Secret, s.env.JWTSecret)
	if err != nil {
		return cfg, err
	}
	cfg.Secret = secret
	return cfg, nil
}

func (s *Server) newXZNClient(ctx context.Context) (*xznpay.Client, xznPaymentConfig, error) {
	cfg, err := s.loadXZNConfig(ctx)
	if err != nil || cfg.PID == "" || cfg.Secret == "" {
		return nil, cfg, errors.New("not configured")
	}
	requestSignType := cfg.SignType
	if requestSignType == "MD5+RSA" {
		// MD5+RSA 是商户后台的允许策略，API 单次请求仍须选择其中一种。
		requestSignType = "MD5"
	}
	return xznpay.New(xznpay.Config{BaseURL: cfg.BaseURL, PID: cfg.PID, Key: cfg.Secret, SignType: requestSignType}), cfg, nil
}

func encryptXZNSecret(value, key string) (string, error) {
	sum := sha256.Sum256([]byte(key))
	block, err := aes.NewCipher(sum[:])
	if err != nil {
		return "", err
	}
	gcm, err := cipher.NewGCM(block)
	if err != nil {
		return "", err
	}
	nonce := make([]byte, gcm.NonceSize())
	if _, err = io.ReadFull(rand.Reader, nonce); err != nil {
		return "", err
	}
	return base64.StdEncoding.EncodeToString(gcm.Seal(nonce, nonce, []byte(value), nil)), nil
}
func decryptXZNSecret(value, key string) (string, error) {
	raw, err := base64.StdEncoding.DecodeString(value)
	if err != nil {
		return "", err
	}
	sum := sha256.Sum256([]byte(key))
	block, err := aes.NewCipher(sum[:])
	if err != nil {
		return "", err
	}
	gcm, err := cipher.NewGCM(block)
	if err != nil || len(raw) < gcm.NonceSize() {
		return "", errors.New("invalid encrypted secret")
	}
	plain, err := gcm.Open(nil, raw[:gcm.NonceSize()], raw[gcm.NonceSize():], nil)
	return string(plain), err
}
func getenvDefault(k, d string) string {
	if v := strings.TrimSpace(os.Getenv(k)); v != "" {
		return v
	}
	return d
}
