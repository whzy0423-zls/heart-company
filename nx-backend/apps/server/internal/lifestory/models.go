// Package lifestory contains the private, user-scoped life-story domain.
//
// The package deliberately keeps story content as structured JSON-compatible
// values.  This lets the API and the asynchronous generator evolve without
// coupling the feature to ordinary chat sessions or SSE streams.
package lifestory

import (
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"unicode/utf8"
)

type StoryStatus string

const (
	StatusDraft         StoryStatus = "draft"
	StatusOutlineReady  StoryStatus = "outline_ready"
	StatusQueued        StoryStatus = "queued"
	StatusGenerating    StoryStatus = "generating"
	StatusCompleted     StoryStatus = "completed"
	StatusFailed        StoryStatus = "failed"
	StatusCancelled     StoryStatus = "cancelled"
	StatusSafetyBlocked StoryStatus = "safety_blocked"
)

func (s StoryStatus) Valid() bool {
	switch s {
	case StatusDraft, StatusOutlineReady, StatusQueued, StatusGenerating,
		StatusCompleted, StatusFailed, StatusCancelled, StatusSafetyBlocked:
		return true
	default:
		return false
	}
}

// Stage is intentionally separate from generation job status. A story may be
// in the outline stage while a previous job is failed, and a queued job may
// still refer to the confirmed outline stage.
type StoryStage string

const (
	StageDraft         StoryStage = "draft"
	StageCapture       StoryStage = "capture"
	StageQuestions     StoryStage = "questions"
	StageFacts         StoryStage = "facts"
	StageOutline       StoryStage = "outline"
	StageOutlineReady  StoryStage = "outline_ready"
	StageQueued        StoryStage = "queued"
	StageGeneration    StoryStage = "generation"
	StageGenerating    StoryStage = "generating"
	StageCompleted     StoryStage = "completed"
	StageFailed        StoryStage = "failed"
	StageCancelled     StoryStage = "cancelled"
	StageReading       StoryStage = "reading"
	StageSafetyBlocked StoryStage = "safety_blocked"
)

func (s StoryStage) Valid() bool {
	switch s {
	case StageDraft, StageCapture, StageQuestions, StageFacts, StageOutline,
		StageOutlineReady, StageQueued, StageGeneration, StageGenerating,
		StageCompleted, StageFailed, StageCancelled, StageReading, StageSafetyBlocked:
		return true
	default:
		return false
	}
}

type MaterialSourceType string

const (
	MaterialText  MaterialSourceType = "text"
	MaterialVoice MaterialSourceType = "voice"
)

type ASRStatus string

const (
	ASRNotApplicable ASRStatus = "not_applicable"
	ASRPending       ASRStatus = "pending"
	ASRQueued        ASRStatus = "queued"
	ASRProcessing    ASRStatus = "processing"
	ASRReady         ASRStatus = "ready"
	ASRCompleted     ASRStatus = "completed"
	ASRFailed        ASRStatus = "failed"
)

type JobStatus string

const (
	JobQueued        JobStatus = "queued"
	JobRunning       JobStatus = "running"
	JobSucceeded     JobStatus = "succeeded"
	JobFailed        JobStatus = "failed"
	JobCancelled     JobStatus = "cancelled"
	JobSafetyBlocked JobStatus = "safety_blocked"
)

type VersionStatus string

const (
	VersionDraft      VersionStatus = "draft"
	VersionPublished  VersionStatus = "published"
	VersionSuperseded VersionStatus = "superseded"
	VersionBlocked    VersionStatus = "blocked"
)

type Perspective string

const (
	PerspectiveFirst Perspective = "first_person"
	PerspectiveThird Perspective = "third_person"
)

type Tone string

const (
	ToneWarm    Tone = "warm"
	TonePlain   Tone = "plain"
	ToneHealing Tone = "healing"
)

type StoryStyle string

const (
	StoryStyleRealistic StoryStyle = "realistic"
	StoryStyleNovel     StoryStyle = "novel"
	StoryStyleFairyTale StoryStyle = "fairy_tale"
	StoryStyleMyth      StoryStyle = "myth"
)

func (s StoryStyle) Valid() bool {
	switch s {
	case StoryStyleRealistic, StoryStyleNovel, StoryStyleFairyTale, StoryStyleMyth:
		return true
	default:
		return false
	}
}

