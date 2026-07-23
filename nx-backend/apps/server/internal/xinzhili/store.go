package xinzhili

import (
	"context"
	"database/sql"
	"errors"
	"strings"
)

var (
	ErrModePreferenceConflict    = errors.New("xinzhili: mode preference conflict")
	ErrDeliveryNotFound          = errors.New("xinzhili: delivery not found")
	ErrInvalidDeliveryTransition = errors.New("xinzhili: invalid delivery transition")
	ErrInvalidDeliveredText      = errors.New("xinzhili: invalid delivered text")
)

type ModePreference struct {
	UserID    int64
	Requested Mode
	Revision  int64
}

type DeliveryStatus string

const (
	DeliveryGenerated    DeliveryStatus = "generated"
	DeliverySynthesizing DeliveryStatus = "synthesizing"
	DeliverySent         DeliveryStatus = "sent"
	DeliveryPlayed       DeliveryStatus = "played"
	DeliveryTTSFailed    DeliveryStatus = "tts_failed"
	DeliveryInterrupted  DeliveryStatus = "interrupted"
	DeliveryUnconfirmed  DeliveryStatus = "unconfirmed"
)

type DeliveryState struct {
	MessageID     int64
	Status        DeliveryStatus
	DeliveredText string
}

type Store struct {
	db *sql.DB
}

func NewStore(db *sql.DB) *Store {
	return &Store{db: db}
}

func (s *Store) ReadMode(ctx context.Context, userID int64) (ModePreference, bool, error) {
	var preference ModePreference
	var requested string
	err := s.db.QueryRowContext(ctx,
		`SELECT app_user_id, requested_mode, revision
		 FROM app_xinzhili_mode_preferences WHERE app_user_id=$1`, userID,
	).Scan(&preference.UserID, &requested, &preference.Revision)
	if errors.Is(err, sql.ErrNoRows) {
		return ModePreference{}, false, nil
	}
	if err != nil {
		return ModePreference{}, false, err
	}
	preference.Requested = Mode(requested)
	return preference, true, nil
}

func (s *Store) UpdateMode(ctx context.Context, userID int64, mode Mode, expectedRevision int64) (ModePreference, error) {
	mode = Mode(strings.TrimSpace(string(mode)))
	if userID <= 0 || expectedRevision < 0 || !knownMode(mode) {
		return ModePreference{}, ErrModePreferenceConflict
	}
	var preference ModePreference
	var requested string
	if expectedRevision == 0 {
		err := s.db.QueryRowContext(ctx,
			`INSERT INTO app_xinzhili_mode_preferences(app_user_id, requested_mode, revision, update_time)
			 VALUES ($1,$2,1,now()) ON CONFLICT (app_user_id) DO NOTHING
			 RETURNING app_user_id, requested_mode, revision`, userID, string(mode),
		).Scan(&preference.UserID, &requested, &preference.Revision)
		if errors.Is(err, sql.ErrNoRows) {
			return ModePreference{}, ErrModePreferenceConflict
		}
		if err != nil {
			return ModePreference{}, err
		}
	} else {
		err := s.db.QueryRowContext(ctx,
			`UPDATE app_xinzhili_mode_preferences
			 SET requested_mode=$2, revision=revision+1, update_time=now()
			 WHERE app_user_id=$1 AND revision=$3
			 RETURNING app_user_id, requested_mode, revision`, userID, string(mode), expectedRevision,
		).Scan(&preference.UserID, &requested, &preference.Revision)
		if errors.Is(err, sql.ErrNoRows) {
			return ModePreference{}, ErrModePreferenceConflict
		}
		if err != nil {
			return ModePreference{}, err
		}
	}
	preference.Requested = Mode(requested)
	return preference, nil
}

func (s *Store) UpdateDelivery(ctx context.Context, messageID int64, next DeliveryStatus, deliveredText string) (DeliveryState, error) {
	if !knownDeliveryStatus(next) {
		return DeliveryState{}, ErrInvalidDeliveryTransition
	}
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return DeliveryState{}, err
	}
	defer func() { _ = tx.Rollback() }()

	var currentRaw sql.NullString
	var currentText sql.NullString
	var content string
	err = tx.QueryRowContext(ctx,
		`SELECT m.delivery_status, m.delivered_text, m.content
		 FROM app_chat_messages m
		 JOIN app_chat_sessions s ON s.id=m.session_id
		 WHERE m.id=$1 AND m.role='assistant' AND s.scene='xinzhili_voice'
		 FOR UPDATE OF m`, messageID,
	).Scan(&currentRaw, &currentText, &content)
	if errors.Is(err, sql.ErrNoRows) || !currentRaw.Valid {
		return DeliveryState{}, ErrDeliveryNotFound
	}
	if err != nil {
		return DeliveryState{}, err
	}

	current := DeliveryStatus(currentRaw.String)
	previousText := currentText.String
	if !strings.HasPrefix(content, deliveredText) || !strings.HasPrefix(deliveredText, previousText) {
		return DeliveryState{}, ErrInvalidDeliveredText
	}
	textAdvanced := previousText != deliveredText
	if current == next {
		if textAdvanced && current != DeliverySent {
			return DeliveryState{}, ErrInvalidDeliveryTransition
		}
	} else {
		if !deliveryTransitionAllowed(current, next) {
			return DeliveryState{}, ErrInvalidDeliveryTransition
		}
		if textAdvanced && next != DeliveryPlayed && next != DeliveryInterrupted {
			return DeliveryState{}, ErrInvalidDeliveryTransition
		}
	}
	if next == DeliveryPlayed && deliveredText != content {
		return DeliveryState{}, ErrInvalidDeliveredText
	}
	if current != next || previousText != deliveredText {
		if _, err := tx.ExecContext(ctx,
			`UPDATE app_chat_messages SET delivery_status=$2, delivered_text=$3 WHERE id=$1`,
			messageID, string(next), deliveredText,
		); err != nil {
			return DeliveryState{}, err
		}
	}
	if err := tx.Commit(); err != nil {
		return DeliveryState{}, err
	}
	return DeliveryState{MessageID: messageID, Status: next, DeliveredText: deliveredText}, nil
}

func knownDeliveryStatus(status DeliveryStatus) bool {
	switch status {
	case DeliveryGenerated, DeliverySynthesizing, DeliverySent, DeliveryPlayed,
		DeliveryTTSFailed, DeliveryInterrupted, DeliveryUnconfirmed:
		return true
	default:
		return false
	}
}

func deliveryTransitionAllowed(current, next DeliveryStatus) bool {
	switch current {
	case DeliveryGenerated:
		return next == DeliverySynthesizing || next == DeliveryTTSFailed
	case DeliverySynthesizing:
		return next == DeliverySent || next == DeliveryTTSFailed
	case DeliverySent:
		return next == DeliveryPlayed || next == DeliveryInterrupted || next == DeliveryUnconfirmed
	case DeliveryUnconfirmed:
		return next == DeliveryPlayed
	default:
		return false
	}
}
