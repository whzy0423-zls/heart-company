package theorystore

import (
	"encoding/json"
	"math"
	"strings"
	"testing"
	"time"
)

func TestValidateLibraryRejectsInvalidStatus(t *testing.T) {
	err := ValidateLibrary(Library{Key: "xinzhili", Name: "芯之力", Status: LibraryStatus("unknown")})
	assertFieldError(t, err, "status")
}

func TestValidateReleaseEnforcesRetrievalContract(t *testing.T) {
	tests := []struct {
		name    string
		release Release
		field   string
	}{
		{
			name:    "invalid status",
			release: Release{LibraryID: 1, Version: 1, Status: ReleaseStatus("unknown"), RetrievalMode: RetrievalLexicalOnly},
			field:   "status",
		},
		{
			name:    "hybrid dimensions",
			release: Release{LibraryID: 1, Version: 1, Status: ReleaseStatusDraft, RetrievalMode: RetrievalHybrid, EmbeddingModel: "text-embedding-3-small", EmbeddingDimensions: 768},
			field:   "embedding_dimensions",
		},
		{
			name:    "hybrid model",
			release: Release{LibraryID: 1, Version: 1, Status: ReleaseStatusDraft, RetrievalMode: RetrievalHybrid, EmbeddingDimensions: 1536},
			field:   "embedding_model",
		},
		{
			name:    "lexical dimensions",
			release: Release{LibraryID: 1, Version: 1, Status: ReleaseStatusDraft, RetrievalMode: RetrievalLexicalOnly},
			field:   "embedding_dimensions",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			assertFieldError(t, ValidateRelease(tt.release), tt.field)
		})
	}

	if err := ValidateRelease(Release{LibraryID: 1, Version: 1, Status: ReleaseStatusReady, RetrievalMode: RetrievalLexicalOnly, EmbeddingDimensions: 1536}); err != nil {
		t.Fatalf("lexical-only release should not require an embedding model: %v", err)
	}
}

func TestValidateSourceWorkRejectsAuthorityOutsideRange(t *testing.T) {
	work := validSourceWork()
	work.AuthorityLevel = 6
	assertFieldError(t, ValidateSourceWork(work), "authority_level")
}

func TestValidateSourceFileRejectsInvalidHashQualityAndSelfDuplicate(t *testing.T) {
	tests := []struct {
		name   string
		mutate func(*SourceFile)
		field  string
	}{
		{name: "sha256", mutate: func(file *SourceFile) { file.SHA256 = strings.Repeat("A", 64) }, field: "sha256"},
		{name: "quality", mutate: func(file *SourceFile) { file.ExtractionQuality = 1.01 }, field: "extraction_quality"},
		{name: "quality nan", mutate: func(file *SourceFile) { file.ExtractionQuality = math.NaN() }, field: "extraction_quality"},
		{name: "duplicate", mutate: func(file *SourceFile) { file.ID = 7; duplicateID := int64(7); file.DuplicateOfFileID = &duplicateID }, field: "duplicate_of_file_id"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			file := validSourceFile()
			tt.mutate(&file)
			assertFieldError(t, ValidateSourceFile(file), tt.field)
		})
	}
}

func TestValidateCardRejectsInvalidStatusAndAuthority(t *testing.T) {
	card := validCard()
	card.Status = CardStatus("unknown")
	assertFieldError(t, ValidateCard(card), "status")

	card = validCard()
	card.AuthorityLevel = 0
	assertFieldError(t, ValidateCard(card), "authority_level")
}

func TestValidateCardForPublishRequiresContentSafetyAndPrimarySource(t *testing.T) {
	card := Card{CanonicalKey: "observer", CanonicalName: "内在观察者", Status: StatusPublished}
	err := ValidateCardForPublish(card, nil)
	for _, field := range []string{"definition", "applicable_context", "non_applicable_context", "epistemic_status", "evidence_level", "clinical_safety", "primary source"} {
		assertErrorContains(t, err, field)
	}
}

func TestValidateCardForPublishReturnsStableFieldOrder(t *testing.T) {
	card := Card{CanonicalKey: "observer", CanonicalName: "内在观察者", Status: StatusPublished}
	for i := 0; i < 100; i++ {
		message := ValidateCardForPublish(card, nil).Error()
		definition := strings.Index(message, "definition")
		applicable := strings.Index(message, "applicable_context")
		nonApplicable := strings.Index(message, "non_applicable_context")
		if !(definition < applicable && applicable < nonApplicable) {
			t.Fatalf("publish validation field order is unstable: %s", message)
		}
	}
}

