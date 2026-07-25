package theorystore

import (
	"encoding/json"
	"errors"
	"fmt"
	"math"
	"regexp"
	"strings"
	"time"
	"unicode/utf8"
)

type LibraryStatus string

const (
	LibraryStatusDraft    LibraryStatus = "draft"
	LibraryStatusEnabled  LibraryStatus = "enabled"
	LibraryStatusDisabled LibraryStatus = "disabled"
)

type ReleaseStatus string

const (
	ReleaseStatusDraft    ReleaseStatus = "draft"
	ReleaseStatusBuilding ReleaseStatus = "building"
	ReleaseStatusReady    ReleaseStatus = "ready"
	ReleaseStatusActive   ReleaseStatus = "active"
	ReleaseStatusRetired  ReleaseStatus = "retired"
	ReleaseStatusFailed   ReleaseStatus = "failed"
)

type RetrievalMode string

const (
	RetrievalLexicalOnly RetrievalMode = "lexical_only"
	RetrievalHybrid      RetrievalMode = "hybrid"
)

type WorkType string

const (
	WorkTypeBook         WorkType = "book"
	WorkTypeCourse       WorkType = "course"
	WorkTypeHandout      WorkType = "handout"
	WorkTypeArticle      WorkType = "article"
	WorkTypeOriginalText WorkType = "original_text"
	WorkTypeResearch     WorkType = "research"
	WorkTypeOther        WorkType = "other"
)

type EpistemicStatus string

const (
	EpistemicSourceText           EpistemicStatus = "source_text"
	EpistemicAuthorInterpretation EpistemicStatus = "author_interpretation"
	EpistemicCourseAdaptation     EpistemicStatus = "course_adaptation"
	EpistemicTraditionalSymbolism EpistemicStatus = "traditional_symbolism"
	EpistemicHypothesis           EpistemicStatus = "hypothesis"
	EpistemicEvidenceInformed     EpistemicStatus = "evidence_informed"
)

type CopyrightScope string

const (
	CopyrightMetadataOnly    CopyrightScope = "metadata_only"
	CopyrightInternalExcerpt CopyrightScope = "internal_excerpt"
	CopyrightLicensed        CopyrightScope = "licensed"
	CopyrightFullInternal    CopyrightScope = "full_internal"
)

type SourceWorkStatus string

const (
	SourceWorkStatusRegistered SourceWorkStatus = "registered"
	SourceWorkStatusExtracting SourceWorkStatus = "extracting"
	SourceWorkStatusReviewed   SourceWorkStatus = "reviewed"
	SourceWorkStatusArchived   SourceWorkStatus = "archived"
)

type TitleSource string

const (
	TitleSourceFilename TitleSource = "filename"
	TitleSourceMetadata TitleSource = "metadata"
	TitleSourceCover    TitleSource = "cover"
	TitleSourceManual   TitleSource = "manual"
)

type ExtractionClass string

const (
	ExtractionClassTextRich      ExtractionClass = "text_rich"
	ExtractionClassMixed         ExtractionClass = "mixed"
	ExtractionClassImageDominant ExtractionClass = "image_dominant"
	ExtractionClassCoverOnly     ExtractionClass = "cover_only"
)

type ExtractionStatus string

const (
	ExtractionStatusPending        ExtractionStatus = "pending"
	ExtractionStatusExtracted      ExtractionStatus = "extracted"
	ExtractionStatusNeedsOCR       ExtractionStatus = "needs_ocr"
	ExtractionStatusOCRRunning     ExtractionStatus = "ocr_running"
	ExtractionStatusReviewRequired ExtractionStatus = "review_required"
	ExtractionStatusFailed         ExtractionStatus = "failed"
)

type CardKind string

const (
	CardKindConcept  CardKind = "concept"
	CardKindClaim    CardKind = "claim"
	CardKindAxis     CardKind = "axis"
	CardKindStage    CardKind = "stage"
	CardKindRelation CardKind = "relation"
	CardKindProfile  CardKind = "profile"
	CardKindPractice CardKind = "practice"
	CardKindWarning  CardKind = "warning"
)

type CardStatus string

const (
	StatusDraft      CardStatus = "draft"
	StatusInReview   CardStatus = "in_review"
	StatusPublished  CardStatus = "published"
	StatusSuperseded CardStatus = "superseded"
	StatusRetired    CardStatus = "retired"
)

type EvidenceLevel string

const (
	EvidenceStrong       EvidenceLevel = "strong"
	EvidenceModerate     EvidenceLevel = "moderate"
	EvidenceLimited      EvidenceLevel = "limited"
	EvidenceTraditional  EvidenceLevel = "traditional"
	EvidenceExperiential EvidenceLevel = "experiential"
	EvidenceUnknown      EvidenceLevel = "unknown"
)

type ClinicalSafety string

