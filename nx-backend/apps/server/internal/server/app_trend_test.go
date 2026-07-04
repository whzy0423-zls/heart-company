package server

import (
	"testing"
	"time"
)

func TestBuildAppTrendSeriesUsesRealDailySignals(t *testing.T) {
	start := time.Date(2026, 7, 1, 0, 0, 0, 0, time.UTC)
	end := start.AddDate(0, 0, 2)
	signals := map[string]appTrendDaySignals{
		"2026-07-01": {
			UserMessages: 2,
			StressHits:   3,
		},
		"2026-07-02": {
			UserMessages:      1,
			AssistantMessages: 1,
			UserChars:         200,
			EnergyHits:        2,
			RelationshipHits:  2,
			AwarenessHits:     1,
			FavoriteHelpful:   1,
			Checkins:          1,
			Memories:          1,
		},
	}

	series := buildAppTrendSeries(start, end, signals)

	if got := appTrendValue(t, series, "stress", "2026-07-01"); got != 74 {
		t.Fatalf("expected high stress day to score 74, got %.1f", got)
	}
	if got := appTrendValue(t, series, "energy", "2026-07-01"); got != 46 {
		t.Fatalf("expected stress-heavy day to lower energy to 46, got %.1f", got)
	}
	if got := appTrendValue(t, series, "relationship", "2026-07-02"); got != 72 {
		t.Fatalf("expected relationship signals to score 72, got %.1f", got)
	}
	if got := appTrendValue(t, series, "awareness", "2026-07-02"); got != 77 {
		t.Fatalf("expected awareness signals to score 77, got %.1f", got)
	}
}

func TestBuildAppTrendSeriesUsesNeutralBaselineWithoutSignals(t *testing.T) {
	start := time.Date(2026, 7, 1, 0, 0, 0, 0, time.UTC)
	end := start.AddDate(0, 0, 3)

	series := buildAppTrendSeries(start, end, nil)

	for _, item := range []struct {
		dimension string
		want      float64
	}{
		{"stress", 48},
		{"energy", 54},
		{"relationship", 52},
		{"awareness", 50},
	} {
		if got := appTrendValue(t, series, item.dimension, "2026-07-02"); got != item.want {
			t.Fatalf("expected neutral %s baseline %.1f, got %.1f", item.dimension, item.want, got)
		}
	}
}

func appTrendValue(t *testing.T, series []appTrendSeries, dimension, date string) float64 {
	t.Helper()
	for _, item := range series {
		if item.Dimension != dimension {
			continue
		}
		for _, point := range item.Points {
			if point.Date == date {
				return point.Value
			}
		}
	}
	t.Fatalf("missing trend point dimension=%s date=%s", dimension, date)
	return 0
}
