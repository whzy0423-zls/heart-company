package server

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"strings"
	"sync"
	"testing"
	"time"

	"nine-xing/nx-backend/apps/server/internal/wxpay"
)

type coursePaymentStub struct {
	mu               sync.Mutex
	query            func(context.Context, string, int) (wxpay.CallbackResult, error)
	prepay           func(string) (wxpay.PrepayResult, error)
	queries, prepays int
}

func (p *coursePaymentStub) QueryOrder(ctx context.Context, id string) (wxpay.CallbackResult, error) {
	p.mu.Lock()
	p.queries++
	n := p.queries
	p.mu.Unlock()
	if p.query != nil {
		return p.query(ctx, id, n)
	}
	return wxpay.CallbackResult{OutTradeNo: id, TradeState: "NOTPAY", AppID: "test-app", MchID: "test-merchant", AmountTotal: 19900}, nil
}
func (p *coursePaymentStub) Prepay(_ context.Context, id, _, _ string, _ int) (wxpay.PrepayResult, error) {
	p.mu.Lock()
	p.prepays++
	p.mu.Unlock()
	if p.prepay != nil {
		return p.prepay(id)
	}
	return wxpay.PrepayResult{TimeStamp: "1", NonceStr: "nonce", Package: "prepay_id=fixture", SignType: "RSA", PaySign: "fixture-sign"}, nil
}
func (*coursePaymentStub) CloseOrder(context.Context, string) error { return nil }
func (p *coursePaymentStub) counts() (int, int) {
	p.mu.Lock()
	defer p.mu.Unlock()
	return p.queries, p.prepays
}
func paidCourseQuery(id string) wxpay.CallbackResult {
	return wxpay.CallbackResult{OutTradeNo: id, TradeState: "SUCCESS", Success: true, TransactionID: "wx-paid", AmountTotal: 19900, AppID: "test-app", MchID: "test-merchant"}
}
func installPendingCourse(t *testing.T, s *Server) {
	t.Helper()
	s.env.WxPay.AppID = "test-app"
	s.env.WxPay.MchID = "test-merchant"
	if _, err := s.db.Exec(`INSERT INTO bookings(id,wx_user_id,course_id,course_title,price_cents,payment_mode,payment_status) VALUES(42,1,'paid-course','原课程名称',19900,'paid','pending');
 INSERT INTO orders(out_trade_no,wx_user_id,product,ref_id,title,amount,status) VALUES('crs-lost-callback',1,'course_booking',42,'原课程名称',19900,'pending')`); err != nil {
		t.Fatal(err)
	}
}
func TestCoursePaymentSyncPostgresRecoversLostCallback(t *testing.T) {
	s, mux := courseCheckoutServer(t)
	installPendingCourse(t, s)
	gateway := &coursePaymentStub{query: func(_ context.Context, id string, _ int) (wxpay.CallbackResult, error) {
		return paidCourseQuery(id), nil
	}}
	s.coursePay = gateway
	response := courseCheckoutRequest(mux, 1, http.MethodGet, "/api/miniapp/course/orders/status?bookingId=42", "")
	if response.Code != 200 || !strings.Contains(response.Body.String(), `"status":"paid"`) {
		t.Fatalf("lost callback not recovered: %s", response.Body.String())
	}
	for _, body := range []string{`{"courseId":"paid-course"}`, `{"bookingId":"42"}`} {
		response = courseCheckoutRequest(mux, 1, http.MethodPost, "/api/miniapp/course/orders", body)
		if response.Code != 200 || !strings.Contains(response.Body.String(), `"status":"paid"`) || strings.Contains(response.Body.String(), `"payParams"`) {
			t.Fatalf("paid repeat must bypass cashier: %d %s", response.Code, response.Body.String())
		}
	}
	var orders, bookings, messages int
	if err := s.db.QueryRow(`SELECT (SELECT count(*) FROM orders),(SELECT count(*) FROM bookings),(SELECT count(*) FROM messages WHERE event_key='miniapp.course.paid')`).Scan(&orders, &bookings, &messages); err != nil {
		t.Fatal(err)
	}
	if orders != 1 || bookings != 1 || messages != 1 {
		t.Fatalf("recovery duplicated records: %d/%d/%d", orders, bookings, messages)
	}
	if queries, prepays := gateway.counts(); queries != 1 || prepays != 0 {
		t.Fatalf("paid order queried/re-prepaid: %d/%d", queries, prepays)
	}
}
func TestCoursePaymentSyncPostgresOrderPaidRaceRequeries(t *testing.T) {
	s, mux := courseCheckoutServer(t)
	installPendingCourse(t, s)
	gateway := &coursePaymentStub{
		query: func(_ context.Context, id string, n int) (wxpay.CallbackResult, error) {
			if n == 1 {
				return wxpay.CallbackResult{OutTradeNo: id, TradeState: "NOTPAY", AppID: "test-app", MchID: "test-merchant", AmountTotal: 19900}, nil
			}
			return paidCourseQuery(id), nil
		},
		prepay: func(string) (wxpay.PrepayResult, error) {
			return wxpay.PrepayResult{}, &wxpay.HTTPError{StatusCode: 400, Body: `{"code":"ORDERPAID","message":"订单已支付"}`}
		},
	}
	s.coursePay = gateway
	response := courseCheckoutRequest(mux, 1, http.MethodPost, "/api/miniapp/course/orders", `{"courseId":"paid-course"}`)
	if response.Code != 200 || !strings.Contains(response.Body.String(), `"status":"paid"`) {
		t.Fatalf("ORDERPAID recovery=%d %s", response.Code, response.Body.String())
	}
	if queries, prepays := gateway.counts(); queries != 2 || prepays != 1 {
		t.Fatalf("must requery through NOTPAY cache after ORDERPAID: %d/%d", queries, prepays)
	}
}
func TestCoursePaymentSyncPostgresValidatesRemoteIdentity(t *testing.T) {
	for _, field := range []string{"amount", "appid", "mchid", "order", "transaction"} {
		t.Run(field, func(t *testing.T) {
			s, mux := courseCheckoutServer(t)
			installPendingCourse(t, s)
			s.coursePay = &coursePaymentStub{query: func(_ context.Context, id string, _ int) (wxpay.CallbackResult, error) {
				result := paidCourseQuery(id)
				switch field {
				case "amount":
					result.AmountTotal++
				case "appid":
					result.AppID = "other"
				case "mchid":
					result.MchID = "other"
				case "order":
					result.OutTradeNo = "other"
				case "transaction":
					result.TransactionID = ""
				}
				return result, nil
			}}
			response := courseCheckoutRequest(mux, 1, http.MethodGet, "/api/miniapp/course/orders/status?bookingId=42", "")
			if response.Code != 200 || !strings.Contains(response.Body.String(), `"syncStatus":"retrying"`) || !strings.Contains(response.Body.String(), `"status":"pending"`) {
				t.Fatalf("invalid identity response=%s", response.Body.String())
			}
			var status string
			if err := s.db.QueryRow(`SELECT status FROM orders WHERE out_trade_no='crs-lost-callback'`).Scan(&status); err != nil {
				t.Fatal(err)
			}
			if status != "pending" {
				t.Fatalf("mismatched %s marked paid", field)
			}
		})
	}
}
func TestCoursePaymentSyncPostgresQueriesAreBoundedAndThrottled(t *testing.T) {
	s, mux := courseCheckoutServer(t)
	installPendingCourse(t, s)
	gateway := &coursePaymentStub{query: func(ctx context.Context, _ string, _ int) (wxpay.CallbackResult, error) {
		deadline, ok := ctx.Deadline()
		if !ok || time.Until(deadline) > 5*time.Second {
			t.Error("query missing bounded deadline")
		}
		return wxpay.CallbackResult{}, errors.New("query unavailable")
	}}
	s.coursePay = gateway
	var wg sync.WaitGroup
	for i := 0; i < 8; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			response := courseCheckoutRequest(mux, 1, http.MethodGet, "/api/miniapp/course/orders/status?bookingId=42", "")
			if response.Code != 200 || !strings.Contains(response.Body.String(), `"syncStatus":"retrying"`) {
				t.Errorf("transient query must remain confirming: %d %s", response.Code, response.Body.String())
			}
		}()
	}
	wg.Wait()
	if queries, prepays := gateway.counts(); queries != 1 || prepays != 0 {
		t.Fatalf("unbounded duplicate queries: %d/%d", queries, prepays)
	}
	other := courseCheckoutRequest(mux, 2, http.MethodGet, "/api/miniapp/course/orders/status?bookingId=42", "")
	if other.Code != 404 {
		t.Fatalf("other user status=%d", other.Code)
	}
}
func TestCoursePaymentSyncPostgresNewOrderNotExistCanPrepay(t *testing.T) {
	s, mux := courseCheckoutServer(t)
	gateway := &coursePaymentStub{query: func(context.Context, string, int) (wxpay.CallbackResult, error) {
		return wxpay.CallbackResult{}, &wxpay.HTTPError{StatusCode: 404, Body: `{"code":"ORDER_NOT_EXIST"}`}
	}}
	s.coursePay = gateway
	response := courseCheckoutRequest(mux, 1, http.MethodPost, "/api/miniapp/course/orders", `{"courseId":"paid-course"}`)
	var result struct {
		Data struct {
			Status    string
			PayParams *wxpay.PrepayResult
		}
	}
	if err := json.Unmarshal(response.Body.Bytes(), &result); err != nil {
		t.Fatal(err)
	}
	if response.Code != 200 || result.Data.Status != "pending" || result.Data.PayParams == nil || result.Data.PayParams.Package == "" {
		t.Fatalf("new merchant order blocked: %s", response.Body.String())
	}
	if queries, prepays := gateway.counts(); queries != 1 || prepays != 1 {
		t.Fatalf("new order flow=%d/%d", queries, prepays)
	}
}

