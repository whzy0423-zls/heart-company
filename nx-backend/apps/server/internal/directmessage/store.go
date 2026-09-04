package directmessage

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"strings"
	"time"
)

type Store struct{ db *sql.DB }

func NewStore(db *sql.DB) *Store { return &Store{db: db} }

func (s *Store) requireDB() error {
	if s == nil || s.db == nil {
		return errors.New("direct message database is not configured")
	}
	return nil
}

func (s *Store) GetOrCreateConversation(ctx context.Context, userA, userB int64) (Conversation, error) {
	if err := s.requireDB(); err != nil {
		return Conversation{}, err
	}
	low, high, err := normalizePair(userA, userB)
	if err != nil {
		return Conversation{}, err
	}
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return Conversation{}, err
	}
	defer tx.Rollback()
	if blocked, err := blockedEither(ctx, tx, low, high); err != nil {
		return Conversation{}, err
	} else if blocked {
		return Conversation{}, ErrBlocked
	}
	var friends bool
	if err := tx.QueryRowContext(ctx, `SELECT EXISTS(SELECT 1 FROM friendships WHERE user_low_id=$1 AND user_high_id=$2 AND status='active')`, low, high).Scan(&friends); err != nil {
		return Conversation{}, err
	}
	if !friends {
		return Conversation{}, ErrNotFriend
	}
	if _, err := tx.ExecContext(ctx, `INSERT INTO direct_conversations(user_low_id,user_high_id) VALUES($1,$2) ON CONFLICT (user_low_id,user_high_id) DO NOTHING`, low, high); err != nil {
		return Conversation{}, err
	}
	var item Conversation
	if err := tx.QueryRowContext(ctx, `SELECT id,user_low_id,user_high_id,event_sequence,updated_at FROM direct_conversations WHERE user_low_id=$1 AND user_high_id=$2`, low, high).Scan(&item.ID, &item.UserLowID, &item.UserHighID, &item.EventSequence, &item.UpdatedAt); err != nil {
		return Conversation{}, err
	}
	if err := tx.Commit(); err != nil {
		return Conversation{}, err
	}
	return item, nil
}

func (s *Store) ListConversations(ctx context.Context, userID int64) ([]Conversation, error) {
	if err := s.requireDB(); err != nil {
		return nil, err
	}
	rows, err := s.db.QueryContext(ctx, `SELECT id,user_low_id,user_high_id,event_sequence,updated_at FROM direct_conversations WHERE status='active' AND (user_low_id=$1 OR user_high_id=$1) ORDER BY updated_at DESC,id DESC`, userID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	items := []Conversation{}
	for rows.Next() {
		var item Conversation
		if err := rows.Scan(&item.ID, &item.UserLowID, &item.UserHighID, &item.EventSequence, &item.UpdatedAt); err != nil {
			return nil, err
		}
		items = append(items, item)
	}
	return items, rows.Err()
}

func (s *Store) Send(ctx context.Context, input SendInput) (Message, error) {
	if err := s.requireDB(); err != nil {
		return Message{}, err
	}
	if input.ConversationID <= 0 || input.SenderID <= 0 || strings.TrimSpace(input.ClientMessageID) == "" {
		return Message{}, ErrInvalidConversation
	}
	if input.MessageType == "" {
		input.MessageType = "text"
	}
	if input.MessageType != "text" && input.MessageType != "image" && input.MessageType != "voice" && input.MessageType != "sticker" {
		return Message{}, ErrInvalidConversation
	}
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return Message{}, err
	}
	defer tx.Rollback()
	var low, high int64
	if err := tx.QueryRowContext(ctx, `SELECT user_low_id,user_high_id FROM direct_conversations WHERE id=$1 AND status='active' FOR UPDATE`, input.ConversationID).Scan(&low, &high); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return Message{}, ErrInvalidConversation
		}
		return Message{}, err
	}
	if input.SenderID != low && input.SenderID != high {
		return Message{}, ErrNotParticipant
	}
	if blocked, err := blockedEither(ctx, tx, low, high); err != nil {
		return Message{}, err
	} else if blocked {
		return Message{}, ErrBlocked
	}
	var mediaID int64
	if input.MediaID != nil {
		mediaID = *input.MediaID
	}
	hash := PayloadHash(input.MessageType, input.Body, mediaID)
	var existing Message
	var existingHash string
	var existingMedia sql.NullInt64
	err = tx.QueryRowContext(ctx, `SELECT id,conversation_id,sender_id,client_message_id,payload_hash,message_type,body,media_id,sequence_no,recalled_at,created_at FROM direct_messages WHERE conversation_id=$1 AND client_message_id=$2`, input.ConversationID, input.ClientMessageID).Scan(&existing.ID, &existing.ConversationID, &existing.SenderID, &existing.ClientMessageID, &existingHash, &existing.MessageType, &existing.Body, &existingMedia, &existing.SequenceNo, &existing.RecalledAt, &existing.CreatedAt)
	if err == nil {
		if !SamePayload(existingHash, hash) {
			return Message{}, ErrPayloadConflict
		}
		if existingMedia.Valid {
			existing.MediaID = &existingMedia.Int64
		}
		return existing, nil
	}
	if !errors.Is(err, sql.ErrNoRows) {
		return Message{}, err
	}
	var sequence int64
	if err := tx.QueryRowContext(ctx, `UPDATE direct_conversations SET event_sequence=event_sequence+1, updated_at=now() WHERE id=$1 RETURNING event_sequence`, input.ConversationID).Scan(&sequence); err != nil {
		return Message{}, err
	}
	var item Message
	if err := tx.QueryRowContext(ctx, `INSERT INTO direct_messages(conversation_id,sender_id,client_message_id,payload_hash,message_type,body,media_id,sequence_no) VALUES($1,$2,$3,$4,$5,$6,$7,$8) RETURNING id,conversation_id,sender_id,client_message_id,message_type,body,media_id,sequence_no,recalled_at,created_at`, input.ConversationID, input.SenderID, input.ClientMessageID, hash, input.MessageType, input.Body, input.MediaID, sequence).Scan(&item.ID, &item.ConversationID, &item.SenderID, &item.ClientMessageID, &item.MessageType, &item.Body, &existingMedia, &item.SequenceNo, &item.RecalledAt, &item.CreatedAt); err != nil {
		return Message{}, err
	}
	if existingMedia.Valid {
		item.MediaID = &existingMedia.Int64
	}
	if err := tx.Commit(); err != nil {
		return Message{}, err
	}
	return item, nil
}

