package server

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"testing"

	"nine-xing/nx-backend/apps/server/internal/auth"
	"nine-xing/nx-backend/apps/server/internal/testdb"
)

// These tests exercise PostgreSQL, not substring matching or a permissive SQL stub.
func distributionDatabase(t *testing.T) *sql.DB {
	t.Helper()
	dsn := os.Getenv("TEST_DATABASE_URL")
	if dsn == "" {
		t.Skip("TEST_DATABASE_URL required for distribution PostgreSQL tests")
	}
	db, _ := testdb.OpenIsolatedSchema(t, dsn, "distribution")
	_, err := db.Exec(`CREATE TABLE users(id BIGINT PRIMARY KEY);
 CREATE TABLE app_users(id BIGINT PRIMARY KEY);
 CREATE TABLE app_orders(id BIGINT PRIMARY KEY);
 INSERT INTO app_users VALUES(100),(200),(300),(400);
 INSERT INTO app_orders VALUES(1);`)
	if err != nil {
		t.Fatal(err)
	}
	raw, err := os.ReadFile("../db/schema.sql")
	if err != nil {
		t.Fatal(err)
	}
	start := strings.Index(string(raw), "-- ===== 分销代理体系 =====")
	if start < 0 {
		t.Fatal("distribution schema missing")
	}
	if _, err = db.Exec(string(raw[start:])); err != nil {
		t.Fatal(err)
	}
	_, err = db.Exec(`INSERT INTO distribution_agents(id,app_user_id,agent_code,level,parent_agent_id,root_agent_id,agent_path) VALUES
 (10,100,'A10',1,NULL,10,'/10/'),(20,200,'A20',2,10,10,'/10/20/'),(30,300,'A30',3,20,10,'/10/20/30/');
 INSERT INTO distribution_user_relations(app_user_id,direct_agent_id) VALUES(200,10),(300,20),(400,30);
 UPDATE distribution_commission_rule_items SET rate_bps=1000;`)
	if err != nil {
		t.Fatal(err)
	}
	return db
}

func TestDistributionPostgresCommissionSnapshotAndReplay(t *testing.T) {
	db := distributionDatabase(t)
	for i := 0; i < 2; i++ {
		tx, err := db.Begin()
		if err != nil {
			t.Fatal(err)
		}
		if err = generateDistributionCommissionsTx(context.Background(), tx, 1, 400, 10000); err != nil {
			tx.Rollback()
			t.Fatalf("paid order commission generation: %v", err)
		}
		if err = tx.Commit(); err != nil {
			t.Fatal(err)
		}
	}
	var count, amount int64
	var paths string
	err := db.QueryRow(`SELECT count(*),sum(commission_amount),string_agg(DISTINCT chain_snapshot,',') FROM distribution_commission_records`).Scan(&count, &amount, &paths)
	if err != nil {
		t.Fatal(err)
	}
	if count != 3 || amount != 3000 || paths != "/10/20/30/" {
		t.Fatalf("snapshot count=%d amount=%d paths=%s", count, amount, paths)
	}
}

func TestDistributionPostgresAdminAgentCreateAutoGeneratesAgentCode(t *testing.T) {
	db := distributionDatabase(t)
	s := &Server{db: db}

	missingUser := distributionRequest(s.adminDistributionAgentCreate, "/", `{"appUserId":999}`)
	if missingUser.Code != 404 || !strings.Contains(missingUser.Body.String(), "app user not found") {
		t.Fatalf("missing user status=%d body=%s", missingUser.Code, missingUser.Body.String())
	}

	existingAgent := distributionRequest(s.adminDistributionAgentCreate, "/", `{"appUserId":100}`)
	if existingAgent.Code != 409 || !strings.Contains(existingAgent.Body.String(), "user is already an agent") {
		t.Fatalf("existing agent status=%d body=%s", existingAgent.Code, existingAgent.Body.String())
	}

	created := distributionRequest(s.adminDistributionAgentCreate, "/", `{"appUserId":400}`)
	if created.Code != 200 || !strings.Contains(created.Body.String(), `"agentCode":"A400"`) {
		t.Fatalf("create agent status=%d body=%s", created.Code, created.Body.String())
	}
}

