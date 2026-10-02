package server

import (
	"context"
	"net/http"
	"strings"
	"testing"

	"nine-xing/nx-backend/apps/server/internal/businessmessage"
	"nine-xing/nx-backend/apps/server/internal/dbtx"
	"nine-xing/nx-backend/apps/server/internal/miniapp"
	"nine-xing/nx-backend/apps/server/internal/signup"
	"nine-xing/nx-backend/apps/server/internal/siteconfig"
	"nine-xing/nx-backend/apps/server/internal/wxpay"
)

func setCourseEnrollmentEnabled(t *testing.T, s *Server, enabled bool) {
	t.Helper()
	cfg, err := siteconfig.ReadStore(context.Background(), s.db, "")
	if err != nil {
		t.Fatal(err)
	}
	cfg.Home["miniappCourses"].(map[string]any)["enabled"] = enabled
	if err := siteconfig.UpsertStore(context.Background(), s.db, cfg); err != nil {
		t.Fatal(err)
	}
	stored, err := siteconfig.ReadStore(context.Background(), s.db, "")
	if err != nil || siteconfig.MiniappCoursesEnabled(stored) != enabled {
		t.Fatalf("course switch did not persist in database: enabled=%v, err=%v", enabled, err)
	}
	items, err := siteconfig.MiniappCourses(stored)
	if err != nil || len(items) != 2 {
		t.Fatalf("switch changed catalog: %+v, %v", items, err)
	}
}

func TestCourseEnrollmentSwitchPostgresBlocksNewCheckoutAndResumesAfterEnable(t *testing.T) {
	s, mux := courseCheckoutServer(t)
	s.env.WxPay.AppID = "test-app"
	s.env.WxPay.MchID = "test-merchant"
	setCourseEnrollmentEnabled(t, s, false)
	gateway := &coursePaymentStub{}
	s.coursePay = gateway
	response := courseCheckoutRequest(mux, 1, http.MethodPost, "/api/miniapp/course/orders", `{"courseId":"paid-course"}`)
	if response.Code != http.StatusConflict || !strings.Contains(response.Body.String(), "课程报名已关闭") {
		t.Fatalf("disabled catalog accepted checkout: %d %s", response.Code, response.Body.String())
	}
	var orders, bookings int
	if err := s.db.QueryRow(`SELECT (SELECT count(*) FROM orders),(SELECT count(*) FROM bookings)`).Scan(&orders, &bookings); err != nil {
		t.Fatal(err)
	}
	if orders != 0 || bookings != 0 {
		t.Fatalf("disabled checkout wrote purchase records: %d/%d", orders, bookings)
	}
	if _, prepays := gateway.counts(); prepays != 0 {
		t.Fatal("disabled checkout reached payment provider")
	}
	setCourseEnrollmentEnabled(t, s, true)
	response = courseCheckoutRequest(mux, 1, http.MethodPost, "/api/miniapp/course/orders", `{"courseId":"paid-course"}`)
	if response.Code != http.StatusOK || !strings.Contains(response.Body.String(), `"payParams"`) {
		t.Fatalf("reenabled catalog did not restore checkout: %d %s", response.Code, response.Body.String())
	}
}

func TestCourseEnrollmentSwitchPostgresKeepsPlainFormAndRejectsBoundForm(t *testing.T) {
	s, mux := courseCheckoutServer(t)
	setCourseEnrollmentEnabled(t, s, false)
	// The checkout fixture needs only a minimal signups table; add the lead
	// fields so this path exercises the real booking transaction and message.
	if _, err := s.db.Exec(`ALTER TABLE signups
	 ADD COLUMN name TEXT NOT NULL DEFAULT '', ADD COLUMN contact_type TEXT NOT NULL DEFAULT '',
	 ADD COLUMN contact TEXT NOT NULL DEFAULT '', ADD COLUMN message TEXT NOT NULL DEFAULT '',
	 ADD COLUMN visitor_id TEXT NOT NULL DEFAULT '', ADD COLUMN source_path TEXT NOT NULL DEFAULT '',
	 ADD COLUMN landing_page TEXT NOT NULL DEFAULT '', ADD COLUMN referrer TEXT NOT NULL DEFAULT '',
	 ADD COLUMN utm_source TEXT NOT NULL DEFAULT '', ADD COLUMN utm_medium TEXT NOT NULL DEFAULT '',
	 ADD COLUMN utm_campaign TEXT NOT NULL DEFAULT '', ADD COLUMN utm_content TEXT NOT NULL DEFAULT '',
	 ADD COLUMN utm_term TEXT NOT NULL DEFAULT '', ADD COLUMN game_result_id BIGINT,
	 ADD COLUMN ip TEXT NOT NULL DEFAULT '', ADD COLUMN user_agent TEXT NOT NULL DEFAULT '',
	 ADD COLUMN source_platform TEXT NOT NULL DEFAULT '', ADD COLUMN create_time TIMESTAMPTZ NOT NULL DEFAULT now()`); err != nil {
		t.Fatal(err)
	}
	s.miniappBookingService = miniapp.NewService(dbtx.SQLBeginner{DB: s.db}, s.miniapp, businessmessage.Store{},
		miniapp.WithSignupWriter(signup.NewStore(s.db)), miniapp.WithBookingWriter(s.miniapp))
	subscriber := make(chan signup.Lead, 1)
	s.signupSubscribers = map[chan signup.Lead]struct{}{subscriber: {}}
	mux.HandleFunc("/api/miniapp/bookings", s.miniappBookings)
	response := courseCheckoutRequest(mux, 1, http.MethodPost, "/api/miniapp/bookings", `{"kind":"course","courseId":"consult-course","contactName":"张三","phone":"13812345678"}`)
	if response.Code != http.StatusConflict {
		t.Fatalf("disabled catalog accepted bound form: %d %s", response.Code, response.Body.String())
	}
	response = courseCheckoutRequest(mux, 1, http.MethodPost, "/api/miniapp/bookings", `{"kind":"course","contactName":"张三","phone":"13812345678","intent":"课程咨询"}`)
	if response.Code != http.StatusOK {
		t.Fatalf("plain form no longer reaches transactional booking service: %d %s", response.Code, response.Body.String())
	}
	var courseID, source, contact string
	var signups, bookings, messages int
	if err := s.db.QueryRow(`SELECT b.course_id,s.source_platform,s.contact,
	 (SELECT count(*) FROM signups),(SELECT count(*) FROM bookings),
	 (SELECT count(*) FROM messages WHERE event_key='miniapp.booking.created')
	 FROM bookings b JOIN signups s ON b.signup_id=s.id WHERE b.wx_user_id=1`).Scan(&courseID, &source, &contact, &signups, &bookings, &messages); err != nil {
		t.Fatal(err)
	}
	if courseID != "" || source != "miniapp" || contact != "13812345678" || signups != 1 || bookings != 1 || messages != 1 {
		t.Fatalf("plain form lost persisted lead/booking/message: %q/%q/%q %d/%d/%d", courseID, source, contact, signups, bookings, messages)
	}
	select {
	case <-subscriber:
	default:
		t.Fatal("plain form did not notify the admin subscriber")
	}
}

