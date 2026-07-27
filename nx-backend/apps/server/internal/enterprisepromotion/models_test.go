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

func TestConsentLinkTypedSubjectIntegrity(t *testing.T) {
	tests := []ConsentLink{
		{ConsentID: 1, MediaAssetID: 9, SubjectType: ConsentMediaAsset, SubjectID: 8, UseScope: "public_playback"},
		{ConsentID: 1, TestimonialID: 7, SubjectType: ConsentTestimonial, SubjectID: 6, UseScope: "public_quote"},
	}
	for _, link := range tests {
		if err := link.Validate(); !errors.Is(err, ErrInvalidConsentLink) {
			t.Fatalf("typed subject mismatch accepted: %+v err=%v", link, err)
		}
	}

	link := ConsentLink{ConsentID: 3, MediaAssetID: 9, SubjectType: ConsentMediaAsset, SubjectID: 9, UseScope: "public_playback"}
	if err := link.ValidateForConsent(PublicationConsent{ID: 3, SubjectType: ConsentPerson}); !errors.Is(err, ErrInvalidConsentLink) {
		t.Fatalf("consent/link subject type mismatch accepted: %v", err)
	}
}

func TestCaseMediaStatusMatchesPersistenceStates(t *testing.T) {
	for _, status := range []CaseMediaStatus{CaseMediaDraft, CaseMediaPublished, CaseMediaOffline} {
		if !status.Valid() {
			t.Errorf("case media status %q rejected", status)
		}
	}
	if CaseMediaStatus("review").Valid() {
		t.Fatal("case review state leaked into case media status")
	}
	media := CaseMedia{Status: CaseMediaDraft}
	if media.Status != CaseMediaDraft {
		t.Fatal("case media does not use CaseMediaStatus")
	}
}

func TestPromotionMediaStateIncludesQAPending(t *testing.T) {
	if !MediaQAPending.Valid() {
		t.Fatal("qa_pending media state rejected")
	}
	if PromotionMediaState("published").Valid() {
		t.Fatal("content publication state accepted as media processing state")
	}
}

func TestPersistenceModelsCarryEnterprisePromotionContent(t *testing.T) {
	caseRecord := TrainingCase{
		BusinessChallenges: []string{"communication"}, TrainingGoals: []string{"alignment"},
		TrainingModules: []string{"workshop"}, TrainingMethods: []string{"role-play"},
	}
	if len(caseRecord.BusinessChallenges)+len(caseRecord.TrainingGoals)+len(caseRecord.TrainingModules)+len(caseRecord.TrainingMethods) != 4 {
		t.Fatal("case persistence content fields lost")
	}
	solution := EnterpriseSolution{
		Audiences: []string{"managers"}, Problems: []string{"conflict"}, Goals: []string{"cohesion"},
		Modules: []string{"practice"}, DeliveryMethods: []string{"onsite"}, RecommendedParticipants: "20-30",
		RecommendedDuration: "2 days", CustomizableItems: []string{"industry cases"},
	}
	if len(solution.Audiences)+len(solution.Problems)+len(solution.Goals)+len(solution.Modules)+len(solution.DeliveryMethods)+len(solution.CustomizableItems) != 6 {
		t.Fatal("solution persistence content fields lost")
	}
	trainer := EnterpriseTrainer{Specialties: []string{"communication"}, Credentials: []string{"credential"}, ServiceIndustries: []string{"manufacturing"}}
	if len(trainer.Specialties)+len(trainer.Credentials)+len(trainer.ServiceIndustries) != 3 {
		t.Fatal("trainer persistence content fields lost")
	}
	consent := PublicationConsent{Channels: []string{"miniapp"}, UsageScopes: []string{"face"}, EvidenceAssetID: 4, ReviewedBy: 5, RevocationReason: "withdrawn"}
	if len(consent.Channels) != 1 || len(consent.UsageScopes) != 1 || consent.EvidenceAssetID == 0 || consent.ReviewedBy == 0 || consent.RevocationReason == "" {
		t.Fatal("consent persistence fields lost")
	}
	consultation := EnterpriseConsultation{
		RequestIdempotencyHash: "request", CompanyNameEncrypted: []byte{1}, RequirementsEncrypted: []byte{2},
		ContactNameEncrypted: []byte{3}, PhoneEncrypted: []byte{4}, PhoneLookupHash: "phone", WechatEncrypted: []byte{5},
		NoteEncrypted: []byte{6}, Industry: "industry", City: "city", ParticipantRange: "20-30", PreferredTrainingTime: "Q4",
		PrivacyNoticeVersion: "v1", ConsentIPHash: "ip", ConsentUserAgentHash: "ua",
	}
	if consultation.RequestIdempotencyHash == "" || len(consultation.CompanyNameEncrypted) == 0 || len(consultation.PhoneEncrypted) == 0 {
		t.Fatal("consultation persistence fields lost")
	}
}
