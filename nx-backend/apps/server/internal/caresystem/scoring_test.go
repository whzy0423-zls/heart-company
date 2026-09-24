package caresystem

import "testing"

func TestScoreReturnsInsufficientForTooLittleEvidence(t *testing.T) {
	result := Score([]Evidence{{Content: "最近有点压力"}}, Baseline{})
	if result.DataStatus != DataStatusInsufficient || result.Level != nil {
		t.Fatalf("unexpected insufficient result: %+v", result)
	}
}

func TestScoreBoundsHighDistressAndCapsWithoutLossOfControl(t *testing.T) {
	evidence := make([]Evidence, 0, 12)
	for i := 0; i < 12; i++ {
		evidence = append(evidence, Evidence{Content: "压力 焦虑 崩溃 绝望"})
	}
	result := Score(evidence, Baseline{})
	if result.Level == nil || *result.Level != 8 {
		t.Fatalf("level = %v, want capped level 8", result.Level)
	}
	if result.Label != "持续关怀" {
		t.Fatalf("label = %q", result.Label)
	}
}

func TestScoreAllowsLevelTenOnlyWithLossOfControlSignals(t *testing.T) {
	evidence := []Evidence{{Content: "我控制不住自己，活不下去"}, {Content: "失控，不想活"}}
	result := Score(evidence, Baseline{})
	if result.Level == nil || *result.Level != 10 {
		t.Fatalf("level = %v, want 10", result.Level)
	}
}

func TestTrendUsesHistoricalBaseline(t *testing.T) {
	baseline := 4
	if got := TrendFor(7, &baseline); got != TrendUp {
		t.Fatalf("trend = %q, want up", got)
	}
	if got := TrendFor(2, &baseline); got != TrendDown {
		t.Fatalf("trend = %q, want down", got)
	}
}
