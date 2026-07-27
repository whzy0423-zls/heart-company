package server

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"nine-xing/nx-backend/apps/server/internal/auth"
	"nine-xing/nx-backend/apps/server/internal/classroom"
	"nine-xing/nx-backend/apps/server/internal/config"
	"nine-xing/nx-backend/apps/server/internal/wxpay"
)

type fakeClassroomOrderService struct {
	created classroomOrderCheckout
	status  classroomOrderStatus
	err     error
	uid     int64
	target  classroomOrderTarget
}

func (f *fakeClassroomOrderService) Create(_ context.Context, uid int64, target classroomOrderTarget) (classroomOrderCheckout, error) {
	f.uid, f.target = uid, target
	return f.created, f.err
}

func (f *fakeClassroomOrderService) Status(_ context.Context, uid int64, target classroomOrderTarget) (classroomOrderStatus, error) {
	f.uid, f.target = uid, target
	return f.status, f.err
}

func (f *fakeClassroomOrderService) DevPay(_ context.Context, uid int64, outTradeNo, transactionID string) (bool, error) {
	f.uid = uid
	return outTradeNo != "", f.err
}

func TestClassroomOrderRoutesCreateSeriesAndReadContentStatus(t *testing.T) {
	svc := &fakeClassroomOrderService{
		created: classroomOrderCheckout{OutTradeNo: "cls-7", Product: "classroom_series", RefID: "41", Title: "基础系列", Amount: 2990, PayParams: wxpay.PrepayResult{Dev: true, Package: "prepay_id=dev", SignType: "RSA", PaySign: "dev-signature"}},
		status:  classroomOrderStatus{Product: "classroom_content", RefID: "52", Title: "第一课", Amount: 990, Status: "paid", Owned: true},
	}
	s := &Server{env: config.Env{AppEnv: "development"}, classroomOrders: svc}
	mux := http.NewServeMux()
	authn := func(next http.HandlerFunc) http.HandlerFunc {
		return func(w http.ResponseWriter, r *http.Request) {
			next(w, r.WithContext(withUser(r.Context(), auth.UserInfo{ID: 7, TokenKind: auth.TokenKindMiniapp})))
		}
	}
	registerClassroomOrderRoutes(mux, authn, s)

	create := httptest.NewRecorder()
	mux.ServeHTTP(create, httptest.NewRequest(http.MethodPost, "/api/miniapp/classroom/orders", strings.NewReader(`{"targetType":"series","refId":"41"}`)))
	if create.Code != http.StatusOK || !strings.Contains(create.Body.String(), `"amount":2990`) || svc.target.Type != "series" || svc.target.RefID != 41 {
		t.Fatalf("unexpected create response=%d %s target=%+v", create.Code, create.Body.String(), svc.target)
	}

	status := httptest.NewRecorder()
	mux.ServeHTTP(status, httptest.NewRequest(http.MethodGet, "/api/miniapp/classroom/orders/status?targetType=content&refId=52", nil))
	if status.Code != http.StatusOK || !strings.Contains(status.Body.String(), `"owned":true`) || svc.target.Type != "content" || svc.target.RefID != 52 {
		t.Fatalf("unexpected status response=%d %s target=%+v", status.Code, status.Body.String(), svc.target)
	}
}

func TestClassroomOrderRoutesRejectInvalidTargetsAndMapSaleErrors(t *testing.T) {
	svc := &fakeClassroomOrderService{err: ErrClassroomTargetNotForSale}
	s := &Server{classroomOrders: svc}
	mux := http.NewServeMux()
	registerClassroomOrderRoutes(mux, func(next http.HandlerFunc) http.HandlerFunc { return next }, s)

	bad := httptest.NewRecorder()
	mux.ServeHTTP(bad, httptest.NewRequest(http.MethodPost, "/api/miniapp/classroom/orders", strings.NewReader(`{"targetType":"album","refId":"1"}`)))
	if bad.Code != http.StatusBadRequest {
		t.Fatalf("invalid target status=%d body=%s", bad.Code, bad.Body.String())
	}

	notSale := httptest.NewRecorder()
	mux.ServeHTTP(notSale, httptest.NewRequest(http.MethodPost, "/api/miniapp/classroom/orders", strings.NewReader(`{"targetType":"series","refId":"1"}`)))
	if notSale.Code != http.StatusConflict {
		t.Fatalf("not-for-sale status=%d body=%s", notSale.Code, notSale.Body.String())
	}
}

func TestClassroomPaymentParamsBoundary(t *testing.T) {
	dev := wxpay.PrepayResult{Dev: true, Package: "prepay_id=dev_x", SignType: "RSA", PaySign: "dev-signature"}
	if err := validateClassroomPaymentParams(config.Env{AppEnv: "development"}, dev); err != nil {
		t.Fatalf("development should accept explicit dev params: %v", err)
	}
	if err := validateClassroomPaymentParams(config.Env{AppEnv: "production"}, dev); err == nil {
		t.Fatal("production must reject dev payment params")
	}
	prod := wxpay.PrepayResult{TimeStamp: "1", NonceStr: "n", Package: "prepay_id=wx", SignType: "RSA", PaySign: "signed"}
	if err := validateClassroomPaymentParams(config.Env{AppEnv: "production"}, prod); err != nil {
		t.Fatalf("production signed params rejected: %v", err)
	}
}

func TestClassroomOrderServiceStaysDisabledWithoutPaymentClient(t *testing.T) {
	if service := newClassroomOrderDB(nil, nil, nil, config.Env{AppEnv: "production"}); service != nil {
		t.Fatal("incomplete production payment configuration must leave classroom checkout disabled")
	}
}

