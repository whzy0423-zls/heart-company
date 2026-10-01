package server

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"strings"
	"testing"

	"nine-xing/nx-backend/apps/server/internal/siteconfig"
	"nine-xing/nx-backend/apps/server/internal/wxpay"
)

type enrollmentResult struct {
	Owned             bool
	BookingID         string
	CourseID          string
	BookingStatus     string
	SyncStatus        string
	CatalogAvailable  bool
	CustomerServiceQr string
	Order             *struct {
		ID, OutTradeNo, BookingID, CourseID, Title, Status, CreateTime, PaidAt string
		Amount                                                                 int
	}
	Course struct {
		ID, Title, Subtitle, Description, Cover, Format, Duration, Schedule, Location, Notice string
		Bullets, Outline                                                                      []string
		Enabled                                                                               bool
	}
}

func readEnrollment(t *testing.T, mux http.Handler, uid int64, query string) enrollmentResult {
	t.Helper()
	response := courseCheckoutRequest(mux, uid, http.MethodGet, "/api/miniapp/course/enrollment?"+query, "")
	if response.Code != http.StatusOK {
		t.Fatalf("enrollment status=%d body=%s", response.Code, response.Body.String())
	}
	for _, privateField := range []string{"payParams", "prepay_id", "transaction_id", "wx-paid"} {
		if strings.Contains(response.Body.String(), privateField) {
			t.Fatalf("enrollment exposed payment internals: %s", response.Body.String())
		}
	}
	var envelope struct{ Data enrollmentResult }
	if err := json.Unmarshal(response.Body.Bytes(), &envelope); err != nil {
		t.Fatal(err)
	}
	return envelope.Data
}

func setEnrollmentCatalog(t *testing.T, s *Server, items []siteconfig.MiniappCourse) {
	t.Helper()
	cfg, err := siteconfig.ReadStore(context.Background(), s.db, "")
	if err != nil {
		t.Fatal(err)
	}
	cfg.Site.CustomerServiceQr = "https://configured.example/teacher-support.png"
	cfg.Home["miniappCourses"] = siteconfig.MiniappCoursesConfig{Items: items}
	if err := siteconfig.UpsertStore(context.Background(), s.db, cfg); err != nil {
		t.Fatal(err)
	}
}

func settleEnrollment(t *testing.T, s *Server) {
	t.Helper()
	installPendingCourse(t, s)
	if _, err := s.miniapp.MarkOrderPaidDetailed(context.Background(), "crs-lost-callback", "wx-paid"); err != nil {
		t.Fatal(err)
	}
}

