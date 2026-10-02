package server

import (
	"context"
	"database/sql"
	"errors"
	"net/http"
	"strconv"
	"strings"

	"nine-xing/nx-backend/apps/server/internal/httpx"
	"nine-xing/nx-backend/apps/server/internal/siteconfig"
)

// The receipt is an immutable purchase snapshot. Arrangements are today's
// administrator configuration, so title/amount never come from the catalog.
type courseEnrollmentOrder struct {
	ID         string `json:"id"`
	OutTradeNo string `json:"outTradeNo"`
	BookingID  string `json:"bookingId"`
	CourseID   string `json:"courseId"`
	Title      string `json:"title"`
	Amount     int    `json:"amount"`
	Status     string `json:"status"`
	CreateTime string `json:"createTime"`
	PaidAt     string `json:"paidAt"`
}

type courseEnrollmentDetails struct {
	ID          string   `json:"id"`
	Title       string   `json:"title"`
	Subtitle    string   `json:"subtitle"`
	Description string   `json:"description"`
	Cover       string   `json:"cover"`
	Format      string   `json:"format"`
	Duration    string   `json:"duration"`
	Schedule    string   `json:"schedule"`
	Location    string   `json:"location"`
	Bullets     []string `json:"bullets"`
	Outline     []string `json:"outline"`
	Notice      string   `json:"notice"`
	Enabled     bool     `json:"enabled"`
}

type courseEnrollmentResponse struct {
	Owned             bool                    `json:"owned"`
	CourseID          string                  `json:"courseId"`
	BookingID         string                  `json:"bookingId"`
	BookingStatus     string                  `json:"bookingStatus"`
	SyncStatus        string                  `json:"syncStatus"`
	CatalogAvailable  bool                    `json:"catalogAvailable"`
	CustomerServiceQr string                  `json:"customerServiceQr"`
	Order             *courseEnrollmentOrder  `json:"order"`
	Course            courseEnrollmentDetails `json:"course"`
}

func (s *Server) courseEnrollment(w http.ResponseWriter, r *http.Request) {
	uid := userFromRequest(r).ID
	if uid <= 0 {
		httpx.Fail(w, http.StatusUnauthorized, "请先登录")
		return
	}
	query := r.URL.Query()
	courseID := strings.TrimSpace(query.Get("courseId"))
	rawBookingID := strings.TrimSpace(query.Get("bookingId"))
	if (courseID == "") == (rawBookingID == "") || len(query["courseId"]) > 1 || len(query["bookingId"]) > 1 {
		httpx.Fail(w, http.StatusBadRequest, "provide exactly one of courseId or bookingId")
		return
	}
	var bookingID int64
	if rawBookingID != "" {
		var err error
		bookingID, err = parseBookingID(rawBookingID)
		if err != nil {
			httpx.Fail(w, http.StatusBadRequest, err.Error())
			return
		}
	}
	if s.db == nil || s.miniapp == nil {
		httpx.Fail(w, http.StatusServiceUnavailable, "课程报名服务暂不可用")
		return
	}
	response, err := s.readCourseEnrollment(r.Context(), uid, courseID, bookingID)
	if errors.Is(err, sql.ErrNoRows) && bookingID == 0 {
		// A course can be browsed before purchase without creating a booking.
		response = courseEnrollmentResponse{CourseID: courseID, SyncStatus: "confirmed"}
	} else if err != nil {
		writeCourseOrderError(w, err)
		return
	}
	if order := response.Order; order != nil && (order.Status == "pending" || order.Status == "closed") {
		outcome := s.syncCoursePayment(r.Context(), courseOrderResponse{
			OutTradeNo: order.OutTradeNo, Product: "course_booking", RefID: order.BookingID,
			BookingID: order.BookingID, CourseID: order.CourseID, Title: order.Title,
			Amount: order.Amount, Status: order.Status, PaidAt: order.PaidAt,
		}, false)
		// Both callbacks and compensation settle in a transaction. Always reread
		// the persisted order; a provider/URL success flag cannot grant ownership.
		response, err = s.readCourseEnrollment(r.Context(), uid, courseID, bookingID)
		if err != nil {
			writeCourseOrderError(w, err)
			return
		}
		if outcome.Retry && !response.Owned {
			response.SyncStatus = "retrying"
		}
	}
	cfg, err := siteconfig.ReadStore(r.Context(), s.db, s.env.SiteConfig)
	if err != nil {
		httpx.Fail(w, http.StatusInternalServerError, "读取课程安排失败")
		return
	}
	courses, err := siteconfig.MiniappCourses(cfg)
	if err != nil {
		httpx.Fail(w, http.StatusInternalServerError, "读取课程安排失败")
		return
	}
	response.CustomerServiceQr = cfg.Site.CustomerServiceQr
	for _, course := range courses {
		if course.ID != response.CourseID {
			continue
		}
		response.CatalogAvailable = true
		if (!course.Enabled || !siteconfig.MiniappCoursesEnabled(cfg)) && !response.Owned {
			if response.Order == nil {
				writeCourseOrderError(w, errCourseBookingNotFound)
				return
			}
			// Keep the purchaser's pending/refunded receipt readable without
			// exposing unpublished arrangements to an unconfirmed enrollment.
			response.Course = courseEnrollmentDetails{ID: response.CourseID, Title: response.Order.Title}
			break
		}
		response.Course = courseEnrollmentDetails{
			ID: course.ID, Title: course.Title, Subtitle: course.Subtitle,
			Description: course.Description, Cover: course.Cover, Format: course.Format,
			Duration: course.Duration, Schedule: course.Schedule, Location: course.Location,
			Bullets: course.Bullets, Outline: course.Outline, Notice: course.Notice, Enabled: course.Enabled,
		}
		break
	}
	if !response.CatalogAvailable && response.BookingID == "" {
		writeCourseOrderError(w, errCourseBookingNotFound)
		return
	}
	// Deleted catalog entries retain only recorded historical details. Do not
	// invent a past schedule or turn the course into unrelated video access.
	if !response.CatalogAvailable && response.Order != nil {
		response.Course.Title = response.Order.Title
	}
	response.Course.ID = response.CourseID
	if response.Course.Bullets == nil {
		response.Course.Bullets = []string{}
	}
	if response.Course.Outline == nil {
		response.Course.Outline = []string{}
	}
	w.Header().Set("Cache-Control", "no-store")
	httpx.OK(w, response)
}