func TestCoursePaymentSyncPostgresPaidOrderSurvivesCatalogRemoval(t *testing.T) {
	s, mux := courseCheckoutServer(t)
	installPendingCourse(t, s)
	gateway := &coursePaymentStub{query: func(_ context.Context, id string, _ int) (wxpay.CallbackResult, error) {
		return paidCourseQuery(id), nil
	}}
	s.coursePay = gateway
	if _, err := s.db.Exec(`UPDATE site_configs SET config=jsonb_set(config,'{home,miniappCourses}', '{"items":[]}'::jsonb)`); err != nil {
		t.Fatal(err)
	}
	response := courseCheckoutRequest(mux, 1, http.MethodPost, "/api/miniapp/course/orders", `{"courseId":"paid-course"}`)
	if response.Code != 200 || !strings.Contains(response.Body.String(), `"status":"paid"`) || !strings.Contains(response.Body.String(), `"title":"原课程名称"`) || !strings.Contains(response.Body.String(), `"paidAt":`) {
		t.Fatalf("removed catalog hides actual purchase: %d %s", response.Code, response.Body.String())
	}
}

func TestCoursePaymentSyncPostgresUnknownStateCannotStartSecondPayment(t *testing.T) {
	s, mux := courseCheckoutServer(t)
	installPendingCourse(t, s)
	gateway := &coursePaymentStub{query: func(context.Context, string, int) (wxpay.CallbackResult, error) {
		return wxpay.CallbackResult{}, context.DeadlineExceeded
	}}
	s.coursePay = gateway
	response := courseCheckoutRequest(mux, 1, http.MethodPost, "/api/miniapp/course/orders", `{"courseId":"paid-course"}`)
	if response.Code != 200 || !strings.Contains(response.Body.String(), `"status":"pending"`) || !strings.Contains(response.Body.String(), `"syncStatus":"retrying"`) || strings.Contains(response.Body.String(), `"payParams"`) {
		t.Fatalf("uncertain payment should remain confirming: %d %s", response.Code, response.Body.String())
	}
	if _, prepays := gateway.counts(); prepays != 0 {
		t.Fatalf("uncertain prior charge must not open another cashier: %d", prepays)
	}
	var orders, bookings int
	if err := s.db.QueryRow(`SELECT (SELECT count(*) FROM orders),(SELECT count(*) FROM bookings)`).Scan(&orders, &bookings); err != nil {
		t.Fatal(err)
	}
	if orders != 1 || bookings != 1 {
		t.Fatalf("query failure duplicated checkout: %d/%d", orders, bookings)
	}
}

