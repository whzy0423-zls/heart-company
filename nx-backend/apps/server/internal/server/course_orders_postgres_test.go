package server

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"sync"
	"testing"

	"nine-xing/nx-backend/apps/server/internal/auth"
	"nine-xing/nx-backend/apps/server/internal/config"
	"nine-xing/nx-backend/apps/server/internal/miniapp"
	"nine-xing/nx-backend/apps/server/internal/siteconfig"
	"nine-xing/nx-backend/apps/server/internal/testdb"
	"nine-xing/nx-backend/apps/server/internal/wxpay"
)

func courseCheckoutServer(t *testing.T) (*Server, *http.ServeMux) {
	t.Helper()
	database, _ := testdb.OpenEnvIsolatedSchema(t, "course_checkout")
	raw, err := os.ReadFile("../db/schema.sql")
	if err != nil {
		t.Fatal(err)
	}
	schema := string(raw)
	for _, bounds := range [][2]string{
		{"CREATE TABLE IF NOT EXISTS wx_users (", "-- ============ 平台来源与统一业务消息升级"},
		{"CREATE TABLE IF NOT EXISTS orders (", "-- 报告解锁："},
	} {
		start, end := strings.Index(schema, bounds[0]), strings.Index(schema, bounds[1])
		if start < 0 || end < start {
			t.Fatal("schema boundaries changed")
		}
		if _, err := database.Exec(schema[start:end]); err != nil {
			t.Fatal(err)
		}
	}
	if _, err := database.Exec(`CREATE TABLE signups (id BIGSERIAL PRIMARY KEY,interest TEXT NOT NULL DEFAULT '');
 CREATE TABLE site_configs (key TEXT PRIMARY KEY,config JSONB NOT NULL,update_time TIMESTAMPTZ NOT NULL DEFAULT now());
 CREATE TABLE messages (id BIGSERIAL PRIMARY KEY,type TEXT NOT NULL,title TEXT NOT NULL,content TEXT NOT NULL,platform TEXT NOT NULL,event_key TEXT NOT NULL,business_id TEXT NOT NULL,business_type TEXT NOT NULL,target_path TEXT NOT NULL,UNIQUE(event_key,business_type,business_id));
 INSERT INTO wx_users(id,openid,nickname,phone) VALUES(1,'buyer-1','张同学',''),(2,'buyer-2','另一位同学','13912345678');`); err != nil {
		t.Fatal(err)
	}
	var cfg siteconfig.SiteConfig
	if err := json.Unmarshal([]byte(`{"site":{"brandName":"九型课堂","logo":"/logo.png"},"navigation":{"main":[{"label":"首页","to":"/"}]},"types":[{"id":"1","name":"一型"}],"home":{"miniappCourses":{"items":[{"id":"paid-course","title":"九型成长课","enabled":true,"priceCents":19900,"paymentMode":"consult","cover":"/course.jpg"},{"id":"consult-course","title":"课程咨询","enabled":true,"paymentMode":"paid","priceCents":0}]}}}`), &cfg); err != nil {
		t.Fatal(err)
	}
	if err := siteconfig.UpsertStore(context.Background(), database, cfg); err != nil {
		t.Fatal(err)
	}
	pay, err := wxpay.NewClient(wxpay.Config{Dev: true})
	if err != nil {
		t.Fatal(err)
	}
	s := &Server{db: database, miniapp: miniapp.NewStore(database), pay: pay, env: config.Env{AppEnv: "test"}}
	mux := http.NewServeMux()
	registerCourseOrderRoutes(mux, func(h http.HandlerFunc) http.HandlerFunc { return h }, s)
	return s, mux
}

func courseCheckoutRequest(mux http.Handler, uid int64, method, path, body string) *httptest.ResponseRecorder {
	r := httptest.NewRequest(method, path, strings.NewReader(body))
	r = r.WithContext(withUser(r.Context(), auth.UserInfo{ID: uid}))
	w := httptest.NewRecorder()
	mux.ServeHTTP(w, r)
	return w
}

