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
	if again := distributionRequest(s.adminDistributionSettlementAction, "/api/admin/distribution/settlements/1/approve", `{}`); again.Code != 409 {
		t.Fatal("duplicate approval accepted")
	}
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

func TestDistributionPostgresAppOverviewMetrics(t *testing.T) {
	db := distributionDatabase(t)
	seedDistributionCommission(t, db)
	s := &Server{db: db}
	request := func() map[string]any {
		req := httptest.NewRequest(http.MethodGet, "/api/app/distribution/overview", nil)
		req = req.WithContext(contextWithAppUser(req.Context(), auth.UserInfo{ID: 100}))
		w := httptest.NewRecorder()
		s.appDistributionOverview(w, req)
		if w.Code != 200 {
			t.Fatalf("overview %d %s", w.Code, w.Body.String())
		}
		var out struct {
			Data map[string]any `json:"data"`
		}
		if err := json.Unmarshal(w.Body.Bytes(), &out); err != nil {
			t.Fatal(err)
		}
		return out.Data
	}
	data := request()
	for key, want := range map[string]float64{"todayInvites": 1, "monthInvites": 1, "directUsers": 1, "teamUserCount": 3, "commissionAmount": 1000, "pendingCommission": 1000, "settleableCommission": 1000, "settledCommission": 0} {
		if data[key] != want {
			t.Errorf("%s=%v want %v", key, data[key], want)
		}
	}
	created := distributionRequest(s.adminDistributionSettlementCreate, "/", `{"agentId":10,"start":"2026-09-01","end":"2026-09-30"}`)
	if created.Code != 200 {
		t.Fatal(created.Body.String())
	}
	if got := request()["settleableCommission"]; got != float64(0) {
		t.Errorf("reserved amount remains available: %v", got)
	}
}

func TestDistributionPostgresAnalyticsCountsEachOrderOnce(t *testing.T) {
	db := distributionDatabase(t)
	tx, _ := db.Begin()
	if err := generateDistributionCommissionsTx(context.Background(), tx, 1, 400, 10000); err != nil {
		t.Fatal(err)
	}
	if err := tx.Commit(); err != nil {
		t.Fatal(err)
	}
	s := &Server{db: db}
	w := httptest.NewRecorder()
	s.adminDistributionAnalytics(w, httptest.NewRequest(http.MethodGet, "/", nil))
	if w.Code != 200 {
		t.Fatal(w.Body.String())
	}
	var out struct {
		Data struct {
			Summary distributionAnalyticsSummary     `json:"summary"`
			Trend   []distributionAnalyticsTrendItem `json:"trend"`
		} `json:"data"`
	}
	if err := json.Unmarshal(w.Body.Bytes(), &out); err != nil {
		t.Fatal(err)
	}
	if out.Data.Summary.TotalOrderAmount != 10000 || out.Data.Summary.TotalCommissionAmount != 3000 {
		t.Errorf("summary %+v", out.Data.Summary)
	}
	var sum int64
	for _, day := range out.Data.Trend {
		sum += day.OrderAmount
	}
	if sum != 10000 {
		t.Errorf("trend duplicates order: %d", sum)
	}
	start, end := distributionAnalyticsRange(nil)
	current, err := s.currentDistributionAgent(context.Background(), 100)
	if err != nil {
		t.Fatal(err)
	}
	scoped, err := s.scopedDistributionAnalyticsSummary(context.Background(), current, start, end)
	if err != nil {
		t.Fatal(err)
	}
	if scoped.TotalOrderAmount != 10000 {
		t.Errorf("scoped duplicates order: %d", scoped.TotalOrderAmount)
	}
	trend, err := s.scopedDistributionTrend(context.Background(), current, start, end)
	if err != nil {
		t.Fatal(err)
	}
	sum = 0
	for _, day := range trend {
		sum += day.OrderAmount
	}
	if sum != 10000 {
		t.Errorf("scoped trend duplicates order: %d", sum)
	}
}

