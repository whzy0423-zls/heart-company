package server

import (
	"context"
	"crypto/aes"
	"crypto/cipher"
	"crypto/rand"
	"crypto/sha256"
	"database/sql"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log"
	"net/http"
	"net/url"
	"os"
	"strconv"
	"strings"
	"time"

	"nine-xing/nx-backend/apps/server/internal/auditlog"
	"nine-xing/nx-backend/apps/server/internal/httpx"
	"nine-xing/nx-backend/apps/server/internal/xznpay"
)

const xznConfigKey = "xzn_payment"

type xznPaymentConfig struct {
	PID             string `json:"pid"`
	Secret          string `json:"secret"`
	BaseURL         string `json:"baseURL"`
	SignType        string `json:"signType"`
	NotifyURL       string `json:"notifyURL"`
	ReturnURL       string `json:"returnURL"`
	ChannelID       string `json:"channelID"`
	Enabled         bool   `json:"enabled"`
	AlipayEnabled   bool   `json:"alipayEnabled"`
	WechatEnabled   bool   `json:"wechatEnabled"`
	AlipayGatewayID string `json:"alipayGatewayId"`
	WechatGatewayID string `json:"wechatGatewayId"`
}

func (s *Server) xznPayConfig(w http.ResponseWriter, r *http.Request) {
	switch r.Method {
	case http.MethodGet:
		cfg, _ := s.loadXZNConfig(r.Context())
		httpx.OK(w, xznConfigResponse(cfg))
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
		input.ChannelID = strings.TrimSpace(input.ChannelID)
		input.NotifyURL = strings.TrimSpace(input.NotifyURL)
		input.ReturnURL = strings.TrimSpace(input.ReturnURL)
		input.AlipayGatewayID = strings.TrimSpace(input.AlipayGatewayID)
		input.WechatGatewayID = strings.TrimSpace(input.WechatGatewayID)
		if input.BaseURL == "" {
			input.BaseURL = "https://pay.xzncraft.cn/openapi"
		}
		if input.SignType == "" {
			input.SignType = "MD5"
		}
		if err := validateXZNSignType(input.SignType); err != nil {
			httpx.Fail(w, http.StatusBadRequest, "当前支付接口仅支持 MD5 签名")
			return
		}
		if err := validateXZNBaseURL(input.BaseURL); err != nil {
			httpx.Fail(w, http.StatusBadRequest, "支付平台地址不在允许范围内")
			return
		}
		if input.PID == "" || input.Secret == "" {
			httpx.Fail(w, http.StatusBadRequest, "商户号和商户密钥不能为空")
			return
		}
		if input.Enabled && input.AlipayEnabled && input.AlipayGatewayID == "" {
			httpx.Fail(w, http.StatusBadRequest, "请填写支付宝 App 网关 ID")
			return
		}
		if input.Enabled && input.WechatEnabled && input.WechatGatewayID == "" {
			httpx.Fail(w, http.StatusBadRequest, "请填写微信 App 网关 ID")
			return
		}
		if input.WechatEnabled && isXZNAppBlockedWechatGateway(input.WechatGatewayID) {
			httpx.Fail(w, http.StatusBadRequest, "网关 3 和网关 31 不能作为 App 内微信支付网关")
			return
		}
		notifyURL, err := url.ParseRequestURI(input.NotifyURL)
		if err != nil || notifyURL.Scheme != "https" || notifyURL.Host == "" {
			httpx.Fail(w, http.StatusBadRequest, "异步回调地址必须是公网 HTTPS 地址")
			return
		}
		if err := validateXZNReturnURL(input.ReturnURL); err != nil {
			httpx.Fail(w, http.StatusBadRequest, "同步返回地址格式不正确")
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
		s.recordAdminAudit(r, auditlog.Entry{
			Action:     "xzn_payment_config.update",
			TargetType: "xzn_payment_config",
			TargetID:   "global",
			Before:     xznConfigResponse(current),
			After:      xznConfigResponse(input),
			Summary:    "更新星之柠支付配置",
		})
		httpx.OK(w, map[string]any{"configured": true})
	default:
		w.WriteHeader(http.StatusMethodNotAllowed)
	}
}

func xznConfigResponse(cfg xznPaymentConfig) map[string]any {
	configured := xznCredentialsConfigured(cfg)
	return map[string]any{
		"configured":      configured,
		"pid":             cfg.PID,
		"secretSet":       cfg.Secret != "",
		"baseURL":         cfg.BaseURL,
		"signType":        cfg.SignType,
		"notifyURL":       cfg.NotifyURL,
		"returnURL":       cfg.ReturnURL,
		"channelID":       cfg.ChannelID,
		"enabled":         cfg.Enabled,
		"alipayEnabled":   cfg.AlipayEnabled,
		"wechatEnabled":   cfg.WechatEnabled,
		"alipayGatewayId": cfg.AlipayGatewayID,
		"wechatGatewayId": cfg.WechatGatewayID,
	}
}

func isXZNAppBlockedWechatGateway(gatewayID string) bool {
	return gatewayID == "3" || gatewayID == "31"
}

// validateXZNBaseURL prevents the payment configuration from turning the
// server into an arbitrary outbound HTTP proxy. The production endpoint is
// fixed; loopback HTTP is retained for local integration tests only.
func validateXZNBaseURL(raw string) error {
	u, err := url.Parse(strings.TrimSpace(raw))
	if err != nil || u.Scheme == "" || u.Host == "" || u.User != nil || u.RawQuery != "" || u.Fragment != "" {
		return errors.New("invalid xzn base url")
	}
	u.Path = strings.TrimRight(u.Path, "/")
	if u.Path != "/openapi" {
		return errors.New("invalid xzn base path")
	}
	host := strings.ToLower(u.Hostname())
	if host == "pay.xzncraft.cn" {
		if u.Scheme != "https" || (u.Port() != "" && u.Port() != "443") {
			return errors.New("invalid xzn production endpoint")
		}
		return nil
	}
	if (host == "127.0.0.1" || host == "localhost" || host == "::1") && u.Scheme == "http" {
		return nil
	}
	return errors.New("xzn endpoint is not allowlisted")
}

func validateXZNSignType(signType string) error {
	if !strings.EqualFold(strings.TrimSpace(signType), "MD5") {
		return errors.New("xzn only supports MD5 signatures")
	}
	return nil
}

func validateXZNReturnURL(raw string) error {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return nil
	}
	if strings.ContainsAny(raw, "\r\n") {
		return errors.New("invalid xzn return url")
	}
	u, err := url.Parse(raw)
	if err != nil || u.Scheme == "" || u.User != nil || urlComponentHasControl(u) {
		return errors.New("invalid xzn return url")
	}
	scheme := strings.ToLower(u.Scheme)
	if scheme != "http" && scheme != "https" && scheme != "ninexing" {
		return errors.New("unsupported xzn return scheme")
	}
	if (scheme == "http" || scheme == "https") && u.Host == "" {
		return errors.New("xzn return url host is required")
	}
	return nil
}