func TestCourseEnrollmentPostgresPaidOrderAndCurrentArrangements(t *testing.T) {
	s, mux := courseCheckoutServer(t)
	settleEnrollment(t, s)
	setEnrollmentCatalog(t, s, []siteconfig.MiniappCourse{{
		ID: "paid-course", Title: "后台调整后的课程", PriceCents: 29900, Enabled: true,
		Subtitle: "课程副标题", Description: "当前课程介绍", Cover: "/updated-cover.jpg",
		Format: "线下课", Duration: "2 天", Schedule: "10 月 18 日 09:30", Location: "九型课堂 201",
		Bullets: []string{"小班教学"}, Outline: []string{"认识自我", "关系练习"}, Notice: "请提前 10 分钟到场",
	}})
	for _, query := range []string{"bookingId=42", "courseId=paid-course"} {
		got := readEnrollment(t, mux, 1, query)
		if !got.Owned || got.BookingID != "42" || got.CourseID != "paid-course" || got.BookingStatus != "pending" || got.SyncStatus != "confirmed" || !got.CatalogAvailable {
			t.Fatalf("paid enrollment=%+v", got)
		}
		if got.Order == nil || got.Order.ID == "" || got.Order.BookingID != "42" || got.Order.CourseID != "paid-course" || got.Order.OutTradeNo != "crs-lost-callback" || got.Order.Status != "paid" || got.Order.Title != "原课程名称" || got.Order.Amount != 19900 || got.Order.CreateTime == "" || got.Order.PaidAt == "" {
			t.Fatalf("historical receipt changed or missing: %+v", got.Order)
		}
		if got.Course.Title != "后台调整后的课程" || got.Course.Schedule != "10 月 18 日 09:30" || got.Course.Location != "九型课堂 201" || got.Course.Duration != "2 天" || got.Course.Notice != "请提前 10 分钟到场" || got.Course.Cover != "/updated-cover.jpg" || len(got.Course.Outline) != 2 || len(got.Course.Bullets) != 1 || !got.Course.Enabled {
			t.Fatalf("current arrangements missing: %+v", got.Course)
		}
		if got.CustomerServiceQr != "https://configured.example/teacher-support.png" {
			t.Fatalf("support QR did not come from configuration: %q", got.CustomerServiceQr)
		}
	}
	var bookings, orders, messages int
	if err := s.db.QueryRow(`SELECT (SELECT count(*) FROM bookings),(SELECT count(*) FROM orders),(SELECT count(*) FROM messages)`).Scan(&bookings, &orders, &messages); err != nil {
		t.Fatal(err)
	}
	if bookings != 1 || orders != 1 || messages != 1 {
		t.Fatalf("reading enrollment created records: %d/%d/%d", bookings, orders, messages)
	}
}

func TestCourseEnrollmentPostgresAccessBoundaries(t *testing.T) {
	s, mux := courseCheckoutServer(t)
	settleEnrollment(t, s)
	for _, tt := range []struct {
		uid   int64
		query string
		code  int
	}{
		{0, "bookingId=42", http.StatusUnauthorized},
		{1, "", http.StatusBadRequest},
		{1, "bookingId=42&courseId=paid-course", http.StatusBadRequest},
		{1, "bookingId=-1", http.StatusBadRequest},
		{1, "bookingId=not-an-id", http.StatusBadRequest},
		{1, "bookingId=99999", http.StatusNotFound},
		{2, "bookingId=42&paid=true&owned=true", http.StatusNotFound},
		{1, "courseId=missing", http.StatusNotFound},
	} {
		response := courseCheckoutRequest(mux, tt.uid, http.MethodGet, "/api/miniapp/course/enrollment?"+tt.query, "")
		if response.Code != tt.code {
			t.Errorf("user=%d query=%s: status=%d want=%d body=%s", tt.uid, tt.query, response.Code, tt.code, response.Body.String())
		}
	}
	for _, query := range []string{"courseId=paid-course", "courseId=paid-course&paid=true&owned=true&bookingStatus=paid"} {
		got := readEnrollment(t, mux, 2, query)
		if got.Owned || got.Order != nil || got.BookingID != "" || got.CourseID != "paid-course" || !got.CatalogAvailable || got.CustomerServiceQr != "" {
			t.Fatalf("unpaid user received another user's ownership or invented support: %+v", got)
		}
	}
}

func TestCourseEnrollmentPostgresPaymentRecordRequired(t *testing.T) {
	s, mux := courseCheckoutServer(t)
	if _, err := s.db.Exec(`INSERT INTO bookings(id,wx_user_id,course_id,course_title,price_cents,payment_mode,payment_status) VALUES(42,1,'paid-course','原课程名称',19900,'paid','paid');
 INSERT INTO orders(out_trade_no,wx_user_id,product,ref_id,title,amount,status) VALUES
 ('unrelated-product',1,'report',42,'其他商品',19900,'paid'),('foreign-order',2,'course_booking',42,'其他用户的订单',19900,'paid')`); err != nil {
		t.Fatal(err)
	}
	for _, query := range []string{"bookingId=42&paid=true", "courseId=paid-course&owned=true"} {
		got := readEnrollment(t, mux, 1, query)
		if got.Owned || got.Order != nil {
			t.Fatalf("booking flag, unrelated product or foreign order granted ownership: %+v", got)
		}
	}
}

