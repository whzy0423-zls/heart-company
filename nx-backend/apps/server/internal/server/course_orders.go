package server

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"log"
	"net/http"
	"strconv"
	"strings"
	"time"

	"nine-xing/nx-backend/apps/server/internal/config"
	"nine-xing/nx-backend/apps/server/internal/httpx"
	"nine-xing/nx-backend/apps/server/internal/miniapp"
	"nine-xing/nx-backend/apps/server/internal/siteconfig"
	"nine-xing/nx-backend/apps/server/internal/wxpay"
)

var (
	errCourseBookingNotFound     = errors.New("course booking not found")
	errCourseBookingNotPayable   = errors.New("course booking is not payable")
	errCourseBookingPriceChanged = errors.New("课程价格已调整，请重新确认课程后报名")
)

type courseOrderResponse struct {
	OutTradeNo string              `json:"outTradeNo"`
	Product    string              `json:"product"`
	RefID      string              `json:"refId"`
	BookingID  string              `json:"bookingId"`
	Title      string              `json:"title"`
	Amount     int                 `json:"amount"`
	PayParams  *wxpay.PrepayResult `json:"payParams,omitempty"`
	Status     string              `json:"status"`
	CourseID   string              `json:"courseId"`
	PaidAt     string              `json:"paidAt,omitempty"`
	SyncStatus string              `json:"syncStatus,omitempty"`
	Message    string              `json:"message,omitempty"`
}
type courseOrderStatusResponse struct {
	Status     string `json:"status"`
	Amount     int    `json:"amount"`
	OutTradeNo string `json:"outTradeNo,omitempty"`
	BookingID  string `json:"bookingId"`
	Title      string `json:"title"`
	CourseID   string `json:"courseId"`
	PaidAt     string `json:"paidAt,omitempty"`
	SyncStatus string `json:"syncStatus,omitempty"`
	Message    string `json:"message,omitempty"`
}
type courseOrderRequest struct {
	BookingID string `json:"bookingId"`
	CourseID  string `json:"courseId"`
}

func registerCourseOrderRoutes(mux *http.ServeMux, authn func(http.HandlerFunc) http.HandlerFunc, s *Server) {
	mux.HandleFunc("/api/miniapp/orders", s.method(http.MethodGet, authn(s.miniappOrders)))
	mux.HandleFunc("/api/miniapp/course/enrollment", s.method(http.MethodGet, authn(s.courseEnrollment)))
	mux.HandleFunc("/api/miniapp/course/orders", s.method(http.MethodPost, authn(s.courseOrderCreate)))
	mux.HandleFunc("/api/miniapp/course/orders/status", s.method(http.MethodGet, authn(s.courseOrderStatus)))
	mux.HandleFunc("/api/miniapp/course/orders/dev-pay", s.method(http.MethodPost, authn(s.courseOrderDevPay)))
}

func parseBookingID(raw string) (int64, error) {
	id, err := strconv.ParseInt(strings.TrimSpace(raw), 10, 64)
	if err != nil || id <= 0 {
		return 0, errors.New("bookingId is required")
	}
	return id, nil
}

func (s *Server) readMiniappCourse(ctx context.Context, id string) (siteconfig.MiniappCourse, error) {
	cfg, err := siteconfig.ReadStore(ctx, s.db, s.env.SiteConfig)
	if err != nil {
		return siteconfig.MiniappCourse{}, err
	}
	items, err := siteconfig.MiniappCourses(cfg)
	if err != nil {
		return siteconfig.MiniappCourse{}, err
	}
	for _, item := range items {
		if item.ID == strings.TrimSpace(id) {
			return item, nil
		}
	}
	return siteconfig.MiniappCourse{}, errCourseBookingNotFound
}

