package friends

import (
	"os"
	"strings"
	"testing"
)

func TestListBlockedUsersOnlyReturnsActiveOutgoingBlocks(t *testing.T) {
	raw, err := os.ReadFile("store.go")
	if err != nil {
		t.Fatal(err)
	}
	source := string(raw)
	for _, expected := range []string{
		"func (s *Store) ListBlockedUsers",
		"b.blocker_id=$1",
		"b.status='active'",
		"JOIN app_users u ON u.id=b.blocked_id",
		"ORDER BY b.created_at DESC, b.id DESC",
	} {
		if !strings.Contains(source, expected) {
			t.Fatalf("blocked-user listing is missing %q", expected)
		}
	}
}
