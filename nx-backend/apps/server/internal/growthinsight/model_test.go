package growthinsight

import (
	"encoding/json"
	"strings"
	"testing"
	"time"
)

func validAnalysis() Analysis {
	c := Claim{Text: "You described taking a pause.", EvidenceRefs: []string{"message:1"}}
	return Analysis{Summary: c, WeeklyReview: c, TrendExplanation: c, Observations: []Claim{c}, Recommendations: []Claim{c}, Actions: []SuggestedAction{{Title: "Pause", Detail: "Take a short pause before replying.", EvidenceRefs: c.EvidenceRefs}}}
}

func TestValidateAnalysisReferencesAndBounds(t *testing.T) {
	now := time.Date(2026, 10, 9, 1, 0, 0, 0, time.UTC)
	evidence := []Evidence{{ID: "message:1", Kind: "message", Text: "I paused", OccurredAt: now}}
	if err := validateAnalysis(validAnalysis(), evidence, now.Add(-24*time.Hour)); err != nil {
		t.Fatal(err)
	}
	for _, tc := range []struct {
		name   string
		mutate func(*Analysis)
	}{
		{"invented evidence", func(a *Analysis) { a.Summary.EvidenceRefs = []string{"message:999"} }},
		{"unreferenced claim", func(a *Analysis) { a.Summary.EvidenceRefs = nil }},
		{"oversize claim", func(a *Analysis) { a.Summary.Text = strings.Repeat("x", 2001) }},
		{"too many actions", func(a *Analysis) { a.Actions = append(a.Actions, a.Actions[0], a.Actions[0], a.Actions[0]) }},
		{"too many section items", func(a *Analysis) {
			for i := 0; i < 12; i++ {
				a.Patterns = append(a.Patterns, a.Summary)
			}
		}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			a := validAnalysis()
			tc.mutate(&a)
			if validateAnalysis(a, evidence, now.Add(-24*time.Hour)) == nil {
				t.Fatal("accepted invalid analysis")
			}
		})
	}
	if validateAnalysis(validAnalysis(), evidence, now.Add(time.Hour)) == nil {
		t.Fatal("accepted evidence outside weekly period")
	}
}

func TestUserProjectionNeverSerializesEvidenceOrUncertainty(t *testing.T) {
	r := Report{ID: 7, CardID: 3, Version: 2, Analysis: validAnalysis(), Evidence: []Evidence{{ID: "private-source", Text: "private raw text"}}}
	r.Analysis.Uncertainties = []Claim{{Text: "internal uncertainty", EvidenceRefs: []string{"private-source"}}}
	b, err := json.Marshal(project(r, nil))
	if err != nil {
		t.Fatal(err)
	}
	for _, forbidden := range []string{"private-source", "private raw text", "internal uncertainty", "evidenceRefs", "uncertainty"} {
		if strings.Contains(string(b), forbidden) {
			t.Fatalf("projection leaked %s: %s", forbidden, b)
		}
	}
	if !strings.Contains(string(b), `"reportId":7`) {
		t.Fatal(string(b))
	}
}

func TestFeedbackUnicodeLimitAndEnums(t *testing.T) {
	if validateFeedback(Feedback{Status: "attempted", Outcome: "helpful", Note: strings.Repeat("好", 500)}) != nil {
		t.Fatal("500 Unicode characters must fit")
	}
	for _, v := range []Feedback{{Status: "done", Outcome: "unknown"}, {Status: "attempted", Outcome: "yes"}, {Status: "skipped", Outcome: "helpful"}, {Status: "skipped", Outcome: "unknown", Note: strings.Repeat("好", 501)}} {
		if validateFeedback(v) == nil {
			t.Fatal("accepted invalid feedback")
		}
	}
}

func TestShanghaiWeeklyPeriod(t *testing.T) {
	start, end := weeklyPeriod(time.Date(2026, 10, 11, 17, 0, 0, 0, time.UTC))
	if start.Format(time.RFC3339) != "2026-10-11T16:00:00Z" || !end.Equal(start.Add(7*24*time.Hour)) {
		t.Fatalf("%s %s", start, end)
	}
}
