package chat

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"strings"
)

type SuggestionTurn struct {
	SessionID int64
	Question  string
	Answer    string
}

// GetSuggestionTurn reads only the requested user's assistant turn in the
// selected scene. Realtime turns carry an explicit user-message association;
// legacy atomic text/voice pairs use their shared transaction create_time.
func (s *Store) GetSuggestionTurn(ctx context.Context, userID, sessionID, messageID int64, scene string) (SuggestionTurn, error) {
	var turn SuggestionTurn
	if userID <= 0 || messageID <= 0 || sessionID < 0 || !suggestionSceneAllowed(scene) || (sessionID == 0 && scene != "xinzhili_voice") {
		return turn, ErrNotFound
	}
	if s == nil || s.db == nil {
		return turn, errors.New("chat: store unavailable")
	}
	skillAvailability := ""
	if scene == "skill_chat" {
		skillAvailability = ` AND EXISTS (
		 SELECT 1 FROM app_skill_versions version
		 JOIN app_skills skill ON skill.id=version.skill_id
		 JOIN app_skill_categories category ON category.id=skill.category_id
		 JOIN app_skill_libraries library ON library.id=category.library_id
		 WHERE version.id=s.skill_version_id AND version.theory_release_id > 0 AND version.status IN ('published','retired')
		 AND skill.status='enabled' AND category.status='enabled' AND library.status='enabled')`
	}
	err := s.db.QueryRowContext(ctx, `
	 SELECT s.id, previous.question, m.content
	 FROM app_chat_messages m
	 JOIN app_chat_sessions s ON s.id=m.session_id
	 JOIN LATERAL (
	   SELECT CASE WHEN u.message_type='voice' THEN u.transcript ELSE u.content END AS question
	   FROM app_chat_messages u
	   WHERE u.session_id=s.id AND u.role = 'user'
	     AND (u.id = m.reply_to_message_id OR (m.reply_to_message_id IS NULL AND u.create_time = m.create_time AND u.id < m.id))
	     AND btrim(CASE WHEN u.message_type='voice' THEN u.transcript ELSE u.content END) <> ''
	   ORDER BY u.id DESC
	   LIMIT 1
	 ) previous ON true
	 WHERE s.app_user_id = $1 AND m.id = $2 AND ($3=0 OR s.id = $3)
	   AND s.scene = $4 AND m.role = 'assistant' AND btrim(m.content) <> ''
	   AND (s.scene <> 'xinzhili_voice' OR m.delivery_status IS NULL OR m.delivery_status IN ('generated','played'))`+skillAvailability,
		userID, messageID, sessionID, scene).Scan(&turn.SessionID, &turn.Question, &turn.Answer)
	if errors.Is(err, sql.ErrNoRows) {
		return turn, ErrNotFound
	}
	if err != nil {
		return turn, fmt.Errorf("read suggestion turn: %w", err)
	}
	turn.Question, turn.Answer = strings.TrimSpace(turn.Question), strings.TrimSpace(turn.Answer)
	if turn.Question == "" || turn.Answer == "" {
		return SuggestionTurn{}, ErrNotFound
	}
	return turn, nil
}

// CreateSceneAssistantForUser binds a realtime answer to its exact saved
// question rather than relying on adjacency across concurrent connections.
func (s *Store) CreateSceneAssistantForUser(ctx context.Context, sessionID, userMessageID int64, content, mode string) (int64, error) {
	content, mode = strings.TrimSpace(content), strings.TrimSpace(mode)
	if sessionID <= 0 || userMessageID <= 0 || content == "" || mode == "" {
		return 0, errors.New("chat: invalid scene assistant message")
	}
	var messageID int64
	err := s.db.QueryRowContext(ctx, `
	 INSERT INTO app_chat_messages(session_id,role,content,sources,message_type,delivery_status,delivered_text,xinzhili_mode,reply_to_message_id)
	 SELECT s.id,'assistant',$3,'[]'::jsonb,'text','sent','',$4,u.id
	 FROM app_chat_sessions s JOIN app_chat_messages u ON u.session_id=s.id
	 WHERE s.id=$1 AND s.scene='xinzhili_voice' AND u.id=$2 AND u.role='user'
	 RETURNING id`, sessionID, userMessageID, content, mode).Scan(&messageID)
	if errors.Is(err, sql.ErrNoRows) {
		return 0, ErrNotFound
	}
	return messageID, err
}

func suggestionSceneAllowed(scene string) bool {
	if scene == "chat" || scene == "skill_chat" || scene == "xinzhili_voice" {
		return true
	}
	for value := 1; value <= 9; value++ {
		if scene == fmt.Sprintf("enneagram_%d", value) {
			return true
		}
	}
	return false
}