// NormalizeStoryStyle keeps snapshots and clients created before story styles
// compatible while rejecting unknown non-empty values.
func NormalizeStoryStyle(value StoryStyle) (StoryStyle, error) {
	value = StoryStyle(strings.TrimSpace(string(value)))
	if value == "" {
		return StoryStyleRealistic, nil
	}
	if !value.Valid() {
		return "", fmt.Errorf("story style is invalid")
	}
	return value, nil
}

func resolveStoryStyleForWrite(incoming, existing StoryStyle) (StoryStyle, error) {
	if strings.TrimSpace(string(incoming)) == "" {
		incoming = existing
	}
	return NormalizeStoryStyle(incoming)
}

type Story struct {
	ID               int64            `json:"id"`
	AppUserID        int64            `json:"appUserId"`
	Title            string           `json:"title"`
	Status           StoryStatus      `json:"status"`
	Stage            StoryStage       `json:"stage"`
	Summary          string           `json:"summary,omitempty"`
	MaterialCount    int              `json:"materialCount"`
	Materials        []Material       `json:"materials,omitempty"`
	FactCard         FactCard         `json:"factCard"`
	Outline          Outline          `json:"outline"`
	CurrentVersionID int64            `json:"currentVersionId,omitempty"`
	CurrentVersion   *Version         `json:"currentVersion,omitempty"`
	Versions         []Version        `json:"versions,omitempty"`
	LatestJob        *Job             `json:"latestJob,omitempty"`
	Jobs             []Job            `json:"jobs,omitempty"`
	Progress         *ReadingProgress `json:"progress,omitempty"`
	StoryRemaining   int              `json:"storyRemaining"`
	IsFavorite       bool             `json:"isFavorite"`
	CreatedAt        string           `json:"createdAt"`
	UpdatedAt        string           `json:"updatedAt"`
	DeletedAt        string           `json:"deletedAt,omitempty"`
	Revision         int64            `json:"revision"`
	DraftVersion     int64            `json:"draftVersion"`
}

type Material struct {
	ID              int64              `json:"id"`
	StoryID         int64              `json:"storyId,omitempty"`
	SourceType      MaterialSourceType `json:"sourceType"`
	Sequence        int                `json:"sequence"`
	Text            string             `json:"text,omitempty"`
	Transcript      string             `json:"transcript,omitempty"`
	ASRStatus       ASRStatus          `json:"asrStatus"`
	DurationSeconds int                `json:"durationSeconds,omitempty"`
	DurationMs      int                `json:"durationMs,omitempty"`
	ByteLength      int                `json:"byteLength,omitempty"`
	InputHash       string             `json:"inputHash,omitempty"`
	ErrorCategory   string             `json:"errorCategory,omitempty"`
	CreatedAt       string             `json:"createdAt"`
	UpdatedAt       string             `json:"updatedAt"`
}

type Question struct {
	ID         string `json:"id"`
	Prompt     string `json:"prompt"`
	Sequence   int    `json:"sequence"`
	Required   bool   `json:"required"`
	Answer     string `json:"answer,omitempty"`
	Skipped    bool   `json:"skipped,omitempty"`
	AnsweredAt string `json:"answeredAt,omitempty"`
}

type FactCharacter struct {
	ID            string `json:"id,omitempty"`
	Name          string `json:"name"`
	Relation      string `json:"relation,omitempty"`
	Role          string `json:"role,omitempty"`
	Alias         string `json:"alias,omitempty"`
	RealName      string `json:"realName,omitempty"`
	Description   string `json:"description,omitempty"`
	PrivacyMode   string `json:"privacyMode,omitempty"`
	RedactionMode string `json:"redactionMode,omitempty"`
}

type FactEvent struct {
	ID            string   `json:"id,omitempty"`
	Time          string   `json:"time,omitempty"`
	Location      string   `json:"location,omitempty"`
	Description   string   `json:"description"`
	TurningPoint  string   `json:"turningPoint,omitempty"`
	Outcome       string   `json:"outcome,omitempty"`
	People        []string `json:"people,omitempty"`
	Confirmed     bool     `json:"confirmed"`
	RedactionMode string   `json:"redactionMode,omitempty"`
}

type FactOrganization struct {
	ID            string `json:"id,omitempty"`
	Name          string `json:"name"`
	Alias         string `json:"alias,omitempty"`
	RedactionMode string `json:"redactionMode,omitempty"`
}

