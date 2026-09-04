package friends

import (
	"errors"
	"strings"
)

var (
	ErrInvalidUserID = errors.New("friend.invalid_user_id")
	ErrSelfRelation  = errors.New("friend.self_relation")
	ErrBlocked       = errors.New("friend.blocked")
	ErrNotFound      = errors.New("friend.not_found")
	ErrForbidden     = errors.New("friend.forbidden")
	ErrInvalidState  = errors.New("friend.invalid_state")
)

const (
	FriendshipActive  = "active"
	FriendshipDeleted = "deleted"
	RequestPending    = "pending"
	RequestAccepted   = "accepted"
	RequestRejected   = "rejected"
	RequestCancelled  = "cancelled"
	VisibilityPrivate = "private"
	VisibilityFriends = "friends"
)

type Pair struct {
	LowID  int64
	HighID int64
}

type Profile struct {
	ID                           int64  `json:"id"`
	UserCode                     string `json:"userCode"`
	Nickname                     string `json:"nickname"`
	Avatar                       string `json:"avatar"`
	PersonalityType              *int   `json:"personalityType,omitempty"`
	PersonalityVisibility        string `json:"personalityVisibility"`
	PersonalityVisibilityVersion int64  `json:"personalityVisibilityVersion"`
}

type Friend struct {
	Profile
	FriendshipID int64  `json:"friendshipId"`
	CreatedAt    string `json:"createdAt"`
}

type FriendRequest struct {
	ID          int64    `json:"id"`
	RequesterID int64    `json:"requesterId"`
	AddresseeID int64    `json:"addresseeId"`
	Status      string   `json:"status"`
	Message     string   `json:"message"`
	CreatedAt   string   `json:"createdAt"`
	UpdatedAt   string   `json:"updatedAt"`
	Peer        *Profile `json:"peer,omitempty"`
}

type SearchResult struct {
	Profile
	Relation string `json:"relation"`
}

func NormalizePair(a, b int64) (Pair, error) {
	if a <= 0 || b <= 0 {
		return Pair{}, ErrInvalidUserID
	}
	if a == b {
		return Pair{}, ErrSelfRelation
	}
	if a < b {
		return Pair{LowID: a, HighID: b}, nil
	}
	return Pair{LowID: b, HighID: a}, nil
}

func RelationAllowed(blocked bool, friends bool) bool {
	return !blocked && friends
}

func CanViewPersonality(viewerID, targetID int64, visibility string, friends bool) bool {
	if viewerID > 0 && viewerID == targetID {
		return true
	}
	return visibility == VisibilityFriends && friends
}

func RestoreFriendshipState(status string) string {
	if strings.TrimSpace(status) == FriendshipDeleted || strings.TrimSpace(status) == FriendshipActive {
		return FriendshipActive
	}
	return FriendshipActive
}

func NormalizeVisibility(value string) (string, error) {
	value = strings.ToLower(strings.TrimSpace(value))
	if value != VisibilityPrivate && value != VisibilityFriends {
		return "", ErrInvalidState
	}
	return value, nil
}

func NormalizeLookup(value string) string { return strings.ToLower(strings.TrimSpace(value)) }
