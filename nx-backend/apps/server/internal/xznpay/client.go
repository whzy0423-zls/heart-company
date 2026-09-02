package xznpay

import (
	"context"
	"crypto"
	"crypto/md5"
	"crypto/rand"
	"crypto/rsa"
	"crypto/sha256"
	"crypto/subtle"
	"crypto/x509"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"encoding/pem"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"sort"
	"strconv"
	"strings"
	"time"
)

const maxResponseBodyBytes = 1 << 20

type Config struct {
	BaseURL, PID, Key, SignType string
	PrivateKey                  *rsa.PrivateKey
	NotifyURL, ReturnURL        string
}

type HTTPClient interface {
	Do(*http.Request) (*http.Response, error)
}

type Option func(*Client)

func WithHTTPClient(client HTTPClient) Option {
	return func(c *Client) {
		if client != nil {
			c.http = client
		}
	}
}

func WithClock(clock func() time.Time) Option {
	return func(c *Client) {
		if clock != nil {
			c.now = clock
		}
	}
}

type Client struct {
	cfg  Config
	http HTTPClient
	now  func() time.Time
}

func New(cfg Config, options ...Option) *Client {
	client := &Client{
		cfg:  cfg,
		http: &http.Client{Timeout: 20 * time.Second},
		now:  time.Now,
	}
	for _, option := range options {
		option(client)
	}
	return client
}

type CreateRequest struct {
	OutTradeNo  string
	TotalAmount string
	Subject     string
	PaytypeCode string
	ChannelID   string
	Attach      string
	ClientIP    string
	NotifyURL   string
	ReturnURL   string
}

type CreateResult struct {
	TradeNo    string
	OutTradeNo string
	PayURL     string
}

type QueryRequest struct {
	TradeNo    string
	OutTradeNo string
}

type QueryResult struct {
	TradeNo          string
	OutTradeNo       string
	TotalAmount      string
	TotalAmountCents int64
	TradeStatus      string
}

func VerifyMD5(values url.Values, key, signature string) bool {
	keys := make([]string, 0, len(values))
	for name := range values {
		if name != "sign" && name != "sign_type" {
			keys = append(keys, name)
		}
	}
	sort.Strings(keys)
	parts := make([]string, 0, len(keys))
	for _, name := range keys {
		if value := values.Get(name); value != "" {
			parts = append(parts, name+"="+value)
		}
	}
	digest := md5.Sum([]byte(strings.Join(parts, "&") + "&key=" + key))
	expected := strings.ToUpper(hex.EncodeToString(digest[:]))
	actual := strings.ToUpper(strings.TrimSpace(signature))
	return subtle.ConstantTimeCompare([]byte(expected), []byte(actual)) == 1
}
func ParsePrivateKey(raw string) (*rsa.PrivateKey, error) {
	b, _ := pem.Decode([]byte(raw))
	if b == nil {
		return nil, errors.New("invalid RSA private key")
	}
	k, e := x509.ParsePKCS8PrivateKey(b.Bytes)
	if e != nil {
		if k2, e2 := x509.ParsePKCS1PrivateKey(b.Bytes); e2 == nil {
			return k2, nil
		}
	}
	if e != nil {
		return nil, e
	}
	k2, ok := k.(*rsa.PrivateKey)
	if !ok {
		return nil, errors.New("private key is not RSA")
	}
	return k2, nil
}
func (c *Client) sign(v url.Values) string {
	keys := make([]string, 0, len(v))
	for k := range v {
		if k != "sign" && k != "sign_type" {
			keys = append(keys, k)
		}
	}
	sort.Strings(keys)
	p := make([]string, 0, len(keys))
	for _, k := range keys {
		if v.Get(k) != "" {
			p = append(p, k+"="+v.Get(k))
		}
	}
	raw := strings.Join(p, "&")
	if strings.EqualFold(c.cfg.SignType, "RSA") && c.cfg.PrivateKey != nil {
		digest := sha256.Sum256([]byte(raw))
		s, _ := rsa.SignPKCS1v15(rand.Reader, c.cfg.PrivateKey, crypto.SHA256, digest[:])
		return base64.StdEncoding.EncodeToString(s)
	}
	h := md5.Sum([]byte(raw + "&key=" + c.cfg.Key))
	return strings.ToUpper(hex.EncodeToString(h[:]))
}
func (c *Client) Post(path string, fields url.Values) (map[string]any, error) {
	return c.post(context.Background(), path, fields)
}