func TestDistributionPostgresChildCreationReturnsPersistedAgent(t *testing.T) {
	db := distributionDatabase(t)
	if _, err := db.Exec(`INSERT INTO app_users VALUES(500); INSERT INTO distribution_user_relations(app_user_id,direct_agent_id) VALUES(500,10)`); err != nil {
		t.Fatal(err)
	}
	s := &Server{db: db}
	req := httptest.NewRequest(http.MethodPost, "/api/app/distribution/children", strings.NewReader(`{"appUserId":500}`))
	req = req.WithContext(contextWithAppUser(req.Context(), auth.UserInfo{ID: 100}))
	w := httptest.NewRecorder()
	s.appDistributionCreateChild(w, req)
	if w.Code != 200 {
		t.Fatalf("child creation status=%d body=%s", w.Code, w.Body.String())
	}
	if !strings.Contains(w.Body.String(), `"status":"active"`) {
		t.Fatalf("missing active child: %s", w.Body.String())
	}
}

func TestDistributionPostgresActivateDocumentedRoute(t *testing.T) {
	db := distributionDatabase(t)
	var id int64
	if err := db.QueryRow(`INSERT INTO distribution_commission_rules(version,status) VALUES(2,'draft') RETURNING id`).Scan(&id); err != nil {
		t.Fatal(err)
	}
	// Baseline id is 1, new draft is 2 in the isolated schema.
	w := httptest.NewRecorder()
	s := &Server{db: db}
	s.adminDistributionRuleActivate(w, httptest.NewRequest(http.MethodPost, "/api/admin/distribution/rules/2/activate", strings.NewReader(`{}`)))
	if w.Code != 200 {
		t.Fatalf("activate status=%d body=%s", w.Code, w.Body.String())
	}
	var status string
	if err := db.QueryRow(`SELECT status FROM distribution_commission_rules WHERE id=$1`, id).Scan(&status); err != nil {
		t.Fatal(err)
	}
	if status != "active" {
		t.Fatalf("status=%s", status)
	}
}

func TestDistributionPostgresFrontendListContract(t *testing.T) {
	db := distributionDatabase(t)
	if _, err := db.Exec(`INSERT INTO distribution_settlements(agent_id,period_start,period_end,amount) VALUES(10,'2026-09-01','2026-09-30',1000)`); err != nil {
		t.Fatal(err)
	}
	s := &Server{db: db}
	for _, tc := range []struct {
		name    string
		handler http.HandlerFunc
		keys    []string
	}{
		{"rules", s.adminDistributionRules, []string{"id", "version", "status", "createdAt", "activatedAt"}},
		{"settlements", s.adminDistributionSettlements, []string{"id", "agentId", "periodStart", "periodEnd", "amount", "status", "createdAt"}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			w := httptest.NewRecorder()
			tc.handler(w, httptest.NewRequest(http.MethodGet, "/", nil))
			if w.Code != 200 {
				t.Fatalf("status=%d body=%s", w.Code, w.Body.String())
			}
			var body struct {
				Data struct {
					Items []map[string]any `json:"items"`
				} `json:"data"`
			}
			if err := json.Unmarshal(w.Body.Bytes(), &body); err != nil {
				t.Fatal(err)
			}
			if len(body.Data.Items) != 1 {
				t.Fatalf("items=%v", body.Data.Items)
			}
			for _, key := range tc.keys {
				if _, ok := body.Data.Items[0][key]; !ok {
					t.Errorf("missing frontend key %s in %v", key, body.Data.Items[0])
				}
			}
		})
	}
}

func TestDistributionPostgresConcurrentActivation(t *testing.T) {
	db := distributionDatabase(t)
	_, err := db.Exec(`INSERT INTO distribution_commission_rules(version,status) VALUES(2,'draft'),(3,'draft');
 CREATE FUNCTION slow_rule_activate() RETURNS trigger LANGUAGE plpgsql AS $$ BEGIN IF NEW.status='active' THEN PERFORM pg_sleep(0.15); END IF; RETURN NEW; END $$;
 CREATE TRIGGER slow_rule_activate BEFORE UPDATE ON distribution_commission_rules FOR EACH ROW EXECUTE FUNCTION slow_rule_activate();`)
	if err != nil {
		t.Fatal(err)
	}
	s := &Server{db: db}
	start := make(chan struct{})
	results := make(chan int, 2)
	for _, id := range []int{2, 3} {
		go func(id int) {
			<-start
			w := httptest.NewRecorder()
			s.adminDistributionRuleActivate(w, httptest.NewRequest(http.MethodPost, fmt.Sprintf("/api/admin/distribution/rules/%d/activate", id), nil))
			results <- w.Code
		}(id)
	}
	close(start)
	for range 2 {
		if code := <-results; code != 200 {
			t.Errorf("activation status=%d", code)
		}
	}
	var active int
	if err = db.QueryRow(`SELECT count(*) FROM distribution_commission_rules WHERE status='active'`).Scan(&active); err != nil {
		t.Fatal(err)
	}
	if active != 1 {
		t.Fatalf("active rules=%d, want exactly one", active)
	}
}

