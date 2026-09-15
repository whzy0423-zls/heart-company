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