type FactCard struct {
	Characters      []FactCharacter    `json:"characters,omitempty"`
	Events          []FactEvent        `json:"events,omitempty"`
	Timeline        []FactEvent        `json:"timeline,omitempty"`
	Organizations   []FactOrganization `json:"organizations,omitempty"`
	Setting         string             `json:"setting,omitempty"`
	Conflict        string             `json:"conflict,omitempty"`
	TurningPoint    string             `json:"turningPoint,omitempty"`
	CentralQuestion string             `json:"centralQuestion,omitempty"`
	Ending          string             `json:"ending,omitempty"`
	Unresolved      string             `json:"unresolved,omitempty"`
	Questions       []Question         `json:"questions,omitempty"`
	Perspective     Perspective        `json:"perspective,omitempty"`
	Tone            Tone               `json:"tone,omitempty"`
	QuestionSetID   string             `json:"questionSetId,omitempty"`
	Version         int64              `json:"version"`
	Confirmed       bool               `json:"confirmed"`
	ConfirmedAt     string             `json:"confirmedAt,omitempty"`
}

type OutlineChapter struct {
	Order    int      `json:"order"`
	Title    string   `json:"title"`
	Summary  string   `json:"summary,omitempty"`
	Beat     string   `json:"beat,omitempty"`
	KeyBeats []string `json:"keyBeats,omitempty"`
}

type Outline struct {
	Perspective        Perspective      `json:"perspective"`
	Tone               Tone             `json:"tone"`
	StoryStyle         StoryStyle       `json:"storyStyle"`
	StoryStyleSelected bool             `json:"storyStyleSelected"`
	Chapters           []OutlineChapter `json:"chapters"`
	Confirmed          bool             `json:"confirmed"`
	ConfirmedAt        string           `json:"confirmedAt,omitempty"`
	Version            int64            `json:"version"`
}

func (o *Outline) UnmarshalJSON(raw []byte) error {
	type plainOutline Outline
	var decoded plainOutline
	if err := json.Unmarshal(raw, &decoded); err != nil {
		return err
	}
	var fields map[string]json.RawMessage
	if err := json.Unmarshal(raw, &fields); err != nil {
		return err
	}
	if _, ok := fields["storyStyle"]; !ok {
		if value, exists := fields["story_style"]; exists {
			if err := json.Unmarshal(value, &decoded.StoryStyle); err != nil {
				return err
			}
		}
	}
	selectedRaw, selectedPresent := fields["storyStyleSelected"]
	if !selectedPresent {
		selectedRaw, selectedPresent = fields["story_style_selected"]
	}
	if selectedPresent {
		if err := json.Unmarshal(selectedRaw, &decoded.StoryStyleSelected); err != nil {
			return err
		}
	} else {
		// Before the presence flag existed, sending storyStyle was the only
		// way a client could express an explicit selection.
		decoded.StoryStyleSelected = strings.TrimSpace(string(decoded.StoryStyle)) != ""
	}
	*o = Outline(decoded)
	return nil
}

func normalizeOutlineStoryStyle(outline *Outline) error {
	style, err := NormalizeStoryStyle(outline.StoryStyle)
	if err != nil {
		return err
	}
	outline.StoryStyle = style
	return nil
}

func resolveOutlineStoryStyleForWrite(incoming, existing Outline) (Outline, error) {
	if err := normalizeOutlineStoryStyle(&incoming); err != nil {
		return Outline{}, err
	}
	if err := normalizeOutlineStoryStyle(&existing); err != nil {
		return Outline{}, err
	}
	if incoming.StoryStyleSelected {
		return incoming, nil
	}
	if existing.StoryStyleSelected {
		incoming.StoryStyle = existing.StoryStyle
		incoming.StoryStyleSelected = true
		return incoming, nil
	}
	incoming.StoryStyle = StoryStyleRealistic
	return incoming, nil
}

type Chapter struct {
	Order   int    `json:"order"`
	Title   string `json:"title"`
	Summary string `json:"summary,omitempty"`
	Body    string `json:"body"`
}