func seedDistributionCommission(t *testing.T, db *sql.DB) {
	t.Helper()
	_, err := db.Exec(`INSERT INTO distribution_commission_records(order_id,agent_id,app_user_id,agent_level,order_amount,rate_bps,commission_amount,rule_version,chain_snapshot,created_at) VALUES(1,10,400,1,10000,1000,1000,1,'/10/','2026-09-10')`)
	if err != nil {
		t.Fatal(err)
	}
}
func distributionRequest(h http.HandlerFunc, path, body string) *httptest.ResponseRecorder {
	w := httptest.NewRecorder()
	h(w, httptest.NewRequest(http.MethodPost, path, strings.NewReader(body)))
	return w
}
func TestDistributionPostgresSettlementReservations(t *testing.T) {
	db := distributionDatabase(t)
	seedDistributionCommission(t, db)
	s := &Server{db: db}
	body := `{"agentId":10,"start":"2026-09-01","end":"2026-09-30"}`
	first := distributionRequest(s.adminDistributionSettlementCreate, "/", body)
	if first.Code != 200 {
		t.Fatal(first.Body.String())
	}
	preview := distributionRequest(s.adminDistributionSettlementPreview, "/", body)
	if !strings.Contains(preview.Body.String(), `"count":0`) {
		t.Errorf("reserved commission still available: %s", preview.Body.String())
	}
	second := distributionRequest(s.adminDistributionSettlementCreate, "/", `{"agentId":10,"start":"2026-09-02","end":"2026-09-30"}`)
	if second.Code != 409 {
		t.Errorf("overlapping settlement status=%d: %s", second.Code, second.Body.String())
	}
	cancel := distributionRequest(s.adminDistributionSettlementAction, "/api/admin/distribution/settlements/1/cancel", `{"reason":"重新核账"}`)
	if cancel.Code != 200 {
		t.Fatalf("cancel: %s", cancel.Body.String())
	}
	retry := distributionRequest(s.adminDistributionSettlementCreate, "/", body)
	if retry.Code != 200 {
		t.Fatalf("recreate canceled period: %s", retry.Body.String())
	}
}
func TestDistributionPostgresPayoutRejectsReversedCommission(t *testing.T) {
	db := distributionDatabase(t)
	seedDistributionCommission(t, db)
	s := &Server{db: db}
	distributionRequest(s.adminDistributionSettlementCreate, "/", `{"agentId":10,"start":"2026-09-01","end":"2026-09-30"}`)
	distributionRequest(s.adminDistributionSettlementAction, "/api/admin/distribution/settlements/1/approve", `{}`)
	if _, err := db.Exec(`UPDATE distribution_commission_records SET status='reversed'`); err != nil {
		t.Fatal(err)
	}
	w := distributionRequest(s.adminDistributionSettlementAction, "/api/admin/distribution/settlements/1/paid", `{"paymentReference":"test-bank-001"}`)
	if w.Code != 409 {
		t.Fatalf("refunded commission payout status=%d body=%s", w.Code, w.Body.String())
	}
}
func TestDistributionPostgresPayoutAtomicOnFailure(t *testing.T) {
	db := distributionDatabase(t)
	seedDistributionCommission(t, db)
	s := &Server{db: db}
	distributionRequest(s.adminDistributionSettlementCreate, "/", `{"agentId":10,"start":"2026-09-01","end":"2026-09-30"}`)
	distributionRequest(s.adminDistributionSettlementAction, "/api/admin/distribution/settlements/1/approve", `{}`)
	_, err := db.Exec(`CREATE FUNCTION fail_settle() RETURNS trigger LANGUAGE plpgsql AS $$ BEGIN IF NEW.status='settled' THEN RAISE EXCEPTION 'simulated storage failure'; END IF; RETURN NEW; END $$; CREATE TRIGGER fail_settle BEFORE UPDATE ON distribution_commission_records FOR EACH ROW EXECUTE FUNCTION fail_settle();`)
	if err != nil {
		t.Fatal(err)
	}
	w := distributionRequest(s.adminDistributionSettlementAction, "/api/admin/distribution/settlements/1/paid", `{"paymentReference":"test-bank-002"}`)
	if w.Code < 400 {
		t.Fatal("expected storage error")
	}
	var status string
	if err = db.QueryRow(`SELECT status FROM distribution_settlements WHERE id=1`).Scan(&status); err != nil {
		t.Fatal(err)
	}
	if status != "approved" {
		t.Fatalf("partial payout committed: status=%s", status)
	}
}
func TestDistributionPostgresLoginBindingRejectsSelfAndAgentCycle(t *testing.T) {
	db := distributionDatabase(t)
	bindDistributionAgent(context.Background(), db, 100, "A10")
	var n int
	if err := db.QueryRow(`SELECT count(*) FROM distribution_user_relations WHERE app_user_id=100`).Scan(&n); err != nil {
		t.Fatal(err)
	}
	if n != 0 {
		t.Fatal("login created self referral")
	}
	bindDistributionAgent(context.Background(), db, 100, "A30")
	if err := db.QueryRow(`SELECT count(*) FROM distribution_user_relations WHERE app_user_id=100`).Scan(&n); err != nil {
		t.Fatal(err)
	}
	if n != 0 {
		t.Fatal("existing root agent bound to own descendant")
	}
}