func TestPaymentCallbackAcceptsClassroomProductsWithExactSnapshotAmount(t *testing.T) {
	env := config.Env{WxPay: config.WxPayConfig{MchID: "merchant", AppID: "miniapp"}}
	for _, product := range []string{"classroom_series", "classroom_content"} {
		err := validateWxPayCallbackAgainstOrder(env, wxpay.CallbackResult{
			OutTradeNo: "classroom-order", MchID: "merchant", AppID: "miniapp", AmountTotal: 2990,
		}, paymentOrderSnapshot{Amount: 2990, Product: product})
		if err != nil {
			t.Fatalf("%s callback rejected: %v", product, err)
		}
	}
	if err := validateWxPayCallbackAgainstOrder(env, wxpay.CallbackResult{
		OutTradeNo: "classroom-order", MchID: "merchant", AppID: "miniapp", AmountTotal: 1,
	}, paymentOrderSnapshot{Amount: 2990, Product: "classroom_series"}); err == nil {
		t.Fatal("callback amount must match the persisted order snapshot")
	}
}

func TestClassroomSaleSnapshotRequiresPublishedExplicitlyPaidTarget(t *testing.T) {
	series := classroom.Series{Title: "基础系列", Status: classroom.SeriesPublished, AccessLevel: classroom.AccessPaid, PriceCents: 2990}
	title, price, err := classroomSeriesSaleSnapshot(series)
	if err != nil || title != "基础系列" || price != 2990 {
		t.Fatalf("valid series snapshot rejected: title=%q price=%d err=%v", title, price, err)
	}
	series.Status = classroom.SeriesOffline
	if _, _, err := classroomSeriesSaleSnapshot(series); !errors.Is(err, ErrClassroomTargetNotForSale) {
		t.Fatalf("offline series must not be sold: %v", err)
	}
	series.Status, series.PlaybackBlocked = classroom.SeriesPublished, true
	if _, _, err := classroomSeriesSaleSnapshot(series); !errors.Is(err, ErrClassroomTargetNotForSale) {
		t.Fatalf("emergency-blocked series must not accept new purchases: %v", err)
	}

	content := classroom.Content{Title: "第一课", Status: classroom.ContentPublished, AccessLevel: classroom.AccessPaid, PriceCents: 990}
	title, price, err = classroomContentSaleSnapshot(content)
	if err != nil || title != "第一课" || price != 990 {
		t.Fatalf("valid content snapshot rejected: title=%q price=%d err=%v", title, price, err)
	}
	content.AccessLevel = classroom.AccessInherit
	if _, _, err := classroomContentSaleSnapshot(content); !errors.Is(err, ErrClassroomTargetNotForSale) {
		t.Fatalf("inherited lesson must be purchased through its series: %v", err)
	}
	content.AccessLevel, content.PlaybackBlocked = classroom.AccessPaid, true
	if _, _, err := classroomContentSaleSnapshot(content); !errors.Is(err, ErrClassroomTargetNotForSale) {
		t.Fatalf("emergency-blocked lesson must not accept new purchases: %v", err)
	}
}

func TestClassroomContentOwnershipIncludesItsCurrentSeriesGrant(t *testing.T) {
	query := classroomOwnershipQuery(classroomOrderTarget{Type: "content", RefID: 52})
	if !strings.Contains(query, "e.content_id=$2") || !strings.Contains(query, "e.series_id=(SELECT series_id FROM classroom_contents") {
		t.Fatalf("content ownership must include direct and current-series grants: %s", query)
	}
	seriesQuery := classroomOwnershipQuery(classroomOrderTarget{Type: "series", RefID: 41})
	if !strings.Contains(seriesQuery, "e.series_id=$2") || strings.Contains(seriesQuery, "classroom_contents") {
		t.Fatalf("series ownership query must stay target-specific: %s", seriesQuery)
	}
}

func TestClassroomOutTradeNoFitsWeChatBoundary(t *testing.T) {
	tradeNo, err := generateClassroomOutTradeNo(9223372036854775807, classroomOrderTarget{Type: "series", RefID: 9223372036854775807})
	if err != nil {
		t.Fatal(err)
	}
	if len(tradeNo) > 32 || !strings.HasPrefix(tradeNo, "cls") {
		t.Fatalf("invalid WeChat out_trade_no %q length=%d", tradeNo, len(tradeNo))
	}
}

func TestClassroomPaymentDescriptionFitsWeChatBoundary(t *testing.T) {
	description := classroomPaymentDescription(strings.Repeat("课", 100))
	if len([]byte(description)) > 127 || !strings.HasPrefix(description, "老师课堂·") {
		t.Fatalf("invalid payment description bytes=%d value=%q", len([]byte(description)), description)
	}
}

func TestClassroomOrderErrorResponseDoesNotLeakInternalDetails(t *testing.T) {
	svc := &fakeClassroomOrderService{err: errors.New("SELECT secret FROM orders")}
	s := &Server{classroomOrders: svc}
	w := httptest.NewRecorder()
	s.classroomOrderCreate(w, httptest.NewRequest(http.MethodPost, "/api/miniapp/classroom/orders", strings.NewReader(`{"targetType":"series","refId":"1"}`)))
	var response map[string]any
	_ = json.Unmarshal(w.Body.Bytes(), &response)
	if w.Code != http.StatusInternalServerError || strings.Contains(w.Body.String(), "SELECT secret") {
		t.Fatalf("internal error leaked: %d %s", w.Code, w.Body.String())
	}
}
