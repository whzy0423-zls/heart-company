package friends

import (
	"context"
	"crypto/rand"
	"database/sql"
	"encoding/base32"
	"errors"
	"fmt"
	"strings"
	"time"
)

type Store struct{ db *sql.DB }

func NewStore(db *sql.DB) *Store { return &Store{db: db} }

func (s *Store) requireDB() error {
	if s == nil || s.db == nil {
		return errors.New("friend database is not configured")
	}
	return nil
}

func (s *Store) CreateRequest(ctx context.Context, requesterID, addresseeID int64, message string) (FriendRequest, error) {
	if err := s.requireDB(); err != nil {
		return FriendRequest{}, err
	}
	if _, err := NormalizePair(requesterID, addresseeID); err != nil {
		return FriendRequest{}, err
	}
	message = strings.TrimSpace(message)
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return FriendRequest{}, err
	}
	defer tx.Rollback()
	if err := lockPairUsers(ctx, tx, requesterID, addresseeID); err != nil {
		return FriendRequest{}, err
	}
	blocked, err := blockedEither(ctx, tx, requesterID, addresseeID)
	if err != nil {
		return FriendRequest{}, err
	}
	if blocked {
		return FriendRequest{}, ErrBlocked
	}
	var active bool
	if err := tx.QueryRowContext(ctx, `SELECT EXISTS (SELECT 1 FROM friendships WHERE user_low_id=LEAST($1,$2) AND user_high_id=GREATEST($1,$2) AND status='active')`, requesterID, addresseeID).Scan(&active); err != nil {
		return FriendRequest{}, err
	}
	if active {
		return FriendRequest{}, fmt.Errorf("%w: already friends", ErrInvalidState)
	}
	var item FriendRequest
	err = tx.QueryRowContext(ctx, `
		INSERT INTO friend_requests(requester_id, addressee_id, status, message)
		VALUES ($1,$2,'pending',$3)
		ON CONFLICT (requester_id, addressee_id, status)
		DO UPDATE SET message=EXCLUDED.message, updated_at=now()
		RETURNING id, requester_id, addressee_id, status, message, created_at, updated_at`, requesterID, addresseeID, message).
		Scan(&item.ID, &item.RequesterID, &item.AddresseeID, &item.Status, &item.Message, &item.CreatedAt, &item.UpdatedAt)
	if err != nil {
		return FriendRequest{}, err
	}
	if err := tx.Commit(); err != nil {
		return FriendRequest{}, err
	}
	return item, nil
}

func (s *Store) RespondRequest(ctx context.Context, userID, requestID int64, accept bool) (FriendRequest, error) {
	if err := s.requireDB(); err != nil {
		return FriendRequest{}, err
	}
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return FriendRequest{}, err
	}
	defer tx.Rollback()
	var req FriendRequest
	if err := tx.QueryRowContext(ctx, `SELECT id, requester_id, addressee_id, status, message, created_at, updated_at FROM friend_requests WHERE id=$1 FOR UPDATE`, requestID).
		Scan(&req.ID, &req.RequesterID, &req.AddresseeID, &req.Status, &req.Message, &req.CreatedAt, &req.UpdatedAt); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return FriendRequest{}, ErrNotFound
		}
		return FriendRequest{}, err
	}
	if req.AddresseeID != userID || req.Status != RequestPending {
		return FriendRequest{}, ErrForbidden
	}
	if err := lockPairUsers(ctx, tx, req.RequesterID, req.AddresseeID); err != nil {
		return FriendRequest{}, err
	}
	blocked, err := blockedEither(ctx, tx, req.RequesterID, req.AddresseeID)
	if err != nil {
		return FriendRequest{}, err
	}
	if blocked {
		return FriendRequest{}, ErrBlocked
	}
	status := RequestRejected
	if accept {
		status = RequestAccepted
	}
	if err := tx.QueryRowContext(ctx, `UPDATE friend_requests SET status=$1, updated_at=now() WHERE id=$2 RETURNING id, requester_id, addressee_id, status, message, created_at, updated_at`, status, requestID).
		Scan(&req.ID, &req.RequesterID, &req.AddresseeID, &req.Status, &req.Message, &req.CreatedAt, &req.UpdatedAt); err != nil {
		return FriendRequest{}, err
	}
	if accept {
		if _, err := tx.ExecContext(ctx, `INSERT INTO friendships(user_low_id,user_high_id,status) VALUES (LEAST($1,$2),GREATEST($1,$2),'active') ON CONFLICT DO NOTHING`, req.RequesterID, req.AddresseeID); err != nil {
			return FriendRequest{}, err
		}
	}
	if err := tx.Commit(); err != nil {
		return FriendRequest{}, err
	}
	return req, nil
}

