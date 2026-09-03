package lifestory

import (
	"context"
	"crypto/rand"
	"database/sql"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"time"

	"nine-xing/nx-backend/apps/server/internal/dbtx"
)

var (
	ErrNilDB            = errors.New("life story: database is nil")
	ErrNotFound         = errors.New("life story: not found")
	ErrConflict         = errors.New("life story: revision conflict")
	ErrPayloadConflict  = errors.New("life story: request payload conflict")
	ErrInvalidState     = errors.New("life story: invalid state")
	ErrInactiveUser     = errors.New("life story: user is inactive")
	ErrQuotaExhausted   = errors.New("life story: quota exhausted")
	ErrTooManyQuestions = errors.New("life story: too many questions")
	ErrValidation       = errors.New("life story: validation failed")
)

// maxFactLocationCodePoints is the public limit for newly entered event
// locations.  Existing rows may predate this limit; validateFactCardLocations
// permits those values only when the caller carries the exact historical value
// forward, so unrelated fact edits remain possible without truncating data.
const maxFactLocationCodePoints = 120

// validateFactCardLocations applies the location length rule to both event
// collections.  An over-limit incoming value is treated as a legacy value
// only when it matches the current value for the same stable event ID.  Older
// clients did not send IDs, so those rows fall back to their collection index.
func validateFactCardLocations(incoming, current FactCard) error {
	if err := validateFactEventLocations("events", incoming.Events, current.Events); err != nil {
		return err
	}
	return validateFactEventLocations("timeline", incoming.Timeline, current.Timeline)
}

func validateFactEventLocations(name string, incoming, current []FactEvent) error {
	for index, event := range incoming {
		if len([]rune(event.Location)) <= maxFactLocationCodePoints {
			continue
		}
		oldLocation, found := historicalFactEventLocation(event, index, current)
		if !found || oldLocation != event.Location {
			return fmt.Errorf("%w: %s location exceeds %d Unicode code points", ErrValidation, name, maxFactLocationCodePoints)
		}
	}
	return nil
}

func historicalFactEventLocation(incoming FactEvent, index int, current []FactEvent) (string, bool) {
	if strings.TrimSpace(incoming.ID) != "" {
		for _, event := range current {
			if strings.TrimSpace(event.ID) == strings.TrimSpace(incoming.ID) {
				return event.Location, true
			}
		}
		return "", false
	}
	if index < 0 || index >= len(current) {
		return "", false
	}
	return current[index].Location, true
}

type Store struct {
	db       *sql.DB
	tokenKey []byte
}

func nullableInt64(value int64) any {
	if value <= 0 {
		return nil
	}
	return value
}

func NewStore(db *sql.DB) *Store { return &Store{db: db, tokenKey: tokenKeyBytes("")} }

// NewStoreWithTokenKey derives an application-scoped AES key for reversible
// PII token maps. The key is never serialized into story rows.
func NewStoreWithTokenKey(db *sql.DB, secret string) *Store {
	return &Store{db: db, tokenKey: tokenKeyBytes(secret)}
}

func (s *Store) SetTokenKey(secret string) {
	if s != nil {
		s.tokenKey = tokenKeyBytes(secret)
	}
}

func (s *Store) queryTarget() (dbtx.DBTX, error) {
	if s == nil || s.db == nil {
		return nil, ErrNilDB
	}
	return s.db, nil
}

type CreateStoryInput struct {
	Title     string     `json:"title"`
	Materials []Material `json:"materials,omitempty"`
}

type DraftInput struct {
	Title     string     `json:"title"`
	Materials []Material `json:"materials,omitempty"`
}

type StorySnapshot struct {
	StoryID             int64      `json:"storyId"`
	RequestKey          string     `json:"requestKey,omitempty"`
	Materials           []Material `json:"materials"`
	FactCard            FactCard   `json:"factCard"`
	Outline             Outline    `json:"outline"`
	RevisionInstruction string     `json:"revisionInstruction,omitempty"`
	FactsVersion        int64      `json:"factsVersion"`
	OutlineVersion      int64      `json:"outlineVersion"`
	SourceVersionID     int64      `json:"sourceVersionId,omitempty"`
}

type GenerationInput struct {
	RequestKey      string `json:"requestKey"`
	FactsVersion    int64  `json:"factsVersion"`
	OutlineVersion  int64  `json:"outlineVersion"`
	SourceVersionID int64  `json:"sourceVersionId,omitempty"`
	Instruction     string `json:"instruction,omitempty"`
}

func normalizeTitle(title string) string {
	title = strings.TrimSpace(title)
	if title == "" {
		return "我的故事"
	}
	if len([]rune(title)) > 80 {
		return string([]rune(title)[:80])
	}
	return title
}

func normalizeMaterial(m Material, sequence int) (Material, error) {
	m.SourceType = MaterialSourceType(strings.TrimSpace(string(m.SourceType)))
	if m.SourceType == "" {
		m.SourceType = MaterialText
	}
	if m.SourceType != MaterialText && m.SourceType != MaterialVoice {
		return Material{}, fmt.Errorf("invalid material source type")
	}
	m.Sequence = sequence
	m.Text = strings.TrimSpace(m.Text)
	m.Transcript = strings.TrimSpace(m.Transcript)
	if m.SourceType == MaterialVoice {
		if m.DurationMs > 60_000 || m.DurationMs < 0 {
			return Material{}, fmt.Errorf("voice material duration must be 0-60 seconds")
		}
		if m.DurationSeconds == 0 && m.DurationMs > 0 {
			m.DurationSeconds = (m.DurationMs + 999) / 1000
		}
		if m.DurationMs == 0 && m.DurationSeconds > 0 {
			m.DurationMs = m.DurationSeconds * 1000
		}
		if m.ByteLength < 0 || m.ByteLength > 10*1024*1024 {
			return Material{}, fmt.Errorf("%w: voice material is too large", ErrValidation)
		}
		if m.ASRStatus == "" || m.ASRStatus == ASRNotApplicable {
			m.ASRStatus = ASRCompleted
		}
		if m.DurationSeconds < 0 || m.DurationSeconds > 60 {
			return Material{}, fmt.Errorf("voice material duration must be 0-60 seconds")
		}
		if m.Text == "" && m.Transcript != "" {
			m.Text = m.Transcript
		}
	} else {
		m.ASRStatus = ASRNotApplicable
		m.DurationSeconds = 0
		m.DurationMs = 0
		m.ByteLength = 0
		if m.Transcript == "" {
			m.Transcript = m.Text
		}
	}
	if m.Text == "" && m.Transcript == "" {
		return Material{}, fmt.Errorf("material text is required")
	}
	if len([]rune(m.Text)) > 20_000 || len([]rune(m.Transcript)) > 20_000 {
		return Material{}, fmt.Errorf("material is too long")
	}
	return m, nil
}

func normalizeMaterials(materials []Material) ([]Material, error) {
	if len(materials) > 40 {
		return nil, fmt.Errorf("too many materials")
	}
	result := make([]Material, 0, len(materials))
	total := 0
	for i, material := range materials {
		m, err := normalizeMaterial(material, i+1)
		if err != nil {
			return nil, err
		}
		total += len([]rune(m.Transcript))
		if total > 80_000 {
			return nil, fmt.Errorf("story materials are too long")
		}
		result = append(result, m)
	}
	return result, nil
}

func marshalJSON(value any) ([]byte, error) {
	raw, err := json.Marshal(value)
	if err != nil {
		return nil, err
	}
	if len(raw) == 0 {
		return []byte(`{}`), nil
	}
	return raw, nil
}

func nowString(t time.Time) string {
	if t.IsZero() {
		return ""
	}
	return t.UTC().Format(time.RFC3339Nano)
}

func scanStory(row interface{ Scan(...any) error }) (Story, error) {
	var story Story
	var factRaw, outlineRaw []byte
	var created, updated, deleted sql.NullTime
	var currentVersionID sql.NullInt64
	if err := row.Scan(&story.ID, &story.AppUserID, &story.Title, &story.Summary,
		&story.Stage, &story.Status, &factRaw, &outlineRaw, &currentVersionID,
		&story.DraftVersion, &story.Revision, &story.IsFavorite, &created, &updated, &deleted); err != nil {
		return Story{}, err
	}
	if currentVersionID.Valid {
		story.CurrentVersionID = currentVersionID.Int64
	}
	if len(factRaw) > 0 {
		_ = json.Unmarshal(factRaw, &story.FactCard)
	}
	if len(outlineRaw) > 0 {
		_ = json.Unmarshal(outlineRaw, &story.Outline)
	}
	if err := normalizeOutlineStoryStyle(&story.Outline); err != nil {
		return Story{}, err
	}
	story.CreatedAt, story.UpdatedAt = nowString(created.Time), nowString(updated.Time)
	if deleted.Valid {
		story.DeletedAt = nowString(deleted.Time)
	}
	if story.DraftVersion <= 0 {
		story.DraftVersion = story.Revision
	}
	return story, nil
}

const storyColumns = `id, app_user_id, title, summary, stage, status, fact_card, outline,
 current_version_id, draft_version, revision, is_favorite, created_at, updated_at, deleted_at`

// CreateStory creates a user-scoped draft and its ordered source materials.
func (s *Store) CreateStory(ctx context.Context, appUserID int64, input CreateStoryInput) (Story, error) {
	if appUserID <= 0 {
		return Story{}, fmt.Errorf("invalid app user id")
	}
	materials, err := normalizeMaterials(input.Materials)
	if err != nil {
		return Story{}, err
	}
	if _, err := s.queryTarget(); err != nil {
		return Story{}, err
	}
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return Story{}, err
	}
	defer tx.Rollback()
	var userStatus string
	if err := tx.QueryRowContext(ctx, `SELECT status FROM app_users WHERE id=$1 FOR UPDATE`, appUserID).Scan(&userStatus); errors.Is(err, sql.ErrNoRows) {
		return Story{}, ErrNotFound
	} else if err != nil {
		return Story{}, err
	}
	if strings.TrimSpace(userStatus) != "active" {
		return Story{}, ErrInactiveUser
	}
	var story Story
	row := tx.QueryRowContext(ctx, `INSERT INTO app_life_stories(app_user_id,title,stage,status)
VALUES($1,$2,'draft','draft') RETURNING `+storyColumns, appUserID, normalizeTitle(input.Title))
	if story, err = scanStory(row); err != nil {
		return Story{}, fmt.Errorf("create life story: %w", err)
	}
	if err := insertMaterials(ctx, tx, appUserID, story.ID, materials); err != nil {
		return Story{}, err
	}
	if err := tx.Commit(); err != nil {
		return Story{}, err
	}
	story.Materials = materials
	story.MaterialCount = len(materials)
	return story, nil
}