func TestCourseCheckoutPostgresDirectPaymentAndOrderVisibility(t *testing.T) {
	s, mux := courseCheckoutServer(t)
	create := courseCheckoutRequest(mux, 1, http.MethodPost, "/api/miniapp/course/orders", `{"courseId":"paid-course"}`)
	if create.Code != http.StatusOK {
		t.Fatalf("direct purchase status=%d body=%s", create.Code, create.Body.String())
	}
	var result struct {
		Data courseOrderResponse `json:"data"`
	}
	if err := json.Unmarshal(create.Body.Bytes(), &result); err != nil {
		t.Fatal(err)
	}
	order := result.Data
	if order.Amount != 19900 || order.BookingID == "" || order.PayParams.Package == "" {
		t.Fatalf("invalid checkout=%+v", order)
	}
	var name, phone string
	var signupID *int64
	if err := s.db.QueryRow(`SELECT contact_name,phone,signup_id FROM bookings WHERE id=$1`, order.BookingID).Scan(&name, &phone, &signupID); err != nil {
		t.Fatal(err)
	}
	if name != "张同学" || phone != "" || signupID != nil {
		t.Fatalf("must use actual profile without fabricated contact: %q/%q/%v", name, phone, signupID)
	}
	var wg sync.WaitGroup
	responses := make(chan *httptest.ResponseRecorder, 8)
	for i := 0; i < 8; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			responses <- courseCheckoutRequest(mux, 1, http.MethodPost, "/api/miniapp/course/orders", `{"courseId":"paid-course"}`)
		}()
	}
	wg.Wait()
	close(responses)
	for response := range responses {
		var got struct {
			Data courseOrderResponse `json:"data"`
		}
		_ = json.Unmarshal(response.Body.Bytes(), &got)
		if response.Code != 200 || got.Data.BookingID != order.BookingID || got.Data.OutTradeNo != order.OutTradeNo {
			t.Fatalf("double submit produced another order: %s", response.Body.String())
		}
	}
	for i := 0; i < 2; i++ {
		if _, err := s.miniapp.MarkOrderPaidDetailed(context.Background(), order.OutTradeNo, "wx-transaction"); err != nil {
			t.Fatal(err)
		}
	}
	list := courseCheckoutRequest(mux, 1, http.MethodGet, "/api/miniapp/orders?status=paid&page=1&pageSize=1", "")
	var orders struct {
		Data struct {
			Items []struct {
				ID, Title, Product, BookingID, CourseID, Cover, Status, PaidAt string
				Amount                                                         int
			}
			Total, Page, PageSize int
		}
	}
	if err := json.Unmarshal(list.Body.Bytes(), &orders); err != nil {
		t.Fatal(err)
	}
	if list.Code != 200 || orders.Data.Total != 1 || len(orders.Data.Items) != 1 || orders.Data.Items[0].Status != "paid" || orders.Data.Items[0].CourseID != "paid-course" || orders.Data.Items[0].BookingID != order.BookingID || orders.Data.Items[0].Cover != "/course.jpg" || orders.Data.Items[0].PaidAt == "" {
		t.Fatalf("paid order list=%s", list.Body.String())
	}
	other := courseCheckoutRequest(mux, 2, http.MethodGet, "/api/miniapp/orders", "")
	if !strings.Contains(other.Body.String(), `"total":0`) {
		t.Fatalf("other user can see order: %s", other.Body.String())
	}
	repeat := courseCheckoutRequest(mux, 1, http.MethodPost, "/api/miniapp/course/orders", `{"courseId":"paid-course"}`)
	if repeat.Code != http.StatusConflict {
		t.Fatalf("paid repeat=%d %s", repeat.Code, repeat.Body.String())
	}
	forbidden := courseCheckoutRequest(mux, 2, http.MethodPost, "/api/miniapp/course/orders", `{"bookingId":"`+order.BookingID+`"}`)
	if forbidden.Code != http.StatusNotFound {
		t.Fatalf("other user booking status=%d", forbidden.Code)
	}
	admin := httptest.NewRecorder()
	s.adminMiniappOrders(admin, httptest.NewRequest(http.MethodGet, "/api/admin/miniapp/orders?product=course_booking", nil))
	if admin.Code != 200 || !strings.Contains(admin.Body.String(), `"courseId":"paid-course"`) || !strings.Contains(admin.Body.String(), `"status":"paid"`) {
		t.Fatalf("admin order=%s", admin.Body.String())
	}
	var count int
	if err := s.db.QueryRow(`SELECT count(*) FROM messages WHERE event_key='miniapp.course.paid'`).Scan(&count); err != nil {
		t.Fatal(err)
	}
	if count != 1 {
		t.Fatalf("expected one paid notification, got %d", count)
	}
}

func TestCourseCheckoutPostgresConsultationAndLegacyBooking(t *testing.T) {
	s, mux := courseCheckoutServer(t)
	consult := courseCheckoutRequest(mux, 1, http.MethodPost, "/api/miniapp/course/orders", `{"courseId":"consult-course"}`)
	if consult.Code != 409 {
		t.Fatalf("consultation accepted payment: %d %s", consult.Code, consult.Body.String())
	}
	if _, err := s.db.Exec(`INSERT INTO bookings(id,wx_user_id,course_id,course_title,price_cents,payment_mode,payment_status) VALUES (42,2,'paid-course','九型成长课',19900,'paid','pending')`); err != nil {
		t.Fatal(err)
	}
	legacy := courseCheckoutRequest(mux, 2, http.MethodPost, "/api/miniapp/course/orders", `{"bookingId":"42"}`)
	if legacy.Code != 200 {
		t.Fatalf("legacy booking checkout=%d %s", legacy.Code, legacy.Body.String())
	}
	status := courseCheckoutRequest(mux, 1, http.MethodGet, "/api/miniapp/course/orders/status?bookingId=42", "")
	if status.Code != 404 {
		t.Fatalf("other user status access=%d", status.Code)
	}
	ambiguous := courseCheckoutRequest(mux, 2, http.MethodPost, "/api/miniapp/course/orders", `{"courseId":"paid-course","bookingId":"42"}`)
	if ambiguous.Code != 400 {
		t.Fatalf("ambiguous target accepted: %d", ambiguous.Code)
	}
	invalid := courseCheckoutRequest(mux, 2, http.MethodGet, "/api/miniapp/orders?status=arbitrary", "")
	if invalid.Code != 400 {
		t.Fatalf("invalid status accepted: %d", invalid.Code)
	}
}