const xznURLControlChars = "\x00\x01\x02\x03\x04\x05\x06\x07\x08\x09\x0a\x0b\x0c\x0d\x0e\x0f\x10\x11\x12\x13\x14\x15\x16\x17\x18\x19\x1a\x1b\x1c\x1d\x1e\x1f\x7f"

// urlComponentHasControl checks both decoded and escaped URL components. Go's
// net/url keeps an escaped newline in RawPath/RawQuery while exposing the
// decoded value separately, so checking only the raw fields would allow
// control characters such as "%0a" through configuration validation.
func urlComponentHasControl(u *url.URL) bool {
	if u == nil {
		return true
	}
	components := []string{u.Path, u.RawPath, u.RawQuery, u.Fragment}
	for _, component := range components {
		if strings.ContainsAny(component, xznURLControlChars) {
			return true
		}
	}
	path, err := url.PathUnescape(u.EscapedPath())
	if err != nil || strings.ContainsAny(path, xznURLControlChars) {
		return true
	}
	query, err := url.QueryUnescape(u.RawQuery)
	if err != nil || strings.ContainsAny(query, xznURLControlChars) {
		return true
	}
	fragment, err := url.PathUnescape(u.Fragment)
	return err != nil || strings.ContainsAny(fragment, xznURLControlChars)
}

func xznOrderReturnURL(raw, outTradeNo string) string {
	raw = strings.TrimSpace(raw)
	outTradeNo = strings.TrimSpace(outTradeNo)
	if raw == "" || outTradeNo == "" {
		return ""
	}
	u, err := url.Parse(raw)
	if err != nil || u.Scheme == "" || u.User != nil || strings.ContainsAny(raw, "\r\n") || urlComponentHasControl(u) {
		return ""
	}
	if (u.Scheme == "http" || u.Scheme == "https") && u.Host == "" {
		return ""
	}
	query := u.Query()
	query.Set("payment_return", "1")
	query.Set("outTradeNo", outTradeNo)
	u.RawQuery = query.Encode()
	return u.String()
}