// Create is a short alias retained for callers that use other domain stores.
func (s *Store) Create(ctx context.Context, appUserID int64, input CreateStoryInput) (Story, error) {
	return s.CreateStory(ctx, appUserID, input)
}

func insertMaterials(ctx context.Context, q dbtx.DBTX, appUserID, storyID int64, materials []Material) error {
	for _, m := range materials {
		metadata, err := marshalJSON(map[string]any{})
		if err != nil {
			return err
		}
		if len(m.InputHash) > 128 {
			m.InputHash = m.InputHash[:128]
		}
		_, err = q.ExecContext(ctx, `INSERT INTO app_life_story_materials
	 (story_id,app_user_id,source_type,sequence,text,transcript,asr_status,duration_seconds,duration_ms,byte_length,input_hash,error_category,metadata)
	 VALUES($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11,$12,$13::jsonb)
	 ON CONFLICT(story_id,sequence) DO UPDATE SET source_type=EXCLUDED.source_type,
	 text=EXCLUDED.text, transcript=EXCLUDED.transcript, asr_status=EXCLUDED.asr_status,
	 duration_seconds=EXCLUDED.duration_seconds, duration_ms=EXCLUDED.duration_ms, byte_length=EXCLUDED.byte_length,
	 input_hash=EXCLUDED.input_hash, error_category=EXCLUDED.error_category, updated_at=now()`,
			storyID, appUserID, m.SourceType, m.Sequence, m.Text, m.Transcript, m.ASRStatus,
			m.DurationSeconds, m.DurationMs, m.ByteLength, m.InputHash, m.ErrorCategory, string(metadata))
		if err != nil {
			return fmt.Errorf("save life story material: %w", err)
		}
	}
	return nil
}

func (s *Store) saveTokenMapTx(ctx context.Context, tx dbtx.DBTX, appUserID, storyID, jobID int64, tokens TokenMap) error {
	if len(tokens) == 0 {
		return nil
	}
	key := s.tokenKey
	if len(key) == 0 {
		key = tokenKeyBytes("")
	}
	ciphertext, err := encryptTokenMap(tokens, key)
	if err != nil {
		return err
	}
	_, err = tx.ExecContext(ctx, `INSERT INTO app_life_story_token_maps(app_user_id,story_id,job_id,ciphertext,expires_at) VALUES($1,$2,$3,$4,now()+INTERVAL '30 days') ON CONFLICT(job_id) DO UPDATE SET ciphertext=EXCLUDED.ciphertext,expires_at=EXCLUDED.expires_at`, appUserID, storyID, jobID, ciphertext)
	return err
}

func (s *Store) loadJobTokenMap(ctx context.Context, job Job) (TokenMap, error) {
	if s == nil || s.db == nil {
		return nil, ErrNilDB
	}
	opaqueTokens := tokenWordPattern.FindAll(job.InputSnapshot, -1)
	if len(opaqueTokens) == 0 {
		return TokenMap{}, nil
	}
	var ciphertext []byte
	err := s.db.QueryRowContext(ctx, `SELECT ciphertext FROM app_life_story_token_maps
		WHERE job_id=$1 AND story_id=$2 AND app_user_id=$3 AND expires_at>now()`, job.ID, job.StoryID, job.AppUserID).Scan(&ciphertext)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, fmt.Errorf("life story token map is unavailable")
	}
	if err != nil {
		return nil, err
	}
	key := s.tokenKey
	if len(key) == 0 {
		key = tokenKeyBytes("")
	}
	tokens, err := decryptTokenMap(ciphertext, key)
	if err != nil {
		return nil, err
	}
	for _, rawToken := range opaqueTokens {
		if _, ok := tokens[string(rawToken)]; !ok {
			return nil, fmt.Errorf("life story token map is incomplete")
		}
	}
	return tokens, nil
}

func deleteJobTokenMapTx(ctx context.Context, tx dbtx.DBTX, jobID int64) error {
	_, err := tx.ExecContext(ctx, `DELETE FROM app_life_story_token_maps WHERE job_id=$1`, jobID)
	return err
}

// PurgeExpiredTokenMaps removes bounded batches of reversible PII mappings.
// Terminal jobs delete their map immediately; this handles abandoned legacy
// rows and jobs that never reached a terminal transition.
func (s *Store) PurgeExpiredTokenMaps(ctx context.Context, limit int) (int64, error) {
	if s == nil || s.db == nil {
		return 0, ErrNilDB
	}
	if limit <= 0 || limit > 1000 {
		limit = 1000
	}
	result, err := s.db.ExecContext(ctx, `WITH expired AS (
		SELECT id FROM app_life_story_token_maps
		WHERE expires_at<=now()
		ORDER BY expires_at,id
		LIMIT $1
		FOR UPDATE SKIP LOCKED
	)
	DELETE FROM app_life_story_token_maps token_map
	USING expired WHERE token_map.id=expired.id`, limit)
	if err != nil {
		return 0, err
	}
	return result.RowsAffected()
}

func (s *Store) List(ctx context.Context, appUserID int64) ([]Story, error) {
	q, err := s.queryTarget()
	if err != nil {
		return nil, err
	}
	rows, err := q.QueryContext(ctx, `SELECT `+storyColumns+` FROM app_life_stories
 WHERE app_user_id=$1 AND deleted_at IS NULL ORDER BY updated_at DESC,id DESC`, appUserID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	stories := make([]Story, 0)
	for rows.Next() {
		story, scanErr := scanStory(rows)
		if scanErr != nil {
			return nil, scanErr
		}
		var count int
		_ = q.QueryRowContext(ctx, `SELECT count(*) FROM app_life_story_materials WHERE story_id=$1 AND app_user_id=$2`, story.ID, appUserID).Scan(&count)
		story.MaterialCount = count
		stories = append(stories, story)
	}
	return stories, rows.Err()
}

func (s *Store) Get(ctx context.Context, appUserID, storyID int64) (Story, error) {
	q, err := s.queryTarget()
	if err != nil {
		return Story{}, err
	}
	story, err := scanStory(q.QueryRowContext(ctx, `SELECT `+storyColumns+` FROM app_life_stories
 WHERE id=$1 AND app_user_id=$2 AND deleted_at IS NULL`, storyID, appUserID))
	if errors.Is(err, sql.ErrNoRows) {
		return Story{}, ErrNotFound
	}
	if err != nil {
		return Story{}, err
	}
	materials, err := listMaterials(ctx, q, appUserID, storyID)
	if err != nil {
		return Story{}, err
	}
	story.Materials, story.MaterialCount = materials, len(materials)
	if story.CurrentVersionID > 0 {
		if version, versionErr := getVersionByID(ctx, q, appUserID, story.CurrentVersionID); versionErr == nil {
			story.CurrentVersion = &version
		}
	}
	if versions, versionsErr := listVersions(ctx, q, appUserID, storyID); versionsErr == nil {
		story.Versions = versions
	}
	if jobs, jobsErr := listJobs(ctx, q, appUserID, storyID); jobsErr == nil {
		story.Jobs = jobs
		if len(jobs) > 0 {
			story.LatestJob = &jobs[0]
		}
	}
	if progress, progressErr := getProgressWithDBTX(ctx, q, appUserID, storyID); progressErr == nil {
		story.Progress = &progress
	}
	return story, nil
}

func (s *Store) GetStory(ctx context.Context, appUserID, storyID int64) (Story, error) {
	return s.Get(ctx, appUserID, storyID)
}

func listMaterials(ctx context.Context, q dbtx.DBTX, appUserID, storyID int64) ([]Material, error) {
	rows, err := q.QueryContext(ctx, `SELECT id,story_id,source_type,sequence,text,transcript,asr_status,
	 duration_seconds,duration_ms,byte_length,input_hash,error_category,created_at,updated_at FROM app_life_story_materials
 WHERE story_id=$1 AND app_user_id=$2 ORDER BY sequence,id`, storyID, appUserID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	items := make([]Material, 0)
	for rows.Next() {
		var m Material
		var created, updated time.Time
		if err := rows.Scan(&m.ID, &m.StoryID, &m.SourceType, &m.Sequence, &m.Text, &m.Transcript, &m.ASRStatus, &m.DurationSeconds, &m.DurationMs, &m.ByteLength, &m.InputHash, &m.ErrorCategory, &created, &updated); err != nil {
			return nil, err
		}
		m.CreatedAt, m.UpdatedAt = nowString(created), nowString(updated)
		items = append(items, m)
	}
	return items, rows.Err()
}

// SaveDraft uses optimistic revision control. ExpectedRevision=0 means "do
// not enforce a revision" and is reserved for initial migration clients.
func (s *Store) SaveDraft(ctx context.Context, appUserID, storyID, expectedRevision int64, input DraftInput) (Story, error) {
	if appUserID <= 0 || storyID <= 0 {
		return Story{}, fmt.Errorf("invalid story id")
	}
	var materials []Material
	var err error
	if input.Materials != nil {
		materials, err = normalizeMaterials(input.Materials)
		if err != nil {
			return Story{}, err
		}
	}
	if s == nil || s.db == nil {
		return Story{}, ErrNilDB
	}
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return Story{}, err
	}
	defer tx.Rollback()
	var currentDraftVersion int64
	var currentTitle string
	var status StoryStatus
	if err := tx.QueryRowContext(ctx, `SELECT draft_version,title,status FROM app_life_stories WHERE id=$1 AND app_user_id=$2 AND deleted_at IS NULL FOR UPDATE`, storyID, appUserID).Scan(&currentDraftVersion, &currentTitle, &status); errors.Is(err, sql.ErrNoRows) {
		return Story{}, ErrNotFound
	} else if err != nil {
		return Story{}, err
	}
	if expectedRevision > 0 && expectedRevision != currentDraftVersion {
		return Story{}, ErrConflict
	}
	if status == StatusQueued || status == StatusGenerating {
		return Story{}, ErrInvalidState
	}
	if input.Materials == nil {
		materials, err = listMaterials(ctx, tx, appUserID, storyID)
		if err != nil {
			return Story{}, err
		}
	}
	if _, err := tx.ExecContext(ctx, `DELETE FROM app_life_story_materials WHERE story_id=$1 AND app_user_id=$2`, storyID, appUserID); err != nil {
		return Story{}, err
	}
	if err := insertMaterials(ctx, tx, appUserID, storyID, materials); err != nil {
		return Story{}, err
	}
	title := strings.TrimSpace(input.Title)
	if title == "" {
		title = currentTitle
	}
	if _, err := tx.ExecContext(ctx, `UPDATE app_life_stories SET title=$1,stage='draft',status='draft',draft_version=draft_version+1,revision=revision+1,updated_at=now() WHERE id=$2 AND app_user_id=$3`, normalizeTitle(title), storyID, appUserID); err != nil {
		return Story{}, err
	}
	story, err := scanStory(tx.QueryRowContext(ctx, `SELECT `+storyColumns+` FROM app_life_stories WHERE id=$1 AND app_user_id=$2`, storyID, appUserID))
	if err != nil {
		return Story{}, err
	}
	if err := tx.Commit(); err != nil {
		return Story{}, err
	}
	story.Materials, story.MaterialCount = materials, len(materials)
	return story, nil
}

func (s *Store) ReplaceMaterials(ctx context.Context, appUserID, storyID int64, materials []Material) ([]Material, error) {
	returnMaterials, err := normalizeMaterials(materials)
	if err != nil {
		return nil, err
	}
	if s == nil || s.db == nil {
		return nil, ErrNilDB
	}
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return nil, err
	}
	defer tx.Rollback()
	var exists int
	var storyStatus StoryStatus
	if err := tx.QueryRowContext(ctx, `SELECT 1,status FROM app_life_stories WHERE id=$1 AND app_user_id=$2 AND deleted_at IS NULL FOR UPDATE`, storyID, appUserID).Scan(&exists, &storyStatus); errors.Is(err, sql.ErrNoRows) {
		return nil, ErrNotFound
	} else if err != nil {
		return nil, err
	}
	if storyStatus == StatusQueued || storyStatus == StatusGenerating {
		return nil, ErrInvalidState
	}
	if _, err := tx.ExecContext(ctx, `DELETE FROM app_life_story_materials WHERE story_id=$1 AND app_user_id=$2`, storyID, appUserID); err != nil {
		return nil, err
	}
	if err := insertMaterials(ctx, tx, appUserID, storyID, returnMaterials); err != nil {
		return nil, err
	}
	if _, err := tx.ExecContext(ctx, `UPDATE app_life_stories SET draft_version=draft_version+1,revision=revision+1,updated_at=now() WHERE id=$1 AND app_user_id=$2`, storyID, appUserID); err != nil {
		return nil, err
	}
	if err := tx.Commit(); err != nil {
		return nil, err
	}
	return returnMaterials, nil
}