func TestDistributionPostgresRulesValidateBudget(t *testing.T) {
	db := distributionDatabase(t)
	s := &Server{db: db}
	for _, body := range []string{`{"name":"over","rates":{"1":6000,"2":6000,"3":0}}`, `{"name":"missing","rates":{"1":1000}}`} {
		w := distributionRequest(s.adminDistributionRuleCreate, "/", body)
		if w.Code != 400 {
			t.Errorf("invalid rule accepted %d: %s", w.Code, w.Body.String())
		}
	}
}
func TestDistributionPostgresConcurrentSettlementCreation(t *testing.T) {
	db := distributionDatabase(t)
	seedDistributionCommission(t, db)
	s := &Server{db: db}
	start := make(chan struct{})
	results := make(chan int, 2)
	for _, day := range []string{"01", "02"} {
		go func(day string) {
			<-start
			w := distributionRequest(s.adminDistributionSettlementCreate, "/", fmt.Sprintf(`{"agentId":10,"start":"2026-09-%s","end":"2026-09-30"}`, day))
			results <- w.Code
		}(day)
	}
	close(start)
	a, b := <-results, <-results
	if !((a == 200 && b == 409) || (a == 409 && b == 200)) {
		t.Fatalf("concurrent reservation statuses %d/%d", a, b)
	}
}
func TestDistributionPostgresSuccessfulPayoutRequiresProof(t *testing.T) {
	db := distributionDatabase(t)
	seedDistributionCommission(t, db)
	s := &Server{db: db}
	distributionRequest(s.adminDistributionSettlementCreate, "/", `{"agentId":10,"start":"2026-09-01","end":"2026-09-30"}`)
	distributionRequest(s.adminDistributionSettlementAction, "/api/admin/distribution/settlements/1/approve", `{}`)
	noProof := distributionRequest(s.adminDistributionSettlementAction, "/api/admin/distribution/settlements/1/paid", `{}`)
	if noProof.Code != 400 {
		t.Fatal("payout proof must be required")
	}
	w := distributionRequest(s.adminDistributionSettlementAction, "/api/admin/distribution/settlements/1/paid", `{"paymentReference":"test-bank-003"}`)
	if w.Code != 200 {
		t.Fatal(w.Body.String())
	}
	var status string
	if err := db.QueryRow(`SELECT status FROM distribution_commission_records WHERE agent_id=10`).Scan(&status); err != nil {
		t.Fatal(err)
	}
	if status != "settled" {
		t.Fatal(status)
	}
	replay := distributionRequest(s.adminDistributionSettlementAction, "/api/admin/distribution/settlements/1/paid", `{"paymentReference":"test-bank-003"}`)
	if replay.Code != 409 {
		t.Fatal("duplicate payout accepted")
	}
}
