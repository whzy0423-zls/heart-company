// Package enterprisepromotion defines the content and lead domain used by the
// enterprise-training promotion experience. Persistence and HTTP concerns live
// outside these records.
package enterprisepromotion

import (
	"errors"
	"strings"
	"time"
)

var (
	ErrInvalidCase        = errors.New("enterprise promotion case is invalid")
	ErrInvalidSolution    = errors.New("enterprise promotion solution is invalid")
	ErrTrainerRequired    = errors.New("enterprise promotion trainer is required")
	ErrInvalidConsentLink = errors.New("enterprise promotion consent link is invalid")
)

type CaseStatus string

const (
	CaseDraft     CaseStatus = "draft"
	CaseReview    CaseStatus = "review"
	CasePublished CaseStatus = "published"
	CaseOffline   CaseStatus = "offline"
)

func (t ConsentSubjectType) Valid() bool {
	switch t {
	case ConsentCompany, ConsentPerson, ConsentMediaAsset, ConsentTestimonial, ConsentDocumentScreen:
		return true
	default:
		return false
	}
}

func (s CaseStatus) Valid() bool {
	switch s {
	case CaseDraft, CaseReview, CasePublished, CaseOffline:
		return true
	default:
		return false
	}
}

type AuthorizationStatus string

const (
	AuthorizationPending  AuthorizationStatus = "pending"
	AuthorizationApproved AuthorizationStatus = "approved"
	AuthorizationExpired  AuthorizationStatus = "expired"
	AuthorizationRevoked  AuthorizationStatus = "revoked"
)

func (s AuthorizationStatus) Valid() bool {
	switch s {
	case AuthorizationPending, AuthorizationApproved, AuthorizationExpired, AuthorizationRevoked:
		return true
	default:
		return false
	}
}

type TopicKey string

const (
	TopicTeamCommunication TopicKey = "team-communication"
	TopicLeadership        TopicKey = "leadership"
	TopicCohesion          TopicKey = "cohesion"
	TopicCulture           TopicKey = "culture"
	TopicEmployeeGrowth    TopicKey = "employee-growth"
)

func (k TopicKey) Valid() bool {
	switch k {
	case TopicTeamCommunication, TopicLeadership, TopicCohesion, TopicCulture, TopicEmployeeGrowth:
		return true
	default:
		return false
	}
}

type MediaRole string

const (
	MediaPromo     MediaRole = "promo"
	MediaHighlight MediaRole = "highlight"
	MediaTopicClip MediaRole = "topic_clip"
	MediaGallery   MediaRole = "gallery"
)

func (r MediaRole) Valid() bool {
	switch r {
	case MediaPromo, MediaHighlight, MediaTopicClip, MediaGallery:
		return true
	default:
		return false
	}
}

type ConsentSubjectType string

const (
	ConsentCompany        ConsentSubjectType = "company"
	ConsentPerson         ConsentSubjectType = "person"
	ConsentMediaAsset     ConsentSubjectType = "media_asset"
	ConsentTestimonial    ConsentSubjectType = "testimonial"
	ConsentDocumentScreen ConsentSubjectType = "document_screen"
)

type ConsultationStatus string

const (
	ConsultationNew       ConsultationStatus = "new"
	ConsultationContacted ConsultationStatus = "contacted"
	ConsultationQualified ConsultationStatus = "qualified"
	ConsultationProposal  ConsultationStatus = "proposal"
	ConsultationWon       ConsultationStatus = "won"
	ConsultationLost      ConsultationStatus = "lost"
	ConsultationSpam      ConsultationStatus = "spam"
)

type PrivacyRequestType string

const (
	PrivacyAccess     PrivacyRequestType = "access"
	PrivacyCorrection PrivacyRequestType = "correction"
	PrivacyDeletion   PrivacyRequestType = "deletion"
)