func TestDistributionPostgresAgentScopeAndChildPermissions(t *testing.T) {
	db := distributionDatabase(t)
	if _, err := db.Exec(`INSERT INTO app_users VALUES(500),(600); INSERT INTO distribution_agents(id,app_user_id,agent_code,level,root_agent_id,agent_path) VALUES(50,500,'OTHER',1,50,'/50/'); INSERT INTO distribution_user_relations(app_user_id,direct_agent_id) VALUES(600,50)`); err != nil {
		t.Fatal(err)
	}
	s := &Server{db: db}
	call := func(userID int64, h http.HandlerFunc, body string) *httptest.ResponseRecorder {
		req := httptest.NewRequest(http.MethodPost, "/", strings.NewReader(body))
		req = req.WithContext(withUser(req.Context(), auth.UserInfo{ID: userID}))
		w := httptest.NewRecorder()
		h(w, req)
		return w
	}
	for _, test := range []struct {
		user int64
		body string
		want int
	}{
		{100, `{"appUserId":600}`, 403}, {300, `{"appUserId":400}`, 403}, {400, `{"appUserId":600}`, 403},
	} {
		if got := call(test.user, s.agentDistributionCreateChild, test.body); got.Code != test.want {
			t.Fatalf("user=%d status=%d %s", test.user, got.Code, got.Body.String())
		}
	}
	out := call(100, s.agentDistributionAgents, "")
	if out.Code != 200 || strings.Contains(out.Body.String(), `"agentCode":"OTHER"`) || !strings.Contains(out.Body.String(), `"agentCode":"A20"`) {
		t.Fatalf("scope %s", out.Body.String())
	}
	if _, err := db.Exec(`UPDATE distribution_agents SET status='paused' WHERE id=10`); err != nil {
		t.Fatal(err)
	}
	if got := call(100, s.agentDistributionAgents, ""); got.Code != 403 {
		t.Fatalf("paused agent status=%d", got.Code)
	}
}

func TestDistributionPostgresPostRegistrationBindIsDisabled(t *testing.T) {
	db := distributionDatabase(t)
	s := &Server{db: db}
	req := httptest.NewRequest(http.MethodPost, "/api/app/distribution/bind", strings.NewReader(`{"agentCode":"A20"}`))
	req = req.WithContext(contextWithAppUser(req.Context(), auth.UserInfo{ID: 100}))
	w := httptest.NewRecorder()
	s.appDistributionBind(w, req)
	if w.Code != 403 {
		t.Fatalf("post-registration binding status=%d %s", w.Code, w.Body.String())
	}
	var n int
	if err := db.QueryRow(`SELECT count(*) FROM distribution_user_relations WHERE app_user_id=100`).Scan(&n); err != nil || n != 0 {
		t.Fatalf("relation changed count=%d err=%v", n, err)
	}
}

func TestDistributionPostgresSMSInviteOnlyBindsNewAccounts(t *testing.T) {
	db := distributionDatabase(t)
	if _, err := db.Exec(`ALTER TABLE app_users ADD COLUMN phone text UNIQUE; CREATE SEQUENCE sms_fixture_user_id START 1000; ALTER TABLE app_users ALTER COLUMN id SET DEFAULT nextval('sms_fixture_user_id'); UPDATE app_users SET phone='existing' WHERE id=100`); err != nil {
		t.Fatal(err)
	}
	ctx := context.Background()
	if err := createSMSUserWithDistributionInvite(ctx, db, "existing", "A20"); err != nil {
		t.Fatal(err)
	}
	if err := createSMSUserWithDistributionInvite(ctx, db, "new", "A20"); err != nil {
		t.Fatal(err)
	}
	if err := createSMSUserWithDistributionInvite(ctx, db, "new", "A30"); err != nil {
		t.Fatal(err)
	}
	var existing, agent int64
	if err := db.QueryRow(`SELECT count(*) FROM distribution_user_relations WHERE app_user_id=100`).Scan(&existing); err != nil {
		t.Fatal(err)
	}
	if existing != 0 {
		t.Fatal("existing SMS login created relation")
	}
	if err := db.QueryRow(`SELECT r.direct_agent_id FROM distribution_user_relations r JOIN app_users u ON u.id=r.app_user_id WHERE u.phone='new'`).Scan(&agent); err != nil {
		t.Fatal(err)
	}
	if agent != 20 {
		t.Fatalf("first registration relation overwritten: %d", agent)
	}
	if err := createSMSUserWithDistributionInvite(ctx, db, "invalid", "NOT_FOUND"); err == nil {
		t.Fatal("invalid code accepted")
	}
}