func (s *Store) DeleteFriend(ctx context.Context, userID, peerID int64) error {
	if err := s.requireDB(); err != nil {
		return err
	}
	pair, err := NormalizePair(userID, peerID)
	if err != nil {
		return err
	}
	result, err := s.db.ExecContext(ctx, `UPDATE friendships SET status='deleted', deleted_at=now(), updated_at=now() WHERE user_low_id=$1 AND user_high_id=$2 AND status='active'`, pair.LowID, pair.HighID)
	if err != nil {
		return err
	}
	n, err := result.RowsAffected()
	if err != nil {
		return err
	}
	if n == 0 {
		return ErrNotFound
	}
	return nil
}

func (s *Store) SetVisibility(ctx context.Context, userID int64, visibility string) (string, int64, error) {
	if err := s.requireDB(); err != nil {
		return "", 0, err
	}
	var err error
	visibility, err = NormalizeVisibility(visibility)
	if err != nil {
		return "", 0, err
	}
	var version int64
	err = s.db.QueryRowContext(ctx, `UPDATE app_users SET personality_visibility=$1, personality_visibility_version=personality_visibility_version+1, update_time=now() WHERE id=$2 RETURNING personality_visibility, personality_visibility_version`, visibility, userID).Scan(&visibility, &version)
	if errors.Is(err, sql.ErrNoRows) {
		return "", 0, ErrNotFound
	}
	return visibility, version, err
}

func (s *Store) RotateInviteCode(ctx context.Context, userID int64) (string, error) {
	if err := s.requireDB(); err != nil {
		return "", err
	}
	buf := make([]byte, 8)
	if _, err := rand.Read(buf); err != nil {
		return "", err
	}
	code := strings.TrimRight(base32.StdEncoding.EncodeToString(buf), "=")
	var saved string
	err := s.db.QueryRowContext(ctx, `UPDATE app_users SET invite_code=$1, update_time=now() WHERE id=$2 RETURNING invite_code`, code, userID).Scan(&saved)
	if errors.Is(err, sql.ErrNoRows) {
		return "", ErrNotFound
	}
	return saved, err
}

func (s *Store) GetOrCreateInviteCode(ctx context.Context, userID int64) (string, error) {
	if err := s.requireDB(); err != nil {
		return "", err
	}
	buf := make([]byte, 8)
	if _, err := rand.Read(buf); err != nil {
		return "", err
	}
	candidate := strings.TrimRight(base32.StdEncoding.EncodeToString(buf), "=")
	var code string
	err := s.db.QueryRowContext(ctx, `
		UPDATE app_users
		SET invite_code=COALESCE(NULLIF(btrim(invite_code),''), $1), update_time=now()
		WHERE id=$2
		RETURNING invite_code`, candidate, userID).Scan(&code)
	if errors.Is(err, sql.ErrNoRows) {
		return "", ErrNotFound
	}
	return code, err
}