const (
	ClinicalGeneral    ClinicalSafety = "general"
	ClinicalCaution    ClinicalSafety = "caution"
	ClinicalRestricted ClinicalSafety = "restricted"
	ClinicalEscalate   ClinicalSafety = "escalate"
)

type RelationType string

const (
	RelationBelongsTo    RelationType = "belongs_to"
	RelationPrerequisite RelationType = "prerequisite"
	RelationNextStage    RelationType = "next_stage"
	RelationSupports     RelationType = "supports"
	RelationExtends      RelationType = "extends"
	RelationContrasts    RelationType = "contrasts"
	RelationConflicts    RelationType = "conflicts"
	RelationRisks        RelationType = "risks"
	RelationPractices    RelationType = "practices"
)

type RelationStatus string

const (
	RelationStatusDraft     RelationStatus = "draft"
	RelationStatusPublished RelationStatus = "published"
	RelationStatusDisabled  RelationStatus = "disabled"
)

type SourceRole string

const (
	SourceRolePrimary      SourceRole = "primary"
	SourceRoleSupporting   SourceRole = "supporting"
	SourceRoleExtension    SourceRole = "extension"
	SourceRoleCounterpoint SourceRole = "counterpoint"
	SourceRoleControversy  SourceRole = "controversy"
)

type ChunkKind string

const (
	ChunkKindCard     ChunkKind = "card"
	ChunkKindPractice ChunkKind = "practice"
)

type ChunkStatus string

const (
	ChunkStatusEnabled  ChunkStatus = "enabled"
	ChunkStatusDisabled ChunkStatus = "disabled"
	ChunkStatusRetired  ChunkStatus = "retired"
)

type EmbeddingStatus string

const (
	EmbeddingStatusPending EmbeddingStatus = "pending"
	EmbeddingStatusReady   EmbeddingStatus = "ready"
	EmbeddingStatusFailed  EmbeddingStatus = "failed"
	EmbeddingStatusStale   EmbeddingStatus = "stale"
)

const PracticeSchemaV1 = "xinzhili.practice.v1"

type Library struct {
	ID              int64         `json:"id"`
	Key             string        `json:"key"`
	Name            string        `json:"name"`
	Description     string        `json:"description"`
	Status          LibraryStatus `json:"status"`
	DefaultLanguage string        `json:"defaultLanguage"`
	CurrentVersion  int           `json:"currentVersion"`
	CreatedBy       *int64        `json:"createdBy"`
	UpdatedBy       *int64        `json:"updatedBy"`
	CreateTime      time.Time     `json:"createTime"`
	UpdateTime      time.Time     `json:"updateTime"`
}

type Release struct {
	ID                  int64         `json:"id"`
	LibraryID           int64         `json:"libraryId"`
	Version             int           `json:"version"`
	Status              ReleaseStatus `json:"status"`
	EmbeddingModel      string        `json:"embeddingModel"`
	EmbeddingDimensions int           `json:"embeddingDimensions"`
	RetrievalMode       RetrievalMode `json:"retrievalMode"`
	IndexVersion        string        `json:"indexVersion"`
	CardCount           int           `json:"cardCount"`
	ChunkCount          int           `json:"chunkCount"`
	BuildError          string        `json:"buildError"`
	ActivatedBy         *int64        `json:"activatedBy"`
	ActivatedAt         *time.Time    `json:"activatedAt"`
	CreateTime          time.Time     `json:"createTime"`
	UpdateTime          time.Time     `json:"updateTime"`
}

type SourceWork struct {
	ID              int64            `json:"id"`
	LibraryID       int64            `json:"libraryId"`
	CanonicalKey    string           `json:"canonicalKey"`
	Title           string           `json:"title"`
	OriginalTitle   string           `json:"originalTitle"`
	Authors         json.RawMessage  `json:"authors"`
	Editors         json.RawMessage  `json:"editors"`
	Translators     json.RawMessage  `json:"translators"`
	Publisher       string           `json:"publisher"`
	PublishedYear   *int             `json:"publishedYear"`
	Edition         string           `json:"edition"`
	ISBN            string           `json:"isbn"`
	WorkType        WorkType         `json:"workType"`
	AuthorityLevel  int              `json:"authorityLevel"`
	EpistemicStatus EpistemicStatus  `json:"epistemicStatus"`
	CopyrightScope  CopyrightScope   `json:"copyrightScope"`
	CanonicalWorkID *int64           `json:"canonicalWorkId"`
	Metadata        json.RawMessage  `json:"metadata"`
	Status          SourceWorkStatus `json:"status"`
	CreateTime      time.Time        `json:"createTime"`
	UpdateTime      time.Time        `json:"updateTime"`
}

