package enterprisepromotion

import (
	"errors"
	"testing"
)

func TestTrainingCaseValidateForReview(t *testing.T) {
	valid := TrainingCase{Title: "Team workshop", Slug: "team-workshop", TrainerID: 42}
	if err := valid.ValidateForReview(); err != nil {
		t.Fatalf("valid case rejected: %v", err)
	}

	tests := []struct {
		name string
		in   TrainingCase
		want error
	}{
		{name: "title", in: TrainingCase{Slug: "case", TrainerID: 1}, want: ErrInvalidCase},
		{name: "slug", in: TrainingCase{Title: "Case", TrainerID: 1}, want: ErrInvalidCase},
		{name: "trainer", in: TrainingCase{Title: "Case", Slug: "case"}, want: ErrTrainerRequired},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if err := tt.in.ValidateForReview(); !errors.Is(err, tt.want) {
				t.Fatalf("got %v, want %v", err, tt.want)
			}
		})
	}
}

func TestEnterprisePromotionEnumsAcceptOnlyKnownValues(t *testing.T) {
	for _, status := range []CaseStatus{CaseDraft, CaseReview, CasePublished, CaseOffline} {
		if !status.Valid() {
			t.Errorf("case status %q should be valid", status)
		}
	}
	if CaseStatus("deleted").Valid() {
		t.Fatal("unknown case status accepted")
	}

	for _, key := range []TopicKey{TopicTeamCommunication, TopicLeadership, TopicCohesion, TopicCulture, TopicEmployeeGrowth} {
		if !key.Valid() {
			t.Errorf("topic key %q should be valid", key)
		}
	}
	if TopicKey("unreviewed").Valid() {
		t.Fatal("arbitrary topic key accepted")
	}
}

func TestEnterpriseSolutionValidateForReview(t *testing.T) {
	if err := (EnterpriseSolution{Title: "Leadership", Slug: "leadership", TrainerID: 7}).ValidateForReview(); err != nil {
		t.Fatalf("valid solution rejected: %v", err)
	}
	if err := (EnterpriseSolution{Title: "Leadership", Slug: "leadership"}).ValidateForReview(); !errors.Is(err, ErrTrainerRequired) {
		t.Fatalf("got %v, want trainer required", err)
	}
}