func TestCourseEnrollmentSwitchPostgresKeepsReceiptsAndPaymentReconciliation(t *testing.T) {
	s, mux := courseCheckoutServer(t)
	installPendingCourse(t, s)
	setCourseEnrollmentEnabled(t, s, false)
	gateway := &coursePaymentStub{}
	s.coursePay = gateway
	for _, body := range []string{`{"courseId":"paid-course"}`, `{"bookingId":"42"}`} {
		response := courseCheckoutRequest(mux, 1, http.MethodPost, "/api/miniapp/course/orders", body)
		if response.Code != http.StatusConflict {
			t.Fatalf("disabled catalog restarted pending payment: %d %s", response.Code, response.Body.String())
		}
	}
	unowned := courseCheckoutRequest(mux, 2, http.MethodGet, "/api/miniapp/course/enrollment?courseId=paid-course", "")
	if unowned.Code != http.StatusNotFound {
		t.Fatalf("disabled course details still public: %d %s", unowned.Code, unowned.Body.String())
	}
	pending := readEnrollment(t, mux, 1, "bookingId=42")
	if pending.Order == nil || pending.Order.Status != "pending" || pending.Course.Cover != "" || pending.Course.Enabled {
		t.Fatalf("pending receipt/details incorrect: %+v", pending)
	}
	// A payment initiated before the switch may settle afterwards; it must
	// still reconcile and remain accessible without starting another payment.
	s.courseSync = coursePaymentSyncCache{}
	gateway.query = func(_ context.Context, id string, _ int) (wxpay.CallbackResult, error) {
		return paidCourseQuery(id), nil
	}
	status := courseCheckoutRequest(mux, 1, http.MethodGet, "/api/miniapp/course/orders/status?bookingId=42", "")
	if status.Code != http.StatusOK || !strings.Contains(status.Body.String(), `"status":"paid"`) {
		t.Fatalf("disabled catalog prevented payment reconciliation: %d %s", status.Code, status.Body.String())
	}
	owned := readEnrollment(t, mux, 1, "bookingId=42")
	if !owned.Owned || owned.Order == nil || owned.Order.Amount != 19900 || owned.Course.Cover != "/course.jpg" {
		t.Fatalf("paid course lost receipt/access: %+v", owned)
	}
	list := courseCheckoutRequest(mux, 1, http.MethodGet, "/api/miniapp/orders?status=paid", "")
	if list.Code != http.StatusOK || !strings.Contains(list.Body.String(), `"courseId":"paid-course"`) || !strings.Contains(list.Body.String(), `"cover":"/course.jpg"`) {
		t.Fatalf("paid order disappeared: %d %s", list.Code, list.Body.String())
	}
	repeat := courseCheckoutRequest(mux, 1, http.MethodPost, "/api/miniapp/course/orders", `{"courseId":"paid-course"}`)
	if repeat.Code != http.StatusOK || !strings.Contains(repeat.Body.String(), `"status":"paid"`) || strings.Contains(repeat.Body.String(), `"payParams"`) {
		t.Fatalf("paid repeat lost receipt or reopened payment: %d %s", repeat.Code, repeat.Body.String())
	}
	if _, prepays := gateway.counts(); prepays != 0 {
		t.Fatal("disabled/purchased course reached prepay")
	}
}