type Version struct {
	ID               int64           `json:"id,omitempty"`
	StoryID          int64           `json:"storyId,omitempty"`
	Number           int             `json:"number"`
	Status           VersionStatus   `json:"status"`
	Perspective      Perspective     `json:"perspective"`
	Tone             Tone            `json:"tone"`
	StoryStyle       StoryStyle      `json:"storyStyle"`
	Chapters         []Chapter       `json:"chapters"`
	Reflection       string          `json:"reflection"`
	CharacterCount   int             `json:"characterCount"`
	WordCount        int             `json:"wordCount,omitempty"`
	Model            string          `json:"model,omitempty"`
	GenerationConfig json.RawMessage `json:"generationConfig,omitempty"`
	CreatedAt        string          `json:"createdAt,omitempty"`
}

type Job struct {
	ID              int64           `json:"id"`
	StoryID         int64           `json:"storyId,omitempty"`
	AppUserID       int64           `json:"appUserId,omitempty"`
	RequestKey      string          `json:"requestKey"`
	PayloadHash     string          `json:"-"`
	SnapshotHash    string          `json:"-"`
	SourceVersionID int64           `json:"sourceVersionId,omitempty"`
	Status          JobStatus       `json:"status"`
	Attempt         int             `json:"attempt"`
	MaxAttempts     int             `json:"maxAttempts,omitempty"`
	Progress        int             `json:"progress,omitempty"`
	ErrorCategory   string          `json:"errorCategory,omitempty"`
	ErrorMessage    string          `json:"errorMessage,omitempty"`
	VersionID       int64           `json:"versionId,omitempty"`
	ClaimToken      string          `json:"-"`
	ClaimedAt       string          `json:"claimedAt,omitempty"`
	LeaseUntil      string          `json:"leaseUntil,omitempty"`
	WorkerID        string          `json:"workerId,omitempty"`
	RetryAfter      string          `json:"retryAfter,omitempty"`
	InputSnapshot   json.RawMessage `json:"-"`
	CreatedAt       string          `json:"createdAt"`
	StartedAt       string          `json:"startedAt,omitempty"`
	FinishedAt      string          `json:"finishedAt,omitempty"`
}

type ReadingProgress struct {
	StoryID   int64 `json:"storyId"`
	VersionID int64 `json:"versionId"`
	// ChapterIndex is the canonical zero-based code-point pagination index.
	ChapterIndex int `json:"chapterIndex"`
	// ChapterOrder is retained for older callers and database rows. It is
	// one-based; JSON always exposes the canonical chapterIndex field.
	ChapterOrder    int    `json:"-"`
	CharacterOffset int    `json:"characterOffset"`
	Completed       bool   `json:"completed"`
	ClientUpdatedAt string `json:"clientUpdatedAt,omitempty"`
	UpdatedAt       string `json:"updatedAt,omitempty"`
}

func (p ReadingProgress) EffectiveChapterIndex() int {
	if p.ChapterIndex > 0 || p.ChapterOrder <= 0 {
		return p.ChapterIndex
	}
	return p.ChapterOrder - 1
}

func (p ReadingProgress) MarshalJSON() ([]byte, error) {
	type payload struct {
		StoryID         int64  `json:"storyId"`
		VersionID       int64  `json:"versionId"`
		ChapterIndex    int    `json:"chapterIndex"`
		ChapterOrder    int    `json:"chapterOrder"`
		CharacterOffset int    `json:"characterOffset"`
		Completed       bool   `json:"completed"`
		ClientUpdatedAt string `json:"clientUpdatedAt,omitempty"`
		UpdatedAt       string `json:"updatedAt,omitempty"`
	}
	index := p.EffectiveChapterIndex()
	return json.Marshal(payload{StoryID: p.StoryID, VersionID: p.VersionID,
		ChapterIndex: index, ChapterOrder: index + 1, CharacterOffset: p.CharacterOffset,
		Completed: p.Completed, ClientUpdatedAt: p.ClientUpdatedAt, UpdatedAt: p.UpdatedAt})
}

func (p *ReadingProgress) UnmarshalJSON(raw []byte) error {
	type payload struct {
		StoryID         int64  `json:"storyId"`
		VersionID       int64  `json:"versionId"`
		ChapterIndex    *int   `json:"chapterIndex"`
		ChapterOrder    *int   `json:"chapterOrder"`
		CharacterOffset int    `json:"characterOffset"`
		Completed       bool   `json:"completed"`
		ClientUpdatedAt string `json:"clientUpdatedAt"`
		UpdatedAt       string `json:"updatedAt"`
	}
	var value payload
	if err := json.Unmarshal(raw, &value); err != nil {
		return err
	}
	index := 0
	if value.ChapterIndex != nil {
		index = *value.ChapterIndex
	} else if value.ChapterOrder != nil {
		index = *value.ChapterOrder - 1
	}
	if index < 0 {
		index = 0
	}
	p.StoryID, p.VersionID, p.ChapterIndex, p.ChapterOrder = value.StoryID, value.VersionID, index, index+1
	p.CharacterOffset, p.Completed = value.CharacterOffset, value.Completed
	p.ClientUpdatedAt, p.UpdatedAt = value.ClientUpdatedAt, value.UpdatedAt
	return nil
}

