package server

import (
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"testing"

	"nine-xing/nx-backend/apps/server/internal/config"
	"nine-xing/nx-backend/apps/server/internal/siteconfig"
)

func TestRequireMiniappPaymentUsesPersistedSiteSwitch(t *testing.T) {
	path := writeMiniappPaymentSiteConfig(t, false)
	s := &Server{env: config.Env{SiteConfig: path}}

	w := httptest.NewRecorder()
	if s.requireMiniappPayment(w, httptest.NewRequest(http.MethodPost, "/api/miniapp/report/order", nil)) {
		t.Fatal("expected disabled site switch to reject payment")
	}
	if w.Code != http.StatusServiceUnavailable || !strings.Contains(w.Body.String(), miniappPaymentDisabledMessage) {
		t.Fatalf("unexpected disabled payment response: status=%d body=%s", w.Code, w.Body.String())
	}

	path = writeMiniappPaymentSiteConfig(t, true)
	s.env.SiteConfig = path
	w = httptest.NewRecorder()
	if !s.requireMiniappPayment(w, httptest.NewRequest(http.MethodPost, "/api/miniapp/report/order", nil)) {
		t.Fatalf("expected enabled site switch to allow payment: status=%d body=%s", w.Code, w.Body.String())
	}
	if w.Code != http.StatusOK {
		t.Fatalf("allow response should remain untouched, got status=%d body=%s", w.Code, w.Body.String())
	}
}

func TestMiniappPaymentDisabledRejectsAllNewPaymentEntrypoints(t *testing.T) {
	path := writeMiniappPaymentSiteConfig(t, false)
	devPay := mustWxPayClient(config.Env{WxPay: config.WxPayConfig{Dev: true}})
	classroom := &fakeClassroomOrderService{}
	s := &Server{
		env:             config.Env{AppEnv: "development", SiteConfig: path},
		pay:             devPay,
		classroomOrders: classroom,
	}

	tests := []struct {
		name string
		fn   http.HandlerFunc
		body string
	}{
		{name: "report order", fn: s.createReportOrder, body: `{"testRecordId":"1"}`},
		{name: "wechat test order", fn: s.createWechatPayTestOrder},
		{name: "report dev pay", fn: s.devPayReportOrder, body: `{"outTradeNo":"rpt-1"}`},
		{name: "classroom order", fn: s.classroomOrderCreate, body: `{"targetType":"series","refId":"1"}`},
		{name: "classroom dev pay", fn: s.classroomOrderDevPay, body: `{"outTradeNo":"cls-1"}`},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			w := httptest.NewRecorder()
			r := httptest.NewRequest(http.MethodPost, "/api/miniapp/payment", strings.NewReader(tt.body))
			tt.fn(w, r)
			if w.Code != http.StatusServiceUnavailable || !strings.Contains(w.Body.String(), miniappPaymentDisabledMessage) {
				t.Fatalf("expected payment-disabled 503, got status=%d body=%s", w.Code, w.Body.String())
			}
		})
	}
}

func TestMiniappPaymentDisabledDoesNotBlockClassroomStatus(t *testing.T) {
	path := writeMiniappPaymentSiteConfig(t, false)
	classroom := &fakeClassroomOrderService{
		status: classroomOrderStatus{Product: "classroom_series", RefID: "1", Status: "none"},
	}
	s := &Server{env: config.Env{SiteConfig: path}, classroomOrders: classroom}
	w := httptest.NewRecorder()
	r := httptest.NewRequest(http.MethodGet, "/api/miniapp/classroom/orders/status?targetType=series&refId=1", nil)
	s.classroomOrderStatus(w, r)
	if w.Code != http.StatusOK || !strings.Contains(w.Body.String(), `"status":"none"`) {
		t.Fatalf("expected status query to remain available while payment is disabled: status=%d body=%s", w.Code, w.Body.String())
	}
}

func writeMiniappPaymentSiteConfig(t *testing.T, enabled bool) string {
	t.Helper()
	config := siteconfig.SiteConfig{}
	config.Site.BrandName = "九型芯之力"
	config.Site.Logo = "/assets/logo.svg"
	config.Navigation.Main = []siteconfig.NavItem{{Label: "首页", To: "/", Type: "route"}}
	config.Navigation.Drawer = []siteconfig.NavItem{{Label: "首页", To: "/", Type: "route"}}
	config.Navigation.Tabs = []siteconfig.TabItem{{NavItem: siteconfig.NavItem{Label: "首页", To: "/", Type: "route"}, Icon: "home", Match: "/"}}
	config.Home = map[string]any{"miniappPayment": map[string]any{"enabled": enabled}}
	config.Types = []struct {
		Avatar      string `json:"avatar"`
		Description string `json:"description"`
		ID          string `json:"id"`
		Keywords    string `json:"keywords"`
		Name        string `json:"name"`
	}{{ID: "1", Name: "完美型", Avatar: "/assets/avatars/1.webp"}}

	path := filepath.Join(t.TempDir(), "site-config.json")
	if err := siteconfig.Write(path, config); err != nil {
		t.Fatal(err)
	}
	return path
}