func (s *Store) SaveFactCard(ctx context.Context, appUserID, storyID int64, facts FactCard, expectedRevision int64) (Story, error) {
	// A PATCH edits facts only; confirmation is a separate explicit action.
	facts.Confirmed = false
	facts.ConfirmedAt = ""
	return s.updateStoryJSON(ctx, appUserID, storyID, expectedRevision, "fact_card", facts, StageFacts, StatusDraft)
}

func (s *Store) ConfirmFacts(ctx context.Context, appUserID, storyID int64, facts FactCard, expectedRevision int64) (Story, error) {
	facts.Confirmed = true
	facts.ConfirmedAt = nowString(time.Now())
	return s.updateStoryJSON(ctx, appUserID, storyID, expectedRevision, "fact_card", facts, StageOutline, StatusDraft)
}

func (s *Store) SaveOutline(ctx context.Context, appUserID, storyID int64, outline Outline, expectedRevision int64) (Story, error) {
	outline.Confirmed = false
	outline.ConfirmedAt = ""
	return s.updateStoryJSON(ctx, appUserID, storyID, expectedRevision, "outline", outline, StageOutline, StatusDraft)
}

func (s *Store) ConfirmOutline(ctx context.Context, appUserID, storyID int64, outline Outline, expectedRevision int64) (Story, error) {
	story, err := s.Get(ctx, appUserID, storyID)
	if err != nil {
		return Story{}, err
	} else if !story.FactCard.Confirmed {
		return Story{}, fmt.Errorf("facts must be confirmed before outline")
	}
	if outline.Perspective == "" {
		outline.Perspective = PerspectiveFirst
	}
	if outline.Tone == "" {
		outline.Tone = ToneWarm
	}
	outline, err = resolveOutlineStoryStyleForWrite(outline, story.Outline)
	if err != nil {
		return Story{}, err
	}
	if err := ValidateOutline(outline); err != nil {
		return Story{}, err
	}
	outline.Confirmed = true
	outline.ConfirmedAt = nowString(time.Now())
	return s.updateStoryJSON(ctx, appUserID, storyID, expectedRevision, "outline", outline, StageOutline, StatusOutlineReady)
}

func (s *Store) ConfirmStoredOutline(ctx context.Context, appUserID, storyID, expectedVersion int64) (Story, error) {
	story, err := s.Get(ctx, appUserID, storyID)
	if err != nil {
		return Story{}, err
	}
	if !story.FactCard.Confirmed {
		return Story{}, fmt.Errorf("facts must be confirmed before outline")
	}
	outline := story.Outline
	if outline.Perspective == "" {
		outline.Perspective = PerspectiveFirst
	}
	if outline.Tone == "" {
		outline.Tone = ToneWarm
	}
	if err := normalizeOutlineStoryStyle(&outline); err != nil {
		return Story{}, err
	}
	if err := ValidateOutline(outline); err != nil {
		return Story{}, err
	}
	outline.Confirmed = true
	outline.ConfirmedAt = nowString(time.Now())
	return s.updateStoryJSON(ctx, appUserID, storyID, expectedVersion, "outline", outline, StageOutline, StatusOutlineReady)
}

func (s *Store) SaveQuestions(ctx context.Context, appUserID, storyID int64, questions []Question, expectedRevision int64) (Story, error) {
	if len(questions) > 3 {
		return Story{}, fmt.Errorf("%w: at most three questions", ErrTooManyQuestions)
	}
	story, err := s.Get(ctx, appUserID, storyID)
	if err != nil {
		return Story{}, err
	}
	story.FactCard.Questions = questions
	return s.updateStoryJSON(ctx, appUserID, storyID, expectedRevision, "fact_card", story.FactCard, StageQuestions, StatusDraft)
}

func validatePreparationFactVersion(incoming, current FactCard) error {
	if incoming.Version != current.Version {
		return ErrConflict
	}
	return nil
}

func validatePreparationSnapshot(incomingFacts, currentFacts FactCard, incomingOutline, currentOutline Outline, expectedRevision, currentRevision int64) error {
	if expectedRevision != currentRevision {
		return ErrConflict
	}
	if err := validatePreparationFactVersion(incomingFacts, currentFacts); err != nil {
		return err
	}
	if incomingOutline.Version != currentOutline.Version {
		return ErrConflict
	}
	return nil
}

func (s *Store) SavePrepared(ctx context.Context, appUserID, storyID int64, facts FactCard, outline Outline, questionSetID string, expectedRevision int64) (Story, error) {
	if len(facts.Questions) > 3 {
		return Story{}, fmt.Errorf("%w: at most three questions", ErrTooManyQuestions)
	}
	questionSetID = strings.TrimSpace(questionSetID)
	if questionSetID == "" {
		return Story{}, fmt.Errorf("question set id is required")
	}
	if s == nil || s.db == nil {
		return Story{}, ErrNilDB
	}
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return Story{}, err
	}
	defer tx.Rollback()
	var currentFactsRaw, currentOutlineRaw []byte
	var currentStatus StoryStatus
	var currentRevision int64
	if err := tx.QueryRowContext(ctx, `SELECT fact_card,outline,status,revision FROM app_life_stories WHERE id=$1 AND app_user_id=$2 AND deleted_at IS NULL FOR UPDATE`, storyID, appUserID).Scan(&currentFactsRaw, &currentOutlineRaw, &currentStatus, &currentRevision); errors.Is(err, sql.ErrNoRows) {
		return Story{}, ErrNotFound
	} else if err != nil {
		return Story{}, err
	}
	if currentStatus == StatusQueued || currentStatus == StatusGenerating {
		return Story{}, ErrInvalidState
	}
	var currentFacts FactCard
	var currentOutline Outline
	_ = json.Unmarshal(currentFactsRaw, &currentFacts)
	_ = json.Unmarshal(currentOutlineRaw, &currentOutline)
	if err := validateFactCardLocations(facts, currentFacts); err != nil {
		return Story{}, err
	}
	if err := validatePreparationSnapshot(facts, currentFacts, outline, currentOutline, expectedRevision, currentRevision); err != nil {
		return Story{}, err
	}
	outline, err = resolveOutlineStoryStyleForWrite(outline, currentOutline)
	if err != nil {
		return Story{}, err
	}
	facts.Version = currentFacts.Version + 1
	facts.QuestionSetID = questionSetID
	facts.Confirmed = false
	facts.ConfirmedAt = ""
	outline.Version = currentOutline.Version + 1
	outline.Confirmed = false
	outline.ConfirmedAt = ""
	factsRaw, err := marshalJSON(facts)
	if err != nil {
		return Story{}, err
	}
	outlineRaw, err := marshalJSON(outline)
	if err != nil {
		return Story{}, err
	}
	if _, err := tx.ExecContext(ctx, `UPDATE app_life_stories SET fact_card=$1::jsonb,outline=$2::jsonb,stage='questions',status='draft',revision=revision+1,updated_at=now() WHERE id=$3 AND app_user_id=$4`, string(factsRaw), string(outlineRaw), storyID, appUserID); err != nil {
		return Story{}, err
	}
	story, err := scanStory(tx.QueryRowContext(ctx, `SELECT `+storyColumns+` FROM app_life_stories WHERE id=$1 AND app_user_id=$2`, storyID, appUserID))
	if err != nil {
		return Story{}, err
	}
	if err := tx.Commit(); err != nil {
		return Story{}, err
	}
	story.Materials, _ = listMaterials(ctx, s.db, appUserID, storyID)
	story.MaterialCount = len(story.Materials)
	return story, nil
}

