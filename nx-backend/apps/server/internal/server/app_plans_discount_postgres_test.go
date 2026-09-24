package server

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"testing"

	"nine-xing/nx-backend/apps/server/internal/testdb"
)

func TestAdminAppPlanUpdateRejectsConflictingAgentAmountDiscount(t *testing.T) {
	db, _ := testdb.OpenEnvIsolatedSchema(t, "plan_discount")
	schema, err := os.ReadFile("../db/schema.sql")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := db.Exec(string(schema)); err != nil {
		t.Fatalf("initialize app schema: %v", err)
	}
	if _, err := db.Exec(`INSERT INTO app_agent_discount_rules(product_id,audience,mode,value,enabled)
		VALUES('vip_month','invited_user','amount_off',2500,true)`); err != nil {
		t.Fatal(err)
	}
	s := &Server{db: db}
	updatePrice := func(price int) *httptest.ResponseRecorder {
		t.Helper()
		plan := s.appPlan(context.Background(), "vip_month")
		plan.PriceCents = price
		body, err := json.Marshal(plan)
		if err != nil {
			t.Fatal(err)
		}
		r := httptest.NewRequest(http.MethodPut, "/api/admin/app-plans/vip_month", bytes.NewReader(body))
		w := httptest.NewRecorder()
		s.adminAppPlanUpdate(w, r)
		return w
	}
	storedPrice := func() int {
		t.Helper()
		var price int
		if err := db.QueryRow(`SELECT price_cents FROM app_plans WHERE code='vip_month'`).Scan(&price); err != nil {
			t.Fatal(err)
		}
		return price
	}

	if response := updatePrice(2500); response.Code != http.StatusBadRequest {
		t.Fatalf("price equal to enabled reduction: status=%d body=%s", response.Code, response.Body.String())
	}
	if got := storedPrice(); got != 2900 {
		t.Fatalf("rejected update changed price to %d, want 2900", got)
	}
	if response := updatePrice(2600); response.Code != http.StatusOK {
		t.Fatalf("price above enabled reduction: status=%d body=%s", response.Code, response.Body.String())
	}
	if got := storedPrice(); got != 2600 {
		t.Fatalf("valid update left price=%d, want 2600", got)
	}
	if _, err := db.Exec(`UPDATE app_agent_discount_rules SET enabled=false WHERE product_id='vip_month'`); err != nil {
		t.Fatal(err)
	}
	if response := updatePrice(2500); response.Code != http.StatusOK {
		t.Fatalf("disabled reduction still blocked price: status=%d body=%s", response.Code, response.Body.String())
	}
}