func TestValidateCardForPublishRejectsLowQualityAndUnverifiedQuotation(t *testing.T) {
	card := validCard()
	card.Status = StatusPublished

	lowQuality := validCardSource()
	lowQuality.ExtractionQuality = 0.69
	assertFieldError(t, ValidateCardForPublish(card, []CardSource{lowQuality}), "extraction_quality")

	unverifiedQuote := validCardSource()
	unverifiedQuote.Quotation = "逐字引文"
	unverifiedQuote.QuoteVerified = false
	assertFieldError(t, ValidateCardForPublish(card, []CardSource{unverifiedQuote}), "quote_verified")
}

func TestValidateCardForPublishRequiresQuotationAuditFields(t *testing.T) {
	card := validCard()
	card.Status = StatusPublished
	verifiedBy := int64(9)
	verifiedAt := time.Now()
	tests := []struct {
		name   string
		mutate func(*CardSource)
		field  string
	}{
		{name: "missing verifier", mutate: func(source *CardSource) { source.VerifiedBy = nil }, field: "verified_by"},
		{name: "invalid verifier", mutate: func(source *CardSource) { invalid := int64(0); source.VerifiedBy = &invalid }, field: "verified_by"},
		{name: "missing verification time", mutate: func(source *CardSource) { source.VerifiedAt = nil }, field: "verified_at"},
		{name: "zero verification time", mutate: func(source *CardSource) { zero := time.Time{}; source.VerifiedAt = &zero }, field: "verified_at"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			source := validCardSource()
			source.Quotation = "逐字引文"
			source.QuoteVerified = true
			source.VerifiedBy = &verifiedBy
			source.VerifiedAt = &verifiedAt
			tt.mutate(&source)
			assertFieldError(t, ValidateCardForPublish(card, []CardSource{source}), tt.field)
		})
	}
}

func TestValidateCardForPublishAcceptsCompleteCard(t *testing.T) {
	card := validCard()
	card.Status = StatusPublished
	source := validCardSource()
	if err := ValidateCardForPublish(card, []CardSource{source}); err != nil {
		t.Fatalf("complete publishable card rejected: %v", err)
	}
}

func TestValidatePracticeRequiresVersionedStringArrays(t *testing.T) {
	tests := []struct {
		name   string
		mutate func(*Practice)
		field  string
	}{
		{name: "unknown schema", mutate: func(p *Practice) { p.PracticeSchemaVersion = "xinzhili.practice.v2" }, field: "practice_schema_version"},
		{name: "steps not json", mutate: func(p *Practice) { p.Steps = json.RawMessage(`not-json`) }, field: "steps"},
		{name: "steps not array", mutate: func(p *Practice) { p.Steps = json.RawMessage(`{"text":"step"}`) }, field: "steps"},
		{name: "steps contain non-string", mutate: func(p *Practice) { p.Steps = json.RawMessage(`[1]`) }, field: "steps"},
		{name: "steps invalid utf8", mutate: func(p *Practice) { p.Steps = json.RawMessage{'[', '"', 0xff, '"', ']'} }, field: "steps"},
		{name: "reflection prompts", mutate: func(p *Practice) { p.ReflectionPrompts = json.RawMessage(`[false]`) }, field: "reflection_prompts"},
		{name: "expected feedback", mutate: func(p *Practice) { p.ExpectedFeedback = json.RawMessage(`null`) }, field: "expected_feedback"},
		{name: "stop conditions", mutate: func(p *Practice) { p.StopConditions = json.RawMessage(`[{}]`) }, field: "stop_conditions"},
		{name: "professional escalation", mutate: func(p *Practice) { p.ProfessionalEscalation = json.RawMessage(`1`) }, field: "professional_escalation"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			practice := validPractice()
			tt.mutate(&practice)
			assertFieldError(t, ValidatePractice(practice), tt.field)
		})
	}
}

func TestValidatePracticeAllowsDraftWithEmptySteps(t *testing.T) {
	practice := validPractice()
	practice.Steps = json.RawMessage(`[]`)
	if err := ValidatePractice(practice); err != nil {
		t.Fatalf("draft practice should allow empty steps: %v", err)
	}
}

func TestValidatePracticeOnlyChecksVersionAndJSONArraySchema(t *testing.T) {
	practice := Practice{
		PracticeSchemaVersion:  PracticeSchemaV1,
		Steps:                  json.RawMessage(`[]`),
		ReflectionPrompts:      json.RawMessage(`[]`),
		ExpectedFeedback:       json.RawMessage(`[]`),
		StopConditions:         json.RawMessage(`[]`),
		ProfessionalEscalation: json.RawMessage(`[]`),
	}
	if err := ValidatePractice(practice); err != nil {
		t.Fatalf("base practice validation should only enforce the versioned JSON schema: %v", err)
	}
}