func (s *Store) AnswerQuestion(ctx context.Context, appUserID, storyID int64, questionSetID, questionID, answer string, skip bool) (Story, error) {
	story, err := s.Get(ctx, appUserID, storyID)
	if err != nil {
		return Story{}, err
	}
	facts := story.FactCard
	if strings.TrimSpace(questionSetID) == "" || strings.TrimSpace(questionSetID) != facts.QuestionSetID {
		return Story{}, ErrConflict
	}
	questionID = strings.TrimSpace(questionID)
	found := false
	for i := range facts.Questions {
		if facts.Questions[i].ID == questionID {
			nextAnswer := strings.TrimSpace(answer)
			if skip {
				nextAnswer = ""
			}
			if !skip && nextAnswer == "" {
				return Story{}, fmt.Errorf("%w: question answer is required unless skipped", ErrValidation)
			}
			completed := strings.TrimSpace(facts.Questions[i].Answer) != "" || facts.Questions[i].Skipped
			if facts.Questions[i].AnsweredAt != "" && completed {
				if facts.Questions[i].Answer == nextAnswer && facts.Questions[i].Skipped == skip {
					return story, nil
				}
				return Story{}, ErrConflict
			}
			facts.Questions[i].Answer = nextAnswer
			facts.Questions[i].Skipped = skip
			facts.Questions[i].AnsweredAt = nowString(time.Now())
			found = true
			break
		}
	}
	if !found {
		return Story{}, ErrNotFound
	}
	return s.updateStoryJSON(ctx, appUserID, storyID, facts.Version, "fact_card", facts, StageQuestions, StatusDraft)
}

func (s *Store) updateStoryJSON(ctx context.Context, appUserID, storyID, expectedVersion int64, column string, value any, stage StoryStage, status StoryStatus) (Story, error) {
	if column != "fact_card" && column != "outline" {
		return Story{}, fmt.Errorf("invalid story json column")
	}
	if s == nil || s.db == nil {
		return Story{}, ErrNilDB
	}
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return Story{}, err
	}
	defer tx.Rollback()
	var currentRaw []byte
	var storyRevision int64
	queryCurrent := fmt.Sprintf(`SELECT %s,revision FROM app_life_stories WHERE id=$1 AND app_user_id=$2 AND deleted_at IS NULL FOR UPDATE`, column)
	if err := tx.QueryRowContext(ctx, queryCurrent, storyID, appUserID).Scan(&currentRaw, &storyRevision); errors.Is(err, sql.ErrNoRows) {
		return Story{}, ErrNotFound
	} else if err != nil {
		return Story{}, err
	}
	var storyStatus StoryStatus
	if err := tx.QueryRowContext(ctx, `SELECT status FROM app_life_stories WHERE id=$1 AND app_user_id=$2`, storyID, appUserID).Scan(&storyStatus); err != nil {
		return Story{}, err
	}
	if storyStatus == StatusQueued || storyStatus == StatusGenerating {
		return Story{}, ErrInvalidState
	}
	var currentVersion int64
	switch column {
	case "fact_card":
		var current FactCard
		_ = json.Unmarshal(currentRaw, &current)
		currentVersion = current.Version
		facts, ok := value.(FactCard)
		if !ok {
			return Story{}, fmt.Errorf("invalid fact card")
		}
		if err := validateFactCardLocations(facts, current); err != nil {
			return Story{}, err
		}
		facts.Version = currentVersion + 1
		value = facts
	case "outline":
		var current Outline
		_ = json.Unmarshal(currentRaw, &current)
		currentVersion = current.Version
		outline, ok := value.(Outline)
		if !ok {
			return Story{}, fmt.Errorf("invalid outline")
		}
		outline, err = resolveOutlineStoryStyleForWrite(outline, current)
		if err != nil {
			return Story{}, err
		}
		outline.Version = currentVersion + 1
		value = outline
	}
	if expectedVersion > 0 && expectedVersion != currentVersion && expectedVersion != storyRevision {
		return Story{}, ErrConflict
	}
	raw, err := marshalJSON(value)
	if err != nil {
		return Story{}, err
	}
	query := fmt.Sprintf(`UPDATE app_life_stories SET %s=$1::jsonb,stage=$2,status=$3,revision=revision+1,updated_at=now() WHERE id=$4 AND app_user_id=$5`, column)
	if _, err := tx.ExecContext(ctx, query, string(raw), stage, status, storyID, appUserID); err != nil {
		return Story{}, err
	}
	story, err := scanStory(tx.QueryRowContext(ctx, `SELECT `+storyColumns+` FROM app_life_stories WHERE id=$1 AND app_user_id=$2`, storyID, appUserID))
	if err != nil {
		return Story{}, err
	}
	if err := tx.Commit(); err != nil {
		return Story{}, err
	}
	return story, nil
}

func (s *Store) Snapshot(ctx context.Context, appUserID, storyID int64) (StorySnapshot, error) {
	story, err := s.Get(ctx, appUserID, storyID)
	if err != nil {
		return StorySnapshot{}, err
	}
	return StorySnapshot{StoryID: story.ID, Materials: story.Materials, FactCard: story.FactCard, Outline: story.Outline,
		FactsVersion: story.FactCard.Version, OutlineVersion: story.Outline.Version,
		SourceVersionID: story.CurrentVersionID}, nil
}

func (s *Store) CreateJob(ctx context.Context, appUserID, storyID int64, requestKey string) (Job, bool, error) {
	return s.CreateGenerationJobWithInput(ctx, appUserID, storyID, GenerationInput{RequestKey: requestKey})
}

func (s *Store) CreateJobWithInstruction(ctx context.Context, appUserID, storyID int64, requestKey, instruction string) (Job, bool, error) {
	return s.CreateGenerationJobWithInput(ctx, appUserID, storyID, GenerationInput{RequestKey: requestKey, Instruction: instruction})
}

func (s *Store) CreateGenerationJob(ctx context.Context, appUserID, storyID int64, requestKey string) (Job, bool, error) {
	return s.CreateJob(ctx, appUserID, storyID, requestKey)
}

