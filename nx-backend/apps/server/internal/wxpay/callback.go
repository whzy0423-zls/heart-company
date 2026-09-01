package wxpay

import (
	"crypto"
	"crypto/aes"
	"crypto/cipher"
	"crypto/rsa"
	"crypto/sha256"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"strconv"
	"time"
)

// notifyEnvelope 是回调通知的外层结构。
type notifyEnvelope struct {
	ID        string `json:"id"`
	EventType string `json:"event_type"`
	Resource  struct {
		Algorithm      string `json:"algorithm"`
		Ciphertext     string `json:"ciphertext"`
		AssociatedData string `json:"associated_data"`
		Nonce          string `json:"nonce"`
	} `json:"resource"`
}

// transactionResource 是解密后的交易明文关键字段。
type transactionResource struct {
	AppID         string `json:"appid"`
	MchID         string `json:"mchid"`
	OutTradeNo    string `json:"out_trade_no"`
	TransactionID string `json:"transaction_id"`
	TradeState    string `json:"trade_state"`
	Amount        struct {
		Total int `json:"total"`
	} `json:"amount"`
}

func (c *Client) ParseCallback(rawBody []byte) (CallbackResult, error) {
	return c.ParseCallbackWithHeaders(nil, rawBody)
}

// ParseCallbackWithHeaders 解析并解密真实微信支付回调。
// 即使 dev 模式也不在公开回调路径接受明文模拟，避免公网误触发支付成功。
func (c *Client) ParseCallbackWithHeaders(headers http.Header, rawBody []byte) (CallbackResult, error) {
	if c.devMode {
		return CallbackResult{}, errors.New("dev payment must use the authenticated simulation endpoint")
	}
	if err := c.verifyCallbackSignature(headers, rawBody, time.Now()); err != nil {
		return CallbackResult{}, err
	}

	var env notifyEnvelope
	if err := json.Unmarshal(rawBody, &env); err != nil {
		return CallbackResult{}, err
	}
	plain, err := c.decryptAESGCM(env.Resource.AssociatedData, env.Resource.Nonce, env.Resource.Ciphertext)
	if err != nil {
		return CallbackResult{}, err
	}
	var tx transactionResource
	if err := json.Unmarshal(plain, &tx); err != nil {
		return CallbackResult{}, err
	}
	return CallbackResult{
		AppID:         tx.AppID,
		MchID:         tx.MchID,
		AmountTotal:   tx.Amount.Total,
		OutTradeNo:    tx.OutTradeNo,
		TransactionID: tx.TransactionID,
		TradeState:    tx.TradeState,
		Success:       tx.TradeState == "SUCCESS",
	}, nil
}

// ParseDevCallback accepts the explicit dev-only simulation payload. It is kept
// separate from the public callback parser so production callback routing never
// silently accepts unsigned JSON.
func (c *Client) ParseDevCallback(rawBody []byte) (CallbackResult, error) {
	if !c.devMode {
		return CallbackResult{}, errors.New("dev payment simulation is disabled")
	}
	var direct transactionResource
	if err := json.Unmarshal(rawBody, &direct); err != nil {
		return CallbackResult{}, err
	}
	if direct.OutTradeNo == "" {
		return CallbackResult{}, errors.New("dev callback requires out_trade_no")
	}
	state := direct.TradeState
	if state == "" {
		state = "SUCCESS"
	}
	return CallbackResult{
		OutTradeNo:    direct.OutTradeNo,
		TransactionID: direct.TransactionID,
		TradeState:    state,
		Success:       state == "SUCCESS",
	}, nil
}

func (c *Client) verifyCallbackSignature(headers http.Header, rawBody []byte, now time.Time) error {
	if c.callbackKey == nil {
		return errors.New("wxpay callback verification key is not configured")
	}
	if headers == nil {
		return errors.New("missing wxpay callback headers")
	}
	timestamp := headers.Get("Wechatpay-Timestamp")
	nonce := headers.Get("Wechatpay-Nonce")
	signature := headers.Get("Wechatpay-Signature")
	if timestamp == "" || nonce == "" || signature == "" {
		return errors.New("missing wxpay callback signature headers")
	}
	if c.callbackKeyID != "" && headers.Get("Wechatpay-Serial") != c.callbackKeyID {
		return errors.New("wxpay callback public key ID does not match Wechatpay-Serial")
	}
	ts, err := strconv.ParseInt(timestamp, 10, 64)
	if err != nil {
		return errors.New("invalid wxpay callback timestamp")
	}
	callbackTime := time.Unix(ts, 0)
	if now.Sub(callbackTime) > 5*time.Minute || callbackTime.Sub(now) > 5*time.Minute {
		return errors.New("wxpay callback timestamp is outside allowed window")
	}
	sig, err := base64.StdEncoding.DecodeString(signature)
	if err != nil {
		return errors.New("invalid wxpay callback signature encoding")
	}
	message := fmt.Sprintf("%s\n%s\n%s\n", timestamp, nonce, string(rawBody))
	digest := sha256.Sum256([]byte(message))
	if err := rsa.VerifyPKCS1v15(c.callbackKey, crypto.SHA256, digest[:], sig); err != nil {
		return errors.New("invalid wxpay callback signature")
	}
	return nil
}

// decryptAESGCM 用 APIv3 密钥解密回调密文（base64 ciphertext，附加 nonce/associated_data）。
func (c *Client) decryptAESGCM(associatedData, nonce, ciphertextB64 string) ([]byte, error) {
	ciphertext, err := base64.StdEncoding.DecodeString(ciphertextB64)
	if err != nil {
		return nil, err
	}
	block, err := aes.NewCipher([]byte(c.cfg.APIv3Key))
	if err != nil {
		return nil, err
	}
	gcm, err := cipher.NewGCM(block)
	if err != nil {
		return nil, err
	}
	return gcm.Open(nil, []byte(nonce), ciphertext, []byte(associatedData))
}