func (s *Server) courseOrderCreate(w http.ResponseWriter, r *http.Request) {
	if s.miniapp == nil {
		httpx.Fail(w, http.StatusServiceUnavailable, "course payment is not configured")
		return
	}
	var body courseOrderRequest
	dec := json.NewDecoder(http.MaxBytesReader(w, r.Body, 8*1024))
	if err := dec.Decode(&body); err != nil {
		httpx.Fail(w, http.StatusBadRequest, "Invalid JSON payload")
		return
	}
	body.BookingID, body.CourseID = strings.TrimSpace(body.BookingID), strings.TrimSpace(body.CourseID)
	if (body.BookingID == "") == (body.CourseID == "") {
		httpx.Fail(w, http.StatusBadRequest, "provide exactly one of courseId or bookingId")
		return
	}
	uid := userFromRequest(r).ID
	if uid <= 0 {
		httpx.Fail(w, http.StatusUnauthorized, "请先登录")
		return
	}
	var targetBookingID int64
	if body.BookingID != "" {
		var parseErr error
		targetBookingID, parseErr = parseBookingID(body.BookingID)
		if parseErr != nil {
			httpx.Fail(w, http.StatusBadRequest, parseErr.Error())
			return
		}
		if _, err := s.miniapp.CourseBooking(r.Context(), uid, targetBookingID); err != nil {
			writeCourseOrderError(w, err)
			return
		}
	}
	if existing, done, err := s.existingCourseCheckout(r.Context(), uid, body.CourseID, targetBookingID); err != nil {
		writeCourseOrderError(w, err)
		return
	} else if done {
		httpx.OK(w, existing)
		return
	}
	// Resolve an existing successful payment first so disabling enrollment
	// cannot hide a receipt or prevent reconciliation of an earlier payment.
	// Every new/retried prepay must still honor the current enrollment switch.
	config, err := siteconfig.ReadStore(r.Context(), s.db, s.env.SiteConfig)
	if err != nil {
		httpx.Fail(w, http.StatusInternalServerError, "课程配置读取失败")
		return
	}
	if !siteconfig.MiniappCoursesEnabled(config) {
		httpx.Fail(w, http.StatusConflict, "课程报名已关闭，请填写报名意向表单")
		return
	}
	if !s.requireMiniappPayment(w, r) {
		return
	}
	gateway := s.coursePaymentClient()
	if gateway == nil {
		httpx.Fail(w, http.StatusServiceUnavailable, "course payment is not configured")
		return
	}
	var booking miniapp.CourseBooking
	var course siteconfig.MiniappCourse
	if body.CourseID != "" {
		course, err = s.readMiniappCourse(r.Context(), body.CourseID)
		if err == nil {
			if !course.Enabled {
				httpx.Fail(w, http.StatusConflict, "课程报名已下架")
				return
			}
			if course.PriceCents <= 0 {
				httpx.Fail(w, http.StatusConflict, "该课程需要咨询确认")
				return
			}
			booking, err = s.miniapp.CreateOrReuseCourseBooking(r.Context(), uid, course)
		}
	} else {
		id, parseErr := parseBookingID(body.BookingID)
		if parseErr != nil {
			httpx.Fail(w, http.StatusBadRequest, parseErr.Error())
			return
		}
		booking, err = s.miniapp.CourseBooking(r.Context(), uid, id)
		if err == nil {
			course, err = s.readMiniappCourse(r.Context(), booking.CourseID)
		}
	}
	if errors.Is(err, miniapp.ErrOrderAlreadyOwned) {
		if paid, lookupErr := s.findCourseOrderSnapshot(r.Context(), uid, body.CourseID, targetBookingID); lookupErr == nil && paid.Status == "paid" {
			paid.SyncStatus = "confirmed"
			httpx.OK(w, paid)
			return
		}
	}
	if err != nil {
		writeCourseOrderError(w, err)
		return
	}
	id, _ := parseBookingID(booking.ID)
	if !course.Enabled {
		httpx.Fail(w, http.StatusConflict, "课程报名已下架")
		return
	}
	if booking.PaymentStatus == "paid" {
		paid, err := s.findCourseOrderSnapshot(r.Context(), uid, "", id)
		if err != nil {
			writeCourseOrderError(w, err)
			return
		}
		httpx.OK(w, paid)
		return
	}
	if course.PaymentMode != booking.PaymentMode || course.PriceCents != booking.PriceCents {
		httpx.Fail(w, http.StatusConflict, errCourseBookingPriceChanged.Error())
		return
	}
	if course.PaymentMode != "paid" || course.PriceCents <= 0 || booking.PaymentMode != "paid" || booking.PriceCents <= 0 {
		httpx.Fail(w, http.StatusConflict, "该课程需要咨询确认")
		return
	}
	if booking.PaymentStatus != "pending" {
		httpx.Fail(w, http.StatusConflict, errCourseBookingNotPayable.Error())
		return
	}
	openid, err := s.miniapp.OpenIDByUserID(r.Context(), uid)
	if err != nil {
		httpx.Fail(w, http.StatusInternalServerError, "获取微信用户标识失败")
		return
	}
	outTradeNo, err := generateCourseBookingOutTradeNo(uid, id)
	if err != nil {
		httpx.Fail(w, http.StatusInternalServerError, "创建课程订单失败")
		return
	}
	order, err := s.miniapp.CreateOrReusePendingOrder(r.Context(), uid, outTradeNo, miniapp.ProductCourseBooking, id, booking.CourseTitle, booking.PriceCents)
	if errors.Is(err, miniapp.ErrOrderAlreadyOwned) {
		if paid, lookupErr := s.findCourseOrderSnapshot(r.Context(), uid, booking.CourseID, 0); lookupErr == nil && paid.Status == "paid" {
			paid.SyncStatus = "confirmed"
			httpx.OK(w, paid)
			return
		}
		writeCourseOrderError(w, err)
		return
	}
	if errors.Is(err, miniapp.ErrPendingOrderSnapshotChanged) {
		httpx.Fail(w, http.StatusConflict, errCourseBookingPriceChanged.Error())
		return
	}
	if err != nil {
		httpx.Fail(w, http.StatusInternalServerError, "创建课程订单失败")
		return
	}
	response := courseOrderFromPending(order, booking)
	syncOutcome := s.syncCoursePayment(r.Context(), response, false)
	latest, err := s.findCourseOrderSnapshot(r.Context(), uid, "", id)
	if err != nil {
		writeCourseOrderError(w, err)
		return
	}
	if latest.Status == "paid" {
		latest.SyncStatus = "confirmed"
		httpx.OK(w, latest)
		return
	}
	if syncOutcome.Retry {
		response.SyncStatus = "retrying"
		response.Message = "正在同步微信支付结果，请稍后查看"
		httpx.OK(w, response)
		return
	}
	prepayCtx, cancel := context.WithTimeout(r.Context(), 7*time.Second)
	prepay, err := gateway.Prepay(prepayCtx, order.OutTradeNo, openid, "九型课堂报名·"+order.Title, order.Amount)
	cancel()
	if err != nil {
		code, httpStatus := courseProviderError(err)
		log.Printf("[WXPAY] course prepay failed: out_trade_no=%s provider_code=%s http_status=%d", order.OutTradeNo, code, httpStatus)
		if code == "ORDERPAID" {
			s.syncCoursePayment(r.Context(), response, true)
			latest, readErr := s.findCourseOrderSnapshot(r.Context(), uid, "", id)
			if readErr != nil {
				writeCourseOrderError(w, readErr)
				return
			}
			if latest.Status == "paid" {
				latest.SyncStatus = "confirmed"
				httpx.OK(w, latest)
				return
			}
			response.SyncStatus = "retrying"
			response.Message = "正在同步微信支付结果，请稍后查看"
			httpx.OK(w, response)
			return
		}
		httpx.Fail(w, http.StatusBadGateway, "微信支付下单失败，请稍后重试")
		return
	}
	if err := validateClassroomPaymentParams(s.env, prepay); err != nil {
		httpx.Fail(w, http.StatusBadGateway, "invalid payment parameters")
		return
	}
	response.PayParams = &prepay
	response.SyncStatus = "confirmed"
	httpx.OK(w, response)
}

