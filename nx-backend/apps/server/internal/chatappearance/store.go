package chatappearance

import (
	"context"
	"database/sql"
	"errors"
	"strings"
)

type Appearance struct {
	ConversationID  int64  `json:"conversationId"`
	UserID          int64  `json:"userId"`
	BackgroundType  string `json:"backgroundType"`
	BackgroundValue string `json:"backgroundValue"`
}
type Store struct{ db *sql.DB }

var ErrNotParticipant = errors.New("chat_appearance.not_participant")

func NewStore(db *sql.DB) *Store { return &Store{db: db} }
func (s *Store) Get(ctx context.Context, userID, conversationID int64) (Appearance, error) {
	if err := s.requireDB(); err != nil {
		return Appearance{}, err
	}
	if !s.participant(ctx, userID, conversationID) {
		return Appearance{}, ErrNotParticipant
	}
	var a Appearance
	err := s.db.QueryRowContext(ctx, `SELECT conversation_id,user_id,background_type,background_value FROM direct_chat_appearances WHERE user_id=$1 AND conversation_id=$2`, userID, conversationID).Scan(&a.ConversationID, &a.UserID, &a.BackgroundType, &a.BackgroundValue)
	if errors.Is(err, sql.ErrNoRows) {
		return Appearance{ConversationID: conversationID, UserID: userID, BackgroundType: "preset", BackgroundValue: "default"}, nil
	}
	return a, err
}
func (s *Store) Upsert(ctx context.Context, userID, conversationID int64, kind, value string) (Appearance, error) {
	if err := s.requireDB(); err != nil {
		return Appearance{}, err
	}
	if !s.participant(ctx, userID, conversationID) {
		return Appearance{}, ErrNotParticipant
	}
	kind = strings.TrimSpace(kind)
	value = strings.TrimSpace(value)
	if kind != "preset" && kind != "color" && kind != "image" {
		return Appearance{}, errors.New("chat_appearance.invalid_type")
	}
	if value == "" {
		return Appearance{}, errors.New("chat_appearance.invalid_value")
	}
	var a Appearance
	err := s.db.QueryRowContext(ctx, `INSERT INTO direct_chat_appearances(conversation_id,user_id,background_type,background_value) VALUES($1,$2,$3,$4) ON CONFLICT(conversation_id,user_id) DO UPDATE SET background_type=EXCLUDED.background_type,background_value=EXCLUDED.background_value,updated_at=now() RETURNING conversation_id,user_id,background_type,background_value`, conversationID, userID, kind, value).Scan(&a.ConversationID, &a.UserID, &a.BackgroundType, &a.BackgroundValue)
	return a, err
}

func (s *Store) requireDB() error {
	if s == nil || s.db == nil {
		return errors.New("chat appearance database is not configured")
	}
	return nil
}

func (s *Store) participant(ctx context.Context, userID, conversationID int64) bool {
	if s == nil || s.db == nil || userID <= 0 || conversationID <= 0 {
		return false
	}
	var ok bool
	_ = s.db.QueryRowContext(ctx, `SELECT EXISTS(SELECT 1 FROM direct_conversations WHERE id=$1 AND status='active' AND (user_low_id=$2 OR user_high_id=$2))`, conversationID, userID).Scan(&ok)
	return ok
}