type SourceFile struct {
	ID                int64            `json:"id"`
	WorkID            int64            `json:"workId"`
	RelativePath      string           `json:"relativePath"`
	OriginalFilename  string           `json:"originalFilename"`
	FileFormat        string           `json:"fileFormat"`
	MIMEType          string           `json:"mimeType"`
	ByteSize          int64            `json:"byteSize"`
	PageCount         *int             `json:"pageCount"`
	SHA256            string           `json:"sha256"`
	DuplicateOfFileID *int64           `json:"duplicateOfFileId"`
	TitleSource       TitleSource      `json:"titleSource"`
	ExtractionClass   ExtractionClass  `json:"extractionClass"`
	ExtractionStatus  ExtractionStatus `json:"extractionStatus"`
	ExtractionQuality float64          `json:"extractionQuality"`
	ExtractedTextURI  string           `json:"extractedTextUri"`
	OCRTextURI        string           `json:"ocrTextUri"`
	ExtractorName     string           `json:"extractorName"`
	ExtractorVersion  string           `json:"extractorVersion"`
	ErrorCode         string           `json:"errorCode"`
	ErrorMessage      string           `json:"errorMessage"`
	Metadata          json.RawMessage  `json:"metadata"`
	CreateTime        time.Time        `json:"createTime"`
	UpdateTime        time.Time        `json:"updateTime"`
}

type Card struct {
	ID                   int64           `json:"id"`
	LibraryID            int64           `json:"libraryId"`
	CanonicalKey         string          `json:"canonicalKey"`
	CanonicalName        string          `json:"canonicalName"`
	Aliases              json.RawMessage `json:"aliases"`
	Domain               string          `json:"domain"`
	Subdomain            string          `json:"subdomain"`
	CardKind             CardKind        `json:"cardKind"`
	Summary              string          `json:"summary"`
	Definition           string          `json:"definition"`
	CoreClaim            string          `json:"coreClaim"`
	Mechanism            string          `json:"mechanism"`
	ApplicableContext    string          `json:"applicableContext"`
	NonApplicableContext string          `json:"nonApplicableContext"`
	ObservableSignals    json.RawMessage `json:"observableSignals"`
	CommonTriggers       json.RawMessage `json:"commonTriggers"`
	AutomaticPattern     string          `json:"automaticPattern"`
	ResourceState        string          `json:"resourceState"`
	ShadowOrRisk         string          `json:"shadowOrRisk"`
	GrowthDirection      string          `json:"growthDirection"`
	EpistemicStatus      EpistemicStatus `json:"epistemicStatus"`
	EvidenceLevel        EvidenceLevel   `json:"evidenceLevel"`
	ClinicalSafety       ClinicalSafety  `json:"clinicalSafety"`
	ControversyNotes     string          `json:"controversyNotes"`
	CulturalContext      string          `json:"culturalContext"`
	AuthorityLevel       int             `json:"authorityLevel"`
	Language             string          `json:"language"`
	Status               CardStatus      `json:"status"`
	Version              int             `json:"version"`
	ReviewedBy           *int64          `json:"reviewedBy"`
	ReviewedAt           *time.Time      `json:"reviewedAt"`
	PublishedAt          *time.Time      `json:"publishedAt"`
	CreatedBy            *int64          `json:"createdBy"`
	UpdatedBy            *int64          `json:"updatedBy"`
	CreateTime           time.Time       `json:"createTime"`
	UpdateTime           time.Time       `json:"updateTime"`
}

type Practice struct {
	ID                     int64           `json:"id"`
	CardID                 int64           `json:"cardId"`
	Goal                   string          `json:"goal"`
	EstimatedMinutes       int             `json:"estimatedMinutes"`
	Steps                  json.RawMessage `json:"steps"`
	ReflectionPrompts      json.RawMessage `json:"reflectionPrompts"`
	ExpectedFeedback       json.RawMessage `json:"expectedFeedback"`
	StopConditions         json.RawMessage `json:"stopConditions"`
	ProfessionalEscalation json.RawMessage `json:"professionalEscalation"`
	Contraindications      string          `json:"contraindications"`
	PracticeSchemaVersion  string          `json:"practiceSchemaVersion"`
	Status                 CardStatus      `json:"status"`
	Version                int             `json:"version"`
	CreateTime             time.Time       `json:"createTime"`
	UpdateTime             time.Time       `json:"updateTime"`
}

type Relation struct {
	ID           int64          `json:"id"`
	FromCardID   int64          `json:"fromCardId"`
	ToCardID     int64          `json:"toCardId"`
	RelationType RelationType   `json:"relationType"`
	Note         string         `json:"note"`
	Confidence   float64        `json:"confidence"`
	Status       RelationStatus `json:"status"`
	CreatedBy    *int64         `json:"createdBy"`
	ReviewedBy   *int64         `json:"reviewedBy"`
	CreateTime   time.Time      `json:"createTime"`
	UpdateTime   time.Time      `json:"updateTime"`
}

