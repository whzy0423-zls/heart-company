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

func TestConsentLinkSupportsMediaAssetWithoutCase(t *testing.T) {
	link := ConsentLink{
		ConsentID:    3,
		MediaAssetID: 9,
		SubjectType:  ConsentMediaAsset,
		SubjectID:    9,
		UseScope:     "public_playback",
	}
	if err := link.Validate(); err != nil {
		t.Fatalf("media-only consent link rejected: %v", err)
	}
	if link.CaseID != 0 {
		t.Fatalf("media-only consent link unexpectedly requires case %d", link.CaseID)
	}
}

func TestConsentLinkRequiresTypedSubjectAndScope(t *testing.T) {
	for _, link := range []ConsentLink{
		{ConsentID: 1, MediaAssetID: 2, SubjectID: 2, UseScope: "public_playback"},
		{ConsentID: 1, MediaAssetID: 2, SubjectType: ConsentMediaAsset, UseScope: "public_playback"},
		{ConsentID: 1, MediaAssetID: 2, SubjectType: ConsentMediaAsset, SubjectID: 2},
	} {
		if err := link.Validate(); !errors.Is(err, ErrInvalidConsentLink) {
			t.Fatalf("invalid link accepted: %+v, err=%v", link, err)
		}
	}
}
