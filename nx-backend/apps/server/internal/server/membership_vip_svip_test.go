package server

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"testing"
	"time"
)

func TestMembershipPlanNormalizesLegacySKUAndExpiry(t *testing.T) {
	now := time.Date(2026, 9, 21, 10, 0, 0, 0, time.UTC)
	future := now.Add(24 * time.Hour)
	past := now.Add(-time.Hour)
	tests := []struct {
		name       string
		level      string
		sku        string
		expires    *time.Time
		wantLevel  string
		wantCycle  string
		wantActive bool
	}{
		{name: "free", level: "free", wantLevel: "free", wantCycle: "none", wantActive: true},
		{name: "legacy vip month", level: "vip_month", wantLevel: "vip", wantCycle: "month", expires: &future, wantActive: true},
		{name: "legacy vip quarter", level: "vip_quarter", wantLevel: "vip", wantCycle: "quarter", expires: &future, wantActive: true},
		{name: "legacy vip year", level: "vip_year", wantLevel: "vip", wantCycle: "year", expires: &future, wantActive: true},
		{name: "legacy svip without expiry", level: "svip", wantLevel: "svip", wantCycle: "year", wantActive: true},
		{name: "expired membership", level: "vip_month", wantLevel: "free", wantCycle: "none", expires: &past, wantActive: false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := resolveMembershipPlan(tt.level, tt.sku, tt.expires, now)
			if got.PlanLevel != tt.wantLevel || got.BillingCycle != tt.wantCycle || got.Active != tt.wantActive {
				t.Fatalf("resolveMembershipPlan() = %+v, want level=%q cycle=%q active=%v", got, tt.wantLevel, tt.wantCycle, tt.wantActive)
			}
		})
	}
}

func TestDefaultMembershipLevelQuotas(t *testing.T) {
	wants := map[string]int{"free": 1, "vip": 3, "svip": 10}
	for level, wantCards := range wants {
		got := defaultMembershipLevelPlan(level)
		if got.PlanLevel != level || got.CardLimit != wantCards {
			t.Fatalf("defaultMembershipLevelPlan(%q) = %+v, want card limit %d", level, got, wantCards)
		}
	}
}

func TestLegacyBillingCyclesUseCanonicalMembershipCardLimits(t *testing.T) {
	for _, code := range []string{"vip_month", "vip_quarter", "vip_year", "vip"} {
		plan := normalizeLoadedAppPlan(defaultAppPlan(code))
		if plan.PlanLevel != "vip" || plan.CardLimit != 3 || plan.Limits["cardLimit"] != 3 {
			t.Fatalf("defaultAppPlan(%q) = level=%q cardLimit=%d limits=%v, want VIP/3", code, plan.PlanLevel, plan.CardLimit, plan.Limits)
		}
	}
	plan := normalizeLoadedAppPlan(defaultAppPlan("svip"))
	if plan.PlanLevel != "svip" || plan.CardLimit != 10 || plan.Limits["cardLimit"] != 10 {
		t.Fatalf("defaultAppPlan(svip) = level=%q cardLimit=%d limits=%v, want SVIP/10", plan.PlanLevel, plan.CardLimit, plan.Limits)
	}
}

func TestHistoricalResourceMetadataKeepsContentReadableAfterDowngrade(t *testing.T) {
	readOnly := membershipResourceMetadataForPlan("free", "vip", "历史故事")
	if readOnly.State != resourceAccessReadOnlyOverLimit || !readOnly.UpgradeRequired || readOnly.RequiredPlanLevel != "vip" {
		t.Fatalf("free metadata = %+v, want retained read-only VIP gate", readOnly)
	}
	active := membershipResourceMetadataForPlan("svip", "vip", "")
	if active.State != resourceAccessActive || active.UpgradeRequired {
		t.Fatalf("svip metadata = %+v, want active", active)
	}
}