type CardSource struct {
	ID                 int64      `json:"id"`
	CardID             int64      `json:"cardId"`
	WorkID             int64      `json:"workId"`
	FileID             *int64     `json:"fileId"`
	SourceRole         SourceRole `json:"sourceRole"`
	Chapter            string     `json:"chapter"`
	PageStart          *int       `json:"pageStart"`
	PageEnd            *int       `json:"pageEnd"`
	LocationLabel      string     `json:"locationLabel"`
	Quotation          string     `json:"quotation"`
	InterpretationNote string     `json:"interpretationNote"`
	ExtractionQuality  float64    `json:"extractionQuality"`
	QuoteVerified      bool       `json:"quoteVerified"`
	VerifiedBy         *int64     `json:"verifiedBy"`
	VerifiedAt         *time.Time `json:"verifiedAt"`
	CreateTime         time.Time  `json:"createTime"`
	UpdateTime         time.Time  `json:"updateTime"`
}

type Chunk struct {
	ID             int64           `json:"id"`
	LibraryID      int64           `json:"libraryId"`
	CardID         int64           `json:"cardId"`
	PracticeID     *int64          `json:"practiceId"`
	ChunkKey       string          `json:"chunkKey"`
	ChunkKind      ChunkKind       `json:"chunkKind"`
	Title          string          `json:"title"`
	Content        string          `json:"content"`
	Keywords       json.RawMessage `json:"keywords"`
	Tags           json.RawMessage `json:"tags"`
	AuthorityLevel int             `json:"authorityLevel"`
	EvidenceLevel  EvidenceLevel   `json:"evidenceLevel"`
	ClinicalSafety ClinicalSafety  `json:"clinicalSafety"`
	TokenCount     int             `json:"tokenCount"`
	ContentHash    string          `json:"contentHash"`
	Version        int             `json:"version"`
	Status         ChunkStatus     `json:"status"`
	CreateTime     time.Time       `json:"createTime"`
	UpdateTime     time.Time       `json:"updateTime"`
}

type EmbeddingRecord struct {
	ID             int64           `json:"id"`
	ChunkID        int64           `json:"chunkId"`
	EmbeddingModel string          `json:"embeddingModel"`
	Dimensions     int             `json:"dimensions"`
	Embedding      []float32       `json:"embedding,omitempty"`
	ContentHash    string          `json:"contentHash"`
	EmbeddedAt     *time.Time      `json:"embeddedAt"`
	Status         EmbeddingStatus `json:"status"`
	ErrorMessage   string          `json:"errorMessage"`
}

var sha256Pattern = regexp.MustCompile(`^[0-9a-f]{64}$`)

func ValidateLibrary(library Library) error {
	if strings.TrimSpace(library.Key) == "" {
		return fieldError("key", "must not be empty")
	}
	if strings.TrimSpace(library.Name) == "" {
		return fieldError("name", "must not be empty")
	}
	if !validLibraryStatus(library.Status) {
		return fieldError("status", "invalid value %q", library.Status)
	}
	if library.CurrentVersion < 0 {
		return fieldError("current_version", "must be at least 0")
	}
	return nil
}

func ValidateRelease(release Release) error {
	if release.LibraryID <= 0 {
		return fieldError("library_id", "must be positive")
	}
	if release.Version <= 0 {
		return fieldError("version", "must be positive")
	}
	if !validReleaseStatus(release.Status) {
		return fieldError("status", "invalid value %q", release.Status)
	}
	if release.RetrievalMode != RetrievalLexicalOnly && release.RetrievalMode != RetrievalHybrid {
		return fieldError("retrieval_mode", "invalid value %q", release.RetrievalMode)
	}
	if release.EmbeddingDimensions != 1536 {
		return fieldError("embedding_dimensions", "must equal 1536")
	}
	if release.RetrievalMode == RetrievalHybrid {
		if strings.TrimSpace(release.EmbeddingModel) == "" {
			return fieldError("embedding_model", "must not be empty for hybrid retrieval")
		}
	}
	if release.CardCount < 0 {
		return fieldError("card_count", "must be at least 0")
	}
	if release.ChunkCount < 0 {
		return fieldError("chunk_count", "must be at least 0")
	}
	return nil
}