func TestDistributionAgentCannotUseAdminPermissions(t *testing.T) {
	s := &Server{}
	user := auth.UserInfo{ID: 100, TokenKind: auth.TokenKindBackend, Roles: []string{"agent"}}
	for _, code := range []string{"Customer:App:Write", "Customer:App:List", "Customer:AppOrders:List", "System:User:Write"} {
		allowed, err := s.hasAnyPermission(context.Background(), user, code)
		if err != nil || allowed {
			t.Errorf("agent granted %s: %t %v", code, allowed, err)
		}
	}
	allowed, err := s.hasAnyPermission(context.Background(), user, "Agent:Distribution:View")
	if err != nil || !allowed {
		t.Fatal("agent lost own view permission")
	}
}

func TestDistributionPostgresPaidOrderAndRefundLifecycle(t *testing.T) {
	db := distributionDatabase(t)
	_, err := db.Exec(`ALTER TABLE app_users ADD COLUMN member_level text NOT NULL DEFAULT 'free', ADD COLUMN member_started_at timestamptz, ADD COLUMN member_expires_at timestamptz, ADD COLUMN update_time timestamptz;
 ALTER TABLE app_orders ADD COLUMN app_user_id bigint, ADD COLUMN product_id text, ADD COLUMN status text, ADD COLUMN duration_days int, ADD COLUMN activation_at timestamptz, ADD COLUMN membership_expires_at timestamptz, ADD COLUMN amount bigint, ADD COLUMN paid_at timestamptz, ADD COLUMN provider_trade_no text, ADD COLUMN provider_status text, ADD COLUMN transaction_id text, ADD COLUMN member_level_before text, ADD COLUMN member_started_at_before timestamptz, ADD COLUMN member_expires_at_before timestamptz, ADD COLUMN payment_error text, ADD COLUMN update_time timestamptz, ADD COLUMN refunded_at timestamptz, ADD COLUMN refund_reason text;
 UPDATE app_orders SET app_user_id=400,product_id='vip_month',status='pending',duration_days=30,amount=10000 WHERE id=1;`)
	if err != nil {
		t.Fatal(err)
	}
	ctx := context.Background()
	for i := 0; i < 2; i++ {
		tx, err := db.Begin()
		if err != nil {
			t.Fatal(err)
		}
		result, err := settleAppOrderTx(ctx, tx, appOrderSettlementInput{OrderID: 1})
		if err != nil {
			tx.Rollback()
			t.Fatal(err)
		}
		if err = tx.Commit(); err != nil {
			t.Fatal(err)
		}
		if result.AlreadyGranted != (i == 1) {
			t.Fatalf("payment replay=%t", result.AlreadyGranted)
		}
	}
	var count, amount int64
	if err = db.QueryRow(`SELECT count(*),sum(commission_amount) FROM distribution_commission_records WHERE status='pending'`).Scan(&count, &amount); err != nil {
		t.Fatal(err)
	}
	if count != 3 || amount != 3000 {
		t.Fatalf("paid commission count=%d amount=%d", count, amount)
	}
	for i := 0; i < 2; i++ {
		tx, err := db.Begin()
		if err != nil {
			t.Fatal(err)
		}
		result, err := refundAppOrderTx(ctx, tx, appOrderRefundInput{OrderID: 1, Reason: "isolated test refund"})
		if err != nil {
			tx.Rollback()
			t.Fatal(err)
		}
		if err = tx.Commit(); err != nil {
			t.Fatal(err)
		}
		if result.AlreadyRefunded != (i == 1) {
			t.Fatalf("refund replay=%t", result.AlreadyRefunded)
		}
	}
	if err = db.QueryRow(`SELECT count(*) FROM distribution_commission_records WHERE status='reversed'`).Scan(&count); err != nil {
		t.Fatal(err)
	}
	if count != 3 {
		t.Fatalf("refund reversed %d of 3 records", count)
	}
}
