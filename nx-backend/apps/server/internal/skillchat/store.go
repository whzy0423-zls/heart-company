package skillchat

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"time"

	"nine-xing/nx-backend/apps/server/internal/chat"
)

const sessionSelect = `
	SELECT session.id, session.app_user_id, skill.id, version.id, skill.key, skill.name,
	       skill.icon_key, session.title, session.scene, version.version, version.instructions,
	       version.opening_prompts, version.theory_release_id, version.safety_profile, version.min_app_version, version.source_metadata, version.status,
	       library.status, category.status, skill.status, session.generation_revision,
	       session.updated_at, session.create_time
	FROM app_chat_sessions session
	JOIN app_skill_versions version ON version.id = session.skill_version_id
	JOIN app_skills skill ON skill.id = version.skill_id
	JOIN app_skill_categories category ON category.id = skill.category_id
	JOIN app_skill_libraries library ON library.id = category.library_id`

type Store struct{ db *sql.DB }

func NewStore(db *sql.DB) *Store { return &Store{db: db} }

func (s *Store) available() error {
	if s == nil || s.db == nil {
		return ErrStoreUnavailable
	}
	return nil
}

type rowScanner interface{ Scan(...any) error }

func scanSession(row rowScanner) (Session, error) {
	var item Session
	var openingPrompts []byte
	var sourceMetadata []byte
	var updatedAt, createTime time.Time
	err := row.Scan(
		&item.ID, &item.AppUserID, &item.SkillID, &item.SkillVersionID, &item.SkillKey, &item.SkillName,
		&item.SkillIconKey, &item.Title, &item.Scene, &item.Version, &item.Instructions,
		&openingPrompts,
		&item.TheoryReleaseID, &item.SafetyProfile, &item.MinAppVersion, &sourceMetadata, &item.VersionStatus,
		&item.LibraryStatus, &item.CategoryStatus, &item.SkillStatus, &item.GenerationRevision,
		&updatedAt, &createTime,
	)
	item.UpdatedAt, item.CreateTime = formatTime(updatedAt), formatTime(createTime)
	if len(openingPrompts) == 0 {
		item.OpeningPrompts = []string{}
	} else if err := json.Unmarshal(openingPrompts, &item.OpeningPrompts); err != nil {
		return item, fmt.Errorf("scan skill session opening prompts: %w", err)
	}
	if item.OpeningPrompts == nil {
		item.OpeningPrompts = []string{}
	}
	item.SourceMetadata = sanitizeSessionSourceMetadata(sourceMetadata)
	return item, err
}

func (s *Store) ListSessions(ctx context.Context, appUserID, skillID int64) ([]Session, error) {
	if err := s.available(); err != nil {
		return nil, err
	}
	if appUserID <= 0 || skillID <= 0 {
		return nil, ErrInvalidInput
	}
	rows, err := s.db.QueryContext(ctx, sessionSelect+`
		WHERE session.app_user_id = $1 AND session.scene = 'skill_chat' AND skill.id = $2
		  AND library.status = 'enabled' AND category.status = 'enabled' AND skill.status = 'enabled'
		  AND version.status IN ('published','retired')
		ORDER BY session.updated_at DESC, session.id DESC`, appUserID, skillID)
	if err != nil {
		return nil, fmt.Errorf("list skill sessions: %w", err)
	}
	defer rows.Close()
	out := make([]Session, 0)
	for rows.Next() {
		item, err := scanSession(rows)
		if err != nil {
			return nil, fmt.Errorf("list skill sessions: scan: %w", err)
		}
		out = append(out, item)
	}
	return out, rows.Err()
}