func TestCourseCheckoutPostgresPriceChangePreservesHistory(t *testing.T) {
	s, mux := courseCheckoutServer(t)
	create := courseCheckoutRequest(mux, 1, http.MethodPost, "/api/miniapp/course/orders", `{"courseId":"paid-course","amount":1}`)
	if create.Code != 200 {
		t.Fatalf("create=%s", create.Body.String())
	}
	var result struct{ Data courseOrderResponse }
	if err := json.Unmarshal(create.Body.Bytes(), &result); err != nil {
		t.Fatal(err)
	}
	old := result.Data
	if old.Amount != 19900 {
		t.Fatalf("client modified checkout price: %d", old.Amount)
	}
	cfg, err := siteconfig.ReadStore(context.Background(), s.db, "")
	if err != nil {
		t.Fatal(err)
	}
	cfg.Home["miniappCourses"] = siteconfig.MiniappCoursesConfig{Items: []siteconfig.MiniappCourse{{ID: "paid-course", Title: "新版课程名称", Enabled: true, PriceCents: 29900}}}
	if err := siteconfig.UpsertStore(context.Background(), s.db, cfg); err != nil {
		t.Fatal(err)
	}
	retry := courseCheckoutRequest(mux, 1, http.MethodPost, "/api/miniapp/course/orders", `{"bookingId":"`+old.BookingID+`"}`)
	if retry.Code != 409 {
		t.Fatalf("old price should require confirmation: %s", retry.Body.String())
	}
	fresh := courseCheckoutRequest(mux, 1, http.MethodPost, "/api/miniapp/course/orders", `{"courseId":"paid-course"}`)
	if err := json.Unmarshal(fresh.Body.Bytes(), &result); err != nil {
		t.Fatal(err)
	}
	if fresh.Code != 200 || result.Data.Amount != 29900 || result.Data.BookingID == old.BookingID {
		t.Fatalf("new price checkout=%s", fresh.Body.String())
	}
	for i := 0; i < 2; i++ {
		applied, err := s.miniapp.MarkOrderPaidDetailed(context.Background(), result.Data.OutTradeNo, "paid-new-price")
		if err != nil {
			t.Fatal(err)
		}
		if len(applied.PendingToClose) != 1 || applied.PendingToClose[0] != old.OutTradeNo {
			t.Fatalf("callback must close old-price WeChat prepay and retry until confirmed: %+v", applied)
		}
	}
	if err := s.miniapp.ClosePendingOrders(context.Background(), []string{old.OutTradeNo}); err != nil {
		t.Fatal(err)
	}
	cfg.Home["miniappCourses"] = siteconfig.MiniappCoursesConfig{Items: []siteconfig.MiniappCourse{}}
	if err := siteconfig.UpsertStore(context.Background(), s.db, cfg); err != nil {
		t.Fatal(err)
	}
	list := courseCheckoutRequest(mux, 1, http.MethodGet, "/api/miniapp/orders?pageSize=1&page=2", "")
	var orders struct {
		Data struct {
			Items []struct {
				Title  string
				Amount int
			}
			Total, Page, PageSize int
		}
	}
	if err := json.Unmarshal(list.Body.Bytes(), &orders); err != nil {
		t.Fatal(err)
	}
	if list.Code != 200 || orders.Data.Total != 2 || orders.Data.Page != 2 || orders.Data.PageSize != 1 || len(orders.Data.Items) != 1 || orders.Data.Items[0].Title != "九型成长课" || orders.Data.Items[0].Amount != 19900 {
		t.Fatalf("historical order lost after catalog deletion: %s", list.Body.String())
	}
}

func TestCourseCheckoutPostgresStatusReadFailureIsNotPending(t *testing.T) {
	s, mux := courseCheckoutServer(t)
	if _, err := s.db.Exec(`INSERT INTO bookings(id,wx_user_id,course_id,course_title,price_cents,payment_mode,payment_status) VALUES(42,1,'paid-course','九型成长课',19900,'paid','pending'); DROP TABLE orders`); err != nil {
		t.Fatal(err)
	}
	response := courseCheckoutRequest(mux, 1, http.MethodGet, "/api/miniapp/course/orders/status?bookingId=42", "")
	if response.Code != 500 {
		t.Fatalf("database error must not masquerade as pending: %d %s", response.Code, response.Body.String())
	}
}