func TestCourseEnrollmentPostgresDisabledAndDeletedCatalog(t *testing.T) {
	s, mux := courseCheckoutServer(t)
	settleEnrollment(t, s)
	// Access by ID must not depend on the latest-50 bookings list.
	if _, err := s.db.Exec(`INSERT INTO bookings(id,wx_user_id,course_id) SELECT 100+n,1,'other-course' FROM generate_series(1,60) AS n`); err != nil {
		t.Fatal(err)
	}
	setEnrollmentCatalog(t, s, []siteconfig.MiniappCourse{{ID: "paid-course", Title: "已下架课程", Enabled: false, Schedule: "原教室如期上课", PriceCents: 39900}})
	for _, query := range []string{"bookingId=42", "courseId=paid-course"} {
		got := readEnrollment(t, mux, 1, query)
		if !got.Owned || !got.CatalogAvailable || got.Course.Enabled || got.Course.Schedule != "原教室如期上课" || got.Order == nil || got.Order.Title != "原课程名称" || got.Order.Amount != 19900 {
			t.Fatalf("disabled paid course lost: %+v", got)
		}
	}
	setEnrollmentCatalog(t, s, []siteconfig.MiniappCourse{})
	for _, query := range []string{"bookingId=42", "courseId=paid-course"} {
		got := readEnrollment(t, mux, 1, query)
		if !got.Owned || got.CatalogAvailable || got.Course.Enabled || got.Course.ID != "paid-course" || got.Course.Title != "原课程名称" || got.Course.Schedule != "" || got.Course.Location != "" || got.Course.Description != "" || len(got.Course.Outline) != 0 || got.Course.Outline == nil || got.Course.Bullets == nil || got.Order == nil || got.Order.Amount != 19900 {
			t.Fatalf("deleted paid course history/fallback invalid: %+v", got)
		}
	}
}

func TestCourseEnrollmentPostgresDisabledArrangementsRequireOwnership(t *testing.T) {
	for _, state := range []string{"unpurchased", "pending", "refunded"} {
		t.Run(state, func(t *testing.T) {
			s, mux := courseCheckoutServer(t)
			if state != "unpurchased" {
				installPendingCourse(t, s)
				if state == "refunded" {
					if _, err := s.db.Exec(`UPDATE orders SET status='refunded'; UPDATE bookings SET payment_status='paid'`); err != nil {
						t.Fatal(err)
					}
				}
			}
			setEnrollmentCatalog(t, s, []siteconfig.MiniappCourse{{
				ID: "paid-course", Title: "未发布的内部安排", Enabled: false,
				Schedule: "内部排课时间", Location: "内部教室", Description: "内部课程介绍",
				Cover: "/private-draft.jpg", Outline: []string{"内部课纲"}, Notice: "内部通知",
			}})
			if state == "unpurchased" {
				response := courseCheckoutRequest(mux, 1, http.MethodGet, "/api/miniapp/course/enrollment?courseId=paid-course", "")
				if response.Code != http.StatusNotFound {
					t.Fatalf("disabled catalog visible without purchase: %d %s", response.Code, response.Body.String())
				}
				return
			}
			for _, query := range []string{"bookingId=42", "courseId=paid-course"} {
				got := readEnrollment(t, mux, 1, query)
				if got.Owned || !got.CatalogAvailable || got.Order == nil || got.Order.Status != state || got.Order.Title != "原课程名称" || got.Order.Amount != 19900 {
					t.Fatalf("historical order status lost: %+v", got)
				}
				if got.Course.ID != "paid-course" || got.Course.Title != "原课程名称" || got.Course.Enabled || got.Course.Schedule != "" || got.Course.Location != "" || got.Course.Description != "" || got.Course.Cover != "" || got.Course.Notice != "" || len(got.Course.Outline) != 0 {
					t.Fatalf("unowned order exposed disabled arrangements: %+v", got.Course)
				}
			}
		})
	}
}

