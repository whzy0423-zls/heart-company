package xznpay

import (
	"context"
	"io"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"
	"time"
)

func TestCreateBuildsSignedRequestAndParsesResponse(t *testing.T) {
	fixedNow := time.Unix(1_725_250_400, 0)
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost || r.URL.Path != "/pay/create" {
			t.Fatalf("unexpected request: %s %s", r.Method, r.URL.Path)
		}
		if err := r.ParseForm(); err != nil {
			t.Fatal(err)
		}
		for name, want := range map[string]string{
			"pid":          "merchant-1",
			"timestamp":    "1725250400",
			"sign_type":    "MD5",
			"out_trade_no": "order-1",
			"total_amount": "12.30",
			"subject":      "月度会员",
			"paytype_code": "34",
			"channel_id":   "channel-1",
			"attach":       "user-42",
			"client_ip":    "127.0.0.1",
			"notify_url":   "https://example.com/notify",
			"return_url":   "https://example.com/return",
		} {
			if got := r.Form.Get(name); got != want {
				t.Errorf("%s = %q, want %q", name, got, want)
			}
		}
		if !VerifyMD5(r.Form, "secret", r.Form.Get("sign")) {
			t.Fatal("request has invalid MD5 signature")
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = io.WriteString(w, `{"code":1,"msg":"ok","data":{"trade_no":"xzn-1","out_trade_no":"order-1","pay_url":"https://cashier.example/pay"}}`)
	}))
	defer server.Close()

	client := New(Config{BaseURL: server.URL, PID: "merchant-1", Key: "secret", SignType: "md5"}, WithClock(func() time.Time { return fixedNow }))
	result, err := client.Create(context.Background(), CreateRequest{
		OutTradeNo: "order-1", TotalAmount: "12.30", Subject: "月度会员", PaytypeCode: "34",
		ChannelID: "channel-1", Attach: "user-42", ClientIP: "127.0.0.1",
		NotifyURL: "https://example.com/notify", ReturnURL: "https://example.com/return",
	})
	if err != nil {
		t.Fatal(err)
	}
	if result.TradeNo != "xzn-1" || result.OutTradeNo != "order-1" || result.PayURL != "https://cashier.example/pay" {
		t.Fatalf("unexpected result: %#v", result)
	}
}

func TestQueryBuildsTypedRequestAndConvertsAmount(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/pay/query" {
			t.Fatalf("unexpected path %q", r.URL.Path)
		}
		if err := r.ParseForm(); err != nil {
			t.Fatal(err)
		}
		if got := r.Form.Get("out_trade_no"); got != "order-2" {
			t.Fatalf("out_trade_no = %q", got)
		}
		_, _ = io.WriteString(w, `{"code":1,"data":{"trade_no":"xzn-2","out_trade_no":"order-2","total_amount":"0.01","trade_status":"TRADE_SUCCESS"}}`)
	}))
	defer server.Close()

	client := New(Config{BaseURL: server.URL, PID: "merchant-1", Key: "secret", SignType: "MD5"})
	result, err := client.Query(context.Background(), QueryRequest{OutTradeNo: "order-2"})
	if err != nil {
		t.Fatal(err)
	}
	if result.TradeNo != "xzn-2" || result.OutTradeNo != "order-2" || result.TotalAmount != "0.01" || result.TotalAmountCents != 1 || result.TradeStatus != "TRADE_SUCCESS" {
		t.Fatalf("unexpected result: %#v", result)
	}
}

func TestCreateRejectsMismatchedOutTradeNo(t *testing.T) {
	client := clientReturningJSON(t, `{"code":1,"data":{"trade_no":"xzn-1","out_trade_no":"another-order","pay_url":"https://cashier.example/pay"}}`)
	_, err := client.Create(context.Background(), validCreateRequest())
	if err == nil {
		t.Fatal("expected mismatched out_trade_no to be rejected")
	}
}

func TestQueryRejectsMismatchedRequestedIdentifiers(t *testing.T) {
	tests := []struct {
		name     string
		request  QueryRequest
		response string
	}{
		{
			name:     "trade number",
			request:  QueryRequest{TradeNo: "xzn-expected"},
			response: `{"code":1,"data":{"trade_no":"xzn-other","out_trade_no":"order-1","total_amount":"1.00","trade_status":"TRADE_SUCCESS"}}`,
		},
		{
			name:     "merchant trade number",
			request:  QueryRequest{OutTradeNo: "order-expected"},
			response: `{"code":1,"data":{"trade_no":"xzn-1","out_trade_no":"order-other","total_amount":"1.00","trade_status":"TRADE_SUCCESS"}}`,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			client := clientReturningJSON(t, tt.response)
			if _, err := client.Query(context.Background(), tt.request); err == nil {
				t.Fatal("expected mismatched query identifier to be rejected")
			}
		})
	}
}

func TestCreateRequiresClientIPAndNotifyURL(t *testing.T) {
	tests := []struct {
		name   string
		mutate func(*CreateRequest)
	}{
		{name: "client IP", mutate: func(request *CreateRequest) { request.ClientIP = "" }},
		{name: "notify URL", mutate: func(request *CreateRequest) { request.NotifyURL = "" }},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			request := validCreateRequest()
			tt.mutate(&request)
			client := clientReturningJSON(t, `{"code":1,"data":{"trade_no":"xzn-1","out_trade_no":"order-1","pay_url":"https://cashier.example/pay"}}`)
			if _, err := client.Create(context.Background(), request); err == nil {
				t.Fatal("expected missing documented required field to be rejected")
			}
		})
	}
}

