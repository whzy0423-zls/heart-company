package server

import (
	"context"
	"database/sql"
	"database/sql/driver"
	"encoding/json"
	"errors"
	"net/http"
	"strings"
	"sync"
	"testing"
)

func TestProblemFollowupPlanFlags(t *testing.T) {
	for _, level := range []string{"free", "vip", "vip_year", "svip", "svip_month", "svip_quarter", "svip_year"} {
		for _, flag := range []string{"missing", "enabled", "disabled"} {
			t.Run(level+"/"+flag, func(t *testing.T) {
				base := map[string]bool{"unrelated": true}
				if flag != "missing" {
					base["problemFollowup"] = flag == "enabled"
				}
				got := membershipFeatureFlags(level, base)
				want := normalizeMembershipLevel(level) == "svip" && flag != "disabled"
				if value, exists := got["problemFollowup"]; !exists || value != want {
					t.Fatalf("flags = %v, want problemFollowup=%v", got, want)
				}
				if !got["unrelated"] {
					t.Fatal("unrelated feature was changed")
				}
			})
		}
	}
	for _, plan := range append(defaultAppPlans(), defaultMembershipLevelPlan("svip"), defaultMembershipLevelPlan("vip"), defaultMembershipLevelPlan("free")) {
		want := plan.PlanLevel == "svip"
		if flag, exists := plan.FeatureFlags["problemFollowup"]; !exists || flag != want {
			t.Errorf("default %s missing tier-specific flag: %v", plan.Code, plan.FeatureFlags)
		}
		if strings.Contains(strings.Join(plan.Features, " "), "问题解决跟进") != want {
			t.Errorf("default %s has wrong followup copy: %v", plan.Code, plan.Features)
		}
	}
}

func TestProblemFollowupEntitlementMatrix(t *testing.T) {
	for _, tc := range []struct {
		level, flag string
		want        bool
	}{
		{"active:free", "true", false}, {"active:vip", "true", false}, {"active:vip_year", "true", false},
		{"active:svip", "missing", true}, {"svip", "missing", true},
		{"active:svip", "no_row", true},
		{"active:svip_month", "true", true}, {"active:svip_quarter", "true", true}, {"active:svip_year", "true", true},
		{"active:svip", "false", false}, {"active:svip_month", "false", false}, {"active:svip_quarter", "false", false}, {"active:svip_year", "false", false},
		{"expired:svip", "true", false}, {"expired:svip_month", "true", false}, {"svip_month", "true", false},
		{"active:svip", "error", false}, {"active:svip", "invalid", false},
	} {
		t.Run(tc.level+"/"+tc.flag, func(t *testing.T) {
			s := newProblemFollowupEntitlementServer(t, tc.level, tc.flag)
			response := performAppBillingRequest(t, s.appBillingEntitlements, http.MethodGet, "/api/app/billing/entitlements", nil)
			if response.Code != 200 {
				t.Fatalf("status=%d body=%s", response.Code, response.Body.String())
			}
			var body struct {
				Data appEntitlementResp `json:"data"`
			}
			if err := json.Unmarshal(response.Body.Bytes(), &body); err != nil {
				t.Fatal(err)
			}
			if value, exists := body.Data.FeatureFlags["problemFollowup"]; !exists || value != tc.want {
				t.Fatalf("flags=%v want problemFollowup=%v", body.Data.FeatureFlags, tc.want)
			}
			if strings.Contains(strings.Join(body.Data.Features, " "), "问题解决跟进") != tc.want {
				t.Fatalf("misleading entitlement copy: %v", body.Data.Features)
			}
			if body.Data.PlanLevel == "svip" && (!body.Data.FeatureFlags["deepChat"] || body.Data.CardLimit != 10) {
				t.Fatal("unrelated canonical SVIP capabilities changed")
			}
		})
	}
}

func TestProblemFollowupCanonicalSwitchAppliesToEverySKU(t *testing.T) {
	for _, enabled := range []bool{false, true} {
		plans := append(defaultAppPlans(), defaultMembershipLevelPlan("svip"))
		plans[len(plans)-1].FeatureFlags["problemFollowup"] = enabled
		// A contradictory SKU flag is not an independent tier policy.
		plans[4].FeatureFlags["problemFollowup"] = !enabled
		got := applyProblemFollowupPlanPolicy(plans)
		for _, plan := range got {
			want := plan.PlanLevel == "svip" && enabled
			if plan.FeatureFlags["problemFollowup"] != want {
				t.Fatalf("%s has inconsistent switch %v", plan.Code, plan.FeatureFlags)
			}
			if strings.Contains(strings.Join(plan.Features, " "), "问题解决跟进") != want {
				t.Fatalf("%s has inconsistent copy %v", plan.Code, plan.Features)
			}
		}
	}
}

func TestProblemFollowupCopyKeepsConfiguredFeaturesWithinLimit(t *testing.T) {
	features := []string{"一", "二", "三", "四", "五", "六", "七", "八"}
	got := normalizeProblemFollowupFeatureCopy(features, true)
	if strings.Join(got, ",") != strings.Join(features, ",") {
		t.Fatalf("existing eight custom features must be preserved: %v", got)
	}
}

