package server

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"log"
	"strconv"
	"strings"
	"sync"
	"time"

	"nine-xing/nx-backend/apps/server/internal/miniapp"
	"nine-xing/nx-backend/apps/server/internal/wxpay"
)

type coursePaymentGateway interface {
	Prepay(context.Context, string, string, string, int) (wxpay.PrepayResult, error)
	QueryOrder(context.Context, string) (wxpay.CallbackResult, error)
	CloseOrder(context.Context, string) error
}

const coursePaymentQueryTimeout = 3 * time.Second
const coursePaymentQueryInterval = 2 * time.Second

type coursePaymentSyncOutcome struct {
	Retry    bool
	NotFound bool
}
type coursePaymentSyncEntry struct {
	done    chan struct{}
	expires time.Time
	outcome coursePaymentSyncOutcome
}
type coursePaymentSyncCache struct {
	mu      sync.Mutex
	entries map[string]*coursePaymentSyncEntry
}

func (s *Server) coursePaymentClient() coursePaymentGateway {
	if s.coursePay != nil {
		return s.coursePay
	}
	if s.pay != nil {
		return s.pay
	}
	return nil
}

func courseProviderError(err error) (string, int) {
	var provider *wxpay.HTTPError
	if !errors.As(err, &provider) {
		return "TRANSPORT_ERROR", 0
	}
	var payload struct {
		Code string `json:"code"`
	}
	if json.Unmarshal([]byte(provider.Body), &payload) != nil {
		return "INVALID_ERROR", provider.StatusCode
	}
	// Provider error bodies can contain request data. Log only their short code.
	if len(payload.Code) == 0 || len(payload.Code) > 64 {
		return "UNKNOWN", provider.StatusCode
	}
	for _, r := range payload.Code {
		if !(r >= 'A' && r <= 'Z' || r >= '0' && r <= '9' || r == '_') {
			return "UNKNOWN", provider.StatusCode
		}
	}
	return payload.Code, provider.StatusCode
}

// syncCoursePayment bounds external work and shares in-flight/recent queries.
// force bypasses a cached NOTPAY result when Prepay reports ORDERPAID.
func (s *Server) syncCoursePayment(ctx context.Context, order courseOrderResponse, force bool) coursePaymentSyncOutcome {
	if order.Status == "paid" {
		return coursePaymentSyncOutcome{}
	}
	gateway := s.coursePaymentClient()
	if gateway == nil {
		return coursePaymentSyncOutcome{Retry: true}
	}
	if s.coursePay == nil && s.pay.DevMode() {
		return coursePaymentSyncOutcome{}
	}
	now := time.Now()
	s.courseSync.mu.Lock()
	if s.courseSync.entries == nil {
		s.courseSync.entries = make(map[string]*coursePaymentSyncEntry)
	}
	if entry := s.courseSync.entries[order.OutTradeNo]; entry != nil {
		select {
		case <-entry.done:
			if !force && entry.expires.After(now) {
				outcome := entry.outcome
				s.courseSync.mu.Unlock()
				return outcome
			}
		default:
			done := entry.done
			s.courseSync.mu.Unlock()
			select {
			case <-done:
				return entry.outcome
			case <-ctx.Done():
				return coursePaymentSyncOutcome{Retry: true}
			}
		}
	}
	// Retain only a small time window, rather than accumulating user order IDs.
	for key, entry := range s.courseSync.entries {
		select {
		case <-entry.done:
			if entry.expires.Before(now) {
				delete(s.courseSync.entries, key)
			}
		default:
		}
	}
	entry := &coursePaymentSyncEntry{done: make(chan struct{})}
	s.courseSync.entries[order.OutTradeNo] = entry
	s.courseSync.mu.Unlock()
	outcome := s.queryAndApplyCoursePayment(ctx, gateway, order)
	s.courseSync.mu.Lock()
	entry.outcome = outcome
	entry.expires = time.Now().Add(coursePaymentQueryInterval)
	close(entry.done)
	s.courseSync.mu.Unlock()
	return outcome
}

