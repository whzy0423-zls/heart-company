package xznpay

import (
	"crypto"
	"crypto/md5"
	"crypto/rand"
	"crypto/rsa"
	"crypto/sha256"
	"crypto/x509"
	"encoding/base64"
	"encoding/hex"
	"encoding/pem"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"sort"
	"strings"
	"time"
)

type Config struct {
	BaseURL, PID, Key, SignType string
	PrivateKey                  *rsa.PrivateKey
	NotifyURL, ReturnURL        string
}
type Client struct {
	cfg  Config
	http *http.Client
}

func New(cfg Config) *Client { return &Client{cfg: cfg, http: &http.Client{Timeout: 20 * time.Second}} }
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
	fields.Set("pid", c.cfg.PID)
	fields.Set("timestamp", fmt.Sprint(time.Now().Unix()))
	fields.Set("sign_type", strings.ToUpper(c.cfg.SignType))
	fields.Set("sign", c.sign(fields))
	req, e := http.NewRequest(http.MethodPost, strings.TrimRight(c.cfg.BaseURL, "/")+path, strings.NewReader(fields.Encode()))
	if e != nil {
		return nil, e
	}
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	resp, e := c.http.Do(req)
	if e != nil {
		return nil, e
	}
	defer resp.Body.Close()
	b, _ := io.ReadAll(io.LimitReader(resp.Body, 1<<20))
	if resp.StatusCode/100 != 2 {
		return nil, fmt.Errorf("xzn http %d: %s", resp.StatusCode, strings.TrimSpace(string(b)))
	}
	return map[string]any{"raw": string(b)}, nil
}