func (c *Client) Create(ctx context.Context, request CreateRequest) (CreateResult, error) {
	outTradeNo := strings.TrimSpace(request.OutTradeNo)
	clientIP := strings.TrimSpace(request.ClientIP)
	notifyURL := strings.TrimSpace(request.NotifyURL)
	if outTradeNo == "" || strings.TrimSpace(request.Subject) == "" || strings.TrimSpace(request.PaytypeCode) == "" || clientIP == "" || notifyURL == "" {
		return CreateResult{}, errors.New("xzn create request is missing required fields")
	}
	cents, err := ParseYuanToCents(request.TotalAmount)
	if err != nil {
		return CreateResult{}, fmt.Errorf("xzn create total_amount: %w", err)
	}
	result, err := c.post(ctx, "/pay/create", url.Values{
		"out_trade_no": {outTradeNo},
		"total_amount": {formatCents(cents)},
		"subject":      {strings.TrimSpace(request.Subject)},
		"paytype_code": {strings.TrimSpace(request.PaytypeCode)},
		"channel_id":   {strings.TrimSpace(request.ChannelID)},
		"attach":       {request.Attach},
		"client_ip":    {clientIP},
		"notify_url":   {notifyURL},
		"return_url":   {strings.TrimSpace(request.ReturnURL)},
	})
	if err != nil {
		return CreateResult{}, err
	}
	data, err := responseData(result)
	if err != nil {
		return CreateResult{}, err
	}
	created := CreateResult{
		TradeNo:    responseString(data, "trade_no"),
		OutTradeNo: responseString(data, "out_trade_no"),
		PayURL:     responseString(data, "pay_url"),
	}
	if created.TradeNo == "" || created.OutTradeNo == "" || created.PayURL == "" {
		return CreateResult{}, errors.New("xzn create response is missing required data")
	}
	if created.OutTradeNo != outTradeNo {
		return CreateResult{}, errors.New("xzn create response out_trade_no does not match request")
	}
	return created, nil
}

func (c *Client) Query(ctx context.Context, request QueryRequest) (QueryResult, error) {
	tradeNo := strings.TrimSpace(request.TradeNo)
	outTradeNo := strings.TrimSpace(request.OutTradeNo)
	if tradeNo == "" && outTradeNo == "" {
		return QueryResult{}, errors.New("xzn query request requires trade_no or out_trade_no")
	}
	result, err := c.post(ctx, "/pay/query", url.Values{
		"trade_no":     {tradeNo},
		"out_trade_no": {outTradeNo},
	})
	if err != nil {
		return QueryResult{}, err
	}
	data, err := responseData(result)
	if err != nil {
		return QueryResult{}, err
	}
	queried := QueryResult{
		TradeNo:     responseString(data, "trade_no"),
		OutTradeNo:  responseString(data, "out_trade_no"),
		TotalAmount: responseString(data, "total_amount"),
		TradeStatus: responseString(data, "trade_status"),
	}
	if queried.TradeNo == "" || queried.OutTradeNo == "" || queried.TotalAmount == "" || queried.TradeStatus == "" {
		return QueryResult{}, errors.New("xzn query response is missing required data")
	}
	if tradeNo != "" && queried.TradeNo != tradeNo {
		return QueryResult{}, errors.New("xzn query response trade_no does not match request")
	}
	if outTradeNo != "" && queried.OutTradeNo != outTradeNo {
		return QueryResult{}, errors.New("xzn query response out_trade_no does not match request")
	}
	queried.TotalAmountCents, err = ParseYuanToCents(queried.TotalAmount)
	if err != nil {
		return QueryResult{}, fmt.Errorf("xzn query total_amount: %w", err)
	}
	queried.TotalAmount = formatCents(queried.TotalAmountCents)
	return queried, nil
}