func TestCoursePaymentSyncPostgresConcurrentCallbackIsIdempotent(t *testing.T) {
	s, mux := courseCheckoutServer(t)
	installPendingCourse(t, s)
	s.coursePay = &coursePaymentStub{query: func(_ context.Context, id string, _ int) (wxpay.CallbackResult, error) {
		return paidCourseQuery(id), nil
	}}
	s.payNotifyParser = func(http.Header, []byte) (wxpay.CallbackResult, error) {
		return paidCourseQuery("crs-lost-callback"), nil
	}
	var wg sync.WaitGroup
	for i := 0; i < 8; i++ {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			if i%2 == 0 {
				response := courseCheckoutRequest(mux, 1, http.MethodGet, "/api/miniapp/course/orders/status?bookingId=42", "")
				if response.Code != 200 || !strings.Contains(response.Body.String(), `"status":"paid"`) {
					t.Errorf("query status=%d %s", response.Code, response.Body.String())
				}
			} else {
				response := courseCheckoutRequest(http.HandlerFunc(s.payNotify), 1, http.MethodPost, "/api/pay/notify", `{}`)
				if response.Code != 200 {
					t.Errorf("callback status=%d %s", response.Code, response.Body.String())
				}
			}
		}(i)
	}
	wg.Wait()
	var count int
	if err := s.db.QueryRow(`SELECT count(*) FROM messages WHERE event_key='miniapp.course.paid'`).Scan(&count); err != nil {
		t.Fatal(err)
	}
	if count != 1 {
		t.Fatalf("concurrent recovery duplicated notification: %d", count)
	}
}