func TestTypedOperationsRejectInvalidResponses(t *testing.T) {
	tests := []struct {
		name string
		body string
		code int
		call func(*Client) error
	}{
		{name: "non-2xx", code: http.StatusBadGateway, body: `gateway down`, call: callCreate},
		{name: "invalid JSON", code: http.StatusOK, body: `{`, call: callCreate},
		{name: "trailing JSON", code: http.StatusOK, body: `{"code":1,"data":{"trade_no":"x","out_trade_no":"o","pay_url":"https://example.com"}} {}`, call: callCreate},
		{name: "missing code", code: http.StatusOK, body: `{"data":{"trade_no":"x","out_trade_no":"o","pay_url":"https://example.com"}}`, call: callCreate},
		{name: "non-positive code", code: http.StatusOK, body: `{"code":0,"msg":"rejected"}`, call: callCreate},
		{name: "missing create field", code: http.StatusOK, body: `{"code":1,"data":{"trade_no":"x","out_trade_no":"o"}}`, call: callCreate},
		{name: "missing query field", code: http.StatusOK, body: `{"code":1,"data":{"trade_no":"x","out_trade_no":"o","total_amount":"1.00"}}`, call: callQuery},
		{name: "invalid query amount", code: http.StatusOK, body: `{"code":1,"data":{"trade_no":"x","out_trade_no":"o","total_amount":"1.001","trade_status":"SUCCESS"}}`, call: callQuery},
		{name: "body too large", code: http.StatusOK, body: strings.Repeat("x", maxResponseBodyBytes+1), call: callCreate},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
				w.WriteHeader(tt.code)
				_, _ = io.WriteString(w, tt.body)
			}))
			defer server.Close()
			client := New(Config{BaseURL: server.URL, PID: "p", Key: "k", SignType: "MD5"})
			if err := tt.call(client); err == nil {
				t.Fatal("expected error")
			}
		})
	}
}

func TestParseYuanToCents(t *testing.T) {
	valid := map[string]int64{
		"0": 0, "1": 100, "1.2": 120, "1.23": 123,
		"92233720368547758.07": 9223372036854775807,
	}
	for input, want := range valid {
		got, err := ParseYuanToCents(input)
		if err != nil || got != want {
			t.Errorf("ParseYuanToCents(%q) = %d, %v; want %d, nil", input, got, err, want)
		}
	}
	for _, input := range []string{"", "-1", "+1", " 1", "1 ", ".1", "1.", "1.234", "1e2", "NaN", "92233720368547758.08", "999999999999999999999999999"} {
		if got, err := ParseYuanToCents(input); err == nil {
			t.Errorf("ParseYuanToCents(%q) = %d, want error", input, got)
		}
	}
}

func TestNewAcceptsInjectedHTTPClientAndPostRemainsCompatible(t *testing.T) {
	doer := roundTripFunc(func(req *http.Request) (*http.Response, error) {
		if req.URL.String() != "https://gateway.example/pay/create" {
			t.Fatalf("unexpected URL: %s", req.URL)
		}
		return &http.Response{
			StatusCode: http.StatusOK,
			Header:     make(http.Header),
			Body:       io.NopCloser(strings.NewReader(`{"code":1,"data":{}}`)),
			Request:    req,
		}, nil
	})
	client := New(Config{BaseURL: "https://gateway.example", PID: "p", Key: "k", SignType: "MD5"}, WithHTTPClient(doer))
	result, err := client.Post("/pay/create", url.Values{"subject": {"test"}})
	if err != nil {
		t.Fatal(err)
	}
	if result["code"] == nil {
		t.Fatalf("unexpected result: %#v", result)
	}
}

func callCreate(client *Client) error {
	request := validCreateRequest()
	request.OutTradeNo = "o"
	_, err := client.Create(context.Background(), request)
	return err
}

func callQuery(client *Client) error {
	_, err := client.Query(context.Background(), QueryRequest{OutTradeNo: "o"})
	return err
}

type roundTripFunc func(*http.Request) (*http.Response, error)

func (fn roundTripFunc) Do(req *http.Request) (*http.Response, error) { return fn(req) }

func validCreateRequest() CreateRequest {
	return CreateRequest{
		OutTradeNo: "order-1", TotalAmount: "1.00", Subject: "会员",
		PaytypeCode: "34", ClientIP: "127.0.0.1", NotifyURL: "https://example.com/notify",
	}
}

func clientReturningJSON(t *testing.T, body string) *Client {
	t.Helper()
	return New(Config{BaseURL: "https://gateway.example", PID: "p", Key: "k", SignType: "MD5"}, WithHTTPClient(roundTripFunc(func(req *http.Request) (*http.Response, error) {
		return &http.Response{
			StatusCode: http.StatusOK,
			Header:     make(http.Header),
			Body:       io.NopCloser(strings.NewReader(body)),
			Request:    req,
		}, nil
	})))
}
