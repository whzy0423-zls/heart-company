package server

import (
	"context"
	"crypto/rand"
	"database/sql"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"strconv"
	"strings"
	"time"

	"nine-xing/nx-backend/apps/server/internal/classroom"
	"nine-xing/nx-backend/apps/server/internal/config"
	"nine-xing/nx-backend/apps/server/internal/httpx"
	"nine-xing/nx-backend/apps/server/internal/miniapp"
	"nine-xing/nx-backend/apps/server/internal/wxpay"
)

var (
	ErrClassroomTargetNotForSale = errors.New("classroom target is not for sale")
	ErrClassroomAlreadyOwned     = errors.New("classroom target is already owned")
)

type classroomOrderTarget struct {
	Type  string
	RefID int64
}

type classroomOrderCheckout struct {
	OutTradeNo string             `json:"outTradeNo"`
	Product    string             `json:"product"`
	RefID      string             `json:"refId"`
	Title      string             `json:"title"`
	Amount     int                `json:"amount"`
	PayParams  wxpay.PrepayResult `json:"payParams"`
}

type classroomOrderStatus struct {
	OutTradeNo string `json:"outTradeNo,omitempty"`
	Product    string `json:"product"`
	RefID      string `json:"refId"`
	Title      string `json:"title,omitempty"`
	Amount     int    `json:"amount,omitempty"`
	Status     string `json:"status"`
	Owned      bool   `json:"owned"`
}

type classroomOrderService interface {
	Create(context.Context, int64, classroomOrderTarget) (classroomOrderCheckout, error)
	Status(context.Context, int64, classroomOrderTarget) (classroomOrderStatus, error)
	DevPay(context.Context, int64, string, string) (bool, error)
}

func registerClassroomOrderRoutes(mux *http.ServeMux, authn func(http.HandlerFunc) http.HandlerFunc, s *Server) {
	mux.HandleFunc("/api/miniapp/classroom/orders", s.method(http.MethodPost, authn(s.classroomOrderCreate)))
	mux.HandleFunc("/api/miniapp/classroom/orders/status", s.method(http.MethodGet, authn(s.classroomOrderStatus)))
	mux.HandleFunc("/api/miniapp/classroom/orders/dev-pay", s.method(http.MethodPost, authn(s.classroomOrderDevPay)))
}

type classroomOrderRequest struct {
	TargetType string `json:"targetType"`
	RefID      string `json:"refId"`
}

func parseClassroomOrderTarget(targetType, refID string) (classroomOrderTarget, error) {
	targetType = strings.TrimSpace(targetType)
	id, err := strconv.ParseInt(strings.TrimSpace(refID), 10, 64)
	if err != nil || id <= 0 || (targetType != "series" && targetType != "content") {
		return classroomOrderTarget{}, errors.New("targetType and refId are required")
	}
	return classroomOrderTarget{Type: targetType, RefID: id}, nil
}

func (s *Server) classroomOrderCreate(w http.ResponseWriter, r *http.Request) {
	if s.classroomOrders == nil {
		httpx.Fail(w, http.StatusServiceUnavailable, "classroom payment is not configured")
		return
	}
	var body classroomOrderRequest
	if err := json.NewDecoder(http.MaxBytesReader(w, r.Body, 8*1024)).Decode(&body); err != nil {
		httpx.Fail(w, http.StatusBadRequest, "Invalid JSON payload")
		return
	}
	target, err := parseClassroomOrderTarget(body.TargetType, body.RefID)
	if err != nil {
		httpx.Fail(w, http.StatusBadRequest, err.Error())
		return
	}
	result, err := s.classroomOrders.Create(r.Context(), userFromRequest(r).ID, target)
	if err != nil {
		writeClassroomOrderError(w, err)
		return
	}
	if err := validateClassroomPaymentParams(s.env, result.PayParams); err != nil {
		httpx.Fail(w, http.StatusBadGateway, "invalid payment parameters")
		return
	}
	httpx.OK(w, result)
}

func (s *Server) classroomOrderStatus(w http.ResponseWriter, r *http.Request) {
	if s.classroomOrders == nil {
		httpx.Fail(w, http.StatusServiceUnavailable, "classroom payment is not configured")
		return
	}
	target, err := parseClassroomOrderTarget(r.URL.Query().Get("targetType"), r.URL.Query().Get("refId"))
	if err != nil {
		httpx.Fail(w, http.StatusBadRequest, err.Error())
		return
	}
	result, err := s.classroomOrders.Status(r.Context(), userFromRequest(r).ID, target)
	if err != nil {
		writeClassroomOrderError(w, err)
		return
	}
	httpx.OK(w, result)
}

