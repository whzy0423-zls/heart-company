package friends

import (
	"errors"
	"testing"
)

func TestNormalizePairOrdersIDs(t *testing.T) {
	pair, err := NormalizePair(42, 7)
	if err != nil {
		t.Fatal(err)
	}
	if pair.LowID != 7 || pair.HighID != 42 {
		t.Fatalf("unexpected pair: %+v", pair)
	}
}

func TestNormalizePairRejectsSelfAndInvalidIDs(t *testing.T) {
	if _, err := NormalizePair(9, 9); !errors.Is(err, ErrSelfRelation) {
		t.Fatalf("expected ErrSelfRelation, got %v", err)
	}
	if _, err := NormalizePair(0, 9); !errors.Is(err, ErrInvalidUserID) {
		t.Fatalf("expected ErrInvalidUserID, got %v", err)
	}
}

func TestBlockPrecedence(t *testing.T) {
	if got := RelationAllowed(true, true); got {
		t.Fatal("a block must take precedence over friendship")
	}
	if got := RelationAllowed(false, true); !got {
		t.Fatal("friends should be allowed when neither side is blocked")
	}
}

func TestRestoreFriendshipStatePreservesHistory(t *testing.T) {
	if got := RestoreFriendshipState("deleted"); got != FriendshipActive {
		t.Fatalf("expected deleted friendship to restore as active, got %q", got)
	}
	if got := RestoreFriendshipState("active"); got != FriendshipActive {
		t.Fatalf("expected active friendship to remain active, got %q", got)
	}
}

func TestVisibilityRequiresSelfOrFriend(t *testing.T) {
	if CanViewPersonality(10, 11, VisibilityFriends, false) {
		t.Fatal("non-friends must not see a friends-only personality")
	}
	if !CanViewPersonality(10, 11, VisibilityFriends, true) {
		t.Fatal("friends should see a friends-only personality")
	}
	if CanViewPersonality(10, 11, VisibilityPrivate, true) {
		t.Fatal("private personality must remain hidden from friends")
	}
	if !CanViewPersonality(10, 10, VisibilityPrivate, false) {
		t.Fatal("a user can always see their own personality")
	}
}

func TestNormalizeLookupIsCaseInsensitiveAndTrimmed(t *testing.T) {
	if got := NormalizeLookup("  Invite-AbC  "); got != "invite-abc" {
		t.Fatalf("unexpected lookup normalization: %q", got)
	}
}