// ListRecentSessions returns at most one latest session per skill. This keeps
// the catalog's recent section independent from the number of published skills.
func (s *Store) ListRecentSessions(ctx context.Context, appUserID int64, limit int) ([]Session, error) {
	if err := s.available(); err != nil {
		return nil, err
	}
	if appUserID <= 0 {
		return nil, ErrInvalidInput
	}
	if limit <= 0 {
		limit = 2
	}
	if limit > 20 {
		limit = 20
	}
	rows, err := s.db.QueryContext(ctx, sessionSelect+`
		WHERE session.app_user_id = $1 AND session.scene = 'skill_chat'
		  AND library.status = 'enabled' AND category.status = 'enabled' AND skill.status = 'enabled'
		  AND version.status IN ('published','retired')
		  AND session.id IN (
		    SELECT DISTINCT ON (version2.skill_id) session2.id
		    FROM app_chat_sessions session2
		    JOIN app_skill_versions version2 ON version2.id=session2.skill_version_id
		    WHERE session2.app_user_id=$1 AND session2.scene='skill_chat'
		    ORDER BY version2.skill_id,session2.updated_at DESC,session2.id DESC
		  )
		ORDER BY session.updated_at DESC,session.id DESC
		LIMIT $2`, appUserID, limit)
	if err != nil {
		return nil, fmt.Errorf("list recent skill sessions: %w", err)
	}
	defer rows.Close()
	out := make([]Session, 0, limit)
	for rows.Next() {
		item, err := scanSession(rows)
		if err != nil {
			return nil, fmt.Errorf("list recent skill sessions: scan: %w", err)
		}
		out = append(out, item)
	}
	return out, rows.Err()
}

func (s *Store) LatestSession(ctx context.Context, appUserID, skillID int64) (Session, error) {
	if err := s.available(); err != nil {
		return Session{}, err
	}
	item, err := scanSession(s.db.QueryRowContext(ctx, sessionSelect+`
		WHERE session.app_user_id = $1 AND session.scene = 'skill_chat' AND skill.id = $2
		  AND library.status = 'enabled' AND category.status = 'enabled' AND skill.status = 'enabled'
		  AND version.status IN ('published','retired')
		ORDER BY session.updated_at DESC, session.id DESC LIMIT 1`, appUserID, skillID))
	if errors.Is(err, sql.ErrNoRows) {
		return Session{}, ErrNotFound
	}
	return item, err
}

func (s *Store) CreateSession(ctx context.Context, appUserID, skillID int64, title string) (Session, error) {
	if err := s.available(); err != nil {
		return Session{}, err
	}
	if appUserID <= 0 || skillID <= 0 {
		return Session{}, ErrInvalidInput
	}
	title = strings.TrimSpace(title)
	if runes := []rune(title); len(runes) > 120 {
		title = string(runes[:120])
	}
	var sessionID int64
	err := s.db.QueryRowContext(ctx, `
		INSERT INTO app_chat_sessions (app_user_id, card_id, title, scene, skill_version_id)
		SELECT $1, NULL, COALESCE(NULLIF($3, ''), skill.name || '对话'), 'skill_chat', version.id
		FROM app_skills skill
		JOIN app_skill_categories category ON category.id = skill.category_id AND category.status = 'enabled'
		JOIN app_skill_libraries library ON library.id = category.library_id AND library.status = 'enabled'
		JOIN app_skill_versions version ON skill.latest_published_version_id = version.id AND version.status = 'published'
		WHERE skill.id = $2 AND skill.status = 'enabled'
		RETURNING id`, appUserID, skillID, title).Scan(&sessionID)
	if errors.Is(err, sql.ErrNoRows) {
		return Session{}, ErrNotFound
	}
	if err != nil {
		return Session{}, fmt.Errorf("create skill session: %w", err)
	}
	return s.GetSession(ctx, appUserID, sessionID)
}

func (s *Store) GetSession(ctx context.Context, appUserID, sessionID int64) (Session, error) {
	if err := s.available(); err != nil {
		return Session{}, err
	}
	if appUserID <= 0 || sessionID <= 0 {
		return Session{}, ErrNotFound
	}
	item, err := scanSession(s.db.QueryRowContext(ctx, sessionSelect+`
		WHERE session.id = $1 AND session.app_user_id = $2 AND session.scene = 'skill_chat'`, sessionID, appUserID))
	if errors.Is(err, sql.ErrNoRows) {
		return Session{}, ErrNotFound
	}
	if err != nil {
		return Session{}, fmt.Errorf("get skill session: %w", err)
	}
	if !item.Runnable() {
		return Session{}, ErrVersionUnavailable
	}
	return item, nil
}