// CreateGenerationJobWithInput creates a queued job and reserves one story
// generation in the same transaction. A repeated request key is reusable only
// when the canonical input snapshot hash is identical.
func (s *Store) CreateGenerationJobWithInput(ctx context.Context, appUserID, storyID int64, input GenerationInput) (Job, bool, error) {
	input.RequestKey = strings.TrimSpace(input.RequestKey)
	if input.RequestKey == "" || len([]rune(input.RequestKey)) > 128 {
		return Job{}, false, fmt.Errorf("request key is required")
	}
	input.Instruction = strings.TrimSpace(input.Instruction)
	if len([]rune(input.Instruction)) > 6000 {
		return Job{}, false, fmt.Errorf("generation instruction is too long")
	}
	if s == nil || s.db == nil {
		return Job{}, false, ErrNilDB
	}
	tx, err := s.db.BeginTx(ctx, &sql.TxOptions{Isolation: sql.LevelSerializable})
	if err != nil {
		return Job{}, false, err
	}
	defer tx.Rollback()
	var userStatus string
	if err := tx.QueryRowContext(ctx, `SELECT status FROM app_users WHERE id=$1 FOR UPDATE`, appUserID).Scan(&userStatus); errors.Is(err, sql.ErrNoRows) {
		return Job{}, false, ErrNotFound
	} else if err != nil {
		return Job{}, false, err
	}
	if strings.TrimSpace(userStatus) != "active" {
		return Job{}, false, ErrInactiveUser
	}
	var factRaw, outlineRaw []byte
	var status StoryStatus
	var currentVersionID sql.NullInt64
	if err := tx.QueryRowContext(ctx, `SELECT fact_card,outline,status,current_version_id FROM app_life_stories WHERE id=$1 AND app_user_id=$2 AND deleted_at IS NULL FOR UPDATE`, storyID, appUserID).Scan(&factRaw, &outlineRaw, &status, &currentVersionID); errors.Is(err, sql.ErrNoRows) {
		return Job{}, false, ErrNotFound
	} else if err != nil {
		return Job{}, false, err
	}
	var facts FactCard
	var outline Outline
	_ = json.Unmarshal(factRaw, &facts)
	_ = json.Unmarshal(outlineRaw, &outline)
	if err := normalizeOutlineStoryStyle(&outline); err != nil {
		return Job{}, false, err
	}
	if input.FactsVersion <= 0 {
		input.FactsVersion = facts.Version
	}
	if input.OutlineVersion <= 0 {
		input.OutlineVersion = outline.Version
	}
	if input.SourceVersionID <= 0 && currentVersionID.Valid {
		input.SourceVersionID = currentVersionID.Int64
	}
	if !facts.Confirmed || !outline.Confirmed {
		return Job{}, false, fmt.Errorf("facts and outline must be confirmed")
	}
	if input.FactsVersion != facts.Version || input.OutlineVersion != outline.Version {
		return Job{}, false, ErrConflict
	}
	if input.SourceVersionID > 0 {
		if err := tx.QueryRowContext(ctx, `SELECT id FROM app_life_story_versions WHERE id=$1 AND story_id=$2 AND app_user_id=$3`, input.SourceVersionID, storyID, appUserID).Scan(new(int64)); errors.Is(err, sql.ErrNoRows) {
			return Job{}, false, ErrNotFound
		} else if err != nil {
			return Job{}, false, err
		}
	}
	materials, err := listMaterials(ctx, tx, appUserID, storyID)
	if err != nil {
		return Job{}, false, err
	}
	snapshotValue := StorySnapshot{StoryID: storyID, RequestKey: input.RequestKey, Materials: materials, FactCard: facts, Outline: outline, RevisionInstruction: input.Instruction, FactsVersion: input.FactsVersion, OutlineVersion: input.OutlineVersion, SourceVersionID: input.SourceVersionID}
	originalSnapshot, err := marshalJSON(snapshotValue)
	if err != nil {
		return Job{}, false, err
	}
	safeSnapshot, tokenMap := TokenizeSnapshot(snapshotValue)
	if err := ValidateSnapshot(safeSnapshot); err != nil {
		return Job{}, false, err
	}
	snapshot, err := marshalJSON(safeSnapshot)
	if err != nil {
		return Job{}, false, err
	}
	payloadHash := snapshotPayloadHMAC(originalSnapshot, s.tokenKey)
	snapshotHash := snapshotPayloadHash(snapshot)
	var existing Job
	if existing, err = scanJob(tx.QueryRowContext(ctx, `SELECT `+jobColumns+` FROM app_life_story_jobs WHERE app_user_id=$1 AND request_key=$2 FOR UPDATE`, appUserID, input.RequestKey)); err == nil {
		existingHash := existing.PayloadHash
		requestedHash := payloadHash
		if existing.SnapshotHash == "" {
			// Rows created before snapshot_hash stored the safe snapshot digest in
			// payload_hash. Preserve retry compatibility without replacing the
			// original row's encrypted token map.
			requestedHash = snapshotHash
		}
		if existingHash == "" || existingHash != requestedHash {
			return Job{}, false, ErrPayloadConflict
		}
		if err := tx.Commit(); err != nil {
			return Job{}, false, err
		}
		return existing, true, nil
	} else if !errors.Is(err, sql.ErrNoRows) {
		return Job{}, false, err
	}
	var latestStatus JobStatus
	var latestSnapshotRaw []byte
	if err := tx.QueryRowContext(ctx, `SELECT status,input_snapshot FROM app_life_story_jobs WHERE app_user_id=$1 AND story_id=$2 ORDER BY id DESC LIMIT 1 FOR UPDATE`, appUserID, storyID).Scan(&latestStatus, &latestSnapshotRaw); err == nil {
		if latestStatus == JobSafetyBlocked {
			var latestSnapshot StorySnapshot
			if err := json.Unmarshal(latestSnapshotRaw, &latestSnapshot); err != nil {
				return Job{}, false, err
			}
			if latestSnapshot.FactsVersion == input.FactsVersion && latestSnapshot.OutlineVersion == input.OutlineVersion {
				return Job{}, false, fmt.Errorf("%w: facts or outline must change after safety review", ErrInvalidState)
			}
		}
	} else if !errors.Is(err, sql.ErrNoRows) {
		return Job{}, false, err
	}
	// Idempotency is resolved before the active-job guard. A client retry of
	// the same request must reuse the queued/running row rather than receiving
	// an invalid-state error merely because the first request is still active.
	if status == StatusQueued || status == StatusGenerating {
		return Job{}, false, ErrInvalidState
	}
	var activeID int64
	if err := tx.QueryRowContext(ctx, `SELECT id FROM app_life_story_jobs WHERE app_user_id=$1 AND status IN ('queued','running') ORDER BY id DESC LIMIT 1 FOR UPDATE`, appUserID).Scan(&activeID); err == nil {
		return Job{}, false, ErrInvalidState
	} else if !errors.Is(err, sql.ErrNoRows) {
		return Job{}, false, err
	}
	var job Job
	row := tx.QueryRowContext(ctx, `INSERT INTO app_life_story_jobs(story_id,app_user_id,request_key,input_snapshot,payload_hash,snapshot_hash,source_version_id,status,max_attempts) VALUES($1,$2,$3,$4::jsonb,$5,$6,$7,'queued',3) RETURNING `+jobColumns, storyID, appUserID, input.RequestKey, string(snapshot), payloadHash, snapshotHash, nullableInt64(input.SourceVersionID))
	job, err = scanJob(row)
	if err != nil {
		return Job{}, false, err
	}
	if len(tokenMap) > 0 {
		if err := s.saveTokenMapTx(ctx, tx, appUserID, storyID, job.ID, tokenMap); err != nil {
			return Job{}, false, err
		}
	}
	if _, err := tx.ExecContext(ctx, `UPDATE app_life_stories SET status='queued',stage='queued',revision=revision+1,updated_at=now() WHERE id=$1 AND app_user_id=$2`, storyID, appUserID); err != nil {
		return Job{}, false, err
	}
	if _, err := reserveQuotaTx(ctx, tx, appUserID, job.ID, ""); err != nil {
		return Job{}, false, err
	}
	if err := tx.Commit(); err != nil {
		return Job{}, false, err
	}
	return job, false, nil
}

const jobColumns = `id,story_id,app_user_id,request_key,input_snapshot,payload_hash,snapshot_hash,source_version_id,status,attempt,max_attempts,progress,error_category,error_message,claim_token,claimed_at,lease_until,worker_id,retry_after,started_at,finished_at,version_id,created_at,updated_at`

func scanJob(row interface{ Scan(...any) error }) (Job, error) {
	var j Job
	var inputSnapshot []byte
	var payloadHash string
	var snapshotHash string
	var sourceVersionID sql.NullInt64
	var claimed, lease, retryAfter, started, finished, created, updated sql.NullTime
	var versionID sql.NullInt64
	err := row.Scan(&j.ID, &j.StoryID, &j.AppUserID, &j.RequestKey, &inputSnapshot, &payloadHash, &snapshotHash, &sourceVersionID, &j.Status, &j.Attempt, &j.MaxAttempts, &j.Progress, &j.ErrorCategory, &j.ErrorMessage, &j.ClaimToken, &claimed, &lease, &j.WorkerID, &retryAfter, &started, &finished, &versionID, &created, &updated)
	if err != nil {
		return Job{}, err
	}
	if versionID.Valid {
		j.VersionID = versionID.Int64
	}
	if sourceVersionID.Valid {
		j.SourceVersionID = sourceVersionID.Int64
	}
	j.InputSnapshot = append(j.InputSnapshot[:0], inputSnapshot...)
	j.PayloadHash = payloadHash
	j.SnapshotHash = snapshotHash
	j.ClaimedAt = nowString(claimed.Time)
	j.LeaseUntil = nowString(lease.Time)
	j.RetryAfter = nowString(retryAfter.Time)
	j.StartedAt = nowString(started.Time)
	j.FinishedAt = nowString(finished.Time)
	j.CreatedAt = nowString(created.Time)
	_ = updated
	return j, nil
}

func latestJob(ctx context.Context, q dbtx.DBTX, userID, storyID int64) (Job, error) {
	return scanJob(q.QueryRowContext(ctx, `SELECT `+jobColumns+` FROM app_life_story_jobs WHERE app_user_id=$1 AND story_id=$2 ORDER BY id DESC LIMIT 1`, userID, storyID))
}

