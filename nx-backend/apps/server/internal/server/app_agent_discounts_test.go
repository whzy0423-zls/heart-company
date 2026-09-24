package server

import (
	"bytes"
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"

	"nine-xing/nx-backend/apps/server/internal/auth"
	"nine-xing/nx-backend/apps/server/internal/testdb"
)

func TestAppAgentDiscountAmountUsesCentsAndKeepsPositivePayable(t *testing.T) {
	tests := []struct {
		name    string
		base    int
		mode    string
		value   int
		wantOff int
		wantPay int
	}{
		{"percentage rounds to cents", 2999, "percent_off", 3333, 1000, 1999},
		{"fixed reduction", 5900, "amount_off", 1000, 1000, 4900},
		{"large fixed reduction keeps one cent", 2900, "amount_off", 4000, 2899, 1},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			off, payable := calculateAppAgentDiscount(tt.base, tt.mode, tt.value)
			if off != tt.wantOff || payable != tt.wantPay {
				t.Fatalf("discount=%d payable=%d, want %d/%d", off, payable, tt.wantOff, tt.wantPay)
			}
		})
	}
}

func TestValidateAppAgentDiscountRule(t *testing.T) {
	for _, rule := range []appAgentDiscountRule{
		{ProductID: "vip_month", Audience: "agent_self", Mode: "percent_off", Value: 1500, Enabled: true},
		{ProductID: "svip_year", Audience: "invited_user", Mode: "amount_off", Value: 1200, Enabled: true},
		{ProductID: "svip_year", Audience: "invited_user", Mode: "amount_off", Value: 0, Enabled: false},
	} {
		if err := validateAppAgentDiscountRule(rule); err != nil {
			t.Fatalf("valid rule %+v: %v", rule, err)
		}
	}
	for _, rule := range []appAgentDiscountRule{
		{ProductID: "free", Audience: "agent_self", Mode: "percent_off", Value: 1000},
		{ProductID: "vip_month", Audience: "agent_self", Mode: "percent_off", Value: 10000, Enabled: true},
		{ProductID: "vip_month", Audience: "other", Mode: "percent_off", Value: 1000},
		{ProductID: "vip_month", Audience: "invited_user", Mode: "amount_off", Value: 0, Enabled: true},
	} {
		if err := validateAppAgentDiscountRule(rule); err == nil {
			t.Fatalf("invalid rule accepted: %+v", rule)
		}
	}
}

func appAgentDiscountTestDatabase(t *testing.T) *sql.DB {
	t.Helper()
	db, _ := testdb.OpenEnvIsolatedSchema(t, "agent_discount")
	_, err := db.Exec(`
CREATE TABLE app_users(id BIGINT PRIMARY KEY);
CREATE TABLE distribution_agents(id BIGINT PRIMARY KEY, app_user_id BIGINT NOT NULL UNIQUE, agent_code TEXT NOT NULL UNIQUE, level SMALLINT NOT NULL, root_agent_id BIGINT NOT NULL, status TEXT NOT NULL);
CREATE TABLE distribution_user_relations(app_user_id BIGINT NOT NULL UNIQUE, direct_agent_id BIGINT NOT NULL);
CREATE TABLE app_plans(code TEXT PRIMARY KEY, price_cents INT NOT NULL, enabled BOOLEAN NOT NULL);
CREATE TABLE app_agent_discount_rules(product_id TEXT NOT NULL, audience TEXT NOT NULL, mode TEXT NOT NULL, value INT NOT NULL, enabled BOOLEAN NOT NULL DEFAULT false, updated_at TIMESTAMPTZ NOT NULL DEFAULT now(), PRIMARY KEY(product_id,audience));
INSERT INTO app_users(id) VALUES(10),(20),(30),(40),(50),(60),(70),(80);
INSERT INTO distribution_agents(id,app_user_id,agent_code,level,root_agent_id,status) VALUES(1,10,'A10',1,1,'active'),(2,20,'A20',1,2,'paused'),(3,30,'A30',2,1,'active'),(4,70,'A70',1,4,'active');
INSERT INTO distribution_user_relations(app_user_id,direct_agent_id) VALUES(40,1),(50,3),(80,4);
INSERT INTO app_plans(code,price_cents,enabled) VALUES('vip_month',2900,true),('svip_year',49900,true),('vip_year',19900,false);
INSERT INTO app_agent_discount_rules(product_id,audience,mode,value,enabled) VALUES('vip_month','agent_self','percent_off',2000,true),('vip_month','invited_user','amount_off',500,true);
`)
	if err != nil {
		t.Fatal(err)
	}
	return db
}

