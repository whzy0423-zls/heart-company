// Package wechat 封装微信小程序服务端能力（登录态换取）。
// 未配置 APPID/SECRET 或开启 LoginDev 时，使用本地回退：
// 由 code 派生稳定的伪 openid，便于无微信凭证时本地联调。
package wechat

import (
	"context"
	"crypto/sha1"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"time"
)

type Client struct {
	apiBase string
	appID   string
	secret  string
	devMode bool
	http    *http.Client
}

// Session 是 code2session 的结果。
type Session struct {
	OpenID     string
	UnionID    string
	SessionKey string
}

func NewClient(appID, secret string, devMode bool) *Client {
	return &Client{
		apiBase: "https://api.weixin.qq.com",
		appID:   appID,
		secret:  secret,
		devMode: devMode || appID == "" || secret == "",
		http:    &http.Client{Timeout: 8 * time.Second},
	}
}

// DevMode 返回是否处于本地回退模式（无真实微信凭证）。
func (c *Client) DevMode() bool { return c.devMode }

// Code2Session 用 wx.login 的 code 换取 openid/session。
func (c *Client) Code2Session(ctx context.Context, code string) (Session, error) {
	if code == "" {
		return Session{}, errors.New("code is required")
	}
	if c.devMode {
		// 本地回退：由 code 稳定派生伪 openid（同一 code 多次登录得到同一用户）
		sum := sha1.Sum([]byte("nx-dev-" + code))
		return Session{OpenID: "dev_" + hex.EncodeToString(sum[:8])}, nil
	}

	endpoint := strings.TrimRight(c.apiBase, "/") + "/sns/jscode2session?" + url.Values{
		"appid":      {c.appID},
		"secret":     {c.secret},
		"js_code":    {code},
		"grant_type": {"authorization_code"},
	}.Encode()

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, endpoint, nil)
	if err != nil {
		return Session{}, err
	}
	resp, err := c.http.Do(req)
	if err != nil {
		return Session{}, err
	}
	defer resp.Body.Close()
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		raw, _ := io.ReadAll(io.LimitReader(resp.Body, 4096))
		return Session{}, fmt.Errorf("微信登录请求失败(%d): %s", resp.StatusCode, strings.TrimSpace(string(raw)))
	}

	var out struct {
		OpenID     string `json:"openid"`
		UnionID    string `json:"unionid"`
		SessionKey string `json:"session_key"`
		ErrCode    int    `json:"errcode"`
		ErrMsg     string `json:"errmsg"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&out); err != nil {
		return Session{}, err
	}
	if out.ErrCode != 0 || out.OpenID == "" {
		return Session{}, fmt.Errorf("wx code2session failed: %d %s", out.ErrCode, out.ErrMsg)
	}
	return Session{OpenID: out.OpenID, UnionID: out.UnionID, SessionKey: out.SessionKey}, nil
}