func listJobs(ctx context.Context, q dbtx.DBTX, userID, storyID int64) ([]Job, error) {
	rows, err := q.QueryContext(ctx, `SELECT `+jobColumns+` FROM app_life_story_jobs WHERE app_user_id=$1 AND story_id=$2 ORDER BY id DESC`, userID, storyID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	jobs := make([]Job, 0)
	for rows.Next() {
		job, scanErr := scanJob(rows)
		if scanErr != nil {
			return nil, scanErr
		}
		jobs = append(jobs, job)
	}
	return jobs, rows.Err()
}

func listVersions(ctx context.Context, q dbtx.DBTX, userID, storyID int64) ([]Version, error) {
	rows, err := q.QueryContext(ctx, `SELECT id,story_id,version_no,status,perspective,tone,story_style,chapters,reflection,character_count,word_count,model,generation_config,created_at FROM app_life_story_versions WHERE story_id=$1 AND app_user_id=$2 ORDER BY version_no DESC,id DESC`, storyID, userID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	versions := make([]Version, 0)
	for rows.Next() {
		var v Version
		var chapters, config []byte
		var created time.Time
		if err := rows.Scan(&v.ID, &v.StoryID, &v.Number, &v.Status, &v.Perspective, &v.Tone, &v.StoryStyle, &chapters, &v.Reflection, &v.CharacterCount, &v.WordCount, &v.Model, &config, &created); err != nil {
			return nil, err
		}
		_ = json.Unmarshal(chapters, &v.Chapters)
		v.GenerationConfig = config
		v.CreatedAt = nowString(created)
		versions = append(versions, v)
	}
	return versions, rows.Err()
}

func getVersionByID(ctx context.Context, q dbtx.DBTX, userID, versionID int64) (Version, error) {
	var v Version
	var chapters, config []byte
	var created time.Time
	err := q.QueryRowContext(ctx, `SELECT id,story_id,version_no,status,perspective,tone,story_style,chapters,reflection,character_count,word_count,model,generation_config,created_at FROM app_life_story_versions WHERE id=$1 AND app_user_id=$2`, versionID, userID).Scan(&v.ID, &v.StoryID, &v.Number, &v.Status, &v.Perspective, &v.Tone, &v.StoryStyle, &chapters, &v.Reflection, &v.CharacterCount, &v.WordCount, &v.Model, &config, &created)
	if err != nil {
		return Version{}, err
	}
	_ = json.Unmarshal(chapters, &v.Chapters)
	v.GenerationConfig = config
	v.CreatedAt = nowString(created)
	return v, nil
}

func (s *Store) GetJob(ctx context.Context, appUserID, storyID, jobID int64) (Job, error) {
	q, err := s.queryTarget()
	if err != nil {
		return Job{}, err
	}
	j, err := scanJob(q.QueryRowContext(ctx, `SELECT `+jobColumns+` FROM app_life_story_jobs WHERE id=$1 AND story_id=$2 AND app_user_id=$3`, jobID, storyID, appUserID))
	if errors.Is(err, sql.ErrNoRows) {
		return Job{}, ErrNotFound
	}
	return j, err
}

func randomToken() string {
	var b [24]byte
	if _, err := rand.Read(b[:]); err != nil {
		return fmt.Sprintf("fallback-%d", time.Now().UnixNano())
	}
	return hex.EncodeToString(b[:])
}

func lockLifeStoryUserShared(ctx context.Context, tx *sql.Tx, appUserID int64) error {
	var id int64
	if err := tx.QueryRowContext(ctx, `SELECT id FROM app_users WHERE id=$1 FOR SHARE`, appUserID).Scan(&id); errors.Is(err, sql.ErrNoRows) {
		return ErrNotFound
	} else {
		return err
	}
}

func discoverAndLockJobUserShared(ctx context.Context, tx *sql.Tx, jobID int64) (int64, error) {
	var appUserID int64
	if err := tx.QueryRowContext(ctx, `SELECT app_user_id FROM app_life_story_jobs WHERE id=$1`, jobID).Scan(&appUserID); errors.Is(err, sql.ErrNoRows) {
		return 0, ErrNotFound
	} else if err != nil {
		return 0, err
	}
	if err := lockLifeStoryUserShared(ctx, tx, appUserID); err != nil {
		return 0, err
	}
	return appUserID, nil
}

// ClaimNextJob atomically claims one queued or lease-expired job. The caller
// must pass the returned claim token to CompleteJob/FailJob.
func (s *Store) ClaimNextJob(ctx context.Context, lease time.Duration) (Job, error) {
	return s.ClaimNextJobWithWorker(ctx, lease, "")
}

func (s *Store) ClaimNextJobWithWorker(ctx context.Context, lease time.Duration, workerID string) (Job, error) {
	if lease <= 0 {
		lease = 2 * time.Minute
	}
	if s == nil || s.db == nil {
		return Job{}, ErrNilDB
	}
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return Job{}, err
	}
	defer tx.Rollback()
	reaped, err := reapExhaustedJobTx(ctx, tx)
	if err != nil {
		return Job{}, err
	}
	if reaped {
		if err := tx.Commit(); err != nil {
			return Job{}, err
		}
		// This scan performed terminal recovery rather than claiming provider
		// work. The next poll can claim another row without exceeding concurrency.
		return Job{}, sql.ErrNoRows
	}
	var id, appUserID int64
	err = tx.QueryRowContext(ctx, `SELECT id,app_user_id FROM app_life_story_jobs
		WHERE attempt < max_attempts AND (
			(status='queued' AND (retry_after IS NULL OR retry_after <= now()))
			OR (status='running' AND (lease_until IS NULL OR lease_until < now()))
		)
		ORDER BY created_at,id LIMIT 1`).Scan(&id, &appUserID)
	if errors.Is(err, sql.ErrNoRows) {
		return Job{}, sql.ErrNoRows
	}
	if err != nil {
		return Job{}, err
	}
	if err := lockLifeStoryUserShared(ctx, tx, appUserID); errors.Is(err, ErrNotFound) {
		return Job{}, sql.ErrNoRows
	} else if err != nil {
		return Job{}, err
	}
	var claimableID int64
	err = tx.QueryRowContext(ctx, `SELECT id FROM app_life_story_jobs
		WHERE id=$1 AND app_user_id=$2 AND attempt < max_attempts AND (
			(status='queued' AND (retry_after IS NULL OR retry_after <= now()))
			OR (status='running' AND (lease_until IS NULL OR lease_until < now()))
		) FOR UPDATE`, id, appUserID).Scan(&claimableID)
	if errors.Is(err, sql.ErrNoRows) {
		return Job{}, sql.ErrNoRows
	}
	if err != nil {
		return Job{}, err
	}
	token := randomToken()
	workerID = strings.TrimSpace(workerID)
	if workerID == "" {
		workerID = "life-story-worker"
	}
	var job Job
	row := tx.QueryRowContext(ctx, `UPDATE app_life_story_jobs SET status='running',attempt=attempt+1,claim_token=$1,worker_id=$2,claimed_at=now(),lease_until=now()+$3::interval,started_at=COALESCE(started_at,now()),updated_at=now(),progress=GREATEST(progress,1),retry_after=NULL WHERE id=$4 RETURNING `+jobColumns, token, workerID, fmt.Sprintf("%f seconds", lease.Seconds()), claimableID)
	job, err = scanJob(row)
	if err != nil {
		return Job{}, err
	}
	if _, err := tx.ExecContext(ctx, `UPDATE app_life_stories SET status='generating',stage='generating',updated_at=now() WHERE id=$1 AND app_user_id=$2 AND status IN ('queued','generating')`, job.StoryID, job.AppUserID); err != nil {
		return Job{}, err
	}
	if err := tx.Commit(); err != nil {
		return Job{}, err
	}
	return job, nil
}

func reapExhaustedJobTx(ctx context.Context, tx *sql.Tx) (bool, error) {
	// Candidate discovery intentionally takes no job lock. The durable lock
	// order is user then job, matching generation, deletion, and quota paths.
	var jobID, appUserID int64
	err := tx.QueryRowContext(ctx, `SELECT id,app_user_id FROM app_life_story_jobs
		WHERE attempt >= max_attempts AND (
			status='queued'
			OR (status='running' AND (lease_until IS NULL OR lease_until < now()))
		)
		ORDER BY created_at,id LIMIT 1`).Scan(&jobID, &appUserID)
	if errors.Is(err, sql.ErrNoRows) {
		return false, nil
	}
	if err != nil {
		return false, err
	}
	var userStatus string
	if err := tx.QueryRowContext(ctx, `SELECT status FROM app_users WHERE id=$1 FOR UPDATE`, appUserID).Scan(&userStatus); errors.Is(err, sql.ErrNoRows) {
		// Account deletion may have cascaded the candidate between discovery and
		// the user lock. Let the normal claim query continue in this transaction.
		return false, nil
	} else if err != nil {
		return false, err
	}

	job, err := scanJob(tx.QueryRowContext(ctx, `SELECT `+jobColumns+` FROM app_life_story_jobs
		WHERE id=$1 AND app_user_id=$2 AND attempt >= max_attempts AND (
			status='queued'
			OR (status='running' AND (lease_until IS NULL OR lease_until < now()))
		) FOR UPDATE`, jobID, appUserID))
	if errors.Is(err, sql.ErrNoRows) {
		// A live worker may have renewed the lease while this transaction waited
		// for its generation guard. Revalidation prevents terminating that job.
		return false, nil
	}
	if err != nil {
		return false, err
	}
	job, err = scanJob(tx.QueryRowContext(ctx, `UPDATE app_life_story_jobs SET
		status='failed',error_category='attempts_exhausted',
		error_message='故事生成失败，请稍后重试',claim_token='',lease_until=NULL,
		retry_after=NULL,finished_at=now(),updated_at=now()
		WHERE id=$1 AND app_user_id=$2 RETURNING `+jobColumns, job.ID, job.AppUserID))
	if err != nil {
		return false, err
	}
	if _, err := tx.ExecContext(ctx, `UPDATE app_life_stories SET
		status=CASE WHEN current_version_id IS NULL THEN 'failed' ELSE 'completed' END,
		stage=CASE WHEN current_version_id IS NULL THEN 'failed' ELSE 'reading' END,
		updated_at=now()
		WHERE id=$1 AND app_user_id=$2 AND status IN ('generating','queued')`, job.StoryID, job.AppUserID); err != nil {
		return false, err
	}
	if _, _, err := releaseQuotaTx(ctx, tx, job.AppUserID, job.ID); err != nil {
		return false, err
	}
	if err := deleteJobTokenMapTx(ctx, tx, job.ID); err != nil {
		return false, err
	}
	return true, nil
}

func (s *Store) RenewJobLease(ctx context.Context, jobID int64, token string, lease time.Duration) (bool, error) {
	if lease <= 0 {
		lease = 2 * time.Minute
	}
	if s == nil || s.db == nil {
		return false, ErrNilDB
	}
	res, err := s.db.ExecContext(ctx, `UPDATE app_life_story_jobs SET lease_until=now()+$1::interval,updated_at=now() WHERE id=$2 AND status='running' AND claim_token=$3`, fmt.Sprintf("%f seconds", lease.Seconds()), jobID, token)
	if err != nil {
		return false, err
	}
	n, _ := res.RowsAffected()
	return n == 1, nil
}

// acquireGenerationGuard keeps the user row stable while private story data is
// sent to the model. Account deletion uses FOR UPDATE on the same row, so it
// either completes before this check (and generation is rejected) or waits for
// the in-flight model call before deleting the story data.
func (s *Store) acquireGenerationGuard(ctx context.Context, job Job) (func(), error) {
	if s == nil || s.db == nil {
		return nil, ErrNilDB
	}
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return nil, err
	}
	release := func() { _ = tx.Rollback() }
	var userStatus string
	if err := tx.QueryRowContext(ctx, `SELECT status FROM app_users WHERE id=$1 FOR SHARE`, job.AppUserID).Scan(&userStatus); errors.Is(err, sql.ErrNoRows) {
		release()
		return nil, ErrInactiveUser
	} else if err != nil {
		release()
		return nil, err
	}
	if strings.TrimSpace(userStatus) != "active" {
		release()
		return nil, ErrInactiveUser
	}
	var status JobStatus
	var claimToken string
	if err := tx.QueryRowContext(ctx, `SELECT status,claim_token FROM app_life_story_jobs
		WHERE id=$1 AND story_id=$2 AND app_user_id=$3`, job.ID, job.StoryID, job.AppUserID).Scan(&status, &claimToken); errors.Is(err, sql.ErrNoRows) {
		release()
		return nil, ErrConflict
	} else if err != nil {
		release()
		return nil, err
	}
	if status != JobRunning || strings.TrimSpace(job.ClaimToken) == "" || claimToken != job.ClaimToken {
		release()
		return nil, ErrConflict
	}
	return release, nil
}