func TestCoursePaymentSyncPostgresUncertainClosedOrderRemainsConfirming(t *testing.T) {
	s, mux := courseCheckoutServer(t)
	installPendingCourse(t, s)
	if _, err := s.db.Exec(`UPDATE orders SET status='closed' WHERE out_trade_no='crs-lost-callback'`); err != nil {
		t.Fatal(err)
	}
	s.coursePay = &coursePaymentStub{query: func(context.Context, string, int) (wxpay.CallbackResult, error) {
		return wxpay.CallbackResult{}, context.DeadlineExceeded
	}}
	response := courseCheckoutRequest(mux, 1, http.MethodGet, "/api/miniapp/course/orders/status?bookingId=42", "")
	if response.Code != 200 || !strings.Contains(response.Body.String(), `"status":"pending"`) || !strings.Contains(response.Body.String(), `"syncStatus":"retrying"`) {
		t.Fatalf("unverified close shown as final failure: %d %s", response.Code, response.Body.String())
	}
	var status string
	if err := s.db.QueryRow(`SELECT status FROM orders WHERE out_trade_no='crs-lost-callback'`).Scan(&status); err != nil {
		t.Fatal(err)
	}
	if status != "closed" {
		t.Fatal("query timeout rewrote local state")
	}
	anonymous := courseCheckoutRequest(mux, 0, http.MethodGet, "/api/miniapp/course/orders/status?bookingId=42", "")
	if anonymous.Code != http.StatusUnauthorized {
		t.Fatalf("anonymous status=%d", anonymous.Code)
	}
}