func (s *Server) classroomOrderDevPay(w http.ResponseWriter, r *http.Request) {
	if config.IsProduction(s.env.AppEnv) || s.pay == nil || !s.pay.DevMode() || s.classroomOrders == nil {
		httpx.Fail(w, http.StatusNotFound, "Not Found")
		return
	}
	var body struct {
		OutTradeNo    string `json:"outTradeNo"`
		TransactionID string `json:"transactionId"`
	}
	if err := json.NewDecoder(http.MaxBytesReader(w, r.Body, 8*1024)).Decode(&body); err != nil || strings.TrimSpace(body.OutTradeNo) == "" {
		httpx.Fail(w, http.StatusBadRequest, "outTradeNo is required")
		return
	}
	paid, err := s.classroomOrders.DevPay(r.Context(), userFromRequest(r).ID, body.OutTradeNo, body.TransactionID)
	if err != nil {
		writeClassroomOrderError(w, err)
		return
	}
	httpx.OK(w, map[string]any{"paid": paid})
}

func writeClassroomOrderError(w http.ResponseWriter, err error) {
	switch {
	case errors.Is(err, classroom.ErrNotFound), errors.Is(err, sql.ErrNoRows):
		httpx.Fail(w, http.StatusNotFound, "classroom target not found")
	case errors.Is(err, ErrClassroomTargetNotForSale), errors.Is(err, ErrClassroomAlreadyOwned):
		httpx.Fail(w, http.StatusConflict, err.Error())
	default:
		httpx.Fail(w, http.StatusInternalServerError, "classroom order failed")
	}
}

func validateClassroomPaymentParams(env config.Env, params wxpay.PrepayResult) error {
	if config.IsProduction(env.AppEnv) && params.Dev {
		return errors.New("development payment parameters are forbidden in production")
	}
	if strings.TrimSpace(params.Package) == "" || strings.TrimSpace(params.SignType) != "RSA" || strings.TrimSpace(params.PaySign) == "" {
		return errors.New("incomplete payment parameters")
	}
	if config.IsProduction(env.AppEnv) && (strings.TrimSpace(params.TimeStamp) == "" || strings.TrimSpace(params.NonceStr) == "") {
		return errors.New("incomplete production payment signature")
	}
	return nil
}

type classroomOrderDB struct {
	db        *sql.DB
	classroom *classroom.Store
	orders    *miniapp.Store
	pay       *wxpay.Client
	env       config.Env
}

func newClassroomOrderDB(db *sql.DB, orders *miniapp.Store, pay *wxpay.Client, env config.Env) classroomOrderService {
	if db == nil || orders == nil || pay == nil {
		return nil
	}
	return &classroomOrderDB{db: db, classroom: classroom.NewStore(db), orders: orders, pay: pay, env: env}
}

func classroomOrderProduct(target classroomOrderTarget) string {
	if target.Type == "series" {
		return miniapp.ProductClassroomSeries
	}
	return miniapp.ProductClassroomContent
}

func (d *classroomOrderDB) saleSnapshot(ctx context.Context, target classroomOrderTarget) (string, int, error) {
	switch target.Type {
	case "series":
		item, err := d.classroom.GetSeries(ctx, target.RefID)
		if err != nil {
			return "", 0, err
		}
		return classroomSeriesSaleSnapshot(item)
	case "content":
		item, err := d.classroom.GetContent(ctx, target.RefID)
		if err != nil {
			return "", 0, err
		}
		title, price, err := classroomContentSaleSnapshot(item)
		if err != nil {
			return "", 0, ErrClassroomTargetNotForSale
		}
		if item.SeriesID != nil {
			parent, err := d.classroom.GetSeries(ctx, *item.SeriesID)
			if err != nil || parent.PlaybackBlocked || (!item.ShowAsStandalone && parent.Status != classroom.SeriesPublished) {
				return "", 0, ErrClassroomTargetNotForSale
			}
		}
		return title, price, nil
	default:
		return "", 0, ErrClassroomTargetNotForSale
	}
}

func classroomSeriesSaleSnapshot(item classroom.Series) (string, int, error) {
	if item.Status != classroom.SeriesPublished || item.PlaybackBlocked || item.AccessLevel != classroom.AccessPaid || item.PriceCents <= 0 {
		return "", 0, ErrClassroomTargetNotForSale
	}
	return item.Title, item.PriceCents, nil
}

func classroomContentSaleSnapshot(item classroom.Content) (string, int, error) {
	if item.Status != classroom.ContentPublished || item.PlaybackBlocked || item.AccessLevel != classroom.AccessPaid || item.PriceCents <= 0 {
		return "", 0, ErrClassroomTargetNotForSale
	}
	return item.Title, item.PriceCents, nil
}

func (d *classroomOrderDB) owned(ctx context.Context, uid int64, target classroomOrderTarget) (bool, error) {
	var owned bool
	err := d.db.QueryRowContext(ctx, classroomOwnershipQuery(target), uid, target.RefID).Scan(&owned)
	return owned, err
}