func TestProblemFollowupCatalogFailsClosedOnCapabilityErrors(t *testing.T) {
	for _, flag := range []string{"missing", "true", "false", "invalid", "null", "malformed", "error"} {
		t.Run(flag, func(t *testing.T) {
			s := newProblemFollowupEntitlementServer(t, "catalog:svip", flag)
			plans, err := s.loadAppPlans(context.Background())
			if err != nil {
				t.Fatal(err)
			}
			for _, plan := range plans {
				want := plan.PlanLevel == "svip" && (flag == "missing" || flag == "true")
				if plan.FeatureFlags["problemFollowup"] != want || strings.Contains(strings.Join(plan.Features, " "), "问题解决跟进") != want {
					t.Fatalf("catalog %s inconsistent: flags=%v copy=%v want=%v", plan.Code, plan.FeatureFlags, plan.Features, want)
				}
				if !plan.DeepChatEnabled || !plan.FeatureFlags["deepChat"] || plan.PriceCents != 49900 {
					t.Fatalf("legacy capability changed: %+v", plan)
				}
			}
		})
	}
}

func TestProblemFollowupCatalogFallbackDoesNotAdvertiseUnverifiedFeature(t *testing.T) {
	s := newProblemFollowupEntitlementServer(t, "catalog:svip", "catalog_error")
	response := performAppBillingRequest(t, s.appBillingProducts, http.MethodGet, "/api/app/billing/products", nil)
	if response.Code != 200 {
		t.Fatalf("status=%d body=%s", response.Code, response.Body.String())
	}
	var body struct {
		Data []appProductResp `json:"data"`
	}
	if err := json.Unmarshal(response.Body.Bytes(), &body); err != nil {
		t.Fatal(err)
	}
	for _, product := range body.Data {
		if strings.Contains(strings.Join(product.Features, " "), "问题解决跟进") {
			t.Fatalf("fallback advertises unverified feature: %+v", product)
		}
	}
}

var problemFollowupEntitlementDriverOnce sync.Once

type problemFollowupEntitlementDriver struct{}
type problemFollowupEntitlementConn struct {
	*appBillingTestConn
	flag string
}

func (problemFollowupEntitlementDriver) Open(dsn string) (driver.Conn, error) {
	parts := strings.SplitN(dsn, "|", 2)
	return &problemFollowupEntitlementConn{appBillingTestConn: &appBillingTestConn{memberLevel: parts[0]}, flag: parts[1]}, nil
}

func (c *problemFollowupEntitlementConn) QueryContext(ctx context.Context, query string, args []driver.NamedValue) (driver.Rows, error) {
	if strings.HasPrefix(c.memberLevel, "catalog:") && strings.Contains(query, "FROM app_plans ORDER BY") {
		if c.flag == "catalog_error" {
			return nil, errors.New("catalog fixture offline")
		}
		values := make([][]driver.Value, 0, 2)
		for _, code := range []string{"svip", "svip_year"} {
			values = append(values, []driver.Value{code, code, "", int64(49900), int64(0), "", []byte(`["深度对话"]`), true, int64(1), int64(365), int64(-1), int64(12), int64(10), true, true, true})
		}
		return &appBillingTestRows{columns: strings.Split(appPlanColumns, ","), values: values}, nil
	}
	if strings.HasPrefix(c.memberLevel, "catalog:") && strings.Contains(query, "SELECT plan_level,billing_cycle,feature_flags,limits") {
		if c.flag == "error" {
			return nil, errors.New("capability fixture offline")
		}
		raw := `{"deepChat":true}`
		if c.flag == "invalid" {
			raw = `{"deepChat":true,"problemFollowup":"yes"}`
		} else if c.flag == "malformed" {
			raw = `{"deepChat":true,`
		} else if c.flag != "missing" {
			raw = `{"deepChat":true,"problemFollowup":` + c.flag + `}`
		}
		return &appBillingTestRows{columns: []string{"plan_level", "billing_cycle", "feature_flags", "limits"}, values: [][]driver.Value{{"svip", "year", []byte(raw), []byte(`{}`)}}}, nil
	}
	if strings.Contains(query, "SELECT feature_flags FROM app_plans WHERE code='svip'") {
		if c.flag == "no_row" {
			return &appBillingTestRows{columns: []string{"feature_flags"}}, nil
		}
		if c.flag == "error" {
			return nil, errors.New("fixture database offline")
		}
		raw := `{}`
		if c.flag == "invalid" {
			raw = `{"problemFollowup":"yes"}`
		} else if c.flag != "missing" {
			raw = `{"problemFollowup":` + c.flag + `}`
		}
		return &appBillingTestRows{columns: []string{"feature_flags"}, values: [][]driver.Value{{[]byte(raw)}}}, nil
	}
	return c.appBillingTestConn.QueryContext(ctx, query, args)
}

func newProblemFollowupEntitlementServer(t *testing.T, level, flag string) *Server {
	t.Helper()
	problemFollowupEntitlementDriverOnce.Do(func() { sql.Register("problem_followup_entitlement_test", problemFollowupEntitlementDriver{}) })
	db, err := sql.Open("problem_followup_entitlement_test", level+"|"+flag)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = db.Close() })
	return &Server{db: db}
}