func ValidateSourceWork(work SourceWork) error {
	if work.LibraryID <= 0 {
		return fieldError("library_id", "must be positive")
	}
	if strings.TrimSpace(work.CanonicalKey) == "" {
		return fieldError("canonical_key", "must not be empty")
	}
	if strings.TrimSpace(work.Title) == "" {
		return fieldError("title", "must not be empty")
	}
	if !validWorkType(work.WorkType) {
		return fieldError("work_type", "invalid value %q", work.WorkType)
	}
	if err := validateAuthority(work.AuthorityLevel); err != nil {
		return err
	}
	if !validEpistemicStatus(work.EpistemicStatus) {
		return fieldError("epistemic_status", "invalid value %q", work.EpistemicStatus)
	}
	if !validCopyrightScope(work.CopyrightScope) {
		return fieldError("copyright_scope", "invalid value %q", work.CopyrightScope)
	}
	if !validSourceWorkStatus(work.Status) {
		return fieldError("status", "invalid value %q", work.Status)
	}
	if work.CanonicalWorkID != nil && *work.CanonicalWorkID <= 0 {
		return fieldError("canonical_work_id", "must be positive")
	}
	if work.ID > 0 && work.CanonicalWorkID != nil && *work.CanonicalWorkID == work.ID {
		return fieldError("canonical_work_id", "must not refer to itself")
	}
	return nil
}

func ValidateSourceFile(file SourceFile) error {
	if file.WorkID <= 0 {
		return fieldError("work_id", "must be positive")
	}
	if strings.TrimSpace(file.RelativePath) == "" {
		return fieldError("relative_path", "must not be empty")
	}
	if strings.TrimSpace(file.OriginalFilename) == "" {
		return fieldError("original_filename", "must not be empty")
	}
	if strings.TrimSpace(file.FileFormat) == "" {
		return fieldError("file_format", "must not be empty")
	}
	if file.ByteSize < 0 {
		return fieldError("byte_size", "must be at least 0")
	}
	if file.PageCount != nil && *file.PageCount <= 0 {
		return fieldError("page_count", "must be positive")
	}
	if !sha256Pattern.MatchString(file.SHA256) {
		return fieldError("sha256", "must be 64 lowercase hexadecimal characters")
	}
	if file.DuplicateOfFileID != nil && *file.DuplicateOfFileID <= 0 {
		return fieldError("duplicate_of_file_id", "must be positive")
	}
	if file.ID > 0 && file.DuplicateOfFileID != nil && *file.DuplicateOfFileID == file.ID {
		return fieldError("duplicate_of_file_id", "must not refer to itself")
	}
	if !validTitleSource(file.TitleSource) {
		return fieldError("title_source", "invalid value %q", file.TitleSource)
	}
	if !validExtractionClass(file.ExtractionClass) {
		return fieldError("extraction_class", "invalid value %q", file.ExtractionClass)
	}
	if !validExtractionStatus(file.ExtractionStatus) {
		return fieldError("extraction_status", "invalid value %q", file.ExtractionStatus)
	}
	return validateUnitInterval("extraction_quality", file.ExtractionQuality)
}

func ValidateCard(card Card) error {
	if card.LibraryID <= 0 {
		return fieldError("library_id", "must be positive")
	}
	if strings.TrimSpace(card.CanonicalKey) == "" {
		return fieldError("canonical_key", "must not be empty")
	}
	if strings.TrimSpace(card.CanonicalName) == "" {
		return fieldError("canonical_name", "must not be empty")
	}
	if !validCardKind(card.CardKind) {
		return fieldError("card_kind", "invalid value %q", card.CardKind)
	}
	if !validEpistemicStatus(card.EpistemicStatus) {
		return fieldError("epistemic_status", "invalid value %q", card.EpistemicStatus)
	}
	if !validEvidenceLevel(card.EvidenceLevel) {
		return fieldError("evidence_level", "invalid value %q", card.EvidenceLevel)
	}
	if !validClinicalSafety(card.ClinicalSafety) {
		return fieldError("clinical_safety", "invalid value %q", card.ClinicalSafety)
	}
	if err := validateAuthority(card.AuthorityLevel); err != nil {
		return err
	}
	if !validCardStatus(card.Status) {
		return fieldError("status", "invalid value %q", card.Status)
	}
	if card.Version <= 0 {
		return fieldError("version", "must be positive")
	}
	return nil
}

