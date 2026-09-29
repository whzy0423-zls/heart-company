package server

import (
	"database/sql"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
)

func distributionManualGrantDatabase(t *testing.T, amount int64) *sql.DB {
	t.Helper()
	database := openAppOrderLifecycleTestDatabase(t)
	_, err := database.Exec(`
INSERT INTO app_users(id,phone,member_level) VALUES
 (8101,'19900008101','free'),(8102,'19900008102','free'),
 (8103,'19900008103','free'),(8104,'19900008104','free');
INSERT INTO distribution_agents(id,app_user_id,agent_code,level,parent_agent_id,root_agent_id,agent_path) VALUES
 (711,8101,'MAN711',1,NULL,711,'/711/'),
 (712,8102,'MAN712',2,711,711,'/711/712/'),
 (713,8103,'MAN713',3,712,711,'/711/712/713/');
INSERT INTO distribution_user_relations(app_user_id,direct_agent_id) VALUES(8104,713);
UPDATE distribution_commission_rule_items SET rate_bps=1000;`)
	if err != nil {
		t.Fatal(err)
	}
	_, err = database.Exec(`INSERT INTO app_orders
 (id,out_trade_no,app_user_id,product_id,title,amount,base_price_cents,discount_cents,duration_days,status,purchase_mode,payment_provider)
 VALUES(8104,'manual-distribution-fixture',8104,'vip_month','VIP 月卡',$1,10000,10000-$1,30,'pending_confirmation','customer_service','manual')`, amount)
	if err != nil {
		t.Fatal(err)
	}
	return database
}

func distributionManualGrantRequest(s *Server, activation time.Time) *httptest.ResponseRecorder {
	request := httptest.NewRequest(http.MethodPost, "/api/app-orders/8104/grant", strings.NewReader(fmt.Sprintf(`{"activationAt":%q}`, activation.Format(time.RFC3339))))
	response := httptest.NewRecorder()
	s.adminAppOrderGrant(response, request)
	return response
}

func TestDistributionPostgresManualGrantCommissionsAndRefund(t *testing.T) {
	for _, amount := range []int64{10000, 7901} {
		t.Run(fmt.Sprintf("paid_%d_cents", amount), func(t *testing.T) {
			database := distributionManualGrantDatabase(t, amount)
			s := &Server{db: database}
			activation := time.Now().UTC().Truncate(time.Second)
			for attempt := 0; attempt < 2; attempt++ {
				response := distributionManualGrantRequest(s, activation)
				if response.Code != http.StatusOK {
					t.Fatalf("grant attempt %d: status=%d body=%s", attempt, response.Code, response.Body.String())
				}
				assertAppOrderLifecycleState(t, database, 8104, 8104, "paid", "vip_month", activation.AddDate(0, 0, 30))
				var count, total, minimumOrderAmount, maximumOrderAmount int64
				if err := database.QueryRow(`SELECT count(*),COALESCE(sum(commission_amount),0),COALESCE(min(order_amount),0),COALESCE(max(order_amount),0)
 FROM distribution_commission_records WHERE order_id=8104 AND status='pending'`).Scan(&count, &total, &minimumOrderAmount, &maximumOrderAmount); err != nil {
					t.Fatal(err)
				}
				if count != 3 || total != 3*(amount/10) || minimumOrderAmount != amount || maximumOrderAmount != amount {
					t.Fatalf("manual grant commission attempt=%d count=%d total=%d paid range=%d..%d; want 3 records, total=%d, paid=%d", attempt, count, total, minimumOrderAmount, maximumOrderAmount, 3*(amount/10), amount)
				}
				t.Logf("grant attempt=%d HTTP 200 paid=%d cents, commission records=%d total=%d cents", attempt, amount, count, total)
			}
			for attempt := 0; attempt < 2; attempt++ {
				response := httptest.NewRecorder()
				s.adminAppOrderRefund(response, httptest.NewRequest(http.MethodPost, "/api/app-orders/8104/refund", strings.NewReader(`{"reason":"isolated fixture refund"}`)))
				if response.Code != http.StatusOK {
					t.Fatalf("refund attempt %d: status=%d body=%s", attempt, response.Code, response.Body.String())
				}
				assertAppOrderLifecycleState(t, database, 8104, 8104, "refunded", "free", time.Time{})
				var reversed, other int64
				if err := database.QueryRow(`SELECT count(*) FILTER (WHERE status='reversed'), count(*) FILTER (WHERE status<>'reversed') FROM distribution_commission_records WHERE order_id=8104`).Scan(&reversed, &other); err != nil {
					t.Fatal(err)
				}
				if reversed != 3 || other != 0 {
					t.Fatalf("refund reversed=%d other=%d, want 3 and 0", reversed, other)
				}
			}
		})
	}
}