func (s *Server) courseOrderStatus(w http.ResponseWriter, r *http.Request) {
	if userFromRequest(r).ID <= 0 {
		httpx.Fail(w, http.StatusUnauthorized, "请先登录")
		return
	}
	id, err := parseBookingID(r.URL.Query().Get("bookingId"))
	if err != nil {
		httpx.Fail(w, http.StatusBadRequest, err.Error())
		return
	}
	booking, err := s.miniapp.CourseBooking(r.Context(), userFromRequest(r).ID, id)
	if err != nil {
		writeCourseOrderError(w, err)
		return
	}
	response := courseOrderStatusResponse{Status: booking.PaymentStatus, Amount: booking.PriceCents, BookingID: booking.ID, Title: booking.CourseTitle, CourseID: booking.CourseID, SyncStatus: "confirmed"}
	order, orderErr := s.findCourseOrderSnapshot(r.Context(), userFromRequest(r).ID, "", id)
	if orderErr != nil && !errors.Is(orderErr, sql.ErrNoRows) {
		httpx.Fail(w, http.StatusInternalServerError, "读取课程订单状态失败")
		return
	}
	if orderErr == nil {
		outcome := s.syncCoursePayment(r.Context(), order, false)
		order, orderErr = s.findCourseOrderSnapshot(r.Context(), userFromRequest(r).ID, "", id)
		if orderErr != nil {
			httpx.Fail(w, http.StatusInternalServerError, "读取课程订单状态失败")
			return
		}
		response.Status = order.Status
		response.Amount = order.Amount
		response.Title = order.Title
		response.OutTradeNo = order.OutTradeNo
		response.PaidAt = order.PaidAt
		if outcome.Retry && order.Status != "paid" {
			response.Status = "pending"
			response.SyncStatus = "retrying"
			response.Message = "正在同步微信支付结果，请稍后查看"
		}
	}
	httpx.OK(w, response)
}

