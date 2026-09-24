package server

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"testing"

	"nine-xing/nx-backend/apps/server/internal/appuser"
	"nine-xing/nx-backend/apps/server/internal/auth"
	"nine-xing/nx-backend/apps/server/internal/testdb"
)

func TestAppBillingDiscountPostgresCheckoutAndCancel(t *testing.T) {
	db, _ := testdb.OpenEnvIsolatedSchema(t, "billing_discount")
	schema, err := os.ReadFile("../db/schema.sql")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := db.Exec(string(schema)); err != nil {
		t.Fatalf("initialize billing schema: %v", err)
	}
	if _, err := db.Exec(`
		INSERT INTO app_users(id,phone) VALUES(9001,'billing-agent-9001'),(9002,'billing-buyer-9002');
		INSERT INTO distribution_agents(id,app_user_id,agent_code,level,root_agent_id,agent_path)
		VALUES(9001,9001,'A9001',1,9001,'/9001/');
		INSERT INTO app_agent_discount_rules(product_id,audience,mode,value,enabled)
		VALUES('vip_month','invited_user','amount_off',500,true),
		      ('svip_month','agent_self','percent_off',2000,true);
	`); err != nil {
		t.Fatal(err)
	}
	s := &Server{db: db, appUsers: appuser.NewStore(db)}
	request := func(userID int64, handler http.HandlerFunc, path string, payload any) *httptest.ResponseRecorder {
		t.Helper()
		body, err := json.Marshal(payload)
		if err != nil {
			t.Fatal(err)
		}
		r := httptest.NewRequest(http.MethodPost, path, bytes.NewReader(body))
		r = r.WithContext(contextWithAppUser(r.Context(), auth.UserInfo{ID: userID}))
		response := httptest.NewRecorder()
		handler(response, r)
		return response
	}

	created := request(9002, s.appBillingCreateOrder, "/api/app/billing/orders", map[string]any{
		"productId": "vip_month", "agentCode": "A9001", "expectedAmountCents": 2400,
	})
	if created.Code != http.StatusOK {
		t.Fatalf("discounted checkout: status=%d body=%s", created.Code, created.Body.String())
	}
	order := decodeAppBillingResponse(t, created).Data
	if order.Amount != 2400 || order.BasePriceCents != 2900 || order.DiscountCents != 500 {
		t.Fatalf("unexpected checkout price: %+v", order)
	}
	var amount, base, discount int
	var snapshot []byte
	if err := db.QueryRow(`SELECT amount,base_price_cents,discount_cents,discount_snapshot FROM app_orders WHERE out_trade_no=$1`, order.OutTradeNo).Scan(&amount, &base, &discount, &snapshot); err != nil {
		t.Fatal(err)
	}
	var storedDiscount appAgentDiscountQuote
	if err := json.Unmarshal(snapshot, &storedDiscount); err != nil {
		t.Fatal(err)
	}
	if amount != 2400 || base != 2900 || discount != 500 || storedDiscount.AgentCode != "A9001" || storedDiscount.PayableCents != amount {
		t.Fatalf("stored price snapshot mismatch: amount=%d base=%d discount=%d snapshot=%s", amount, base, discount, snapshot)
	}
	var bound int
	if err := db.QueryRow(`SELECT count(*) FROM distribution_user_relations WHERE app_user_id=9002`).Scan(&bound); err != nil || bound != 0 {
		t.Fatalf("checkout code changed registration attribution: count=%d err=%v", bound, err)
	}
	canceled := request(9002, s.appBillingCancelOrder, "/api/app/billing/orders/cancel", map[string]any{"outTradeNo": order.OutTradeNo})
	if canceled.Code != http.StatusOK {
		t.Fatalf("cancel manual pending order: status=%d body=%s", canceled.Code, canceled.Body.String())
	}
	regular := request(9002, s.appBillingCreateOrder, "/api/app/billing/orders", map[string]any{
		"productId": "vip_month", "expectedAmountCents": 2900,
	})
	if regular.Code != http.StatusOK || decodeAppBillingResponse(t, regular).Data.Amount != 2900 {
		t.Fatalf("checkout after cancel: status=%d body=%s", regular.Code, regular.Body.String())
	}
	conflicting := request(9002, s.appBillingCreateOrder, "/api/app/billing/orders", map[string]any{
		"productId": "vip_month", "agentCode": "A9001", "expectedAmountCents": 2400,
	})
	if conflicting.Code != http.StatusConflict {
		t.Fatalf("existing full-price order must not be reused for new discount: status=%d body=%s", conflicting.Code, conflicting.Body.String())
	}

	if _, err := db.Exec(`
		UPDATE app_users SET member_level='vip_month',member_started_at=now()-INTERVAL '15 days',member_expires_at=now()+INTERVAL '15 days' WHERE id=9001;
		INSERT INTO app_orders(out_trade_no,app_user_id,product_id,title,amount,base_price_cents,discount_cents,duration_days,status,purchase_mode,payment_provider,paid_at,activation_at)
		VALUES('prior-discounted-vip',9001,'vip_month','VIP 月卡',2320,2900,580,30,'paid','customer_service','manual',now()-INTERVAL '15 days',now()-INTERVAL '15 days');
	`); err != nil {
		t.Fatal(err)
	}
	paidCancel := request(9001, s.appBillingCancelOrder, "/api/app/billing/orders/cancel", map[string]any{"outTradeNo": "prior-discounted-vip"})
	if paidCancel.Code != http.StatusConflict {
		t.Fatalf("paid order must not be cancellable: status=%d body=%s", paidCancel.Code, paidCancel.Body.String())
	}
	quoted := request(9001, s.appBillingUpgradeQuote, "/api/app/billing/upgrade-quote", map[string]any{"targetProductId": "svip_month"})
	if quoted.Code != http.StatusOK {
		t.Fatalf("upgrade quote: status=%d body=%s", quoted.Code, quoted.Body.String())
	}
	var quoteBody struct {
		Data appUpgradeQuoteResp `json:"data"`
	}
	if err := json.Unmarshal(quoted.Body.Bytes(), &quoteBody); err != nil {
		t.Fatal(err)
	}
	quote := quoteBody.Data
	if quote.BasePayableCents != quote.PayableCents+quote.DiscountCents || quote.CreditCents >= 3000 || quote.DiscountCents <= 0 {
		t.Fatalf("upgrade credit or discount wrong: %+v", quote)
	}
	upgraded := request(9001, s.appBillingCreateOrder, "/api/app/billing/orders", map[string]any{
		"productId": "svip_month", "upgradeFromPlan": "vip_month", "expectedAmountCents": quote.PayableCents,
	})
	if upgraded.Code != http.StatusOK || decodeAppBillingResponse(t, upgraded).Data.Amount != quote.PayableCents {
		t.Fatalf("upgrade checkout differs from quote: status=%d body=%s quote=%+v", upgraded.Code, upgraded.Body.String(), quote)
	}
	upgradeOrder := decodeAppBillingResponse(t, upgraded).Data
	var upgradeSnapshot []byte
	if err := db.QueryRow(`SELECT discount_snapshot FROM app_orders WHERE out_trade_no=$1`, upgradeOrder.OutTradeNo).Scan(&upgradeSnapshot); err != nil {
		t.Fatal(err)
	}
	var pricingSnapshot struct {
		BasePriceCents     int `json:"basePriceCents"`
		UpgradeCreditCents int `json:"upgradeCreditCents"`
		DiscountCents      int `json:"discountCents"`
		PayableCents       int `json:"payableCents"`
	}
	if err := json.Unmarshal(upgradeSnapshot, &pricingSnapshot); err != nil {
		t.Fatal(err)
	}
	if pricingSnapshot.BasePriceCents-pricingSnapshot.UpgradeCreditCents-pricingSnapshot.DiscountCents != pricingSnapshot.PayableCents || pricingSnapshot.PayableCents != upgradeOrder.Amount {
		t.Fatalf("upgrade snapshot does not reconcile to charge: %+v order=%+v", pricingSnapshot, upgradeOrder)
	}
}