func ValidateCardForPublish(card Card, sources []CardSource) error {
	var problems []string
	if err := ValidateCard(card); err != nil {
		problems = append(problems, err.Error())
	}
	for _, required := range []struct {
		field string
		value string
	}{
		{field: "definition", value: card.Definition},
		{field: "applicable_context", value: card.ApplicableContext},
		{field: "non_applicable_context", value: card.NonApplicableContext},
	} {
		if strings.TrimSpace(required.value) == "" {
			problems = append(problems, fieldError(required.field, "must not be empty for publish").Error())
		}
	}
	if !validEpistemicStatus(card.EpistemicStatus) {
		problems = appendUnique(problems, fieldError("epistemic_status", "must be set for publish").Error())
	}
	if !validEvidenceLevel(card.EvidenceLevel) {
		problems = appendUnique(problems, fieldError("evidence_level", "must be set for publish").Error())
	}
	if !validClinicalSafety(card.ClinicalSafety) {
		problems = appendUnique(problems, fieldError("clinical_safety", "must be set for publish").Error())
	}

	hasPrimary := false
	for i, source := range sources {
		if err := ValidateCardSource(source); err != nil {
			problems = append(problems, fmt.Sprintf("sources[%d].%s", i, err.Error()))
			continue
		}
		if source.SourceRole == SourceRolePrimary {
			hasPrimary = true
		}
		if source.ExtractionQuality < 0.70 {
			problems = append(problems, fmt.Sprintf("sources[%d].extraction_quality: must be at least 0.70 for publish", i))
		}
		if strings.TrimSpace(source.Quotation) != "" {
			if !source.QuoteVerified {
				problems = append(problems, fmt.Sprintf("sources[%d].quote_verified: must be true for a published quotation", i))
			}
			if source.VerifiedBy == nil || *source.VerifiedBy <= 0 {
				problems = append(problems, fmt.Sprintf("sources[%d].verified_by: must be positive for a published quotation", i))
			}
			if source.VerifiedAt == nil || source.VerifiedAt.IsZero() {
				problems = append(problems, fmt.Sprintf("sources[%d].verified_at: must be set for a published quotation", i))
			}
		}
	}
	if !hasPrimary {
		problems = append(problems, "primary source: at least one valid primary source is required for publish")
	}
	if len(problems) > 0 {
		return errors.New(strings.Join(problems, "; "))
	}
	return nil
}

func ValidatePractice(practice Practice) error {
	if practice.CardID <= 0 {
		return fieldError("card_id", "must be positive")
	}
	if strings.TrimSpace(practice.Goal) == "" {
		return fieldError("goal", "must not be empty")
	}
	if practice.EstimatedMinutes < 0 {
		return fieldError("estimated_minutes", "must be at least 0")
	}
	if practice.PracticeSchemaVersion != PracticeSchemaV1 {
		return fieldError("practice_schema_version", "unsupported value %q", practice.PracticeSchemaVersion)
	}
	steps, err := decodeStringArray("steps", practice.Steps)
	if err != nil {
		return err
	}
	if !hasNonBlankString(steps) {
		return fieldError("steps", "must contain at least one non-empty item")
	}
	if _, err := decodeStringArray("reflection_prompts", practice.ReflectionPrompts); err != nil {
		return err
	}
	if _, err := decodeStringArray("expected_feedback", practice.ExpectedFeedback); err != nil {
		return err
	}
	if _, err := decodeStringArray("stop_conditions", practice.StopConditions); err != nil {
		return err
	}
	if _, err := decodeStringArray("professional_escalation", practice.ProfessionalEscalation); err != nil {
		return err
	}
	if !validCardStatus(practice.Status) {
		return fieldError("status", "invalid value %q", practice.Status)
	}
	if practice.Version <= 0 {
		return fieldError("version", "must be positive")
	}
	return nil
}

func ValidatePracticeForPublish(practice Practice, card Card) error {
	if err := ValidatePractice(practice); err != nil {
		return err
	}
	if card.ID <= 0 {
		return fieldError("card_id", "associated card id must be positive")
	}
	if practice.CardID != card.ID {
		return fieldError("card_id", "must match associated card id")
	}
	if !validClinicalSafety(card.ClinicalSafety) {
		return fieldError("clinical_safety", "invalid value %q", card.ClinicalSafety)
	}
	steps, _ := decodeStringArray("steps", practice.Steps)
	if !hasNonBlankString(steps) {
		return fieldError("steps", "must contain at least one non-empty item for publish")
	}
	if card.ClinicalSafety != ClinicalRestricted && card.ClinicalSafety != ClinicalEscalate {
		return nil
	}
	stopConditions, _ := decodeStringArray("stop_conditions", practice.StopConditions)
	if !hasNonBlankString(stopConditions) {
		return fieldError("stop_conditions", "must contain at least one non-empty item for %s practice", card.ClinicalSafety)
	}
	escalation, _ := decodeStringArray("professional_escalation", practice.ProfessionalEscalation)
	if !hasNonBlankString(escalation) {
		return fieldError("professional_escalation", "must contain at least one non-empty item for %s practice", card.ClinicalSafety)
	}
	return nil
}

func ValidateRelation(relation Relation) error {
	if relation.FromCardID <= 0 {
		return fieldError("from_card_id", "must be positive")
	}
	if relation.ToCardID <= 0 {
		return fieldError("to_card_id", "must be positive")
	}
	if relation.FromCardID == relation.ToCardID {
		return fieldError("to_card_id", "must differ from from_card_id")
	}
	if !validRelationType(relation.RelationType) {
		return fieldError("relation_type", "invalid value %q", relation.RelationType)
	}
	if err := validateUnitInterval("confidence", relation.Confidence); err != nil {
		return err
	}
	if !validRelationStatus(relation.Status) {
		return fieldError("status", "invalid value %q", relation.Status)
	}
	return nil
}

