package directmedia

import (
	"context"
	"database/sql"
	"errors"
	"strconv"

	"nine-xing/nx-backend/apps/server/internal/uploadasset"
)

var ErrNotParticipant = errors.New("direct_media.not_participant")

type Media struct {
	ID             int64  `json:"id"`
	ConversationID int64  `json:"conversationId"`
	UploaderID     int64  `json:"uploaderId"`
	MediaType      string `json:"mediaType"`
	DurationMs     int    `json:"durationMs"`
	URL            string `json:"url"`
}

type Store struct {
	db      *sql.DB
	uploads *uploadasset.Store
}

func NewStore(db *sql.DB, uploads *uploadasset.Store) *Store {
	return &Store{db: db, uploads: uploads}
}

func (s *Store) Create(ctx context.Context, userID, conversationID int64, mediaType string, durationMs int, input uploadasset.CreateInput) (Media, error) {
	if s == nil || s.db == nil || s.uploads == nil {
		return Media{}, errors.New("direct media store is not configured")
	}
	ok, err := s.isParticipant(ctx, userID, conversationID)
	if err != nil {
		return Media{}, err
	}
	if !ok {
		return Media{}, ErrNotParticipant
	}
	if (mediaType != "image" && mediaType != "voice") || durationMs < 0 || durationMs > 60000 {
		return Media{}, errors.New("direct_media.invalid")
	}
	asset, err := s.uploads.Create(ctx, input)
	if err != nil {
		return Media{}, err
	}
	var item Media
	err = s.db.QueryRowContext(ctx, `INSERT INTO direct_message_media(conversation_id,uploader_id,asset_id,media_type,duration_ms) VALUES($1,$2,$3,$4,$5) RETURNING id,conversation_id,uploader_id,media_type,duration_ms`, conversationID, userID, asset.ID, mediaType, durationMs).Scan(&item.ID, &item.ConversationID, &item.UploaderID, &item.MediaType, &item.DurationMs)
	item.URL = "/api/app/direct/media/" + strconv.FormatInt(item.ID, 10)
	return item, err
}

func (s *Store) ValidateForMessage(ctx context.Context, userID, conversationID, mediaID int64, mediaType string) error {
	if s == nil || s.db == nil {
		return errors.New("direct media store is not configured")
	}
	var ok bool
	err := s.db.QueryRowContext(ctx, `SELECT EXISTS(SELECT 1 FROM direct_message_media WHERE id=$1 AND conversation_id=$2 AND uploader_id=$3 AND media_type=$4)`, mediaID, conversationID, userID, mediaType).Scan(&ok)
	if err != nil {
		return err
	}
	if !ok {
		return ErrNotParticipant
	}
	return nil
}

func (s *Store) FindAsset(ctx context.Context, userID, mediaID int64) (uploadasset.Asset, error) {
	if s == nil || s.db == nil || s.uploads == nil {
		return uploadasset.Asset{}, errors.New("direct media store is not configured")
	}
	var assetID int64
	err := s.db.QueryRowContext(ctx, `SELECT m.asset_id FROM direct_message_media m JOIN direct_conversations c ON c.id=m.conversation_id AND c.status='active' WHERE m.id=$1 AND (c.user_low_id=$2 OR c.user_high_id=$2)`, mediaID, userID).Scan(&assetID)
	if errors.Is(err, sql.ErrNoRows) {
		return uploadasset.Asset{}, ErrNotParticipant
	}
	if err != nil {
		return uploadasset.Asset{}, err
	}
	return s.uploads.Find(ctx, assetID)
}

func (s *Store) isParticipant(ctx context.Context, userID, conversationID int64) (bool, error) {
	var ok bool
	err := s.db.QueryRowContext(ctx, `SELECT EXISTS(SELECT 1 FROM direct_conversations WHERE id=$1 AND status='active' AND (user_low_id=$2 OR user_high_id=$2))`, conversationID, userID).Scan(&ok)
	return ok, err
}