func (s *Store) History(ctx context.Context, userID, conversationID int64, cursor HistoryCursor, limit int) ([]Message, error) {
	if err := s.requireDB(); err != nil {
		return nil, err
	}
	if _, err := NormalizeHistoryCursor(cursor.Before, cursor.After); err != nil {
		return nil, err
	}
	if limit < 1 || limit > 100 {
		limit = 50
	}
	if !s.participant(ctx, userID, conversationID) {
		return nil, ErrNotParticipant
	}
	where := "conversation_id=$1"
	args := []any{conversationID}
	order := "sequence_no DESC"
	if cursor.Before > 0 {
		args = append(args, cursor.Before)
		where += fmt.Sprintf(" AND sequence_no < $%d", len(args))
	}
	if cursor.After > 0 {
		args = append(args, cursor.After)
		where += fmt.Sprintf(" AND sequence_no > $%d", len(args))
		order = "sequence_no ASC"
	}
	args = append(args, limit)
	rows, err := s.db.QueryContext(ctx, `SELECT id,conversation_id,sender_id,client_message_id,message_type,body,media_id,sequence_no,recalled_at,created_at FROM direct_messages WHERE `+where+` ORDER BY `+order+` LIMIT $`+fmt.Sprint(len(args)), args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	items := []Message{}
	for rows.Next() {
		var m Message
		var media sql.NullInt64
		if err := rows.Scan(&m.ID, &m.ConversationID, &m.SenderID, &m.ClientMessageID, &m.MessageType, &m.Body, &media, &m.SequenceNo, &m.RecalledAt, &m.CreatedAt); err != nil {
			return nil, err
		}
		if media.Valid {
			m.MediaID = &media.Int64
		}
		items = append(items, m)
	}
	return items, rows.Err()
}

func (s *Store) MarkRead(ctx context.Context, userID, conversationID, sequence int64) error {
	if err := s.requireDB(); err != nil {
		return err
	}
	if sequence < 0 {
		return ErrCursorConflict
	}
	if !s.participant(ctx, userID, conversationID) {
		return ErrNotParticipant
	}
	_, err := s.db.ExecContext(ctx, `INSERT INTO direct_message_read_cursors(conversation_id,user_id,last_read_sequence) VALUES($1,$2,$3) ON CONFLICT(conversation_id,user_id) DO UPDATE SET last_read_sequence=GREATEST(direct_message_read_cursors.last_read_sequence,EXCLUDED.last_read_sequence),updated_at=now()`, conversationID, userID, sequence)
	return err
}

func (s *Store) Recall(ctx context.Context, userID, messageID int64) (Message, error) {
	if err := s.requireDB(); err != nil {
		return Message{}, err
	}
	var m Message
	var media sql.NullInt64
	err := s.db.QueryRowContext(ctx, `SELECT id,conversation_id,sender_id,client_message_id,message_type,body,media_id,sequence_no,recalled_at,created_at FROM direct_messages WHERE id=$1`, messageID).Scan(&m.ID, &m.ConversationID, &m.SenderID, &m.ClientMessageID, &m.MessageType, &m.Body, &media, &m.SequenceNo, &m.RecalledAt, &m.CreatedAt)
	if errors.Is(err, sql.ErrNoRows) {
		return Message{}, ErrMessageNotFound
	}
	if err != nil {
		return Message{}, err
	}
	if m.SenderID != userID {
		return Message{}, ErrNotParticipant
	}
	if !CanRecall(time.Since(m.CreatedAt)) {
		return Message{}, ErrRecallWindow
	}
	err = s.db.QueryRowContext(ctx, `UPDATE direct_messages SET recalled_at=COALESCE(recalled_at,now()),body='' WHERE id=$1 RETURNING recalled_at`, messageID).Scan(&m.RecalledAt)
	if media.Valid {
		m.MediaID = &media.Int64
	}
	return m, err
}

func (s *Store) participant(ctx context.Context, userID, conversationID int64) bool {
	var ok bool
	if s == nil || s.db == nil {
		return false
	}
	_ = s.db.QueryRowContext(ctx, `SELECT EXISTS(SELECT 1 FROM direct_conversations WHERE id=$1 AND status='active' AND (user_low_id=$2 OR user_high_id=$2))`, conversationID, userID).Scan(&ok)
	return ok
}
func normalizePair(a, b int64) (int64, int64, error) {
	if a <= 0 || b <= 0 || a == b {
		return 0, 0, ErrInvalidConversation
	}
	if a > b {
		return b, a, nil
	}
	return a, b, nil
}
func blockedEither(ctx context.Context, tx *sql.Tx, a, b int64) (bool, error) {
	var blocked bool
	err := tx.QueryRowContext(ctx, `SELECT EXISTS(SELECT 1 FROM user_blocks WHERE status='active' AND ((blocker_id=$1 AND blocked_id=$2) OR (blocker_id=$2 AND blocked_id=$1)))`, a, b).Scan(&blocked)
	return blocked, err
}