func TestValidatePracticeForPublishRequiresTrimmedNonEmptyItems(t *testing.T) {
	card := validCard()
	card.ID = 1
	practice := validPractice()
	practice.Steps = json.RawMessage(`["  "]`)
	assertFieldError(t, ValidatePracticeForPublish(practice, card), "steps")

	card.ClinicalSafety = ClinicalRestricted
	practice = validPractice()
	practice.StopConditions = json.RawMessage(`["\t"]`)
	assertFieldError(t, ValidatePracticeForPublish(practice, card), "stop_conditions")

	card.ClinicalSafety = ClinicalEscalate
	practice = validPractice()
	practice.ProfessionalEscalation = json.RawMessage(`["\n"]`)
	assertFieldError(t, ValidatePracticeForPublish(practice, card), "professional_escalation")
}

func TestValidatePracticeRequiresSafetyActionsForRestrictedAndEscalate(t *testing.T) {
	for _, safety := range []ClinicalSafety{ClinicalRestricted, ClinicalEscalate} {
		t.Run(string(safety), func(t *testing.T) {
			card := validCard()
			card.ID = 1
			card.ClinicalSafety = safety
			practice := validPractice()
			practice.StopConditions = json.RawMessage(`[]`)
			assertFieldError(t, ValidatePracticeForPublish(practice, card), "stop_conditions")

			practice = validPractice()
			practice.ProfessionalEscalation = json.RawMessage(`[]`)
			assertFieldError(t, ValidatePracticeForPublish(practice, card), "professional_escalation")
		})
	}
}

func TestValidatePracticeForPublishAcceptsGeneralPracticeWithoutSafetyActions(t *testing.T) {
	card := validCard()
	card.ID = 1
	practice := validPractice()
	practice.StopConditions = json.RawMessage(`[]`)
	practice.ProfessionalEscalation = json.RawMessage(`[]`)
	if err := ValidatePracticeForPublish(practice, card); err != nil {
		t.Fatalf("general practice should not require restricted safety actions: %v", err)
	}
}

func TestValidatePracticeForPublishBindsPracticeToCard(t *testing.T) {
	card := validCard()
	card.ID = 2
	assertFieldError(t, ValidatePracticeForPublish(validPractice(), card), "card_id")
}

func TestValidatePracticeForPublishCannotDowngradeParentCardSafety(t *testing.T) {
	card := validCard()
	card.ID = 1
	card.ClinicalSafety = ClinicalRestricted
	practice := validPractice()
	practice.StopConditions = json.RawMessage(`[]`)
	assertFieldError(t, ValidatePracticeForPublish(practice, card), "stop_conditions")
}

func TestValidateRelationRejectsSelfRelationAndInvalidConfidence(t *testing.T) {
	relation := validRelation()
	relation.ToCardID = relation.FromCardID
	assertFieldError(t, ValidateRelation(relation), "to_card_id")

	relation = validRelation()
	relation.Confidence = -0.01
	assertFieldError(t, ValidateRelation(relation), "confidence")
}

func TestValidateCardSourceRejectsInvalidIDsPageRangeAndQuality(t *testing.T) {
	tests := []struct {
		name   string
		mutate func(*CardSource)
		field  string
	}{
		{name: "card id", mutate: func(source *CardSource) { source.CardID = 0 }, field: "card_id"},
		{name: "work id", mutate: func(source *CardSource) { source.WorkID = 0 }, field: "work_id"},
		{name: "page start", mutate: func(source *CardSource) { page := 0; source.PageStart = &page }, field: "page_start"},
		{name: "page end", mutate: func(source *CardSource) { page := 0; source.PageEnd = &page }, field: "page_end"},
		{name: "page order", mutate: func(source *CardSource) { start, end := 5, 4; source.PageStart, source.PageEnd = &start, &end }, field: "page_end"},
		{name: "quality", mutate: func(source *CardSource) { source.ExtractionQuality = -0.1 }, field: "extraction_quality"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			source := validCardSource()
			tt.mutate(&source)
			assertFieldError(t, ValidateCardSource(source), tt.field)
		})
	}
}

func TestValidateChunkChecksEnumsAuthorityAndOwnership(t *testing.T) {
	chunk := validChunk()
	chunk.Status = ChunkStatus("unknown")
	assertFieldError(t, ValidateChunk(chunk), "status")

	chunk = validChunk()
	chunk.AuthorityLevel = 7
	assertFieldError(t, ValidateChunk(chunk), "authority_level")

	chunk = validChunk()
	chunk.ChunkKind = ChunkKindPractice
	assertFieldError(t, ValidateChunk(chunk), "practice_id")

	chunk = validChunk()
	practiceID := int64(3)
	chunk.PracticeID = &practiceID
	assertFieldError(t, ValidateChunk(chunk), "practice_id")
}