func TestQuoteAppAgentDiscountEligibilityAndInviteIsolation(t *testing.T) {
	db := appAgentDiscountTestDatabase(t)
	s := &Server{db: db}
	ctx := context.Background()
	tests := []struct {
		name         string
		userID       int64
		code         string
		wantAudience string
		wantAgentID  int64
		wantPay      int
		wantErr      error
	}{
		{"agent self", 10, "", "agent_self", 1, 2320, nil},
		{"direct invite", 40, "", "invited_user", 1, 2400, nil},
		{"descendant invite", 50, "", "invited_user", 1, 2400, nil},
		{"same root code", 50, "A10", "invited_user", 1, 2400, nil},
		{"checkout code", 60, " a10 ", "invited_user", 1, 2400, nil},
		{"unbound regular price", 60, "", "", 0, 2900, nil},
		{"paused agent", 60, "A20", "", 0, 0, errAppAgentDiscountInvalidCode},
		{"second level agent", 60, "A30", "", 0, 0, errAppAgentDiscountInvalidCode},
		{"self invite", 10, "A10", "", 0, 0, errAppAgentDiscountSelfInvite},
		{"different existing root", 80, "A10", "", 0, 0, errAppAgentDiscountAttributionConflict},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			quote, err := s.quoteAppAgentDiscount(ctx, tt.userID, "vip_month", tt.code, 2900)
			if !errors.Is(err, tt.wantErr) {
				t.Fatalf("error=%v, want %v", err, tt.wantErr)
			}
			if err == nil && (quote.Audience != tt.wantAudience || quote.AgentID != tt.wantAgentID || quote.PayableCents != tt.wantPay) {
				t.Fatalf("quote=%+v, want audience=%s agent=%d payable=%d", quote, tt.wantAudience, tt.wantAgentID, tt.wantPay)
			}
		})
	}
	var count int
	if err := db.QueryRow(`SELECT count(*) FROM distribution_user_relations WHERE app_user_id=60`).Scan(&count); err != nil || count != 0 {
		t.Fatalf("checkout code changed attribution: count=%d err=%v", count, err)
	}
}

func TestAppAgentDiscountAdminReplaceAndAppQuote(t *testing.T) {
	db := appAgentDiscountTestDatabase(t)
	s := &Server{db: db}
	put := httptest.NewRequest(http.MethodPut, "/api/admin/app-agent-discounts", bytes.NewBufferString(`{"rules":[{"productId":"vip_month","audience":"invited_user","mode":"percent_off","value":1000,"enabled":true},{"productId":"svip_year","audience":"agent_self","mode":"amount_off","value":0,"enabled":false}]}`))
	putRec := httptest.NewRecorder()
	s.adminAppAgentDiscounts(putRec, put)
	if putRec.Code != http.StatusOK {
		t.Fatalf("admin PUT status=%d body=%s", putRec.Code, putRec.Body.String())
	}
	var count int
	if err := db.QueryRow(`SELECT count(*) FROM app_agent_discount_rules`).Scan(&count); err != nil || count != 2 {
		t.Fatalf("replace left %d rules: %v", count, err)
	}
	quoteReq := httptest.NewRequest(http.MethodPost, "/api/app/billing/discount-quote", bytes.NewBufferString(`{"agentCode":"A10"}`))
	quoteReq = quoteReq.WithContext(contextWithAppUser(quoteReq.Context(), auth.UserInfo{ID: 60}))
	quoteRec := httptest.NewRecorder()
	s.appBillingDiscountQuote(quoteRec, quoteReq)
	if quoteRec.Code != http.StatusOK {
		t.Fatalf("App quote status=%d body=%s", quoteRec.Code, quoteRec.Body.String())
	}
	var response struct {
		Data struct {
			Audience  string                  `json:"audience"`
			AgentCode string                  `json:"agentCode"`
			Prices    []appAgentDiscountQuote `json:"prices"`
		} `json:"data"`
	}
	if err := json.Unmarshal(quoteRec.Body.Bytes(), &response); err != nil {
		t.Fatal(err)
	}
	if response.Data.Audience != "invited_user" || response.Data.AgentCode != "A10" || len(response.Data.Prices) != 2 {
		t.Fatalf("quote response=%+v", response.Data)
	}
	if response.Data.Prices[0].ProductID != "vip_month" || response.Data.Prices[0].DiscountCents != 290 || response.Data.Prices[0].PayableCents != 2610 {
		t.Fatalf("discount quote=%+v", response.Data.Prices[0])
	}
	disabled, err := s.quoteAppAgentDiscount(context.Background(), 10, "svip_year", "", 49900)
	if err != nil || disabled.DiscountCents != 0 || disabled.PayableCents != 49900 {
		t.Fatalf("disabled rule changed price: quote=%+v err=%v", disabled, err)
	}
}