func (s *Server) xznPayCreate(w http.ResponseWriter, r *http.Request) {
	var in struct{ OutTradeNo, TotalAmount, Subject, PaytypeCode, ChannelID, Attach string }
	if json.NewDecoder(r.Body).Decode(&in) != nil || strings.TrimSpace(in.OutTradeNo) == "" || strings.TrimSpace(in.TotalAmount) == "" || strings.TrimSpace(in.Subject) == "" || strings.TrimSpace(in.PaytypeCode) == "" {
		httpx.Fail(w, 400, "outTradeNo、totalAmount、subject 和 paytypeCode 不能为空")
		return
	}
	c, cfg, err := s.newXZNClient(r.Context())
	if err != nil {
		httpx.Fail(w, 503, "星之柠支付尚未配置，请先保存商户配置")
		return
	}
	paytypeCode := normalizeXZNCreatePaytypeCode(in.PaytypeCode)
	if paytypeCode == "" {
		httpx.Fail(w, http.StatusBadRequest, "paytypeCode 格式不正确")
		return
	}
	channelID := strings.TrimSpace(in.ChannelID)
	if channelID == "" {
		channelID = cfg.ChannelID
	}
	clientIP := strings.TrimSpace(s.clientIP(r))
	if clientIP == "" {
		clientIP = "127.0.0.1"
	}
	v := url.Values{"out_trade_no": {strings.TrimSpace(in.OutTradeNo)}, "total_amount": {strings.TrimSpace(in.TotalAmount)}, "subject": {strings.TrimSpace(in.Subject)}, "paytype_code": {paytypeCode}, "channel_id": {channelID}, "attach": {in.Attach}, "client_ip": {clientIP}, "notify_url": {cfg.NotifyURL}, "return_url": {xznOrderReturnURL(cfg.ReturnURL, in.OutTradeNo)}}
	out, err := c.Post("/pay/create", v)
	if err != nil {
		httpx.Fail(w, 502, err.Error())
		return
	}
	if code, ok := out["code"].(float64); ok && code <= 0 {
		message, _ := out["msg"].(string)
		if strings.TrimSpace(message) == "" {
			message = "星之柠下单失败"
		}
		httpx.Fail(w, http.StatusBadGateway, message)
		return
	}
	httpx.OK(w, out)
}

// normalizeXZNCreatePaytypeCode is used by the admin/debug checkout, which
// still supports provider channels outside the App catalog (for example the
// legacy Douyin gateway). App orders use normalizeAppPayChannel instead and
// remain restricted to Alipay and WeChat.
func normalizeXZNCreatePaytypeCode(value string) string {
	value = strings.ToLower(strings.TrimSpace(value))
	switch value {
	case "wechat", "weixin", "wechatpay":
		return "wxpay"
	case "alipay", "ali_pay", "ali":
		return "alipay"
	case "wxpay", "douyinpay":
		return value
	}
	if value == "" || len(value) > 32 || value[0] < 'a' || value[0] > 'z' {
		return ""
	}
	for _, char := range value[1:] {
		if (char < 'a' || char > 'z') && (char < '0' || char > '9') && char != '_' && char != '-' {
			return ""
		}
	}
	return value
}

func (s *Server) xznPayNotify(w http.ResponseWriter, r *http.Request) {
	cfg, err := s.loadXZNConfig(r.Context())
	if err != nil || cfg.Secret == "" {
		xznNotifyFail(w, http.StatusServiceUnavailable)
		return
	}
	r.Body = http.MaxBytesReader(w, r.Body, 32<<10)
	if err := r.ParseForm(); err != nil {
		xznNotifyFail(w, http.StatusBadRequest)
		return
	}
	if !strings.EqualFold(strings.TrimSpace(r.Form.Get("sign_type")), "MD5") ||
		!xznpay.VerifyMD5(r.Form, cfg.Secret, r.Form.Get("sign")) {
		xznNotifyFail(w, http.StatusUnauthorized)
		return
	}
	if strings.TrimSpace(r.Form.Get("pid")) != strings.TrimSpace(cfg.PID) {
		xznNotifyFail(w, http.StatusUnauthorized)
		return
	}
	callback, err := parseXZNCallback(r.Form)
	if err != nil {
		xznNotifyFail(w, http.StatusBadRequest)
		return
	}
	if err := s.applyXZNCallback(r.Context(), cfg, callback); err != nil {
		status := http.StatusBadRequest
		if errors.Is(err, errXZNCallbackNotFound) {
			status = http.StatusNotFound
		} else if errors.Is(err, errXZNCallbackDatabase) {
			status = http.StatusInternalServerError
		}
		xznNotifyFail(w, status)
		return
	}
	log.Printf("[XZNPAY] callback trade_no=%s out_trade_no=%s status=%s", callback.TradeNo, callback.OutTradeNo, callback.TradeStatus)
	w.Header().Set("Content-Type", "text/plain; charset=utf-8")
	w.WriteHeader(http.StatusOK)
	_, _ = w.Write([]byte("success"))
}