func TestDistributionPostgresManualGrantCommissionFailureRollsBackPayment(t *testing.T) {
	database := distributionManualGrantDatabase(t, 10000)
	if _, err := database.Exec(`CREATE FUNCTION fixture_reject_commission() RETURNS trigger LANGUAGE plpgsql AS $$
 BEGIN RAISE EXCEPTION 'fixture commission persistence failure'; END $$;
 CREATE TRIGGER fixture_commission_failure BEFORE INSERT ON distribution_commission_records
 FOR EACH ROW EXECUTE FUNCTION fixture_reject_commission()`); err != nil {
		t.Fatal(err)
	}
	response := distributionManualGrantRequest(&Server{db: database}, time.Now().UTC().Truncate(time.Second))
	if response.Code != http.StatusInternalServerError {
		t.Fatalf("grant with failed commission write status=%d body=%s, want 500", response.Code, response.Body.String())
	}
	assertAppOrderLifecycleState(t, database, 8104, 8104, "pending_confirmation", "free", time.Time{})
	var count int
	if err := database.QueryRow(`SELECT count(*) FROM distribution_commission_records`).Scan(&count); err != nil || count != 0 {
		t.Fatalf("rollback commission count=%d err=%v", count, err)
	}
}

func TestDistributionPostgresManualGrantWithoutReferralStillGrantsMembership(t *testing.T) {
	database := distributionManualGrantDatabase(t, 10000)
	if _, err := database.Exec(`DELETE FROM distribution_user_relations WHERE app_user_id=8104`); err != nil {
		t.Fatal(err)
	}
	activation := time.Now().UTC().Truncate(time.Second)
	response := distributionManualGrantRequest(&Server{db: database}, activation)
	if response.Code != http.StatusOK {
		t.Fatalf("unattributed grant status=%d body=%s", response.Code, response.Body.String())
	}
	assertAppOrderLifecycleState(t, database, 8104, 8104, "paid", "vip_month", activation.AddDate(0, 0, 30))
	var count int
	if err := database.QueryRow(`SELECT count(*) FROM distribution_commission_records`).Scan(&count); err != nil || count != 0 {
		t.Fatalf("unattributed commission count=%d err=%v", count, err)
	}
}