func (s *Store) FailJob(ctx context.Context, jobID int64, token string, category, message string, retry bool) (Job, error) {
	if s == nil || s.db == nil {
		return Job{}, ErrNilDB
	}
	category = strings.TrimSpace(category)
	message = strings.TrimSpace(message)
	if len([]rune(message)) > 500 {
		message = string([]rune(message)[:500])
	}
	status := JobFailed
	if retry {
		status = JobQueued
	}
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return Job{}, err
	}
	defer tx.Rollback()
	if _, err := discoverAndLockJobUserShared(ctx, tx, jobID); errors.Is(err, ErrNotFound) {
		return Job{}, ErrConflict
	} else if err != nil {
		return Job{}, err
	}
	var current Job
	current, err = scanJob(tx.QueryRowContext(ctx, `SELECT `+jobColumns+` FROM app_life_story_jobs WHERE id=$1 FOR UPDATE`, jobID))
	if errors.Is(err, sql.ErrNoRows) {
		return Job{}, ErrConflict
	}
	if err != nil {
		return Job{}, err
	}
	if current.Status != JobRunning || strings.TrimSpace(token) == "" || current.ClaimToken != token {
		return Job{}, ErrConflict
	}
	// Backoff is persisted so a crashed/restarted process cannot hot-loop a
	// failing provider. The claim token is always cleared before requeue.
	q := `UPDATE app_life_story_jobs SET status=$1,error_category=$2,error_message=$3,claim_token='',lease_until=NULL,retry_after=CASE WHEN $4='queued' THEN now()+make_interval(secs => LEAST(300, POWER(2,GREATEST(attempt-1,0))::int)) ELSE NULL END,finished_at=CASE WHEN $4='failed' THEN now() ELSE NULL END,updated_at=now() WHERE id=$5 AND status='running' AND claim_token=$6 RETURNING ` + jobColumns
	job, err := scanJob(tx.QueryRowContext(ctx, q, status, category, message, string(status), jobID, token))
	if errors.Is(err, sql.ErrNoRows) {
		return Job{}, ErrConflict
	}
	if err != nil {
		return Job{}, err
	}
	if status == JobFailed {
		if _, err := tx.ExecContext(ctx, `UPDATE app_life_stories SET status=CASE WHEN current_version_id IS NULL THEN 'failed' ELSE 'completed' END,stage=CASE WHEN current_version_id IS NULL THEN 'failed' ELSE 'reading' END,updated_at=now() WHERE id=$1 AND app_user_id=$2 AND status IN ('generating','queued')`, job.StoryID, job.AppUserID); err != nil {
			return Job{}, err
		}
		if _, _, err := releaseQuotaTx(ctx, tx, job.AppUserID, job.ID); err != nil {
			return Job{}, err
		}
		if err := deleteJobTokenMapTx(ctx, tx, job.ID); err != nil {
			return Job{}, err
		}
	}
	if err := tx.Commit(); err != nil {
		return Job{}, err
	}
	return job, nil
}

func (s *Store) CancelJob(ctx context.Context, appUserID, storyID, jobID int64) (Job, error) {
	if s == nil || s.db == nil {
		return Job{}, ErrNilDB
	}
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return Job{}, err
	}
	defer tx.Rollback()
	if err := lockLifeStoryUserShared(ctx, tx, appUserID); err != nil {
		return Job{}, err
	}
	var currentStatus JobStatus
	if err := tx.QueryRowContext(ctx, `SELECT status FROM app_life_story_jobs WHERE id=$1 AND story_id=$2 AND app_user_id=$3 FOR UPDATE`, jobID, storyID, appUserID).Scan(&currentStatus); errors.Is(err, sql.ErrNoRows) {
		return Job{}, ErrNotFound
	} else if err != nil {
		return Job{}, err
	}
	if currentStatus != JobQueued && currentStatus != JobRunning {
		return Job{}, ErrNotFound
	}
	j, err := scanJob(tx.QueryRowContext(ctx, `UPDATE app_life_story_jobs SET status='cancelled',claim_token='',lease_until=NULL,retry_after=NULL,finished_at=now(),updated_at=now() WHERE id=$1 AND story_id=$2 AND app_user_id=$3 AND status IN ('queued','running') RETURNING `+jobColumns, jobID, storyID, appUserID))
	if errors.Is(err, sql.ErrNoRows) {
		return Job{}, ErrNotFound
	}
	if err != nil {
		return Job{}, err
	}
	if _, err := tx.ExecContext(ctx, `UPDATE app_life_stories SET status=CASE WHEN current_version_id IS NULL THEN 'cancelled' ELSE 'completed' END,stage=CASE WHEN current_version_id IS NULL THEN 'cancelled' ELSE 'reading' END,updated_at=now() WHERE id=$1 AND app_user_id=$2 AND status IN ('queued','generating')`, storyID, appUserID); err != nil {
		return Job{}, err
	}
	if _, _, err := releaseQuotaTx(ctx, tx, appUserID, jobID); err != nil {
		return Job{}, err
	}
	if err := deleteJobTokenMapTx(ctx, tx, jobID); err != nil {
		return Job{}, err
	}
	if err := tx.Commit(); err != nil {
		return Job{}, err
	}
	return j, nil
}

func (s *Store) RejectQueuedJob(ctx context.Context, appUserID, storyID, jobID int64, category, message string) (Job, error) {
	if s == nil || s.db == nil {
		return Job{}, ErrNilDB
	}
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return Job{}, err
	}
	defer tx.Rollback()
	if err := lockLifeStoryUserShared(ctx, tx, appUserID); err != nil {
		return Job{}, err
	}
	var currentStatus JobStatus
	if err := tx.QueryRowContext(ctx, `SELECT status FROM app_life_story_jobs WHERE id=$1 AND story_id=$2 AND app_user_id=$3 FOR UPDATE`, jobID, storyID, appUserID).Scan(&currentStatus); errors.Is(err, sql.ErrNoRows) {
		return Job{}, ErrConflict
	} else if err != nil {
		return Job{}, err
	}
	if currentStatus != JobQueued {
		return Job{}, ErrConflict
	}
	job, err := scanJob(tx.QueryRowContext(ctx, `UPDATE app_life_story_jobs SET status='failed',error_category=$1,error_message=$2,retry_after=NULL,finished_at=now(),updated_at=now() WHERE id=$3 AND story_id=$4 AND app_user_id=$5 AND status='queued' RETURNING `+jobColumns, strings.TrimSpace(category), strings.TrimSpace(message), jobID, storyID, appUserID))
	if errors.Is(err, sql.ErrNoRows) {
		return Job{}, ErrConflict
	}
	if err != nil {
		return Job{}, err
	}
	if _, err := tx.ExecContext(ctx, `UPDATE app_life_stories SET status=CASE WHEN current_version_id IS NULL THEN 'failed' ELSE 'completed' END,stage=CASE WHEN current_version_id IS NULL THEN 'failed' ELSE 'reading' END,updated_at=now() WHERE id=$1 AND app_user_id=$2 AND status='queued'`, storyID, appUserID); err != nil {
		return Job{}, err
	}
	if _, _, err := releaseQuotaTx(ctx, tx, appUserID, jobID); err != nil {
		return Job{}, err
	}
	if err := deleteJobTokenMapTx(ctx, tx, jobID); err != nil {
		return Job{}, err
	}
	if err := tx.Commit(); err != nil {
		return Job{}, err
	}
	return job, nil
}