var (
	errXZNCallbackNotFound = errors.New("xzn callback order not found")
	errXZNCallbackDatabase = errors.New("xzn callback database error")
)

type xznCallback struct {
	PID           string
	TradeNo       string
	OutTradeNo    string
	TotalAmount   string
	TotalCents    int64
	Subject       string
	PaytypeCode   string
	ChannelID     string
	Attach        string
	TradeStatus   string
	TransactionID string
}

func parseXZNCallback(values url.Values) (xznCallback, error) {
	callback := xznCallback{
		PID:           strings.TrimSpace(values.Get("pid")),
		TradeNo:       strings.TrimSpace(values.Get("trade_no")),
		OutTradeNo:    strings.TrimSpace(values.Get("out_trade_no")),
		TotalAmount:   strings.TrimSpace(values.Get("total_amount")),
		Subject:       strings.TrimSpace(values.Get("subject")),
		PaytypeCode:   normalizeAppPayChannel(values.Get("paytype_code")),
		ChannelID:     strings.TrimSpace(values.Get("channel_id")),
		Attach:        strings.TrimSpace(values.Get("attach")),
		TradeStatus:   strings.ToUpper(strings.TrimSpace(values.Get("trade_status"))),
		TransactionID: strings.TrimSpace(values.Get("transaction_id")),
	}
	if callback.TradeNo == "" || callback.OutTradeNo == "" || callback.TotalAmount == "" ||
		callback.Subject == "" || callback.PaytypeCode == "" || callback.ChannelID == "" || callback.TradeStatus == "" {
		return xznCallback{}, errors.New("xzn callback is missing required fields")
	}
	switch callback.TradeStatus {
	case "WAIT_BUYER_PAY", "TRADE_SUCCESS", "TRADE_CLOSED", "TRADE_REFUND", "TRADE_FREEZE", "TRADE_UNFREEZE":
	default:
		return xznCallback{}, errors.New("xzn callback has unsupported trade status")
	}
	var err error
	callback.TotalCents, err = xznpay.ParseYuanToCents(callback.TotalAmount)
	if err != nil {
		return xznCallback{}, err
	}
	return callback, nil
}

func (s *Server) applyXZNCallback(ctx context.Context, cfg xznPaymentConfig, callback xznCallback) error {
	tx, err := s.db.BeginTx(ctx, &sql.TxOptions{Isolation: sql.LevelSerializable})
	if err != nil {
		return fmt.Errorf("%w: begin: %v", errXZNCallbackDatabase, err)
	}
	defer tx.Rollback()
	var orderID, appUserID int64
	var amount int
	var productID, title, status, provider, payChannel, gatewayID, providerTradeNo string
	err = tx.QueryRowContext(ctx, `
		SELECT id, app_user_id, product_id, title, amount, status,
		       COALESCE(payment_provider,''), COALESCE(pay_channel,''), COALESCE(gateway_id,''),
		       COALESCE(provider_trade_no,'')
		FROM app_orders WHERE out_trade_no=$1 FOR UPDATE`, callback.OutTradeNo).Scan(
		&orderID, &appUserID, &productID, &title, &amount, &status,
		&provider, &payChannel, &gatewayID, &providerTradeNo,
	)
	if err == sql.ErrNoRows {
		return errXZNCallbackNotFound
	}
	if err != nil {
		return fmt.Errorf("%w: order: %v", errXZNCallbackDatabase, err)
	}
	if provider != "xzn" || normalizeAppPayChannel(payChannel) != callback.PaytypeCode || gatewayID != callback.ChannelID ||
		providerTradeNo != "" && providerTradeNo != callback.TradeNo || int64(amount) != callback.TotalCents ||
		(strings.TrimSpace(title) != "" && title != callback.Subject) {
		return errors.New("xzn callback order details do not match")
	}
	if _, err := tx.ExecContext(ctx, `
		UPDATE app_orders
		SET provider_trade_no=$2, provider_status=$3, transaction_id=CASE WHEN $4<>'' THEN $4 ELSE transaction_id END,
		    payment_error='', update_time=now()
		WHERE id=$1`, orderID, callback.TradeNo, callback.TradeStatus, callback.TransactionID); err != nil {
		return fmt.Errorf("%w: metadata: %v", errXZNCallbackDatabase, err)
	}
	if callback.TradeStatus != "TRADE_SUCCESS" {
		if _, err := tx.ExecContext(ctx, `UPDATE app_orders SET status=$2, update_time=now() WHERE id=$1 AND status <> 'paid'`, orderID, xznLocalStatus(callback.TradeStatus)); err != nil {
			return fmt.Errorf("%w: state: %v", errXZNCallbackDatabase, err)
		}
		if err := tx.Commit(); err != nil {
			return fmt.Errorf("%w: commit: %v", errXZNCallbackDatabase, err)
		}
		return nil
	}
	if _, err := settleAppOrderTx(ctx, tx, appOrderSettlementInput{
		OrderID:        orderID,
		ActivationAt:   time.Now(),
		ProviderTrade:  callback.TradeNo,
		ProviderStatus: callback.TradeStatus,
		TransactionID:  callback.TransactionID,
	}); err != nil {
		return err
	}
	if err := tx.Commit(); err != nil {
		return fmt.Errorf("%w: commit: %v", errXZNCallbackDatabase, err)
	}
	return nil
}

