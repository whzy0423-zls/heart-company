package server

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"strings"
	"testing"
	"time"

	"golang.org/x/crypto/bcrypt"
	"nine-xing/nx-backend/apps/server/internal/appuser"
	"nine-xing/nx-backend/apps/server/internal/config"
	"nine-xing/nx-backend/apps/server/internal/system"
	"nine-xing/nx-backend/apps/server/internal/teacher"
	"nine-xing/nx-backend/apps/server/internal/testdb"
	"nine-xing/nx-backend/apps/server/internal/xznpay"
)

// This journey crosses the production route table, bearer authentication and
// real PostgreSQL transactions. Only SMS delivery and the payment provider are
// local fixtures; it never sends an SMS, charges a card or performs a payout.
func TestDistributionHTTPPaymentJourneyPostgres(t *testing.T) {
	database, _ := testdb.OpenEnvIsolatedSchema(t, "distribution_http")
	schema, err := os.ReadFile("../db/schema.sql")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := database.Exec(string(schema)); err != nil {
		t.Fatal(err)
	}
	const password = "LocalJourney123!"
	hash, err := bcrypt.GenerateFromPassword([]byte(password), bcrypt.MinCost)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := database.Exec(`INSERT INTO users(id,username,password_hash) VALUES(1,'distribution-test-admin',$1)`, string(hash)); err != nil {
		t.Fatal(err)
	}
	if _, err := database.Exec(`INSERT INTO roles(id,code,name) VALUES(1,'admin','Test admin'); INSERT INTO user_roles VALUES(1,1)`); err != nil {
		t.Fatal(err)
	}
	s := &Server{db: database, mux: http.NewServeMux(), env: config.Env{AppEnv: "test", JWTSecret: "distribution-http-local-test-only"},
		appUsers: appuser.NewStore(database), system: system.NewStore(database), teachers: teacher.NewStore(database)}
	s.routes()
	api := httptest.NewServer(s)
	t.Cleanup(api.Close)
	client := api.Client()
	client.Timeout = 15 * time.Second
	request := func(t *testing.T, method, path, token string, payload any, status int) json.RawMessage {
		t.Helper()
		body, err := json.Marshal(payload)
		if err != nil {
			t.Fatal(err)
		}
		req, err := http.NewRequest(method, api.URL+path, bytes.NewReader(body))
		if err != nil {
			t.Fatal(err)
		}
		req.Header.Set("Content-Type", "application/json")
		if token != "" {
			req.Header.Set("Authorization", "Bearer "+token)
		}
		res, err := client.Do(req)
		if err != nil {
			t.Fatal(err)
		}
		defer res.Body.Close()
		raw, err := io.ReadAll(res.Body)
		if err != nil {
			t.Fatal(err)
		}
		if res.StatusCode != status {
			t.Fatalf("%s %s: status=%d want=%d body=%s", method, path, res.StatusCode, status, raw)
		}
		var envelope struct {
			Data json.RawMessage `json:"data"`
		}
		if err := json.Unmarshal(raw, &envelope); err != nil {
			t.Fatalf("%s: %v: %s", path, err, raw)
		}
		return envelope.Data
	}
	step := func(name string, fn func(*testing.T)) {
		t.Helper()
		if !t.Run(name, fn) {
			t.FailNow()
		}
	}
	decode := func(t *testing.T, raw json.RawMessage, out any) {
		t.Helper()
		if err := json.Unmarshal(raw, out); err != nil {
			t.Fatalf("decode: %v: %s", err, raw)
		}
	}
	login := func(t *testing.T, path, field, account string) string {
		t.Helper()
		var out struct {
			AccessToken string `json:"accessToken"`
		}
		decode(t, request(t, "POST", path, "", map[string]any{field: account, "password": password}, 200), &out)
		if out.AccessToken == "" {
			t.Fatal("missing login token")
		}
		return out.AccessToken
	}
	admin := login(t, "/api/auth/login", "username", "distribution-test-admin")
	type session struct {
		AccessToken string `json:"accessToken"`
		User        struct {
			ID int64 `json:"id"`
		} `json:"user"`
	}
	register := func(t *testing.T, phone, code string) session {
		t.Helper()
		if _, err := database.Exec(`INSERT INTO app_sms_codes(phone,code_hash,expires_at) VALUES($1,$2,now()+interval '5 minutes')`, phone, appuser.HashToken("654321")); err != nil {
			t.Fatal(err)
		}
		var out session
		decode(t, request(t, "POST", "/api/app/auth/register", "", map[string]any{"phone": phone, "code": "654321", "password": password, "nickname": "接口测试用户", "agentCode": code}, 200), &out)
		if out.User.ID == 0 || out.AccessToken == "" {
			t.Fatal("registration did not return identity")
		}
		return out
	}
	var agents [3]distributionAgentResponse
	var sessions [3]session
	var buyer, outsider session
	step("create_three_tiers_click_register_and_admin_attribution", func(t *testing.T) {
		sessions[0] = register(t, "19900006101", "")
		decode(t, request(t, "POST", "/api/admin/distribution/agents", admin, map[string]any{"appUserId": sessions[0].User.ID}, 200), &agents[0])
		for i := 1; i < 3; i++ {
			request(t, "GET", "/api/public/distribution/invite?agent="+url.QueryEscape(agents[i-1].AgentCode), "", nil, 200)
			sessions[i] = register(t, fmt.Sprintf("1990000610%d", i+1), agents[i-1].AgentCode)
			decode(t, request(t, "POST", "/api/app/distribution/children", sessions[i-1].AccessToken, map[string]any{"appUserId": sessions[i].User.ID}, 200), &agents[i])
			if agents[i].Level != i+1 || agents[i].ParentAgentID != agents[i-1].ID || agents[i].RootAgentID != agents[0].ID {
				t.Fatalf("incorrect hierarchy: %+v", agents)
			}
		}
		request(t, "GET", "/api/public/distribution/invite?agent="+agents[2].AgentCode, "", nil, 200)
		buyer = register(t, "19900006104", agents[2].AgentCode)
		outsider = register(t, "19900006105", "")
		var users struct {
			Items []struct{ ID, DirectAgentID int64 }
			Total int
		}
		decode(t, request(t, "GET", "/api/admin/distribution/users", admin, nil, 200), &users)
		found := false
		for _, u := range users.Items {
			if u.ID == buyer.User.ID {
				found = u.DirectAgentID == agents[2].ID
			}
		}
		if !found || users.Total != 3 {
			t.Fatalf("admin attribution mismatch: %+v", users)
		}
		var clicks, registrations int
		if err := database.QueryRow(`SELECT count(*) FILTER(WHERE event_type='click'),count(*) FILTER(WHERE event_type='register') FROM distribution_invite_events`).Scan(&clicks, &registrations); err != nil {
			t.Fatal(err)
		}
		if clicks != 3 || registrations != 3 {
			t.Fatalf("invite events click=%d register=%d", clicks, registrations)
		}
	})
	const secret = "distribution-local-gateway-secret"
	gateway := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/openapi/pay/create" || r.Method != "POST" {
			http.Error(w, "unexpected gateway call", 400)
			return
		}
		if err := r.ParseForm(); err != nil || !xznpay.VerifyMD5(r.Form, secret, r.Form.Get("sign")) {
			http.Error(w, "invalid gateway signature", 401)
			return
		}
		_ = json.NewEncoder(w).Encode(map[string]any{"code": 1, "data": map[string]any{"trade_no": "fixture-" + r.Form.Get("out_trade_no"), "out_trade_no": r.Form.Get("out_trade_no"), "pay_url": "https://cashier.example.test/local-only"}})
	}))
	t.Cleanup(gateway.Close)
	var rule struct{ ID, Version int64 }
	step("configure_rules_discount_and_isolated_web_gateway", func(t *testing.T) {
		decode(t, request(t, "POST", "/api/admin/distribution/rules", admin, map[string]any{"name": "HTTP journey ten percent each", "rates": map[string]int{"1": 1000, "2": 1000, "3": 1000}}, 200), &rule)
		request(t, "POST", fmt.Sprintf("/api/admin/distribution/rules/%d/activate", rule.ID), admin, map[string]any{}, 200)
		request(t, "PUT", "/api/admin/app-agent-discounts", admin, map[string]any{"rules": []appAgentDiscountRule{{ProductID: "vip_month", Audience: "invited_user", Mode: "amount_off", Value: 500, Enabled: true}}}, 200)
		request(t, "PUT", "/api/admin/xzn-pay/config", admin, xznPaymentConfig{PID: "fixture-merchant", Secret: secret, BaseURL: gateway.URL + "/openapi", SignType: "MD5", NotifyURL: "https://api.example.test/api/xzn-pay/notify", WebReturnURL: "https://web.example.test/#/billing", Enabled: true, AlipayEnabled: true, AlipayGatewayID: "34"}, 200)
	})
	var order appOrderResp
	commissions := func(t *testing.T) []struct {
		ID, OrderID, AgentID, AppUserID, OrderAmount, RateBPS, Amount, RuleVersion int64
		Status                                                                     string
	} {
		t.Helper()
		var out struct {
			Items []struct {
				ID, OrderID, AgentID, AppUserID, OrderAmount, RateBPS, Amount, RuleVersion int64
				Status                                                                     string
			}
		}
		decode(t, request(t, "GET", "/api/admin/distribution/commissions", admin, nil, 200), &out)
		return out.Items
	}
	callback := func(t *testing.T, ord appOrderResp, amount, state string, valid bool, wantStatus int) {
		t.Helper()
		form := url.Values{"pid": {"fixture-merchant"}, "trade_no": {"fixture-" + ord.OutTradeNo}, "out_trade_no": {ord.OutTradeNo}, "total_amount": {amount}, "subject": {ord.Title}, "paytype_code": {"alipay"}, "channel_id": {"34"}, "trade_status": {state}, "sign_type": {"MD5"}}
		signWebBillingCallback(form, secret)
		if !valid {
			form.Set("sign", "invalid")
		}
		res, err := client.PostForm(api.URL+"/api/xzn-pay/notify", form)
		if err != nil {
			t.Fatal(err)
		}
		defer res.Body.Close()
		raw, err := io.ReadAll(res.Body)
		if err != nil {
			t.Fatal(err)
		}
		if res.StatusCode != wantStatus || (wantStatus == 200 && string(raw) != "success") {
			t.Fatalf("callback %s: %d %s", state, res.StatusCode, raw)
		}
	}
	step("discounted_web_order_pending_bad_callbacks_do_not_pay", func(t *testing.T) {
		decode(t, request(t, "POST", "/api/app/web/billing/orders", buyer.AccessToken, map[string]any{"productId": "vip_month", "payChannel": "alipay", "expectedAmountCents": 2400, "webReturnUrl": "https://web.example.test/#/billing"}, 200), &order)
		if order.Amount != 2400 || order.BasePriceCents != 2900 || order.DiscountCents != 500 {
			t.Fatalf("price mismatch: %+v", order)
		}
		callback(t, order, "24.00", "TRADE_SUCCESS", false, 401)
		callback(t, order, "29.00", "TRADE_SUCCESS", true, 400)
		callback(t, order, "24.00", "WAIT_BUYER_PAY", true, 200)
		if rows := commissions(t); len(rows) != 0 {
			t.Fatalf("unpaid commission: %+v", rows)
		}
	})
	var paidExpiry time.Time
	step("successful_payment_replay_and_admin_reconciliation", func(t *testing.T) {
		callback(t, order, "24.00", "TRADE_SUCCESS", true, 200)
		if err := database.QueryRow(`SELECT member_expires_at FROM app_users WHERE id=$1`, buyer.User.ID).Scan(&paidExpiry); err != nil {
			t.Fatal(err)
		}
		callback(t, order, "24.00", "TRADE_SUCCESS", true, 200)
		var expiry time.Time
		if err := database.QueryRow(`SELECT member_expires_at FROM app_users WHERE id=$1`, buyer.User.ID).Scan(&expiry); err != nil {
			t.Fatal(err)
		}
		if !expiry.Equal(paidExpiry) {
			t.Fatal("duplicate callback extended membership")
		}
		rows := commissions(t)
		if len(rows) != 3 {
			t.Fatalf("commission count=%d want=3", len(rows))
		}
		seen := map[int64]bool{}
		for _, row := range rows {
			seen[row.AgentID] = true
			if row.AppUserID != buyer.User.ID || row.OrderAmount != 2400 || row.RateBPS != 1000 || row.Amount != 240 || row.RuleVersion != rule.Version || row.Status != "pending" {
				t.Fatalf("commission mismatch: %+v", row)
			}
		}
		for _, a := range agents {
			if !seen[a.ID] {
				t.Fatalf("missing agent %d", a.ID)
			}
		}
		var summary struct{ Summary distributionAnalyticsSummary }
		decode(t, request(t, "GET", "/api/admin/distribution/analytics", admin, nil, 200), &summary)
		if summary.Summary.TotalOrderAmount != 2400 || summary.Summary.TotalCommissionAmount != 720 || summary.Summary.PendingCommissionAmount != 720 {
			t.Fatalf("admin totals: %+v", summary)
		}
		var orders struct {
			Items []struct {
				ID, AppUserID, Amount int64
				OutTradeNo, Status    string
			}
		}
		decode(t, request(t, "GET", "/api/admin/distribution/orders", admin, nil, 200), &orders)
		if len(orders.Items) != 1 || orders.Items[0].Amount != 2400 || orders.Items[0].OutTradeNo != order.OutTradeNo || orders.Items[0].Status != "paid" {
			t.Fatalf("admin order mismatch: %+v", orders)
		}
		request(t, "GET", "/api/app-orders/list", admin, nil, 200)
		for _, ss := range sessions {
			var overview struct{ CommissionAmount, PendingCommission int64 }
			decode(t, request(t, "GET", "/api/app/distribution/overview", ss.AccessToken, nil, 200), &overview)
			if overview.CommissionAmount != 240 || overview.PendingCommission != 240 {
				t.Fatalf("own agent commission: %+v", overview)
			}
		}
	})
	step("auth_boundary_and_order_ownership", func(t *testing.T) {
		request(t, "GET", "/api/admin/distribution/commissions", "", nil, 401)
		request(t, "GET", "/api/admin/distribution/commissions", buyer.AccessToken, nil, 401)
		agentAdmin := login(t, "/api/auth/login", "username", "19900006101")
		request(t, "GET", "/api/admin/distribution/commissions", agentAdmin, nil, 403)
		request(t, "GET", "/api/agent/distribution/analytics", agentAdmin, nil, 200)
		request(t, "GET", "/api/app/web/billing/orders/status?outTradeNo="+order.OutTradeNo, outsider.AccessToken, nil, 404)
		var outsiderRows struct{ Items []json.RawMessage }
		decode(t, request(t, "GET", "/api/app/distribution/commissions", outsider.AccessToken, nil, 200), &outsiderRows)
		if len(outsiderRows.Items) != 0 {
			t.Fatal("unrelated user saw commission records")
		}
	})
	var nextRule struct{ ID, Version int64 }
	step("new_rule_preserves_paid_order_snapshot", func(t *testing.T) {
		decode(t, request(t, "POST", "/api/admin/distribution/rules", admin, map[string]any{"name": "Future orders twenty percent each", "rates": map[string]int{"1": 2000, "2": 2000, "3": 2000}}, 200), &nextRule)
		request(t, "POST", fmt.Sprintf("/api/admin/distribution/rules/%d/activate", nextRule.ID), admin, map[string]any{}, 200)
		callback(t, order, "24.00", "TRADE_SUCCESS", true, 200)
		rows := commissions(t)
		if len(rows) != 3 {
			t.Fatalf("rule change duplicated commission records: %d", len(rows))
		}
		for _, row := range rows {
			if row.RuleVersion != rule.Version || row.Amount != 240 || row.RateBPS != 1000 {
				t.Fatalf("historical snapshot changed: %+v", row)
			}
		}
	})
	step("settlement_reservation_approval_payment_and_replay", func(t *testing.T) {
		period := map[string]any{"agentId": agents[0].ID, "start": time.Now().AddDate(0, 0, -1).Format("2006-01-02"), "end": time.Now().AddDate(0, 0, 1).Format("2006-01-02")}
		var preview struct{ Amount, Count int64 }
		decode(t, request(t, "POST", "/api/admin/distribution/settlements/preview", admin, period, 200), &preview)
		if preview.Amount != 240 || preview.Count != 1 {
			t.Fatalf("preview: %+v", preview)
		}
		var settlement struct{ SettlementID, Amount, Count int64 }
		decode(t, request(t, "POST", "/api/admin/distribution/settlements", admin, period, 200), &settlement)
		if settlement.Amount != 240 || settlement.Count != 1 {
			t.Fatalf("reservation: %+v", settlement)
		}
		request(t, "POST", "/api/admin/distribution/settlements", admin, period, 409)
		base := fmt.Sprintf("/api/admin/distribution/settlements/%d", settlement.SettlementID)
		request(t, "POST", base+"/paid", admin, map[string]any{"paymentReference": "local-fixture-no-real-transfer"}, 409)
		request(t, "POST", base+"/approve", admin, map[string]any{}, 200)
		request(t, "POST", base+"/paid", admin, map[string]any{}, 400)
		request(t, "POST", base+"/paid", admin, map[string]any{"paymentReference": "local-fixture-no-real-transfer"}, 200)
		request(t, "POST", base+"/paid", admin, map[string]any{"paymentReference": "local-fixture-no-real-transfer"}, 409)
		var summary struct{ Summary distributionAnalyticsSummary }
		decode(t, request(t, "GET", "/api/admin/distribution/analytics", admin, nil, 200), &summary)
		if summary.Summary.SettledCommissionAmount != 240 || summary.Summary.PendingCommissionAmount != 480 {
			t.Fatalf("settled totals: %+v", summary)
		}
		request(t, "GET", "/api/admin/distribution/settlements", admin, nil, 200)
	})
	step("refund_replay_and_late_success_are_terminal", func(t *testing.T) {
		callback(t, order, "24.00", "TRADE_REFUND", true, 200)
		callback(t, order, "24.00", "TRADE_REFUND", true, 200)
		callback(t, order, "24.00", "TRADE_SUCCESS", true, 200)
		rows := commissions(t)
		if len(rows) != 3 {
			t.Fatalf("refund lost commission history: %d", len(rows))
		}
		for _, row := range rows {
			if row.Status != "reversed" {
				t.Fatalf("refund commission: %+v", row)
			}
		}
		var summary struct{ Summary distributionAnalyticsSummary }
		decode(t, request(t, "GET", "/api/admin/distribution/analytics", admin, nil, 200), &summary)
		if summary.Summary.TotalCommissionAmount != 0 || summary.Summary.TotalOrderAmount != 0 {
			t.Fatalf("refund totals: %+v", summary)
		}
		var status, level string
		if err := database.QueryRow(`SELECT o.status,u.member_level FROM app_orders o JOIN app_users u ON u.id=o.app_user_id WHERE o.out_trade_no=$1`, order.OutTradeNo).Scan(&status, &level); err != nil {
			t.Fatal(err)
		}
		if status != "refunded" || level != "free" {
			t.Fatalf("refund state: %s %s", status, level)
		}
	})
	step("no_invitation_no_commission_and_app_stays_manual", func(t *testing.T) {
		var other appOrderResp
		decode(t, request(t, "POST", "/api/app/web/billing/orders", outsider.AccessToken, map[string]any{"productId": "vip_month", "payChannel": "alipay", "expectedAmountCents": 2900, "webReturnUrl": "https://web.example.test/#/billing"}, 200), &other)
		callback(t, other, "29.00", "TRADE_SUCCESS", true, 200)
		if len(commissions(t)) != 3 {
			t.Fatal("uninvited buyer generated commissions")
		}
		var native appOrderResp
		decode(t, request(t, "POST", "/api/app/native/billing/orders", buyer.AccessToken, map[string]any{"productId": "vip_month"}, 200), &native)
		if native.PurchaseMode != appPurchaseModeCustomerService || native.Status != appOrderPendingConfirmation || native.PayEnabled || !strings.Contains(native.Message, "客服") {
			t.Fatalf("App payment changed: %+v", native)
		}
		var nativeID int64
		if err := database.QueryRow(`SELECT id FROM app_orders WHERE out_trade_no=$1`, native.OutTradeNo).Scan(&nativeID); err != nil {
			t.Fatal(err)
		}
		grantPath := fmt.Sprintf("/api/app-orders/%d/grant", nativeID)
		payload := map[string]any{"activationAt": time.Now().UTC().Format(time.RFC3339)}
		request(t, "POST", grantPath, admin, payload, 200)
		request(t, "POST", grantPath, admin, payload, 200)
		rows := commissions(t)
		if len(rows) != 6 {
			t.Fatalf("manual receipt should add exactly three commissions, got %d", len(rows))
		}
		count := 0
		for _, row := range rows {
			if row.OrderID == nativeID {
				count++
				if row.Amount != 480 || row.OrderAmount != 2400 || row.Status != "pending" || row.RuleVersion != nextRule.Version {
					t.Fatalf("manual receipt ledger: %+v", row)
				}
			}
		}
		if count != 3 {
			t.Fatalf("manual receipt commission count=%d", count)
		}

	})
}