// This documents the current accounting boundary: refunding after an agent
// payout retains its payment evidence but does not recover money or offset
// a future settlement. A recovery policy must be implemented explicitly.
func TestDistributionPostgresRefundAfterPayoutRetainsPaymentEvidence(t *testing.T) {
	database := distributionManualGrantDatabase(t, 10000)
	s := &Server{db: database}
	activation := time.Now().UTC().Truncate(time.Second)
	if response := distributionManualGrantRequest(s, activation); response.Code != http.StatusOK {
		t.Fatalf("grant status=%d body=%s", response.Code, response.Body.String())
	}
	period := fmt.Sprintf(`{"agentId":711,"start":%q,"end":%q}`, activation.AddDate(0, 0, -1).Format("2006-01-02"), activation.AddDate(0, 0, 1).Format("2006-01-02"))
	created := distributionRequest(s.adminDistributionSettlementCreate, "/api/admin/distribution/settlements", period)
	if created.Code != http.StatusOK {
		t.Fatalf("settlement create status=%d body=%s", created.Code, created.Body.String())
	}
	var body struct {
		Data struct {
			SettlementID int64 `json:"settlementId"`
			Amount       int64 `json:"amount"`
		} `json:"data"`
	}
	if err := json.Unmarshal(created.Body.Bytes(), &body); err != nil || body.Data.SettlementID <= 0 || body.Data.Amount != 1000 {
		t.Fatalf("settlement payload=%s err=%v", created.Body.String(), err)
	}
	for _, action := range []string{"approve", "paid"} {
		response := distributionRequest(s.adminDistributionSettlementAction, fmt.Sprintf("/api/admin/distribution/settlements/%d/%s", body.Data.SettlementID, action), `{"paymentReference":"fixture-paid-proof-1000"}`)
		if response.Code != http.StatusOK {
			t.Fatalf("settlement %s status=%d body=%s", action, response.Code, response.Body.String())
		}
	}
	refunded := distributionRequest(s.adminAppOrderRefund, "/api/app-orders/8104/refund", `{"reason":"fixture refund after agent payout"}`)
	if refunded.Code != http.StatusOK {
		t.Fatalf("refund status=%d body=%s", refunded.Code, refunded.Body.String())
	}
	var settlementStatus, reference, commissionStatus, reversalReason string
	var paidAmount, itemAmount int64
	var paidAt sql.NullTime
	if err := database.QueryRow(`SELECT st.status,st.amount,st.payment_reference,st.paid_at,i.amount,c.status,c.reversal_reason
 FROM distribution_settlements st JOIN distribution_settlement_items i ON i.settlement_id=st.id
 JOIN distribution_commission_records c ON c.id=i.commission_id WHERE st.id=$1`, body.Data.SettlementID).
		Scan(&settlementStatus, &paidAmount, &reference, &paidAt, &itemAmount, &commissionStatus, &reversalReason); err != nil {
		t.Fatal(err)
	}
	if settlementStatus != "paid" || paidAmount != 1000 || itemAmount != 1000 || reference != "fixture-paid-proof-1000" || !paidAt.Valid || commissionStatus != "reversed" || reversalReason != "fixture refund after agent payout" {
		t.Fatalf("paid evidence state=%s amount=%d item=%d reference=%s paidAt=%v commission=%s reversal=%s", settlementStatus, paidAmount, itemAmount, reference, paidAt, commissionStatus, reversalReason)
	}
	// A later payment produces its full positive commission; it does not
	// automatically recoup the refunded commission that was already paid out.
	if _, err := database.Exec(`INSERT INTO app_orders
 (id,out_trade_no,app_user_id,product_id,title,amount,duration_days,status,purchase_mode,payment_provider)
 VALUES(8105,'manual-distribution-after-refund',8104,'vip_month','VIP 月卡',10000,30,'pending_confirmation','customer_service','manual')`); err != nil {
		t.Fatal(err)
	}
	grant := distributionRequest(s.adminAppOrderGrant, "/api/app-orders/8105/grant", fmt.Sprintf(`{"activationAt":%q}`, activation.Format(time.RFC3339)))
	if grant.Code != http.StatusOK {
		t.Fatalf("next grant status=%d body=%s", grant.Code, grant.Body.String())
	}
	preview := distributionRequest(s.adminDistributionSettlementPreview, "/api/admin/distribution/settlements/preview", period)
	var previewBody struct {
		Data struct {
			Amount int64 `json:"amount"`
			Count  int   `json:"count"`
		} `json:"data"`
	}
	if err := json.Unmarshal(preview.Body.Bytes(), &previewBody); err != nil || preview.Code != http.StatusOK || previewBody.Data.Amount != 1000 || previewBody.Data.Count != 1 {
		t.Fatalf("future settlement preview status=%d body=%s err=%v", preview.Code, preview.Body.String(), err)
	}
	t.Logf("after refund: paid settlement retains %d cents and proof; linked commission=%s; next payment still offers %d cents for settlement without recovery", paidAmount, commissionStatus, previewBody.Data.Amount)
}