func xznNotifyFail(w http.ResponseWriter, status int) {
	w.Header().Set("Content-Type", "text/plain; charset=utf-8")
	w.WriteHeader(status)
	_, _ = w.Write([]byte("fail"))
}

func xznLocalStatus(providerStatus string) string {
	switch strings.ToUpper(strings.TrimSpace(providerStatus)) {
	case "WAIT_BUYER_PAY":
		return "pending"
	case "TRADE_FREEZE":
		return "paying"
	case "TRADE_UNFREEZE":
		return "pending"
	case "TRADE_CLOSED", "TRADE_REFUND":
		return "closed"
	case "TRADE_SUCCESS":
		return "paid"
	default:
		return "failed"
	}
}

func (s *Server) loadXZNConfig(ctx context.Context) (xznPaymentConfig, error) {
	cfg := xznPaymentConfig{
		PID:             os.Getenv("XZN_PID"),
		Secret:          os.Getenv("XZN_KEY"),
		BaseURL:         getenvDefault("XZN_API_BASE", "https://pay.xzncraft.cn/openapi"),
		SignType:        getenvDefault("XZN_SIGN_TYPE", "MD5"),
		NotifyURL:       os.Getenv("XZN_NOTIFY_URL"),
		ReturnURL:       os.Getenv("XZN_RETURN_URL"),
		ChannelID:       os.Getenv("XZN_CHANNEL_ID"),
		Enabled:         parseEnvBool("XZN_ENABLED"),
		AlipayEnabled:   parseEnvBool("XZN_ALIPAY_ENABLED"),
		WechatEnabled:   parseEnvBool("XZN_WECHAT_ENABLED"),
		AlipayGatewayID: getenvDefault("XZN_ALIPAY_GATEWAY_ID", "34"),
		WechatGatewayID: os.Getenv("XZN_WECHAT_GATEWAY_ID"),
	}
	if s == nil || s.db == nil {
		return cfg, nil
	}
	var raw []byte
	if err := s.db.QueryRowContext(ctx, `SELECT config FROM site_configs WHERE key=$1`, xznConfigKey).Scan(&raw); err != nil {
		return cfg, nil
	}
	if json.Unmarshal(raw, &cfg) != nil {
		return cfg, errors.New("invalid stored config")
	}
	if cfg.Secret == "" {
		return cfg, nil
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
	if err != nil || cfg.PID == "" || cfg.Secret == "" || !cfg.Enabled || validateXZNBaseURL(cfg.BaseURL) != nil {
		return nil, cfg, errors.New("not configured")
	}
	if validateXZNSignType(cfg.SignType) != nil {
		return nil, cfg, errors.New("xzn only supports MD5 signatures")
	}
	return xznpay.New(xznpay.Config{BaseURL: cfg.BaseURL, PID: cfg.PID, Key: cfg.Secret, SignType: "MD5"}), cfg, nil
}

func parseEnvBool(name string) bool {
	value, err := strconv.ParseBool(strings.TrimSpace(os.Getenv(name)))
	return err == nil && value
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