func ValidateCardSource(source CardSource) error {
	if source.CardID <= 0 {
		return fieldError("card_id", "must be positive")
	}
	if source.WorkID <= 0 {
		return fieldError("work_id", "must be positive")
	}
	if source.FileID != nil && *source.FileID <= 0 {
		return fieldError("file_id", "must be positive")
	}
	if !validSourceRole(source.SourceRole) {
		return fieldError("source_role", "invalid value %q", source.SourceRole)
	}
	if source.PageStart != nil && *source.PageStart <= 0 {
		return fieldError("page_start", "must be positive")
	}
	if source.PageEnd != nil && *source.PageEnd <= 0 {
		return fieldError("page_end", "must be positive")
	}
	if source.PageStart != nil && source.PageEnd != nil && *source.PageEnd < *source.PageStart {
		return fieldError("page_end", "must be greater than or equal to page_start")
	}
	return validateUnitInterval("extraction_quality", source.ExtractionQuality)
}

func ValidateChunk(chunk Chunk) error {
	if chunk.LibraryID <= 0 {
		return fieldError("library_id", "must be positive")
	}
	if chunk.CardID <= 0 {
		return fieldError("card_id", "must be positive")
	}
	if strings.TrimSpace(chunk.ChunkKey) == "" {
		return fieldError("chunk_key", "must not be empty")
	}
	if chunk.ChunkKind != ChunkKindCard && chunk.ChunkKind != ChunkKindPractice {
		return fieldError("chunk_kind", "invalid value %q", chunk.ChunkKind)
	}
	if chunk.ChunkKind == ChunkKindPractice && (chunk.PracticeID == nil || *chunk.PracticeID <= 0) {
		return fieldError("practice_id", "must be positive for a practice chunk")
	}
	if chunk.ChunkKind == ChunkKindCard && chunk.PracticeID != nil {
		return fieldError("practice_id", "must be nil for a card chunk")
	}
	if strings.TrimSpace(chunk.Title) == "" {
		return fieldError("title", "must not be empty")
	}
	if strings.TrimSpace(chunk.Content) == "" {
		return fieldError("content", "must not be empty")
	}
	if err := validateAuthority(chunk.AuthorityLevel); err != nil {
		return err
	}
	if !validEvidenceLevel(chunk.EvidenceLevel) {
		return fieldError("evidence_level", "invalid value %q", chunk.EvidenceLevel)
	}
	if !validClinicalSafety(chunk.ClinicalSafety) {
		return fieldError("clinical_safety", "invalid value %q", chunk.ClinicalSafety)
	}
	if chunk.TokenCount < 0 {
		return fieldError("token_count", "must be at least 0")
	}
	if strings.TrimSpace(chunk.ContentHash) == "" {
		return fieldError("content_hash", "must not be empty")
	}
	if chunk.Version <= 0 {
		return fieldError("version", "must be positive")
	}
	if !validChunkStatus(chunk.Status) {
		return fieldError("status", "invalid value %q", chunk.Status)
	}
	return nil
}

func ValidateEmbeddingRecord(record EmbeddingRecord) error {
	if record.ChunkID <= 0 {
		return fieldError("chunk_id", "must be positive")
	}
	if strings.TrimSpace(record.EmbeddingModel) == "" {
		return fieldError("embedding_model", "must not be empty")
	}
	if record.Dimensions != 1536 {
		return fieldError("dimensions", "must equal 1536")
	}
	if strings.TrimSpace(record.ContentHash) == "" {
		return fieldError("content_hash", "must not be empty")
	}
	if !validEmbeddingStatus(record.Status) {
		return fieldError("status", "invalid value %q", record.Status)
	}
	if len(record.Embedding) > 0 && len(record.Embedding) != record.Dimensions {
		return fieldError("embedding", "length must equal dimensions")
	}
	if record.Status == EmbeddingStatusReady && len(record.Embedding) != record.Dimensions {
		return fieldError("embedding", "must contain 1536 values when status is ready")
	}
	if record.Status == EmbeddingStatusReady {
		for _, value := range record.Embedding {
			if math.IsNaN(float64(value)) || math.IsInf(float64(value), 0) {
				return fieldError("embedding", "must contain only finite values when status is ready")
			}
		}
	}
	return nil
}

func decodeStringArray(field string, raw json.RawMessage) ([]string, error) {
	var values []string
	if len(raw) == 0 || !utf8.Valid(raw) || json.Unmarshal(raw, &values) != nil || values == nil {
		return nil, fieldError(field, "must be a JSON array of strings")
	}
	return values, nil
}