func (s *Store) DeleteSession(ctx context.Context, appUserID, sessionID int64) error {
	if err := s.available(); err != nil {
		return err
	}
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()
	if err := deleteSkillSessionAudioAssets(ctx, tx, appUserID, sessionID); err != nil {
		return fmt.Errorf("delete skill session audio: %w", err)
	}
	result, err := tx.ExecContext(ctx, `
		DELETE FROM app_chat_sessions
		WHERE id = $1 AND app_user_id = $2 AND scene = 'skill_chat'`, sessionID, appUserID)
	if err != nil {
		return fmt.Errorf("delete skill session: %w", err)
	}
	affected, err := result.RowsAffected()
	if err != nil {
		return err
	}
	if affected == 0 {
		return ErrNotFound
	}
	return tx.Commit()
}

func deleteSkillSessionAudioAssets(ctx context.Context, tx *sql.Tx, appUserID, sessionID int64) error {
	_, err := tx.ExecContext(ctx, `
		DELETE FROM upload_assets asset
		USING app_chat_messages message, app_chat_sessions session
		WHERE asset.id = message.audio_asset_id
		  AND message.session_id = session.id
		  AND session.id = $1 AND session.app_user_id = $2 AND session.scene = 'skill_chat'`, sessionID, appUserID)
	return err
}

func (s *Store) UpdateSession(ctx context.Context, appUserID, sessionID int64, title *string, clear bool) (Session, error) {
	if err := s.available(); err != nil {
		return Session{}, err
	}
	if appUserID <= 0 || sessionID <= 0 || (title == nil && !clear) {
		return Session{}, ErrInvalidInput
	}
	var normalizedTitle string
	if title != nil {
		normalizedTitle = strings.TrimSpace(*title)
		if normalizedTitle == "" {
			return Session{}, ErrInvalidInput
		}
		if runes := []rune(normalizedTitle); len(runes) > 120 {
			normalizedTitle = string(runes[:120])
		}
	}
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return Session{}, err
	}
	defer tx.Rollback()
	var lockedID int64
	if err := tx.QueryRowContext(ctx, `
		SELECT id FROM app_chat_sessions
		WHERE id=$1 AND app_user_id=$2 AND scene='skill_chat' FOR UPDATE`, sessionID, appUserID).Scan(&lockedID); errors.Is(err, sql.ErrNoRows) {
		return Session{}, ErrNotFound
	} else if err != nil {
		return Session{}, err
	}
	if clear {
		if err := deleteSkillSessionAudioAssets(ctx, tx, appUserID, sessionID); err != nil {
			return Session{}, err
		}
		if _, err := tx.ExecContext(ctx, `DELETE FROM app_chat_messages WHERE session_id=$1`, sessionID); err != nil {
			return Session{}, err
		}
		if _, err := tx.ExecContext(ctx, `
			UPDATE app_chat_sessions SET context_summary='',context_summary_through_message_id=0,
			  generation_revision=generation_revision+1,updated_at=now()
			WHERE id=$1`, sessionID); err != nil {
			return Session{}, err
		}
	}
	if title != nil {
		if _, err := tx.ExecContext(ctx, `UPDATE app_chat_sessions SET title=$2,updated_at=now() WHERE id=$1`, sessionID, normalizedTitle); err != nil {
			return Session{}, err
		}
	}
	if err := tx.Commit(); err != nil {
		return Session{}, err
	}
	return s.GetSession(ctx, appUserID, sessionID)
}