func TestValidateEmbeddingRecordChecksDimensionsAndStatus(t *testing.T) {
	record := validEmbeddingRecord()
	record.Dimensions = 768
	assertFieldError(t, ValidateEmbeddingRecord(record), "dimensions")

	record = validEmbeddingRecord()
	record.Status = EmbeddingStatus("unknown")
	assertFieldError(t, ValidateEmbeddingRecord(record), "status")

	record = validEmbeddingRecord()
	record.Status = EmbeddingStatusReady
	assertFieldError(t, ValidateEmbeddingRecord(record), "embedding")
}

func TestValidateEmbeddingRecordRejectsNonFiniteReadyVector(t *testing.T) {
	for _, tt := range []struct {
		name  string
		value float32
	}{
		{name: "nan", value: float32(math.NaN())},
		{name: "positive infinity", value: float32(math.Inf(1))},
		{name: "negative infinity", value: float32(math.Inf(-1))},
	} {
		t.Run(tt.name, func(t *testing.T) {
			record := validEmbeddingRecord()
			record.Status = EmbeddingStatusReady
			record.Embedding = make([]float32, 1536)
			record.Embedding[17] = tt.value
			assertFieldError(t, ValidateEmbeddingRecord(record), "embedding")
		})
	}
}

func validSourceWork() SourceWork {
	return SourceWork{
		LibraryID:       1,
		CanonicalKey:    "observer-source",
		Title:           "内在观察者",
		WorkType:        WorkTypeBook,
		AuthorityLevel:  3,
		EpistemicStatus: EpistemicSourceText,
		CopyrightScope:  CopyrightMetadataOnly,
		Status:          SourceWorkStatusRegistered,
	}
}

func validSourceFile() SourceFile {
	return SourceFile{
		WorkID:            1,
		RelativePath:      "books/observer.pdf",
		OriginalFilename:  "observer.pdf",
		FileFormat:        "pdf",
		SHA256:            strings.Repeat("a", 64),
		TitleSource:       TitleSourceFilename,
		ExtractionClass:   ExtractionClassTextRich,
		ExtractionStatus:  ExtractionStatusExtracted,
		ExtractionQuality: 0.9,
	}
}

func validCard() Card {
	return Card{
		LibraryID:            1,
		CanonicalKey:         "observer",
		CanonicalName:        "内在观察者",
		CardKind:             CardKindConcept,
		Definition:           "观察自动化心理活动的能力。",
		ApplicableContext:    "一般自我观察。",
		NonApplicableContext: "不用于人格诊断。",
		EpistemicStatus:      EpistemicEvidenceInformed,
		EvidenceLevel:        EvidenceModerate,
		ClinicalSafety:       ClinicalGeneral,
		AuthorityLevel:       4,
		Status:               StatusDraft,
		Version:              1,
	}
}

func validPractice() Practice {
	return Practice{
		CardID:                 1,
		Goal:                   "练习观察",
		Steps:                  json.RawMessage(`["暂停并观察"]`),
		ReflectionPrompts:      json.RawMessage(`["注意到了什么？"]`),
		ExpectedFeedback:       json.RawMessage(`["能够命名体验"]`),
		StopConditions:         json.RawMessage(`["明显不适时停止"]`),
		ProfessionalEscalation: json.RawMessage(`["危机时联系专业人员"]`),
		PracticeSchemaVersion:  PracticeSchemaV1,
		Status:                 StatusDraft,
		Version:                1,
	}
}

func validRelation() Relation {
	return Relation{FromCardID: 1, ToCardID: 2, RelationType: RelationSupports, Confidence: 0.8, Status: RelationStatusDraft}
}

func validCardSource() CardSource {
	return CardSource{CardID: 1, WorkID: 1, SourceRole: SourceRolePrimary, ExtractionQuality: 0.9}
}

func validChunk() Chunk {
	return Chunk{
		LibraryID:      1,
		CardID:         1,
		ChunkKey:       "observer-card",
		ChunkKind:      ChunkKindCard,
		Title:          "内在观察者",
		Content:        "观察自动化心理活动的能力。",
		AuthorityLevel: 4,
		EvidenceLevel:  EvidenceModerate,
		ClinicalSafety: ClinicalGeneral,
		ContentHash:    strings.Repeat("b", 64),
		Version:        1,
		Status:         ChunkStatusEnabled,
	}
}

func validEmbeddingRecord() EmbeddingRecord {
	return EmbeddingRecord{
		ChunkID:        1,
		EmbeddingModel: "text-embedding-3-small",
		Dimensions:     1536,
		ContentHash:    strings.Repeat("b", 64),
		Status:         EmbeddingStatusPending,
	}
}

func assertFieldError(t *testing.T, err error, field string) {
	t.Helper()
	if err == nil {
		t.Fatalf("expected %s validation error", field)
	}
	assertErrorContains(t, err, field)
}

func assertErrorContains(t *testing.T, err error, want string) {
	t.Helper()
	if err == nil || !strings.Contains(err.Error(), want) {
		t.Fatalf("error %v does not contain %q", err, want)
	}
}
