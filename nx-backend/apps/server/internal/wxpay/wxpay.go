// Package wxpay 封装微信支付 v3（JSAPI）服务端能力：下单、生成小程序拉起参数、回调验签解密。
// 未配齐商户参数（商户号/证书/APIv3 密钥）或显式 WXPAY_DEV=true 时启用 dev 回退：
// 下单返回伪 prepay_id，可由「模拟回调」接口直接置为支付成功，便于无真实商户号时本地联调全闭环。
package wxpay

import (
	"crypto/rsa"
	"crypto/x509"
	"encoding/pem"
	"errors"
	"fmt"
	"net/http"
	"os"
	"time"
)

const apiBase = "https://api.mch.weixin.qq.com"

// Config 来自 config.WxPayConfig 的子集（避免反向依赖）。
type Config struct {
	MchID          string
	AppID          string
	APIv3Key       string
	SerialNo       string
	PrivateKeyPath string
	NotifyURL      string
	Dev            bool
}

type Client struct {
	cfg        Config
	privateKey *rsa.PrivateKey
	http       *http.Client
	devMode    bool
}

// PrepayResult 下单结果 + 小程序 wx.requestPayment 所需参数。
type PrepayResult struct {
	PrepayID  string `json:"prepayId"`
	TimeStamp string `json:"timeStamp"`
	NonceStr  string `json:"nonceStr"`
	Package   string `json:"package"`
	SignType  string `json:"signType"`
	PaySign   string `json:"paySign"`
	Dev       bool   `json:"devMode,omitempty"`
}

// CallbackResult 回调解密后的关键字段。
type CallbackResult struct {
	OutTradeNo    string
	TransactionID string
	TradeState    string // SUCCESS / ...
	Success       bool
}

func NewClient(cfg Config) (*Client, error) {
	devMode := cfg.Dev || cfg.MchID == "" || cfg.APIv3Key == "" || cfg.PrivateKeyPath == "" || cfg.SerialNo == ""
	c := &Client{
		cfg:     cfg,
		http:    &http.Client{Timeout: 12 * time.Second},
		devMode: devMode,
	}
	if devMode {
		return c, nil
	}
	key, err := loadPrivateKey(cfg.PrivateKeyPath)
	if err != nil {
		return nil, fmt.Errorf("load wxpay private key: %w", err)
	}
	c.privateKey = key
	return c, nil
}

// DevMode 是否处于模拟支付模式。
func (c *Client) DevMode() bool { return c.devMode }

func loadPrivateKey(path string) (*rsa.PrivateKey, error) {
	raw, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	block, _ := pem.Decode(raw)
	if block == nil {
		return nil, errors.New("invalid PEM in private key file")
	}
	key, err := x509.ParsePKCS8PrivateKey(block.Bytes)
	if err != nil {
		// 兼容 PKCS1
		if k1, e1 := x509.ParsePKCS1PrivateKey(block.Bytes); e1 == nil {
			return k1, nil
		}
		return nil, err
	}
	rsaKey, ok := key.(*rsa.PrivateKey)
	if !ok {
		return nil, errors.New("private key is not RSA")
	}
	return rsaKey, nil
}