func scanMessage(row rowScanner) (chat.Message, error) {
	var item chat.Message
	var audioAssetID sql.NullInt64
	var createTime time.Time
	err := row.Scan(&item.ID, &item.SessionID, &item.Role, &item.Content, &item.Sources,
		&item.Favorite, &item.Feedback, &item.MessageType, &audioAssetID,
		&item.AudioDurationMs, &item.Transcript, &createTime)
	item.AudioAssetID = audioAssetID.Int64
	item.CreateTime = formatTime(createTime)
	if item.MessageType == "voice" && item.AudioAssetID > 0 {
		item.AudioURL = fmt.Sprintf("/api/app/skill-messages/%d/audio", item.ID)
	}
	return item, err
}

const messageSelect = `
	SELECT message.id, message.session_id, message.role, message.content, message.sources,
	       message.favorite, message.feedback, message.message_type, message.audio_asset_id,
	       message.audio_duration_ms, message.transcript, message.create_time
	FROM app_chat_messages message
	JOIN app_chat_sessions session ON session.id = message.session_id`

func (s *Store) ListMessages(ctx context.Context, appUserID, sessionID int64) ([]chat.Message, error) {
	if err := s.available(); err != nil {
		return nil, err
	}
	rows, err := s.db.QueryContext(ctx, messageSelect+`
		WHERE session.id = $1 AND session.app_user_id = $2 AND session.scene = 'skill_chat'
		ORDER BY message.create_time, message.id`, sessionID, appUserID)
	if err != nil {
		return nil, fmt.Errorf("list skill messages: %w", err)
	}
	defer rows.Close()
	out := make([]chat.Message, 0)
	for rows.Next() {
		item, err := scanMessage(rows)
		if err != nil {
			return nil, fmt.Errorf("list skill messages: scan: %w", err)
		}
		out = append(out, item)
	}
	return out, rows.Err()
}

func (s *Store) ListRecentMessages(ctx context.Context, appUserID, sessionID int64, limit int) ([]chat.Message, error) {
	if err := s.available(); err != nil {
		return nil, err
	}
	if limit <= 0 {
		return []chat.Message{}, nil
	}
	if limit > 50 {
		limit = 50
	}
	rows, err := s.db.QueryContext(ctx, `
		SELECT recent.id, recent.session_id, recent.role, recent.content, recent.sources,
		       recent.favorite, recent.feedback, recent.message_type, recent.audio_asset_id,
		       recent.audio_duration_ms, recent.transcript, recent.create_time
		FROM (
		  SELECT message.* FROM app_chat_messages message
		  JOIN app_chat_sessions session ON session.id = message.session_id
		  WHERE session.id = $1 AND session.app_user_id = $2 AND session.scene = 'skill_chat'
		    AND message.role IN ('user','assistant')
		  ORDER BY message.create_time DESC, message.id DESC LIMIT $3
		) recent ORDER BY recent.create_time, recent.id`, sessionID, appUserID, limit)
	if err != nil {
		return nil, fmt.Errorf("list recent skill messages: %w", err)
	}
	defer rows.Close()
	out := make([]chat.Message, 0, limit)
	for rows.Next() {
		item, err := scanMessage(rows)
		if err != nil {
			return nil, fmt.Errorf("list recent skill messages: scan: %w", err)
		}
		out = append(out, item)
	}
	return out, rows.Err()
}

func (s *Store) GetConversationState(ctx context.Context, appUserID, sessionID int64) (chat.ConversationState, error) {
	if err := s.available(); err != nil {
		return chat.ConversationState{}, err
	}
	var state chat.ConversationState
	err := s.db.QueryRowContext(ctx, `
		SELECT session.context_summary, session.context_summary_through_message_id
		FROM app_chat_sessions session
		WHERE session.id = $1 AND session.app_user_id = $2 AND session.scene = 'skill_chat'`, sessionID, appUserID).
		Scan(&state.Summary, &state.SummaryThroughMessageID)
	if errors.Is(err, sql.ErrNoRows) {
		return chat.ConversationState{}, ErrNotFound
	}
	return state, err
}