func TestRecomputeResourceAccessStatesKeepsOldestCardsWritable(t *testing.T) {
	now := time.Date(2026, 9, 21, 10, 0, 0, 0, time.UTC)
	items := []resourceAccessCandidate{
		{ResourceID: 40, CreatedAt: now.Add(4 * time.Hour)},
		{ResourceID: 10, CreatedAt: now.Add(1 * time.Hour)},
		{ResourceID: 20, CreatedAt: now.Add(2 * time.Hour)},
		{ResourceID: 30, CreatedAt: now.Add(3 * time.Hour)},
	}
	got := recomputeResourceAccessStates(items, "vip", now)
	if len(got) != 4 {
		t.Fatalf("got %d states, want 4", len(got))
	}
	if got[0].ResourceID != 10 || got[0].State != resourceAccessActive || got[0].PriorityRank != 1 {
		t.Fatalf("oldest card should be active rank 1: %+v", got[0])
	}
	if got[2].ResourceID != 30 || got[2].State != resourceAccessActive {
		t.Fatalf("third card should be active for vip: %+v", got[2])
	}
	if got[3].ResourceID != 40 || got[3].State != resourceAccessReadOnlyOverLimit || got[3].RequiredPlanLevel != "svip" || got[3].RetentionUntil == nil {
		t.Fatalf("overflow card should be retained read-only: %+v", got[3])
	}
}

func TestAppEntitlementJSONIncludesNewMembershipFieldsAndLegacyFields(t *testing.T) {
	body, err := json.Marshal(appEntitlementResp{
		PlanName:       "VIP",
		PlanCode:       "vip_month",
		PlanLevel:      "vip",
		BillingCycle:   "month",
		Features:       []string{"deep_chat"},
		Quotas:         map[string]any{"cardLimit": 3},
		ResourceAccess: map[string][]appResourceAccessResp{"cards": {{ResourceID: 8, State: resourceAccessReadOnlyOverLimit}}},
		DowngradedAt:   "2026-09-21T10:00:00Z",
		RetentionUntil: "2026-10-21T10:00:00Z",
		CardLimit:      5,
	})
	if err != nil {
		t.Fatal(err)
	}
	for _, key := range []string{"planLevel", "billingCycle", "features", "quotas", "resourceAccess", "downgradedAt", "retentionUntil", "cardLimit"} {
		if !strings.Contains(string(body), `"`+key+`"`) {
			t.Fatalf("entitlement JSON missing %q: %s", key, body)
		}
	}
}

func TestMembershipSchemaContainsPlanAndResourceAccessMigration(t *testing.T) {
	raw, err := os.ReadFile("../db/schema.sql")
	if err != nil {
		t.Fatal(err)
	}
	schema := string(raw)
	for _, fragment := range []string{
		"plan_level",
		"billing_cycle",
		"feature_flags",
		"limits JSONB",
		"CREATE TABLE IF NOT EXISTS app_membership_resource_access",
		"required_plan_level",
		"priority_rank",
		"retention_until",
		"('vip_year','vip','year'",
		"('svip','svip','year'",
	} {
		if !strings.Contains(schema, fragment) {
			t.Fatalf("schema missing %q", fragment)
		}
	}
	dropIndex := strings.Index(schema, "ALTER TABLE app_plans DROP CONSTRAINT IF EXISTS app_plans_code_check")
	svipSeedIndex := strings.Index(schema, "('svip','svip','year'")
	if dropIndex < 0 || svipSeedIndex < 0 || svipSeedIndex < dropIndex {
		t.Fatalf("SVIP seed must run after legacy plan constraints are replaced (drop=%d seed=%d)", dropIndex, svipSeedIndex)
	}
}

func TestMembershipUpgradeRequiredResponse(t *testing.T) {
	response := httptest.NewRecorder()
	writeMembershipUpgradeRequired(response, "vip", resourceAccessReadOnlyOverLimit)
	if response.Code != http.StatusForbidden {
		t.Fatalf("status = %d, want 403", response.Code)
	}
	var body map[string]any
	if err := json.Unmarshal(response.Body.Bytes(), &body); err != nil {
		t.Fatal(err)
	}
	if body["code"] != "membership_upgrade_required" || body["requiredPlanLevel"] != "vip" || body["accessState"] != resourceAccessReadOnlyOverLimit {
		t.Fatalf("unexpected upgrade response: %#v", body)
	}
}