func TestCourseEnrollmentPostgresRecoversLostCallback(t *testing.T) {
	for _, query := range []string{"bookingId=42", "courseId=paid-course"} {
		t.Run(query, func(t *testing.T) {
			s, mux := courseCheckoutServer(t)
			installPendingCourse(t, s)
			gateway := &coursePaymentStub{query: func(_ context.Context, id string, _ int) (wxpay.CallbackResult, error) {
				return paidCourseQuery(id), nil
			}}
			s.coursePay = gateway
			got := readEnrollment(t, mux, 1, query)
			if !got.Owned || got.SyncStatus != "confirmed" || got.Order == nil || got.Order.Status != "paid" || got.Order.PaidAt == "" {
				t.Fatalf("verified provider payment did not produce enrollment: %+v", got)
			}
			var orderStatus, bookingPayment string
			var messages int
			if err := s.db.QueryRow(`SELECT o.status,b.payment_status,(SELECT count(*) FROM messages WHERE event_key='miniapp.course.paid') FROM orders o JOIN bookings b ON b.id=o.ref_id WHERE o.out_trade_no='crs-lost-callback'`).Scan(&orderStatus, &bookingPayment, &messages); err != nil {
				t.Fatal(err)
			}
			if orderStatus != "paid" || bookingPayment != "paid" || messages != 1 {
				t.Fatalf("recovered payment not persisted: %s/%s/%d", orderStatus, bookingPayment, messages)
			}
			if queries, prepays := gateway.counts(); queries != 1 || prepays != 0 {
				t.Fatalf("enrollment should query once without cashier: %d/%d", queries, prepays)
			}
		})
	}
}

func TestCourseEnrollmentPostgresUnconfirmedAndRefundedRemainUnowned(t *testing.T) {
	for _, state := range []string{"pending", "query-error", "refunded"} {
		t.Run(state, func(t *testing.T) {
			s, mux := courseCheckoutServer(t)
			installPendingCourse(t, s)
			gateway := &coursePaymentStub{}
			s.coursePay = gateway
			if state == "query-error" {
				gateway.query = func(context.Context, string, int) (wxpay.CallbackResult, error) {
					return wxpay.CallbackResult{}, errors.New("temporary provider failure")
				}
			}
			if state == "refunded" {
				if _, err := s.db.Exec(`UPDATE orders SET status='refunded'; UPDATE bookings SET payment_status='paid'`); err != nil {
					t.Fatal(err)
				}
				gateway.query = func(_ context.Context, id string, _ int) (wxpay.CallbackResult, error) {
					return paidCourseQuery(id), nil
				}
			}
			got := readEnrollment(t, mux, 1, "bookingId=42&paid=true&owned=true")
			if got.Owned || got.Order == nil || got.Order.Status == "paid" {
				t.Fatalf("unconfirmed/refunded payment granted ownership: %+v", got)
			}
			if state == "query-error" && (got.SyncStatus != "retrying" || got.Order.Status != "pending") {
				t.Fatalf("query failure must remain retrying: %+v", got)
			}
			if state == "refunded" {
				queries, prepays := gateway.counts()
				if got.Order.Status != "refunded" || queries != 0 || prepays != 0 {
					t.Fatalf("refunded order was retried: %+v; calls=%d/%d", got, queries, prepays)
				}
			}
		})
	}
}

func TestCourseEnrollmentPostgresReadFailureDoesNotClaimUnpurchased(t *testing.T) {
	s, mux := courseCheckoutServer(t)
	if _, err := s.db.Exec(`DROP TABLE orders`); err != nil {
		t.Fatal(err)
	}
	response := courseCheckoutRequest(mux, 1, http.MethodGet, "/api/miniapp/course/enrollment?courseId=paid-course", "")
	if response.Code != http.StatusInternalServerError {
		t.Fatalf("DB failure must not be a successful unowned response: %d %s", response.Code, response.Body.String())
	}
}