func ParseYuanToCents(amount string) (int64, error) {
	if amount == "" || strings.TrimSpace(amount) != amount || strings.HasPrefix(amount, "+") || strings.HasPrefix(amount, "-") {
		return 0, errors.New("amount must be a non-negative decimal")
	}
	parts := strings.Split(amount, ".")
	if len(parts) > 2 || parts[0] == "" || (len(parts) == 2 && (parts[1] == "" || len(parts[1]) > 2)) {
		return 0, errors.New("amount must have at most two decimal places")
	}
	for _, part := range parts {
		for _, char := range part {
			if char < '0' || char > '9' {
				return 0, errors.New("amount must contain decimal digits only")
			}
		}
	}
	whole, err := strconv.ParseInt(parts[0], 10, 64)
	if err != nil || whole > (1<<63-1)/100 {
		return 0, errors.New("amount is too large")
	}
	fraction := int64(0)
	if len(parts) == 2 {
		fraction = int64(parts[1][0]-'0') * 10
		if len(parts[1]) == 2 {
			fraction += int64(parts[1][1] - '0')
		}
	}
	if whole == (1<<63-1)/100 && fraction > (1<<63-1)%100 {
		return 0, errors.New("amount is too large")
	}
	return whole*100 + fraction, nil
}

func formatCents(cents int64) string {
	return strconv.FormatInt(cents/100, 10) + "." + fmt.Sprintf("%02d", cents%100)
}

func (c *Client) post(ctx context.Context, path string, fields url.Values) (map[string]any, error) {
	fields = cloneValues(fields)
	fields.Set("pid", c.cfg.PID)
	fields.Set("timestamp", strconv.FormatInt(c.now().Unix(), 10))
	fields.Set("sign_type", strings.ToUpper(c.cfg.SignType))
	fields.Set("sign", c.sign(fields))
	req, e := http.NewRequestWithContext(ctx, http.MethodPost, strings.TrimRight(c.cfg.BaseURL, "/")+path, strings.NewReader(fields.Encode()))
	if e != nil {
		return nil, e
	}
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	resp, e := c.http.Do(req)
	if e != nil {
		return nil, e
	}
	defer resp.Body.Close()
	b, readErr := io.ReadAll(io.LimitReader(resp.Body, maxResponseBodyBytes+1))
	if readErr != nil {
		return nil, fmt.Errorf("read xzn response: %w", readErr)
	}
	if len(b) > maxResponseBodyBytes {
		return nil, errors.New("xzn response body is too large")
	}
	if resp.StatusCode/100 != 2 {
		return nil, fmt.Errorf("xzn http %d: %s", resp.StatusCode, strings.TrimSpace(string(b)))
	}
	var result map[string]any
	decoder := json.NewDecoder(strings.NewReader(string(b)))
	decoder.UseNumber()
	if err := decoder.Decode(&result); err != nil {
		return nil, fmt.Errorf("xzn returned invalid JSON")
	}
	if err := decoder.Decode(&struct{}{}); err != io.EOF {
		return nil, fmt.Errorf("xzn returned invalid JSON")
	}
	if err := requirePositiveCode(result); err != nil {
		return nil, err
	}
	return result, nil
}

func cloneValues(values url.Values) url.Values {
	cloned := make(url.Values, len(values))
	for name, entries := range values {
		cloned[name] = append([]string(nil), entries...)
	}
	return cloned
}

func requirePositiveCode(result map[string]any) error {
	raw, exists := result["code"]
	if !exists {
		return errors.New("xzn response is missing code")
	}
	var text string
	switch value := raw.(type) {
	case json.Number:
		text = value.String()
	case string:
		text = value
	default:
		return errors.New("xzn response has invalid code")
	}
	code, err := strconv.ParseInt(text, 10, 64)
	if err != nil || code <= 0 {
		message, _ := result["msg"].(string)
		if strings.TrimSpace(message) == "" {
			message = "request rejected"
		}
		return fmt.Errorf("xzn %s", message)
	}
	return nil
}

func responseData(result map[string]any) (map[string]any, error) {
	data, ok := result["data"].(map[string]any)
	if !ok {
		return nil, errors.New("xzn response is missing data")
	}
	return data, nil
}

func responseString(data map[string]any, name string) string {
	switch value := data[name].(type) {
	case string:
		return strings.TrimSpace(value)
	case json.Number:
		return value.String()
	default:
		return ""
	}
}