func (s *Store) Search(ctx context.Context, viewerID int64, raw string) (SearchResult, error) {
	if err := s.requireDB(); err != nil {
		return SearchResult{}, err
	}
	query := strings.TrimSpace(raw)
	if query == "" {
		return SearchResult{}, ErrNotFound
	}
	var targetID int64
	if _, err := fmt.Sscan(query, &targetID); err != nil {
		targetID = 0
	}
	var item SearchResult
	var visibility string
	var visibilityVersion int64
	var enneagram sql.NullInt64
	err := s.db.QueryRowContext(ctx, `
		SELECT u.id, COALESCE(u.user_code,''), u.nickname, u.avatar, u.personality_visibility,
		       u.personality_visibility_version, c.enneagram,
		       CASE WHEN f.status='active' THEN 'friends' ELSE 'none' END
		FROM app_users u
		LEFT JOIN LATERAL (SELECT enneagram FROM app_user_cards WHERE app_user_id=u.id AND card_type='primary' AND status='active' ORDER BY update_time DESC, id DESC LIMIT 1) c ON TRUE
		LEFT JOIN friendships f ON f.user_low_id=LEAST(u.id,$1) AND f.user_high_id=GREATEST(u.id,$1) AND f.status='active'
		WHERE u.status='active'
		  AND NOT EXISTS (SELECT 1 FROM user_blocks b WHERE b.status='active' AND ((b.blocker_id=$1 AND b.blocked_id=u.id) OR (b.blocker_id=u.id AND b.blocked_id=$1)))
		  AND (lower(COALESCE(u.user_code,''))=lower($2) OR lower(COALESCE(u.invite_code,''))=lower($2) OR ($3 > 0 AND u.id=$3))
		LIMIT 1`, viewerID, query, targetID).
		Scan(&item.ID, &item.UserCode, &item.Nickname, &item.Avatar, &visibility, &visibilityVersion, &enneagram, &item.Relation)
	if errors.Is(err, sql.ErrNoRows) {
		return SearchResult{}, ErrNotFound
	}
	if err != nil {
		return SearchResult{}, err
	}
	if item.ID == viewerID {
		return SearchResult{}, ErrSelfRelation
	}
	item.PersonalityVisibility = visibility
	item.PersonalityVisibilityVersion = visibilityVersion
	if enneagram.Valid && CanViewPersonality(viewerID, item.ID, visibility, item.Relation == "friends") {
		value := int(enneagram.Int64)
		item.PersonalityType = &value
	}
	return item, nil
}