func (s *Store) ListMessagesAfter(ctx context.Context, appUserID, sessionID, afterMessageID int64) ([]chat.Message, error) {
	if err := s.available(); err != nil {
		return nil, err
	}
	rows, err := s.db.QueryContext(ctx, messageSelect+`
		WHERE session.id = $1 AND session.app_user_id = $2 AND session.scene = 'skill_chat'
		  AND message.id > $3 AND message.role IN ('user','assistant')
		ORDER BY message.id`, sessionID, appUserID, afterMessageID)
	if err != nil {
		return nil, fmt.Errorf("list skill messages after summary: %w", err)
	}
	defer rows.Close()
	out := make([]chat.Message, 0)
	for rows.Next() {
		item, err := scanMessage(rows)
		if err != nil {
			return nil, fmt.Errorf("list skill messages after summary: scan: %w", err)
		}
		out = append(out, item)
	}
	return out, rows.Err()
}

func (s *Store) UpdateConversationSummary(ctx context.Context, appUserID, sessionID, expectedRevision, expectedThroughMessageID int64, summary string, throughMessageID int64) (bool, error) {
	if err := s.available(); err != nil {
		return false, err
	}
	result, err := s.db.ExecContext(ctx, `
		UPDATE app_chat_sessions SET context_summary=$5,context_summary_through_message_id=$6
		WHERE id=$1 AND app_user_id=$2 AND scene='skill_chat'
		  AND generation_revision=$3 AND context_summary_through_message_id=$4`,
		sessionID, appUserID, expectedRevision, expectedThroughMessageID, strings.TrimSpace(summary), throughMessageID)
	if err != nil {
		return false, fmt.Errorf("update skill session summary: %w", err)
	}
	affected, err := result.RowsAffected()
	return affected > 0, err
}

func (s *Store) SavePair(ctx context.Context, appUserID, sessionID int64, trace GenerationTrace, question, answer string, sources json.RawMessage) (int64, error) {
	if err := s.available(); err != nil {
		return 0, err
	}
	question, answer = strings.TrimSpace(question), strings.TrimSpace(answer)
	if appUserID <= 0 || sessionID <= 0 || !trace.valid() || question == "" || answer == "" {
		return 0, ErrInvalidInput
	}
	if sources == nil {
		sources = json.RawMessage("[]")
	}
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return 0, err
	}
	defer tx.Rollback()
	var exists bool
	err = tx.QueryRowContext(ctx, `
		SELECT true FROM app_chat_sessions session
		JOIN app_skill_versions version ON version.id = session.skill_version_id
		JOIN app_skills skill ON skill.id = version.skill_id
		JOIN app_skill_categories category ON category.id = skill.category_id
		JOIN app_skill_libraries library ON library.id = category.library_id
		WHERE session.id = $1 AND session.app_user_id = $2 AND session.scene = 'skill_chat'
		  AND session.generation_revision = $3 AND version.id=$4 AND version.theory_release_id=$5
		  AND version.status IN ('published','retired')
		  AND skill.status = 'enabled' AND category.status = 'enabled' AND library.status = 'enabled'
		FOR UPDATE OF session`, sessionID, appUserID, trace.GenerationRevision, trace.SkillVersionID, trace.TheoryReleaseID).Scan(&exists)
	if errors.Is(err, sql.ErrNoRows) {
		return 0, classifySessionWriteMiss(ctx, tx, appUserID, sessionID, trace.GenerationRevision)
	}
	if err != nil {
		return 0, err
	}
	if _, err := tx.ExecContext(ctx, `
		INSERT INTO app_chat_messages(session_id, role, content, sources, message_type)
		VALUES ($1, 'user', $2, '[]'::jsonb, 'text')`, sessionID, question); err != nil {
		return 0, err
	}
	var messageID int64
	if err := tx.QueryRowContext(ctx, `
		INSERT INTO app_chat_messages(session_id, role, content, sources, message_type)
		VALUES ($1, 'assistant', $2, $3, 'text') RETURNING id`, sessionID, answer, sources).Scan(&messageID); err != nil {
		return 0, err
	}
	if err := insertGenerationTrace(ctx, tx, sessionID, messageID, trace); err != nil {
		return 0, err
	}
	if _, err := tx.ExecContext(ctx, `UPDATE app_chat_sessions SET updated_at = now() WHERE id = $1`, sessionID); err != nil {
		return 0, err
	}
	if err := tx.Commit(); err != nil {
		return 0, err
	}
	return messageID, nil
}