func (s *Server) readCourseEnrollment(ctx context.Context, uid int64, courseID string, bookingID int64) (courseEnrollmentResponse, error) {
	where := ` WHERE b.wx_user_id=$1 AND b.course_id<>''`
	args := []any{uid}
	if courseID != "" {
		args = append(args, courseID)
		where += ` AND b.course_id=$` + strconv.Itoa(len(args))
	}
	if bookingID > 0 {
		args = append(args, bookingID)
		where += ` AND b.id=$` + strconv.Itoa(len(args))
	}
	var response courseEnrollmentResponse
	var order courseEnrollmentOrder
	err := s.db.QueryRowContext(ctx, `SELECT b.id::text,b.course_id,b.status,b.course_title,
 COALESCE(o.id::text,''),COALESCE(o.out_trade_no,''),COALESCE(o.title,''),COALESCE(o.amount,0),COALESCE(o.status,''),
 COALESCE(to_char(o.create_time AT TIME ZONE 'Asia/Shanghai','YYYY/MM/DD HH24:MI:SS'),''),
 COALESCE(to_char(o.paid_at AT TIME ZONE 'Asia/Shanghai','YYYY/MM/DD HH24:MI:SS'),'')
 FROM bookings b LEFT JOIN orders o ON o.product='course_booking' AND o.ref_id=b.id AND o.wx_user_id=b.wx_user_id`+where+`
 ORDER BY (o.status='paid') DESC NULLS LAST,o.create_time DESC NULLS LAST,o.id DESC NULLS LAST,b.create_time DESC,b.id DESC LIMIT 1`, args...).Scan(
		&response.BookingID, &response.CourseID, &response.BookingStatus, &response.Course.Title,
		&order.ID, &order.OutTradeNo, &order.Title, &order.Amount, &order.Status, &order.CreateTime, &order.PaidAt,
	)
	if err != nil {
		return courseEnrollmentResponse{}, err
	}
	response.SyncStatus = "confirmed"
	if order.ID != "" {
		order.BookingID, order.CourseID = response.BookingID, response.CourseID
		response.Order = &order
		response.Owned = order.Status == "paid"
	}
	return response, nil
}