func (s *Server) courseOrderDevPay(w http.ResponseWriter, r *http.Request) {
	if config.IsProduction(s.env.AppEnv) || s.pay == nil || !s.pay.DevMode() || s.miniapp == nil {
		httpx.Fail(w, http.StatusNotFound, "Not Found")
		return
	}
	if !s.requireMiniappPayment(w, r) {
		return
	}
	var body struct {
		OutTradeNo string `json:"outTradeNo"`
	}
	if json.NewDecoder(http.MaxBytesReader(w, r.Body, 8*1024)).Decode(&body) != nil || strings.TrimSpace(body.OutTradeNo) == "" {
		httpx.Fail(w, http.StatusBadRequest, "outTradeNo is required")
		return
	}
	_, owner, refID, product, _, err := s.miniapp.OrderByOutTradeNo(r.Context(), body.OutTradeNo)
	if err != nil {
		writeCourseOrderError(w, err)
		return
	}
	if owner != userFromRequest(r).ID || product != miniapp.ProductCourseBooking {
		httpx.Fail(w, http.StatusNotFound, "Not Found")
		return
	}
	if _, err = s.miniapp.MarkOrderPaidDetailed(r.Context(), body.OutTradeNo, "dev-"+body.OutTradeNo); err != nil {
		writeCourseOrderError(w, err)
		return
	}
	httpx.OK(w, map[string]any{"paid": true, "bookingId": strconv.FormatInt(refID, 10)})
}

func writeCourseOrderError(w http.ResponseWriter, err error) {
	switch {
	case errors.Is(err, sql.ErrNoRows), errors.Is(err, errCourseBookingNotFound):
		httpx.Fail(w, http.StatusNotFound, "course booking not found")
	case errors.Is(err, errCourseBookingPriceChanged):
		httpx.Fail(w, http.StatusConflict, err.Error())
	case errors.Is(err, miniapp.ErrOrderAlreadyOwned):
		httpx.Fail(w, http.StatusConflict, "该报名已支付")
	default:
		httpx.Fail(w, http.StatusInternalServerError, "course order failed")
	}
}

func generateCourseBookingOutTradeNo(uid, bookingID int64) (string, error) {
	suffix, err := randomHex(4)
	if err != nil {
		return "", err
	}
	return fmt.Sprintf("crs%d-%d-%s", uid, bookingID, suffix), nil
}
