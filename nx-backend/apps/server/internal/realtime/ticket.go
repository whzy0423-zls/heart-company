package realtime

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"database/sql"
	"encoding/base64"
	"errors"
	"time"
)

var ErrTicketInvalid = errors.New("realtime.ticket_invalid")

type TicketStore struct {
	db  *sql.DB
	ttl time.Duration
}

func NewTicketStore(db *sql.DB, ttl time.Duration) *TicketStore {
	if ttl <= 0 {
		ttl = 60 * time.Second
	}
	return &TicketStore{db: db, ttl: ttl}
}

func HashTicket(raw string) string {
	sum := sha256.Sum256([]byte(raw))
	return base64.RawURLEncoding.EncodeToString(sum[:])
}

func NewRawTicket() (string, error) {
	b := make([]byte, 32)
	if _, err := rand.Read(b); err != nil {
		return "", err
	}
	return base64.RawURLEncoding.EncodeToString(b), nil
}

func (s *TicketStore) Issue(ctx context.Context, userID int64, raw string) (time.Time, error) {
	if s == nil || s.db == nil || userID <= 0 || raw == "" {
		return time.Time{}, ErrTicketInvalid
	}
	expires := time.Now().Add(s.ttl)
	_, err := s.db.ExecContext(ctx, `INSERT INTO direct_realtime_tickets(user_id,token_hash,expires_at) VALUES($1,$2,$3)`, userID, HashTicket(raw), expires)
	return expires, err
}

func (s *TicketStore) Consume(ctx context.Context, raw string) (int64, error) {
	if s == nil || s.db == nil || raw == "" {
		return 0, ErrTicketInvalid
	}
	var userID int64
	err := s.db.QueryRowContext(ctx, `UPDATE direct_realtime_tickets SET consumed_at=now() WHERE token_hash=$1 AND consumed_at IS NULL AND expires_at>now() RETURNING user_id`, HashTicket(raw)).Scan(&userID)
	if errors.Is(err, sql.ErrNoRows) {
		return 0, ErrTicketInvalid
	}
	return userID, err
}
