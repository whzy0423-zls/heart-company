package server

import (
	"encoding/json"
	"net/http"
	"net/url"
	"os"
	"strings"

	"nine-xing/nx-backend/apps/server/internal/httpx"
	"nine-xing/nx-backend/apps/server/internal/xznpay"
)

func (s *Server) xznPayConfig(w http.ResponseWriter, r *http.Request) {
	httpx.OK(w, map[string]any{"configured": strings.TrimSpace(os.Getenv("XZN_PID")) != "" && strings.TrimSpace(os.Getenv("XZN_KEY")) != "", "pid": os.Getenv("XZN_PID"), "baseURL": getenvDefault("XZN_API_BASE", "https://pay.xzncraft.cn/openapi"), "signType": getenvDefault("XZN_SIGN_TYPE", "MD5"), "notifyURL": os.Getenv("XZN_NOTIFY_URL"), "returnURL": os.Getenv("XZN_RETURN_URL")})
}
func (s *Server) xznPayCreate(w http.ResponseWriter, r *http.Request) {
	var in struct{ OutTradeNo, TotalAmount, Subject, PaytypeCode, ChannelID, Attach, ClientIP string }
	if json.NewDecoder(r.Body).Decode(&in) != nil || in.TotalAmount == "" || in.PaytypeCode == "" {
		httpx.Fail(w, 400, "totalAmount and paytypeCode are required")
		return
	}
	c := newXZNClient()
	if c == nil {
		httpx.Fail(w, 503, "XZN payment is not configured")
		return
	}
	v := url.Values{"out_trade_no": {in.OutTradeNo}, "total_amount": {in.TotalAmount}, "subject": {in.Subject}, "paytype_code": {in.PaytypeCode}, "channel_id": {in.ChannelID}, "attach": {in.Attach}, "client_ip": {in.ClientIP}, "notify_url": {os.Getenv("XZN_NOTIFY_URL")}, "return_url": {os.Getenv("XZN_RETURN_URL")}}
	out, e := c.Post("/pay/create", v)
	if e != nil {
		httpx.Fail(w, 502, e.Error())
		return
	}
	httpx.OK(w, out)
}
func newXZNClient() *xznpay.Client {
	pid, key := strings.TrimSpace(os.Getenv("XZN_PID")), strings.TrimSpace(os.Getenv("XZN_KEY"))
	if pid == "" || key == "" {
		return nil
	}
	return xznpay.New(xznpay.Config{BaseURL: getenvDefault("XZN_API_BASE", "https://pay.xzncraft.cn/openapi"), PID: pid, Key: key, SignType: getenvDefault("XZN_SIGN_TYPE", "MD5")})
}
func getenvDefault(k, d string) string {
	if v := strings.TrimSpace(os.Getenv(k)); v != "" {
		return v
	}
	return d
}