func (s *Server) queryAndApplyCoursePayment(ctx context.Context, gateway coursePaymentGateway, order courseOrderResponse) coursePaymentSyncOutcome {
	queryCtx, cancel := context.WithTimeout(ctx, coursePaymentQueryTimeout)
	defer cancel()
	result, err := gateway.QueryOrder(queryCtx, order.OutTradeNo)
	if err != nil {
		code, status := courseProviderError(err)
		if code == "ORDER_NOT_EXIST" {
			return coursePaymentSyncOutcome{NotFound: true}
		}
		log.Printf("[WXPAY] course query deferred: out_trade_no=%s provider_code=%s http_status=%d", order.OutTradeNo, code, status)
		return coursePaymentSyncOutcome{Retry: true}
	}
	// Verify the queried merchant order, rather than trusting a success flag or
	// the ORDERPAID error from a different API operation.
	valid := result.OutTradeNo == order.OutTradeNo && result.AppID == s.env.WxPay.AppID && result.MchID == s.env.WxPay.MchID && result.AmountTotal == order.Amount && order.Amount > 0
	if !valid {
		log.Printf("[WXPAY] course query rejected: out_trade_no=%s reason=identity_or_amount_mismatch", order.OutTradeNo)
		return coursePaymentSyncOutcome{Retry: true}
	}
	switch result.TradeState {
	case "SUCCESS":
		if !result.Success || strings.TrimSpace(result.TransactionID) == "" {
			log.Printf("[WXPAY] course query rejected: out_trade_no=%s reason=incomplete_success", order.OutTradeNo)
			return coursePaymentSyncOutcome{Retry: true}
		}
		apply, err := s.miniapp.MarkOrderPaidDetailed(queryCtx, order.OutTradeNo, result.TransactionID)
		if err != nil {
			log.Printf("[WXPAY] course payment settlement deferred: out_trade_no=%s", order.OutTradeNo)
			return coursePaymentSyncOutcome{Retry: true}
		}
		// Settlement is committed before closing competing orders. A close failure
		// must not hide a completed payment from its purchaser.
		if err := closeCompetingPendingOrders(queryCtx, gateway, apply.PendingToClose); err != nil {
			code, status := courseProviderError(err)
			log.Printf("[WXPAY] course sibling close deferred: out_trade_no=%s provider_code=%s http_status=%d", order.OutTradeNo, code, status)
		} else if err := s.miniapp.ClosePendingOrders(queryCtx, apply.PendingToClose); err != nil {
			log.Printf("[WXPAY] course sibling finalize deferred: out_trade_no=%s", order.OutTradeNo)
		}
	case "CLOSED", "REVOKED", "PAYERROR":
		if _, err := s.db.ExecContext(queryCtx, `UPDATE orders SET status='closed',update_time=now() WHERE out_trade_no=$1 AND status='pending'`, order.OutTradeNo); err != nil {
			return coursePaymentSyncOutcome{Retry: true}
		}
	case "NOTPAY", "USERPAYING":
	default:
		return coursePaymentSyncOutcome{Retry: true}
	}
	return coursePaymentSyncOutcome{}
}

const courseOrderSnapshotSelect = `SELECT o.out_trade_no,o.product,o.ref_id::text,b.id::text,o.title,o.amount,o.status,b.course_id,COALESCE(to_char(o.paid_at AT TIME ZONE 'Asia/Shanghai','YYYY/MM/DD HH24:MI:SS'),'')
 FROM orders o JOIN bookings b ON o.product='course_booking' AND b.id=o.ref_id AND b.wx_user_id=o.wx_user_id`

func (s *Server) findCourseOrderSnapshot(ctx context.Context, uid int64, courseID string, bookingID int64) (courseOrderResponse, error) {
	where := ` WHERE o.wx_user_id=$1`
	args := []any{uid}
	if courseID != "" {
		args = append(args, courseID)
		where += ` AND b.course_id=$` + strconv.Itoa(len(args))
	}
	if bookingID > 0 {
		args = append(args, bookingID)
		where += ` AND b.id=$` + strconv.Itoa(len(args))
	}
	var order courseOrderResponse
	err := s.db.QueryRowContext(ctx, courseOrderSnapshotSelect+where+` ORDER BY (o.status='paid') DESC,o.create_time DESC,o.id DESC LIMIT 1`, args...).Scan(&order.OutTradeNo, &order.Product, &order.RefID, &order.BookingID, &order.Title, &order.Amount, &order.Status, &order.CourseID, &order.PaidAt)
	return order, err
}

// Existing course orders are resolved before consulting today's catalog. A
// price edit or course removal must not hide an already completed purchase.
func (s *Server) existingCourseCheckout(ctx context.Context, uid int64, courseID string, bookingID int64) (courseOrderResponse, bool, error) {
	order, err := s.findCourseOrderSnapshot(ctx, uid, courseID, bookingID)
	if errors.Is(err, sql.ErrNoRows) {
		return courseOrderResponse{}, false, nil
	}
	if err != nil {
		return courseOrderResponse{}, false, err
	}
	if order.Status == "paid" {
		order.SyncStatus = "confirmed"
		return order, true, nil
	}
	if order.Status != "pending" && order.Status != "closed" {
		return order, false, nil
	}
	outcome := s.syncCoursePayment(ctx, order, false)
	latest, err := s.findCourseOrderSnapshot(ctx, uid, "", mustCourseBookingID(order.BookingID))
	if err != nil {
		return courseOrderResponse{}, false, err
	}
	if latest.Status == "paid" {
		latest.SyncStatus = "confirmed"
		return latest, true, nil
	}
	if outcome.Retry {
		latest.Status = "pending"
		latest.SyncStatus = "retrying"
		latest.Message = "正在同步微信支付结果，请稍后查看"
		return latest, true, nil
	}
	return latest, false, nil
}
func mustCourseBookingID(value string) int64 { id, _ := strconv.ParseInt(value, 10, 64); return id }

func courseOrderFromPending(order miniapp.Order, booking miniapp.CourseBooking) courseOrderResponse {
	return courseOrderResponse{OutTradeNo: order.OutTradeNo, Product: order.Product, RefID: order.RefID, BookingID: booking.ID, Title: order.Title, Amount: order.Amount, Status: order.Status, CourseID: booking.CourseID}
}
