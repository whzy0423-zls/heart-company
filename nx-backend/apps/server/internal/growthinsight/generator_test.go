package growthinsight

import (
	"context"
	"encoding/json"
	"errors"
	"strings"
	"testing"
	"time"
)

func TestGeneratorRejectsUntrustedShapeAndNormalizesSections(t *testing.T) {
	now := time.Date(2026, 10, 9, 4, 0, 0, 0, time.UTC)
	sources := sourceSet{Evidence: []Evidence{{ID: "message:1", Kind: "message", Text: "I took a pause", OccurredAt: now.Add(-time.Hour)}}}
	for _, raw := range []string{`{"summary":{"text":"claim","evidenceRefs":["invented"]}}`, `{"summary":{"text":"claim","evidenceRefs":["message:1"]},"secret":"extra"}`, `{"summary":{"text":"claim","evidenceRefs":["message:1"]}} {}`, strings.Repeat("x", 100001)} {
		_, err := generate(context.Background(), func(context.Context, string) (string, error) { return raw, nil }, sources, now)
		if !errors.Is(err, ErrInvalid) {
			t.Fatalf("accepted invalid model output: %v", err)
		}
	}
	a, err := generate(context.Background(), func(context.Context, string) (string, error) {
		return `{"summary":{"text":"claim","evidenceRefs":["message:1"]}}`, nil
	}, sources, now)
	if err != nil {
		t.Fatal(err)
	}
	b, _ := json.Marshal(a)
	if strings.Contains(string(b), "null") {
		t.Fatalf("sections must be arrays: %s", b)
	}
}

func TestClassifiedProjectionUsesOnlyExplicitEvidenceClaims(t *testing.T) {
	now := time.Now().UTC()
	a := validAnalysis()
	a.Strengths = []Claim{{Text: "An explicitly stated strength", EvidenceRefs: []string{"message:1"}}}
	a.StressPoints = []Claim{{Text: "An explicitly stated pressure", EvidenceRefs: []string{"message:1"}}}
	a.AwarenessPrompts = []Claim{{Text: "What did you notice?", EvidenceRefs: []string{"message:1"}}}
	if err := validateAnalysis(a, []Evidence{{ID: "message:1", OccurredAt: now}}, now.Add(-time.Hour)); err != nil {
		t.Fatal(err)
	}
	v := project(Report{Analysis: a}, nil)
	if len(v.Strengths) != 1 || len(v.StressPoints) != 1 || len(v.AwarenessPrompts) != 1 {
		t.Fatalf("missing explicit projection: %+v", v)
	}
	a.Strengths[0].EvidenceRefs = []string{"invented"}
	if validateAnalysis(a, []Evidence{{ID: "message:1", OccurredAt: now}}, now.Add(-time.Hour)) == nil {
		t.Fatal("unchecked projection evidence")
	}
}

func TestWeeklyPeriodEndsAtActualSourceCutoff(t *testing.T) {
	start, _ := weeklyPeriod(time.Date(2026, 10, 9, 4, 0, 0, 0, time.UTC))
	through := time.Date(2026, 10, 9, 3, 0, 0, 0, time.UTC)
	if got := periodEnd(start, through); !got.Equal(through) {
		t.Fatalf("period end %s, want %s", got, through)
	}
	if got := periodEnd(start, start.Add(-time.Hour)); !got.Equal(start) {
		t.Fatalf("pre-week evidence moved period start: %s", got)
	}
}