func (s *Store) SaveVoicePair(ctx context.Context, appUserID, sessionID int64, trace GenerationTrace, audioAssetID int64, durationMs int, transcript, answer string, sources json.RawMessage) (int64, int64, error) {
	if err := s.available(); err != nil {
		return 0, 0, err
	}
	transcript, answer = strings.TrimSpace(transcript), strings.TrimSpace(answer)
	if appUserID <= 0 || sessionID <= 0 || !trace.valid() || audioAssetID <= 0 || durationMs <= 0 || transcript == "" || answer == "" {
		return 0, 0, ErrInvalidInput
	}
	if sources == nil {
		sources = json.RawMessage("[]")
	}
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return 0, 0, err
	}
	defer tx.Rollback()
	var exists bool
	if err := tx.QueryRowContext(ctx, `
		SELECT true FROM app_chat_sessions session
		JOIN app_skill_versions version ON version.id = session.skill_version_id
		JOIN app_skills skill ON skill.id = version.skill_id
		JOIN app_skill_categories category ON category.id = skill.category_id
		JOIN app_skill_libraries library ON library.id = category.library_id
		WHERE session.id=$1 AND session.app_user_id=$2 AND session.scene='skill_chat'
		  AND session.generation_revision=$3 AND version.id=$4 AND version.theory_release_id=$5
		  AND version.status IN ('published','retired')
		  AND skill.status='enabled' AND category.status='enabled' AND library.status='enabled'
		FOR UPDATE OF session`, sessionID, appUserID, trace.GenerationRevision, trace.SkillVersionID, trace.TheoryReleaseID).Scan(&exists); errors.Is(err, sql.ErrNoRows) {
		return 0, 0, classifySessionWriteMiss(ctx, tx, appUserID, sessionID, trace.GenerationRevision)
	} else if err != nil {
		return 0, 0, err
	}
	var userMessageID int64
	err = tx.QueryRowContext(ctx, `
		INSERT INTO app_chat_messages(session_id,role,content,sources,message_type,audio_asset_id,audio_duration_ms,transcript)
		VALUES($1,'user','','[]'::jsonb,'voice',$2,$3,$4) RETURNING id`, sessionID, audioAssetID, durationMs, transcript).Scan(&userMessageID)
	if err != nil {
		return 0, 0, err
	}
	var assistantMessageID int64
	err = tx.QueryRowContext(ctx, `
		INSERT INTO app_chat_messages(session_id,role,content,sources,message_type)
		VALUES($1,'assistant',$2,$3,'text') RETURNING id`, sessionID, answer, sources).Scan(&assistantMessageID)
	if err != nil {
		return 0, 0, err
	}
	if err := insertGenerationTrace(ctx, tx, sessionID, assistantMessageID, trace); err != nil {
		return 0, 0, err
	}
	if _, err := tx.ExecContext(ctx, `UPDATE app_chat_sessions SET updated_at=now() WHERE id=$1`, sessionID); err != nil {
		return 0, 0, err
	}
	if err := tx.Commit(); err != nil {
		return 0, 0, err
	}
	return userMessageID, assistantMessageID, nil
}

func (t GenerationTrace) valid() bool {
	if t.GenerationRevision < 0 || t.SkillVersionID <= 0 || t.TheoryReleaseID <= 0 {
		return false
	}
	for _, id := range t.ChunkIDs {
		if id <= 0 {
			return false
		}
	}
	return true
}

func insertGenerationTrace(ctx context.Context, tx *sql.Tx, sessionID, assistantMessageID int64, trace GenerationTrace) error {
	_, err := tx.ExecContext(ctx, `
		INSERT INTO app_skill_chat_traces(session_id,assistant_message_id,generation_revision,
		  skill_version_id,theory_release_id,chunk_ids)
		VALUES($1,$2,$3,$4,$5,$6)`, sessionID, assistantMessageID, trace.GenerationRevision,
		trace.SkillVersionID, trace.TheoryReleaseID, trace.ChunkIDs)
	if err != nil {
		return fmt.Errorf("save skill generation trace: %w", err)
	}
	return nil
}