func (v Version) CharacterCountValue() int {
	count := utf8.RuneCountInString(v.Reflection)
	for _, chapter := range v.Chapters {
		count += utf8.RuneCountInString(chapter.Body)
	}
	return count
}

func (v Version) WordCountValue() int {
	count := 0
	for _, chapter := range v.Chapters {
		count += len(strings.Fields(chapter.Body))
	}
	return count
}

// ValidateVersion enforces the public generation contract. Bounds are based
// on Unicode code points, which are also the offsets persisted by the reader.
func ValidateVersion(v Version) error {
	if _, err := NormalizeStoryStyle(v.StoryStyle); err != nil {
		return err
	}
	if len(v.Chapters) < 4 || len(v.Chapters) > 6 {
		return fmt.Errorf("story must contain 4-6 chapters")
	}
	for i, chapter := range v.Chapters {
		if strings.TrimSpace(chapter.Title) == "" {
			return fmt.Errorf("chapter %d title is required", i+1)
		}
		if strings.TrimSpace(chapter.Body) == "" {
			return fmt.Errorf("chapter %d body is required", i+1)
		}
		if chapter.Order != i+1 {
			return fmt.Errorf("chapter %d order must be %d", i+1, i+1)
		}
	}
	if strings.TrimSpace(v.Reflection) == "" {
		return errors.New("growth reflection is required")
	}
	count := 0
	for _, chapter := range v.Chapters {
		count += utf8.RuneCountInString(chapter.Body)
	}
	if count < 2500 || count > 3500 {
		return fmt.Errorf("story character count %d outside 2500-3500", count)
	}
	return nil
}

// ValidateOutline enforces the confirmation contract without constraining
// drafts.  Draft outlines may be partial; once the user confirms, the server
// requires a bounded chapter list with stable order and usable labels.
func ValidateOutline(o Outline) error {
	if len(o.Chapters) < 4 || len(o.Chapters) > 6 {
		return fmt.Errorf("outline must contain 4-6 chapters")
	}
	if o.Perspective != "" && o.Perspective != PerspectiveFirst && o.Perspective != PerspectiveThird {
		return fmt.Errorf("outline perspective is invalid")
	}
	if o.Tone != "" && o.Tone != ToneWarm && o.Tone != TonePlain && o.Tone != ToneHealing {
		return fmt.Errorf("outline tone is invalid")
	}
	if _, err := NormalizeStoryStyle(o.StoryStyle); err != nil {
		return fmt.Errorf("outline %w", err)
	}
	for i, chapter := range o.Chapters {
		if chapter.Order != i+1 {
			return fmt.Errorf("outline chapter %d order must be %d", i+1, i+1)
		}
		if strings.TrimSpace(chapter.Title) == "" {
			return fmt.Errorf("outline chapter %d title is required", i+1)
		}
		if len([]rune(chapter.Title)) > 120 {
			return fmt.Errorf("outline chapter %d title is too long", i+1)
		}
		if len([]rune(chapter.Summary)) > 800 {
			return fmt.Errorf("outline chapter %d summary is too long", i+1)
		}
		if len(chapter.KeyBeats) > 12 {
			return fmt.Errorf("outline chapter %d has too many key beats", i+1)
		}
		for _, beat := range chapter.KeyBeats {
			if len([]rune(beat)) > 300 {
				return fmt.Errorf("outline chapter %d key beat is too long", i+1)
			}
		}
	}
	return nil
}

func (f FactCard) MarshalJSON() ([]byte, error) {
	type alias FactCard
	return json.Marshal(alias(f))
}

func NormalizeStoryStatus(value string) (StoryStatus, error) {
	status := StoryStatus(strings.TrimSpace(value))
	if !status.Valid() {
		return "", fmt.Errorf("invalid story status %q", value)
	}
	return status, nil
}