type TrainingCase struct {
	ID                  int64
	Slug                string
	Title               string
	Summary             string
	CoverAssetID        int64
	CompanyDisplayName  string
	Industry            string
	City                string
	ParticipantRange    string
	TrainingDate        *time.Time
	DurationLabel       string
	TrainerID           int64
	TrainerNameSnapshot string
	Status              CaseStatus
	AuthorizationStatus AuthorizationStatus
	Featured            bool
	SortOrder           int
	Version             int64
	PublishedAt         *time.Time
	CreatedAt           time.Time
	UpdatedAt           time.Time
}

func (c TrainingCase) ValidateForReview() error {
	if strings.TrimSpace(c.Title) == "" || strings.TrimSpace(c.Slug) == "" {
		return ErrInvalidCase
	}
	if c.TrainerID <= 0 {
		return ErrTrainerRequired
	}
	return nil
}

type EnterpriseSolution struct {
	ID                  int64
	Slug                string
	Title               string
	Summary             string
	CoverAssetID        int64
	TrainerID           int64
	TrainerNameSnapshot string
	Status              CaseStatus
	Featured            bool
	SortOrder           int
	Version             int64
	PublishedAt         *time.Time
	CreatedAt           time.Time
	UpdatedAt           time.Time
}

func (s EnterpriseSolution) ValidateForReview() error {
	if strings.TrimSpace(s.Title) == "" || strings.TrimSpace(s.Slug) == "" {
		return ErrInvalidSolution
	}
	if s.TrainerID <= 0 {
		return ErrTrainerRequired
	}
	return nil
}

type EnterpriseTrainer struct {
	ID                int64
	Key               string
	Name              string
	Title             string
	AvatarAssetID     int64
	ShortBio          string
	FullBio           string
	ExperienceSummary string
	Status            CaseStatus
	SortOrder         int
	Version           int64
	CreatedAt         time.Time
	UpdatedAt         time.Time
}

type TrainingTopic struct {
	ID        int64
	Key       TopicKey
	Title     string
	SortOrder int
	Enabled   bool
}

type CaseMedia struct {
	ID           int64
	CaseID       int64
	MediaAssetID int64
	Role         MediaRole
	Position     int
	Caption      string
	Status       CaseStatus
}

type PublicationConsent struct {
	ID               int64
	SubjectType      ConsentSubjectType
	SubjectReference string
	DisplayAlias     string
	Status           AuthorizationStatus
	EffectiveAt      *time.Time
	ExpiresAt        *time.Time
	Version          int64
}

// ConsentLink associates a consent with its independently reviewable subject.
// CaseID is optional so media authorization can be completed before an asset is
// selected for a training case.
type ConsentLink struct {
	ID             int64
	ConsentID      int64
	CaseID         int64
	MediaAssetID   int64
	TestimonialID  int64
	SubjectType    ConsentSubjectType
	SubjectID      int64
	UseScope       string
	RequirementKey string
	Required       bool
}

func (l ConsentLink) Validate() error {
	if l.ConsentID <= 0 || !l.SubjectType.Valid() || l.SubjectID <= 0 || strings.TrimSpace(l.UseScope) == "" {
		return ErrInvalidConsentLink
	}
	if l.CaseID <= 0 && l.MediaAssetID <= 0 && l.TestimonialID <= 0 {
		return ErrInvalidConsentLink
	}
	if l.SubjectType == ConsentMediaAsset && l.MediaAssetID != l.SubjectID {
		return ErrInvalidConsentLink
	}
	return nil
}

type EnterpriseConsultation struct {
	ID                        int64
	ConsultationReferenceHash string
	SourcePage                string
	CaseID                    int64
	SolutionID                int64
	FirstTouchSessionID       int64
	LastTouchSessionID        int64
	Channel                   string
	Status                    ConsultationStatus
	AssigneeID                int64
	Version                   int64
	PrivacyNoticeVersion      string
	ConsentedAt               time.Time
	ConsentSource             string
	CreatedAt                 time.Time
	UpdatedAt                 time.Time
}
