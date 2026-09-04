package directmessage

import (
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"strconv"
	"strings"
	"time"
)

var (
	ErrInvalidConversation = errors.New("direct_message.invalid_conversation")
	ErrNotParticipant      = errors.New("direct_message.not_participant")
	ErrNotFriend           = errors.New("direct_message.not_friend")
	ErrBlocked             = errors.New("direct_message.blocked")
	ErrCursorConflict      = errors.New("direct_message.cursor_conflict")
	ErrPayloadConflict     = errors.New("direct_message.payload_conflict")
	ErrMessageNotFound     = errors.New("direct_message.message_not_found")
	ErrRecallWindow        = errors.New("direct_message.recall_window")
)

type HistoryCursor struct {
	Before int64
	After  int64
}

type Conversation struct {
	ID            int64     `json:"id"`
	UserLowID     int64     `json:"userLowId"`
	UserHighID    int64     `json:"userHighId"`
	EventSequence int64     `json:"eventSequence"`
	UpdatedAt     time.Time `json:"updatedAt"`
}

type Message struct {
	ID              int64      `json:"id"`
	ConversationID  int64      `json:"conversationId"`
	SenderID        int64      `json:"senderId"`
	ClientMessageID string     `json:"clientMessageId"`
	MessageType     string     `json:"messageType"`
	Body            string     `json:"body"`
	MediaID         *int64     `json:"mediaId,omitempty"`
	SequenceNo      int64      `json:"sequenceNo"`
	RecalledAt      *time.Time `json:"recalledAt,omitempty"`
	CreatedAt       time.Time  `json:"createdAt"`
}

type SendInput struct {
	ConversationID  int64
	SenderID        int64
	ClientMessageID string
	MessageType     string
	Body            string
	MediaID         *int64
}

func NormalizeHistoryCursor(before, after int64) (HistoryCursor, error) {
	if before < 0 || after < 0 {
		return HistoryCursor{}, ErrCursorConflict
	}
	if before > 0 && after > 0 {
		return HistoryCursor{}, ErrCursorConflict
	}
	return HistoryCursor{Before: before, After: after}, nil
}

func PayloadHash(messageType, body string, mediaID int64) string {
	value := strings.Join([]string{strings.TrimSpace(messageType), body, strconv.FormatInt(mediaID, 10)}, "\x00")
	sum := sha256.Sum256([]byte(value))
	return hex.EncodeToString(sum[:])
}

func SamePayload(a, b string) bool { return strings.TrimSpace(a) != "" && strings.EqualFold(a, b) }

func CanRecall(age time.Duration) bool { return age >= 0 && age < 2*time.Minute }
