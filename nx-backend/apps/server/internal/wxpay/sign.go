package wxpay

import (
	"bytes"
	"context"
	"crypto/rand"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strconv"
	"time"
)

// doSigned 发起带 v3 签名的请求。Authorization 头格式：
// WECHATPAY2-SHA256-RSA2048 mchid="..",nonce_str="..",signature="..",timestamp="..",serial_no=".."
func (c *Client) doSigned(ctx context.Context, method, path string, reqBody any, out any) error {
	var bodyBytes []byte
	if reqBody != nil {
		b, err := json.Marshal(reqBody)
		if err != nil {
			return err
		}
		bodyBytes = b
	}

	ts := strconv.FormatInt(time.Now().Unix(), 10)
	nonce := randomString(32)
	// 签名串：method\n url\n timestamp\n nonce\n body\n
	message := method + "\n" + path + "\n" + ts + "\n" + nonce + "\n" + string(bodyBytes) + "\n"
	signature, err := c.sign(message)
	if err != nil {
		return err
	}
	auth := fmt.Sprintf(
		`WECHATPAY2-SHA256-RSA2048 mchid="%s",nonce_str="%s",signature="%s",timestamp="%s",serial_no="%s"`,
		c.cfg.MchID, nonce, signature, ts, c.cfg.SerialNo,
	)

	req, err := http.NewRequestWithContext(ctx, method, apiBase+path, bytes.NewReader(bodyBytes))
	if err != nil {
		return err
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Accept", "application/json")
	req.Header.Set("Authorization", auth)

	resp, err := c.http.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	data, _ := io.ReadAll(resp.Body)
	if resp.StatusCode >= 300 {
		return fmt.Errorf("wxpay %s %s: status %d: %s", method, path, resp.StatusCode, string(data))
	}
	if out != nil && len(data) > 0 {
		return json.Unmarshal(data, out)
	}
	return nil
}

func randomString(n int) string {
	const charset = "abcdefghijklmnopqrstuvwxyzABCDEFGHIJKLMNOPQRSTUVWXYZ0123456789"
	b := make([]byte, n)
	if _, err := rand.Read(b); err != nil {
		// 退化为时间填充，极少触发
		for i := range b {
			b[i] = charset[(int(time.Now().UnixNano())+i)%len(charset)]
		}
		return string(b)
	}
	for i := range b {
		b[i] = charset[int(b[i])%len(charset)]
	}
	return string(b)
}
