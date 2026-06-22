package wxpay

import (
	"crypto/aes"
	"crypto/cipher"
	"encoding/base64"
	"encoding/json"
	"errors"
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
	OutTradeNo    string `json:"out_trade_no"`
	TransactionID string `json:"transaction_id"`
	TradeState    string `json:"trade_state"`
}

// ParseCallback 解析并解密回调。dev 模式下接受直传明文 {out_trade_no,...}。
// 注意：完整实现还应校验微信平台证书签名（Wechatpay-Signature 头）；
// 此处先做 APIv3 解密 + 状态判定，签名校验在拿到平台证书后补。
func (c *Client) ParseCallback(rawBody []byte) (CallbackResult, error) {
	if c.devMode {
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
		OutTradeNo:    tx.OutTradeNo,
		TransactionID: tx.TransactionID,
		TradeState:    tx.TradeState,
		Success:       tx.TradeState == "SUCCESS",
	}, nil
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