func (s *Store) SaveProgress(ctx context.Context, appUserID, storyID int64, progress ReadingProgress) (ReadingProgress, error) {
	if appUserID <= 0 || storyID <= 0 {
		return ReadingProgress{}, fmt.Errorf("invalid story id")
	}
	chapterIndex := progress.EffectiveChapterIndex()
	if chapterIndex < 0 {
		return ReadingProgress{}, fmt.Errorf("chapter index must be non-negative")
	}
	if progress.CharacterOffset < 0 {
		return ReadingProgress{}, fmt.Errorf("character offset must be non-negative")
	}
	if s == nil || s.db == nil {
		return ReadingProgress{}, ErrNilDB
	}
	clientUpdatedAt := time.Now().UTC()
	if value := strings.TrimSpace(progress.ClientUpdatedAt); value != "" {
		parsed, err := time.Parse(time.RFC3339Nano, value)
		if err != nil {
			return ReadingProgress{}, fmt.Errorf("invalid client updated time: %w", err)
		}
		clientUpdatedAt = parsed.UTC()
	}
	var chaptersRaw []byte
	if progress.VersionID <= 0 {
		if err := s.db.QueryRowContext(ctx, `SELECT version.id,version.chapters
		 FROM app_life_stories story JOIN app_life_story_versions version ON version.id=story.current_version_id
		 WHERE story.id=$1 AND story.app_user_id=$2 AND story.deleted_at IS NULL`, storyID, appUserID).Scan(&progress.VersionID, &chaptersRaw); errors.Is(err, sql.ErrNoRows) {
			return ReadingProgress{}, ErrNotFound
		} else if err != nil {
			return ReadingProgress{}, err
		}
	} else if err := s.db.QueryRowContext(ctx, `SELECT chapters FROM app_life_story_versions WHERE id=$1 AND story_id=$2 AND app_user_id=$3`, progress.VersionID, storyID, appUserID).Scan(&chaptersRaw); errors.Is(err, sql.ErrNoRows) {
		return ReadingProgress{}, ErrNotFound
	} else if err != nil {
		return ReadingProgress{}, err
	}
	var chapters []Chapter
	if err := json.Unmarshal(chaptersRaw, &chapters); err != nil {
		return ReadingProgress{}, err
	}
	if chapterIndex >= len(chapters) {
		return ReadingProgress{}, fmt.Errorf("chapter index is outside the current version")
	}
	if chapterIndex >= 0 {
		maxOffset := len([]rune(chapters[chapterIndex].Body))
		if progress.CharacterOffset > maxOffset {
			return ReadingProgress{}, fmt.Errorf("character offset is outside the current chapter")
		}
	}
	var saved ReadingProgress
	var savedClientUpdatedAt, updated time.Time
	err := s.db.QueryRowContext(ctx, `INSERT INTO app_life_story_progress(app_user_id,story_id,version_id,chapter_order,character_offset,completed,client_updated_at,updated_at)
		VALUES($1,$2,$3,$4,$5,$6,$7,now())
		ON CONFLICT(app_user_id,story_id) DO UPDATE SET
			version_id=EXCLUDED.version_id,
			chapter_order=EXCLUDED.chapter_order,
			character_offset=EXCLUDED.character_offset,
			completed=EXCLUDED.completed,
			client_updated_at=EXCLUDED.client_updated_at,
			updated_at=now()
		WHERE EXCLUDED.client_updated_at >= app_life_story_progress.client_updated_at
		RETURNING story_id,version_id,chapter_order,character_offset,completed,client_updated_at,updated_at`,
		appUserID, storyID, progress.VersionID, chapterIndex, progress.CharacterOffset, progress.Completed, clientUpdatedAt,
	).Scan(&saved.StoryID, &saved.VersionID, &saved.ChapterIndex, &saved.CharacterOffset, &saved.Completed, &savedClientUpdatedAt, &updated)
	if errors.Is(err, sql.ErrNoRows) {
		return getProgressWithDBTX(ctx, s.db, appUserID, storyID)
	}
	if err != nil {
		return ReadingProgress{}, err
	}
	saved.ChapterOrder = saved.ChapterIndex + 1
	saved.ClientUpdatedAt = nowString(savedClientUpdatedAt)
	saved.UpdatedAt = nowString(updated)
	return saved, nil
}

func (s *Store) GetProgress(ctx context.Context, appUserID, storyID int64) (ReadingProgress, error) {
	if s == nil || s.db == nil {
		return ReadingProgress{}, ErrNilDB
	}
	return getProgressWithDBTX(ctx, s.db, appUserID, storyID)
}

func getProgressWithDBTX(ctx context.Context, q dbtx.DBTX, appUserID, storyID int64) (ReadingProgress, error) {
	var p ReadingProgress
	var clientUpdatedAt, updated time.Time
	err := q.QueryRowContext(ctx, `SELECT story_id,version_id,chapter_order,character_offset,completed,client_updated_at,updated_at FROM app_life_story_progress WHERE app_user_id=$1 AND story_id=$2`, appUserID, storyID).Scan(&p.StoryID, &p.VersionID, &p.ChapterIndex, &p.CharacterOffset, &p.Completed, &clientUpdatedAt, &updated)
	if errors.Is(err, sql.ErrNoRows) {
		var owned int
		if ownerErr := q.QueryRowContext(ctx, `SELECT 1 FROM app_life_stories WHERE id=$1 AND app_user_id=$2 AND deleted_at IS NULL`, storyID, appUserID).Scan(&owned); errors.Is(ownerErr, sql.ErrNoRows) {
			return ReadingProgress{}, ErrNotFound
		} else if ownerErr != nil {
			return ReadingProgress{}, ownerErr
		}
		return ReadingProgress{StoryID: storyID, ChapterIndex: 0, ChapterOrder: 1}, nil
	}
	if err == nil {
		p.ChapterOrder = p.ChapterIndex + 1
		p.ClientUpdatedAt = nowString(clientUpdatedAt)
		p.UpdatedAt = nowString(updated)
	}
	return p, err
}

func (s *Store) Delete(ctx context.Context, appUserID, storyID int64) error {
	if s == nil || s.db == nil {
		return ErrNilDB
	}
	tx, err := s.db.BeginTx(ctx, &sql.TxOptions{Isolation: sql.LevelSerializable})
	if err != nil {
		return err
	}
	defer tx.Rollback()
	var userStatus string
	if err := tx.QueryRowContext(ctx, `SELECT status FROM app_users WHERE id=$1 FOR UPDATE`, appUserID).Scan(&userStatus); errors.Is(err, sql.ErrNoRows) {
		return ErrNotFound
	} else if err != nil {
		return err
	}
	// Job rows are locked before the story row. Worker transitions take the same
	// user -> job -> story -> quota order, so deletion cannot form a lock cycle
	// with a claim, cancellation, failure, rejection, or safety block.
	rows, err := tx.QueryContext(ctx, `SELECT id FROM app_life_story_jobs
		WHERE app_user_id=$1 AND story_id=$2 ORDER BY id FOR UPDATE`, appUserID, storyID)
	if err != nil {
		return err
	}
	jobIDs := make([]int64, 0)
	for rows.Next() {
		var jobID int64
		if scanErr := rows.Scan(&jobID); scanErr != nil {
			rows.Close()
			return scanErr
		}
		jobIDs = append(jobIDs, jobID)
	}
	if err := rows.Err(); err != nil {
		rows.Close()
		return err
	}
	rows.Close()
	var id int64
	if err := tx.QueryRowContext(ctx, `SELECT id FROM app_life_stories WHERE id=$1 AND app_user_id=$2 AND deleted_at IS NULL FOR UPDATE`, storyID, appUserID).Scan(&id); errors.Is(err, sql.ErrNoRows) {
		return ErrNotFound
	} else if err != nil {
		return err
	}
	// Release every outstanding reservation before the story/job rows are
	// removed because job_id becomes NULL when a job is cascaded.
	for _, jobID := range jobIDs {
		_, released, releaseErr := releaseQuotaTx(ctx, tx, appUserID, jobID)
		if releaseErr != nil {
			return releaseErr
		}
		_ = released
	}
	if _, err := tx.ExecContext(ctx, `DELETE FROM app_life_story_outbox WHERE app_user_id=$1 AND story_id=$2`, appUserID, storyID); err != nil {
		return err
	}
	if _, err := tx.ExecContext(ctx, `DELETE FROM app_story_quota_ledger
		WHERE app_user_id=$1 AND job_id IN (SELECT id FROM app_life_story_jobs WHERE story_id=$2)`, appUserID, storyID); err != nil {
		return err
	}
	result, err := tx.ExecContext(ctx, `DELETE FROM app_life_stories WHERE id=$1 AND app_user_id=$2`, storyID, appUserID)
	if err != nil {
		return err
	}
	if affected, _ := result.RowsAffected(); affected != 1 {
		return ErrNotFound
	}
	rootLink := fmt.Sprintf("/life-stories/%d", storyID)
	_, _ = tx.ExecContext(ctx, `DELETE FROM app_notifications
		WHERE app_user_id=$1 AND (deep_link=$2 OR deep_link LIKE $3)`, appUserID, rootLink, rootLink+"/%")
	return tx.Commit()
}

func (s *Store) DeleteStory(ctx context.Context, appUserID, storyID int64) error {
	return s.Delete(ctx, appUserID, storyID)
}

func (s *Store) UpdateMeta(ctx context.Context, appUserID, storyID int64, title *string, favorite *bool) (Story, error) {
	if s == nil || s.db == nil {
		return Story{}, ErrNilDB
	}
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return Story{}, err
	}
	defer tx.Rollback()
	var currentTitle string
	var currentFavorite bool
	if err := tx.QueryRowContext(ctx, `SELECT title,is_favorite FROM app_life_stories WHERE id=$1 AND app_user_id=$2 AND deleted_at IS NULL FOR UPDATE`, storyID, appUserID).Scan(&currentTitle, &currentFavorite); errors.Is(err, sql.ErrNoRows) {
		return Story{}, ErrNotFound
	} else if err != nil {
		return Story{}, err
	}
	if title != nil {
		currentTitle = normalizeTitle(*title)
	}
	if favorite != nil {
		currentFavorite = *favorite
	}
	if _, err := tx.ExecContext(ctx, `UPDATE app_life_stories SET title=$1,is_favorite=$2,revision=revision+1,updated_at=now() WHERE id=$3 AND app_user_id=$4`, currentTitle, currentFavorite, storyID, appUserID); err != nil {
		return Story{}, err
	}
	story, err := scanStory(tx.QueryRowContext(ctx, `SELECT `+storyColumns+` FROM app_life_stories WHERE id=$1 AND app_user_id=$2`, storyID, appUserID))
	if err != nil {
		return Story{}, err
	}
	if err := tx.Commit(); err != nil {
		return Story{}, err
	}
	story.Materials, _ = listMaterials(ctx, s.db, appUserID, storyID)
	story.MaterialCount = len(story.Materials)
	return story, nil
}

func (s *Store) GetVersion(ctx context.Context, appUserID, storyID, versionID int64) (Version, error) {
	q, err := s.queryTarget()
	if err != nil {
		return Version{}, err
	}
	if versionID <= 0 {
		if err := q.QueryRowContext(ctx, `SELECT current_version_id FROM app_life_stories WHERE id=$1 AND app_user_id=$2 AND deleted_at IS NULL`, storyID, appUserID).Scan(&versionID); errors.Is(err, sql.ErrNoRows) {
			return Version{}, ErrNotFound
		} else if err != nil {
			return Version{}, err
		}
		if versionID <= 0 {
			return Version{}, ErrNotFound
		}
	}
	version, err := getVersionByID(ctx, q, appUserID, versionID)
	if errors.Is(err, sql.ErrNoRows) {
		return Version{}, ErrNotFound
	}
	if err != nil {
		return Version{}, err
	}
	if version.StoryID != storyID {
		return Version{}, ErrNotFound
	}
	return version, nil
}