func (s *Store) ListRequests(ctx context.Context, userID int64, incoming bool) ([]FriendRequest, error) {
	if err := s.requireDB(); err != nil {
		return nil, err
	}
	column := "requester_id"
	if incoming {
		column = "addressee_id"
	}
	rows, err := s.db.QueryContext(ctx, fmt.Sprintf(`SELECT id, requester_id, addressee_id, status, message, created_at, updated_at FROM friend_requests WHERE %s=$1 ORDER BY created_at DESC, id DESC`, column), userID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	items := []FriendRequest{}
	for rows.Next() {
		var item FriendRequest
		if err := rows.Scan(&item.ID, &item.RequesterID, &item.AddresseeID, &item.Status, &item.Message, &item.CreatedAt, &item.UpdatedAt); err != nil {
			return nil, err
		}
		items = append(items, item)
	}
	return items, rows.Err()
}

func (s *Store) ListFriends(ctx context.Context, userID int64) ([]Friend, error) {
	if err := s.requireDB(); err != nil {
		return nil, err
	}
	rows, err := s.db.QueryContext(ctx, `
		SELECT f.id, f.created_at, u.id, COALESCE(u.user_code,''), u.nickname, u.avatar,
		       u.personality_visibility, u.personality_visibility_version, c.enneagram
		FROM friendships f
		JOIN app_users u ON u.id = CASE WHEN f.user_low_id=$1 THEN f.user_high_id ELSE f.user_low_id END
		LEFT JOIN LATERAL (SELECT enneagram FROM app_user_cards WHERE app_user_id=u.id AND card_type='primary' AND status='active' ORDER BY update_time DESC, id DESC LIMIT 1) c ON TRUE
		WHERE f.status='active' AND (f.user_low_id=$1 OR f.user_high_id=$1)
		  AND NOT EXISTS (SELECT 1 FROM user_blocks b WHERE b.status='active' AND ((b.blocker_id=$1 AND b.blocked_id=u.id) OR (b.blocker_id=u.id AND b.blocked_id=$1)))
		ORDER BY f.updated_at DESC, f.id DESC`, userID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	items := []Friend{}
	for rows.Next() {
		var item Friend
		var created time.Time
		var visibility string
		var version int64
		var enneagram sql.NullInt64
		if err := rows.Scan(&item.FriendshipID, &created, &item.ID, &item.UserCode, &item.Nickname, &item.Avatar, &visibility, &version, &enneagram); err != nil {
			return nil, err
		}
		item.CreatedAt = formatTime(created)
		item.PersonalityVisibility = visibility
		item.PersonalityVisibilityVersion = version
		if enneagram.Valid && CanViewPersonality(userID, item.ID, visibility, true) {
			value := int(enneagram.Int64)
			item.PersonalityType = &value
		}
		items = append(items, item)
	}
	return items, rows.Err()
}

func (s *Store) Report(ctx context.Context, reporterID, reportedID int64, reason, details string) (int64, error) {
	if err := s.requireDB(); err != nil {
		return 0, err
	}
	if _, err := NormalizePair(reporterID, reportedID); err != nil {
		return 0, err
	}
	var id int64
	err := s.db.QueryRowContext(ctx, `INSERT INTO user_reports(reporter_id,reported_id,reason,details) VALUES($1,$2,$3,$4) RETURNING id`, reporterID, reportedID, strings.TrimSpace(reason), strings.TrimSpace(details)).Scan(&id)
	return id, err
}

func (s *Store) Block(ctx context.Context, userID, peerID int64, reason string) error {
	if err := s.requireDB(); err != nil {
		return err
	}
	if _, err := NormalizePair(userID, peerID); err != nil {
		return err
	}
	_, err := s.db.ExecContext(ctx, `INSERT INTO user_blocks(blocker_id,blocked_id,status,reason) VALUES($1,$2,'active',$3) ON CONFLICT (blocker_id,blocked_id) WHERE status='active' DO UPDATE SET reason=EXCLUDED.reason`, userID, peerID, strings.TrimSpace(reason))
	return err
}

func (s *Store) Unblock(ctx context.Context, userID, peerID int64) error {
	if err := s.requireDB(); err != nil {
		return err
	}
	_, err := s.db.ExecContext(ctx, `UPDATE user_blocks SET status='removed', removed_at=now() WHERE blocker_id=$1 AND blocked_id=$2 AND status='active'`, userID, peerID)
	return err
}

func lockPairUsers(ctx context.Context, tx *sql.Tx, a, b int64) error {
	pair, err := NormalizePair(a, b)
	if err != nil {
		return err
	}
	rows, err := tx.QueryContext(ctx, `SELECT id FROM app_users WHERE id IN ($1,$2) ORDER BY id FOR UPDATE`, pair.LowID, pair.HighID)
	if err != nil {
		return err
	}
	defer rows.Close()
	count := 0
	for rows.Next() {
		count++
	}
	if err := rows.Err(); err != nil {
		return err
	}
	if count != 2 {
		return ErrNotFound
	}
	return nil
}

func blockedEither(ctx context.Context, tx *sql.Tx, a, b int64) (bool, error) {
	var blocked bool
	err := tx.QueryRowContext(ctx, `SELECT EXISTS (SELECT 1 FROM user_blocks WHERE status='active' AND ((blocker_id=$1 AND blocked_id=$2) OR (blocker_id=$2 AND blocked_id=$1)))`, a, b).Scan(&blocked)
	return blocked, err
}

func formatTime(t time.Time) string {
	if t.IsZero() {
		return ""
	}
	return t.Format(time.RFC3339)
}