func (s *Store) SaveVoiceMessage(ctx context.Context, appUserID, sessionID, expectedRevision, audioAssetID int64, durationMs int, transcript string) (int64, error) {
	if err := s.available(); err != nil {
		return 0, err
	}
	var messageID int64
	err := s.db.QueryRowContext(ctx, `
		INSERT INTO app_chat_messages(session_id,role,content,sources,message_type,audio_asset_id,audio_duration_ms,transcript)
		SELECT session.id,'user','','[]'::jsonb,'voice',$4,$5,$6
		FROM app_chat_sessions session
		JOIN app_skill_versions version ON version.id=session.skill_version_id
		JOIN app_skills skill ON skill.id=version.skill_id
		JOIN app_skill_categories category ON category.id=skill.category_id
		JOIN app_skill_libraries library ON library.id=category.library_id
		WHERE session.id=$1 AND session.app_user_id=$2 AND session.scene='skill_chat'
		  AND session.generation_revision=$3
		  AND version.status IN ('published','retired')
		  AND skill.status='enabled' AND category.status='enabled' AND library.status='enabled'
		RETURNING id`, sessionID, appUserID, expectedRevision, audioAssetID, durationMs, strings.TrimSpace(transcript)).Scan(&messageID)
	if errors.Is(err, sql.ErrNoRows) {
		var revision int64
		lookupErr := s.db.QueryRowContext(ctx, `SELECT generation_revision FROM app_chat_sessions WHERE id=$1 AND app_user_id=$2 AND scene='skill_chat'`, sessionID, appUserID).Scan(&revision)
		if lookupErr == nil && revision != expectedRevision {
			return 0, ErrSessionChanged
		}
		return 0, ErrNotFound
	}
	if err == nil {
		_, err = s.db.ExecContext(ctx, `UPDATE app_chat_sessions SET updated_at=now() WHERE id=$1`, sessionID)
	}
	return messageID, err
}

func classifySessionWriteMiss(ctx context.Context, tx *sql.Tx, appUserID, sessionID, expectedRevision int64) error {
	var revision int64
	err := tx.QueryRowContext(ctx, `
		SELECT generation_revision FROM app_chat_sessions
		WHERE id=$1 AND app_user_id=$2 AND scene='skill_chat'`, sessionID, appUserID).Scan(&revision)
	if err == nil && revision != expectedRevision {
		return ErrSessionChanged
	}
	return ErrNotFound
}

func (s *Store) GetVoiceAudioAssetID(ctx context.Context, appUserID, messageID int64) (int64, error) {
	var assetID int64
	err := s.db.QueryRowContext(ctx, `
		SELECT message.audio_asset_id FROM app_chat_messages message
		JOIN app_chat_sessions session ON session.id=message.session_id
		WHERE message.id=$1 AND session.app_user_id=$2 AND session.scene='skill_chat'
		  AND message.role='user' AND message.message_type='voice' AND message.audio_asset_id IS NOT NULL`, messageID, appUserID).Scan(&assetID)
	if errors.Is(err, sql.ErrNoRows) {
		return 0, ErrNotFound
	}
	return assetID, err
}

func (s *Store) GetVoiceTranscript(ctx context.Context, appUserID, messageID int64) (string, error) {
	var transcript string
	err := s.db.QueryRowContext(ctx, `
		SELECT message.transcript FROM app_chat_messages message
		JOIN app_chat_sessions session ON session.id=message.session_id
		WHERE message.id=$1 AND session.app_user_id=$2 AND session.scene='skill_chat'
		  AND message.role='user' AND message.message_type='voice'`, messageID, appUserID).Scan(&transcript)
	if errors.Is(err, sql.ErrNoRows) {
		return "", ErrNotFound
	}
	return strings.TrimSpace(transcript), err
}
