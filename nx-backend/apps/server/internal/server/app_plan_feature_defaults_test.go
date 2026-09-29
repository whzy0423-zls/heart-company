package server

import "testing"

func TestMissingVIPFeatureFlagsUseCurrentTierDefaults(t *testing.T) {
	for _, level := range []string{"vip", "vip_month", "vip_quarter", "vip_year"} {
		t.Run(level, func(t *testing.T) {
			flags := membershipFeatureFlags(level, map[string]bool{"deepChat": true, "custom": true})
			for _, key := range []string{"growthPortrait", "trendAnalysis", "relationshipInsight", "xinzhili"} {
				if !flags[key] {
					t.Errorf("%s missing %s must use current VIP default: %v", level, key, flags)
				}
			}
			if flags["prioritySupport"] || flags["problemFollowup"] {
				t.Fatalf("VIP gained an SVIP-only feature: %v", flags)
			}
			if !flags["deepChat"] || !flags["custom"] {
				t.Fatalf("unrelated explicit flag changed: %v", flags)
			}
		})
	}
}

func TestTierFeatureNormalizationPreservesExplicitConfiguration(t *testing.T) {
	for _, level := range []string{"vip", "svip"} {
		for _, key := range []string{"growthPortrait", "trendAnalysis", "relationshipInsight", "xinzhili", "prioritySupport"} {
			t.Run(level+"/"+key, func(t *testing.T) {
				base := map[string]bool{key: false, "custom": true}
				flags := membershipFeatureFlags(level, base)
				if flags[key] || !flags["custom"] {
					t.Fatalf("explicit settings changed: %v", flags)
				}
				if len(base) != 2 {
					t.Fatalf("source config mutated: %v", base)
				}
			})
		}
	}
	stale := map[string]bool{"growthPortrait": true, "trendAnalysis": true, "relationshipInsight": true, "xinzhili": true, "prioritySupport": true, "problemFollowup": true}
	for key, enabled := range membershipFeatureFlags("free", stale) {
		if enabled {
			t.Errorf("free stale flag %s remained enabled", key)
		}
	}
}

func TestLegacyVIPPlanWithoutAdditiveFlagsRetainsCurrentCapabilities(t *testing.T) {
	for _, code := range []string{"vip_month", "vip_quarter", "vip_year"} {
		plan := appPlanConfig{Code: code, DeepChatEnabled: true, CompanionEnabled: true, MemberPosterEnabled: true, PriceCents: 7777}
		got := normalizeLoadedAppPlan(plan)
		if !got.FeatureFlags["growthPortrait"] || !got.FeatureFlags["trendAnalysis"] || !got.FeatureFlags["relationshipInsight"] || !got.FeatureFlags["xinzhili"] {
			t.Fatalf("legacy %s lost current VIP capability defaults: %v", code, got.FeatureFlags)
		}
		if got.FeatureFlags["prioritySupport"] || got.FeatureFlags["problemFollowup"] || got.PriceCents != 7777 || !got.FeatureFlags["deepChat"] || !got.FeatureFlags["companion"] || !got.FeatureFlags["memberPoster"] {
			t.Fatalf("legacy %s changed independent config: %+v", code, got)
		}
	}
}
