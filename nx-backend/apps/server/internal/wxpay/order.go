package wxpay

import (
	"context"
	"crypto"
	"crypto/rand"
	"crypto/rsa"
	"crypto/sha256"
	"encoding/base64"
	"fmt"
	"strconv"
	"strings"
	"time"
)

// Prepay 调用 JSAPI 下单，返回小程序拉起支付所需参数。
// dev 模式下不请求微信，直接返回伪 prepay_id（paySign 为占位）。
func (c *Client) Prepay(ctx context.Context, outTradeNo, openID, description string, amountCents int) (PrepayResult, error) {
	if c.devMode {
		return PrepayResult{
			PrepayID:  "dev_prepay_" + outTradeNo,
			TimeStamp: strconv.FormatInt(time.Now().Unix(), 10),
			NonceStr:  randomString(24),
			Package:   "prepay_id=dev_prepay_" + outTradeNo,
			SignType:  "RSA",
			PaySign:   "dev-signature",
			Dev:       true,
		}, nil
	}

	body := map[string]any{
		"appid":        c.cfg.AppID,
		"mchid":        c.cfg.MchID,
		"description":  description,
		"out_trade_no": outTradeNo,
		"notify_url":   c.cfg.NotifyURL,
		"amount":       map[string]any{"total": amountCents, "currency": "CNY"},
		"payer":        map[string]any{"openid": openID},
	}
	var resp struct {
		PrepayID string `json:"prepay_id"`
		Code     string `json:"code"`
		Message  string `json:"message"`
	}
	if err := c.doSigned(ctx, "POST", "/v3/pay/transactions/jsapi", body, &resp); err != nil {
		return PrepayResult{}, err
	}
	if resp.PrepayID == "" {
		return PrepayResult{}, fmt.Errorf("prepay failed: %s %s", resp.Code, resp.Message)
	}
	return c.buildPayParams(resp.PrepayID)
}

// buildPayParams 用商户私钥对拉起参数二次签名（小程序 requestPayment 需要）。
func (c *Client) buildPayParams(prepayID string) (PrepayResult, error) {
	ts := strconv.FormatInt(time.Now().Unix(), 10)
	nonce := randomString(24)
	pkg := "prepay_id=" + prepayID
	// 签名串：appId\n timeStamp\n nonceStr\n package\n
	message := strings.Join([]string{c.cfg.AppID, ts, nonce, pkg}, "\n") + "\n"
	sign, err := c.sign(message)
	if err != nil {
		return PrepayResult{}, err
	}
	return PrepayResult{
		PrepayID:  prepayID,
		TimeStamp: ts,
		NonceStr:  nonce,
		Package:   pkg,
		SignType:  "RSA",
		PaySign:   sign,
	}, nil
}

// sign 用商户私钥 SHA256-RSA 签名并 base64。
func (c *Client) sign(message string) (string, error) {
	h := sha256.Sum256([]byte(message))
	sig, err := rsa.SignPKCS1v15(rand.Reader, c.privateKey, crypto.SHA256, h[:])
	if err != nil {
		return "", err
	}
	return base64.StdEncoding.EncodeToString(sig), nil
}