func hasNonBlankString(values []string) bool {
	for _, value := range values {
		if strings.TrimSpace(value) != "" {
			return true
		}
	}
	return false
}

func validateAuthority(value int) error {
	if value < 1 || value > 5 {
		return fieldError("authority_level", "must be between 1 and 5")
	}
	return nil
}

func validateUnitInterval(field string, value float64) error {
	if math.IsNaN(value) || value < 0 || value > 1 {
		return fieldError(field, "must be between 0 and 1")
	}
	return nil
}

func fieldError(field, format string, args ...any) error {
	return fmt.Errorf("%s: %s", field, fmt.Sprintf(format, args...))
}

func appendUnique(values []string, value string) []string {
	for _, existing := range values {
		if existing == value {
			return values
		}
	}
	return append(values, value)
}

func validLibraryStatus(value LibraryStatus) bool {
	return value == LibraryStatusDraft || value == LibraryStatusEnabled || value == LibraryStatusDisabled
}

func validReleaseStatus(value ReleaseStatus) bool {
	return value == ReleaseStatusDraft || value == ReleaseStatusBuilding || value == ReleaseStatusReady || value == ReleaseStatusActive || value == ReleaseStatusRetired || value == ReleaseStatusFailed
}

func validWorkType(value WorkType) bool {
	return value == WorkTypeBook || value == WorkTypeCourse || value == WorkTypeHandout || value == WorkTypeArticle || value == WorkTypeOriginalText || value == WorkTypeResearch || value == WorkTypeOther
}

func validEpistemicStatus(value EpistemicStatus) bool {
	return value == EpistemicSourceText || value == EpistemicAuthorInterpretation || value == EpistemicCourseAdaptation || value == EpistemicTraditionalSymbolism || value == EpistemicHypothesis || value == EpistemicEvidenceInformed
}

func validCopyrightScope(value CopyrightScope) bool {
	return value == CopyrightMetadataOnly || value == CopyrightInternalExcerpt || value == CopyrightLicensed || value == CopyrightFullInternal
}

func validSourceWorkStatus(value SourceWorkStatus) bool {
	return value == SourceWorkStatusRegistered || value == SourceWorkStatusExtracting || value == SourceWorkStatusReviewed || value == SourceWorkStatusArchived
}

func validTitleSource(value TitleSource) bool {
	return value == TitleSourceFilename || value == TitleSourceMetadata || value == TitleSourceCover || value == TitleSourceManual
}

func validExtractionClass(value ExtractionClass) bool {
	return value == ExtractionClassTextRich || value == ExtractionClassMixed || value == ExtractionClassImageDominant || value == ExtractionClassCoverOnly
}

func validExtractionStatus(value ExtractionStatus) bool {
	return value == ExtractionStatusPending || value == ExtractionStatusExtracted || value == ExtractionStatusNeedsOCR || value == ExtractionStatusOCRRunning || value == ExtractionStatusReviewRequired || value == ExtractionStatusFailed
}

func validCardKind(value CardKind) bool {
	return value == CardKindConcept || value == CardKindClaim || value == CardKindAxis || value == CardKindStage || value == CardKindRelation || value == CardKindProfile || value == CardKindPractice || value == CardKindWarning
}

func validCardStatus(value CardStatus) bool {
	return value == StatusDraft || value == StatusInReview || value == StatusPublished || value == StatusSuperseded || value == StatusRetired
}

func validEvidenceLevel(value EvidenceLevel) bool {
	return value == EvidenceStrong || value == EvidenceModerate || value == EvidenceLimited || value == EvidenceTraditional || value == EvidenceExperiential || value == EvidenceUnknown
}

func validClinicalSafety(value ClinicalSafety) bool {
	return value == ClinicalGeneral || value == ClinicalCaution || value == ClinicalRestricted || value == ClinicalEscalate
}

func validRelationType(value RelationType) bool {
	return value == RelationBelongsTo || value == RelationPrerequisite || value == RelationNextStage || value == RelationSupports || value == RelationExtends || value == RelationContrasts || value == RelationConflicts || value == RelationRisks || value == RelationPractices
}

func validRelationStatus(value RelationStatus) bool {
	return value == RelationStatusDraft || value == RelationStatusPublished || value == RelationStatusDisabled
}

func validSourceRole(value SourceRole) bool {
	return value == SourceRolePrimary || value == SourceRoleSupporting || value == SourceRoleExtension || value == SourceRoleCounterpoint || value == SourceRoleControversy
}

func validChunkStatus(value ChunkStatus) bool {
	return value == ChunkStatusEnabled || value == ChunkStatusDisabled || value == ChunkStatusRetired
}

func validEmbeddingStatus(value EmbeddingStatus) bool {
	return value == EmbeddingStatusPending || value == EmbeddingStatusReady || value == EmbeddingStatusFailed || value == EmbeddingStatusStale
}