func classroomOwnershipQuery(target classroomOrderTarget) string {
	targetClause := "e.series_id=$2"
	if target.Type == "content" {
		targetClause = "(e.content_id=$2 OR e.series_id=(SELECT series_id FROM classroom_contents WHERE id=$2))"
	}
	return fmt.Sprintf(`SELECT EXISTS(SELECT 1 FROM classroom_entitlements e WHERE e.wx_user_id=$1 AND %s AND e.revoked_at IS NULL AND (e.expires_at IS NULL OR e.expires_at>now()))`, targetClause)
}

func (d *classroomOrderDB) Create(ctx context.Context, uid int64, target classroomOrderTarget) (classroomOrderCheckout, error) {
	if d.pay == nil {
		return classroomOrderCheckout{}, errors.New("payment service is not configured")
	}
	title, amount, err := d.saleSnapshot(ctx, target)
	if err != nil {
		return classroomOrderCheckout{}, err
	}
	owned, err := d.owned(ctx, uid, target)
	if err != nil {
		return classroomOrderCheckout{}, err
	}
	if owned {
		return classroomOrderCheckout{}, ErrClassroomAlreadyOwned
	}
	openid, err := d.orders.OpenIDByUserID(ctx, uid)
	if err != nil {
		return classroomOrderCheckout{}, err
	}
	product := classroomOrderProduct(target)
	outTradeNo, err := generateClassroomOutTradeNo(uid, target)
	if err != nil {
		return classroomOrderCheckout{}, err
	}
	order, err := d.orders.CreateOrReusePendingOrder(ctx, uid, outTradeNo, product, target.RefID, title, amount)
	if err != nil {
		return classroomOrderCheckout{}, err
	}
	prepay, err := d.pay.Prepay(ctx, order.OutTradeNo, openid, classroomPaymentDescription(order.Title), order.Amount)
	if err != nil {
		return classroomOrderCheckout{}, err
	}
	return classroomOrderCheckout{OutTradeNo: order.OutTradeNo, Product: order.Product, RefID: order.RefID, Title: order.Title, Amount: order.Amount, PayParams: prepay}, nil
}

func classroomPaymentDescription(title string) string {
	const prefix = "老师课堂·"
	var b strings.Builder
	b.WriteString(prefix)
	for _, r := range strings.TrimSpace(title) {
		if b.Len()+len(string(r)) > 127 {
			break
		}
		b.WriteRune(r)
	}
	return b.String()
}

func generateClassroomOutTradeNo(_ int64, target classroomOrderTarget) (string, error) {
	suffix, err := randomHex(6)
	if err != nil {
		return "", err
	}
	prefix := "clc"
	if target.Type == "series" {
		prefix = "cls"
	}
	return prefix + strconv.FormatInt(time.Now().UnixNano(), 36) + suffix, nil
}

func randomHex(size int) (string, error) {
	buf := make([]byte, size)
	if _, err := rand.Read(buf); err != nil {
		return "", err
	}
	return hex.EncodeToString(buf), nil
}

func (d *classroomOrderDB) Status(ctx context.Context, uid int64, target classroomOrderTarget) (classroomOrderStatus, error) {
	product := classroomOrderProduct(target)
	owned, err := d.owned(ctx, uid, target)
	if err != nil {
		return classroomOrderStatus{}, err
	}
	order, err := d.orders.LatestOrderForTarget(ctx, uid, product, target.RefID)
	if errors.Is(err, sql.ErrNoRows) {
		return classroomOrderStatus{Product: product, RefID: strconv.FormatInt(target.RefID, 10), Status: "none", Owned: owned}, nil
	}
	if err != nil {
		return classroomOrderStatus{}, err
	}
	return classroomOrderStatus{OutTradeNo: order.OutTradeNo, Product: order.Product, RefID: order.RefID, Title: order.Title, Amount: order.Amount, Status: order.Status, Owned: owned}, nil
}

func (d *classroomOrderDB) DevPay(ctx context.Context, uid int64, outTradeNo, transactionID string) (bool, error) {
	if config.IsProduction(d.env.AppEnv) || d.pay == nil || !d.pay.DevMode() {
		return false, errors.New("development payment is disabled")
	}
	_, ownerID, _, product, status, err := d.orders.OrderByOutTradeNo(ctx, outTradeNo)
	if err != nil {
		return false, err
	}
	if ownerID != uid {
		return false, errors.New("order owner mismatch")
	}
	if product != miniapp.ProductClassroomSeries && product != miniapp.ProductClassroomContent {
		return false, errors.New("not a classroom order")
	}
	if status == "paid" {
		return true, nil
	}
	if strings.TrimSpace(transactionID) == "" {
		transactionID = "dev-" + outTradeNo
	}
	_, err = d.orders.MarkOrderPaid(ctx, outTradeNo, transactionID)
	return err == nil, err
}
